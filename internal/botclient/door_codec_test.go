package botclient

import (
	"errors"
	"reflect"
	"testing"
)

func TestDecodeDoorPackets(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Object 0x10000001, door 24190001, no HP bar, closed, 1000 of 1000 HP.
		{
			"DoorInfo bytes",
			[]byte{
				0x4c,
				0x01, 0, 0, 0x10, 0x31, 0x1c, 0x71, 0x01,
				0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0,
				0xe8, 0x03, 0, 0, 0xe8, 0x03, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			DoorInfo{ObjectID: 0x10000001, DoorID: 24190001, Closed: true},
		},
		{
			"DoorInfo open with HP bar",
			packet(opGameDoorInfo, 7, 21170001, 1, 1, 0, 500, 250, 0, 0),
			DoorInfo{ObjectID: 7, DoorID: 21170001},
		},
		// Object 0x10000001, open, damage 0, no HP bar, door 24190001.
		{
			"DoorStatusUpdate bytes",
			[]byte{
				0x4d,
				0x01, 0, 0, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0x31, 0x1c, 0x71, 0x01, 0xe8, 0x03, 0, 0, 0xe8, 0x03, 0, 0,
			},
			DoorStatusUpdate{ObjectID: 0x10000001, DoorID: 24190001},
		},
		{
			"DoorStatusUpdate closed and damaged",
			packet(opGameDoorStatusUpdate, 7, 1, 3, 1, 21170001, 500, 100),
			DoorStatusUpdate{ObjectID: 7, DoorID: 21170001, Closed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDecodeDoorPacketsRejects: each decoder consumes exactly its layout,
// so a short frame and a frame with a trailing byte are errors.
func TestDecodeDoorPacketsRejects(t *testing.T) {
	info := packet(opGameDoorInfo, 7, 21170001, 1, 1, 0, 500, 250, 0, 0)
	status := packet(opGameDoorStatusUpdate, 7, 1, 3, 1, 21170001, 500, 100)
	tests := []struct {
		name     string
		raw      []byte
		trailing bool
	}{
		{"DoorInfo short", info[:len(info)-1], false},
		{"DoorInfo trailing", append(info[:len(info):len(info)], 0), true},
		{"DoorStatusUpdate short", status[:len(status)-1], false},
		{"DoorStatusUpdate trailing", append(status[:len(status):len(status)], 0), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeEvent(tt.raw)
			if err == nil {
				t.Fatal("decodeEvent accepted the frame")
			}
			if got := errors.Is(err, errTrailingBytes); got != tt.trailing {
				t.Fatalf("decodeEvent = %v, trailing %v, want %v", err, got, tt.trailing)
			}
		})
	}
}
