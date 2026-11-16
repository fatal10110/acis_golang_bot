// Package botclient is the client side of the login and game connections,
// copied from the acis_golang server's internal/botclient: the login-server
// session that yields the session keys, and the game connection that
// authenticates, selects or creates a character, enters the world and logs
// out. Opcode constants are redeclared here; the bot never imports the server.
package botclient
