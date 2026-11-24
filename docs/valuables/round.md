# The round: start and store hold

## The rule

Rule "valuables" (persona `Valuables` enabled) sits right after "loot", ahead of "store"
(`rules.go:474-514`; today `rules.go:491-492`); combat, prompts, the trade window (rule 2) and earlier
rules keep priority. During a round it matches on every decision, also while waiting, so no later
rule walks the bot away. Rule "warehouse" sits after the errands (`rules.go:493-500`) and steps its
errand only when reached (`warehouse.go:402-406`), so the round queues its request in `privateWant`
([warehouse](warehouse.md#requests)), starts it with `startWarehouse` and steps it with
`stepWarehouse`, which V3a splits out of `ruleWarehouse` (`warehouse.go:407-433`); unlike the
crafter, which waits for rule "warehouse" (`crafter.go:279-281`). An open store runs `storeRules`
(`rules.go:518-524`), with no "valuables", and the server refuses trades in store mode
(`network/trade.go:69-75`): no round with the store open or opening.

## Start

`roundReady` takes `storeReady`'s state checks (`store.go:356-379`: registry and travel map, alive,
out of combat and PvP flag, not casting, still, no trade prompt or window, no shop, trip, newbie,
class, dyes or train visit, not called), not its loot check or store budget gate (`store.go:358`;
the [valuables budget](release.md#valuables-budget-d34) applies), and adds, since `storeReady` has no
town check (a store's is `hasRoom` in `startStore`, `store.go:336`):

1. **Registry town:** `town` (nearest town point within `storeTownRadius`, 3000,
   `storeregistry.go:49-53`) finds one; no free place needed. A town point is the first gatekeeper
   spawn of a cluster in id order (`TravelMap.Towns`, `storeregistry.go:114-131`,
   `travel.go:158-180`), not a centre.
2. **Private keeper:** a keeper spawn (`keepers`, `travel.go:152-156`) not marked `noPrivate` within
   `RoundReach` (6000) ([keepers by town](#keepers-by-town)).
3. **Region** ([ads](ads.md#region)) not -1, and the **ad board** `Free` there
   ([ads](ads.md#ad-board)).
4. **No errand** `storeReady` misses (`store.go:367`): warehouse request (`wh.running`), market visit
   (`market.active()`), gear `UseItem` (`gear.obj`), crafter cycle (`craft.phase`), enchant exchange
   (`ench.step` not `enchantIdle`), unsettled clan visit (`member.asked`), store opening or open.
   Rule "enchant" (`rules.go:501`) runs in town (`enchantFree`, `enchant.go:274-286`) over
   `enchantUsed`, `enchantChosen`, `enchantRequested`, `enchantSettling` (`enchant.go:222-259`); a
   round between would keep the scroll selected 9 minutes (logout refused, `enchant.go:232-238`), and
   a private request cancels it (server `network/warehouse.go:291`, `cancelActiveEnchant`). Rule
   "member" (`rules.go:502`) settles a clan visit when reached (`clanmember.go:422-462`: rows off the
   board, lease back, ore shot orders posted, fees reset), at the very arrival a round would start.
5. **Open due:** `openDue(now)` and `retryAt` (`warehouse.go:588-590`). Clan and private requests
   share one gap: 60 s (`warehouseOpenGap`) after the last open, doubled per unanswered open or
   request in a row (`min(timeouts, 5)`, at most `warehouseBackoffMax` 30 min,
   `warehouse.go:22-23,302-311`); a failed request sets `retryAt` 60 s on (`warehouse.go:351-352`).
   Not due, a request is refused or holds the turn in `warehouseLink` (`warehouse.go:444-446`) up to
   30 min. Checked with nothing to withdraw too, for the deposit's sake.

A round starts when `roundReady` holds, the [look](ledger.md#every-session-looks-first) is done, an
entry has carried units and is not released, `RoundGap` (30 min) has passed since the last start,
and the budget is not spent. Leftovers (a `called()` cut-off, a failed or skipped deposit) start the
next round; stored valuables alone start none, as the owner asked. The position at the start is the
**anchor**; every walk of the round ends within `RoundReach` of it.

### Keepers by town

From the town point (shipped datapack, travel map rules); Dion's 5348 is the farthest nearest private
keeper of any point with one within 6000, hence `RoundReach` 6000. Where no round can start, the
store opens as today, the valuables carried.

| Case | Town point | Keepers |
| --- | --- | --- |
| Private keeper past 3000 | Dion, Trisha 30059 (`spawnlist/20_22.xml:620`) | 30058 (`:551`) at 5348; 5398 from gatekeeper 31219 (`:756`, 53 from Trisha, the same point) |
| | Gludin, Richlin 30320 (`spawnlist/17_22.xml:462`) | 30210 (`:291`) at 3928 |
| | Dark Elven Village, Jasmine 30134 (`spawnlist/20_18.xml:441`) | 30139 (`:364`) at 4165 |
| | Arisha 31698 (`spawnlist/21_16.xml:819`) | 31313 (`:89`) at 5115 |
| Freightman nearest: each bot's first private request of the process marks it, the next goes on | Talking Island, Roxxy 30006 | Rant 30054 at 2317, Rolfe 30055 at 2508 (`spawnlist/17_25.xml:197,198,200`) |
| | Giran, Clarissa 30080 | Parman 30104 at 1176, Valkon 30103 at 1309 |
| | Elven Village, Mirabel 30146 | Chad 30151 at 1641, Julia 30152 at 1664 |
| | Esmeralda 30233 | Silva 30686 at 1735, Sorint 30232 at 1870 |
| | Bella 30256 | Ballin 30322 at 1946, Haprock 30255 at 2208 |
| | Tamil 30576 | Imantu 30563 at 2328, Grookin 30562 at 2495 |
| | Flauen 30899 | Kluck 30895 at 1889, Mia 30896 at 1945 |
| Alliance keeper only: keeper-less once the first look [marks](warehouse.md#keeper-marks) it (one visit per bot and process) | Teranu Mardui 31383 (`spawnlist/23_16.xml:850`) | Hagos 31381 (`:826`) at 3326; Hakon 31268 (`spawnlist/24_16.xml:68`) 39103 away |
| | Kurfa 31376 (`spawnlist/24_15.xml:1099`) | Jaff 31374 (`:1123`) at 1007; Lietta 31267 (`spawnlist/24_16.xml:67`) 22595 away |
| No keeper | Minerva 30836 (`spawnlist/23_21.xml:119`), Angelina 30878 (`spawnlist/21_23.xml:146`) | none within 30000 |

## Back to the anchor

A keeper visit ends at the keeper (`warehouse.go:420-433`), a buyer walk at the buyer; at Dion's
keeper `town` (`storeregistry.go:49-56`) finds no town, so the next round, `startStore`'s
`hasRoom(at)` (`store.go:332-336`) and the ad region would fail. So the rule, holding the turn:

- after each keeper visit of its own (look, withdrawal, deposit, off-round errand), whatever the
  result, and after a buyer walk once the trade, refusal or expiry is over, walks back to within
  `anchorArrive` (100, a constant from V2b) of the anchor (`goTo` with `walkPlace`, as
  `supplytrip.go:205`); a request ended on a new `noPrivate` mark is queued again at once from where
  the bot stands, the walk back following that one (or at once with no unmarked keeper left);
- holds its next line, invite and end for the walk: the first line goes out at the anchor
  (`FirstLineWait` counts from the walk back after the withdrawal) and the store opens there, where
  `town`, region and `hasRoom` decide as with no round (a place claimed meanwhile within the 150
  spacing fails as today);
- on an unreachable answer stays: after the withdrawal or a buyer walk it ends the round like a
  [cut-off](round-end.md#cut-off), otherwise it lets go.

## The store waits for a pending round

`startStore` opens wherever the bot stands (`store.go:329-353`), and the store stays until sold out, a
teleport or a `follow` (`store.go:310-314`), declining trades (`trade.go:348-351`). So a round is
**pending** when all start conditions hold but the **short waits** (board, open, errand checks, look
not done). A **release withdrawal** (V7b) is pending when the look is done, released stored
valuables are held, the budget is not spent and a registry town has a private keeper in reach (short
waits: open, errands); common after a session whose budget ran out, since after the look the next
open is 60 s away (`openDue` from `openedAt`, `warehouse.go:302-311`; `endTransaction` exempts only a
deposit queued with a withdrawal, `warehouse.go:330-341`). `StoreHold` covers that gap: the
withdrawal runs, the bot walks back and the opening lists the released valuables ahead of the loot;
only a longer wait (a backoff) lets the store open first, the stored ones waiting for the next.

- While one is pending, `storeReady` returns false for at most `StoreHold` (2 min) from the first
  decision it held the store in this town (reset when `town` finds none or another; during the rule's
  own visit or walk back the anchor's town counts). A running look holds the turn and does not count.
- Pending on timers only (board, open), the rule holds the turn with a wait while the count runs,
  never past `StoreHold` (a board can hold an hour, a backoff 30 min). While an errand runs the rule
  does not match and the errand's rule steps it to its end.
- After `StoreHold`: the store opens with its loot (and released valuables carried) or the bot goes
  on with errands or farming; the valuables wait for the store to close or `roundReady` in town (the
  count restarts only in another town or after leaving).
- Within `RoundGap` of the last start nothing is pending: the store opens ([Risks](README.md#risks)).

## Tests

- `TestValuablesRoundStart` (V7a): with the look done, arriving with a new valuable sends the first
  Trade line (Say2 8) and no `RequestPrivateStoreManageSell`. None: within `RoundGap`; store open or
  opening; out of a registry town (within `storeTownRadius` of a full town still starts); region -1;
  board held (a line within `RegionGap`, or the hour cap); a warehouse request, market visit, gear
  `UseItem`, craft cycle or enchant exchange running (at `enchantChosen` the `RequestEnchantItem` and
  its result come first, no `_WithdrawP`/`_DepositP` meanwhile); a clan member with `member.asked`
  until rule "member" settles it (rows off, lease back, orders posted); the open not due (60 s after
  a clan open, backoff, `retryAt`), then once due. A leftover starts the next round `RoundGap` later;
  stored ones alone none; no line names a released carried one. Dion: from Trisha's spawn the look
  goes to 30058 (5348) and walks back within `anchorArrive` before the round starts; withdrawal at
  30058, first line after the walk back; the end's deposit at 30058, the walk back, then
  `RequestPrivateStoreManageSell` at the anchor with the loot; a buyer walk is followed by the walk
  back before the next line; an unreachable walk back after the withdrawal ends the round with no ad
  or deposit. No private keeper in reach: no round, the store with the loot. Talking Island with Rant
  marked: withdrawal and deposit at Rolfe (30055). Teranu Mardui with Hagos marked: nothing pending,
  the store at once. Sessions: none before the look; after a process start the stored Sword of
  Watershadow and +5 Conjuror's Staff start none, and a new carried valuable starts a round whose
  `_WithdrawP` takes both and whose first line names all three (in a variant the gear errand wears
  the staff next and it leaves the ledger); after a reconnect no round or deposit before the look,
  and a new valuable is deposited after its round and counts stored; a round open at a session's end
  is ended by the next as a cut-off, nothing carried over. Cap cases are V7b's.
- `TestValuablesStoreHold` (V7a): pending on another bot's board line: no
  `RequestPrivateStoreManageSell`, no move; freed within `StoreHold`: the round starts, no store until
  it ends; held past it: the store with the loot only; the same for a backoff; a running market visit
  steps to its end, no opening; within `RoundGap` of the last start: the store at once; leaving and
  returning restarts the count; a running look does not count; no loot and the board held past
  `StoreHold`: the hold stops, a due supply trip starts; a 30 min backoff holds the turn `StoreHold`
  only; look not done and board held: the look starts once the open is due, its wait counting; in
  Dion the look's visit does not reset the count.
