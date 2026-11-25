# Live e2e runbook

The e2e scenarios (`internal/e2e`, `cmd/bots/ui_live_test.go`) run real bots against the live
server from the owner's machine (#107). They never run in CI, never touch the server's database,
and never edit the server's config: a scenario the server cannot satisfy is recorded on the issue.

## Owner prerequisites

- **Login server:** `AutoCreateAccounts = True` (the first login of an `e2e` account creates it),
  `MaximumOnlineUsers` headroom for 8 more characters per test process (2 scenarios of 4).
- **GM characters:** at least one existing account whose character has access level 7, created
  over the protocol and promoted once by the owner: the root GMs. A run that pins no GM file
  mints the GMs its scenarios use (owner decision on #367, "GM pool" below), so the pool has no
  size limit; the harness promotes only the GMs it mints, and only through a root GM.
- **Config files** in `.e2e/` at the root of the primary checkout (gitignored; dir 0700, files
  0600, refused when readable by others):
  - `secret`: the HMAC secret the `e2e` account passwords derive from (strong, as persistent mode
    wants it);
  - `gm`, `gm2`, `gm3`, ...: the owner's GMs, one GM file each, three `key=value` lines,
    `account=`, `password=` and `character=`: the roots of minting, or the pinned pool. The
    harness keeps a lock file next to each (`gm.lock`, ...).
  - `minted-<prefix>` (`minted-e2e`): written by the harness, the accounts it minted for the
    prefix, one name a line, never a password; refused like the others when readable by others.
    Its lock file (`minted-e2e.lock`) is the mint lock, and each minted GM has a lock file of its
    own (`e2emint1.lock`, ...).
- **Allowlist:** set `BOT_E2E_ALLOWLISTED=1` only when the source address is on
  `AcceptFilterAllowlist` of both servers; otherwise logins are paced by the bot's login gate.

## Configuration

Environment variables only, each a value or a path, never a secret:

| Variable | Meaning | Default |
| --- | --- | --- |
| `BOT_E2E_LOGIN` | login server `host:port` | none: every scenario skips |
| `BOT_E2E_SECRET_FILE` | secret file | `.e2e/secret` of the primary checkout |
| `BOT_E2E_GM_FILE` | GM file, or a list of them (`:` between, `;` on Windows): pins the run's GM pool, which then never mints | unset: the run mints its GMs, rooted in every `gm`, `gm2`, ... of the primary checkout's `.e2e/` |
| `BOT_E2E_PREFIX` | account prefix; refused when it overlaps `hz` or the owner's `bots ui` panel prefix | `e2e` |
| `BOT_E2E_ALLOWLISTED` | `1`: no login spacing | unset |
| `BOT_E2E_DATAPACK` | local datapack XML directory (the panel's `town` preset) | none |
| `BOT_E2E_ALLOW_LOGINFAIL` | `1`: run the wrong-password scenario | unset |
| `BOT_E2E_GEODATA` | geodata region directory the caches were built from (docs/home.md, section 3) | none: every whole-bot scenario skips |
| `BOT_E2E_GEOENGINE` | local copy of the server's `geoengine.properties` | none |
| `BOT_E2E_GEODATA_CACHE` | geodata cache of `bots geopack` | none |
| `BOT_E2E_NAV_CACHE` | nav cache of `bots navbuild` (also needs `BOT_E2E_DATAPACK` for `doors.xml`) | none |
| `BOT_E2E_SOAK` | soak length | `16m` (past the 15 min idle timeout) |
| `BOT_E2E_RECORD` | directory the login scenario records fixtures into (the clan scenario into its `clan` subdirectory) | a temporary one |
| `BOT_REQUIRE_E2E` | `1`: a scenario whose config is missing fails instead of skipping | unset |

Every scenario that runs a whole bot (and the panel scenario) walks on the navigation (D13a): it
maps the two caches once per test process and skips without them, or fails under
`BOT_REQUIRE_E2E=1`. Build both caches outside the worktree, never in it.

The default files are found through git (`git rev-parse --git-common-dir`), so a run from a
linked worktree reads the primary checkout's `.e2e/`.

## Running

```bash
BOT_E2E_LOGIN=l2-login.iterharbor.com:2106 BOT_E2E_DATAPACK=/path/to/aCis_datapack/data/xml make e2e-live
BOT_E2E_LOGIN=l2-login.iterharbor.com:2106 make e2e-live-loginfail   # alone: it trips the breaker
```

One group at a time: `go test -count=1 -v -run '^TestLiveRecall' ./internal/e2e/` with the same
variables and `BOT_REQUIRE_E2E=1`.

`make e2e-live` runs in parallel:

- **Scenarios:** inside a test process, every scenario but the soak (it counts the process's
  goroutines) and the wrong-password one calls `t.Parallel`, and at most 2 run at once
  (`e2e.MaxScenarios`, `-parallel 2`). Scenarios that share ground take a group lock and run one
  at a time: the farmers on the newbie spots (farm, spots, arrows), and the bots sent to
  the village's grocer, stores, trainers and merchants. Each scenario logs in accounts of its own
  (an account group no other scenario uses): a login to an account already in game evicts that
  session.
- **GM pool:** each scenario that needs a GM leases one from the pool through a lock on its lock
  file, held until the GM logs out, so two scenarios of one process, of two processes or of two
  runs never share a GM. The kernel drops the lock of a process that dies, so a killed run never
  blocks a later one.
  - **Minting** (no `BOT_E2E_GM_FILE`, #367): the pool is the GMs minted for the prefix
    (`.e2e/minted-<prefix>`). With every one held, the scenario mints the next instead of
    waiting: account `<prefix>mint<N>` (`e2emint1`, `e2emint2`, ...; no scenario may use the
    account group `mint`, and a root GM file's account is never minted), its password derived from the secret like every `e2e` account's, logs in once
    (the login server creates the account) and creates its character `<prefix>mint<N>Bot`, which
    then logs out; a root GM (a free one of `.e2e/gm*`, leased like a pool GM for the few
    seconds it takes) sends `//set access <prefix>mint<N>Bot 7`, which the server stores for the
    character out of the world ahead of its next login (`admin_access.go`,
    `admin_offline.go`), so the GM's next login is a GM's and needs no relog. Then the name is
    recorded and the scenario plays the new GM. One mint runs at a time over every process (the
    mint lock); a scenario that finds the lock held waits for a GM another frees or for its own
    turn to mint, at most 2 minutes. A mint that fails records nothing; the next one mints the
    same `N` again and adopts the character already made. Minted GMs stay on the server and in
    the record, and later runs lease them before minting any more. Minting adds two logins (the
    new account's and the root's) through the run's login gate and breaker.
  - **Pinned** (`BOT_E2E_GM_FILE`): the pool is the listed files and never mints. With every GM
    held, the scenario waits for one at most 2 minutes, on a bound of the pool's own, apart from
    its deadline, then fails with `GM pool exhausted` (#250). A pinned run whose pool holds fewer
    GMs than the scenarios it keeps open at once (`-parallel`, at most 2) says so as it starts.
- **Packages:** `./internal/e2e/` and `./cmd/bots/` are separate test processes, each with its own
  login gate. They run at the same time only with `BOT_E2E_ALLOWLISTED=1`: two spaced gates
  dialing at once would burst the accept filter.

### Parallel runs

Live runs may run at the same time, as real bot runs do (several agents, or an agent and the
owner's own bots). Each concurrent run needs:

- **GMs:** the second login of an account drops the first, so two runs never share a GM session.
  Runs of this harness lease GMs through the per-GM locks and mint more when every one is held,
  so they share the pool with no pin: run the related GM scenarios in one invocation with
  `-parallel 2` (a test process keeps at most 2 scenarios open, `e2e.MaxScenarios`; a third
  waits for a scenario, not for a GM). A run of an older harness that cannot mint still pins its
  own GM files (`BOT_E2E_GM_FILE=.e2e/gm2`, or a list), and with one GM pinned runs its GM
  scenarios one at a time (`-parallel 1`).
- **Its own lock** when the run goes through `scripts/agent-tools/heavy`: `heavy` puts every e2e
  run (an argument naming `internal/e2e`, an `e2e-live` target or a `TestLive` scenario) on the
  e2e lock with a 60m timeout, never on the unit-test and lint lock, so live runs queue behind each
  other only. A run that may go in parallel (its own GMs and prefix) sets a `HEAVY_LOCK` of its
  own (`HEAVY_LOCK=/tmp/acis-bot-live-gm2.lock`).
- **Its own scenarios or its own prefix:** two runs of the same scenario at once log in the same
  `e2e` accounts and drop each other; give one of them another `BOT_E2E_PREFIX`.
- **Headroom:** each scenario keeps its cap of 4 characters online and a test process keeps at
  most 2 scenarios open, so the login server's `MaximumOnlineUsers` needs 8 more per concurrent
  test process.

Every scenario tolerates other players and bots around it: another run may play in the same town
at the same time. A scenario asserts only on its own characters, the monsters it spawned and the
packets about them (the bot's own inventory, a skill cast on the bot, a GM seeing the bot), never
on a global count, on who else is in sight, or on the absence of everyone else.

## Safety envelope

Operational limits, not persona defaults; unit-tested with a fake target (`internal/e2e`):

- **Login pacing and the credential breaker:** every harness login of a test process passes one
  `botclient.LoginGate`, spaced under the accept filter unless allowlisted. The panel scenario's
  bots log in through the panel's own gate and breaker; the scenario first checks the panel's
  first account through the run's gate. The first refused credential halts every later login of
  the process and leaves the marker `halted` next to the secret file (account and reason, never a
  password). While the marker exists every scenario fails before it logs in, checked when the
  process opens the run, again before each scenario and before each harness login, so a test
  process running beside the one that tripped stops at its next login: the login server
  counts failed logins per address over all accounts for at least an hour, only a good login of
  the same account takes one back, and 3 ban the address (`LoginTryBeforeBan`). After a refusal,
  fix the secret file, then delete `.e2e/halted`. The wrong password is sent at most once a run,
  only with `BOT_E2E_ALLOW_LOGINFAIL=1`, after a good login of the same account, and leaves no
  marker: the next good login of that account takes its failure back.
- **Cap:** at most 4 characters of a scenario online at once, the GM's and a panel's bots
  included, and at most 2 scenarios of a test process open at once; every scenario logs its
  characters out in cleanup.
- **GM commands** (`e2e.Scope`) touch only the scenario's characters and the monsters it spawned:
  `teleport X Y Z`, `teleportto|recall|kick NAME`, `kill|res|heal|cancel [NAME]`, `set level N`,
  `set sp N` (at most 100000), `skill set` and `give ID [COUNT]` (at most 10000) on a targeted run
  character, `hide` (the GM's own invisibility), `spawn ID`, `delete` and `para` (paralysis) on a spawned monster. The forms
  that reach others (`kick all`, `recall party|clan`, a radius) and every server-wide command
  (announce, shutdown, reload) are refused before anything is sent. `set access` is refused to
  every scenario GM; a root GM promoting a minted one may send only `set access NAME 7`, NAME a
  character of the minted namespace (`<prefix>mint<N>Bot`), never another level (a ban is one
  below 0) and nothing else. The GM targets only run
  objects, so a command that falls back on its target cannot reach anyone else.
- **Pacing:** every frame a scenario sends passes the bot's pacer (`botclient.Pacer`); GM
  commands are also spaced like dialog clicks.

## What a run creates and leaves behind

- Accounts `<prefix><group><n>` (`e2ea1`, `e2ec1`-`e2ec6`, `e2er1`, `e2ew1`-`e2ew2`, `e2es1`,
  `e2ept1`, `e2esw1`-`e2esw3`, `e2ewh1`-`e2ewh2`, `e2ewl1`-`e2ewl2`, `e2efo1`, `e2efb1`, `e2esu1`-`e2esu3`,
  `e2erv1`-`e2erv3`, `e2ef1`, `e2ek1`, `e2en1`-`e2en2`, `e2em1`, `e2etw1`, `e2eg1`, `e2etd1`-`e2etd2`,
  `e2est1`-`e2est3`, `e2ems1`, `e2ech1`-`e2ech2`, `e2et1`, `e2egw1`, `e2egb1`-`e2egb2`, `e2ear1`,
  `e2epn1`, `e2ewm1`, `e2evm1`-`e2evm3`) and
  the panel's `<prefix>p1`, `<prefix>p2`, each with one character whose name carries the bot
  marker. They stay on the server; later runs adopt the same characters.
- The valuables scenarios' buyer `<prefix>vb1` (`e2evb1`), no GM, with one human fighter whose
  name does **not** carry the bot marker, so the merchant answers it: the first of the fixed
  candidates `buyerNames` (`internal/e2e/valuables_buyer_test.go`) the server accepts; the
  scenario fails before it logs in when the marker marks one (`checkUnmarkedNames`). It destroys the valuables it bought
  when the scenario ends and keeps the adena it was given. `e2evm2`, the warehouse scenario's
  merchant, ends each run carrying one Sword of Watershadow and one Tears of Eva; the next run's
  scripted login destroys that Tears (and any second sword) before its bot logs in.
- The chat scenario's speaker `<prefix>ch3` (`e2ech3`), no GM, with one human fighter whose name
  does **not** carry the bot marker, so the bot under test answers its whispers: the first of the
  fixed candidates `speakerNames` (`internal/e2e/chat_speaker_test.go`) the server accepts, checked
  like the buyer's. Every GM, minted or pinned, carries the marker, so no GM can be the speaker
  (#485).
- The minted GMs `<prefix>mint<N>` (`e2emint1`, ...), each with one character `<prefix>mint<N>Bot` of
  access level 7, recorded in `.e2e/minted-<prefix>`; later runs lease them again. They stay
  until the owner retires them (`//set access <name> 0` from a GM, then delete the record's
  line).
- Characters end where the scenario left them (a recalled bot next to the GM, a killed one in a
  village); nothing is restored but the level below.
- `TestLiveFarmFight` sets its farmer to level 5 and runs it with every item but adena, potions
  and shots marked as junk: each run the farmer drops all its unequipped loot at its spot, where it
  lies until the server clears it, so the farmer's inventory never fills up across runs. The GM then
  gives it Power Strike, 10 Healing Potions, 500 no-grade soulshots and 3 Stems (a gift waits while
  the inventory is full); what it did not use stays with it. Other farmers keep their loot and the
  levels they gained, but `TestLiveNavPickUpObstacle`, whose level 5 farmer drops its junk the
  same way; `TestLiveSpots` sets its first farmer back to level 1 at the end, as
  `TestLiveFarmStarters` and `TestLiveSpotSpread` do theirs (`runFresh`: a character a failed
  cleanup left above the scenario's level is set back to 1, then logs out and enters again, so its
  new brain claims a spot for level 1, #268), and `TestLiveSupplyTrip` sets its farmer to level 6 at the start (#258: past level 14 in its
  Squire's gear, every Talking Island spot that fit it was an aggressive pack that killed it).
- `TestLiveArrows` makes the GM visible (`//hide` toggled off, it logs in invisible and no monster
  fights an invisible player) and lets the GM land the first hit on a spawned wolf (`gmProvoke`),
  which then fights the GM: the datapack's monsters of the Warrior AI attack nobody on sight.
- The M4 scenarios seed with `give` only: adena for purchases, trades and the town bot's gatekeeper
  hop (Gludin, 18000), Stems (1864) for the merchant visit's sale and the trade, and Steel Powder
  (1664), loot no NPC shop sells, for the stores. The
  merchant bot `e2em1` spends all its adena on Lesser Healing Potions each run, the town bot
  `e2etw1` all but 19000, and both keep what they bought; the town bot ends in another town, the
  Newbie Guide's bot `e2eg1` at level 8 with the newbie buffs. The buyers keep what they
  bought and the adena they did not spend. `TestLiveChatReplies` has its speaker say one line on the
  shout and one on the trade channel ("e2e test line, please ignore"), which other players in
  the region see.
- `TestLiveTrainer` sets its mystic to level 1 with no SP at the start and the end (the server
  takes away the skills a lower level does not allow), and in between to levels 10 and 15 with
  20000 SP; it ends next to the village's trainers. The GM gives a Spellbook: Heal only when the
  trainer asks for one; learning takes it.
- The gear scenarios (`TestLiveGearWear`, `TestLiveGearBuy`, `TestLiveArrows`) first enter each of
  their characters alone and strip it to its starting set: every weapon, armor and jewelry piece
  and every arrow an earlier run left it, worn or not, is destroyed (`RequestDestroyItem`, sent by
  the harness only: the server refuses to drop the starting weapons, and drops would litter the
  village), and a starting piece it lost is given by the GM and worn again. The buyer of
  `TestLiveGearBuy` starts from a GM-given Short Sword, Shirt and Pants instead, as the starting
  set cannot be sold. For the buying scenarios the GM then tops the character's adena up to 30000
  (gives of at most 10000) and recalls it next to its first merchant, so that its bot logs in with
  something to buy in reach (a bot that finds nothing to buy holds its shop errand off for five minutes);
  `TestLiveGearWear`'s GM gives the pieces instead. The bots keep what they bought and the adena
  left, until the next strip.
  Every strip also lifts the death penalty earlier runs left the character, so a scenario judges
  the same character every run: it logs the level from the entry `EtcStatusUpdate`, the GM gives
  as many Scrolls: Recovery of the character's level band as it lacks, and the character reads
  one per 10 s reuse until the server reports the penalty lifted (SystemMessage 1917). A scroll
  the server refuses or never answers fails setup, naming the level.
  `TestLiveArrows` ends with the GM visible at the Talking Island newbie spots.
- `TestLiveFarmFight`'s GM tops the farmer's soulshots up to 500 rather than giving 500 every
  run: its pinned character keeps them, and they weigh 4 each.
- Monsters the GM spawned are deleted in cleanup; one a bot killed is gone already. A `//spawn`
  never respawns.
- The GM's commands land in the server's GM audit log.
- `BOT_E2E_RECORD` fixtures, when set; the panel's temporary data directory is removed.

## Scenarios

| Scenario | Needs | Covers |
| --- | --- | --- |
| `TestLiveLoginEnterLogout` | login | #4 login-enter-logout, #7 recorded frames decode, #8 world model |
| `TestLiveLogoutRefusedInCombat` | GM | #4 refused logout, #12 AccountInUse after a dropped socket |
| `TestLiveWrongPassword` | `BOT_E2E_ALLOW_LOGINFAIL=1` | #4 wrong password, the breaker |
| `TestLiveCharacterClasses` | login | #10 races and classes, CharCreateFail 2, relogin by name |
| `TestLivePinning` | login | #92 known name, adoption, the refusals |
| `TestLiveReconnectAfterKick` | GM | #12 reconnect after `//kick` |
| `TestLiveRecallReappear` | GM | #11 `//recall` reappear |
| `TestLiveKilledBotRevivesInVillage` | GM | #11 revive in the village |
| `TestLiveSoak` | login | #9 soak past the idle timeout, #14 flat metrics |
| `TestLivePanelBots` | datapack | #94, #96, #89 panel create, despawn, respawn, restart |
| `TestLivePartyGMInviteLeave` | GM | #17 GM invite, GM leads; #19 whispered `leave` |
| `TestLivePartySwarmOneJoins` | login | #72 a player invites two bots of one swarm, one joins |
| `TestLiveWhisperCommands` | GM | #19 `invite me`, `follow`; a non-commander's `follow` ignored |
| `TestLiveWhisperLookup` | GM | #19 commands through the access level check (a counting stand-in for the database): a non-GM's three `invite me` checked once and never answered, the GM's checked once and answered |
| `TestLiveFollow` | GM | #18 follow from out of sight, settle, walk and run; #148 the GM leaves the bot's sight by teleport |
| `TestLiveFollowAcrossBand` | GM | #141 follow out of sight across Talking Island's band of refused cells (acis_golang#5412), on the bot's own route (#159) |
| `TestLiveFollowLedge` | GM | #147 follow out of sight over the ledges past that band, on the bot's own route |
| `TestLiveSupport` | GM | #22 buffs, heal under 60 %, assist after the leader's hit, renewal |
| `TestLiveRecoverVillageRestart` | GM | #24 village restart after `ResurrectWait`, back to the GM |
| `TestLiveRecoverLeaderLogout` | GM | #24 leader logout: `LeaderWait`, `/unstuck`, town |
| `TestLiveFarmFight` | GM, datapack | #20 melee and Power Strike kills of a GM-spawned monster, #23 potion and auto soulshots, #29 loot pickup and junk drop |
| `TestLiveSpots` | GM, datapack | #27 two farmers claim fitting spots within the cap, #38 an outleveled farmer leaves its spot for good |
| `TestLiveFarmStarters` | GM, datapack | #133 a level 1 elf, dark elf and orc farmer each claim a spot of their own newbie zone and kill a monster there |
| `TestLiveMerchantBuySell` | GM, datapack | #31 a merchant visit buys potions and sells junk: potion, adena and stem counts from the bot's inventory packets |
| `TestLiveTownErrands` | GM, datapack | #33 the town persona's errand cycle, a merchant visit then a gatekeeper hop; #32 the hop's teleport, the bot seen by the GM at the destination |
| `TestLiveNewbieGuideBuffs` | GM, datapack | #32 a level 8 bot asks the Newbie Guide and gets its buffs |
| `TestLiveTradeFromBot` | GM | #34, #82 a second client buys a bot's stems by trade; short of the price the bot cancels, never confirms |
| `TestLiveStoreSale` | GM, datapack | #35 a second client buys out a bot's private store, which closes sold out |
| `TestLiveStoreClosesOnFollow` | GM, datapack | #77 a GM's `follow` closes the bot's open store, and the bot follows |
| `TestLiveMerchantSwarm` | GM, datapack | #111 a merchant-preset bot with loot in town opens its store, seen by the GM |
| `TestLiveChatReplies` | GM | #36 one whispered reply after the delay; none to a repeat sender, a marked name, shout or trade |
| `TestLiveTrainer` | GM, datapack | #21 a level 10 mystic visits its class trainer (the spellbook when asked), #39 Heal's level offered from level 14 learned after the level-up to 15 |
| `TestLiveMysticStart` | GM, datapack | #134 a low level human mystic at its start point (a level up after the recall starts its errand there, #241), whose legs and /unstuck recall the server keeps there (acis_golang#5412), gives its trainer walk up within minutes and stays off it |
| `TestLiveGearWear` | GM, datapack | #79 shield, necklace and two rings worn, a better ring for the lesser, a two-handed sword worn and the shield taken off |
| `TestLiveGearBuy` | GM, datapack | #40 a better piece bought and worn, the old one sold; #79 a rogue's table buys and wears a dagger |
| `TestLiveArrows` | GM, datapack | #85, #102 an archer's table wielding a sword buys a bow and its arrows, wears the bow, and shoots without NotEnoughArrows |
| `TestLiveShotsByGrade` | GM, datapack | #169 a level 20 fighter with a no-grade Falchion, its shots and a D Saber but no D shots keeps the Falchion, no-grade shots on and spent; given D shots past its minimum it wears the Saber, D shots on and spent, no-grade ones off |
| `TestLiveTownPrep` | GM, datapack | #170, #182 a farmer logged in next to the grocer with adena buys shots, Lesser Healing Potions and two Scrolls of Escape before it claims a spot, under 45 % load, and keeps the scrolls |
| `TestLiveSupplyTrip` | GM, datapack | #175, #182, #251, #258 a level 6 farmer whose shots run out within ten minutes reads a Scroll of Escape at its starter spot (a walk or a death home fails), lands in the village, restocks its shots and scrolls before it leaves, and farms a spot again |
| `TestLiveBuffPotion` | GM, datapack | #176 a fighter drinks a Potion of Alacrity at its spot; one hasted (Haste given by the GM, of the potion's stack type and order) drinks none |
| `TestLiveStoreBuy` | GM, datapack | #177 a bot buys the shots it wants from a second client's sell store at the listed price, not a Scroll of Escape listed over its price cap |
| `TestLiveClassChangeDyes` | GM, datapack, class master | #178 a new fighter becomes a Warrior at the class master at 20 and a Gladiator at 40; #179 it buys and draws its two Warrior symbols in Gludin, then removes them and draws its three Gladiator symbols, each draw answered by its HennaInfo, dye InventoryUpdate and SymbolAdded before the next; fails if the master (shown as 31228 "Hasha", #343) is not at its Talking Island spawn |
| `TestLiveNavPickUpObstacle` | GM, datapack | #239 a level 5 farmer kills wolves the GM spawns and paralyses (a spawned monster wanders) at a measured site, recalled to its spot before each one, until it turns to a drop where the straight walk from where it stands fails: its kill spot for the first drop, the drop before for each next one (no site closes every drop spot: a scatter step blocked at once leaves the item on the corpse); it walks to a `StandPickUp` stand point, picks the item up from there, and the item reaches its inventory |
| `TestLiveValuablesTradeSale` | GM, datapack | #402 a merchant-preset mystic given a Sword of Watershadow says a Trade line the unmarked buyer hears; the buyer's offer gets a counter, the counter an agreement and the merchant's TradeRequest, which the buyer accepts and pays: settled at that price (both inventories); private links at Rolfe only, the sword never worn |
| `TestLiveValuablesWarehouse` | GM, datapack | #402 with `RoundGap` lowered, the merchant (its cap off, so what earlier runs left goes in too) deposits the unsold sword at Rolfe after its round (30 adena a row), the GM gives it Tears of Eva, and its next round withdraws the sword at Rolfe before one Trade line naming both; no `_DepositP`/`_WithdrawP` names Rant |
| `TestLiveValuablesRelease` | GM, datapack | #402 with `HoldRows` 1, a round with both valuables releases them: no deposit, the store lists only released valuables at their floor, and the buyer buys them there at the listed prices (what earlier runs left listed too) |

CharCreateFail 2 is exercised on the run that creates `e2ec6`'s character; later runs adopt it.

### Refusal checks

A wait polls a condition until its deadline, and on its own never looks at what the server
answered. A step that sends one request the server may refuse (a buy, an item use, a learn, a
teleport) waits with `untilServed` and a `RefusalCheck` over the bot's `Watch` instead of
`until`: the check names the step's packet (opcode and target) and the answers that refuse it
(the SystemMessage ids the server's handler refuses it with, and ActionFailed when the handler
answers a refusal with it), and the scenario fails as soon as such an answer is read within 3 s
after the packet, with the refusal and the last packets and events around it, instead of waiting
out its deadline (#248). The check is opt-in per step: count ActionFailed only where the server
answers no successful step with it and the scenario tolerates no failed try, since a dialog
click, a move or an interrupted cast also end with one. `TestLiveStoreBuy`, `TestLiveTownPrep`
and `TestLiveSupplyTrip` check their buys and the scroll read.

### Step deadlines

Every waiting step of a live scenario has a deadline of its own, short of the scenario's: a step
that is overdue fails at once, naming the step, how long it waited, what it still misses and the
last packets it saw, instead of waiting out the scenario's context (#366: a support bot that never
buffed its leader waited out 12 minutes). The wait helpers (`until`, `untilServed`, `waitFor`,
`watchUntil`) all run through `runStep` (`internal/e2e/helpers_refusal_live_test.go`, over
`awaitStep` in `internal/e2e/step.go`) and give a step `stepBound`, seven minutes (the five a bot holds its next merchant visit off after one,
`shopRetry`, plus the walk and the visit), unless it names its
own deadline through the `...By` form of the helper (`untilBy`, `untilServedBy`, `waitForBy`,
`watchUntilBy`). A deadline is a time, so a step may count it from an event of the run (the support
bot's renewal is due a minute past four minutes after its first Might on the leader) rather than
from when it starts waiting. A step that takes stepNote (10 s) or more logs how long it took and how
far its deadline was, so a run's log shows each slow step's margin.

A step that may take longer than `stepBound` names its deadline with the reason at its call site:
the farm fight (`TestLiveFarmFight`, 25 minutes: a passing run took 19m50s), the trip home of
`TestLiveSupplyTrip` (15 minutes: the shots run out after about ten), the visits and the delivery
of `TestLiveClanMember` (the walk to town and the supply-trip gap), the gear purchases of
`TestLiveGearBuy` and `TestLiveArrows`, the walks of `TestLiveSpotSpread`, `TestLiveMysticStart`,
`TestLiveTrainer` and the follow and nav scenarios. `TestLiveSupport` bounds each of its steps:
the leader's buffs are overdue two minutes after the invitation. Observation windows
(`TestLivePackSpot`, `TestLivePackRisk`, `TestLiveSoak`) wait out their window on purpose, and a
scenario whose own deadline is within `stepBound` needs no step deadline. A raw `Watch.Await` on the
scenario's context gets a sub-context of its own (`awaitDeath`, the login `UserInfo` of `runWatched`,
the guide's buff of `TestLiveNewbieGuideBuffs`, the restart teleport of
`TestLiveRecoverVillageRestart`), and each leg of `TestLiveNavFacts` is overdue once its walk at speed
plus `EntryTimeout` has passed.

No scenario covers #28, a monster another player fights skipped: #138 dropped that rule, and a
farmer may take such a monster.
