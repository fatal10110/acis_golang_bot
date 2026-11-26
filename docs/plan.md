# Playable Bots Plan (#4522)

Status: draft. Revised after five aspect reviews (feasibility, concurrency and scale, security and
abuse, repo rules and scope, gameplay), the owner's answers of 2026-11-07, and three re-reviews
(separate-repo feasibility, consistency, operations and abuse under D6). Extended on 2026-11-17 with
the owner's decisions D9-D11 (milestone M6: run from the owner's machine, gameplay control panel
with a local bot registry).

Code paths in this document (`internal/...`, `cmd/...`, `docs/...`, `ops/...`) refer to the server
repository [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang) unless they name this
repository.

## Tracking

| Milestone | Epic | Sub-issues |
| --- | --- | --- |
| M0 - Bootstrap | #1 | #2-#5 |
| M1 - Core client and brain | #6 | #7-#15 |
| M2 - Support persona | #16 | #17-#24 |
| M3 - Farmer persona | #25 | #26-#29 |
| M4 - Town, trade and merchant | #30 | #31-#36 |
| M5 - Progression | #37 | #38-#40 |
| M6 - Home run and control panel | to be filed | to be filed (PR breakdown, rows 6a-6d) |
| M7 - Own navigation | #151 | #152-#162 (`docs/navigation.md`, PRs N0-N8 and two gap issues) |
| M8 - Items and loadout | #166 | #167-#182 (`docs/items.md`, PRs I0-I13 with I8b, and the enchant backlog issue) |
| M9 - Clan crafter | #184 | #185-#194 (`docs/crafting.md`, PRs C1-C10) |
| M10 - Race and class strategy | #319 | #320-#333 (`docs/strategy.md`, PR #318; backlog #332); second professions epic #624 (#625-#636, S17 deferred); servitor lines #592 (no milestone yet) |
| M11 - Bot parties | #371 | #373-#392 (`docs/parties/`, PRs P1a-P8, 13 PRs, D25-D29); follow-ups #395, #397, #398, #438 |
| M12 - Merchant valuables | #376 | #378-#402 (`docs/valuables/`, PRs V1-V9, 13 PRs, D30-D34); follow-ups #403, #404 |
| M13 - PvP persona | #405 | #406-#420 (`docs/pvp/`, PRs X1-X13, 15 PRs, D35-D39); follow-ups #421-#429; X14-X16 #602-#604 (`docs/pvp/war.md`, `siege.md`, `olympiad.md`; D40-D42 proposed, not approved); X17-X19 #612-#614 (`docs/pvp/cp-potions.md`; D43 proposed, not approved) |
| M14 - Hardening and structure | #641 | #642-#661 (`docs/structure.md`, PRs HD1-HD6, RT1-RT2, BR1a-BR3, GM1-GM3, WR1, WP1-WP3, OP0, D45-D48; #661 is the backlog issue OP4; OP1, OP2 and BR1b open, not filed) |


Server-side prerequisites: fatal10110/acis_golang#4525, fatal10110/acis_golang#4526,
fatal10110/acis_golang#4527, fatal10110/acis_golang#4528. The PR numbers in "PR breakdown" map to
milestones M0-M6 (PR2a and PR2b are both M2; PR6a-PR6d are all M6).

## Goal

Bots that play like real users so people have fun playing alongside them: follow and assist a
player, farm, loot, rest, die and come back, party, trade, sell in town, level up. Run from the
owner's machine (D9) and spawned one by one, each with its own persona, from a local control panel
(D10); the Hetzner host deployment is retired. Load testing (#260) is not
the goal.

## Owner decisions

| # | Decision | Answer |
| --- | --- | --- |
| D1 | First persona | Follow/assist a player (support), before autonomous farming |
| D2 | Server-side fast path | Only if measurements cross a threshold written down in advance |
| D3 | Who may command a bot in game | GM characters: access level with `isGM="true"` (levels 7-8 in the shipped `accessLevels.xml`; levels 2-5 are named GM but are not) |
| D4 | Protocol | Real client protocol; no new lightweight protocol, no direct domain calls |
| D5 | Where the bot lives | Its own Go module in a separate repository |
| D6 | What bots may do | Any game action (trade, private store, drop, party, PvP); they exist for players' fun, not as a safety boundary. Persona defaults below are configurable and never restrict D6 |
| D7 | How bots are marked | A nickname marker only; no title marker, no separate player count |
| D8 | Java server | Not targeted |
| D9 | Where the bots run | On the owner's machine, dialing the server's public login address. The owner's home IP is dynamic, so the owner's provider range (CIDR) is on `AcceptFilterAllowlist` of both servers, and the bots run `-allowlisted` (no login spacing). The bots know nothing of the range |
| D10 | How bots are run | A control panel on the owner's machine: bots are spawned one by one or in small batches, each with its own persona. Every bot created is stored locally, and the panel logs existing bots back in (on reconnect, on respawn, after its own restart) instead of creating new ones. Gameplay bots, not load testing |
| D11 | Ban and security risk on the home run | Accepted by the owner: it is the owner's private server. M6 adds no ban or abuse handling beyond what the bots already have (login gate, credential breaker), and the panel is a local tool |
| D12 | Panel page stack | React, TypeScript (strict), Vite and Tailwind, built from `web/` and embedded in the binary; Node is needed only to build the page (`make ui`, the CI `web` job, the release build), never by the Go jobs or at run time. Replaces the earlier plain-JavaScript, no-Node rule (owner, 2026-11-17, #106) |
| D13 | Who finds a bot's route | The bot, on its own copy of the server's geodata: every move it sends is a segment it checked with the copied `CanMove` and step chain, and the server's pathfinder is never relied on. Geodata is never loaded whole into memory: it is packed offline and mapped read-only, with regions read in place, as the game server does. Walking personas need the geodata and nav caches; today's legs are not kept as a fallback (D13a). Owner, 2026-11-21. Design and delivery: `docs/navigation.md` |
| D14 | Where D-grade and higher shots come from | A Dwarven crafter bot makes them for its clan through the clan warehouse (the clan crafter plan, M9). No spawned NPC sells them; until then the bots weigh weapons by whether their grade's shots can be had. Owner, 2026-11-21. Design: `docs/items.md` |
| D15 | Class change | At the class master NPC 50000: free and quest-free under the shipped `ConfigClassMaster`, given a persistent spawn in the towns by fatal10110/acis_golang#5419. Bots pick a profession for their race and persona at 20 and 40, then draw dyes. Owner, 2026-11-21. Design: `docs/items.md` |
| D16 | Supply trips | A farmer may leave its spot to restock in town (by Scroll of Escape, a walk or a gatekeeper); its spot is released and may be taken. Owner, 2026-11-21. Design: `docs/items.md` |
| D17 | Who withdraws from the clan warehouse | The owner's server sets `MembersCanWithdrawFromClanWH=True`, and the clan leader gives grade 6 (where new members join) privilege 8; a server setting, not a bot special case. Owner, 2026-11-21. Design: `docs/crafting.md` |
| D18 | How the clan crafter's Create Item grows | Through the class change of D15 (M8 #178): Artisan at 20, Warsmith at 40; until then Create Item 1 and no crystallizing. Owner, 2026-11-21. Design: `docs/crafting.md` |
| D19 | What the clan crafter may take from the clan warehouse | Anything a crafting chain it plans needs, any time; it crafts a chain only when the chain is worth it. Never adena, items no reachable recipe uses, or unpriced items without an order. Owner, 2026-11-21. Design: `docs/crafting.md` |
| D20 | Who may add the clan crafter to a clan and order crafts | Beside GMs, any player: a clanless crafter accepts any player's clan invitation (or only the names in its preset's `ClanInviters`), and its clan's leader orders crafts like a GM. Extends D3 for the crafter only. Owner, 2026-11-22. Design: `docs/crafting.md` |
| D21 | Which role each preset plays, and the default preset | New `farmer` preset: Damage, and the default for `-persona`, the panel and imported bots (`ImportPreset`); `support`: Support; `merchant`: Damage; `crafter`: Crafter; `town`: Support. Owner, 2026-11-24. Design: `docs/strategy.md` |
| D22 | How a bot picks its profession branch | Weighted by race fit, drawn from a hash of the account name and role while a base class; a bot with a profession keeps its line; the panel may pin a branch per bot (registry field). Owner, 2026-11-24. Design: `docs/strategy.md` |
| D23 | Which professions are out of scope | Servitor lines (Warlock, Elemental Summoner, Phantom Summoner) and Necromancer for its summons, a later milestone (#332); the Overlord is not picked (a group class). Owner, 2026-11-24. Design: `docs/strategy.md` |
| D24 | May the M10 rewrite change today's behavior | Yes, per PR: S4 lands behavior-neutral, and every later PR states the behavior it changes. Owner, 2026-11-24. Design: `docs/strategy.md` |
| D25 | Who organizes bot parties | The panel's supervisor, from the registry and the live snapshots: online bots of the same town within walking reach and a level band; no in-game discovery. On by default in `bots ui` with a panel switch. `bots swarm` runs the same planner over its own bots, on by default (`-no-parties` turns it off; `-support N` makes the last N bots of a `farmer` swarm play `support`). Owner, 2026-11-25; swarm organizer 2026-11-26 (#395). Design: `docs/parties/` |
| D26 | Who is in a bot party | A Damage-role bot (`farmer`) leads; Support-role bots (`support`) join, at least one healer; a third is any other Support bot, else a second Damage bot with no healer to lead with. Size 3, at most 4 (persona defaults). `merchant`, `crafter` and `town` never take part; party mode `off` keeps a bot out; the panel may pin a party. Owner, 2026-11-25. Design: `docs/parties/composition.md` |
| D27 | Loot and level band of a bot party | Loot by turn: items rotate, the server splits adena in range, a spoil stays with its sweeper. A party forms within 3 levels and breaks past 5; players are not counted. Owner, 2026-11-25. Design: `docs/parties/composition.md` |
| D28 | What players may ask of a bot party | Any player may whisper `join` to a bot party's leader to be invited, and `invite me` to a Support member to take it out (extends D3 as D20 did, for these two whispers to bots of bot parties only); a GM's `invite me` takes the bot out and a GM's `leave` to the leader disperses the party. Owner, 2026-11-25. Design: `docs/parties/players.md` |
| D29 | Whom a bot invites | Only the bots the organizer or the panel assigned to its party, and players who whisper `join`; never unasked. Bots of a bot party never whisper each other. Owner, 2026-11-25. Design: `docs/parties/README.md` |
| D30 | Where the merchant advertises valuables | On Trade only, under the caps of `docs/valuables/ads.md`; never Shout. Owner, 2026-11-25. Design: `docs/valuables/` |
| D31 | What is valuable | The score of `docs/valuables/score.md` (price x 2^R from the supply ceiling, NPC multisell and buylist rules, halved for craftables, raised by enchant past safe); valuable at score 100000 and price 10000. A valuable precedes the clan member's hold. Owner, 2026-11-25 |
| D32 | How a valuable is priced and sold | Ask 100-150 % of reference by rarity plus the enchant scrolls, 10 points lower per unsold round; never below the floor (reference plus scrolls, or the store price if higher); two counters per buyer and item a round; the bot may invite a whisperer in sight, one request per buyer a round. Owner, 2026-11-25. Design: `docs/valuables/whispers.md`, `trades.md` |
| D33 | Hold, warehouse and cap | A 5 min round of ads, then the private warehouse; a new valuable brings the stored out for another round; after a round all go to the store once they take more than 6 rows, or by the 2000000 ask cap, or one unsold through 3 rounds; released valuables list at their floor. Owner, 2026-11-25. Design: `docs/valuables/round.md`, `release.md` |
| D34 | Valuables budget | A per-session `-valuables-budget`, default 2000000, apart from `-store-budget`; valuables are offered and listed while sales are under it. Owner, 2026-11-25 |
| D35 | PvP modes | A new `pvp` preset (Damage role, farms between fights): duels, flagged and karma players, arenas on; open-world PK off (opt-in per bot). Other presets fight back only and decline duels, but every preset fights players with the PvP layer. PvP events wake the brain after 200-600 ms. Persona defaults (D6). Owner, 2026-11-25. Design: `docs/pvp/modes.md` |
| D36 | How a `pvp` bot finds fights | Takes any duel it can; challenges a player who whispers `duel` (extends D3 as D20 did); challenges non-bot players and hunts flagged and karma players of its tier when the engage estimate passes; visits the nearest arena on a timer. Owner, 2026-11-25. Design: `docs/pvp/finding.md` |
| D37 | PvP items | One loadout scored by `Value` and `PvPValue` blended by relative gains, 50 % each for `pvp` (others 0 %); no set swap; cures bought on supply trips; CP potions kept, never sold or bought (buying amended by D43). Owner, 2026-11-25. Design: `docs/pvp/model.md` |
| D38 | Bots against bots | Outside an arena a bot never attacks, hunts or challenges a bot and fights one back only after its hit or offensive cast; in an arena anyone but its party. No bot duels. Owner, 2026-11-25. Design: `docs/pvp/targets.md` |
| D39 | PvP death and karma | Duels to the end, no surrender; arenas to the death under a death cap; in the open world break off by the row and avoid the killer. No karma by accident and nothing that would cancel a duel. Opt-in PK mode and life with karma as in `docs/pvp/karma.md`. Owner, 2026-11-25. Design: `docs/pvp/guards.md` |
| D40 | Clan wars | A mutual-war enemy is a target like a flagged player for `pvp` bots (`War` mode, on by default for `pvp`), attacked with force; marks drop when a war ends or the enemy's `CharInfo` shows another clan. Bots never declare, answer, stop or surrender a war. Owner, 2026-11-26. Design: `docs/pvp/war.md` |
| D41 | Sieges | Bots never register for a siege; every bot walks out of a running battlefield it does not fight in; a `pvp` bot of a clan its leader registered fights the other side's players there, bots only after their hit (D38). Owner, 2026-11-26. Design: `docs/pvp/siege.md` |
| D42 | The Olympiad | Opt-in per bot, off for every preset: noblesse at the class master for any `Olympiad` bot, bounded registration, the drawn opponent fought whoever it is; never spectates, trades points or claims hero status. Owner, 2026-11-26. Design: `docs/pvp/olympiad.md` |
| D43 | Buying CP potions | Amends D37: CP potions are bought (still never sold), from player sell stores for `pvp` bots, and by an opt-in `SevenSigns` Dusk member (named bots only, `SevenSigns.MaxMembers` default 1) at a priest for Ancient Adena in a validation week its cabal won; never contributes stones, pays the Dawn fee or votes Strife; votes the seal Dusk holds, else Gnosis. Owner, 2026-11-26. Design: `docs/pvp/cp-potions.md` |
| D44 | Does a `town` bot farm? | It rotates: town spells of 10-20 minutes (idle drift, errands, gatekeeper hops; no spot) and farming spells of 30-60 minutes at a spot, as a support bot farms alone; the farming spell's end sends it home on a supply trip. Persona defaults (D6); still never in a bot party (D26). Owner, 2026-11-26 ("there is no point to stay only in town"). Design: `internal/botclient/town.go` `townRota` (#618) |
| D45 | What a bot does on a packet it cannot decode | It logs an error and skips the packet; the session plays on. Before the world the attempt ends with the error logged. Never counted as a server abort. Owner, 2026-11-26. Design: `docs/structure.md` (HD4) |
| D46 | Where the wire code lives | In its own package, `internal/botwire` (events, decoders, encoders, opcodes), so the brain boundary is compiled. Owner, 2026-11-26. Design: `docs/structure.md` (WP1-WP3) |
| D47 | Outcome log | The bots write what they measure at their spots (experience, adena, deaths, kill times against the model's) to a bounded JSONL file, off unless `-outcome-log` is given; no names or text. Owner, 2026-11-26. Design: `docs/structure.md` (OP0) |
| D48 | Human-looking delays; a language model for whispers | No log-normal delays: the bots are not meant to pass for people. A small language model for whisper replies is a backlog issue, not scheduled. Owner, 2026-11-26. Design: `docs/structure.md` (Options) |
| D49 | Skill reagents | Amends `docs/strategy.md` (S13c): a skill is never cast without the item its level consumes (`itemConsumeId`/`itemConsumeCount`; the server's refusal, SystemMessage 113, is not a charge refusal), and the shop errand restocks reagents at spawned merchants, `ReagentCastsMin` 20 / `ReagentCastsMax` 100 casts per skill summed per item (persona defaults, D6), only from the adena past the supplies tier's floor (M8 tiers), after potions, shots and arrows. Crystal D has no spawned seller: the cubic summons stay silent (#636 deferred). Owner, 2026-11-26 (epic #624, decision 3). Design: `docs/strategy.md` Design 4 "Skill reagents" (#627) |

## Consequences of a separate repository (D5)

Go forbids importing `github.com/fatal10110/acis_golang/internal/...` from another module, so the bot
repository owns:

- framing, string and integer codecs (from `internal/commons/wire`, about 590 lines);
- client-side login crypt: Blowfish, Init decrypt and un-XOR, RSA modulus unscramble (from
  `internal/commons/crypt` and `internal/loginserver/crypt`, about 480 lines);
- the game cipher (from `internal/gameserver/network/cipher`, 86 lines);
- opcode constants, redeclared locally (today `internal/botclient` pulls about 70 server packages
  only for about 60 opcode constants);
- packet decoders and encoders, starting from `internal/botclient` (about 1.3k lines);
- minimal datapack readers for the fields bots use.

PR0 is about 2.5k lines of copied non-test code with `golang.org/x/crypto` as the only external
dependency. Tests that today use the server-side halves (`NewLoginKeyPair`, `NewLoginCrypt`) as
oracles get those halves copied as test helpers, or frozen vectors. The code is the owner's and
acis_golang has no license file. `cmd/botswarm` stays in acis_golang for #260 and is not extended.

The repo rules in `acis_public/CLAUDE.md` govern acis_golang, not the bot repository.

### Server-side prerequisites (acis_golang issues, before PR0's e2e)

The compose setup cannot boot a playable server from another repository today: it builds from
source (`build: .`), needs the private `aCis_datapack` checkout and the Java tree's
`aCis_gameserver/config` (`ops/docker/init-config.sh`), needs an interactive `gsregister` step plus a
`hexid.txt` rename, has no geodata in any repository, and pins `container_name` and ports. CI never
boots the servers.

| Item | Scope |
| --- | --- |
| P1 (fatal10110/acis_golang#4525) | Publish a gameserver/loginserver image to GHCR per `main` commit |
| P2 (fatal10110/acis_golang#4526) | Non-interactive gameserver registration for compose |
| P3 (fatal10110/acis_golang#4527) | Config bundle that does not need the Java tree |
| P4 (fatal10110/acis_golang#4528) | A small geodata subset (2-3 test regions) as a CI artifact; `geoengine.properties` lists only those regions |

Server config, not code, for every bot deployment: the accept-filter allowlist
(`AcceptFilterAllowlist`, both `loginserver.properties` and `server.properties`; the owner's
provider range for the home run of D9), the advertised game address (`Hostname` in
`server.properties`, public for the home run), `AutoCreateAccounts = True` in
`loginserver.properties` (the default; the panel's new bots create their accounts on first login),
and `MaximumOnlineUsers` raised by the bot cap. The e2e stack sets its own values (Test strategy).

## Starting point

`cmd/botswarm` and `internal/botclient` (#4444) already speak the real protocol end to end: login
(Init, GameGuard, RSA credentials, Blowfish, server list, PlayOk), game connection (ProtocolVersion,
VersionCheck cipher, AuthLogin, character create or select, EnterWorld), a round-robin behavior cycle,
and a report. Character creation is fixed to a human fighter in slot 0, and the client never sends
`Appearing`, so a teleported bot stays out of the world.

## Reference bots reviewed

Design input only; no code is copied. L2Grinder is MIT, L2InterludeBot and Ariakan79/L2Bot have no
license, L2Net and ellysh/l2bot are GPL.

| Repo | Kind | Takeaways |
| --- | --- | --- |
| BudongJW/L2Grinder | DLL injected into the client | Combat FSM as a table of `(from states, to state, predicate)`; declarative thresholds and filters; attackers take priority over a fresh target |
| kpyrkosz/L2InterludeBot | DLL injected into the client | Feature checklist; per-concern modules with cooldowns under one intention; target blacklist; priority for mobs hitting self or party |
| Ariakan79/L2Bot (High Five) | Out-of-game Node client | Protocol client emits events, bot is a pure state machine with no I/O; pace skills from cast feedback; dead reckoning with snap on drift > 200; serial loot queue; stuck detection (< 40 units in 2.5 s) |
| L2-Net/L2Net | Out-of-game client and proxy | Option groups (rest, leash, loot, anti-KS, blacklist after N tries); death policy; follow/assist leader |
| ellysh/l2bot | Pixel clicker | Nothing |

## Where the cost of a real-protocol bot is

- Bot side: two goroutines per bot, XOR cipher, decode only what the brain reads. In a crowd every
  bot tracks every visible object, so bot memory and decode work grow as N²; keep object records
  compact and measure memory per bot in a clustered scenario.
- Server side, inherent to any character: visibility, aggro, AI, persistence.
- Server side, per recipient: visibility frames such as `CharInfo` are encoded per viewer; broadcast
  frames are a `CopyFrame`; every frame then costs two mutexes, a writer wakeup, cipher and a
  `writev`. Total is N² only for bots within view of each other; the support persona clusters bots.
- Login: bcrypt verify at cost 10 per login. Persistent bots log in once.

D2 options are server work in acis_golang with their own design approval (S1, S2 below). An
in-process mode is not possible: the server cannot embed code from the bot repository.

## Architecture (bot repository)

Concrete types; add interfaces only when a second implementation exists.

### 1. Reader and events

A read returns a payload in a reused buffer, so frames never cross goroutines. The reader goroutine
filters by opcode, decodes into typed value events, and sends only those. Skipped opcodes are dropped
without parsing. Move events coalesce to the latest position per object when the channel is backed
up.

After every `TeleportToLocation` for itself (respawn, GM recall, gatekeeper, scroll) the bot sends
`Appearing`; until then the server keeps the character out of the world.

Codec tests: round trips inside the bot repository plus recorded fixtures. A `-record <dir>` tap after
decryption writes the first payload per opcode during e2e runs; fixture tests assert each decoder
consumes exactly `len(payload)` and that fields are sane (ids, coordinates and times vary).

Server packets, by the first PR that reads them:

| PR | Packets |
| --- | --- |
| 1 | `UserInfo`, `StatusUpdate`, `CharInfo`, `NPCInfo`, `DeleteObject`, `MoveToLocation`, `MoveToPawn`, `StopMove`, `ValidateLocation`, `TeleportToLocation`, `Die`, `Revive`, `MyTargetSelected`, `TargetUnselected`, `SystemMessage` (id), `ActionFailed`, `CreatureSay`, `ChangeWaitType`, `CharCreateOk`, `CharCreateFail` |
| 2a | `AskJoinParty`, `JoinParty`, `PartySmallWindowAll/Add/Delete/DeleteAll/Update`, `PartyMemberPosition` |
| 2b | `Attack`, `AutoAttackStop`, `MagicSkillUse`, `MagicSkillLaunched`, `SkillList`, `SkillCoolTime`, `PartySpelled`, `AbnormalStatusUpdate`, `ConfirmDlg`, `ItemList`, `InventoryUpdate`, `ExAutoSoulShot`, `NpcHtmlMessage`, `AcquireSkillList`, `AcquireSkillInfo`, `AcquireSkillDone` |
| 3 | `SpawnItem`, `DropItem`, `GetItem` |
| 4 | `BuyList`, `SellList`, `SendTradeRequest`, `TradeStart`, `TradeOwnAdd`, `TradeOtherAdd`, `TradeUpdate`/`TradeItemUpdate`, `TradePressOwnOk`, `TradePressOtherOk`, `SendTradeDone`, `PrivateStoreManageListSell`, `PrivateStoreListSell`, `PrivateStoreMsgSell` |

Client packets, added with their first user:

| PR | Packets |
| --- | --- |
| 1 | `Action`, `MoveBackwardToLocation`, `ValidatePosition`, `Appearing`, `RequestTargetCancel`, `RequestActionUse` (sit/stand), `RequestSocialAction`, `Say2`, `RequestRestartPoint`, `RequestRestart`, `Logout`, `RequestCharacterCreate` with race, sex, class, hair and face |
| 2a | `RequestJoinParty`, `RequestAnswerJoinParty`, `RequestWithdrawParty` |
| 2b | `AttackRequest`, `RequestMagicSkillUse`, `UseItem`, auto soulshot toggle, `DlgAnswer`, `RequestBypassToServer`, `RequestAcquireSkillInfo`, `RequestAcquireSkill` |
| 3 | `RequestDropItem` |
| 4 | `RequestBuyItem`, `RequestSellItem`, `TradeRequest`, `AnswerTradeRequest`, `AddTradeItem`, `TradeDone`, `RequestPrivateStoreManageSell` (or `RequestActionUse` action 10), `SetPrivateStoreMsgSell`, `SetPrivateStoreListSell`, `RequestPrivateStoreQuitSell`, `RequestPrivateStoreBuy` |

### 2. Brain

One goroutine per bot owns its world model. It selects on the event channel, one resettable timer
holding its next deadline (injected clock), and a per-session `done` channel. The reader's send also
selects on `done`, so closing a session never deadlocks on a full channel; logout waits run inside
the brain's own select loop.

World model: self (position with dead reckoning, HP/MP/CP and max, level, class, sitting, in combat,
PvP flag, karma, store type, weight, dead), known objects by id (NPCs with template id, players with
name, flag and karma), per-NPC "engaged by" record with timestamp, current target, party with member
HP/MP, own and party buffs with remaining time, inventory, skills with reuse, pending prompts.

Decision each tick (base interval plus uniform jitter), first match wins:

1. dead -> death policy;
2. pending prompt (party, trade, resurrection) -> answer per persona;
3. in combat and HP critical -> potion, heal, or flee to the leader;
4. two or more attackers -> fight back on the one hitting hardest, or flee when outmatched;
5. being attacked -> fight back (attackers before fresh targets; never sit while targeted);
6. out of combat and HP/MP low -> sit to rest, stand at threshold;
7. buffs due (< 60 s left, out of combat) -> rebuff;
8. loot queued and no attacker left -> pick up one item, with timeout;
9. persona goal (follow/assist, farm, errand);
10. idle drift (small moves, turns, sit, emote).

Store mode (merchant persona) replaces rules 3-10: the bot stays seated in its store and only
watches for the store closing.

Combat pacing: melee sends `Action`/`AttackRequest` once per target (the server repeats the swing);
skills are paced from `MagicSkillUse` hit time plus reuse, with a safety timeout.

Shared behavior in every persona:

- no anti-KS (owner decision on #138, superseding the PR3 rule): kill stealing is acceptable, so a
  bot may attack a monster another player is fighting, and it does not track who fights what;
- PvP defaults (configurable, D6): assist and fight-back target NPCs, or players who are flagged or
  have karma; never force-attack an unflagged player with no karma; stop fighting back when the
  attacker unflags; do not heal or buff a flagged or karma party member (that flags the bot);
  a player's offensive cast at the bot is an attack, a heal or buff is not (`docs/pvp/wire.md`
  "Changes to today's code (X4)");
- blacklist a target after N failed attempts; stuck detection over 2.5 s, then a side step;
- reaction delay 300-1200 ms before acting on a new event; a PvP event (a duel packet, a cast
  cancelled, a cast by or at a player the bot fights or by a player at a party member, a fought
  player's move or change of mask or flag) is acted on 200-600 ms after it instead, moving a
  later reaction earlier, never later (D35, `docs/pvp/wire.md`);
- server limits: 160 frames/s burst, 80/s average over 5 s, disconnect after 2 floods per minute;
  bypasses inside 100 ms dropped silently; emotes every 2 s; chat flood gates; traffic within the
  15 minute idle timeout.

### 3. Personas

**Builds, every persona** (M10, `docs/strategy.md`, D21-D24): a bot plays a build compiled from
its race, its class and its persona's role: its class's skills gated by its weapon, the armor
kinds and sets that pay off for its race, a profession fitting its race and role, and thresholds
and spots from a character model fed by its own `UserInfo` stats. It replaces the support
persona's role by class (below) and the M8 build table (`docs/items.md` section 2); its spec is
`docs/strategy.md` "Behavior per class". The `farmer` preset (Damage, farms alone) is the default
`-persona`, the panel's first preset and the imported bots' preset (D21, S9 #331); the panel pins a
bot's branch (D22, `POST /api/bots/{account}/branch`) and shows its build beside its class.

**Walking, every persona** (D13, M7, `docs/navigation.md`): a bot finds its own routes on its copy
of the server's geodata and sends only segments it checked with the copied `CanMove` and step
chain, from standstill. Reachability is the nav hierarchy's: a destination in the bot's SCC is
walked to, one only one-way-out ground leads to is taken as a drop by a follower alone, and one
it cannot reach is not tried. NPCs, monsters and drops are approached on stand points (talk, melee,
pick-up, ranged). A server answer that differs from the check (a blocked stop, a desync) replans,
and repeated ones end the walk unreachable for a minute instead of looping; a door the bot sees
change replans its route. `/unstuck` is used only on isolated ground, which reaches no SCC;
sidesteps, detours, hops and the hour-long spot avoid are gone. The live scenarios hold
`nav_desync_total` at 0 on door-free routes and `nav_blocked_total` at 1 per 100 segments
(`nav_segments_total`) or fewer.

**support** (D1):

- joins a party on invite from a GM commander, and from other players when its persona allows it;
  at most one support bot per non-GM leader; a bot party's members are not counted: they join their
  assigned bot leader outside the claim table (D29, `docs/parties/members.md`); a partyless one asks
  to enter a party-matching room a player opened near it, one bot per room leader under the same
  claim table, and waits there for the leader's invitation (`docs/parties/rooms.md`, #438);
- follows from a slot behind the leader at a random angle, 150-350 units out; re-paths only when the
  leader moves more than ~250 units from the last anchor, after 200-800 ms; matches walk/run.
  Server-side follow (second `Action`, offset 70) is not used: it glues the bot to the leader;
- assists only after the leader's `Attack` or `MagicSkillUse` hits a valid target (PvP defaults
  above), after 0.5-1.5 s; no assist on a bare target selection;
- heals party members only (others' HP is not visible): leader below 60 % with Heal, below 35 % with
  Battle Heal, then self, then others; keeps an MP reserve (no nukes below ~50 % MP); rebuffs from
  `PartySpelled` timers;
- leader out of sight: travel to the last `PartyMemberPosition`; leader dead: stay by the body and
  sit (base mystics have no resurrection); leader teleported, logged out or left the party: wait
  N minutes, whisper once, then fall back to town;
- loot follows the leader's party loot rule; under finders-keepers the support never loots;
- role by class: human, elven or dark elven mystic heals and buffs (Heal, Might, Shield from level 7,
  Battle Heal from 14); orc mystic buffs only; fighters assist only. Skills are learned at the
  trainer with SP (`AutoLearnSkills` is off), from PR2b. M10 (`docs/strategy.md`)
  replaces this list with the Support rows of "Behavior per class": fighters fight with their own
  skills and take a tank line, and the orc mystic also burns and drains.

**farmer** (PR3): spot choice from spawnlist territories (polygons with minZ/maxZ) and NPC levels,
excluding makers with `event=` or `ban=`, raid and grand bosses, non-monsters, aggressive mobs
(`aggroRange > 0`) above the bot's level, and likely quest mobs (no data marker; heuristic on type and
spawn flags). Up to level 5 the aggressive bound is that of any monster (two levels above the bot):
nearly every starter monster has an aggro range, so the strict rule left new characters outside
Talking Island no spot (#133). A process-wide spot registry (one owner) caps bots per spot at about
`maximumNpcs`/3; on arrival the bot counts visible players and moves on when crowded, and a bot that
dies holding a spot avoids it for 30 minutes, a persona default (#258). With the navigation loaded,
the registry leaves out at start the spots whose centre snaps to a wet cell, where a bot would
drown (#275). Each race starts
in its own newbie zone. Owner decision (#137): the farmers spread so that as many spots as possible
are covered rather than waiting for one. A bot claims, among the fitting spots in its own SCC (D13)
within `MaxTravel` (20000) of where it stands in straight line, the nearest by abstract-graph distance
that no bot holds, else the nearest with room; with none, the same within `Wander` (40000); it walks
there through its walker, and a walk that ends unreachable gives the spot up for a minute (#158,
#144), three such walks in a row stop its search for a minute with their spots avoided past it
(#497); with none, a gatekeeper trip planned by Dijkstra over its position, the gatekeepers, their
destinations and the spots with room, walks priced by time and teleports by adena, when it carries
the price; with none, it shares past its cap the fitting spot within `Wander` that its bots fill least for
its cap, up to one bot per monster (`maximumNpcs`), and looks again each minute for a spot with room.
Owner decision (#142): with every fitting spot within `Wander` at that limit, a bot holding no spot
farms beside one of them with no claim, counted toward neither its cap nor its limit, under the same
level fit, leash and pacing as a holder; the bots beside spread over the full spots by their monsters,
and look again each minute for a spot to claim. The owner also asked for a longer `Wander` for the
Dwarven start as far as routes reach more level-1 spots: the datapack has none between 9353 and 168107
of the Dwarven start in straight line, and those at 168107 are the Orc village's, full with its own
farmers and over 24 minutes' run (115) away, so the Dwarven start keeps the default `Wander`.
The same choice runs at every level-up that outlevels its spot. Level 1 farmers find room at 17
(Talking Island), 36 (elven and dark elven), 13 (orc) and 4 (dwarven) per village and share up to 58,
118, 46 and 13; a home run's 340 hold 235 spots at once and the other 105 farm beside full ones
(31 orc, 43 dwarven), until a holder levels on (`TestStarterSpread`).
Owner decision (#138): it is fine for a farmer to steal a kill. A monster another player is fighting
is a target like any other, which keeps more farmers fighting on shared spots. A farm target lies
within the leash (1500) of the spot's centre or inside the spot's territory and its height band, and
a bot that ends a fight beyond the leash and outside the territory walks back; the walk to a target,
around a wall or a rock, may leave the leash on its way (#269). A maker spreads its monsters over its
whole territory, which can be far wider than the leash (#447). A bot at its spot that goes 5 minutes
out of combat with no farm target, other players taking every monster or none coming in reach, moves
on and avoids the spot for 10 minutes; a pause of over 30 seconds in that stretch (a rest, a supply
trip) starts it over (#360). Both durations are persona defaults (D6).

**town** (PR4): idles, sits, buys potions and shots, sells loot to merchants, uses the gatekeeper,
visits the Newbie Helper for buffs (levels 8-24). It rotates between town spells and farming spells
at a spot (D44, #618): the town life above for 10-20 minutes, then 30-60 minutes of farming, then a
supply trip home. An order to a spot (the panel's or a whispered `go`, a standing `go` at login)
ends the town spell at once and starts a farming spell there.

**merchant** (PR4):

- opening rules (server): standing, out of attack stance and PvP flag, not casting, no pending trade
  request; jail is the only no-store zone. Sequence: open action, manage list,
  `SetPrivateStoreMsgSell` (at most 29 UTF-16 units), `SetPrivateStoreListSell` (rows capped by
  `PrivateSellStoreLimit`); the server seats the character. A teleport closes the store; a sold-out
  store closes on its own, seen as the store type in `UserInfo`;
- sells loot only, never items NPC shops sell; price between 1.2 and 1.5 times the NPC buy-back price
  and below the NPC shop price; valuables the cap released list ahead of the loot at their floor (at
  least the reference price, twice the buy-back) under the valuables budget (D33, D34,
  `docs/valuables/release.md`);
- a process-wide registry keeps spacing between bot stores and at most about 5 bot stores per town;
  a per-bot sales budget per session.

**trade defaults** (every persona, configurable):

- trades are fixed-price sales of the bot's own goods for adena, but a valuable in the merchant's
  round, haggled between ask and floor (D32, `docs/valuables/trades.md`); a GM is exempt;
- confirm only after `TradeOtherAdd` shows enough adena, and only within 150 units of the partner
  (the server requires it); offers lock at the first confirm and requests expire after 15 s;
- never offer equipped or quest items; refuse incoming items that push weight above 50 %;
- rate-limit trade requests per partner; the bot sends none but the merchant's valuables round, one
  per whisperer a round, one open at a time (D32, `docs/valuables/trades.md#asking-for-a-trade`); no
  trading while in store mode (the server forbids it).

Tunables are flags until a persona needs a file.

### 4. Names and human feel

- Character names match the server rule `^[A-Za-z0-9]{1,16}$`: a syllable-based base plus a
  configurable alphanumeric marker (D7), within 16 characters. No title marker. Ships in PR1, since
  bots are visible from PR1.
- Account names are separate from character names and never shown in game.
- Random hair and face, staggered logins, reaction delays, idle pauses and turns.
- Chat only when answering a whisper, after a delay; rate-limited per sender and capped per bot in
  total, but the merchant's valuables whisper replies in a round and within `AnswerAfter` after it,
  which haggle under their own caps per partner and per round instead (D32,
  `docs/valuables/whispers.md#pacing-queue-and-caps`); never answer a name carrying the bot marker
  (no bot-to-bot loops); never on Shout or Trade, but the merchant's valuables ads on Trade, the only
  unprompted lines (D30, `docs/valuables/ads.md`).
- Uniform jitter; no seeded distributions.

### 5. Sessions, reconnect, accounts and host

These keep the server and the bot host working; they are operational, not player safety (D6).

- `-persistent` keeps bots in world.
- One session object per attempt: join its reader before dialing again; start a fresh world model.
- A dropped character stays in the world 100 ms, or 15 s in combat; login meanwhile answers
  AccountInUse, which is retryable with backoff.
- One process-wide login rate limit with jitter: each login and game dial starts at least 1.25 s
  plus up to 0.5 s of jitter after the previous one, under the accept filter's burst window; with
  `-allowlisted` the dials are not spaced.
  While the server is down (connection refused) all bots back off exponentially, re-jittered.
- Process-wide credential breaker: the login ban counts failures per IP, not per bot (3 in a row ban
  the IP for 10 minutes; under D9 that is the home IP, so the ban also locks out the owner's own
  game client). The first credential `LoginFail` halts all logins and alerts.
- Stop reconnecting a bot after repeated server aborts (the 2 MiB outbound cap) and report it.
- Accounts are pre-created for a swarm (a `bots accounts` step); the control panel's accounts are
  created on their first login instead (`AutoCreateAccounts`, section 8). Each has its own password
  derived as HMAC(secret, account) from a 0600 secret file. Each swarm instance or panel uses its
  own account prefix. A kick caused by another login is reported, not fought.
- Home run (D9): bots run on the owner's machine and dial the server's public login address. The
  game address is the server's `Hostname` (`server.properties`, advertised in the ServerList),
  which must be public as for real players, or the bots replace it with `-game <public host:port>`.
  The owner's provider range is on `AcceptFilterAllowlist` of both servers (read at boot only: a
  change needs a restart of both), and the bots run with `-allowlisted`, which no longer needs
  `-local-addr`. Ban risks on this run are accepted (D11).
- Reconnects. The panel runs every bot persistent: after a dropped session it logs back in with
  backoff, to the same account and character (section 8). If only the bot process crashes, the
  kernel closes its sockets and the character lingers 100 ms, or 15 s in combat. After a dead line
  or a lost machine the server holds the old session until its 15-minute read idle deadline, unless
  a login evicts it: each AccountInUse answer also kicks the old session, whose writer then closes
  within about a minute (its write deadline to the dead peer). So the bot must keep retrying logins,
  not wait, and the login server answers AccountInUse until the eviction completes. Today two
  AccountInUse in a row stop the bot as `ErrAccountTaken` (`maxInUse`,
  `internal/botclient/session.go` in this repository). For the home run, AccountInUse is retried
  with backoff (capped at 30 s while it lasts) for 3 minutes counted from the first AccountInUse,
  when that answer follows a run start (a process start, a spawn, a respawn, a persona change) or
  any end that was not a kick, a failed game entry after a good login included; the bot stops as
  taken at the first AccountInUse after a `ServerClose` kick (another login took the account, as
  today) or once that window has passed. Every session that ends without a kick counts one abort
  toward `maxAborts` (3), and a session that played at least `stableSession` (10 minutes) resets the
  count first (`afterSession`, `internal/botclient/session.go` in this repository). A dead line
  found by the timeouts below costs nothing; a reset or EOF (a home IP change, or a long PC sleep
  after which the server has dropped the idle client and the first write meets a reset) costs a
  long-running bot one abort, and only a line that keeps dropping short sessions stops it, after
  which the panel restarts it by itself after a cool-down (section 8).
- Dead sessions end fast. Today a session on a dead line (PC sleep, home IP change) can linger for
  many minutes: the in-world reader has no read deadline, Linux sends no keepalive probe while sent
  data is unacknowledged, retransmissions run for about 15 minutes (`tcp_retries2`), and writes into
  the kernel buffer succeed. The dialer (`internal/botclient/dial.go` in this repository) therefore
  sets `TCP_USER_TIMEOUT` (through `Dialer.Control` with a local constant in a Linux-only file; the
  standard `syscall` package does not name it, and no new dependency is needed) and a
  `KeepAliveConfig` (for example 30 s idle, 10 s interval, 3 probes),
  so such a session ends within about a minute and the bot reconnects. A session ended by those
  timeouts is not counted as an abort; an RST (which a home IP change often brings) still counts.
- The Hetzner host run is retired (D9): its unit is stopped and disabled before the first home run
  (both would log in the same accounts and kick each other), and its `127.0.0.2` allowlist entry is
  replaced by the provider range. `docs/hetzner.md` in this repository keeps the old setup as an
  archive with `ops/acis-bots.service` (in this repository) and points to `docs/home.md` (in this
  repository). The agent rules that place the bots' resource cap "on the shared host" (`CLAUDE.md`
  and `.claude/skills/bot-issue/SKILL.md` in this repository) are updated to the owner's machine.
- A fixed bot cap; `MaximumOnlineUsers` is raised by the same number. Bots cannot see the live
  player count, so there is no dynamic log-off. With the control panel (section 8) the cap is its
  `-max-online` setting, not the size of its registry.
- On the owner's machine the bot cap and `GOMEMLIMIT` bound the bot process; the gameserver no
  longer shares a host with it.

### 6. In-game control (D3)

- Only `CreatureSay` of the Tell type is a command, and only text that parses as a command; other
  text never reaches the authority check. The bot ignores its own outgoing echo.
- The sender is identified by the packet's object id, which the server fills from the speaker (no
  spoofing path: Tell lines come only from the whisper handler); names are not trusted.
- Authority, one of:
  - DB check: an uncached `SELECT accesslevel, deletetime FROM characters WHERE obj_Id = ?` per
    command, resolved through a copy of the server's `accessLevels.xml` to `isGM` (or a configured
    minimum level); negative levels refused. A per-sender token bucket and a 60 s negative cache per
    non-GM object id stop players from turning whispers into DB load (whispers have no server flood
    gate). Without a DSN every command is refused;
  - no-DB fallback: a `-commanders` allowlist of GM object ids.
- DB access from a laptop: MariaDB stays bound to 127.0.0.1; an `ssh -N -L 13306:127.0.0.1:3306`
  tunnel with a no-shell key restricted to `port-forwarding,permitopen="127.0.0.1:3306"`; a user
  `'acisbot'@'127.0.0.1'` granted only `SELECT(obj_Id, accesslevel, deletetime)` on `characters`, with
  `MAX_USER_CONNECTIONS 2`.
- Commands: `follow`, `assist`, `stay`, `go <spot>`, `invite me` (a bot invites only the bots the
  organizer or the panel assigned to its party and players who whisper `join`, and never invites
  unasked; it asks the GM to invite it, so the GM stays leader; `invite me` from a player takes a
  Support member of a bot party out, under the claim table, and is ignored by any other bot),
  `leave`, `logout`. `join` from any player to a bot party's leader with a free place invites the
  player (D28, `docs/parties/players.md`). `duel` from any player to a bot whose persona takes
  duels (`PvPPolicy.Duels`) asks for its challenge, and like `join` never reaches the authority
  (D36, `docs/pvp/finding.md`); other bots take it as chat. A GM's `invite me` and `leave` take the
  bot out of its bot party and clear its party slot first, as its `follow`, `assist`, `stay` and
  `go <spot>` do (`docs/parties/orders.md`, "Slot clearing").

### 7. Metrics

Per-bot counters summed periodically, histograms instead of unbounded latency slices, channel depth,
and a live `expvar` endpoint bound to loopback.

### 8. Control panel (D10)

A new `bots ui` command: one long-running process on the owner's machine that serves a web panel on
`127.0.0.1` and runs the bots in-process. `bots swarm` stays for headless runs and is built on the
same pieces. `bots ui` holds an OS advisory lock (`flock`, released when the process dies, so a
crash never blocks a restart) on its data directory (`-data-dir`) for its whole run, so two panels
never share a registry. Running a swarm on a prefix the panel's registry holds is the owner's call
to avoid: both would log in the same accounts and kick each other.

- Personas are presets in one catalog: `farmer` (the default, D21: the `damage` role, farms the
  datapack's spots alone), `support`, `town`, and `merchant` (the `damage` role with a
  store budget, D21: it farms the datapack's spots for loot and sells it in a town store; a `town` bot sells
  the loot of its farming spells to NPC merchants only), extended as personas land. A preset is a persona plus its
  options. Presets that need the datapack (`town`, `merchant`, farming spots) are offered only when
  the panel started with one. `bots swarm -persona` takes one preset name from the same catalog; `-support N` makes the last N
  bots of a `farmer` swarm play `support`.
- Bot parties (D25, `docs/parties/`): the supervisor runs the party organizer, which groups online
  `farmer` (Damage, the leader) and `support` bots of the same town into bot parties and hands their
  `lead`, `join` and `unparty` orders to the bots every `PartyTick` (30 s); `merchant`, `crafter`
  and `town` bots never take part. It is on in `bots ui`; `bots swarm` runs it over its own bots unless `-no-parties` (no registry,
  so no party mode and no pin there; #395). Each registry
  bot has a party mode, `""` (auto) or `off` (never in a bot party), and may be pinned to a named
  party (`PartyGroup`, its leader `PartyLead`), which forms whatever band and composition say and
  outlives a panel restart until a member leaves it or the panel dissolves it; auto parties live in
  the supervisor's memory and form again after a restart.
- Each bot gets its own `BrainConfig` from its preset: today `SessionConfig.Brain` is one field of
  the config every bot shares, so `Brain` moves out of `SessionConfig` and `NewBot` takes the
  preset's `BrainConfig` (with the bot's `Character`). The party-claims table is created once (today
  `applyPartyAndAuthority`, `cmd/bots/swarm.go` in this repository, creates it inside) and applied,
  with the GM authority hooks, to each preset's `BrainConfig`, so one bot per non-GM party leader
  holds across presets. The process-wide pieces stay one instance shared by every bot: the login
  gate and credential breaker, the spot registry, travel map, store registry, party claims, the GM
  authority and the metrics registry. A preset may still leave a piece out, as every preset but `merchant`
  leaves out the store registry. Every preset farms with the spot registry now: `town` got it with
  its farming spells (D44, #618). The
  metrics registry never frees a slot, so each bot of the registry gets one slot when it is first
  run, reused by every later run.
- Bot registry. Every bot the panel creates is stored locally, in `bots.json` in a data directory
  (`acis-bots` under `os.UserConfigDir` by default, `-data-dir` to change it; tests use their own),
  written atomically (temp file and rename). Per prefix it records the marker and an account
  counter. Per bot it records the prefix and account, the character name once known, the class, the
  "never create" flag (set for imported bots), the preset, whether it should be online, its last
  known level and position, its valuables ledger (M12, #404: `docs/valuables/ledger.md` "Across
  restarts"), and when it stopped and why. It holds no secret: passwords stay derived
  from the secret file.
- New bot: the panel takes the next account of its own prefix from a counter in the registry that
  only grows: an account is never handed out twice, and deleting a bot keeps a tombstone of its
  account. The server's `AutoCreateAccounts` creates the account on its first login, as `bots
  accounts` does today. The bot creates its character with the chosen class (random if none, drawn
  once and stored in the registry); an account that already holds characters is skipped (below).
- Online cap: `-max-online` bounds the bots online at once; the supervisor refuses a spawn past it
  and restores at most that many on start, and the server's `MaximumOnlineUsers` is raised by the
  same number (a login past it gets `PlayFail`, which the bot retries forever).
- Character pinning. The known name, the stored class and a "never create" flag go in the bot's
  `Character` (`internal/botclient/character.go` in this repository), and `NewBot` takes that
  `Character` from the supervisor instead of building it: today `NewBot` derives the names from the
  account and `-name-marker` and draws class and appearance itself, and a session finds its
  character only through the derived names (`chooseSlot`). The rules:
  - a bot with a known name selects that character in `CharSelectInfo` and never creates one; if it
    is gone (deleted or renamed on the server), the bot stops as "character missing";
  - every adoption records the class of the chosen slot from `CharSelectInfo` (today its class id
    is read and discarded), replacing the class the bot drew;
  - the class and appearance checks (`Character.Validate`, run by `NewBot` and `enterGame` today)
    apply only to a bot that may create a character; a known-name or "never create" bot carries no
    starting class (an imported one learns its class at first entry) and is not refused for it;
  - a new bot (no known name) first adopts a character bearing one of its derived names (its own,
    created by an earlier attempt that dropped before entering the world, or one an older swarm
    left on the prefix), then creates one on an empty account; on an account holding only other
    characters it stops as "account has characters", and the supervisor drops that account (a
    tombstone) and retries the new bot on the next one;
  - an imported bot (no known name, "never create") adopts the character bearing a derived name, or
    stops as "no character to adopt";
  - as soon as the slot is chosen or the character created (before `RequestGameStart`), a callback
    that `enterGame` fires hands the name to the `Bot` (today `enterGame` returns `Self{}` on every
    later error, so the name would be lost), and the `Bot` pins it as known, so every later attempt
    of that run, reconnects included, is a known-name login;
  - at the same moment the `Bot` reports the name and the slot's class id to the supervisor (a run
    event posted, without waiting, to the same mailbox as its end report and ahead of it), and the
    supervisor saves both to the registry at once, so a drop, crash or restart before the first entry never loses it.
- The derived names depend on the name marker, so the registry stores each prefix with its marker
  and placement, and the panel always uses the stored ones. Each bot gets its prefix's marker for
  its names (with its `Character`), while the brain's bot filter (no chat reply to a name carrying a
  bot marker) matches every marker the registry holds, so bots of different prefixes still see each
  other as bots; today `SessionConfig.Marker` serves both and is one per process. New bots go under
  the panel's own prefix and marker, given once when the registry is created (`bots ui -prefix P`,
  with `-name-marker` and `-name-marker-at` as in `bots swarm`).
- Accounts made before the registry (a prefix's accounts 1 to N from `bots accounts` or the swarm)
  are imported only on request (`-import-prefix P -import N`, with `-import-marker` and
  `-import-marker-at` when not the default, or the panel's import action with the same fields) into
  the registry under prefix P with that swarm's marker (the Hetzner unit ran prefix `hz` with the
  default marker), which the registry records. Passwords are derived from the secret file, so the
  home run imports with the secret file that swarm used (the Hetzner host's `/etc/acis-bots/secret`,
  copied to the owner's machine with mode 0600); another secret would make every imported login a
  refused credential. Imported bots have no known name, the "never create" flag and the `farmer`
  preset (`ImportPreset`, D21; `support` before M10, which bots already imported keep); the first login adopts the character with a derived name and records
  its name and its class, read from `CharSelectInfo` (today the class id there is read and
  discarded). The import moves prefix P's counter past N, also when P is the panel's own prefix, and
  a new bot never takes an account the registry already holds.
- The panel runs every bot persistent. Restarting the panel logs back in every bot marked online,
  each with its own preset, through the login gate. Despawn logs a bot out and marks it offline; it
  stays in the registry and can be spawned again later. A persona change logs the bot out and back
  in with the new preset. One run per account at a time: a new run of a bot starts only after its
  previous run reported its end, so the server never sees the account twice.
- A bot whose run ends by itself gets a state by its reason (`Bot.after` and `afterSession`): a
  `logout` command (`ErrCommandedLogout`) is offline; a bot ended by the tripped credential breaker
  (`ErrLoginsHalted`) stays online and comes back when the owner presses "resume logins", which
  resets the breaker (`LoginGate.Reset`). The breaker is a generation: a `{halt, err}` pair under
  the gate's mutex whose err is set before its channel closes, and a reset installs a new pair. `Take` and `Dial` fetch the current pair under the mutex; when a fired pair is no longer
  current (a reset came in between), they wait on the new one instead of failing, so no bot gets a
  nil error or a stale `ErrLoginsHalted` after a reset. A halted report that still reaches the
  supervisor while the gate is open restarts that bot at once; repeated server aborts
  (`ErrRepeatedAborts`, which a run of home line drops can cause for every bot at once) restart the
  bot by itself after a 10-minute cool-down; every other end (a refused credential, a taken or
  banned account, a character that cannot be created or is missing, any other error) is stopped with
  its reason until the owner starts it again. A kick is not an end: a persistent bot reconnects.
- Operator commands: `stay`, `go <spot>`, `leave` and `logout` reuse the whispered commands. The
  whispered `follow` and `assist` take no argument and target the GM who whispered (an object id,
  `internal/botclient/command.go` in this repository), so the panel's `follow <player>` and
  `assist <player>` are a new brain-side order: the brain resolves the name to a player in sight in
  its world model and refuses when none is. No panel command ever sends a Tell: the whispered
  replies (`unknown spot`, `that spot is full`, `I know no spots`) go back to the panel as the
  command's answer instead. The party commands (`docs/parties/orders.md`) are `lead <name>...` (lead
  a party of these bots), `join <leader> <member>...` (accept that leader's invitation, ahead of the
  persona and the claim table) and `unparty` (clear the party slot, withdrawing only from the bot
  party the slot named); `lead` and `join` answer "held by a GM's whispered order" while a GM's
  whispered `follow`, `assist`, `stay` or `go` holds the session. They reach a bot only through
  `Bot.Command`: the raw command route refuses them.
- Standing orders survive reconnects. Today orders belong to the brain and last for its session,
  and every session builds a new brain, so the `Bot` keeps the panel's standing order (`stay`,
  `go <spot>`, `follow` or `assist <player>`) next to its mailbox, re-applies it at each session
  start, and shows it in the snapshot; `leave` and `logout` are one-off. `lead` and `join` are
  standing the same way, in a party slot of their own beside the standing order, which they replace
  as a new panel order does; the slot outlives the sessions of one run and a new run starts with
  none. `unparty` is one-off. A standing `follow` or
  `assist` keeps its target's name and object id (stable for a character) and is re-resolved when
  that player comes in sight, since at session start the world model has not seen the player yet;
  only an order the panel sends to a running session is refused when the player is not in sight.
- Ownership: one supervisor goroutine owns the registry, its file and every bot's cancel func; HTTP
  handlers send it requests on a channel and get an answer at once. Each run posts its events
  (its bot, the character learned with its name and class id, then `{account, err}` at its end) to
  one mailbox the supervisor always selects on: a post never waits on the supervisor, so a report
  made while the bot holds its login gate turn never stalls the other bots' logins, and the
  supervisor takes the events in the order posted, so a run's learned character is saved before its
  end report is applied. A learned name and class, a new bot's account and counter, a tombstone and
  an import are written to `bots.json` before the change returns; a run's end, progress and online
  flag are applied in memory and written by one save a few seconds later (or by the next of those
  saves, or once at stop), so a mass end costs one write and a crash loses at most that record; a
  respawn or persona change of a running bot marks it stopping,
  cancels the run and starts the new one when that report arrives. Stopping the panel cancels every
  run, keeps draining reports until all have arrived, and joins every goroutine (bots, HTTP server,
  event streams, metrics); a bot's reader is joined before its next dial as today.
- Status: a new brain is built for every session, so the snapshot belongs to the `Bot`, which
  outlives its sessions, and the `Bot`'s goroutine (which also runs each session's brain) is its
  only writer. The brain publishes an immutable snapshot (character, class, level, HP/MP/CP,
  position (and the nearest town of the travel map when the panel has a datapack; the world model
  has no zone), preset, current rule, party, session state, reconnects, last error) through the
  `Bot`'s atomic pointer at most once a second, timed by the injected clock; between sessions the
  `Bot` publishes its offline state (reconnecting, waiting for the gate, stopped and why). The panel
  only reads the snapshots; the supervisor copies level, position and the valuables ledger into the
  registry.
- Commands: each `Bot` has a small bounded mailbox that outlives its sessions (an event queue
  exists only during a session and its push blocks when full). The supervisor never blocks on it: a
  full mailbox answers "busy". Each session's brain selects on the mailbox next to its event queue,
  as it does for authority verdicts, and every packet still goes through the per-connection pacer
  (frames per second, chat, bypass and emote gates). A command that arrives between sessions waits
  for the next one, or is refused when the bot is stopped. The commands are `stay`, `go <spot>`,
  `follow <player>`, `assist <player>`, `leave`, `logout`, and the party commands `lead`, `join`
  and `unparty`.
- Inspect (#437): opening a bot's page asks its live session once, over the mailbox
  (`GET /api/bots/{account}/inspect`), for its inventory, worn gear, penalties and effects. The
  brain sends one `RequestItemList` (no server gate: the frame bucket bounds it) and answers once
  the `ItemList` is applied, or with its model marked stale at the request's deadline; a bot with
  no session answers "offline". The panel allows one inspect in flight per bot, a per-bot
  cooldown and a global cap, names items and skills from the datapack, and stores nothing.
- HTTP: bound to `127.0.0.1`; a JSON API plus server-sent events for live updates. The page is a
  React and TypeScript application (D12) built from `web/` by `make ui` into
  `internal/botpanel/web/app`, which git ignores, and embedded with `go:embed`. A binary built
  without it serves a placeholder page that says to run `make ui`, so `go build` and `go test`
  need no Node; the CI `web` job and the release build run Node. Every response carries a strict
  content security policy (scripts, styles and connections from the panel only, no inline script,
  no framing). Passwords, the HMAC secret and the GM DSN never reach the API, the page or the logs.
- Logs: a `slog` handler keeps a bounded ring of recent lines (filterable per bot) for the panel,
  next to stderr.
- Pages: Bots (every bot of the registry with preset, class, level, HP/MP, position or town,
  activity and state; create new, spawn existing, despawn, change persona, delete from the
  registry), Bot detail (snapshot, commands, inspect, its log), Dashboard (gate and breaker with "resume
  logins", the metrics of section 7), Logs. The API adds the class names and the farming spots
  (with their caps and how many bots hold each) for the page.
- On WSL2 the Windows browser reaches the panel through WSL's localhost forwarding.
- Later, not in M6: saved spawn presets with custom persona values, a map of bot positions.

## Test strategy (bot repository)

- Unit tests: codecs, crypt against fixed vectors, brain decisions (events and an injected clock in,
  packets out), table-driven.
- End-to-end suite on the P1-P4 compose stack. Config: `AutoCreateAccounts = True`,
  `MaximumOnlineUsers` and `LoginTryBeforeBan` raised, accept filter off or the Docker gateway CIDR
  allowlisted (host bots appear as the gateway IP), project-scoped container names and ports.
- Seeding:
  - create the GM character over the protocol, then `UPDATE characters SET accesslevel = 7` while it
    is offline (nothing in the protocol grants the first GM);
  - the GM then uses admin commands over `SendBypassBuildCmd`: `//set level`, `//skill set`,
    `//spawn` at its target, `//kill`, `//heal`;
  - there is no `//set hp`, so "GM below 60 % HP" uses SQL `curHp` before login or real damage.
  Direct SQL couples the suite to the server schema; keep it to these two statements.
- Real time with a timeout per scenario. Scenarios run beside a town, spawn fresh monsters per
  scenario (spawnlist respawn is 60 s ± 20 s), and use a warm geodata cache volume.
- Nightly e2e against acis_golang `main`; a failure files a protocol-drift issue.

## PR breakdown (bot repository)

| PR | Scope | Evidence |
| --- | --- | --- |
| 0 | New repository: module, copied wire/crypt/login/game client, local opcode constants, copied test oracles, CI; e2e harness on the P1-P4 stack | Existing login-enter-logout flow passes e2e |
| 1 | Reader decode-to-events, brain loop with injected clock and `done` teardown, PR1 codecs and world model, `Appearing` after teleport, character creation with race/class/appearance and the nickname marker, `-persistent`, safe reconnect, login pacing and credential breaker, pre-created accounts with per-bot passwords, metrics, `-record` tap | Bot stays in world; after a kick it reconnects once the old character leaves; after `//recall` it reappears; a credential failure halts logins without a ban |
| 2a | Party accept, follow slot, GM Tell commands with both authority options | GM whispers `invite me`/`follow`; bot is invited, follows at slot distance; a non-GM whisper is ignored and triggers no DB query after the negative cache |
| 2b | Assist, heal and buff with skill pacing, trainer skill learning, rebuff from timers, potions and auto soulshots, death policy, resurrection accept, leader edge cases, PvP defaults | Level-15 human mystic learns Heal at the trainer, heals the GM below 60 % HP, assists only after the GM hits a monster, dies and returns to the leader |
| 3 | Farmer persona, datapack readers (NPC templates, spawnlist territories), spot registry, loot queue, drop, stuck and blacklist (anti-KS dropped, #138; stuck handling replaced by the M7 walker, D13) | Kills a monster, loot row in `items`, exp grew |
| 4 | Town and merchant personas: merchant buy and sell, gatekeeper, Newbie Helper buffs, trade defaults, private store with registry and pricing | Adena and items change in DB after buy, sell, a player trade and a store sale; teleport reaches the spot and the bot reappears |
| 5 | Progression: better spots by level, more skills at the trainer, equip bought gear | Bot learns a skill it can now afford; skill row in DB |
| 6a | Home run (D9): `-allowlisted` without `-local-addr` in `bots accounts` and `bots swarm`; dialer with `TCP_USER_TIMEOUT` and a `KeepAliveConfig` so dead sessions end in about a minute, and sessions ended by those timeouts not counted as aborts; AccountInUse retried for 3 minutes from the first AccountInUse after a run start or any non-kick end (failed entries included); `docs/home.md` in this repository (provider range on both servers, public `Hostname` or `-game`, `AutoCreateAccounts` on, swarm flags, secret file and account prefix on the owner's machine (the Hetzner host's secret file copied over to re-use its `hz` bots), local datapack copy, GM control over the SSH tunnel, WSL2 notes); retiring the Hetzner run (all paths in this repository: `docs/hetzner.md` and `ops/acis-bots.service` kept as an archive pointing to `docs/home.md`; the owner's steps in `docs/home.md`: stop and disable the unit, replace the `127.0.0.2` allowlist entry; `README.md`, `docs/gm-control.md` and the "shared host" lines of `CLAUDE.md` and `.claude/skills/bot-issue/SKILL.md` updated) | Unit: `-allowlisted` accepted without `-local-addr`; the dialer's socket options (user timeout and keepalive) come from a plain function tested on its own, with the `setsockopt` call behind a small seam, so the test needs no network and no build tag; a session ended by a timeout is not counted as an abort, EOF and a reset are; the window starts at the first AccountInUse, however late it comes after a run start or a non-kick end (failed entry included), retries come at most 30 s apart, and only after 3 minutes of AccountInUse does the bot stop, while after a kick the first one stops it as today. e2e on the shared stack (its Docker gateway is allowlisted): `-allowlisted` without `-local-addr` logs in unspaced |
| 6b | Persona catalog and per-bot `BrainConfig` out of `SessionConfig`, with the authority and party-claims wrapping per preset; starting class per bot; known character name, stored class and "never create" flag in the per-bot `Character`, which `NewBot` now takes instead of building; the pinning rules of section 8 (select a known name or stop as missing; a new bot adopts its derived-name character or creates on an empty account; pin and report the name through a callback that `enterGame` fires after `chooseSlot` or the creation, before `RequestGameStart`); `bots swarm` on the catalog | Unit: each preset yields its persona; shared registries and the party-claims table are one instance across presets, and `town` gets the spot registry too (D44, #618; it had none before); a set class reaches `RequestCharacterCreate`; a known name selects its slot and sends no `RequestCharacterCreate`; a known name absent from `CharSelectInfo` stops the bot as character missing; a known-name or never-create bot with no starting class passes `NewBot` and `enterGame`; a new bot adopts its derived-name character first and records that slot's class, creates only on an empty account and stops on one holding only other characters; the name is pinned and reported once the slot is chosen or created, before entry, so a drop between `CharCreateOk` and `UserInfo` and a retry or restart reuse that character; an imported bot adopts a derived-name character or stops; a reconnect after the name was pinned logs in by it even if the character was deleted meanwhile (it stops instead of creating); the learned name is reported once. e2e: `bots swarm -persona merchant` farms and opens a store |
| 6c | Bot registry file; `-data-dir` and its advisory lock in `bots ui`; supervisor (account counter with tombstones, `-max-online`, new, existing and imported bots, saving the learned name at once, restore on start, despawn, persona change, end-reason states); resettable breaker; status snapshot owned by `Bot`; per-`Bot` command mailbox | Unit (injected clock, fake bot runner, temporary data directory): a new bot takes the next account from the counter, never a deleted bot's account, skips an account holding characters, and the registry holds its character name as soon as the run reports it; `-import-prefix P -import N` adds P's accounts 1 to N with the never-create flag and moves P's counter past N, the panel's own prefix included; a new bot never takes an account the registry holds; the chat filter matches the markers of every prefix; an imported bot not yet adopted keeps its flag across a restart and never creates; spawns and restores stop at `-max-online`; an existing bot relogs into its stored character; a restart logs back in the bots marked online with their presets and creates no character; imported accounts record the name found on first login; a respawn starts only after the previous run reported, and a respawn whose previous session the server still holds rides out AccountInUse instead of stopping; each end reason maps to its state; "resume logins" brings back the bots ended by the breaker, a reset racing `Take` and `Dial` passes `-race`, and a `Take` or `Dial` whose halt fired before a reset waits on the new generation instead of failing, and a halted report arriving after the reset restarts its bot at once; the learned-character event carries the class id and the registry saves it; the registry keeps the marker and an import with it adopts the old characters and learns their class; a second `bots ui` on the same data directory is refused by the lock, and a lock file left by a killed process does not block a restart; a bot ended by repeated aborts restarts by itself after the cool-down (injected clock); `follow <player>` resolves a player in sight and is refused otherwise; a `stay`, `go` or `follow <player>` from the panel is still in force after a reconnect and shows in the snapshot, the follow target resolving once the player comes in sight; a panel `go` to an unknown spot answers the panel and sends no Tell; a full mailbox answers busy without blocking; every goroutine joined on stop. e2e driving the supervisor in-process: create a bot, stop the supervisor, start a second one on the same data directory: the same character logs back in and no new character row appears |
| 6d | `bots ui`: loopback HTTP server, JSON API, event stream, log ring, embedded page; then (#106, D12) the React page from `web/` with its placeholder, `make ui`, the CI `web` job, the release build's Node step, the class and spot endpoints and the CSP | Unit (`httptest`): the API creates, spawns, despawns and changes presets through the supervisor; no secret appears in any response, served asset or log line; the class and spot endpoints; every response carries the CSP; the placeholder is served without a built page. Vitest: the bots table, the command form and the event-stream hook with its reconnect. e2e: create two bots of different presets from the API, despawn one, spawn it again: it comes back as the same character |
| 7 (M7, N0-N8) | Own navigation (D13, `docs/navigation.md`): geodata copy packed and mapped, cell planner with exact stretch check, hierarchy and nav cache, door table, the walker (stand points, blocked and desync replans, isolated-only `/unstuck`), spots and trips on SCCs, follow on routes, `nav_segments_total` | Live nav scenarios: a long walk, the step chain along walls and ledges with every blocked stop checked against `WalkSteps`, the clan hall doors toggled by the GM (only door ends), melee round an obstacle; `nav_desync_total` 0 on door-free routes, `nav_blocked_total` at most 1 per 100 segments |

Class change is at the class master under D15 (M8, #178). The village master path, which needs
quest items, stays out of scope.

The live control API deferred here before is the control panel of section 8 (D10).

## Server-side work for D2 (acis_golang)

Under D9 the bots no longer share the server's host, so S2 (a unix-socket listener, which needs
bots on the same host) does not apply; it would need its own host decision first. S1 stays useful
as server measurement.

| Item | Scope |
| --- | --- |
| S1 | Measurement: server metrics for outbound frames and bytes per second, high-water aborts, sim queue lag, handler latency, heap and GC pauses; clustered and spread runs of 30 minutes or more at 500/1000/2000 bots on another host or cgroup; a probe client measuring move round trip p99; pprof split of network-write CPU versus world CPU; D2 threshold written before the runs |
| S2 | Conditional on S1, own design approval: a unix-socket listener only (never TCP), mode 0660 in a group-owned directory, cipher off per connection and set only by that listener, opcode filter in `Session.send` for those connections (after `CopyFrame`, so it saves the write, not the copy), a connection cap, and a synthetic IP per connection for bans and accounting; AuthLogin keys and flood protection still apply |

## Gap handling

Server features a bot hits that are not ported yet get a follow-up issue in acis_golang in their
owning milestone. The bot logs the gap and degrades; no server special-casing for bots.

## Open questions

- Q1: Repository name and visibility (placeholder `fatal10110/acis_bots`), and whether it joins
  `acis_public` as a submodule.
- Q2: The nickname marker text and whether it is a prefix or suffix.
