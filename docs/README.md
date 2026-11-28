# Documentation

Where to find what. Start with the page for your role; the design docs are long and each covers one
milestone.

## Start here

| If you want to | Read |
| --- | --- |
| know what the project is and build it | [`README.md`](../README.md) |
| run the bots | [`home.md`](home.md), then [`cli.md`](cli.md) |
| understand the code | [`architecture.md`](architecture.md) |
| contribute a change | [`CONTRIBUTING.md`](../CONTRIBUTING.md) and [`CLAUDE.md`](../CLAUDE.md) |
| know why it is built this way | [`plan.md`](plan.md): goal, every owner decision (D1-D49), milestones |

## Running the bots

| Page | What it covers |
| --- | --- |
| [`cli.md`](cli.md) | Every `bots` subcommand and flag, exit codes, personas, files written. |
| [`home.md`](home.md) | The current deployment: server configuration, secret file, datapack and geodata caches, swarm, reconnects, WSL2 and native Windows, the control panel. |
| [`gm-control.md`](gm-control.md) | Commanding bots by GM whisper: commands, replies, who may command, the read-only database user, the SSH tunnel. |
| [`e2e-live.md`](e2e-live.md) | Runbook of the live e2e scenarios against the real server: prerequisites, configuration, safety envelope, cleanup. |
| [`hetzner.md`](hetzner.md) | **Archived.** The retired same-host deployment (D9); its metrics and profiling sections are still referenced. |

## Design documents

Each milestone was designed in a document before it was filed as issues. They record the server
behavior each feature rests on (with server file references), the owner decisions, the design, the
delivery table of issues, risks and known gaps. They are the reference for *why*; the code is the
reference for *what is*.

| Milestone | Document | Topic |
| --- | --- | --- |
| M0-M6 | [`plan.md`](plan.md) | The master plan: architecture, reader and events, brain, personas (support, farmer, town, merchant, progression), names, sessions and accounts, GM control, metrics, control panel. |
| M7 | [`navigation.md`](navigation.md), [`walking.md`](walking.md) | The bot's own route planning on the server's geodata; walking efficiency. |
| M8 | [`items.md`](items.md) | Loadout, supplies and gear like a real player. |
| M9 | [`crafting.md`](crafting.md) | The clan crafter. |
| M10 | [`strategy.md`](strategy.md) | Each bot plays its race, class and role. |
| M11 | [`parties/`](parties/README.md) | Bot parties: composition, organizer, leader, members, orders, spots, panel. |
| M12 | [`valuables/`](valuables/README.md) | Merchants selling rare loot by ads, whispers, trades and the warehouse. |
| M13 | [`pvp/`](pvp/README.md) | The PvP persona: duels, flagged and karma targets, arenas, clan wars, sieges, Olympiad, CP potions. |
| M14 | [`structure.md`](structure.md) | Hardening, the route planner and the brain's structure. |

Inside a design doc, code paths such as `internal/gameserver/...` usually name the server
repository, [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang); each doc says so
in its opening or "Paths" section.

## Contributor and agent guides

[`CLAUDE.md`](../CLAUDE.md) holds the rules every change follows (for people and for coding agents;
[`AGENTS.md`](../AGENTS.md) points there). The scoped guides:

| Guide | Covers |
| --- | --- |
| [`agents/go-style.md`](agents/go-style.md) | Naming, types, errors, concurrency, lock order, packages, dependencies. |
| [`agents/test-strategy.md`](agents/test-strategy.md) | Test tiers, fixtures, the e2e stack, fakes versus real types. |
| [`agents/verification.md`](agents/verification.md) | Focused checks, final gates, fuzzing, completion evidence. |
| [`agents/tooling.md`](agents/tooling.md) | `rg`, `gopls`, `rtk`, `jq`, ast-grep and grepai commands. |
| [`agents/process-skills.md`](agents/process-skills.md) | Task classes A-E and the process each one needs. |
| [`agents/model-policy.md`](agents/model-policy.md) | Agent model and effort choices. |

## Keeping the docs true

- A change that adds or renames a flag updates [`cli.md`](cli.md); one that moves a package
  boundary or a goroutine owner updates [`architecture.md`](architecture.md).
- A deployment step changes [`home.md`](home.md).
- Design docs are not rewritten after delivery; a later decision gets a new section or document,
  and its row in [`plan.md`](plan.md) "Tracking".
