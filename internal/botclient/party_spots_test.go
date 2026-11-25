package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// mate is the window row of the lead tests' member id at level, alive.
func mate(id, level int32) PartyMember {
	return PartyMember{ObjectID: id, Name: leadNames[id], Level: level, HP: 100, MaxHP: 100}
}

// leadWindow is the window of the party the bot leads with ms.
func leadWindow(ms ...PartyMember) PartySmallWindowAll {
	return PartySmallWindowAll{LeaderID: selfID, Loot: lootByTurn, Members: ms}
}

// feed hands b the events evs at t0+d.
func feed(t *testing.T, b *Brain, d time.Duration, evs ...Event) {
	t.Helper()
	if err := b.handle(evs, at(d)); err != nil {
		t.Fatal(err)
	}
}

// newLeadSpotBrain is newSpotBrain under a lead slot listing the bots
// Abel, Beth and Cole, leading the party of ms (none: no party).
func newLeadSpotBrain(t *testing.T, level int32, ms []PartyMember, spots ...datapack.Spot) *Brain {
	t.Helper()
	b := newSpotBrain(t, level, spots...)
	b.cfg.link = newSlotLink(standing{})
	b.setPartySlot(lead("Abel Beth Cole"))
	if len(ms) > 0 {
		feed(t, b, 0, leadWindow(ms...))
	}
	return b
}

// spotAt has b hold or farm beside spot i with weight w, counted in its
// registry.
func spotAt(b *Brain, i int, beside bool, w int) {
	r := b.cfg.Spots
	if beside {
		r.beside[i] += w
	} else {
		r.held[i] += w
	}
	b.spot.idx, b.spot.held, b.spot.beside, b.spot.weight = i, !beside, beside, w
	b.spot.at = Point{X: r.spots[i].Center.X, Y: r.spots[i].Center.Y}
}

// wantCounts fails unless spot i of b's registry is held by held and
// farmed beside by beside.
func wantCounts(t *testing.T, b *Brain, when string, i, held, beside int) {
	t.Helper()
	r := b.cfg.Spots
	if r.Held(i) != held || r.Beside(i) != beside {
		t.Fatalf("%s: spot %d held %d beside %d, want %d and %d (spot %+v)", when, i, r.Held(i), r.Beside(i), held, beside, b.spot)
	}
}

// TestPartyClaimN: a leader claims, keeps and gives back room for its whole
// party window, players included (docs/parties/spots.md, "Claim weight"
// and "Weighted release"); a window add or delete, the party's end or its
// slot clearing re-weigh it in one step, keeping the spot.
func TestPartyClaimN(t *testing.T) {
	// two takes 2 (cap), three 3, four 4; all fit level 1.
	two, three, four := testSpot("two", 2000, 0, 1, 3, 6), testSpot("three", 0, 3000, 1, 3, 9), testSpot("four", 3000, 3000, 1, 3, 12)
	trio := []PartyMember{mate(abelID, 1), mate(bethID, 1)}
	tests := []struct {
		name  string
		spots []datapack.Spot
		ms    []PartyMember
		run   func(t *testing.T, b *Brain)
	}{
		{
			name: "a party of 3 claims 3, skipping room for 2", spots: []datapack.Spot{two, three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				b.ruleSpot(at(time.Second))
				if !b.spot.held || b.spot.idx != 1 || b.spot.weight != 3 {
					t.Fatalf("spot %+v: want three held, weight 3", b.spot)
				}
				wantCounts(t, b, "claimed", 0, 0, 0)
				wantCounts(t, b, "claimed", 1, 3, 0)
				// A member's death or raise leaves the window as it is.
				dead := mate(bethID, 1)
				dead.HP = 0
				feed(t, b, 2*time.Second, PartySmallWindowUpdate{Member: dead})
				wantCounts(t, b, "a member's death", 1, 3, 0)
				feed(t, b, 3*time.Second, PartySmallWindowUpdate{Member: mate(bethID, 1)})
				wantCounts(t, b, "a member's raise", 1, 3, 0)
				feed(t, b, 4*time.Second, PartySmallWindowDelete{ObjectID: bethID, Name: "Beth"})
				wantCounts(t, b, "a member left", 1, 2, 0)
				if !b.spot.held || b.spot.idx != 1 || b.spot.weight != 2 {
					t.Fatalf("spot %+v: want three kept, weight 2", b.spot)
				}
			},
		},
		{
			name: "a player joining at a cap-3 spot moves the party to room for 4", spots: []datapack.Spot{three, four}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 3)
				feed(t, b, time.Second, PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: mate(talbotID, 40)})
				wantCounts(t, b, "a player joined", 0, 4, 0)
				b.ruleSpot(at(2 * time.Second))
				wantCounts(t, b, "moved", 0, 0, 0)
				wantCounts(t, b, "moved", 1, 4, 0)
			},
		},
		{
			name: "a player joining with no room for 4 keeps it over cap, no trip", spots: []datapack.Spot{three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 3)
				feed(t, b, time.Second, PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: mate(talbotID, 40)})
				b.ruleSpot(at(2 * time.Second))
				if !b.spot.held || b.trip.v.active() || !b.spot.retryAt.Equal(at(2*time.Second+spotRetry)) {
					t.Fatalf("spot %+v trip %v: want three kept, no trip, re-pick a minute later", b.spot, b.trip.v.active())
				}
				wantCounts(t, b, "kept over cap", 0, 4, 0)
			},
		},
		{
			name: "the leader's death and stop give back its whole weight", spots: []datapack.Spot{three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 3)
				b.diedAtSpot(at(time.Second))
				wantCounts(t, b, "died", 0, 0, 0)
				b.cfg.Spots.held[0] = 0
				spotAt(b, 0, false, 3)
				b.leaveSpot()
				wantCounts(t, b, "stopped", 0, 0, 0)
			},
		},
		{
			name: "every spot at cap: it takes one with room for 3 within its limit", spots: []datapack.Spot{two, three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				r.held[0], r.held[1] = 4, 3 // two: room for 2 within its limit 6; three: for 6 within 9
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "claimed over", 0, 4, 0)
				wantCounts(t, b, "claimed over", 1, 6, 0)
			},
		},
		{
			name: "every spot too near its limit: beside with its whole weight", spots: []datapack.Spot{two, three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				r.held[0], r.held[1] = 4, 7
				b.ruleSpot(at(time.Second))
				if !b.spot.beside || b.spot.weight != 3 {
					t.Fatalf("spot %+v: want beside, weight 3", b.spot)
				}
				i := b.spot.idx
				wantCounts(t, b, "beside", i, r.held[i], 3)
				feed(t, b, 2*time.Second, PartySmallWindowDelete{ObjectID: bethID, Name: "Beth"})
				wantCounts(t, b, "a member left", i, r.held[i], 2)
				r.held[1] = 1 // room for 2 at three
				b.ruleSpot(at(time.Second + spotRetry))
				if !b.spot.held || b.spot.idx != 1 {
					t.Fatalf("spot %+v: want three claimed", b.spot)
				}
				wantCounts(t, b, "claimed", 1, 3, 0)
				wantCounts(t, b, "claimed", 0, 4, 0)
				b.leaveSpot()
				wantCounts(t, b, "stopped", 1, 1, 0)
			},
		},
		{
			name: "a leader of 3 beside whose party disperses: beside 1", spots: []datapack.Spot{three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, true, 3)
				feed(t, b, time.Second, PartySmallWindowDeleteAll{})
				wantCounts(t, b, "dispersed", 0, 0, 1)
			},
		},
		{
			name: "a member logging out and invited again", spots: []datapack.Spot{three}, ms: []PartyMember{mate(abelID, 1)},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				feed(t, b, time.Second, PartySmallWindowDeleteAll{})
				wantCounts(t, b, "logged out", 0, 1, 0)
				if !b.spot.held || b.spot.idx != 0 {
					t.Fatalf("spot %+v: want kept", b.spot)
				}
				feed(t, b, 2*time.Second, leadWindow(mate(abelID, 1)))
				wantCounts(t, b, "invited again", 0, 2, 0)
			},
		},
		{
			name: "unparty: weight 1, a go to it keeps 1 and returns 1", spots: []datapack.Spot{three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 3)
				b.unparty()
				wantCounts(t, b, "unparty", 0, 1, 0)
				if err := b.goToSpot("three", at(time.Second)); err != nil {
					t.Fatal(err)
				}
				wantCounts(t, b, "go", 0, 1, 0)
				b.leaveSpot()
				wantCounts(t, b, "left", 0, 0, 0)
			},
		},
		{
			name: "a slot clearing: weight 1 at once; a go then takes 1", spots: []datapack.Spot{two, three}, ms: trio,
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 1, false, 3)
				b.clearSlot()
				wantCounts(t, b, "cleared", 1, 1, 0)
				if err := b.goToSpot("two", at(time.Second)); err != nil {
					t.Fatal(err)
				}
				wantCounts(t, b, "go", 0, 1, 0)
				wantCounts(t, b, "go", 1, 0, 0)
				b.leaveSpot()
				wantCounts(t, b, "left", 0, 0, 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeadSpotBrain(t, 1, tt.ms, tt.spots...)
			b.world.Objects = map[int32]*Object{}
			tt.run(t, b)
		})
	}
}

// TestPartyCrowdOwnWeight: the crowd check takes the bot's own weight off
// the spot's counts, so a leader's party is not strangers to itself, and a
// solo bot takes a party farming beside as its players and its weight
// alike (docs/parties/spots.md, "Crowd check").
func TestPartyCrowdOwnWeight(t *testing.T) {
	tests := []struct {
		name           string
		held, beside   int
		weight         int
		ownBeside      bool
		calm, crowding int
	}{
		{name: "solo, as today", held: 1, weight: 1, calm: 3, crowding: 4},
		{name: "leader of 3 alone", held: 3, weight: 3, calm: 3, crowding: 4},
		{name: "leader of 3 and another bot's claim", held: 4, weight: 3, calm: 4, crowding: 5},
		{name: "leader of 3 beside a spot one bot holds", held: 1, beside: 3, weight: 3, ownBeside: true, calm: 4, crowding: 5},
		{name: "solo where a party of 3 farms beside", held: 1, beside: 3, weight: 1, calm: 3 + 3, crowding: 3 + 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 2000, 0, 1, 3, 30))
			r := b.cfg.Spots
			r.held[0], r.beside[0] = tt.held, tt.beside
			b.spot.idx, b.spot.held, b.spot.beside, b.spot.weight = 0, !tt.ownBeside, tt.ownBeside, tt.weight
			b.world.Objects = map[int32]*Object{}
			players := func(n int) {
				for id := range int32(n) {
					b.world.Apply(CharInfo{ObjectID: 0x7000 + id, Name: "p", Pos: Point{X: 2000, Y: 50 * id}}, at(time.Second))
				}
			}
			players(tt.calm)
			if b.crowded(Point{X: 2000}, at(time.Second)) {
				t.Fatalf("%d players: crowded", tt.calm)
			}
			players(tt.crowding)
			if !b.crowded(Point{X: 2000}, at(time.Second)) {
				t.Fatalf("%d players: not crowded", tt.crowding)
			}
		})
	}
}

// TestSpotRegistryWeighted: the weighted forms take and give back n at
// once, claimN wants room for n under the cap (an empty spot first),
// claimOverN held plus n within the limit, and no count goes below 0.
func TestSpotRegistryWeighted(t *testing.T) {
	r := NewSpotRegistry([]datapack.Spot{testSpot("a", 0, 0, 1, 3, 6), testSpot("b", 0, 0, 1, 3, 9)}, 1)
	if i, ok := r.claimN([]int{0, 1}, 3); !ok || i != 1 || r.Held(1) != 3 {
		t.Fatalf("claimN 3 = %d, %v held %d: want b, 3 (a's cap is 2)", i, ok, r.Held(1))
	}
	if _, ok := r.claimN([]int{1}, 1); ok {
		t.Fatal("claimN past b's cap")
	}
	if i, ok := r.claimOverN([]int{0, 1}, 7); ok {
		t.Fatalf("claimOverN 7 = %d: want none (a's limit is 6, b's 9 with 3 held)", i)
	}
	if i, ok := r.claimOverN([]int{1}, 5); !ok || i != 1 || r.Held(1) != 8 {
		t.Fatalf("claimOverN 5 = %d, %v held %d: want b, 8", i, ok, r.Held(1))
	}
	if _, ok := r.claimOverN([]int{1}, 2); ok {
		t.Fatal("claimOverN past b's limit")
	}
	r.reweigh(1, false, 5, 2)
	r.releaseN(1, 2)
	if r.Held(1) != 3 {
		t.Fatalf("held %d after a re-weigh 5 to 2 and a release of 2: want 3", r.Held(1))
	}
	r.releaseN(1, 9)
	if i, ok := r.farmBesideN([]int{0}, 3); !ok || i != 0 || r.Beside(0) != 3 {
		t.Fatalf("farmBesideN 3 = %d, %v beside %d: want a, 3", i, ok, r.Beside(0))
	}
	r.reweigh(0, true, 3, 1)
	r.leaveBesideN(0, 4)
	if r.Held(1) != 0 || r.Beside(0) != 0 {
		t.Fatalf("held %d beside %d: want none below 0", r.Held(1), r.Beside(0))
	}
}
