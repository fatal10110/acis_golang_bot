package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestSpotBeside: a level 1 bot that finds every spot that fits it at its
// limit farms beside one with no claim (#142): it walks there and farms its
// monsters, counting toward neither the cap nor the limit, gives the place
// up on a death, and claims a spot once one has room again, never before
// spotRetry. With Beside off it farms nothing.
func TestSpotBeside(t *testing.T) {
	const mob = 0x5001
	tests := []struct {
		name   string
		beside bool
		run    func(t *testing.T, b *Brain)
	}{
		{
			name: "off: no spot", beside: false,
			run: func(t *testing.T, b *Brain) {
				if rule, _ := b.decide(at(time.Second)); rule == "spot" || b.spot.farming() {
					t.Fatalf("rule %q spot %+v: want nothing", rule, b.spot)
				}
			},
		},
		{
			name: "walks there and farms, claiming nothing", beside: true,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				rule, a := b.decide(at(time.Second))
				if rule != "spot" || a.kind != actMove || !b.spot.beside || b.spot.held || b.spot.idx != 0 {
					t.Fatalf("rule %q action %d spot %+v: want a walk to spot 0, beside it", rule, a.kind, b.spot)
				}
				if r.Held(0) != 3 || r.Beside(0) != 1 {
					t.Fatalf("held %d beside %d: want 3 and 1", r.Held(0), r.Beside(0))
				}
				b.world.Self.move = stopAt(Point{X: 2000})
				b.world.Apply(NPCInfo{ObjectID: mob, NPCID: 20001, Attackable: true, Pos: Point{X: 2300}}, at(2*time.Second))
				if rule, a := b.decide(at(2 * time.Second)); rule != "farm" || a.object != mob {
					t.Fatalf("at the spot: rule %q action %d on %#x, want farm %#x", rule, a.kind, a.object, mob)
				}
				if st := r.Statuses()[0]; st.Held != 3 || st.Beside != 1 {
					t.Fatalf("status %+v: want 3 held, 1 beside", st)
				}
			},
		},
		{
			name: "claims a spot once one has room", beside: true,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				b.decide(at(time.Second))
				r.release(1)
				b.decide(at(spotRetry))
				if !b.spot.beside {
					t.Fatalf("spot %+v before spotRetry: want still beside", b.spot)
				}
				b.decide(at(spotRetry + time.Second))
				if !b.spot.held || b.spot.idx != 1 || r.Held(1) != 3 || r.Beside(0) != 0 || r.Beside(1) != 0 {
					t.Fatalf("spot %+v held %d beside %d/%d: want spot 1 claimed, none beside", b.spot, r.Held(1), r.Beside(0), r.Beside(1))
				}
			},
		},
		{
			name: "keeps its place on a fruitless retry", beside: true,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				r.beside[1] = 2 // two bots beside spot 1: it picks spot 0
				b.decide(at(time.Second))
				// Another bot joins it beside spot 0 and the two leave spot
				// 1: spot 1 is now the emptier one beside.
				r.beside[0], r.beside[1] = 2, 0
				b.decide(at(spotRetry + time.Second))
				if !b.spot.beside || b.spot.idx != 0 || r.Beside(0) != 2 || r.Beside(1) != 0 {
					t.Fatalf("spot %+v beside %d/%d: want spot 0 kept", b.spot, r.Beside(0), r.Beside(1))
				}
			},
		},
		{
			name: "a death gives the place up", beside: true,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				b.decide(at(time.Second))
				b.diedAtSpot(at(2 * time.Second))
				if b.spot.farming() || r.Beside(0) != 0 || !at(2*time.Second).Before(b.spot.avoid[0]) {
					t.Fatalf("spot %+v beside %d avoid %v: want none, spot 0 avoided", b.spot, r.Beside(0), b.spot.avoid)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 2000, 0, 1, 3, 3), testSpot("b", 0, 6000, 1, 3, 3))
			b.persona.Spots.Beside = tt.beside
			b.world.Objects = map[int32]*Object{}
			r := b.cfg.Spots
			r.held[0], r.held[1] = 3, 3 // both at their limit
			tt.run(t, b)
		})
	}
}

// TestSpotBesideSpread: the bots beside full spots spread over them by
// their monsters, not onto the nearest, and none of them is counted as a
// holder: a holder arriving among them does not find its spot crowded.
func TestSpotBesideSpread(t *testing.T) {
	r := NewSpotRegistry([]datapack.Spot{testSpot("near", 2000, 0, 1, 3, 3), testSpot("far", 0, 6000, 1, 3, 6)}, 1)
	r.held[0], r.held[1] = 3, 6
	for range 6 {
		b := newSpotBrain(t, 1)
		b.cfg.Spots = r
		b.decide(at(time.Second))
		if !b.spot.beside {
			t.Fatalf("spot %+v: want beside one", b.spot)
		}
	}
	if r.Beside(0) != 2 || r.Beside(1) != 4 || r.Held(0) != 3 || r.Held(1) != 6 {
		t.Fatalf("beside %d/%d held %d/%d: want 2/4 beside, holders unchanged", r.Beside(0), r.Beside(1), r.Held(0), r.Held(1))
	}
	b := newSpotBrain(t, 1)
	b.cfg.Spots = r
	b.spot.idx, b.spot.held = 0, true
	b.world.Objects = map[int32]*Object{}
	// The other two holders and the two bots beside stand at the spot.
	for id := range int32(4) {
		b.world.Apply(CharInfo{ObjectID: 0x7000 + id, Name: "bot", Pos: Point{X: 2000, Y: 100 * id}}, at(time.Second))
	}
	if b.crowded(Point{X: 2000}, at(time.Second)) {
		t.Fatal("crowded by its own holders and the bots beside")
	}
	for id := range int32(4) {
		b.world.Apply(CharInfo{ObjectID: 0x7100 + id, Name: "player", Pos: Point{X: 2000, Y: -100 * id}}, at(time.Second))
	}
	if !b.crowded(Point{X: 2000}, at(time.Second)) {
		t.Fatal("four other players: want crowded")
	}
}

// TestSpotBesideLeaves: a bot beside a full spot that is no longer a
// candidate for it, outlevelled or avoided, gives its place there up on the
// next retry: it moves beside the full spot still left, or drops out when
// none is left, and the beside count it held is returned either way.
func TestSpotBesideLeaves(t *testing.T) {
	tests := []struct {
		name   string
		change func(b *Brain)
		want   int // the spot it farms beside after the retry, -1 for none
	}{
		{name: "outlevelled", change: func(b *Brain) { b.world.Self.Level = 5 }, want: 1},
		{name: "avoided", change: func(b *Brain) { b.spot.avoid = map[int]time.Time{0: at(time.Hour)} }, want: 1},
		{name: "none left", change: func(b *Brain) { b.world.Self.Level = 10 }, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 2000, 0, 1, 1, 3), testSpot("b", 0, 6000, 1, 3, 3))
			b.world.Objects = map[int32]*Object{}
			r := b.cfg.Spots
			r.held[0], r.held[1] = 3, 3 // both at their limit
			b.decide(at(time.Second))
			if !b.spot.beside || b.spot.idx != 0 || r.Beside(0) != 1 {
				t.Fatalf("spot %+v beside %d: want beside spot 0", b.spot, r.Beside(0))
			}
			tt.change(b)
			b.decide(at(spotRetry + time.Second))
			if r.Beside(0) != 0 {
				t.Fatalf("beside spot 0 %d after the retry: want 0", r.Beside(0))
			}
			if tt.want < 0 {
				if b.spot.farming() || r.Beside(1) != 0 {
					t.Fatalf("spot %+v beside spot 1 %d: want none", b.spot, r.Beside(1))
				}
				return
			}
			if !b.spot.beside || b.spot.idx != tt.want || r.Beside(tt.want) != 1 {
				t.Fatalf("spot %+v beside spot 1 %d: want beside spot %d", b.spot, r.Beside(1), tt.want)
			}
		})
	}
}
