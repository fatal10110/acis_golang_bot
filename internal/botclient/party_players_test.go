package botclient

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The players of the join tests: joinFrom(i) is the join of player i.
const joinPlayerBase = int32(0x30000300)

var joinPlayers = []string{"Pia", "Quin", "Rolf", "Sven", "Tove", "Ulla", "Vera", "Wido", "Xena", "Yuri", "Ziva", "Odo"}

func joinFrom(i int) CreatureSay {
	return whisper(joinPlayerBase+int32(i), joinPlayers[i], "join")
}

// withGuests adds n players named Guest<i> to rows and returns it.
func withGuests(rows map[int32]string, n int) map[int32]string {
	for i := range n {
		rows[joinPlayerBase+0x100+int32(i)] = fmt.Sprintf("Guest%d", i)
	}
	return rows
}

// invitedAll are the names out invited, in order.
func invitedAll(t *testing.T, out *recordSender) []string {
	t.Helper()
	var got []string
	for _, f := range framesOf(out, opGameRequestJoinParty) {
		r := wire.NewReader(f[1:])
		got = append(got, r.ReadString())
	}
	return got
}

// tellsOf are out's whispers as "target: text", in order.
func tellsOf(out *recordSender) []string {
	var got []string
	for _, f := range framesOf(out, opGameSay2) {
		r := wire.NewReader(f[1:])
		text := r.ReadString()
		if r.ReadInt32() != chatTell {
			continue
		}
		got = append(got, r.ReadString()+": "+text)
	}
	return got
}

// wantList fails unless got is want.
func wantList(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s %q, want %q", what, got, want)
	}
}

// queryingBrain is clearBrain with an authority over a database, so a
// whisper reaching it queues a check.
func queryingBrain(t *testing.T, slot Command, party *PartySmallWindowAll, p Persona) (*Brain, *botLink, *recordSender) {
	t.Helper()
	b, l, out := clearBrain(t, slot, party, p)
	b.cfg.Authority = NewAuthority(AuthorityConfig{Lookup: newFakeLevels(nil), Clock: b.clock})
	return b, l, out
}

// noQuery fails when b's authority queued a check.
func noQuery(t *testing.T, b *Brain) {
	t.Helper()
	if n := len(b.cfg.Authority.queue); n != 0 {
		t.Fatalf("%d authority checks, want none", n)
	}
}

// TestPartyJoinWhisper checks a player's join (docs/parties/players.md): a
// lead bot with room invites it, asking no authority; a full one answers
// "party is full", a member "ask <leader>", a bot with no slot takes it as
// chat; one answer per sender per 30 s; the queue goes ahead of the listed
// members once nothing is in flight, players never filling a listed
// member's place; at most ten join replies an hour, apart from the chat
// replies; no reply whisper to a marked name.
func TestPartyJoinWhisper(t *testing.T) {
	chatty := Persona{Chat: defaultChatPolicy()}
	t.Run("parse", func(t *testing.T) {
		for text, want := range map[string]bool{"join": true, "  JOIN ": true, "join me": false, "joinme": false} {
			if c, ok := parseCommand(text); ok != want || ok && c.kind != cmdJoin {
				t.Errorf("parseCommand(%q) = %+v, %v; want join %v", text, c, ok, want)
			}
		}
	})
	t.Run("leader with room invites", func(t *testing.T) {
		b, _, out := queryingBrain(t, lead("Abel"), ptr(ledBy(abelID)), Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		wantList(t, "invited", invitedAll(t, out), "Pia")
		noQuery(t, b)
		wantList(t, "tells", tellsOf(out))
	})
	t.Run("full", func(t *testing.T) {
		b, _, out := queryingBrain(t, lead("Abel"), ptr(slotParty(selfID, withGuests(map[int32]string{abelID: "Abel"}, 7))), Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		wantList(t, "tells", tellsOf(out), "Pia: party is full")
		wantList(t, "invited", invitedAll(t, out))
		noQuery(t, b)
	})
	t.Run("member", func(t *testing.T) {
		b, _, out := queryingBrain(t, joinSlot, &memberParty, Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		wantList(t, "tells", tellsOf(out), "Pia: ask Leader")
		noQuery(t, b)
	})
	t.Run("no slot, chat", func(t *testing.T) {
		b, _, out := queryingBrain(t, Command{}, nil, chatty)
		handleAt(t, b, at(time.Second), joinFrom(0))
		if b.chat.reply.kind == actHold {
			t.Fatal("no chat reply waiting, want join taken as chat")
		}
		decideAt(t, b, 14*time.Second)
		wantList(t, "invited", invitedAll(t, out))
		if got := tellsOf(out); len(got) != 1 || got[0] == "Pia: ask Leader" || got[0] == "Pia: party is full" {
			t.Fatalf("tells %q, want one chat reply", got)
		}
		noQuery(t, b)
	})
	t.Run("again within 30 s", func(t *testing.T) {
		b, _, out := clearBrain(t, joinSlot, &memberParty, Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		handleAt(t, b, at(20*time.Second), joinFrom(0))
		decideAt(t, b, 21*time.Second, 25*time.Second)
		wantList(t, "tells by 25 s", tellsOf(out), "Pia: ask Leader")
		handleAt(t, b, at(31*time.Second), joinFrom(0))
		decideAt(t, b, 32*time.Second)
		wantList(t, "tells by 32 s", tellsOf(out), "Pia: ask Leader", "Pia: ask Leader")
	})
	t.Run("chat a minute earlier", func(t *testing.T) {
		b, _, out := clearBrain(t, joinSlot, &memberParty, chatty)
		handleAt(t, b, at(time.Second), whisper(joinPlayerBase, "Pia", "hi"))
		decideAt(t, b, 14*time.Second)
		handleAt(t, b, at(time.Minute), joinFrom(0))
		decideAt(t, b, time.Minute+time.Second)
		if got := tellsOf(out); len(got) != 2 || got[1] != "Pia: ask Leader" {
			t.Fatalf("tells %q, want a chat reply then ask Leader", got)
		}
	})
	t.Run("after a listed member's invitation", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
		decideAt(t, b, 0)
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		wantList(t, "invited in flight", invitedAll(t, out), "Abel")
		// Abel declines: its InviteRetry starts; the player goes next.
		handleAt(t, b, at(3*time.Second), JoinParty{Response: 0})
		decideAt(t, b, 4*time.Second)
		wantList(t, "invited after Abel", invitedAll(t, out), "Abel", "Pia")
		// The player declines: off the queue, no InviteRetry.
		handleAt(t, b, at(5*time.Second), JoinParty{Response: 0})
		decideAt(t, b, 6*time.Second)
		handleAt(t, b, at(7*time.Second), JoinParty{Response: 1},
			PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: bethID, Name: "Beth"}})
		decideAt(t, b, 8*time.Second, 32*time.Second)
		wantList(t, "invited by 32 s", invitedAll(t, out), "Abel", "Pia", "Beth")
		decideAt(t, b, 33*time.Second, 51*time.Second)
		wantList(t, "invited by 51 s", invitedAll(t, out), "Abel", "Pia", "Beth", "Abel")
	})
	t.Run("164 on the player", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel"), ptr(ledBy(abelID)), Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		decideAt(t, b, 2*time.Second)
		handleAt(t, b, at(3*time.Second), SystemMessage{ID: msgWaitingForReply})
		decideAt(t, b, 4*time.Second, 19*time.Second)
		wantList(t, "invited by 19 s", invitedAll(t, out), "Pia")
		decideAt(t, b, 20*time.Second)
		wantList(t, "invited at 20 s", invitedAll(t, out), "Pia", "Pia")
	})
	t.Run("every place queued", func(t *testing.T) {
		// Eight members: one free place, Pia's.
		b, _, out := clearBrain(t, lead("Abel"), ptr(slotParty(selfID, withGuests(map[int32]string{abelID: "Abel"}, 6))), Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0), joinFrom(1))
		decideAt(t, b, 2*time.Second, 3*time.Second)
		wantList(t, "invited", invitedAll(t, out), "Pia")
		wantList(t, "tells", tellsOf(out), "Quin: party is full")
	})
	t.Run("a listed member's place", func(t *testing.T) {
		// Abel in, six players: eight members and one free place, which
		// Pia's join takes before the slot lists Beth.
		b, _, out := clearBrain(t, lead("Abel"), ptr(slotParty(selfID, withGuests(map[int32]string{abelID: "Abel"}, 6))), Persona{})
		handleAt(t, b, at(time.Second), joinFrom(0))
		b.setPartySlot(lead("Abel Beth"))
		// 9 - 8 - 1 = 0: full, and Beth, not Pia, gets the place.
		handleAt(t, b, at(2*time.Second), joinFrom(1))
		decideAt(t, b, 3*time.Second, 3*time.Second+time.Millisecond)
		wantList(t, "tells", tellsOf(out), "Quin: party is full")
		wantList(t, "invited", invitedAll(t, out), "Beth")
		handleAt(t, b, at(4*time.Second), JoinParty{Response: 1},
			PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: bethID, Name: "Beth"}})
		decideAt(t, b, 5*time.Second, 25*time.Second)
		wantList(t, "invited with nine", invitedAll(t, out), "Beth")
		if !slices.Equal(b.partyLead.queue, []string{"Pia"}) {
			t.Fatalf("queue %q, want Pia waiting", b.partyLead.queue)
		}
		// Beth leaves: her place stays hers, so Pia still waits.
		handleAt(t, b, at(26*time.Second), SystemMessage{ID: msgLeftParty, Text: [2]string{"Beth"}}, PartySmallWindowDelete{ObjectID: bethID, Name: "Beth"})
		decideAt(t, b, 27*time.Second, 55*time.Second)
		wantList(t, "invited after Beth left", invitedAll(t, out), "Beth")
		decideAt(t, b, 57*time.Second)
		wantList(t, "invited Beth back", invitedAll(t, out), "Beth", "Beth")
	})
	t.Run("ten replies an hour", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel"), ptr(slotParty(selfID, withGuests(map[int32]string{abelID: "Abel"}, 7))), chatty)
		handleAt(t, b, at(time.Second), whisper(joinPlayerBase+0x200, "Chatter", "hi"))
		decideAt(t, b, 14*time.Second)
		for i := range 11 {
			d := time.Minute + time.Duration(i)*5*time.Minute
			handleAt(t, b, at(d), joinFrom(i))
			decideAt(t, b, d+time.Second)
		}
		handleAt(t, b, at(56*time.Minute), whisper(joinPlayerBase+0x201, "Talker", "hi"))
		decideAt(t, b, 57*time.Minute)
		got := tellsOf(out)
		var full, chat int
		for _, s := range got {
			if strings.HasSuffix(s, replyPartyFull) {
				full++
			} else {
				chat++
			}
		}
		if full != 10 || chat != 2 || slices.Contains(got, "Ziva: party is full") {
			t.Fatalf("tells %q, want ten join replies, none to the eleventh, and both chat replies", got)
		}
	})
	t.Run("marked player", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel"), ptr(ledBy(abelID)), Persona{})
		handleAt(t, b, at(time.Second), whisper(talbotID, "Talbot", "join"))
		decideAt(t, b, 2*time.Second)
		wantList(t, "invited", invitedAll(t, out), "Talbot")
		m, _, mout := clearBrain(t, joinSlot, &memberParty, Persona{})
		handleAt(t, m, at(time.Second), whisper(talbotID, "Talbot", "join"))
		decideAt(t, m, 2*time.Second)
		f, _, fout := clearBrain(t, lead("Abel"), ptr(slotParty(selfID, withGuests(map[int32]string{abelID: "Abel"}, 7))), Persona{})
		handleAt(t, f, at(time.Second), whisper(talbotID, "Talbot", "join"))
		decideAt(t, f, 2*time.Second)
		wantList(t, "tells", append(append(tellsOf(out), tellsOf(mout)...), tellsOf(fout)...))
	})
}

// claimsBrain is clearBrain of bot self playing role, accepting every
// invitation through the shared claims and the commander's hooks, handed
// party (nil for none) at t0.
func claimsBrain(t *testing.T, self int32, slot Command, party *PartySmallWindowAll, role Role, claims *PartyClaims) (*Brain, *botLink, *recordSender) {
	t.Helper()
	b, l, out := clearBrain(t, slot, nil, Persona{})
	b.world.Self.ObjectID = self
	cfg := BrainConfig{Persona: Persona{JoinParty: acceptParty}}
	claims.Apply(&cfg)
	b.persona.JoinParty = b.cfg.Authority.Hooks(cfg.Persona).JoinParty
	b.persona.Role = role
	b.cfg.PartyClaims = claims
	if party != nil {
		handleAt(t, b, t0, *party)
	}
	return b, l, out
}

// The bots sharing a claim table in the player tests, besides selfID.
const (
	otherBotID = int32(0x10000201)
	thirdBotID = int32(0x10000202)
)

// TestPartyInviteMePlayer checks a player's invite me (D28): a refused
// verdict, or no authority, takes a Support member of a bot party out under
// the claim table, whispering back; any other bot and any other refused
// command ignore it; the pending claim outlives the bot's own withdrawal,
// so a second member stays, and lapses 15 s after it was taken.
func TestPartyInviteMePlayer(t *testing.T) {
	inviteMe := whisper(joinPlayerBase, "Pia", "invite me")
	takenOut := func(t *testing.T, b *Brain, l *botLink, out *recordSender, claims *PartyClaims) {
		t.Helper()
		decideAt(t, b, 2*time.Second, 6*time.Second)
		cleared(t, b, l, at(6*time.Second))
		if w := framesOf(out, opGameRequestWithdrawParty); len(w) != 1 {
			t.Fatalf("%d withdrawals, want 1", len(w))
		}
		wantList(t, "tells", tellsOf(out), "Pia: invite me")
		if i, j := slices.IndexFunc(out.frames, func(f []byte) bool { return f[0] == opGameRequestWithdrawParty }),
			slices.IndexFunc(out.frames, func(f []byte) bool { return f[0] == opGameSay2 }); i > j {
			t.Fatal("whisper before the withdrawal")
		}
		if leader, _, ok := claims.held(b.world.Self.ObjectID); !ok || leader != "Pia" {
			t.Fatalf("claim %q %v, want Pia", leader, ok)
		}
	}
	ignored := func(t *testing.T, b *Brain, l *botLink, out *recordSender, slot Command) {
		t.Helper()
		decideAt(t, b, 2*time.Second, 6*time.Second)
		if l.party != slot {
			t.Fatalf("slot %q, want %q kept", l.party, slot)
		}
		if n, m := len(framesOf(out, opGameRequestWithdrawParty)), len(tellsOf(out)); n+m != 0 || b.orders.stay {
			t.Fatalf("%d withdrawals, %d whispers, stay %v; want nothing", n, m, b.orders.stay)
		}
	}
	t.Run("support member", func(t *testing.T) {
		claims := NewPartyClaims()
		b, l, out := claimsBrain(t, selfID, joinSlot, &memberParty, RoleSupport, claims)
		handleAt(t, b, at(time.Second), inviteMe)
		takenOut(t, b, l, out, claims)
	})
	t.Run("no authority", func(t *testing.T) {
		claims := NewPartyClaims()
		b, l, out := claimsBrain(t, selfID, joinSlot, &memberParty, RoleSupport, claims)
		b.cfg.Authority = nil
		handleAt(t, b, at(time.Second), whisper(joinPlayerBase, "Pia", "stay"))
		ignored(t, b, l, out, joinSlot)
		handleAt(t, b, at(7*time.Second), inviteMe)
		decideAt(t, b, 8*time.Second, 12*time.Second)
		cleared(t, b, l, at(12*time.Second))
		wantList(t, "tells", tellsOf(out), "Pia: invite me")
	})
	t.Run("held by another bot", func(t *testing.T) {
		claims := NewPartyClaims()
		claims.claim("Pia", otherBotID, t0)
		b, l, out := claimsBrain(t, selfID, joinSlot, &memberParty, RoleSupport, claims)
		handleAt(t, b, at(time.Second), inviteMe)
		ignored(t, b, l, out, joinSlot)
	})
	others := []struct {
		name  string
		slot  Command
		party *PartySmallWindowAll
		role  Role
	}{
		{"damage member", joinSlot, &memberParty, RoleDamage},
		{"leader", leadSlot, &leadParty, RoleSupport},
		{"in no party", joinSlot, nil, RoleSupport},
		{"in a player's party", Command{}, ptr(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot"})), RoleSupport},
	}
	for _, tt := range others {
		t.Run(tt.name, func(t *testing.T) {
			claims := NewPartyClaims()
			b, l, out := claimsBrain(t, selfID, tt.slot, tt.party, tt.role, claims)
			handleAt(t, b, at(time.Second), inviteMe)
			ignored(t, b, l, out, tt.slot)
			if n := claims.Held(); n != 0 {
				t.Fatalf("%d claims, want none", n)
			}
		})
	}
	t.Run("other refused command", func(t *testing.T) {
		claims := NewPartyClaims()
		b, l, out := claimsBrain(t, selfID, joinSlot, &memberParty, RoleSupport, claims)
		handleAt(t, b, at(time.Second), whisper(joinPlayerBase, "Pia", "leave"))
		ignored(t, b, l, out, joinSlot)
	})
	t.Run("second member, and the lapse", func(t *testing.T) {
		claims := NewPartyClaims()
		b, l, out := claimsBrain(t, selfID, joinSlot, &memberParty, RoleSupport, claims)
		handleAt(t, b, at(time.Second), inviteMe)
		takenOut(t, b, l, out, claims)
		handleAt(t, b, at(7*time.Second), PartySmallWindowDeleteAll{})
		c, cl, cout := claimsBrain(t, otherBotID, joinSlot, &memberParty, RoleSupport, claims)
		handleAt(t, c, at(8*time.Second), inviteMe)
		decideAt(t, c, 9*time.Second, 12*time.Second)
		if cl.party != joinSlot || len(framesOf(cout, opGameRequestWithdrawParty))+len(tellsOf(cout)) != 0 {
			t.Fatalf("second member slot %q, sent %d frames; want kept, nothing", cl.party, len(cout.frames))
		}
		decideAt(t, b, 15*time.Second)
		if _, _, ok := claims.held(selfID); !ok {
			t.Fatal("claim gone at 15 s, want held 15 s from 1 s")
		}
		decideAt(t, b, 16*time.Second)
		if n := claims.Held(); n != 0 {
			t.Fatalf("%d claims at 16 s, want the claim lapsed", n)
		}
	})
}

// TestPartyHandoverToPlayer checks a bot party whose leadership passes to a
// player: every slot clears, the bot whose claim succeeds stays and every
// other, a Damage member too, withdraws; a later unparty sends nothing and
// the withdrawn bot declines its old leader; to a commander every bot stays
// unclaimed; a bot with no slot keeps today's behavior.
func TestPartyHandoverToPlayer(t *testing.T) {
	toPlayer := slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot", slotMateID: "Mate"})
	t.Run("player", func(t *testing.T) {
		claims := NewPartyClaims()
		var withdrew []int
		var brains []*Brain
		var outs []*recordSender
		for _, bot := range []struct {
			id   int32
			role Role
		}{{selfID, RoleSupport}, {otherBotID, RoleDamage}, {thirdBotID, RoleSupport}} {
			b, l, out := claimsBrain(t, bot.id, joinSlot, &memberParty, bot.role, claims)
			handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, toPlayer)
			decideAt(t, b, 2*time.Second, 4*time.Second)
			cleared(t, b, l, at(4*time.Second))
			n := len(out.frames)
			// A later unparty sends nothing.
			b.unparty()
			decideAt(t, b, 6*time.Second)
			if len(out.frames) != n {
				t.Fatalf("bot %x sent % x after the unparty, want nothing", bot.id, out.frames[n:])
			}
			withdrew = append(withdrew, len(framesOf(out, opGameRequestWithdrawParty)))
			brains, outs = append(brains, b), append(outs, out)
		}
		if !slices.Equal(withdrew, []int{0, 1, 1}) {
			t.Fatalf("withdrawals %v, want the claiming bot staying, the others leaving once", withdrew)
		}
		if leader, _, ok := claims.held(selfID); !ok || leader != "Abbot" || claims.Held() != 1 {
			t.Fatalf("claim %q %v of %d, want the first bot holding Abbot", leader, ok, claims.Held())
		}
		// The withdrawn bot declines its old leader until the organizer
		// drops it.
		b := brains[1]
		handleAt(t, b, at(7*time.Second), PartySmallWindowDeleteAll{})
		if got := invite(t, b, outs[1], "Leader", at(8*time.Second)); len(got) != 1 || !bytes.Equal(got[0], declineFrame) {
			t.Fatalf("answers % x, want declined", got)
		}
	})
	t.Run("commander", func(t *testing.T) {
		claims := NewPartyClaims()
		toGM := slotParty(gmID, map[int32]string{gmID: "GM", slotMateID: "Mate"})
		for _, id := range []int32{selfID, otherBotID} {
			b, l, out := claimsBrain(t, id, joinSlot, &memberParty, RoleDamage, claims)
			handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, toGM)
			decideAt(t, b, 2*time.Second, 4*time.Second)
			cleared(t, b, l, at(4*time.Second))
			if n := len(framesOf(out, opGameRequestWithdrawParty)); n != 0 {
				t.Fatalf("bot %x: %d withdrawals, want none", id, n)
			}
		}
		if n := claims.Held(); n != 0 {
			t.Fatalf("%d claims, want none", n)
		}
	})
	t.Run("no slot", func(t *testing.T) {
		claims := NewPartyClaims()
		b, _, out := claimsBrain(t, selfID, Command{}, nil, RoleSupport, claims)
		invite(t, b, out, "Abbot", at(time.Second))
		handleAt(t, b, at(2*time.Second), slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot", zedID: "Zed"}))
		handleAt(t, b, at(3*time.Second), PartySmallWindowDeleteAll{}, slotParty(zedID, map[int32]string{slotPlayerID: "Abbot", zedID: "Zed"}))
		decideAt(t, b, 4*time.Second, 6*time.Second)
		if n := claims.Held(); n != 0 {
			t.Fatalf("%d claims, want none", n)
		}
		if n := len(framesOf(out, opGameRequestWithdrawParty)); n != 0 {
			t.Fatalf("%d withdrawals, want none", n)
		}
	})
}
