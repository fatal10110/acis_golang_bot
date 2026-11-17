# Test strategy

The bot has two test tiers: fast, deterministic unit tests that run everywhere, and e2e scenarios
that drive real bots against a real server stack. The plan's "Test strategy (bot repository)"
section is the source; this guide turns it into rules.

## Tier 1: unit tests (every PR, CI)

Table-driven, no shared fixtures, no network, no wall clock, no build tags.

- **Codecs.** Round trips for every encoder and decoder, plus recorded fixtures: payloads captured
  by the `-record <dir>` tap during e2e runs (the first payload per opcode, after decryption). A
  fixture test asserts the decoder consumes exactly `len(payload)` and that the fields are sane;
  ids, coordinates and times vary between recordings, so do not pin them.
- **Crypt.** Fixed vectors. Where the server-side half is the natural oracle (login key pair,
  login crypt), copy it as a test helper or freeze its output as vectors; never import the server.
- **Brain.** Events and an injected clock in, packets out: build the world model from events,
  advance the clock, assert the packets the bot sends and the state it ends in. One table row per
  rule of the decision list.
- **Process-wide registries and limiters** (spot registry, store registry, login rate limiter,
  credential breaker): their contracts under concurrency, with `-race`, driving the concurrent
  callers directly.
- **Fuzz targets** for decoders of server frames: a malformed frame must end one session with an
  error, never panic or allocate without bound.

## Tier 2: e2e scenarios (local and nightly, not gated by PR CI)

Real bots against a real server: the live server (`docs/e2e-live.md`, #107) today, the published
server stack (server image, non-interactive registration, config bundle, geodata subset: the
plan's server prerequisites P1-P4, #5) once it exists, both through the same harness.

- **Harness.** Package `internal/e2e`: the target (`e2e.Live`), its safety envelope (the bot's
  login gate and credential breaker, at most 4 e2e characters online, the GM command filter, the
  bot's pacer) and the scenarios, its `*_live_test.go` files; M6 panel scenarios live in
  `cmd/bots/ui_live_test.go`. No build tag hides them.
- **Target.** Environment variables that point at files only, never a secret: `BOT_E2E_LOGIN`,
  `BOT_E2E_SECRET_FILE`, `BOT_E2E_GM_FILE` (both default to the primary checkout's gitignored
  `.e2e/`), `BOT_E2E_PREFIX` (default `e2e`; refused when it overlaps `hz` or the owner's panel
  prefix), `BOT_E2E_ALLOWLISTED`, `BOT_E2E_DATAPACK`, `BOT_E2E_ALLOW_LOGINFAIL`.
- **Server config:** `AutoCreateAccounts = True`, `MaximumOnlineUsers` headroom for 4 more
  characters, and an existing GM character (access level 7).
- **Seeding** goes through GM commands only, never the database (no DSN, tunnel or SQL):
  `//teleport`, `//teleportto`, `//recall`, `//kick`, `//kill`, `//spawn` and `//delete` on the
  run's own characters and monsters (`e2e.Scope`). **Assertions** use the packets the bot or the
  GM client receives: CharSelectInfo slot count, UserInfo, Die, Revive, TeleportToLocation,
  CharInfo, AutoAttackStart.
- **Time** is real, with a deadline per scenario. Scenarios spawn their own monsters (a `//spawn`
  never respawns) and delete them in cleanup.
- **Gating.** A scenario skips when its config is missing (each declares what it needs: login,
  GM, datapack), and fails instead of skipping when `BOT_REQUIRE_E2E=1` is set; agents set it
  whenever they claim a scenario ran. Run them with `make e2e-live` (`-p 1`: one login gate at a
  time); the wrong-password scenario trips the breaker and runs alone (`make e2e-live-loginfail`).
- One shared stack per machine; never bring up a second one next to it.
- The nightly run against the server's `main` files a protocol-drift issue on failure.

## Fakes vs. real types

Replace a fake with the real type only when both hold:

1. A real, already-existing type satisfies the same method set the fake stands in for (check with
   `gopls` or `rg` before converting).
2. Building the real type is not disproportionate to the behavior under test.

Keep a double when it is a sanctioned infrastructure seam where determinism is the point (clock,
timer, RNG source, a scripted connection that replays recorded frames), or when it serves a pure
codec or crypt test. When a fake duplicates real logic, delete the fake and call the real type: a
fake that reimplements a rule drifts silently.

## Checklist before writing any test

1. A codec, crypt primitive, or pure function? Tier 1, table-driven, with fixtures or vectors.
2. A brain rule or persona behavior? Tier 1 with events and the injected clock.
3. Something only a real server can show (login, visibility, party, trade, store, death, teleport)?
   An e2e scenario, plus tier-1 coverage of the bot's own decision logic around it.
