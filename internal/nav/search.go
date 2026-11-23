package nav

import (
	"unsafe"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// scratch is one query's working memory. It comes from getScratch and goes
// back when the query returns, so a steady stream of queries allocates
// nothing once the buffers have grown. Every buffer grows with what one query
// touches (bounded by the node budget and the segment length), never with
// the region or the world.
type scratch struct {
	// A* search.
	nodes []node
	open  []openItem
	index indexTable
	edges []neighbour
	cells []Cell
	// Pulling.
	points []location.Location
	route  []location.Location
	// Corridor test.
	line      []lineCell
	grid      corridorGrid
	stretches []stretch
	// Exact check.
	band  []bandPoint
	seen  indexTable
	queue []chainState
	// Stand points.
	stands []standCandidate
	layers [block.MaxLayers]block.Cell
	// Hierarchy: the condensation walk, the abstract search and the
	// refined cell path across clusters.
	sccSeen   []uint32
	sccGen    uint32
	sccQueue  []int32
	anodes    []anode
	aindex    indexTable
	apath     []int32
	waypoints []Cell
	path      []Cell
}

// Idle scratch kept between queries. Unlike a sync.Pool, which a GC
// empties, the free list keeps its buffers, so a query does not regrow them
// after every collection (#209). It keeps at most keptScratch, each holding
// at most maxKeptScratchBytes; a larger one (a search near its node budget)
// goes to the GC, so idle scratch never holds more than their product.
const (
	keptScratch         = 2
	maxKeptScratchBytes = 32 << 20
)

var scratchFree = make(chan *scratch, keptScratch)

func getScratch() *scratch {
	select {
	case s := <-scratchFree:
		return s
	default:
		return new(scratch)
	}
}

func putScratch(s *scratch) {
	if s.bytes() > maxKeptScratchBytes {
		return
	}
	select {
	case scratchFree <- s:
	default:
	}
}

// bytes is the memory s's buffers hold.
func (s *scratch) bytes() int {
	return capBytes(s.nodes) + capBytes(s.open) + s.index.bytes() + capBytes(s.edges) + capBytes(s.cells) +
		capBytes(s.points) + capBytes(s.route) +
		capBytes(s.line) + capBytes(s.grid.slots) + capBytes(s.stretches) +
		capBytes(s.band) + s.seen.bytes() + capBytes(s.queue) +
		capBytes(s.stands) +
		capBytes(s.sccSeen) + capBytes(s.sccQueue) + capBytes(s.anodes) + s.aindex.bytes() + capBytes(s.apath) +
		capBytes(s.waypoints) + capBytes(s.path)
}

// capBytes is the memory a slice's backing array holds.
func capBytes[T any](s []T) int {
	var zero T
	return cap(s) * int(unsafe.Sizeof(zero))
}

// node is one cell the search reached, its coordinates held in 32 bits so
// a node takes 20 bytes, not a Cell's 24 plus 12.
type node struct {
	x, y   int32
	z      int16
	closed bool
	g      int32
	parent int32
}

func newNode(c Cell, g, parent int32) node {
	return node{x: int32(c.X), y: int32(c.Y), z: c.Z, g: g, parent: parent}
}

func (n *node) cell() Cell { return Cell{X: int(n.x), Y: int(n.y), Z: n.z} }

// openItem is an open-set entry. Entries are never updated in place: a better
// path pushes a new one, and a popped entry whose g no longer matches its
// node's is stale and skipped.
type openItem struct {
	f, g int32
	idx  int32
}

// edgeKey is one directed cell-graph edge.
type edgeKey struct{ from, to Cell }

// search runs A* from start to goal over the cell graph, expanding at most
// budget cells and skipping the edges in avoid. It returns the cell path,
// start first, in s.cells, and the cells it expanded.
func (p *Planner) search(s *scratch, start, goal Cell, budget int, avoid map[edgeKey]struct{}) ([]Cell, int, error) {
	s.nodes = s.nodes[:0]
	s.open = s.open[:0]
	s.index.reset()

	s.index.getOrPut(cellKey(start), 0)
	s.nodes = append(s.nodes, newNode(start, 0, -1))
	s.push(openItem{f: heuristic(start, goal), idx: 0})

	expanded := 0
	for len(s.open) > 0 {
		it := s.pop()
		n := &s.nodes[it.idx]
		if n.closed || it.g != n.g {
			continue
		}
		cur, g := n.cell(), n.g
		if cur == goal {
			return s.trace(it.idx), expanded, nil
		}
		if expanded >= budget {
			return nil, expanded, ErrBudget
		}
		n.closed = true
		expanded++
		s.edges = p.neighbours(s.edges[:0], cur)
		for _, e := range s.edges {
			if avoid != nil {
				if _, ok := avoid[edgeKey{cur, e.cell}]; ok {
					continue
				}
			}
			ng := g + e.cost
			idx, seen := s.index.getOrPut(cellKey(e.cell), int32(len(s.nodes)))
			if !seen {
				s.nodes = append(s.nodes, newNode(e.cell, ng, it.idx))
			} else {
				m := &s.nodes[idx]
				if m.closed || ng >= m.g {
					continue
				}
				m.g, m.parent = ng, it.idx
			}
			s.push(openItem{f: ng + heuristic(e.cell, goal), g: ng, idx: idx})
		}
	}
	return nil, expanded, ErrUnreachable
}

// trace returns the path ending at node idx, start first, in s.cells.
func (s *scratch) trace(idx int32) []Cell {
	s.cells = s.cells[:0]
	for i := idx; i >= 0; i = s.nodes[i].parent {
		s.cells = append(s.cells, s.nodes[i].cell())
	}
	for i, j := 0, len(s.cells)-1; i < j; i, j = i+1, j-1 {
		s.cells[i], s.cells[j] = s.cells[j], s.cells[i]
	}
	return s.cells
}

// heuristic is the octile distance in step costs: admissible, since every
// step costs at least its cardinal or diagonal base.
func heuristic(a, b Cell) int32 {
	dx, dy := abs(a.X-b.X), abs(a.Y-b.Y)
	lo, hi := min(dx, dy), max(dx, dy)
	return int32(costCardinal*(hi-lo) + costDiagonal*lo)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// less orders the open set by f, then by larger g (deeper first) to break
// ties toward the goal.
func (a openItem) less(b openItem) bool {
	if a.f != b.f {
		return a.f < b.f
	}
	return a.g > b.g
}

func (s *scratch) push(it openItem) {
	s.open = append(s.open, it)
	i := len(s.open) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !s.open[i].less(s.open[parent]) {
			break
		}
		s.open[i], s.open[parent] = s.open[parent], s.open[i]
		i = parent
	}
}

func (s *scratch) pop() openItem {
	top := s.open[0]
	last := len(s.open) - 1
	s.open[0] = s.open[last]
	s.open = s.open[:last]
	i := 0
	for {
		l, r := 2*i+1, 2*i+2
		smallest := i
		if l < last && s.open[l].less(s.open[smallest]) {
			smallest = l
		}
		if r < last && s.open[r].less(s.open[smallest]) {
			smallest = r
		}
		if smallest == i {
			return top
		}
		s.open[i], s.open[smallest] = s.open[smallest], s.open[i]
		i = smallest
	}
}
