# Live e2e runbook

The e2e scenarios (`internal/e2e`, `cmd/bots/ui_live_test.go`) run real bots against the live
server from the owner's machine (#107). They never run in CI, never touch the server's database,
and never edit the server's config: a scenario the server cannot satisfy is recorded on the issue.

## Owner prerequisites

- **Login server:** `AutoCreateAccounts = True` (the first login of an `e2e` account creates it),
  `MaximumOnlineUsers` headroom for 8 more characters per test process (2 scenarios of 4).
- **GM characters:** existing accounts whose characters have access level 7, created over the
  protocol and promoted once by the owner, one per scenario running at once. The harness never
  promotes anything.
- **Config files** in `.e2e/` at the root of the primary checkout (gitignored; dir 0700, files
  0600, refused when readable by others):
  - `secret`: the HMAC secret the `e2e` account passwords derive from (strong, as persistent mode
    wants it);
  - `gm`, `gm2`, `gm3`, ...: the GM pool, one GM file each, three `key=value` lines, `account=`,
    `password=` and `character=`. The harness keeps a lock file next to each (`gm.lock`, ...).
- **Allowlist:** set `BOT_E2E_ALLOWLISTED=1` only when the source address is on
  `AcceptFilterAllowlist` of both servers; otherwise logins are paced by the bot's login gate.

## Configuration

Environment variables only, each a value or a path, never a secret:

| Variable | Meaning | Default |
| --- | --- | --- |
| `BOT_E2E_LOGIN` | login server `host:port` | none: every scenario skips |
| `BOT_E2E_SECRET_FILE` | secret file | `.e2e/secret` of the primary checkout |
| `BOT_E2E_GM_FILE` | GM file, or a list of them (`:` between, `;` on Windows): pins the run's GM pool | every `gm`, `gm2`, ... of the primary checkout's `.e2e/` |
| `BOT_E2E_PREFIX` | account prefix; refused when it overlaps `hz` or the owner's `bots ui` panel prefix | `e2e` |
| `BOT_E2E_ALLOWLISTED` | `1`: no login spacing | unset |
| `BOT_E2E_DATAPACK` | local datapack XML directory (the panel's `town` preset) | none |
| `BOT_E2E_ALLOW_LOGINFAIL` | `1`: run the wrong-password scenario | unset |
| `BOT_E2E_SOAK` | soak length | `16m` (past the 15 min idle timeout) |
| `BOT_E2E_RECORD` | directory the login scenario records fixtures into | a temporary one |
| `BOT_REQUIRE_E2E` | `1`: a scenario whose config is missing fails instead of skipping | unset |

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
  at a time: the farmers on the newbie spots (farm, anti-KS, spots, arrows), and the bots sent to
  the village's grocer, stores, trainers and merchants.
- **GM pool:** each scenario that needs a GM leases one from the pool (`.e2e/gm*`, or the
  `BOT_E2E_GM_FILE` list) through a lock on its lock file, held until the GM logs out, so two
  scenarios of one process, of two processes or of two runs never share a GM. With every GM
  held, the scenario waits. The kernel drops the lock of a process that dies, so a killed run
  never blocks a later one.
- **Packages:** `./internal/e2e/` and `./cmd/bots/` are separate test processes, each with its own
  login gate. They run at the same time only with `BOT_E2E_ALLOWLISTED=1`: two spaced gates
  dialing at once would burst the accept filter.

### Parallel runs

Live runs may run at the same time, as real bot runs do (several agents, or an agent and the
owner's own bots). Each concurrent run needs:

- **GMs:** the second login of an account drops the first, so two runs never share a GM session.
  Runs of this harness lease GMs from the pool through the per-GM locks, so they may share the
  pool; a run may still pin its own GM files (`BOT_E2E_GM_FILE=.e2e/gm2`, or a list) to keep out
  of the GMs another run uses, for example a run of an older harness that takes no lock.
- **Its own lock** when the run goes through `scripts/agent-tools/heavy`: a `HEAVY_LOCK` of its
  own (`HEAVY_LOCK=/tmp/acis-bot-live-gm2.lock`, with `HEAVY_TIMEOUT=60m`). A live run never takes
  the default heavy lock, which stays for race tests and lint on the owner's machine.
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
  process opens the run and again before each scenario, so a test process running beside the one
  that tripped stops at its next scenario: the login server
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
  character, `hide` (the GM's own invisibility), `spawn ID`, `delete` on a spawned monster. The forms
  that reach others (`kick all`, `recall party|clan`, a radius) and every server-wide command
  (announce, shutdown, reload) are refused before anything is sent. The GM targets only run
  objects, so a command that falls back on its target cannot reach anyone else.
- **Pacing:** every frame a scenario sends passes the bot's pacer (`botclient.Pacer`); GM
  commands are also spaced like dialog clicks.

## What a run creates and leaves behind

- Accounts `<prefix><group><n>` (`e2ea1`, `e2ec1`-`e2ec6`, `e2er1`, `e2ew1`-`e2ew2`, `e2es1`,
  `e2ept1`, `e2esw1`-`e2esw3`, `e2ewh1`-`e2ewh2`, `e2efo1`, `e2esu1`-`e2esu2`, `e2erv1`-`e2erv3`,
  `e2ef1`, `e2ek1`, `e2en1`-`e2en2`, `e2em1`, `e2etw1`, `e2eg1`, `e2etd1`-`e2etd2`, `e2est1`-`e2est3`,
  `e2ems1`, `e2ech1`-`e2ech2`, `e2et1`, `e2egw1`, `e2egb1`-`e2egb2`, `e2ear1`) and
  the panel's `<prefix>p1`, `<prefix>p2`, each with one character whose name carries the bot
  marker. They stay on the server; later runs adopt the same characters.
- Characters end where the scenario left them (a recalled bot next to the GM, a killed one in a
  village); nothing is restored but the level below.
- `TestLiveFarmFight` sets its farmer to level 5 and runs it with every item but adena, potions
  and shots marked as junk: each run the farmer drops all its unequipped loot at its spot, where it
  lies until the server clears it, so the farmer's inventory never fills up across runs. The GM then
  gives it Power Strike, 10 Healing Potions, 500 no-grade soulshots and 3 Stems (a gift waits while
  the inventory is full); what it did not use stays with it. Other farmers keep their loot and the
  levels they gained; `TestLiveSpots` sets its first farmer back to level 1 at the end.
- `TestLiveAntiKS` makes the GM visible (`//hide` toggled off, it logs in invisible and no monster
  fights an invisible player) and lets the GM land the first hit on a spawned wolf, which then
  fights the GM: the datapack's monsters of the Warrior AI attack nobody on sight. A farmer `e2ek1`
  found inside the Talking Island village is recalled to the newbie spots first.
- The M4 scenarios seed with `give` only: adena for purchases, trades and the town bot's gatekeeper
  hop (Gludin, 18000), Stems (1864) for the merchant visit's sale and the trade, and Steel Powder
  (1664), loot no NPC shop sells, for the stores. The
  merchant bot `e2em1` spends all its adena on Lesser Healing Potions each run, the town bot
  `e2etw1` all but 19000, and both keep what they bought; the town bot ends in another town, the
  Newbie Guide's bot `e2eg1` at level 8 with the newbie buffs. The buyers keep what they
  bought and the adena they did not spend. `TestLiveChatReplies` has the GM say one line on the
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
  `TestLiveArrows` ends with the GM visible at the Talking Island newbie spots.
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
| `TestLiveFollow` | GM | #18 follow from out of sight, settle, walk and run |
| `TestLiveSupport` | GM | #22 buffs, heal under 60 %, assist after the leader's hit, renewal |
| `TestLiveRecoverVillageRestart` | GM | #24 village restart after `ResurrectWait`, back to the GM |
| `TestLiveRecoverLeaderLogout` | GM | #24 leader logout: `LeaderWait`, `/unstuck`, town |
| `TestLiveFarmFight` | GM, datapack | #20 melee and Power Strike kills of a GM-spawned monster, #23 potion and auto soulshots, #29 loot pickup and junk drop |
| `TestLiveAntiKS` | GM, datapack | #28 a monster fighting the GM is left alone past `EngagedWindow`, then taken on once the GM is gone |
| `TestLiveSpots` | GM, datapack | #27 two farmers claim fitting spots within the cap, #38 an outleveled farmer leaves its spot for good |
| `TestLiveMerchantBuySell` | GM, datapack | #31 a merchant visit buys potions and sells junk: potion, adena and stem counts from the bot's inventory packets |
| `TestLiveTownErrands` | GM, datapack | #33 the town persona's errand cycle, a merchant visit then a gatekeeper hop; #32 the hop's teleport, the bot seen by the GM at the destination |
| `TestLiveNewbieGuideBuffs` | GM, datapack | #32 a level 8 bot asks the Newbie Guide and gets its buffs |
| `TestLiveTradeFromBot` | GM | #34, #82 a second client buys a bot's stems by trade; short of the price the bot cancels, never confirms |
| `TestLiveStoreSale` | GM, datapack | #35 a second client buys out a bot's private store, which closes sold out |
| `TestLiveStoreClosesOnFollow` | GM, datapack | #77 a GM's `follow` closes the bot's open store, and the bot follows |
| `TestLiveMerchantSwarm` | GM, datapack | #111 a merchant-preset bot with loot in town opens its store, seen by the GM |
| `TestLiveChatReplies` | GM | #36 one whispered reply after the delay; none to a repeat sender, a marked name, shout or trade |
| `TestLiveTrainer` | GM, datapack | #21 a level 10 mystic visits its class trainer (the spellbook when asked), #39 Heal's level offered from level 14 learned after the level-up to 15 |
| `TestLiveGearWear` | GM, datapack | #79 shield, necklace and two rings worn, a better ring for the lesser, a two-handed sword worn and the shield taken off |
| `TestLiveGearBuy` | GM, datapack | #40 a better piece bought and worn, the old one sold; #79 a rogue's table buys and wears a dagger |
| `TestLiveArrows` | GM, datapack | #85, #102 an archer's table wielding a sword buys a bow and its arrows, wears the bow, and shoots without NotEnoughArrows |

CharCreateFail 2 is exercised on the run that creates `e2ec6`'s character; later runs adopt it.
