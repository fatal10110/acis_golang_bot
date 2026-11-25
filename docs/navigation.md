# Bot navigation redesign

Status: approved by the owner on 2026-11-21:

- D13 option A: the bot plans its own routes. Condition: geodata is never loaded whole into
  memory, and it is used the way the game server uses it.
- D13a option 1: walking personas require geodata.

Revised after four rounds of independent review; the fourth found it sound to file. The rounds
found, in order:

1. a mid-walk step check the first draft missed, one-way ledges, doors left out of the hierarchy,
   and Action approaches described wrongly;
2. a step-chain check that depended on the order of steps, cluster-level component membership,
   door-edge tagging, and how door events are ordered;
3. the cost and the first step of the step-chain check, entrance granularity, door-dependent
   cluster labels, and cache size;
4. one-way-out ground being mistaken for a pit.

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
- flying and boats; the hierarchy keeps out of water, and only the cell planner swims, where a walk
  has no dry route (see "Water" and "Swimming");
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
  elapsed milliseconds (`playerPassed`, `:408`). So the length varies with each tick's real
  timing;
- the first 5 updates of a move use the start speed, which for a player is the walk speed
  (`walkStartUpdates`, `:34`, `:443-447`). The end of a walk is run out in fixed 100 ms steps
  (`runOutLegLocked`, `:1084-1095`). The server keeps one destination. A click while already
  moving first runs a catch-up step along the current destination, speed × the time since the
  last update, bounded like every step and checked with `CanMove` (`catchUpLocked`, `:1482`),
  then starts a fresh click from that point with route resolution (`moveToLocationLocked`,
  `:876`). A retarget in flight keeps the start-speed update count (`:880-889`) (#308);
- the step point is rounded half up, and its Z is `Height(next, origin.Z + 2*CellHeight)`;
- the server checks `CanMove(stepFrom, stepTo)`. If that fails, the walk stops there, and the
  server broadcasts a `MoveToLocation` whose origin and destination are both the stop point
  (`BroadcastBlockedCorrection`, `controller.go:748`). A blocked approach to talk to an NPC
  answers differently: `ActionFailed`, then a `StopMove`, and the talk goes ahead anyway
  (`network/targeting.go:690-704`);
- on a normal arrival the mover receives nothing.

**Pick-up** (`network/targeting.go:265-275`): a `MoveToLocation` to the item's raw position (Z not
raised), with full route resolution. The server answers `ActionFailed` even when the pick-up
succeeds. So does talking (`:565`).

**Unlisted regions** (`geo/block/null.go:22-61`): a region with no geodata is open in every
direction. A query that resolves a layer reads height 0, while `HeightNearest` echoes the queried
Z. `CanMove` ends by comparing the two (`engine_query.go:84`), so a walk into an unlisted region
holds only for a target Z of 0. The planner treats unlisted regions as no-go. **The bot must load
exactly the regions the server lists in `geoengine.properties`** (139 in the shipped config, all
present as `*_conv.dat`), in the same `GeoDataType`.

**Doors** (`geo/dynamic`, `geo/engine/engine_dynamic.go`): doors from `doors.xml` are overlaid on
the static cells while closed. The client learns door state from `DoorInfo` and
`DoorStatusUpdate`. Both carry the object id and the door id; the update also carries the
open/closed state (`network/serverpackets/worldobjects.go:60-70`).

**Dynamic objects outside `doors.xml`** (#162). The engine's `AddObject` and `RemoveObject` have
two callers, doors and fences:

- **Doors: all come from `doors.xml`.** `newDoor` (`data/manager/worldobjects.go:616-646`) is the
  only place a door is built. Its callers are `newDoors` at startup and `ReloadDoors`, and both
  read a `door.Table` loaded from `doors.xml`: at startup by `cmd/gameserver/data.go:176`, passed
  to `NewWorldObjects` at `cmd/gameserver/world.go:42`, and on `//reload` by
  `cmd/gameserver/reload.go:57` through `network/admin_reload.go:283` (the `cmd/` paths are
  relative to the repository root). Sieges, clan halls and scripts never build doors. They change
  the state or HP of a door that is already there, by its door id (`SetDoorSiege`, `UpgradeDoor`,
  `SetClanCall`, `worldobjects.go:199-244`). The door table and the hierarchy's door sets
  therefore already cover every door.
- **Fences: GM-only, not modeled.** The only other dynamic object is a fence
  (`fence/manager.go:90-98` adds it, `:116` removes it). Only the admin commands
  `//spawnfence` and `//deletefence` create and remove fences (`network/admin_fence.go:37, 65`).
  No script, siege or event places one, so fences do not reach the places bots go. A fence would
  also be hard to model:
  - The client learns of it from `ExColosseumFenceInfo` (`0xFE`/`0x0009`, object id, type, x, y,
    z, size x, size y; `network/serverpackets/fence.go`). The packet does not carry the height.
  - A fence that leaves view and a fence that is deleted reach the client as the same
    `DeleteObject`. A bot could not tell a fence still standing out of its view from a removed
    one, so a modeled fence would go stale.
  - It has no door id, so it cannot be in the offline door sets.

  If a GM puts a fence across a route, the server either stops the walker's segment at the fence
  (a blocked stop, `nav_blocked_total`) or answers with a route around it (a desync,
  `nav_desync_total`). The walker replans each time. After `walkMaxBlocked` or `walkMaxDesync`
  of them it gives up with that reason (`internal/botclient/walker.go`).

**Other characters** never block movement. Geodata, doors and GM fences are the only obstacles.

**`cmd/geoprobe`**: evaluates geodata queries (height, canmove, los, path, validlocation) against
the server engine:

- with `-expected-dump` it reads a dump written by anyone (`datadiff.ReadDump`), turns each record
  id such as `canmove:x,y,z->x,y,z` back into a query (`probe.ParseQuery`), and reports how far
  its answers agree (`cmd/geoprobe/main.go:186-212`, `geo/probe/query.go:41-82`);
- it has no walk (step-chain) category (asked in fatal10110/acis_golang#5435);
- it loads every region file in the directory onto the heap, ignoring the region list, and loads
  no doors (`geo/geodata/load.go:87-105`);
- without `-geodata`, it takes the directory from the config's `GeoDataPath`, which is relative
  to the server checkout (`main.go:80-87`).

**Settled by N0** (#152), from the server code and two checks: the live scenario
`TestLiveNavFacts` (`internal/e2e/navfacts_live_test.go`) and the `bots geoquery` parity file.

- **Head-level Z on multilayer, and the Z the bot sends.**
  - The click's target Z is the packet's Z plus the collision height (`network/movement.go:42`).
    The packet's origin is used only for the 9900 check (`:51-56`).
  - A direct walk keeps that head-level Z as its destination and broadcasts it
    (`creature.go:917-938`). Each update first lands the destination on
    `Height(dest.X, dest.Y, dest.Z)` (`creature.go:1368-1370`), and the finish does the same
    (`:1180-1182`). Steps on the way land on the layer nearest `origin.Z + 2*CellHeight`, that is
    16 above the current Z (`:1397`).
  - `Height` is the nearest layer. Of two equally near layers the lower one wins
    (`geo/engine/engine.go:133-135`, `geo/block/multilayer.go:46-75`; the bot's copy has the same
    rule).
  - So the bot sends the floor Z of the layer it wants. With collision height `c`, the server lands
    on layer `L` whenever the next layer above is at least `2c` higher: `L + c` is then no farther
    from `L` than from the upper layer, and ties go down. Layers below never compete. A layer with
    another layer less than `2c` above it cannot be clicked to, because the server takes the upper
    one. The planner must not put a click target on such a cell.
  - Pinned in CI by `TestEvaluate` (`internal/geo/geoquery`) on the synthetic bridge: the deck is at
    160 and the ground at 0. At head level 24 each layer picks itself, 80 ties to the ground and 81
    picks the deck. Pinned against the server by the parity file's multilayer heights: at head
    offsets 0 to 48 above every layer, and at each layer pair's midpoint and one either side.
- **`Emax`, measured live.**
  - Method: the server sends no per-update position, but it answers a `ValidatePosition` that
    drifts more than the move speed from its own position with `ValidateLocation`. That reply
    carries the position as of the last update (`network/movement.go:185-198`,
    `creature.go` `Position`).
  - On a straight direct run, that position is the origin plus speed × (last update − move start).
    So a reply's age (the time since the last update) is the time since the click less
    distance / speed, plus a constant per leg. The speed is fitted per leg.
  - Polls at random phases see ages from about 0 up to the longest gap between updates, so
    `Emax ≈ max − min` age per leg. The scenario polls at most 5 times a second through the client
    pacer; the reported position sits two run speeds off the leg, so it is corrected and never
    adopted.
  - Value (2026-11-22, live server, two runs of 551 polls over 6 legs of 2000 at run speed 104,
    about 110 s each): max 115 and 141 ms; age p50 54 and 65, p90 93 and 106, p99 106 and 122 ms.
    Network jitter between polls is included, so these are upper estimates at that load.
  - The elapsed time is not capped (`playerPassed`, `creature.go:408-415`). The ticker owes its
    runs at a fixed interval and runs missed ones at once (`task/positionupdates.go:12, 35-37`). So
    a server stall gives one long step followed by near-zero ones, and `Emax` grows with load.
  - The walker (N5) takes `Lmax` = run speed × `Emax`, with a margin over the measured 141 ms. The
    residual is measured by `nav_blocked_total` (Risks).
- **`pathDirect` echo: confirmed.**
  - A direct walk's `MoveToLocation` carries the clicked X and Y exactly, and Z = clicked Z +
    collision height. There is no rounding, clamping or `ValidLocation` trimming on the direct
    path, and the frame writes the destination verbatim
    (`network/serverpackets/movetolocation.go:13-22`).
  - Its origin is the server's position, caught up to the request (`catchUpLocked`).
  - Live: 6 of 6 direct legs echoed X and Y exactly, with Z +23 (the GM's collision height).
  - The desync signal stands: a destination X, Y other than the click, on a segment the bot's own
    `CanMove` passes.

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
  up, ground Z from `Height(next, prevZ + 2*CellHeight)`) together with the per-step `CanMove`.
  It becomes a pure function `geo.WalkSteps(from, to, startSpeed, speed, stepDurations)`, which
  returns where one given sequence of steps stops. The walker's exact stretch check is built on
  it (section 3).

Configuration:

- `-geodata DIR`, `-geoengine FILE` and `-geodata-cache FILE`;
- `-geoengine` is a local copy of the server's `geoengine.properties`. It supplies the region
  list, `GeoDataType`, `MaxObstacleHeight` and `PartOfCharacterHeight`, the last two used by
  `CanSee`;
- the region list goes through the server's `ListedRegions` rule: a key present counts, whatever
  its value; a commented-out line does not;
- at start the bot logs a hash of the region list, to compare with the server's;
- N1 also checks what `internal/commons` code the copied packages import, and copies that too;
- collision heights for `CanSee` come from the bot's own `UserInfo` and from NPC templates
  (datapack).

**Doors, the server's way.** The server keeps doors as an overlay inside the shared engine
(`engine_dynamic.go`):

- readers take one atomic load and a plain map read. On a block a door overlays, they also take
  that `dynamic.Block`'s own mutex (`geo/dynamic/block.go:36, 222-243`);
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
  - **entrances**: every pair of cells that crosses a cluster border, per layer. A run of
    neighbouring crossings is merged into one entrance node only when its cells are mutually
    reachable inside both clusters, so one-way terrain along a border keeps its own nodes. N3
    reports the node and edge counts;
  - precomputed directed intra-cluster edge costs between entrances;
  - the **one-way edges** between two abstract SCCs (a crossing, or a pair of entrances joined
    inside a cluster) are stored too, costed by their straight line, for the one-way routes
    below (#231); a pair joined inside a cluster has no doors, a crossing keeps its door set. On the owner's water-aware build that is 722,553 edges more
    (4,292,761 in all): the cache grew from 98.6 to 113.0 MB, built in 6 min 8 s on 2 workers
    with 901 MB of heap from the OS;
  - **strongly connected components (SCCs)** labelled on the directed abstract graph, and its
    condensation (the graph of components) stored.
- **Reachability**: which abstract SCC a cell belongs to, worked out inside its cluster.
  - **Local SCCs.** `Locate` runs Tarjan's algorithm on the directed cell graph, bounded to the
    cell's cluster (at most 256×256 cells per layer). It reads the **static base blocks only,
    with every door open** (`dynamic.Block.Base()`), exactly as `navbuild` does for the abstract
    SCCs. So cached labels never depend on the door state at the moment they were computed; doors
    are handled by the route search (below).
  - **Entrance sets.** For each local SCC it also records the entrances it can reach and the
    entrances that can reach it, inside the cluster.
  - **Membership rule.** A cell is in abstract SCC X exactly when X holds both an entrance the
    cell reaches and an entrance that reaches the cell. Such an X is unique: two would be joined
    through the cell, and would be one SCC. A local SCC with no such X is one of two kinds:
    - **one-way out**: it reaches some abstract SCC, but none reaches it back. This is ground the
      bot can drop off but never climb back to, such as a ledge a teleport or a restart point puts
      it on. The walker takes one checked drop into the nearest abstract SCC it reaches, then goes
      on as normal;
    - **isolated**: it reaches no abstract SCC at all. This is a closed pocket or pit, and the only
      `/unstuck` case. The rule is exact
    because every border crossing is an entrance or merged into one only when mutually reachable
    (above): any path leaving and re-entering the cluster passes through entrance nodes.
  - **Cache.**
    - A result is immutable. A cluster with a single local SCC (the common case: open ground)
      stores only that fact and its abstract SCC, a few bytes. A cluster with several stores a
      16-bit label per cell layer plus a small per-label table, about 130 KB for one layer.
    - Results live in a process-wide LRU **bounded by bytes** (default 64 MB, a flag), owned by
      one mutex. The mutex guards only the lookup and the insert; the computing bot runs Tarjan
      without holding it. Two bots can rarely compute the same cluster at the same time; the
      second insert is dropped.
    - Each walker also holds its own references to the results for its current cluster and its
      destination's cluster, so eviction never forces a bot to recompute the ground it stands on.
      Memory is then at most the LRU bound plus two results per bot, and most results are the
      few-byte kind.
  - **Queries.** After `Locate`, `SameSCC(a, b)` (reachable each way) is O(1). `Reachable(a, b)`
    one way is a search over the condensation, which is small.
  - **Consumers ask for `SameSCC` by default**, so a bot never walks down into a pit it cannot
    climb out of. Only follow and death return accept a one-way drop.
  - **`RouteOneWay`** is the route for them: it also takes a destination only `Reachable` from
    the start. Its abstract search runs over the SCCs on a way from the start's to the
    destination's over the condensation, found by one depth-first walk of it, and crosses from one
    to the next along the one-way edges; the legs are refined as `Route`'s are, under the same
    door handling and budget. So a far leader past a drop costs the cell search no more than one
    in the bot's SCC, where the cell search alone across the whole distance ran out of its budget
    (#231).
- **Doors in the hierarchy**:
  - `navbuild` builds every door's footprint from `doors.xml` (the copied `NewDoorObject` over the
    static geodata).
  - For each intra-cluster edge it stores two costs: the cheapest with every door open, with the
    set of doors that path crosses, and the cheapest crossing no door (infinite when there is
    none).
  - SCCs are labelled with every door open, so they are optimistic.
  - At run time an edge whose door set holds a closed door costs its door-free cost. Door state is
    read from the door table's per-door atomic array (section 3).
  - Only after the real search fails, a second search ignores doors. If that one succeeds, the
    result is `unreachable: door`, never plain unreachable. Both searches count against the same
    node budget.
- **Routes**:
  - an abstract search over the condensation-restricted graph: the edges inside the start's SCC
    (`Route`), or inside the SCCs on a way to the destination's and the one-way edges between
    them (`RouteOneWay`);
  - refinement inside each cluster the abstract path crosses, reading cells through the mapped
    geodata;
  - **middle legs inside their clusters** (#256): a leg between two entrance nodes searches the
    box of the clusters holding them, where the build found the abstract edge it stands for (a
    way inside one cluster, or one crossing between two). Only when the box does not join them
    (an edge avoided after a failed pull, a node left out in an avoided column) does it search
    anywhere;
  - **seating the ends** (#256): the abstract search joins the start to every entrance node of
    its SCC in the start's cluster by a straight line, and the goal likewise, but a node of the
    same SCC may join the endpoint only by a way out of the cluster and back. The first leg is
    therefore searched inside the start's cluster first; when it finds no way there, that search
    has flooded everything the start reaches in the cluster, and the abstract search runs again
    from the nodes it reached. The last leg does the same, walking back from the goal over the
    cluster's cardinal crossings to the nodes that reach it. Without this the leg's cell search
    flooded the world until the cell budget ran out;
  - string pulling with the copied `CanMove`;
  - splitting into segments of at most `navSegment` (about 2500 units, roughly a click on screen,
    far under 9900 in 3D);
  - **every segment passes both checks of the Goal**, the step chain through the open-corridor
    test and, on the stretches it fails, the exact check (section 3). The planner asserts this. A
    pull candidate is judged by `CanMove` and the corridor test only; a pull that fails is not
    taken, and the next-shorter pull is tried. Inside the clearance corridor a waypoint may get a small
    random offset, re-checked the same way.
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
  - each query takes its scratch buffers from a small free list and gives them back; the graphs
    are immutable. Unlike a `sync.Pool`, the free list survives a GC, so a query does not regrow
    its buffers after every collection. It keeps at most 2 idle query scratches of at most 32 MB
    each (a search near its node budget is dropped to the GC), and each hierarchy keeps at most 2
    idle cluster computation scratches.
- **Memory**: the same rule as geodata.
  - Nothing in `nav` is sized to the world's cell count: about 6×10^8 cells, so even one byte per
    cell would be 600 MB.
  - A refinement's open set and its closed set (a map) are sized by the node budget, not by the
    region.
- **Build** (`bots navbuild -geodata-cache FILE -geoengine FILE -datapack DIR`): reads the packed
  geodata cache, `doors.xml` and `zones/WaterZone.xml`, never the raw region files. It writes the cluster graph, the door
  tags, the SCC labels and the condensation to a nav cache file with the geodata cache's
  discipline:
  - a magic and version, a CRC-32C, and keyed by the geodata cache's own CRC, a hash of
    `doors.xml`, the build parameters and a hash of the water zones (see "Water");
  - written to a temporary file, synced, then renamed;
  - mapped read-only at run time, with the graph read in place.

  A stale or missing nav cache stops the walking personas at start with a clear message; it is
  never rebuilt silently inside `bots ui`. The build works region by region with at most half the
  cores, so its working set stays at a few clusters' scratch per worker.

**Built by N3** (#155). Choices the design left open:

- **Entrances are cardinal crossings.** A diagonal step across a border needs both L-shaped
  detours open, so a cardinal path always joins the same two cells: leaving diagonals out changes
  no reachability, only the ranking costs a little. For the same reason the cluster-local Tarjan
  in `Locate` reads cardinal crossings only, which keeps it free of `CanMove` calls.
- **A node is a run on one side of a border**: border cells (cells a crossing leaves from or lands
  on) of one cluster-local SCC, at most 16 positions apart, its representative the run's middle
  cell. Each node's cells are mutually reachable inside its cluster, which is what keeps the
  membership rule exact; the crossing between two runs is an edge of its own.
- **Two build phases.** First, per cluster, the entrances and which nodes reach which inside it
  (one cycle per local SCC, plus the transitive reduction between local SCCs). That is enough to
  label the abstract SCCs exactly. Second, per cluster, costs by Dijkstra, and only between nodes
  of one abstract SCC: those are the only edges a route or distance search follows, and the
  condensation carries the rest. An edge whose shortest path runs through another node's
  representative is dropped, since it is the sum of two kept edges (no distance changes). Edges
  where a door lies are kept, so their door-free costs stay per pair.
- **Ranking costs** use the cardinal steps and the diagonals whose two L-shaped detours land on the
  same cell, with the clearance extra. The build skips the diagonal's own `CanMove`.
- **Doors-open reads**: `engine.Static()` is an engine over the same mapped regions with no
  dynamic object. `navbuild` and `Locate` read only it.
- **Door footprints**: a door's fully blocked cells (`NewDoorObject`), each on the one layer the
  server's overlay lifts: the layer nearest the door's floor. An edge's door set holds the doors of
  the cells its all-open path crosses; its door-free cost is the cheapest path avoiding every
  footprint cell. Crossings between the same two entrance nodes merge into one edge: the cheapest
  crossing's cost and doors, and the cheapest door-free crossing as its door-free cost.
- **Cluster side** is a build parameter (default 256 cells) and part of the cache key; the CI
  tests run the same code on 32- and 64-cell clusters over windows of the synthetic region.
- **`Reachable`** misses one case only: a drop between two cells of one cluster that are in no
  abstract SCC (a ledge inside a pit). It answers false there, which is the safe side.
- The persona-start check on a missing or stale nav cache lands with the walker (N5, #157): N3
  ships `OpenNavCache` and its errors.

**Measured by N3** on the owner's 139 regions (`bots navbuild` with 2 workers on 4 cores; the
benchmarks are `BenchmarkOwnerNav`, local only):

| Measure | Target | Measured |
| --- | --- | --- |
| `navbuild` wall time | ≤ 15 min | 7 min 27 s |
| `navbuild` heap | ≤ 1 GB | 1044 MB from the OS under a 1 GiB soft limit; max RSS 1.08 GB, mapped geodata included |
| entrance nodes | — | 850,612 (largest SCC 332,109); 421,077 SCCs, mostly one-way slope cells |
| edges | — | 4,257,722: 3,814,436 intra-cluster after pruning (13.2 M before), 443,286 crossings; 648,246 condensation edges |
| doors | — | 547, 16,069 door refs |
| nav cache | — | 113.4 MB; `MaxNavCacheBytes` is set to 512 MiB from it |
| `Locate` cold | p99 ≤ 30 ms | p50 9.5 ms, p99 21-23 ms |
| `Locate` + `SameSCC` warm | p99 ≤ 1 ms | p99 0.02 ms |
| abstract distance warm | p99 ≤ 2 ms | p50 0.14 ms, p99 22.9 ms (#209 below) |
| 40000-unit route warm | p99 ≤ 10 ms | p50 182 ms, p99 1262 ms (#209 below) |
| 40000-unit route cold | p99 ≤ 150 ms | p50 194 ms, p99 1238 ms (#209 below) |
| heap per query | ≤ 4 MB | cold `Locate` up to 21.6 MB, route up to 86 MB (#209 below) |

**Re-measured by #209** on the water-aware nav cache (`nav-water.cache`: 804,918 entrance nodes,
largest SCC 254,168, 3,570,208 edges; `BOT_NAV_WATER` names `WaterZone.xml`), same 200 pairs
rule. #209 changes no answer: routes, distances and members are byte for byte those of before
(checked on these 200 pairs and on the synthetic region). Its changes:

- **Pull**: the corridor test runs on a per-scratch grid that caches what it reads of each cell
  across one pull's candidates, stops at its first failing cell, and runs before `CanMove` (both
  must pass, so the order changes nothing). The grid is toroidal and fixed at 256×256 cells
  (1 MB) for a segment of any length: the corridor fills in line order and each line cell's
  verdict is taken `2 × (⌊(Lmax + 3)/16⌋ + 3)` line cells later (eight at the measured `Lmax`;
  six before #307's layer rule), so no cell is evicted while it is still needed.
- **Indexes**: the cell search, the abstract search and the exact check index their nodes in an
  open-addressing table cleared in O(1), instead of Go maps.
- **Water**: boxes are indexed per 2048-unit bucket, not per region tile, and a crossing tests
  its own cell once, not once per direction.
- **Scratch**: query and cluster scratch survive a GC (Cost bounds above).

| Measure | Target | Before #209 | After #209 |
| --- | --- | --- | --- |
| `Locate` cold | p99 ≤ 30 ms | p50 15.5 ms, p99 40.2 ms | p50 12.4 ms, p99 25.1 ms |
| `Locate` + `SameSCC` warm | p99 ≤ 1 ms | p99 0.00 ms | p99 0.00 ms |
| abstract distance warm | p99 ≤ 2 ms | p50 6.7 ms, p99 413 ms | p50 0.15 ms, p99 11.9 ms (#256) |
| 40000-unit route warm | p99 ≤ 10 ms | p50 377 ms, p99 1912 ms | p50 108 ms, p99 564 ms (#256) |
| 40000-unit route cold | p99 ≤ 150 ms | p50 343 ms, p99 15.4 s | p50 133 ms, p99 723 ms (#256) |
| heap per route query, warm | ≤ 4 MB | up to 130 MB | p99 0.01 MB, max 0.5 MB |
| heap per route query, cold | ≤ 4 MB | up to 116 MB | p99 27 MB, max 40 MB: scratch growing on first touch (#256) |
| heap per cold `Locate` | — | up to 34.5 MB | p99 9.1 MB, max 30.4 MB, its cached cluster result excluded |

"Heap per query" is what one query allocates, the cluster results it adds to the LRU excluded:
those are held, bounded by the LRU's byte limit. The "before" heap column is the older bench's
largest allocation of one query, cluster results included. Warm (pages resident) includes one pass over the
distances before they are timed.

The `Locate` targets and the warm route heap target hold. The heap target ("heap per route
query ≤ 4 MB" under Cost bounds) is not met by a cold route, whose scratch grows on first touch;
the distance and route time targets need the searches to do less work, which changes answers.
#256 tracks all three (re-measured below). Over 200 warm routes the cell search expands about
60,000 cells per route, 27 times the cells on the path (the octile heuristic ignores the
clearance extra), and 62% of the segments fail the corridor test and take the exact check.

**Re-measured by #256** on the one-way nav cache (format 3, `nav-water-oneway.cache`: 804,918
entrance nodes, 4,292,761 edges), same 200 pairs rule. On that cache 32 of the 200 SameSCC routes
failed with `ErrBudget`: in each, a leg from the start or to the goal ran between an endpoint and
an entrance node of its SCC in its cluster that join only through other clusters, and the cell
search flooded 400,000 cells. Seating the ends (Routes, above) routes 26 of them; 6 still exhaust
the budget in a middle leg. This changes answers: distances and the synthetic region's results
are unchanged, but 55 of the 200 owner routes differ: 26 from `ErrBudget` to a route, 28 to
another route (a different first or last entrance node, or a tie broken another way), and 1
from a route to `ErrBudget` (its seated search spends the budget in a middle leg).

| Measure | Target | Before (format 3 cache) | After seating |
| --- | --- | --- | --- |
| `Locate` cold | p99 ≤ 30 ms | p50 10.7 ms, p99 22.1 ms | p50 11.1 ms, p99 29.3 ms |
| abstract distance warm | p99 ≤ 2 ms | p50 0.15 ms, p99 11.0 ms | p50 0.16 ms, p99 12.5 ms |
| 40000-unit route warm | p99 ≤ 10 ms | p50 106 ms, p99 565 ms | p50 112 ms, p99 850 ms |
| 40000-unit route cold | p99 ≤ 150 ms | p50 129 ms, p99 715 ms | p50 133 ms, p99 877 ms |
| heap per route query, warm | ≤ 4 MB | p99 0.01 MB | p99 0.01 MB |
| heap per route query, cold | ≤ 4 MB | p99 15.2 MB, max 27.1 MB | p99 19.4 MB, max 35.6 MB |
| routes failed (both passes) | — | 64 of 400 | 14 of 400 |

The p99 rose because the routes that failed now finish, and they are the long ones: over the
same 200 warm routes the CPU fell from 34.1 s to 32.4 s while 25 more of them route. The cell
search and the exact step-chain check now take about 45% of route CPU each. The remaining work
on the time and cold heap targets stays with #256.

**Narrowed corridor and boxed middle legs** (#256, second pass), same cache and 200 pairs, both
builds measured back to back on an idle machine. A profile of 200 warm routes put 49% of route
CPU in the exact check (151 million `CanMove` calls from 10,682 segments that took it, 12,400
per stretch, 99% of them distinct, so no cache helps) and 46% in the cell search (middle legs
85% of it). Three changes:

- **Corridor reach** (Step-chain check, below): the corridor test takes the cells next to the line
  only within 4 units of the segment, not the whole one-cell margin, so a wall or ledge one cell
  aside no longer fails it.
- **Stacked tops** (same section): for string-pull candidates a cell with several layers counts
  as its top when the chain cannot drop to a lower layer.
- **Middle legs inside their clusters** (Routes, above).

Over the 200 warm routes, segments taking the exact check fell from 10,682 to 4,505 and its
`CanMove` calls from 151 to 61 million; middle-leg cell expansions fell from 14.0 to 11.9
million, and no middle leg leaves its clusters (97 did). Answers change: distances are
unchanged; of the 200 owner routes 4 are identical, 189 are another route, 3 go from `ErrBudget`
to a route (a middle leg no longer floods past its clusters), none from a route to `ErrBudget`,
and 4 still fail with `ErrBudget`. The other routes are the same way pulled into longer segments:
17,186 waypoints became 10,333 (−40%) and the routes' total length fell by 1.0% (per route p50
−0.4%, at worst +6.0%). The synthetic region's route tests pass unchanged; two corridor cases
flip as intended (a bridge deck's stacked tops, a ledge one cell aside). One walker answer
changes: the drop off the synthetic bridge deck (`TestWalkerRecovery`) was one exact segment to
the ground beside it and is now two, to the deck's edge on its stacked tops, then down. The pull
takes the farthest candidate that passes the corridor test before it tries any exact one, and the
tops now pass the corridor test up to the edge.

**Longer segments** (#307). Most segments were short because the pull gave up early: when the
corridor test failed it tried exact candidates only within `exactCells` (8) cells, so a quarter of
all segments were exactly 128 units. Three changes:

- **Walking layer** (Step-chain check, below): the corridor test carries the floor the chain
  stands on from the segment's start, through cells of several layers, so a corridor may run on
  the ground under a deck and the stretches the exact check walks shrink to the cells where the
  layer is in doubt. String-pull candidates and the exact check's stretches use the same rule.
- **Farther exact candidates** (`pull`): from a waypoint whose corridor segment is shorter than
  `farShort` (600), or that has none, the pull tries the path cells past `exactCells`, farthest
  first. A candidate is taken when it passes `CanMove`, its stretches failing the corridor test
  total at most `farStretch` (16 cells) and the exact check passes over them. A candidate whose
  line passes within `farMiss` (two cells) of where an earlier candidate's exact check failed
  is skipped. The work is capped per route at `farWork` (800,000 corridor cells filled plus
  `CanMove` calls); once it is spent the rest of the route takes the `exactCells` rule. Without
  the 600 gate, the extra exact checks went to segments already long; without a cap, warm p50
  rose to 85 ms. The cap's sweep (warm p50, share under 300): 150,000 58.5 ms, 32.4%; 300,000
  59.3 ms, 29.1%; 500,000 62.5 ms, 26.3%; 800,000 58.5 ms, 24.1%; none 60.5 ms, 22.7% (cold p50
  92 ms).
- **Blocked-stop avoid width** (Server answers to its own moves, below).

Measured with `BenchmarkOwnerNav` (same cache and 200 pairs of 40,000 units, both builds back to
back on the same machine; main 3f62617 before, this change after; the segment lengths are those of
the 200 warm routes, logged by the benchmark since #307):

| | before | after |
| --- | --- | --- |
| route warm p50 / p99 | 61.9 / 663.7 ms | 59.1 / 392.1 ms |
| route cold p50 / p99 | 89.7 / 777.4 ms | 86.5 / 435.2 ms |
| cold heap per route p99 / max | 19.4 / 43.8 MB | 19.4 / 43.8 MB |
| segments | 11,047 | 6,853 (−38%) |
| segment length p25 / median / p75 | 128 / 181 / 1776 | 316 / 1371 / 2486 |
| segments under 300 units | 54.8% | 24.1% |
| routes failed | 8 of 400 | 8 of 400 |

Answers change: the routes are pulled into fewer, longer segments; as many routes fail as before.
The p99 fell with the walking layer's first build already (warm p99 377 ms with a 150,000 cap and
no gate): the exact check no longer walks every cell of several layers point by point.

| Measure | Target | Before | After |
| --- | --- | --- | --- |
| `Locate` cold | p99 ≤ 30 ms | p50 10.8 ms, p99 24.1 ms | p50 11.7 ms, p99 32.3 ms (code unchanged: noise) |
| abstract distance warm | p99 ≤ 2 ms | p50 0.17 ms, p99 12.7 ms | p50 0.18 ms, p99 12.4 ms |
| 40000-unit route warm | p99 ≤ 10 ms | p50 112 ms, p99 972 ms | p50 65 ms, p99 674 ms |
| 40000-unit route cold | p99 ≤ 150 ms | p50 141 ms, p99 892 ms | p50 89 ms, p99 715 ms |
| heap per route query, warm | ≤ 4 MB | p99 0.01 MB | p99 0.01 MB |
| heap per route query, cold | ≤ 4 MB | p99 19.4 MB, max 35.6 MB | p99 19.4 MB, max 43.8 MB |
| routes failed (both passes) | — | 14 of 400 | 8 of 400 |

This was the last pass against the N3 route time, cold route heap and distance targets. On
2026-11-23 the owner reset them from these numbers (#256; the new targets are under "`nav`
performance" in Testing). A route is planned once per destination, so walking efficiency (#304)
matters more than route latency at this length. The leads the reset defers are in #315:

- **The cell search is now the larger half** of route CPU. Its expansions are bounded by A*
  with a heuristic that cannot see the clearance extra. Weighted A* (heuristic × 1.5 on middle
  legs, cost at most 1.5 times the cheapest) halved the expansions (11.9 to 6.1 million) but
  added 15% `CanMove` calls in the exact check, as the paths keep less clear of walls, for
  about 15% less route time. The owner rejected it: it trades away the wall margin the
  clearance cost keeps. An admissible heuristic that sees the clearance is deferred (#315).
- **The cold heap** is the scratch growing on its first long search. The owner made the heap
  target warm-only. Its max is the first search that runs out of node budget and grows the idle
  scratch to the budget's size, at most 32 MB per idle query scratch (Cost bounds). Before, that
  was the first sampled route (an `ErrBudget`), and a later `ErrBudget` route grew it further.
  Now the first route completes, and the later one does the growing: measured route by route,
  45.2 + 30.1 MB became 29.7 + 46.8 MB. The total is the same, and p99 did not change.
- **The distance tail** needs landmarks (ALT) on the abstract graph, a nav cache format change.
  Deferred (#315).

### Water

While a player's position is inside a water zone the server moves it as a swimmer, not a walker
(acis_golang `model/actor/move/creature.go`: `waterLocked`, and the swim branch of
`resolvePathLocked`, which checks `CanMove` and then `HasLayerAtOrBelow` at the target, never
pathfinds, and measures legs in 3D). Swimming also costs breath (`SwimStateChanged`, drowning
damage). A planner without a breath (`Planner.Swim` zero, the hierarchy always) does not model
any of it: **routes keep out of water** (#161). The swimming cell planner is "Swimming" below.

- **Data**: `datapack.LoadWaterZones` reads `zones/WaterZone.xml` (373 zones in the shipped
  datapack, every one a `Cuboid` of two nodes); `datapack.Load` fills `Datapack.WaterZones`. Any
  other shape fails the load rather than leave water the planner cannot see. A zone contains a
  point when X, Y and Z all lie within its bounds, inclusive, as the server's cuboid form does.
- **Model**: `nav.Water` indexes the boxes by region tile. A cell is *wet* when any point of its
  square lies in a zone at the cell's floor height: a walker on that floor can stand anywhere in
  the cell. Water above the floor (a pool's band under a bridge deck, say) leaves the floor dry.
- **Planner**: no crossing (`cross`, so the cell search, the build and the cluster results alike)
  enters or leaves a wet cell; the open-corridor test fails a wet cell, and the exact step-chain
  check fails at any step point in water, so no segment passes water; `NearestWalkable` and
  `StandPoint` never answer a wet cell; an endpoint on a wet cell is `ErrWater`.
- **Hierarchy**: `BuildConfig.Water` keeps wet cells out of the abstract graph, the nav cache key
  carries `Water.Hash` (`NavKey.WaterHash`, nav cache format version 2), and `NewHierarchy`
  takes the same `Water` for its cluster results. A walker builds its `Water` from the datapack
  the way `bots navbuild` does.
- **Getting out**: a bot that ends up in water (a fall, a knockback, a fear) gets `ErrWater` from
  the planner; the walker steps it to the nearest dry cell (#157).

**Measured for #161** on the owner's 139 regions and the shipped datapack, with the cell planner.
The sample was every third farming spot, routed from the nearest STANDARD teleport destination
within 8000 units:

| Measure | Result |
| --- | --- |
| spot centres on a wet cell | 37 of the sample's 1555 (120 of all 4664, #275) |
| wet STANDARD teleport destinations | 0 of 429 |
| routes planned without water that pass water | 23 of 488 (4.7%) |
| of those, with water avoided | 9 detour (most under 1.15× the length), 4 have no dry cell within 300 of an endpoint, 4 no route, 6 exceed a 400000-cell budget |

The losses are spots reachable only through water, such as Schuttgart's region 23_11 and part
of region 21_24, where the sea cuboid's band (maxZ -3780) covers low ground. "Swimming" plans
them (#223).

### Swimming

The server's rules for a player in water (acis_golang `model/actor/move/creature.go`,
`network/movement.go`, `network/taskeffects.go`):

- A leg's move type is the zone flag at its origin: a player inside a water zone swims
  (`flagTypeLocked`, set by `SetSwimming` on zone enter and exit). A swimming leg is
  `CanMove(origin, target)` and then `HasLayerAtOrBelow(target)` (`resolvePathLocked`); it is
  never routed around anything, and a leg that fails ends at `ValidFlyLocation`, a desync for
  the walker.
- A swimmer moves straight in 3D toward the destination's height, measures its legs and steps in
  3D, is not capped at the surface (`waterLocked`: only a flier or a boat is), and no closed
  geodata line stops it under way (`catchUpLocked` blocks a ground move only). A walker that
  wades in mid-leg swims on; a swimmer that leaves the water mid-leg walks on by the ground
  rules (`stepLocked`).
- In water a player moves at its swim speed (`Character.MoveSpeed`): UserInfo's base swim speed
  times the same multiplier as the run speed, run or walk alike. Every shipped class has base 50,
  against 115 to 130 run.
- Breath is `breathTime`: 60 s times the race's `BreathMultiplier` (0.8 dwarf to 1.5 elf),
  changed by the Breath stat; past it the player drowns. With `AllowWater` off there is no
  countdown and no drowning, but the bot cannot tell.
- The server sends a player `ChangeMoveType` only when it toggles run and walk, so the bot takes
  its swim state from its own position in the water zones, as the server's zone flag does.

The planner with a breath (`Planner.Swim`, the longest stretch of water a route may swim, in 3D
world units):

- **Search**: wet cells are graph nodes; a swimmer's leg must pass the same `CanMove`, so the
  floor graph is its graph too. A step into a wet cell costs `swimCost` (3) times as much, so a
  route swims only where the dry way round is over three times as long. A wet endpoint is
  allowed.
- **Pull**: from a waypoint in water the pull takes the farthest point the straight 3D line
  reaches without leaving water (`swimLeg`: `Water.exit`, then `CanMove` and
  `HasLayerAtOrBelow`), up to `navSegment`. Water zones are boxes, so a line between two points
  of one box stays in it. Wading in and climbing out go through the exact check.
- **Exact check**: a step-chain state in water is a swimmer (`swimStates`): nothing on its
  straight line to the segment's end is checked while it stays in water; where the line leaves
  water every band point a step past the exit becomes a state at the line's height, and the
  chain goes on by the ground rules. The open-corridor test still fails a wet cell.
- **Breath**: the in-water part of each segment (`Water.Span`, in 3D) adds up until a segment
  ends out of water; a stretch longer than `Swim` is `ErrBreath`. The search weighs water but
  cannot count breath, so `Route` then plans again keeping out of water.
- **Hierarchy**: unchanged. The abstract graph and the nav cache hold no wet cell, and
  `Hierarchy.Route` refines with `Swim` zero. A walk the hierarchy cannot route (no member under
  the bot, a destination outside its SCC or in water, `ErrWater`, `ErrUnreachable` or `ErrBudget`) with water
  within `walkSwimNear` (1000) of the box its two ends span tries the swimming cell planner,
  `walkBudget` cells. Only a follower swims to a destination in water: a bot left standing in
  water drowns once its breath runs out, so any other walk to a place in water stays
  unreachable, as before. A bot in water always swims out.
- **Spots**: no farmer can hold a spot whose centre snaps to a wet cell (`Planner.WetAt`), so with
  the navigation loaded the spot registry leaves those out at start (`Nav.DrySpots`, #275) and logs
  how many (`wet_dropped`): 120 of 4664 on the owner's data, 113 of them region 21_24's sea floor,
  6 in Innadril, 1 in Aden. Without the navigation the registry keeps them. Farming such a spot
  from a dry shore is not done.
- **Walker**: `Planner.Swim` is the swim speed times `swimBreath`, 24 s: half the dwarf's 48 s,
  since the bot sees neither its Breath stat nor, with `AllowWater` off, a countdown. The bot's
  own dead reckoning swims the part of a move in water (`Water.Span`) at the swim speed in 3D,
  its height on the swim line rather than the floor, and the arrival model (below, "Walker") times
  the part in water at the swim speed, with one more update per change between land and water.

**Measured for #223** with `TestOwnerSwimSample` (`internal/nav/swim_owner_test.go`, local only:
`BOT_NAV_GEODATA`, `BOT_NAV_GEOENGINE`, `BOT_NAV_GEODATA_CACHE`, `BOT_NAV_DATAPACK`) on the owner's
139 regions and the shipped datapack: #161's sample rebuilt (every third farming spot, from the
nearest STANDARD teleport destination within 8000, cell planner, 400000 cells, breath 1200 = base
swim speed 50 × 24 s). The rebuilt sample plans 558 routes when water is ignored and 14 of them
pass water (the #161 count of 23 of 488 came from a sampling script that was never committed;
this one tests each segment's straight line against the zones):

| Of the 14 | Keeping out of water | Swimming |
| --- | --- | --- |
| route | 3 (one a 21,468 detour for a 7,522 way) | 8 |
| endpoint in water | 9 | 0 |
| longer swim than the breath | - | 5 |
| over the cell budget | 2 | 1 |

The swims take 11 to 79 segments and 25 to 244 ms. Schuttgart's `schttgart08_2311_05m1` (region
23_11), over budget dry, routes in 5,345 with a swim of about 600; region 21_24's spots that lie
on the sea floor near (41000..51000, 199000..207000) route where the swim is short, and the five
deepest (`t21_24_00302` to `t21_24_00802`, floors -4093 to -4580) need a longer swim than 24 s of
breath. `oren13_1920_052sm1` is over budget either way; water is not why. The walker takes the
planner's routes to the 9 spots whose centre is in water only to follow a leader there; the spot
registry drops those spots (#275).

### 3. `botclient`: door table and walker

**Door table (process-wide).**

- Writes are serialized by one mutex. Reads go to a per-door atomic state array, so the route
  search never takes the mutex.
- Each bot's brain reports a door only when the state it sees differs from what that bot saw last
  for the door (door id from `DoorInfo` and `DoorStatusUpdate`). The table then sets the state
  and toggles the door in the shared engine through the engine's writer path.
- **Best effort.** Sessions lag differently, so a slow bot can report an older state after a fast
  bot reported a newer one. There is no server order to recover, and the per-bot dedupe only makes
  this rarer. A door left in the wrong state shows up as a door replan, which never counts toward
  unreachable.
- A door no bot has seen keeps its `doors.xml` default.

**Walker (one per bot, owned by the brain goroutine).** It replaces `legToward`, `spotLeg`,
`followLeg`, `walkTo`, `unstick` and the detours. Each consumer asks
`goTo(dest, arrive, purpose)` and gets back an `Action` plus a status: walking, arrived, or
unreachable (with a reason).

- **Position**: the server is the authority (`MoveToLocation` origin, `ValidateLocation`,
  `StopMove`, `TeleportToLocation`). Between those, the bot simulates its own walk with the copied
  step chain at the nominal 100 ms steps. That replaces the linear Z interpolation in `world.go`.
  Arrival is inferred from the simulation, because the server sends nothing on arrival.
- **Arrival model** (#305): the end of the bot's own move is timed like the server's arrival
  timer (`creature.go` `travelTicksLocked`, `arrivalDelayLocked`), in whole 100 ms updates. The
  first 5 updates of a move from standstill advance at the walk speed on land, even running
  (`moveStartSpeed`; in water the start speed is the full speed). Each change between land and
  water along the line adds one update, where the server re-times the rest. The anchor (the
  receipt of the answer that started the move from standstill) and so the update count survive a
  resample, a `MoveToPawn` and a retarget; a resample re-times the distance left from now, as
  `retimeLocked` does, never earlier unless the speed changed. A move received within one tick
  plus the walker's answer latency (the maximum over its last 8 segments, at least one tick) of
  the modeled end counts as a fresh start. The distance timed is the larger of the server's
  origin and the bot's estimate to the destination. The fields sit behind the motion's one
  Self-only pointer, so `Object` stays compact. `Moving`, the walk wake, `segmentDone` and the
  watchdog all read this end (`docs/walking.md`, "One arrival model").
- **Step-chain check**: it holds whatever sequence of steps the server takes. It comes in two
  parts: a cheap test that covers almost every segment, and an exact check only where that test
  fails.
  - **Facts it rests on.**
    - Every step, the first included, is at most `Lmax` = run speed × `Emax` (`Emax` from N0); the
      100 ms run-out steps fall inside that. The server sets the move's `lastUpdate` when the
      request arrives (`model/actor/move/creature.go:890-891`) and times the first position update
      from it (`:1360-1362`, `playerPassed`), so the first step spans at most the time to the next
      tick of the global ticker (#307; this section took it as any length before).
    - Each step re-aims from its rounded point toward the destination (`creature.go:1374-1376`),
      so step points stay within about one unit of the segment's line.
  - **Open-corridor test (cheap, sufficient).** Take every cell the segment's line crosses, plus
    each cell next to one of those that lies within 4 units (`corridorReach`) of the segment
    (#256; before, the whole one-cell margin). Every cell the chain can touch is in it: the rounded
    step points lie within 1.5 units of the line (`bandRadius`, below), and `CanMove`'s border walk
    between two of them strays from their own line by at most about 1.5 more (it picks each
    crossing from the line's Y at the cell's last column, truncated to an integer). The segment
    passes when, in all of those cells:
    1. the cell has geodata and a **walking layer** L, the floor the chain stands on there, carried
       from the segment's start floor (#307; #256 took a cell of several layers on its top only).
       A cell of one layer has that layer. A cell of several gets, as the segment's start cell,
       the layer nearest the start's Z (`CanMove`'s origin floor), and elsewhere the layer
       `CanMove` crosses onto from a neighbour N already on its own, `belowCell(C, L_N + 48)`: a
       neighbour the quadrant crosses from first, then any other 4-neighbour, then a diagonal one.
       A cell no neighbour reaches fails, and so does the end's cell when its walking layer is
       not the end's floor. The cells fill in line order, each line cell before the ring around
       it, so the layer spreads from the start's cell. The floor must be dry;
    2. the NSWE flags allow crossing in both directions of the segment's quadrant. No step turns
       back on either axis: a step adds (destination − rounded origin) × a fraction below 1 to a
       float position that rounds half up to the origin, so it never passes the destination's
       integer coordinate (`stepLocked`; N2 found the reversal earlier assumed here does not
       occur, and `TestStepChainAgreesWithWalkSteps` checks it against `WalkSteps`);
    3. no two neighbouring corridor cells differ in walking layer by 48 or more: `belowCell(z + 48)`
       takes a layer strictly below `z + 48`, so a step up of exactly 48 is blocked (#256 fixed
       the earlier "more than 48"). And in each direction the quadrant crosses from N into C,
       `belowCell(C, L_N + 48) = L_C`: no other layer of C lies in `(L_C, L_N + 48)`. A cell two
       neighbours would put on two walking layers fails here;
    4. **layer rule**: for each corridor cell C of several layers and each corridor cell O within
       one step of it (Chebyshev distance `⌊(Lmax + 3)/16⌋ + 1` cells: a step of at most
       `Lmax + 2 × 1.5` between rounded points), `Height(C, L_O + 16) = L_C`, the nearest layer
       to `L_O + 16`, a tie going to the lower one (`block.Multilayer.Nearest`). O = C counts: a
       short update rounds back onto its own point and re-floors there. The rule is what keeps a
       step that crosses a low cell of a stair from landing on the layer under the deck, and
       what fails the ground under a deck less than 32 above it.

    Under those conditions every per-step `CanMove` between two points in the corridor holds,
    wherever the rounding lands: on a corridor of one-layer cells whatever the step lengths, and
    with cells of several layers for steps of at most `Lmax + 3`, the layer rule's bound. By
    induction along the chain, a state in cell O stands on L_O:
    - `CanMove` starts on `HeightNearest(O, L_O) = L_O`;
    - each border crossing goes from a corridor cell N on L_N into a corridor cell C in a direction
      of the quadrant, which N's NSWE allows (condition 2), and lands on
      `belowCell(C, L_N + 48) = L_C` (condition 3), so the walk stays on walking layers;
    - the step's target Z is `Height(T, L_O + 16) = L_T` (condition 4, trivially on one layer),
      and the final `goz == Height(target)` compares L_T with itself, so the next state stands
      on L_T.

    The test costs O(cells along the segment): no `CanMove` calls, a few microseconds.
    String-pull candidates take this test first; farther candidates add the exact check on the
    stretches that fail it (#307, "Longer segments" above). `TestNarrowCorridorAgreesWithExact`
    checks every segment the narrowed test passes and the old one failed against the exact check
    over its whole length and against random step sequences of the copied step chain, around the
    bridge, the stair and stacked floors 30, 40, 60 and 100 apart. `TestCorridorReachBothSigns`
    checks `corridorReach` against `CanMove`'s traced border walk at every sign of the world
    coordinates: the walk takes each crossing's row with `int()`, which truncates toward zero, so
    down for positive coordinates and up for negative ones (server `engine_query.go:46`).
  - **Exact check (short stretches only).** Where the corridor test fails (a doorway with partial
    NSWE, a bridge end, a stair with mixed layers), the stretch from the last corridor cell before
    the failure to the first after it is checked exactly:
    - It is a dynamic program over states keyed by rounded `(X, Y, Z)`.
    - It starts from every rounded point within one unit of the line and within `Lmax` before the
      stretch, on the walking layer the corridor gave its cell (#307; before, `Height` at the
      segment start's Z). A stretch at the segment's start starts from the start alone: the first
      step is bounded like the others. Every state it reaches past the stretch must stand on the
      corridor's walking layer there, where the corridor test takes over.
    - The server carries a float position inside each rounded point. So from each state, every
      rounded point within `Lmax + 1.5` ahead, within 1.5 units of the line, and between the state
      and the destination on each axis must pass `CanMove`, with Z from
      `Height(next, prevZ + 2*CellHeight)`. The float position stays within √2/2 of the line and
      rounding adds √2/2 more, so 1.5 bounds both (N2, `internal/nav/exact.go`).
    - The stretch passes when every reachable state does, up to the far side.
    - It runs on the segments the pull takes past the corridor test: within `exactCells` (8) cells
      of a waypoint, and farther ones while the route's budget lasts (#307, "Longer segments"
      above). A stretch is a few cells long, so it costs a few thousand short `CanMove` calls; N1
      benchmarks one call, and N3 sets the route thresholds from that measurement.
    - A segment whose exact check fails is split at the stretch, and the planner tries another
      way past it.
  - **What it does not cover**: a server tick that stalls past `Emax` (a GC pause, for example)
    inside a stretch that needed the exact check, or over a corridor cell of several layers, where
    a longer step can land on another layer. The ticker runs missed updates back to back
    (`commons/scheduler/ticker.go`), so a stall shows as one long step, and a move requested
    during one as a long first step: the same exposure. Corridors of one-layer cells stay immune
    to it. The exposure is per exact stretch and per cell of several layers walked, so it depends
    on the terrain crossed, not on segment length. `nav_blocked_total` measures what is left.
- **Sending**:
  - Every segment goes out from standstill, from the server-confirmed position, except a
    click-ahead at a checked corner (below).
  - A pawn walk (an approach to attack or cast) ends on the server's first 100 ms step in range,
    with no packet, up to a step short of the dead-reckoned end. The server stops the bot before it
    swings or casts and the packet carries its position then, so the world model adopts it
    exactly: every `Attack` position, and the position in the bot's own `MagicSkillUse` for a cast
    it asked for with a hit time of 500 ms or more (shorter casts may go out while walking:
    `stopForCast` stops only above a 50 ms template hit time, which cast speed can scale up).
  - **Wake on arrival** (#305). The walker sends the next segment one tick after the modeled end
    of the bot's own move. The walk wake brings the decision there instead of the next regular
    tick: `decisionAt` offers the end plus one tick while the walk is walking with a segment in
    flight or the last decision called `goTo`, and only while that time is later than the last
    walk wake. The wake is derived from the bot's motion, fires once per end and re-arms when the
    end moves later (a resample, a speed change, the next answer). It covers a normal answer, a
    reset mid-segment, a desync or door replan (once the server's own move ends) and a blocked
    stop (one tick later). The measured idle per waypoint is in "Walking efficiency".
  - **Click-ahead at checked corners** (#308; owner, 2026-11-25). A segment that is not the
    route's last sends the next one before its own move ends, so the server turns the walk at the
    corner instead of stopping there. A click while moving retargets the server's walk: a catch-up
    step along the old destination, then a fresh routed click from there, with the start-speed
    update count kept (see "Server behavior this rests on"). So the click's origin is wherever the
    server is when it takes the click, and every line from there must be one the walker checked
    (`walker_ahead.go`):
    - **When.** `aheadLead`, the answer latency A plus one position tick, before the modeled end of
      the segment in flight, once the server answered it. The modeled end errs late by the
      downlink, so the click reaches the server about a tick before the server's own arrival. The
      walk wake fires there first, then at the end plus a tick as before.
    - **The fan.** The server takes the click on the last stretch of the segment: the last
      speed × (`aheadLead` + 2 × `navEmax`) of the server's line from the segment's answer's origin
      to its end, the whole line when shorter, at the larger of run and walk speed. One `navEmax`
      covers the arrival model's error; the other a catch-up step the server refuses, which leaves
      the walk at its last update point (`catchUpLocked`). A click that comes after the arrival
      starts from the segment's end, which is in the fan.
    - **The check** (`nav.Planner.FanOpen`). Every line from a point within `FanBand` (2 units, the
      server's rounded step points stray 1.5 from the line) of that stretch to the next waypoint
      must hold. The planner checks the lines from the stretch's ends and from points at most half
      a cell apart between them, each from the layer nearest the stretch's height there:
      - `CanMove` to the waypoint at head height, as the server checks the click (the target's Z
        raised by the collision height);
      - the open-corridor test with the NSWE of the union of the quadrants of every line of the fan
        (the band's corners at both ends bound them), and the reach of the cells next to the line
        widened by √(4² + 2²), how far a line between two checked ones, from a point up to
        `FanBand` off the stretch, strays from the nearer one;
      - no avoided column (`Planner.Avoid`) on the line.

      The exact check vouches for one line only, not for the lines between two checked ones, so a
      fan needs the corridor test throughout. A segment whose fan fails is marked, and the next
      segment goes out from standstill as before.
    - **Not ahead**: into or out of water (the bot's move or the next segment), across a door cell
      (either segment: a door may toggle under the click), during the bot's own swing or cast,
      sitting, once the bot's own move has ended (the standstill send takes over), and after a
      segment whose own answer missed its fan.
    - **Origin.** `standing()` and the `!Moving` gate of `walkStep` belong to the standstill send:
      a click-ahead's origin is its fan, never a mid-move estimate snapped to a layer. The packet's
      origin field carries the dead-reckoned position; the server only bounds the click's 9900
      distance by it.
    - **Pacing**: the segment's one `MoveBackwardToLocation`, sent earlier through the frame pacer
      (`gateNone`); click-ahead adds no packet.
  - No move goes out while the bot's own swing or cast is in progress, because the server would
    defer it and answer `ActionFailed`.
- **Server answers to its own moves**:
  - A `MoveToLocation` for the bot whose origin equals its destination is a **blocked stop**. The
    step chain differed from the bot's check. So is a `StopMove` short of the stand point during an
    approach to talk. The walker counts `nav_blocked_total`, replans from the stop point, and
    avoids the refused part of the segment for this route: every plan of the walk keeps out of
    its geodata columns (`nav.Avoid`, per planner; the shared nav cache never sees it). The server
    stops before the step that failed (`creature.go:1384-1390`), so that step starts at the stop
    and spans at most `Lmax`. The refused part (#307, `Planner.BlockedEnd`) runs from the stop
    one step and a cell on, then to the end of the next stretch beyond that the corridor test
    fails, as far as the exact check walked it: a blocked stop means the bot's data differ from
    the server's (a stale door, a GM fence), so an open stretch can block too, but the next one
    the check only vouched for exactly is the likelier cause. A stop inside such a stretch refuses
    the whole rest of the segment, as before #307. The route's own start and goal columns stay
    allowed; a new destination clears the set. When the set cuts every way, the walk ends
    unreachable as blocked.
  - A `MoveToLocation` answering the bot's click with another destination is a **desync**. The
    server routed or truncated, so the bot's geodata or door state differs from the server's. The
    walker counts `nav_desync_total`, logs the segment (from, to) with the server's origin and
    destination at warn level (`nav desync`), so a run names the leg its check got wrong, and
    replans from the server's origin. If the segment crosses a door cell, it counts
    `nav_door_replan_total` instead, which does not count toward unreachable.
  - **Answers around a click-ahead.** The connection keeps the server's order, so what the server
    sent about the previous segment before it took the click arrives before the click's answer,
    and that answer always moves (a direct click toward the next waypoint):
    - A blocked correction while the click is unanswered is the previous segment's. At that
      segment's end it is its arrival: a catch-up into a closed line keeps `routeBlocked` set
      through the retarget (`moveToLocationLocked` clears it only for a fresh move), and the walk
      then ends as blocked where it arrives (`arrivalHookLocked`). Short of the end it is a
      blocked stop of the previous segment, counted and avoided along that segment's line, never
      the click's; the click then starts from the stop point, off the fan.
    - A `StopMove` while the click is unanswered is likewise judged against the previous
      segment's end.
    - The click's answer has its origin checked against the fan. Off the fan the server walks a
      line the walker did not check: the walker stops following the route, keeps to the server's
      move and replans from where it ends. It sends no stop: that would be another packet and a
      fresh walk-speed start, and the server checked `CanMove` on the line itself (it routes
      otherwise, a desync), so what is left is a blocked stop, handled as any other. The segment
      starts at the server's origin from then on, so such a stop builds its avoided line from
      there.
    - A blocked correction at the end of an answered click-ahead (the sticky block again) is its
      arrival.
  - A segment starts where the server holds the bot. A talk click in reach answers with a
    `MoveToPawn` from the player's current location (`targeting.go` `thinkInteract`), and the bot
    stands exactly there, not on its own estimate of the approach's end: a cell off on a long
    segment, the bot's line and the server's cross other cells (#434).
  - After repeated blocked stops or desyncs on one route, the walker ends with status
    `unreachable: blocked` or `unreachable: desync` and shows it in the status snapshot, instead
    of looping.
  - `ActionFailed` is not a navigation signal: the server sends it for many other reasons,
    successful pick-up and talk included.
- **Progress watchdog**: from the arrival model the walker knows when each segment should arrive
  (injected clock). A segment unanswered after 2 s, or still on its way past its end plus half its
  run plus 2 s, counts as failed. This replaces "100 units closer per leg".
- **Recovery**: replan from where the server says the bot is.
  - A bot standing on one-way-out ground first drops, with one checked route, into the nearest
    abstract SCC it reaches.
  - `/unstuck` is kept for isolated ground only, which reaches no abstract SCC (a closed pit, the
    restart area of acis_golang#5412).
    The server's recall is a five-minute cast; the walker holds every walk while it runs.
    A refused or cut-short recall is asked again `escapeRetry` (30 s) after the ask or the cast's
    end, at most three times in ten minutes; a recall that lands stops the asks for ten minutes
    from the landing (the landing may be isolated too); the server's petition refusal (system
    message 1043) stops the asks until a teleport (#286).
    The breaker (`escapeBreaker`, `escape.go`) is shared with rule "leader"'s `/unstuck` for a lost
    leader, so the two together send at most three in ten minutes, and a recall either asked
    blocks both. The petition refusal stays with the rule whose ask it answers: a refusal of the
    leader rule's ask in a boss zone does not stop the walker once the bot is out of it (#335).
  - Sidesteps and wall detours are deleted.
- **Pacing**:
  - one `MoveBackwardToLocation` per segment, through the frame pacer (`gateNone`);
  - a segment's next click waits for the server's answer plus the move's end and one tick (100
    ms), so at most 10 walker moves per second (16-unit segments) and about one per second
    typically, far below the server's 80 frames/s average;
  - `Action` (melee, cast, talk, pick-up) keeps its existing gates.

### 4. Consumers

| Consumer | Today | Redesigned |
| --- | --- | --- |
| Spot pick and walk (`spots.go`) | straight-line distance, hops, stall counter, one-hour avoid | candidates in the bot's SCC only, ranked by abstract distance; spot centre snapped inside its territory and SCC; `walker.goTo` |
| Trips (`travel.go`) | gatekeeper when nothing is in walking range | Dijkstra over {here, gatekeepers, their destinations, target} on abstract distances: walk edges cost distance over speed, teleport edges cost adena; only legs inside one SCC |
| Follow (`follow.go`) | slot point as drawn, legs, sidesteps, detours | slot snapped into the leader's SCC, one-way drops allowed toward the leader; route; re-plan on the same `Repath` and delay rules; out of sight, route to the last `PartyMemberPosition` |
| NPC visits and errands (`visit.go`, `town.go`, `shop.go`, `trainer.go`, `newbie.go`) | `Action` on the NPC, with no line check | walk to `StandPoint(npc, talkRange, line)`, then `Action`: the tracking walk's straight line is open |
| Melee and casting | `Action` or attack, with no line check | send the attack or cast only while the straight line to the target is open and within range, else walk to a stand point first; a moving monster is checked again at each decision, and a blocked stop moves the bot to a new stand point |
| Pick-up (`loot.go`) | `Action`; the server routes the walk | send it only when `CanMove(self, item at raw Z)` holds and the step chain reaches it, and, out of the server's pick-up range, the item's cell is in the bot's SCC (a straight walk down a ledge must not leave the bot on isolated ground, #278); else walk to `StandPoint(item, r, pickup)` in the bot's SCC first |
| Bows and spells at range | server chase | `StandPoint` with `CanSee` and the skill's or weapon's range |
| Flee (`rules.go`), idle drift, death return | straight point | a point in the bot's SCC (death return may take one-way drops), chosen away from the attacker or inside the drift radius, as checked segments |

### 5. What is deleted

`unstick.go` (except the `/unstuck` for isolated ground); the detour and stall fields of `followState`;
`spotWaypoint`, `legToward`, `spotStalls`, `spotProgress`, `spotUnreachable`; `walkStuck` and the
errand back-off it drives; and the tests that pin those heuristics. Each consumer's PR deletes its
own heuristics, and nothing is kept alongside the new code. N6 (#158) and N7 (#159) moved the spot
rule and follow off `legToward`, `spotLeg`, `spotStalls` and `spotProgress`, which are gone.

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
  - N0 adds `bots geoquery -geodata <dir> -geoengine <geoengine.properties> -geodata-cache
    <cache> -out <file> [-seed 1] [-n 1000]`. It writes a curated query file in `geoprobe`'s dump
    format with the bot's answers (`internal/geo/geoquery`). The sites are sampled from the listed
    regions, `-n` of each kind:
    - walls: `canmove` both ways across a closed NSWE side and along it, and `los` across;
    - ledges: a neighbour at least 32 higher or lower; `canmove` and `validlocation` both ways, and
      the height on both sides;
    - multilayer cells: the heights described in "Settled by N0", plus `canmove` and
      `validlocation` from each layer;
    - random short pairs within 8 cells: `canmove`, `los` and `validlocation`.

    A query that reaches an unlisted region is dropped, because `geoprobe` loads every region file
    while the bot answers an unlisted region as open ground. The path category is not used. N0's
    run (seed 1, `-n 2000`) wrote 78,432 queries, and `geoprobe` reported `agreement=100.00%`.
  - `geoprobe -expected-dump <file> -config <geoengine.properties> -geodata
    <datapack>/data/geodata` runs from the server checkout without editing it, and must report
    full agreement. This is the server's own tool: it decodes every region onto its heap (several
    GB), on the owner's machine and only for this check, never in the bot's runtime.
  - Doors and the step chain are not covered by `geoprobe`. A live e2e scenario covers both: it
    walks segments along walls, ledges and stairs, and compares every blocked stop with what
    `WalkSteps` predicts for the tick timings seen. A server issue asks `geoprobe` for a walk
    category, which would add an offline oracle (fatal10110/acis_golang#5435).
- **`nav`** (CI, synthetic regions):
  - every returned path passes both checks of the Goal, segment by segment;
  - the open-corridor test accepts open ground and the ground under a deck at least 60 above,
    and rejects corridors whose walking layer a crossing or a step leaves (the ground under a
    40 deck, a stair's mixed layers), partial NSWE or a climb over 48;
  - the exact check rejects a stretch that only some step sequences get through (a synthetic
    ledge edge, a step longer than `Lmax`), and agrees with `WalkSteps` run over random step
    sequences, and over a first step of every length up to `Lmax`
    (`TestFirstStepAgreesWithWalkSteps`);
  - entrances: a border with one-way terrain keeps separate nodes, and no run is merged across
    cells that are not mutually reachable;
  - cluster labels computed while a door is closed equal those computed with it open;
  - the cluster membership rule matches a whole-region SCC computed by brute force on every
    synthetic region, one-way-out ledges and isolated pockets included;
  - a closed door on the cheapest entrance-to-entrance path falls back to the door-free cost, not
    `unreachable: door`;
  - on door-free regions, `Route` succeeds exactly when `SameSCC` holds;
  - the pit is never entered by a default (`SameSCC`) route, and the ledge is taken down only by
    the one-way modes;
  - closed doors give `unreachable: door`;
  - budget exhaustion is reported;
  - a property test over random pairs checks the same invariants.
- **`nav` performance**: benchmarks with recorded thresholds, measured on the owner's geodata
  locally and reported in the N3 PR.
  - The route, distance and heap targets below were reset by the owner on 2026-11-23 (#256)
    from the #314 measurements, about 10% above them. They are now regression ceilings: a change
    that pushes a measure past its ceiling needs an owner decision. The first targets were
    abstract distance p99 ≤ 2 ms, a warm route p99 ≤ 10 ms, a cold route p99 ≤ 150 ms and
    ≤ 4 MB heap per route query, cold included. The leads deferred by the reset are in #315.
  - **Warm** (cluster LRU warm, pages resident):
    - `Locate` + `SameSCC` p99 ≤ 1 ms;
    - abstract distance p99 ≤ 15 ms;
    - a full 40000-unit route p50 ≤ 75 ms and p99 ≤ 750 ms, corridor tests and exact checks
      included.
  - **Cold** (first touch of the clusters and pages): `Locate` p99 ≤ 30 ms, and a full route
    p50 ≤ 100 ms and p99 ≤ 800 ms. A cold route happens at most once per destination per bot.
  - Heap per warm route query ≤ 4 MB. A cold route's heap has no target: its query scratch grows
    once on the first long search, bounded by the node budget and the free list's 32 MB per
    idle scratch (Cost bounds).
  - `navbuild` ≤ 15 minutes with half the cores, peak heap ≤ 1 GB. N3 reports the measured
    entrance and edge counts, then sets the nav cache's size bound from them.
- **Door table** (CI, `-race`): concurrent reports from many bots are serialized, every toggle
  reaches the engine, the per-bot dedupe drops repeats, and route-search reads never block on the
  table mutex. A misordered state is not tested as prevented (it is best effort, section 3); the
  walker test checks that it costs only a door replan.
- **`walker`** (events and an injected clock in, packets out):
  - segments in order, each from standstill;
  - a same-point `MoveToLocation` replans as blocked, and so does a `StopMove` short of a talk
    stand point;
  - another destination counts a desync, or a door replan on a door segment;
  - no move during the bot's own swing or cast;
  - the watchdog;
  - a bot on one-way-out ground drops into the nearest SCC, and only one on isolated ground uses
    `/unstuck`.
- **Consumers**: today's rule tests rewritten against a nav built from the synthetic regions, so
  they no longer pin heuristics.
- **e2e** against the live server (`docs/e2e-live.md`), with the owner's geodata:
  - scenarios: a long walk across a region, the dark elven trip (#144), following over ledges
    (#147, #148), the grocer visit (#122), melee and pick-up around obstacles;
  - each asserts `nav_desync_total` = 0 on door-free routes, and `nav_blocked_total` ≤ 1 per 100
    segments;
  - the step-chain scenario above;
  - a door scenario asserts door replans and no plain unreachable.
  - Delivered by N8 (#160): every scenario that runs a whole bot through `runWatched`
    (`internal/e2e`) asserts the two thresholds, with `nav_segments_total` (the walker's segments
    and talk clicks) as the base of the blocked rate, and checks every blocked stop it saw: one
    `WalkSteps` predicts from the stop for the step seen is explained, so is one after a step
    longer than `Emax` on a segment that fails the open-corridor test, any other fails. The long
    walk is `TestLiveSpotSpread` (the trip of #144), the ledges `TestLiveFollowLedge` and
    `TestLiveFollowAcrossBand`, the grocer `TestLiveTownErrands`; `TestLiveNavStepChain` walks
    along Gludin's walls, `TestLiveNavStair` (#243) up and down a stair flight in the Dark Elf
    village, every cell of it multilayer under the cavern roof, `TestLiveNavMeleeObstacle` walks round an
    obstacle before its first click on the monster and asserts the click's origin has a straight
    walk to it, `TestLiveNavPickUpObstacle` (#239) kills wolves at a measured site until the bot turns
    to a drop where the straight walk from where it stands fails, and asserts that the bot walks to a
    `StandPickUp` stand point before its pick-up and that the item reaches its inventory, and
    `TestLiveNavDoor` toggles the doors of a Gludin clan hall (the GM's `//open`
    and `//close`, only on doors a scenario claims). A follower that sees a door change replans
    before it clicks, so door replans by the server's answer are rare: the door scenario asserts
    that a walk shut in by the closed doors ends unreachable for the door, that every walk ending
    unreachable ends for the door, and that the bot walks through once the door opens.

### Walking efficiency

`TestLiveWalkEfficiency` (`internal/e2e/walk_efficiency_live_test.go`, #304) measures the walker on
two fixed multi-corner routes in and around Talking Island village. A fresh human fighter (run speed
126) per route is recalled to the route's start and follows the GM, who stands still at its end, so
each run is one walk to a fixed point. The follow hold comes before the first segment and is not
measured.

- **town**: from the armory round the village houses to the master (`tiArmory` to `tiMaster`).
- **field**: across the open ground north-east of the village (`tiSpots` to `openGround`).

Each route logs one `walk-efficiency` line. Its fields:

- **length**: the summed ground length of the server's answers to the segments.
- **straight**: the ground distance from the first answer's origin to the last answer's end.
- **elapsed**: from the first segment's answer to the last segment's end at run speed.
- **route run / straight run**: length and straight distance at run speed.
- **efficiency**: length / (elapsed × run speed). **bot** is the bot's own
  `nav_walk_efficiency_permille`, timed from its first send to its arrival decision.
- **idle**: `nav_walk_idle_ms`, for each segment after the first, the time from the previous
  segment's modeled arrival to its send (mean and max; power-of-two buckets in the log line).

The scenario fails unless `nav_blocked_total` and `nav_desync_total` are both 0. Each walking lane
(#305, #306, #307) reruns it and appends its rows:

```sh
go test -count=1 -timeout 12m ./internal/e2e -run 'TestLiveWalkEfficiency' -v
```

| Route | Build | Date | Segments | Length | Straight | Elapsed | Route run | Straight run | Efficiency | Bot | Idle mean / max | Blocked | Desync |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| town | main 6199fe3 (before #305) | 2026-11-24 | 11 | 3295 | 3020 | 36.98 s | 26.15 s | 23.97 s | 0.707 | 0.691 | 1076 / 1706 ms | 0 | 0 |
| field | main 6199fe3 (before #305) | 2026-11-24 | 14 | 6503 | 5668 | 65.06 s | 51.61 s | 44.98 s | 0.793 | 0.788 | 1025 / 1857 ms | 0 | 0 |
| town | wf/walker-wake a3d8512 (after #305) | 2026-11-24 | 10 | 3235 | 3021 | 28.41 s | 25.67 s | 23.98 s | 0.904 | 0.894 | 100 / 101 ms | 0 | 0 |
| field | wf/walker-wake a3d8512 (after #305) | 2026-11-24 | 14 | 6494 | 5674 | 55.64 s | 51.54 s | 45.03 s | 0.926 | 0.921 | 100 / 101 ms | 0 | 0 |
| town | wf/follow-keepwalk 151a503 (after #306) | 2026-11-24 | 15 | 3260 | 3021 | 29.94 s | 25.87 s | 23.98 s | 0.864 | 0.855 | 100 / 101 ms | 0 | 0 |
| field | wf/follow-keepwalk 151a503 (after #306) | 2026-11-24 | 25 | 6777 | 5673 | 61.46 s | 53.79 s | 45.02 s | 0.875 | 0.870 | 100 / 101 ms | 0 | 0 |
| town | wf/nav-long-segments 95d875c (after #307) | 2026-11-24 | 9 | 3206 | 3021 | 27.92 s | 25.44 s | 23.98 s | 0.911 | 0.899 | 100 / 100 ms | 0 | 0 |
| field | wf/nav-long-segments 95d875c (after #307) | 2026-11-24 | 16 | 6748 | 5690 | 58.59 s | 53.56 s | 45.16 s | 0.914 | 0.908 | 100 / 100 ms | 0 | 0 |
| town | main 49f10a4 (before #308) | 2026-11-25 | 8 | 3227 | 3021 | 27.82 s | 25.61 s | 23.98 s | 0.921 | 0.909 | 100 / 100 ms | 0 | 0 |
| field | main 49f10a4 (before #308) | 2026-11-25 | 16 | 6731 | 5669 | 58.25 s | 53.42 s | 44.99 s | 0.917 | 0.912 | 100 / 101 ms | 0 | 0 |
| town | wf/nav-click-ahead 457f696 (after #308) | 2026-11-25 | 9 | 3214 | 3021 | 27.92 s | 25.51 s | 23.98 s | 0.914 | 0.902 | 100 / 101 ms | 0 | 0 |
| field | wf/nav-click-ahead 457f696 (after #308) | 2026-11-25 | 17 | 6837 | 5691 | 57.53 s | 54.26 s | 45.16 s | 0.943 | 0.932 | 56 / 101 ms | 0 | 0 |
| town | wf/nav-click-ahead 50ca07f (after #308) | 2026-11-25 | 10 | 3274 | 3033 | 28.65 s | 25.99 s | 24.07 s | 0.907 | 0.892 | 89 / 101 ms | 0 | 0 |
| field | wf/nav-click-ahead 50ca07f (after #308) | 2026-11-25 | 16 | 6784 | 5662 | 56.92 s | 53.84 s | 44.93 s | 0.946 | 0.935 | 60 / 101 ms | 0 | 0 |

On the baseline, the 10 town idles fell in the buckets up to 511 ms (1), 1023 ms (4) and 2047 ms
(5). The 13 field idles fell up to 255 ms (1), 511 ms (1), 1023 ms (4) and 2047 ms (7). A
segment waits about a second after its arrival, the brain tick of `docs/walking.md`.

After #305 every idle, 9 in town and 13 in the field, fell in the bucket up to 127 ms: the walk wake
sends each segment one position tick after the modeled end, so the idle is that tick by
construction. What the wake saves shows in the elapsed time: 8.6 s on the town route and 9.4 s on
the field route, efficiency 0.707 to 0.904 and 0.793 to 0.926, with no blocked stop or desync, so
no segment went out before the server's move had ended. The rest of the gap to run speed is the
start-speed updates and the answer latency of each segment (#306, #307, #308).

After #306 the scenario's follow walk takes segments of at most `movingSegment` (500): 15 instead
of 10 in town and 25 instead of 14 in the field. Every idle is still the one position tick (all in
the bucket up to 127 ms) and nothing was blocked or desynced, but each extra segment pays its
answer latency and the server's walk-speed start, and the shorter pull bends the field route 283
units longer: efficiency 0.904 to 0.864 in town and 0.926 to 0.875 in the field. That is the cost
of the cap toward a leader that stands still; the scenario's GM never moves. Owner, 2026-11-25:
a follow toward a leader that stands still takes the long segments of #307, without the cap (#344).

After #307 the pull reaches past the corridor test with exact-checked candidates, within the same
500 cap: 9 segments instead of 15 in town and 16 instead of 25 in the field, efficiency 0.864 to
0.911 and 0.875 to 0.914, the elapsed time 2.0 s and 2.9 s shorter. Every idle is still the one
position tick, and nothing was blocked or desynced. Fewer segments mean fewer answer latencies
and walk-speed starts, which is most of what the elapsed time saved.

After #308 the field route clicked 7 of its 16 later segments ahead in the first run (457f696)
and 6 of 15 in the second (50ca07f): those idled 0 ms (the bucket up to 0), the others one tick.
Its efficiency went from 0.917 to 0.943 and 0.946 (the bot's own from 0.912 to 0.932 and 0.935),
0.7 s and 1.3 s faster on a route 53-106 units longer, with nothing blocked or desynced. The town
route clicked none of its 8 later corners in the first run and 1 of 9 in the second: most of its
corners are where the string pull wraps a house or a fence, and a line from any point before such
a corner to the next waypoint cuts through the obstacle, so those fans fail and the segment goes
out from standstill as before (0.921 to 0.914 and 0.907, one or two segments more, within the
run-to-run spread of the route the pull takes). Only corners that wrap nothing qualify, mostly
where the 500 cap of a follow walk splits open ground, so the gain is under the 5-7% `walking.md`
estimated for every waypoint: about 3 points in the field, none in town.

What #306 buys shows with a leader that moves. `TestLiveFollowMovingLeader` starts the bot 1900
from the field route's end, in sight of the GM, who paces 300 back and forth there while the bot
walks; the bot's follow delay is 1.5 s, so a re-path is pending for most of each leg. Its gaps are
the time from the modeled end of one of the bot's moves, as the server answered them, to the start
of the next:

```sh
go test -count=1 -timeout 12m ./internal/e2e -run 'TestLiveFollowMovingLeader' -v
```

| Build | Date | Legs | Segments | Gap mean / max | Idle mean / max | Blocked | Desync |
| --- | --- | --- | --- | --- | --- | --- | --- |
| main 673403d + the scenario (before #306) | 2026-11-24 | 11 | 18 | 1011 / 1841 ms | 739 / 1601 ms | 0 | 0 |
| wf/follow-keepwalk 151a503 (after #306) | 2026-11-24 | 6 | 13 | 311 / 401 ms | 100 / 101 ms | 0 | 0 |
| main 3f62617 (before #307) | 2026-11-24 | 7 | 17 | 326 / 399 ms | 100 / 101 ms | 0 | 0 |
| wf/nav-long-segments 0849cc1 (after #307) | 2026-11-24 | 12 | 14 | 349 / 381 ms | 100 / 101 ms | 0 | 0 |
| main 49f10a4 (before #308) | 2026-11-25 | 8 | 7 | 337 / 382 ms | 100 / 101 ms | 0 | 0 |
| wf/nav-click-ahead 457f696 (after #308) | 2026-11-25 | 7 | 7 | 321 / 382 ms | 100 / 101 ms | 0 | 0 |
| wf/nav-click-ahead 50ca07f (after #308) | 2026-11-25 | 18 | 15 | 358 / 383 ms | 100 / 101 ms | 0 | 0 |

Before #306 the bot stood through the follow delay after each segment that ended while a re-path
was pending; after it, the bot keeps walking to its old slot and turns to the new one when the
delay ends, so it arrives in about half the GM's legs. The gaps include the server's walk-speed
start of each move, which is why they exceed the bot's own idle.

#307's longer segments broke the scenario's start: it took the first waypoint of the bot's route
within 1900 of the end, and with segments as long as a click that waypoint was the end itself, so
the bot started at the goal and walked no leg. The start is now the point on the route where it
first comes within 1900, on the segment that crosses that distance. That start lies farther out
than the waypoint before #307 did, so the GM paces more legs; the rows are not comparable in legs.

After #308 the follower clicked none ahead in either run: every idle is still the one tick, and
the gaps are unchanged within the spread. The leader's slot moves on while a segment is in flight, and each move
past `walkSameDest` replans the walk, so the segment in flight rarely has a planned next waypoint.

## Delivery (milestone M7, "Own navigation", epic #151)

| PR | Scope | Closes or supersedes |
| --- | --- | --- |
| N0 (#152) | Settle the open server points by test: head-level Z on multilayer, the longest tick elapsed time `Emax` measured live, the echo of `pathDirect`. Write `bots geoquery`. File the server issue for a `geoprobe` walk category. Record the findings in this document | — |
| N1 (#153) | `internal/geo` copy with its dependencies (location, door template, geometry, properties, `doors.xml`): stored form, `ParseRegion` views, cache writer and reader, mmap on Unix and Windows, readers for `bots geopack`, queries, step chain `WalkSteps`, door overlay. Flags `-geodata`, `-geoengine` and `-geodata-cache`; `docs/home.md` steps (rsync with mtimes kept, `bots geopack`). Synthetic fixtures, memory test, local parity run, measured heap and RSS for the 139 regions | — |
| N2 (#154) | `internal/nav` on cells: directed edges, clearance cost, open-corridor test and exact stretch check, smoothing, snap, stand points (routes inside one cluster) | — |
| N3 (#155) | Hierarchy: entrances per crossing (merged only when mutually reachable), directed abstract graph, edges with a door set and a door-free cost, SCCs and condensation, the cluster-local Tarjan membership rule on static doors-open blocks (in X, one-way out, isolated), the byte-bounded LRU and per-walker pins, `bots navbuild` and its cache file, benchmarks and final thresholds | — |
| N4 (#156) | Door table (mutex writes, per-door atomic reads, per-bot dedupe), `DoorInfo` and `DoorStatusUpdate` decoders, toggles into the shared engine; door-aware route search with the doors-ignored second search | — |
| N5 (#157) | Walker: one-way-out drop and isolated-only `/unstuck`; step-chain position, `CanMove` plus the corridor test and exact check before sending, standstill sends, blocked (including the talk `StopMove`) and desync handling, watchdog, swing and cast hold. NPC visits, errands, melee, casting, pick-up and ranged approach on stand points. `unstick` sidesteps and `walkStuck` deleted | #122 class |
| N6 (#158) | Spots and trips on SCCs and abstract distance; hops, stalls and the hour-long avoid deleted | #144 |
| N7 (#159) | Follow on routes with one-way drops toward the leader; detour machinery deleted; flee, idle drift and death return | #147, #148 |
| N8 (#160) | e2e scenarios and metrics above; the PR rows of `docs/plan.md` updated for D13 | — |

N1 to N4 change no behavior. From N5 on, each PR moves one set of consumers over and deletes their
heuristics in the same PR. N5 needs N4, so no consumer uses routes that ignore doors.

Walking efficiency (the stop at every waypoint, short segments, the follower's hold, click-ahead at
checked corners) is planned in [`walking.md`](walking.md) and delivered by #304 to #308.

## Risks

- **The geodata copy drifts from the server's** (the server's geodata is updated and the local copy
  is not). The cache key rejects a changed local copy, and the desync counter catches a changed
  server copy at the first segment. `docs/home.md` keeps the geodata in the same rsync step as the
  datapack.
- **Tick jitter in the step chain.** Open corridors of one-layer cells are immune to it; one over
  cells of several layers, like the exact check, holds for steps up to `Lmax + 3`. The exact
  check covers every step length in `(0, Lmax]` on the stretches that need it; the clearance cost
  keeps routes off such stretches where it can; a blocked stop is a replan. What is left, ticks
  past `Emax` inside exact stretches or over cells of several layers, is measured by
  `nav_blocked_total` against its e2e threshold.
- **Stale or misordered door state** (a door no bot sees, changed by a script or timer, or events
  from lagging sessions): it shows up as door replans, never as plain unreachable.
- **Multilayer Z** (bridges, buildings): settled in N0 (the floor Z goes out; a layer less than
  twice the collision height under another is no click target), pinned by the parity queries.
- **Memory and CPU on the owner's machine**: one shared mapped engine and nav cache, both read in
  place; searches bounded by a budget and run synchronously, at most one full route per decision;
  offline builds. The thresholds above are acceptance criteria.
- **Windows mapping**: the server has no Windows mapping to copy. N1 tests the `MapViewOfFile`
  shim in the `windows` job of `.github/workflows/go.yml`.

## Gaps

- Fences and doors outside `doors.xml`: resolved in #162. Every door, siege and scripted doors
  included, is a `doors.xml` door, so N4 covers them all. Fences are GM-only and not modeled; a
  fence across a route shows up as blocked stops or desyncs ("Server behavior this rests on",
  "Dynamic objects outside `doors.xml`").
