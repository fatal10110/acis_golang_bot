# Real players and GMs

Ships in P6, except a GM's `invite me` and `leave` clearing the slot (P1b,
[orders](orders.md#slot-clearing)). A player inviting a bot of a bot party is answered 160 and the
bot never hears of it; the player can whisper instead.

## join

A new whisper from any player (D28) to a bot party's leader with a free place: it invites the
whisperer. A full party answers "party is full"; a non-leader "ask <leader>", naming its `join`
slot's leader. A bot with no slot treats `join` as ordinary chat (`chatHeard`). A player in a bot
party is a member like any other (healed and buffed, `support.go` `patients`), outside the band;
one far above lowers everyone's exp by the server's party-level rule, its choice.

- **Routing:** `heard` routes `join` before the authority check, as a craft order goes to
  `heardCraft` (`command.go:151-155`), but never to `askAuthority` (unlike a craft order,
  `craftorder.go:57-66`): no database query.
- **Queue:** the leader queues the player (owned by the brain goroutine beside the slot); the
  [party rule](leader.md#invitations) invites the head ahead of listed members not in the party
  (players first, D28) once no invitation is in flight. A queued player that joined, or was invited
  and refused or declined, leaves the queue; a new session empties it.
- **Free places:** 9 less the members, less every listed bot not in the party (reconnecting, in the
  join hold, between invitations). A `join` when every free place is queued gets "party is full".
  Re-read when the `party` rule picks its invitee: the head is invited only while a place is free,
  else the listed members first, so players never fill a listed member's place into 155s
  (`party/party.go:217-218`, `network/party.go:85-87`).
- **One answer per sender per `InviteRetry`** (30 s from when heard): a queue place or a reply
  whisper; a sooner `join`, or one while queued, is dropped.
- **Replies** are command replies through the reply slot and `ruleCommand`, held only by the chat
  gate (`command.go:252-258`), so `ChatPolicy.PerSender` (10 minutes, `chat.go:23-24,37`) does not
  apply. Plan section 4's per-bot total (`docs/plan.md:343`) applies as its own count: at most
  `JoinReplies` (10, persona default) `join` reply whispers in any hour, in a log that outlives a
  session as `chatLog` does (`chat.go:43-46`, owned by the current session's brain goroutine),
  apart from `ChatPolicy.Total` (5 an hour, `chat.go:25-28,38-39`); past it a `join` gets nothing.
  Today's uncapped command replies go only to a confirmed GM (`command.go:170-173,194-197`); these
  answer any player but a name carrying the bot marker (plan section 4, `docs/plan.md:344`, as
  `answerOrder` skips it, `craftorder.go:190`), as does a player's `invite me` whisper-back; the
  invitation and the claim ignore the marker. Invitations need no cap: the queue and the one in flight bound them.

## Handover to a player

By the server (the bot leader disconnected, the player first in join order) or a bot made leader
([leader](leader.md#handover)): every bot clears its slot (player-led-party rule) and is dropped by
the organizer, out of reach of the old leader's invitations and `unparty`. A new step of P6: today
the leader change (`PartySmallWindowDeleteAll` then `PartySmallWindowAll`, `party/party.go:422-424`)
releases the claim and the bot stays unclaimed (`partyclaim.go:111-121`; claims come only from
accepted invitations, `partyclaim.go:46-48`). P6 acts only for a bot whose `join` or `lead` slot
that very window clears, reached by no invitation answer of its own (no `RequestAnswerJoinParty`
since its last window). It asks its party hook about the new leader, as an invitation would
(`Persona.JoinParty`): `Authority.Hooks` accepts a commander or a GM confirmed within `knownFor`
(10 minutes) with no claim (`authority.go:274-288,296-300`), else the claim table claims the leader
for the first bot asking (`partyclaim.go:46-48,61-73`). A bot accepted stays; every other bot,
Support or Damage, withdraws through the `withdraw` step. A commander or known GM so keeps every
bot; an unconfirmed GM counts as a player and keeps one bot. A bot with no slot keeps today's behavior.

## invite me

**Path:** through the authority check as today (`command.go` `heard`, `askAuthority`). A GM's
verdict takes the GM path. A refused verdict, dropped by `obey` today (`command.go:171-173`), is
still dropped for every other command, but for `invite me` takes the player path (one query, then
none within the 60 s negative cache, plan section 6, `authority.go` `ask`); a failed check counts
as refused. With no authority (`startAuthority` returns nil, `cmd/bots/gm.go:115-124`;
`askAuthority` returns early, `command.go:161-164`), `heard` hands `invite me` to the player path.

**A player's** (D28) acts only on a Support member of a bot party. It looks the player up in the
claim table (a new read-only `PartyClaims` lookup beside `claim`, `partyclaim.go:61-74`): held by
another bot, it stays silent. Else it claims the player at once, clears its slot, leaves and
whispers "invite me" back. The claim is marked pending (the player's name beside it), so
`partyClaimEvent` (`partyclaim.go:117-121`, from `brain.go:424`) keeps it on the
`PartySmallWindowDeleteAll` that answers the bot's own withdrawal (`party/party.go:361`; `:382` on
a disband) and only drops the mark: a second member hearing the same player does not leave too.
Then it is an ordinary claim: the player's invitation keeps it (`partyclaim.go:61-73`), else
`tidyPartyClaim` lapses it `partyInviteTimeout` (15 s) after it was taken (`partyclaim.go:101-108`, out of a party only, so after the withdrawal lands).
To any other bot a player's `invite me` stays ignored (D3).

**A GM's** to any bot of a bot party: it clears its slot, leaves (a leader disperses the party) and
whispers "invite me" back, so the GM leads (D3); a GM's `leave` clears and withdraws. Nobody
re-invites and the member declines its old leader until the organizer drops it.

## Tests (brain unit, P6)

- `TestPartyJoinWhisper`: to a leader with room, `RequestJoinParty` with no authority query; full,
  "party is full"; to a member, "ask <leader>"; to a bot with no slot, chat (no invitation or
  query); again within 30 s, nothing; a player answered with chat a minute earlier still answered;
  during a listed member's invitation, no second one: the player goes after that `JoinParty`, before
  the next listed member; that member's `JoinParty` 0 starts its `InviteRetry`, the player's starts
  none and dequeues it; a 164 on the player re-invites 17 s later; every place queued, "party is
  full"; `lead A B`, A in, B out, six players (8 members): "party is full" (9 - 8 - 1 = 0), and B is
  not refused 155; a player queued before B left waits while no place is free, B first; eleven
  `join`s to a full party in an hour: ten replies, the eleventh nothing, chat replies not counted; a
  marked player: invited, never a reply whisper.
- `TestPartyInviteMePlayer`: a refused verdict to a Support member clears, withdraws, whispers and
  claims the player; the player held by another bot: nothing, slot kept; to a Damage member, a
  leader or a bot in no bot party: nothing; any other refused command dropped; no authority: the
  same path, a whispered `stay` still ignored; a second Support member hearing it after the first
  one's `PartySmallWindowDeleteAll`: nothing, slot kept; the claim lapses 15 s after it was taken
  with no invitation.
- `TestPartyHandoverToPlayer`: leadership passing to a player clears every slot; the bot whose claim
  succeeds stays, every other (a Damage member included) leaves; a later `unparty` to it sends
  nothing; the returning bot leader's invitation is refused (160) until the organizer drops it; to a
  commander or a GM confirmed within `knownFor`: slots cleared, no claim, every bot kept; a bot with
  no slot whose player leader changes to another player stays unclaimed, no withdrawal
  (`partyclaim.go:111-121`).
