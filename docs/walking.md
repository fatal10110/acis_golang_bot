# Walking efficiency

Walking bots run a bit, stop, run again, stop again, and their walks take too long. This document
gives the cause, the fix, and how each fix is measured. It extends the walker of
`docs/navigation.md` (section 3, "Walker") and keeps its rule: every move a bot sends is a segment it
has checked against the server's own geodata code.

Tracked in milestone M7 (epic #151):

| Issue | Scope | Depends on |
| --- | --- | --- |
| #304 | Measurement: idle metric, live route timing | none (run first, as the baseline) |
| #305 | Walker sends the next segment when the move ends | none |
| #306 | Follower keeps walking through the follow delay; segment cap for moving destinations | #305 |
| #307 | Longer checked segments in `internal/nav` | #306 first or together |
| #308 | Click-ahead at checked corners (deferred design; needs the owner's sign-off) | #304's numbers after #305-#307 |

The plan went through four rounds of independent review against the bot code and the server
reference. The last round left no blocker or major finding.

## Diagnosis

Server references are to the `acis_golang` checkout (`internal/gameserver/...`); bot references
are to this repository.

1. **Idle at every waypoint.**
   - The walker never clicks while the bot moves: every segment goes out from standstill (`walker.go`,
     `goTo`).
   - The next segment goes out only on a brain decision. Decisions run on the regular tick, 1 s plus
     0-0.5 s of jitter (`brain.go`, `defaultTick`, `defaultTickJitter`).
   - Nothing wakes the brain when a segment arrives. The server sends nothing on a normal arrival,
     and `MoveToLocation` and `StopMove` are not salient (`brain.go`, `salient`).
   - Result: about 0.6-1 s idle at every waypoint.
2. **Idle after a blocked stop or a desync.** Those answers are not salient either, so the replan also
   waits a full tick.
3. **Walk-speed restart.**
   - A player's move from standstill runs its first 5 position updates at the walk speed when running
     on land (`model/actor/move/creature.go`, `walkStartUpdates`, `updateSpeedLocked`;
     `model/actor/player/character_attack.go`, `moveStartSpeed`). In water or flying it starts at full
     speed.
   - Retargeting a move in flight keeps the update count (`moveToLocationLocked`).
   - Every stop therefore costs about 0.15 s more on restart.
4. **Short segments.** Measured on the owner's caches (`nav-water-main-v3.cache`, 100 sampled routes
   per length, collision height 23, Lmax = 126 × 0.2):

   | Route length | Segments per route | p25 | Median | p90 | Under 300 units |
   | --- | --- | --- | --- | --- | --- |
   | ~3000 units | 6.8 | 128 | 170 | 1729 | 73% |
   | ~10000 units | 20.4 | 128 | 151 | 2042 | 74% |

   - The p25 is exactly `exactCells` (8) × 16. When the corridor test fails, `pull` (`nav/route.go`)
     tries only candidates within 8 path cells, although `stepChainOK` (`nav/exact.go`) already
     exact-checks only the stretches that fail.
   - The corridor test fails often (62% of segments, `docs/navigation.md`, "nav performance"). It
     takes a full one-cell ring around every line cell, and it fails every multi-layer cell, even one
     whose other layer is far above or below.
5. **Follower hold.**
   - Each time the leader moves `Repath` (250) from the anchor, `ruleFollow` holds for `followDelay`
     (0.2-0.8 s) without calling `goTo` (`follow.go`).
   - A follower whose segment ends in that window stands until the hold ends, then resets the walk.
   - Other consumers whose destination moves more than `walkSameDest` also reset the route
     (`walkStep`). A reset lets the segment in flight run out and adds no hold of its own.

With today's segments, a 10000-unit walk has about 20 stops of about 1 s each, on top of the
running time.

## Why the bot does not just send the next segment early

The server does not queue moves. It keeps one destination per character, and a new
`MoveBackwardToLocation` replaces it at once (`moveToLocationLocked`):

1. a catch-up step moves the bot along the current segment to where it is now, P, short of the
   waypoint W1;
2. the server then aims from P straight at W2, cutting the corner at W1;
3. the click is a fresh one, so P to W2 goes through the server's route resolution
   (`resolvePathLocked`).

P to W2 is a line the bot never checked, and corners usually exist because something is in the way:
- if `CanMove(P, W2)` fails, the server's own pathfinder routes around (a desync);
- if a 100 ms step fails, the server stops the bot partway (a blocked stop);
- if neither route works, the server walks the bot to the first wall on the line.

The bot also cannot click exactly at W1: latency and the server's 100 ms update phase blur the
server's position by tens of units. A click sent too early cuts the corner; one sent too late
arrives after the stop. So the fix first removes the waiting at the stop and the stops themselves
(#305, #306, #307), and treats click-ahead as a separate design (#308).

## Walker: send the next segment when the move ends (#305)

### Walk wake

The walk wake is derived from Self's motion and consumed once per value. No wake time is stored.

- At each `decisionAt`, the walk's due time is `Self.move end + positionTick`.
- It is offered only when both hold:
  - the walk's status is walking, and either a walker segment is in flight (`seg.sentAt` set) or
    the last decision called `goTo`. A new walker field `asked` is set in `walkStep` and cleared at
    the start of each decision;
  - the due time is later than `w.wokeAt`, the time the last walk wake fired.
- `Brain.wake` sets `w.wokeAt = now` once now is at or after the due time.
  - It does so where it spends `follow.wake`, before the Appearing branch: a paced Appearing returns
    before any decision.
  - It does so whether or not the decision reaches `goTo` (a hold, a higher rule, a paced send).

Consequences:
- `decisionAt` never stays in the past. A fired due time is at or before `wokeAt` and is not offered
  again, so the loop cannot spin on `timer.Reset(0)`.
- When Self's move end moves later after firing, the new due time is after `wokeAt` and the wake
  re-arms. That happens on a resample (UserInfo, ValidateLocation) or a speed change the server
  re-times (`retimeLocked`).
- A decision that a higher rule holds mid-segment (a salient self `StatusUpdate`) keeps the
  segment's arrival wake. A walk that no rule drives any more stops waking once its last segment
  ends.
- It covers every case in which the walker waits for a move's end:
  - a normal segment answer;
  - a reset to a new destination, which keeps the segment in flight (`reset`, `segmentDone`);
  - a desync or door replan: the server walks its own destination, and the walker replans once that
    move ends;
  - a blocked stop: `Self.move` stops at now, so the replan runs one tick later.
- Replans stay capped by `walkMaxBlocked`, `walkMaxDesync` and `walkMaxDoor` per route.

### One arrival model

Today `observeWalk` computes the eta with `legTime` from `e.Origin`, before `World.Apply` builds
`Self.move` from `sample(now, e.Origin)`. Neither knows the start speed. The change:

- **Self-only motion fields**, held behind a pointer like `swimLine` so the `Object` record stays
  compact (`TestObjectRecordCompact`):
  - the request anchor: receipt of the answer that started the move from standstill;
  - whether start speed applies: running on land; not in water or flying; in walk mode the start
    speed equals the move speed.
- **The move's end is computed like the server's arrival timer**: ceil(travelTicks) × 100 ms, with
  the first 5 updates at `WalkSpeed`/10 per tick (`travelTicksLocked`, `rescheduleLocked`). Each change
  of move type along a wet line (wading in or out) adds one tick, since the server re-times there.
- **The anchor and the update count survive resample, MoveToPawn and a retarget while moving.**
  - Resample re-times from now, with updates done = floor((now − anchor) / 100 ms), for the distance
    left at the current speeds, as `retimeLocked` does. Counting fewer updates leaves more
    start-speed ticks and a later end, which is conservative.
  - MoveToPawn copies the Self-only fields instead of building a fresh literal; today it also drops
    `swim`.
- **Fresh start.** The anchor resets when a move starts from standstill. The bot's judgement of
  standstill errs late: its end is rounded up and anchored at the answer's receipt.
  - So any new Self move received within positionTick + A of the modeled end counts as a fresh start
    (anchor reset, start-speed ticks again), since the server may already have stopped and restarted
    its count.
  - A is the walker's own answer latency, from `seg.sentAt` to the answer's receipt: the maximum
    over its last 8 segments (`walkLags`), floored at positionTick. The bot measures no other
    round trip.
  - Counting a fresh start where the server only retargeted makes the end later: conservative.
- **The distance timed** is the larger of |e.Origin → dest| and |sample estimate → dest|. `sample`
  keeps the estimate within `snapDrift` (200).
- **No eta is stored any more.**
  - `segmentDone` holds when the segment is answered and now ≥ end + positionTick: the same
    quantity the wake uses.
  - The watchdog deadline is end + run/2 + `walkSlack`.
  - `observeWalk` keeps its answer classification before Apply.
- **Safety.**
  - The server arrives no later than request receipt + ceil(ticks) × 100 ms, re-timed the same way
    on a speed change. The bot's anchor is the answer's receipt, which is later.
  - So end + positionTick is never early unless the server's actor queue lags more than 100 ms plus
    latency. The click is then a retarget from just short of the waypoint, on the checked segment's
    line. `nav_blocked_total` and `nav_desync_total` measure it.

### Pacing

- One `MoveBackwardToLocation` per segment, through the frame pacer (`gateNone`).
- A segment's next click waits for the server's answer plus the move's end + 100 ms. That is at most
  10 walker moves per second (16-unit segments) and about 1 per second typically, far below the
  server's 80 frames/s.
- Replans after blocked stops and desyncs are capped per route. Route-planning CPU per replan is
  unchanged.

### Tests

Table-driven, with an injected clock:
- an answered segment: the next decision comes at the move's end + 100 ms, not at the next tick;
- the end for running on land, walk mode, swimming and a wade;
- a mid-walk UserInfo speed change, and a ValidateLocation: the wake moves later and re-arms;
- a decision held by a higher rule spends the wake, and the loop sleeps until the regular tick;
  with a segment in flight, a salient self StatusUpdate keeps the arrival wake;
- a reset mid-segment still wakes at that segment's end;
- a desync: replan at the end of the server's move; a blocked stop: replan one tick later; the
  counters still end the walk unreachable;
- a zero `wokeAt` fires at most one extra decision;
- an attack or talk click received right at the modeled end counts as a fresh start;
- a paced Appearing does not spin;
- `TestObjectRecordCompact` still holds.

## Follower and moving destinations (#306)

- **Keep walking through the follow delay.** While a follow delay is pending and the bot is
  traveling, `ruleFollow` keeps calling `goTo` toward the old slot instead of returning a hold, so
  the next segment goes out.
  - When the delay ends, the new slot resets the walk. The walker's reset lets the segment in flight
    run out and replans from its end.
  - A bot standing at its slot keeps the human-like delay before it moves.
- **Segment cap for moving destinations.** A segment in flight cannot be cut short, so a walk whose
  destination keeps moving must not commit to a long one.
  - `nav.Planner` gets `MaxSegment` (zero = `navSegment`), read by `inReach`. It therefore applies to
    every pull: `Planner.Route`, `Hierarchy.Route`, `RouteOneWay`, the one-way drop and the swim
    planner.
  - The walker sets a constant `movingSegment` = 500 for `walkFollow`, `walkMelee` and `walkFlee`,
    and `navSegment` for fixed destinations (`walkPlace`, `walkTalk`, `walkPickup`, `walkRanged`,
    `walkIdle`).
  - The cap is a constant, not a persona knob: a persona's `Repath` must not shrink melee or flee
    walks.
- **Tests.**
  - The leader moves `Repath` while the bot is mid-route: during the delay, the bot is never idle
    past its segment's end + 100 ms.
  - Follow, melee and flee routes have no segment longer than 500.
  - `Hierarchy.Route` and `RouteOneWay` honor `MaxSegment`.

## Longer checked segments (#307)

1. **Corridor margin from the server's own cell walk.**
   - Replace the one-cell ring with every cell within R of the segment's line.
   - R covers the band of step points (`bandRadius`, 1.5). It also covers the cells engine `CanMove`
     visits between two step points: it picks the next row with `int(oy + m*(checkX-ox))`, which
     truncates toward zero (`geo/engine/engine_query.go`), about 1 unit, worse at negative
     coordinates.
   - R is derived from that code and pinned by a property test: random step chains (`WalkSteps`,
     random step lengths up to Lmax, both coordinate signs) whose `CanMove` cell walks stay inside the
     corridor.
2. **Multi-layer cells.** A corridor cell with several layers passes when the step chain can only
   land on one of them.
   - Each corridor cell C gets a walking layer L_C, propagated from the segment's start floor.
   - C passes only when both hold:
     - for every corridor cell O within one step (Lmax + 2 × `stepSlack`) that a step can start from
       before reaching C, `Height(C, L_O + 2*CellHeight) == L_C`. Height is the nearest layer, ties
       down (`geo/engine/engine.go`; `stepLocked`);
     - for every corridor neighbour N, `belowCell(C, L_N + 48) == L_C` (`engine_query.go`).
   - A cell reached with two different walking layers fails. The climb condition (at most 48) is
     evaluated between walking layers.
   - Every `CanMove` of the chain then starts at goz = heightNearest(oz) = L_O, crosses cells by
     belowCell(goz + 48), and ends on goz == Height(target), both L_C.
   - **First step.** This rests on the first step being bounded like the others.
     - The server sets `rs.lastUpdate` at the request (`moveToLocationLocked`) and measures the first
       update from it (the position update, `playerPassed`).
     - So the "first step may be any length" assumption is dropped from the exact check and from
       `docs/navigation.md` ("Facts it rests on").
     - The exact check's start states for a stretch away from the segment's start take Z from the
       corridor's walking layer of their cell, not `Height(bp, a.Z)`. A stretch beginning on a deck
       reached by stairs then starts on the deck.
     - A test checks that the step-chain check and `WalkSteps` agree on a first step of every length
       up to Lmax.
   - **Stall caveat.**
     - The server's ticker runs missed updates back to back (`commons/scheduler/ticker.go`), so a
       request during a stall gets one long first step: the same Emax caveat as every step.
     - An arrival timer that fires before any update snaps to the destination without a `CanMove`,
       so it cannot block.
     - Single-layer corridors stay immune to stalls. Corridors over multi-layer cells now rest on
       step length, and `docs/navigation.md` ("What it does not cover") must say so.
   - The proof goes into `docs/navigation.md`. Multi-layer fixtures (bridge deck over ground, stacked
     floors 40, 60 and 100 apart, stairs) go into `TestStepChainAgreesWithWalkSteps` and the corridor
     tests.
3. **Farther exact-checked candidates.**
   - When the farthest corridor-clear candidate is shorter than the segment cap, `pull` tries farther
     path points, farthest first.
   - A candidate must pass `CanMove`, have failing stretches (`corridorStretches`) totalling at most S
     cells, then pass `stepChainOK`.
   - Work is capped per route, counted in `CanMove` calls (the exact check's per-state band checks
     plus each candidate's `CanMove`) and in corridor cells filled. Once the budget is spent, `pull`
     falls back to today's `exactCells` rule for the rest of the route.
4. **Width of the avoided line after a blocked stop.**
   - With longer segments, `walkBlocked`'s Avoid line from the stop point to the segment's end
     over-constrains.
   - A blocked stop means the bot's data differ from the server's (a stale door, a GM fence), so an
     "open" stretch can block too. The server stops before the failing step.
   - Avoid always the line from the stop point to one step (Lmax) plus one cell beyond it, then to
     the end of the first non-corridor stretch after that within the segment.
   - When the stop lies inside an exact-checked stretch, avoid the whole rest of the segment, as
     today.
5. **Stall exposure** applies per exact-checked stretch and per multi-layer corridor cell walked. It
   depends on the terrain crossed, not on segment length; `nav_blocked_total` keeps measuring it.
6. **Targets**, on the owner caches (the segment-length measurement and `BenchmarkOwnerNav`):
   - median segment at least 600 units, and under 25% of segments shorter than 300;
   - route CPU p50/p99 and per-route heap no worse than #256's latest numbers (warm route p50 112 ms,
     p99 850 ms);
   - replans per minute counted for a follower run, since every follow re-path replans.

## Measurement (#304)

- **Metric** `nav_walk_idle_ms`: for each walker segment, the time from the previous segment's
  modeled move end to this segment's send, in milliseconds. Per arrived walk,
  `nav_walk_efficiency_permille` = route length / (elapsed × run speed), in per mille.
- **Live e2e scenario** on the server stack: one fixed multi-corner route in a town and one in open
  field. For each, record:
  - elapsed time against straight running time;
  - segment count;
  - the idle histogram;
  - `nav_blocked_total` and `nav_desync_total`, both of which must be 0.
- Run it on `main` before #305, after #305 and #306, and after #307. The scenario is
  `TestLiveWalkEfficiency`; its numbers are in `docs/navigation.md`, "Walking efficiency".

## Click-ahead at checked corners (#308, deferred)

Not part of this delivery. After #305-#307 it saves only the walk-speed restart, the answer latency
and one position tick per waypoint: about 5-7% at a 600-unit median segment. It also reverses the
"No click-ahead" rule of `docs/navigation.md` (Walker, Sending), which needs the owner's decision,
made with #304's numbers.

Server facts, checked:
- retargeting a move in flight keeps the start-speed update count;
- the catch-up step is speed × (time since the last update) (`catchUpLocked`), bounded like every
  step. The "catch-up step of any length" wording in `docs/navigation.md` is out of date, and #308
  corrects it either way.

Requirements any click-ahead design must meet:
- Send early only when every line from every possible P (the last stretch of the current segment,
  widened by the timing uncertainty) to the next waypoint passes:
  - `CanMove` with the head-height target;
  - the step-chain check, with NSWE over the union of the lines' quadrants.
- Check the origin in the server's answer (after catch-up) against the checked fan. A miss is an
  unchecked line: stop and replan.
- Tell a stale blocked correction or `StopMove` of the previous segment, arriving after the early
  click, from the new segment's answer. Today `observeWalk` would charge it to the new segment and
  build a wrong Avoid line.
- Handle a click that lands after the server already stopped blocked: it starts from the stop
  point, outside the fan.
- Handle the sticky block: a catch-up into a closed line makes `routeBlocked` sticky, and the arrival
  at the next waypoint is then reported as a blocked correction, which `walker.go` currently reads as
  an arrival.
- Rework `standing()` and the `!Moving` gate in `walkStep`. `standing()` snaps a mid-move estimate to
  a layer, and both assume a standstill origin.
