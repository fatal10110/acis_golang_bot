package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

func TestDecodeHennaPackets(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Adena 50000, 2 slots, symbols 1 and 6: id, dye, 10, price, 1.
		{
			"HennaEquipList",
			packet(opGameHennaEquipList, int32(50000), int32(2), int32(2),
				int32(1), int32(4445), int32(10), int32(37000), int32(1),
				int32(6), int32(4450), int32(10), int32(37000), int32(1)),
			HennaEquipList{Adena: 50000, MaxSlots: 2, Symbols: []HennaRow{
				{SymbolID: 1, DyeID: 4445, Dyes: 10, Price: 37000},
				{SymbolID: 6, DyeID: 4450, Dyes: 10, Price: 37000},
			}},
		},
		{
			"HennaEquipList empty",
			packet(opGameHennaEquipList, int32(0), int32(0), int32(0)),
			HennaEquipList{Symbols: []HennaRow{}},
		},
		// INT 0, STR 1, CON 0 (a -3 clamps on the server's uint8 cast is
		// not ours to undo), MEN 0, DEX 1, WIT 0; 3 slots; symbol 1
		// inactive, symbol 37 active.
		{
			"HennaInfo",
			packet(opGameHennaInfo, byte(0), byte(1), byte(253), byte(0), byte(1), byte(0), int32(3), int32(2),
				int32(1), int32(0), int32(37), int32(37)),
			HennaInfo{STR: 1, CON: 253, DEX: 1, MaxSlots: 3, Worn: []WornSymbol{{SymbolID: 1}, {SymbolID: 37, Active: 37}}},
		},
		{
			"HennaInfo none worn",
			packet(opGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(0), int32(0)),
			HennaInfo{Worn: []WornSymbol{}},
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

func TestDecodeHennaPacketsRejects(t *testing.T) {
	list := packet(opGameHennaEquipList, int32(0), int32(2), int32(1), int32(1), int32(4445), int32(10), int32(37000), int32(1))
	info := packet(opGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(2), int32(1), int32(1), int32(1))
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"list short header", packet(opGameHennaEquipList, int32(0), int32(2)), wire.ErrShortPacket},
		{"list short row", list[:len(list)-1], wire.ErrShortPacket},
		{"list count past the frame", packet(opGameHennaEquipList, int32(0), int32(2), int32(2), int32(1), int32(4445), int32(10), int32(37000), int32(1)), wire.ErrShortPacket},
		{"list trailing", append(list, 0), errTrailingBytes},
		{"info short stats", packet(opGameHennaInfo, byte(0), byte(0)), wire.ErrShortPacket},
		{"info short row", info[:len(info)-1], wire.ErrShortPacket},
		{"info rows past the count", packet(opGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(2), int32(0), int32(1), int32(1)), errTrailingBytes},
		{"info trailing", append(info, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestHennaEncoders pins the request bytes: the opcode and one int32, the
// symbol id, as clientpackets/henna.go decodeHennaInt reads them.
func TestHennaEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestHennaEquip", encodeRequestHennaEquip(6), []byte{0xbc, 6, 0, 0, 0}},
		{"RequestHennaEquip high id", encodeRequestHennaEquip(0x0125), []byte{0xbc, 0x25, 1, 0, 0}},
		{"RequestHennaUnequip", encodeRequestHennaUnequip(37), []byte{0xbf, 37, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("payload % x, want % x", tt.got, tt.want)
			}
		})
	}
}
