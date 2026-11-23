package nav

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

func TestIndexTableGet(t *testing.T) {
	var tab indexTable
	if _, ok := tab.get(7); ok {
		t.Fatal("empty table holds 7")
	}
	for i := range 3000 {
		tab.getOrPut(uint64(i)*3, int32(i))
	}
	tests := []struct {
		key  uint64
		want int32
		ok   bool
	}{
		{0, 0, true},
		{3, 1, true},
		{8997, 2999, true},
		{4, 0, false},
		{9000, 0, false},
	}
	for _, tt := range tests {
		got, ok := tab.get(tt.key)
		if ok != tt.ok || got != tt.want {
			t.Errorf("get(%d) = %d, %v; want %d, %v", tt.key, got, ok, tt.want, tt.ok)
		}
	}
	tab.reset()
	if _, ok := tab.get(3); ok {
		t.Fatal("reset table holds 3")
	}
}

// TestReachersMatchSearch checks the backward walk against the forward
// search: a cell inside the box reaches the goal there exactly when the
// search bounded to the box finds a way.
func TestReachersMatchSearch(t *testing.T) {
	p := testPlanner(t)
	ox, oy := regionOrigin()
	box := func(x0, y0, x1, y1 int) cellBox { return cellBox{ox + x0, oy + y0, ox + x1, oy + y1} }
	tests := []struct {
		name string
		goal geotest.Point
		box  cellBox
	}{
		{"below the ledge", geotest.LedgeLow, box(780, 278, 816, 292)},
		{"on the ledge", geotest.LedgeHigh, box(780, 278, 816, 292)},
		{"in the pit", geotest.PitInside, box(1026, 280, 1066, 296)},
		{"out of the pit", geotest.PitOutside, box(1026, 280, 1066, 296)},
		{"across the bridge", geotest.BridgeWestDeck, box(1284, 280, 1336, 294)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goal := pointCell(tt.goal)
			s := getScratch()
			defer putScratch(s)
			p.reachers(s, goal, tt.box)
			var in []Cell
			var buf [block.MaxLayers]block.Cell
			for x := tt.box.x0; x < tt.box.x1; x++ {
				for y := tt.box.y0; y < tt.box.y1; y++ {
					for _, l := range p.Engine.AppendLayers(buf[:0], x, y) {
						c := Cell{X: x, Y: y, Z: l.Height}
						if _, ok := s.index.get(cellKey(c)); ok {
							in = append(in, c)
						}
					}
				}
			}
			reached := map[Cell]bool{}
			for _, c := range in {
				reached[c] = true
			}
			ok, no := 0, 0
			for x := tt.box.x0; x < tt.box.x1; x += 3 {
				for y := tt.box.y0; y < tt.box.y1; y += 3 {
					for _, l := range p.Engine.AppendLayers(buf[:0], x, y) {
						c := Cell{X: x, Y: y, Z: l.Height}
						_, _, err := p.search(s, c, goal, 1<<20, nil, tt.box)
						if found := err == nil; found != reached[c] {
							t.Fatalf("%v: search found a way %v, reachers %v (%v)", c, found, reached[c], err)
						}
						if reached[c] {
							ok++
						} else {
							no++
						}
					}
				}
			}
			if ok == 0 {
				t.Fatal("no sampled cell reaches the goal")
			}
			t.Logf("%d sampled cells reach the goal, %d do not", ok, no)
		})
	}
}

// TestAbstractSearchSeats checks that a seated route end starts or ends the
// abstract path on its seat nodes alone.
func TestAbstractSearchSeats(t *testing.T) {
	th := hierScenes32(t)
	h, g := th.h, th.g
	tests := []struct {
		name     string
		from, to geotest.Point
	}{
		{"around the U", geotest.UInside, geotest.UBehind},
		{"along the corridor", geotest.CorridorWest, geotest.CorridorEast},
		{"over the bridge", geotest.BridgeWestDeck, geotest.BridgeEastDeck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := h.Locate(pointCell(tt.from), nil)
			if err != nil {
				t.Fatal(err)
			}
			b, err := h.Locate(pointCell(tt.to), nil)
			if err != nil {
				t.Fatal(err)
			}
			if !h.SameSCC(a, b) || a.res.cluster == b.res.cluster {
				t.Fatalf("%v, %v: want SameSCC cells in two clusters", tt.from, tt.to)
			}
			s := getScratch()
			defer putScratch(s)
			sp := sameSpan(a)
			nodesOf := func(m Member) []int32 {
				var ns []int32
				first, end := g.clusterNodes(int(m.res.cluster))
				for n := first; n < end; n++ {
					if g.nodeSCC(n) == m.SCC() {
						ns = append(ns, int32(n))
					}
				}
				return ns
			}
			for _, n := range nodesOf(a) {
				seat := seating{starts: []int32{n}}
				if _, _, err := h.abstractSearch(s, a, b, &sp, nil, abstractBudget, &seat); err != nil {
					t.Fatalf("seated on start node %d: %v", n, err)
				}
				if s.apath[0] != n {
					t.Errorf("seated on start node %d: path starts on %d", n, s.apath[0])
				}
			}
			for _, n := range nodesOf(b) {
				seat := seating{goals: []int32{n}}
				if _, _, err := h.abstractSearch(s, a, b, &sp, nil, abstractBudget, &seat); err != nil {
					t.Fatalf("seated on goal node %d: %v", n, err)
				}
				if last := s.apath[len(s.apath)-1]; last != n {
					t.Errorf("seated on goal node %d: path ends on %d", n, last)
				}
			}
			seat := seating{starts: []int32{}, goals: nil}
			if _, _, err := h.abstractSearch(s, a, b, &sp, nil, abstractBudget, &seat); !errors.Is(err, ErrUnreachable) {
				t.Errorf("seated on no start node: %v, want ErrUnreachable", err)
			}
		})
	}
}
