# Architecture

This page explains how the code is organized and what happens while a bot plays, from its first
connection to each packet it sends. It describes the code as it is today. The reasons behind the
design are in the [decision record](decisions.md), and the full designs are in the documents listed
in [`docs/README.md`](README.md). Unfamiliar terms are in the [glossary](glossary.md).

## Contents

- [Overview](#overview)
- [Packages](#packages)
- [The life of a bot](#the-life-of-a-bot)
- [Gameplay](#gameplay)
- [What bots share](#what-bots-share)
- [Swarm and control panel](#swarm-and-control-panel)
- [Safety limits](#safety-limits)
- [Testing](#testing)

## Overview

The repository builds one program, `bots`. It runs many bots in one process, each bot being one
game account with one character.

```
                         bots (one process)
 ┌───────────────────────────────────────────────────────────────────┐
 │  control panel (bots ui) or fixed swarm (bots swarm)              │
 │                                                                   │
 │  shared by all bots:                                              │
 │    login gate · map caches · game data · spot and store registries│
 │    GM authority · metrics                                         │
 │                                                                   │
 │  per bot:                                                         │
 │    ┌──────────┐  events   ┌──────────────────────────┐  packets   │
 │    │  reader  │ ────────▶ │  brain                   │ ─────────┐ │
 │    │goroutine │           │  world model · rules ·   │          │ │
 │    └────▲─────┘           │  pacing                  │          │ │
 │         │                 └──────────────────────────┘          │ │
 └─────────┼───────────────────────────────────────────────────────┼─┘
           │                 TCP to the game server                ▼
```

Three ideas run through the whole design:

1. **A bot is a client.** It speaks the game's own protocol and nothing else
   ([D4](decisions.md#d4-the-real-client-protocol-nothing-else)). The server handles a bot's
   connection like any player's.
2. **Each thing has one owner.** Each connection has one reader and one writer. Each piece of
   shared state belongs to one goroutine or one lock. This keeps the bots free of data races.
3. **Limits are code.** Login pacing, send rates and resource caps are enforced by the program, not
   left to configuration.

## Packages

The code lives under `internal/`, so other modules cannot import it. The command is in
`cmd/bots/`.

### The bot

| Package | What it does |
| --- | --- |
| `cmd/bots` | The `bots` command: reads flags, builds the shared pieces, holds the persona catalog (`persona.go`), runs a swarm or the panel. |
| `internal/botclient` | Everything one bot does: connecting and logging in, the login gate, the brain, the world model, and all gameplay. By far the largest package. |
| `internal/botwire` | Turns packets into typed events and back into bytes, for the login and game connections. It knows nothing of the brain ([D46](decisions.md#d46-the-wire-code-has-its-own-package)), and a test enforces that. |

### Running bots

| Package | What it does |
| --- | --- |
| `internal/botregistry` | The control panel's list of bots (`bots.json`) and the supervisor that runs them. Also the party organizer used by `bots swarm`. |
| `internal/botpanel` | The control panel's web server: a JSON API, live updates over server-sent events, the log view, and the embedded web page. |
| `internal/botparty` | Decides which bots form a party and who leads. Pure logic, no input or output. |
| `internal/botmetrics` | Counters and histograms, served as JSON on a local address. |
| `internal/botaccounts` | Account names (`prefix1`, `prefix2`, ...) and their passwords, derived from the secret file. |
| `internal/botnames` | Generates pronounceable character names with the bot marker. |
| `internal/gmdb` | Looks up a whispering character's access level in the game database. |
| `internal/ownerfile` | Refuses to read a secret file that other users could read. |

### Game knowledge

| Package | What it does |
| --- | --- |
| `internal/datapack` | Reads what the bots need from the server's XML data: monsters, spawns, items, skills, classes, recipes, shops, teleports and zones. |
| `internal/charmodel` | Predicts a character's stats and damage from its class, level, gear and skills. Used to choose gear and judge fights. |
| `internal/craftplan` | Plans what the clan crafter withdraws and crafts. Pure logic. |
| `internal/geo/...` | The bot's copy of the server's geodata engine: reading map regions, checking whether a character can walk from one point to another, and the map cache. |
| `internal/nav` | Builds the route hierarchy (`nav.cache`) and plans routes on it. |
| `internal/mmap` | Maps a cache file into memory read-only. |

### Protocol primitives and tests

| Package | What it does |
| --- | --- |
| `internal/commons/wire`, `internal/commons/crypt` | Frame and number encoding, Blowfish, RSA. Copied from the server. |
| `internal/loginserver/crypt`, `internal/gameserver/network/cipher` | The client side of the login encryption and the game connection's stream cipher. |
| `internal/e2e` | The harness that runs real bots against a live server. |
| `internal/testsupport/decodefuzz` | Shared checks for the fuzz tests that feed broken packets to the decoders. |

The web page's source is in `web/` (React, TypeScript, Vite). `make ui` builds it into
`internal/botpanel/web/app`, which is embedded in the binary.

**Dependencies flow one way:** `cmd/bots` uses `botpanel`, which uses `botregistry`, which uses
`botclient`, which uses `botwire`, which uses the protocol primitives. The pure planners and the
leaf packages depend on nothing else in the bot. Nothing imports the server: the code the bot needs
from it is copied here ([D5](decisions.md#d5-a-separate-repository)).

## The life of a bot

The loop that runs one account is `Bot.Run` in `internal/botclient/session.go`. It connects, plays
until the connection ends, waits a backoff, and connects again. It stops for good only when its
context ends or when retrying would only repeat a failure, such as a refused password or an account
that stays in use.

### 1. Waiting for a turn to log in

All bots in a process share one **login gate** (`internal/botclient/gate.go`). A bot must take a
turn before it connects, and only one bot holds a turn at a time.

While holding the turn, the bot waits for three things before it dials:

- **Spacing.** At least 1.25 seconds (plus up to half a second of random jitter) since the
  previous connection. The server bans an IP that opens four connections less than a second apart.
  A bot host on the server's allowlist (`-allowlisted`) skips the spacing until the first time the
  server closes a connection on accept.
- **Server backoff.** While the servers refuse connections or are not ready, every bot waits a
  shared delay that starts at 2 seconds and doubles up to 2 minutes. It resets as soon as any bot
  gets into the world.
- **Ban quiet period.** If the server closes a connection the moment it opens (a sign of a ban),
  no bot connects for about 330 seconds, the length of a ban plus a margin. Connecting earlier
  would only extend the ban.

The gate also holds the **credential breaker**. If the login server refuses a password, the gate
halts, and every later login in the process fails until someone resets it: the panel's "resume
logins" button, or a restart of the swarm. Three refused passwords in a row would get the IP banned
for 10 minutes.

A bot gives its turn back once it is in the world, so a playing bot never holds up the others.

### 2. Logging in

The bot runs the login server's handshake (`login.go`): it receives the server's keys, sends the
account name and password encrypted with RSA, picks the game server from the list, and gets a
session key. Then it connects to the game server (still inside its turn, so this connection is
spaced too), switches on the game's stream cipher (`game.go`), selects or creates its character,
and enters the world.

A new character takes the starting class its preset names (the crafter is always a Dwarven
Fighter) or a drawn one, and a generated name with the bot marker.

### 3. Reading packets

Once in the world, the bot starts a **reader goroutine** for the connection. It reads one frame at
a time into a reused buffer. For each frame of a kind the brain cares about, `botwire.DecodeEvent`
copies the fields into a new event value; frames of every other kind are dropped without being
parsed. Copying matters: the buffer is reused for the next frame, so no part of it may reach
another goroutine.

Events go into a bounded queue (`Events`, in `events.go`). The reader is its only producer and the
brain its only consumer. If the brain falls behind, a newer movement update for a character
replaces the older queued one, without changing the order of that character's other events.

A frame that fails to decode is logged and skipped, and the bot plays on
([D45](decisions.md#d45-an-undecodable-packet-is-skipped)). A read error ends the session.
Before the bot reconnects, the reader is always stopped and waited for.

### 4. Deciding what to do

The **brain** (`brain.go`) runs on the bot's own goroutine. It waits for whichever comes first:

- **New events.** It takes everything queued, applies it to the **world model** (`world.go`: the
  bot itself, the characters and items around it, its inventory, party, clan and target), and lets
  each gameplay subsystem react.
- **Its timer.** On each wake-up, `decide` (`rules.go`) walks the persona's list of rules in order.
  The first rule that applies returns an action, such as attack, cast, loot, rest, walk, buy or
  say.
- **Commands.** GM whispers that passed the authority check, and orders from the panel or the
  party organizer.
- **Shutdown.**

The brain gets its clock passed in. Tests drive it with events and a fake clock and check the
packets it sends, so they never wait on real time.

A **persona** (`rules.go`) is data: the bot's role and the policies its rules read (what to loot,
whom to fight, what to sell, whether to join a clan). The presets that build personas are in
`cmd/bots/persona.go`.

### 5. Sending packets

The brain is the only goroutine that writes to the connection. Every packet passes through the
**pacer** (`pacing.go`), which enforces these limits:

| What | Bot's limit | Server's limit it stays under |
| --- | --- | --- |
| Any packet | 5 per second on average, bursts of up to 10 | 160 per second, 400 per 5 seconds |
| Emotes | 3 seconds apart | 2 seconds |
| General chat | 3 seconds apart | none by default; the bot keeps a human pace |
| Dialog links | 0.5 seconds apart | 0.1 seconds |
| Dropping an item | 2 seconds apart | 1 second |
| Crafting | 0.5 seconds apart | 0.3 seconds |
| Item exchanges | 0.5 seconds apart | 0.1 seconds |
| Keepalive | a position update after 5 silent minutes | disconnects after 15 silent minutes |

If an action would break a limit, it is not sent, and the brain tries again on a later wake-up.

Some limits apply to the whole process rather than one bot: how often merchants may advertise
(`-ad-region-gap`, `-ads-per-hour`) and how many bot stores may open in a town and how close
together (`-stores-per-town`, `-store-spacing`).

### 6. Disconnecting

When the connection drops, the session closes, waits for its reader to stop, and `Bot.Run` backs
off before it reconnects. On shutdown the brain sends a logout and waits up to 30 seconds for the
server to confirm. TCP keepalive (and on Linux a user timeout) ends a dead connection within about
a minute. If the server says the account is still in use, the bot retries for up to 3 minutes.

## Gameplay

All gameplay lives in `internal/botclient`. File names group it by subsystem, and each subsystem
was designed in its own document.

| Subsystem | Files | Design |
| --- | --- | --- |
| Connections and login | `session.go`, `login.go`, `game.go`, `dial*.go`, `gate.go`, `reader.go`, `events.go` | [`plan.md`](plan.md) §1, §5 |
| Brain | `brain.go`, `rules.go`, `pacing.go`, `budget.go`, `command.go`, `authority.go`, `mailbox.go` | [`plan.md`](plan.md) §2-3, [`structure.md`](structure.md) |
| World model | `world.go`, `combat_world.go`, `pvp_world.go`, `character.go`, `items.go` | [`plan.md`](plan.md) §2 |
| Walking | `walker*.go`, `travel.go`, `follow.go`, `escape.go`, `visit.go`, `town.go` | [`navigation.md`](navigation.md), [`walking.md`](walking.md) |
| Fighting and farming | `combat_rules.go`, `engage.go`, `rotation.go`, `farm.go`, `spot*.go`, `death.go`, `loot*.go` | [`strategy.md`](strategy.md) |
| Classes | `kits_*.go`, `build.go`, `classchange.go`, `trainer.go`, `support.go` | [`strategy.md`](strategy.md) |
| Gear and supplies | `gear*.go`, `enchant.go`, `supply*.go`, `shop.go`, `warehouse.go`, `weight*.go` | [`items.md`](items.md) |
| Selling | `store*.go`, `market.go`, `trade.go`, `valuables*.go`, `ads.go`, `whispers.go` | [`valuables/`](valuables/README.md) |
| Crafting | `crafter*.go`, `craftboard.go`, `craftorder.go`, `recipe.go` | [`crafting.md`](crafting.md) |
| Parties | `party*.go`, `channel.go` | [`parties/`](parties/README.md) |
| Fighting players | `pvp_*.go`, `challengeholds.go` | [`pvp/`](pvp/README.md) |
| Chat and clans | `chat.go`, `dialog.go`, `clan.go`, `clanmember.go`, `sevensigns*.go` | [`gm-control.md`](gm-control.md) |

## What bots share

Some state is shared by every bot in the process. `cmd/bots/wiring.go` builds most of it, and the
run (`swarm.go`, `ui.go`) adds the rest. Each piece has one owner.

- **The login gate** and its credential breaker.
- **The map.** Both caches are mapped into memory read-only, and one size-bounded cache
  (`-nav-lru-bytes`) holds recently planned route pieces. Geodata never lands on the Go heap
  ([D13](decisions.md#d13-bots-find-their-own-routes)).
- **Game data:** the datapack tables, the travel map between towns, and the registry of farming
  spots, which caps how many bots share a spot.
- **Coordination between bots:** the store registry and advertising limits, the clan crafting
  board, claims on party slots and ground items, and duel challenge holds.
- **GM authority:** a fixed list of GM ids, or a lookup in the game database.
- **Metrics** and the outcome log.

## Swarm and control panel

**`bots swarm`** creates one bot per account, all with the same preset. With `-support N`, the last
N accounts play `support` instead. Unless `-no-parties` is given, it runs the party organizer over
its bots ([D25](decisions.md#d25-the-panel-organizes-bot-parties)). It runs until it gets a signal,
and its exit code tells whoever started it whether a restart is safe
([`cli.md`](cli.md#exit-codes)).

**`bots ui`** runs a **supervisor** (`internal/botregistry`): one goroutine that owns the list of
bots and every bot's run. Creating, spawning, despawning and deleting bots, changing presets,
sending commands, forming parties and resetting the login gate all go through it. Each bot is
saved in `bots.json`, and after a disconnect or a restart of the panel it logs back in as the same
character ([D10](decisions.md#d10-a-local-control-panel)).

The panel's web server (`internal/botpanel`) listens on a loopback address only. It offers a JSON
API (`/api/bots`, `/api/parties`, `/api/dashboard`, `/api/logs`, ...), live updates at
`/api/events`, and the web page. It answers only requests that name a loopback host and come from
its own page. No password, secret or database credential ever reaches the API, the page or the
logs.

## Safety limits

The limits that protect the server and the host are built into the code, not persona options
([decisions](decisions.md#how-decisions-are-made)):

- one login at a time per process, spaced unless the host is allowlisted, with the credential
  breaker;
- the per-connection pacer, plus the process-wide limits on ads and stores;
- the panel, metrics and profiling endpoints listen on loopback addresses only;
- secrets are read at run time from files only their owner can read, and never appear in flags,
  logs or the API;
- the bot count (`-bots`, `-max-online`), `GOMEMLIMIT`, mapped (not loaded) geodata and a bounded
  route cache keep the process within its memory budget.

What a bot does in the game is set by its persona and can be changed
([D6](decisions.md#d6-bots-may-do-anything-a-player-may)). These limits cannot.

## Testing

- **Unit tests** (`make test`; CI adds the race detector) need no network, database or server.
  They cover packet round trips and recorded packets (each decoder must use exactly the bytes it is
  given), encryption against fixed vectors, and brain decisions: events and a fake clock in,
  packets out.
- **Fuzz tests** feed broken packets to the decoders. CI runs their saved inputs.
- **Map parity.** `bots geoquery` together with the server's `geoprobe` must agree on 100% of
  sampled points before any change to `internal/geo` is merged.
- **Live scenarios** (`make e2e-live`) run real bots against the real server from the owner's
  machine, never in CI. See [`e2e-live.md`](e2e-live.md).

More in [`agents/test-strategy.md`](agents/test-strategy.md).
