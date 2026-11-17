package e2e

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// defaultSoak outlasts the server's 15 minute idle timeout of a silent
// connection (session.go clientReadIdleTimeout).
const defaultSoak = 16 * time.Minute

// TestLiveSoak keeps one bot in the world past the idle timeout: it is
// never dropped nor reconnects, its sends stay far inside the server's
// flood limit (80 frames a second averaged), and its metrics and the
// process's goroutines stay flat (#9, #14). BOT_E2E_SOAK sets the length.
func TestLiveSoak(t *testing.T) {
	tg := Live(t)
	soak := defaultSoak
	if v := os.Getenv("BOT_E2E_SOAK"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("BOT_E2E_SOAK: %v", err)
		}
		soak = d
	}
	ctx := scenarioCtx(t, soak+5*time.Minute)
	reg := botmetrics.NewRegistry()
	account, _ := tg.Account("s", 1)
	b := runBot(t, ctx, tg, account, false, reg.NewBot())
	waitFor(t, ctx, "the bot online", online(b))
	start := reg.Collect(time.Now())
	goroutines := runtime.NumGoroutine()

	timer := time.NewTimer(soak)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}

	end := reg.Collect(time.Now())
	if st := b.Status(); st.State != botclient.StateOnline || st.Reconnects != 0 {
		t.Fatalf("after %v: state %v, %d reconnects, error %v; want online all along", soak, st.State, st.Reconnects, st.Err)
	}
	if end.Reconnects != start.Reconnects || end.Aborts != start.Aborts || end.BreakerOpen {
		t.Fatalf("metrics moved: reconnects %d->%d, aborts %d->%d, breaker open %v", start.Reconnects, end.Reconnects, start.Aborts, end.Aborts, end.BreakerOpen)
	}
	sent := end.Sent - start.Sent
	if rate := float64(sent) / soak.Seconds(); rate > 5 {
		t.Fatalf("sent %d frames in %v (%.2f/s), past the pacer's 5/s", sent, soak, rate)
	}
	if n := runtime.NumGoroutine(); n > goroutines+2 {
		t.Fatalf("%d goroutines after the soak, %d before", n, goroutines)
	}
	t.Logf("soak %v: %d frames sent, %d events, queue depth %+v", soak, sent, end.Events-start.Events, end.QueueDepth)
}
