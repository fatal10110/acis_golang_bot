package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// chatShout is the Say2 channel of shout, which a reply never answers
// (say2.go), as it never answers trade (chatTrade).
const chatShout int32 = 1

// chatDelay is the fixed reply delay of chatPersona.
const chatDelay = 5 * time.Second

// chatPersona replies "hey" after chatDelay, once per sender per ten
// minutes and twice per hour in all.
func chatPersona() Persona {
	return Persona{Chat: ChatPolicy{
		Replies:   []string{"hey"},
		DelayMin:  chatDelay,
		DelayMax:  chatDelay,
		PerSender: 10 * time.Minute,
		Total:     2,
		Window:    time.Hour,
	}}
}

// TestBrainChatHeard checks which lines get a reply: a whisper from another
// player that is no command, from a name without the bot marker.
func TestBrainChatHeard(t *testing.T) {
	prefixed := botnames.Marker{Text: "X1", Placement: botnames.Prefix}
	tests := []struct {
		name    string
		ev      CreatureSay
		markers botnames.Markers
		noLines bool
		want    bool
	}{
		{"player whisper", whisper(pcID, "Alden", "hi there"), nil, false, true},
		{"marked sender", whisper(pcID, "AldenBot", "hi"), nil, false, false},
		{"marked sender in any case", whisper(pcID, "aldenBOT", "hi"), nil, false, false},
		{"configured prefix marker", whisper(pcID, "X1Alden", "hi"), botnames.Markers{prefixed}, false, false},
		{"default suffix under a prefix marker", whisper(pcID, "AldenBot", "hi"), botnames.Markers{prefixed}, false, true},
		{"first of two markers", whisper(pcID, "AldenBot", "hi"), botnames.Markers{botnames.DefaultMarker, prefixed}, false, false},
		{"second of two markers", whisper(pcID, "X1Alden", "hi"), botnames.Markers{botnames.DefaultMarker, prefixed}, false, false},
		{"all channel", CreatureSay{ObjectID: pcID, Type: chatAll, Name: "Alden", Text: "hi"}, nil, false, false},
		{"shout", CreatureSay{ObjectID: pcID, Type: chatShout, Name: "Alden", Text: "hi"}, nil, false, false},
		{"trade", CreatureSay{ObjectID: pcID, Type: chatTrade, Name: "Alden", Text: "hi"}, nil, false, false},
		{"echo of the bot's whisper", whisper(pcID, "->Alden", "hi"), nil, false, false},
		{"own line", whisper(selfID, "Self", "hi"), nil, false, false},
		{"no speaker", whisper(0, "Alden", "hi"), nil, false, false},
		{"command", whisper(pcID, "Alden", "follow"), nil, false, false},
		{"no reply lines", whisper(pcID, "Alden", "hi"), nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := chatPersona()
			if tt.noLines {
				p.Chat.Replies = nil
			}
			b, _ := newTestBrain(t, p)
			if tt.markers != nil {
				b.cfg.markers = tt.markers
			}
			if err := b.handle([]Event{tt.ev}, t0); err != nil {
				t.Fatal(err)
			}
			if got := b.chat.reply.kind == actChatReply; got != tt.want {
				t.Fatalf("reply scheduled = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBrainChatReply checks that a whisper gets one reply after the delay,
// woken for, to the sender's name, and that one waits at a time.
func TestBrainChatReply(t *testing.T) {
	b, out := newTestBrain(t, chatPersona())
	b.nextTick = at(time.Minute)
	if err := b.handle([]Event{whisper(pcID, "Alden", "hi"), whisper(pcID+1, "Other", "hi")}, t0); err != nil {
		t.Fatal(err)
	}
	if b.chat.to != pcID {
		t.Fatalf("reply to %d, want the first sender %d", b.chat.to, pcID)
	}
	if got := b.decisionAt(); !got.Equal(at(chatDelay)) {
		t.Fatalf("decision at %v, want the reply's %v", got.Sub(t0), chatDelay)
	}
	if name, _ := b.decide(at(chatDelay - time.Millisecond)); name == "chat" {
		t.Fatal("replied before the delay")
	}
	if err := b.wake(at(chatDelay)); err != nil {
		t.Fatal(err)
	}
	want := encodeSay2Tell("Alden", "hey")
	if b.lastRule != "chat" || len(out.frames) != 1 || !bytes.Equal(out.frames[0], want) {
		t.Fatalf("rule %q sent %x, want chat sending %x", b.lastRule, out.frames, want)
	}
	if b.chat.reply.kind != actHold || b.chat.wake {
		t.Fatalf("reply still waiting: %+v", b.chat)
	}
	if name, _ := b.decide(at(time.Minute)); name == "chat" {
		t.Fatal("replied twice")
	}
}

// TestBrainChatGate checks that a reply waits for the chat gate a whisper
// just passed, and that store mode replies too.
func TestBrainChatGate(t *testing.T) {
	b, _ := newTestBrain(t, chatPersona())
	b.world.Self.StoreType = 1
	if err := b.handle([]Event{whisper(pcID, "Alden", "hi")}, t0); err != nil {
		t.Fatal(err)
	}
	b.pace.sent(gateChat, at(chatDelay-time.Second))
	if name, _ := b.decide(at(chatDelay)); name == "chat" {
		t.Fatal("replied inside the chat gate")
	}
	if name, _ := b.decide(at(chatDelay - time.Second + chatGap)); name != "chat" {
		t.Fatalf("after the chat gate: rule %q, want chat", name)
	}
}

// TestBrainChatAfterSurvival checks that a due reply never delays a
// critical-HP flee: "critical" wins and the reply stays pending until the
// survival rules pass.
func TestBrainChatAfterSurvival(t *testing.T) {
	p := chatPersona()
	p.CriticalHP, p.FleeDistance = 20, 600
	b, _ := newTestBrain(t, p)
	if err := b.handle([]Event{whisper(pcID, "Alden", "hi")}, t0); err != nil {
		t.Fatal(err)
	}
	s := &b.world.Self
	s.InCombat, s.HP, s.MaxHP = true, 10, 100
	if name, _ := b.decide(at(chatDelay)); name != "critical" {
		t.Fatalf("critical HP with a reply due: rule %q, want critical", name)
	}
	if b.chat.reply.kind != actChatReply {
		t.Fatal("reply dropped while fleeing")
	}
	s.InCombat, s.HP = false, 100
	if name, _ := b.decide(at(chatDelay)); name != "chat" {
		t.Fatalf("after the flee: rule %q, want chat", name)
	}
}

// TestBrainChatApartFromCommands checks that a chat reply and a GM
// command's reply, both whispers, never clear each other once sent.
func TestBrainChatApartFromCommands(t *testing.T) {
	b, _ := newTestBrain(t, chatPersona())
	if err := b.handle([]Event{whisper(pcID, "Alden", "hi")}, t0); err != nil {
		t.Fatal(err)
	}
	command := tellAction("GM", inviteMeText)
	b.orders.reply = command
	if _, err := b.send(b.chat.reply, at(chatDelay)); err != nil {
		t.Fatal(err)
	}
	if b.orders.reply.kind != actTell {
		t.Fatal("the chat reply cleared the command reply")
	}
	if err := b.handle([]Event{whisper(pcID+1, "Other", "hi")}, at(2*chatGap)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.send(command, at(3*chatGap)); err != nil {
		t.Fatal(err)
	}
	if b.chat.reply.kind != actChatReply {
		t.Fatal("the command reply cleared the chat reply")
	}
}

// TestChatLogCaps checks the per-sender and per-bot caps of a run of
// replies, the window sliding and the senders swept.
func TestChatLogCaps(t *testing.T) {
	p := chatPersona().Chat
	l := newChatLog()
	steps := []struct {
		name   string
		sender int32
		at     time.Duration
		send   bool // record a reply; else check allows
		want   bool
	}{
		{"first", 1, 0, false, true},
		{"reply to 1", 1, 0, true, false},
		{"1 again inside PerSender", 1, 5 * time.Minute, false, false},
		{"another sender", 2, 5 * time.Minute, false, true},
		{"reply to 2", 2, 5 * time.Minute, true, false},
		{"third sender past Total", 3, 6 * time.Minute, false, false},
		{"1 past PerSender but not Total", 1, 30 * time.Minute, false, false},
		{"first reply out of the window", 3, time.Hour, false, true},
		{"reply to 3", 3, time.Hour, true, false},
		{"second reply out of the window", 1, time.Hour + 5*time.Minute, false, true},
	}
	for _, s := range steps {
		now := at(s.at)
		if s.send {
			l.record(s.sender, now, &p)
			continue
		}
		if got := l.allows(s.sender, now, &p); got != s.want {
			t.Fatalf("%s: allows = %v, want %v", s.name, got, s.want)
		}
	}
	if _, ok := l.to[1]; ok || len(l.to) != 1 {
		t.Fatalf("senders kept past PerSender: %v", l.to)
	}
}

// TestChatLogLifetime checks that Window 0 caps the replies for good, and
// that the cap holds across the sessions of one bot.
func TestChatLogLifetime(t *testing.T) {
	p := chatPersona()
	p.Chat.Total, p.Chat.Window = 1, 0
	log := newChatLog()
	first, _ := newTestBrain(t, p)
	first.cfg.chat = log
	if err := first.handle([]Event{whisper(pcID, "Alden", "hi")}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := first.send(first.chat.reply, at(chatDelay)); err != nil {
		t.Fatal(err)
	}
	next, _ := newTestBrain(t, p)
	next.cfg.chat = log
	if err := next.handle([]Event{whisper(pcID+1, "Other", "hi")}, at(1000*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if next.chat.reply.kind != actHold {
		t.Fatal("replied past the bot's lifetime cap in its next session")
	}
}
