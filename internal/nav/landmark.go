package nav

import (
	"encoding/binary"
	"math"
	"slices"
)

// navLandmarks is the landmark count a nav build picks (ALT,
// docs/navigation.md, "Landmarks"): each costs 8 bytes per entrance node in
// the nav cache, 6.4 MB on the owner's 804,918 nodes.
const navLandmarks = 8

// noDist marks a node a landmark does not reach, or that does not reach it.
const noDist = math.MaxUint32

// landmarks picks up to k landmarks among the nodes of g's largest SCC and
// returns them with, node-major, every node's distance from each landmark,
// then to each, over the all-open edge costs (noDist when there is no way).
// The first landmark is the node farthest from the SCC's first node; each
// next one is the node whose nearest landmark, there and back, is farthest.
// g's landmark section is not read.
func landmarks(g *Graph, k int) (ids, dist []uint32) {
	n := g.Nodes()
	sizes := make([]int, g.SCCs())
	for v := range n {
		sizes[g.nodeSCC(v)]++
	}
	largest := int32(0)
	for s, c := range sizes {
		if c > sizes[largest] {
			largest = int32(s)
		}
	}
	k = min(k, slices.Max(append(sizes, 0)))
	if k == 0 {
		return nil, nil
	}
	rev := reverseEdges(g)
	d := &dijkstra{dist: make([]uint32, n)}
	farthest := func(score func(v int) uint64) int {
		best, at := uint64(0), -1
		for v := range n {
			if g.nodeSCC(v) != largest || slices.Contains(ids, uint32(v)) {
				continue
			}
			if s := score(v); at < 0 || s > best {
				best, at = s, v
			}
		}
		return at
	}
	first := 0
	for g.nodeSCC(first) != largest {
		first++
	}
	d.run(g, nil, first)
	from0 := slices.Clone(d.dist)
	near := make([]uint64, n)
	for v := range near {
		near[v] = math.MaxUint64
	}
	dist = make([]uint32, n*2*k)
	for i := range k {
		var l int
		if i == 0 {
			l = farthest(func(v int) uint64 { return uint64(from0[v]) })
		} else {
			l = farthest(func(v int) uint64 { return near[v] })
		}
		ids = append(ids, uint32(l))
		d.run(g, nil, l)
		for v, x := range d.dist {
			dist[v*2*k+i] = x
		}
		d.run(g, rev, l)
		for v, x := range d.dist {
			dist[v*2*k+k+i] = x
			near[v] = min(near[v], uint64(dist[v*2*k+i])+uint64(x))
		}
	}
	return ids, dist
}

// reversed is the abstract graph's edges by target: the edges into node v
// are from[first[v]:first[v+1]], with their all-open costs.
type reversed struct {
	first      []uint32
	from, cost []uint32
}

func reverseEdges(g *Graph) *reversed {
	n := g.Nodes()
	r := &reversed{first: make([]uint32, n+1), from: make([]uint32, g.Edges()), cost: make([]uint32, g.Edges())}
	for i := range g.Edges() {
		r.first[g.edge(i).to+1]++
	}
	for v := range n {
		r.first[v+1] += r.first[v]
	}
	next := slices.Clone(r.first[:n])
	for v := range n {
		lo, hi := g.nodeEdges(v)
		for i := lo; i < hi; i++ {
			e := g.edge(i)
			r.from[next[e.to]], r.cost[next[e.to]] = uint32(v), e.open
			next[e.to]++
		}
	}
	return r
}

// dijkstra is one single-source search's working memory.
type dijkstra struct {
	dist []uint32
	heap []dijkstraItem
}

type dijkstraItem struct {
	d uint64
	v int32
}

// run leaves in d.dist every node's distance from src over g's edges, or
// to src over rev's when rev is not nil.
func (d *dijkstra) run(g *Graph, rev *reversed, src int) {
	for v := range d.dist {
		d.dist[v] = noDist
	}
	d.heap = d.heap[:0]
	relax := func(v int, nd uint64) {
		if nd >= uint64(d.dist[v]) {
			return
		}
		d.dist[v] = uint32(min(nd, noDist-1))
		d.push(dijkstraItem{nd, int32(v)})
	}
	relax(src, 0)
	for len(d.heap) > 0 {
		it := d.pop()
		if it.d != uint64(d.dist[it.v]) {
			continue
		}
		if rev != nil {
			for i := rev.first[it.v]; i < rev.first[it.v+1]; i++ {
				relax(int(rev.from[i]), it.d+uint64(rev.cost[i]))
			}
			continue
		}
		lo, hi := g.nodeEdges(int(it.v))
		for i := lo; i < hi; i++ {
			e := g.edge(i)
			relax(e.to, it.d+uint64(e.open))
		}
	}
}

func (d *dijkstra) push(it dijkstraItem) {
	h := append(d.heap, it)
	for i := len(h) - 1; i > 0; {
		p := (i - 1) / 2
		if h[p].d <= h[i].d {
			break
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
	d.heap = h
}

func (d *dijkstra) pop() dijkstraItem {
	h := d.heap
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for i := 0; ; {
		c := 2*i + 1
		if c >= len(h) {
			break
		}
		if c+1 < len(h) && h[c+1].d < h[c].d {
			c++
		}
		if h[i].d <= h[c].d {
			break
		}
		h[i], h[c] = h[c], h[i]
		i = c
	}
	d.heap = h
	return top
}

// goalBounds is one abstract search's landmark bounds on the cost from a
// node to its goal: the goal is reached from the goal nodes m, each at the
// straight-line cost s(m) to the goal cell. For landmark L, the cost from
// node v is at least
//
//	via[L] − d(L, v), via[L] = min over m of d(L, m) + s(m),
//
// as d(L, m) ≤ d(L, v) + cost(v, m) (a goal node L does not reach is not
// reached from v either, when L reaches v), and at least
//
//	d(v, L) − back[L], back[L] = max over m of d(m, L) − s(m),
//
// as d(v, L) ≤ cost(v, m) + d(m, L), which needs every goal node to reach L;
// then a node that does not reach L reaches no goal node, and the search
// drops it. The distances are over the all-open costs: a closed door, the
// search's span and its seats only make a way dearer or absent, so the
// bounds hold, and both are consistent (the triangle inequality on one
// edge), as the octile bound is: their maximum keeps A* optimal
// (docs/navigation.md, "Landmarks").
type goalBounds struct {
	k, goals      int
	via, back     [navLandmarks]int64
	viaOK, backOK [navLandmarks]bool
}

// reset starts the bounds of a goal with no goal node yet.
func (b *goalBounds) reset(g *Graph) {
	b.k, b.goals = g.counts.landmarks, 0
	for i := range b.k {
		b.via[i], b.viaOK[i] = math.MaxInt64, false
		b.back[i], b.backOK[i] = math.MinInt64, true
	}
}

// add adds goal node m, s(m) = straight.
func (b *goalBounds) add(g *Graph, m int, straight int32) {
	b.goals++
	off := g.landmarkDist + m*8*b.k
	for i := range b.k {
		if d := g.u32(off + 4*i); d != noDist {
			b.via[i], b.viaOK[i] = min(b.via[i], int64(d)+int64(straight)), true
		}
		if d := g.u32(off + 4*(b.k+i)); d != noDist {
			b.back[i] = max(b.back[i], int64(d)-int64(straight))
		} else {
			b.backOK[i] = false
		}
	}
}

// bound returns the landmarks' lower bound on the cost from node v to the
// goal, at least lo, or false when v reaches no goal node.
func (b *goalBounds) bound(g *Graph, v int, lo int32) (int32, bool) {
	if b.goals == 0 {
		return lo, true
	}
	off := g.landmarkDist + v*8*b.k
	best := int64(lo)
	for i := range b.k {
		if b.viaOK[i] {
			if d := g.u32(off + 4*i); d != noDist {
				best = max(best, b.via[i]-int64(d))
			}
		}
		if b.backOK[i] {
			d := g.u32(off + 4*(b.k+i))
			if d == noDist {
				return 0, false
			}
			best = max(best, int64(d)-b.back[i])
		}
	}
	return int32(min(best, math.MaxInt32/2)), true
}

// appendLandmarks appends the landmark section, landmarks' result, to a
// nav cache image.
func appendLandmarks(img []byte, ids, dist []uint32) []byte {
	for _, x := range ids {
		img = binary.LittleEndian.AppendUint32(img, x)
	}
	for _, x := range dist {
		img = binary.LittleEndian.AppendUint32(img, x)
	}
	return img
}
