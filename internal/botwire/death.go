package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// ConfirmDlg parameter types the server writes (confirmdlg.go).
const (
	confirmParamText int32 = 0 // a string
	confirmParamZone int32 = 7 // a position: x, y, z
)

// ConfirmDlg is a yes/no question from the server, such as a resurrection
// offer (MessageID 1510, Name the reviver).
type ConfirmDlg struct {
	MessageID int32
	// Name is the first text parameter, "" for none.
	Name string
	// Time is the countdown in milliseconds and RequesterID the object id
	// the answer echoes; the server leaves out each one that is zero, so a
	// lone trailing value reads as Time.
	Time, RequesterID int32
}

func (ConfirmDlg) IsEvent() {}

// decodeConfirmDlg: message id, parameter count, each parameter's type and
// value (text: a string; zone: x, y, z), then the optional countdown and
// requester id. A parameter type the server never writes leaves the rest
// unparsable: it is skipped.
func decodeConfirmDlg(r *wire.Reader) Event {
	e := ConfirmDlg{MessageID: r.ReadInt32()}
	named := false
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		switch r.ReadInt32() {
		case confirmParamText:
			if s := r.ReadString(); !named {
				e.Name, named = s, true
			}
		case confirmParamZone:
			r.Skip(3 * 4)
		default:
			r.Skip(r.Remaining())
			return e
		}
	}
	if r.Remaining() >= 4 {
		e.Time = r.ReadInt32()
	}
	if r.Remaining() >= 4 {
		e.RequesterID = r.ReadInt32()
	}
	return e
}

// EncodeDlgAnswer answers ConfirmDlg msg (accept 1, decline 0), echoing its
// requester id.
func EncodeDlgAnswer(msg int32, accept bool, requester int32) []byte {
	w := wire.NewPacketWriter(OpGameDlgAnswer)
	w.WriteInt32(msg)
	var v int32
	if accept {
		v = 1
	}
	w.WriteInt32(v)
	w.WriteInt32(requester)
	return w.Bytes()
}

// EncodeRequestUserCommand runs a numbered slash command.
func EncodeRequestUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}
