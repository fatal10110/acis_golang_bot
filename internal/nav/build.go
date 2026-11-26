package nav

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"math"
	"math/bits"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// BuildConfig is what Build builds the hierarchy from.
type BuildConfig struct {
	// Engine is the geodata. Build reads its static regions only, every
	// door open.
	Engine *engine.Engine
	// Doors are the door templates (doors.xml) whose footprints tag the
	// edges that cross them.
	Doors []*door.Template
	// Water is the water zones no edge enters or leaves (nil: none); Build
	// sets the key's WaterHash from it.
	Water *Water
	// Clusters lists the clusters to build; a crossing into a cluster not
	// listed is not an entrance. `bots navbuild` lists every cluster of the
	// loaded regions (RegionClusters).
	Clusters []ClusterKey
	// ClusterCells is the cluster side in cells, DefaultClusterCells when 0.
	ClusterCells int
	// Workers caps the clusters built at once, 1 when 0. `bots navbuild`
	// uses half the cores (docs/home.md, resource cap).
	Workers int
	// Key carries the geodata cache CRC and the doors.xml hash the image is
	// keyed by; Build sets its ClusterCells and WaterHash.
	Key NavKey
	// Log reports skipped doors; nil discards.
	Log *slog.Logger

	// onCluster, when set, runs after each cluster a worker builds: a test
	// seam for cancellation.
	onCluster func()
}

// BuildStats are the measured sizes N3 reports and sets the nav cache bound
// from (docs/navigation.md, "Measured by N3").
type BuildStats struct {
	Clusters, Nodes, IntraEdges, CrossEdges, DoorRefs int
	SCCs, LargestSCC, CondEdges                       int
	Doors, SkippedDoors                               int
	Bytes                                             int
	// OneWayEdges join two abstract SCCs: crossings and pairs joined
	// inside a cluster.
	OneWayEdges int
}

// RegionClusters returns the clusters of one geodata region tile.
func RegionClusters(regionX, regionY, clusterCells int) []ClusterKey {
	cells := block.RegionBlocksX * block.CellsX
	n := cells / clusterCells
	x0 := (regionX - engine.TileXMin) * cells / clusterCells
	y0 := (regionY - engine.TileYMin) * cells / clusterCells
	keys := make([]ClusterKey, 0, n*n)
	for y := range n {
		for x := range n {
			keys = append(keys, ClusterKey{int32(x0 + x), int32(y0 + y)})
		}
	}
	return keys
}

// Build builds the hierarchy and returns its nav cache image. Clusters are
// built by at most cfg.Workers goroutines, each with its own scratch of a
// few clusters' size; Build joins them all before it returns, on success,
// error or ctx cancellation alike.
func Build(ctx context.Context, cfg BuildConfig) ([]byte, BuildStats, error) {
	size := cfg.ClusterCells
	if size <= 0 {
		size = DefaultClusterCells
	}
	if size%block.CellsX != 0 || size > 1<<15 {
		return nil, BuildStats{}, fmt.Errorf("nav: cluster side %d is not a multiple of %d", size, block.CellsX)
	}
	keys := slices.Clone(cfg.Clusters)
	slices.SortFunc(keys, cmpKey)
	keys = slices.Compact(keys)
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	static := cfg.Engine.Static()
	b := &builder{size: size, keys: keys, p: Planner{Engine: static, Water: cfg.Water}, built: make(map[ClusterKey]bool, len(keys))}
	for _, k := range keys {
		b.built[k] = true
	}
	var stats BuildStats
	stats.Doors, stats.SkippedDoors = b.loadDoors(cfg.Doors, static, log)

	workers := max(cfg.Workers, 1)
	progress := func(phase string) func() {
		var done atomic.Int64
		step := int64(max(len(keys)/20, 1))
		return func() {
			if d := done.Add(1); d%step == 0 || d == int64(len(keys)) {
				log.Info("nav: clusters done", "phase", phase, "done", d, "of", len(keys))
			}
			if cfg.onCluster != nil {
				cfg.onCluster()
			}
		}
	}
	outs := make([]clusterOut, len(keys))
	if err := b.parallel(ctx, workers, progress("entrances"), func(s *buildScratch, i int) { outs[i] = b.cluster(s, i) }); err != nil {
		return nil, stats, err
	}
	l, err := b.link(outs, &stats)
	if err != nil {
		return nil, stats, err
	}
	// Edge costs only between nodes of one abstract SCC: the only edges a
	// route or distance search follows; the condensation holds the rest.
	if err := b.parallel(ctx, workers, progress("costs"), func(s *buildScratch, i int) { b.intraEdges(s, i, &outs[i], l) }); err != nil {
		return nil, stats, err
	}
	key := cfg.Key
	key.ClusterCells = size
	key.WaterHash = cfg.Water.Hash()
	image, err := b.encode(outs, l, key, &stats)
	if err != nil {
		return nil, stats, err
	}
	return image, stats, nil
}

func cmpKey(a, b ClusterKey) int {
	if c := cmp.Compare(a.Y, b.Y); c != 0 {
		return c
	}
	return cmp.Compare(a.X, b.X)
}

// builder holds what every worker reads: immutable once Build starts the
// workers.
type builder struct {
	size  int
	keys  []ClusterKey
	p     Planner
	built map[ClusterKey]bool
	// doorCells maps a door footprint cell to the doors covering it.
	doorCells map[[2]int][]doorFoot
}

// doorFoot is one door over one footprint cell: the door blocks the
// cell's layer at height z only.
type doorFoot struct {
	id int32
	z  int16
}

// loadDoors builds every door's footprint over the static geodata, as the
// server shapes a door (dynamic.NewDoorObject) and lifts it into the
// overlay (dynamic block rebuild): each fully blocked cell of the door
// blocks one layer, the one nearest the door's floor GeoZ.
func (b *builder) loadDoors(templates []*door.Template, static *engine.Engine, log *slog.Logger) (doors, skipped int) {
	b.doorCells = make(map[[2]int][]doorFoot)
	for _, t := range templates {
		if t.ID < math.MinInt32 || t.ID > math.MaxInt32 {
			log.Warn("nav: door footprint skipped", "door", t.ID, "err", "id outside int32")
			skipped++
			continue
		}
		obj, err := dynamic.NewDoorObject(t, static)
		if err != nil {
			log.Warn("nav: door footprint skipped", "door", t.ID, "err", err)
			skipped++
			continue
		}
		doors++
		for ix, col := range obj.GeoData() {
			for iy, n := range col {
				if n == block.NoDirections {
					gx, gy := obj.GeoX()+ix, obj.GeoY()+iy
					k := [2]int{gx, gy}
					f := doorFoot{id: int32(t.ID), z: static.HeightNearest(gx, gy, obj.GeoZ())}
					b.doorCells[k] = append(b.doorCells[k], f)
				}
			}
		}
	}
	return doors, skipped
}

// doorsAt appends the doors whose footprint holds c.
func (b *builder) doorsAt(dst []int32, c Cell) []int32 {
	for _, f := range b.doorCells[[2]int{c.X, c.Y}] {
		if c.Z == f.z {
			dst = append(dst, f.id)
		}
	}
	return dst
}

func (b *builder) clusterOf(c Cell) ClusterKey {
	return ClusterKey{int32(floorDiv(c.X, b.size)), int32(floorDiv(c.Y, b.size))}
}

func (b *builder) clusterIndex(k ClusterKey) int {
	i, ok := slices.BinarySearchFunc(b.keys, k, cmpKey)
	if !ok {
		return -1
	}
	return i
}

// runGap is the widest gap along a border side that one entrance node
// spans: border cells of one local SCC at most this many positions apart
// join one node. Joining them is exact, since they are mutually reachable
// inside the cluster; it keeps a border broken up by rocks and walls from
// costing a node, and squared edges, per piece.
const runGap = 16

// clusterOut is one built cluster, in local node indices.
type clusterOut struct {
	nodes      []buildNode
	reach      [][2]int32  // node pairs joined inside the cluster, transitively reduced
	edges      []buildEdge // sorted by from, then to
	edgeDoors  []int32
	cross      []buildCross
	crossDoors []int32      // the door sets of the crossings
	border     []borderCell // sorted by side, pos, z
}

type buildNode struct {
	rep        Cell
	side       int
	pos0, pos1 int
}

type buildEdge struct {
	from, to       int32
	open, free     uint32
	doorOff, doorN int32 // the door set, in clusterOut.edgeDoors
}

// buildCross is a crossing out of the cluster: from a cell of node from
// into cell (x, y, z) of the next cluster through side. Its door set (the
// doors at either end) is crossDoors[doorOff:doorOff+doorN]. A build holds
// one per border cell of the world, so the record is kept small.
type buildCross struct {
	from    int32
	x, y    int32
	doorOff int32
	z       int16
	// side is the border side in its low crossSideBits bits; the bits above
	// them are the crossing's dropCost in costDrop units (0: no drop), capped
	// at maxCrossDrop.
	side  uint8
	doorN uint8
}

// crossSideBits is the width of the side in buildCross.side, and
// maxCrossDrop the most costDrop units the bits above it hold.
const (
	crossSideBits = 2
	maxCrossDrop  = 1<<(8-crossSideBits) - 1
)

func (x buildCross) to() Cell { return Cell{X: int(x.x), Y: int(x.y), Z: x.z} }

// borderCell is one cell an entrance node holds.
type borderCell struct {
	node int32
	pos  uint16
	z    int16
	side uint8
}

func cmpBorder(a, b borderCell) int {
	// side, then pos, then z
	if c := cmp.Compare(a.side, b.side); c != 0 {
		return c
	}
	if c := cmp.Compare(a.pos, b.pos); c != 0 {
		return c
	}
	return cmp.Compare(a.z, b.z)
}

func (o *clusterOut) nodeAt(side, pos int, z int16) (int32, bool) {
	i, ok := slices.BinarySearchFunc(o.border, borderCell{side: uint8(side), pos: uint16(pos), z: z}, cmpBorder)
	if !ok {
		return 0, false
	}
	return o.border[i].node, true
}

// parallel runs fn on every cluster index on at most workers goroutines,
// each with its own scratch, and joins them all before it returns. On ctx
// cancellation the workers stop at their next cluster.
func (b *builder) parallel(ctx context.Context, workers int, onCluster func(), fn func(s *buildScratch, i int)) error {
	var (
		next    atomic.Int64
		wg      sync.WaitGroup
		errOnce sync.Once
		failed  error
	)
	for range min(workers, max(len(b.keys), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &buildScratch{}
			for {
				i := int(next.Add(1) - 1)
				if i >= len(b.keys) {
					return
				}
				if err := ctx.Err(); err != nil {
					errOnce.Do(func() { failed = err })
					return
				}
				fn(s, i)
				if onCluster != nil {
					onCluster()
				}
			}
		}()
	}
	wg.Wait()
	return failed
}

// buildScratch is one worker's working memory.
type buildScratch struct {
	g       localGraph
	scc     sccScratch
	entries []borderEntry
	out     []outCrossing
	wStart  []int32
	wTo     []int32
	wCost   []int32
	steps   []step
	dist    []uint32
	parent  []int32
	heap    []distItem
	isDoor  []bool
	target  []bool
	via     []bool
	doors   []int32
	// reach edges
	holders  []int32
	holderOf map[int32]int
	bits     []uint64
	lstart   []int32
	lto      []int32
	lpos     []int32
}

type borderEntry struct {
	side, pos int
	cell      int32
	label     int32
}

type outCrossing struct {
	cell int32
	side int
	to   Cell
}

// sidePos returns the column of side position pos and the cell just
// outside it.
func (b *builder) sidePos(g *localGraph, side, pos int) (lx, ly, ox, oy int) {
	last := b.size - 1
	switch side {
	case sideEast:
		lx, ly, ox, oy = last, pos, b.size, pos
	case sideWest:
		lx, ly, ox, oy = 0, pos, -1, pos
	case sideSouth:
		lx, ly, ox, oy = pos, last, pos, b.size
	default:
		lx, ly, ox, oy = pos, 0, pos, -1
	}
	return lx, ly, g.x0 + ox, g.y0 + oy
}

// cluster builds cluster i: its local SCCs, its entrance nodes, the
// crossings out of it and the directed edges between its nodes.
func (b *builder) cluster(s *buildScratch, i int) clusterOut {
	k := b.keys[i]
	g := &s.g
	g.load(&b.p, int(k.X)*b.size, int(k.Y)*b.size, b.size)
	s.scc.localSCCs(g)
	label := s.scc.label

	// Border cells: every cell a crossing leaves the cluster from or enters
	// it on, per side, into or from a built cluster.
	s.entries, s.out = s.entries[:0], s.out[:0]
	for side, dir := range cardinals {
		for pos := range b.size {
			lx, ly, ox, oy := b.sidePos(g, side, pos)
			col := ly*b.size + lx
			outside := Cell{X: ox, Y: oy}
			if !b.built[b.clusterOf(outside)] {
				continue
			}
			mark := func(c int32) {
				for _, e := range s.entries[max(len(s.entries)-2*block.MaxLayers, 0):] {
					if e.side == side && e.pos == pos && e.cell == c {
						return
					}
				}
				s.entries = append(s.entries, borderEntry{side: side, pos: pos, cell: c, label: label[c]})
			}
			for c := g.colStart[col]; c < g.colStart[col+1]; c++ {
				if to, _, ok := b.p.cross(g.cell(c), g.nswe[c], dir); ok {
					s.out = append(s.out, outCrossing{cell: c, side: side, to: to})
					mark(c)
				}
			}
			if !b.p.hasGeo(ox, oy) {
				continue
			}
			for _, l := range b.p.Engine.AppendLayers(g.layers[:0], ox, oy) {
				if to, _, ok := b.p.cross(Cell{X: ox, Y: oy, Z: l.Height}, l.NSWE, cardinals[opposite(side)]); ok {
					if c := g.index(to); c >= 0 {
						mark(c)
					}
				}
			}
		}
	}

	// Entrance nodes: runs of neighbouring border cells on one side in one
	// local SCC, so every node's cells are mutually reachable inside the
	// cluster and one-way terrain keeps its own nodes.
	slices.SortFunc(s.entries, func(a, c borderEntry) int {
		if r := cmp.Compare(a.side, c.side); r != 0 {
			return r
		}
		if r := cmp.Compare(a.label, c.label); r != 0 {
			return r
		}
		if r := cmp.Compare(a.pos, c.pos); r != 0 {
			return r
		}
		return cmp.Compare(g.z[a.cell], g.z[c.cell])
	})
	var out clusterOut
	for lo := 0; lo < len(s.entries); {
		hi := lo + 1
		for hi < len(s.entries) && s.entries[hi].side == s.entries[lo].side && s.entries[hi].label == s.entries[lo].label &&
			s.entries[hi].pos <= s.entries[hi-1].pos+runGap {
			hi++
		}
		n := int32(len(out.nodes))
		out.nodes = append(out.nodes, buildNode{
			rep:  g.cell(s.entries[(lo+hi-1)/2].cell),
			side: s.entries[lo].side,
			pos0: s.entries[lo].pos,
			pos1: s.entries[hi-1].pos,
		})
		for _, e := range s.entries[lo:hi] {
			out.border = append(out.border, borderCell{side: uint8(e.side), pos: uint16(e.pos), z: g.z[e.cell], node: n})
		}
		lo = hi
	}
	slices.SortFunc(out.border, cmpBorder)

	for _, oc := range s.out {
		c := g.cell(oc.cell)
		lx, ly := c.X-g.x0, c.Y-g.y0
		pos := ly
		if oc.side == sideSouth || oc.side == sideNorth {
			pos = lx
		}
		from, _ := out.nodeAt(oc.side, pos, c.Z)
		off := len(out.crossDoors)
		out.crossDoors = b.doorsAt(b.doorsAt(out.crossDoors, c), oc.to)
		x := buildCross{
			from: from, x: int32(oc.to.X), y: int32(oc.to.Y), z: oc.to.Z, side: uint8(oc.side),
			doorOff: int32(off), doorN: uint8(len(out.crossDoors) - off),
		}
		x.side |= uint8(min(dropCost(c.Z, oc.to.Z)/costDrop, maxCrossDrop)) << crossSideBits
		out.cross = append(out.cross, x)
	}

	b.reachEdges(s, &out)
	return out
}

// reachEdges records which of the cluster's nodes reach which inside it:
// the nodes of one local SCC in a cycle, and between local SCCs the
// transitive reduction of their reachability over the local condensation.
// That is exact for the abstract SCCs at a few edges per node, where all
// pairs would grow with the square of a fragmented border's node count.
func (b *builder) reachEdges(s *buildScratch, out *clusterOut) {
	if len(out.nodes) < 2 {
		return
	}
	g := &s.g
	label := s.scc.label
	// The labels holding nodes, each with its nodes.
	s.holders = s.holders[:0]
	clear(s.holderOf)
	if s.holderOf == nil {
		s.holderOf = map[int32]int{}
	}
	byHolder := make([][]int32, 0, len(out.nodes))
	for u, nd := range out.nodes {
		l := label[g.index(nd.rep)]
		h, ok := s.holderOf[l]
		if !ok {
			h = len(s.holders)
			s.holderOf[l] = h
			s.holders = append(s.holders, l)
			byHolder = append(byHolder, nil)
		}
		byHolder[h] = append(byHolder[h], int32(u))
	}
	for _, nodes := range byHolder {
		for i := range nodes {
			if len(nodes) > 1 {
				out.reach = append(out.reach, [2]int32{nodes[i], nodes[(i+1)%len(nodes)]})
			}
		}
	}
	k := len(s.holders)
	if k < 2 {
		return
	}
	// Reachability between holders over the local condensation, whose
	// edges all go from a higher label to a lower one.
	words := (k + 63) / 64
	nLabels := 0
	for _, l := range label[:g.cells()] {
		nLabels = max(nLabels, int(l)+1)
	}
	s.bits = fill(s.bits, nLabels*words, 0)
	for h, l := range s.holders {
		s.bits[int(l)*words+h/64] |= 1 << (h % 64)
	}
	s.lstart = fill(s.lstart, nLabels+1, 0)
	n := g.cells()
	for v := range n {
		for d := range 4 {
			if w := g.next[4*v+d]; w >= 0 && label[w] != label[v] {
				s.lstart[label[v]+1]++
			}
		}
	}
	for l := range nLabels {
		s.lstart[l+1] += s.lstart[l]
	}
	s.lto = resize(s.lto, int(s.lstart[nLabels]))
	s.lpos = append(s.lpos[:0], s.lstart[:nLabels]...)
	for v := range n {
		for d := range 4 {
			if w := g.next[4*v+d]; w >= 0 && label[w] != label[v] {
				s.lto[s.lpos[label[v]]] = label[w]
				s.lpos[label[v]]++
			}
		}
	}
	for l := range nLabels {
		row := s.bits[l*words : (l+1)*words]
		for _, m := range s.lto[s.lstart[l]:s.lstart[l+1]] {
			for w := range words {
				row[w] |= s.bits[int(m)*words+w]
			}
		}
	}
	// reach[h]: the holders h reaches, itself excluded; then drop every
	// holder reached through another (the transitive reduction of a DAG).
	reach := make([]uint64, k*words)
	for h, l := range s.holders {
		copy(reach[h*words:(h+1)*words], s.bits[int(l)*words:(int(l)+1)*words])
		reach[h*words+h/64] &^= 1 << (h % 64)
	}
	imm := make([]uint64, words)
	for h := range k {
		row := reach[h*words : (h+1)*words]
		copy(imm, row)
		for w, word := range row {
			for word != 0 {
				t := w*64 + bits.TrailingZeros64(word)
				word &= word - 1
				for x := range words {
					imm[x] &^= reach[t*words+x]
				}
			}
		}
		for w, word := range imm {
			for word != 0 {
				t := w*64 + bits.TrailingZeros64(word)
				word &= word - 1
				out.reach = append(out.reach, [2]int32{byHolder[h][0], byHolder[t][0]})
			}
		}
	}
}

// intraEdges adds the directed edges between the cluster's nodes: for each
// ordered pair joined inside the cluster, the cheapest cost with every door
// open and the doors that path crosses, and the cheapest cost crossing no
// door (docs/navigation.md, "Doors in the hierarchy").
func (b *builder) intraEdges(s *buildScratch, ci int, out *clusterOut, l *linked) {
	// Group the nodes by abstract SCC; only groups of two or more need
	// costs.
	groups := map[int32][]int32{}
	for u := range out.nodes {
		x := l.scc[l.base[ci]+u]
		groups[x] = append(groups[x], int32(u))
	}
	need := false
	for _, grp := range groups {
		need = need || len(grp) > 1
	}
	if !need {
		return
	}
	g := &s.g
	k := b.keys[ci]
	g.load(&b.p, int(k.X)*b.size, int(k.Y)*b.size, b.size)
	n := g.cells()
	s.wStart = resize(s.wStart, n+1)
	s.wTo, s.wCost = s.wTo[:0], s.wCost[:0]
	s.isDoor = resize(s.isDoor, n)
	anyDoor := false
	for i := range int32(n) {
		s.wStart[i] = int32(len(s.wTo))
		c := g.cell(i)
		s.steps = g.appendSteps(s.steps[:0], i)
		for _, st := range s.steps {
			s.wTo = append(s.wTo, st.to)
			s.wCost = append(s.wCost, st.cost)
		}
		s.doors = b.doorsAt(s.doors[:0], c)
		s.isDoor[i] = len(s.doors) > 0
		anyDoor = anyDoor || s.isDoor[i]
	}
	s.wStart[n] = int32(len(s.wTo))

	reps := make([]int32, len(out.nodes))
	for u, nd := range out.nodes {
		reps[u] = g.index(nd.rep)
	}
	free := make([]uint32, len(out.nodes))
	var targets []int32
	for u := range out.nodes {
		grp := groups[l.scc[l.base[ci]+u]]
		if len(grp) < 2 {
			continue
		}
		targets = targets[:0]
		for _, v := range grp {
			targets = append(targets, reps[v])
		}
		if anyDoor {
			b.dijkstra(s, reps[u], targets, true)
			for _, v := range grp {
				free[v] = s.dist[reps[v]]
			}
		}
		b.dijkstra(s, reps[u], targets, false)
		for _, vv := range grp {
			v, r := int(vv), reps[vv]
			// An edge whose shortest path runs through another node's
			// representative is the sum of two edges kept anyway: drop it,
			// which keeps every distance exact (edge pruning). Kept where a
			// door lies, so the door-free costs stay per pair.
			if v == u || s.dist[r] == noCost || !anyDoor && s.via[r] {
				continue
			}
			e := buildEdge{from: int32(u), to: int32(v), open: s.dist[r], free: s.dist[r]}
			if anyDoor {
				e.free = free[v]
				s.doors = s.doors[:0]
				for c := r; c >= 0; c = s.parent[c] {
					if s.isDoor[c] {
						s.doors = b.doorsAt(s.doors, g.cell(c))
					}
				}
				slices.Sort(s.doors)
				doors := slices.Compact(s.doors)
				e.doorOff, e.doorN = int32(len(out.edgeDoors)), int32(len(doors))
				out.edgeDoors = append(out.edgeDoors, doors...)
			}
			out.edges = append(out.edges, e)
		}
	}
}

type distItem struct {
	d uint32
	v int32
}

// dijkstra runs from cell src over the cluster's weighted cells, skipping
// door cells when noDoors, until every target is settled.
func (b *builder) dijkstra(s *buildScratch, src int32, targets []int32, noDoors bool) {
	n := s.g.cells()
	s.dist = fill(s.dist, n, uint32(noCost))
	s.parent = fill(s.parent, n, -1)
	s.heap = s.heap[:0]
	if noDoors && s.isDoor[src] {
		return
	}
	s.dist[src] = 0
	s.via = fill(s.via, n, false)
	s.heap = pushDist(s.heap, distItem{0, src})
	s.target = resize(s.target, n)
	left := 0
	for _, t := range targets {
		if !s.target[t] {
			s.target[t] = true
			left++
		}
	}
	defer func() {
		for _, t := range targets {
			s.target[t] = false
		}
	}()
	for len(s.heap) > 0 && left > 0 {
		var it distItem
		it, s.heap = popDist(s.heap)
		if it.d != s.dist[it.v] {
			continue
		}
		if s.target[it.v] {
			left--
		}
		for k := s.wStart[it.v]; k < s.wStart[it.v+1]; k++ {
			w := s.wTo[k]
			if noDoors && s.isDoor[w] {
				continue
			}
			if d := it.d + uint32(s.wCost[k]); d < s.dist[w] {
				s.dist[w], s.parent[w] = d, it.v
				s.via[w] = s.via[it.v] || it.v != src && s.target[it.v]
				s.heap = pushDist(s.heap, distItem{d, w})
			}
		}
	}
}

func pushDist(h []distItem, it distItem) []distItem {
	h = append(h, it)
	for i := len(h) - 1; i > 0; {
		p := (i - 1) / 2
		if h[p].d <= h[i].d {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
	return h
}

func popDist(h []distItem) (distItem, []distItem) {
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for i := 0; ; {
		l, r, m := 2*i+1, 2*i+2, i
		if l < last && h[l].d < h[m].d {
			m = l
		}
		if r < last && h[r].d < h[m].d {
			m = r
		}
		if m == i {
			return top, h
		}
		h[i], h[m] = h[m], h[i]
		i = m
	}
}

// globalEdge is one abstract edge, in global node ids.
type globalEdge struct {
	to         uint32
	open, free uint32
	doors      []int32
}

// linked is the abstract graph before costs: node ids, each node's SCC and
// the condensation, and the crossing and one-way edges.
type linked struct {
	base  []int // first global node of each cluster
	scc   []int32
	nSCC  int
	cond  [][]uint32
	cross [][]globalEdge // per node
}

// link joins the clusters into the abstract graph and labels its SCCs, every
// door open, and its condensation.
func (b *builder) link(outs []clusterOut, stats *BuildStats) (*linked, error) {
	l := &linked{base: make([]int, len(outs)+1)}
	for i, o := range outs {
		l.base[i+1] = l.base[i] + len(o.nodes)
	}
	nNodes := l.base[len(outs)]
	adj := make([][]uint32, nNodes)
	l.cross = make([][]globalEdge, nNodes)
	for ci := range outs {
		o := &outs[ci]
		for _, r := range o.reach {
			adj[l.base[ci]+int(r[0])] = append(adj[l.base[ci]+int(r[0])], uint32(l.base[ci]+int(r[1])))
		}
		for _, x := range o.cross {
			xto := x.to()
			ti := b.clusterIndex(b.clusterOf(xto))
			if ti < 0 {
				return nil, fmt.Errorf("nav: crossing into unbuilt cluster at %d,%d", xto.X, xto.Y)
			}
			side := opposite(int(x.side & (1<<crossSideBits - 1)))
			ox, oy := int(b.keys[ti].X)*b.size, int(b.keys[ti].Y)*b.size
			pos := xto.Y - oy
			if side == sideSouth || side == sideNorth {
				pos = xto.X - ox
			}
			to, ok := outs[ti].nodeAt(side, pos, xto.Z)
			if !ok {
				return nil, fmt.Errorf("nav: crossing target %d,%d,%d is no entrance", xto.X, xto.Y, xto.Z)
			}
			from := l.base[ci] + int(x.from)
			cost := uint32(max(costCardinal, heuristic(o.nodes[x.from].rep, outs[ti].nodes[to].rep)))
			cost += uint32(x.side>>crossSideBits) * costDrop
			e := globalEdge{to: uint32(l.base[ti] + int(to)), open: cost, free: cost}
			if x.doorN > 0 {
				e.doors = slices.Clone(o.crossDoors[x.doorOff : x.doorOff+int32(x.doorN)])
				e.free = noCost
			}
			if addEdge(&l.cross[from], e) {
				adj[from] = append(adj[from], e.to)
			}
		}
	}
	// The crossings and border cells are resolved: free them.
	for ci := range outs {
		o := &outs[ci]
		o.cross, o.crossDoors, o.border = nil, nil, nil
	}

	var scc sccScratch
	l.nSCC = scc.run(nNodes,
		func(v int32) int32 { return int32(len(adj[v])) },
		func(v, k int32) int32 { return int32(adj[v][k]) })
	l.scc = scc.label
	sizes := make([]int, l.nSCC)
	l.cond = make([][]uint32, l.nSCC)
	for v := range nNodes {
		sv := l.scc[v]
		sizes[sv]++
		for _, t := range adj[v] {
			if st := l.scc[t]; st != sv {
				l.cond[sv] = append(l.cond[sv], uint32(st))
			}
		}
	}
	for i := range l.cond {
		slices.Sort(l.cond[i])
		l.cond[i] = slices.Compact(l.cond[i])
		stats.CondEdges += len(l.cond[i])
	}
	// A node pair joined inside a cluster across two abstract SCCs is a
	// one-way edge a route that may take drops follows (RouteOneWay),
	// costed as a crossing is: its straight line, which the leg refinement
	// settles. Pairs inside one SCC get their costs from intraEdges.
	for ci := range outs {
		o := &outs[ci]
		for _, r := range o.reach {
			from, to := l.base[ci]+int(r[0]), l.base[ci]+int(r[1])
			if l.scc[from] == l.scc[to] {
				continue
			}
			cost := uint32(max(costCardinal, heuristic(o.nodes[r[0]].rep, o.nodes[r[1]].rep)))
			addEdge(&l.cross[from], globalEdge{to: uint32(to), open: cost, free: cost})
		}
		o.reach = nil
	}
	for v := range l.cross {
		slices.SortFunc(l.cross[v], func(a, c globalEdge) int { return cmp.Compare(a.to, c.to) })
	}
	stats.Clusters, stats.Nodes, stats.SCCs = len(outs), nNodes, l.nSCC
	for _, n := range sizes {
		stats.LargestSCC = max(stats.LargestSCC, n)
	}
	return l, nil
}

// addEdge merges e into edges. Crossings between the same two nodes
// merge into one edge: its all-open cost and doors are those of the
// cheapest crossing, a door-free one first on a tie, and its door-free
// cost is the cheapest door-free crossing's. It reports whether e's target
// is new.
func addEdge(edges *[]globalEdge, e globalEdge) bool {
	for i := range *edges {
		old := &(*edges)[i]
		if old.to != e.to {
			continue
		}
		if e.open < old.open || e.open == old.open && len(e.doors) < len(old.doors) {
			old.open, old.doors = e.open, e.doors
		}
		old.free = min(old.free, e.free)
		return false
	}
	*edges = append(*edges, e)
	return true
}

// eachEdge calls fn for every stored edge, in node order and each node's
// edges by target: the intra-cluster edges between two nodes of one
// abstract SCC, the crossings, and the one-way edges joining two SCCs
// inside a cluster (link).
func eachEdge(outs []clusterOut, l *linked, fn func(v int, e globalEdge, intra bool)) {
	for ci := range outs {
		o := &outs[ci]
		base := l.base[ci]
		k := 0
		for u := range o.nodes {
			v := base + u
			cross := l.cross[v]
			x := 0
			for {
				hasIntra := k < len(o.edges) && int(o.edges[k].from) == u
				switch {
				case hasIntra && (x >= len(cross) || uint32(base+int(o.edges[k].to)) < cross[x].to):
					in := &o.edges[k]
					fn(v, globalEdge{
						to: uint32(base + int(in.to)), open: in.open, free: in.free,
						doors: o.edgeDoors[in.doorOff : in.doorOff+in.doorN],
					}, true)
					k++
				case x < len(cross):
					fn(v, cross[x], false)
					x++
				default:
					x = -1
				}
				if x < 0 {
					break
				}
			}
		}
	}
}

// encode writes the nav cache image: every edge between two nodes of one
// abstract SCC, intra-cluster with its two costs and door set, or a
// crossing; and the one-way edges between two SCCs.
func (b *builder) encode(outs []clusterOut, l *linked, key NavKey, stats *BuildStats) ([]byte, error) {
	nNodes := l.base[len(outs)]
	counts := make([]uint32, nNodes)
	nEdges, nDoorRefs := 0, 0
	eachEdge(outs, l, func(v int, e globalEdge, intra bool) {
		counts[v]++
		nEdges++
		nDoorRefs += len(e.doors)
		switch {
		case l.scc[v] != l.scc[e.to]:
			stats.OneWayEdges++
		case intra:
			stats.IntraEdges++
		default:
			stats.CrossEdges++
		}
	})
	stats.DoorRefs = nDoorRefs
	nSCC, nCond := l.nSCC, stats.CondEdges
	cond := l.cond
	base := l.base

	size := navHeaderSize + len(outs)*clusterRecSize + nNodes*nodeRecSize + nEdges*edgeRecSize + nDoorRefs*4 + (nSCC+1)*4 + nCond*4
	if size > MaxNavCacheBytes {
		return nil, errors.New("nav: nav cache would exceed MaxNavCacheBytes")
	}
	img := make([]byte, 0, size)
	le := binary.LittleEndian
	img = append(img, navMagic...)
	img = le.AppendUint32(img, navVersion)
	img = le.AppendUint32(img, 0) // CRC, below
	img = le.AppendUint32(img, key.GeoCRC)
	img = le.AppendUint32(img, uint32(key.ClusterCells))
	img = append(img, key.DoorsHash[:]...)
	for _, n := range []int{len(outs), nNodes, nEdges, nDoorRefs, nSCC, nCond} {
		img = le.AppendUint32(img, uint32(n))
	}
	img = append(img, key.WaterHash[:]...)
	for ci, k := range b.keys {
		img = le.AppendUint32(img, uint32(k.X))
		img = le.AppendUint32(img, uint32(k.Y))
		img = le.AppendUint32(img, uint32(base[ci]))
	}
	firstEdge := uint32(0)
	for ci, o := range outs {
		for li, nd := range o.nodes {
			v := base[ci] + li
			img = le.AppendUint32(img, uint32(int32(nd.rep.X)))
			img = le.AppendUint32(img, uint32(int32(nd.rep.Y)))
			img = le.AppendUint16(img, uint16(nd.rep.Z))
			img = append(img, byte(nd.side), 0)
			img = le.AppendUint16(img, uint16(nd.pos0))
			img = le.AppendUint16(img, uint16(nd.pos1))
			img = le.AppendUint32(img, uint32(ci))
			img = le.AppendUint32(img, uint32(l.scc[v]))
			img = le.AppendUint32(img, firstEdge)
			firstEdge += counts[v]
		}
	}
	firstDoor := 0
	eachEdge(outs, l, func(_ int, e globalEdge, _ bool) {
		img = le.AppendUint32(img, e.to)
		img = le.AppendUint32(img, e.open)
		img = le.AppendUint32(img, e.free)
		img = le.AppendUint32(img, uint32(firstDoor))
		img = le.AppendUint32(img, uint32(len(e.doors)))
		firstDoor += len(e.doors)
	})
	eachEdge(outs, l, func(_ int, e globalEdge, _ bool) {
		for _, d := range e.doors {
			img = le.AppendUint32(img, uint32(d))
		}
	})
	first := 0
	for _, c := range cond {
		img = le.AppendUint32(img, uint32(first))
		first += len(c)
	}
	img = le.AppendUint32(img, uint32(first))
	for _, c := range cond {
		for _, t := range c {
			img = le.AppendUint32(img, t)
		}
	}
	le.PutUint32(img[12:], crc32.Checksum(img[16:], navCRC))
	stats.Bytes = len(img)
	return img, nil
}
