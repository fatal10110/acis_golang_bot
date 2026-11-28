package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Party PvP fixtures (docs/pvp/party.md): a player's party led by the GM
// commander (gmID) with a pvp damage bot, a support KN and a support CL; two
// flagged enemies, foeName (pcID, a Human Fighter: Melee) and ptyFoe2 (a
// Human Mystic: Caster), and a third, ptyFoe3, a Cleric (the Healer prior).
const (
	ptyKN   = int32(0x30000003)
	ptyCL   = int32(0x30000004)
	ptyDmg  = int32(0x30000005)
	ptyFoe2 = int32(0x20000006)
	ptyFoe3 = int32(0x20000007)
)

// Where the party fixtures stand: the bot at the origin, the GM behind it,
// the foes in front.
var (
	ptyGMPos   = Point{Y: -200}
	ptyFoe1Pos = Point{Y: 300}
	ptyFoe2Pos = Point{X: 300, Y: 300}
	ptyFoe3Pos = Point{X: -300, Y: 300}
)

// partySkills are the datapack entries the party tests cast or see cast:
// 1015 Battle Heal (HEAL), 1027 Group Heal (HEAL, target PARTY), 1035 Mental
// Shield (a target ONE buff, hit 4000), 28 Aggression (AGGDAMAGE with TargetMe) and 112
// Deflect Arrow (a target SELF buff).
var partySkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillBattleHeal, Level: 1}:   {Type: "HEAL", Target: "ONE", Magic: true, HitTime: time.Second, Power: 100},
	{ID: skillMentalShield, Level: 1}: {Type: "BUFF", Target: "ONE", Magic: true, HitTime: 4 * time.Second},
	{ID: skillAggression, Level: 1}:   {Type: "AGGDAMAGE", Target: "ONE", Offensive: true, HitTime: time.Second, Effects: []datapack.Effect{{Name: "TargetMe"}}},
	{ID: skillGroupHeal, Level: 1}:    {Type: "HEAL", Target: "PARTY", Magic: true, HitTime: time.Second, Power: 100},
	{ID: skillDeflectArrow, Level: 1}: {Type: "BUFF", Target: "SELF", HitTime: time.Second},
}

// ptyRow is a party window row: id named name of class at hp of 100.
func ptyRow(id int32, name string, class, hp int32) PartyMember {
	return PartyMember{ObjectID: id, Name: name, HP: hp, MaxHP: 100, ClassID: class}
}

// ptyHit is attacker's hit on target, swung from at.
func ptyHit(attacker, target int32, at Point) Attack {
	return Attack{AttackerID: attacker, Pos: at, Hits: []AttackHit{{TargetID: target, Damage: 10}}}
}

// ptyHeal is caster's Battle Heal on target, cast from at.
func ptyHeal(caster, target int32, at Point) MagicSkillUse {
	return MagicSkillUse{CasterID: caster, TargetID: target, SkillID: skillBattleHeal, Level: 1, HitTime: time.Second, CasterPos: at}
}

// partyBrain is a level 30 bot of class, persona edited by edit, kit as its
// build knowing every skill of it, in the GM's party with rows (each in
// sight, flagged when listed in flagged) and the flagged foes in sight. Its
// regular tick is pushed past the test, so only the test decides.
func partyBrain(t *testing.T, class int32, edit func(*Persona), kit ClassKit, rows []PartyMember, flagged ...int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	if edit != nil {
		edit(&p)
	}
	b, _ := newTestBrain(t, p)
	b.cfg.Travel = pvpTravel(false)
	for ref, s := range layerSkills {
		b.cfg.Travel.skills[ref] = s
	}
	for ref, s := range partySkills {
		b.cfg.Travel.skills[ref] = s
	}
	s := &b.world.Self
	s.ClassID, s.Level = class, 30
	var known []KnownSkill
	for _, st := range append(append([]Step{}, kit.Rotation...), kit.PvP...) {
		known = append(known, KnownSkill{ID: st.Skill, Level: 1})
	}
	for _, h := range kit.Heals {
		known = append(known, KnownSkill{ID: h.Skill, Level: 1})
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	withKit(b, kit)
	pos := map[int32]Point{gmID: ptyGMPos, ptyKN: {X: 100}, ptyCL: {X: -100}, ptyDmg: {X: 50, Y: -50}}
	for _, r := range rows {
		var flag int32
		for _, f := range flagged {
			if f == r.ObjectID {
				flag = 1
			}
		}
		b.handleOne(t, CharInfo{ObjectID: r.ObjectID, Name: r.Name, ClassID: r.ClassID, Pos: pos[r.ObjectID], PvPFlag: flag}, t0)
	}
	b.handleOne(t, PartySmallWindowAll{LeaderID: gmID, Members: rows}, t0)
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 0, Pos: ptyFoe1Pos, PvPFlag: 1}, t0)
	b.handleOne(t, CharInfo{ObjectID: ptyFoe2, Name: "foe2", ClassID: 10, Pos: ptyFoe2Pos, PvPFlag: 1}, t0)
	b.nextTick = t0.Add(time.Hour)
	b.reactAt = time.Time{}
	return b
}

// ptyParty is the GM's party seen by a member other than self: the GM, the
// pvp damage bot, the KN and the CL, all at full HP.
func ptyParty(self int32) []PartyMember {
	var out []PartyMember
	for _, r := range []PartyMember{ptyRow(gmID, "GM", 88, 100), ptyRow(ptyDmg, "dmg", 10, 100), ptyRow(ptyKN, "kn", 4, 100), ptyRow(ptyCL, "cl", 15, 100)} {
		if r.ObjectID != self {
			out = append(out, r)
		}
	}
	return out
}

// pvpPolicy is the pvp preset's PvP policy as the persona edit.
func pvpPolicy(p *Persona) {
	p.PvP = PvPPolicy{FightFlagged: true, Duels: true, Hunt: true, Arenas: true, SupportFlagged: true}
}

// wantDecision decides b at now and checks the rule, kind, object and skill
// (a cast's) of the action.
func wantDecision(t *testing.T, b *Brain, now time.Time, rule string, kind actionKind, object, skill int32) {
	t.Helper()
	got, a := b.decide(now)
	if got != rule || a.kind != kind || a.object != object || kind == actCast && a.skill != skill {
		t.Fatalf("decision rule %q kind %v object %#x skill %d, want rule %q kind %v object %#x skill %d", got, a.kind, a.object, a.skill, rule, kind, object, skill)
	}
}

// TestPvPTargetHealer checks party PvP (docs/pvp/party.md, X9) in the GM's
// party of a pvp damage bot, a support KN and a support CL hit by two
// flagged enemies, and a support CL alone with the GM.
func TestPvPTargetHealer(t *testing.T) {
	dmgKit := ClassKit{Rotation: []Step{{Skill: lyrNuke}}}
	t.Run("the pvp bot switches to the enemy healer", func(t *testing.T) {
		b := partyBrain(t, 10, pvpPolicy, dmgKit, ptyParty(selfID))
		b.handleOne(t, ptyHit(ptyFoe2, ptyCL, ptyFoe2Pos), at(time.Second))
		if b.reactAt.IsZero() {
			t.Error("a hit on a member did not wake the bot")
		}
		b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		wantDecision(t, b, at(2500*time.Millisecond), "party fight", actSelect, pcID, 0)
		b.handleOne(t, ptyHeal(ptyFoe2, pcID, ptyFoe2Pos), at(3*time.Second))
		if r := b.pvp.opponents[ptyFoe2]; r == nil || !r.HealedAt.Equal(at(3*time.Second)) {
			t.Fatal("Battle Heal on an enemy did not mark the enemy healer")
		}
		wantDecision(t, b, at(3500*time.Millisecond), "party fight", actSelect, ptyFoe2, 0)
		b.world.Target = ptyFoe2
		wantDecision(t, b, at(3600*time.Millisecond), "party fight", actCast, ptyFoe2, lyrNuke)
	})
	knKit := ClassKit{PvP: []Step{{Skill: skillAggression, Peel: true}}}
	for _, tc := range []struct {
		name   string
		victim int32
		peel   bool
	}{
		{"the KN peels the enemy hitting the CL", ptyCL, true},
		{"the KN peels the enemy hitting the leader", gmID, true},
		{"no peel for the enemy hitting the damage bot", ptyDmg, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := partyBrain(t, 4, nil, knKit, ptyParty(ptyKN))
			b.handleOne(t, ptyHit(pcID, tc.victim, ptyFoe1Pos), at(time.Second))
			wantDecision(t, b, at(1500*time.Millisecond), "party fight", actSelect, pcID, 0)
			b.world.Target = pcID
			if got := castOf(t, b, at(1600*time.Millisecond)); (got == skillAggression) != tc.peel {
				t.Errorf("cast %d, want Aggression %v", got, tc.peel)
			}
		})
	}
	sleepKit := ClassKit{Rotation: []Step{{Skill: lyrSleep}, {Skill: lyrNuke}}}
	for _, tc := range []struct {
		name string
		dot  int32
		want bool
	}{
		{"a sleep skips the healer being hit and sleeps the other enemy", 0, true},
		{"no sleep on an enemy carrying a DOT", 0x2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := partyBrain(t, 10, pvpPolicy, sleepKit, ptyParty(selfID))
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 0, Pos: ptyFoe1Pos, PvPFlag: 1, Abnormal: tc.dot}, t0)
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
			b.handleOne(t, ptyHit(ptyFoe2, ptyCL, ptyFoe2Pos), at(time.Second))
			b.handleOne(t, ptyHeal(ptyFoe2, pcID, ptyFoe2Pos), at(2*time.Second))
			b.handleOne(t, ptyHit(gmID, ptyFoe2, ptyGMPos), at(2*time.Second))
			b.world.Target = ptyFoe2
			if !tc.want {
				wantDecision(t, b, at(2500*time.Millisecond), "party fight", actCast, ptyFoe2, lyrNuke)
				return
			}
			wantDecision(t, b, at(2500*time.Millisecond), "party fight", actSelect, pcID, 0)
			b.world.Target = pcID
			wantDecision(t, b, at(2600*time.Millisecond), "party fight", actCast, pcID, lyrSleep)
		})
	}
	clKit := ClassKit{
		Heals: []Heal{{Skill: skillHeal, HP: 60}},
		PvP:   []Step{{Skill: skillMentalShield, Target: TargetSelf, Vs: mysticVsCB}},
	}
	shielded := []Effect{{SkillID: skillMentalShield, Level: 1, Left: time.Hour}}
	for _, tc := range []struct {
		name    string
		rows    []PartyMember
		flagged int32
		support bool
		shield  bool
		rule    string
		kind    actionKind
		object  int32
	}{
		{"the CL leaves the flagged damage bot unhealed with SupportFlagged off", ptyCLParty(ptyDmg), ptyDmg, false, false, "party buff", actSelect, gmID},
		{"the CL heals the flagged damage bot with SupportFlagged on", ptyCLParty(ptyDmg), ptyDmg, true, false, "heal", actSelect, ptyDmg},
		{"Mental Shield skips the flagged damage bot with SupportFlagged off", ptyCLParty(ptyDmg), ptyDmg, false, true, "party fight", actMove, 0},
		{"Mental Shield on the flagged damage bot with SupportFlagged on", ptyParty(ptyCL), ptyDmg, true, true, "party buff", actSelect, ptyDmg},
		{"the CL alone leaves the flagged leader unhealed with SupportFlagged off", []PartyMember{ptyRow(gmID, "GM", 88, 20)}, gmID, false, false, "party buff", actSelect, selfID},
		{"the CL alone heals the flagged leader with SupportFlagged on", []PartyMember{ptyRow(gmID, "GM", 88, 20)}, gmID, true, false, "heal", actSelect, gmID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edit := func(p *Persona) { p.PvP.SupportFlagged = tc.support }
			b := partyBrain(t, 15, edit, clKit, tc.rows, tc.flagged)
			if tc.shield {
				b.handleOne(t, AbnormalStatusUpdate{Effects: shielded}, t0)
				for _, r := range tc.rows {
					if r.ObjectID != tc.flagged {
						b.handleOne(t, PartySpelled{ObjectID: r.ObjectID, Effects: shielded}, t0)
					}
				}
			}
			b.handleOne(t, ptyHit(ptyFoe2, tc.flagged, ptyFoe2Pos), at(time.Second))
			wantDecision(t, b, at(1500*time.Millisecond), tc.rule, tc.kind, tc.object, 0)
			if tc.rule == "party buff" {
				b.world.Target = tc.object
				wantDecision(t, b, at(1600*time.Millisecond), tc.rule, actCast, tc.object, skillMentalShield)
			}
		})
	}
}

// ptyCLParty is the GM's party seen by the CL with the damage bot under 30 %
// and every other member at full HP; the member wounded is wounded (the GM or
// the damage bot).
func ptyCLParty(wounded int32) []PartyMember {
	out := ptyParty(ptyCL)
	for i := range out {
		if out[i].ObjectID == wounded {
			out[i].HP = 20
		}
	}
	return out
}

// TestPvPPartyFoe checks the party's choice of enemy (docs/pvp/party.md "The
// healer first"): the latest attacker of the party, the Healer prior within
// reach, the attacker of a member under 30 %, the leader's target and a
// summon's owner.
func TestPvPPartyFoe(t *testing.T) {
	cleric := func(b *Brain, pos Point) {
		b.handleOne(t, CharInfo{ObjectID: ptyFoe3, Name: "foe3", ClassID: 15, Pos: pos, PvPFlag: 1}, t0)
		b.handleOne(t, ptyHit(ptyFoe3, ptyDmg, pos), at(time.Second))
	}
	tests := []struct {
		name  string
		rows  []PartyMember
		setup func(b *Brain)
		want  int32
	}{
		{"the latest attacker", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, ptyHit(ptyFoe2, ptyKN, ptyFoe2Pos), at(time.Second))
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		}, pcID},
		{"the Healer prior within reach", ptyParty(ptyCL), func(b *Brain) {
			cleric(b, ptyFoe3Pos)
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		}, ptyFoe3},
		{"a group heal on an enemy marks its healer", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
			// The server names the caster as a target PARTY skill's target
			// (handler/target/group.go partyHandler.FinalTarget) and its
			// party in the launch.
			b.handleOne(t, MagicSkillUse{CasterID: ptyFoe2, TargetID: ptyFoe2, SkillID: skillGroupHeal, Level: 1, HitTime: time.Second, CasterPos: ptyFoe2Pos}, at(2*time.Second))
			if r := b.pvp.opponents[ptyFoe2]; !r.HealedAt.IsZero() {
				t.Fatal("a group heal's start marked the enemy healer")
			}
			b.handleOne(t, MagicSkillLaunched{CasterID: ptyFoe2, SkillID: skillGroupHeal, Level: 1, Targets: []int32{ptyFoe2, pcID}}, at(2500*time.Millisecond))
		}, ptyFoe2},
		{"a group heal on the healer alone marks no healer", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
			b.handleOne(t, MagicSkillLaunched{CasterID: ptyFoe2, SkillID: skillGroupHeal, Level: 1, Targets: []int32{ptyFoe2}}, at(2500*time.Millisecond))
		}, pcID},
		{"no Healer prior out of reach", ptyParty(ptyCL), func(b *Brain) {
			cleric(b, Point{X: -3000, Y: 300})
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		}, pcID},
		{"the attacker of a member under 30 %", ptyCLParty(ptyKN), func(b *Brain) {
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
			b.handleOne(t, ptyHit(ptyFoe2, gmID, ptyFoe2Pos), at(2*time.Second))
		}, pcID},
		{"the leader's target", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, ptyHit(gmID, ptyFoe2, ptyGMPos), at(time.Second))
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		}, ptyFoe2},
		{"a summon's owner", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, NPCInfo{ObjectID: pvpSummon, Summon: true, Title: foeName, PvPFlag: 1, Pos: ptyFoe1Pos}, t0)
			b.handleOne(t, ptyHit(pvpSummon, ptyKN, ptyFoe1Pos), at(time.Second))
		}, pcID},
		{"a player the bot may not fight", ptyParty(ptyCL), func(b *Brain) {
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: ptyFoe1Pos}, t0)
			b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
		}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			edit := func(p *Persona) {
				pvpPolicy(p)
				p.AssistMin, p.AssistMax = 100*time.Millisecond, 200*time.Millisecond
			}
			b := partyBrain(t, 10, edit, ClassKit{Rotation: []Step{{Skill: lyrNuke}}}, tc.rows)
			tc.setup(b)
			if got := b.partyFoe(at(3 * time.Second)); got != tc.want {
				t.Errorf("foe %#x, want %#x", got, tc.want)
			}
		})
	}
}

// TestPvPHealerStand checks the bot's healer stands behind the leader, away
// from the enemies, once it has nothing to heal or buff
// (docs/pvp/party.md "The bot's healer").
func TestPvPHealerStand(t *testing.T) {
	kit := ClassKit{Heals: []Heal{{Skill: skillHeal, HP: 60}}}
	b := partyBrain(t, 15, nil, kit, ptyParty(ptyCL))
	b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(time.Second))
	got, a := b.decide(at(1500 * time.Millisecond))
	if got != "party fight" || a.kind != actMove {
		t.Fatalf("decision rule %q kind %v, want a party fight move", got, a.kind)
	}
	if to := moveTarget(a.payload); to.X != 0 || to.Y >= ptyGMPos.Y {
		t.Errorf("stands at %v, want behind the GM at %v, away from the foe at %v", to, ptyGMPos, ptyFoe1Pos)
	}
	if row, _ := charmodel.ClassRow(15, false); row.Archetype() != charmodel.Healer {
		t.Fatal("the Cleric fixture plays no Healer row")
	}
}

// TestPvPPartyBuffSelf checks a non-healer's PvP self buff in a party fight
// (docs/pvp/party.md "The bot's healer"): a KN puts up 112 Deflect Arrow on
// itself while an archer of the enemy set hits a member, and casts it again
// no sooner than buffRetry.
func TestPvPPartyBuffSelf(t *testing.T) {
	kit := ClassKit{PvP: []Step{{Skill: skillDeflectArrow, Target: TargetSelf, Vs: []charmodel.Archetype{charmodel.Archer}}}}
	b := partyBrain(t, 4, nil, kit, ptyParty(ptyKN))
	asArcher(b)
	b.handleOne(t, ptyHit(pcID, ptyCL, ptyFoe1Pos), at(time.Second))
	now := at(1500 * time.Millisecond)
	got, a := b.decide(now)
	if got != "party buff" || a.kind != actCast || a.object != selfID || a.skill != skillDeflectArrow {
		t.Fatalf("decision rule %q kind %v object %#x skill %d, want Deflect Arrow on self", got, a.kind, a.object, a.skill)
	}
	if ok, err := b.send(a, now); !ok || err != nil {
		t.Fatalf("send = %v, %v", ok, err)
	}
	if got := b.buffedAt[buffKey{selfID, skillDeflectArrow}]; !got.Equal(now) {
		t.Errorf("buffedAt = %v, want the cast time %v", got, now)
	}
	// The cast's hold has run out, the buff did not land: buffRetry holds it.
	later := now.Add(castTimeout + time.Second)
	if got, a := b.decide(later); got == "party buff" {
		t.Errorf("decision %q skill %d within buffRetry, want no party buff", got, a.skill)
	}
	b.handleOne(t, ptyHit(pcID, ptyCL, ptyFoe1Pos), now.Add(buffRetry-time.Second))
	if got, a := b.decide(now.Add(buffRetry)); got != "party buff" || a.skill != skillDeflectArrow {
		t.Errorf("decision %q skill %d after buffRetry, want Deflect Arrow", got, a.skill)
	}
}

// TestPvPOutnumberedParty checks the outnumbered rule hands a party fight to
// "party fight" (docs/pvp/party.md "The healer first"): two enemies hitting
// the bot itself, the bot fights the marked enemy healer rather than its
// hardest hitter; the bot's healer still fights back as outnumbered.
func TestPvPOutnumberedParty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		class  int32
		kit    ClassKit
		rule   string
		object int32
	}{
		{"a damage bot fights the enemy healer", 10, ClassKit{Rotation: []Step{{Skill: lyrNuke}}}, "party fight", ptyFoe3},
		{"the bot's healer fights back outnumbered", 15, ClassKit{Heals: []Heal{{Skill: skillHeal, HP: 60}}}, "outnumbered", pcID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := partyBrain(t, tc.class, pvpPolicy, tc.kit, ptyParty(selfID))
			b.handleOne(t, CharInfo{ObjectID: ptyFoe3, Name: "foe3", ClassID: 10, Pos: ptyFoe3Pos, PvPFlag: 1}, t0)
			b.handleOne(t, ptyHit(ptyFoe2, selfID, ptyFoe2Pos), at(time.Second))
			b.handleOne(t, ptyHit(pcID, selfID, ptyFoe1Pos), at(time.Second))
			b.handleOne(t, ptyHit(pcID, selfID, ptyFoe1Pos), at(1200*time.Millisecond))
			b.handleOne(t, ptyHeal(ptyFoe3, pcID, ptyFoe3Pos), at(1500*time.Millisecond))
			b.world.Self.InCombat = true
			if n := len(b.attackers(at(2 * time.Second))); n < 2 {
				t.Fatalf("%d attackers, want 2", n)
			}
			wantDecision(t, b, at(2*time.Second), tc.rule, actSelect, tc.object, 0)
		})
	}
}
