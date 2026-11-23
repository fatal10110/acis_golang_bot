package botclient

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestWalkerLogsDesync logs a segment the server answered with another
// destination: the segment's ends and the server's answer, so a failing run
// names the leg its check got wrong (#246). An answer toward the segment's
// own end logs nothing.
func TestWalkerLogsDesync(t *testing.T) {
	n := testNav(t)
	from, dest := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	tests := []struct {
		name string
		// reply answers the segment from origin to to.
		reply func(origin, to Point) MoveToLocation
		// want is the log line the answer writes, given the segment and
		// its answer; nil writes none.
		want func(origin, to Point, answer MoveToLocation) string
	}{
		{
			name: "another destination logs the segment and the answer",
			reply: func(o, to Point) MoveToLocation {
				other := Point{X: o.X + (to.X-o.X)/3, Y: o.Y + (to.Y-o.Y)/3, Z: o.Z}
				return MoveToLocation{ObjectID: selfID, Origin: o, Dest: other}
			},
			want: func(o, to Point, a MoveToLocation) string {
				return fmt.Sprintf("level=WARN msg=\"nav desync\" from=%d,%d,%d to=%d,%d,%d server_origin=%d,%d,%d server_dest=%d,%d,%d\n",
					o.X, o.Y, o.Z, to.X, to.Y, to.Z, a.Origin.X, a.Origin.Y, a.Origin.Z, a.Dest.X, a.Dest.Y, a.Dest.Z)
			},
		},
		{
			name:  "the segment's own end logs nothing",
			reply: segAnswer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := newNavBrain(t, n, Persona{}, from)
			var logs bytes.Buffer
			b.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{}
					}
					return a
				},
			}))
			a, _ := walkStep(t, b, dest, t0)
			o, to := moveOrigin(a.payload), moveTarget(a.payload)
			answer := tt.reply(o, to)
			b.handle([]Event{answer}, t0)
			var want string
			if tt.want != nil {
				want = tt.want(o, to, answer)
			}
			if got := logs.String(); got != want {
				t.Errorf("log %q, want %q", got, want)
			}
		})
	}
}
