package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// refusalWindow bounds how long after a packet an answer counts as its
// refusal: the server refuses a request as it handles it, so its answer
// follows within a round trip, while a later one answers something else.
const refusalWindow = 3 * time.Second

// refusalTail is how many of the last packets written and events read a
// Refusal keeps for the failure message.
const refusalTail = 8

// RefusalCheck names what refuses one scenario step: an answer the server
// reads to the step's packet. ActionFailed is a normal answer in some flows
// (a brain retry, a move, the end of a dialog click), so a step counts it
// only when it says so.
type RefusalCheck struct {
	// From is the index of the first packet of the step (len(Watch.Sent())
	// as the step starts): an earlier packet is not the step's.
	From int
	// Packet reports whether a written packet is the one under test. It
	// runs with the watch locked: it must not call the watch's methods.
	Packet func(Sent) bool
	// ActionFailed counts an ActionFailed as a refusal.
	ActionFailed bool
	// Messages are the SystemMessage ids that refuse the packet.
	Messages []int32
}

// refuses reports whether ev is one of c's refusals.
func (c RefusalCheck) refuses(ev botclient.Event) bool {
	switch e := ev.(type) {
	case botclient.ActionFailed:
		return c.ActionFailed
	case botclient.SystemMessage:
		return slices.Contains(c.Messages, e.ID)
	}
	return false
}

// Refusal is the server's refusal of a step's packet, with the last packets
// written and events read up to it for the failure message.
type Refusal struct {
	Packet Sent
	Answer Seen
	Sent   []Sent
	Seen   []Seen
}

func (r *Refusal) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "server refused packet 0x%02x (target %d) with %s %s after it",
		r.Packet.Op, r.Packet.Target, eventText(r.Answer.Event), r.Answer.At.Sub(r.Packet.At))
	b.WriteString("; last sent:")
	for _, s := range r.Sent {
		fmt.Fprintf(&b, " 0x%02x(%d)@%s", s.Op, s.Target, s.At.Format("15:04:05.000"))
	}
	b.WriteString("; last read:")
	for _, s := range r.Seen {
		fmt.Fprintf(&b, " %s@%s", eventText(s.Event), s.At.Format("15:04:05.000"))
	}
	return b.String()
}

// eventText is ev's type and fields, cut short: an ItemList would fill
// the failure message.
func eventText(ev botclient.Event) string {
	const most = 120
	s := fmt.Sprintf("%T%+v", ev, ev)
	if len(s) > most {
		return s[:most] + "..."
	}
	return s
}

// refusalScan finds a refusal over a watch's growing record, looking at
// each packet and event once.
type refusalScan struct {
	check    RefusalCheck
	nextSent int
	nextSeen int
	packets  []int // indexes of the packets under test written so far
}

// scan looks at the packets and events recorded since its last call and
// returns the first refusal, or nil. Every event answers only packets
// written before it, all of which sent already holds.
func (r *refusalScan) scan(sent []Sent, seen []Seen) *Refusal {
	for r.nextSent = max(r.nextSent, r.check.From); r.nextSent < len(sent); r.nextSent++ {
		if r.check.Packet(sent[r.nextSent]) {
			r.packets = append(r.packets, r.nextSent)
		}
	}
	for ; r.nextSeen < len(seen); r.nextSeen++ {
		ev := seen[r.nextSeen]
		if ev.SentBefore <= r.check.From || !r.check.refuses(ev.Event) {
			continue
		}
		for _, i := range r.packets {
			if i < ev.SentBefore && ev.At.Sub(sent[i].At) <= refusalWindow {
				return &Refusal{
					Packet: sent[i],
					Answer: ev,
					Sent:   slices.Clone(sent[max(0, ev.SentBefore-refusalTail):ev.SentBefore]),
					Seen:   slices.Clone(seen[max(0, r.nextSeen+1-refusalTail) : r.nextSeen+1]),
				}
			}
		}
	}
	return nil
}

// Refused returns the first refusal c names that the watch holds, or nil.
func (w *Watch) Refused(c RefusalCheck) *Refusal {
	r := refusalScan{check: c}
	w.mu.Lock()
	defer w.mu.Unlock()
	return r.scan(w.sent, w.seen)
}

// AwaitRefusal waits until the watch holds a refusal c names and returns
// it, or ctx's error once ctx ends. It looks again on every frame recorded,
// so a refusal ends the wait as soon as it is read.
func (w *Watch) AwaitRefusal(ctx context.Context, c RefusalCheck) (*Refusal, error) {
	r := refusalScan{check: c}
	for {
		w.mu.Lock()
		ref := r.scan(w.sent, w.seen)
		changed := w.changed
		w.mu.Unlock()
		if ref != nil {
			return ref, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
