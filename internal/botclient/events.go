package botclient

import (
	"slices"
	"sync"
)

// Event is one decoded server packet, handed from a session's reader to its
// brain through Events. Every event is a value type that shares no memory
// with the frame it was decoded from: the reader reuses its frame buffer, so
// no slice of it may cross goroutines. The brain type-switches on the
// concrete types below.
type Event interface{ event() }

// Point is a world coordinate.
type Point struct{ X, Y, Z int32 }

// MoveToLocation reports an object starting to move from Origin toward Dest.
// It is the one event Events coalesces: a newer one for the same object
// supersedes a queued one.
type MoveToLocation struct {
	ObjectID     int32
	Dest, Origin Point
}

// StopMove reports an object stopped at Pos.
type StopMove struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// ValidateLocation corrects an object's position to Pos.
type ValidateLocation struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// TeleportToLocation reports an object placed at Pos without walking there.
// Fast is an in-place correction rather than a loading-screen teleport.
type TeleportToLocation struct {
	ObjectID int32
	Pos      Point
	Fast     bool
}

// DeleteObject reports an object leaving sight. Seated marks a sitting
// character the client stands up before removing it.
type DeleteObject struct {
	ObjectID int32
	Seated   bool
}

// LeaveWorld is the server's answer to an accepted Logout; it closes the
// connection right after.
type LeaveWorld struct{}

// ServerClose is the server's last packet before it drops the connection.
type ServerClose struct{}

func (MoveToLocation) event()     {}
func (StopMove) event()           {}
func (ValidateLocation) event()   {}
func (TeleportToLocation) event() {}
func (DeleteObject) event()       {}
func (LeaveWorld) event()         {}
func (ServerClose) event()        {}

// Events is the bounded queue from one session's reader to its brain, one
// per session attempt.
//
// The contract: the reader (GameConn.ReadEvents) is the only producer and
// the brain the only consumer. The brain selects on Ready next to its timer
// and the session's done channel, then takes everything queued with Drain.
// When limit events are queued the reader waits for a Drain or for done,
// whichever comes first, so closing done never deadlocks a reader on a full
// queue. While the brain is behind, a MoveToLocation replaces the queued
// MoveToLocation of the same object and moves to the back of the queue: an
// older move is superseded by a newer one, and the newer one stays behind
// every other event already queued for that object (StopMove, a teleport,
// DeleteObject), so coalescing never reorders one object's events.
type Events struct {
	mu    sync.Mutex
	queue []Event // guarded by mu; at most one MoveToLocation per object
	limit int
	ready chan struct{} // capacity 1: signaled when an event is queued
	space chan struct{} // capacity 1: signaled when Drain empties the queue
}

// NewEvents returns an empty queue that holds up to limit events (at least
// one).
func NewEvents(limit int) *Events {
	limit = max(limit, 1)
	return &Events{
		queue: make([]Event, 0, limit),
		limit: limit,
		ready: make(chan struct{}, 1),
		space: make(chan struct{}, 1),
	}
}

// Ready yields once events are queued. A wakeup may find the queue already
// drained; Drain then returns dst unchanged.
func (q *Events) Ready() <-chan struct{} { return q.ready }

// Drain appends every queued event to dst in arrival order, empties the
// queue and returns dst.
func (q *Events) Drain(dst []Event) []Event {
	q.mu.Lock()
	dst = append(dst, q.queue...)
	clear(q.queue)
	q.queue = q.queue[:0]
	q.mu.Unlock()
	signal(q.space)
	return dst
}

// push queues ev, coalescing a MoveToLocation, and waits for room while the
// queue is full. It reports false when done closed first.
func (q *Events) push(ev Event, done <-chan struct{}) bool {
	for {
		q.mu.Lock()
		ok := q.put(ev)
		q.mu.Unlock()
		if ok {
			signal(q.ready)
			return true
		}
		select {
		case <-q.space:
		case <-done:
			return false
		}
	}
}

// put queues ev unless the queue is full. A MoveToLocation first drops the
// queued move of its object, so it always fits.
func (q *Events) put(ev Event) bool {
	if m, ok := ev.(MoveToLocation); ok {
		if i := slices.IndexFunc(q.queue, func(e Event) bool {
			prev, ok := e.(MoveToLocation)
			return ok && prev.ObjectID == m.ObjectID
		}); i >= 0 {
			q.queue = slices.Delete(q.queue, i, i+1)
		}
	}
	if len(q.queue) >= q.limit {
		return false
	}
	q.queue = append(q.queue, ev)
	return true
}

// signal wakes c's receiver without blocking; a pending wakeup suffices.
func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}
