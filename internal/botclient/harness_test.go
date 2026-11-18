package botclient

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestHarnessEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"SendBypassBuildCmd", EncodeSendBypassBuildCmd("kick e2e"), []byte{0x5b, 'k', 0, 'i', 0, 'c', 0, 'k', 0, ' ', 0, 'e', 0, '2', 0, 'e', 0, 0, 0}},
		{"RequestRestartPoint village", EncodeRequestRestartPoint(RestartVillage), []byte{0x6d, 0, 0, 0, 0}},
		{"Appearing", EncodeAppearing(), []byte{0x30}},
		{"Action", EncodeAction(0x01020304, Point{X: 1, Y: -1, Z: 2}), []byte{0x04, 4, 3, 2, 1, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0, 0}},
		{"MoveBackwardToLocation", EncodeMoveBackwardToLocation(Point{X: 1, Y: -1, Z: 2}, Point{X: 3, Y: 4, Z: 5}), []byte{0x01, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0}},
		{"WalkRunToggle", EncodeWalkRunToggle(), []byte{0x45, 1, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestJoinParty", EncodeRequestJoinParty("ab", 1), []byte{0x29, 'a', 0, 'b', 0, 0, 0, 1, 0, 0, 0}},
		{"Say2Tell", EncodeSay2Tell("ab", "hi"), []byte{0x38, 'h', 0, 'i', 0, 0, 0, 2, 0, 0, 0, 'a', 0, 'b', 0, 0, 0}},
		// The server's own vector (clientpackets_item_trade_test.go).
		{"RequestDestroyItem", EncodeRequestDestroyItem(501, 2), []byte{0x59, 0xf5, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}},
		{"UseItem", EncodeUseItem(0x01020304), []byte{0x14, 4, 3, 2, 1, 0, 0, 0, 0}},
		{"RequestDropItem", EncodeRequestDropItem(0x01020304, 2, Point{X: 1, Y: -1, Z: 2}), []byte{0x12, 4, 3, 2, 1, 2, 0, 0, 0, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("% x, want % x", tt.got, tt.want)
			}
		})
	}
}

func TestDecodeEventExported(t *testing.T) {
	ev, ok, err := DecodeEvent([]byte{opGameRevive, 7, 0, 0, 0})
	if err != nil || !ok || ev != (Revive{ObjectID: 7}) {
		t.Fatalf("DecodeEvent(Revive) = %v %v %v", ev, ok, err)
	}
	if _, _, err := DecodeEvent([]byte{opGameRevive, 7, 0, 0, 0, 0}); !errors.Is(err, errTrailingBytes) {
		t.Fatalf("DecodeEvent(long Revive) error %v, want trailing bytes", err)
	}
}

// paceAsync runs one Pacer wait on its own goroutine and reports when it ends.
func paceAsync(wait func(context.Context) error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- wait(context.Background()) }()
	return done
}

func TestPacerFrameBucket(t *testing.T) {
	clock := newFakeClock(time.Unix(1000, 0))
	p := NewPacer(clock)
	for i := range frameBurst {
		if err := p.Frame(t.Context()); err != nil {
			t.Fatalf("frame %d of the burst: %v", i+1, err)
		}
	}
	done := paceAsync(p.Frame)
	<-clock.armed // parked on the next token
	select {
	case err := <-done:
		t.Fatalf("frame past the burst went out at once (%v)", err)
	default:
	}
	clock.Advance(time.Second / frameRate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPacerCommandGap(t *testing.T) {
	clock := newFakeClock(time.Unix(1000, 0))
	p := NewPacer(clock)
	if err := p.Command(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := p.Frame(t.Context()); err != nil {
		t.Fatalf("a plain frame waits on the command gap: %v", err)
	}
	done := paceAsync(p.Command)
	<-clock.armed
	clock.Advance(bypassGap - time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("command inside the gap went out (%v)", err)
	default:
	}
	clock.Advance(time.Millisecond)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPacerEndsWithContext(t *testing.T) {
	clock := newFakeClock(time.Unix(1000, 0))
	p := NewPacer(clock)
	_ = p.Command(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- p.Command(ctx) }()
	<-clock.armed
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Command after cancel = %v, want context.Canceled", err)
	}
}
