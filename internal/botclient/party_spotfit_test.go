package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPartySpotFitByLevels: a leader fits spots to its bot members' band,
// Below and AggroAbove against the lowest, Above against the highest, by
// its own level when no spot of the map fits the band; a player member
// counts for none (docs/parties/spots.md, "Fit by level").
func TestPartySpotFitByLevels(t *testing.T) {
	spots := []datapack.Spot{
		testSpot("top11", 1000, 0, 9, 11, 9), testSpot("top12", 0, 3000, 10, 12, 9), testSpot("top13", 3000, 3000, 11, 13, 9),
	}
	tests := []struct {
		name  string
		level int32
		ms    []PartyMember
		band  bool
		fit   []string
		claim string
	}{
		{name: "solo 10", level: 10, fit: []string{"top11", "top12"}, claim: "top11"},
		{name: "bots at 10 and 15", level: 10, ms: []PartyMember{mate(abelID, 15)}, band: true, fit: []string{"top12"}, claim: "top12"},
		{name: "bots at 15 and 10", level: 15, ms: []PartyMember{mate(abelID, 10)}, band: true, fit: []string{"top12"}, claim: "top12"},
		{name: "bots at 10 and 16: its own level", level: 10, ms: []PartyMember{mate(abelID, 16)}, fit: []string{"top11", "top12"}, claim: "top11"},
		{name: "a player at 30 changes nothing", level: 10, ms: []PartyMember{mate(abelID, 15), mate(talbotID, 30)}, band: true, fit: []string{"top12"}, claim: "top12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeadSpotBrain(t, tt.level, tt.ms, spots...)
			lo, hi, band := b.fitBand()
			var fit []string
			for i := range spots {
				if b.persona.Spots.fitsBand(&spots[i], lo, hi) {
					fit = append(fit, spots[i].Maker)
				}
			}
			if band != tt.band || !slices.Equal(fit, tt.fit) {
				t.Fatalf("band %v [%d, %d] fits %q, want band %v fitting %q", band, lo, hi, fit, tt.band, tt.fit)
			}
			b.ruleSpot(at(time.Second))
			if !b.spot.held || spots[b.spot.idx].Maker != tt.claim {
				t.Fatalf("spot %+v: want %s claimed", b.spot, tt.claim)
			}
		})
	}
}

// TestPartyHeldSpotMisfit: a leader holding a spot its bot members' band
// fails while a spot of the map fits it looks for one that does, as an
// outlevelled bot does, keeps it while every fitting one is too near its
// limit, and with none in reach gives it back (docs/parties/spots.md,
// "Misfit" and "No gatekeeper trip").
func TestPartyHeldSpotMisfit(t *testing.T) {
	s17, s13, s16 := testSpot("s17", 1000, 0, 15, 17, 9), testSpot("s13", 0, 3000, 11, 13, 9), testSpot("s16", 3000, 0, 14, 16, 9)
	s13far := testSpot("s13", travelFar, 0, 11, 13, 9)
	member12 := []PartyMember{mate(abelID, 12)}
	tests := []struct {
		name  string
		level int32
		ms    []PartyMember
		spots []datapack.Spot
		run   func(t *testing.T, b *Brain)
	}{
		{
			name: "claims a free band-fitting spot", level: 15, ms: member12, spots: []datapack.Spot{s17, s13},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "moved", 0, 0, 0)
				wantCounts(t, b, "moved", 1, 2, 0)
			},
		},
		{
			name: "claims it past its cap with room for 2 within its limit", level: 15, ms: member12, spots: []datapack.Spot{s17, s13},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				b.cfg.Spots.held[1] = 6 // cap 3, limit 9
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "moved", 0, 0, 0)
				wantCounts(t, b, "moved", 1, 8, 0)
			},
		},
		{
			name: "keeps its own with no spot of the map fitting the band", level: 15, ms: member12, spots: []datapack.Spot{s17, s16},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				if b.misfit() {
					t.Fatal("misfit with no spot fitting the band")
				}
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "kept", 0, 2, 0)
				wantCounts(t, b, "kept", 1, 0, 0)
			},
		},
		{
			name: "band-fitting spots only beyond reach: gives its spot back", level: 15, ms: member12, spots: []datapack.Spot{s17, s16, s13far},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				b.ruleSpot(at(time.Second))
				if b.spot.farming() {
					t.Fatalf("spot %+v: want none", b.spot)
				}
				for i := range 3 {
					wantCounts(t, b, "given back", i, 0, 0)
				}
			},
		},
		{
			name: "keeps a misfit spot while the fitting one is too near its limit", level: 15, ms: member12,
			spots: []datapack.Spot{testSpot("s15", 1000, 0, 13, 15, 9), testSpot("s13", 0, 3000, 11, 13, 3)},
			run: func(t *testing.T, b *Brain) {
				r := b.cfg.Spots
				spotAt(b, 0, false, 2)
				r.held[1] = 2 // cap 1, limit 3: no room for 2
				b.ruleSpot(at(time.Second))
				if !b.spot.held || b.spot.idx != 0 || !b.spot.retryAt.Equal(at(time.Second+spotRetry)) {
					t.Fatalf("spot %+v: want s15 kept, a re-pick after spotRetry", b.spot)
				}
				// The member reaching 13: band [12, 15], s15 fits.
				feed(t, b, 2*time.Second, PartySmallWindowUpdate{Member: mate(abelID, 13)})
				if b.misfit() {
					t.Fatal("misfit with the member at 13")
				}
				r.held[1] = 0
				b.ruleSpot(at(2*time.Second + spotRetry))
				wantCounts(t, b, "stays", 0, 2, 0)
				wantCounts(t, b, "stays", 1, 0, 0)
			},
		},
		{
			name: "a member 12 joining 14 and 15 at a top-16 spot", level: 15, ms: []PartyMember{mate(abelID, 14)}, spots: []datapack.Spot{s16, s13},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 2)
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "fits", 0, 2, 0)
				feed(t, b, 2*time.Second, PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: mate(bethID, 12)})
				wantCounts(t, b, "joined", 0, 3, 0)
				b.ruleSpot(at(2 * time.Second))
				wantCounts(t, b, "re-picked", 0, 0, 0)
				wantCounts(t, b, "re-picked", 1, 3, 0)
			},
		},
		{
			name: "a beside spot leaving the fit is left", level: 15, ms: []PartyMember{mate(abelID, 14)}, spots: []datapack.Spot{s16, s13},
			run: func(t *testing.T, b *Brain) {
				b.cfg.Spots.held[0] = 9
				spotAt(b, 0, true, 2)
				feed(t, b, time.Second, PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: mate(bethID, 12)})
				wantCounts(t, b, "joined", 0, 9, 3)
				b.ruleSpot(at(2 * time.Second))
				wantCounts(t, b, "left", 0, 9, 0)
				wantCounts(t, b, "left", 1, 3, 0)
			},
		},
		{
			name: "a solo held spot is kept", level: 15, spots: []datapack.Spot{s17, s13},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 0, false, 1)
				b.ruleSpot(at(time.Second))
				wantCounts(t, b, "kept", 0, 1, 0)
				wantCounts(t, b, "kept", 1, 0, 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeadSpotBrain(t, tt.level, tt.ms, tt.spots...)
			b.world.Objects = map[int32]*Object{}
			tt.run(t, b)
		})
	}
}

// TestPartyNoGatekeeperTrip: a leader with bot members plans no gatekeeper
// trip; an outlevelled one with nothing in reach fitting gives its spot
// back instead, and keeps it while a candidate is only too near its limit
// (docs/parties/spots.md, "No gatekeeper trip"). A solo bot is as today.
func TestPartyNoGatekeeperTrip(t *testing.T) {
	low, far, s15 := testSpot("low", 1000, 0, 1, 5, 9), testSpot("far", travelFar, 0, 18, 21, 9), testSpot("s15", 2000, 2000, 10, 15, 9)
	mid := testSpot("mid", 0, 3000, 18, 21, 3) // cap 1, limit 3
	trio := []PartyMember{mate(abelID, 21), mate(bethID, 21)}
	tests := []struct {
		name  string
		level int32
		ms    []PartyMember
		spots []datapack.Spot
		run   func(t *testing.T, b *Brain)
	}{
		{
			name: "alone it plans a trip", level: 20, spots: []datapack.Spot{low, far},
			run: func(t *testing.T, b *Brain) {
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() {
					t.Fatal("no trip")
				}
			},
		},
		{
			name: "with members it plans none", level: 20, ms: []PartyMember{mate(abelID, 20)}, spots: []datapack.Spot{low, far},
			run: func(t *testing.T, b *Brain) {
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want neither", b.trip.v.active(), b.spot)
				}
			},
		},
		{
			name: "outlevelled with nothing in reach: gives its spot back", level: 21, ms: trio, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 2, false, 3)
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want neither", b.trip.v.active(), b.spot)
				}
				for i := range 3 {
					wantCounts(t, b, "given back", i, 0, 0)
				}
				b.ruleSpot(at(time.Second + spotRetry))
				if b.spot.farming() {
					t.Fatalf("spot %+v: reclaimed", b.spot)
				}
			},
		},
		{
			name: "one candidate too near its limit: keeps its spot", level: 21, ms: trio, spots: []datapack.Spot{low, far, s15, mid},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 2, false, 3)
				b.cfg.Spots.held[3] = 1
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || !b.spot.held || b.spot.idx != 2 || !b.spot.retryAt.Equal(at(time.Second+spotRetry)) {
					t.Fatalf("trip %v spot %+v: want s15 kept, a re-pick after spotRetry", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "kept", 2, 3, 0)
			},
		},
		{
			name: "solo outlevelled: a trip and its give-back", level: 21, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				spotAt(b, 2, false, 1)
				b.ruleSpot(at(time.Second))
				if !b.trip.v.active() || b.spot.farming() {
					t.Fatalf("trip %v spot %+v: want a trip, the spot given back", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "given back", 2, 0, 0)
			},
		},
		{
			name: "solo outlevelled with no adena: no trip, kept", level: 21, spots: []datapack.Spot{low, far, s15},
			run: func(t *testing.T, b *Brain) {
				b.world.Inventory.Items = nil
				spotAt(b, 2, false, 1)
				b.ruleSpot(at(time.Second))
				if b.trip.v.active() || !b.spot.held || b.spot.idx != 2 {
					t.Fatalf("trip %v spot %+v: want s15 kept", b.trip.v.active(), b.spot)
				}
				wantCounts(t, b, "kept", 2, 1, 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeadSpotBrain(t, tt.level, tt.ms, tt.spots...)
			b.persona.Newbie.MaxLevel = 0
			b.cfg.Travel = testTravelMap()
			b.world.Inventory.Items = map[int32]Item{1: {ObjectID: 1, TemplateID: adenaID, Count: 5000}}
			b.world.Objects = map[int32]*Object{}
			tt.run(t, b)
		})
	}
}
