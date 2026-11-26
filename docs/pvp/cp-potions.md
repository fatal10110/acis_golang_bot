# Buying CP potions

Plan for #427. Ships in X17-X19 (#612, #613, #614). Decision D43 (proposed, needs owner
approval): it amends D37's "CP potions kept, never sold or bought". Paths per
[README](README.md#paths); Seven Signs server code is under `internal/gameserver/sevensigns/`,
`internal/gameserver/signspriest/` and `model/actor/npc/folk_chat_signs.go`, its pages under
`aCis_datapack/data/html/seven_signs/`.

Today a `pvp` bot drinks CP potions 5591 and 5592 from stock and never buys them
([CP](layer.md#cp-cures-pool-break-off), [consumables](model.md#gear-and-consumables-x10)), so
the stock runs out and stays out. A GM grant is the only source.

## Server facts

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Who sells CP potions | Only multisell 500, "Purchase consumable items", at the Seven Signs priests (the list's `npcs`: Priest of Dawn 31078-31084, Dusk Priestess 31085-31091, and 31168, 31169, 31692-31695): CP Potion 5591 for 240 Ancient Adena (5575) plus 200 adena, Greater CP Potion 5592 for 600 plus 500 (the adena is the castle tax ingredient). Buy list 3001 has no merchant. No monster drops either kind. Reference prices 240 and 600, weights 25 and 100. | `multisell/500.xml:3-26`, `:58-68`; `items/5500-5599.xml:655-677`; [items](server-items.md) |
| When a priest offers it | A priest greets a member of its own cabal with page `2a` or `2c` only in the Seal Validation period and only when that cabal won the competition. Those two pages link multisell 500, the stone exchange (`SevenSigns 16`) and the reward (`SevenSigns 9`). Every other case gets a page without them: `1` (competition), `5` (results), `6` (recruiting), `2b` (its cabal lost), `2d` (a tie), `3` (a member of the other cabal), `4` (a non-member while that cabal won). | `folk_chat_signs.go:119-147`; `dawn_priest_2a.htm:11-12`, `dusk_priest_2a.htm:11-12`, `*_priest_2c.htm:10-11` |
| Links are checked | The server accepts an `npc_` bypass only when it is a link of the last page it sent the player, so a bot cannot reach multisell 500 or the exchange without that page. | `network/bypass.go:64`, `:139`; the Java reference's `validateBypass` |
| The cycle | Recruiting (15 min), Competition (to Monday 18:00 server time), Results (15 min), Seal Validation (to the next Monday 18:00). One cycle is two weeks; a new one clears every sign-up, contribution and stone score. The winner is the cabal ahead on total score (stone score plus festival score); equal totals, 0 to 0 included, mean no winner. No GM command changes the period (Java and Go). | `sevensigns/sevensigns.go:88-95`, `:620-655`, `:695-711`; `sevensigns/players.go:153-163`; `adminCommands.xml` |
| Signing up | At a priest during Competition (page `1`: `SevenSigns 3`, `33`, `34`, then `4 <cabal> <seal>`). Needs a first profession (page `signs_33_*_firstclass` otherwise). Dusk is free except for a castle-owning clan's member; Dawn costs 50 000 adena or a Certificate of Approval unless the clan owns a castle. A seal must be chosen: it is a vote. `SevenSignsBypassPrerequisites` is `False` shipped. | `signspriest.go:282-345`; `dusk_priest_1.htm`; `ops/docker/config/events.properties:71` |
| Seal votes | At the end of a competition the winning cabal claims a seal when at least 35 % of its members chose it (a held seal is kept at 10 %); the losing cabal's votes count only to keep a seal it holds. A held seal changes the world for every player: Gnosis gates the catacombs and Avarice the necropolises in Seal Validation, Strife settles the castles and casts the Strife skills on everyone. | `sevensigns/score.go:82-125`, `sevensigns.go:439-450`, `:657-678`; `dungeon.go:18-24` |
| Seal stones | Blue 6360, Green 6361, Red 6362, worth 3, 5 and 10 points. Dropped only by the 42 catacomb and necropolis monsters 21166-21207 (levels 21-80), two or three CURRENCY categories at 70 % each, scaled by `RateDropCurrency` (1 shipped, "affects Adena and Seal Stones"). Expected per kill, in Ancient Adena at 1 per point: about 400 at level 21, 640 at 30, 1 100 at 42, 1 650 at 48, 2 400 at 57, 3 150 at 63, 4 400 at 80. | `npcs/21000-21999.xml` (e.g. `:14560-14566`); `sevensigns/score.go:7-28`; `ops/docker/config/server.properties:115-120` |
| Where they spawn | 1 308 stone droppers in `ssq_event` makers (Recruiting and Competition), and in Seal Validation 594-714 per seal in the `ssq_seal1_*`/`ssq_seal2_*` makers of the cabal that owns that seal (none when no one owns it). | `spawnlist/*.xml` (e.g. `18_24.xml:547-606`); [valuables server facts](../valuables/server.md) "Spawns" |
| Entering a dungeon | A Dungeon Gatekeeper at each entrance (`necro*`, `cata*` bypasses) admits a signed-up player of either cabal outside Seal Validation, and in it only a member of the winning cabal that owns the dungeon's seal (catacombs: Gnosis, necropolises: Avarice), any member after a tie; no sign-up, no entry. | `sevensigns/dungeon.go:20-45`; `model/actor/npc/folk_dungeon.go` |
| Stones into Ancient Adena | On page `2a`/`2c`: `SevenSigns 17 4` turns every stone held into Ancient Adena at once (3/5/10 each, no cap); `SevenSigns 17 <1-3>` and `18 <id> <n>` one color. `SevenSigns 9` pays the stones contributed during the competition (`SevenSigns 6`, capped at `MaxPlayerContrib` 1 000 000 points) at the same rate, and contributing adds to the cabal's stone score. Stones held are never lost: a losing cabal's member keeps them for a later cycle. | `signspriest.go:356-420`, `:497-517`, `:535-620`; `sevensigns/players.go:80-111` |
| Other Ancient Adena | The Festival of Darkness (party content). The Black Marketeer of Mammon trades Ancient Adena for adena 1:1 (`SevenSigns 7`), never the reverse. Players can sell it in private stores. | `blkmrkt_1.htm`; `signspriest.go:476-495` |
| Multisell | `MultiSellList` pages (`multisell.PageSize` rows) after the bypass; `MultiSellChoose` (list, entry, amount) is dropped in silence when it falls inside `MultisellTime` (100 ms shipped) of the last one, or does not match the shown list, the NPC or its reach; weight and slot refusals answer with a `SystemMessage`. The bot has no multisell wire today. | `network/multisell.go:16-80`, `serverpackets/multisell.go`, `network/client_ingame.go:258`; `ops/docker/config/server.properties:222-223` |

## Options

The cost is never the problem: one level-21 catacomb kill pays for a CP potion and a half. The gate
is: the priests sell only in a Seal Validation week and only to a member of the cabal that won.

1. **Status quo.** CP potions from GM grants only; no work. A `pvp` bot runs dry and fights without
   them.
2. **Player stores (proposed, `pvp` default).** Buy 5591 and 5592 from private sell stores with the
   M8 market errand ([items](../items.md#8-buying-from-player-stores)): no Ancient Adena, no Seven
   Signs, no world effect. It works only while players sell them, which a quiet server may never
   see; the bot just finds none.
3. **Seven Signs member (proposed, opt-in `SevenSigns` mode, off for every preset).** Sign up with
   Dusk in Competition, farm seal stones in the catacombs and necropolises, hold them; in a
   validation week its cabal won, turn them into Ancient Adena at a priest and buy CP potions with
   multisell 500. A losing or tied cycle costs nothing: the stones wait. Volume is ample (an hour
   of catacomb kills covers many cycles of stock); the expected payout depends on who wins:
   - **3a, never contribute (proposed).** The bots never touch the competition score; they win
     only when human players' stones and festivals carry their cabal. On a server where no human
     plays the Seven Signs, every cycle is a 0-0 tie and the route never pays.
   - **3b, contribute.** Bots also turn stones in during Competition. Their cabal then wins any
     cycle humans do not outscore, every cycle on a quiet server, and the bots' seal votes decide
     who holds Gnosis, Avarice and Strife for every player: a world change, the owner's call.
4. **Buy Ancient Adena from player stores.** Option 3's purchase with Ancient Adena bought with
   adena instead of farmed stones: still gated by membership of the winning cabal, so it adds a
   buying rule to option 3 and removes nothing. Not proposed.
5. **Festival of Darkness.** Party timed instance content for Ancient Adena; far more work than
   option 3 for the same gate. Not proposed.

## Proposed decision D43

CP potions are bought (amends D37); the keep rule stays (never sold, deposited or drunk on
monsters). Option 2 for `pvp` bots; option 3a as an opt-in `SevenSigns` mode, off for every preset
(D6). Bots never contribute stones, never pay the Dawn fee and never vote Strife.

## What the bot does

**Stock.** `PvPPolicy.CPStock` (default 10): the CP potions a `pvp` bot holds, Greater counted as
4 (its 200 CP against 50). Below `Supply.Refill` percent of it, the supply list asks for the
difference, Greater first.

**X17, player stores.** The M8 market errand (rule 9 errand "market") counts 5591 and 5592 as
trip targets of the `pvp` list: rows at no more than `Market.MaxOverReference` percent of the
reference price (240 and 600), cheapest per CP first, within the budget; never a karma or flagged
owner. Pacing, refusals and cool-downs are the errand's ([items](../items.md#8-buying-from-player-stores)).

**X18, Seven Signs member (`SevenSigns` mode).**

- *Sign-up.* In Competition, with a first profession and outside a castle-owning clan, at the
  nearest Dusk Priestess on a supply trip: page `1`, then the sign-up links for Dusk and
  `PvPPolicy.SignsSeal` (default Gnosis; Strife refused by config). A page the bot does not expect
  (`signs_33_dusk_no`, `_firstclass`, `_member`) ends the errand for the cycle; `_member` means
  already signed. The bot keeps no sign-up record: after a restart it reads its membership from the
  priest's page, so the server stays the only record (nothing for #404's ledger).
- *Farming.* The catacombs and necropolises become spot candidates for a member in Competition
  (and in a validation week for the dungeon of a seal its winning cabal owns, per `dungeon.go`),
  through the Dungeon Gatekeeper as a travel leg (enter and exit bypasses, the teleport landing as
  the leg's end). The spot model already counts `ssq_event` makers at half time
  (`SupplyConfig.SevenSignsShare`); stones join the drop value at 3/5/10 per stone only for a member.
- *Keep.* Seal stones are kept like CP potions: never sold, deposited or traded. No contribution
  (`SevenSigns 6` is never sent).

**X19, the priest trip.** In Seal Validation, a `SevenSigns` bot holding stones or Ancient Adena,
below its CP stock, adds a priest stop to its supply trip:

1. Talk to the nearest Dusk Priestess. Only a page linking `_multisell 500` and `_SevenSigns 16`
   goes on (the bot reads links, not text); any other page ends the stop until the next
   validation week, stones kept.
2. `SevenSigns 16`, then `SevenSigns 17 4`: every stone into Ancient Adena; the page that follows
   and the `InventoryUpdate` confirm it.
3. Talk to the priest again (the exchange's answer page replaced the links the server accepts),
   `_multisell 500`, decode `MultiSellList`, and one `MultiSellChoose` for the Greater CP row with the amount the
   stock lacks, bounded by the Ancient Adena and the adena it holds (the tax ingredient included)
   and by weight; then the plain row for what is left. A silent drop shows as no `InventoryUpdate`
   within the reply timeout: the stop ends.

Ancient Adena left over is kept; turning it into adena at the Black Marketeer of Mammon is M12's
business, not this plan's.

**Pacing.** Every priest bypass under `bypassGap` (500 ms, above `ServerBypassTime` 100 ms), at
most one `MultiSellChoose` per answer and never two within 500 ms (above `MultisellTime` 100 ms),
at most two per trip; the trip itself under `Travel.Gap`. Nothing new runs while the bot is not
on a trip.

## Tests

- **X17 (unit):** supply-list rows for 5591/5592 from a store list at, above and below
  `MaxOverReference`; Greater first; `CPStock` counting; a karma owner skipped. E2e: a seller
  character's store with CP potions (GM-seeded) bought by a `pvp` bot (`TestLivePvPCPStore`).
- **X18 (unit):** sign-up decision by period, profession and clan; the page answers (`_no`,
  `_firstclass`, `_member`); dungeon spot eligibility by period, membership and seal owner; the
  gatekeeper leg; the stone keep rule. E2e: sign-up in Competition and dungeon entry
  (`TestLiveSevenSignsJoin`), and the refusal of a non-member at the gatekeeper, when the stack is
  in Competition; skipped with its reason in any other period.
- **X19 (unit):** codec tests for `MultiSellList` and `MultiSellChoose` against the server's
  encoder and golden bytes; the trip from recorded frames of each priest page (`2a`, `2b`, `2c`,
  `2d`, `3`, `4`); amounts from held Ancient Adena, adena and weight; a silent drop ending the
  stop. E2e: possible only while the stack is in a validation week its test character's cabal won
  ([owner question 3](#owner-questions)).

## Owner questions

1. Approve D43? Options: player stores only (2), stores plus the opt-in Seven Signs member
   (2 + 3a, proposed), or the status quo (1).
2. Option 3: never contribute (3a, proposed: the bots never decide the Seven Signs, and on a quiet
   server the route never pays) or contribute (3b: the bots' cabal wins every cycle humans do not
   outscore, and their votes pick the seal owners)?
3. X19's live scenario needs the stack in Seal Validation with a known winner. Options: a GM
   command to set the Seven Signs period, filed in `fatal10110/acis_golang` as a test-stack tool
   (Java aCis has none either), or X19 ships on unit tests and recorded frames, recorded once on a
   real validation week.
4. Seal vote: Gnosis by default (proposed) or Avarice; Strife never?
