# Merchant valuables (M12)

Status: draft for owner review; proposed decisions D30-D34 open. The M12 epic and issues V1-V9 (V2,
V3 as a and b, V7 as a to c) are filed once the owner approves.

## Problem and goal

A player who loots something rare shouts "WTS" on Trade, answers whispers, haggles, trades, parks it
in the warehouse when nobody bites and tries again with the next find, and dumps it in a store only
when too much piles up. The merchant bots sell a Sword of Watershadow like a stack of stems. The
owner (2026-11-24): "before selling some valuable thing on the town store, try to sell it to a
player: open a trade, send a global message, and wait for some time, then put it in the warehouse;
when next time there is another valuable item, take from the warehouse and try to sell both; when
too much valuable is collected, just sell on the town store", and "valuable should be marked not
only by price, but also by how rare the item is".

Goal: a merchant that loots a **valuable** (worth much for its price and scarcity) keeps it out of
the NPC sale and its store; in town, before its store, advertises it on Trade, answers whispers with
a price, haggles to a floor, invites a whisperer in sight and sells by trade; deposits the unsold in
its private warehouse; with a later new valuable withdraws and advertises them all; and lists
everything in its store once too much piles up. Out of scope: buying from players, multi-item trade
windows (one valuable per trade), the clan warehouse for valuables, presets other than `merchant`.

## Approach and files

- [server](server.md): server and datapack facts, the M8 prices, the bot today, the pacing table.
- [score](score.md): reference price × 2^rarity, rarity from a server-wide supply ceiling read from
  drops, spawns, makers and privates; shop, multisell, craft and enchant marks; the clan hold order.
- [ledger](ledger.md): entries per template and enchant with derived units; each session's look.
- [round](round.md), [round-end](round-end.md): rule "valuables" holds the turn in a registry town:
  withdraw, advertise, answer, invite, count, cap, deposit, walk back, then the store.
- [ads](ads.md), [whispers](whispers.md), [trades](trades.md): Trade ads under a process-wide ad
  board, a haggling whisper responder, trades at the haggled price and invites.
- [warehouse](warehouse.md): private requests in the existing errand.
- [release](release.md): the cap sends valuables to the store at their floor; their own budget.
- [preset](preset.md): wiring, defaults, process settings, live scenarios.

## Owner decisions

Proposed, awaiting the owner.

- **D30, Trade ads.** No channel a non-hero speaks on reaches the whole server, only the speaker's
  restart region ([server](server.md#server)). Proposal: `merchant`
  advertises valuables on **Trade** only, under the caps of [ads](ads.md); never Shout. Amends the
  `docs/plan.md` section 4 bullet (lines 343-344): "never on Shout or Trade" gains this exception,
  "Chat only when answering a whisper" gains the ads, the only unprompted lines. Alternative: Trade
  and Shout under the same caps.
- **D31, what is valuable.** The [score](score.md): price × 2^R, R from the supply ceiling (Seven
  Signs catacomb and `_none` seal makers at half, seasonal makers not at all), 0 from a spawned
  NPC's multisell row with no weapon or armor ingredient, halved for craftables, raised by enchant
  past safe; valuable at score 100000 and price 10000. Refines "multisell items are never rare"
  (upgrade rows do not count) and "items sold in buylists are never rare" (only NPC id above 0,
  `cheapest`; the 212 `npcId="-1"` lists hold all four valuable rows, which read literally would be
  none). A valuable precedes the clan member's hold, so a member merchant sells a valuable recipe or
  graded gear instead of depositing it for the crafter. Alternative: the hold first, losing
  Conjuror's Staff and Elven Tunic (D grade) ([score](score.md#valuable-test)).
- **D32, pricing and selling.** Ask 100-150 % of reference by rarity plus the enchant scrolls, 10
  points lower per unsold round; floor the worth (reference plus scrolls) or the store price,
  whichever is higher; two counters per buyer and item a round; nothing below the floor. The bot may
  invite a whisperer in sight (one request per buyer a round). Amends `docs/plan.md` line 328
  ("trades are fixed-price sales...": a valuable is haggled between ask and floor), line 332
  ("rate-limit trade requests per partner", today only incoming, `trade.go:359`: the bot may send
  one per partner a round, one open at a time), and section 4 lines 343-344 ("rate-limited per
  sender and capped per bot"): whisper replies in a round and within `AnswerAfter` (30 min) after it
  (no offer, no trade then), under [their own caps](whispers.md#pacing-queue-and-caps) instead of
  the canned ones, never to a marked name.
- **D33, hold, warehouse, cap.** A 5 min round of ads, then the private warehouse (30 adena a row);
  a new valuable brings the stored out for another round; after a round, all go to the store once
  they take more than 6 rows, those each asking at most 2000000 once their asks together pass
  2000000, and one unsold through 3 rounds alone (the only way for one asking more); the cap
  runs before the deposit. Released valuables list at their [floor](whispers.md#prices) (at least
  reference, twice the buy-back): amends the `docs/plan.md` merchant default "price between 1.2 and
  1.5 times the NPC buy-back price" (line 321).
- **D34, budget.** A per-session `-valuables-budget`, default 2000000, apart from `-store-budget`;
  valuables are offered and listed while sales are under it, so one sale may pass it by its own
  price.

## Delivery

Milestone M12. Each PR is behavior-neutral for every preset until V8 enables `merchant`, except
V3a's [142 refusal](warehouse.md#refused-links), which changes today's clan errand too. The round is
split (V7a round and deposit, V7b store hold and off-round deposit, V7c cap and store), and so are
V2 (read-only supply vs brain policy) and V3 (errand vs the look that drives it), each one
reviewable diff. No unit test waits on the wall clock; no e2e scenario runs with `-race`.

| PR | Scope | Reads | Depends on |
| --- | --- | --- | --- |
| V1 | Datapack readers and fixtures of the [score tests](score.md#tests) (V1 bullet), with `Names` beside `Prices` ([ads](ads.md#lines)); shipped tests | [score](score.md), [ads](ads.md#region), [server](server.md) | M10 S2 (#321), which also extends `datapack/npc.go` |
| V2a | `TravelMap.supply` and its `SupplyConfig` ([preset](preset.md#process-settings)); the score-only `ValuablePolicy` fields (`PriceFloor`, `ValueMin`, `CommonSupply`, `RMax`, `CraftDivisor`, `EnchantRare`; disabled), called from tests only; the restart region; `TestValueScore`, `TestValuablesShipped`, `TestRestartRegion` | [score](score.md), [ads](ads.md#region), [preset](preset.md#process-settings) | V1 |
| V2b | The rest of `ValuablePolicy` (disabled); the [valuable test](score.md#valuable-test) and its precedence; the [`valuables` value](ledger.md#the-valuables-value) with rule "valuables" as a stepper and the walk back; `itemNames` and labels; `TestValuableClassification`, `TestValuablesLedger`, `TestItemLabel` | [score](score.md#valuable-test) and [its tests](score.md#tests), [ledger](ledger.md), [ads](ads.md#lines), [whispers](whispers.md#prices), [round](round.md#back-to-the-anchor) | V2a |
| V3a | All of [warehouse](warehouse.md); `stepWarehouse` ([round](round.md#the-rule)); `cancelPrivate` for the [cut-off](round-end.md#cut-off); `TestPrivateWarehouseErrand` | [warehouse](warehouse.md), [round-end](round-end.md#cut-off) | none |
| V3b | The session's [look](ledger.md#every-session-looks-first) (conditions, request, walk back, cancel), entries from rows, the stored-units accessor and `noPrivate` set wired; `TestValuablesLedgerLook`. No round yet | [ledger](ledger.md), [whispers](whispers.md#prices), [round](round.md#start) (the conditions) and [its walk back](round.md#back-to-the-anchor), [cut-off](round-end.md#cut-off), [keeper marks](warehouse.md#keeper-marks) | V2b, V3a |
| V4 | [ads](ads.md): `AdBoard`, the composer over V2b's labels, pacing, the ad step; `TestAdText`, `TestAdPacing` | [ads](ads.md) | V2b |
| V5 | [whispers](whispers.md) (after-round `in my store` with V7c); `TestValuablesWhisperHaggle` | [whispers](whispers.md), [ledger](ledger.md#the-valuables-value), [ads](ads.md#lines) | V2b |
| V6 | [trades](trades.md); trade sales in the valuables budget and its offer rule; `TestValuablesTradeInvite`, `TestValuablesTradeConfirm` | [trades](trades.md), [release](release.md#valuables-budget-d34) | V2b, V4, V5 |
| V7a | The [start](round.md#start) and [round-end](round-end.md) steps 1-5 with the [cut-off](round-end.md#cut-off), without store hold, off-round errands, cap or release; `valuableFees`; `TestValuablesRoundStart`, `TestValuablesFirstLineWait`, `TestValuablesHoldsTurn`, `TestValuablesRoundDeposit`, `TestValuablesGrace`, `TestValuablesReadvertise` | [round](round.md) but its [store hold](round.md#the-store-waits-for-a-pending-round), [round-end](round-end.md) but its [off-round errands](round-end.md#off-round-errands), [ledger](ledger.md#every-session-looks-first), [warehouse](warehouse.md#fees-and-slots), [whispers](whispers.md#prices) and [after a round](whispers.md#after-a-round), [trades](trades.md#asking-for-a-trade) | V3b, V4, V6 |
| V7b | The [pending round and `StoreHold`](round.md#the-store-waits-for-a-pending-round), step 6's wait for the bot's request, the budget-spent [off-round deposit](round-end.md#off-round-errands); `TestValuablesStoreHold`, `TestValuablesOffRound` | [round](round.md#the-store-waits-for-a-pending-round), [round-end](round-end.md#off-round-errands) | V7a |
| V7c | All of [release](release.md): the cap, the release withdrawal (step 1 and off-round) with its pending hold, the store changes; released-entry replies; `TestValuablesRelease`, `TestValuablesStoreSplit` | [release](release.md), [round](round.md#the-store-waits-for-a-pending-round), [round-end](round-end.md) (step 1, [off-round errands](round-end.md#off-round-errands)), [whispers](whispers.md#after-a-round), [ledger](ledger.md#released-entries) | V7b |
| V8 | [Wiring](preset.md#wiring): `merchant` enables the policy, shared ad board, `-valuables-budget`, ad board flags, metrics and panel view; `TestValuablesMetrics` | [preset](preset.md) | V7a, V7b, V7c, M10 S9 (#331), which also edits the preset catalog |
| V9 | The [live scenarios](preset.md#live-scenarios-v9), their buyer and merchant | [preset](preset.md#live-scenarios-v9) | V8 |

## Risks

- **Spam:** ads are the one place bots speak to a region; at most one bot line a minute per region,
  Trade only, one preset (D30).
- **Supply overstated:** no server kills every spawn at respawn, and the share differs by item. The
  maker rules remove the largest differences (without them 11 templates priced 10000 or more crossed
  `ValueMin` the wrong way); left approximate: unequal totals in `random_spawn`, the even spread
  over linked sets, the 43 default-like types and the `maximumNpcs` caps (read, `Maker.MaximumNPCs`,
  `datapack/spawnlist.go:31-33`, not applied). `ValueMin` and `CommonSupply` absorb the scale and are persona defaults.
- **Unknown sources:** quest rewards and event drops count as common; price decides.
- **Haggling text:** misreads are bounded ([whispers](whispers.md)): unclear lines get the price or
  the asks, an offer under half the floor draws no counter, and nothing sells below the floor.
- **Process restart:** the look recovers stored items, but rounds restart at 0 and release marks are
  lost (up to three more rounds); a row failing the row test (raised `ValueMin` or `PriceFloor`, now
  sold by a counted buylist) stays in the warehouse for good, few since nothing else goes there.
- **Time in town:** about 9 minutes a round plus two keeper visits and walks back (Dion 5348 each
  way), up to `StoreHold` with the store held, a round per 30 min at most; backoffs never stretch a
  round. The merchant farms less, which suits it.
- **Store before a valuable:** within `RoundGap` of the last round, or past `StoreHold`, the store
  opens first; the valuable waits for it to close (sold out, teleport, `follow`), late, never sold
  as loot.
- **Gaming the bot:** a buyer pays at least the floor (store price, worth), by haggle or in the
  store; the trade confirms only on the partner's adena (`tradePaid`).

## Gaps

- One valuable per trade window, so a buyer of two trades twice (a multi-item `tradeGoods` offer): a follow-up issue in M12 once the
  owner approves.
- The ledger lives in memory, not the bot registry: a follow-up in M12 if the owner wants it kept
  across process restarts.
- No server gap.
