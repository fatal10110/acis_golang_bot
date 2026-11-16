package crypt

import (
	"crypto/rsa"
	"fmt"
	"math/big"
)

// modulusSize is the byte length of a 1024-bit RSA modulus.
const modulusSize = 128

// ModulusBytes returns pub's modulus as a big-endian byte slice compatible
// with a reader that treats the bytes as signed two's-complement (as the L2
// game server does when reconstructing this login server's public key from
// InitLS): since an RSA modulus is always positive, this is pub.N's
// big-endian bytes with a leading 0x00 prepended whenever the top bit of the
// first byte is set, so a signed reader never mistakes the value for
// negative.
func ModulusBytes(pub *rsa.PublicKey) []byte {
	b := pub.N.Bytes()
	if len(b) > 0 && b[0]&0x80 != 0 {
		return append([]byte{0x00}, b...)
	}
	return b
}

// UnscrambleModulus reverses the client-bound modulus scrambling and returns
// the login server's RSA public key (exponent 65537), as a client recovers it
// from Init. Returns an error if scrambled is not 128 bytes.
func UnscrambleModulus(scrambled []byte) (*rsa.PublicKey, error) {
	if len(scrambled) != modulusSize {
		return nil, fmt.Errorf("scrambled modulus is %d bytes, want %d", len(scrambled), modulusSize)
	}
	b := make([]byte, modulusSize)
	copy(b, scrambled)
	for i := 0; i < 0x40; i++ {
		b[0x40+i] ^= b[i]
	}
	for i := 0; i < 4; i++ {
		b[0x0d+i] ^= b[0x34+i]
	}
	for i := 0; i < 0x40; i++ {
		b[i] ^= b[0x40+i]
	}
	for i := 0; i < 4; i++ {
		b[i], b[0x4d+i] = b[0x4d+i], b[i]
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(b), E: 65537}, nil
}
