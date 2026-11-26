package botregistry

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// townBot is an online fake bot of character name at level 20 in the
// test's town, of class (10, a Human Mystic, heals).
func townBot(name string, class int32) *fakeBot {
	f := newFakeBot(name)
	town := botclient.Point{X: 1000, Y: 1000}
	f.set(func(s *botclient.Status) {
		s.ClassID, s.Heals, s.Town, s.Position = class, class == 10, &town, town
	})
	return f
}

// organizerOf returns the organizer of the bots of presets ("farmer",
// "support", ...), accounts hz1 on, and their fakes: farmers are Human
// Fighters, the others Human Mystics.
func organizerOf(t *testing.T, clock *fakeClock, presets ...string) (*Organizer, []*fakeBot) {
	t.Helper()
	var bots []OrganizedBot
	var fakes []*fakeBot
	for i, p := range presets {
		class := int32(10)
		if p == botparty.PresetFarmer {
			class = 0
		}
		f := townBot(fmt.Sprintf("Bot%d", i+1), class)
		fakes = append(fakes, f)
		bots = append(bots, OrganizedBot{Account: fmt.Sprintf("hz%d", i+1), Preset: p, Bot: f})
	}
	o, err := NewOrganizer(OrganizerConfig{Bots: bots, Policy: botparty.DefaultPolicy(), Clock: clock, Metrics: botmetrics.NewRegistry()})
	if err != nil {
		t.Fatal(err)
	}
	return o, fakes
}

func TestOrganizerPass(t *testing.T) {
	farmer, support := botparty.PresetFarmer, botparty.PresetSupport
	tests := []struct {
		name    string
		presets []string
		// setup edits the fakes before the first pass.
		setup func(f []*fakeBot)
		// want are the commands handed to each fake after the first pass.
		want [][]string
	}{
		{
			name:    "a farmer and a healer form a party: the member's join first",
			presets: []string{farmer, support},
			want:    [][]string{nil, {"join Bot1"}},
		},
		{
			name:    "at most FormPerPass parties a pass",
			presets: []string{farmer, farmer, farmer, support, support, support},
			want:    [][]string{nil, nil, nil, {"join Bot1"}, {"join Bot2"}, nil},
		},
		{
			name:    "an offline bot takes no part",
			presets: []string{farmer, support},
			setup:   func(f []*fakeBot) { f[1].set(func(s *botclient.Status) { s.State = botclient.StateReconnecting }) },
			want:    [][]string{nil, nil},
		},
		{
			name:    "a stopped bot takes no part",
			presets: []string{farmer, support},
			setup:   func(f []*fakeBot) { f[0].set(func(s *botclient.Status) { s.State = botclient.StateStopped }) },
			want:    [][]string{nil, nil},
		},
		{
			name:    "farmers alone form nothing: no healer",
			presets: []string{farmer, farmer, farmer},
			want:    [][]string{nil, nil, nil},
		},
		{
			name:    "a swarm of a non-party preset forms nothing",
			presets: []string{"town", "merchant", "pvp"},
			want:    [][]string{nil, nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, f := organizerOf(t, newClock(), tt.presets...)
			if tt.setup != nil {
				tt.setup(f)
			}
			o.pass()
			for i := range f {
				wantHanded(t, fmt.Sprintf("bot %d", i+1), f[i], tt.want[i]...)
			}
		})
	}
}

// TestOrganizerOrders walks a party's orders through their answers: one
// order at a time, the pending answer polled before the snapshot, a full
// mailbox and an error answer planned again.
func TestOrganizerOrders(t *testing.T) {
	clock := newClock()
	o, f := organizerOf(t, clock, botparty.PresetFarmer, botparty.PresetSupport)
	lead, mem := f[0], f[1]
	if !o.links["hz1"].newRun {
		t.Fatal("a bot before the first pass is not a new run")
	}

	o.pass()
	wantHanded(t, "first pass", mem, "join Bot1")
	wantHanded(t, "first pass", lead)
	if l := o.links["hz2"]; l.sent.Answer != botparty.AnswerPending || l.newRun {
		t.Fatalf("member after the first pass: %+v, want the join pending", l)
	}
	if len(o.state.Parties) != 1 {
		t.Fatalf("state %+v, want one party", o.state)
	}

	clock.Advance(PartyTick)
	o.pass()
	wantHanded(t, "join pending", mem, "join Bot1")
	wantHanded(t, "join pending", lead)

	mem.answer(nil, "join Bot1")
	lead.setBusy(true)
	clock.Advance(PartyTick)
	o.pass()
	wantHanded(t, "leader busy", lead, "lead Bot2")
	if l := o.links["hz1"]; l.sent.Answer != botparty.AnswerNone {
		t.Fatalf("leader after ErrBusy: %+v, want no answer", l.sent)
	}

	lead.setBusy(false)
	clock.Advance(PartyTick)
	o.pass()
	wantHanded(t, "busy retried", lead, "lead Bot2", "lead Bot2")

	lead.answer(botclient.ErrStopped, "")
	clock.Advance(PartyTick)
	o.pass()
	wantHanded(t, "error answer", lead, "lead Bot2", "lead Bot2", "lead Bot2")

	lead.answer(nil, "lead Bot2")
	clock.Advance(PartyTick)
	o.pass()
	wantHanded(t, "answered", lead, "lead Bot2", "lead Bot2", "lead Bot2")
	wantHanded(t, "answered", mem, "join Bot1")
	if snap := o.cfg.Metrics.Collect(clock.Now()); snap.PartiesFormed != 1 {
		t.Fatalf("parties formed %d, want 1", snap.PartiesFormed)
	}
}

// TestOrganizerRun: Run passes on every PartyTick of its clock and returns,
// its timer stopped, once its context is done.
func TestOrganizerRun(t *testing.T) {
	clock := newClock()
	o, f := organizerOf(t, clock, botparty.PresetFarmer, botparty.PresetSupport)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		o.Run(ctx)
	}()
	timer := func() *fakeTimer {
		for range 10000 {
			clock.mu.Lock()
			n := len(clock.timers)
			clock.mu.Unlock()
			if n == 1 {
				clock.mu.Lock()
				defer clock.mu.Unlock()
				return clock.timers[0]
			}
			runtime.Gosched()
		}
		t.Fatal("Run never armed its timer")
		return nil
	}()

	clock.Advance(PartyTick - 1)
	wantHanded(t, "before the tick", f[1])
	clock.Advance(1)
	for range 10000 {
		if len(f[1].handed()) > 0 {
			break
		}
		runtime.Gosched()
	}
	wantHanded(t, "first tick", f[1], "join Bot1")

	cancel()
	<-done
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if timer.active {
		t.Fatal("Run returned with its timer armed")
	}
}

func TestNewOrganizer(t *testing.T) {
	f := newFakeBot("A")
	small := botparty.DefaultPolicy()
	small.Size = 1
	tests := []struct {
		name    string
		cfg     OrganizerConfig
		wantErr string
	}{
		{"no clock", OrganizerConfig{Policy: botparty.DefaultPolicy()}, "clock"},
		{"party of one", OrganizerConfig{Policy: small, Clock: newClock()}, "party size 1"},
		{"no account", OrganizerConfig{Policy: botparty.DefaultPolicy(), Clock: newClock(), Bots: []OrganizedBot{{Bot: f}}}, "account"},
		{"no bot", OrganizerConfig{Policy: botparty.DefaultPolicy(), Clock: newClock(), Bots: []OrganizedBot{{Account: "hz1"}}}, "account and a bot"},
		{
			"account twice", OrganizerConfig{Policy: botparty.DefaultPolicy(), Clock: newClock(), Bots: []OrganizedBot{
				{Account: "hz1", Bot: f}, {Account: "hz1", Bot: f},
			}}, "hz1 twice",
		},
		{"no bots", OrganizerConfig{Policy: botparty.DefaultPolicy(), Clock: newClock()}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOrganizer(tt.cfg)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
