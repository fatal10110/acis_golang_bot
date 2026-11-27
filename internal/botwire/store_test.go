package botwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Items of the store tests (aCis_datapack/data/xml/items): wooden arrows
// are sold by NPC shops, the others are loot. The Tutorial Guide a new
// character carries is loot its store cannot price (reference price 1).
const (
	woodenArrow   int32 = 17
	stoneSword    int32 = 2370
	thread        int32 = 1868
	tutorialGuide int32 = 5588
)

var (
	// Candidates of the manage list, their price the reference price.
	skinCand  = StoreItem{Type2: 5, ObjectID: 102, TemplateID: animalSkin, Count: 10, Price: 10, ReferencePrice: 10}
	swordCand = StoreItem{Type2: 0, ObjectID: 107, TemplateID: stoneSword, Count: 1, BodySlot: 128, Price: 1000, ReferencePrice: 1000}
	// swordListed is the sword listed at 700, enchanted +3.
	swordListed = StoreItem{Type2: 0, ObjectID: 107, TemplateID: stoneSword, Count: 1, Enchant: 3, BodySlot: 128, Price: 700, ReferencePrice: 1000}
)

// storeRowFields is a store window's int32 row count and rows as packet
// fields; listed rows carry their reference price.
func storeRowFields(listed bool, items ...StoreItem) []any {
	f := []any{int32(len(items))}
	for _, it := range items {
		f = append(f, it.Type2, it.ObjectID, it.TemplateID, it.Count, uint16(0), it.Enchant, uint16(0), it.BodySlot, it.Price)
		if listed {
			f = append(f, it.ReferencePrice)
		}
	}
	return f
}

func manageList(owner, adena int32, candidates, listed []StoreItem) []byte {
	f := append([]any{owner, int32(0), adena}, storeRowFields(false, candidates...)...)
	return packet(OpGamePrivateStoreManageListSell, append(f, storeRowFields(true, listed...)...)...)
}

func TestDecodeStoreWindows(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Owner 0x10000001, not packaged, 1000 adena; the skin a candidate
		// at its reference price 10, the sword +3 listed at 700 of 1000.
		{
			"PrivateStoreManageListSell bytes",
			[]byte{
				0x9a, 1, 0, 0, 0x10, 0, 0, 0, 0, 0xe8, 3, 0, 0,
				1, 0, 0, 0,
				5, 0, 0, 0, 102, 0, 0, 0, 0x4b, 7, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0,
				1, 0, 0, 0,
				0, 0, 0, 0, 107, 0, 0, 0, 0x42, 9, 0, 0, 1, 0, 0, 0, 0, 0, 3, 0, 0, 0, 128, 0, 0, 0, 0xbc, 2, 0, 0, 0xe8, 3, 0, 0,
			},
			PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: []StoreItem{skinCand}, Listed: []StoreItem{swordListed}},
		},
		{
			"PrivateStoreManageListSell empty",
			manageList(selfID, 0, nil, nil),
			PrivateStoreManageListSell{OwnerID: selfID, Candidates: []StoreItem{}, Listed: []StoreItem{}},
		},
		{
			"PrivateStoreListSell",
			packet(OpGamePrivateStoreListSell, append([]any{selfID, int32(1), int32(5000)}, storeRowFields(true, swordListed)...)...),
			PrivateStoreListSell{OwnerID: selfID, Packaged: true, BuyerAdena: 5000, Items: []StoreItem{swordListed}},
		},
		{
			"PrivateStoreMsgSell",
			packet(OpGamePrivateStoreMsgSell, selfID, "Loot for sale"),
			PrivateStoreMsgSell{OwnerID: selfID, Title: "Loot for sale"},
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

func TestDecodeStoreWindowsRejects(t *testing.T) {
	manage := manageList(selfID, 1, []StoreItem{skinCand}, nil)
	list := packet(OpGamePrivateStoreListSell, append([]any{selfID, int32(0), int32(1)}, storeRowFields(true, swordListed)...)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"manage list short header", packet(OpGamePrivateStoreManageListSell, selfID, int32(0)), wire.ErrShortPacket},
		{"manage list no listed count", manage[:len(manage)-4], wire.ErrShortPacket},
		{"manage list trailing", append(manage, 0), ErrTrailingBytes},
		{"manage list count past the frame", packet(OpGamePrivateStoreManageListSell, selfID, int32(0), int32(1), int32(2)), wire.ErrShortPacket},
		{"store list short row", list[:len(list)-4], wire.ErrShortPacket},
		{"store list trailing", append(list, 0), ErrTrailingBytes},
		{"store title unterminated", packet(OpGamePrivateStoreMsgSell, selfID, []byte{'a', 0}), wire.ErrShortPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
