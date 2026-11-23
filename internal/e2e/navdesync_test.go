package e2e

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// clickSlop is how far, horizontally, the server's answer to a ground click
// may end from the clicked point and still be the click's own destination:
// one geodata cell, the walker's walkSlop.
const clickSlop = 16

// desync is a ground click of the bot the server answered toward another
// destination (docs/navigation.md, "Server answers to its own moves"): the
// click went from from toward to, and the server's move started at origin
// toward dest, a routed first leg or the last point it reaches.
type desync struct {
	from, to, origin, dest botclient.Point
}

// desyncs reads the desyncs of the bot self's own ground clicks from a
// Watch's frames: a click's answer is the bot's first MoveToLocation after
// it; one that moves toward a point farther than a cell from the clicked
// one is a desync. A standstill answer is a blocked stop (blockedStops),
// and another click of the bot's own, a StopMove or a teleport ends the
// wait for the answer.
func desyncs(seen []Seen, sent []Sent, self int32) []desync {
	var (
		out      []desync
		active   bool
		from, to botclient.Point
	)
	for _, f := range watchFrames(seen, sent) {
		if f.sent != nil {
			switch f.sent.Op {
			case opMoveBackwardToLocation:
				to, from, active = groundClick(f.sent.Payload)
			case opAction, opAttackRequest, opMagicSkillUse:
				active = false
			}
			continue
		}
		switch e := f.ev.(type) {
		case botclient.MoveToLocation:
			if !active || e.ObjectID != self {
				continue
			}
			active = false
			if (e.Origin.X != e.Dest.X || e.Origin.Y != e.Dest.Y) && near2(e.Dest, to) > clickSlop*clickSlop {
				out = append(out, desync{from: from, to: to, origin: e.Origin, dest: e.Dest})
			}
		case botclient.StopMove:
			active = active && e.ObjectID != self
		case botclient.TeleportToLocation:
			active = active && e.ObjectID != self
		}
	}
	return out
}

// desyncCause names a desync and where its cause lies, read on the bot's
// own geodata (canMove, doors included) as the server reads a click: from
// the position it holds the bot at, toward the clicked point at head level
// (the floor Z plus the character's collision height, network/movement.go;
// move/creature.go moveToLocationLocked routes only a line CanMove closes).
// door reports whether the click's line crosses a door cell (a door
// replan, which the walker counts apart).
func desyncCause(d desync, collision int32, door, canMove func(a, b botclient.Point) bool) string {
	head := d.to
	head.Z += collision
	var cause string
	switch {
	case door != nil && door(d.from, d.to):
		cause = "the line crosses a door cell: a door replan, counted apart"
	case near2(d.origin, d.from) > clickSlop*clickSlop:
		cause = "the server held the bot elsewhere than the click's origin: the click went out while the server still moved it, or on a stale position"
	case !canMove(d.from, d.to):
		cause = "the bot's geodata closes the click's own line: not a segment the walker checked"
	case !canMove(d.origin, head):
		cause = "the bot's geodata closes the line from the server's origin to the head-level target: the walker's check differs from the server's inputs (height or layer)"
	default:
		cause = "the bot's geodata opens the line from the server's origin: the server's geodata or a dynamic object (a GM fence, a door state) differs from the bot's on this line"
	}
	return fmt.Sprintf("desync on %v -> %v: the server moved from %v toward %v; %s", d.from, d.to, d.origin, d.dest, cause)
}

func TestDesyncs(t *testing.T) {
	const self, other = 7, 8
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	pt := func(x, y int32) botclient.Point { return botclient.Point{X: x, Y: y, Z: -3000} }
	from, to, corner := pt(0, 0), pt(1000, 0), pt(400, 300)
	click := Sent{At: at(0), Op: opMoveBackwardToLocation, Payload: botclient.EncodeMoveBackwardToLocation(to, from)}
	move := func(ms int, id int32, o, d botclient.Point) Seen {
		return Seen{At: at(ms), Event: botclient.MoveToLocation{ObjectID: id, Origin: o, Dest: d}}
	}
	tests := []struct {
		name string
		seen []Seen
		sent []Sent
		want []desync
	}{
		{
			name: "answered toward the click",
			seen: []Seen{move(50, self, from, botclient.Point{X: 1000, Y: 10, Z: -2950})},
			sent: []Sent{click},
		},
		{
			name: "routed around",
			seen: []Seen{move(50, self, from, corner), move(4000, self, corner, to)},
			sent: []Sent{click},
			want: []desync{{from: from, to: to, origin: from, dest: corner}},
		},
		{
			name: "answered from another origin",
			seen: []Seen{move(50, self, pt(40, 0), corner)},
			sent: []Sent{click},
			want: []desync{{from: from, to: to, origin: pt(40, 0), dest: corner}},
		},
		{
			name: "a standstill answer is a blocked stop",
			seen: []Seen{move(50, self, from, from)},
			sent: []Sent{click},
		},
		{
			name: "another creature's move",
			seen: []Seen{move(50, other, from, corner)},
			sent: []Sent{click},
		},
		{
			name: "an attack ends the wait",
			seen: []Seen{move(1050, self, from, corner)},
			sent: []Sent{click, {At: at(1000), Op: opAttackRequest}},
		},
		{
			name: "a StopMove ends the wait",
			seen: []Seen{{At: at(20), Event: botclient.StopMove{ObjectID: self, Pos: from}}, move(50, self, from, corner)},
			sent: []Sent{click},
		},
		{
			name: "a teleport ends the wait",
			seen: []Seen{{At: at(20), Event: botclient.TeleportToLocation{ObjectID: self, Pos: from}}, move(50, self, from, corner)},
			sent: []Sent{click},
		},
		{
			name: "a short click payload is no click",
			seen: []Seen{move(50, self, from, corner)},
			sent: []Sent{{At: at(0), Op: opMoveBackwardToLocation, Payload: click.Payload[:24]}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desyncs(tt.seen, tt.sent, self); !slices.Equal(got, tt.want) {
				t.Errorf("desyncs = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDesyncCause(t *testing.T) {
	pt := func(x, y, z int32) botclient.Point { return botclient.Point{X: x, Y: y, Z: z} }
	d := desync{from: pt(0, 0, -3000), to: pt(1000, 0, -3000), origin: pt(0, 0, -3000), dest: pt(400, 300, -3000)}
	head := pt(1000, 0, -2976)
	// open is a geodata whose only closed lines are closed.
	open := func(closed ...[2]botclient.Point) func(a, b botclient.Point) bool {
		return func(a, b botclient.Point) bool { return !slices.Contains(closed, [2]botclient.Point{a, b}) }
	}
	const prefix = "desync on {0 0 -3000} -> {1000 0 -3000}: the server moved from {0 0 -3000} toward {400 300 -3000}; "
	tests := []struct {
		name    string
		d       desync
		door    func(a, b botclient.Point) bool
		canMove func(a, b botclient.Point) bool
		want    string
	}{
		{
			name:    "open on the bot's geodata",
			d:       d,
			canMove: open(),
			want:    prefix + "the bot's geodata opens the line from the server's origin: the server's geodata or a dynamic object (a GM fence, a door state) differs from the bot's on this line",
		},
		{
			name:    "a door cell",
			d:       d,
			door:    func(a, b botclient.Point) bool { return a == d.from && b == d.to },
			canMove: open(),
			want:    prefix + "the line crosses a door cell: a door replan, counted apart",
		},
		{
			name:    "closed at head level from the server's origin",
			d:       d,
			canMove: open([2]botclient.Point{d.origin, head}),
			want:    prefix + "the bot's geodata closes the line from the server's origin to the head-level target: the walker's check differs from the server's inputs (height or layer)",
		},
		{
			name:    "closed from the click's own origin",
			d:       d,
			canMove: open([2]botclient.Point{d.from, d.to}),
			want:    prefix + "the bot's geodata closes the click's own line: not a segment the walker checked",
		},
		{
			name:    "another origin",
			d:       desync{from: d.from, to: d.to, origin: pt(40, 0, -3000), dest: d.dest},
			canMove: open(),
			want: "desync on {0 0 -3000} -> {1000 0 -3000}: the server moved from {40 0 -3000} toward {400 300 -3000}; " +
				"the server held the bot elsewhere than the click's origin: the click went out while the server still moved it, or on a stale position",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desyncCause(tt.d, 24, tt.door, tt.canMove); got != tt.want {
				t.Errorf("desyncCause =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
