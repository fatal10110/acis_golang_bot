package botclient

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

const (
	slotLeaderID = int32(0x30000101)
	slotMateID   = int32(0x30000102)
	slotPlayerID = int32(0x30000103)
)

// newSlotBrain is a started brain over a test world at full HP and MP,
// playing p, lent l as a session is.
func newSlotBrain(t *testing.T, l *botLink, p Persona) (*Brain, *recordSender) {
	t.Helper()
	w := newTestWorld(t)
	w.Self.HP, w.Self.MaxHP, w.Self.MP, w.Self.MaxMP = 100, 100, 100, 100
	out := &recordSender{}
	b := NewBrain(w, NewEvents(64), out, newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(1, 2)), link: l})
	b.start(t0)
	return b, out
}

// newSlotLink is a Bot's link holding order.
func newSlotLink(order standing) *botLink {
	l := &botLink{mail: make(chan mail, mailboxSize), order: order}
	l.board.store(Status{})
	return l
}

// panel hands c to b as the panel does to a running session and returns
// its answer.
func panel(b *Brain, c Command, now time.Time) error {
	m := mail{cmd: c, answer: make(chan error, 1)}
	b.command(m, now)
	return <-m.answer
}

// mustPanel hands c to b and fails on an answer other than nil.
func mustPanel(t *testing.T, b *Brain, c Command, now time.Time) {
	t.Helper()
	if err := panel(b, c, now); err != nil {
		t.Fatalf("%q = %v, want nil", c, err)
	}
}

// slotParty is the window of a party led by leader with the members named
// in rows, by object id.
func slotParty(leader int32, rows map[int32]string) PartySmallWindowAll {
	e := PartySmallWindowAll{LeaderID: leader, Loot: 1}
	for id, name := range rows {
		e.Members = append(e.Members, PartyMember{ObjectID: id, Name: name})
	}
	return e
}

// wakeAll wakes b at each offset.
func wakeAll(t *testing.T, b *Brain, offsets ...time.Duration) {
	t.Helper()
	for _, d := range offsets {
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
	}
}

// framesOf are the frames of out with opcode op.
func framesOf(out *recordSender, op byte) [][]byte {
	var got [][]byte
	for _, f := range out.frames {
		if f[0] == op {
			got = append(got, f)
		}
	}
	return got
}

// invite hands b an invitation from requester at ev and wakes it at its
// reaction; it returns the answers sent.
func invite(t *testing.T, b *Brain, out *recordSender, requester string, ev time.Time) [][]byte {
	t.Helper()
	n := len(framesOf(out, opGameRequestAnswerJoinParty))
	if err := b.handle([]Event{AskJoinParty{Requester: requester, Loot: 1}}, ev); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(b.decisionAt()); err != nil {
		t.Fatal(err)
	}
	return framesOf(out, opGameRequestAnswerJoinParty)[n:]
}

var (
	acceptFrame   = botwire.EncodeRequestAnswerJoinParty(true)
	declineFrame  = botwire.EncodeRequestAnswerJoinParty(false)
	withdrawFrame = []byte{opGameRequestWithdrawParty}
)

// TestPartyCommandValid checks the party commands Bot.Command takes: lead
// and join name 1 to 8 characters, single-spaced, each 1 to 16 letters or
// digits, none twice; unparty takes none.
func TestPartyCommandValid(t *testing.T) {
	sixteen := strings.Repeat("a", 16)
	tests := []struct {
		cmd  Command
		want bool
	}{
		{Command{Kind: CommandLead, Arg: "A"}, true},
		{Command{Kind: CommandLead, Arg: "A B"}, true},
		{Command{Kind: CommandJoin, Arg: "L"}, true},
		{Command{Kind: CommandJoin, Arg: "L A B"}, true},
		{Command{Kind: CommandJoin, Arg: "L A B C D E F G"}, true},
		{Command{Kind: CommandLead, Arg: sixteen + " Bot9"}, true},
		{Command{Kind: CommandUnparty}, true},
		{Command{Kind: CommandLead}, false},
		{Command{Kind: CommandJoin}, false},
		{Command{Kind: CommandJoin, Arg: "L A B C D E F G H"}, false},
		{Command{Kind: CommandLead, Arg: "A B A"}, false},
		{Command{Kind: CommandLead, Arg: "A b a"}, false},
		{Command{Kind: CommandJoin, Arg: "L  A"}, false},
		{Command{Kind: CommandJoin, Arg: " L A"}, false},
		{Command{Kind: CommandJoin, Arg: "L A "}, false},
		{Command{Kind: CommandLead, Arg: sixteen + "a"}, false},
		{Command{Kind: CommandLead, Arg: "A-B"}, false},
		{Command{Kind: CommandLead, Arg: "Ä"}, false},
		{Command{Kind: CommandUnparty, Arg: "A"}, false},
		{Command{Kind: CommandUnparty + 1}, false},
	}
	for _, tt := range tests {
		if got := tt.cmd.valid(); got != tt.want {
			t.Errorf("%q valid = %v, want %v", tt.cmd, got, tt.want)
		}
		_, err := (&Bot{}).Command(tt.cmd)
		if !tt.want && !errors.Is(err, ErrBadCommand) {
			t.Errorf("Command(%q) = %v, want ErrBadCommand", tt.cmd, err)
		}
	}
	for k, want := range map[CommandKind]string{CommandLead: "lead", CommandJoin: "join", CommandUnparty: "unparty"} {
		if k.String() != want {
			t.Errorf("%d String = %q, want %q", k, k, want)
		}
	}
}

// TestPartyJoinReplacesStanding checks that a join or a lead replaces the
// panel's standing order: it is cleared, the bot follows its party leader
// instead, and the next session applies no standing order.
func TestPartyJoinReplacesStanding(t *testing.T) {
	t.Run("join over follow", func(t *testing.T) {
		l := newSlotLink(standing{})
		b, out := newSlotBrain(t, l, Persona{})
		mustPanel(t, b, Command{Kind: CommandFollow, Arg: "other"}, at(time.Second))
		if b.followed() != pcID {
			t.Fatalf("follows %d, want the player", b.followed())
		}
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader Mate"}, at(2*time.Second))
		b.publishStatus(at(2*time.Second), true)
		if st := l.board.load(); st.Order != (Command{}) || st.Party != "join Leader Mate" {
			t.Fatalf("status order %q party %q, want none and the join", st.Order, st.Party)
		}
		if b.followed() != 0 || b.orderPending {
			t.Fatalf("follows %d (pending %v) after the join", b.followed(), b.orderPending)
		}
		if got := invite(t, b, out, "Leader", at(3*time.Second)); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
			t.Fatalf("answers % x, want the leader accepted", got)
		}
		b.world.Apply(slotParty(slotLeaderID, map[int32]string{slotLeaderID: "Leader"}), at(4*time.Second))
		if b.followed() != slotLeaderID {
			t.Fatalf("follows %d, want the leader", b.followed())
		}
		next, _ := newSlotBrain(t, l, Persona{})
		if !reflect.DeepEqual(next.orders, orders{}) || next.orderPending {
			t.Fatalf("next session orders %+v pending %v, want none", next.orders, next.orderPending)
		}
	})
	t.Run("lead over stay", func(t *testing.T) {
		l := newSlotLink(standing{cmd: Command{Kind: CommandStay}})
		b, _ := newSlotBrain(t, l, Persona{})
		if !b.orders.stay {
			t.Fatal("standing stay not applied")
		}
		mustPanel(t, b, Command{Kind: CommandLead, Arg: "Mate"}, at(time.Second))
		if b.orders.stay || l.order != (standing{}) || l.party != (Command{Kind: CommandLead, Arg: "Mate"}) {
			t.Fatalf("orders %+v standing %+v slot %q after the lead", b.orders, l.order, l.party)
		}
		if name, _ := b.decide(at(2 * time.Second)); name == "stay" {
			t.Fatal("still staying after the lead")
		}
		next, _ := newSlotBrain(t, l, Persona{})
		if next.orders.stay {
			t.Fatal("next session applied the old stay")
		}
	})
}

// TestPartyUnparty checks that unparty clears the slot and withdraws only
// from the bot party the slot named.
func TestPartyUnparty(t *testing.T) {
	tests := []struct {
		name     string
		slot     Command
		party    *PartySmallWindowAll // nil for none
		withdraw bool
	}{
		{
			"join member in its bot party",
			Command{Kind: CommandJoin, Arg: "Leader Mate"},
			ptr(slotParty(slotLeaderID, map[int32]string{slotLeaderID: "Leader", slotMateID: "Mate"})), true,
		},
		{
			"lead leading its bot party",
			Command{Kind: CommandLead, Arg: "Mate"},
			ptr(slotParty(selfID, map[int32]string{slotMateID: "Mate"})), true,
		},
		{
			"join member in a player's party",
			Command{Kind: CommandJoin, Arg: "Leader"},
			ptr(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot", slotLeaderID: "Leader"})), false,
		},
		{
			"lead in a player's party",
			Command{Kind: CommandLead, Arg: "Mate"},
			ptr(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Abbot", slotMateID: "Mate"})), false,
		},
		{"join in no party", Command{Kind: CommandJoin, Arg: "Leader"}, nil, false},
		{
			"no slot in a party",
			Command{},
			ptr(slotParty(slotLeaderID, map[int32]string{slotLeaderID: "Leader"})), false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newSlotLink(standing{})
			l.party = tt.slot
			b, out := newSlotBrain(t, l, Persona{})
			if tt.party != nil {
				b.world.Apply(*tt.party, t0)
			}
			watch := leaderWatch{id: slotLeaderID, name: "Leader", lost: t0}
			b.lead = watch
			mustPanel(t, b, Command{Kind: CommandUnparty}, at(time.Second))
			if l.party != (Command{}) {
				t.Fatalf("slot %q after unparty", l.party)
			}
			if tt.party == nil && b.lead != (leaderWatch{}) {
				t.Fatalf("watch %+v after unparty in no party, want reset", b.lead)
			}
			if tt.slot.Kind == 0 && b.lead != watch {
				t.Fatalf("watch %+v after unparty with no slot, want kept", b.lead)
			}
			for _, d := range []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second} {
				wakeAll(t, b, d)
				if n := len(framesOf(out, opGameRequestWithdrawParty)); n == 1 && d == 2*time.Second {
					// The send drops the flag and resets the watch.
					if b.orders.withdraw || b.lead != (leaderWatch{}) {
						t.Fatalf("after the withdrawal flag %v watch %+v", b.orders.withdraw, b.lead)
					}
				}
			}
			got := framesOf(out, opGameRequestWithdrawParty)
			if want := tt.withdraw; (len(got) == 1) != want || len(got) > 1 {
				t.Fatalf("withdrawals % x, want %v once", got, want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

// TestPartyJoinAssignedLeader checks that a join accepts its leader ahead
// of the persona and outside the claim table, declines the leaders earlier
// joins of the run named, and leaves every other invitation, a marked
// player's too, to the persona.
func TestPartyJoinAssignedLeader(t *testing.T) {
	t.Run("claimed leader", func(t *testing.T) {
		claims := NewPartyClaims()
		holder, holderOut := newClaimBrain(t, claimBot1, claims)
		if got := invite(t, holder, holderOut, "Leader", at(time.Second)); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
			t.Fatalf("holder answers % x, want accepted", got)
		}
		b, out := newClaimBrain(t, claimBot0, claims)
		linkBrain(b, standing{})
		if got := invite(t, b, out, "Leader", at(time.Second)); !reflect.DeepEqual(got, [][]byte{declineFrame}) {
			t.Fatalf("answers % x with no slot, want the claim table's decline", got)
		}
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader Other"}, at(20*time.Second))
		if got := invite(t, b, out, "Leader", at(21*time.Second)); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
			t.Fatalf("answers % x, want the assigned leader accepted", got)
		}
		if claims.Held() != 1 {
			t.Fatalf("claims hold %d leaders, want the holder's one", claims.Held())
		}
		if leader, _, _ := claims.held(claimBot0); leader != "" {
			t.Fatalf("the joining bot holds %q", leader)
		}
		if leader, _, _ := claims.held(claimBot1); leader != "Leader" {
			t.Fatalf("the holder holds %q, want Leader", leader)
		}
	})
	t.Run("earlier leaders declined", func(t *testing.T) {
		b, out := newSlotBrain(t, newSlotLink(standing{}), Persona{JoinParty: acceptParty})
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Old"}, at(time.Second))
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader"}, at(2*time.Second))
		steps := []struct {
			requester string
			want      []byte
		}{{"Old", declineFrame}, {"Leader", acceptFrame}}
		for i, s := range steps {
			if got := invite(t, b, out, s.requester, at(time.Duration(10+20*i)*time.Second)); !reflect.DeepEqual(got, [][]byte{s.want}) {
				t.Fatalf("%s: answers % x, want % x", s.requester, got, s.want)
			}
		}
		mustPanel(t, b, Command{Kind: CommandUnparty}, at(time.Minute))
		for i, name := range []string{"Old", "Leader"} {
			if got := invite(t, b, out, name, at(time.Duration(70+20*i)*time.Second)); !reflect.DeepEqual(got, [][]byte{declineFrame}) {
				t.Fatalf("%s with no slot: answers % x, want declined", name, got)
			}
		}
	})
	for _, tt := range []struct {
		name    string
		persona Persona
		want    []byte
	}{
		{"players to an accepting persona", Persona{JoinParty: acceptParty}, acceptFrame},
		{"players to a declining persona", Persona{}, declineFrame},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, out := newSlotBrain(t, newSlotLink(standing{}), tt.persona)
			mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader"}, at(time.Second))
			for i, name := range []string{"Talbot", "Abbot", "leader", "LeaderBot"} {
				if got := invite(t, b, out, name, at(time.Duration(10+20*i)*time.Second)); !reflect.DeepEqual(got, [][]byte{tt.want}) {
					t.Fatalf("%s: answers % x, want % x", name, got, tt.want)
				}
			}
		})
	}
}

// TestPartyWhispered checks that a GM's whispered follow, assist, stay or
// go holds the session against a lead or a join, which then answer
// ErrWhispered and set no slot, until a panel order replaces it.
func TestPartyWhispered(t *testing.T) {
	for name, kind := range map[string]commandKind{"follow": cmdFollow, "assist": cmdAssist, "stay": cmdStay, "go": cmdGo} {
		t.Run(name, func(t *testing.T) {
			b, _ := newCommandBrain(t, Persona{})
			l := linkBrain(b, standing{})
			b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: kind, spot: "Wolves"}}, ok: true}, at(time.Second))
			b.publishStatus(at(time.Second), true)
			if !l.board.load().Whispered {
				t.Fatal("status not whispered")
			}
			for _, c := range []Command{{Kind: CommandJoin, Arg: "Leader"}, {Kind: CommandLead, Arg: "Mate"}} {
				if err := panel(b, c, at(2*time.Second)); !errors.Is(err, ErrWhispered) {
					t.Fatalf("%q = %v, want ErrWhispered", c, err)
				}
			}
			if l.party != (Command{}) {
				t.Fatalf("slot %q set under a whispered order", l.party)
			}
			mustPanel(t, b, Command{Kind: CommandStay}, at(3*time.Second))
			b.publishStatus(at(3*time.Second), true)
			if l.board.load().Whispered {
				t.Fatal("status whispered after the panel's stay")
			}
			mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader"}, at(4*time.Second))
		})
	}
	t.Run("leave keeps no hold", func(t *testing.T) {
		b, _ := newCommandBrain(t, Persona{})
		linkBrain(b, standing{})
		b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdLeave}}, ok: true}, at(time.Second))
		mustPanel(t, b, Command{Kind: CommandJoin, Arg: "Leader"}, at(2*time.Second))
	})
}

// TestPartyWithdrawFlag checks the unparty withdrawal: dropped unsent,
// resetting the leader watch, once the bot is found in no party or answers
// an invitation; kept on a leader change and by the panel's orders.
func TestPartyWithdrawFlag(t *testing.T) {
	member := slotParty(slotLeaderID, map[int32]string{slotLeaderID: "Leader", slotMateID: "Mate"})
	setUp := func(t *testing.T) (*Brain, *recordSender) {
		t.Helper()
		l := newSlotLink(standing{})
		l.party = Command{Kind: CommandJoin, Arg: "Leader Mate"}
		b, out := newSlotBrain(t, l, Persona{JoinParty: acceptParty})
		b.world.Apply(member, t0)
		mustPanel(t, b, Command{Kind: CommandUnparty}, t0)
		if !b.orders.withdraw {
			t.Fatal("unparty set no withdrawal")
		}
		b.lead = leaderWatch{id: slotLeaderID, name: "Leader"}
		return b, out
	}
	dropped := func(t *testing.T, b *Brain, out *recordSender) {
		t.Helper()
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
			t.Fatalf("withdrawals % x, want none", got)
		}
		if b.orders.withdraw || b.lead != (leaderWatch{}) {
			t.Fatalf("flag %v watch %+v, want dropped and reset", b.orders.withdraw, b.lead)
		}
	}
	t.Run("disbanded before the decision", func(t *testing.T) {
		b, out := setUp(t)
		if err := b.handle([]Event{PartySmallWindowDeleteAll{}}, at(10*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		wakeAll(t, b, time.Second, 3*time.Second, 5*time.Second)
		dropped(t, b, out)
	})
	t.Run("disbanded and invited at once", func(t *testing.T) {
		b, out := setUp(t)
		if got := func() [][]byte {
			if err := b.handle([]Event{PartySmallWindowDeleteAll{}}, at(10*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			return invite(t, b, out, "Talbot", at(10*time.Millisecond))
		}(); !reflect.DeepEqual(got, [][]byte{acceptFrame}) {
			t.Fatalf("answers % x, want the persona's accept", got)
		}
		if b.orders.withdraw || b.lead != (leaderWatch{}) {
			t.Fatalf("flag %v watch %+v after the answer, want dropped and reset", b.orders.withdraw, b.lead)
		}
		b.world.Apply(slotParty(slotPlayerID, map[int32]string{slotPlayerID: "Talbot"}), at(2*time.Second))
		wakeAll(t, b, 3*time.Second, 5*time.Second)
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 0 {
			t.Fatalf("withdrawals % x from the new party, want none", got)
		}
	})
	t.Run("kept on a leader change", func(t *testing.T) {
		b, out := setUp(t)
		if err := b.handle([]Event{slotParty(slotMateID, map[int32]string{slotLeaderID: "Leader", slotMateID: "Mate"})}, at(10*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		wakeAll(t, b, time.Second, 3*time.Second)
		if got := framesOf(out, opGameRequestWithdrawParty); len(got) != 1 {
			t.Fatalf("withdrawals % x, want one", got)
		}
	})
	t.Run("kept by the panel's orders", func(t *testing.T) {
		b, out := setUp(t)
		for _, c := range []Command{{Kind: CommandStay}, {Kind: CommandFollow, Arg: "other"}, {Kind: CommandAssist, Arg: "other"}} {
			mustPanel(t, b, c, at(10*time.Millisecond))
			if !b.orders.withdraw {
				t.Fatalf("%q dropped the withdrawal", c)
			}
		}
		b.orders.reply = tellAction("GM", "hi")
		wakeAll(t, b, time.Second, 3*time.Second, 5*time.Second)
		if len(out.frames) == 0 || !bytes.Equal(out.frames[0], withdrawFrame) {
			t.Fatalf("sent % x, want the withdrawal first", out.frames)
		}
	})
}

// TestPartyStatus checks the party fields of the snapshot online and
// offline: the slot, and the whispered hold only while a session runs.
func TestPartyStatus(t *testing.T) {
	l := newSlotLink(standing{})
	b, _ := newSlotBrain(t, l, Persona{})
	b.publishStatus(at(time.Second), true)
	if st := l.board.load(); st.Party != "" || st.Whispered || st.PartyLeft != "" {
		t.Fatalf("no slot: party %q whispered %v left %q", st.Party, st.Whispered, st.PartyLeft)
	}
	mustPanel(t, b, Command{Kind: CommandLead, Arg: "A B"}, at(2*time.Second))
	b.publishStatus(at(2*time.Second), true)
	if st := l.board.load(); st.Party != "lead A B" {
		t.Fatalf("party %q, want lead A B", st.Party)
	}
	mustPanel(t, b, Command{Kind: CommandJoin, Arg: "L A B"}, at(3*time.Second))
	b.publishStatus(at(3*time.Second), true)
	if st := l.board.load(); st.Party != "join L A B" {
		t.Fatalf("party %q, want join L A B", st.Party)
	}
	bot, _, _ := startBot(t, true, &fakeNet{})
	bot.link.party = Command{Kind: CommandJoin, Arg: "L A"}
	bot.link.board.store(Status{Whispered: true})
	bot.publish(StateReconnecting, nil)
	if st := bot.Status(); st.Party != "join L A" || st.Whispered {
		t.Fatalf("offline party %q whispered %v, want the slot and no hold", st.Party, st.Whispered)
	}
}
