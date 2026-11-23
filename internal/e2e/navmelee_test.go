package e2e

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// meleeRun is how a bot came to attack a monster: the Action that selected
// it (sent at Select, from Decided: where the bot stood when it chose the
// monster), the attack click that followed (Click, from Origin), and the
// ground moves it sent in between.
type meleeRun struct {
	Select  time.Time
	Decided botclient.Point
	Click   Sent
	Origin  botclient.Point
	Moves   int
}

// meleeApproach is the bot's first attack on object id at or after since,
// with the selection it came from. The packets sent and the events seen are
// replayed in the order the watch recorded them (Seen.SentBefore): an
// Action on id while id is the bot's target (its MyTargetSelected, not
// cleared by its own TargetUnselected since) is an attack click, as is an
// AttackRequest; any other Action on id selects it, and the ground moves
// are counted from the latest such selection. self is the bot's object id.
func meleeApproach(seen []Seen, sent []Sent, self, id int32, since time.Time) (meleeRun, bool) {
	var (
		r        meleeRun
		selected bool // a selecting Action on id went out since
		target   int32
		next     int // index in seen of the next event to apply
	)
	for j, s := range sent {
		for ; next < len(seen) && seen[next].SentBefore <= j; next++ {
			switch e := seen[next].Event.(type) {
			case botclient.MyTargetSelected:
				target = e.ObjectID
			case botclient.TargetUnselected:
				if e.ObjectID == self {
					target = 0
				}
			}
		}
		if s.At.Before(since) {
			continue
		}
		switch {
		case s.Op == opMoveBackwardToLocation:
			r.Moves++
		case (s.Op == opAction || s.Op == opAttackRequest) && len(s.Payload) >= 17 && s.Target == id:
			p := s.Payload
			at := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
			origin := botclient.Point{X: at(5), Y: at(9), Z: at(13)}
			if target == id && selected {
				r.Click, r.Origin = s, origin
				return r, true
			}
			if s.Op == opAction {
				r = meleeRun{Select: s.At, Decided: origin}
				selected = true
			}
		}
	}
	return meleeRun{}, false
}

// meleeVerdict judges an attack round the obstacle: r is the approach, the
// monster stood at chosen when the bot selected it and at there when it
// clicked to attack, and canMove is the bot's geodata straight walk. fault
// is the bot's error: an attack from a point with no straight walk to the
// monster, or, from a choice made behind the obstacle, one without a ground
// move round it. lost says why the round could not test the walk round: the
// bot chose the monster from an open line (something moved it there first:
// a fight with another monster, a drift), or the monster walked round to
// the bot itself.
func meleeVerdict(r meleeRun, chosen, there botclient.Point, canMove func(p, q botclient.Point) bool) (fault, lost string) {
	if !canMove(r.Origin, there) {
		return fmt.Sprintf("attacked the monster at %v from %v: no straight walk, no stand point", there, r.Origin), ""
	}
	switch {
	case r.Moves > 0:
		return "", ""
	case canMove(r.Decided, chosen):
		return "", fmt.Sprintf("chose the monster at %v from %v, an open line: no walk round to test", chosen, r.Decided)
	case near2(chosen, there) > 16*16:
		return "", fmt.Sprintf("the monster walked from %v to %v, an open line from %v", chosen, there, r.Origin)
	}
	return fmt.Sprintf("chose the monster at %v from %v behind the obstacle and attacked it from %v without a ground move round it", chosen, r.Decided, r.Origin), ""
}

func TestMeleeApproach(t *testing.T) {
	const self, wolf, other = 100, 900, 901
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	stand := botclient.Point{X: -83064, Y: 249960, Z: -3633}
	round := botclient.Point{X: -83200, Y: 250000, Z: -3640}
	pkt := func(ms int, op byte, id int32, from botclient.Point) Sent {
		p := botclient.EncodeAction(id, from)
		if op == opAttackRequest {
			p = botclient.EncodeAttackRequest(id, from)
		}
		return Sent{At: at(ms), Op: op, Target: id, Payload: p}
	}
	move := func(ms int) Sent {
		return Sent{At: at(ms), Op: opMoveBackwardToLocation, Payload: botclient.EncodeMoveBackwardToLocation(round, stand)}
	}
	// ev is an event read after the first before packets were sent.
	ev := func(ms, before int, e botclient.Event) Seen { return Seen{At: at(ms), Event: e, SentBefore: before} }
	tests := []struct {
		name string
		seen []Seen
		sent []Sent
		want meleeRun
		ok   bool
	}{
		{
			name: "select, walk round, attack",
			seen: []Seen{ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{pkt(200, opAction, wolf, stand), move(300), move(900), pkt(1500, opAction, wolf, round)},
			want: meleeRun{Select: at(200), Decided: stand, Click: pkt(1500, opAction, wolf, round), Origin: round, Moves: 2},
			ok:   true,
		},
		{
			name: "an attack request once selected",
			seen: []Seen{ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{pkt(200, opAction, wolf, stand), pkt(400, opAttackRequest, wolf, stand)},
			want: meleeRun{Select: at(200), Decided: stand, Click: pkt(400, opAttackRequest, wolf, stand), Origin: stand},
			ok:   true,
		},
		{
			name: "moves before since and before the selection are not counted",
			seen: []Seen{ev(250, 3, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{move(50), move(150), pkt(200, opAction, wolf, stand), pkt(400, opAction, wolf, stand)},
			want: meleeRun{Select: at(200), Decided: stand, Click: pkt(400, opAction, wolf, stand), Origin: stand},
			ok:   true,
		},
		{
			// The bot chose the wolf, fought another monster that engaged
			// it and chose the wolf again from where that fight left it.
			name: "a second selection after another target",
			seen: []Seen{
				ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf}),
				ev(350, 1, botclient.MyTargetSelected{ObjectID: other}),
				ev(2050, 4, botclient.MyTargetSelected{ObjectID: wolf}),
			},
			sent: []Sent{pkt(200, opAction, wolf, stand), pkt(400, opAction, other, stand), move(500), pkt(2000, opAction, wolf, round), pkt(2100, opAction, wolf, round)},
			want: meleeRun{Select: at(2000), Decided: round, Click: pkt(2100, opAction, wolf, round), Origin: round},
			ok:   true,
		},
		{
			name: "the selection read only after the click: no attack",
			seen: []Seen{ev(450, 2, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{pkt(200, opAction, wolf, stand), pkt(400, opAction, wolf, stand)},
		},
		{
			name: "the target cleared: the next Action selects again",
			seen: []Seen{
				ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf}),
				ev(300, 1, botclient.TargetUnselected{ObjectID: self}),
				ev(450, 2, botclient.MyTargetSelected{ObjectID: wolf}),
			},
			sent: []Sent{pkt(200, opAction, wolf, stand), pkt(400, opAction, wolf, round), pkt(500, opAction, wolf, round)},
			want: meleeRun{Select: at(400), Decided: round, Click: pkt(500, opAction, wolf, round), Origin: round},
			ok:   true,
		},
		{
			name: "another object's TargetUnselected keeps the target",
			seen: []Seen{
				ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf}),
				ev(300, 1, botclient.TargetUnselected{ObjectID: other}),
			},
			sent: []Sent{pkt(200, opAction, wolf, stand), pkt(400, opAction, wolf, stand)},
			want: meleeRun{Select: at(200), Decided: stand, Click: pkt(400, opAction, wolf, stand), Origin: stand},
			ok:   true,
		},
		{
			name: "selected before since: the click selects nothing new",
			seen: []Seen{ev(50, 1, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{pkt(20, opAction, wolf, stand), pkt(300, opAction, wolf, stand)},
		},
		{
			name: "a short payload",
			seen: []Seen{ev(250, 1, botclient.MyTargetSelected{ObjectID: wolf})},
			sent: []Sent{pkt(200, opAction, wolf, stand), {At: at(300), Op: opAction, Target: wolf, Payload: pkt(0, opAction, wolf, stand).Payload[:16]}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := meleeApproach(tt.seen, tt.sent, self, wolf, at(100))
			if ok != tt.ok || ok && (!got.Select.Equal(tt.want.Select) || got.Decided != tt.want.Decided ||
				!got.Click.At.Equal(tt.want.Click.At) || got.Click.Op != tt.want.Click.Op || got.Origin != tt.want.Origin || got.Moves != tt.want.Moves) {
				t.Errorf("meleeApproach = %+v %v, want %+v %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestMeleeVerdict(t *testing.T) {
	// The line is closed between the two sides of x = 0, open within a
	// side.
	canMove := func(p, q botclient.Point) bool { return (p.X < 0) == (q.X < 0) }
	behind := botclient.Point{X: 10, Y: 0}
	mob := botclient.Point{X: -10, Y: 0}
	beside := botclient.Point{X: -40, Y: 30}
	tests := []struct {
		name          string
		r             meleeRun
		chosen, there botclient.Point
		fault, lost   bool
	}{
		{name: "walked round from behind", r: meleeRun{Decided: behind, Origin: beside, Moves: 3}, chosen: mob, there: mob},
		{name: "attacked through the obstacle after a walk", r: meleeRun{Decided: behind, Origin: behind, Moves: 1}, chosen: mob, there: mob, fault: true},
		{name: "attacked through the obstacle without a walk", r: meleeRun{Decided: behind, Origin: behind}, chosen: mob, there: mob, fault: true},
		// #273: the bot stood behind the obstacle when the wolf came, but
		// something else carried it to an open line before it chose the
		// wolf: no walk to test, no fault.
		{name: "chose from an open line", r: meleeRun{Decided: beside, Origin: beside}, chosen: mob, there: mob, lost: true},
		{name: "the monster walked round to the bot", r: meleeRun{Decided: behind, Origin: behind}, chosen: mob, there: botclient.Point{X: 20, Y: 5}, lost: true},
		{name: "chosen behind, attacked from an open line without a walk", r: meleeRun{Decided: behind, Origin: beside}, chosen: mob, there: mob, fault: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fault, lost := meleeVerdict(tt.r, tt.chosen, tt.there, canMove)
			if (fault != "") != tt.fault || (lost != "") != tt.lost {
				t.Errorf("meleeVerdict = fault %q lost %q, want fault %v lost %v", fault, lost, tt.fault, tt.lost)
			}
		})
	}
}
