# Bot parties: bots form parties with each other and farm as a group

Status: draft for owner review. The review-fix loop ran 12 rounds; round 12's fixes are not
re-reviewed. Proposed decisions D25-D29 are open (see "Owner decisions").
Milestone M11 ("Bot parties"): epic and issues to be filed after the owner's answers ("Delivery").

Review history (each round's findings are checked against the server, the datapack or the bot code
before they are applied):

- Round 1 (10 findings, all confirmed and applied): the level band now forms at `Band` 3 and
  breaks above `MaxSpread` 5, the widest spread a spot can hold, so no party outlives every spot
  (sections 2, 9, D27, tests); every whispered command that takes a bot out of its party clears its
  party slot, a bot declines a bot's invitation its slot does not name, and a leader waits
  `InviteRetry` before re-inviting a member that left, so `invite me` and `leave` no longer re-form
  the party (sections 3, 4, 10); the handover leave applies only to a bot with a `join` slot, which
  hands leadership to a player with `RequestChangePartyLeader` instead of disbanding a party that
  holds one (section 4, Pacing, tests); the crowd check subtracts only the other bots' claims, not
  the party's own weight (section 6); a player's `invite me` path is specified: after the
  authority check, a refused verdict for that command alone, only to a Support member of a bot
  party, and only when no other bot holds that player in the claim table (section 10, D28);
  spot fit and the party model use only the bot members named by the `lead` list, not players
  (sections 6, 7); the party slot lives in a run's `Bot`, so the organizer reads it back from
  `Status.Party` (now in P1) and re-sends it to a new run (sections 1, 3, 8, Delivery); the Pacing
  row gives the bot's own frame bucket (5/s, burst 10); the healer flag needs the heal's level
  (base mystics from 7), the tank flag M10 never defined is dropped and the third member is any
  other Support bot (section 2, D26); the oust touches only bots once on the `lead` list, the
  refusal list adds the blocked, detached and jailed answers and names the silent Olympiad case,
  and the e2e exp and raise scenarios are made checkable (sections 4, Testing, Delivery P8 on S8).
- Round 2 (9 findings, all confirmed and applied): the organizer keeps each order's answer
  (`Sent`) and reads an empty slot as cleared by the bot only after a nil answer, so an order
  refused busy or queued across a reconnect is no longer taken for a bot that left, and the brain
  publishes its snapshot before it answers (section 1, tests); the slot clears whenever the bot is
  in a party a player leads, a leader's `lead` slot included, the `party` rule invites only from no
  party or its own, and `unparty` withdraws only from the bot party, never from a player's
  (sections 3, 5, 8, 10); the panel's one-off `leave` clears the slot too (section 3); a decline
  (`JoinParty` 0) starts `InviteRetry` like a refusal (section 4, Pacing, tests); section 3 names
  three commands with `unparty` one-off and gives `Command.valid` its forms for them; the healer
  flag counts the Orc Shaman from 20 (1229 Chant of Life, a party heal over time, also in the
  model) and the spot margins list glass cannon 3.0 (section 2, 7, D26); the `join` whisper is
  routed like a craft order but never reaches the authority, and a bot with no slot treats it as
  chat (section 10); e2e scenarios form parties through `Bot.Command` since the harness runs no
  organizer, and the raise scenario gives the Cleric 1016 with `//skill set` (Testing); P6 depends
  on S4 for roles (Delivery).
- Round 3 (10 findings, all confirmed and applied): `Plan` takes and returns the organizer's
  `State` (parties, freed times for `Cooldown`, pair bans for `RePairAfter`, and per party the
  no-healer, leader-gone and fallen-behind timers), so the supervisor keeps it between passes
  (section 1, tests `TestPlanState`); a player's `invite me` claim is marked pending and survives
  the `PartySmallWindowDeleteAll` that answers the bot's own withdrawal, so a second member does
  not leave for the same player (section 10, `TestPartyInviteMe`); the brain records its spot
  weight and `releaseN` and a one-step re-claim give it all back on a trip, a death, a move or a
  stop (section 6, `TestPartyClaimN`); "no bot member left" breaks a party with an `unparty` to
  the leader instead of a `lead` with no names, and an `ErrBadCommand` answer is not retried
  (sections 1, 9); a member's supply trip on its leader's teleport gets its own cause, since
  `supplyDue` needs a held spot (section 8, P2); the leader's invitation timeout is `InviteWait`
  17 s, a 2 s margin past the server's window, and a 164 is the leader's own window, not a
  member's refusal (section 4, Pacing); the raw command route keeps refusing the party kinds and a
  delivered slot that differs from the record is re-sent (sections 1, 3); a GM's whispered `stay`
  or `go` clears the slot too, and on a handover to a player the bot whose claim succeeds stays
  while every other bot, Damage included, leaves (sections 3, 10); a `join` member skips
  `ruleLeader`'s hold, whisper and `/unstuck` for any loss of its bot leader, and `unparty` clears
  the leader watch (section 8); `Plan` hands `lead` only after every member's `join` was answered
  nil, and the e2e scenarios do the same (section 1, Testing).
- Round 4 (6 findings, all confirmed and applied): bots no longer tell bots from players by the
  name marker, a suffix or prefix match that also takes a player named Talbot for a bot
  (`botnames/names.go:66-79`) and that the e2e harness's players carry: a member declines only
  the bot leaders it knows by exact name (those its own `join` slots named this run), `join`
  carries the party's other bot members so a bot made leader tells bots from players by name, and
  the existing e2e scenarios where a marked player invites a bot keep their acceptance (sections
  3-5, Testing, Delivery P1, P8); a member that goes offline or reconnects gets a per-member
  `GoneSince` timer and is dropped and replaced only after `LeaderGone`, so a short reconnect keeps
  it, while a member made ineligible on purpose (party mode `off`, a preset change, a standing or
  whispered order) is dropped at once, and a panel standing order clears a `join` slot also while
  the member is between invitations (sections 1, 3, 8, 9, `TestPlanBreaks`); the withdrawal a
  slot-clearing command needs becomes its own `orders` step sent before the one reply slot, so a
  GM's `invite me` sends the withdrawal and then the whisper (section 3, `TestPartyInviteMe`); the
  supervisor's two timers (cool-down and registry flush) are named, the party tick being a third
  (section 1); spot fit is stated as the leader's brain sees it, falling back to its own level
  when no spot fits the band, with no pin flag needed in the brain (section 6); a GM's whispered
  follow, assist, stay or go is reported in the snapshot (`Status.Whispered`) and makes `join` and
  `lead` answer `ErrWhispered`, so the organizer never assigns a bot that follows a GM (sections
  1, 3, 12).
- Round 5 (10 findings, all confirmed and applied): every reset of `orders` keeps `withdraw` as
  it keeps `reply`, the panel `go` (`mailbox.go:200`), `holdStill` (panel and whispered `stay`,
  `mailbox.go:234-237`) and `orderPlayer` (panel `follow` and `assist`, `mailbox.go:241-249`)
  alike, so a standing order to a member in its bot party withdraws it (section 3, tests);
  `LeaderGoneSince` reads a leader that is not online in the world as gone, never its stale
  snapshot, and while a party is `forming` it counts the leader's party only once its `lead` was
  delivered, so a reconnecting leader is timed and a party whose members never accept breaks
  (sections 1, 9, `TestPlanBreaks`); grouping keeps every bot member within `Band` of every other,
  not only of the leader (section 2, `TestPlanBand`); the `join` replies and the "invite me"
  whisper are command replies, bound by the chat gate as a GM's "invite me" is today, and a
  `join` sender gets at most one answer per `InviteRetry`, not `ChatPolicy.PerSender` (section 10,
  Pacing); with no authority configured a player's `invite me` takes the player path at once
  (section 10, D28); `lead` and `join` replace a panel standing order, so a hand-formed or pinned
  member no longer keeps following another player (sections 3, 12); a panel `leave` to a
  partyless bot holding a slot clears it and answers nil, `ErrNoParty` only with no slot either
  (section 3); P1 is split into P1a (orders, slot, clearing, members) and P1b (the leader), and
  P2 no longer repeats the whisper skip (Delivery); the supervisor test is split between P5 and
  P7, and the kills-per-minute comparison, which no scenario measures, moves to Gaps (Testing,
  Risks, Gaps); the raise test names an Elven Mystic under 20 (Elven Oracle starts at 20,
  `classes/elvenMystic.xml:266-267`) and the name pattern cites the server
  (`data/manager/roster.go:26`).
- Round 6 (7 findings, all confirmed and applied): a `join` member that leads its own party (the
  server made it leader, `party/party.go:337-346,418-427`) no longer clears its slot by the
  player-led-party rule, so the handover runs (sections 3, 4, `TestPartyHandoverLeaves`, P1a); a
  partyless member with a `join` slot now has a hold of its own, the join hold in `ruleLeader`,
  bounded by the organizer's `unparty` and by `JoinWait` (6 min), since skipping today's
  lost-leader hold let it fall through to `spot` and `farm` (`rules.go:491-496`) and a dispersed
  party leaves no leader position to walk to (`party.go:259-260`) (section 8,
  `TestPartyMemberLeaderLost`, P1a); a GM's whispered `leave` and `invite me` clearing the slot
  are delivered in P1a, and P6 keeps only a player's `invite me` path (`TestPartyWhisperNoReform`);
  `LeaderGoneSince` does not count a leader leading no party while every bot member is gone, so a
  party of two whose member logs out does not run both timers together, and a member dropped while
  an order is pending gets its `unparty` the first pass after the answer, through a new rule for a
  candidate in no record with a slot set (sections 1, 8, 9, `TestPlanBreaks`, P5); the citations
  for the 154 and 164 branches, the decline's `JoinParty`, the oust and withdraw handlers and the
  loot rule now point at the lines that hold them (`party/party.go:203-224,215-216`,
  `network/party.go:71-74,82-84,88-90,101-103,126,133-138`, `network/partymatch.go:169-183`); the
  composition table's leader is the highest-level Damage candidate, as Grouping orders them
  (section 2); the spot claim weight `n` is the party window's size, unchanged by a death, raise or
  trip, and an upward `reweigh` past the cap is moved by today's over-cap re-pick (`spots.go:237`)
  (sections 6, 7, `TestPartyClaimN`).
- Round 7 (8 findings, all confirmed and applied): a bot that leaves a pinned party on purpose (a
  panel `leave` or standing order after the pin, a GM's whisper, a player's `invite me` or party,
  party mode `off`, a preset change) leaves the pin and is not ordered back, its registry pin rows
  cleared and a leader's departure dissolving the pin, told from a `JoinWait` clear by the new
  `Status.PartyLeft` (`left` or `timeout`), while a pinned party is broken by none of section 9's
  timers (sections 1, 8, 9, 12, `TestPlanPinned`, P1a, P5); a player's `join` goes on a queue the
  `party` rule invites from, players first, within its one invitation in flight that records its
  invitee, since the server answers a second invitation 164 (`network/party.go:71-74,88-90`) and
  `JoinParty` names no one (`party.go:25`), and a queued player gets no `InviteRetry`
  (sections 4, 10, Pacing, `TestPartyJoinWhisper`, P6); a party whose leader holds no spot for
  `NoSpotFor` (15 min, `Party.NoSpotSince` from the snapshot's `Spot`) is broken and its pairs
  banned for `RePairAfter`, so a band that outgrew its town's walking reach does not idle forever
  without the gatekeeper trip (sections 1, 4, 9, Risks, Gaps, `TestPlanBreaks`, P5); each
  `docs/plan.md` rule change ships in the PR that brings its behavior (P1a, P1b, P5, P6), not all
  in P6 (section 11, Delivery); today's over-cap re-pick is described as it is: a held spot the bot
  is not outlevelled for is kept over cap and re-picked every `spotRetry`, never `claimOver`
  (`spots.go:493,503-505`), and the `planTrip` on that path is the trip a leader does not take
  (section 6, `TestPartyClaimN`); a member reads its own supply causes within `GatherRange` of its
  leader instead of at a held spot, so Damage and Support members keep their D16 trips
  (section 8, `TestPartyMemberSupply`, P2); the auto switch off dissolves every auto party with
  `unparty`, so no leader re-invites declining members forever, and `JoinWait` covers only a lost
  `unparty` (sections 8, 12, `TestSupervisorPartyCalls`, P7); `TestPartyWhisperedOrderClears`
  asserts the registry after `obey` and the packet order, since a `go` claims inside `obey`
  (`command.go:189-192,238`) and the withdrawal goes out later through `ruleCommand`.
- Round 8 (6 findings, all confirmed and applied): every clear of a slot while the bot is in no
  party (`JoinWait`, an `unparty`, a panel `leave` answered nil, a panel standing order, a GM's
  whispered order, a player's `invite me`) resets the leader watch, so a member whose slot clears
  at `JoinWait`, which comes after `LeaderWait` from the same dispersal (`death.go:164,195-218`),
  no longer whispers its old bot leader and casts `/unstuck` at once (`death.go:274-297`); the
  whisper skip says why the watch never holds a bot leader once the slot is gone (sections 3, 8,
  `TestPartyMemberLeaderLost`, P1b, P1c); the handover to a player is a new step of P6, since today
  a bot stays unclaimed on a leader change (`partyclaim.go:111-121`), taken only by a bot whose slot
  that leader-change window clears, through its party hook, so a commander or a GM the hook knows
  (`authority.go:274-288,296-300`) keeps every bot (section 10, `TestPartyHandoverToPlayer`, P6);
  the `withdraw` flag is set only in a party and dropped unsent from no party or on the bot's own
  invitation answer, so it never takes the bot out of a later party (section 3, Pacing,
  `TestPartyWithdrawLapses`); `JoinWait` names its one real case, a pinned party whose leader is
  gone, and is otherwise a defensive bound, a supervisor restart taking every run with it
  (`supervisor.go:276-292`) (section 8); a pinned party gets no `join` while its leader is not
  online, so its members play their persona instead of standing 6 of every 6.5 minutes (section 12,
  `TestPlanPinned`, P5); P1a is split into P1a (orders and the slot), P1b (slot clearing), P1c (the
  member's hold) and P1d (the leader, formerly P1b) (Delivery, section 11).
- Round 9 (6 findings, all confirmed and applied): P1c depends on P1b as well, since its
  `TestPartyMemberLeaderLost` clears the slot from no party by P1b's commands, and the player's
  invitation clearing a slot moves out of `TestPartyJoinAssignedLeader` (P1a) into a new
  `TestPartyJoinPlayerClears` (P1b), each test naming its PR (Testing, Delivery); the handover
  goes to the player with the lowest object id, since `World.Party.Members` is a map and keeps no
  join order (`party.go:198-206,231-250`) and the server takes any member not in a duel
  (`party/party.go:395-407`) (section 4, `TestPartyHandoverLeaves`); `join` reply whispers, which
  answer any player unchecked, get plan section 4's per-bot total as a count of their own,
  `JoinReplies` 10 an hour (section 10, Pacing, `TestPartyJoinWhisper`, P6); a listed member whose
  invitation times out at `InviteWait` is invited again only after `InviteRetry`, the next listed
  member at once (section 4, Pacing, `TestPartyLeadInviteTimeout`); the Problem table's joining
  row says only a leader the authority knows is never limited, as section 10 does
  (`authority.go:274-288,296-300`, `cmd/bots/persona.go:143-153`); the healer flag also needs the
  heal learned, through a new `Status.Heals` read from the skill list, since 1011 and 1229 are
  bought from a trainer (`classes/humanMystic.xml:50`, `classes/orcMystic.xml:250`) (sections 1,
  2, 12, D26, `TestPlanNeedsHealer`, P5).
- Round 10 (8 findings, all confirmed and applied): a leader with bot members re-checks the spot
  it holds against the band, through a new `misfit` that counts as `outleveled` does in
  `ruleSpot`'s re-pick and `claimSpot`'s keep (`spots.go:237,404-407,503`), so a solo spot kept at
  formation, a level-up or a member change no longer holds members below its fit (section 6,
  `TestPartyHeldSpotMisfit`, P2); the party gate on errand walks (`trainer.go:469-477`,
  `visit.go:89-106`) is lifted for a bot in its own bot party, so leaders and members still take
  their class changes, trainers and dyes out of sight (D15's class change, 1016 at 20, 1011 at 7
  and so `Status.Heals`), the leader's members following it to town (Problem table, sections 2, 8,
  Pacing, `TestPartyErrandWalks`, P2, the raise scenario); the `join` queue's free places subtract
  the listed bots not in the party and are read again when the invitee is picked, so players never
  fill the place of a reconnecting member into 155 refusals (`party/party.go:217-218`) (section 10,
  `TestPartyJoinWhisper`); `TestPartyWhisperNoReform` keeps its P1b part and the leader's part moves
  to `TestPartyWhisperNoReformLeader` (P1d), and `TestPartyInviteMe` splits into
  `TestPartyInviteMeGM` (P1b) and `TestPartyInviteMePlayer` (P6); a handover to a player in a duel,
  which the server refuses in silence (`party/party.go:403-405`), is retried over the players every
  `HandoverWait` under the join hold, ends in a withdrawal at `HandoverFor` past a duel's 2 minutes
  (`duel/state.go:58`), and an `unparty` meanwhile withdraws (sections 3, 4, 8, Pacing,
  `TestPartyHandoverLeaves`, P1d); the Parties page shows a player member by name only, since the
  leader's snapshot carries names only (`status.go:80-83,155-159`) (section 12); the command rule
  change cites plan section 8's operator commands, standing orders and mailbox
  (`docs/plan.md:527-537,563`) (section 11); grouping takes a third member Support first and a
  Damage candidate only when it has no healer to lead with, in two steps (section 2, D26,
  `TestPlanThirdMember`).
- Round 11 (3 findings; all confirmed and applied, one in part): section 6 now describes the spot
  code after #142 (#353): `claimSpot` keeps a held spot, then tries `claimOver` when `Overflow` is
  on, then makes a bot holding no claim farm beside a full spot (`spots.go:524-534`), counted in
  the registry's `beside`, apart from `held` (`spotregistry.go:152-176`). A leader that finds no
  room for `n` below any limit farms beside with its whole weight (`farmBesideN`), so other bots'
  crowd checks do not count its members as strangers; `spot.weight` covers a claim or a beside
  count, `leaveBesideN` and a `reweigh` of either count give it back or change it in one step, and
  `leaveSpot`'s callers are listed as they are now (five in `spots.go`). `claimOverN` takes only a
  spot whose held count plus `n` stays within its limit, the `go` claim weighs 1, and the crowd
  check subtracts `Held + Beside - spot.weight`, keeping the beside term. A misfit leader with no
  band-fitting room keeps its held spot as an outlevelled bot does, and a beside spot that leaves
  the band's fit is left at the next re-pick. A leader farming beside has a `Spot` in its snapshot
  (`status.go:139-141`), so it starts no `NoSpotSince`, by design (sections 1, 4 "Trips", 5, 6, 9,
  Problem table, `TestPartyClaimN`, `TestPartyCrowdOwnWeight`, `TestPartyJoinReleasesSpot`,
  `TestPlanBreaks`, P2). Line citations moved by #346, #353, #356 and #359 are updated in
  `docs/plan.md`, `rules.go`, `death.go`, `brain.go`, `spots.go`, `spotregistry.go`,
  `supervisor.go` and `helpers_live_test.go`, and the header records D21-D24 as merged
  (`docs/plan.md:68-71`). The `status.go` citations were not changed: `Spot` is at
  `status.go:73-75` and the offline reset at `status.go:119`, as cited. `TestPartyHeldSpotMisfit`
  now builds the fits-again case: a held spot of top monster 15 misfits `[12, 14]`, is kept while
  no band-fitting spot has room, and fits `[12, 15]` after the member's level-up to 13.
- Round 12 (4 findings, all confirmed and applied): a leader that outlevelled the spot it holds,
  or whose spot misfits the band, with no candidate in walking reach, kept that spot for good:
  `spotCands` lists neither the held spot nor any that fails the fit (`spots.go:466-491`), the
  keep is skipped but `claimOver` gets an empty order and a holder skips `farmBeside`
  (`spots.go:513-535`), so its `Spot` stayed set (`status.go:139-141`) and `NoSpotSince` never
  started; P2 adds the gated trip's give-back, which returns such a spot at the step where a solo
  bot plans its trip, so the party idles and `NoSpotFor` frees the leader, and section 6 no longer
  says a solo outlevelled bot keeps its spot when it has a trip (sections 4, 6, 9, Risks, Gaps,
  `TestPartyNoGatekeeperTrip`, `TestPartyHeldSpotMisfit`, P2); the spot weight is the window's
  size only while the bot leads its bot party, and a leader whose party ends under it
  (`PartySmallWindowDeleteAll` zeroes `World.Party`, `party.go:259-260`) or whose slot clears
  re-weighs to 1 and keeps its spot, so a freed leader no longer holds `n` of the cap, which a
  `go` to its own spot would keep (`command.go:235-236`) (section 6, `TestPartyClaimN`, P2); the
  own-level fallback is map-wide and by level only, so band-fitting spots out of walking reach
  leave the party idle until `NoSpotFor` rather than at spots too strong for its lowest member,
  and sections 4 and 9 say so (sections 4, 6, 9, `TestPartyHeldSpotMisfit`); section 8's two
  supply rows describe `supplyDue` after #142, a spot farmed, held or beside, and reached
  (`supplytrip.go:324`, `spots.go:229`).

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang); `cmd/gameserver/...` too.
Datapack paths (`classes/...`, `skills/...`) refer to `aCis_datapack/data/xml`. The server config
file named is `aCis_gameserver/config/players.properties`, whose values the Go server reads with
the same defaults. Bot paths name this repository. M10 is `docs/strategy.md` (epic #319); its
roles Support, Damage and Crafter, the `charmodel` package and the `farmer` preset are assumed
(D21-D24, recorded in `docs/plan.md:68-71` by PR #346, now merged).

The owner asked: "Bots party - organize bots in parties, by location or by the bot manager, what is
easier."

## Problem

Players party: a fighter pulls, a healer keeps it up, a buffer makes both stronger, and the group
takes spots none of them could take alone. The bots already play a full party member when a player
invites them, but no bot ever leads, so bots never party with each other.

### What the bot does today

| Concern | Today | Where |
| --- | --- | --- |
| Inviting | The brain never invites. The invite encoder exists and only the e2e harness's player uses it. A GM's `invite me` makes the bot whisper "invite me" back: "the bot invites nobody, so the GM stays its party's leader". | `internal/botclient/party.go:145` `encodeRequestJoinParty`, `harness.go` `EncodeRequestJoinParty`, `command.go:22` `inviteMeText`, `command.go:194` |
| Joining | Every persona accepts every invitation (`acceptParty`), limited to one bot per leader by the process-wide claim table, except a leader the authority knows: a commander, or a GM a database check confirmed within `knownFor` (10 minutes) or in sight under such a check, is never limited; any other GM's invitation is claimed like a player's, and with no authority configured every invitation is. | `party.go:181`, `partyclaim.go:40` `PartyClaims.Apply`, `authority.go:274-288` `knowsName`, `authority.go:296-300` `Hooks`, `cmd/bots/persona.go:143-153` `applyPartyAndAuthority` |
| Playing as a member | Follows at a slot 150-350 behind the leader, assists the leader's target 0.5-1.5 s after the leader's hit, heals and buffs the members in sight (leader, self, others). | `follow.go` `ruleFollow`, `defaultFollowPolicy`; `support.go` `leaderHit`, `ruleAssist`, `ruleHeal`, `ruleRebuff` |
| Leader edge cases | Sits by a dead leader's body; a leader that left, logged out or teleported away is waited for 5 minutes, whispered "where did you go?", then the bot casts `/unstuck` and waits in town. | `death.go` `defaultRecoverPolicy`, `ruleLeader`, `byLeaderBody` |
| Leading | A bot leads only when the server hands it a party (its leader disconnected); `watchLeader` then counts its leader as lost. | `death.go` `watchLeader` |
| Spot while a member | A bot keeps the spot it held, or the beside count it farmed with, before it joined: `ruleFollow` runs before `ruleSpot`, so it never uses it, and nothing releases it. | `rules.go:507-510`; `leaveSpot` (`spots.go:601-610`) callers in `brain.go`, `command.go`, `spots.go`, `supplytrip.go` |
| Errands while in a party | A bot in any party, its leader included, never walks to a trainer, class master or dye maker out of sight ("a party member stays with its party"); it visits one only when the NPC is already in sight. | `trainer.go:469-477` `startTrainerWalk`, `visit.go:89-106` `pickVisit` (called from `classchange.go:212`, `dyes.go:217`) |
| Loot | Under finders keepers a member, not the leader, leaves the loot alone. | `loot.go:47`, `loot.go:306` |
| What the manager knows | The registry holds each bot's preset, class, level, online flag and position; each live run's snapshot adds class, level, HP, position, nearest town, preset, current rule, the party leader's and members' names, and the panel's standing order. | `internal/botregistry/registry.go` `Bot`, `internal/botclient/status.go` `Status` (`Town`, `Leader`, `Members`, `Order`) |
| Panel | Shows each bot's party leader and members; no party view and no party command. | `status.go:80-83`, `internal/botpanel/api.go` routes |

## Goal

Online bots of a similar level near one another form small parties, each led by a Damage-role bot
with Support-role members, and farm together like a player group:

- the leader picks a spot the whole party fits, by the members' levels and by the party's strength
  through the M10 model, and claims room for the whole party there;
- the members follow, assist, heal, buff and raise the dead, and the party regroups after a death,
  a logout or a trip to town;
- parties dissolve and re-form when they break or drift apart in level;
- real players can still get a bot: a player can ask a bot party's leader to invite them, and a
  GM's command always wins;
- the panel shows the parties and can form and dissolve them by hand.

Out of scope: command channels, party matching rooms (`network/partymatch.go`), a party travelling
by gatekeeper, and `bots swarm` (headless runs get no organizer; the home run uses the panel, D10).

## Which approach

| | By location (in-game discovery) | By the manager (the panel's supervisor) |
| --- | --- | --- |
| What a bot can learn about another | Only what is in sight: `CharInfo` carries race and class but no level (`network/serverpackets/charinfo.go:61,63,146`), nothing of its preset or role, and it is sent only to players in view. | Every bot's preset (role), class, level, position, town, party and online state, from the registry and the live snapshots it already reads (`Status.Live`, `supervisor.go` `List`). |
| Who decides the leader | Each brain decides alone, so two bots would invite each other or the same third bot; avoiding that needs a process-wide table like `PartyClaims` anyway. Whispered negotiation would be bot-to-bot chat, which the plan forbids (plan section 4: never answer a name carrying the bot marker). | One goroutine decides for everyone: no race between leaders, one assignment per bot. |
| Distance | Bots meet only by chance on a shared spot. | The server's invite has no distance check (`network/party.go:41-112` `requestJoinParty`), and every member gets every member's position every 12 s wherever it is (`network/party_channel.go:161,207-218`), so a member walks to its leader. Location stays a grouping key (same town, a walk apart) for gameplay, not for the protocol. |
| Code | A discovery and negotiation layer in every brain. | A pure planner called from the supervisor goroutine, and three new panel commands the brain carries out. |

**The manager.** It knows everything the grouping needs, decides in one place, and the protocol
does not care how far apart the bots stand. Location enters as the grouping key: the planner groups
only bots of the same town within walking reach. A bot never discovers other bots in game.

## Server behavior this rests on

Read in the Go server and the datapack on 2026-11-24; every design choice below rests on these.

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Invite | `RequestJoinParty` names the target; there is no distance check. Refusals by message id: target offline or unknown 185, the target blocking everyone 940 or ignoring the inviter 619, inviting oneself or an invisible target 152, target in a party 160, the target or inviter busy with another request 153 / 164, not the leader 154, party full 155; refusals as text in message 1987 (`SystemMessageS1`): a detached (offline-mode) target, a jailed target or inviter. An inviter or target in the Olympiad gets no answer at all. On success the inviter gets 105 and the target `AskJoinParty` with the loot rule. | `network/party.go:41-112` (blocked 47-50 via `trade.go:99-108` `blockRefusal`, detached and jailed 59-65 with texts at 28-29, Olympiad 67-69); ids in `network/serverpackets/party.go:30-39`, `systemmessage.go:77,197,609,613` |
| Change leader | `RequestChangePartyLeader` (0xd0 0x0004, a name) hands the party its leader leads to the named member unless that member duels, in which case nothing changes and nothing is sent (a duel lasts at most 2 minutes, `duel/state.go:58` `Length`); on a change every member is sent the new window. | `network/clientpackets/party.go:15,77-80,122-132`, `network/party.go:140-143`, `party/party.go:395-427`, `network/client_dispatch.go:324` |
| One request at a time | The trade book holds one pending request per player for 15 s; a leader's party waits for the answer until then (`InviteWaiting`). | `party/party.go:20` `InviteTimeout`, `party/party.go:203-224` `BeginInvite`, `trade/book.go:12-13` |
| Loot rule | Chosen by a partyless inviter: 0 finders keepers, 1 random, 2 random with spoil, 3 by turn, 4 by turn with spoil; another value invites nobody. A leader's later invitations offer the party's rule, whatever they send. | `party/party.go:40-46`, `party/party.go:203-224` `BeginInvite` (a partyless requester's choice 208-212, the party's rule 223) |
| Answer | The invitee's answer goes to the inviter as `JoinParty`; acceptance forms the party or adds the member, unless the target is already in one or the party filled. | `network/party.go:117-131`, `party/party.go:245-262` |
| Size | At most 9 members. | `party/party.go:16` |
| Leaving | A party of two disperses when either leaves; a leader leaving other than by disconnecting disperses it; a disconnecting leader hands it to the first other member in join order. | `party/party.go:335-368` (lines 337, 346) |
| Withdraw, oust | `RequestWithdrawParty` (0x2b, no body) leaves; `RequestOustPartyMember` (0x2c, a name) lets the leader expel a member. Neither has a server gate beyond the frame limits. | `network/client_dispatch.go:151-152`, `network/partymatch.go:169-183` `withdrawParty`, `network/party.go:133-138` `requestOustPartyMember`, `network/clientpackets/party.go:13-14,68-70` |
| Positions | Every member is told every member's position 6 s after forming, then every 12 s, at any distance. | `network/party_channel.go:157-218` |
| Exp share | Every living member within party range (1500, body to body) of the corpse is rewarded, attacker or not; the party level is the highest rewarded member's; the pool's exp falls by 5/6 per level once that level is more than 5 above the monster; members split it by squared level, after the party bonus by the number that qualify (1.30 for 2, 1.39 for 3, 1.50 for 4). | `data/manager/npcs_party_rewards.go:41-107`, `party/range.go:13-22`, `model/actor/player/party_reward.go:11,93-141`, `model/actor/player/reward.go:9-15,41-45`, `players.properties:173` `PartyRange = 1500` |
| Level cutoff | Default method `level`: a member more than 20 levels under the party level gets nothing. | `players.properties:162,170`, `cmd/gameserver/config.go:1256,1273`, `party_reward.go:103-104` |
| Drops | The top damage dealer receives the drops; any member of the receiver's party may pick them up. Adena picked up in a party is split evenly among the members in range; any other item goes to the member the loot rule picks among those alive, with room and in range. A spoil stays with its sweeper under rules 1 and 3. | `data/manager/npcs_rewards.go:42-69`, `inventory/service.go:281-283` `LootLocked`, `network/party_loot_ground.go:30-49,102-112`, `network/party_loot.go:18-23,38-43`, `party/loot.go:17-65` |
| Resurrection | Resurrection (1016) targets a player's corpse; a player caster sends the dead player an offer (`ConfirmDlg`), which it accepts. Cleric, Elven Oracle and Shillien Oracle learn it at 20. | `skills/1000-1099.xml:447-460`, `handler/skill/resurrect.go:25-60`, `network/live_player_events.go:236-237`; `classes/humanMystic.xml:369` (Cleric, block from 278), `classes/elvenMystic.xml:362` (Elven Oracle, from 266), `classes/darkMystic.xml:375` (Shillien Oracle, from 279) |
| Heal amount | A heal restores its power plus √M.Atk (plus shot terms). | `skill/formulas/heal.go:38-62` |

Nothing the design needs is missing on the server, so this plan files no server issue.

## Design

### 1. The organizer (`internal/botparty`, new, pure; wired in `internal/botregistry`)

A pure planner with no goroutine, no clock of its own and no wire dependency:

```go
// Plan returns the organizer's next state and the orders that move the parties toward the groups
// policy wants, given its current state and every candidate bot, at now. It never mutates s.
func Plan(s State, cands []Candidate, p Policy, now time.Time) (State, []Order)
```

- **Candidate** (one per registry bot with a live run): account, character name, role (from its
  preset, D21), class id, level, whether its class kit heals at its level (from the M10 build, S4
  and S7: section 2) and whether it has learned that heal (`Status.Heals`, section 2), town and position, HP, its live party (leader and member names), its party
  slot as its run reports it (`Status.Party`) and why the bot last cleared it itself
  (`Status.PartyLeft`, section 12), whether that run is new since the last
  pass (`NewRun`, below), the last order the organizer handed that run and whether it was carried
  out (`Sent`, below), the panel's standing order, whether a GM's whispered order holds
  (`Status.Whispered`, section 12), its run state (online in the world, or reconnecting), and its
  registry party mode. A candidate holds only what the supervisor reads this pass; every timer
  lives in `State`. A registry bot with no live run (cooling down, despawned) is no candidate.
- **State** (everything the organizer remembers between passes, returned by one pass and handed to
  the next):
  - `Parties []Party`, the organizer's records, including the parties this pass formed;
  - `Freed map[account]time.Time`: when each bot left a party, which starts its `Cooldown`;
  - `PairBans map[[2]account]time.Time`: until when two bots may not share a party
    (`RePairAfter`, section 9).
- **Party** (one record): an id, the leader's account, the members' accounts, when it formed, its
  state (`forming`, `active`, `broken`) and why, whether the panel pinned it, and its break timers:
  `NoHealerSince` (no living healer member online since), `NoSpotSince` (the leader online in the
  world with no spot to farm since: its snapshot's `Spot` reads empty, `status.go:73-75`; an
  online snapshot names the spot while the bot holds it or farms beside it, `status.go:139-141`
  `farming()`, so a leader farming beside a full spot that fits starts no timer, by design: it
  farms, and the break is for a leader with no fitting spot in walking reach at all, section 4;
  an offline run's `Spot` is reset too, `status.go:119`, so a gone leader is timed by
  `LeaderGoneSince` instead), `LeaderGoneSince` (below),
  `GoneSince map[account]time.Time` (a member not online in the world since: reconnecting, or no
  candidate at all because its run ended or cools down) and
  `BehindSince map[account]time.Time` (a member online in the world but out of party range of the
  leader since). A timer is zero while its condition does not hold; `Plan` sets it the first pass
  the condition holds and clears it the first pass it does not. `BehindSince` counts only while
  the member is online in the world: an offline run's snapshot keeps its last position
  (`status.go:114-125` `Bot.publish`), so a member that is gone could look in range, and its time
  away counts in `GoneSince` instead.
- **`LeaderGoneSince`** holds while the leader is not online in the world (reconnecting, or no
  candidate because its run ended or cools down), or, online in the world with its `lead` order
  answered nil (`Sent`), leads no party or another. A leader's party is read from its snapshot
  only while it is online: an offline run's snapshot keeps the last session's `Leader` and
  `Members` (`Bot.publish` resets only `At`, `State`, `Rule`, `Spot`, `Walk` and `Crafter`,
  `status.go:119`; `publishStatus` sets `State` online and rewrites `Leader` and `Members` in
  the same store, `status.go:137,148-160`), so a reconnecting leader would look like it still
  leads its party. While the party is `forming` and the leader's `lead` is not yet delivered (the
  members' `join` orders come first, below), the leader leads no party by design, so only the
  first condition counts; once `lead` is delivered, a leader that still leads no party after
  `LeaderGone` (its members never accepted: 10 `InviteRetry` rounds, section 4) breaks the party
  like a lost one. "Leads no party" counts only while at least one of the party's bot members is
  online in the world: when every member is gone (`GoneSince` set), the leader leads no party
  because the server dispersed it under them (a party of two disbands on any leave, a disconnect
  included, `party/party.go:337`), not because it lost them, so `LeaderGoneSince` stays zero and
  each member is timed by its own `GoneSince`. Without this, a party of two (leader and healer,
  section 2) whose member logs out would start both timers on the same pass, and a member back
  just inside `LeaderGone` would find its party broken before the re-invitation (up to
  `InviteRetry`) and the next pass both landed. The leader's timer then starts on the first pass
  a member is back online and the party has not re-formed, and gives the leader a full
  `LeaderGone` to re-invite it. A party turns `active` on the first pass whose leader snapshot,
  online, lists one of its bot members.
- **Order:** `lead <names>`, `join <leader> <members>` or `unparty` for one account (section 3).

The supervisor owns the organizer's `State` on its own goroutine, like the registry and the runs.
Its timers today are the cool-down `arm` (`supervisor.go:572-589`) and the registry flush timer
(`armFlush`, `supervisor.go:310-321`); `Run` selects on both (`supervisor.go:252-274`). The
organizer brings its state with it: the supervisor keeps the `State` the last pass returned and
hands it to the next. It calls `Plan` every `PartyTick` (30 s) on its injected clock, a third timer
in the same `select` beside the cool-down tick and the flush, and hands each order to the bot
through the existing `Bot.Command` mailbox, which never blocks
(`mailbox.go:136-152`). No new goroutine, no new shared table: the `State` and the `Sent` records
below are the supervisor goroutine's own. The panel's `FormParty` and `DissolveParty` (section 12)
edit the same `State` on the same goroutine.

**Which orders arrived.** `Bot.Command` answers `ErrBusy` at once when the mailbox (4 deep,
`mailbox.go:11`) is full, and otherwise returns a channel whose one answer comes only once a
session's brain carried the order out; an order that comes between sessions (a reconnect) waits
in the mailbox for the next session (`mail.queued`). Until then the bot's slot is unchanged, so an
empty slot alone does not tell an order that never arrived from a slot the bot cleared itself. The
supervisor therefore keeps, per account, the last order it handed the current run and that order's
answer channel (`Sent`), and before each pass polls every open channel without waiting:

| `Sent` state | Meaning | What `Plan` does |
| --- | --- | --- |
| none, or `ErrBusy` at `Command` | the order never entered the mailbox | plans the order again this pass |
| `ErrBadCommand` at `Command` | the organizer built an order `Command.valid` refuses (a bug: `Plan` never builds a `lead` with no names, below) | not retried: the supervisor logs it and breaks that party, so a bad order is not resent every pass |
| pending (no answer yet) | in the mailbox or queued for the next session | reads nothing into that bot's slot and hands it no further order until the answer comes (a break's `unparty` waits too), so orders reach a bot one at a time and in order |
| answered with an error (`ErrStopped`, or a refusal the brain answers) | not carried out | plans the order again this pass (a stopped run is a new run next pass) |
| answered nil (delivered) | the brain set the slot | from now on an empty slot on this run means the bot cleared it itself |

The brain sets the slot, then publishes its snapshot at once (`publishStatus` with force,
`status.go`), then answers the order; when it clears the slot itself it publishes at once too. The
supervisor polls the answers before it reads the snapshots, so a nil answer it has seen is always
backed by a snapshot that holds the slot.

The party slot lives in the run's `Bot` (section 3), and the supervisor builds a new `Bot` for every
run (`supervisor.go` `start`, the `Live` hook; `cmd/bots/ui.go:218`), so a despawn and respawn, a
cool-down restart or a replace starts with no slot. The supervisor marks a candidate whose `Live`
hook handed it a new `Bot` since the last pass (`Candidate.NewRun`) and drops that account's `Sent`
record: the old `Bot`'s mailbox answers `ErrStopped` (`mailbox.go` `stop`). `Plan` compares each
member's `Candidate` slot with its party record: a new run, or a run with no delivered order, that
lacks its slot (missing or different) is ordered its slot again; a run whose last order was
delivered and whose slot now reads empty cleared it itself (a panel `leave`, a whispered command or
a player's party, sections 3, 5 and 10), so `Plan` drops it from its party, starts its cool-down
(`State.Freed`), re-sends the remaining members their shorter `join` and orders the leader its
shorter `lead` list, or, when no bot member is left, breaks the party with an `unparty` to the
leader (section 9), since `lead` takes at least one name (section 3). A run whose last order was
delivered and whose slot is set but differs from the record is ordered its recorded slot again: only
the organizer sets slots (the raw command route refuses the party kinds, section 3), so a difference
means the record moved since that order, and the new order brings the bot in line. A run in no
party record whose slot is set (a bot dropped while an order was pending, below) is so ordered
`unparty`.

**Formation order.** A bot must hold its `join` slot before its leader's invitation reaches it, or
it takes the invitation for a player's (section 5): the persona and the claim table decide, and
the table lets one bot in per non-GM leader (`partyclaim.go:61-73`), so the second member would
decline and the leader would wait `InviteRetry` for it.
So `Plan` hands the `join` orders first and the leader's `lead` order only on a later pass, once
every member it names has its `join` answered nil (`Sent`); a member whose `join` is still pending
or was refused keeps the leader's `lead` back. A `join` order names the leader and the party's
other bot members (section 3), so a membership change re-sends every remaining member its new
`join` (one order per member, through the slot re-send below). A fill works the same way: the new
member's `join` and every existing member's longer `join` first, the leader's longer `lead` list
after, so no bot in the party is missing from a member's list when a handover needs it (section
4). A drop sends the shorter lists in any order: a list that still names a bot that left is
harmless. Forming so takes one `PartyTick` longer, and no invitation ever races its member's slot.

Eligible bots: online and in the world, preset `farmer` (Damage) or `support` (Support), registry
party mode not `off`, no standing panel order (`stay`, `go`, `follow`, `assist`), no GM's whispered
`follow`, `assist`, `stay` or `go` holding (`Status.Whispered`, section 12: the panel's `Order`
shows only the standing order, `status.go:91,137`, and a whispered one lives only in the brain's
`orders`, `command.go:183-193`), and in no party other than a bot party the organizer formed.
`merchant` (its store), `crafter` (its cycle) and `town` (farms nowhere) never take part. A bot
without a town (no travel map, so no datapack) is never grouped.

**A member that stops being eligible.** Two cases, told apart by why:

- *Gone* (reconnecting, or no candidate because its run ended or cools down): `Plan` keeps it in
  the record and sets `Party.GoneSince`; a member back online within `LeaderGone` (5 min, the
  plan's leader wait) clears the timer and its leader re-invites it (section 8), so a short
  reconnect never breaks the party. Past `LeaderGone` `Plan` drops it (section 9).
- *Made ineligible on purpose* (its party mode set `off`, its preset changed out of `farmer` and
  `support`, a panel standing order, a GM's whispered order): `Plan` drops it on that pass.

Dropping a member removes it from the record, starts its cool-down (`State.Freed`), sends it
`unparty` when it has a live run and no order pending (queued in its mailbox while it reconnects,
`Sent` pending meanwhile), re-sends the remaining members their shorter `join`, and orders the
leader its shorter `lead` list, or breaks the party with an `unparty` to the leader when no bot
member is left. A member with no live run gets no order: its next run starts with no slot. A
dropped member whose last order is still pending (a `join` re-sent while it reconnects, say) gets
no `unparty` that pass, by the one-order-at-a-time rule of the `Sent` table; instead the slot
re-send rule above catches it: a candidate that no party record holds and whose slot reads set once its
last order is answered nil (its recorded slot is none, so its slot differs from the record) is
ordered `unparty`. Until that pass the bot holds a `join` slot its leader's shorter `lead` list no
longer names, so it is invited by no one and stands in the join hold (section 8) for at most one
`PartyTick`.

### 2. Composition and level band

| Slot | Who | Why |
| --- | --- | --- |
| Leader | A Damage-role bot (preset `farmer`): the highest-level Damage candidate of its group, taken first as the Grouping rule below orders them; a Support member may stand above it | It farms alone today, picks spots and pulls; its kit is the damage. |
| Healer (required) | A Support bot whose class kit heals another member at its level, with that heal learned (below): a human, elven or dark mystic from 7 (1011 Heal @7), a Cleric, Elven Oracle or Shillien Oracle (heals @20), an Orc Shaman from 20 (1229 Chant of Life, a party heal) | Keeps the leader up, so the party rests less and takes stronger packs. |
| Third (optional) | Any other Support bot (a second healer, an orc mystic with its chants, a Support fighter), else a second Damage bot that has no healer to lead with (Grouping, step 2) | Buffs, a second target for packs, more damage. |
| Fourth (optional, off by default) | Any of the above | Size up to `MaxSize`. |

The healer flag reads the S7 kit rows (`docs/strategy.md`): the HM row's Support heals
(`1011 Heal @7`, which EM and DM share), CL, EO and SO (heals @20), and OS (`1229 Chant of Life @20
(party) with a member under 50 %`). A level 1-6 mystic is no healer, and neither is an orc mystic
(the OM row heals no one but itself, through 1090 Life Drain). M10 defines no tank flag (its spot
margin, tank 1.5, normal 2.0 or glass cannon 3.0, is per row and does not match a class list), so
the planner picks no tank.

The kit row says only which heal a class casts from which level, not that the bot bought it: 1011
Heal is learned from a trainer (160 SP from 7, `classes/humanMystic.xml:50`) and 1229 Chant of
Life costs 2900 SP at 20 (`classes/orcMystic.xml:250`), so a level-7 mystic or a level-20 Orc
Shaman that has not trained yet would count as the party's healer, and `NoHealerSince` (section
9), which checks only a living healer member online, would never break its party. So the flag also
needs `Status.Heals`, true while the run's skill list (`World.Skills`, from `SkillList`,
`world.go:29-32`) holds one of the heals the kit row names for its class: 1011, 1015 or 1027 for a
mystic, Cleric, Elven Oracle or Shillien Oracle, 1229 for an Orc Shaman. `Status` carries no
skills today (`status.go:55-98`); the field ships with the organizer (P5), and a bot that trains
the heal later counts from the next pass. A mystic that reaches 7 inside a bot party trains it
there, since P2 lets a bot of a bot party walk to its trainer (section 8, "Errands in a bot
party").

- **Size:** 3 by default, at most `MaxSize` 4 (persona defaults, D6). Small parties keep spot caps
  (a third of a maker's `maximumNpcs`, `spotregistry.go` `spotCap`) and the visibility cost of
  clustered bots (plan, "Where the cost is") low, and already earn the 1.39 party bonus.
- **Level band:** a party forms only with every bot member within `Band` (3) levels of every
  other, and breaks once its bot members' spread exceeds `MaxSpread` (5, section 9). Two server
  facts set `MaxSpread`: the pool's exp falls once the party level is more than 5 above the
  monster (table above), and under today's `SpotPolicy` (`Below` 2, `Above` 3, `spots.go:110-117`,
  `fits` at `spots.go:120-125`) a spot fits a lowest member at `Lmin` and a highest at `Lmax` only
  while its top monster lies in `[Lmax-3, Lmin+2]`, which is empty once `Lmax - Lmin > 5`. The 2
  levels between `Band` and `MaxSpread` are the hysteresis, so a level-up does not break a party,
  and they lie inside the range a spot can hold. The server's own cutoff (20 levels) never bites
  inside it.
- **Location:** members share the leader's town (`Status.Town`) and stand within `MaxTravel`
  (20000, straight line) of it, the farmer's own reach for a spot.
- **Grouping:** per town, in two steps, each member taken only when the group's bot members, it
  included, stay within `Band` of one another (the level band above), not only within `Band` of
  the leader: a Support member may stand above its leader, so a leader at 20 with a healer at 17
  takes a third member from 17 to 20, never one at 23 (a spread of 6, past `MaxSpread`, which the
  next pass would break and `Cooldown` re-form).
  1. *Leaders and healers:* Damage candidates from the highest level down; each takes the nearest
     free healer. A group with no healer forms no party, and its Damage candidate is left
     *healerless*.
  2. *Third members:* groups in the same order (highest leader first); each takes the nearest free
     Support candidate (a second healer or any other Support bot), and only when none fits, the
     nearest healerless Damage candidate, as the composition table orders them. A Damage
     candidate that step 1 gave a healer always leads its own group and is never another group's
     third.
  At most `FormPerPass` (2) new parties a pass, so invitations spread out: the first groups in
  that order form, and the others wait for the next pass. A Damage candidate whose group waits so
  is not healerless and is not taken as a third member meanwhile. A fill (section 9) takes its one
  member a pass in the order of step 2.

### 3. Orders to bots: `lead`, `join` and `unparty`

Three new panel commands. `lead` and `join` are standing like `stay` and `go` (plan section 8: the
`Bot` keeps them next to its mailbox and applies them again at each session start, and shows them
in the snapshot), in a slot of their own beside the standing order; `unparty` is one-off, like
`leave`:

- `lead <name> <name>...`: lead a party of these characters (the server accepts only character
  names matching `^[A-Za-z0-9]{1,16}$`, `data/manager/roster.go:26` `validCharacterName`, so a
  space separates them);
- `join <leader> <member>...`: accept that leader's invitation; the names after the leader are the
  party's other bot members (none for a party of two), so the member knows every bot of its party
  by exact name for the handover (section 4) and for telling a player-led party (below);
- `unparty`: clear the slot.

`Command.valid` (`mailbox.go:66-75`, today only the known kinds with a trimmed, non-empty `Arg`)
gains the three kinds: `unparty` takes no `Arg`; `join` takes 1 to 8 names (the leader first);
`lead` takes 1 to 8 names (a party holds 9); both separated by single spaces, each
`[A-Za-z0-9]{1,16}`, none twice. Anything else answers `ErrBadCommand` as today.

A `lead` or `join` replaces the bot's panel standing order (`stay`, `go`, `follow`, `assist`), as
a new panel order replaces the old one today (`mailbox.go:211,229` set `link.order`): the brain
clears `link.order`, drops the session's `stay`, `roam`, `follow` and `assist` it set (keeping
`reply` and `withdraw`) and clears `orderPending`, then sets the slot. Without this a standing
`follow X` would be applied again at each session start (`applyStanding`, `mailbox.go:254-266`)
and, since `followed` puts `orders.follow` first (`command.go:277-286`), the member would accept
its leader's invitation and go on following X. The organizer never orders a bot with a standing
order (section 1), so only a party the panel forms or pins by hand (section 12) replaces one:
the panel's choice, the latest panel order winning as between panel orders today.

While a GM's whispered `follow`, `assist`, `stay` or `go` holds this session, the brain answers
`lead` and `join` with a new `ErrWhispered` and keeps no slot: a bot that follows a GM would
otherwise accept its bot leader's invitation and go on following the GM, since `ruleLeader` and
`followed` give a whispered `follow` precedence (`death.go:293` `b.orders.follow != 0`). The
snapshot reports the same state (`Status.Whispered`, section 12), so the organizer does not pick
such a bot (section 1); the error covers a whisper that lands between the snapshot and the order,
and as an error answer it is planned again only once the bot is eligible again.

The three kinds are added after `CommandLogout`, and the raw command route
(`POST /api/bots/{account}/command`, `internal/botpanel/api.go:39`) keeps refusing them:
`parseCommand` tries only the kinds from `CommandStay` to `CommandLogout` (`api.go:290-296`), and
that bound stays. A `lead` or `join` sent there by hand would set a slot the organizer has no
record or `Sent` entry for, so hand forming goes only through `FormParty` (section 12), which
writes the organizer's records and hands the orders as a pass does.

`unparty` clears the slot, and withdraws (`RequestWithdrawParty`) only when the bot sits in the
bot party that slot named: the party it leads under a `lead` slot, or the party of the leader its
`join` slot names. A leader leaving disperses it (table above). In any other party (a player's),
or in none, `unparty` only clears the slot: it never takes a bot out of a party a player leads.
The one exception is a `join` bot the server made leader while its handover to a player has not
gone through (section 4, "Handover"): it leads that party only until the handover, so `unparty`
withdraws it.

A panel standing order (`stay`, `go`, `follow`, `assist`), or a GM's whispered `follow`,
`assist`, `stay` or `go <spot>`, given to a bot that holds a party slot clears the slot, whether
the bot sits in its bot party or is between invitations (a member whose party dispersed, a leader
whose members have not joined yet), and, when it sits in the bot party, takes it out first:
following a player and following the bot leader cannot both hold, a `stay` would leave the member
behind its leader until the fall-behind drop (section 8), and a `go` would claim a spot
(`command.go:236-241` `goToSpot`) for a bot that stays a member, which never claims (section 6).
A slot left set on a partyless member would accept the leader's next invitation and undo the
order. Today's `obey` handles all four (`command.go:180-193`) and gains that step before them;
the panel's `panelOrder` (`mailbox.go:191`) gains it too. The panel's one-off `leave`
(`CommandLeave`, `mailbox.go:213-220`, which today answers `ErrNoParty` from no party and
otherwise only sets `leavePartyAction`) clears the slot too, before it withdraws: without that a
member's `join` slot would accept its leader's next invitation and a leader's `lead` slot would
invite everyone back, undoing the `leave` within `InviteRetry`. The clear runs before the
`ErrNoParty` check: a partyless bot holding a slot (a member between invitations, a leader whose
members have not joined) clears it and answers nil, since the `leave` took it out of its bot
party's plan, and resets its leader watch as every clear from no party does (section 8), so no
lost-leader wait, whisper or `/unstuck` follows; only a partyless bot with no slot answers
`ErrNoParty` as today. Every whispered
command that takes the bot out of its party (a GM's `leave`, a GM's or player's `invite me`,
section 10) clears the slot too, before the bot withdraws, and today's `obey`
(`command.go:194-203`, which only sets a reply) gains that step.

**Withdrawal and reply in order.** Today a command's packet goes through one reply slot,
`orders.reply`, sent by rule `command`, and "a later command's reply replaces one not yet sent"
(`command.go:122-125`, `ruleCommand` at `command.go:252-258`). A slot-clearing command that also
answers needs two packets in order: a GM's `invite me` the withdrawal, then the "invite me"
whisper; a whispered `go <spot>` that fails the withdrawal, then the error whisper. Setting
`o.reply` twice would lose the withdrawal. So `orders` gains a `withdraw` flag beside `reply`: the
slot-clearing step sets it only while the bot sits in a party, and `ruleCommand` sends
`RequestWithdrawParty` while it is set and the bot is still in a party, and only then the reply.
The withdrawal is an `actLeaveParty`, whose send clears the flag and, as today, resets the leader
watch (`brain.go:815-817`). The flag never outlives the party it was set in, since today's reply
slot clears only when an action of its own kind is sent (`brain.go:840-842`) and nothing would
clear it otherwise. `ruleCommand` drops it unsent, resetting the leader watch as the send would, at
the first decision that finds the bot in no party: the bot party dispersed between the order and
that decision (a disband sends every member `PartySmallWindowDeleteAll`,
`party/party.go:381-386`). The bot's own `RequestAnswerJoinParty` drops it too, when that answer is
sent (`brain.go:818-820`): a bot enters another party only by answering an invitation, and
`rulePrompt`, which answers it (`rules.go:585-587`), runs ahead of `ruleCommand`
(`rules.go:476-477`), so an invitation answered in the very decision that first finds the bot
partyless cannot carry the flag into the new party (the GM's party that the "invite me" whisper
asks for, for one). A leader change keeps the flag: the bot stays in the same party
(`PartySmallWindowDeleteAll` then `PartySmallWindowAll`, `party/party.go:418-427`) and answers no
invitation, so the withdrawal still goes out. A later command's reply still replaces a reply not
yet sent, never the withdrawal. The withdrawal has no chat gate, so it goes
out at once and the whisper follows within the chat gate as today. The slot-clearing step runs
before the order it precedes, and three orders then reset `orders` keeping only `reply`: the
panel `go` (`mailbox.go:200`, `*o = orders{roam: true, reply: o.reply}`), `holdStill` for the
panel `stay` and a whispered `stay` (`mailbox.go:234-237`, `orders{stay: true, reply: ...}`,
called from `mailbox.go:195` and `command.go:187-188`) and `orderPlayer` for the panel `follow`
and `assist` (`mailbox.go:241-249`, `*o = orders{reply: o.reply}`). Each keeps `withdraw` as it
keeps `reply`, so every one of these orders given to a member in its bot party withdraws it;
otherwise the slot would be gone and the bot still in the party, which the later `unparty` would
not withdraw either (it withdraws only from the party a slot names, above). The panel `leave`
sets `withdraw` instead of the reply.

The slot also clears when the bot is in a party led by anyone its slot does not name, whatever
slot it holds: for a `join` slot, a leader other than the bot itself, the slot's leader and its
listed members; for a `lead` slot, any leader but itself. Only a bot with a `lead` slot invites,
and it invites only its listed bots and players who whispered `join` (sections 4, 10), so such a
party is a player's (or a GM's): a `join` member that stays after the leadership passed to a
player (section 10), or a bot of either slot that accepted a player's invitation (section 5). The
bot tells this by the exact names its slot holds, never by the bot marker (section 5). A `join`
member in a party led by a bot its slot names keeps its slot: that is its own bot party, or a
handover in progress (section 4). A `join` member that leads its own party never clears its slot
by this rule: the server made it leader when its bot leader disconnected
(`party/party.go:337-346`, and `changeLeader` sends it `PartySmallWindowAll` with itself as
leader, `party/party.go:418-427`), the slot names neither itself nor any player, and that case
belongs to the handover (section 4), which withdraws it or hands the party on at once. Without
this exception the slot would clear before the handover ran, and the bot would keep the party and
fall back to today's lost-leader wait (`watchLeader` reads a party the bot leads as no leader,
`death.go:199-204`). P1b ships the rule with the exception, so a `join` bot made leader keeps its
slot there and P1d adds what it then does. The `party` rule (section 4) invites only while the bot
is in no party or leads its own, so a `lead` slot never sends `RequestJoinParty` from inside
someone else's party (the server would answer 154, `party/party.go:215-216`,
`network/party.go:82-84`).

The slot is what `Status.Party` reports, so the organizer sees the bot left (section 1). The brain
keeps its copy of the assignment, owned by its goroutine.

Because the slot outlives the sessions of one run (`mailbox.go` `botLink`, lent to each
session's brain), a reconnecting member is invited again by its leader, and a reconnecting leader
invites its members again, with no organizer pass in between. A new run starts with no slot; the
organizer orders it again (section 1).

### 4. The leader

- **Invitations** (a new rule `party`, after `announce`, so the flee, heal and chat rules go
  first): one `RequestJoinParty` at a time, with no invitation in flight, and only while the bot
  is in no party or leads its own (section 3). The invitee is the first player waiting from a
  `join` whisper (section 10), else the first listed member not in the party. The in-flight state
  records whom it invited, so every answer below is charged to that invitee: the server answers
  the inviter's own book or the party's `InviteWaiting` with 164 to whichever invitation it sees
  second (`network/party.go:71-74,88-90`, `party/party.go:219`), and `JoinParty` carries only the
  answer, no name (`party.go:25`, `reader.go:101`), so a second invitation in flight could not be
  told from the first. In
  flight ends on `JoinParty` (`party.go:24-25`, `reader.go:101`), on a refusal the bot reads (152,
  153, 154, 155, 160, 164, 185, 619, 940, or message 1987 naming a detached or jailed target), or
  `InviteWait` (17 s) after sending, which also covers the Olympiad case the server leaves
  unanswered. The server's own 15 s window (`InviteTimeout`, `party/party.go:20`) starts when it
  processes the request (`setInviting`, `party/party.go:453-456`), later than the bot sent it, so
  `InviteWait` keeps a 2 s margin past it; an invitation sent at exactly 15 s could still land
  inside the window and be answered 164. A 164 (`WaitingForAnotherReply`, sent for a requester with
  a request still pending, `network/party.go:71-74`, a party still waiting, `network/party.go:88-90`,
  or a requester that turned busy before the request was booked, `network/party.go:101-103`)
  speaks of the leader's own window, not of the member: it ends the invitation in flight, starts
  no `InviteRetry` for that member, and the next invitation goes out `InviteWait` later. An
  invitation that ends at `InviteWait` with no answer at all (a member whose client never answers, or
  the Olympiad case the server leaves unanswered, `network/party.go:67-69`) names the member too: the
  server's own window is over by then, so the next listed member is invited at once, and the
  silent member again only after `InviteRetry` (30 s), counted from the timeout, never every 17 s.
  Any other refusal names the member: a refused member is tried again after `InviteRetry` (30 s), and so is
  a member that declined: the server sends the inviter `JoinParty` with the invitee's answer for a
  decline too (`network/party.go:126`, in `requestAnswerJoinParty` before the answer is applied),
  and a member whose slot no longer names this leader declines at
  once in `rulePrompt` (`rules.go:584-587`, section 5), so a `JoinParty` with `Response` 0 starts
  `InviteRetry` for that member like a refusal; otherwise the leader would re-invite it at its
  reaction delay, bounded only by the frame bucket, until the organizer's next pass. A listed
  member that left the party waits `InviteRetry` too: the server tells the members a withdrawal
  and a disconnect alike (`MsgLeftParty` for both, `party/party.go:354-360`), so the leader waits
  before inviting it back, long enough for the organizer's next pass to drop a member that left on
  a panel `leave` or a whisper (section 1); a member whose slot is cleared declines it meanwhile
  (section 5), which starts `InviteRetry` again. The first invitation of a
  partyless leader carries the loot rule (D27: 3, by turn); later ones carry it too, though the
  server keeps the party's. A player invited from the `join` queue gets no `InviteRetry`: whatever
  ends its invitation (an acceptance, a decline, a refusal, `InviteWait`) takes it off the queue,
  and only a 164 puts it back at the queue's head, since that answer speaks of the leader's window,
  not of the player.
- **Loot rule constants:** the bot names all five server rules (today `encodeRequestJoinParty`'s
  comment lists three).
- **A member dropped from the list** (the organizer reshuffled): the leader expels it with
  `RequestOustPartyMember` (a new encoder), one per decision. Only a bot that was on an earlier
  `lead` list is ousted: a player who joined through the `join` whisper was never listed and stays.
- **Handover:** a bot with a `join` slot that finds itself leading (`PartySmallWindowAll` with its
  own id as leader: its bot leader disconnected, `party/party.go:345-347,418-427`) keeps its slot
  (the exception in section 3) and does one of two things.
  When every other member is a bot its `join` slot names (the old leader or a member the order
  listed, section 3), it leaves at once (`RequestWithdrawParty`), which disperses the party
  (`party/party.go:337`); the members' `join` slots still name the old leader, who re-invites them
  when it is back. A member its slot does not name is a player: only the leader invites, and only
  its listed bots and the players who whispered `join` (section 10), and the formation order
  (section 1) lists every bot in each member's `join` before the leader invites it. When such a
  member is among them, the bot hands the party to the one with the lowest object id with
  `RequestChangePartyLeader` (a new encoder) instead. The bot keeps no join order: `World.Party`
  holds the members in a map by object id (`party.go:198-206`), which `applyParty` rebuilds from
  `PartySmallWindowAll` and adds `PartySmallWindowAdd` rows to (`party.go:231-250`), so the
  server's member order (`g.others`, `party/party.go:423,469-471`) is lost; the lowest object id is
  a choice the bot can make from what it holds, and the server takes any member not in a duel by
  name (`ChangeLeader`, `party/party.go:395-407`). The player keeps the party, and section
  10's handover to a player applies: every bot in it, the one that handed over included, is now in
  a party a player leads and clears its slot (section 3). `watchLeader` no longer counts either
  case as a lost leader.
  A player in a duel is refused in silence: `ChangeLeader` changes nothing for a target with
  `InDuel()` and sends no notice (`party/party.go:403-405`), so no new `PartySmallWindowAll` comes.
  Without a fallback the bot would stay leader of a player's party, holding its `join` slot by the
  exception, and since `followed()` returns the party's leader (`command.go:282-285`), the bot
  itself, `ruleFollow` passes (`follow.go:99-102`) and `ruleSpot` after it (`rules.go:510`) would
  claim a spot, against "members never claim"; the organizer's `unparty` would not withdraw it
  either, since the bot sits in no party its slot names (section 3). So the handover is a small
  state of its own, `handover` (the players tried, when the last request went out), owned by the
  brain goroutine: while it runs, the join hold (section 8) holds the bot too, so it claims no spot
  and farms nowhere. When no `PartySmallWindowAll` naming a new leader arrives within `HandoverWait`
  (5 s, a persona default; the server answers a change at once, `party/party.go:418-427`), the bot
  asks the player with the next object id, and after the last one starts again from the lowest,
  one `RequestChangePartyLeader` per `HandoverWait`. When none has taken the party within
  `HandoverFor` (2.5 minutes, past a duel's 2-minute `Length`, `duel/state.go:58`), the bot
  withdraws, which disperses the party (a leader leaving, `party/party.go:336-339`), its players
  free to form it again. The players to try are read from the party window at each request, so
  one who left is skipped and one who joined is tried; once no player is left, every other member
  is a bot its slot names and the bot leaves at once as above. An `unparty` while the handover
  runs withdraws too, as from its bot party, so the
  organizer's break never leaves the bot leading a player's party.
  A bot without a `join` slot that ends up leading a player's party keeps today's behavior: it
  stays and waits for its lost leader (`death.go` `watchLeader`). Before P1d ships this handover
  no bot leads, so the case never arises outside unit tests.
- **Trips:** while it leads members, the leader takes no gatekeeper trip (`planTrip`): members
  follow by walking only (`follow.go`, the walker within the SCC). Its supply trip (D16) still
  runs; the members' answer is in section 8. Today a farmer reaches a new spot by gatekeeper once
  none fits with room in walking reach: `claimSpot` calls `planTrip` when no candidate can be
  claimed (`spots.go:509-517`, `travel.go:416-431`), gives its spot back for an active trip
  (`spots.go:518-523`) and picks again only `spotRetry` (1 minute, `spots.go:197`) later. A
  leader denied the trip goes on down `claimSpot` instead: it keeps a held spot it is not
  outlevelled for and that fits the band (section 6), else claims past the cap (`claimOverN`),
  else farms beside a full spot (`farmBesideN`, section 6, `spots.go:524-534`).
  One branch of that path needs a change. A holder that outlevelled its spot, or whose spot
  misfits the band (section 6), and that finds no candidate at all in walking reach falls through
  the keep (`spots.go:524`), finds nothing for `claimOver` on an empty order (`spots.go:527-529`)
  and, holding a claim, skips `farmBeside` (`spots.go:530-532`): it returns and keeps the spot.
  `spotCands` never lists the held spot nor one that fails `spotFits` (`spots.go:466-491`), so
  every re-pick, one `spotRetry` apart (`spots.go:254`), ends the same way. A solo bot leaves this
  state through its trip, which gives the spot back (`spots.go:518-523`); a leader without the trip
  would farm the spot it outgrew, its snapshot's `Spot` set (`status.go:139-141`), with no
  `NoSpotSince` ever starting. This is how a party most often outgrows its town: the leader
  levels at its spot until it outlevels it. So P2 adds the gated trip's give-back: at the step
  where a solo bot calls `planTrip` (`spots.go:515-517`, a located bot, `here != nil`), a leader
  with members that holds a spot it is outlevelled for (`outleveled`) or that misfits the band
  (`misfit`, section 6), and finds no candidate (`len(cands) == 0`), gives that spot back
  (`leaveSpot`, its whole `spot.weight`) as a solo bot does for its trip. Holding no claim, it then
  farms beside nothing (`farmBesideN` on an empty order), its snapshot's `Spot` reads empty and
  `NoSpotSince` starts. The given-up spot is no candidate at the next re-pick (it fails the fit
  that made it outlevelled or misfit, `spots.go:120-125`), so the leader does not claim it back.
  With at least one candidate, every one of them too near its limit for `n`, it keeps the spot and
  re-picks every `spotRetry` (section 6): room may free in walking reach, and no trip would help.
  It so stands idle with its members only when no spot in walking reach fits its band (its own
  level when no spot of the map fits the band, section 6), or with `Beside` turned off when every
  fitting one is at its limit, which levelling past its town's spots makes likely, while no other
  break reason fires (the leader is online, the healer alive, the members in range and the spread
  under `MaxSpread`). The organizer therefore breaks a party whose leader has had no spot to farm,
  held or beside, for `NoSpotFor` (section 9); freed, the leader plans its trip alone as today.

### 5. The members

- **Joining:** `rulePrompt` (`rules.go:584-587`) accepts an invitation from the leader its `join`
  slot names before the persona's `JoinParty`, so the claim table neither limits nor records it
  (D29). It declines an invitation from a bot leader it knows by exact name: any leader a `join`
  slot of this run named other than the one its current slot names, whether a slot is set or not.
  The `Bot` keeps these names beside the slot (in its `botLink`, owned by the brain goroutine like
  the slot, a handful of names per run). Only a bot with a `lead` slot invites a bot, only the bots
  on its list, and each of them got its `join` naming that leader first (section 1), so every bot
  leader that can invite it is one it knows: a bot that just left its bot party on a whisper, or
  that the organizer moved to another leader, does not drift back. A new run knows no leader and
  has no slot; an invitation from its old leader then goes to the persona as a player's would,
  which can only put it back in the party the organizer re-orders it to (section 1, `NewRun`).
  The bot marker plays no part: `botnames.Marks` (`names.go:66-79`) is a case-insensitive suffix
  or prefix match that also takes a player named Talbot for a bot under the default `Bot` suffix
  (`names.go:51`), fit for skipping a chat reply ("costs a reply, never a loop") but not for
  refusing a player's invitation. So a player whose name ends in the marker keeps the bots'
  acceptance, and so do the e2e harness's players, which are named with `DefaultMarker`
  (`internal/e2e/helpers_live_test.go:34`) and invite bots expecting 1
  (`party_live_test.go:56`, `support_live_test.go:304`, `recover_live_test.go:122`). Every other
  invitation still goes to the persona and the claim table as today, and accepting it clears the
  slot, whether a `join` or a `lead` slot (players first, D28; section 3): every persona accepts
  invitations (`rules.go:228` `JoinParty: acceptParty`, `party.go:181`), so a partyless leader can
  end up in a player's party too, and with its slot cleared it neither invites from inside it nor
  is pulled out of it by a later `unparty`.
- **Spot:** on joining any party it does not lead, a bot gives back the spot it holds or the
  beside count it farms with (`leaveSpot`, `spots.go:601-610`). This also fixes today's member
  that keeps a spot it never uses.
- **Play:** today's follow slot, assist, heals and buffs, unchanged; under rule 3 every member may
  pick up loot, since the server hands each item to the member in turn.

### 6. Spots for a party

- **Claim weight:** the leader claims room for the whole party: `SpotRegistry.claimN(order, n)`
  takes `n` of a spot's cap (empty spots first, as `claim` does, `spotregistry.go:103-119`),
  `claimOverN` takes it past the cap from the spots whose held count plus `n` stays within their
  limit (`claimOver` takes a spot only below its limit, `spotregistry.go:126-144`), `farmBesideN`
  counts `n` beside a spot (below), and the leader re-weighs when the party's size changes. `n` is
  the party's size in the leader's party window: every member in the party, players too, since
  each takes room at the spot, whether it stands there, is dead or is away on a trip. It changes only when a member joins
  or leaves the party (the window's add and delete), never on a death, a raise or a trip, so a
  death frees no room another bot could claim and a raise takes none back. Members never claim
  and never farm beside.
  The weight is the window's size only while the bot leads its bot party: its `lead` slot set and
  its party window naming it leader. Any other bot weighs 1, as a solo bot does, and a leader that
  stops leading re-weighs to 1 at once, keeping the spot it farms. That covers the party ending
  under it, which resets `World.Party` to the zero `Party` (`PartySmallWindowDeleteAll`,
  `party.go:259-260`, a window of size 0): a party of two whose member logged out (section 8), its
  own withdrawal on `unparty`, a panel or GM order, or a break; and its slot clearing while it
  still leads. Without this, a freed leader would keep `n` of the spot's cap: other bots would find
  the spot full (`overCap`, `spotregistry.go:179-183`), and its own crowd check would subtract `n`
  (`crowded`, `spots.go:416-418`), until it next left the spot. That can take long, since a
  whispered or panel `go` to the spot it already holds keeps the claim as it is
  (`command.go:235-236`). So the brain re-weighs whenever this target weight changes: on every
  party window event (an add, a delete, the dispersal) and every change of its slot. A member's
  return and the leader's re-invitation then re-weigh from 1 to the new size, as at formation. The "members present" of the party model (section 7) is a different
  count, for the model only.
- **Farming beside:** since #142 a bot that finds every fitting spot full farms beside one with no
  claim. `claimSpot` (`spots.go:495-541`) tries `claim` on the spots within `MaxTravel`, then on
  those beyond (`spots.go:509-512`); with neither it sets `retryAt` (`spots.go:514`), plans a trip
  (`spots.go:515-517`), gives its spot back when a trip is active (`spots.go:518-523`), keeps a held
  spot it is not outlevelled for (`spots.go:524-526`), tries `claimOver` when `Overflow` is on
  (`spots.go:527-529`), and, failing that too, a bot holding no claim calls `farmBeside`
  (`spots.go:530-534`). The registry counts those bots in `beside`, apart from `held` and toward
  neither the cap nor the limit (`spotregistry.go:23-27,152-167`), and `Beside` and `Overflow` are
  on by default (`spots.go:113`). A leader that holds no claim and finds no spot with room for `n`
  below its limit lands on that same step, and it farms beside with its whole weight:
  `farmBesideN(order, n)` picks the spot as `farmBeside` does (the fewest beside per monster,
  `spotregistry.go:152-167`) and adds `n` to its beside count. Counted 1, as today's `farmBeside`
  would count it, the party would look to every other bot at the spot like `n - 1` strangers,
  since their crowd check subtracts the beside count (below), and they would move on from a spot
  the party only shares. A leader farming beside re-picks as a solo bot does: holding no claim,
  `ruleSpot` calls `pickSpot` at every decision past `retryAt` (`spots.go:254`, `!v.held`), so it
  claims `n` (`claimN`, else `claimOverN`) the first time a fitting spot has room for them, and
  meanwhile `farmBeside` keeps the spot it farms beside while that spot is still a candidate
  (`spots.go:550-552`). Candidates are the spots that fit the band (below), so a band change that
  puts the beside spot out of fit moves the party at the next re-pick, within one `spotRetry`.
  The beside spot is the leader's spot in every other respect: the members follow the leader there,
  the gather gate applies at it, and the snapshot's `Spot` names it (`status.go:139-141`, set while
  `farming()`, held or beside, `spots.go:229`), so a leader farming beside starts no
  `NoSpotSince` (sections 1, 9).
- **Weighted release:** today's `release(i)` takes 1 off a spot's held count
  (`spotregistry.go:193-199`), `leaveBeside(i)` 1 off its beside count
  (`spotregistry.go:169-176`), and `leaveSpot` calls the one that matches what the bot farms, once
  (`spots.go:601-610`). The brain records the weight it farms with (`spot.weight`, 1 for a solo
  bot, `n` for a leader's claim or beside count), `SpotRegistry.releaseN(i, n)` and
  `leaveBesideN(i, n)` take `n` off (never below 0), and `leaveSpot` gives back `spot.weight` from
  the count the bot sits in, so every caller that gives a spot up returns the whole weight. In
  `spots.go` those are five: the give-back for an active trip in `claimSpot` (`:522`), the old spot
  on a new claim (`:539`), `farmBeside` moving to another beside spot or none (`:553`), `moveOn`
  (`:584`, on an unreachable walk `:274`, a crowded spot `:283` and a death with `DeathAvoid`
  `:593`) and a death without it in `diedAtSpot` (`:596`); outside it, the supply trip
  `supplytrip.go:121`, a command `command.go:241` and the brain's stop `brain.go:359`. P2 adds a
  sixth in `claimSpot`, the gated trip's give-back of an outlevelled or misfit spot (section 4,
  "Trips"). Every new
  claim or beside count sets `spot.weight` to what it took: `n` for `claimN`, `claimOverN` and
  `farmBesideN`, and 1 for the `go` command's claim (`command.go:238-242`), whose order clears the
  party slot first (section 3). A re-weigh on a size change (formation included, from 1 to `n`,
  and the end of leading, from `n` back to 1, "Claim weight" above) is
  one `reweigh(i, beside, old, new)` under the registry's lock on the count the leader sits in
  (held or beside changes by `new - old`), so no other bot sees the spot half released, and it
  sets `spot.weight` to `new`. A re-weigh down, or of a beside count, needs no check: beside counts
  toward neither cap nor limit. A held re-weigh up (a member or a player joined) may take the spot
  past its cap, as `claimOver` does for a solo bot (`spotregistry.go:126-144`); today's re-pick
  then applies: `ruleSpot` calls `pickSpot` when `overCap` holds (`spots.go:254`,
  `spotregistry.go:179-183`). When another spot has room for `n` (within `MaxTravel` first, as
  `claimSpot` orders them, `spots.go:497-512`), it gives the spot up and claims `n` there. When
  none has, a bot that holds a spot it is not outlevelled for keeps it: `claimSpot` sets
  `retryAt` to `spotRetry` (1 minute, `spots.go:197,514`) and returns before `claimOver` and
  `farmBeside` (`spots.go:524-526`, `if b.trip.v.active() || (v.held && !b.outleveled())`). No
  trip is active for it: the `planTrip` call on that path (`spots.go:515-517`) is the gatekeeper
  trip a leader with members does not take (section 4, P2 gates it), so the trip's give-back
  (`spots.go:518-523`) does not run either. So a party a player joined stays at its spot past the
  cap until a spot with room for `n` frees within walking reach, re-picking every minute, as a solo
  bot shared past its cap does today; it never travels for it. `claimOverN` serves only a leader
  that holds no claim (after a death, a trip or a move, or while it farms beside) when no spot has
  room for `n` below its cap, as `claimOver` does today (`spots.go:527-529`), and `farmBesideN`
  only one that `claimOverN` found no room for either (`spots.go:530-534`).
- **Crowd check:** `crowded` (`spots.go:402-418`) skips party members and then subtracts
  `Held + Beside - 1` (`spots.go:418`), the other bots holding the spot or farming beside it, the
  bot's own 1 aside. With a leader's weight `n` in one of those counts, it subtracts
  `Held + Beside - spot.weight` instead (`spot.weight` 1 alone, so a solo bot's check is today's):
  the other bots' claims and beside counts only, never the party's own members, who are not
  counted. Without this a party of 3 would need `Crowd + 2` strangers before it judged a spot
  crowded. The other side is `farmBesideN`'s weight: a solo bot at a spot the party farms beside
  counts the party's members as players and subtracts the party's whole beside weight, so the
  party costs it nothing, as `n` bots beside it would.
- **Bot members:** the leader tells its bot members from players by its `lead` list: a party
  window row whose name the list holds is a bot member. Spot fit and the party model (section 7)
  use only bot members; a player in the party is ignored for both (section 10).
- **Fit by level:** a spot fits the party when `Below` and `AggroAbove` hold against its lowest
  bot member's level and `Above` and `Outlevel` against its highest (section 2), from the party
  window's levels (`PartyMember.Level`). When no spot of the map fits that band, the leader fits
  spots by its own level alone. The test is map-wide and by levels only (the band fit above, in
  `SpotPolicy.fits`' terms, `spots.go:120-125`), not limited to walking reach, avoidance or pack risk as the
  candidates are (`spotCands`, `spots.go:466-491`): the fallback is for a band no spot can hold,
  not for one whose spots lie out of walking reach. When band-fitting spots exist only beyond
  walking reach, the candidate list is empty even if a spot in reach fits the leader's own level:
  the leader claims nothing, gives back a held spot that misfits the band (section 4, "Trips"),
  and the party stands idle until `NoSpotFor` breaks it (section 9) and the freed leader travels.
  A reach-wide fallback would instead keep the party at spots too strong for its lowest member for
  as long as it lasts, with no break ever firing. The brain needs no pin flag for this: the `lead`
  order carries only names and `MaxSpread` is the organizer's policy, so the leader only sees that
  no spot fits. That happens to a pinned party (section 12) whose spread exceeds `MaxSpread`, the
  panel's choice, and to an auto party for up to one `PartyTick` after a level-up past
  `MaxSpread`, until the organizer breaks it (section 9).
- **A held spot that no longer fits the band:** band fit would otherwise apply only when a spot is
  picked. Today `ruleSpot` re-picks a held spot only when the bot is outlevelled for it, it is over
  its pack risk or the spot is over cap (`spots.go:254`), `outleveled` compares only the bot's own
  level with the spot's `MaxLevel + Outlevel` (`spots.go:423-426`), and `claimSpot` keeps a held
  spot while `v.held && !b.outleveled()` (`spots.go:524`). At formation the leader is the highest
  Damage bot of its group and already holds a solo spot fitted to its own level (`fits`, `Below` 2,
  `Above` 3, `AggroAbove` 0, `spots.go:110-125`); members up to `Band` 3 levels lower join, and the
  party would keep a spot whose top monster stands up to `Lmin + 5` and whose aggressive monsters
  up to `Lmin + 3`, against the band's `Lmin + 2` and `Lmin`. A member's level-up that moves `Lmin`
  or `Lmax`, or a member joining or leaving, does the same. So P2 adds `misfit`: true while the
  bot leads bot members, holds a spot, and that spot fails the band fit above while at least one
  spot of the map fits the band, in walking reach or not (when none does, the leader fits by its
  own level, and the spot it holds was chosen so; "Fit by level" above). `misfit` counts as `outleveled` does in both places: `ruleSpot` re-picks on
  it (`!v.held || b.outleveled() || b.misfit() || ...`), and `claimSpot` keeps the held spot only
  while `!b.outleveled() && !b.misfit()`, so with no band-fitting spot free the leader gives up the
  misfit spot and claims a band-fitting one past its cap (`claimOverN`, as `claimOver` serves an
  outlevelled bot today, `spots.go:527-529`) rather than keep its members at a spot too strong for
  the lowest of them. Only when every band-fitting candidate is too near its limit for `n` as well
  does it keep the misfit spot, as a solo outlevelled bot keeps its own today when it finds no trip
  (`spots.go:513-535`: a solo bot with a trip gives the spot back first, `spots.go:518-523`; the
  beside step serves only a bot holding no claim, `spots.go:530-534`): a held spot that fits its
  own level rather than a beside one, and it re-picks every `spotRetry`, moving the first time a
  band-fitting spot has room for `n`. With no band-fitting candidate at all in walking reach, it
  keeps nothing: the gated trip's give-back (section 4, "Trips") returns the misfit spot, as it
  returns an outlevelled one, so `NoSpotSince` starts. `misfit`, like `outleveled` (`spots.go:425`), reads only a held claim: a beside
  spot that leaves the band's fit is no candidate at the next re-pick, and `farmBeside` leaves it
  then (`spots.go:550-553`, above). The check runs at every decision past `retryAt`, so the
  party's levels and size, which the party window updates (`PartyMember.Level`), are re-read
  within one decision of a change, or within `spotRetry` (1 minute) after a fruitless search.
- **Gather gate:** at its spot the leader picks no new target while a living bot member stands
  farther than `GatherRange` (1200, inside the 1500 party range that pays exp) from it, for at most
  `GatherWait` (60 s); then it fights anyway. A member beyond `FarAway` (10000) does not hold it.

### 7. Party danger through the M10 model

M10's S11 (#330) fits a spot for one bot when `TimeToDie` against its likely pack stays above
`TimeToKill` times the row's margin. A party fits by the same test with party terms:

- `TimeToKill`: one pack monster under the summed damage rate of the leader and every Damage
  member, each from its `charmodel` profile;
- `TimeToDie`: the pack's damage rate on the leader (the worst case: the whole pack on whoever
  pulls) against its HP plus the healers' rate, each heal `power + √M.Atk`
  (`skill/formulas/heal.go`) over its hit time and reuse (S2's skill fields), for as long as the
  healer's MP pays for it; the Orc Shaman's 1229 Chant of Life is a heal over time (`skillType`
  HOT, 15 `HealOverTime` ticks, reuse 20 s, `skills/1200-1299.xml:557`) and counts by its ticks'
  total over its reuse instead;
- the margin: the leader's row margin.

The leader computes it: its own profile is calibrated (M10 section 3), and its bot members' (the
rows its `lead` list names, section 6) come uncalibrated from their class id and level in the
party window (`PartyMember.ClassID`, `Level`) through the class templates (S2). A player in the
party adds neither damage nor heals to the model. A party with a healer so takes spots its leader
passes over alone. The members present count: a member away on a trip or dead drops out of the
model until it is back. This changes only the model's terms, not the spot claim, whose weight is
the party's size (section 6).

### 8. Keeping the party together

| Event | What the bots do |
| --- | --- |
| A member dies | A Support member whose kit has Resurrection (Cleric, Elven Oracle, Shillien Oracle, from 20) casts it out of combat on a dead member in sight, the leader first (a new rule `raise` after `heal`; 1016 joins their learn tables). The dead bot accepts (`AcceptResurrection`) within its `ResurrectWait` (2 min in a party). Unraised, it restarts in the village and follows the leader's reported position back. |
| The leader dies | Members sit by the body as today, and a raiser raises it. Unraised, the leader restarts in the village (a teleport the members see). |
| The leader teleports away (a restart, its supply trip) | A member of a bot leader skips the wait, the whisper and `/unstuck` of `ruleLeader`. It starts its own supply trip when it carries a Scroll of Escape and nothing attacks it, so it lands in town too and restocks; else it walks to the leader's last reported position. Today's trip cannot start for a member: `ruleSupply` starts only from `supplyDue` (`supplytrip.go:113-122`), which needs a spot the bot farms, held or beside (`farming()`, `spots.go:229`), and has reached (`supplytrip.go:322-324`), and a member gives up both its claim and its beside count on joining (section 5). So P2 adds a trip cause of its own, the leader's teleport, which `ruleSupply` takes for a member with a `join` slot whose bot leader teleported, with no spot held or farmed beside, and which skips `supplyDue`'s spot test; the trip itself (the scroll, the town, the restock) is today's. The gather gate holds the leader at its next spot until they arrive. |
| A member runs low on its own | `supplyDue` reads the bot's own causes (`supplytrip.go:19-33`: a consumable running out, potions gone, load, gear) only at a spot the bot farms, held or beside (`farming()`, `spots.go:229`), and has reached (`supplytrip.go:322-324`), which a member never has (section 5). Without a change a Damage member (section 2's optional third) would lose its D16 trips altogether, and a Support member that runs out of potions or shots before its leader does would have none. So P2 gives a bot with a `join` slot, in its bot party, another place to read them: within `GatherRange` (1200) of its leader's reported position (`Party.Members`, `follow.go:154-161`) instead of at a farmed and reached spot. The rest of `supplyDue` (the `supplyCheck` cadence, `supplyTripGap` after the last trip) and the trip itself (the scroll home when nothing attacks it, the restock) are today's. It stays in the party throughout and keeps its slot; back from town it follows its leader as any member does, by the position the party window reports every 12 s at any distance. The gather gate holds the leader for it at most `GatherWait`, and not while it is beyond `FarAway`; a trip and walk back that keep it out of party range past `BehindFor` drop it as fallen behind (below). |
| A member falls behind | Online in the world and not within the 1500 party range of the leader for `BehindFor` (10 min) by the snapshots' positions (`Party.BehindSince`, section 1): the organizer drops it (`unparty`, and the leader's oust), and pairs the two no more for `RePairAfter` (30 min, `State.PairBans`). |
| A member logs out | The server takes it out (a party of two disperses). Its `join` slot stays with its `Bot` while the run lasts; the leader re-invites it when it is back, `InviteRetry` after it left at the earliest (section 4). A new run (despawn, cool-down restart) gets the slot again from the organizer (section 1). While it is away (reconnecting, or between runs) the organizer keeps its place and times it (`Party.GoneSince`, section 1); back within `LeaderGone` (5 min, the plan's leader wait) it is re-invited, and past `LeaderGone` the organizer drops it and fills its place (section 9). In a party of two the leader is left leading no party, but its `LeaderGoneSince` does not start while its only member is gone (section 1), so the two timers never run together; the leader's starts only once the member is back, with a full `LeaderGone` for the re-invitation. |
| The leader logs out | The server hands the party to the first other member. A bot made leader of bots only leaves, dispersing it; the members keep their `join` slot, stand in the join hold where the party dispersed (below) and the leader re-invites them when it is back. A party that a player now leads (the server made the player leader, or the bot made leader handed it on, section 4) is the player's: every bot in it clears its slot (section 3), and the organizer drops those bots from the bot party on its next pass (section 1), so the returning leader does not invite them. After `LeaderGone` the organizer frees the members that kept their slot; its `unparty` never takes a bot out of the player's party (section 3). |

**Errands in a bot party.** Today a bot in any party walks to no trainer, class master or dye maker
out of sight: `startTrainerWalk` returns at once while `b.world.Party.In()` (`trainer.go:469-477`),
and `pickVisit` looks at spawns out of sight only while `!b.world.Party.In()` (`visit.go:89-106`;
its callers are the class change, `classchange.go:212`, and the dyes, `dyes.go:217`). That rule
was written for a bot in a player's party, which stays with the player. A bot of a bot party is in
a party almost all the time, so under it the bot would stop learning skills while it levels: the
farmer leader would never take its D15 class change at 20, a Support member would never become a
Cleric, Elven Oracle or Shillien Oracle (1016 at 20, `classes/humanMystic.xml:369`, so the `raise`
rule above would never apply), and a mystic that reaches 7 inside the party would never buy 1011
(`classes/humanMystic.xml:50`), so its `Status.Heals` (section 2) would stay false and no party
would form around it. So P2 lifts the gate for a bot in its own bot party: one holding a `lead`
slot and leading, or a `join` slot and sitting in the party of the leader the slot names. Both
gates read `b.world.Party.In() && !b.inBotParty()` instead; the `orders.follow` gate stays, and a
bot in a player's party (no slot, or a slot cleared by the player-led-party rule, section 3) keeps
today's behavior. The errand rules (`classchange`, `train`, `dyes`) run before `leader` and
`follow` (`rules.go:494-507`), so a bot walking an errand is not pulled back by its follow slot
until the errand ends:

- **The leader** walks its errand as a solo farmer does. Its members follow it by the party
  window's positions at any distance (`follow.go:154-161`), so the party goes to town together;
  each member then takes its own errand there when its NPC comes in sight, or walks to it under
  the lifted gate, and follows its leader again once done.
- **A member** walks its own errand within the rule's reach (8000: `TrainerReach`, `rules.go:239`; `Reach`,
  `classchange.go:83`, `dyes.go:77`) and back by the party window's positions. While it is away
  the gather gate holds the leader's next target at most `GatherWait` (section 6), and an errand
  that keeps it out of party range past `BehindFor` drops it as fallen behind (table above).

No new sender: the walks are today's errand legs through the frame pacer and their bypasses
through the bypass gate (Pacing).

No bot of a bot party whispers another, as the plan's no-bot-to-bot-chat rule wants
(`docs/plan.md:343-344`): while the slot is set, `ruleLeader` skips its lost-leader branch for the bot
leader the slot names (below), and the moment the slot clears the watch can no longer hold a bot
leader. A clear from inside the bot party ends with the bot's withdrawal, whose send resets the
watch today (`actLeaveParty`, `brain.go:815-817`), or with the `withdraw` flag dropped unsent
because the party dispersed first, which resets it too (section 3); a clear by the player-led-party
rule leaves the bot under a player, whom the watch then follows as today; and every clear from no
party resets the watch itself (end of this section). So the bot never needs to tell a bot leader
from a player once its slot is gone: the lost-leader branch (the whisper after `LeaderWait`, then
`/unstuck`, `death.go:287-317`) has no bot leader left to fire for.

**A dispersed bot party.** When a bot party disperses under its members (a party of two whose
leader logged out, or a bot made leader that withdrew, section 4), the members are partyless, and
today's `watchLeader` marks their leader lost (`death.go:199-223`): `ruleLeader` then holds 5
minutes (`LeaderWait`, `death.go:164`), whispers and casts `/unstuck` to town
(`death.go:287-317`). That would recall every member to town before its leader is back to
re-invite it. So a bot with a `join` slot skips `ruleLeader`'s lost-leader hold, whisper and
`/unstuck` for any loss of its bot leader, not only a teleport, while sitting by a dead leader's
body stays as today (`byLeaderBody`).

Skipping that hold alone would not keep the member in place: today's hold (`ruleLeader` returning
`Action{}, true` until `LeaderWait`, `death.go:293-298`) is the only thing that stops a partyless
bot, since `ruleFollow` passes when `followed()` is 0 (`follow.go:97-103`), which it is outside a
party (`command.go:277-286`), and the rules after it are `newbie`, `travel`, `spot` and `farm`
(`rules.go:506-511`): the member would claim a spot and walk off to farm, against "members never
claim" (section 6). Nor can it walk to its leader's last position: `PartySmallWindowDeleteAll`
resets the bot's `Party` (`party.go:259-260`), member positions included, and the leader's
position comes only from sight or `Party.Members` (`follow.go:154-161`). So P1c adds a hold of its
own, the **join hold**, in `ruleLeader` after `byLeaderBody`: while the bot holds a `join` slot and
is in no party (or, from P1d, leads a party while its handover to a player runs, section 4),
`ruleLeader` returns `Action{}, true`, so the bot stands where the party dispersed
and waits for its leader's invitation, claiming no spot. The rules before `leader` still run
(`rules.go:474-505`: `attacked`, `heal`, `rest`, `rebuff` among them), so it still defends itself
and rests. The hold is bounded twice: the organizer ends it with `unparty` (the leader gone past
`LeaderGone`, or the member dropped, section 9), and the brain ends it itself after `JoinWait`
(6 minutes: `LeaderGone` plus two `PartyTick`s, a persona default) from when it found itself
partyless with the slot set, by clearing its slot (`Status.PartyLeft` `timeout`, section 12),
which the organizer reads as a bot that left an auto party (section 1; a pinned member is ordered
its slot again once its leader is back, section 12). `JoinWait` covers one real case and is
otherwise a defensive bound. The real case is a pinned party whose leader is gone (reconnecting,
despawned, cooling down): the organizer breaks a pinned party for none of section 9's timers, so no
`unparty` comes, and `JoinWait` lets each member play its persona instead of standing until the
leader is back. For an auto party no known path leaves a member waiting with no `unparty` coming:
a supervisor restart cancels and joins every run (`supervisor.go:279-298` `shutdown`) and the slot
lives in the run's `Bot` (section 1), so it cannot outlive its organizer; an `unparty` lost to a
busy mailbox or a reconnect is resent by the `Sent` rule (section 1); and switching the panel's
auto mode off dissolves every auto party with `unparty` orders (section 12). There `JoinWait` only
bounds the hold against an organizer bug. The same hold covers a member between invitations at
formation, whose leader invites one member at a time (section 4).

Every clear of a slot while the bot is in no party ends the join hold and resets the leader watch
(`leaderWatch{}`): an `unparty`, the brain's own clear at `JoinWait`, a panel `leave` (which then
answers nil, section 3), a panel standing order, a GM's whispered `follow`, `assist`, `stay`,
`go`, `leave` or `invite me`, and a player's `invite me` (section 10). The dispersal that put the
bot in the join hold also set the watch's `lost` time (`death.go:199-223`), and `JoinWait` (6
minutes) is counted from that same dispersal, past `LeaderWait` (5 minutes, `death.go:164`).
Without the reset the `join` slot's skip of `ruleLeader` would stop applying the moment the slot
cleared and the lost-leader branch would fire at once: the bot would whisper "where did you go?" to
its old bot leader, bot-to-bot chat the plan forbids, and cast `/unstuck` to town
(`death.go:287-317`). With it, the freed bot plays its persona's own rules from where it stands. A
bot with no slot keeps today's behavior.

### 9. Dissolving and re-forming

The organizer breaks a party (an `unparty` to every member, the leader first) when:

- its leader is gone for more than `LeaderGone` (`Party.LeaderGoneSince`, section 1): not online
  in the world (reconnecting, or no live run), or online with its `lead` delivered and leading no
  party or another while at least one bot member is online, its members never having accepted
  included;
- it has had no living healer member online for 10 minutes (`Party.NoHealerSince`);
- its leader, online in the world, has had no spot to farm for `NoSpotFor` (15 minutes,
  `Party.NoSpotSince`): no spot within walking reach fits the band (its own level only when no
  spot of the map fits the band, section 6, "Fit by level"), so it neither claims one nor farms
  beside one, and a held spot it outlevelled or that misfits the band it has given back through
  the gated trip's give-back (section 4, "Trips"; a leader farming beside a full spot has a
  spot, its snapshot's `Spot` names it, section 6). The span is longer than a death and restart, a supply trip or the
  town errands that run without a spot, each of which ends with a new claim. The leader and each
  member are then paired no more for `RePairAfter` (30 minutes, `State.PairBans`), so the group
  does not re-form in the same town and idle again while the freed leader takes its gatekeeper
  trip alone;
- no bot member is left (its last one cleared its slot, fell behind, was gone too long or was made
  ineligible):
  the leader alone gets `unparty`, never a `lead` with no names, which `Command.valid` refuses
  (section 3, `mailbox.go:137-139`); a party of two (leader and healer, section 2) reaches this
  as soon as its one member goes;
- its bot members' level spread exceeds `MaxSpread` (5): no spot fits it any more (section 2);
  a party formed within `Band` (3) has 2 levels of hysteresis before that, so a level-up does not
  break it;
- a member cleared its slot itself (a panel `leave`, a whispered command or a player's party,
  sections 3, 5 and 10), as section 1 tells it from an order not yet delivered: that member is
  dropped and its place filled, or the whole party broken when it was the leader;
- a member has been gone (not online in the world: reconnecting, or no live run) for more than
  `LeaderGone` (`Party.GoneSince`): that member is dropped and its place filled; one back sooner
  clears the timer and stays (section 1);
- a member is made ineligible on purpose (party mode `off`, a preset change out of `support` or
  `farmer`, a panel standing order, a GM's whispered order): dropped on that pass;
- its leader is made ineligible on purpose (party mode `off`, a preset change out of `farmer`, a
  panel standing order or a GM's whispered order, section 1); a leader that is only gone (offline,
  reconnecting, despawned) is timed by `LeaderGoneSince` above;
- the panel dissolves it, or switches auto parties off (section 12).

These reasons break auto parties. A pinned party breaks only as section 12 says: a bot leaving it
on purpose, or the panel.

It fills a short party with one fitting free candidate per pass, and never merges or reshuffles
working parties. A bot freed from a party waits `Cooldown` (5 min, from its `State.Freed` time)
before it is grouped again, so a broken party does not re-form in a loop. Auto parties live in the
supervisor's memory and form again after a panel restart; pinned ones (section 12) are kept in the
registry.

### 10. Real players and GMs

- **A player invites a bot that is in a bot party:** the server answers the player 160 (already in
  a party) and the bot never hears of it. The player can whisper instead:
- **`join`** (new whisper, any player, D28): to a bot party's leader with room (its free places,
  below, above zero), it invites the whisperer; a full party or a non-leader answers with a whisper
  ("party is full", "ask <leader>", naming the leader its `join` slot names). The invitation goes
  through the `party` rule's one in flight (section 4), never beside it: the leader puts the
  player on a `join` queue, owned by the brain goroutine beside the slot, and the `party` rule
  invites the queue's head ahead of the listed members not in the party (players first, D28), as
  soon as no invitation is in flight, so a `join` that arrives while a listed bot's invitation is in
  flight waits behind that one invitation (at most `InviteWait`, 17 s) and the players queued
  before it, and is not dropped. The queue never holds more players than the party has free places:
  9 less its members, less every bot its `lead` list names that is not in the party (a member
  reconnecting, in the join hold or between invitations), whose place stays kept for it; a `join`
  when every free place is queued gets "party is full". Without the listed bots in that count,
  players could fill the party to 9 while a listed member is out: the server answers the leader's
  invitation to that member 155 (`BeginInvite` returns `InviteFull` at `len(g.members) >=
  MaxMembers`, `party/party.go:217-218`; `network/party.go:85-87`), which section 4 retries every
  `InviteRetry`, and the member would stand in the join hold until `JoinWait` cleared its slot; a
  pinned member, ordered its `join` again once its leader is online (section 12), would stand idle
  about 6 of every 6.5 minutes, the loop section 12's hold-back exists to avoid. The count is
  read again when the `party` rule picks its invitee, since a listed member can drop out of the
  party after players were queued: the queue's head is invited only while the party has a free
  place by that count, and otherwise the listed members not in the party are invited first and
  the queue waits. A queued player that joined the party in the meantime, or that the
  leader invited and that refused or declined, leaves the queue; a new session empties it (the
  player whispers again). `heard` routes the `join` before the authority check, as it routes a craft order to `heardCraft` (`command.go:151-155`),
  but unlike a craft order, which skips the authority only for the clan leader and sends anyone
  else's to `askAuthority` (`craftorder.go:57-66`), a `join` never reaches `askAuthority`, so it
  costs no database query. A bot with no party slot (in no bot party, or one that cleared it)
  treats a `join` as ordinary chat, as today's bot treats any line that is no command
  (`chatHeard`), so nothing changes for bots outside bot parties. One answer per sender per
  `InviteRetry` (30 s), counted from when the `join` is heard: a place on the queue (the
  invitation that follows) or a reply whisper, whichever the `join` gets; a `join` from the same
  sender sooner, or while that sender is still queued, is dropped. The replies are command replies like the "invite me" whisper
  today: they go through the reply slot and `ruleCommand`, which holds them only by the chat gate
  (`command.go:252-258` checks only `pace.allow`), not through `chatHeard`, so
  `ChatPolicy.PerSender` (10 minutes, `chat.go:23-24,37`, shared with chat replies) does not apply:
  a player answered with chat a minute earlier still gets its `join` answered. The per-bot total
  that plan section 4 asks of every whisper reply ("capped per bot in total", `docs/plan.md:343`)
  still applies, with a count of its own: a bot sends at most `JoinReplies` (10, persona default)
  `join` reply whispers within any hour, kept in a log that outlives a session as `chatLog` does
  (`chat.go:43-46`, owned by the current session's brain goroutine), apart from
  `ChatPolicy.Total` (5 an hour, `chat.go:25-28,38-39`), so chat replies do not use up `join`
  answers. A `join` that would get a reply whisper past the cap gets none and is dropped. Without
  it any number of senders (alts) whispering `join` would get a whisper every 3 s from one bot:
  unlike today's uncapped command replies, which go only to a GM the authority confirmed
  (`command.go:170-173,194-197`), these answer any player unchecked. The invitations a `join`
  gets need no such cap: they are game actions, bounded by the queue (the party's free places)
  and the one invitation in flight.
- **A player in a bot party** is a member like any other: the Support bots heal and buff it (they
  support every member in sight, `support.go` `patients`), and the leader keeps choosing spots by
  the bot members' band; a player far above it lowers everyone's exp by the server's party-level
  rule, which is the player's choice.
- **The leadership handed to a player**: by the server, when the bot leader disconnected and the
  player was first in join order, or by the bot member the server made leader (section 4): the
  party is the player's now, so every bot in it clears its slot at once (section 3) and the
  organizer drops it from the bot party on its next pass; neither the old bot leader's
  re-invitations nor a later `unparty` reach it. The claim rule of one bot per non-GM leader
  applies again, and the claim table settles who stays. This is a new step of P6, not today's
  behavior: the server's leader change reaches every member as `PartySmallWindowDeleteAll` then
  `PartySmallWindowAll` (`party/party.go:422-424`), and today the first releases the bot's claim
  and the bot stays in the party unclaimed on purpose, a claim never following a new leader
  (`partyclaim.go:111-121`; a claim is taken only when an invitation is accepted,
  `partyclaim.go:46-48`). P6 adds the step only for a bot whose `join` or `lead` slot that very
  window clears by the player-led-party rule (section 3), on a window it reached by no invitation
  answer of its own (no `RequestAnswerJoinParty` since its last party window, so the party is the
  one it sat in, under a new leader). Such a bot asks its party hook about the new leader, the same
  `Persona.JoinParty` an invitation from that leader would ask: `Authority.Hooks` first accepts a
  commander or a GM a database check confirmed within `knownFor` (10 minutes), taking no claim
  (`knowsName`, `authority.go:274-288`, used at `authority.go:296-300`), and otherwise the claim
  table's wrapper claims the leader for the first bot that asks (`partyclaim.go:46-48`, `claim` at
  `partyclaim.go:61-73`). A bot the hook accepts stays; every other bot, Support or Damage (the
  optional third slot) alike, withdraws (`RequestWithdrawParty`, through the `withdraw` step).
  Every other bot keeps today's behavior on a leader change: a bot with no slot, in any party a
  player leads, still stays unclaimed, so the step changes nothing for bots outside bot parties. A
  commander, or a GM the hook knows, made leader so keeps every bot, as its invitation would; only
  a GM no check confirmed within `knownFor` is taken for a player, keeps one bot and can invite the
  others again, which a GM's invitation is never limited for.
- **The path of `invite me`:** unlike `join`, it goes through the authority check as today
  (`command.go` `heard`, `askAuthority`), since a GM and a player get different answers. A GM's
  verdict takes the GM path below. A refused verdict, which today `obey` drops
  (`command.go:171-173`), is dropped for every other command still; for `invite me` alone it takes
  the player path below instead. So a player's first `invite me` costs one database query, as any
  non-GM whisper does today, and later ones within the 60 s negative cache none (plan section 6;
  `authority.go` `ask`). A check that failed is refused too and takes the player path, which only
  ever asks more of a GM, never less. With no authority configured (`bots` started with neither
  commanders nor a GM database, so `startAuthority` returns a nil authority,
  `cmd/bots/gm.go:115-124`, and `askAuthority` returns before any verdict,
  `command.go:161-164`), no sender can be a GM, so
  `heard` hands an `invite me` straight to the player path; every other command is still dropped
  there, as today.
- **A player's `invite me`** (D28) acts only on a Support member of a bot party. The member first
  looks the player up in the claim table (a new read-only `PartyClaims` lookup beside `claim`,
  `partyclaim.go:61-74`): when another bot holds that player, it stays and answers nothing. Else it
  claims the player for itself at once (so a second Support member of the same or another party
  does not leave too), clears its slot, leaves its bot party and whispers "invite me" back. That
  claim must outlive the bot's own withdrawal: the server answers the leaver with
  `PartySmallWindowDeleteAll` (`party/party.go:361`; a disband sends it to every member, `:382`),
  and today's `partyClaimEvent` releases every claim on that packet (`partyclaim.go:117-121`,
  called from `brain.go:424`), so a second member that hears the same player a moment later
  would find no holder and leave too. The brain therefore marks this claim pending (the player's
  name, beside the claim): `partyClaimEvent` keeps a pending claim on the `DeleteAll` that answers
  the bot's own withdrawal and only drops the mark. From then on it is an ordinary claim out of a
  party: the player's invitation keeps it (`claim` by the same bot keeps the leader it holds,
  `partyclaim.go:61-73`) and the party window that follows holds it, and without one
  `tidyPartyClaim` lapses it `partyInviteTimeout` (15 s) after the claim (`partyclaim.go:101-108`,
  which acts only out of a party, so only after the withdrawal lands).
  To any other bot (a leader, a Damage member, a bot in no bot party) a player's `invite me` stays
  ignored, as D3 has it today.
- **A GM's `invite me`** to any bot of a bot party: the bot clears its slot, leaves (a leader
  leaving disperses the party) and whispers "invite me" back, so the GM leads (D3). A GM's `leave`
  to a bot of a bot party clears its slot and withdraws it; to the leader that disperses the party.
  With its slot cleared, the leader invites nobody back and the member declines its old leader's
  invitations (section 5), so the party does not re-form before the organizer's next pass, which
  sees the empty slot in the snapshot and drops the member or breaks the party (sections 1, 9).

### 11. Plan rule changes (`docs/plan.md`, each in the PR that ships the behavior)

`docs/plan.md` ranks above the code (`CLAUDE.md` section 2, precedence item 2), so a rule change
lands in the same PR as the behavior it describes; otherwise a review of that PR would have to
flag it against `docs/plan.md` section 3 ("at most one support bot per non-GM leader",
`docs/plan.md:257`) or section 6 ("the bot invites nobody", `docs/plan.md:431`).

| Rule change | `docs/plan.md` section | PR |
| --- | --- | --- |
| "at most one support bot per non-GM leader" gains "a bot party's members are not counted: they join their assigned bot leader outside the claim table" | 3, support | P1a (members accept their leader outside the claim table) |
| the `lead`, `join` and `unparty` commands | 8, operator commands and standing orders (`docs/plan.md:541-554`: `lead` and `join` kept by the `Bot` like `stay` and `go`, `unparty` one-off like `leave`) and the mailbox's Commands bullet (`docs/plan.md:577`) | P1a |
| `invite me` and `leave` from a GM taking the bot out of a bot party and clearing its slot first | 6, commands | P1b |
| "`invite me` (the bot invites nobody ...)" becomes "a bot invites only the bots the organizer or the panel assigned to its party; it never invites unasked" | 6, commands | P1d (the leader invites) |
| the organizer and the party mode per bot | 8, presets | P5 |
| the bot also invites "players who whisper `join`", and `invite me` "from a player takes a Support member of a bot party out, under the claim table, and is ignored by any other bot" | 6, commands | P6 |

A `docs/plan.md` tracking row for M11 and D25-D29 is added once the owner answers, with the epic.

### 12. Panel and registry

- **Registry** (`botregistry.Bot`): `PartyMode` (`""` auto, `"off"`), and for a pinned party
  `PartyGroup` (a name) and `PartyLead`, written atomically like the other fields. A pinned party
  is formed from its group whatever the band and composition rules say, and the organizer breaks
  it for none of section 9's timers or rules: a pinned member or leader that is only gone
  (reconnecting, despawned, cooling down) keeps its place for as long as it takes, and is ordered
  its slot again when it has a live run. While the pinned leader is not online in the world
  (reconnecting, or no live run), `Plan` hands the pinned members no `join`, neither a first one
  nor a resend: a member that still holds its slot stands in the join hold at most `JoinWait`
  (section 8) and then plays its persona with no slot, and it is ordered its `join` again only once
  the leader is online, then the leader its `lead` as at formation (section 1). Without that hold-back
  a member would stand `JoinWait`, clear its slot at `timeout`, be ordered `join` again on the next
  pass and stand again, idle about 6 of every 6.5 minutes for as long as the leader is gone. Its `join` and `lead` orders replace a member's panel
  standing order given before the pin (section 3); a member under a GM's whispered order answers
  `ErrWhispered` and is ordered again once the whisper no longer holds (`Status.Whispered`).
  A bot that leaves the pin on purpose is not ordered back: a member or leader whose delivered
  slot reads empty with `Status.PartyLeft` `left` (below: a panel `leave`, a panel standing order
  given after the pin, a GM's whispered `follow`, `assist`, `stay`, `go`, `leave` or `invite me`,
  a player's `invite me`, a player's party) leaves the pin, and so does a bot the panel sets to
  party mode `off` or to a preset out of `farmer` and `support`, the latest panel order winning as
  in section 3. `Plan` drops such a member from the pinned record as section 1 drops an auto
  member (its cool-down, the remaining members' shorter `join`, the leader's shorter `lead`), and
  when it was the leader, or no bot member is left, it dissolves the pinned party as the panel
  does (an `unparty` to each, the leader first); the supervisor then writes the registry's pin
  rows from the pinned records `Plan` returned, clearing that bot's `PartyGroup` and `PartyLead`
  (every bot's, for a dissolved pin), so neither the next pass nor a supervisor restart forms it
  again. Without this the pinned group would re-send `join` to a member a GM's `leave` took out
  (against D28 and "a GM's command always wins"), override a later panel order with the earlier
  pin, and set and clear the slot of a bot in a GM's party on every pass. A slot the brain cleared
  only at `JoinWait` (`Status.PartyLeft` `timeout`, section 8) is ordered again once its leader
  is online (above): the pinned leader is merely away. An auto party treats both values alike (section 1).
- **Status** (`botclient.Status`): the bot's `Party` slot (`lead ...` or `join <leader> ...`,
  empty for none) beside `Order`, and `Whispered`, true while a GM's whispered `follow`, `assist`,
  `stay` or `go` holds this session (`Order` shows only the panel's standing order,
  `status.go:91,137`), and `PartyLeft`, why the brain last cleared its own slot this run: `left`
  when a command or a player's party took it out (sections 3, 5, 10), `timeout` at `JoinWait`
  (section 8), empty while a slot is set or the organizer's `unparty` cleared it. All three ship
  in P1a, since the organizer (P5) reads them back (section 1), `PartyLeft` `left` set by P1b's clears
  and `timeout` by P1c's join hold; P7 only shows them. `Heals`, whether the bot has learned a heal
  its kit row names (section 2), ships in P5 with the healer flag that reads it.
- **Supervisor:** `Parties(ctx)` returns each party: id, leader, members (bots and players, from the
  leader's snapshot), the leader's spot, state and reason, since when, pinned. The leader's
  snapshot carries member names only (`Status.Members []string`, `status.go:80-83`, filled with
  `m.Name` alone, `status.go:155-159`), so a bot member's class, level and role come from its own
  snapshot and registry row (matched by the exact names of the party record), and a player member
  is a name only.
  `FormParty(ctx, leader, members, pin)`, `DissolveParty(ctx, id)`, `SetPartyMode(ctx, account,
  mode)` and the organizer's on/off switch go through `request` like the others. The switch
  governs auto parties only: switched off, the next pass forms no auto party and dissolves every
  auto party as `DissolveParty` does (an `unparty` to each member, the leader first, one order at a
  time by the `Sent` rule), so no leader keeps a `lead` slot whose members' slots are cleared and
  re-invites them every `InviteRetry` to be declined (sections 4, 5); pinned parties are kept and
  planned as before. Switched on, auto parties form again from the next pass, each freed bot after
  its `Cooldown`.
- **API:** `GET /api/parties`, `POST /api/parties`, `DELETE /api/parties/{id}`,
  `POST /api/bots/{account}/party`, `POST /api/parties/auto`.
- **Page:** a Parties page (each party with its bot members' class, level and role and its player
  members' names only, the spot, its state; the free pool by town; form by hand, dissolve, auto on or off), and the party slot on the
  Bots table and the Bot detail.
- **Metrics:** parties formed and broken by reason, members invited and refused.

## Pacing

| Sender | Bound |
| --- | --- |
| `RequestJoinParty` (leader) | One in flight per leader, listed bots and players queued by a `join` whisper alike (the queue's head first, section 10), until `JoinParty`, a refusal or `InviteWait` 17 s (the server's own 15 s request window, which starts when the server processes the request, plus a 2 s margin); after a 164 (the leader's own window still open) the next one `InviteWait` later; a refused, declining, silent (its invitation timed out at `InviteWait`) or departed member again after 30 s, never at once; the bot's frame bucket (5 frames/s, burst 10: at most 15 in any second and 35 in any 5 s, `pacing.go:11-16`), well inside the server's flood limits of 160 a second and 400 in 5 s that it guards. No server gate beyond that (`client_dispatch.go:147`). |
| `RequestAnswerJoinParty` (member) | Once per invitation, as today. |
| `RequestWithdrawParty` | Once per handover, `unparty` (only from the bot party), panel `leave`, standing order or whispered command that takes the bot out, through the `withdraw` step, which sends it only while the bot is in a party, clears itself on the send, and is dropped unsent when a decision finds the bot in no party or the bot answers an invitation, so it never takes the bot out of a later, unrelated party (section 3); frame bucket (`client_dispatch.go:151`). |
| `RequestOustPartyMember` (leader) | One per dropped member, one per decision; frame bucket (`client_dispatch.go:152`). |
| `RequestChangePartyLeader` (a member made leader) | Per handover with a player in the party: one request, then one more per `HandoverWait` (5 s) while no new leader window arrives (a player in a duel is refused in silence, `party/party.go:403-405`), at most `HandoverFor` (2.5 minutes, 30 requests) before the bot withdraws (section 4); frame bucket. No server gate beyond the leader check (`network/party.go:140-143`, `client_dispatch.go:324`). |
| `RequestMagicSkillUse` Resurrection | The cast path as every cast: one at a time, its `MagicSkillUse` or `castTimeout`, then its reuse. |
| Whispers (`join` replies, "invite me") | Command replies through the reply slot and `ruleCommand`, like a GM's "invite me" today: the chat gate (`chatGap`, 3 s, `pacing.go:25`; `command.go:252-258`), not `ChatPolicy.PerSender`; a `join` sender gets at most one answer (invitation or reply) per `InviteRetry` (30 s), and a bot sends at most `JoinReplies` (10) `join` reply whispers within any hour, all senders together (plan section 4's per-bot total, a count apart from `ChatPolicy.Total`); a player's "invite me" whisper back follows the bot's one withdrawal from its bot party, so once per bot party it leaves. Whispers have no server gate (`command.go:15` `chatTell`, `command.go:104-106`). |
| Supervisor orders | No packet of their own: mailbox commands every 30 s at most per bot, each causing at most the packets above. |
| Supply trip on the leader's teleport | The existing trip: one Scroll of Escape cast, the same holds as a farmer's trip. |
| Errand walks of a bot party (trainer, class master, dye maker out of sight) | No new sender: today's errand legs through the frame pacer and their bypasses through the bypass gate, as for a solo bot; only the party gate on starting the walk is lifted (section 8). |

## Owner decisions

Proposed, awaiting the owner:

| # | Question | Proposal |
| --- | --- | --- |
| D25 | How are bot parties organized? | By the panel's supervisor, from the registry and the live snapshots: online bots of the same town within walking reach and a level band. No in-game discovery. On by default in `bots ui`, with a panel switch; no organizer in `bots swarm`. |
| D26 | Who leads, who joins, how large? | A Damage-role bot (`farmer`) leads; Support-role bots (`support`) join, at least one healer (a Support bot whose kit heals another member at its level and that has learned that heal: human, elven and dark mystics from 7, Cleric, Elven Oracle and Shillien Oracle, Orc Shaman from 20 with its party heal; not an orc mystic); a third member is any other Support bot, else a second Damage bot that has no healer to lead with. Size 3, at most 4 (persona defaults). `merchant`, `crafter` and `town` never take part; a per-bot party mode `off` keeps a bot out; the panel may pin a party by hand. |
| D27 | Loot rule and level band? | Loot by turn (rule 3): items rotate, adena is split by the server in range, a spoil stays with its sweeper. A party forms with its bot members within 3 levels of each other and breaks once they spread past 5, the widest spread one spot fits under today's spot policy and the server's 5-level exp threshold; the 2 levels between are hysteresis. Players in the party are not counted. |
| D28 | How do real players get at bots in bot parties? | Any player may whisper `join` to a bot party's leader to be invited, and `invite me` to a Support member of a bot party to take it out for them (extends D3 as D20 did, for these two whispers to bots of bot parties only; a player's `invite me` to any other bot stays ignored). `invite me` still passes the authority check (with no authority configured every sender takes the player path), and a player's goes ahead only when no other bot holds that player in the claim table. A GM's `invite me` always takes the bot out, and a GM's `leave` to the leader disperses the party; each such whisper clears the bot's party slot, so the party does not re-form. |
| D29 | May bots invite? | Yes: a bot invites only the bots the organizer or the panel assigned to its party, and players who whisper `join`; it never invites unasked. Bot party members join outside the claim table. Bots of a bot party never whisper each other. |

## Testing

- **Codec (unit):** `RequestOustPartyMember` encodes opcode 0x2c and the name, and
  `RequestChangePartyLeader` 0xd0 0x0004 and the name, as the server decodes them; the five loot
  rules' values.
- **Planner (unit, `internal/botparty`, table-driven, a `State`, candidates and a fixed `now` in,
  the next `State` and orders out):**
  - `TestPlanLeaderIsDamage`: two farmers and two supports of one town form one party led by the
    higher farmer;
  - `TestPlanNeedsHealer`: a farmer and an orc-mystic support form nothing; nor a level-6 human
    mystic support, while a level-7 one with `Heals` set forms a party; a level-20 Orc Shaman
    support with `Heals` forms one too; a level-7 mystic or level-20 Orc Shaman with `Heals` unset
    (heal not yet trained) forms nothing, and a party whose only healer reads `Heals` unset starts
    `NoHealerSince`;
  - `TestPlanBand`: a support 4 levels under the farmer is left out; one 3 under joins; with a
    farmer at 20 and a healer at 17, a support at 23 (3 above the farmer, 6 above the healer) is
    not taken as the third member, while one at 20 is;
  - `TestPlanThirdMember`: a farmer F1 at 20 with a healer H1 at 19, an orc-mystic support S at
    18 and a farmer F2 at 19 standing nearer F1 than S: F1 takes H1 and S (Support first); with no
    S, F2, which has no healer, is F1's third; with a second healer H2 free, F2 leads its own
    party with H2 and F1 gets no third; with two free supports, F1 takes the nearer; with
    H2 free, no S and `FormPerPass` 1, F2's group with H2 waits for the next pass and F2 is not
    taken as F1's third meanwhile;
  - `TestPlanSameTown`: bots of two towns never share a party; nor beyond `MaxTravel`;
  - `TestPlanSkipsIneligible`: `merchant`, `crafter`, `town`, party mode `off`, a standing order,
    `Whispered` set, a reconnecting run, a player's party;
  - `TestPlanFormPerPass`: six groups ready form two parties a pass;
  - `TestPlanBreaks`: each break reason of section 9 gives `unparty` orders, the leader first, with
    its timer handed in through `State` (`NoHealerSince`, `NoSpotSince`, `LeaderGoneSince`,
    `BehindSince` set 10 minutes, 15 minutes or `LeaderGone` before `now`, and one minute short of
    it breaks nothing); a leader whose snapshot `Spot` reads empty while online starts
    `NoSpotSince`, one that claims a spot or farms beside one (`Spot` set either way) clears it, one offline starts none, and the no-spot break
    sets a `State.PairBans` entry for the leader and each member; a bot
    spread of 5 (`MaxSpread`) does not break, 6 does; a player member far above does not count; a
    party of two whose one member cleared its slot gives the leader `unparty` and no order is a
    `lead` with no names; a member reconnecting (or with no candidate) keeps its place and gets no
    order while its `GoneSince` is one minute short of `LeaderGone`, comes back and clears the
    timer, while at `LeaderGone` it is dropped (cool-down started, the remaining members' shorter
    `join` and the leader's shorter `lead` sent) and the next pass fills its place; a member whose
    party mode turned `off`, whose preset changed or that got a standing order or `Whispered` is
    dropped on that pass; a member's stale position while reconnecting starts no `BehindSince`;
    a reconnecting leader whose stale snapshot still names its party and members starts
    `LeaderGoneSince` and breaks the party at `LeaderGone`; in a party of two whose member is
    gone, the online leader leading no party starts no `LeaderGoneSince`, the member back at
    `LeaderGone` minus one minute starts it on that pass, and the party is not broken on the
    passes that follow before the re-invitation lands; a member dropped while its re-sent `join`
    is pending gets no `unparty` that pass, and the first pass after that `join` was answered nil
    orders it `unparty` (it is in no record and its slot reads set); a `forming` party whose leader is
    online with its `lead` still undelivered starts no `LeaderGoneSince`, while one whose `lead`
    was answered nil and that still leads no party breaks at `LeaderGone`; a leader snapshot
    listing a bot member turns the party `active`;
  - `TestPlanState`: a pass that forms a party returns it in `State.Parties` as `forming`; a
    condition seen for the first time sets its timer to `now` and one gone clears it; a freed bot
    gets its `State.Freed` time and a fallen-behind pair its `State.PairBans` entry; the `State`
    handed in is not changed;
  - `TestPlanFormationOrder`: a new party's first pass orders `join` to each member, naming the
    leader and the other bot members, and no `lead`; the leader's `lead` comes on the first pass
    after every member's `join` was answered nil, and not while one is pending or refused; a fill
    orders the new member's `join` and every existing member's longer `join` before the longer
    `lead`;
  - `TestPlanSlotResent`: a member whose run is new (`NewRun`) with no slot is ordered `join`
    again; a member whose order got `ErrBusy`, or was answered with an error, is ordered again;
    a member whose order is still pending (queued while it reconnects) with an empty slot is kept,
    ordered nothing and not cooled down, across several passes; only a member whose order was
    answered nil and whose slot then reads empty is dropped, its cool-down starts and the leader
    gets its shorter `lead` list; a leader so is a broken party;
  - `TestPlanCooldown`: a freed bot (its `State.Freed` time handed in) is not grouped before 5
    minutes, nor re-paired with the member that fell behind before 30 (its `State.PairBans` entry);
  - `TestPlanSlotDiffers`: a member whose order was delivered and whose slot names another leader
    than the record is ordered its recorded slot again; an `ErrBadCommand` record breaks its party
    and is not resent;
  - `TestPlanFillsSlot`: a short party gains one fitting free support per pass;
  - `TestPlanPinned`: a pinned party forms out of band and none of section 9's timers breaks it (a
    member or leader gone past `LeaderGone`, no healer, no spot, a spread past `MaxSpread`); a
    pinned member whose delivered slot reads empty with `PartyLeft` `left` (a GM's `leave`, a
    panel standing order after the pin, a GM's `invite me` that put it in the GM's party) is
    dropped from the pinned record, gets no `join` on the passes after, and the returned `State`
    no longer pins it; the leader so dissolves the pin (an `unparty` to each, the leader first);
    one with `PartyLeft` `timeout` is ordered its `join` again once its leader is online; while the
    pinned leader is not online in the world (no live run, or reconnecting) no pinned member gets a
    `join`, first or resent, on any pass; a pinned member set to party mode `off` leaves the pin;
- **Supervisor (unit, injected clock, fake runs), `-race` in CI:**
  - `TestSupervisorPartyTick` (P5): plans every 30 s and hands the orders through `Bot.Command`; a
    busy mailbox is retried next pass without blocking; an order's answer is polled without
    waiting and sets the `Sent` state the next pass reads; a new run drops its account's `Sent`
    record; the `State` one pass returns is the one the next pass gets; a pinned party whose
    registry rows (`PartyGroup`, `PartyLead`) the test writes is formed again after a supervisor
    restart; a member that left the pin has its rows cleared and is not formed again after a
    restart;
  - `TestSupervisorPartyCalls` (P7): `FormParty` writes a record into that `State` and the next
    pass orders it; `DissolveParty` breaks it; `SetPartyMode` `off` drops the bot; the auto switch
    off forms no new auto party, orders `unparty` to every member of each auto party (the leader
    first), so no leader keeps its `lead` slot, and keeps pinned ones; switched on again, a freed
    bot is grouped only after its `Cooldown`.
- **Brain (unit, events and an injected clock in, packets out):**
  - `TestPartyLeadInvitesOneAtATime`: `lead A B` sends `RequestJoinParty(A, 3)`, nothing more until
    `JoinParty`, then `RequestJoinParty(B, 3)`;
  - `TestPartyLeadInviteTimeout`: `lead A B` with no answer from A: no invitation at 15 s, B invited
    at `InviteWait` (17 s), and A, with B answered, not again before 47 s (`InviteRetry` after the
    timeout) and invited then; with `lead A` alone, A again at 47 s, nothing at 34 s; a 164 ends the invitation and the same member is invited again `InviteWait` later, not
    30 s; a 153 retries the member 30 s later, and a 185 likewise;
  - `TestPartyLeadInviteRefusals`: 619, 940 and message 1987 with the detached or jailed text end
    the invitation in flight and retry after 30 s;
  - `TestPartyLeadInviteDeclined`: a `JoinParty` with `Response` 0 ends the invitation in flight;
    that member is not invited again before 30 s, while the next listed member is invited at
    once;
  - `TestPartyLeadInsideOtherParty`: a bot with a `lead` slot that sits in a party a player leads
    sends no `RequestJoinParty` and its slot reads empty;
  - `TestPartyLeadRejoinWait`: a listed member that left the party is invited again 30 s later, not
    at once;
  - `TestPartyLeadOustsDropped`: a `lead A` after `lead A B` with B in the party sends
    `RequestOustPartyMember(B)`; a player P in the party, never listed, is not ousted;
  - `TestPartyHandoverLeaves`: `PartySmallWindowAll` naming a bot with a `join` slot leader keeps
    its slot on that window (the snapshot's `Party` still reads `join`), with or without a player
    among the members, until the handover acts (that part ships in P1b, section 3); every other member named by its slot, it sends
    `RequestWithdrawParty`, and no lost-leader wait starts;
    with a member its slot does not name among them it sends `RequestChangePartyLeader` to that
    member and no withdrawal, also when that player's name ends in the bot marker (`Talbot`), and
    with two such players to the one with the lower object id, in whatever order the windows
    named them; with that player in a duel (no new window comes), it claims no spot (the
    registry's held count is unchanged) and asks the next player `HandoverWait` (5 s) later, the
    lowest again after the last, and withdraws at `HandoverFor` (2.5 minutes) with no new leader;
    an `unparty` meanwhile withdraws it; a new window naming a player leader ends the handover with
    no further request; a bot
    without a slot made leader of a player's party sends neither and waits as today;
  - `TestPartyJoinAssignedLeader` (P1a): `AskJoinParty` from the `join` leader is accepted while
    the claim table holds that leader for another bot, and the table is unchanged; an invitation
    from a leader an earlier `join` slot of this run named is declined, with a slot set or none; a
    player's goes to the persona, also when the player's name ends in the bot marker (`Talbot`,
    `Abbot`) and the bot has a slot or none;
  - `TestPartyJoinPlayerClears` (P1b, the player-led-party rule): a bot with a `join` slot, and one
    with a `lead` slot, that accepts a player's invitation (also a player named `Talbot`) clears
    its slot on the party window that follows (the snapshot's `Party` reads empty, `PartyLeft`
    `left`);
  - `TestPartyJoinReleasesSpot`: a member holding a spot joins: the registry's held count drops;
    one farming beside a spot joins: its beside count drops;
  - `TestPartyMemberLeaderLost` (P1c, on P1a's `unparty` and P1b's clears): a member with a `join`
    slot whose party dispersed (its leader logged out), holding no spot, with a fitting spot free in the registry and a monster in
    sight, sends no whisper and no `/unstuck`, claims no spot (the registry's held count is
    unchanged), sends no move and no attack, and the current rule reads `leader` (the join hold);
    attacked meanwhile, it fights back; it accepts the returning leader's invitation and then
    follows; at `JoinWait` (6 minutes, already past `LeaderWait` since the dispersal) without one
    it clears its slot (the snapshot's `Party` reads empty, `PartyLeft` `timeout`) and plays its
    persona from where it stands, sending no whisper to its old leader and no `/unstuck` then or
    in the 10 minutes after; the same holds after an `unparty`, a panel `leave` (answered nil), a
    panel `follow` and a GM's whispered `leave` given in the join hold, each at least `LeaderWait`
    after the dispersal; a bot with no slot whose leader left waits, whispers and casts
    `/unstuck` as today;
  - `TestPartySessionReinvite`: a new session with `lead A` re-invites A without an order;
  - `TestPartyClaimN`: a party of 3 claims 3 of a spot's cap, a spot with room for 2 is skipped, and
    a member leaving re-claims 2 (held drops by 1 in one step); a member's death, its raise and its
    supply trip leave the held count unchanged; a player joining a party of 3 at a spot of cap 3
    re-claims 4 and the leader's next spot pass moves the party to a spot with room for 4 in walking
    reach; with none, it keeps its spot over cap (held 4), plans no gatekeeper trip and picks again
    `spotRetry` (1 minute) later; the
    leader's supply trip, its death and its brain's stop each return the spot's held count to its
    value before the claim; with every fitting spot at its cap, a leader of 3 holding no claim
    takes the one whose held count plus 3 stays within its limit (`claimOverN`) and skips one with
    room for 2 only; with every one too near its limit, it farms beside one (its beside count rises
    by 3, its held count is unchanged, the snapshot's `Spot` names it), a member leaving re-weighs
    the beside count to 2 in one step, the next re-pick after a spot frees room for 2 below its cap
    claims 2 there and drops the beside count by 2, and the leader's stop while beside returns the
    beside count to its value before; a solo bot's `go` claim after a leader's slot cleared takes 1
    and its later release gives back 1; a leader of 2 whose member logs out
    (`PartySmallWindowDeleteAll`) re-weighs its held count from 2 to 1 in one step and keeps the
    spot, and the member's re-invitation re-weighs it back to 2; a leader of 3 that withdraws on
    `unparty` keeps its spot at weight 1 (held drops by 2 in one step), and a whispered `go` to that
    same spot afterwards leaves the held count at 1 and its later release gives back 1; a leader of 3
    farming beside whose party disperses drops its beside count by 2 the same way; a leader whose
    slot clears while it still leads re-weighs to 1 at once;
  - `TestPartyCrowdOwnWeight`: a leader of 3 holding a spot alone (`Held` 3) counts 4 strangers as
    crowded (`Crowd` 4); with another bot's claim on it (`Held` 4), 5 strangers; a leader of 3
    farming beside a spot (`Beside` 3) that one bot holds (`Held` 1), 5 strangers; a solo bot
    holding a spot (`Held` 1) where a party of 3 farms beside (`Beside` 3) is not crowded by the
    party's 3 players and 3 others, and is by the party and 4 others (counted 1 beside, as today's
    `farmBeside` would count it, the party and 3 others would already crowd it); a solo bot's
    check is today's (`Held + Beside - 1`);
  - `TestPartySpotFitByLevels`: bot members at 10 and 15 fit only spots whose top monster is 12; at
    10 and 16 none fits; a player member at 30 changes nothing;
  - `TestPartyHeldSpotMisfit` (P2): a leader at 15 holding a solo spot whose top monster is 17
    gets a member at 12 (band `[12, 14]`): with a spot of top monster 13 (no aggressive monster
    above 12) free in walking reach it gives up its spot and claims that one; with that spot at its
    cap but room for 2 within its limit it still gives its own up and claims it past the cap; with
    no spot of the map fitting the band it keeps its own (its own level fits it); with band-fitting
    spots only beyond walking reach and a spot fitting its own level in reach, it claims neither,
    gives its misfit spot back (the gated trip's give-back, its held count drops by 2) and its
    snapshot's `Spot` reads empty. A leader at 15 holding a spot of top
    monster 15 (no aggressive monster above 12, so it fits the leader alone) gets a member at 12
    while the spot of top monster 13 is too near its limit for 2: the held spot misfits `[12, 14]`,
    the leader finds no room and keeps it, re-picking every `spotRetry`; the member's level-up from
    12 to 13 (band `[12, 15]`) makes the held spot fit again, so `misfit` clears, and when the
    spot of top monster 13 then frees room the leader stays where it is. A party window that puts
    the held spot out of the band's fit (a new member at 12 joining a band of 14 and 15 at a spot
    of top monster 16) re-picks on the next decision; a leader farming beside a spot that leaves
    the band's fit leaves it at its next re-pick; a solo bot's held spot is kept as today;
  - `TestPartyGatherGate`: a member at 2000 holds the leader's next target until it is within 1200
    or 60 s passed; a member at 12000 does not hold it;
  - `TestPartyNoGatekeeperTrip`: a leader with members plans no trip; alone it does; a leader of 3
    at 21 holding a spot of top monster 15 (outlevelled past `Outlevel` 5) with no spot in walking
    reach fitting its band or its level plans no trip, gives the spot back on its next re-pick (held
    drops by 3), farms beside nothing, and its snapshot's `Spot` reads empty, so `NoSpotSince`
    starts; it does not claim the spot back at the following re-pick; with one candidate in walking
    reach too near its limit for 3 it keeps the outlevelled spot instead and re-picks every
    `spotRetry`; a solo bot in the first case plans its trip and gives the spot back as today, and
    with no trip keeps it as today;
  - `TestPartyRaise`: a level-20 Cleric member, out of combat, with the leader dead in sight,
    selects it and casts 1016; not in combat; a level-19 Elven Mystic never (it has no 1016: the
    Elven Oracle, `baseLvl` 20, learns it at 20, `classes/elvenMystic.xml:266-267,362`);
  - `TestPartyLeaderTeleported`: a member of a bot leader that teleports sends no whisper and no
    `/unstuck`; holding no spot, with a Scroll of Escape and no attacker, it starts its supply trip;
    without one it walks to the last `PartyMemberPosition`; a bot with no `join` slot holding no
    spot starts no trip;
  - `TestPartyErrandWalks` (P2): a member with a `join` slot in its bot party, a trainer visit due
    and no trainer in sight, walks to the nearest trainer spawn within `TrainerReach`, learns, and
    then follows its leader again; at 20 with a class change due it walks to a class master's spawn
    out of sight, and with dyes due to a dye maker's; a leader with a `lead` slot does the same and
    its members follow it; a bot in a player's party (no slot, or its slot cleared by that party)
    and a bot told to follow walk to none out of sight, as today;
  - `TestPartyMemberSupply`: a Damage member and a Support member in their bot party, holding no
    spot, within 1200 of the leader, with their potions gone (or a consumable running out, or the
    load past `ReturnLoad`), start their own supply trip at the next `supplyCheck`, keep their slot
    and stay in the party, and after the restock follow the leader's reported position; beyond
    1200 of the leader, or within `supplyTripGap` of the last trip, they start none; a bot with no
    slot and no spot starts none, as today;
  - `TestPartyJoinWhisper`: a player's `join` to a leader with room sends `RequestJoinParty` to it
    with no authority query; to a full party a whisper "party is full"; to a member "ask <leader>";
    to a bot with no party slot it is ordinary chat: no `RequestJoinParty` and no authority query;
    a second `join` from the same player within 30 s gets nothing; a player the bot answered with
    chat a minute earlier still gets its `join` answered; a `join` heard while a listed member's
    invitation is in flight sends no second `RequestJoinParty`: the player is invited after that
    member's `JoinParty` and before the next listed member; that member's `JoinParty` 0 starts
    `InviteRetry` for the member, not the player, and the player's own `JoinParty` 0 starts none and
    takes it off the queue; a 164 on the player's invitation invites the player again `InviteWait`
    later; a `join` with every free place queued answers "party is full"; a leader with `lead A B`,
    A in the party and B out (reconnecting), and six players in it (8 members) answers a `join`
    "party is full" (9 - 8 - 1 = 0) and B's invitation, once B is back, is not refused 155; a
    player queued before B left the party waits while B is out and the party has no free place by
    that count, and B is invited first; eleven `join`s from
    eleven players to a full party within an hour get ten "party is full" whispers and the
    eleventh gets nothing, while a chat reply in the same hour does not lower that count;
  - `TestPartyInviteMeGM` (P1b): a GM's `invite me` to a member clears its slot, sends
    `RequestWithdrawParty`, then the whisper, both (the withdrawal through the `withdraw` step, not
    the reply slot); a whispered `go` to an unknown spot from a member sends the withdrawal, then
    the error whisper;
  - `TestPartyInviteMePlayer` (P6): a refused verdict for a player's `invite me` to a Support
    member clears its slot, sends `RequestWithdrawParty`, then the whisper, and claims the player; with that player held by another bot it
    sends nothing and keeps its slot; to a Damage member, a leader or a bot in no bot party nothing;
    a refused verdict for any other command is still dropped; with no authority configured a
    player's `invite me` to a Support member takes the same player path, and a whispered `stay`
    is still ignored; a second Support member of the same
    party that gets the same player's `invite me` after the first member's
    `PartySmallWindowDeleteAll` sends nothing and keeps its slot, and the first member's claim
    lapses 15 s after it was taken when the player invites no one;
  - `TestPartyWhisperNoReform` (P1b): a GM's `leave` to a member clears its `join` slot (the
    snapshot's `Party` reads empty, `PartyLeft` `left`), sends `RequestWithdrawParty` through the
    `withdraw` step, and the member declines its old leader's next invitation (an `AskJoinParty`
    the test hands in, since no bot leads before P1d); a GM's `invite me` to a member does the same
    before its whisper; a GM's `leave` to a bot with a `lead` slot in its party clears the slot and
    withdraws;
  - `TestPartyWhisperNoReformLeader` (P1d, the `party` rule): after a GM's `leave` to the leader,
    the leader sends no `RequestJoinParty`; after a member's `invite me`, the leader's next
    invitation to it comes 30 s later, the member declines it, and the leader's next one after that
    comes 30 s after the decline;
  - `TestPartyWhisperedOrderClears`: a GM's whispered `stay` or `go <spot>` to a member clears its
    slot inside `obey` (the snapshot's `Party` reads empty), and its first packet after is
    `RequestWithdrawParty`, ahead of any move to the spot; a `go`'s claim is already in the
    registry when `obey` returns (`goToSpot` claims synchronously, `command.go:189-192,238`, while
    the withdrawal goes out later through `ruleCommand`, `command.go:252-258`), so the test asserts
    the registry state after `obey` and the packet order, not a withdrawal before the claim; a panel `stay`, `go`, `follow` or
    `assist` to a member in its bot party clears its slot and sends `RequestWithdrawParty` (the
    `withdraw` flag survives `holdStill`, `orderPlayer` and the panel `go`'s reset); a panel
    `follow` to a partyless member
    with a `join` slot clears the slot and sends no withdrawal, and the leader's next invitation is
    declined; while a GM's whispered `follow` holds, `join` and `lead` answer `ErrWhispered`, set no
    slot and the snapshot reads `Whispered`;
  - `TestPartyWithdrawLapses`: a GM's `leave` to a member of its bot party sets the withdrawal;
    when `PartySmallWindowDeleteAll` (a disband) arrives before the next decision, no
    `RequestWithdrawParty` is sent, no lost-leader whisper or `/unstuck` follows, and after the bot
    accepts a GM's invitation and gets the GM's `PartySmallWindowAll` it sends no
    `RequestWithdrawParty`; with the disband and an `AskJoinParty` handled before one decision, the
    bot answers the invitation and sends no withdrawal after the new window; with a leader change
    (`PartySmallWindowDeleteAll` then `PartySmallWindowAll` naming another leader) in between, the
    withdrawal still goes out;
  - `TestPartyPanelLeaveClears`: a panel `leave` to a member clears its `join` slot and withdraws;
    the old leader's next invitation is declined; a panel `leave` to the leader clears its `lead`
    slot, withdraws and invites nobody after; a panel `leave` to a partyless member with a `join`
    slot, or a partyless leader whose members have not joined, clears the slot, sends nothing and
    answers nil; to a partyless bot with no slot it answers `ErrNoParty` as today;
  - `TestPartyJoinReplacesStanding`: a `join` to a bot with a standing `follow X` clears the
    standing order (the snapshot's `Order` reads empty), the bot accepts its leader's invitation
    and follows the leader, not X, and the next session applies no standing order; a `lead` to a
    bot with a standing `stay` likewise;
  - `TestPartyUnparty`: `unparty` to a member of its bot party clears the slot and withdraws; to a
    bot in a party a player leads, or in none, it sends nothing;
  - `TestPartyCommandValid`: `lead A B` and `join L A B` with single spaces, 1 to 8 names, are
    valid; a ninth name, a repeated name, a double space or a name of 17 characters answers
    `ErrBadCommand`; `unparty` takes none; the panel's raw command route refuses `lead`, `join` and
    `unparty` with `ErrBadCommand`;
  - `TestPartyHandoverToPlayer`: leadership passing to a player clears every bot's slot; the bot
    whose claim of the player succeeds stays, and every other bot, a Damage member included, leaves;
    a later `unparty` to it sends nothing; the returning bot leader's invitation to it is refused
    (160) at most until the organizer drops it; leadership passing to a commander, or to a GM a
    check confirmed within `knownFor`, clears every slot, takes no claim and keeps every bot; a bot
    with no slot in a player's party whose leader changes to another player stays unclaimed and
    sends no withdrawal, as today (`partyclaim.go:111-121`).
- **Model (unit):** `TestPartyFitHealer`: the same leader and pack fail alone and fit with a fixed
  healer profile; a healer with no MP left adds nothing.
- **Panel (unit, `httptest`):** the party endpoints through the supervisor; no secret in any
  response. Vitest: the Parties page and the form dialog.
- **e2e (live harness), at most 4 bots a scenario.** The live harness runs no supervisor
  (`internal/e2e/live.go` uses `botregistry` only for `PanelPrefix`, lines 12 and 43), so no
  organizer runs there: each scenario forms its party by handing `join <leader> <other members>`
  to each member through `Bot.Command` and waiting for each nil answer, and only then
  `lead <members>` to the leader, the organizer's own order (section 1), as the scenarios already
  hand panel orders (`arrows_live_test.go:125`). The organizer itself is covered by the planner
  and supervisor unit tests above. The harness keeps naming its players with `DefaultMarker`
  (`helpers_live_test.go:34`): no party rule reads the marker (section 5), so the existing
  scenarios where such a player invites a bot and expects 1 (`party_live_test.go:56`,
  `support_live_test.go:304`, `recover_live_test.go:116-122`) pass unchanged after P1a, and P1a
  runs them as its regression check.
  - `TestLiveBotPartyForms` (3 bots): a farmer and two supports of one town form a party; the
    members follow; after the leader's kills every member within the party range gains exp (its
    `characters` exp row grows). Whether a member hit the monster is not asserted: Support members
    assist the leader's target (`support.go` `leaderHit`), and the server pays exp in range
    attacker or not (table above).
  - `TestLiveBotPartyLeaderDeath` (3 bots): the GM kills the leader at the spot (`//kill`); the
    party is farming again together within a timeout, as the same party.
  - `TestLiveBotPartyRaise` (3 bots): a human mystic support set to level 20 by `//set level`
    becomes a Cleric through M10's class change at 20 (S8, #328; the harness's GM forms set level,
    SP and skills, not the class, `internal/e2e/gm.go:152-154`), walking to the class master while
    in its bot party under P2's lifted errand gate (section 8). `//set level` gives no SP, and
    1016 costs 3300 SP at 20 (`classes/humanMystic.xml:369`), so the GM then gives the Cleric the
    skill with `//skill set 1016 1` (P3's learn table is covered by its unit test); the Cleric
    raises the killed leader.
  - `TestLiveBotPartyPlayers` (2 bots, the harness's GM and a plain player, whose name carries the
    marker as every harness player's does): the player's `join` gets an invitation all the same;
    the GM's `invite me` takes a member out and it whispers back.

## Delivery (milestone M11, "Bot parties")

| PR | Scope | Depends on |
| --- | --- | --- |
| P1a | Party orders and the slot: the `lead`, `join <leader> <members>` and `unparty` commands and their `Command.valid` forms, the `Bot`'s party slot, its known bot leaders, `Status.Party` (published before the order is answered), the `Status.Whispered` and `Status.PartyLeft` fields, `ErrWhispered`, `lead` and `join` replacing a panel standing order, the raw command route refusing the party kinds, `unparty` withdrawing only from the bot party and resetting the leader watch; members accepting their leader outside the claim table and declining only the bot leaders they know by name, never by the marker (the existing e2e scenarios with marked harness players stay green) (sections 1, 3, 5, 12); its `docs/plan.md` rule changes (section 11). Unit tests only: no bot leads yet. | — |
| P1b | Slot clearing (section 3): the `withdraw` step sent before the reply, kept by every reset of `orders` and dropped unsent from no party or on the bot's own invitation answer; the panel `leave` (a slot cleared from no party answers nil), a panel standing order (in a party or between invitations), a whispered `follow`, `assist`, `stay` or `go`, a GM's whispered `leave` and a GM's `invite me`, each clearing the slot before the withdrawal; a party led by a name the slot does not hold clearing the slot, except a `join` member leading its own party; `Status.PartyLeft` `left`; every clear from no party resetting the leader watch (section 8); its `docs/plan.md` rule change (section 11). Unit tests only. | P1a |
| P1c | The member's hold (sections 5, 8): a `join` member skipping `ruleLeader`'s lost-leader hold, whisper and `/unstuck`; the join hold with its `JoinWait` bound, `Status.PartyLeft` `timeout` and the leader-watch reset at that clear; releasing the spot on joining. Unit tests only. | P1a, P1b (its test's clears from no party: the panel `leave`, a panel `follow` and a GM's whispered `leave` in the join hold, each resetting the leader watch) |
| P1d | The leader: the `party` rule with its refusals, declines, `InviteWait` timeout, 164 handling and rejoin wait; the five loot rules; `RequestOustPartyMember` and `RequestChangePartyLeader` (encoders) with the oust of a dropped member; the handover by exact names, which `watchLeader` no longer counts as a lost leader, with its `HandoverWait` retry over the players, the join hold while it runs, the withdrawal at `HandoverFor` and `unparty` withdrawing it (sections 3, 4); its `docs/plan.md` rule change (section 11). | P1a, P1b (the handover's exception to the player-led-party rule), P1c (members hold between invitations) |
| P2 | Party play: `claimN`, `claimOverN` within the limit, `farmBesideN`, `releaseN` and `leaveBesideN` with the brain's weight for a claim or a beside count, `leaveSpot` giving back that weight from either, `reweigh` of a held or beside count on a size change, the crowd check's own weight (`Held + Beside - spot.weight`), fit by the bot members' levels, `misfit` re-picking a held spot the band no longer fits (section 6), the gated trip's give-back of an outlevelled or misfit held spot with no candidate in walking reach (section 4), the weight back to 1 when a leader stops leading (section 6), errand walks for bots of their bot party (section 8), the gather gate, no gatekeeper trips while leading, the member's answer to its leader's teleport with a new supply-trip cause that needs no held spot, a member's own supply causes read at its leader's side (sections 6, 8; the whisper skip ships in P1c). | P1d |
| P3 | Resurrection: 1016 in the Cleric, Elven Oracle and Shillien Oracle learn tables and the `raise` rule (section 8). | P1a, S7 (#327) |
| P4 | Party danger through the model: party `TimeToKill` and `TimeToDie` with heals; the spot fit for a party (section 7). | P2, S3 (#322), S11 (#330) |
| P5 | The organizer: `internal/botparty` planner, the supervisor's tick and orders (`TestSupervisorPartyTick`), the registry's party mode and pins, roles and the healer flag per candidate with `Status.Heals` (the learned heal, section 2), `NewRun`, the `State` it returns (parties, freed times, pair bans, break timers with the per-member `GoneSince` and the online-only `LeaderGoneSince`, which does not run while every bot member is gone), dropping a gone or ineligible member (its `unparty` held back while an order is pending, then sent to a candidate in no record whose slot is set), the `Sent` record of each order's answer and the slot re-send, `join` before `lead`, breaks (no bot member left and the leader's `NoSpotFor` among them) and cool-downs, pinned parties kept through every timer, their `join` orders held back while the pinned leader is not online in the world, and a bot leaving its pin by `Status.PartyLeft`, with the registry's pin rows written from the pinned records (sections 1, 2, 9, 12); its `docs/plan.md` rule change (section 11). | P1a-P1d, S4 (#323), S7 (#327, the healer flag), S9 (#331, the `farmer` preset) |
| P6 | Players and GMs: the `join` whisper and its queue in the `party` rule's one invitation in flight, the `JoinReplies` cap on its reply whispers, a player's `invite me` path (the refused verdict and the no-authority route to the player path, the claim lookup, the pending claim kept across the bot's own withdrawal; a GM's `invite me` and `leave` clearing the slot ship in P1b), the handover to a player as a new step (today a bot stays unclaimed on a leader change, `partyclaim.go:111-121`): only a bot whose slot that leader-change window clears asks its party hook about the new leader, so a commander or a GM the hook knows keeps every bot and a player keeps the one whose claim succeeds, while every other bot keeps today's behavior (section 10), and its `docs/plan.md` rule changes (sections 3, 4, 10, 11). "Support member" is the role from the preset that S4 adds. | P1d, S4 (#323, roles) |
| P7 | Panel: the supervisor's party calls (`FormParty`, `DissolveParty`, `SetPartyMode`, the auto switch, whose off dissolves every auto party; `TestSupervisorPartyCalls`), the API, the Parties page, the party slot on the Bots table and the Bot detail, metrics (section 12). | P5 |
| P8 | e2e live scenarios for P1a-P7, at most 4 bots each; the harness keeps its marked player names (no party rule reads the marker). | each; the raise scenario also P2 (the errand gate) and S8 (#328) |

Most urgent: P1a-P1d and P5 give parties at all; P2 makes them play as a group.

## Risks

- **Exp per member.** Three equal members each get 1.39 / 3 = 46 % of a solo kill's exp, so a
  party gains only where it kills more than twice as fast or rests less. The healer's sustain and
  the model's stronger spots (P4) are what pay for it; without P4 parties may level slower than
  solo farmers. No test measures this: a live run's kill rate is timing-bound and no e2e scenario
  asserts it, so the comparison of a party's exp per hour against a solo farmer's on the owner's
  home run is a follow-up (Gaps).
- **Fewer bots for players.** Support bots in bot parties are not free for player invitations; the
  `join` whisper and D28 give them back on request.
- **Churn.** Re-invites after every reconnect, breaks and fills. Cool-downs, hysteresis, one
  invitation at a time and `FormPerPass` bound it.
- **Walking members.** Members follow by walking only, so a leader that strays far (a trip, a long
  route) strands them; the no-gatekeeper rule and the fall-behind break bound it. The other side
  of the no-gatekeeper rule is a party with no spot in walking reach of its town, most often
  because its leader levelled at its spot until it outlevelled it: the leader gives that spot back
  (the gated trip's give-back, section 4) and the no-spot break (`NoSpotFor`, section 9) frees it, so the leader travels alone and the party re-forms only
  where its bots meet again, at the cost of party play for a band that outgrew its town.
- **Model error.** The party model inherits the M10 model's approximations and adds the
  uncalibrated member profiles; it only compares spots, and the leader's row margin applies.

## Gaps

To be filed in M11 with the epic:

- `bots swarm` has no organizer (D25); headless runs keep today's party behavior.
- A party travelling together by gatekeeper or recalling together on purpose (the leader's trip
  ends gatekeeper travel while leading; P2 covers only the teleport answer). Until then a party
  with no spot in walking reach, an outlevelled or misfit held spot given back included
  (section 4, "Trips", P2), is broken after `NoSpotFor` (section 9, P5), which ships with the
  organizer.
- Party matching rooms (`network/partymatch.go`) to list bot parties for players: backlog.
- Command channels: out of scope.
- Measuring a bot party's exp per hour against a solo farmer of the same level on the home run,
  once P4 lands (Risks, "Exp per member"): a follow-up, since no test can assert it.
