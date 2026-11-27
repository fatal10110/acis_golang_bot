package botclient

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Party duel fixtures (docs/pvp/party.md "Party duels", #424): the farmer's
// M11 bot party (pvp_party_farmer_test.go) with the Duels mode on. The other
// side is foeName (pcID) and foe2 (ptyFoe2), both clean.

// duelPartyKit is the damage rotation the party duel tests fight with.
var duelPartyKit = ClassKit{Rotation: []Step{{Skill: lyrNuke}}}

// duelLeader is the farmer's bot party seen by the farmer, the bot leading
// it with the Duels mode on, edited by edit; the foes are unflagged.
func duelLeader(t *testing.T, edit func(*Persona)) *Brain {
	t.Helper()
	b := farmerBrain(t, 10, func(p *Persona) {
		farmerPersona(p)
		p.PvP.Duels = true
		if edit != nil {
			edit(p)
		}
	}, duelPartyKit, selfID, farmerParty(selfID, 100))
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: ptyFoe1Pos}, t0)
	b.handleOne(t, CharInfo{ObjectID: ptyFoe2, Name: "foe2", ClassID: 10, Pos: ptyFoe2Pos}, t0)
	return b
}

// duelMember is the same party seen by the KN, the bot (fighting with
// duelPartyKit), led by the farmer: a member that never sees the challenge.
func duelMember(t *testing.T) *Brain {
	t.Helper()
	b := farmerBrain(t, 10, farmerPersona, duelPartyKit, ptyDmg, farmerParty(ptyKN, 100))
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: ptyFoe1Pos}, t0)
	b.handleOne(t, CharInfo{ObjectID: ptyFoe2, Name: "foe2", ClassID: 10, Pos: ptyFoe2Pos}, t0)
	return b
}

// window is foe id's (name's) duel window at hp of 100.
func window(id int32, name string, hp int32) ExDuelUpdateUserInfo {
	return ExDuelUpdateUserInfo{Name: name, ObjectID: id, HP: hp, MaxHP: 100}
}

// partyDuelUntilFight plays a party duel into the fight from the member's
// side: 1944 at from, the countdown's windows, then the start 35 s on with
// both foes' windows. It returns when the bot may attack.
func partyDuelUntilFight(t *testing.T, b *Brain, from time.Time) time.Time {
	t.Helper()
	b.handleOne(t, SystemMessage{ID: msgTransportedToDuelSite}, from)
	start := from.Add(35 * time.Second)
	b.handleOne(t, ExDuelReady{Party: true}, start)
	b.handleOne(t, ExDuelStart{Party: true}, start)
	b.handleOne(t, window(pcID, foeName, 100), start)
	b.handleOne(t, window(ptyFoe2, "foe2", 100), start)
	d := b.duel(start)
	if d.Phase != DuelFighting || !d.Party || len(d.Foes) != 2 {
		t.Fatalf("duel %+v after the start, want a party duel fighting two foes", *d)
	}
	return d.FightFrom
}

// wantHold checks that the duel rule holds b at now with no action.
func wantHold(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	if rule, a := b.decide(now); rule != "duel" || a.kind != 0 {
		t.Fatalf("decision rule %q kind %v, want the duel's hold", rule, a.kind)
	}
}

// TestPvPPartyDuelAnswer checks the leader's answer to a party duel
// (docs/pvp/party.md "Party duels"): yes at the PvP reaction when it leads
// a party whose members it sees passing the server's duel checks, else no
// at once.
func TestPvPPartyDuelAnswer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*Persona)
		setup func(*Brain)
		yes   bool
	}{
		{name: "every member in sight and ready", yes: true},
		{name: "the persona takes no duels", edit: func(p *Persona) { p.PvP.Duels = false }},
		{name: "a member out of sight", setup: func(b *Brain) { b.world.Apply(DeleteObject{ObjectID: ptyCL}, t0) }},
		{name: "a member under half HP", setup: func(b *Brain) { b.world.Party.Members[ptyKN].HP = 40 }},
		{name: "a member under half MP", setup: func(b *Brain) {
			m := b.world.Party.Members[ptyKN]
			m.MP, m.MaxMP = 40, 100
		}},
		{name: "a member in combat stance", setup: func(b *Brain) { b.world.Apply(AutoAttackStart{ObjectID: ptyCL}, t0) }},
		{name: "a member flagged", setup: func(b *Brain) { b.world.object(ptyCL).PvPFlag = 1 }},
		{name: "a member in a town", setup: func(b *Brain) {
			b.cfg.Travel.zones.Towns = []datapack.Zone{box(-150, -50, -50, 50)}
		}},
		{name: "a member leads: the bot is no leader", setup: func(b *Brain) { b.world.Party.LeaderID = ptyKN }},
		{name: "the bot under half MP", setup: func(b *Brain) { b.world.Self.MP = 40 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := duelLeader(t, tc.edit)
			if tc.setup != nil {
				tc.setup(b)
			}
			b.handleOne(t, ExDuelAskStart{Requester: foeName, Party: true}, at(time.Second))
			if d := b.duel(at(time.Second)); d.Phase != DuelAsked || !d.Party || d.Name != foeName {
				t.Fatalf("duel %+v after the party challenge, want asked by %s", *d, foeName)
			}
			due := b.pvp.asked.due
			rule, a := b.decide(at(time.Second))
			if tc.yes {
				if a.kind == actDuelAnswer {
					t.Fatal("answered before the PvP reaction")
				}
				rule, a = b.decide(due)
			}
			want := botwire.EncodeRequestDuelAnswerStart(true, tc.yes)
			if rule != "duel" || a.kind != actDuelAnswer || !bytes.Equal(a.payload, want) {
				t.Fatalf("decision rule %q kind %v payload % x, want % x", rule, a.kind, a.payload, want)
			}
		})
	}
	t.Run("a bot-marked challenger", func(t *testing.T) {
		b := duelLeader(t, nil)
		b.handleOne(t, ExDuelAskStart{Requester: markedName, Party: true}, at(time.Second))
		if _, a := b.decide(at(time.Second)); a.kind != actDuelAnswer || !bytes.Equal(a.payload, botwire.EncodeRequestDuelAnswerStart(true, false)) {
			t.Fatalf("payload % x, want the decline at once (D38)", a.payload)
		}
	})
	t.Run("the acceptance starts the countdown", func(t *testing.T) {
		b := duelLeader(t, nil)
		b.handleOne(t, ExDuelAskStart{Requester: foeName, Party: true}, at(time.Second))
		_, a := b.decide(b.pvp.asked.due)
		if _, err := b.send(a, b.pvp.asked.due); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, SystemMessage{ID: msgYouAcceptedS1PartyDuel, Text: [2]string{foeName}}, at(3*time.Second))
		b.handleOne(t, SystemMessage{ID: msgTransportedToDuelSite}, at(3*time.Second))
		if d := b.duel(at(3 * time.Second)); d.Phase != DuelCountdown || !d.Party || d.Name != foeName {
			t.Fatalf("duel %+v after 1933, want a party countdown against %s", *d, foeName)
		}
		wantHold(t, b, at(4*time.Second))
		// Its leader is a foe before any window names it: never healed.
		if b.supportAllowed(pcID, b.world.object(pcID), at(4*time.Second)) {
			t.Error("the other side's leader may be supported")
		}
	})
}

// TestPvPPartyDuelChallenge checks the leader's party duel challenge on a
// whisper, and its refusals (docs/pvp/party.md "Party duels").
func TestPvPPartyDuelChallenge(t *testing.T) {
	whispered := func(t *testing.T, b *Brain) (string, Action) {
		t.Helper()
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
		now := b.pvp.find.whisper.due
		rule, a := b.decide(now)
		if a.kind == actDuelStart {
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
		}
		return rule, a
	}
	t.Run("the leader challenges a party duel", func(t *testing.T) {
		b := duelLeader(t, nil)
		rule, a := whispered(t, b)
		if rule != "challenge" || !bytes.Equal(a.payload, botwire.EncodeRequestDuelStart(foeName, true)) {
			t.Fatalf("decision rule %q payload % x, want the party duel challenge", rule, a.payload)
		}
		if d := b.pvp.duel; d.Phase != DuelAsked || !d.Party || !d.Challenged {
			t.Fatalf("duel %+v, want its party duel challenge asked", d)
		}
		b.handleOne(t, SystemMessage{ID: msgS1AcceptedYourPartyDuel, Text: [2]string{"theirLeader"}}, at(5*time.Second))
		if d := b.duel(at(5 * time.Second)); d.Phase != DuelCountdown || !d.Party || d.Name != "theirLeader" {
			t.Fatalf("duel %+v after 1934, want a party countdown against the accepting leader", *d)
		}
	})
	for _, id := range []int32{msgChallengedNotInParty, msgOpposingPartyDeclined, msgOpposingPartyUnableToDuel} {
		t.Run("refused by "+strconv.Itoa(int(id)), func(t *testing.T) {
			b := duelLeader(t, nil)
			whispered(t, b)
			b.handleOne(t, SystemMessage{ID: id}, at(5*time.Second))
			if d := b.duel(at(5 * time.Second)); d.Phase != DuelIdle {
				t.Fatalf("duel %+v after %d, want idle", *d, id)
			}
			if !at(5 * time.Second).Before(b.pvp.find.held[foeName]) {
				t.Errorf("no decline hold on %s after %d", foeName, id)
			}
		})
	}
	t.Run("no challenge with a member out of sight", func(t *testing.T) {
		b := duelLeader(t, nil)
		b.world.Apply(DeleteObject{ObjectID: ptyKN}, t0)
		if _, a := whispered(t, b); a.kind == actDuelStart {
			t.Fatalf("challenged % x with a member out of sight", a.payload)
		}
	})
	t.Run("a party mate's whisper gets a one-on-one challenge", func(t *testing.T) {
		b := duelLeader(t, nil)
		b.handleOne(t, whisper(ptyKN, "kn", "duel"), at(time.Second))
		now := b.pvp.find.whisper.due
		if _, a := b.decide(now); !bytes.Equal(a.payload, botwire.EncodeRequestDuelStart("kn", false)) {
			t.Fatalf("payload % x, want the one-on-one challenge of a party mate", a.payload)
		}
	})
	t.Run("a member challenges one-on-one", func(t *testing.T) {
		b := duelMember(t)
		b.persona.PvP.Duels = true
		if _, a := whispered(t, b); !bytes.Equal(a.payload, botwire.EncodeRequestDuelStart(foeName, false)) {
			t.Fatalf("payload % x, want the one-on-one challenge", a.payload)
		}
	})
}

// TestPvPPartyDuelFight checks a member's party duel (docs/pvp/party.md
// "Party duels"): 1944 starts the countdown that holds it, the windows name
// the foes, it fights the nearest one or its leader's target, drops a
// defeated one, never fights a bot, holds once defeated, and supports its
// own side only.
func TestPvPPartyDuelFight(t *testing.T) {
	t.Run("1944 starts the countdown, the arena teleport included", func(t *testing.T) {
		b := duelMember(t)
		b.handleOne(t, SystemMessage{ID: msgTransportedToDuelSite}, at(time.Second))
		if d := b.duel(at(time.Second)); d.Phase != DuelCountdown || !d.Party {
			t.Fatalf("duel %+v after 1944, want a party countdown", *d)
		}
		arena := Point{X: -83760 + 40 - 180, Y: -238825 + 150, Z: -3331}
		b.handleOne(t, TeleportToLocation{ObjectID: selfID, Pos: arena}, at(3*time.Second))
		if _, err := b.send(Action{kind: actAppear, payload: botwire.EncodeAppearing()}, at(3*time.Second)); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, window(pcID, foeName, 100), at(10*time.Second))
		if d := b.duel(at(10 * time.Second)); d.Phase != DuelCountdown || len(d.Foes) != 1 {
			t.Fatalf("duel %+v after a countdown window, want the countdown with one foe", *d)
		}
		wantHold(t, b, at(20*time.Second))
		// The 2 minutes count from the countdown, its 35 s included
		// (duel/duel.go Begin): the one-on-one limit holds.
		if !b.inDuel(at(time.Second + duelLimit)) {
			t.Error("the party duel lapsed before its limit")
		}
		if b.inDuel(at(time.Second + duelLimit + time.Second)) {
			t.Error("the party duel outlived its limit")
		}
	})
	t.Run("the nearest foe, then the next once defeated", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		wantHold(t, b, from.Add(-time.Millisecond))
		wantDecision(t, b, from, "duel", actSelect, pcID, 0)
		if k := b.fightKind(ptyFoe2, b.world.object(ptyFoe2), from); k != FightDuel {
			t.Errorf("the other foe's fight kind %v, want the duel's", k)
		}
		b.handleOne(t, window(pcID, foeName, 1), from.Add(time.Second))
		wantDecision(t, b, from.Add(time.Second), "duel", actSelect, ptyFoe2, 0)
		// The click on the last foe answered by a 692: frozen too.
		if _, err := b.send(Action{kind: actSelect, payload: botwire.EncodeAction(ptyFoe2, Point{}), object: ptyFoe2}, from.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, SystemMessage{ID: msgOtherPartyIsFrozen}, from.Add(2*time.Second))
		if d := b.duel(from.Add(2 * time.Second)); d.Phase != DuelFighting {
			t.Fatalf("duel %+v after 692, want its side still fighting", *d)
		}
		wantHold(t, b, from.Add(2*time.Second))
	})
	t.Run("a foe's window decides: 1 HP, then 50, is live", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		b.handleOne(t, window(pcID, foeName, 1), from)
		b.handleOne(t, window(pcID, foeName, 50), from.Add(time.Second))
		if d := b.duel(from.Add(time.Second)); d.Foes[pcID] {
			t.Fatalf("foes %v, want %d live after a window at 50", d.Foes, pcID)
		}
	})
	t.Run("a 692 on the foe it clicked marks it defeated", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		_, a := b.decide(from)
		if _, err := b.send(a, from); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, SystemMessage{ID: msgOtherPartyIsFrozen}, from.Add(time.Second))
		if d := b.duel(from.Add(time.Second)); !d.Foes[pcID] || d.Opponent != 0 {
			t.Fatalf("duel %+v after a 692 on %d, want it defeated", *d, pcID)
		}
	})
	t.Run("a 692 after a click on a mate leaves the foe live", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		_, a := b.decide(from)
		if _, err := b.send(a, from); err != nil {
			t.Fatal(err)
		}
		b.pvp.duel.Opponent = pcID
		if _, err := b.send(Action{kind: actSelect, payload: botwire.EncodeAction(ptyCL, Point{}), object: ptyCL}, from); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, SystemMessage{ID: msgOtherPartyIsFrozen}, from.Add(time.Second))
		if d := b.duel(from.Add(time.Second)); d.Foes[pcID] || d.Opponent != pcID {
			t.Fatalf("duel %+v after a 692 on a mate, want %d live and fought", *d, pcID)
		}
	})
	t.Run("its leader's target first", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		b.handleOne(t, ptyHit(ptyDmg, ptyFoe2, Point{X: 50, Y: -50}), from)
		wantDecision(t, b, from, "duel", actSelect, ptyFoe2, 0)
	})
	t.Run("never a bot-marked foe (D38)", func(t *testing.T) {
		b := duelMember(t)
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: markedName, Pos: ptyFoe1Pos}, t0)
		from := partyDuelUntilFight(t, b, at(time.Second))
		wantDecision(t, b, from, "duel", actSelect, ptyFoe2, 0)
	})
	t.Run("defeated, it holds and does not flee", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		s := &b.world.Self
		s.HP, s.InCombat = 1, true
		b.world.Target = ptyCL
		wantHold(t, b, from)
	})
	t.Run("its own side supported, the other never", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		if !b.supportAllowed(ptyCL, b.world.object(ptyCL), from) {
			t.Error("its own party's CL may not be supported")
		}
		if b.supportAllowed(ptyFoe2, b.world.object(ptyFoe2), from) {
			t.Error("a foe may be supported")
		}
	})
	t.Run("a mate defeated at 1 HP is no patient", func(t *testing.T) {
		b := duelMember(t)
		from := partyDuelUntilFight(t, b, at(time.Second))
		m := b.world.Party.Members[ptyCL].PartyMember
		m.HP = 1
		b.handleOne(t, PartySmallWindowUpdate{Member: m}, from)
		if b.maySupport(ptyCL, from) {
			t.Error("a mate frozen at 1 HP may be supported")
		}
		b.handleOne(t, ExDuelEnd{Party: true}, from.Add(time.Second))
		if !b.maySupport(ptyCL, from.Add(time.Second)) {
			t.Error("the mate at 1 HP may not be supported after the duel")
		}
	})
	for _, tc := range []struct {
		name   string
		winner string
		want   DuelResult
	}{
		{"its leader's side won", "farmer", DuelWon},
		{"the other side won", "theirLeader", DuelLost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := duelMember(t)
			from := partyDuelUntilFight(t, b, at(time.Second))
			b.handleOne(t, SystemMessage{ID: msgS1PartyWonTheDuel, Text: [2]string{tc.winner}}, from)
			if d := b.duel(from); d.Phase != DuelOver || d.Result != tc.want {
				t.Fatalf("duel %+v, want over with result %d", *d, tc.want)
			}
			b.handleOne(t, ExDuelEnd{Party: true}, from)
			if d := b.duel(from); d.Phase != DuelIdle || d.Party || d.Foes != nil {
				t.Fatalf("duel %+v after ExDuelEnd, want idle", *d)
			}
		})
	}
}
