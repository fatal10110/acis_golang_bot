# Contributing

Thanks for your interest in improving the bots. This guide covers what you need to know to get a
change merged. The complete rule set is in [`CLAUDE.md`](CLAUDE.md). It applies to people and to
coding agents alike, and it wins wherever the two documents differ.

## Contents

- [Before you start](#before-you-start)
- [Setting up](#setting-up)
- [Making a change](#making-a-change)
- [Checks to run](#checks-to-run)
- [What reviewers look for](#what-reviewers-look-for)
- [Writing tests](#writing-tests)
- [Updating the documentation](#updating-the-documentation)

## Before you start

- **Read the background.** [`README.md`](README.md) explains what the bots do,
  [`docs/architecture.md`](docs/architecture.md) how the code is organized, and
  [`docs/decisions.md`](docs/decisions.md) which decisions are settled and why.
- **Work from an issue.** Every change starts from an issue in
  [fatal10110/acis_golang_bot](https://github.com/fatal10110/acis_golang_bot/issues). If none
  exists for what you want to do, open one first so the approach can be agreed on.
- **Product decisions belong to the owner.** If your change would alter a numbered decision
  (D1-D49), raise it on the issue instead of changing it in code.

## Setting up

You need:

- **Go**, the version named in `go.mod`;
- **[golangci-lint](https://golangci-lint.run/) v2**, built with that Go version or newer, for
  formatting (gofumpt) and linting with the repository's `.golangci.yml`;
- **Node.js 22 and npm** (`web/.nvmrc`), only if you change the control panel's page in `web/`;
- **a checkout of the server**, [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang),
  to look up how the server behaves. You read it; you never change it from this project.

Then enable the pre-commit hook once per clone. It formats and lints the Go files you stage:

```bash
make hooks
```

## Making a change

1. **Understand the issue.** Read its description and comments, its milestone, and the section of
   the design document it comes from ([`docs/README.md`](docs/README.md#design-documents) lists
   them).
2. **Check the server's code.** Packet layouts, field order, opcodes, the order packets are sent
   in, and what the server accepts, rejects, rate-limits or bans must come from the server's code.
   Do not rely on memory of other servers or client lore. If the bot needs server code, copy it
   into this repository: the bot never imports the server.
3. **Create a branch** from `main`, and change only what the issue needs.
4. **Add or update tests** (see [Writing tests](#writing-tests)).
5. **Run the checks** below.
6. **Open a pull request** against `main`. In the description:
   - link the issue with a closing keyword, for example `Closes #123`;
   - list the checks you ran and their results;
   - end with a gap audit: either `Gap audit: no shipped gaps`, or each thing left undone with the
     issue that now tracks it.

### When the server is missing something

Sometimes a feature needs server behavior that does not exist yet. That is a server issue, not a
bot problem: open an issue in
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang/issues) and leave the bot feature
deferred. Never work around missing server behavior in the bot, and never ask the server to treat
bots specially.

Anything else you leave out of scope gets a follow-up issue in the right milestone before the pull
request is opened.

## Checks to run

```bash
make lint        # must print nothing
make test        # unit tests
go vet ./...
make ui-check    # only if you changed web/
```

Run `make test-race` (the unit tests with the race detector) when your change touches
concurrency. CI runs it on every pull request anyway, together with the build, `go vet`, the
linters and the web page's checks.

The live scenarios (`make e2e-live`) run real bots against a real server. They are run by hand from
the owner's machine and never in CI; see [`docs/e2e-live.md`](docs/e2e-live.md). Do not run them
with the race detector.

## What reviewers look for

These rules block a merge:

- **The protocol matches the server.** Every packet you touch matches the server byte for byte,
  and each decoder uses exactly the bytes it is given.
- **No data races.** Every piece of shared state has one owner: a single goroutine or a named lock.
  Every goroutine you start has a way to stop, and a session's reader is stopped before the bot
  reconnects.
- **The server stays safe.** Anything a bot sends stays within the server's limits: packets per
  second, dialog clicks, chat, emotes, item drops, crafting, logins per IP. If you add code that
  sends packets, say which limit bounds it. Nothing may risk getting the host's IP banned.
- **Secrets stay secret.** Account passwords, the password secret and database credentials never
  appear in code, flags, logs, issues, pull requests or test data.
- **Plain, idiomatic Go.** Use concrete types. Add an interface only when a second implementation
  or a real test needs it, and define it where it is used. Do not build for needs that do not
  exist yet.

More detail: [`docs/agents/go-style.md`](docs/agents/go-style.md).

## Writing tests

- **Table-driven unit tests.** Packet round trips and recorded real packets; encryption against
  fixed test vectors; brain decisions, where events and a fake clock go in and packets come out.
- **No waiting on real time** in unit tests: pass a clock in instead.
- **No build tags** to hide tests.
- **Recording packets.** `bots swarm -record DIR` saves the first frame of every kind of packet the
  server sends, for use as test data.
- **Map code changes** need a parity check: `bots geoquery` plus the server's `geoprobe` must agree
  on 100% of sampled points ([`docs/cli.md`](docs/cli.md#bots-geoquery)).

More detail: [`docs/agents/test-strategy.md`](docs/agents/test-strategy.md) and
[`docs/agents/verification.md`](docs/agents/verification.md).

## Updating the documentation

| If you change... | Update |
| --- | --- |
| a flag or a command | [`docs/cli.md`](docs/cli.md) |
| a package, a package boundary or which goroutine owns what | [`docs/architecture.md`](docs/architecture.md) and the package's `// Package` comment |
| a setup or deployment step | [`docs/home.md`](docs/home.md) |
| a decision (approved by the owner) | the design document, [`docs/plan.md`](docs/plan.md) and [`docs/decisions.md`](docs/decisions.md) |

A new feature area is designed in its own document before the code is written. It describes the
server behavior it depends on, the decisions it needs, the issues that will deliver it, and the
known gaps.
