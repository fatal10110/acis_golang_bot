package botclient

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The servitor rows DA WK NC EL PS (docs/servitors.md section 5, #761):
// each test compiles the real row for its level and checks the row's key
// behavior, the packets it sends checked against the encoders' bytes.

// TestServitorRowKits checks what each servitor row compiles at 64 in both
// roles: its summons newest first with its Crystal D fallback last, its
// servitor heal and buffs, the ServitorLead flag, its thresholds and its
// spot margins (docs/servitors.md section 5, "Numbers and summary" and
// "The servitor").
func TestServitorRowKits(t *testing.T) {
	summoner := func(mpReserve, sitHP, standHP, critical, crowded, outmatched int) Thresholds {
		return Thresholds{
			CriticalHP: critical, CrowdedHP: crowded, OutmatchedHP: outmatched, MPReserve: mpReserve,
			SitHP: sitHP, SitMP: 30, StandHP: standHP, StandMP: 90,
		}
	}
	tests := []struct {
		name          string
		class         int32
		summons       []int32
		heal          int32
		buffs         []int32
		lead          bool
		toggles       []int32
		thresholds    Thresholds
		margin, withS float64
	}{
		{
			name: "DA", class: classDarkAvenger, summons: []int32{skillSummonDarkPanther},
			toggles: []int32{skillShieldFortress},
			thresholds: Thresholds{
				CriticalHP: 15, CrowdedHP: 60, OutmatchedHP: 30, MPReserve: 20,
				SitHP: 35, SitMP: 20, StandHP: 90, StandMP: 80,
			},
			margin: marginTank,
		},
		{
			name: "WK", class: classWarlock, summons: []int32{skillSummonFelineQueen, skillSummonKaiTheCat, skillSummonKatTheCat},
			heal: skillServitorHeal, buffs: []int32{1140, 1139, 1141, 1144}, lead: true, toggles: []int32{skillTransferPain},
			thresholds: summoner(20, 50, 90, 30, 85, 60), margin: marginGlassCannon, withS: marginNormal,
		},
		{
			name: "NC", class: classNecromancer, heal: skillServitorHeal,
			thresholds: Thresholds{
				CriticalHP: 35, CrowdedHP: 90, OutmatchedHP: 65, MPReserve: 10,
				SitHP: 55, SitMP: 20, StandHP: 95, StandMP: 90,
			},
			margin: marginGlassCannon,
		},
		{
			name: "EL", class: classElementalSummoner, summons: []int32{skillSummonUnicornSeraphim, skillSummonMerrowTheUnicorn, skillSummonBoxerTheUnicorn},
			heal: skillServitorHeal, buffs: []int32{1140, 1139, 1141, 1145}, lead: true, toggles: []int32{skillTransferPain},
			thresholds: summoner(20, 50, 90, 30, 85, 60), margin: marginGlassCannon, withS: marginNormal,
		},
		{
			name: "PS", class: classPhantomSummoner, summons: []int32{skillSummonNightshade, skillSummonSoulless, skillSummonShadow},
			heal: skillServitorHeal, buffs: []int32{1140, 1139, 1141, 1146}, lead: true, toggles: []int32{skillTransferPain},
			thresholds: summoner(20, 55, 95, 35, 90, 65), margin: marginGlassCannon, withS: marginNormal,
		},
	}
	for _, tt := range tests {
		for _, role := range []Role{RoleDamage, RoleSupport} {
			t.Run(tt.name+" "+role.String(), func(t *testing.T) {
				race, _ := datapack.ClassRace(tt.class)
				bd := CompileBuild(race, tt.class, 64, role, BranchChoice{})
				k := bd.Kit
				if !slices.Equal(k.Summons, tt.summons) || k.ServitorHeal != tt.heal || !slices.Equal(k.ServitorBuffs, tt.buffs) || k.ServitorLead != tt.lead {
					t.Errorf("servitor: summons %v heal %d buffs %v lead %v, want %v %d %v %v",
						k.Summons, k.ServitorHeal, k.ServitorBuffs, k.ServitorLead, tt.summons, tt.heal, tt.buffs, tt.lead)
				}
				var toggles []int32
				for _, tg := range k.Toggles {
					toggles = append(toggles, tg.Skill)
				}
				if !slices.Equal(toggles, tt.toggles) {
					t.Errorf("toggles %v, want %v", toggles, tt.toggles)
				}
				if bd.Thresholds != tt.thresholds {
					t.Errorf("thresholds %+v, want %+v", bd.Thresholds, tt.thresholds)
				}
				if k.SpotMargin != tt.margin || k.ServitorSpotMargin != tt.withS {
					t.Errorf("spot margin %v, with a servitor %v; want %v, %v", k.SpotMargin, k.ServitorSpotMargin, tt.margin, tt.withS)
				}
				for _, id := range slices.Concat(k.Summons, k.ServitorBuffs) {
					if !slices.Contains(k.Skills, id) {
						t.Errorf("skill %d is not learned (Skills)", id)
					}
				}
			})
		}
	}
}

// TestRowDA checks the dark avenger: Aggression opens from 400, else Drain
// Health; Aura of Hate with two attackers; Shield Stun only with a
// shield; Judgment from 55; Power Strike; Drain Health under 60 % HP,
// Divine Heal under 40 %, Ultimate Defense under 20 %; Shield Fortress
// from 64, off before it sits; Reflect Damage out of combat.
func TestRowDA(t *testing.T) {
	now := at(time.Second)
	attacks := []int32{skillAggression, skillDrainHealth, skillAuraOfHate, skillShieldStun, skillJudgment, skillPowerStrike}
	sustain := append(slices.Clone(attacks), skillDivineHeal, skillUltimateDefense)
	withFortress := append(slices.Clone(attacks), skillShieldFortress)
	daHP := func(pct int32) func(b *Brain) { return func(b *Brain) { b.world.Self.HP = pct } }
	checkTankFight(t, classDarkAvenger, now, []tankCase{
		{"aggression opens from 400", 40, "SWORD", true, true, attacks, func(b *Brain) { moveNPC(b, 400) }, skillAggression, npcID},
		{"drain health opens while aggression cools", 40, "SWORD", true, true, attacks, func(b *Brain) { hold(b, now, skillAggression) }, skillDrainHealth, npcID},
		{"aura of hate with two attackers", 40, "SWORD", true, false, attacks, hitByTwo, skillAuraOfHate, npcID},
		{"no aura of hate with one attacker", 40, "SWORD", true, false, attacks, nil, skillShieldStun, npcID},
		{"no shield stun without a shield", 40, "SWORD", false, false, attacks, nil, skillPowerStrike, npcID},
		{"judgment at 55", 55, "BLUNT", true, false, attacks, func(b *Brain) { hold(b, now, skillShieldStun) }, skillJudgment, npcID},
		{"no judgment at 54", 54, "BLUNT", true, false, attacks, func(b *Brain) { hold(b, now, skillShieldStun) }, skillPowerStrike, npcID},
		{"drain health under 60 %", 40, "SWORD", true, false, sustain, daHP(55), skillDrainHealth, npcID},
		{"divine heal under 40 %", 40, "SWORD", true, false, sustain, daHP(35), skillDivineHeal, selfID},
		{"ultimate defense under 20 % while divine heal cools", 40, "SWORD", true, false, sustain, func(b *Brain) {
			b.world.Self.HP = 15
			hold(b, now, skillDivineHeal)
		}, skillUltimateDefense, selfID},
		{"shield fortress on at engage from 64", 64, "SWORD", true, true, withFortress, nil, skillShieldFortress, selfID},
		{"no shield fortress at 63", 63, "SWORD", true, true, withFortress, nil, skillAggression, npcID},
	})
	t.Run("shield fortress off before it sits", func(t *testing.T) {
		checkToggleRest(t, classDarkAvenger, 64, skillShieldFortress, now)
	})
	t.Run("reflect damage out of combat", func(t *testing.T) {
		checkIdleBuff(t, classDarkAvenger, 40, []int32{skillReflectDamage}, skillReflectDamage, now)
	})
	t.Run("the aggression frame", func(t *testing.T) {
		b := rowBrain(t, classDarkAvenger, 40, "SWORD", attacks...)
		wearShield(b)
		wantFrame(t, fightCast(t, b, now, skillAggression, npcID), skillAggression)
	})
	t.Run("the panther's order once the pull lands", func(t *testing.T) {
		// No ServitorLead: the order goes first, the opener right after.
		b := rowBrain(t, classDarkAvenger, 40, "SWORD", attacks...)
		wearShield(b)
		b.world.Apply(testServitorInfo, t0)
		a := b.fight(npcID, now)
		if !bytes.Equal(a.payload, servitorAttackFrame) {
			t.Fatalf("first action %v % x, want the attack order", a.kind, a.payload)
		}
		sendNow(t, b, a, now)
		wantFrame(t, fightCast(t, b, now.Add(100*time.Millisecond), skillAggression, npcID), skillAggression)
	})
}

// TestRowWK checks the warlock without a servitor: Blaze from 750, Ice
// Bolt while it cools, Aura Burn within 150; Vampiric Touch under 60 %
// HP, Body To Mind; melee under its 20 % MPReserve.
func TestRowWK(t *testing.T) {
	all := []int32{skillBlaze, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind, skillTransferPain}
	wk := func(name string, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classWarlock, level: 40, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		wk("blaze from 750, no transfer pain without a servitor", func(b *Brain, _ time.Time) {
			moveNPC(b, 750)
			castRange(b, skillBlaze, 750)
		}, skillBlaze, npcID),
		wk("ice bolt while blaze cools", nukeHolding(skillBlaze), skillIceBolt, npcID),
		wk("aura burn within 150", nukeSteps(nukeHolding(skillBlaze, skillIceBolt), func(b *Brain, _ time.Time) { moveNPC(b, 150) }), skillAuraBurn, npcID),
		wk("vampiric touch under 60 % HP", hp(55), skillVampiricTouch, npcID),
		wk("body to mind", func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 20, 80 }, skillBodyToMind, selfID),
		wk("melee under 20 % MP", func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 15, 70 }, 0, 0),
	})
	checkSummonerServitor(t, classWarlock, skillBlaze)
}

// TestRowNC checks the necromancer, a curse nuker without a summon of its
// own: Curse: Poison opens above 60 % MP from 600, else Death Spike from
// 44; Vampiric Claw under 70 % HP and in the rotation; Vampiric Touch
// under 60 %; Blaze once the others cool; melee under its 10 % MPReserve.
func TestRowNC(t *testing.T) {
	all := []int32{
		skillCursePoison, skillDeathSpike, skillVampiricClaw, skillBlaze, skillIceBolt, skillAuraBurn,
		skillVampiricTouch, skillBodyToMind, skillSummonReanimatedMan, skillCorpseLifeDrain, skillCorpseBurst,
	}
	nc := func(name string, level int32, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classNecromancer, level: level, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		nc("curse: poison opens from 600", 44, func(b *Brain, _ time.Time) {
			moveNPC(b, 600)
			castRange(b, skillCursePoison, 600)
		}, skillCursePoison, npcID),
		nc("death spike at 60 % MP", 44, mp(60), skillDeathSpike, npcID),
		nc("no death spike at 43", 43, mp(60), skillVampiricClaw, npcID),
		nc("vampiric claw while death spike cools", 44, nukeSteps(nukeOpened, nukeHolding(skillDeathSpike)), skillVampiricClaw, npcID),
		nc("vampiric claw under 70 % HP", 44, hp(65), skillVampiricClaw, npcID),
		nc("vampiric touch under 60 % HP while the claw cools", 44, nukeSteps(hp(55), nukeHolding(skillVampiricClaw)), skillVampiricTouch, npcID),
		nc("blaze while the others cool", 44, nukeSteps(nukeOpened, nukeHolding(skillDeathSpike, skillVampiricClaw)), skillBlaze, npcID),
		nc("melee under 10 % MP", 44, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 5, 70 }, 0, 0),
	})
	t.Run("no summon without a corpse rule", func(t *testing.T) {
		b := rowBrain(t, classNecromancer, 58, "", skillSummonReanimatedMan, skillSummonCursedMan)
		b.cfg.Travel = rowSummonMap([3]int32{skillSummonCursedMan, skillSummonReanimatedMan, 0})
		carrying(b, reagentStack(crystalC, 99), reagentStack(crystalD, 99))
		if got := b.summonGate(); got != (datapack.SkillRef{}) {
			t.Fatalf("summonGate = %v, want none (#762 casts the corpse summons)", got)
		}
	})
}

// TestRowEL checks the elemental summoner without a servitor: Wind
// Shackle opens above 50 % MP, else Solar Spark from 750, Aqua Swirl, Ice
// Bolt; Self Heal under 40 % HP; melee under its 20 % MPReserve.
func TestRowEL(t *testing.T) {
	all := []int32{skillWindShackle, skillSolarSpark, skillAquaSwirl, skillIceBolt, skillAuraBurn, skillSelfHeal, skillTransferPain}
	el := func(name string, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classElementalSummoner, level: 40, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		el("wind shackle opens", nil, skillWindShackle, npcID),
		el("no wind shackle at 50 % MP: solar spark from 750", nukeSteps(mp(50), func(b *Brain, _ time.Time) {
			moveNPC(b, 750)
			castRange(b, skillSolarSpark, 750)
		}), skillSolarSpark, npcID),
		el("aqua swirl while solar spark cools", nukeSteps(nukeOpened, nukeHolding(skillSolarSpark)), skillAquaSwirl, npcID),
		el("ice bolt while the fixed nukes cool", nukeSteps(nukeOpened, nukeHolding(skillSolarSpark, skillAquaSwirl)), skillIceBolt, npcID),
		el("self heal under 40 % HP", hp(35), skillSelfHeal, selfID),
		el("melee under 20 % MP", nukeSteps(nukeOpened, mp(15)), 0, 0),
	})
	checkSummonerServitor(t, classElementalSummoner, skillSolarSpark)
}

// TestRowPS checks the phantom summoner without a servitor: Wind Shackle
// opens above 50 % MP, else Shadow Spark, Twister, Ice Bolt; Vampiric
// Touch under 70 % HP, Body To Mind; melee under its 20 % MPReserve.
func TestRowPS(t *testing.T) {
	all := []int32{
		skillWindShackle, skillShadowSpark, skillTwister, skillIceBolt, skillAuraBurn, skillVampiricTouch, skillBodyToMind,
		skillTransferPain,
	}
	ps := func(name string, setup func(b *Brain, now time.Time), skill, object int32) mysticCase {
		return mysticCase{name: name, class: classPhantomSummoner, level: 40, role: RoleDamage, skills: all, setup: setup, skill: skill, object: object}
	}
	runMysticCases(t, "", []mysticCase{
		ps("wind shackle opens", nil, skillWindShackle, npcID),
		ps("shadow spark once opened", nukeOpened, skillShadowSpark, npcID),
		ps("twister while shadow spark cools", nukeSteps(nukeOpened, nukeHolding(skillShadowSpark)), skillTwister, npcID),
		ps("vampiric touch under 70 % HP", hp(65), skillVampiricTouch, npcID),
		ps("body to mind", func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 20, 80 }, skillBodyToMind, selfID),
		ps("melee under 20 % MP", nukeSteps(nukeOpened, func(b *Brain, _ time.Time) { b.world.Self.MP, b.world.Self.HP = 15, 70 }), 0, 0),
	})
	checkSummonerServitor(t, classPhantomSummoner, skillShadowSpark)
}

// checkSummonerServitor checks a mystic summoner row of class at 40 with
// its servitor out: the order goes first, then Transfer Pain goes on,
// and the first nuke (nuke) waits for ServitorLead; Transfer Pain stays off without a servitor and goes off once it
// dies; the spot margin is normal with the servitor out, glass cannon
// without.
func checkSummonerServitor(t *testing.T, class, nuke int32) {
	t.Helper()
	now := at(time.Second)
	known := []int32{nuke, skillTransferPain}
	brain := func(t *testing.T) *Brain {
		b := mysticBrain(t, class, 40, RoleDamage, "", known...)
		hold(b, now, skillWindShackle) // the shackle opener aside
		b.world.Apply(testServitorInfo, t0)
		return b
	}
	t.Run("servitor lead, then transfer pain, then the nuke", func(t *testing.T) {
		b := brain(t)
		a := b.fight(npcID, now)
		if !bytes.Equal(a.payload, servitorAttackFrame) {
			t.Fatalf("first action %v % x, want the attack order", a.kind, a.payload)
		}
		sendNow(t, b, a, now)
		// The toggle is no attack: it goes on inside the lead.
		wantFrame(t, fightCast(t, b, now, skillTransferPain, selfID), skillTransferPain)
		togglesOn(b, skillTransferPain)
		if a := b.fight(npcID, now.Add(time.Second)); a.kind == actCast {
			t.Fatalf("cast %d inside ServitorLead, want a hold", a.skill)
		}
		lead := now.Add(b.persona.ServitorLead)
		wantFrame(t, fightCast(t, b, lead, nuke, npcID), nuke)
	})
	t.Run("no transfer pain without a servitor", func(t *testing.T) {
		b := mysticBrain(t, class, 40, RoleDamage, "", known...)
		hold(b, now, skillWindShackle)
		b.opened = npcID
		fightCast(t, b, now, nuke, npcID)
	})
	t.Run("transfer pain off once the servitor dies", func(t *testing.T) {
		b := brain(t)
		b.opened, b.engaged = npcID, npcID
		togglesOn(b, skillTransferPain)
		b.world.Servitor.Dead = true
		fightCast(t, b, now, skillTransferPain, selfID)
	})
	t.Run("spot margin", func(t *testing.T) {
		b := mysticBrain(t, class, 40, RoleDamage, "")
		withServitorTemplates(b)
		if bd := b.build(); b.spotMargin(&bd) != marginGlassCannon {
			t.Errorf("spot margin without a servitor %v, want %v", b.spotMargin(&bd), marginGlassCannon)
		}
		kaiOut(b)
		if bd := b.build(); b.spotMargin(&bd) != marginNormal {
			t.Errorf("spot margin with a servitor out %v, want %v", b.spotMargin(&bd), marginNormal)
		}
	})
}

// rowSummonMap is a travel map with summons' templates: own (Crystal C,
// XP penalty 0.1) newest first, then fallback (Crystal D, 0.3), 0 for
// none; every one costs 1 crystal a cast and an upkeep step.
func rowSummonMap(ids [3]int32) *TravelMap {
	skills := make(map[datapack.SkillRef]datapack.Skill)
	for i, id := range ids {
		if id == 0 {
			continue
		}
		crystal, penalty := crystalC, 0.1
		if i == 2 {
			crystal, penalty = crystalD, 0.3
		}
		skills[datapack.SkillRef{ID: id, Level: 1}] = datapack.Skill{
			Type: "SUMMON", Target: "SELF", NPCID: kaiCat.ID, MPConsume: 50, ExpPenalty: penalty,
			ItemConsumeID: crystal, ItemConsumeCount: 1, ItemConsumeIDOT: crystal, ItemConsumeCountOT: 1, ItemConsumeSteps: 14,
			SummonLifetime: 1200 * time.Second,
		}
	}
	return &TravelMap{skills: skills, reagents: indexReagents(skills), servitors: map[int32]datapack.NPC{kaiCat.ID: kaiCat}}
}

// TestServitorRowSummons checks the summon each servitor row casts
// (summonGate over its compiled Summons): its newest summon it knows and
// can pay for, its older one before it learns the newest, its
// first-profession Crystal D summon with Crystal D only (D51 answer 1);
// the Dark Avenger's panther on Crystal C only.
func TestServitorRowSummons(t *testing.T) {
	rows := []struct {
		name  string
		class int32
		ids   [3]int32 // newest, older, the Crystal D fallback
	}{
		{"WK", classWarlock, [3]int32{skillSummonFelineQueen, skillSummonKaiTheCat, skillSummonKatTheCat}},
		{"EL", classElementalSummoner, [3]int32{skillSummonUnicornSeraphim, skillSummonMerrowTheUnicorn, skillSummonBoxerTheUnicorn}},
		{"PS", classPhantomSummoner, [3]int32{skillSummonNightshade, skillSummonSoulless, skillSummonShadow}},
		{"DA", classDarkAvenger, [3]int32{skillSummonDarkPanther, 0, 0}},
	}
	for _, r := range rows {
		cases := []struct {
			name  string
			known []int32
			items []Item
			want  int32
		}{
			{"the newest with Crystal C", r.ids[:], []Item{reagentStack(crystalC, 9), reagentStack(crystalD, 9)}, r.ids[0]},
			{"the older before the newest is learned", r.ids[1:], []Item{reagentStack(crystalC, 9)}, r.ids[1]},
			{"the fallback with Crystal D only", r.ids[:], []Item{reagentStack(crystalD, 9)}, r.ids[2]},
			{"none without crystals", r.ids[:], nil, 0},
		}
		for _, tc := range cases {
			t.Run(r.name+" "+tc.name, func(t *testing.T) {
				b := rowBrain(t, r.class, 58, "", tc.known...)
				b.world.Self.InCombat = false
				b.cfg.Travel = rowSummonMap(r.ids)
				carrying(b, tc.items...)
				got := b.summonGate()
				if got.ID != tc.want {
					t.Fatalf("summonGate = %v, want summon %d", got, tc.want)
				}
			})
		}
	}
}

// TestServitorRowRecordedSummon replays the recorded Kat the Cat summon
// (testdata/servitor_summon, TestServitorRecordedReplay) into a level 40
// warlock: Kat the Cat is its Crystal D fallback, and with it out the
// row switches its spot margin to normal and puts Transfer Pain on in a
// fight.
func TestServitorRowRecordedSummon(t *testing.T) {
	frames := recordedServitorFrames(t)
	var pet PetInfo
	for _, ev := range frames {
		if p, ok := ev.(PetInfo); ok {
			pet = p
			break
		}
	}
	if pet.ObjectID == 0 {
		t.Fatal("no PetInfo in the recording")
	}
	b := mysticBrain(t, classWarlock, 40, RoleDamage, "", skillBlaze, skillTransferPain)
	kat := kaiCat
	kat.ID = pet.TemplateID
	b.cfg.Travel.servitors = map[int32]datapack.NPC{kat.ID: kat}
	pet.Pos = Point{X: 50, Y: 50}
	b.world.Apply(pet, t0)
	if s := b.liveServitor(); s == nil || s.TemplateID != 14115 {
		t.Fatalf("servitor %+v, want the recorded Kat the Cat", b.world.Servitor)
	}
	if bd := b.build(); b.spotMargin(&bd) != marginNormal {
		t.Fatalf("spot margin %v, want normal with the servitor out", b.spotMargin(&bd))
	}
	now := at(time.Second)
	a := b.fight(npcID, now)
	if !bytes.Equal(a.payload, servitorAttackFrame) {
		t.Fatalf("first action %v % x, want the attack order", a.kind, a.payload)
	}
	sendNow(t, b, a, now)
	wantFrame(t, fightCast(t, b, now, skillTransferPain, selfID), skillTransferPain)
}

// recordedServitorFrames are the decoded frames of the recorded summon.
func recordedServitorFrames(t *testing.T) []Event {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "servitor_summon", "*.bin"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("recorded frames: %d, %v", len(paths), err)
	}
	evs := make([]Event, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ev, ok, err := botwire.DecodeEvent(raw)
		if err != nil || !ok {
			t.Fatalf("%s: DecodeEvent = %v, %v", filepath.Base(path), ok, err)
		}
		evs = append(evs, ev)
	}
	return evs
}

// wantFrame fails unless a's frame is the cast request of skill.
func wantFrame(t *testing.T, a Action, skill int32) {
	t.Helper()
	if want := encodeCast(skill); !bytes.Equal(a.payload, want) {
		t.Fatalf("frame % x, want the cast of %d % x", a.payload, skill, want)
	}
}

// preServitorTables are the branch tables before the servitor lines
// (#761), copied: with ServitorLines off the tables must stay these.
var preServitorTables = map[Role]map[int32][]line{
	RoleSupport: {
		0: {{4, 5, 1}}, 18: {{19, 20, 1}}, 31: {{32, 33, 1}}, 44: {{45, 46, 1}}, 53: {{54, 55, 1}},
		10: {{15, 16, 2}, {15, 17, 1}}, 25: {{29, 30, 1}}, 38: {{42, 43, 1}}, 49: {{50, 52, 1}},
	},
	RoleDamage: {
		0:  {{1, 2, 2}, {1, 3, 1}, {7, 8, 1}, {7, 9, 1}},
		18: {{19, 21, 1}, {22, 23, 1}, {22, 24, 1}},
		31: {{32, 34, 1}, {35, 36, 2}, {35, 37, 1}},
		44: {{45, 46, 2}, {47, 48, 1}},
		53: {{54, 55, 1}},
		10: {{11, 12, 1}}, 25: {{26, 27, 1}}, 38: {{39, 40, 1}}, 49: {{50, 52, 1}},
	},
	RoleCrafter: {53: {{classArtisan, classWarsmith, 1}}},
}

// TestBranchServitorLines checks the persona switch ServitorLines
// (docs/servitors.md section 5 "Branch tables", D51 answer 2): off, the
// tables are those before the servitor lines and every draw of every base
// class and its first profession is theirs; on, the four changed entries
// draw by their weights (Paladin 2, Dark Avenger 1; Sorcerer 2,
// Necromancer 1, Warlock 1; Spellsinger 2, Elemental Summoner 1;
// Spellhowler 2, Phantom Summoner 1), every other draw is the same, and
// the branch pins do not change.
func TestBranchServitorLines(t *testing.T) {
	if !reflect.DeepEqual(branchTables, preServitorTables) {
		t.Fatalf("branchTables changed:\n%v\nwant\n%v", branchTables, preServitorTables)
	}
	if reflect.ValueOf((BranchChoice{}).tables()).Pointer() != reflect.ValueOf(branchTables).Pointer() {
		t.Fatal("a choice with ServitorLines off does not draw from branchTables")
	}
	changed := map[Role]map[int32][]line{
		RoleSupport: {0: {{4, 5, 2}, {4, 6, 1}}},
		RoleDamage: {
			10: {{11, 12, 2}, {11, 13, 1}, {11, 14, 1}},
			25: {{26, 27, 2}, {26, 28, 1}},
			38: {{39, 40, 2}, {39, 41, 1}},
		},
	}
	if !reflect.DeepEqual(servitorBranches, changed) {
		t.Fatalf("servitorBranches = %v, want %v", servitorBranches, changed)
	}
	for r, tab := range branchTables {
		for base, ls := range tab {
			want := ls
			if c, ok := changed[r][base]; ok {
				want = c
			}
			if got := servitorTables[r][base]; !slices.Equal(got, want) {
				t.Errorf("%v %d: servitor table %v, want %v", r, base, got, want)
			}
		}
		if !slices.Equal(slices.Sorted(maps.Keys(servitorTables[r])), slices.Sorted(maps.Keys(tab))) {
			t.Errorf("%v: servitor table bases differ from branchTables'", r)
		}
	}
	// On: drawLine over every r below the total picks each line its
	// weight's times.
	for r, tab := range changed {
		for base, ls := range tab {
			kept := keptLines(servitorTables, r, ls[0].first)
			if !slices.Equal(kept, ls) {
				t.Errorf("%v %d: kept lines %v, want %v", r, base, kept, ls)
			}
			counts := make(map[int32]uint64)
			var total uint64
			for _, l := range ls {
				total += l.weight
			}
			for x := range total {
				l, _ := drawLine(kept, x)
				counts[l.second]++
			}
			for _, l := range ls {
				if counts[l.second] != l.weight {
					t.Errorf("%v %d: %d drawn %d of %d, want %d", r, base, l.second, counts[l.second], total, l.weight)
				}
			}
		}
	}
	// Draws across accounts: off is the old table's, on differs only in
	// the changed entries and reaches every new second profession.
	reached := make(map[int32]bool)
	bases := []int32{0, 10, 18, 25, 31, 38, 44, 49, 53}
	for i := range 2000 {
		acct := "acct" + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676))
		for _, role := range []Role{RoleSupport, RoleDamage} {
			for _, base := range bases {
				off, okOff := chooseLine(base, role, BranchChoice{Account: acct})
				on, okOn := chooseLine(base, role, BranchChoice{Account: acct, ServitorLines: true})
				if !okOff || !okOn {
					t.Fatalf("%v %d: no line (off %v, on %v)", role, base, okOff, okOn)
				}
				if _, isChanged := changed[role][base]; !isChanged && on != off {
					t.Fatalf("%v %d %s: on %+v, off %+v: an unchanged entry differs", role, base, acct, on, off)
				}
				if !slices.Contains(preServitorTables[role][base], off) && !slices.Contains(preServitorTables[RoleDamage][base], off) {
					t.Fatalf("%v %d %s: off draws %+v, not a line of the old tables", role, base, acct, off)
				}
				reached[on.second] = true
				// The first profession keeps its line's draw.
				atFirst, _ := chooseLine(on.first, role, BranchChoice{Account: acct, ServitorLines: true})
				if atFirst != on {
					t.Fatalf("%v %d %s: base draw %+v, at its first profession %+v", role, base, acct, on, atFirst)
				}
			}
		}
	}
	for _, c := range []int32{classDarkAvenger, classNecromancer, classWarlock, classElementalSummoner, classPhantomSummoner} {
		if !reached[c] {
			t.Errorf("class %d never drawn with ServitorLines on", c)
		}
	}
	for base := range preServitorTables[RoleSupport] {
		var firsts []int32
		for _, r := range tableOrder {
			for _, l := range servitorTables[r][base] {
				if !slices.Contains(firsts, l.first) {
					firsts = append(firsts, l.first)
				}
			}
		}
		slices.Sort(firsts)
		if pins := BranchPins(base); !slices.Equal(pins, firsts) {
			t.Errorf("base %d: pins %v, the servitor tables' firsts %v", base, pins, firsts)
		}
	}
}

// TestBuildServitorLines checks the brain recompiles its build when the
// persona switches ServitorLines: a Human Wizard whose account draws the
// Necromancer with it on goes on to the Sorcerer with it off.
func TestBuildServitorLines(t *testing.T) {
	var acct string
	for i := range 1000 {
		a := "wizard" + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
		if l, _ := chooseLine(11, RoleDamage, BranchChoice{Account: a, ServitorLines: true}); l.second == classNecromancer {
			acct = a
			break
		}
	}
	if acct == "" {
		t.Fatal("no account draws the Necromancer")
	}
	b := mysticBrain(t, 11, 39, RoleDamage, "")
	b.cfg.account = acct
	if got := b.build().Race.Professions[11]; got != classSorcerer {
		t.Fatalf("switch off: next profession %d, want the Sorcerer", got)
	}
	b.persona.ServitorLines = true
	if got := b.build().Race.Professions[11]; got != classNecromancer {
		t.Fatalf("switch on: next profession %d, want the Necromancer", got)
	}
	if DefaultPersona().ServitorLines {
		t.Fatal("DefaultPersona turns ServitorLines on, want off (D51 answer 2)")
	}
}
