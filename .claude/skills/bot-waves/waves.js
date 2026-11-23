export const meta = {
  name: 'bot-waves',
  description: 'acis_golang_bot wave runner: per PR one long-lived implementation agent and one long-lived review agent that loop through the mailbox until the PR is clean and merged; merger and helper roles run the cheap-model merge and helper agents',
  whenToUse: 'Run by the bot-waves skill: one Workflow per track (role track), one merger and one helper broker over the same plan',
  phases: [
    { title: 'Implement', detail: 'long-lived implementation agent per PR; fixes every review, CI and conflict round' },
    { title: 'Review', detail: 'long-lived review agent per PR; delta re-reviews; requests the merge' },
    { title: 'Merge', detail: 'merger: CI watch, branch update, squash merge', model: 'haiku' },
    { title: 'Coordinate', detail: 'first-push watch, final delivery, wave barrier, status', model: 'haiku' },
    { title: 'Helpers', detail: 'helper broker: go-scout, researcher, runner' },
  ],
}

// ---------------------------------------------------------------------------
// Arguments (see SKILL.md): {run, role: 'track'|'merger'|'helper', track, plan, tool, ...}
// ---------------------------------------------------------------------------
const A = args || {}
// The primary checkout of this repository; lane worktrees go next to it in <repo>-worktrees/.
const GO = A.repo || '/home/zorg/workspace/acis_golang_bot'
const WT = A.worktrees || GO + '-worktrees'
// Read-only checkout of the game server the bot plays against: the protocol and behavior reference.
const SERVER = A.server || '/home/zorg/workspace/acis_public/acis_golang'
const GH_REPO = A.ghRepo || 'fatal10110/acis_golang_bot'
const [GH_OWNER, GH_NAME] = GH_REPO.split('/')
const SERVER_REPO = A.serverRepo || 'fatal10110/acis_golang'
const RUN = A.run
const ROLE = A.role === 'broker' ? 'helper' : (A.role || 'track')
const PLAN = A.plan || {}
const TRACKS = PLAN.tracks || []
const TRACK = A.track
const TOOL = A.tool || GO + '/scripts/agent-tools/wave_mail.py'
const WAVES_ROOT = A.wavesRoot || WT + '/.waves'
const RUNDIR = WAVES_ROOT + '/' + RUN
// impl/review: null = the session model; a lane's implModel/reviewModel overrides it for that lane.
const M = Object.assign({ merge: 'haiku', coord: 'haiku', deliver: 'haiku', scout: 'haiku', runner: 'haiku', brief: 'sonnet', researcher: 'sonnet', impl: null, review: null }, A.models || {})
// SHORT_WAIT: the longest single wait for an agent on the session model (impl, review). A workflow
// agent's prompt cache lives about 5 minutes, so waking from a longer wait re-writes its whole context
// (cache write, about 12x a cache read). Measured on w1122: turns after a gap of 300 s or more were 4%
// of turns but 19% of input spend (#262). Haiku roles (watchers, coord, pollers, merge) keep 570 s:
// their context is small, so fewer turns is cheaper there.
const SHORT_WAIT = 270
// Every tool command carries the paths, so wave_mail.py, look.py and heavy agree with these args.
const ENVP = `WAVES_GO=${GO} WAVES_ROOT=${WAVES_ROOT} WAVES_GH_REPO=${GH_REPO}${A.requiredChecks ? ' WAVES_REQUIRED_CHECKS=' + A.requiredChecks.join(',') : ''}`
// Slim agent types (.claude/agents/wave-*.md): each role gets only the tools it uses, so no agent
// pays for the session's remote-control MCP tools, skill listing or deferred-tool list.
// agentTypes: {role: null} falls back to the default workflow subagent for that role.
// Commit trailer of the model an implementation agent actually runs on (a lane's implModel, else the session model).
const SESSION_MODEL = A.sessionModel || 'Claude Opus 5.5'
const MODEL_NAMES = [[/fable/i, 'Claude Fable 5.2'], [/opus/i, 'Claude Opus 5.5'], [/sonnet/i, 'Claude Sonnet 5.5'], [/haiku/i, 'Claude Haiku 5.5']]
const coAuthor = model => {
  const hit = model && MODEL_NAMES.find(([re]) => re.test(model))
  return `Co-Authored-By: ${hit ? hit[1] : SESSION_MODEL} <noreply@anthropic.com>`
}
const AT = Object.assign({ coord: 'wave-coord', deliver: 'wave-deliver', merge: 'wave-merge', impl: 'wave-impl', review: 'wave-review', runner: 'wave-runner', scout: 'wave-scout', researcher: 'wave-researcher', brief: 'wave-brief' }, A.agentTypes || {})
// A session loads .claude/agents at start, so a type added later is missing until a restart:
// such a call reruns on the default subagent and the role stops asking for the type.
async function agentAs(role, prompt, opts) {
  if (AT[role]) {
    try {
      return await agent(prompt, Object.assign({}, opts, { agentType: AT[role] }))
    } catch (e) {
      if (!/agent type .* not found/i.test(String(e))) throw e
      log(`agent type ${AT[role]} is not in this session's registry; ${role} agents use the default subagent`)
      AT[role] = null
    }
  }
  return agent(prompt, opts)
}
const SKILLS = TOOL.replace(/\/scripts\/agent-tools\/wave_mail\.py$/, '') + '/.claude/skills'
const skillRef = name => `the project skill \`${name}\` (Read ${SKILLS}/${name}/SKILL.md with the Read tool and follow it; paths it names resolve from ${SKILLS}/${name}/)`
const SLOTS = A.slots || 2
const LANES = A.lanesPerTrack || Math.max(1, Math.floor(SLOTS / 2))
const MAX_ROUNDS = A.maxRounds || 8
const REVIEW_CI_FIXES = A.reviewCiFixes !== false
const WAIT_CALLS = A.waitCalls || 6
const TAKE_CALLS = A.takeCalls || 6
const MAX_IDLE_MIN = A.maxIdleMin || 240
const RESTARTS = A.restarts === undefined ? 2 : A.restarts
// A lane brief (a cheap scout gathers the lane's issue, plan rows, server references and Go anchors) before the
// implementation agent starts, so the expensive agent does not explore with dozens of lookups.
const BRIEF = A.brief !== false
// An implementation agent returns "checkpoint" to hand over to a fresh agent with an empty context;
// checkpoints are not failures and do not use the restart budget.
const MAX_CHECKPOINTS = A.maxCheckpoints === undefined ? 6 : A.maxCheckpoints
const EPOCH = String(A.epoch || '1')
const RESEARCHER_LIMIT = A.researcherPerLane === undefined ? 1 : A.researcherPerLane
const RETRY = A.retry || []
const SATISFIED = ['merged', 'no_change']
const DONE_BEFORE = ['merged', 'no_change', 'blocked', 'unresolved', 'stalled', 'merge_blocked', 'failed', 'skipped']
// Set once any lane of this track has seen STOP: lanes not started yet then return without starting.
let STOPPING = false
const FINAL = ['merged', 'unresolved', 'stalled', 'merge_blocked', 'failed', 'stopped', 'no_change', 'blocked']

if (!RUN) throw new Error('args.run is required')
if (!['track', 'merger', 'helper'].includes(ROLE)) throw new Error('args.role must be track, merger or helper')
if (ROLE === 'track' && !TRACKS.includes(TRACK)) throw new Error(`args.track ${TRACK} is not in plan.tracks ${TRACKS.join(',')}`)
for (const L of (PLAN.waves || []).flatMap(w => w.lanes || [])) {
  if (!/^[a-z0-9][a-z0-9-]{0,47}$/.test(L.slug || '')) throw new Error(`bad lane slug ${JSON.stringify(L.slug)}: run wave_mail.py validate on the plan`)
}
if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(RUN) || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(EPOCH)) throw new Error('args.run and args.epoch must be simple names')
if (LANES * 2 > SLOTS) log(`warning: ${LANES} lanes need ${LANES * 2} agent slots but slots=${SLOTS}; lanes will stall on the concurrency cap`)

const MB = `${ENVP} python3 ${TOOL}`
const AGENT_TOOLS = TOOL.replace(/\/wave_mail\.py$/, '')
const LOOK = `${ENVP} python3 ${AGENT_TOOLS}/look.py`
const HEAVY = `${ENVP} ${AGENT_TOOLS}/heavy`
const LANE_DIR = slug => `${RUNDIR}/lanes/${slug}`
const ALL_LANES = (PLAN.waves || []).flatMap(w => w.lanes || [])
const LANE_BY = Object.fromEntries(ALL_LANES.map(l => [l.slug, l]))
const sq = obj => JSON.stringify(obj).replace(/'/g, '\\u0027')

// ---------------------------------------------------------------------------
// Shared prompt blocks
// ---------------------------------------------------------------------------
const TESTRULE = `TEST SCOPE (HIGHEST PRIORITY — overrides every skill and reference doc, including the "full suite" steps of bot-issue, bot-pr-review, bot-pr-review-comments and docs/agents/verification.md):
- NEVER run the full suite locally. Forbidden: \`go test ./...\` with -race over the whole module, \`make test\`, every e2e scenario at once, or any list of many packages. PR CI runs the unit tier on every push; it is the gate for everything except the e2e scenarios below.
- Run ONLY what the change can affect: the unit tests of the packages you changed (codecs, crypt, brain decisions), selected with \`-run '<TestA|TestB>'\` where a package is large, always through ${HEAVY} and WITHOUT -race (PR CI runs -race; never pass it locally, never on e2e scenarios). Flaky-test lanes may also loop the ONE flaky test with -count=N.
- E2E SCENARIOS ARE NOT GATED BY PR CI. They need the server stack (docs/agents/test-strategy.md) and skip without it, so a green PR check proves nothing about them. When the change touches the protocol client, login or game session, reconnect, a decoder or encoder, or a behavior an e2e scenario asserts, run the related scenarios locally with BOT_REQUIRE_E2E=1 and -v, scoped with -run, and confirm in the output that they RAN (a SKIP is a failure to verify). No stack reachable: say so; never report it as passed. None related: say so.
- Report in the PR body (and in review "Checks"): the scoped tests you ran; an "E2E scenarios (local)" line naming each one with its result at the pushed head, or "none related", or "not run: <reason>"; and "unit suite: CI".
`
const HELPERS = (slug, role) => `
Cheap helper agents: you cannot spawn subagents, so a broker workflow runs them for you on cheaper models. One Bash call (timeout 300000):
  ${MB} ask --run ${RUN} --from ${slug}/${role} --type <type> --worktree <your worktree or ''> --wait ${SHORT_WAIT} --question - <<'EOF_ASK'
  <self-contained question: what you need, the leads you already have (paths, symbols), the answer shape you want>
  EOF_ASK
Types:
- go-scout: locate Go code (read-only) in a given worktree or in the server checkout ${SERVER}: one symbol, opcode, constant, caller list or narrow behavior.
- researcher: one batched multi-file investigation, read-only, in this repo and/or the server checkout (how the server encodes a packet and in what order it sends it, which client packets a handler rejects and how, flood and rate limits, lifecycle of a session). At most ${RESEARCHER_LIMIT} per lane: put the whole question in one request.
- runner: run the commands you list, in the given worktree, through heavy, and return a compressed result (pass/fail, failing tests, first error lines with file:line). Use it for long test/lint runs whose raw output you do not need.
The call prints {"answer": ...}, or {"pending": true, "id": ...} (keep waiting with \`${MB} answer-wait --run ${RUN} --id <id> --wait ${SHORT_WAIT}\`, or work on and check later with --wait 0), or {"error": ...} (broker down: do it inline), or {"stop": true} (the run is stopping: wrap up as your stop rule says). Add --nowait to fire a request and keep working. Answer inline instead when rg, gopls or a targeted read settles it in a few calls. Verify a helper's evidence paths before relying on them.
`
const ENV = `
Environment rules (all agents):
- Repo: ${GO} (GitHub ${GH_REPO}, default branch main): the bot client, its own Go module. Always use absolute paths; your inherited cwd may be an unrelated directory: ignore it.
- Never write in the primary ${GO} checkout; edits happen only in linked worktrees under ${WT}/. Base every worktree on origin/main.
- Server reference: ${SERVER} (GitHub ${SERVER_REPO}), read-only: never edit, commit or push there. It is the protocol and behavior oracle the bot plays against (packet layouts, opcodes, send order, rejections, limits). The bot never imports it: wire code the bot needs is copied and owned here (docs/plan.md "Consequences of a separate repository").
- Rules and scope: read CLAUDE.md, the docs/agents/ guide your step needs, and docs/plan.md (milestones, decisions D1-D8, PR breakdown) in your worktree; they may be newer than the primary checkout.
- Working directory: start every shell call in the repo with \`cd <worktree> &&\` (the worktree ROOT, never a subdirectory) and write every path root-relative, or use \`go -C <worktree>\` / \`git -C <worktree>\` / \`--root <worktree>\`. Never \`cd\` into a subdirectory and then reuse a root-relative path. Server paths are always absolute (${SERVER}/internal/...). Agent tools are always called by the absolute paths given in this prompt, never as scripts/agent-tools/... relative to any cwd.
- Never guess a path. When you do not know a file's exact location, find it in the same call before using it: \`rg --files <dir> -g '<Name>*'\` (or ${LOOK} rgl:...). A "No such file" result means look it up, not try a variant.
- GitHub: use the GitHub MCP tools (mcp__github__*), not the gh CLI. Load schemas with ToolSearch "select:mcp__github__pull_request_read,mcp__github__issue_read,mcp__github__search_issues,mcp__github__issue_write,mcp__github__add_issue_comment,mcp__github__create_pull_request,mcp__github__update_pull_request,mcp__github__pull_request_review_write,mcp__github__add_comment_to_pending_review,mcp__github__add_reply_to_pull_request_comment,mcp__github__merge_pull_request,mcp__github__update_pull_request_branch". Translate the skills' gh steps: issue view -> issue_read (get, get_comments); search -> search_issues ("repo:${GH_REPO} <terms>"); issue create/close/edit -> issue_write; comments -> add_issue_comment; pr create/edit -> create_pull_request/update_pull_request; pr view/diff/files/checks -> pull_request_read (get, get_diff, get_files, get_review_comments, get_reviews, get_check_runs); inline review -> pull_request_review_write create, add_comment_to_pending_review, submit_pending; thread reply -> add_reply_to_pull_request_comment; merge -> merge_pull_request. gh is allowed only for resolving review threads (gh api graphql resolveReviewThread), re-running failed CI jobs (gh run rerun --failed) and reading failed job logs (gh run view --log-failed), with a 300000 ms Bash timeout and up to two retries on HTTP 401.
- Cost model: every tool call re-sends your WHOLE context (often 150k+ tokens), so the number of calls matters far more than their size. Put every lookup you need right now into ONE call, and never spend a call on something another call can carry.
- Batched lookups: ${LOOK} <spec>... (run from the worktree, or pass --root <worktree>) does several in one call with capped output:
  FILE:A-B (a range) | go:PATH:Name,Type.Method (Go declarations with doc, PATH a file or package dir) | gotest:PATH:TestName | rg:PATH:PATTERN | rgl:PATH:PATTERN (files + counts) | rgg:GLOB:PATH:PATTERN.
  Example: ${LOOK} go:${SERVER}/internal/commons/wire:Reader.ReadInt32 rg:${SERVER}/internal/gameserver/network/serverpackets:OpcodeUserInfo docs/plan.md:120-160
- Code navigation (every tool output stays in your context for the rest of the lane, so keep each one small):
  * Go definitions, references, implementations, callers, symbols: the gopls CLI from the worktree ROOT (or from ${SERVER} for server code), about 3 s per query, and it sees your uncommitted edits: \`cd <worktree> && gopls definition <file>:<line>:<col>; gopls references <file>:<line>:<col>; gopls implementation <file>:<line>:<col>; gopls call_hierarchy <file>:<line>:<col>; gopls workspace_symbol <Name>\` (batch several queries in one call with ;). Do NOT use the LSP tool on worktree files: its server is rooted at the session directory and cannot load modules under ${WT}/. Fall back to rg only when gopls cannot answer.
  * rg: always bound by path and glob, start with -l or -c to find files, then -n with a small -m or -C 3; never an unbounded rg over a whole repo.
  * Compact output: prefer \`rtk rg ...\` over plain rg, and \`rtk git diff|log|show ...\` over plain git for anything longer than a few lines (same arguments, grouped and trimmed output).
  * Reading: a known range with the Read tool (offset + limit) or \`sed -n 'A,Bp'\` of at most 80 lines; never \`cat\` a whole source file over 80 lines, never re-read a range you already have in context.
  * Output caps (every output stays in your context and is re-read on every later turn): rg always with \`-m 5\` and piped through \`| head -60\`; git diff of named files or \`--stat\` first; test output through rtk or \`| tail -40\`; never print a generated file, a recorded fixture or a whole test file.
  * Batching: before you look anything up, list every lookup the next step needs and fetch them all in ONE call (several ${LOOK} specs, or \`;\`-chained commands). Two single-lookup calls in a row is a mistake; so is a lookup the lane brief already answers.
  * Other: structural -> ast-grep; unknown-location behavior in the server -> \`cd ${SERVER} && grepai search "<behavior>" --toon --compact --limit 5\` (its index reflects the server's main). On a grepai error fall back to rg + gopls without retrying.
- Issues: issue_read get (and get_comments) once per agent; keep what you need in your notes/worklog. The issue's milestone epic and its docs/plan.md row are the scope.
- Editing: the Edit tool needs a Read of that file first (a sed/cat/look view does not count): Read the range, then Edit. For several mechanical edits at once a python heredoc in one call is fine.
- Tests and lint ALWAYS go through ${HEAVY} (it serializes heavy runs on this small box; waiting for its lock is normal). Always -timeout 12m:
  ${HEAVY} rtk go -C <worktree> test -count=1 -timeout 12m <pkgs> -run '<tests>'
  cd <worktree> && golangci-lint fmt --diff && ${HEAVY} golangci-lint run <touched package dirs>
  A hang is almost always a real deadlock in the code under test: read the goroutine dump.
- E2E stack: one shared stack per machine (docs/agents/test-strategy.md); if it is down, start it as that guide says; never bring up a second one.
- Waiting: never use sleep loops; wait only through the ${MB} commands named in your instructions, each in one Bash call with timeout 300000, and never with a --wait above ${SHORT_WAIT}: your prompt cache lives about 5 minutes, and waking after a longer wait re-writes your whole context at about 12x the price of reading it (#262). Calling again every ${SHORT_WAIT} s is the cheap way to wait.
- Long commands: anything you expect to take longer than about 4 minutes (a live e2e scenario, a long run behind the heavy lock) runs in the background (Bash run_in_background) with its output in a log file; wait on the log in calls of at most ${SHORT_WAIT} s, for example \`timeout ${SHORT_WAIT} tail -n +1 -f <log> | grep -m1 -E '^(ok|FAIL)\\s|^\\s*--- (FAIL|PASS|SKIP)'\`, and call again until the run ends. Never wait on one call for longer than ${SHORT_WAIT} s.
- Follow-up issues in ${GH_REPO} go to the milestone (M0-M5) that owns the work; dedupe with search_issues first and amend a matching issue instead of duplicating.
- Server gaps (docs/plan.md "Gap handling"): when the bot hits server behavior that is not ported yet, the bot logs the gap and degrades; file or amend the issue in ${SERVER_REPO} (search it first) and link it from the PR. Never special-case the server for bots, and never edit the server from a lane.
- Where an issue offers several options, choose the one matching docs/plan.md and its owner decisions, else the conservative one, stated in a "Decisions" section of the PR body.
- Merged PRs close their issues: one "Closes #n" line per fully resolved issue, "Part of #n" for partial ones. The PR body ends with the gap audit line CLAUDE.md requires.
- Commit messages end with a blank line then the Co-Authored-By trailer your lane instructions name (it names the model you run on).
- PR bodies end with: 🤖 Generated with [Claude Code](https://claude.com/claude-code)
- Never put model names or IDs in commit messages, PR text or code, except the Co-Authored-By trailer named above.
- Put new tests in new or small test files to avoid conflicts between lanes.
${PLAN.exclusions ? '- Never touch (other sessions / owner decisions): ' + PLAN.exclusions + '\n' : ''}${PLAN.env ? PLAN.env + '\n' : ''}- Other lanes run concurrently or later; stay within your lane and do not implement what other lanes own: ${ALL_LANES.map(l => l.slug + '=' + l.issues.join('/')).join('; ')}.
`
const issuesText = L => L.issues.map(i => '#' + i).join(', ')

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------
const IMPL_SCHEMA = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['merged', 'no_change', 'blocked', 'unresolved', 'stalled', 'merge_blocked', 'failed', 'stopped', 'idle_exit', 'checkpoint'] },
    pr_number: { type: 'integer' },
    closes: { type: 'array', items: { type: 'integer' } },
    summary: { type: 'string' },
    followups_filed: { type: 'array', items: { type: 'string' } },
  },
  required: ['status', 'summary'],
}
const REVIEWER_SCHEMA = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['merged', 'unresolved', 'stalled', 'merge_blocked', 'failed', 'stopped', 'idle_exit', 'no_change', 'blocked'] },
    pr_number: { type: 'integer' },
    rounds: { type: 'integer' },
    merge_sha: { type: 'string' },
    summary: { type: 'string', description: 'one line per round: verdict and item count; merge attempts and outcomes' },
  },
  required: ['status', 'summary'],
}
const WAIT_SCHEMA = {
  type: 'object',
  properties: {
    messages: { type: 'array', items: { type: 'object', properties: { seq: { type: 'integer' }, kind: { type: 'string' }, data: { type: 'string', description: 'the message data object, as a JSON string' } }, required: ['seq', 'kind', 'data'] } },
    timeout: { type: 'boolean' },
    stop: { type: 'boolean' },
  },
  required: ['messages', 'timeout'],
}
const MERGE_SCHEMA = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['merged', 'conflict', 'ci_failed', 'head_moved', 'merge_blocked', 'retry'] },
    answered: { type: 'boolean', description: 'true once the answer file was written' },
    merge_sha: { type: 'string' },
    notes: { type: 'string' },
  },
  required: ['status', 'answered', 'notes'],
}
const LINES_SCHEMA = {
  type: 'object',
  properties: { passed: { type: 'boolean' }, stop: { type: 'boolean' }, lines: { type: 'array', items: { type: 'string' } } },
  required: ['lines'],
}
const TAKE_SCHEMA = {
  type: 'object',
  properties: {
    events: { type: 'array', items: { type: 'object', properties: { id: { type: 'string' }, type: { type: 'string' }, from: { type: 'string' } }, required: ['id', 'type', 'from'] } },
    timeout: { type: 'boolean' },
    stop: { type: 'boolean' },
  },
  required: ['events', 'stop'],
}

// ---------------------------------------------------------------------------
// Prompts: the two long-lived agents of a lane
// ---------------------------------------------------------------------------
function implPrompt(L, resume, brief, canCheckpoint) {
  const D = LANE_DIR(L.slug)
  const checkpointRules = canCheckpoint
    ? `CHECKPOINTS (your cost is turns x context, so hand over instead of growing):
- Right after you send the first ready (reason initial): bring ${D}/worklog.md fully up to date (state, PR number, decisions and why, files changed, the exact test commands, open questions) and return status checkpoint. A fresh agent handles the review, CI and conflict rounds from the worklog.
- Any other time your conversation has grown long (about 60 tool calls since you started, or a context warning) and you are at a clean point (edits saved, worklog listing the exact remaining steps), or you are about to wait on recv after a long round: update the worklog and return status checkpoint. Never checkpoint in the middle of an edit, with uncommitted work the worklog does not describe, or with a message read but not handled.`
    : `CHECKPOINTS: none are left for this lane. Never return status checkpoint; keep working until the final message or another return the protocol names.`
  return `${TESTRULE}
You are the long-lived implementation agent for lane "${L.slug}" (wave ${L.wave}, track ${TRACK}) of ${GH_REPO}: issues ${issuesText(L)}${L.milestone ? ' (' + L.milestone + ')' : ''}.
${L.note || ''}
You own this PR's code: you implement it and fix every review round, CI failure and merge conflict, handing over to a fresh implementation agent at the CHECKPOINTS below so no agent carries a huge context. One long-lived review agent reviews every push and routes merge results back through the lane mailbox. You never merge.
${brief ? `\nLANE BRIEF (a scout prepared it from origin/main; it is also in ${D}/brief.md). Rely on it instead of repeating its searches; verify only what you change or what looks wrong:\n${brief}\n` : resume ? `\nThe lane brief is in ${D}/brief.md: Read it once if the lane is still at stage impl; review, CI and conflict rounds rarely need it.\n` : ''}

PHASE 1 — implement. Load and follow ${skillRef('bot-issue')}: ${brief
    ? 'the LANE BRIEF above already holds the issue, its plan rows, the server references and the Go anchors, so start from it: read live only what it lacks or marks open (issue comments newer than the brief included), and research only the server behavior it does not cover'
    : 'read each issue live (issue_read get and get_comments), verify the work is still open on origin/main, read its docs/plan.md rows and research the server behavior as the skill prescribes'} (through the helpers below where they fit), implement the smallest complete change for this lane with TDD (table-driven unit tests for codecs, crypt and brain decisions with an injected clock; e2e scenarios for flows against the server, per docs/agents/test-strategy.md), run the scoped gates, then open ONE PR, ready for review (not draft), with "Closes #n" per fully resolved issue, the gap audit and the evidence the skill requires.
- Worktree: ${MB} worktree --run ${RUN} --lane ${L.slug} (one call: creates or reuses ${WT}/wf-${L.slug} on branch wf/${L.slug} from origin/main, records it in the lane state, and prints head, dirty files and how far it is behind main). Push with git -C ${WT}/wf-${L.slug} push -u origin wf/${L.slug}.
${(L.deps || []).length ? `- This lane builds on lane(s) ${L.deps.join(', ')}, already merged to origin/main: read their merged PRs and reuse their primitives.` : ''}
- An issue already fixed on main or stale: no code for it; comment with evidence (main SHA, file:line, merging PR), close it, leave it out of "Closes".
- Live e2e debug loop (docs/agents/test-strategy.md, "Live debug loop"): run every scenario still failing in a round in ONE invocation, never one after another; give a parallel live run as many GMs as GM scenarios it runs at once, or run them one at a time (-parallel 1 with one pinned GM: a scenario left without a GM fails with "GM pool exhausted"); reproduce a live failure from its recorded frames (BOT_E2E_RECORD) in a unit test before rerunning live, and keep the test; seed the smallest setup that triggers the behavior; give the step under test a refusal check (untilServed with a RefusalCheck, docs/e2e-live.md "Refusal checks") so a refused request fails at once.
- Keep ${D}/worklog.md current: decisions and why, server evidence paths, what each round changed, open questions. A replacement agent resumes from it if you stop.
- STOP: once the run is stopping, every ${MB} call prints "stop": true, ${LOOK} prints "### STOP", and heavy prints "### STOP" and refuses to run. When you see any of them: update the worklog (where you are, what is left), push nothing, and return status stopped. Never spend a call of its own checking for it.

MAILBOX (each command in its own Bash call, timeout 300000):
- To the review agent: ${MB} send --run ${RUN} --lane ${L.slug} --box review --kind <kind> --data '<json object>' [--set key=value ...]
  * ready {"reason":"initial|review-fix|ci-fix|conflict","pr":<n>,"head":"<pushed sha>","semantic":<bool>} after every push that should move on; add --set stage=review --set pr=<n> --set head=<sha>.
  * no_change {"summary":"..."} with --set status=no_change, or blocked {"reason":"..."} with --set status=blocked (after commenting the blocker on the issue): only before a PR exists; then return at once.
- Wait for the review agent: ${MB} recv --run ${RUN} --lane ${L.slug} --box impl --wait ${SHORT_WAIT} ; run it again while it prints "timeout": true. Before working on a review, ci or conflict message, run git -C ${WT}/wf-${L.slug} pull --no-rebase origin wf/${L.slug} : the merge agent may have pushed a branch-update merge commit, and your push must stay a fast-forward (not for final: a merged branch is already deleted). Messages:
  * review {"round":N,"file":"..."}: read the file (and the PR's unresolved threads), then follow ${skillRef('bot-pr-review-comments')}: verify each item against the code, docs/plan.md and the server reference, fix every confirmed in-scope finding with tests, reply on every thread with evidence (fixed-in SHA, or rebuttal with code/server evidence), resolve fixed threads, file follow-ups for valid out-of-scope items, push (never force-push), send ready reason review-fix. You already know the code: re-read only what the findings point at.
  * ci {"file":"...","head":"..."}: CI failed for a real reason; read the file, fix, scoped-verify, push, send ready reason ci-fix.
  * conflict {"notes":"..."}: the branch cannot be updated automatically. git fetch origin; merge origin/main into your branch (never rebase, never force-push); resolve; fmt, build, vet and scoped tests for what the resolution touched; push; send ready reason conflict with "semantic": true when the resolution changed the PR's logic (any change to what the bot sends, when, or how it decodes is semantic), else false.
  * final {"reason":"merged|unresolved|stalled|merge_blocked|failed|stopped"}: if merged, run ${MB} worktree --run ${RUN} --lane ${L.slug} --remove (worktree and local branch); otherwise keep both. Update the worklog and return your result with status = the final reason.
  * A message identical to one you already handled (same kind and round/file): ignore it.
  * recv printing "stop": true: the run is stopping; update the worklog and return status stopped.
- If recv times out for ${MAX_IDLE_MIN} consecutive minutes (since the last message you received), update the worklog and return status idle_exit.
${checkpointRules}
- Skills: Read each SKILL.md once per agent and keep what you need in your notes; never re-read or \`cat\` it again.
${HELPERS(L.slug, 'impl')}
${ENV}- Your commit trailer: ${coAuthor(L.implModel || M.impl)}
${resume ? '\nRESUME: ' + resume : ''}
Return: status, pr_number, closes, summary (what changed, key files, decisions), followups_filed.`
}

function implResume(L, why, restart) {
  const D = LANE_DIR(L.slug)
  // After a run restart (resume or retry) a final left by the previous launch would end this agent at
  // its first recv. Only then: within a launch, a queued final is meant for this lane's agent.
  const drain = restart ? ` Then discard finals a previous launch left queued: ${MB} drain --run ${RUN} --lane ${L.slug} --box impl --kinds final .` : ''
  return `${why} First run ${MB} state --run ${RUN} --lane ${L.slug} --set status=active ; the state it prints (stage, pr, round) is authoritative.${drain} Rebuild your context from ${D}/worklog.md, the PR if there is one (pull_request_read get and its unresolved review threads; for the code, \`git -C ${WT}/wf-${L.slug} diff --stat origin/main...HEAD\`, then only the hunks a review item or the worklog points at; never get_diff of the whole PR, which carries generated files), ${D}/reviews/ and ${D}/ci/. Reuse worktree ${WT}/wf-${L.slug} (recreate it from origin/wf/${L.slug} if missing). Then by stage:
- impl: finish the implementation and continue with the protocol.
- review or merge: the review agent is working; go straight to recv.
- fix: the newest review in ${D}/reviews/ may be partly addressed; compare what is pushed with what was asked, finish it, push, send ready reason review-fix.
- ci-fix or conflict: the triggering message may already be consumed; read the newest file in ${D}/ci/ or merge origin/main as the protocol says, fix, push, send ready with the matching reason.`
}

function briefPrompt(L) {
  const D = LANE_DIR(L.slug)
  return `You prepare the LANE BRIEF for lane "${L.slug}" of ${GH_REPO} (issues ${issuesText(L)}${L.milestone ? ', ' + L.milestone : ''}). An expensive implementation agent starts right after you with your brief in its prompt; every fact you hand it saves it a lookup. Gather facts; do not design the change, do not write code, do not run tests.
Lane note: ${L.note || '(none)'}
Steps (batch: several lookups per Bash call):
1. ${MB} worktree --run ${RUN} --lane ${L.slug}   (creates ${WT}/wf-${L.slug} on origin/main; all repo paths below are relative to it).
2. Issues: gh issue view <n> -R ${GH_REPO} --comments (timeout 300000) for each lane issue and its milestone epic. Check every blocker the issue or note names with gh issue view <b> -R <its repo> --json state,title; server prerequisites live in ${SERVER_REPO}.
3. Plan: the docs/plan.md rows that bind this lane, with line numbers: its PR row in "PR breakdown" (scope and evidence), the server and client packets its PR reads or sends, the owner decisions (D1-D8) and architecture rules it touches.
4. Server reference (${SERVER}, read-only): for every packet the lane decodes or encodes, the server's encoder or decoder (file:line), opcode constant and field order; for every client action, the handler and its rejection paths and limits (flood gates, distance checks, timeouts) the bot must respect. Paths with line numbers.
5. Go anchors (worktree-relative, with line numbers): the packages and files to change, the closest existing pattern to copy, existing tests and fixtures to extend, the newest merged PR touching the same area and its file list.
6. Blockers: a dependency lane or issue not merged, or a server prerequisite still open that the lane cannot work around, goes first, under BLOCKER, with evidence.
7. Commands: the scoped test commands (${HEAVY} rtk go -C ${WT}/wf-${L.slug} test -count=1 -timeout 12m <pkgs> -run '<tests>') and the related e2e scenarios.
Write the brief as markdown, at most 250 lines: sections BLOCKER (only if any), Issues (requirements, acceptance, decisions already made in comments), Plan, Server reference, Go anchors, Tests and commands, Open questions. Paths with line numbers; short excerpts (at most 15 lines) only where the exact text matters. Save it with one Bash call: cat > ${D}/brief.md <<'EOF_BRIEF' ... EOF_BRIEF (mkdir -p ${D} first). Your final message is the brief itself, verbatim.
You have no GitHub MCP tools and no ToolSearch: the gh commands in the steps above override the GitHub rule below.\nSTOP: once the run is stopping (or draining), ${MB} calls print "stop": true and ${LOOK} prints "### STOP". When you see either, do nothing more and make your final message exactly STOP.
${ENV}`
}

function reviewerPrompt(L, resume, roundLimit) {
  const D = LANE_DIR(L.slug)
  const RW = `${WT}/review-${L.slug}`
  return `${TESTRULE}
You are the long-lived review agent for lane "${L.slug}" (issues ${issuesText(L)}) of ${GH_REPO}. You stay alive for the PR's whole life: you review the first push in full, then wait for the implementation agent's fixes and re-review ONLY what changed, request the merge when the PR is clean, and route CI failures and conflicts back to the implementation agent. You keep your context between rounds: never re-read a file, issue, thread or server source you already read unless the new delta touches it.
${L.note || ''}
Review worktree: ${RW}, detached. At the start of every round run ${MB} worktree --run ${RUN} --lane ${L.slug} --review in a Bash call of its own (creates it the first time, then moves it to the current origin/wf/${L.slug}, and prints the head). Remove it when you return: the same command with --remove.
Cost rules: Read the review skill's SKILL.md ONCE (round 1) with the Read tool and keep its checklist in your notes; never re-read or \`cat\` it again, and never chain it to another command. Review from the diff: \`git -C ${RW} diff --stat <base>..<head>\`, then the hunks of the files you review, then surrounding code in ranges of at most 80 lines; server code through ${LOOK} go:${SERVER}/<pkg>:<Name>. Do not print whole files.

MAILBOX (each command in its own Bash call, timeout 300000):
- Wait for the implementation agent: ${MB} recv --run ${RUN} --lane ${L.slug} --box review --wait ${SHORT_WAIT} ; run it again while it prints "timeout": true. You get ready {"reason","pr","head","semantic"}, or rarely no_change/blocked (then return that status).
- To the implementation agent: ${MB} send --run ${RUN} --lane ${L.slug} --box impl --kind <review|ci|conflict|final> --data '<json object>' [--set key=value ...]
- Merge requests go to the merge agent (cheap model) through the merger broker:
  ${MB} ask --run ${RUN} --from ${L.slug}/review --type merge --question '{"pr":<n>,"head":"<sha>"}' --wait ${SHORT_WAIT}
  The merge takes a while (CI): while it prints "pending": true, run ${MB} answer-wait --run ${RUN} --id <id> --wait ${SHORT_WAIT} again, always with the same id (a pending request is still being served; never ask again while one is pending). The answer is a JSON object {"status":"merged|conflict|ci_failed|head_moved|merge_blocked|retry","merge_sha","file","head","notes"}. "stop": true: return status stopped. An "error" (the merge broker is not running): ${MB} pause --secs 300, then ask again; after 6 such errors, send final {"reason":"failed"} --set status=failed and return status failed.

LOOP:
1. Round 1 (the first ready): load ${skillRef('bot-pr-review')} and follow it end to end: review against the server reference, docs/plan.md (the PR row, owner decisions, architecture rules) and the acceptance criteria of every issue the PR closes or is part of; run the PR's new/changed tests in the review worktree; submit one GitHub review with inline comments; decide the Bottom line. Only [blocker]/[must-fix]/[should-fix]/non-optional [nit] items count as action required.
   E2E coverage is part of every round: PR CI does not gate the e2e scenarios, so check that the PR's "E2E scenarios" line names the related ones (TEST SCOPE above) and run them yourself at the head you review when the stack is reachable (BOT_REQUIRE_E2E=1, scoped). A related scenario that is missing or failing is a [must-fix]; one you could not run is named as not run in Checks. Never run the full suite: CI does.
2. Every round N: write ${D}/reviews/r<N>.md (verdict, the numbered action list '[tag] path:line — what to do' or "none", and for N > 1 the status of each earlier item), then ${MB} state --run ${RUN} --lane ${L.slug} --set round=<N> .
   - Action required and N < ${roundLimit}: send review {"round":N,"file":"${D}/reviews/r<N>.md"} --set stage=fix, then wait.
   - Action required and N >= ${roundLimit}: send final {"reason":"unresolved"} --set status=unresolved and return status unresolved.
   - Clean: go to step 4.
3. On ready (round N+1), by reason:
   - review-fix${REVIEW_CI_FIXES ? ', ci-fix' : ''}, or conflict with "semantic": true: a DELTA re-review. Diff your last reviewed head to the new head in the review worktree; verify each earlier item is fixed; review the delta in full and other code only where the delta reaches. Read the author's thread replies; do not re-raise a finding rebutted with concrete code/server evidence unless you can show the rebuttal wrong with better evidence, and do not add nits you could have raised earlier unless they are real defects. Submit the re-review on GitHub, then step 2.
   - ${REVIEW_CI_FIXES ? '' : 'ci-fix, or '}conflict with "semantic": false: no review; go to step 4.
4. Merge: ${MB} state --run ${RUN} --lane ${L.slug} --set stage=merge , then ask for the merge with head = the head of the latest ready you accepted (the head you reviewed, or for a ready you took to merge without review, the head it names), and act on the answer:
   - merged: send final {"reason":"merged","merge_sha":"<sha>"} --set status=merged; return status merged.
   - ci_failed: send ci {"file":"<file>","head":"<head>"} --set stage=ci-fix; wait (step 3).
   - conflict: send conflict {"notes":"<notes>"} --set stage=conflict; wait (step 3).
   - head_moved: the branch changed without a ready (an unexpected push). Fetch it and do a delta re-review of the new head as the next round (step 3 rules, then step 2).
   - merge_blocked: send final {"reason":"merge_blocked"} --set status=merge_blocked; return status merge_blocked.
   - retry: ask again; after 6 merge requests in total, send final {"reason":"failed"} --set status=failed and return status failed.
- If recv times out for ${MAX_IDLE_MIN} consecutive minutes (since the last message you received): send final {"reason":"stalled"} --set status=stalled; return status stalled.
- recv or answer-wait printing "stop": true: return status stopped (no final message).
${HELPERS(L.slug, 'review')}
${ENV}
${resume ? '\nRESUME: ' + resume : ''}
Return: status, pr_number, rounds, merge_sha, summary.`
}

function reviewerResume(L, why) {
  const D = LANE_DIR(L.slug)
  return `${why} You have no memory of earlier rounds: rebuild it once. First run ${MB} state --run ${RUN} --lane ${L.slug} ; the state it prints (stage, pr, round) is authoritative. Read every review in ${D}/reviews/ and the PR (pull_request_read get, get_reviews, get_review_comments). Your next round number is the highest r<N>.md number + 1, and the last reviewed head is the one in the newest review file. Then by stage:
- review (or impl with a PR open): a ready message may already be consumed. If the PR head differs from your last reviewed head (or there is no review yet), review it now (round 1 in full if there is none, otherwise a delta re-review); else wait.
- fix, ci-fix or conflict: wait for the implementation agent's ready.
- merge: check the PR first. MERGED: send final {"reason":"merged","merge_sha":"<sha>"} --set status=merged and return status merged. Otherwise look for an earlier merge request of this lane still pending (ls ${RUNDIR}/requests/taken/ | rg '^${L.slug}-review-' , newest first, with no matching ${RUNDIR}/answers/<id>.md): if one exists, keep waiting on it with answer-wait; else ask for the merge again.`
}

// ---------------------------------------------------------------------------
// Prompts: small coordination agents
// ---------------------------------------------------------------------------
function watchPrompt(L, drain) {
  return `You are a mailbox watcher. Run only ${drain ? 'these commands, each' : 'this command'} in one Bash call with timeout 600000, copied exactly; do nothing else:
${drain ? `1. ${MB} drain --run ${RUN} --lane ${L.slug} --box review --kinds no_change,blocked   (discards answers left from an earlier run)
2. ` : ''}${MB} recv --run ${RUN} --lane ${L.slug} --box review --peek --wait 570
While its output has "timeout": true and no "stop", run it again: at most ${WAIT_CALLS} recv runs in total.
Return the last output: messages (seq, kind, and data re-encoded as a JSON string), timeout, stop.`
}

function deliverFinalPrompt(L, data) {
  return `Run only this command in one Bash call with timeout 600000, copied exactly, and do nothing else; then return messages [], timeout true:
${MB} send --run ${RUN} --lane ${L.slug} --box impl --kind final --data '${sq(data)}' --set status=${data.reason} --unless-ended`
}

function statusPrompt() {
  return `Run exactly this Bash command (timeout 600000) and return its "lines" array verbatim: ${MB} status --run ${RUN}`
}

function barrierPrompt(wi, finish) {
  return `You are a wave barrier. Run only these commands, each in its own Bash call with timeout 600000, copied exactly:
1. ${MB} barrier --run ${RUN} --epoch ${EPOCH} --wave ${wi} --track ${TRACK} --tracks ${TRACKS.join(',')} --wait 570
   While it prints "passed": false without "stop", run it again: at most 30 runs.
${finish ? `2. If it passed: ${MB} finish-track --run ${RUN} --epoch ${EPOCH} --track ${TRACK}\n` : ''}Return passed, stop and lines from the last barrier output (lines verbatim; empty if absent).`
}

// One take agent polls up to TAKE_CALLS times (~9.5 min each) before it returns, so an idle
// broker pays one agent's context per ~TAKE_CALLS × 9.5 min instead of one per poll.
function takePrompt(broker, first) {
  const cmd = `${MB} take --run ${RUN} --broker ${broker} --epoch ${EPOCH} --tracks ${TRACKS.join(',')} --wait 570`
  return `You are a request poller. Run only this Bash command (timeout 600000), copied exactly:
${cmd}${first ? '\nOn the FIRST run only, append " --requeue" to it.' : ''}
While it prints "events": [] with "timeout": true and no "stop": true, run it again: at most ${TAKE_CALLS} runs in total. Do nothing else between runs.
As soon as a run prints a non-empty "events" list, stop polling and return at once: never run the command again after that, because each run removes the events it prints from the queue and a dropped event is lost.
Return the events (id, type, from only), timeout and stop fields of the last run.`
}

function helperPrompt(ev) {
  const file = `${RUNDIR}/requests/taken/${ev.id}.json`
  const body = {
    'go-scout': `Locate the Go code the question asks about, in the worktree it names or in the server checkout ${SERVER} (read-only). Report file:line locations with one line each on what is there; no file dumps.`,
    'researcher': `Investigate the question read-only, in the worktree it names and/or the server checkout ${SERVER}; batch everything it asks into one report with file:line evidence for each claim.`,
    'runner': `Run the commands the question lists, in the worktree it names; tests and lint always through ${HEAVY} with -timeout 12m. Refuse a full-suite command (go test ./..., ./tests/..., ./internal/..., make test): answer that CI runs the full suite and ask for scoped packages and -run patterns. Do not edit, commit or push anything. Report per command: pass/fail, the failing test names, and the first error lines with file:line (at most 60 lines in total).`,
  }[ev.type]
  return `Helper request ${ev.id} from ${ev.from}. Read the request file ${file} with the Read tool; its "question" field is your task and its "worktree" field is the requester's worktree. ${body}
Your final message is delivered verbatim to the requester: lead with the answer, cite file:line evidence, say what is confirmed versus inferred.`
}

function deliverPrompt(id, text) {
  const path = `${RUNDIR}/answers/${id}.md`
  return `Deliver a helper answer. Use the Write tool to create ${path}.tmp with exactly the text between the markers (not the markers), then run (Bash) mv ${path}.tmp ${path} . Do nothing else.
<<<ANSWER
${text}
ANSWER>>>`
}

function mergePrompt(ev, L) {
  const file = `${RUNDIR}/requests/taken/${ev.id}.json`
  const slug = L.slug
  const D = LANE_DIR(slug)
  return `You are the CI and merge agent for lane "${slug}" (branch wf/${slug}, issues ${issuesText(L)}) in ${GH_REPO}. Read the request file ${file} with the Read tool: its "question" field is a JSON object {"pr": <number>, "head": "<sha the reviewer approved>"}. The PR passed review with no action required, and the user explicitly authorized merging PRs that passed this review loop. Be mechanical: you never edit code, never resolve conflicts and never run tests locally (CI runs the unit suite; the implementation and review agents already ran the e2e scenarios CI does not gate).
Every Bash call to ${MB} uses timeout 600000. You answer exactly once, at the end, with:
  ${MB} answer --run ${RUN} --id ${ev.id} --text '<json object>'
where the object is {"status":"merged|conflict|ci_failed|head_moved|merge_blocked|retry","merge_sha":"","file":"","head":"","notes":""} (no single quotes inside).
PR state and CI come from two compact commands (use them instead of pull_request_read polling):
  ${MB} pr-state --pr <n>   -> {state, merged, merge_sha, draft, head, mergeable_state, behind_by}
  ${MB} ci-wait --run ${RUN} --broker merge --head <sha> --wait 570   -> waits in-process; {state: success|failure|pending|error, failed: [{name, conclusion, run_id, job_id}], pending, missing}
1. Head check: ${MB} pr-state --pr <n>. If the PR is already MERGED (an earlier merge agent merged it, then stopped before answering), do step 5's checks and answer merged with its merge SHA. If the PR head is the requested head, continue. Otherwise git -C ${GO} fetch origin wf/${slug} main and list the extra commits: git -C ${GO} log --format='%H %P %s' <requested head>..origin/wf/${slug} . Continue only if the requested head is an ancestor (git -C ${GO} merge-base --is-ancestor <requested head> origin/wf/${slug}) and every listed commit is a merge commit (two parent hashes) whose subject starts with "Merge branch 'main'": a branch update an earlier merge attempt made. Anything else is a push the reviewer has not seen: answer head_moved with head = the PR head.
2. Update: only when pr-state shows behind_by > 0: mcp__github__update_pull_request_branch (owner ${GH_OWNER}, repo ${GH_NAME}), then pr-state again for the new head. A merge conflict: answer conflict with the conflicting files in notes. behind_by 0: continue.
3. CI: ${MB} ci-wait --run ${RUN} --broker merge --head <head> --wait 570 ; while it prints state pending, run it again (at most 10 runs, about 90 minutes; longer: answer retry). It returns success only when every check run on the head finished green and the required checks are present (a code-scanning check alone is not enough). state error: run it once more, then fall back to pull_request_read get_check_runs.
   On a failure: list the failing jobs and read their logs (gh run view <run-id> --log-failed | tail -n 200). It is a flake only if the failing test has an open flaky-test issue (search_issues "repo:${GH_REPO} Flaky <TestName>") or it fails the same way on the latest main CI run; then rerun the failed jobs once (gh run rerun <run-id> --failed), comment the test name on that issue (or file one titled "Flaky <TestName>" in the milestone of this lane's issue), and go back to waiting. Otherwise write ${D}/ci/c<next number>.md (create the directory if needed) with the head sha, each failing check, the failing test names and about 40 log lines per failure, and answer ci_failed with file = that path and head = the sha.
   Checks that never start or fail for GitHub Actions infrastructure reasons (cancelled, queued > 30 min, runner errors) twice in a row: answer retry with notes "ci outage".
4. Merge: never edit the PR title or body (its Closes/Part of lines are the implementation agent's decision). If the PR is a draft, mark it ready (update_pull_request draft=false). mcp__github__merge_pull_request (merge_method squash, expectedHeadSha = the head verified green). "Head is not up to date" or "head changed": back to step 2 (at most 3 times). If the permission system denies the merge, do not try another route: answer merge_blocked.
5. After the merge: git -C ${GO} push origin --delete wf/${slug}; confirm every "Closes" issue is closed (close any that did not auto-close, commenting the merge SHA); ${MB} state --run ${RUN} --lane ${slug} --set status=merged --set merge_sha=<sha> ; answer merged with merge_sha.
GitHub: use the mcp__github__* tools (load with ToolSearch "select:mcp__github__pull_request_read,mcp__github__update_pull_request_branch,mcp__github__merge_pull_request,mcp__github__update_pull_request,mcp__github__issue_read,mcp__github__issue_write,mcp__github__add_issue_comment,mcp__github__search_issues"); gh only for the run logs and reruns above, with a 300000 ms timeout.
Return: status, answered (true once the answer command printed "written": true), merge_sha, notes.`
}

// ---------------------------------------------------------------------------
// Concurrency helpers
// ---------------------------------------------------------------------------
function sem(n) {
  let active = 0
  const q = []
  return async fn => {
    if (active < n) active++
    else await new Promise(r => q.push(r))
    try { return await fn() } finally {
      const nx = q.shift()
      if (nx) nx(); else active--
    }
  }
}
const hotLocks = {}
for (const L of ALL_LANES) for (const h of (L.hot || [])) if (!hotLocks[h]) hotLocks[h] = sem(1)
function withHot(groups, fn) {
  const gs = [...groups].sort()
  const step = i => i >= gs.length ? fn() : hotLocks[gs[i]](() => step(i + 1))
  return step(0)
}
function parseLines(lines) {
  const st = {}
  for (const ln of (lines || [])) {
    const [slug, status, stage, pr, round] = String(ln).split('|')
    if (slug) st[slug] = { status: status || 'active', stage: stage || '', pr: pr ? Number(pr) : undefined, round: round ? Number(round) : 0 }
  }
  return st
}
function parseData(s) {
  if (s && typeof s === 'object') return s
  try { return JSON.parse(s || '{}') } catch (e) { return {} }
}

// ---------------------------------------------------------------------------
// Track role: one lane = one long-lived implementation agent + one long-lived review agent
// ---------------------------------------------------------------------------
async function lifecycle(L, prev, retried) {
  const slug = L.slug
  const history = []
  const resuming = prev && !DONE_BEFORE.includes(prev.status) && prev.stage ? prev : null
  // A retried lane gets a fresh round budget on top of the rounds it already used.
  const roundLimit = (retried && prev && prev.round ? prev.round : 0) + MAX_ROUNDS
  const runs = { impl: 0, review: 0 }
  const fails = { impl: 0, review: 0 } // early exits that are not checkpoints
  let checkpoints = 0
  let brief = null
  // Set once the PR exists; agents started after it get a pointer to the brief, not the brief.
  let pushed = !!(resuming && resuming.stage !== 'impl')
  const live = { impl: null, review: null }
  const done = { impl: null, review: null }
  const seen = { impl: 0, review: 0 } // the run number whose exit the loop below has handled

  const start = (role, resume) => {
    const n = ++runs[role]
    done[role] = null
    const model = role === 'impl' ? (L.implModel || M.impl) : (L.reviewModel || M.review)
    const opts = role === 'impl'
      ? { label: `impl:${slug}${n > 1 ? '#' + n : ''}`, phase: 'Implement', schema: IMPL_SCHEMA }
      : { label: `review:${slug}${n > 1 ? '#' + n : ''}`, phase: 'Review', schema: REVIEWER_SCHEMA }
    if (model) opts.model = model
    const effort = role === 'impl' ? L.implEffort : L.reviewEffort
    if (effort) opts.effort = effort
    const prompt = role === 'impl' ? implPrompt(L, resume, pushed ? null : brief, checkpoints < MAX_CHECKPOINTS) : reviewerPrompt(L, resume, roundLimit)
    live[role] = agentAs(role, prompt, opts).then(r => {
      if (r && r.pr_number) pushed = true
      done[role] = r || { status: 'died', summary: `${role} agent died` }
      return { role, n, r: done[role] }
    })
  }
  const why = (role, r) => `The previous ${role === 'impl' ? 'implementation' : 'review'} agent of this lane exited early (${r.status}: ${(r.summary || '').slice(0, 300)}).`
  const sendFinal = async data => {
    await agentAs('coord', deliverFinalPrompt(L, data), { label: `final:${slug}`, phase: 'Coordinate', model: M.coord, effort: 'low' })
  }

  if (BRIEF && !resuming) {
    const b = await agentAs('brief', briefPrompt(L), { label: `brief:${slug}`, phase: 'Implement', model: M.brief })
    if (typeof b === 'string' && b.trim() === 'STOP') { STOPPING = true; return { slug, status: 'stopped', history } }
    if (typeof b === 'string' && b.trim()) brief = b.trim().slice(0, 24000)
    else if (b && typeof b === 'object' && b.brief) brief = String(b.brief).slice(0, 24000)
  }
  start('impl', resuming ? implResume(L, 'A previous implementation agent for this lane stopped (run restart).', true) : null)
  // A checkpoint hands the lane to a fresh implementation agent; anything else counts as an early exit.
  const checkpointed = r => r.status === 'checkpoint' && checkpoints < MAX_CHECKPOINTS && ++checkpoints
  const ckptWhy = 'The previous implementation agent of this lane checkpointed to keep its context small; you continue its work with an empty context.'

  // Until the first push only the implementation agent runs; a cheap watcher holds the second slot.
  let reviewing = pushed
  let watchFails = 0
  let firstWatch = true
  // An implementation agent's exit (a checkpoint above all) posts no mailbox message, so the watcher
  // races it instead of being waited out; a watcher still in flight is reused on the next pass.
  let watching = null
  const settle = async res => { if (watching) await watching; return res }
  while (!reviewing) {
    if (!watching) watching = agentAs('coord', watchPrompt(L, firstWatch), { label: `watch:${slug}`, phase: 'Coordinate', model: M.coord, effort: 'low', schema: WAIT_SCHEMA }).then(w => ({ w }))
    const ev = await Promise.race([watching, live.impl.then(() => null)])
    const w = ev && ev.w
    if (ev) watching = null
    if (w) firstWatch = false
    if (w && w.stop) { STOPPING = true; await live.impl; return { slug, status: 'stopped', history, impl: done.impl } }
    const msgs = w ? (w.messages || []).map(m => ({ kind: m.kind, data: parseData(m.data) })) : []
    const term = msgs.find(m => m.kind === 'no_change' || m.kind === 'blocked')
    if (term) {
      await live.impl
      log(`${slug}: ${term.kind} — ${(term.data.summary || term.data.reason || '').slice(0, 160)}`)
      return { slug, status: term.kind, history, impl: done.impl }
    }
    if (msgs.some(m => m.kind === 'ready')) { reviewing = pushed = true; break }
    if (done.impl) {
      seen.impl = runs.impl
      if (done.impl.status === 'stopped') STOPPING = true
      if (['no_change', 'blocked', 'stopped'].includes(done.impl.status)) return settle({ slug, status: done.impl.status, history, impl: done.impl })
      if (checkpointed(done.impl)) {
        history.push(`impl#${runs.impl} checkpoint`)
        start('impl', implResume(L, ckptWhy))
        continue
      }
      if (++fails.impl > RESTARTS) {
        // The last exit may follow a first push whose ready the watcher has not reported yet: then
        // the review side still finishes the lane, as it does for an exit after the push.
        const late = watching ? (await watching).w : null
        watching = null
        if (!(late && (late.messages || []).some(m => m.kind === 'ready'))) return { slug, status: 'failed', notes: 'implementation agent exited too often', history, impl: done.impl }
        history.push(`impl gave up after the first push: ${done.impl.status}`)
        reviewing = pushed = true
        break
      }
      history.push(`impl#${runs.impl} exited before the first push: ${done.impl.status}`)
      log(`${slug}: implementation agent exited (${done.impl.status}); starting a replacement`)
      start('impl', implResume(L, why('impl', done.impl)))
      continue
    }
    if (!w) {
      if (++watchFails > 3) {
        await sendFinal({ reason: 'failed' })
        await live.impl
        return { slug, status: 'failed', notes: 'mailbox watcher failed repeatedly', history, impl: done.impl }
      }
      continue
    }
    // A watcher timeout only means the implementation agent is still working before its first push.
    // There is no idle cutoff here: the agent's own run ends it, and a real early exit is handled above.
    watchFails = 0
  }

  log(`${slug}: first push ready; review agent starting`)
  start('review', resuming && resuming.stage !== 'impl' ? reviewerResume(L, 'A previous review agent for this lane stopped (run restart).') : null)

  for (let guard = 0; guard < 50; guard++) {
    // Handle an exit the loop has not seen yet before waiting on anything: an agent can finish
    // while the other one is still being started, or both in the same tick.
    const exitOf = role => (done[role] && seen[role] < runs[role]) ? { role, n: runs[role], r: done[role] } : null
    let ev = exitOf('review') || exitOf('impl')
    if (!ev) {
      const pending = ['impl', 'review'].filter(role => !done[role]).map(role => live[role])
      if (!pending.length) break
      ev = await Promise.race(pending)
    }
    const { role, r } = ev
    seen[role] = ev.n
    if (role === 'review') {
      if (FINAL.includes(r.status)) {
        history.push(`review: ${r.status} — ${(r.summary || '').slice(0, 300)}`)
        if (!done.impl) {
          if (r.status !== 'stopped') await sendFinal({ reason: r.status, merge_sha: r.merge_sha || '' })
          await live.impl
        }
        log(`${slug}: ${r.status}${r.pr_number ? ' (PR #' + r.pr_number + ')' : ''}${r.merge_sha ? ' ' + r.merge_sha : ''}`)
        return { slug, status: r.status, pr: r.pr_number, rounds: r.rounds, merge_sha: r.merge_sha, history, impl: done.impl, review: r }
      }
      const implGone = done.impl && seen.impl === runs.impl && (FINAL.includes(done.impl.status) || fails.impl > RESTARTS)
      if (++fails.review > RESTARTS || implGone) {
        if (!done.impl) {
          await sendFinal({ reason: 'failed' })
          await live.impl
        }
        return { slug, status: 'failed', notes: 'review agent exited too often', history, impl: done.impl, review: r }
      }
      history.push(`review#${runs.review} exited: ${r.status}`)
      log(`${slug}: review agent exited (${r.status}); starting a replacement`)
      start('review', reviewerResume(L, why('review', r)))
      continue
    }
    // The implementation agent returned.
    if (FINAL.includes(r.status)) continue // it got its final message; the review agent returns next
    if (checkpointed(r)) {
      history.push(`impl#${runs.impl} checkpoint`)
      start('impl', implResume(L, ckptWhy))
      continue
    }
    if (++fails.impl > RESTARTS) {
      history.push(`impl gave up: ${r.status}`)
      if (!done.review) continue // the review agent ends the lane (stalled at worst)
      return { slug, status: 'failed', notes: 'implementation agent exited too often', history, impl: r, review: done.review }
    }
    history.push(`impl#${runs.impl} exited: ${r.status}`)
    log(`${slug}: implementation agent exited (${r.status}); starting a replacement`)
    start('impl', implResume(L, why('impl', r)))
  }
  return { slug, status: (done.review && done.review.status) || 'failed', history, impl: done.impl, review: done.review }
}

async function trackMain() {
  phase('Coordinate')
  const st0 = await agentAs('coord', statusPrompt(), { label: 'status', phase: 'Coordinate', model: M.coord, effort: 'low', schema: LINES_SCHEMA })
  let STATE = parseLines(st0 && st0.lines)
  const laneSem = sem(LANES)
  const results = []
  const waves = PLAN.waves || []
  log(`track ${TRACK}: ${waves.length} waves, ${LANES} lane(s) at a time, ${Object.keys(STATE).length} lanes with saved state`)
  for (let wi = 0; wi < waves.length; wi++) {
    const mine = (waves[wi].lanes || []).filter(L => L.track === TRACK).map(L => Object.assign({ wave: waves[wi].name || String(wi + 1) }, L))
    const settled = {}
    for (const L of mine) { let res; settled[L.slug] = { p: new Promise(r => { res = r }), res: s => res(s) } }
    const runOne = async L => {
      const prev = STATE[L.slug]
      if (prev && DONE_BEFORE.includes(prev.status) && !RETRY.includes(L.slug)) return { slug: L.slug, status: prev.status, pr: prev.pr, resumed: true }
      const same = (L.deps || []).filter(d => settled[d])
      const sameSt = await Promise.all(same.map(d => settled[d].p))
      const bad = (L.deps || []).filter(d => {
        const s = settled[d] ? sameSt[same.indexOf(d)] : (STATE[d] && STATE[d].status)
        return !SATISFIED.includes(s)
      })
      if (bad.length) {
        log(`${L.slug}: skipped, deps not merged: ${bad.join(', ')}`)
        return { slug: L.slug, status: 'skipped', notes: 'deps not merged: ' + bad.join(', ') }
      }
      const retryPrev = prev && RETRY.includes(L.slug) ? Object.assign({}, prev, { status: 'active' }) : prev
      const notStarted = { slug: L.slug, status: 'stopped', notes: 'run stopping: lane not started' }
      if (STOPPING) return notStarted
      return withHot(L.hot || [], () => laneSem(() => STOPPING ? notStarted : lifecycle(L, retryPrev, RETRY.includes(L.slug))))
    }
    const rs = await parallel(mine.map(L => async () => {
      let r
      try { r = await runOne(L) } catch (e) { r = { slug: L.slug, status: 'failed', notes: String(e) } }
      if (r.status === 'stopped') STOPPING = true
      settled[L.slug].res(r.status)
      STATE[L.slug] = Object.assign({}, STATE[L.slug], { status: r.status, pr: r.pr })
      return r
    }))
    results.push(...rs.filter(Boolean))
    log(`track ${TRACK}: wave ${wi + 1} done (${rs.filter(r => r && r.status === 'merged').length}/${mine.length} merged); waiting for the other tracks`)
    let b = null
    for (let tries = 0; tries < 20; tries++) {
      b = await agentAs('coord', barrierPrompt(wi, wi === waves.length - 1), { label: `barrier:w${wi + 1}`, phase: 'Coordinate', model: M.coord, effort: 'low', schema: LINES_SCHEMA })
      if (b && (b.passed || b.stop)) break
    }
    if (!b || b.stop || !b.passed) { log(`track ${TRACK}: stopping after wave ${wi + 1} (${b && b.stop ? 'STOP' : 'barrier did not pass'})`); break }
    STATE = Object.assign(STATE, parseLines(b.lines))
  }
  return results.map(r => ({ slug: r.slug, status: r.status, pr: r.pr, rounds: r.rounds, resumed: r.resumed, merge_sha: r.merge_sha, notes: r.notes, history: r.history,
    summary: r.impl ? r.impl.summary : undefined, closes: r.impl ? r.impl.closes : undefined, followups: r.impl ? r.impl.followups_filed : undefined,
    review: r.review ? r.review.summary : undefined }))
}

// ---------------------------------------------------------------------------
// Merger role: one merge agent at a time, for every track
// ---------------------------------------------------------------------------
async function mergerMain() {
  phase('Merge')
  const handled = []
  let requeued = false
  log(`merger: serving merge requests for tracks ${TRACKS.join(',')}`)
  for (let i = 0; i < 900; i++) {
    const t = await agentAs('coord', takePrompt('merge', !requeued), { label: 'take', phase: 'Merge', model: M.coord, effort: 'low', schema: TAKE_SCHEMA })
    if (!t) continue
    requeued = true // only a take that actually ran has re-served the orphans
    if (t.stop) break
    for (const ev of (t.events || [])) {
      const slug = String(ev.from).split('/')[0]
      const L = LANE_BY[slug] || { slug, issues: [] }
      const m = await agentAs('merge', mergePrompt(ev, L), { label: `merge:${slug}`, phase: 'Merge', model: M.merge, schema: MERGE_SCHEMA })
      if (!m || !m.answered) {
        await agentAs('deliver', deliverPrompt(ev.id, JSON.stringify({ status: 'retry', notes: 'merge agent ' + (m ? 'did not answer' : 'failed') })), { label: `deliver:${ev.id}`, phase: 'Merge', model: M.deliver, effort: 'low' })
      }
      handled.push(`${slug}: ${m ? m.status : 'failed'}`)
      log(`merger: ${slug} ${m ? m.status : 'failed'}${m && m.merge_sha ? ' ' + m.merge_sha : ''}`)
    }
  }
  return { handled }
}

// ---------------------------------------------------------------------------
// Helper role: serve helper requests from every track
// ---------------------------------------------------------------------------
async function helperMain() {
  phase('Helpers')
  const pending = []
  const perLane = {}
  const served = []
  let requeued = false
  const deliver = (id, text) => agentAs('deliver', deliverPrompt(id, text), { label: `deliver:${id}`, phase: 'Helpers', model: M.deliver, effort: 'low' })
  const helperOpts = ev => {
    const base = { label: `${ev.type}:${ev.from}`, phase: 'Helpers' }
    if (ev.type === 'researcher') return Object.assign(base, { model: M.researcher })
    if (ev.type === 'go-scout') return Object.assign(base, { model: M.scout })
    return Object.assign(base, { model: M.runner, effort: 'low' })
  }
  const ROLE_OF = { 'go-scout': 'scout', researcher: 'researcher', runner: 'runner' }
  const helperRun = ev => agentAs(ROLE_OF[ev.type] || 'runner', helperPrompt(ev), helperOpts(ev))
  log(`helper broker: serving helper requests for tracks ${TRACKS.join(',')}`)
  for (let i = 0; i < 900; i++) {
    const t = await agentAs('coord', takePrompt('helper', !requeued), { label: 'take', phase: 'Helpers', model: M.coord, effort: 'low', schema: TAKE_SCHEMA })
    if (!t) continue
    requeued = true // only a take that actually ran has re-served the orphans
    if (t.stop) break
    for (const ev of (t.events || [])) {
      const lane = String(ev.from).split('/')[0]
      if (ev.type === 'researcher') {
        perLane[lane] = (perLane[lane] || 0) + 1
        if (perLane[lane] > RESEARCHER_LIMIT) {
          pending.push(deliver(ev.id, `DENIED: the researcher limit (${RESEARCHER_LIMIT} per lane) is used up for ${lane}. Ask go-scout for single facts or research inline.`))
          continue
        }
      }
      served.push(`${ev.type} ${ev.from}`)
      pending.push(helperRun(ev).then(ans =>
        deliver(ev.id, ans ? String(ans) : `ERROR: the ${ev.type} helper failed. Research inline.`)))
    }
  }
  await Promise.all(pending)
  log(`helper broker: served ${served.length} requests`)
  return { served }
}

if (ROLE === 'merger') return await mergerMain()
if (ROLE === 'helper') return await helperMain()
return await trackMain()
