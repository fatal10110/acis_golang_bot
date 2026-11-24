package botclient

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// compileFor is p's build for class at level, with p's overrides, as the
// brain plays it.
func compileFor(p Persona, class, level int32) Build {
	race, _ := datapack.ClassRace(class)
	return p.override(CompileBuild(race, class, level, p.Role), class)
}

func stepSkills(steps []Step) []int32 {
	var ids []int32
	for _, st := range steps {
		ids = append(ids, st.Skill)
	}
	return ids
}

// TestCompileBuild compiles every starting class and one profession for
// every role: a mystic line keeps the part of the support table it can
// know (D24), the orc mystic's spells from level 7, with the support
// thresholds; a fighter line its base row's steps from the level reached,
// buffs, Relax, skills and thresholds (S6); every class its gear entry and
// dyes and its race's professions only, and no trainers (they come from the
// travel map). RoleNone compiles nothing.
func TestCompileBuild(t *testing.T) {
	if bd := CompileBuild(RaceHuman, classHumanMystic, 20, RoleNone); !reflect.DeepEqual(bd, Build{}) {
		t.Errorf("RoleNone build = %+v, want the zero Build", bd)
	}
	caster := []int32{skillWindStrike}
	attacks := []int32{skillPowerShot, skillMortalBlow, skillPowerStrike}
	tests := []struct {
		class, level int32
		rotation     []int32
		heals, buffs int
		kit          ClassKit
	}{
		{class: HumanFighter.ID, level: 1, kit: humanFighterKit},
		{class: HumanFighter.ID, level: 5, rotation: attacks, kit: humanFighterKit},
		{class: HumanMystic.ID, level: 1, rotation: caster, heals: 2, buffs: 2},
		{class: ElvenFighter.ID, level: 10, rotation: attacks, buffs: 2, kit: elvenFighterKit},
		{class: ElvenFighter.ID, level: 15, rotation: append([]int32{skillElementalHeal}, attacks...), buffs: 2, kit: elvenFighterKit},
		{class: ElvenMystic.ID, level: 10, rotation: caster, heals: 2, buffs: 2},
		{class: DarkFighter.ID, level: 10, rotation: attacks, buffs: 2, kit: darkFighterKit},
		{class: DarkFighter.ID, level: 15, rotation: append([]int32{skillDrainHealth, skillDrainHealth}, attacks...), buffs: 2, kit: darkFighterKit},
		{class: DarkMystic.ID, level: 10, rotation: caster, heals: 2, buffs: 2},
		{class: OrcFighter.ID, level: 10, rotation: []int32{skillIronPunch, skillPowerStrike}, kit: orcFighterKit},
		{class: OrcMystic.ID, level: 6, rotation: nil, heals: 0, buffs: 2},
		{class: OrcMystic.ID, level: 7, rotation: []int32{skillChillFlame, skillLifeDrain}, heals: 0, buffs: 2},
		{class: DwarvenFighter.ID, level: 9, kit: dwarvenFighterKit},
		{class: DwarvenFighter.ID, level: 10, rotation: []int32{skillSpoil}, kit: dwarvenFighterKit},
		{class: classShillienOracle, level: 25, rotation: caster, heals: 2, buffs: 2},
		{class: classWarcryer, level: 45, rotation: []int32{skillChillFlame, skillLifeDrain}, heals: 0, buffs: 2},
	}
	for _, tt := range tests {
		race, _ := datapack.ClassRace(tt.class)
		for _, role := range []Role{RoleDamage, RoleSupport, RoleCrafter} {
			bd := CompileBuild(race, tt.class, tt.level, role)
			k := bd.Kit
			wantTh, wantSkills := defaultThresholds, mysticSkills[tt.class]
			if tt.kit.Thresholds != (Thresholds{}) {
				wantTh, wantSkills = tt.kit.Thresholds, tt.kit.Skills
			}
			if bd.Role != role || bd.Branch != 0 || bd.Thresholds != wantTh || k.Relax != tt.kit.Relax || k.Sweep != tt.kit.Sweep {
				t.Errorf("class %d %v: role %v branch %d thresholds %+v", tt.class, role, bd.Role, bd.Branch, bd.Thresholds)
			}
			if got := stepSkills(k.Rotation); !slices.Equal(got, tt.rotation) || len(k.Heals) != tt.heals || len(k.Buffs) != tt.buffs {
				t.Errorf("class %d level %d %v: rotation %v heals %d buffs %d, want %v %d %d", tt.class, tt.level, role, got, len(k.Heals), len(k.Buffs), tt.rotation, tt.heals, tt.buffs)
			}
			if !slices.Equal(k.Skills, wantSkills) || k.Trainers != nil ||
				!reflect.DeepEqual(k.Gear, gearClassOf(defaultGearClasses(), tt.class)) || !slices.Equal(k.Dyes, defaultDyeTable()[tt.class]) ||
				k.Flee != 0 || k.Melee != MeleeAlways {
				t.Errorf("class %d %v: kit %+v, want the class's tables", tt.class, role, k)
			}
			if bd.Race.Race != race || bd.Race.Professions[tt.class] != supportBuilds[tt.class] {
				t.Errorf("class %d %v: race %d professions %v", tt.class, role, bd.Race.Race, bd.Race.Professions)
			}
			for from := range bd.Race.Professions {
				if r, _ := datapack.ClassRace(from); r != race {
					t.Errorf("class %d: race %d profile holds class %d of race %d", tt.class, race, from, r)
				}
			}
		}
	}
}

// TestCompileBuildStepRole checks a step marked for a role is compiled into
// that role's rotation only.
func TestCompileBuildStepRole(t *testing.T) {
	saved := lineKits[classHumanMystic]
	t.Cleanup(func() { lineKits[classHumanMystic] = saved })
	lineKits[classHumanMystic] = ClassKit{Rotation: []Step{{Skill: 1, Role: RoleDamage}, {Skill: 2}, {Skill: 3, Role: RoleSupport}, {Skill: 4, From: 14}}}
	for _, tt := range []struct {
		role  Role
		level int32
		want  []int32
	}{
		{RoleDamage, 13, []int32{1, 2}},
		{RoleSupport, 14, []int32{2, 3, 4}},
		{RoleCrafter, 20, []int32{2, 4}},
	} {
		if got := stepSkills(CompileBuild(RaceHuman, classHumanMystic, tt.level, tt.role).Kit.Rotation); !slices.Equal(got, tt.want) {
			t.Errorf("%v at %d: rotation %v, want %v", tt.role, tt.level, got, tt.want)
		}
	}
}

// TestBuildRecompiles checks the brain compiles its build at entry and again
// on a UserInfo that changed its class or level.
func TestBuildRecompiles(t *testing.T) {
	b, _ := newTestBrain(t, DefaultPersona())
	b.builtOK = false // the entry compile, not newTestBrain's fixture kit
	if got := b.build(); got.Role != RoleSupport || got.Race.Professions[HumanFighter.ID] != supportBuilds[HumanFighter.ID] {
		t.Fatalf("entry build = %+v, want the human fighter's support build", got)
	}
	steps := []struct {
		class, level int32
		rotation     []int32
		next         int32
	}{
		{class: classOrcMystic, level: 6, rotation: nil, next: classOrcShaman},
		{class: classOrcMystic, level: 7, rotation: []int32{skillChillFlame, skillLifeDrain}, next: classOrcShaman},
		{class: classOrcShaman, level: 20, rotation: []int32{skillChillFlame, skillLifeDrain}, next: classWarcryer},
	}
	for _, st := range steps {
		b.world.Apply(UserInfo{ObjectID: selfID, Name: "bot7", ClassID: st.class, Level: st.level, RunSpeed: 100, WalkSpeed: 50}, t0)
		bd := b.build()
		if got := stepSkills(bd.Kit.Rotation); !slices.Equal(got, st.rotation) || bd.Race.Professions[st.class] != st.next ||
			!slices.Equal(bd.Kit.Skills, mysticSkills[st.class]) || bd.Race.Race != RaceOrc {
			t.Errorf("class %d level %d: rotation %v next %d skills %v race %d, want %v %d", st.class, st.level, got, bd.Race.Professions[st.class], bd.Kit.Skills, bd.Race.Race, st.rotation, st.next)
		}
	}
	b.persona.Role = RoleNone
	if got := b.build(); !reflect.DeepEqual(got, Build{}) {
		t.Errorf("build after the role is cleared = %+v, want the zero Build", got)
	}
}

// TestOverrides checks every persona table and threshold the build replaced
// wins over it, a table for the bot's class only, and a negative threshold
// turns the build's off.
func TestOverrides(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(p *Persona)
		class int32
		check func(bd Build) bool
	}{
		{"none", func(*Persona) {}, classCleric, func(bd Build) bool {
			return reflect.DeepEqual(bd, CompileBuild(RaceHuman, classCleric, 20, RoleSupport))
		}},
		{"learn skills", func(p *Persona) { p.LearnSkills = map[int32][]int32{classCleric: {7}} }, classCleric, func(bd Build) bool { return slices.Equal(bd.Kit.Skills, []int32{7}) }},
		{"learn skills of another class", func(p *Persona) { p.LearnSkills = map[int32][]int32{classBishop: {7}} }, classCleric, func(bd Build) bool { return bd.Kit.Skills == nil }},
		{"trainers", func(p *Persona) { p.Trainers = map[int32][]int32{classCleric: {30022}} }, classCleric, func(bd Build) bool { return slices.Equal(bd.Kit.Trainers, []int32{30022}) }},
		{"attack skills", func(p *Persona) { p.AttackSkills = []int32{skillLifeDrain} }, classCleric, func(bd Build) bool { return slices.Equal(stepSkills(bd.Kit.Rotation), []int32{skillLifeDrain}) }},
		{"no attack skills", func(p *Persona) { p.AttackSkills = []int32{} }, classCleric, func(bd Build) bool { return len(bd.Kit.Rotation) == 0 }},
		{"heals", func(p *Persona) { p.Heals = []Heal{{Skill: skillHeal, HP: 90}} }, classCleric, func(bd Build) bool { return reflect.DeepEqual(bd.Kit.Heals, []Heal{{Skill: skillHeal, HP: 90}}) }},
		{"buffs", func(p *Persona) { p.Buffs = []Buff{} }, classCleric, func(bd Build) bool { return bd.Kit.Buffs != nil && len(bd.Kit.Buffs) == 0 }},
		{"class change builds", func(p *Persona) { p.ClassChange.Builds = map[int32]int32{classCleric: 17} }, classCleric, func(bd Build) bool { return bd.Race.Professions[classCleric] == 17 }},
		{"dyes", func(p *Persona) { p.Dyes.Symbols = map[int32][]int32{classCleric: {8}} }, classCleric, func(bd Build) bool { return slices.Equal(bd.Kit.Dyes, []int32{8}) }},
		{"gear classes", func(p *Persona) { p.Gear.Classes = []GearClass{{Armor: "HEAVY"}} }, classCleric, func(bd Build) bool { return bd.Kit.Gear.Armor == "HEAVY" }},
		{"threshold", func(p *Persona) { p.MPReserve, p.SitHP = 70, 10 }, classCleric, func(bd Build) bool {
			want := defaultThresholds
			want.MPReserve, want.SitHP = 70, 10
			return bd.Thresholds == want
		}},
		{"threshold off", func(p *Persona) { p.CriticalHP = -1 }, classCleric, func(bd Build) bool {
			return bd.Thresholds.CriticalHP == -1 && !below(1, 100, bd.Thresholds.CriticalHP) && bd.Thresholds.CrowdedHP == defaultThresholds.CrowdedHP
		}},
		{"no role: the persona's own only", func(p *Persona) { *p = Persona{AttackSkills: []int32{skillWindStrike}, SitHP: 40} }, classCleric, func(bd Build) bool {
			return reflect.DeepEqual(bd, Build{Kit: ClassKit{Rotation: []Step{{Skill: skillWindStrike}}}, Thresholds: Thresholds{SitHP: 40}})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			tt.edit(&p)
			if bd := compileFor(p, tt.class, 20); !tt.check(bd) {
				t.Errorf("build = %+v", bd)
			}
		})
	}
	// The crafter keeps Create Item learning and its Artisan line by
	// overrides over its Crafter build.
	crafter := CrafterPersona()
	bd := compileFor(crafter, DwarvenFighter.ID, 20)
	if bd.Role != RoleCrafter || bd.Race.Professions[DwarvenFighter.ID] != classArtisan || !slices.Equal(bd.Kit.Skills, []int32{skillCreateItem}) || len(bd.Kit.Trainers) == 0 {
		t.Errorf("crafter build = %+v, want the Crafter role, Artisan next and Create Item", bd)
	}
}

// TestOverridesInPlay checks the brain plays an override over the build: a
// human mystic of the default persona at 80 % MP casts Wind Strike above
// the build's MP reserve, and melees under a persona's own higher one.
func TestOverridesInPlay(t *testing.T) {
	for _, tt := range []struct {
		name    string
		reserve int
		want    actionKind
	}{
		{"build reserve", 0, actCast},
		{"persona reserve wins", 90, actAttack},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance, p.MPReserve = 0, tt.reserve
			b, _ := newTestBrain(t, p)
			b.world.Self.ClassID, b.world.Self.Level, b.world.Self.MP = classHumanMystic, 10, 80
			b.world.Self.InCombat = true
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
			b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
			b.world.Target = npcID
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillWindStrike, Level: 1}}}, t0)
			if rule, a := b.decide(at(time.Second)); rule != "attacked" || a.kind != tt.want {
				t.Errorf("decide = rule %q action %d, want attacked %d", rule, a.kind, tt.want)
			}
		})
	}
}

// TestStepPasses checks each rotation gate: zero is no gate, an unseen
// target's HP counts as full.
func TestStepPasses(t *testing.T) {
	base := stepState{weapon: "SWORD", hp: 50, maxHP: 100, mp: 50, maxMP: 100, targetHP: 1, targetMaxHP: 1, dist: 300, attackers: 1, inCombat: true}
	with := func(f func(*stepState)) stepState { st := base; f(&st); return st }
	tests := []struct {
		name string
		step Step
		st   stepState
		want bool
	}{
		{"no gate", Step{Skill: 1}, base, true},
		{"weapon worn", Step{Weapons: []string{"BLUNT", "SWORD"}}, base, true},
		{"other weapon", Step{Weapons: []string{"DAGGER"}}, base, false},
		{"unknown weapon", Step{Weapons: []string{"SWORD"}}, with(func(s *stepState) { s.weapon = "" }), false},
		{"shield worn", Step{Weapons: []string{"SHIELD"}}, with(func(s *stepState) { s.shield = true }), true},
		{"no shield", Step{Weapons: []string{"SHIELD"}}, base, false},
		{"unseen target is full", Step{MinTargetHP: 100}, base, true},
		{"target HP at the bound", Step{MinTargetHP: 50}, with(func(s *stepState) { s.targetHP, s.targetMaxHP = 50, 100 }), true},
		{"target HP under", Step{MinTargetHP: 50}, with(func(s *stepState) { s.targetHP, s.targetMaxHP = 49, 100 }), false},
		{"self HP under", Step{SelfHPBelow: 60}, base, true},
		{"self HP at", Step{SelfHPBelow: 50}, base, false},
		{"self HP above", Step{SelfHPAbove: 40}, base, true},
		{"self HP not above", Step{SelfHPAbove: 50}, base, false},
		{"MP above", Step{MPAbove: 30}, base, true},
		{"MP not above", Step{MPAbove: 50}, base, false},
		{"MP below", Step{MPBelow: 60}, base, true},
		{"MP not below", Step{MPBelow: 50}, base, false},
		{"within", Step{Within: 300}, base, true},
		{"too far", Step{Within: 299}, base, false},
		{"enough attackers", Step{Attackers: 1}, base, true},
		{"too few attackers", Step{Attackers: 2}, base, false},
		{"charges", Step{Charges: 2}, with(func(s *stepState) { s.charges = 2 }), true},
		{"too few charges", Step{Charges: 2}, with(func(s *stepState) { s.charges = 1 }), false},
		{"charge under max", Step{ChargeToMax: true}, with(func(s *stepState) { s.charges, s.maxCharges = 1, 2 }), true},
		{"charge at max", Step{ChargeToMax: true}, with(func(s *stepState) { s.charges, s.maxCharges = 2, 2 }), false},
		{"idle in combat", Step{Idle: true}, base, false},
		{"idle out of combat", Step{Idle: true}, with(func(s *stepState) { s.inCombat = false }), true},
		{"opener on a fresh target", Step{Opener: true}, with(func(s *stepState) { s.fresh = true }), true},
		{"opener used", Step{Opener: true}, base, false},
	}
	for _, tt := range tests {
		if got := tt.step.passes(tt.st); got != tt.want {
			t.Errorf("%s: passes = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestMeleePolicy checks when each kit policy melees.
func TestMeleePolicy(t *testing.T) {
	tests := []struct {
		m                   MeleePolicy
		underReserve, alone bool
		want                bool
	}{
		{MeleeAlways, false, false, true},
		{MeleeUnderReserve, false, true, false},
		{MeleeUnderReserve, true, false, true},
		{MeleeAloneUnderReserve, true, false, false},
		{MeleeAloneUnderReserve, false, true, false},
		{MeleeAloneUnderReserve, true, true, true},
	}
	for _, tt := range tests {
		if got := tt.m.allows(tt.underReserve, tt.alone); got != tt.want {
			t.Errorf("%d.allows(%v, %v) = %v, want %v", tt.m, tt.underReserve, tt.alone, got, tt.want)
		}
	}
}

// TestBuffFits checks the buff gates: the worn weapon kind and the member's
// class.
func TestBuffFits(t *testing.T) {
	tests := []struct {
		bf     Buff
		weapon string
		class  int32
		want   bool
	}{
		{Buff{Skill: 1}, "", 0, true},
		{Buff{Weapons: []string{"DUALFIST", "FIST"}}, "FIST", 0, true},
		{Buff{Weapons: []string{"DUALFIST", "FIST"}}, "SWORD", 0, false},
		{Buff{Members: []int32{classHumanMystic}}, "", classHumanMystic, true},
		{Buff{Members: []int32{classHumanMystic}}, "", HumanFighter.ID, false},
	}
	for _, tt := range tests {
		if got := tt.bf.fits(tt.weapon, tt.class); got != tt.want {
			t.Errorf("%+v.fits(%q, %d) = %v, want %v", tt.bf, tt.weapon, tt.class, got, tt.want)
		}
	}
}

// TestHealMembers checks a group heal goes out only with enough patients
// under its bound, else the single heal does.
func TestHealMembers(t *testing.T) {
	const pc2, groupHeal = int32(0x10000002), int32(1027)
	for _, tt := range []struct {
		name   string
		mateHP int32
		want   int32
	}{
		{"two under the bound", 50, groupHeal},
		{"one under the bound", 90, skillHeal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			p.Heals = []Heal{{Skill: groupHeal, HP: 60, Members: 2}, {Skill: skillHeal, HP: 60}}
			b, _ := newTestBrain(t, p)
			b.world.Self.HP = 50
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: groupHeal, Level: 1}, {ID: skillHeal, Level: 1}}}, t0)
			b.world.Apply(CharInfo{ObjectID: pc2, Name: "mate", Pos: Point{X: 100}}, t0)
			b.world.Party.LeaderID = selfID
			b.world.Party.Members = map[int32]*PartyMate{pc2: {PartyMember: PartyMember{ObjectID: pc2, HP: tt.mateHP, MaxHP: 100}}}
			if got := b.healSkill(selfID, at(time.Second)); got != tt.want {
				t.Errorf("healSkill = %d, want %d", got, tt.want)
			}
		})
	}
}
