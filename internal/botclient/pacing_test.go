package botclient

import (
	"math/rand/v2"
	"testing"
	"time"
)

// Server limits the pacer stays under (clientstats.go, config defaults).
const (
	serverFramesPerSecond = 160
	serverFramesPer5s     = 80 * 5
	serverSocialGap       = 2 * time.Second
)

// maxInWindow is the most of the sorted times that fall in any window of
// length w.
func maxInWindow(times []time.Time, w time.Duration) int {
	best, j := 0, 0
	for i := range times {
		for times[i].Sub(times[j]) >= w {
			j++
		}
		best = max(best, i-j+1)
	}
	return best
}

func TestPacerGates(t *testing.T) {
	type try struct {
		at   time.Duration
		g    gate
		want bool
	}
	tests := []struct {
		name  string
		tries []try
	}{
		{"emote waits emoteGap", []try{{0, gateEmote, true}, {emoteGap - time.Millisecond, gateEmote, false}, {emoteGap, gateEmote, true}}},
		{"chat waits chatGap", []try{{0, gateChat, true}, {chatGap - time.Millisecond, gateChat, false}, {chatGap, gateChat, true}}},
		{"multisell waits multiSellGap", []try{{0, gateMultiSell, true}, {multiSellGap - time.Millisecond, gateMultiSell, false}, {multiSellGap, gateMultiSell, true}}},
		{"gates are separate", []try{{0, gateEmote, true}, {time.Millisecond, gateChat, true}, {2 * time.Millisecond, gateNone, true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newPacer(t0)
			for _, tr := range tc.tries {
				now := at(tr.at)
				got := p.allow(tr.g, now)
				if got != tr.want {
					t.Fatalf("allow(gate %d) at %v = %v, want %v", tr.g, tr.at, got, tr.want)
				}
				if got {
					p.sent(tr.g, now)
				}
			}
		})
	}
}

func TestPacerBurstThenRate(t *testing.T) {
	p := newPacer(t0)
	for i := range frameBurst {
		if !p.allow(gateNone, t0) {
			t.Fatalf("frame %d of the burst refused", i)
		}
		p.sent(gateNone, t0)
	}
	if p.allow(gateNone, t0) {
		t.Fatal("frame past the burst allowed")
	}
	step := time.Second / frameRate
	if p.allow(gateNone, t0.Add(step-time.Millisecond)) {
		t.Fatal("frame allowed before a token refilled")
	}
	if !p.allow(gateNone, t0.Add(step)) {
		t.Fatal("frame refused after a token refilled")
	}
}

// TestPacerSoak offers a frame of a random gate every millisecond for two
// simulated hours and checks what went out against the server's limits.
func TestPacerSoak(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 9))
	p := newPacer(t0)
	var frames []time.Time
	var last [gateCount]time.Time
	for ms := range int64(2 * time.Hour / time.Millisecond) {
		now := t0.Add(time.Duration(ms) * time.Millisecond)
		g := gate(rng.IntN(int(gateCount)))
		if !p.allow(g, now) {
			continue
		}
		p.sent(g, now)
		frames = append(frames, now)
		if g == gateEmote && !last[g].IsZero() && now.Sub(last[g]) < serverSocialGap {
			t.Fatalf("emotes %v apart, server gate is %v", now.Sub(last[g]), serverSocialGap)
		}
		if g == gateChat && !last[g].IsZero() && now.Sub(last[g]) < chatGap {
			t.Fatalf("chat lines %v apart, want at least %v", now.Sub(last[g]), chatGap)
		}
		last[g] = now
	}
	assertUnderServerLimits(t, frames)
}

func assertUnderServerLimits(t *testing.T, frames []time.Time) {
	t.Helper()
	if n := maxInWindow(frames, time.Second); n > frameBurst+frameRate || n >= serverFramesPerSecond {
		t.Errorf("%d frames in one second, want at most %d (server floods past %d)", n, frameBurst+frameRate, serverFramesPerSecond)
	}
	if n := maxInWindow(frames, 5*time.Second); n > frameBurst+5*frameRate || n >= serverFramesPer5s {
		t.Errorf("%d frames in 5 s, want at most %d (server floods past %d)", n, frameBurst+5*frameRate, serverFramesPer5s)
	}
}
