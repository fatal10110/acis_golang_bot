# Clan crafter: a dwarf that crafts from the clan warehouse

Status: owner-approved direction (2026-11-21): D17 as recommended, D18 through the items plan's
class change, D19 "take everything needed for crafting, any time", and chain planning. Revised
after one independent review round, which found: shot materials cannot be looted (crystals come
from crystallizing, ores from shops); 448 recipes consume a copy of their own scroll; craft and
registration replies are ambiguous; several warehouse facts were missing; shared stacks race; the
order board had no lifecycle; chain valuation ignored unpriced items, yields and recipe variants;
crafting orders went beyond D3 (since settled by D20).

Builds on the items plan (`docs/items.md`, milestone M8, epic #166), whose D14 (b) is this plan.
Its section "Interfaces the clan crafter plan (M9) consumes" lists what M9 uses: I1 #168 (material
etc items), I2 #169 (shot source), I3 #170 (trip targets), I4 #171 (reserve tier and keep hook),
I8b #182 (trip-in-town hook), I11 #178 (class change, the dwarf line 53 → 56 → 57, and the
profession learn and trainer tables).
The items plan's third review round split I8: #175 keeps the trip trigger, #182 the town hook.

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Datapack paths (`recipes.xml`,
`classes/...`, `npcs/...`, `buyLists.xml`) refer to `aCis_datapack/data/xml`. Bot paths name this
repository.

## Goal

The owner invites a dwarf bot into their clan. Clan members (the owner and other bots) deposit
items in the clan warehouse. The dwarf checks the warehouse, withdraws whatever its recipes need,
plans crafting chains from base materials up to weapons, armor or shots, crafts those worth it, and
deposits the products back. Member bots deposit the materials they loot, and the ore their shots
need, and get the products they use back. The dwarf keeps improving its craft: levels, Create
Item, professions, crystallizing and recipes.

## Server behavior this rests on

Every handler below is ported in the Go server. No server gap was found.

### Clan

| Fact | Evidence |
| --- | --- |
| Invite: `RequestJoinPledge` 0x24 from a member with the invite right. The target gets `SystemMessage` S1HasInvitedYouToJoinTheClanS2 (the inviter's name), then `AskJoinPledge` 0x32 {requester object id, clan name}, and answers `RequestAnswerJoinPledge` 0x25 {answer}. On join: `JoinPledge` 0x33 {clan id}, `UserInfo`, member lists. | `network/clan.go:117-247`, `serverpackets/pledge.go:110-115` |
| New members join at grade 6. Privileges per grade are set by the leader (`RequestPledgePower` 0xc0); grades by ex 0xd0 sub 0x1c. Bots send neither: the owner sets privilege 8 on grade 6 once. | `clan/clan.go:30`, `network/clan.go:373-385` |
| A level-1 clan holds 15 members; higher levels hold more. | `clan/clan.go:294-307` |
| Clan chat is Say2 type 4, broadcast to the clan as `CreatureSay` 0x4a. | `handler/chat/chat.go:14-18`, `network/chat.go:189-194` |

### Clan warehouse

| Fact | Evidence |
| --- | --- |
| Opened at a `WarehouseKeeper` (50 templates), `CastleWarehouseKeeper` or clan hall manager that the player last talked to and is in reach of, by the exact bypasses `npc_<id>_DepositC` / `npc_<id>_WithdrawC`. Both need clan level 1; withdraw also needs privilege 8, so without it a member cannot see the contents at all. An empty warehouse answers `NoItemDepositedInWH`, not a list. | `model/actor/npc/folk_warehouse.go:7-55`, `network/clan_warehouse.go:132-160`, `clan/warehouse.go:24-33` |
| `WareHouseDepositList` 0x41 / `WareHouseWithdrawalList` 0x42: u16 type (2 = clan), i32 adena, u16 count, then per item u16 type1, i32 object id, i32 item id, i32 count, u16 type2, u16, i32 body slot, u16 enchant, u16, u16, i32 object id, i64 augmentation. | `network/serverpackets/warehouse.go` |
| `SendWareHouseDepositList` 0x31 / `SendWareHouseWithDrawList` 0x32: i32 count (1 to `MaxItemInPacket`, the larger race inventory size: 80 on the shipped config), then {i32 object id, i32 count}; the payload length must be exact. | `network/clientpackets/warehouse.go:110-132`, `character_inventory_limit.go:27-30` |
| Withdraw: with `MembersCanWithdrawFromClanWH=False` (the default) only the leader withdraws; with `True` a member needs privilege 8. | `clan/warehouse.go:50-63`, `cmd/gameserver/clan.go:42` |
| Deposit: 30 adena per requested row, even for rows that are then skipped. Capacity `MaximumWarehouseSlotsForClan` (150 Go default, 200 in the aCis properties); a stack that merges into a held row takes no slot. Only tradable, unequipped, non-quest items. | `network/warehouse.go:21,334`, `inventory/warehouse.go:56-162` |
| Answered refusals: AlreadyTrading, OverCapacity, NotEnoughAdena, SlotsFull, WeightLimitExceeded. Silent ones: a keeper not last talked to or out of reach, karma, and a withdraw row with a stale object id or a count above what is held, which drops the whole request. | `network/warehouse.go:287-312,425-439`, `inventory/warehouse.go:237-250` |
| Nobody is told when the clan warehouse changes: a bot must reopen it to see. | `network/warehouse.go:318-366` |

### Craft

| Fact | Evidence |
| --- | --- |
| Recipe book: `RequestRecipeBookOpen` 0xac {0 dwarven, 1 common} → `RecipeBookItemList` 0xd6 {type, max MP, count, {recipe id, index}}. A recipe scroll is learned by `UseItem` (handler "Recipes"); success answers S1Added and a fresh 0xd6; a refusal answers only a system message (RecipeAlreadyRegistered, CantRegisterNoAbilityToCraft, CreateLvlTooLowToRegister, UpToS1RecipesCanRegister). The book holds 50 dwarven and 50 common recipes, plus stat bonuses. | `network/recipe.go:141-150`, `craft/craft.go:178-215` |
| `RequestRecipeItemMakeSelf` 0xaf {recipe id}: instant, no standing or weight check. Refused in combat or duel with only CantOperateStoreDuringCombat and no 0xd7. Otherwise it answers `RecipeItemMakeInfo` 0xd7 {recipe id, type, cur MP, max MP, status}, where status 0 means a failed roll (ItemMixingFailed) or a refusal (MissingS2S1ToCreate per material, NotEnoughMP), told apart only by the system messages. On success: S2S1Disappeared per material, EarnedS2S1S, status 1; `InventoryUpdate` comes later. Requests inside `ManufactureTime` (300 ms) are dropped silently. | `craft/craft.go:96-176`, `network/recipe.go:125-139,180`, `network/client_ingame.go:227` |
| **Every listed material is consumed, and 448 of the 871 recipes list a copy of their own scroll** (for example recipe 64, scroll 2171, material `2171-1;...`). Java does the same; the server is the reference. | `recipes.xml:66`, `craft/craft.go:166-176` |
| Recipes: `id`, `itemId` (the scroll), `level`, `mpConsume`, `successRate`, `material` (`id-count;...`), `product` (`id-count`; 42 recipes yield more than one, up to 500), `isDwarven`. 178 products have more than one recipe (60 % and 100 % variants). No cycles; chains are at most 5 deep; all 46 intermediates are 100 % recipes. | `recipes.xml` |
| Shots: Soulshot D (recipe 20, level 2, 156 per craft) needs Soul Ore 1785 × 3 and Crystal D 1458; Spiritshot D needs Spirit Ore 3031 and Crystal D. No NPC drops or spoils crystals or ores. Ores are sold on spawned grocer lists (35 lists). Crystals come only from crystallizing graded gear. Soulshot D is itself a material of the compressed shot packs. | `recipes.xml:22,482,500`, `npcs/*.xml`, `buyLists.xml` |
| Crystallize: `RequestCrystallizeItem` 0x72 {object id, count}; skill 248, level 1 at character level 20 (Artisan), level 2 at 40 (Warsmith); grade above the skill answers CrystallizeLevelTooLow; success answers ItemCrystallized and the crystals earned. | `network/inventory.go:839-880`, `clientpackets/item_ops.go:77`, `classes/dwarfFighter.xml:188,554` |
| Create Item (skill 172): level 1 at 5 (Dwarven Fighter); 2-4 at 20/28/36 (Artisan, 56); 5-9 at 43/49/55/62/70 (Warsmith, 57). Common Craft (1320) is free and automatic. | `classes/dwarfFighter.xml:39,115-117,381-385` |
| Class change is at the class master NPC 50000 (items plan D15, I11 #178; spawn requested in fatal10110/acis_golang#5419). | `cmd/gameserver/classmaster.go`, `npcs/50000-50999.xml:3` |

## Owner decisions

| # | Question | Answer |
| --- | --- | --- |
| D17 | How do bots withdraw from the clan warehouse? | The owner's server sets `MembersCanWithdrawFromClanWH=True`, and the owner gives grade 6 (where new members join) privilege 8. A server setting, not a bot special case. |
| D18 | How does the dwarf get past Create Item 1? | The items plan's class change (D15, I11 #178): Artisan at 20, Warsmith at 40. Until then it stays at Create Item 1: wooden arrows, steel, cokes, leather, braided hemp, no-grade gear and the first potions; no shots (Soulshot D is level 2) and no crystallizing. |
| D19 | What may the crafter take from the clan warehouse? | Anything a chain it plans needs, any time, including recipe scroll copies that recipes consume. Never adena, never items no recipe it can reach uses, never items without a reference price unless an order names them. |
| — | Chains | The crafter plans chains from base materials up to an end product and crafts a chain only when it is worth it (owner, 2026-11-21). |
| D20 | Who may add the crafter to a clan and order crafts? | A non-GM player too, beside GMs (owner, 2026-11-22, answering Q3). A clanless crafter accepts a clan invitation from any player, or only from the names in its preset's `ClanInviters` when set. Its clan's leader orders crafts like a GM. This extends D3 for the crafter only; every other command stays GM-only. |

## Design

### 1. Clan membership (every persona)

- `Persona.JoinClan(w, requester, clan, now) bool`, like `JoinParty`. The requester's name comes
  from the S1HasInvitedYouToJoinTheClanS2 message just before `AskJoinPledge`, or from the known
  object of its id. The default accepts a GM's invitation; the crafter preset also accepts any
  player's while it has no clan, or only `ClanInviters`' when that list is set (D20).
- `PledgeShowMemberListAll` 0x53 (sent on entering the world and on joining) gives the clan's
  leader name: i32 sub-unit flag, i32 clan id, i32 pledge type, str clan name, str leader name, then
  crest, level, castle, hall, rank, reputation, dissolving, 0, ally id, str ally name, ally crest,
  at war, i32 member count and the member rows (`serverpackets/pledge*.go`,
  `FramePledgeShowMemberListAll`). The world model keeps the leader name; it is read again at every
  entry, so a leader change is seen at the next login.
- The world model keeps the clan id from `UserInfo` and `JoinPledge`, and clears it on leaving.
- Clan chat is decoded into the existing chat event with its channel.

### 2. Warehouse errand (shared)

- The nearest `WarehouseKeeper` spawn, from the datapack NPC type through the travel map, walked to
  like the trainer; the bot stays in reach of it for the whole transaction.
- One transaction per open: talk, bypass, wait for the list (or `NoItemDepositedInWH`, which means
  empty, not a failure), send one deposit or withdraw request, wait for the `InventoryUpdate` or an
  answered refusal, done. Bypasses keep `bypassGap`.
- A withdraw is sent right after its fresh list, under the clan's warehouse lease (section 3), and
  only for counts that list shows.
- Before a deposit: adena ≥ 30 × rows, free slots counted only for item kinds the warehouse does not
  hold, no open trade, rows ≤ 80.
- Warehouse opens are at least `MinOpenGap` (60 s) apart per bot, except the deposit that follows
  its own withdraw cycle. A silent drop is a timeout; repeated timeouts back the errand off.

### 3. The order board (process-wide)

- `CraftBoard`, one per process, keyed by clan id; one mutex owns its state. Bots in another
  process (a `bots swarm` beside `bots ui`) do not share it.
- **Warehouse lease.** One bot of a clan at a time holds the right to withdraw: a lease with an
  expiry on the injected clock. A withdraw needs a held lease and a list fetched under it, so two
  bots never withdraw from stale lists of the same stacks.
- **Crafter entry.** The crafter publishes its recipe ids, Create Item level and the grades of
  shots it can make now (recipe in the book and materials or stock in the warehouse).
- **Orders.** A member posts: product, count, member name. States: posted → materials deposited →
  claimed → crafted → delivered. A claim is a lease that expires.
- **Ledger** as counts per item id, not rows: how many of each product in the warehouse belong to
  each member, and how many intermediates are reserved for a chain. A member withdraws only its own
  counts. Everything else is the clan's.
- **Lifecycle.** Every lease and claim expires on the injected clock and is released when its bot
  stops (the bot's stop path calls the board, like the other registries). Brains read the board on
  their own tick: no goroutine waits on it, and the board hands out copies.
- In memory only. After a restart open orders are lost; their items count as the clan's.

### 4. Craft planner (pure)

Input: a fresh warehouse list, the crafter's inventory, recipes, the recipe book, Create Item
level, MP, the board's orders and reservations. Output: what to withdraw, an ordered craft list and
what to keep.

It is `craftplan.Build(Input, Config) Plan` (`internal/craftplan`). `Config` holds `MinGain`,
`MaxChains` and `MaxWork`, a bound on the nodes one plan visits; the defaults are any positive
margin, 20 chains for the clan and 200000 nodes. A plan takes from the crafter's inventory before
the warehouse. It crafts only recipes already in the book: a scroll it learns counts from the next
plan, once the server has answered. A chain for the clan is one craft of its top step, so
`MaxChains` bounds the top crafts in a cycle; an order's chain crafts its whole count, or as much
of it as the pool covers.

1. **Orders**: the board's member orders, then the clan leader's and GMs' orders (section 8).
2. **Recipe scrolls** in the warehouse the crafter can register now (level and room): withdraw one
   copy and use it. Further copies stay, as the materials of recipes that consume their own scroll.
3. **Chains** from everything else (D19), below.

#### Chain planning

- **Recipe graph.** Over recipes, not products: a product with several recipes is a node with a
  choice (the 100 % variant first, the 60 % one only when its margin still wins). Only recipes the
  crafter can craft now (in its book, level ≤ Create Item); the reader rejects a cycle.
- **A chain** is one end product and every step under it, using what the warehouse and the
  crafter's inventory hold at each level (base materials, intermediates, scroll copies) and
  crafting the rest from below. A step yielding more than one unit runs ceil(need / yield) times
  and credits the leftovers to the pool.
- **Value.** The datapack reference price, with a demand weight for products the clan uses (shots
  and arrows of the members' grades, the board's orders). An item with no reference price is never
  consumed unless an order names it. Products the board has delivered or reserved for a member are
  not in the pool, so the compressed shot packs never eat a member's shots.
- **Worth it.** Value of the end product minus the value of everything the chain uses up. Every
  intermediate in the shipped data is a 100 % recipe, so only the top step can fail; its expected
  cost is its inputs over its success rate (a failed roll is crafted again). A chain is worth it
  when the margin is over `MinGain`; ties go to fewer steps.
- **Allocation.** Orders first. Then greedily the chain with the best margin per value used, its
  inputs taken from the pool, repeated until no chain is worth it or `MaxChains`.
- **Execution across cycles.** Bottom-up, resting for MP between steps. Intermediates made for a
  chain are reserved on the board. A chain the warehouse no longer covers is re-planned; what it
  made goes back into the pool. Leftovers the next plan needs stay in the crafter's inventory
  instead of being deposited and withdrawn again.

### 5. The `crafter` preset

Created as a Dwarven Fighter. Its goal rule:

1. **Check**: reopen the warehouse every `CheckEvery` (a few minutes, jittered, never under
   `MinOpenGap`), sooner when its tick sees a board order with materials in. A tripped breaker
   holds every check for `BreakerHold`, that order's too; the hold and the last cycle's end are
   kept on the bot, so a reconnect keeps both (#233).
2. **Plan** (section 4); take the lease; reopen; **withdraw**; release the lease.
3. **Craft**: one `RequestRecipeItemMakeSelf`, then wait for 0xd7 with its system messages, or the
   combat refusal, or a timeout, and at least 500 ms plus jitter before the next. MissingS2S1 or
   NotEnoughMP re-plans (rest for MP, or refresh the inventory); ItemMixingFailed counts a failed
   roll. A breaker stops the cycle after `FailStreak` results that are neither success nor a
   failed roll, or after a timeout streak. Never in combat; a fight interrupts the cycle.
4. **Deposit** products, and leftovers no plan needs, before weight passes 50 % and at the end of
   the cycle; update the board.
5. **Improve**:
   - Create Item at the dwarven trainer: 172 for classes 53/56/57 in the learn and trainer tables
     (for 56/57 in #178's profession tables), trainer ids from the datapack's `teachTo`;
   - class change through I11 #178;
   - recipe scrolls it loots or finds are registered (one copy) and the copies kept as materials;
   - with nothing to craft, it farms for XP on the spot registry and returns every `ReturnEvery`.

### 6. Shot materials

Graded shots are D14 (b), and their inputs cannot be looted.

- **Crystals.** From Artisan on, the crafter crystallizes graded gear of the clan pool whose
  crystals are worth more than the item (reference prices), within skill 248's grade, one
  `RequestCrystallizeItem` per answer. It never crystallizes a member's ledger items. Members
  deposit graded gear they do not wear instead of selling it, through the keep hook.
- **Ore.** A member buys the Soul Ore or Spirit Ore its shot orders need at a grocer: M9 fills I3's
  extra supply hook (#170, template → count), and I4 (#171) buys it in the supplies tier and keeps
  it out of junk, after the bot's own shots, potions and SoE. Ore counts toward neither the town
  preparation gate (#182) nor the run-out trigger (#175): an unfilled ore order never holds a member
  in town or sends it home; the member orders as many shots as the ore it carries covers. The crafter
  never buys with clan adena (D19); the owner may deposit ore too.

### 7. Clan member option (farmer, support, merchant presets)

Active when the bot is in a clan with a crafter on the board.

- The keep hook (I4 #171) takes the carried item (object and template) and answers keep and
  may-wear; the NPC sale, the store listing and trade pricing all consult it. M9 answers keep for
  every recipe scroll and for the materials of recipes in the crafter's book or whose scrolls are in
  the warehouse (I1 #168 marks material etc items). For graded gear the bot does not wear, kept to
  deposit for crystallizing, it answers keep and not may-wear, so the gear errand never puts it on.
  `shop.go` gains no clan logic.
- On its supply trip (I8b #182, its trip-in-town hook) it visits a warehouse keeper: deposits what
  it keeps, posts orders sized by I3's per-item trip target, deposits the ore those orders need (bought
  through I3's extra supply hook, #170),
  and withdraws its delivered counts. Fees come from I4's reserve tier. The hook also reports the
  weight it will hand off (what it deposits), so #175's load trigger counts kept materials and
  sends a heavy member home.
- The board implements I2's shot source (#169): a grade counts only when the crafter can make it
  now.

### 8. Leader and GM orders

- `craft <item name> [count]` by whisper or clan chat from the crafter's clan leader (D20) or a GM
  (D3), handled by the existing command path; the
  crafter answers once with the plan or the reason it cannot (unknown recipe, Create Item too low,
  materials missing), through the chat gates and caps. A finished order is announced once in clan
  chat.
- The leader is the clan roster's leader name (0x53); it reaches `craft` only. Anyone else's order
  goes to the authority (D3). The answer goes back on the order's channel, at most 100 characters
  (the server's `MaxTextLength`), counts in the persona's chat `Total` and passes the chat gate;
  none to a name with the bot marker. A clan line is answered by the first crafter of the clan to
  hear it (the board's `Heard`).
- The order goes on the board with its materials "in" (the clan's pool pays), behind the members'
  orders in the plan, at most 10 open per clan. Materials are judged from the last warehouse list
  and what the crafter carries; it opens no warehouse to answer. A cycle whose plan from a fresh
  list cannot start it cancels it. Only a GM's finished order is announced.

### 9. Panel

- A `crafter` preset (needs `-datapack`), class forced to Dwarven Fighter.
- Status: Create Item level, recipes known, open orders and chains, last cycle, breaker state.

## Pacing

| Sender | Bound |
| --- | --- |
| Craft requests | One per answer, at least 500 ms plus jitter apart (server gate 300 ms, silent); MP bounds it far lower; a breaker on failure streaks |
| Warehouse | At least `MinOpenGap` (60 s) between opens per bot, one request per open, bypasses `bypassGap` apart |
| Recipe learning, crystallizing | One `UseItem` or `RequestCrystallizeItem` per answer, with a timeout |
| Chat | One answer per leader or GM order and one clan line per finished order, through the chat gates |

All of it stays far below 80 frames/s average.

## Testing

- Unit: codec round trips and recorded fixtures (each decoder consumes exactly `len(payload)`) for
  0x32, 0x33, 0x41, 0x42, 0xd6, 0xd7, the system messages above and every client packet; the
  recipes reader on the shipped file (871 recipes, no cycle, yields, variants, scroll copies).
- Planner tables: a multi-step chain to a weapon crafted when worth it and skipped when not; yields
  above one and their leftovers; recipe variants; scroll copies counted; unpriced items untouched;
  members' shots excluded from the pack recipes; competing chains allocated by margin; a chain
  re-planned after the warehouse changed. Its cost is bounded by a work counter (nodes visited) on
  the full `recipes.xml`, and a benchmark; no wall-clock assertion.
- Board under `-race`: lease exclusion, expiry on the injected clock, release on stop, ledger counts.
- Brain sequences with an injected clock (events in, packets out): the 300 ms gate, MP rest, each
  0xd7 outcome, the combat refusal, the breaker, `MinOpenGap`, an empty warehouse.
- e2e on the stack, asserting on packets (`WareHouseWithdrawalList` contents), not on the DB: a GM
  invites the crafter; steel materials deposited; steel appears in the warehouse list; a member bot
  deposits and withdraws its product (`TestLiveClanMember`: a scripted leader gives rank 6
  privilege 8 with a test-only `RequestPledgePower`, the member deposits and orders on one supply
  trip and withdraws the shots on the next). Prerequisites: `MembersCanWithdrawFromClanWH=True` on
  the stack, `//pledge create` and the clan level command added to the e2e GM filter
  (`internal/e2e/gm.go`), and no rejoin penalty: a clan the GM dissolves leaves none
  (`//pledge dismiss`), so the scenarios keep their characters and dissolve a clan a run left.

## Delivery (milestone M9, "Clan crafter", epic #184)

| PR | Scope | Depends on |
| --- | --- | --- |
| C1 #185 | Clan membership: codecs, `JoinClan` (GM by default, any player or `ClanInviters` for the crafter, D20), requester name, clan id and leader name (0x53), clan chat channel | — |
| C2 #186 | Warehouse errand: keeper spawns, open, lists, empty answer, deposit, withdraw, fee and slot checks, refusals, `MinOpenGap` | C1 |
| C3 #187 | `recipes.xml` reader; recipe book, make-self and make-info codecs with their system messages; recipe learning | I1 #168 |
| C4 #188 | Craft planner with chain planning (pure) | C3 |
| C5 #189 | `CraftBoard`: warehouse lease, crafter entry, orders, ledger, reservations, expiry, release on stop | C2 |
| C6 #190 | `crafter` preset: cycle, MP rest, weight guard, breakers, Create Item learning, idle farming | C4, C5 |
| C7 #191 | Shot materials: crystallize for the crafter; ore with members' orders | C6, I11 #178, I3 #170, I4 #171 |
| C8 #192 | Clan member option: keep hook and extra supply hook wiring, deposit and order, collect, board shot source | C6, I2 #169, I3 #170, I4 #171, I8b #182 (and I8 #175, M7 N6 #158 below it) |
| C9 #193 | Clan leader and GM orders, clan announcements | C6 |
| C10 #194 | Panel preset and status; e2e scenarios and GM filter commands | C6 |

## Risks

- **Materials the owner meant for something else.** D19 lets the crafter use every material.
  Chains run only when worth it, unpriced items need an order, and the owner keeps other items out
  of reach by not depositing them.
- **Shots depend on ore money.** Members pay for ore under their budget; a broke member orders
  fewer shots and fights without (the items plan's floor).
- **Warehouse full.** Products need slots; the crafter stops depositing new kinds at the cap and
  says so in clan chat once.
- **Clan size.** A level-1 clan holds 15 members; more member bots need a higher clan level.
- **Adena.** Fees are 30 per row; a broke crafter farms.

## Gaps

- Create Item above 1 and crystallizing wait for I11 #178 and the class master spawn
  (fatal10110/acis_golang#5419).
- The manufacture store (0xb2/0xb6, ported) for players outside the clan is not planned here.
