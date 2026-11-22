package e2e

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveWhisperLookup commands a bot through the authority's access level
// check (botclient.LevelLookup) instead of the -commanders allowlist. The
// run has no database, so a counting lookup stands in for it and answers the
// GM's level (7, an isGM level of accessLevels.xml) and the user level for
// everyone else. A player who is no GM whispers "invite me" three times: the
// bot never whispers back, and only the first whisper is checked (the
// negative cache, deniedFor). The GM's "invite me" is checked once and
// whispered back. The lookup count is the bot's own, never the database's
// (#19).
func TestLiveWhisperLookup(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 5*time.Minute)
	gm := enterGM(t, ctx, tg)
	levels := &countingLevels{gm: gm.Self.ObjectID, checks: make(map[int32]int)}
	account, _ := tg.Account("wl", 1)
	b := runBotWith(t, ctx, tg, account, botSetup{persona: partyPersona, brain: checkedBy(t, ctx, levels)})
	name := botOnline(t, ctx, tg, b)
	acc, _ := tg.Account("wl", 2)
	other := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)

	echo := func(ev botclient.Event) bool {
		s, ok := ev.(botclient.CreatureSay)
		return ok && s.Type == 2 && strings.EqualFold(s.Name, name) && s.Text == "invite me"
	}
	for i := range 3 {
		tell(t, ctx, other, name, "invite me")
		answered := false
		if err := other.Watch(ctx, 3*time.Second, func(ev botclient.Event) {
			answered = answered || echo(ev)
		}); err != nil {
			t.Fatal(err)
		}
		if answered {
			t.Fatalf("%s whispered \"invite me\" back to %s, who is no GM (whisper %d)", name, other.Self.Name, i+1)
		}
	}
	if n := levels.count(other.Self.ObjectID); n != 1 {
		t.Fatalf("%d access level checks for %s's three whispers, want 1 (the negative cache)", n, other.Self.Name)
	}

	tell(t, ctx, gm.Client, name, "invite me")
	if _, err := gm.Await(ctx, 30*time.Second, echo); err != nil {
		t.Fatalf("%s whispers \"invite me\" back to the GM its check granted: %v", name, err)
	}
	if n := levels.count(gm.Self.ObjectID); n != 1 {
		t.Fatalf("%d access level checks for the GM's whisper, want 1", n)
	}
}

// countingLevels is the scenario's stand-in for the game database's access
// level check: the GM's character reads as level 7, every other as level 0.
// mu owns checks, the number of checks per character; the authority's
// workers call AccessLevel.
type countingLevels struct {
	gm int32

	mu     sync.Mutex
	checks map[int32]int
}

func (l *countingLevels) AccessLevel(_ context.Context, objectID int32) (int32, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.checks[objectID]++
	if objectID == l.gm {
		return 7, false, nil
	}
	return 0, false, nil
}

func (l *countingLevels) count(objectID int32) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.checks[objectID]
}

// checkedBy has the bot's whispered commands checked by lookup, as the
// panel's database mode does (cmd/bots/gm.go): the authority's workers run
// until the scenario ends, joined in cleanup after the bot is stopped (a
// cleanup registered earlier runs later).
func checkedBy(t *testing.T, ctx context.Context, lookup botclient.LevelLookup) func(*botclient.BrainConfig) {
	t.Helper()
	a := botclient.NewAuthority(botclient.AuthorityConfig{Lookup: lookup, Clock: botclient.SystemClock{}})
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { a.Run(runCtx) })
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	return func(cfg *botclient.BrainConfig) {
		cfg.Authority = a
		cfg.Persona = a.Hooks(cfg.Persona)
	}
}
