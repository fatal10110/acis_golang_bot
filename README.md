# acis_golang_bot

Playable bots for the [acis_golang](https://github.com/fatal10110/acis_golang) game server. Each
bot logs in and plays through the real client protocol, like a person at a game client: it farms,
loots, rests, dies and comes back, levels and changes class, buys supplies and upgrades its gear,
parties, trades, sells in town, crafts for its clan and fights players. The bots exist so people
have fun playing alongside them. Load testing is not the goal.

The design, the owner decisions and the milestones are in [`docs/plan.md`](docs/plan.md). All the
documentation is indexed in [`docs/README.md`](docs/README.md).

## What it does

- **Real protocol only.** The bots speak the client protocol to the login and game servers. The
  server needs no bot-specific code and gives bots no special treatment.
- **Personas.** `farmer`, `support`, `town`, `merchant`, `crafter` and `pvp`. Each bot plays its
  race, class and role (`docs/strategy.md`).
- **Own navigation.** Bots plan routes on their own copy of the server's geodata, checked with the
  server's own geodata rules. The geodata is mapped from a cache and never loaded onto the heap
  (`docs/navigation.md`).
- **Two ways to run.** `bots swarm` runs a fixed set of accounts with one persona. `bots ui` runs a
  local web control panel where bots are created, spawned and commanded one by one, and remembered
  across restarts.
- **GM control.** GMs command bots in game by whisper: `follow`, `assist`, `stay`, `go <spot>`, ...
  (`docs/gm-control.md`).
- **Operational safety.** One login at a time with spacing, a credential breaker that stops every
  login at the first refused password, per-connection send pacing under the server's flood limits,
  secrets read from owner-only files, and loopback-only endpoints. The bots never get the host's IP
  banned or flood the server.

## Quick start

Needs the Go version named in `go.mod`, and a copy of the server's datapack XML, geodata and
`geoengine.properties`. The full walkthrough (server configuration, secret file, WSL2, Windows) is
[`docs/home.md`](docs/home.md).

```bash
go build ./cmd/bots
```

Build the two caches once (rebuild after a geodata or `doors.xml` update):

```bash
./bots geopack -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -out ~/acis-datapack/geodata.cache
```

```bash
./bots navbuild -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -datapack ~/acis-datapack/xml \
  -out ~/acis-datapack/nav.cache
```

Create a secret file the bots' passwords derive from. Keep it at mode `0600` and never commit it:

```bash
install -d -m 0700 ~/.config/acis-bots
head -c 32 /dev/urandom | base64 > ~/.config/acis-bots/secret && chmod 0600 ~/.config/acis-bots/secret
```

Run the control panel and open `http://127.0.0.1:8090/`:

```bash
./bots ui -secret-file ~/.config/acis-bots/secret -prefix home -login game.example.org:2106 \
  -max-online 10 -datapack ~/acis-datapack/xml \
  -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -nav-cache ~/acis-datapack/nav.cache
```

Or a fixed swarm of ten farmers:

```bash
./bots swarm -secret-file ~/.config/acis-bots/secret -prefix hz -bots 10 -persistent \
  -login game.example.org:2106 -datapack ~/acis-datapack/xml \
  -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -nav-cache ~/acis-datapack/nav.cache
```

The server must have `AutoCreateAccounts = True` for the first login to create each account, and
`MaximumOnlineUsers` headroom for the bots. A swarm that exits with code 3 stopped itself (for
example, the credential breaker tripped). **Do not restart it in a loop.** Every command and flag
is described in [`docs/cli.md`](docs/cli.md).

The panel's web page is embedded only in release binaries, or after `make ui` (Node.js 22 and
npm) before `go build`. Without it the panel shows a page that says so, and its API still works.

## Releases

Every published GitHub release carries `bots-linux-amd64`, `bots-windows-amd64.exe` and
`SHA256SUMS`, built with the panel's page by `.github/workflows/release.yml` from the release's
tag.

## Repository layout

```
cmd/bots/            the binary: subcommands, flags, persona catalog, wiring
internal/botclient/  one bot: sessions, login gate, brain, world model, gameplay rules
internal/botwire/    packet codecs (login and game)
internal/botregistry/, botpanel/   the control panel's registry, supervisor and HTTP server
internal/geo/, nav/  geodata engine port, caches and route planning
internal/datapack/   readers for the server's XML datapack
internal/...         accounts, names, metrics, party and craft planners, crypt, e2e harness
web/                 the control panel's React page
docs/                runbooks, reference and design documents (docs/README.md)
ops/                 the archived systemd unit
scripts/agent-tools/ helpers for coding agents
```

[`docs/architecture.md`](docs/architecture.md) explains how the packages fit together.

## Develop

```bash
make hooks      # once per clone: format and lint staged Go files on commit
make test       # unit tests
make test-race  # unit tests with the race detector, as CI runs them
make lint       # golangci-lint fmt --diff and golangci-lint run (v2, .golangci.yml); must print nothing
make ui-check   # the web page's typecheck, lint and tests
```

CI (`.github/workflows/go.yml`) runs build, vet, the unit tests with `-race`, and lint on every
pull request, plus the web checks. The live e2e scenarios against a real server
(`make e2e-live`, [`docs/e2e-live.md`](docs/e2e-live.md)) never run in CI.

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) before opening a pull request.
