package botclient

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/dynamic"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// doorEngine is the geodata writer path the door table toggles doors
// through: *engine.Engine, or a recording fake in tests.
type doorEngine interface {
	AddObject(dynamic.Object)
	RemoveObject(dynamic.Object)
}

// DoorTable is the process-wide door state every bot shares
// (docs/navigation.md, "Door table"). Set is serialized by one mutex and
// toggles the door in the shared engine; Closed reads a per-door atomic and
// never takes the mutex, so a route search never waits on a report.
//
// It is best effort: sessions lag differently and there is no server order
// across them, so a slow bot can set an older state after a fast bot set a
// newer one. A door left in the wrong state costs a door replan.
type DoorTable struct {
	engine doorEngine
	log    *slog.Logger
	slot   map[int]int      // door id to slot; read-only after newDoorTable
	objs   []dynamic.Object // per slot; read-only after newDoorTable
	closed []atomic.Bool    // per slot; stored only under mu, loaded lock-free
	// changes counts the door state changes Set applied to the engine. The
	// table owns it: Set adds one under mu after the engine toggle and the
	// store, and every brain goroutine loads it lock-free (Changes).
	changes atomic.Uint64

	mu      sync.Mutex       // serializes Set: the engine toggle, the store, then changes
	unknown map[int]struct{} // guarded by mu: ids already logged as not loaded
}

// NewDoorTable builds the door table over e from the doors.xml templates
// and closes in e every door closed by default, as the server spawns each
// door in its template state. A door whose footprint cannot be built is
// logged and left out, as navbuild leaves it out of the hierarchy.
func NewDoorTable(e *engine.Engine, doors *door.Table, log *slog.Logger) *DoorTable {
	return newDoorTable(e, e.Static(), doors.All(), log)
}

// newDoorTable is NewDoorTable over any writer path, the footprints sampled
// from static.
func newDoorTable(e doorEngine, static dynamic.Sampler, templates []*door.Template, log *slog.Logger) *DoorTable {
	t := &DoorTable{
		engine:  e,
		log:     log,
		slot:    make(map[int]int, len(templates)),
		objs:    make([]dynamic.Object, 0, len(templates)),
		unknown: make(map[int]struct{}),
	}
	shut := make([]bool, 0, len(templates))
	for _, tmpl := range templates {
		obj, err := dynamic.NewDoorObject(tmpl, static)
		if err != nil {
			log.Warn("botclient: door left out of the door table", "door", tmpl.ID, "err", err)
			continue
		}
		t.slot[tmpl.ID] = len(t.objs)
		t.objs = append(t.objs, obj)
		shut = append(shut, !tmpl.Opened)
	}
	t.closed = make([]atomic.Bool, len(t.objs))
	for i, c := range shut {
		if c {
			e.AddObject(t.objs[i])
			t.closed[i].Store(true)
		}
	}
	return t
}

// Closed reports whether door id is closed. A door the table does not hold
// reads as open: the hierarchy holds no edge through it either. It never
// blocks, so it is the door state a route search passes to nav.
func (t *DoorTable) Closed(id int) bool {
	i, ok := t.slot[id]
	return ok && t.closed[i].Load()
}

// Set records door id as closed or open and toggles it in the engine when
// its state changes; a state it already holds is dropped, so the engine
// never gets the same toggle twice. An id the table does not hold (not in
// doors.xml, or its footprint left out) is logged once and dropped.
func (t *DoorTable) Set(id int, closed bool) {
	i, ok := t.slot[id]
	t.mu.Lock()
	defer t.mu.Unlock()
	if !ok {
		if _, logged := t.unknown[id]; !logged {
			t.unknown[id] = struct{}{}
			t.log.Info("botclient: door not in the door table", "door", id)
		}
		return
	}
	if t.closed[i].Load() == closed {
		return
	}
	if closed {
		t.engine.AddObject(t.objs[i])
	} else {
		t.engine.RemoveObject(t.objs[i])
	}
	t.closed[i].Store(closed)
	t.changes.Add(1)
}

// Changes is the number of door state changes Set applied to the engine so
// far; the doors closed at construction count none. It never blocks. A
// reader that loads it before a route search and finds it unchanged later
// knows every change applied since was applied before the search read the
// engine: Set toggles the engine before it counts the change (#566).
func (t *DoorTable) Changes() uint64 {
	return t.changes.Load()
}

// DoorReporter is one bot's door reports, owned by its brain goroutine. It
// forwards a door to the shared table only when the state the bot sees
// differs from what that bot saw last for the door, which keeps repeats
// (every DoorInfo, then its DoorStatusUpdate) off the table mutex.
type DoorReporter struct {
	table *DoorTable
	seen  map[int32]bool // door id to the state this bot saw last
}

// NewDoorReporter returns a reporter for one bot into table.
func NewDoorReporter(table *DoorTable) *DoorReporter {
	return &DoorReporter{table: table, seen: make(map[int32]bool)}
}

// Observe reports the door state ev carries, if ev is a DoorInfo or a
// DoorStatusUpdate, and ignores every other event.
func (r *DoorReporter) Observe(ev Event) {
	switch ev := ev.(type) {
	case DoorInfo:
		r.report(ev.DoorID, ev.Closed)
	case DoorStatusUpdate:
		r.report(ev.DoorID, ev.Closed)
	}
}

func (r *DoorReporter) report(id int32, closed bool) {
	if last, ok := r.seen[id]; ok && last == closed {
		return
	}
	r.seen[id] = closed
	r.table.Set(int(id), closed)
}

// Crosses reports whether the segment from a to b passes over a door's
// footprint, one cell of margin around both: the server's answer to such a
// segment that differs from the bot's check is a door replan
// (docs/navigation.md, "Server answers to its own moves"). It reads only
// what newDoorTable built, so it never blocks.
func (t *DoorTable) Crosses(a, b Point) bool {
	ax, ay := engine.GeoX(int(a.X)), engine.GeoY(int(a.Y))
	bx, by := engine.GeoX(int(b.X)), engine.GeoY(int(b.Y))
	x0, x1 := min(ax, bx)-1, max(ax, bx)+1
	y0, y1 := min(ay, by)-1, max(ay, by)+1
	for _, o := range t.objs {
		data := o.GeoData()
		if len(data) == 0 {
			continue
		}
		ox, oy := o.GeoX(), o.GeoY()
		if ox > x1 || ox+len(data)-1 < x0 || oy > y1 || oy+len(data[0])-1 < y0 {
			continue
		}
		if segmentNearBox(ax, ay, bx, by, ox-1, oy-1, ox+len(data), oy+len(data[0])) {
			return true
		}
	}
	return false
}

// segmentNearBox reports whether the segment from (ax, ay) to (bx, by), in
// cells, touches the box from (x0, y0) to (x1, y1): sampled every half cell.
func segmentNearBox(ax, ay, bx, by, x0, y0, x1, y1 int) bool {
	n := 2 * max(abs(bx-ax), abs(by-ay))
	for i := 0; i <= n; i++ {
		f := 0.0
		if n > 0 {
			f = float64(i) / float64(n)
		}
		x := float64(ax) + f*float64(bx-ax)
		y := float64(ay) + f*float64(by-ay)
		if x >= float64(x0) && x <= float64(x1) && y >= float64(y0) && y <= float64(y1) {
			return true
		}
	}
	return false
}
