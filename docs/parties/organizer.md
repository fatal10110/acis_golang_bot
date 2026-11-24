# The organizer

`internal/botparty`, new and pure, ships in P5a; P5b wires it in `internal/botregistry` (supervisor
wiring, `Sent` polling, the registry fields and pin rows, `TestSupervisorPartyTick`). Who is grouped
with whom: [composition](composition.md). The orders it sends: [orders](orders.md).

## Plan

A pure planner with no goroutine, no clock of its own and no wire dependency:

```go
// Plan returns the organizer's next state and the orders that move the parties toward the groups
// policy wants, given its current state and every candidate bot, at now. It never mutates s.
func Plan(s State, cands []Candidate, p Policy, now time.Time) (State, []Order)
```

- **Candidate** (one per registry bot with a live run): account, character name, role (preset,
  D21), class id, level, healer flag and `Status.Heals` ([composition](composition.md#healer)),
  town, position, HP, live party (leader and member names), slot (`Status.Party`),
  `Status.PartyLeft`, `NewRun`, `Sent` (below), the panel's standing order, `Status.Whispered`,
  run state (online in the world, or reconnecting), registry party mode. It holds only what the
  supervisor reads this pass; every timer lives in `State`.
- **State** (returned by one pass, handed to the next): `Parties []Party`; `Freed
  map[account]time.Time` (when a bot left a party; starts `Cooldown`); `PairBans
  map[[2]account]time.Time` (until when two bots may not share a party, `RePairAfter`).
- **Party**: id, leader, members, formed at, state (`forming`, `active`, `broken`) and why, pinned,
  and timers (zero while the condition fails, set the first pass it holds): `NoHealerSince` (no
  living healer member online); `NoSpotSince` (leader online in the world, `Spot` empty; farming
  beside sets `Spot`, and a gone leader is timed by `LeaderGoneSince`); `LeaderGoneSince` (below);
  `GoneSince map[account]time.Time` (member not online in the world); `BehindSince
  map[account]time.Time` (member online but out of party range, 1500, of the leader; offline
  snapshots keep stale positions, [server](server.md#the-bot-today)).
- **Order:** `lead <names>`, `join <leader> <members>` or `unparty` for one account.

**`LeaderGoneSince`** holds while the leader is not online in the world, or is online with `lead`
answered nil and leads no party or another (read only from an online snapshot). Before `lead` is
delivered only the first condition counts; after, members never accepting (10 `InviteRetry`
rounds by `LeaderGone`) break the party. "Leads no party" counts only while a bot member is online:
with every member gone the server dispersed the party (`party/party.go:337`), so each is timed by
its `GoneSince` and the leader's timer starts the first pass one is back unpartied. A party turns
`active` on the first pass whose online leader snapshot lists one of its bot members.

## Supervisor wiring

The supervisor goroutine owns the `State` (and `FormParty`, `DissolveParty` edit it there): no new
goroutine or shared table. `PartyTick` (30 s, injected clock) joins `Run`'s `select`
(`supervisor.go:252-274`) beside the cool-down `arm` (`supervisor.go:572-589`) and the flush
(`armFlush`, `supervisor.go:310-321`); orders go through `Bot.Command`, which never blocks
(`mailbox.go:136-152`).

## Which orders arrived

`Bot.Command` answers `ErrBusy` when the mailbox (4 deep, `mailbox.go:11`) is full, else returns a
channel answered once a session's brain carried the order out; between sessions it waits
(`mail.queued`). The supervisor keeps per account the last order handed to the current run and its
answer channel (`Sent`), and polls every open channel without waiting before each pass:

| `Sent` state | Meaning | What `Plan` does |
| --- | --- | --- |
| none, or `ErrBusy` at `Command` | never entered the mailbox | plans the order again |
| `ErrBadCommand` at `Command` | `Command.valid` refused it (a bug) | logs it, breaks that party, never resends |
| pending | in the mailbox or queued for the next session | reads nothing into the slot and sends that bot no further order (a break's `unparty` included) until the answer |
| answered with an error (`ErrStopped`, a refusal) | not carried out | plans the order again |
| answered nil | the brain set the slot | from now on an empty slot means the bot cleared it itself |

The brain sets or clears the slot, publishes at once (`publishStatus` with force), then answers;
answers are polled before snapshots are read. Every run gets a new `Bot` (`supervisor.go` `start`,
the `Live` hook; `cmd/bots/ui.go:218`): such a candidate is `NewRun` and its `Sent` is dropped (the
old mailbox answers `ErrStopped`). Against the record: a new or undelivered run lacking its slot is
ordered it again; a delivered run whose slot reads empty cleared it itself and is
[dropped](#breaks-and-drops); a delivered run whose slot differs gets its recorded slot (only the
organizer sets slots); a run in no record with a slot set gets `unparty`.

## Formation order

A bot must hold its `join` slot before its leader's invitation arrives, or the persona and the claim
table (one bot per non-GM leader, `partyclaim.go:61-73`) decide. So `join` goes first and `lead`
on a later pass, once every named member's `join` was answered nil. A membership change re-sends
every remaining member its `join`; a fill sends the new and longer `join`s before the longer
`lead`; a drop sends the shorter lists in any order.

## Eligible bots

Online in the world, preset `farmer` (Damage) or `support` (Support), party mode not `off`, no
standing panel order (`stay`, `go`, `follow`, `assist`), `Status.Whispered` false (`Order` shows only the standing order,
`status.go:91,137`; a whispered one lives in the brain's `orders`, `command.go:183-193`), in no
party but a bot party the organizer formed, and with a town (a travel map). `merchant`, `crafter`
and `town` never take part.

## Breaks and drops

A member is **dropped** (that pass) when it cleared its slot, was gone past `LeaderGone` (5 min, the
plan's leader wait; back sooner, it is re-invited), fell behind (past `BehindFor`, 10 min; the pair
banned for `RePairAfter`, the leader ousting it) or was made ineligible on purpose (mode `off`,
preset out of `farmer` and `support`, a standing or whispered order): out of the record, cooled
down, `unparty` if it has a live run and no order pending (else once answered; meanwhile at most a
`PartyTick` in the [join hold](members.md#join-hold)), shorter `join`s and `lead` to the rest. One
fitting free candidate a pass fills a short party; working parties are never merged or reshuffled.

An auto party is **broken** (an `unparty` to every member, the leader first) when:

- the leader is gone past `LeaderGone` (`LeaderGoneSince`), made ineligible on purpose or cleared
  its slot itself;
- no living healer member has been online for 10 minutes (`NoHealerSince`);
- the online leader has had no spot, held or beside, for `NoSpotFor` (15 minutes, longer than a
  death, supply trip or errands; [spots](spots.md#no-gatekeeper-trip)); leader and members are then
  paired no more for `RePairAfter` (30 minutes, `PairBans`);
- no bot member is left: `unparty` to the leader alone, never an empty `lead`
  (`mailbox.go:137-139`);
- the bot members' spread exceeds `MaxSpread` (5);
- the panel dissolves it or switches auto parties off ([panel](panel.md)).

A freed bot waits `Cooldown` (5 min from `Freed`) before it is grouped again. Auto parties live in
the supervisor's memory and form again after a panel restart.

## Pinned parties

Registry (`botregistry.Bot`): `PartyMode` (`""` auto, `"off"`), and for a pin `PartyGroup` (a name)
and `PartyLead`, written atomically like the other fields. A pin forms from its group whatever band
and composition say, and of the breaks above only `DissolveParty` and a bot leaving the pin (below)
apply: a pinned bot only gone
keeps its place and gets its slot again with a live run. While the pinned leader is offline its members get no `join`, first or
resent (one holding its slot stands in the join hold at most `JoinWait`, then plays its persona);
once it is online, `join`, then `lead`. Pin orders replace an earlier standing order; a whispered
member answers `ErrWhispered` and is ordered again once `Whispered` clears.

A bot leaves the pin, not ordered back, when its delivered slot reads empty with `PartyLeft`
`left` (any [slot clear](orders.md#slot-clearing) after the pin), or the panel sets it `off`
or out of `farmer` and `support`. `Plan` drops it as an auto member, dissolving the pin (an
`unparty` to each, leader first) when it was the leader or no bot member is left; the supervisor
rewrites the pin rows from the returned records (clearing that bot's `PartyGroup` and `PartyLead`,
every bot's for a dissolved pin, `DissolveParty` included), so no pass or restart re-forms it.
`PartyLeft` `timeout` gets `join` again once the leader is online; an auto party treats both values alike.

## Tests

Planner (unit, table-driven: `State`, candidates, fixed `now` in; `State` and orders out):

- `TestPlanSkipsIneligible`: `merchant`, `crafter`, `town`, mode `off`, a standing order,
  `Whispered`, a reconnecting run, a player's party.
- `TestPlanBreaks`: each break gives `unparty`, leader first, at its timer (10, 15 or `LeaderGone`
  minutes; one minute short, nothing). `Spot` empty online starts `NoSpotSince`, a claim or beside
  clears it, offline starts none; the no-spot break bans the leader with each member. Spread 5
  holds, 6 breaks; a player far above does not count. A party of two whose member cleared its slot:
  `unparty` to the leader, never an empty `lead`. A gone member keeps its place, unordered, at
  `LeaderGone` minus one minute and clears it on return; at `LeaderGone` it is dropped (cool-down,
  shorter lists) and the next pass fills. A member set `off`, preset changed, given a standing
  order or `Whispered` is dropped that pass. A stale position starts no `BehindSince`; a stale
  leader snapshot naming its party starts `LeaderGoneSince` and breaks at `LeaderGone`. Party of
  two, member gone: the partyless leader starts none; the member back at `LeaderGone` minus one
  minute starts it, with no break before the re-invitation. A member dropped with `join` pending
  gets `unparty` the pass after it is answered nil. `forming` with `lead` undelivered starts no
  `LeaderGoneSince`; delivered and no party, it breaks at `LeaderGone`. A leader snapshot listing a
  bot member turns the party `active`.
- `TestPlanState`: a forming pass returns the party `forming`; a new condition sets its timer to
  `now`, a gone one clears it; a freed bot gets `Freed`, a fallen-behind pair `PairBans`; the input
  `State` is unchanged.
- `TestPlanFormationOrder`: the first pass orders each member `join` (leader and other bot members)
  and no `lead`; `lead` comes the pass after every `join` was answered nil, not while one is pending
  or refused; a fill orders the new and every longer `join` before the longer `lead`.
- `TestPlanSlotResent`: a `NewRun` member with no slot, or one whose order got `ErrBusy` or an error
  answer, is ordered again; one pending with an empty slot is kept, ordered nothing and not cooled
  down over several passes; only one answered nil whose slot then reads empty is dropped (and the
  leader gets its shorter `lead`); a leader so breaks the party.
- `TestPlanSlotDiffers`: a delivered member whose slot names another leader is ordered its recorded
  slot; an `ErrBadCommand` record breaks its party and is not resent.
- `TestPlanCooldown`: a freed bot is not grouped before 5 minutes, nor re-paired with the member
  that fell behind before 30.
- `TestPlanPinned`: a pin forms out of band and no break applies (a bot gone past `LeaderGone`, no
  healer, no spot, spread past `MaxSpread`); a member with `PartyLeft` `left` (a GM's `leave`, a
  standing order after the pin, a GM's `invite me`) is dropped, gets no `join` after and is no
  longer pinned; the leader so dissolves the pin (an `unparty` to each, the leader first);
  `timeout` gets `join` again once the leader is online; while the leader is offline no member gets
  a `join`; a member set `off` leaves the pin.

Supervisor (unit, injected clock, fake runs, `-race` in CI):

- `TestSupervisorPartyTick`: plans every 30 s through `Bot.Command`; a busy mailbox is retried
  next pass without blocking; answers are polled without waiting and set `Sent`; a new run drops
  its `Sent`; one pass's `State` is the next pass's; a pin whose registry rows the test writes
  forms again after a restart; a member that left the pin has its rows cleared and does not.
