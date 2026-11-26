package botclient

import (
	"maps"
	"time"
)

// holds keeps keys held until a time each: shunned NPCs, avoided spots,
// retries, the blacklist. A key is held while now is before its time. A
// holds is unsynchronized: the brain goroutine owns each one it keeps, and
// a map shared by brains behind a lock does not use it.
type holds[K comparable] map[K]time.Time

// set holds k until until, replacing the time it had.
func (h holds[K]) set(k K, until time.Time) { h[k] = until }

// extend holds k until until unless it is held later already: it keeps the
// later time and never shortens a hold.
func (h holds[K]) extend(k K, until time.Time) {
	if until.After(h[k]) {
		h[k] = until
	}
}

// held reports whether k is held at now: false for a key never set, and
// for any key of a nil holds.
func (h holds[K]) held(k K, now time.Time) bool { return now.Before(h[k]) }

// prune forgets the keys not held at now. It does nothing on a nil holds.
func (h holds[K]) prune(now time.Time) {
	maps.DeleteFunc(h, func(_ K, until time.Time) bool { return !now.Before(until) })
}
