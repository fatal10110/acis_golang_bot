package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// fallbackSpots are a level 14 squire's spots for the fallback (#446):
// the werewolves and the orcs of its level window (MaxLevel 11 or more),
// which no potion carries it through (TestPackFitPick), and under it toads
// of level 9, 7 and 5, which fit, and werewolves marked level 10, which do
// not.
func fallbackSpots() []datapack.Spot {
	place := func(s datapack.Spot, name string, x, y int32) datapack.Spot {
		s.Maker, s.Center, s.MaximumNPCs = name, datapack.Point{X: x, Y: y}, 11
		return s
	}
	toads := func(level int) datapack.Spot {
		s := tiToads27
		s.MinLevel, s.MaxLevel, s.AggroLevel = level, level, level
		s.Mobs = []datapack.Mob{{Level: level, Count: 5, Combat: giantToad}}
		return s
	}
	wolves10 := tiWolves
	wolves10.MaxLevel = 10
	return []datapack.Spot{
		place(tiWolves, "wolves", 1000, 0),
		place(tiOrcs01, "orcs", 0, 3000),
		place(toads(5), "toads5", 2000, 0),
		place(toads(9), "toads9", 0, 6000),
		place(wolves10, "wolves10", 4000, 0),
		place(toads(7), "toads7", 2000, 3000),
	}
}

// heldSpot is the spot b holds, -1 for none.
func heldSpot(b *Brain) int {
	if !b.spot.held {
		return -1
	}
	return b.spot.idx
}

// TestSpotFallback checks which spot a level 14 squire claims when no spot
// of its level passes the model (#446): the highest under its level window
// that does, however far, never one that does not, and one of its level
// while one fits.
func TestSpotFallback(t *testing.T) {
	const wolves, orcs, toads5, toads9, wolves10 = 0, 1, 2, 3, 4
	tests := []struct {
		name    string
		spots   []int
		potions int32
		want    int
		lower   bool
	}{
		{"3 potions: the orcs of its level", nil, 3, orcs, false},
		{"no potion: the highest under its window", nil, 0, toads9, true},
		{"no potion, no toads of 9: the toads of 5", []int{wolves, orcs, toads5, wolves10}, 0, toads5, true},
		{"no potion, only a pack too much under it: none", []int{wolves, orcs, wolves10}, 0, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all := fallbackSpots()
			spots := all
			if tt.spots != nil {
				spots = nil
				for _, i := range tt.spots {
					spots = append(spots, all[i])
				}
			}
			b := newSpotBrain(t, 14, spots...)
			b.persona.Potions = []int32{lesserHealingPotion}
			setSquire(b, squire14, tt.potions)
			b.decide(at(time.Second))
			got := heldSpot(b)
			if got >= 0 {
				got = indexOf(all, spots[got].Maker)
			}
			if got != tt.want || b.spot.lower != tt.lower {
				t.Fatalf("spot %d (lower %v), want %d (lower %v)", got, b.spot.lower, tt.want, tt.lower)
			}
		})
	}
}

func indexOf(spots []datapack.Spot, maker string) int {
	for i := range spots {
		if spots[i].Maker == maker {
			return i
		}
	}
	return -1
}

// TestSpotFallbackRepick checks a held spot over the fallback (#446): a
// squire that claimed the werewolves before its stats came in gives them up
// for the highest spot under its window that fits once they do, keeps that
// one a spotRetry later with nothing better, and takes the orcs of its
// level once potions carry it through them.
func TestSpotFallbackRepick(t *testing.T) {
	const wolves, orcs, toads9 = 0, 1, 3
	b := newSpotBrain(t, 14, fallbackSpots()...)
	b.persona.Potions = []int32{lesserHealingPotion}
	b.decide(at(time.Second))
	if got := heldSpot(b); got != wolves {
		t.Fatalf("before its stats: spot %d, want the werewolves %d", got, wolves)
	}
	setSquire(b, squire14, 0)
	steps := []struct {
		name    string
		after   time.Duration
		potions int32
		want    int
		lower   bool
	}{
		{"stats in: the toads of 9", 2 * time.Second, 0, toads9, true},
		{"a retry on: kept", 2*time.Second + spotRetry, 0, toads9, true},
		{"3 potions a retry on: the orcs", 2*time.Second + 2*spotRetry, 3, orcs, false},
	}
	for _, st := range steps {
		b.world.Apply(ItemList{Items: []Item{{ObjectID: 0x30000011, TemplateID: lesserHealingPotion, Count: st.potions}}}, t0)
		b.decide(at(st.after))
		if got := heldSpot(b); got != st.want || b.spot.lower != st.lower {
			t.Fatalf("%s: spot %d (lower %v), want %d (lower %v)", st.name, got, b.spot.lower, st.want, st.lower)
		}
		if n := b.cfg.Spots.Held(st.want); n != 1 {
			t.Fatalf("%s: spot %d held by %d, want 1", st.name, st.want, n)
		}
	}
}

// TestSpotFallbackShared checks the fallback with other bots on the spots
// (#446): a level 14 squire with no potion takes the highest spot under its
// window with room, even one shared below its cap over an empty lower one,
// shares past its cap the one its bots fill least when every one is full
// (SpotPolicy.Overflow), and keeps a held fallback while the higher ones
// are full rather than hop to an empty lower one, until one has room.
func TestSpotFallbackShared(t *testing.T) {
	const toads5, toads9, toads7 = 2, 3, 5
	type step struct {
		after   time.Duration
		release int // a bot leaving this spot before the step, -1 for none
		want    int
	}
	tests := []struct {
		name  string
		held  map[int]int // bots already on each spot
		steps []step
	}{
		{
			"toads 9 shared below its cap: toads 9",
			map[int]int{toads9: 1},
			[]step{{time.Second, -1, toads9}},
		},
		{
			"every fallback at its cap: past the cap on the least filled",
			map[int]int{toads9: 4, toads7: 3, toads5: 3},
			[]step{{time.Second, -1, toads7}},
		},
		{
			"toads 9 full: toads 7, kept, then toads 9 with room",
			map[int]int{toads9: 3},
			[]step{
				{time.Second, -1, toads7},
				{time.Second + spotRetry, -1, toads7},
				{time.Second + 2*spotRetry, -1, toads7},
				{time.Second + 3*spotRetry, toads9, toads9},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, fallbackSpots()...)
			b.persona.Potions = []int32{lesserHealingPotion}
			setSquire(b, squire14, 0)
			r := b.cfg.Spots
			for i, n := range tt.held {
				for range n {
					if _, ok := r.claimOver([]int{i}); !ok {
						t.Fatalf("pre-claim spot %d: full", i)
					}
				}
			}
			for _, st := range tt.steps {
				if st.release >= 0 {
					r.release(st.release)
				}
				b.decide(at(st.after))
				if got := heldSpot(b); got != st.want || !b.spot.lower {
					t.Fatalf("at %v: spot %d (lower %v), want %d (lower true)", st.after, got, b.spot.lower, st.want)
				}
				if n, want := r.Held(st.want), tt.held[st.want]+1; st.release < 0 && n != want {
					t.Fatalf("at %v: spot %d held by %d, want %d", st.after, st.want, n, want)
				}
			}
		})
	}
}
