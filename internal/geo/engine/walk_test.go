package engine_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

func scenes(tb testing.TB) *engine.Engine {
	tb.Helper()
	path, err := geotest.Scenes().WriteL2OFF(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	decoded, _, err := reader.ReadL2OFF(path)
	if err != nil {
		tb.Fatal(err)
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		tb.Fatal(err)
	}
	region, err := block.ParseRegion(stored.Bytes())
	if err != nil {
		tb.Fatal(err)
	}
	e := engine.New()
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		tb.Fatal(err)
	}
	return e
}

func at(p geotest.Point, dx, dy int) location.Location {
	x, y, z := p.World()
	return location.Location{X: x + dx, Y: y + dy, Z: z}
}

func steps(n int, d time.Duration) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = d
	}
	return out
}

// TestWalkSteps pins the server's player step chain arithmetic: whole
// milliseconds of the update speed (at least one), the start speed for the
// first five updates, the step measured from the current cell with the
// landing cell rounded half up, the ground height at most two cell heights
// above, and the walk stopping blocked where CanMove refuses a step.
func TestWalkSteps(t *testing.T) {
	e := scenes(t)
	open := geotest.CorridorWest
	tests := []struct {
		name              string
		from, to          location.Location
		startSpeed, speed float64
		durations         []time.Duration
		want              engine.Walk
	}{
		{
			name: "diagonal step rounds half up", from: at(open, 0, 0), to: at(open, 100, 100),
			startSpeed: 100, speed: 100, durations: steps(1, 100*time.Millisecond),
			want: engine.Walk{Stop: at(open, 7, 7), Steps: 1},
		},
		{
			name: "sub-millisecond time is dropped", from: at(open, 0, 0), to: at(open, 100, 0),
			startSpeed: 1000, speed: 1000, durations: []time.Duration{2*time.Millisecond + 900*time.Microsecond},
			want: engine.Walk{Stop: at(open, 2, 0), Steps: 1},
		},
		{
			name: "no elapsed time still moves one millisecond", from: at(open, 0, 0), to: at(open, 100, 0),
			startSpeed: 3000, speed: 3000, durations: []time.Duration{0},
			want: engine.Walk{Stop: at(open, 3, 0), Steps: 1},
		},
		{
			name: "start speed for five updates, then run speed", from: at(open, 0, 0), to: at(open, 500, 0),
			startSpeed: 10, speed: 100, durations: steps(6, time.Second),
			want: engine.Walk{Stop: at(open, 150, 0), Steps: 6},
		},
		{
			name: "the step reaching the destination lands on it", from: at(open, 0, 0), to: at(open, 50, 0),
			startSpeed: 120, speed: 120, durations: steps(10, 100*time.Millisecond),
			want: engine.Walk{Stop: at(open, 50, 0), Steps: 5, Reached: true},
		},
		{
			name: "walking into the corridor wall stops blocked", from: at(open, 0, 0), to: at(open, 0, -200),
			startSpeed: 160, speed: 160, durations: steps(10, 100*time.Millisecond),
			want: engine.Walk{Stop: at(open, 0, -16), Steps: 1, Blocked: true},
		},
		{
			name: "dropping off the ledge lands on the ground", from: at(geotest.LedgeHigh, 0, 0), to: at(geotest.LedgeLow, 0, 0),
			startSpeed: 300, speed: 300, durations: steps(10, 100*time.Millisecond),
			want: engine.Walk{Stop: at(geotest.LedgeLow, 0, 0), Steps: 8, Reached: true},
		},
		{
			name: "climbing the ledge stops at its foot", from: at(geotest.LedgeLow, 0, 0), to: at(geotest.LedgeHigh, 0, 0),
			startSpeed: 300, speed: 300, durations: steps(10, 100*time.Millisecond),
			want: engine.Walk{Stop: at(geotest.LedgeLow, 150, 0), Steps: 5, Blocked: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.WalkSteps(tt.from, tt.to, tt.startSpeed, tt.speed, tt.durations)
			if got != tt.want {
				t.Errorf("WalkSteps() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// BenchmarkCanMoveShort measures one short CanMove, a few cells across
// complex blocks, the unit the planner thresholds are set in.
func BenchmarkCanMoveShort(b *testing.B) {
	e := scenes(b)
	ox, oy, oz := geotest.CorridorWest.World()
	tx, ty := ox+64, oy+16
	b.ReportAllocs()
	for b.Loop() {
		if !e.CanMove(ox, oy, oz, tx, ty, oz) {
			b.Fatal("CanMove() = false along the corridor")
		}
	}
}
