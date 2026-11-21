# Bot items: loadout, supplies and gear like a real player

Status: draft for the owner. It needs answers to D14-D16 (below) before any issue is filed. Two
independent review rounds; the second found it sound to file after its fixes, which are applied.
Revised after the first round. That round found: SoE casts are queued, not cancelled, by
moves; enchanting would freeze gear upgrades; buff potion stacking and pacing were wrong; the shot
fix sat in the wrong PR; beginner shots were wasted; player stores need a walk and exclude package
stores; and several PRs were too large.

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Datapack paths (`items/...`,
`buyLists.xml`) refer to `aCis_datapack/data/xml`. Bot paths name this repository.

## Problem

A real player plans their character: a weapon their class uses, armor of the right type (ideally a
full set), shots that match the weapon's grade on auto, potions, a Scroll of Escape for the trip
home, and adena saved for the next grade. They go back to town when supplies run low, and after the
first profession they draw dyes for their build. The bots do only part of this, and some of it
wrong.

### What the bot does today

| Area | Today | Where |
| --- | --- | --- |
| Gear | Wears and buys the best piece per slot by **reference price**, of the class's weapon and armor type (`defaultGearClasses`), up to the Expertise grade. Purchases keep 10000 adena (`KeepAdena`). Enchanted pieces are never replaced. Bows wait for their arrows. | `gear.go` |
| Shots | Turns on auto-use for **every** shot template it carries (`shotTemplates`, all grades). Restocks only **no-grade** soulshots (fighters, 100-500) or no-grade spiritshots (mystics, 50-200). | `items.go` `ruleShots`, `shop.go` `defaultShopPolicy` |
| Potions | Drinks the best of 1539, 1061, 1060 below 20 % HP in combat. Restocks only Lesser Healing Potion (5-20). | `items.go` `drinkPotion`, `rules.go` `ruleCritical` |
| Arrows | Grade-matched arrows for the worn bow and for the next bow, 200-1000. This is done right. | `arrows.go` |
| Restocking | Only when a merchant is in sight or its spawn is within `Shop.Reach` (8000), at most every 5 min. A miss pushes the next try 5 min out. | `shop.go` `startShop` |
| Selling | Everything not kept is junk and goes to an NPC merchant, enchant scrolls included. Merchants sell loot in their store. | `shop.go` `junk`, `store.go` |
| Enchant, dyes, class change, crafting, buying from player stores | None. | — |

### Findings

1. **Fighters from level 20 fight without shots.** The server needs the shot's grade to match the
   weapon's grade exactly (below). At 20 the free Expertise D skill arrives. The gear errand then
   wears any D-grade weapon it loots, and buys one once its adena passes the 10000 reserve plus the
   price. The no-grade soulshots stop charging. Auto stays on and fails in silence, so the bot never
   notices.

   No spawned NPC sells D-grade or higher shots. List 5720 belongs to the unspawned Sales Cat
   (31075), list 300511 is the GM shop, and no multisell has them. They come only from Dwarven
   recipes or other players' stores. So from 20, a bot with a D weapon has no shots, while the
   best no-grade weapon with shots might hit harder.
2. **Mystics buy plain spiritshots.** Grocers also sell Blessed Spiritshots (3947, no grade, in 29
   lists), the kind casters use. Finding 1's grade problem applies to them too.
3. **Farmers never restock or go home.** A spot is usually farther than 8000 from a merchant's
   spawn. A farmer runs out of shots, the server turns auto off, and the bot keeps farming without
   shots or potions until it dies or levels out of the spot. Nothing makes it go back to town, and
   nothing checks its supplies before it leaves for a spot.
4. **Stock is fixed, not measured.** 100-500 shots is a few minutes of fighting for a dagger user
   and a long session for a level-5 mystic. Real players buy for the trip they plan, up to what
   their adena and weight allow.
5. **Price is a rough stand-in for strength, and sets are ignored.** The item files carry the real
   stats (`pAtk`, `mAtk`, `pDef`, `mDef`, `pAtkSpd`, `rCrit`) and `armorSets.xml` lists 51 sets
   whose bonus the server applies. Buying per slot by price mixes sets and loses the bonus. A real
   player buys the full set (Brigandine, Reinforced Leather, Knowledge and so on).
6. **No saving for the next grade.** At 18-19 the bot buys no-grade upgrades it throws away a level
   later. The fixed 10000 adena reserve is not a savings plan.
7. **Good consumables go unused.** Healing Potion (1061) is sold in 25 lists, but only Lesser is
   restocked. Potion of Alacrity (735, attack speed), Magic Haste Potion (6035, casting speed) and
   Scroll of Escape (736) are sold by every grocer. The bot never buys them.
8. **Rare enchant scrolls are sold as junk.** D-grade scrolls (955/956) drop from only a handful of
   NPC templates (Hatu Dire Wolf, Bandit Watchman, invaders, raid bosses), so this is minor. But the
   few a bot finds are worth using up to the safe level, not selling.
9. **Specialization stops at the base class, and dyes need a profession.** Class change is out of
   scope in `docs/plan.md` (PR breakdown, the note after the table), so a bot stays Human Fighter
   or Elven Mystic forever. A base class has 0 dye slots.

## Goal

Every bot keeps a **loadout**, worked out from its class, level, worn grade, adena and weight:

- the strongest gear it can afford for its class, scored by the item's real stats, enchant and set
  bonus;
- shots of the worn weapon's grade, of the kind its class uses, on auto; beginner shots first while
  it carries them; none it cannot use;
- potions and a Scroll of Escape for one trip, sized from what it actually uses;
- adena saved for the next grade once that grade is near;
- once D15 allows it, a profession picked for its race and role, and the dyes that go with it.

It prepares before it leaves town and goes back when it runs low, the way a player does.

## Server behavior this rests on

Checked in the server and the datapack on 2026-11-21. Every feature below has a handler in the Go
server. The gaps are in data and spawns, not in code.

| Fact | Evidence |
| --- | --- |
| A shot charges only when the weapon has capacity for it and the grades match. On a mismatch nothing is consumed. | `model/actor/player/character_attack_shots.go:79-115`, `model/item/shots.go:59-65` |
| Turning on auto for a mismatched shot still enables it, with one GRADE_MISMATCH message. Later failures are silent and auto stays on. Auto turns off only when the stack is empty or too small. | `network/inventory.go:451-492`, `network/item_shot_use.go:90-100,155-170` |
| Shots per charge are the weapon's `soulshots` / `spiritshots` count (a random roll can lower the soulshot count). | `model/item/shots.go:59-65`, `data/xml/item.go:275-276` |
| Beginner shots 5789/5790 have no grade (they fit only no-grade weapons) and cannot be sold or traded. | `items/5700-5799.xml:1014-1035` |
| Expertise (239) is free, granted at 20/40/52/61/76 even with `AutoLearnSkills` off. | `classes/*.xml` (cost 0); `model/actor/player/template.go:203-224` |
| D-grade and higher shots: only list 5720 (unspawned Sales Cat 31075) and the GM list 300511 sell them, and no multisell does. Dwarven recipes make them at Create Item 2 (Artisan, class 56). | `buyLists.xml:10871,16366`; `recipes.xml:22,319,325` |
| Spawned grocers sell 1060, 1061, 734 (Haste), 735 (Alacrity), 6035 (Magic Haste), 736 (SoE), and no-grade 1835, 2509, 3947. 1539, 728 and 5591/5592 are not on any spawned list. | `buyLists.xml` (lists 14, 20-23, ...) |
| HP potions: 10 s reuse, and the stronger heal-over-time replaces the weaker (`hp_recover`). | `items/1000-1099.xml:492`, `skills/2000-2099.xml:251` |
| Buff potions: 500 ms reuse, 1200 s effect. Alacrity (skill 2012) is `attack_time_down` order 1, the same type as Haste (1086) and Haste for Beginners (4327, Newbie Guide). Magic Haste (2169) is `casting_time_down`, the same type as Acumen (1085) and Acumen for Beginners (4329). The higher order wins. | `skills/2000-2099.xml:127`, `skills/1000-1099.xml:1548,1586`, `skills/4300-4399.xml` (4327 order 2, 4329 order 4) |
| A UseItem whose item is still in reuse is dropped in silence. | `network/inventory.go` (`live.ItemDisabled`) |
| SoE: a 20 s cast of skill 2013 (RECALL, not magic). Damage does not interrupt it. A move sent mid-cast is answered ActionFailed and **queued** to run when the cast ends. It lands at the nearest town. | `skills/2000-2099.xml:132-139`, `model/actor/cast/controller.go:919-924`, `network/movement.go:71-81`, `network/recall.go:14-30` |
| Enchant is ported. It is safe below +3 (+4 for full-body armor). The item files give each piece an `<enchant stat=...>` gain. Weapon scrolls come only from multisells; armor scrolls only from castle and clan hall stewards. | `enchant/service.go:45-88,558` |
| Armor set skills, shield skills and +6 skills are applied on equip. | `skill/persistence.go:723-830` |
| Item stats are `<for>` children: `<set stat="pAtk" val=.../>` on weapons, `<baseadd stat="pDef" val=.../>` on armor. | `items/0200-0299.xml:2010-2034`, `items/0300-0399.xml:1194-1206` |
| A private store buy needs the buyer within trade interaction distance of the owner. A package store (type 8) sells only whole. | `network/privatestore.go:385-395`; bot `store.go:37` |
| Dyes are ported: `RequestHennaItemList`, `RequestHennaItemInfo`, `RequestHennaEquip`. Neither the list nor the draw checks for a Symbol Maker nearby. Slots by class level are 0 / 2 / 3. Each `hennas.xml` row lists only profession ids. Drawing takes 10 dyes plus the fee (37000 / 87000). First-profession dyes trade +1 of one stat for -3 of another. Dyes (4445+) are on merchant lists (for example npc 30093). | `network/client_dispatch.go:85-90`, `network/henna.go:65`, `network/client_ingame.go:237`, `model/henna/list.go:58-66`, `symbolmaker/symbolmaker.go:82-156`, `hennas.xml:3-15` |
| Class master mod is ported, and `ConfigClassMaster = 1;[];[];2;[];[];3;[];[]` makes it free with no quests. Its NPC 50000 is in no spawnlist. Grand masters need the path quest token (q401-q419 are ported). | `classmaster/`, `network/classmaster.go:91`, `npcs.properties:52` |
| Crystallize, recipe crafting, `RequestPrivateStoreBuy` and private buy stores are ported. | `network/client_dispatch.go:80-100`, `network/privatestore_dispatch.go:54,60` |

## Design

Everything below runs in the brain goroutine, which owns the world model. No new goroutine and no
new shared state, except the read-only item index built once at start (like the travel map).
Every policy value is a configurable persona default (D6). Every new sender states its pacing.

### 1. Item knowledge (`internal/datapack`)

Extend `LoadItems` to cover the etc items the loadout uses. For each, read `default_action` /
`handler` (soulshot, spiritshot, blessed spiritshot, potion, scroll), `crystal_type`, `weight`,
`item_skill`, `reuse_delay`, `is_tradable`. For weapons and armor, read the `<for>` stat block
(`pAtk`, `mAtk`, `pDef`, `mDef`, `pAtkSpd`, `rCrit`, and the `<enchant>` gains) and the
`soulshots` / `spiritshots` counts. Add a skill reader for the potions' effect stack type and order
(section 4).

The hard-coded `healingPotions` and `shotTemplates` lists in `items.go` give way to this index.
The persona's `Potions` and `Shots` stay as overrides. The `armorSets.xml` reader comes with set
scoring (I6).

### 2. The build table

`GearClass` grows into a **build** per class: weapon types, armor type, shield, **stat weights**
(fighters weigh `pAtk` and attack speed, casters `mAtk`, healers `mAtk` and MP), shot kind
(soulshot, or blessed spiritshot falling back to plain spiritshot), buff potion (Alacrity for
fighters, Magic Haste for casters), and, once D15 allows, a profession per race and a dye set.

The race enters through its class tree. Each race has its own professions (human fighter to
warrior, knight or rogue; elven fighter to elven knight or scout; and so on), and the build table
picks one per persona.

### 3. Shots by weapon grade, and the shot factor

- The bot wants shots of the **worn weapon's grade** and its class's kind, only when the weapon has
  capacity for them. While it carries beginner shots of that grade (no grade only), it uses those
  first: they cannot be sold, so selling would waste them.
- Auto is turned **on** for the template in use and **off** (`RequestAutoSoulShot` with 0) for any
  other template the server has on. The server stays silent on later mismatches, so the bot cannot
  rely on it to notice.
- No restock of shots the bot cannot use. Shots of a grade no weapon the bot wears or will wear
  next can use become junk (beginner shots excepted: they are kept until used).
- **Shot factor** (in the same PR, on today's price score): a weapon's value is multiplied by the
  shot factor only when the bot can get shots of its grade. So without D shots, the bot keeps a
  no-grade weapon with shots over a D weapon without them until the D weapon's value wins on its
  own. This is the actual fix for finding 1 under D14 (c).

  "Can get" is a small consumer-side interface, a **shot source** (grade → obtainable). The default
  answers from carried shots and from the lists of merchants that have a spawn in `Travel.spawns`.
  The travel map indexes every buylist with an NPC id, spawned or not (`shop.go` `indexShops`), and
  the unspawned Sales Cat's list sells D shots, so unspawned lists must not count. The clan crafter
  plan (M9) adds "the clan crafter can make this grade now" as a second source.
- Pacing: one toggle per template per change, with a repeat only after `shotRetry` (30 s)
  unanswered, the same gates as `ruleShots` today.

### 4. Consumables sized from use

- A **use meter** per consumable counts what the bot spends per minute of fighting: shots from
  `InventoryUpdate` deltas while in combat, potions per hour at the spot. Its first estimate comes
  from the weapon's shot count and attack speed. It is a decayed average and uses only the injected
  clock.
- The **trip target** for each consumable is use rate × `Supply.TripMinutes` (default 60) plus a
  margin. It is clamped by weight headroom (`SelfState.Load`/`MaxLoad` plus item weights from the
  index; never above `Supply.MaxLoad`, default 60 %) and by the budget (section 5).
- The default list: shots (section 3); the best HP potion a merchant sells (1061 over 1060); 1-2
  Scrolls of Escape; the class's buff potion only when the budget's comfort tier allows it.
- **HP potions** keep rule 3, with a persona threshold that also allows drinking early when two or
  more attackers are on the bot. Pacing: `potionReuse` as today.
- **Buff potions** are drunk at the spot, out of combat, only when no effect of the same stack type
  with an equal or higher order is on the bot. The bot sees only skill ids in its abnormals, so it
  maps them through the skill reader. This covers a support bot's Haste or Acumen and the Newbie
  Guide's beginner buffs, which would make the potion a waste. Pacing: per potion, one UseItem, then
  none until its effect shows in the abnormals or 5 s pass (the server drops a use still in its
  500 ms reuse in silence).

### 5. One budget

Each shop visit plans its spending in priority order, instead of separate restock and gear lists:

1. **Reserve**: a trip home (the gatekeeper price of the trip back, or one SoE).
2. **Supplies** for one trip (section 4), cheapest-sufficient first.
3. **Gear goal**: the next upgrade from section 6. If it is unaffordable but within reach, adena is
   **saved** for it (purchases in tier 4 are skipped).
4. **Comfort**: buff potions, a larger supply margin.

This replaces `shopWants`, `gearWants` and `gearReserve` (`shop.go`, `gear.go`) with one planner.
`ShopPolicy.Restock` stays as an explicit override list; `GearPolicy.KeepAdena` becomes the reserve
floor.

### 6. Gear by stats, enchant and sets

- **Score.** A piece's score is its stats, its enchant gain included, weighted by the build, times
  the shot factor for weapons (section 3). The "never replace an enchanted piece" rule becomes
  "replace it only when the new piece's score is higher, enchant counted." So a +3 D weapon does
  not block a C upgrade. An enchant gain the item file does not give keeps today's rule.
- **Sets.** Armor is scored per **combination**: a complete set adds a bonus valued as a persona
  fraction of its pieces' score (default 15 %), because the set skill's stats are not read. An
  upgrade that breaks a worn set must beat the set including its bonus. Purchases plan the whole set
  when the budget covers it, and otherwise save for it.
- **Grade jump.** Within `Gear.SaveBefore` levels (default 2) of the next Expertise level, the bot
  skips same-grade purchases worth less than a persona fraction of their price in score, and saves
  for the next grade's core pieces (weapon, then chest and legs).
- Kept as they are: bows wait for their arrows, refused templates stay refused, and one UseItem
  waits at a time.

### 7. Supply trip and preparation

- **Going home.** A farmer starts a supply trip when it would run out within
  `Supply.ReturnMinutes` (default 10) of fighting, when its HP potions are gone, when its weight
  passes `Supply.MaxLoad`, or when the saved adena covers the gear goal. It goes only with no
  attacker and nothing aggressive within its aggro range.
- **SoE.** It stands up if sitting, then sends one UseItem of a carried SoE. While the cast runs it
  is in an **escaping hold**: every rule that moves, attacks, loots or clicks returns a hold until
  the teleport (`TeleportToLocation`) or the cast's end. This matters because a move sent mid-cast
  is queued by the server and would run on arrival. If the cast ends with no teleport, it walks.
  Pacing: one UseItem, with no repeat before the cast ends or `escapeRetry`.
- **Without SoE,** it walks or takes a gatekeeper with the travel errand.
- **Spot.** The trip releases its hold in the spot registry, so another bot may take the spot; the
  returning bot picks again, as at a level-up.
- **In town.** Arrival resets `shop.nextAt`, because a miss from the spot would otherwise push the
  first visit up to 5 min out. Then the errands run in the existing order: sell, shop (the budget),
  gear, trainer, Newbie Guide.
- **Preparation.** No farmer leaves town for a spot until its supplies meet the trip target, or the
  budget has spent all it may. A broke bot still goes and farms.
- Dependency: walks go through M7 navigation (#151). This lands after N6 (#158).

### 8. Buying from player stores

Items no spawned NPC sells (Greater Healing Potion, graded shots, gear) are bought from private sell
stores.

- **Who:** a player whose `CharInfo` store type is sell (1). Package stores (8) are skipped.
- **Flow:** walk to within trade interaction distance of the owner, then select the owner to open
  its list. Then send `RequestPrivateStoreBuy` for wanted rows at no more than
  `Market.MaxOverReference` (default 1.5 × reference price, and never above an NPC's price for the
  same item), within the budget. The exact list-open packets are settled in I0.
- **Pacing:** walks and clicks through the frame pacer, one buy request per list window, and a
  per-seller cool-down (default 10 min).
- This also gives the bot merchants buyers, so trading runs both ways.

### 9. Class change and dyes (gated by D15)

- **Class change:** at level 20 and 40, the bot visits the class master and picks the build's
  profession for its race and persona, through the page's bypass links. Pacing: `bypassGap`
  (500 ms). The build table includes the dwarf crafter line the clan crafter plan (M9) needs:
  53 → 56 Artisan (20) → 57 Warsmith (40). That plan owns the Create Item (172) learn data.
- **Dyes:**
  1. **Buy.** The dyes go on the shop errand's list from merchant buylists, under the budget's gear
     tier: 10 per symbol plus the fee.
  2. **Draw.** The bot walks to a Symbol Maker, as a player does, although the server does not
     require one. There it sends `RequestHennaItemList` and waits for the list, then
     `RequestHennaEquip` per symbol into its free slots. One request per answer; bypass links paced
     by `bypassGap`.
  3. **Which dyes.** A persona table per profession that respects the +1/-3 trade and each row's
     class list. For example: warriors STR+1 CON-3 and DEX+1 CON-3; nukers INT+1 WIT-3 or WIT+1
     INT-3; healers (Cleric 15, Oracle 29, Shillien Oracle 42, Shaman 50), whom the INT/WIT rows
     exclude, MEN+1 WIT-3 (`hennas.xml:12`).

### 10. Enchant to the safe level (backlog)

Given finding 8, this is backlog, not M8 core. Enchant scrolls of a grade the bot wears would be
kept. Out of combat, in town, the bot uses them on its best worn piece of that grade up to the safe
level (+3, +4 full-body armor), never past it unless `Enchant.Risk` is set (D6). One enchant at a
time, the next only after `EnchantResult`. It depends on section 6's enchant-aware score.

## Owner decisions

| # | Question | Options | Recommendation |
| --- | --- | --- | --- |
| D14 | Where do D-grade and higher shots come from? | (a) The server adds graded shots to grocer buylists (a data change for every player, not only bots). (b) A Dwarven crafter bot makes them for its clan through the clan warehouse (the clan crafter plan, milestone M9, D17-D19; needs D15). (c) None: bots fight without shots at D+ and keep the better of a weapon with shots and one without. | (c) as the floor now (I2 ships it). Then (a) for the classic shop economy, or (b) for a player-run one. |
| D15 | May bots change class? | (a) Spawn the class master NPC 50000 on the server: the free, quest-free mod is already configured, and it serves every player. (b) Bots run the profession quests (large). (c) Stay out of scope: no professions, no dyes, no crafting. | (a): it unlocks professions, dyes and crafting with one spawn. |
| D16 | Supply trips | Farmers leave their spot to restock (the spot is released and may be taken). | Yes, as in section 7. |

## Testing

- **Datapack (unit):** fixture item files with stats, enchant gains, shots, potions and enchant
  scrolls; potion skills with stack type and order; an armor set file. The reader keeps exactly the
  fields it reads and fails on mangled values, like the existing `items_test.go`.
- **Brain (unit, table-driven, events and injected clock in, packets out):**
  - a worn D weapon turns auto on only for the D template, and off for a no-grade one that is on;
  - beginner shots go first and are never junk;
  - no D shots on sale: the bot keeps a no-grade weapon with shots;
  - an unspawned merchant's list (the Sales Cat's D shots) gives no shot factor;
  - the use meter after N `InventoryUpdate` deltas sets the trip target, and weight caps it;
  - the budget tiers: reserve, supplies, gear saving, comfort;
  - no buff potion while Haste or a beginner Haste is on;
  - a buff potion gated until its abnormal arrives;
  - a supply trip starts at its threshold, stands, reads SoE, and holds every mover until the
    teleport, then resets `shop.nextAt` on arrival;
  - no departure before the trip target is met;
  - set scoring keeps a full set over a pricier mixed piece;
  - a +3 D weapon gives way to a better C weapon;
  - the grade-jump saving;
  - a store buy walks into range first, skips package stores, and respects the price caps.
- **e2e (live harness):** a level-20 fighter keeps or wears the right weapon for its shot supply; a
  farmer runs low, reads SoE, restocks, and comes back; the class master and dyes under D15.

## Delivery (milestone M8, "Items and loadout")

| PR | Scope | Depends on |
| --- | --- | --- |
| P0 | After the owner's answers: `docs/plan.md` gains the M8 milestone row and D14-D16, and the class change note is amended under D15. Server issues in acis_golang for D14 (a) (graded shots on grocer lists) and D15 (a) (spawn NPC 50000), if chosen. | D14-D16 |
| I0 | Settle the remaining server facts: private store list open, class master pages, the Symbol Maker flow. Notes go into this doc. | — |
| I1 | Datapack: etc items, gear stats and enchant gains, potion skill stack types. Replace the hard-coded potion and shot lists. | — |
| I2 | Shots by weapon grade, beginner shots first, blessed spiritshots for casters, no unusable shots bought, the shot factor on the price score (findings 1, 2). | I1 |
| I3 | Use meter and trip targets (section 4). | I1 |
| I4 | One budget replacing `shopWants`, `gearWants` and `gearReserve` (section 5). | I3 |
| I5 | Gear score by stats and enchant, replacing price and the never-replace-enchanted rule (section 6). | I1, I2 |
| I6 | Armor set reader and set-combination scoring and buying. | I5, I4 |
| I7 | Grade-jump saving. | I5, I4 |
| I8 | Supply trip, SoE with the escaping hold, preparation (section 7). | I4, M7 N6 |
| I9 | Better HP potion and buff potions with the stack-type check and gate. | I1, I4 |
| I10 | Buying from private sell stores (section 8). | I0, I4 |
| I11 | Class change at the class master with a race and persona build table. | D15 (a), P0, I0 |
| I12 | Dye purchase and drawing. | I11, I4 |
| I13 | e2e live scenarios for I2-I12. | each |

Most urgent: I2 (every bot at level 20 today), then I3, I4 and I8 (farmers never restock).

## Risks

- **Adena.** Shots and buff potions can cost more than low-level farming earns. The budget's tiers
  and the use meter keep spending inside income. A broke bot still farms.
- **SoE.** Damage does not interrupt SoE, but a bot reading it while monsters close in takes 20 s
  of hits. It reads only with no attacker and nothing aggressive in range. The escaping hold must
  cover every mover, or the server runs a stale queued move on arrival.
- **Spot churn.** Released holds may be taken during a trip. The returning bot picks again.
- **Player stores.** Bots could buy out a player's shop. The price caps and per-seller cool-down
  bound this, and buying from players is allowed under D6.
- **Score weights** are a heuristic. The weights are persona defaults, tested on fixed fixtures.

## Gaps

- Mana Potion (728), Greater Healing Potion (1539) and CP potions are not on any spawned list. Bots
  buy them only from player stores (I10).
- Weapon enchant scrolls come only from multisells, and enchanting is backlog (section 10).
- Dwarven crafting of graded shots (D14 b) is the clan crafter plan (M9), not an M8 PR.

## Interfaces the clan crafter plan (M9) consumes

M9 builds on M8 rather than duplicating it. M8 exposes these and adds no clan logic itself:

- the **shot source** interface of section 3, to which M9 adds the crafter;
- the **per-item trip target** of I3, which sizes a clan member's orders;
- the budget's **reserve tier** (I4), which carries the warehouse fees;
- a **"trip in town" hook** in I8, when a member deposits materials and collects products;
- a persona **keep hook** on the junk rule (I1/I4), which M9 fills from its recipe index so that
  members keep crafting materials instead of selling them. I1 exposes the material etc items for
  this.
