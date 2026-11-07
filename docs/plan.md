# Playable Bots Plan (#4522)

Status: draft. Revised after five aspect reviews (feasibility, concurrency and scale, security and
abuse, repo rules and scope, gameplay), the owner's answers of 2026-11-07, and three re-reviews
(separate-repo feasibility, consistency, operations and abuse under D6).

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

Server-side prerequisites: fatal10110/acis_golang#4525, fatal10110/acis_golang#4526,
fatal10110/acis_golang#4527, fatal10110/acis_golang#4528. The PR numbers in "PR breakdown" map to
milestones M0-M5 (PR2a and PR2b are both M2).

## Goal

Bots that play like real users so people have fun playing alongside them: follow and assist a
player, farm, loot, rest, die and come back, party, trade, sell in town, level up. Launched en masse
from a local machine or from the Hetzner host. Load testing (#260) is not the goal.

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
| P1 (#4525) | Publish a gameserver/loginserver image to GHCR per `main` commit |
| P2 (#4526) | Non-interactive gameserver registration for compose |
| P3 (#4527) | Config bundle that does not need the Java tree |
| P4 (#4528) | A small geodata subset (2-3 test regions) as a CI artifact; `geoengine.properties` lists only those regions |

Server config, not code, for every bot deployment: the accept-filter allowlist
(`AcceptFilterAllowlist`, both `loginserver.properties` and `server.properties`), a loopback
`GameserverHostname` for same-host bots, and `MaximumOnlineUsers` raised by the bot cap.

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

- anti-KS: monster targets are inferred from `MoveToPawn` (NPC chasing), `Attack` and
  `MagicSkillUse`; skip a monster engaged by a non-party player in the last ~10 s. Aggro held before
  any action is invisible to the client, so this is a heuristic;
- PvP defaults (configurable, D6): assist and fight-back target NPCs, or players who are flagged or
  have karma; never force-attack an unflagged player with no karma; stop fighting back when the
  attacker unflags; do not heal or buff a flagged party member (that flags the bot);
- blacklist a target after N failed attempts; stuck detection over 2.5 s, then a side step;
- reaction delay 300-1200 ms before acting on a new event;
- server limits: 160 frames/s burst, 80/s average over 5 s, disconnect after 2 floods per minute;
  bypasses inside 100 ms dropped silently; emotes every 2 s; chat flood gates; traffic within the
  15 minute idle timeout.

### 3. Personas

**support** (D1):

- joins a party on invite from a GM commander, and from other players when its persona allows it;
  at most one support bot per non-GM leader;
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
  trainer with SP (`AutoLearnSkills` is off), from PR2b.

**farmer** (PR3): spot choice from spawnlist territories (polygons with minZ/maxZ) and NPC levels,
excluding makers with `event=` or `ban=`, raid and grand bosses, non-monsters, aggressive mobs
(`aggroRange > 0`) above the bot's level, and likely quest mobs (no data marker; heuristic on type and
spawn flags). A process-wide spot registry (one owner) caps bots per spot at about `maximumNpcs`/3; on
arrival the bot counts visible players and moves on when crowded. Each race starts in its own newbie
zone.

**town** (PR4): idles, sits, buys potions and shots, sells loot to merchants, uses the gatekeeper,
visits the Newbie Helper for buffs (levels 8-24).

**merchant** (PR4):

- opening rules (server): standing, out of attack stance and PvP flag, not casting, no pending trade
  request; jail is the only no-store zone. Sequence: open action, manage list,
  `SetPrivateStoreMsgSell` (at most 29 UTF-16 units), `SetPrivateStoreListSell` (rows capped by
  `PrivateSellStoreLimit`); the server seats the character. A teleport closes the store; a sold-out
  store closes on its own, seen as the store type in `UserInfo`;
- sells loot only, never items NPC shops sell; price between 1.2 and 1.5 times the NPC buy-back price
  and below the NPC shop price;
- a process-wide registry keeps spacing between bot stores and at most about 5 bot stores per town;
  a per-bot sales budget per session.

**trade defaults** (every persona, configurable):

- trades are fixed-price sales of the bot's own goods for adena; a GM is exempt;
- confirm only after `TradeOtherAdd` shows enough adena, and only within 150 units of the partner
  (the server requires it); offers lock at the first confirm and requests expire after 15 s;
- never offer equipped or quest items; refuse incoming items that push weight above 50 %;
- rate-limit trade requests per partner; no trading while in store mode (the server forbids it).

Tunables are flags until a persona needs a file.

### 4. Names and human feel

- Character names match the server rule `^[A-Za-z0-9]{1,16}$`: a syllable-based base plus a
  configurable alphanumeric marker (D7), within 16 characters. No title marker. Ships in PR1, since
  bots are visible from PR1.
- Account names are separate from character names and never shown in game.
- Random hair and face, staggered logins, reaction delays, idle pauses and turns.
- Chat only when answering a whisper, after a delay; rate-limited per sender and capped per bot in
  total; never answer a name carrying the bot marker (no bot-to-bot loops); never on Shout or Trade.
- Uniform jitter; no seeded distributions.

### 5. Sessions, reconnect, accounts and host

These keep the server and the bot host working; they are operational, not player safety (D6).

- `-persistent` keeps bots in world.
- One session object per attempt: join its reader before dialing again; start a fresh world model.
- A dropped character stays in the world 100 ms, or 15 s in combat; login meanwhile answers
  AccountInUse, which is retryable with backoff.
- One process-wide login rate limit with jitter, under the accept filter's 3 per second per IP.
  While the server is down (connection refused) all bots back off exponentially, re-jittered.
- Process-wide credential breaker: the login ban counts failures per IP, not per bot (3 in a row ban
  the IP for 10 minutes, and on one host that ban also cuts the gameserver link). The first
  credential `LoginFail` halts all logins and alerts.
- Stop reconnecting a bot after repeated server aborts (the 2 MiB outbound cap) and report it.
- Accounts are pre-created (a `bots accounts` step), each with its own password derived as
  HMAC(secret, account) from a 0600 secret file. Each swarm instance uses its own account prefix. A
  kick caused by another login is reported, not fought.
- On the Hetzner host bots connect from a dedicated source address (for example 127.0.0.2,
  `Dialer.LocalAddr` on both the login and game dials) to a loopback game address; only that address
  is on the allowlist. Remote runs from a home or carrier-shared IP pace under the burst limit.
- A fixed bot cap; `MaximumOnlineUsers` is raised by the same number. Bots cannot see the live
  player count, so there is no dynamic log-off.
- The host is small (2 vCPU, 3.8 GB; server heap about 1.4 GB). Bots run in a systemd slice with
  `CPUQuota`, `MemoryMax`, `Nice` and `OOMScoreAdjust=+500`, so the OOM killer takes bots, not the
  gameserver.

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
- Commands: `follow`, `assist`, `stay`, `go <spot>`, `invite me` (the bot invites nobody; it asks the
  GM to invite it, so the GM stays leader), `leave`, `logout`.

### 7. Metrics

Per-bot counters summed periodically, histograms instead of unbounded latency slices, channel depth,
and a live `expvar` endpoint bound to loopback.

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
| 3 | Farmer persona, datapack readers (NPC templates, spawnlist territories), spot registry, loot queue, drop, anti-KS, stuck and blacklist | Kills a monster, loot row in `items`, exp grew; a monster engaged by another player is skipped |
| 4 | Town and merchant personas: merchant buy and sell, gatekeeper, Newbie Helper buffs, trade defaults, private store with registry and pricing | Adena and items change in DB after buy, sell, a player trade and a store sale; teleport reaches the spot and the bot reappears |
| 5 | Progression: better spots by level, more skills at the trainer, equip bought gear | Bot learns a skill it can now afford; skill row in DB |

Class change is out of scope: the class master NPC is not in the shipped spawnlist and the village
master path needs quest items. Revisit when bots can run quests.

A live control API is deferred: flags and GM whispers cover D1. If built: a 0600 unix socket
forwarded with `ssh -L`, or a bearer token; Host check; JSON POST only; capped spawn count.

## Server-side work for D2 (acis_golang)

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
