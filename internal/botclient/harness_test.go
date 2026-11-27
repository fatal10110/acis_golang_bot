package botclient

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestHarnessEncoders pins the adapters a script calls in place of a botwire
// encoder: each fixes an argument a script does not choose.
func TestHarnessEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"WalkRunToggle", EncodeWalkRunToggle(), []byte{0x45, 1, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestPrivateStoreBuy", EncodeRequestPrivateStoreBuy(9, []StoreBuy{{ObjectID: 7, Count: 2, Price: 300}, {ObjectID: 8, Count: 1, Price: 5}}), []byte{0x79, 9, 0, 0, 0, 2, 0, 0, 0, 7, 0, 0, 0, 2, 0, 0, 0, 0x2c, 1, 0, 0, 8, 0, 0, 0, 1, 0, 0, 0, 5, 0, 0, 0}},
		{"RequestJoinPledge main clan", EncodeRequestJoinPledge(0x20000002), []byte{0x24, 2, 0, 0, 0x20, 0, 0, 0, 0}},
		{"SetPrivateStoreListSell not packaged", EncodeSetPrivateStoreListSell([]StoreBuy{{ObjectID: 7, Count: 2, Price: 300}}), []byte{0x74, 0, 0, 0, 0, 1, 0, 0, 0, 7, 0, 0, 0, 2, 0, 0, 0, 0x2c, 1, 0, 0}},
		{"RequestListPartyWaiting auto", EncodeRequestListPartyWaiting(RoomsAnywhere, RoomsAnyLevel), []byte{0x6f, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 1, 0, 0, 0}},
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
	if _, _, err := DecodeEvent([]byte{opGameRevive, 7, 0, 0, 0, 0}); !errors.Is(err, botwire.ErrTrailingBytes) {
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
