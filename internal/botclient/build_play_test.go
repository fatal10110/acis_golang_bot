package botclient

import (
	"testing"
	"time"
)

// withKit makes k the bot's compiled kit for its class, level and role, as
// build would have compiled it, with the support thresholds.
func withKit(b *Brain, k ClassKit) {
	s := &b.world.Self
	b.built = Build{Role: b.persona.Role, Kit: k, Thresholds: defaultThresholds}
	b.builtFor, b.builtOK = buildKey{s.ClassID, s.Level, b.persona.Role}, true
}

// kitBrain is a brain of the default persona fighting npcID next to it at
// mp % MP, knowing skills, with kit as its build.
func kitBrain(t *testing.T, kit ClassKit, mp int32, skills ...int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level, b.world.Self.MP, b.world.Self.MaxMP = classHumanMystic, 10, mp, 100
	b.world.Self.InCombat = true
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
	b.world.Target = npcID
	known := make([]KnownSkill, len(skills))
	for i, id := range skills {
		known[i] = KnownSkill{ID: id, Level: 1}
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	withKit(b, kit)
	return b
}

// TestFleeCastsKitFlee checks flee casts the kit's Flee skill on the bot
// first while it is ready, and runs without it while it cools down.
func TestFleeCastsKitFlee(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cooling bool
		cast    bool
	}{
		{"ready", false, true},
		{"cooling", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{Flee: skillSprint}, 100, skillSprint)
			now := at(time.Second)
			if tt.cooling {
				b.world.skillReady[skillSprint] = now.Add(time.Minute)
			}
			a := b.flee(npcID, now)
			if cast := a.kind == actCast && a.skill == skillSprint; cast != tt.cast {
				t.Fatalf("flee = kind %d skill %d object %d, want a Sprint cast %v", a.kind, a.skill, a.object, tt.cast)
			}
			if tt.cast && a.object != selfID {
				t.Errorf("Sprint cast on %d, want the bot %d", a.object, selfID)
			}
		})
	}
}

// TestFightMeleePolicy checks the kit's melee policy once no step passes: a
// caster melees only under its MP reserve, a healer only under it and out
// of a party, a fighter always.
func TestFightMeleePolicy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		melee MeleePolicy
		mp    int32
		party bool
		want  actionKind
	}{
		{"always above reserve", MeleeAlways, 80, false, actAttack},
		{"under reserve above it", MeleeUnderReserve, 80, false, 0},
		{"under reserve under it", MeleeUnderReserve, 20, false, actAttack},
		{"under reserve in a party", MeleeUnderReserve, 20, true, actAttack},
		{"alone above reserve", MeleeAloneUnderReserve, 80, false, 0},
		{"alone under reserve", MeleeAloneUnderReserve, 20, false, actAttack},
		{"alone under reserve in a party", MeleeAloneUnderReserve, 20, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{Melee: tt.melee}, tt.mp)
			if tt.party {
				b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID}}}, t0)
			}
			if a := b.fight(npcID, at(time.Second)); a.kind != tt.want {
				t.Errorf("fight = kind %d, want %d", a.kind, tt.want)
			}
		})
	}
}

// TestFightSkipsGatedStep checks fight skips a step whose gate fails and
// casts the next: a MinTargetHP step on a target seen under it (an unseen
// one counts as full), an Opener step on a target the bot already engaged.
func TestFightSkipsGatedStep(t *testing.T) {
	next := Step{Skill: skillLifeDrain}
	for _, tt := range []struct {
		name  string
		gated Step
		setup func(b *Brain)
		want  int32
	}{
		{"target hp unseen", Step{Skill: skillWindStrike, MinTargetHP: 50}, func(*Brain) {}, skillWindStrike},
		{"target hp above", Step{Skill: skillWindStrike, MinTargetHP: 50}, func(b *Brain) {
			b.world.Objects[npcID].HP, b.world.Objects[npcID].MaxHP = 60, 100
		}, skillWindStrike},
		{"target hp under", Step{Skill: skillWindStrike, MinTargetHP: 50}, func(b *Brain) {
			b.world.Objects[npcID].HP, b.world.Objects[npcID].MaxHP = 10, 100
		}, skillLifeDrain},
		{"opener on a fresh target", Step{Skill: skillWindStrike, Opener: true}, func(*Brain) {}, skillWindStrike},
		{"opener on an engaged target", Step{Skill: skillWindStrike, Opener: true}, func(b *Brain) { b.engaged = npcID }, skillLifeDrain},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := kitBrain(t, ClassKit{Rotation: []Step{tt.gated, next}, Melee: MeleeUnderReserve}, 80, skillWindStrike, skillLifeDrain)
			tt.setup(b)
			if a := b.fight(npcID, at(time.Second)); a.kind != actCast || a.skill != tt.want || a.object != npcID {
				t.Errorf("fight = kind %d skill %d object %d, want a cast of %d on the target", a.kind, a.skill, a.object, tt.want)
			}
		})
	}
}
