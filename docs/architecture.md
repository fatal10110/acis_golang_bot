# Architecture

How the code of this module is laid out and how a bot runs, from the login dial to a packet sent.
It describes the code as it is; the reasons and the owner decisions behind it are in
[`docs/plan.md`](plan.md) ("Architecture") and the milestone design docs
([`docs/README.md`](README.md)). Rules for changing it are in [`CLAUDE.md`](../CLAUDE.md) and
[`docs/agents/go-style.md`](agents/go-style.md).

## The shape in one picture

```
                         cmd/bots (one binary)
   geopack / navbuild / geoquery        accounts        swarm            ui
          │                                │              │               │
          ▼                                │              │        botpanel (HTTP, SSE, page)
   geo, nav ──► geodata.cache, nav.cache   │              │               │
                     (mapped read-only)    │              │        botregistry.Supervisor
                                           │              │        (bots.json, organizer)
                                           ▼              ▼               ▼
                                   ┌──────── process-wide pieces (cmd/bots/wiring.go) ────────┐
                                   │ LoginGate + credential breaker, Nav, datapack tables,    │
                                   │ spot and store registries, craft board, metrics, GM auth │
                                   └───────────────────────────┬──────────────────────────────┘
                                                               │ one per account
                                                               ▼
                                                   botclient.Bot (session.go)
                                       login server ──► game server ──► play ──► reconnect
                                                               │
                              reader goroutine ── botwire.DecodeEvent ──► Events queue
                                                               │
                                       Brain (Bot goroutine): World.Apply, rules, pacer ──► writer
```

## Packages

| Package | Responsibility |
| --- | --- |
| `cmd/bots` | The binary: subcommand dispatch, flags, wiring of the process-wide pieces, the persona catalog (`persona.go`), swarm and panel runs. See [`docs/cli.md`](cli.md). |
| `internal/botclient` | One bot: sessions, the reconnect loop, the login gate, the brain, its world model, the persona rules and every gameplay subsystem. The bulk of the module. |
| `internal/botwire` | Packet codecs for the login and game connections: encoders for what bots send, `DecodeEvent` into typed value events. Knows nothing of the brain (`boundary_test.go`). |
| `internal/commons/wire`, `commons/crypt` | Little-endian frame primitives; Blowfish, RSA, checksums. Copied from the server. |
| `internal/loginserver/crypt`, `gameserver/network/cipher` | The login packet crypt and the game XOR stream cipher, client side. |
| `internal/botaccounts` | Account names `PREFIX1..N` and their passwords, derived by HMAC from the secret file. |
| `internal/ownerfile` | Refuses a secret file another user can read (mode bits on Unix, the DACL on Windows). |
| `internal/botnames` | Pronounceable character names plus the bot marker (D7). |
| `internal/datapack` | Reads the parts of the server's XML datapack the bots plan with: NPCs, spawns and spots, items, skills, classes, recipes, buylists, multisells, teleports, zones. |
| `internal/charmodel` | Predicts a character's final stats and damage from class, level, gear and skills; used to pick gear and judge fights. |
| `internal/craftplan` | The clan crafter's pure planner: what to withdraw and which recipe chains to make. |
| `internal/geo/...` | The bot's port of the server's geodata engine: region readers, blocks, doors, the engine's walk and line-of-sight checks, the mmap cache, the parity query (`geoquery`). |
| `internal/mmap` | Read-only file mapping for the two caches. |
| `internal/nav` | Builds the cluster hierarchy (`nav.cache`) and plans ground routes on it. |
| `internal/botparty` | Pure planner of bot parties: given bots and policy, which bot leads, joins or leaves. |
| `internal/botregistry` | The panel's registry (`bots.json`) and its `Supervisor`; the swarm's party `Organizer`. |
| `internal/botpanel` | The panel's loopback HTTP server: JSON API, server-sent events, log ring, embedded page. |
| `internal/botmetrics` | Counters and histograms of one process, served as expvar JSON on loopback; pprof serving. |
| `internal/gmdb` | Reads a whispering character's access level from the game database for GM authority. |
| `internal/e2e` | The live e2e harness and scenarios (`docs/e2e-live.md`). |
| `internal/testsupport/decodefuzz` | Shared checks for the decoder fuzz targets. |
| `web/` | The panel's React and TypeScript page (Vite). `make ui` builds it into `internal/botpanel/web/app`, which `go:embed` picks up. |

Dependency direction: `cmd/bots` → `botpanel` → `botregistry` → `botclient` → `botwire` →
`commons`, crypt and cipher. `botclient` also uses `datapack`, `charmodel`, `craftplan`, `nav`
and `geo`. The planners (`botparty`, `craftplan`) and the leaf packages import nothing of the
bot. The module never imports the acis_golang server: code it needs is copied here and owned here.

## One bot's life

The per-account loop is `botclient.Bot` (`internal/botclient/session.go`). `Bot.Run` repeats an
attempt, computes a backoff, and waits, until its context ends or a stop reason a retry would only
repeat (a refused credential, an account kept in use, repeated aborts).

### 1. The login gate

Every bot of the process shares one `LoginGate` (`internal/botclient/gate.go`), built in
`cmd/bots/wiring.go`. A bot takes a turn before it dials; the turn is a single token, so only one
bot logs in at a time. `Turn.Dial` waits for:

- the **spacing**: 1.25 s plus up to 0.5 s of jitter between dials, which keeps the process out of
  the accept filter's burst window (four connections under 1 s apart ban the IP). An allowlisted
  gate starts at 0 and turns the spacing on at the first connection closed at accept;
- the **fleet backoff** while the servers refuse connections or are not ready: 2 s doubling to
  2 min, reset when any bot enters the world;
- the **ban quiet**: no dial for about 330 s after a connection closed at accept, the servers'
  `BanTime` plus a margin; any dial before would refresh the ban.

The **credential breaker**: the first refused credential halts the gate while the turn is still
held, and every later `Take` or `Dial` of the process fails with `ErrLoginsHalted` until the gate
is reset (the panel's "resume logins"; a swarm exits with code 3). Three refused credentials in a
row ban the IP for 10 minutes on the server.

The turn is released before play, so a bot in the world never holds up the others.

### 2. Login server, then game server

`login.go` runs the login handshake: `Init` (decrypted with the static key), GameGuard, the RSA
`RequestAuthLogin`, the server list, `RequestServerLogin`, and `PlayOk` with the play keys. The
bot then dials the game server (through the same turn, so the dial is spaced too), sends
`ProtocolVersion`, installs the XOR cipher (`game.go`), authenticates, selects or creates its
character, and enters the world. A new character takes the preset's starting class when it names
one (the crafter's Dwarven Fighter) or a drawn one, and a name from `botnames`.

### 3. Reader and events

`Bot.play` starts one reader goroutine per session. It reads frames into a reused buffer, so no
frame crosses goroutines; `botwire.DecodeEvent` copies each frame of an opcode the brain consumes
into a value event, and frames of every other opcode are dropped unparsed. A frame that fails to
decode is queued as a skip, and the reader goes on; a read error is queued as a lost connection.

`Events` (`internal/botclient/events.go`) is the bounded queue between the reader (only producer)
and the brain (only consumer). When the brain falls behind, a newer `MoveToLocation` of an object
replaces its queued one, without reordering that object's other events. The reader is joined
before `play` returns, so it is always gone before the next dial.

### 4. The brain

The `Brain` (`internal/botclient/brain.go`) runs on the `Bot`'s own goroutine. Its loop selects
on:

- the event queue: it drains everything queued and applies it to the `World`
  (`internal/botclient/world.go`), its model of itself, the objects around it, its inventory,
  party, clan and targets, and runs each subsystem's event hooks;
- its timer: on wake, `decide` (`internal/botclient/rules.go`) walks the persona's rule table in
  order and the first rule that matches returns an `Action`;
- commands: GM whispers that passed the authority check, and orders from the panel or the party
  organizer, delivered through a mailbox;
- logout and shutdown.

The brain takes an injected clock, so unit tests drive it with events and a fake clock and assert
the packets it sends.

A `Persona` (`rules.go`) is data: the role, policies (loot, PvP, valuables, Seven Signs, clan
membership) and options the rules read. The presets that build personas live in
`cmd/bots/persona.go`; the persona catalog is listed in [`docs/cli.md`](cli.md#personas).

### 5. Sending and pacing

The brain is the connection's only writer. Every send goes through the `pacer`
(`internal/botclient/pacing.go`), which the brain goroutine owns:

| Limit | Value | Server gate it stays under |
| --- | --- | --- |
| All frames | token bucket, 5/s, burst 10 | 160 frames/s, 400 per 5 s (two floods in a minute disconnect) |
| Emote | 3 s apart | `SocialTime` 2 s |
| All-channel chat | 3 s apart | human pace; `GlobalChatTime` is 0 by default |
| Dialog bypass | 0.5 s apart | `ServerBypassTime` 100 ms |
| Item drop | 2 s apart | `DropItemTime` 1 s |
| Craft | 0.5 s apart | `ManufactureDelay` 300 ms |
| Multisell choice | 0.5 s apart | `MultisellTime` 100 ms |
| Keepalive | `ValidatePosition` after 5 min of silence | idle close after 15 min |

A paced action is not sent; the rule runs again on a later wake. Merchant ads have their own
process-wide limits (`-ad-region-gap`, `-ads-per-hour`), and stores are spaced by the store
registry (`-stores-per-town`, `-store-spacing`).

### 6. Teardown and reconnect

On a lost connection `play` closes its `done` channel, closes the connection and joins the reader;
`Run` backs off and dials again. On shutdown the brain sends `Logout` and waits a short grace for
`LeaveWorld` (30 s). TCP keepalive and, on Linux, `TCP_USER_TIMEOUT` end a dead line within about a minute
(`dial_linux.go`). `AccountInUse` is retried with backoff for up to 3 minutes, then the bot stops.

## Gameplay subsystems

All in `internal/botclient`, grouped by file names. Each has a design doc.

| Subsystem | Files | Design |
| --- | --- | --- |
| Sessions, gate, reader | `session.go`, `login.go`, `game.go`, `dial*.go`, `gate.go`, `reader.go`, `events.go`, `recorder.go` | `docs/plan.md` §1, §5 |
| Brain core | `brain.go`, `rules.go`, `pacing.go`, `budget.go`, `command.go`, `authority.go`, `mailbox.go`, `inspect.go` | `docs/plan.md` §2-3, `docs/structure.md` |
| World model | `world.go`, `combat_world.go`, `pvp_world.go`, `character.go`, `items.go`, `itemindex.go` | `docs/plan.md` §2 |
| Movement | `walker*.go`, `navopen.go`, `travel.go`, `follow.go`, `escape.go`, `visit.go`, `town.go` | `docs/navigation.md`, `docs/walking.md` |
| Combat and farming | `combat_rules.go`, `engage.go`, `rotation.go`, `farm.go`, `spot*.go`, `danger.go`, `death.go`, `loot*.go`, `spoil.go`, `shots.go`, `arrows.go` | `docs/plan.md` (M3), `docs/strategy.md` |
| Classes | `kits_*.go`, `build.go`, `classchange.go`, `newbie.go`, `trainer.go`, `support.go` | `docs/strategy.md` |
| Gear and supplies | `gear*.go`, `enchant.go`, `crystallize.go`, `dyes.go`, `henna.go`, `supply*.go`, `shop.go`, `warehouse.go`, `weight*.go`, `overweight.go` | `docs/items.md` |
| Stores and valuables | `store*.go`, `market.go`, `trade.go`, `valuables*.go`, `ads.go`, `whispers.go` | `docs/plan.md` (M4), `docs/valuables/` |
| Crafting | `crafter*.go`, `craftboard.go`, `craftorder.go`, `recipe.go` | `docs/crafting.md` |
| Parties | `party*.go`, `channel.go` | `docs/parties/` |
| PvP | `pvp_*.go`, `challengeholds.go` | `docs/pvp/` |
| Social and clan | `chat.go`, `dialog.go`, `clan.go`, `clanmember.go`, `sevensigns*.go` | `docs/gm-control.md`, `docs/pvp/` |

## Process-wide pieces

`cmd/bots/wiring.go` and the run (`swarm.go`, `ui.go`) build the pieces every bot of a process
shares, each with one owner:

- the `LoginGate` and its breaker;
- the navigation: both caches mapped read-only, and one bounded LRU of cluster results
  (`-nav-lru-bytes`). Geodata never lands on the heap;
- the datapack tables, travel map and farming spot registry (spot caps scale with
  `-spawn-multiplier`);
- the store registry and the merchants' ad limits;
- the clan craft board, the party and ground-item claims and the duel challenge holds;
- the GM authority (`-commanders` or a database check through `-gm-dsn-file`);
- the metrics registry and the outcome log writer.

## Swarm and panel

**`bots swarm`** builds one `Bot` per account, all with one preset (the last `-support` accounts
play `support`), starts the party `Organizer` unless `-no-parties`, and runs until a signal. Its
exit code tells a supervisor whether a restart is safe ([`docs/cli.md`](cli.md#exit-codes)).

**`bots ui`** runs a `botregistry.Supervisor`, the one goroutine that owns the registry file and
every bot's run: create, spawn, despawn, delete, preset changes, commands, party forming, and the
login gate's reset. A bot created once is kept in `bots.json` and logged back in as the same
character after a reconnect or a panel restart (D10). `botpanel` serves a JSON API over the
supervisor on loopback (`/api/bots`, `/api/parties`, `/api/dashboard`, `/api/logs`, ...),
server-sent events at `/api/events`, and the embedded page. Requests must carry a loopback `Host`
and come from the panel's own page. No password, secret or DSN reaches the API, the page or the
logs.

## Safety, by construction

The operational limits are code, not persona options:

- one login at a time per process, spaced unless allowlisted, with the credential breaker;
- the per-connection pacer and the process-wide ad and store limits;
- loopback-only panel, metrics and pprof endpoints;
- secrets read at run time from owner-only files, never in flags, logs or the API;
- the bot cap (`-bots`, `-max-online`), `GOMEMLIMIT`, mapped (not loaded) geodata and a bounded
  nav cache keep the process inside its resource cap.

What bots may do in game is the persona's (D6) and is configurable; these limits are not.

## Testing

- **Unit tier** (`make test`, CI with `-race`): codec round trips and recorded fixtures (a decoder
  consumes exactly `len(payload)`), crypt against fixed vectors, brain decisions as events and an
  injected clock in, packets out. No network, database or server.
- **Fuzz targets** feed malformed frames to the decoders; CI runs their seed corpus.
- **Geodata parity**: `bots geoquery` plus the server's `geoprobe` must agree 100%.
- **Live e2e** (`make e2e-live`, never in CI): real bots against the server from the owner's
  machine, inside a safety envelope. See [`docs/e2e-live.md`](e2e-live.md).

Details: [`docs/agents/test-strategy.md`](agents/test-strategy.md).
