package botclient

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestSpotSpread checks the spot choice once spots fill (#137): an empty
// spot before a held one with room, the next spot with room when the
// nearest are full, one beyond MaxTravel within Wander, then sharing the
// spot least filled for its cap past it, up to its limit.
func TestSpotSpread(t *testing.T) {
	now := at(time.Second)
	// Each spot takes 3 bots, 9 once shared; all fit level 5 but "high".
	spots := []datapack.Spot{
		testSpot("a", 2000, 0, 3, 6, 9),
		testSpot("b", 0, 6000, 3, 6, 9),
		testSpot("c", 30000, 0, 3, 6, 9),
		testSpot("beyond", 50000, 0, 3, 6, 9),
		testSpot("high", 100, 0, 9, 9, 9),
	}
	hold := func(r *SpotRegistry, i, n int) {
		for range n {
			r.held[i]++
		}
	}
	tests := []struct {
		name  string
		setup func(b *Brain)
		// want is the spot held after, -1 for none; wantHeld its count.
		want, wantHeld int
		// wantRetry: the bot looks again only after spotRetry.
		wantRetry bool
	}{
		{name: "all empty: the nearest", want: 0, wantHeld: 1},
		{name: "a held: b, empty, first", setup: func(b *Brain) { hold(b.cfg.Spots, 0, 1) }, want: 1, wantHeld: 1},
		{
			name:  "a and b held with room: a, the nearest",
			setup: func(b *Brain) { hold(b.cfg.Spots, 0, 1); hold(b.cfg.Spots, 1, 2) }, want: 0, wantHeld: 2,
		},
		{
			name:  "a and b full: c, beyond MaxTravel",
			setup: func(b *Brain) { hold(b.cfg.Spots, 0, 3); hold(b.cfg.Spots, 1, 3) }, want: 2, wantHeld: 1,
		},
		{
			name: "all in reach full: shares the least filled",
			setup: func(b *Brain) {
				hold(b.cfg.Spots, 0, 4)
				hold(b.cfg.Spots, 1, 3)
				hold(b.cfg.Spots, 2, 4)
			},
			want: 1, wantHeld: 4, wantRetry: true,
		},
		{
			name: "all in reach full, no overflow: waits",
			setup: func(b *Brain) {
				b.persona.Spots.Overflow = false
				for i := range 3 {
					hold(b.cfg.Spots, i, 3)
				}
			},
			want: -1, wantRetry: true,
		},
		{
			name: "all in reach at their limit: waits",
			setup: func(b *Brain) {
				for i := range 3 {
					hold(b.cfg.Spots, i, 9)
				}
			},
			want: -1, wantRetry: true,
		},
		{
			name:  "Wander under MaxTravel: no further than MaxTravel",
			setup: func(b *Brain) { b.persona.Spots.Wander = 0; hold(b.cfg.Spots, 0, 3); hold(b.cfg.Spots, 1, 3) },
			want:  0, wantHeld: 4, wantRetry: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 5, spots...)
			if tt.setup != nil {
				tt.setup(b)
			}
			b.pickSpot(now)
			got := -1
			if b.spot.held {
				got = b.spot.idx
			}
			if got != tt.want {
				t.Fatalf("spot %d, want %d", got, tt.want)
			}
			if got >= 0 && b.cfg.Spots.Held(got) != tt.wantHeld {
				t.Errorf("held %d, want %d", b.cfg.Spots.Held(got), tt.wantHeld)
			}
			if retry := b.spot.retryAt.Equal(now.Add(spotRetry)); retry != tt.wantRetry {
				t.Errorf("retry at %v, want spotRetry later %v", b.spot.retryAt, tt.wantRetry)
			}
		})
	}
}

// TestSpotOverflowMovesOn: a bot sharing a spot past its cap looks again
// each spotRetry and takes a spot with room once one frees, never sooner; a
// bot within its spot's cap stays.
func TestSpotOverflowMovesOn(t *testing.T) {
	b := newSpotBrain(t, 5, testSpot("a", 2000, 0, 3, 6, 9), testSpot("b", 0, 6000, 3, 6, 9))
	b.world.Objects = map[int32]*Object{}
	r := b.cfg.Spots
	r.held[0], r.held[1] = 3, 3
	b.decide(at(time.Second))
	if !b.spot.held || b.spot.idx != 0 || r.Held(0) != 4 {
		t.Fatalf("overflow: spot %d held %v, a held by %d", b.spot.idx, b.spot.held, r.Held(0))
	}
	r.release(1)
	b.decide(at(spotRetry))
	if b.spot.idx != 0 {
		t.Fatalf("moved to spot %d before spotRetry", b.spot.idx)
	}
	b.decide(at(spotRetry + time.Second))
	if !b.spot.held || b.spot.idx != 1 || r.Held(0) != 3 || r.Held(1) != 3 {
		t.Fatalf("a spot with room: spot %d held %v, held a %d b %d", b.spot.idx, b.spot.held, r.Held(0), r.Held(1))
	}
	r.release(0)
	b.decide(at(3 * spotRetry))
	if b.spot.idx != 1 {
		t.Fatalf("a bot within its spot's cap moved to spot %d", b.spot.idx)
	}
}

// TestSpotSpreadLevels: bots that outlevel one spot together spread over
// every spot that fits their new level, not onto the nearest.
func TestSpotSpreadLevels(t *testing.T) {
	r := NewSpotRegistry([]datapack.Spot{
		testSpot("newbie", 1000, 0, 1, 3, 30),
		testSpot("near", 3000, 0, 8, 11, 9),
		testSpot("mid", 0, 8000, 8, 11, 9),
		testSpot("far", 15000, 0, 8, 11, 9),
	}, 1)
	bots := make([]*Brain, 3)
	for i := range bots {
		b := newSpotBrain(t, 1)
		b.cfg.Spots = r
		b.pickSpot(at(time.Second))
		if !b.spot.held || b.spot.idx != 0 {
			t.Fatalf("bot %d at level 1: spot %d held %v", i, b.spot.idx, b.spot.held)
		}
		bots[i] = b
	}
	var got []int
	for _, b := range bots {
		b.world.Self.Level = 10
		b.decide(at(2 * time.Second))
		got = append(got, b.spot.idx)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("spots at level 10 %v, want one each [1 2 3]", got)
	}
}

// homeRun is how many farmers each start point had on the home run (#137);
// Talking Island's 69 are split between its two.
var homeRun = map[string]int{
	"talking island, fighter": 35,
	"talking island, mystic":  34,
	"elven village":           67,
	"dark elven village":      71,
	"orc village":             77,
	"dwarven village":         56,
}

// TestStarterSpread ramps the home run's level 1 farmers of every village
// at once on one registry over the real datapack's starter spots (#137):
// every spot that fits them within Wander of a start is held, none past its
// limit, and a farmer holds nothing only when every spot that fits it in
// reach is at its limit.
func TestStarterSpread(t *testing.T) {
	spots := loadStarterSpots(t)
	r := NewSpotRegistry(spots, 1)
	p := defaultSpotPolicy()
	now := at(time.Second)
	type farmer struct {
		village string
		b       *Brain
	}
	var farmers []farmer
	left := maps.Clone(homeRun)
	for more := true; more; {
		more = false
		for _, st := range starterStarts {
			if left[st.village] == 0 {
				continue
			}
			left[st.village]--
			more = true
			b := newSpotBrain(t, 1)
			b.cfg.Spots = r
			b.world.Self.move = stopAt(st.at)
			b.pickSpot(now)
			farmers = append(farmers, farmer{st.village, b})
		}
	}
	wander := int64(p.Wander) * int64(p.Wander)
	inReach := func(at Point, i int) bool {
		return p.fits(&spots[i], 1) && dist2(at, Point{X: spots[i].Center.X, Y: spots[i].Center.Y}) <= wander
	}
	placed := map[string]int{}
	for _, f := range farmers {
		if f.b.spot.held {
			placed[f.village]++
			continue
		}
		at := f.b.world.Self.Position(now)
		for i := range spots {
			if inReach(at, i) && r.Held(i) < r.limits[i] {
				t.Errorf("a %s farmer holds nothing; %s in reach is below its limit", f.village, spots[i].Maker)
			}
		}
	}
	total := 0
	for _, st := range starterStarts {
		t.Logf("%s: %d of %d farmers hold a spot", st.village, placed[st.village], homeRun[st.village])
		total += placed[st.village]
	}
	t.Logf("%d of %d farmers hold a spot", total, len(farmers))
	for i := range spots {
		if r.Held(i) > r.limits[i] {
			t.Errorf("%s held by %d, past its limit %d", spots[i].Maker, r.Held(i), r.limits[i])
		}
		if r.Held(i) == 0 && slices.ContainsFunc(starterStarts, func(st starterStart) bool { return inReach(st.at, i) }) {
			t.Errorf("%s fits level 1 in reach of a start and no farmer holds it", spots[i].Maker)
		}
	}
	if total*2 <= len(farmers) {
		t.Errorf("%d of %d farmers hold a spot, not most", total, len(farmers))
	}
}
