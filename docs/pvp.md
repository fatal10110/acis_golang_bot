# PvP persona: bots that fight players

Status: draft for owner review. The review-fix loop ran 11 rounds; round 11's fixes are not
re-reviewed. Proposed decisions D35-D39 are open (see "Owner decisions").
Milestone M13 ("PvP persona"): epic and issues to be filed after the owner's answers ("Delivery").

Review history (each round's findings are checked against the server, the datapack or the bot code
before they are applied):

- Round 1 (10 findings, all confirmed and fixed): the no-karma guard now stops every damaging
  attack once the opponent's flag blinks and cancels the bot's own cast in flight with the Esc
  cancel (`RequestTargetCancel`), since a direct cast or swing, not a DOT, is what can kill a
  player whose flag ran out; while checking this, a player's DOT tick was found never to kill
  (`killByDOT`), so the DOT rule and its test were replaced (sections 3, 13, server table,
  Testing, Risks). PK mode uses only `AttackRequest` and ctrl-held non-debuff damage skills until
  the victim flags (sections 3, 13, D39, X11, `TestPvPPKCap`). Break steps no longer start a 10 s
  Sleep; long Break casts follow a break-cast gate (sections 9, 10). The `pvp` preset buys and
  uses Antidote, Greater Antidote, Bandage and Emergency Dressing against poison and bleed
  (sections 9, 12, D37, Pacing, Testing). `TestLivePvPHealer` waits for M11 bot parties (Testing,
  X9, X13). Pacing names the real frame bucket (5 a second, burst 10). Allies use the ally ids of
  `CharInfo` and `UserInfo`, and the peace check covers towns and the Derby track (sections 2, 3,
  X1, X2). Delivery dependencies match the scopes (S11 for X3 and X6, X5 and X6 for X12,
  `TestPvPRowsShipped` hosted in X6). The section 6 table is floored, as the golden test now
  states, and the land base names every case. The duel state clears on 153 and 1926, a duelling
  bot fights only its opponent, several bots challenging one player is a stated risk, and D36 no
  longer cites the proposed D28 as a precedent.
- Round 2 (10 findings, all confirmed and fixed): the section 6 table is recomputed at equal level
  with the level term (1.01 for `lvlDepend` 2, 1.005 for Stun Attack), and the golden test states
  it. Ultimate Defense, which locks its caster in place for 30 s, is cast only where the bot will
  not break off (section 9, KN, EK and PK rows, `TestPvPBreakOff`). The clan-mate rule needs a
  non-zero clan id. A new "Area hits and bystanders" rule keeps area damage and pole swings off
  flagged karma-free bystanders, and in a duel keeps area skills off third players and turns
  `SupportFlagged` off (sections 1, 3, 11, 13, D39, two new tests). `PvPValue` mixes PvP rows with
  their DOT and control steps, not archetypes (section 12, X3). Duels are checked against the peace,
  town, Derby, arena, water and no-restart zones, with a hold after 1940 (section 4, X2). Section 2
  names the three changes to today's code (`forced`, `hostileCaster`, `FlagBlinkAt` from
  `CharInfo`), in X4. Every brain test names its PR, mechanics are tested on test-built steps, and
  X9, X10 and X11 gained their missing dependencies (I3 #170, I4 #171 for X10). The base and healer
  mystic rows no longer list an Ice Bolt interrupt that cannot reach `MinBreak`, section 7 gives the
  real damage bounds, and the kite list keeps only caster and bow skills. The server table and
  section 12 add the 1.05 PvP damage Special Abilities of 186 C- to S-grade weapons.
- Round 3 (10 findings, all confirmed and fixed): `MayFight` now puts the duel opponent first
  (a clan mate or party member it duels one on one is fought, as the server allows) and the
  arena before the clan and ally rule (clan mates and allies fight there, party members never);
  the duel answer takes such challenges and declines bot-marked ones, and D38 says so (sections 3,
  4, D38, tests). The no-karma guard covers the server's follow-up swing after a
  `nextActionAttack` skill, Esc-cancelled ones included: none (Shield Stun and Bleed among them)
  is started after the blink, one in flight is cancelled and followed by a move (section 3, a new
  server row, Pacing, X2, X4, D39, Risks, `TestPvPNoKarmaFlagEnding`). A new "Sleep and damage"
  rule casts a sleep only on an opponent free of DOTs that nobody will hit, so the OM and OS
  break-offs fear instead after their own DOTs, and party PvP stuns rather than sleeps the healer
  it hits (sections 5, 9, 10, 11, tests). The Mithril Heavy over Brigandine claim and its golden
  test are dropped (Brigandine has more P.Def and HP; the test now checks only the `poisonVuln`
  term). X4 depends on X3 and X12 on X11. The Artisan line uses SC's row as an opponent. Hostile
  casts need an offensive skill. A whispered `duel` shares the bot's 3-minute challenge budget
  (section 4, Pacing, test). CP potions are not drunk in a duel, whose end restores the vitals;
  cures still are, with the reason (section 9). The `forced` citation names the cast line `:209`.
- Round 4 (9 findings, all confirmed and fixed): any hit between a duellist and an NPC, or a
  monster's hit on it, cancels the duel and a cancelled one-on-one duel restores nothing, so the
  bot hits no NPC and uses no area skill or pole swing with an NPC in reach during a duel, and
  takes and offers duels only clear of monsters (server table, sections 3, 4, D36, D39, Risks,
  `TestPvPDuelAccept`, `TestPvPDuelFight`, `TestPvPAreaBystander`). Every preset now fights a
  player with the PvP layer, so the KN, CL, EO and SO rows (Support role only in M10) are played
  by `support` bots fighting back, and party PvP names its hosts (`pvp` in a player's party,
  `farmer` and `support` in M11 bot parties) and heals under the healer's own `SupportFlagged`
  (sections 1, 11, D35, X9, `TestPvPTargetHealer`, `TestLivePvPHealer` moved to the open world).
  A new arena-edge rule keeps an arena opponent who steps out, now a clean player, from dying to
  the bot's hit, and the bot leaves an arena only with nothing in flight; the no-karma claim is
  scoped to what the bot can see (server table Kill credit, sections 2, 3, 4, 13, D39, Pacing,
  Risks, `TestPvPArenaAttackable`). `PvPValue` is defined as one number (the expected share of
  won fights, from time to die and time to kill per row) and blended with M10's `Value` by
  relative gains in M10's unit (section 12, D37, Testing). The arena return count is settled (walk
  back after deaths 1-4, home after the fifth). The own-challenge row names the 30-minute
  per-player hold. The poison rows are listed once (HM, DM, SO, DW, PK, AS, OM) and the Mithril
  test expects an equal score without them. Only an unexplained 1940 holds duels, until a
  2000 move or 10 minutes. The bystander test separates Wild Sweep's radius around the target
  from the pole's reach around the bot.
- Round 5 (8 findings, all confirmed and fixed): duellists attack each other without force only
  while both are Duelling, and a hit before that cancels the duel, so `MayFight` now allows nobody
  during the countdown and the opponent only from `ExDuelStart`, its first `ExDuelUpdateUserInfo`
  and the reaction delay; a countdown hit is not answered (server table, sections 2, 3, D39,
  `TestPvPDuelFight`). The arena-edge rule now subtracts the time a root the bot landed holds the
  opponent, and its cost is stated: in the three 1000 × 1000 arenas the 3-5 s nukes, drains,
  Ice Bolt, Power Shot and Drain Health almost never start against an unrooted opponent (new
  Arenas sizes, Run speed and Long hit times rows, section 3, D39, Risks, Gaps,
  `TestPvPArenaAttackable`, `TestLivePvPArena`). Reference opponents apply their steps' `Vs`
  gates, every poison step is gated to casters or healers, so the Mithril test expects a gain for
  a CL and none for a WA (section 12, Testing). The bystander reach adds both collision radii in
  3D as the server does, so the bot decodes them (server table, sections 2, 3, X1, D39,
  `TestPvPAreaBystander`). Section 11 runs for every preset in any party, a `support` bot in a
  player's party included (sections 1, 11, D35, X9, `TestPvPTargetHealer`, Gaps). A late Esc that
  clears the target is expected and answered by one re-selecting `Action` (server table, section 3,
  Pacing, Risks, `TestPvPNoKarmaFlagEnding`). The `forced` and `hostileCaster` citations are
  corrected, and the OF row no longer calls the orc the slowest runner.
- Round 6 (8 findings, all confirmed and fixed): the bot now decodes `MagicSkillCanceled` (0x49),
  which the guard's disengage and its late-Esc test read (section 2, X1, codec test). `charmodel`
  stays pure: it defines `Archetype` and a `RowTable` input, the brain's `pvpRowTable` (X6, filled
  by X7 and X8) feeds it, the seen-class mix table is specified and built in X10, which now
  depends on X7 and X8 and hosts the Mithril test over the shipped rows, while X3 tests on
  test-built tables (sections 8, 12, Testing, Delivery). A bot with karma has a stated policy:
  clean attackers stay clean, so it flees from them and fights one back only as a PK under the
  D39 limits, takes a death over a PK past them, restarts at the server's chaotic point outside
  town, and starts no hunt, duel, arena or town trip until karma is 0 (new server row, section 3
  rule 7, section 13, D39, Pacing, `TestPvPKarma`, X11). Player collision radii are 5 to 11 by
  race and sex, the reaches are stated as ranges and the bystander test fixes its radii, with an
  orc and a dwarf case (server table, section 3, Testing). Sting's abnormal level is read from
  `effectAbnormalLvl`, as the server's negate does, so cures choose by the lower set level
  (server table, section 9, X2, `TestPvPCure`). Stunning Shot (3000 ms, PDAM, a bow's low attack
  speed) is counted with Power Shot as a long step the small arenas hold, not a short one
  (server table, section 3, D39, Risks, Gaps). X4 depends on S6 (#325), the rotation runner its
  rules act on. `MaySupport` now refuses an unflagged karma player too (healing one flags the
  caster), and the duel support test covers it (Problem table, section 2, section 3, X4,
  `TestPvPDuelNoFlaggedSupport`).
- Round 7 (8 findings, all confirmed and fixed): a hostile cast is now an offensive skill aimed at
  the bot from any player, as every `Attack` hit already is, since the server lets an offensive
  skill reach the bot only from a player who may attack it; `MayFight` decides only fight or not.
  So a bot with karma flees a clean mystic's nukes, and a bot's nukes count as the first hit rule 5
  and D38 ask for; `unfought`, which today returns only blacklisted attackers, also returns a
  refused player attacker while the bot has karma (X11) (sections 2, 3, 13, D38,
  `TestPvPBotsOnlyInArena`, `TestPvPKarma` and `TestPvPDuelFight` with `MagicSkillUse` cases).
  Summons and pets are bystanders: area skills and pole swings hit them, a summon kill of a clean
  owner gives karma, and a duellist's hit on an outside summon cancels its duel; the bot reads a
  summon's owner flag and karma from its `NPCInfo` and `RelationChanged` (not "cannot see the
  owner", as the finding assumed, but not its party or clan), holds area damage while a summon
  showing a flag without karma is in reach, in an arena while a summon's owner (its `NPCInfo`
  title) is not seen inside, and in a duel while any summon is in reach (server table Area skills
  and Duel end, section 3, X1, D39, Risks, `TestPvPAreaBystander`). The OM, OS, RO, ES and AS
  interrupt cells follow section 7's timing rule with their own scaled hit times (Dreaming Spirit
  about 5.1 s, so almost never against a heal; Stunning Shot about 3.4 s with a bow). X6 passes
  `pvpRowTable` to X5's engage estimate and depends on X5 (sections 8, 12, Delivery). Killing a
  karma player is a PvP kill only with `AwardPKKillPVPPoint`, off in the shipped config, and the
  config sentence names where the Go code default differs (server table Kill credit, sections 3,
  14). Chant of Life falls under the long-cast rule (sections 5, 10). The root-ended cancel case
  of `TestPvPArenaAttackable` states the time left (a cancel at 2.5 s, none at 0.4 s). The
  `restoreDuelCondition` citation starts at `:553`.
- Round 8 (7 findings, all confirmed and fixed; one detail of the first corrected): a pole has no
  single-target step (M10 gates Power Smash and Stun Attack off it, and any `nextActionAttack`
  skill cast with a pole ends in a pole swing that hits the cone; 255 does carry a
  `weaponsAllowed`, unlike the finding said, which changes nothing), so a pole wielder with a
  bystander in reach drops its auto-attack with a move and walks to a stand point clear of the
  cone, or holds; the residual swing under way is stated (section 3, Pacing, Risks, D39, X4,
  `TestPvPAreaBystander`). Summons are no longer fought by the stale `NPCInfo` `Attackable`: a new
  "Summons and pets" rule fights the owner first and the summon only by its own current flag or
  karma, under the flag guard, never in a duel, with plain attacks only (Out of scope, sections
  2, 3, 11, 13, D39, Risks, X4, new `TestPvPSummon`). The zone reader takes the town file's
  `Cuboid` (Rune Castle Town) as well as `NPoly`, and water stays on today's `LoadWaterZones`
  (section 4, X2, Testing). The bot never supports its duel opponent (sections 2, 3, 11,
  `TestPvPDuelNoFlaggedSupport`). The engage estimate is a stated condition of own challenges
  (section 4, section 8, D36, `TestPvPChallengeCooldown`), and PK mode picks a victim on the
  hunting preconditions (sections 3, 8, 13, D39, `TestPvPPKCap`). The Problem table's "Fighting
  back" row now says every `Attack` hit counts and only casts are gated.
- Round 9 (6 findings, all confirmed and fixed): under its own Ultimate Defense the bot still
  sends the disengage move of the flag guard, the arena-edge stop, the arena leave and the pole
  drop, since the server answers a move from an immobilized player by idling it and dropping every
  intention (`startLiveMove`, `tryToIdle`, `goIdle`); only flee, kite, chase, reposition and the
  walk out of an arena wait for the effect to end (sections 3 and 9, `TestPvPBreakOff`). The
  `ArenaStay` leave case moved from `TestPvPArenaAttackable` (X4) to a new X5 test,
  `TestPvPArenaTarget`, since X5 adds the arena trips and their leave. A new "Arena targets" rule
  says whom the bot attacks unprompted in an arena (its class tier, its own HP and MP above
  `StandHP` and `StandMP`, at least `ArenaEdge` inside the polygon, the nearest, fight back
  first), with the arenas row, section 8 and X5 pointing to it and the same new test covering it.
  Pacing states the duel answer bound for both cases (an acceptance after the reaction delay, a
  decline at once, one answer per challenge). The X1 row puts the abnormal mask, max CP and CP
  and the duel team byte under `CharInfo`, not `UserInfo`. The `ruleAttacked` flee branch is cited
  at `rules.go:724-726`.
- Round 10 (5 findings, all confirmed and fixed): a stale first copy of section 3, with an older
  "Arena targets" wording and a misplaced copy of section 2's summons bullet, is deleted, so
  section 3 appears once. A new "Duel ground" rule (section 4) checks every move the bot sends in
  a duel (kite stand points, the pole reposition, disengage moves, monster clearance) against the
  ground that cancels a duel: `DuelEdge` (100) outside every peace, town, Derby track and arena
  polygon and every castle siege polygon it is not already in, and within `DuelLeash` (1800) of
  the opponent; the bot does not chase across such a polygon (sections 3, 5, D36, X2 now reads
  `SiegeZone.xml`, X4, X6, Risks, `TestPvPDuelFight`, `TestPvPKite`). The Ultimate Defense case
  of `TestPvPBreakOff` now says plainly that a KN able to outrun its opponent flees and one that
  cannot casts it. In an arena, area damage and pole swings are also held while a karma-free
  player other than the target is in reach and not at least `ArenaEdge` inside the polygon, since
  Wild Sweep anchored on the target reaches past the edge and the server fixes its hit set at the
  launch (section 3, D39, Risks, `TestPvPAreaBystander`). Section 14 names each counter once and
  which PR sets each fight kind (`duel` and `fight_back` X4, `arena` and `flagged` X5, `pk` X11),
  the death kinds, and the duel results the bot can tell apart (won, lost, tie: a cancel and a
  timeout send the same tie message); X12 and X4 match.
- Round 11 (7 findings, all confirmed and fixed): the cast time now includes the spiritshot
  factor (a magic skill's scaled hit time × 0.70 while a spiritshot or blessed spiritshot is
  charged, floored at 500 ms, `cast/controller.go:966-970`, `:993-999`), which the bots' mystics
  charge by auto-use. Section 7 predicts the bot's own step time with it while the server reports
  the auto-use on and a stack is carried; the arena-edge rule re-checks against the hit time of
  the bot's own `MagicSkillUse` and Esc-cancels when that is longer. The arena cost is restated:
  with a spiritshot the Elven and Human Mystics' 4000 ms nukes (2.4-2.8 s) start in the middle
  part of the small arenas, the Dark Mystic's (3.0 s) only at the middle, the Orc Mystic's
  (3.6 s) never. The OM and OS Dreaming Spirit cells now say about 3.6 s, so it can interrupt a
  Heal cast without a spiritshot. Server rows Cast time and Long hit times, sections 3, 5, 7 and
  10, D39, Risks, Gaps, `TestPvPArenaAttackable` (cast speed and shot state named, a late-hit-time
  cancel case) and `TestPvPInterruptTooLate` are updated. Duel ground: the server keeps following
  an opponent the bot attacks with no new packet (`ai/player.go:358-394`), so the bot checks the
  line to the opponent and its own follow legs while it has an intention and sends a checked
  disengage move before the follow could lead into a blocked polygon (section 4, Pacing, D36, X4,
  Risks, `TestPvPDuelFight`). With karma, the walk-back and the flee take a karma route: on foot,
  no gatekeeper or shop (both refuse a karma player under the shipped `players.properties:119-121`),
  `KarmaClear` (600) from town polygons and guard spawns, a nearer spot when its own has no such
  route (new server row Karma and NPC services, Guards row extended, section 13, D39, X11, Pacing,
  `TestPvPKarma`). Bot-code line citations are moved to HEAD after S4 (#323), and the siege
  polygons are named as 9 castles and 6 clan halls. The duel kite case moved from
  `TestPvPDuelFight` (X4) to `TestPvPKite` (X6). The opponent record carries `Kind`, X4 owns the
  duel result (1950, 1952, 1955), and deaths to an attacker `MayFight` refused count as
  `unfought` (section 14, X4, X12, `TestPvPDuelFight`). A whispered `duel` is never held by
  `DeclineHold`, and a decline or refusal of it starts that hold for the bot's own challenges
  (section 4, D36, Pacing, `TestPvPChallengeWhisper`).

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Datapack paths (`classes/...`,
`skills/...`, `items/...`, `zones/...`, `armorSets.xml`, `buyLists.xml`, `multisell/...`) refer to
`aCis_datapack/data/xml`. The server config files named are `aCis_gameserver/config/*.properties`;
the Go server reads the same keys, and its shipped copies (`ops/docker/config/*.properties`) carry
the same values. A key missing from a file falls back to the Go code's default, which can differ
from the shipped value: `AwardPKKillPVPPoint` is `False` in both `players.properties:144` but
`true` in `task/pvpflag.go:47` (`DefaultPvPFlagOptions`). This plan assumes the shipped files. Bot paths name this repository. M10 is
`docs/strategy.md` (epic #319, issues #320-#333): its build (race profile, class kit, role), the
`charmodel` package, the `Step` gates, the "Behavior per class" rows and the presets `farmer`,
`support`, `merchant`, `crafter` and `town` are assumed (D21-D24, approved in open PR #346). M11 is
`docs/parties.md` (bot parties, proposed).

The owner asked: "PVP - prepare a PVP persona that will fight with players." And: "PVP also needs
different kinds of items, and a fight strategy."

## Problem

Players fight each other three ways on this server: a duel (no death, nothing lost), a fight
between flagged or karma players or inside an arena (an arena death costs nothing; an open-world
one costs experience), and a player kill of a clean player (karma). A player looking for a fight
finds no bot that wants one: every bot only defends itself, plays its PvE rotation against a player,
and declines every duel.

### What the bot does today

| Concern | Today | Where |
| --- | --- | --- |
| Whom it fights | Attackable NPCs, and players who are flagged or have karma (`FightFlagged`); never a clean player (`ForceAttack` off); never heals or buffs a flagged player (`SupportFlagged` off), but the gate reads only the PvP flag (`SupportFlagged \|\| o.PvPFlag == 0`), so an unflagged karma player passes it, although healing one flags the caster as well ("Server behavior", PvP flag). A persona default (D6), not a safety rule. | `combat_rules.go` `PvPPolicy`, `MayFight`, `MaySupport` (`:71-73`); `rules.go` `DefaultPersona` (`PvP: PvPPolicy{FightFlagged: true}`) |
| Fighting back | Every `Attack` hit on the bot puts its attacker in the attackers table, with no check of the attacker (`applyAttack`, `:74-78`); a `MagicSkillUse` aimed at the bot does so only from a monster or summon showing `Attackable`, or from a player who is flagged or has karma (`hostileCaster`, `:107-120`), beneficial casts included. The bot fights the hardest hitter `MayFight` allows and drops a target that unflags. | `combat_world.go` `applyAttack`, `applyMagicSkillUse`, `hostileCaster`, `flagged`; `rules.go` `ruleAttacked` |
| Player relation | `RelationChanged` decoded: auto-attackable flag, karma, PvP flag, kept on the object. | `combat.go` `decodeRelationChanged`; `world.go` (`case RelationChanged`) |
| Forced attack | A clean player target gets `AttackRequest` (ctrl-click) and ctrl-held casts, sent only when `ForceAttack` lets `MayFight` pass. | `brainpackets.go` `encodeAttackRequest`, `encodeRequestMagicSkillUse`; `combat_rules.go` `fight`, `forced` |
| What it knows of another player | `CharInfo`: name, class id, PvP flag, karma, speeds, sitting, in combat, dead, store. Skipped: paperdoll, clan id, ally id, abnormal mask, max CP and CP, duel team. HP only from a `StatusUpdate`. | `inworld.go` `decodeCharInfo` (`r.Skip(5 * 4) // clan, crest, ally, ally crest, relation`); `world.go` `applyStatus` |
| Its own CP, clan and effects | CP decoded from `UserInfo` and `StatusUpdate`, used by nothing. Its clan id is kept; its ally id, PK and PvP kill counts are skipped. Its own effects (skill id and level) come from `AbnormalStatusUpdate`. | `world.go` (`Self.CP`, `applyClanID`), `inworld.go`; `support_test.go` (`AbnormalStatusUpdate`) |
| Duels | None: no duel packet is decoded or sent. | `opcodes.go` |
| Combat | One M10 build rotation for monsters and players alike: no control-first order, no interrupt, no read of the opponent's class. | `docs/strategy.md` "Behavior per class" (M10 S6, S6b, S7) |
| Potions | HP potions in `ruleCritical`; no CP potion. | `items.go` `healingPotions`, `drinkPotion` |
| Death | Accept a resurrection, else restart in the village (`restartToVillage`); the M8 death penalty scrolls. Same for every kind of death. | `death.go` `RecoverPolicy`; `rules.go` `ruleDead` |
| Whispers | GM commands (D3); the crafter's (D20); no `duel`. Names with the bot marker are never answered. | `chat.go` `ruleChat`; `command.go` |

## Goal

A `pvp` preset whose bots look for fair fights with players and fight them like a player who knows
its class:

- It takes and makes duel challenges, hunts flagged and karma players of its own class tier, visits
  the arenas, and fights back as today. Open-world player killing exists but is off (D35).
- It fights a player with a PvP order: control first, interrupts on the opponent's casts, a
  matchup by the opponent's class, CP management, and a break-off that knows where a death costs
  experience.
- It values gear for PvP too: survival, CP and HP pool, M.Def against casters, debuff resistance
  from jewels and sets, poison and bleed cures bought for fights, CP potions kept for them (D37).
- It does not get karma by accident (a flag that ends, an arena opponent who steps out, a
  bystander in an area hit), and its death and karma policy follows the server's rules for each
  kind of fight (D39).

Out of scope: the Olympiad, sieges, clan wars as a source of targets, party duels (they wait for
M11 bot parties), fighting servitors and pets for their own sake (the bot fights the owner, and a summon or pet
only under section 3's "Summons and pets" rule),
buying CP potions (their only shop takes Ancient Adena, "Gaps"), and PvP rows for second
professions (they play their first profession's PvP row, as M10 does for PvE, until the M10
backlog #332 adds their kits).

## Server behavior this rests on

Read in the Go server and the datapack on 2026-11-24; every design choice below rests on these.

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Duel challenge | `RequestDuelStart` (client 0xd0, sub 0x0027: name, party flag) is refused unless both players pass `duelRefusal`: not in combat or jail, HP and MP at least half, not already duelling, not in the Olympiad, no karma, no PvP flag, no store, not mounted, not fishing, not in a PvP, peace, siege, water or no-restart zone; and within 2000. The target gets `ExDuelAskStart` (0xfe, sub 0x004b: requester name, party flag) and SystemMessage 1938; the challenger 1927 or a refusal: 1926 (no player by that name, or itself), 1940 (the challenger cannot duel; an acceptor that cannot duel gets it too), 2017-2028 (the target cannot, or is too far), 153 (the target is busy with another request). | `network/duel.go:31-83`, `:86-93`, `:199-232`; `serverpackets/systemmessage.go:77`, `systemmessage_duel.go:5`, `:17`, `:36`; `duel/state.go:54-56`; `network/clientpackets/duel.go:11-38`; `network/serverpackets/duel.go:7-22`, `systemmessage_duel.go` |
| Duel answer | `RequestDuelAnswerStart` (0xd0, sub 0x0028: party, an unused int, accepted) takes the pending challenge; a decline sends the challenger 1931. A challenge stays open 15 s, and a player with an open request is "busy" to other challengers. | `network/duel.go:145-197`; `clientpackets/duel.go:41-58`; `trade/book.go:12-13`, `:342-345` |
| Duel run | A 5 s countdown (SystemMessage 1945 each second), then 1949, `ExDuelReady`, `ExDuelStart` and each opponent's `ExDuelUpdateUserInfo` (name, object id, class id, level, HP, max HP, MP, max MP, CP, max CP), resent when its CP or HP gauge moves. A duel lasts at most 2 minutes. | `duel/state.go:57-64`; `network/duel.go:313-332`, `:364-395`, `:614-625`; `serverpackets/duel.go:42-66` |
| Duel end | A hit that would kill leaves 1 HP and defeats the duellist. A duellist that gets a PvP flag (a hit on any karma-free player outside the duel flags it), enters a peace, siege or PvP zone or strays past 2000 cancels the duel. So does a duellist that hits an NPC, whatever the damage (a direct hit, or an area skill or a pole swing that also catches one), and one whose hit, direct or area, lands on a summon or pet whose owner is not in its duel (whatever the damage), and one that is hit by any creature outside its duel, a monster included, or by anyone before the fight starts: it is marked interrupted, and the duel's next check (once a second) cancels it. The end stops each side's cast and action and clears its target, shows the result (1950, 1952, 1955) and `ExDuelEnd`, then gives back the CP, HP, MP and effects saved at the start, except for a cancelled one-on-one duel, which gives back nothing. Acting on a defeated duellist answers 692 and `ActionFailed`. | `model/actor/player/character_stats.go:583-588`; `duel/duel.go:166-213` (`disturbed`: `Interrupted`, `p.PvPFlagged()`), `duel/state.go:59-61` (`TickPeriod`); `model/actor/creature/duel.go:34-41` (`InterruptDuelOnNPCHit`, called from `npc/hostile_formula.go:275`, `:340`, `npc/hostile_hp.go:95`, `npc/folk_hp.go:155`), `:43-50` (`InterruptDuelOnSummonHit`, called from `summon/formula.go:752`, `:807`); `model/actor/player/character_duel.go:110-126` (`duelHitAllowed`); `character_pvpflag.go:122-136`; `network/duel.go:433-480` (`:445`: a cancelled one-on-one duel is `abnormal`), `:543-548` (`stopToFight`), `:553-579`, `:629-653` |
| Duel opponent attackable | Players in the same active duel attack each other without force, and the plain `Action` click attacks, but only while both are in the Duelling state: each side enters it in its own start step, after `ExDuelReady` and `ExDuelStart`, and each opponent's `ExDuelUpdateUserInfo` is sent from that opponent's start step. During the countdown they need force, and any hit on a duellist not yet Duelling, its opponent's included, interrupts the duel (Duel end); such a hit raises no PvP flag (same duel id). The duel start sends no `RelationChanged` (the look refresh is `CharInfo` "with no relation update"), so the bot's auto-attackable flag for its opponent is stale. | `model/actor/player/character_social.go:200-203` (`inSameActiveDuel`); `duel/state.go:82-86` (`SameActive`: both `Duelling`); `network/duel.go:364-395` (`startDuel`), `:396-399` (`prepareToDuel` sets `Duelling`); `character_duel.go:114-124`; `character_pvpflag.go:127-129`; `network/player_follow.go:24-41`; `network/duel.go:405-411`; `network/inventory.go:917-925` |
| When `RelationChanged` is sent | When a player comes into sight, on a PvP flag change, a karma change and a siege change; not on entering an arena. | `network/relations.go:98-121`; `network/visibility.go:99-100`; `model/actor/player/character_pvpflag.go:17-27`; `character_karma.go:92-96`; `network/siege.go:378` |
| Attack without force | A player may be attacked without force when it has karma or a PvP flag, when both stand in a PvP zone, in the same duel, or both in an arena and not in the same party or command channel (a clan mate in an arena too). | `model/actor/player/character_attack.go:937-948`; `character_social.go:200-213` |
| Forcing | A ctrl-click (`AttackRequest`) attacks a player `AttackableBy` the attacker. A ctrl-held cast on a clean player not at war passes only for a damage skill (`IsDamage`: PDAM, MDAM, DRAIN, BLOW, ...); a skill with `isDebuff` is refused even with ctrl. | `network/player_follow.go:33`; `clientpackets/magic_skill_use.go:25-29`; `character_social.go:225-266`; `model/skill/definition.go:464-470`; `data/xml/skill.go:628` |
| Area skills | A player's area or aura skill affects only creatures it may attack without force; a clean player in the radius is never hit, but any player whose PvP flag is up or blinking, or who has karma, is (outside its party, clan and ally), and so is every attackable NPC, and so is every summon or pet (a playable, like a player) that it may attack without force: one whose owner has karma or a PvP flag, or one standing in a PvP zone with the caster in one too. A pole swing also hits up to `atkCountMax` other creatures in the 120° cone the attacker faces, within its own attack range (80 for a spear): any NPC it may attack, and other players and summons by the same rule as an area skill. A summon's `NPCInfo` (with `Summon` set) carries its owner's name as its title and its owner's PvP flag and karma, and the owner's flag and karma changes reach observers as the summon's own `RelationChanged` too; nothing shows the owner's party, clan or position. Killing the summon of a karma-free, unflagged owner gives the killer karma (`KarmaGain` at the summon rate, no PK kill), with the owner's flag, karma and zone read at the summon's death, so a summon in an arena whose owner stands outside it is such a kill. Both reaches are measured in 3D from the anchor (the target for an AREA skill, the caster for an AURA skill and the pole) and widened by the collision radii of the anchor and the creature hit: a creature is hit when its distance is at most reach + both radii. A player's radius is its class template's by race and sex, 5 to 11 (Human Fighter 9, female 8; Orc Fighter 11 and 7; Dwarven Fighter 9 and 5; Human Mystic 7.5 and 6.5; Orc Mystic 7 and 8; Dark Elf 7.5 and 7), sent in `CharInfo`; an NPC's is often larger. Each player hit counts as a PvP hit for the caster's flag (Duel end, PvP flag) and its death as a kill (Kill credit); each NPC hit takes a duellist out of its duel (Duel end). | `handler/target/target_predicates.go:22` (`isPlayable`: a player or its summon), `:31-46` (`isAttackable`, `areaCanAffect`); `model/actor/summon/target_actor.go:151-164` (a summon's `AttackableWithoutForceBy`: its owner's social rules, its own zone, its owner's karma and flag); `network/visibility.go:415-442` (a summon's `NPCInfo`: title, `PvpFlag`, `Karma`, `Team` from its owner); `network/relations.go:98-121` (the summon's `RelationChanged`); `character_karma.go:24-56` (`creditKill`: the owner's zone, flag and karma), `:84-87` (`AwardSummonKillKarma`); `skill/formulas/karma.go:21-23` (the summon rate); `handler/target/target_handlers_area.go:89-93` and `target.go:129-133` (`ForEachKnownInRadius`); `model/actor/player/character_runtime.go:576-580` and `model/door/doorhit/doorhit.go:50-51` (the pole's search, the same call); `world/visibility.go:759-761`, `:837-862` (`rangeBody3D` adds both `CollisionRadius`, then `<=`); `classes/humanFighter.xml:7` (`radius` 9, female 8), `orcFighter.xml:7` (11, 7), `dwarfFighter.xml:7` (9, 5), `humanMystic.xml:7` (7.5, 6.5), `orcMystic.xml:7` (7, 8), `darkFighter.xml:7` (7.5, 7); `serverpackets/charinfo.go:53`, `:107` (the radius sent); `model/actor/player/character_attack.go:937-948`; `model/actor/attack/controller.go:358-389` (`:367-375`: radius `PhysicalAttackRange`, `IsFacing` the attacker's heading); `items/0000-0099.xml` 15, 16 (`attack_range` 80); `character_pvpflag.go:65-95` |
| Arenas | Four arena zones (Dion, Gludin, Giran, the Colosseum), each an `NPoly` with `minZ`/`maxZ`. Dion, Gludin and Giran are about 1000 × 1000 squares, so no point inside is more than about 490 from an edge; the Colosseum is about 3450 × 2740 with notched sides, up to about 1370 from an edge near its middle. Inside, everyone fights freely; entering and leaving send SystemMessage 283 and 284. | `zones/ArenaZone.xml` (dion 11941..12943 × 183019..184010, gludin -88411..-87429 × 141732..142708, giran 72493..73493 × 142263..143261, colosseum 147769..151221 × 45344..48087, sides notched to 148048 and 150948); `model/zone/kinds.go:3-30`; `network/arena_zone.go:9-16`; `serverpackets/olympiad_match.go:37-38` |
| Run speed | A player's run speed is its template `runSpd` times the DEX bonus `floor(1.009^(DEX − 19.36) × 100 + 0.5) / 100`. The base classes run about 121 (Human Mystic), 124 (Orc Fighter), 125-127 (Dwarven Fighter, Dark, Elven and Orc Mystic, Human Fighter), 139 (Dark Fighter) and 144 (Elven Fighter) before buffs and Dash or Sprint. | `classes/*.xml` (`runSpd`, `dex`); `skill/statbonus/statbonus.go:37` (`dexCurve`), `buildCurve`; `skill/funcs/movespeed.go:10` |
| Long hit times | Template hit times of the rows' long damage steps: 4000 ms for 1177 Wind Strike, 1220 Blaze, 1175 Aqua Swirl, 1178 Twister, 1090 Life Drain, 1147 Vampiric Touch; 5000 for 1264 Solar Spark and 1266 Shadow Spark; 3200 for 56 Power Shot, 3100 for 1184 Ice Bolt, 3000 for 70 Drain Health and for 101 Stunning Shot; 2500 for 1201 Dryad Root. Power Shot and Stunning Shot are PDAM bow skills, scaled by the P.Atk speed, and a bow's is low (293 for the Bow 14, against 379 for a sword), so before DEX and buffs they take about 3.6 s and 3.4 s. A player's cast speed is 333 × the WIT bonus (`1.05^(WIT − 20)`), so with the base mystics' WIT 15-23 and no cast speed buff the 4000 ms skills take about 3.4-4.2 s scaled (5.1 s for an Orc Mystic). Those are the times without a spiritshot: a charged spiritshot or blessed spiritshot multiplies a magic skill's scaled time by 0.70 (Cast time), and the bots' mystics charge one on every cast while their auto-use is on, so with it the 4000 ms skills take about 2.4 s (Elven Mystic, WIT 23, cast 386), 2.8 s (Human Mystic, WIT 20, cast 333), 3.0 s (Dark Mystic, WIT 19, cast 316) and 3.6 s (Orc Mystic, WIT 15, cast 259-260), and the 5000 ms ones 3.0-4.5 s. Power Shot and Stunning Shot are not magic and keep their time; 70 Drain Health is magic, but its fighter rows charge soulshots, not spiritshots. | `skills/1100-1199.xml`, `skills/1200-1299.xml`, `skills/1000-1099.xml`, `skills/0000-0099.xml` (`hitTime`); `skill/formulas/speed.go:16-30`; `model/actor/player/character_attack.go:24`, `:376-385`; `skill/funcs/attack.go` (`MAtkSpeed`); `statbonus.go:38`, `:61-65` (`witCurve`, rounded to 0.01); `classes/*Mystic.xml` (`wit`: elven 23, human 20, dark 19, orc 15); `model/actor/cast/controller.go:966-967`, `:993-999` (spiritshot factor); bot `shots.go:127-134` (`shotKindsOf`: spiritshots for a mystic, soulshots otherwise), `:421-441` (`ruleShots`); `skills/0000-0099.xml` 70 (`isMagic` true); `skills/0100-0199.xml` 101 (`hitTime` 3000, `skillType` PDAM, `weaponsAllowed` BOW); `items/0000-0099.xml` 14 (`pAtkSpd` 293), 1 (379) |
| Peace flag | Three zone kinds raise the peace flag that refuses an attack (84, 85): peace zones, every town (a town is peaceful by default; only the town combat rule 2 lifts it) and the Derby track. | `zones/PeaceZone.xml`, `zones/TownZone.xml` (18 zones), `zones/DerbyTrackZone.xml`; `model/zone/kinds.go:45-50`, `:227-228`, `:360-367`, `:386`, `:394-406` |
| PvP flag | A hit (physical or offensive skill) on a karma-free player flags the attacker: 40 s (`PvPVsNormalTime`), or 20 s (`PvPVsPvPTime`) when the victim was already flagged and the attacker has no karma; no flag for a hit on a karma player, between duellists, or inside a PvP zone. A beneficial skill on a flagged or karma player outside a PvP zone flags the caster. Only the attacker is flagged: being hit never refreshes the victim's flag. The flag task ticks once a second and blinks the flag (state 2) once 5 s or less are left, so a blink is seen 4-5 s before the flag ends; a refreshed flag goes back to state 1 on the next tick. | `character_pvpflag.go:89-102`, `:122-136` (`notePvPHitFromAttacker` flags `pk`, the attacker); `task/pvpflag.go:14` (`PvPFlagTick`), `:19-26`, `:43-49`, `:125-132`, `:154-170`; `players.properties:147-150` |
| Kill credit | No credit in a duel or with both in a PvP zone at the death. Killing a flagged, karma-free player (killer karma-free) or a clan-war enemy is a PvP kill; so is killing a karma player, but only while `AwardPKKillPVPPoint` is on, and the shipped `players.properties:144` sets it to `False`, so under the shipped config the kill of a karma player is neither a PvP kill nor a PK (no count, no karma). Killing any other karma-free, unflagged player adds a PK kill and karma `KarmaGain(PKKills)` (240 for the first). The victim's karma and flag and both players' zones are read at death, so a direct hit that lands after the victim's flag ended is a PK, and so is one that lands on an arena opponent who has stepped out of the arena (arena hits raise no flag, so it is unflagged) or after the killer has stepped out. | `model/actor/player/character_karma.go:24-71` (`:29-34` the PvP-zone exemption, `:44-56`, `:60-71` `pvpKill`: `victimKarma > 0 && AwardPKKillPVPPoint`); `players.properties:144`; `character_pvpflag.go:131-133` (no flag inside a PvP zone); `attackable/playable.go:27-31` (`InArena`, read live); `skill/formulas/karma.go:9-25` |
| A DOT never kills | A damage-over-time tick that would kill leaves the target at 1 HP, unless its skill sets `killByDOT`, which only the NPC skills 4082 Poison of Death and 5086 Deadly Poison do. So a player's poison, bleed or burn cannot kill anyone. | `skill/effect/hooks_dot.go:5-30`, `:100-119`; `data/xml/skill.go:618`; `skills/4000-4099.xml:1201`, `skills/5000-5099.xml:1370` |
| Attack after a cast | When a cast of a skill with `nextActionAttack` ends, or is stopped (the Esc cancel included), on a target the caster may attack without force, the server makes that target the caster's attack intention: it swings at once when in range and free, else walks to it or queues the swing. A flagged player, blinking included, may be attacked without force. The player melee skills of the rows that carry it: 3, 16, 29, 92 Shield Stun, 96 Bleed, 100, 120, 223, 245, 255, 284. | `network/live_player_events.go:794-822`; `model/actor/ai/player.go:218-229`, `:384-394`; `character_social.go:257`; `skills/0000-0099.xml`, `skills/0100-0199.xml`, `skills/0200-0299.xml` (`nextActionAttack`) |
| Esc cancel | `RequestTargetCancel` (client 0x37, `unselect` 0) while casting stops the cast if it is still inside its interrupt window, which ends 200 ms before the hit lands; later in the cast it does nothing. Once the cast has ended on the server, the same packet clears the sender's target like a plain cancel: `ActionFailed`, then `TargetUnselected`; the attack intention stays. | `network/clientpackets/wiresafe.go:31`, `targetaction.go:64-78`; `network/targeting.go:898-915`, `:969-984` (`unselectLiveTarget`, `announceTargetCleared`); `model/actor/cast/controller.go:839-843`, `:973`, `:1012` |
| Death costs | In an arena, a death to a player or its summon costs no experience; in a siege zone only a Charm of Courage exempts. Elsewhere a death costs the level's normal loss, times `RateKarmaExpLost` with karma, a quarter at clan war or in a siege zone. | `character_death_exp_karma.go:11-24`, `:93-104`, `:119-130` |
| Death drops | No item drops for a player kill inside a PvP zone, nor for any karma-free player killed by a player. A karma player with at least the PK limit (`MinimumPKRequiredToDrop`, 5) rolls the karma drop rates whoever kills it. | `character_death_drop.go:24-68`; `players.properties:141` |
| A karma player | Anyone may attack a player with karma without force, and a hit on it raises no PvP flag on the attacker, so a clean player who attacks a karma player stays clean; a karma player who kills that clean, unflagged attacker gains a PK kill and more karma. A karma player's restart to the village lands at a random chaotic point (`chaoPoint`) of its nearest restart area, outside the town. Its karma falls with the experience it gains from kills and with the experience a death costs it (each through the karma-loss formula). | `character_attack.go:937-948`; `character_pvpflag.go:122-125` (`c.Karma() != 0` returns before flagging); `character_karma.go:44-56`; `network/restart_selection.go:101-111` (`townRestart`), `network/restartpoint.go:179-198` (`restartDestination`), `model/restart/restart.go:148-160` (`ChaoPoints` when karma > 0); `restartPointAreas.xml` (`chaoPoint`); `character_death_exp_karma.go:108-138` (`updateKarmaLoss` with the lost experience), `:186-212`; `data/manager/npcs_rewards.go:300` (`UpdateKarmaLoss` on a kill) |
| Guards | Guards (`Guard` NPCs, in and around towns) attack a karma player on sight, within an NPC's see range (450 by default). A death to them is a death with karma: the level's loss × `RateKarmaExpLost` (1 in the shipped `server.properties:134`). | `model/actor/npc/hostile_target.go:45-46`; `model/actor/npc/ai_config.go:14-22` (`SeeRange`, 450); `character_death_exp_karma.go:119-130` |
| Karma and NPC services | Under the shipped `players.properties:119-121` (`KarmaPlayerCanShop = False`, `KarmaPlayerCanTeleport = True`, `KarmaPlayerCanUseGK = False`), a gatekeeper, merchant, fisherman, manor manager, castle chamberlain or wyvern manager answers a player with karma with its refusal page (`<npcId>-pk.htm`) where that page exists, on the first chat and on every bypass command, instead of serving it: no gatekeeper teleport and no shop for a karma player. | `players.properties:119-121`; `network/folk.go:256-262` (`chatRules`); `model/actor/npc/folk_chat.go:86-88`, `:97-100`, `:118-119` (the `pk` gates), `:156` (the first chat), `:173-181` (`pkRefusal`); `model/actor/npc/folk_bypass.go:296-300`, `:389-393` (the gate on every command); `data/html/gatekeeper/*-pk.htm` (25 pages), `data/html/merchant/*-pk.htm` (149) |
| CP | Damage from a playable takes CP first, then HP. Only the player itself gets its CP in `StatusUpdate`. | `character_stats.go:574-588`; `network/targeting.go:1204-1220` |
| Another player's HP | Selecting a player sends one `StatusUpdate` with its max HP and HP; re-selecting the same target sends nothing, and no later update follows ("Other players see a player's health only through the one-off update sent when they select it"). A duel opponent's vitals come in `ExDuelUpdateUserInfo`, a party member's in the party window. | `network/targeting.go:856-891`, `:1204-1207`, `:1277-1296` |
| Damage feedback | Each skill or auto-attack hit sends the attacker SystemMessage 35 with the damage (and 44 or the magic critical first); the victim gets 36 with the attacker's name and the damage. A resisted effect sends the caster 139. | `network/magic_skill.go:1171-1200`; `network/live_player_events.go:442-447` |
| `CharInfo` | Carries the paperdoll template ids (right and left hand among them), the clan id, the abnormal mask (stun 0x40, sleep 0x80, root 0x200), the class id, max CP and CP, and the duel team; it is rebroadcast whenever the player's abnormal state changes. No level. | `serverpackets/charinfo.go:13`, `:63-66`, `:115-121`, `:146-153`; `model/skill/abnormal.go:9-13`; `skill/effect/list.go:325-327`; `network/live_player_events.go:264-267` |
| `UserInfo` counters | PK kills and PvP kills, after the karma. | `serverpackets/userinfo.go:212-213`, `:258-259` |
| Debuff land rate | `rate = base × stat × vuln × matk × level`, clamped to 1-99 % and kept as a float; the effect lands when an integer roll in [0, 100) is under it (a perfect shield block fails it). `base` is the first positive `effectPower` of the skill's effect templates, else the skill's own `effectPower` (per level), else 20 for a PDAM or MDAM skill, else the skill's `power` when it is in (0, 100], else 20. `stat` = `2 − sqrt(CON bonus)` for STUN, BLEED, POISON; `2 − sqrt(MEN bonus)` for magic SLEEP, DEBUFF, ROOT, FEAR, MUTE, PARALYZE and the like; else 1. `vuln` is the target's matching vulnerability stat (`stunVuln`, `sleepVuln`, `rootVuln`, `poisonVuln`, `bleedVuln`, `paralyzeVuln`, `debuffVuln`, `derangementVuln`, `cancelVuln`) over its elemental term. `matk` = `11 × sqrt(M.Atk) / M.Def` for a magic skill (M.Atk ×4 with a blessed spiritshot). `level` = `1 + 0.005 × Δ` (0.01 when Δ < 0), Δ = magic level (or caster level) + `lvlDepend` − target level, only when `lvlDepend` is set. No PvP-only term. | `skill/formulas/skillsuccess.go:26-52`; `model/actor/creature/formula.go:418-432`, `:534-613`; `model/skill/landing.go:5-48` (`LandingPower` at `:9-24`) |
| `addMul` | An `addMul` of `v` multiplies the stat by `1 − v/100`: a jewel's `stunVuln` 60 leaves 40 % of the stun rate. | `skill/effect/mod.go:158-159` |
| Cast break | Damage interrupts a magic cast with `15 + sqrt(13 × damage) − (MEN bonus × 100 − 100)` %, clamped 1-99, rolled on every hit. Stun and sleep abort the cast outright; root does not. | `skill/formulas/cast.go:9-30`; `model/actor/player/character_cast.go:80-97`; `skill/effect/hooks_cc.go:10-27` |
| Waking | Every hit or DOT tick ends sleep; a non-DOT hit ends a stun one time in ten. | `character_stats.go:856-875` |
| Cast time | A skill's hit time is scaled by `333 / speed` (cast speed for magic, attack speed otherwise, integer division). Then, for a magic skill (`isMagic`) cast while a spiritshot or blessed spiritshot is charged, the scaled hit time and cool time are multiplied by 0.70. Last, a skill whose template hit time is at least 500 ms is floored at 500 ms. A static hit time and the fusion and signet casts skip all three. The `MagicSkillUse` the bot sees, its own included, carries the final time. | `skill/formulas/speed.go:16-30`; `model/actor/cast/controller.go:966-970` (`spiritshotCastRate` 0.70, `minScaledHitTime` 500), `:985-1000` (`buildPlan`); `network/magic_skill.go:228` (`plan.HitTime` in the packet); bot `combat_world.go` `applyMagicSkillUse` |
| Taunt and confusion | Aggression's `TargetMe` works only on players: it makes the opponent target the caster. `RandomizeHate` (Confusion) works only on NPCs. Spoil skips anything not an NPC. | `skill/effect/hooks_buff.go:27-42`; `skill/effect/hooks_status.go:109-119`; `handler/skill/spoil.go:41-46` |
| Flood gates | No per-client gate covers duel requests; the per-connection frame limits do. | `network/floodprotector.go:6-24` |
| CP potions | CP Potion 5591 and Greater CP Potion 5592 cast skill 2166 (50 and 200 CP), item reuse 500 ms. Sold only by buy list 3001, whose `npcId` is -1 (no spawned merchant), and by multisell 500 for Ancient Adena at the Seven Signs priests. | `items/5500-5599.xml:655-677`; `skills/2100-2199.xml:669-681`; `buyLists.xml:10520`, `:10566-10567`; `multisell/500.xml:58-68`; `html/seven_signs/dawn_priest_2a.htm` |
| Cure items | Antidote 1831 and Greater Antidote 1832 (skills 2042, 2043) remove poison effects, Bandage 1833 and Emergency Dressing 1834 (2044, 2045) bleed effects, of abnormal level up to 3 (the plain ones) or 7 (the greater ones); item reuse 500 ms; reference prices 75 and 180. The plain ones are in 30 NPC buy lists, the greater ones in 23 (86-270 adena). A negate matches an effect by its skill type or its effect type, so it also removes Sting's bleed (a PDAM skill with `effectType` BLEED). The player DOTs here have abnormal levels 1-8 by skill level: Poison 129, Curse: Poison 1168, Venom 1095 (stack type poison), Bleed 96 and Sting 223 (bleeding). Four of them set the level in `abnormalLvl`; Sting sets it in `effectAbnormalLvl` and leaves `abnormalLvl` unset. Both default to -1. The negate removes an effect when either level is set and within `negateLvl`, `EffectAbnormalLevel` counting only for a skill with an `effectType`; an effect with neither set is never removed by a level-limited negate. | `items/1800-1899.xml:271-318`; `skills/2000-2099.xml:371` and the next three skills (`negateStats`, `negateLvl`); `buyLists.xml` (`<product id="1831"` 30 times, `1832` 23, `1833` 30, `1834` 23); `skill/effect/hooks_buff.go:180-240`; skills 129, 1168, 1095, 96 (`<set name="abnormalLvl" val="#abnormalLvl" />`) and 223 (`<set name="effectAbnormalLvl" val="#abnormalLvl" />`, no `abnormalLvl`; levels 3-8); `data/xml/skill.go:594`, `:646` (both default -1); `skill/effect/hooks_buff.go:221-233` (`negateLevelAllows`) |
| PvP jewels | Necklace of Valakas (6657, S: `sleepVuln` 80), Earring of Antharas (6656, S: bleed 80, stun 60, derangement 60), Zaken's Earring (6659, S: bleed 60, stun 40, derangement 40), Earring of Orfen (6661, A: bleed 40), Ring of Baium (6658, S: poison 80, root 60), Ring of Queen Ant (6660, B: poison 60, root 40), Ring of Core (6662, A: poison 40), Frintezza's Necklace (8191, A: sleep 30, poison 50, bleed 50, paralyze 30, stun 30). Every other jewel, no grade to S, carries only M.Def (and some max MP). | `items/6600-6699.xml:900-1005`, `items/8100-8199.xml:1548-1557`; `skills/3500-3599.xml:641-735`, `skills/3600-3699.xml:77-94` |
| PvP damage weapons | Special Abilities on 186 C- to S-grade weapons (C 26, B 14, A 102, S 44) multiply `pvpPhysDmg`, `pvpPhysSkillsDmg` and `pvpMagicalDmg` by 1.05 (for example 3068 Haste on 4809 Blood Tornado - Haste, 3552 PvP Damage, 3564-3602). The damage formulas multiply a hit on a player by the attacker's matching stat. | `skills/3000-3099.xml:531` (3045), `:857` (3068), `skills/3500-3599.xml:573` (3552) and 3564-3597, `skills/3600-3699.xml:36` (3602); `items/4800-4899.xml:232`, `:246`; `model/actor/creature/formula.go:148`, `:191`, `:298`, `:486-488` |
| PvP sets | Mithril Heavy (D: chest 58, legs 59, head 47; `poisonVuln` 20; its shield 628 adds 126 HP). Doom Light (B: poison 20). Tallum (A: poison and bleed 80), Dark Crystal (A: paralyze 50), Nightmare (A: sleep and root 70), Majestic (A: stun 50), Imperial Crusader (S: sleep and root 70; shield poison and bleed 80), Major Arcana (S: stun 50). | `armorSets.xml:5`, `:45`; `skills/3500-3599.xml:22-30`, `:492-500` and skills 3527-3556 |
| PvP self buffs | Lionheart 287 (WA, OR from 36; 60 s: cancel 40, stun, root, sleep, paralyze), Mental Shield 1035 (CL, EO, SO from 25: root, sleep, derangement), Deflect Arrow 112 (KN, EK, PK from 24: bow damage), Ultimate Defense 110 and Ultimate Evasion 111 (cancel 80). Ultimate Defense is an `ImobileBuff`: it locks the caster in place for its 30 s; Ultimate Evasion is a plain buff. | `skills/0200-0299.xml` 287, `skills/1000-1099.xml` 1035, `skills/0100-0199.xml` 110-112 (110: `<effect name="ImobileBuff" time="30">`); `skill/effect/core.go:84-85`, `:245`; `classes/*.xml` |

Nothing the design needs is missing on the server, and nothing here differs from the reference, so
this plan files no server issue. Two server facts shape the design and are not gaps: a bot sees
another player's HP once per selection, and no `RelationChanged` follows an arena entry or a duel
start (the reference server does the same).

## Design

### 1. Scope modes and the `pvp` preset (D35)

`PvPPolicy` (today `FightFlagged`, `ForceAttack`, `SupportFlagged`) gains the modes below. Each
mode is a persona default (D6), shown and switched per bot on the panel.

| Mode | What the bot does | `pvp` preset | Other presets |
| --- | --- | --- | --- |
| Fight back | Fights a flagged or karma player that hits it (today), with the PvP layer of sections 5-11 | on | on (today), with the PvP layer |
| Duels | Takes challenges it can take; challenges players (section 4) | on | off: declines at once (today a challenge expires unanswered after 15 s) |
| Flagged and karma | Attacks flagged and karma players in sight of its class tier (section 3) | on | off |
| Arenas | Visits an arena on a timer and fights there (section 4) | on | off |
| Open-world PK | Force-attacks clean players under the D39 limits | off (opt-in per bot) | off |

`pvp` is a new preset: the Damage role of M10 (its build, its branch weights, its gear), farming
like `farmer` between fights so it levels, with the PvP layer of sections 3-13 on top. Its
`SupportFlagged` is on, so in a party it keeps healing and buffing a flagged member (the server
flags it for that: "Server behavior", PvP flag), except during its own duel, where that flag would
cancel the duel (section 3, "Area hits and bystanders"). `support`, `farmer`, `merchant`, `crafter`
and `town` keep today's modes (fight back only, `SupportFlagged` off) and get no other mode.

The modes decide *when* a bot fights a player; the PvP layer decides *how*. Every preset fights a
player with it: the rules of section 3, the PvP order of sections 5-9, its own row of section 10
and, in any party it is in, section 11. This matters because the `pvp` preset is a Damage build, and M10's
Damage role never picks Knight, Cleric, Elven Oracle or Shillien Oracle (`docs/strategy.md`
section 6, "Professions by race and role": they are in the Support column only), while M11 bot
parties take only `farmer` and `support` bots (`docs/parties.md` "Eligible bots"). So the rows a
`pvp` bot plays are HF, EF, DF, OF, WF, HM, EM, DM, OM (base classes) and WA, RO, EK, ES, PK, AS,
OR, MO, SC, HW, EW, DW, OS; the KN, CL, EO and SO rows are played by `support` bots when they fight
back, alone, in a player's party or in an M11 bot party. Section 11 runs for every preset in
whatever party it is in, whoever leads it: a `support` bot in a player's party (today's main use:
it joins on invite and heals the leader, `docs/plan.md` "support"), a `pvp` bot in a player's
party, and, once M11 lands, `farmer` and `support` bots in a bot party. Nothing in section 11 reads
whether the leader is a player or a bot. A `support` healer keeps `SupportFlagged` off
(today's default, D6), so in the open world it stops healing a party member once that member is
flagged (by fighting back, for one) unless the owner switches it on for that bot (section 11, D35).
Section 12's PvP value and the arena, duel and hunting modes stay with the `pvp` preset.

### 2. Wire and world model

New decoders and encoders, each with a recorded or server-built fixture and an exact
`len(payload)` test (X1):

- `ExDuelAskStart`, `ExDuelReady`, `ExDuelStart`, `ExDuelEnd`, `ExDuelUpdateUserInfo`
  (0xfe sub-opcodes 0x004b-0x004f); `RequestDuelStart` and `RequestDuelAnswerStart` (0xd0 subs
  0x0027 and 0x0028). The bot never surrenders (D39), so `RequestDuelSurrender` is not encoded.
- `RequestTargetCancel` (0x37, `unselect` 0), the Esc cast cancel the no-karma guard sends
  (section 3).
- `MagicSkillCanceled` (0x49: the caster's object id, nothing else), which the server broadcasts to
  the caster and everyone watching it when a cast in flight is stopped, the Esc cancel included
  (`serverpackets/magic_skill.go:15-16`, `:78-82`, `FrameMagicSkillCanceled`;
  `network/magic_skill.go:765-779`, `broadcastCastAborted`; `network/targeting.go:898-906`). The
  bot does not decode it today (`opcodes.go` has no 0x49 entry). It becomes a typed event that
  ends the bot's own cast in the world model (`castEnd` cleared, so the step in flight is gone) and,
  for another caster, clears that opponent's `LastCast` (section 7). Section 3's guard and the
  arena-edge rule read it to tell a cancelled cast from a late Esc.
- `CharInfo` keeps the right- and left-hand template ids, the collision radius, the clan id, the
  ally id, the abnormal mask, max CP and CP, and the duel team byte; `NPCInfo` keeps its collision
  radius and its title (a summon's owner name, for section 3's arena summon rule); `UserInfo`
  keeps the bot's own collision radius, ally id, PK kills and PvP kills (all skipped or discarded
  today: `UserInfo` at `inworld.go:56`, `:62`, `:66`; `CharInfo` at `:130`, `:137`, `:140`, `:148`,
  `:150-151`; `NPCInfo` at `:171`, `:183`, and its title read at `:178` but not kept by
  `applyNPCInfo`, `world.go:888-900`). Section 3's area rule adds them to its
  reaches, as the server does.
- SystemMessages read by id: 35 (damage dealt, a number), 36 (damage taken, name and number), 139
  (resisted), 283 and 284 (arena in and out), 84 and 85 (peace zone refusals), 692 (frozen duel
  opponent), 153 (busy) and the duel set 1926-2028.

The world model keeps, per player in sight, an **opponent record** owned by the brain goroutine:

```go
// Opponent is what the bot knows of a player it may fight.
type Opponent struct {
	Archetype Archetype // melee, archer, caster, healer, battle caster (section 8)
	Tier      int       // 0 base class, 1 first profession, 2 second, 3 third
	ClanID    int32
	AllyID    int32
	Abnormal  int32     // CharInfo mask: stunned, asleep, rooted
	// FlagBlinkAt is when its PvP flag was last seen turn to state 2 (RelationChanged, or a
	// CharInfo carrying PvP flag 2, the first sight included); zero while the flag is steady or
	// gone (the no-karma guard, section 3).
	FlagBlinkAt time.Time
	// Pool estimate (section 9): HP and max HP from the selection StatusUpdate or the duel
	// window, CP and max CP from CharInfo or the duel window, minus damage dealt since.
	HP, MaxHP, CP, MaxCP int32
	SeenAt               time.Time
	// LastCast is the opponent's cast in flight: skill, target, when it lands (MagicSkillUse
	// hit time from its arrival).
	LastCast Cast
	// Kind is the fight kind section 14 counts by, set by the rule that allowed the fight
	// (duel, fight_back, arena, flagged, pk), or unfought for an attacker MayFight refuses.
	Kind FightKind
}
```

Two states the server never announces with `RelationChanged` are the bot's to compute:

- **The duel**: `idle → asked` (`ExDuelAskStart`, or a challenge it sent) `→ countdown` (1930 or
  1929) `→ fighting` (`ExDuelStart`, then the opponent's first `ExDuelUpdateUserInfo`, which
  carries its object id, then the PvP reaction delay) `→ idle` (`ExDuelEnd`, 1950-1956, 1931, any
  refusal: 153, 1926, 1940, 2017-2028, or 15 s with no answer). Only while fighting is the
  opponent attackable with a plain `Action`: the server lets duellists attack each other without
  force only while both are Duelling, which each side becomes in its own start step after
  `ExDuelStart`, and each opponent's `ExDuelUpdateUserInfo` is sent from that opponent's start
  step (`network/duel.go:364-395`, `:396-399`; `duel/state.go:82-86`). The bot's own start step is
  queued by the same loop, so the reaction delay (200-600 ms) covers the moment between the two.
  In the countdown the opponent needs force like any clean player, and `MayFight` allows nobody
  (section 3, rule 1).
- **The arena**: the bot is in an arena while its own position is in an arena polygon (datapack,
  X2), confirmed by 283 and 284. Another player is attackable without force while both stand in the
  same arena and it is not in the bot's party. The server reads both positions live, at every
  attack and at the death ("Server behavior", Kill credit), so the bot keeps, per arena opponent,
  its distance to the polygon's edge from its latest known position (`CharInfo`, its moves) and
  the bot's own; section 3's arena-edge rule reads them.

Five behaviors of today's code change with these states (X4):

- **Forced attacks.** `forced()` (`combat_rules.go:251-254`) makes the bot send ctrl-held casts
  (`:214`) and `AttackRequest` (`:228-230`) for every player who is neither flagged nor has karma.
  It becomes "a player the server would not let the bot attack without force": not its duel
  opponent in the fighting state (the countdown does not count), not a player in the same arena,
  and not one flagged or with karma. A duel opponent in the fighting state or an arena opponent
  gets the plain `Action` and plain casts ("Server behavior", Duel opponent attackable). In the
  countdown `MayFight` allows nobody (section 3, rule 1), so `forced()` is never asked then.
- **Hostile casts.** `hostileCaster` (`combat_world.go:115-120`) counts a player's cast at the bot
  only while that player is flagged or has karma, so an unflagged duel opponent's or arena player's
  casts never enter the attackers table; and it counts any `MagicSkillUse` aimed at the bot,
  beneficial ones included (`combat_world.go:107-113`). It becomes "an offensive skill (the
  datapack's `offensive` flag, X2, which the server derives from the skill type and `isDebuff`
  when the data omits it: `model/skill/definition.go:443-446`) whose `MagicSkillUse` names the
  bot as its target, from any player", with no test of the caster and none of `MayFight`. The
  server lets a player cast an offensive skill on the bot only when it may attack the bot: without
  force (the bot has karma or a PvP flag, both stand in an arena or another PvP zone, or the caster
  is its duel opponent in the fighting state: `character_attack.go:937-948`,
  `character_social.go:200-213`) or with a ctrl-held damage skill (`OffensiveCastAllowed`,
  `character_social.go:225-266`). So such a cast is an attack, as every physical hit already is
  (`applyAttack` records each `Attack` hit on the bot with no check, `combat_world.go:74-77`).
  Whether the bot then fights the caster or not is `MayFight`'s answer (section 3), not the
  table's: a clean mystic nuking a bot with karma enters the table and is fled from (section 13,
  "With karma"); a flagged bot's nukes are the first hit that rule 5 asks for; the duel opponent's
  cast in the countdown enters it and is not answered (rule 1). Gating the table on `MayFight`
  would lose both of the first two: the karma bot would neither flee nor fight a clean mystic,
  and a bot would never answer another bot's nukes outside an arena. A heal or buff from anyone,
  a player in an arena or a clean player while PK mode is on included, is not an attack; section
  11's hostile `MagicSkillUse` on a party member uses the same test with that member as the
  target. A summon's or pet's offensive cast at the bot enters the table the same way;
  whether the bot fights that summon is section 3's "Summons and pets" rule.
- **Blink seen on arrival.** `CharInfo` carries the PvP flag state 0, 1 or 2
  (`serverpackets/charinfo.go:81`, `:85`), and the bot already decodes it (`inworld.go:131`), but
  only `RelationChanged` would set `FlagBlinkAt`. A player first seen already blinking, or whose
  `CharInfo` refresh shows 2, sets it too, so the no-karma guard trips on it.
- **Support gate.** `MaySupport` (`combat_rules.go:71-73`) passes any player whose PvP flag is 0,
  karma players included, but a beneficial skill on a flagged *or karma* player outside a PvP zone
  flags the caster (`character_pvpflag.go:88-93`, `skillTargetFlagsPvP`). It becomes
  `SupportFlagged || (o.PvPFlag == 0 && o.Karma == 0)`, so `SupportFlagged` off means what it says
  (no support that flags the bot), and during the bot's own duel it is false whatever
  `SupportFlagged` says and refuses the duel opponent too (section 3, "Area hits and bystanders"). While the bot itself stands in a
  PvP zone (an arena) the server flags it for nothing (`skillTargetFlagsPvP` returns false for a
  caster in a PvP zone), so there the gate passes everyone.
- **Summons and pets.** A summon or pet is a `KindNPC` object (`NPCInfo` with `Summon` set,
  `world.go:888-900`), so `MayFight` and `hostileCaster` take it "as today", by `o.Attackable`
  (`combat_rules.go:60-68`, `combat_world.go:115-120`). That value is the `NPCInfo` snapshot of the
  server's `AttackableWithoutForceBy` for the bot (`network/visibility.go:441`;
  `summon/target_actor.go:151-164`: the owner's karma or PvP flag, or both in a PvP zone), and
  `RelationChanged` refreshes only the summon's karma and PvP flag, never its `Attackable`
  (`world.go:766-771`, `if o.Kind == KindPlayer`). So a summon that hit the bot while its owner was
  flagged still reads attackable after that flag has ended (being hit never refreshes the owner's
  flag, `character_pvpflag.go:122-136`), and its death to the bot's hit then gives the bot karma
  (`AwardSummonKillKarma`, `character_karma.go:84-87`, with the owner's flag and karma read at the
  death, `:44-56`). `MayFight` stops reading `o.Attackable` for a summon and applies section 3's
  "Summons and pets" rule, which reads the summon's own `PvPFlag` and `Karma` (kept current by its
  `RelationChanged`) instead; `hostileCaster` treats a summon's cast like a player's (above).
  Farming already skips summons (`farm.go:79`, `monster`).

### 3. Whom the bot may fight

`PvPPolicy.MayFight` becomes, for a player `o` (a monster or folk NPC is as today, except during a
duel: rule 1; a summon or pet follows "Summons and pets" below, never its `Attackable`):

1. from the duel countdown to its end, nobody but its duel opponent, and the opponent only in the
   fighting state (section 2: from `ExDuelStart` and the opponent's first `ExDuelUpdateUserInfo`,
   plus the reaction delay). During the countdown `MayFight` allows no player and no NPC, the
   opponent included: the server lets duellists attack each other without force only while both
   are Duelling (`duel/state.go:82-86`, `character_social.go:201`), and any hit on a duellist
   that is not yet Duelling interrupts the duel (`character_duel.go:114-124`), which the next 1 s
   check cancels with nothing given back. An opponent that hits the bot in the countdown is not
   answered: the bot holds off until the cancel reaches it (1952 and `ExDuelEnd`, as below), and
   since a hit between the two raises no PvP flag (same duel id, `character_pvpflag.go:127-129`),
   that player is then a clean player and the ordinary rules below apply. In the fighting state
   the opponent is fought whatever else they share: a clan mate or a party member it duels one on
   one is fought, since the server lets duellists attack each other before any party or clan
   check (`character_social.go:201`, `:230`), and the one-on-one challenge and answer check no
   party, clan or ally (`network/duel.go:31-62`, `:145-187`; only the party duel checks the
   party, `:102`). A hit on any other karma-free player would flag the bot, and a PvP flag cancels
   its own duel ("Server behavior", Duel end), so any other player, one that attacks it included,
   waits until the duel ends. No NPC either: the bot's hit on any NPC takes it out of its duel
   (`creature/duel.go:34-41`), so in that time it targets no NPC, and casts no area skill and
   swings no pole while an attackable NPC is in its reach ("Area hits and bystanders" below). A
   monster's hit on the bot cancels the duel too (`character_duel.go:121-124`), and a cancelled
   one-on-one duel gives back nothing. The bot still holds off that monster until the cancel
   reaches it (1952, the tie message a cancel shows, then `ExDuelEnd`, at the duel's next 1 s
   check: `network/duel.go:538`, `duelEndMessages`; `serverpackets/systemmessage_duel.go:24`),
   then fights it as today and rests after. Section 4 keeps monsters out of its duels in the first
   place;
2. never a party member, in an arena too (the server's arena rule excludes the same party and
   command channel, `character_social.go:204-206`, `:233-235`);
3. both in the same arena: anyone else there, clan mates, allies and bots included (D38), as the
   server allows (`character_social.go:206`, `:235`);
4. never a clan mate (`CharInfo` clan id equal to the bot's own non-zero clan id: two clanless
   players both carry 0 and are not clan mates, as the server's `IsInSameClan` requires a clan,
   `character_social.go:120-122`, `charinfo.go:121`) or an ally (`CharInfo` ally id equal to the
   bot's own non-zero ally id from `UserInfo`);
5. never a bot (nickname marker), unless it hit the bot first: an `Attack` hit or a hostile cast
   (section 2) that put it in the attackers table (D38; rule 3 already covered arenas);
6. `FightFlagged` and `o` flagged or with karma (today's rule);
7. open-world PK on and the D39 limits pass (this includes a clean player who attacks the bot while
   it has karma: killing that player is a PK, so it is fought back only under the same limits and
   otherwise fled from, section 13, "With karma");
8. else no.

"Hunting" (attacking a player that has not touched the bot) adds, for rule 6: `o`'s class tier
equals the bot's (a level 1-19 base class does not hunt a second profession, nor the reverse,
since `CharInfo` carries no level and the tier is the only level the bot can read), the bot's own
HP and MP are above its `StandHP` and `StandMP`, neither it nor `o` stands where the peace flag is
raised (peace zones, towns and the Derby track: datapack, X2; the server refuses with 84 or 85
anyway, `model/zone/kinds.go:394-406`), and the engage estimate (section 8) passes. The same four
preconditions apply to rule 7 when PK mode picks a victim (a clean player that has not touched
the bot), on top of the D39 limits: no PK starts at low HP or MP, from or into a town or peace
zone, or against an opponent the estimate says the bot loses to. Fighting back needs none of
those, a fight back as a PK while the bot has karma (rule 7, section 13) included.

**Arena targets.** Rule 3 says whom the bot may fight in an arena; this says whom it attacks
unprompted there (X5). An arena fight costs nothing, so no engage estimate applies (section 8),
but the other hunting preconditions do. With nobody in its attackers table, the bot picks a new
target only when its own HP and MP are above its `StandHP` and `StandMP` (else it rests as today,
inside the polygon at least `ArenaEdge` from its edge), and only among the players rule 3 allows
that are seen inside the same arena polygon, whose class tier equals the bot's (so a level-39 bot
leaves a level-1 base class standing inside alone; `CharInfo` carries no level, as for hunting),
and that are at least `ArenaEdge` inside the polygon (one nearer the edge could take no damage
step anyway: the arena-edge rule below). Among those it takes the nearest. It keeps that target until the target dies, leaves the arena or
falls out of the tier; a player who hits it meanwhile goes into the attackers table and is fought
back as today (`ruleAttacked`), which always comes before a new pick. A player who just walked in,
idle or not, is a target like any other once inside by the margin: entering an arena is the
consent the server reads (rule 3).

**Summons and pets.** The bot fights the owner, not its summon: a summon or pet in the attackers
table puts its owner, the player in sight whose `CharInfo` name is the summon's `NPCInfo` title,
in the table with it, and the bot fights that owner by rules 1-8 first (a summon's hit on the bot
flags its owner unless the bot has karma or both stand in a PvP zone, `character_pvpflag.go:52-62`,
`:122-136`, so rule 6 usually allows it).
The summon itself is the bot's target only when its owner is not in sight or `MayFight` refuses
the owner, and then only:

- outside a duel (rule 1: the bot hits no NPC, a summon included, in its duel), never as a hunting
  or PK-mode target, and never the summon of a party member (title equal to a member's name),
  clan mate or ally (the owner's `CharInfo` ids, when the owner is in sight);
- while the summon's own `Karma` is above 0 or its own `PvPFlag` is 1, as `RelationChanged` last
  showed them; at flag 2 the flag guard below applies to the summon exactly as to a player
  (no new direct-damage or `nextActionAttack` step, its own such cast in flight Esc-cancelled,
  its auto-attack dropped with a move), and at flag 0 and karma 0 it is not fought, whatever its
  `NPCInfo` `Attackable` said;
- in an arena, only while its owner is seen inside the polygon by at least `ArenaEdge`, and the
  arena-edge rule below is read on the owner's distance to the edge and run speed, since the
  server credits the summon's death by its owner's zone ("Server behavior", Area skills);
- with plain `Action` and plain casts only, never `AttackRequest` or a ctrl-held cast, so the
  server's own social check still refuses a summon whose owner the bot could not place.

A summon the bot may not fight is held off like any attacker the policy refuses: neither
`attackers` nor `unfought` returns it (`combat_rules.go:88-120`), so the bot does not answer
it. The same rule holds for a summon that a party leader attacks (section 11).

**No accidental karma.** A karma-free opponent whose PvP flag has ended dies to the bot's direct
hit as a PK, and the bot's own hits never refresh that flag, so a fleeing flagged player runs out
of it while the bot is still casting at it. A DOT cannot do this (a player's DOT tick never kills:
"Server behavior"); a direct-damage cast or a swing can. So, outside duels, arenas and PvP zones,
against a flagged karma-free opponent (the flag guard):

- the moment its flag blinks (state 2, which the server shows 4-5 s before the flag ends), the bot
  starts no new skill with direct damage (PDAM, MDAM, BLOW, DRAIN, CHARGEDAM and the other
  `IsDamage` types), no new skill that carries `nextActionAttack`, and no new swing: a move drops
  its auto-attack (today's `disengage`), and a cast of its own still in flight on that opponent
  (its own `MagicSkillUse`, not yet landed) that has direct damage or carries `nextActionAttack`
  is cancelled with `RequestTargetCancel` (`unselect` 0, the Esc cancel). The server honours the
  cancel until 200 ms before the hit lands, so a cast it can no longer stop lands within about
  200 ms plus latency of the blink, and a swing already under way within one of the bot's attack
  intervals: both well before the flag ends. The bot times the window from its own
  `MagicSkillUse` arrival, so an Esc sent near the window's end can reach the server after the
  cast has ended; the server then clears the bot's target instead (`ActionFailed`, then
  `TargetUnselected`, which sets the world's `Target` to 0: `network/targeting.go:898-915`; bot
  `world.go`, `case TargetUnselected`). The bot expects that: a `TargetUnselected` after its Esc
  is not a lost opponent, and before its next step on that opponent (a DOT, a root) it selects it
  again with one `Action`, which on a player that is not the current target only selects it (and
  brings a fresh `StatusUpdate` for the pool estimate, section 9). It never sends a second
  `Action` on it while the guard holds, since that click would attack. The attack intention the
  late Esc leaves in place is dropped by the move below;
- **the follow-up swing.** When a cast of a skill with `nextActionAttack` ends on a player the
  server lets the bot attack without force (a blinking one included), the server makes that player
  the bot's attack intention and swings at it, or walks to it; an Esc-cancelled cast does the same
  (`network/live_player_events.go:794-822`, `endCastIntention`, `attackAfterCast`;
  `model/actor/ai/player.go:218-229`, `:384-394`; `character_social.go:257`). The rows' melee
  skills carry it: 3 Power Strike, 16 Mortal Blow, 29 Iron Punch, 92 Shield Stun, 96 Bleed,
  100 Stun Attack, 120 Stunning Fist, 223 Sting, 245 Wild Sweep, 255 Power Smash, 284 Hurricane
  Assault (`skills/0000-0099.xml`, `0100-0199.xml`, `0200-0299.xml`, `nextActionAttack`). So
  after the blink none of them is started on that opponent, Shield Stun and Bleed included,
  although neither deals direct damage; and for one already in flight at the blink, the bot
  sends its disengage move as its first send after the cast ends (its `MagicSkillCanceled` after
  the Esc cancel, or the cast's landing time), which drops that attack intention. A follow-up
  swing the server started before the move arrives lands within one of the bot's attack intervals
  of the cast's end, before the flag ends, like any swing under way;
- skills with no direct damage and no `nextActionAttack` (a pure DOT, root, sleep or other
  debuff: Poison 129, Curse: Poison 1168, Dryad Root 1201, Wind Shackle 1206, Power Break 115) may
  still be cast, since none of them can kill or start a swing; Ice Bolt 1184 and Freezing Strike
  105 are MDAM skills and stop at the blink;
- when the flag goes back to state 1 (the opponent hit a karma-free player again, which refreshes
  it), the bot attacks again;
- once the flag is gone, no attack at all (today's `disengage`).

**The arena edge.** Inside an arena the kill of a karma-free player is free only while both stand
in a PvP zone at the death, and arena hits raise no flag (`character_karma.go:29-34`,
`character_pvpflag.go:131-133`). So an arena opponent who steps out of the polygon is a clean,
unflagged player (unless it still carries a flag from outside, which the flag guard then covers),
and a damaging hit of the bot's that kills it there is a PK; the same holds when the bot itself
has stepped out. No blink warns of this, so the bot predicts it. Against a karma-free arena
opponent (the flag guard's step classes: direct damage, `nextActionAttack`, swings):

- such a step starts only while the opponent's distance to the arena polygon's edge (section 2)
  is more than its run speed (`CharInfo`) × (its **free time** + `EdgeSlack`, 0.5 s of latency).
  The free time is the time until the step lands (the step's scaled hit time as section 7
  predicts it, the spiritshot factor included, or the bot's attack interval for a swing; once the
  bot's own `MagicSkillUse` for the step arrives, the hit time that packet carries) minus the time the opponent is still **held** by a root the bot itself
  landed on it, floored at 0. A root counts as held from the bot's own root cast landing with no
  139 and the root bit (0x200) in the opponent's `CharInfo` mask, for the root's datapack
  duration from that landing (Dryad Root 1201, Seal of Binding 1208); a `CharInfo` that clears the
  bit (a cure, a cleanse) ends the hold at once. A stun or sleep does not count: a hit ends a stun
  one time in ten and wakes a sleeper ("Server behavior", Waking), and the arena's other players
  may hit it. So an opponent running straight out cannot be outside when the step lands. The
  bot's own distance to the edge must be more than `ArenaEdge` (150);
- **what this costs.** An unheld opponent runs 121-144 ("Server behavior", Run speed), and no point
  of the Dion, Gludin or Giran arena is more than about 490 from an edge, so there a step starts
  only when its scaled hit time is under about 2.9-3.5 s at the arena's middle (490 / run speed −
  0.5) and less anywhere else. With a spiritshot charged ("Server behavior", Cast time and Long hit
  times), the 4000 ms nukes and drains (Wind Strike, Blaze, Aqua Swirl, Twister, Life Drain,
  Vampiric Touch) take about 2.4 s for an Elven Mystic, 2.8 s for a Human Mystic, 3.0 s for a Dark
  Mystic and 3.6 s for an Orc Mystic. So an Elven or Human Mystic starts them against any base
  runner in the middle part of a small arena (Human Mystic: from 400-475 from the edge, 121-144 ×
  (2.8 + 0.5); Elven Mystic: from 350-420, 121-144 × (2.4 + 0.5)), a Dark Mystic only at the middle and not against the
  fastest runners (it needs 420-500), and an Orc Mystic never (it needs 495-590). Ice Bolt (3100
  ms, about 2.2 s for a Human Mystic with a spiritshot) starts there too. The 5000 ms Solar Spark
  and Shadow Spark (3.0-4.5 s with a spiritshot) start only for the fastest casters at the very
  middle. Without a spiritshot (auto-use off or no shot carried) every 4-5 s step is 3.4-6.4 s and
  almost never starts. Drain Health (a fighter's magic skill, no spiritshot) and the two bow
  skills, Power Shot (3200 ms) and Stunning Shot (3000 ms), both scaled by a bow's low attack
  speed to about 3.4-3.6 s and never shortened by a shot, almost never start there against a
  karma-free opponent unless the bot rooted it first. The bow rows and the Orc Mystic therefore
  fight a square arena with roots (CL, EO, SO, OS), DOTs and other non-damage control, their short
  steps (Energy Bolt 2 s, Aura Burn 1.5 s), their plain bow shots (one attack interval each) and
  melee. The bow rows' Stunning Shot `Hold` and `Int` steps (RO, ES, AS) are damage steps too, so
  they wait with Power Shot. The Orc Mystic (and any mystic out of spiritshots) does little more
  than its DOTs and melee there; the other base mystics keep their nukes near the middle and lose
  them near the edges. The Colosseum (up to about 1370 from an edge,
  room for about 9 s) lets every step start near its middle. This is a stated limit for the owner
  (D39, "Risks"); a preference for the Colosseum for those rows is a backlog item ("Gaps");
- when the opponent comes nearer the edge than its run speed × (the bot's attack interval +
  `EdgeSlack`), or is seen outside, or a root the bot counted on ends while its own step is in
  flight and the opponent's distance to the edge is now under its run speed × (the time until
  that step lands + `EdgeSlack`), or the bot's own `MagicSkillUse` for the step carries a longer
  hit time than it predicted (no spiritshot was charged) and the opponent's distance to the edge
  is under its run speed × (the time left by that hit time + `EdgeSlack`), the bot acts as at a
  blink: it starts no such step, Esc-cancels its own such cast in flight (`RequestTargetCancel`,
  `unselect` 0, inside the cancel window), and drops its auto-attack and the follow-up swing with
  a move to a point inside the polygon at least `ArenaEdge` from its edge; it attacks again once
  the opponent is back inside by that margin. Pure DOTs and non-damage control still go: a DOT
  never kills, and a root holds the opponent inside;
- the bot's own moves in an arena (chasing a target, kiting, its stand points) stay at least
  `ArenaEdge` inside the polygon. To leave (`ArenaAlone`, `ArenaStay`, `ArenaDeaths`), it first
  stops as at a blink: no new step, its own damage or `nextActionAttack` cast Esc-cancelled or,
  past the window, landed, its auto-attack dropped with a move inside the polygon, and one of its
  attack intervals waited out after that move (a swing the server had under way lands inside);
  only then does it walk out.

What is left is an opponent that speeds up after the step started (a Dash or a Sprint), or whose
root the bot counted on is removed, and reaches the edge while the bot's cast is past its cancel
window, its last 200 ms ("Risks").

Under the bot's own Ultimate Defense, which locks it in place, every disengage move above (the
guard's, the edge stop's and the leave's stop move) is still sent: the server answers it by
dropping the attack intention without moving the bot; only the walks wait for the effect to end
(section 9).

Against a karma player any step goes: its kill is never a PK (a PvP kill only while
`AwardPKKillPVPPoint` is on, which the shipped config leaves off: "Server behavior", Kill credit). In PK mode (section 13) the guard is
off for the one victim the D39 limits allow.

**Area hits and bystanders.** The rules and the guard above pick whom the bot aims at, but an area
skill or a pole swing also hits every other player in its reach that the server lets it attack
without force: anyone flagged (blinking included) or with karma, outside the bot's party, clan and
ally ("Server behavior", Area skills). So:

- outside duels and arenas, the bot casts no area or aura skill with direct damage
  (Wild Sweep 245, target AREA, PDAM, `skills/0200-0299.xml`; any other `IsDamage` skill with an
  AREA, AURA or cone target) and a pole wielder does not swing, while a flagged karma-free player
  other than its target, or a summon or pet (`NPCInfo` with `Summon` set) showing a PvP flag and
  no karma, is **in reach**. The summon shows its owner's flag and karma, kept current by its own
  `RelationChanged` ("Server behavior", Area skills), and its death to the bot's hit after that
  flag ends gives the bot karma just as a player's would (`AwardSummonKillKarma`,
  `character_karma.go:84-87`). The bot does not link a summon to its owner, so its target's own
  summon counts too, and so does a flagged party or clan mate's summon, which the server would not
  hit: a summoner opponent next to its summon is fought with single-target steps. The reach is the skill's radius around its anchor (the
  target for AREA, the bot for AURA) or, for a pole, the bot's own attack range (80 for a spear)
  around the bot inside the 120° cone it faces (`attack/controller.go:367-375`); and, as the server
  measures it, a creature is in reach when its 3D distance from the anchor is at most that reach
  plus the anchor's collision radius plus its own (`world/visibility.go:837-862`, `rangeBody3D`;
  radii from `CharInfo`, `NPCInfo` and `UserInfo`, section 2). Player radii run from 5 to 11 by
  race and sex ("Server behavior", Area skills), so Wild Sweep (`skillRadius` 150) reaches a player
  160-172 from its target (168 between two male humans), and a spear 90-102 from the bot (98
  between two male humans), further for a large NPC. The bot always adds the two radii it decoded,
  never a fixed figure. That bystander's flag may end before the hit and its death would be a PK
  (`character_karma.go:45-55`). The two reaches differ: with a male human target and bystander, a
  bystander 165 from the target on its far side is in Wild Sweep's reach (150 + 9 + 9 around the
  target, `castRange` 40) but out of the pole's, so it blocks Wild Sweep and not the swing.
  What the bot does instead depends on its weapon. With any weapon but a pole it uses its
  single-target steps (the WA and OR rows' 255 Power Smash and 100 Stun Attack, which M10 gates to
  SWORD, BLUNT, BIGBLUNT, BIGSWORD and to BLUNT, BIGBLUNT, `docs/strategy.md:293`, `:299`; 255
  carries the same `weaponsAllowed` in `skills/0200-0299.xml`; the WA's Warlord branch wields the
  pole, `docs/strategy.md:262`). **A pole has no single-target step.** Its wielder's damage steps are Wild Sweep and the swing itself, and any skill with
  `nextActionAttack` cast with a pole ends in a pole swing at the cast's target once the cast ends
  (`attackAfterCast`, `network/live_player_events.go:794-822`), which hits the same cone; the one
  skill that narrows a pole to one target, 317 Focus Attack (`PolearmTargetSingle`,
  `skills/0300-0399.xml:508`; `character_attack.go:620-627`), is learned at 40
  (`classes/humanFighter.xml:1289`), past the first-profession rows. So with a pole equipped a
  `nextActionAttack` step counts as a swing, and the moment a bystander is in the pole's reach
  the bot sends its disengage move (today's `disengage`, `combat_rules.go:324`: a move replaces
  the attack intention, so the server swings no more after the swing under way), then
  **repositions**: it walks to a stand point within its attack range of the target from which no
  such bystander is in the pole's reach inside the 120° cone facing the target (the server turns
  the attacker to its target before each swing, `attack/controller.go:334`, so the cone is centred
  on the line to it), trying points around the target in 30° steps from the one nearest the bot,
  each holding on the geodata as `disengage`'s step does (and, in a duel, passing section 4's
  duel-ground check), and attacks again from there. With no
  such point (bystanders on both sides of the target), it **holds**: no swing and no damage or
  `nextActionAttack` step on that target, only steps with neither (its self buffs), until the
  bystander leaves the reach of some stand point. A swing already under way when a bystander
  steps into the cone lands within one of the bot's attack intervals; a flagged bystander's flag
  cannot end in that time, since the server shows the blink 4-5 s before the end, so that hit is
  no PK;
- in an arena, the same holds for every karma-free player other than its target who is in reach
  and does not stand at least `ArenaEdge` inside the arena polygon, flagged or not. The bot keeps
  `ArenaEdge` (150) inside, but its target may stand anywhere in the arena, and Wild Sweep anchored
  on it reaches 160-172 past it, so across the edge. A flagged player outside the polygon is hit
  and flags the bot, since only a hit with both in a PvP zone raises no flag
  (`character_pvpflag.go:131-133`), and its death is a PK if its flag ended before the hit, since
  the kill exemption needs both in a PvP zone at the death (`character_karma.go:29-34`). A player
  inside near the edge may step out after the cast's launch, which fixes the set the hit lands on
  (`network/magic_skill.go:287-302`, `launchCastTargets`), and is then such a player. The bot's
  own target is held by the arena-edge rule above instead. The same also holds for a summon or pet
  in reach whose owner the bot does not see inside
  the arena polygon by at least `ArenaEdge` (the owner is the player whose `CharInfo` name is the
  summon's `NPCInfo` title): the server hits a summon by its own zone but credits its death by its
  owner's, so a summon left inside while its clean owner stands outside dies as a karma kill
  ("Server behavior", Area skills);
- from the duel countdown to its end, the bot casts no offensive area or aura skill (Wild Sweep,
  Seal of Binding 1208, target AURA) and a pole wielder does not swing while any player other than
  its opponent, any summon or pet, or any attackable NPC, is in that reach (collision radii
  included, so a large monster counts from further out): a hit on a flagged karma-free
  bystander flags the bot, a flag cancels its duel (`duel/duel.go:206-213`), a hit on an NPC
  takes it out of the duel, and so does a hit on a summon whose owner is not in its duel
  ("Server behavior", Duel end; area skills and pole swings hit attackable NPCs and summons,
  `target_predicates.go:22`, `:33-37`, `attack/controller.go:370-381`; `creature/duel.go:43-50`).
  Every summon counts, its opponent's included, since the bot does not read the summon's duel
  team. A pole wielder repositions or holds as in the open world above (a pole has no
  single-target step); a swing already under way when an NPC or summon steps into the cone still
  lands and cancels the duel ("Risks");
- from the duel countdown to its end, `SupportFlagged` is treated as off, through the changed
  `MaySupport` gate (section 2, "Support gate"; today's gate would pass an unflagged karma player,
  and healing it flags the bot just the same, `character_pvpflag.go:88-93`, which cancels the duel
  at its next check, `duel/duel.go:205-207`): the bot casts no beneficial skill on a flagged or
  karma player other than itself, and no party skill (Chant of
  Life 1229, Chant of Fire 1006, target PARTY) while a party member in its radius is flagged or
  has karma, since each such target flags the caster (`character_pvpflag.go:65-95`). Nor does
  it support its duel opponent, whatever the opponent's flag: the changed gate passes a clean
  party member, and the bot may duel one one on one (rule 1; the one-on-one challenge checks no
  party, `network/duel.go:31-62`). So in that time `MaySupport` refuses the opponent, and a
  party skill is not cast while the opponent is a party member inside the skill's radius.

### 4. How the bot finds fights (D36)

| Source | What the bot does | Bound |
| --- | --- | --- |
| A player attacks it | Fights back (today). | — |
| A duel challenge (`ExDuelAskStart`) | Accepts after the reaction delay when it can duel (out of combat, HP and MP at least half, no flag, no karma, not in a store, not in a party duel, standing where a duel is allowed, and clear of monsters: see below); else declines at once. A one-on-one challenge from a clan mate, an ally or a party member is taken like any other: the server lets them duel and lets the duellists attack each other (section 3, rule 1). A bot-marked challenger is declined at once (D38). Party duels are declined (out of scope). | one answer per challenge |
| A whispered `duel` from a player | Challenges that player (`RequestDuelStart` with its name) when both can duel and it is within 2000. Extends D3 for this whisper, as D20 did for the crafter's (M11's `join`, D28, would do the same but is still proposed). The challenge takes the same per-bot budget as its own challenges: a whispered `duel` that arrives within `ChallengeEvery` of the bot's last `RequestDuelStart`, own or whispered, is dropped (the player may whisper again later). `DeclineHold` does not apply to a whisper: the player asks for this duel, so a hold from an earlier decline or refusal does not drop it. A decline (1931) or refusal (153, 1926, 2017-2028) of the whispered challenge starts `DeclineHold` for the bot's own challenges of that player, as in the next row, and holds nothing for the player's later whispers but `ChallengeSameEvery`. | one per `ChallengeEvery` (3 min) per bot, shared with its own challenges, and once per player per `ChallengeSameEvery` (30 min); never held by `DeclineHold` |
| Its own challenge | Out of combat and clear of monsters (below), at its spot or on the road, it challenges at most one non-bot player in sight within 2000 who is out of combat, not flagged, without karma, not in a store, of its class tier, both standing where a duel is allowed (below), and only when the engage estimate (section 8) passes against that player's row. The same player is challenged at most once per `ChallengeSameEvery` (30 min). A decline (1931) or a refusal (153, 1926, 2017-2028) of any challenge to that player, its own or a whispered one, holds that player from its own challenges for `DeclineHold` (2 h). The holds are per bot (bots share no hold table). | one per `ChallengeEvery` (3 min) per bot, shared with whispered challenges; once per player per `ChallengeSameEvery` (30 min); `DeclineHold` (2 h) after a decline or refusal |
| Flagged and karma players in sight | Attacks per section 3 ("hunting"). | the fight rules |
| Arenas | Every `ArenaEvery` (90 min, ± jitter) it travels to the nearest arena reachable in its SCC or by gatekeeper (M7 trips), and stays while any other player is inside, attacking the players section 3's "Arena targets" picks (its class tier, its own HP and MP above `StandHP` and `StandMP`, the nearest one at least `ArenaEdge` inside); it leaves after `ArenaAlone` (5 min) alone or `ArenaStay` (20 min), always by section 3's arena-edge rule: no attack started, its own damage cast cancelled or landed, its auto-attack dropped with a move inside, one attack interval waited, then out. Its spot is released as for a supply trip (D16). | one trip per `ArenaEvery` |

**Where a duel is allowed.** The server refuses a duellist, challenger or acceptor, who stands in a
peace zone, a town, the Derby track, a PvP zone (every arena), a running siege zone, water or a
no-restart zone: the one who cannot duel gets 1940, a challenger whose target cannot gets 2017-2028
(`network/duel.go:37-38`, `:170-172`, `:199-232`, `duelProhibitedArea` at `:226`;
`character_duel.go:67-69`; an arena sets the PvP flag, `model/zone/kinds.go:17-21`). So "can duel"
in every row above includes, for the bot and for the player it challenges: neither position in a
peace, town, Derby track, arena, water or no-restart polygon (`zones/PeaceZone.xml`,
`TownZone.xml`, `DerbyTrackZone.xml`, `ArenaZone.xml`, `WaterZone.xml`, `NoRestartZone.xml`, X2;
the water zones through today's `LoadWaterZones`, `internal/datapack/zones.go:34-60`).
A siege zone is a duel block only while its siege runs (`model/zone/siege.go:45-57`), which the bot
cannot see. The server sends 1940 for any reason the bot's own side cannot duel
(`network/duel.go:37-40`, `:170-173`), passing ones among them: its server-side combat stance and HP
or MP under half (`:202-205`). So every 1940 sets the duel state back to idle, and only an
**unexplained** 1940, one that comes while the bot's own pre-checks pass (no combat stance by its
`AutoAttackStart` and `AutoAttackStop`, `world.go:749-752`, `setCombat` at `:988`; HP and MP at
least half by its `StatusUpdate`; no flag, no karma, no store, no known blocked polygon), also holds
the bot's answers (it declines) and its own challenges, until it has moved more than 2000 from where
the 1940 came or `DuelBlockHold` (10 min) has passed, whichever comes first. A 1940 with a failed
pre-check (a race with a combat stance or a vitals change) holds nothing: the pre-check already
keeps the bot from duelling until it passes. A `pvp` bot on a supply trip in town or visiting an
arena therefore declines every challenge at once.

**Clear of monsters.** Any hit between the bot and an NPC cancels its duel, and a cancelled
one-on-one duel gives back nothing ("Server behavior", Duel end). So the bot accepts or makes a
challenge only when no NPC is in its attackers table and no aggressive monster in sight (its
template's `aggroRange` above 0, `internal/datapack/npc.go` `AggroRange`) stands within that aggro
range plus `DuelClear` (300) of the bot. From sending or accepting a challenge until the duel
ends or its state is back to idle, it starts no fight with a monster (a fight would also put it in
combat, which the server refuses at the answer, `network/duel.go:166-173`), and during the duel
its chase and kite moves keep the same clearance from aggressive monsters. A monster that attacks
it anyway cancels the duel (section 3, rule 1); the bot then fights it as today.

**Duel ground.** The server also cancels a duel, once a second, when a duellist stands in a peace,
siege or PvP zone or more than 2000 (3D) from its opponent (`duel/duel.go:203-213`, `disturbed`;
`character_duel.go:66-69`, `InDuelBlockedZone`; `duel/state.go:54-56`, `Range`), and a cancelled
one-on-one duel gives back nothing (`network/duel.go:445`, `:553-564`). A town and the Derby track
raise the peace flag ("Server behavior", Peace flag), and every arena is a PvP zone
(`model/zone/kinds.go:17-22`). So from the countdown to the duel's end, every move the bot sends
itself is checked before it is sent: the clearance moves above, section 5's kite stand points,
section 3's pole reposition and its disengage moves. Its destination and every point of its
checked path (the M7 segments) must lie at least `DuelEdge` (100) outside every peace, town, Derby
track and arena polygon (X2) and outside every siege polygon (`zones/SiegeZone.xml`, 15
`NPoly` zones: 9 castle battlefields and 6 clan hall battlefields, `partisan_agit001`,
`devastated_castle`, `bandits_stronghold`, `rainbow_spring`, `beast_farm_guildhall` and
`fortress_of_dead`, X2) the bot is not already standing in, since it cannot see whether a siege runs
(`model/zone/siege.go:38-42`, `setBattlefieldHolds`, raised only by a running siege); and its
destination must lie within `DuelLeash` (1800) of the opponent's last known position. A stand
point that fails is skipped like one the geodata blocks; with none left the bot does not kite or
reposition and fights from where it stands, and a pole wielder holds as with bystanders on both
sides (section 3). A disengage move takes the first of its headings (`turnAround`,
`internal/botclient/combat_rules.go:364`) whose step passes the check, else a move to where the
bot stands, which goes nowhere and still ends the attack intention (`orStay`, `:352`). The bot's
chase is the server's walk toward the opponent after its `Action` or cast, and that walk goes on
without any new packet from the bot: once the bot attacks, the server keeps an attack intention
and re-follows the moving opponent on every think (`model/actor/ai/player.go:358-394`,
`MaybeStartOffensiveFollow` at `:376`), on a path of its own, until a move replaces the intention
(`orStay`'s comment, `combat_rules.go:347-351`). So the check does not stop at what the bot sends.
While the bot has an attack or cast intention on its duel opponent, on every decision and on
every `MoveToLocation` or `MoveToPawn` of its own (`world.go:682-694`, the server's follow legs),
it checks the straight segment from itself to the opponent's current position and the
destination of its own current leg. When the opponent is out of the bot's attack or cast range
(so the server would walk it) and that segment or that destination passes within `DuelEdge` of
such a polygon, or the bot's own position is already within `DuelEdge` of one, the bot sends the
checked disengage move above (the first heading whose step passes the check, else `orStay`),
which drops the intention and stops the server's follow. Then, and whenever that segment fails
before an attack, it sends no `Action` and no skill out of its range, uses only the steps it
can cast from where it stands, and waits. Otherwise a chase only shortens the distance to the
opponent, so an opponent beyond `DuelLeash` is chased at once, even by a row that would rather
keep its range. A duel that runs
out its 2 minutes this way ends in a timeout, which gives everything back (`duel/duel.go:183-184`; only `Canceled` is abnormal,
`network/duel.go:445`). An opponent who walks into such a zone or past 2000 cancels the duel by
its own move ("Risks").

Every value is a persona default. The PvP reaction delay is 200-600 ms (the plan's 300-1200 ms is
the persona default for other events), so an interrupt can land in time (section 7).

### 5. The fight strategy: a PvP layer over the M10 rotation

Against a player the bot runs its M10 row (the same `Step` list, gates and `Melee` policy), with
the PvP row of "Behavior per class" applied on top. New `Step` fields:

```go
// Vs casts the step only against those opponent archetypes; empty for any. PvP marks a step
// that runs only against a player (Lionheart, Mental Shield, Aggression as a peel).
Vs  []Archetype
PvP bool
// Interrupt casts the step only on an opponent's interrupt-worthy cast that lands later than
// this step would (section 7). Hold keeps the step ready while the opponent is a caster or a
// healer: it is not cast at its reuse.
Interrupt, Hold bool
// Break casts the step only while breaking off (section 9); Peel only on an enemy attacking the
// bot's healer or leader (section 11).
Break, Peel bool
```

Against a player the rotation runs in this order, each group under the M10 gates:

1. sustain under its HP gate, the **long-cast rule** applied: a heal or HOT with a template hit
   time of 4000 ms or more, on itself or on its party (Self Heal, Elemental Heal, Divine Heal,
   Heal, and 1229 Chant of Life: `skillType` HOT, `target` PARTY, `hitTime` 7000,
   `skills/1200-1299.xml`, about 9 s at an Orc Shaman's cast ×0.78, 6.3 s with a spiritshot), is cast only while no melee
   opponent is within its attack range plus 150, or the opponent is stunned, asleep or rooted
   (`CharInfo` abnormal mask);
2. interrupts (section 7);
3. control: the row's control steps on a fresh opponent and again when the effect has worn off
   (the abnormal mask cleared, or its datapack duration passed) and the land rate (section 6) is
   at least `MinLand` (35 %);
4. the opener, then damage steps, the finishers first once the pool estimate (section 9) is under
   the step's expected damage;
5. the `Melee` policy.

**Sleep and damage.** Every hit and every DOT tick ends sleep (`character_stats.go:858-869`,
`applyNonConsumptionDamageEffects`: `StopByType(effect.TypeSleep)` for normal hits and DOTs alike);
root and fear stay. So a sleep step (1097 Dreaming Spirit, 1069 Sleep) used to hold an opponent,
as a `Break` step or as party control (section 11), is cast only on an opponent that:

- carries no HP DOT: its `CharInfo` abnormal mask shows none of the bleeding (0x1), poison (0x2)
  and flame (0x4000) bits that every player DOT of section 10 sets (Bleed 96, Sting 223, Poison
  129, Curse: Poison 1168, Venom 1095: `abnormal="bleeding"` or `"poison"`; Chill Flame 1100 and
  Frost Flame 1107: `abnormal="flame"`; `model/skill/abnormal.go:32-33`, `:46`), and no DOT step
  of the bot's own landed on it within that DOT's duration (its tick count times its period,
  datapack), so a `CharInfo` still on its way does not hide one; Venom ticks every 3 s for 10
  ticks, Chill Flame and Frost Flame 15 ticks (`skills/1000-1099.xml` 1095, `skills/1100-1199.xml`
  1100, 1107);
- neither the bot nor its party will hit while the sleep lasts: a `Break` sleep is followed by the
  flee, never by an attack, and in a party the target is one no member is hitting (section 11).

Otherwise the row uses its root or fear `Break` step (1092 Fear: 10 ticks of 2 s,
`skills/1000-1099.xml:1672`), or none.
An `Int` sleep is exempt: its job is to abort the cast, which it does when it lands, and a later hit
only wakes the opponent.

**Kite.** A caster (any mystic row, and the bow branches) whose root or slow landed on a melee
opponent (Dryad Root and Seal of Binding, Ice Bolt and Slow from the mystic rows; Entangle from the
ES bow branch) walks to a stand point at its cast range away from the opponent (the M7 stand point
search, checked segments; in a duel only a stand point that passes section 4's duel-ground check)
before its next cast, and keeps the distance while the effect lasts. The
melee rows' slows (Freezing Strike on PK, Cripple on MO, Entangle on EK) do not kite: they keep a
fleeing or kiting opponent within reach. Kiting monsters stays out of scope, as in M10.

### 6. Debuff land rates (`charmodel`)

`charmodel.LandRate(skill, caster, target)` copies the server formula ("Server behavior"). The
opponent's base CON and MEN come from its class template by class id (base stats are fixed per
class; dyes add at most ±5 and are ignored); its M.Def and level are estimated from its tier at the
bot's level band (both cancel in `matk` to about 1 at equal level). The bot's own vulnerabilities
come from its gear and buffs through the model (the jewel, set and buff stat functions, X2).
Rates at equal level, with `matk` = 1, no dyes, no resist gear, rounded down to a whole percent
(`LandRate` returns the server's float; the roll lands on `ceil(rate)` of the 100 integer rolls).
"Equal level" means the skill's `magicLvl` (or the caster's level, for a skill without one) equals
the target's level, so Δ = `lvlDepend` and the level term is not 1: 1.01 for Shield Stun, Sleep
and Dryad Root (`lvlDepend` 2) and 1.005 for Stun Attack (`lvlDepend` 1) (`formula.go`
`SkillLevelModifier`, `:599-613`; `skills/0000-0099.xml` 92, `skills/0100-0199.xml` 100,
`skills/1000-1099.xml` 1069, `skills/1200-1299.xml` 1201). The land bases are Shield Stun's
`power` 80, Stun Attack's effect `effectPower` 50, Sleep's and Dryad Root's `power` 70 and 80. So a
cell is `floor(base × stat × level term)`, for example Human Fighter Shield Stun
80 × 0.743 × 1.01 = 60.04 → 60:

| Opponent base | CON, MEN | Shield Stun (80, ×1.01) | Stun Attack (50, ×1.005) | Sleep (70, ×1.01) | Dryad Root (80, ×1.01) | Cast-break offset |
| --- | --- | --- | --- | --- | --- | --- |
| Human Fighter | 43, 25 | 60 % | 37 % | 61 % | 70 % | −28 |
| Elven Fighter | 36, 26 | 70 % | 43 % | 60 % | 69 % | −30 |
| Dark Fighter | 32, 26 | 75 % | 46 % | 60 % | 69 % | −30 |
| Orc Fighter | 47, 27 | 54 % | 33 % | 60 % | 69 % | −31 |
| Dwarven Fighter | 45, 27 | 57 % | 35 % | 60 % | 69 % | −31 |
| Human Mystic | 27, 39 | 81 % | 50 % | 55 % | 63 % | −48 |
| Elven Mystic | 25, 40 | 83 % | 52 % | 55 % | 62 % | −49 |
| Dark Mystic | 24, 37 | 84 % | 52 % | 56 % | 64 % | −45 |
| Orc Mystic | 31, 42 | 76 % | 47 % | 54 % | 61 % | −52 |

So stuns are a fighter's tool against casters and dark elves, and lose about a third of their rate
on an orc or a dwarf; sleep and root land about the same on everyone; a mystic's MEN makes its casts
hard to break by damage. A jewel or set multiplies the matching rate by `1 − v/100` (Earring of
Antharas: stun × 0.4). The bot casts a control step only above `MinLand`, skips a sleep or root on
an opponent it saw resist the same skill twice in this fight (139), and the table is a `charmodel`
golden test.

### 7. Interrupts

An opponent's `MagicSkillUse` is **interrupt-worthy** when its skill type (datapack) is HEAL, HOT,
HEAL_PERCENT, RESURRECT, SLEEP, ROOT, FEAR or MUTE, or an MDAM with a template hit time of 3000 or
more, and its target is the opponent itself, one of its allies, the bot or a member of the bot's
party. It lands at `arrival + HitTime` (the scaled time the packet carries).

The bot answers with its row's `Interrupt` step when the step's own scaled hit time plus the
reaction delay and the walk to its cast range end before the opponent's cast lands. The bot
predicts its own scaled hit time as the server computes it ("Server behavior", Cast time): the
template hit time × 333 / the bot's cast speed (a magic step) or attack speed (`UserInfo`),
truncated; × 0.70 for a magic step (`isMagic`, X2) while a spiritshot or blessed spiritshot is
expected charged, that is while the server reports the auto-use of one on (`Inventory.AutoShots`
from `ExAutoSoulShot`, `items.go:178-181`, `:224-227`) and a stack of it is carried; then floored
at 500 ms when the template time is at least 500. A prediction that is too short (the shot ran
out between casts) costs one late interrupt; the arena-edge rule, where a late landing matters,
re-checks against the hit time of the bot's own `MagicSkillUse` (section 3).

- a stun (Shield Stun, Stun Attack, Stunning Fist, Stunning Shot) or a sleep that lands aborts the
  cast;
- a damage step rolls the cast break, `15 + sqrt(13 × damage) − MEN offset` with the model's
  expected damage; a damage interrupt is used only when that chance is at least `MinBreak` (30 %),
  so `sqrt(13 × damage)` must reach 15 plus the offset: against a mystic's −45 to −52 offset a hit
  of about 280-345 or more (MEN 37: 277, MEN 39: 305, MEN 42: 345), against a fighter's −28 to −31
  about 140-165 (`skill/formulas/cast.go:13-14`). A level 7-39 mystic's Ice Bolt (power 8-16,
  `skills/1100-1199.xml` 1184) is far below that, so the base and healer mystic rows have no
  interrupt against the mystics who cast the Heals, Sleeps and long nukes worth breaking.

A `Hold` step is not cast at its reuse while the opponent is a caster or a healer, so it is ready
for the next interrupt-worthy cast; against melee and archers it is cast at reuse. A skill with no
damage and no stun (Wind Shackle, Curse: Weakness) never interrupts: nothing rolls a break.

### 8. Matchup by the opponent's class

The archetype comes from the opponent's class id through its first-profession ancestor (the
server's `classParent`, copied with the M10 class templates), with the right-hand weapon kind from
`CharInfo` splitting the scout lines:

| Archetype | Classes |
| --- | --- |
| Melee | the fighter base classes; Warrior, Knight, Elven Knight, Palus Knight, Orc Raider, Monk, Scavenger, Artisan and their lines; Rogue, Elven Scout and Assassin lines with a dagger or a sword |
| Archer | Rogue, Elven Scout and Assassin lines with a bow |
| Caster | Human, Elven and Dark Mystic; Human, Elven and Dark Wizard and their lines (summoners included: the bot fights the owner) |
| Healer | Cleric, Elven Oracle, Shillien Oracle and their lines |
| Battle caster | Orc Mystic, Orc Shaman and their lines |

The **engage estimate** (hunting, the bot's own unprompted challenges and PK mode's victim choice:
sections 3 and 4) is the model's time to kill the
reference opponent of the opponent's PvP row (from its class id; class template, the bot's level,
grade-typical gear, and the row's steps from the brain's row table, `pvpRowTable`, which X6 passes
to the estimate X5 built: section 12) against its time to
die, with the M10 spot margin of the row (tank 1.5, normal 2.0, glass
cannon 3.0). A duel offered to the bot is taken whatever the estimate: it costs nothing. A
whispered `duel` is the player's own request and is answered whatever the estimate too.
Fighting back and arenas need no estimate (an arena target is picked by section 3's "Arena
targets": class tier, the bot's own HP and MP, nearest inside the edge margin).

The Artisan line (Artisan 56, Warsmith 57, Maestro 118) has no PvP row: M10 leaves it to the
crafter role (`docs/strategy.md:612`), and section 10 covers the 26 rows M10 plays. An Artisan
opponent therefore uses SC's row wherever a row is needed (its `PvPValue` share, its reference
opponent, its interrupt-worthy steps): both are dwarven BLUNT melee with 100 Stun Attack from 20
(`classes/dwarfFighter.xml:94` for Artisan), so SC's row is the closest stand-in.

Each row's matchup columns name how it plays each archetype: close in on a caster, keep a melee
opponent off (slow, root, kite), stun a healer's heals, Deflect Arrow against an archer.

### 9. CP, the pool estimate and breaking off

- **CP.** Player damage takes CP first, so CP is the bot's first pool in PvP. The bot drinks a CP
  potion (5592, then 5591) when its CP falls under `CPPotion` (50 %) in a PvP fight other than a
  duel and it carries one, under the potion's 500 ms reuse; HP potions, heals and drains follow
  M10's `ruleCritical` on HP. CP potions are kept (M8 keep hook): never sold, deposited or used
  against monsters, and not drunk in a duel: a normal duel's end gives back the CP, HP and MP
  saved at its start (`network/duel.go:553-564`, `restoreDuelCondition`), the bot buys none
  (section 12), and a duel costs nothing to lose.
- **Cures in a duel.** The same end gives back the effects saved at the start, so a poison or
  bleed is gone after the duel anyway (`network/duel.go:565-569`). The bot still uses a cure during
  a duel: cures are cheap (86-270 adena) and bought on every supply trip (section 12), and a DOT's
  ticks decide a two-minute duel.
- **Cures.** In a PvP fight, when its own effects (`AbnormalStatusUpdate`, skill id and level)
  show a poison or bleed effect (the datapack skill's stack type `poison` or `bleeding`, or effect
  type POISON or BLEED) with at least half of its ticks left, the bot uses the cure that removes
  it. The effect's abnormal level is the lower of the skill's two set levels, as the server's
  negate passes either (`hooks_buff.go:221-233`, `negateLevelAllows`): `effectAbnormalLvl`, counted
  only for a skill with an `effectType` (Sting 223 sets only this one), and `abnormalLvl` (Poison,
  Curse: Poison, Venom, Bleed); with neither set (both -1) no cure works. Antidote 1831 or Bandage
  1833 for level 3 or less, Greater Antidote 1832 or Emergency Dressing 1834 for 4-7 (the plain
  kind first when both work); level 8 has no cure. One
  `UseItem` per effect seen, under the 500 ms item reuse. Against monsters it uses none.
- **Pool estimate.** The opponent's HP and max HP are read once, at selection (or continuously in
  a duel from `ExDuelUpdateUserInfo`); its CP and max CP from its latest `CharInfo` (rebroadcast on
  every abnormal change) or the duel window. Every SystemMessage 35 on the target is taken from CP,
  then HP. The estimate drives finishers and the break-off; it never makes the bot re-select a
  target to refresh HP.
- **Break off** (open world only; never in a duel or an arena): with its CP gone and its HP under
  the row's `PvPBreakHP` (tank 20, normal 30, glass cannon 40 %), no heal or potion ready, and its
  pool fraction under the opponent's estimate, the bot casts the row's ready `Break` step (a
  root, stun, fear, sleep, slow or Dash; a sleep only under section 5's "Sleep and damage" rule,
  since the bot's own DOT on the opponent would wake it within one tick), then flees as in M10
  (`FleeOutrun` against the opponent's run speed from `CharInfo`). A bot that cannot outrun fights
  on.
- **Break-cast gate.** A `Break` step is cast only when it can finish: its scaled hit time is
  under 4 s, or the opponent is stunned, asleep or rooted (`CharInfo` abnormal mask), or it is a
  melee opponent that cannot reach the bot before the cast lands (its distance minus its attack
  range, over its run speed, is more than the scaled hit time), or the model's chance that the
  opponent's hits during the cast break it (section 7's cast-break chance with the bot's own MEN,
  over the opponent's expected hits) is under 50 %. Otherwise the bot skips the step and flees at
  once. No row uses Sleep 1069 to break off: its 10 s hit time almost never finishes.
- **Ultimate Defense rules out the break-off.** 110 Ultimate Defense (KN, EK, PK rows, `S<20 vs M`)
  locks its caster in place for 30 s ("Server behavior", PvP self buffs), and its HP gate falls
  where the break-off starts. So in the open world the row casts it only when it will not break off:
  the break-off conditions above do not all hold, or the bot cannot outrun the opponent (it fights
  on anyway). A bot that breaks off skips it. In a duel or an arena, where nobody breaks off, it is
  cast at its gate. While its own Ultimate Defense is on (`AbnormalStatusUpdate`, skill 110), the
  bot starts no walk (no break-off flee, no kite, no chase, no stand-point or pole reposition, no
  walk out of an arena) until the effect ends. **The disengage move still goes.** Section 3's flag
  guard, its arena-edge stop and arena leave, and the pole's bystander drop drop the bot's
  auto-attack and the follow-up swing with a move, the only way this plan has to end an attack,
  so under Ultimate Defense the bot still sends that one move (after the Esc cancel of its own cast
  in flight, as without the buff). The server takes it as a stop: a move request from a player
  that cannot move (`MovementDisabled`, which counts the `ImobileBuff` lock) idles it, dropping
  every intention, active and queued, and answers `ActionFailed`
  (`network/movement.go:87-91`, `startLiveMove`; `network/live_player.go:992-1001`, `tryToIdle`,
  and `:1024-1038`, `goIdle`; `model/actor/player/character_attack.go:306-313`;
  `skill/effect/core.go:245`). The move takes the bot nowhere, and a swing already under way still
  lands within one attack interval, as without the buff. In an arena the leave then waits: the
  stop move and one attack interval as in section 3, and the walk out once the effect has ended.

### 10. Behavior per class (PvP)

This section is the spec that X7 (fighter rows) and X8 (mystic rows) implement and that the brain
tests check row by row (section "Testing"). It covers the nine base classes and seventeen first
professions of M10 ("Behavior per class" there), with the same row ids and notation (`@N` from
level N, `[KIND]` weapon gates, `S<N` own HP, `T≥N` target HP, `≤N` range, `A≥2` attackers,
`C≥N` charges). PvP marks: `vs M` (melee), `vs A` (archer), `vs C` (caster), `vs H` (healer), `vs
B` (battle caster); `Int` interrupt step, `Hold` held for interrupts, `Break` break-off step,
`Peel` party peel. Every skill id below is in the row's merged class tree at the level given
(`classes/*.xml`), and every control's land base is from `skills/*.xml` (power or effect power).
A row lists only what differs from its M10 row; the M10 opener, rotation and gates stay otherwise.
`TestPvPRowsShipped` checks the tables against the shipped datapack as `TestBehaviorRowsShipped`
does.

#### Opener, control and interrupt

| Row | PvP opener | Control-first rotation | Interrupt |
| --- | --- | --- | --- |
| HF | Bow: 56 Power Shot @5 from 700. Else walks in. | No control skill: 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5, 16 Mortal Blow [DAGGER] @5, melee. | Damage only (Power Strike), under `MinBreak`. |
| EF | As HF. | As HF. | As HF. |
| DF | 70 Drain Health @15 from 600 (vs any). | 70 Drain Health; then as HF. | As HF. |
| OF | Walks in. | 29 Iron Punch [DUALFIST] @5, 3 Power Strike, melee. | Iron Punch, under `MinBreak`. |
| WF | Walks in (no Spoil on players). | Melee. | none |
| HM | vs M, A: 1184 Ice Bolt @7 (slow) from 600; vs C, H, B: 1177 Wind Strike. | vs M: 1164 Curse: Weakness @14 (P.Atk down, base 80); 1184 Ice Bolt @7 (kite); vs H: 1168 Curse: Poison @7; 1177 Wind Strike. | None: Ice Bolt (power 8-16) never reaches `MinBreak` against a mystic's MEN, and mystics cast the Heals and Sleeps worth breaking (section 7). |
| EM | vs M, A: 1206 Wind Shackle @14 (attack speed down, base 80); else 1184 Ice Bolt. | vs M: 1206 Wind Shackle, 1164 Curse: Weakness @14, 1184 Ice Bolt (kite); 1177 Wind Strike. | As HM. |
| DM | vs M, A: 1206 Wind Shackle @14; vs C, H: 1168 Curse: Poison @7. | vs M: 1206 Wind Shackle, 1184 Ice Bolt (kite); vs C, H: 1168 Curse: Poison; 1177 Wind Strike. | As HM. |
| OM | 1100 Chill Flame @7 from 600. | vs C, H: 1095 Venom @7; 1100 Chill Flame; 1090 Life Drain @7. Break: 1097 Dreaming Spirit @7 (sleep, base 80) only on an opponent carrying no DOT (section 5, "Sleep and damage"), else 1092 Fear @14 (base 40); with its own Venom or Chill Flame on the opponent (the usual case) that means Fear, and below 14 no `Break` step: it flees at once. | 1097 Dreaming Spirit Int, under section 7's timing rule: its own scaled hit time (4000 ms at an Orc Mystic's cast ×0.78 and a spiritshot's ×0.70, about 3.6 s; about 5.1 s with no spiritshot charged) plus the reaction delay (0.2-0.6 s) must end before the cast lands, so with a spiritshot only casts landing more than about 3.8-4.2 s after their `MagicSkillUse`: a Sleep 1069 (10000 ms), and a Human Cleric's 1011 Heal cast without a spiritshot (5000 ms, 5.0 s at cast ×1.00); the same Heal cast with one (3.5 s) lands first. Without its own spiritshot only casts landing after about 5.3-5.7 s, so no heal. |
| KN | vs C, A: 70 Drain Health @20 from 600. Else walks in. | 92 Shield Stun [SHIELD] @20 (base 80) Hold vs C, H, else at reuse; 3 Power Strike; melee. Peel: 28 Aggression @24 (TargetMe). | 92 Shield Stun Int. |
| WA | Walks in; 78 War Cry @20 at engage. | vs C, H, B: 287 Lionheart @36 at engage; 100 Stun Attack [BLUNT, BIGBLUNT] @20 (base 50) Hold vs C, H; 255 Power Smash @20; 245 Wild Sweep [POLE] A≥2 (area rule, section 3). | 100 Stun Attack Int. |
| RO | Dagger: walks in, 4 Dash @20 from beyond 400 vs C, A. Bow: 56 Power Shot from 700; 101 Stunning Shot @36 from 900. | Dagger: 96 Bleed [DAGGER] @24 T≥50, 16 Mortal Blow. Bow: 101 Stunning Shot [BOW] @36 (stun and target drop, base 50) Hold vs C, H; 56 Power Shot. Break: 4 Dash. | Bow: 101 Stunning Shot Int, under section 7's timing rule: its own scaled time with a bow (about 3.4 s before DEX and buffs, "Server behavior", Long hit times) plus the reaction delay must end before the cast lands, so only casts landing more than about 3.6-4.0 s after their `MagicSkillUse` (a Heal, a Sleep, the 4-5 s nukes of a slow caster), never a Battle Heal (2000 ms). Dagger: damage only. |
| EK | Walks in. | vs C, A: 102 Entangle @36 (slow, base 80); 3 Power Strike; melee. Peel: 28 Aggression @24. Break: 102 Entangle. | Damage only. |
| ES | Bow: 56 Power Shot from 700; vs M: 102 Entangle @36 first (kite). Dagger: walks in. | 102 Entangle vs M (bow) or vs C, A (dagger); dagger: 96 Bleed, 16 Mortal Blow; bow: 101 Stunning Shot @36 Hold vs C, H, 56 Power Shot. Break: 102 Entangle, 230 Sprint @32. | Bow: 101 Stunning Shot Int, timed as RO's. |
| PK | 70 Drain Health @20 from 600. | vs C, H: 129 Poison @20 (base 70, CON); vs M, A: 115 Power Break @32 (P.Atk down, base 80); 105 Freezing Strike @36 (slow) vs M; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24; 3 Power Strike. Peel: 28 Aggression @24. | Damage only. |
| AS | Dagger: 70 Drain Health @20 from 600. Bow: as RO. | Dagger: vs C, H: 129 Poison @20; vs M: 115 Power Break @32; 96 Bleed; 16 Mortal Blow; 223 Sting. Bow: as RO. | Bow: 101 Stunning Shot Int, timed as RO's. Dagger: damage only. |
| OR | Walks in. | vs C, H, B: 94 Rage @24 and 287 Lionheart @36 at engage (Rage costs P.Def, so never vs M, A); 100 Stun Attack Hold vs C, H; 255 Power Smash. | 100 Stun Attack Int. |
| MO | 54 Force Blaster [DUALFIST] @24 C≥1 from 600 vs C, A. Else walks in. | vs C, A: 95 Cripple [FIST, DUALFIST] @20 (slow, base 80); 120 Stunning Fist [DUALFIST] @20 (base 50) Hold vs C, H; 284 Hurricane Assault @36 C≥2; 54 Force Blaster C≥1; 29 Iron Punch. | 120 Stunning Fist Int. |
| SC | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20 Hold vs C, H; melee (no Spoil on players). | 100 Stun Attack Int. |
| CL | vs M: 1201 Dryad Root @25 (base 80) from 600, then kite; else 1177 Wind Strike. | vs M: 1201 Dryad Root, 1184 Ice Bolt (kite); vs C: 1177 Wind Strike. Break: 1201 Dryad Root (hit 2500, a 30 s root), then flee. | None: no stun or sleep, and Ice Bolt never reaches `MinBreak` against a mystic's MEN (section 7). |
| HW | vs M, A: 1184 Ice Bolt (slow); vs C, H: 1220 Blaze @20 from 750. | vs M: 1184 Ice Bolt, 1160 Slow @35 (kite); vs M, A: 1222 Curse Chaos @35 (accuracy down); 1220 Blaze; 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | 1274 Energy Bolt Int (hit 2000), 1172 Aura Burn Int ≤150 (hit 1500), when the break chance passes `MinBreak`. |
| EO | vs M: 1206 Wind Shackle (base), then 1201 Dryad Root @25 (kite); else 1177 Wind Strike. | As CL, with 1206 Wind Shackle vs M first. | As CL. |
| EW | vs M, A: 1206 Wind Shackle; else 1264 Solar Spark @25 from 750, before 25 1175 Aqua Swirl. | vs M: 1206 Wind Shackle, 1184 Ice Bolt (kite); 1264 Solar Spark; 1175 Aqua Swirl; 1274 Energy Bolt ≤400; 1172 Aura Burn ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | As HW. |
| SO | vs M: 1206 Wind Shackle, then 1201 Dryad Root @25; vs C, H: 1168 Curse: Poison. | As CL, with 1206 Wind Shackle vs M and 1168 Curse: Poison vs C, H. | As CL. |
| DW | vs M, A: 1206 Wind Shackle; vs C, H: 1168 Curse: Poison; else 1266 Shadow Spark @25 from 750. | vs M: 1206 Wind Shackle, 1160 Slow @35, 1184 Ice Bolt (kite); 1266 Shadow Spark; 1178 Twister; 1172 Aura Burn ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | 1172 Aura Burn Int ≤150, when the break chance passes `MinBreak`. |
| OS | 1107 Frost Flame @20 from 750. | vs C, H: 1102 Aura Sink @25 (MP drain over time); 1107 Frost Flame; 1090 Life Drain; 100 Stun Attack [BLUNT, BIGBLUNT] ≤40 Hold vs C, H. Break: 1208 Seal of Binding @25 (root around itself) on a melee opponent next to it, 1097 Dreaming Spirit only on an opponent carrying no DOT (its own Frost Flame included: section 5, "Sleep and damage"), 1092 Fear (all hit 4000, so under the break-cast gate: an orc mystic's MEN keeps the break chance low). | 100 Stun Attack Int ≤40; 1097 Dreaming Spirit Int timed as OM's (about 3.6 s scaled with a spiritshot, so casts landing after about 3.8-4.2 s, such as a Sleep 1069 or a Heal cast without a spiritshot; 5.1 s without one, so no heal). |

#### Matchup, CP and break-off

| Row | vs melee | vs caster or healer | Sustain and CP (besides CP potions) | Breaks off at |
| --- | --- | --- | --- | --- |
| HF | Trades blows. | Closes in fast; stays in melee. | HP potions (rule 3). | normal 30 |
| EF | Trades blows. | Closes in (run 144). | 58 Elemental Heal @15 S<50, long-cast rule. | normal 30 |
| DF | Drains first. | Drain Health from 600 while it closes. | 70 Drain Health S<60. | glass 40 |
| OF | Outlasts (HP ×1.77). | Closes in; at about 124 it outruns only a Human Mystic line (about 121: "Server behavior", Run speed), so it usually fights on (`FleeOutrun` decides). | none | tank 20 |
| WF | Trades blows. | Closes in. | none | normal 30 |
| HM | Slows and weakens, kites, nukes. | Out-casts; poisons a healer. | 1147 Vampiric Touch @14 S<60; 1216 Self Heal S<40, long-cast rule. | glass 40 |
| EM | Shackles and weakens, kites. | Out-casts (cast ×1.16). | 1216 Self Heal S<50, long-cast rule. | glass 40 |
| DM | Shackles, kites. | Poisons, then nukes hardest. | 1147 Vampiric Touch @14 S<70. | glass 40 |
| OM | Burns and drains; fears to break off (sleeps only an opponent free of DOTs). | Venom and burns; sleeps a long cast. | 1090 Life Drain S<70. | normal 30 |
| KN | Stuns at reuse; heavy armor. | Holds Shield Stun for heals and long casts. | 45 Divine Heal @28 S<40 (long-cast rule); 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M, only where it will not break off (section 9: it locks the KN in place for 30 s). vs A: 112 Deflect Arrow @24 buffed out of combat once an archer is in sight. | tank 20 |
| WA | Stuns, smashes. | Lionheart, then holds Stun Attack for casts. | 121 Battle Roar @28 S<30. | normal 30 |
| RO | Bleeds and blows; Ultimate Evasion. | Dashes in; bow: Stunning Shot on long casts. | 111 Ultimate Evasion @28 S<30 vs M, A (no help against magic). | normal 30 |
| EK | Trades blows behind a shield. | Entangle a kiting caster, closes in. | 58 Elemental Heal @20 S<50 (long-cast rule); 110 Ultimate Defense S<20 vs M, only where it will not break off (section 9); 112 Deflect Arrow vs A. | normal 30 |
| ES | Bow: Entangle, kite and shoot. Dagger: blows. | Closes in (dagger) or shoots (bow). | 58 Elemental Heal S<50 (long-cast rule); 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| PK | Power Break, Freezing Strike, Sting. | Drains and poisons, closes in. | 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M, only where it will not break off (section 9); 112 Deflect Arrow vs A. | normal 30 |
| AS | Power Break, then blows. | Drains and poisons, hardest blows. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 vs M, A. | glass 40 |
| OR | Smashes; no Rage. | Rage and Lionheart, holds Stun Attack. | 176 Frenzy @32 S<30, else 139 Guts @36; 121 Battle Roar S<30. | tank 20 |
| MO | Stunning Fist at reuse, Hurricane Assault. | Cripples a kiter, holds Stunning Fist. | none | normal 30 |
| SC | Stuns, trades blows. | Holds Stun Attack for casts. | none | tank 20 |
| CL | Roots, kites, nukes. | Out-heals: 1015 Battle Heal @20 (hit 2000) S<35 first. | 1015 Battle Heal S<35; 1011 Heal S<60 (long-cast rule); 1035 Mental Shield @25 on itself vs C, B. | glass 40 |
| HW | Slows, kites, burns. | Blaze race; breaks long casts with Energy Bolt. | 1147 Vampiric Touch S<60; 1157 Body To Mind @25. | glass 40 |
| EO | Shackles, roots, kites. | As CL. | As CL. | glass 40 |
| EW | Shackles, kites. | Solar Spark race; Energy Bolt interrupts. | 1216 Self Heal S<40 (long-cast rule). | glass 40 |
| SO | Shackles, roots, kites. | Poisons, out-heals. | As CL; 1147 Vampiric Touch S<60. | glass 40 |
| DW | Shackles, slows, kites. | Poisons, Shadow Spark burst. | 1147 Vampiric Touch S<70; 1157 Body To Mind. | glass 40 |
| OS | Stuns at reach; Seal of Binding to break off. | Aura Sink drains a caster's MP; holds Stun Attack. | 1090 Life Drain S<70; 1229 Chant of Life (party HOT, 7000 ms, about 9 s at cast ×0.78, 6.3 s with a spiritshot) under the long-cast rule; 1006 Chant of Fire @20 (M.Def) vs C, B before the fight. | normal 30 |

Second professions play their first profession's PvP row until the M10 backlog (#332) adds their
kits; this plan's backlog issue adds their PvP rows then ("Gaps").

### 11. Party PvP

When the bot's party fights players. This section runs for every preset in any party the bot is
in, and nothing in it depends on whether the leader is a player or a bot (section 1): a `support`
bot in a player's party (the common case today: it joins on invite and heals the leader,
`docs/plan.md` "support"), a `pvp` bot in a player's party, and, once M11 lands, `farmer` (Damage)
and `support` (Support) bots in a bot party. The KN, CL, EO and SO rows below are `support` bots;
the damage rows are `farmer` or `pvp` bots. So a `support` healer in a player's party marks the
enemy healer, a `support` tank there peels with Aggression, and the healer buffs Mental Shield,
exactly as in a bot party. For every preset, fighting the enemy set below counts as fighting back,
so no hunting mode is needed:

- **Enemies** are the players hitting a party member (`Attack` or a hostile `MagicSkillUse` on a
  member: an offensive skill by a player the bot may fight, flagged or not, section 2, "Hostile
  casts"), a player healing one of them (below) once `MayFight` allows it (a heal on a flagged
  player outside a PvP zone flags the healer: "Server behavior", PvP flag), and the target of the
  leader's attack (today's assist). A summon or pet hitting a member, or the leader's target, puts
  its owner in the set when the owner is in sight; the summon itself is attacked only under
  section 3's "Summons and pets" rule (its own flag or karma, the flag guard, never in a duel),
  never on its `NPCInfo` `Attackable` alone.
- **The healer first.** An enemy whose `MagicSkillUse` is a heal (skill type HEAL, HOT,
  HEAL_PERCENT, BALANCE_LIFE) on another enemy is marked the enemy healer for 60 s; a healer-class
  enemy (section 8) is the prior before it casts. Damage rows target the enemy healer when it is
  within their reach; their `Hold` and `Int` steps go to its heals (a stun holds under their hits:
  each non-DOT hit ends it only one time in ten, `character_stats.go:876`). A sleep row never sleeps
  the healer the damage rows are hitting (their first hit wakes it: section 5, "Sleep and damage");
  it sleeps another enemy that no member is hitting and that carries no DOT, and a root row roots a
  fleeing healer. Otherwise they assist the leader's target, and a member under 30 % HP pulls them
  onto its attacker.
- **Peel.** A tank row (KN, EK, PK) casts 28 Aggression on the enemy hitting the bot's healer or
  leader: `TargetMe` turns that player onto the tank.
- **The bot's healer** heals as today (leader, self, others) under its own `SupportFlagged`: on for
  a `pvp` bot that heals (a base mystic's 1011 Heal, the OS row's Chant of Life; off during its own
  duel, and its duel opponent, a party member included, is never healed or buffed then:
  section 3), off for a `support` healer (today's default). In the open world a member that
  fights back gets flagged, so a `support` healer stops healing it unless the owner switches
  `SupportFlagged` on for that bot (D6, D35); in an arena or against a karma attacker nobody is
  flagged by the fight and it heals as usual. Under the same setting, the healer buffs 1035 Mental
  Shield on members against casters and battle casters (a tank row buffs its own 112 Deflect Arrow,
  a self buff, against archers), and it keeps its stand point behind the leader.
- Party duels: declined until M11 gives a bot party a leader that can answer one ("Gaps").

### 12. PvP value, gear and consumables (D37)

M10's `charmodel.Value(profile, role, target)` gains a PvP target: `PvPValue(profile, mix)`, where
`mix` is a share per **PvP row** of section 10 (the 26 base-class and first-profession rows; a
second profession counts as its first profession's row, a bow or dagger scout as its weapon branch,
the Artisan line as SC's row, section 8): the classes the bot saw in the last hour, else equal
shares over the rows of the bot's class tier. Archetypes (section 8) are too coarse here, since
poison and bleed come from rows, not archetypes (Poison 129 from PK and AS, a melee archetype; Venom
1095 from OM, a battle caster; Curse: Poison 1168 from HM, DM, SO and DW). Each row has a
**reference opponent** (class template, the bot's level, grade-typical gear) with its row's damage
steps and its DOT and control steps, each under its `Vs` gate against the bot's own archetype
(section 8), as the row would play the bot: a step marked `vs C, H` is in the reference opponent's
rotation only when the bot is a caster or a healer. The gates matter for the model: every poison
step of section 10 is gated (Curse: Poison `vs H` on HM, `vs C, H` on DM, SO and DW; Poison 129
`vs C, H` on PK and AS; Venom `vs C, H` on OM), so no reference opponent poisons a melee, archer
or battle-caster bot, while the bleeds (Bleed 96 on the RO, ES and AS daggers, Sting 223 on PK and AS) are
ungated and reach every bot.

**Where the rows and the mix come from.** `charmodel` is pure, with no brain or wire dependency
(`docs/strategy.md` section 3), while the `Step` type and every row live in `internal/botclient`
(`docs/strategy.md` section 4, `type Step`), so the model cannot read section 10 itself. It takes
the rows as data instead:

- `charmodel` (X3) defines `Archetype` (section 8; `Step.Vs`, added in X6, is a list of it) and a
  row table, `RowTable`: per PvP row, a list of `RefStep{Skill, Level, Vs}` (the row's damage, DOT
  and control steps with the levels the row learns by the bot's level; the skill data comes from
  the datapack fields the model already reads, X2). `PvPValue(profile, mix, rows)` and the engage
  estimate (section 8) take a `RowTable` argument. A row with no entry in the table gives its
  reference opponent no steps, only its plain attacks with grade-typical gear.
- The brain builds the table from its shipped PvP rows (`pvpRowTable`, X6: each row's `Step` list
  filtered to its damage, DOT and control steps, with `Vs` copied), which X7 and X8 fill as they
  ship rows. X5 lands first and passes its engage estimate an empty `charmodel.RowTable`; X6
  replaces that argument with `pvpRowTable`. Until X7 and X8 fill the table it is empty either
  way, so the estimate compares plain attacks and template stats, and grows more exact as each
  row ships. X10 wires the same table into the gear scores.
- The **mix** is a per-bot table owned by the brain goroutine: for each non-bot player seen in a
  `CharInfo`, its object id, class id and the time last seen; entries older than an hour drop out,
  and each class id maps to its PvP row through X3's class-to-row map (the right-hand weapon kind
  splitting the scout lines). It is built in X10, the only reader.

`PvPValue` is one number, the bot's expected share of won fights over the mix, built per row `r`
(share `s_r`) from two times:

- **Time to die**, `TTD_r` = (the bot's max CP + max HP) / the damage per second `r`'s reference
  opponent deals it: its physical damage against the bot's P.Def and evasion, its magic damage
  against its M.Def, and the ticks of its poison and bleed steps weighted by their land rates
  against the bot's CON, `poisonVuln` and `bleedVuln`.
- **Time to kill**, `TTK_r` = (the reference opponent's max CP + max HP) / (the bot's damage per
  second against it, with the bot's PvP damage multipliers `pvpPhysDmg`, `pvpPhysSkillsDmg`,
  `pvpMagicalDmg` from its gear, times `u_r`), where `u_r` is the share of time the bot is not
  controlled: one minus the time `r`'s control steps hold it, from their land rates (section 6)
  against its own vulnerabilities (stun by CON and `stunVuln`, sleep and root by MEN and their
  vulns).
- `win_r` = `TTD_r / (TTD_r + TTK_r)`, between 0 and 1 (0.5 is an even fight), and
  `PvPValue = Σ s_r × win_r`.

**The blend.** M10 compares gear by the difference a candidate makes to `Value`, which for a
Damage role is kills per minute (`docs/strategy.md` section 3, "Value", and section 5, "Gear
score"); `PvPValue` is a share. The two are put on one scale by their relative gains: a
candidate's gain for the `pvp` preset is

`Value(now) × ((1 − w) × ΔValue / Value(now) + w × ΔPvPValue / PvPValue(now))`

with `w` 0.5, where `Δ` is the candidate's value minus the current loadout's. It is in M10's own
unit (kills per minute), so every M10 comparison (gear, sets, dyes, jewels, purchases, the
grade-jump saving) takes it unchanged; with `w` 0 it is exactly M10's `ΔValue`, which other
presets keep. At `w` 0.5 a candidate that raises the PvE kill rate by 10 % weighs the same as one
that raises the expected win share by 10 %. One loadout: no swap between a PvE and a PvP set. So:

- In the M10 range (1-39) PvP value means HP, CP, P.Def and M.Def, plus the cure items below.
  Jewels up to C carry only M.Def, so a mix heavy in mystic rows buys better jewels sooner. The
  Mithril Heavy Set (D) is not expected to beat the Brigandine Set for a heavy row, poison rows or
  not: Brigandine's pieces carry more P.Def (41 + 103 + 64 = 208 against 37 + 95 + 61 = 193) and
  its set adds P.Def ×1.05 and 153 HP, its shield 20 HP, against Mithril's `poisonVuln` 20 alone
  and its shield's 126 HP (`armorSets.xml:5-6`; `items/0000-0099.xml` 47, 58, 59;
  `items/2400-2499.xml` 2411, `items/0300-0399.xml` 352, `items/2300-2399.xml` 2378;
  `skills/3500-3599.xml:22-30`, `:59-66`, `:492-507`). The `poisonVuln` term only cuts the land
  rate of the poison rows' DOTs: the **poison rows** are every row whose section 10 steps include
  Poison 129, Curse: Poison 1168 or Venom 1095, namely HM (vs H), DM, SO and DW (Curse: Poison),
  PK and AS (Poison) and OM (Venom), and all of those steps are gated `vs C, H` or `vs H`. So for
  a melee bot, the one that wears a heavy set, the term is zero against every mix: no reference
  opponent poisons it, and Mithril Heavy's set skill changes nothing; Brigandine wins on its P.Def
  and HP. The term counts only for a caster or healer bot wearing the pieces (a heavy-armor CL or
  OS, for one). The model, not this plan, ranks the two sets.
- **PvP damage weapons.** 186 C- to S-grade weapon variants carry a Special Ability with
  `pvpPhysDmg`, `pvpPhysSkillsDmg` and `pvpMagicalDmg` × 1.05 (C 26, B 14, A 102, S 44; for
  example 4809 Blood Tornado - Haste, `item_skill` 3068-2), which the server applies to every hit
  on a player ("Server behavior", PvP damage weapons). They raise kill speed through the same
  model and, like the boss jewels, are worn when owned and within the bot's grade, never bought
  for it; they lie past the M10 range (C grade is level 40).
- From the grades that have them (B and up), Tallum, Nightmare, Majestic and the boss jewels score
  through the same model, worn when owned and within the bot's grade (M8). They are never bought
  (the boss jewels list at 616 000 to 4 940 000 adena); the bot wears one it loots or is given.
- **Consumables**: the `pvp` preset adds cures to its M8 supply list ("The default list" in
  `docs/items.md`): `CureStock` (5) Antidotes 1831 and 5 Bandages 1833, and from level 20 the same
  count of Greater Antidotes 1832 and Emergency Dressings 1834 where a merchant on its trip sells
  them (23 buy lists carry each greater kind, 30 each plain one). They cost 86-270 adena each, are
  bought on the same supply trips as its potions and shots, are kept by the junk rule, and are used
  per section 9 ("Cures"); poison and bleed are the DOTs the PvP rows themselves rely on (Poison,
  Curse: Poison, Venom, Bleed, Sting). CP potions are kept and drunk in PvP only, never in a duel
  (section 9); none are bought in this milestone (their only shop takes Ancient Adena). HP potions
  and M8 buff potions as today.

### 13. Death and karma (D39)

| Fight | Death costs (server) | What the bot does |
| --- | --- | --- |
| Duel | Nothing: no death; vitals back at the end | Fights to the end and never surrenders; after `ExDuelEnd` it rests and rebuffs as after any fight. |
| Arena | No experience, no drop | Accepts a resurrection, else restarts in the village. After each of its first four deaths in the last hour it walks back to the arena; its fifth (`ArenaDeaths`, 5) sends it back to its spot instead. |
| Open world, flagged or karma opponent | The normal loss (a quarter at clan war) | Breaks off per section 9. After a death to a player, that player is not hunted or challenged for `AvoidAfterDeath` (30 min); fighting back stays. M8 lifts the death penalty as today. |
| Open-world PK (opt-in) | The killer gains karma; with karma its own death costs `RateKarmaExpLost` times the loss, and from `MinimumPKRequiredToDrop` PK kills its items may drop | PKs only a clean player of its class tier, picked on section 3's hunting preconditions (HP and MP above `StandHP` and `StandMP`, neither in a peace zone, town or Derby track, the engage estimate passing), at most one per `PKEvery` (1 h), and never while its PK kills (`UserInfo`) are at or above `PKCap` (4, one under the server's drop limit), so its items never drop. Until the victim is flagged or has karma, the server refuses every skill with `isDebuff` on it even with ctrl held, so the bot uses only `AttackRequest` and ctrl-held skills that deal direct damage and are not `isDebuff` (Power Strike, Mortal Blow, Power Shot, Drain Health, Wind Strike, Blaze, Energy Bolt, ...); the row's control, `Hold` and `Int` stuns, slows and DOT openers (Shield Stun, Stun Attack, Stunning Shot, Stunning Fist, Ice Bolt, Freezing Strike, Sting, Bleed, Poison, Curse: Poison, Chill Flame, Frost Flame, Dryad Root, Sleep, Wind Shackle, Entangle, ...) are skipped. Once the victim hits back and is flagged, the full PvP order runs. With karma, see "With karma" below: it farms monsters until karma is 0, flees from clean attackers instead of killing them past the limits, restarts at a chaotic point, and makes no town trip. |

**With karma.** A bot has karma after a PK in PK mode, or, outside it, after one of the timing
accidents in "Risks". While its karma (`UserInfo`) is above 0:

- **Clean attackers.** Anyone may attack it without force, and a clean player who does stays clean
  ("Server behavior", A karma player), so rule 6 never allows fighting back and killing that player
  would be another PK: more karma, one more PK kill. The bot fights such an attacker back only
  when PK mode is on and the D39 limits pass at that moment (`PKEvery` since its last PK, PK kills
  under `PKCap`); the kill then counts as that PK, with PK mode's non-debuff damage only. Otherwise
  it flees through `ruleAttacked`'s flee branch (`rules.go:744-746`, `unfought`, then `flee`),
  with its `Flee` skill (Dash, Sprint) and no `Break` control step: the server refuses an
  `isDebuff` skill on a clean player even with ctrl ("Server behavior", Forcing). Today `unfought`
  returns only a blacklisted attacker and holds against a player the policy leaves alone
  (`combat_rules.go:105-120`); X11 makes it also return, while the bot's karma is above 0, a live
  player in the attackers table that `MayFight` refuses. The attackers table holds that player's
  `Attack` hits and its hostile casts alike (section 2: an offensive skill aimed at the bot, from
  any player), so a clean mystic's nukes make the bot flee as a clean fighter's swings do.
  It does not stop to fight when it cannot outrun the attacker: it keeps fleeing and takes the
  death rather than a PK past the limits. That death costs `RateKarmaExpLost` times the normal
  loss but also lowers its karma by the karma-loss formula, and its items cannot drop, since
  `PKCap` keeps its PK kills under the server's drop limit ("Server behavior", Death costs, Death
  drops). The attacker is then avoided like any player who killed it (`AvoidAfterDeath`).
- **Flagged and karma attackers** are fought as rule 6 allows, the flag guard included: killing a
  flagged or karma player is no PK whatever the killer's karma (`character_karma.go:44-56`).
- **Death and restart.** It accepts a resurrection as today. A restart to the village lands at a
  chaotic point outside the town ("Server behavior", A karma player), not in a town, so the death
  policy expects that: no town trip and no M8 death penalty or supply trip until its karma is 0.
  It walks back from the chaotic point to its spot (M7 walker) on a **karma route**: on foot inside
  its SCC, with no gatekeeper hop (a gatekeeper, like a shop, refuses a karma player under the
  shipped config: "Server behavior", Karma and NPC services), and with every segment at least
  `KarmaClear` (600, the guards' 450 see range and a margin) outside every town polygon (X2) and
  from every guard spawn (the spawn list's NPCs whose template type is `Guard`,
  `internal/datapack/npc.go:13-15`), since guards attack it on sight and a guard death is a death
  with karma ("Server behavior", Guards). When its spot has no such route, it farms the nearest
  spot of its level band that has one and returns to its own spot once its karma is 0. The flee
  of "Clean attackers" picks its walker segments under the same rule (a heading whose segment
  enters that margin is skipped like one the geodata blocks).
- **Work-off.** It farms monsters at its spot (each kill's experience lowers its karma) and starts
  no hunt, challenge or arena trip (a karma player cannot duel, and a karma bot in an arena is
  everyone's target). When its karma reaches 0 it resumes its modes and its town trips.

Outside PK mode the bot does not get karma by its own choice: section 3 keeps it off clean
players, stops every damaging attack and every `nextActionAttack` skill on a flagged karma-free
opponent the moment its flag blinks, cancels the bot's own such cast in flight and drops the
server's follow-up swing with a move, starts such a step on a karma-free arena opponent only when
the opponent cannot reach the arena's edge before it lands and stops as at a blink when it nears
the edge, leaves an arena only with nothing of its own in flight, and keeps its area damage and
pole swings off any flagged karma-free bystander and any summon showing a flag without karma
(a pole wielder repositions or holds, since every pole step swings), and fights a summon only by
its own current flag or karma, never by a stale `NPCInfo` `Attackable`;
DOTs cannot kill. What remains is timing the bot
cannot see: a flag or move packet delayed by seconds of lag, or an arena opponent that speeds up
inside the last 200 ms of the bot's cast ("Risks").

### 14. Panel and metrics

The panel shows a `pvp` bot's modes as switches (open-world PK among them, off), its PvP and PK
kill counts (`UserInfo`) and its duel results. The PvP count is the server's: under the shipped
`AwardPKKillPVPPoint = False` the bot's kills of karma players do not raise it, and a bot whose
kills are mostly karma players shows few; the fights counter below counts those fights. New
counters:

- `bot_pvp_fights_total{kind}`: one per opponent, at the bot's first attack on it, under the kind
  its opponent record carries, set by the rule that allowed the fight: `duel` (the duel opponent,
  section 3 rule 1, X4), `fight_back` (an attacker `MayFight` allows, X4), `arena` (an arena
  target, X5), `flagged` (a hunted flagged or karma player, X5), `pk` (the PK-mode victim, X11);
- `bot_pvp_deaths_total{kind}`: one per death to a player or its summon, under the kind on the
  killer's opponent record (`fight_back`, `arena`, `flagged`, `pk`, or `unfought`: an attacker
  `MayFight` refused, such as a clean attacker the bot fled from with karma (section 13) or one it
  held against; X4 sets `unfought` on such an attacker's record); a duel defeat leaves 1 HP and is
  counted in `bot_duels_total`;
- `bot_duels_total{result}` (won, lost, tie), from the duel state's result, which X4 reads from
  the end messages before `ExDuelEnd`: 1950 (S1 won the duel) with the bot's own name → won,
  with the opponent's → lost (a 1955 withdrawal comes before the 1950, which decides), 1952 (a
  tie) → tie. The bot cannot tell a cancel from a timeout, since both end in the same tie message
  (`network/duel.go:513-539`, `duelEndMessages`; `serverpackets/systemmessage_duel.go:22-25`);
- `bot_pvp_interrupts_total{result}` (cast, landed, too late), from X6's interrupts.

## Pacing

| New sender | What bounds it |
| --- | --- |
| `RequestDuelAnswerStart` | One answer per `ExDuelAskStart`: an acceptance after the PvP reaction delay, inside the server's 15 s request window; a decline (another preset, a bot-marked challenger, a party duel, a failed pre-check) at once. No answer is ever repeated for one challenge. |
| `RequestDuelStart` | One per `ChallengeEvery` (3 min) per bot, own and whispered challenges together, and one per player per `ChallengeSameEvery` (30 min), or per `DeclineHold` (2 h) for its own challenges; the server keeps one open request per target. No server gate covers it, so these persona bounds and the frame bucket do. |
| Hunting attacks, control, interrupts | Casts as today: one in flight, the next only after its `MagicSkillUse` or `castTimeout`, then the skill's reuse; an interrupt is a cast like any other, at most one per opponent cast. Melee clicks once per target. |
| Kite moves | Walker segments as today (one checked segment at a time, from standstill). |
| CP potions | `UseItem` at most once per 500 ms item reuse, under the bot's potion reuse. |
| Whispered `duel` | Answered by a challenge, not chat; the chat policy's per-sender limit still applies to anything said. |
| Arena trips | Walker segments and gatekeeper bypasses as today (`bypassGap`, reply timeouts). |
| Esc cancel (`RequestTargetCancel`) | At most one per own damage or `nextActionAttack` cast in flight, only when the opponent's flag (or, for a summon it fights, the summon's own flag) turns to state 2 (the server's 1 s flag task changes it at most once a second), when a karma-free arena opponent comes nearer the edge than its reach rule allows or a root the bot counted on ends early, or before the bot leaves an arena. An Esc that reaches the server after the cast ended clears the bot's target (`TargetUnselected`), which the bot expects. |
| Pole reposition (area rule) | One disengage move when a bystander enters the pole's reach, then one walker segment to the chosen stand point (a walker segment as today); a new stand point is chosen only after the bot reached the last one or a bystander moved, and at most once per attack interval. A hold sends nothing. |
| Re-select after a late Esc | One `Action` (a selection, never a second click that would attack) per `TargetUnselected` that follows its own Esc, sent only before its next step on that opponent; at most one per Esc. |
| Disengage after a `nextActionAttack` cast | One move per such cast in flight at the blink or the arena-edge stop, sent once the cast ends (a walker segment as today). |
| Arena-edge stop and leaving | One move inside the polygon per stop (the bot then waits for the opponent to come back inside or for one attack interval before walking out), a walker segment as today; a new stop only after the opponent was back inside by the margin. |
| Cure items | `UseItem` at most once per poison or bleed effect seen on itself, under the 500 ms item reuse and the bot's potion reuse. |
| Duel-ground disengage | One move (a checked step or `orStay`) when the duel-ground check first fails while the bot has an attack or cast intention on its opponent; the next only after a new `Action` or cast re-created the intention, which the bot sends only once the check passes again, so at most one per crossing; the frame bucket bounds the rest. |
| Target refresh | None: the bot never re-selects a target to read its HP. |
| Fleeing with karma | Today's `flee` (one walker segment at a time) and the `Flee` skill under its reuse; a fight back as a PK only under `PKEvery` (1 h) and `PKCap`. The walk-back on the karma route sends walker segments as today and no gatekeeper bypass. |

All of it goes through the per-connection frame bucket (`pacing.go` `frameRate` 5 a second,
`frameBurst` 10: at most 15 frames in any second and 35 in any 5 s, against the server's flood
limits of 160 a second and 400 in 5 s). A PvP fight's sends (a cast, an interrupt, a CP potion, a
cure, a kite move, an Esc cancel) share those 5 frames a second with everything else the bot sends,
and the brain still sends at most one of them per decision, in the order of section 5 with the Esc
cancel first, never assuming a larger budget.

## Owner decisions

Proposed, awaiting the owner:

| # | Question | Proposal |
| --- | --- | --- |
| D35 | Which PvP modes exist, and which are on by default? | A new `pvp` preset (M10 Damage role, farms between fights) with duels, flagged and karma players, and arenas on, and open-world PK off (opt-in per bot on the panel). Other presets keep today's modes (fight back only, `SupportFlagged` off) and now decline duels at once, but every preset fights a player with the PvP layer (sections 3, 5-11 and its own row), so the KN, CL, EO and SO rows, which only the Support role picks, are played by `support` bots fighting back, and party PvP runs in any party a bot is in, whoever leads it: a `support` or `pvp` bot in a player's party now, and M11 bot parties of `farmer` and `support` bots once M11 lands. A `support` healer keeps `SupportFlagged` off, so in the open world it stops healing a member flagged by fighting back unless the owner switches it on per bot. All persona defaults under D6. |
| D36 | How does a `pvp` bot find fights? | It takes any duel it can take (clear of monsters, since any hit between a duellist and an NPC cancels the duel; during the duel its own moves stay off the ground that cancels one: peace, town, Derby track, arena and siege polygons, and past 1800 from the opponent, and a move stops the server's follow of the opponent before it would lead there); challenges a player who whispers `duel` (extends D3 for this whisper, as the approved D20 did; at most once per 30 minutes per player, an earlier decline not counted); challenges by itself at most one non-bot player of its class tier in sight every 3 minutes, only when the engage estimate passes against that player, never the same player within 30 minutes or within 2 hours of a decline or refusal of any challenge to it; hunts flagged and karma players of its class tier when the engage estimate passes; visits the nearest arena every 90 minutes and stays while others are there (20 minutes at most). |
| D37 | How does PvP change the bot's items? | One loadout scored by a blend of PvE and PvP value: the relative gains of M10's `Value` and of `PvPValue` (the expected share of won fights over the seen class mix), weighted 50 % each for the `pvp` preset and expressed in M10's kills per minute (others 0 % PvP, so exactly M10's score); no PvP set swap. PvP sets and boss jewels are worn when owned and within grade, never bought. The `pvp` preset buys poison and bleed cures (Antidote, Bandage, and their greater kinds from level 20) on its M8 supply trips and uses them in PvP fights. CP potions are kept for PvP and never sold; none are bought (their only shop takes Ancient Adena). |
| D38 | May bots fight bots? | Outside an arena a bot never attacks, hunts or challenges another bot, and fights one back only when it was attacked by it (an `Attack` hit or an offensive skill cast at the bot, section 2). Inside an arena it fights anyone there but its party members (bots, clan mates and allies included, as the server's arena rule allows), so two `pvp` bots in an arena make a fight for players to watch. Bots never duel each other: a bot declines a bot-marked challenger and never challenges a bot. |
| D39 | How does the bot treat death and karma? | Duels: fight to the end, never surrender. Arenas: fight to the death; after each of the first 4 deaths in an hour walk back, after the 5th go back to its spot; start a damaging step on a karma-free arena opponent only when it cannot reach the arena's edge before the step lands (a root the bot landed counts as holding it), stop as at a blink when it nears the edge, and leave only with nothing of its own in flight (an arena opponent who steps out is a clean player). The cost, stated for the owner: in the Dion, Gludin and Giran arenas (about 1000 × 1000) steps with a scaled hit time above about 3 s almost never start against an unrooted karma-free opponent. With a spiritshot charged (which multiplies a magic cast's time by 0.70) the Elven and Human Mystics' 4000 ms nukes and drains take 2.4-2.8 s and still start in the middle part of those arenas, the Dark Mystic's 3.0 s ones only at the middle; the Orc Mystic's (3.6 s), any mystic's without a spiritshot, Drain Health, Power Shot and Stunning Shot do not, so the Orc Mystic row does little there but DOTs and melee and the bow rows lose their skill shots; the Colosseum allows them near its middle. Open world: break off by the row's threshold; avoid hunting a player who killed it for 30 minutes. Outside PK mode, never karma by accident: every damaging attack and every skill that hands over to an auto-attack (`nextActionAttack`) on a flagged karma-free opponent stops when its flag blinks, such a cast in flight is cancelled and its follow-up swing dropped with a move, and no area damage or pole swing is used while a flagged karma-free bystander, or a summon showing a flag and no karma, is in its reach (the server's reach: radius plus both collision radii; in an arena, any karma-free player but its target not at least 150 inside, and any summon whose owner is not seen inside); a pole wielder, which has no single-target step, then moves to a stand point clear of the bystander or holds; a summon is fought only while its own flag or karma, kept current by `RelationChanged`, allows it, under the same flag guard, and never in a duel. In a duel, no attack at all during the countdown (any hit before both duellists fight cancels the duel), then no area skill or pole swing near a third player, a summon or an NPC (their collision radii counted), no hit on an NPC, and no support on a flagged player, each of which would cancel the duel; duels are taken and offered only clear of monsters. PK mode (opt-in): one PK an hour at most, only a clean player of its tier, chosen on the hunting preconditions (its HP and MP above its stand levels, neither in a peace zone, town or Derby track, the engage estimate passing), with plain and ctrl-held non-debuff damage only until the victim flags, never at 4 PK kills or more (one under the server's item-drop limit). With karma (after a PK, or an accident outside PK mode): it farms monsters until karma is 0 and keeps out of towns; a clean player who attacks it stays clean and killing it would be another PK, so the bot fights it back only when PK mode is on and the PK limits pass (that kill is its next PK), else flees and, if it cannot outrun the attacker, takes the death (karma loss, no item drop under the cap) rather than a PK past the limits; it restarts at the server's chaotic point outside town and walks back to its spot on foot, using no gatekeeper or shop (both refuse a karma player under the shipped config) and keeping 600 from towns and guard spawns (a guard death costs the same loss), and farms a nearer spot when its own has no such route; its flee keeps the same distance; no hunt, duel, arena or town trip until karma is 0. |

## Testing

- **Codec (unit):** `ExDuelAskStart`, `ExDuelReady`, `ExDuelStart`, `ExDuelEnd` and
  `ExDuelUpdateUserInfo` decode from frames built in the server's layout and consume exactly
  `len(payload)`; `MagicSkillCanceled` decodes from a `FrameMagicSkillCanceled` frame and consumes
  exactly `len(payload)`; `RequestDuelStart` and `RequestDuelAnswerStart` encode the server's field order;
  `CharInfo`, `NPCInfo` and `UserInfo` keep their new fields (the collision radii among them) over
  the recorded fixtures, still consuming exactly `len(payload)`.
- **Datapack (unit):** the zone reader for the arena, peace, town, Derby track, siege
  (`SiegeZone.xml`, castles and clan halls) and no-restart files on fixture files, a position in and out of each: an `NPoly` zone with `minZ`/`maxZ`, a
  `Cuboid` zone given with its corners reversed (as Rune Castle Town is, `zones/TownZone.xml:445-449`:
  x 47150 then 32531, y -44815 then -52045) contains a point between them, any other shape fails
  the load; over the shipped `TownZone.xml`, 18 towns with Rune among them; over the shipped
  `SiegeZone.xml`, 15 siege polygons (9 castles, 6 clan halls); the water zones stay
  on today's `LoadWaterZones` and its tests; the skill landing fields
  (`isDebuff`, `isMagic`, `lvlDepend`, `magicLvl`, effect powers and types, `abnormalLvl`,
  `effectAbnormalLvl` (Sting 223 read with level 3 at skill level 1 and 8 at 45, and `abnormalLvl`
  -1) and stack types), `nextActionAttack` and the `offensive` flag (its default derived from the skill type),
  skill target types and `skillRadius`, the cure skills' `negateStats` and `negateLvl`, the jewel
  item skills' vulnerability functions and the weapon Special Abilities' `pvp*Dmg` multipliers;
  `cpTable`.
- **Model (unit, table-driven, X3, on `RowTable`s built in the test):** `floor(LandRate)` equal to the section 6 table, with the target's
  level equal to the skill's `magicLvl`, so the level term is 1 + 0.005 × `lvlDepend` (whole
  percent, rounded down; for example Human Fighter Sleep 70 × 0.8686 × 1.01 = 61.41 → 61, Human
  Mystic Shield Stun 80 × 1.0100 × 1.01 = 81.61 → 81, Orc Fighter Stun Attack 50 × 0.6696 × 1.005 =
  33.65 → 33), and `LandRate` equal to the server formula within 1e-9 for fixed inputs (a vuln
  jewel, a blessed spiritshot, a level gap); the land base of a PDAM, an MDAM, an effect-power and a
  level `effectPower` skill; the cast-break chance; the archetype and the PvP row of every class id
  0-57 and 88-118 with each right-hand weapon kind (the Artisan line, 56, 57 and 118, maps to SC's
  row: section 8); reference opponents run their table's steps under the `Vs` gates against the
  bot's archetype (section 12): a test-built row whose only DOT is a Poison 129 step gated
  `vs C, H` lowers a CL bot's `PvPValue` and leaves a WA bot's unchanged (within 1e-9), the same
  step ungated lowers both, and a row missing from the table plays only its plain attacks;
  `win_r` is 0.5 for a reference
  opponent equal to the bot, and `PvPValue` is the share-weighted sum of `win_r`; the blended gain
  with `w` 0 equals M10's `ΔValue` exactly, and with `w` 0.5 a candidate giving +10 % `Value` and
  no `PvPValue` change scores the same as one giving +10 % `PvPValue` and no `Value` change; a
  1.05 `pvpPhysDmg` weapon scores above the same weapon without it; Earring of Antharas raises
  `u_r` against a test-built stun row.
- **Model over the shipped rows (unit, X10, which depends on X7 and X8):** `pvpRowTable` holds
  every section 10 row with its damage, DOT and control steps and their `Vs` gates; `PvPValue` of
  the Mithril Heavy Set with its set skill 3502, worn by a healer bot (a CL), scores above the same
  pieces without it against a mix of the poison rows (HM, DM, SO, DW, PK, AS, OM: section 12); worn
  by a melee bot (a WA) it scores equal to them (within 1e-9) against the same poison-row mix,
  since every poison step there is gated `vs C, H` or `vs H`; and for either bot it scores equal
  against a mix with no poison row (HF, WA, KN, HW, CL), since 3502 adds only `poisonVuln`
  (`skills/3500-3599.xml:22-30`). The mix table: a class seen 59 minutes ago counts, one seen 61
  minutes ago does not, a bot-marked player is not counted, and with no one seen the shares are
  equal over the rows of the bot's tier.
- **Brain (unit, events and an injected clock in, packets out).** Each test names the PR that
  hosts it. A PR tests its mechanic on a `Step` list built in the test, so X4 and X6 need no
  shipped PvP row; a case that names a shipped row (KN, WA, EF, CL, OM, HM, ...) runs in the PR
  that ships the row (X7 fighters, X8 mystics), and a case on the supply list or the junk rule runs
  in X10:
  - `TestPvPDuelAccept` (X5): `ExDuelAskStart` to a `pvp` bot at full HP and MP out of combat →
    `RequestDuelAnswerStart` accepted after the reaction delay; under half MP → declined at once; a
    `farmer` bot → declined; a party duel → declined; the bot in a town, an arena, water or a
    no-restart zone → declined at once; an aggressive monster in sight within its aggro range plus
    300, or a monster in its attackers table → declined at once, and no own challenge; after a
    1940 while its own pre-checks pass → declined, and no own challenge, until it has moved 2000
    away or 10 minutes have passed; after a 1940 while it is in combat stance (`AutoAttackStart`
    without `AutoAttackStop`) → no hold: accepted once the stance ends; a whispered `duel` from
    a player standing in a town → no challenge; a clan mate's or a party member's one-on-one
    challenge → accepted; a bot-marked challenger → declined at once.
  - `TestPvPDuelFight` (X4): during the countdown (1945 each second) no `Action`,
    `AttackRequest` or cast on anyone, the opponent included; the opponent hits the bot in the
    countdown, with an `Attack` hit or a Wind Strike `MagicSkillUse` at the bot → not answered
    (`MayFight` refuses it, whatever the attackers table holds), then 1952 and `ExDuelEnd` → the
    unflagged player is not fought (the ordinary rules); `ExDuelStart` alone → still no attack;
    the opponent's first `ExDuelUpdateUserInfo` and the reaction delay → the unflagged opponent
    gets a plain `Action` and plain casts (never `AttackRequest` or a ctrl-held cast); its `MagicSkillUse` at the bot enters the attackers table; an opponent who is
    a clan mate or a party member is attacked the same way; 692 or `ExDuelEnd` stops the attack;
    no surrender is ever sent; the duel result: 1950 with the bot's own name, then `ExDuelEnd` →
    won, with the opponent's name → lost, 1955 then 1950 with the bot's name → won, 1952 → tie;
    an attacker `MayFight` refuses → its record's kind `unfought`. During the duel a monster in sight is never targeted, a monster
    that attacks the bot is not fought until 1952 and `ExDuelEnd` arrive and is fought after; a
    test-built area step and a pole with an attackable monster in reach → neither is used.
    Duel ground: a pole wielder repositioning around a third player, whose nearest stand points
    lie inside a town polygon or less than 100 outside it → those skipped, the walk to the first
    point at least 100 outside; every stand point inside or near a town polygon, an arena
    polygon or a siege polygon the bot is not in, or more than 1800 from the opponent → no walk,
    the disengage move to the first heading that passes, or to the bot's own position when none
    does, then the hold; the opponent standing 30 beyond the edge of a peace polygon, its
    straight segment from the bot crossing within 100 of that polygon → no `Action` and no cast
    out of range, the bot's in-range steps only, then the opponent back 200 from the polygon →
    `Action`; the bot already attacking (attack intention on, the opponent out of reach and
    walking), the opponent's moves then taking the straight segment within 100 of a town polygon →
    the checked disengage move at once, no `Action` while the segment fails, then `Action` once it
    clears; the same with the segment clear but its own `MoveToLocation` leg (the server's follow)
    ending 60 from an arena polygon → the disengage move; the bot's own position seen within 100
    of a peace polygon while it attacks → the disengage move to the first heading leading out.
  - `TestPvPChallengeWhisper` (X5): a player's whispered `duel` → `RequestDuelStart` with its name;
    the same within 30 minutes → nothing; a bot-marked name → nothing; the target flagged → nothing;
    a second player's whisper within 3 minutes of the bot's own challenge, or of the first
    whispered one → nothing, the same whisper after 3 minutes → `RequestDuelStart`; a player who
    declined the bot's own challenge (1931) whispers `duel` 31 minutes later, inside its 2-hour
    `DeclineHold` → `RequestDuelStart`; the bot's whispered challenge refused with 2019 (the
    target's HP or MP under half) → no own challenge of that player for 2 hours, and the player's next
    whisper 31 minutes later → `RequestDuelStart`.
  - `TestPvPChallengeCooldown` (X5): one self challenge per 3 minutes; after 1931, 153 or 1926 that
    player waits 2 hours and the duel state is back to idle at once; on a `charmodel.RowTable`
    built in the test, a candidate whose row the engage estimate says the bot loses to → no
    challenge, one it says the bot beats → `RequestDuelStart`; the losing candidate's whispered
    `duel` → still `RequestDuelStart` (a whisper needs no estimate).
  - `TestPvPHuntFlagged` (X5): a flagged player of the bot's tier in sight → attack; a flagged
    clanless player while the bot is clanless too (both clan ids 0) → attack; a flagged party
    member, clan mate, ally, other-tier player or clean player → none; in a peace zone, a town or
    the Derby track → none; during its own duel, a flagged third player → none.
  - `TestPvPArenaAttackable` (X4): both positions in the Gludin arena polygon with a stale
    `RelationChanged` → plain attack; the player walks out → disengage; the karma-free opponent
    100 from the edge with run speed 120 → no Wind Strike (hit 4000) started, a pure DOT still
    cast; the bot a Human Mystic (cast speed 333 in `UserInfo`) with no spiritshot auto-use
    reported, the opponent at the Gludin arena's middle (about 490 from the edge) with run speed
    124 → no Wind Strike (4.0 s: 124 × 4.5 = 558 > 490), a test-built 2000 ms damage step started;
    the same with the auto-use of its blessed spiritshot reported on (`ExAutoSoulShot`) and a stack
    carried → Wind Strike started (2.8 s: 124 × 3.3 ≈ 409 < 490), and at 380 from the edge → not
    started; that Wind Strike started at 490 whose own `MagicSkillUse` carries 4000 ms (no shot
    was charged), arriving 0.1 s after the send while the opponent is 450 from the edge →
    `RequestTargetCancel` at once (3.9 s left: 450 < 124 × (3.9 + 0.5) ≈ 546); the shot-less bot with the opponent under its
    own Dryad Root (landed, no 139, mask bit 0x200, 20 s left) → Wind Strike started; a `CharInfo`
    clearing the root bit while that Wind Strike has 2.5 s left until it lands and the opponent,
    run speed 124, is 300 from the edge (300 < 124 × (2.5 + 0.5) = 372) → `RequestTargetCancel`;
    the same with 0.4 s left, still inside the cancel window (300 > 124 × (0.4 + 0.5) ≈ 112) → no
    cancel, the cast lands; the opponent nears the edge while the bot's Wind Strike is in its cancel window →
    `RequestTargetCancel`, then a move to a point inside the polygon at least 150 from the edge;
    back inside by the margin → attacks resume; a karma opponent at the edge → the fight goes on;
    a clan mate or an ally in
    the arena → attack; a party member in the arena → never; an unflagged player in the arena
    casting a heal or a buff on the bot → not in the attackers table, the same player casting Wind
    Strike on it → in it.
  - `TestPvPArenaTarget` (X5): the bot at the Gludin arena's middle, nobody in its attackers
    table, its HP and MP above `StandHP` and `StandMP`: players of its tier 100 and 250 from it
    (both at least 240 from the edge) → `Action` on the one at 100; a level-1 base class player
    50 from it (the bot a first profession) → not picked; with only a player of its tier 120 from
    the edge (under `ArenaEdge`) → no pick; a party member → not picked; the bot's MP under
    `StandMP` → no pick, it rests; while it attacks its pick, which has not hit it, the player at
    250 hits it → it fights back that player; a player of its tier who just walked in, the only
    candidate, 300 inside the edge and idle → picked like any other. `ArenaStay` reached with its own damage cast in flight → the
    cast cancelled, a move inside, one attack interval, then the walk out (never a walk out first).
  - `TestPvPBotsOnlyInArena` (X5, D38): a bot-marked flagged player outside an arena is not hunted,
    is fought back once it hits, with an `Attack` hit and, in a second case, with only a Wind
    Strike `MagicSkillUse` at the bot (no `Attack`); a Heal `MagicSkillUse` at the bot → still
    not fought; inside an arena it is fought.
  - `TestPvPNoKarmaFlagEnding` (X4): the bot casting Wind Strike (hit 4000) at a flagged karma-free
    opponent whose `RelationChanged` turns the flag to 2 → `RequestTargetCancel` with `unselect` 0,
    then no damage cast and no `AttackRequest` or `Action` on it, a move dropping the auto-attack;
    a pure DOT or a root still allowed; the flag back to 1 → attacks resume; the flag gone → no
    attack; the same blink on a karma opponent, in a duel or in an arena → the fight goes on; an
    opponent first seen in a `CharInfo` with PvP flag 2 (no `RelationChanged`) → no damage attack.
    The follow-up swing, on test-built steps that carry `nextActionAttack` (one PDAM like Power
    Strike, one STUN like Shield Stun, one BLEED like Bleed): after the blink none of them is
    started, the STUN and BLEED ones included; one in flight inside the cancel window →
    `RequestTargetCancel`, then on its `MagicSkillCanceled` a move as the next send; one past the
    window → no cancel, and a move as the first send once its hit time has passed; a pure DOT with
    no `nextActionAttack` → still cast, and no move follows it. A late Esc: `ActionFailed` and
    `TargetUnselected` instead of `MagicSkillCanceled` → the move, then one `Action` selecting the
    opponent before the next DOT step, never a second `Action`.
  - `TestPvPAreaBystander` (X4 on a test-built area step and pole; the WA case below in X7). The
    fixtures set every collision radius: the bot, its target and the bystander are male Human
    Fighter lines with radius 9 in their `UserInfo` and `CharInfo`, unless a case names another. A WA
    with a pole fighting a flagged karma-free opponent in the open world, a second flagged
    karma-free player within the pole's reach (40 from the bot, inside the cone it faces) → no
    Wild Sweep, no swing and no `nextActionAttack` step (a test-built PDAM step carrying it, with
    no weapon gate, is not cast): a move that drops the auto-attack, then a walk to a stand point
    within the pole's range of the target from which the bystander is outside the cone toward the
    target, where `Action` resumes the swing; bystanders on both sides of the target, so that one
    is in the cone from every stand point → no swing and no damage step, the self buff (War Cry)
    still cast, until one walks out; the same bystander with a BIGBLUNT instead of the pole →
    255 Power Smash and 100 Stun Attack, single-target steps; the same bystander 95 from the bot in
    the cone (inside 80 + 9 + 9) → no swing; the bot 40 from its target (Wild Sweep's cast range)
    and the bystander 165 from the target on its far side (inside 150 + 9 + 9; 205 from the bot)
    → no Wild Sweep, the swing allowed; the bystander 175 from the target → Wild Sweep allowed;
    a male Orc Fighter bystander (radius 11) 169 from the target → no Wild Sweep (inside
    150 + 9 + 11); a female Dwarven Fighter bystander (radius 5) 165 from the target → Wild Sweep
    allowed (outside 150 + 9 + 5);
    the bystander walks out or has karma → Wild Sweep allowed; a summon (`NPCInfo` with `Summon`
    set, radius 9) showing PvP flag 1 and karma 0, 165 from the target → no Wild Sweep, then its
    `RelationChanged` showing karma → allowed, and showing flag 0 and karma 0 → allowed; in an
    arena, a summon in reach whose title names a player seen outside the polygon → no Wild Sweep,
    one whose owner stands inside by more than 150 → allowed; in an arena, the target 160 inside
    the edge and a flagged karma-free player 165 from it just outside the polygon → no Wild Sweep;
    the same bystander unflagged and karma-free 100 inside the edge → no Wild Sweep; 200 inside →
    allowed; with karma, outside → allowed; in a duel, any third player,
    summon or attackable NPC in reach → no Wild Sweep, no swing, and an NPC with collision radius 30 at 115
    from the bot in the cone (inside 80 + 9 + 30) → no swing, the reposition move instead; in an
    arena → allowed.
  - `TestPvPSummon` (X4): a summon (`NPCInfo` with `Summon` set, `Attackable` true, PvP flag 1,
    karma 0, title naming a player out of sight) hits the bot → fought with plain `Action` and
    plain casts, never `AttackRequest` or a ctrl-held cast; its `RelationChanged` turns the flag
    to 2 → no damage or `nextActionAttack` step, the bot's own damage cast in flight
    Esc-cancelled, a move; then flag 0 and karma 0 while its `NPCInfo` `Attackable` is still
    true → not fought (held, as today's `unfought`); karma above 0 → fought; the owner (a
    `CharInfo` whose name is the title) in sight and flagged → the owner fought, not the summon;
    a summon whose title names a party member, flagged → never fought; during the bot's duel, any
    summon that hits it → not fought; in an arena with the owner seen inside by more than 150 →
    fought, the owner seen outside → not; a summon's Wind Strike `MagicSkillUse` at the bot → in
    the attackers table, a Heal → not.
  - `TestPvPDuelNoFlaggedSupport` (X4): a `pvp` bot with `SupportFlagged` on, in a duel, a flagged
    party member under its heal gate → no heal; an unflagged party member with karma under its heal
    gate → no heal; the same two after `ExDuelEnd` → heal; a PARTY buff with a flagged member, or
    an unflagged member with karma, in range during the duel → not cast. Out of a duel with
    `SupportFlagged` off: an unflagged karma member → no heal (today's gate would heal it); a clean
    member → heal; both in an arena → heal. A `pvp` bot duelling a clean, unflagged party member
    one on one, that member under its heal gate → no heal and no buff on it, and no PARTY skill
    while it is inside the skill's radius; the same member after `ExDuelEnd` → heal.
  - `TestPvPInterruptHeal` (X6 on a test-built stun step; the KN case below in X7, a `support`
    bot fighting back): KN with a shield
    against a Cleric casting 1011 Heal (hit 5000) on itself → Shield Stun before it lands; Shield
    Stun not cast at reuse against the Cleric, cast at reuse against a Warrior.
  - `TestPvPInterruptTooLate` (X6): a cast landing before the bot's own scaled hit time plus delay →
    no interrupt; a damage interrupt under `MinBreak` → none; a test-built magic sleep step (hit
    4000) at cast speed 260 against a cast landing 4.5 s after its `MagicSkillUse`: with no
    spiritshot auto-use reported → no interrupt (5.1 s), with the auto-use of a spiritshot on and a
    stack carried → the interrupt (3.6 s plus a delay of at most 0.6 s), with the auto-use on but
    no stack left → no interrupt.
  - `TestPvPLongCastRule` (X6 on a test-built heal step; the EF case below in X7): EF under 50 % HP
    with a melee opponent in reach → no Elemental Heal; the opponent's `CharInfo` abnormal mask
    shows a stun → Elemental Heal.
  - `TestPvPControlLandRate` (X6 on test-built steps; the Stun Attack case in X7, the Dryad Root
    case in X8): Stun Attack against an Orc Fighter (33 %) under a `MinLand` of 35 % → skipped;
    against a Human Mystic (50 %) → cast; two 139s on CL's Dryad Root → no third.
  - `TestPvPKite` (X6 on a test-built root step; the CL case below in X8, a `support` bot
    fighting back): CL's Dryad Root lands on
    a melee opponent → a walk to a stand point at 600, then the next cast; the same in a duel with
    the stand points at 600 inside or within 100 of a town polygon → those skipped, the first
    point that passes; none passes → no walk, the next cast from where it stands; a kiting caster
    in a duel whose opponent stands 1900 away → `Action` (the chase), no kite.
  - `TestPvPCPPotion` (X6; the junk-set case in X10): CP under 50 % in a PvP fight with 5592 carried
    → `UseItem`; in a PvE fight → none; in a duel → none, while a Poison 129 on it in the same duel
    → the cure `UseItem`; CP potions are not in the junk set.
  - `TestPvPBreakOff` (X6 on a test-built row; the OM case in X8, the Ultimate Defense cases in X7):
    open world, CP gone, HP under the row's bound, losing pool, faster than the opponent → the row's
    `Break` step, then flee; the same in a duel or an arena → fights on; an OM row with a melee
    opponent next to it whose hits would break Dreaming Spirit more often than not → no `Break`
    cast, flees at once; a test-built sleep `Break` step against an opponent whose `CharInfo` mask
    shows the poison bit, or on which the bot's own DOT step landed 5 s ago with ticks left → no
    sleep, the row's fear step instead (the OM case: Venom on the opponent → 1092 Fear, not
    Dreaming Spirit; below 14 → flees at once); the same opponent with no DOT → the sleep; a KN
    (a `support` bot fighting back) in the open world, CP gone, HP under 20 %, losing pool, that
    can outrun its melee opponent → flees, no Ultimate Defense; a KN that cannot outrun its melee
    opponent, or the same KN in a duel → Ultimate Defense; with Ultimate Defense on itself → no flee, no kite and no
    stand-point walk until it ends, while the disengage move still goes: in the open world its
    flagged karma-free opponent's `RelationChanged` turns the flag to 2 with the bot's test-built
    damage cast in flight → `RequestTargetCancel`, then the move (the server's `ActionFailed`
    answer to it changes nothing), then no damage step; in an arena the opponent nears the edge
    → the same cancel and move; `ArenaStay` reached under the effect → the stop move and one
    attack interval, and the walk out only after the effect's `AbnormalStatusUpdate` drops 110.
  - `TestPvPTargetHealer` (X9): a player's party (the leader a player, from the party packets:
    the GM commander, who may lead several support bots) of a `pvp` damage bot, a `support` KN
    and a `support` CL healer; two flagged enemies hitting them, one casting 1015 Battle Heal on
    the other → the `pvp` bot switches to the healer; the KN casts Aggression on the enemy hitting
    the CL or the leader; a test-built sleep step does not sleep the healer the damage bot is
    hitting, and sleeps a second enemy no member is hitting; the CL does not heal the flagged
    damage bot with its default `SupportFlagged` off, heals it with it switched on, and buffs
    Mental Shield on members against a caster. A `support` CL alone with a player leader → the
    same marking and healing rules. The same cases with a `farmer` damage bot in a bot-led party
    are added with M11 (X13, "Gaps").
  - `TestPvPArenaDeath` (X11): the first to the fourth death in the arena within an hour →
    restart, walk back; the fifth → back to its spot; a death after the first has aged past the
    hour → walk back again.
  - `TestPvPAvoidAfterDeath` (X11): killed by player X in the open world → X not hunted or
    challenged for 30 minutes, still fought back.
  - `TestPvPPKCap` (X11): PK mode on with 4 PK kills in `UserInfo` → no forced attack on a clean
    player; with 0 PK kills against an unflagged clean victim, a WA row → `AttackRequest` and
    ctrl-held 255 Power Smash, never 100 Stun Attack, and an HM row → ctrl-held Wind Strike, never
    Ice Bolt or Curse: Poison; the victim flagged → the full PvP order; karma above 0 → no town
    trip, farming chosen. With 0 PK kills and no PK in the last hour, no attack on a clean victim
    when the bot's HP is under `StandHP`, when the victim or the bot stands in a town polygon, or
    when the engage estimate fails (a test-built `RowTable`); each of those removed → the attack.
  - `TestPvPKarma` (X11): the bot with karma 240 in `UserInfo`, PK mode off, hit by a clean
    player → flee (no `Action`, `AttackRequest` or cast on it), no `Break` control step, Dash if
    ready; the same with a clean mystic that only casts Wind Strike at the bot (`MagicSkillUse`,
    no `Attack`) → flee; a bot with karma 0 and a clean player's Wind Strike `MagicSkillUse` at it
    (a ctrl-held cast) → in the attackers table, held, not fled (today's `unfought`), then fought
    once that player's `RelationChanged` shows its flag; it cannot outrun the attacker → it keeps fleeing, never fights; PK mode on, 1 PK kill,
    its last PK 2 hours ago → fights back with `AttackRequest` and ctrl-held non-debuff damage,
    and the PK clock restarts; PK mode on, last PK 30 minutes ago, or 4 PK kills → flee; a flagged
    attacker → fought, the flag guard included; a karma attacker → fought. Dead with karma → the
    village restart sent, then a position outside any town polygon expected: no town trip, a walk
    to its spot, farming; that walk on a test map where a gatekeeper hop is shorter and the
    straight way crosses a town polygon → a walk-only route, no `Action` or bypass on the
    gatekeeper, every segment at least 600 outside the town polygon and from the guard spawns; a
    spot with no such route → the nearest spot of its band that has one, and its own spot again
    after a `UserInfo` with karma 0; a flee from a clean attacker whose away heading enters a town
    polygon's margin → the first heading that keeps out of it; no hunt, challenge (declined at once) or arena trip until a `UserInfo`
    with karma 0, then its modes and town trips resume.
  - `TestPvPCure` (X6; the supply-list case in X10): in a PvP fight, the bot's
    `AbnormalStatusUpdate` shows Poison 129 at abnormal level 3 with most ticks left → `UseItem`
    1831; Bleed at level 5 → 1834; level 8 → none; Sting 223 at skill level 1 (`effectAbnormalLvl`
    3) → 1833, at skill level 20 (level 5) → 1834, at skill level 45 (level 8) → none; a test-built
    poison skill with neither level set → none; against a monster → none; the `pvp` supply list
    holds 1831 and 1833, and from level 20 1832 and 1834.
  - One `TestPvPRow<ID>` (X7, X8) per row of section 10, each checking the row's key PvP behavior
    (its opener against the named archetype, its `Hold` and `Int` step, its `Break` step).
- **e2e (live harness), at most 4 characters a scenario, the GM included:**
  - `TestLivePvPDuel`: a second client challenges a `pvp` bot; it accepts, fights, the duel ends
    without a death and both get their vitals back (GM, client, bot).
  - `TestLivePvPFightBack`: a second client force-attacks a `pvp` bot; it fights back; the client
    stops and unflags and the bot drops it (GM, client, bot).
  - `TestLivePvPArena`: two `pvp` bots of fighter rows and a client in the Gludin arena fight (a
    base mystic row's long nukes are held by the arena-edge rule there, section 3); a bot's death
    there leaves its experience unchanged and it walks back (GM, client, two bots).
  - `TestLivePvPHealer` (needs M11 bot parties, since a bot must lead the two bots' party within
    the 4-character cap): in the open world an M11 bot party of a `farmer` bot and a `support`
    Cleric bot (its `SupportFlagged` switched on for the test) is attacked by the GM, and a client
    heals the GM (which flags the client); the farmer fights back and targets the client, and the
    Cleric heals the flagged farmer (GM, client, two bots).

## Delivery (milestone M13, "PvP persona")

Each PR is one reviewable change; dependencies name M10 issues by number where they apply.

| PR | Scope | Plan sections | Depends on |
| --- | --- | --- | --- |
| X1 | Wire: duel packets (decode and encode), `RequestTargetCancel`, the `MagicSkillCanceled` decoder and event (the bot's own cast ended, another caster's cast cleared), `CharInfo` paperdoll hands, collision radius, clan and ally ids, abnormal mask, max CP and CP, and duel team byte, the `NPCInfo` collision radius and title, the `UserInfo` collision radius, ally id, PK kills and PvP kills, the SystemMessage ids; codec tests. | 2 | S1 (#320), which changes the same `UserInfo` decoder |
| X2 | Datapack: a zone reader for the arena, peace, town, Derby track, siege (castles and clan halls) and no-restart zones that takes both shapes they use, `NPoly` (all of them but one) and `Cuboid` (Rune Castle Town, `zones/TownZone.xml:445`, its corners and Z band normalized as `NewCuboid` does) and fails the load on any other shape; the water zones reuse today's `LoadWaterZones` (every shipped water zone a `Cuboid`); skill landing fields (`isDebuff`, `isMagic`, `lvlDepend`, `magicLvl`, effect powers and types, `abnormalLvl`, `effectAbnormalLvl` (both default -1) and stack types), `nextActionAttack` and the `offensive` flag, skill target types and `skillRadius` (the area rule), the cure items' negate fields, jewel item skills' stat functions, the weapon Special Abilities' `pvp*Dmg` multipliers, `cpTable`. | 2, 3, 4, 6, 9, 12 | S2 (#321) |
| X3 | `charmodel` PvP: `LandRate`, cast-break chance, the `Archetype` type and class archetypes, the class-to-PvP-row map, the `RowTable` input (`RefStep` with its `Vs` gate) and a reference opponent per row built from it (plain attacks for a row the table lacks), `PvPValue` over a row mix and a `RowTable`, the engage estimate (with the row's spot margin); its tests on tables built in the test. No row data: the rows stay in `internal/botclient` (section 12). | 6, 8, 12 | S3 (#322), S11 (#330), X2 |
| X4 | World and policy: the opponent record (with the fight kind section 14 counts by, `unfought` for a refused attacker), the duel and arena states (the duel result, won, lost or tie, read from 1950 and 1952), `MayFight` per section 3 (bots, clan, ally, tier, peace flag, duel opponent only and no NPC in a duel, the no-karma guard with its Esc cancel and the follow-up swing rule, the arena-edge rule with the arena edge distances, the area and duel-support rules with NPCs in reach, the duel-ground check of section 4 on every move it sends in a duel, its hold on the chase and the disengage that stops the server's follow), the `PvPPolicy` modes, the `pvp` preset (D35, D38); the five changes to today's code of section 2 (`forced`, `hostileCaster` with the offensive-skill test, `FlagBlinkAt` from `CharInfo`, `MaySupport` with karma and without the duel opponent, summons by their own flag and karma instead of `Attackable`), the pole reposition and hold. | 1, 2, 3 | X1, X2, X3 (the class-to-archetype and tier map the opponent record and the tier rule read), S4 (#323, the `Step` type), S6 (#325, the rotation runner whose step in flight the guard and the arena-edge rule cancel, scale and classify), S9 (#331) |
| X5 | Finding fights: duel answer, whispered `duel`, own challenges with their holds (the unexplained-1940 hold among them), the clear-of-monsters check, hunting with the engage estimate (given an empty `charmodel.RowTable` until X6 passes `pvpRowTable`), arena trips with the arena target choice (section 3, "Arena targets") and the arena-edge leave after `ArenaAlone` or `ArenaStay` (D36); `TestPvPArenaTarget`. | 4 | X3, X4 |
| X6 | The PvP layer: the new `Step` fields, the PvP order, the long-cast rule, the sleep-and-damage rule, interrupts, control by land rate, kite (its stand points under section 4's duel-ground check), the pool estimate, CP potions, cures, break-off with the break-cast gate and `PvPBreakHP` by the row's spot margin; `pvpRowTable` (the shipped rows as a `charmodel.RowTable`, empty until X7 and X8), passed to X5's engage estimate in place of the empty table X5 ships with; `TestPvPRowsShipped` with no row yet, which X7 and X8 extend; its brain tests on `Step` lists built in the test (Testing). | 5, 7, 8, 9 | X3, X4, X5 (the engage estimate it wires to `pvpRowTable`), S6 (#325), S11 (#330) |
| X7 | PvP rows for the fighters: HF, EF, DF, OF, WF and KN, WA, RO, EK, ES, PK, AS, OR, MO, SC; their `TestPvPRowsShipped` cases. | 10 | X6, S6b (#326) |
| X8 | PvP rows for the mystics: HM, EM, DM, OM and CL, HW, EO, EW, SO, DW, OS; their `TestPvPRowsShipped` cases. | 10 | X6, S7 (#327) |
| X9 | Party PvP for every preset in any party, whoever leads it: enemy set (the healer of an enemy included, as fighting back), healer marking and priority, peel, the healer's PvP buffs under its own `SupportFlagged`. | 11 | X6, X7 (the KN peel), X8 (the healer's Mental Shield); works at once for `support` and `pvp` bots in a player's party; `farmer` bots in a party, bot parties (and `TestLivePvPHealer`) need M11 |
| X10 | PvP gear and consumables: the seen-class mix table, `PvPValue` over it and `pvpRowTable` in the gear, set, jewel and dye scores for the `pvp` preset, the model tests over the shipped rows (the Mithril test), the cures on its supply list, the CP potion and cure keep rule (D37). | 12 | X3, X6 (the cure and CP potion use, `pvpRowTable`), X7 and X8 (the rows the value reads), S5 (#324), M8 I3 (#170: the supply list and trip targets) and I4 (#171: `keeps` and the keep hook) |
| X11 | Death and karma: the arena return loop and its cap, the avoid list, PK mode with its limits, its non-debuff damage until the victim flags, and the karma state (section 13, "With karma": clean attackers fled from (`unfought` returns a refused player attacker while karma is above 0) or fought as a PK under the limits, the chaotic-point restart, the karma route for the walk-back and the flee (on foot, no gatekeeper or shop, `KarmaClear` from town polygons and guard spawns, a nearer spot when its own has none), no town trip, no hunt, duel or arena trip, work-off) (D39). | 13 | X4, X5, X6, X7 and X8 (`TestPvPPKCap` checks the WA and HM rows) |
| X12 | Panel switches, PvP counts, and the four counters of section 14: `bot_pvp_fights_total` and `bot_pvp_deaths_total` by the kind on the opponent record (`duel`, `fight_back` and `unfought` set by X4, `arena` and `flagged` by X5, `pk` by X11), `bot_duels_total` from X4's duel result, `bot_pvp_interrupts_total` from X6's interrupts. | 14 | X4, X5, X6, X11, S9 (#331) |
| X13 | e2e live scenarios for X5-X11, at most 4 characters each. | Testing | each; M11 for `TestLivePvPHealer` |
| Backlog | PvP rows for second professions (with M10 #332), party duels (with M11), the gaps below. | Gaps | X7, X8 |

Most urgent: X1 to X5 give a bot that takes duels and fights in arenas with its M10 rotation (X4
needs X3's class map, X5 answers the duels and makes the arena trips);
X6 with X7 and X8 make it fight like its class.

## Risks

- **Annoying players.** Unasked duel challenges are a pop-up for a player. The holds (3 minutes per
  bot, 30 minutes per player, 2 hours after a decline) and the tier match bound them, and the
  owner may switch own challenges off on the panel and leave only the whispered `duel`.
- **An accidental PK.** A karma-free opponent whose flag ends while the bot's cast or swing is on
  its way would make the bot a PKer. Section 3's guard stops every damaging attack the moment the
  flag blinks (4-5 s before it ends) and cancels the bot's own damage cast in flight; a cast past
  the server's cancel window lands within about 200 ms. Skills with `nextActionAttack` (Shield Stun
  and Bleed among them) make the server swing at a target it may attack without force once their
  cast ends, even a cancelled one, so none is started after the blink, and a move follows the end
  of one in flight; a follow-up swing the server started first lands within one attack interval.
  Area damage and pole swings are held while a flagged karma-free bystander, or a summon showing
  a flag and no karma, is in reach (in an arena, any karma-free player but the target not well
  inside, and any summon whose owner is not seen inside), since they would hit it too; a pole wielder moves clear or
  holds, since every step it has swings. A summon is fought only by its own current flag or
  karma, never by the `Attackable` its `NPCInfo` carried when it appeared. DOTs cannot kill a player. In an arena no blink warns that an opponent is
  about to step out (and so become a clean player), so the arena-edge rule starts a damaging step
  only when the opponent cannot reach the edge before it lands, stops as at a blink near the
  edge, and walks the bot out only with nothing of its own in flight. The remaining risks are a
  `RelationChanged` or `CharInfo` delayed by more than about 3.5 s of network lag, a move packet
  delayed past the arena margin, and an arena opponent that speeds up (Dash, Sprint) or loses the
  bot's root to a cure inside the last 200 ms of the bot's cast.
- **Mystics and archers in the small arenas.** The arena-edge rule keeps a step whose scaled hit
  time is above about 3 s from starting against an unrooted karma-free opponent in the Dion,
  Gludin and Giran arenas, which are about 1000 × 1000 (no point more than about 490 from an
  edge, against run speeds of 121-144). With a spiritshot charged the Elven and Human Mystics'
  4000 ms nukes and drains (2.4-2.8 s) and Ice Bolt still start in the middle part of the arena,
  and the Dark Mystic's (3.0 s) only at the middle; near the edges they wait. The Orc Mystic's
  (3.6 s), any mystic's without a spiritshot, Drain Health, Power Shot and Stunning Shot almost
  never start. Rows with a root (CL, EO, SO, OS) root first; the OM row fights there with DOTs,
  control and melee and loses most of its damage, and the bow branches lose Power Shot and
  Stunning Shot (their interrupt) but keep their plain shots. The Colosseum allows those steps near its middle.
  This is the price of never killing an opponent who steps out; the owner sees it in D39, and a
  Colosseum preference for those rows is a backlog item ("Gaps").
- **A late Esc.** An Esc sent near the end of the bot's estimated cancel window can reach the
  server after the cast ended; it then clears the bot's target (`TargetUnselected`). The bot
  expects it, drops the attack with its move and selects the opponent again before its next
  step, so it costs one `Action` and no attack.
- **Duels cancelled by monsters.** Any hit between a duellist and an NPC cancels the duel, and a
  cancelled one-on-one duel gives back no CP, HP or MP. The bot takes and offers duels only clear
  of monsters and hits no NPC during one, but a monster that wanders in and attacks still cancels
  it; the bot then fights the monster and rests, as after any fight. A pole wielder's swing
  already under way when an NPC or summon steps into its cone lands on it too and cancels the
  duel the same way.
- **Duels cancelled by ground.** A duellist in a peace, town, Derby track, arena or running siege
  zone, or more than 2000 from its opponent, cancels the duel, and nothing is given back. The
  bot's own moves in a duel keep `DuelEdge` (100) outside those polygons and within 1800 of the
  opponent, and the server's own follow, which walks the bot after the opponent with no new
  packet, is stopped by a checked disengage move as soon as the line to the opponent or the
  bot's current follow leg comes within `DuelEdge` of one (section 4, "Duel ground"). What can
  still cancel it: the opponent's own move, a siege that starts mid-duel in a siege polygon the
  bot already stood in, a server follow leg that bends into a polygon inside the margin between
  two of the bot's checks (the bot sees each leg's destination, not the path the server plans to
  it), and a position error past the margin. Against an opponent who keeps near a town edge the
  bot fights only with what it can cast from where it stands, and the duel may run out its
  2 minutes (a timeout gives everything back).
- **Several bots, one player.** The challenge holds are per bot, so N `pvp` bots near one player
  can each challenge it once per 30 minutes. The server keeps one open request per target (a
  second challenger gets 153), so the player sees one pop-up at a time, but in turn. The owner can
  switch own challenges off per bot; a shared hold table across bots is a backlog item ("Gaps").
- **Blind HP.** Outside duels the bot sees an opponent's HP once; the pool estimate misses others'
  damage and the opponent's heals and potions. It is only used for finishers and the break-off,
  and errs toward fighting on.
- **Level unknown.** `CharInfo` has no level; the class tier is a coarse proxy, and a level-39
  first profession will hunt a level-20 one. The engage estimate assumes equal level; hunting only
  flagged and karma players, who chose to fight, keeps this fair.
- **Size.** X6, X7 and X8 change the rotation runner most brain tests use. X6 lands with no PvP row
  (the M10 rotation unchanged against monsters, golden tests), and the rows split by fighter and
  mystic as M10's do.
- **Bots dying in arenas.** Two `pvp` bots can kill each other for an hour; the 5-deaths cap and
  the stay limit end it.

## Gaps

To be filed with the milestone's issues, in the M13 backlog unless noted:

- PvP rows for second professions (40+): with the M10 backlog #332.
- An arena preference: the OM row, the other base mystic rows when out of spiritshots, and the bow
  rows (for Power Shot and Stunning Shot) choosing the Colosseum, where the arena-edge rule lets
  their long steps start, when a gatekeeper reaches it ("Risks").
- Party PvP with a `farmer` bot in a bot-led party: its `TestPvPTargetHealer` cases and
  `TestLivePvPHealer` land with M11 bot parties.
- Party duels: need a bot party leader that can answer for its party (M11).
- A challenge hold shared by all bots, so several `pvp` bots near one player do not each challenge
  it ("Risks").
- Clan war targets, sieges and the Olympiad.
- Buying CP potions: only the Seven Signs priests sell them, for Ancient Adena.
- Fighting servitors and pets for their own sake (the bot fights the owner).
- Kiting monsters stays out of scope, as in M10; this plan kites only players.
