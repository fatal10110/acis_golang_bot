package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The layer tests' mystic row cases (docs/pvp/layer.md "Tests", X8): the
// shipped CL and OM rows in the kite, control land rate and break-off
// cases.

// kiteCL is kiteBrain playing the shipped CL row at 30 in the Support
// role (a support bot fighting back), knowing its PvP and rotation skills
// with their datapack entries; a duel opponent shows 5000 HP, so no nuke
// counts as a finisher.
func kiteCL(t *testing.T, duel bool, to Point, towns, arenas []datapack.Zone) *Brain {
	t.Helper()
	b := kiteBrain(t, duel, to, towns, arenas)
	if duel {
		b.handleOne(t, ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID, HP: 5000, MaxHP: 5000}, at(9*time.Second))
	}
	m := b.cfg.Travel
	for id, s := range mysticPvPSkills {
		ref := datapack.SkillRef{ID: id, Level: 1}
		m.skills[ref] = s
		if s.CastRange > 0 {
			m.castRanges[ref] = s.CastRange
		}
	}
	s := &b.world.Self
	s.ClassID, s.Level = classCleric, 30
	b.persona.Role = RoleSupport
	b.builtOK = false
	k := b.build().Kit
	var known []KnownSkill
	for _, st := range slices.Concat(k.PvP, k.Rotation) {
		known = append(known, KnownSkill{ID: st.Skill, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	return b
}

// TestPvPKiteCL checks the kite on the CL row (docs/pvp/layer.md "Kite"),
// a support bot fighting back: its Dryad Root lands on a melee opponent,
// it walks to a stand point at 600 (less walkSlop), then casts Ice Bolt;
// in a duel the points on or within duelEdge of a town are skipped, the
// first passing one taken; none passing, Ice Bolt in place; a duel
// opponent 1900 away is the chase (Ice Bolt, whose cast the server walks
// to), no kite.
func TestPvPKiteCL(t *testing.T) {
	foe := Point{Y: 300}
	tests := []struct {
		name  string
		duel  bool
		towns []datapack.Zone
		want  Point // the kite's stand point; zero for none
	}{
		{name: "straight away", want: Point{Y: 300 - 584}},
		{name: "duel: the first point clear of a town", duel: true, towns: []datapack.Zone{kiteBox(-300, 300, -1000, -250)}, want: Point{X: 413, Y: -113}},
		{name: "duel: no point passes", duel: true, towns: []datapack.Zone{
			kiteBox(-5000, 5000, 750, 5000), kiteBox(-5000, 5000, -5000, -150), kiteBox(450, 5000, -5000, 5000), kiteBox(-5000, -450, -5000, 5000),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := kiteCL(t, tc.duel, foe, tc.towns, nil)
			rootLands(t, b, foe)
			now := at(11*time.Second + 500*time.Millisecond)
			_, a := b.decide(now)
			if tc.want == (Point{}) {
				if a.kind != actCast || a.skill != skillIceBolt {
					t.Fatalf("kind %v skill %d, want Ice Bolt in place", a.kind, a.skill)
				}
				return
			}
			if got := moveDest(t, a); got != tc.want {
				t.Fatalf("kite to %v, want %v", got, tc.want)
			}
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
			b.world.Self.move = stopAt(tc.want)
			if got := castOf(t, b, at(13*time.Second)); got != skillIceBolt {
				t.Fatalf("cast %d at the stand point, want Ice Bolt %d", got, skillIceBolt)
			}
		})
	}
	t.Run("duel opponent 1900 away", func(t *testing.T) {
		far := Point{Y: 1900}
		b := kiteCL(t, true, far, nil, nil)
		b.layerCast(selfCast(skillDryadRoot, time.Second), at(10*time.Second))
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: far, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: rootAbnormal, CollisionRadius: 9}, at(11*time.Second+200*time.Millisecond))
		if _, ok := b.kiting(pcID, at(12*time.Second)); !ok {
			t.Fatal("the root does not count for the kite")
		}
		_, a := b.decide(at(12 * time.Second))
		if a.kind == actMove {
			t.Fatalf("kite move to %v, want the chase", moveDest(t, a))
		}
		if a.kind != actCast || a.skill != skillIceBolt {
			t.Fatalf("kind %v skill %d, want Ice Bolt, whose cast the server walks to", a.kind, a.skill)
		}
	})
}

// TestPvPControlLandRateCL checks the CL row's Dryad Root under the
// resist rule (docs/pvp/layer.md "PvP order"): after two 139s on it this
// fight no third, Ice Bolt instead.
func TestPvPControlLandRateCL(t *testing.T) {
	b := pvpRowBrain(t, classCleric, 30, lyrWarrior, false)
	resist := SystemMessage{ID: msgResisted}
	play(t, b, []beat{
		{at: 0, kind: actCast, skill: skillDryadRoot},
		{at: 100 * time.Millisecond, ev: selfCast(skillDryadRoot, 2500*time.Millisecond), noDecide: true},
		{at: 2600 * time.Millisecond, ev: resist, noDecide: true},
		{at: 5 * time.Second, kind: actCast, skill: skillDryadRoot},
		{at: 5100 * time.Millisecond, ev: selfCast(skillDryadRoot, 2500*time.Millisecond), noDecide: true},
		{at: 7600 * time.Millisecond, ev: resist, noDecide: true},
	})
	if got := castOf(t, b, at(10*time.Second)); got != skillIceBolt {
		t.Fatalf("cast %d after two resists, want Ice Bolt %d, no third Dryad Root", got, skillIceBolt)
	}
}

// TestPvPBreakOffOM checks the OM row's break-off (docs/pvp/layer.md
// "Break off", rows.md OM): a melee opponent that would break Dreaming
// Spirit (hit 4000) more often than not → no Break step, the flee at once;
// Venom on an opponent that cannot reach it first → 1092 Fear (the sleep
// rule); below 14, no Fear: the flee at once.
func TestPvPBreakOffOM(t *testing.T) {
	poisoned := foeSeen(lyrWarrior, Point{Y: 900}, 0x2)
	tests := []struct {
		name  string
		level int32
		setup func(t *testing.T, b *Brain)
		skill int32 // the cast; 0 for the flee
	}{
		{name: "a melee opponent next to it: the flee", level: 15},
		{name: "Venom on the opponent: Fear", level: 15, setup: poisoned, skill: skillFear},
		{name: "Venom on the opponent, below 14: the flee", level: 12, setup: poisoned},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, classOrcMystic, tc.level, lyrWarrior, false)
			breaking(t, b)
			hold(b, rowNow, skillLifeDrain)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			bot, opp, ok := b.pvpFighters(pcID)
			if !ok {
				t.Fatal("no fighters")
			}
			if tc.setup == nil {
				if odds := charmodel.CastBreakOdds(opp, bot, b.hitTime(skillDreamingSpirit)-escLead); odds < breakOdds {
					t.Fatalf("the Warrior breaks Dreaming Spirit at %.1f %%, the case needs %d or more", odds, breakOdds)
				}
			}
			_, a := b.decide(rowNow)
			switch {
			case tc.skill == 0 && a.kind != actMove:
				t.Fatalf("action %v skill %d, want the flee", a.kind, a.skill)
			case tc.skill != 0 && (a.kind != actCast || a.skill != tc.skill):
				t.Fatalf("action %v skill %d, want a cast of %d", a.kind, a.skill, tc.skill)
			}
		})
	}
}

// TestPvPRotation checks pvpRotation: the PvP row's steps first, the M10
// steps of a skill it lists left out but for a side cast on another
// attacker, the other M10 steps kept in order.
func TestPvPRotation(t *testing.T) {
	pvp := []Step{{Skill: skillWindShackle, Vs: mysticVsM}, {Skill: skillDryadRoot, From: 25, Vs: mysticVsM}}
	rot := []Step{
		{Skill: skillSelfHeal, Target: TargetSelf, SelfHPBelow: 50},
		{Skill: skillWindShackle, Opener: true, MPAbove: 50},
		{Skill: skillDryadRoot, Target: TargetOtherAttacker, Attackers: 2},
		{Skill: skillWindStrike, Target: TargetLeaderTarget},
	}
	got := pvpRotation(pvp, rot)
	want := []int32{skillWindShackle, skillDryadRoot, skillSelfHeal, skillDryadRoot, skillWindStrike}
	if len(got) != len(want) {
		t.Fatalf("%d steps %+v, want %v", len(got), got, want)
	}
	for i, st := range got {
		if st.Skill != want[i] {
			t.Errorf("step %d: skill %d, want %d", i, st.Skill, want[i])
		}
	}
	if got[3].Target != TargetOtherAttacker {
		t.Errorf("step 3 targets %v, want the side cast", got[3].Target)
	}
	if len(pvp) != 2 {
		t.Errorf("the PvP row grew to %d steps", len(pvp))
	}
	if got := pvpRotation(nil, rot); len(got) != len(rot) {
		t.Errorf("no PvP row: %d steps, want the M10 row's %d", len(got), len(rot))
	}
}

// TestPvPAuraCastAt checks that Seal of Binding, an AURA root whose own
// MagicSkillUse names the bot (server handler/target/target_handlers_area.go
// auraHandler.FinalTarget), counts at the opponent the bot selected: its
// landing and root are kept there, the root bit after the landing holds
// it, and a 139 after it is a resist of Seal.
func TestPvPAuraCastAt(t *testing.T) {
	seal := MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skillSealBinding, Level: 1, HitTime: 4 * time.Second}
	lands := rowNow.Add(seal.HitTime)
	cast := func(t *testing.T) *Brain {
		t.Helper()
		b := pvpRowBrain(t, classOrcShaman, 30, lyrWarrior, false)
		b.handleOne(t, MyTargetSelected{ObjectID: pcID}, rowNow)
		b.handleOne(t, seal, rowNow)
		if _, ok := b.pvp.opponents[pcID].fight.landed[skillSealBinding]; !ok {
			t.Fatal("no Seal of Binding landing on the opponent")
		}
		if b.pvp.root.target != pcID {
			t.Fatalf("root cast at %d, want the opponent %d", b.pvp.root.target, pcID)
		}
		return b
	}
	t.Run("root holds", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, rowFoe(lyrWarrior, false, Point{Y: 300}, rootAbnormal), lands.Add(500*time.Millisecond))
		if got := b.rootHold(pcID, lands.Add(time.Second)); got != 29*time.Second {
			t.Fatalf("root hold %v, want 29s", got)
		}
	})
	t.Run("139 is a resist", func(t *testing.T) {
		b := cast(t)
		b.handleOne(t, SystemMessage{ID: msgResisted}, lands.Add(100*time.Millisecond))
		f := b.pvp.opponents[pcID].fight
		if f.resists[skillSealBinding] != 1 {
			t.Fatalf("Seal resists %d, want 1", f.resists[skillSealBinding])
		}
		if _, ok := f.landed[skillSealBinding]; ok {
			t.Fatal("a resisted Seal still landed")
		}
	})
}
