package nav

import (
	"bytes"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

// testLmax is run speed 104 × the 141 ms Emax N0 measured.
const testLmax = 15

// Extra shapes next to the fixture scenes, in region-local cells.
var (
	// Stair: ground 0 west of column 200, a one-cell step at 40 on column
	// 200, then a cell pair of layers 20 and 80 from column 201. Walking
	// east in short steps climbs 0, 40, 80; a step long enough to cross the
	// 40 cell lands on the 20 layer while CanMove's border walk reaches 80,
	// so that step is blocked.
	StairLow  = geotest.Point{CellX: 195, CellY: 1005, Z: 0}
	StairDeck = geotest.Point{CellX: 210, CellY: 1005, Z: 80}
	// Stacked: ground 0 under a second layer at 30, too close above it to
	// click the ground.
	StackedGround = geotest.Point{CellX: 302, CellY: 1002, Z: 0}
	// Floors: ground 0 under a deck 40, 60 or 100 above it, each over the
	// 11 columns around the point's, rows 1000..1010. CanMove crossing the ground under
	// the 40 deck lands on the deck (belowCell(0 + 48)); under the others it
	// stays on the ground.
	Floors40  = geotest.Point{CellX: 325, CellY: 1005, Z: 0}
	Floors60  = geotest.Point{CellX: 345, CellY: 1005, Z: 0}
	Floors100 = geotest.Point{CellX: 365, CellY: 1005, Z: 0}
	// Open: plain ground far from every shape.
	Open = geotest.Point{CellX: 100, CellY: 100, Z: 0}
)

func terrain() *geotest.Terrain {
	t := geotest.Scenes()
	t.Raise(200, 1000, 200, 1010, 40).Raise(201, 1000, 215, 1010, 20).AddDeck(201, 1000, 215, 1010, 80)
	t.AddDeck(300, 1000, 305, 1005, 30)
	t.AddDeck(Floors40.CellX-5, 1000, Floors40.CellX+5, 1010, 40)
	t.AddDeck(Floors60.CellX-5, 1000, Floors60.CellX+5, 1010, 60)
	t.AddDeck(Floors100.CellX-5, 1000, Floors100.CellX+5, 1010, 100)
	return t
}

var (
	sharedOnce   sync.Once
	sharedEngine *engine.Engine
	sharedErr    error
)

// testEngine builds the synthetic region once per test binary: the fixture
// scenes plus the shapes above, written as L2OFF and read back through the
// stored form, as geoquery's tests do.
func testEngine(tb testing.TB) *engine.Engine {
	tb.Helper()
	sharedOnce.Do(func() { sharedEngine, sharedErr = buildEngine(terrain()) })
	if sharedErr != nil {
		tb.Fatal(sharedErr)
	}
	return sharedEngine
}

func buildEngine(t *geotest.Terrain) (*engine.Engine, error) {
	return buildEngineAt(t, [2]int{geotest.RegionX, geotest.RegionY})
}

// buildEngineAt builds an engine holding t's region at each given region
// tile.
func buildEngineAt(t *geotest.Terrain, tiles ...[2]int) (*engine.Engine, error) {
	dir, err := os.MkdirTemp("", "navtest")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path, err := t.WriteL2OFF(dir)
	if err != nil {
		return nil, err
	}
	decoded, _, err := reader.ReadL2OFF(path)
	if err != nil {
		return nil, err
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		return nil, err
	}
	region, err := block.ParseRegion(stored.Bytes())
	if err != nil {
		return nil, err
	}
	e := engine.New()
	for _, tile := range tiles {
		if err := e.SetRegion(tile[0], tile[1], region); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func testPlanner(tb testing.TB) *Planner {
	return &Planner{Engine: testEngine(tb), Lmax: testLmax, CollisionHeight: 23, Budget: 40000}
}

func at(p geotest.Point, dx, dy, dz int) location.Location {
	x, y, z := p.World()
	return location.Location{X: x + dx, Y: y + dy, Z: z + dz}
}

func loc(p geotest.Point) location.Location { return at(p, 0, 0, 0) }

func cellOf(l location.Location) (int, int) { return engine.GeoX(l.X), engine.GeoY(l.Y) }

// floor puts l on the layer nearest its Z.
func floor(e *engine.Engine, l location.Location) location.Location {
	l.Z = int(e.Height(l.X, l.Y, l.Z))
	return l
}

// walkSteps runs n random update sequences of the copied step chain from a
// to b, every step at most lmax long (the first one too), and reports the
// first one that is blocked or does not arrive.
func walkSteps(e *engine.Engine, rng *rand.Rand, a, b location.Location, lmax float64, n int) (engine.Walk, bool) {
	speed := lmax * 10 // per second: a 100 ms update covers lmax
	dist := hypot(b.X-a.X, b.Y-a.Y)
	steps := int(dist) + 8 // a 1 ms update covers lmax/100; long enough in practice
	durations := make([]time.Duration, steps*100/int(max(lmax, 1))+16)
	for range n {
		for i := range durations {
			durations[i] = time.Duration(1+rng.IntN(100)) * time.Millisecond
		}
		w := e.WalkSteps(a, b, speed, speed, durations)
		if w.Blocked || !w.Reached {
			return w, false
		}
	}
	return engine.Walk{}, true
}

// walkFirst runs n random update sequences of the copied step chain from a
// to b whose first step is first units long, every later one at most lmax,
// and reports the first one that is blocked or does not arrive.
func walkFirst(e *engine.Engine, rng *rand.Rand, a, b location.Location, lmax, first float64, n int) (engine.Walk, bool) {
	speed := lmax * 10
	steps := int(hypot(b.X-a.X, b.Y-a.Y)) + 8
	durations := make([]time.Duration, steps*100/int(max(lmax, 1))+16)
	for range n {
		durations[0] = time.Duration(first / speed * float64(time.Second))
		for i := range durations[1:] {
			durations[i+1] = time.Duration(1+rng.IntN(100)) * time.Millisecond
		}
		w := e.WalkSteps(a, b, speed, speed, durations)
		if w.Blocked || !w.Reached {
			return w, false
		}
	}
	return engine.Walk{}, true
}
