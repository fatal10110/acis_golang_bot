package nav

import (
	"errors"
	"sync"
	"sync/atomic"
)

// CacheStats counts how the hierarchy answered its Locates since it was
// built, and what its cluster result LRU and place table hold now
// (docs/navigation.md, "Cache").
type CacheStats struct {
	// Pinned, Cached and Computed count the Locates answered from the
	// caller's pins, from the LRU and by computing the cluster's result;
	// Places counts the LocatePlace calls the place table answered.
	Pinned, Cached, Computed, Places uint64
	// LRUBytes and LRUResults are the LRU's accounted bytes and results.
	LRUBytes   int64
	LRUResults int
	// PlaceCells is the number of cells the place table holds.
	PlaceCells int
}

// cacheCounters are the Locate counts of CacheStats.
type cacheCounters struct {
	pinned, cached, computed, places atomic.Uint64
}

// Stats returns the hierarchy's cache counts.
func (h *Hierarchy) Stats() CacheStats {
	s := CacheStats{
		Pinned:   h.counts.pinned.Load(),
		Cached:   h.counts.cached.Load(),
		Computed: h.counts.computed.Load(),
		Places:   h.counts.places.Load(),
	}
	s.LRUBytes, s.LRUResults = h.cache.size()
	s.PlaceCells = h.places.len()
	return s
}

// LocatePlace is Locate for a static place, a cell the callers look up again
// and again whatever bot asks, such as a gatekeeper or a spot's centre: the
// first answer is kept for the life of the process in the place table, so
// the place's cluster is never recomputed for it once the LRU dropped it. A
// place in an abstract SCC keeps only its SCC, a few bytes shared by every
// place of the same cluster and SCC, never the cluster's labels: its Member
// works with SameSCC, Reachable and Distance as Locate's does, but never as
// a pin (Pins skips it). A cell off the navigation is kept as such; a cell
// in no abstract SCC is not kept and is located again each time. Labels
// never depend on the door state, so a kept answer never goes stale.
//
// The table never evicts: its callers bound it by passing static places
// only, never where a bot happens to stand.
func (h *Hierarchy) LocatePlace(c Cell) (Member, error) {
	if a, ok := h.places.get(c); ok {
		h.counts.places.Add(1)
		if a.noNav {
			return Member{}, ErrNoNav
		}
		return a.m, nil
	}
	m, err := h.Locate(c, nil)
	switch {
	case errors.Is(err, ErrNoNav):
		h.places.put(c, Member{}, true)
	case err != nil:
	case m.Kind() == InSCC:
		m = h.places.put(c, m, false)
	}
	return m, err
}

// placeTable is the hierarchy's LocatePlace answers. mu owns both maps.
type placeTable struct {
	mu    sync.Mutex
	cells map[Cell]placeAnswer
	// sccs holds one label-free result per cluster and abstract SCC, which
	// every kept place of that cluster and SCC shares.
	sccs map[[2]int32]*clusterResult
}

// placeAnswer is a kept LocatePlace answer: the place's Member, or off the
// navigation.
type placeAnswer struct {
	m     Member
	noNav bool
}

func (t *placeTable) get(c Cell) (placeAnswer, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.cells[c]
	return a, ok
}

// put keeps c's answer: off the navigation when noNav, else m, which must be
// InSCC, as a Member of its cluster's shared label-free result. It returns
// the Member kept.
func (t *placeTable) put(c Cell, m Member, noNav bool) Member {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cells == nil {
		t.cells = make(map[Cell]placeAnswer)
		t.sccs = make(map[[2]int32]*clusterResult)
	}
	if a, ok := t.cells[c]; ok {
		return a.m
	}
	if noNav {
		t.cells[c] = placeAnswer{noNav: true}
		return Member{}
	}
	key := [2]int32{m.res.cluster, m.SCC()}
	r := t.sccs[key]
	if r == nil {
		r = &clusterResult{
			cluster: m.res.cluster, x0: m.res.x0, y0: m.res.y0, size: m.res.size,
			entries: []entry{{kind: InSCC, scc: m.SCC()}},
			place:   true,
		}
		t.sccs[key] = r
	}
	kept := Member{cell: c, res: r}
	t.cells[c] = placeAnswer{m: kept}
	return kept
}

func (t *placeTable) len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.cells)
}
