package botclient

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestChallengeHolds checks the process-wide challenge holds (#425): one
// bot's challenge of a player holds the others' 30 minutes, a decline holds
// them all 2 hours, and lapsed entries leave the table.
func TestChallengeHolds(t *testing.T) {
	type step struct {
		op   string // claim, sent or hold
		name string
		bot  int32
		at   time.Duration
		want bool // claim only
	}
	for _, tc := range []struct {
		name  string
		steps []step
	}{
		{name: "first claim passes", steps: []step{{op: "claim", name: foeName, bot: 1, want: true}}},
		{name: "another bot waits 30 minutes", steps: []step{
			{op: "claim", name: foeName, bot: 1, want: true},
			{op: "claim", name: foeName, bot: 2, at: time.Second, want: false},
			{op: "claim", name: foeName, bot: 2, at: challengeSameEvery - time.Second, want: false},
			{op: "claim", name: foeName, bot: 2, at: challengeSameEvery, want: true},
		}},
		{name: "the same bot renews its claim", steps: []step{
			{op: "claim", name: foeName, bot: 1, want: true},
			{op: "claim", name: foeName, bot: 1, at: time.Minute, want: true},
			{op: "claim", name: foeName, bot: 2, at: challengeSameEvery, want: false},
			{op: "claim", name: foeName, bot: 2, at: challengeSameEvery + time.Minute, want: true},
		}},
		{name: "a sent whisper challenge holds the others", steps: []step{
			{op: "sent", name: foeName, bot: 1},
			{op: "claim", name: foeName, bot: 2, at: 10 * time.Minute, want: false},
			{op: "claim", name: foeName, bot: 1, at: 10 * time.Minute, want: true},
		}},
		{name: "a decline holds every bot 2 hours", steps: []step{
			{op: "claim", name: foeName, bot: 1, want: true},
			{op: "hold", name: foeName, at: time.Second},
			{op: "claim", name: foeName, bot: 1, at: time.Minute, want: false},
			{op: "claim", name: foeName, bot: 2, at: declineHold, want: false},
			{op: "claim", name: foeName, bot: 2, at: declineHold + time.Second, want: true},
		}},
		{name: "other players are not held", steps: []step{
			{op: "claim", name: foeName, bot: 1, want: true},
			{op: "hold", name: foeName, at: time.Second},
			{op: "claim", name: otherName, bot: 2, at: 2 * time.Second, want: true},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewChallengeHolds()
			for i, s := range tc.steps {
				now := t0.Add(s.at)
				switch s.op {
				case "claim":
					if got := h.claim(s.name, s.bot, now); got != s.want {
						t.Fatalf("step %d: bot %d claim of %q at %v = %v, want %v", i, s.bot, s.name, s.at, got, s.want)
					}
				case "sent":
					h.sent(s.name, s.bot, now)
				case "hold":
					h.hold(s.name, now)
				}
			}
		})
	}
	t.Run("lapsed entries leave the table", func(t *testing.T) {
		h := NewChallengeHolds()
		h.claim(foeName, 1, t0)
		h.hold(otherName, t0)
		h.sent(markedName, 2, t0.Add(declineHold))
		if len(h.asked) != 1 || len(h.held) != 0 {
			t.Fatalf("asked %v, held %v; want only %q", h.asked, h.held, markedName)
		}
	})
	t.Run("nil allows every challenge", func(t *testing.T) {
		var h *ChallengeHolds
		h.sent(foeName, 1, t0)
		h.hold(foeName, t0)
		if !h.claim(foeName, 2, t0) {
			t.Fatal("a nil table held a claim")
		}
	})
}

// TestChallengeHoldsConcurrent checks that bots claiming one player at once
// get it once: the claim checks and records under one lock.
func TestChallengeHoldsConcurrent(t *testing.T) {
	const bots = 16
	h := NewChallengeHolds()
	var wg sync.WaitGroup
	got := make([]bool, bots)
	for i := range bots {
		wg.Go(func() {
			bot := int32(i + 1)
			h.sent("p"+strconv.Itoa(i), bot, t0)
			h.hold("q"+strconv.Itoa(i), t0)
			got[i] = h.claim(foeName, bot, t0.Add(time.Second))
		})
	}
	wg.Wait()
	n := 0
	for _, ok := range got {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d bots claimed %q at once, want 1", n, foeName)
	}
}

// TestPvPChallengeHoldShared checks two bots sharing the holds with one
// player in sight of both (#425, docs/pvp/finding.md "Sources").
func TestPvPChallengeHoldShared(t *testing.T) {
	// pair is two find brains, the second a bot of its own object id,
	// sharing h.
	pair := func(t *testing.T, h *ChallengeHolds) (a, b *Brain) {
		a, b = findBrain(t, nil), findBrain(t, nil)
		b.world.Self.ObjectID = selfID + 1
		a.cfg.Challenges, b.cfg.Challenges = h, h
		return a, b
	}
	declined := func(t *testing.T, b *Brain, d time.Duration) {
		t.Helper()
		b.handleOne(t, SystemMessage{ID: msgS1DeclinedYourDuel, Text: [2]string{foeName}}, at(d))
	}
	t.Run("one challenge per 30 minutes", func(t *testing.T) {
		a, b := pair(t, NewChallengeHolds())
		if got := challenged(t, a, at(time.Second)); got != foeName {
			t.Fatalf("first bot challenged %q, want %q", got, foeName)
		}
		for _, d := range []time.Duration{2 * time.Second, 30 * time.Minute} {
			if got := challenged(t, b, at(d)); got != "" {
				t.Fatalf("second bot challenged %q at %v, within 30 minutes of the first", got, d)
			}
		}
		if got := challenged(t, b, at(30*time.Minute+time.Second)); got != foeName {
			t.Fatalf("second bot challenged %q after 30 minutes, want %q", got, foeName)
		}
	})
	t.Run("a decline holds every bot 2 hours", func(t *testing.T) {
		a, b := pair(t, NewChallengeHolds())
		if got := challenged(t, a, at(time.Second)); got != foeName {
			t.Fatalf("first bot challenged %q, want %q", got, foeName)
		}
		declined(t, a, 2*time.Second)
		for _, d := range []time.Duration{31 * time.Minute, 2*time.Hour + time.Second} {
			if got := challenged(t, b, at(d)); got != "" {
				t.Fatalf("second bot challenged %q at %v, inside the first's DeclineHold", got, d)
			}
		}
		if got := challenged(t, b, at(2*time.Hour+3*time.Second)); got != foeName {
			t.Fatalf("second bot challenged %q after the hold, want %q", got, foeName)
		}
	})
	t.Run("a whisper is never held", func(t *testing.T) {
		a, b := pair(t, NewChallengeHolds())
		if got := challenged(t, a, at(time.Second)); got != foeName {
			t.Fatalf("first bot challenged %q, want %q", got, foeName)
		}
		declined(t, a, 2*time.Second)
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(10*time.Minute))
		if got := challenged(t, b, b.pvp.find.whisper.due); got != foeName {
			t.Fatalf("second bot challenged %q on the whisper, want %q", got, foeName)
		}
	})
	t.Run("a whispered challenge holds the other bot", func(t *testing.T) {
		a, b := pair(t, NewChallengeHolds())
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
		due := b.pvp.find.whisper.due
		if got := challenged(t, b, due); got != foeName {
			t.Fatalf("second bot challenged %q on the whisper, want %q", got, foeName)
		}
		if got := challenged(t, a, due.Add(time.Second)); got != "" {
			t.Fatalf("first bot challenged %q within 30 minutes of the whispered one", got)
		}
		if got := challenged(t, a, due.Add(challengeSameEvery)); got != foeName {
			t.Fatalf("first bot challenged %q after 30 minutes, want %q", got, foeName)
		}
	})
	t.Run("no table holds per bot", func(t *testing.T) {
		a, b := pair(t, nil)
		for _, br := range []*Brain{a, b} {
			if got := challenged(t, br, at(time.Second)); got != foeName {
				t.Fatalf("bot %d challenged %q, want %q", br.world.Self.ObjectID, got, foeName)
			}
		}
		declined(t, a, 2*time.Second)
		if got := challenged(t, a, at(31*time.Minute)); got != "" {
			t.Fatalf("first bot challenged %q inside its DeclineHold", got)
		}
	})
}
