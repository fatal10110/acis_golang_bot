package botclient

import (
	"testing"
	"time"
)

// Fixture skills of the filler and Once tests: no datapack entry, so a
// self step goes out with no select.
const (
	fixFiller = int32(9401) // a charging self skill (GL's Sonic Focus)
	fixHit    = int32(9402) // an attack step
	fixDrain  = int32(9403) // an enemy sustain step
	fixGuard  = int32(9404) // a self step
	fixOnce   = int32(9405) // a Once step
)

// TestRotationFiller checks a filler (Step.Filler), listed ahead of the
// other steps: the attack goes while it passes, a self step and a sustain
// step go before it, and the filler goes on the bot only when none of them
// passes; never under MPReserve, and without using up the opener.
func TestRotationFiller(t *testing.T) {
	now := at(time.Second)
	kit := ClassKit{Rotation: []Step{
		{Skill: fixFiller, Target: TargetSelf, Filler: true},
		{Skill: fixHit},
		{Skill: fixDrain, SelfHPBelow: 50},
		{Skill: fixGuard, Target: TargetSelf, SelfHPBelow: 30},
	}}
	for _, tt := range []struct {
		name    string
		mp, hp  int32
		cooling bool
		skill   int32
		object  int32
	}{
		{"attack while it passes", 80, 100, false, fixHit, npcID},
		{"filler once the attack cools", 80, 100, true, fixFiller, selfID},
		{"sustain before the filler", 80, 40, true, fixDrain, npcID},
		{"self step before the filler", 80, 20, true, fixGuard, selfID},
		{"no filler under the reserve", 30, 100, true, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, kit, tt.mp, fixFiller, fixHit, fixDrain, fixGuard)
			b.world.Self.HP = tt.hp
			if tt.cooling {
				b.world.holdSkill(fixHit, now.Add(time.Minute))
			}
			a := b.fight(npcID, now)
			if tt.skill == 0 {
				if a.kind == actCast && a.skill == fixFiller {
					t.Fatal("the filler went out under the MP reserve")
				}
				return
			}
			wantCast(t, a, tt.skill, tt.object)
			if tt.skill != fixFiller {
				return
			}
			if a.opens {
				t.Error("the filler uses up the opener")
			}
			if ok, err := b.send(a, now); err != nil || !ok {
				t.Fatalf("send = %v %v", ok, err)
			}
			if b.opened == npcID {
				t.Error("the filler's cast opened the target")
			}
		})
	}
}

// TestPvPNoFiller checks the PvP order casts no filler against a player
// (docs/pvp/layer.md "PvP order"): the attack goes while it passes, and
// with it cooling the filler does not.
func TestPvPNoFiller(t *testing.T) {
	filler := Step{Skill: fixFiller, Target: TargetSelf, Filler: true}
	b := layerBrain(t, lyrWarrior, filler, Step{Skill: lyrNuke})
	if got := castOf(t, b, at(time.Second)); got != lyrNuke {
		t.Fatalf("cast %d, want the nuke %d", got, lyrNuke)
	}
	b = layerBrain(t, lyrWarrior, filler, Step{Skill: lyrNuke})
	b.world.holdSkill(lyrNuke, at(time.Hour))
	if got := castOf(t, b, at(time.Second)); got == fixFiller {
		t.Fatal("the filler went out against a player")
	}
}

// TestStepOnce checks a Once step (owner decision 4 on #624): it goes on a
// target another attack already went out on, and never again on it once
// the server took it; a refused cast, or one the server answers only after
// castTimeout, leaves it castable; a change of target forgets it, so it
// goes on the new target and on the first one again.
func TestStepOnce(t *testing.T) {
	now := at(time.Second)
	kit := ClassKit{Rotation: []Step{{Skill: fixOnce, Once: true}, {Skill: fixHit}}}
	// cast has the bot, an attack already out on npcID, send the Once
	// step's cast at now.
	cast := func(t *testing.T) *Brain {
		t.Helper()
		b := kitBrain(t, kit, 100, fixOnce, fixHit)
		b.opened = npcID
		a := b.fight(npcID, now)
		wantCast(t, a, fixOnce, npcID)
		if ok, err := b.send(a, now); err != nil || !ok {
			t.Fatalf("send = %v %v", ok, err)
		}
		return b
	}
	took := MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: fixOnce, Level: 1}
	later := now.Add(time.Minute)
	t.Run("once per target", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, took, now.Add(100*time.Millisecond))
		fightCast(t, b, now.Add(5*time.Second), fixHit, npcID)
		fightCast(t, b, later, fixHit, npcID)
	})
	t.Run("refused stays castable", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, SystemMessage{ID: msgCannotBeUsed}, now.Add(100*time.Millisecond))
		fightCast(t, b, later, fixOnce, npcID)
	})
	t.Run("answered after castTimeout stays castable", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, took, now.Add(castTimeout+time.Second))
		fightCast(t, b, later, fixOnce, npcID)
	})
	t.Run("selecting self keeps the record", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, took, now.Add(100*time.Millisecond))
		b.handleOne(t, MyTargetSelected{ObjectID: selfID}, now.Add(time.Second))
		b.handleOne(t, MyTargetSelected{ObjectID: npcID}, now.Add(2*time.Second))
		fightCast(t, b, later, fixHit, npcID)
	})
	t.Run("a new target starts afresh", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, took, now.Add(100*time.Millisecond))
		b.world.Apply(NPCInfo{ObjectID: rowNPC2, NPCID: 20001, Attackable: true, Pos: Point{X: -30}}, now)
		b.handleOne(t, MyTargetSelected{ObjectID: rowNPC2}, now.Add(time.Second))
		b.opened = rowNPC2
		wantCast(t, b.fight(rowNPC2, later), fixOnce, rowNPC2)
		// Back on the first target, the change forgot it as well.
		b.handleOne(t, MyTargetSelected{ObjectID: npcID}, later)
		wantCast(t, b.fight(npcID, later), fixOnce, npcID)
	})
}
