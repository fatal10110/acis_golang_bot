package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The fighter base rows (docs/strategy.md, "Behavior per class", rows HF EF
// DF OF WF; "Testing", TestRow<ID>): each test compiles the real row for
// its class and level and checks the row's key behavior.

// Object ids of the row tests: the bot's weapon, a second attacker.
const (
	rowWeapon   = int32(0x30000100)
	rowWeaponID = int32(9000)
	rowNPC2     = int32(0x20000004)
)

// rowBrain is a bot of the default persona of class at level, in combat
// with npcID next to it, knowing skills at level 1, wielding a weapon of
// datapack kind weapon ("" bare-handed), with its real compiled build.
func rowBrain(t *testing.T, class, level int32, weapon string, skills ...int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, _ := newTestBrain(t, p)
	s := &b.world.Self
	s.ClassID, s.Level, s.InCombat, s.RunSpeed = class, level, true, 200
	b.builtOK = false
	b.cfg.Travel = &TravelMap{}
	if weapon != "" {
		b.cfg.Travel.gear = map[int32]gearItem{rowWeaponID: {part: partRHand, kind: weapon, tmpl: datapack.Item{Kind: weapon}}}
		b.world.Apply(ItemList{Items: []Item{{ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
	}
	b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
	b.world.Target = npcID
	known := make([]KnownSkill, len(skills))
	for i, id := range skills {
		known[i] = KnownSkill{ID: id, Level: 1}
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	return b
}

// hitByTwo has npcID and a second monster beside it hit the bot.
func hitByTwo(b *Brain) {
	b.world.Apply(NPCInfo{ObjectID: rowNPC2, NPCID: 20001, Attackable: true, Pos: Point{X: -30}}, t0)
	for _, id := range []int32{npcID, rowNPC2} {
		b.world.Apply(Attack{AttackerID: id, Pos: b.world.Objects[id].Position(t0), Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
	}
}

// wantCast fails unless a is a cast of skill on object.
func wantCast(t *testing.T, a Action, skill, object int32) {
	t.Helper()
	if a.kind != actCast || a.skill != skill || a.object != object {
		t.Fatalf("action kind %d skill %d object %#x, want a cast of %d on %#x", a.kind, a.skill, a.object, skill, object)
	}
}

// TestRowHF checks the human fighter: with a sword Power Strike at its
// reuse and never Mortal Blow (melee while Power Strike cools), none before
// level 5; with a bow Power Shot opens; below SitHP out of combat it casts
// Relax instead of sitting, and sits plainly without the MP for it.
func TestRowHF(t *testing.T) {
	now := at(time.Second)
	all := []int32{skillPowerStrike, skillMortalBlow, skillPowerShot, skillRelax}
	for _, tt := range []struct {
		name   string
		level  int32
		weapon string
		setup  func(b *Brain)
		skill  int32 // 0: a melee click
	}{
		{"sword", 5, "SWORD", nil, skillPowerStrike},
		{"sword cooling", 5, "SWORD", func(b *Brain) { b.world.holdSkill(skillPowerStrike, now.Add(time.Minute)) }, 0},
		{"dagger", 5, "DAGGER", nil, skillMortalBlow},
		{"bow", 5, "BOW", nil, skillPowerShot},
		{"bow under 20 % MP", 5, "BOW", func(b *Brain) { b.world.Self.MP = 15 }, 0},
		{"before level 5", 4, "SWORD", nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, HumanFighter.ID, tt.level, tt.weapon, all...)
			if tt.setup != nil {
				tt.setup(b)
			}
			a := b.fight(npcID, now)
			if tt.skill == 0 {
				if a.kind != actAttack {
					t.Fatalf("fight = kind %d skill %d, want a melee click", a.kind, a.skill)
				}
				return
			}
			wantCast(t, a, tt.skill, npcID)
		})
	}
	for _, tt := range []struct {
		name string
		mp   int32
		want actionKind
	}{
		{"relax", 100, actCast},
		{"no MP for it", 10, actSitStand},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, HumanFighter.ID, 5, "SWORD", all...)
			s := &b.world.Self
			s.InCombat, s.HP, s.MP = false, 30, tt.mp
			rule, a := b.decide(now)
			if rule != "rest" || a.kind != tt.want {
				t.Fatalf("decide = %q kind %d, want rest kind %d", rule, a.kind, tt.want)
			}
			if tt.want == actCast {
				wantCast(t, a, skillRelax, selfID)
			}
		})
	}
}

// TestRowEF checks the elven fighter: under CriticalHP, Elemental Heal
// (level 15) goes out before the potion; cooling, the potion goes; under
// 50 % HP with one attacker fight casts it before any attack.
func TestRowEF(t *testing.T) {
	now := at(10 * time.Second)
	for _, tt := range []struct {
		name    string
		hp      int32
		cooling bool
		rule    string
		want    actionKind
	}{
		{"critical heals first", 20, false, "critical", actCast},
		{"critical heal cooling drinks", 20, true, "critical", actUseItem},
		{"under 50 % in the fight", 45, false, "attacked", actCast},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, ElvenFighter.ID, 15, "SWORD", skillElementalHeal, skillPowerStrike)
			b.world.Apply(ItemList{Items: []Item{potionsOf(20), {ObjectID: rowWeapon, TemplateID: rowWeaponID, Count: 1, Equipped: true, BodySlot: partRHand}}}, t0)
			b.world.Self.HP = tt.hp
			b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
			if tt.cooling {
				b.world.holdSkill(skillElementalHeal, now.Add(time.Minute))
			}
			rule, a := b.decide(now)
			if rule != tt.rule || a.kind != tt.want {
				t.Fatalf("decide = %q kind %d skill %d, want %q kind %d", rule, a.kind, a.skill, tt.rule, tt.want)
			}
			if tt.want == actCast {
				wantCast(t, a, skillElementalHeal, selfID)
			}
		})
	}
}

// TestRowDF checks the dark fighter: Drain Health (level 15) opens on a
// fresh target and not once an attack went out on it; with Drain Health on
// reuse and no potion, two attackers make it flee at 55 % HP (under its
// OutmatchedHP 60) where the human fighter fights on.
func TestRowDF(t *testing.T) {
	now := at(time.Second)
	b := rowBrain(t, DarkFighter.ID, 15, "", skillDrainHealth, skillPowerStrike)
	a := b.fight(npcID, now)
	wantCast(t, a, skillDrainHealth, npcID)
	if !a.opens {
		t.Error("the opener does not use the opener up")
	}
	b.opened = npcID
	if a := b.fight(npcID, now); a.kind != actAttack {
		t.Errorf("fight on an opened target = kind %d skill %d, want a melee click", a.kind, a.skill)
	}
	b.world.Self.HP = 55
	a = b.fight(npcID, now)
	wantCast(t, a, skillDrainHealth, npcID)
	if a.opens {
		t.Error("the sustain Drain Health uses the opener up")
	}

	// Seated when HP goes critical in combat: the sustain Drain Health
	// waits for the stand, as the server refuses a seated cast.
	b = rowBrain(t, DarkFighter.ID, 15, "", skillDrainHealth)
	b.world.Self.Sitting, b.world.Self.HP = true, 20
	if a, ok := b.sustain(now); !ok || a.kind != actSitStand {
		t.Errorf("seated sustain = %v kind %d skill %d, want a stand", ok, a.kind, a.skill)
	}

	for _, tt := range []struct {
		name  string
		class int32
		flee  bool
	}{
		{"dark fighter flees", DarkFighter.ID, true},
		{"human fighter fights on", HumanFighter.ID, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, tt.class, 15, "", skillDrainHealth)
			b.world.holdSkill(skillDrainHealth, now.Add(time.Minute))
			b.world.Self.HP = 55
			hitByTwo(b)
			onGround(t, b)
			rule, a := b.decide(now)
			if rule != "outnumbered" || (a.kind == actMove) != tt.flee {
				t.Fatalf("decide = %q kind %d, want outnumbered with a flee %v", rule, a.kind, tt.flee)
			}
		})
	}
}

// TestRowOF checks the orc fighter: Iron Punch only with a DUALFIST weapon
// (bare hands are FIST: never), Power Strike with a sword; with two
// attackers it fights on down to 25 % HP and flees under it.
func TestRowOF(t *testing.T) {
	now := at(time.Second)
	for _, tt := range []struct {
		weapon string
		skill  int32
	}{
		{"DUALFIST", skillIronPunch},
		{"", 0},
		{"SWORD", skillPowerStrike},
	} {
		t.Run("weapon "+tt.weapon, func(t *testing.T) {
			b := rowBrain(t, OrcFighter.ID, 5, tt.weapon, skillIronPunch, skillPowerStrike)
			a := b.fight(npcID, now)
			if tt.skill == 0 {
				if a.kind != actAttack {
					t.Fatalf("fight = kind %d skill %d, want a melee click", a.kind, a.skill)
				}
				return
			}
			wantCast(t, a, tt.skill, npcID)
		})
	}
	for _, tt := range []struct {
		hp   int32
		flee bool
	}{{26, false}, {24, true}} {
		b := rowBrain(t, OrcFighter.ID, 5, "")
		b.world.Self.HP = tt.hp
		hitByTwo(b)
		onGround(t, b)
		rule, a := b.decide(now)
		if rule != "outnumbered" || (a.kind == actMove) != tt.flee {
			t.Errorf("HP %d: decide = %q kind %d, want outnumbered with a flee %v", tt.hp, rule, a.kind, tt.flee)
		}
	}
}

// TestRowWF checks the dwarven fighter melees with no step (Spoil is S10)
// and plays its row's thresholds.
func TestRowWF(t *testing.T) {
	b := rowBrain(t, DwarvenFighter.ID, 10, "BLUNT")
	if a := b.fight(npcID, at(time.Second)); a.kind != actAttack {
		t.Fatalf("fight = kind %d skill %d, want a melee click", a.kind, a.skill)
	}
	if th := b.build().Thresholds; th != dwarvenFighterKit.Thresholds {
		t.Errorf("thresholds %+v, want the row's %+v", th, dwarvenFighterKit.Thresholds)
	}
}
