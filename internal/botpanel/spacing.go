package botpanel

import (
	"fmt"
	"net/http"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// setLoginSpacing sets the login gate's least time between two dial starts,
// in milliseconds, and keeps it for the next start. The gate refuses one
// under its floor (the accept filter's limit on a spaced gate) with 400.
func (p *Panel) setLoginSpacing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MS *int64 `json:"ms"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	// Bounded before the conversion, so a huge value cannot wrap into range.
	if req.MS == nil || *req.MS < 0 || *req.MS > botclient.MaxLoginSpacing.Milliseconds() {
		writeErr(w, badRequest(fmt.Errorf("login spacing: want ms from %d to %d",
			p.cfg.Gate.MinSpacing().Milliseconds(), botclient.MaxLoginSpacing.Milliseconds())))
		return
	}
	if err := p.cfg.Supervisor.SetLoginSpacing(r.Context(), time.Duration(*req.MS)*time.Millisecond); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, okAnswer)
}
