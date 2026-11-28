package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// rowSkills are the level 1 datapack entries of the skills the fighter PvP
// rows cast or answer (docs/pvp/rows.md), as rowView projects them from the
// shipped datapack (TestPvPRowsShipped checks them against it): the brain
// tests of the rows play them without the datapack.
var rowSkills = map[datapack.SkillRef]datapack.Skill{
	// Power Strike
	{ID: 3, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"SWORD", "BLUNT", "BIGBLUNT", "BIGSWORD"}, HitTime: 1080 * time.Millisecond, Power: 25, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 3},
	// Mortal Blow
	{ID: 16, Level: 1}: {Type: "BLOW", Target: "ONE", WeaponsAllowed: []string{"DAGGER"}, HitTime: 1080 * time.Millisecond, Power: 73, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 40, MagicLevel: 3},
	// Iron Punch
	{ID: 29, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"DUALFIST"}, HitTime: 1604 * time.Millisecond, Power: 29, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 3},
	// Power Shot
	{ID: 56, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"BOW"}, HitTime: 3200 * time.Millisecond, Power: 65, Offensive: true, CastRange: 700, Radius: 80, MagicLevel: 3},
	// Elemental Heal
	{ID: 58, Level: 1}: {Type: "HEAL", Target: "SELF", HitTime: 5 * time.Second, Power: 71, Magic: true, Radius: 80, MagicLevel: 13},
	// Drain Health
	{ID: 70, Level: 1}: {Type: "DRAIN", Target: "ONE", HitTime: 3 * time.Second, Power: 20, Magic: true, Offensive: true, CastRange: 600, Radius: 80, MagicLevel: 13},
	// Shield Stun
	{ID: 92, Level: 1}: {Type: "STUN", Target: "ONE", WeaponsAllowed: []string{"SHIELD"}, HitTime: 1200 * time.Millisecond, Power: 80, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 18, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectPower: -1}}},
	// Aggression
	{ID: 28, Level: 1}: {Type: "AGGDAMAGE", Target: "ONE", HitTime: 1 * time.Second, Power: 655, Offensive: true, CastRange: 400, Radius: 80, MagicLevel: 22, Effects: []datapack.Effect{{Name: "TargetMe", Time: 1, Count: 1, EffectPower: -1}}},
	// Divine Heal
	{ID: 45, Level: 1}: {Type: "HEAL", Target: "SELF", HitTime: 4 * time.Second, Power: 143, Magic: true, Radius: 80, MagicLevel: 26},
	// Ultimate Defense
	{ID: 110, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1 * time.Second, Radius: 80, MagicLevel: 20, Effects: []datapack.Effect{{Name: "ImobileBuff", Time: 30, Count: 1, EffectPower: -1}}},
	// Deflect Arrow
	{ID: 112, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1500 * time.Millisecond, Radius: 80, MagicLevel: 24, Effects: []datapack.Effect{{Name: "Buff", Time: 1200, Count: 1, EffectPower: -1}}},
	// War Cry
	{ID: 78, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1500 * time.Millisecond, Radius: 80, MagicLevel: 20, Effects: []datapack.Effect{{Name: "Buff", Time: 60, Count: 1, EffectPower: -1}}},
	// Lionheart
	{ID: 287, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1500 * time.Millisecond, Radius: 80, MagicLevel: 36, Effects: []datapack.Effect{{Name: "Buff", Time: 60, Count: 1, EffectPower: -1}}},
	// Stun Attack
	{ID: 100, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"BLUNT", "BIGBLUNT"}, HitTime: 1080 * time.Millisecond, Power: 30, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 18, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}}},
	// Power Smash
	{ID: 255, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"SWORD", "BLUNT", "BIGBLUNT", "BIGSWORD"}, HitTime: 1080 * time.Millisecond, Power: 90, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 18},
	// Wild Sweep
	{ID: 245, Level: 1}: {Type: "PDAM", Target: "AREA", WeaponsAllowed: []string{"POLE"}, HitTime: 1080 * time.Millisecond, Power: 90, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 150, MagicLevel: 18},
	// Battle Roar
	{ID: 121, Level: 1}: {Type: "HEAL_PERCENT", Target: "SELF", HitTime: 1500 * time.Millisecond, Power: 9.1, Radius: 80, MagicLevel: 28, Effects: []datapack.Effect{{Name: "Buff", Time: 600, Count: 1, EffectPower: -1}}},
	// Dash
	{ID: 4, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1 * time.Second, Radius: 80, MagicLevel: 20, Effects: []datapack.Effect{{Name: "Buff", Time: 15, Count: 1, EffectPower: -1}}},
	// Bleed
	{ID: 96, Level: 1}: {Type: "BLEED", Target: "ONE", WeaponsAllowed: []string{"DAGGER"}, HitTime: 1080 * time.Millisecond, Power: 100, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 24, Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 7, EffectPower: -1}}},
	// Stunning Shot
	{ID: 101, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"BOW"}, HitTime: 3 * time.Second, Power: 344, Offensive: true, Debuff: true, CastRange: 900, Radius: 80, MagicLevel: 34, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}, {Name: "RemoveTarget", Time: 1, Count: 1, EffectType: "DEBUFF", EffectPower: 50}}},
	// Ultimate Evasion
	{ID: 111, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1 * time.Second, Radius: 80, MagicLevel: 28, Effects: []datapack.Effect{{Name: "Buff", Time: 30, Count: 1, EffectPower: -1}}},
	// Entangle
	{ID: 102, Level: 1}: {Type: "DEBUFF", Target: "ONE", HitTime: 3 * time.Second, Power: 80, Magic: true, Offensive: true, Debuff: true, CastRange: 600, Radius: 80, MagicLevel: 36, Effects: []datapack.Effect{{Name: "Debuff", Time: 60, Count: 1, EffectPower: -1, Funcs: slowFuncs}}},
	// Sprint
	{ID: 230, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 4 * time.Second, Magic: true, Radius: 80, MagicLevel: 32, Effects: []datapack.Effect{{Name: "Buff", Time: 1200, Count: 1, EffectPower: -1}}},
	// Poison
	{ID: 129, Level: 1}: {Type: "POISON", Target: "ONE", HitTime: 3 * time.Second, Power: 70, Magic: true, Offensive: true, Debuff: true, CastRange: 600, Radius: 80, MagicLevel: 20, Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 10, EffectPower: -1}}},
	// Power Break
	{ID: 115, Level: 1}: {Type: "DEBUFF", Target: "ONE", HitTime: 1200 * time.Millisecond, Power: 80, Magic: true, Offensive: true, Debuff: true, CastRange: 600, Radius: 80, MagicLevel: 32, Effects: []datapack.Effect{{Name: "Debuff", Time: 15, Count: 1, EffectPower: -1}}},
	// Freezing Strike
	{ID: 105, Level: 1}: {Type: "MDAM", Target: "ONE", HitTime: 3 * time.Second, Power: 26, Magic: true, Offensive: true, Debuff: true, CastRange: 600, Radius: 80, MagicLevel: 34, Effects: []datapack.Effect{{Name: "Debuff", Time: 30, Count: 1, EffectType: "DEBUFF", EffectPower: 60, Funcs: slowFuncs}}},
	// Sting
	{ID: 223, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"SWORD", "DAGGER", "DUAL", "BIGSWORD"}, HitTime: 1080 * time.Millisecond, Power: 41, Offensive: true, Debuff: true, NextActionAttack: true, EffectType: "BLEED", CastRange: 40, Radius: 80, MagicLevel: 22, Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 7, EffectType: "BLEED", EffectPower: 50}}},
	// Rage
	{ID: 94, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: 1500 * time.Millisecond, Radius: 80, MagicLevel: 24, Effects: []datapack.Effect{{Name: "Buff", Time: 90, Count: 1, EffectPower: -1}}},
	// Force Blaster
	{ID: 54, Level: 1}: {Type: "CHARGEDAM", Target: "ONE", WeaponsAllowed: []string{"DUALFIST"}, HitTime: 1900 * time.Millisecond, Power: 143, Offensive: true, CastRange: 600, Radius: 80, MagicLevel: 22},
	// Cripple
	{ID: 95, Level: 1}: {Type: "DEBUFF", Target: "ONE", WeaponsAllowed: []string{"FIST", "DUALFIST"}, HitTime: 1604 * time.Millisecond, Power: 80, Offensive: true, Debuff: true, CastRange: 40, Radius: 80, MagicLevel: 20, Effects: []datapack.Effect{{Name: "Debuff", Time: 30, Count: 1, EffectPower: -1, Funcs: slowFuncs}}},
	// Stunning Fist
	{ID: 120, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"DUALFIST"}, HitTime: 1604 * time.Millisecond, Power: 45, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 18, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}}},
	// Hurricane Assault
	{ID: 284, Level: 1}: {Type: "CHARGEDAM", Target: "ONE", WeaponsAllowed: []string{"DUALFIST"}, HitTime: 1360 * time.Millisecond, Power: 501, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 34},
	// Lightning Strike
	{ID: 279, Level: 1}: {Type: "MDAM", Target: "ONE", HitTime: 3 * time.Second, Power: 82, Magic: true, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 400, Radius: 80, MagicLevel: 58, Effects: []datapack.Effect{{Name: "Paralyze", Time: 30, Count: 1, EffectType: "PARALYZE", EffectPower: 25}}},
	// Arrest
	{ID: 402, Level: 1}: {Type: "ROOT", Target: "ONE", HitTime: 2 * time.Second, Power: 70, Offensive: true, Debuff: true, CastRange: 150, Radius: 80, MagicLevel: 55, Effects: []datapack.Effect{{Name: "TargetMe", Time: 1, Count: 1, EffectPower: -1}, {Name: "Root", Time: 8, Count: 1, EffectPower: -1}}},
	// Shackle
	{ID: 403, Level: 1}: {Type: "ROOT", Target: "ONE", HitTime: 2 * time.Second, Power: 80, Offensive: true, Debuff: true, CastRange: 600, Radius: 80, MagicLevel: 55, Effects: []datapack.Effect{{Name: "Root", Time: 10, Count: 1, EffectPower: -1}}},
	// Triple Slash
	{ID: 1, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"DUAL"}, HitTime: 1733 * time.Millisecond, Power: 431, Offensive: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 38},
	// Thunder Storm
	{ID: 48, Level: 1}: {Type: "PDAM", Target: "AURA", WeaponsAllowed: []string{"POLE"}, HitTime: 1080 * time.Millisecond, Power: 351, Offensive: true, Debuff: true, Radius: 150, MagicLevel: 38, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}}},
	// Hammer Crush
	{ID: 260, Level: 1}: {Type: "PDAM", Target: "ONE", WeaponsAllowed: []string{"BLUNT", "BIGBLUNT"}, HitTime: 1080 * time.Millisecond, Power: 406, Offensive: true, Debuff: true, NextActionAttack: true, CastRange: 40, Radius: 80, MagicLevel: 38, Effects: []datapack.Effect{{Name: "Stun", Time: 9, Count: 1, EffectType: "STUN", EffectPower: 50}}},
	// Wrath
	{ID: 320, Level: 1}: {Type: "CPDAMPERCENT", Target: "AREA", WeaponsAllowed: []string{"POLE"}, HitTime: 1080 * time.Millisecond, Power: 7, Offensive: true, CastRange: 40, Radius: 80, MagicLevel: 65},
	// Heal
	{ID: 1011, Level: 1}: {Type: "HEAL", Target: "ONE", HitTime: 5 * time.Second, Power: 49, Magic: true, CastRange: 600, Radius: 80, MagicLevel: 3},
	// Battle Heal
	{ID: 1015, Level: 1}: {Type: "HEAL", Target: "ONE", HitTime: 2 * time.Second, Power: 83, Magic: true, CastRange: 600, Radius: 80, MagicLevel: 10},
	// Poison Blade Dance
	{ID: 84, Level: 1}: {Type: "PDAM", Target: "AURA", WeaponsAllowed: []string{"DUAL"}, HitTime: 1833 * time.Millisecond, Power: 1, Offensive: true, Debuff: true, EffectType: "POISON", Radius: 150, MagicLevel: 55, Effects: []datapack.Effect{{Name: "DamOverTime", Time: 3, Count: 10, EffectType: "POISON", EffectPower: 35}}},
	// Demonic Blade Dance
	{ID: 408, Level: 1}: {Type: "PDAM", Target: "AURA", WeaponsAllowed: []string{"DUAL"}, HitTime: 3 * time.Second, Power: 229, Offensive: true, Debuff: true, Radius: 150, MagicLevel: 55, Effects: []datapack.Effect{{Name: "Debuff", Time: 15, Count: 1, EffectType: "DEBUFF", EffectPower: 40}}},
}

// slowFuncs are a run speed slow's stat functions (Entangle, Cripple,
// Freezing Strike).
var slowFuncs = []datapack.StatFunc{{Op: "mul", Stat: "runSpd", Value: 0.7}}

// rowView is the part of skill s the PvP layer reads: its type, target,
// weapons, hit time, power, landing roll, ranges and its effects' names,
// spans, landing rolls and slows.
func rowView(s datapack.Skill) datapack.Skill {
	v := datapack.Skill{
		Type: s.Type, Target: s.Target, HitTime: s.HitTime, Power: s.Power, Magic: s.Magic,
		Offensive: s.Offensive, Debuff: s.Debuff, NextActionAttack: s.NextActionAttack,
		EffectType: s.EffectType, EffectPower: s.EffectPower, CastRange: s.CastRange,
		Radius: s.Radius, MagicLevel: s.MagicLevel,
	}
	if len(s.WeaponsAllowed) > 0 {
		v.WeaponsAllowed = s.WeaponsAllowed
	}
	for _, e := range s.Effects {
		fe := datapack.Effect{Name: e.Name, Time: e.Time, Count: e.Count, EffectType: e.EffectType, EffectPower: e.EffectPower}
		if slows(e) {
			fe.Funcs = slowFuncs
		}
		v.Effects = append(v.Effects, fe)
	}
	return v
}
