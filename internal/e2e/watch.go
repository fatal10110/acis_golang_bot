package e2e

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Watch is a scenario's window on a brain-driven bot, whose own events stay
// inside its brain: a botclient.FrameTap (SessionConfig.Recorder) that keeps
// every game event the bot's connections read and every packet they wrote,
// in order, each with the time it passed the tap. The bot's reader and
// writer feed it while the scenario reads it; its methods may be called from
// any goroutine.
type Watch struct {
	clock botclient.Clock

	mu      sync.Mutex
	seen    []Seen        // guarded by mu
	sent    []Sent        // guarded by mu
	changed chan struct{} // guarded by mu; closed and replaced on every append
}

// Seen is an event a watched connection read, at At.
type Seen struct {
	At    time.Time
	Event botclient.Event
}

// Sent is a packet a watched connection wrote, at At. Target is the first
// field after the opcode: the object, skill or item of the botclient.Op*
// packets, 0 for a packet shorter than that.
type Sent struct {
	At      time.Time
	Op      byte
	Target  int32
	Payload []byte
}

// NewWatch returns an empty Watch timed by clock.
func NewWatch(clock botclient.Clock) *Watch {
	return &Watch{clock: clock, changed: make(chan struct{})}
}

// Record keeps the event payload decodes to; a frame the reader drops or
// cannot decode is skipped, as the bot's reader skips or fails on it.
func (w *Watch) Record(payload []byte) {
	ev, ok, err := botclient.DecodeEvent(payload)
	if err != nil || !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen = append(w.seen, Seen{At: w.clock.Now(), Event: ev})
	w.notify()
}

// RecordSent keeps a copy of payload.
func (w *Watch) RecordSent(payload []byte) {
	s := Sent{Op: payload[0], Payload: bytes.Clone(payload)}
	if len(payload) >= 5 {
		s.Target = int32(binary.LittleEndian.Uint32(payload[1:5]))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s.At = w.clock.Now()
	w.sent = append(w.sent, s)
	w.notify()
}

// notify wakes every Await; w.mu is held.
func (w *Watch) notify() {
	close(w.changed)
	w.changed = make(chan struct{})
}

// Seen returns the events read from index from on.
func (w *Watch) Seen(from int) []Seen {
	w.mu.Lock()
	defer w.mu.Unlock()
	if from >= len(w.seen) {
		return nil
	}
	return append([]Seen(nil), w.seen[from:]...)
}

// Len is the number of events read so far: the index the next one gets.
func (w *Watch) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.seen)
}

// Sent returns the packets written so far.
func (w *Watch) Sent() []Sent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Sent(nil), w.sent...)
}

// Await waits until an event read at index from or later matches and
// returns its index and it, or ctx's error once ctx ends. match runs with
// the watch locked, once per event: it must not call w's methods.
func (w *Watch) Await(ctx context.Context, from int, match func(botclient.Event) bool) (int, Seen, error) {
	for {
		w.mu.Lock()
		for i := max(from, 0); i < len(w.seen); i++ {
			if match(w.seen[i].Event) {
				s := w.seen[i]
				w.mu.Unlock()
				return i, s, nil
			}
		}
		from = max(from, len(w.seen))
		changed := w.changed
		w.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, Seen{}, ctx.Err()
		case <-changed:
		}
	}
}
