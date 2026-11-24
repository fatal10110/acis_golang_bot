# e2e scenarios

P8, at most 4 bots a scenario. The harness runs no supervisor (`internal/e2e/live.go` uses
`botregistry` only for `PanelPrefix`, lines 12 and 43): a scenario hands each member `join <leader>
<others>` through `Bot.Command`, waits for each nil answer, then the leader `lead <members>`
([organizer](organizer.md#formation-order)), as scenarios hand panel orders
(`arrows_live_test.go:125`). The harness keeps naming its players with `DefaultMarker`
(`helpers_live_test.go:34`): no party rule reads the marker, so the existing scenarios where such
a player invites a bot expecting 1 (`party_live_test.go:56`, `support_live_test.go:304`,
`recover_live_test.go:116-122`) pass unchanged, and P1a runs them as its regression check.

## Tests

- `TestLiveBotPartyForms` (3 bots): a farmer and two supports of one town form a party; the members
  follow; after the leader's kills every member within party range gains exp (its `characters` exp
  row grows). Whether a member hit the monster is not asserted: the server pays exp in range,
  attacker or not.
- `TestLiveBotPartyLeaderDeath` (3 bots): after the GM's `//kill` of the leader the same party farms
  together again within a timeout.
- `TestLiveBotPartyRaise` (3 bots; needs P2 and S8, #328): a human mystic support set to 20 by
  `//set level` becomes a Cleric through M10's class change at 20 (the harness's GM forms set
  level, SP and skills, not the class, `internal/e2e/gm.go:152-154`), walking to the class master
  in its bot party under P2's lifted errand gate. `//set level` gives no SP and 1016 costs 3300 SP,
  so the GM gives it with `//skill set 1016 1` (P3's learn table is unit-tested); the Cleric raises
  the killed leader.
- `TestLiveBotPartyPlayers` (2 bots, the harness's GM and a plain player whose name carries the
  marker): the player's `join` gets an invitation all the same; the GM's `invite me` takes a member
  out and it whispers back.
