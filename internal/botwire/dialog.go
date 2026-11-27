package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// NpcHtmlMessage opens a dialog: the HTML page of object ObjectID, or of
// item ItemID when an item opened it.
type NpcHtmlMessage struct {
	ObjectID int32
	HTML     string
	ItemID   int32
}

func (NpcHtmlMessage) IsEvent() {}

// decodeNpcHtmlMessage: object id, the page, item id
// (serverpackets/npchtmlmessage.go).
func decodeNpcHtmlMessage(r *wire.Reader) Event {
	return NpcHtmlMessage{ObjectID: r.ReadInt32(), HTML: r.ReadString(), ItemID: r.ReadInt32()}
}
