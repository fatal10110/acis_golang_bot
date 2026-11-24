# The leader

A bot with a `lead` slot ([orders](orders.md)), P1d. Spot and trips: [spots](spots.md).

## Invitations

A new rule `party`, after `announce` (every rule from `dead` to `chat` goes first, `rules.go:475-483`): one `RequestJoinParty` at a
time, only while the bot is in no party or leads its own. The invitee is the head of the `join`
queue ([players](players.md#join)), else the first listed member not in the party. The in-flight
state records its invitee, since the server answers a second invitation 164
(`network/party.go:71-74,88-90`, `party/party.go:219`) and `JoinParty` names no one
(`party.go:24-25`, `reader.go:101`).

In flight ends on `JoinParty`, a refusal the bot reads (152, 153, 154, 155, 160, 164, 185, 619,
940, or 1987 naming a detached or jailed target), or `InviteWait` (17 s: the server's 15 s window
starts when it processes the request, so 2 s margin). Then:

| End | Effect |
| --- | --- |
| 164 | The leader's own window: no `InviteRetry`; the next invitation `InviteWait` later. |
| `InviteWait`, no answer (a silent client, the Olympiad, `network/party.go:67-69`) | The next listed member at once; the silent one only `InviteRetry` (30 s) after the timeout. |
| Any other refusal | That member again after `InviteRetry`. |
| `JoinParty` `Response` 0 (`network/party.go:126`) | Like a refusal. A member whose slot no longer names this leader declines at once ([members](members.md#joining)). |
| A listed member left (`MsgLeftParty` for a withdrawal or a disconnect, `party/party.go:354-360`), or the party it led dispersed (a party of two when the member left: `PartySmallWindowDeleteAll` and 203, no `MsgLeftParty`, `party/party.go:337-338,370-386`), every former member counting as left | Invited back only after `InviteRetry`, so the organizer can drop one that left on purpose. |

A `join` player gets no `InviteRetry`: any end takes it off the queue, except a 164, which puts it
back at the head. Every invitation carries loot rule 3, by turn (D27); the server keeps the
party's rule for later ones. The bot names all five loot rules as constants (today
`encodeRequestJoinParty`'s comment lists three).

## Oust

A member dropped from the `lead` list is expelled with `RequestOustPartyMember` (a new encoder),
one per decision; only a bot once on a `lead` list, never a `join` player.

## Handover

A `join` bot that finds itself leading (`PartySmallWindowAll` with its own id as leader: its bot
leader disconnected, `party/party.go:345-347,418-427`) keeps its slot
([orders](orders.md#slot-clearing)); `watchLeader` does not count it as a lost leader.

- Every other member is a bot its slot names: it withdraws at once, dispersing the party
  (`party/party.go:337`); the members' slots still name the old leader, who re-invites them.
- A member its slot does not name is a player: it hands the party to the player with the lowest
  object id (`RequestChangePartyLeader`, a new encoder), since `World.Party` is a map by object id
  (`party.go:198-206`, rebuilt by `applyParty`, `party.go:231-250`) that loses the join order. Then
  [handover to a player](players.md#handover-to-a-player) applies.

A dueling player is refused in silence, so `handover` (players tried, last request time) is a brain
goroutine state under the [join hold](members.md#join-hold), else the bot follows itself
(`follow.go:99-102`, `command.go:282-285`) and claims a spot. With no window naming a new leader
within `HandoverWait` (5 s, persona default) it asks the next object id, back to the lowest after
the last; with none within `HandoverFor` (2.5 minutes, past a duel's 2), or no player left in the
window (re-read each request), or an `unparty`, it withdraws, dispersing the party
(`party/party.go:336-339`). A bot without a `join` slot leading a player's party keeps today's wait;
before P1d no bot leads, so that arises only in unit tests.

## Tests

Codec (unit): `RequestOustPartyMember` encodes 0x2c and the name, `RequestChangePartyLeader` 0xd0
0x0004 and the name, as the server decodes them; the five loot rules' values.

Brain (unit, events and an injected clock in, packets out):

- `TestPartyLeadInvitesOneAtATime`: `lead A B` sends `RequestJoinParty(A, 3)`, nothing until
  `JoinParty`, then `RequestJoinParty(B, 3)`. `TestPartySessionReinvite`: a new session with `lead
  A` re-invites A without an order.
- `TestPartyLeadInviteTimeout`: `lead A B`, A silent: nothing at 15 s, B at 17 s, A (B answered)
  not before 47 s and then; `lead A` alone: A at 47 s, nothing at 34 s; a 164 re-invites the same
  member 17 s later, not 30 s; a 153 or 185 retries 30 s later. `TestPartyLeadInviteRefusals`: 619,
  940, and 1987 (detached or jailed) end it, retrying after 30 s. `TestPartyLeadInviteDeclined`:
  `JoinParty` 0 ends it, that member not before 30 s, the next at once. `TestPartyLeadRejoinWait`: a
  listed member that left is invited again 30 s later, not at once, also from a dispersed party of two.
- `TestPartyLeadInsideOtherParty`: a `lead` bot in a player's party sends no `RequestJoinParty`, its
  slot empty. `TestPartyLeadOustsDropped`: `lead A` after `lead A B` with B in sends
  `RequestOustPartyMember(B)`; a never-listed player is not ousted.
- `TestPartyHandoverLeaves`: a window naming a `join` bot leader keeps its slot, with or without a
  player, until the handover acts (that part ships in P1b); all others named: it withdraws, no
  lost-leader wait; an unnamed member: `RequestChangePartyLeader` to it and no withdrawal, also for
  `Talbot`, and with two players to the lower object id whatever the window order; that player
  dueling: it claims no spot (held unchanged), asks the next 5 s later, the lowest after the last,
  and withdraws at 2.5 minutes; an `unparty` meanwhile withdraws; a window naming a player leader
  ends it; a bot without a slot made leader of a player's party sends neither.
- `TestPartyWhisperNoReformLeader`: after a GM's `leave` to the leader no `RequestJoinParty`; after
  a member's `invite me`, the leader's next invitation to it comes 30 s later, is declined, and the
  next 30 s after the decline.
