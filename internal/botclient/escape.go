package botclient

import "time"

// escapeEvery and escapeTries bound the bot's /unstuck (RequestUserCommand
// userCommandEscape), which the server does not gate (client_ingame.go
// handleRequestUserCommand): at most escapeTries asks in escapeEvery from
// the first, and none for escapeEvery after a recall the bot asked landed.
// Between two asks the asking rule waits at least escapeRetry.
const (
	escapeEvery = 10 * time.Minute
	escapeTries = 3
)

// escapeBreaker is the one breaker of every /unstuck the bot sends, from the
// walker on isolated ground or from rule "leader" (#286, #335), owned by the
// brain goroutine. Sharing it bounds the sum: escapeTries asks per
// escapeEvery in all. The server's petition refusal is not kept here: each
// asking rule keeps its own, so one rule's refusal never blocks the other.
type escapeBreaker struct {
	// first opens the window that count asks went out in; walker is set
	// when the last ask was the walker's (a petition refusal answers it).
	first  time.Time
	count  int
	walker bool
}

// open reports whether no /unstuck may go out at now.
func (e *escapeBreaker) open(now time.Time) bool {
	return e.count >= escapeTries && now.Sub(e.first) < escapeEvery
}

// asked counts an /unstuck sent at now, by the walker or not.
func (e *escapeBreaker) asked(now time.Time, walker bool) {
	if e.first.IsZero() || now.Sub(e.first) >= escapeEvery {
		e.first, e.count = now, 0
	}
	e.count++
	e.walker = walker
}

// landed opens the breaker for escapeEvery from now, when a recall the bot
// asked landed: where the landing is no better (the restart area of
// acis_golang#5412), asking again only repeats the five-minute cast.
func (e *escapeBreaker) landed(now time.Time) {
	e.first, e.count = now, escapeTries
}
