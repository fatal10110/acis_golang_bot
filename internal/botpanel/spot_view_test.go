package botpanel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestSpotWalkView checks the spot and walk fields of a bot's live view as
// GET /api/bots and GET /api/bots/{account} write it (both serve viewOf
// through Panel.list): each one set appears, each one empty is absent.
func TestSpotWalkView(t *testing.T) {
	at := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		spot, walk string
	}{
		{name: "no spot, no failed walk"},
		{name: "spot held", spot: "gludio_wolves"},
		{name: "spot held, unreachable", spot: "gludio_wolves", walk: "unreachable: no route"},
		{name: "no spot, unreachable", walk: "unreachable: blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := botregistry.Status{Live: &botclient.Status{At: at, Preset: "farmer", Rule: "farm", Spot: tt.spot, Walk: tt.walk}}
			w := httptest.NewRecorder()
			writeJSON(w, http.StatusOK, viewOf(st))
			var got struct {
				Live map[string]json.RawMessage `json:"live"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"spot": tt.spot, "walk": tt.walk} {
				raw, ok := got.Live[key]
				if want == "" {
					if ok {
						t.Errorf("%s = %s, want absent", key, raw)
					}
					continue
				}
				var s string
				if err := json.Unmarshal(raw, &s); err != nil || s != want {
					t.Errorf("%s = %s (%v), want %q", key, raw, err, want)
				}
			}
		})
	}
}
