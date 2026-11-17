package botclient

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		text string
		want command
		ok   bool
	}{
		{"follow", command{kind: cmdFollow}, true},
		{"  FOLLOW  ", command{kind: cmdFollow}, true},
		{"assist", command{kind: cmdAssist}, true},
		{"Stay", command{kind: cmdStay}, true},
		{"leave", command{kind: cmdLeave}, true},
		{"logout", command{kind: cmdLogout}, true},
		{"invite me", command{kind: cmdInviteMe}, true},
		{"Invite \t ME", command{kind: cmdInviteMe}, true},
		{"go gludio_wolves", command{kind: cmdGo, spot: "gludio_wolves"}, true},
		{"GO  Gludio_Wolves ", command{kind: cmdGo, spot: "Gludio_Wolves"}, true},
		{"", command{}, false},
		{"   ", command{}, false},
		{"hello there", command{}, false},
		{"follow me", command{}, false},
		{"go", command{}, false},
		{"go to town", command{}, false},
		{"invite", command{}, false},
		{"invite you", command{}, false},
		{"invite me now", command{}, false},
		{"logout please", command{}, false},
	}
	for _, tt := range tests {
		got, ok := parseCommand(tt.text)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseCommand(%q) = %+v, %v; want %+v, %v", tt.text, got, ok, tt.want, tt.ok)
		}
	}
}

func TestEncodeSay2Tell(t *testing.T) {
	// "hi" in UTF-16LE with its terminator, the Tell type, then "GM".
	want := []byte{opGameSay2, 'h', 0, 'i', 0, 0, 0, 2, 0, 0, 0, 'G', 0, 'M', 0, 0, 0}
	if got := encodeSay2Tell("GM", "hi"); !bytes.Equal(got, want) {
		t.Fatalf("encodeSay2Tell = % x, want % x", got, want)
	}
	if g := tellAction("GM", "hi").gate(); g != gateChat {
		t.Fatalf("tell gate %d, want gateChat", g)
	}
}

// newCommandBrain is a test brain obeying the commander gmID, which stands
// in sight 2000 units away.
func newCommandBrain(t *testing.T, p Persona) (*Brain, *recordSender) {
	t.Helper()
	b, out := newTestBrain(t, p)
	b.cfg.Authority = NewAuthority(AuthorityConfig{Commanders: []int32{gmID}, Clock: b.clock})
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 2000}, RunSpeed: 100, WalkSpeed: 50, Running: true}, t0)
	return b, out
}

func whisper(from int32, name, text string) CreatureSay {
	return CreatureSay{ObjectID: from, Type: chatTell, Name: name, Text: text}
}

// TestBrainHeard checks which chat lines reach the authority: only
// whispers from others that parse as commands.
func TestBrainHeard(t *testing.T) {
	tests := []struct {
		name    string
		ev      CreatureSay
		obeyed  bool
		checked bool // reached the database check
	}{
		{"commander whisper", whisper(gmID, "GM", "stay"), true, false},
		{"commander on the all channel", CreatureSay{ObjectID: gmID, Type: chatAll, Name: "GM", Text: "stay"}, false, false},
		{"own echo by id", whisper(selfID, "->GM", "stay"), false, false},
		{"echo by name", whisper(gmID, "->GM", "stay"), false, false},
		{"no speaker", whisper(0, "GM", "stay"), false, false},
		{"commander chatting", whisper(gmID, "GM", "how are you"), false, false},
		{"player command", whisper(pcID, "other", "stay"), false, true},
		{"player chatting", whisper(pcID, "other", "hi"), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{})
			db := newFakeLevels(nil)
			b.cfg.Authority = NewAuthority(AuthorityConfig{Lookup: db, Clock: b.clock})
			b.cfg.Authority.commanders[gmID] = true
			if err := b.handle([]Event{tt.ev}, at(time.Second)); err != nil {
				t.Fatal(err)
			}
			if b.orders.stay != tt.obeyed {
				t.Fatalf("stay = %v, want %v", b.orders.stay, tt.obeyed)
			}
			if got := len(b.cfg.Authority.queue) == 1; got != tt.checked {
				t.Fatalf("checked = %v, want %v", got, tt.checked)
			}
		})
	}
}

// TestBrainNoAuthority checks that a brain with no authority obeys nobody.
func TestBrainNoAuthority(t *testing.T) {
	b, _ := newTestBrain(t, Persona{})
	if err := b.handle([]Event{whisper(gmID, "GM", "stay")}, t0); err != nil {
		t.Fatal(err)
	}
	if b.orders.stay || !b.reactAt.IsZero() {
		t.Fatal("obeyed with no authority")
	}
}

// TestBrainCommandReplies checks the one-off packets commands send through
// rule "command", once each.
func TestBrainCommandReplies(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		party bool // the bot is in the commander's party
		want  []byte
	}{
		{"invite me", "invite me", false, encodeSay2Tell("GM", inviteMeText)},
		{"invite me in its party", "invite me", true, nil},
		{"leave", "leave", true, encodeRequestWithdrawParty()},
		{"leave outside a party", "leave", false, nil},
		{"go without spots", "go wolves", false, encodeSay2Tell("GM", replyNoSpots)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, out := newCommandBrain(t, Persona{})
			if tt.party {
				b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
			}
			if err := b.handle([]Event{whisper(gmID, "GM", tt.text)}, t0); err != nil {
				t.Fatal(err)
			}
			if d := b.decisionAt().Sub(t0); d < defaultReactMin || d > defaultReactMax {
				t.Fatalf("reaction %v after the command, want [%v, %v]", d, defaultReactMin, defaultReactMax)
			}
			for _, w := range []time.Time{b.decisionAt(), at(4 * time.Second), at(8 * time.Second)} {
				if err := b.wake(w); err != nil {
					t.Fatal(err)
				}
			}
			var got [][]byte
			for _, f := range out.frames {
				if f[0] == opGameSay2 || f[0] == opGameRequestWithdrawParty {
					got = append(got, f)
				}
			}
			switch {
			case tt.want == nil && len(got) != 0:
				t.Fatalf("sent % x, want nothing", got)
			case tt.want != nil && (len(got) != 1 || !bytes.Equal(got[0], tt.want)):
				t.Fatalf("sent % x, want % x once", got, tt.want)
			}
		})
	}
}

func TestBrainCommandLogout(t *testing.T) {
	b, out := newCommandBrain(t, Persona{})
	if err := b.handle([]Event{whisper(gmID, "GM", "logout")}, t0); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(out.frames) == 0 || out.frames[0][0] != opGameLogout {
		t.Fatalf("sent % x, want Logout first", out.frames)
	}
	if err := b.handle([]Event{LeaveWorld{}}, at(2*time.Second)); !errors.Is(err, ErrCommandedLogout) {
		t.Fatalf("handle(LeaveWorld) = %v, want ErrCommandedLogout", err)
	}
}

// TestRuleCommandChatGate checks that a whisper reply held back by the chat
// gate lets the survival rules run, and goes out once the gate opens.
func TestRuleCommandChatGate(t *testing.T) {
	b, _ := newCommandBrain(t, Persona{CriticalHP: 20, FleeDistance: 600})
	b.start(t0)
	b.pace.sent(gateChat, t0)
	s := &b.world.Self
	s.InCombat, s.HP, s.MaxHP = true, 10, 100
	b.orders.reply = tellAction("GM", inviteMeText)
	if name, _ := b.decide(at(chatGap - time.Millisecond)); name != "critical" {
		t.Fatalf("inside the chat gate: rule %q, want critical", name)
	}
	s.InCombat, s.HP = false, 100
	if name, _ := b.decide(at(chatGap)); name != "command" {
		t.Fatalf("after the chat gate: rule %q, want command", name)
	}
}

// TestSessionCommandedLogout checks that a session a GM's "logout" ended
// stops the bot, persistent or not, and never counts as an abort.
func TestSessionCommandedLogout(t *testing.T) {
	for _, persistent := range []bool{true, false} {
		reg := botmetrics.NewRegistry()
		m := reg.NewBot()
		b := &Bot{cfg: SessionConfig{Persistent: persistent}, brain: BrainConfig{Metrics: m}, rng: rand.New(rand.NewPCG(1, 2)), log: discardLog(), retry: backoff{base: retryBase, max: retryMax}}
		for range maxAborts {
			d, err := b.after(true, time.Minute, ErrCommandedLogout)
			if d != 0 || !errors.Is(err, ErrCommandedLogout) {
				t.Fatalf("persistent %v: after = %v, %v; want 0, ErrCommandedLogout", persistent, d, err)
			}
		}
		if b.aborts != 0 || reg.Collect(t0).Aborts != 0 {
			t.Fatalf("persistent %v: %d aborts counted", persistent, b.aborts)
		}
	}
}

// TestBrainCommandStayFollow checks that "stay" holds the bot, even in its
// leader's party, and stops its move, and that "follow" takes it to the
// commander outside a party.
func TestBrainCommandStayFollow(t *testing.T) {
	b, _ := newCommandBrain(t, Persona{Follow: defaultFollowPolicy(), IdleChance: 1, IdleRadius: 300})
	if name, _ := b.decide(at(time.Second)); name == "follow" {
		t.Fatal("follows a commander outside a party unasked")
	}
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdFollow}}, ok: true}, at(time.Second))
	a, due := repath(t, b, at(2*time.Second))
	if a.kind != actMove {
		t.Fatalf("follow sent kind %d, want a move", a.kind)
	}
	if _, err := b.send(a, due); err != nil {
		t.Fatal(err)
	}
	b.world.Apply(MoveToLocation{ObjectID: selfID, Dest: moveTarget(a.payload), Origin: Point{}}, at(3*time.Second))
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdStay}}, ok: true}, at(3*time.Second))
	name, a := b.decide(at(4 * time.Second))
	here := b.world.Self.Position(at(4 * time.Second))
	if name != "stay" || a.kind != actMove || moveTarget(a.payload) != here {
		t.Fatalf("decide = %q kind %d to %v, want stay stopping at %v", name, a.kind, moveTarget(a.payload), here)
	}
	b.world.Apply(StopMove{ObjectID: selfID, Pos: here}, at(4*time.Second))
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, at(4*time.Second))
	for i := range 20 {
		if name, a := b.decide(at(5*time.Second + time.Duration(i)*time.Second)); name != "stay" || a.kind != actHold {
			t.Fatalf("decision %d = %q kind %d, want stay holding", i, name, a.kind)
		}
	}
}

// TestBrainCommandGo checks that "go" claims the named spot within its cap,
// stops following the party leader, and answers what it cannot do.
func TestBrainCommandGo(t *testing.T) {
	spots := []datapack.Spot{testSpot("Wolves", 5000, 0, 1, 3, 3), testSpot("Bears", 0, 5000, 1, 3, 3)}
	b, _ := newCommandBrain(t, DefaultPersona())
	b.persona.IdleChance = 0
	b.world.Self.Level = 2
	b.cfg.Spots = NewSpotRegistry(spots, 1)
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
	if name, _ := b.decide(at(time.Second)); name != "follow" {
		t.Fatalf("decide = %q, want follow before the command", name)
	}
	if i, ok := b.cfg.Spots.claim([]int{1}); !ok || i != 1 {
		t.Fatal("cannot fill the bears")
	}
	b.cfg.Spots.claim([]int{1})
	b.cfg.Spots.claim([]int{1})
	tests := []struct {
		spot      string
		wantReply string
		wantIdx   int
	}{
		{"nowhere", replyUnknownSpot, -1},
		{"bears", replySpotFull, -1},
		{"wOLVES", "", 0},
	}
	for _, tt := range tests {
		b.orders.reply = Action{}
		b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdGo, spot: tt.spot}}, ok: true}, at(2*time.Second))
		var want Action
		if tt.wantReply != "" {
			want = tellAction("GM", tt.wantReply)
		}
		if !bytes.Equal(b.orders.reply.payload, want.payload) {
			t.Fatalf("go %s: reply % x, want % x", tt.spot, b.orders.reply.payload, want.payload)
		}
		if tt.wantIdx >= 0 && (!b.spot.held || b.spot.idx != tt.wantIdx) {
			t.Fatalf("go %s: spot %+v, want %d held", tt.spot, b.spot, tt.wantIdx)
		}
	}
	if b.cfg.Spots.Held(0) != 1 {
		t.Fatalf("wolves held by %d, want 1", b.cfg.Spots.Held(0))
	}
	b.orders.reply = Action{}
	name, a := b.decide(at(3 * time.Second))
	if to := moveTarget(a.payload); name != "spot" || a.kind != actMove || to.X <= 0 || max(to.Y, -to.Y) >= to.X {
		t.Fatalf("decide = %q kind %d, want spot walking to the wolves", name, a.kind)
	}
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdFollow}}, ok: true}, at(4*time.Second))
	if name, _ := b.decide(at(5 * time.Second)); name != "follow" {
		t.Fatalf("decide = %q, want follow after the follow command", name)
	}
}

// TestBrainCommandAssist checks that "assist" makes the bot assist the
// commander outside a party.
func TestBrainCommandAssist(t *testing.T) {
	p := Persona{AssistMin: 100 * time.Millisecond, AssistMax: 100 * time.Millisecond}
	b, _ := newCommandBrain(t, p)
	hit := Attack{AttackerID: gmID, Hits: []AttackHit{{TargetID: npcID}}}
	if err := b.handle([]Event{hit}, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.assist.target != 0 {
		t.Fatal("assists a commander unasked")
	}
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdAssist}}, ok: true}, at(time.Second))
	if err := b.handle([]Event{hit}, at(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	name, a := b.decide(at(3 * time.Second))
	if name != "assist" || a.object != npcID {
		t.Fatalf("decide = %q on %d, want assist on the npc", name, a.object)
	}
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdStay}}, ok: true}, at(3*time.Second))
	if name, _ := b.decide(at(4 * time.Second)); name != "stay" {
		t.Fatalf("decide = %q, want stay after the stay command", name)
	}
}

// TestBrainCommandDatabase checks a command checked in the database: the
// verdict arrives through the brain's queue, and a refused one does nothing.
func TestBrainCommandDatabase(t *testing.T) {
	for _, level := range []int32{0, 7} {
		b, _ := newTestBrain(t, Persona{})
		a := NewAuthority(AuthorityConfig{Lookup: newFakeLevels(map[int32]levelRow{gmID: {level: level}}), Clock: b.clock})
		b.cfg.Authority = a
		if err := b.handle([]Event{whisper(gmID, "GM", "stay")}, t0); err != nil {
			t.Fatal(err)
		}
		if b.orders.stay {
			t.Fatal("obeyed before the check")
		}
		v := answer(t, a, b.verdicts)
		b.obey(v, at(time.Second))
		if b.orders.stay != (level == 7) {
			t.Fatalf("level %d: stay = %v", level, b.orders.stay)
		}
	}
}

// TestBrainRunVerdict checks that Run takes a verdict off its queue and
// obeys it.
func TestBrainRunVerdict(t *testing.T) {
	b, done, _, errc := runTestBrain(t, Persona{})
	b.verdicts <- verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdLogout}}, ok: true}
	got := <-b.out.(chanSender)
	if got[0] != opGameLogout {
		t.Fatalf("sent % x, want Logout", got)
	}
	b.events.push(LeaveWorld{}, done)
	if err := <-errc; !errors.Is(err, ErrCommandedLogout) {
		t.Fatalf("Run = %v, want ErrCommandedLogout", err)
	}
}
