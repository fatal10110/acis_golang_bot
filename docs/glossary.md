# Glossary

Terms used across the documentation, in alphabetical order within each group. Game terms follow
the Lineage II Interlude client the server speaks to.

## The project

**aCis.** An open-source Lineage II server emulator written in Java. acis_golang is a Go port of
it.

**acis_golang.** The game server these bots play on
([fatal10110/acis_golang](https://github.com/fatal10110/acis_golang)). It is the reference for
every protocol and game-behavior question.

**Bot.** One account playing one character, run by this project.

**Brain.** The part of a bot that decides what to do next. It keeps a model of the world, runs the
persona's rules and sends packets.

**Datapack.** The server's XML game data: monsters, items, skills, shops, spawn points, zones,
doors. The bots read a local copy (`-datapack`).

**Geodata.** The server's map data: for each small cell of the world, how high the ground is and
which directions a character can walk. The bots use their own copy, packed into a cache.

**GM.** Game Master: a character with an administrator access level. In this project, the only
kind of character that may command bots (with a few exceptions, see
[decisions](decisions.md#who-can-command-a-bot)).

**Milestone (M0-M14).** A group of issues delivering one feature area, each designed in a document
first. See [`docs/README.md`](README.md#design-documents).

**Nav cache.** The route hierarchy built from the geodata by `bots navbuild`, which walking bots
plan their routes on.

**Owner decision (D1-D49).** A numbered product decision made by the project's owner. See the
[decision record](decisions.md).

**Persona, preset.** A persona is the set of rules and options that make a bot behave a certain
way. A preset is a named persona you can pick: `farmer`, `support`, `town`, `merchant`, `crafter`,
`pvp`.

**Role.** What a bot does in a group: Damage (deals damage), Support (heals, buffs, tanks) or
Crafter.

**Swarm.** A fixed set of bots, all with one preset, run by `bots swarm`.

## Connections and safety

**Accept filter.** The servers' protection against connection floods. An IP that opens four
connections less than a second apart is banned for a while. The bots space their logins to stay
under it.

**Allowlist.** Addresses the accept filter never bans (`AcceptFilterAllowlist`). Bots that dial
from an allowlisted address may run `-allowlisted`, which turns off login spacing.

**Credential breaker.** The bots' rule that the first refused password stops every further login
of the process. Three refused logins in a row would get the IP banned.

**Frame, packet, opcode.** A packet is one message between client and server. On the wire it is
sent as a length-prefixed frame. Its first byte, the opcode, says which packet it is.

**Login gate.** The process-wide queue that lets one bot log in at a time and spaces the logins.

**Login server, game server.** A client first logs in to the login server, which checks the
password and lists the game servers. It then connects to the chosen game server to play.

**Pacing.** The bots' limits on how often they send packets, kept under the server's flood and
anti-spam limits.

**Secret file.** A file only its owner can read, holding the key every bot password is derived from.

## The game

**Adena.** The game's money.

**Arena.** A zone where players fight without penalties for dying.

**Bypass.** A link in an NPC's dialog window. Clicking one sends a bypass packet to the server.

**Class change, profession.** At levels 20 and 40 a character picks a profession that decides its
skills, at the class master NPC on this server.

**Clan, clan warehouse.** A player guild. Its warehouse is shared storage that members with the
right privilege may withdraw from.

**Clan war.** A declared feud between two clans. Members of clans at war with each other may fight
without penalty.

**CP, HP, MP.** Combat Points (a shield that absorbs damage from players), Hit Points, and Mana
Points (spent on skills).

**Duel.** A consensual fight between two players (or parties), with nothing lost.

**Flagged.** A player who attacked another player and may be attacked back without penalty for a
while.

**Gatekeeper.** An NPC that teleports characters between towns and areas for adena.

**Grade (no-grade, D, C, B, A, S).** The tier of an item. Shots must match the weapon's grade.

**Karma, PK.** Killing a player who is not flagged (a PK, "player kill") gives the killer karma: a
penalty that makes them a target and risks item loss on death.

**Multisell, buylist.** The two kinds of NPC shop. A buylist sells for adena. A multisell exchanges
items for other items.

**Noblesse, Olympiad, hero.** The Olympiad is a ranked tournament for noble characters. Its best
player of each class becomes a hero.

**Party.** A group of up to nine players who share experience and loot.

**Private store.** A shop a player sets up by sitting down in town, selling (or buying) items at
their own prices.

**Scroll of Escape.** An item that teleports its user to the nearest town.

**Seven Signs.** A server-wide two-week competition between two sides, Dawn and Dusk. Members
collect seal stones, and the winning side's members get access to special shops.

**Siege.** A scheduled battle over a castle between registered clans.

**Soulshot, spiritshot.** Consumables that boost attacks (soulshots) or spells (spiritshots).

**Spawn, spot.** A spawn is a place where monsters appear. A spot is a farming area built from a
spawn, with a cap on how many bots may share it.

**Spoil, sweep.** A skill that marks a monster so its killer can collect extra materials from the
corpse.

**Trade channel, Shout.** Chat channels that reach a whole region rather than only nearby players.

**Whisper.** A private message to one character. GMs command bots by whispering to them.
