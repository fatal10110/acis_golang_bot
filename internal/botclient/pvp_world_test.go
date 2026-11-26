package botclient

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The PvP world model's fixtures: a third player and a summon, the
// opponent's name, and a bow (Bow, items/0000-0099.xml 14) with its
// arrows.
const (
	pvpThird  = int32(0x20000003)
	pvpSummon = int32(0x20000004)
	foeName   = "foe"
	pvpBow    = int32(14)
)

// pvpTravel is a travel map with the skills the PvP tests cast (Wind
// Strike offensive, Heal not), a bow's arrows and, with arena set, one
// arena box around the origin, 2000 to each side.
func pvpTravel(arena bool) *TravelMap {
	m := &TravelMap{
		skills: map[datapack.SkillRef]datapack.Skill{
			{ID: skillWindStrike, Level: 1}: {Offensive: true},
			{ID: skillHeal, Level: 1}:       {},
		},
		arrows: map[int32]int32{pvpBow: 17},
	}
	if arena {
		m.zones.Arenas = []datapack.Zone{{Cuboid: true, X: []int32{-2000, 2000}, Y: []int32{-2000, 2000}, MinZ: -1000, MaxZ: 1000}}
	}
	return m
}

// pvpBrain is a brain of the default persona, edited by edit, with the PvP
// travel map and the opponent pcID (foeName) seen through the loop 300
// away.
func pvpBrain(t *testing.T, arena bool, edit func(*Persona)) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	if edit != nil {
		edit(&p)
	}
	b, _ := newTestBrain(t, p)
	b.cfg.Travel = pvpTravel(arena)
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, RunSpeed: 100, Running: true}, t0)
	return b
}

// duelUntil plays the bot's duel with foeName (pcID) through the loop from
// its acceptance at from up to phase: the countdown (1930), the start
// (ExDuelReady with ExDuelStart, as the server sends them together,
// network/duel.go startDuel) or the fight (the opponent's first
// ExDuelUpdateUserInfo).
func duelUntil(t *testing.T, b *Brain, phase DuelPhase, from time.Time) {
	t.Helper()
	b.handleOne(t, SystemMessage{ID: msgYouAcceptedS1Duel, Text: [2]string{foeName}}, from)
	if phase >= DuelStarted {
		b.handleOne(t, ExDuelReady{}, from.Add(5*time.Second))
		b.handleOne(t, ExDuelStart{}, from.Add(5*time.Second))
	}
	if phase >= DuelFighting {
		b.handleOne(t, ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID, HP: 100, MaxHP: 100}, from.Add(5*time.Second))
	}
}

// hitSelf is id's hit of damage on the bot.
func hitSelf(id, damage int32) Attack {
	return Attack{AttackerID: id, Hits: []AttackHit{{TargetID: selfID, Damage: damage}}}
}

// castAt is id's cast of skill at target.
func castAt(id, target, skill int32) MagicSkillUse {
	return MagicSkillUse{CasterID: id, TargetID: target, SkillID: skill, Level: 1, HitTime: 4 * time.Second}
}

// TestPvPOpponentRecord checks the opponent record a player in sight gets
// from CharInfo, RelationChanged, StatusUpdate, the duel window, the bot's
// damage (35) and the player's casts.
func TestPvPOpponentRecord(t *testing.T) {
	b := pvpBrain(t, false, nil)
	t1, t2, t3 := at(time.Second), at(2*time.Second), at(3*time.Second)
	b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "scout", ClassID: 7, RHand: pvpBow, PvPFlag: 2, ClanID: 5, AllyID: 6, Abnormal: 0x40, CP: 300, MaxCP: 400}, t1)
	r := b.pvp.opponents[pvpThird]
	if r == nil {
		t.Fatal("no record on sight")
	}
	if r.Tier != 1 || r.Archetype != charmodel.Archer || r.ClanID != 5 || r.AllyID != 6 || r.Abnormal != 0x40 || r.CP != 300 || r.MaxCP != 400 {
		t.Errorf("record %+v, want tier 1 archer, clan 5, ally 6, mask 0x40, CP 300/400", *r)
	}
	if !r.FlagBlinkAt.Equal(t1) {
		t.Errorf("blink at %v on a first sight blinking, want %v", r.FlagBlinkAt, t1)
	}
	b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "scout", ClassID: 7, PvPFlag: 2}, t2)
	if !r.FlagBlinkAt.Equal(t1) {
		t.Errorf("blink moved to %v while still blinking", r.FlagBlinkAt)
	}
	if row, _ := charmodel.ClassRow(7, false); r.Archetype != row.Archetype() {
		t.Errorf("archetype %v without the bow", r.Archetype)
	}
	b.handleOne(t, RelationChanged{ObjectID: pvpThird, PvPFlag: 1}, t2)
	if !r.FlagBlinkAt.IsZero() {
		t.Error("blink kept once the flag is steady")
	}
	b.handleOne(t, RelationChanged{ObjectID: pvpThird, PvPFlag: 2}, t3)
	if !r.FlagBlinkAt.Equal(t3) {
		t.Errorf("blink at %v on turning to 2, want %v", r.FlagBlinkAt, t3)
	}

	for _, tc := range []struct {
		class int32
		tier  int
	}{{0, 0}, {1, 1}, {2, 2}, {88, 3}} {
		if got := classTier(tc.class); got != tc.tier {
			t.Errorf("classTier(%d) = %d, want %d", tc.class, got, tc.tier)
		}
	}

	// Vitals: the selection's StatusUpdate, the CharInfo CP, then the bot's
	// damage, CP first; the duel window overrides both.
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, CP: 300, MaxCP: 300}, t1)
	b.handleOne(t, StatusUpdate{ObjectID: pcID, Set: StatusHP | StatusMaxHP, HP: 500, MaxHP: 1000}, t1)
	b.world.Target = pcID
	b.handleOne(t, SystemMessage{ID: msgYouDidS1Dmg, Number: 400}, t2)
	f := b.pvp.opponents[pcID]
	if f.CP != 0 || f.HP != 400 || f.MaxHP != 1000 || !f.SeenAt.Equal(t1) {
		t.Errorf("after 400 damage %+v, want CP 0, HP 400/1000 seen at t1", *f)
	}
	b.handleOne(t, ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID, HP: 900, MaxHP: 1000, CP: 200, MaxCP: 300}, t3)
	if f.HP != 900 || f.CP != 200 || f.MaxCP != 300 || !f.SeenAt.Equal(t3) {
		t.Errorf("after the duel window %+v, want HP 900, CP 200/300 seen at t3", *f)
	}

	// The last cast lands at arrival plus hit time; a cancel clears it.
	b.handleOne(t, castAt(pcID, pvpThird, skillHeal), t2)
	if want := (CastSeen{Skill: skillHeal, Level: 1, Target: pvpThird, At: t2, Lands: t2.Add(4 * time.Second)}); f.LastCast != want {
		t.Errorf("last cast %+v, want %+v", f.LastCast, want)
	}
	b.handleOne(t, MagicSkillCanceled{CasterID: pcID}, t3)
	if f.LastCast != (CastSeen{}) {
		t.Errorf("last cast %+v after the cancel", f.LastCast)
	}
	b.handleOne(t, DeleteObject{ObjectID: pcID}, t3)
	if b.pvp.opponents[pcID] != nil {
		t.Error("record kept out of sight")
	}
}

// TestPvPHostileCast checks that an offensive skill at the bot from any
// player counts as a hit, flagged or not, and a beneficial or unknown one
// does not.
func TestPvPHostileCast(t *testing.T) {
	for _, tc := range []struct {
		name  string
		skill int32
		want  bool
	}{
		{"wind strike", skillWindStrike, true},
		{"heal", skillHeal, false},
		{"unknown skill", 4001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, nil)
			b.handleOne(t, castAt(pcID, selfID, tc.skill), at(time.Second))
			if got := b.world.attacking(pcID, at(time.Second)); got != tc.want {
				t.Errorf("attacking = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPReaction checks the PvP reaction (D35): an opponent's Heal on its
// ally, the first ExDuelUpdateUserInfo and an arena opponent's move bring a
// decision 200-600 ms later, moving an armed 300-1200 ms one earlier and
// never later; a stranger's move brings none.
func TestPvPReaction(t *testing.T) {
	ev := at(10 * time.Second)
	tests := []struct {
		name  string
		arena bool
		setup func(t *testing.T, b *Brain)
		event Event
		// want is whether the event arms a PvP reaction.
		want bool
	}{
		{
			name:  "an opponent's heal on its ally",
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, hitSelf(pcID, 10), ev.Add(-time.Second)) },
			event: castAt(pcID, pvpThird, skillHeal), want: true,
		},
		{
			name:  "the first duel window",
			setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelStarted, ev.Add(-6*time.Second)) },
			event: ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID}, want: true,
		},
		{
			name:  "an arena opponent's move",
			arena: true,
			event: MoveToLocation{ObjectID: pcID, Origin: Point{Y: 300}, Dest: Point{Y: 600}}, want: true,
		},
		{
			name:  "a fought player's abnormal mask",
			setup: func(t *testing.T, b *Brain) { b.handleOne(t, hitSelf(pcID, 10), ev.Add(-time.Second)) },
			event: CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, Abnormal: 0x40}, want: true,
		},
		{
			name: "a player's heal on a party member",
			setup: func(t *testing.T, b *Brain) {
				b.world.Party = Party{LeaderID: selfID, Members: map[int32]*PartyMate{pvpThird: {}}}
			},
			event: castAt(pcID, pvpThird, skillHeal), want: true,
		},
		{
			name:  "a stranger's move",
			event: MoveToLocation{ObjectID: pcID, Origin: Point{Y: 300}, Dest: Point{Y: 600}},
		},
		{
			name:  "a stranger's heal on a stranger",
			event: castAt(pcID, pvpThird, skillHeal),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for seed := range uint64(50) {
				for _, armed := range []time.Duration{0, 100 * time.Millisecond, 1200 * time.Millisecond} {
					b := pvpBrain(t, tc.arena, nil)
					b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "ally", Pos: Point{X: 300}}, t0)
					if tc.setup != nil {
						tc.setup(t, b)
					}
					b.rng = rand.New(rand.NewPCG(seed, seed))
					b.reactAt = time.Time{}
					if armed != 0 {
						b.reactAt = ev.Add(armed)
					}
					b.handleOne(t, tc.event, ev)
					d := b.reactAt.Sub(ev)
					switch {
					case !tc.want && armed == 0 && !b.reactAt.IsZero():
						t.Fatalf("seed %d: a reaction armed %v later", seed, d)
					case !tc.want:
					case armed == 100*time.Millisecond && d != armed:
						t.Fatalf("seed %d: an earlier reaction moved to %v", seed, d)
					case armed != 100*time.Millisecond && (d < defaultPvPReactMin || d > defaultPvPReactMax):
						t.Fatalf("seed %d, armed %v: reaction %v later, want %v-%v", seed, armed, d, defaultPvPReactMin, defaultPvPReactMax)
					}
				}
			}
		})
	}
}

// TestPvPSummon checks the X4 summon cases (docs/pvp/targets.md "Summons
// and pets"; the flag-2 and arena cases are X4b's).
func TestPvPSummon(t *testing.T) {
	now := at(time.Second)
	// summon puts a summon of owner's, flag 1 and attackable, 400 from the
	// bot, and has it hit the bot.
	summon := func(t *testing.T, b *Brain, owner string) {
		b.handleOne(t, NPCInfo{ObjectID: pvpSummon, NPCID: 12077, Summon: true, Attackable: true, PvPFlag: 1, Title: owner, Pos: Point{X: -400}}, t0)
		b.handleOne(t, hitSelf(pvpSummon, 20), t0)
		b.world.Self.InCombat = true
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, b *Brain)
		// wantRule, wantKind and wantObject are the decision; a zero
		// wantObject checks none.
		wantRule   string
		wantKind   actionKind
		wantObject int32
		check      func(t *testing.T, b *Brain)
	}{
		{
			name:     "a flagged summon, its owner out of sight, is selected",
			setup:    func(t *testing.T, b *Brain) { summon(t, b, "away") },
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name: "a selected flagged summon is attacked with a plain Action",
			setup: func(t *testing.T, b *Brain) {
				summon(t, b, "away")
				b.world.Target = pvpSummon
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: pvpSummon,
			check: func(t *testing.T, b *Brain) {
				if b.forced(pvpSummon, now) {
					t.Error("a summon's cast would be forced")
				}
			},
		},
		{
			name: "a clean summon still attackable is held",
			setup: func(t *testing.T, b *Brain) {
				summon(t, b, "away")
				b.handleOne(t, RelationChanged{ObjectID: pvpSummon}, t0)
				b.world.Target = pvpSummon
			},
			wantRule: "attacked",
			check: func(t *testing.T, b *Brain) {
				if b.world.Objects[pvpSummon].Attackable == false {
					t.Fatal("the fixture lost Attackable")
				}
				b.blacklist[pvpSummon] = now.Add(time.Minute)
				if id := b.unfought(now); id != 0 {
					t.Errorf("unfought = %#x, want the refused summon held", id)
				}
			},
		},
		{
			name: "a summon with karma is fought",
			setup: func(t *testing.T, b *Brain) {
				summon(t, b, "away")
				b.handleOne(t, RelationChanged{ObjectID: pvpSummon, Karma: 100}, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name: "the flagged owner in sight is fought, not the summon",
			setup: func(t *testing.T, b *Brain) {
				summon(t, b, foeName)
				b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "a party member's flagged summon is never fought",
			setup: func(t *testing.T, b *Brain) {
				b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pvpThird, Name: "mate", HP: 100, MaxHP: 100}}}, t0)
				summon(t, b, "mate")
			},
			wantRule: "attacked",
		},
		{
			name: "a summon hitting it in its duel is not fought",
			setup: func(t *testing.T, b *Brain) {
				summon(t, b, "away")
				duelUntil(t, b, DuelFighting, t0.Add(-10*time.Second))
				b.handleOne(t, hitSelf(pvpSummon, 20), t0)
			},
			wantRule: "duel", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "a summon's Wind Strike enters the attackers table, a Heal does not",
			setup: func(t *testing.T, b *Brain) {
				b.handleOne(t, NPCInfo{ObjectID: pvpSummon, Summon: true, PvPFlag: 1, Title: "away"}, t0)
				b.handleOne(t, castAt(pvpSummon, selfID, skillHeal), t0)
				if b.world.attacking(pvpSummon, now) {
					t.Fatal("a summon's Heal counted as a hit")
				}
				b.handleOne(t, castAt(pvpSummon, selfID, skillWindStrike), t0)
				b.world.Self.InCombat = true
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, nil)
			tc.setup(t, b)
			rule, a := b.decide(now)
			if rule != tc.wantRule || a.kind != tc.wantKind || tc.wantObject != 0 && a.object != tc.wantObject {
				t.Fatalf("decide = rule %q action %d on %#x, want rule %q action %d on %#x", rule, a.kind, a.object, tc.wantRule, tc.wantKind, tc.wantObject)
			}
			if a.kind == actAttack && a.payload[0] != opGameAction {
				t.Errorf("attack opcode %#02x, want a plain Action", a.payload[0])
			}
			if tc.check != nil {
				tc.check(t, b)
			}
		})
	}
}

// TestPvPDuelNoFlaggedSupport checks the duel support gate
// (docs/pvp/guards.md "Duel support").
func TestPvPDuelNoFlaggedSupport(t *testing.T) {
	now := at(20 * time.Second)
	const chant = skillChantBattle
	// party puts the bot in a party with member pvpThird, 300 away at the
	// HP given out of 100, in need of the party chant, and the support kit
	// with a party heal for one wounded member.
	party := func(t *testing.T, b *Brain, hp int32, flag, karma int32) {
		b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "mate", Pos: Point{X: 300}, PvPFlag: flag, Karma: karma}, t0)
		b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pvpThird, Name: "mate", HP: hp, MaxHP: 100}}}, t0)
		var known []KnownSkill
		for _, id := range []int32{skillHeal, skillGroupHeal, chant} {
			known = append(known, KnownSkill{ID: id, Level: 1})
		}
		b.world.Apply(SkillList{Skills: known}, t0)
		withKit(b, ClassKit{
			Heals: []Heal{{Skill: skillHeal, HP: 60}},
			Buffs: []Buff{{Skill: chant, Party: true}},
		})
	}
	// partyHealKit leaves the bot a party heal only, with the bot wounded.
	partyHealKit := func(b *Brain) {
		withKit(b, ClassKit{Heals: []Heal{{Skill: skillGroupHeal, HP: 60, Members: 1}}})
		b.world.Self.HP = 50
	}
	// heals reports whether the heal rule heals pvpThird (selects it or
	// casts on it) or casts the party heal.
	heals := func(b *Brain) bool {
		a, ok := b.ruleHeal(now)
		return ok && (a.object == pvpThird || a.skill == skillGroupHeal)
	}
	chants := func(b *Brain) bool {
		a, ok := b.ruleRebuff(now)
		return ok && a.skill == chant
	}
	duel := func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) }
	end := func(t *testing.T, b *Brain) { b.handleOne(t, ExDuelEnd{}, t0.Add(10*time.Second)) }
	tests := []struct {
		name     string
		flagged  bool
		arena    bool
		setup    func(t *testing.T, b *Brain)
		wantHeal bool
		// wantChant is whether the party chant goes out; checked when
		// chant is set.
		chant, wantChant bool
	}{
		{name: "flagged member in a duel, SupportFlagged: no heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 50, 1, 0); duel(t, b) }},
		{name: "flagged member after the duel, SupportFlagged: heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 50, 1, 0); duel(t, b); end(t, b) }, wantHeal: true},
		{name: "karma member in a duel, SupportFlagged: no heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 50, 0, 100); duel(t, b) }},
		{name: "karma member after the duel, SupportFlagged: heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 50, 0, 100); duel(t, b); end(t, b) }, wantHeal: true},
		{name: "flagged member in range in a duel: no party chant", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 100, 1, 0); duel(t, b) }, chant: true},
		{name: "flagged member in range in a duel: no party heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 100, 1, 0); duel(t, b); partyHealKit(b) }},
		{name: "flagged member after the duel: party heal", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 100, 1, 0); duel(t, b); end(t, b); partyHealKit(b) }, wantHeal: true},
		{name: "flagged member after the duel: party chant", flagged: true, setup: func(t *testing.T, b *Brain) { party(t, b, 100, 1, 0); duel(t, b); end(t, b) }, chant: true, wantChant: true},
		{name: "karma member out of a duel: no heal", setup: func(t *testing.T, b *Brain) { party(t, b, 50, 0, 100) }},
		{name: "karma member out of a duel: no party chant", setup: func(t *testing.T, b *Brain) { party(t, b, 100, 0, 100) }, chant: true},
		{name: "clean member out of a duel: heal", setup: func(t *testing.T, b *Brain) { party(t, b, 50, 0, 0) }, wantHeal: true},
		{name: "karma member, both in an arena: heal", arena: true, setup: func(t *testing.T, b *Brain) { party(t, b, 50, 0, 100) }, wantHeal: true},
		{
			name: "duelling a clean party member: no heal on it",
			setup: func(t *testing.T, b *Brain) {
				party(t, b, 50, 0, 0)
				duelWith(t, b, pvpThird, "mate")
			},
		},
		{
			name: "duelling a clean party member: no party chant while in range",
			setup: func(t *testing.T, b *Brain) {
				party(t, b, 100, 0, 0)
				duelWith(t, b, pvpThird, "mate")
			},
			chant: true,
		},
		{
			name: "duelling a clean party member out of range: party chant",
			setup: func(t *testing.T, b *Brain) {
				party(t, b, 100, 0, 0)
				duelWith(t, b, pvpThird, "mate")
				b.handleOne(t, StopMove{ObjectID: pvpThird, Pos: Point{X: 1500}}, t0)
			},
			chant: true, wantChant: true,
		},
		{
			name: "a clean party member after the duel: heal",
			setup: func(t *testing.T, b *Brain) {
				party(t, b, 50, 0, 0)
				duelWith(t, b, pvpThird, "mate")
				end(t, b)
			},
			wantHeal: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, tc.arena, func(p *Persona) { p.PvP.SupportFlagged = tc.flagged })
			tc.setup(t, b)
			b.world.Target = pvpThird
			if tc.chant {
				if got := chants(b); got != tc.wantChant {
					t.Errorf("party chant = %v, want %v", got, tc.wantChant)
				}
				return
			}
			if got := heals(b); got != tc.wantHeal {
				t.Errorf("heal = %v, want %v", got, tc.wantHeal)
			}
		})
	}
}

// duelWith plays the bot's duel with name (id) into the fight at t0.
func duelWith(t *testing.T, b *Brain, id int32, name string) {
	t.Helper()
	b.handleOne(t, SystemMessage{ID: msgYouAcceptedS1Duel, Text: [2]string{name}}, t0)
	b.handleOne(t, ExDuelReady{}, t0)
	b.handleOne(t, ExDuelStart{}, t0)
	b.handleOne(t, ExDuelUpdateUserInfo{Name: name, ObjectID: id, HP: 100, MaxHP: 100}, t0)
}

// TestPvPDuelFight checks the X4 duel fight cases (docs/pvp/finding.md
// Tests; the area, pole and duel ground cases are X4b's).
func TestPvPDuelFight(t *testing.T) {
	start := at(5 * time.Second) // ExDuelStart and the duel window
	fight := start.Add(time.Second)
	knowWindStrike := func(b *Brain) {
		b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillWindStrike, Level: 1}}}, t0)
		withKit(b, ClassKit{Rotation: []Step{{Skill: skillWindStrike}}})
	}
	plain := encodeRequestMagicSkillUse(skillWindStrike, false)
	tests := []struct {
		name  string
		setup func(t *testing.T, b *Brain)
		now   time.Time
		// wantRule and wantKind are the decision, wantObject the object it
		// acts on when set.
		wantRule   string
		wantKind   actionKind
		wantObject int32
		check      func(t *testing.T, b *Brain, a Action)
	}{
		{
			name:     "the countdown holds",
			setup:    func(t *testing.T, b *Brain) { duelUntil(t, b, DuelCountdown, t0) },
			now:      at(2 * time.Second),
			wantRule: "duel",
		},
		{
			name: "the countdown holds at 3, 2 and 1 with the opponent selected",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				knowWindStrike(b)
				b.world.Target = pcID
				for i := range int32(3) {
					b.handleOne(t, SystemMessage{ID: msgDuelBeginsInS1Seconds, Number: 3 - i}, at(time.Duration(2+i)*time.Second))
				}
			},
			now:      at(4 * time.Second),
			wantRule: "duel",
		},
		{
			name: "the opponent's countdown duel window does not start the fight",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				knowWindStrike(b)
				b.world.Target = pcID
				b.handleOne(t, ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID, HP: 80, MaxHP: 100}, at(2*time.Second))
				if d := b.duel(at(2 * time.Second)); d.Phase != DuelCountdown {
					t.Fatalf("phase after a countdown window = %v, want the countdown", d.Phase)
				}
			},
			now:      at(4 * time.Second),
			wantRule: "duel",
		},
		{
			name: "an opponent's countdown hit is not answered",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.handleOne(t, hitSelf(pcID, 30), at(2*time.Second))
				b.world.Self.InCombat = true
			},
			now:      at(2 * time.Second),
			wantRule: "duel",
		},
		{
			name: "after the cancel the unflagged hitter is not fought",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.handleOne(t, hitSelf(pcID, 30), at(2*time.Second))
				b.handleOne(t, SystemMessage{ID: msgDuelEndedInTie}, at(3*time.Second))
				b.handleOne(t, ExDuelEnd{}, at(3*time.Second))
				b.world.Self.InCombat = true
				b.world.Target = pcID
			},
			now:      at(3 * time.Second),
			wantRule: "attacked",
			check: func(t *testing.T, b *Brain, _ Action) {
				if k := b.pvp.opponents[pcID].Kind; k != FightUnfought {
					t.Errorf("kind %v, want unfought", k)
				}
			},
		},
		{
			name: "after the cancel a countdown Wind Strike's caster is not fought",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.handleOne(t, castAt(pcID, selfID, skillWindStrike), at(2*time.Second))
				b.handleOne(t, SystemMessage{ID: msgDuelEndedInTie}, at(3*time.Second))
				b.handleOne(t, ExDuelEnd{}, at(3*time.Second))
				b.world.Self.InCombat = true
			},
			now:      at(3 * time.Second),
			wantRule: "attacked",
		},
		{
			name:     "ExDuelStart alone is no attack",
			setup:    func(t *testing.T, b *Brain) { duelUntil(t, b, DuelStarted, t0); b.world.Target = pcID },
			now:      fight,
			wantRule: "duel",
		},
		{
			name: "the duel window before the reaction delay is no attack",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				b.world.Target = pcID
			},
			now:      start.Add(defaultPvPReactMin - time.Millisecond),
			wantRule: "duel",
		},
		{
			name:     "the duel window plus the delay selects the opponent",
			setup:    func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) },
			now:      fight,
			wantRule: "duel", wantKind: actSelect, wantObject: pcID,
		},
		{
			name:     "the selected opponent gets a plain Action",
			setup:    func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0); b.world.Target = pcID },
			now:      fight,
			wantRule: "duel", wantKind: actAttack, wantObject: pcID,
			check: func(t *testing.T, b *Brain, a Action) {
				if a.payload[0] != opGameAction {
					t.Errorf("opcode %#02x, want a plain Action", a.payload[0])
				}
				if k := b.pvp.opponents[pcID].Kind; k != FightDuel {
					t.Errorf("kind %v, want duel", k)
				}
			},
		},
		{
			name: "the selected opponent gets an unforced cast",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				knowWindStrike(b)
				b.world.Target = pcID
			},
			now:      fight,
			wantRule: "duel", wantKind: actCast, wantObject: pcID,
			check: func(t *testing.T, _ *Brain, a Action) {
				if !bytes.Equal(a.payload, plain) {
					t.Errorf("cast % x, want % x (no ctrl)", a.payload, plain)
				}
			},
		},
		{
			name: "its Wind Strike at the bot enters the attackers table",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				b.handleOne(t, castAt(pcID, selfID, skillWindStrike), fight)
			},
			now:      fight,
			wantRule: "duel", wantKind: actSelect, wantObject: pcID,
			check: func(t *testing.T, b *Brain, _ Action) {
				if !b.world.attacking(pcID, fight) {
					t.Error("the opponent's Wind Strike is not in the attackers table")
				}
			},
		},
		{
			name: "a clan mate opponent is fought",
			setup: func(t *testing.T, b *Brain) {
				b.world.Clan.ID = 5
				b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, ClanID: 5}, t0)
				duelUntil(t, b, DuelFighting, t0)
				b.world.Target = pcID
			},
			now:      fight,
			wantRule: "duel", wantKind: actAttack, wantObject: pcID,
		},
		{
			name: "a party member opponent is fought",
			setup: func(t *testing.T, b *Brain) {
				b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pcID, Name: foeName, HP: 100, MaxHP: 100}}}, t0)
				duelUntil(t, b, DuelFighting, t0)
				b.world.Target = pcID
			},
			now:      fight,
			wantRule: "duel", wantKind: actAttack, wantObject: pcID,
		},
		{
			name: "692 stops the fight",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				b.world.Target = pcID
				b.handleOne(t, SystemMessage{ID: msgOtherPartyIsFrozen}, fight)
			},
			now:      fight,
			wantRule: "duel",
		},
		{
			name: "ExDuelEnd stops the fight",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0)
				b.world.Target = pcID
				b.handleOne(t, hitSelf(pcID, 10), fight)
				b.handleOne(t, ExDuelEnd{}, fight)
				b.world.Self.InCombat = true
			},
			now:      fight,
			wantRule: "attacked",
		},
		{
			name: "a monster in sight is never targeted",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.world.Target = npcID
			},
			now:      at(2 * time.Second),
			wantRule: "duel",
		},
		{
			name: "a monster hitting in the duel waits",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.handleOne(t, hitSelf(npcID, 10), at(2*time.Second))
				b.world.Self.InCombat = true
			},
			now:      at(2 * time.Second),
			wantRule: "duel",
		},
		{
			name: "a monster hitting in the duel is fought after 1952 and ExDuelEnd",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelCountdown, t0)
				b.handleOne(t, hitSelf(npcID, 10), at(2*time.Second))
				b.handleOne(t, SystemMessage{ID: msgDuelEndedInTie}, at(3*time.Second))
				b.world.Self.InCombat = true
				if rule, a := b.decide(at(3 * time.Second)); rule != "duel" || a.kind != actHold {
					t.Fatalf("after 1952 alone: rule %q action %d, want the duel hold", rule, a.kind)
				}
				b.handleOne(t, ExDuelEnd{}, at(3*time.Second))
			},
			now:      at(3 * time.Second),
			wantRule: "attacked", wantKind: actSelect, wantObject: npcID,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, nil)
			b.rng = rand.New(rand.NewPCG(7, 7))
			// The reaction lands within PvPReactMax: fight is past it.
			b.cfg.PvPReactMin, b.cfg.PvPReactMax = defaultPvPReactMin, defaultPvPReactMax
			tc.setup(t, b)
			rule, a := b.decide(tc.now)
			if rule != tc.wantRule || a.kind != tc.wantKind || tc.wantObject != 0 && a.object != tc.wantObject {
				t.Fatalf("decide = rule %q action %d on %#x, want rule %q action %d on %#x", rule, a.kind, a.object, tc.wantRule, tc.wantKind, tc.wantObject)
			}
			if tc.check != nil {
				tc.check(t, b, a)
			}
		})
	}
}

// TestPvPDuelResult checks the duel phases and the result read before
// ExDuelEnd: 1950 with the bot's name won, the opponent's lost, 1955 then
// 1950 with its own won, 1952 a tie; a challenge refused or unanswered for
// 15 s ends.
func TestPvPDuelResult(t *testing.T) {
	won := SystemMessage{ID: msgS1WonTheDuel, Text: [2]string{"bot7"}}
	lost := SystemMessage{ID: msgS1WonTheDuel, Text: [2]string{foeName}}
	withdrew := SystemMessage{ID: msgS1WithdrewS2Won, Text: [2]string{foeName, "bot7"}}
	tie := SystemMessage{ID: msgDuelEndedInTie}
	for _, tc := range []struct {
		name string
		evs  []Event
		want DuelResult
	}{
		{"won", []Event{won}, DuelWon},
		{"lost", []Event{lost}, DuelLost},
		{"withdrawal then won", []Event{withdrew, won}, DuelWon},
		{"tie", []Event{tie}, DuelTie},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, nil)
			duelUntil(t, b, DuelFighting, t0)
			now := at(30 * time.Second)
			for _, ev := range tc.evs {
				b.handleOne(t, ev, now)
			}
			if d := b.duel(now); d.Phase != DuelOver || d.Result != tc.want {
				t.Fatalf("duel %+v, want over with result %d", *d, tc.want)
			}
			b.handleOne(t, ExDuelEnd{}, now)
			if d := b.duel(now); d.Phase != DuelIdle || d.Result != tc.want {
				t.Fatalf("after ExDuelEnd %+v, want idle with result %d", *d, tc.want)
			}
		})
	}

	b := pvpBrain(t, false, nil)
	b.handleOne(t, ExDuelAskStart{Requester: foeName}, t0)
	if d := b.duel(t0); d.Phase != DuelAsked || d.Name != foeName {
		t.Fatalf("after ExDuelAskStart %+v, want asked by %s", *d, foeName)
	}
	if d := b.duel(t0.Add(duelAskOpen + time.Millisecond)); d.Phase != DuelIdle {
		t.Errorf("a challenge unanswered past 15 s is %d, want idle", d.Phase)
	}
	b.handleOne(t, ExDuelAskStart{Requester: foeName}, at(time.Minute))
	b.handleOne(t, SystemMessage{ID: msgS1IsBusyTryLater}, at(time.Minute))
	if d := b.duel(at(time.Minute)); d.Phase != DuelIdle {
		t.Errorf("a refused challenge is %d, want idle", d.Phase)
	}
	b.handleOne(t, SystemMessage{ID: msgS1CannotDuelChaotic, Text: [2]string{foeName}}, at(time.Minute))
	if d := b.duel(at(time.Minute)); d.Phase != DuelIdle {
		t.Errorf("phase %d after a refusal while idle", d.Phase)
	}
}

// TestPvPForced checks who takes a forced attack: a clean player, but not
// the duel opponent while fighting, a player in the bot's arena, a flagged
// or karma one, or a summon.
func TestPvPForced(t *testing.T) {
	now := at(10 * time.Second)
	for _, tc := range []struct {
		name  string
		arena bool
		setup func(t *testing.T, b *Brain)
		want  bool
	}{
		{name: "a clean player", want: true},
		{name: "the duel opponent in the countdown", setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelCountdown, t0) }, want: true},
		{name: "the duel opponent while fighting", setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) }},
		{name: "a player in its arena", arena: true},
		{name: "a flagged player", setup: func(t *testing.T, b *Brain) { b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, t0) }},
		{name: "a karma player", setup: func(t *testing.T, b *Brain) { b.handleOne(t, RelationChanged{ObjectID: pcID, Karma: 5}, t0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, tc.arena, nil)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			if got := b.forced(pcID, now); got != tc.want {
				t.Errorf("forced = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPMayFight checks MayFight's rules 2, 4, 5 and 6 outside a duel.
func TestPvPMayFight(t *testing.T) {
	now := at(time.Second)
	for _, tc := range []struct {
		name  string
		info  CharInfo
		setup func(t *testing.T, b *Brain)
		hit   bool
		want  FightKind
	}{
		{name: "a flagged attacker", info: CharInfo{PvPFlag: 1}, hit: true, want: FightBack},
		{name: "a flagged player", info: CharInfo{PvPFlag: 1}, want: FightFlagged},
		{name: "a clean attacker", hit: true, want: fightNone},
		{name: "a flagged party member", info: CharInfo{PvPFlag: 1}, hit: true, setup: func(t *testing.T, b *Brain) {
			b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pcID, Name: foeName}}}, t0)
		}},
		{name: "a flagged clan mate", info: CharInfo{PvPFlag: 1, ClanID: 5}, hit: true, setup: func(t *testing.T, b *Brain) { b.world.Clan.ID = 5 }},
		{name: "a flagged clanless player, the bot clanless", info: CharInfo{PvPFlag: 1}, hit: true, want: FightBack},
		{name: "a flagged ally", info: CharInfo{PvPFlag: 1, AllyID: 6}, hit: true, setup: func(t *testing.T, b *Brain) { b.world.Self.AllyID = 6 }},
		{name: "a flagged bot", info: CharInfo{Name: "RunnerBot", PvPFlag: 1}},
		{name: "a flagged bot that hit it", info: CharInfo{Name: "RunnerBot", PvPFlag: 1}, hit: true, want: FightBack},
		{name: "a flagged player, FightFlagged off", info: CharInfo{PvPFlag: 1}, hit: true, setup: func(t *testing.T, b *Brain) { b.persona.PvP.FightFlagged = false }},
		{name: "a clean player, ForceAttack on", hit: true, setup: func(t *testing.T, b *Brain) { b.persona.PvP.ForceAttack = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, nil)
			info := tc.info
			info.ObjectID, info.Pos = pcID, Point{Y: 300}
			if info.Name == "" {
				info.Name = foeName
			}
			b.handleOne(t, info, t0)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			if tc.hit {
				b.handleOne(t, hitSelf(pcID, 10), t0)
			}
			o := b.world.object(pcID)
			if got := b.fightKind(pcID, o, now); got != tc.want {
				t.Errorf("kind %v, want %v", got, tc.want)
			}
			if want := tc.want; tc.hit && want == fightNone && b.pvp.opponents[pcID].Kind != FightUnfought {
				t.Errorf("record kind %v for a refused attacker, want unfought", b.pvp.opponents[pcID].Kind)
			}
		})
	}
}

// TestPvPArenaEdge checks the arena position: the arena holding a point and
// its distance to the edge, nothing outside.
func TestPvPArenaEdge(t *testing.T) {
	b := pvpBrain(t, true, nil)
	b.cfg.Travel.zones.Arenas = append(b.cfg.Travel.zones.Arenas, datapack.Zone{X: []int32{5000, 6000, 5000}, Y: []int32{0, 0, 1000}, MinZ: -1000, MaxZ: 1000})
	for _, tc := range []struct {
		p    Point
		want arenaPos
	}{
		{Point{X: 1500}, arenaPos{Arena: 1, Edge: 500}},
		{Point{X: 5100, Y: 500}, arenaPos{Arena: 2, Edge: 100}},
		{Point{X: 3000}, arenaPos{}},
	} {
		if got := b.arenaAt(tc.p); got != tc.want {
			t.Errorf("arenaAt(%v) = %+v, want %+v", tc.p, got, tc.want)
		}
	}
}
