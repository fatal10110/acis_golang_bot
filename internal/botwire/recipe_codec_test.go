package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

func TestDecodeRecipePackets(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Dwarven page, max MP 100, recipes 1 and 20 at positions 1 and 2.
		{
			"RecipeBookItemList bytes",
			[]byte{0xd6, 0, 0, 0, 0, 100, 0, 0, 0, 2, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 20, 0, 0, 0, 2, 0, 0, 0},
			RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{1, 20}},
		},
		{
			"RecipeBookItemList common",
			packet(OpGameRecipeBookItemList, int32(1), int32(842), int32(1), int32(300), int32(1)),
			RecipeBookItemList{MaxMP: 842, Recipes: []int32{300}},
		},
		{
			"RecipeBookItemList empty",
			packet(OpGameRecipeBookItemList, int32(1), int32(50), int32(0)),
			RecipeBookItemList{MaxMP: 50, Recipes: []int32{}},
		},
		// Recipe 20, dwarven, 60 of 100 MP, success.
		{
			"RecipeItemMakeInfo bytes",
			[]byte{0xd7, 20, 0, 0, 0, 0, 0, 0, 0, 60, 0, 0, 0, 100, 0, 0, 0, 1, 0, 0, 0},
			RecipeItemMakeInfo{RecipeID: 20, Dwarven: true, MP: 60, MaxMP: 100, Status: RecipeMadeSuccess},
		},
		{
			"RecipeItemMakeInfo common failed",
			packet(OpGameRecipeItemMakeInfo, int32(300), int32(1), int32(10), int32(90), RecipeMadeFailed),
			RecipeItemMakeInfo{RecipeID: 300, MP: 10, MaxMP: 90, Status: RecipeMadeFailed},
		},
		{
			"RecipeItemMakeInfo none",
			packet(OpGameRecipeItemMakeInfo, int32(300), int32(1), int32(10), int32(90), RecipeMadeNone),
			RecipeItemMakeInfo{RecipeID: 300, MP: 10, MaxMP: 90, Status: RecipeMadeNone},
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

func TestDecodeRecipePacketsRejects(t *testing.T) {
	book := packet(OpGameRecipeBookItemList, int32(0), int32(100), int32(1), int32(1), int32(1))
	mk := packet(OpGameRecipeItemMakeInfo, int32(20), int32(0), int32(60), int32(100), int32(1))
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"book short header", packet(OpGameRecipeBookItemList, int32(0), int32(100)), wire.ErrShortPacket},
		{"book short row", book[:len(book)-1], wire.ErrShortPacket},
		{"book count past the frame", packet(OpGameRecipeBookItemList, int32(0), int32(100), int32(2), int32(1), int32(1)), wire.ErrShortPacket},
		{"book rows past the count", packet(OpGameRecipeBookItemList, int32(0), int32(100), int32(0), int32(1), int32(1)), ErrTrailingBytes},
		{"book trailing", append(book, 0), ErrTrailingBytes},
		{"make short", mk[:len(mk)-1], wire.ErrShortPacket},
		{"make trailing", append(mk, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
