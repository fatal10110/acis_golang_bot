package botclient

import (
	"bytes"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

// flatGround samples open ground at height 0 everywhere.
type flatGround struct{}

func (flatGround) HeightNearest(int, int, int) int16 { return geotest.Ground }
func (flatGround) Above(int, int, int) (int16, bool) { return 0, false }

// recordingEngine is a doorEngine that records each door's state and every
// toggle that does not change it. The table calls it under its mutex only.
type recordingEngine struct {
	shut    map[dynamic.Object]bool
	toggles int
	bad     []string
}

func newRecordingEngine() *recordingEngine {
	return &recordingEngine{shut: make(map[dynamic.Object]bool)}
}

func (e *recordingEngine) AddObject(o dynamic.Object)    { e.toggle(o, true) }
func (e *recordingEngine) RemoveObject(o dynamic.Object) { e.toggle(o, false) }

func (e *recordingEngine) toggle(o dynamic.Object, shut bool) {
	if e.shut[o] == shut {
		e.bad = append(e.bad, fmt.Sprintf("door at %d,%d toggled to closed=%v twice", o.GeoX(), o.GeoY(), shut))
	}
	e.shut[o] = shut
	e.toggles++
}

// testDoor is the corridor door with id and default state opened.
func testDoor(id int, opened bool) *door.Template {
	t := geotest.CorridorDoor()
	t.ID, t.Opened = id, opened
	return t
}

// TestDoorTableDefaults: each door starts in its doors.xml state, closed
// ones toggled into the engine; a door with no footprint and an id not in
// doors.xml read as open, and a report of either reaches nothing.
func TestDoorTableDefaults(t *testing.T) {
	degenerate := testDoor(3, false)
	degenerate.Coordinates = degenerate.Coordinates[:2]
	e := newRecordingEngine()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	tbl := newDoorTable(e, flatGround{}, []*door.Template{testDoor(1, true), testDoor(2, false), degenerate}, log)
	if e.toggles != 1 {
		t.Fatalf("%d toggles at boot, want 1 (door 2)", e.toggles)
	}
	for _, tc := range []struct {
		id     int
		closed bool
	}{{1, false}, {2, true}, {3, false}, {99, false}} {
		if got := tbl.Closed(tc.id); got != tc.closed {
			t.Errorf("Closed(%d) = %v, want %v", tc.id, got, tc.closed)
		}
	}
	tbl.Set(3, true)
	tbl.Set(99, true)
	tbl.Set(99, false)
	if e.toggles != 1 || tbl.Closed(3) || tbl.Closed(99) {
		t.Fatalf("toggles %d, Closed(3) %v, Closed(99) %v after reports of doors not held", e.toggles, tbl.Closed(3), tbl.Closed(99))
	}
	if n := bytes.Count(logs.Bytes(), []byte("door=99")); n != 1 {
		t.Fatalf("door 99 logged %d times, want once:\n%s", n, logs.String())
	}
}

// TestDoorReporterDedupe: a bot reports a door only when the state it sees
// differs from what it saw last, and the table drops a state it holds.
func TestDoorReporterDedupe(t *testing.T) {
	e := newRecordingEngine()
	tbl := newDoorTable(e, flatGround{}, []*door.Template{testDoor(1, true)}, discardLog())
	a, b := NewDoorReporter(tbl), NewDoorReporter(tbl)
	steps := []struct {
		bot     *DoorReporter
		ev      Event
		closed  bool // the table's state after the step
		toggles int  // engine toggles so far
	}{
		{a, DoorInfo{ObjectID: 5, DoorID: 1}, false, 0},         // the default, dropped by the table
		{a, DoorStatusUpdate{ObjectID: 5, DoorID: 1}, false, 0}, // a repeat for a
		{b, DoorStatusUpdate{ObjectID: 5, DoorID: 1, Closed: true}, true, 1},
		{a, DoorInfo{ObjectID: 5, DoorID: 1}, true, 1},               // still a repeat for a: never reaches the table
		{a, DoorInfo{ObjectID: 5, DoorID: 1, Closed: true}, true, 1}, // a change for a, the table holds it
		{a, DoorStatusUpdate{ObjectID: 5, DoorID: 1}, false, 2},
		{b, DeleteObject{ObjectID: 5}, false, 2}, // not a door event
	}
	for i, s := range steps {
		s.bot.Observe(s.ev)
		if tbl.Closed(1) != s.closed || e.toggles != s.toggles {
			t.Fatalf("step %d (%+v): Closed %v, toggles %d, want %v, %d", i, s.ev, tbl.Closed(1), e.toggles, s.closed, s.toggles)
		}
	}
	if len(e.bad) > 0 {
		t.Fatal(e.bad)
	}
}

// TestDoorTableReadsNeverTakeTheMutex: Closed answers while the mutex is
// held; a Closed that took it would deadlock here.
func TestDoorTableReadsNeverTakeTheMutex(t *testing.T) {
	tbl := newDoorTable(newRecordingEngine(), flatGround{}, []*door.Template{testDoor(1, false)}, discardLog())
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	if !tbl.Closed(1) {
		t.Fatal("Closed(1) = false, want the closed default")
	}
}

// TestDoorTableConcurrentReports (run under -race): many bots report many
// doors at once while route searches read them. The reports are serialized,
// the engine never gets a toggle that does not change a door, and every
// door's engine state ends equal to the table's.
func TestDoorTableConcurrentReports(t *testing.T) {
	const doors, bots, steps, readers = 6, 16, 300, 4
	templates := make([]*door.Template, doors)
	for d := range templates {
		templates[d] = testDoor(d+1, d%2 == 0)
	}
	e := newRecordingEngine()
	tbl := newDoorTable(e, flatGround{}, templates, discardLog())

	done := make(chan struct{})
	var rd sync.WaitGroup
	for range readers {
		rd.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				for d := 1; d <= doors; d++ {
					tbl.Closed(d)
				}
			}
		})
	}
	var wg sync.WaitGroup
	for bot := range bots {
		wg.Go(func() {
			r := NewDoorReporter(tbl)
			for i := range steps {
				for d := 1; d <= doors; d++ {
					closed := (bot+i/3+d)%2 == 0 // each state seen three times running
					if i%2 == 0 {
						r.Observe(DoorInfo{ObjectID: int32(d), DoorID: int32(d), Closed: closed})
					} else {
						r.Observe(DoorStatusUpdate{ObjectID: int32(d), DoorID: int32(d), Closed: closed})
					}
				}
			}
		})
	}
	wg.Wait()
	close(done)
	rd.Wait()

	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	if len(e.bad) > 0 {
		t.Fatalf("%d redundant toggles, first: %s", len(e.bad), e.bad[0])
	}
	if e.toggles == 0 {
		t.Fatal("no toggle reached the engine")
	}
	for d := 1; d <= doors; d++ {
		if got, want := e.shut[tbl.objs[tbl.slot[d]]], tbl.Closed(d); got != want {
			t.Errorf("door %d: engine closed %v, table closed %v", d, got, want)
		}
	}
}

// TestDoorTableTogglesTheEngine: over real geodata, the corridor door
// blocks its cells while closed and frees them when opened.
func TestDoorTableTogglesTheEngine(t *testing.T) {
	e := scenesEngine(t)
	doors, err := door.NewTable([]*door.Template{testDoor(1, false)})
	if err != nil {
		t.Fatal(err)
	}
	tbl := NewDoorTable(e, doors, discardLog())
	gx := (geotest.RegionX-engine.TileXMin)*block.RegionBlocksX*block.CellsX + 288
	gy := (geotest.RegionY-engine.TileYMin)*block.RegionBlocksY*block.CellsY + 261
	for i, s := range []struct {
		set, closed bool
	}{{true, true}, {false, false}, {false, false}, {true, true}} {
		if i > 0 {
			tbl.Set(1, s.set)
		}
		if blocked := e.NSWENearest(gx, gy, geotest.Ground) == block.NoDirections; blocked != s.closed || tbl.Closed(1) != s.closed {
			t.Fatalf("step %d: door cell blocked %v, Closed %v, want %v", i, blocked, tbl.Closed(1), s.closed)
		}
	}
}

func scenesEngine(tb testing.TB) *engine.Engine {
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
