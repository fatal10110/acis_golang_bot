package botclient

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/testsupport/decodefuzz"
)

// FuzzDecodeEvent: any non-empty frame decodes or fails without panicking,
// within a heap budget bounded by its length, and an accepted frame decodes
// to the same event again.
func FuzzDecodeEvent(f *testing.F) {
	f.Add(moveToLocation(1, Point{X: 1}, Point{Y: 2}))
	f.Add(stopMove(1, Point{Z: 3}, 4))
	f.Add(packet(opGameDeleteObject, 1, 0))
	f.Add(packet(opGameTeleportToLocation, 1, 2, 3, 4))
	f.Add(packet(opGameLeaveWorld))
	f.Add([]byte{opGameUserInfo, 1})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) == 0 {
			return // GameConn.Read rejects empty frames
		}
		decodefuzz.Bounded(t, "decodeEvent", raw, func() { _, _, _ = decodeEvent(raw) })
		ev, ok, err := decodeEvent(raw)
		if err != nil || !ok {
			return
		}
		again, _, _ := decodeEvent(raw)
		if again != ev {
			t.Fatalf("decodeEvent not repeatable: %v then %v", ev, again)
		}
	})
}
