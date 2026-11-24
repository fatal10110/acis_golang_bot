package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestWalkerBlockedStopAvoidsOneStep blocks the straight segment along the
// open corridor halfway (#307): the server stops before the step that
// failed, so the walk avoids the columns of one step and a cell past the
// stop, not the whole rest of the segment, and the replan still leaves the
// refused row.
func TestWalkerBlockedStopAvoidsOneStep(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.CorridorWest, 0, 0), scenePoint(geotest.CorridorEast, 0, 0)
	b, _, _ := newNavBrain(t, n, Persona{}, from)
	_, to, stop := blockMidway(t, b, dest)
	var rest nav.Avoid
	rest.AddLine(toLoc(stop), toLoc(to))
	got := b.walk.plan.Avoid.Len()
	if steps := int(b.walk.plan.Lmax+16)/16 + 1; got == 0 || got > steps {
		t.Fatalf("a blocked stop on an open segment avoids %d columns, want 1..%d (one step and a cell); the rest of the segment is %d", got, steps, rest.Len())
	}
	if got >= rest.Len() {
		t.Fatalf("avoids %d columns, as many as the rest of the segment (%d)", got, rest.Len())
	}
}
