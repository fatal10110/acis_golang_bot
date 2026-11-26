package botpanel

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestPanelLoginSpacing(t *testing.T) {
	g := newRig(t, Config{})
	dashboard := func() dashboardView {
		t.Helper()
		var d dashboardView
		if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/dashboard", nil)), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	floor, top := g.gate.MinSpacing().Milliseconds(), botclient.MaxLoginSpacing.Milliseconds()
	if d := dashboard(); d.LoginSpacing != floor || d.LoginSpacingMin != floor || d.LoginSpacingMax != top {
		t.Fatalf("dashboard spacing %d in [%d, %d], want the floor %d in [%d, %d]",
			d.LoginSpacing, d.LoginSpacingMin, d.LoginSpacingMax, floor, floor, top)
	}

	tests := []struct {
		name string
		body any
		want int
	}{
		{"no ms", map[string]any{}, http.StatusBadRequest},
		{"not a number", map[string]any{"ms": "5000"}, http.StatusBadRequest},
		{"unknown field", map[string]any{"ms": 5000, "x": 1}, http.StatusBadRequest},
		{"negative", map[string]any{"ms": -1}, http.StatusBadRequest},
		{"under the floor", map[string]any{"ms": floor - 1}, http.StatusBadRequest},
		{"zero", map[string]any{"ms": 0}, http.StatusBadRequest},
		{"over the cap", map[string]any{"ms": top + 1}, http.StatusBadRequest},
		// Wraps into range once multiplied to nanoseconds unless bounded first.
		{"wraps", map[string]any{"ms": int64(1<<63-1)/1000 + 5000}, http.StatusBadRequest},
		{"raised", map[string]any{"ms": 6000}, http.StatusOK},
	}
	for _, tt := range tests {
		g.want(tt.want, "POST", "/api/login-spacing", tt.body)
		if tt.want != http.StatusOK {
			if got := g.gate.Spacing(); got != g.gate.MinSpacing() {
				t.Fatalf("%s: gate spacing %s after a refusal, want the floor", tt.name, got)
			}
		}
	}
	if got := g.gate.Spacing(); got != 6*time.Second {
		t.Fatalf("gate spacing %s, want 6s", got)
	}
	if d := dashboard(); d.LoginSpacing != 6000 {
		t.Fatalf("dashboard spacing %d ms, want 6000", d.LoginSpacing)
	}
	g.stop()
	// Kept in the registry for the next start; Run has returned, so the
	// registry is the test's again.
	if got := g.reg.LoginSpacing(); got != 6*time.Second {
		t.Fatalf("registry spacing %s, want 6s", got)
	}
}
