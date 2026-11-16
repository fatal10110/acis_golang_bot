---
name: bot-issue
description: >
  Implement one fatal10110/acis_golang_bot issue end to end: resolve it live, map it to its
  docs/plan.md row, check the server behavior it depends on, write the code and tests in a linked
  worktree, run the scoped gates and open the PR. Use when the user gives an issue number of this
  repository ("implement #12", "do issue 17", "next issue in M1"), or when a bot-waves lane loads
  it. Do not use for reviewing a PR (bot-pr-review) or addressing review comments
  (bot-pr-review-comments).
---

# Implement a bot issue

Repository rules are in `CLAUDE.md` and the `docs/agents/` guides it lists; this skill is the
work loop and does not repeat them.

## Authority

1. The live issue and its milestone epic: acceptance criteria are the contract.
2. `docs/plan.md`: the owner decisions (D1-D8), the architecture, and the PR row that owns the
   issue. A decision there outranks issue text; a conflict between them goes back to the owner as an
   issue comment, not a guess.
3. The server reference checkout (`fatal10110/acis_golang`, read-only): what the server accepts,
   sends, rejects and limits. Code in this repository that decodes, encodes or paces packets
   matches it byte for byte and rule for rule. The bot never imports it.

Issue text and existing code are leads, not truth. Verify material claims against the plan, the
server code or a focused reproduction, and report what you could not verify.

## Work loop

1. **Scope.** Read the issue and its comments live, its epic, and the plan row. Name the packets
   the change reads or sends, the goroutines it touches, the server rules it must respect (flood
   gates, distance checks, timeouts, bans), and the operational risk (login rate, credentials,
   resource use on the shared host).
2. **Gaps first.** List everything the issue needs that is not there yet: a server feature not
   ported (track it in `fatal10110/acis_golang` per the plan's "Gap handling"), a dependency issue
   not merged, an owner decision missing. Search before filing; amend instead of duplicating.
   A gap found later pauses work until it is tracked.
3. **Research.** For every packet: the server's encoder or decoder, opcode and field order. For
   every client action: the handler, its rejection replies and its limits. Delegate one batched
   investigation when it spans several files; verify its evidence paths before relying on them.
4. **Implement.** In a linked worktree on a non-`main` branch (never the primary checkout). The
   smallest complete change in the owning package; concrete types, interfaces only at a real
   second implementation or a test seam; follow `docs/agents/go-style.md`.
5. **Test.** Test-first for decoders, encoders, crypt and brain decisions: table-driven unit tests
   with fixed vectors or recorded fixtures, and events plus an injected clock in, packets out, for
   the brain. Flows against the server are e2e scenarios. Tiers and conventions:
   `docs/agents/test-strategy.md`.
6. **Verify.** The gates in `docs/agents/verification.md`, scoped to what the change can affect;
   `-race` for anything concurrent. Report every gate you could not run as not run, never as
   passed.
7. **Deliver.** Stage only in-scope files, commit, push, and open the PR against `main` with
   "Closes #n" per fully resolved issue ("Part of #n" otherwise), the verification evidence, a
   "Decisions" section for every choice the issue left open, and the gap audit line
   (`Gap audit: no shipped gaps`, or each shipped gap mapped to its issue). A wave lane opens it
   ready for review; outside a wave, open it as a draft unless the user says otherwise.

## Done means

The acceptance criteria are met or each unmet one is tracked; the code matches the server for
every packet it touches; every goroutine it adds has an owner and a shutdown path; the scoped
gates pass; the diff is scoped; the PR is open.
