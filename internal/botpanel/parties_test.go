package botpanel

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestPanelParties checks the party endpoints through the supervisor: the
// view, the refusals of a hand-formed party, dissolve, party mode and the
// auto switch; no secret in any response.
func TestPanelParties(t *testing.T) {
	g := newRig(t, Config{})
	parties := func() partiesView {
		t.Helper()
		var v partiesView
		if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/parties", nil)), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := parties(); !v.Auto || v.Parties == nil || len(v.Parties) != 0 || v.Free == nil {
		t.Fatalf("fresh parties %+v, want auto on and empty lists", v)
	}
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r1 := g.next()
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r2 := g.next()

	// The bots never reach the world: offline, so refused.
	g.want(http.StatusConflict, "POST", "/api/parties", map[string]any{"leader": "hz1", "members": []string{"hz2"}, "pin": true})
	g.want(http.StatusBadRequest, "POST", "/api/parties", map[string]any{"leader": "hz1", "members": []string{"hz1"}})
	g.want(http.StatusBadRequest, "POST", "/api/parties", map[string]any{"leader": "hz1"})
	g.want(http.StatusNotFound, "POST", "/api/parties", map[string]any{"leader": "hz9", "members": []string{"hz2"}})
	g.want(http.StatusBadRequest, "POST", "/api/parties", map[string]any{"leader": "hz1", "members": []string{"hz2"}, "secret": "x"})

	g.want(http.StatusNotFound, "DELETE", "/api/parties/5", nil)
	g.want(http.StatusBadRequest, "DELETE", "/api/parties/x", nil)

	g.want(http.StatusOK, "POST", "/api/bots/hz2/party", map[string]any{"mode": "off"})
	if v := g.bots()["hz2"]; v.PartyMode != "off" {
		t.Fatalf("hz2 party mode %q, want off", v.PartyMode)
	}
	g.want(http.StatusOK, "POST", "/api/bots/hz2/party", map[string]any{"mode": "auto"})
	if v := g.bots()["hz2"]; v.PartyMode != "" {
		t.Fatalf("hz2 party mode %q, want auto", v.PartyMode)
	}
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz2/party", map[string]any{"mode": "pinned"})
	g.want(http.StatusNotFound, "POST", "/api/bots/hz9/party", map[string]any{"mode": "off"})

	g.want(http.StatusBadRequest, "POST", "/api/parties/auto", map[string]any{})
	g.want(http.StatusOK, "POST", "/api/parties/auto", map[string]any{"on": false})
	if v := parties(); v.Auto {
		t.Fatal("auto parties still on")
	}
	// An auto party is refused while auto parties are off.
	g.want(http.StatusConflict, "POST", "/api/parties", map[string]any{"leader": "hz1", "members": []string{"hz2"}})
	g.want(http.StatusOK, "POST", "/api/parties/auto", map[string]any{"on": true})
	if v := parties(); !v.Auto {
		t.Fatal("auto parties still off")
	}
	g.noSecret()
	g.stop(r1, r2)
}
