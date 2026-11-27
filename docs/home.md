# Running the bots from home

The bots run on the owner's machine and play over the internet, through the servers' public
addresses, like real players (`docs/plan.md`, D9 and "Home run"). The ban risks of this run are
accepted (D11). The servers stay on their own host; this guide replaces the same-host run in
[`docs/hetzner.md`](hetzner.md), which is kept as an archive. Server keys are from the acis_golang
`docs/run-servers.md`.

## 1. Retire the Hetzner run first

Both runs log in the same accounts, so running both makes them kick each other. Before the first
home run, on the server host:

```bash
sudo systemctl disable --now acis-bots      # stop it and keep it from starting at boot
```

Then replace its `127.0.0.2` allowlist entry with the provider range (section 2). Keep
`/etc/acis-bots/secret` until it is copied to the owner's machine (section 3).

## 2. Server configuration

In `loginserver.properties` and in `server.properties` (both files take the key), allowlist the
owner's provider range instead of `127.0.0.2`:

```properties
AcceptFilterAllowlist = 203.0.113.0/24
```

- The value is a list of IPs and CIDR prefixes (comma, semicolon or space separated). A malformed
  entry fails the boot.
- The servers read it at boot only: restart **both** servers after a change.
- The range is the provider's, so a home IP change inside it stays allowlisted. Every other
  customer of that range skips the accept filter too: that is the accepted risk (D11).

In `loginserver.properties`:

```properties
AutoCreateAccounts = True
```

The bot accounts are created by their first login (`bots accounts` checks them).

In `server.properties`:

```properties
# The address the login server advertises in the ServerList: public, as for real players.
Hostname = game.example.org
# The bot cap plus the real players you expect.
MaximumOnlineUsers = 150
```

- If `Hostname` cannot be public, give the swarm the game address with `-game <public host:port>`:
  it replaces the advertised address on every game dial.
For a clan crafter (`docs/crafting.md`, D17), in `clans.properties`:

```properties
MembersCanWithdrawFromClanWH = True
```

Then, as the clan leader in game, give grade 6 (where new members join) the clan warehouse
privilege once. Without both, only the leader withdraws and the crafter cannot see the warehouse.

- Raise `MaximumOnlineUsers` by the bot cap: `-bots N` today, the control panel's `-max-online`
  once it lands (`docs/plan.md`, section 8). Bots cannot see the live player count, so a full
  server refuses their logins instead of the cap adapting.

## 3. The owner's machine

Copy the Hetzner host's secret file, so the swarm derives the same passwords and re-uses the `hz`
bots (their accounts and characters):

```bash
install -d -m 0700 ~/.config/acis-bots
scp HOST:/etc/acis-bots/secret ~/.config/acis-bots/secret    # read it with sudo on the host
chmod 0600 ~/.config/acis-bots/secret                        # never commit or log it
```

The account prefix stays `hz` (`-prefix hz`). Delete the file from the host once the home run
works, or keep it there only with the unit disabled.

Keep a local copy of the server's datapack XML, which the bots pick farming spots from:

```bash
rsync -a HOST:PATH/aCis_datapack/data/xml/ ~/acis-datapack/xml/
```

Keep a local copy of the server's geodata and of its `geoengine.properties`, then pack the listed
regions once into the cache the bots map read-only (docs/navigation.md, section 1). `rsync -a`
keeps each region file's size and mtime, which the cache is keyed on: a cache built by the
server's `geopack` stays valid, and any changed region file makes the bots refuse the cache until
it is packed again. The bots never load geodata onto their heap.

```bash
rsync -a HOST:PATH/aCis_datapack/data/geodata/ ~/acis-datapack/geodata/
rsync -a HOST:PATH/config/geoengine.properties ~/acis-datapack/geoengine.properties
bots geopack -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -out ~/acis-datapack/geodata.cache
```

`geopack` prints the region-list hash; it matches the server's region list when both
`geoengine.properties` copies list the same regions. Pack again after every geodata update.

Then build the nav cache, the hierarchy walking personas plan on (docs/navigation.md, section 2).
It reads the geodata cache and `doors.xml`, never the region files, and takes about ten minutes on
half the cores. The nav cache is keyed by the geodata cache, `doors.xml` and the build parameters:
build it again after every `geopack` or `doors.xml` update, and after a bots update that changes
the cache's format version (#231, #504, #514 and #315 did), since walking personas refuse a stale or missing
one at start and the bots never rebuild it themselves.

```bash
bots navbuild -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -datapack ~/acis-datapack/xml \
  -out ~/acis-datapack/nav.cache
```

The walking presets (`support`, `town`, `merchant`) run only on both caches: `bots swarm` refuses
to start them, and `bots ui` does not offer them, without the run-time flags below. Both commands
map the caches read-only, log the region-list hash at start, and never rebuild a cache: a missing
or stale one is an error naming `bots geopack` or `bots navbuild`. `-nav-lru-bytes` bounds the
process-wide cluster cache of the hierarchy (default 64 MB); each walking bot pins up to two more
results, 251 KB each on average (docs/navigation.md, "Cache"), and the dashboard's "Nav cache"
card shows how often a cluster is computed. The flags are `-geodata`,
`-geoengine`, `-geodata-cache` and `-nav-cache` (all four, with `-datapack` for `doors.xml`), as in
the swarm example of section 4.

To check the bot's geodata answers against the server's engine (before a change to
`internal/geo`), write the parity query file and run the server's `geoprobe` on it from the
server checkout. `geoprobe` decodes every region onto its heap (several GB), so run it only for
this check; it must report `agreement=100.00%`.

```bash
bots geoquery -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -out /tmp/geoquery.dump
go run ./cmd/geoprobe -expected-dump /tmp/geoquery.dump \
  -config ~/acis-datapack/geoengine.properties -geodata ~/acis-datapack/geodata
```

## 4. Swarm

```bash
GOMEMLIMIT=1GiB bots swarm \
  -secret-file ~/.config/acis-bots/secret -prefix hz -bots 50 -persistent \
  -login game.example.org:2106 -allowlisted \
  -datapack ~/acis-datapack/xml \
  -geodata ~/acis-datapack/geodata -geoengine ~/acis-datapack/geoengine.properties \
  -geodata-cache ~/acis-datapack/geodata.cache -nav-cache ~/acis-datapack/nav.cache \
  -metrics 127.0.0.1:6060 \
  -gm-dsn-file ~/.config/acis-bots/gm-dsn
```

- `-allowlisted`: the address the bots dial from is in `AcceptFilterAllowlist` of both servers
  (section 2), so logins are not spaced. It no longer needs `-local-addr`: leave the source address
  to the OS. Never pass it from an address the servers do not allowlist.
- Bots still log in one at a time. The credential breaker stops every later login at the first
  refused credential (three in a row ban the IP for 10 minutes), and the swarm then exits with
  code 3: do not restart it in a loop.
- A connection the server closes before its first byte (a ban at accept, a credential or admin
  ban, or a server that failed) logs "closed at accept: the address is banned or filtered, or the
  server failed" with the port and counts in `login_accept_closed_total`. No bot dials for the
  next 330 s (the servers' `BanTime` 300 s plus a margin; a longer `BanTime` needs `banQuiet`
  raised in the code), then one probes. With `-allowlisted` the first such close also turns the
  login spacing on for the rest of the run.
- While the servers are not ready (`PlayFail`, a system `LoginFail` from two different bots in a
  row, or a refused dial) every bot's next login waits one shared backoff, 2 s doubling to 2 min,
  reset when a bot enters the world. Two accounts whose logins always fail keep it stepping, so a
  healthy bot that reconnects then may wait up to 2 min.
- `-game <public host:port>` when `Hostname` is not public (section 2).
- The bot cap (`-bots`) and `GOMEMLIMIT` bound the process on the owner's machine.
- Farmer and support bots form bot parties (`docs/parties/`): `-support N` makes the last N bots
  play `support`, the healers the farmers lead; `-no-parties` keeps the bots in player parties
  only.
- `-metrics` serves live counters on loopback only (`docs/hetzner.md`, section 8, still applies).
- `-pprof 127.0.0.1:6061` serves Go profiles on loopback, off by default; recipes and the
  server's `-debug-addr` are in `docs/hetzner.md`, "Profiling". `bots ui` takes it too.
- A Go runtime crash is also written to `crash.log` in `acis-bots` under the user's config
  directory (`-crash-file` names another file), as `bots ui` does (section 8). A swarm and a panel
  on one host share the default file: each start appends its own header line.
- `-outcome-log <file>` writes one JSON line per farming bot's spot stay (`docs/structure.md`,
  Design section 6), off by default; past 64 MiB the file becomes `<file>.1` and a new one
  starts, so it stays under 128 MiB. It holds no account or character name and no chat text.
  `bots ui` takes it too.

## 5. Dead lines and reconnects

A PC that sleeps or a home IP change leaves the bots' sessions on a dead line.

- Every connection has TCP keepalive (30 s idle, then 3 probes 10 s apart) and, on Linux, a
  60 s `TCP_USER_TIMEOUT`, so a dead session ends within about a minute and the bot reconnects.
  Such an end does not count toward the 3 aborts that stop a bot; a reset or an EOF does.
- The server holds the old session until a login evicts it: each AccountInUse answer kicks it.
  The bot retries AccountInUse with backoff, at most 30 s apart, for 3 minutes from the first one,
  then stops as "account in use". After the server's ServerClose kick (another login took the
  account), the first AccountInUse stops it.

## 6. GM control over the SSH tunnel

The game database stays bound to loopback on the server host. The swarm reaches it through an SSH
tunnel whose key can only forward to MariaDB: set it up as in
[`docs/gm-control.md`](gm-control.md), "Running the bots from a laptop", and pass the DSN file
with `-gm-dsn-file`. If the tunnel is down, GM commands are refused and the bots play on.

## 7. WSL2

- The bots are a Linux build and run inside the WSL2 distribution; its kernel applies the
  keepalive and the user timeout above.
- Outbound connections leave through Windows, so the servers see the home public IP: the provider
  range in section 2 is what matters, not the WSL2 address.
- Localhost forwarding (`localhostForwarding=true` in `%UserProfile%\.wslconfig`, the default)
  makes the metrics endpoint (`127.0.0.1:6060`) and the control panel (`http://127.0.0.1:8090/`,
  section 8) reachable from a Windows browser too. Run the SSH tunnel
  inside the distribution, next to the swarm, so `127.0.0.1:13306` in the DSN is the tunnel's port.
- Keep the WSL2 window (or a `tmux` session in it) open while the swarm runs: WSL may shut an idle
  distribution down, which ends the swarm like a crash.
- A Windows sleep suspends the WSL2 VM: the bots wake on dead lines, and section 5 applies.

### Native Windows

Without WSL2, run `bots-windows-amd64.exe` from a GitHub release: every published release carries
it, `bots-linux-amd64` and `SHA256SUMS` (compare with `certutil -hashfile bots-windows-amd64.exe
SHA256`). Flags and commands are the same as on Linux.

- Windows has no mode bits, so the secret file (and a `-gm-dsn-file`) is checked by its ACL
  instead of `0600`: it is refused when anyone but its owner, the user running the bots, SYSTEM and
  Administrators may access it. Keep it under the profile and drop the inherited entries:

  ```
  mkdir %AppData%\acis-bots
  rem copy the secret to %AppData%\acis-bots\secret, then:
  icacls %AppData%\acis-bots\secret /inheritance:r /grant:r "%USERNAME%:F"
  ```

  A file under `C:\` or a shared folder inherits Users or Authenticated Users entries and is
  refused until `icacls` drops them.
- `bots ui` keeps its registry in `%AppData%\acis-bots` unless `-data-dir` names another
  directory. A second `bots ui` on the same directory is refused while the first runs; a killed
  panel leaves no lock behind.
- A runtime crash of `bots ui` or `bots swarm` is also written to `%AppData%\acis-bots\crash.log`
  (section 8): the file keeps the panic or goroutine dump, but a runtime throw's `fatal error:`
  line (and the `runtime: ...` lines before it) reach stderr only (#704), so keep a `2> ui.log`
  redirect to catch them. When another bots
  process holds the file, a start that finds it past 1 MiB cannot move it to `crash.log.1` and
  empties it instead, with a warning.
- The panel (`127.0.0.1:8090`) and the metrics and pprof endpoints listen on loopback only, so they need no
  firewall rule; if Windows Defender Firewall asks anyway, allow nothing. The bots' outbound
  connections need no rule either.
- The 60 s `TCP_USER_TIMEOUT` of section 5 is Linux's: on Windows the keepalive alone ends an idle
  dead line, and a line that dies with data in flight ends on Windows' own retransmission timeout.

## 8. Control panel

`bots ui` runs the bots in-process under a web panel (plan section 8) instead of a fixed swarm:

```
bots ui -secret-file ~/.config/acis-bots/secret -prefix home -login <server>:2106 -max-online 10 \
  -datapack <aCis_datapack/data/xml> [-import-prefix hz -import 20]
```

- The panel serves on `-listen` (default `127.0.0.1:8090`; a loopback IP only, any other address
  is refused) and answers only requests whose `Host` is loopback and that come from its own page.
  Open `http://127.0.0.1:8090/`.
- Pages: Bots (sort, filter, bulk spawn and despawn, create, import), a bot's detail (its
  snapshot with HP, MP and CP, persona change, spawn, despawn, delete, commands `stay`,
  `go <spot>` from a spot picker, `follow <player>`, `assist <player>`, `leave`, `logout`, each
  answered in a toast, and its log), Dashboard (login gate and breaker with "resume logins",
  login spacing, online count against `-max-online`, the metrics and their charts), Logs (filter, search,
  pause). Live state comes over server-sent events; the header shows when the stream reconnects.
  Light and dark themes follow the system until toggled.
- Login spacing (Dashboard) is the least time between two login or game dials of the panel. It
  only goes up from the gate's floor: 1250 ms (plus up to 500 ms jitter) without `-allowlisted`,
  which keeps every dial out of the accept filter's burst window, and 0 with it until a connection
  is closed at accept, which raises the floor to 1250 ms for the rest of the run; at most one
  minute. The panel keeps it in `bots.json` and applies it on the next start; a kept value under
  the floor of that start's gate leaves the floor.
- The page is a React application (`web/`). Release binaries embed it. A binary built from source
  embeds it only after `make ui` (Node.js 22 and npm), run before `go build ./cmd/bots`; without
  it the panel serves a page that says so, and the API still works. `npm run dev` in `web/`
  serves the page with hot reload against a `bots ui` on the default `-listen`.
- The registry lives in `-data-dir` (default `acis-bots` under the user's config directory); a
  restart on the same directory logs back in every bot marked online, as the same characters.
- A Go runtime crash (a fatal error such as out of memory, or an unrecovered panic) is written to
  `crash.log` in `-data-dir` as well as to stderr, so the first lines of a dump of thousands of
  goroutines survive a console that keeps only the last few thousand. Each start appends a line
  with its time, command, process id and build; a start that finds the file past 1 MiB moves it to
  `crash.log.1` (the one older generation kept). The file is `0600`: a goroutine dump can hold
  strings. One line stays on stderr only: the `fatal error: <message>` line the runtime prints
  before it starts dying (a runtime throw such as out of memory or a stack overflow); the file
  still holds the faulting goroutine, whose top frames name the throw site. A panic, a `SIGQUIT`
  dump and every goroutine trace reach the file whole (#704 tracks the missing line).
- `town` and `merchant` are offered only with `-datapack`; without it they are listed disabled.
  The spot picker lists the datapack's spots, so it is empty without `-datapack`.
- No password, secret or GM DSN reaches the API, the page or the logs.
