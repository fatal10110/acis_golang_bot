package botclient

import (
	"testing"
	"time"
)

// rotaPolicy is a town policy whose spells are fixed: a town spell of
// stay, a farming spell of farm, and no hop within the test.
func rotaPolicy(stay, farm time.Duration) TownPolicy {
	return TownPolicy{HopMin: 24 * time.Hour, HopMax: 24 * time.Hour, StayMin: stay, StayMax: stay, FarmMin: farm, FarmMax: farm}
}

// TestTownRotation walks a town bot through one rotation (#618): its town
// spell claims no spot and drifts around the town; the farming spell claims
// the spot; the spell over at the spot sends it home on a supply trip
// (causeRotation) that releases the spot; landed in town, the next town
// spell starts.
func TestTownRotation(t *testing.T) {
	const stay, farm = 10 * time.Minute, 30 * time.Minute
	b := newTripBrain(t, soeStack(2))
	b.persona.Town = rotaPolicy(stay, farm)
	b.spot = spotVisit{}
	b.cfg.Spots.releaseN(0, 1)
	start := at(time.Second)

	if _, ok := b.ruleSpot(start); ok || b.spot.held || b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("town spell: ruleSpot %v, spot held %v (registry %d), want no spot", ok, b.spot.held, b.cfg.Spots.Held(0))
	}
	if b.rota.farm || b.rota.until != start.Add(stay) {
		t.Fatalf("rota %+v, want the town spell until %v", b.rota, start.Add(stay))
	}
	if c := b.idleCenter(b.world.Self.Position(start)); c != town {
		t.Fatalf("town spell drift centre %+v, want the town %+v", c, town)
	}

	farming := start.Add(stay)
	b.ruleSpot(farming)
	if !b.rota.farm || b.rota.until != farming.Add(farm) || !b.spot.held {
		t.Fatalf("farming spell: rota %+v, spot held %v; want farming until %v with the spot", b.rota, b.spot.held, farming.Add(farm))
	}
	if c := b.idleCenter(Point{X: 50}); c == town {
		t.Fatal("farming spell drift centre is the town, want the anchor")
	}
	b.spot.arrived = true
	if c := b.supplyDue(farming.Add(farm - time.Second)); c != causeNone {
		t.Fatalf("before the spell's end: cause %d, want none", c)
	}
	b.supplyTrip.checkAt = time.Time{}

	over := farming.Add(farm)
	if c := b.supplyDue(over); c != causeRotation {
		t.Fatalf("spell over at the spot: cause %d, want causeRotation", c)
	}
	b.supplyTrip.checkAt = time.Time{}
	if rule, a := decideSend(t, b, over); rule != "supply" || a.kind != actUseItem || a.item != scrollOfEscape {
		t.Fatalf("decide = rule %q action %d, want supply reading the scroll", rule, a.kind)
	}
	if b.spot.held || b.cfg.Spots.Held(0) != 0 {
		t.Fatalf("spot held %v, registry %d after the trip started: want it released", b.spot.held, b.cfg.Spots.Held(0))
	}
	b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2013, HitTime: 20 * time.Second}, over.Add(300*time.Millisecond))

	land := over.Add(21 * time.Second)
	b.world.Objects = map[int32]*Object{}
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: town}}, land); err != nil {
		t.Fatal(err)
	}
	b.prep = townPrep{}
	if _, ok := b.ruleSpot(land.Add(time.Second)); ok || b.spot.held {
		t.Fatalf("landed in town: ruleSpot %v, spot held %v, want no spot", ok, b.spot.held)
	}
	if b.rota.farm || b.rota.until != land.Add(time.Second+stay) {
		t.Fatalf("landed in town: rota %+v, want the next town spell until %v", b.rota, land.Add(time.Second+stay))
	}
}

// TestTownRotationSpellsHold: a farming spell over away from town and away
// from the spot (on the way to it) starts no town spell; a farming spell not
// over sends no bot home; a rotating bot takes no hop in its farming spell;
// without a spot registry or with StayMax 0 the bot does not rotate.
func TestTownRotationSpellsHold(t *testing.T) {
	now := at(time.Hour)
	tests := []struct {
		name  string
		setup func(b *Brain)
		check func(t *testing.T, b *Brain)
	}{
		{
			name: "farming spell over in the field",
			setup: func(b *Brain) {
				b.rota = townRota{farm: true, until: now.Add(-time.Minute)}
			},
			check: func(t *testing.T, b *Brain) {
				if !b.rotate(now) && b.rota.farm {
					return
				}
				t.Fatalf("rota %+v, want the farming spell kept until the bot is in town", b.rota)
			},
		},
		{
			name: "no hop in the farming spell",
			setup: func(b *Brain) {
				b.rota = townRota{farm: true, until: now.Add(time.Hour)}
				b.trip.hopAt = now.Add(-time.Minute)
			},
			check: func(t *testing.T, b *Brain) {
				if b.planHop(now) || b.trip.hopAt != now.Add(-time.Minute) {
					t.Fatalf("planHop in the farming spell, hopAt %v", b.trip.hopAt)
				}
			},
		},
		{
			name:  "no spot registry",
			setup: func(b *Brain) { b.cfg.Spots = nil },
			check: func(t *testing.T, b *Brain) {
				if b.rotates() || b.rotate(now) || !b.townLife() {
					t.Fatal("a town bot without spots rotates or leaves town life")
				}
			},
		},
		{
			name:  "StayMax 0",
			setup: func(b *Brain) { b.persona.Town.StayMax = 0 },
			check: func(t *testing.T, b *Brain) {
				if b.rotates() || b.townLife() {
					t.Fatal("a town bot with spots and StayMax 0 rotates or lives in town")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newTripBrain(t, soeStack(2))
			b.persona.Town = rotaPolicy(10*time.Minute, 30*time.Minute)
			tc.setup(b)
			tc.check(t, b)
		})
	}
}
