package botclient

import (
	"reflect"
	"testing"
)

func TestEventsCoalesceMoves(t *testing.T) {
	move := func(oid, x int32) Event { return MoveToLocation{ObjectID: oid, Dest: Point{X: x}} }
	stop := StopMove{ObjectID: 1, Pos: Point{X: 15}}
	tele := TeleportToLocation{ObjectID: 1, Pos: Point{X: 99}}
	gone := DeleteObject{ObjectID: 2}
	tests := []struct {
		name   string
		limit  int
		pushed []Event
		want   []Event
	}{
		{"distinct objects keep every move", 8, []Event{move(1, 10), move(2, 20)}, []Event{move(1, 10), move(2, 20)}},
		{"latest move per object", 8, []Event{move(1, 10), move(2, 20), move(1, 11), move(1, 12)}, []Event{move(2, 20), move(1, 12)}},
		{"never ahead of the object's stop", 8, []Event{move(1, 10), stop, move(1, 11)}, []Event{stop, move(1, 11)}},
		{"never ahead of the object's teleport", 8, []Event{move(1, 10), tele, move(1, 11)}, []Event{tele, move(1, 11)}},
		{"other kinds never coalesce", 8, []Event{stop, stop, gone, gone}, []Event{stop, stop, gone, gone}},
		{"a move coalesces into a full queue", 2, []Event{move(1, 10), gone, move(1, 11)}, []Event{gone, move(1, 11)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewEvents(tt.limit)
			done := make(chan struct{})
			close(done) // a push that would wait fails instead
			for i, ev := range tt.pushed {
				if !q.push(ev, done) {
					t.Fatalf("push %d (%v) found the queue full", i, ev)
				}
			}
			if got := q.Drain(nil); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("drained %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventsReadyAndDrain(t *testing.T) {
	q := NewEvents(4)
	select {
	case <-q.Ready():
		t.Fatal("Ready fired on an empty queue")
	default:
	}
	done := make(chan struct{})
	q.push(LeaveWorld{}, done)
	q.push(ServerClose{}, done)
	<-q.Ready()
	buf := make([]Event, 0, 4)
	if got := q.Drain(buf); !reflect.DeepEqual(got, []Event{LeaveWorld{}, ServerClose{}}) {
		t.Fatalf("Drain = %v", got)
	}
	if got := q.Drain(nil); len(got) != 0 {
		t.Fatalf("second Drain = %v, want empty", got)
	}
}

// TestEventsPushWaitsForRoom: a push on a full queue waits for a Drain, and
// gives up when done closes instead.
func TestEventsPushWaitsForRoom(t *testing.T) {
	q := NewEvents(1)
	done := make(chan struct{})
	q.push(DeleteObject{ObjectID: 1}, done)

	pushed := make(chan bool)
	go func() { pushed <- q.push(DeleteObject{ObjectID: 2}, done) }()
	if got := q.Drain(nil); !reflect.DeepEqual(got, []Event{DeleteObject{ObjectID: 1}}) {
		t.Fatalf("first Drain = %v", got)
	}
	if !<-pushed {
		t.Fatal("push after Drain reported done")
	}

	go func() { pushed <- q.push(DeleteObject{ObjectID: 3}, done) }()
	close(done)
	if <-pushed {
		t.Fatal("push on a full queue succeeded with nobody draining")
	}
	if got := q.Drain(nil); !reflect.DeepEqual(got, []Event{DeleteObject{ObjectID: 2}}) {
		t.Fatalf("second Drain = %v", got)
	}
}
