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

Real bots against the published server stack (server image, non-interactive registration, config
bundle, geodata subset: the plan's server prerequisites P1-P4).

- Config for the stack: `AutoCreateAccounts = True`, `MaximumOnlineUsers` and `LoginTryBeforeBan`
  raised, the accept filter off or the Docker gateway allowlisted (host bots appear as the gateway
  IP), project-scoped container names and ports.
- **Seeding** stays minimal: create the GM character over the protocol, then one
  `UPDATE characters SET accesslevel = 7` while it is offline; everything else through admin
  commands the GM sends (`//set level`, `//skill set`, `//spawn`, `//kill`, `//heal`). Direct SQL
  couples the suite to the server schema: keep it to the statements the plan names.
- **Time** is real, with a timeout per scenario. Scenarios run next to a town, spawn fresh monsters
  per scenario (spawnlist respawn is 60 s ± 20 s), and use a warm geodata cache volume.
- **Gating.** An e2e test skips when the stack is not reachable, and fails instead of skipping when
  `BOT_REQUIRE_E2E=1` is set; agents set it whenever they claim a scenario ran. The exact package
  location and the variables that point at the stack are fixed by the e2e harness issue (#5); keep
  this section in sync with it.
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
