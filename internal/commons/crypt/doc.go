// Package crypt contains the cryptographic primitives copied from the
// acis_golang server: the Blowfish variant used by the login protocol, the
// block checksum and padding helpers, the GS-LS link cipher, raw (unpadded)
// RSA for key and credential blocks, and RSA modulus unscrambling as a client
// recovers the login server's public key from Init. The server-side key
// generation and modulus scrambling are test helpers only.
package crypt
