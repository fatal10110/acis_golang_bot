# Panel and supervisor calls

Ships in P7. Party mode and pins: [organizer](organizer.md#pinned-parties).

- **Supervisor:** `Parties(ctx)` returns each party: id, leader, members (bots and players, from
  the leader's snapshot), the leader's spot, state and reason, since when, pinned. The leader's
  snapshot carries member names only (`Status.Members []string`, `status.go:80-83`, filled with
  `m.Name`, `status.go:155-159`), so a bot member's class, level and role come from its own
  snapshot and registry row (matched by the record's exact names), and a player member is a name
  only. `FormParty(ctx, leader, members, pin)`, `DissolveParty(ctx, id)`, `SetPartyMode(ctx,
  account, mode)` and the organizer's on/off switch go through `request` like the others.
- **`FormParty`:** refuses a bot offline, in a party the organizer did not form, set `off`, or
  with a preset other than `farmer` and `support` (D26). It takes each
  named bot out of any other record as a drop without cool-down (that record breaks when the bot
  led it or was its last bot member, a pin's rows rewritten as [pins](organizer.md#pinned-parties)
  say), and the new record orders such a bot only once that `unparty` was answered nil and its
  snapshot shows no party, so the player-led-party rule never clears the new slot. With `pin` false
  the record is an auto party: band, composition and every break apply, and the switch off
  dissolves it.
- **Auto switch:** governs auto parties only. Off: the next pass forms no auto party and dissolves
  every auto party as `DissolveParty` does (an `unparty` to each member, the leader first, one
  order at a time by the `Sent` rule), so no leader keeps re-inviting declining members; pinned
  parties are kept. On: auto parties form from the next pass, each freed bot after its `Cooldown`.
- **API:** `GET /api/parties`, `POST /api/parties`, `DELETE /api/parties/{id}`,
  `POST /api/bots/{account}/party`, `POST /api/parties/auto`.
- **Page:** a Parties page (each party with its bot members' class, level and role and its player
  members' names, the spot, its state; the free pool by town; form by hand, dissolve, auto on or
  off), and the party slot on the Bots table and the Bot detail.
- **Metrics:** parties formed and broken by reason, members invited and refused.

## Tests

- `TestSupervisorPartyCalls` (unit, injected clock, fake runs, `-race` in CI): `FormParty` writes a
  record into the `State` and the next pass orders it; a member of an auto party leaves that
  record, gets `unparty`, then `join` once partyless, and keeps its new slot; a bot in a player's
  party, a `crafter` or an `off` bot is refused; with `pin` false a spread of 6 breaks it; `DissolveParty` breaks it (a pin's
  rows cleared); `SetPartyMode` `off` drops the bot; the switch off forms no auto party, orders
  `unparty` to every member of each auto party (the leader first), so no leader keeps its `lead`
  slot, and keeps pinned ones; switched on, a freed bot is grouped only after its `Cooldown`.
- Panel (unit, `httptest`): the party endpoints through the supervisor; no secret in any response.
  Vitest: the Parties page and the form dialog.
