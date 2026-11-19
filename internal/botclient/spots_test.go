package botclient

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// testSpot is a spot centred at (x, y) whose monsters span levels lo-hi,
// none aggressive, with a cap of max NPCs.
func testSpot(name string, x, y int32, lo, hi, maxNPCs int) datapack.Spot {
	return datapack.Spot{Maker: name, Center: datapack.Point{X: x, Y: y}, MinLevel: lo, MaxLevel: hi, MaximumNPCs: maxNPCs}
}

// newSpotBrain is a level brain at the origin with the default persona and
// no idle drift, picking from spots (spawn multiplier 1).
func newSpotBrain(t *testing.T, level int32, spots ...datapack.Spot) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, _ := newTestBrain(t, p)
	b.world.Self.Level = level
	b.cfg.Spots = NewSpotRegistry(spots, 1)
	return b
}

func TestSpotCap(t *testing.T) {
	tests := []struct {
		maximumNPCs int
		mult        float64
		want        int
	}{
		{9, 1, 3},
		{10, 1, 3},
		{2, 1, 1},
		{0, 1, 1},
		{10, 1.5, 5},
		// 5 x 0.5 rounds half up to 3, as the server does.
		{5, 0.5, 1},
		{20, 0.5, 3},
	}
	for _, tt := range tests {
		if got := spotCap(tt.maximumNPCs, tt.mult); got != tt.want {
			t.Errorf("spotCap(%d, %g) = %d, want %d", tt.maximumNPCs, tt.mult, got, tt.want)
		}
	}
}

func TestSpotRegistryClaim(t *testing.T) {
	r := NewSpotRegistry([]datapack.Spot{testSpot("a", 0, 0, 1, 3, 6), testSpot("b", 0, 0, 1, 3, 3)}, 1)
	steps := []struct {
		name    string
		claim   []int
		release int
		want    int
		wantOK  bool
	}{
		{name: "first in order", claim: []int{0, 1}, release: -1, want: 0, wantOK: true},
		{name: "a again, its cap is 2", claim: []int{0, 1}, release: -1, want: 0, wantOK: true},
		{name: "a full: b", claim: []int{0, 1}, release: -1, want: 1, wantOK: true},
		{name: "both full", claim: []int{0, 1}, release: -1},
		{name: "a released: a", claim: []int{1, 0}, release: 0, want: 0, wantOK: true},
		{name: "empty order", release: -1},
	}
	for _, s := range steps {
		if s.release >= 0 {
			r.release(s.release)
		}
		got, ok := r.claim(s.claim)
		if ok != s.wantOK || (ok && got != s.want) {
			t.Fatalf("%s: claim = %d, %v, want %d, %v", s.name, got, ok, s.want, s.wantOK)
		}
	}
	if r.Held(0) != 2 || r.Held(1) != 1 {
		t.Fatalf("held = %d, %d, want 2, 1", r.Held(0), r.Held(1))
	}
	r.release(1)
	r.release(1)
	if r.Held(1) != 0 {
		t.Fatalf("held after a second release = %d, want 0", r.Held(1))
	}
}

// TestSpotRegistryConcurrent has many bots claim one spot at once: no more
// than its cap get it.
func TestSpotRegistryConcurrent(t *testing.T) {
	r := NewSpotRegistry([]datapack.Spot{testSpot("a", 0, 0, 1, 3, 12)}, 1)
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 20 {
		wg.Go(func() {
			if _, ok := r.claim([]int{0}); ok {
				mu.Lock()
				won++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if won != 4 || r.Held(0) != 4 {
		t.Fatalf("won %d, held %d, want 4", won, r.Held(0))
	}
}

func TestSpotPolicyFits(t *testing.T) {
	p := defaultSpotPolicy()
	tests := []struct {
		name  string
		spot  datapack.Spot
		level int
		want  bool
	}{
		{"monsters around the level", datapack.Spot{MinLevel: 3, MaxLevel: 6}, 5, true},
		{"highest two above", datapack.Spot{MinLevel: 3, MaxLevel: 7}, 5, true},
		{"highest three above", datapack.Spot{MinLevel: 3, MaxLevel: 8}, 5, false},
		{"aggressive at the level", datapack.Spot{MinLevel: 3, MaxLevel: 5, AggroLevel: 5}, 5, true},
		{"aggressive above the level", datapack.Spot{MinLevel: 3, MaxLevel: 7, AggroLevel: 7}, 6, false},
		{"newbie: aggressive two above", datapack.Spot{MinLevel: 1, MaxLevel: 3, AggroLevel: 3}, 1, true},
		{"newbie at the last level: aggressive two above", datapack.Spot{MinLevel: 3, MaxLevel: 7, AggroLevel: 7}, 5, true},
		{"newbie: aggressive three above", datapack.Spot{MinLevel: 1, MaxLevel: 4, AggroLevel: 4}, 1, false},
		{"past newbie: aggressive one above", datapack.Spot{MinLevel: 4, MaxLevel: 7, AggroLevel: 7}, 6, false},
		{"highest three below", datapack.Spot{MinLevel: 1, MaxLevel: 2}, 5, true},
		{"highest four below", datapack.Spot{MinLevel: 1, MaxLevel: 1}, 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.fits(&tt.spot, tt.level); got != tt.want {
				t.Fatalf("fits = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuleSpot(t *testing.T) {
	now := at(time.Second)
	// near and far fit level 5; low is outlevelled, high too strong,
	// distant out of reach.
	spots := []datapack.Spot{
		testSpot("low", 100, 0, 1, 1, 9),
		testSpot("high", 100, 0, 8, 9, 9),
		testSpot("far", 0, 5000, 2, 4, 9),
		testSpot("near", 4000, 0, 3, 6, 9),
		testSpot("distant", 30000, 0, 3, 6, 9),
	}
	tests := []struct {
		name     string
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
		// wantMove is the leg's destination of a move.
		wantMove Point
		wantHeld int
		// wantSpot is the spot held after, -1 for none.
		wantSpot int
	}{
		{name: "nearest fitting spot: straight to it", wantRule: "spot", wantKind: actMove, wantMove: Point{X: 4000}, wantSpot: 3},
		{name: "leg to the end", setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 1500}) }, wantRule: "spot", wantKind: actMove, wantMove: Point{X: 4000}, wantSpot: 3},
		{name: "no registry", setup: func(b *Brain) { b.cfg.Spots = nil }, wantSpot: -1},
		{name: "combat goes first", setup: func(b *Brain) { b.world.Self.InCombat = true }, wantRule: "attacked", wantSpot: -1},
		{
			name:     "nearest full: the next one",
			setup:    func(b *Brain) { b.cfg.Spots.claim([]int{3}); b.cfg.Spots.claim([]int{3}); b.cfg.Spots.claim([]int{3}) },
			wantRule: "spot", wantKind: actMove, wantMove: Point{Y: 5000}, wantSpot: 2,
		},
		{
			name:     "nearest avoided: the next one",
			setup:    func(b *Brain) { b.spot.avoid = map[int]time.Time{3: at(time.Minute)} },
			wantRule: "spot", wantKind: actMove, wantMove: Point{Y: 5000}, wantSpot: 2,
		},
		{
			name:     "avoidance over",
			setup:    func(b *Brain) { b.spot.avoid = map[int]time.Time{3: now} },
			wantRule: "spot", wantKind: actMove, wantMove: Point{X: 4000}, wantSpot: 3,
		},
		{
			name:     "none fits: the goal's turn",
			setup:    func(b *Brain) { b.world.Self.Level = 20 },
			wantSpot: -1,
		},
		{
			name:     "sitting stands up first",
			setup:    func(b *Brain) { b.world.Self.Sitting = true },
			wantRule: "spot", wantKind: actSitStand, wantSpot: 3,
		},
		{
			name:     "on the way: holds",
			setup:    func(b *Brain) { b.world.Self.move = motion{to: Point{X: 3000}, start: t0, speed: 100} },
			wantRule: "spot", wantSpot: 3,
		},
		{
			name: "arrived: the goal's turn",
			setup: func(b *Brain) {
				b.world.Self.move = stopAt(Point{X: 3900})
			},
			wantSpot: 3,
		},
		{
			name: "arrived crowded: moves on",
			setup: func(b *Brain) {
				b.world.Self.move = stopAt(Point{X: 3900})
				for i := range int32(4) {
					b.world.Apply(CharInfo{ObjectID: pcID + 1 + i, Name: "p", Pos: Point{X: 4000}}, t0)
				}
			},
			wantRule: "spot", wantSpot: -1,
		},
		{
			name: "co-holders under the cap are no crowd",
			setup: func(b *Brain) {
				// Spot 3 takes 3 bots; the 2 others holding it stand there
				// with 3 strangers: 5 in sight, 3 that count.
				b.cfg.Spots.claim([]int{3})
				b.cfg.Spots.claim([]int{3})
				b.world.Self.move = stopAt(Point{X: 3900})
				for i := range int32(5) {
					b.world.Apply(CharInfo{ObjectID: pcID + 1 + i, Name: "p", Pos: Point{X: 4000}}, t0)
				}
			},
			wantHeld: 3, wantSpot: 3,
		},
		{
			name: "players beyond the leash are no crowd",
			setup: func(b *Brain) {
				b.world.Self.move = stopAt(Point{X: 3900})
				for i := range int32(4) {
					b.world.Apply(CharInfo{ObjectID: pcID + 1 + i, Name: "p", Pos: Point{X: 4000, Y: 1600}}, t0)
				}
			},
			wantSpot: 3,
		},
		{
			name: "party members are no crowd",
			setup: func(b *Brain) {
				b.world.Self.move = stopAt(Point{X: 3900})
				b.world.Party.LeaderID = selfID
				b.world.Party.Members = map[int32]*PartyMate{}
				for i := range int32(4) {
					b.world.Apply(CharInfo{ObjectID: pcID + 1 + i, Name: "p", Pos: Point{X: 4000}}, t0)
					b.world.Party.Members[pcID+1+i] = &PartyMate{}
				}
			},
			wantSpot: 3,
		},
		{
			name: "beyond the leash: back",
			setup: func(b *Brain) {
				b.cfg.Spots.claim([]int{3})
				b.spot.idx, b.spot.held, b.spot.arrived = 3, true, true
				b.world.Self.move = stopAt(Point{X: 2000})
			},
			wantRule: "spot", wantKind: actMove, wantMove: Point{X: 4000}, wantHeld: 1, wantSpot: 3,
		},
		{
			name: "within the leash after a fight",
			setup: func(b *Brain) {
				b.cfg.Spots.claim([]int{3})
				b.spot.idx, b.spot.held, b.spot.arrived = 3, true, true
				b.world.Self.move = stopAt(Point{X: 3000})
			},
			wantHeld: 1, wantSpot: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, spots...)
			b.world.Objects = map[int32]*Object{}
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(now)
			if rule != tt.wantRule || a.kind != tt.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tt.wantRule, tt.wantKind)
			}
			if a.kind == actMove {
				if got := moveTarget(a.payload); got != tt.wantMove {
					t.Fatalf("move to %+v, want %+v", got, tt.wantMove)
				}
			}
			gotSpot := -1
			if b.spot.held {
				gotSpot = b.spot.idx
			}
			if gotSpot != tt.wantSpot {
				t.Fatalf("spot = %d, want %d", gotSpot, tt.wantSpot)
			}
			if r := b.cfg.Spots; r != nil && tt.wantSpot >= 0 && tt.wantHeld > 0 && r.Held(tt.wantSpot) != tt.wantHeld {
				t.Fatalf("held = %d, want %d", r.Held(tt.wantSpot), tt.wantHeld)
			}
		})
	}
}

// TestSpotCrowdMovesOn checks a crowded spot is released and avoided, and
// the next decision walks to another.
func TestSpotCrowdMovesOn(t *testing.T) {
	b := newSpotBrain(t, 3, testSpot("near", 400, 0, 1, 3, 9), testSpot("next", 0, 2000, 1, 3, 9))
	b.world.Objects = map[int32]*Object{}
	for i := range int32(4) {
		b.world.Apply(CharInfo{ObjectID: pcID + i, Name: "p", Pos: Point{X: 400}}, t0)
	}
	if rule, a := b.decide(at(time.Second)); rule != "spot" || a.kind != actMove {
		t.Fatalf("first decide = %q %d, want a move", rule, a.kind)
	}
	b.world.Self.move = stopAt(Point{X: 350})
	if rule, a := b.decide(at(2 * time.Second)); rule != "spot" || a.kind != actHold || b.spot.held {
		t.Fatalf("arrival = %q %d held %v, want moving on", rule, a.kind, b.spot.held)
	}
	if b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("crowded spot still held %d", b.cfg.Spots.Held(0))
	}
	rule, a := b.decide(at(3 * time.Second))
	if rule != "spot" || a.kind != actMove || b.spot.idx != 1 {
		t.Fatalf("after moving on = %q %d spot %d, want a move to spot 1", rule, a.kind, b.spot.idx)
	}
	if got, want := moveTarget(a.payload), (Point{Y: 2000}); got != want {
		t.Fatalf("move to %+v, want %+v", got, want)
	}
}

// TestSpotTeleportWalksBack checks an own teleport away from the spot keeps
// the claim and walks back to the spot's centre, the leg count starting over.
func TestSpotTeleportWalksBack(t *testing.T) {
	b := newSpotBrain(t, 3, testSpot("near", 4000, 0, 1, 3, 9))
	b.world.Objects = map[int32]*Object{}
	b.world.Self.move = stopAt(Point{X: 3950})
	b.decide(at(time.Second))
	b.spot.lastDist, b.spot.stalls = 50, 2
	if !b.spot.held || !b.spot.arrived {
		t.Fatalf("not at the spot: held %v arrived %v", b.spot.held, b.spot.arrived)
	}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: Point{X: 9000}}}, at(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.spot.arrived || b.spot.lastDist != 0 || b.spot.stalls != 0 || !b.spot.held || b.cfg.Spots.Held(0) != 1 {
		t.Fatalf("after teleport: %+v held %d, want the claim kept and the walk reset", b.spot, b.cfg.Spots.Held(0))
	}
	b.world.Self.move = stopAt(Point{X: 9000})
	b.appearing = false // the Appearing the teleport asks went out
	rule, a := b.decide(at(3 * time.Second))
	if rule != "spot" || a.kind != actMove {
		t.Fatalf("after teleport = %q %d, want a leg to the spot", rule, a.kind)
	}
	if got, want := moveTarget(a.payload), (Point{X: 4000}); got != want {
		t.Fatalf("leg to %+v, want %+v", got, want)
	}
}

// TestSpotStalls checks legs that get nowhere give the spot up.
func TestSpotStalls(t *testing.T) {
	b := newSpotBrain(t, 3, testSpot("near", 4000, 0, 1, 3, 9), testSpot("next", 0, -5000, 1, 3, 9))
	for i := range spotStalls {
		if rule, a := b.decide(at(time.Duration(i+1) * time.Second)); rule != "spot" || a.kind != actMove {
			t.Fatalf("leg %d = %q %d, want a move", i, rule, a.kind)
		}
	}
	if rule, a := b.decide(at(10 * time.Second)); rule != "spot" || a.kind != actHold || b.spot.held {
		t.Fatalf("stalled = %q %d held %v, want the spot given up", rule, a.kind, b.spot.held)
	}
	if !at(10*time.Second).Before(b.spot.avoid[0]) || b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("stalled spot not avoided or still held: %v, %d", b.spot.avoid, b.cfg.Spots.Held(0))
	}
}

// TestSpotReselect checks re-selection on level-up with hysteresis: the bot
// keeps its spot up to Outlevel levels over it, then claims a fitting one
// and frees the old; the old one no longer fits, so it does not flip back.
func TestSpotReselect(t *testing.T) {
	b := newSpotBrain(t, 5, testSpot("newbie", 1000, 0, 2, 5, 9), testSpot("next", 0, 3000, 8, 11, 9))
	b.world.Objects = map[int32]*Object{}
	steps := []struct {
		name     string
		level    int32
		wantSpot int
	}{
		{"level 5: the newbie spot", 5, 0},
		{"level 10: still fits under Outlevel", 10, 0},
		{"level 11: outlevelled, moves up", 11, 1},
		{"level 16: still keeps the new one", 16, 1},
	}
	for i, s := range steps {
		b.world.Self.Level = s.level
		b.decide(at(time.Duration(i+1) * time.Second))
		if !b.spot.held || b.spot.idx != s.wantSpot {
			t.Fatalf("%s: spot %d held %v, want %d", s.name, b.spot.idx, b.spot.held, s.wantSpot)
		}
		other := 1 - s.wantSpot
		if b.cfg.Spots.Held(s.wantSpot) != 1 || b.cfg.Spots.Held(other) != 0 {
			t.Fatalf("%s: held %d/%d, want 1/0", s.name, b.cfg.Spots.Held(s.wantSpot), b.cfg.Spots.Held(other))
		}
	}
	// Outlevelled with nothing better free: keeps the spot it has.
	b.world.Self.Level = 30
	b.decide(at(time.Minute))
	if !b.spot.held || b.spot.idx != 1 || !b.spot.retryAt.Equal(at(time.Minute).Add(spotRetry)) {
		t.Fatalf("no better spot: spot %d held %v retry %v", b.spot.idx, b.spot.held, b.spot.retryAt)
	}
}

// TestSpotReleased checks the spot is given back when the bot dies and
// when its brain stops.
func TestSpotReleased(t *testing.T) {
	b := newSpotBrain(t, 3, testSpot("near", 4000, 0, 1, 3, 9))
	b.decide(at(time.Second))
	if b.cfg.Spots.Held(0) != 1 {
		t.Fatalf("held %d, want 1", b.cfg.Spots.Held(0))
	}
	if err := b.handle([]Event{Die{ObjectID: selfID}}, at(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.spot.held || b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("after death: held %v, %d", b.spot.held, b.cfg.Spots.Held(0))
	}

	b = newSpotBrain(t, 3, testSpot("near", 4000, 0, 1, 3, 9))
	b.decide(at(time.Second))
	done := make(chan struct{})
	close(done)
	if err := b.Run(done, nil); err != nil {
		t.Fatal(err)
	}
	if b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("after Run: held %d, want 0", b.cfg.Spots.Held(0))
	}
}

// TestSpotLegLength: a spot within spotDirect is one move to its centre,
// for the server's pathfinder to route; a farther one a spotLeg leg.
func TestSpotLegLength(t *testing.T) {
	tests := []struct {
		name string
		x    int32
		want Point
	}{
		{"within spotDirect", spotDirect, Point{X: spotDirect}},
		{"beyond spotDirect", spotDirect + 1000, Point{X: spotLeg}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, testSpot("s", tt.x, 0, 3, 6, 9))
			name, act := b.decide(at(time.Second))
			if got := moveTarget(act.payload); name != "spot" || act.kind != actMove || got != tt.want {
				t.Fatalf("decide = %q kind %v, move to %v; want a move to %v", name, act.kind, moveTarget(act.payload), tt.want)
			}
		})
	}
}
