package nav

import (
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// scratch is one query's working memory. It comes from scratchPool and goes
// back when the query returns, so a steady stream of queries allocates
// nothing once the buffers have grown. Every buffer grows with what one query
// touches (bounded by the node budget and the segment length), never with
// the region or the world.
type scratch struct {
	// A* search.
	nodes []node
	open  []openItem
	index map[Cell]int32
	edges []neighbour
	cells []Cell
	// Pulling.
	points []location.Location
	route  []location.Location
	// Corridor test.
	line      []lineCell
	corridor  map[[2]int]corridorCell
	stretches []stretch
	// Exact check.
	band  []bandPoint
	seen  map[chainState]struct{}
	queue []chainState
	// Stand points.
	stands []standCandidate
	layers [block.MaxLayers]block.Cell
}

var scratchPool = sync.Pool{New: func() any {
	return &scratch{
		index:    make(map[Cell]int32),
		corridor: make(map[[2]int]corridorCell),
		seen:     make(map[chainState]struct{}),
	}
}}

func getScratch() *scratch { return scratchPool.Get().(*scratch) }

func putScratch(s *scratch) { scratchPool.Put(s) }

// node is one cell the search reached.
type node struct {
	cell   Cell
	g      int32
	parent int32
	closed bool
}

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
	clear(s.index)

	s.index[start] = 0
	s.nodes = append(s.nodes, node{cell: start, parent: -1})
	s.push(openItem{f: heuristic(start, goal), idx: 0})

	expanded := 0
	for len(s.open) > 0 {
		it := s.pop()
		n := &s.nodes[it.idx]
		if n.closed || it.g != n.g {
			continue
		}
		if n.cell == goal {
			return s.trace(it.idx), expanded, nil
		}
		if expanded >= budget {
			return nil, expanded, ErrBudget
		}
		n.closed = true
		expanded++
		cur, g := n.cell, n.g
		s.edges = p.neighbours(s.edges[:0], cur)
		for _, e := range s.edges {
			if avoid != nil {
				if _, ok := avoid[edgeKey{cur, e.cell}]; ok {
					continue
				}
			}
			ng := g + e.cost
			idx, seen := s.index[e.cell]
			if !seen {
				idx = int32(len(s.nodes))
				s.index[e.cell] = idx
				s.nodes = append(s.nodes, node{cell: e.cell, g: ng, parent: it.idx})
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
		s.cells = append(s.cells, s.nodes[i].cell)
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
