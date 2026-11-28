# acis_golang_bot

[![Go](https://github.com/fatal10110/acis_golang_bot/actions/workflows/go.yml/badge.svg)](https://github.com/fatal10110/acis_golang_bot/actions/workflows/go.yml)

Bots that play [acis_golang](https://github.com/fatal10110/acis_golang), a Lineage II game
server written in Go, so real players have company in the world.

Each bot is an ordinary game account that logs in through the same protocol as the game client.
Once in the world it plays like a person would: it farms monsters, loots, rests, dies and comes
back, levels up and picks a profession, buys supplies and better gear, joins parties, trades,
sells in town, crafts for its clan, and fights players who are looking for a fight.

The bots are for gameplay, not load testing.

## Contents

- [Features](#features)
- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Getting started](#getting-started)
- [Documentation](#documentation)
- [Development](#development)
- [Project status](#project-status)
- [License](#license)

## Features

- **Plays through the real protocol.** Bots log in and play exactly as a game client does. The
  server needs no bot mode and gives bots no special treatment.
- **Six personas.** Each bot plays one of six personas, and plays its race, class and role the way
  a player would:

  | Persona | What it does |
  | --- | --- |
  | `farmer` | Hunts monsters, alone or leading a party of bots. The default. |
  | `support` | Heals and buffs its party while it farms. |
  | `town` | Spends time in town running errands, then goes out to farm. |
  | `merchant` | Farms for loot and sells it: in a private store, or by advertising rare finds and haggling with buyers. |
  | `crafter` | A dwarf that joins a player's clan and crafts from the clan warehouse. |
  | `pvp` | Takes duels, fights flagged players and visits arenas. Never attacks innocent players unless told to. |

- **Finds its own way.** Bots plan routes on their own copy of the server's map data and check
  every step with the server's own movement rules.
- **Two ways to run.** `bots ui` opens a local web control panel where you create, spawn and
  command bots one by one, and it remembers them across restarts. `bots swarm` runs a fixed group
  of bots from the command line.
- **Commanded in game.** A GM can whisper commands to any bot: `follow`, `assist`, `stay`,
  `go <spot>` and more.
- **Safe for the server.** Bots log in one at a time, slowly enough that the server's
  flood protection never bans the host's IP. All logins stop at the first wrong password, and
  every bot keeps its traffic well under the server's limits. Passwords come from a private secret
  file and never appear in logs.
- **Bots are not disguised.** Every bot's name carries a marker (`Bot` by default), so players can
  always tell.

## How it works

```
 your machine                                              game host
┌───────────────────────────────────────────────┐        ┌───────────────┐
│ bots ui  (web panel)  or  bots swarm          │        │ login server  │
│   ├─ login gate: one login at a time, paced ──┼──TCP──▶│               │
│   ├─ bot 1: reader → brain → paced writer ────┼──TCP──▶│ game server   │
│   ├─ bot 2: reader → brain → paced writer ────┼──TCP──▶│               │
│   └─ shared: map caches, game data, metrics   │        └───────────────┘
└───────────────────────────────────────────────┘
```

Each bot has two goroutines. A reader decodes the server's packets into events, and a brain keeps
a model of the world around the bot, decides what to do, and sends packets at a pace the server
accepts. Bots share the map caches, the game data and one login gate. The full picture is in
[`docs/architecture.md`](docs/architecture.md).

## Requirements

- **Go**, the version named in `go.mod`, to build from source. Prebuilt binaries for Linux and
  Windows are attached to every [release](https://github.com/fatal10110/acis_golang_bot/releases).
- **An acis_golang server** you run. The bots need:
  - `AutoCreateAccounts = True` on the login server, so each bot's first login creates its
    account;
  - room under `MaximumOnlineUsers` for the bots;
  - for a host that runs many bots, its address on both servers' `AcceptFilterAllowlist`.
- **Copies of the server's files** on the machine that runs the bots: the datapack XML
  (`aCis_datapack/data/xml`), the geodata directory and `geoengine.properties`.
- **Node.js 22 and npm**, only to build the control panel's web page from source.

## Getting started

The steps below run a few bots against your server. The complete guide, including server
configuration, WSL2 and Windows, is [`docs/home.md`](docs/home.md).

**1. Build.**

```bash
make ui          # optional: embeds the control panel's page (needs Node.js)
go build ./cmd/bots
```

**2. Pack the map data.** The bots read geodata from a cache they map into memory, never loading
it whole. Do this once, and again whenever the server's geodata or doors change.

```bash
./bots geopack -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -out ~/acis-datapack/geodata.cache
```

```bash
./bots navbuild -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -datapack ~/acis-datapack/xml \
  -out ~/acis-datapack/nav.cache
```

`navbuild` takes about ten minutes.

**3. Create a secret file.** Every bot's password is derived from it, so keep it private and back
it up: losing it means losing access to the bots' accounts.

```bash
install -d -m 0700 ~/.config/acis-bots
head -c 32 /dev/urandom | base64 > ~/.config/acis-bots/secret
chmod 0600 ~/.config/acis-bots/secret
```

**4. Start the control panel** and open <http://127.0.0.1:8090/>:

```bash
./bots ui -secret-file ~/.config/acis-bots/secret -prefix home -login game.example.org:2106 \
  -max-online 10 -datapack ~/acis-datapack/xml \
  -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -nav-cache ~/acis-datapack/nav.cache
```

Or run ten farmers from the command line instead:

```bash
./bots swarm -secret-file ~/.config/acis-bots/secret -prefix hz -bots 10 -persistent \
  -login game.example.org:2106 -datapack ~/acis-datapack/xml \
  -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -nav-cache ~/acis-datapack/nav.cache
```

> [!WARNING]
> If `bots swarm` exits with code 3, it stopped itself on purpose, for example after a refused
> password. Do not restart it automatically: repeated refused logins get the IP banned. Find the
> cause in the log first.

Every command and flag is described in [`docs/cli.md`](docs/cli.md).

## Documentation

| Read | To learn |
| --- | --- |
| [`docs/home.md`](docs/home.md) | How to set up and run the bots, step by step. |
| [`docs/cli.md`](docs/cli.md) | Every command, flag and exit code. |
| [`docs/gm-control.md`](docs/gm-control.md) | How GMs command bots in game. |
| [`docs/architecture.md`](docs/architecture.md) | How the code is organized and how a bot runs. |
| [`docs/decisions.md`](docs/decisions.md) | The decisions behind the project, and why they were made. |
| [`docs/glossary.md`](docs/glossary.md) | Game and project terms. |
| [`docs/README.md`](docs/README.md) | The index of all documentation, including the design documents. |

## Development

```bash
make hooks      # once per clone: formats and lints staged Go files on commit
make test       # unit tests
make test-race  # unit tests with the race detector, as CI runs them
make lint       # formatting check and linters; must print nothing
make ui-check   # the web page's type check, lint and tests
```

Unit tests need no network, database or server. CI runs the build, `go vet`, the unit tests with
the race detector, the linters and the web page's checks on every pull request. Scenarios that run
real bots against a live server are run by hand ([`docs/e2e-live.md`](docs/e2e-live.md)).

Contributions follow [`CONTRIBUTING.md`](CONTRIBUTING.md). The complete rules, for people and
coding agents alike, are in [`CLAUDE.md`](CLAUDE.md).

### Repository layout

| Path | Contents |
| --- | --- |
| `cmd/bots/` | The `bots` command: subcommands, flags, the persona catalog. |
| `internal/botclient/` | One bot: connections, login gate, brain, world model and all gameplay. |
| `internal/botwire/` | Encoding and decoding of the game's packets. |
| `internal/botregistry/`, `internal/botpanel/` | The control panel's bot registry, supervisor and web server. |
| `internal/geo/`, `internal/nav/` | The map data engine and route planning. |
| `internal/datapack/` | Readers for the server's XML game data. |
| `internal/...` | Accounts, names, metrics, planners, encryption and the live test harness. |
| `web/` | The control panel's web page (React, TypeScript). |
| `docs/` | Guides, reference and design documents. |

## Project status

The bots run on the owner's own server, and new behavior lands milestone by milestone. The
milestones and their issues are tracked in [`docs/plan.md`](docs/plan.md#tracking). Flags and the
control panel may still change from one release to the next.

## License

This project does not have a license yet. Until one is added, the code may be read, but it is not
licensed for reuse or redistribution.
