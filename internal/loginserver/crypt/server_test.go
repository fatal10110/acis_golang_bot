package crypt

// The server half of the login crypt, copied from the acis_golang server as a
// test oracle: the bot never encrypts the first (static key) packet, so this
// stays out of the shipped package.

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang_bot/internal/commons/crypt"
)

// serverLoginCrypt is the server's LoginCrypt: its first Encrypt pads,
// rolling-XORs and encrypts with the static bootstrap key; every later
// Encrypt, and every Decrypt, uses the session's dynamic key exactly as the
// client side does.
type serverLoginCrypt struct {
	*LoginCrypt
	static *crypt.BlowfishCipher
	first  bool
}

// newServerLoginCrypt builds the server side of one session's crypt from its
// dynamic Blowfish key (1-56 bytes).
func newServerLoginCrypt(dynamicKey []byte) (*serverLoginCrypt, error) {
	client, err := NewClientLoginCrypt(dynamicKey)
	if err != nil {
		return nil, err
	}
	static, err := staticBootstrapCipher()
	if err != nil {
		return nil, fmt.Errorf("static bootstrap cipher: %w", err)
	}
	return &serverLoginCrypt{LoginCrypt: client, static: static, first: true}, nil
}

// Encrypt encrypts payload as the server does: the first call with the static
// bootstrap key and a rolling XOR pass, every later call with the dynamic key.
func (c *serverLoginCrypt) Encrypt(payload []byte) []byte {
	if c.first {
		c.first = false
		return c.encryptFirst(payload)
	}
	return c.LoginCrypt.Encrypt(payload)
}

func (c *serverLoginCrypt) encryptFirst(payload []byte) []byte {
	buf := make([]byte, crypt.PaddedSize(len(payload)+4+4))
	copy(buf, payload)
	xorPass(buf, rand.Uint32())
	crypt.EncryptBlocks(c.static, buf)
	return buf
}

// xorPass runs the rolling XOR pass the login server applies to the very
// first outbound packet: each 4-byte little-endian word starting at offset 4
// is XORed against a running key that accumulates the word's original
// value, and the final rolled key is stored in the 4 bytes before buf's final word.
// Requires len(buf) >= 8.
func xorPass(buf []byte, key uint32) {
	stop := len(buf) - 8
	ecx := key
	pos := 4
	for pos < stop {
		edx := binary.LittleEndian.Uint32(buf[pos : pos+4])
		ecx += edx
		edx ^= ecx
		binary.LittleEndian.PutUint32(buf[pos:pos+4], edx)
		pos += 4
	}
	binary.LittleEndian.PutUint32(buf[pos:pos+4], ecx)
}
