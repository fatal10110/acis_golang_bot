# PvP persona: bots that fight players (M13)

Status: draft for owner review; proposed decisions D35-D39 open. Milestone M13 ("PvP persona"):
epic and issues to be filed after the owner's answers.

## Paths

Server paths (`network/`, `duel/`, `model/`, `skill/`, `handler/`, `task/`, ...) are under
`internal/gameserver/` in [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang);
datapack paths (`classes/`, `skills/`, `items/`, `zones/`, `armorSets.xml`, `buyLists.xml`,
`multisell/`) under `aCis_datapack/data/xml`; config under `aCis_gameserver/config/*.properties`
(the Go server's shipped copies, `ops/docker/config/`, carry the same values; a missing key takes
the Go default, which can differ: `AwardPKKillPVPPoint` is `False` in `players.properties:144`,
`true` in `task/pvpflag.go:47`, `DefaultPvPFlagOptions`; this plan assumes the shipped files). Other
paths are this repository's, mostly `internal/botclient`. M10 is `docs/strategy.md` (epic #319,
#320-#333; build, `charmodel`, `Step` gates, "Behavior per class", presets `farmer`, `support`,
`merchant`, `crafter`, `town`; D21-D24, approved in open PR #346). M11 is [bot parties](../parties/README.md)
(proposed).

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

Out of scope: the Olympiad, sieges, clan wars as targets, party duels (M11), fighting summons for
their own sake ([summons](targets.md#summons-and-pets)), buying CP potions (Ancient Adena only),
second-profession PvP rows (first profession's row until M10's #332), kiting monsters.

## Approach

A PvP layer in the brain over each bot's M10 rotation: new wire, an opponent record, duel and arena
states, `MayFight` and guards against accidental karma, a PvP order per class row, a `charmodel`
PvP value blended into M10's gear score, a karma policy. No server change.

## Owner decisions (proposed)

- **D35, modes.** A new `pvp` preset (M10 Damage role, farms between fights): duels, flagged and
  karma players, arenas on; open-world PK off (opt-in per bot). Other presets keep fight back only
  with `SupportFlagged` off and now decline duels at once, but every preset fights players with the
  PvP layer (`support` bots play KN, CL, EO, SO; party PvP in any party). Persona defaults (D6).
  [modes](modes.md)
- **D36, finding fights.** Any duel it can take, clear of monsters, its duel moves kept off
  cancelling ground; challenges a player who whispers `duel` (extends D3 as the approved D20 did; once per 30
  minutes per player, a decline not counted); its own challenge of one non-bot player of its tier
  every 3 minutes when the engage estimate passes, not the same player within 30 minutes or 2 hours
  after a decline or refusal; hunts flagged and karma players of its tier on the estimate; the
  nearest arena every 90 minutes, staying while others are there (20 minutes at most).
  [finding](finding.md)
- **D37, items.** One loadout scored by M10's `Value` and `PvPValue` blended by relative gains,
  50 % each for `pvp` (others 0 %); no set swap; PvP sets and boss jewels worn when owned and
  within grade, never bought; poison and bleed cures (Antidote, Bandage, and their greater kinds
  from level 20) bought on M8 trips; CP potions kept, never sold or bought.
  [model](model.md)
- **D38, bots vs bots.** Outside an arena a bot never attacks, hunts or challenges a bot and fights
  one back only after its hit or offensive cast; in an arena anyone but its party. No bot duels.
  [targets](targets.md)
- **D39, death and karma.** Duels to the end, no surrender; arenas to the death, walking back after
  each of the first 4 deaths an hour, home after the 5th; open world, break off by the row, avoid
  its killer 30 minutes. No karma by accident ([guards](guards.md)), at a stated cost: in the three
  small arenas steps over about 3 s rarely start against an unrooted karma-free opponent. With a
  spiritshot the Elven and Human Mystics' 4000 ms nukes and drains (2.4-2.8 s) still start in the
  middle part, the Dark Mystic's (3.0 s) only at the very middle; Orc Mystic nukes, shot-less
  nukes, Drain Health, Power Shot and Stunning Shot do not (the Colosseum allows them). In a duel:
  no attack at all during the countdown; then no area skill or pole swing near a third player, a
  summon or an NPC (collision radii counted), no hit on an NPC, no support on a flagged player,
  each of which would cancel the duel. PK mode: one an hour, on the hunting preconditions, non-debuff damage until the victim
  flags, never at 4 PK kills. With karma: farm to 0, flee clean attackers unless a PK within limits
  (else take the death), walk on foot 600 from towns and guards, no gatekeeper, shop, hunt, duel,
  arena or town trip. [karma](karma.md)

## Delivery (milestone M13)

Dependencies name M10 issues by number. Every PR also reads [server](server.md) for its senders.

| PR | Scope | Depends on | Reads |
| --- | --- | --- | --- |
| X1 | Wire: duel packets, `RequestTargetCancel`, `MagicSkillCanceled` event, new `CharInfo`, `NPCInfo`, `UserInfo` fields, SystemMessage ids; codec tests. | S1 (#320, same `UserInfo` decoder) | [wire](wire.md#wire-x1) |
| X2 | Datapack: zone reader (`NPoly`, `Cuboid`; water on `LoadWaterZones`), skill landing, `nextActionAttack`, `offensive`, target and radius fields, cure negate fields, jewel stats, `pvp*Dmg`, `cpTable`. | S2 (#321) | [wire](wire.md#datapack-x2) |
| X3 | `charmodel` PvP: `LandRate`, cast-break chance, archetypes, class-to-row map, `RowTable` and reference opponents, `PvPValue`, engage estimate; tests on test-built tables; no row data. | S3 (#322), S11 (#330), X2 | [model](model.md) |
| X4 | World and policy: opponent record (fight kind, `unfought`), duel and arena states, duel result, `MayFight`, the guards (Esc, follow-up swing, arena edge, area and duel support, pole reposition and hold), duel ground and the follow stop, summons by own flag and karma, the five code changes, `PvPPolicy` modes, the `pvp` preset (D35, D38). | X1, X2, X3 (class and tier map), S4 (#323, `Step`), S6 (#325, rotation runner), S9 (#331) | [targets](targets.md), [guards](guards.md), [wire](wire.md#world-model-x4), [finding](finding.md#duel-ground), [modes](modes.md) |
| X5 | Finding fights: duel answer, whispered `duel`, own challenges and holds (unexplained 1940), clear of monsters, hunting with the estimate (empty `RowTable` until X6), arena trips, targets and leave (D36). | X3, X4 | [finding](finding.md), [targets](targets.md#arena-targets) |
| X6 | The PvP layer: `Step` fields, PvP order, long-cast and sleep rules, interrupts, land-rate control, kite, pool estimate, CP potions, cures, break-off and gate; `pvpRowTable` into X5's estimate; `TestPvPRowsShipped` with no row. | X3, X4, X5, S6 (#325), S11 (#330) | [layer](layer.md), [model](model.md#row-table) |
| X7 | Fighter PvP rows (HF, EF, DF, OF, WF, KN, WA, RO, EK, ES, PK, AS, OR, MO, SC) and their `TestPvPRowsShipped` cases. | X6, S6b (#326) | [rows](rows.md), [layer](layer.md) |
| X8 | Mystic PvP rows (HM, EM, DM, OM, CL, HW, EO, EW, SO, DW, OS) and their `TestPvPRowsShipped` cases. | X6, S7 (#327) | [rows](rows.md), [layer](layer.md) |
| X9 | Party PvP for every preset in any party: enemy set, healer marking, peel, healer buffs under its own `SupportFlagged`. | X6, X7 (KN peel), X8 (Mental Shield); `farmer` in bot parties needs M11 | [party](party.md) |
| X10 | PvP gear: mix table, `PvPValue` in gear, set, jewel and dye scores, shipped-row model tests, cures on the supply list, CP potion and cure keep rule (D37). | X3, X6, X7, X8, S5 (#324), M8 I3 (#170, supply list), I4 (#171, `keeps`) | [model](model.md) |
| X11 | Death and karma: arena return loop, avoid list, PK mode, karma state (flee or PK, chaotic restart, karma route, work-off) (D39). | X4, X5, X6, X7, X8 (`TestPvPPKCap` rows) | [karma](karma.md) |
| X12 | Panel switches, PvP counts, the four counters. | X4, X5, X6, X11, S9 (#331) | [modes](modes.md#panel-and-metrics-x12) |
| X13 | e2e live scenarios for X5-X11, at most 4 characters each, GM included. | each; M11 for `TestLivePvPHealer` | the e2e cases in each file's Tests |
| Backlog | Second-profession rows (with #332), party duels (with M11), the gaps below. | X7, X8 | [Risks and gaps](#risks-and-gaps) |

Most urgent: X1-X5 give a bot that takes duels and fights in arenas with its M10 rotation; X6-X8
make it fight like its class.

## Risks and gaps

Risks: **annoying players** (challenge pop-ups, bounded by the holds and tier match, switchable per
bot; per-bot holds let N bots each challenge one player per 30 minutes, one pop-up at a time);
**accidental PK** only from what the [guards](guards.md) cannot see (a `RelationChanged` or
`CharInfo` delayed past about 3.5 s, a move delayed past the arena margin, a Dash, Sprint or root
cure in a cast's last 200 ms); the **arena cost** (D39); **duels cancelled** with nothing restored
(a wandering monster, a pole swing meeting an NPC or summon, the opponent's own move, a siege
starting, a server follow leg bending into a polygon between checks, a position error past
`DuelEdge`; near a town edge a duel may time out instead, all restored); **blind HP** (errs toward
fighting on); **unknown level** (tier as proxy: a level-39 first profession will hunt a level-20 one, and the
engage estimate assumes equal level; hunting only flagged and karma players keeps this fair); **size** (X6 lands with no row); **arena deaths**
(the 5-death cap and stay limit).

To file in the M13 backlog: second-profession rows (#332); a Colosseum preference for the OM row,
shot-less mystics and bow rows, when a gatekeeper reaches it; party PvP with a `farmer` in a bot-led party (M11); party duels
(M11); a challenge hold shared by all bots; clan wars, sieges, the Olympiad; buying CP potions;
fighting summons for their own sake.

## Files

[server](server.md) facts, the bot today, pacing · [wire](wire.md) packets, datapack, opponent
record, states, code changes · [modes](modes.md) modes, preset, rows by preset, metrics ·
[targets](targets.md) `MayFight`, hunting, arena targets, summons · [guards](guards.md) flag
guard, arena edge, bystanders, duel support · [finding](finding.md) duels, arenas, duel ground ·
[layer](layer.md) PvP order, interrupts, CP, cures, break off · [model](model.md) land rates,
archetypes, PvP value, gear · [rows](rows.md) per-class rows · [party](party.md) party PvP ·
[karma](karma.md) death, PK mode, karma.
