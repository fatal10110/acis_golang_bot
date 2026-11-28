// Package botclient plays one bot through the real client protocol.
//
// A Bot (session.go) is the per-account loop: it takes a turn of the
// process-wide LoginGate, logs in to the login server, enters the game
// server, plays until the connection ends, and reconnects with backoff.
// While it plays, one reader goroutine decodes frames into botwire events
// and queues them on Events; the Brain, on the Bot's own goroutine, applies
// them to its World model, runs the Persona's rule tables (rules.go) and is
// the connection's only writer, every send bounded by the pacer
// (pacing.go). The persona subsystems (farming, parties, trade, stores,
// valuables, crafting, PvP, Seven Signs) are rules and state the brain runs;
// docs/architecture.md maps them to their files.
//
// The wire codecs live in botwire; opcode constants are redeclared there
// from the acis_golang server, which the bot never imports.
package botclient
