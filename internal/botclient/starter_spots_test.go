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
// every spot within Wander of a start point whose monsters are at most
// level starterMaxLevel.
const starterSpotsFile = "testdata/starter_spots.json"

// starterSpot is a datapack.Spot of the fixture, its territory aside.
type starterSpot struct {
	Maker              string
	X, Y, Z            int32
	MinLevel, MaxLevel int
	AggroLevel         int
	MaximumNPCs        int
}

// starterMaxLevel is the highest monster level of the fixture: all a
// level 5 farmer fits (SpotPolicy.Below).
const starterMaxLevel = 7

// starterSpots picks the fixture's spots from the spots of a datapack.
func starterSpots(all []datapack.Spot) []starterSpot {
	reach := int64(defaultSpotPolicy().Wander) * int64(defaultSpotPolicy().Wander)
	var out []starterSpot
	for _, s := range all {
		c := Point{X: s.Center.X, Y: s.Center.Y}
		if s.MaxLevel > starterMaxLevel || !slices.ContainsFunc(starterStarts, func(st starterStart) bool {
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
// bot) finds none outside Talking Island (#133); then farmers claim until
// every spot in reach (Wander) is full, which is the village's room, and
// share them until every one is at its limit; the next one waits
// spotRetry before it looks again (#137).
func TestStarterSpots(t *testing.T) {
	spots := loadStarterSpots(t)
	// newbieZone is how far from its start a new character's spot lies.
	const newbieZone = 10000
	tests := []struct {
		village string
		// planRule: a farmer held to AggroAbove alone finds a spot.
		planRule bool
		// room is how many level 1 farmers find a spot with room from the
		// start, limit how many find one sharing past the caps.
		room, limit int
	}{
		{"talking island, fighter", true, 17, 58},
		{"talking island, mystic", true, 17, 58},
		{"elven village", false, 36, 118},
		{"dark elven village", false, 36, 118},
		{"orc village", false, 13, 46},
		{"dwarven village", false, 4, 13},
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
			pickStraight(plan, now)
			if plan.spot.held != tt.planRule {
				t.Errorf("plan rule alone: spot held %v, want %v", plan.spot.held, tt.planRule)
			}

			r := NewSpotRegistry(spots, 1)
			first := farmer(r)
			pickStraight(first, now)
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

			claimAll := func(overflow bool) (int, *Brain) {
				for n := 0; ; n++ {
					f := farmer(r)
					f.persona.Spots.Overflow = overflow
					if pickStraight(f, now); !f.spot.held {
						return n, f
					}
				}
			}
			if n, _ := claimAll(false); n+1 != tt.room {
				t.Errorf("room for %d level 1 farmers, want %d", n+1, tt.room)
			}
			n, last := claimAll(true)
			if n += tt.room; n != tt.limit {
				t.Errorf("%d level 1 farmers sharing, want %d", n, tt.limit)
			}
			// Overflow: the next farmer waits, and finds the spot a farmer
			// gives up once the wait is over.
			first.leaveSpot()
			if last.spot.held || !last.spot.retryAt.Equal(now.Add(spotRetry)) {
				t.Fatalf("overflow farmer: held %v, looks again at %v, want spotRetry later", last.spot.held, last.spot.retryAt)
			}
			if pickStraight(last, now.Add(spotRetry)); !last.spot.held {
				t.Fatalf("overflow farmer finds no spot after spotRetry, a spot free")
			}
		})
	}
}
