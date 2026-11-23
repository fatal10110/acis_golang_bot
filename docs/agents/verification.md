# Verification policy

Verification has two levels: fast feedback while editing, and complete gates before claiming a
change is done.

## Focused checks during implementation

Choose the smallest command that exercises the changed behavior:

```bash
rtk go -C <worktree> test ./internal/<affected-package>
rtk go -C <worktree> test ./internal/<affected-package> -run '<FocusedTest>'
```

Do not rerun the whole module after each small edit. Do not pass `-race` locally unless the owner
explicitly asks for a race run; PR CI runs the unit tier with `-race`, and e2e scenarios never run
with it.

Contract-specific checks supplement unit tests:

- codecs: round trips, recorded fixtures (exact `len(payload)` consumption), and a field-by-field
  comparison with the server's encoder or decoder for every packet touched;
- crypt: fixed vectors, including the boundary cases (padding, block alignment, key scrambling);
- brain: table rows with the injected clock for every rule touched, including the "do nothing" rows;
- pacing and rate limits: a test that drives the sender past the limit and asserts it holds;
- concurrency: a test that drives the concurrent callers (PR CI runs it under `-race`) and teardown
  (the session's goroutines all exit, no send blocks after `done`).

## Complete gates

Before claiming a source change is done, the formatting check prints nothing and the build, vet,
lint and scoped tests pass:

```bash
(cd <worktree> && golangci-lint fmt --diff)
rtk go -C <worktree> vet ./...
rtk go -C <worktree> build ./...
rtk go -C <worktree> test -count=1 <changed packages> -run '<tests that exercise the change>'
(cd <worktree> && rtk err golangci-lint run <touched package dirs>)
```

Run only the tests the change can affect. PR CI runs the full unit tier on every push and is the
gate for it; the PR body lists the scoped tests run and says "unit suite: CI". If a binary is
unavailable, report it as unavailable, never as passed.

### E2E scenarios

PR CI does not gate the e2e scenarios (`test-strategy.md`). A change to the protocol client,
login or game sessions, reconnect, a codec, or a behavior a scenario asserts runs the related
scenarios locally against the live server (`docs/e2e-live.md`) or the stack with
`BOT_REQUIRE_E2E=1`, scoped with `-run` and `-v`.
Confirm in the output that they ran: a SKIP verifies nothing. Report them in the PR body as an
"E2E scenarios (local)" line with each result, "none related", or "not run: <reason>".

### Fuzzing

A change to the frame reader, a decryption path, or any decoder of server frames also runs the
fuzz target that covers it for at least 60 s:

```bash
go -C <worktree> test ./<package> -run '^$' -fuzz '^<FuzzTarget>$' -fuzztime 60s
```

A new decoder joins its package's fuzz target with a well-formed seed.

## Documentation and agent configuration

When only Markdown, JSON, YAML or agent configuration changes, the Go gates are not required.
Instead:

- parse every changed JSON and YAML file;
- for the waves tooling, run `node .claude/skills/bot-waves/waves_sim.mjs` and
  `python3 -m unittest discover -s scripts/agent-tools -p 'test_*.py'`;
- check Markdown links and stale paths;
- verify no source file changed.

## Completion evidence

Report the commands actually run and their exit status. Do not claim an unrun gate passed. For each
shipped gap or out-of-scope item, report the follow-up issue number and confirm it covers the
deferred work. For issue work, report the PR URL and confirm it targets `main`, carries the closing
keyword, includes only the scoped change, and ends with `Gap audit: no shipped gaps` or the mapped
gaps.
