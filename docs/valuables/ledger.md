# The valuables ledger

What the bot holds, owned by the current brain goroutine: the `Bot` hands it to each session's brain
like the chat log (`chat.go` `chatLog`).

## Entries and units

- **Entry:** one per template and `Enchant`, holding only what the bot cannot read back: rounds
  advertised unsold, current ask, released or not.
- **Units, derived** on each decision. Carried: inventory items (`World.Inventory.Items`,
  `items.go:37-51`: `TemplateID`, `Count`, `Enchant`) of its template and enchant passing the
  [valuable test](score.md#valuable-test); stored: rows of the private status (`private.Contents`,
  [warehouse](warehouse.md#private-status)) of its template and enchant. So a trade sale
  (`SendTradeDone`, `trade.go:257`, then the `InventoryUpdate`), a store sale, a drop or a destroy
  removes carried units, and a withdrawal or deposit moves them, with no hook of its own.
- **Added** when a carried item passes the test and no entry has its template and enchant (rounds 0,
  the [first ask](whispers.md#prices)). A unit found while its entry exists joins it with its rounds
  and ask (the template is already on offer at that price).
- **Dropped** at the first decision with no unit carried and the private status known with none
  stored, so a sold valuable leaves the ledger, ads and cap at once; never while the status is not
  known (each session until its look). A unit found after a drop starts a new entry.

## Stored rows with no entry

A private status row (look or withdraw list) with no entry adds one (rounds 0, the first ask, not
released) when it passes the **warehouse row test**. A `WarehouseItem` (`warehouse.go:75-84`) has
`TemplateID`, `Enchant`, `Augmentation`, `SubCategory`, no `Equipped` or `ManaLeft`. The test:
`ref ≥ PriceFloor`, `V ≥ ValueMin` at the row's `Enchant`, not sold by a counted buylist (`cheapest`),
not a quest item (`SubCategory` 3, `subCategoryQuest`, `trade.go:31`), not augmented (`Augmentation`
0), not a shadow item (the template's `Duration`, as `indexCrystals`, `crystallize.go:66`). The carry
checks are left out (`keeps`, `shop.go:610`; the persona's keep hook, which takes a carried `Item`,
`hold`/`keepsItem`, `shop.go:625-631`, `clanmember.go:53-61`; worn next; set piece): they decide what
the bot wants to carry, and once withdrawn the carried test decides (a unit the gear errand now
wants leaves its entry). After a process start every stored valuable comes in so. A
row that fails (`ValueMin` raised since) gets no entry, is never withdrawn or capped, and stays
([Risks](README.md#risks)). Only rule "valuables" deposits there, so each row was a valuable once.

## Rows

The cap and the deposit count rows as the server does (`storeSlots`, server
`inventory/warehouse.go:307-316`): a row per unit of a non-stackable template (two +0 Swords of
Watershadow are two objects, two rows), one per stackable template. Withdrawals and deposits name a
non-stackable entry's units by object, as `pickRows` takes them (`warehouse.go:507-520`: a row
naming an object takes only it, `Object` 0 any), so releasing a +0 sword never takes a held +5 one.
Objects come from the look, the withdraw list, or the row `trackContents` appends on a deposit
(today template and count only, `warehouse.go:734`; V3a adds the item's `ObjectID` and `Enchant`).
The server moves a whole item as the same instance, a new id only for a split into a warehouse with
no stack of it (`move`, server `inventory/warehouse.go:329-336`), never a non-stackable unit. A
stackable entry (enchant 0, one row) is named by template.

## Released entries

A round starts only for an entry with carried units not released ([round](round.md#start)). A
**released** entry is in no round: no ad line, reply or trade names it, its rounds and ask do not
step, the deposit leaves it carried. It goes to the store: carried, at the next opening; stored, by
the round's withdrawal or the [off-round withdrawal](round-end.md#off-round-errands), only while the
valuables budget is not spent. The cap counts each entry once, released or not.

## Every session looks first

The private status dies with the session: the errand is a brain field (`wh`, `brain.go:230`),
`NewBrain` builds a brain per session (`session.go:350`), only the chat log is handed across
(`session.go:212`, `brain.go:311-313`), and `trackContents` ignores a status not known
(`warehouse.go:718-720`). So each session **looks** (withdraw list, or 282 for empty) on its first
town visit, before any round and any other private request, valuable carried or not.

- **Conditions** (also the [off-round errands'](round-end.md#off-round-errands)):
  [`roundReady`](round.md#start) without the region and ad board checks, and no `RoundGap` or budget
  wait; the look says nothing, and the board's hour cap could hold a region an hour.
- **Turn:** while they fail the rule does not match for it, except under `StoreHold` for a
  [pending round](round.md#the-store-waits-for-a-pending-round); once they hold, the rule queues it,
  steps it and [walks back](round.md#back-to-the-anchor), holding the turn.
- **Until done:** no round, cap or other private request; entries keep rounds, asks and marks.
- **Keeper-less towns:** none within `RoundReach`: no look, no round, the store as today. Teranu
  Mardui and Kurfa ([alliance keepers](round.md#keepers-by-town)) become so once each bot's first
  look of the process [marks](warehouse.md#keeper-marks) them. One open a session
  ([pacing](server.md#pacing)).

## The valuables value

The ledger, the round state (open or not, start time, **anchor**, ad lines said, and per partner:
replies, counters left, agreed price, asked for a trade) and the `noPrivate` marks are one
`valuables` value with `openRound(anchor)` and `endRound`. Each `Bot` owns its own and hands it to
its sessions like the chat log (`cfg.chat`; `session.go:212,349`; `chat.go:43-45`), so `RoundGap`,
`AnswerAfter`, reply counts and marks survive a reconnect, per bot. The anchor is the position at
`openRound`, read by the invite walk and every walk back. V2b adds the value, rule "valuables" as a
stepper matching only while a round is open (only tests call `openRound` before V7a), the [walk
back](round.md#back-to-the-anchor) (`anchorArrive`), and a stored-units accessor reporting none
stored and the status known until V3b wires it to V3a's private status.

A round open when its session ends is ended by the next session's first decision as a
[cut-off](round-end.md#cut-off): after its first line rounds step and asks drop at once, the cap
(V7b) running once that session's look is done; before it nothing changes. Its reply queue, partners
and trade request are dropped; nothing is deposited.

## Tests

- `TestValuablesLedger` (V2b): a carried valuable adds an entry (rounds 0, the first ask); two +0
  Swords of Watershadow are one entry of two units and two rows, a stackable valuable of 5 one entry
  of one row; +0 and +5 of a template are two entries; an `InventoryUpdate` removing a unit lowers
  the carried count; with none carried an entry is dropped with the status known and none stored,
  kept with it not known (the test sets the accessor); a found unit joins its entry with rounds and
  ask; one found after a drop starts a new entry; `openRound` keeps its anchor until `endRound`; the walk back sends legs to within
  `anchorArrive`, none once there. Stored units: `TestValuablesLedgerLook` and
  [`TestValuablesRoundDeposit`](round-end.md#tests) (look rows and a deposited row with `ObjectID`
  and `Enchant` count stored, a withdrawn one carried).
- `TestValuablesLedgerLook` (V3b; no round before V7a): a fresh session's first town visit looks
  (282 or a type 1 list) with nothing carried, once the open is due, and within `RoundGap`; not
  while a warehouse request, market visit, gear `UseItem`, crafter cycle or enchant exchange runs,
  out of a registry town, or with no private keeper in reach; then it walks back within
  `anchorArrive` and lets go. After a process start a list with a +0 Sword of Watershadow, a +5
  Conjuror's Staff, an augmented valuable template, a quest item and a template under `ValueMin`:
  the sword and staff get entries (rounds 0, the first ask, the staff's with five scrolls, not
  released, one stored unit), the others none. After a reconnect an entry with a stored unit is kept
  unchanged while the status is unknown, and the look finds the unit. Talking Island (Roxxy 30006,
  Rant 30054, Rolfe 30055; Rant's fixtures `30054.htm` with only `Chat 1`, `30054-1.htm` with only
  `_DepositF`/`_WithdrawF`): the look follows Rant's `Chat 1`, marks Rant, ends with no `retryAt`,
  and next decision, with no walk back between, takes Rolfe's type 1 list, then walks back; the next
  session goes straight to Rolfe. Teranu Mardui (Hagos 31381 with a link-less page, as
  `31381-no.htm`): Hagos marked, the walk back, no look that session or the next. A `follow` on the
  walk to the keeper ends the look (no `_WithdrawP`, no walk back, the next legs rule "follow"'s);
  it runs on the next town visit.
