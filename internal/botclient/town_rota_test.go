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
	b.world.Self.move = stopAt(town)
	a, ok := b.ruleSpot(farming)
	if !b.rota.farm || b.rota.until != farming.Add(farm) || !b.spot.held {
		t.Fatalf("farming spell: rota %+v, spot held %v; want farming until %v with the spot", b.rota, b.spot.held, farming.Add(farm))
	}
	if !ok || a.kind != actMove || !a.walk || b.walk.dest != b.spot.at {
		t.Fatalf("farming spell from town: ruleSpot %v action %d walk %v to %+v, want a walk to the spot %+v", ok, a.kind, a.walk, b.walk.dest, b.spot.at)
	}
	b.world.Self.move = stopAt(Point{})
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
			name:  "HopMax 0",
			setup: func(b *Brain) { b.persona.Town.HopMax = 0 },
			check: func(t *testing.T, b *Brain) {
				if b.rotates() || b.rotate(now) || b.townLife() {
					t.Fatal("a town bot with spots and HopMax 0 rotates or lives in town")
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

// TestTownRotationOrder: an order to a spot (goToSpot: the panel's go, a
// whispered go, a standing go at login) ends the town spell and the bot
// walks to the spot at once; one given with the farming spell over starts
// a new farming spell, so the bot is not sent home on its arrival.
func TestTownRotationOrder(t *testing.T) {
	const stay, farm = 10 * time.Minute, 30 * time.Minute
	tests := []struct {
		name string
		rota func(now time.Time) townRota
	}{
		{name: "town spell", rota: func(now time.Time) townRota { return townRota{until: now.Add(stay)} }},
		{name: "first decision not taken yet", rota: func(time.Time) townRota { return townRota{} }},
		{name: "farming spell over", rota: func(now time.Time) townRota { return townRota{farm: true, until: now.Add(-time.Minute)} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newTripBrain(t, soeStack(2))
			b.persona.Town = rotaPolicy(stay, farm)
			b.spot = spotVisit{}
			b.cfg.Spots.releaseN(0, 1)
			b.world.Self.move = stopAt(town)
			now := at(time.Minute)
			b.rota = tc.rota(now)

			if err := b.goToSpot("s", now); err != nil {
				t.Fatalf("goToSpot: %v", err)
			}
			if !b.rota.farm || b.rota.until != now.Add(farm) {
				t.Fatalf("rota %+v after the order, want the farming spell until %v", b.rota, now.Add(farm))
			}
			if b.farmOver(now) {
				t.Fatal("farming spell over after the order")
			}
			a, ok := b.ruleSpot(now)
			if !ok || a.kind != actMove || !a.walk || b.walk.dest != b.spot.at || !b.spot.held {
				t.Fatalf("ruleSpot %v action %d walk %v to %+v (held %v), want a walk to the spot %+v", ok, a.kind, a.walk, b.walk.dest, b.spot.held, b.spot.at)
			}
		})
	}
}

// TestTownRotationHop: a rotating town bot with a spot registry takes its
// hop when due in its town spell, and a town spell that starts with the hop
// left due by the farming spell (the bot home from its rotation trip) takes
// none at once: the idle spell to the hop starts over.
func TestTownRotationHop(t *testing.T) {
	now := at(time.Hour)
	tests := []struct {
		name    string
		setup   func(b *Brain)
		wantHop bool
	}{
		{
			name: "town spell, hop due",
			setup: func(b *Brain) {
				b.rota = townRota{until: now.Add(time.Minute)}
				b.trip.hopAt = now
			},
			wantHop: true,
		},
		{
			name: "town spell started home from the farming spell",
			setup: func(b *Brain) {
				b.rota = townRota{farm: true, until: now.Add(-time.Minute)}
				b.trip.hopAt = now.Add(-30 * time.Minute)
				if b.planHop(now) {
					t.Fatal("hop in the farming spell")
				}
				if _, ok := b.ruleSpot(now); ok || b.rota.farm {
					t.Fatalf("in town with the farming spell over: rota %+v, want the town spell", b.rota)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newTownBrain(t, 5000)
			b.persona.Town.StayMin, b.persona.Town.StayMax = 10*time.Minute, 10*time.Minute
			b.persona.Town.FarmMin, b.persona.Town.FarmMax = 30*time.Minute, 30*time.Minute
			b.cfg.Spots = NewSpotRegistry(nil, 1)
			tc.setup(b)
			next := now.Add(time.Second)
			if got := b.planHop(next); got != tc.wantHop {
				t.Fatalf("planHop = %v, want %v (hopAt %v)", got, tc.wantHop, b.trip.hopAt.Sub(t0))
			}
			if !tc.wantHop && b.trip.hopAt != next.Add(time.Minute) {
				t.Fatalf("hopAt %v, want the idle spell from %v", b.trip.hopAt.Sub(t0), next.Sub(t0))
			}
		})
	}
}
