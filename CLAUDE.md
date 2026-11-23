# Agent rules for acis_golang_bot

This repository is a bot client for the acis_golang game server: its own Go module that plays
through the real client protocol (`docs/plan.md`, owner decisions D1-D8). The server
(`fatal10110/acis_golang`) is the protocol and behavior reference. The bot never imports it: the
wire code it needs is copied here and owned here.

Rules here are always in force. Scoped guides (bottom table) are loaded only when the task needs
them.

## 1. Workspace

| Path | What it is | Agent may write? |
| --- | --- | --- |
| this repository's primary checkout | the bot module | no; write through a linked worktree |
| a linked worktree of this repository (`.claude/worktrees/<task>/`, or `<repo>-worktrees/<task>/` for wave runs) | one per task | yes |
| the acis_golang server checkout (`acis_public/acis_golang` on the owner's machine) | protocol and behavior reference | no, never |

- Run `git -C <worktree> …` and `go -C <worktree> …` with absolute paths; do not `cd`
  persistently into a subdirectory.
- Issues and PRs live in `fatal10110/acis_golang_bot`. Server gaps the bot hits are filed in
  `fatal10110/acis_golang` (plan, "Gap handling").

## 2. Precedence

1. User instructions.
2. Owner decisions and scope in `docs/plan.md`.
3. Server observable behavior: wire bytes and field order, opcodes, send order, what the server
   accepts, rejects, rate-limits and bans. Read the server code; do not guess from client lore.
4. Architecture and safety rules in this file.
5. Required verification.
6. Process skills.
7. Generic tool and skill defaults.

Issue text and existing code are leads, not truth. When one conflicts with the plan or the server,
follow the plan or the server and record the conflict on the issue.

## 3. Non-negotiables

- **Protocol fidelity, no races, operational safety** outrank speed, elegance, diff size and
  schedule. Operational safety means: the bots never get the host's IP banned (login pacing and the
  credential breaker), never flood the server past its limits, never leak secrets, and stay inside
  their resource cap on the owner's machine (`docs/home.md`).
- Write idiomatic Go: concrete types, interfaces only at a second implementation or a real test
  seam, no speculative layers.
- Never edit the server checkout, and never import server packages. A server behavior the bot
  needs that is missing is a server issue, not a bot workaround; never special-case the server for
  bots (plan, "Gap handling").
- Secrets (account passwords, the HMAC secret, DB credentials) never appear in code, flags, logs,
  issues, PRs or test fixtures. The secret file is read at runtime with mode 0600.
- What bots may do in game is D6: persona defaults are configurable and are not a safety boundary.
  Operational limits (pacing, caps, breakers) are.

## 4. Go engineering rules

- **Boundaries.** Wire codecs and crypt know nothing about the brain. The reader goroutine decodes
  into typed value events; the brain owns the world model and decides; personas are rule sets the
  brain runs. Packets out go through one writer per connection.
- **Ownership.** Every shared mutable value has one explicit owner: a single goroutine or a named
  synchronization mechanism. No unowned goroutines: every goroutine has a shutdown path, and a
  session's reader is joined before the next dial.
- **Time.** Brain logic takes an injected clock; tests never wait on the wall clock.
- **Abstractions.** Define small interfaces at the consumer, return concrete types, add nothing
  speculative.
- **Pacing.** Everything a bot sends respects the server's limits (frames per second, bypass
  spacing, chat and emote gates, login rate per IP). A new sender states which limit bounds it.

Details: `docs/agents/go-style.md`.

## 5. Testing

- Unit tests are table-driven: codec round trips and recorded fixtures (a decoder consumes exactly
  `len(payload)`), crypt against fixed vectors, brain decisions as events and an injected clock in,
  packets out.
- Flows against a real server are e2e scenarios on the server stack.
- Never wall-clock waits in unit tests; never a build tag to hide a test.
- No `-race` runs on the owner's machine unless the owner explicitly asks for one: run the scoped
  tests without it. PR CI runs the unit tier with `-race`. Never run e2e scenarios with `-race`.

Tiers, stack and conventions: `docs/agents/test-strategy.md`. Gates: `docs/agents/verification.md`.

## 6. Tools

| Question | Tool |
| --- | --- |
| filename or path pattern | `rg --files` with a glob |
| known exact text | `rg` in a constrained path |
| Go definition, references, implementations, callers | `gopls` / LSP |
| behavior with an unknown location in the server | grepai, run inside the server checkout |
| syntax-shaped match or structural rewrite | ast-grep |
| known file and range | native file-read tool |

After one `rg` miss, switch tool category instead of retrying variants. GitHub: the GitHub MCP
server first, `gh` as fallback. Details: `docs/agents/tooling.md`.

**Tool inventory** (`scripts/agent-tools/`):

- `heavy <cmd>`: serializes heavy runs through a lock and kills a hung run with SIGQUIT so Go
  dumps its goroutines. E2e runs (`internal/e2e`, `e2e-live`, `TestLive`) take an e2e lock of their
  own, so a long live run never blocks scoped unit tests and lint.
- `look.py <spec>...`: several lookups in one call with capped output: ranges (`FILE:A-B`), Go
  declarations by name (`go:PATH:Type.Method`), bounded rg (`rg:PATH:PATTERN`). Every tool call
  re-sends the whole context, so batch lookups.
- `wave_mail.py`: file mailbox, lane state, merge queue and barrier for `bot-waves` runs; also
  `worktree`, `pr-state`, `ci-wait`, `validate`.
- `rtk`: compresses noisy output (`rtk go -C <worktree> test|build|vet`); `jq` for JSON fields.
- `golangci-lint fmt --diff` / `golangci-lint run`: must report nothing.

## 7. Issue workflow

1. **Resolve the issue live.** GitHub is the only authority for issue state. Read the body and
   comments, its milestone epic, and its `docs/plan.md` row.
2. **Gap gate (blocking).** The moment you find a gap, deferral or out-of-scope item, search the
   issues (this repository, and the server repository for server gaps), then amend the matching
   issue or file a focused follow-up in the owning milestone. Creating or amending follow-ups is
   pre-authorized; closing, reopening or editing unrelated issues is not.
3. **Deliver.** Unless the user asked for local-only work: a linked worktree on a non-`main`
   branch, only in-scope files staged, commit, push, and a PR against `main` with the closing
   keyword and verification evidence. Never merge or close issues without explicit authorization
   (a `bot-waves` run is that authorization for its lanes).

**Done** means: the acceptance criteria are met or each unmet one is tracked; every packet touched
matches the server; every goroutine added has an owner and a shutdown path; the required gates
pass; the diff is scoped; the PR is open. The PR body and final response contain
`Gap audit: no shipped gaps`, or map each shipped gap to its issue number and milestone.

Full loop: the `bot-issue` skill.

## 8. Agents and process skills

- **Classify first** with `docs/agents/process-skills.md` (Classes A-E). That classification
  satisfies any generic "check skills first" rule; load only the skills it selects.
- **Delegation ladder**, stop at the first rung that works: answer inline when `rg`, `gopls` or a
  targeted read settles it in a few calls; one bounded read-only research agent for a multi-file
  question (server encoding, send order, handler rules); a reviewer only when the user asks.
- Limits: one research agent per ordinary issue, two concurrent agent threads, delegated reports
  of at most ten lines (conclusions, evidence paths, failures). Model choices:
  `docs/agents/model-policy.md`.

## 9. Scoped guides

Guides live in `docs/agents/`. Load only the one the task needs.

| Guide | Load when |
| --- | --- |
| [`go-style.md`](docs/agents/go-style.md) | writing Go: naming, types, errors, concurrency, lock order, packages, dependencies |
| [`test-strategy.md`](docs/agents/test-strategy.md) | writing tests: tiers, fixtures, e2e stack, fakes vs real types |
| [`verification.md`](docs/agents/verification.md) | focused checks, final gates, fuzzing, docs-only checks, completion evidence |
| [`process-skills.md`](docs/agents/process-skills.md) | classifying a task and choosing process skills |
| [`tooling.md`](docs/agents/tooling.md) | exact `rg`, `gopls`, `rtk`, `jq`, ast-grep, grepai commands and fallbacks |
| [`model-policy.md`](docs/agents/model-policy.md) | choosing or changing agent models and effort |

## 10. Claude Code notes

Project skills are in `.claude/skills/`: `bot-issue` (implement one issue), `bot-pr-review` and
`bot-pr-review-comments` (review a PR, address its comments), `bot-waves` (multi-PR wave runs on
the Workflow tool). Wave agent types are in `.claude/agents/wave-*.md`.
