# Bot items: loadout, supplies and gear like a real player

Status: approved by the owner on 2026-11-21: D14 (b), D15 (a), D16 yes (see "Owner decisions").
Filed as milestone M8, epic #166, issues #167-#182, and server issue fatal10110/acis_golang#5419.

Five rounds of independent review; the fifth found the final doc and the filed issues sound.

- **Round 1:** SoE casts are queued, not cancelled, by moves; enchanting would freeze gear
  upgrades; buff potion stacking and pacing were wrong; the shot fix sat in the wrong PR; beginner
  shots were wasted; player stores need a walk and exclude package stores; several PRs were too
  large.
- **Round 2:** unspawned merchants' lists must not count as a shot source.
- **Round 3** (a fresh reviewer, over the final doc and the filed issues):
  - enchant gains are a server formula, not item data;
  - junk rules would sell the new consumables;
  - carried-shot and weight thresholds that flip back and forth;
  - the profession trainer tables and the class report;
  - per-level stack orders;
  - splits and dependency fixes in the issues.
- **Round 4:**
  - the class is saved by extending `keepProgress`;
  - first-profession dyes go dead at the second profession;
  - the keep set across PRs;
  - the keep hook's three callers and may-wear;
  - kept weight looping supply trips;
  - extra supplies outside the gate.
- **Round 5:** wording and one dependency order.

Everything found is applied here and in the issues.

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
| Gear | Wears and buys the best piece per slot by **reference price**, of the class's weapon and armor type (`defaultGearClasses`), up to the Expertise grade. Purchases keep 10000 adena (`KeepAdena`). An enchanted piece is replaced only by a piece of a higher score, its enchant counted (section 6). Bows wait for their arrows. | `gear.go` |
| Shots | Turns on auto-use for **every** shot template it carries (`shotTemplates`, all grades). Restocks only **no-grade** soulshots (fighters, 100-500) or no-grade spiritshots (mystics, 50-200). | `items.go` `ruleShots`, `shop.go` `defaultShopPolicy` |
| Potions | Drinks the best of 1539, 1061, 1060 below 20 % HP in combat. Restocks only Lesser Healing Potion (5-20). | `items.go` `drinkPotion`, `rules.go` `ruleCritical` |
| Arrows | Grade-matched arrows for the worn bow and for the next bow, 200-1000. This is done right. | `arrows.go` |
| Restocking | Only when a merchant is in sight or its spawn is within `Shop.Reach` (8000), at most every 5 min. A miss pushes the next try 5 min out. | `shop.go` `startShop` |
| Selling | Everything not kept is junk and goes to an NPC merchant. Enchant scrolls of a grade and kind the bot wears are kept (section 10). Merchants sell loot in their store. | `shop.go` `junk`, `store.go` |
| Enchant | Enchants the worn gear to the safe level in town (section 10). | `enchant.go` |
| Dyes, class change, crafting, buying from player stores | None. | — |

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
9. **Specialization stops at the base class, and dyes need a profession.** Class change was out of
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
| Buff potions: 500 ms reuse, 1200 s effect. Alacrity (skill 2012) is `attack_time_down` order 1, the same type as Haste (1086) and Haste for Beginners (4327, Newbie Guide). Magic Haste (2169) is `casting_time_down`, the same type as Acumen (1085) and Acumen for Beginners (4329). The higher order wins. Some skills give the order per level through a `#stackOrder` table (Haste 1086, Magic Haste 2169), so the order depends on the effect's level. | `skills/2000-2099.xml:127`, `skills/2100-2199.xml:714,725`, `skills/1000-1099.xml:1548,1566,1586`, `skills/4300-4399.xml` (4327 order 2, 4329 order 4) |
| Weight penalty starts at 50 % of the limit (then 66.6 %, 80 %, 100 %). Regen is halved from 50 %, move speed from 66.6 %, and the character cannot move from 100 %. Attack speed does not change. | `model/actor/player/character_weight_penalty.go:48-52,80-99` |
| Grade penalty: every worn piece above the Expertise (skill 239) level counts, arrows excepted. Each armor piece, a full armor twice, up to 4, slows move and cast speed by 16 % (0.84 per count) and costs 2 evasion; an over-grade weapon costs 20 accuracy. While it holds, skill 4267 is at level 1. A change sends the bot SkillList and EtcStatusUpdate; a weight band change sends UserInfo and EtcStatusUpdate. Another player's EtcStatusUpdate is broadcast too, so the bot reads the bands from the self-only frames. | `model/actor/player/character_grade_penalty.go:22-70`, `character_attack.go` (`moveSpeedFrom`), `network/live_player_events.go` (`GradePenaltyChanged`, `sendLiveWeightPenalty`) |
| A UseItem whose item is still in reuse is dropped in silence. | `network/inventory.go` (`live.ItemDisabled`) |
| SoE: a 20 s cast of skill 2013 (RECALL, not magic). Damage does not interrupt it. A move sent mid-cast is answered ActionFailed and **queued** to run when the cast ends. It lands at the nearest town. | `skills/2000-2099.xml:132-139`, `model/actor/cast/controller.go:919-924`, `network/movement.go:71-81`, `network/recall.go:14-30` |
| Enchant is ported. It is safe below +3 (+4 for full-body armor). Every `<enchant>` entry in the item files has `val="0"`: the gain is a server formula keyed by stat, grade and weapon type (`applyEnchant`), and `<enchant>` only names the stats an enchant raises. Weapon scrolls come only from multisells; armor scrolls only from castle and clan hall stewards. | `enchant/service.go:45-88,558`, `skill/effect/mod.go:175-230`, `items/0200-0299.xml:2032-2033` |
| Armor set skills, shield skills and +6 skills are applied on equip. | `skill/persistence.go:723-830` |
| Item stats are `<for>` children: `<set stat="pAtk" val=.../>` on weapons, `<baseadd stat="pDef" val=.../>` on armor. | `items/0200-0299.xml:2010-2034`, `items/0300-0399.xml:1194-1206` |
| A private store buy needs the buyer within trade interaction distance (150, 3D) of the owner. A package store (type 8) sells only whole. A store list opens through the second `Action` on the owner, not a packet of its own. The full flow is under "Buying from a private sell store" below. | `network/privatestore.go:389-398`, `network/trade.go:15`; bot `store.go:37` |
| Dyes are ported: `RequestHennaItemList`, `RequestHennaItemInfo`, `RequestHennaEquip`, and the unequip trio. Neither the list nor the draw checks for a Symbol Maker nearby. Slots by class level are 0 / 2 / 3. Each `hennas.xml` row lists only profession ids. Drawing takes 10 dyes plus the fee. The fee is 37000, 87000 or 149000 for the first-profession symbols 1-36, and 87000 to 628000 for symbols 37+. First-profession dyes trade +1 of one stat for -1, -2 or -3 of another, and the dearer the dye, the smaller the loss. Dyes (4445+) are on merchant lists (for example npc 30093). The full flow is under "Drawing dyes" below. | `network/client_dispatch.go:85-90`, `network/henna.go:65`, `network/client_ingame.go:237`, `model/henna/list.go:58-66`, `symbolmaker/symbolmaker.go:82-160`, `hennas.xml:3-40` |
| Class master mod is ported, and `ConfigClassMaster = 1;[];[];2;[];[];3;[];[]` makes it free with no quests. That value is a deployment setting: the code default is empty, so no tier is offered. Its NPC 50000 stands in every town (`spawnlist/custom_classmaster.xml`, fatal10110/acis_golang#5419). Grand masters need the path quest token (q401-q419 are ported). The full flow is under "Changing class at the class master" below. | `classmaster/settings.go:39-66`, `network/classmaster.go:91`, `npcs.properties:52`, `internal/config/testdata/reference_defaults.golden:106` |
| Crystallize, recipe crafting, `RequestPrivateStoreBuy` and private buy stores are ported. | `network/client_dispatch.go:80-100`, `network/privatestore_dispatch.go:54,60` |

### Flows settled in I0

Checked in the server and the datapack on 2026-11-22 (#167). Server paths are relative to
`internal/gameserver/`, datapack paths to `aCis_datapack/data/`. Every integer is little-endian
int32 unless stated. Every request below runs on the player's queue. None of them has a flood
protector of its own, only the frame bucket.

#### Buying from a private sell store

1. **Find the owner.** `CharInfo` store type 1 (sell). Skip 8 (package sell). 3 (buy) and 5
   (manufacture) are also `Operating()`, but they open other windows.
2. **Select.** The client sends `Action` (0x04): objectId, originX, originY, originZ, then uint8
   shift, 17 bytes in all (`network/clientpackets/targetaction.go:9-23`; bot
   `brainpackets.go:33`). The first `Action` only selects. The server answers `ValidateLocation`,
   then `MyTargetSelected` (`network/targeting.go:48`, `selectLiveTarget` :848-878).
3. **Open the list.** A second `Action` on the same target (`actOnTargetAgain` :105,
   `actOnSelectedTarget` :145) reaches `actOnPlayer` (`network/player_follow.go:24-41`). Its cases
   run in this order:
   - across a boat boundary: `ActionFailed`;
   - an owner the bot may attack without force (karma or flagged): the bot **attacks** it. So the
     bot must never click a flagged or karma owner;
   - `Operating()` (`privatestore/store.go:31`): interact;
   - anyone else is followed.

   The interact (`tryToInteract` :473, `thinkInteract` :564-601) first sends `ActionFailed`. It
   needs the bot standing and not flying. Beyond `interactApproachOffset` (100) plus both collision
   radii (:379, :570), the server walks the bot there itself (`MoveToPawn`, offset 100), unless
   shift is held. In reach (`interactionDistance` 150, :383) it broadcasts the bot facing the owner
   (`MoveToPawn`) and opens the store (`onInteract` :618-621 → `showPrivateStore`,
   `network/privatestore.go:544-570`). A store still being set up opens nothing.
4. **The list.** `PrivateStoreListSell` (0x9b, `network/serverpackets/privatestore.go:13,48-97`):
   - header: ownerId, packaged (0/1), buyer's adena, row count;
   - per row: type2, objectId, itemId, count, uint16 0, uint16 enchant, uint16 0, body part slot,
     price, reference price.

   Rows whose template is missing are dropped. A buy store answers `PrivateStoreListBuy` (0xb8)
   instead, and a manufacture store the recipe shop list. The bot already decodes the sell list
   (`internal/botclient/store.go:156-185`).
5. **Buy.** `RequestPrivateStoreBuy` (0x79, `network/clientpackets/privatestore.go:15,136-157`):
   storeId (the owner's object id), count, then per row objectId, count, unit price.
   - Each row needs objectId and count of at least 1 and a price of at least 0.
   - The row count is capped at `MaxItemInPacket` (`network/privatestore_dispatch.go:15`), and
     `count × 12` must equal the bytes left.
   - A malformed request decodes to no rows and is dropped in silence.
   - Bot encoder: `internal/botclient/store.go:203-211`.
6. **Refusals** (`buyFromStore`, `network/privatestore.go:407-442`; `Store.Buy`,
   `privatestore/deal.go:87-185`). Buying is all rows or nothing, and every refusal is silent
   unless named:
   - no rows;
   - the buyer is dead or holds a cursed weapon;
   - the owner is unknown, dead, or farther than 150 in 3D (`storeOwner` :389-398);
   - the owner no longer sells (`OperateSell` or `OperatePackageSell`);
   - the buyer's access level lacks `AllowTransaction`: `SystemMessage` NotAuthorizedToDoThat;
   - a row that names an objectId not listed, or not at **exactly** the listed price
     (`sellRowLocked` :278-285). So a changed price or a sold-out row refuses the whole deal;
   - a count above the listed count (summed per object), or a total above MaxInt32;
   - a package store asked for less than every row in full;
   - an owner's item that can no longer be moved (bound, augmented and the like);
   - not enough adena: `SystemMessage` YouNotEnoughAdena; then weight: WeightLimitExceeded; then
     slots: SlotsFull. They are checked in that order (`privatestore/deal.go:162-173`, `answerDeal`
     `network/privatestore.go:483-499`).
7. **Success.**
   - One `SystemMessage` per row to the buyer (`storeDealFrame` :519-540): PurchasedS3S2SFromS1
     for a stackable item (seller, item, count), PurchasedS2S3FromS1 for an enchanted one,
     PurchasedS2FromS1 otherwise. The owner gets the mirror messages.
   - The bought items and the adena come in the batched `InventoryUpdate` of the next 333 ms
     inventory tick (`task/inventoryupdates.go:13,91-113`).
   - The list is not sent again.
   - A store left empty closes: the owner gets `UserInfo`, and everyone around a `CharInfo` with
     store type 0 (`closeSoldOutStore` :504-514). Other buyers learn it only from that `CharInfo`.

For the bot (section 8): walk to within 150 of the owner on its own navigation, then send two
`Action`s. Buy at the listed price exactly; the reference price is only a guide. Partial counts of
stackable items are fine in a sell store. Send one request per list.

#### Changing class at the class master

NPC 50000 (Hasha, the Class Manager, `xml/npcs/50000-50999.xml:3`). Its pages are
`html/mods/classmaster/50000.htm` and `50000-1.htm` to `50000-6.htm`
(`model/actor/npc/folk_chat.go:95`).

On the wire it is not 50000: `NpcInfo` carries the template's `idTemplate`, 31228, the look of
Roy the Cat (an Event Manager in most villages, one about 1190 from Talking Island's master). The
master uses its server-side name and title, so its `NpcInfo` names it "Hasha", while Roy's name
is empty (`model/actor/npc/folk.go` `infoSnapshot`). The bot tells them apart by display id and
name (`NPCInfo.Template`, #343).

1. **Talk.** The same two `Action`s as any folk NPC. The answer is `NpcHtmlMessage` (0x0f,
   `network/serverpackets/npchtmlmessage.go:10`) with `50000.htm`. Its links are
   `bypass -h npc_%objectId%_1stClass`, `_2ndClass`, `_3rdClass`, `_become_noble` and
   `_learn_skills` (`50000.htm:4-9`).
2. **Bypass rules** (`RequestBypassToServer`, `network/bypass.go:115-166`):
   - The command must be a link on the **last page sent** (`bypassWhitelist.allows` :65, :139).
     Anything else is dropped in silence.
   - The NPC must be within `interactionDistance` (150, `playerCanDoInteract`,
     `network/targeting.go:725-730`).
   - The handler closes with `ActionFailed`.
   - The server's bypass gap (`ServerBypassTime`) is 100 ms. The bot's `bypassGap` (500 ms,
     `pacing.go:26-30`) is wider.
3. **Commands** (`classmaster/command.go:42-62`, routed by `model/actor/npc/folk_bypass.go:369` to
   `network/classmaster.go:15-46`):
   - `1stClass`, `2ndClass` and `3rdClass` ask for the menu of tier 1, 2 or 3;
   - `change_class <id>`: a malformed id stops the request with nothing sent;
   - `become_noble` and `learn_skills`.
4. **Menu page** (`MenuPage`, `classmaster/dialog.go:84-122`). The change levels
   (`changeLevel` :33-44) are 20 from a base class, 40 from a first profession and 76 from a second.
   - A tier the config does not offer: no change.
   - The class is already at or past the tier asked: page `-1` (no change).
   - At or above the level: page `-2`, the menu. It has one
     `<a action="bypass -h npc_%objectId%_change_class N">Name</a>` per class of that tier that is
     a **direct** child of the current class (`canTransfer` :48-57; the whole line only with
     `AllowEntireTree`). The page also fills `%req_items%`.
   - No entry, or too low a level: page `-3` ("come back at level N").

   So a base class at 20 sends `1stClass`, and a first profession at 40 sends `2ndClass`. The bot
   reads N from the menu's links. It never builds one: a link not on the page is dropped.
5. **Transfer refusals** (`classMasterTransfer`, `network/classmaster.go:53-98`; `CheckTransfer`,
   `classmaster/transfer.go:39-66`):
   - level below the change level, or a class that is not a child: silent, no page;
   - weight penalty above 2 when the tier gives rewards: `SystemMessage` InventoryLessThan80Percent;
   - required items missing: `SystemMessage` NotEnoughItems;
   - a tier not configured: the request stops with nothing sent, not even `ActionFailed`.
6. **Success**, in this order:
   - required items destroyed and rewards given;
   - `MagicSkillUse` of skill 5103 on the bot, broadcast (`network/occupation.go:11,28-30`);
   - `SystemMessage` ClassTransfer (ThirdClassTransfer at the third), and the party and clan
     window rows;
   - the skill list, **only** when `AutoLearnSkills` is on. It is off by default
     (`internal/config/testdata/reference_defaults.golden:139`), so no `SkillList` comes;
   - the base class is set (not on a subclass), then `HennaInfo` (0xe4), then `UserInfo` with the
     new class id, and `CharInfo` to everyone around (`broadcastCharacterInfo`,
     `network/inventory.go:917-921`);
   - `NpcHtmlMessage` with `50000-4.htm` (changed), then `ActionFailed`.
7. **`learn_skills`** (`network/classmaster.go:42-43` → `rewardLiveSkills`,
   `network/occupation.go:59-68`; `RewardSkills`, `skill/leveling.go:54-71`) grants every skill
   the class and level unlock and charges no SP. It sends the raised skills (shortcuts, then
   `SkillList`) and no page.
8. **Spawn.** NPC 50000 is in no spawnlist. Until fatal10110/acis_golang#5419 lands, the flow can
   only be tested on a GM-spawned master.

#### Drawing dyes

No request below checks for a Symbol Maker or an NPC distance (`network/henna.go:11-14`,
`network/client_ingame.go:236-242`).

| Client packet | Opcode | Body | Answer |
| --- | --- | --- | --- |
| `RequestHennaItemList` | 0xba | none | `HennaEquipList` 0xe2 |
| `RequestHennaItemInfo` | 0xbb | symbolId | `HennaItemInfo` 0xe3 |
| `RequestHennaEquip` | 0xbc | symbolId | see below |
| `RequestHennaUnequipList` | 0xbd | none | `HennaUnequipList` 0xe5 |
| `RequestHennaUnequipInfo` | 0xbe | symbolId | `HennaItemUnequipInfo` 0xe6 |
| `RequestHennaUnequip` | 0xbf | symbolId | see below |

Opcodes and bodies: `network/clientpackets/henna.go:11-16,44-70`,
`network/serverpackets/henna.go:10-13`, `network/serverpackets/hennainfo.go:10`. Dispatch:
`network/client_dispatch.go:85-90`.

1. **List.** `HennaEquipList` (`network/serverpackets/henna.go:34-47`): adena, max slots, count,
   then per symbol symbolId, dyeId, 10 (the dyes a draw takes), price, 1. It lists only symbols the
   class may use **and** whose dye the bot carries at least one of (`Drawable`,
   `symbolmaker/symbolmaker.go:32-44`). Without dyes the list is empty.
2. **Slots.** `henna.MaxSlots(classTier)` (`model/henna/list.go:58-66`): 0 for a base class, 2
   for a first profession, 3 after. A base-class bot cannot draw at all, so the class change comes
   first.
3. **Draw refusals** (`drawHenna`, `network/henna.go:65-89`; `Draw`,
   `symbolmaker/symbolmaker.go:82-117`), in order:
   - unknown symbol id: silent;
   - class not allowed: `SystemMessage` CantDrawSymbol;
   - no free slot: SymbolsFull;
   - fewer than 10 of its dye (`henna.DrawAmount`, `model/henna/henna.go:7`): CantDrawSymbol
     again, so a missing dye and a class refusal look the same;
   - not enough adena: YouNotEnoughAdena.
4. **Draw success.** The adena is paid **before** the dyes are taken. The bot gets `SystemMessage`
   S1DisappearedAdena (when the price is above 0), then S2S1Disappeared (the dye, 10). Then comes
   `HennaInfo` (0xe4, `network/serverpackets/hennainfo.go:15-45`): uint8 INT, STR, CON, MEN, DEX,
   WIT, then max slots, count, and per symbol symbolId, then activeSymbolId (the same id when the
   class can use the dye, else 0; `model/henna/list.go:31`). Then `UserInfo` (`sendHennaChange`
   :129-132), then SymbolAdded. The symbol takes the first free slot.
5. **Remove.** `RequestHennaUnequipList` always answers the list, even an empty one
   (`network/client_ingame.go:245-250`, `sendHennaUnequipList` `network/henna.go:35-37`).
   `RequestHennaUnequip` (`deleteHenna` :95-115, `Delete`, `symbolmaker/symbolmaker.go:134-160`)
   costs a fifth of the draw price (`model/henna/henna.go:27`). Refusals, in order
   (`symbolmaker/symbolmaker.go:135-144`):
   - a symbol not worn: silent;
   - not enough adena: YouNotEnoughAdena.
   - Success: `HennaInfo`, `UserInfo`, `SystemMessage` YouPickedUpS2S1 (5 dyes back), then
     SymbolDeleted.
6. **Data** (`xml/hennas.xml`, 180 symbols).
   - Symbols 1-36 list only first-profession ids (`hennas.xml:3-38`). Symbols 1-12 cost 37000
     (+1/-3), 13-24 cost 87000 (+1/-2), and 25-36 cost 149000 (+1/-1).
   - Symbols 37+ list second and third-profession ids, with stronger rows (for example symbol 73:
     STR+2 CON-3 for 190000, `:75`).
   - Draw prices across the file: 12 × 37000, 24 × 87000, 24 × 149000, 12 × 176000, 12 × 190000,
     36 × 206000, 36 × 258000, 12 × 387000, 12 × 628000.
   - The per-profession table (I12 #179) re-checks each symbol's price and class list.

## Design

Everything below runs in the brain goroutine, which owns the world model. No new goroutine and no
new shared state, except the read-only item index built once at start (like the travel map).
Every policy value is a configurable persona default (D6). Every new sender states its pacing.

### 1. Item knowledge (`internal/datapack`)

Extend `LoadItems` to cover the etc items the loadout uses. For each, read `default_action`,
`handler` and `etcitem_type`. Potions and SoE share `default_action` `skill_reduce`, so only
`etcitem_type` tells them apart. Also read `crystal_type`, `weight`, `item_skill`, `reuse_delay`,
`is_tradable` and `is_sellable` (beginner shots are blocked by `is_sellable`).

For weapons and armor, read the `<for>` stat block (`pAtk`, `mAtk`, `pDef`, `mDef`, `pAtkSpd`,
`rCrit`), which stats `<enchant>` names, and the `soulshots` / `spiritshots` counts. The enchant
gain itself is a copy of the server's `applyEnchant` formula (stat, grade, weapon type, +3 cap with
the over-enchant rate), owned here (D5), because every `<enchant>` value in the files is 0.

Add a skill reader for the effects' stack type and order, by skill id **and level** (`#stackOrder`
tables), for section 4.

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
  next can use become junk (beginner shots excepted: they are kept until used). For that,
  `Persona.Shots`, which today defaults to all 19 shot templates and keeps them all out of junk
  (`rules.go`, `shop.go` `keeps`), defaults to nothing: the wanted grade is derived. A persona
  that lists templates still keeps them.
- **Shot factor** (in the same PR, on today's price score): a weapon's value is multiplied by the
  shot factor only when the bot can get shots of its grade. So without D shots, the bot keeps a
  no-grade weapon with shots over a D weapon without them until the D weapon's value wins on its
  own. This is the fix for finding 1 until the clan crafter (M9, D14 b) supplies graded shots.

  "Can get" is a small consumer-side interface, a **shot source** (grade → obtainable). The default
  answers from the lists of merchants that have a spawn in `Travel.spawns`, and from carried shots
  only when they reach the trip target (I3). A few looted D shots must not switch the bot to a D
  weapon it cannot keep charged. While a weapon's grade is obtainable only from carried shots, the
  weapon it replaced is kept, not junk, so the bot can switch back when the shots run out.
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
  index; never above `Supply.BuyLoad`, default 45 %, below the server's first weight-penalty band at
  50 %) and by the budget (section 5).
- The default list: shots (section 3); the best HP potion a merchant sells (1061 over 1060); 1-2
  Scrolls of Escape; the class's buff potion only when the budget's comfort tier allows it.
- **HP potions** keep rule 3, with a persona threshold (`CrowdedHP`, default 80 %, above
  `OutmatchedHP`) that also allows drinking early when two or more attackers are on the bot; the
  heal and the flee stay under `CriticalHP`. Rules 3 and 4 do not flee while an attacker runs as
  fast as the bot or faster (`FleeOutrun`, #281): the bot fights back instead. Pacing:
  `potionReuse` as today.
- **Buff potions** (`GearClass.BuffPotion`, rule `buffpotion` after the rebuff) are drunk at the
  claimed spot, standing, out of combat with no attacker, only when no effect of the same stack type
  with an equal or higher order is on the bot. The bot's effects carry skill id and level
  (`support.go` `Effect`, kept in `World.BuffLevels`), which it maps through the skill reader to
  stack type and order at that level (`TravelMap.stackOf`; an enchanted level stacks as the top
  plain one). This covers a support bot's Haste or Acumen and the Newbie Guide's beginner buffs,
  which would make the potion a waste. Pacing: per potion, one UseItem, then none until its effect
  shows in the abnormals (its own stack then stops the next) or 5 s pass (`buffPotionWait`; the
  server drops a use still in its 500 ms reuse in silence). The comfort tier buys one per effect
  time of `TripMinutes`, margin included, within the weight the supplies leave under `BuyLoad`.
- **Death penalty** (#299, `RecoverPolicy.LiftDeathPenalty`, rule `recover` after the buff
  potion). The level (0-15) is `EtcStatusUpdate`'s seventh int32 (0xf3). That packet has no object
  id, and the server also broadcasts another player's to everyone in view when its Charm of Courage
  starts or ends. So the bot takes the level only from the first one of the session, sent at
  EnterWorld before the spawn. After that it takes the level from the system messages sent only to
  the bot with each change: 1916 carries the new level, 1917 means it is lifted. A death raises
  it by one at `DeathPenaltyChance`, and it never decays. Each Scroll: Recovery lifts one level and
  fits one character level band: 8594 under 20, then 8595-8599 from 20, 40, 52, 61 and 76 (item
  skill 2286; a scroll outside its band is refused). The supplies tier buys one scroll of the bot's
  band per level, less those carried, and the band's scroll is in the keep set. The bot reads one
  anywhere, standing, out of combat with no attacker. Pacing: one UseItem per 11 s
  (`recoveryReuse`), past the scroll's 10 s reuse, which the server enforces in silence. Black
  Judge (npc 30981, `Quest BlackJudge remove_dp`) does the same for adena, but has only three
  spawns, so the bot does not use it.

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

The junk rule (`keeps`) learns everything the loadout buys. Today it keeps only `Potions`, `Shots`,
`Restock`, `Keep`, books and arrows, so a sell visit would sell the SoE, buff potions and dyes the
bot just bought. It now keeps every template with a trip target (section 4), plus a **keep set**
that later PRs add to: the build's buff potion (I9), the planned dyes until they are drawn
(I12), and the Scroll: Recovery of the bot's level band (#299), each tested there.

A persona **keep hook** lets the clan crafter plan (M9) keep more, with no clan logic in M8:

- **Input.** It is called per carried item (the `Item`, object and template), so it can keep
  crafting materials, every recipe scroll, and unworn graded gear deposited for crystallizing.
- **Callers.** Three places decide what the bot lets go of, and all three consult it: the NPC sale
  (`shop.go` `junk`), the store listing (`store.go` `storeLoot`), and trade pricing (`trade.go`
  `tradePrice`). A kept item is never sold, listed or offered.
- **Result.** It answers keep, and whether the gear errand may still wear the item, so a piece
  kept for crystallizing is deposited, not worn.

A persona **extra supply hook** (template → count) adds supply targets beyond the use meter's.
M9 uses it for the ore its members' shot orders need (Soul Ore 1785, Spirit Ore 3031, on spawned
grocer lists).

- I4 buys them in the supplies tier **after** the bot's own shots, potions and SoE, and `keeps`
  keeps them.
- They count toward neither the preparation gate nor the run-out trigger (section 7), so an
  unfilled order never holds a farmer in town or sends it home.

### 6. Gear by stats, enchant and sets

- **Score.** A piece's score is its stats, its enchant gain included, weighted by the build, times
  the shot factor for weapons (section 3). The "never replace an enchanted piece" rule becomes
  "replace it only when the new piece's score is higher, enchant counted." So a +3 D weapon does
  not block a C upgrade. The enchant gain is the copied server formula (section 1).
- **Sets.** Armor is scored per **combination**: a complete set adds a bonus valued as a persona
  fraction of its pieces' score (default 15 %), because the set skill's stats are not read. An
  upgrade that breaks a worn set must beat the set including its bonus. Purchases plan the whole set
  when the budget covers it, and otherwise save for it.
- **Through the model (M10 S5, #324; `docs/strategy.md` Design section 5).** Where the travel map
  holds the bot's class template (every datapack-built map), a piece's score is the character
  model's value gain of wearing it (`gear_model.go` `pieceScore`): the value of the arrangement with
  the piece over the arrangement with its slot empty, as a fraction of the value of what the bot
  wears now, against the reference monster of its level (the median-HP monster template,
  `indexRefMobs`), calibrated by the stats `UserInfo` reports. The value is the build role's
  (`charmodel.Value`): kills a minute of the first damaging rotation skill the bot knows, else its
  melee; a Support build's heal per MP bar while it survives; a Support build without a heal is a
  tank and values how long it lasts. A fight the bot loses still ranks gear by how close it comes
  (time to kill over time to die). The `StatWeights` score is kept as the tie-break
  (×10⁻⁶: jewelry's M. Def against a melee monster, anything a heal's value does not see) and as the
  whole score without the model. Weapon kinds still come from the kit; the shot factor and the
  enchant gain stay (the enchant gain raises the template's stats the model reads).
  - A set's bonus is the value its set skill adds (and its shield skill with the shield worn), so a
    Demon set (−270 max HP) is worth less than a Doom Robe set to a low-HP dark mystic;
    `Gear.SetBonus` only adds its fraction of the stat score as a tie-break.
  - Armor kinds come from the class (`GearClass.Armor`) and the race profile
    (`RaceProfile.MysticArmor`): an orc mystic's Support build may wear light armor once it knows
    Light Armor Mastery (from 7) and heavy armor once it knows Heavy Armor Mastery (from 14), when
    they score higher; every other mystic stays in robes.
  - Chest and legs are a pair: a single piece of another kind than the other worn one is never
    worn or bought (the server counts a chest's kind only with legs of the same kind). The bot
    changes kinds only by a whole pair, or a set covering both, planned and bought like a set
    (`pairPlans`).
- **Grade jump.** Within `Gear.SaveBefore` levels (default 2) of the next Expertise level, the bot
  skips same-grade purchases worth less than a persona fraction of their price in score, and saves
  for the next grade's core pieces (weapon, then chest and legs).
  - The Expertise levels are a bot-owned table (20, 40, 52, 61, 76 for D to S, the class files'
    skill 239 `minLvl`), capped by `Gear.MaxGrade`.
  - The jump goal is what the bot would buy once it wears the next grade: the best weapon of that
    grade, then, once one is carried, its set, else its best body piece and legs. It is saved for
    through the budget's gear goal (section 5), so a visit with it within reach skips the comfort
    tier. It is never bought before the jump.
  - A piece (or set) of the current grade is bought in the window only when its score gain over
    what it replaces, as a fraction of that score, is at least `Gear.SaveWorth` (default 1) times
    its price as a fraction of the jump goal's. A Falchion over a Long Sword (+29 % for 38 % of an
    Artisan's Sword) waits, a Falchion over a Short Sword or a cheap shirt does not. The shot factor
    and set bonus are left out of that comparison.
- Kept as they are: bows wait for their arrows, refused templates stay refused, and one UseItem
  waits at a time.

### 7. Supply trip and preparation

- **Going home.** A farmer starts a supply trip when any of these holds:
  - it would run out within `Supply.ReturnMinutes` (default 10) of fighting;
  - its HP potions are gone;
  - its load passes `Supply.ReturnLoad` (default 60 %, well above the 45 % `BuyLoad`, so a bot
    that bought to the cap does not turn round at its first loot, and under the server's
    half-speed band at 66.6 %, so it walks home at full speed) **and** a town visit would shed
    at least `Supply.ShedMin` (default 10 % of the limit). Shed weight is the junk it would sell
    (I8); I8b adds what the "trip in town" hook reports it will hand off. So items kept with
    nowhere to go do not send it home after every return. In the half-speed band (66.6 % and up,
    a GM grant, a trade or a lower limit after a level change put it there) anything to shed
    sends it home, under `ShedMin` too (#436);
  - the saved adena covers the gear goal.

  A load the trip cannot shed must not slow the bot down either. It gives up a queued pickup that
  would bring its load to `Loot.StopAbove` (default 65 %, under the 66.6 % band), weighing the
  item by its template's weight times its count, and drops its `Loot.Junk` from `Loot.DropAbove`
  (default 65 %). An item that weighs nothing, such as adena, is still picked up. The server's
  pickup checks only free slots, never weight (`network/pickup.go:76-78`), and past 100 % the bot
  cannot move at all, so the bot caps itself. The gear it buys stays under the 66.6 % band too: a
  piece that would take the load there is left off the list (`gearFits`).

  The gear errand never leaves the bot under the grade penalty (#436): while the server holds it
  (skill 4267), the bot takes off a worn armor or jewelry piece above its Expertise grade, then wears
  the best piece of its grade it carries. An over-grade weapon costs only accuracy, so it comes off
  only once a weapon of the bot's grade it carries replaces it. Status publishes the load, its band,
  the run speed and the grade and death penalties for the panel.

  It reads SoE only with no attacker. The recall is not a magic cast, so a hit does not break it
  (server `cast/controller.go` `InterruptOnDamage`); only death, or a stun, sleep or paralysis a
  fight lands, does. A monster that would attack on sight but has not does not block the read: at a
  starter spot nearly every monster would (#251). If no such moment comes within `Supply.SafeWait`
  (default 2 min), as at a spot where something always attacks, it walks out instead. The triggers are read at the bot's spot every 5 s, and
  not within 10 min of the last trip's end, so a bot back no better off farms a while first.
- **SoE.** It stands up if sitting, then sends one UseItem of a carried SoE. While the cast runs it
  is in an **escaping hold**: every rule that moves, attacks, loots or clicks returns a hold until
  the teleport (`TeleportToLocation`) or the cast's end. This matters because a move sent mid-cast
  is queued by the server and would run on arrival. If the cast ends with no teleport, it walks.
  Pacing: one UseItem, with no repeat before the cast ends or `escapeRetry` (30 s, `death.go`).
  The hold (rule "escape") sits right after "critical": a potion still goes down, while a heal
  cast or a run away, also queued mid-cast, waits. The hold lasts until the teleport, or
  `escapeRetry` past the cast's end (the walker's `/unstuck` wait).
- **Without SoE,** it walks to the nearest town (the travel map's town gatekeepers). When that
  town is out of walking reach (another SCC, or past `MaxTravel` in straight line) or the walker
  answers it unreachable (its gatekeepers, where the walk would end the same way, then shunned
  for the errand's shun time), it takes a gatekeeper through the travel errand (#234): the trip search
  of the spot rule (Dijkstra, walks priced by time, teleports by adena), its goal a landing within
  the town radius (3000) of the nearest town, else of any town, within `Travel.MaxPrice` and the
  adena carried. Pacing is the errand's: the bypass gate and `Travel.Gap` since the last trip. A
  landing on the way plans the next teleport at once; the last ends the trip, as the SoE landing
  does. With no such trip a town out of reach is walked to all the same, and one the walker
  cannot reach ends the trip.
- **Spot.** The trip releases its hold in the spot registry, so another bot may take the spot; the
  returning bot picks again, as at a level-up.
- **In town.** Arrival resets `shop.nextAt`, because a miss from the spot would otherwise push the
  first visit up to 5 min out. Then the errands run in the existing order: sell, shop (the budget),
  gear, trainer, Newbie Guide. The persona's "trip in town" hook is called once per arrival (a
  scroll landing, a gatekeeper trip's last landing, or a walk that reached the gatekeeper; a walk
  that found no town, a gatekeeper trip refused, or a teleport of another cause during the trip
  such as a GM's, is none).
- **Preparation.** No farmer leaves town for a spot until its supplies meet the trip target, the
  budget has spent all it may, or no spawned merchant within reach sells what is missing. A broke
  bot still goes and farms. The same gate applies after a death and respawn in town and at a fresh
  login in town.
  - The gate is armed by those events (arrival, `Revive`, login), not by a town radius: the
    merchant-in-reach test is what makes it a town gate. While armed, the shop errand buys the
    bot's own supplies up to the trip target, not only under `Supply.Refill` percent; the extra
    supplies count toward neither.
  - It opens for good, until armed again, once the targets are met, nothing short is sold in
    reach (`Shop.Reach`) at a price the supplies tier pays, or the shop errand ends a round (a
    visit over, so the budget spent what it may, or a search that found no merchant).
  - It holds only the spot claim; every errand ahead of rule "spot" runs as before.
- Dependency: walks go through M7 navigation (#151). This lands after N6 (#158).

### 8. Buying from player stores

Items no spawned NPC sells (Greater Healing Potion, graded shots, gear) are bought from private sell
stores.

- **Who:** a player whose `CharInfo` store type is sell (1). Package stores (8) are skipped.
- **Never** a karma or flagged owner, nor one whose last `RelationChanged` says the bot may attack it
  without force (a PvP zone, an arena, a duel): the second `Action` on one attacks it.
- **What:** the use meter's trip targets carried under `Supply.Refill` percent, up to the target,
  wherever they are sold, then the gear goal's pieces (`gearGoal`). Rows are taken cheapest first,
  each tier above its budget floor (section 5); of the gear, one piece per slot, and a set piece only
  with the rest of the set still paid.
- **Flow:** walk to within trade interaction distance (150) of the owner, then send `Action` twice
  on it: the first selects, the second opens `PrivateStoreListSell`. There is no list-open packet
  of its own. Then send `RequestPrivateStoreBuy` for wanted rows at no more than
  `Market.MaxOverReference` percent of the reference price (default 150, 1.5 ×; 0 buys nothing from
  stores), and never above a spawned NPC's base price for the same item, within the budget. The
  bot walks to within 100, inside the server's interact approach offset, so the server walks it
  nowhere. Each row carries the listed price exactly; any other price, or a
  row sold out meanwhile, refuses the whole request in silence. Only adena, weight and slots
  refusals answer, with a `SystemMessage`. The purchase shows as one `SystemMessage` per row and
  then the batched `InventoryUpdate`; the list is not sent again. Packets, layouts and refusals:
  "Buying from a private sell store" above.
- **Pacing:** walks and clicks through the frame pacer, one buy request per list window, and a
  per-seller cool-down (`Market.Cooldown`, default 10 min) after every visit, a refused or
  unanswered one included. The errand is rule 9 errand "market", ahead of "shop".
- This also gives the bot merchants buyers, so trading runs both ways.

### 9. Class change and dyes (D15)

- **Class change:** at level 20 and 40, the bot visits the class master and picks the build's
  profession for its race and persona, through the page's bypass links: `1stClass` at 20 and
  `2ndClass` at 40, then the `change_class N` link of the menu page for the profession. The bot
  stands within 150 of the master and clicks only links of the last page it got, since the server
  drops any other. Pacing: `bypassGap` (500 ms). The page's answer, a new page or the class change
  in `UserInfo`, is waited for 5 s; then the visit backs off for `ClassChange.RetryAfter` (10
  min). A menu without the build's profession also backs off: the bot never takes another class.
  The build table includes the dwarf crafter line the clan crafter plan (M9) needs: 53 → 56
  Artisan (20) → 57 Warsmith (40); the crafter learns Create Item (172) as 53, 56 and 57, each at
  the dwarven trainers whose `teachTo` lists that class (23 for 56, 22 for 57).
- **After the change.**
  - The change sends `HennaInfo` and `UserInfo` (new class id), then the changed page. It sends no
    `SkillList`, because `AutoLearnSkills` is off by default. The page's `learn_skills` link would
    grant every skill free of SP. The bot does not use it, since a player pays SP at the trainer
    (D6 keeps it a persona choice).
  - `LearnSkills` and `Trainers` are keyed by class id, and today only base classes have entries
    (`trainer.go`). So the change adds the professions' learn and trainer tables, or the support
    mystic stops learning heals and buffs at 20.
  - The registry learns the new class through the existing status path. The brain's snapshot
    already carries `ClassID` (`status.go`). The supervisor's `keepProgress`, which today copies
    only the level and position when a run ends (`internal/botregistry/supervisor.go`), gains
    `b.Class = &st.ClassID`, so the new class is saved at the run's end, not live. This is safe:
    the starting-class check in `character()` is skipped once the bot has a name. The one-shot
    `pin` report (`session.go`), which fires once per run, is not used for this.
- **Dyes:**
  1. **Buy.** The dyes go on the shop errand's list from merchant buylists, after the gear in the
     budget's gear tier: 10 per symbol, in full or not at all, with the draw fees (and the fees of
     the removals ahead) left above the tier's floor. A symbol is planned only while the adena
     pays both its dyes and its fee. Bought dyes join the keep set until they are drawn. They
     count toward neither the preparation gate nor the run-out trigger.
  2. **Draw.** The bot walks to a Symbol Maker and talks to it, as a player does, although the
     server does not require one. It follows the page's `Draw` link (`npc_<id>_Draw`, a bypass
     answered by `HennaEquipList`, `npc/folk_bypass.go`), as the client does: the server opens the
     draw window only from the dialog, and `RequestHennaItemList` only asks for it again, so the
     bot never sends it. Then it sends `RequestHennaEquip` per symbol into its free slots. The list
     shows only symbols whose dye the bot carries, so the dyes are bought first. One request per
     answer, waited for 5 s (`dyeReplyTimeout`): a draw's answer is `HennaInfo` and then the
     `InventoryUpdate` behind it (the server batches inventory changes every 333 ms,
     `task/inventoryupdates.go`), so the next draw sees the dyes and adena the last one spent; a
     refusal is CantDrawSymbol, SymbolsFull, YouNotEnoughAdena or NotEnoughItems. The server drops
     an unknown symbol in silence (`network/henna.go:62-66`). A missing dye and a class refusal
     answer the same CantDrawSymbol, and the adena goes before the dyes, so the bot checks the
     class list (`hennas.xml`, read into the travel map), its 10 dyes and the price past the
     reserve before it sends. A refusal, a timeout, a page without the link, a window without the
     symbol, or a maker not found ends the visit and backs off for `Dyes.RetryAfter` (default
     `visitShunFor`, 10 min). The `Draw` link is paced by `bypassGap`; the henna requests have no
     server gate and go one per answer.
  3. **At the second profession.** First-profession dyes (`hennas.xml` symbols 1-36) list only
     first-profession ids, so after the change at 40 they stay in their slots but add nothing
     (`model/henna/list.go` `recalculateLocked`; `HennaInfo` reports their active id as 0). The
     bot removes them (`RequestHennaUnequip`, fee: a fifth of the price, 5 dyes back), then draws
     the second-profession rows (37+) into all 3 slots. It uses the same one-request-per-answer
     pacing and 5 s timeout. A worn active symbol off the table is left in place.
  4. **Which dyes.** A persona table per profession that respects each row's trade (+1 of one stat
     for -1 to -3 of another, by price) and its class list. For example: warriors STR+1 CON-3 and
     DEX+1 CON-3; nukers INT+1 WIT-3 or WIT+1 INT-3; healers (Cleric 15, Oracle 29, Shillien
     Oracle 42, Shaman 50), whom the INT/WIT rows exclude, MEN+1 WIT-3 (`hennas.xml:12`).
     Shipped default (`Dyes.Symbols`, by class id, drawn in order): first-profession fighters 1
     and 6, nukers 8 twice, healers 10 twice (37000 each); second-profession fighters 37, 42, 37,
     nukers 44 three times, healers 46 three times (87000 each). Each row is checked against the
     shipped `hennas.xml` class lists and prices (`TestDefaultDyeTableShipped`).
     Through the model (M10 S5, #324), with no persona table the bot draws, slot by slot, the
     symbol with the highest value gain among the symbols its class may draw that raise its row's
     stat (`ClassKit.DyeStats`, `docs/strategy.md` column 11; a second profession keeps its first
     profession's), whose fee and missing dyes the adena past the gear floor pays (`dyeDraws`). The
     table above is what a bot draws without the model.

### 10. Enchant to the safe level (#181)

Shipped as rule "enchant" (`enchant.go`), after the errands that wear, buy and draw and ahead of
every rule that walks the bot out of town.

- **Scrolls.** The bot's table of the scrolls the server takes is copied from
  `enchant/service.go:56-87` (D5): plain, crystal and blessed, weapon and armor, grades D to S.
  The item files set no crystal grade on scrolls; `TestEnchantScrollsShipped` checks the table
  against their names. A scroll of a grade and kind the bot wears (its weapon for a weapon
  scroll, any worn armor piece, shield or jewelry for an armor scroll) is kept, not junk
  (`keepsScroll`).
- **When.** In town (within `storeTownRadius` of a travel map town, the gatekeepers' spawns), out
  of combat, standing, with no attacker, no private store, no trade open and no gear UseItem
  awaiting its answer.
- **What.** The weapon first, then the armor piece whose next level adds the most score (section
  6), the body piece first on a tie; plain and crystal scrolls before blessed ones. A piece is
  enchanted only below +3 (+4 for full-body armor), where the server's chance is 1
  (`enchant/service.go:556-559`, its default `SafeMax` 3 and `SafeMaxFull` 4), so the last
  use ends at +3 (+4). `Enchant.Risk` (default off, D6) lifts that cap: past it a failure breaks
  the piece (a blessed scroll's sets it to +0).
- **Flow and pacing.** UseItem of the scroll, then `ChooseInventoryItem` (0x6f, the scroll's
  template id); `RequestEnchantItem` (0x58, the target's object id), then `EnchantResult`
  (0x81); after a success, the InventoryUpdate showing the new level (the server's 333 ms
  ticker may send it before or after the result). One packet per answer, each waited for 5 s;
  a silence or a result other than a success ends the exchange for `Enchant.RetryAfter` (5
  min). Neither packet has a server flood protector; the frame pacer bounds the frames. The
  server refuses a logout while a scroll is selected, so the request always follows the
  window unless the target changed.

## Owner decisions

Answered by the owner on 2026-11-21.

| # | Question | Decision |
| --- | --- | --- |
| D14 | Where do D-grade and higher shots come from? | (b) A Dwarven crafter bot makes them for its clan through the clan warehouse: the clan crafter plan (`docs/crafting.md` and the D17-D19 rows in PR #183; milestone M9, epic #184). Until it lands, I2's shot factor keeps the weapon choice honest without them. The rejected options were (a) adding graded shots to grocer lists, and (c) no source at all. |
| D15 | May bots change class? | (a) At the class master NPC 50000, which is free and quest-free under the shipped config, given a persistent spawn by fatal10110/acis_golang#5419. |
| D16 | Supply trips | Yes: farmers leave their spot to restock, and the spot is released and may be taken. |

## Testing

- **Datapack (unit):** fixture item files with stats, `<enchant>` stat names, shots, potions and enchant
  scrolls; potion skills with stack type and order; an armor set file. The reader keeps exactly the
  fields it reads and fails on mangled values, like the existing `items_test.go`.
- **Brain (unit, table-driven, events and injected clock in, packets out):**
  - the copied enchant formula matches the server's on fixed cases (weapon types, grades, over +3);
  - a worn D weapon turns auto on only for the D template, and off for a no-grade one that is on;
  - a few looted D shots below the trip target do not switch weapons, and the replaced weapon is
    kept while only carried shots back the new grade;
  - bought SoE, buff potions and planned dyes are never junk;
  - at the second profession, inactive first-profession dyes are removed and second-profession dyes
    drawn;
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

## Delivery (milestone M8, "Items and loadout", epic #166)

P0 (the `docs/plan.md` rows and the server issue for D15) is done in PR #165:
fatal10110/acis_golang#5419. D14 (b) needs no server change.

| PR | Issue | Scope | Depends on |
| --- | --- | --- | --- |
| I0 | #167 | Settle the remaining server facts: private store list open, class master pages, the dye flow. Notes go into this doc ("Flows settled in I0"). | — |
| I1 | #168 | Datapack: etc items (`etcitem_type`, `is_sellable`), gear stats, the copied enchant formula, skill stack types by id and level. Replace the hard-coded potion and shot lists. | — |
| I2 | #169 | Shots by weapon grade, beginner shots first, blessed spiritshots for casters, no unusable shots bought, the shot source and the shot factor on the price score (findings 1, 2). | I1 |
| I3 | #170 | Use meter and trip targets (section 4). | I1 |
| I4 | #171 | One budget replacing `shopWants`, `gearWants` and `gearReserve`; `keeps` covers everything the loadout buys; the keep hook (section 5). | I3 |
| I5 | #172 | The build table, and gear score by stats and enchant, replacing price and the never-replace-enchanted rule (sections 2, 6). | I1, I2 |
| I6 | #173 | Armor set reader and set-combination scoring and buying. | I5, I4 |
| I7 | #174 | Grade-jump saving. | I5, I4 |
| I8 | #175 | Supply trip: triggers, SoE with the escaping hold, the walk-out fallback, the spot release (section 7). | I4, M7 N6 (#158) |
| I8b | #182 | Preparation in town before leaving, the arrival reset of `shop.nextAt`, the "trip in town" hook (section 7). | I8, I4 |
| I9 | #176 | Better HP potion and buff potions with the stack-type check and gate. | I1, I4, I5 (build table) |
| I10 | #177 | Buying from private sell stores (section 8). | I0, I4 |
| I11 | #178 | Class change at the class master: professions in I5's build table, profession learn and trainer tables, the class through the status snapshot. | acis_golang#5419, I0, I5 |
| I12 | #179 | Dye purchase and drawing. | I11, I4 |
| I13 | #180 | e2e live scenarios for I2-I12. | each |
| — | #181 | Backlog: enchant to the safe level (section 10). | I5 |

Most urgent: I2 (every bot at level 20 today), then I3, I4, I8 and I8b (farmers never restock).

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
- Dwarven crafting of graded shots (D14 b) is the clan crafter plan (M9, epic #184), not an M8 PR.

## Interfaces the clan crafter plan (M9, epic #184) consumes

M9 builds on M8 rather than duplicating it. M8 exposes these and adds no clan logic itself:

- the **shot source** interface of section 3, to which M9 adds the crafter;
- the **per-item trip target** of I3, which sizes a clan member's orders;
- the budget's **reserve tier** (I4), which carries the warehouse fees;
- a **"trip in town" hook** in I8b (#182), when a member deposits materials and collects products.
  It also reports the weight it will hand off, which counts toward `ReturnLoad` (section 7);
- a persona **keep hook** (I4), called per carried item by the NPC sale, the store listing and
  trade pricing, answering keep and may-wear.
  M9 fills it so that members keep crafting materials, recipe scrolls and unworn graded gear for
  crystallizing instead of selling them. I1 exposes the material etc items for this;
- a persona **extra supply hook** (template → count) in I3/I4, which M9 fills with the ore its shot
  orders need. It is bought in the supplies tier after the bot's own supplies, and counts toward
  neither the preparation gate nor the run-out trigger;
