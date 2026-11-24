# Bot parties (M11)

Status: draft for owner review; proposed decisions D25-D29 open. Milestone M11 ("Bot parties"):
epic and issues to be filed after the owner's answers.

## Paths

Server paths (`network/`, `party/`, `data/`, `model/`, `skill/`, `handler/`, `trade/`,
`inventory/`, `duel/`) are under `internal/gameserver/` in
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang), as is `cmd/gameserver/`.
Datapack paths (`classes/`, `skills/`) are under `aCis_datapack/data/xml`; `players.properties` is
`aCis_gameserver/config/players.properties` (same defaults in Go). Other paths are this
repository's; bare file names are mostly `internal/botclient` (`supervisor.go`, `registry.go`:
`internal/botregistry`; `names.go`: `internal/botnames`). M10 ([strategy](../strategy.md), epic
#319): roles, `charmodel` and `farmer` are assumed (D21-D24, `docs/plan.md:68-71`, PR #346, merged).

## Problem and goal

The owner asked: "Bots party - organize bots in parties, by location or by the bot manager, what is
easier." No bot leads today, so bots never party together ([today](server.md#the-bot-today)).
Goal: online bots of a similar level near one another form small parties, a Damage leader with
Support members, farming like a player group: the leader picks a spot the party fits (levels, M10
model) and claims room for all; members follow, assist, heal, buff and raise; parties regroup after
a death, logout or trip, and break and re-form; players can still get a bot, a GM always wins; the
panel shows, forms and dissolves parties.

Out of scope: command channels, party matching rooms (`network/partymatch.go`), a party travelling
by gatekeeper, and `bots swarm` (headless runs get no organizer; the home run uses the panel, D10).

## Approach

By the manager: in game a bot sees only what is in sight (`CharInfo` has no level or role) and lone
leaders would race and need bot-to-bot chat (plan section 4), while the supervisor knows every
bot's preset, class, level, position, town, party and state (`Status.Live`, `supervisor.go` `List`)
and decides in one goroutine. The invite has no distance check (`network/party.go:41-112`) and
positions reach members at any distance (`network/party_channel.go:161,207-218`), so location is
only a grouping key. The code: a pure planner on the supervisor goroutine, three panel commands.

## Owner decisions (proposed)

- **D25, organization.** By the panel's supervisor, from the registry and the live snapshots:
  online bots of the same town within walking reach and a level band; no in-game discovery. On by
  default in `bots ui`, with a panel switch; no organizer in `bots swarm`.
- **D26, composition.** A Damage-role bot (`farmer`) leads; Support-role bots (`support`) join, at
  least one healer (its kit heals another member at its level and it learned the heal: human,
  elven and dark mystics from 7; Cleric, Elven Oracle, Shillien Oracle, Orc Shaman from 20 with its
  party heal; not an orc mystic); a third is any other Support bot, else a second Damage bot with
  no healer to lead with. Size 3, at most 4 (persona defaults). `merchant`, `crafter` and `town`
  never take part; party mode `off` keeps a bot out; the panel may pin a party by hand.
- **D27, loot and band.** Loot by turn (rule 3): items rotate, the server splits adena in range, a
  spoil stays with its sweeper. A party forms with its bot members within 3 levels of each other
  and breaks past 5, the widest spread one spot fits under today's spot policy and the server's
  5-level exp threshold; the 2 between are hysteresis. Players are not counted.
- **D28, players.** Any player may whisper `join` to a bot party's leader to be invited, and
  `invite me` to a Support member to take it out (extends D3 as D20 did, for these two whispers to
  bots of bot parties only). `invite me` still passes the authority check (with none configured
  every sender takes the player path), and a player's goes ahead only when no other bot holds that
  player in the claim table. A GM's `invite me` always takes the bot out, and a GM's `leave` to the
  leader disperses the party; each clears the bot's slot, so the party does not re-form.
- **D29, bots invite.** A bot invites only the bots the organizer or the panel assigned to its
  party, and players who whisper `join`; never unasked. Members join outside the claim table. Bots
  of a bot party never whisper each other.

## Plan rule changes

`docs/plan.md` outranks the code (`CLAUDE.md` section 2), so each lands with its behavior (sections
3, `:257`, and 6, `:431`); an M11 row with D25-D29 is added with the epic.

| Rule change | `docs/plan.md` section | PR |
| --- | --- | --- |
| "at most one support bot per non-GM leader" gains "a bot party's members are not counted: they join their assigned bot leader outside the claim table" | 3, support | P1a |
| the `lead`, `join` and `unparty` commands (`lead` and `join` kept by the `Bot` like `stay` and `go`, `unparty` one-off like `leave`) | 8, operator commands and standing orders (`docs/plan.md:541-554`) and the mailbox's Commands bullet (`:577`) | P1a |
| a GM's `invite me` and `leave` taking the bot out of a bot party and clearing its slot first | 6, commands | P1b |
| "`invite me` (the bot invites nobody ...)" becomes "a bot invites only the bots the organizer or the panel assigned to its party; it never invites unasked" | 6, commands | P1d |
| the organizer and the party mode per bot | 8, presets | P5 |
| the bot also invites "players who whisper `join`", and `invite me` "from a player takes a Support member of a bot party out, under the claim table, and is ignored by any other bot" | 6, commands | P6 |

## Delivery (milestone M11)

| PR | Scope | Depends on | Reads |
| --- | --- | --- | --- |
| P1a | Party orders and the slot: `lead`, `join`, `unparty` and their `Command.valid` forms, the slot and known bot leaders, `Status.Party` (published before the order is answered), `Whispered`, `PartyLeft`, `ErrWhispered`, replacing a standing order, the raw route refusing the kinds, `unparty` from the bot party only, resetting the leader watch; members accepting their leader outside the claim table and declining known bot leaders by name, never by marker (existing marked-player e2e scenarios stay green); plan rules. Unit tests only. | — | [orders](orders.md), [members](members.md#joining) |
| P1b | Slot clearing: the `withdraw` step; panel `leave`, panel standing orders, whispered orders, a GM's `leave` and `invite me` clearing the slot; the player-led-party rule with its exception; `PartyLeft` `left`; leader-watch reset on clears from no party; plan rule. Unit tests only. | P1a | [orders](orders.md), [members](members.md#clearing-from-no-party), [leader](leader.md#handover) |
| P1c | The member's hold: skipping the lost-leader hold, whisper and `/unstuck`; the join hold with `JoinWait`, `PartyLeft` `timeout` and the watch reset; releasing the spot on joining. Unit tests only. | P1a, P1b (its test's clears from no party) | [members](members.md) |
| P1d | The leader: the `party` rule (refusals, declines, `InviteWait`, 164, rejoin wait), five loot rules, the oust and change-leader encoders, the handover with `HandoverWait`, `HandoverFor`, the join hold and `unparty`; plan rule. | P1a, P1b (the handover's exception), P1c (members hold between invitations) | [leader](leader.md), [members](members.md#join-hold), [orders](orders.md#unparty), [server](server.md#pacing) |
| P2 | Party play: weighted claim, beside and release, `reweigh`, the crowd check, band fit, `misfit`, the gated trip's give-back, weight back to 1, errand walks, the gather gate, no gatekeeper trips while leading, the member's answer to its leader's teleport and its own supply causes (the whisper skip ships in P1c). | P1d | [spots](spots.md), [members](members.md#events) and [errands](members.md#errands-in-a-bot-party) |
| P3 | Resurrection: 1016 in the Cleric, Elven Oracle and Shillien Oracle learn tables and the `raise` rule. | P1a, S7 (#327) | [members](members.md#events) |
| P4 | Party danger through the model: party `TimeToKill` and `TimeToDie` with heals; the spot fit for a party. | P2, S3 (#322), S11 (#330) | [model](model.md), [spots](spots.md#fit-by-level) |
| P5 | The organizer: planner, supervisor tick and orders, party mode and pins, roles, healer flag and `Status.Heals`, `NewRun`, `State` and its timers, drops, `Sent` and slot re-send, `join` before `lead`, breaks (`NoSpotFor` among them) and cool-downs, pins kept through every timer and held back while their leader is offline, leaving a pin by `PartyLeft`, pin rows written back; plan rule. | P1a-P1d, S4 (#323), S7 (#327, the healer flag), S9 (#331, the `farmer` preset) | [organizer](organizer.md), [composition](composition.md) |
| P6 | Players and GMs: the `join` whisper and queue, the `JoinReplies` cap, a player's `invite me` (refused verdict and no-authority routes, claim lookup, pending claim), the handover to a player as a new step; plan rules. "Support member" is S4's role. | P1d, S4 (#323, roles) | [players](players.md), [leader](leader.md#invitations) and [handover](leader.md#handover) |
| P7 | Panel: `FormParty`, `DissolveParty`, `SetPartyMode`, the auto switch (off dissolves every auto party), the API, the Parties page, the slot on the Bots table and Bot detail, metrics. | P5 | [panel](panel.md) |
| P8 | e2e live scenarios for P1a-P7, at most 4 bots each; the harness keeps its marked player names. | each; the raise scenario also P2 (the errand gate) and S8 (#328) | [e2e](e2e.md) |

Most urgent: P1a-P1d and P5 give parties at all; P2 makes them play as a group.

## Risks and gaps

- **Exp per member:** three equal members each get 1.39 / 3 = 46 % of a solo kill's exp, so a party
  gains only killing over twice as fast or resting less (the healer, P4's spots); no test measures
  it; without P4 parties may level slower than solo farmers.
- **Fewer bots for players** (`join` and D28 give them back); **churn**, bounded by cool-downs,
  hysteresis, one invitation at a time and `FormPerPass`; **walking members** stranded by a straying
  leader, bounded by no gatekeeper trips and the fall-behind break, a band that outgrew its town
  idling until `NoSpotFor` frees the leader; **model error** from uncalibrated member profiles, the
  model only comparing spots under the leader's row margin.

To be filed in M11 with the epic: `bots swarm` has no organizer (D25; headless runs keep today's party behavior); a party travelling by
gatekeeper or recalling together (P2 covers only the teleport answer; until then a party with no
spot in reach, a given-back outgrown spot included, is broken at `NoSpotFor`, P5); party matching
rooms (`network/partymatch.go`) listing bot parties: backlog; command channels: out of scope;
measuring a bot party's exp per hour against a solo farmer of the same level on the home run once P4 lands.

## Files

[server](server.md): server and datapack facts, the bot today, pacing. [organizer](organizer.md):
`Plan`, `State`, `Sent`, formation, eligibility, drops, breaks, pins. [composition](composition.md):
slots, healer, band, grouping. [orders](orders.md): commands, status fields, slot clearing,
`withdraw`. [leader](leader.md): invitations, oust, handover. [members](members.md): joining,
events, errands, join hold. [spots](spots.md): claim weight, beside, release, crowd, fit, misfit,
trips, gather gate. [model](model.md): party danger. [players](players.md): `join`, handover to a
player, `invite me`. [panel](panel.md): supervisor calls, API, page. [e2e](e2e.md): live scenarios.
