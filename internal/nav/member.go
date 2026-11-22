package nav

import (
	"container/list"
	"errors"
	"math/bits"
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// DefaultMemberCacheBytes bounds the process-wide cluster result LRU
// (docs/navigation.md, "Cache").
const DefaultMemberCacheBytes = 64 << 20

// ErrNoNav reports a cell outside every cluster the nav cache holds, or on
// no layer of its column.
var ErrNoNav = errors.New("nav: cell is outside the nav hierarchy")

// Kind is how a cell relates to the abstract SCCs (docs/navigation.md,
// "Membership rule").
type Kind uint8

const (
	// Isolated ground reaches no abstract SCC: a closed pocket or pit.
	Isolated Kind = iota
	// OneWayOut ground reaches some abstract SCC, none of which reaches it
	// back: a ledge the bot can only drop off.
	OneWayOut
	// InSCC ground is in an abstract SCC: it reaches one of its entrances
	// and one of them reaches it.
	InSCC
)

func (k Kind) String() string {
	switch k {
	case InSCC:
		return "in-scc"
	case OneWayOut:
		return "one-way-out"
	default:
		return "isolated"
	}
}

// Hierarchy answers reachability and abstract distances over a Graph. It
// reads the engine's static regions only, every door open, exactly as
// navbuild did, so its labels never depend on the door state. Its cluster
// results live in a byte-bounded LRU; it is safe for concurrent use.
type Hierarchy struct {
	graph  *Graph
	static Planner
	cache  *memberCache
}

// NewHierarchy returns the hierarchy over g, reading e's static regions and
// the water w (nil: none) g was built with, with a cluster result LRU bounded
// by cacheBytes. A w whose Hash is not g's key's WaterHash would place cells
// in clusters the graph does not know; OpenNavCache refuses such a pair when
// the key is built from w.
func NewHierarchy(g *Graph, e *engine.Engine, w *Water, cacheBytes int64) *Hierarchy {
	return &Hierarchy{graph: g, static: Planner{Engine: e.Static(), Water: w}, cache: newMemberCache(cacheBytes)}
}

// Graph returns the abstract graph.
func (h *Hierarchy) Graph() *Graph { return h.graph }

// Member is a cell's place in the hierarchy, from Locate. It holds the
// cell's cluster result, so a Member a walker keeps also pins that result.
type Member struct {
	cell  Cell
	res   *clusterResult
	entry int32
}

// Cell returns the located cell.
func (m Member) Cell() Cell { return m.cell }

// Kind returns how the cell relates to the abstract SCCs.
func (m Member) Kind() Kind { return m.res.entries[m.entry].kind }

// SCC returns the cell's abstract SCC, or -1 unless its Kind is InSCC.
func (m Member) SCC() int32 { return m.res.entries[m.entry].scc }

// Pins holds one walker's references to the cluster results of its current
// cell and its destination (docs/navigation.md, "Cache"): Locate looks there
// before the LRU, so eviction never forces a walker to recompute the ground
// it stands on. The walker sets the fields to the Members it works with.
type Pins struct {
	Current, Destination Member
}

func (p *Pins) lookup(cluster int32) *clusterResult {
	if p == nil {
		return nil
	}
	for _, m := range [2]Member{p.Current, p.Destination} {
		if m.res != nil && m.res.cluster == cluster {
			return m.res
		}
	}
	return nil
}

// clusterResult is one cluster's membership, immutable once built.
type clusterResult struct {
	cluster int32
	x0, y0  int
	size    int
	// labels[layer][ly*size+lx] is the entry of each cell layer; nil when
	// every cell has entry 0 (a single local SCC).
	labels16 [][]uint16
	labels32 [][]uint32
	entries  []entry
	sccs     []int32 // the out and in SCC lists of the entries
	bytes    int64
}

// entry is one local SCC's membership. Local SCCs in the same abstract SCC
// share one entry; every other local SCC has its own.
type entry struct {
	kind         Kind
	scc          int32
	outOff, outN int32
	inOff, inN   int32
}

const (
	noEntry   = ^uint32(0)
	noEntry16 = ^uint16(0)
)

func (r *clusterResult) entryOf(layer, lx, ly int) uint32 {
	i := ly*r.size + lx
	switch {
	case r.labels16 != nil:
		if layer < len(r.labels16) && r.labels16[layer] != nil {
			if v := r.labels16[layer][i]; v != noEntry16 {
				return uint32(v)
			}
		}
		return noEntry
	case r.labels32 != nil:
		if layer < len(r.labels32) && r.labels32[layer] != nil {
			return r.labels32[layer][i]
		}
		return noEntry
	}
	return 0
}

func (r *clusterResult) outs(e entry) []int32 { return r.sccs[e.outOff : e.outOff+e.outN] }
func (r *clusterResult) ins(e entry) []int32  { return r.sccs[e.inOff : e.inOff+e.inN] }

// Locate returns c's membership: which abstract SCC its cell lies in, or
// whether it is one-way out or isolated. The cluster's result comes from
// pins (may be nil), then the LRU, else it is computed: Tarjan bounded to
// the cluster on the static base blocks, run without holding the LRU lock.
func (h *Hierarchy) Locate(c Cell, pins *Pins) (Member, error) {
	ci := h.graph.cluster(c.X, c.Y)
	if ci < 0 {
		return Member{}, ErrNoNav
	}
	r := pins.lookup(int32(ci))
	if r == nil {
		r = h.cache.get(int32(ci))
	}
	if r == nil {
		var err error
		if r, err = h.compute(ci); err != nil {
			return Member{}, err
		}
		h.cache.put(r)
	}
	if !h.static.hasGeo(c.X, c.Y) {
		return Member{}, ErrNoNav
	}
	layer := -1
	var buf [block.MaxLayers]block.Cell
	for i, l := range h.static.Engine.AppendLayers(buf[:0], c.X, c.Y) {
		if l.Height == c.Z {
			layer = i
			break
		}
	}
	if layer < 0 {
		return Member{}, ErrNoNav
	}
	e := r.entryOf(layer, c.X-r.x0, c.Y-r.y0)
	if e == noEntry {
		return Member{}, ErrNoNav
	}
	return Member{cell: c, res: r, entry: int32(e)}, nil
}

// SameSCC reports whether a and b are reachable from each other: both in
// one abstract SCC, or both in one local SCC of the same cluster. A cell
// that is not in an abstract SCC can only be mutually reachable with cells
// of its own cluster, since any way out and back passes entrances that
// would put it in their SCC.
func (h *Hierarchy) SameSCC(a, b Member) bool {
	ea, eb := a.res.entries[a.entry], b.res.entries[b.entry]
	if ea.kind == InSCC || eb.kind == InSCC {
		return ea.kind == InSCC && eb.kind == InSCC && ea.scc == eb.scc
	}
	return a.res.cluster == b.res.cluster && a.entry == b.entry
}

// Reachable reports whether b can be reached from a, one way: over the
// condensation from the SCCs a reaches to the SCCs reaching b. A one-way
// drop that stays inside one cluster between two cells in no abstract SCC
// is not seen, and answers false.
func (h *Hierarchy) Reachable(a, b Member) bool {
	if h.SameSCC(a, b) {
		return true
	}
	ea, eb := a.res.entries[a.entry], b.res.entries[b.entry]
	from, to := []int32{ea.scc}, []int32{eb.scc}
	if ea.kind != InSCC {
		from = a.res.outs(ea)
	}
	if eb.kind != InSCC {
		to = b.res.ins(eb)
	}
	if len(from) == 0 || len(to) == 0 {
		return false
	}
	s := getScratch()
	defer putScratch(s)
	s.sccGen++
	if s.sccGen == 0 || len(s.sccSeen) < h.graph.counts.sccs {
		s.sccSeen = make([]uint32, h.graph.counts.sccs)
		s.sccGen = 1
	}
	queue := s.sccQueue[:0]
	for _, x := range from {
		if s.sccSeen[x] != s.sccGen {
			s.sccSeen[x] = s.sccGen
			queue = append(queue, x)
		}
	}
	for len(queue) > 0 {
		x := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if slices.Contains(to, x) {
			s.sccQueue = queue
			return true
		}
		lo, hi := h.graph.condRange(x)
		for i := lo; i < hi; i++ {
			if t := h.graph.condTarget(i); s.sccSeen[t] != s.sccGen {
				s.sccSeen[t] = s.sccGen
				queue = append(queue, t)
			}
		}
	}
	s.sccQueue = queue
	return false
}

// memberScratch is the working memory of one cluster result computation.
type memberScratch struct {
	g      localGraph
	scc    sccScratch
	start  []int32 // label edge CSR
	to     []int32
	out    []uint64
	in     []uint64
	nodeOf []int32
}

var memberPool = sync.Pool{New: func() any { return &memberScratch{} }}

// compute builds cluster ci's result: Tarjan on its static cell graph, then
// for each local SCC the abstract SCCs whose entrances it reaches and those
// reaching it, inside the cluster.
func (h *Hierarchy) compute(ci int) (*clusterResult, error) {
	s := memberPool.Get().(*memberScratch)
	defer memberPool.Put(s)
	g := &s.g
	size := h.graph.key.ClusterCells
	x0, y0 := h.graph.clusterOrigin(ci)
	g.load(&h.static, x0, y0, size)
	nLabels := s.scc.localSCCs(g)
	label := s.scc.label

	// The cluster's abstract SCCs, numbered locally, and each local SCC's
	// own bits: the SCCs of the entrance nodes it holds.
	first, end := h.graph.clusterNodes(ci)
	var sccs []int32
	s.nodeOf = s.nodeOf[:0]
	for n := first; n < end; n++ {
		rep := g.index(h.graph.nodeRep(n))
		if rep < 0 {
			return nil, ErrNavCacheStale
		}
		x := h.graph.nodeSCC(n)
		if !slices.Contains(sccs, x) {
			sccs = append(sccs, x)
		}
		s.nodeOf = append(s.nodeOf, rep)
	}
	words := (len(sccs) + 63) / 64
	s.out = fill(s.out, nLabels*words, 0)
	s.in = fill(s.in, nLabels*words, 0)
	for i, rep := range s.nodeOf {
		bit := slices.Index(sccs, h.graph.nodeSCC(first+i))
		l := int(label[rep])
		s.out[l*words+bit/64] |= 1 << (bit % 64)
		s.in[l*words+bit/64] |= 1 << (bit % 64)
	}

	// Local condensation edges, grouped by source label. Labels are in
	// completion order: every edge goes from a higher label to a lower.
	s.start = fill(s.start, nLabels+1, 0)
	n := g.cells()
	for v := range n {
		for k := range 4 {
			if w := g.next[4*v+k]; w >= 0 && label[w] != label[v] {
				s.start[label[v]+1]++
			}
		}
	}
	for l := range nLabels {
		s.start[l+1] += s.start[l]
	}
	s.to = resize(s.to, int(s.start[nLabels]))
	pos := slices.Clone(s.start[:nLabels])
	for v := range n {
		for k := range 4 {
			if w := g.next[4*v+k]; w >= 0 && label[w] != label[v] {
				s.to[pos[label[v]]] = label[w]
				pos[label[v]]++
			}
		}
	}
	for l := range nLabels {
		for _, m := range s.to[s.start[l]:s.start[l+1]] {
			for w := range words {
				s.out[l*words+w] |= s.out[int(m)*words+w]
			}
		}
	}
	for l := nLabels - 1; l >= 0; l-- {
		for _, m := range s.to[s.start[l]:s.start[l+1]] {
			for w := range words {
				s.in[int(m)*words+w] |= s.in[l*words+w]
			}
		}
	}

	r := &clusterResult{cluster: int32(ci), x0: x0, y0: y0, size: size}
	entryOf := make([]uint32, nLabels)
	inSCC := map[int32]uint32{}
	// SCC lists are shared between the entries with the same set: a slope
	// of one-way cells has thousands of entries and a handful of sets. A
	// cluster with at most 64 abstract SCCs (nearly all) keys the set by its
	// one word, without allocating.
	lists := map[string][2]int32{}
	small := map[uint64][2]int32{}
	list := func(bitsOf []uint64) (int32, int32) {
		var key string
		if len(bitsOf) == 1 {
			if l, ok := small[bitsOf[0]]; ok {
				return l[0], l[1]
			}
		} else {
			key = string(u64Bytes(bitsOf))
			if l, ok := lists[key]; ok {
				return l[0], l[1]
			}
		}
		off := int32(len(r.sccs))
		for w, word := range bitsOf {
			for word != 0 {
				b := bits.TrailingZeros64(word)
				word &^= 1 << b
				r.sccs = append(r.sccs, sccs[w*64+b])
			}
		}
		l := [2]int32{off, int32(len(r.sccs)) - off}
		if len(bitsOf) == 1 {
			small[bitsOf[0]] = l
		} else {
			lists[key] = l
		}
		return l[0], l[1]
	}
	for l := range nLabels {
		out, in := s.out[l*words:(l+1)*words], s.in[l*words:(l+1)*words]
		x := int32(-1)
		for w := range words {
			if both := out[w] & in[w]; both != 0 {
				x = sccs[w*64+bits.TrailingZeros64(both)]
				break
			}
		}
		if x >= 0 {
			e, ok := inSCC[x]
			if !ok {
				e = uint32(len(r.entries))
				inSCC[x] = e
				r.entries = append(r.entries, entry{kind: InSCC, scc: x})
			}
			entryOf[l] = e
			continue
		}
		en := entry{kind: Isolated, scc: -1}
		en.outOff, en.outN = list(out)
		en.inOff, en.inN = list(in)
		if en.outN > 0 {
			en.kind = OneWayOut
		}
		entryOf[l] = uint32(len(r.entries))
		r.entries = append(r.entries, en)
	}

	r.bytes = 128 + int64(len(r.entries))*24 + int64(len(r.sccs))*4
	if len(r.entries) > 1 {
		layers := 0
		for i := range n {
			layers = max(layers, g.layer(int32(i))+1)
		}
		cols := size * size
		if len(r.entries) < int(noEntry16) {
			r.labels16 = make([][]uint16, layers)
			for i := range n {
				li := g.layer(int32(i))
				if r.labels16[li] == nil {
					r.labels16[li] = slices.Repeat([]uint16{noEntry16}, cols)
					r.bytes += int64(cols) * 2
				}
				r.labels16[li][g.col[i]] = uint16(entryOf[label[i]])
			}
		} else {
			r.labels32 = make([][]uint32, layers)
			for i := range n {
				li := g.layer(int32(i))
				if r.labels32[li] == nil {
					r.labels32[li] = slices.Repeat([]uint32{noEntry}, cols)
					r.bytes += int64(cols) * 4
				}
				r.labels32[li][g.col[i]] = entryOf[label[i]]
			}
		}
	} else if len(r.entries) == 0 {
		// A cluster with no cell: every lookup misses.
		r.labels16 = [][]uint16{}
	}
	return r, nil
}

func u64Bytes(ws []uint64) []byte {
	b := make([]byte, 0, 8*len(ws))
	for _, w := range ws {
		for i := range 8 {
			b = append(b, byte(w>>(8*i)))
		}
	}
	return b
}

// memberCache is the process-wide LRU of cluster results, bounded by bytes.
// mu guards only the lookup and the insert; results are computed outside it
// and are immutable. A second insert of the same cluster is dropped.
type memberCache struct {
	mu    sync.Mutex
	max   int64
	used  int64
	items map[int32]*list.Element
	order list.List // front: most recent; values *clusterResult
}

func newMemberCache(maxBytes int64) *memberCache {
	return &memberCache{max: maxBytes, items: make(map[int32]*list.Element)}
}

func (c *memberCache) get(cluster int32) *clusterResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[cluster]
	if !ok {
		return nil
	}
	c.order.MoveToFront(el)
	return el.Value.(*clusterResult)
}

func (c *memberCache) put(r *clusterResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[r.cluster]; ok || r.bytes > c.max {
		return
	}
	c.items[r.cluster] = c.order.PushFront(r)
	c.used += r.bytes
	for c.used > c.max {
		el := c.order.Back()
		old := el.Value.(*clusterResult)
		c.order.Remove(el)
		delete(c.items, old.cluster)
		c.used -= old.bytes
	}
}
