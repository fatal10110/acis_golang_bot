package botclient

import (
	"bytes"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"
)

// The members of the lead tests: Abel, Beth and Cole are bots a lead lists,
// Talbot and Zed players.
const (
	abelID   = int32(0x30000201)
	bethID   = int32(0x30000202)
	coleID   = int32(0x30000203)
	talbotID = int32(0x30000204)
	zedID    = int32(0x30000205)
)

var leadNames = map[int32]string{abelID: "Abel", bethID: "Beth", coleID: "Cole", talbotID: "Talbot", zedID: "Zed"}

// lead is a lead slot listing names.
func lead(names string) Command { return Command{Kind: CommandLead, Arg: names} }

// ledBy is the window of the party the bot leads with the members ids.
func ledBy(ids ...int32) PartySmallWindowAll {
	rows := make(map[int32]string, len(ids))
	for _, id := range ids {
		rows[id] = leadNames[id]
	}
	return slotParty(selfID, rows)
}

// decideAt has b decide at t0+d, whatever its tick.
func decideAt(t *testing.T, b *Brain, ds ...time.Duration) {
	t.Helper()
	for _, d := range ds {
		b.nextTick, b.reactAt = time.Time{}, time.Time{}
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
	}
}

// invited are the names out invited, in order; it fails on an invitation
// to anyone else or under any loot rule but by turn.
func invited(t *testing.T, out *recordSender) []string {
	t.Helper()
	var got []string
	for _, f := range framesOf(out, opGameRequestJoinParty) {
		i := slices.IndexFunc(inviteNames, func(n string) bool {
			return bytes.Equal(f, encodeRequestJoinParty(n, lootByTurn))
		})
		if i < 0 {
			t.Fatalf("invitation % x, want one of a listed member by turn", f)
		}
		got = append(got, inviteNames[i])
	}
	return got
}

// inviteNames are the names a lead test invites.
var inviteNames = []string{"Abel", "Beth", "Cole", "Mate"}

// wantInvited fails unless out invited want, in order.
func wantInvited(t *testing.T, out *recordSender, when string, want ...string) {
	t.Helper()
	if got := invited(t, out); !slices.Equal(got, want) {
		t.Fatalf("%s: invited %q, want %q", when, got, want)
	}
}

// TestPartyLeadCodec checks the leader's packets as the server decodes
// them (clientpackets/party.go): RequestOustPartyMember 0x2c and the name,
// RequestChangePartyLeader 0xd0 0x0004 and the name, the five loot rules,
// and the invitation under by turn.
func TestPartyLeadCodec(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestOustPartyMember", encodeRequestOustPartyMember("GM"), []byte{0x2c, 'G', 0, 'M', 0, 0, 0}},
		{"RequestChangePartyLeader", encodeRequestChangePartyLeader("GM"), []byte{0xd0, 0x04, 0x00, 'G', 0, 'M', 0, 0, 0}},
		{"invitation", inviteAction("GM").payload, []byte{0x29, 'G', 0, 'M', 0, 0, 0, 3, 0, 0, 0}},
		{"oust action", oustAction("GM").payload, []byte{0x2c, 'G', 0, 'M', 0, 0, 0}},
		{"change leader action", changeLeaderAction("GM").payload, []byte{0xd0, 0x04, 0x00, 'G', 0, 'M', 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
	loot := []int32{lootFindersKeepers, lootRandom, lootRandomSpoil, lootByTurn, lootByTurnSpoil}
	if !reflect.DeepEqual(loot, []int32{0, 1, 2, 3, 4}) || partyLoot != lootByTurn {
		t.Fatalf("loot rules %v, invitations under %d, want 0-4 and by turn", loot, partyLoot)
	}
}

// TestPartyLeadInvitesOneAtATime checks that a lead bot invites its listed
// members one at a time, in list order: the next only after the answer.
func TestPartyLeadInvitesOneAtATime(t *testing.T) {
	b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
	decideAt(t, b, 0, 2*time.Second, 5*time.Second, 10*time.Second)
	wantInvited(t, out, "before the answer", "Abel")
	handleAt(t, b, at(11*time.Second), JoinParty{Response: 1},
		PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: abelID, Name: "Abel"}})
	decideAt(t, b, 12*time.Second, 14*time.Second)
	wantInvited(t, out, "after the answer", "Abel", "Beth")
}

// TestPartySessionReinvite checks that a new session of a Bot holding a
// lead slot invites its listed members without a new order.
func TestPartySessionReinvite(t *testing.T) {
	b, l, out := clearBrain(t, lead("Abel"), nil, Persona{})
	decideAt(t, b, 0)
	wantInvited(t, out, "first session", "Abel")
	b2, out2 := newCommandBrain(t, Persona{})
	b2.cfg.link, b2.mail = l, l.mail
	decideAt(t, b2, time.Second)
	wantInvited(t, out2, "next session", "Abel")
}

// TestPartyLeadInviteTimeout checks the ends of an invitation: no answer
// within InviteWait (17 s) invites the next at once and the silent member
// 30 s later; a 164 (the bot's own window open) the same member 17 s later;
// a 153 or 185 that member 30 s later.
func TestPartyLeadInviteTimeout(t *testing.T) {
	t.Run("silent, next at once", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
		decideAt(t, b, 0, 15*time.Second)
		wantInvited(t, out, "at 15 s", "Abel")
		decideAt(t, b, 17*time.Second)
		wantInvited(t, out, "at 17 s", "Abel", "Beth")
		handleAt(t, b, at(18*time.Second), JoinParty{Response: 1},
			PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: bethID, Name: "Beth"}})
		decideAt(t, b, 20*time.Second, 46*time.Second)
		wantInvited(t, out, "at 46 s", "Abel", "Beth")
		decideAt(t, b, 47*time.Second)
		wantInvited(t, out, "at 47 s", "Abel", "Beth", "Abel")
	})
	t.Run("silent, alone", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel"), nil, Persona{})
		decideAt(t, b, 0, 17*time.Second, 34*time.Second)
		wantInvited(t, out, "at 34 s", "Abel")
		decideAt(t, b, 47*time.Second)
		wantInvited(t, out, "at 47 s", "Abel", "Abel")
	})
	t.Run("164, the same member 17 s later", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
		decideAt(t, b, 0)
		handleAt(t, b, at(time.Second), SystemMessage{ID: msgWaitingForReply})
		decideAt(t, b, 2*time.Second, 17*time.Second)
		wantInvited(t, out, "at 17 s", "Abel")
		decideAt(t, b, 18*time.Second)
		wantInvited(t, out, "at 18 s", "Abel", "Abel")
	})
	for _, m := range []SystemMessage{
		{ID: msgS1IsBusyTryLater, Text: [2]string{"Abel"}},
		{ID: msgNoSuchInvitee},
	} {
		t.Run(strconv.Itoa(int(m.ID)), func(t *testing.T) {
			b, _, out := clearBrain(t, lead("Abel"), nil, Persona{})
			decideAt(t, b, 0)
			handleAt(t, b, at(time.Second), m)
			decideAt(t, b, 2*time.Second, 18*time.Second, 30*time.Second)
			wantInvited(t, out, "at 30 s", "Abel")
			decideAt(t, b, 31*time.Second)
			wantInvited(t, out, "at 31 s", "Abel", "Abel")
		})
	}
}

// TestPartyLeadInviteRefusals checks that the refusals the bot reads end
// the invitation, the member invited again 30 s later and the next at once,
// and that a like message about anyone else does not.
func TestPartyLeadInviteRefusals(t *testing.T) {
	tests := []struct {
		name string
		msg  SystemMessage
		ends bool
	}{
		{"ignored by", SystemMessage{ID: msgIgnoredBy, Text: [2]string{"Abel"}}, true},
		{"blocks everyone", SystemMessage{ID: msgBlockedEverything, Text: [2]string{"Abel"}}, true},
		{"detached", SystemMessage{ID: msgS1, Text: [2]string{inviteDetachedText}}, true},
		{"jailed", SystemMessage{ID: msgS1, Text: [2]string{inviteJailedText}}, true},
		{"in a party", SystemMessage{ID: msgAlreadyInParty, Text: [2]string{"Abel"}}, true},
		{"wrong target", SystemMessage{ID: msgWrongTarget}, true},
		{"not the leader", SystemMessage{ID: msgOnlyLeaderInvites}, true},
		{"full", SystemMessage{ID: msgPartyFull}, true},
		{"another 1987", SystemMessage{ID: msgS1, Text: [2]string{"something else"}}, false},
		{"someone else busy", SystemMessage{ID: msgS1IsBusyTryLater, Text: [2]string{"Zed"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
			decideAt(t, b, 0)
			handleAt(t, b, at(time.Second), tt.msg)
			decideAt(t, b, 2*time.Second)
			if !tt.ends {
				wantInvited(t, out, "in flight", "Abel")
				return
			}
			wantInvited(t, out, "the next at once", "Abel", "Beth")
			handleAt(t, b, at(3*time.Second), JoinParty{Response: 1},
				PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: bethID, Name: "Beth"}})
			decideAt(t, b, 4*time.Second, 30*time.Second)
			wantInvited(t, out, "at 30 s", "Abel", "Beth")
			decideAt(t, b, 31*time.Second)
			wantInvited(t, out, "at 31 s", "Abel", "Beth", "Abel")
		})
	}
}

// TestPartyLeadInviteDeclined checks that a declined invitation ends at
// once: the next member at once, the one who declined 30 s later.
func TestPartyLeadInviteDeclined(t *testing.T) {
	b, _, out := clearBrain(t, lead("Abel Beth"), nil, Persona{})
	decideAt(t, b, 0)
	handleAt(t, b, at(time.Second), JoinParty{Response: 0})
	decideAt(t, b, 2*time.Second)
	wantInvited(t, out, "after the decline", "Abel", "Beth")
	handleAt(t, b, at(3*time.Second), JoinParty{Response: 1},
		PartySmallWindowAdd{LeaderID: selfID, Loot: lootByTurn, Member: PartyMember{ObjectID: bethID, Name: "Beth"}})
	decideAt(t, b, 4*time.Second, 30*time.Second)
	wantInvited(t, out, "at 30 s", "Abel", "Beth")
	decideAt(t, b, 31*time.Second)
	wantInvited(t, out, "at 31 s", "Abel", "Beth", "Abel")
}

// TestPartyLeadRejoinWait checks that a listed member who left the party,
// or every member of a party of two that dispersed, is invited again only
// 30 s later.
func TestPartyLeadRejoinWait(t *testing.T) {
	tests := []struct {
		name  string
		slot  Command
		party PartySmallWindowAll
		left  []Event
	}{
		{"left", lead("Abel Beth"), ledBy(abelID, bethID), []Event{
			SystemMessage{ID: msgLeftParty, Text: [2]string{"Abel"}},
			PartySmallWindowDelete{ObjectID: abelID, Name: "Abel"},
		}},
		{"dispersed", lead("Abel"), ledBy(abelID), []Event{PartySmallWindowDeleteAll{}, SystemMessage{ID: 203}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, out := clearBrain(t, tt.slot, &tt.party, Persona{})
			handleAt(t, b, at(time.Second), tt.left...)
			decideAt(t, b, 2*time.Second, 16*time.Second, 30*time.Second)
			wantInvited(t, out, "at 30 s")
			decideAt(t, b, 31*time.Second)
			wantInvited(t, out, "at 31 s", "Abel")
		})
	}
}

// TestPartyLeadInsideOtherParty checks that a lead bot in a player's party
// invites nobody: the window clears its slot.
func TestPartyLeadInsideOtherParty(t *testing.T) {
	b, l, out := clearBrain(t, lead("Abel"), nil, Persona{JoinParty: acceptParty})
	handleAt(t, b, at(time.Second), slotParty(talbotID, map[int32]string{talbotID: "Talbot"}))
	decideAt(t, b, 2*time.Second, 20*time.Second, 40*time.Second)
	wantInvited(t, out, "in a player's party")
	if l.party != (Command{}) {
		t.Fatalf("slot %q, want empty", l.party)
	}
}

// TestPartyLeadOustsDropped checks that a member a lead dropped is expelled,
// one per decision and once, and that a player no lead listed never is.
func TestPartyLeadOustsDropped(t *testing.T) {
	party := ledBy(abelID, bethID, coleID, talbotID)
	b, _, out := clearBrain(t, lead("Abel Beth Cole"), &party, Persona{})
	decideAt(t, b, time.Second)
	if got := framesOf(out, opGameRequestOustPartyMember); len(got) != 0 {
		t.Fatalf("ousted % x while listed, want none", got)
	}
	b.setPartySlot(lead("Abel"))
	decideAt(t, b, 2*time.Second)
	want := [][]byte{encodeRequestOustPartyMember("Beth")}
	if got := framesOf(out, opGameRequestOustPartyMember); !reflect.DeepEqual(got, want) {
		t.Fatalf("ousted % x, want Beth alone", got)
	}
	decideAt(t, b, 3*time.Second, 4*time.Second, 5*time.Second)
	want = append(want, encodeRequestOustPartyMember("Cole"))
	if got := framesOf(out, opGameRequestOustPartyMember); !reflect.DeepEqual(got, want) {
		t.Fatalf("ousted % x, want Beth then Cole, once each", got)
	}
	wantInvited(t, out, "all in")
}

// TestPartyHandoverLeaves checks a join member the server made leader: it
// keeps its slot and waits for no lost leader; with only bots its slot
// names it withdraws at once, else it hands the party to its players, the
// lowest object id first whatever the window order, the next each 5 s and
// the lowest after the last, holding where it stands, and withdraws at
// 2.5 minutes or on an unparty; a window naming a player leader ends it,
// and a bot with no slot sends neither.
func TestPartyHandoverLeaves(t *testing.T) {
	withdrawals := func(out *recordSender) int { return len(framesOf(out, opGameRequestWithdrawParty)) }
	asked := func(out *recordSender) []string {
		var got []string
		for _, f := range framesOf(out, opGameClientExtended) {
			for _, n := range []string{"Abbot", "Talbot", "Zed", "Mate"} {
				if bytes.Equal(f, encodeRequestChangePartyLeader(n)) {
					got = append(got, n)
				}
			}
		}
		return got
	}
	tests := []struct {
		name     string
		slot     Command
		rows     map[int32]string
		tells    int
		lost     bool
		withdraw int
		asked    []string
	}{
		{"bots only", joinSlot, map[int32]string{slotMateID: "Mate"}, 0, false, 1, nil},
		{"with a player", joinSlot, map[int32]string{slotMateID: "Mate", slotPlayerID: "Abbot"}, 0, false, 0, []string{"Abbot"}},
		{"with Talbot", joinSlot, map[int32]string{slotMateID: "Mate", talbotID: "Talbot"}, 0, false, 0, []string{"Talbot"}},
		{"no slot", Command{}, map[int32]string{talbotID: "Talbot"}, 1, true, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, l, out := clearBrain(t, tt.slot, &memberParty, waiter)
			handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, slotParty(selfID, tt.rows))
			decideAt(t, b, 2*time.Second, 4*time.Second)
			if l.party != tt.slot {
				t.Fatalf("slot %q, want %q kept", l.party, tt.slot)
			}
			if n := withdrawals(out); n != tt.withdraw {
				t.Fatalf("%d withdrawals, want %d", n, tt.withdraw)
			}
			if got := asked(out); !slices.Equal(got, tt.asked) {
				t.Fatalf("asked %q, want %q", got, tt.asked)
			}
			if tt.slot.Kind != 0 {
				return
			}
			wakeAll(t, b, 6*time.Minute, 7*time.Minute, 8*time.Minute)
			if lost := !b.lead.lost.IsZero(); lost != tt.lost {
				t.Fatalf("leader lost %v, want %v", lost, tt.lost)
			}
			if n := len(framesOf(out, opGameSay2)); n != tt.tells {
				t.Fatalf("%d whispers, want %d", n, tt.tells)
			}
			if n := withdrawals(out) + len(asked(out)); n != 0 {
				t.Fatalf("%d withdrawals and handovers, want none", n)
			}
		})
	}
	t.Run("turns and timeout", func(t *testing.T) {
		b, l, out := clearBrain(t, joinSlot, &memberParty, waiter)
		handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{},
			slotParty(selfID, map[int32]string{slotMateID: "Mate", zedID: "Zed", talbotID: "Talbot"}))
		decideAt(t, b, 2*time.Second, 4*time.Second, 7*time.Second, 9*time.Second, 12*time.Second)
		if got, want := asked(out), []string{"Talbot", "Zed", "Talbot"}; !slices.Equal(got, want) {
			t.Fatalf("asked %q, want %q", got, want)
		}
		for d := 15 * time.Second; d < 150*time.Second; d += 5 * time.Second {
			decideAt(t, b, d)
		}
		if n := withdrawals(out); n != 0 || b.spot.held || len(framesOf(out, opGameMoveBackwardToLocation)) != 0 {
			t.Fatalf("%d withdrawals, spot held %v, %d moves before 2.5 min, want none", n, b.spot.held, len(framesOf(out, opGameMoveBackwardToLocation)))
		}
		n := len(asked(out))
		decideAt(t, b, 153*time.Second, 160*time.Second)
		if withdrawals(out) != 1 || len(asked(out)) != n || l.party != joinSlot {
			t.Fatalf("%d withdrawals, %d more asks, slot %q at 2.5 min, want one withdrawal and the slot kept", withdrawals(out), len(asked(out))-n, l.party)
		}
	})
	t.Run("unparty", func(t *testing.T) {
		b, l, out := clearBrain(t, joinSlot, &memberParty, waiter)
		handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, slotParty(selfID, map[int32]string{talbotID: "Talbot"}))
		decideAt(t, b, 2*time.Second)
		b.unparty()
		decideAt(t, b, 3*time.Second, 9*time.Second)
		if withdrawals(out) != 1 || len(asked(out)) != 1 || l.party != (Command{}) {
			t.Fatalf("%d withdrawals, asked %q, slot %q, want one withdrawal after one ask", withdrawals(out), asked(out), l.party)
		}
	})
	t.Run("player made leader", func(t *testing.T) {
		b, l, out := clearBrain(t, joinSlot, &memberParty, waiter)
		handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, slotParty(selfID, map[int32]string{talbotID: "Talbot"}))
		decideAt(t, b, 2*time.Second)
		handleAt(t, b, at(3*time.Second), PartySmallWindowDeleteAll{}, slotParty(talbotID, map[int32]string{talbotID: "Talbot"}))
		decideAt(t, b, 4*time.Second, 9*time.Second, 3*time.Minute)
		if withdrawals(out) != 0 || len(asked(out)) != 1 || l.party != (Command{}) {
			t.Fatalf("%d withdrawals, asked %q, slot %q, want the slot cleared and nothing more sent", withdrawals(out), asked(out), l.party)
		}
	})
}

// TestPartyWhisperNoReformLeader checks the leader's half: after a GM's
// leave the leader invites nobody; a member who left (its invite me) is
// invited 30 s after it left, and after its decline 30 s later again.
func TestPartyWhisperNoReformLeader(t *testing.T) {
	t.Run("leave", func(t *testing.T) {
		b, l, out := clearBrain(t, lead("Mate"), &leadParty, Persona{})
		gmSays(b, cmdLeave, "", at(time.Second))
		cleared(t, b, l, at(time.Second))
		decideAt(t, b, 2*time.Second)
		handleAt(t, b, at(3*time.Second), PartySmallWindowDeleteAll{}, SystemMessage{ID: 203})
		for d := 4 * time.Second; d < 2*time.Minute; d += 5 * time.Second {
			decideAt(t, b, d)
		}
		if n := len(framesOf(out, opGameRequestJoinParty)); n != 0 {
			t.Fatalf("%d invitations, want none", n)
		}
	})
	t.Run("member's invite me", func(t *testing.T) {
		b, _, out := clearBrain(t, lead("Mate"), &leadParty, Persona{})
		handleAt(t, b, at(time.Second), PartySmallWindowDeleteAll{}, SystemMessage{ID: 203})
		decideAt(t, b, 2*time.Second, 30*time.Second)
		if n := len(framesOf(out, opGameRequestJoinParty)); n != 0 {
			t.Fatalf("%d invitations before 31 s, want none", n)
		}
		decideAt(t, b, 31*time.Second)
		handleAt(t, b, at(32*time.Second), JoinParty{Response: 0})
		decideAt(t, b, 33*time.Second, 61*time.Second)
		if n := len(framesOf(out, opGameRequestJoinParty)); n != 1 {
			t.Fatalf("%d invitations before 62 s, want 1", n)
		}
		decideAt(t, b, 62*time.Second)
		want := [][]byte{encodeRequestJoinParty("Mate", lootByTurn), encodeRequestJoinParty("Mate", lootByTurn)}
		if got := framesOf(out, opGameRequestJoinParty); !reflect.DeepEqual(got, want) {
			t.Fatalf("invitations % x, want Mate at 31 s and 62 s", got)
		}
	})
}
