# Party-matching rooms (#438)

A partyless support bot volunteers for the party-matching rooms (Alt+L) players open near it: it
asks to enter a room, then waits there for the leader's party invitation, which it answers as any
invitation. Entering a room is not entering the party: the leader still invites
(`RequestJoinParty`). The bot never invites anyone here, so D29 (a bot invites only its assigned
bots and the players who whisper `join`) is untouched.

Code: `internal/botclient/partymatch.go`, rule `rooms`. Server:
`internal/gameserver/network/partymatch.go`, `partymatch/partymatch.go`,
`network/clientpackets/partymatch.go`, `network/serverpackets/partymatch.go`.

## Who volunteers

A bot whose persona sets `RoomPolicy.Join` (on in `DefaultPersona`, so the `support` preset; off in
`TownPersona`), plays `RoleSupport`, has a `JoinParty` hook, and runs in a process with a claim
table (`PartyClaims`, every `bots` run). It must be alive, out of combat, in no party, without a
party slot (a bot party's members join their organizer's party), without an open invitation, holding
no other claim, and on no errand or trip (`onErrand`).

## Steps

1. **List** every `ListEvery` (default 2 min, plus up to half again at random; the first one a
   random part of it after the bot may first volunteer): `RequestListPartyWaiting` (`0x6f`: `Auto`
   1, `Location` -2 NearMe, `LevelMode` 0 LevelsMine). The server puts the bot on its waiting list
   and answers `PartyMatchList` (`0x96`) with the rooms whose leader stands in the bot's world
   region, whose level range holds its level and that are not full.
2. **Pick** the first room whose leader is a player (no name with the bot marker: bot parties are
   the organizer's), not the bot, that has a place and fits its level, whose leader is not in the
   bot's retry list, and whose leader the bot claims in the claim table (one bot per leader, as for
   invitations).
3. **Join**: `RequestJoinPartyRoom` (`0x71`: room id, -2, 0). Entering answers `PartyMatchDetail`
   (`0x97`) and takes the bot off the waiting list; a refusal (full, level, no such room) is system
   message 1413; a bot off the waiting list hears nothing.
4. **Wait** `Wait` (default 2 min) for the leader's `AskJoinParty`; rule `prompt` answers it through
   the persona's `JoinParty`, whose claim the bot already holds. The claim's 15 s lapse
   (`tidyPartyClaim`) skips the room join's leader while it waits.
5. **Leave**: a list with no room to pick, or an unanswered list (5 s), ends in
   `RequestExitPartyMatchingWaitingRoom` (`d0:0017`, no body, no answer). A refusal also exits; a
   wait that runs out leaves the room (`RequestWithdrawPartyRoom`, `d0:0003`: room id, an unused
   int). Both give the claim back and put the leader in the retry list for `Retry` (default 15 min);
   the next list waits a whole `ListEvery`. A room closed on the bot (`ExClosePartyRoom`, `fe:000f`)
   ends in an exit too, as the server may have put it back on the waiting list.
   A bot in the room that joins a party its leader does not lead (it took another invitation)
   withdraws too: the server takes a room member out only when it leaves its party.

## Pacing

The server limits none of these packets beyond the connection's frame flood gate
(`network/client_dispatch.go`). The bot's limits are operational constants, not persona defaults
(D6): `ListEvery` is at least 30 s (`roomListMin`) per bot, the bots of one process list at most
once per 500 ms together (`roomListGap`, `PartyClaims.listTurn`), one join per list and per leader
per `Retry` (at least 2 min, `roomRetryMin`), one exit per list or refusal and one withdrawal per
room; everything under the frame bucket.

## Decisions

- **One bot per leader.** The issue asks for up to the room's free places; `docs/plan.md` section 3
  keeps "at most one support bot per non-GM leader", which outranks it, so a room gets at most one
  volunteer of a process. A GM's room is claimed like any other: volunteering is unasked, unlike a
  GM commander's invitation.
- **Rooms that list bot parties** (the other direction: a bot leader opening a room) stay out of
  scope.
