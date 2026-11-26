package botclient

import "github.com/fatal10110/acis_golang_bot/internal/botmetrics"

// ReportCache makes r collect the counts of n's cluster result cache and
// place table (botmetrics.NavCache, #496).
func (n *Nav) ReportCache(r *botmetrics.Registry) {
	h := n.Hier
	r.SetNavCache(func() botmetrics.NavCache {
		s := h.Stats()
		return botmetrics.NavCache{
			Pinned: s.Pinned, Cached: s.Cached, Computed: s.Computed, Places: s.Places,
			LRUBytes: s.LRUBytes, PlaceCells: s.PlaceCells,
		}
	})
}
