package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

func TestNearestWalkable(t *testing.T) {
	p := testPlanner(t)
	tests := []struct {
		name string
		at   location.Location
		r    float64
		want geotest.Point
		ok   bool
	}{
		{"on open ground", at(Open, 2, 3, 0), 50, Open, true},
		{"deck by Z", at(geotest.BridgeWestDeck, 0, 0, 20), 50, geotest.BridgeWestDeck, true},
		{"unloaded region edge", at(geotest.Point{CellX: -2, CellY: 100}, 0, 0, 0), 64, geotest.Point{CellX: 0, CellY: 100}, true},
		{"ground under a low layer: next cell", at(StackedGround, 0, -4, 0), 80, geotest.Point{CellX: 302, CellY: 999}, true},
		{"nothing loaded in reach", at(geotest.Point{CellX: -20, CellY: 100}, 0, 0, 0), 64, geotest.Point{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := p.NearestWalkable(tt.at, tt.r)
			if ok != tt.ok {
				t.Fatalf("NearestWalkable = %v, %v; want ok %v", c, ok, tt.ok)
			}
			if ok && c.World() != loc(tt.want) {
				t.Fatalf("NearestWalkable = %v, want %v", c.World(), loc(tt.want))
			}
		})
	}
}

func TestStandPoint(t *testing.T) {
	p := testPlanner(t)
	e := p.Engine
	baseX := at(geotest.Point{CellX: 543}, 0, 0, 0).X // east face of the U's base
	tests := []struct {
		name   string
		from   location.Location
		target Target
		check  func(location.Location) bool
	}{
		{
			"line: around the U's base, not inside it",
			loc(geotest.UInside),
			Target{At: loc(geotest.UBehind), Range: 300, Mode: StandLine},
			func(s location.Location) bool { return s.X > baseX },
		},
		{
			"sight: around the U's base, not inside it",
			loc(geotest.UInside),
			Target{At: loc(geotest.UBehind), Range: 300, Mode: StandSight, CollisionHeight: 23},
			func(s location.Location) bool { return s.X > baseX },
		},
		{
			"line with Accept: points north of the target skipped like blocked ones",
			loc(geotest.UInside),
			Target{At: loc(geotest.UBehind), Range: 300, Mode: StandLine, Accept: func(s location.Location) bool {
				return s.Y >= loc(geotest.UBehind).Y+64
			}},
			func(s location.Location) bool { return s.X > baseX && s.Y >= loc(geotest.UBehind).Y+64 },
		},
		{
			"pick-up on the plateau: CanMove to the raw Z needs the plateau",
			loc(geotest.LedgeLow),
			Target{At: at(geotest.LedgeHigh, -60, 0, 0), Range: 120, Mode: StandPickUp},
			func(s location.Location) bool { return s.Z == geotest.Plateau },
		},
		{
			"line into the pit from outside",
			loc(geotest.PitOutside),
			Target{At: loc(geotest.PitInside), Range: 300, Mode: StandLine},
			func(s location.Location) bool { return s.Z == geotest.Ground },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := p.StandPoint(tt.from, tt.target)
			if !ok {
				t.Fatal("no stand point")
			}
			if !p.offers(s, tt.target) || !tt.check(s) {
				t.Fatalf("stand point %v", s)
			}
			d := hypot(s.X-tt.target.At.X, s.Y-tt.target.At.Y)
			if d > tt.target.Range {
				t.Fatalf("stand point %v is %.0f from the target", s, d)
			}
			if c, ok := p.Locate(s); !ok || c.World() != s || int(e.Height(s.X, s.Y, s.Z)) != s.Z {
				t.Fatalf("stand point %v is not a cell centre on its floor", s)
			}
		})
	}
}

// TestStandPointAcceptNone checks that with every cell refused by Accept
// there is no stand point.
func TestStandPointAcceptNone(t *testing.T) {
	p := testPlanner(t)
	tgt := Target{At: loc(geotest.UBehind), Range: 300, Mode: StandLine, Accept: func(location.Location) bool { return false }}
	if s, ok := p.StandPoint(loc(geotest.UInside), tgt); ok {
		t.Fatalf("stand point %v, want none", s)
	}
}
