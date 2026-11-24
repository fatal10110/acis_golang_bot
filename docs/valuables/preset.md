# Preset, defaults and live scenarios

## Wiring

- `ValuablePolicy` in the persona: `Enabled` and every default below; zero disables it. Only the
  `merchant` preset enables it (`cmd/bots/persona.go`), with the shared ad board.
- Metrics: `valuables_ads_total`, `valuables_replies_total`,
  `valuables_sold_total{via="trade|store"}`, `valuables_deposits_total`, `valuables_released_total`,
  and a gauge of held valuables. The panel's bot view shows held valuables (name, place, rounds, ask).

## Defaults

| Field | Default | Where |
| --- | --- | --- |
| `PriceFloor`, `ValueMin` | 10000, 100000 | [score](score.md) |
| `CommonSupply`, `RMax`, `CraftDivisor`, `EnchantRare` | 100/h, 4, 2, 0.25 | [score](score.md) |
| `RoundGap`, `RoundWait`, `Grace`, `FirstLineWait` | 30 min, 5 min, 2 min, 2 min | [round](round.md), [round-end](round-end.md) |
| `RoundReach`, `StoreHold` | 6000, 2 min | [round](round.md) |
| `AdEvery`, `AdLines`, `RegionGap`, `ProcessPerHour` | 2 min, 3, 60 s, 30 | [ads](ads.md) |
| `MaxAsk`, `AskPerRarity`, `FloorPct`, `RoundStep`, `Counters` | 150, 12.5, 100, 10, 2 | [whispers](whispers.md) |
| `Replies`, `RepliesPerPartner`, `InviteReach`, `AnswerAfter` | 20, 4, 1200, 30 min | [whispers](whispers.md), [trades](trades.md) |
| `HoldRows`, `HoldValue`, `MaxRounds` | 6, 2000000, 3 | [release](release.md) |
| `Budget` | 2000000 per session | [release](release.md#valuables-budget-d34) |

All are persona defaults (D6) except `RegionGap` and `ProcessPerHour`, operational limits on what the
process says, set by flags.

## Process settings

The supply inputs shape the one `TravelMap.supply` all bots share, so they are no persona fields: V2a
gives `NewTravelMap` a second argument, a `SupplyConfig`, built in `cmd/bots/swarm.go`.

| Setting | Default | Set by |
| --- | --- | --- |
| spawn events | `extra_mob;18age;start_weapon` (the shipped `SpawnEvents`) | flag `-spawn-events`, mirroring the server config the bot cannot read |
| `KillFloor`, `NoRespawnCycle`, `SpoilWeight`, `SevenSignsShare` | 60 s, 24 h, 0.2, 0.5 | `SupplyConfig` defaults, no flag |

## Live scenarios (V9)

Live harness, at most 4 characters each, never with `-race`.

- **Buyer.** Every run account's character carries the bot marker (`docs/e2e-live.md:145-146`;
  `character`, `internal/e2e/helpers_live_test.go:31-43`, via `botnames.DefaultMarker`) and the bot
  replies to no marked name (`TestLiveChatReplies`, `internal/e2e/chat_live_test.go:12-17,73-78`);
  the unmarked GM is trade-exempt: offered nothing, confirmed only after it confirms
  (`trade.go:272-276,383-393`, `authority.go:294-306`). V9 adds a **buyer**: run account
  `<prefix>vb1`, no GM, whose character gets fixed candidate names the marker does not mark (a helper
  beside `character` building the `botclient.Character` with those `Names`, failing the scenario if
  `botnames.DefaultMarker.Marks` one); the `docs/e2e-live.md` rows name it. To accept the bot's
  request it needs `EncodeAnswerTradeRequest(accept bool)`, which V9 adds to `harness.go` like
  `EncodeTradeRequest`, `EncodeAddTradeItem`, `EncodeTradeDone`
  (`internal/botclient/harness.go:147-154`); `encodeAnswerTradeRequest` is unexported
  (`trade.go:145-150`, used by `trade.go:181` and unit tests).
- **Merchant.** A Human Mystic (`character(t, account, &botclient.HumanMystic)`, `character.go:34`),
  so the gear errand, ahead of the valuable test, wears neither GM-given item: mystics wield only
  `BLUNT/MAGIC`, `SWORD/MAGIC`, `BIGBLUNT/MAGIC` (`gear.go:219`), and Sword of Watershadow is a
  non-magic sword (`items/0100-0199.xml:300-302`, pAtk 24, no grade); Tears of Eva is `weapon_type`
  ETC (`items/0300-0399.xml:224-227`), which `indexGear` skips for every class (`gear.go:350`). Each
  scenario checks that it wears neither after the gift.
- **Keepers.** Talking Island (`townTI`, `internal/e2e/farm_live_test.go:114`, beside Roxxy; `toTown`,
  `helpers_m4_live_test.go:53-57`): the first look follows the freightman Rant's (30054, 2317 from
  Roxxy) `Chat 1`, marks him and goes on to Rolfe (30055, 2508; `spawnlist/17_25.xml:197-200`), where
  every deposit and withdrawal happens.

Scenarios:

- `TestLiveValuablesTradeSale`: the GM gives the merchant in town a Sword of Watershadow; the buyer in
  the region hears the Trade line, whispers an offer, gets a counter, whispers the counter, accepts
  the bot's request (`EncodeAnswerTradeRequest(true)`) and adds the adena; the trade settles at that
  price (inventory counts on both sides).
- `TestLiveValuablesWarehouse` (`RoundGap` lowered to fit the scenario): the buyer stays silent;
  after the round the merchant deposits the sword (adena down 30, the item gone); the GM gives it
  Tears of Eva; its next round withdraws the sword and one Trade line, heard by the buyer, names both. Deposit and
  withdrawal name Rolfe's object; no `_DepositP` or `_WithdrawP` bypass names Rant's.
- `TestLiveValuablesRelease`: with `HoldRows` lowered, the merchant lists its valuables in its store;
  the buyer buys one there.
