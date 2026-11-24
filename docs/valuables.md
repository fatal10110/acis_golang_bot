# Merchant valuables: offer rare loot to players before the town store

Status: draft for owner review. The review-fix loop ran 11 rounds; round 11's fixes are not
re-reviewed. Proposed decisions D30-D34 are open (see "Owner decisions").
Milestone M12 ("Merchant valuables"): the epic and the V1-V9 issues (V2 as V2a and V2b, V3 as V3a and V3b, V7 as V7a and V7b) are filed once
the owner approves.

Review history (one line per round: what the round found, and that each finding was checked
against the datapack, the server or the bot code before it was fixed):

- Round 1 (independent reviewer, 10 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): the round now holds the turn and steps the private
  warehouse errand itself, since rule "warehouse" sits after the errands (`rules.go:461-498`), and
  it checks the warehouse, market, gear and crafter errands that `storeReady` does not
  (section 3); the store budget gate no longer blocks released valuables, and the per-row split of
  store sales is named as new V7 work (sections 7, 8); release waits for a valuable's first round
  and a valuable priced above `HoldValue` or `Budget` can still sell (Scythe, sections 7, 8, D33,
  D34); Seven Signs makers count at half (`ssq_event`, the `_none` seal groups) and the
  `SpawnEvents` list fully, with the section 1 figures recomputed; the round state moves to V2 so
  V5 and V6 can test it; the normal enchant scroll prices a level; keeper pages and the `noClan`
  mark; the plan.md amendments of D32 and D33; the reserve term, the known ledger in
  `TestValuablesRoundStart` and the lowered `RoundGap` of the warehouse scenario; the Shout row,
  the buylist scope of `cheapest` and the full list of trade request refusals.
- Round 2 (independent reviewer, 10 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): private requests get a status of their own apart from the
  clan view the clan member reads, with the private slot count (100/120), and an in-town keeper
  rule named in V3 (section 6); D30 and D32 name the plan.md chat amendments (ads are unprompted;
  per-round reply caps outside `chatLog`), and the responder states its 3-12 s delay and a
  per-partner reply queue so a second buyer's whisper is not dropped (section 5); the supply
  formula reads the server's respawn rules and the `<private>` rows (V1 reads them): checking
  this finding against the server also showed that `respawnRand` is an even draw around
  `respawn` and that `respawn="0sec"`, like `no`, never respawns under the default maker, so the
  cycle drops `respawnRand / 2`, both count once per `NoRespawnCycle`, and the six-row table is
  recomputed (no verdict changes); the valuables budget's listing rule is stated (section 8);
  `roundReady` checks the town (not in `storeReady`), region -1 and the ad board, and
  `FirstLineWait` bounds a round (section 3); the `network/serverpackets` paths; the label and
  price shorthand move to V2 for V5, and the reply wording allows `WTS`; the short-confirm
  cancel and the invite walk bounded to the town.
- Round 3 (independent reviewer, 9 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): the clan member's keep hook no longer takes valuables: the
  precedence is the bot's own keeps and the persona's own hook, then the gear errand, then valuable,
  then the member's hold, with `wireMember` keeping the persona's own hook and `memberKeep` leaving
  valuables, which also takes them out of `memberDeposit`, `memberHandOff` and the member's fees
  (section 1, D31, V2 and its test); a private counts `total` times, once per leader the server
  places (`CreatePrivates`), and an entry with no `<privates>` rows takes its template's
  (`privatesOf`): checking this finding against the server found the template fallback too
  (175 spawned entries, 746 rows); the section 1 figures were recomputed with both and do not
  change, and `TestValueScore` pins both; the floor includes the enchant scrolls (`W × FloorPct /
  100`), so neither a haggle nor a release sells an enchanted item at the bare reference price
  (section 5, 7, D32, D33, tests); a round's end runs the cap before the deposit and keeps
  released carried items out of it, and a round that ended at `FirstLineWait` runs no cap
  (sections 3, 7); V7 splits now into V7a (the round) and V7b (cap, release, store gate, sales
  split), V8 depending on both; the keeper page counts (19 first pages, 11 second); D30 says no
  channel reaches the whole server for a non-hero (Hero Voice row added); D32 quotes the plan.md
  trade bullets it amends; `TestValuablesTradeConfirm` runs on a round opened with `openRound`.
- Round 4 (independent reviewer, 7 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): Sword of Watershadow and Tears of Eva have no grade, so the
  member's hold never takes them; section 1 and D31 now say two of the four valuable rows (184,
  433) are D grade, an item with no grade has no enchant scroll term (`enchant/service.go:549`),
  and the floor example is a +5 Conjuror's Staff (sections 1, 5); a valuable left carried by a
  round cut off by `called()` or by a failed or skipped deposit now starts the next round itself
  (a round starts for any carried valuable neither stored nor released), a round cut off after its
  first line counts its rounds and runs the cap, and once the valuables budget is spent an
  off-round errand deposits the carried ones (sections 3, 7, 8); `roundReady` requires the
  warehouse errand's open to be due (`openDue`, `retryAt`, shared with clan requests) and a
  deposit not due at a round's end is skipped, so no round waits out the backoff (sections 3, 6,
  Risks); the supply `S` and its inputs (`KillFloor`, `NoRespawnCycle`, `SpoilWeight`,
  `SevenSignsShare`, spawn events) are process settings given to `NewTravelMap` in a
  `SupplyConfig`, while `R`, `V` and their other knobs are per persona (section 1, Persona
  defaults); rule "valuables" lands in V2 as a stepper of an open round, V4 adds its ad step and
  V6 its invite step, so `TestAdPacing` and `TestValuablesTradeInvite` can pass in their PRs
  (section 2, Delivery); the armor enchant chance is 0.66^(level - 2), and `EnchantRare` stays
  flat on purpose (server table, section 1); the buylists left out are those with NPC id 0 or
  less, the 212 `npcId="-1"` lists, which carry all four valuable rows (Problem, M8 table,
  section 1).
- Round 5 (independent reviewer, 3 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): the supply now reads each maker's `<ai type>`, its params
  and its `spawnTime` (V1): the event is `event=` else the `EventName` param, so the 231
  `event_maker` makers are gated by `-spawn-events`; makers that do not start at boot count
  nothing; `random_spawn` divides by its entries, `on_day_night_spawn` counts 0.25 by night and
  0.75 by day (the server's night is 6 of 24 game hours, so the review's factor of 2 is 4 or 4/3),
  `maker_instant_spawn` groups come back whole every `KillFloor × N`, and
  `maker_instant_spawn_random` sets share their turns; checking this finding found the
  `spawnTime` and `on_start_spawn` starts too. The section 1 table was recomputed (Elven Tunic,
  Conjuror's Staff and the D-grade scroll move a little, no verdict changes), 11 templates priced
  at 10000 or more cross `ValueMin` with the types, so the Risks bullet no longer says the ceiling
  overstates every item alike, and `TestValueScore` pins each type rule (server table "Maker
  types" and "Spawns", section 1, Testing, V1, V2, Risks); `TestValuablesHoldsTurn` (V7a) asserts
  only the round step at a `follow` cut-off and `TestValuablesRelease` (V7b) the cap there; V3
  names the private request slot `privateWant` and both `startWarehouse` changes, the guard
  (no `Want`, no crafter cycle and no clan no longer stop a queued private request) and the order
  (the slot ahead of `warehouseWant`), with test cases (sections 3, 6, Delivery).
- Round 6 (independent reviewer, 8 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed): the private keeper is bounded by a round's area,
  `RoundReach` (6000) around the round's anchor (the bot's position at its start), not by
  `storeTownRadius` of the town point, which is a gatekeeper spawn (`TravelMap.Towns`), not a
  centre: in Dion (5348 from Trisha to keeper 30058), Arisha, Dark Elven Village, Gludin and Teranu
  Mardui no keeper stands within 3000 of it; `roundReady` requires a keeper spawn within
  `RoundReach`, the invite walk is bounded to the same area, and `TestPrivateWarehouseErrand` and
  `TestValuablesRoundStart` gain a Dion case (sections 3, 5, 6); the store no longer opens ahead
  of a pending round held back only by a short wait (ad board, warehouse open, an errand): `storeReady`
  waits up to `StoreHold` (2 min) for it and rule "valuables" holds the turn on the timers, with
  `TestValuablesStoreHold` (section 3, V7a); the ledger's lifecycle is stated: entries by template
  and enchant with derived carried and stored unit counts, so a sale, a drop, a withdrawal or a
  deposit moves them with no hook of its own, and the cap counts warehouse rows (a non-stackable
  unit is a row), with `TestValuablesLedger` and the sold item gone from the ledger and the next ad
  in `TestValuablesTradeConfirm` and `TestValuablesStoreSplit` (sections 2, 7, V2, V3); the Shout
  row no longer claims bot idle lines on the general channel (`sayAction` has only test callers);
  whispers within `AnswerAfter` (30 min) of a round's end that name a held item get an item reply,
  not a canned one (section 5, V5); item names match by full name or a distinctive word, never by
  stop words or words several held names share (section 5); the name index for every scored
  template moves from V1 to V2 (the datapack already keeps every name); D31 states the buylist
  exclusion of the `npcId="-1"` lists next to the multisell refinement.
- Round 7 (independent reviewer, 8 findings; each checked against the cited bot code and server,
  all confirmed and fixed): the private status is the brain's and is rebuilt every session
  (`brain.go:220`, `session.go:350`), so every session looks at the private warehouse before any
  round, cap or private request, no entry is dropped while its stored units are unknown, and a
  round open when its session ends is ended by the next session as a `called()` cut-off, with the
  `valuables` value handed across sessions like the chat log (section 2); rule "valuables" holds
  the turn for a pending round only while `StoreHold` runs, so a long warehouse backoff or a busy
  ad board no longer keeps the store and every errand waiting (section 3); an unexpired trade
  request counts on the server as a transaction in progress (`ProcessingTransaction`), which
  refuses the keeper's page, the warehouse request and a store opening, so invites stop 15 s
  before the round's end and the deposit and store opening wait for a pending request, and the
  "Warehouse request gate" row is corrected with a store opening row added (sections 3, 5, server
  table); V6 depends on V4, the trade-sale cap check moves to V7b's `TestValuablesRelease` and
  V5's after-round reply is tested on a carried valuable, the stored case moving to V7a's
  `TestValuablesRoundDeposit`; V3 carries an `Anchor` and `Reach` on the private request instead
  of reading the round; the row `trackContents` appends keeps the deposited `ObjectID` (the server
  moves a whole item as the same instance, `inventory/warehouse.go:329-336`), so a release names
  the right enchant; the reply queue outlives the round for `AnswerAfter`, a reply pending at the
  end going out as an after-round reply (section 5); and once the valuables budget is spent the
  release withdraw no longer runs and the off-round deposit takes released carried valuables too
  (sections 3, 7, 8).
- Round 8 (independent reviewer, 5 findings; each checked against the cited bot code, server and
  datapack, all confirmed and fixed, one in part): a keeper visit leaves the bot at the keeper
  (`warehouse.go:420-433`), and in Dion, Arisha, Dark Elven Village, Gludin and Teranu Mardui the
  keeper is past `storeTownRadius` of every town point (Dion's 30058 is 5348 from Trisha and 5398
  from 31219, `spawnlist/20_22.xml:551,620,756`), so `town` (`storeregistry.go:49-56`) and
  `startStore`'s `hasRoom(at)` (`store.go:332-336`) failed there: rule "valuables" now walks the
  bot back to within `anchorArrive` of the anchor after every keeper visit and every walk to a
  buyer, before the ads, the next invite and the store opening, with a Dion store-opening case,
  a pacing row and the walks in the time bound (section 3 "Back to the anchor", steps 1, 2, 4, 6,
  sections 2, 5, 6, Testing, V6, V7a, Risks); look rows with no entry now start one (rounds 0, the
  first ask) by a warehouse row test on what a `WarehouseItem` carries (template, enchant,
  augmentation, subcategory; the template's shadow mark), with a process-restart case in
  `TestValuablesLedgerLook` and the Risks bullet on rows that fail it (section 2); the busy test
  covers a pending request of any kind in the server's one request slot (trade, party, party room,
  command channel, friend, duel, `trade/book.go:15-30,53-62,195-209,350-357`): the server rows
  say so, the deposit and store waits name what the bot sees and answers (rule "prompt" answers a
  party invitation first) and what it cannot (a friend or duel request fails the deposit like any
  failed deposit). Checking this finding showed that clan and alliance invitations are kept in a
  separate book (`clan/invite.go:11-13,57-60`) and do not count, so that part of the finding was
  not applied (server table, steps 5, 6, section 5, `TestValuablesGrace`); the look's start conditions are the
  off-round errands' (no ad board, `RoundGap` or budget wait), and a look not done is one of the
  pending round's short waits (sections 2, 3, `TestValuablesStoreHold`, `TestValuablesLedgerLook`);
  released entries are in no round: the round's withdrawal, ads, haggle replies, trades and round
  step leave them out (sections 2, 3, 5, `TestValuablesRelease`, V4-V6).
- Round 9 (independent reviewer, 8 findings; each checked against the cited bot code, server,
  harness and datapack, all confirmed and fixed, one in part): the V7a tests no longer assert V7b
  behavior: the Dion store opening after a round lists the loot only in `TestValuablesRoundStart`,
  and the look's rows in the cap and a left-over round's cap waiting for the look move from
  `TestValuablesLedgerLook` to `TestValuablesRelease`, with the Dion opening of a released
  valuable; the e2e scenarios get an unmarked, non-GM buyer account from V9 (every harness
  character carries the bot marker, `docs/e2e-live.md:146`, and the GM is trade-exempt,
  `trade.go:272-276`, `authority.go:294-306`) and a Human Mystic merchant, for whom the Sword of
  Watershadow is no gear (`gear.go:219`); checking this finding showed that Tears of Eva is
  `weapon_type` ETC, which `indexGear` skips for every class (`gear.go:356-358`), so that half of
  the gear claim was not applied, the mystic merchant covering the sword (Testing, V9); whisper
  replies are bounded to 100 UTF-16 units, listing replies taking whole labels in score order
  (section 5, `TestValuablesWhisperHaggle`); the 142 at a keeper answers its bypasses (`Chat`
  links and private links), not the talk (`folk_bypass.go:432-434`), a 142 to the `Chat 1` link
  today leaves a session-long no-link mark (`warehouse.go:451-468,675-676`), which V3 replaces
  with a refusal and no mark, and a refused open ends through `endTransaction` with no
  `retryAt`, the next open waiting on `openedAt` (server table, sections 3, 5, 6,
  `TestValuablesGrace`, `TestPrivateWarehouseErrand`, V3); a round waits for rule "member" to
  settle a clan member's finished visit (`member.asked`, `clanmember.go:422-462`), so the clan
  crafter's board is not held stale through a round (sections 2, 3, `TestValuablesRoundStart`,
  V7a); a released entry not listed in the open store gets the `not now` reply, not a canned
  line (section 5, `TestValuablesStoreSplit`, V7b); `HoldRows` is stated as the bound right after
  an advertised round only (a `FirstLineWait` round and the budget-spent deposit store past it),
  the fee reserve follows the carried rows and the Pacing row counts the off-round opens
  (sections 6, 7, Pacing); the ad board's `Claim` records a line atomically at the decision, under
  its mutex like `StoreRegistry.claim`, only when the chat gate lets the line out, and counts it as
  said at the claim (section 4, `TestAdPacing`, V4).
- Round 10 (independent reviewer, 5 findings; each checked against the cited bot code, server,
  harness and datapack, all confirmed and fixed): 20 of the 50 spawned warehouse keepers show the
  bot no private link (18 freightmen whose `Chat 1` page links only `_DepositF` and `_WithdrawF`,
  `html/warehouse/30054-1.htm:2-3`; the Ketra and Varka alliance keepers Jaff and Hagos, whose
  script page for a non-allied player has no link, `varkasilenossupport.go:185-198`,
  `ketraorcsupport.go:186-200`), so the server table names them, and `roundReady`, the look and
  the pending round count only keepers not marked `noPrivate`: Teranu Mardui leaves the served
  towns (its only keeper within reach is Hagos; its nearest private keeper is 39103 away) and with
  Kurfa counts as keeper-less once marked, the farthest nearest private keeper staying Dion's 5348;
  a request ended by a new `noPrivate` mark sets no `retryAt` and goes on to the next keeper, and
  the marks live in the `valuables` value across sessions, so the seven towns whose nearest
  keeper is a freightman (Talking Island, Giran, the Elven Village, Esmeralda, Bella, Tamil,
  Flauen) cost one visit per process, not one per session and a 60 s wait (sections 2, 3, 6,
  Pacing; `TestPrivateWarehouseErrand` gains freight-only and alliance keeper cases,
  `TestValuablesLedgerLook` and `TestValuablesRoundStart` Talking Island and Teranu Mardui cases,
  and the e2e scenarios expect Rant marked and Rolfe used); the round state holds the anchor,
  which `openRound(anchor)` records, so V6 reads it without V7a work (section 2, V2b, V6, V7a,
  `TestValuablesTradeInvite`); V9 adds the harness's `EncodeAnswerTradeRequest`, which the buyer
  needs to accept the bot's `TradeRequest` (`harness.go:136-143` has none, `trade.go:145-150` is
  unexported); V2 splits into V2a (supply table, score, restart region) and V2b (policy, ledger,
  round state, classification, labels), and V3 into V3a (the private errand) and V3b (the
  session's look, with `TestValuablesLedgerLook`, whose round cases move to
  `TestValuablesRoundStart`), which takes the look out of V7a; the Risks bullet says the
  `maximumNpcs` caps are read (`datapack/spawnlist.go:31-33`) but not applied.
- Round 11 (independent reviewer, 6 findings; each checked against the cited bot code, server,
  datapack and `docs/plan.md`, all confirmed and fixed): a bot called away no longer keeps walking
  to a keeper: rule "warehouse" (`rules.go:500`) comes before "follow" (`rules.go:507`) and steps
  any running request (`warehouse.go:402-406`), so at the `called()` cut-off rule "valuables"
  ends its own private request through a new `cancelPrivate` (V3a) before the request's own
  packet went out, with no result, no `retryAt` and no walk back, and waits at most 5 s for one
  already sent (`whSent`), since the errand drops events once ended (`warehouse.go:655-659`); the
  look and the off-round errands are cancelled the same way (section 3, V3a, V3b, V7a,
  `TestValuablesHoldsTurn`, `TestPrivateWarehouseErrand`, `TestValuablesLedgerLook`); released
  stored valuables now reach the store opening ahead of the loot: the round's withdrawal takes
  them too while the valuables budget is not spent, and a release withdrawal waiting for its open
  (60 s after the look or a deposit, `warehouse.go:302-311,330-341`) is pending like a round and
  holds the store back under `StoreHold` (sections 2, 3, 7, V7b, `TestValuablesRoundStart`,
  `TestValuablesRelease`); the enchant exchange of #356 joins the errand checks of `roundReady`,
  the pending round and the look (`ench.step` idle; `enchant.go:222-259`, server
  `network/warehouse.go:291` cancels the selected scroll), with test cases; the stale bot and
  `docs/plan.md` line citations moved by #356, #359 and the M11-M13 rows are updated (`rules.go`,
  `brain.go`, `harness.go`, `gear.go`, `crafter.go`, `shop.go`, `store.go`, `clanmember.go`,
  `datapack_test.go`, `farm_live_test.go`; plan.md lines 321, 328, 332, 343-344); the `noPrivate`
  marks are per bot and process, since each `Bot` hands its own `valuables` value to its sessions
  (`session.go:212,349`, `chat.go:43-45`), so N merchants visit a freightman up to N times (sections
  2, 3, 6, Pacing); the datapack name is `Recipe: Halberd(60%)`, with no space
  (`items/5400-5499.xml:476`, sections 1, 5).

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Datapack paths (`npcs/...`,
`spawnlist/...`, `multisell/...`, `buyLists.xml`, `recipes.xml`, `restartPointAreas.xml`) refer to
`aCis_datapack/data/xml`; HTML pages (`html/...`) to `aCis_datapack/data`. Bot paths name this
repository.

## Problem

A real player who loots something rare does not hand it to a store at a loot price. They shout
"WTS" on the trade channel, answer the whispers, haggle a little, trade it, and only when nobody
bites do they park it in the warehouse and try again later, often together with the next good find.
When too much has piled up, they dump it in their store. The merchant bots treat a Sword of
Watershadow like a stack of stems.

The owner asked for this (2026-11-24): "before selling some valuable thing on the town store, try
to sell it to a player: open a trade, send a global message, and wait for some time, then put it in
the warehouse; when next time there is another valuable item, take from the warehouse and try to
sell both; when too much valuable is collected, just sell on the town store", and "valuable should
be marked not only by price, but also by how rare the item is".

### What the bot does today

| Concern | Today | Where |
| --- | --- | --- |
| What is worth something | Nothing is. A carried item is kept (supplies, keep set, keep hook, gear worn next), sold to an NPC as junk, or listed in the merchant's store as loot. The `merchant` preset is a clan member: while that option is active (a clan with a crafter on the board), its part of the keep hook keeps every recipe scroll, every material of a clan recipe and every graded item the bot would not wear, and the supply trip's arrival deposits them in the clan warehouse. | `shop.go` `junk`, `keepsItem`; `store.go` `storeLoot`; `cmd/bots/persona.go:46`; `clanmember.go` `wireMember`, `memberActive`, `memberHold`, `memberKeep`, `memberDeposit` (lines 51-60, 101-137, 326-334) |
| Store | The `merchant` preset opens a private sell store of its loot in a town of the store registry (at most 5 per town, 150 apart), priced 120-150 % of the NPC buy-back price (60-75 % of the reference price), at most 4 rows, within a 50000 adena budget per session. It stays open until sold out, a teleport or a `follow`. | `cmd/bots/persona.go:46`; `cmd/bots/store.go:13,28-30`; `store.go` `StorePolicy`, `defaultStorePolicy`, `ruleStore`, `storePrice`, `storeRows`; `storeregistry.go` |
| Trade | Answers a player's request: declines in store mode, in combat, from a player out of sight, within `Every` (1 min) of the partner's last accepted request, or with no goods. Offers the whole stack of the first priced item, at the policy price or the store price, adena only in return, confirms once the partner's adena covers the price and the partner stands within 120 (150 less a margin), cancels after `Timeout` (1 min). | `trade.go` `acceptTrade`, `tradeGoods`, `tradePrice`, `tradePaid`, `inTradeRange`, `defaultTradePolicy` |
| Asking for a trade | Never. `encodeTradeRequest` is reached only from the e2e harness. | `trade.go:139`; `harness.go:147` |
| Chat out | Whisper replies from a canned list, at most 5 an hour, 3-12 s after the whisper, never to a name with the bot marker; the crafter's clan lines. Every line passes the chat gate (3 s). No Shout or Trade line: `docs/plan.md` section 4 says "never on Shout or Trade". The Say2 encoder takes any channel. | `chat.go` `ChatPolicy`, `defaultChatPolicy`, `chatHeard`; `craftorder.go` `answerOrder`; `pacing.go` `chatGap`; `brainpackets.go` `encodeSay2` |
| Chat in | Whispers (commands, then the canned reply) and clan lines; every other channel is ignored, so bots never react to Trade lines. | `command.go` `heard` (lines 134-157) |
| Warehouse | The clan warehouse only: the `_DepositC` / `_WithdrawC` links, list type 2, no errand without a clan; 30 adena per deposited row; opens at least 60 s apart. | `warehouse.go` `clanWarehouseCommand`, `clanWarehouseType`, `startWarehouse`, `warehouseFee`, `warehouseOpenGap` |
| Prices | The reference price of every tradable priced template (`items/*.xml` `price`), and the cheapest price of every buylist of an NPC, spawned or not: `indexShops` skips every list whose NPC id is 0 or less, i.e. the 212 shipped lists with `npcId="-1"` that no NPC sells (`buyLists.xml`, e.g. list 1001 at line 9408). An item any counted buylist sells is never loot; the market cap alone looks at spawned sellers (`spawnedSold`). | `travel.go` `prices`, `cheapest`; `shop.go` `indexShops` (lines 280-298, the skip at 283-285); `store.go` `storeLoot`; `market.go:233`; `datapack/items.go` `buildPrice` |
| Rarity | Not read. The NPC reader skips `<drops>`; the spawnlist reader skips each maker's `<ai>` and its `spawnTime`; multisell and restart regions are not read. | `datapack/npc.go` `npcElement`; `datapack/spawnlist.go:85-89` `makerElement` |

## Goal

A merchant bot that loots a **valuable** (an item worth much for its price and its scarcity):

1. keeps it out of the NPC sale and its store;
2. in town, before it opens its store, advertises it on the Trade channel, answers whispers with a
   price, haggles down to a floor, asks a whisperer in sight for a trade, and sells by trade;
3. after the wait, deposits what did not sell in its private warehouse;
4. on a later town visit with a new valuable, withdraws the stored ones and advertises them all;
5. once too much has piled up, lists everything in its town store.

Out of scope: buying valuables from players, multi-item trade windows (one valuable per trade),
the clan warehouse for valuables, and every preset but `merchant`.

## Server behavior this rests on

Read in the Go server and the datapack on 2026-11-24. Every design choice below rests on these.

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Trade channel (8) | Heard by every player whose position falls in the speaker's restart region; its own flood gate, `TradeChatTime`. | `network/chat.go:115-121`, `handler/chat/chat.go:14-22` |
| Shout channel (1) | The same region, but its flood gate `GlobalChatTime` is shared with the general (all) channel. Whisper replies go on the tell channel, which has no flood gate. (The bot says nothing on the general channel today: `sayAction`, bot `rules.go:415-418`, has only test callers, `brain_test.go:205,246`, `rules_test.go:69`.) | `network/chat.go:24-28,89-113,149-171` |
| Hero Voice channel (17) | The only channel heard by every player online, but a line from a player who is not a hero is dropped with no answer. So no channel a non-hero bot speaks on reaches the whole server. | `network/chat.go:123-128`; `handler/chat/chat.go:12-33` |
| Restart region | The restart point whose `map` tiles (32768 units square) contain the position: Talking Island town's region is nine tiles (X 15-18, Y 23-25), so farmers at the island's spots hear the town. A position in no region (index -1) is grouped with every other such position. | `network/chat.go:236-255`; `model/restart/restart.go:83-117`; `restartPointAreas.xml:1062-1108` |
| Chat gates as shipped | `GlobalChatTime = 0`, `TradeChatTime = 0`: no server gate. A line inside a configured gate is dropped with no answer, and the gate is not extended by the dropped line. | `ops/docker/config/server.properties:228-229`; `aCis_gameserver/config/server.properties:228-229`; `network/floodprotector.go:32-41` |
| Line limits | At most 100 UTF-16 units, else dropped in silence. A whisper starting with a walker command (`SAY`, `MSG`, `EXIT`, ... in upper case) is dropped when walker protection is on. Literal `\n` pairs are removed. | `handler/chat/chat.go:63-65,76-84,94-127` |
| Whisper | Delivered as `CreatureSay` with the sender's object id and name; the sender gets the line back addressed `->name`. An absent target answers SystemMessage 145; a jailed, chat-banned or blocking target answers a refusal message. No flood gate. | `network/chat.go:144-173` |
| Trade request | Needs the target in the requester's known list (no distance check, else 144), neither side in olympiad or (configurable) chaotic (1987 with a text), neither side setting up a store (340) or running one (1065), neither side busy (142 the requester, 153 the target), no block (940 blocking everything, 619 on the ignore list), and transactions allowed (794). The target gets `SendTradeRequest`, the requester SystemMessage 118. No flood gate. | `network/trade.go:19-23,44-108`; `network/serverpackets/systemmessage.go:57,62-77,134,197,293,609,613` |
| Answer | A denial sends 119 to the requester. Accepting opens the window at any distance; the 150 range is enforced at the first confirm and at settlement. | `network/trade.go:139-146,315,518-519` |
| Request expiry | 15 s. | `trade/book.go:12-13` |
| Private warehouse links | `npc_<obj>_DepositP` (exact) and `npc_<obj>_WithdrawP` (prefix) of a warehouse keeper. Keepers differ: of the keeper pages that carry `_DepositP`, 19 are first pages (`<id>.htm`, e.g. Wilford, 30005) and 11 are second pages reached by `Chat 1` (`<id>-1.htm`, e.g. 30083). Not every keeper has them: of the 50 `WarehouseKeeper` templates spawned by a maker with no event (the travel map's keepers, bot `travel.go:152-156`), 30 show both private links; 18 are freightmen whose first page links only `Chat 1` and whose second page only `_DepositF` and `_WithdrawF` (Rant 30054, Aldo 30057, Barder 30079, Parman 30104, Chad 30151, Marty 30169, Cliff 30182, Raut 30316, Ballin 30322, Carlon 30350, Murdoc 30521, Imantu 30563, Silva 30686, Romp 30843, Kluck 30895, Stefano 31270, Daisy 31315, Axel 31959); and the alliance keepers Jaff (31374, Ketra) and Hagos (31381, Varka) have no `html/warehouse` page: a script answers their first talk, and a player not allied with that tribe gets a page with no link (`31374-no.htm`, `31381-no.htm`). An allied player's pages carry `_DepositP`, but withdrawal there is the quest bypass `Quest VarkaSilenosSupport Withdraw` (or `KetraOrcSupport`), not `_WithdrawP`. | `model/actor/npc/folk_warehouse.go:44-59`; `html/warehouse/30083-1.htm:2-3`; `html/warehouse/30005.htm:3-4`; `_DepositP` in `html/warehouse/*.htm`; `html/warehouse/30054.htm:4`, `30054-1.htm:2-3` (Rant); `npcs/30000-30999.xml:1915-1916`; `script/feature/varkasilenossupport/varkasilenossupport.go:79,141-142,185-198`, `script/feature/ketraorcsupport/ketraorcsupport.go:186-200`; `html/script/feature/VarkaSilenosSupport/31381-no.htm:1-3`, `31381-2.htm:3-4`; `html/script/feature/KetraOrcSupport/31374-no.htm:1-3` |
| Private lists | Deposit list and withdraw list of type 1; an empty private warehouse answers SystemMessage 282 instead of a list. | `network/warehouse.go:152-167`; `network/serverpackets/warehouse.go:20-26` |
| Deposit fee | 30 adena per requested row, taken once the rows fit, before the moves; a row the warehouse refuses is skipped and its fee kept; short of the fee, SystemMessage 279. Withdrawal is free. | `network/warehouse.go:20-21,318-365`; `inventory/warehouse.go:124-169` |
| Private warehouse size | 100 slots, 120 for a dwarf, plus the `whLimit` stat. | `model/actor/player/character_storage_limits.go:23-33,71-76` |
| Warehouse request gate | The keeper last talked to within interaction reach, no transaction in progress (142), no karma (configurable); every other refusal is silent. A transaction in progress is an open trade window or any unexpired request in the server's one pending-request slot, sent or received, whatever its kind: trade, party, party room, command channel, friend, duel or party duel all use that slot and its 15 s requester clock (`processingRequestLocked` checks the slot and the clock with no kind filter). So a `TradeRequest` the bot sent less than 15 s ago and not yet answered refuses its own deposit or withdrawal, and so does a party or friend invitation a player sent the bot and it has not answered. Clan and alliance invitations sit in a book of their own and do not count. The keeper's links are refused the same way, not the talk: talking to a warehouse keeper during a transaction opens its first page as usual, but every bypass to it (its `Chat` links as well as `_DepositP` and `_WithdrawP`) answers 142 and no page, since the keeper's bypass checks the transaction before the warehouse commands and the generic `Chat` command; nothing on the talk path checks it. | `network/warehouse.go:285-313`; `trade/book.go:15-30` (kinds), `:53-62` (one slot "whatever the kind of request"), `:195-209` (`Invite` records every other kind with `recordLocked`), `:332-358,775-780` (`ProcessingTransaction`, `processingRequestLocked`); callers `network/party.go:100`, `partymatch.go:124`, `party_channel.go:38`, `friends.go:170`, `duel.go:88`; `clan/invite.go:11-13,57-60` (the clan book); `network/bypass.go:195,333-334` (bypass path only); `model/actor/npc/folk_bypass.go:432-434` (ahead of the warehouse commands and of `dialogCommand`'s `Chat`, `:523-537`) |
| Store opening during a request | A private store set-up is refused (busy) while the player has an unexpired request of any of those kinds sent or received. | `network/privatestore.go:86-96` (`ProcessingRequest` sets `Busy`); `trade/book.go:342-357` |
| Drop roll | Per kill, each category rolls its chance once per unit of rate; a DROP (normal) category then gives at most one of its drops, weighted by the drops' chances; a SPOIL category rolls every drop on its own, and only on a spoiled monster; HERB and CURRENCY are herbs and adena. Shipped rates are 1. | `model/item/drop.go:97-145`; `model/item/reward.go:30-35`; `ops/docker/config/server.properties:121-123` |
| Respawn | A killed NPC of the default maker (`default_maker` and every type with no maker of its own; other types below) comes back after `respawn` plus a whole number of seconds drawn evenly from ±`respawnRand` (the draw clamped to `respawn`), so the mean delay is `respawn`. An entry with `respawn` 0 (`0sec`) or `no` (read as -1 s) never respawns: the default maker schedules nothing for it, and a delay of 0 or less arms nothing. | `model/spawn/respawn.go:9-29`; `script/maker/maker.go:47-52,158-165`; `data/manager/npcs_makers.go:428-431`; `internal/commons/gameduration.go:19-20` |
| Maker types | An npcmaker's `<ai type>` picks how it spawns; a type with no maker of its own on the server runs the default one, among them `random_spawn_treasurebox` (909 makers) and `random_maker` (49). `random_spawn` (372) places one of its entries, drawn evenly, at start (its whole total) and again at each NPC leaving, after the drawn entry's delay, while that entry is short of its total and the group under `maximumNpcs`; `random_spawn_pawn` (13) places every entry at start and respawns the same way. `on_day_night_spawn` (98) lives only by night (`IsNight` 1, 66 makers) or only by day (`IsNight` 0, 32); night is in-game 00:00-05:59, a quarter of the game day. `maker_instant_spawn` (53) respawns no NPC on its own: its group's last NPC leaving brings the whole group back with no delay. `maker_instant_spawn_random` (439) respawns nothing itself: its group's last NPC leaving wakes one of `maker_name1`-`maker_name3` (a draw below `maker_cnt`), which brings its group back after the leaving NPC's respawn delay; 435 of them name themselves, and they form 217 linked sets (196 pairs and 13 triples each with one maker that starts at boot, 6 single makers that do, 2 that never start). `no_on_start_maker` (13) spawns nothing at start. `event_maker` (231) spawns only while the `SpawnEvents` list names its `EventName` param. 43 other types have makers of their own and hold 290 makers, among them `royal_rush_maker` (68), `manage_teleport_dungeon` (54), `royal_req_next_maker` (40), `unique_npc_kill_event` (26) and boss makers such as `sailren_maker`. | `script/maker/maker.go:14-19` (`Catalog`: every other type runs `Default`), `:29-55,57-96` (default start, respawn, event 1001), `:98-136` (`NoOnStart`, `Event`, `eventListed`); `script/maker/random_spawn.go:7-12,33-98`; `script/maker/instant_spawn.go:5-16`; `script/maker/on_day_night_spawn.go:11-17`; `task/gameclock.go:19-20`; `script/catalog/makers_1.go:8-29`; counts of `<ai type>` in `spawnlist/*.xml`, e.g. `spawnlist/19_23.xml:1129-1134` (`random_spawn`), `spawnlist/18_23.xml:589-592` (`maker_instant_spawn`), `spawnlist/19_16.xml:1213-1218` (`maker_instant_spawn_random`), `spawnlist/17_22.xml:1109-1112` (`event_maker`) |
| Privates (minions) | An `<npc>` entry's `<privates>` block lists `<private id weight respawn>` rows (respawn defaults to 0). The leader's AI places them when it spawns, once for each leader: an entry of `total` N places its privates N times. An entry with no `<privates>` rows takes its template's (`npcs/*.xml` `<privates>`). A private follows its leader and is deleted with it; one with a positive `respawn` comes back on its own while its leader lives. The shipped spawnlist holds 3159 `<private>` rows under 1124 entries; 331 entries with `total` above 1 carry 943 of them (e.g. NPC 20986, `total="2"`). 205 NPC templates have a `<privates>` block; 175 entries of spawning makers have no rows of their own and take their template's 746. | `data/xml/spawnlist.go:40-45,225-237`; `model/spawn/entry.go:20-24`; `data/manager/npcs_script_spawn.go:100-164` (`CreatePrivates`, `privatesOf`); `data/xml/npc.go:37,150-158`; `spawnlist/21_24.xml:3292-3295` |
| Spawns | A maker entry's `total` (times `SpawnMultiplier`, 1 shipped, for territory spawns), `respawn` and `respawnRand`. A maker's event is its `event=` attribute, else its `EventName` AI param (`Maker.EventName`); a maker with one spawns only while it runs. Only a maker with no `event=`, no `spawnTime` (333 makers: siege, clan hall and door-open times) and no `on_start_spawn` 0 starts at boot (`Maker.OnStart`). The `SpawnEvents` list (`extra_mob;18age;start_weapon` shipped) starts at boot and is matched against that name: the 231 `event_maker` makers carry no `event=`, their names are `EventName` params (`extra_mob` 48 makers, the only ones whose NPCs have DROP or SPOIL rows; `event_mutant_pig` 66, `event_treasure_box` 41, `christmas`, `medal`, ...). The Seven Signs groups start by period: `ssq_event` (429 entries, the catacomb and necropolis monsters, e.g. NPC 21236 `respawn="142sec"`) through Recruiting and Competition; in Results and Seal Validation one group per seal, `_dawn` or `_twilight` when that cabal owns the seal and won, else `_none`. Competition and Seal Validation each run a week (to Monday 18:00), Recruiting and Results 15 min. No shipped config starts any other event (`medal`, `letter_collector`, `heart_event`, `christmas`, ...). | `model/spawn/entry.go:44-52`, `model/spawn/maker.go:27-40`, `model/spawn/group.go:220-245` (`EventName`, `OnStart`); `data/xml/spawnlist.go:166-167,209`; `data/manager/npcs.go:552-573` (`spawnOnStart`); `data/manager/npcs_makers.go:549-561,567-593,629-667` (`held`, the Seven Signs groups, `startEvent`); `spawnlist/17_22.xml:1110-1112`; `ops/docker/config/npcs.properties:6,12`; `sevensigns/sevensigns.go:93,700-711`; `spawnlist/18_10.xml:2534-2536` |
| Raid bosses | Spawned like monsters with long delays: Greyclaw Kutus (25001, `type RaidBoss`) `respawn="36hour" respawnRand="24hour"`. | `spawnlist/18_22.xml:864`; `npcs/25000-25999.xml:3-4` |
| NPC buy-back | An NPC buys at half the reference price. | `network/serverpackets/shop_trade.go:121` |
| Enchant | Safe to +3 (+4 full body); past it a weapon succeeds at 0.7 a try (0.4 magic; 0.35 and 0.2 from +15), and armor at 0.66^(level - 2), the level being the item's before the try: 0.66 from +3, 0.44 from +4, 0.29 from +5, so each armor level is rarer than the last. A scroll enchants only an item of its own grade and kind, so an item with no grade has no scroll (no entry of the table has no grade). Three scroll templates per grade and kind: normal, blessed and crystal (D-grade weapon: 955, 6575, 957). The normal ones: weapon 729, 947, 951, 955, 959 and armor 730, 948, 952, 956, 960 for A, B, C, D, S. | `enchant/service.go:38-48,56-87,531-549` (`return s.grade == tmpl.Crystal`), `:552-576` (`math.Pow(cfg.ChanceArmor, float64(enchantLevel-2))`) |

The datapack's drop data (`npcs/*.xml`), as the server reads it (`data/xml/npc.go:36-47,127-131`):
`<drops><category type="DROP|SPOIL|HERB|CURRENCY" chance="%"><drop itemid min max chance="%"/>`;
`npcs/20000-20999.xml:209-216` shows a SPOIL and a CURRENCY category. The shipped NPC files hold
4561 DROP, 1645 SPOIL, 1905 CURRENCY and 7362 HERB categories. Multisell rows
(`multisell/*.xml`) name their NPCs, ingredients and productions (`multisell/002.xml:3-28`); some
rows are upgrades that consume another weapon (`multisell/311262511.xml:2-12`, the Blacksmith of
Mammon: a weapon plus Ancient Adena). Recipes name their product (`recipes.xml:3`, `product="3-1"`).

Nothing the design needs is missing on the server, so this plan files no server issue.

## What the M8 reference prices give

M8 already reads one number per tradable template: the reference price (`items/*.xml` `price`,
`datapack/items.go` `buildPrice`, kept only for `price > 0` and `is_tradable`). From it:

| Price | Value | Where |
| --- | --- | --- |
| NPC buy-back | 50 % of reference | `network/serverpackets/shop_trade.go:121` |
| Bot store (loot) | 60-75 % of reference (120-150 % of buy-back) | bot `store.go` `defaultStorePolicy`, `storePrice` |
| Most a bot pays at a player store | 150 % of reference (`Market.MaxOverReference`), never above a spawned NPC's price | bot `market.go` `defaultMarketPolicy` |
| NPC buylist (any list whose NPC id is above 0, spawned or not; the `npcId="-1"` lists are not counted) | the cheapest buylist price; such an item is never loot | bot `shop.go` `indexShops`, `travel.go` `cheapest`, `store.go` `storeLoot` |

The reference price is a fair measure of what an item is worth, and an untradable template has
none, so it can never be valuable (nor traded). It says nothing about scarcity: Scroll: Enchant
Weapon (Grade D) (955) costs 50000 and has a supply ceiling of 54 per hour; Tears of Eva (309)
costs 136000 and has one of 0.62 per hour (section 1). The valuables price band below sits between
the store's top (75 %) and the market cap (150 %), so a released valuable in a bot store is also
something another bot's market errand may buy as gear.

## Design

### 1. The value score

Split in two by who owns the inputs. The process builds one travel map from the datapack and every
bot shares it (`cmd/bots/swarm.go:313`, `botclient.NewTravelMap(dp)`; read-only after
`NewTravelMap`, `travel.go:21`), so only inputs that are the same for every bot of the process can
go into it:

- **On the travel map, once at start:** for every template with a reference price, the supply
  ceiling `S` and two marks, `multisell` (a spawned NPC's non-upgrade multisell row makes it) and
  `craftable` (a recipe product), in a read-only table `TravelMap.supply` beside `prices`. `S`
  depends on the maker shares and the cycle terms below (`KillFloor`, `NoRespawnCycle`,
  `SpoilWeight`, `SevenSignsShare`, and the spawn events), which are therefore **process
  settings**, not persona fields: V2a gives `NewTravelMap` a second argument, a `SupplyConfig`
  holding them, built in `cmd/bots/swarm.go` from the `-spawn-events` flag and the defaults named
  below (no flags for the other four).
- **Per persona, when an item is tested:** `R` from `S` and the marks, with the persona's
  `CommonSupply`, `RMax` and `CraftDivisor`; the enchant terms with its `EnchantRare`; the worth
  and the score `V`; the thresholds `PriceFloor` and `ValueMin`. These are a few arithmetic
  operations per carried item, so nothing is cached per persona.

Each signal and how it is read:

- **Price** `ref`: the reference price.
- **Supply ceiling** `S`, items per hour server-wide, as if every spawn were killed the moment it
  respawned: over every maker, every `<npc>` entry of a template with drops and every private
  of an entry, `S += share × total × 3600 / cycle × p × mean`, with the entry's `total` for its
  privates too: the server places an entry's privates once for each NPC of the entry it spawns
  (server table, "Privates"), so a private row under an entry of `total="2"` yields two privates.
  An entry's privates are its `<private>` rows, or its template's `<private>` rows when it has
  none, as the server's `privatesOf` reads them (V1 reads both). The maker's `share` is the
  part of the time it spawns, and the entry's `cycle` how often it comes back; both depend on the
  maker's event, whether it starts at boot, and its `<ai type>` (server table, "Spawns" and "Maker
  types"; V1 reads each maker's `<ai type>`, its `<set>` params and its `spawnTime`, which the bot
  reader skips today, `datapack/spawnlist.go:85-89`). A private row takes its leader's share.
  - **Event.** The maker's event is its `event=` attribute, else its `EventName` param, as the
    server's `Maker.EventName` reads it, so the 231 `event_maker` makers are gated by their
    `EventName`. Share 1 with no event or with an event of the spawn events list (a flag
    defaulting to the shipped `extra_mob;18age;start_weapon`); `SevenSignsShare` (0.5, one week in
    two) for `ssq_event` and for the `ssq_seal1_none` and `ssq_seal2_none` groups; 0 for the
    `_dawn` and `_twilight` seal groups (they need a cabal that owns the seal and won, which the
    bot cannot know ahead) and for every other event, which no shipped config starts.
  - **Start.** A maker with no event that does not start at boot (a `spawnTime`, or
    `on_start_spawn` 0 outside the linked sets below) and every `no_on_start_maker` has share 0:
    only a script or a siege starts it.
  - **Type.** `default_maker` and every type with no maker of its own on the server (such as
    `random_spawn_treasurebox`) keep the share and take the cycle below. `random_spawn` and
    `random_spawn_pawn`: the share is divided by the maker's entry count, since one entry, drawn
    evenly, is placed at a time (closest when the entries' totals are equal, as in
    `spawnlist/19_23.xml:1129-1134`). `on_day_night_spawn`: the share times 0.25 for `IsNight` 1
    and 0.75 for `IsNight` 0, the server's night being 6 of the game day's 24 hours (a constant
    from `task/gameclock.go:20`, not a setting). `maker_instant_spawn`: every entry's cycle is
    `KillFloor × N`, N the group's NPC count (the sum of its entries' totals): its NPCs are killed
    one after another and the group comes back whole with no delay, so the entries' `respawn` is
    not used. `maker_instant_spawn_random`: the cycle is `respawn + KillFloor × N`, and the share
    is multiplied by the fraction of its linked set's makers (the makers joined by their
    `maker_name1`-`maker_name3` params) that start at boot: the set passes each cleared group's
    turn on to one of its named makers, so a pair with one starter counts 0.5 each (an even spread
    over the set, an approximation). The 43 other types with makers of their own (290 makers,
    such as `royal_rush_maker` and `manage_teleport_dungeon`) are counted like `default_maker`, an
    approximation that `TestValueScore` names.

  Then the `cycle` of a maker counted like the default one (server table, "Respawn" and
  "Privates"):
  - an entry with `respawn` above 0: `respawn + KillFloor` (`KillFloor` 60 s, the time to find and
    kill it); `respawnRand` adds nothing, since the server's draw is even around `respawn`;
  - an entry with `respawn` 0 or `no` (the bot's `datapack.NoRespawn`, -1 s): it spawns once per
    server start, counted as once per `NoRespawnCycle` (24 h). Freya's Gardener (22100,
    `spawnlist/23_14.xml:1289`, `respawn="no"`) is one of the 93 `respawn="no"` entries;
  - a private row (under a maker of any type): its leader's cycle, since it spawns with its
    leader, or `respawn + KillFloor` when its own `respawn` is above 0 and that is shorter (it
    comes back while its leader lives).
    12 templates priced at 10000 or more drop (DROP or SPOIL) only from privates, among them
    Recipe: Halberd(60%) (5458, 366000; the datapack name has no space, `items/5400-5499.xml:476`), a SPOIL of Judge of Light (21546), a private of Judge of
    Splendor (21544, `respawn="100sec"`) with its own `respawn="0sec"`
    (`spawnlist/22_15.xml:441`). It comes out at `S` 0.045 and `R` 3.35, not at `R = 0`.

  `p = category% × drop% / 10000` and `mean = (min + max) / 2`. DROP categories count fully,
  SPOIL ones times `SpoilWeight` (0.2: only a spoiler rolls them), HERB and CURRENCY not at all.
  Raid bosses count like any spawn, so their drops come out rare by their long cycles.
- **Rarity** `R = clamp(log10(CommonSupply / S), 0, RMax)`, `CommonSupply` 100 per hour, `RMax` 4.
  A drop source is an entry or a private row with a share above 0; every source adds a positive
  term, so `S > 0` whenever an item has one. An item with no drop source and no other source
  (only seasonal event makers, quest rewards) has `R = 0` (unknown counts as common, so price
  alone decides).
- **Shop and multisell:** an item any NPC buylist sells (`cheapest`: every list whose NPC id is
  above 0, the test `storeLoot` uses) is not loot at all. The lists with `npcId="-1"`, which
  `indexShops` skips (`shop.go:283-285`), do not count: they carry all four valuable rows of the
  table below (433 in list 1001, `buyLists.xml:9408,9477`; 309 in 2012, `:10437,10442`; 121 in
  9001, `:11612,11620`; 184 in 9017, `:11868,11872`), and counting them would make every one of
  them a shop item. An item a
  multisell row of a spawned NPC produces from ingredients that are no weapon or armor has `R = 0`.
  A row that consumes a weapon or armor (an upgrade or exchange) is not a source: it moves one rare
  item into another.
- **Craftable:** a `recipes.xml` product has `R / CraftDivisor` (2): a crafter can make more.
- **Grade:** not a signal of its own. Grade is already in the reference price and in the supply
  (higher grades drop only from higher monsters); counting it again would count it twice. The ad
  names it for D grade and up.
- **Enchant** `e` (the carried item's `Enchant`): the worth adds the reference price of the
  item's grade's **normal** enchant scroll per level (the cheapest scroll a player could have
  spent; the weapon or the armor scroll by the item's kind: weapon 729, 947, 951, 955, 959 and
  armor 730, 948, 952, 956, 960 for A, B, C, D, S, `enchant/service.go:57-66`; the blessed and
  crystal scrolls of the same grade, `enchant/service.go:67-86`, are not used). An item with no
  grade (no `crystal_type`, such as Sword of Watershadow and Tears of Eva below) has no scroll
  term: no scroll enchants it (`enchant/service.go:549`), so its scroll price is 0 and `W = ref`
  whatever its `Enchant`. Every level past the safe level (+3, +4 full body) adds `EnchantRare`
  (0.25) to `R`, since each such level survived a roll. The term is flat on purpose, for weapons
  and armor alike: the server's armor chance falls with each level (0.66^(level - 2), server
  table, "Enchant") while a weapon's stays at 0.7, so the flat term understates high armor
  levels; it only nudges `R`, and an enchanted find is rare enough already.
- **Worth** `W = ref + e × scroll`, **score** `V = W × 2^R`.

A carried item is **valuable** when `ref ≥ PriceFloor` (10000), `V ≥ ValueMin` (100000), and it is
not kept for the bot's own use (`keeps`: supplies, keep set, shots, ...), not kept by the persona's
own keep hook, not worn next, not a set piece kept for a set, not a quest item, not augmented, not
a shadow item, and not sold by any NPC buylist (`cheapest`, the `npcId="-1"` lists left out). The precedence is: the bot's own keeps and the
persona's own keep hook, then the gear errand, then valuable, then the clan member's hold (M9),
then loot. Augmented items stay out because the store excludes them today (`storeLoot`) and their
worth is not in the datapack.

**The clan member's hold comes after valuable, on purpose.** The `merchant` preset is a clan
member (`member: true`, `cmd/bots/persona.go:46`). While that option is active (in a clan with a
crafter on the board, `clanmember.go:101-104`), `wireMember` wraps the persona's keep hook with
`memberHold` (`clanmember.go:51-61`), which keeps every recipe scroll, every material of a clan
recipe, and every graded item the bot would not wear (`memberKeep`, `clanmember.go:112-137`); the
member's visit deposits them all in the clan warehouse (`memberDeposit`, `clanmember.go:326-334`).
Ahead of the valuable test, that hold would take every graded valuable and every valuable recipe
scroll of a member merchant whose clan has a crafter. Of the four valuable rows of the table below,
two are D grade, Conjuror's Staff (184, `items/0100-0199.xml`, `crystal_type` D) and Elven Tunic
(433, `items/0400-0499.xml`, `crystal_type` D), and Recipe: Halberd is a recipe scroll. The other
two, Sword of Watershadow (121, `items/0100-0199.xml`) and Tears of Eva (309,
`items/0300-0399.xml`), have no `crystal_type`, so the server does not crystallize them,
`indexCrystals` leaves them out of `TravelMap.crystals` (`crystallize.go:66`, `crystalItem` 0 for
no grade), and `memberKeep` returns no hold for them (`clanmember.go:127-129`); neither is a
material of any recipe (`recipes.xml`). The hold would still take half of the table and every
valuable recipe. So V2b makes two changes in `clanmember.go`:

- `wireMember` keeps the persona's own hook it wraps (`holds`, `clanmember.go:53`) in the member
  state, and the valuable test asks that hook (with `keeps`), not the wrapped `Persona.Holds`, so
  it never runs back through `memberHold`;
- `memberKeep` returns no hold for a valuable. It asks the valuable test under its `inHold` guard
  (the test asks the gear errand, which asks the keep hook back, `clanmember.go:40-44`).

`memberDeposit` keeps only rows `memberKeep` keeps, `memberHandOff` weighs `memberDeposit`'s rows,
and the member's fees (`memberFees`, the `fees` the supply trip's arrival sets from
`memberDeposit`, `clanmember.go:72-80,351-370`) count them, so this one change takes a valuable out
of the member's deposit, its load trigger and its reserved fees. A graded item or recipe scroll
that is not valuable still goes to the clan warehouse as today. A member merchant thus sells a
valuable recipe or valuable gear to players instead of handing it to the clan crafter (D31).

Computed from the shipped datapack on 2026-11-24 with this formula, the maker shares, starts
and types, the respawn rules and the privates included (each private `total` times, the
template's when the entry has none; a throwaway script; V2a's shipped test pins the same figures).
Counting the Seven Signs makers at half moves only two rows: Conjuror's Staff (S 0.045 without
them) and the D-grade scroll (S 53.6). Counting the template privates moves none of the rows or
Recipe: Halberd. The maker types move two rows a little and no verdict: Elven Tunic from S 0.45
(three `maker_instant_spawn` groups, such as `spawnlist/18_23.xml:588-592`, whose three NPCs now
come back every `KillFloor × 3`, 180 s, instead of at their entries' `respawn="68sec"`) and the D-grade scroll from 54.4
(`random_spawn` and `maker_instant_spawn` makers). Over all 1236 tradable
templates priced at 10000 or more with a drop source, they move 11 across `ValueMin` on price and
rarity alone (the shop, multisell and craft marks not applied), all toward valuable, and `R` by
0.1 or more for 112: the rule did not overstate every item alike.

| Item | ref | S per hour | R | V | Valuable |
| --- | --- | --- | --- | --- | --- |
| Sword of Watershadow (121) | 136000 | 0.67 | 2.17 | 612000 | yes |
| Tears of Eva (309) | 136000 | 0.62 | 2.21 | 628000 | yes |
| Elven Tunic (433) | 99900 | 0.44 | 2.35 | 510000 | yes, by rarity (price alone would not) |
| Conjuror's Staff (184) | 644000 | 0.056 | 3.25, craftable: 1.63 | 1988000 | yes |
| Scroll: Enchant Weapon (Grade D) (955) | 50000 | 54.3 | 0.27 | 60100 | no |
| Fist of Butcher (2371) | 25500 | 3.77 | 1.42 | 68400 | no |

Of the DROP-category items of spawned monsters up to level 20 that no spawned NPC sells and no
non-upgrade multisell makes, 5 of 71 are valuable; up to level 30, 14 of 155. These two counts
were taken before the Seven Signs shares, the respawn rules, the private rows and the maker
types, and with the
spawned-seller filter rather than `cheapest`; V2a's shipped test recounts them with the final rules. So a low-level merchant meets a
valuable now and then, not every trip.

### 2. The valuables ledger

What the bot holds, owned like the chat log: the `Bot` hands it to each session's brain in turn, so
its owner is the current brain goroutine (`chat.go` `chatLog` is the model).

- **Entry.** One per template and enchant level (`Enchant`), holding only what the bot cannot
  read back: the rounds it was advertised unsold, its current ask, and whether it is released.
- **Units, derived.** An entry's units are not stored in it; they are counted on each decision
  from what the bot already tracks. Carried units: the inventory items (`World.Inventory.Items`,
  `items.go:37-51`, each with `TemplateID`, `Count` and `Enchant`) of the entry's template and
  enchant that pass the valuable test (section 1). Stored units: the rows of the private
  warehouse status (`private.Contents`, section 6) of the same template and enchant. The look and
  the withdraw list give each row's `ObjectID` and `Enchant` (`WarehouseItem`,
  `warehouse.go:75-84`), but `trackContents` appends a deposited row with its template and count
  only (`warehouse.go:734`), so V3a also copies the deposited item's `ObjectID` and `Enchant` into
  that row (rows, below). So the ledger follows every
  change with no hook of its own: a trade sale (`SendTradeDone`, `trade.go:256-257`, then the
  `InventoryUpdate` that takes the item away), a store sale, a drop or a destroy takes carried
  units away, a withdrawal moves units from stored to carried, a deposit back.
- **Added and dropped.** An entry is added the first time a carried item passes the valuable test
  and no entry has its template and enchant (rounds 0, the first ask of section 5). It is dropped
  at the first decision where it has no unit carried and the session's private status is known
  with no unit stored, so a sold valuable leaves the ledger, the ads and the cap at once. While
  the private status is not known (every session until its look, below), stored units are
  unknown and no entry is dropped for want of them. A unit found while its entry exists joins it, with its
  rounds and ask (the template is already on offer at that price); one found after its entry was
  dropped starts a new entry.
- **Stored rows with no entry.** A row of the private status (the look's withdraw list, or a later
  withdraw list) whose template and enchant no entry has also adds an entry, rounds 0 and the
  first ask, not released, when the row passes the valuable test as far as a warehouse row can
  be tested. A `WarehouseItem` (`warehouse.go:75-84`) carries `TemplateID`, `Enchant`,
  `Augmentation` and `SubCategory`, but no `Equipped` and no `ManaLeft`, and `keeps` and the
  keep hook take an `Item` the bot carries (`shop.go:610`, `clanmember.go:53-61`). So a row is
  tested on its template and enchant: `ref ≥ PriceFloor`, `V ≥ ValueMin` at the row's `Enchant`,
  not sold by a counted buylist (`cheapest`), not a quest item (`SubCategory` 3,
  `subCategoryQuest`, `trade.go:31`), not augmented (`Augmentation` 0), and not a shadow item (the
  template's `Duration`, as `indexCrystals` reads it, `crystallize.go:66`). The carry checks
  (`keeps`, the persona's keep hook, worn next, a set piece) are left out: they decide what the bot
  wants to carry, and once a stored unit is withdrawn the carried test decides it again like any
  carried item (a unit the gear errand now wants to wear leaves its entry). This is the case after
  a process start, when the ledger is empty and every stored valuable has no entry (Risks); within
  one process the deposited units already have their entries. A row that fails the test (say `ValueMin` was
  raised since it was deposited) gets no entry: the bot neither withdraws it nor counts it in the
  cap, and it stays in the private warehouse (Risks). Only rule "valuables" deposits in the
  private warehouse, so every row there was a valuable when it went in.
- **Rows.** The cap (section 7) and the deposit count warehouse rows, as the server does
  (`storeSlots`, server `inventory/warehouse.go:307-316`): each unit of a non-stackable template
  is a row of its own (two +0 Swords of Watershadow are two objects and two rows), a stackable
  template one row. A withdrawal or deposit names a non-stackable entry's units by object, as
  `pickRows` takes them (`warehouse.go:507-520`, a row naming an object takes that object only;
  a row with `Object` 0 takes any object of the template), so releasing a +0 sword never takes a
  held +5 one of the same template. A stored unit's object is known from the look or the withdraw
  list, or, for a unit the session deposited, from the row `trackContents` appends: the server
  moves a whole item into the warehouse as the same instance and takes a new object id only for
  a split into a warehouse holding no stack of it (`move`, server
  `inventory/warehouse.go:329-336`), which a non-stackable unit never is. A stackable entry
  (enchant 0, one row) is named by template.
- **What a round uses.** A round starts for an entry with carried units that is not released
  (section 3); each ad line names the entries with carried units that are not released, at the
  time of the line, so an item sold earlier in the round is not named again. A released entry is
  in no round: no ad line names it, the round's whisper replies and trades leave it out (section
  5), its rounds and ask do not step, and the round's deposit leaves it carried. It goes to the
  store: carried, at the next store opening; stored, through the round's withdrawal, which takes
  it along while the valuables budget is not spent, or through the off-round withdraw errand,
  which holds the store opening back for it under `StoreHold` (sections 3, 7). The cap counts each entry once, released or not, from its derived units.

**Every session looks first.** The ledger lives on the `Bot` and survives sessions, but the
private status it reads stored units from does not: the warehouse errand is a field of the brain
(`wh`, `brain.go:230`), `NewBrain` builds a new brain for every session (`session.go:350`), and
only the chat log is handed across (`session.go:212`, `brain.go:311-313`). After any reconnect the
private status is not known, and `trackContents` applies no moved row to a status that is not
known (`warehouse.go:718-720`), so a deposit before a look would leave the ledger too. So every
session, not only the first after a process start, learns the stored items from a look at the
private warehouse (the withdraw list, or 282 for empty) on its first town visit, before any round
and before any other private request (the off-round errands of section 3 included). The rule
"valuables" runs the look with the session's private status not known, whether or not a new
valuable is carried, as soon as the off-round errands' conditions hold (section 3): `roundReady`
without its region and ad board checks, that is `storeReady`'s state checks, a registry town, a
private keeper within `RoundReach` (a keeper spawn not marked `noPrivate`, section 6), no errand running (warehouse request, market visit,
gear `UseItem`, crafter cycle, enchant exchange, a clan member's visit not yet settled, store
opening or open store) and the warehouse open due (`openDue`, `retryAt`). The look says nothing, so it waits neither on `RoundGap`, the valuables budget nor the
ad board (whose `ProcessPerHour` cap can hold a region up to an hour, section 3), and the
off-round deposit and release withdraw that wait for it are not held up by the board either.
While those conditions do not hold, the look waits outside the turn: the rule does not match for
it, except under `StoreHold` for a pending round (section 3, where a look not done counts as one
of the short waits). Once they hold, the rule queues the look, steps it and walks the bot back to
its anchor (section 3, "Back to the anchor"), holding the turn throughout. Until the look is done,
no round starts, no cap runs and no private withdrawal or deposit is queued; the entries keep
their rounds, asks and release marks. The look's rows join the entries of their template and
enchant, and a row with no entry starts one at rounds 0 (above, "Stored rows with no entry"):
after a process start there is no ledger yet, so every stored valuable starts so (Risks). The
look needs a private keeper within `RoundReach` like a round (section 3); in a registry town with
none, the bot neither looks nor starts a round, and its store opens as today with its loot, the
valuables staying carried. A town whose only keepers within reach show no private link (Teranu
Mardui and Kurfa, whose keepers are the alliance keepers; section 3) counts as having none once
those keepers are marked: its first look of the process (each bot's own, section 2) marks them (section 6) and ends, and no
look or round follows there. A look is one warehouse open per session, under `warehouseOpenGap`
like any other (Pacing).

The ledger, the round state (open or not, start time, the **anchor**, ad lines said, and per
partner: replies, counters left, agreed price and whether it was asked for a trade) and the
keepers marked `noPrivate` (section 6) live in one `valuables` value, with `openRound(anchor)`
and `endRound`, that the `Bot` hands to each session's brain like the chat log
(`cfg.chat`, `session.go:349`), so the last round's start and end times and its reply counts
(`RoundGap`, `AnswerAfter`) hold across a reconnect, and so do the `noPrivate` marks, so a
keeper with no private link is visited once per bot and process, not once per session (section 6).
The value is the bot's own, like its chat log (a field of each `Bot`, `session.go:212`, handed on
at `session.go:349`; `chat.go:43-45`), so every merchant learns the marks for itself: with N
merchants in one town, a freightman there is visited up to N times per process. The
anchor is the bot's position when the round opens: `openRound` takes it and records it with the
start time, and the invite walk (V6) and every walk back (section 3) read it from the round
state. A round still open when its session ends
(the connection lost, a logout) is ended by the next session's brain on its first decision as a
`called()` cut-off (section 3): after its first ad line each unsold entry's rounds go up by one
and its ask drops at once, and the cap (V7b) runs once that session's look is done, since it
counts stored units; before its first line nothing changes. Its reply queue, its partners and any
trade request it had open are dropped, since the connection they belong to is gone, and nothing
is deposited. V2b adds them and rule "valuables" at its place in the
rule list (section 3) as a stepper only: it matches only while a round is open, holds the turn
with a wait, and nothing opens a round before V7a (`openRound` is called from tests only), so V2b
is behavior-neutral. V2b also adds the walk back to an anchor (section 3, "Back to the anchor",
with `anchorArrive`) as a step the later PRs call. Each later PR adds its step to that rule: V3b
the session's look, V4 the ad step (section 4), V6 the invite step (section 5, "Asking for a
trade"); the whisper replies (V5) go out through rule "chat". So the tests of V4, V5 and V6 run on
a round opened with `openRound` at a given anchor and see their packets come out of `decide`.
V2b's ledger reads stored units through one accessor, which reports none stored and the status
known until V3b wires it to the private status (V3a). V3b adds the session's look (its
conditions, the walk back after it, the entries its rows start, the errand's `noPrivate` marks
kept in the `valuables` value). V7a adds what opens and ends a round (`roundReady`, which calls
`openRound` with the bot's position, the withdraw, the end and the deposit) and the hand-over
across sessions (the left-over round).

### 3. The round (rule "valuables")

A new rule, right after "loot" and ahead of rule "store" in the brain's rule list
(`rules.go:474-514`), for a persona with `Valuables` enabled. Combat, prompts and the trade window
(rule 2) and every rule before "loot" keep their priority over it.

**Holding the turn.** From the round's start to its end the rule matches on every decision, also
while it only waits, so no rule after it runs: no store opening, assist, gear, market, shop, dyes,
warehouse, supply, travel or spot errand starts and walks the bot away in the middle of a round.
The private warehouse steps (the look, withdraw and deposit) therefore cannot wait for rule
"warehouse", which sits after the errands (`rules.go:493-500`) and starts and steps its errand only
when it is reached (`warehouse.go:402-406`). The round queues its private request in the warehouse
errand's private slot (`privateWant`, section 6), starts it with `startWarehouse`, which takes
that slot ahead of `warehouseWant`, and steps the errand itself: V3a splits the body of `ruleWarehouse` past its start check
(`warehouse.go:407-433`) into `stepWarehouse`, which both rules call. This turns the crafter's
arrangement around: "craft" sits after rule "warehouse" and waits for it (`crafter.go:279-281`),
which works because a craft cycle may let errands run between its steps. A round must keep the bot
where it advertised, so it drives the errand itself.

**Start** (`roundReady`). The state checks of a store opening (`storeReady`, `store.go:356-379`:
a store registry and a travel map exist, alive, out of combat and PvP flag, not casting, still, no
trade prompt or window, no shop, trip, newbie, class, dyes or train visit, not called).
`storeReady` has no town check: it only asks that the registry exists (`b.cfg.Stores == nil`,
`store.go:358`), and the town test of a store opening is `hasRoom` in `startStore`
(`store.go:336`). So `roundReady` adds its own:

- in a store-registry town: the registry's `town` lookup (the nearest town point within
  `storeTownRadius`, 3000, `storeregistry.go:49-53`) finds one for the bot's position; a free
  store place is not needed. A town point is a gatekeeper spawn, not a town centre: `TravelMap.Towns`
  keeps the first gatekeeper spawn of each cluster, gatekeepers in id order (`storeregistry.go:114-131`,
  `travel.go:158-180`);
- a private keeper within reach: a keeper spawn (the travel map's `keepers`, `travel.go:152-156`)
  not marked `noPrivate` (section 6) within `RoundReach` (6000) of the bot. The travel map takes
  every spawned `WarehouseKeeper`, and 20 of its 50 keepers show no private link to the bot (server
  table, "Private warehouse links"): 18 freightmen and the two alliance keepers. In seven towns the
  keeper nearest the town point is a freightman with a private keeper a little farther: Talking
  Island (Rant 30054 at 2317 from Roxxy 30006, Rolfe 30055 at 2508; `spawnlist/17_25.xml:197,198,200`),
  Giran (Parman 30104 at 1176 from Clarissa 30080, Valkon 30103 at 1309), the Elven Village (Chad
  30151 at 1641 from Mirabel 30146, Julia 30152 at 1664), Esmeralda (30233: Silva 30686 at 1735,
  Sorint 30232 at 1870), Bella (30256: Ballin 30322 at 1946, Haprock 30255 at 2208), Tamil
  (30576: Imantu 30563 at 2328, Grookin 30562 at 2495) and Flauen (30899: Kluck 30895 at 1889, Mia
  30896 at 1945). There each bot's first private request of the process marks the freightman and the
  next takes the private keeper (section 6). At two town points the only keeper within reach is
  an alliance keeper: Teranu Mardui (31383, `spawnlist/23_16.xml:850`) has Hagos (31381, `:826`)
  at 3326 and its nearest private keeper, Hakon (31268, `spawnlist/24_16.xml:68`), 39103 away;
  Kurfa (31376, `spawnlist/24_15.xml:1099`) has Jaff (31374, `:1123`) at 1007 and Lietta (31267,
  `spawnlist/24_16.xml:67`) 22595 away. Before its first visit the check passes there; once the
  look has marked the alliance keeper, the town counts as having no keeper (below), and with the
  marks kept for the process that costs one visit per bot and process. A merchant is not allied with
  either tribe (no preset runs the alliance quests); an allied bot would see `_DepositP` but no
  `_WithdrawP`, and its look marks the keeper the same way. The bot's position when the round
  starts is the round's
  **anchor**, and every walk of the round (the private keeper, section 6; a buyer, section 5) ends
  within `RoundReach` of it. `storeTownRadius` is too short for this: the keepers of several towns
  stand farther than 3000 from their town point. Dion's point is Trisha (30059,
  `spawnlist/20_22.xml:620`) and its nearest keeper 30058 stands 5348 away (`:551`); Gludin's,
  Richlin (30320, `spawnlist/17_22.xml:462`), 3928 from keeper 30210 (`:291`); Dark Elven
  Village's, Jasmine (30134, `spawnlist/20_18.xml:441`), 4165 from keeper 30139 (`:364`); Arisha
  (31698, `spawnlist/21_16.xml:819`) 5115 from keeper 31313 (`:89`). These four keepers are
  private keepers. 6000 covers each of them, 5348 being the farthest nearest private keeper of any
  town point with one within 6000 (computed from the shipped datapack with the travel map's rules,
  freightmen and alliance keepers left out). A town point with no private keeper near it starts
  no round: Minerva (30836, `spawnlist/23_21.xml:119`) and Angelina (30878,
  `spawnlist/21_23.xml:146`), each more than 30000 from any keeper, and Teranu Mardui and Kurfa
  (above) once their alliance keeper is marked. The store opens there as today, the valuables
  staying carried;
- in a restart region: the bot's region (section 4) is not -1;
- the ad board would let a line out in that region now (`AdBoard.Free`: no line of the process
  in the region within `RegionGap`, and the process under `ProcessPerHour`), so a round does not
  start into a board that holds it;
- the errands `storeReady` does not check (`store.go:367`): no warehouse request running
  (`wh.running`), no market visit (`market.active()`), no gear `UseItem` waited for (`gear.obj`),
  no crafter cycle (`craft.phase`), no enchant exchange (`ench.step` at `enchantIdle`), no clan
  member's visit waiting to be settled (`member.asked`), and no store opening under way or store
  open. The enchant check matters because rule "enchant" (`rules.go:501`) sits after
  "valuables" and runs in town (`enchantFree` asks `inTown`, `enchant.go:274-286`), where rounds
  start, and its exchange spans several decisions: the scroll's `UseItem`, the server's selection,
  `RequestEnchantItem`, then the result (`ruleEnchant`, `enchant.go:222-259`, steps
  `enchantUsed`, `enchantChosen`, `enchantRequested`, `enchantSettling`). A round started between
  those steps would hold the turn for about 9 minutes with the scroll selected on the server, which
  refuses a logout meanwhile (`enchant.go:232-238`), and the round's private request would cancel
  the selection (server `network/warehouse.go:291`, `cancelActiveEnchant` in
  `warehouseRequestGate`). With the check, rule "enchant" steps its exchange to its end first.
  Rule "member" sits after
  "warehouse" and runs only once the clan request has ended and it is reached
  (`clanmember.go:422-462`, `rules.go:502`): it takes the withdrawn rows off the craft board,
  gives the warehouse lease back, posts the ore shot orders the deposit carried in and resets the
  reserved fees, and never takes the turn. The supply trip's arrival in town, a member's clan
  deposit, is exactly when a round would start, and a round holds the turn for about 9 minutes
  plus its walks, so without this check the clan crafter's board would miss those orders for the
  whole round. With it, rule "valuables" does not match while `member.asked` is set, rule
  "member" settles the visit when it is reached, and the round starts on a later decision;
- the warehouse errand could open now: `openDue(now)` and `retryAt` passed, the two checks
  `startWarehouse` makes before it takes a request (`warehouse.go:588-590`). The errand has one
  open gap for clan and private requests alike: `openDue` waits `warehouseOpenGap` (60 s) after
  the last open, doubled for each unanswered open or request in a row (`min(timeouts, 5)`
  doublings, at most `warehouseBackoffMax`, 30 min, `warehouse.go:22-23,302-311`), and a failed
  request sets `retryAt` 60 s on (`warehouse.go:351-352`). A round's look or withdrawal queued
  while the open is not due would either be refused by `startWarehouse` or hold the turn in
  `warehouseLink` until the open is due (`warehouse.go:444-446`), up to 30 min. So no round starts
  then: the bot waits outside a round, with its store held back for at most `StoreHold` (below),
  and starts the round once the open is due. This check holds also for a round with nothing to withdraw, so its deposit at
  the end is not held back by an earlier backoff either (the round lasts longer than the 60 s
  gap; a timeout within the round is handled at its end, step 5).

It leaves out `storeReady`'s loot check and its store budget gate (`store.go:358`); the valuables
budget (section 8) bounds a round instead. A round starts when `roundReady` holds, the
session's look is done (section 2), an entry has carried units and is not released (section 2), at least `RoundGap`
(30 min) has passed since its last round's start, and the valuables budget is not spent. A
carried held valuable is either a new find or one a round left carried: a round cut off by
`called()`, or a deposit that failed or was skipped (step 5). So such leftovers start the next
round on their own, 30 min on, with no new find; they are never stuck carried. Stored valuables
alone start no round: they wait for a carried one, as the owner asked ("when next time there is
another valuable item, take from the warehouse and try to sell both").

**Back to the anchor.** A keeper visit leaves the bot at the keeper: `ruleWarehouse` approaches
the keeper and ends the request there (`warehouse.go:420-433`), and nothing walks it back. The
walk to a buyer (section 5) likewise ends at the buyer. In the four towns named above (Dion,
Arisha, Dark Elven Village, Gludin) the private keeper stands farther than
`storeTownRadius` (3000) from every town point: Dion's keeper 30058 (`spawnlist/20_22.xml:551`)
stands 5348 from Trisha (30059, `:620`) and 5398 from the only other gatekeeper within 6000 of it,
31219 (`:756`, 53 from Trisha, so merged into the same town point). At the keeper the registry's
`town` lookup (`storeregistry.go:49-56`) finds no town for the bot's position, so `roundReady`
refuses the next round, `startStore`'s `hasRoom(at)` (`store.go:332-336`) refuses a store at the
bot's current position, and an ad would be said in the region of the keeper's spot. Left there,
a Dion merchant would never start a round after its look, and the loot and released valuables
of a round's end would leave town unsold. So rule "valuables" walks the bot back after each of
its own moves away from the anchor, holding the turn:

- after each private keeper visit (the look, the round's withdrawal, the round's deposit, an
  off-round errand), whatever the request's result, and after a walk to a buyer once its trade
  has ended or its request was refused or expired, the rule walks the bot back to within
  `anchorArrive` (100, a constant V2b adds with the walk back itself, which V3b, V6 and V7a call) of the anchor through the walker (`goTo` with `walkPlace`,
  as the supply trip's walk home does, `supplytrip.go:205`). One exception: a request that ended
  on a new `noPrivate` mark (section 6) is queued again at once, from where the bot stands and
  with the same anchor, and the walk back follows the request after it; when no unmarked keeper
  is left within `RoundReach`, the walk back follows at once;
- the round's next ad line, its next invite and its end wait for that walk: the first ad line
  goes out at the anchor (`FirstLineWait` counts from the walk back after the withdrawal), and at
  a round's or errand's end the rule lets the turn go only once the bot is back, so the store
  opening runs at the anchor;
- the anchor passed `roundReady`'s town check (the look's and the errands' anchors pass the same
  check, section 2 and below), so `town` finds the same town there, the ad region is the one
  `roundReady` checked, and `hasRoom` decides the store place where the bot stood before the
  round, as it would have with no round. A place another bot claimed within the 150 spacing
  meanwhile fails the opening as today;
- a walk back the walker answers unreachable (rare: the walker found the way out a moment ago)
  ends where the bot stands. After the withdrawal or a buyer's walk it ends the round like a
  `called()` cut-off (below: no deposit, the rounds step only after a first line), so the round
  does not advertise from the wrong spot; after the deposit, the look or an errand the rule
  simply lets the turn go. `roundReady` and `hasRoom` then decide from the bot's position as
  today.

**Store and round take turns.** A round holds the turn, so rule "store" is not reached while one
runs. An open store puts the brain on `storeRules` (`rules.go:518-524`), which have no "valuables"
rule, so no round starts while the store is open; `roundReady` also refuses an opening under way.
The server refuses any trade in store mode (`network/trade.go:69-75`), so this order is required,
not a choice.

**The store waits for a pending round.** Rule "store" sits right after "valuables"
(`rules.go:491-492` today has "loot", then "store"), and `startStore` opens wherever the bot
stands in a registry town (`store.go:329-353`; `storeReady`, `store.go:356-379`, has no valuables
check). So when a round cannot start for a moment, the store would open in the same decision; an
open store puts the brain on `storeRules` (`rules.go:518-524`), where no round starts, and
`acceptTrade` declines every trade in store mode (`trade.go:348-351`); the store then stays open
until it sells out, a teleport or a `follow` (`store.go:310-314`, it closes only when `StoreType`
goes to 0). The new valuable would get no round for that whole store session. To prevent this, a
round is **pending** when every start condition holds except the short waits: valuables
enabled, an entry with carried units not released, the valuables budget not spent, `RoundGap`
passed, a registry town with a private keeper (a keeper spawn not marked `noPrivate`) within
`RoundReach`, region not -1. The short waits
are the ad board (`Free`), the warehouse open (`openDue`, `retryAt`), the errand checks
(warehouse request, market visit, gear `UseItem`, crafter cycle, enchant exchange, a clan
member's visit not yet settled) and the session's look not yet
done (it waits on the same warehouse open and errand checks, section 2).

A **release withdrawal** is pending the same way (V7b): the session's look is done, the ledger
holds released stored valuables, the valuables budget is not spent, and the bot is in a registry
town with a private keeper within `RoundReach`; its short waits are the warehouse open and the
errand checks. Without this the released stored valuables would miss the store in the usual
flows. The withdrawal needs an open to be due, and right after the session's look or a round's
deposit the next open is `warehouseOpenGap` (60 s) away: `openDue` waits that long after
`openedAt` (`warehouse.go:302-311`), and `endTransaction` lifts the wait (`exempt`) only for a
deposit queued in the same request as a withdrawal (`warehouse.go:330-341`). `storeReady` has no
such wait (`store.go:356-379`), so rule "store" would open in that minute with the loot and the
released valuables already carried. The brain would then run `storeRules`, which have no
"valuables" rule (`rules.go:518-524`), and the store stays open until it sells out, a teleport or
a `follow` (`store.go:310-314`), so the stored ones would wait that long. This is the common case
in the session after one whose valuables budget ran out, when the off-round deposit put released
valuables in the warehouse. So everything below that holds for a pending round holds for a pending
release withdrawal too, under the same `StoreHold` count. `StoreHold` (2 min) covers the 60 s gap:
the withdrawal runs, the bot walks back to its anchor, and the opening lists the released
valuables ahead of the loot (section 7). Only a longer wait, such as a backoff after unanswered
opens, lets the store open first; the stored ones then wait for the next opening.

- While a round is pending, `storeReady` returns false, for at most `StoreHold` (2 min) counted
  from the first decision it held the store back in this town (the count resets when `town`
  finds no town or another one for the bot; while the rule's own keeper visit or walk back runs,
  the anchor's town counts, so a keeper beyond `storeTownRadius` does not reset it). The ledger look does not count against it: once the look's
  conditions hold (section 2) the rule runs the look and the walk back itself, holding the turn,
  so rule "store" is not reached meanwhile; only the wait for those conditions counts.
- While it is pending only on the ad board or the warehouse open (timers), and only while the
  `StoreHold` count runs, rule "valuables" holds the turn with a wait, so the bot stays where it
  stands instead of walking off on an errand or to farm. The turn hold ends when `StoreHold`
  ends, whatever the timers say: the board can hold a region until the process falls back under
  `ProcessPerHour` (up to an hour), and the warehouse open can be up to `warehouseBackoffMax`
  (30 min, `warehouse.go:22-23,302-311`) away, so a hold on the timers alone would keep rule
  "store" (`rules.go:491-492`, right after "valuables") and every errand after it from running
  for that long. While an errand runs, the rule does not match, and that errand's rule steps it to
  its end (holding the turn would stall it); the store stays shut by `storeReady`.
- Once `StoreHold` has passed with no round (or no release withdrawal), the rule stops holding
  the turn for it and `storeReady` stops holding the store back: the store opens as today with
  its loot (and any released valuables already carried), or, with nothing to sell, the bot goes on with its errands or farming. The carried
  valuables stay out of the store (section 1) and start a round once the store has closed, or
  when `roundReady` holds again while the bot is in town (the count starts afresh only in another
  town or after leaving this one).
- A valuable carried while `RoundGap` has not passed since the last round's start is not pending:
  the store opens at once, and the valuable waits for the store to close and the next round.
  Holding a store back up to 30 min would stall the loot sale for longer than the round's own
  bound (about 9 minutes, step 4); Risks names this case.

**Steps.**

1. **Withdraw** (only when the ledger holds stored valuables that are not released, or released
   ones while the valuables budget is not spent): the private warehouse errand (section 6)
   withdraws every stored valuable not released, one request. While the valuables budget is not
   spent, the same request also takes every released stored one (V7b): no ad names them and the
   round leaves them out (section 2), but they come out with the others, skip the round's deposit
   (step 5) and are listed by the store opening at the round's end (step 6), with no second open
   for the off-round withdraw errand. Then the bot
   walks back to the anchor ("Back to the anchor"). A withdrawal that fails (no keeper, no answer)
   leaves them stored in the ledger, and the round advertises only what it carries.
2. **Advertise:** the first ad line (section 4), said at the anchor once the walk back has ended,
   then one every `AdEvery` (2 min), at most `AdLines` (3) a round, each through the ad board.
   The lines name only entries not released (section 2).
3. **Answer** whispers (section 5) and trade requests, and ask whisperers for a trade (section 5).
   The bot stays in its town: it stands at the anchor, and the walk to a buyer it invites ends
   within `RoundReach` of the anchor (section 5, "Asking for a trade") and is followed by the
   walk back.
4. **End** `RoundWait` (5 min) after the first ad line. An open trade, or a buyer with an agreed
   price who stands in sight, extends the end by up to `Grace` (2 min) once. A round whose first
   line the ad board still holds `FirstLineWait` (2 min) after the walk back from the withdrawal
   ended (or after the round's start, with nothing to withdraw) ends there with no ad: it goes on
   to the deposit, but leaves each item's rounds and ask unchanged, since nobody heard of them. So
   a round lasts at most the withdrawal, `FirstLineWait`, `RoundWait`, `Grace` and the deposit:
   about 9 minutes plus the two warehouse visits and their walks back (each at most `RoundReach`,
   6000, from the anchor).
5. **Count, cap, deposit**, in that order. First each unsold entry the round could advertise (not
   released) gets its rounds up by one and its next ask lowered by `RoundStep` (section 5). Then the cap (section 7) runs on the updated rounds
   and asks, before any deposit. Then the private warehouse (section 6) takes every valuable still
   carried that the cap did not release; a released carried valuable skips the deposit and stays
   carried for the store, so it never pays the 30 adena a row (`warehouse.go:556`, server
   `network/warehouse.go:21`) only to be withdrawn by a second open at least `warehouseOpenGap`
   (60 s, `warehouse.go:22`) later. When the cap releases every carried valuable, the deposit is
   skipped. The deposit is queued only when the errand could open at once (`openDue` and
   `retryAt`, as in `roundReady`); when it could not (an unanswered open during the round doubled
   the gap), the round does not wait for it: it ends, and the valuables stay carried for the next
   round. A round that ended at `FirstLineWait` with no ad changes no rounds or asks and runs no
   cap, so no valuable is released before a round that advertised it. The deposit also waits while
   a request is pending: the server counts any unexpired request the player sent or received, of
   any kind in its one request slot (trade, party, party room, command channel, friend, duel), as
   a transaction in progress (`ProcessingTransaction`, server `trade/book.go:53-62,332-358,775-780`;
   server table, "Warehouse request gate") and answers every bypass to a warehouse keeper with 142:
   its `Chat` links and its `_DepositP` and `_WithdrawP` links
   (`model/actor/npc/folk_bypass.go:432-434`, `network/bypass.go:195,333-334`), while the talk
   still opens the first page; a deposit or withdraw request likewise
   (`network/warehouse.go:287-290`). The bot checks only an open window today (`depositRows`,
   `warehouse.go:535`). A 142 to the open link or to the request ends the transaction as refused
   (142 is in `warehouseRefusals`, `warehouse.go:68-71`; `endTransaction`, `warehouse.go:329-341`,
   which sets no `retryAt`), and the next open waits `warehouseOpenGap` from `openedAt`, set when
   the open link went out (`warehouse.go:302-311,373-374`). A 142 to a `Chat` link (the 11 keepers
   whose private link sits on `Chat 1`) comes while the errand is at its talk step, where it
   ignores system messages (`warehouse.go:675-676`): today the visit times out, talks again, gets
   the first page, does not follow the `Chat 1` link it already followed, and marks the keeper as
   having no link and fails (`warehouse.go:451-468`), a mark that lasts the session. V3a fixes this
   (section 6): a 142 at the talk step fails the request with `WarehouseRefused` and no mark.
   Either way the deposit is lost for this round. So the
   deposit is queued only once the bot's own `TradeRequest` has been answered (`TradeStart` or a
   refusal) or 15 s have passed since it was sent (`RequestTimeout`, server `trade/book.go:13`),
   and no request from a player waits for the bot's answer. The bot sends no other request
   (`encodeRequestJoinParty` and `encodeRequestJoinPledge` are reached only from the e2e harness,
   `harness.go:124,130`). Of the requests it receives it sees three (`Prompts`, `world.go:269-284`):
   a party invitation and a clan invitation, which rule "prompt" (rule 2, ahead of "valuables")
   answers at once (`rulePrompt`, `rules.go:584-603`), and a trade request, which it answers there too
   (`ruleTrade`) and which `storeReady` already checks; a clan invitation does not count on the
   server in any case (server table). A friend, duel, party room or command channel request the
   bot does not decode, so it cannot wait for one: such a request received in the 15 s before the
   keeper's `Chat` link, its `_DepositP` link or the deposit request refuses it with 142, and the
   deposit fails like any failed deposit (below: the valuables stay carried for the next round),
   with the keeper left unmarked so the next round deposits there again. Invites stop 15 s before the
   round's end (section 5), so the wait for the bot's own request is rare and at most 15 s.
6. The rule walks the bot back to the anchor ("Back to the anchor"), then stops matching; the
   store opening runs as today, at the anchor, with the ordinary loot and any released valuables.
   It too waits for a pending request: the server refuses a store set-up while the player has an
   unexpired request of any of those kinds sent or received (`ProcessingRequest`,
   `network/privatestore.go:88`), so `storeReady` also returns false while the bot's own
   `TradeRequest` is unanswered and under 15 s old (the trade prompt it already checks, and rule
   "prompt" answers a party invitation before rule "store" is reached). A request the bot cannot
   see refuses the set-up as it can today, since any player can already send one.

The rule checks `called()` on every decision: a bot called away (`follow`, assist, or the party
leader it follows, `followed`, `command.go:277-286`; `called`, `store.go:400-403`) ends the round at
once, keeping its valuables carried and depositing nothing.

**The cut-off cancels the private request.** Ending the round is not enough on its own: a private
request of the rule may be running (the look, the round's withdrawal or deposit, an off-round
errand), with the bot on its way to a keeper up to `RoundReach` (6000) away. Once rule
"valuables" stops matching, rule "warehouse" (`rules.go:500`) is reached before rule "follow"
(`rules.go:507`), and `ruleWarehouse` steps any running request to its end: it returns early only
in combat or when no request runs and none starts (`warehouse.go:402-406`), whatever `called()`
says. Left alone, a `follow` during the deposit would still walk the bot to the keeper and
deposit. So at the cut-off rule "valuables" ends its own running private request itself, through a
`cancelPrivate(now)` that V3a adds to the errand:

- before the request's own `RequestWarehouseDeposit` or `RequestWarehouseWithdraw` has gone out
  (the walk, the talk, the `Chat` links, the open link sent and its list awaited or received, steps
  `whTalk`, `whOpening`, `whListed`), the request ends at once through `end()`
  (`warehouse.go:345-348`): no result is recorded, `retryAt` is not set and the private status is
  left as it was. An open that already went out keeps its `openedAt`, so the next open still waits
  `warehouseOpenGap` from it (`warehouse.go:302-311`); its late list or 282 is ignored, since the
  errand drops every event while no request runs (`warehouseEvent`, `warehouse.go:655-659`). A
  cancelled look leaves the session's look not done, to be run again on a later town visit; a
  cancelled withdrawal leaves the valuables stored, a cancelled deposit leaves them carried;
- once the request has gone out (step `whSent`), it is not cancelled: the server may already have
  moved the rows, and only the errand's `InventoryUpdate` step applies them to the private status
  (`trackContents`, `warehouse.go:687-693`), which an ended errand would skip. Rule "valuables"
  keeps holding the turn with a wait, without moving, and steps the request (`stepWarehouse`)
  until its answer or its timeout, at most `warehouseReplyTimeout` (5 s, `warehouse.go:29`), then
  lets the turn go;
- no walk back to the anchor follows a cut-off: the bot is wanted elsewhere, and rule "follow" or
  "assist" takes it there. A walk back or a walk to a buyer already under way simply stops being
  stepped.

The same holds for the look and the off-round errands, which are not rounds: a bot called away
during one cancels its request the same way and lets the turn go. A clan request is never
cancelled this way: `cancelPrivate` touches only a running request with `Private` set.

A round cut off after its first ad
line counts like a round that ran to its end: each unsold item's rounds go up by one, its ask
drops, and the cap runs (step 5), since the items were advertised and did not sell. A round cut
off before its first line changes nothing, like one that ended at `FirstLineWait`. A failed
deposit (no keeper reached, no answer, fee short) also keeps them carried, out of the NPC sale and
the store. Either way the carried valuables start the next round `RoundGap` later (Start above),
which deposits them at its end; one that keeps failing to deposit still gets its advertised
rounds, so the cap releases it to the store after `MaxRounds` at most.

Outside a round the rule also runs short errands under `roundReady` without its region and ad
board checks (they say nothing), each one private request, holding the turn only while it and
its walk back to the anchor run (the same conditions start the session's look, section 2); the
bot's position when an errand starts is its anchor, and its keeper is chosen as a round's
(section 6):

- withdrawing released valuables that are stored (section 7), only while the valuables budget is
  not spent; while it waits for the warehouse open it is a pending release withdrawal and holds
  the store back under `StoreHold` ("The store waits for a pending round", above), so it runs
  ahead of the store opening: once `sold` reaches `Budget` no valuable lists for the rest of the session
  (section 8), so a withdrawal then would only pay an open for an item that stays carried; the
  released ones stay stored, keeping their release mark, for a later session;
- depositing every carried valuable, released or not, once the valuables budget is spent, so none
  stays carried for the rest of the session when no round can start for it and no store can list
  it (section 8): they go to the warehouse, as the owner asked, instead of staying carried. A
  released one keeps its mark and is withdrawn for the store in a later session whose budget is
  not spent.

Both errands run only after the session's look (section 2), which waits on the same conditions,
so neither is held up by the ad board. The bot sends no `TradeRequest` outside a round, and
`roundReady`'s state checks refuse a trade prompt or window, so no request of its own stands in
their way; one the bot cannot see fails the errand as in step 5.

### 4. Advertising on the Trade channel (D30)

- **Channel:** Trade (Say2 channel 8), never Shout: players read Trade for sales, and a sale on
  Shout reads as noise. (Shout's gate is shared with the general channel, but no bot line goes out
  on the general channel today, so that is no reason either way.) The owner asked for "a global message", but no
  channel a bot can use reaches the whole server: Trade and Shout reach only the speaker's restart
  region (`network/chat.go:108-122`), and Hero Voice, the one server-wide channel, drops the line
  of a player who is not a hero (`network/chat.go:123-128`). An ad said in Giran is not heard in
  Aden; D30 puts this to the owner.
- **Wording:** `WTS <name> <price>[, <name> <price>]... pm me`, names from `items/*.xml`: the
  datapack reader already keeps every item's name (`datapack/items.go:238` for weapons and armor,
  `:354` for etc items), but the travel map's `itemNames` holds only recipe products
  (`travel.go:88-91,138`, `indexProductNames`, `craftorder.go:260-277`, read by the craft order's
  label, `craftorder.go:212`); V2b extends it to every scored template. An entry of several units
  is named once as `2x <name> <price>` (the price each); an enchanted item as `+5 <name>`, a graded item with its grade
  (`<name> (C)`), prices in the shorthand players use: `173k`, `1.2kk` (rounded up to the shown
  unit, never below the ask). A line is at most 100 UTF-16 units (the server's limit); items that do
  not fit go to the next ad line of the round, in score order. Example:
  `WTS Sword of Watershadow 173k, Tears of Eva 174k, pm me`.
- **Region:** the server delivers a Trade line to the speaker's restart region. The bot computes its
  region from its position with the server's map-tile formula (copied, `model/restart/restart.go`
  `mapRegion`, with the bot's `geo` bounds) over the restart points' `map` lists (V1). In no region
  (-1), it does not advertise: the line would reach every player outside a region.
- **Ad board** (`AdBoard`, process-wide, one per process, like `StoreRegistry`: read-only regions,
  a mutex owns the counters): an ad line goes out only when no bot of the process said one in the
  same region within `RegionGap` (60 s) and the process said fewer than `ProcessPerHour` (30) in the
  last hour. `Free(region, now)` reports whether a line would go out now (`roundReady` and the pending
  round ask it). Brains decide on goroutines of their own, so `Free` is only a hint: two
  merchants of one region could both see it free. The ad step therefore claims its line at the
  decision: `Claim(region, now)` checks `RegionGap` and `ProcessPerHour` and records the line in
  one step under the board's mutex, as `StoreRegistry.claim` takes a store place under its own
  (`storeregistry.go:14-25,90-101`, claimed at the decision, `store.go:345-350`); a failed claim
  says nothing that decision. The ad step claims only when the chat gate lets a line out now
  (`b.pace.allow(gateChat, now)`, as rule "announce" does, `craftorder.go:225`), so the chat gate
  never holds a claimed line; one the frame bucket still holds (`send`, `brain.go:774-779`) stays
  the round's pending line and goes out on a later decision with no second claim. A line counts
  as said at its claim, also when it never goes out (the session ended first), so the board may
  over-count but never under-counts. A held line waits for the next tick; it never queues past the round, and `FirstLineWait` bounds
  the wait for a round's first line (section 3).
- **Per bot:** `AdEvery`, `AdLines` and `RoundGap` bound it to 6 lines an hour at most; every line
  also passes the chat gate (`chatGap`, 3 s).

### 5. Whispers, haggling and trades (D32)

**Prices.** For each valuable a round has an **ask** and a **floor**:

- first ask `ref × min(MaxAsk, 100 + AskPerRarity × R) / 100`, `MaxAsk` 150, `AskPerRarity` 12.5
  (so `R = 4` asks 150 %); the enchant scrolls are added at their reference price;
- floor `max(W × FloorPct / 100, storePrice(ref))`, `FloorPct` 100, with `W = ref + e × scroll`
  the worth of section 1: never below what the bot's store would ask, never below the reference
  price, and never below the enchant scrolls spent on the item. `storePrice` prices the bare
  template (`store.go:446-456`), so the floor adds the scrolls itself: a +5 Conjuror's Staff (D
  grade weapon, worth 644000 + 5 × 50000 = 894000, scroll 955 at 50000) has the floor 894000, not
  644000. An item with no grade (Sword of Watershadow) has no scroll term (section 1), so its floor
  is its reference price at any `Enchant`;
- each unsold round lowers the ask by `RoundStep` (10 points of the reference price), never below
  the floor.

**Whispers during a round** (from a name without the bot marker, not a command) go to the
valuables responder instead of the canned reply (`chatHeard`):

- **naming an item** (case-insensitive, over the entries with carried units that are not
  released, section 2). A word of an entry's name **points at** that entry when it has at least
  4 letters, is no stop word (`of`, `the`, `and`, `for`, `with`, `from`) and matches a whole word
  of the line, not a part of one. A pointing word is **distinctive** unless it is a word the item
  names share by kind (`scroll`, `enchant`, `recipe`, `grade`, `weapon`, `armor`). Each line has
  exactly one of three outcomes, tested in this order:
  1. it **names** an entry when it contains the entry's full name, the longest such name winning
     (`sword of watershadow`);
  2. else, when its distinctive words point at any entry: it names that entry when they point at
     exactly one, and it **points at several** when they point at two or more;
  3. else, when its kind words point at any entry: it names that entry when they point at exactly
     one, and it points at several when they point at two or more;
  4. else it **names nothing**.

  Stop words point at nothing, and a distinctive word wins over a kind word. Common words in
  names are real: Sword of Watershadow (`items/0100-0199.xml:300`) and Tears of Eva
  (`items/0300-0399.xml:224`) both hold `of`, so "of course" or "out of adena" name nothing,
  while "tears?" names Tears of Eva and "watershadow 150k" names the sword. With two scrolls
  held, "scroll 50k" points at both (its only pointing word is the kind word `scroll`); with one
  scroll and the sword held, it names the scroll; "watershadow scroll" names the sword;
- **reading a price.** A price is a whole word of the line, after the full item name it
  contains (if any) is taken out, that is a number written with digits (an optional `.` or `,`
  decimal part) and either a suffix `k` (thousand), `kk` or `m` (million), or no suffix and a
  value of at least 1000 (`150000`, `150k`, `1.2kk`, `1.2m`; `150 000` reads as no price). These
  are never a price: a number with a `+` before it (the enchant of a label, `+5`), a number with
  `x` right before or after it (a unit count, `2x`, `x2`), a number inside a word or with other
  signs around it (`Halberd(60%)`, `items/5400-5499.xml:476`, has `60%`, which is no whole word of
  digits), and a bare number under 1000 (`how much for 2 tears`). A price is per unit, as the
  labels show (section 4: `2x <name> <price>`, the price each). A line with two or more prices
  takes none and gets the asks (it is not clear which one is the offer). So "how much for +5
  conjuror's staff?" holds no price and is a price question, "2x tears 150k" an offer of 150000
  each, "recipe: halberd(60%) 300k" an offer of 300000;
- a line naming a held item or asking a price (`?`, `how much`, `price`) is answered `<name> <ask>,
  trade me`; one that points at several entries is answered with their asks, `WTS <name>
  <ask>, <name> <ask>, which?`;
- a line with a price is an **offer** on the item it names; when it names nothing, on the only
  valuable, else on the top-scored one; an offer in a line that points at several entries is
  not taken, and the line gets the asks as above. An offer below half the item's floor is not
  haggled on: it is answered like a price question (`<name> <current>, trade me`, the partner's
  current price) and uses no counter, so a lowball or a misread number never draws the floor out
  of the bot. Otherwise:
  - at or above the partner's current price: agreed at the current price, answered `ok <price>,
    trade me`;
  - below it, with counters left (`Counters`, 2 per partner and item a round): the bot counters
    `max(floor, (offer + current) / 2)`, rounded up to 1k, answered `<price> lowest`, which becomes
    the partner's current price;
  - with no counters left: agreed at the offer when it is at or above the floor, else answered
    `sorry, no`;
- anything else: `WTS <names>, trade me`.

Replies start with a lowercase word, an item name (a `+` for an enchanted one) or `WTS`, never a
walker command: the server drops a whisper that starts with one of its upper-case walker commands
(`SAY`, `MSG`, `EXIT`, ..., a case-sensitive prefix test, `handler/chat/chat.go:78-84,114-121`), and
`WTS` is none of them; a reply that would start with one gets a lowercase `wts`/`ok` lead instead.

Replies are at most 100 UTF-16 units, like an ad line (`MaxTextLength`, server
`handler/chat/chat.go:63-64`; `Admit` drops a longer line in silence, `:101-103`, and the bot's
own bound is `chatMaxText`, `craftorder.go:23`). A dropped reply would still count against
`RepliesPerPartner` and `Replies` and leave the buyer with nothing, so every reply is composed to
fit: a reply that lists entries (`WTS <name> <ask>, <name> <ask>, which?`, `WTS <names>, trade
me`, `WTS <names> again soon`) takes whole labels in score order, as many as fit with its fixed
words, and leaves the rest out (six held entries with names such as `Recipe: Halberd(60%)` or
`Conjuror's Staff (D)` do not fit one line); a one-item reply whose label alone would not fit
is cut on a rune by `clipChat` (`craftorder.go:231-240`), which no shipped name needs. The
check runs when the reply is composed, as it goes out (Queue, below).

Reply pacing and queue:

- **Delay:** each reply waits `DelayMin`-`DelayMax` (3-12 s, the canned reply's `ChatPolicy`
  delay, `chat.go:32-40`) after the whisper it answers, then passes the chat gate (3 s).
- **Queue:** today's reply path holds one pending reply and drops a whisper heard while it waits
  (`chat.go:128`, `b.chat.reply.kind != actHold`). The responder keeps its own queue in the round
  state instead: at most one pending reply per partner (a newer whisper from that partner replaces
  its pending reply, answered on the newest state), any number of partners, sent earliest due
  first by rule "chat" (`rules.go:482`, ahead of "valuables"), so a second buyer's whisper is not
  dropped while the first one's reply waits. The queue belongs to the responder, not to the open
  round: it outlives the round for `AnswerAfter`, so a whisper heard during the round whose reply
  is still waiting when the round ends keeps its reply. Such a reply is composed when it goes out,
  on the state at that time: after the round's end it is an after-round reply (below: no offer
  taken, no trade asked for), not a haggle answer. A reply still waiting when the next round
  starts is composed on that round; one still waiting when `AnswerAfter` has passed is dropped
  (that whisper got no reply, rather than a canned one 30 min late); the queue is cleared when the
  session ends.
- **Caps:** at most `RepliesPerPartner` (4) per partner and `Replies` (20) per round; past that,
  silence. Rounds start at least `RoundGap` (30 min) apart, so a bot sends about 40 valuables
  replies an hour. These replies are not counted in the canned reply's `chatLog` and its
  `PerSender` (10 min) and `Total` (5 an hour) caps (`chat.go:32-40`), which would cut a haggle
  off after its first line; D32 names this amendment.

**Whispers after a round.** A buyer often answers an ad minutes later, after the round has ended
and the bot has deposited or opened its store; the canned reply would brush them off with a random
line such as `lol`, `brb` or `hi` (`defaultChatPolicy`, `chat.go:32-40`, picked at random by
`chatHeard`, `chat.go:121-136`). So for `AnswerAfter` (30 min) after a round's end, until the next
round starts, a whisper (no bot marker, no command) that names a held entry (by the rule above,
over every entry, carried or stored) or asks a price still
goes to the responder, with its delay and queue, under the caps of the round just ended
(`RepliesPerPartner`, `Replies`; the responder keeps the last round's end time and reply counts
for this):

- an entry that is not released, carried or stored: `<name> not now, wts again soon`;
- a released entry listed in the bot's open store: `<name> in my store <price>` (V7b);
- a released entry not listed in the open store (stored and waiting for the release withdraw,
  carried before the opening or while a full town keeps the store shut, or left out by the
  valuables budget, section 8): `<name> not now, wts again soon` as well (V7b), never the canned
  line;
- a price question naming nothing: the held entries' names, `WTS <names> again soon`.

No offer is taken and no trade is asked for outside a round: the bot may sit in its store, where
the server refuses every trade (`network/trade.go:69-75`). Any other whisper, and every whisper
past `AnswerAfter` or with no round yet, gets the canned reply as today, with today's caps.

**Trades.** The trade rule (`trade.go`) learns valuables:

- `acceptTrade` counts a valuable as goods during a round (and `Every` does not hold back a partner
  with an agreed price);
- `tradeGoods` offers the valuable the partner named or agreed on, else the top-scored one, never
  a released one (it waits for the store, section 2); one valuable per trade;
- `tradePrice` of a valuable is the partner's agreed price, else its current price, else the ask;
  `tradePaid` confirms only once the partner's adena covers it, so the window itself is a haggle:
  a partner who adds less never gets a confirm. As today (`tradeStep`, `trade.go:409-413`), a
  partner who stays short gets the cancel at `Timeout`, and one who confirms while short gets the
  cancel at once (the confirm locks the offers below the price).

**Asking for a trade.** A partner who whispered in this round and stands in sight (in the bot's
known list, which the server requires) within `InviteReach` (1200) of the bot, and whose position
is within `RoundReach` less 100 of the round's anchor (section 3, the bot's position at the
round's start, which `openRound` records in the round state, section 2; not the town point, which is a gatekeeper spawn, `storeregistry.go:114-131`): the
bot walks to within 100 of it (the walker, inside the 150 confirm range) and sends `TradeRequest`
once. The walk's end thus stays inside the round's area, the same one the private keeper is
chosen in (section 6), and once the trade has ended or the request was refused or expired the bot
walks back to the anchor (section 3, "Back to the anchor") before its next ad line or invite; a
partner standing outside it is answered by whisper only (`trade me`), and
a request of its own is still accepted. It waits for
`TradeStart` up to 15 s (the request expiry). On any refusal the server sends (server table,
"Trade request": 119 denied, 153 busy, 144 target incorrect, 340 or 1065 the partner is setting up
or running a store, 940 or 619 a block, 794, or 1987 with the olympiad or chaotic text) or no
answer, it asks that partner no more this round. At most one request is open (the server refuses
a second with 142); none while the store is open or opening, and none within 15 s
(`RequestTimeout`) of the round's end as it stands (`RoundWait` after the first line, or the
`Grace` end once extended), nor during a private warehouse request of the round. An unexpired
request the bot sent counts on the server as a transaction in progress, which refuses the keeper's
links (its `Chat` links and the private links), the deposit request and a store opening (server table, "Warehouse request gate" and "Store
opening during a request"); so does any other unexpired request of the server's one request slot,
sent or received (party, party room, command channel, friend, duel). This cut-off keeps the bot's
own request from refusing the round's deposit or the store opening after it; the deposit and the
store opening still wait for the bot's own request left unanswered and for a trade prompt, rule
"prompt" answers a party invitation first, and a request the bot cannot see fails the deposit
like any failed deposit (section 3, steps 5 and 6).

### 6. The private warehouse

The warehouse errand (`warehouse.go`) gains a `Private` request flag:

- links `_DepositP` and `_WithdrawP` instead of the clan ones; lists of type 1 (a type 2 list is
  not taken for a private request); 282 is `WarehouseEmpty`;
- a private request slot and no clan needed: today `startWarehouse` returns early unless
  `Persona.Warehouse.Want` is set or the crafter's cycle runs, and unless the bot is in a clan
  (`if (p.Want == nil && !b.crafting()) || ... || b.world.Clan.ID == 0 || ...`,
  `warehouse.go:588-590`), and it takes its request only from `warehouseWant`: the crafter's
  cycle while it runs, else `Persona.Warehouse.Want`, else none (`crafter.go:214-224`). A
  valuables persona has no `Want` of its own, so as written no private request could start. V3a
  adds a slot on the errand, `e.privateWant`, holding at most one queued `WarehouseRequest` with
  `Private` set; rule "valuables" fills it with the round's look, withdrawal or deposit and with
  the off-round errands (section 3), then calls `startWarehouse` and `stepWarehouse` itself.
  `startWarehouse` changes in two places: its guard returns early only when there is no `Want`,
  no crafter cycle and no queued private request (the travel map, keeper, `openDue` and `retryAt`
  checks stay), and `Clan.ID == 0` refuses only a clan request; and it takes the queued private
  request before it asks `warehouseWant`, clearing the slot when the request starts;
- the keeper of a private request is chosen by a new rule, in the round's area: the nearest
  warehouse keeper in sight, not marked `noPrivate`, whose position is within `RoundReach` (6000)
  of the anchor (the bot's position when the round, the look or the off-round errand started,
  section 3), else the nearest keeper spawn (the travel map's keepers) not marked `noPrivate`
  within that reach of it; none there fails the request (`WarehouseUnreachable`), which
  `roundReady`'s keeper check makes rare. The errand does not read
  the round: V3a adds the area to the request itself, an `Anchor` (a position) and a `Reach` on a
  private `WarehouseRequest` (today it carries only `Withdraw`, `Deposit` and `Look`,
  `warehouse.go:176-179`), and the keeper rule reads them from the running request; rule
  "valuables" (V3b for the look, V7a for the round and the off-round errands) fills them from the
  anchor and `RoundReach`, a persona default that V2b adds to `ValuablePolicy`. So V3a stands on its own, and its test sets
  the anchor and reach on the request. The bound is the anchor,
  not `storeTownRadius` (3000, `storeregistry.go:12`) of the town point: the town point is a
  gatekeeper spawn, and in Dion, Gludin, Dark Elven Village and Arisha no private keeper
  stands within 3000 of it (section 3, `roundReady`), so a town-point rule would end every private
  request there `WarehouseUnreachable`. Today's rule, which a clan request keeps, is the nearest
  keeper in sight anywhere, else the nearest spawn within `Reach` (8000) of the bot
  (`startWarehouse`, `warehouse.go:585-637`, `Reach` at `warehouse.go:244`); it is not bounded by
  where the round stands. The walk and `warehouseChats` are today's, and so is the request's end
at the keeper: the errand walks the bot there and not back, and rule "valuables" walks it back to
the request's `Anchor` once the request ends (section 3, "Back to the anchor"), so V3a adds no walk. The private link is looked for on the keeper's first page and
  then on its `Chat` pages: most keepers show it at once (19 first pages carry `_DepositP`, e.g. Wilford, 30005), the
  others on `Chat 1` (11 second pages, e.g. `30083-1.htm`);
- the errand's `noClan` mark, which leaves out a keeper whose pages show no clan link
  (`warehouseLink`, `warehouse.go:440-470`; read by the keeper choice, `warehouse.go:607`), is
  neither set nor read for a private request; a keeper whose pages show no link the request
  needs (`_WithdrawP` for a look or a withdrawal, `_DepositP` for a deposit) gets a mark of its
  own, `noPrivate`, keyed by NPC template like `noClan`. That is 20 of the 50 spawned keepers
  (server table, "Private warehouse links"): the 18 freightmen, whose `Chat 1` page links only
  freight, and the alliance keepers Jaff and Hagos, whose script page shows a non-allied player
  no link (an allied player gets `_DepositP` and a quest withdrawal, so a look still marks
  them). In seven towns the freightman is the keeper nearest the town point (section 3,
  `roundReady`), so the first private request there goes to it. Two changes keep the
  mark from costing a wait or a visit per session. First, a request ended by a new `noPrivate`
  mark ends `WarehouseUnreachable` with no `retryAt`: no open went out and the server refused
  nothing, so the next decision queues it again and the keeper rule takes the next unmarked
  keeper (the walk back waits for that request, section 3); this repeats at most once per keeper
  within `RoundReach`, and when none is left the town counts as having no private keeper for
  `roundReady`, the look and the pending round. Second, the marks outlive the session: the
  errand's `noPrivate` set is a field V3a adds, and V3b gives the errand the set kept in the
  `valuables` value (section 2), which the `Bot` hands to each session's brain, so a session
  after a reconnect neither walks to a marked keeper again nor waits `retryAt` for it. The marks
  are per bot and per process, not shared between bots: each `Bot` hands its own `valuables`
  value to its sessions (section 2), so each merchant marks a keeper once, and a process restart
  learns them again, once per keeper and bot. A 142 does not set the
  mark (next bullet);
- a refused link is not a missing link: during a transaction in progress the server answers every
  bypass to a keeper, its `Chat` links included, with 142 (server table, "Warehouse request
  gate"), and today the errand ignores a system message at its talk step (`warehouse.go:675-676`),
  times out, talks again, skips the `Chat 1` link it already followed and marks the keeper
  (`warehouse.go:451-468`) for the rest of the session. In a town whose only keeper within
  `RoundReach` keeps its private link on `Chat 1` (11 keepers), that mark would end every later
  look, round withdrawal and deposit there. So V3a takes a 142 at the talk step, after a link went
  out, as a refusal: the request fails with `WarehouseRefused` (`fail`, which sets `retryAt` 60 s
  on, `warehouse.go:350-360`, since no open went out), and neither `noPrivate` nor `noClan` is
  set. A clan request gets the same handling, since the cause is the same;
- the fee is 30 adena a row, counted before the request (`depositRows` already checks it). Today
  `reserve()` covers only `KeepAdena`, the trip home and `memberFees` (`budget.go:36-44`); V7a adds
  a `valuableFees` term for a persona with valuables enabled: 30 adena times the larger of
  `HoldRows` and the carried valuable rows (the rows the next deposit would take, released ones
  included once the valuables budget is spent), so no shop visit spends the deposit fees.
  `HoldRows` alone does not bound a deposit (section 7, "What bounds the stored rows");
- a status of its own: today one `WarehouseStatus` (`Contents`, `Known`, the last `Withdraw` and
  `Deposit` results, `warehouse.go:226-230`) sits in the errand (`status`, `warehouse.go:287`),
  and the clan code reads it: the withdraw list and 282 set `Contents`/`Known`
  (`warehouse.go:672,679-682`), `trackContents` applies moved rows to it (`warehouse.go:715-736`),
  `depositRows` counts free slots from it (`warehouse.go:545-546`), and the clan member reads its
  contents and results (`clanmember.go:170,221,428,439`). The `merchant` preset is a clan member
  (`member: true`, `cmd/bots/persona.go:46`). V3a adds a second field, `private`, and every place
  above uses the status of the running request's kind (one accessor, `e.statusOf(req)`), so a
  private look, withdrawal or deposit never touches the clan view and `Persona.Warehouse.Want`
  keeps getting the clan status. The valuables ledger reads `private` (its stored units, section
  2), so for a private deposit `trackContents` also copies the deposited item's `ObjectID` and
  `Enchant` into the row it appends (today `WarehouseItem{TemplateID, Count}` only,
  `warehouse.go:734`; the object stays the same for a whole non-stackable unit, section 2, "Rows").
  This status is the brain's, so it is not known at a session's start; the ledger's look fills it
  (section 2);
- slots: `depositRows` counts a private deposit against the private contents and the private
  size, 100, or 120 for a dwarf (the shipped `WarehouseNoDwarf`/`WarehouseDwarf`,
  `model/actor/player/character_storage_limits.go:23-33`; the `whLimit` stat the bot cannot see
  is left out, which only undercounts), not `clanWarehouseSlots` (150, `warehouse.go:40`). The
  valuables do not stay within `HoldRows` (6) rows at all times (section 7, "What bounds the
  stored rows"), but the bot deposits nothing else in its private warehouse, so they stay far
  below its 100 slots, and a deposit past the free slots fails like any failed deposit;
- a private request takes the errand ahead of the clan member's and the crafter's requests, which
  wait as they wait for any running request: `startWarehouse` takes the queued `privateWant`
  before it asks `warehouseWant` (above). A crafter's cycle does not meet it: `roundReady` and the
  off-round errands require no crafter cycle (section 3). Private and clan requests share the errand's one open
  gap: `openedAt`, the `timeouts` backoff (60 s doubled per unanswered open in a row, at most
  30 min) and `retryAt` (`warehouse.go:281-297,302-311`), since the server's request gate does not
  tell them apart; `warehouseOpenGap` (60 s) holds between opens except the deposit after its own
  withdrawal. A round therefore starts only when an open is due, and queues its deposit only when
  one is due (section 3), so it never waits out a backoff in town.

### 7. The cap: everything to the store (D33)

The cap is checked only at the end of a round that advertised (step 5: after the rounds and asks
are updated, before the deposit; a round cut off by `called()` after its first line runs it too,
section 3; a round that ended at `FirstLineWait` or was cut off before its first line runs none), so every
valuable gets at least its first round before any release. The bot then **releases** its
valuables to its store when, counting carried and stored:

- its entries take more than `HoldRows` (6) warehouse rows (all are released; a row as in section
  2: each unit of a non-stackable template, one per stackable template), or
- the asks of the entries that each ask at most `HoldValue` (2000000 adena), times their units,
  add up to more than `HoldValue` (all of those are released), or
- one entry went unsold through `MaxRounds` (3) rounds (that one alone is released).

The counts are the entries' derived units (section 2), so a valuable sold by trade or from the
store, or otherwise gone, no longer counts.

**What bounds the stored rows.** The cap runs only at the end of a round that advertised, so
`HoldRows` bounds the held valuables only right after such a round: the unreleased ones then take
at most `HoldRows` rows, and that round's deposit at most that many. Between those points the
rows can grow past it: a round that ended at `FirstLineWait` deposits with no cap (section 3, step
5); once the valuables budget is spent no round starts (section 3, Start), so every later find of
the session goes in through the off-round deposit with no cap; and released stored rows stay
until the release withdraw, which does not run once the budget is spent. The next round that
advertises, in this session or in a later one with a fresh budget, withdraws every unreleased
stored valuable and runs the cap on all of them, releasing them all when they take more than
`HoldRows` rows. So nothing rests on `HoldRows` as a hard bound: the fee reserve follows the
carried rows (section 6, `valuableFees`), and `depositRows` checks the free private slots on every
deposit (section 6).

A valuable whose ask alone passes `HoldValue` does not count toward the sum and never sets it off:
one big find does not dump the others to the store. It gets its rounds like any valuable and is
released alone after `MaxRounds`. Example: Scythe (96), reference price 2290000
(`items/0000-0099.xml`, item 96), a DROP of monsters and raid bosses of level 38 to 46 (e.g. NPC
25394, level 38, and 20235, level 45, in `npcs/*.xml`), as are Cursed Dagger (226) and Orcish
Glaive (298) at the same price.

A released valuable that is carried at the round's end is not deposited (step 5); one that is
stored is withdrawn while the valuables budget is not spent, by the next round's withdrawal
(step 1) or by the rule's off-round withdraw errand (section 3), which holds the store opening
back under `StoreHold` while it waits for its open; once the budget is spent, the released one
stays stored for a later session. It needs no round, and is listed by the store opening ahead
of the ordinary loot, at its floor (section 5: the worth `W`, the enchant
scrolls included, times `FloorPct`). That needs three changes to the store, all in V7b:

- `storeReady` returns false once the store budget is spent (`store.go:358`); it now holds back
  only loot, and an opening also starts while the bot carries a released valuable the valuables
  budget allows (section 8). `storeSells`, the loot check at its end, also accepts a released
  valuable.
- `storeRows` takes released valuables first, each at its floor, by the valuables budget's
  listing rule (section 8), not by the store's `left := p.Budget - st.sold` and
  `n := min(count, left/price, room/price)` (`store.go:466,494-498`), which would never list a
  valuable priced above the budget left; then loot against the store budget as today, within
  `MaxRows` (4, `store.go:69`) and the int32 room the server takes.
- At the default `FloorPct` (100) the floor is the worth `W`, so at least the reference price,
  i.e. at least twice the NPC buy-back: a released valuable's row is priced outside
  `StorePolicy`'s markup band (`MaxMarkup` under 200, `store.go:50-53`); D33 names the plan.md
  amendment.

The store registry's town limit and spacing apply as today, so a full town keeps them carried until
a place frees. Released valuables stay listed until sold, like the rest of the store.

### 8. The valuables budget (D34)

Valuable sales, by trade and by store, count in a per-session `Valuables.Budget` (flag
`-valuables-budget`, default 2000000), not in the store's `-store-budget` (50000): a single
valuable is worth more than a whole store session of loot, and would otherwise never list
(`storeRows` lists no row whose price exceeds the budget left, `store.go:466,494-498`).

The rule is not the store's. With `left = Budget - sold` (`sold` the session's valuable sales so
far):

- **Trade:** a valuable is offered while `left > 0`, at most `ceil(left / unit price)` units of
  its stack (one, for a single item).
- **Store listing:** an opening lists released valuable rows in order (highest score first) while
  `left > 0`, a row's units at most `ceil(left / floor)`, then takes the listed units' value off
  `left` before the next row: a row lists only while the value listed before it is under
  `Budget - sold`. Example: with `left` 1000000 and three released rows of one item at 600000,
  the first two list and the third waits for the next opening.

Either way, every unit offered or listed starts under `Budget`, so a valuable priced above it (the
Scythe, 2290000) still sells once in a session, and the session's valuable sales stay under
`Budget` plus one unit's price, since only the last unit may pass it. Once `sold` reaches
`Budget`, no valuable is offered or listed for the rest of the session; they stay held, the
carried ones, released or not, are deposited by the rule's off-round errand, and no released
stored one is withdrawn (section 3).

Counting: a trade counts its price on `SendTradeDone` with success. The store counts one total
today: `countSold` adds the adena gained since the list to `store.sold` when the store closes, with
no split by row (`countSold`, `store.go:547-556`). V7b adds the split: the opening keeps each listed valuable
row's object, count and price; at the close, the units a valuable row lost (its listed count less
the count still carried) times its price count in the valuables budget, and the store budget gets
the total less that. The ledger needs no step of its own for either sale: the sold units leave the
inventory, so they leave the entry's carried count, and an entry with no unit left is dropped
(section 2).

### 9. Wiring

- `ValuablePolicy` in the persona: `Enabled` and every default named above; zero disables it. Only
  the `merchant` preset enables it (`cmd/bots/persona.go`), and the preset gets the shared ad board.
- Metrics: `valuables_ads_total`, `valuables_replies_total`, `valuables_sold_total{via="trade|store"}`,
  `valuables_deposits_total`, `valuables_released_total`, and a gauge of held valuables.
- The panel shows a merchant's held valuables (name, place, rounds, ask) in its bot view.

### Persona defaults

| Field | Default | Section |
| --- | --- | --- |
| `PriceFloor`, `ValueMin` | 10000, 100000 | 1 |
| `CommonSupply`, `RMax`, `CraftDivisor`, `EnchantRare` | 100/h, 4, 2, 0.25 | 1 |
| `RoundGap`, `RoundWait`, `Grace`, `FirstLineWait` | 30 min, 5 min, 2 min, 2 min | 3 |
| `RoundReach`, `StoreHold` | 6000, 2 min | 3 |
| `AdEvery`, `AdLines`, `RegionGap`, `ProcessPerHour` | 2 min, 3, 60 s, 30 | 4 |
| `MaxAsk`, `AskPerRarity`, `FloorPct`, `RoundStep`, `Counters` | 150, 12.5, 100, 10, 2 | 5 |
| `Replies`, `RepliesPerPartner`, `InviteReach`, `AnswerAfter` | 20, 4, 1200, 30 min | 5 |
| `HoldRows`, `HoldValue`, `MaxRounds` | 6, 2000000, 3 | 7 |
| `Budget` | 2000000 per session | 8 |

All are persona defaults (D6), except the ad board's `RegionGap` and `ProcessPerHour`, which are
operational limits on what the process says, set by flags, not by a persona.

The supply ceiling's inputs are process settings, not persona fields (section 1): they shape the
one `TravelMap.supply` table every bot shares, so `NewTravelMap` takes them in its `SupplyConfig`.

| Process setting | Default | Set by |
| --- | --- | --- |
| spawn events | `extra_mob;18age;start_weapon` (the shipped `SpawnEvents`) | flag `-spawn-events`: it mirrors the server's config, which the bot cannot read |
| `KillFloor`, `NoRespawnCycle`, `SpoilWeight`, `SevenSignsShare` | 60 s, 24 h, 0.2, 0.5 | `SupplyConfig` defaults, no flag |

## Pacing

| Sender | Bot bound | Server limit it stays inside |
| --- | --- | --- |
| Trade ad (Say2, channel 8) | per bot: one per `AdEvery` (2 min), 3 a round, a round per 30 min (at most 6 an hour); per region: one per 60 s across the process; process: 30 an hour; the chat gate (3 s) and the frame bucket | `TradeChatTime` (0 shipped; a gate of up to 2 min is still met per bot, and a dropped line is silent, so the bot never relies on it), 100 UTF-16 units |
| Whisper reply (Say2 tell) | 3-12 s after the whisper, then the chat gate (3 s); 4 per partner and 20 per round, rounds 30 min apart (about 40 an hour), the replies within `AnswerAfter` after a round counted under that round's caps; one queued reply per partner | none (whispers have no flood gate, `network/chat.go:144-173`) |
| `TradeRequest` | one open at a time, one per partner a round, 15 s wait | the server's busy refusal (142) and 15 s expiry; no flood gate |
| Trade window (`AddTradeItem`, `TradeDone`) | unchanged: one add and one confirm per trade | unchanged |
| Walk to a buyer | the walker's legs through the frame bucket (5/s, burst 10) | 160 frames/s, 400 per 5 s (`pacing.go`) |
| Walk back to the anchor (after each keeper visit and each walk to a buyer) | the walker's legs (`goTo`) through the frame bucket (5/s, burst 10); one walk per keeper visit or buyer, at most `RoundReach` (6000) long | 160 frames/s, 400 per 5 s (`pacing.go`) |
| Private warehouse (bypasses, two requests) | one look per session, then a withdrawal and a deposit per round; off-round, a release withdraw while released valuables are stored and the valuables budget allows, and a deposit of the carried valuables once the budget is spent (one open each, the deposit at most once per new find); every open at least `warehouseOpenGap` (60 s) after the last but the deposit after its own withdrawal, `retryAt` (60 s) and the timeout backoff (up to 30 min) after a failure; `bypassGap` (500 ms), the reply timeout (5 s); a keeper visit that ends on a new `noPrivate` mark sends no open (a talk and its `Chat` clicks, at most `visitClicks`) and repeats at most once per keeper template per bot and process (N merchants in one town visit its freightman up to N times), the marks kept across sessions of that bot | `ServerBypassTime` (100 ms), the silent request gate |
| Store | unchanged | unchanged |

## Owner decisions

Proposed, awaiting the owner:

| # | Question | Proposal |
| --- | --- | --- |
| D30 | May a bot speak on Trade or Shout? | The owner asked for "a global message"; the server has none for a bot: Trade and Shout are heard only in the speaker's restart region (`network/chat.go:108-122`), and Hero Voice, the only channel heard server-wide, drops a non-hero's line (`network/chat.go:123-128`). So an ad reaches the players of the bot's own region only (a merchant advertising in Giran is not heard in Aden). Proposal: the `merchant` preset may advertise its valuables on **Trade** only (channel 8, heard in the speaker's restart region), at the per-bot, per-region and process caps of section 4; never on Shout. This amends the `docs/plan.md` section 4 bullet (lines 343-344) in two places: "never on Shout or Trade" gains this exception, and "Chat only when answering a whisper" gains the ads, the only unprompted lines a bot says. The alternative is Trade and Shout both, under the same caps. |
| D31 | What makes an item valuable? | The score of section 1: reference price times `2^R`, `R` from the server-wide supply ceiling (drops times spawns over the respawn cycle; the Seven Signs catacomb and `_none` seal makers at half, seasonal event makers not at all), 0 for items a spawned NPC multisell makes from non-gear ingredients, halved for craftable items, raised by enchant past the safe level; valuable at score 100000 and price 10000. Multisell upgrade rows (which consume another weapon or armor) do not make an item common, a refinement of "multisell items are never rare". A second refinement of the same kind for shops: an item an NPC buylist sells is never valuable, but only buylists of an NPC id above 0 count (`cheapest`, the test `storeLoot` uses; `indexShops` skips the rest, `shop.go:283-285`). The 212 `npcId="-1"` lists, which no NPC sells from, are left out, and they carry all four valuable rows of the section 1 table (433 in list 1001, `buyLists.xml:9408,9477`; 309 in 2012, `:10437,10442`; 121 in 9001, `:11612,11620`; 184 in 9017, `:11868,11872`): read literally, "items sold in buylists are never rare" would make none of them valuable. A valuable comes before the clan member's hold (section 1): a member merchant sells a valuable recipe scroll or valuable graded gear to players instead of depositing it for the clan crafter. The alternative is the member's hold first, so a member merchant whose clan has a crafter never sells a graded valuable or a valuable recipe scroll (two of the four valuable rows of the section 1 table, Conjuror's Staff and Elven Tunic, are D grade, and Recipe: Halberd is a recipe); the two with no grade, Sword of Watershadow and Tears of Eva, are not in the member's hold either way. |
| D32 | How does a bot price and sell a valuable? | Ask 100-150 % of the reference price by rarity, plus the reference price of the enchant scrolls on the item, lowered 10 points per unsold round; floor the worth (reference price plus the enchant scrolls) or the store price, whichever is higher; two counters per buyer and item a round; no sale below the floor, so an enchanted item never goes for its bare reference price. The bot may ask a whisperer in sight for a trade (one request per buyer a round). This amends two `docs/plan.md` trade defaults: "trades are fixed-price sales of the bot's own goods for adena" (line 328) gains that a valuable's trade price is haggled between ask and floor; and "rate-limit trade requests per partner" (line 332), which today covers only requests a player sends (the bot never sends one, `trade.go:139`), gains that the bot may send one, at most one per partner a round and one open at a time. It also amends the section 4 chat bullet (lines 343-344, "rate-limited per sender and capped per bot in total") for whisper replies during a round and for `AnswerAfter` (30 min) after it to a whisper naming a held item (no offer, no trade outside a round): up to 4 per partner and 20 per round (rounds 30 min apart, about 40 an hour), outside the canned reply's caps of one per sender per 10 min and 5 an hour (`chat.go:32-40`), still after a 3-12 s delay, still never to a marked name. |
| D33 | Hold, warehouse and cap | A 5 min round of ads, then the private warehouse (30 adena a row); a new valuable brings the stored ones out for another round together; after a round, everything goes to the town store once the held valuables take more than 6 warehouse rows or the asks of those asking at most 2000000 pass 2000000, and an item unsold through 3 rounds (or one asking more than 2000000 alone, after its 3 rounds) goes alone. The cap runs before the round's deposit, so a released item that is carried is never deposited. Released valuables list at their floor, the worth (reference price plus the enchant scrolls, so a +5 item lists at its scrolls' worth too), at least the reference price, i.e. at least twice the NPC buy-back: this amends the `docs/plan.md` merchant default "price between 1.2 and 1.5 times the NPC buy-back price" (line 321), and `StorePolicy.MaxMarkup` (under 200) does not bound these rows. |
| D34 | Which budget bounds valuable sales? | Their own per-session budget, `-valuables-budget`, default 2000000 adena, apart from `-store-budget`. Valuables are offered and listed while the session's valuable sales are under it, so one sale may pass it by its own price (an item priced above it still sells once a session). |

## Testing

- **Datapack (unit, fixtures):** the drop reader keeps each category's kind and chance and each
  drop's item, min, max and chance, and fails on a mangled value; the NPC reader keeps a template's `<private>` rows and the spawnlist reader an entry's; the
  spawnlist reader keeps a maker's `<ai type>`, its `<set>` params (`EventName`, `IsNight`,
  `on_start_spawn`, `maker_name1`-`maker_name3`, `maker_cnt`) and its `spawnTime`; the multisell reader keeps a
  row's NPCs, ingredients and productions; the restart reader keeps each point's map tiles; the
  enchant scroll table by grade and kind. `TestLoadShippedDatapack` and a
  `TestLoadShippedValuables` (both under `BOT_DATAPACK_XML`, skipped without it, as
  `datapack_test.go:215-221` does) load the shipped files.
- **Score (unit, table-driven):** `TestValueScore`: a shop-sold item is no loot; a non-gear
  multisell product has `R = 0`; an upgrade row is no source; a recipe halves `R`; a raid-only
  drop is rare; SPOIL counts at 0.2; a maker with no event or a `SpawnEvents` event counts
  fully, an `ssq_event` or `ssq_seal*_none` maker at 0.5, a `_dawn` or `_twilight` seal maker and a
  seasonal event maker (`medal`) nothing; an `event_maker` with no `event=` is gated by its
  `EventName` param (`extra_mob` counts fully, `christmas` nothing, and a changed spawn event
  list flips them); a maker with a `spawnTime` or `on_start_spawn` 0 and a `no_on_start_maker`
  count nothing; a `random_spawn` maker of three equal entries counts each at a third; an
  `on_day_night_spawn` maker counts at 0.25 by night and 0.75 by day; a `maker_instant_spawn`
  group of three NPCs comes back every `3 × KillFloor` whatever its entries' `respawn`; a
  `maker_instant_spawn_random` pair with one starter counts each maker at half, at the cycle
  `respawn + KillFloor × N`; a type with a maker of its own the rules do not name counts like
  `default_maker`; no source is `R = 0`; `respawnRand` does not change the
  cycle; a `respawn="0sec"` and a `respawn="no"` entry count once per `NoRespawnCycle`, not once
  per `KillFloor`; a drop of a private row only counts at its leader's cycle (own `respawn` 0) or
  its own shorter one, with its leader maker's share, and is not `R = 0`; a private row under an
  entry of `total` 2 counts twice; an entry with no `<private>` rows takes its template's, and an
  entry with rows of its own does not add its template's; +5 on a graded weapon adds five prices
  of the normal scroll (not the blessed or crystal one) and 0.5 to `R`; +5 on an item with no
  grade adds no scroll price (`W = ref`) and still 0.5 to `R`; the thresholds; `S` comes from
  `NewTravelMap` with a `SupplyConfig` (a changed `KillFloor` or spawn event list changes it),
  while two personas with different `CommonSupply` or `ValueMin` over one shared map score one
  item differently. `TestValuablesShipped`
  (`BOT_DATAPACK_XML`): the six rows of section 1's table. `TestRestartRegion`: the copied tile
  formula against fixed points of the server's (Talking Island town, a point in no region).
- **Brain (unit, events and an injected clock in, packets out):**
  - `TestValuableClassification`: the bot's own keeps and the persona's own keep hook, then gear
    worn next, then valuable, then the clan member's hold, then loot; a held valuable is in no NPC
    sale (`junk`), no `storeRows`, no `tradeGoods` outside a round (a round opened with
    `openRound` is not needed for these). For an active clan member (a clan with a crafter on the
    board): a D-grade valuable it would not wear and a valuable recipe scroll are valuable, in no
    `memberDeposit` row, not in `memberHandOff`'s weight and not in the fees the supply trip's
    arrival sets; a graded item and a recipe scroll that are not valuable are still kept and
    deposited as today; a valuable with no grade (Sword of Watershadow) is in no member hold
    either way; the valuable test does not recurse through `memberHold`.
  - `TestValuablesLedger` (V2b): a carried valuable adds an entry (rounds 0, the first ask); two
    identical non-stackable valuables (two +0 Swords of Watershadow, two objects) are one entry
    of two units and two warehouse rows, a stackable valuable of 5 one entry of one row; a +0 and
    a +5 of one template are two entries; an `InventoryUpdate` taking a carried unit away (a sale,
    a drop) lowers its carried count, and an entry with no unit carried and the private status
    known with none stored is dropped, while with the private status not known it is kept (V2b's
    ledger reads stored units through one accessor, which reports none stored and known until V3b
    wires it to the private status of V3a; the test sets it to not known); a
    unit found while its entry exists joins it with its rounds and ask, one found after it was
    dropped starts a new entry; `openRound` records the anchor it is given and the round state
    returns it until `endRound`; the walk back sends legs to within `anchorArrive` of an anchor
    and none once the bot is there. Stored units need the private status (V3a), so V3b's
    `TestValuablesLedgerLook` and V7a's `TestValuablesRoundDeposit` check them: the look's rows and a
    deposited row (with its `ObjectID` and `Enchant`) count as stored, a withdrawn one as carried.
  - `TestValuablesRoundStart`: a merchant whose session's look is done arriving in town with
    a new valuable sends the first Trade line (Say2 channel 8) and no
    `RequestPrivateStoreManageSell`; none within `RoundGap`; none with the store open or an
    opening under way; none out of a registry town (a bot within `storeTownRadius` of a town with
    no free store place still starts one); none in region -1; none while the ad board holds the
    region (another bot's line within `RegionGap`, or the process hour cap); none while a
    warehouse request, a market visit, a gear `UseItem`, a craft cycle or an enchant exchange runs
    (with the scroll selected, `enchantChosen`, the exchange's `RequestEnchantItem` goes out and
    its result is taken before the round starts, and no `_WithdrawP` or `_DepositP` goes out
    meanwhile); for a clan member
    arriving with its supply trip's clan deposit just ended (`member.asked` set), none until rule
    "member" has settled it (the withdrawn rows taken off the board, the lease given back, the
    ore shot orders posted), and the round starts on a later decision; none while the
    warehouse open is not due (within 60 s of a clan open, or in the backoff after an unanswered
    one) or before `retryAt`, and the round starts once it is due; a valuable left carried by a
    cut-off round or a failed deposit starts the next round `RoundGap` later with no new find;
    stored valuables with none carried start no round; no ad line names a released carried one
    (that the round's `_WithdrawP` also takes the released stored ones is V7b's, in
    `TestValuablesRelease`). Dion (`town` at
    keeper 30058's spawn finds no town): a bot at Dion's town point (Trisha's spawn) with the
    session's look not done looks at keeper 30058, 5348 away (within `RoundReach`), walks back to
    within `anchorArrive` of where it started, and only then starts the round, so `roundReady`'s
    town check passes; the round's withdrawal goes to 30058 too, and the first Trade line goes
    out only after the walk back, at the anchor; at the round's end the deposit goes to 30058,
    the bot walks back, and the store opening after it sends `RequestPrivateStoreManageSell` at
    the anchor with the loot (`hasRoom` finds Dion), not at the keeper (the same opening with a
    released valuable is V7b's, in `TestValuablesRelease`, since V7a releases nothing); a
    walk to a buyer is followed by the walk back before the next ad line; a walk back answered
    unreachable after the withdrawal ends the round with no ad and no deposit. A registry town
    point with no private keeper within `RoundReach` starts no round and its store opens with the
    loot only. Talking Island, with Rant (30054, freight pages) marked `noPrivate` by the look:
    the round's withdrawal and deposit go to Rolfe (30055), never to Rant. Teranu Mardui, with
    Hagos (31381) marked by the look: no round is pending or starts, and the store opens at once
    with the loot only (no `StoreHold` wait). Across sessions: none before the session's look is
    done; after a process start the look's stored Sword of Watershadow and +5 Conjuror's Staff
    alone start no round, and a new valuable carried into town then starts a round whose
    `_WithdrawP` takes both and whose first line names all three; once withdrawn, the staff leaves
    the ledger in a variant where the gear errand wears it next. A second session after a
    reconnect, with a valuable deposited in the first: no round starts, no deposit is queued
    before the new session's look, and a new valuable carried into town in that session is
    deposited after its round and counts as stored. A round open when its session ends: the next
    session ends it as a cut-off (after its first line the rounds and asks step; before it nothing
    changes), with no reply or request carried over. V7a has no cap, so the look's rows in the cap
    and the cut-off's cap waiting for the look are V7b's (`TestValuablesRelease`).
  - `TestValuablesStoreHold` (V7a): with a valuable carried and a round pending, the ad board
    held by another bot's line: no `RequestPrivateStoreManageSell` and no move; the board frees
    within `StoreHold`: the round starts (the first Trade line) and no store opens until the round
    has ended; the board held past `StoreHold`: the store opens with the loot only (the valuable in
    no row); the warehouse open in its backoff likewise; a market visit running: the market
    errand steps to its end and no store opens meanwhile; a valuable carried within `RoundGap` of
    the last round's start: the store opens at once; leaving the town and coming back restarts the
    `StoreHold` count; a ledger look running does not count against it; with the board held past
    `StoreHold` and no loot to sell, the rule stops holding the turn at `StoreHold` and a due
    errand (a supply trip) starts, no store opening; with the warehouse open in a 30 min backoff,
    the turn is held for `StoreHold` only; with the session's look not done and the board held,
    the look still starts once the warehouse open is due (it does not wait on the board), and a
    look waiting for the open counts against `StoreHold` like the other short waits; in Dion the
    keeper visit of the look does not reset the `StoreHold` count.
  - `TestValuablesFirstLineWait`: a round whose first line the board holds past `FirstLineWait`
    ends with no Trade line, deposits, and leaves the items' rounds and asks unchanged.
  - `TestValuablesHoldsTurn`: during a round, a due gear, market, shop, supply or clan warehouse
    errand does not start and the bot does not move off; the deposit's private request runs to its
    end while rule "warehouse" is not reached; a `follow` ends the round at once with no deposit:
    after the first line it steps the items' rounds and asks, before it nothing changes. A
    `follow` (and, in a variant, the party leader moving off) while the round's deposit walks to
    a keeper 5348 away (Dion): the private request ends on that decision, no `_DepositP` and no
    `RequestWarehouseDeposit` go out, no walk back follows, and the next decision's legs are rule
    "follow"'s toward the commander, not toward the keeper; the valuables stay carried and no
    `retryAt` is set. A `follow` with the deposit's list open (`whListed`): no deposit request goes
    out. A `follow` after `RequestWarehouseDeposit` went out (`whSent`): the bot sends no legs
    until the `InventoryUpdate` (the ledger then counts the units stored) or the 5 s timeout, then
    follows. A `follow` during the round's withdrawal likewise ends it, the valuables staying
    stored. The cap at that cut-off is V7b's and is tested in `TestValuablesRelease`.
  - `TestItemLabel` (V2b): names (every scored template has one in `itemNames`, not only recipe
    products), enchant and grade, an entry of two units as `2x <name>`, `k` and `kk` rounded up to
    the shown unit, never below the ask.
  - `TestAdText` (V4): a line of several labels, the 100-unit clip with the rest on the next line
    in score order.
  - `TestAdPacing` (rounds opened with `openRound`; the lines come out of `decide` through rule
    "valuables"): two brains sharing one `AdBoard` in one region say one line per 60 s between
    them, also when both decide at the same instant (one claims, the other says nothing that
    decision); a claimed line the frame bucket holds goes out later with no second claim; `AdEvery` and `AdLines` per round; the process hour cap; the chat gate.
  - `TestValuablesWhisperHaggle` (a round opened with `openRound`): a price question gets the ask; an offer below it gets a counter,
    a second below the counter gets the last counter, a third at the floor is agreed, one below the
    floor is refused; for a +5 valuable the floor includes the five scrolls, and an offer at its
    bare reference price is refused after the counters; a marked name, a partner's fifth reply and the 21st reply get nothing; each
    reply goes out 3-12 s after its whisper; two partners whispering 1 s apart both get a reply
    (the second is not dropped), and a partner's second whisper before its reply replaces it; the
    replies do not count in the canned `chatLog`; the canned reply is not sent during a round.
    Names: with Sword of Watershadow and Tears of Eva held, "of course" and "out of adena" name
    nothing (a price-less line gets `WTS <names>, trade me`, an offer in it goes to the top-scored
    one), "tears?" names Tears of Eva, "watershadow 150k" is an offer on the sword, "sword of
    watershadow" names it by its full name; with two held scrolls, "scroll 50k" points at both,
    takes no offer and gets both asks; with one held scroll and the sword, "scroll 50k" is an offer
    on the scroll, and "watershadow scroll" names the sword. Prices: with a +5 Conjuror's Staff
    held, "how much for +5 conjuror's staff?" gets the ask and uses no counter (`+5` is no price);
    "2x tears 150k" is an offer of 150000 a unit; with `Recipe: Halberd(60%)` held, "recipe:
    halberd(60%) 300k" is an offer of 300000 (the `60` is no price); "tears 500" (under 1000) and
    "tears 150k 140k" (two prices) take no offer; an offer below half the floor gets the current
    price and leaves both counters. Length: with seven held entries of long names, a price question naming
    several, a line naming none and an after-round price question each get a reply of at most
    100 UTF-16 units listing the top-scored labels that fit, whole, and every reply sent counts
    once in the caps. After the round (ended with `endRound`, the valuable still carried, since
    stored units need the private status of V3a and the deposit of V7a): within `AnswerAfter`, a
    whisper naming the held valuable gets `<name> not now, wts again soon` 3-12 s later, under the
    round's caps, and takes no offer; a whisper heard in the round whose reply is still waiting at
    `endRound` gets the after-round reply, not a haggle answer; an unrelated whisper gets the
    canned reply; past `AnswerAfter` every whisper gets the canned reply. The same reply to a
    stored valuable is V7a's (`TestValuablesRoundDeposit`).
  - `TestValuablesTradeInvite` (a round opened with `openRound` at an anchor the test gives; the legs and the request come
    out of `decide` through rule "valuables"): a whisperer in sight within `InviteReach` gets legs then one
    `TradeRequest`; after SystemMessage 119, 340, 1065 or 940 none more that round; a whisperer
    out of sight none; a whisperer within `InviteReach` but farther than `RoundReach` less 100
    from the round's anchor none; none while a request is open; none within 15 s of `RoundWait`
    after the first line; after the trade ends, a refusal or 15 s with no answer, legs back to
    within `anchorArrive` of the anchor come before the next Trade line or `TradeRequest`.
  - `TestValuablesTradeConfirm` (a round opened with `openRound`, the partner having named a
    valuable or agreed a price in it; and one case with no round open, where the valuable is not
    offered): `TradeStart` brings `AddTradeItem` of the named valuable;
    `TradeOtherAdd` adena under the agreed price brings no `TradeDone`, and the cancel at
    `Timeout`; a partner's confirm while short brings the cancel at once; enough adena within range
    brings `TradeDone(1)`; success counts in the valuables budget, not the store's; a valuable
    priced above the budget left is still offered while sales are under `Budget`, and none once
    they reach it. After the success and the `InventoryUpdate` that takes the sword away, the
    ledger has no entry for it and the round's next ad line (V4's ad step, which V6 depends on)
    names only the valuables still carried. That it counts in no cap is V7b's
    (`TestValuablesRelease`).
  - `TestValuablesRoundDeposit`: at `RoundWait` after the first line the bot walks to the keeper,
    follows its pages to the private link (a keeper with it on `Chat 1`, and Wilford's fixture
    with it on the first page), sends `npc_<obj>_DepositP`, takes the type 1 list, deposits the valuables'
    rows with 30 a row counted; on the `InventoryUpdate` the ledger has them stored, the bot walks
    back to within `anchorArrive` of the anchor, and only then does the store opening start, with
    the loot only, at the anchor; within `AnswerAfter`, a whisper naming the stored valuable
    gets `<name> not now, wts again soon`. An open timeout during the round (the gap doubled) makes the
    round end at its time with no deposit and no wait, the valuables carried; with the valuables
    budget spent, the off-round errand deposits the carried valuables with no Trade line.
  - `TestValuablesGrace`: an open trade at `RoundWait` holds the deposit up to `Grace`; no
    `TradeRequest` within 15 s of the extended end. A request the bot sent and that is still
    unanswered at the round's end holds the deposit until `TradeStart`, a refusal or 15 s after it
    was sent, and then the `_DepositP` link goes out (no 142); a request from a player not yet
    answered holds it likewise; a party invitation received at the round's end is answered by
    rule "prompt" before the `_DepositP` link goes out; the store opening after the round waits
    the same way. A 142 on the keeper's link (a request the bot cannot see, such as a friend
    invitation): at a keeper with its private link on `Chat 1`, the 142 answering the `Chat 1`
    bypass fails the deposit with `WarehouseRefused` and no `noPrivate` mark, and the next round
    (or the off-round deposit) deposits at that same keeper; at Wilford's fixture, the 142
    answering `_DepositP` ends the transaction refused and the next open waits
    `warehouseOpenGap` from that open. Either way the valuables stay carried for the next round.
  - `TestValuablesReadvertise`: a new valuable with two stored ones: `_WithdrawP` first, then one
    Trade line naming all three; the stored ones' asks are `RoundStep` lower.
  - `TestValuablesRelease`: at a round's end, a seventh valuable, or asks past `HoldValue`, or a
    third unsold round release them: the stored ones withdrawn, then listed first in
    `SetPrivateStoreListSell` at their floor, with no Trade line; the cap runs before the round's
    deposit, so a released carried valuable is in no `DepositP` row and pays no fee, and a round
    whose carried valuables are all released sends no deposit; a +5 released valuable lists at its
    reference price plus five scroll prices; a round that ended at `FirstLineWait` releases
    nothing; a round cut off by a `follow` after its first line runs the cap (a valuable's third
    round so cut off releases it), one cut off before its first line runs none; nothing is released before a valuable's first round; one
    valuable asking more than `HoldValue` releases nothing else and goes alone after its third
    round; with the store budget spent, an opening still starts for released valuables and lists
    them only; with `Budget - sold` 1000000 and three released rows at 600000 the first two list
    and the third does not; a row priced above `Budget - sold` lists while `sold < Budget`; none
    lists once `sold` reaches `Budget`; a valuable sold by trade (its entry gone from the ledger)
    counts in no cap; with the budget spent, a released stored valuable is not withdrawn, and the
    off-round deposit takes the carried released ones too. A round with a released stored
    valuable and a released carried one (both from an earlier round's cap): the round's
    `_WithdrawP` rows take the stored one too while the valuables budget is not spent (none with
    it spent), no Trade line names either, a whisper naming one
    gets no haggle answer and `tradeGoods` offers neither, and their rounds do not step at the
    round's end. Dion: the store opening after a round whose cap released a carried valuable
    sends `RequestPrivateStoreManageSell` at the anchor, not at keeper 30058, and lists the
    released valuable first, then the loot. After a process start, the look's stored rows (a +0
    Sword of Watershadow, a +5 Conjuror's Staff) count in the cap of the next round's end; a
    round left open by an ended session and cut off after its first line runs its cap only once
    the next session's look is done. A session whose look finds a released stored valuable
    (marked in an earlier session of the process, with loot to sell): no
    `RequestPrivateStoreManageSell` goes out and the bot does not move during the 60 s until the
    open is due; then the off-round `_WithdrawP` takes it, the bot walks back, and the opening
    lists it first, then the loot. With the warehouse open in a backoff past `StoreHold`, the store
    opens at `StoreHold` with the loot only.
  - `TestValuablesStoreSplit`: a store listing one valuable row and two loot rows closes with the
    valuable and one loot row sold: the valuable's price counts in the valuables budget, the rest
    of the adena gained in the store budget; the sold valuable's entry is gone from the ledger and
    the cap. With the store open, a whisper naming a released listed valuable within `AnswerAfter`
    gets `<name> in my store <price>`; a whisper naming a released valuable that is stored, or
    carried with no store open, gets `<name> not now, wts again soon`, not a canned line.
  - `TestValuablesLedgerLook` (V3b; no round opens before V7a): a fresh session's first town
    visit looks at the private warehouse (282 or a type 1 list) with no valuable carried too, as
    soon as the warehouse open is due, with the ad board held (another bot's line within
    `RegionGap`, or the process hour cap) and within `RoundGap` of the last round; not while a
    warehouse request, market visit, gear `UseItem`, crafter cycle or enchant exchange runs, nor out of a registry
    town or with no private keeper within `RoundReach`; after the look the bot walks back to within
    `anchorArrive` of where it started and lets the turn go. After a process start (a fresh
    `valuables` value, no entries) the look's type 1 list holds a +0 Sword of Watershadow, a +5
    Conjuror's Staff, an augmented valuable template, a quest item and a template under
    `ValueMin`: the sword and the staff each get an entry, rounds 0, the first ask (the staff's
    with its five scrolls), not released, one stored unit; the other three get none. A second
    session after a reconnect, with a stored unit in its entry (the first session's look):
    its entry is kept (rounds and ask unchanged) while the new session's private status is not
    known, and the new session's look finds the stored unit again. Talking Island (the shipped
    spawns of Roxxy 30006, Rant 30054 and Rolfe 30055, Rant's fixture pages `30054.htm` with only
    `Chat 1` and `30054-1.htm` with only `_DepositF` and `_WithdrawF`): the first look talks to
    Rant, follows `Chat 1`, marks Rant `noPrivate`, ends with no `retryAt`, and on the next
    decision, with no walk back between, talks to Rolfe and takes the type 1 list, then walks back;
    the next session's look goes straight to Rolfe, the mark kept in the `valuables` value.
    Teranu Mardui (Hagos 31381 answering a page with no link, as `31381-no.htm`): the look marks
    Hagos, no unmarked keeper is left within `RoundReach`, the bot walks back, and no further look
    runs in that session or the next. A `follow` while the look walks to its keeper ends the look
    request at once (no `_WithdrawP`, no walk back, the next legs rule "follow"'s), and the
    session's look is still not done, so it runs on the next town visit.
  - `TestPrivateWarehouseErrand`: a queued private request starts for a persona with no
    `Persona.Warehouse.Want`, no crafter cycle and no clan, and with a clan `Want` due it starts
    first; with the slot empty and no `Want`, nothing starts; no clan needed; the exact `_DepositP` and `_WithdrawP` links; a
    type 2 list refused for a private request; 282 is `WarehouseEmpty`; 279 is `WarehouseShort`;
    links found on a first page and on `Chat 1`; a private request neither sets nor reads
    `noClan`; a 142 answering the `Chat 1` bypass fails the request `WarehouseRefused` with
    `retryAt` 60 s on and marks the keeper neither `noPrivate` nor (for a clan request) `noClan`,
    and a later request at that keeper follows `Chat 1` again; `stepWarehouse` steps a request without rule "warehouse";
    `cancelPrivate` ends a private request on its walk, at its talk, with its open link sent and
    with its list received, each with no result recorded, no `retryAt`, the private status
    unchanged and `openedAt` kept (a list arriving after it is ignored), leaves a request at
    `whSent` running, and leaves a running clan request alone; for a clan member with a
    known clan view, a private look (a type 1 list, or 282), withdrawal and deposit leave the clan
    `Contents`, `Known`, `Withdraw` and `Deposit` unchanged and fill the private status; a private
    deposit counts free slots against the private contents and 100 (120 for a dwarf), and the
    row it appends keeps the deposited item's `ObjectID` and `Enchant`, so a withdrawal naming
    that object takes it and not a unit of another enchant of the same template; the private keeper
    is the nearest one within the request's `Reach` of its `Anchor` (both set on the request, 6000
    as `RoundReach` will be), a nearer keeper in sight farther from the anchor is not taken, and
    none within it is `WarehouseUnreachable`; Dion: from an anchor at Trisha's spawn
    (`spawnlist/20_22.xml:620`) with `Reach` 6000 the keeper is 30058 (`:551`, 5348 away), not
    refused. A freight-only keeper: with Rant (30054) nearest the anchor and Rolfe (30055) a
    little farther, Rant's first page showing only `Chat 1` and its `Chat 1` page only
    `_DepositF` and `_WithdrawF`, a private look follows `Chat 1`, marks Rant `noPrivate`, ends
    `WarehouseUnreachable` with no `retryAt` and no open sent, and the next private request,
    queued on the next decision, goes to Rolfe; a keeper in sight that is marked is not taken; a
    clan request neither sets nor reads `noPrivate`. An alliance keeper (Hagos, 31381) answering
    the talk with a page with no link is marked the same way; one answering with `_DepositP` and a
    `Quest ... Withdraw` link (an allied player's page) is marked by a look, which needs
    `_WithdrawP`. With every keeper within `Reach` marked, the request is
    `WarehouseUnreachable` with no walk. A `noPrivate` set handed to the errand (as V3b hands the
    one in the `valuables` value) is read and filled in place, so a new errand given the same set
    skips Rant at once. A clan request keeps today's keeper rule.
- **e2e (live harness, at most 4 characters each):**
  - Characters. No harness client can be the buyer as the harness stands. Every run account's
    character carries the bot marker (`docs/e2e-live.md:145-146`; `character`,
    `internal/e2e/helpers_live_test.go:31-43`, names it through `botnames.DefaultMarker`), and the
    bot replies to no marked name, so it never gets the counter or the bot's `TradeRequest`
    (section 5; `TestLiveChatReplies`, `internal/e2e/chat_live_test.go:12-17,73-78`). The GM,
    the one unmarked character, is a known GM and so trade-exempt: the bot offers it nothing and
    confirms only after it does (`trade.go:272-276,383-393`, `authority.go:294-306`), so no sale
    at a haggled price can settle with it. V9 therefore adds a **buyer**: one run account
    (`<prefix>vb1`) whose character gets fixed candidate names the marker does not mark (a helper
    beside `character` that builds the `botclient.Character` with those `Names` and fails the
    scenario if `botnames.DefaultMarker.Marks` one), and which is no GM, so the merchant's
    `Authority` does not exempt it; the `docs/e2e-live.md` rows name it. The **merchant** is a
    Human Mystic (`character(t, account, &botclient.HumanMystic)`, `character.go:34`), so the
    gear errand, which runs before the valuable test (section 1), wears neither GM-given item: the
    mystic gear entry wields only `BLUNT/MAGIC`, `SWORD/MAGIC` and `BIGBLUNT/MAGIC`
    (`gear.go:219`), and Sword of Watershadow is a sword that is not magical
    (`items/0100-0199.xml:300-302`, pAtk 24, no grade), an upgrade for a fighter but no gear for
    a mystic; Tears of Eva is `weapon_type` ETC (`items/0300-0399.xml:224-227`), which `indexGear`
    leaves out for every class (`gear.go:350`). Each scenario checks at its start that the
    merchant wears neither item after the gift. The buyer accepts the bot's `TradeRequest`, which
    no harness client has done: the harness exports `EncodeTradeRequest`, `EncodeAddTradeItem`
    and `EncodeTradeDone` (`internal/botclient/harness.go:147-154`) but no answer, and
    `encodeAnswerTradeRequest` is unexported (`trade.go:145-150`, used only by `trade.go:181` and
    the unit tests), so V9 adds `EncodeAnswerTradeRequest(accept bool)` to `harness.go`, a
    wrapper like the other three.
  - Keepers. The scenarios run in Talking Island (`townTI`, `internal/e2e/farm_live_test.go:114`,
    beside Roxxy; `toTown`, `helpers_m4_live_test.go:53-56`), where the keeper nearest the town
    point is the freightman Rant (30054, 2317 from Roxxy) and the private keeper is Rolfe (30055,
    2508; `spawnlist/17_25.xml:197-200`). So the merchant's first look of the process talks to
    Rant, follows `Chat 1`, marks it `noPrivate` and goes on to Rolfe at once (section 6); every
    deposit and withdrawal of the scenarios is at Rolfe. `TestLiveValuablesWarehouse` asserts
    that the deposit and the later withdrawal go to Rolfe's object and that no `_DepositP` or
    `_WithdrawP` bypass names Rant's.
  - `TestLiveValuablesTradeSale`: the GM gives the merchant in a town a Sword of Watershadow; the
    buyer in the region gets the Trade line, whispers an offer, gets a counter, whispers the
    counter, accepts the bot's trade request (`EncodeAnswerTradeRequest(true)`), adds the adena; the trade settles at that price
    (inventory counts on both sides).
  - `TestLiveValuablesWarehouse` (with `RoundGap` lowered for the scenario, so the next round
    starts within the scenario's time): the buyer stays silent: after the round the merchant
    deposits the sword (adena down 30, the item gone from its inventory); the GM gives it Tears of
    Eva; on its next round it withdraws the sword and one Trade line, which the buyer hears, names
    both.
  - `TestLiveValuablesRelease`: with `HoldRows` lowered for the scenario, the merchant lists its
    valuables in its store; the buyer buys one from the store.

No unit test waits on the wall clock, and no e2e scenario runs with `-race`.

## Delivery (milestone M12, "Merchant valuables")

Each PR lands behavior-neutral for every preset until V8 enables the policy for `merchant`.

| PR | Scope | Sections | Depends on |
| --- | --- | --- | --- |
| V1 | Datapack: NPC drops and each NPC template's `<private>` rows (id, respawn), multisell rows, the spawnlist's `<private>` rows under each entry (id, respawn; `spawnlist.go:92` skips them today), each maker's `<ai type>` with its `<set>` params and its `spawnTime` attribute (`spawnlist.go:85-89` skips `<ai>` today; the supply needs the type, `EventName`, `IsNight`, `on_start_spawn`, `maker_name1`-`maker_name3` and `maker_cnt`), restart point map tiles, the normal enchant scroll ids by grade and kind (item names need no reader work: `datapack/items.go:238,354` already keep every name); fixture and shipped tests. | 1, 4 | M10 S2 (#321), which also extends `datapack/npc.go` |
| V2a | The supply table on the travel map (`S` and the multisell and craftable marks), with the maker shares by event, start and type, and `NewTravelMap`'s `SupplyConfig` (the `-spawn-events` flag and the supply defaults); the score: `ValuablePolicy` with only its score fields (`PriceFloor`, `ValueMin`, `CommonSupply`, `RMax`, `CraftDivisor`, `EnchantRare`; disabled), the per-persona rarity `R`, the enchant terms and the score `V` over the map's table, called from tests only; the restart region of a position; `TestValueScore`, `TestValuablesShipped`, `TestRestartRegion`. | 1, 4 | V1 |
| V2b | The rest of `ValuablePolicy` (`Enabled`, `RoundReach` and the other persona defaults; disabled); the classification and its precedence in `junk`, `storeLoot` and `tradeGoods`, ahead of the clan member's hold (`wireMember` keeps the persona's own hook, `memberKeep` leaves valuables, section 1); the `valuables` value: the ledger's entries, derived units, rows and lifecycle, the round state with its anchor, `openRound(anchor)` and `endRound`, and the set for the `noPrivate` marks (section 2); rule "valuables" in the rule list as a stepper that matches only while a round is open and opens none, with the walk back to an anchor (`anchorArrive`) that V3b, V6 and V7a call; the travel map's `itemNames` extended from recipe products to every scored template (section 4); the item label (name, `+N`, grade) and the price shorthand (`173k`, `1.2kk`), which the ad composer (V4) and the responder (V5) both use; `TestValuableClassification`, `TestValuablesLedger`, `TestItemLabel`. | 1, 2, 3, 4, 9 | V2a |
| V3a | The private warehouse in the warehouse errand: the `Private` flag, links on any page, list type, the queued private request slot (`privateWant`) and `startWarehouse`'s guard and order (a queued private request starts with no `Want`, no crafter cycle and no clan, ahead of `warehouseWant`), no clan, `noPrivate` apart from `noClan` (keyed by template, set when the pages lack the link the request needs, ending the request `WarehouseUnreachable` with no `retryAt`, the keeper choice skipping marked keepers, the set one the errand can be handed), a 142 at the talk step failing the request with no mark (private and clan requests), look, priority over clan requests; the private status apart from the clan one (list, 282, results, `trackContents` with the deposited row's `ObjectID` and `Enchant`, `depositRows`) and the private slot count; the `Anchor` and `Reach` of a private request and the keeper rule that reads them (within `Reach` of the request's `Anchor`; V3b and V7a pass the anchor and `RoundReach`); `stepWarehouse` split out of `ruleWarehouse`; `cancelPrivate`, which ends a running private request before its request went out with no result and no `retryAt` (section 3, "The cut-off cancels the private request"); `TestPrivateWarehouseErrand`. | 6 | — |
| V3b | The session's look in rule "valuables": its conditions (the off-round errands': `storeReady`'s state checks, a registry town, a private keeper within `RoundReach`, no errand running (the enchant exchange included), the warehouse open due; not the ad board, `RoundGap` or the budget), the look request with the look's anchor and `RoundReach`, the walk back after it (after the next keeper's request when one ended on a new `noPrivate` mark), the stored-units accessor wired to the private status, entries started by look rows with no entry (the warehouse row test), and the errand handed the `noPrivate` set of the `valuables` value so the marks hold across sessions; a look of a bot called away (`called()`) cancelled with `cancelPrivate`, no walk back; `TestValuablesLedgerLook`. Nothing opens a round yet. | 2, 3, 6 | V2b, V3a |
| V4 | The ad board and the Trade line: region keyed caps, `Free` and the atomic `Claim`, the ad composer (line packing over V2b's labels and prices, entries not released), per-bot pacing, and the ad step of rule "valuables" (an open round says its lines); `TestAdText`, `TestAdPacing`. | 4 | V2b |
| V5 | The whisper responder: intent and price parsing (prices as whole words, never a `+N` enchant, an `Nx` count or digits inside a name; no offer from a line with two prices; an offer below half the floor drawing no counter), the three naming outcomes (names, points at several, names nothing; distinctive words before kind words), asks, floors, counters, the reply delay, the per-partner reply queue sent by rule "chat", reply caps apart from `chatLog`, precedence over the canned reply, item names over entries not released during a round, the after-round replies within `AnswerAfter` (the `in my store` reply comes with V7b); `TestValuablesWhisperHaggle`. | 5 | V2b |
| V6 | Trades: valuables in `acceptTrade`, `tradeGoods`, `tradePrice`, `tradePaid`; asking a whisperer for a trade, the invite step of rule "valuables" (walk bounded to `RoundReach` of the round state's anchor, `TradeRequest`, every refusal, none within 15 s of the round's end, V2b's walk back to the anchor once the trade or request is over), `tradeGoods` never offering a released entry; trade sales counted in the valuables budget and its offer gate; `TestValuablesTradeInvite`, `TestValuablesTradeConfirm`. | 5, 8 | V2b, V4, V5 |
| V7a | The round's opening and end in rule "valuables": `roundReady` (town, a private keeper within `RoundReach`, region, ad board and warehouse open checks, the errand checks with the enchant exchange idle, a clan member's visit settled first), which opens the round with `openRound` at the bot's position, the start condition (an entry with carried units, not released), the pending round and `storeReady`'s `StoreHold` wait (rule "valuables" holding the turn on the timers), no round before the session's look (V3b), stepping the private requests (with the round's anchor and `RoundReach` on each request), V2b's walk back after the round's keeper visits and the off-round errand, before the ads and the store opening (V6 has the one after a buyer), the left-over round of an ended session, withdraw, end, `FirstLineWait` and grace, `called()` with the cancel of the running private request (`cancelPrivate`, or the wait for one at `whSent`) and no walk back, the round step (rounds and asks), deposit (skipped when no open is due, held while a trade request is pending), the store opening held while the bot's own request is pending, the off-round deposit once the budget is spent, then the store as today; the `valuableFees` reserve term; `TestValuablesRoundStart`, `TestValuablesStoreHold`, `TestValuablesFirstLineWait`, `TestValuablesHoldsTurn`, `TestValuablesRoundDeposit`, `TestValuablesGrace`, `TestValuablesReadvertise`. Without V7b nothing is released: unsold valuables stay held, which no preset sees before V8. | 2, 3, 6, 8 | V3b, V4, V6 |
| V7b | The cap and release: the cap between the round step and the deposit (released carried items skip the deposit), and after the round step of a round cut off by `called()` after its first line, the release withdraw errand outside a round (not once the valuables budget is spent) and the pending release withdrawal that holds the store back under `StoreHold`, the round's withdrawal taking the released stored entries too while the budget is not spent, the `storeReady` and `storeSells` gate, released rows first in `storeRows` at their floor by the valuables budget's listing rule, and the per-row split of store sales; the after-round replies for released entries (`in my store` when listed, `not now` when not); `TestValuablesRelease`, `TestValuablesStoreSplit`. | 3, 7, 8 | V7a |
| V8 | The preset: `merchant` enables the policy, the shared ad board, `-valuables-budget`, the ad board flags, metrics, the panel's held valuables. | 9 | V7a, V7b, M10 S9 (#331), which also edits the preset catalog |
| V9 | e2e live scenarios `TestLiveValuablesTradeSale`, `TestLiveValuablesWarehouse`, `TestLiveValuablesRelease`; the unmarked buyer account and its character helper, the Human Mystic merchant; `EncodeAnswerTradeRequest` in `internal/botclient/harness.go`, so the buyer can accept the bot's `TradeRequest`; the `docs/e2e-live.md` rows. | Testing | V8 |

The round is split in two PRs from the start: V7a (the round) and V7b (the cap, the release, the
store gate changes and the store sales split), so each stays one reviewable diff. For the same
reason the supply and score (V2a, read-only travel map work) come apart from the brain's policy,
ledger and classification (V2b), and the private warehouse errand (V3a) from the session's look
that drives it (V3b), which also takes the look out of V7a.

## Risks

- **Spam.** Ads are the one place bots speak to a whole region. The caps of section 4 bound it to
  one bot line a minute per region, and D30 keeps it to the Trade channel and one preset.
- **The supply ceiling overstates supply.** No server kills every spawn at respawn; the real
  supply is a fraction of the ceiling, and the fraction differs between items (a spawn far from
  any town or deep in a dungeon is killed less often). The maker rules take out the largest
  differences the datapack shows (a `random_spawn` maker places one entry of several, a day or
  night maker lives a quarter or three quarters of the day, a group maker comes back only whole,
  an `EventName` or a start condition gates a maker); without them 11 templates priced at 10000
  or more crossed `ValueMin` the wrong way (section 1). What is left is approximate: unequal
  entry totals in a `random_spawn` maker, the even spread over a linked
  `maker_instant_spawn_random` set, the 43 types counted like the default maker and the
  `maximumNpcs` caps (read by the spawnlist reader, `Maker.MaximumNPCs`, `datapack/spawnlist.go:31-33`, but not applied); `TestValueScore` pins each rule as it stands. `ValueMin` and `CommonSupply` absorb the scale and are persona defaults.
- **Unknown sources.** Quest rewards and event drops have no datapack source and count as common;
  price alone decides for them.
- **Haggling text.** The parser reads prices as whole words with `k`, `kk` or `m`, or bare numbers
  of at least 1000, never a `+N` enchant, an `Nx` count or digits inside a name, and item names by
  full name, distinctive word or kind word (section 5), so a common word such as `of` names
  nothing; a line it cannot read gets the price, and one that could mean several items or holds
  two prices gets the asks and no offer is taken. An offer below half the floor draws no counter.
  It never sells below the floor, whatever it misreads.
- **Ledger lost on a process restart.** The stored items are found again by the look, which
  starts an entry for each row that passes the warehouse row test (section 2, "Stored rows with no
  entry"), but their rounds restart at 0 and a release mark is lost, so an item may get up to
  three more rounds before its release. A row that no longer passes the test (a persona's
  `ValueMin` or `PriceFloor` raised since its deposit, or its template now sold by a counted
  buylist) is never withdrawn by the bot and stays in the private warehouse, taking a slot; the
  bot deposits nothing else there, so these rows stay few.
- **Time in town.** A merchant spends up to about 9 minutes of a town visit on a round, plus its
  two warehouse visits and their walks back to the anchor (each up to `RoundReach`, in Dion 5348
  each way) (`FirstLineWait`, `RoundWait` and `Grace` bound it, section 3), and up to
  `StoreHold` (2 min) with its store held back for a pending round, and starts a
  round per 30 minutes at most; the merchant farms less, which suits the preset. The warehouse
  errand's backoff (up to 30 min after unanswered opens, shared with clan requests) does not
  stretch a round: a round starts only when an open is due and skips a deposit that is not due at
  its end (section 3), so the bound holds.
- **A store opened ahead of a valuable.** A valuable carried into town within `RoundGap` of the
  last round's start, or one whose round is held back past `StoreHold` (a long warehouse backoff,
  a busy ad board), does not stop the store opening (section 3); the valuable stays out of the
  store and waits for it to close, which may take long (it closes when sold out, at a teleport or
  a `follow`). It is never sold at a loot price; its round is only late.
- **Players gaming the bot.** A buyer can only pay at or above the floor, which is at least the
  store price and the item's worth (the reference price plus its enchant scrolls), by haggle or
  in the store; the trade confirms only on the partner's adena (`tradePaid`).

## Gaps

- One valuable per trade window: a buyer of two trades twice. A follow-up issue in M12 once the
  owner approves (a multi-item offer in `tradeGoods`).
- The ledger lives in memory, not in the bot registry. A follow-up issue in M12 if the owner wants
  it kept across process restarts.
- No server gap.
