# PvP persona: bots that fight players (M13)

Status: owner approved D35-D39 as proposed on 2026-11-25 (`docs/plan.md`, "Owner decisions").
The owner approved D40-D43 as proposed on 2026-11-26, with the proposed answers to every owner
question in [war](war.md), [Olympiad](olympiad.md) and [CP potions](cp-potions.md): X14-X19 may start.
Milestone M13 ("PvP persona"): epic #405, issues #406-#420, follow-ups #421-#429, #602-#604,
#612-#614, #718-#726.

## Paths

Server paths (`network/`, `duel/`, `model/`, `skill/`, `handler/`, `task/`, ...) are under
`internal/gameserver/` in [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang);
datapack paths (`classes/`, `skills/`, `items/`, `zones/`, `armorSets.xml`, `buyLists.xml`,
`multisell/`) under `aCis_datapack/data/xml`; config under `aCis_gameserver/config/*.properties`
(the Go server's shipped copies, `ops/docker/config/`, carry the same values; a missing key takes
the Go default, which can differ: `AwardPKKillPVPPoint` is `False` in `players.properties:144`,
`true` in `task/pvpflag.go:48`, `DefaultPvPFlagOptions`; this plan assumes the shipped files). Other
paths are this repository's, mostly `internal/botclient`. M10 is `docs/strategy.md` (epic #319,
#320-#333; build, `charmodel`, `Step` gates, "Behavior per class", presets `farmer`, `support`,
`merchant`, `crafter`, `town`; D21-D24, approved in open PR #346). M11 is [bot parties](../parties/README.md)
(D25-D29, approved 2026-11-25).

## Problem and goal

The owner asked: "PVP - prepare a PVP persona that will fight with players." And: "PVP also needs
different kinds of items, and a fight strategy." Players fight by duel (nothing lost), as flagged or
karma players or in an arena (an arena death costs nothing, an open-world one experience), and by
killing a clean player (karma). Today every bot only defends itself with its PvE rotation and
declines every duel ([the bot today](server.md#the-bot-today)).

Goal: a `pvp` preset that takes and makes duels, hunts flagged and karma players of its tier,
visits arenas and fights back (open-world PK exists, off); fights like a player who knows its class
(control first, interrupts, matchups, CP, break-off); values gear for PvP too; never gets karma by
accident; and handles death and karma per the server's rules.

Clan wars as targets ([war](war.md)), sieges ([siege](siege.md)) and the Olympiad
([Olympiad](olympiad.md)) ship in X14-X16 (D40-D42). Out of scope: fighting summons for their own sake beyond killing one before its owner when that pays off (#428,
[summons](targets.md#summons-and-pets)), kiting monsters. Second-profession PvP rows are planned in
[second-profession rows](rows-second.md) (#421, X20-X28).

## Approach

A PvP layer in the brain over each bot's M10 rotation: new wire, an opponent record, duel and arena
states, `MayFight` and guards against accidental karma, a PvP order per class row, a `charmodel`
PvP value blended into M10's gear score, a karma policy. No server change.

## Owner decisions

- **D35, modes.** A new `pvp` preset (M10 Damage role, farms between fights): duels, flagged and
  karma players, arenas on; open-world PK off (opt-in per bot). Other presets keep fight back only
  with `SupportFlagged` off and now decline duels at once, but every preset fights players with the
  PvP layer (`support` bots play KN, CL, EO, SO; party PvP in any party). PvP events wake the
  brain after 200-600 ms, amending `docs/plan.md`'s shared 300-1200 ms for them only
  ([reaction](wire.md#changes-to-todays-code-x4)). Persona defaults (D6). [modes](modes.md)
- **D36, finding fights.** Takes any duel it can; challenges a player who whispers `duel` (extends
  D3 as the approved D20 did); challenges non-bot players and hunts flagged and karma players of its
  tier when the engage estimate passes; visits the nearest arena on a timer. Holds and bounds:
  [finding](finding.md#sources).
- **D37, items.** One loadout scored by M10's `Value` and `PvPValue` blended by relative gains,
  50 % each for `pvp` (others 0 %); no set swap; Mithril Heavy (D) bought like any M10 gear,
  B-and-up PvP sets, PvP weapons and boss jewels only worn; cures bought on M8 trips; CP potions
  kept, never sold or bought (buying amended by D43). [model](model.md)
- **D38, bots vs bots.** Outside an arena a bot never attacks, hunts or challenges a bot and fights
  one back only after its hit or offensive cast; in an arena anyone but its party. No bot duels.
  [targets](targets.md)
- **D39, death and karma.** Duels to the end, no surrender; arenas to the death under a death cap;
  open world, break off by the row and avoid its killer ([by fight](karma.md#by-fight)). No karma
  by accident and, in a duel, nothing that would cancel it ([guards](guards.md), at a cost in the
  small arenas). Opt-in PK mode and life with karma: [karma](karma.md).
- **D40, clan wars (approved 2026-11-26).** A mutual-war enemy (`RelationChanged` 0x8000) is a target like a
  flagged player for `pvp` bots (`War` mode, D6), attacked with force; every war mark is dropped
  when a war ends, and a player's mark when its `CharInfo` shows another clan. Bots never declare, answer, stop or surrender a war. [war](war.md)
- **D41, sieges (approved 2026-11-26).** Bots never register for a siege. Every bot walks out of a running
  battlefield it does not fight in; a `pvp` bot (`Siege` mode) of a clan its leader registered
  fights the other side's players there, bots only after their hit (D38). [siege](siege.md)
- **D42, the Olympiad (approved 2026-11-26).** Opt-in per bot, off for every preset: noblesse at the class
  master, bounded registration, the drawn opponent fought whoever it is (a bot too: D38 gains the
  stadium); never spectates, trades points or claims hero status. A third-profession bot competes
  too (owner, 2026-11-28, #729). [Olympiad](olympiad.md)

- **D43, buying CP potions (approved 2026-11-26).** CP potions are bought (amends D37; the keep rule stays):
  from player sell stores for `pvp` bots; an opt-in `SevenSigns` mode (off for every preset) signs
  up with Dusk, farms seal stones and, in a validation week its cabal won, buys them at a priest for
  Ancient Adena. Bots never contribute stones, never pay the Dawn fee, never vote Strife; their
  seal votes still weigh on the seals, so only the bots named in `SevenSigns.Members` join (at most
  `SevenSigns.MaxMembers` per run, default 1, checked when the config loads), voting the seal Dusk
  holds if any, else Gnosis.
  [CP potions](cp-potions.md)

## Delivery (milestone M13)

Dependencies name M10 issues by number. Every PR also reads [server](server.md) (the bot today,
pacing for its senders) and the server facts of its topic: [duels](server-duels.md) (X1, X4, X4b,
X5), [flags, arenas and karma](server-karma.md) (X2, X4, X4b, X5, X11), [casts](server-casts.md)
(X1, X3, X4, X4b, X6, X6b, X7-X9), [items](server-items.md) (X2, X6b, X10).

| PR | Scope | Depends on | Reads |
| --- | --- | --- | --- |
| X1 | Wire: duel packets, `RequestTargetCancel`, `MagicSkillCanceled` event, new `CharInfo`, `NPCInfo`, `UserInfo` fields, SystemMessage ids; codec tests. | S1 (#320, same `UserInfo` decoder) | [wire](wire.md#wire-x1) |
| X2 | Datapack: zone reader (`NPoly`, `Cuboid`; water on `LoadWaterZones`), skill landing, `nextActionAttack`, `offensive`, target and radius fields, cure negate fields, jewel, set and buff vulnerabilities, `pvp*Dmg`, `cpTable`. | S2 (#321) | [wire](wire.md#datapack-x2) |
| X3 | `charmodel` PvP: `LandRate`, cast-break chance, archetypes, class-to-row map, `RowTable` and reference opponents, `PvPValue`, engage estimate; tests on test-built tables; no row data. | S3 (#322), S11 (#330), X2 | [model](model.md) |
| X4 | World model: opponent record (fight kind, `unfought`), duel and arena states, duel result, `MayFight` but rules 3 (arena) and 7 (PK mode), summons by own flag and karma, the code changes (reaction included), duel support. | X1, X2, X3 (class and tier map), S4 (#323, `Step`), S6 (#325, rotation runner) | [wire](wire.md#world-model-x4) (and changes, tests), [targets](targets.md#mayfight) (and summons, `TestPvPSummon`), [guards](guards.md#duel-support) (and `TestPvPDuelNoFlaggedSupport`), [finding](finding.md#tests) (`TestPvPDuelFight`) |
| X4b | The guards: Esc and late Esc, follow-up swing, arena edge with the hit-time predictor, area and bystander reach, pole reposition and hold; duel ground and the follow stop; `MayFight` rule 3; `PvPPolicy` modes, the `pvp` preset (D35, D38). | X4, S9 (#331) | [guards](guards.md), [layer](layer.md#interrupts) (the predictor), [finding](finding.md#duel-ground) (and its `TestPvPDuelFight` cases), [targets](targets.md), [modes](modes.md) |
| X5 | Finding fights: duel answer, whispered `duel`, own challenges and holds (unexplained 1940), clear of monsters, hunting with the estimate (empty `RowTable` until X6), arena trips, targets and leave (D36). | X3, X4b | [finding](finding.md), [targets](targets.md) (hunting, arena targets, tests), [model](model.md#engage-estimate), [guards](guards.md#arena-edge) |
| X6 | The PvP layer: `Step` fields, PvP order, long-cast and sleep rules, interrupts, land-rate control, kite, pool estimate; `pvpRowTable` into X5's estimate; `TestPvPRowsShipped` with no row. | X3, X4b, X5, S6 (#325), S11 (#330) | [layer](layer.md) (but CP, cures, break off), [model](model.md#row-table), [rows](rows.md#tests) |
| X6b | CP potions, cures, break-off, the break-cast gate and Ultimate Defense. | X6 | [layer](layer.md#cp-cures-pool-break-off) (and its tests) |
| X7 | Fighter PvP rows (HF, EF, DF, OF, WF, KN, WA, RO, EK, ES, PK, AS, OR, MO, SC) and their `TestPvPRowsShipped` cases. | X6, X6b (Ultimate Defense), S6b (#326) | [rows](rows.md), [layer](layer.md), [guards](guards.md#tests) |
| X8 | Mystic PvP rows (HM, EM, DM, OM, CL, HW, EO, EW, SO, DW, OS) and their `TestPvPRowsShipped` cases. | X6, X6b (OM break-off), S7 (#327) | [rows](rows.md), [layer](layer.md), [guards](guards.md#tests) |
| X9 | Party PvP for every preset in any party: enemy set, healer marking, peel, healer buffs under its own `SupportFlagged`. | X6, X7 (KN peel), X8 (Mental Shield); `farmer` in bot parties needs M11 (cases: #423) | [party](party.md), [layer](layer.md) (step fields, sleep rule) |
| X10 | PvP gear: mix table, `PvPValue` in gear, set, jewel and dye scores, shipped-row model tests, cures on the supply list, CP potion and cure keep rule (D37). | X3, X6b, X7, X8, S5 (#324), M8 I3 (#170, supply list), I4 (#171, `keeps`) | [model](model.md), [layer](layer.md#tests) (supply-list and junk cases) |
| X11 | Death and karma: arena return loop, avoid list, PK mode (`MayFight` rule 7), karma state (flee or PK, chaotic restart, karma route, work-off) (D39). | X4, X5, X6b, X7, X8 (`TestPvPPKCap` rows) | [karma](karma.md), [targets](targets.md#hunting), [finding](finding.md) (holds) |
| X12 | Panel switches, PvP counts, the four counters. | X4, X5, X6, X11, S9 (#331) | [modes](modes.md#panel-and-metrics-x12) |
| X13 | e2e live scenarios for X4-X11, at most 4 characters each, GM included. | each; M11 for `TestLivePvPHealer` | the e2e cases in each file's Tests |
| X14 | Clan-war targets (#602): relation bits, war mark and its clearing, `MayFight` rule 6b, forced attack, guard. | D40, X4b, X5 | [war](war.md) |
| X15 | Sieges (#603): 283/284 by position, leave a battlefield, `UserInfo` siege state, fight the other side. | D41, X2, X4b, X11 | [siege](siege.md), [finding](finding.md#duel-ground) |
| X16 | The Olympiad (#604): noblesse, registration, the match. | D42, X4, X6, M8 #178 | [Olympiad](olympiad.md) |
| X17 | CP potions from player sell stores (#612): `CPStock`, store rows on the `pvp` list. | D43, X10, M8 store buying | [CP potions](cp-potions.md) |
| X18 | Seven Signs member mode (#613): sign-up, dungeon gatekeeper leg, catacomb spots, stone keep. | D43, M8 supply trip | [CP potions](cp-potions.md) |
| X19 | Priest trip in Seal Validation (#614): multisell wire, stones to Ancient Adena, CP potions by multisell 500. | D43, X18 | [CP potions](cp-potions.md) |
| X20 | Second-profession rows in the row table (#718): 24 row ids, class-to-row map, `RowClasses` tier 2. | X7, X8, M10 #332 | [second-profession rows](rows-second.md#delivery) |
| X21-X28 | Second-profession PvP rows (#719-#726): tanks, songs, heavy damage, daggers, archers, tyrant, nukers, healers. | X20 | [second-profession rows](rows-second.md) |
| Backlog | The gaps below; party duels shipped with M11 (#424, [party duels](party.md#party-duels)). | X7, X8 | [Risks and gaps](#risks-and-gaps) |

Most urgent: X1-X5 (X4b included) give a bot that takes duels and fights in arenas with its M10
rotation; X6-X8 make it fight like its class.

## Risks and gaps

Risks: **annoying players** (challenge pop-ups, bounded by the holds and tier match, switchable per
bot; the holds are shared by the bots of a process, so one player gets one own challenge per 30
minutes from all of them, #425, though bots of separate processes still hold apart);
**accidental PK** only from what the guards cannot see ([left over](guards.md#arena-edge)); the
**arena cost** (D39); **duels cancelled** with nothing restored by what the bot cannot prevent (a
wandering monster, the opponent's own move, a siege starting, a server follow leg between checks;
[duel ground](finding.md#duel-ground)); **blind HP** (errs toward fighting on); **unknown level**
(tier as proxy: a level-39 first profession will hunt a level-20 one, and the engage estimate
assumes equal level; hunting only flagged and karma players keeps this fair); **size** (X6 lands
with no row); **arena deaths** (the 5-death cap and stay limit).

To file in the M13 backlog: second-profession rows (#421, planned in
[second-profession rows](rows-second.md), #718-#726); a Colosseum preference for the OM row,
shot-less mystics and bow rows, when a gatekeeper reaches it (#422, shipped); party PvP with a `farmer` in a bot-led party (M11, #423, `TestPvPTargetHealerFarmer`); party duels
(M11, #424, shipped: [party duels](party.md#party-duels)); a challenge hold shared by all bots (#425, shipped); buying CP potions (#427, planned in
[CP potions](cp-potions.md), #612-#614, D43);
fighting a summon first when that pays off (#428, shipped: [summons](targets.md#summons-and-pets));
command-channel mates in an arena (#429, shipped: [rule 2](targets.md#mayfight)). Clan wars, sieges and the Olympiad are planned in
[war](war.md), [siege](siege.md) and [Olympiad](olympiad.md) (#602-#604), D40-D42.

## Files

[server](server.md) the bot today, pacing · server facts on [duels](server-duels.md),
[karma](server-karma.md), [casts](server-casts.md), [items](server-items.md) · [wire](wire.md)
packets, datapack, opponent record, states, code changes · [modes](modes.md) modes, preset, rows by preset, metrics ·
[targets](targets.md) `MayFight`, hunting, arena targets, summons · [guards](guards.md) flag
guard, arena edge, bystanders, duel support · [finding](finding.md) duels, arenas, duel ground ·
[layer](layer.md) PvP order, interrupts, CP, cures, break off · [model](model.md) land rates,
archetypes, PvP value, gear · [rows](rows.md) per-class rows ·
[second-profession rows](rows-second.md) · [party](party.md) party PvP ·
[karma](karma.md) death, PK mode, karma · [war](war.md) clan-war targets ·
[siege](siege.md) siege battlefields · [Olympiad](olympiad.md) noblesse, registration, matches ·
[CP potions](cp-potions.md) buying CP potions, Seven Signs.
