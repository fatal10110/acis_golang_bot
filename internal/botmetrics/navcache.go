package botmetrics

// NavCache is the counts of the process's nav cluster result cache
// (docs/navigation.md, "Cache"; #496): the Locates answered from a walker's
// pins, from the LRU, by computing the cluster and from the place table,
// the bytes the LRU holds and the cells the place table holds.
type NavCache struct {
	Pinned, Cached, Computed, Places uint64
	LRUBytes                         int64
	PlaceCells                       int
}

// The keys of Snapshot.NavLocates, every one present.
const (
	LocatePinned   = "pinned"
	LocateCached   = "cached"
	LocateComputed = "computed"
	LocatePlace    = "place"
)

// SetNavCache makes Collect read the nav cache's counts from read, which
// must be safe for concurrent use. Without it they stay zero.
func (r *Registry) SetNavCache(read func() NavCache) {
	r.navCache.Store(&read)
}

// navLocates is Snapshot.NavLocates of c.
func navLocates(c NavCache) map[string]uint64 {
	return map[string]uint64{
		LocatePinned:   c.Pinned,
		LocateCached:   c.Cached,
		LocateComputed: c.Computed,
		LocatePlace:    c.Places,
	}
}

// collectNavCache sets s's nav cache fields.
func (r *Registry) collectNavCache(s *Snapshot) {
	var c NavCache
	if read := r.navCache.Load(); read != nil {
		c = (*read)()
	}
	s.NavLocates = navLocates(c)
	s.NavLRUBytes = c.LRUBytes
	s.NavPlaces = c.PlaceCells
}
