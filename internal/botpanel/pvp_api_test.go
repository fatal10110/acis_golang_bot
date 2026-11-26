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

// TestPanelPvPSwitches checks the PvP switches' API (docs/pvp/modes.md):
// refusals of a bad body and an unknown bot; switches respawn the online
// bot with them and show in its record; "modes": null and {} (modes absent) give back
// the preset's defaults.
func TestPanelPvPSwitches(t *testing.T) {
	g := newRig(t, Config{})
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support", "class": botclient.HumanFighter.ID})
	r1 := g.next()

	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/pvp", map[string]any{"modes": map[string]any{"duels": "on"}})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/pvp", map[string]any{"modes": map[string]any{"karma": true}})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/pvp", map[string]any{"duels": true})
	g.want(http.StatusNotFound, "POST", "/api/bots/hz9/pvp", map[string]any{"modes": map[string]any{"duels": true}})

	switched := func(prev *run, body map[string]any, want *botregistry.PvPModes) *run {
		t.Helper()
		g.want(http.StatusOK, "POST", "/api/bots/hz1/pvp", body)
		endOnCancel(prev)
		next := g.next()
		var v struct {
			PvP *botregistry.PvPModes `json:"pvp"`
		}
		if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/bots/hz1", nil)), &v); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(next.PvP, want) || !reflect.DeepEqual(v.PvP, want) {
			t.Fatalf("after %v: run switches %+v, record %+v, want %+v", body, next.PvP, v.PvP, want)
		}
		return next
	}
	pk := map[string]any{"duels": true, "hunt": false, "arenas": true, "pk": true}
	r2 := switched(r1, map[string]any{"modes": pk}, &botregistry.PvPModes{Duels: true, Arenas: true, PK: true})
	r3 := switched(r2, map[string]any{"modes": nil}, nil)
	r4 := switched(r3, map[string]any{"modes": map[string]any{"hunt": true}}, &botregistry.PvPModes{Hunt: true})
	r5 := switched(r4, map[string]any{}, nil)
	g.stop(r5)
}

// TestPvPView checks a bot's live view shows its PvP view: fight back and
// the four switches as played, the kill counts, the duel tally and the
// last result, and no view before a session.
func TestPvPView(t *testing.T) {
	at := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		pvp  *botclient.PvPStatus
		want map[string]any
	}{
		{name: "no session"},
		{
			name: "pvp bot after duels",
			pvp: &botclient.PvPStatus{
				Modes:    botclient.PvPPolicy{FightFlagged: true, Duels: true, Hunt: true, Arenas: true, SupportFlagged: true},
				PvPKills: 4, PKKills: 1, Duels: botclient.DuelTally{Won: 2, Lost: 1, Tie: 3}, LastDuel: botclient.DuelLost,
			},
			want: map[string]any{
				"fight_back": true, "duels": true, "hunt": true, "arenas": true, "pk": false,
				"pvp_kills": 4.0, "pk_kills": 1.0, "duels_won": 2.0, "duels_lost": 1.0, "duels_tied": 3.0, "last_duel": "lost",
			},
		},
		{
			name: "no duel yet",
			pvp:  &botclient.PvPStatus{Modes: botclient.PvPPolicy{FightFlagged: true}},
			want: map[string]any{
				"fight_back": true, "duels": false, "hunt": false, "arenas": false, "pk": false,
				"pvp_kills": 0.0, "pk_kills": 0.0, "duels_won": 0.0, "duels_lost": 0.0, "duels_tied": 0.0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := botregistry.Status{Live: &botclient.Status{At: at, Preset: "pvp", PvP: tt.pvp}}
			w := httptest.NewRecorder()
			writeJSON(w, http.StatusOK, viewOf(st))
			var got struct {
				Live map[string]json.RawMessage `json:"live"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			raw, ok := got.Live["pvp"]
			if tt.want == nil {
				if ok {
					t.Fatalf("pvp = %s, want absent", raw)
				}
				return
			}
			var v map[string]any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(v, tt.want) {
				t.Fatalf("pvp = %v, want %v", v, tt.want)
			}
		})
	}
}
