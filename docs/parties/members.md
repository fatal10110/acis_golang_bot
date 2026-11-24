# The members and keeping the party together

A bot with a `join` slot ([orders](orders.md)). Joining ships in P1a, the join hold and the spot
release in P1c, trips and errands in P2b, the raise in P3.

## Joining

`rulePrompt` (`rules.go:584-587`) accepts an invitation from the leader its `join` slot names before
the persona's `JoinParty`, so the claim table neither limits nor records it (D29). It declines one
from any bot leader a `join` slot of this run named other than the current one, slot set or not
(names kept beside the slot in `botLink`, owned by the brain goroutine). Every bot leader that can
invite it is one it knows (each listed bot gets its `join` first). A new run knows no leader; its
old leader's invitation goes to the persona, which can only put it back where the organizer
re-orders.

The bot marker plays no part: `botnames.Marks` (`names.go:66-79`) is a case-insensitive suffix or
prefix match that takes a player named Talbot for a bot (default `Bot` suffix, `names.go:51`). Every
other invitation goes to the persona and the claim table as today, and accepting it clears the
slot, `join` or `lead` (players first, D28; [orders](orders.md#slot-clearing)).

On joining any party it does not lead, a bot gives back its spot or beside count (`leaveSpot`,
`spots.go:601-610`), which fixes today's member keeping an unused spot. Play is today's follow,
assist, heals and buffs; under rule 3 every member may pick up loot.

## Events

| Event | What the bots do |
| --- | --- |
| A member dies | A Support member with Resurrection (Cleric, Elven Oracle, Shillien Oracle, from 20) casts it out of combat on a dead member in sight, the leader first (a new rule `raise` after `heal`; 1016 joins their learn tables, P3). The dead bot accepts (`AcceptResurrection`) within `ResurrectWait` (2 min in a party). Unraised, it restarts in the village and follows the leader's reported position back. |
| The leader dies | Members sit by the body as today (`byLeaderBody`), and a raiser raises it. Unraised, the leader restarts in the village (a teleport). |
| The leader teleports away (a restart, its supply trip) | The member skips `ruleLeader`'s wait, whisper and `/unstuck`, and starts its own supply trip when it has a Scroll of Escape and nothing attacks it, else walks to the leader's last reported position. `supplyDue` (`supplytrip.go:322-324`) needs a spot farmed, held or beside (`farming()`, `spots.go:229`), and reached, so P2b adds a trip cause, the bot leader's teleport, for a `join` member with no spot, skipping that test; the trip is today's. The gather gate holds the leader at its next spot. |
| A member runs low on its own | P2b reads `supplyDue`'s own causes (`supplytrip.go:19-33`: a consumable running out, potions gone, load, gear) for a `join` bot in its bot party within `GatherRange` (1200) of its leader's reported position (`Party.Members`, `follow.go:154-161`) instead of at a reached spot. `supplyCheck`, `supplyTripGap` and the trip are today's. It keeps its slot and follows back by the window's positions; the gather gate waits for it at most `GatherWait`, not beyond `FarAway`; a trip that keeps it out of party range past `BehindFor` drops it as fallen behind. |
| A member falls behind | Out of the 1500 party range for `BehindFor` (10 min): dropped, ousted, the pair banned for `RePairAfter` (30 min) ([organizer](organizer.md#breaks-and-drops)). |
| A member logs out | The server takes it out (a party of two disperses). Its slot stays with its `Bot`; the leader re-invites it, `InviteRetry` after it left at the earliest; a new run gets the slot from the organizer, which keeps its place for `LeaderGone`. |
| The leader logs out | The server hands the party to the first other member. A bot made leader of bots leaves, dispersing it; the members keep their slot in the join hold until re-invited. A party a player now leads is the player's ([players](players.md#handover-to-a-player)). After `LeaderGone` the organizer frees the members that kept their slot. |

## Errands in a bot party

Today's party gate (`trainer.go:469-477` `startTrainerWalk`; `visit.go:89-106` `pickVisit`, from
`classchange.go:212`, `dyes.go:217`) would cost a bot party its D15 class change, 1016 and 1011. P2b
lifts it for a bot in its own bot party (`lead` and leading, or `join` in the named leader's party):
both read `b.world.Party.In() && !b.inBotParty()`; the `orders.follow` gate stays. The errand rules
run before `leader` and `follow` (`rules.go:494-507`). Members follow a walking leader by the
window's positions (`follow.go:154-161`) and take their own errands in town; a member walks within
the rule's reach (8000: `TrainerReach`, `rules.go:239`; `Reach`, `classchange.go:83`, `dyes.go:77`)
and back, under the gather gate and `BehindFor`.

## Join hold

A dispersed bot party leaves members partyless and `watchLeader` marks the leader lost
(`death.go:199-223`): `ruleLeader` would hold `LeaderWait` (5 min, `death.go:164`), whisper and
`/unstuck` (`death.go:287-317`). P1c makes a `join` bot skip all three for any loss of its bot
leader (`byLeaderBody` stays), and since today's hold (`death.go:293-298`) is the only stop for a
partyless bot (`ruleFollow` passes with `followed()` 0, `follow.go:97-103`, `command.go:277-286`,
then `newbie`, `travel`, `spot`, `farm`, `rules.go:506-511`; positions are reset with `Party`,
`party.go:259-260`), adds the **join hold** in `ruleLeader` after `byLeaderBody`: while the bot
holds a `join` slot in no party (or, from P1d, runs a [handover](leader.md#handover)) it returns
`Action{}, true`, claiming nothing; the rules before `leader` (`rules.go:475-505`: `attacked`,
`heal`, `rest`, `rebuff`, the errands) still run, so a due errand walks it within its reach as in
its party. It covers a member between invitations too. It ends
with the organizer's `unparty`, or `JoinWait` (6 minutes: `LeaderGone` plus two `PartyTick`s, a
persona default) after the bot found itself partyless with the slot set: the brain clears its slot
(`PartyLeft` `timeout`). Its one real case is a pin whose leader is gone
([organizer](organizer.md#pinned-parties)); for an auto party it bounds an organizer bug, since a
supervisor restart cancels and joins every run (`supervisor.go:279-298` `shutdown`), a lost
`unparty` is resent by the `Sent` rule, and the auto switch off sends `unparty`.

## Clearing from no party

Every clear of a slot while the bot is in no party ends the join hold and resets the leader watch
(`leaderWatch{}`): an `unparty`, the `JoinWait` clear, or any [slot clear](orders.md#slot-clearing).
Else, the dispersal having set `lost` and `JoinWait` passing `LeaderWait`, the bot would whisper its
old leader and `/unstuck` at once; freed, it plays its persona where it stands. So no bot of a bot
party whispers another (`docs/plan.md:343-344`): while the slot is set `ruleLeader` skips the
lost-leader branch for the named leader, and a clear inside the party ends with the withdrawal
or the dropped `withdraw` flag, each resetting the watch
([orders](orders.md#withdrawal-and-reply-in-order)), or leaves the bot under a player. A bot with no slot keeps today's behavior.

## Tests (brain unit: events and an injected clock in, packets out)

- `TestPartyJoinAssignedLeader` (P1a): the `join` leader's `AskJoinParty` is accepted while the
  claim table holds that leader for another bot, the table unchanged; a leader an earlier `join` of
  this run named is declined, slot set or none; a player's goes to the persona, also `Talbot` or
  `Abbot`, slot or none.
- `TestPartyJoinReleasesSpot` (P1c): a member holding a spot joins: held drops; one beside: its
  beside count drops.
- `TestPartyMemberLeaderLost` (P1c, on P1a's `unparty` and P1b's clears): a dispersed `join`
  member, no spot, no errand due, a fitting spot free, a monster in sight: no whisper, `/unstuck`,
  claim, move or attack, rule `leader`; attacked, it fights back; it accepts the returning leader and follows; at
  `JoinWait` it clears (`timeout`) and plays its persona, no whisper or `/unstuck` then or 10
  minutes after; likewise after `unparty`, a panel `leave` (nil) or `follow` and a GM's `leave` in
  the hold, each at least `LeaderWait` after the dispersal; a bot with no slot behaves as today.
- `TestPartyLeaderTeleported` (P2b): a member of a teleporting bot leader: no whisper or `/unstuck`;
  no spot, a Scroll of Escape, no attacker: supply trip; no scroll: walks to the last
  `PartyMemberPosition`; a bot with no `join` slot and no spot: no trip.
- `TestPartyMemberSupply` (P2b): a Damage and a Support member, no spot, within 1200 of the leader,
  potions gone (or a consumable out, or load past `ReturnLoad`): a trip at the next `supplyCheck`,
  slot and party kept, following after; beyond 1200, within `supplyTripGap`, or no slot and no
  spot: none.
- `TestPartyErrandWalks` (P2b): a `join` member, trainer due, none in sight: walks to the nearest
  within `TrainerReach`, learns, follows again; at 20 to a class master out of sight, with dyes due
  to a dye maker; a `lead` leader likewise, members following; a bot in a player's party or told to
  follow: none out of sight.
- `TestPartyRaise` (P3): a level-20 Cleric member out of combat, the leader dead in sight, selects
  it and casts 1016; not in combat; a level-19 Elven Mystic never
  (`classes/elvenMystic.xml:266-267,362`).
