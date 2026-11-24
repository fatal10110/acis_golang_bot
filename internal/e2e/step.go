package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// stepBound is how long a waiting step of a live scenario may take when it
// names no bound of its own: no step of a recorded passing run took that
// long, so an overdue step fails minutes before its scenario's deadline. A
// step that may take longer (a farm fight, a supply trip, a buff's
// renewal) names its own deadline, with the reason, at its call site.
const stepBound = 5 * time.Minute

// stepNote is how long a step waits before its served time is logged: a
// run's log then shows each slow step's margin under its bound.
const stepNote = 10 * time.Second

// Overdue is a waiting step of a live scenario that was not served by its
// deadline.
type Overdue struct {
	What string
	// Elapsed is how long the step waited, Bound how long it was given.
	Elapsed, Bound time.Duration
	// Missing is the step's last report of what it still waits for.
	Missing string
	// Last is the last packets relevant to the step, "" for none recorded.
	Last string
}

func (o *Overdue) Error() string {
	s := fmt.Sprintf("%s: overdue: not served after %v, bound %v; missing: %s",
		o.What, o.Elapsed.Round(time.Second), o.Bound.Round(time.Second), o.Missing)
	if o.Last != "" {
		s += "; " + o.Last
	}
	return s
}

// awaitStep runs one waiting step of a live scenario. It calls report, which
// returns what the step still waits for ("" once it is served), then poll,
// which waits for the next look (a tick, or a client read for a while), and
// so on until report returns "". It returns how long the step waited, with
// an *Overdue once clock reaches due, or poll's error (ctx's when the
// scenario ends first, a refusal) wrapped with the step and what is
// missing. last, when set, renders the packets the step last saw for the
// Overdue.
func awaitStep(ctx context.Context, clock botclient.Clock, what string, due time.Time,
	poll func(context.Context) error, report, last func() string,
) (time.Duration, error) {
	start := clock.Now()
	for {
		missing := report()
		now := clock.Now()
		elapsed := now.Sub(start)
		if missing == "" {
			return elapsed, nil
		}
		if !now.Before(due) {
			o := &Overdue{What: what, Elapsed: elapsed, Bound: due.Sub(start), Missing: missing}
			if last != nil {
				o.Last = last()
			}
			return elapsed, o
		}
		err := ctx.Err()
		if err == nil {
			err = poll(ctx)
		}
		if err != nil {
			return elapsed, fmt.Errorf("%s: %w after %v; missing: %s", what, err, elapsed.Round(time.Second), missing)
		}
	}
}

// tickPoll is a poll that waits for the next tick of c.
func tickPoll(c <-chan time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c:
			return nil
		}
	}
}

// pending is a report of cond: "" once it holds.
func pending(cond func() bool) func() string {
	return func() string {
		if cond() {
			return ""
		}
		return "not yet"
	}
}

// eventTail keeps the last refusalTail events a step read, for an overdue
// step's message. It belongs to the goroutine reading the step's client.
type eventTail struct {
	clock botclient.Clock
	seen  []Seen
}

func (e *eventTail) note(ev botclient.Event) {
	if len(e.seen) == refusalTail {
		e.seen = append(e.seen[:0], e.seen[1:]...)
	}
	e.seen = append(e.seen, Seen{At: e.clock.Now(), Event: ev})
}

func (e *eventTail) String() string {
	var b strings.Builder
	writeSeen(&b, e.seen)
	return b.String()
}

// Tail renders the last refusalTail packets the watch recorded written and
// read, for an overdue step's message.
func (w *Watch) Tail() string {
	w.mu.Lock()
	sent := w.sent[max(0, len(w.sent)-refusalTail):]
	seen := w.seen[max(0, len(w.seen)-refusalTail):]
	var b strings.Builder
	writeSent(&b, sent)
	b.WriteString("; ")
	writeSeen(&b, seen)
	w.mu.Unlock()
	return b.String()
}

func writeSent(b *strings.Builder, sent []Sent) {
	b.WriteString("last sent:")
	for _, s := range sent {
		fmt.Fprintf(b, " 0x%02x(%d)@%s", s.Op, s.Target, s.At.Format("15:04:05.000"))
	}
}

func writeSeen(b *strings.Builder, seen []Seen) {
	b.WriteString("last read:")
	for _, s := range seen {
		fmt.Fprintf(b, " %s@%s", eventText(s.Event), s.At.Format("15:04:05.000"))
	}
}
