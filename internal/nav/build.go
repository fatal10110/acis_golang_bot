package nav

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
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
	// keyed by; Build sets its ClusterCells.
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
	b := &builder{size: size, keys: keys, p: Planner{Engine: static}, built: make(map[ClusterKey]bool, len(keys))}
	for _, k := range keys {
		b.built[k] = true
	}
	var stats BuildStats
	stats.Doors, stats.SkippedDoors = b.loadDoors(cfg.Doors, static, log)

	outs, err := b.clusters(ctx, max(cfg.Workers, 1), cfg.onCluster)
	if err != nil {
		return nil, stats, err
	}
	key := cfg.Key
	key.ClusterCells = size
	image, err := b.assemble(outs, key, &stats)
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

type doorFoot struct {
	id         int
	zmin, zmax int
}

// loadDoors builds every door's footprint over the static geodata, as the
// server shapes a door (dynamic.NewDoorObject): its fully blocked cells,
// from its floor up to its height.
func (b *builder) loadDoors(templates []*door.Template, static *engine.Engine, log *slog.Logger) (doors, skipped int) {
	b.doorCells = make(map[[2]int][]doorFoot)
	for _, t := range templates {
		obj, err := dynamic.NewDoorObject(t, static)
		if err != nil {
			log.Warn("nav: door footprint skipped", "door", t.ID, "err", err)
			skipped++
			continue
		}
		doors++
		foot := doorFoot{id: t.ID, zmin: obj.GeoZ() - block.CellIgnoreHeight, zmax: obj.GeoZ() + obj.Height()}
		for ix, col := range obj.GeoData() {
			for iy, n := range col {
				if n == block.NoDirections {
					k := [2]int{obj.GeoX() + ix, obj.GeoY() + iy}
					b.doorCells[k] = append(b.doorCells[k], foot)
				}
			}
		}
	}
	return doors, skipped
}

// doorsAt appends the doors whose footprint holds c.
func (b *builder) doorsAt(dst []int32, c Cell) []int32 {
	for _, f := range b.doorCells[[2]int{c.X, c.Y}] {
		if int(c.Z) >= f.zmin && int(c.Z) <= f.zmax {
			dst = append(dst, int32(f.id))
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

// clusterOut is one built cluster, in local node indices.
type clusterOut struct {
	nodes  []buildNode
	edges  []buildEdge
	cross  []buildCross
	border []borderCell // sorted by side, pos, z
}

type buildNode struct {
	rep        Cell
	side       int
	pos0, pos1 int
}

type buildEdge struct {
	from, to   int32
	open, free uint32
	doors      []int32
}

// buildCross is a crossing out of the cluster: from a cell of node from
// into cell to of the next cluster through side.
type buildCross struct {
	from  int32
	side  int
	to    Cell
	doors []int32 // doors at either end of the crossing
}

// borderCell is one cell an entrance node holds.
type borderCell struct {
	side, pos int
	z         int16
	node      int32
}

func cmpBorder(a, b borderCell) int {
	if c := cmp.Compare(a.side, b.side); c != 0 {
		return c
	}
	if c := cmp.Compare(a.pos, b.pos); c != 0 {
		return c
	}
	return cmp.Compare(a.z, b.z)
}

func (o *clusterOut) nodeAt(side, pos int, z int16) (int32, bool) {
	i, ok := slices.BinarySearchFunc(o.border, borderCell{side: side, pos: pos, z: z}, cmpBorder)
	if !ok {
		return 0, false
	}
	return o.border[i].node, true
}

// clusters builds every cluster on workers goroutines, joined before it
// returns.
func (b *builder) clusters(ctx context.Context, workers int, onCluster func()) ([]clusterOut, error) {
	outs := make([]clusterOut, len(b.keys))
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
				outs[i] = b.cluster(s, i)
				if onCluster != nil {
					onCluster()
				}
			}
		}()
	}
	wg.Wait()
	if failed != nil {
		return nil, failed
	}
	return outs, nil
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
	edges   []neighbour
	dist    []uint32
	parent  []int32
	heap    []distItem
	isDoor  []bool
	doors   []int32
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
			s.entries[hi].pos <= s.entries[hi-1].pos+1 {
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
			out.border = append(out.border, borderCell{side: e.side, pos: e.pos, z: g.z[e.cell], node: n})
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
		doors := b.doorsAt(b.doorsAt(nil, c), oc.to)
		out.cross = append(out.cross, buildCross{from: from, side: oc.side, to: oc.to, doors: doors})
	}

	if len(out.nodes) > 1 {
		b.intraEdges(s, &out)
	}
	return out
}

// intraEdges adds the directed edges between the cluster's nodes: for each
// ordered pair joined inside the cluster, the cheapest cost with every door
// open and the doors that path crosses, and the cheapest cost crossing no
// door (docs/navigation.md, "Doors in the hierarchy").
func (b *builder) intraEdges(s *buildScratch, out *clusterOut) {
	g := &s.g
	n := g.cells()
	s.wStart = resize(s.wStart, n+1)
	s.wTo, s.wCost = s.wTo[:0], s.wCost[:0]
	s.isDoor = resize(s.isDoor, n)
	anyDoor := false
	for i := range int32(n) {
		s.wStart[i] = int32(len(s.wTo))
		c := g.cell(i)
		s.edges = b.p.neighbours(s.edges[:0], c)
		for _, e := range s.edges {
			if j := g.index(e.cell); j >= 0 {
				s.wTo = append(s.wTo, j)
				s.wCost = append(s.wCost, e.cost)
			}
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
	for u := range out.nodes {
		if anyDoor {
			b.dijkstra(s, reps[u], reps, true)
			for v, r := range reps {
				free[v] = s.dist[r]
			}
		}
		b.dijkstra(s, reps[u], reps, false)
		for v, r := range reps {
			if v == u || s.dist[r] == noCost {
				continue
			}
			e := buildEdge{from: int32(u), to: int32(v), open: s.dist[r], free: s.dist[r]}
			if anyDoor {
				e.free = free[v]
				var doors []int32
				for c := r; c >= 0; c = s.parent[c] {
					if s.isDoor[c] {
						doors = b.doorsAt(doors, g.cell(c))
					}
				}
				slices.Sort(doors)
				e.doors = slices.Compact(doors)
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
	s.heap = pushDist(s.heap, distItem{0, src})
	left := len(targets)
	for len(s.heap) > 0 && left > 0 {
		var it distItem
		it, s.heap = popDist(s.heap)
		if it.d != s.dist[it.v] {
			continue
		}
		if slices.Contains(targets, it.v) {
			left--
		}
		for k := s.wStart[it.v]; k < s.wStart[it.v+1]; k++ {
			w := s.wTo[k]
			if noDoors && s.isDoor[w] {
				continue
			}
			if d := it.d + uint32(s.wCost[k]); d < s.dist[w] {
				s.dist[w], s.parent[w] = d, it.v
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

// assemble joins the clusters into the abstract graph, labels its SCCs and
// condensation, and encodes the nav cache image.
func (b *builder) assemble(outs []clusterOut, key NavKey, stats *BuildStats) ([]byte, error) {
	base := make([]int, len(outs)+1)
	for i, o := range outs {
		base[i+1] = base[i] + len(o.nodes)
	}
	nNodes := base[len(outs)]
	edges := make([][]globalEdge, nNodes)
	add := func(from int, e globalEdge) {
		for i, old := range edges[from] {
			if old.to == e.to {
				if e.open < old.open {
					edges[from][i] = e
				}
				return
			}
		}
		edges[from] = append(edges[from], e)
	}
	for ci, o := range outs {
		for _, e := range o.edges {
			add(base[ci]+int(e.from), globalEdge{to: uint32(base[ci] + int(e.to)), open: e.open, free: e.free, doors: e.doors})
			stats.IntraEdges++
		}
		for _, x := range o.cross {
			ti := b.clusterIndex(b.clusterOf(x.to))
			if ti < 0 {
				return nil, fmt.Errorf("nav: crossing into unbuilt cluster at %d,%d", x.to.X, x.to.Y)
			}
			side := opposite(x.side)
			ox, oy := int(b.keys[ti].X)*b.size, int(b.keys[ti].Y)*b.size
			pos := x.to.Y - oy
			if side == sideSouth || side == sideNorth {
				pos = x.to.X - ox
			}
			to, ok := outs[ti].nodeAt(side, pos, x.to.Z)
			if !ok {
				return nil, fmt.Errorf("nav: crossing target %d,%d,%d is no entrance", x.to.X, x.to.Y, x.to.Z)
			}
			from := base[ci] + int(x.from)
			cost := uint32(max(costCardinal, heuristic(o.nodes[x.from].rep, outs[ti].nodes[to].rep)))
			e := globalEdge{to: uint32(base[ti] + int(to)), open: cost, free: cost, doors: x.doors}
			if len(x.doors) > 0 {
				e.free = noCost
			}
			add(from, e)
		}
	}
	nEdges, nDoorRefs := 0, 0
	for i := range edges {
		slices.SortFunc(edges[i], func(a, c globalEdge) int { return cmp.Compare(a.to, c.to) })
		nEdges += len(edges[i])
		for _, e := range edges[i] {
			nDoorRefs += len(e.doors)
		}
	}
	stats.CrossEdges = nEdges - stats.IntraEdges

	// Abstract SCCs with every door open, and the condensation.
	var scc sccScratch
	nSCC := scc.run(nNodes,
		func(v int32) int32 { return int32(len(edges[v])) },
		func(v, k int32) int32 { return int32(edges[v][k].to) })
	sizes := make([]int, nSCC)
	cond := make([][]uint32, nSCC)
	for v := range nNodes {
		sv := scc.label[v]
		sizes[sv]++
		for _, e := range edges[v] {
			if st := scc.label[e.to]; st != sv {
				cond[sv] = append(cond[sv], uint32(st))
			}
		}
	}
	nCond := 0
	for i := range cond {
		slices.Sort(cond[i])
		cond[i] = slices.Compact(cond[i])
		nCond += len(cond[i])
	}

	stats.Clusters, stats.Nodes, stats.DoorRefs, stats.SCCs, stats.CondEdges = len(outs), nNodes, nDoorRefs, nSCC, nCond
	for _, n := range sizes {
		stats.LargestSCC = max(stats.LargestSCC, n)
	}

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
	img = append(img, make([]byte, navHeaderSize-len(img))...)
	for ci, k := range b.keys {
		img = le.AppendUint32(img, uint32(k.X))
		img = le.AppendUint32(img, uint32(k.Y))
		img = le.AppendUint32(img, uint32(base[ci]))
	}
	firstEdge := 0
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
			img = le.AppendUint32(img, uint32(scc.label[v]))
			img = le.AppendUint32(img, uint32(firstEdge))
			firstEdge += len(edges[v])
		}
	}
	firstDoor := 0
	for v := range edges {
		for _, e := range edges[v] {
			img = le.AppendUint32(img, e.to)
			img = le.AppendUint32(img, e.open)
			img = le.AppendUint32(img, e.free)
			img = le.AppendUint32(img, uint32(firstDoor))
			img = le.AppendUint32(img, uint32(len(e.doors)))
			firstDoor += len(e.doors)
		}
	}
	for v := range edges {
		for _, e := range edges[v] {
			for _, d := range e.doors {
				img = le.AppendUint32(img, uint32(d))
			}
		}
	}
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
