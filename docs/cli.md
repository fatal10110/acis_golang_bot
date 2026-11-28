# `bots` command reference

`bots` is the one binary of this repository (`cmd/bots`). It has six subcommands:

| Command | What it does |
| --- | --- |
| [`bots geopack`](#bots-geopack) | Packs the server's geodata regions into the read-only cache the bots map. |
| [`bots navbuild`](#bots-navbuild) | Builds the navigation hierarchy (nav cache) the walking bots plan routes on. |
| [`bots geoquery`](#bots-geoquery) | Writes a parity query file to check the bot's geodata answers against the server's engine. |
| [`bots accounts`](#bots-accounts) | Creates or verifies the accounts of one swarm instance, one paced login at a time. |
| [`bots swarm`](#bots-swarm) | Plays a fixed set of bots, all with one persona, until a signal. |
| [`bots ui`](#bots-ui) | Runs the bots of a local registry under a web control panel. |

`bots <command> -h` prints the flags of the binary you have; this page explains them and groups
them. The step-by-step run (secret file, server configuration, WSL2, Windows) is
[`docs/home.md`](home.md).

## Typical order

```bash
bots geopack  ...   # once per geodata update
bots navbuild ...   # after every geopack, doors.xml update or nav-cache format change
bots accounts ...   # once per swarm instance (optional: the first login also creates an account)
bots swarm    ...   # or: bots ui ...
```

Every persona walks on geodata (`docs/navigation.md`, D13a), so `swarm` and `ui` need both caches
and the datapack before a bot can play.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Done, or stopped by `SIGINT`/`SIGTERM`, or `-h`. |
| 1 | A run-time error: unreadable secret file, stale or missing cache, a taken port, a refused login in `accounts`, ... The message on stderr names it. |
| 2 | Bad flags or arguments, or an unknown subcommand. |
| 3 | `swarm` only: it stopped by itself. The credential breaker tripped, or every bot stopped for a reason a restart would repeat (refused credential, account in use, repeated aborts). **Do not restart on 3**: a restart loop is a login loop, and three refused logins in a row ban the IP. `ops/acis-bots.service` sets `RestartPreventExitStatus=3`. |

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

Without `-allowlisted`, no two dials start within 1.25 s of each other (plus jitter): the login
server's accept filter bans an IP on the fourth connection that arrives under 1 s after the
previous one. With it, the spacing is 0 until a connection is closed at accept, then 1.25 s for the
rest of the run. Logins are always one at a time, and the first refused credential stops every
later login (the credential breaker).

### Accounts and names

| Flag | Commands | Meaning |
| --- | --- | --- |
| `-secret-file PATH` | all three | The secret the per-bot passwords derive from (HMAC-SHA256 over the account name). A regular file of at most 4 KiB, mode `0600` (on Windows: an owner-only ACL), or it is refused; surrounding whitespace is ignored. Never log, commit or paste it. |
| `-prefix P` | all three | Account prefix: lowercase letters and digits ending in a letter. Accounts are `P1` to `PN`. For `ui`, required when the registry is created, then stored. |
| `-bots N` | `accounts`, `swarm` | Number of accounts. |
| `-persistent` | `accounts`, `swarm` | Persistent swarm: a bot reconnects after its session in the world ends (without it, the bot stops), and the secret must be strong: at least 16 bytes, not one repeated character, no placeholder word such as `changeme`. |
| `-name-marker TEXT` | `swarm`, `ui` | Text that marks the bots' character names (default `Bot`; D7). |
| `-name-marker-at suffix\|prefix` | `swarm`, `ui` | Where the marker goes (default `suffix`). |

### Datapack and geodata

Used by `swarm` and `ui`; `navbuild` and `geoquery` take the geodata ones too.

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

The bots map both caches read-only and never rebuild them. A missing or stale cache is an error
that names the command to run.

### GM control

Used by `swarm` and `ui`. See [`docs/gm-control.md`](gm-control.md).

| Flag | Meaning |
| --- | --- |
| `-commanders IDS` | Comma-separated object ids of the GMs who may command the bots by whisper, with no database check. Excludes `-gm-dsn-file`. |
| `-gm-dsn-file PATH` | File (mode `0600`) holding the game database DSN the bots check a whispering GM's access level in. Excludes `-commanders`. |
| `-gm-min-level N` | Access level (1-8) from which a character commands the bots, in place of the server's `isGM` levels. Needs `-gm-dsn-file`. |

With neither flag, no whisper commands a bot.

### Merchants

Used by `swarm` and `ui`; they bound the `merchant` persona.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-store-budget N` | `50000` | Adena each merchant's private store may sell for per session. |
| `-valuables-budget N` | `2000000` | Adena each merchant's valuables may sell for per session, by trade and store, apart from `-store-budget` (`docs/valuables/`). |
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

`GOMEMLIMIT` (for example `GOMEMLIMIT=1GiB`) and the bot cap (`-bots`, `-max-online`) bound the
process's memory on the owner's machine.

## `bots geopack`

```bash
bots geopack -geodata DIR -geoengine FILE [-out geodata.cache]
```

Packs the regions listed in `geoengine.properties` into one cache file the bots map read-only, and
prints the region-list hash. The bots never load geodata onto their heap. Copy the geodata with
`rsync -a`, which keeps each file's size and mtime: the cache is keyed on them, and any changed
region file makes the bots refuse the cache until it is packed again.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-geodata DIR` | | The datapack's `data/geodata`. |
| `-geoengine FILE` | | Local copy of the server's `geoengine.properties`. |
| `-out FILE` | `geodata.cache` | Cache to write. |

## `bots navbuild`

```bash
bots navbuild -geodata DIR -geoengine FILE -geodata-cache FILE -datapack DIR [-out nav.cache]
```

Builds the cluster hierarchy walking bots plan routes on (`docs/navigation.md`, section 2). Reads
the geodata cache, `doors.xml` and `zones/WaterZone.xml`, never the region files. Takes about ten
minutes on half the cores. Build it again after every `geopack`, every `doors.xml` change, and every
bots update that changes the cache's format version.

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

Samples wall, ledge, multilayer and random sites and writes the bot's answers for them. Run the
server's `geoprobe -expected-dump FILE` on the result from the server checkout before any change to
`internal/geo`: it must report `agreement=100.00%`. The same seed gives the same file.

## `bots accounts`

```bash
bots accounts -secret-file PATH -prefix P -bots N [-persistent] [-login host:port] [-interval 1.5s] [-allowlisted]
```

Logs each account in once, up to the server list. With `AutoCreateAccounts` on, the login server
creates it with its derived password; an existing account has its password verified. The first
failure stops the run, since refused credentials count toward the IP ban. Run it before a swarm,
never beside one on the same IP.

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

Plays accounts `P1` to `PN` until `SIGINT` or `SIGTERM`, each logging in one at a time through the
login gate. With `-persistent` a bot reconnects whenever its session ends (`docs/home.md`, section
5); without it, a bot whose session in the world ends stops.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-persona NAME` | `farmer` | What every bot plays; see [Personas](#personas). |
| `-support K` | `0` | With `-persona farmer`, the last K accounts play `support` instead: the healers the party organizer groups with the farmers. |
| `-no-parties` | off | Run no bot party organizer; bots only join parties players invite them to (`docs/parties/`). |
| `-clan-inviters NAMES` | any player | Characters whose clan invitation a `crafter` accepts while clanless. |
| `-sevensigns-members NAMES` | none | `pvp` bots that play Seven Signs member mode (`docs/pvp/`). |
| `-sevensigns-max-members N` | `1` | The most names `-sevensigns-members` may list. |
| `-signs-seal avarice\|gnosis` | `gnosis` | The seal a Seven Signs member votes when Dusk holds none. |
| `-record DIR` | none | Writes the first frame of every game opcode read: test fixtures for the decoders. |

## `bots ui`

```bash
bots ui -secret-file PATH -prefix P -login host:port -max-online 10 \
  -datapack DIR -geodata DIR -geoengine FILE -geodata-cache FILE -nav-cache FILE \
  [-import-prefix hz -import 20]
```

Runs bots in-process under a web panel instead of a fixed swarm (`docs/plan.md`, D10). Every bot
created is stored in a local registry, and a restart logs the bots marked online back in as the
same characters. Open `http://127.0.0.1:8090/`. The panel's pages are described in
[`docs/home.md`](home.md), section 8.

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

`-persona` (swarm) and the panel's preset picker choose from one catalog (`cmd/bots/persona.go`).
Every preset farms the datapack's spots and walks on the nav cache.

| Preset | What the bot does | Design |
| --- | --- | --- |
| `farmer` | Damage dealer that farms alone, or leads a bot party. | `docs/strategy.md`, `docs/parties/` |
| `support` | Party support (heals, buffs) that farms with its party. | `docs/plan.md` (M2), `docs/parties/` |
| `town` | Idles in town and runs errands (merchants, trainer, Newbie Helper, gatekeeper hops) for 10-20 minutes, then farms for 30-60. | `docs/plan.md` (M4) |
| `merchant` | Farms for loot and sells it in a private store; sells valuables by Trade ads, whispers and trades. | `docs/plan.md` (M4), `docs/valuables/` |
| `crafter` | Dwarven Fighter that joins a player's clan, crafts from the clan warehouse and deposits the products. | `docs/crafting.md` |
| `pvp` | Farms between fights; takes duels, fights flagged and karma players, arenas, clan wars and sieges. Never PKs by default. | `docs/pvp/` |

Every bot, whatever its preset, levels, equips, buys supplies, rests, dies and returns, answers GM
whispers, and fights back when attacked. What bots may do in game is configurable (D6): persona
defaults are not a safety boundary, the operational limits above are.

## Files

| Path | Written by | Notes |
| --- | --- | --- |
| secret file | the owner | `0600`, never in the repository. Every password derives from it: losing it loses the accounts. |
| `geodata.cache`, `nav.cache` | `geopack`, `navbuild` | Rebuild after geodata, `doors.xml` or format changes. |
| `<data-dir>/bots.json` | `ui` | The registry: bots, presets, online flags, login spacing. |
| `crash.log`, `crash.log.1` | `swarm`, `ui` | `0600`; rotated past 1 MiB. |
| outcome log | `-outcome-log` | JSON lines, rotated past 64 MiB. |
