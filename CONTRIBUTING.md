# Contributing

This page is the short version for people. [`CLAUDE.md`](CLAUDE.md) is the full rule set; it
applies to every contributor, human or coding agent, and wins where the two differ.

## Set up

- Go, the version in `go.mod`.
- [golangci-lint](https://golangci-lint.run/) v2: the formatter (gofumpt) and linters in
  `.golangci.yml`.
- Node.js 22 and npm (`web/.nvmrc`), only for the control panel's page in `web/`.
- A checkout of [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang), the server, as
  a read-only reference for protocol and behavior.

```bash
make hooks   # once per clone: formats and lints staged Go files on commit
```

## Workflow

1. **Start from an issue** in [fatal10110/acis_golang_bot](https://github.com/fatal10110/acis_golang_bot/issues).
   Read its body, comments and milestone epic, and its row in the design document it comes from
   ([`docs/README.md`](docs/README.md) maps milestones to documents).
2. **Check the server.** Packet layouts, field order, opcodes, send order, and what the server
   accepts, rejects, rate-limits and bans come from the server's code, not from client lore or
   memory. Wire code the bot needs is copied into this repository; the bot never imports the
   server.
3. **Branch** off `main` (a linked worktree per task is the convention), change only what the
   issue needs, and add tests.
4. **Run the gates** below.
5. **Open a pull request** against `main` with a closing keyword (`Closes #N`), what you ran as
   evidence, and a gap audit: `Gap audit: no shipped gaps`, or each gap mapped to the issue that
   tracks it.

### Gaps

If a feature needs server behavior that does not exist yet, that is a server issue: file it in
`fatal10110/acis_golang` and defer the bot feature. Never work around a missing server behavior in
the bot, and never special-case the server for bots. Anything else you leave undone gets a
follow-up issue in the owning milestone before the PR is opened.

## Gates

```bash
make lint        # golangci-lint fmt --diff and golangci-lint run: must print nothing
make test        # unit tier
go vet ./...
make ui-check    # only when web/ changed
```

CI runs build, vet, `go test -race ./...`, lint and the web checks on every pull request. Run
`make test-race` locally only when your change touches concurrency.

The live e2e scenarios (`make e2e-live`) run real bots against the real server from the owner's
machine and never in CI; see [`docs/e2e-live.md`](docs/e2e-live.md). Never run them with `-race`.

## Rules that block a merge

- **Protocol fidelity.** Every packet you touch matches the server byte for byte; decoders consume
  exactly `len(payload)`.
- **No races.** Every shared mutable value has one owner: a goroutine or a named lock. Every
  goroutine has a shutdown path; a session's reader is joined before the next dial.
- **Operational safety.** Anything a bot sends respects the server's limits (frame rate, bypass,
  chat, emote, drop and craft gates, login rate per IP). A new sender states which limit bounds
  it. Nothing may risk the host's IP being banned.
- **Secrets** (account passwords, the HMAC secret, database credentials) never appear in code,
  flags, logs, issues, PRs or test fixtures.
- **Idiomatic Go.** Concrete types; an interface only for a second implementation or a real test
  seam, defined at the consumer. Nothing speculative.

Details: [`docs/agents/go-style.md`](docs/agents/go-style.md).

## Tests

- Table-driven unit tests: codec round trips and recorded fixtures, crypt against fixed vectors,
  brain decisions as events and an injected clock in, packets out.
- Never wait on the wall clock in a unit test, and never hide a test behind a build tag.
- `bots swarm -record DIR` captures the first frame of every game opcode as fixtures.
- A change to `internal/geo` needs the geodata parity check: `bots geoquery` plus the server's
  `geoprobe` at `agreement=100.00%` ([`docs/cli.md`](docs/cli.md#bots-geoquery)).

Details: [`docs/agents/test-strategy.md`](docs/agents/test-strategy.md) and
[`docs/agents/verification.md`](docs/agents/verification.md).

## Documentation

- A new or changed flag: update [`docs/cli.md`](docs/cli.md).
- A new package, package boundary or goroutine owner: update
  [`docs/architecture.md`](docs/architecture.md) and the package's `// Package` comment.
- A deployment step: update [`docs/home.md`](docs/home.md).
- A new feature area is designed in its own document first, with the server behavior it rests on,
  the owner decisions, the delivery table and the known gaps.
