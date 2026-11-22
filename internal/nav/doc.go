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
// The hierarchy (HPA*) groups cells into clusters of DefaultClusterCells
// square. Entrance nodes are runs of border cells with a crossing, merged
// only inside one cluster-local SCC; directed edges between them carry a
// cost with every door open, the doors that path crosses and a door-free
// cost; the abstract SCCs and their condensation are labelled with every
// door open. `bots navbuild` builds it (Build) into a nav cache file read
// in place (OpenNavCache). Hierarchy.Locate places a cell by Tarjan bounded
// to its cluster on the static blocks: in an abstract SCC, one-way out or
// isolated; results live in a byte-bounded LRU, and walkers pin theirs.
//
// Searches are synchronous on the caller's goroutine and start no goroutine.
// Scratch buffers come from a sync.Pool and grow with the node budget, never
// with the size of the world. Regions without geodata are no-go.
package nav
