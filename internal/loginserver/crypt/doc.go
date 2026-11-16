// Package crypt is the client side of the login server's packet crypt: it
// decrypts the server's first packet (Init) with the static bootstrap key, and
// encrypts and decrypts every later packet with the session's dynamic key.
package crypt
