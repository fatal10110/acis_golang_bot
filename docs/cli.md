# `bots` command reference

Everything this project does is one program, `bots`, with six subcommands. This page describes
each of them, every flag, and what the exit codes mean. For a step-by-step setup (server
configuration, the secret file, WSL2 and Windows), follow [`home.md`](home.md) first.

`bots <command> -h` prints the flags of the version you have.

## Contents

- [Commands](#commands)
- [Exit codes](#exit-codes)
- [Shared flags](#shared-flags): server and login, accounts and names, game data and maps, GM
  control, merchants, diagnostics
- Commands in detail: [`geopack`](#bots-geopack), [`navbuild`](#bots-navbuild),
  [`geoquery`](#bots-geoquery), [`accounts`](#bots-accounts), [`swarm`](#bots-swarm),
  [`ui`](#bots-ui)
- [Personas](#personas)
- [Files](#files)

## Commands

| Command | What it does | When you run it |
| --- | --- | --- |
| [`bots geopack`](#bots-geopack) | Packs the server's map data (geodata) into a cache file. | Once, and after each geodata update. |
| [`bots navbuild`](#bots-navbuild) | Builds the route cache that bots plan their walks on. | After each `geopack`, after a `doors.xml` change, and after an update that changes the cache format. |
| [`bots geoquery`](#bots-geoquery) | Writes a test file to check the bot's map answers against the server's. | Before changing the map code in `internal/geo`. |
| [`bots accounts`](#bots-accounts) | Creates or checks a swarm's accounts, one slow login at a time. | Optional, before the first swarm run. A bot's first login also creates its account. |
| [`bots swarm`](#bots-swarm) | Runs a fixed group of bots, all with one persona, until stopped. | To run bots from the command line or a service. |
| [`bots ui`](#bots-ui) | Runs bots under a local web control panel and remembers them. | To manage bots one by one from a browser. |

Every persona walks using the map caches
([D13a](decisions.md#d13a-no-walking-without-geodata)), so `swarm` and `ui` need both caches and
the datapack before any bot can play.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Finished, or stopped by `Ctrl+C` or `SIGTERM`, or `-h` was given. |
| 1 | Something went wrong while running: the secret file could not be read, a cache is missing or out of date, a port is taken, a login was refused during `accounts`, ... The error message says which. |
| 2 | The flags or arguments are wrong, or the subcommand is unknown. |
| 3 | `swarm` only: the swarm stopped itself because restarting would not help. Either a password was refused (the credential breaker), or every bot stopped for a reason that would happen again: a refused password, an account kept in use elsewhere, or repeated disconnects. |

> [!WARNING]
> **Never restart automatically on exit code 3.** A restart loop is a login loop, and three refused
> logins in a row get the IP banned. The sample systemd unit (`ops/acis-bots.service`) sets
> `RestartPreventExitStatus=3` for this reason.

## Shared flags

### Server and login

Used by `accounts`, `swarm` and `ui`.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-login host:port` | `127.0.0.1:2106` | Login server address. |
| `-game host:port` | advertised | Game server address to dial instead of the one the login server advertises (use when the server's `Hostname` is not public). Not on `accounts`. |
| `-server-id N` | `1` | Game server id to play on. Not on `accounts`. |
| `-timeout D` | `10s` | Longest wait for each read or write of a login or game entry. |
| `-local-addr IP` | OS picks | Source IP of every dial, when the host has several and only one is allowlisted. |
| `-allowlisted` | off | The source address is on `AcceptFilterAllowlist` of both servers, so logins are not spaced. Never pass it from an address the servers do not allowlist. |

**How logins are paced.** The server bans an IP that opens four connections less than a second
apart. So, without `-allowlisted`, the bots leave at least 1.25 seconds (plus a little random
jitter) between connections. With `-allowlisted` there is no spacing, until the first time the
server closes a connection the moment it opens; from then on the spacing is back for the rest of
the run. Either way, bots log in one at a time, and the first refused password stops all further
logins (the credential breaker).

### Accounts and names

| Flag | Commands | Meaning |
| --- | --- | --- |
| `-secret-file PATH` | all three | The secret the per-bot passwords derive from (HMAC-SHA256 over the account name). A regular file of at most 4 KiB, mode `0600` (on Windows: an owner-only ACL), or it is refused; surrounding whitespace is ignored. Never log, commit or paste it. |
| `-prefix P` | all three | Account prefix: lowercase letters and digits ending in a letter. Accounts are `P1` to `PN`. For `ui`, required when the registry is created, then stored. |
| `-bots N` | `accounts`, `swarm` | Number of accounts. |
| `-persistent` | `accounts`, `swarm` | Persistent swarm: a bot reconnects after its session in the world ends (without it, the bot stops), and the secret must be strong: at least 16 bytes, not one repeated character, no placeholder word such as `changeme`. |
| `-name-marker TEXT` | `swarm`, `ui` | Text that marks the bots' character names (default `Bot`; D7). |
| `-name-marker-at suffix\|prefix` | `swarm`, `ui` | Where the marker goes (default `suffix`). |

### Game data and maps

Used by `swarm` and `ui`; `navbuild` and `geoquery` take the map flags too.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-datapack DIR` | none | Local copy of the server's `aCis_datapack/data/xml`: farming spots, NPCs, items, skills, zones, doors. |
| `-geodata DIR` | none | Geodata region files the caches were built from. Only stat'ed (size and mtime key the cache). |
| `-geoengine FILE` | none | Local copy of the server's `geoengine.properties` (region list, `GeoDataType`). |
| `-geodata-cache FILE` | none | Cache written by `bots geopack`. |
| `-nav-cache FILE` | none | Cache written by `bots navbuild`. |
| `-nav-lru-bytes N` | 64 MiB | Bound of the process-wide cluster result cache of the nav hierarchy. |
| `-spawn-multiplier F` | `1` | The server's `SpawnMultiplier` (`npcs.properties`); scales how many bots a spot takes. |
| `-spawn-events LIST` | `extra_mob;18age;start_weapon` | The server's `SpawnEvents`, semicolon-separated: which event spawns run. |
| `-servitor-lines` | off | Draw the bots' branches with the servitor lines (Dark Avenger, Necromancer, Warlock, Elemental Summoner, Phantom Summoner; `docs/servitors.md`). Off keeps the branch tables without them (D51). |

The bots read both caches without changing them, and never rebuild them. If a cache is missing or
out of date, the error message names the command that rebuilds it.

### GM control

Used by `swarm` and `ui`. See [`gm-control.md`](gm-control.md).

| Flag | Meaning |
| --- | --- |
| `-commanders IDS` | Comma-separated object ids of the GMs who may command the bots by whisper, with no database check. Excludes `-gm-dsn-file`. |
| `-gm-dsn-file PATH` | File (mode `0600`) holding the game database DSN the bots check a whispering GM's access level in. Excludes `-commanders`. |
| `-gm-min-level N` | Access level (1-8) from which a character commands the bots, in place of the server's `isGM` levels. Needs `-gm-dsn-file`. |

With neither flag, bots ignore command whispers. Who may command a bot, and why, is explained in
[D3](decisions.md#d3-only-gms-command-bots).

### Merchants

Used by `swarm` and `ui`; they bound the `merchant` persona.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-store-budget N` | `50000` | Adena each merchant's private store may sell for per session. |
| `-valuables-budget N` | `2000000` | Adena each merchant's valuables may sell for per session, by trade and store, apart from `-store-budget` ([valuables](valuables/README.md)). |
| `-stores-per-town N` | `5` | Most bot stores open in one town at a time. |
| `-store-spacing N` | `150` | Least distance between two bot stores, in game units. |
| `-ad-region-gap D` | `1m` | Least time between two Trade ad lines of the process's merchants in one region. |
| `-ads-per-hour N` | `30` | Most Trade ad lines the process's merchants say in an hour. |

### Diagnostics

Used by `swarm` and `ui`. All endpoints accept loopback addresses only.

| Flag | Meaning |
| --- | --- |
| `-metrics ADDR` | Serves live counters as JSON at `/debug/vars` (for example `127.0.0.1:6060`). The panel's dashboard shows them without it. |
| `-pprof ADDR` | Serves Go profiles at `/debug/pprof/`. Heap profiles can hold strings: an owner diagnostic. |
| `-outcome-log FILE` | One JSON line per farming bot's spot stay (spot, class, level, minutes, experience, adena, deaths, kills, the model's estimate). No names, no chat. Rotated past 64 MiB, one older file kept. |
| `-crash-file FILE` | `swarm` only: where the Go runtime's crash output is kept besides stderr. Default `crash.log` in `acis-bots` under the user's config directory; `ui` writes it to `-data-dir`. |

To bound memory, cap the number of bots (`-bots`, `-max-online`) and set the `GOMEMLIMIT`
environment variable, for example `GOMEMLIMIT=1GiB`.

## `bots geopack`

```bash
bots geopack -geodata DIR -geoengine FILE [-out geodata.cache]
```

Packs the map regions listed in `geoengine.properties` into one cache file, and prints a hash of
the region list. The bots map this file into memory read-only instead of loading the geodata.

Copy the geodata with `rsync -a`, which keeps each file's size and modification time. The cache
records them, and the bots refuse the cache as soon as any region file changes, until you pack it
again.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-geodata DIR` | | The datapack's `data/geodata`. |
| `-geoengine FILE` | | Local copy of the server's `geoengine.properties`. |
| `-out FILE` | `geodata.cache` | Cache to write. |

## `bots navbuild`

```bash
bots navbuild -geodata DIR -geoengine FILE -geodata-cache FILE -datapack DIR [-out nav.cache]
```

Builds the route hierarchy that bots plan their walks on ([`navigation.md`](navigation.md),
section 2). It reads the geodata cache, `doors.xml` and `zones/WaterZone.xml`, and takes about ten
minutes on half the CPU cores. Build it again after every `geopack`, every `doors.xml` change, and
every bots update that changes the cache format (the bots will tell you).

| Flag | Default | Meaning |
| --- | --- | --- |
| `-geodata`, `-geoengine`, `-geodata-cache` | | As above; the cache defaults to `geodata.cache`. |
| `-datapack DIR` | | The datapack's `data/xml`. |
| `-out FILE` | `nav.cache` | Cache to write. |
| `-workers N` | half the cores | Clusters built at once. |
| `-covered N` | `10` | Extra cost, in tenths of a cell, of a step out of covered ground (under a roof or an upper floor). |

## `bots geoquery`

```bash
bots geoquery -geodata DIR -geoengine FILE -geodata-cache FILE [-out geoquery.dump] [-seed 1] [-n 1000]
```

Picks sample points (walls, ledges, places with several floors, random spots), records what the
bot's map code answers for each, and writes them to a file. Then run the server's
`geoprobe -expected-dump FILE` from a server checkout: it must report `agreement=100.00%`. Do this
before any change to `internal/geo`. The same seed always gives the same file.

## `bots accounts`

```bash
bots accounts -secret-file PATH -prefix P -bots N [-persistent] [-login host:port] [-interval 1.5s] [-allowlisted]
```

Logs each account in once, just far enough to see the server list. If the login server has
`AutoCreateAccounts` on, this creates the account with its derived password; for an existing
account it checks the password. The first failure stops the run, because refused passwords count
toward an IP ban. Run it before a swarm, never at the same time as one from the same IP.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-interval D` | `1.5s` | Pause between the end of one login and the next dial: at least `1.25s`, or any value from 0 with `-allowlisted` (0 when unset). |

## `bots swarm`

```bash
GOMEMLIMIT=1GiB bots swarm -secret-file PATH -prefix P -bots N -persistent \
  -login host:port -datapack DIR \
  -geodata DIR -geoengine FILE -geodata-cache FILE -nav-cache FILE \
  [-persona farmer] [-support K] [-metrics 127.0.0.1:6060] [-gm-dsn-file PATH]
```

Plays accounts `P1` to `PN` until you stop it with `Ctrl+C` or `SIGTERM`. The bots log in one at
a time. With `-persistent`, a bot reconnects whenever its session ends ([`home.md`](home.md),
section 5); without it, a bot stops once its session ends.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-persona NAME` | `farmer` | What every bot plays; see [Personas](#personas). |
| `-support K` | `0` | With `-persona farmer`, the last K accounts play `support` instead: the healers the party organizer groups with the farmers. |
| `-no-parties` | off | Run no bot party organizer; bots only join parties players invite them to ([parties](parties/README.md)). |
| `-clan-inviters NAMES` | any player | Characters whose clan invitation a `crafter` accepts while clanless. |
| `-sevensigns-members NAMES` | none | `pvp` bots that play Seven Signs member mode ([CP potions](pvp/cp-potions.md)). |
| `-sevensigns-max-members N` | `1` | The most names `-sevensigns-members` may list. |
| `-signs-seal avarice\|gnosis` | `gnosis` | The seal a Seven Signs member votes when Dusk holds none. |
| `-record DIR` | none | Writes the first frame of every game opcode read: test fixtures for the decoders. |

## `bots ui`

```bash
bots ui -secret-file PATH -prefix P -login host:port -max-online 10 \
  -datapack DIR -geodata DIR -geoengine FILE -geodata-cache FILE -nav-cache FILE \
  [-import-prefix hz -import 20]
```

Runs bots under a web control panel instead of a fixed swarm
([D10](decisions.md#d10-a-local-control-panel)). Every bot you create is saved in a local registry,
and when the panel restarts, the bots that were online log back in as the same characters. Open
<http://127.0.0.1:8090/> in a browser. The panel's pages are described in [`home.md`](home.md),
section 8.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen ADDR` | `127.0.0.1:8090` | Loopback address the panel serves on; any other address is refused. |
| `-data-dir DIR` | `acis-bots` under the user's config directory | Registry directory (`bots.json`), locked while the panel runs. |
| `-max-online N` | `10` | Most bots online at once. Raise the server's `MaximumOnlineUsers` by as many. |
| `-import-prefix P`, `-import N` | | Import accounts `P1` to `PN` of an earlier swarm as never-create bots of the `farmer` preset. |
| `-import-marker`, `-import-marker-at` | `Bot`, `suffix` | The name marker the imported swarm ran with. |

A binary built from source embeds the panel's page only after `make ui`; without it the panel
serves a page that says so, and the API still works.

## Personas

`-persona` (for `swarm`) and the panel's preset picker choose from the same list
(`cmd/bots/persona.go`). Every preset farms at the datapack's spots and walks using the route
cache. Each preset's role is set by [D21](decisions.md#d21-each-preset-plays-a-role-and-farmer-is-the-default).

| Preset | What the bot does | Design |
| --- | --- | --- |
| `farmer` | Deals damage. Hunts monsters alone or leads a party of bots. | [strategy](strategy.md), [parties](parties/README.md) |
| `support` | Heals and buffs its party while farming with it. | [plan](plan.md) (M2), [parties](parties/README.md) |
| `town` | Spends 10-20 minutes in town running errands (shops, trainer, Newbie Helper, gatekeepers), then farms for 30-60 minutes. | [plan](plan.md) (M4), [D44](decisions.md#d44-town-bots-also-farm) |
| `merchant` | Farms for loot and sells it in a private store. Sells rare finds by advertising on Trade and haggling by whisper. | [plan](plan.md) (M4), [valuables](valuables/README.md) |
| `crafter` | A Dwarven Fighter that joins a player's clan, crafts from the clan warehouse and puts the products back. | [crafting](crafting.md) |
| `pvp` | Farms between fights. Takes duels, fights flagged and karma players, visits arenas, joins clan wars and sieges. Never attacks innocent players unless enabled. | [pvp](pvp/README.md) |

Whatever its preset, every bot levels up, equips better gear, buys supplies, rests, dies and comes
back, obeys GM whispers, and fights back when attacked. What bots do in the game is a matter of
persona defaults, which can be changed
([D6](decisions.md#d6-bots-may-do-anything-a-player-may)). The safety limits are not.

## Files

| Path | Written by | Notes |
| --- | --- | --- |
| secret file | the owner | `0600`, never in the repository. Every password derives from it: losing it loses the accounts. |
| `geodata.cache`, `nav.cache` | `geopack`, `navbuild` | Rebuild after geodata, `doors.xml` or format changes. |
| `<data-dir>/bots.json` | `ui` | The registry: bots, presets, online flags, login spacing. |
| `crash.log`, `crash.log.1` | `swarm`, `ui` | `0600`; rotated past 1 MiB. |
| outcome log | `-outcome-log` | JSON lines, rotated past 64 MiB. |
