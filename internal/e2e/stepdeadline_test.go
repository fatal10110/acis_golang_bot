package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

func TestAwaitStep(t *testing.T) {
	refusal := &Refusal{Packet: Sent{Op: botclient.OpRequestBuyItem}, Answer: Seen{Event: botclient.ActionFailed{}}}
	tests := []struct {
		name   string
		bound  time.Duration
		poll   time.Duration // the clock's advance on every poll
		served int           // the report call that returns "", 0 for none
		end    int           // the poll on which the scenario's ctx ends, 0 for none
		refuse int           // the poll that returns a refusal, 0 for none
		// want is the step's wait, and its error: overdue, ctx's or the
		// refusal (nil for served).
		want    time.Duration
		overdue bool
		wantErr error
	}{
		{name: "served at once", bound: 2 * time.Minute, poll: 2 * time.Second, served: 1},
		{name: "served in time", bound: 2 * time.Minute, poll: 2 * time.Second, served: 31, want: time.Minute},
		{name: "served on the last look before due", bound: 2 * time.Minute, poll: 30 * time.Second, served: 4, want: 90 * time.Second},
		{name: "overdue", bound: 2 * time.Minute, poll: 2 * time.Second, want: 2 * time.Minute, overdue: true},
		{name: "served on the look at due is served", bound: 2 * time.Minute, poll: 30 * time.Second, served: 5, want: 2 * time.Minute},
		{name: "overdue past a slow poll", bound: 2 * time.Minute, poll: 50 * time.Second, want: 150 * time.Second, overdue: true},
		// TestLiveSupport's leader buffs (#366): a bot that never buffs the
		// leader fails at its bound, not at the scenario's 12 minutes.
		{name: "support leader buffs never cast", bound: leaderBuffBound, poll: 500 * time.Millisecond, want: leaderBuffBound, overdue: true},
		{name: "scenario ctx ends first", bound: 2 * time.Minute, poll: 2 * time.Second, end: 10, want: 18 * time.Second, wantErr: context.Canceled},
		{name: "refused", bound: 2 * time.Minute, poll: 2 * time.Second, refuse: 3, want: 4 * time.Second, wantErr: refusal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newParkClock()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var reports, polls int
			report := func() string {
				reports++
				if reports == tt.served {
					return ""
				}
				return fmt.Sprintf("look %d", reports)
			}
			poll := func(ctx context.Context) error {
				polls++
				if polls == tt.end {
					cancel()
					return ctx.Err()
				}
				if polls == tt.refuse {
					return refusal
				}
				clock.now = clock.now.Add(tt.poll)
				return nil
			}
			last := func() string { return "last read: ActionFailed" }
			got, err := awaitStep(ctx, clock, "the step", clock.now.Add(tt.bound), poll, report, last)
			if got != tt.want {
				t.Errorf("waited %v, want %v", got, tt.want)
			}
			var o *Overdue
			switch {
			case tt.overdue:
				if !errors.As(err, &o) {
					t.Fatalf("error %v, want overdue", err)
				}
				if o.What != "the step" || o.Elapsed != tt.want || o.Bound != tt.bound || o.Missing != fmt.Sprintf("look %d", reports) {
					t.Errorf("overdue %+v", *o)
				}
				for _, part := range []string{"the step: overdue", tt.want.String(), "missing: look", "last read: ActionFailed"} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("%q lacks %q", err, part)
					}
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) || errors.As(err, &o) {
					t.Fatalf("error %v, want %v", err, tt.wantErr)
				}
				if !strings.HasPrefix(err.Error(), "the step: ") || !strings.Contains(err.Error(), "missing: look") {
					t.Errorf("error %q names neither the step nor what is missing", err)
				}
			case err != nil:
				t.Fatalf("error %v, want served", err)
			}
		})
	}
}

func TestEventTail(t *testing.T) {
	clock := newParkClock()
	tail := eventTail{clock: clock}
	for i := range refusalTail + 3 {
		tail.note(botclient.SystemMessage{ID: int32(i)})
	}
	if len(tail.seen) != refusalTail {
		t.Fatalf("kept %d events, want %d", len(tail.seen), refusalTail)
	}
	got := tail.String()
	if strings.Contains(got, "ID:2 ") || !strings.Contains(got, "ID:3 ") || !strings.Contains(got, fmt.Sprintf("ID:%d ", refusalTail+2)) {
		t.Errorf("tail %q, want the last %d events", got, refusalTail)
	}
}

func TestWatchTail(t *testing.T) {
	w := NewWatch(newParkClock())
	for i := range refusalTail + 2 {
		w.RecordSent(botwire.EncodeAction(int32(100+i), botclient.Point{}))
	}
	got := w.Tail()
	if strings.Contains(got, "(101)") || !strings.Contains(got, "(102)") || !strings.Contains(got, fmt.Sprintf("(%d)", 101+refusalTail)) {
		t.Errorf("tail %q, want the last %d packets", got, refusalTail)
	}
}
