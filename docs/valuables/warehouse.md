# The private warehouse (V3a)

The warehouse errand (`warehouse.go`) gains a `Private` request flag. Server facts:
[server](server.md#server) ("Private warehouse links" through "Warehouse request gate").

## Requests

- **Links and lists:** `_DepositP` and `_WithdrawP` instead of the clan links; type 1 lists (a type
  2 list is refused for a private request); 282 is `WarehouseEmpty`.
- **Private slot, no clan:** today `startWarehouse` returns early unless `Persona.Warehouse.Want` is
  set or the crafter's cycle runs, and unless the bot is in a clan (`if (p.Want == nil &&
  !b.crafting()) || ... || b.world.Clan.ID == 0 || ...`, `warehouse.go:588-590`), and takes its
  request only from `warehouseWant` (crafter cycle, else `Want`, `crafter.go:214-224`). V3a adds
  `e.privateWant`, at most one queued `WarehouseRequest` with `Private` set, which rule "valuables"
  fills before calling `startWarehouse` and `stepWarehouse` itself. `startWarehouse`'s guard returns
  early only with no `Want`, no crafter cycle and no queued private request (the travel map, keeper,
  `openDue` and `retryAt` checks stay), `Clan.ID == 0` refuses only a clan request, and the queued
  private request is taken before `warehouseWant`, clearing the slot when it starts.
- **Priority and gap:** so a private request goes ahead of the clan member's and the crafter's,
  which wait as for any running request (a crafter cycle never meets it: rounds and errands require
  none). Both kinds share the one open gap: `openedAt`, the `timeouts` backoff (60 s doubled per
  unanswered open, at most 30 min) and `retryAt` (`warehouse.go:281-297,302-311`), since the
  server's gate does not tell them apart; 60 s between opens except a deposit after its own
  withdrawal.
- **Cancel:** `cancelPrivate(now)` ends a private request before its request packet went out
  ([round-end](round-end.md#cut-off)).

## Keeper choice

A private request goes to the nearest keeper in sight, not marked `noPrivate`, within `RoundReach`
(6000) of the anchor, else the nearest such keeper spawn (travel map); none fails it
`WarehouseUnreachable`, which `roundReady`'s keeper check makes rare. The errand does not read the
round: V3a adds `Anchor` and `Reach` to a private `WarehouseRequest` (today `Withdraw`, `Deposit`,
`Look`, `warehouse.go:176-179`), and the keeper rule reads them; rule "valuables" (V3b for the look,
V7a-V7c for rounds and errands) fills them from the anchor and `RoundReach` (a `ValuablePolicy`
default from V2b), so V3a stands alone. The anchor, not `storeTownRadius` (3000,
`storeregistry.go:12`) of the town point, since in four towns no private keeper is within 3000 of it
([keepers by town](round.md#keepers-by-town)). A clan request keeps today's rule: nearest in sight
anywhere, else the nearest spawn within `Reach` (8000) of the bot (`warehouse.go:585-637`, `Reach`
at `warehouse.go:244`). The walk, `warehouseChats` and the end at the keeper are today's; rule
"valuables" [walks back](round.md#back-to-the-anchor). The private link is looked for on the first
page, then on `Chat` pages (19 first pages, e.g. Wilford 30005; 11 second pages, e.g.
`30083-1.htm`).

## Keeper marks

`noClan` (`warehouseLink`, `warehouse.go:440-470`; read at `warehouse.go:607`) is neither set nor
read for a private request. A keeper whose pages lack the link the request needs (`_WithdrawP` for a
look or withdrawal, `_DepositP` for a deposit) gets `noPrivate`, keyed by NPC template: 20 of the 50
spawned keepers ([server](server.md#server), "Private warehouse links"; an allied page lacks
`_WithdrawP`, so a look still marks Jaff and Hagos).

- A request ended by a new mark ends `WarehouseUnreachable` with no `retryAt` (no open went out):
  the next decision queues it again and the next unmarked keeper is taken, at most once per keeper
  within `RoundReach`; with none left the town has no private keeper for `roundReady`, the look and
  the pending round.
- The set is an errand field V3a adds; V3b hands it the set kept in the [`valuables`
  value](ledger.md#the-valuables-value), so marks survive a reconnect (no revisit, no `retryAt`
  wait). They are per bot and process: a process restart learns them again.

## Refused links

A 142 (the [request gate](server.md#server), `Chat` links included) sets no mark. Today the errand
ignores system messages at its talk step (`warehouse.go:675-676`), times out, talks again, skips the
`Chat 1` link it followed and marks the keeper for the session (`warehouse.go:451-468`), which would
end every later private request where the only keeper in reach has its link on `Chat 1` (11
keepers). V3a takes a 142 at the talk step, after a link went out, as a refusal: `WarehouseRefused`
(`fail`, `retryAt` 60 s on, `warehouse.go:350-360`), with neither `noPrivate` nor `noClan` set. Clan
requests too.

## Private status

Today one `WarehouseStatus` (`Contents`, `Known`, the last `Withdraw` and `Deposit` results,
`warehouse.go:226-230`; `status`, `warehouse.go:287`) serves the clan code: the withdraw list and
282 set it (`warehouse.go:672,679-682`), `trackContents` applies moved rows
(`warehouse.go:715-736`), `depositRows` counts free slots (`warehouse.go:545-546`), and the clan
member reads it (`clanmember.go:170,221,428,439`). V3a adds a second field, `private`, and each
place uses the status of the running request's kind (`e.statusOf(req)`), so a private request never
touches the clan view. A private deposit's appended row also keeps the item's `ObjectID` and
`Enchant` (today `WarehouseItem{TemplateID, Count}`, `warehouse.go:734`; [ledger](ledger.md#rows)).
The status is the brain's, unknown until the session's [look](ledger.md#every-session-looks-first).

## Fees and slots

- **Fee:** 30 a row, checked by `depositRows`. `reserve()` covers `KeepAdena`, the trip home and
  `memberFees` (`budget.go:36-44`); V7a adds `valuableFees`: 30 times the larger of `HoldRows` and
  the carried valuable rows (released ones included once the budget is spent), so no shop visit
  spends the deposit fees ([release](release.md#the-cap) on why `HoldRows` alone bounds nothing).
- **Slots:** a private deposit counts against the private contents and 100, or 120 for a dwarf
  (`WarehouseNoDwarf`/`WarehouseDwarf`, `model/actor/player/character_storage_limits.go:23-33`; the
  unseen `whLimit` is left out, which only undercounts), not `clanWarehouseSlots` (150,
  `warehouse.go:40`). Only valuables go there, far below 100; a deposit past the free slots fails
  like any failed deposit.

## Tests

`TestPrivateWarehouseErrand` (V3a):

- **Start:** a queued private request starts with no `Want`, no crafter cycle and no clan, and ahead
  of a due clan `Want`; an empty slot and no `Want` start nothing; `stepWarehouse` steps a request
  without rule "warehouse".
- **Links:** exact `_DepositP` and `_WithdrawP`; a type 2 list refused; 282 `WarehouseEmpty`, 279
  `WarehouseRefused` as today (`WarehouseShort` only from the fee check before the open); links on a first page and on `Chat 1`; `noClan` neither set nor read. A 142 to
  the `Chat 1` bypass: `WarehouseRefused`, `retryAt` 60 s on, neither `noPrivate` nor (clan)
  `noClan`; a later request there follows `Chat 1` again.
- **Cancel:** `cancelPrivate` on the walk, at the talk, with the open link sent, and with the list
  received: no result, no `retryAt`, status unchanged, `openedAt` kept (a later list ignored); a
  request at `whSent` keeps running; a clan request is left alone.
- **Status:** for a clan member with a known clan view, a private look (type 1 list or 282),
  withdrawal and deposit leave the clan `Contents`, `Known`, `Withdraw`, `Deposit` unchanged and
  fill the private status; a private deposit counts against the private contents and 100 (120
  dwarf); its row keeps `ObjectID` and `Enchant`, so a withdrawal naming that object takes it, not
  another enchant of the template.
- **Keeper:** nearest within the request's `Reach` (6000) of its `Anchor`; a nearer keeper in sight
  farther from the anchor is not taken; none within is `WarehouseUnreachable`. Dion ([keepers by
  town](round.md#keepers-by-town)): from Trisha's spawn 30058. Freight-only: with Rant nearest and
  Rolfe a little farther, Rant's pages showing only `Chat 1`, then only
  `_DepositF`/`_WithdrawF`, a look follows `Chat 1`, marks Rant, ends `WarehouseUnreachable` with no
  `retryAt` and no open, and the next decision's request goes to Rolfe; a marked keeper in sight is
  not taken; a clan request neither sets nor reads `noPrivate`. Hagos (31381) with a link-less page
  is marked the same; with `_DepositP` and a `Quest ... Withdraw` link (an allied page) a look marks
  it. Every keeper within `Reach` marked: `WarehouseUnreachable`, no walk. A handed-in set is read
  and filled in place (a new errand given it skips Rant). A clan request keeps today's keeper rule.
