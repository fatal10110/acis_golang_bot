# Decision record

This page lists every decision that shapes the project, in plain language: what was decided, why,
and what it means in practice. It is a summary. Each entry links to the design document that holds
the full reasoning, the options that were weighed and the server behavior the decision rests on.

New to the game's vocabulary (karma, clan warehouse, Seven Signs, ...)? See the
[glossary](glossary.md).

## Contents

- [How decisions are made](#how-decisions-are-made)
- [At a glance](#at-a-glance)
- [What the project is](#what-the-project-is) (D1, D2, D4, D5, D6, D7, D8)
- [Who can command a bot](#who-can-command-a-bot) (D3 and its extensions)
- [Where and how the bots run](#where-and-how-the-bots-run) (D9-D12)
- [Moving around the world](#moving-around-the-world) (D13, D13a)
- [Items, classes and crafting](#items-classes-and-crafting) (D14-D20, D49)
- [Personas and roles](#personas-and-roles) (D21-D24, D44)
- [Bot parties](#bot-parties) (D25-D29)
- [Merchants selling valuables](#merchants-selling-valuables) (D30-D34)
- [Fighting players](#fighting-players) (D35-D43)
- [Robustness and code structure](#robustness-and-code-structure) (D45-D48)
- [Engineering principles](#engineering-principles)
- [Amendments](#amendments)
- [Open questions](#open-questions)

## How decisions are made

The project has one owner, who makes every product decision. A decision starts as an open question
in a design document. The document lays out the options, a recommendation and the evidence from the
server's code. Independent reviewers check it, and the owner then approves it, picks another option,
or changes it. Approved decisions are numbered (**D1**, **D2**, ...) and recorded with their date in
the document's "Owner decisions" section, and in [`plan.md`](plan.md), which keeps the full list.

A decision is never edited away. When a later decision changes an earlier one, the later one says
"amends Dn", and the [Amendments](#amendments) table below tracks the chain.

Each entry gives a **Why** where the design document records one. Where it records only the
decision, the entry gives the context or the consequences instead, and does not guess at a reason.

Two kinds of rule should not be confused:

- **Persona defaults** (what a bot does in the game: whom it fights, what it sells, whether it joins a
  party) are choices for players' fun. They are configurable, and they are not a safety boundary
  (D6).
- **Operational limits** (login pacing, the credential breaker, send rate limits, resource caps) keep
  the server and its host safe. They are fixed in code and are not persona options.

## At a glance

The decisions that shape the project most:

1. Bots exist so people have fun playing alongside them. Load testing is not the goal.
2. Bots speak the real game protocol, like a person at a game client (D4). The server has no bot
   mode, and the bot never gets special treatment from it.
3. The bot is its own Go module in its own repository and never imports the server's code (D5).
4. Bots may do anything a player may do (D6). Only GMs command them, with a few named exceptions
   (D3).
5. Bots are marked by their name only (D7). Nothing else tells players apart from bots.
6. Bots run on the owner's machine and connect over the internet like real players (D9), managed
   from a local web control panel (D10).
7. Each bot finds its own way around, using its own copy of the server's map data (D13).
8. Bots are not disguised as humans (D48).

## What the project is

### D1: The first persona follows a player

**Decision.** The first thing a bot learned was to follow a player and help in fights (the
`support` persona). Farming on its own came after.


**Source.** [`plan.md`](plan.md), M2. Approved 2026-11-07.

### D2: No server-side shortcut unless measured

**Decision.** The server gets no special fast path for bots unless measurements cross a threshold
that is written down before the measuring starts.

**Why.** A fast path is server work with its own design approval, and only measured numbers can
justify it. Fixing the threshold before measuring keeps the result honest.

**What it means.** Since the bots moved off the server's host (D9), the one fast path that was
sketched (a local socket for bots on the same machine) no longer applies.

**Source.** [`plan.md`](plan.md), "Server-side work for D2". Approved 2026-11-07.

### D4: The real client protocol, nothing else

**Decision.** Bots connect, log in and play through the same protocol as the game client. There is
no lightweight bot protocol and no direct calls into the server.

**What it means.** The server needs no bot-specific code, and a bot can do only what a player's
client could. The bot carries its own copy of the protocol (packet layouts, encryption and
opcodes), checked byte for byte against the server's code.

**Source.** [`plan.md`](plan.md). Approved 2026-11-07.

### D5: A separate repository

**Decision.** The bot is its own Go module in its own repository.

**What it means.** Go does not allow one module to import another module's `internal` packages, so
the bot owns copies of everything it needs from the server: framing, encryption, the game cipher,
opcode constants and readers for the game's data files (about 2,500 lines at the start). The server
is a read-only reference.

**Source.** [`plan.md`](plan.md), "Consequences of a separate repository". Approved 2026-11-07.

### D6: Bots may do anything a player may

**Decision.** Bots may take any game action: trade, open a private store, drop items, party, fight
players. Persona defaults narrow what each bot does, but they can be changed and are not a safety
boundary.

**Why.** The bots exist for players' fun, and a bot that cannot trade or fight is less fun to play
with. Safety comes from the operational limits, not from forbidding game actions.

**Source.** [`plan.md`](plan.md). Approved 2026-11-07.

### D7: Marked by name only

**Decision.** A bot is recognizable by a marker in its character name (by default the suffix
`Bot`). There is no title marker and no separate player count.

**What it means.** Players can tell a bot by its name, and otherwise it is an ordinary character.
The marker text and its position are configurable (`-name-marker`, `-name-marker-at`).

**Source.** [`plan.md`](plan.md). Approved 2026-11-07.

### D8: The Java server is not targeted

**Decision.** The bots play against acis_golang only, not the original Java aCis server.


**Source.** [`plan.md`](plan.md). Approved 2026-11-07.

## Who can command a bot

### D3: Only GMs command bots

**Decision.** In the game, a bot takes commands (`follow`, `assist`, `stay`, `go <spot>`, ...) only
by whisper from a GM: a character whose access level the server marks as GM (`isGM="true"`, levels
7 and 8 in the shipped configuration).

**What it means.** The sender is identified by the object id the server writes into the whisper,
never by its name. The bots check a whisperer's access level in the game database through a
read-only database user (`-gm-dsn-file`), or trust a fixed list of GM ids (`-commanders`). With
neither, no whisper commands a bot. See [`gm-control.md`](gm-control.md).

**Source.** [`plan.md`](plan.md), "In-game control". Approved 2026-11-07.

### The exceptions to D3

Later decisions open a few specific commands to every player, each for one feature:

| Decision | Who | What they may do |
| --- | --- | --- |
| D20 | Any player | Invite a clanless crafter bot into their clan. The clan's leader may then order crafts like a GM. |
| D28 | Any player | Whisper `join` to a bot party's leader to be invited, or `invite me` to a party's support bot to take it into their own party. |
| D36 | Any player | Whisper `duel` to a `pvp` bot to be challenged. |

Every other command stays GM-only, and a GM always wins a conflict.

## Where and how the bots run

### D9: On the owner's machine, over the internet

**Decision.** The bots run on the owner's own computer and connect to the server's public address,
like real players. The earlier setup on the server's own host is retired.

**Context.** The bots used to run on the server's own host, a 2 vCPU, 3.8 GB machine where the game
server's heap alone takes about 1.4 GB, under a strict cap so they could never push the server into
the out-of-memory killer ([`hetzner.md`](hetzner.md), archived).

**What it means.** The owner's home IP address changes, so the owner's provider address range is put
on the servers' accept-filter allowlist, and the bots run with `-allowlisted`, which skips login
spacing. The bots themselves know nothing of the range. See [`home.md`](home.md).

**Source.** [`plan.md`](plan.md), "Home run". Approved 2026-11-17.

### D10: A local control panel

**Decision.** Bots are run from a web control panel on the owner's machine (`bots ui`). They are
spawned one at a time or in small batches, each with its own persona. Every bot created is stored
locally, and the panel logs existing bots back in (after a disconnect, a respawn, or a restart of
the panel) instead of creating new ones.

**Why.** These are gameplay bots, not a load test: each one is a character with its own persona
that keeps playing as the same character.

**Source.** [`plan.md`](plan.md), section 8. Approved 2026-11-17.

### D11: The home run's ban risk is accepted

**Decision.** The owner accepts the remaining risk of running bots from home. The home run adds no
ban or abuse handling beyond what the bots already have (the login gate and the credential breaker),
and the panel is treated as a local tool.

**Why.** It is the owner's private server.

**Source.** [`plan.md`](plan.md). Approved 2026-11-17.

### D12: The panel's page is a React application

**Decision.** The panel's page is built with React, TypeScript (strict), Vite and Tailwind, from
`web/`, and embedded in the binary. Node.js is needed only to build the page, never to run the
bots.

**What it means.** The bots stay a single binary with the page inside. This replaced an earlier rule
of plain JavaScript and no Node.

**Source.** [`plan.md`](plan.md). Approved 2026-11-17 (#106).

## Moving around the world

### D13: Bots find their own routes

**Decision.** A bot plans its own routes on its own copy of the server's geodata (the map data
that says where a character can walk). Every move it sends is a straight segment it has already
checked with the server's own movement rules, copied into the bot. The server's pathfinder is never
relied on.

**Why.** Before this, bots clicked toward a point and left the route to the server. The server's
pathfinder is built for a click on the screen and for monster AI, not for crossing a map, and the
bot never learned whether a click had worked. Each failure added another workaround (counting
stalled walks, sidestepping, giving up on spots), and the workarounds kept growing. Planning in the
bot removes the cause: the bot knows the terrain, so every move it sends is one the server will
walk.

**What it means.** The geodata is never loaded whole into memory. It is packed offline into a cache
(`bots geopack`) and mapped read-only, the way the game server reads it. A second cache holds a
route hierarchy built from it (`bots navbuild`). Any change to the bot's copy of the geodata code
must still agree 100% with the server's (`bots geoquery`).

**Source.** [`navigation.md`](navigation.md). Approved 2026-11-21.

### D13a: No walking without geodata

**Decision.** Walking personas need the geodata and route caches. The old straight-line walking is
not kept as a fallback.

**What it means.** Today every preset walks, so `bots swarm` and `bots ui` need both caches to run
any bot.

**Source.** [`navigation.md`](navigation.md). Approved 2026-11-21.

## Items, classes and crafting

### D14: Graded soulshots come from a crafter bot

**Decision.** Soulshots of D grade and above (consumables that boost attacks) are made for a clan
by a Dwarven crafter bot from the clan warehouse. No NPC was added to sell them.

**Why.** In the shipped game data no spawned NPC sells graded shots. The rejected options were
adding them to shop lists (a server data change for bots) or having no source at all. Until the
crafter existed, bots weighed weapons by whether shots for them could be had.

**Source.** [`items.md`](items.md). Approved 2026-11-21.

### D15: Class change at the class master

**Decision.** Bots change class at the class master NPC, which is free and needs no quest under
the shipped configuration. Bots pick a profession for their race and persona at levels 20 and 40,
then pick dyes.

**What it means.** The server gave the class master a permanent spawn in the towns
(fatal10110/acis_golang#5419).

**Source.** [`items.md`](items.md). Approved 2026-11-21.

### D16: Farmers leave their spot to restock

**Decision.** A farming bot may leave its spot to buy supplies in town, by Scroll of Escape, on foot
or through a gatekeeper. While it is away, its spot is released and others may take it.


**Source.** [`items.md`](items.md). Approved 2026-11-21.

### D17: Clan warehouse access is a server setting

**Decision.** Bots withdraw from the clan warehouse because the owner's server sets
`MembersCanWithdrawFromClanWH=True` and the clan leader grants the joining rank the withdraw
privilege.

**Why.** It is a server configuration that applies to every player, not a special case for bots.

**Source.** [`crafting.md`](crafting.md). Approved 2026-11-21.

### D18: The crafter grows through class changes

**Decision.** The crafter's Create Item skill grows through the class change of D15: Artisan at
level 20, Warsmith at level 40. Until then it crafts only what Create Item 1 allows (arrows, basic
materials, no-grade gear, first potions) and does not crystallize.

**Source.** [`crafting.md`](crafting.md). Approved 2026-11-21.

### D19: What the crafter may take

**Decision.** The crafter may take anything from the clan warehouse that a crafting chain it plans
needs, at any time. It plans chains from base materials up to a finished product and crafts a
chain only when the result is worth more than its inputs. It never takes adena, items no recipe it
can reach uses, or items with no known price unless an order names them.


**Source.** [`crafting.md`](crafting.md). Approved 2026-11-21.

### D20: Any player may recruit the crafter

**Decision.** A clanless crafter bot accepts a clan invitation from any player (or only from the
names in `-clan-inviters`), and its clan's leader orders crafts as a GM would.

**What it means.** This extends D3 for the crafter only.

**Source.** [`crafting.md`](crafting.md). Approved 2026-11-22.

### D49: Skills are never cast without their reagent

**Decision.** A bot never casts a skill without the item that skill consumes, and its shopping
trips restock those reagents from shops in the game, after potions, shots and arrows, and only from
the adena it has left over.

**Why.** The server refuses such a cast.

**What it means.** Crystal: D-Grade has no seller in the game, so the cubic summons that need it
are left out for now (#636).

**Source.** [`strategy.md`](strategy.md), "Skill reagents". Approved 2026-11-26.

## Personas and roles

### D21: Each preset plays a role, and `farmer` is the default

**Decision.** Each preset plays one role: `farmer` and `merchant` deal damage, `support` and `town`
support (heal, buff, tank), `crafter` crafts. `farmer` is the default preset for `bots swarm`, the
panel and imported bots.

**Source.** [`strategy.md`](strategy.md). Approved 2026-11-24.

### D22: How a bot picks its profession

**Decision.** At a class change a bot picks a profession weighted by how well it suits the bot's
race, drawn from a hash of its account name and role, so the same bot always picks the same way.
Once it has a profession it keeps that line. The panel may fix a bot's choice.

**What it means.** The same bot always draws the same profession, and bots of one race spread over
the professions that suit it.

**Source.** [`strategy.md`](strategy.md). Approved 2026-11-24.

### D23: Some professions are left out

**Decision.** Summoner lines (Warlock, Elemental Summoner, Phantom Summoner) and the Necromancer's
summons are left for a later milestone (#332). The Overlord is never picked.

**Why.** The summoner lines are planned as a later milestone of their own. The Overlord is a group
class.

**Source.** [`strategy.md`](strategy.md). Approved 2026-11-24.

### D24: The class rewrite may change behavior, one PR at a time

**Decision.** The rewrite of class behavior could change what bots do, but each pull request had to
say which behavior it changed, and the first one had to change none.

**What it means.** Later plans (such as M14) follow the same rule: every pull request states the
behavior it changes.

**Source.** [`strategy.md`](strategy.md). Approved 2026-11-24.

### D44: Town bots also farm

**Decision.** A `town` bot alternates: 10-20 minutes in town (drifting, running errands, using
gatekeepers), then 30-60 minutes farming, then home on a supply trip.

**Why.** In the owner's words: "there is no point to stay only in town".

**Source.** `internal/botclient/town.go` (#618). Approved 2026-11-26.

## Bot parties

### D25: The panel organizes bot parties

**Decision.** The panel's supervisor forms bot parties from what it knows: online bots in the same
town, within walking reach and a similar level. Bots do not look for each other in the game.
`bots swarm` runs the same planner over its own bots. Both are on by default (`-no-parties` turns
the swarm's off).

**Why.** In the game a bot only sees what is near it, and the game does not show other characters'
levels or roles. Leaders searching on their own would compete with each other and need to chat
bot to bot. The supervisor already knows every bot's preset, class, level, position and state, and
decides in one place.

**Source.** [`parties/`](parties/README.md). Approved 2026-11-25; swarm organizer 2026-11-26.

### D26: Who is in a bot party

**Decision.** A damage bot (`farmer`) leads, and support bots join, at least one of them a healer.
A party has 3 bots, at most 4. `merchant`, `crafter` and `town` bots never take part, and the panel
can keep any bot out or set a party by hand.

**Why.** Small parties stay within a farming spot's capacity and keep down the cost of bots that
see each other.

**Source.** [`parties/composition.md`](parties/composition.md). Approved 2026-11-25.

### D27: Loot and level range

**Decision.** Items are looted in turn, the server splits adena, and a spoil stays with whoever
swept it. A party forms only from bots within 3 levels of each other and breaks when they drift
more than 5 apart. Players in the party are not counted.

**Why.** Experience falls once the party's level is more than 5 above the monster's, and no farming
spot fits a party whose levels spread more than 5. The gap between 3 and 5 keeps a party from
forming and breaking over and over.

**Source.** [`parties/composition.md`](parties/composition.md). Approved 2026-11-25.

### D28: What players may ask of a bot party

**Decision.** Any player may whisper `join` to a bot party's leader to be invited, or `invite me` to
one of its support bots to take that bot into their own party. A GM's `invite me` always takes the
bot, and a GM's `leave` to the leader breaks the party up.

**What it means.** This extends D3 for these two whispers only.

**Source.** [`parties/players.md`](parties/players.md). Approved 2026-11-25.

### D29: Bots never invite unasked

**Decision.** A bot invites only the bots assigned to its party and players who whispered `join`.
Bots in a bot party never whisper each other.

**Source.** [`parties/`](parties/README.md). Approved 2026-11-25.

## Merchants selling valuables

### D30: Merchants advertise on Trade only

**Decision.** A `merchant` bot advertises valuable loot on the Trade chat channel, under caps on how
often, and never on Shout.

**Why.** The owner asked that a merchant try to sell a valuable find to players before putting it
in its store, as a player would. No channel a normal character speaks on reaches the whole server,
only the speaker's region. Before this decision bots never spoke unprompted. The alternative,
Trade and Shout both, was turned down.

**Source.** [`valuables/`](valuables/README.md), [`valuables/ads.md`](valuables/ads.md). Approved
2026-11-25.

### D31: What counts as valuable

**Decision.** An item is valuable when its rarity score reaches 100,000 and its price 10,000 adena.
The score multiplies the price by how hard the item is to obtain (how few spawns drop it, whether an
NPC trades it, whether it can be crafted, how far past safe it is enchanted).

**Why.** Merchants used to sell a rare sword the same way as a stack of common materials.

**What it means.** A valuable is sold before it would be deposited for the clan crafter.

**Source.** [`valuables/score.md`](valuables/score.md). Approved 2026-11-25.

### D32: How a valuable is priced and sold

**Decision.** The bot asks 100-150% of the item's reference price, depending on rarity, and lowers
its ask by 10 points for each round it goes unsold. It never goes below a floor. It makes at most
two counter-offers per buyer and item in a round, and may invite a whispering buyer it can see to
trade.

**Why.** A player selling something rare answers whispers and haggles.

**Source.** [`valuables/whispers.md`](valuables/whispers.md),
[`valuables/trades.md`](valuables/trades.md). Approved 2026-11-25.

### D33: Rounds, the warehouse and the store

**Decision.** A valuable gets a 5-minute round of ads, then goes into the bot's private warehouse.
Each new valuable brings the stored ones out for another round. When too many pile up (more than 6
warehouse rows, asks over 2,000,000 adena in total, or one unsold through 3 rounds), they go to the
bot's private store at their floor price.

**Why.** It copies how a player handles rare loot: try to sell it, park it in the warehouse when
nobody bites, try again with the next find, and dump it in a store only when too much piles up.

**Source.** [`valuables/round.md`](valuables/round.md),
[`valuables/release.md`](valuables/release.md). Approved 2026-11-25.

### D34: A separate budget for valuables

**Decision.** Valuables have their own per-session sales budget (`-valuables-budget`, default
2,000,000 adena), apart from the store budget.


**Source.** [`valuables/`](valuables/README.md). Approved 2026-11-25.

## Fighting players

### D35: A PvP preset; everyone else only fights back

**Decision.** A new `pvp` preset takes duels, fights flagged and karma players, and visits arenas.
Killing innocent players (PK) is off unless enabled per bot. Every other preset only fights back
and declines duels. All presets fight players with the same PvP skill logic.

**Why.** The owner asked for "a PVP persona that will fight with players".

**Source.** [`pvp/modes.md`](pvp/modes.md). Approved 2026-11-25.

### D36: How a PvP bot finds fights

**Decision.** A `pvp` bot accepts any duel it can, challenges a player who whispers `duel` to it,
challenges non-bot players and hunts flagged and karma players of its strength when it estimates it
can win, and visits the nearest arena from time to time.

**Source.** [`pvp/finding.md`](pvp/finding.md). Approved 2026-11-25.

### D37: PvP gear

**Decision.** A `pvp` bot scores gear half for monster farming and half for PvP. It keeps one set
of gear (no swapping), buys cures on supply trips, and keeps CP potions.

**Source.** [`pvp/model.md`](pvp/model.md). Approved 2026-11-25; amended by D43.

### D38: Bots do not hunt bots

**Decision.** Outside an arena a bot never attacks, hunts or challenges another bot, and fights one
back only after being hit. In an arena it fights anyone outside its party. Bots never duel each
other.


**Source.** [`pvp/targets.md`](pvp/targets.md). Approved 2026-11-25.

### D39: Death and karma

**Decision.** Duels are fought to the end, without surrender. Arena fights are to the death, under
a cap on deaths. In the open world a bot breaks off a losing fight and avoids its killer. A bot
never gains karma by accident and never does anything that would cancel a duel. PK mode and living
with karma are opt-in.

**Source.** [`pvp/guards.md`](pvp/guards.md), [`pvp/karma.md`](pvp/karma.md). Approved
2026-11-25.

### D40: Clan wars

**Decision.** For a `pvp` bot, an enemy in a mutual clan war is a target like a flagged player.
Bots never declare, accept, stop or surrender a war.


**Source.** [`pvp/war.md`](pvp/war.md). Approved 2026-11-26.

### D41: Sieges

**Decision.** Bots never register for a siege. Every bot walks out of a battlefield it is not
fighting in. A `pvp` bot whose clan leader registered the clan fights the other side's players,
and other bots only after they hit it.

**Source.** [`pvp/siege.md`](pvp/siege.md). Approved 2026-11-26.

### D42: The Olympiad is opt-in

**Decision.** Olympiad play is off for every preset and enabled per bot. Such a bot becomes a
noble at the class master, registers within limits and fights whoever it is drawn against. It never
spectates, trades points or claims hero status.


**Source.** [`pvp/olympiad.md`](pvp/olympiad.md). Approved 2026-11-26.

### D43: Buying CP potions

**Decision.** This amends D37: CP potions may now be bought, but are still never sold. `pvp` bots buy
them from players' private stores. A bot named for Seven Signs member mode (one per run by default)
joins Dusk and buys them at a priest in a week its side won. Bots never contribute seal stones,
never pay the Dawn fee and never vote for the Seal of Strife.

**Why.** No shop or monster provides CP potions. Only the Seven Signs priests sell them, to members
of the winning side, in the week after the competition. Seven Signs votes affect the seals for the
whole server, so only bots the owner names take part.

**Source.** [`pvp/cp-potions.md`](pvp/cp-potions.md). Approved 2026-11-26.

## Robustness and code structure

### D45: An undecodable packet is skipped

**Decision.** When a bot receives a packet it cannot decode, it logs an error, skips the packet and
plays on. Before the character is in the world, where nothing can be skipped, the attempt ends with
the error logged. Either way it never counts as the server aborting the session.

**Why.** Frames are length-delimited and decrypted whole before decoding, so skipping a bad one
leaves the stream and the cipher in step: nothing is lost by playing on. A decoding bug in the bot
is not the server's fault and must not look like one.

**Source.** [`structure.md`](structure.md), HD4. Approved 2026-11-26.

### D46: The wire code has its own package

**Decision.** Packet encoding and decoding lives in its own package, `internal/botwire`.

**Why.** The compiler then enforces the boundary between the protocol and the bot's
decision-making: the wire code cannot depend on the brain.

**Source.** [`structure.md`](structure.md), Design 5. Approved 2026-11-26.

### D47: An optional outcome log

**Decision.** Bots can write what they measure at farming spots (experience, adena, deaths, kill
times against what the model predicted) to a size-bounded JSON Lines file. It is off unless
`-outcome-log` is given, and holds no names and no chat.

**Why.** The bots' estimates of fights and spots can only be improved against real results. The
data is kept free of anything personal.

**Source.** [`structure.md`](structure.md), Design 6. Approved 2026-11-26.

### D48: Bots are not disguised

**Decision.** Bots do not add human-looking random delays. A small language model for answering
whispers is a backlog idea, not scheduled.

**Why.** The bots are not meant to pass for people (see D7).

**Source.** [`structure.md`](structure.md). Approved 2026-11-26.

## Engineering principles

These standing rules come from the plan and [`CLAUDE.md`](../CLAUDE.md). They are the decisions
behind how the code is written.

- **The server is the reference.** Packet layouts, field order, send order, and what the server
  accepts, rejects and rate-limits come from reading the server's code, not from client lore.
- **Missing server behavior is a server issue.** If a feature needs something the server does not
  do yet, the bot defers the feature and the gap is filed against the server. The bot never works
  around it, and the server never special-cases bots.
- **Safety beats speed.** Protocol accuracy, freedom from data races, and never getting the host's IP
  banned all come before speed, elegance or schedule.
- **One owner for everything shared.** Every piece of shared state belongs to one goroutine or one
  named lock. Every goroutine has a way to stop. Each connection has exactly one writer.
- **Time is injected.** Decision-making code takes a clock as input, so tests never wait on real
  time.
- **No speculative design.** Concrete types first. An interface appears only when a second
  implementation or a real test needs it, and it is defined where it is used.
- **Every sender has a limit.** Code that sends packets names the server limit that bounds it.
- **Secrets stay secret.** Passwords, the password secret and database credentials never appear in
  code, flags, logs, issues or test data.

## Amendments

| Earlier decision | Changed by | Change |
| --- | --- | --- |
| D3, only GMs command | D20, D28, D36 | Specific whispers opened to every player. |
| D37, CP potions never bought | D43 | CP potions may be bought, still never sold. |
| Plan rule: plain JavaScript panel | D12 | React and TypeScript, built with Node. |
| Plan rule: bots never speak on Trade | D30 | Merchants advertise valuables on Trade. |
| Plan rule: fixed-price trades | D32 | Valuables are haggled between an ask and a floor. |
| Plan rule: shared 300-1200 ms reaction time | D35 | PvP events get a faster 200-600 ms reaction. |
| `town` idles in town | D44 | `town` alternates between town and farming. |
| Class strategy S13c | D49 | Skill reagents are bought, and a skill is never cast without one. |
| D13a: four walking presets | Later presets | Every preset now walks. |

## Open questions

These are proposed and waiting for the owner. Nothing is filed for them yet.

| Question | Options | Proposal |
| --- | --- | --- |
| Prototype learned choices once the outcome log has data (SD4) | A fight estimate that corrects itself (OP1); spot choice by measured rate (OP2); either, both or neither | Both, behind flags |
| Restructure the brain's send tracking (SD5, BR1b) | Do it in one change across the package, or fix the two known problems one at a time | The owner's call |

See [`structure.md`](structure.md), "Owner decisions".
