package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Role is the part a bot plays (docs/strategy.md, Design section 4): Support
// plays for a party, Damage farms alone, Crafter is the clan crafter (M9).
// The preset sets it (Persona.Role).
type Role uint8

const (
	// RoleNone compiles no build: a persona without a role plays only its
	// own tables and thresholds, and a step without one is in every role's
	// rotation.
	RoleNone Role = iota
	RoleDamage
	RoleSupport
	RoleCrafter
)

func (r Role) String() string {
	switch r {
	case RoleDamage:
		return "damage"
	case RoleSupport:
		return "support"
	case RoleCrafter:
		return "crafter"
	}
	return "none"
}

// Thresholds are the HP and MP percents the shared rules act on (Persona has
// each one's meaning): the build's defaults, which a persona's own non-zero
// value replaces. A negative value turns the threshold off over the build,
// as 0 does without one: below never holds for it.
type Thresholds struct {
	CriticalHP, CrowdedHP, OutmatchedHP int
	MPReserve                           int
	SitHP, SitMP, StandHP, StandMP      int
}

// defaultThresholds are every role's thresholds: the support persona's
// values from before builds (docs/plan.md, support persona), which the
// later M10 rows tune per row (D24).
var defaultThresholds = Thresholds{
	CriticalHP: 20, CrowdedHP: 80, OutmatchedHP: 50,
	MPReserve: 50,
	SitHP:     40, SitMP: 30, StandHP: 90, StandMP: 90,
}

// override is t with every non-zero value of o in its place.
func (t Thresholds) override(o Thresholds) Thresholds {
	pick := func(dst *int, v int) {
		if v != 0 {
			*dst = v
		}
	}
	pick(&t.CriticalHP, o.CriticalHP)
	pick(&t.CrowdedHP, o.CrowdedHP)
	pick(&t.OutmatchedHP, o.OutmatchedHP)
	pick(&t.MPReserve, o.MPReserve)
	pick(&t.SitHP, o.SitHP)
	pick(&t.SitMP, o.SitMP)
	pick(&t.StandHP, o.StandHP)
	pick(&t.StandMP, o.StandMP)
	return t
}

// StepTarget is whom a rotation step is cast on.
type StepTarget uint8

const (
	// TargetEnemy is the bot's combat target.
	TargetEnemy StepTarget = iota
	// TargetSelf is the bot itself (an in-combat self buff or sustain).
	TargetSelf
	// TargetOtherAttacker is an attacker the bot is not fighting (Dryad
	// Root on the second one).
	TargetOtherAttacker
	// TargetLeaderTarget is the party leader's target (a support's assist).
	TargetLeaderTarget
)

// Step is one step of a kit's rotation (docs/strategy.md, Design section 4):
// fight casts the first step whose gates pass, else follows the kit's Melee
// policy. A zero field is no gate.
type Step struct {
	Skill int32
	// From is the level the step starts at (the row's "@N"); CompileBuild
	// leaves it out below that level. 0 starts it with the row.
	From int32
	// Weapons: the worn weapon's datapack kind must be one of them, or
	// SHIELD for a worn shield (the server checks the held items).
	Weapons []string
	// MinTargetHP casts only while the target's HP percent is at least
	// that, SelfHPBelow only with the bot's HP under it and SelfHPAbove
	// only above it, MPAbove only with its MP above it and MPBelow only
	// under it. A target whose HP the bot has not seen counts as full.
	MinTargetHP, SelfHPBelow, SelfHPAbove int
	MPAbove, MPBelow                      int
	// Within casts only while the target is that close; Attackers only
	// with at least that many attackers.
	Within    int32
	Attackers int
	// Target is whom the step is cast on.
	Target StepTarget
	// Idle casts only out of combat.
	Idle bool
	// Charges casts only with at least that many force charges, the skill's
	// own Charges condition; ChargeToMax casts a charging skill only below
	// its maxCharges at the level the bot knows.
	Charges     int
	ChargeToMax bool
	// Opener casts only as the first attack on a fresh target.
	Opener bool
	// Role puts the step in that role's rotation only; RoleNone in every
	// role's. CompileBuild leaves the other roles' steps out.
	Role Role
}

// stepState is what a step's gates read at one decision.
type stepState struct {
	// weapon is the worn weapon's datapack kind, "" with none worn or none
	// known; shield is whether a shield is worn.
	weapon string
	shield bool
	// hp, maxHP, mp and maxMP are the bot's; targetHP and targetMaxHP the
	// target's, both 1 while unseen.
	hp, maxHP, mp, maxMP  int32
	targetHP, targetMaxHP int32
	// dist is the distance to the target.
	dist int32
	// attackers are the attackers the bot may fight; charges its force
	// charges and maxCharges the most its charging skill holds.
	attackers           int
	charges, maxCharges int
	inCombat            bool
	// fresh is whether no attack of the bot has gone out on the target.
	fresh bool
}

// passes reports whether every gate of the step holds in st. Target is not
// a gate: fight casts only TargetEnemy steps until the rotation casts on
// others.
func (s Step) passes(st stepState) bool {
	if len(s.Weapons) > 0 && !slices.ContainsFunc(s.Weapons, func(k string) bool {
		return k == "SHIELD" && st.shield || k != "SHIELD" && k == st.weapon
	}) {
		return false
	}
	above := func(cur, maxV int32, pct int) bool { return maxV > 0 && int64(cur)*100 > int64(maxV)*int64(pct) }
	switch {
	case s.MinTargetHP > 0 && below(st.targetHP, st.targetMaxHP, s.MinTargetHP),
		s.SelfHPBelow > 0 && !below(st.hp, st.maxHP, s.SelfHPBelow),
		s.SelfHPAbove > 0 && !above(st.hp, st.maxHP, s.SelfHPAbove),
		s.MPAbove > 0 && !above(st.mp, st.maxMP, s.MPAbove),
		s.MPBelow > 0 && !below(st.mp, st.maxMP, s.MPBelow),
		s.Within > 0 && st.dist > s.Within,
		s.Attackers > 0 && st.attackers < s.Attackers,
		s.Charges > 0 && st.charges < s.Charges,
		s.ChargeToMax && st.charges >= st.maxCharges,
		s.Idle && st.inCombat,
		s.Opener && !st.fresh:
		return false
	}
	return true
}

// MeleePolicy is when a kit melees once no rotation step passes.
type MeleePolicy uint8

const (
	// MeleeAlways melees whenever no step passes (fighters).
	MeleeAlways MeleePolicy = iota
	// MeleeUnderReserve melees only with MP under MPReserve (casters).
	MeleeUnderReserve
	// MeleeAloneUnderReserve melees only out of a party and with MP under
	// MPReserve (healers).
	MeleeAloneUnderReserve
)

// allows reports whether the policy melees with MP under the reserve or
// not, in a party or alone.
func (m MeleePolicy) allows(underReserve, alone bool) bool {
	switch m {
	case MeleeUnderReserve:
		return underReserve
	case MeleeAloneUnderReserve:
		return underReserve && alone
	}
	return true
}

// ClassKit is what one class plays (docs/strategy.md, Design section 4): the
// skills it learns, at which trainers, its rotation, its buffs and heals,
// its flee skill and melee policy, its gear and its dyes.
type ClassKit struct {
	// Skills are the skills the bot learns with SP at a trainer, first
	// listed first; Trainers the NPC template ids of the class's trainers.
	Skills, Trainers []int32
	Rotation         []Step
	Buffs            []Buff
	Heals            []Heal
	// Flee is the skill flee casts first when ready (Dash, Sprint); 0 none.
	Flee  int32
	Melee MeleePolicy
	// Gear is the class's gear table entry, Dyes its dye symbols: the
	// table the dye errand draws where the model cannot value dyes
	// (dyePlan).
	Gear GearClass
	Dyes []int32
	// DyeStats are the base stats (STR, CON, DEX, INT, WIT, MEN) the
	// class's dyes raise (docs/strategy.md column 11): the dye errand draws
	// the symbols raising one of them with the highest value gain
	// (dyePlan); none lets every symbol the class may draw compete.
	DyeStats []string
}

// RaceProfile is what a race's lines share (docs/strategy.md, Design section
// 4): the profession each class of its lines takes next, and the armor
// kinds its mystic line may wear besides its robes.
type RaceProfile struct {
	Race int32
	// Professions maps a class id to the profession the bot takes next.
	Professions map[int32]int32
	// MysticArmor are the armor kinds a mystic of the race may wear besides
	// the class's (GearClass.Armor), compiled for the build's level and
	// role.
	MysticArmor []ArmorGrant
}

// ArmorGrant is an armor kind a mystic line may wear in Role from level
// From, the level its class tree grants Mastery at; the gear errand weighs
// it only once the bot knows the mastery (Brain.armorKinds).
type ArmorGrant struct {
	Kind    string
	Mastery int32
	From    int32
	Role    Role
}

// The armor masteries of the orc mystic line (data/xml/classes/orcMystic.xml,
// skills/0200-0299.xml): Light Armor Mastery casts ×1.9 and attacks ×1.25 in
// light armor, Heavy Armor Mastery ×1.71 and ×1.25 in heavy armor, both
// with P. Def.
const (
	skillLightArmorMastery int32 = 252
	skillHeavyArmorMastery int32 = 253
)

// raceMysticArmor are the armor kinds by race a mystic line may wear
// besides robes (docs/strategy.md, "Armor kinds by race", rows OM and OS):
// the orc mystic's Support role wears light armor from 7 and heavy armor
// from 14 when they score higher; its Damage role, and every other race's
// mystic, stays in robes, where Spellcraft ranks every other kind far
// below.
var raceMysticArmor = map[int32][]ArmorGrant{
	3: {
		{Kind: "LIGHT", Mastery: skillLightArmorMastery, From: 7, Role: RoleSupport},
		{Kind: "HEAVY", Mastery: skillHeavyArmorMastery, From: 14, Role: RoleSupport},
	},
}

// dyeStats are the base stats each profession's dyes raise, by the
// first-profession rows of docs/strategy.md column 11; a second profession
// keeps its first profession's (second-profession kits are #332).
var dyeStats = func() map[int32][]string {
	t := make(map[int32][]string)
	set := func(stats []string, classes ...int32) {
		for _, c := range classes {
			t[c] = stats
		}
	}
	set([]string{"STR"}, 1, 2, 3, 22, 23, 24, 35, 36, 37, 45, 46, 47, 48, 54, 55, 56, 57)
	set([]string{"CON"}, 4, 5, 6, 19, 20, 21, 32, 33, 34)
	set([]string{"DEX", "STR"}, 7, 8, 9)
	set([]string{"INT"}, 11, 12, 13, 14, 26, 27, 28, 39, 40, 41)
	set([]string{"WIT"}, 15, 16, 17, 29, 30)
	set([]string{"MEN"}, 42, 43, 50, 51, 52)
	return t
}()

// Build is what the bot plays: its race profile, its class kit and its role,
// compiled for its class and level, and the thresholds its rules act on.
type Build struct {
	Race RaceProfile
	Kit  ClassKit
	Role Role
	// Branch is the first profession's class id the bot heads for; 0 until
	// the branch choice (S8, #328).
	Branch     int32
	Thresholds Thresholds
}

// lineKits are the combat parts of the kits by base class, the same for a
// line's every profession (S4 compiles the support persona's values from
// before builds, D24): the human, elven and dark elven mystics heal with
// Battle Heal under 35 % and Heal under 60 %, buff Might and Shield and nuke
// with Wind Strike; the orc mystics buff Soul Shield and Chant of Battle and
// nuke with Chill Flame and Life Drain, from level 7. Every skill is in the
// line's tree (data/xml/classes, TestBehaviorRowsShipped), so each line
// keeps exactly the part of supportKit it can know.
var lineKits = map[int32]ClassKit{
	classHumanMystic: casterKit,
	classElvenMystic: casterKit,
	classDarkMystic:  casterKit,
	classOrcMystic: {
		Rotation: []Step{{Skill: skillChillFlame, From: 7}, {Skill: skillLifeDrain, From: 7}},
		Buffs:    []Buff{{Skill: skillSoulShield}, {Skill: skillChantBattle, Party: true}},
	},
}

// supportKit is the support persona's whole table from before builds
// (docs/plan.md, support persona), which the bot cast as far as it knew the
// skills. A line without a kit of its own keeps it until its rows replace
// it (the fighters, S6 #325): a fighter knows none of its skills.
var supportKit = ClassKit{
	Rotation: []Step{{Skill: skillWindStrike}, {Skill: skillChillFlame}, {Skill: skillLifeDrain}},
	Heals:    []Heal{{Skill: skillBattleHeal, HP: 35}, {Skill: skillHeal, HP: 60}},
	Buffs:    []Buff{{Skill: skillMight}, {Skill: skillShield}, {Skill: skillSoulShield}, {Skill: skillChantBattle, Party: true}},
}

var casterKit = ClassKit{
	Rotation: []Step{{Skill: skillWindStrike}},
	Heals:    []Heal{{Skill: skillBattleHeal, HP: 35}, {Skill: skillHeal, HP: 60}},
	Buffs:    []Buff{{Skill: skillMight}, {Skill: skillShield}},
}

// CompileBuild is the build of a bot of race playing class at level in role
// (docs/strategy.md, Design section 4): its line's kit (lineKits, else
// supportKit), with the steps the role plays from the level reached, its
// class's skills, trainers, gear and dyes, its race's professions and the
// thresholds. RoleNone compiles the zero Build. The tables are Go tables,
// checked against the shipped datapack by TestBehaviorRowsShipped.
func CompileBuild(race, class, level int32, role Role) Build {
	if role == RoleNone {
		return Build{}
	}
	bd := Build{Role: role, Thresholds: defaultThresholds, Race: RaceProfile{Race: race, Professions: make(map[int32]int32)}}
	for from, to := range supportBuilds {
		if r, ok := datapack.ClassRace(from); ok && r == race {
			bd.Race.Professions[from] = to
		}
	}
	k := &bd.Kit
	k.Skills, k.Trainers = mysticSkills[class], mysticTrainers[class]
	k.Gear, k.Dyes, k.DyeStats = gearClassOf(defaultGearClasses(), class), defaultDyeTable()[class], dyeStats[class]
	line := supportKit
	if base, ok := datapack.BaseClass(class); ok {
		if l, ok := lineKits[base]; ok {
			line = l
		}
		if slices.Contains(mysticClasses, base) {
			for _, g := range raceMysticArmor[race] {
				if g.From <= level && g.Role == role {
					bd.Race.MysticArmor = append(bd.Race.MysticArmor, g)
				}
			}
		}
	}
	k.Buffs, k.Heals, k.Flee, k.Melee = line.Buffs, line.Heals, line.Flee, line.Melee
	for _, st := range line.Rotation {
		if st.From <= level && (st.Role == RoleNone || st.Role == role) {
			k.Rotation = append(k.Rotation, st)
		}
	}
	return bd
}

// override is bd with the persona's own tables and thresholds in place of
// the build's, for class: each non-nil table and non-zero threshold of the
// persona replaces the build's (docs/strategy.md, "Overrides").
func (p *Persona) override(bd Build, class int32) Build {
	k := &bd.Kit
	if p.LearnSkills != nil {
		k.Skills = p.LearnSkills[class]
	}
	if p.Trainers != nil {
		k.Trainers = p.Trainers[class]
	}
	if p.AttackSkills != nil {
		k.Rotation = make([]Step, len(p.AttackSkills))
		for i, id := range p.AttackSkills {
			k.Rotation[i] = Step{Skill: id}
		}
	}
	if p.Heals != nil {
		k.Heals = p.Heals
	}
	if p.Buffs != nil {
		k.Buffs = p.Buffs
	}
	if p.Gear.Classes != nil {
		k.Gear = gearClassOf(p.Gear.Classes, class)
	}
	if p.Dyes.Symbols != nil {
		k.Dyes = p.Dyes.Symbols[class]
	}
	if p.ClassChange.Builds != nil {
		bd.Race.Professions = p.ClassChange.Builds
	}
	bd.Thresholds = bd.Thresholds.override(Thresholds{
		CriticalHP: p.CriticalHP, CrowdedHP: p.CrowdedHP, OutmatchedHP: p.OutmatchedHP,
		MPReserve: p.MPReserve,
		SitHP:     p.SitHP, SitMP: p.SitMP, StandHP: p.StandHP, StandMP: p.StandMP,
	})
	return bd
}

// buildKey is what a compiled build is for.
type buildKey struct {
	class, level int32
	role         Role
}

// build is the bot's build now, with the persona's overrides: CompileBuild
// runs again whenever the bot's class, level or role differs from the last
// compile's, so at entry and on every UserInfo that changed its class or
// level. The overrides are applied at each call, so a persona edited between
// compiles still wins.
func (b *Brain) build() Build {
	s := &b.world.Self
	k := buildKey{s.ClassID, s.Level, b.persona.Role}
	if !b.builtOK || b.builtFor != k {
		race, _ := datapack.ClassRace(s.ClassID)
		b.built, b.builtFor, b.builtOK = CompileBuild(race, s.ClassID, s.Level, k.role), k, true
	}
	return b.persona.override(b.built, s.ClassID)
}

// gearClassOf is the first entry of table for class, an entry without
// classes matching any; the zero GearClass when none does.
func gearClassOf(table []GearClass, class int32) GearClass {
	for _, c := range table {
		if len(c.Classes) == 0 || slices.Contains(c.Classes, class) {
			return c
		}
	}
	return GearClass{}
}

// stepState is the state the gates of steps read at now, fighting id.
func (b *Brain) stepState(id int32, steps []Step, now time.Time) stepState {
	s := &b.world.Self
	from := s.Position(now)
	st := stepState{
		hp: s.HP, maxHP: s.MaxHP, mp: s.MP, maxMP: s.MaxMP, targetHP: 1, targetMaxHP: 1,
		attackers: len(b.attackers(now)), inCombat: s.InCombat, fresh: b.engaged != id,
	}
	if o := b.world.Objects[id]; o != nil {
		if o.MaxHP > 0 {
			st.targetHP, st.targetMaxHP = o.HP, o.MaxHP
		}
		st.dist = int32(math.Sqrt(float64(dist2(from, o.Position(now)))))
	}
	if slices.ContainsFunc(steps, func(x Step) bool { return len(x.Weapons) > 0 }) {
		st.weapon, st.shield = b.wornKind()
	}
	return st
}

// wornKind is the datapack kind of the weapon the bot wields, FIST with
// none (the class's fists) and "" for one the travel map does not know, and
// whether it wears a shield.
func (b *Brain) wornKind() (kind string, shield bool) {
	w := b.worn()
	kind = "FIST"
	if it := w[slotWeapon]; it.ObjectID != 0 {
		kind = ""
		if g, ok := b.gearItem(it.TemplateID); ok {
			kind = g.tmpl.Kind
		}
	}
	return kind, w[slotShield].ObjectID != 0
}

// buffWeapon is the worn weapon's kind when a buff of buffs is gated by
// one, else "".
func (b *Brain) buffWeapon(buffs []Buff) string {
	if !slices.ContainsFunc(buffs, func(bf Buff) bool { return len(bf.Weapons) > 0 }) {
		return ""
	}
	kind, _ := b.wornKind()
	return kind
}

// fits reports whether the buff's gates let it go out with the worn weapon
// kind weapon on a patient of class.
func (bf Buff) fits(weapon string, class int32) bool {
	return (len(bf.Weapons) == 0 || slices.Contains(bf.Weapons, weapon)) &&
		(len(bf.Members) == 0 || slices.Contains(bf.Members, class))
}

// classOf is the class of patient id: the bot's own or a party member's.
func (b *Brain) classOf(id int32) int32 {
	if id == b.world.Self.ObjectID {
		return b.world.Self.ClassID
	}
	if m := b.world.Party.Members[id]; m != nil {
		return m.ClassID
	}
	return 0
}
