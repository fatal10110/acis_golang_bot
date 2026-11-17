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

## 4. Swarm

```bash
GOMEMLIMIT=1GiB bots swarm \
  -secret-file ~/.config/acis-bots/secret -prefix hz -bots 50 -persistent \
  -login game.example.org:2106 -allowlisted \
  -datapack ~/acis-datapack/xml \
  -metrics 127.0.0.1:6060 \
  -gm-dsn-file ~/.config/acis-bots/gm-dsn
```

- `-allowlisted`: the address the bots dial from is in `AcceptFilterAllowlist` of both servers
  (section 2), so logins are not spaced. It no longer needs `-local-addr`: leave the source address
  to the OS. Never pass it from an address the servers do not allowlist.
- Bots still log in one at a time. The credential breaker stops every later login at the first
  refused credential (three in a row ban the IP for 10 minutes), and the swarm then exits with
  code 3: do not restart it in a loop.
- `-game <public host:port>` when `Hostname` is not public (section 2).
- The bot cap (`-bots`) and `GOMEMLIMIT` bound the process on the owner's machine.
- `-metrics` serves live counters on loopback only (`docs/hetzner.md`, section 8, still applies).

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

## 8. Control panel

`bots ui` runs the bots in-process under a web panel (plan section 8) instead of a fixed swarm:

```
bots ui -secret-file ~/.config/acis-bots/secret -prefix home -login <server>:2106 -max-online 10 \
  -datapack <aCis_datapack/data/xml> [-import-prefix hz -import 20]
```

- The panel serves on `-listen` (default `127.0.0.1:8090`; a loopback IP only, any other address
  is refused) and answers only requests whose `Host` is loopback and that come from its own page.
  Open `http://127.0.0.1:8090/`.
- Pages: Bots (create, spawn, despawn, change persona, delete, import), a bot's detail (its
  snapshot, commands `stay`, `go <spot>`, `follow <player>`, `assist <player>`, `leave`, `logout`,
  and its log), Dashboard (login gate and breaker with "resume logins", the metrics), Logs. Live
  state comes over server-sent events; nothing needs Node.
- The registry lives in `-data-dir` (default `acis-bots` under the user's config directory); a
  restart on the same directory logs back in every bot marked online, as the same characters.
- `town` and `merchant` are offered only with `-datapack`.
- No password, secret or GM DSN reaches the API, the page or the logs.
