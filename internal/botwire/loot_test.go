package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	// adenaDrop and junkDrop are ground items, junkTemplate a junk item's
	// template id.
	adenaDrop    = int32(0x40000001)
	junkDrop     = int32(0x40000002)
	junkTemplate = int32(1864) // Stem
)

var (
	adenaOnGround = GroundItem{ObjectID: adenaDrop, TemplateID: 57, Pos: Point{X: 900, Y: 20, Z: -5}, Stackable: true, Count: 120}
	junkOnGround  = GroundItem{ObjectID: junkDrop, TemplateID: junkTemplate, Pos: Point{X: 950}, Stackable: true, Count: 3}
)

func TestDecodeGroundItems(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"SpawnItem bytes",
			[]byte{
				0x0b, 1, 0, 0, 0x40, 57, 0, 0, 0, 0x84, 3, 0, 0, 20, 0, 0, 0, 0xfb, 0xff, 0xff, 0xff,
				1, 0, 0, 0, 120, 0, 0, 0, 0, 0, 0, 0,
			},
			SpawnItem{Item: adenaOnGround},
		},
		{
			"DropItem",
			packet(OpGameDropItem, npcID, junkDrop, junkTemplate, 950, 0, 0, 1, 3, 1),
			DropItem{DropperID: npcID, Item: junkOnGround},
		},
		{
			"DropItem not stackable",
			packet(OpGameDropItem, selfID, 7, 2369, 1, 2, 3, 0, 1, 1),
			DropItem{DropperID: selfID, Item: GroundItem{ObjectID: 7, TemplateID: 2369, Pos: Point{1, 2, 3}, Count: 1}},
		},
		{"GetItem", packet(OpGameGetItem, pcID, adenaDrop, 900, 20, -5), GetItem{PickerID: pcID, ObjectID: adenaDrop, Pos: Point{900, 20, -5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeGroundItemsRejects(t *testing.T) {
	spawn := packet(OpGameSpawnItem, adenaDrop, 57, 1, 2, 3, 1, 5, 0)
	drop := packet(OpGameDropItem, npcID, adenaDrop, 57, 1, 2, 3, 1, 5, 1)
	get := packet(OpGameGetItem, pcID, adenaDrop, 1, 2, 3)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"SpawnItem short", spawn[:len(spawn)-1], wire.ErrShortPacket},
		{"SpawnItem trailing", append(spawn, 0), ErrTrailingBytes},
		{"DropItem short", drop[:len(drop)-4], wire.ErrShortPacket},
		{"DropItem trailing", append(drop, 0), ErrTrailingBytes},
		{"GetItem short", get[:len(get)-1], wire.ErrShortPacket},
		{"GetItem trailing", append(get, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
