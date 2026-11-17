package botpanel

import (
	"encoding/json"
	"net/http"
	"time"
)

// update is what an event stream sends: every bot and the dashboard.
type update struct {
	Bots      []botView     `json:"bots"`
	Dashboard dashboardView `json:"dashboard"`
}

// writeWait bounds each write of an event stream: a page that stops reading
// ends its stream.
const writeWait = 10 * time.Second

// events streams an update at once and then every Every, as server-sent
// events, until the page goes away or the panel stops.
func (p *Panel) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	tick := time.NewTicker(p.cfg.Every)
	defer tick.Stop()
	for {
		l, err := p.list(r)
		if err != nil {
			return
		}
		b, err := json.Marshal(update{Bots: l, Dashboard: p.dashboardOf(l)})
		if err != nil {
			return
		}
		_ = rc.SetWriteDeadline(time.Now().Add(writeWait))
		if _, err := w.Write(append(append([]byte("event: update\ndata: "), b...), '\n', '\n')); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
		select {
		case <-tick.C:
		case <-r.Context().Done():
			return
		case <-p.quit:
			return
		}
	}
}
