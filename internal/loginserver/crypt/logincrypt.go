package crypt

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/commons/crypt"
)

// staticBootstrapKey is the fixed Blowfish key that encrypts the very first
// server-to-client login packet. That packet itself delivers the session's
// dynamic key, used for everything after.
var staticBootstrapKey = []byte{
	0x6b, 0x60, 0xcb, 0x5b, 0x82, 0xce, 0x90, 0xb1,
	0xcc, 0x2b, 0x6c, 0x55, 0x6c, 0x6c, 0x6c, 0x6c,
}

// staticBootstrapCipher builds the static first-packet cipher once.
// BlowfishCipher is immutable after construction, so every session shares it.
var staticBootstrapCipher = sync.OnceValues(func() (*crypt.BlowfishCipher, error) {
	return crypt.NewBlowfishCipher(staticBootstrapKey)
})

// LoginCrypt is the client side of one login session's dynamic crypt: Encrypt
// pads, checksums and encrypts with the session's dynamic key, as the server's
// Decrypt expects, and Decrypt reads the server's post-Init packets.
// A LoginCrypt is immutable after construction.
type LoginCrypt struct {
	dynamic *crypt.BlowfishCipher
}

// NewClientLoginCrypt builds the client side of a session's crypt from the
// dynamic Blowfish key (1-56 bytes) Init delivered.
func NewClientLoginCrypt(dynamicKey []byte) (*LoginCrypt, error) {
	dynamic, err := crypt.NewBlowfishCipher(dynamicKey)
	if err != nil {
		return nil, fmt.Errorf("dynamic session cipher: %w", err)
	}
	return &LoginCrypt{dynamic: dynamic}, nil
}

// DecryptFirst reverses the first-packet encryption in place on the client
// side: it decrypts buf with the static bootstrap key and undoes the rolling
// XOR pass, leaving the padded Init payload.
func DecryptFirst(buf []byte) error {
	if len(buf) < 2*crypt.BlockSize || len(buf)%crypt.BlockSize != 0 {
		return fmt.Errorf("first login packet length %d is not a multiple of %d of at least %d", len(buf), crypt.BlockSize, 2*crypt.BlockSize)
	}
	static, err := staticBootstrapCipher()
	if err != nil {
		return fmt.Errorf("static bootstrap cipher: %w", err)
	}
	crypt.DecryptBlocks(static, buf)
	unxorPass(buf)
	return nil
}

// Encrypt pads payload to a Blowfish block boundary, appends a checksum and
// encrypts it with the session's dynamic key, returning the (longer)
// encrypted packet body.
func (c *LoginCrypt) Encrypt(payload []byte) []byte {
	buf := make([]byte, crypt.PaddedSize(len(payload)+4))
	copy(buf, payload)
	crypt.AppendChecksum(buf)
	crypt.EncryptBlocks(c.dynamic, buf)
	return buf
}

// Decrypt decrypts payload in place with the session's dynamic key and
// verifies its checksum, returning an error if payload's length is not a
// whole number of Blowfish blocks or the checksum does not match.
func (c *LoginCrypt) Decrypt(payload []byte) error {
	if len(payload) == 0 || len(payload)%crypt.BlockSize != 0 {
		return fmt.Errorf("login packet length %d is not a positive multiple of %d", len(payload), crypt.BlockSize)
	}
	crypt.DecryptBlocks(c.dynamic, payload)
	if !crypt.VerifyChecksum(payload) {
		return fmt.Errorf("login packet checksum verification failed")
	}
	return nil
}

// unxorPass reverses the server's first-packet rolling XOR pass: starting from
// the rolled key stored before buf's final word, it walks the words back to
// offset 4, recovering each original word and unrolling the key by it.
func unxorPass(buf []byte) {
	pos := len(buf) - 8
	ecx := binary.LittleEndian.Uint32(buf[pos : pos+4])
	for pos -= 4; pos >= 4; pos -= 4 {
		orig := binary.LittleEndian.Uint32(buf[pos:pos+4]) ^ ecx
		ecx -= orig
		binary.LittleEndian.PutUint32(buf[pos:pos+4], orig)
	}
}
