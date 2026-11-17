package e2e

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// runBot runs a whole Bot of account on the run's gate and a place of its
// own until the test ends, which logs it out and joins it.
func runBot(t *testing.T, ctx context.Context, tg *Target, account string, persistent bool, metrics *botmetrics.Bot) *botclient.Bot {
	t.Helper()
	release, err := tg.Hold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := tg.SessionConfig(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cfg.Persistent = persistent
	brain := botclient.BrainConfig{Persona: botclient.DefaultPersona(), Metrics: metrics}
	b, err := botclient.NewBot(cfg, account, tg.Password(account), botclient.BotConfig{Character: character(t, account, nil), Brain: brain}, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	if err != nil {
		release()
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Logf("bot %s stopped: %v", account, err)
		}
		release()
	})
	return b
}

func online(b *botclient.Bot) func() bool {
	return func() bool { return b.Status().State == botclient.StateOnline }
}

// TestLiveReconnectAfterKick kicks a persistent bot with the GM's //kick:
// the server sends ServerClose and drops it, and the bot logs back in once
// the login server lets the account go (#12).
func TestLiveReconnectAfterKick(t *testing.T) {
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 5*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("r", 1)
	b := runBot(t, ctx, tg, account, true, nil)
	waitFor(t, ctx, "the bot online", online(b))
	name := b.Status().Character
	tg.Own(name, 0)
	if err := gm.Command(ctx, "kick "+name); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ctx, "the bot back online", func() bool {
		s := b.Status()
		return s.Reconnects >= 1 && s.State == botclient.StateOnline
	})
	t.Logf("%s reconnected after the kick (%d reconnects)", name, b.Status().Reconnects)
}
