---
name: bot-pr-review
description: >
  Produce a first-pass review of an open pull request in fatal10110/acis_golang_bot: protocol
  fidelity against the server, races and goroutine lifecycle, operational safety (bans, login
  rate, credentials, resource caps), correctness, tests, and whether the PR actually satisfies the
  acceptance criteria of the issue it closes and its docs/plan.md row. Submit every evidence-backed
  defect as an inline GitHub review comment and end with an explicit bottom line. Use when the user
  says "review PR N" for this repository, or when a bot-waves review agent loads it. Do not use
  for addressing comments on a PR (bot-pr-review-comments).
---

# Bot PR review

A plain diff review misses PRs that quietly narrow their own scope. The PR body's scope and
verification sections are the author's claims, not evidence. The linked issue's acceptance
criteria and its `docs/plan.md` row are the contract; the code is the evidence; the server
reference checkout is the oracle for anything on the wire.

Every review ends with a **bottom line** that says, unambiguously, whether any change is required,
including when the only items are nits.

## Priorities

These outrank every other finding and lead the review:

1. **Protocol fidelity.** A decoder consumes exactly `len(payload)` with the server's field widths
   and order; an encoder produces what the server's decoder accepts; the bot never sends what the
   server rejects or ignores (wrong state, too far, too soon). Open the server's encoder, decoder
   or handler yourself; never repeat a citation you have not read.
2. **Concurrency and lifecycle.** Every goroutine has an owner and a shutdown path; a session's
   reader is joined before the next dial; a send on a channel also selects on `done`; a reused read
   buffer never crosses goroutines; timers and tickers stop on teardown. Write out the
   interleaving before concluding either way.
3. **Operational safety.** Login pacing and the credential breaker hold (3 failed logins ban the
   IP for everyone on it); reconnect backs off; frames stay under the server's flood limits;
   secrets never reach logs, flags or the repository; DB access stays read-only and rate-limited;
   resource use stays inside the bot cap.

Then: issue scope met, then plan conformance (owner decisions D1-D8, the PR row's scope),
correctness, tests, reuse and style.

## 1. Gather

```bash
gh pr view <N> --repo fatal10110/acis_golang_bot \
  --json number,title,body,baseRefName,headRefName,headRefOid,isDraft,additions,deletions,changedFiles,closingIssuesReferences,reviews,comments
gh pr diff <N> --repo fatal10110/acis_golang_bot > <scratchpad>/pr<N>.diff
gh api repos/fatal10110/acis_golang_bot/pulls/<N>/comments --paginate
```

(GitHub MCP tools first where connected; the commands name the data needed.)

- Read the **whole** diff. Never write a finding from a truncated view.
- Note what earlier reviews and threads already raised or resolved, so you do not repost it.
- For every `Closes #X`: copy the issue's acceptance criteria into your checklist, and find the
  `docs/plan.md` rows that bind it.
- Check out the head in a detached worktree for tests; never touch the author's branch.

## 2. Verify the PR's claims

Every self-report in the PR body ("decoder matches the server", "no behavior change", "covered by
e2e") is a claim to check. Map each acceptance item to the hunk that satisfies it: no hunk means
already on `main` (prove it) or **unmet**. A PR that describes its scope more narrowly than the
issue requires has a gap, and the gap is a finding.

## 3. Review the diff

- **Wire code**: compare each changed decoder or encoder with the server's, field by field; check
  string and integer widths, signedness, and what happens on a short or oversized payload.
- **Correctness**: trace changed control flow by hand, including error paths and every call site
  of a changed function.
- **Lifecycle**: goroutines, channels, timers, connections and files closed on every path;
  unbounded maps and slices (per-object records grow with every visible object).
- **Tests**: every new branch has a deterministic test; brain logic uses the injected clock, never
  wall time; concurrency changes have a test that drives the bad interleaving (PR CI runs it under
  `-race`); a fixture test asserts the decoder consumed the whole payload.
- **Reuse and design**: an existing helper for the same job, a one-implementation interface, a
  speculative option or layer: cite the existing `file:line` or the single caller.

Run the checks yourself in the review worktree; they are evidence, not the author's word:
`go build ./...`, `go vet ./...`, and the scoped tests of the changed packages without `-race`
(through `scripts/agent-tools/heavy` when running inside a wave). Run the related e2e scenarios
when the stack is reachable, except one the author's full `go test -v` log at the reviewed head
shows passing (in a wave, the lane's `logs/e2e-<head>*.log`): cite that log instead. Still rerun a
scenario that failed, timed out or was flaky in an earlier round, and one the change reaches that
the author did not run. Record exactly what ran, what passed, which logs you accepted, and what
you skipped and why.

## 4. Gate every finding before posting

Post a finding only if you can state: **where** (path and new-side line), the **failure scenario**
(input or interleaving, then the wrong packet, state or ban), the **evidence** (server code, plan
row, test output, call sites checked), and the **fix** in one or two lines. Drop anything
speculative, unrelated to the diff, or already raised.

| Tag | Meaning | Blocks merge? |
| --- | --- | --- |
| `[blocker]` | wire mismatch, race or leak, ban or credential risk, unmet acceptance criterion, artifact that contradicts the plan | yes |
| `[must-fix]` | real bug or missing test for a new branch, lower impact | yes |
| `[should-fix]` | correctness-adjacent or maintainability issue worth fixing in this PR | no, but action expected |
| `[nit]` | small style or naming cleanup | no, but action expected unless marked optional |
| `[note]` | pre-existing gap or follow-up candidate; say where it should be tracked | no |

## 5. Submit

Confirm the head has not moved, then submit one review (event `COMMENT`) with every gated finding
inline. Body, in this order:

```markdown
## Bottom line
**Verdict:** <Blocked | Changes required | Approve after small fixes | Approve as-is>
**Action required before merge:** <YES | NO>
<If YES: numbered list, `[tag] path:line — what to do`, every should-fix and non-optional nit included.>
<Optional items: list or "none". Follow-ups: list with target issue or "none".>

## Protocol, concurrency, operations
<What changes on the wire (server code read to confirm it); goroutines touching changed state and
what serializes them; ban, rate and secret exposure. Checks run and results; skipped + why.>

## Acceptance criteria (#<issue>)
- [x]/[ ] <criterion> — <evidence hunk, or "unmet: …", or "already on main: …">

## Other findings
<one line each, path:line, severity>
```

Verdict rules are mechanical: any `[blocker]` or unmet criterion is **Blocked**; any `[must-fix]`
is **Changes required**; only `[should-fix]` or non-optional `[nit]` is **Approve after small
fixes**; all three mean action YES. Only optional nits, notes or nothing is **Approve as-is**,
action NO. Remove the review worktree afterwards.

## 6. Final chat response

Repeat the bottom line: verdict, action required (YES with the full numbered list, or NO),
optional items, follow-ups, and the checks run. If submission failed, say so first.
