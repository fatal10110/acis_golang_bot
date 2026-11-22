package e2e

import (
	"cmp"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// The navigation the whole bots of a test process walk on (D13a): mapped
// once on first use and left mapped until the process exits, like the
// caches of a `bots swarm` run, which every bot of the process shares.
var (
	navOnce sync.Once
	runNav  *botclient.Nav
	navErr  error
)

// Nav returns the run's navigation for a whole bot. With its config missing
// the scenario skips, or fails under BOT_REQUIRE_E2E=1: a walking bot
// without fresh caches refuses to walk, so a run claimed without them would
// pass nothing it asserts about walking.
func (t *Target) Nav(tb testing.TB) *botclient.Nav {
	tb.Helper()
	if miss := t.cfg.Missing(NeedNav); len(miss) > 0 {
		msg := "e2e navigation missing: " + strings.Join(miss, "; ")
		if os.Getenv(EnvRequire) == "1" {
			tb.Fatal(msg)
		}
		tb.Skip(msg)
	}
	navOnce.Do(func() {
		f := t.cfg.Nav
		f.LRUBytes = cmp.Or(f.LRUBytes, nav.DefaultMemberCacheBytes)
		runNav, _, navErr = botclient.OpenNav(f, t.cfg.Datapack, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	})
	if navErr != nil {
		tb.Fatal(navErr)
	}
	return runNav
}
