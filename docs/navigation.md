# Bot navigation redesign

Status: approved by the owner on 2026-11-21:

- D13 option A: the bot plans its own routes. Condition: geodata is never loaded whole into
  memory, and it is used the way the game server uses it.
- D13a option 1: walking personas require geodata.

Revised after an independent review of the first draft. That review found a mid-walk step check
the draft missed, one-way ledges, doors left out of the hierarchy, and Action approaches described
wrongly.

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

The bot never learns which branch ran. It guesses from progress, and each failure has added
another heuristic:

| Heuristic | Where | Patches over |
| --- | --- | --- |
| Count legs, give up after 3 that gain less than 100 units | `spots.go` (`spotStalls`, `spotProgress`), `visit.go`, `follow.go` | the bot cannot tell a wall from a slow route |
| Hop from one spot centre to the next (`spotWaypoint`) | `spots.go` | a leg ending on ground no route reaches (#133, #137) |
| Avoid a spot for an hour after it stalls (`spotUnreachable`) | `spots.go` | no reachability test (#144) |
| Seven sidesteps, then `/unstuck` | `unstick.go` | a click into a pocket the server cannot route out of (#122) |
| Wall detours: about 13 state fields, 8 detours per wall | `follow.go` | follow legs stalling at walls and ledges (#128, #141, #147) |
| Back NPC errands off after a walk that got nowhere (`walkStuck`) | `visit.go` | the same (#134, #135, acis_golang#5412) |
| Melee, cast and talk by `Action`, with no line check | `combat*.go`, `visit.go` | the server's tracking walk to the target is straight and never routed (see below), so a wall between the two blocks it, and the attack think sends the same walk again |
| Pick-up by `Action` | `loot.go` | the server walks to the item with full route resolution, its pathfinder included |

Open issues #144, #147 and #148 are more of the same. Another heuristic will not end this. The
redesign removes the cause.

## Goal

**Every move a bot sends is a segment the bot has checked with the server's own code, run on the
server's own geodata. Each segment is checked twice:**

1. **`CanMove(from, end)`**, so `resolvePathLocked` takes `pathDirect`, and the server's
   pathfinder and its `ValidLocation` fallback are never reached;
2. **the step chain the server walks the segment with**, so the walk does not stop partway. The
   server checks `CanMove` again from each 100 ms step point to the next (see "Server behavior").

Routing, reachability and recovery all happen in the bot, which knows the terrain. The same rule
covers every server-side walk the bot starts: clicks, tracking walks to a target (melee, cast,
talk) and pick-up walks.

Non-goals:

- server changes (none are needed);
- flying and boats; swimming is a gap (see "Gaps");
- a navigation mesh in another format: the bot uses the server's geodata cells as they are, so its
  answers match the server's exactly.

## Server behavior this rests on

Read from the server checkout. N0 confirms each point with tests, and anything new it finds is
written back into this list.

**The click** (`network/movement.go`):

- the target's Z is raised by the collision height (floor to head);
- the target must be less than 9900 units (3D) from the packet's origin;
- the walk runs from the server's own position, never from the packet's origin;
- a click while out of control, rooted, at zero speed, or mid-swing or mid-cast is answered
  `ActionFailed`. A mid-swing or mid-cast click is still carried out once the swing or cast ends
  (`movement.go:23-33, 74-91`).

**Route resolution** (`creature.go:917`) runs for clicks and pick-ups only. A tracking pawn walk
(melee, cast, talk, friendly follow) and a straight walk skip it: they head straight for the
target and never route (`creature.go:853-860`; callers in `controller.go:546-552`,
`model/actor/player/character_attack.go:708`, `network/targeting.go:579`, `network/magic_skill.go:487`).

**`CanMove`** (`geo/engine/engine_query.go:19-83`):

- walks the cells along the line;
- each border crossed must be allowed by the NSWE flags of the cell left;
- the next cell must have a layer at or below `z + CellIgnoreHeight` (48);
- the walk holds when the final layer is `Height(target)`.

So **a walker steps down any height but up at most 48 units**, which makes ledges and pits
one-way. `CanMove` is a pure function of geodata and the door overlay.

**The walk itself** (`creature.go:1290-1330, 1380-1400`):

- every `PositionUpdateInterval` (100 ms), a player's step length is speed × the measured
  elapsed milliseconds (`playerPassed`, `:408`);
- the step point is rounded half up, and its Z is `Height(next, origin.Z + 2*CellHeight)`;
- the server checks `CanMove(stepFrom, stepTo)`. If that fails, the walk stops there, and the
  server broadcasts a `MoveToLocation` whose origin and destination are both the stop point
  (`BroadcastBlockedCorrection`, `controller.go:748`);
- on a normal arrival the mover receives nothing.

**Pick-up** (`network/targeting.go:265-275`): a `MoveToLocation` to the item's raw position (Z not
raised), with full route resolution. The server answers `ActionFailed` even when the pick-up
succeeds. So does talking (`:565`).

**Unlisted regions** (`geo/block/null.go:45-61`): a region with no geodata is open in every
direction, but its layer heights read as 0. A walk from loaded geodata into such a region lands at
Z 0. **The bot must load exactly the regions the server lists in `geoengine.properties`** (139 in
the shipped config, all present as `*_conv.dat`), in the same `GeoDataType`.

**Doors** (`geo/dynamic`, `geo/engine/engine_dynamic.go`): doors from `doors.xml` are overlaid on
the static cells while closed. The client learns door state from `DoorInfo`, which maps an object
id to a door, and `DoorStatusUpdate`, which carries only the object id and the state
(`network/serverpackets/worldobjects.go:60-63`).

**Other characters** never block movement. Geodata and doors are the only obstacles.

**`cmd/geoprobe`**: evaluates geodata queries (height, canMove, LOS, path) against the server
engine. With `-expected-dump` it re-evaluates a dump's queries and reports how far its answers
agree. It loads every region file in the directory and no doors (`geo/geodata/load.go:87-105`).

Points N0 must settle, by test:

- which layer the target's head-level Z selects under a bridge or in a multi-storey building, and
  so the Z the bot sends;
- how sensitive the step chain is to tick jitter near walls and ledges, which decides how many
  step lengths the walker checks (section 3);
- that a `pathDirect` walk's `MoveToLocation` echoes the requested X and Y exactly.

## Design

```
internal/geo       copied server geodata: stored form, mapped cache, Height/CanMove/CanSee,
                   step chain, doors overlay; readers used only by `bots geopack`
internal/nav       pure planning on geo: snap, directed reachability, routes, stand points
internal/botclient door table (process-wide), walker (one per bot, brain-owned)
                   consumers: spots, trips, follow, visits, combat approach, pick-up, idle, flee
cmd/bots           geopack (geodata cache), navbuild (nav cache)
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
   protected by a CRC-32C and keyed by each source file's size and mtime (`cache.go:21-27`). The
   bot gets `bots geopack -geodata DIR -geoengine FILE -out geodata.cache`, copied from that
   command. A cache built by the server's `geopack` is accepted only when the bot's region files
   carry the same sizes and mtimes; `rsync -a` keeps both.
2. **Map at run time.** As the server's `-geodata-cache` does (`geodata.LoadRegionsCached`,
   `openCache`, `cache_mmap.go`), the bot maps the cache file read-only (`MAP_PRIVATE`,
   `PROT_READ`). `block.ParseRegion` builds each region as a view over the mapped bytes
   (`viewRegion`), with no copy and no decode. After the open-time checks it advises random access
   (`MADV_RANDOM`), so later queries fault in only the pages they touch. As on the server, the
   region directory is needed at run time only to stat the source files the cache is keyed on;
   none of them is read.
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
5. **One engine per process.** Its static regions are written only during load, before the engine
   is shared (the server's `SetRegion` rule). After that every bot reads them without locks.

**What is copied.** About 4.5k non-test lines from `geo/{block,reader,geodata,engine,dynamic}`,
plus the small pieces they depend on, copied the same way:

- `model/location`;
- `model/door` (the door template);
- `model/geometry` (polygon triangulation for door footprints);
- the `doors.xml` reader;
- the properties parser.

The copy covers:

- readers for L2OFF (`*_conv.dat`, the shipped type) and L2J, used only by `bots geopack`;
- the stored form, `ParseRegion` and its validation, and the cache writer and reader;
- the queries `Height`, `HeightNearest`, `NSWE`, `CanMove`, `CanMoveAround`, and
  `CanSee` / `CanSeeActor`, which archers and casters need to choose where to stand. The 32-bit
  overflow quirks are copied as they are, because they are observable behavior;
- **the player step chain**: `stepLocked`'s step-point arithmetic (`playerPassed`, rounding half
  up, ground Z from `Height(next, origin.Z + 2*CellHeight)`) together with the per-step
  `CanMove`, as a pure function `geo.WalkSteps(from, to, speed, stepDurations)`. It returns where
  a walk stops for a given sequence of step durations.

Configuration:

- `-geodata DIR`, `-geoengine FILE` and `-geodata-cache FILE`;
- `-geoengine` is a local copy of the server's `geoengine.properties`. It supplies the region
  list, `GeoDataType`, `MaxObstacleHeight` and `PartOfCharacterHeight`, the last two used by
  `CanSee`;
- the region list goes through the server's `ListedRegions` rule: a key present counts, whatever
  its value; a commented-out line does not;
- at start the bot logs a hash of the region list, to compare with the server's;
- collision heights for `CanSee` come from the bot's own `UserInfo` and from NPC templates
  (datapack).

**Doors, the server's way.** The server keeps doors as an overlay inside the shared engine
(`engine_dynamic.go`):

- readers take one atomic load and a plain map read; on a block a door overlays, they also take
  that `dynamic.Block`'s own mutex (`geo/dynamic/block.go`);
- writers build a new copy and swap it in, serialized by `dynamicBlocksMu`;
- a per-region bitmap means a cell with no door pays no map lookup.

The bot copies that mechanism unchanged. Door objects are built from `doors.xml` with the copied
`dynamic.NewDoorObject`, and each door starts in its template's default state. Runtime state comes
from the door table (section 3).

### 2. `internal/nav`: planning

Nodes are geodata cells, `(geoX, geoY, layer)`. **Edges are directed.** A step from one cell to
its neighbour is allowed exactly when the server's `CanMove` allows that border crossing: down any
height, up at most 48. A diagonal step is allowed when both L-shaped detours are open, as in the
server's pathfinder.

- **Snap** (`Locate`, `NearestWalkable(p, r, scc)`): a world point goes to its cell and layer by
  Z. A target point (spot centre, follow slot, NPC spawn) goes to the nearest walkable cell inside
  the required strongly connected component.
- **Wall clearance cost**: cells next to a blocked NSWE edge, or next to a drop, cost more. Routes
  then keep off walls and ledges. This looks more natural, and it leaves a margin for the step
  chain's jitter and for dead-reckoning error.
- **Hierarchy (HPA\*)**:
  - clusters of 32×32 geodata blocks: 256×256 cells, 4096 units, 64 per region, about 8900 over
    139 regions;
  - entrances on cluster borders, with precomputed directed intra-cluster edge costs;
  - **strongly connected components (SCCs)** labelled on the directed abstract graph, and its
    condensation (the graph of components) stored.
- **Reachability**:
  - `Locate` finds a cell's local component inside its cluster by a flood fill bounded to that
    cluster (at most 256×256 cells per layer).
  - The results are kept in a process-wide LRU of a bounded size, owned by one mutex, so a cluster
    is filled once and not by every bot.
  - After `Locate`, `SameSCC(a, b)` (each way reachable) is O(1).
  - `Reachable(a, b)` one way is a search over the condensation, which is small.
  - **Consumers ask for `SameSCC` by default**, so a bot never walks down into a pit it cannot
    climb out of. Only follow and death return accept a one-way drop.
- **Doors in the hierarchy**:
  - `navbuild` builds every door's footprint from `doors.xml` (the copied `NewDoorObject` over the
    static geodata) and tags each abstract edge that crosses door cells with the door's id.
  - SCCs are labelled with every door open, so they are optimistic.
  - At run time the search skips edges whose door is closed in the shared engine.
  - A route that fails only because of a closed door is reported as `unreachable: door`, never as
    plain unreachable.
- **Routes**:
  - an abstract search over the condensation-restricted graph;
  - refinement inside each cluster the abstract path crosses, reading cells through the mapped
    geodata;
  - string pulling with the copied `CanMove`;
  - splitting into segments of at most `navSegment` (about 2500 units, roughly a click on screen,
    far under 9900 in 3D);
  - **every segment passes both checks of the Goal**, which the planner asserts. A pull that fails
    the step chain is not taken, and the next-shorter pull is tried. Inside the clearance corridor
    a waypoint may get a small random offset, re-checked the same way.
- **Stand points** (`StandPoint(target, range, mode)`): a walkable cell in the bot's SCC within
  `range` of the target. Depending on `mode`, it has:
  - a straight walkable line to the target (melee, cast, talk), the line the tracking walk will
    take;
  - `CanMove` to the item's raw Z (pick-up);
  - `CanSee` with collision heights (bows and spells).
- **Distances for ranking**: abstract-graph distance only, with no refinement. A full route is
  refined only for the destination chosen.
- **Cost bounds**:
  - every search takes a node budget, and running out of budget means unreachable for now;
  - searches run synchronously on the brain goroutine, at most one full route per decision. That
    adds no goroutine and keeps tests deterministic;
  - each query gets its scratch buffers from a `sync.Pool`; the graphs are immutable.
- **Memory**: the same rule as geodata.
  - Nothing in `nav` is sized to the world's cell count: about 6×10^8 cells, so even one byte per
    cell would be 600 MB.
  - A refinement's open set and its closed set (a map) are sized by the node budget, not by the
    region.
- **Build** (`bots navbuild -geodata-cache FILE -geoengine FILE -datapack DIR`): reads the packed
  geodata cache and `doors.xml`, never the raw region files. It writes the cluster graph, the door
  tags, the SCC labels and the condensation to a nav cache file with the geodata cache's
  discipline:
  - a magic and version, a CRC-32C, and keyed by the geodata cache's own CRC, a hash of
    `doors.xml` and the build parameters;
  - written to a temporary file, synced, then renamed;
  - mapped read-only at run time, with the graph read in place.

  A stale or missing nav cache stops the walking personas at start with a clear message; it is
  never rebuilt silently inside `bots ui`. The build works region by region with at most half the
  cores, so its working set stays at a few clusters' scratch per worker.

### 3. `botclient`: door table and walker

**Door table (process-wide).**

- It is one value owned by a mutex.
- It maps object ids to doors (from `DoorInfo`) and records each door's state with the
  reader-side receive time of the packet that set it.
- A bot's brain hands each door event to the table. The table applies an event only when it is
  newer than the last one applied for that door, then toggles the door in the shared engine
  through the engine's writer path. So packets that different bots handle in a different order
  never flip a door back.
- A door no bot has seen keeps its `doors.xml` default.

**Walker (one per bot, owned by the brain goroutine).** It replaces `legToward`, `spotLeg`,
`followLeg`, `walkTo`, `unstick` and the detours. Each consumer asks
`goTo(dest, arrive, purpose)` and gets back an `Action` plus a status: walking, arrived, or
unreachable (with a reason).

- **Position**: the server is the authority (`MoveToLocation` origin, `ValidateLocation`,
  `StopMove`, `TeleportToLocation`). Between those, the bot simulates its own walk with the copied
  step chain at the nominal 100 ms steps. That replaces the linear Z interpolation in `world.go`.
  Arrival is inferred from the simulation, because the server sends nothing on arrival.
- **Step-chain check**: for each segment, `geo.WalkSteps` runs over a sweep of constant step
  durations around 100 ms. N0 sets the range and the count from the measured sensitivity. The
  segment is sent only if every run reaches the end.
- **Sending**:
  - From standstill, the next segment goes out from the server-confirmed position.
  - Clicking ahead while still moving is allowed only where the corridor has at least two cells
    of clearance on both sides. Then the checks must hold from every point of a ±32-unit square
    around the predicted position, which covers error along the walk and across it.
  - Otherwise the walker waits for the simulated arrival.
  - No move goes out while the bot's own swing or cast is in progress, because the server would
    defer it and answer `ActionFailed`.
- **Server answers to its own moves**:
  - A `MoveToLocation` for the bot whose origin equals its destination is a **blocked stop**. The
    step chain differed from the bot's check. The walker counts `nav_blocked_total`, replans from
    the stop point, and avoids the segment's cells for this route.
  - A `MoveToLocation` answering the bot's click with another destination is a **desync**. The
    server routed or truncated, so the bot's geodata or door state differs from the server's. The
    walker counts `nav_desync_total` and replans from the server's origin. If the segment crosses
    a door cell, it counts `nav_door_replan_total` instead, which does not count toward
    unreachable.
  - After repeated blocked stops or desyncs on one route, the walker ends with status
    `unreachable: blocked` or `unreachable: desync` and shows it in the status snapshot, instead
    of looping.
  - `ActionFailed` is not a navigation signal: the server sends it for many other reasons,
    successful pick-up and talk included.
- **Progress watchdog**: from the route length and the bot's speed, the walker knows when each
  segment should arrive (injected clock). This replaces "100 units closer per leg".
- **Recovery**: replan from where the server says the bot is. `/unstuck` is kept for one case
  only: the bot's own SCC holds no destination it needs (a pit, the restart area of
  acis_golang#5412). Sidesteps and wall detours are deleted.
- **Pacing**:
  - one `MoveBackwardToLocation` per segment, through the frame pacer (`gateNone`);
  - segments are at least about 300 units long, so a running bot sends at most about one move per
    second, far below the server's 80 frames/s average;
  - `Action` (melee, cast, talk, pick-up) keeps its existing gates.

### 4. Consumers

| Consumer | Today | Redesigned |
| --- | --- | --- |
| Spot pick and walk (`spots.go`) | straight-line distance, hops, stall counter, one-hour avoid | candidates in the bot's SCC only, ranked by abstract distance; spot centre snapped inside its territory and SCC; `walker.goTo` |
| Trips (`travel.go`) | gatekeeper when nothing is in walking range | Dijkstra over {here, gatekeepers, their destinations, target} on abstract distances: walk edges cost distance over speed, teleport edges cost adena; only legs inside one SCC |
| Follow (`follow.go`) | slot point as drawn, legs, sidesteps, detours | slot snapped into the leader's SCC, one-way drops allowed toward the leader; route; re-plan on the same `Repath` and delay rules; out of sight, route to the last `PartyMemberPosition` |
| NPC visits and errands (`visit.go`, `town.go`, `shop.go`, `trainer.go`, `newbie.go`) | `Action` on the NPC, with no line check | walk to `StandPoint(npc, talkRange, line)`, then `Action`: the tracking walk's straight line is open |
| Melee and casting | `Action` or attack, with no line check | send the attack or cast only while the straight line to the target is open and within range, else walk to a stand point first; a moving monster is checked again at each decision, and a blocked stop moves the bot to a new stand point |
| Pick-up (`loot.go`) | `Action`; the server routes the walk | send it only when `CanMove(self, item at raw Z)` holds and the step chain reaches it; else walk to `StandPoint(item, r, pickup)` first |
| Bows and spells at range | server chase | `StandPoint` with `CanSee` and the skill's or weapon's range |
| Flee (`rules.go`), idle drift, death return | straight point | a point in the bot's SCC (death return may take one-way drops), chosen away from the attacker or inside the drift radius, as checked segments |

### 5. What is deleted

`unstick.go` (except the small-SCC `/unstuck`); the detour and stall fields of `followState`;
`spotWaypoint`, `legToward`, `spotStalls`, `spotProgress`, `spotUnreachable`; `walkStuck` and the
errand back-off it drives; and the tests that pin those heuristics. Each consumer's PR deletes its
own heuristics, and nothing is kept alongside the new code.

## Owner decisions

Both were approved on 2026-11-21.

| # | Decision | Answer |
| --- | --- | --- |
| D13 | Who finds a bot's route | The bot, on its own copy of the server's geodata. Every move it sends is a segment it checked with the copied `CanMove` and step chain; the server's pathfinder is never relied on. Geodata is never loaded whole into memory: it is packed offline and mapped read-only, with regions read in place, as the game server does (section 1) |
| D13a | Walking without geodata | Not supported: `farmer`, `support`, `town` and `merchant` need a fresh geodata cache and nav cache, as `town` and `merchant` already need `-datapack`. Today's legs are not kept as a fallback |

## Testing

This repository is public and the server's datapack, geodata included, is private. So **no real
geodata is committed here, and CI never sees it.**

- **CI fixtures**: synthetic regions written in Go with the copied region builder and packed with
  the copied cache writer:
  - corridor, U-wall, a one-way ledge and a pit, a multilayer bridge, a closed and an open door,
    an enclosed pocket;
  - flat, complex and multilayer blocks.
- **`geo` memory** (CI):
  - opening a packed fixture cache grows the heap (`runtime.MemStats.HeapAlloc`) by less than a
    small fixed bound, far below the file's size;
  - every region's data slices point into the mapped bytes, so nothing was copied;
  - a missing or stale cache is an error, never a decode onto the heap.
- **`geo` codecs** (CI): each reader consumes exactly its input; the stored form round-trips.
- **Parity with the server** (local, against the owner's geodata). The checks below run before
  each PR that touches `internal/geo`, and the PR body reports the result as a "Geodata parity
  (local)" line, with the same rules as e2e scenarios:
  - N0 adds `bots geoquery`, which writes a curated query file in `geoprobe`'s dump format with
    the bot's answers. It covers short-range `canMove` near walls, ledges both ways, multilayer
    heights, `los` and random short pairs.
  - `geoprobe -expected-dump <file> -config <geoengine.properties>` runs from the server checkout
    without editing it, and must report full agreement.
  - A Go test gated on `BOT_GEODATA` checks the step chain against points captured with
    `geoprobe`. Without the variable it skips with a message, as the e2e-live tests do; no build
    tag.
  - Doors are not covered by `geoprobe`; e2e covers them.
- **`nav`** (CI, synthetic regions):
  - every returned path passes both checks of the Goal, segment by segment;
  - on door-free regions, `Route` succeeds exactly when `SameSCC` holds;
  - the pit is never entered by a default (`SameSCC`) route, and the ledge is taken down only by
    the one-way modes;
  - closed doors give `unreachable: door`;
  - budget exhaustion is reported;
  - a property test over random pairs checks the same invariants.
- **`nav` performance**: benchmarks with recorded thresholds, measured on the owner's geodata
  locally and reported in the N3 PR:
  - `Locate` + `SameSCC` p99 ≤ 1 ms with the cluster LRU warm and ≤ 20 ms cold;
  - abstract distance p99 ≤ 2 ms;
  - a full 40000-unit route p99 ≤ 10 ms;
  - heap per route query ≤ 4 MB;
  - `navbuild` ≤ 15 minutes with half the cores, peak heap ≤ 1 GB, nav cache ≤ 100 MB.
- **Door table** (CI, `-race`): out-of-order events from two bots never flip a door back; the
  engine sees the newest state.
- **`walker`** (events and an injected clock in, packets out):
  - segments in order;
  - a same-point `MoveToLocation` replans as blocked;
  - another destination counts a desync, or a door replan on a door segment;
  - no move during the bot's own swing or cast;
  - click-ahead only with clearance;
  - the watchdog;
  - `/unstuck` only in a small SCC.
- **Consumers**: today's rule tests rewritten against a nav built from the synthetic regions, so
  they no longer pin heuristics.
- **e2e** against the live server (`docs/e2e-live.md`), with the owner's geodata:
  - scenarios: a long walk across a region, the dark elven trip (#144), following over ledges
    (#147, #148), the grocer visit (#122), melee and pick-up around obstacles;
  - each asserts `nav_desync_total` = 0 on door-free routes and reports `nav_blocked_total`;
  - a door scenario asserts door replans and no plain unreachable.

## Delivery (milestone M7, "Own navigation")

| PR | Scope | Closes or supersedes |
| --- | --- | --- |
| N0 | Settle the open server points by test: head-level Z on multilayer, step-chain sensitivity, echo of `pathDirect`. Write `bots geoquery`. Record the findings in this document | — |
| N1 | `internal/geo` copy with its dependencies (location, door template, geometry, properties, `doors.xml`): stored form, `ParseRegion` views, cache writer and reader, mmap on Unix and Windows, readers for `bots geopack`, queries, step chain `WalkSteps`, door overlay. Flags `-geodata`, `-geoengine` and `-geodata-cache`; `docs/home.md` steps (rsync with mtimes kept, `bots geopack`). Synthetic fixtures, memory test, local parity run, measured heap and RSS for the 139 regions | — |
| N2 | `internal/nav` on cells: directed edges, clearance cost, smoothing with both checks, snap, stand points (routes inside one cluster) | — |
| N3 | Hierarchy: directed abstract graph, door-tagged edges, SCCs and condensation, the cluster LRU, `bots navbuild` and its cache file, benchmarks against the thresholds above | — |
| N4 | Door table, `DoorInfo` and `DoorStatusUpdate` decoders, toggles into the shared engine; door-aware route search | — |
| N5 | Walker: step-chain position, both checks before sending, blocked and desync handling, watchdog, swing and cast hold. NPC visits, errands, melee, casting, pick-up and ranged approach on stand points. `unstick` sidesteps and `walkStuck` deleted | #122 class |
| N6 | Spots and trips on SCCs and abstract distance; hops, stalls and the hour-long avoid deleted | #144 |
| N7 | Follow on routes with one-way drops toward the leader; detour machinery deleted; flee, idle drift and death return | #147, #148 |
| N8 | e2e scenarios and metrics above; the PR rows of `docs/plan.md` updated for D13 | — |

N1 to N4 change no behavior. From N5 on, each PR moves one set of consumers over and deletes their
heuristics in the same PR. N5 needs N4, so no consumer uses routes that ignore doors.

## Risks

- **The geodata copy drifts from the server's** (the server's geodata is updated and the local copy
  is not). The cache key rejects a changed local copy, and the desync counter catches a changed
  server copy at the first segment. `docs/home.md` keeps the geodata in the same rsync step as the
  datapack.
- **Tick jitter in the step chain.** The sweep of step lengths, the clearance cost, and treating a
  blocked stop as a replan cover it. `nav_blocked_total` measures what is left.
- **Stale door state** (a door no bot sees, changed by a script or timer): it shows up as door
  replans, never as plain unreachable.
- **Multilayer Z** (bridges, buildings): settled in N0, then pinned by the parity queries.
- **Memory and CPU on the owner's machine**: one shared mapped engine and nav cache, both read in
  place; searches bounded by a budget and run synchronously, at most one full route per decision;
  offline builds. The thresholds above are acceptance criteria.
- **Windows mapping**: the server has no Windows mapping to copy. N1 tests the `MapViewOfFile`
  shim in the `windows` job of `.github/workflows/go.yml`.

## Gaps

- Swimming: water zones are not in the bot's world model, and the planner treats water cells as
  ground, while the server switches to swim moves in water (`waterLocked`). Bots rarely enter
  water today. A follow-up issue in M7 tracks it.
- Fences and doors that are not in `doors.xml` (siege, scripted): N4 covers `doors.xml` doors
  only. A follow-up issue in M7 tracks it.
