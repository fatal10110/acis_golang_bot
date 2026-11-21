# Bot navigation redesign

Status: approved by the owner on 2026-11-21: D13 option A (the bot plans its own routes), with
the condition that geodata is never loaded whole into memory and is used the way the game server
uses it, and D13a option 1 (geodata required for walking personas).

Server paths below (`internal/gameserver/...`, `cmd/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang); bot paths name this repository.

## Problem

The bot is blind: it knows only a straight line to wherever it wants to go. Every walk is a
`MoveBackwardToLocation` to a point up to 9000 units away (`spotDirect`) or 3000 along the straight
line (`legToward`). Finding the route is left to the game server's pathfinder.

The server does not offer that as a service. For a ground click,
`CreatureMove.resolvePathLocked` (`model/actor/move/creature.go:917`) does this:

1. if `geo.CanMove(origin, target)` holds, it walks straight (`pathDirect`);
2. else it runs a bounded A* search (`findPathLocked`, `geo/pathfind`), sized for a click on screen
   and for NPC AI, not for crossing a map;
3. else it walks to `geo.ValidLocation`, the first wall on the straight line (`pathFailed`).

The bot never learns which branch ran. All it sees is a `MoveToLocation` whose destination may
differ from the one it asked for, or an `ActionFailed`. So it guesses from progress, and each
failure has added another heuristic:

| Heuristic | Where | Patches over |
| --- | --- | --- |
| Count legs, give up after 3 that gain less than 100 units | `spots.go` (`spotStalls`, `spotProgress`), `visit.go`, `follow.go` | the bot cannot tell a wall from a slow route |
| Hop from one spot centre to the next (`spotWaypoint`) | `spots.go` | a leg ending on ground no route reaches (#133, #137) |
| Avoid a spot for an hour after it stalls (`spotUnreachable`) | `spots.go` | no reachability test (#144) |
| Seven sidesteps, then `/unstuck` | `unstick.go` | a click into a pocket the server cannot route out of (#122) |
| Wall detours: about 13 state fields, 8 detours per wall | `follow.go` | follow legs stalling at walls and ledges (#128, #141, #147) |
| Back NPC errands off after a walk that got nowhere (`walkStuck`) | `visit.go` | the same (#134, #135, acis_golang#5412) |
| Melee, talk and pick-up approach by `Action` | `combat*.go`, `visit.go` | the server chases the target (`MoveToPawn`) with its own pathfinder |

Open issues #144, #147 and #148 are more of the same. Another heuristic will not end this. The
redesign removes the cause.

## Goal

**Every move a bot sends is a straight segment the bot has checked with the server's own geodata
and the server's own `CanMove`.** Then `resolvePathLocked` always takes the `pathDirect` branch,
and no bot click ever reaches the server's pathfinder or its `ValidLocation` fallback. Routing,
reachability and recovery all happen in the bot, which knows the terrain.

Non-goals:

- server changes (none are needed);
- flying and boats; swimming is a gap (see "Gaps");
- a navigation mesh in another format: the bot uses the server's geodata cells as they are, so its
  answers match the server's exactly.

## Server behavior this rests on

Read from the server checkout. N0 confirms each point, and anything new found there is written
back into this list.

- `network/movement.go` `moveLivePlayer`: the target's Z is raised by the collision height (floor
  to head). The target must be less than 9900 units (3D) from the packet's origin. The walk runs
  from the server's own position, never from the packet's origin.
- `geo/engine/engine_query.go` `CanMove`: walks the geodata cells along the line. Each border it
  crosses must be allowed by the NSWE flags of the cell it leaves, and the next cell must have a
  layer at or below `z + CellIgnoreHeight`. The walk holds when the final layer is
  `Height(target)`. It is a pure function of geodata plus the door overlay.
- `geo/block/null.go`: a region with no geodata loaded is open at any height. **The bot must load
  exactly the regions the server lists in `geoengine.properties`, in the same `GeoDataType`.** A
  region the server lists but the bot does not would make the bot's answers differ from the
  server's.
- `geo/dynamic`: doors (`doors.xml`) and fences overlay the static cells while closed. Door state
  reaches the client in the world-object packets (`network/serverpackets/worldobjects.go`).
- Other characters never block movement; geodata and doors are the only obstacles.
- `cmd/geoprobe`: dumps the server engine's answers (height, canMove, LOS, path) for a sample of
  queries. It is the parity oracle for the bot's copy, run from the server checkout without
  editing it.

Points N0 must still settle:

- which layer `Height(tx, ty, tz + collisionHeight)` picks under a bridge or a multi-storey
  building, which decides the Z the bot sends;
- whether a direct walk's `MoveToLocation` always echoes the requested X and Y exactly (the echo
  check below relies on it);
- what the mover receives on arrival (`StopMove`, nothing), and how the server sets Z along a
  walk.

## Design

```
internal/geo       copied server geodata: readers, blocks, mmap cache, Height/CanMove/CanSee, doors
internal/nav       pure planning on geo: snap, reachability, routes, stand points; no brain knowledge
internal/botclient walker (one per bot, brain-owned): runs a route as checked segments
                   consumers: spots, follow, visits, combat approach, travel, idle, flee
cmd/bots navbuild  offline build of the nav hierarchy cache
```

### 1. `internal/geo`: an owned copy of the server geodata (D5)

This is copied the same way as `wire` and `crypt`: the code the bot needs, owned here, never
imported. It is the server's code path, not a re-implementation. The same packed format, the same
in-place views and the same query functions give the same answers.

**How the server holds geodata, and the bot copies it** (owner condition: never load it whole
into memory):

1. **Offline pack.** The server's `cmd/geopack` decodes the listed region files once
   (`geodata.WriteCache`) into one packed cache file, format `ACISGEO` version 2. This is a
   bit-packed stored form with per-block palettes and height residuals (`geo/block/region.go`),
   protected by a CRC-32C and keyed by each source file's size and mtime. The bot gets
   `bots geopack -geodata DIR -geoengine FILE -out geodata.cache`, copied from that command. Both
   write the same format, so a cache built by the server's `geopack` from the same files can be
   used too.
2. **Map at run time.** As the server's `-geodata-cache` does (`geodata.LoadRegionsCached`,
   `openCache`, `cache_mmap.go`), the bot maps the cache file read-only (`MAP_PRIVATE`,
   `PROT_READ`). `block.ParseRegion` builds each region as a view over the mapped bytes
   (`viewRegion`), with no copy and no decode. After the open-time checks it advises random access
   (`MADV_RANDOM`), so later queries fault in only the pages they touch.
3. **What is in memory.** The Go heap holds the engine and one small region header per region.
   Cell data stays in file-backed page-cache pages, which the kernel drops under memory pressure
   and reloads on the next touch. They are not anonymous memory, and `GOMEMLIMIT` never counts
   them. Like the server, the bot checks the CRC and validates each region when it opens the
   cache, which reads the file through once. Those pages are reclaimable and are not pinned.
4. **No load-onto-heap mode.** The server falls back to decoding region files onto the heap when
   it has no cache or on a platform without mmap (`LoadRegions`; `cache_read.go` for non-Unix).
   The bot copies neither fallback:
   - a missing or stale cache stops the walking personas at start with "run `bots geopack`";
   - on Windows (release builds, #110) the mapping goes through `CreateFileMapping` and
     `MapViewOfFile` (`golang.org/x/sys/windows`, read-only).

   This is the one place the bot's copy differs from the server. It changes only the platform
   shim, never the format, the views or the queries.
5. **One engine per process.** It is read-only after load (the server's `SetRegion`-before-share
   rule) and shared by every bot without locks.

What is copied, about 2.5k lines from `geo/{block,reader,geodata,engine,dynamic}`:

- readers for L2OFF (`*_conv.dat`, the shipped type) and L2J, used only by `bots geopack`;
- the stored form, `ParseRegion` and its validation, and the cache writer and reader;
- queries: `Height`, `HeightNearest`, `NSWE`, `CanMove`, `CanMoveAround`, and `CanSee` (archers
  and casters need line of sight to choose where to stand). The 32-bit overflow quirks are copied
  as they are, because they are observable behavior.

Configuration:

- `-geodata DIR`, `-geoengine FILE` and `-geodata-cache FILE`. The second is a local copy of the
  server's `geoengine.properties` (region list, `GeoDataType`, `MaxObstacleHeight`), copied over
  like the datapack XML in `docs/home.md`.
- The region list goes through the server's `ListedRegions` rule: a key present counts, whatever
  its value; a commented-out line does not.
- At start the bot logs a hash of the region list, to compare with the server's.

**Doors, the server's way.** The server keeps doors as an overlay inside the shared engine
(`engine_dynamic.go`). Readers take one atomic load and a plain map read, with no lock. Writers
build a new copy and swap it in, serialized by `dynamicBlocksMu`. A per-region bitmap means a cell
with no door pays no map lookup. The bot copies that mechanism unchanged:

- door objects are built from `doors.xml` with the copied `dynamic.NewDoorObject`;
- each door starts in its template's default state;
- a door packet received by any bot toggles the door in the one shared engine. The toggle is
  idempotent and takes the engine's own writer mutex.

This is shared mutable state with a named synchronization mechanism, the engine's, as CLAUDE.md
section 4 requires. It also matches the server: there is one world, with one state per door.

### 2. `internal/nav`: planning

Nodes are geodata cells, `(geoX, geoY, layer)`. A step between neighbours is allowed exactly when
the server's `CanMove` would allow that border crossing. A diagonal step is allowed when both
L-shaped detours are open, as in the server's pathfinder.

- **Snap** (`Locate`, `NearestWalkable(p, r, component)`): a world point goes to its cell and
  layer by Z. A target point (spot centre, follow slot, NPC spawn) goes to the nearest walkable cell
  inside the required component.
- **Wall clearance cost**: cells next to a blocked NSWE edge cost more. Routes then keep off walls
  and ledges. This looks more natural, and it leaves a margin for the small error between the bot's
  dead-reckoned position and the server's.
- **Hierarchy (HPA\*)**: clusters of 32×32 geodata blocks (256×256 cells, 4096 units; 64 per
  region), with entrances on cluster borders and precomputed intra-cluster edge costs. The
  abstract graph holds about 10^5 nodes over the shipped 143 regions. Connected components are
  labelled on it.
  - `Reachable(a, b)` is O(1): compare component ids. Spot picking, errands and trips stop trying
    places they can never reach (the cause of #144, and of the hour-long avoidance).
  - A route of 40000 units is an abstract search over a few hundred nodes, then refinement inside
    each cluster on the way.
- **Smoothing**: string pulling over the cell path with the copied `CanMove`. The result is a
  waypoint list in which **every consecutive pair passes `CanMove`**. The planner asserts this and
  never returns a path that fails it.
- **Segments**: waypoints are split so each segment is at most `navSegment` (about 2500 units,
  roughly a click on screen, far under 9900 in 3D). Inside the clearance corridor a waypoint gets a
  small random offset, and the offset point is checked with `CanMove` again.
- **Stand points** (`StandPoint(target, range, needLOS)`): a walkable cell within `range` of the
  target, with `CanMove(stand, target)` for melee, talk and pick-up, or with `CanSee` for bows and
  spells.
- **Cost bounds**: every search takes a node budget, and running out of budget means unreachable
  for now. Searches run on the brain goroutine, synchronously. That adds no goroutine, gives
  deterministic tests, and costs milliseconds. Each query gets scratch buffers from a
  `sync.Pool`; the graphs themselves are immutable.
- **Memory**: the same rule as geodata.
  - Nothing in `nav` is sized to the world's cell count: about 6×10^8 cells, so even one byte per
    cell would be 600 MB.
  - A refinement searches only the cells of the clusters the abstract path crosses, reading them
    through the mapped geodata. Its open set and its closed set (a map) are sized by the node
    budget, not by the region.
  - Which local component a cell belongs to inside its cluster is found by a flood fill bounded to
    that cluster (at most 256×256 cells per layer), in the query's scratch.
- **Build** (`bots navbuild -geodata-cache FILE -geoengine FILE`): reads the packed geodata
  cache, never the raw files. It writes the cluster graph and component labels to a nav cache file
  with the same discipline as the geodata cache:
  - a magic and version, a CRC-32C, and keyed by the geodata cache's own CRC and the build
    parameters;
  - written to a temporary file, synced, then renamed;
  - mapped read-only at run time, with the graph read in place.

  A stale or missing nav cache stops the walking personas at start with a clear message; it is
  never rebuilt silently inside `bots ui`. The build works region by region with at most half the
  cores, so its working set stays at a few clusters' scratch per worker.

### 3. `botclient` walker: running a route

One `walker` per bot, owned by the brain goroutine. It replaces `legToward`, `spotLeg`,
`followLeg`, `walkTo`, `unstick` and the detours. Each consumer asks it
`goTo(dest, arrive, purpose)` and gets back an `Action` plus a status: walking, arrived, or
unreachable (with a reason).

- **Position**: the server is the authority (`MoveToLocation` origin, `ValidateLocation`,
  `StopMove`, `TeleportToLocation`). While walking, Z comes from `geo.Height` at the dead-reckoned
  XY, replacing the linear Z interpolation in `world.go`.
- **Before each send**: `CanMove(from, segmentEnd)` on the shared engine, doors included. `from` is the
  server-confirmed position when standing still. When clicking ahead while still moving, the check
  must hold from the predicted point and from points ±32 units along the walk; if it does not, the
  walker waits for the stop.
- **Echo check**: a direct walk's `MoveToLocation` must carry the requested destination. Any other
  destination, or an `ActionFailed`, means the server did not take `pathDirect`, so the bot's
  geodata or door state differs from the server's. The walker counts it in the metrics
  (`nav_desync_total`), logs it once per segment, replans from the server's origin, and blocks that
  segment's cells for this bot for a while. After repeated desyncs it ends with status
  `unreachable: desync` and shows in the status snapshot, instead of looping. In a healthy run this
  counter stays at zero, which makes it the live parity signal.
- **Progress watchdog**: from the route length and the bot's speed, the walker knows when each
  segment should arrive (injected clock). This replaces "100 units closer per leg".
- **Recovery**: replan from where the server says the bot is. `/unstuck` is kept for one case
  only: the bot's own cell sits in a component too small to hold any destination (a pit, the
  restart area of acis_golang#5412). Sidesteps and wall detours are deleted.
- **Pacing**: one `MoveBackwardToLocation` per segment, through the frame pacer (`gateNone`).
  Segments are at least about 300 units long, so a running bot sends at most about one move per
  second, far below the server's 80 frames/s average.

### 4. Consumers

| Consumer | Today | Redesigned |
| --- | --- | --- |
| Spot pick and walk (`spots.go`) | straight-line distance, hops, stall counter, one-hour avoid | candidates filtered by `Reachable` from the bot's component and ranked by route length; spot centre snapped inside its territory; `walker.goTo` |
| Trips (`travel.go`) | gatekeeper when nothing is in walking range | Dijkstra over {here, gatekeepers, their destinations, target}: walk edges cost route length over speed, teleport edges cost adena; only reachable legs |
| Follow (`follow.go`) | slot point as drawn, legs, sidesteps, detours | slot snapped into the leader's component; route; re-plan on the same `Repath` and delay rules; out of sight, route to the last `PartyMemberPosition` |
| NPC visits and errands (`visit.go`, `town.go`, `shop.go`, `trainer.go`, `newbie.go`) | `Action` on the NPC, the server walks the approach | walk to `StandPoint(npc, talkRange)`, then `Action`: the server's `MoveToPawn` for the last stretch is a direct line |
| Melee, loot pick-up | `Action`/attack, the server chases | the same rule: send the attack or pick-up only while `CanMove(self, target)` holds within range, else walk to a stand point first; a moving monster is re-checked at each decision |
| Bows and spells | server chase | `StandPoint` with `CanSee` and the skill's or weapon's range |
| Flee (`rules.go`), idle drift, death return | straight point | a point in the bot's component, chosen away from the attacker or inside the drift radius, as one checked segment |

### 5. What is deleted

`unstick.go` (except the small-component `/unstuck`); the detour and stall fields of
`followState`; `spotWaypoint`, `legToward`, `spotStalls`, `spotProgress`, `spotUnreachable`;
`walkStuck` and the errand back-off it drives; and the tests that pin those heuristics. Each
consumer's PR deletes its own heuristics, and nothing is kept alongside the new code.

## Owner decisions

Both were approved on 2026-11-21.

| # | Decision | Answer |
| --- | --- | --- |
| D13 | Who finds a bot's route | The bot, on its own copy of the server's geodata. Every move it sends is a segment it checked with the copied `CanMove`; the server's pathfinder is never relied on. Geodata is never loaded whole into memory: it is packed offline and mapped read-only, with regions read in place, as the game server does (section 1) |
| D13a | Walking without geodata | Not supported: `farmer`, `support`, `town` and `merchant` need a fresh geodata cache and nav cache, as `town` and `merchant` already need `-datapack`. Today's legs are not kept as a fallback |

## Testing

- `geo` memory: open a packed fixture cache and assert that heap growth (`runtime.MemStats.HeapAlloc`) stays under a small fixed bound, far below the file's size. Assert that every region's data slices point into the mapped bytes (no copy). Assert that a missing cache is an error, not a decode onto the heap.
- `geo`: decode fixture regions (the P4 subset, fatal10110/acis_golang#4528) and check that each
  decoder consumes exactly its input. Run `cmd/geoprobe` from the server checkout to dump golden
  answers for a fixed sample (height, canMove, LOS) over the subset; the bot's copy must match all
  of them (table test against the dump stored in `testdata`).
- `nav`: table tests on small hand-built regions (corridor, U-wall, ledge, multilayer bridge,
  closed and open door, an enclosed pocket). Every returned path passes `CanMove` pair by pair.
  `Route` succeeds exactly when `Reachable` holds. Budget exhaustion is reported. A property test
  over random pairs in a fixture region checks the same invariants. Benchmarks: p99 route time for
  a 40000-unit trip, `navbuild` time and size, and heap per route query (bounded by the node
  budget, not by the region size).
- `walker`: events and an injected clock in, packets out: segment order; an echo mismatch causes a
  replan and a desync status; the watchdog; the click-ahead check; `/unstuck` only in a small
  component.
- Consumers: today's rule tests rewritten against a fake nav built from a fixture region, so they
  no longer pin heuristics.
- e2e on the stack with the P4 subset: a long walk across a region, the dark elven trip (#144),
  following over ledges (#147, #148), and the grocer visit (#122). Each asserts zero
  `nav_desync_total` and zero `ActionFailed` answers to moves.

## Delivery (milestone M7, "Own navigation")

| PR | Scope | Closes or supersedes |
| --- | --- | --- |
| N0 | Settle the open server points above; capture a `geoprobe` golden dump over the P4 subset; record the findings in this document | — |
| N1 | `internal/geo` copy: stored form, `ParseRegion` views, cache writer and reader, mmap on Unix and Windows, L2OFF and L2J readers for `bots geopack`, queries; `-geodata`, `-geoengine`, `-geodata-cache`; `docs/home.md` steps; parity test against the golden dump; memory test; measured heap and RSS for the full 143 regions | — |
| N2 | `internal/nav` cell A\*, clearance cost, smoothing, snap and stand points (no hierarchy yet: routes inside one cluster) | — |
| N3 | Hierarchy, components, `bots navbuild`, cache file, benchmarks | — |
| N4 | Walker, echo check, Z from geodata; NPC visits, errands, melee, pick-up and ranged approach on stand points; `unstick` sidesteps and `walkStuck` deleted | #122 class |
| N5 | Spots and trips on reachability and route length; hops, stalls and the hour-long avoid deleted | #144 |
| N6 | Follow on routes; detour machinery deleted; flee and idle drift | #147, #148 |
| N7 | Doors: `doors.xml` reader, door packet decoders, the copied engine overlay (`dynamic.NewDoorObject`, clone-and-swap toggle) shared by every bot | — |
| N8 | e2e scenarios and metrics above; the PR rows of `docs/plan.md` updated for D13 | — |

N1 to N3 change no behavior. From N4 on, each PR moves one set of consumers over and deletes their
heuristics in the same PR.

## Risks

- **The geodata copy drifts from the server's** (the server's geodata is updated and the local copy
  is not). The nav cache key rejects a changed local copy, and the echo check catches a changed
  server copy at the first segment. `docs/home.md` keeps the geodata in the same rsync step as the
  datapack.
- **Dead-reckoning error near walls.** The clearance cost, the ±32-unit check when clicking ahead,
  and waiting for the stop when the check fails cover it.
- **Multilayer Z** (bridges, buildings): settled in N0, then pinned by golden vectors.
- **Memory and CPU on the owner's machine**: one shared mapped engine and nav cache, both read in
  place; searches bounded by a budget and run synchronously; offline builds. N1 and N3 report
  measured heap, RSS and file sizes.
- **Windows mapping**: the server has no Windows mapping to copy. N1 tests the
  `MapViewOfFile` shim in the `windows` job of `.github/workflows/go.yml`.

## Gaps

- Swimming: water zones are not in the bot's world model, and the planner treats water cells as
  ground. Bots rarely enter water today; a follow-up issue tracks it once N2 lands.
- Fences and siege doors beyond `doors.xml`: N7 covers `doors.xml` only.
