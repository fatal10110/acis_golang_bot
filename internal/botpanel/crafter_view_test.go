package botpanel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestCrafterView checks the crafter's status fields of a bot's live view
// as the API writes them: absent for a bot that plays no crafter, every
// field for one, the last cycle and the breaker only once they happened.
func TestCrafterView(t *testing.T) {
	at := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		crafter *botclient.CrafterStatus
		want    map[string]any
	}{
		{name: "no crafter"},
		{
			name:    "idle before any cycle",
			crafter: &botclient.CrafterStatus{CreateItem: 1, Phase: "idle", NextCheck: at},
			want: map[string]any{
				"create_item": 1.0, "recipes": 0.0, "book_known": false, "orders": 0.0, "claimed": 0.0,
				"phase": "idle", "chains": 0.0, "chains_done": 0.0, "next_check": "2026-11-22T10:00:00Z",
			},
		},
		{
			name: "making, a breaker tripped before",
			crafter: &botclient.CrafterStatus{
				CreateItem: 3, Recipes: 12, BookKnown: true, Orders: 2, Claimed: 1, Phase: "make", Chains: 3, ChainsDone: 1,
				Last:         botclient.CraftCycleEnd{At: at, Outcome: botclient.CraftCycleTripped},
				BreakerUntil: at.Add(30 * time.Minute),
			},
			want: map[string]any{
				"create_item": 3.0, "recipes": 12.0, "book_known": true, "orders": 2.0, "claimed": 1.0,
				"phase": "make", "chains": 3.0, "chains_done": 1.0,
				"last_cycle": "breaker tripped", "last_cycle_at": "2026-11-22T10:00:00Z", "breaker_until": "2026-11-22T10:30:00Z",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := botregistry.Status{Live: &botclient.Status{At: at, Preset: "crafter", Crafter: tt.crafter}}
			w := httptest.NewRecorder()
			writeJSON(w, http.StatusOK, viewOf(st))
			var got struct {
				Live map[string]json.RawMessage `json:"live"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			raw, ok := got.Live["crafter"]
			if tt.want == nil {
				if ok {
					t.Fatalf("crafter = %s, want absent", raw)
				}
				return
			}
			var c map[string]any
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c, tt.want) {
				t.Fatalf("crafter = %v, want %v", c, tt.want)
			}
		})
	}
}
