# The value score (D31)

## Who owns the inputs

Every bot shares one travel map, read-only after `NewTravelMap` (`cmd/bots/swarm.go:313`,
`travel.go:21`). It gets, once at start, for every template with a reference price, the supply
ceiling `S` and the marks `multisell` (made by a spawned NPC's non-upgrade multisell row) and
`craftable` (a recipe product), in a read-only `TravelMap.supply` beside `prices`; `S`'s inputs come
in a `SupplyConfig` ([preset](preset.md#process-settings)). The rest is per persona and computed when
an item is tested (a few operations, nothing cached).

## Supply ceiling

`S`, items per hour server-wide as if every spawn were killed at respawn, summed over every maker,
every `<npc>` entry of a template with drops and every private of an entry:
`S += share × total × 3600 / cycle × p × mean`, the entry's `total` applying to its privates too
(placed once per NPC spawned). Privates are the entry's `<private>` rows, else its template's
(`privatesOf`; V1 reads both). `p = category% × drop% / 10000`, `mean = (min + max) / 2`; DROP counts
fully, SPOIL times `SpoilWeight` (0.2: only a spoiler rolls it), HERB and CURRENCY not at all; raid
bosses like any spawn. V1 reads each maker's `<ai type>`, `<set>` params and `spawnTime`, skipped
today (`datapack/spawnlist.go:85-89`).

**Share** (a private row takes its leader's):

- **Event** (`event=`, else `EventName`, as `Maker.EventName`; it gates the 231 `event_maker`
  makers): 1 with none or a spawn event (flag, default `extra_mob;18age;start_weapon`);
  `SevenSignsShare` (0.5, one week in two) for `ssq_event`, `ssq_seal1_none`, `ssq_seal2_none`; 0 for
  `_dawn`, `_twilight` (they need a cabal that owns the seal and won, unknowable ahead) and any other.
- **Start:** 0 for a maker with no event not starting at boot (a `spawnTime`, or `on_start_spawn` 0
  outside the linked sets) and for every `no_on_start_maker`.
- **Type:** `default_maker` and types with no maker of their own (`random_spawn_treasurebox`, ...)
  as is; `random_spawn`, `random_spawn_pawn` ÷ the entry count (one drawn evenly at a time; closest
  when the entries' totals are equal, as `spawnlist/19_23.xml:1129-1134`); `on_day_night_spawn` × 0.25 for `IsNight` 1,
  × 0.75 for 0 (night is 6 of 24 game hours, `task/gameclock.go:20`, a constant);
  `maker_instant_spawn_random` × the fraction of its linked set (by `maker_name1`-`maker_name3`)
  starting at boot, a pair with one starter 0.5 each (even spread, approximate). The 43 other
  own-maker types (290 makers, such as `royal_rush_maker`, `manage_teleport_dungeon`) count like
  `default_maker`, an approximation `TestValueScore` names.

**Cycle:**

- `respawn` above 0: `respawn + KillFloor` (60 s to find and kill it); `respawnRand` adds nothing
  (even draw around `respawn`);
- `respawn` 0 or `no` (`datapack.NoRespawn`, -1 s): once per `NoRespawnCycle` (24 h); Freya's
  Gardener (22100, `spawnlist/23_14.xml:1289`) is one of 93 `respawn="no"` entries;
- `maker_instant_spawn`: `KillFloor × N`, N the group's NPC count (the sum of its entries' totals; it returns whole with no delay, so
  `respawn` is unused); `maker_instant_spawn_random`: `respawn + KillFloor × N`;
- a private row: its leader's cycle, or `respawn + KillFloor` when its own `respawn` is above 0 and
  that is shorter. 12 templates priced 10000 or more drop only from privates, such as Recipe:
  Halberd(60%) (5458, 366000; no space in the name, `items/5400-5499.xml:476`), a SPOIL of Judge of
  Light (21546), a private of Judge of Splendor (21544, `respawn="100sec"`) with its own
  `respawn="0sec"` (`spawnlist/22_15.xml:441`): `S` 0.045, `R` 3.35, not 0 (yet no valuable: buylist 5723 of NPC 31075
  sells it, `buyLists.xml:11160,11247`).

## Rarity, marks and worth

- **Rarity** `R = clamp(log10(CommonSupply / S), 0, RMax)`, `CommonSupply` 100/h, `RMax` 4. Every
  source with share above 0 adds to `S`; with none (seasonal makers, quest rewards only) `R = 0`.
- **Shop:** an item a counted buylist sells (`cheapest`: NPC id above 0, as `storeLoot`) is no loot.
  The 212 `npcId="-1"` lists, skipped by `indexShops` (`shop.go:283-285`), do not count; they carry
  all four valuable [figures](#figures) rows (433 in list 1001, `buyLists.xml:9408,9477`; 309 in
  2012, `:10437,10442`; 121 in 9001, `:11612,11620`; 184 in 9017, `:11868,11872`).
- **Multisell:** a spawned NPC's row from no weapon or armor gives `R = 0`; a row consuming a weapon
  or armor (upgrade, exchange) is no source. **Craftable:** `R / CraftDivisor` (2). **Grade:** no
  signal (already in price and supply); the ad names it, D and up.
- **Enchant** `e`: the worth adds the reference price (`prices`) of the grade's **normal** scroll
  per level by kind, the plain rows of the bot's `enchantScrolls` (`enchant.go:66-83`, not blessed
  or crystal); no grade, no scroll (`enchant/service.go:549`), so `W = ref`. Each level past safe
  (+3, +4 full body) adds `EnchantRare` (0.25) to `R`, flat for both kinds (understating high armor
  levels, 0.66^(level - 2), but only a nudge).
- **Worth** `W = ref + e × scroll`; **score** `V = W × 2^R`.

## Valuable test

A carried item is **valuable** when `ref ≥ PriceFloor` (10000), `V ≥ ValueMin` (100000), and it is
not equipped, not kept for the bot's use (`keeps`: supplies, keep set, shots, ...), not kept by the
persona's own keep hook, not worn next nor fallen back to (`fallsBack`, as `junk`, `shop.go:596`;
`tradeGoods` and `carriedStock` never move an equipped item), not a set piece kept for a set, not a
quest item, not augmented (the store excludes them, `storeLoot`; their worth is not in the
datapack), not a shadow item, and not sold by a counted buylist. Precedence: the bot's keeps and the persona's own hook, then the gear errand, then
valuable, then the clan member's hold (M9), then loot. Rows: a
[narrower test](ledger.md#stored-rows-with-no-entry).

**The clan member's hold comes after valuable.** The `merchant` member's hold
([today](server.md#what-the-bot-does-today); `memberHold`, `clanmember.go:112-118`; `memberKeep`,
`:112-137`) would first take every valuable recipe and the D-grade rows Conjuror's Staff (184,
`items/0100-0199.xml`) and Elven Tunic (433, `items/0400-0499.xml`). Sword of Watershadow (121,
`items/0100-0199.xml`) and Tears of Eva (309, `items/0300-0399.xml`) have no `crystal_type`: out of
`TravelMap.crystals` (`indexCrystals`, `crystallize.go:66`), held by no `memberKeep`
(`clanmember.go:127-129`), no recipe material. V2b: `wireMember` keeps the persona's own hook
(`holds`, `clanmember.go:53`) in the member state for the valuable test (with `keeps`), never the
wrapped `Persona.Holds`; `memberKeep` returns no hold for a valuable, asking the test under its
`inHold` guard (the test asks the gear errand, which asks the hook back, `clanmember.go:40-44`).
`memberDeposit` keeps only `memberKeep`'s rows, which `memberHandOff` weighs and the fees count
(`memberFees`, `clanmember.go:72-80,351-370`), so a valuable leaves all three; other graded items
and recipes still go to the clan.

## Figures

Shipped datapack, 2026-11-24, these rules (a throwaway script; V2a's test pins them):

| Item | ref | S per hour | R | V | Valuable |
| --- | --- | --- | --- | --- | --- |
| Sword of Watershadow (121) | 136000 | 0.67 | 2.17 | 612000 | yes |
| Tears of Eva (309) | 136000 | 0.62 | 2.21 | 628000 | yes |
| Elven Tunic (433) | 99900 | 0.44 | 2.35 | 510000 | yes, by rarity (price alone would not) |
| Conjuror's Staff (184) | 644000 | 0.056 | 3.25, craftable: 1.63 | 1988000 | yes |
| Scroll: Enchant Weapon (Grade D) (955) | 50000 | 54.3 | 0.27 | 60100 | no |
| Fist of Butcher (2371) | 25500 | 3.77 | 1.42 | 68400 | no |

The Seven Signs half share moves only Conjuror's Staff (S 0.045 without) and the D scroll (53.6);
template privates move no row nor Recipe: Halberd; maker types move Elven Tunic from 0.45 (three
`maker_instant_spawn` groups, e.g. `spawnlist/18_23.xml:588-592`, back every 180 s, not at
`respawn="68sec"`) and the D scroll from 54.4; no verdict changes. Of 1236 tradable templates priced
10000 or more with a drop source, the types move 11 across `ValueMin` (price and rarity alone), all
toward valuable, and `R` by 0.1 or more for 112. Of the DROP items of spawned monsters to level 20
that no spawned NPC sells or non-upgrade multisell makes, 5 of 71 are valuable; to level 30, 14 of 155
(counted before the Seven Signs, respawn, private and type rules, by spawned seller, not `cheapest`;
V2a's test recounts them).

## Tests

- **Datapack (V1, fixtures):** the drop reader keeps each category's kind and chance and each drop's
  item, min, max, chance, failing on a mangled value; the NPC reader a template's `<private>` rows;
  the spawnlist reader an entry's, and each maker's `<ai type>`, `<set>` params (`EventName`,
  `IsNight`, `on_start_spawn`, `maker_name1`-`maker_name3`, `maker_cnt`) and `spawnTime`; the
  multisell reader a row's NPCs, ingredients, productions; the restart reader each point's tiles;
  the item reader every priced template's name, a `RECIPE` scroll included.
  `TestLoadShippedDatapack` and `TestLoadShippedValuables` (`BOT_DATAPACK_XML`, skipped without, as
  `datapack_test.go:215-221`) load the shipped files.
- `TestValueScore` (V2a, table-driven): shop-sold no loot; a multisell row with no weapon or armor
  ingredient `R = 0`, gear product or not; an upgrade row no source; a recipe halves `R`; a
  raid-only drop rare; SPOIL 0.2; no event or a `SpawnEvents` event fully,
  `ssq_event`/`ssq_seal*_none` 0.5, `_dawn`/`_twilight` and `medal` nothing; an `event_maker`
  without `event=` gated by `EventName` (`extra_mob` fully, `christmas` nothing, a changed list
  flips them); `spawnTime`, `on_start_spawn` 0, `no_on_start_maker` nothing; `random_spawn` of three
  equal entries a third each; `on_day_night_spawn` 0.25 night, 0.75 day; a `maker_instant_spawn`
  group of three back every `3 × KillFloor` whatever its `respawn`; a `maker_instant_spawn_random`
  pair with one starter half each at `respawn + KillFloor × N`; an unnamed own-maker type like
  `default_maker`; no source `R = 0`; `respawnRand` no change; `0sec` and `no` once per
  `NoRespawnCycle`; a private row at its leader's cycle (own `respawn` 0) or its shorter own one,
  with the leader's share, not `R = 0`; under `total` 2 twice; no rows takes the template's, own
  rows do not add them; +5 graded weapon five normal scroll prices (not blessed or crystal) and +0.5
  `R`; +5 no grade no price (`W = ref`), still +0.5; the thresholds; `S` from `NewTravelMap` with a
  `SupplyConfig` (a changed `KillFloor` or event list changes it), while two personas with different
  `CommonSupply` or `ValueMin` on one map score an item differently.
- `TestValuablesShipped` (V2a, `BOT_DATAPACK_XML`): the six [figures](#figures) rows.
- `TestValuableClassification` (V2b): the precedence; an equipped valuable or a fall-back weapon is
  none; a held valuable in no NPC sale (`junk`), no
  `storeRows`, no `tradeGoods` outside a round. An active clan member: a D-grade valuable it would not
  wear and a valuable recipe are valuable, in no `memberDeposit` row, `memberHandOff` weight or
  arrival fees; other graded items and recipes kept and deposited as today; a no-grade valuable in no
  member hold either way; no recursion through `memberHold`.
