package botclient

import (
	"math"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// SpotRegistry is the farming spots of a process and how many of its bots
// hold each one; every bot of the process shares one. The spots, caps and
// limits are read-only after NewSpotRegistry; mu owns the counts.
type SpotRegistry struct {
	spots []datapack.Spot
	caps  []int
	// limits are how many bots a spot takes once every spot that fits them
	// is at its cap (claimOver): one per monster the server keeps alive
	// there.
	limits []int

	mu   sync.Mutex
	held []int
}

// NewSpotRegistry returns a registry over spots (datapack.Datapack.Spots).
// multiplier is the server's SpawnMultiplier (npcs.properties, 1 by
// default): the server scales a maker's maximumNpcs by it, rounded, and a
// spot takes a third of that many bots, at least one, and up to that many
// when every spot that fits a bot is full (claimOver).
func NewSpotRegistry(spots []datapack.Spot, multiplier float64) *SpotRegistry {
	r := &SpotRegistry{
		spots: spots, caps: make([]int, len(spots)), limits: make([]int, len(spots)),
		held: make([]int, len(spots)),
	}
	for i, s := range spots {
		r.caps[i] = spotCap(s.MaximumNPCs, multiplier)
		r.limits[i] = max(r.caps[i], int(math.Round(float64(s.MaximumNPCs)*multiplier)))
	}
	return r
}

// spotCap is the number of bots a spot of a maker with maximumNPCs takes
// under the spawn multiplier: a third of the live NPC cap the server
// applies (spawn/maker.go scaleBySpawnMultiplier), at least one.
func spotCap(maximumNPCs int, multiplier float64) int {
	return max(1, int(math.Round(float64(maximumNPCs)*multiplier))/3)
}

// Len is the number of spots.
func (r *SpotRegistry) Len() int { return len(r.spots) }

// Held is the number of bots holding spot i.
func (r *SpotRegistry) Held(i int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held[i]
}

// SpotStatus is a spot of the registry as the control panel lists it: its
// maker name (the argument of "go"), its monsters' levels, its cap and how
// many bots hold it.
type SpotStatus struct {
	Name                           string
	MinLevel, MaxLevel, AggroLevel int
	Cap, Held                      int
}

// Statuses returns every spot in registry order with its held count; nil
// for a nil registry (no datapack).
func (r *SpotRegistry) Statuses() []SpotStatus {
	if r == nil {
		return nil
	}
	out := make([]SpotStatus, len(r.spots))
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.spots {
		out[i] = SpotStatus{
			Name: s.Maker, MinLevel: s.MinLevel, MaxLevel: s.MaxLevel, AggroLevel: s.AggroLevel,
			Cap: r.caps[i], Held: r.held[i],
		}
	}
	return out
}

// claim takes the first spot of order (indices, best first) that no bot
// holds, else the first below its cap, and reports which; none when every
// one is full. Empty spots first spread the bots over as many spots as
// there are (#137).
func (r *SpotRegistry) claim(order []int) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range order {
		if r.held[i] == 0 {
			r.held[i]++
			return i, true
		}
	}
	for _, i := range order {
		if r.held[i] < r.caps[i] {
			r.held[i]++
			return i, true
		}
	}
	return 0, false
}

// claimOver takes, past its cap, the spot of order below its limit that
// its bots fill least for its cap, the first in order on a tie, and
// reports which; none when every one is at its limit. It is for a bot
// that finds every spot that fits it at its cap: the overflow spreads
// evenly rather than piling on the nearest spot (#137).
func (r *SpotRegistry) claimOver(order []int) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	best := -1
	for _, i := range order {
		if r.held[i] >= r.limits[i] {
			continue
		}
		// held/cap below the best's, in integers.
		if best < 0 || r.held[i]*r.caps[best] < r.held[best]*r.caps[i] {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	r.held[best]++
	return best, true
}

// overCap reports whether more bots than its cap hold spot i.
func (r *SpotRegistry) overCap(i int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held[i] > r.caps[i]
}

// hasRoom reports whether spot i is below its cap.
func (r *SpotRegistry) hasRoom(i int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held[i] < r.caps[i]
}

// release gives back a spot claim took.
func (r *SpotRegistry) release(i int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.held[i] > 0 {
		r.held[i]--
	}
}

// index is the spot of the maker named name, in any case.
func (r *SpotRegistry) index(name string) (int, bool) {
	for i := range r.spots {
		if strings.EqualFold(r.spots[i].Maker, name) {
			return i, true
		}
	}
	return 0, false
}
