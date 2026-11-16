---
name: bot-waves
description: >
  Plan and run a multi-PR wave of fatal10110/acis_golang_bot issues on the Workflow tool: group
  open issues into lanes (one PR each) and waves, then fan out. Each PR gets a Sonnet lane brief,
  an implementation side that implements and fixes every review round, CI failure and merge
  conflict (handing over to a fresh agent at context checkpoints), and one long-lived review agent
  that reviews the first push in full, re-reviews only each new delta, and requests the merge. A
  merger workflow runs the cheap-model CI/merge agent; a helper workflow runs the cheap helper
  agents (go-scout, researcher, runner) that workflow agents cannot spawn themselves. Use when the
  user says "run the waves", "fan out wave 1", "ship these issues in waves", "implement -> review
  -> merge loop", or asks to execute a grouping/waves plan. Merging is authorized by the user's
  invocation of this skill. Do not use for a single issue (use bot-issue) or for reviewing one PR
  (use bot-pr-review).
---

# bot-waves

Runs a waves plan through `waves.js` (next to this file), launched as several Workflows over one
plan:

- **track** (one Workflow per track): works the track's lanes wave by wave. Per lane (one PR):
  0. **Lane brief.** A Sonnet `wave-brief` agent gathers the issue, its `docs/plan.md` rows, the
     server references (encoders, decoders, opcodes, handler rules), the Go anchors and the test
     commands into `lanes/<slug>/brief.md`, which goes into the implementation agent's prompt, so
     the expensive agent starts coding instead of exploring (`brief: false` turns it off).
  1. **Implementation agent** (session model, or the lane's `implModel`). It implements and opens
     the PR, following `bot-issue`, and the implementation side fixes every review round
     (`bot-pr-review-comments`), CI failure and merge conflict. It hands over at checkpoints
     (right after the first push, and whenever its context has grown long): it returns
     `checkpoint` and a fresh agent continues from the worklog, because cost is turns × context.
  2. **Review agent.** It starts at the first push and stays alive until the PR is merged or
     given up. Round 1 is a full `bot-pr-review`. Every later round is a delta re-review: it
     checks that its own earlier items are fixed and reviews the diff since the head it last
     reviewed. It keeps its context, so it never re-reads code, threads or server sources it
     already read unless the delta touches them. When the PR is clean, it asks the merger to
     merge and routes the result back.

  The two agents talk directly through the lane mailbox, so the script relays nothing between
  rounds. Before the first push, a Haiku watcher holds the second slot instead of an idle review
  agent. The script replaces either agent if it dies; a replacement rebuilds its context once,
  from the worklog or the review files. At the end of each wave, every track meets at a file
  barrier, then the next wave starts.
- **merger** (one Workflow): runs one Haiku CI/merge agent per merge request, one at a time
  across all tracks. The agent checks that the head is still the one the reviewer approved,
  updates the branch, waits for CI (rerunning known flakes) and squash-merges. It never edits code
  or runs tests. A real CI failure or a conflict goes back to the review agent, which sends it to
  the implementation agent.
- **helper** (one Workflow): serves helper requests from every lane. Workflow agents cannot spawn
  subagents, so an agent runs `wave_mail.py ask` and this broker starts the helper:
  - `go-scout` (`wave-scout`, Haiku): one location or fact in a lane worktree or the server
    checkout;
  - `researcher` (`wave-researcher`, Sonnet; at most `researcherPerLane` per lane): one batched
    multi-file investigation, typically how the server encodes, orders or rejects something;
  - `runner` (`wave-runner`, Haiku): runs scoped test or lint commands and returns a compressed
    result.
  The helper agent types are read-only, so a Haiku deliver agent writes each answer back.

The loop per lane:

1. The implementation agent implements and pushes.
2. The review agent reviews. If there are findings, they go to the implementation agent, which
   fixes, pushes, and gets a delta re-review from the same review agent. This repeats until a
   review is clean, at most `maxRounds` rounds.
3. The review agent asks the merger to merge. A real CI failure goes to the implementation agent,
   then gets a delta review. A conflict goes to the implementation agent and is re-reviewed only
   if the resolution was semantic. Then merge again.

Per PR that is one implementation agent and one review agent (session model), one Haiku merge
agent per merge attempt, Haiku watcher, barrier and deliver agents, and any helpers.

All coordination goes through `scripts/agent-tools/wave_mail.py` under
`<repo>-worktrees/.waves/<run>/`. That directory holds the mailboxes (`box/impl`, `box/review`),
lane state, the helper and merge request queues and their answers, reviews, CI notes, and the
barrier markers. It survives container restarts.

## Paths

| Thing | Default | Arg / env |
| --- | --- | --- |
| primary checkout of this repo | `/home/zorg/workspace/acis_golang_bot` | `repo` / `WAVES_GO` |
| lane and review worktrees | `<repo>-worktrees/wf-<slug>`, `review-<slug>` | `worktrees` |
| run state | `<repo>-worktrees/.waves/<run>` | `wavesRoot` / `WAVES_ROOT` |
| server reference checkout (read-only) | `/home/zorg/workspace/acis_public/acis_golang` | `server` |
| GitHub repo | `fatal10110/acis_golang_bot` | `ghRepo` / `WAVES_GH_REPO` |
| CI checks a merge needs green | `lint`, `test` | `requiredChecks` / `WAVES_REQUIRED_CHECKS` |

`waves.js` puts the env values on every tool command it hands an agent, so `wave_mail.py`,
`look.py` and `heavy` agree with the args. Set `requiredChecks` to the job names the repository's
CI actually has (a check matches by exact name or by name followed by a space, as in
`test (ubuntu)`); a required check that never reports leaves the merge waiting.

## Slim agent types

Every workflow agent runs as a role-specific type from `.claude/agents/wave-*.md`. Each type lists
only the tools its role uses (Bash, Read/Edit/Write, ToolSearch and the `mcp__github__*` tools it
calls), so no agent loads the session's remote-control MCP tools, the skill listing or the
deferred-tool list: about 12k tokens per fresh agent instead of about 44k with the default
subagent. Implementation and review agents read their skill files (`bot-issue`, `bot-pr-review`,
`bot-pr-review-comments`) with Read instead of the Skill tool. A role that needs a new tool needs
it added to its agent file. A session loads `.claude/agents` at start, so a type added later runs
on the default subagent until the session restarts.

## Lane tools (token savers, built into every prompt)

A long-lived agent's every tool call re-sends its whole context, so cost tracks the number of
calls, not their size. The prompts therefore point every agent at:

- `look.py <spec>...`: ranges, Go declarations by name (built on demand as
  `<repo>-worktrees/.tools/godecl` from `scripts/agent-tools/godecl`), bounded rg; many lookups in
  one call, in a lane worktree or the server checkout.
- `wave_mail.py worktree --lane S [--review] [--remove]`: creates, moves or removes lane
  worktrees with absolute paths.
- `wave_mail.py pr-state --pr N` and `ci-wait --head SHA`: compact PR state (with `behind_by`, so
  the branch is updated only when behind) and an in-process CI wait for the merge agent.
- `heavy <cmd>`: serializes heavy runs (race tests, e2e, lint) through one lock so parallel lanes
  do not OOM a small box, and kills a hung run with SIGQUIT so Go dumps its goroutines.
- STOP reaches agents through commands they run anyway: every `wave_mail.py` output of a stopping
  run carries `"stop": true`, `look.py` prints `### STOP`, and `heavy` prints it and refuses to
  run from a lane or review worktree of a stopping run.

## Test policy (built into every prompt)

- No agent runs the full suite locally. PR CI runs the unit tier on every push. This overrides
  the full-suite steps in the skills and in `docs/agents/verification.md`.
- The implementation agent (including on fix rounds) and the review agent run only the tests the
  change can affect: unit tests of the changed packages with `-race`, selected with `-run` where a
  package is large. Every run goes through `heavy`.
- **PR CI does not gate the e2e scenarios** (they need the server stack;
  `docs/agents/test-strategy.md`). When a change touches the protocol client, sessions, a codec or
  a behavior a scenario asserts, the agents run the related scenarios locally with
  `BOT_REQUIRE_E2E=1` and confirm they ran rather than skipped.
- The PR body carries an "E2E scenarios (local)" line, and the reviewer re-runs them when the
  stack is reachable. A related scenario that is missing or failing is a [must-fix].
- The merge agent never runs tests. The `runner` helper refuses full-suite commands.

## Concurrency budget (read before choosing tracks)

The Workflow tool caps concurrent agents per workflow at `min(16, CPUs - 2)`. On a 4-CPU box the
cap is **2**. A lane needs 2 slots: the implementation agent plus the review agent, or plus the
watcher before the first push. Merges and helpers run in their own Workflows with their own caps,
so they never take a lane's slot. So:

- One track workflow runs `lanesPerTrack = floor(slots / 2)` lanes at a time: **1** on 4 CPUs.
- Parallel PRs come from **several track workflows** launched at once.
- On 4 CPU: **2 tracks + merger + helper** (4 Workflows). Heavy test runs are serialized by
  `heavy` anyway. On a bigger box, pass `slots` (the cap) and more tracks.

## Step 1: build the plan

Start from the live open issues the user put in scope and their milestone epics (M0-M5 in
`docs/plan.md` "Tracking"). Rules:

- **Lane** = one PR. Combine issues that touch the same function or file for the same reason, or
  would conflict with each other. Keep a lane to one reviewable change; the plan's "PR breakdown"
  is the default grouping.
- **Wave** = lanes that can start together. A dependency on another lane goes to an earlier wave,
  or to the same wave on the **same track** (it then waits for that lane to merge). Within a
  milestone, the plan's sub-issue order is the usual dependency order (M0: skeleton and CI, then
  codecs and crypt, then the client).
- **Track**: lanes that share a hot file (`hot` group) in the same wave go on one track. The plan
  validator refuses a hot group split across tracks. Balance the tracks by expected size.
- Leave out issues that need an owner decision, are blocked on an open server prerequisite
  (`fatal10110/acis_golang#4525`-`#4528`), belong to another session's uncommitted work, or are
  epics. Put them in `exclusions` where agents must not touch them, and list them for the user.
- A lane may be audit-only (close done issues, file follow-ups): say so in `note`; the
  implementation agent returns `no_change` with no PR.

Plan file (`<repo>-worktrees/.waves/<run>/plan.json`). Shape, also in `plan.example.json`:

```json
{
  "tracks": ["A", "B"],
  "exclusions": "epics #1 and #6; the server checkout (read-only)",
  "env": "optional extra environment rule lines for every agent",
  "waves": [
    {"name": "1", "lanes": [
      {"slug": "wire-crypt", "track": "A", "issues": [3], "milestone": "M0 - Bootstrap",
       "note": "copy wire codecs and crypt per the plan's PR0", "hot": ["layout"], "deps": []}
    ]}
  ]
}
```

`slug` is the branch `wf/<slug>` and worktree `<repo>-worktrees/wf-<slug>`; it must be unique
across the run. Optional per lane: `implModel` / `reviewModel` (a model alias such as `sonnet`)
and `implEffort` / `reviewEffort` (low|medium|high|xhigh|max) run that lane's implementation or
review agent on another model or effort than the session's. Use the cheaper implementation model
only for mechanical lanes (test harness, fixtures, docs, deployment guides); keep protocol,
concurrency and every review on the session model.

Validate the plan:

```bash
python3 <repo>/scripts/agent-tools/wave_mail.py validate <plan.json>
```

Show the user the plan (waves, tracks, lanes, exclusions) before launching unless they already
approved a grouping in this conversation.

## Step 2: preflight

- The primary checkout is on `main` and clean; `git -C <repo> fetch origin` works.
- The server reference checkout exists and is current enough for the lanes' packets
  (`git -C <server> log -1`).
- The CI job names match `requiredChecks` (look at a recent PR's checks).
- E2E lanes: the server stack is up or can be started as `docs/agents/test-strategy.md` says. If
  not, say so; e2e lanes will report their scenarios as not run.
- No leftover `wf-<slug>` worktree or `wf/<slug>` branch from another run uses a planned slug. If
  one does, either rename the lane or keep it on purpose: a fresh lane reuses and inspects an
  existing worktree.
- No open PR already covers a lane (search open PRs for the issue numbers).
- `<tool>` is the absolute path of `scripts/agent-tools/wave_mail.py` in the checkout this skill
  was loaded from (the skill files are read from the same checkout).
- Pick `run`: a short id such as `w1116`. Reusing a run id resumes it (Step 4).
- Cost check: each PR uses one implementation agent and one review agent on the session model,
  plus Haiku merge, watcher and deliver agents. Say so if the user has not seen it.

## Step 3: launch

Launch one Workflow per track, one merger and one helper broker, all in the same turn, with the
same `run`, `plan` and `tool`. Pass `scriptPath` as the absolute path of `waves.js`. If the tool
refuses a path outside the session directory, Read the file once and pass its text as `script`;
then reuse the persisted path that result returns.

```text
Workflow({scriptPath: "<skill dir>/waves.js", args: {run, epoch, role: "merger", plan, tool}})
Workflow({scriptPath: "<skill dir>/waves.js", args: {run, epoch, role: "helper", plan, tool}})
Workflow({scriptPath: "<skill dir>/waves.js", args: {run, epoch, role: "track", track: "A", plan, tool}})
Workflow({scriptPath: "<skill dir>/waves.js", args: {run, epoch, role: "track", track: "B", plan, tool}})
```

`plan` is the parsed plan object (a JSON value, not a string). `epoch` names this launch: `"1"` for
the first, and a new value for every relaunch of the whole run. The wave-barrier and
track-finished markers are kept per epoch, so a relaunch never sees markers from an earlier launch.
Pass the same path args (table above) to all four Workflows. Optional args:

| Arg | Default | Meaning |
| --- | --- | --- |
| `slots` | 2 | the per-workflow agent cap; sets `lanesPerTrack` = floor(slots/2) |
| `lanesPerTrack` | from `slots` | lanes running at once in one track |
| `maxRounds` | 8 | review rounds per PR before the lane ends `unresolved` |
| `reviewCiFixes` | true | a CI fix gets a delta review before the next merge attempt |
| `waitCalls` | 6 | recv calls (~9.5 min each) per first-push watcher agent |
| `takeCalls` | 6 | take polls (~9.5 min each) per broker poller agent before it returns |
| `maxIdleMin` | 240 | minutes without a mailbox message before a lane ends `stalled` |
| `restarts` | 2 | replacements per lane for the review agent, and separately for the implementation side, when an agent dies or exits early (checkpoints do not count) |
| `researcherPerLane` | 1 | researcher requests per lane (impl and review combined) |
| `agentTypes` | `{coord: 'wave-coord', deliver: 'wave-deliver', merge: 'wave-merge', impl: 'wave-impl', review: 'wave-review', runner: 'wave-runner', scout: 'wave-scout', researcher: 'wave-researcher', brief: 'wave-brief'}` | agent type per role; `null` for a role falls back to the default workflow subagent |
| `models` | `{merge, coord, deliver, scout, runner: "haiku", brief, researcher: "sonnet"}` | model overrides per role (`impl`, `review` included, for every lane; a lane's `implModel`/`reviewModel` wins) |
| `retry` | [] | slugs to run again even though saved state says they finished; a retried lane gets a fresh `maxRounds` budget |
| `brief` | true | a `wave-brief` agent writes `lanes/<slug>/brief.md` before a fresh lane's implementation agent starts; it is put in that agent's prompt |
| `maxCheckpoints` | 6 | hand-overs per lane; once spent, the next agent's prompt says none are left |
| `sessionModel` | `Claude Opus 5.5` | the model name in the Co-Authored-By trailer of agents on the session model |

Every role's workers inherit the session model unless listed in `models`.

## Step 4: monitor, intervene, resume

- Progress: `python3 <tool> status --run <run>` prints one line per lane:
  `slug|status|stage|pr|round`. Stages are `impl`, `review`, `fix`, `merge`, `ci-fix` and
  `conflict`. Each workflow's result lists every lane's status, PR, rounds, history and follow-ups.
- Each lane directory has the implementation agent's `worklog.md`, `brief.md`, `reviews/r<N>.md`,
  `ci/c<N>.md` and its mailboxes `box/impl` and `box/review`.
- Terminal lane statuses: `merged`, `no_change`, `blocked`; `unresolved` (still had findings
  after `maxRounds`; the PR stays open); `stalled` (idle for `maxIdleMin`); `merge_blocked` (the
  permission system denied the merge: merge it by hand, then run
  `<tool> state --run R --lane S --set status=merged` so later deps pass); `failed`; `skipped` (a
  dep did not merge); `stopped`.
- Drain (finish what is under way, start nothing new): `<tool> stop --run <run> --drain`. Lanes
  that already have a worktree go on through review and merge; the first lane that would start
  returns `stopped`, and its track then starts no further lane.
- Stop everything: `<tool> stop --run <run>`. Every waiting agent returns, the implementation and
  review agents update their worklog/notes and return `stopped`, the merger and helper end, and
  the tracks stop at their next wait. A merge agent already waiting on CI finishes its merge
  first.
- **Resume** (after a restart, a stop or a usage limit): delete `<run dir>/STOP` if present, then
  launch the same four Workflows again with the same args and a **new `epoch`**. Never use
  `resumeFromRunId`: the concurrent lanes break its cache. Each track skips finished lanes and
  starts replacement agents with a RESUME note for active ones. Put slugs in `retry` to run
  finished lanes again.
- A merger or helper workflow that died can be relaunched alone with the same `epoch`. Its first
  poll re-serves the requests the dead instance took but never answered. Relaunch a broker only
  after it has died: two live brokers of one kind would serve the same request twice.
- A track that crashed never reaches the barrier, so the other tracks wait. Relaunch only that
  track with the **same** `epoch`, or stop the run.

## Step 5: wrap up

When all workflows have returned, report per lane: status, PR, merge SHA, rounds, follow-ups
filed; and every lane that did not merge, with the reason and what is left (open PR, worktree
kept). Check that each "Closes" issue is closed, and that no `wf-<slug>` worktree remains for a
merged lane.

## Editing waves.js

The script is plain JavaScript run by the Workflow tool. It has no filesystem access and no
`Date.now()`/`Math.random()`. After any change, run the offline simulation and the tool tests:

```bash
node .claude/skills/bot-waves/waves_sim.mjs
python3 -m unittest discover -s scripts/agent-tools -p 'test_*.py'
```

The simulation runs the script with fake agents that talk through an in-memory mailbox, under
the 2-slot cap. It checks: one implementation and one review agent per PR across every review, CI
and conflict round; `maxRounds` ends a lane `unresolved` and skips its dependents; resume from
saved state; replacements for dead agents; serial merges and answers for a dead merge agent; the
helper broker's researcher limit and helper agent types; the example plan on both tracks; slim
agent types and the fallback for a missing type; STOP and DRAIN; the lane brief and checkpoints.
