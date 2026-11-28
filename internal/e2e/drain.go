package e2e

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// inboxLimit bounds the events a Client keeps unread: minutes of a crowded
// town's broadcasts (about 30 a second in Gludin), a few MiB at most.
const inboxLimit = 4096

// gameConn is the connection a Client drives: *botclient.GameConn, or a
// scripted one in tests.
type gameConn interface {
	Read(deadline time.Time) ([]byte, error)
	Send(payload []byte) error
	Close() error
}

// frameSource is what a Client's reader reads.
type frameSource interface {
	Read(deadline time.Time) ([]byte, error)
}

// item is one entry of an inbox: an event, or the error decoding a frame.
type item struct {
	ev  botclient.Event
	err error
}

// inbox is the bounded queue between a Client's reader, which pushes, and
// Await, which pops, in the order the frames arrived. push never blocks:
// past limit it drops the oldest event retained does not hold (broadcasts
// about others, mostly), or the oldest entry of all when every one is held,
// and counts the drop. Once the reader ends, pop hands out what is left,
// then the reason it ended, again on every later call.
type inbox struct {
	limit int
	// self is the character's object id, for retained.
	self int32
	// ready holds a token once an entry or the end arrived since the last
	// wait took it.
	ready chan struct{}

	mu      sync.Mutex
	items   []item // guarded by mu
	end     error  // guarded by mu; nil while the reader runs
	dropped uint64 // guarded by mu
}

func newInbox(limit int, self int32) *inbox {
	return &inbox{limit: limit, self: self, ready: make(chan struct{}, 1)}
}

// push queues it, dropping one entry first when the inbox is full.
func (b *inbox) push(it item) {
	b.mu.Lock()
	if len(b.items) >= b.limit {
		b.dropOne()
	}
	b.items = append(b.items, it)
	b.mu.Unlock()
	b.signal()
}

// dropOne removes the oldest entry retained does not hold, else the oldest.
// b.mu is held.
func (b *inbox) dropOne() {
	i := 0
	for j, it := range b.items {
		if it.err == nil && !retained(it.ev, b.self) {
			i = j
			break
		}
	}
	b.dropped++
	if i == 0 {
		b.items[0] = item{}
		b.items = b.items[1:]
		return
	}
	b.items = append(b.items[:i], b.items[i+1:]...)
}

// close records why the reader ended.
func (b *inbox) close(err error) {
	b.mu.Lock()
	b.end = err
	b.mu.Unlock()
	b.signal()
}

func (b *inbox) signal() {
	select {
	case b.ready <- struct{}{}:
	default:
	}
}

// pop takes the oldest entry; once the inbox is empty and the reader ended,
// the reason it ended. ok is false while the inbox is empty and the reader
// runs: wait on ready, then pop again.
func (b *inbox) pop() (it item, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) > 0 {
		it = b.items[0]
		b.items[0] = item{}
		b.items = b.items[1:]
		return it, true
	}
	if b.end != nil {
		return item{err: b.end}, true
	}
	return item{}, false
}

// drops is how many entries push dropped.
func (b *inbox) drops() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// retained reports whether ev outlives a full inbox while broadcasts about
// others go: the session's end, the answers to the character's own requests
// (ActionFailed, MyTargetSelected) and every event follow acts on.
func retained(ev botclient.Event, self int32) bool {
	switch e := ev.(type) {
	case botclient.ServerClose, botclient.LeaveWorld, botclient.ActionFailed, botclient.MyTargetSelected:
		return true
	case botclient.TargetUnselected:
		return e.ObjectID == self
	case botclient.TeleportToLocation:
		return e.ObjectID == self
	case botclient.MoveToLocation:
		return e.ObjectID == self
	case botclient.StopMove:
		return e.ObjectID == self
	case botclient.ValidateLocation:
		return e.ObjectID == self
	}
	return false
}

// drain is a Client's reader: it reads src until a read fails, decodes the
// frames whose opcode has a decoder and pushes the events, and the errors
// of frames that fail to decode, on in; every other frame is dropped without
// parsing. It is src's only reader. Teardown: closing the connection fails
// the blocked read; drain then closes in with the read error and returns,
// and the Client joins it (Close).
func drain(src frameSource, in *inbox) {
	for {
		raw, err := src.Read(time.Time{})
		if err != nil {
			in.close(fmt.Errorf("read game frame: %w", err))
			return
		}
		ev, ok, err := botclient.DecodeEvent(raw)
		switch {
		case err != nil:
			in.push(item{err: err})
		case ok:
			in.push(item{ev: ev})
		}
	}
}
