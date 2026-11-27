package e2e

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// The walker's step-chain timing (botclient walker.go navEmax and
// positionTick): the longest gap between two of the server's position
// updates the walker's check covers, and the update interval.
const (
	navEmax = 200 * time.Millisecond
	navTick = 100 * time.Millisecond
)

// Client opcodes a whole bot sends that start or end a move of its own
// (the server's clientpackets): a ground click, a click on an object, an
// attack and a skill.
const (
	opMoveBackwardToLocation = 0x01
	opAction                 = 0x04
	opAttackRequest          = 0x0a
	opMagicSkillUse          = 0x2f
)

// navVerdict checks one watched bot's walker counters against the e2e
// thresholds of docs/navigation.md (Testing): no desync on door-free
// routes, door replans counted apart, and at most one blocked stop per 100
// segments, so a run under 100 segments may have one.
func navVerdict(s *botmetrics.Snapshot) []string {
	var bad []string
	if s.NavDesync != 0 {
		bad = append(bad, fmt.Sprintf("nav_desync_total = %d, want 0", s.NavDesync))
	}
	if s.NavBlocked*100 > max(s.NavSegments, 100) {
		bad = append(bad, fmt.Sprintf("nav_blocked_total = %d over %d segments, want at most 1 per 100", s.NavBlocked, s.NavSegments))
	}
	return bad
}

// blockedStop is a ground move of the bot the server ended short of its
// destination with its blocked correction (move/controller.go
// BroadcastBlockedCorrection): the move went from from toward to and
// stopped at stop. tick estimates the elapsed time of the step that failed:
// the time from the server's answer to the correction less the run from
// from to stop, or for a correction that answers the click itself, the time
// from the click (round trip included, an upper bound).
type blockedStop struct {
	from, to, stop botclient.Point
	tick           time.Duration
}

// blockedStops reads the blocked stops of the bot self's own ground clicks
// from a Watch's frames, the bot running at speed. A click is followed
// until the bot's first standstill MoveToLocation, a teleport, a StopMove or
// another click of the bot's own; a standstill farther than a cell from the
// clicked point is a blocked stop.
func blockedStops(seen []Seen, sent []Sent, self int32, speed float64) []blockedStop {
	var (
		out                []blockedStop
		active, answered   bool
		from, to           botclient.Point
		sentAt, answeredAt time.Time
	)
	for _, f := range watchFrames(seen, sent) {
		if f.sent != nil {
			switch f.sent.Op {
			case opMoveBackwardToLocation:
				var ok bool
				if to, from, ok = groundClick(f.sent.Payload); !ok {
					continue
				}
				active, answered, sentAt = true, false, f.at
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
			if e.Origin.X != e.Dest.X || e.Origin.Y != e.Dest.Y {
				if !answered {
					answered, answeredAt, from = true, f.at, e.Origin
				}
				continue
			}
			active = false
			if near2(e.Dest, to) <= 16*16 {
				continue
			}
			tick := f.at.Sub(sentAt)
			if answered {
				run := time.Duration(dist(from, e.Dest) / max(speed, 1) * float64(time.Second))
				tick = max(f.at.Sub(answeredAt)-run, 0)
			}
			out = append(out, blockedStop{from: from, to: to, stop: e.Dest, tick: tick})
		case botclient.StopMove:
			if e.ObjectID == self {
				active = false
			}
		case botclient.TeleportToLocation:
			if e.ObjectID == self {
				active = false
			}
		}
	}
	return out
}

// watchFrame is one frame of a Watch: a packet the bot read (ev) or one it
// wrote (sent).
type watchFrame struct {
	at   time.Time
	ev   botclient.Event
	sent *Sent
}

// watchFrames merges a Watch's read and written frames in time order.
func watchFrames(seen []Seen, sent []Sent) []watchFrame {
	frames := make([]watchFrame, 0, len(seen)+len(sent))
	for _, s := range seen {
		frames = append(frames, watchFrame{at: s.At, ev: s.Event})
	}
	for i := range sent {
		frames = append(frames, watchFrame{at: sent[i].At, sent: &sent[i]})
	}
	slices.SortStableFunc(frames, func(a, b watchFrame) int { return a.at.Compare(b.at) })
	return frames
}

// groundClick decodes a MoveBackwardToLocation payload (opcode first): the
// clicked point and the origin the client sent. A payload too short for
// both is no click.
func groundClick(p []byte) (to, from botclient.Point, ok bool) {
	if len(p) < 25 {
		return botclient.Point{}, botclient.Point{}, false
	}
	at := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
	return botclient.Point{X: at(1), Y: at(5), Z: at(9)}, botclient.Point{X: at(13), Y: at(17), Z: at(21)}, true
}

// toLocation is p as a geodata location.
func toLocation(p botclient.Point) location.Location {
	return location.Location{X: int(p.X), Y: int(p.Y), Z: int(p.Z)}
}

// near2 is the squared horizontal distance between a and b.
func near2(a, b botclient.Point) int64 {
	dx, dy := int64(a.X-b.X), int64(a.Y-b.Y)
	return dx*dx + dy*dy
}

// stepPredicted reports whether the server's step chain, run on e from the
// stop toward the move's destination at speed, fails its next step for the
// tick seen or a nominal one: the step that failed on the server, as the
// bot's geodata predicts it.
func stepPredicted(e *engine.Engine, b blockedStop, speed float64) bool {
	for _, d := range []time.Duration{cmp.Or(b.tick, navTick), navTick} {
		if e.WalkSteps(toLocation(b.stop), toLocation(b.to), speed, speed, []time.Duration{d}).Blocked {
			return true
		}
	}
	return false
}

// stepVerdict sorts blocked stops as the step-chain check of
// docs/navigation.md (Testing) asks: a stop WalkSteps predicts is
// explained, so is one after a step longer than Emax on a segment that
// fails the open-corridor test (a server stall, which only stretches that
// need the exact check are exposed to; open corridors are immune to it);
// any other is a stop the bot's step chain did not foresee.
func stepVerdict(stops []blockedStop, predicted, corridorOpen func(blockedStop) bool) (explained []string, bad []string) {
	for _, b := range stops {
		switch {
		case predicted(b):
			explained = append(explained, fmt.Sprintf("stop at %v on %v -> %v: WalkSteps predicts it (tick %v)", b.stop, b.from, b.to, b.tick))
		case b.tick > navEmax && !corridorOpen(b):
			explained = append(explained, fmt.Sprintf("stop at %v on %v -> %v: a step of %v, longer than Emax, off an open corridor", b.stop, b.from, b.to, b.tick))
		default:
			bad = append(bad, fmt.Sprintf("stop at %v on %v -> %v after a step of %v: WalkSteps did not predict it", b.stop, b.from, b.to, b.tick))
		}
	}
	return explained, bad
}

// lastUserInfo is the last UserInfo among the frames seen.
func lastUserInfo(seen []Seen) (botclient.UserInfo, bool) {
	for i := len(seen) - 1; i >= 0; i-- {
		if u, ok := seen[i].Event.(botclient.UserInfo); ok {
			return u, true
		}
	}
	return botclient.UserInfo{}, false
}

// objectAt is where object id stood at at, as the frames seen tell: the
// position of its last NPCInfo, StopMove, ValidateLocation or
// TeleportToLocation, or the point its last MoveToLocation reached by then
// at the speed of its NPCInfo (running or walking, as that NPCInfo says).
func objectAt(seen []Seen, id int32, at time.Time) (botclient.Point, bool) {
	var (
		pos, dest botclient.Point
		movedAt   time.Time
		speed     float64
		known     bool
	)
	for _, s := range seen {
		if s.At.After(at) {
			break
		}
		switch e := s.Event.(type) {
		case botclient.NPCInfo:
			if e.ObjectID == id {
				pos, dest, known = e.Pos, e.Pos, true
				speed = float64(e.WalkSpeed)
				if e.Running {
					speed = float64(e.RunSpeed)
				}
			}
		case botclient.MoveToLocation:
			if e.ObjectID == id {
				pos, dest, movedAt, known = e.Origin, e.Dest, s.At, true
			}
		case botclient.StopMove:
			if e.ObjectID == id {
				pos, dest, known = e.Pos, e.Pos, true
			}
		case botclient.ValidateLocation:
			if e.ObjectID == id {
				pos, dest, known = e.Pos, e.Pos, true
			}
		case botclient.TeleportToLocation:
			if e.ObjectID == id {
				pos, dest, known = e.Pos, e.Pos, true
			}
		}
	}
	if !known || pos == dest {
		return dest, known
	}
	whole := dist(pos, dest)
	run := speed * at.Sub(movedAt).Seconds()
	if speed <= 0 || run >= whole {
		return dest, true
	}
	f := run / whole
	lerp := func(a, b int32) int32 { return a + int32(math.Round(float64(b-a)*f)) }
	return botclient.Point{X: lerp(pos.X, dest.X), Y: lerp(pos.Y, dest.Y), Z: lerp(pos.Z, dest.Z)}, true
}

func TestNavVerdict(t *testing.T) {
	tests := []struct {
		name                       string
		blocked, desync, door, seg uint64
		bad                        int
	}{
		{name: "clean", seg: 40},
		{name: "one blocked under 100 segments", blocked: 1, seg: 12},
		{name: "two blocked under 100 segments", blocked: 2, seg: 60, bad: 1},
		{name: "two blocked over 200 segments", blocked: 2, seg: 200},
		{name: "three blocked over 250 segments", blocked: 3, seg: 250, bad: 1},
		{name: "door replans apart", door: 5, seg: 30},
		{name: "a desync", desync: 1, seg: 30, bad: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &botmetrics.Snapshot{NavBlocked: tt.blocked, NavDesync: tt.desync, NavDoorReplan: tt.door, NavSegments: tt.seg}
			if got := navVerdict(s); len(got) != tt.bad {
				t.Errorf("navVerdict = %q, want %d findings", got, tt.bad)
			}
		})
	}
}

func TestBlockedStops(t *testing.T) {
	const self, other = 7, 8
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	from, to := botclient.Point{X: 0, Y: 0, Z: -3000}, botclient.Point{X: 1000, Y: 0, Z: -3000}
	stop := botclient.Point{X: 520, Y: 0, Z: -3000}
	pt := func(x int32) botclient.Point { return botclient.Point{X: x, Y: 0, Z: -3000} }
	click := Sent{At: at(0), Op: opMoveBackwardToLocation, Payload: botwire.EncodeMoveBackwardToLocation(to, from)}
	move := func(ms int, id int32, o, d botclient.Point) Seen {
		return Seen{At: at(ms), Event: botclient.MoveToLocation{ObjectID: id, Origin: o, Dest: d}}
	}
	tests := []struct {
		name string
		seen []Seen
		sent []Sent
		want []blockedStop
	}{
		{
			name: "arrived",
			seen: []Seen{move(50, self, from, to), move(9050, self, to, to)},
			sent: []Sent{click},
		},
		{
			name: "stopped short after the answer",
			seen: []Seen{move(50, self, from, to), move(4300, self, stop, stop)},
			sent: []Sent{click},
			// 520 at 130 a second is 4000 ms after the answer at 50.
			want: []blockedStop{{from: from, to: to, stop: stop, tick: 250 * time.Millisecond}},
		},
		{
			name: "refused at the click",
			seen: []Seen{move(60, self, from, from)},
			sent: []Sent{click},
			want: []blockedStop{{from: from, to: to, stop: from, tick: 60 * time.Millisecond}},
		},
		{
			name: "another creature's stop",
			seen: []Seen{move(50, self, from, to), move(4300, other, stop, stop)},
			sent: []Sent{click},
		},
		{
			name: "an attack ends the click",
			seen: []Seen{move(50, self, from, to), move(4300, self, stop, stop)},
			sent: []Sent{click, {At: at(1000), Op: opAttackRequest}},
		},
		{
			name: "a teleport ends the click",
			seen: []Seen{move(50, self, from, to), {At: at(2000), Event: botclient.TeleportToLocation{ObjectID: self, Pos: stop}}, move(4300, self, stop, stop)},
			sent: []Sent{click},
		},
		{
			name: "a second click supersedes the first",
			// The second click runs from x130 toward x500; its answer at
			// 1050, then 130 at 130 a second, stops at x260 at 2300.
			seen: []Seen{move(50, self, from, to), move(1050, self, pt(130), pt(500)), move(2300, self, pt(260), pt(260))},
			sent: []Sent{click, {At: at(1000), Op: opMoveBackwardToLocation, Payload: botwire.EncodeMoveBackwardToLocation(pt(500), pt(130))}},
			want: []blockedStop{{from: pt(130), to: pt(500), stop: pt(260), tick: 250 * time.Millisecond}},
		},
		{
			name: "a short click payload is no click",
			seen: []Seen{move(50, self, from, to), move(4300, self, stop, stop)},
			sent: []Sent{{At: at(0), Op: opMoveBackwardToLocation, Payload: click.Payload[:24]}},
		},
		{
			name: "a StopMove ends the click",
			seen: []Seen{move(50, self, from, to), {At: at(2000), Event: botclient.StopMove{ObjectID: self, Pos: stop}}, move(4300, self, stop, stop)},
			sent: []Sent{click},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blockedStops(tt.seen, tt.sent, self, 130)
			if !slices.Equal(got, tt.want) {
				t.Errorf("blockedStops = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStepVerdict(t *testing.T) {
	tests := []struct {
		name             string
		tick             time.Duration
		predicted, open  bool
		explained, isBad bool
	}{
		{name: "predicted", tick: 100 * time.Millisecond, predicted: true, explained: true},
		{name: "long step off an open corridor", tick: 350 * time.Millisecond, explained: true},
		{name: "long step on an open corridor", tick: 350 * time.Millisecond, open: true, isBad: true},
		{name: "short step not predicted", tick: 120 * time.Millisecond, isBad: true},
		{name: "short step on an open corridor", tick: 120 * time.Millisecond, open: true, isBad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stops := []blockedStop{{stop: botclient.Point{X: 1}, tick: tt.tick}}
			explained, bad := stepVerdict(stops,
				func(blockedStop) bool { return tt.predicted },
				func(blockedStop) bool { return tt.open })
			if (len(explained) == 1) != tt.explained || (len(bad) == 1) != tt.isBad {
				t.Errorf("explained %q, bad %q; want explained %v, bad %v", explained, bad, tt.explained, tt.isBad)
			}
		})
	}
}

func TestObjectAt(t *testing.T) {
	const id, other = 900, 901
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	home := botclient.Point{X: 0, Y: 0, Z: -3000}
	away := botclient.Point{X: 100, Y: 0, Z: -3000}
	info := Seen{At: at(0), Event: botclient.NPCInfo{ObjectID: id, Pos: home, WalkSpeed: 50, RunSpeed: 100}}
	walk := Seen{At: at(1000), Event: botclient.MoveToLocation{ObjectID: id, Origin: home, Dest: away}}
	tests := []struct {
		name string
		seen []Seen
		when int
		want botclient.Point
		ok   bool
	}{
		{name: "unseen", seen: []Seen{{At: at(0), Event: botclient.NPCInfo{ObjectID: other, Pos: away}}}, when: 500},
		{name: "spawned", seen: []Seen{info}, when: 500, want: home, ok: true},
		{name: "half way at walking speed", seen: []Seen{info, walk}, when: 2000, want: botclient.Point{X: 50, Y: 0, Z: -3000}, ok: true},
		{name: "arrived", seen: []Seen{info, walk}, when: 4000, want: away, ok: true},
		{name: "a frame after the time", seen: []Seen{info, walk}, when: 900, want: home, ok: true},
		{name: "stopped", seen: []Seen{info, walk, {At: at(1500), Event: botclient.StopMove{ObjectID: id, Pos: botclient.Point{X: 20, Y: 0, Z: -3000}}}}, when: 3000, want: botclient.Point{X: 20, Y: 0, Z: -3000}, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := objectAt(tt.seen, id, at(tt.when))
			if ok != tt.ok || got != tt.want {
				t.Errorf("objectAt = %v %v, want %v %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
