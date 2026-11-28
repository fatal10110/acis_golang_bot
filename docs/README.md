# Documentation

This is the map of the project's documentation. Find what you need below; most readers only need
the first table.

## Start here

| I want to... | Read |
| --- | --- |
| understand what the project is | [`README.md`](../README.md) |
| run the bots | [`home.md`](home.md), then [`cli.md`](cli.md) |
| understand how the code works | [`architecture.md`](architecture.md) |
| know why things are the way they are | [`decisions.md`](decisions.md) |
| look up a game or project term | [`glossary.md`](glossary.md) |
| contribute a change | [`CONTRIBUTING.md`](../CONTRIBUTING.md) |

## Guides and reference

| Page | What it covers |
| --- | --- |
| [`home.md`](home.md) | Setting up and running the bots: server settings, the secret file, the game data and map caches, running a swarm, reconnects, WSL2 and Windows, the control panel. |
| [`cli.md`](cli.md) | Every `bots` command and flag, the exit codes, the personas and the files the bots write. |
| [`gm-control.md`](gm-control.md) | How GMs command bots by whisper: the commands, the bots' replies, who may command them, the read-only database user, the SSH tunnel. |
| [`e2e-live.md`](e2e-live.md) | Running the live test scenarios against the real server: prerequisites, configuration, safety limits, cleanup. |
| [`architecture.md`](architecture.md) | The packages, the life of a bot from login to each packet sent, what the bots share, the control panel. |
| [`decisions.md`](decisions.md) | Every owner decision (D1-D49) in plain language, with its reason, plus the engineering principles and open questions. |
| [`glossary.md`](glossary.md) | Game terms (karma, clan warehouse, Seven Signs, ...) and project terms (persona, login gate, ...). |
| [`hetzner.md`](hetzner.md) | **Archived.** The retired setup that ran the bots on the server's own host. Its metrics and profiling sections are still referenced. |

## Design documents

Every feature area was designed in a document before any code was written. Each one describes the
problem, the server behavior the design depends on (with references into the server's code), the
owner's decisions, the design itself, the list of issues that delivered it, the risks and the
known gaps.

They are long and detailed, written for the people building the feature. Read them to understand
*why* the bots behave as they do. For *what* the code does today, the code and the guides above are
the reference. For a summary of the decisions, see [`decisions.md`](decisions.md).

| Milestone | Document | What it designs |
| --- | --- | --- |
| M0-M6 | [`plan.md`](plan.md) | The master plan: goals, all owner decisions, the overall architecture, the first personas (support, farmer, town, merchant), progression, names, sessions and accounts, GM control, metrics, the control panel. |
| M7 | [`navigation.md`](navigation.md), [`walking.md`](walking.md) | Bots planning their own routes on the server's map data; walking smoothly. |
| M8 | [`items.md`](items.md) | Gear, supplies and consumables, managed like a real player. |
| M9 | [`crafting.md`](crafting.md) | A dwarf bot that crafts for its clan. |
| M10 | [`strategy.md`](strategy.md) | Each bot playing its race, class and role. |
| M11 | [`parties/`](parties/README.md) | Bots forming parties with each other and with players. |
| M12 | [`valuables/`](valuables/README.md) | Merchants selling rare loot by advertising, haggling and trading. |
| M13 | [`pvp/`](pvp/README.md) | Bots that fight players: duels, arenas, flagged and karma players, clan wars, sieges, the Olympiad. |
| M14 | [`structure.md`](structure.md) | Hardening, the route planner and the structure of the brain. |

Inside a design document, a path like `internal/gameserver/...` usually points into the server's
repository, [fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Each document says
so near the top.

## For contributors

[`CONTRIBUTING.md`](../CONTRIBUTING.md) is the short guide. [`CLAUDE.md`](../CLAUDE.md) holds the
full rules, which apply to people and coding agents alike ([`AGENTS.md`](../AGENTS.md) points
there). These detailed guides go with it:

| Guide | What it covers |
| --- | --- |
| [`agents/go-style.md`](agents/go-style.md) | How Go code is written here: naming, types, errors, concurrency, packages, dependencies. |
| [`agents/test-strategy.md`](agents/test-strategy.md) | Kinds of tests, fixtures, the live test stack, fakes versus real types. |
| [`agents/verification.md`](agents/verification.md) | Which checks to run before calling a change done. |
| [`agents/tooling.md`](agents/tooling.md) | Search and code-navigation tools and how to use them. |
| [`agents/process-skills.md`](agents/process-skills.md) | How much process a task needs, by kind of task (written for coding agents). |
| [`agents/model-policy.md`](agents/model-policy.md) | Which AI models coding agents use for which work. |

## Keeping the documentation accurate

| When you change... | Update |
| --- | --- |
| a flag or a command | [`cli.md`](cli.md) |
| a package, a package boundary or which goroutine owns what | [`architecture.md`](architecture.md) and the package's doc comment |
| a setup or deployment step | [`home.md`](home.md) |
| a product decision (with the owner's approval) | the design document, [`plan.md`](plan.md), and [`decisions.md`](decisions.md) |

Design documents are not rewritten after delivery. A later decision gets its own section or
document, and a new number.
