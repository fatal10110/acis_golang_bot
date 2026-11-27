package botclient

import "slices"

// Skill ids of the healer and buffer rows BI, PP, EE and SE (docs/strategy.md
// "Second professions (levels 40+)"; data/xml/skills).
const (
	skillRecharge          int32 = 1013
	skillVitalize          int32 = 1020
	skillMagicBarrier      int32 = 1036
	skillBlessedBody       int32 = 1045
	skillBlessedSoul       int32 = 1048
	skillHaste             int32 = 1086
	skillGreaterHeal       int32 = 1217
	skillGreaterBattleHeal int32 = 1218
	skillGreaterGroupHeal  int32 = 1219
	skillGuidance          int32 = 1240
	skillDeathWhisper      int32 = 1242
	skillBlessShield       int32 = 1243
	skillRestoreLife       int32 = 1258
	skillWildMagic         int32 = 1303
	skillAdvancedBlock     int32 = 1304
	skillPrayer            int32 = 1307
	skillGreaterMight      int32 = 1388
	skillGreaterShield     int32 = 1389
	skillClarity           int32 = 1397
	skillMajorHeal         int32 = 1401
	skillMajorGroupHeal    int32 = 1402
)

// fighterClasses are the fighter class ids: every class id not in
// mysticClasses, every profession of the five fighter base classes
// (aCis_datapack/data/xml/classes/*Fighter.xml).
var fighterClasses = []int32{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 88, 89, 90, 91, 92, 93, // human
	18, 19, 20, 21, 22, 23, 24, 99, 100, 101, 102, // elven
	31, 32, 33, 34, 35, 36, 37, 106, 107, 108, 109, // dark elven
	44, 45, 46, 47, 48, 113, 114, // orc
	53, 54, 55, 56, 57, 117, 118, // dwarven
}

// rechargeClasses are the classes whose tree holds Recharge: the Elven and
// Shillien Oracles, Elders and Saints. Recharge's condition (skills
// 1000-1099.xml, 1013: <not><target active_skill_id="1013"/></not>) makes
// the server refuse it on a target that knows it, so the bot never casts it
// on them.
var rechargeClasses = []int32{29, 30, 105, 42, 43, 112}

// recharge is the Elders' MP heal: Recharge on a member other than the bot
// under 40 % MP (rows EE SE, column 4).
var recharge = Heal{Skill: skillRecharge, HP: 40, MP: true}

// The healer and buffer second professions (docs/strategy.md "Second
// professions (levels 40+)", rows BI PP EE SE), registered in
// professionKits. Each heals the leader first, nukes the leader's target
// with Wind Strike above its 50 % MPReserve and melees only alone and under
// it; Resurrection is cast by ruleRaise as the Cleric's. Their PvP rows are
// their first profession's until #421.
var (
	// bishopKit (BI, from CL): Restore Life from 44 under 20 % HP, Greater
	// Battle Heal under 35 %, Major Group Heal from 58 with two members
	// under 60 %, else Greater Group Heal, Major Heal from 56 under 60 %,
	// else Greater Heal, else Vitalize from 48; Wind Strike (fixed) on the
	// leader's target; Might, Shield (base), Wind Walk, Acumen on the
	// mystic members, Concentration, and Prayer from 66.
	bishopKit = ClassKit{
		Skills: []int32{
			skillRestoreLife, skillGreaterBattleHeal, skillMajorGroupHeal, skillGreaterGroupHeal, skillMajorHeal,
			skillGreaterHeal, skillVitalize, skillMight, skillShield, skillWindWalk, skillAcumen, skillConcentration,
			skillPrayer, skillWindStrike, skillResurrection, skillDryadRoot, skillIceBolt, skillMentalShield,
		},
		Rotation: []Step{{Skill: skillWindStrike, Target: TargetLeaderTarget}},
		Heals: []Heal{
			{Skill: skillRestoreLife, From: 44, HP: 20},
			{Skill: skillGreaterBattleHeal, HP: 35},
			{Skill: skillMajorGroupHeal, From: 58, HP: 60, Members: 2},
			{Skill: skillGreaterGroupHeal, HP: 60, Members: 2},
			{Skill: skillMajorHeal, From: 56, HP: 60},
			{Skill: skillGreaterHeal, HP: 60},
			{Skill: skillVitalize, From: 48, HP: 60},
		},
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillAcumen, Members: mysticClasses},
			{Skill: skillConcentration},
			{Skill: skillPrayer, From: 66, Party: true},
		}),
		Melee:      MeleeAloneUnderReserve,
		Base:       []int32{skillMight, skillShield},
		Fixed:      []int32{skillWindStrike},
		Thresholds: healerThresholds,
		PvP:        clericPvP,
		SpotMargin: marginGlassCannon,
	}

	// prophetKit (PP, from CL): the Cleric's heals (fixed); Dryad Root on
	// a second attacker, Wind Strike (fixed) on the leader's target; the
	// Cleric's buffs, Bless Shield, Blessed Body and Magic Barrier from 44;
	// on the fighter members Haste from 44, Guidance, Death Whisper and
	// Greater Might from 58; on the mystic members Blessed Soul from 44 and
	// Greater Shield from 58 (one stack with Greater Might: the stack rule
	// never casts one over the other).
	prophetKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillMight, skillShield, skillWindWalk, skillAcumen,
			skillConcentration, skillBlessShield, skillBlessedBody, skillMagicBarrier, skillHaste, skillGuidance,
			skillDeathWhisper, skillGreaterMight, skillBlessedSoul, skillGreaterShield, skillDryadRoot, skillWindStrike,
			skillResurrection, skillIceBolt, skillMentalShield,
		},
		Rotation: []Step{
			{Skill: skillDryadRoot, Target: TargetOtherAttacker, Attackers: 2},
			{Skill: skillWindStrike, Target: TargetLeaderTarget},
		},
		Heals: healerHeals,
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillAcumen, Members: mysticClasses},
			{Skill: skillConcentration},
			{Skill: skillBlessShield},
			{Skill: skillBlessedBody, From: 44},
			{Skill: skillMagicBarrier, From: 44},
			{Skill: skillHaste, From: 44, Members: fighterClasses},
			{Skill: skillGuidance, Members: fighterClasses},
			{Skill: skillDeathWhisper, Members: fighterClasses},
			{Skill: skillGreaterMight, From: 58, Members: fighterClasses},
			{Skill: skillBlessedSoul, From: 44, Members: mysticClasses},
			{Skill: skillGreaterShield, From: 58, Members: mysticClasses},
		}),
		Melee:      MeleeAloneUnderReserve,
		Base:       []int32{skillMight, skillShield},
		Fixed:      []int32{skillBattleHeal, skillGroupHeal, skillHeal, skillWindStrike},
		Thresholds: healerThresholds,
		PvP:        clericPvP,
		SpotMargin: marginGlassCannon,
	}

	// elvenElderKit (EE, from EO): the healers' Battle Heal and Group
	// Heal, then Major Heal from 56, Greater Heal and Vitalize from 48
	// ahead of Heal; Recharge on another member under 40 % MP once no
	// patient needs an HP heal; Dryad Root on a second attacker, Wind
	// Strike (fixed) on the leader's target; the Oracle's buffs, Bless
	// Shield, Advanced Block from 58; on the mystic members Clarity from 58
	// and Wild Magic from 62.
	elvenElderKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillMajorHeal, skillGreaterHeal, skillVitalize, skillRecharge,
			skillMight, skillShield, skillWindWalk, skillAgility, skillHolyWeapon, skillBlessShield, skillAdvancedBlock,
			skillClarity, skillWildMagic, skillDryadRoot, skillWindStrike, skillResurrection, skillWindShackle,
			skillIceBolt, skillMentalShield,
		},
		Rotation: []Step{
			{Skill: skillDryadRoot, Target: TargetOtherAttacker, Attackers: 2},
			{Skill: skillWindStrike, Target: TargetLeaderTarget},
		},
		Heals: []Heal{
			{Skill: skillBattleHeal, HP: 35},
			{Skill: skillGroupHeal, HP: 60, Members: 2},
			{Skill: skillMajorHeal, From: 56, HP: 60},
			{Skill: skillGreaterHeal, From: 48, HP: 60},
			{Skill: skillVitalize, From: 48, HP: 60},
			{Skill: skillHeal, HP: 60},
			recharge,
		},
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillAgility},
			{Skill: skillHolyWeapon},
			{Skill: skillBlessShield},
			{Skill: skillAdvancedBlock, From: 58},
			{Skill: skillClarity, From: 58, Members: mysticClasses},
			{Skill: skillWildMagic, From: 62, Members: mysticClasses},
		}),
		Melee:      MeleeAloneUnderReserve,
		Base:       []int32{skillMight, skillShield},
		Fixed:      []int32{skillWindStrike},
		Thresholds: healerThresholds,
		PvP:        elvenOraclePvP,
		SpotMargin: marginGlassCannon,
	}

	// shillienElderKit (SE, from SO): Battle Heal, Greater Group Heal from
	// 48 with two members under 60 % ahead of Group Heal, Greater Heal from
	// 48 ahead of Heal; Recharge as the Elven Elder; Wind Shackle once on
	// the leader's target, then Wind Strike; the Oracle's buffs; on the
	// fighter members Guidance and Death Whisper, on the mystic members Wild
	// Magic from 62.
	shillienElderKit = ClassKit{
		Skills: []int32{
			skillHeal, skillBattleHeal, skillGroupHeal, skillGreaterGroupHeal, skillGreaterHeal, skillRecharge,
			skillMight, skillShield, skillWindWalk, skillGreaterEmpower, skillFocus, skillVampiricRage, skillGuidance,
			skillDeathWhisper, skillWildMagic, skillWindShackle, skillWindStrike, skillResurrection, skillDryadRoot,
			skillCursePoison, skillIceBolt, skillMentalShield, skillVampiricTouch,
		},
		Rotation: []Step{
			{Skill: skillWindShackle, Target: TargetLeaderTarget, Opener: true},
			{Skill: skillWindStrike, Target: TargetLeaderTarget},
		},
		Heals: []Heal{
			{Skill: skillBattleHeal, HP: 35},
			{Skill: skillGreaterGroupHeal, From: 48, HP: 60, Members: 2},
			{Skill: skillGroupHeal, HP: 60, Members: 2},
			{Skill: skillGreaterHeal, From: 48, HP: 60},
			{Skill: skillHeal, HP: 60},
			recharge,
		},
		Buffs: slices.Concat(healerBuffs, []Buff{
			{Skill: skillGreaterEmpower},
			{Skill: skillFocus},
			{Skill: skillVampiricRage},
			{Skill: skillGuidance, Members: fighterClasses},
			{Skill: skillDeathWhisper, Members: fighterClasses},
			{Skill: skillWildMagic, From: 62, Members: mysticClasses},
		}),
		Melee: MeleeAloneUnderReserve,
		Base:  []int32{skillMight, skillShield},
		Thresholds: Thresholds{
			CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 50,
			SitHP: 55, SitMP: 30, StandHP: 95, StandMP: 90,
		},
		PvP:        shillienOraclePvP,
		SpotMargin: marginGlassCannon,
	}
)
