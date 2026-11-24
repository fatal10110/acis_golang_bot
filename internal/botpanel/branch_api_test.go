package botpanel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// TestPanelBranchSetAndClear checks the branch pin's API (D22, #331):
// refusals of a branch of another base class, a bad body and an unknown
// bot; a pin respawns the online bot with it and shows in its record; 0 and
// an empty body clear it.
func TestPanelBranchSetAndClear(t *testing.T) {
	g := newRig(t, Config{})
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support", "class": botclient.HumanFighter.ID})
	r1 := g.next()

	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/branch", map[string]any{"branch": 19})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/branch", map[string]any{"branch": 2})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/branch", map[string]any{"branch": "Rogue"})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/branch", map[string]any{"pin": 7})
	g.want(http.StatusNotFound, "POST", "/api/bots/hz9/branch", map[string]any{"branch": 7})

	pinned := func(prev *run, body map[string]any, want int32) *run {
		t.Helper()
		g.want(http.StatusOK, "POST", "/api/bots/hz1/branch", body)
		endOnCancel(prev)
		next := g.next()
		var v map[string]any
		if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/bots/hz1", nil)), &v); err != nil {
			t.Fatal(err)
		}
		got, has := v["branch"]
		if next.Branch != want || want == 0 && has || want != 0 && got != float64(want) {
			t.Fatalf("after %v: run branch %d, record branch %v, want %d", body, next.Branch, got, want)
		}
		return next
	}
	r2 := pinned(r1, map[string]any{"branch": 7}, 7)
	r3 := pinned(r2, map[string]any{"branch": 0}, 0)
	r4 := pinned(r3, map[string]any{"branch": 4}, 4)
	r5 := pinned(r4, map[string]any{}, 0)
	g.stop(r5)
}

// TestBuildView checks a bot's live view shows the build it plays: role,
// branch by id and name and the kit's skills, empty lists as [], and no
// build for a bot that plays none.
func TestBuildView(t *testing.T) {
	at := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		build *botclient.BuildStatus
		want  map[string]any
	}{
		{name: "no build"},
		{
			name:  "damage human fighter heading for Rogue",
			build: &botclient.BuildStatus{Role: botclient.RoleDamage, Branch: 7, Skills: []int32{3, 56}, Rotation: []int32{3}},
			want: map[string]any{
				"role": "damage", "branch": 7.0, "branch_name": "Rogue", "skills": []any{3.0, 56.0}, "rotation": []any{3.0},
				"buffs": []any{}, "heals": []any{}, "toggles": []any{},
			},
		},
		{
			name:  "no branch",
			build: &botclient.BuildStatus{Role: botclient.RoleSupport, Buffs: []int32{1068}, Heals: []int32{1011}, Toggles: []int32{312}},
			want: map[string]any{
				"role": "support", "branch": 0.0, "skills": []any{}, "rotation": []any{},
				"buffs": []any{1068.0}, "heals": []any{1011.0}, "toggles": []any{312.0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := botregistry.Status{Live: &botclient.Status{At: at, Preset: "farmer", Build: tt.build}}
			w := httptest.NewRecorder()
			writeJSON(w, http.StatusOK, viewOf(st))
			var got struct {
				Live map[string]json.RawMessage `json:"live"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			raw, ok := got.Live["build"]
			if tt.want == nil {
				if ok {
					t.Fatalf("build = %s, want absent", raw)
				}
				return
			}
			var b map[string]any
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b, tt.want) {
				t.Fatalf("build = %v, want %v", b, tt.want)
			}
		})
	}
}

// TestClassBranches checks the class list names the branches a base class
// may be pinned to, and none for a profession.
func TestClassBranches(t *testing.T) {
	want := map[int32][]int32{0: {1, 4, 7}, 10: {11, 15}, 49: {50}, 53: {54, 56}, 4: nil, 51: nil}
	for _, c := range classList() {
		if w, ok := want[c.ID]; ok && !slices.Equal(c.Branches, w) {
			t.Errorf("class %d (%s) branches %v, want %v", c.ID, c.Name, c.Branches, w)
		}
	}
}
