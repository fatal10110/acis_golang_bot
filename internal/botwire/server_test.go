package botwire

import (
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// packet builds a payload: the opcode, then each field in order (int and
// int32 as int32, byte, uint16, string as UTF-16 with its terminator, []byte
// as is).
func packet(opcode byte, fields ...any) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, f := range fields {
		switch v := f.(type) {
		case int:
			w.WriteInt32(int32(v))
		case int32:
			w.WriteInt32(v)
		case byte:
			w.WriteUint8(v)
		case uint16:
			w.WriteUint16(v)
		case string:
			w.WriteString(v)
		case []byte:
			w.WriteBytes(v)
		default:
			panic(fmt.Sprintf("packet: unsupported field %T", f))
		}
	}
	return w.Bytes()
}
