package botmetrics

import "fmt"

// The slots of the decode error counter (Bot.DecodeErrors,
// Bot.LoginDecodeError): one per game opcode, one per extended (0xfe)
// sub-opcode up to 0xff, and one per login server opcode. They are fixed, so the counter does not grow with the run;
// an extended sub-opcode above 0xff counts in the 0xfe opcode's own slot,
// which no other frame uses.
const (
	extendedOpcode = 0xfe
	extSlots       = 256
	loginSlots     = 256 + extSlots
	decodeSlots    = loginSlots + 256
)

// DecodeErrors records n server packets the bot could not decode: op is the
// frame's opcode and sub the sub-opcode of an extended (0xfe) frame, ignored
// for any other.
func (b *Bot) DecodeErrors(op byte, sub uint16, n uint64) {
	if b == nil || n == 0 {
		return
	}
	i := int(op)
	if op == extendedOpcode && sub < extSlots {
		i = 256 + int(sub)
	}
	b.decodeErrors[i].Add(n)
}

// LoginDecodeError records a login server packet of opcode op the bot could
// not decode.
func (b *Bot) LoginDecodeError(op byte) {
	if b != nil {
		b.decodeErrors[loginSlots+int(op)].Add(1)
	}
}

// decodeSlotName is slot i's Snapshot.DecodeErrors key: the opcode as the
// bot's logs name it ("0x04", "0xfe:0x0012"), "0xfe:other" for the extended
// sub-opcodes above 0xff, and "login:0x03" for a login server opcode.
func decodeSlotName(i int) string {
	switch {
	case i == extendedOpcode:
		return "0xfe:other"
	case i < 256:
		return fmt.Sprintf("%#02x", i)
	case i < loginSlots:
		return fmt.Sprintf("%#02x:%#04x", extendedOpcode, i-256)
	}
	return fmt.Sprintf("login:%#02x", i-loginSlots)
}
