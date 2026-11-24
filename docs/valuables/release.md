# Cap, release and the valuables budget (D33, D34)

## The cap

The cap runs only at the end of a round that advertised ([round-end](round-end.md#steps), step 5,
before the deposit; also at a cut-off after the first line; never after `FirstLineWait` or a cut-off
before the first line), so every entry gets a round with players before its release; a unit found
later joins its entry, released or not ([ledger](ledger.md#entries-and-units)). Counting carried and
stored derived units, the bot **releases** to its store:

- all entries, when they take more than `HoldRows` (6) [rows](ledger.md#rows);
- all entries that each ask at most `HoldValue` (2000000), when their asks times units pass
  `HoldValue`;
- one entry unsold through `MaxRounds` (3) rounds, alone.

An entry whose ask alone passes `HoldValue` never counts toward the sum: it gets its rounds and goes
alone after `MaxRounds`. Example: Scythe (96), 2290000 (`items/0000-0099.xml`, item 96), a DROP of
monsters and raid bosses of level 38 to 46 (e.g. NPC 25394, level 38; 20235, level 45;
`npcs/*.xml`), as are Cursed Dagger (226) and Orcish Glaive (298) at the same price.

**What bounds the stored rows.** `HoldRows` holds only right after a round that advertised (the
unreleased ones and that deposit take at most `HoldRows` rows). Between such rounds rows grow: a
`FirstLineWait` round deposits uncapped; with the budget spent, later finds go in by the off-round
deposit uncapped, and released stored rows stay. The next advertising round withdraws and caps them
all. So nothing rests on `HoldRows` as a hard bound: `valuableFees` follows the carried rows and
`depositRows` checks free slots ([warehouse](warehouse.md#fees-and-slots)).

## Release to the store

A released valuable carried at the round's end is not deposited; one stored is withdrawn, budget
allowing, by the next round's withdrawal or the [off-round release
withdrawal](round-end.md#off-round-errands); with the budget spent it stays stored. It needs no
round, and the opening lists it ahead of the loot at its [floor](whispers.md#prices). Three store
changes, all V7b:

- `storeReady`'s store budget gate (`store.go:358`) holds back only loot: an opening also starts for
  a released valuable the valuables budget allows; `storeSells` accepts one.
- `storeRows` lists released valuables first at their floor by the listing rule below, not by `left
  := p.Budget - st.sold` and `n := min(count, left/price, room/price)` (`store.go:466,493`),
  which never lists a valuable priced above the budget left; then loot against the store budget,
  within `MaxRows` (4, `store.go:69`) and the server's int32 room.
- At `FloorPct` 100 the floor is at least the reference price, twice the buy-back, outside
  `StorePolicy`'s band (`MaxMarkup` under 200, `store.go:50-53`); D33 names the amendment.

The registry's town limit and spacing apply (a full town keeps them carried). They stay listed until
sold.

## Valuables budget (D34)

Valuable sales, by trade and store, count in a per-session `Valuables.Budget` (flag
`-valuables-budget`, default 2000000), not `-store-budget` (50000), under which one valuable would
never list. With `left = Budget - sold` (`sold` the session's valuable sales):

- **Trade:** a valuable is offered while `left > 0`, at most `ceil(left / unit price)` units.
- **Store listing:** released rows by score while `left > 0`, a row's units at most `ceil(left /
  floor)`, the listed value then taken off `left`. Example: `left` 1000000 and three one-item rows
  at 600000: the first two list, the third waits.

Every unit starts under `Budget`, so the Scythe still sells once a session and sales stay under
`Budget` plus one unit's price. Once `sold` reaches `Budget`, nothing is offered or listed for the
session: the carried valuables, released or not, are deposited by the [off-round
deposit](round-end.md#off-round-errands), and no released stored one is withdrawn.

**Counting.** A trade counts its price on `SendTradeDone` with success. The store counts one total
today (`countSold` adds the adena gained since the list to `store.sold` at the close,
`store.go:547-556`). V7b keeps each listed valuable row's object, count and price; at the close the
units a row lost (listed less still carried) times its price go to the valuables budget, the rest to
the store's. Sold units leave the inventory, hence the entry, with no ledger step.

## Tests

- `TestValuablesRelease` (V7b): at a round's end a seventh valuable, asks past `HoldValue`, or a
  third unsold round release them: the stored ones withdrawn and listed first in
  `SetPrivateStoreListSell` at their floor, no Trade line; a released carried one is in no
  `DepositP` row and pays no fee, and all released means no deposit; a +5 lists at reference plus
  five scrolls; a `FirstLineWait` round releases nothing; a `follow` cut-off after the first line
  runs the cap (a third round so cut releases), before it none; nothing before a first round; one valuable above `HoldValue` releases nothing else and goes alone after
  its third round; with the store budget spent an opening still lists the released valuables only;
  `Budget - sold` 1000000 and three rows at 600000: two list; a row above `Budget - sold` lists
  while `sold < Budget`, none once reached; a trade-sold valuable counts in no cap; with the budget
  spent no released stored one is withdrawn and the off-round deposit takes released carried ones. A round with a released stored
  and a released carried valuable: `_WithdrawP` takes the stored one while the budget allows (not
  when spent), no line names either, a whisper naming one gets no haggle answer, `tradeGoods` offers
  neither, their rounds do not step. Dion: after a cap released a carried valuable,
  `RequestPrivateStoreManageSell` at the anchor, not at 30058, listing it first, then the loot.
  After a process start the look's +0 Sword of Watershadow and +5 Conjuror's Staff count in the next
  cap; a round left open by an ended session and cut after its first line caps only once the next
  look is done. A look finding a released stored valuable (with loot): no
  `RequestPrivateStoreManageSell` and no move for the 60 s until the open, then the off-round
  `_WithdrawP`, the walk back, and the opening lists it first, then the loot; with a backoff past
  `StoreHold`, the store opens at `StoreHold` with the loot only.
- `TestValuablesStoreSplit` (V7b): a store with one valuable and two loot rows closing with the
  valuable and one loot row sold: the valuable's price in the valuables budget, the rest in the
  store budget; the sold entry leaves the ledger and cap. With the store open, a whisper within
  `AnswerAfter` naming a released listed valuable gets `<name> in my store <price>`; naming a
  released one stored, or carried with no store open, `<name> not now, wts again soon`.
