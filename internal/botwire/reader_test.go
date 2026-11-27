package botwire

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Payloads in the server's layouts (serverpackets/<file> named per decoder).
func moveToLocation(oid int32, dest, origin Point) []byte {
	return packet(OpGameMoveToLocation, oid, dest.X, dest.Y, dest.Z, origin.X, origin.Y, origin.Z)
}

func stopMove(oid int32, at Point, heading int32) []byte {
	return packet(OpGameStopMove, oid, at.X, at.Y, at.Z, heading)
}

func TestDecodeEvent(t *testing.T) {
	at := Point{X: 81000, Y: 148000, Z: -3400}
	to := Point{X: 81200, Y: 148100, Z: -3410}
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"MoveToLocation", moveToLocation(0x10000001, to, at), MoveToLocation{ObjectID: 0x10000001, Dest: to, Origin: at}},
		{"StopMove", stopMove(7, at, 49152), StopMove{ObjectID: 7, Pos: at, Heading: 49152}},
		{"ValidateLocation", packet(OpGameValidateLocation, 7, at.X, at.Y, at.Z, 100), ValidateLocation{ObjectID: 7, Pos: at, Heading: 100}},
		{"TeleportToLocation", packet(OpGameTeleportToLocation, 7, at.X, at.Y, at.Z, 0), TeleportToLocation{ObjectID: 7, Pos: at}},
		{"TeleportToLocation fast", packet(OpGameTeleportToLocation, 7, at.X, at.Y, at.Z, 1), TeleportToLocation{ObjectID: 7, Pos: at, Fast: true}},
		{"DeleteObject", packet(OpGameDeleteObject, 7, 1), DeleteObject{ObjectID: 7}},
		{"DeleteObject seated", packet(OpGameDeleteObject, 7, 0), DeleteObject{ObjectID: 7, Seated: true}},
		{"ServerClose", packet(OpGameServerClose), ServerClose{}},
		{"LeaveWorld", packet(OpGameLeaveWorld), LeaveWorld{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("DecodeEvent = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeEventRejects(t *testing.T) {
	move := moveToLocation(1, Point{}, Point{})
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short", move[:len(move)-1], wire.ErrShortPacket},
		{"trailing", append(move, 0), ErrTrailingBytes},
		{"trailing on empty body", packet(OpGameLeaveWorld, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
