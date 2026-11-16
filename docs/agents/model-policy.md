# Agent model policy

Optimize for correctly completed issues per subscription usage window. Model roles below are
hypotheses to test, not facts about cost or quality.

## Current policy

- Keep the working platform default for the main agent unless comparable local evidence supports
  a change.
- Use medium reasoning for normal implementation and bounded research.
- Use a small, fast model (Haiku) for narrow, objectively verifiable work: one lookup, mailbox and
  CI polling, running a listed command and compressing its output.
- Use Sonnet at high effort for multi-file research and lane briefs.
- Keep implementation of protocol, concurrency and pacing code, and every review, on the session
  model; a cheaper implementation model is for mechanical lanes only (fixtures, test harness, docs,
  deployment guides).
- Escalate to a stronger model or higher reasoning for difficult debugging, high-risk review, or
  major design, not for routine work.
- Limit ordinary work to one main agent plus at most one research agent.
- Do not treat generated-token count as a proxy for subscription efficiency.

Agent files use floating aliases (`haiku`, `sonnet`), so each role follows the newest model of its
family.

## Availability and fallback

Model availability depends on the platform version, account and runtime. Before configuring a
model, verify that the runtime recognizes it and can spawn it as a subagent. If an override is
unavailable, remove it and let the subagent inherit the main model; report the rejected model and
the fallback, never fall back silently.

## Benchmark template

Before changing a role's model, run both candidates on comparable tasks scored against verified
answers, and record one row per task:

| Metric | Value |
| --- | --- |
| Model and reasoning effort | |
| Task category and size | |
| Successful completion | yes / no |
| Human interventions | |
| Tool calls, retries, compactions | |
| Tests passing after first implementation | |
| Reviewer-discovered defects | |
| Approximate usage, when visible | |
| Elapsed time | |

Primary metric: correctly completed tasks per usage window. Samples are small; rerun before
reversing a decision.
