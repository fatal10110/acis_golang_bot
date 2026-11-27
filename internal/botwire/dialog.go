package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

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

// EncodeRequestBypassToServer sends bypass command cmd
// (clientpackets/html.go).
func EncodeRequestBypassToServer(cmd string) []byte {
	w := wire.NewPacketWriter(OpGameRequestBypassToServer)
	w.WriteString(cmd)
	return w.Bytes()
}

// EncodeSendBypassBuildCmd is a GM's admin command: cmd is the command line
// without its leading "//" (server clientpackets/admin.go, one string). The
// server runs it only for a character of enough access level
// (network/admin.go sendBypassBuildCmd).
func EncodeSendBypassBuildCmd(cmd string) []byte {
	w := wire.NewPacketWriter(OpGameSendBypassBuildCmd)
	w.WriteString(cmd)
	return w.Bytes()
}
