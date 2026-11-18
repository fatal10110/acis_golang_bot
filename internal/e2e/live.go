package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// The run's target: one per test process, so every scenario of the process
// shares its login gate, credential breaker, GM pool and bound on scenarios
// open.
var (
	runOnce   sync.Once
	runTarget *Target
	runErr    error
)

// Live opens a scenario of needs on the run (Target.Scenario), waiting while
// MaxScenarios are open, and closes it when the test ends. With its config
// missing the scenario skips, or fails under BOT_REQUIRE_E2E=1. The config
// comes from the environment, its files defaulting to DefaultDir's. A
// scenario that runs in parallel calls t.Parallel before Live: a paused
// test holding a scenario would keep the sequential ones waiting.
func Live(t testing.TB, needs ...Need) *Target {
	t.Helper()
	cfg := ConfigFromEnv(os.Getenv, DefaultDir())
	if miss := cfg.Missing(needs...); len(miss) > 0 {
		msg := "e2e config missing: " + strings.Join(miss, "; ")
		if os.Getenv(EnvRequire) == "1" {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	runOnce.Do(func() {
		var panel string
		if base, err := os.UserConfigDir(); err == nil {
			if panel, runErr = botregistry.PanelPrefix(filepath.Join(base, botregistry.DefaultDirName)); runErr != nil {
				return
			}
		}
		runTarget, runErr = NewTarget(cfg, panel)
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	tg, done, err := runTarget.Scenario(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(done)
	return tg
}
