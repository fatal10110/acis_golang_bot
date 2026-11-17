# Live e2e runbook

The e2e scenarios (`internal/e2e`, `cmd/bots/ui_live_test.go`) run real bots against the live
server from the owner's machine (#107). They never run in CI, never touch the server's database,
and never edit the server's config: a scenario the server cannot satisfy is recorded on the issue.

## Owner prerequisites

- **Login server:** `AutoCreateAccounts = True` (the first login of an `e2e` account creates it),
  `MaximumOnlineUsers` headroom for 4 more characters.
- **GM character:** an existing account whose character has access level 7, created over the
  protocol and promoted once by the owner. The harness never promotes anything.
- **Config files** in `.e2e/` at the root of the primary checkout (gitignored; dir 0700, files
  0600, refused when readable by others):
  - `secret`: the HMAC secret the `e2e` account passwords derive from (strong, as persistent mode
    wants it);
  - `gm`: three `key=value` lines, `account=`, `password=` and `character=`.
- **Allowlist:** set `BOT_E2E_ALLOWLISTED=1` only when the source address is on
  `AcceptFilterAllowlist` of both servers; otherwise logins are paced by the bot's login gate.

## Configuration

Environment variables only, each a value or a path, never a secret:

| Variable | Meaning | Default |
| --- | --- | --- |
| `BOT_E2E_LOGIN` | login server `host:port` | none: every scenario skips |
| `BOT_E2E_SECRET_FILE` | secret file | `.e2e/secret` of the primary checkout |
| `BOT_E2E_GM_FILE` | GM file | `.e2e/gm` of the primary checkout |
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
variables and `BOT_REQUIRE_E2E=1`. Packages run one after the other (`-p 1`): each test process
has its own login gate.

## Safety envelope

Operational limits, not persona defaults; unit-tested with a fake target (`internal/e2e`):

- **Login pacing and the credential breaker:** every login of a test process passes one
  `botclient.LoginGate`, spaced under the accept filter unless allowlisted. The first refused
  credential halts every later login of the process. The wrong password is sent at most once a
  run, and only with `BOT_E2E_ALLOW_LOGINFAIL=1`.
- **Cap:** at most 4 characters of a run online at once, the GM's and a panel's bots included;
  every scenario logs its characters out in cleanup.
- **GM commands** (`e2e.Scope`) touch only the run's characters and the monsters it spawned:
  `teleport X Y Z`, `teleportto|recall|kick NAME`, `kill|res|heal [NAME]`, `set level N` and
  `skill set` on a targeted run character, `spawn ID`, `delete` on a spawned monster. The forms
  that reach others (`kick all`, `recall party|clan`, a radius) and every server-wide command
  (announce, shutdown, reload) are refused before anything is sent. The GM targets only run
  objects, so a command that falls back on its target cannot reach anyone else.
- **Pacing:** every frame a scenario sends passes the bot's pacer (`botclient.Pacer`); GM
  commands are also spaced like dialog clicks.

## What a run creates and leaves behind

- Accounts `<prefix><group><n>` (`e2ea1`, `e2ec1`-`e2ec6`, `e2er1`, `e2ew1`-`e2ew2`, `e2es1`) and
  the panel's `<prefix>p1`, `<prefix>p2`, each with one character whose name carries the bot
  marker. They stay on the server; later runs adopt the same characters.
- Characters end where the scenario left them (a recalled bot next to the GM, a killed one in a
  village); nothing is restored.
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

CharCreateFail 2 is exercised on the run that creates `e2ec6`'s character; later runs adopt it.
