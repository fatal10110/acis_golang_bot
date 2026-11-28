package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Clan ids of the war tests: the bot's clan and the enemy's.
const (
	warOwnClan   = int32(7)
	warEnemyClan = int32(9)
)

// warMark is pcID's RelationChanged at mutual war as the server writes it:
// both war bits (network/relations.go relationTo).
var warMark = RelationChanged{ObjectID: pcID, Relation: botwire.RelationMutualWar | botwire.RelationOneSidedWar}

// warEnded is SystemMessage 1567 naming the enemy clan.
var warEnded = SystemMessage{ID: msgWarAgainstS1Stopped, Text: [2]string{"Enemies"}}

// warFoe is pcID's CharInfo at p in clan, clean, standing.
func warFoe(name string, clan int32, p Point) CharInfo {
	return CharInfo{ObjectID: pcID, Name: name, ClanID: clan, Pos: p, RunSpeed: 124, Running: true, CollisionRadius: 9}
}

// warSetup puts b in clan warOwnClan with War mode on and pcID, named
// name, a clean player of clan warEnemyClan at p, nobody hitting the bot.
func warSetup(t *testing.T, b *Brain, name string, p Point) {
	t.Helper()
	b.persona.PvP.FightWar = true
	b.world.Clan.ID = warOwnClan
	b.handleOne(t, warFoe(name, warEnemyClan, p), t0)
	b.world.object(pcID).move = stopAt(p)
	clear(b.world.Attackers)
	b.reactAt = time.Time{}
}

// TestPvPWarTarget checks MayFight rule 6b (docs/pvp/war.md, D40): a
// mutual-war enemy is fought with War mode on, its mark cleared by a war's
// end, the bot's own clan change and its CharInfo in another clan; it is
// hunted on hunting's preconditions with AttackRequest and ctrl-held casts,
// debuffs included.
func TestPvPWarTarget(t *testing.T) {
	foeAt := Point{Y: 300}
	for _, tc := range []struct {
		name  string
		foe   string
		setup func(*Brain)
		evs   []Event
		fight bool
	}{
		{name: "mutual war", evs: []Event{warMark}, fight: true},
		{name: "the mutual-war bit alone", evs: []Event{RelationChanged{ObjectID: pcID, Relation: botwire.RelationMutualWar}}, fight: true},
		{name: "one-sided war: a clean player", evs: []Event{RelationChanged{ObjectID: pcID, Relation: botwire.RelationOneSidedWar}}},
		{name: "no relation", evs: []Event{RelationChanged{ObjectID: pcID}}},
		{name: "War mode off", setup: func(b *Brain) { b.persona.PvP.FightWar = false }, evs: []Event{warMark}},
		{name: "a marked bot that has not hit", foe: markedName, evs: []Event{warMark}},
		{name: "a marked bot that hit", foe: markedName, evs: []Event{warMark, hitSelf(pcID, 5)}, fight: true},
		{name: "a newer RelationChanged without the bit", evs: []Event{warMark, RelationChanged{ObjectID: pcID}}},
		{name: "1567", evs: []Event{warMark, warEnded}},
		{name: "1566", evs: []Event{warMark, SystemMessage{ID: msgClanS1DecidedToStopWar, Text: [2]string{"Enemies"}}}},
		{name: "1567, then a new RelationChanged", evs: []Event{warMark, warEnded, warMark}, fight: true},
		{name: "another system message keeps the mark", evs: []Event{warMark, SystemMessage{ID: msgYouDidS1Dmg, Number: 10}}, fight: true},
		{name: "the bot's clan id changes in UserInfo", evs: []Event{warMark, UserInfo{ObjectID: selfID, Name: "bot", ClanID: 8}}},
		{name: "the bot leaves its clan in UserInfo", evs: []Event{warMark, UserInfo{ObjectID: selfID, Name: "bot"}}},
		{name: "the bot's clan member list cleared", evs: []Event{warMark, PledgeShowMemberListDeleteAll{}}},
		{name: "the bot joins another clan", evs: []Event{warMark, JoinPledge{ClanID: 8}}},
		{name: "the enemy's CharInfo shows clan 0", evs: []Event{warMark, warFoe(foeName, 0, foeAt)}},
		{name: "the enemy's CharInfo shows another clan", evs: []Event{warMark, warFoe(foeName, 10, foeAt)}},
		{name: "another clan, then a new RelationChanged", evs: []Event{warMark, warFoe(foeName, 10, foeAt), warMark}, fight: true},
		{name: "the enemy's CharInfo shows the same clan", evs: []Event{warMark, warFoe(foeName, warEnemyClan, foeAt)}, fight: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := huntBrain(t)
			name := tc.foe
			if name == "" {
				name = foeName
			}
			warSetup(t, b, name, foeAt)
			if tc.setup != nil {
				tc.setup(b)
			}
			for _, ev := range tc.evs {
				b.handleOne(t, ev, t0)
			}
			if got := b.mayFight(pcID, at(time.Second)); got != tc.fight {
				t.Fatalf("MayFight %v, want %v", got, tc.fight)
			}
		})
	}

	t.Run("hunted with AttackRequest", func(t *testing.T) {
		b := huntBrain(t)
		warSetup(t, b, foeName, foeAt)
		b.handleOne(t, warMark, t0)
		rule, a := b.decide(at(time.Second))
		if rule != "hunt" || a.kind != actSelect || a.object != pcID {
			t.Fatalf("rule %q action %v on %d, want hunting's selection of %d", rule, a.kind, a.object, pcID)
		}
		b.world.Target = pcID
		rule, a = b.decide(at(2 * time.Second))
		if rule != "hunt" || a.kind != actAttack || a.object != pcID {
			t.Fatalf("rule %q action %v on %d, want hunting's AttackRequest on %d", rule, a.kind, a.object, pcID)
		}
		if k := b.pvp.opponents[pcID].Kind; k != FightWar || k.String() != "war" {
			t.Fatalf("fight kind %v, want war", k)
		}
	})
	for _, tc := range []struct {
		name  string
		setup func(*Brain)
	}{
		{"Hunt off", func(b *Brain) { b.persona.PvP.Hunt = false }},
		{"War off", func(b *Brain) { b.persona.PvP.FightWar = false }},
		{"one-sided war", func(b *Brain) {
			b.handleOne(t, RelationChanged{ObjectID: pcID, Relation: botwire.RelationOneSidedWar}, t0)
		}},
		{"HP under StandHP", func(b *Brain) { b.world.Self.HP = 10 }},
		{"another tier", func(b *Brain) {
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 1, ClanID: warEnemyClan, Pos: foeAt}, t0)
		}},
	} {
		t.Run("not hunted: "+tc.name, func(t *testing.T) {
			b := huntBrain(t)
			warSetup(t, b, foeName, foeAt)
			b.handleOne(t, warMark, t0)
			tc.setup(b)
			if fightsWith(b, "hunt", pcID, at(time.Second)) {
				t.Fatal("hunted")
			}
		})
	}

	t.Run("WA: ctrl-held Stun Attack (a debuff) first, as on a flagged player", func(t *testing.T) {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		warSetup(t, b, foeName, Point{X: 90})
		b.handleOne(t, warMark, t0)
		b.world.Self.InCombat, b.world.Target = true, pcID
		b.reactAt = time.Time{}
		_, a := b.decide(at(time.Second))
		if a.kind != actCast || a.skill != skillStunAttack || string(a.payload) != string(botwire.EncodeRequestMagicSkillUse(skillStunAttack, true)) {
			t.Fatalf("decision %v skill %d, want the ctrl-held Stun Attack", a.kind, a.skill)
		}
	})
	t.Run("WA: the mark gone, a clean player is not fought", func(t *testing.T) {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		warSetup(t, b, foeName, Point{X: 90})
		b.handleOne(t, warMark, t0)
		b.handleOne(t, warEnded, t0)
		b.world.Self.InCombat, b.world.Target = true, pcID
		b.reactAt = time.Time{}
		if _, a := b.decide(at(time.Second)); actsOn(a, pcID) {
			t.Fatalf("decision %v on a clean player after the war ended", a.kind)
		}
	})
}

// TestPvPWarGuard checks the guards on a war enemy (docs/pvp/war.md
// "Guards"): its mark cleared mid-fight on a clean target stops the fight
// at once, Esc for a cast in its window, a move for a swing, with no
// further Action, AttackRequest or cast; a flagged one is fought on under
// the flag guard, which holds no damage while the mark stands.
func TestPvPWarGuard(t *testing.T) {
	ws := Step{Skill: skillWindStrike}
	frame := time.Second / frameRate
	warGuard := func(t *testing.T, steps ...Step) *Brain {
		b := guardBrain(t, false, 0, 0, steps...)
		warSetup(t, b, foeName, Point{Y: 300})
		b.handleOne(t, warMark, t0)
		b.reactAt = time.Time{}
		return b
	}
	stopped := func(t *testing.T, b *Brain, now time.Time) {
		t.Helper()
		if b.mayFight(pcID, now) {
			t.Fatal("may fight a clean player whose mark cleared")
		}
		for _, d := range []time.Duration{0, time.Second, 5 * time.Second} {
			if _, a := b.decide(now.Add(d)); actsOn(a, pcID) {
				t.Fatalf("decision %v on pcID after the mark cleared", a.kind)
			}
		}
	}
	for _, tc := range []struct {
		name  string
		clear Event
	}{
		{"1567", warEnded},
		{"the enemy's CharInfo with clan 0", warFoe(foeName, 0, Point{Y: 300})},
	} {
		t.Run(tc.name+" mid-cast: Esc, then a move", func(t *testing.T) {
			b := warGuard(t, ws)
			play(t, b, []beat{
				{at: 0, kind: actCast, skill: skillWindStrike},
				{at: 100 * time.Millisecond, ev: selfCast(skillWindStrike, 4*time.Second), noDecide: true},
				{at: time.Second, ev: tc.clear, kind: actTargetCancel, rule: "guard", wake: time.Second + frame},
				{at: time.Second + 50*time.Millisecond, kind: actHold, rule: "guard"},
				{at: time.Second + 100*time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard", wake: time.Second + 100*time.Millisecond + frame},
			})
			stopped(t, b, at(2*time.Second))
		})
		t.Run(tc.name+" mid-swing: a move", func(t *testing.T) {
			b := warGuard(t)
			play(t, b, []beat{
				{at: 0, kind: actAttack},
				{at: time.Second, ev: tc.clear, kind: actMove, rule: "guard", wake: time.Second + frame},
			})
			stopped(t, b, at(2*time.Second))
		})
	}
	t.Run("the first cast is ctrl-held", func(t *testing.T) {
		b := warGuard(t, ws)
		_, a := b.decide(t0)
		if a.kind != actCast || string(a.payload) != string(botwire.EncodeRequestMagicSkillUse(skillWindStrike, true)) {
			t.Fatalf("decision %v, want the ctrl-held Wind Strike", a.kind)
		}
	})
	t.Run("flagged: the fight goes on under the flag guard", func(t *testing.T) {
		b := warGuard(t, ws)
		// The server's RelationChanged on a flag change carries the war
		// bits too.
		blink := flagTo(2)
		blink.Relation = warMark.Relation
		b.handleOne(t, blink, at(500*time.Millisecond))
		if o := b.world.object(pcID); b.blinking(pcID, o, at(time.Second)) {
			t.Fatal("the flag guard holds damage on a marked enemy: a kill is a PvP kill")
		}
		b.handleOne(t, warFoe(foeName, 0, Point{Y: 300}), at(time.Second))
		b.handleOne(t, flagTo(1), at(time.Second))
		now := at(time.Second)
		if !b.mayFight(pcID, now) || b.guardDue(now) != 0 {
			t.Fatal("a flagged player whose mark cleared is no longer fought")
		}
		b.handleOne(t, flagTo(2), at(2*time.Second))
		if o := b.world.object(pcID); !b.blinking(pcID, o, at(2*time.Second)) {
			t.Fatal("the flag guard is off for a blinking player whose mark cleared")
		}
	})
}
