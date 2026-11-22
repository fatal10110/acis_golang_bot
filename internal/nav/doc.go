// Package nav plans ground routes on the bot's own copy of the server's
// geodata (docs/navigation.md, section 2).
//
// Nodes are geodata cell layers. Edges are directed and follow the copied
// CanMove exactly: down any height, up at most block.CellIgnoreHeight, and a
// diagonal only when both L-shaped detours are open. A route is a cell A*
// search with a node budget, pulled into straight segments of at most
// navSegment units. Every segment passes CanMove and the step-chain check:
// the cheap open-corridor test, and on the stretches that test fails, the
// exact check over every step sequence the server can take (section 3,
// "Step-chain check").
//
// Searches are synchronous on the caller's goroutine and start no goroutine.
// Scratch buffers come from a sync.Pool and grow with the node budget, never
// with the size of the world. Regions without geodata are no-go.
package nav
