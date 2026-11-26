package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestIdleDriftOpen draws idle drifts on the synthetic scenes (#617). Within
// its radius a bot strolls in the open: never under the bridge's deck (a
// roof over the ground) and never round the U's wall to the ground behind
// it, each drift one straight checked segment. A bot under a roof routes
// out to the open, and a bot beyond the radius back toward its centre.
func TestIdleDriftOpen(t *testing.T) {
	n := testNav(t)
	// A roofed hut next to the pocket: a wall ring at columns 1650-1681 and
	// rows 270-301, a door gap at rows 284-287 of its west wall, a roof over
	// its floor.
	hutNav, err := buildTestNav(geotest.Scenes().
		Raise(1650, 270, 1681, 271, geotest.Wall).Raise(1650, 300, 1681, 301, geotest.Wall).
		Raise(1650, 270, 1651, 283, geotest.Wall).Raise(1650, 288, 1651, 301, geotest.Wall).
		Raise(1680, 270, 1681, 301, geotest.Wall).
		AddDeck(1652, 272, 1679, 299, geotest.Deck), navWindow(6, 1, 6, 1))
	if err != nil {
		t.Fatal(err)
	}
	hut := geotest.Point{CellX: 1675, CellY: 295, Z: geotest.Ground}
	deckX0, deckX1 := int32(geotest.CellWorldX(1290)-8), int32(geotest.CellWorldX(1330)+7)
	deckY0, deckY1 := int32(geotest.CellWorldY(286)-8), int32(geotest.CellWorldY(289)+7)
	behindU := int32(geotest.CellWorldX(543) + 7)
	tests := []struct {
		name   string
		nav    *Nav
		from   Point
		anchor Point
		radius int32
		// straight: every drift is one segment the walker's check passes.
		straight bool
		// covered: from is under a roof.
		covered bool
		ok      func(dest Point) bool
	}{
		{
			name: "deck overhead", from: scenePoint(geotest.BridgeWestGround, 18, -7), radius: 160, straight: true,
			ok: func(d Point) bool {
				return d.X < deckX0 || d.X > deckX1 || d.Y < deckY0 || d.Y > deckY1
			},
		},
		{
			name: "wall between", from: scenePoint(geotest.UInside, 0, 0), radius: 400, straight: true,
			ok: func(d Point) bool { return d.X <= behindU },
		},
		{
			// The far corner, no straight segment out through the door: the
			// drift routes out to the open.
			name: "under a roof", nav: hutNav, from: scenePoint(hut, 0, 0), radius: 400, covered: true,
			ok: func(Point) bool { return true },
		},
		{
			name: "beyond the radius", from: scenePoint(geotest.UInside, 0, 0), anchor: scenePoint(geotest.UBehind, 0, 0), radius: 64,
			ok: func(d Point) bool {
				c := scenePoint(geotest.UBehind, 0, 0)
				r := int64(64 + walkSlop)
				return horizontal2(d, c) <= r*r
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tn := tc.nav
			if tn == nil {
				tn = n
			}
			b, _, _ := newNavBrain(t, tn, Persona{IdleChance: 1, IdleRadius: tc.radius}, tc.from)
			b.anchor = tc.anchor
			if b.anchor == (Point{}) {
				b.anchor = tc.from
			}
			w := &b.walk
			if got := w.open(tc.from); got == tc.covered {
				t.Fatalf("from %+v: open %v, want %v", tc.from, got, !tc.covered)
			}
			var drifts int
			for i := range 200 {
				b.walk = newWalker(tn)
				if _, ok := b.ruleIdle(at(time.Duration(i) * time.Second)); !ok || w.purpose != walkIdle {
					continue
				}
				drifts++
				if !tc.ok(w.dest) {
					t.Fatalf("drift %d to %+v", i, w.dest)
				}
				if !w.open(w.dest) {
					t.Fatalf("drift %d to %+v, under a layer over the ground", i, w.dest)
				}
				if tc.straight && !w.segmentOK(tc.from, w.dest) {
					t.Fatalf("drift %d to %+v, not one straight segment from %+v", i, w.dest, tc.from)
				}
			}
			if drifts < 20 {
				t.Errorf("%d drifts in 200 draws, want about half", drifts)
			}
		})
	}
}

// TestWalkerOpen: ground under the bridge's deck is covered, the deck and
// open ground are not.
func TestWalkerOpen(t *testing.T) {
	b, _, _ := newNavBrain(t, testNav(t), Persona{}, scenePoint(geotest.UInside, 0, 0))
	w := b.walker()
	for _, tc := range []struct {
		name string
		p    Point
		want bool
	}{
		{"ground under the deck", scenePoint(geotest.BridgeWestGround, 0, 0), false},
		{"the deck", scenePoint(geotest.BridgeWestDeck, 0, 0), true},
		{"open ground", scenePoint(geotest.UInside, 0, 0), true},
	} {
		if got := w.open(tc.p); got != tc.want {
			t.Errorf("%s %+v: open %v, want %v", tc.name, tc.p, got, tc.want)
		}
	}
}
