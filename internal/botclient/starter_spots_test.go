package botclient

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

var updateStarterSpots = flag.Bool("update-starter-spots", false, "rewrite testdata/starter_spots.json from BOT_DATAPACK_XML")

// starterStart is where a race's new character enters the world.
type starterStart struct {
	village string
	at      Point
}

// starterStarts are the start points of the races' new characters
// (classes/*.xml <spawns>): one per village, two on Talking Island.
var starterStarts = []starterStart{
	{"talking island, fighter", Point{X: -71338, Y: 258271}},
	{"talking island, mystic", Point{X: -90875, Y: 248162}},
	{"elven village", Point{X: 46045, Y: 41251}},
	{"dark elven village", Point{X: 28295, Y: 11063}},
	{"orc village", Point{X: -56733, Y: -113459}},
	{"dwarven village", Point{X: 108644, Y: -173947}},
}

// starterSpotsFile is the fixture of the real datapack's starter spots:
// every spot within MaxTravel of a start point whose monsters are at most
// level 5.
const starterSpotsFile = "testdata/starter_spots.json"

// starterSpot is a datapack.Spot of the fixture, its territory aside.
type starterSpot struct {
	Maker              string
	X, Y, Z            int32
	MinLevel, MaxLevel int
	AggroLevel         int
	MaximumNPCs        int
}

// starterSpots picks the fixture's spots from the spots of a datapack.
func starterSpots(all []datapack.Spot) []starterSpot {
	reach := int64(defaultSpotPolicy().MaxTravel) * int64(defaultSpotPolicy().MaxTravel)
	var out []starterSpot
	for _, s := range all {
		c := Point{X: s.Center.X, Y: s.Center.Y}
		if s.MaxLevel > 5 || !slices.ContainsFunc(starterStarts, func(st starterStart) bool {
			return dist2(st.at, c) <= reach
		}) {
			continue
		}
		out = append(out, starterSpot{
			Maker: s.Maker, X: s.Center.X, Y: s.Center.Y, Z: s.Z,
			MinLevel: s.MinLevel, MaxLevel: s.MaxLevel, AggroLevel: s.AggroLevel, MaximumNPCs: s.MaximumNPCs,
		})
	}
	return out
}

// loadStarterSpots reads the fixture. With BOT_DATAPACK_XML set it first
// checks the fixture against that datapack, or rewrites it with
// -update-starter-spots.
func loadStarterSpots(t *testing.T) []datapack.Spot {
	t.Helper()
	path := filepath.FromSlash(starterSpotsFile)
	if dir := os.Getenv("BOT_DATAPACK_XML"); dir != "" {
		dp, err := datapack.Load(dir)
		if err != nil {
			t.Fatalf("Load(%s): %v", dir, err)
		}
		want, err := json.MarshalIndent(starterSpots(dp.Spots()), "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		if *updateStarterSpots {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != string(want) {
			t.Fatalf("%s is not the starter spots of %s (err %v): run with -update-starter-spots", path, dir, err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture []starterSpot
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	spots := make([]datapack.Spot, len(fixture))
	for i, s := range fixture {
		spots[i] = datapack.Spot{
			Maker: s.Maker, Center: datapack.Point{X: s.X, Y: s.Y}, Z: s.Z,
			MinLevel: s.MinLevel, MaxLevel: s.MaxLevel, AggroLevel: s.AggroLevel, MaximumNPCs: s.MaximumNPCs,
		}
	}
	return spots
}

// TestStarterSpots places level 1 farmers at each race's start point over
// the real datapack's starter spots: the first claims a spot of its newbie
// zone, where the plan's rule alone (aggressive monsters not above the
// bot) finds none outside Talking Island; then farmers claim until every
// spot in reach is full, which is the village's capacity, and the next
// one waits spotRetry before it looks again (#133).
func TestStarterSpots(t *testing.T) {
	spots := loadStarterSpots(t)
	// newbieZone is how far from its start a new character's spot lies.
	const newbieZone = 10000
	tests := []struct {
		village string
		// planRule: a farmer held to AggroAbove alone finds a spot.
		planRule bool
		// capacity is how many level 1 farmers find a spot from the start.
		capacity int
	}{
		{"talking island, fighter", true, 15},
		{"talking island, mystic", true, 17},
		{"elven village", false, 22},
		{"dark elven village", false, 14},
		{"orc village", false, 13},
		{"dwarven village", false, 4},
	}
	for _, tt := range tests {
		t.Run(tt.village, func(t *testing.T) {
			i := slices.IndexFunc(starterStarts, func(st starterStart) bool {
				return st.village == tt.village
			})
			start := starterStarts[i].at
			now := at(time.Second)
			farmer := func(r *SpotRegistry) *Brain {
				b := newSpotBrain(t, 1)
				b.cfg.Spots = r
				b.world.Self.move = stopAt(start)
				return b
			}

			plan := farmer(NewSpotRegistry(spots, 1))
			plan.persona.Spots.Newbie = 0
			plan.pickSpot(now)
			if plan.spot.held != tt.planRule {
				t.Errorf("plan rule alone: spot held %v, want %v", plan.spot.held, tt.planRule)
			}

			r := NewSpotRegistry(spots, 1)
			first := farmer(r)
			first.pickSpot(now)
			if !first.spot.held {
				t.Fatal("a level 1 farmer finds no spot")
			}
			sp := &spots[first.spot.idx]
			d := dist2(start, Point{X: sp.Center.X, Y: sp.Center.Y})
			t.Logf("first spot %s: levels %d-%d, aggressive up to %d, squared distance %d", sp.Maker, sp.MinLevel, sp.MaxLevel, sp.AggroLevel, d)
			if d > newbieZone*newbieZone {
				t.Errorf("first spot %s is %d squared away, beyond the newbie zone", sp.Maker, d)
			}
			if sp.MaxLevel > 3 {
				t.Errorf("first spot %s has monsters up to level %d", sp.Maker, sp.MaxLevel)
			}

			n := 1
			var last *Brain
			for {
				last = farmer(r)
				if last.pickSpot(now); !last.spot.held {
					break
				}
				n++
			}
			if n != tt.capacity {
				t.Errorf("capacity %d level 1 farmers, want %d", n, tt.capacity)
			}
			// Overflow: the next farmer waits, and finds the spot a farmer
			// gives up once the wait is over.
			first.leaveSpot()
			if _, ok := last.ruleSpot(now.Add(spotRetry - time.Second)); ok || last.spot.held {
				t.Fatalf("overflow farmer looked again before spotRetry")
			}
			if last.ruleSpot(now.Add(spotRetry)); !last.spot.held {
				t.Fatalf("overflow farmer finds no spot after spotRetry, a spot free")
			}
		})
	}
}
