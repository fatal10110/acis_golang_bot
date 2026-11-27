package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// GroundItem is an item lying on the ground.
type GroundItem struct {
	ObjectID, TemplateID int32
	Pos                  Point
	Stackable            bool
	Count                int32
}

// SpawnItem reports a ground item coming into sight.
type SpawnItem struct{ Item GroundItem }

// DropItem reports an item dropped to the ground in sight: by DropperID, a
// killed creature or a player.
type DropItem struct {
	DropperID int32
	Item      GroundItem
}

// GetItem reports PickerID picking up ground item ObjectID at Pos.
type GetItem struct {
	PickerID, ObjectID int32
	Pos                Point
}

func (SpawnItem) IsEvent() {}

func (DropItem) IsEvent() {}

func (GetItem) IsEvent() {}

// readGroundItem reads the ground item rows of SpawnItem and DropItem:
// object id, template id, position, stackable flag and count.
func readGroundItem(r *wire.Reader) GroundItem {
	return GroundItem{ObjectID: r.ReadInt32(), TemplateID: r.ReadInt32(), Pos: readPoint(r), Stackable: r.ReadInt32() != 0, Count: r.ReadInt32()}
}

// decodeSpawnItem: the ground item, then a constant 0
// (serverpackets/grounditems.go FrameSpawnItem).
func decodeSpawnItem(r *wire.Reader) Event {
	e := SpawnItem{Item: readGroundItem(r)}
	r.ReadInt32()
	return e
}

// decodeDropItem: the dropper, the ground item, then a constant 1
// (serverpackets/grounditems.go FrameDropItem).
func decodeDropItem(r *wire.Reader) Event {
	e := DropItem{DropperID: r.ReadInt32()}
	e.Item = readGroundItem(r)
	r.ReadInt32()
	return e
}

// decodeGetItem: picker, ground item, position
// (serverpackets/grounditems.go FrameGetItem).
func decodeGetItem(r *wire.Reader) Event {
	return GetItem{PickerID: r.ReadInt32(), ObjectID: r.ReadInt32(), Pos: readPoint(r)}
}

// EncodeRequestDropItem drops count of carried item objectID at p, which
// the server accepts within 150 of the character (the server's
// clientpackets/item_ops.go, inventory.go dropInRange).
func EncodeRequestDropItem(objectID, count int32, p Point) []byte {
	w := wire.NewPacketWriter(OpGameRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	writePoint(w, p)
	return w.Bytes()
}
