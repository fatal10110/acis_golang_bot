# Bot parties (M11)

Status: owner approved D25-D29 as proposed on 2026-11-25 (`docs/plan.md`, "Owner decisions").
Milestone M11 ("Bot parties"): epic #371, issues #373-#392.

## Paths

Server paths (`network/`, `party/`, `data/`, `model/`, `skill/`, `handler/`, `trade/`,
`inventory/`, `duel/`) are under `internal/gameserver/` in
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang); `cmd/gameserver/` is at its
root.
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

Out of scope and follow-ups: [risks and gaps](#risks-and-gaps).

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
  default in `bots ui`, with a panel switch. `bots swarm` runs the same planner over its own bots
  ([organizer](organizer.md#swarm-wiring), #395, owner 2026-11-26).
- **D26, composition.** A Damage-role bot (`farmer`) leads; Support-role bots (`support`) join, at
  least one healer ([composition](composition.md#healer)); a third is any other Support bot, else a
  second Damage bot with no healer to lead with. Size 3, at most 4 (persona defaults). `merchant`, `crafter` and `town`
  never take part; party mode `off` keeps a bot out; the panel may pin a party by hand.
- **D27, loot and band.** Loot by turn (rule 3): items rotate, the server splits adena in range, a
  spoil stays with its sweeper. A party forms with its bot members within 3 levels of each other
  and breaks past 5 ([why](composition.md#size-band-location)). Players are not counted.
- **D28, players.** Any player may whisper `join` to a bot party's leader to be invited, and
  `invite me` to a Support member to take it out (extends D3 as D20 did, for these two whispers to
  bots of bot parties only). `invite me` still passes the authority check (with none configured
  every sender takes the player path), and a player's goes ahead only when no other bot holds that
  player in the claim table. A GM's `invite me` always takes the bot out, and a GM's `leave` to the
  leader disperses the party; each clears the bot's slot: a pin never re-forms, an auto party's
  bots regroup only after `Cooldown`.
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
| the organizer and the party mode per bot | 8, presets | P5b |
| the bot also invites "players who whisper `join`", and `invite me` "from a player takes a Support member of a bot party out, under the claim table, and is ignored by any other bot" | 6, commands | P6 |

## Delivery (milestone M11)

Each scope links the sections its PR builds, which are also its reading list; plan rules land as
the table above says.

| PR | Scope | Depends on |
| --- | --- | --- |
| P1a | [commands](orders.md#commands), [status fields](orders.md#status-fields) but `Heals`, [unparty](orders.md#unparty) with the `withdraw` flag it sets ([withdrawal](orders.md#withdrawal-and-reply-in-order)), [joining](members.md#joining) but the spot release, the watch reset of [clearing](members.md#clearing-from-no-party) for `unparty`; the marked-player scenarios of [e2e](e2e.md) stay green. Unit tests only. | — |
| P1b | [slot clearing](orders.md#slot-clearing) and its step setting `withdraw`, [clearing from no party](members.md#clearing-from-no-party), the slot kept for a [handover](leader.md#handover). Unit tests only. | P1a |
| P1c | [join hold](members.md#join-hold), the spot release of [joining](members.md#joining). Unit tests only. | P1a, P1b (its test's clears from no party) |
| P1d | [leader](leader.md), its part of the [join hold](members.md#join-hold), [pacing](server.md#pacing). | P1a, P1b, P1c |
| P2a | [spots](spots.md). | P1d |
| P2b | the trip causes of [events](members.md#events), [errands](members.md#errands-in-a-bot-party). | P1d |
| P3 | the `raise` rule of [events](members.md#events), 1016 in the Cleric, Elven Oracle and Shillien Oracle learn tables. | P1a, S7 (#327) |
| P4 | [model](model.md). | P2a, S3 (#322), S11 (#330) |
| P5a | the planner: [organizer](organizer.md) but its P5b parts, [composition](composition.md). | P1a, S4 (#323), S7 (#327, the healer flag) |
| P5b | [supervisor wiring](organizer.md#supervisor-wiring), [`Sent`](organizer.md#which-orders-arrived) polling, registry fields and pin rows ([pins](organizer.md#pinned-parties)), `Status.Heals` ([healer](composition.md#healer)). | P5a, P1b-P1d, S9 (#331, the `farmer` preset) |
| P6 | [players](players.md), on [invitations](leader.md#invitations), [handover](leader.md#handover) and [withdrawal](orders.md#withdrawal-and-reply-in-order). | P1d, S4 (#323, roles) |
| P7 | [panel](panel.md), on [pins](organizer.md#pinned-parties) and the [`Sent` rule](organizer.md#which-orders-arrived). | P5b |
| P8 | [e2e](e2e.md). | each; the raise scenario also P2b (the errand gate) and S8 (#328) |

Most urgent: P1a-P1d and P5a-P5b give parties at all; P2a-P2b make them play as a group.

## Risks and gaps

- **Exp per member:** three equal members each get 1.39 / 3 = 46 % of a solo kill's exp, so a party
  gains only killing over twice as fast or resting less (the healer, P4's spots); no test measures
  it; without P4 parties may level slower than solo farmers.
- **Fewer bots for players** (`join` and D28 give them back); **churn**, bounded by cool-downs,
  hysteresis, one invitation at a time and `FormPerPass`; **members left behind** by a straying
  leader, bounded by the gather gate at the gatekeeper, the regroup after the teleport and the
  fall-behind break ([gatekeeper trip](spots.md#gatekeeper-trip), #397), a member short of the fee
  leaving the party; a band with no fitting spot in reach or by gatekeeper idling until `NoSpotFor`
  frees the leader; **model error** from uncalibrated member profiles, the
  model only comparing spots under the leader's row margin.

Out of scope: command channels; party matching rooms (`network/partymatch.go`) listing bot parties
(backlog). A partyless support bot does ask to enter the rooms players open, and waits there for the
leader's invitation ([rooms](rooms.md), #438). To be filed in M11 with the epic: `bots swarm` gets the organizer (#395, shipped:
[swarm wiring](organizer.md#swarm-wiring)); a party travelling by gatekeeper or
recalling together (#397, shipped: [gatekeeper trip](spots.md#gatekeeper-trip)); measuring a bot party's
exp per hour against a solo farmer of the same level on the home run once P4 lands.

## Files

[server](server.md): server and datapack facts, the bot today, pacing. [organizer](organizer.md):
`Plan`, `State`, `Sent`, formation, eligibility, drops, breaks, pins. [composition](composition.md):
slots, healer, band, grouping. [orders](orders.md): commands, status fields, slot clearing,
`withdraw`. [leader](leader.md): invitations, oust, handover. [members](members.md): joining,
events, errands, join hold. [spots](spots.md): claim weight, beside, release, crowd, fit, misfit,
trips, gather gate. [model](model.md): party danger. [players](players.md): `join`, handover to a
player, `invite me`. [rooms](rooms.md): entering the party-matching rooms players open.
[panel](panel.md): supervisor calls, API, page. [e2e](e2e.md): live scenarios.
