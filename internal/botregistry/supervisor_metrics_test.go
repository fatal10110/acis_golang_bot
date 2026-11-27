package botregistry

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestSupervisorDropsDeletedSlots: a bot dropped for an account holding
// characters, or deleted, leaves the metrics with its counters folded into
// the retained total and its slot gone; a stopped bot keeps its slot, and a
// new bot takes a fresh one. Without metrics the same steps run.
func TestSupervisorDropsDeletedSlots(t *testing.T) {
	for _, tt := range []struct {
		name    string
		metrics bool
	}{{"metrics", true}, {"no metrics", false}} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []func(*Config)
			if !tt.metrics {
				opts = append(opts, func(c *Config) { c.Metrics = nil })
			}
			h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 3, opts...)
			// slots checks the live slots and the sent sum.
			slots := func(what string, bots int, sent uint64) {
				t.Helper()
				if !tt.metrics {
					return
				}
				if s := h.metrics.Collect(t0); s.Bots != bots || s.Sent != sent {
					t.Fatalf("%s: %d slots, %d sent, want %d and %d", what, s.Bots, s.Sent, bots, sent)
				}
			}

			_, r1 := h.create(t, "town", nil)
			r1.Metrics.Sent()
			r1.Metrics.Sent()
			r1.end <- botclient.ErrAccountHasCharacters
			r2 := h.next(t)
			if r2.Account != "hz2" || (tt.metrics && (r2.Metrics == nil || r2.Metrics == r1.Metrics)) || (!tt.metrics && r2.Metrics != nil) {
				t.Fatalf("replacement run %s slot %p (old %p)", r2.Account, r2.Metrics, r1.Metrics)
			}
			slots("after the replacement", 1, 2)

			r2.Metrics.Sent()
			if err := h.sup.Despawn(t.Context(), "hz2"); err != nil {
				t.Fatal(err)
			}
			<-r2.ctx.Done()
			r2.end <- nil
			h.settle(t, "hz2", RunStopping)
			slots("stopped", 1, 3)
			if err := h.sup.Delete(t.Context(), "hz2"); err != nil {
				t.Fatal(err)
			}
			slots("after the delete", 0, 3)

			b3, r3 := h.create(t, "town", nil)
			if b3.Account != "hz3" || (tt.metrics && (r3.Metrics == nil || r3.Metrics == r1.Metrics || r3.Metrics == r2.Metrics)) {
				t.Fatalf("new bot %s slot %p (old %p, %p)", b3.Account, r3.Metrics, r1.Metrics, r2.Metrics)
			}
			slots("after a new bot", 1, 3)
			h.stop(t, r3)
		})
	}
}
