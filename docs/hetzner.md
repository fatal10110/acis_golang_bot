# Running the bots on the Hetzner host (archived)

> **Archived.** This run is retired (`docs/plan.md`, D9): the bots now run on the owner's
> machine, see [`docs/home.md`](home.md), which also lists the retirement steps (stop and disable
> `acis-bots`, replace the `127.0.0.2` allowlist entry). `-allowlisted` no longer needs
> `-local-addr`. The rest of this page records the old setup.

The host is 2 vCPU / 3.8 GB, and the gameserver heap takes about 1.4 GB. The bots must never get
the host's IP banned, flood the server, or push the gameserver into the OOM killer. This guide
sets up the same-host deployment (`docs/plan.md`, "Hetzner deployment"). Server keys are from the
acis_golang `docs/run-servers.md`. The unit file is [`ops/acis-bots.service`](../ops/acis-bots.service).

## 1. Dedicated source address

The servers' connection accept filter (3 connections per second per IP after the first; the next
one bans the IP) is keyed by source IP. Give the bots their own address so the allowlist names
only them, and a login ban or flood on the bots never touches the gameserver's own link, which
comes from `127.0.0.1`.

- Bots bind `127.0.0.2` with `-local-addr 127.0.0.2` (`bots accounts` and `bots swarm` both take
  it). It sets `Dialer.LocalAddr` on both the login and the game dial.
- On Linux all of `127.0.0.0/8` is loopback: `127.0.0.2` needs no `ip addr add`.
- Both dials must go to loopback: `-login 127.0.0.1:2106`, and the game address the login server
  advertises (`Hostname`, section 2) must be loopback too, because a loopback source address is not
  routable to a public destination.

## 2. Server configuration

In `loginserver.properties` and in `server.properties` (both files take the same key; the key is
absent from the shipped files):

```properties
AcceptFilterAllowlist = 127.0.0.2
```

Allowlist exactly the bind address; never a public range. A malformed entry fails the boot.

In `server.properties`:

```properties
GameserverHostname = *
# The bot cap (section 4) plus the real players you expect.
MaximumOnlineUsers = 150
```

Keep `GameserverHostname = *`: it only sets the game listener's bind address. The address the
login server advertises in the ServerList is `Hostname` (`*` means the IP the gameserver link
connects from). Two ways to give bots a loopback game address:

- (a) Server for bots only: set `Hostname = 127.0.0.1`, or leave `*` and connect the gameserver
  link over a loopback `LoginHost`.
- (b) Server shared with real players: keep the public `Hostname`, and give the swarm the
  loopback game address with `-game 127.0.0.1:7777` (the gameserver's port): it replaces the
  advertised address on every game dial.

`EnableFloodProtection`, `FastConnection*` and `MaxConnectionPerIP` guard only the
gameserver-to-loginserver link on port 9014 and need no change for bots.

## 3. Pacing and bans

- With `-allowlisted` (the `-local-addr` IP is on `AcceptFilterAllowlist` of both servers, as in
  section 2), logins are not spaced: the accept filter skips that address, so no burst ban can
  follow. `bots accounts` then defaults `-interval` to 0, and `bots swarm` dials as soon as the
  previous bot has entered the world. Bots still log in one at a time, so the credential breaker
  below still stops at the first refused credential, and refused dials still back off.
  This run passed `-allowlisted` with `-local-addr` only: never pass it for an address the servers
  do not allowlist.
- Without `-allowlisted`, `bots accounts` logs in no faster than one per 1.25 s (`-interval`,
  floor enforced), under the filter's burst limit, and `bots swarm` starts every login and game
  dial of the process at least 1.25 s plus a random jitter of up to 0.5 s after the previous one.
- `bots swarm` backs all bots off exponentially while the servers refuse connections.
- Three credential failures in a row ban the IP for 10 minutes (plan, "Login pacing"), so the bots'
  credential breaker halts logins first: the first refused credential stops every later login
  of the swarm, and the swarm exits with code 3 once its bots are done, which the unit does not
  restart. On the bots' own address that ban cuts nothing else.
- Remote runs from a home or carrier-shared IP are not allowlisted: they stay under the burst
  limit with the same pacing. Never allowlist a shared IP.

## 4. Fixed bot cap

Pick a number of bots (`-bots N`, 50 in the unit) and raise `MaximumOnlineUsers` by the same
number. The swarm runs exactly one bot per account, `PREFIX1` to `PREFIXN`, each with one session
at a time, so it never holds more than N characters in the world. There is no dynamic log-off: bots cannot see the live player count, so a full server
refuses the bots' logins instead of the cap adapting. Size the cap against memory: leave the
gameserver its 1.4 GB heap plus the OS, and the swarm inside `MemoryMax`.

## 5. systemd unit

```bash
sudo useradd --system --no-create-home acis-bots
sudo install -d -m 0750 -o acis-bots /etc/acis-bots
sudo install -m 0600 -o acis-bots secret /etc/acis-bots/secret    # never commit or log it
sudo install -m 0755 bots /opt/acis-bots/bots
sudo install -m 0644 ops/acis-bots.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now acis-bots
```

| Setting | Value | Why |
| --- | --- | --- |
| `CPUQuota` | `100%` | One of two vCPUs at most. |
| `MemoryMax` | `1G` | Leaves room for the 1.4 GB heap and the OS. |
| `Nice` | `10` | The gameserver wins CPU contention. |
| `OOMScoreAdjust` | `500` | The OOM killer takes bots, not the gameserver. |
| `LimitNOFILE` | `1024` | About 2 descriptors per bot plus margin; raise it with `-bots`. |
| `RestartSec` | `60s` | A crash loop would be a login loop. |

Check the file with `systemd-analyze verify ops/acis-bots.service` (it warns that the binary
is missing off-host).

## 6. Kernel notes

- Ephemeral ports: each bot holds one connection to the game port from a fixed source address, so
  the 28k default range (`net.ipv4.ip_local_port_range`) is ample for a few hundred bots; a
  reconnect storm leaves `TIME_WAIT` sockets, still well below the range.
- `net.core.somaxconn` (listen backlog) defaults to 4096 on current kernels; if it is lower
  (`sysctl net.core.somaxconn`), raise it above the bot cap so a start-up burst is not dropped.

## 7. Check

After starting, `systemctl status acis-bots` shows the cgroup limits, and `ss -tn src 127.0.0.2`
lists the bot connections. The guide is done when a run on the host stays inside these limits and
the gameserver is untouched; that run is the owner's step on issue #15.

## 8. Metrics endpoint

The swarm serves live counters as `expvar` JSON on loopback only (`internal/botmetrics`) when it
runs with `-metrics 127.0.0.1:6060` (the unit in `ops/acis-bots.service` sets it); it is off by
default. The endpoint binds before the first login, a bind failure stops the run (exit 1), and it
keeps serving until every bot has logged out.

- The address must be a loopback IP and port (`127.0.0.0/8` or `[::1]`). An empty host
  (`:6060`), `0.0.0.0`, `::`, any other IP and any host name are refused before binding, and a
  listener bound anywhere else is closed. Never expose it with a proxy or a firewall rule.
- Read it on the host: `curl -s http://127.0.0.1:6060/debug/vars | jq`.
- `bots` is the swarm total, summed from per-bot counters on a period (its `at` field): events
  drained, packets sent and held by the pacer, rule hits, reconnects, server aborts, whether the
  credential breaker is open, and two histograms with power-of-two buckets (`queue_depth`: events
  per brain wakeup; `wake_lag_us`: how late timed wakeups ran, which grows when `CPUQuota` starves
  the swarm). No account name or secret appears; `cmdline` is not served.
- `memstats` is the Go runtime's: `HeapAlloc` (live heap) and `Sys` (memory from the OS). The
  RSS against `MemoryMax` is `systemctl show -p MemoryCurrent acis-bots`, or `VmRSS` in
  `/proc/$(systemctl show -p MainPID --value acis-bots)/status`. Once every bot is in the world,
  both should stay flat over a long `-persistent` run; a steady climb is a leak.

## 9. GM control

GMs command the bots by whisper when the swarm runs with `-commanders` or `-gm-dsn-file`. The
DSN file is a secret like the secret file: install it as `/etc/acis-bots/gm-dsn` with mode 0600
and owner `acis-bots`. The database user, the commands and running from a laptop are in
`docs/gm-control.md`.
