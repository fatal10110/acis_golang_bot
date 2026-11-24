package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// TestPullFartherExactCandidate walks from open ground 25 cells west of the
// stair up onto its deck (#307). The corridor test fails over the stair's
// cells of several layers, so the corridor reaches only the foot of it;
// the farther candidate, the deck itself, passes CanMove with a short
// failing stretch that passes the exact check: one segment. With the
// route's farWork spent, the pull falls back to exactCells candidates and
// takes more.
func TestPullFartherExactCandidate(t *testing.T) {
	p := testPlanner(t)
	from, to := at(StairLow, -25*block.CellSize, 0, 0), loc(StairDeck)
	route, err := p.Route(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(route) != 2 {
		t.Fatalf("route %v: %d segments, want 1", route, len(route)-1)
	}

	s := getScratch()
	defer putScratch(s)
	start, _ := p.Locate(from)
	goal, _ := p.Locate(to)
	cells, _, err := p.search(s, start, goal, p.Budget, nil, anywhere)
	if err != nil {
		t.Fatal(err)
	}
	from.Z, to.Z = int(start.Z), int(goal.Z)
	s.farEnd = s.work // spent
	if _, ok := p.pull(s, cells, from, to); !ok {
		t.Fatal("pull with farWork spent fails")
	}
	if len(s.route) < 3 {
		t.Fatalf("farWork spent: route %v, want more than one segment", s.route)
	}
	for i := 1; i < len(s.route); i++ {
		a, b := s.route[i-1], s.route[i]
		if !p.CorridorOpen(a, b) && hypot(b.X-a.X, b.Y-a.Y) > exactCells*block.CellSize*1.5 {
			t.Fatalf("farWork spent: exact segment %v -> %v past exactCells", a, b)
		}
	}
}

// TestBlockedEnd checks where a blocked stop's refused part ends (#307):
// one step and a cell past the stop on open ground, on to the end of the
// next stretch only the exact check passed, and the whole rest of the
// segment from a stop inside such a stretch.
func TestBlockedEnd(t *testing.T) {
	p := testPlanner(t)
	open, openEnd := at(Open, 0, 0, 0), at(Open, 640, 0, 0)
	stair, deck := at(StairLow, -25*block.CellSize, 0, 0), loc(StairDeck)
	s := getScratch()
	defer putScratch(s)
	s.grid.invalidate()
	stretches := p.corridorStretches(s, stair, deck)
	if len(stretches) != 1 {
		t.Fatalf("stair stretches %v, want one", stretches)
	}
	st := stretches[0]
	stairX := func(t float64) int { return stair.X + int(t+0.5) }
	tests := []struct {
		name    string
		a, b    location.Location
		stop    location.Location
		wantEnd location.Location
	}{
		{"open ground: a step and a cell", open, openEnd, at(Open, 100, 0, 0), at(Open, 100+testLmax+block.CellSize, 0, 0)},
		{"open ground near the end: the end", open, openEnd, at(Open, 620, 0, 0), openEnd},
		{
			"before the stair: on through its stretch", stair, deck, at(StairLow, -20*block.CellSize, 0, 0),
			location.Location{X: stairX(st.t1 + stretchPad), Y: stair.Y, Z: deck.Z},
		},
		{"inside the stair's stretch: the rest", stair, deck, location.Location{X: stairX(st.t0), Y: stair.Y, Z: 0}, deck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.BlockedEnd(tt.a, tt.b, tt.stop); got != tt.wantEnd {
				t.Fatalf("BlockedEnd = %v, want %v", got, tt.wantEnd)
			}
		})
	}
}
