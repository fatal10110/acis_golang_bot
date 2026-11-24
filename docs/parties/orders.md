# Orders, the party slot and slot clearing

The bot side of [organizer](organizer.md) orders: commands, slot, status in P1a; clearing in P1b.

## Commands

`lead <name>...` (lead a party of these characters) and `join <leader> <member>...` (accept that
leader's invitation; the other names are the party's other bot members, none for a party of two)
are standing like `stay` and `go` (plan section 8: kept by the `Bot` next to its mailbox, applied
at each session start, shown in the snapshot), in a slot of their own beside the standing order;
`unparty` (clear the slot) is one-off like `leave`.

`Command.valid` (`mailbox.go:66-75`): `unparty` takes no `Arg`; `join` and `lead` take 1 to 8
names (leader first for `join`), single spaces, each `[A-Za-z0-9]{1,16}`, none twice; anything else
answers `ErrBadCommand`. The kinds come after `CommandLogout`, so the raw command route
(`POST /api/bots/{account}/command`, `internal/botpanel/api.go:39`; `parseCommand` tries only
`CommandStay` to `CommandLogout`, `api.go:290-296`) keeps refusing them; hand forming goes through
`FormParty` ([panel](panel.md)).

A `lead` or `join` replaces the panel standing order, as a new panel order does
(`mailbox.go:211,229`): the brain clears `link.order`, drops the session's `stay`, `roam`, `follow`
and `assist` (keeping `reply` and `withdraw`), clears `orderPending`, then sets the slot; else
`applyStanding` (`mailbox.go:254-266`) and `followed` (`command.go:277-286`) would keep following.
Only a hand-formed or pinned party meets a standing order, the latest panel order winning.

While `Whispered` holds ([below](#status-fields)), `lead` and `join`
answer a new `ErrWhispered` and set no slot (a whispered `follow` takes precedence, `death.go:293`).

The slot is owned by the brain goroutine and outlives the sessions of one run (`mailbox.go`
`botLink`): a reconnecting member or leader is invited, or invites, again with no organizer pass. A
new run starts with no slot.

## Status fields

On `botclient.Status`, shipped in P1a (read by P5b, shown by P7):

- `Party`: the slot (`lead ...` or `join <leader> ...`, empty for none), beside `Order`.
- `Whispered`: true while a GM's whispered `follow`, `assist`, `stay` or `go` holds this session.
- `PartyLeft`: why the brain last cleared its own slot this run: `left` (P1b's clears), `timeout`
  at `JoinWait` (P1c's [join hold](members.md#join-hold)), empty while a slot is set or after the
  organizer's `unparty`.

`Heals` ships in P5b ([composition](composition.md#healer)).

## unparty

Clears the slot, and withdraws (`RequestWithdrawParty`) only from the bot party the slot named: the
party it leads under `lead`, or the named leader's under `join`. In any other party, or none, it
only clears: it never takes a bot out of a player's party. Exception: a `join` bot made leader whose
[handover](leader.md#handover) is running withdraws.

## Slot clearing

These clear the slot (P1b, `PartyLeft` `left`), and when the bot sits in its bot party take it out
first:

- a panel standing order (`stay`, `go`, `follow`, `assist`) or a GM's whispered `follow`,
  `assist`, `stay` or `go <spot>` (a `go` claims, `command.go:236-241` `goToSpot`), in the party or
  between invitations: `obey` (`command.go:180-193`) and `panelOrder` (`mailbox.go:191`) gain the
  step before the order;
- the panel's `leave` (`CommandLeave`, `mailbox.go:213-220`), clearing before the `ErrNoParty`
  check: a partyless bot holding a slot answers nil, one with no slot `ErrNoParty`;
- a whispered command that takes the bot out (a GM's `leave`, a GM's or player's `invite me`,
  [players](players.md)): `obey` (`command.go:194-203`) gains the step.

Every clear from no party resets the leader watch ([members](members.md#clearing-from-no-party)).

**Player-led-party rule.** The slot also clears when the bot is in a party led by anyone its slot
does not name: for `join`, anyone but itself, the slot's leader and its listed members; for `lead`,
anyone but itself. Such a party is a player's or a GM's (only `lead` bots invite, only listed bots
and `join` players), told by exact names, never the bot marker. Exception: a `join` member leading
its own party (the server made it leader, `party/party.go:337-346,418-427`) keeps its slot for the
[handover](leader.md#handover); else it falls back to today's lost-leader wait
(`death.go:199-204`). The `party` rule invites only from no party or its own, so a `lead` slot
never invites from inside another (154, `party/party.go:215-216`, `network/party.go:82-84`).

## Withdrawal and reply in order

One reply slot, `orders.reply`, sent by `ruleCommand`, is replaced by a later reply
(`command.go:122-125,252-258`); a clearing command that answers needs the withdrawal first (a GM's
`invite me`, a failing `go <spot>`), so `orders` gains a `withdraw` flag:

- set only while the bot is in a party, by `unparty` (P1a, which ships the flag), the clearing
  step (P1b) and a [handover to a player](players.md#handover-to-a-player) (P6); the panel `leave`
  sets it instead of the reply;
- `ruleCommand` sends `RequestWithdrawParty` (`actLeaveParty`) while it is set and the bot is in a
  party, then the reply; the send clears it and resets the leader watch (`brain.go:815-817`). No
  chat gate for it; the whisper follows within the gate;
- dropped unsent (a reply slot clears only on its own send, `brain.go:840-842`), resetting the
  watch, at the first decision finding the bot in no party (a disband, `party/party.go:381-386`) or
  when its own `RequestAnswerJoinParty` is sent (`brain.go:818-820`; `rulePrompt`,
  `rules.go:585-587`, runs before `ruleCommand`, `rules.go:476-477`);
- kept on a leader change (`party/party.go:418-427`), never replaced by a reply, and kept like
  `reply` by the three resets of `orders`: the panel `go` (`mailbox.go:200`),
  `holdStill` for the panel and whispered `stay` (`mailbox.go:234-237`, from `mailbox.go:195` and
  `command.go:187-188`) and `orderPlayer` for the panel `follow` and `assist`
  (`mailbox.go:241-249`).

## Tests (brain unit: events and an injected clock in, packets out)

- `TestPartyCommandValid` (P1a): `lead A B` and `join L A B`, 1 to 8 names, are valid; a ninth, a
  repeat, a double space or a 17-character name answers `ErrBadCommand`; `unparty` takes none; the
  raw route refuses all three kinds.
- `TestPartyJoinReplacesStanding` (P1a): a `join` to a bot with a standing `follow X` clears it
  (`Order` empty), the bot accepts its leader and follows it, not X, and the next session applies no
  standing order; a `lead` to a bot with a standing `stay` likewise.
- `TestPartyUnparty` (P1a): to a member of its bot party it clears and withdraws; in a player's
  party or none it sends nothing.
- `TestPartyJoinPlayerClears` (P1b): a `join` and a `lead` bot accepting a player's invitation (also
  `Talbot`) clear their slot on the next window (`Party` empty, `PartyLeft` `left`).
- `TestPartyPanelLeaveClears` (P1b): a member clears `join`, withdraws, declines the old leader; the
  leader clears `lead`, withdraws, invites nobody; a partyless bot with a slot clears, sends
  nothing, answers nil; one with no slot answers `ErrNoParty`.
- `TestPartyWhisperedOrderClears` (P1b): a GM's `stay` or `go <spot>` to a member clears the slot
  inside `obey`, and its first packet after is the withdrawal, ahead of any move (a `go` claims
  inside `obey`, `command.go:189-192,238`, so the test asserts the registry after `obey` and the
  packet order); a panel `stay`, `go`, `follow` or `assist` in the party clears and withdraws (the
  flag survives the resets); a panel `follow` to a partyless `join` member clears, sends nothing,
  and the next invitation is declined; under a GM's `follow`, `join` and `lead` answer
  `ErrWhispered`, set no slot, and `Whispered` reads true.
- `TestPartyWithdrawLapses` (P1b): after a GM's `leave`, a disband before the next decision sends no
  withdrawal, lost-leader whisper or `/unstuck`, nor after a GM's invitation and window; disband and
  `AskJoinParty` in one decision: it answers, no withdrawal after; a leader change between: sent.
- `TestPartyWhisperNoReform` (P1b): a GM's `leave` to a member clears `join` (`left`), withdraws
  through the step, and the old leader's next invitation (handed in) is declined; a GM's `invite me`
  likewise before its whisper; a GM's `leave` to a `lead` bot in its party clears and withdraws.
- `TestPartyInviteMeGM` (P1b): a GM's `invite me` to a member clears, withdraws through the step,
  then whispers; a whispered `go` to an unknown spot withdraws, then whispers the error.
