package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

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
			packet(opGameRecipeBookItemList, int32(1), int32(842), int32(1), int32(300), int32(1)),
			RecipeBookItemList{MaxMP: 842, Recipes: []int32{300}},
		},
		{
			"RecipeBookItemList empty",
			packet(opGameRecipeBookItemList, int32(1), int32(50), int32(0)),
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
			packet(opGameRecipeItemMakeInfo, int32(300), int32(1), int32(10), int32(90), RecipeMadeFailed),
			RecipeItemMakeInfo{RecipeID: 300, MP: 10, MaxMP: 90, Status: RecipeMadeFailed},
		},
		{
			"RecipeItemMakeInfo none",
			packet(opGameRecipeItemMakeInfo, int32(300), int32(1), int32(10), int32(90), RecipeMadeNone),
			RecipeItemMakeInfo{RecipeID: 300, MP: 10, MaxMP: 90, Status: RecipeMadeNone},
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

func TestDecodeRecipePacketsRejects(t *testing.T) {
	book := packet(opGameRecipeBookItemList, int32(0), int32(100), int32(1), int32(1), int32(1))
	mk := packet(opGameRecipeItemMakeInfo, int32(20), int32(0), int32(60), int32(100), int32(1))
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"book short header", packet(opGameRecipeBookItemList, int32(0), int32(100)), wire.ErrShortPacket},
		{"book short row", book[:len(book)-1], wire.ErrShortPacket},
		{"book count past the frame", packet(opGameRecipeBookItemList, int32(0), int32(100), int32(2), int32(1), int32(1)), wire.ErrShortPacket},
		{"book rows past the count", packet(opGameRecipeBookItemList, int32(0), int32(100), int32(0), int32(1), int32(1)), errTrailingBytes},
		{"book trailing", append(book, 0), errTrailingBytes},
		{"make short", mk[:len(mk)-1], wire.ErrShortPacket},
		{"make trailing", append(mk, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRecipeEncoders pins the request bytes: the opcode and one int32, as
// clientpackets/recipe.go decodeRecipeInt reads them.
func TestRecipeEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestRecipeBookOpen dwarven", encodeRequestRecipeBookOpen(true), []byte{0xac, 0, 0, 0, 0}},
		{"RequestRecipeBookOpen common", encodeRequestRecipeBookOpen(false), []byte{0xac, 1, 0, 0, 0}},
		{"RequestRecipeItemMakeSelf", encodeRequestRecipeItemMakeSelf(20), []byte{0xaf, 20, 0, 0, 0}},
		{"RequestRecipeItemMakeSelf high id", encodeRequestRecipeItemMakeSelf(0x0102), []byte{0xaf, 2, 1, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("payload % x, want % x", tt.got, tt.want)
			}
		})
	}
}

// TestCraftFailure tells the system messages ahead of RecipeItemMakeInfo
// status 0 apart; the success messages and the consumed notices tell none.
func TestCraftFailure(t *testing.T) {
	tests := []struct {
		msg  int32
		want CraftFailure
	}{
		{msgItemMixingFailed, CraftMixingFailed},
		{msgMissingS2S1ToCreate, CraftMissingMaterial},
		{msgNotEnoughMP, CraftNotEnoughMP},
		{msgCantOperateStoreDuringCombat, CraftInCombat},
		{msgS2S1Disappeared, CraftFailNone},
		{msgS1Disappeared, CraftFailNone},
		{msgEarnedS2S1S, CraftFailNone},
		{msgEarnedItemS1, CraftFailNone},
		{msgS1Added, CraftFailNone},
	}
	for _, tt := range tests {
		if got := craftFailure(tt.msg); got != tt.want {
			t.Errorf("craftFailure(%d) = %d, want %d", tt.msg, got, tt.want)
		}
	}
}

// TestCraftGate spaces crafts craftGap apart, clear of the server's
// ManufactureDelay (300 ms), inside which a craft is dropped in silence.
func TestCraftGate(t *testing.T) {
	a := craftAction(20)
	if a.gate() != gateCraft || !bytes.Equal(a.payload, []byte{0xaf, 20, 0, 0, 0}) {
		t.Fatalf("craftAction = gate %d payload % x", a.gate(), a.payload)
	}
	if craftGap < 300*time.Millisecond {
		t.Fatalf("craftGap %v is inside the server's ManufactureDelay", craftGap)
	}
	if recipeBookAction(true).gate() != gateNone {
		t.Fatal("a book open passes a reuse gate")
	}
	p := newPacer(t0)
	tries := []struct {
		at   time.Duration
		want bool
	}{{0, true}, {craftGap - time.Millisecond, false}, {craftGap, true}, {craftGap + time.Millisecond, false}}
	for _, tr := range tries {
		now := at(tr.at)
		if got := p.allow(gateCraft, now); got != tr.want {
			t.Fatalf("allow(gateCraft) at %v = %v, want %v", tr.at, got, tr.want)
		}
		if tr.want {
			p.sent(gateCraft, now)
		}
	}
}
