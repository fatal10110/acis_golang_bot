package botclient

import (
	"errors"
	"fmt"
	"time"
)

// errDecode ends an attempt before the world on a packet the bot could not
// decode (docs/plan.md D45): the login server's Init, LoginOk, ServerList
// and PlayOk, the game server's VersionCheck, CharSelectInfo, and UserInfo,
// its head or the whole UserInfo that ends the EnterWorld burst. It is a not-ready ending for the gate (notReady), never a
// server abort. In the world a frame that fails to decode is skipped
// instead (decodeSkipped).
var errDecode = errors.New("undecodable packet")

// decodeError is a packet the bot could not decode before the world. It
// wraps errDecode and the decoder's error, which names lengths and opcodes,
// never payload bytes.
type decodeError struct {
	packet string
	// login marks a login server packet: key.op is a login opcode.
	login  bool
	key    opcodeKey
	length int
	err    error
}

func (e *decodeError) Error() string {
	return fmt.Sprintf("%v: %s (%d bytes): %v", errDecode, e.packet, e.length, e.err)
}

func (e *decodeError) Unwrap() []error { return []error{errDecode, e.err} }

// gameDecodeError is err, the failure to decode raw, the game server's
// packet named packet, as errDecode.
func gameDecodeError(packet string, raw []byte, err error) error {
	return &decodeError{packet: packet, key: frameKey(raw), length: len(raw), err: err}
}

// loginDecodeError is err, the failure to decode the login server's packet
// named packet, of opcode op and length bytes, as errDecode.
func loginDecodeError(packet string, op byte, length int, err error) error {
	return &decodeError{packet: packet, login: true, key: opcodeKey{op: op}, length: length, err: err}
}

// opcodeKey names a game frame's opcode: the opcode byte, and the
// sub-opcode of an extended frame. Only the opcodes with a decoder fail to
// decode, so a set of keys of failures is bounded by the decoder tables.
type opcodeKey struct {
	op  byte
	sub uint16
}

// frameKey is the key of raw; an extended frame too short to carry its
// sub-opcode has sub 0.
func frameKey(raw []byte) opcodeKey {
	if len(raw) == 0 {
		return opcodeKey{}
	}
	k := opcodeKey{op: raw[0]}
	if raw[0] == opGameExtended && len(raw) >= 3 {
		k.sub = uint16(raw[1]) | uint16(raw[2])<<8
	}
	return k
}

// String formats k as the bot's logs and decode errors name an opcode.
func (k opcodeKey) String() string {
	if k.op == opGameExtended {
		return fmt.Sprintf("%#02x:%#04x", k.op, k.sub)
	}
	return fmt.Sprintf("%#02x", k.op)
}

// decodeSkipped is queued by the session's reader for a frame that failed
// to decode, which it dropped (docs/plan.md D45); the brain logs and counts
// it. It carries the frame's opcode and length, never its payload.
type decodeSkipped struct {
	key    opcodeKey
	length int
	err    error
}

func (decodeSkipped) event() {}

// BurstSkips counts the frames of an EnterWorld burst NewWorld skipped
// because they failed to decode, by opcode. The brain logs and counts them
// (BrainConfig.burstSkips).
type BurstSkips map[opcodeKey]burstSkip

// burstSkip is the count of one opcode's skipped burst frames, with the
// first one's length and error.
type burstSkip struct {
	count  uint64
	length int
	err    error
}

// add counts raw, which failed to decode with err, and returns s, made on
// the first skip.
func (s BurstSkips) add(raw []byte, err error) BurstSkips {
	if s == nil {
		s = make(BurstSkips)
	}
	k := frameKey(raw)
	c, ok := s[k]
	if !ok {
		c = burstSkip{length: len(raw), err: err}
	}
	c.count++
	s[k] = c
	return s
}

// skipped counts n frames of opcode k the bot skipped because they failed
// to decode, in the world or in the EnterWorld burst (where), and logs the
// first skip of each opcode in the session as an error with the first
// frame's length; no payload byte is logged. Run's goroutine owns
// decodeLogged, or NewBrain's before Run.
func (b *Brain) skipped(k opcodeKey, length int, n uint64, where string, err error) {
	b.metrics.DecodeErrors(k.op, k.sub, n)
	if b.decodeLogged[k] {
		return
	}
	if b.decodeLogged == nil {
		b.decodeLogged = make(map[opcodeKey]bool)
	}
	b.decodeLogged[k] = true
	b.log.Error("undecodable packet skipped: the world model may be stale",
		"opcode", k.String(), "length", length, "count", n, "in", where, "err", err)
}

// afterDecode decides after an attempt a packet it could not decode ended
// before the world: it logs and counts the packet, and the bot retries as
// after any other not-ready ending; the gate already took the ending
// (Turn.Dialed). It is never an abort.
func (b *Bot) afterDecode(de *decodeError) time.Duration {
	if de.login {
		b.brain.Metrics.LoginDecodeError(de.key.op)
		b.log.Error("undecodable packet: attempt ends", "packet", de.packet, "opcode", fmt.Sprintf("login:%#02x", de.key.op), "length", de.length, "err", de.err)
	} else {
		b.brain.Metrics.DecodeErrors(de.key.op, de.key.sub, 1)
		b.log.Error("undecodable packet: attempt ends", "packet", de.packet, "opcode", de.key.String(), "length", de.length, "err", de.err)
	}
	return b.retry.next(b.rng)
}
