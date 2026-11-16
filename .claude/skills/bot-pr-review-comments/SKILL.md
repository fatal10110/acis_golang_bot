---
name: bot-pr-review-comments
description: >
  Address review comments on an open fatal10110/acis_golang_bot pull request (human or automated
  reviewer): fetch every unresolved thread and review body, verify each claim against the code,
  docs/plan.md and the server reference before touching anything, fix only confirmed in-scope
  findings with tests, file follow-ups for the rest, and reply with evidence on every thread. Use
  when the user asks to address, fix or answer review feedback on this repository's PR, or when a
  bot-waves implementation agent gets a review round. Do not use to produce a review
  (bot-pr-review).
---

# Address PR review comments

Verify first, fix second. A reviewer's prose is a claim, not a fact.

## 1. Fetch

Fetch the PR head, every **unresolved** review thread (GraphQL `reviewThreads`, which carries
`isResolved` and `isOutdated`; the REST comment list does not), and every review body. A
summary-only review has no thread, so non-empty review bodies with actionable content are work
items too. For an outdated thread, check whether the cited line still exists at its new location.

## 2. Verify every comment

- Open the cited `file:line` and confirm the code does what the comment says.
- A comment about the wire (field order, width, opcode, what the server rejects or limits): open
  the server's encoder, decoder or handler yourself. Never carry a citation you have not read
  into a commit, reply or PR body.
- A race, ordering or leak claim is verified in either direction by writing out the interleaving
  (which goroutine holds what at each step) or by a failing test; rejecting it needs the lock,
  channel or `done` path that makes it impossible.
- A scope claim is checked against the issue's acceptance criteria and its `docs/plan.md` row.

Classify only after verifying:

- **Valid, in scope**: a confirmed defect, or a gap the PR's own issue commits to. Fix it.
- **Valid, out of scope**: confirmed but pre-existing, not introduced or widened by this PR. File
  or amend an issue in the owning milestone (search first), reply with its number.
- **Invalid**: the citation does not hold, the behavior is already correct, or a later commit
  addressed it. Reply with the refuting evidence; no change.
- **Question**: answer it.

## 3. Fix

On the PR's own branch, in its worktree. The smallest complete fix per finding, with a regression
test that fails before and passes after; a race fix is tested in the racing direction. Run the
scoped gates from `docs/agents/verification.md`. The commit message names the behavior fixed, not
"address review". Push (never force-push) before resolving threads that reference the fix.

## 4. Reply and resolve

Every thread gets a reply before it is resolved: the fix commit, the follow-up issue number, or
the refuting evidence. A review body without a thread gets one PR comment answering it point by
point. Never resolve a thread silently, and never close a confirmed wire, race or ban risk with a
reply alone: fix it or track it.

## 5. Report

Which comments were fixed (with commits), rejected (with evidence), or tracked (with issue
numbers), and which gates ran.
