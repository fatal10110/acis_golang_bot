package botclient

import (
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// spotPoints keeps spotPoint's answers for bots standing in an abstract SCC
// (#496): the answer depends on the spot and that SCC only, so every bot of
// the process asks the hierarchy once per spot and SCC, not at every spot
// pick and trip plan. It holds a few entries per spot, one for each SCC its
// bots stood in when they asked, each a place (Hierarchy.LocatePlace), never
// a cluster's labels. mu owns hier and kept; hier is the hierarchy of the
// first answer, and a brain on another one (a test's) bypasses the table.
type spotPoints struct {
	mu   sync.Mutex
	hier *nav.Hierarchy
	kept map[spotPointKey]spotPointHit
}

type spotPointKey struct {
	spot int
	scc  int32
}

// spotPointHit is a kept spotPoint answer.
type spotPointHit struct {
	p  navPoint
	ok bool
}

func (s *spotPoints) get(h *nav.Hierarchy, k spotPointKey) (spotPointHit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hier != h {
		return spotPointHit{}, false
	}
	hit, ok := s.kept[k]
	return hit, ok
}

func (s *spotPoints) put(h *nav.Hierarchy, k spotPointKey, hit spotPointHit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hier == nil {
		s.hier, s.kept = h, make(map[spotPointKey]spotPointHit)
	}
	if s.hier == h {
		s.kept[k] = hit
	}
}

// spotPoint is spot i's centre on walkable ground inside its territory and
// reachable from from: the centre itself, else the walkable cell nearest it
// within spotSnap that is inside the territory and reachable; false when
// none is. From an abstract SCC the registry's spotPoints answers after the
// first time.
func (b *Brain) spotPoint(i int, from navPoint) (navPoint, bool) {
	r, h := b.cfg.Spots, b.walk.nav.Hier
	sp := &r.spots[i]
	if from.m.Kind() != nav.InSCC {
		return b.snapSpot(sp, from)
	}
	k := spotPointKey{spot: i, scc: from.m.SCC()}
	if hit, ok := r.points.get(h, k); ok {
		return hit.p, hit.ok
	}
	p, ok := b.snapSpot(sp, from)
	if ok {
		// p is in from's SCC: keep it as a place, without its cluster.
		m, err := h.LocatePlace(p.m.Cell())
		if err != nil {
			return p, ok
		}
		p.m = m
	}
	r.points.put(h, k, spotPointHit{p: p, ok: ok})
	return p, ok
}
