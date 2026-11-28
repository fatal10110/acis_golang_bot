// Package botwire encodes the packets the bots send and decodes the packets
// they read, on the login and the game connection, from the acis_golang
// server's layouts (redeclared here; the bot never imports the server).
//
// DecodeEvent turns one decrypted game frame into a typed value Event that
// shares no memory with the frame, so it may cross from the reader goroutine
// to the brain. Frames of opcodes no decoder is registered for are dropped
// unparsed. The package knows nothing of the brain: TestBoundary keeps it
// free of botclient.
package botwire
