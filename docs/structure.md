# Hardening, route planner and brain structure

Status: draft, not approved. It proposes owner decisions SD1-SD5 below; they take `D` numbers when
approved (provisional `D` numbers collided once already, #637). No issue is filed yet.

This plan comes from a review of the whole module on 2026-11-26 (`main` at 4f17fe2). The review
found a small set of defects, two gaps in how the route planner joins a route's ends, and a brain
whose size comes from ownership, not from repeated code. It also looked for repeated code worth
one shared mechanism, and for decisions a well-known algorithm or a small trained model would make
better than today's formulas. Each section says what was verified and how.

Server paths (`internal/commons/...`, `internal/loginserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang); bot paths name this
repository. Function names are given instead of line numbers, which drift. Ids in this document
carry two letters (DF, HD, RT, BR, GM, WR, OP, SD) so they collide with no other plan's.

## Problem

### Defects

"Verified" means read in the bot code and, where it rests on the server, in the server code.
"Reproduced" means a failing throwaway test (not kept; each fix starts by adding it for good).

| # | Where | What happens | Evidence |
| --- | --- | --- | --- |
| DF1 | `session.go` (`retryMax`, `Bot.after`), `gate.go` (`Turn.Dialed`) | A ban at accept is never waited out. The server accepts a banned address and closes the connection before any byte (`filteredListener.Accept`), so the bot reads EOF at "read Init", or EOF or a reset at "read VersionCheck". That is not `ECONNREFUSED`: `Dialed` resets the refused backoff and `after` falls to the bot's own backoff, capped at `retryMax` 5 min and jittered into 150-300 s. The filter forgets an address only after `BanTime` 300 s with no connection, and a refused connection counts as one (`AcceptFilter.record` sets `lastAccess` before it tests `banned`; `Sweep`). One bot retrying keeps the address banned. | Verified, both sides; the bot's view reproduced on both ports |
| DF2 | `session.go` (`Bot.after`), `gate.go` | Nothing backs the fleet off while a server is not ready. With the game server down, the login server marks it offline and answers `PlayFail` after a full credential login (`loginPossible`, `onRequestServerLogin`); `after` sends that, and the system `LoginFail`s, to the bot's own backoff only. Each of N bots then logs in every 150-300 s: at 178 bots, one credential login every 1.3-1.8 s for the whole outage. A second path has the same root: when the game port refuses while the login server still lists the game server (a wrong `-game` address, a firewall), the successful login dial calls `Dialed(nil)`, which resets the gate's refused backoff before the game dial is refused, and `after` returns no delay, so the fleet logs in every 2-3 s. | Verified, both sides |
| DF3 | `nav/abstract.go` (`abstractSearch`, `searchSeated`) | A start and a goal in one cluster skip the abstract graph (`a.res.cluster == b.res.cluster` returns an empty node path), so the route is one cell search over the whole world. Across a barrier whose way round leaves the cluster it expands cells until the way round is found or `walkBudget` (400000) is spent. | Reproduced: 50 cells apart across a wall, the wall's end 167 cells away, fails at a 40000 budget and routes at 400000. At the production budget it fails for barriers of about 500 cells (the review's estimate, not measured), and costs a flood for every smaller one |
| DF4 | `nav/abstract.go` (`lastLeg`, `route`) | A goal enclosed by a closed door answers `ErrBudget`, not `ErrDoor`. `lastLeg` finds no seat, falls back to a search `anywhere`, and spends the budget; `route` returns any error but `ErrUnreachable` at once, so the second search with every door open, the one that tells `ErrDoor`, does not run. The walk ends with reason "route budget" after one flood. An enclosed start already answers `ErrDoor` (its free search exhausts the room). | Reproduced, goal side only |
| DF5 | `walker.go` (`walker.drop`) | The drop off one-way-out ground tries up to 4224 cells (rings to `walkDropCells` 32), each with a full `Planner.Route` of `walkBudget`, and no total bound, on the brain goroutine. `Hierarchy.Reachable` is labelled with every door open, so a closed door between the ledge and the ground makes each candidate a failed full-budget search. | Verified by reading; not reproduced |
| DF6 | `session.go` (`Bot.attempt`, `Bot.play`), `botregistry/supervisor.go`, `cmd/bots/swarm.go` | No `recover` exists in the module, so a panic in one bot's brain or reader ends every bot and the panel. `docs/agents/go-style.md` ("Errors and panic isolation") asks for one at the session loop. A runtime throw is not recoverable and stays #616's. | Verified |
| DF7 | `reader.go` (`readEvents`), `session.go` (`afterSession`, `Bot.after`), `botregistry/supervisor.go` (`ended`) | A decode error in the bot (short or trailing bytes in one packet) ends the session and is counted as a server abort; after `maxAborts` 3 the bot stops with `ErrRepeatedAborts`, which the supervisor restarts after its cool-down of 10 minutes, for good, and logs as the server's doing. One changed packet layout does that to every bot. A decode error before the world (login, `EnterGame`) is retried on the bot's backoff for good, with no counter. | Verified |
| DF8 | `botmetrics/registry.go`, `botregistry/supervisor.go` | Metrics slots outlive a deleted bot. | Verified |

DF1 on the home run: D9 runs the bots `-allowlisted` and the filter skips allowlisted addresses.
But the gate only trusts the flag (D9: the bots know nothing of the range). If the address is in
fact off the server's list, unspaced dials are banned, and the retries keep the ban as above. A
spaced gate can be banned by a second dialer behind the same address (another `bots` process, a
game client), since spacing is per process. D11 accepts the ban risk on the home run; this plan
only makes a ban end.

Documented, not a defect, and next to DF3: `Hierarchy.Distance` joins each end to the entrance
nodes of its cluster by a straight line and answers the straight line for two cells of one cluster
(`docs/navigation.md`, "Distances for ranking"). The review measured 500 and 700 against a walk of
about 16000 round a wall. Spot ranking (`candsWhere`, `claimSpot`) sorts by it.

### Route ends

`docs/navigation.md` ("seating the ends", #256) describes how a route joins its two ends to the
abstract graph: a straight line first, a search inside the cluster when the leg is refined, and a
second abstract search from the nodes that search reached. DF3 and DF4 are two cases it leaves
out: two ends in one cluster, and a goal no seat reaches under the door state.

Planning cost: #497 measured route planning at 47 % of CPU samples at 178 bots and was closed on
2026-11-25 with its fix (plan metrics, the spot back-off); the profiles after it are still owed
(#535, #579). The route ceilings were reset by the owner on 2026-11-23 (`docs/navigation.md`,
"`nav` performance": a 40000-unit route warm p50 at most 75 ms, p99 at most 750 ms; last measured
p50 61-73 ms). So this plan proposes no speed work before a new profile (RT3).

### The brain: ownership, not duplication

Measured on `main` at 4f17fe2:

| Measure | Value |
| --- | --- |
| `internal/botclient` | one package, 144 source files; 44900 lines, about 30200 of them code |
| Methods on `*Brain` | 883; about 94 fields |
| Most complex functions of the module (branch count) | `Brain.handle` 68, `World.Apply` 60, `Brain.send` 39 |
| Commits to the package since its first (2026-11-16) that changed `brain.go` / `rules.go` | 95 / 77 of 221 |
| Test files that read brain fields directly | most of 273 (a grep over field names matches 170 to 200) |

A feature is wired by hand in hub files:

1. a row of `playRules` (`rules.go`), and of `storeRules` when store mode runs it;
2. calls from `Brain.handle`: 15 run on every event (7 before `World.Apply`, 8 after) and about 50
   sit in the 31 arms of its type switch, 22 of them into 8 hooks that switch on the type again;
3. calls from `Brain.send`: 26 hooks, 17 of them a trailing list every packet runs through;
4. a name in `karmaRules` (`pvp_karma.go`), keyed by the rule's string name;
5. a `defer` in `Brain.Run` (five today);
6. for some features, calls in `wake` (`tidyPartyClaim`, `resolveOrder`, `interruptsDue`,
   `edgeCross`, `inspectWake`), `decide` (`syncValuables`, `endStaleRound`, `karmaTick`), `start`
   (`armPrep`, `applyStanding`), `NewBrain` (`wireValuables`, `wireMember`) and `Run`'s select
   (`obey`, `command`).

The order inside `handle` carries meaning: `inParty` is read before `World.Apply` and used after
it; `pvpEvent` returns the time the final reaction switch uses; `spoilEvent` reads the cast in
flight before `castRefused` clears it; `standCaster` reads `castAt` before its arm zeroes it.
`decisionAt` merges the next wake from seven sources by hand, and `wake` spends five of them.

The survey for repeated code ("Shared mechanisms" below) found 550-700 repeated lines, about 2 % of
the package. The size of `Brain` is therefore not duplication to factor out. Feature logic is
written as `Brain` methods over the whole brain, and nothing but convention keeps one feature out
of another's state, or the wire code out of the brain (`channel.go` holds a decoder, its `World`
apply and a rule; CLAUDE.md section 4 asks for that boundary).

## Goal

1. Fix DF1-DF8, each with a regression test that fails first.
2. Close the two gaps at a route's ends (DF3, DF4) without changing a route that works today
   within a stated cap, and decide speed work on a new profile.
3. Move a feature's wiring onto its rule row where that is shown to change no behavior (the sent
   hook, the karma flag, the release), and say plainly which wiring stays where it is and why.
4. Replace the repeated shapes that have three or more real instances with one small mechanism
   each, and name the ones that do not.
5. Record what the bots observe at their spots, so a formula can be checked against what happened;
   then list the algorithms and small models that become options.

Non-goals:

- a rewrite of the brain or of the planner. The ordered rule table, the cell graph and the two
  checks of every segment (`docs/navigation.md`, "Goal") stay;
- moving the event hooks of `Brain.handle` into lists or a subscription ("Considered and left
  out" below);
- a rule engine, a behavior-tree library, a planner (GOAP, HTN) or a learned policy in place of the
  rule table ("Options", declined rows);
- a navigation mesh, jump point search or any-angle search ("Options", declined rows);
- one generic lease table for the shared registries: the survey found seven different shapes;
- server changes.

## Server behavior this rests on

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Accept filter | Per listener and per remote address. A connection within `BurstWindow` 1 s of the address's previous one is a try; the try after `BurstLimit` 3 bans. A slower connection is admitted and clears no try. `lastAccess` is set on every connection, a refused one included. Only `Sweep` (every 5 s) forgets an address, once `lastAccess` is more than `BanTime` 300 s old. | `internal/commons/netutil/acceptfilter.go` (`record`, `Sweep`, `DefaultAcceptFilterConfig`, `acceptSweepInterval`) |
| A connection closed at accept | Three causes close a connection before any server byte: the accept filter's ban (`filteredListener.Accept`); the login server's ban list, which holds the 10 minute credential ban and admin bans (`ClientLink.handleConnection` returns before `Init` when `bans.IsBanned`); the game listener's ban list (`netutil.RefuseBanned`). The login server writes first (`Init`); on the game port the client writes first (`ProtocolVersion`), so the bot may also see a broken pipe or a reset on that write. | `acceptfilter.go`; `internal/loginserver/clientlink.go`; `internal/commons/netutil/banlist.go` |
| Allowlist | An allowlisted address is admitted by the accept filter and never recorded. | `acceptfilter.go` (`Allow`, `allowlisted`) |
| Game server down | The login server marks a game server whose link dropped offline, and answers a server selection for it with `PlayFail`, after the credentials passed. | `internal/loginserver/gameserverlink.go` (deferred `MarkOffline`); `clientlink.go` (`loginPossible`, `onRequestServerLogin`) |
| A login that never reaches the game server | The session is kept until 60 s after it connected (`releaseSession`, `LoginTimeout`). A login of the same account inside that time answers `AccountInUse` and drops the session, so the one after passes. | `clientlink.go` |

## Design

### 1. Hardening (HD1-HD6)

**HD1, closed at accept (DF1).** A dial that connects and then ends before the first server frame
is a distinct error, `errClosedAtAccept`: EOF or a reset at "read Init" on the login port; on the
game port EOF or a reset at "read VersionCheck", or a broken pipe or reset on "send
ProtocolVersion". The turn reports it to the gate as its own outcome, and the gate goes quiet for
`banQuiet` 330 s (the server's `BanTime` plus its sweep interval plus a margin): no bot of the
process dials before that, since any dial would refresh the accept filter's ban. After it the next
turn is the probe; a probe closed at accept starts another quiet period. `Bot.after` returns no
delay of its own for it: the gate holds every bot.

It applies to the spaced and to the allowlisted gate alike: the allowlisted gate only trusts its
flag. An allowlisted gate that sees one close at accept also turns its spacing on for the rest of
the process: `spaced` becomes an atomic the turn holder sets, and `spacing`, which an allowlisted
gate leaves at zero, is raised to at least `dialSpacing`. If the address is in fact off the
server's list, unspaced dials after the quiet period would be banned again within seconds. The
same signal follows a credential or admin ban (the probes go on every 330 s until it
lifts) and a server that crashes between accept and its first write. Taking a crash for a ban
costs one 330 s pause; the reverse costs a ban that never lifts. One log line per quiet period
names the port and says "closed at accept: the address is banned or filtered, or the server
failed"; counter `login_accept_closed_total`. `banQuiet` is a constant with the server default it
mirrors named beside it (Gaps).

Pacing: this only delays dials. The limit that bounds every dial stays the gate's spacing.

**HD2, a fleet backoff while a server is not ready (DF2).** The gate keeps one backoff, 2 s
doubling to 2 min as `refused` does today, and it is reset only when an attempt enters the world,
no longer by each dial that connects. Two things step it:

- a refused dial on either port, as today;
- a *not-ready ending*, `PlayFail` or a system `LoginFail` (`SystemError`, `AccessFailed`,
  `ServerOverloaded`, `Maintenance`), when the last not-ready ending since the last entry came
  from a different bot. Two different bots in a row mean the server, not an account.

`Bot.after` is unchanged: a not-ready ending still waits the bot's own backoff, and a refused dial
none. So during an outage the fleet's logins are spaced by the gate, at most one every 1-2 min at
its cap, each bot returning to the gate when its own backoff ends, as today. One account whose login
always fails (`AccessFailed` for a stored hash that cannot be checked) never steps the gate and
retries on its own backoff as today; two or more such accounts step it between them, and a healthy
bot that reconnects then may wait up to 2 min. The rule needs the turn to carry the bot's identity,
which `Take` does not take today. On a full server the bots over the server's cap step it between
them and probe one at a time, and a bot that drops and reconnects waits in that queue;
`docs/home.md` sizes the server's cap above the bot cap, so that is a misconfiguration.

`AccountInUse` neither steps nor resets the gate: it is about one account. A refused game dial
leaves a 60 s session on the login server, so the same bot's next login inside that time answers
`AccountInUse` once; `afterInUse` already retries it, and the test pins that it does not stop the
bot.

**HD3, panic isolation (DF6).** Two recover points, as `go-style.md` asks:

- in `Bot.attempt`, around the whole attempt (the dials, `EnterGame`, `NewWorld`, `NewBrain` and
  `play`), with `turn.Done()` moved to a `defer` so a panic cannot keep the gate's token. Inside
  `play`, `Run`'s deferred releases have run during the unwind, and `play`'s closing of `done`,
  the connection and the reader join become deferred too. `enter` closes a game connection it
  opened on a panic as on an error, or the server would keep a half-entered session and answer
  the next login `AccountInUse`. `attempt`'s named results still report that the bot entered and
  for how long when the panic was inside `play`, so `afterSession`'s stable-session reset holds;
- in the reader goroutine: a panic becomes the `connLost` it queues.

A panic is its own class: logged with its stack, counted in `bot_panics_total`, and the bot stops
with `ErrPanic` after `maxAborts` of them since its last stable session. The supervisor does not
restart a bot that stopped for it. The PR audits every lock a brain or reader takes for an unlock
that a panic would skip: the shared registries, and `Events.push` and `Events.Drain`, whose
unlocks are not deferred today (a reader left blocked there would hang `play`'s join).

**HD4, decode errors (DF7, decision SD1).** A decode error is wrapped as `errDecode` with the
opcode, in `readEvents` and in the login and `EnterGame` decoders alike. `afterSession` and
`Bot.after` treat it as its own class: counted in `decode_errors_total` by opcode, logged with the
opcode and frame length (no payload), never counted as a server abort. Under SD1's option 1 the
bot stops with `ErrDecode` after `maxAborts` such attempts, and the supervisor does not restart a
bot that stopped for it, since the next session meets the same packet.

**HD5, one search for the drop (DF5).** `walker.drop` becomes one search instead of one per
candidate. A new `Planner.RouteToNearest(from, accept)` expands outward from `from` with no
heuristic, so in order of path cost, for at most the planner's budget (`walkBudget`), and stops
at the first cell it pops that `accept` takes. The walker's `accept` takes a cell inside the box
of `walkDropCells` around the bot whose member is in an abstract SCC the bot's member reaches;
`RouteToNearest` itself also requires the cell to be a click target (`clickable`), since the pull
exempts a route's last cell from that test. The way down may leave the box, as today's free
search may; only the landing is bounded by it. Today's scan of the box stays as a precheck: when
no cell of the box is in an abstract SCC the bot's member reaches, the answer is `ErrUnreachable`
with no search, as today.

It otherwise behaves as `Planner.Route` does: the cell path is pulled into checked segments, a
failed pull avoids the failing edge and searches again within the same budget, a swimming planner
applies `breathOK` and retries dry. With no fixed goal there is no goal column to exempt, so it
takes no cell in an avoided column. The total work is one budget, whatever the doors are.

Three differences from today, which the PR states: it takes the cheapest landing by path cost
where today's rings take the nearest by distance that a route reaches, so the drop taken may
differ; one search gets `maxReplans` pulls in all, where today each candidate gets its own; and a
search with no heuristic spends its budget within about 350 cells, so a drop-off farther than
that along a ledge is `ErrBudget` where today's search toward one candidate may find it.

**HD6, metrics slots (DF8).** A deleted bot's counters are folded into a retained total before its
slot is dropped, so the sums `Registry.Collect` reports never fall.

### 2. Routes (RT1-RT3)

**RT1, two ends in one cluster (DF3).** Today's search stays first, with a cap: the free cell
search runs with at most `ClusterCells` squared cells, the columns of one cluster (65536). A
route it finds is today's route, unchanged. When the cap is spent, the route is planned as a
route between two clusters is: the first-leg search inside the cluster's box seats the start on
the entrance nodes it reaches (or finds the goal there), `reachers` seats the goal, and the
abstract search runs between those seats. `abstractSearch` drops its same-cluster shortcut when
it is given seats; `Distance`, which gives none, keeps it. Every fall back of today (the search
`anywhere` after seating, on a replan, with an avoided column or edge) stays as it is.

The cap and the seating are charged to the route's budget: in the worst case the cap plus about
three floods of one box before a leg is refined. So a planner whose budget is at or under the cap
behaves as today.

This changes a same-cluster route only when its free search needs more than the cap: one that
fails today for the budget gets a route (or still `ErrBudget`, when the cap and the seating leave
too little), and one that succeeds after more than the cap gets the hierarchy's route, which may
be longer than the cheapest. A prototype of this flow passed the whole `internal/nav` suite at a
cap of 1024 and of 65536, and routed a same-cluster pair past the cap at 5615 units against the
free search's 5466.

**RT2, a goal behind a closed door (DF4).** When `lastLeg` finds no seat for the goal, and the
walk has no avoided column and the route no avoided edge, the planner checks the static engine
(every door open) before it falls back: first the same search inside the box from the last node
to the goal, then `reachers` seating. If either succeeds there, the answer is `ErrDoor`. If
neither does, today's fall back runs. The check costs at most two passes over one box.

Two ends in one local SCC that is in no abstract SCC (one-way-out ground they share) match no
seat on either side, so the start side fails first. For two ends in one cluster, when the start
got no seat, the planner therefore runs the first of the two static searches only, from the start
to the goal inside the box, and answers `ErrDoor` if it succeeds. The static seating of the goal
proves nothing there, since the goal's seating under the door state was never tried.

**RT3, measure, then decide speed work.** No speed PR is planned. The profiles #535 and #579 owe
come first; if route planning is still a leading cost, these are the leads, with what the review
found about each:

- *The walker's members.* `Hierarchy.route` locates both ends again with no pins although the
  walker holds both `Member`s; under cache pressure that recomputes a cluster the walker pinned.
- *A shared cache of refined middle legs.* Sound only under a narrow key: the directed pair of
  representatives, the engine (live or static), and the door table's change count, read equal
  before and after the search. It may be read only on a route's first pass (no avoided edge) with
  an empty `Avoid`, and may store only a leg found inside its box. A cached leg is still pulled
  and checked like a searched one. The cell search reads neither `Lmax` nor the collision height,
  so a leg is the same for every bot.
- *Where the remaining `ErrBudget` routes spend.* `docs/navigation.md` reports 4 routes of the
  owner sample still failing for the budget and no middle leg leaving its clusters; a budget per
  leg is a guess until that is known, and a share smaller than one end's flood would fail routes
  that pass today.
- *The diagonal step's own `CanMove`* in `Planner.neighbours`: implied by the two detours in the
  review's count (no refusal in 15.7 million diagonals of the test region; `appendSteps` of the
  build already leaves it out) and about half of an expansion's time. It may go only with a parity
  test over the packed regions that keeps the claim.
- *How many budget-size searches run at once*, and no dedupe of one cluster's membership compute
  across bots that go cold on it together (likely, not confirmed).

### 3. Brain: what moves onto the row (BR1, BR2)

**BR1, row columns.** `rule` gains three:

```go
type rule struct {
	name string
	run  func(b *Brain, now time.Time) (Action, bool)
	// sent sees each packet a decision of this row sent.
	sent func(b *Brain, a Action, now time.Time)
	// karma marks a row a bot with karma skips (today's karmaRules).
	karma bool
	// stop gives back what the row holds when Run ends.
	stop func(b *Brain)
}
```

- `karma` replaces the string-keyed `karmaRules` map. `pvp_pk_test.go` and `pvp_karma_test.go`
  index and range over that map and are edited to read the rows; `RuleNames` is unchanged.
- `stop` takes four of `Run`'s five defers: `leaveSpot` on the "spot" row, `releaseStore` on
  "store", `releasePartyClaim` on "rooms" (the party-matching code is what claims a leader) and
  `releaseCraftBoard` on "craft". One `defer` ranges
  over `playRules` once, rows a karma bot skips included. That is not today's order (craft
  board, party claim, store, spot), which carries no meaning: they touch four separate registries.
  `answerInspect` belongs to no row and stays a defer.
- `sent`: `decide` records the row that matched, and `send` calls that row's hook for the packet of
  that decision. The record is one-shot: `wake` clears it after the decision's send, and when the
  decision holds and sends nothing. So the packets `wake` and `inspectWake` send themselves (logout,
  `Appearing`, the keepalive, the inventory request) find no row, and no hook sees them.
  `storeRules` is built from the `playRules` rows it shares (dead, prompt, command, chat, store), so
  a row has one hook in both modes. A hook moves to its row only when it is shown to act on its own
  row's packets alone. Of the 17 trailing hooks, 7 stay (below), 8 can move as they stand, and 2
  more once their defect is fixed (below). These stay explicit calls in `send`, because they see
  other rows' packets by design:

  | Hook | Sees packets of |
  | --- | --- |
  | `walker.sent`, `loot.sent`, `gear.sent`, `supplyTrip.sent`, `guardSent`, `fightSent` | every rule |
  | `travelTrip.sent` | "travel", and "supply" and "arena", which run `ruleTravel` |
  | `warehouseErrand.sent` | "warehouse" and the rule that started the errand (valuables, craft) |
  | `craftSent` | the warehouse errand's packets, to renew the lease |
  | `tradeSent` | "prompt" (`ruleTrade`) and "valuables" |

BR1 is two steps. BR1a is `karma` and `stop`: it changes no packet and two test files. BR1b is
`sent`, one group of hooks a PR, each listing the hooks it moves and the packets each acts on. BR1b
costs test edits: 57 test files call `Brain.send` directly, some after `decide` and some after
calling a rule function (`enchant_test.go` with `ruleEnchant`, `party_match_test.go` with
`ruleRooms`, `dyes_test.go`); for a moved hook the second kind must go through `decide`, or name the
row, and each PR lists the tests it edits. What BR1b buys is eight to ten fewer calls in `send` and
a hook that cannot react to another row's packet. Two hooks are known to do that by accident:
`trainerVisit.sent` counts any talk click while a trainer visit is open, with no object check, and
`recipeLearn.sent` reacts to any recipe-book request while a scroll attempt is open, which the
"craft" row also sends. Those two are defects with PRs of their own (GM2 for the trainer). BR1b is
proposed only if the owner wants that class closed by structure and not hook by hook (SD5).

**BR2, the three armed wakes.** `assist`, `follow` and `chat` hold the same pair, a time and an
armed flag, spent by hand in `wake`:

```go
type alarm struct {
	at    time.Time
	armed bool
}

func (a *alarm) due() (time.Time, bool)
func (a *alarm) spend(now time.Time) // disarms once at is not after now
```

`decisionAt` and the spending block range over the three. The whisper queue has the same pair per
entry in a slice of its own and keeps its `wakeAt` and `spendWake`. The walk wake (derived from the
bot's own move), the landing floor, the logout deadlines, the keepalive and the inspect deadline
are other shapes and stay.

**Considered and left out.**

- *Event hooks as ordered lists, or a subscription by event type.* `handle`'s hooks do not share one
  shape: `inParty` is read before `World.Apply` and passed to a hook after it, `pvpEvent` returns a
  time the final reaction switch uses, about 20 take a field of the typed event under a test such as
  `e.ObjectID == self`, and several sit on both sides of a core write in the same arm. A list of
  `func(*Brain, Event, time.Time)` would need an adapter per hook that switches on the type again,
  the pattern the switch avoids, and each move would need a proof that the order did not matter.
  `handle` stays a typed switch; its order is the contract.
- *A `decide` that passes to the next rule when the matched action's gate is closed.* The rules
  that check the pacer themselves do so before a side effect (the ad board's claim) or pass on
  purpose, and an errand that waits out the bypass gate must keep the turn, or the rules below it
  would walk the bot away from the dialog.
- *The other wiring sites* (`wake`, `decide`, `start`, `NewBrain`, `Run`'s select): each is one
  call with its own timing, and a list would hide when it runs.

So a feature keeps up to four wiring sites after this plan: its row, its arms in `handle`, a call
in `send` (with BR1b, only when it reads other rows' packets; without it, as today), and a core
call when it needs its own timing. The
structural cause, one package and one `Brain`, is decision SD2's.

### 4. Shared mechanisms (GM1-GM3)

The survey counted instances and read each. Adopted: three or more real instances of one shape.

| # | Shape | Instances | Mechanism |
| --- | --- | --- | --- |
| GM1 | Hold until a time, by key (`now.Before(m[k])`): shunned NPCs, avoided spots, retries, the blacklist | 17 maps, with their lazy-init blocks and 14 prune loops, about 120 lines | `type holds[K comparable] map[K]time.Time` with `set(k, until)`, `extend(k, until)` (keeps the later time, as `failedWalk` does), `held(k, now) bool`, `prune(now)` |
| GM2 | The trainer's own visit | `trainer.go` repeats `npcVisit`'s clicks, talk wait and shun, keyed by object id, with no object check in `sent` | the trainer moves onto `npcVisit.approach`. It changes behavior (the object check, the shun key, the re-pick), so it has its own tests |
| GM3 | Two one-line helpers | jitter `d + rng in [0, j]` 13 times; `"npc_<obj>_<cmd>"` 10 times | `Brain.between(lo, hi)`, `npcBypass(obj, cmd)` |

A map never reset to nil is made in `NewBrain` or its feature's constructor instead of lazily. A
map whose nil means something (`trainerVisit.reset` sets `skipped` to nil; `ads.go` tests `named`
for nil) stays as it is.

Looked at and left alone, with the reason:

| Shape | Instances | Why not |
| --- | --- | --- |
| Picking the NPC to visit (`pickVisit` and the copies in `newbie.go`, `shop.go`, `warehouse.go`) | 5 | The copies differ in more than the shun key: `pickVisit` gates the walk to a spawn on the party and the follow order and the copies do not; the warehouse judges a candidate by its position and drops the reach limit for a private request; the shop scans every NPC against its shop map and returns the lists it opens. One function would need three knobs for five users |
| Request and answer with a reply timeout | about 24 | A shared `await` type saves no lines (set, clear and check stay at three sites), and what a timeout does differs every time (talk again, back off, shun, count toward a breaker) |
| Last-used-at, by key (`now.Sub(m[k]) < T`) | 14 maps | Each call site compares against its own window |
| Errand step machines (step enum, `sent` advances, an event advances) | 9 | Only the outline is shared; transitions and failures are the feature |
| Wire list decoding (`capFor` loop) | 14 | A `readList[T]` saves two lines each and hides the row's field order, which a reviewer checks against the server |
| Process-wide registries | 7 | Seven shapes: an owner lease with expiry (`GroundClaims`, the craft board's warehouse lease and its order claim), a tiered counting semaphore (`SpotRegistry`), capacity by place (`StoreRegistry`), a two-way index (`PartyClaims`), a last-asked window (`ChallengeHolds`), a rate limiter with no owner (`AdBoard`) |

WR1: the review reports two process-level duplications, which the PR confirms first. The shared
set (spots, stores, claims, boards, nav, gate) is built by hand in both `swarm.go` and `ui.go`, and
the party organizer pass has two drivers (`botregistry/party.go`, `botregistry/organizer.go`).

### 5. Package boundary (decision SD2)

CLAUDE.md section 4 says the wire codecs know nothing about the brain. In one package the compiler
cannot hold that. Three ways:

- **a. Keep one package.** The boundary stays a review rule. No churn.
- **b. Move the wire out: `internal/botwire`.** The decoders and encoders reference no `Brain`,
  `World` or `Persona` today. The cost is larger than a move: Go has no function aliases, and the
  encoders have 156 call sites in source and 288 in tests, with 82 test files calling a decoder or
  encoder; `connLost` is a brain-side event and cannot satisfy an interface with an unexported
  marker in another package; several encoders take unexported row types (`buyRow`, `sellRow`,
  `storeRow`, `warehouseRow`). So b means exporting the codec API (or a file of forwarding
  variables), an exported marker or a brain-side wrapper for local events, and moving the decoder
  tests, in one PR, in a window with no open lane on `botclient`.
- **c. Features in packages of their own.** Needs `Brain`, `World` and `Action` exported wholesale
  for every feature and most test files; declined as speculative.

Proposed: a. b is the only step that would make the boundary compiled, and it is priced above so
the owner can choose it; nothing else in this plan depends on it.

### 6. Outcome log (OP0, decision SD3)

Every estimate the bots act on is a closed formula that is never compared with what happened:

| Decision | Today | What the bot sees and drops |
| --- | --- | --- |
| "Can I take this pack" (`packFight`, `packFits`, `danger.go`) | modeled time to kill against modeled time to die for the spot's likely pack, with a fixed margin 1.5, 2 or 3 per row | every hit (`Attack`), its own HP (`StatusUpdate`), deaths (`Die`), experience (`UserInfo.Exp`) |
| Spot choice (`candsWhere`, `claimSpot`) | level window and `packFits`, then the nearest by `Distance`, first fit | experience and adena gained per hour at the spot, deaths there |
| Gear (`pieceScore`, `bestSet`) | marginal modeled kills a minute against one reference monster | the rate before and after a change |
| Valuables price (`valuables.go`) | ask by rarity, 10 points off per unsold round | rounds unsold and the ask are kept in the ledger; agreed prices and counter-offers are not |

Persisted today: the registry JSON, nav and geodata caches, and the decoder fixture tap. Metrics are
process sums served live, not written. The only learned quantities are `useMeter` (a decayed use
rate, half-life 30 min) and `Calibration` (reported over modeled stat ratios).

OP0 records one thing, the **spot stay**: one record per spot a bot farms at, from the time it
first arrives there (`ruleSpot` sets `spot.arrived`) until it gives the spot up (`leaveSpot`, a
death, the session's end). A stay beside a full spot is recorded too. Time
counts only while `arrived` is set: a teleport clears it, an in-place correction included, and a
return to the same spot goes on in the same record. The record needs no guess about who killed
what:

- spot, class, level, row, party size at arrival, minutes at the spot, minutes in combat (the use
  meter's raw fighting minutes);
- experience and adena gained (`UserInfo.Exp` and the adena count, end less start), deaths, HP lost
  per combat minute (the drops in the bot's own `StatusUpdate` HP), HP potions drunk in combat
  (counted where `useMeter.spend` is called; the meter's own totals are decayed), monsters that died
  while the bot was engaged on them and the mean time from the bot's first landed hit to that death;
- the model's kill and die times for the spot's pack at arrival (`packFight`), absent for a row
  with no margin, which `packFits` passes without modeling;
- whether the stay is beside a full spot, in earning mode (`spot.earning`) or at a spot under the
  bot's level window (`spot.lower`).

A party member holds no spot (`spotJoinEvent` gives it up), so a party is measured through its
leader's stay, with the party size on the record.

`Die` names no killer and there is no anti-KS rule, so a party's or a shared monster's kills are
not attributed: the stay's experience is the truth, and the engaged times are marked as shared
when another attacker hit the target.

The log is off unless `-outcome-log <file>` is given. The command that opens the file (`swarm`,
`ui`) starts the one writer goroutine and joins it at shutdown; it does not depend on the metrics
registry, which `swarm` runs without unless `-metrics` is set. A brain hands a record to a bounded
queue; a full queue drops the record and counts the drop, and never blocks a brain. The file is
size-bounded JSONL. No account name, character name, password or chat text is written. #398 (a bot
party's experience per hour against a solo farmer) needs this same measurement.

### 7. Options: known algorithms and small models

Each row names what it would replace and what it needs. A row that needs OP0 cannot be judged
before the log exists; the measure to cross is written in the row, as D2 asks of a fast path.

| # | Option | Kind | Replaces or adds | Needs | Proposed |
| --- | --- | --- | --- | --- | --- |
| OP1 | Calibrated fight estimate: keep `packFight`'s formula and learn two factors per bot, observed over modeled kill time (the stay's engaged times) and damage taken (the stay's HP lost per combat minute against the pack's modeled damage), as decayed means with the class row's pooled mean as prior (the shape `useMeter` has) | online estimate: exponential smoothing with shrinkage | the fixed margins of `packFits` | OP0. Adopt if the log shows the modeled times off by more than 25 % for some row | prototype |
| OP2 | Spot choice as a multi-armed bandit: arms are the spots that pass today's filters; reward is measured experience (earning mode: adena) per hour less deaths; statistics shared across bots by spot, level band and role; the model's `earnRate` as prior; UCB1 or Thompson sampling, with the walk there as a switching cost | online learning | nearest-first in `claimSpot` | OP0. Adopt if measured rates differ between fitting spots by more than the walk costs | prototype |
| OP3 | Reaction and tick delays drawn from a log-normal clipped to today's bounds, instead of uniform. Human reaction times are right-skewed; two parameters, nothing to train | distribution | `reactDelay`, `tickDelay`, the chat delay | `docs/plan.md` section 4 says "Uniform jitter; no seeded distributions", and section 2 "base interval plus uniform jitter": OP3 needs the owner to change that line. It also changes which delay a seed draws, so tests that pin a delay change | owner's call, SD4 |
| OP4 | Whisper replies from a small language model in place of the 8 canned lines, under today's caps (5 an hour, 10 min a sender), the canned line on any error or timeout | hosted small model, or a local one outside the process | `ChatPolicy.Replies` | A hosted model sends a player's whisper text off the owner's machine; a local one takes memory beside the bots (`docs/home.md`). The call cannot run on a brain goroutine: one process-wide worker with a bounded queue owns it, and the reply returns through the bot's mailbox as a new kind of mail, dropped, not queued, once the session has ended. The player's text is untrusted input: the model gets no bot state and no tools. Never the merchant's price whispers: money stays with rules | owner's call, SD4; off by default |
| OP5 | Sale price from a fitted demand curve: the chance of a sale against ask over reference, by rarity (logistic regression) | small offline model | the linear markdown of `valuables.go` | a sale history, which the ledger does not keep; few sales a day, so months of data | not now |
| OP6 | Spots assigned centrally as an assignment problem (Hungarian method, or min-cost flow with capacities) | known algorithm | per-bot first fit against `SpotRegistry` | a coordinator pass; bots stop choosing like players. Adopt only if #355's acceptance (most farm bots hold or share a spot within 15 minutes) fails | declined for now |
| OP7 | Party composition by weighted matching with a swap improvement pass | known algorithm | the one-pass greedy of `botparty/compose.go` | evidence of poor parties in `parties_broken_total` by reason; none today | declined for now |
| OP8 | Crafting plan as an integer program or a bounded knapsack | known algorithm | the greedy with rollback of `craftplan` | a solver dependency against `go-style.md`'s dependency rule; no report of a bad plan | declined |
| OP9 | Gear search | — | `bestSet` | the space is linear in sets and body-legs pairs, already enumerated; `bestSet` wants shorter code, not an algorithm | declined |
| OP10 | A shared memory of blocked columns: a decayed process-wide penalty where bots met blocked stops or desyncs, in place of an `Avoid` that lives for one walk | online cost update | per-walk `Avoid` | a count of blocked stops by place first; adopt if the same columns repeat across bots | measure first |
| OP11 | Exact walking distance to a few fixed places (a trainer, a gatekeeper, a spot centre) from a reverse shortest-path tree over the abstract graph | known algorithm | `Distance`'s straight ends for those places | memory per tree (the review estimates about 7 MB; not measured). The tree is exact between entrance nodes only; the end inside the bot's own cluster stays a straight line | option |
| OP12 | An admissible heuristic that counts the clearance cost (a differential heuristic over the landmarks the cache already holds) | known algorithm | the octile bound of `heuristic` | #315 holds this lead; weighted A* was rejected there by the owner | stays with #315 |

Already in the code or in this plan: the hierarchy is HPA* (Botea, Müller and Schaeffer, 2004) and
RT1 extends its insertion of the endpoints to two ends in one cluster; the landmark bounds in the
nav cache are ALT (Goldberg and Harrelson, 2005); the gate's backoff is exponential with jitter.

Declined, with the reason:

| Option | Why not here |
| --- | --- |
| Jump point search | Needs uniform costs and symmetric edges. Costs here vary (clearance, drops, water), edges are directed (one-way NSWE, drops), columns have layers and doors change edges |
| Theta* and other any-angle search | Makes the line check (corridor fill and `CanMove`) the price of every relaxation and buys only path length, which the pull already gives |
| Navigation mesh | A non-goal of `docs/navigation.md`; and only flat one-layer ground merges into regions where a segment passes by construction, while the nav cache's 421000 abstract SCCs are mostly one-way slope cells |
| Contraction hierarchies | The abstract query is already p50 0.15 ms; door-dependent costs would need the customizable variant |
| Behavior tree library | The rule table is a behavior tree's reactive priority selector already; a library adds a dependency and no behavior |
| GOAP, HTN, Monte Carlo tree search | The errands are protocol state machines with one legal next step, not plans to search |
| Reinforcement learning or behavior cloning for the rule order or the rotation | No simulator: the environment is the live server in real time; the tests pin a deterministic rule order; inference for 1000 bots competes with the machine's cap |
| One host-wide dial gate across processes | D11 accepts the ban risk on the home run; HD1 makes a ban end |

## Owner decisions (proposed)

| # | Question | Options | Proposed |
| --- | --- | --- | --- |
| SD1 | What a bot does on a packet it cannot decode | 1. End the session, count it as a decode error, stop the bot after `maxAborts` of them with `ErrDecode`, no restart. 2. Skip the frame, count it by opcode and play on with that event missing (frames are length-delimited, so the stream stays in step) | 1 |
| SD2 | Package boundary | a. one package; b. `internal/botwire`, priced in Design 5; c. feature packages | a |
| SD3 | Write an outcome log to disk | yes, off by default, bounded JSONL, no names or text; or no | yes |
| SD4 | Which options to prototype | OP1 and OP2 after OP0; OP3 (changes `docs/plan.md` section 4); OP4 (sends whisper text off the machine when hosted) | OP1, OP2; OP3 and OP4 the owner's call |
| SD5 | May these PRs change behavior, and is BR1b wanted | See the Delivery table's last column. BR1b (the `sent` column) changes no packet but edits tests across the package | yes, each PR states its change; BR1b the owner's call |

## Testing

- **HD1.** Table-driven on the injected clock with a fake dialer: after a dial closed at accept no
  bot dials for `banQuiet`, then exactly one does, on a spaced and on an allowlisted gate, and the
  allowlisted gate's dials are at least `dialSpacing` apart afterwards; EOF at
  "read Init", EOF and a reset at "read VersionCheck" and a broken pipe at "send ProtocolVersion"
  all classify; a timeout and `ECONNREFUSED` do not.
- **HD2.** With every login answered `PlayFail`, the gaps between logins of a three-bot fleet double
  to 2 min, and an entered attempt resets them; the same with the game dial refused and the login
  accepted; one bot whose login always answers `AccessFailed` beside two that enter never steps the
  gate, and two such bots do; `AccountInUse` after a refused game dial retries and does not stop the
  bot.
- **HD3.** A brain whose rule panics ends its attempt with `ErrPanic`, gives the gate's token
  back, releases its spot, joins its reader, and leaves a second bot of the same registry running;
  a panic in `EnterGame` does the same and closes the game connection.
- **HD4.** A frame one byte short in the world, and one in `EnterGame`, end the attempt with
  `errDecode` and count no abort.
- **HD5.** A drop whose every candidate lies behind a closed door expands at most one budget; a
  box with no cell in a reached abstract SCC answers `ErrUnreachable` with no cell expanded; a
  ledge whose way down leaves the box and lands inside it still drops; a landing that is no click
  target is not taken. `TestWalkerRecovery`'s "one-way-out drops" (`walker_test.go`) keeps its
  routes or the PR lists the ones that change.
- **RT1.** A kept fixture on a hierarchy of 64-cell clusters (a cap of 4096): a wall across one
  cluster with its way round in the next, where the free search passes the cap and the budget covers
  the cap and the seating; today's answer is `ErrBudget`. A second fixture pins that a same-cluster
  route found under the cap is today's route, cell for cell. A new owner sample of same-cluster
  pairs (under 4096 units apart) beside `BenchmarkOwnerNav`, whose pairs are 2500 cells apart and
  never share a cluster, compares outcome and route length before and after.
- **RT2.** A kept fixture: a goal in a closed room answers `ErrDoor`. `TestRouteAvoid`,
  `TestRouteStraight` and `TestLegSeatFallback`, which route through today's falls back, pass
  unchanged.
- **BR1a, BR2, GM1, GM3.** The suite of `internal/botclient` passes with these test edits only:
  `pvp_pk_test.go` and `pvp_karma_test.go` read the `karma` column instead of the map, and
  renames. One new test per mechanism (`alarm`, `holds`).
- **BR1b.** Each PR lists the tests that call a rule function and then `Brain.send` for a hook it
  moves, and edits them to go through `decide` or to name the row; no assertion changes.
- **GM2.** New tests for the trainer's object check, shun key and re-pick; the trainer tests it
  changes are listed in the PR.
- **OP0.** A record built from a scripted stay on the injected clock; the writer drops under a
  full queue and never blocks.
- No `-race` run on the owner's machine; PR CI runs the unit tier with it. The live scenarios are
  unchanged.

## Delivery (proposed milestone M14, "Hardening and structure")

| PR | Scope | Needs | Changes behavior |
| --- | --- | --- | --- |
| HD1 | `errClosedAtAccept`, the quiet period on both gates, spacing turned on for an allowlisted gate that saw one, the counter; `internal/e2e/target.go`, the other user of `Turn.Dialed` | — | reconnect timing |
| HD2 | The gate's backoff reset on entering the world; not-ready endings of two different bots step it; `internal/e2e/target.go` | HD1 (same files) | reconnect timing |
| HD3 | Recover in `Bot.attempt` and the reader; `ErrPanic`; deferred `turn.Done`, `enter` and `play` cleanup; the lock audit | HD2 (same file) | a panic ends one bot |
| HD4 | `errDecode` in the reader, login and `EnterGame`; its counter; no restart; SD1 | HD3 (same file) | what stops a bot |
| HD5 | `Planner.RouteToNearest`; `walker.drop` on it | — | which drop is taken (the cheapest landing by path cost) |
| HD6 | Fold a deleted bot's counters, drop its slot | — | no |
| RT1 | The same-cluster cap and seated search; the same-cluster owner sample | — | same-cluster routes past the cap |
| RT2 | `ErrDoor` for an enclosed goal | RT1 (same function) | the reason of a walk that fails today |
| BR1a | `rule.karma`, `rule.stop` | — | no |
| BR1b | `rule.sent`, one group of hooks a PR (SD5) | BR1a | no; edits tests |
| BR2 | `alarm` | — | no |
| GM1 | `holds[K]`; never-reset maps made at construction | — | no |
| GM2 | The trainer on `npcVisit.approach` | — | the trainer's clicks and shuns |
| GM3 | `between`, `npcBypass` | — | no |
| WR1 | One wiring site for the shared set; one organizer driver | — | no |
| OP0 | Outcome log (SD3) | — | no |
| OP1, OP2 | Prototypes behind flags, judged on the log (SD4) | OP0 | behind a flag |

Three lanes can run side by side: HD1-HD4 and HD6 (session, gate, supervisor), HD5 with RT1 and
RT2 (`internal/nav` and the walker), and BR, GM (the brain). BR1 touches `brain.go` and
`rules.go`, which 43 % and 35 % of the package's commits change, so its PRs land between waves of
feature work, small and one at a time. RT3 is no PR: it is the profile, then a decision.

## Risks

- **A hook that was not owner-only (BR1b).** Moving it would stop it seeing another row's packet.
  Each hook moves only with that shown in its PR; the ten named above stay in `send`.
- **Merge conflicts with open lanes (BR, GM).** Bounded by size and timing, above.
- **A same-cluster route gets longer (RT1).** Only one whose free search passes the cap; the
  same-cluster owner sample measures how many and by how much, and the cap is the knob.
- **A different drop, or none (HD5).** The cheapest landing by path cost instead of the nearest
  by distance, nine pulls in all, and no drop past about 350 cells along a ledge; the drop tests
  show which change.
- **A quiet period on a false signal (HD1).** One 330 s pause for a server that crashed at accept.
- **A slower return after an outage (HD2).** After a long outage the first probe may come up to
  2 min after the server is back; each bot then returns when its own backoff ends (up to 5 min),
  as today. On a full server, or with two or more accounts that always fail, a bot that
  reconnects may wait up to 2 min a turn.
- **A panic that leaves shared state half-written (HD3).** The bot that panicked stops; the lock
  audit covers what other bots share. A registry whose invariant a panic could break mid-update is
  named in the PR.
- **Learned choices herd or drift (OP1, OP2).** The spot registry's caps still bound a spot;
  today's filters run first, so a learned rate only orders spots the model already allows; both
  sit behind flags.
- **The log grows or leaks (OP0).** Off by default, size-bounded, no names or text, dropped under
  pressure.

## Gaps

- `banQuiet` mirrors the server's default `BanTime`; a server configured with a longer one needs
  it raised by hand. The bot cannot read the server's setting.
- `Distance` stays a lower bound with straight ends inside the endpoint clusters, so spot ranking
  stays blind to a wall or river inside the bot's own cluster. OP11 narrows it for fixed places
  only; nothing in this plan fixes it.
- A feature still has up to four wiring sites (Design 3), and wire, world and brain stay in one
  package unless SD2 chooses b.
- Not checked for this plan and left to their PRs: which `sent` hooks are owner-only (BR1b), the two
  process duplications (WR1), the tree size of OP11, the barrier size at which DF3 fails at the
  production budget.
- Reported by the review, unconfirmed and not planned: dial spacing is measured from the dial's
  start, not the accept, so a retransmitted SYN could land two accepts under a second apart on a
  spaced gate.
