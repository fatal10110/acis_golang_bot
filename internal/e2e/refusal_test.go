package e2e

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// packetTo is a client frame of op whose first field is target.
func packetTo(op byte, target int32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{op}, uint32(target))
}

// sysMsg is the server's SystemMessage frame (opcode 0x64) of id, without
// parameters.
func sysMsg(id int32) []byte {
	return binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32([]byte{0x64}, uint32(id)), 0)
}

var actionFailed = []byte{0x25}

const (
	storeID       int32 = 7001
	notEnoughCash int32 = 279
)

// step is one thing that happens on a watched bot: a frame written or
// read, after the clock moved on by wait.
type step struct {
	wait time.Duration
	sent []byte
	read []byte
}

// buyOrBypass is a step's check whose packets under test are the buy and
// any bypass, refused by an ActionFailed or the not-enough-adena message.
func buyOrBypass(isBuy func(Sent) bool) RefusalCheck {
	return RefusalCheck{
		Packet:       func(s Sent) bool { return isBuy(s) || s.Op == opBypass },
		ActionFailed: true,
		Messages:     []int32{notEnoughCash},
	}
}

// TestRefusalBlamesTheBypass covers refusals MessageOps leaves on any
// packet under test: the bypass sent first takes the blame.
func TestRefusalBlamesTheBypass(t *testing.T) {
	isBuy := func(s Sent) bool { return s.Op == botclient.OpRequestPrivateStoreBuy }
	tied := buyOrBypass(isBuy)
	tied.MessageOps = map[int32][]byte{notEnoughCash: {botclient.OpRequestPrivateStoreBuy}}
	tests := []struct {
		name  string
		check RefusalCheck
		read  []byte
	}{
		{"untied message", buyOrBypass(isBuy), sysMsg(notEnoughCash)},
		{"ActionFailed with tied messages", tied, actionFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWatch(newParkClock())
			w.RecordSent(packetTo(opBypass, storeID))
			w.Record(tt.read)
			r := w.Refused(tt.check)
			if r == nil || r.Packet.Op != opBypass {
				t.Fatalf("refusal %v, want one blaming the bypass", r)
			}
		})
	}
}

func TestRefusal(t *testing.T) {
	buy := step{sent: packetTo(botclient.OpRequestPrivateStoreBuy, storeID)}
	move := step{sent: packetTo(0x01, 5)}
	isBuy := func(s Sent) bool { return s.Op == botclient.OpRequestPrivateStoreBuy && s.Target == storeID }
	withFailed := RefusalCheck{Packet: isBuy, ActionFailed: true, Messages: []int32{notEnoughCash}}
	// A step that sends a bypass then the buy, as a warehouse visit sends
	// its open then its withdrawal: a message tied to the buy's opcode
	// blames the buy, never the bypass sent before it.
	bypass := step{sent: packetTo(opBypass, storeID)}
	tiedToBuy := buyOrBypass(isBuy)
	tiedToBuy.MessageOps = map[int32][]byte{notEnoughCash: {botclient.OpRequestPrivateStoreBuy}}
	tests := []struct {
		name  string
		check RefusalCheck
		steps []step
		want  []byte // the refusal's frame, nil for none
	}{
		{"ActionFailed after the packet", withFailed, []step{buy, {read: actionFailed}}, actionFailed},
		{"listed message after the packet", withFailed, []step{buy, {read: sysMsg(notEnoughCash)}}, sysMsg(notEnoughCash)},
		{"answer read after a later packet", withFailed, []step{buy, move, {wait: time.Second, read: actionFailed}}, actionFailed},
		{"ActionFailed before the packet", withFailed, []step{{read: actionFailed}, buy}, nil},
		{"ActionFailed after another op", withFailed, []step{move, {read: actionFailed}}, nil},
		{"ActionFailed the step does not count", RefusalCheck{Packet: isBuy, Messages: []int32{notEnoughCash}}, []step{buy, {read: actionFailed}}, nil},
		{"unlisted message", withFailed, []step{buy, {read: sysMsg(notEnoughCash + 1)}}, nil},
		{"answer past the window", withFailed, []step{buy, {wait: refusalWindow + time.Millisecond, read: actionFailed}}, nil},
		{"packet before the step", RefusalCheck{From: 1, Packet: isBuy, ActionFailed: true}, []step{buy, move, {read: actionFailed}}, nil},
		{"packet of the step after one before it", RefusalCheck{From: 1, Packet: isBuy, ActionFailed: true}, []step{buy, move, buy, {read: actionFailed}}, actionFailed},
		{"tied message after another packet under test and the buy", tiedToBuy, []step{bypass, buy, {read: sysMsg(notEnoughCash)}}, sysMsg(notEnoughCash)},
		{"tied message after another packet under test only", tiedToBuy, []step{bypass, {read: sysMsg(notEnoughCash)}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newParkClock()
			w := NewWatch(clock)
			for _, s := range tt.steps {
				clock.now = clock.now.Add(s.wait)
				if s.sent != nil {
					w.RecordSent(s.sent)
				}
				if s.read != nil {
					w.Record(s.read)
				}
			}
			r := w.Refused(tt.check)
			if tt.want == nil {
				if r != nil {
					t.Fatalf("refusal %v, want none", r)
				}
				return
			}
			want, _, _ := botclient.DecodeEvent(tt.want)
			if r == nil || r.Answer.Event != want || !isBuy(r.Packet) {
				t.Fatalf("refusal %v, want %+v answering the buy", r, want)
			}
			if len(r.Sent) == 0 || len(r.Seen) == 0 || r.Seen[len(r.Seen)-1].Event != want {
				t.Errorf("refusal tails sent %v, seen %v: want them to end at the refusal", r.Sent, r.Seen)
			}
		})
	}
}

func TestAwaitRefusalReturnsAtTheRefusal(t *testing.T) {
	clock := newParkClock()
	w := NewWatch(clock)
	check := RefusalCheck{
		From:         len(w.Sent()),
		Packet:       func(s Sent) bool { return s.Op == botclient.OpRequestPrivateStoreBuy },
		ActionFailed: true,
	}
	got := make(chan *Refusal, 1)
	go func() {
		r, err := w.AwaitRefusal(t.Context(), check)
		if err != nil {
			t.Error(err)
		}
		got <- r
	}()
	w.Record(actionFailed) // before the packet: not its answer
	w.RecordSent(packetTo(0x01, 5))
	w.Record(actionFailed) // answers the move
	w.RecordSent(packetTo(botclient.OpRequestPrivateStoreBuy, storeID))
	w.Record(sysMsg(notEnoughCash)) // not a listed message
	w.Record(actionFailed)
	r := <-got
	if r == nil || r.Packet.Op != botclient.OpRequestPrivateStoreBuy || r.Answer.SentBefore != 2 {
		t.Fatalf("refusal %v, want the ActionFailed read after the buy", r)
	}
	if n := w.Len(); r.Seen[len(r.Seen)-1] != w.Seen(n - 1)[0] {
		t.Errorf("refusal ends its tail at %+v, want the last event read", r.Seen[len(r.Seen)-1])
	}
}

func TestAwaitRefusalEndsWithCtx(t *testing.T) {
	w := NewWatch(newParkClock())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r, err := w.AwaitRefusal(ctx, RefusalCheck{Packet: func(Sent) bool { return true }, ActionFailed: true}); !errors.Is(err, context.Canceled) || r != nil {
		t.Fatalf("AwaitRefusal = %v, %v; want ctx's error", r, err)
	}
}
