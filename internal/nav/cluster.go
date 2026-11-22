package nav

import (
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
)

// DefaultClusterCells is the side of a hierarchy cluster in cells: 32×32
// geodata blocks, 4096 world units, 64 clusters per region
// (docs/navigation.md, "Hierarchy").
const DefaultClusterCells = 256

// ClusterKey names one cluster: its cell coordinates divided by the cluster
// side.
type ClusterKey struct{ X, Y int32 }

// Border sides of a cluster, in the order of cardinals: a crossing in
// direction i leaves the cluster through side i.
const (
	sideEast = iota
	sideWest
	sideSouth
	sideNorth
)

// opposite returns the side a crossing through side s enters the next
// cluster by.
func opposite(s int) int { return s ^ 1 }

// localGraph is the directed cell graph of one cluster: every cell layer of
// its columns, with the cardinal crossings that stay inside it. Diagonal
// edges are left out: a diagonal needs both L-shaped detours open, so a
// cardinal path joins the same two cells and reachability is unchanged
// (docs/navigation.md, "Reachability"). It is scratch, reused cluster after
// cluster; it never outlives the query or build step that loaded it.
type localGraph struct {
	x0, y0, size int
	// colStart[col] is the first cell of column col (ly*size+lx); the last
	// entry is the cell count.
	colStart []int32
	z        []int16
	nswe     []block.NSWE
	col      []int32
	// next holds four entries per cell, in cardinals order: the index of
	// the neighbour a crossing lands on inside the cluster, or -1.
	next []int32
	// edge marks a cell next to a wall, a climb over 48 or a drop, which
	// costs costClearance more to leave.
	edge []bool
	// layers is scratch for AppendLayers.
	layers [block.MaxLayers]block.Cell
}

// load reads the cluster whose top-left cell is (x0, y0) through p's engine
// and links its cardinal crossings.
func (g *localGraph) load(p *Planner, x0, y0, size int) {
	g.x0, g.y0, g.size = x0, y0, size
	cols := size * size
	g.colStart = resize(g.colStart, cols+1)
	g.z, g.nswe, g.col = g.z[:0], g.nswe[:0], g.col[:0]
	for ly := range size {
		for lx := range size {
			c := ly*size + lx
			g.colStart[c] = int32(len(g.z))
			gx, gy := x0+lx, y0+ly
			if !p.hasGeo(gx, gy) {
				continue
			}
			for _, l := range p.Engine.AppendLayers(g.layers[:0], gx, gy) {
				g.z = append(g.z, l.Height)
				g.nswe = append(g.nswe, l.NSWE)
				g.col = append(g.col, int32(c))
			}
		}
	}
	n := len(g.z)
	g.colStart[cols] = int32(n)
	g.next = resize(g.next, 4*n)
	g.edge = resize(g.edge, n)
	for i := range n {
		c := g.cell(int32(i))
		g.edge[i] = g.nswe[i] != block.AllDirections
		for d, dir := range cardinals {
			g.next[4*i+d] = -1
			nc, _, ok := p.cross(c, g.nswe[i], dir)
			if !ok || nc.Z < c.Z-block.CellIgnoreHeight {
				g.edge[i] = true // a wall, a climb over 48, no-go ground or a drop
			}
			if ok {
				g.next[4*i+d] = g.index(nc)
			}
		}
	}
}

// appendSteps appends the weighted steps out of cell i that stay inside the
// cluster, with the step costs of Planner.neighbours: cardinals, and a
// diagonal where both L-shaped detours land on the same cell, plus the
// clearance extra next to a wall or drop. It skips the diagonal's own
// CanMove: the costs only rank, and the build makes no CanMove call per
// cell this way.
func (g *localGraph) appendSteps(dst []step, i int32) []step {
	extra := int32(0)
	if g.edge[i] {
		extra = costClearance
	}
	nx := g.next[4*i : 4*i+4]
	for _, t := range nx {
		if t >= 0 {
			dst = append(dst, step{t, costCardinal + extra})
		}
	}
	for _, xi := range [2]int{0, 1} {
		for _, yi := range [2]int{2, 3} {
			a, b := nx[xi], nx[yi]
			if a < 0 || b < 0 {
				continue
			}
			if via := g.next[4*a+int32(yi)]; via >= 0 && via == g.next[4*b+int32(xi)] {
				dst = append(dst, step{via, costDiagonal + extra})
			}
		}
	}
	return dst
}

// step is one weighted edge of a localGraph.
type step struct {
	to   int32
	cost int32
}

// cells returns the number of cell layers loaded.
func (g *localGraph) cells() int { return len(g.z) }

// cell returns cell i.
func (g *localGraph) cell(i int32) Cell {
	c := int(g.col[i])
	return Cell{X: g.x0 + c%g.size, Y: g.y0 + c/g.size, Z: g.z[i]}
}

// layer returns cell i's index among its column's layers.
func (g *localGraph) layer(i int32) int { return int(i - g.colStart[g.col[i]]) }

// index returns the index of c, or -1 when c is outside the cluster or no
// layer of its column has its floor.
func (g *localGraph) index(c Cell) int32 {
	lx, ly := c.X-g.x0, c.Y-g.y0
	if lx < 0 || ly < 0 || lx >= g.size || ly >= g.size {
		return -1
	}
	col := ly*g.size + lx
	for i := g.colStart[col]; i < g.colStart[col+1]; i++ {
		if g.z[i] == c.Z {
			return i
		}
	}
	return -1
}

// sccScratch is the working memory of Tarjan's algorithm, run iteratively so
// a cluster of 65536 cells cannot overflow the goroutine stack.
type sccScratch struct {
	index, low, label []int32
	onStack           []bool
	stack             []int32
	calls             []sccFrame
}

type sccFrame struct{ v, k int32 }

// run labels the strongly connected components of a graph of n vertices
// whose vertex v has deg(v) edge slots, slot k leading to edge(v, k) (or -1
// for an empty slot). It returns the component count; s.label holds each
// vertex's component. Components are numbered in completion order, so every
// edge between two components goes from a higher label to a lower one.
func (s *sccScratch) run(n int, deg func(v int32) int32, edge func(v, k int32) int32) int {
	s.index = fill(s.index, n, -1)
	s.low = resize(s.low, n)
	s.label = fill(s.label, n, -1)
	s.onStack = resize(s.onStack, n)
	clear(s.onStack)
	s.stack, s.calls = s.stack[:0], s.calls[:0]
	next, count := int32(0), 0
	for root := range int32(n) {
		if s.index[root] >= 0 {
			continue
		}
		s.open(root, &next)
		for len(s.calls) > 0 {
			f := &s.calls[len(s.calls)-1]
			v := f.v
			if f.k < deg(v) {
				w := edge(v, f.k)
				f.k++
				switch {
				case w < 0:
				case s.index[w] < 0:
					s.open(w, &next)
				case s.onStack[w]:
					s.low[v] = min(s.low[v], s.index[w])
				}
				continue
			}
			s.calls = s.calls[:len(s.calls)-1]
			if len(s.calls) > 0 {
				u := s.calls[len(s.calls)-1].v
				s.low[u] = min(s.low[u], s.low[v])
			}
			if s.low[v] != s.index[v] {
				continue
			}
			for {
				w := s.stack[len(s.stack)-1]
				s.stack = s.stack[:len(s.stack)-1]
				s.onStack[w] = false
				s.label[w] = int32(count)
				if w == v {
					break
				}
			}
			count++
		}
	}
	return count
}

func (s *sccScratch) open(v int32, next *int32) {
	s.index[v], s.low[v] = *next, *next
	*next++
	s.stack = append(s.stack, v)
	s.onStack[v] = true
	s.calls = append(s.calls, sccFrame{v: v})
}

// localSCCs labels the cluster-local strongly connected components of g.
func (s *sccScratch) localSCCs(g *localGraph) int {
	return s.run(g.cells(),
		func(int32) int32 { return 4 },
		func(v, k int32) int32 { return g.next[4*v+k] })
}

func resize[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

func fill[T any](s []T, n int, v T) []T {
	s = resize(s, n)
	for i := range s {
		s[i] = v
	}
	return s
}
