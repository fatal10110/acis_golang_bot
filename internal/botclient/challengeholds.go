package botclient

import (
	"maps"
	"sync"
	"time"
)

// ChallengeHolds are the challenge holds every bot of a process shares
// (#425, docs/pvp/finding.md "Sources"): the last challenge of each player
// by any bot, which holds every bot's own challenges of that player
// challengeSameEvery, and the players whose decline or refusal holds them
// declineHold. With it one player gets at most one own challenge pop-up
// per challengeSameEvery from the whole process, not one per bot. Bots of
// other processes keep their own. Keys are player names, exact as the
// server sends them (names are unique). mu owns asked and held.
type ChallengeHolds struct {
	mu    sync.Mutex
	asked map[string]challengeMark
	held  map[string]time.Time
}

// challengeMark is a player's last challenge: the bot that sent or
// decided it, and when.
type challengeMark struct {
	bot int32
	at  time.Time
}

// NewChallengeHolds returns an empty hold table.
func NewChallengeHolds() *ChallengeHolds {
	return &ChallengeHolds{asked: make(map[string]challengeMark), held: make(map[string]time.Time)}
}

// claim reports whether bot may challenge name on its own at now, and if
// so records that challenge in one step, so two bots deciding at once
// cannot both pass: false while a decline hold on name runs or another
// bot challenged name within challengeSameEvery. bot's own mark renews
// (its per-bot clock holds its own repeats; one it decided but never sent
// must not lock it out). A nil table allows every challenge.
func (h *ChallengeHolds) claim(name string, bot int32, now time.Time) bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lapse(now)
	if now.Before(h.held[name]) {
		return false
	}
	if m, ok := h.asked[name]; ok && m.bot != bot {
		return false
	}
	h.asked[name] = challengeMark{bot: bot, at: now}
	return true
}

// sent records bot's challenge of name sent at now, own or whispered: it
// holds the other bots' own challenges of name challengeSameEvery.
func (h *ChallengeHolds) sent(name string, bot int32, now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lapse(now)
	h.asked[name] = challengeMark{bot: bot, at: now}
}

// hold holds every bot's own challenges of name declineHold from now: name
// declined or refused a bot's challenge.
func (h *ChallengeHolds) hold(name string, now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lapse(now)
	h.held[name] = now.Add(declineHold)
}

// lapse drops the marks older than challengeSameEvery and the holds run
// out at now, which bounds the table by the players challenged or held
// lately. The caller holds mu.
func (h *ChallengeHolds) lapse(now time.Time) {
	maps.DeleteFunc(h.asked, func(_ string, m challengeMark) bool { return now.Sub(m.at) >= challengeSameEvery })
	maps.DeleteFunc(h.held, func(_ string, until time.Time) bool { return !now.Before(until) })
}
