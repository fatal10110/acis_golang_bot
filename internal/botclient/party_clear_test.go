package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The bot parties of the clearing tests: a join member in its leader's
// party, a lead bot leading its own.
var (
	joinSlot    = Command{Kind: CommandJoin, Arg: "Leader Mate"}
	leadSlot    = Command{Kind: CommandLead, Arg: "Mate"}
	memberParty = slotParty(slotLeaderID, map[int32]string{slotLeaderID: "Leader", slotMateID: "Mate"})
	leadParty   = slotParty(selfID, map[int32]string{slotMateID: "Mate"})
	// waiter waits LeaderWait for a lost leader, then whispers it.
	waiter = Persona{JoinParty: acceptParty, Recover: RecoverPolicy{LeaderWait: 5 * time.Minute, Whisper: "where are you"}}
)

// clearBrain is a brain obeying the commander GM, with a Bot's link
// holding slot and a farming spot "Wolves", handed party (nil for none) as
// a session is.
func clearBrain(t *testing.T, slot Command, party *PartySmallWindowAll, p Persona) (*Brain, *botLink, *recordSender) {
	t.Helper()
	b, out := newCommandBrain(t, p)
	b.world.Self.Level = 2
	b.cfg.Spots = NewSpotRegistry([]datapack.Spot{testSpot("Wolves", 5000, 0, 1, 3, 3)}, 1)
	l := linkBrain(b, standing{})
	if slot.Kind != 0 {
		b.setPartySlot(slot)
	}
	if party != nil {
		handleAt(t, b, t0, *party)
	}
	return b, l, out
}

// handleAt hands b the events at now.
func handleAt(t *testing.T, b *Brain, now time.Time, evs ...Event) {
	t.Helper()
	if err := b.handle(evs, now); err != nil {
		t.Fatal(err)
	}
}

// gmSays has b obey the GM's whispered command kind (spot for a go) at now.
func gmSays(b *Brain, kind commandKind, spot string, now time.Time) {
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: kind, spot: spot}}, ok: true}, now)
}

// cleared fails unless b's slot is empty and its status reads left.
func cleared(t *testing.T, b *Brain, l *botLink, now time.Time) {
	t.Helper()
	b.publishStatus(now, true)
	if st := l.board.load(); l.party != (Command{}) || st.Party != "" || st.PartyLeft != partyLeftLeft {
		t.Fatalf("slot %q status party %q left %q, want cleared and left", l.party, st.Party, st.PartyLeft)
	}
}

// noLostLeader fails when out holds a whisper or an /unstuck.
func noLostLeader(t *testing.T, out *recordSender) {
	t.Helper()
	if n, m := len(framesOf(out, opGameSay2)), len(framesOf(out, opGameRequestUserCommand)); n+m != 0 {
		t.Fatalf("%d whispers and %d /unstuck, want none", n, m)
	}
}

// TestPartyJoinPlayerClears checks the player-led-party rule: a join or a
// lead bot accepting a player's invitation, a marked player's too, clears
// its slot on the next window, sending nothing; a window of its own bot
// party keeps it, and a bot with no slot behaves as today.
func TestPartyJoinPlayerClears(t *testing.T) {
	tests := []struct {
		name   string
		slot   Command
		window PartySmallWindowAll
		clears bool
	}{
		{"join, player", joinSlot, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot"}), true},
		{"join, marked player", joinSlot, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"}), true},
		{"join, case differs", joinSlot, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "leader"}), true},
		{"lead, player", leadSlot, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot"}), true},
		{"lead, marked player", leadSlot, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"}), true},
		{"lead, its listed member", leadSlot, slotParty(slotMateID, map[int32]string{slotMateID: "Mate"}), true},
		{"join, its leader", joinSlot, memberParty, false},
		{"join, its listed member", joinSlot, slotParty(slotMateID, map[int32]string{slotMateID: "Mate", slotLeaderID: "Leader"}), false},
		{"lead, leading", leadSlot, leadParty, false},
		{"no slot", Command{}, slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, l, out := clearBrain(t, tt.slot, nil, Persona{JoinParty: acceptParty})
			leader := "Abbot"
			for _, m := range tt.window.Members {
				if m.ObjectID == tt.window.LeaderID {
					leader = m.Name
				}
			}
			if got := invite(t, b, out, leader, at(time.Second)); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
				t.Fatalf("answers % x, want accepted", got)
			}
			if l.party != tt.slot {
				t.Fatalf("slot %q before the window, want %q", l.party, tt.slot)
			}
			handleAt(t, b, at(2*time.Second), tt.window)
			wakeAll(t, b, 3*time.Second, 5*time.Second)
			if tt.clears {
				cleared(t, b, l, at(5*time.Second))
			} else if l.party != tt.slot {
				t.Fatalf("slot %q, want %q kept", l.party, tt.slot)
			}
			if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
				t.Fatalf("withdrawals % x, want none", got)
			}
		})
	}
	t.Run("status", func(t *testing.T) {
		b, l, _ := clearBrain(t, joinSlot, ptr(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"})), Persona{})
		cleared(t, b, l, at(time.Second))
		bot, _, _ := startBot(t, true, &fakeNet{})
		bot.link.partyLeft = partyLeftLeft
		bot.publish(StateReconnecting, nil)
		if st := bot.Status(); st.PartyLeft != partyLeftLeft {
			t.Fatalf("offline left %q, want left", st.PartyLeft)
		}
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader"}, at(2*time.Second))
		b.publishStatus(at(2*time.Second), true)
		if st := l.board.load(); st.PartyLeft != "" {
			t.Fatalf("left %q under a new slot, want none", st.PartyLeft)
		}
	})
}

// TestPartyPanelLeaveClears checks the panel's leave: it clears the slot
// before its no-party check and withdraws through the flag.
func TestPartyPanelLeaveClears(t *testing.T) {
	tests := []struct {
		name     string
		slot     Command
		party    *PartySmallWindowAll
		err      error
		withdraw bool
	}{
		{"member", joinSlot, &memberParty, nil, true},
		{"leader", leadSlot, &leadParty, nil, true},
		{"partyless with a slot", joinSlot, nil, nil, false},
		{"no slot, no party", Command{}, nil, ErrNoParty, false},
		{"no slot, a player's party", Command{}, ptr(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot"})), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, l, out := clearBrain(t, tt.slot, tt.party, Persona{JoinParty: acceptParty})
			b.lead = leaderWatch{id: slotLeaderID, name: "Leader", lost: t0}
			if err := panel(b, Command{Kind: CommandLeave}, at(time.Second)); !errors.Is(err, tt.err) {
				t.Fatalf("leave = %v, want %v", err, tt.err)
			}
			if tt.slot.Kind != 0 {
				cleared(t, b, l, at(time.Second))
			}
			if tt.party == nil && tt.slot.Kind != 0 && b.lead != (leaderWatch{}) {
				t.Fatalf("watch %+v after a clear in no party, want reset", b.lead)
			}
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Second)
			if got := framesOf(out, opGameRequestWithdrawParty); (len(got) == 1) != tt.withdraw || len(got) > 1 {
				t.Fatalf("withdrawals % x, want %v once", got, tt.withdraw)
			}
			if got := framesOf(out, opGameRequestJoinParty); len(got) != 0 {
				t.Fatalf("invitations % x, want none", got)
			}
			if tt.slot.Kind == CommandJoin {
				handleAt(t, b, at(7*time.Second), PartySmallWindowDeleteAll{})
				if got := invite(t, b, out, "Leader", at(8*time.Second)); !reflect.DeepEqual(got, [][]byte{declineFrame}) {
					t.Fatalf("old leader answered % x, want declined", got)
				}
			}
		})
	}
}

// TestPartyWhisperedOrderClears checks that a GM's whispered order and the
// panel's standing orders clear the slot and withdraw ahead of the order,
// and that a whispered follow holds against a new slot.
func TestPartyWhisperedOrderClears(t *testing.T) {
	for _, kind := range []commandKind{cmdStay, cmdGo, cmdFollow, cmdAssist} {
		t.Run("gm "+commandName(kind), func(t *testing.T) {
			b, l, out := clearBrain(t, joinSlot, &memberParty, Persona{})
			gmSays(b, kind, "Wolves", at(time.Second))
			cleared(t, b, l, at(time.Second))
			if kind == cmdGo && (b.cfg.Spots.Held(0) != 1 || !b.spot.held) {
				t.Fatalf("go: wolves held by %d, spot %+v", b.cfg.Spots.Held(0), b.spot)
			}
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Second)
			if len(out.frames) == 0 || !bytes.Equal(out.frames[0], withdrawFrame) || len(framesOf(out, opGameRequestWithdrawParty)) != 1 {
				t.Fatalf("sent % x, want the withdrawal first and once", opcodes(out))
			}
		})
	}
	for _, c := range []Command{{Kind: CommandStay}, {Kind: CommandGo, Arg: "Wolves"}, {Kind: CommandFollow, Arg: "GM"}, {Kind: CommandAssist, Arg: "GM"}} {
		t.Run("panel "+c.String(), func(t *testing.T) {
			b, l, out := clearBrain(t, joinSlot, &memberParty, Persona{})
			mustPanel(t, b, c, at(time.Second))
			cleared(t, b, l, at(time.Second))
			if !b.orders.withdraw {
				t.Fatalf("%q dropped the withdrawal", c)
			}
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Second)
			if len(out.frames) == 0 || !bytes.Equal(out.frames[0], withdrawFrame) || len(framesOf(out, opGameRequestWithdrawParty)) != 1 {
				t.Fatalf("sent % x, want the withdrawal first and once", opcodes(out))
			}
		})
	}
	t.Run("panel follow, partyless member", func(t *testing.T) {
		b, l, out := clearBrain(t, joinSlot, nil, Persona{JoinParty: acceptParty})
		mustPanel(t, b, Command{Kind: CommandFollow, Arg: "GM"}, at(time.Second))
		cleared(t, b, l, at(time.Second))
		wakeAll(t, b, 2*time.Second, 4*time.Second)
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
			t.Fatalf("withdrawals % x, want none", got)
		}
		if got := invite(t, b, out, "Leader", at(5*time.Second)); !reflect.DeepEqual(got, [][]byte{declineFrame}) {
			t.Fatalf("old leader answered % x, want declined", got)
		}
	})
	t.Run("panel go refused keeps the slot", func(t *testing.T) {
		b, l, _ := clearBrain(t, joinSlot, &memberParty, Persona{})
		if err := panel(b, Command{Kind: CommandGo, Arg: "nowhere"}, at(time.Second)); !errors.Is(err, ErrUnknownSpot) {
			t.Fatalf("go nowhere = %v, want ErrUnknownSpot", err)
		}
		if l.party != joinSlot || b.orders.withdraw {
			t.Fatalf("slot %q withdraw %v after a refused go, want kept", l.party, b.orders.withdraw)
		}
	})
	t.Run("gm follow holds against a slot", func(t *testing.T) {
		b, l, _ := clearBrain(t, joinSlot, &memberParty, Persona{})
		gmSays(b, cmdFollow, "", at(time.Second))
		for _, c := range []Command{{Kind: CommandJoin, Arg: "Leader"}, {Kind: CommandLead, Arg: "Mate"}} {
			if err := panel(b, c, at(2*time.Second)); !errors.Is(err, ErrWhispered) {
				t.Fatalf("%q = %v, want ErrWhispered", c, err)
			}
		}
		cleared(t, b, l, at(2*time.Second))
		if !l.board.load().Whispered {
			t.Fatal("status not whispered")
		}
	})
}

// commandName names a whispered command kind for a subtest.
func commandName(k commandKind) string {
	return map[commandKind]string{cmdStay: "stay", cmdGo: "go", cmdFollow: "follow", cmdAssist: "assist"}[k]
}

// TestPartyWithdrawLapses checks that the withdrawal a GM's leave set
// lapses with the party: a disband before the next decision sends neither
// it nor a lost-leader whisper or /unstuck, nor after a GM's invitation and
// window; an invitation answered in the same decision drops it; a leader
// change keeps it.
func TestPartyWithdrawLapses(t *testing.T) {
	for _, slot := range []Command{joinSlot, {}} {
		t.Run("disband, slot "+slot.String(), func(t *testing.T) {
			b, _, out := clearBrain(t, slot, &memberParty, waiter)
			gmSays(b, cmdLeave, "", at(time.Second))
			handleAt(t, b, at(time.Second+10*time.Millisecond), PartySmallWindowDeleteAll{})
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Minute, 7*time.Minute, 8*time.Minute)
			if got := invite(t, b, out, "GM", at(9*time.Minute)); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
				t.Fatalf("GM's invitation answered % x, want accepted", got)
			}
			handleAt(t, b, at(9*time.Minute+time.Second), slotParty(gmID, map[int32]string{gmID: "GM"}))
			wakeAll(t, b, 10*time.Minute, 11*time.Minute)
			if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
				t.Fatalf("withdrawals % x, want none", got)
			}
			noLostLeader(t, out)
		})
	}
	t.Run("disband and invitation in one decision", func(t *testing.T) {
		b, _, out := clearBrain(t, joinSlot, &memberParty, waiter)
		gmSays(b, cmdLeave, "", at(time.Second))
		handleAt(t, b, at(time.Second+10*time.Millisecond), PartySmallWindowDeleteAll{}, AskJoinParty{Requester: "Talbot", Loot: 1})
		if err := b.wake(b.decisionAt()); err != nil {
			t.Fatal(err)
		}
		if got := framesOf(out, opGameRequestAnswerJoinParty); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
			t.Fatalf("answers % x, want accepted", got)
		}
		handleAt(t, b, at(3*time.Second), slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"}))
		wakeAll(t, b, 4*time.Second, 6*time.Second)
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
			t.Fatalf("withdrawals % x, want none", got)
		}
	})
	t.Run("leader change between", func(t *testing.T) {
		b, _, out := clearBrain(t, joinSlot, &memberParty, waiter)
		gmSays(b, cmdLeave, "", at(time.Second))
		handleAt(t, b, at(time.Second+10*time.Millisecond), slotParty(slotMateID, map[int32]string{slotLeaderID: "Leader", slotMateID: "Mate"}))
		wakeAll(t, b, 2*time.Second, 4*time.Second)
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 1 {
			t.Fatalf("withdrawals % x, want one", got)
		}
	})
}

// TestPartyWhisperNoReform checks that a GM's leave or invite me takes a
// member out of its bot party through the clearing step and that its old
// leader's next invitation is declined; a GM's leave takes a lead bot out
// of the party it leads.
func TestPartyWhisperNoReform(t *testing.T) {
	tests := []struct {
		name  string
		slot  Command
		party PartySmallWindowAll
		kind  commandKind
		tells int
	}{
		{"leave, member", joinSlot, memberParty, cmdLeave, 0},
		{"invite me, member", joinSlot, memberParty, cmdInviteMe, 1},
		{"leave, leader", leadSlot, leadParty, cmdLeave, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, l, out := clearBrain(t, tt.slot, &tt.party, Persona{JoinParty: acceptParty})
			gmSays(b, tt.kind, "", at(time.Second))
			cleared(t, b, l, at(time.Second))
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Second)
			if len(out.frames) == 0 || !bytes.Equal(out.frames[0], withdrawFrame) || len(framesOf(out, opGameRequestWithdrawParty)) != 1 {
				t.Fatalf("sent % x, want the withdrawal first and once", opcodes(out))
			}
			if n := len(framesOf(out, opGameSay2)); n != tt.tells {
				t.Fatalf("%d whispers, want %d", n, tt.tells)
			}
			if tt.slot.Kind != CommandJoin {
				return
			}
			handleAt(t, b, at(7*time.Second), PartySmallWindowDeleteAll{})
			if got := invite(t, b, out, "Leader", at(8*time.Second)); !reflect.DeepEqual(got, [][]byte{declineFrame}) {
				t.Fatalf("old leader answered % x, want declined", got)
			}
		})
	}
}

// TestPartyInviteMeGM checks that a GM's invite me and a GM's go to an
// unknown spot take a member out of its bot party before their whisper.
func TestPartyInviteMeGM(t *testing.T) {
	for _, tt := range []struct {
		name string
		kind commandKind
		spot string
		text string
	}{
		{"invite me", cmdInviteMe, "", inviteMeText},
		{"go nowhere", cmdGo, "nowhere", replyUnknownSpot},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, l, out := clearBrain(t, joinSlot, &memberParty, Persona{})
			gmSays(b, tt.kind, tt.spot, at(time.Second))
			cleared(t, b, l, at(time.Second))
			wakeAll(t, b, 2*time.Second, 4*time.Second, 6*time.Second)
			want := [][]byte{withdrawFrame, tellAction("GM", tt.text).payload}
			if len(out.frames) < 2 || !reflect.DeepEqual(out.frames[:2], want) {
				t.Fatalf("sent % x, want the withdrawal then the whisper", out.frames)
			}
		})
	}
}
