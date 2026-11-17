package botclient

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// linkBrain gives b a Bot's link, as a session does, holding order.
func linkBrain(b *Brain, order standing) *botLink {
	l := &botLink{mail: make(chan mail, mailboxSize), order: order}
	l.board.store(Status{})
	b.cfg.link, b.mail = l, l.mail
	return l
}

// noTell fails when out holds a whisper.
func noTell(t *testing.T, out *recordSender) {
	t.Helper()
	for _, f := range out.frames {
		if f[0] == opGameSay2 {
			t.Fatalf("sent Say2 % x", f)
		}
	}
}

// TestCommandValid checks the commands Bot.Command takes.
func TestCommandValid(t *testing.T) {
	tests := []struct {
		cmd  Command
		want bool
	}{
		{Command{Kind: CommandStay}, true},
		{Command{Kind: CommandGo, Arg: "Wolves"}, true},
		{Command{Kind: CommandFollow, Arg: "Name"}, true},
		{Command{Kind: CommandAssist, Arg: "Name"}, true},
		{Command{Kind: CommandLeave}, true},
		{Command{Kind: CommandLogout}, true},
		{Command{}, false},
		{Command{Kind: CommandLogout + 1}, false},
		{Command{Kind: CommandStay, Arg: "x"}, false},
		{Command{Kind: CommandGo}, false},
		{Command{Kind: CommandFollow, Arg: " Name"}, false},
	}
	for _, tt := range tests {
		if got := tt.cmd.valid(); got != tt.want {
			t.Errorf("%q valid = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

// TestPanelFollowAssist checks that a follow or an assist of the panel
// resolves a player in sight by name, in any case, and is refused
// otherwise; one that came between sessions waits for the player.
func TestPanelFollowAssist(t *testing.T) {
	for _, kind := range []CommandKind{CommandFollow, CommandAssist} {
		ordered := func(b *Brain) int32 {
			if kind == CommandFollow {
				return b.orders.follow
			}
			return b.orders.assist
		}
		t.Run(kind.String(), func(t *testing.T) {
			b, out := newCommandBrain(t, DefaultPersona())
			l := linkBrain(b, standing{})
			b.holdStill()
			if err := b.panelOrder(Command{Kind: kind, Arg: "Stranger"}, false, at(time.Second)); !errors.Is(err, ErrNotInSight) {
				t.Fatalf("%s out of sight = %v, want ErrNotInSight", kind, err)
			}
			if !b.orders.stay || l.order.cmd.Kind != 0 {
				t.Fatalf("refused %s changed the orders: %+v, standing %+v", kind, b.orders, l.order)
			}
			if err := b.panelOrder(Command{Kind: kind, Arg: "gm"}, false, at(time.Second)); err != nil {
				t.Fatalf("%s in sight = %v", kind, err)
			}
			want := standing{cmd: Command{Kind: kind, Arg: "GM"}, id: gmID}
			if ordered(b) != gmID || b.orders.stay || l.order != want {
				t.Fatalf("orders %+v, standing %+v; want %s of the GM", b.orders, l.order, kind)
			}

			if err := b.panelOrder(Command{Kind: kind, Arg: "stranger"}, true, at(2*time.Second)); err != nil {
				t.Fatalf("queued %s = %v", kind, err)
			}
			if ordered(b) != 0 || !b.orderPending {
				t.Fatalf("queued %s: orders %+v, pending %v; want none pending", kind, b.orders, b.orderPending)
			}
			b.resolveOrder()
			if ordered(b) != 0 {
				t.Fatalf("%s resolved a player not in sight", kind)
			}
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "Stranger", Pos: Point{X: 500}}, at(3*time.Second))
			b.resolveOrder()
			want = standing{cmd: Command{Kind: kind, Arg: "Stranger"}, id: pcID}
			if ordered(b) != pcID || b.orderPending || l.order != want {
				t.Fatalf("in sight: orders %+v, standing %+v; want %s of the stranger", b.orders, l.order, kind)
			}
			noTell(t, out)
		})
	}
}

// TestPanelGo checks that a go of the panel claims its spot and becomes the
// standing order, and that one the bot cannot carry out answers the panel,
// changes nothing and sends no whisper.
func TestPanelGo(t *testing.T) {
	spots := []datapack.Spot{testSpot("Wolves", 5000, 0, 1, 3, 3), testSpot("Bears", 0, 5000, 1, 3, 1)}
	b, out := newCommandBrain(t, DefaultPersona())
	l := linkBrain(b, standing{})
	b.holdStill()
	if err := b.panelOrder(Command{Kind: CommandGo, Arg: "wolves"}, false, at(time.Second)); !errors.Is(err, ErrNoSpots) {
		t.Fatalf("go without spots = %v, want ErrNoSpots", err)
	}
	b.cfg.Spots = NewSpotRegistry(spots, 1)
	b.cfg.Spots.claim([]int{1})
	tests := []struct {
		spot string
		want error
	}{
		{"nowhere", ErrUnknownSpot},
		{"bears", ErrSpotFull},
	}
	for _, tt := range tests {
		if err := b.panelOrder(Command{Kind: CommandGo, Arg: tt.spot}, false, at(time.Second)); !errors.Is(err, tt.want) {
			t.Fatalf("go %s = %v, want %v", tt.spot, err, tt.want)
		}
		if !b.orders.stay || b.orders.reply.kind != actHold || l.order.cmd.Kind != 0 {
			t.Fatalf("go %s: orders %+v, standing %+v; want unchanged", tt.spot, b.orders, l.order)
		}
	}
	if err := b.panelOrder(Command{Kind: CommandGo, Arg: "wOLVES"}, false, at(time.Second)); err != nil {
		t.Fatalf("go wolves = %v", err)
	}
	if !b.spot.held || b.spot.idx != 0 || !b.orders.roam || b.orders.stay || l.order.cmd != (Command{Kind: CommandGo, Arg: "wOLVES"}) {
		t.Fatalf("go wolves: spot %+v, orders %+v, standing %+v", b.spot, b.orders, l.order)
	}
	for i := range 5 {
		if err := b.wake(at(2*time.Second + time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	noTell(t, out)
}

// TestPanelLeaveLogout checks the one-off commands: a leave withdraws from
// a party through rule "command" and is refused outside one, a logout logs
// out on command; neither touches the standing order.
func TestPanelLeaveLogout(t *testing.T) {
	b, _ := newCommandBrain(t, DefaultPersona())
	stay := standing{cmd: Command{Kind: CommandStay}}
	l := linkBrain(b, stay)
	if err := b.panelOrder(Command{Kind: CommandLeave}, false, at(time.Second)); !errors.Is(err, ErrNoParty) {
		t.Fatalf("leave outside a party = %v, want ErrNoParty", err)
	}
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, at(time.Second))
	if err := b.panelOrder(Command{Kind: CommandLeave}, false, at(time.Second)); err != nil {
		t.Fatalf("leave = %v", err)
	}
	if name, a := b.decide(at(2 * time.Second)); name != "command" || a.payload[0] != leavePartyAction().payload[0] {
		t.Fatalf("decide = %q % x, want command withdrawing", name, a.payload)
	}
	if err := b.panelOrder(Command{Kind: CommandLogout}, false, at(3*time.Second)); err != nil {
		t.Fatalf("logout = %v", err)
	}
	if !b.logout.commanded || !b.logout.requested || l.order != stay {
		t.Fatalf("logout: %+v, standing %+v", b.logout, l.order)
	}
}

// TestPanelStanding checks that a new session applies the Bot's standing
// order at its start: a stay holds the bot, a go claims the spot, a follow
// waits for its player, by object id or name, before the bot follows it.
func TestPanelStanding(t *testing.T) {
	newBrain := func(t *testing.T, order standing) (*Brain, *botLink) {
		w := newTestWorld(t)
		w.Self.HP, w.Self.MaxHP, w.Self.MP, w.Self.MaxMP = 100, 100, 100, 100
		l := &botLink{mail: make(chan mail, mailboxSize), order: order}
		l.board.store(Status{})
		cfg := BrainConfig{Persona: DefaultPersona(), Rand: rand.New(rand.NewPCG(1, 2)), link: l}
		cfg.Spots = NewSpotRegistry([]datapack.Spot{testSpot("Wolves", 5000, 0, 1, 3, 3)}, 1)
		b := NewBrain(w, NewEvents(64), &recordSender{}, newFakeClock(t0), cfg)
		b.persona.IdleChance = 0
		b.start(t0)
		return b, l
	}
	t.Run("stay", func(t *testing.T) {
		b, _ := newBrain(t, standing{cmd: Command{Kind: CommandStay}})
		if name, _ := b.decide(at(time.Second)); name != "stay" {
			t.Fatalf("decide = %q, want stay", name)
		}
	})
	t.Run("go", func(t *testing.T) {
		b, _ := newBrain(t, standing{cmd: Command{Kind: CommandGo, Arg: "Wolves"}})
		if !b.spot.held || b.spot.idx != 0 || !b.orders.roam {
			t.Fatalf("spot %+v, orders %+v; want the wolves held", b.spot, b.orders)
		}
	})
	for _, tt := range []struct {
		name  string
		order standing
	}{
		{"follow by id", standing{cmd: Command{Kind: CommandFollow, Arg: "Old"}, id: pcID}},
		{"follow by name", standing{cmd: Command{Kind: CommandFollow, Arg: "friend"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, l := newBrain(t, tt.order)
			if b.followed() != 0 || !b.orderPending {
				t.Fatalf("follows %d before the player came in sight", b.followed())
			}
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "Friend", Pos: Point{X: 3000}}, at(time.Second))
			if err := b.wake(at(time.Minute)); err != nil {
				t.Fatal(err)
			}
			want := standing{cmd: Command{Kind: CommandFollow, Arg: "Friend"}, id: pcID}
			if b.followed() != pcID || l.order != want || b.Status().Order != want.cmd {
				t.Fatalf("follows %d, standing %+v, status order %q; want the friend", b.followed(), l.order, b.Status().Order)
			}
		})
	}
	t.Run("whisper overrides", func(t *testing.T) {
		b, l := newBrain(t, standing{cmd: Command{Kind: CommandFollow, Arg: "Friend"}})
		b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdStay}}, ok: true}, at(time.Second))
		b.world.Apply(CharInfo{ObjectID: pcID, Name: "Friend"}, at(time.Second))
		b.resolveOrder()
		if b.followed() != 0 || !b.orders.stay || l.order.cmd.Kind != CommandFollow {
			t.Fatalf("follows %d, orders %+v, standing %+v; want the whispered stay for the session", b.followed(), b.orders, l.order)
		}
	})
}

// Status reads the snapshot the brain's link holds, for tests.
func (b *Brain) Status() Status { return b.cfg.link.board.load() }

// TestBrainRunMailbox checks that Run takes a panel command off the mailbox
// and answers it, and logs out on a panel logout.
func TestBrainRunMailbox(t *testing.T) {
	out := make(chanSender, 16)
	b := NewBrain(newTestWorld(t), NewEvents(64), out, newFakeClock(t0), BrainConfig{Rand: rand.New(rand.NewPCG(5, 6))})
	l := linkBrain(b, standing{})
	done, errc := make(chan struct{}), make(chan error, 1)
	go func() { errc <- b.Run(done, nil) }()
	stay := mail{cmd: Command{Kind: CommandStay}, answer: make(chan error, 1)}
	l.mail <- stay
	if err := <-stay.answer; err != nil {
		t.Fatalf("stay answered %v", err)
	}
	logout := mail{cmd: Command{Kind: CommandLogout}, answer: make(chan error, 1)}
	l.mail <- logout
	if err := <-logout.answer; err != nil {
		t.Fatalf("logout answered %v", err)
	}
	if got := <-out; got[0] != opGameLogout {
		t.Fatalf("sent % x, want Logout", got)
	}
	b.events.push(LeaveWorld{}, done)
	if err := <-errc; !errors.Is(err, ErrCommandedLogout) {
		t.Fatalf("Run = %v, want ErrCommandedLogout", err)
	}
	// The order shows in the next snapshot, at most statusEvery after the
	// one published at the start.
	if st := b.Status(); l.order.cmd != (Command{Kind: CommandStay}) || st.State != StateOnline {
		t.Fatalf("standing %+v, status %+v; want online staying", l.order, st)
	}
}

// TestBotCommandBusy checks that a full mailbox answers busy at once and a
// bad command is refused.
func TestBotCommandBusy(t *testing.T) {
	b, _, _ := startBot(t, true, &fakeNet{})
	if _, err := b.Command(Command{Kind: CommandGo}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("go without a spot = %v, want ErrBadCommand", err)
	}
	for i := range mailboxSize {
		if _, err := b.Command(Command{Kind: CommandStay}); err != nil {
			t.Fatalf("command %d = %v", i, err)
		}
	}
	if _, err := b.Command(Command{Kind: CommandStay}); !errors.Is(err, ErrBusy) {
		t.Fatalf("command past the mailbox = %v, want ErrBusy", err)
	}
}

// TestBotCommandStopped checks that a command waiting in the mailbox when
// the bot stops is refused, as is any later one.
func TestBotCommandStopped(t *testing.T) {
	refused := &LoginError{Packet: "LoginFail", Reason: loginFailPasswordWrong}
	n := &fakeNet{logins: []error{refused}}
	b, clock, ctx := startBot(t, true, n)
	ans, err := b.Command(Command{Kind: CommandStay})
	if err != nil {
		t.Fatal(err)
	}
	if err := runBot(ctx, b, clock); !errors.Is(err, refused) {
		t.Fatalf("Run = %v, want the refusal", err)
	}
	if err := <-ans; !errors.Is(err, ErrStopped) {
		t.Fatalf("waiting command answered %v, want ErrStopped", err)
	}
	if _, err := b.Command(Command{Kind: CommandStay}); !errors.Is(err, ErrStopped) {
		t.Fatalf("command after the stop = %v, want ErrStopped", err)
	}
	if st := b.Status(); st.State != StateStopped || !errors.Is(st.Err, refused) {
		t.Fatalf("status %v (%v), want stopped by the refusal", st.State, st.Err)
	}
}

// hookNet hands the test each game connection it dials.
type hookNet struct {
	*fakeNet
	games chan *fakeGame
}

func (h hookNet) dialGame(ctx context.Context, addr string) (gameSession, error) {
	g, err := h.fakeNet.dialGame(ctx, addr)
	h.games <- g.(*fakeGame)
	return g, err
}

// TestBotStandingReconnect checks that a standing order sent before the
// first session is carried out by it, outlives a reconnect and shows in the
// snapshot.
func TestBotStandingReconnect(t *testing.T) {
	n := &fakeNet{cancelAt: 2}
	b, clock, ctx := startBot(t, true, n)
	h := hookNet{fakeNet: n, games: make(chan *fakeGame, 4)}
	b.net = h
	ans, err := b.Command(Command{Kind: CommandStay})
	if err != nil {
		t.Fatal(err)
	}
	var orders []Command
	n.onEnter = func(int, Character, func(string, int32)) error {
		orders = append(orders, b.link.order.cmd) // on Run's goroutine
		return nil
	}
	errc := make(chan error, 1)
	go func() { errc <- runBot(ctx, b, clock) }()
	g := <-h.games
	if err := <-ans; err != nil {
		t.Fatalf("stay answered %v", err)
	}
	close(g.events) // the line drops: the bot reconnects
	if err := <-errc; err != nil {
		t.Fatalf("Run = %v", err)
	}
	stay := Command{Kind: CommandStay}
	if len(orders) != 2 || orders[0] != (Command{}) || orders[1] != stay {
		t.Fatalf("standing orders at the entries %v, want none then stay", orders)
	}
	if st := b.Status(); st.Order != stay || st.Reconnects != 1 || st.State != StateStopped {
		t.Fatalf("status %+v, want stopped after a reconnect, staying", st)
	}
	checkJoined(t, n)
}
