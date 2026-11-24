package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Skills of the stack rule tests (data/xml/skills): Frenzy and Guts share
// the pinch stack at one order; Might, Soul Shield, Chant of Shielding and
// Accuracy as named.
const (
	skillFrenzy         = int32(176)
	skillGuts           = int32(139)
	skillChantShielding = int32(1009)
	skillAccuracy       = int32(256)
)

// stackTravel is a travel map with the tests' stack effects: Frenzy and
// Guts pinch 1; Might pa_up 1 and 2 by level; Attack Aura pa_up 1; Soul
// Shield and Chant of Shielding pd_up at their levels.
func stackTravel() *TravelMap {
	return &TravelMap{skillStacks: map[int32]map[int32]stackEffect{
		skillFrenzy:         {1: {"pinch", 1}},
		skillGuts:           {1: {"pinch", 1}},
		skillMight:          {1: {"pa_up", 1}, 2: {"pa_up", 2}},
		skillAttackAura:     {1: {"pa_up", 1}},
		skillSoulShield:     {1: {"pd_up", 1}, 2: {"pd_up", 2}},
		skillChantShielding: {1: {"pd_up", 1}},
	}}
}

// TestBuffDue checks the stack rule: an equal-order partner (Frenzy under
// Guts) or a stronger effect is never cast over; a weaker one of another
// skill is upgraded; the skill's own effect is renewed only under
// RebuffBefore; with no stack data the old rule holds.
func TestBuffDue(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name    string
		skill   int32
		level   int32
		effects []Effect
		travel  bool
		want    bool
	}{
		{"nothing on", skillGuts, 1, nil, true, true},
		{"guts never over frenzy", skillGuts, 1, []Effect{{SkillID: skillFrenzy, Level: 1, Left: time.Second}}, true, false},
		{"own effect with time left", skillMight, 2, []Effect{{SkillID: skillMight, Level: 2, Left: time.Hour}}, true, false},
		{"own effect running out", skillMight, 2, []Effect{{SkillID: skillMight, Level: 2, Left: 10 * time.Second}}, true, true},
		{"own weaker level upgraded", skillMight, 2, []Effect{{SkillID: skillMight, Level: 1, Left: time.Hour}}, true, true},
		{"weaker other upgraded", skillMight, 2, []Effect{{SkillID: skillAttackAura, Level: 1, Left: time.Hour}}, true, true},
		{"stronger other kept", skillAttackAura, 1, []Effect{{SkillID: skillMight, Level: 2, Left: time.Second}}, true, false},
		{"equal other kept", skillAttackAura, 1, []Effect{{SkillID: skillMight, Level: 1, Left: time.Second}}, true, false},
		{"no stack data: other skill", skillGuts, 1, []Effect{{SkillID: skillFrenzy, Level: 1, Left: time.Second}}, false, true},
		{"no stack data: own effect", skillGuts, 1, []Effect{{SkillID: skillGuts, Level: 1, Left: time.Hour}}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, DefaultPersona())
			if tt.travel {
				b.cfg.Travel = stackTravel()
			}
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: tt.skill, Level: tt.level}}}, t0)
			b.world.Apply(AbnormalStatusUpdate{Effects: tt.effects}, now)
			if got := b.buffDue(selfID, tt.skill, now); got != tt.want {
				t.Errorf("buffDue = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFightSelfStepStackRule checks an in-combat self step goes out only
// under the stack rule: Guts not while Frenzy is on the bot.
func TestFightSelfStepStackRule(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name   string
		frenzy bool
		want   int32
	}{
		{"nothing on", false, skillGuts},
		{"frenzy on", true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{Rotation: []Step{{Skill: skillGuts, Target: TargetSelf}}}, 80, skillGuts)
			b.cfg.Travel = stackTravel()
			if tt.frenzy {
				b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillFrenzy, Level: 1, Left: time.Minute}}}, now)
			}
			a := b.fight(npcID, now)
			if tt.want == 0 {
				if a.kind != actAttack {
					t.Fatalf("fight = kind %d skill %d, want a melee click", a.kind, a.skill)
				}
				return
			}
			wantCast(t, a, tt.want, selfID)
		})
	}
}

// TestRebuffPartyStackRule checks a Party buff goes out while some patient
// within partyBuffReach qualifies: never while the bot and every member in
// reach hold a stronger effect of its stack type, once a member in reach
// holds none, but not for a member out of reach.
func TestRebuffPartyStackRule(t *testing.T) {
	now := at(time.Second)
	strong := []Effect{{SkillID: skillSoulShield, Level: 2, Left: time.Hour}}
	for _, tt := range []struct {
		name   string
		member []Effect
		x      int32
		cast   bool
	}{
		{"member holds a stronger effect", strong, 300, false},
		{"member in reach holds none", nil, 300, true},
		{"member out of reach holds none", nil, 2000, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			b, _ := newTestBrain(t, p)
			withKit(b, ClassKit{Buffs: []Buff{{Skill: skillChantShielding, Party: true}}})
			b.cfg.Travel = stackTravel()
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillChantShielding, Level: 1}}}, t0)
			b.world.Apply(AbnormalStatusUpdate{Effects: strong}, now)
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "mate", Pos: Point{X: tt.x}}, t0)
			b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pcID, HP: 100, MaxHP: 100}}}, t0)
			b.world.Apply(PartySpelled{Kind: partySpelledPlayer, ObjectID: pcID, Effects: slices.Clone(tt.member)}, now)
			if got := b.world.Party.Members[pcID].BuffLevels[skillSoulShield]; tt.member != nil && got != 2 {
				t.Fatalf("member's Soul Shield level %d, want 2", got)
			}
			a, ok := b.ruleRebuff(now)
			if ok != tt.cast {
				t.Fatalf("ruleRebuff = kind %d skill %d %v, want a cast %v", a.kind, a.skill, ok, tt.cast)
			}
			if tt.cast {
				wantCast(t, a, skillChantShielding, selfID)
			}
		})
	}
}

// TestCombatToggles checks a combat toggle goes on in a fight above its MP
// bound and off under it, stays as it is in between and while its last
// switch settles, and goes off out of combat (ruleRebuff) and before the
// bot sits (ruleRest), which then casts Relax.
func TestCombatToggles(t *testing.T) {
	now := at(10 * time.Second)
	kit := ClassKit{Toggles: []Toggle{{Skill: skillAccuracy, MPAbove: 30}}, Relax: skillRelax}
	on := AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillAccuracy, Level: 1, Permanent: true}}}
	for _, tt := range []struct {
		name     string
		mp       int32
		on       bool
		settling bool
		toggle   bool
	}{
		{"on at engage", 80, false, false, true},
		{"off under the bound", 20, true, false, true},
		{"stays on above", 80, true, false, false},
		{"stays off under", 20, false, false, false},
		{"settling", 80, false, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, kit, tt.mp, skillAccuracy, skillRelax)
			if tt.on {
				b.world.Apply(on, t0)
			}
			if tt.settling {
				b.skillToggledAt = map[int32]time.Time{skillAccuracy: now.Add(-time.Second)}
			}
			a := b.fight(npcID, now)
			if got := a.kind == actCast && a.skill == skillAccuracy; got != tt.toggle {
				t.Fatalf("fight = kind %d skill %d, want a toggle cast %v", a.kind, a.skill, tt.toggle)
			}
			if tt.toggle && a.object != selfID {
				t.Errorf("toggle cast on %#x, want the bot", a.object)
			}
		})
	}
	t.Run("off out of combat", func(t *testing.T) {
		b := kitBrain(t, kit, 80, skillAccuracy)
		b.world.Self.InCombat = false
		b.world.Apply(on, t0)
		a, ok := b.ruleRebuff(now)
		if !ok {
			t.Fatal("ruleRebuff left the toggle on")
		}
		wantCast(t, a, skillAccuracy, selfID)
	})
	t.Run("off before resting, then relax", func(t *testing.T) {
		b := kitBrain(t, kit, 80, skillAccuracy, skillRelax)
		b.world.Self.InCombat, b.world.Self.HP = false, 10
		b.world.Apply(on, t0)
		rule, a := b.decide(now)
		if rule != "rest" {
			t.Fatalf("decide = %q, want rest", rule)
		}
		wantCast(t, a, skillAccuracy, selfID)
		if sent, err := b.send(a, now); !sent || err != nil {
			t.Fatalf("send = %v, %v", sent, err)
		}
		b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skillAccuracy}, now)
		b.world.Apply(AbnormalStatusUpdate{}, now)
		b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skillAccuracy}, now)
		b.castAt = time.Time{}
		later := now.Add(3 * time.Second)
		rule, a = b.decide(later)
		if rule != "rest" {
			t.Fatalf("decide = %q, want rest", rule)
		}
		wantCast(t, a, skillRelax, selfID)
		if sent, err := b.send(a, later); !sent || err != nil {
			t.Fatalf("send = %v, %v", sent, err)
		}
		b.castAt = time.Time{}
		if rule, a := b.decide(later.Add(time.Second)); rule != "rest" || a.kind != 0 {
			t.Errorf("decide while Relax settles = %q kind %d, want rest holding", rule, a.kind)
		}
	})
}

// TestFightOpener checks an attack step's cast or a melee click uses up
// the target's opener when it goes out, and a self step's does not.
func TestFightOpener(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		name   string
		a      Action
		opened bool
	}{
		{"attack step", Action{kind: actCast, payload: encodeRequestMagicSkillUse(skillPowerStrike, false), object: npcID, skill: skillPowerStrike, opens: true}, true},
		{"sustain step", Action{kind: actCast, payload: encodeRequestMagicSkillUse(skillDrainHealth, false), object: npcID, skill: skillDrainHealth}, false},
		{"melee click", attackAction(npcID, Point{}), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{}, 80)
			if sent, err := b.send(tt.a, now); !sent || err != nil {
				t.Fatalf("send = %v, %v", sent, err)
			}
			if got := b.opened == npcID; got != tt.opened {
				t.Errorf("opened %v, want %v", got, tt.opened)
			}
		})
	}
	// The server gives a dead NPC's object id to its respawn: the opener
	// goes out again on it once the id died or left.
	for _, ev := range []Event{Die{ObjectID: npcID}, DeleteObject{ObjectID: npcID}} {
		b := kitBrain(t, ClassKit{}, 80)
		b.opened = npcID
		if err := b.handle([]Event{ev}, now); err != nil {
			t.Fatal(err)
		}
		if b.opened != 0 {
			t.Errorf("after %T the target is still opened", ev)
		}
	}
}

// TestStepCharges checks a Charges step passes only with the force charges
// the latest EtcStatusUpdate gave the bot.
func TestStepCharges(t *testing.T) {
	b := kitBrain(t, ClassKit{}, 80)
	step := Step{Skill: 54, Charges: 1}
	if step.passes(b.stepState(npcID, nil, at(time.Second))) {
		t.Fatal("a Charges step passes with no charges")
	}
	b.world.Apply(EtcStatusUpdate{Charges: 1}, t0)
	if !step.passes(b.stepState(npcID, nil, at(time.Second))) {
		t.Error("a Charges step does not pass with a charge")
	}
}

// TestIndexTrainers checks the trainers index: each class's trainers are
// the NPCs whose teachTo lists it, ascending, without a trainer whose page
// has no skill list.
func TestIndexTrainers(t *testing.T) {
	got := indexTrainers(map[int32]datapack.NPC{
		30022: {TeachTo: []int32{10, 15}},
		30010: {TeachTo: []int32{0, 10}},
		32098: {TeachTo: []int32{10}},
		30001: {},
	})
	for class, want := range map[int32][]int32{10: {30010, 30022}, 15: {30022}, 0: {30010}} {
		if !slices.Equal(got[class], want) {
			t.Errorf("class %d trainers %v, want %v", class, got[class], want)
		}
	}
	if len(got) != 3 {
		t.Errorf("index %v, want three classes", got)
	}
}
