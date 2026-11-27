package botclient

import (
	"fmt"
	"time"
)

// frameSource is what the reader reads: *GameConn, or a scripted source in
// tests.
type frameSource interface {
	Read(deadline time.Time) ([]byte, error)
}

// ReadEvents is the session's reader. It reads the connection until it
// fails or done closes, decodes the frames whose opcode has a decoder into
// events and queues them on q for the brain; every other frame is dropped
// without parsing. The -record tap (SetRecorder) sees every frame read,
// decoded or not.
//
// Teardown: the session owns done and the connection. It closes done, then
// closes the connection so a blocked Read returns, and joins ReadEvents
// before it dials again; a wait on a full q ends on done alone. ReadEvents
// returns nil when it ends after done closed, else the read error that ended
// it. A frame that fails to decode does not end it (docs/plan.md D45): it is
// dropped and queued as decodeSkipped, its opcode and length, for the brain
// to log and count, and the reader reads on; a ServerClose or LeaveWorld
// that fails to decode is still queued, as its opcode alone. ServerClose and
// LeaveWorld are queued as events like any other; the read error after them
// is still returned.
//
// ReadEvents is the connection's only reader while it runs (see GameConn).
func (g *GameConn) ReadEvents(q *Events, done <-chan struct{}) error {
	return readEvents(g, q, done)
}

func readEvents(src frameSource, q *Events, done <-chan struct{}) error {
	for {
		raw, err := src.Read(time.Time{})
		if err != nil {
			select {
			case <-done:
				return nil
			default:
				return fmt.Errorf("read game frame: %w", err)
			}
		}
		ev, ok, err := decodeEvent(raw)
		if err != nil {
			// The frame is dropped and the session reads on (docs/plan.md
			// D45): the brain logs and counts it. ServerClose and
			// LeaveWorld still end the session: both layouts are empty, so
			// the opcode alone is the event.
			if !q.push(decodeSkipped{key: frameKey(raw), length: len(raw), err: err}, done) {
				return nil
			}
			switch raw[0] {
			case opGameServerClose:
				ev, ok = ServerClose{}, true
			case opGameLeaveWorld:
				ev, ok = LeaveWorld{}, true
			}
		}
		if ok && !q.push(ev, done) {
			return nil
		}
	}
}
