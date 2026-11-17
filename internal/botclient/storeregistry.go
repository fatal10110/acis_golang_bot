package botclient

import (
	"math"
	"sync"
)

// storeTownRadius is how far from its town's centre a bot store opens: a
// town is the spawn of a gatekeeper (TravelMap.Towns), and the gatekeepers
// of one town within it are one town. The datapack the bot reads has no
// town zones, and the server needs none (jail is its only no-store zone).
const storeTownRadius = 3000

// StoreRegistry is the bot stores of a process: in which town each stands
// and where. Every bot of the process shares one. The towns and limits are
// read-only after NewStoreRegistry; mu owns the stores.
type StoreRegistry struct {
	towns   []Point
	perTown int
	spacing int32

	mu     sync.Mutex
	stores map[int]storePlace
	nextID int
}

// storePlace is where a bot store stands, and in which town.
type storePlace struct {
	town int
	at   Point
}

// NewStoreRegistry returns a registry of stores in towns (TravelMap.Towns),
// at most perTown in each and none within spacing of another.
func NewStoreRegistry(towns []Point, perTown int, spacing int32) *StoreRegistry {
	return &StoreRegistry{towns: towns, perTown: perTown, spacing: spacing, stores: make(map[int]storePlace)}
}

// Towns is the number of towns.
func (r *StoreRegistry) Towns() int { return len(r.towns) }

// Open is the number of bot stores open or opening.
func (r *StoreRegistry) Open() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.stores)
}

// town is the town at is in: the nearest one within storeTownRadius.
func (r *StoreRegistry) town(at Point) (int, bool) {
	best, bestDist := 0, math.Inf(1)
	for i, t := range r.towns {
		if d := dist2D(at, t); d <= storeTownRadius && d < bestDist {
			best, bestDist = i, d
		}
	}
	return best, !math.IsInf(bestDist, 1)
}

// roomLocked reports the town a store at at would stand in, and whether it
// may: in a town with fewer than perTown stores, none within spacing.
// r.mu must be held.
func (r *StoreRegistry) roomLocked(at Point) (int, bool) {
	town, ok := r.town(at)
	if !ok {
		return 0, false
	}
	n := 0
	for _, s := range r.stores {
		if s.town == town {
			n++
		}
		if dist2D(at, s.at) < float64(r.spacing) {
			return 0, false
		}
	}
	return town, n < r.perTown
}

// hasRoom reports whether a store may open at at.
func (r *StoreRegistry) hasRoom(at Point) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.roomLocked(at)
	return ok
}

// claim takes the place of a store at at and reports its claim id; none
// when it may not open there.
func (r *StoreRegistry) claim(at Point) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	town, ok := r.roomLocked(at)
	if !ok {
		return 0, false
	}
	r.nextID++
	r.stores[r.nextID] = storePlace{town: town, at: at}
	return r.nextID, true
}

// release gives back the place claim took.
func (r *StoreRegistry) release(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.stores, id)
}

// dist2D is the horizontal distance between a and b.
func dist2D(a, b Point) float64 {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}

// Towns are the towns of the map for a store registry: the gatekeepers'
// spawns, those within storeTownRadius of an earlier one merged into it.
func (m *TravelMap) Towns() []Point {
	var towns []Point
	for _, g := range m.gatekeepers {
		merged := false
		for _, t := range towns {
			if dist2D(g.at, t) <= storeTownRadius {
				merged = true
				break
			}
		}
		if !merged {
			towns = append(towns, g.at)
		}
	}
	return towns
}
