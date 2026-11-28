# Servitor lines: summon control and the five rows (#592)

Status: **plan, waiting for owner approval** (#592). No code ships from this document until the
owner approves it; the implementation issues it lists are filed with no milestone (owner decision
2 of #624: the owner picks it), each naming the milestone it proposes.

Five second professions are never picked and play their first profession's row today
([`strategy.md`](strategy.md) "Second professions (levels 40+)", "Servitor lines"; D23): Dark
Avenger (6, ← KN), Warlock (14, ← HW), Necromancer (13, ← HW), Elemental Summoner (28, ← EW) and
Phantom Summoner (41, ← DW). Their play rests on a servitor, and the bot has no model of one: it
decodes no pet packet, sends no pet order and never casts a summon skill. This plan decides the six
items #592 lists:

1. [Pet state from the server](#1-pet-state-from-the-server): the packets, wire order and widths.
2. [Orders](#2-orders): action ids, pacing and rule order.
3. [The servitor in the bot's rules](#3-the-servitor-in-the-bots-rules): heals, re-summons,
   `TimeToKill`, spot choice and pack threat.
4. [Supply](#4-supply): the crystals, the upkeep rate, and what is playable without a seller.
5. [The five rows](#5-the-five-rows) and the [branch tables](#branch-tables).
6. [Necromancer corpses](#6-necromancer-corpses): summoning inside the decay window.

Then the [owner questions](#owner-questions) and the [implementation issues](#implementation-issues).

Out of scope, as in #592: the Overlord (51, a group class, D23) and the Warsmith's golem and
cannons (the M9 crafter line, D18). Fighting *other players'* summons is M13
([`pvp/targets.md`](pvp/targets.md) "Summons and pets") and is not touched here.

**Paths.** `S/` is `internal/gameserver/` of the server repository
([fatal10110/acis_golang](https://github.com/fatal10110/acis_golang)); `D/` is its datapack,
`aCis_datapack/data/xml/`. Line numbers are at server `main` when this plan was written.

## Server behavior this rests on

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Summon type | A servitor is summon type 1, a pet 2; every pet packet carries it first. | `S/model/actor/summon/live_accessors.go:164-169` |
| What the owner sees | The owner gets `PetInfo` and `PetStatusUpdate` for its own summon; every other player gets `NpcInfo` with the summon flag (the bot decodes that already). The owner is skipped in the `NpcInfo` fan-out. | `S/network/summon_spawn.go:729-760`; `S/network/visibility.go:254-277` |
| Status refresh | A `StatusChanged` of the summon (HP, MP, level) sends the owner `PetStatusUpdate`. | `S/network/summon_events.go:182-183` |
| Death | Observers (the owner among them) get `Die` (0x06) for the summon's object id, then `AutoAttackStop`; once the death settles the owner gets SystemMessage 1520 (servitor passed away). The corpse decays after its template's `corpseTime` (7 s for these servitors). | `S/network/summon_events.go:239-249`; `S/model/actor/summon/death.go:54-93`, `:149-155` |
| Removal | `PetDelete` (0xb6) is sent when the summon is removed (decay, lifetime over, upkeep unpaid, unsummon), never on death alone. | `S/network/summon_events.go:368-370` |
| One summon | A second summon cast while a summon (a corpse included) stands is refused: SystemMessage 580. | `S/model/actor/cast/conditions.go:54-61`, `cast/controller.go:449-452`; `S/network/magic_skill.go:882` |
| Default behavior | A new servitor follows its owner. It never attacks on its own and does not defend its owner: it attacks only on an order. When its target is lost it turns follow back on and returns. There is no aggressive mode; `passive` is never set for a servitor. | `S/model/actor/summon/live.go:272-275`, `:561`; `S/model/actor/ai/summon.go:715-724`; `summon/attacked.go:18-23` |
| Kill credit | The servitor's damage counts as its owner's; the owner gets the kill's XP and SP, and the drop goes to the owner as top damage dealer. The servitor takes no XP share. | `S/data/manager/npcs_rewards.go:127-140`, `:166-168`, `:188-199`, `:321-327` |
| XP penalty | While the owner has a servitor out, *every* kill's XP is cut by the summon skill's `expPenalty` (0.05-0.15 for the second-profession summons, 0.9 for 1154 and the first-profession Mew, Mirage and Silhouette), whether or not the servitor fought. | `S/data/manager/npcs_rewards.go:283-292`; `S/network/summon_spawn.go:481` |
| Lifetime | `summonTotalLifeTime` 1200000. Each second the servitor loses `summonTimeLostIdle` 500, or `summonTimeLostActive` 1000 while the **owner** is in combat stance; a dead servitor's lifetime stands still. So a life lasts 40 min idle, 20 min if the owner fights throughout. | `S/model/actor/summon/live_lifecycle.go:53-82`, `live_accessors.go:644-649`; `D/skills/1200-1299.xml:1946-1948` |
| Upkeep | With `itemConsumeSteps` N > 0 the servitor pays `itemConsumeCountOT` of `itemConsumeIdOT` each time its remaining lifetime reaches a multiple of total/(N+1), down to 0: N+1 payments over a full life, the last one tick before it ends (remaining reaches exactly 0, and `Tick` expires only below 0). An owner who cannot pay loses it at once. | `S/model/actor/summon/lifetime.go:18-50`; `live_lifecycle.go:80-82` |
| Remaining-time bar | `SetSummonRemainTime` (0xd1) is sent whenever the remaining lifetime dropped 1/352 of the total since it was last shown (about every 3.4 s of lifetime). | `live_lifecycle.go:74-77`; `S/network/summon_events.go:258-260` |
| Pet orders | `RequestActionUse` (0x45): no flood protector of its own (only the 160 frames/s connection cap); refused while the owner is dead, fake-dead, out of control or observing. | `S/network/floodprotector.go:10-24`; `S/network/client_ingame.go:257-290`; `S/network/clientstats.go:10` |
| Corpse age | A `CORPSE_MOB` skill is refused (SystemMessage 1247) once a corpse is older than half its `corpseTime`, unless it was spoiled or seeded; checked at cast commit. A landed corpse skill consumes the corpse at once. | `S/handler/target/target_predicates.go:63-78`; `target_handlers_corpse.go:77-94`; `S/model/actor/cast/ai.go:231-247`; `cast/effects.go:250-270` |
| Corpse decay | A corpse lasts its template's `corpseTime` (default 7 s), twice that when spoiled or seeded. | `S/model/actor/npc/hostile_rewards.go:135-147` |

## 1. Pet state from the server

Four packets carry the servitor to its owner; the bot decodes all four in `internal/botwire` (D46)
with codec tests and golden bytes, each decoder consuming exactly `len(payload)`. Strings are the
usual NUL-terminated UTF-16LE. Offsets count from the byte after the opcode.

**`PetInfo` (0xb1)**, `S/network/serverpackets/petinfo.go:87-164`: sent on spawn and on every
full refresh (teleport, abnormal change, level). 125 fixed bytes, the name and the title, then 134
fixed bytes.

| Offset | Width | Field | Bot keeps |
| --- | --- | --- | --- |
| 0 | int32 | summon type (1 servitor, 2 pet) | yes: a 2 is not ours to play, logged and ignored |
| 4 | int32 | object id | yes |
| 8 | int32 | NPC template id + 1000000 | yes, minus 1000000 (the servitor's template: stats for `TimeToKill`) |
| 12 | int32 | 0 (attackable) | no |
| 16-28 | 4 × int32 | x, y, z, heading | yes (x, y, z) |
| 32 | int32 | 0 | no |
| 36, 40 | 2 × int32 | M.Atk speed, P.Atk speed | P.Atk speed |
| 44-72 | 8 × int32 | run and walk speed, four pairs | the first run speed |
| 76-100 | 4 × float64 | move multiplier, attack-speed multiplier, collision radius, collision height | collision radius |
| 108, 112, 116 | 3 × int32 | weapon, armor (0 for a servitor), 0 | no |
| 120-124 | 5 × uint8 | 1, 1, in combat, alike dead, 2 | in combat, alike dead |
| 125 | string | name | no |
| … | string | title (the owner's name) | no |
| +0 | int32 | 1 | no |
| +4, +8 | 2 × int32 | PvP flag, karma | no |
| +12, +16 | 2 × int32 | current and max feed (0 for a servitor) | no |
| +20-+32 | 4 × int32 | current HP, max HP, current MP, max MP | yes |
| +36, +40 | 2 × int32 | SP, level | level |
| +44-+60 | 3 × int64 | exp, exp at this level, exp at the next | no |
| +68, +72 | 2 × int32 | weight, weight limit | no |
| +76-+112 | 10 × int32 | P.Atk, P.Def, M.Atk, M.Def, accuracy, evasion, critical, move speed, P.Atk speed, M.Atk speed | P.Atk, P.Def, M.Atk, M.Def (the model's live numbers, buffs included) |
| +116 | int32 | abnormal effect mask | yes (stun 0x40, sleep 0x80, root 0x200, as `CharInfo`) |
| +120 | uint16 | mountable | no |
| +122 | uint8 | move type (1 swimming) | no |
| +123 | uint16 | 0 | no |
| +125 | uint8 | team | no |
| +126, +130 | 2 × int32 | soulshots and spiritshots per hit (ends at +134) | no |

**`PetStatusUpdate` (0xb5)**, `petinfo.go:64-83`: int32 summon type, int32 object id, int32 x, y,
z, string title, then int32 current feed, max feed, current HP, max HP, current MP, max MP, level,
then int64 exp, exp at this level, exp at the next (52 bytes after the title). The bot keeps
position, HP, MP and level.

**`PetDelete` (0xb6)**, `S/network/serverpackets/pet.go:32-38`: int32 summon type, int32 object
id. The servitor is gone; the bot clears it (and its corpse) from the world.

**`SetSummonRemainTime` (0xd1)**, `pet.go:40-47`: int32 total, int32 remaining, in ms of
lifetime. The bot keeps both, for upkeep accounting (section 4).

`PetStatusShow` (0xb0, `pet.go:25-30`, int32 summon type) only opens the pet window when the owner
targets its summon (`S/network/targeting.go:629`); the bot decodes it to an event it ignores, so it
is not counted as an unknown frame.

**The world model.** A `Servitor` value in the world (owned by the brain goroutine, like
`world.Self`): object id, template id, level, position, HP, max HP, MP, max MP, P.Atk, P.Def,
M.Atk, M.Def, attack speed, abnormal mask, in combat, dead, remaining and total lifetime, the
summon skill that made it, and its **target**. No pet packet carries a target; the servitor's
target comes from the observer frames the bot already decodes, keyed by the servitor's object id:
`Attack` (0x05) with the servitor as attacker sets it, `MoveToPawn` (0x60) with the servitor as
mover sets it while it closes in, `Die` of that target or `PetDelete` clears it. `Die` of the
servitor itself marks it dead; `PetDelete` removes it. A `NpcInfo` with the summon flag whose
title is the bot's own name never reaches the bot (the server skips the owner), so no confusion
between the owner's servitor and another player's summon arises.

## 2. Orders

**Action ids** (`S/network/summon.go:97-134`): servitor follow toggle 21, attack 22, stop 23,
unsummon 52, move 53. The pet ids (15, 16, 17, 19, 54) are never sent: they do nothing for a
servitor that its own ids do not do, and a bot has no pets in this plan. The action ids 36-1001
command a summon's own special skills; this plan sends none (owner question 8).

**Encoding.** `RequestActionUse` 0x45 {int32 action id, int32 ctrl, uint8 shift}
(`S/network/clientpackets/requestactionuse.go:20-29`), the encoder the bot has
(`botwire.EncodeRequestActionUse`), with ctrl 0 and shift 0: a servitor ordered without ctrl
attacks only what its owner may attack without force (`S/network/summon.go:252-267`), so it never
flags the bot.

**What each order does** (`S/model/actor/summon/command.go:103-209`,
`live_lifecycle.go:10-47`):

| Order | Effect | Ignored (no reply) | Refused |
| --- | --- | --- | --- |
| 22 attack | The servitor takes the **owner's current target** and attacks it; a creature it may not attack it only follows. | no target, the target is the servitor or the owner, the target is dead | out of control: SystemMessage 1864 |
| 21 follow | **Toggles** following. | the servitor follows and the owner is out of follow range | out of control: 1864 |
| 23 stop | The servitor goes idle: back to following its owner when following is on, else where it stands. | — | out of control: 1864 |
| 52 unsummon | The servitor leaves (no refund). | — | dead: 589; out of control: 1864; servitor attacking or owner in combat: 579 |

So an attack order is always two frames: `Action` (0x04) on the target when the bot's target is not
it already, then `RequestActionUse` 22.

**Which orders the bot sends.**

- **22 attack** is the only order of the ordinary fight: when the bot engages a target (pull,
  attacked, assist), it orders the servitor onto it unless the servitor's target already is it.
- **23 stop** recalls the servitor from a target the bot gives up: a flee (`escape`), a target the
  bot drops because another player claimed it. A servitor that follows its owner goes back to
  following when it goes idle (`S/model/actor/summon/live_accessors.go:722-735`,
  `S/model/actor/ai/summon.go:537-548`), so the stop alone brings it back.
- **21 follow** is not sent in the first version. Following is on from the summon, a lost target
  turns it back on (`S/model/actor/ai/summon.go:715-724`), and the bot never turns it off; 21
  toggles, so a blind 21 would turn it off.
- **52 unsummon** is not sent in the first version: an idle servitor loses lifetime at half rate,
  so walking to town and back costs less than a new summon (owner question 7).

**Pacing.** Pet orders have no server gate, so the frame bucket (`internal/botclient/pacing.go`,
`frameRate` and `frameBurst`, under the server's 160/s) bounds them, and the order logic adds its
own spacing so a bot never streams orders: one 22 per (servitor, target), repeated only when no
`Attack` or `MoveToPawn` of the servitor toward that target arrived within `ServitorOrderRetry`
(2 s); one 23 per give-up; a refusal (1864) holds orders for `ServitorOrderRetry`.

**Rule order.** The servitor attacks the bot's target whenever the bot fights; it **guards** the
bot when it has no live target of its own and something hits the bot or the servitor (the server
never makes it defend, see the table above): the bot selects that attacker and orders 22. In a
party the servitor takes the bot's assist target, as the bot does. The orders ride on existing
rules rather than one rule of their own, because each is the consequence of a fight decision:

| Where | What | Why there |
| --- | --- | --- |
| the fight step (`hunt`, `attacked`, `assist`, `party fight`) | order 22 onto the target before the opener; a mystic row's opener waits `ServitorLead` (1.5 s, or the servitor's first `Attack` on the target) so the servitor holds the monster's hate | the target is chosen there |
| `escape` | 23 | the bot leaves the fight |
| new `servitor` rule, after `heal` and before `raise` | Servitor Heal; guard (22 onto the bot's or the servitor's attacker when the servitor is idle) | after the bot's own survival (`critical`, `escape`) and the party's heals, before any new fight |
| new `summon` rule, after `rest` and before `rebuff` | summon (or re-summon) out of combat, then the servitor buffs in `rebuff` | the summon cast is 15 s and breaks on damage (`D/skills/*.xml` `hitTime` 15000); the bot summons rested, with MP for the fight |

## 3. The servitor in the bot's rules

**Heal.** Servitor Heal 1127 (HEAL, target SUMMON; 45 levels, power 145-936, reuse 10 s, hit 4 s,
cast range 600, `D/skills/1100-1199.xml:240`) when the servitor's HP is under `ServitorHealHP`
(50 %) and it is in range, from the `servitor` rule; under 25 % it outranks the bot's own attack
steps but never the bot's `critical` heals. The summoners learn it up to level 45; the Necromancer
keeps its Human Wizard level 12 (fixed), the Dark Avenger has none. Servitor Recharge 1126 is not
used: an unordered servitor spends MP only on the special skills this plan does not order.

**Buffs.** On every new servitor, and again when a buff's 1200 s ran out, the `rebuff` rule casts
the row's servitor buffs (1140 Servitor Physical Shield, 1139 Servitor Magic Shield @44, 1141
Servitor Haste @44, and the line's own: 1144 Fast Servitor, 1145 Bright Servitor, 1146 Mighty
Servitor @48). 1299 Servitor Empowerment is not cast: it holds the servitor in place for 30 s
(`ImobilePetBuff`). 1300 Servitor Cure and 1301 Servitor Blessing need the servitor's own effect
list, which only `PartySpelled` refreshes carry; they wait for a later issue.

**Death and re-summon.** On `Die` of the servitor the bot marks it dead; it cannot re-summon until
`PetDelete` (about 7 s later). It then re-summons from the `summon` rule once out of combat. In the
fight meanwhile it plays the row's own steps alone, and its `Outmatched` judgment drops the servitor
(below). If the servitor died twice within `ServitorDeathWindow` (10 min) on the same spot, the spot
is marked as too hard for the servitor, as a death of the bot marks it now.

**Lifetime and upkeep.** From `SetSummonRemainTime` the bot knows the remaining lifetime; from the
summon skill's level it knows the next checkpoint and what it costs. It never starts a pull when
the next checkpoint is due and the bot cannot pay it (the servitor would leave mid-fight); it lets
a servitor run out between fights rather than unsummoning it.

**`TimeToKill`** (`internal/charmodel/combat.go:154`). The servitor is a second attacker: the
combined kill time is 1 / (1/T_bot + 1/T_servitor), where T_servitor is `TimeToKill` of a Profile
built from the servitor's template (datapack NPC stats: level, P.Atk, attack speed, the summon
skill's level) and, once out, the live P.Atk from `PetInfo`. Spot choice (Design section 11) and
the hunt's target choice use the combined time while a servitor is out or can be summoned.

**Pack threat.** The bot's own `TimeToDie` stays as it is in the first version: a servitor that
holds hate shields the bot, but the bot gives itself no credit for that (the conservative choice;
a servitor that dies leaves the bot with the whole pack). `Crowded` and `Outmatched` keep counting
attackers on the bot only; attackers on the servitor count toward the servitor's heal and the
spot's danger, not toward the bot's flight.

**Experience.** The XP penalty applies while a servitor is out, so the bot never summons a skill of
`expPenalty` ≥ 0.5 to level (1154 Summon Corrupted Man; the first-profession 1225 Mew the Cat, 1227
Mirage the Unicorn, 1228 Summon Silhouette, all 0.9). The second-profession summons cost 5-15 % of
XP, paid back by the faster kills (`TimeToKill`).

## 4. Supply

**What each summon costs** (`D/skills/*.xml`; C = Crystal: C-Grade 1459, D = Crystal: D-Grade 1458):

| Summon | Learned (row) | Cast cost | Upkeep: count per checkpoint, checkpoints per life (N+1) | One full life | XP penalty |
| --- | --- | --- | --- | --- | --- |
| 283 Summon Dark Panther | 40-74, 7 levels (DA) | 1-4 C | 1-4 C, 5 | 6-24 C | 0.15 |
| 1276 Kai the Cat, 1277 Merrow the Unicorn, 1278 Soulless | 40-74, 14 levels (WK, EL, PS) | 1-4 C | 1-3 C, 15 | 16-49 C | 0.10 |
| 1331 Feline Queen, 1332 Unicorn Seraphim, 1333 Nightshade | 56-74, 10 levels (WK, EL, PS) | 1-4 C | 1-3 C, 15 | 16-49 C | 0.05 |
| 1129 Summon Reanimated Man | 44-74, 7 levels (NC, a corpse) | 2-4 C | 1-4 C, 5 | 7-24 C | 0.15 |
| 1334 Summon Cursed Man | 56-74, 7 levels (NC, a corpse) | 1-3 C | 2-4 C, 5 | 11-23 C | 0.15 |
| 1154 Summon Corrupted Man | 40-70, 6 levels (NC, a corpse) | 3 D | none | 3 D | 0.9 (never used to level) |
| 1111 Kat the Cat, 1226 Boxer the Unicorn, 1128 Summon Shadow | 20-74, 18 levels (HW, EW, DW and their lines) | 3-8 D | 0-6 D, none or N+1 (N 0-14 by level) | per level | 0.3 |
| 1225 Mew the Cat, 1227 Mirage the Unicorn, 1228 Summon Silhouette | 20-74, 18 levels (HW, EW, DW) | 1 D | none | 1 D | 0.9 (never used to level) |

A life is 20 min of lifetime at combat cost, so a bot fighting without pause spends three lives an
hour: about 50 C an hour for a level-40 Warlock, up to about 150 C at 74; a Dark Avenger 18 to 72.

**Sources.** No spawned NPC sells crystals: the only buy lists holding 1458 and 1459 are 5720 of
the Sales Cat of Ivory Tower (31075), which no spawn list places, and 300534 (npc -1, the GM shop)
(`D/buyLists.xml`, `D/spawnlist`). No monster drops or spoils them (#627; `docs/crafting.md`).
Crystals come only from crystallizing graded gear (skill 248: D at 20, C at 40 Warsmith,
`docs/crafting.md` "Crystallize"), from players (trade, private stores) and from a GM. Crystal: D
has the same gap and waits on #636 (deferred; no cubics in M10, owner decision 1 of #624).

**What is playable without a crystal seller.** A servitor line plays its servitor only **while
the bot holds the crystals for the cast plus one upkeep step** of the summon it would use; the
reagent model of D49 already refuses a cast without its item, and the bot never casts the summon
otherwise. With no crystals, or when they run out (the server unsummons at the unpaid checkpoint),
the row plays **its own steps without a servitor**: the Necromancer is a full nuker (Death Spike,
Vampiric Claw, curses), the Dark Avenger a full tank; the Warlock and both Summoners fall back on
their first profession's nukes, which their own trees no longer raise ("fixed"), so they are weak
without a servitor. This is why the branch tables offer them only behind a switch (below) until a
source exists.

The shop errand (D49) does not stock crystals: they have no spawned seller, as Crystal D for the
cubics. A crystal source, when the owner picks one (owner question 1), is one more reagent row for
D49: `ReagentCastsMin` and `ReagentCastsMax` count casts *and* upkeep checkpoints for a summon
skill, so a full life's crystals count as one cast.

## 5. The five rows

Rows in the three-table form and notation of [`strategy.md`](strategy.md) "Second professions
(levels 40+)" (row start 40, `@N` first learned past 40, `(fixed)` an attack step the own tree does
not raise), plus a fourth table for the servitor. Row ids: DA Dark Avenger, WK Warlock, NC
Necromancer, EL Elemental Summoner, PS Phantom Summoner (WL is the Warlord). Skill lists are the
class trees, `D/classes/humanFighter.xml` (6), `humanMystic.xml` (13, 14), `elvenMystic.xml` (28),
`darkMystic.xml` (41). Dyes are the second-profession symbols every one of these classes may draw
(`D/hennas.xml` 37+); the Dark Avenger cannot draw the MEN symbols, and needs none.

#### Gear and range (columns 1, 2, 9, 11)

| Row | Class (id), role ← first profession | 1. Weapon kind, and why | 2. Armor pair; preferred C-grade sets | 9. Fight range | 11. Dyes |
| --- | --- | --- | --- | --- | --- |
| DA | Dark Avenger (6), Support ← KN | SWORD or BLUNT with a SHIELD: Shield Stun keeps its shield gate, Judgment takes SWORD or BLUNT. | Heavy; Full Plate Set (as PL). | Melee; Aggression 400; Drain Health 600 | CON+1 DEX− (40, 52, 112) |
| WK | Warlock (14), Damage ← HW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (as SN). | Cast 750 (Blaze, fixed), behind its servitor | INT+1 WIT− (44, 56, 116) and INT+1 MEN− (43, 55, 115) |
| NC | Necromancer (13), Damage ← HW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (as SN). | Cast 900 (Death Spike, Vampiric Claw); corpse skills 400, corpse summons 40 (section 6) | INT+1 WIT− (44, 56, 116) and INT+1 MEN− (43, 55, 115) |
| EL | Elemental Summoner (28), Damage ← EW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (as SG). | Cast 750 (Solar Spark, Aqua Swirl, fixed); Wind Shackle 600 | INT+1 MEN− (43, 55, 115): WIT is its strength |
| PS | Phantom Summoner (41), Damage ← DW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (as SH). | Cast 750 (Shadow Spark, Twister, fixed); Wind Shackle 600 | INT+1 WIT− (44, 56, 116) |

#### Combat (columns 3-7)

| Row | 3. Opener | 4. Rotation (priority order, gates) | 5. Sustain | 6. Self-buffs and toggles | 7. Rest |
| --- | --- | --- | --- | --- | --- |
| DA | 28 Aggression from 400, a pull; the servitor is ordered onto the target once the pull lands. | 28 Aggression open; 70 Drain Health open; 18 Aura of Hate A≥2; 92 Shield Stun [SHIELD]; 401 Judgment [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @55; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (fixed); melee. | 70 Drain Health S<60; 46 Life Scavenge S<70 on a fresh corpse (section 6); 45 Divine Heal (fixed) S<40; 110 Ultimate Defense S<20 | 82 Majesty, 72 Iron Will @43, 86 Reflect Damage (out of combat); 322 Shield Fortress @64 (combat toggle, MP>30) | HP; 226 Relax (base tree) |
| WK | Servitor order, then 1220 Blaze (fixed) from 750 after `ServitorLead`. | 1220 Blaze (fixed); 1184 Ice Bolt (fixed); 1172 Aura Burn (fixed) ≤150; melee only under MPReserve. Servitor: 1127 Servitor Heal under 50 % (rule `servitor`). | 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind (fixed) when MP under 30 % and S≥70 | 1078 Concentration; 1262 Transfer Pain (combat toggle while the servitor is out, MP>30); servitor buffs 1140, 1139 @44, 1141 @44, 1144 Fast Servitor @48 | MP; plain sit |
| NC | 1168 Curse: Poison MP>60 from 600, else 1148 Death Spike @44 from 900; before 44 1220 Blaze (fixed) from 750. | 1168 Curse: Poison open MP>60; 1148 Death Spike @44; 1234 Vampiric Claw; 1155 Corpse Burst @48 on a fresh corpse A≥2 (section 6); 1220 Blaze (fixed); 1184 Ice Bolt (fixed); 1172 Aura Burn (fixed) ≤150; melee only under MPReserve. Servitor: 1127 Servitor Heal (fixed) under 50 %. | 1234 Vampiric Claw S<70; 1151 Corpse Life Drain S<70 on a fresh corpse (section 6); 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind when MP under 30 % and S≥70 | 1078 Concentration | MP; plain sit |
| EL | Servitor order, then 1206 Wind Shackle MP>50 from 600 after `ServitorLead`, else the first ready nuke from 750. | 1206 Wind Shackle open MP>50; 1264 Solar Spark (fixed); 1175 Aqua Swirl (fixed); 1184 Ice Bolt (fixed); 1172 Aura Burn (fixed) ≤150; melee only under MPReserve. Servitor: 1127 Servitor Heal under 50 %. | 1216 Self Heal (base) S<40 (no drain) | 1078 Concentration; 1262 Transfer Pain (as WK); servitor buffs 1140, 1139 @44, 1141 @44, 1145 Bright Servitor @48 | MP; plain sit |
| PS | Servitor order, then 1206 Wind Shackle MP>50 from 600 after `ServitorLead`, else the first ready nuke from 750. | 1206 Wind Shackle open MP>50; 1266 Shadow Spark (fixed); 1178 Twister (fixed); 1184 Ice Bolt (fixed); 1172 Aura Burn (fixed) ≤150; melee only under MPReserve. Servitor: 1127 Servitor Heal under 50 %. | 1147 Vampiric Touch (fixed) S<70; 1157 Body To Mind (fixed) when MP under 30 % and S≥70 | 1078 Concentration; 1262 Transfer Pain (as WK); servitor buffs 1140, 1139 @44, 1141 @44, 1146 Mighty Servitor @48 | MP; plain sit |

#### Numbers and summary (columns 8, 10, 12)

| Row | 8. MPReserve | Sit HP / MP | Stand HP / MP | Critical | Crowded | Outmatched | 10. Spot | 12. Plays like |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| DA | 20 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | tank 1.5 | A shield tank with a panther: pulls with Aggression, holds the pack, the panther adds damage. |
| WK | 20 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | normal 2.0 with a servitor out, else glass cannon 3.0 | A cat summoner: Kai, then the Feline Queen, tanks and kills; the Warlock heals and buffs it and adds its Human Wizard nukes. |
| NC | 10 | 55 / 20 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | A curse nuker: Curse: Poison, Death Spike and Vampiric Claw from 900; raises a corpse servitor when one falls close. |
| EL | 20 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | normal 2.0 with a servitor out, else glass cannon 3.0 | A unicorn summoner: Merrow, then the Seraphim, tanks; the Summoner shackles, heals it and adds water nukes. |
| PS | 20 | 55 / 30 | 95 / 90 | 35 | 90 | 65 | normal 2.0 with a servitor out, else glass cannon 3.0 | A shadow summoner: Soulless, then Nightshade, tanks; the Summoner shackles, heals it and drains. |

The summoners keep 20 % MP for Servitor Heal (96 MP at the top level) and sit at 30 % MP; their
thresholds are otherwise their first profession's. The DA's are the tanks' (as PL), the NC's DW's
and SH's, as the curse nuker it is.

#### The servitor (new)

| Row | Summon (by level) | Heal | Buffs | Without a servitor |
| --- | --- | --- | --- | --- |
| DA | 283 Summon Dark Panther | none (the Dark Avenger has no Servitor Heal) | none | plays the row as a tank |
| WK | 1276 Summon Kai the Cat, 1331 Summon Feline Queen @56 | 1127 under 50 % | 1140, 1139 @44, 1141 @44, 1144 @48 | fixed HW nukes, spot glass cannon |
| NC | 1129 Summon Reanimated Man @44, 1334 Summon Cursed Man @56, each from a fresh corpse (section 6); never 1154 (XP penalty 0.9) | 1127 (fixed) under 50 % | none | plays the row as a nuker |
| EL | 1277 Summon Merrow the Unicorn, 1332 Summon Unicorn Seraphim @56 | 1127 under 50 % | 1140, 1139 @44, 1141 @44, 1145 @48 | fixed EW nukes, spot glass cannon |
| PS | 1278 Summon Soulless, 1333 Summon Nightshade @56 | 1127 under 50 % | 1140, 1139 @44, 1141 @44, 1146 @48 | fixed DW nukes, spot glass cannon |

The row summons its newest summon it can pay for (section 4); the D-crystal first-profession
summons (1111, 1226, 1128) are not used: a C-crystal line that holds only D crystals plays without
a servitor (owner question 1).

### Branch tables

Proposed weights for Design section 6, **behind a persona switch `ServitorLines`** (a D6 persona
default, off until the owner turns it on, owner question 2). With the switch off the tables stay
as they are and these lines are reached only by a pin or by a bot made that way.

| Base class | Support role | Damage role (weights) |
| --- | --- | --- |
| Human Fighter | Knight → Paladin 2, Dark Avenger 1 | unchanged |
| Human Mystic | unchanged | Human Wizard → Sorcerer 2, Necromancer 1, Warlock 1 |
| Elven Mystic | unchanged | Elven Wizard → Spellsinger 2, Elemental Summoner 1 |
| Dark Mystic | unchanged | Dark Wizard → Spellhowler 2, Phantom Summoner 1 |

The Necromancer needs no crystals to play its row (only its summons do), so it could be offered
with the switch off (owner question 4).

## 6. Necromancer corpses

**The window.** A monster's corpse lasts its template's `corpseTime` (7 s by default, per template
from the datapack, as the sweep reads it: `internal/botclient/travel.go` `corpseTimes`). A
`CORPSE_MOB` skill (1129, 1334, 1154 summons; 1151 Corpse Life Drain; 1155 Corpse Burst; the Dark
Avenger's 46 Life Scavenge) is accepted only while the corpse is younger than **half** its
`corpseTime` (3.5 s by default), checked when the cast commits; a spoiled or seeded corpse is
usable for its whole doubled life (14 s). The skill consumes the corpse on landing, so a summon
or a drain uses it up, and a dwarf's sweep of it is lost.

**Reach.** The corpse summons have cast range 40 and a 1.5 s hit time; the corpse drains and
Corpse Burst 400 and 1.5 s. A Necromancer that killed from 900 cannot walk to the corpse in time
(about 121 units/s for a human mystic, `docs/pvp/server-casts.md` "Run speed": 860 units take 7
s). So a corpse is **fresh and in reach** when, at the moment the bot sees its `Die`, the walk to
within cast range finishes before the cutoff: distance − cast range − collision ≤ run speed ×
(corpseTime/2 − `CorpseMargin` (0.5 s) − the frame delay). In practice: a monster that died in
melee range of the bot, which is where a caster's monsters die when they reach it, or next to the
servitor while the bot stands close.

**The rule.** A new `corpse` rule right after `sweep` (only the critical, heal and flee rules go
first, as for Sweeper, Design section 8), which:

1. tracks every monster `Die` in sight with its death time and template corpse time;
2. when the row wants a summon (no servitor, or `PetDelete` of a dead one seen) and holds the
   crystals: casts the row's corpse summon on the nearest fresh corpse in reach;
3. else when the bot is under the step's sustain bound: casts 1151 Corpse Life Drain on it;
4. else with two or more attackers near a fresh corpse: 1155 Corpse Burst;
5. never on a corpse the bot spoiled and has not swept, nor one a party member spoiled (the
   party's sweep comes first);
6. drops a corpse on SystemMessage 1247 (too old), on its `DeleteObject`, or once its window ends.

The 15 s summon cast of the other lines does not apply: the corpse summons cast in 1.5 s, so the
Necromancer may raise its servitor in a fight.

**Getting a corpse.** In the first version the Necromancer takes the corpses that fall in reach on
their own; it does not change how it fights to make one (owner question 4).

## Owner questions

1. **Crystal source.** No spawned NPC sells Crystal: C-Grade or D-Grade (section 4). Should the
   servitor lines ship now as "plays its servitor while it holds crystals" (GM-given, crafter- or
   player-supplied, carried by the persona), or wait for a source (a spawned seller in the server's
   datapack, the clan crafter's crystallizing, buying from player stores)? And should a line that
   holds only Crystal D use its first-profession D summon (Kat the Cat, Boxer, Shadow: XP penalty
   0.3)?
2. **Branch tables.** Offer the lines through the weights above behind `ServitorLines` (default
   off), or keep them pin-only until a source exists?
3. **PvP.** Does the servitor attack players (M13: duels, flagged and karma players, sieges)? The
   bot orders without ctrl, so a servitor never attacks a player the bot may not attack without
   force. The first version orders it onto monsters only.
4. **Necromancer.** Ship the NC row as a nuker now (no crystals needed) with corpse summons only on
   corpses that fall in reach, or also make it close in on a dying target (under 30 % HP) to get a
   corpse when it wants a servitor?
5. **Milestone** for the implementation issues below (filed with none).
6. **Dark Avenger.** Its row is the Paladin-like tank plus the panther; keep it a Support line next
   to Paladin (as proposed), or a Damage line?
7. **Unsummon.** The first version never unsummons: idle lifetime burns at half rate, and a new
   summon costs a cast and a 15 s channel. Unsummon (52) before a teleport or a long town errand
   instead?
8. **Special skills.** The servitors have their own skills (Feline Queen 4699-4701, Unicorn
   Seraphim 4702-4704, Nightshade 4705-4707 and others), ordered through action ids 36-1001 and
   paid with the servitor's MP (Servitor Recharge 1126). Leave them out of the first version?

## Implementation issues

Filed from this plan with no milestone (the owner picks; each names the milestone it proposes),
in build order. Each depends on the plan's approval.

| Issue | What | Depends on |
| --- | --- | --- |
| #757 | Pet state: decoders for `PetInfo`, `PetStatusUpdate`, `PetDelete`, `SetSummonRemainTime`, `PetStatusShow`; the `Servitor` world model and its target from `Attack`, `MoveToPawn`, `Die` (section 1) | — |
| #758 | Servitor orders: attack, stop and follow ids with their pacing and refusals; the attack order in the fight steps, the guard, `ServitorLead` (section 2) | #757 |
| #759 | Summon, Servitor Heal, servitor buffs, re-summon after death, lifetime and upkeep accounting, the crystal gate and the XP-penalty rule (sections 3, 4) | #757, #758 |
| #760 | The servitor in `TimeToKill` and spot choice (section 3) | #757 |
| #761 | The five rows and the branch-table weights behind `ServitorLines` (section 5) | #759 (the NC row's nuker steps need none) |
| #762 | The Necromancer's corpse rule: corpse summons, Corpse Life Drain, Corpse Burst; the DA's Life Scavenge (section 6) | #759 |
