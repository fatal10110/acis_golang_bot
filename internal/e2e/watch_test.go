package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// autoAttackStart is the server's AutoAttackStart frame (opcode 0x2b) of id.
func autoAttackStart(id int32) []byte {
	return []byte{0x2b, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)}
}

func TestWatchRecordsBothWays(t *testing.T) {
	clock := newParkClock()
	w := NewWatch(clock)
	w.Record([]byte{0xfe, 0x01}) // dropped by the reader
	w.Record([]byte{0x2b, 0x01}) // malformed
	w.Record(autoAttackStart(7)) // kept
	w.Record([]byte{0x25})       // ActionFailed
	attack := botclient.EncodeAttackRequest(-5, botclient.Point{X: 1})
	w.RecordSent(attack)
	attack[1] = 0 // the watch keeps its own copy
	w.RecordSent([]byte{0x09})

	seen := w.Seen(0)
	if len(seen) != 2 || w.Len() != 2 {
		t.Fatalf("seen %+v (Len %d), want the two decodable events", seen, w.Len())
	}
	if ev, ok := seen[0].Event.(botclient.AutoAttackStart); !ok || ev.ObjectID != 7 || !seen[0].At.Equal(clock.now) {
		t.Errorf("seen[0] = %+v, want AutoAttackStart 7 at %v", seen[0], clock.now)
	}
	if _, ok := seen[1].Event.(botclient.ActionFailed); !ok {
		t.Errorf("seen[1] = %+v, want ActionFailed", seen[1])
	}
	if got := w.Seen(1); len(got) != 1 {
		t.Errorf("Seen(1) = %+v, want the last event", got)
	}
	if got := w.Seen(5); got != nil {
		t.Errorf("Seen(5) = %+v, want none", got)
	}
	sent := w.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %+v, want 2", sent)
	}
	if s := sent[0]; s.Op != botclient.OpAttackRequest || s.Target != -5 || s.Payload[1] != 0xfb {
		t.Errorf("sent[0] = %+v, want AttackRequest on -5, payload kept", s)
	}
	if s := sent[1]; s.Op != 0x09 || s.Target != 0 {
		t.Errorf("sent[1] = %+v, want opcode 0x09 and no target", s)
	}
}

func TestWatchAwait(t *testing.T) {
	w := NewWatch(newParkClock())
	w.Record(autoAttackStart(1))
	of := func(id int32) func(botclient.Event) bool {
		return func(ev botclient.Event) bool {
			a, ok := ev.(botclient.AutoAttackStart)
			return ok && a.ObjectID == id
		}
	}
	if i, s, err := w.Await(t.Context(), 0, of(1)); err != nil || i != 0 || s.Event.(botclient.AutoAttackStart).ObjectID != 1 {
		t.Fatalf("Await(0, 1) = %d, %+v, %v; want the recorded event", i, s, err)
	}

	got := make(chan int, 1)
	go func() {
		i, _, err := w.Await(t.Context(), 1, of(1))
		if err != nil {
			i = -1
		}
		got <- i
	}()
	w.Record(autoAttackStart(2))
	w.Record(autoAttackStart(1))
	if i := <-got; i != 2 {
		t.Fatalf("Await(1, 1) = %d, want the later event at 2", i)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := w.Await(ctx, 3, of(1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Await on an ended ctx = %v, want context.Canceled", err)
	}
}
