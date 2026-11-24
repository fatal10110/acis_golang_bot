# Merchant valuables (M12)

Status: draft for owner review; proposed decisions D30-D34 open. The M12 epic and issues V1-V9 (V2,
V3 and V7 as a and b) are filed once the owner approves.

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

- **D30, Trade ads.** No channel reaches a non-hero's whole server (Trade and Shout: the restart
  region, `network/chat.go:108-122`; Hero Voice drops non-heroes, `:123-128`). Proposal: `merchant`
  advertises valuables on **Trade** only, under the caps of [ads](ads.md); never Shout. Amends the
  `docs/plan.md` section 4 bullet (lines 343-344): "never on Shout or Trade" gains this exception,
  "Chat only when answering a whisper" gains the ads, the only unprompted lines. Alternative: Trade
  and Shout under the same caps.
- **D31, what is valuable.** The [score](score.md): price × 2^R, R from the supply ceiling (Seven
  Signs catacomb and `_none` seal makers at half, seasonal makers not at all), 0 for non-gear
  spawned multisell products, halved for craftables, raised by enchant past safe; valuable at score
  100000 and price 10000. Refines "multisell items are never rare" (upgrade rows do not count) and
  "items sold in buylists are never rare" (only NPC id above 0, `cheapest`; the 212 `npcId="-1"`
  lists hold all four valuable rows, which read literally would be none). A valuable precedes the
  clan member's hold, so a member merchant sells a valuable recipe or graded gear instead of
  depositing it for the crafter. Alternative: the hold first, losing Conjuror's Staff, Elven Tunic
  (D grade) and Recipe: Halberd (Sword of Watershadow and Tears of Eva, with no grade, are in no
  hold either way).
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
  they take more than 6 rows or the asks of those asking at most 2000000 pass 2000000, and one
  unsold through 3 rounds (or asking over 2000000 alone, after its 3 rounds) goes alone; the cap
  runs before the deposit. Released valuables list at their [floor](whispers.md#prices) (at least
  reference, twice the buy-back): amends the `docs/plan.md` merchant default "price between 1.2 and
  1.5 times the NPC buy-back price" (line 321).
- **D34, budget.** A per-session `-valuables-budget`, default 2000000, apart from `-store-budget`;
  valuables are offered and listed while sales are under it, so one sale may pass it by its own
  price.

## Delivery

Milestone M12. Each PR is behavior-neutral for every preset until V8 enables `merchant`, except
V3a's [142 refusal](warehouse.md#refused-links), which changes today's clan errand too. The round is
split (V7a round, V7b cap and store) and so are V2 (read-only supply vs brain policy) and V3 (errand
vs the look that drives it), each one reviewable diff. No unit test waits on the wall clock; no e2e
scenario runs with `-race`.

| PR | Scope | Reads | Depends on |
| --- | --- | --- | --- |
| V1 | Datapack: NPC drops; template and spawnlist `<private>` rows (id, respawn; `spawnlist.go:92` skips them); each maker's `<ai type>`, `<set>` params (`EventName`, `IsNight`, `on_start_spawn`, `maker_name1`-`maker_name3`, `maker_cnt`) and `spawnTime` (`spawnlist.go:85-89` skips them); multisell rows; restart point tiles; normal enchant scroll ids by grade and kind (names need no work, `datapack/items.go:238,354`); fixture and shipped tests | [score](score.md), [ads](ads.md#region), [server](server.md) | M10 S2 (#321), which also extends `datapack/npc.go` |
| V2a | `TravelMap.supply` (`S`, marks) with maker shares by event, start and type, `NewTravelMap`'s `SupplyConfig` (`-spawn-events`, defaults); score-only `ValuablePolicy` fields (`PriceFloor`, `ValueMin`, `CommonSupply`, `RMax`, `CraftDivisor`, `EnchantRare`; disabled), `R`, enchant terms, `V`, called from tests only; the restart region; `TestValueScore`, `TestValuablesShipped`, `TestRestartRegion` | [score](score.md), [ads](ads.md#region), [preset](preset.md#process-settings) | V1 |
| V2b | The rest of `ValuablePolicy` (`Enabled`, `RoundReach`, defaults; disabled); classification and precedence in `junk`, `storeLoot`, `tradeGoods`, before the member's hold (`wireMember`, `memberKeep`); the `valuables` value (entries, derived units, rows, lifecycle, round state with anchor, `openRound`, `endRound`, the `noPrivate` set); rule "valuables" as a stepper only, the walk back (`anchorArrive`); `itemNames` for every scored template; labels and price shorthand; `TestValuableClassification`, `TestValuablesLedger`, `TestItemLabel` | [score](score.md#valuable-test) and [its tests](score.md#tests), [ledger](ledger.md), [ads](ads.md#lines), [round](round.md#back-to-the-anchor) | V2a |
| V3a | Private requests in the errand: `Private`, links on any page, list type, `privateWant` and `startWarehouse`'s guard and order, no clan, `noPrivate` (by template, `WarehouseUnreachable` with no `retryAt`, skipped by the keeper choice, a handed-in set), a 142 at the talk step failing with no mark (private and clan), look, priority; the private status (list, 282, results, `trackContents` with `ObjectID` and `Enchant`, `depositRows`, slots); `Anchor` and `Reach` and the keeper rule; `stepWarehouse`; `cancelPrivate`; `TestPrivateWarehouseErrand` | [warehouse](warehouse.md), [round-end](round-end.md#cut-off) | none |
| V3b | The session's look: its conditions (`storeReady`'s state checks, town, keeper, open and errand checks with the enchant exchange idle, member settled), request with anchor and `RoundReach`, walk back (after the next keeper's request on a new mark), the stored-units accessor wired, entries from rows (row test), the `noPrivate` set handed over; a called-away look cancelled, no walk back; `TestValuablesLedgerLook`. No round yet | [ledger](ledger.md), [round](round.md#back-to-the-anchor) | V2b, V3a |
| V4 | `AdBoard` (`Free`, atomic `Claim`), the ad composer over V2b's labels (entries not released), per-bot pacing, the ad step; `TestAdText`, `TestAdPacing` | [ads](ads.md) | V2b |
| V5 | The responder: intent and price parsing, the naming outcomes, asks, floors, counters, delay, per-partner queue sent by rule "chat", caps apart from `chatLog`, precedence over the canned reply, after-round replies (`in my store` with V7b); `TestValuablesWhisperHaggle` | [whispers](whispers.md) | V2b |
| V6 | Valuables in `acceptTrade`, `tradeGoods` (never released), `tradePrice`, `tradePaid`; the invite step (walk within `RoundReach` of the anchor, `TradeRequest`, refusals, none within 15 s of the end, walk back); trade sales in the valuables budget and its offer rule; `TestValuablesTradeInvite`, `TestValuablesTradeConfirm` | [trades](trades.md), [release](release.md#valuables-budget-d34) | V2b, V4, V5 |
| V7a | `roundReady` (V3b's look conditions plus region and board) opening rounds at the bot's position; the start condition; the pending round and `StoreHold`; no round before the look; requests with anchor and `RoundReach`; walks back; the left-over round; withdraw, end, `FirstLineWait`, `Grace`; `called()` with `cancelPrivate` or the `whSent` wait; the round step; the deposit (skipped when not due, held for a pending request); the store held for the bot's request; the off-round deposit; `valuableFees`; `TestValuablesRoundStart`, `TestValuablesStoreHold`, `TestValuablesFirstLineWait`, `TestValuablesHoldsTurn`, `TestValuablesRoundDeposit`, `TestValuablesGrace`, `TestValuablesReadvertise`. Releases nothing | [round](round.md), [round-end](round-end.md), [ledger](ledger.md#every-session-looks-first), [warehouse](warehouse.md#fees-and-slots) | V3b, V4, V6 |
| V7b | The cap (before the deposit; after a cut-off past the first line), the release withdrawal off-round and in the round's withdrawal (budget allowing) and its pending hold, `storeReady`/`storeSells`, released rows first in `storeRows` by the listing rule, the store sales split, released-entry replies; `TestValuablesRelease`, `TestValuablesStoreSplit` | [release](release.md), [round](round.md#the-store-waits-for-a-pending-round), [whispers](whispers.md#after-a-round) | V7a |
| V8 | `merchant` enables the policy, shared ad board, `-valuables-budget`, ad board flags, metrics, panel | [preset](preset.md) | V7a, V7b, M10 S9 (#331), which also edits the preset catalog |
| V9 | `TestLiveValuablesTradeSale`, `TestLiveValuablesWarehouse`, `TestLiveValuablesRelease`; the unmarked buyer and its helper, the Human Mystic merchant; `EncodeAnswerTradeRequest`; `docs/e2e-live.md` rows | [preset](preset.md#live-scenarios-v9) | V8 |

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
