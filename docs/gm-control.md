# GM control of the bots

A GM commands a bot by whispering to it (`docs/plan.md`, "In-game control"). Only a whisper
(`CreatureSay` of the Tell type) whose text is a command counts; any other line never reaches the
authority check. The sender is the object id the server writes into the line, never its name.

| Command | What the bot does |
| --- | --- |
| `follow` | Follows the GM from a slot behind it, in a party or not. |
| `assist` | Attacks what the GM attacks, after the persona's assist delay. |
| `stay` | Stops where it stands, and drops `follow` and `assist`. It still heals, buffs and fights back. |
| `go <spot>` | Walks to the farming spot of that spawnlist maker (any case) within the spot's cap, and stops following its party leader. It whispers back `unknown spot`, `that spot is full` or `I know no spots` when it cannot. |
| `invite me` | Whispers `invite me` back. The bot invites nobody, so the GM invites it and stays the leader. |
| `leave` | Leaves its party (nothing is sent outside a party). |
| `logout` | Logs out and stops the bot: it stays out until the swarm restarts. |

`follow`, `assist` and `go` cancel `stay`; `follow` cancels `go`. Orders last until the bot's
session ends.

## Chat replies

A whisper that is not a command may get a short reply (rule `chat`, `ChatPolicy` in the persona):
one line picked at random, sent back as a whisper 3 to 12 seconds later. Defaults: at most one
reply per sender per 10 minutes and 5 per bot per hour, counted across the bot's sessions; one
reply waits at a time, and a whisper heard meanwhile gets none. The bot never replies to the all,
shout or trade channels, to its own lines, to a command (even one the authority refuses), or to a
name carrying the bot marker (`-name-marker`, either placement, any case), so two bots never keep
a chat going. Whispers have no server flood gate, so the reply also waits for the 3 second chat
gate every whisper of the bot shares.

## Who may command

Pick one of two flags of `bots swarm`. With neither, every command is refused.

- `-commanders 268435457,268435458`: the object ids of the GMs, with no database. The list is
  trusted as given until the swarm restarts.
- `-gm-dsn-file /etc/acis-bots/gm-dsn`: the database check. For every command the swarm runs
  `SELECT accesslevel, deletetime FROM characters WHERE obj_Id = ?`, with no cache. A character
  commands if its level is a GM level of the server's `accessLevels.xml` (7 Admin, 8 Master; a
  level the table does not define plays as level 0), or with `-gm-min-level N` if its level is N or
  above. Negative levels and characters pending deletion are refused.
  - Each sender gets a token bucket of 5 checks, refilled at one every 2 s.
  - A sender the database showed is no GM is refused with no new query for 60 s, so a player
    whispering commands costs at most one query a minute.
  - At most 2 checks run at once, each bounded to 5 s, and at most 16 wait.
  - A failed query refuses the command and is not cached.

The party and trade hooks never query the database. A bot accepts the party invitation of a GM it
knows: a commander, or a GM whose command a check confirmed in the last 10 minutes. Other
invitations go to the persona. Such a GM is also exempt from the trade policy.

At most one bot of a swarm joins each other leader's party. The swarm holds one claim per leader
name. The first bot that accepts claims that leader, and later bots decline that leader until the
claim is released. The claim is released when that bot's party ends or its leader changes, when
no party window follows its acceptance within the 15 s invitation timeout, or when its session
ends. After a leader change the bot stays in the party without a claim, so another bot may join
the new leader. A GM's invitations take no claim and are never limited.
Without a GM configuration (`-commanders` or `-gm-dsn-file`), no GM is known, so every
invitation is limited.

The data source name is a secret. It lives in a file only its owner can read (mode 0600, as the
secret file). It never goes in a flag, and the swarm never logs it. Driver errors are cleaned of
it, and the driver's own logger is off. Format (go-sql-driver/mysql):

```
acisbot:PASSWORD@tcp(127.0.0.1:3306)/DATABASE
```

On the Hetzner host (`docs/hetzner.md`) the bots and MariaDB share the machine, so the DSN points
at `127.0.0.1:3306` directly.

## Database user

Create a user that can read three columns and nothing else, and that can hold no more than the
two connections the swarm opens:

```sql
CREATE USER 'acisbot'@'127.0.0.1' IDENTIFIED BY '...' WITH MAX_USER_CONNECTIONS 2;
GRANT SELECT (obj_Id, accesslevel, deletetime) ON DATABASE.characters TO 'acisbot'@'127.0.0.1';
```

Check it: `SHOW GRANTS FOR 'acisbot'@'127.0.0.1';` lists only that grant, and
`SELECT char_name FROM characters` as `acisbot` is refused.

## Running the bots from a laptop

MariaDB stays bound to the loopback address of the server host (`bind-address = 127.0.0.1`); its
port is never opened to the network. The laptop reaches it through an SSH tunnel whose key can do
nothing but that one forward.

1. On the server host, create a user with no shell for the tunnel. Put the laptop's dedicated
   public key in its `~/.ssh/authorized_keys`, restricted:

   ```
   restrict,port-forwarding,permitopen="127.0.0.1:3306",command="/bin/false" ssh-ed25519 AAAA... acisbot-tunnel
   ```

   `restrict` turns off the shell, the agent, X11 and PTYs. `port-forwarding` turns local
   forwarding back on, and `permitopen` limits it to MariaDB.
2. On the laptop, open the tunnel and leave it running:

   ```bash
   ssh -N -L 13306:127.0.0.1:3306 -i ~/.ssh/acisbot_tunnel tunnel@SERVER
   ```

   Through the tunnel MariaDB sees the connection come from `127.0.0.1`, which matches the
   `'acisbot'@'127.0.0.1'` user.
3. Write the DSN file with the tunnel's local port, readable only by you:

   ```bash
   install -m 0600 /dev/null ~/.config/acis-bots/gm-dsn
   $EDITOR ~/.config/acis-bots/gm-dsn    # acisbot:PASSWORD@tcp(127.0.0.1:13306)/DATABASE
   bots swarm ... -gm-dsn-file ~/.config/acis-bots/gm-dsn
   ```

If the tunnel is down, every database check fails. Commands are then refused and the failure is
logged without the DSN; the bots play on.
