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
	// beside are how many bots farm each spot with no claim
	// (SpotPolicy.Beside): counted apart from held, toward neither the cap
	// nor the limit.
	beside []int

	// points keeps spotPoint's answers; it has its own lock.
	points spotPoints
}

// NewSpotRegistry returns a registry over spots (datapack.Datapack.Spots).
// multiplier is the server's SpawnMultiplier (npcs.properties, 1 by
// default): the server scales a maker's maximumNpcs by it, rounded, and a
// spot takes a third of that many bots, at least one, and up to that many
// when every spot that fits a bot is full (claimOver).
func NewSpotRegistry(spots []datapack.Spot, multiplier float64) *SpotRegistry {
	r := &SpotRegistry{
		spots: spots, caps: make([]int, len(spots)), limits: make([]int, len(spots)),
		held: make([]int, len(spots)), beside: make([]int, len(spots)),
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

// Beside is the number of bots farming spot i with no claim
// (SpotPolicy.Beside).
func (r *SpotRegistry) Beside(i int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.beside[i]
}

// SpotStatus is a spot of the registry as the control panel lists it: its
// maker name (the argument of "go"), its monsters' levels, its cap, how
// many bots hold it and how many farm beside it with no claim.
type SpotStatus struct {
	Name                           string
	MinLevel, MaxLevel, AggroLevel int
	Cap, Held, Beside              int
}

// Statuses returns every spot in registry order with its held and beside
// counts; nil for a nil registry (no datapack).
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
			Cap: r.caps[i], Held: r.held[i], Beside: r.beside[i],
		}
	}
	return out
}

// claim takes the first spot of order (indices, best first) that no bot
// holds, else the first below its cap, and reports which; none when every
// one is full. Empty spots first spread the bots over as many spots as
// there are (#137).
func (r *SpotRegistry) claim(order []int) (int, bool) { return r.claimN(order, 1) }

// claimN takes n of a cap, as claim one: the first spot of order no bot
// holds whose cap takes n, else the first with room for n under its cap. A
// party's leader claims for its whole party (docs/parties/spots.md, "Claim
// weight").
func (r *SpotRegistry) claimN(order []int, n int) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range order {
		if r.held[i] == 0 && n <= r.caps[i] {
			r.held[i] += n
			return i, true
		}
	}
	for _, i := range order {
		if r.held[i]+n <= r.caps[i] {
			r.held[i] += n
			return i, true
		}
	}
	return 0, false
}

// claimTiers claims n (claimN) among the spots of order (sorted by
// MaxLevel, highest first) one MaxLevel at a time, highest first, so an
// empty spot outranks a shared one only among spots of one level: a bot
// under its level window takes the highest spot with room (#446).
func (r *SpotRegistry) claimTiers(order []int, n int) (int, bool) {
	for lo := 0; lo < len(order); {
		hi := lo + 1
		for hi < len(order) && r.spots[order[hi]].MaxLevel == r.spots[order[lo]].MaxLevel {
			hi++
		}
		if i, ok := r.claimN(order[lo:hi], n); ok {
			return i, true
		}
		lo = hi
	}
	return 0, false
}

// claimOverN takes n, past its cap, at the spot of order whose held plus n
// stays within its limit that its bots fill least for its cap, the first
// in order on a tie, and reports which; none when every one is too near its
// limit. It is for a bot (n 1) or a party's leader (n its party) that finds
// every spot that fits it at its cap: the overflow spreads evenly rather
// than piling on the nearest spot (#137).
func (r *SpotRegistry) claimOverN(order []int, n int) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	best := -1
	for _, i := range order {
		if r.held[i]+n > r.limits[i] {
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
	r.held[best] += n
	return best, true
}

// farmBesideN counts n bots (a bot, or a party's leader for its party)
// farming, with no claim, the spot of order that the bots beside fill least
// per monster the server keeps there (its limit), the first in order on a
// tie, and reports which; none for an empty order. It is for a bot that
// finds every spot that fits it full (SpotPolicy.Beside): those bots spread
// over the full spots by their monsters rather than piling on the nearest
// one.
func (r *SpotRegistry) farmBesideN(order []int, n int) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	best := -1
	for _, i := range order {
		// beside/limit below the best's, in integers.
		if best < 0 || r.beside[i]*r.limits[best] < r.beside[best]*r.limits[i] {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	r.beside[best] += n
	return best, true
}

// leaveBesideN gives back n of the counts farmBesideN took, never below 0.
func (r *SpotRegistry) leaveBesideN(i, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beside[i] = max(0, r.beside[i]-n)
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

// release gives back one claim took.
func (r *SpotRegistry) release(i int) { r.releaseN(i, 1) }

// releaseN gives back n of a spot's held count, never below 0.
func (r *SpotRegistry) releaseN(i, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held[i] = max(0, r.held[i]-n)
}

// reweigh moves the count a party's leader holds at spot i, or farms beside
// it, from old to n in one step (docs/parties/spots.md, "Weighted
// release"): a window add or delete, or the end of its leading. It checks
// nothing: a held count weighed up may pass the cap, and the leader's spot
// rule then looks for room (overCap).
func (r *SpotRegistry) reweigh(i int, beside bool, old, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &r.held[i]
	if beside {
		c = &r.beside[i]
	}
	*c = max(0, *c+n-old)
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
