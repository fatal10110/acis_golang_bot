package botwire

import (
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
			packet(OpGameHennaEquipList, int32(50000), int32(2), int32(2),
				int32(1), int32(4445), int32(10), int32(37000), int32(1),
				int32(6), int32(4450), int32(10), int32(37000), int32(1)),
			HennaEquipList{Adena: 50000, MaxSlots: 2, Symbols: []HennaRow{
				{SymbolID: 1, DyeID: 4445, Dyes: 10, Price: 37000},
				{SymbolID: 6, DyeID: 4450, Dyes: 10, Price: 37000},
			}},
		},
		{
			"HennaEquipList empty",
			packet(OpGameHennaEquipList, int32(0), int32(0), int32(0)),
			HennaEquipList{Symbols: []HennaRow{}},
		},
		// INT 0, STR 1, CON 0 (a -3 clamps on the server's uint8 cast is
		// not ours to undo), MEN 0, DEX 1, WIT 0; 3 slots; symbol 1
		// inactive, symbol 37 active.
		{
			"HennaInfo",
			packet(OpGameHennaInfo, byte(0), byte(1), byte(253), byte(0), byte(1), byte(0), int32(3), int32(2),
				int32(1), int32(0), int32(37), int32(37)),
			HennaInfo{STR: 1, CON: 253, DEX: 1, MaxSlots: 3, Worn: []WornSymbol{{SymbolID: 1}, {SymbolID: 37, Active: 37}}},
		},
		{
			"HennaInfo none worn",
			packet(OpGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(0), int32(0)),
			HennaInfo{Worn: []WornSymbol{}},
		},
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

func TestDecodeHennaPacketsRejects(t *testing.T) {
	list := packet(OpGameHennaEquipList, int32(0), int32(2), int32(1), int32(1), int32(4445), int32(10), int32(37000), int32(1))
	info := packet(OpGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(2), int32(1), int32(1), int32(1))
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"list short header", packet(OpGameHennaEquipList, int32(0), int32(2)), wire.ErrShortPacket},
		{"list short row", list[:len(list)-1], wire.ErrShortPacket},
		{"list count past the frame", packet(OpGameHennaEquipList, int32(0), int32(2), int32(2), int32(1), int32(4445), int32(10), int32(37000), int32(1)), wire.ErrShortPacket},
		{"list trailing", append(list, 0), ErrTrailingBytes},
		{"info short stats", packet(OpGameHennaInfo, byte(0), byte(0)), wire.ErrShortPacket},
		{"info short row", info[:len(info)-1], wire.ErrShortPacket},
		{"info rows past the count", packet(OpGameHennaInfo, byte(0), byte(0), byte(0), byte(0), byte(0), byte(0), int32(2), int32(0), int32(1), int32(1)), ErrTrailingBytes},
		{"info trailing", append(info, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
