package nav

import (
	"errors"
	"testing"
)

// pathCost sums the step costs of a cell path, failing when two cells in a
// row are not joined by an edge outside avoid.
func pathCost(t *testing.T, p *Planner, cells []Cell, avoid map[edgeKey]struct{}) int32 {
	t.Helper()
	var cost int32
	for i := 1; i < len(cells); i++ {
		if _, ok := avoid[edgeKey{cells[i-1], cells[i]}]; ok {
			t.Fatalf("step %d, %v -> %v, is avoided", i, cells[i-1], cells[i])
		}
		found := false
		for _, e := range p.neighbours(nil, cells[i-1]) {
			if e.cell == cells[i] {
				cost, found = cost+e.cost, true
				break
			}
		}
		if !found {
			t.Fatalf("step %d, %v -> %v, is no edge", i, cells[i-1], cells[i])
		}
	}
	return cost
}

// TestMiddleLegRefinesEdge checks the refinement of every abstract edge of
// the scene hierarchy: the leg between its two nodes' representatives is a
// way inside their clusters, where the build found the edge.
func TestMiddleLegRefinesEdge(t *testing.T) {
	th := hierScenes32(t)
	h, g := th.h, th.g
	p := testPlanner(t)
	s := getScratch()
	defer putScratch(s)
	intra, inter := 0, 0
	for n := range g.Nodes() {
		first, end := g.nodeEdges(n)
		for e := first; e < end; e++ {
			ed := g.edge(e)
			a, c := g.nodeRep(n), g.nodeRep(ed.to)
			cells, _, err := h.middleLeg(s, p, a, c, 1<<20, nil)
			if err != nil {
				t.Fatalf("edge %d -> %d: %v", n, ed.to, err)
			}
			ba, bc := h.clusterBox(a), h.clusterBox(c)
			for _, x := range cells {
				if !ba.has(x) && !bc.has(x) {
					t.Fatalf("edge %d -> %d: leg leaves its clusters at %v", n, ed.to, x)
				}
			}
			pathCost(t, p, cells, nil)
			if cells[0] != a || cells[len(cells)-1] != c {
				t.Fatalf("edge %d -> %d: leg runs %v .. %v, want %v .. %v", n, ed.to, cells[0], cells[len(cells)-1], a, c)
			}
			if g.nodeCluster(n) == g.nodeCluster(ed.to) {
				intra++
			} else {
				inter++
			}
		}
	}
	if intra == 0 || inter == 0 {
		t.Fatalf("checked %d edges inside a cluster and %d between two; want both", intra, inter)
	}
}

// TestMiddleLegLeavesBoxWhenCut checks the fallback: when the avoided edges
// cut every way inside the clusters' box, the leg searches anywhere.
func TestMiddleLegLeavesBoxWhenCut(t *testing.T) {
	h := hierScenes32(t).h
	p := testPlanner(t)
	a := pointCell(Open)
	box := h.clusterBox(a)
	c := Cell{X: a.X + 6, Y: a.Y, Z: a.Z}
	if !box.has(c) {
		t.Fatalf("%v and %v: want one cluster", a, c)
	}
	// Cut every edge from column a.X+3 east, across the whole box.
	avoid := map[edgeKey]struct{}{}
	for y := box.y0; y < box.y1; y++ {
		from := Cell{X: a.X + 3, Y: y, Z: a.Z}
		for _, e := range p.neighbours(nil, from) {
			if e.cell.X > from.X {
				avoid[edgeKey{from, e.cell}] = struct{}{}
			}
		}
	}
	s := getScratch()
	defer putScratch(s)
	if _, _, err := p.search(s, a, c, 1<<20, avoid, box); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("search inside the box: %v, want ErrUnreachable", err)
	}
	cells, _, err := h.middleLeg(s, p, a, c, 1<<20, avoid)
	if err != nil {
		t.Fatal(err)
	}
	pathCost(t, p, cells, avoid)
	out := false
	for _, x := range cells {
		out = out || !box.has(x)
	}
	if !out || cells[0] != a || cells[len(cells)-1] != c {
		t.Fatalf("leg %v .. %v (%d cells) out of the box %v; want a leg from %v to %v round the cut", cells[0], cells[len(cells)-1], len(cells), out, a, c)
	}
}
