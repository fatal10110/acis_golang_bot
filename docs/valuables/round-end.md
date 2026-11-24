# The round: steps, end and cut-off

## Steps

1. **Withdraw** (stored entries not released, or released ones while the budget is not spent): one
   private request ([warehouse](warehouse.md)) takes every stored valuable not released and, budget
   allowing, the released stored ones (V7b), which skip ads, replies, trades and the deposit and are
   listed at step 6 with no second open. Then the [walk back](round.md#back-to-the-anchor). A failed
   withdrawal (no keeper, no answer) leaves them stored; the round advertises what it carries.
2. **Advertise:** the first [ad line](ads.md) at the anchor after the walk back, then one per
   `AdEvery` (2 min), at most `AdLines` (3), each through the ad board.
3. **Answer** [whispers](whispers.md) and trade requests; [ask](trades.md#asking-for-a-trade)
   whisperers for a trade (within `RoundReach` of the anchor, then the walk back).
4. **End** `RoundWait` (5 min) after the first line, once extended up to `Grace` (2 min) by an open
   trade or a buyer with an agreed price in sight. If the board still holds the first line
   `FirstLineWait` (2 min) after the withdrawal's walk back (or the start), the round ends with no
   ad: it deposits, steps no rounds or asks and runs no cap (nobody heard). Bound: withdrawal +
   `FirstLineWait` + `RoundWait` + `Grace` + deposit, about 9 minutes plus two keeper visits and
   walks back (each within 6000).
5. **Count, cap, deposit**, in order: each unsold entry not released gets rounds + 1 and its ask
   lowered by `RoundStep` ([prices](whispers.md#prices)); the [cap](release.md#the-cap) runs; the
   deposit takes the carried valuables the cap did not release. A
   released carried one stays for the store, not paying 30 a row (`warehouse.go:556`, server
   `network/warehouse.go:21`) for a second open 60 s later (`warehouse.go:22`); all released, no
   deposit. The deposit is queued only when an open is due (`openDue`, `retryAt`); else (an
   unanswered open doubled the gap) the round ends with them carried.
   - **Pending requests.** The server's request gate ([server](server.md#server), "Warehouse request
     gate") answers the deposit with 142 during any pending request, sent or received; the bot
     checks only an open window today (`depositRows`, `warehouse.go:535`). So the deposit waits
     until the bot's own `TradeRequest` is answered (`TradeStart` or a refusal) or 15 s old
     (`RequestTimeout`, server `trade/book.go:13`) and no player's request awaits an answer. The bot
     sends no other request (`encodeRequestJoinParty`, `encodeRequestJoinPledge`: harness only,
     `harness.go:124,130`) and sees three it receives (`Prompts`, `world.go:269-284`): party and
     clan invitations, answered at once by rule "prompt" (rule 2, `rulePrompt`, `rules.go:584-603`;
     clan ones do not count on the server anyway), and trade requests (`ruleTrade`, checked by
     `storeReady`). A friend, duel, party room or command channel request it cannot decode, received
     in the 15 s before the link or request, fails the deposit like any failure, the keeper
     unmarked. Invites stop 15 s before the end, so its own wait is rare and at most 15 s.
   - **142.** A 142 to the open link or the request ends the transaction refused
     (`warehouseRefusals`, `warehouse.go:68-71`; `endTransaction`, `warehouse.go:329-341`, no
     `retryAt`); the next open waits 60 s from `openedAt`, set when the open link went out
     (`warehouse.go:302-311,373-374`). A 142 to a `Chat` link is a [refusal with no
     mark](warehouse.md#refused-links) from V3a on. The deposit is lost this round.
6. **Store:** the walk back, then the rule lets go and the store opens as today at the anchor with
   the loot and released valuables. The server refuses a set-up during a pending request
   (`ProcessingRequest`, `network/privatestore.go:88`), so `storeReady` also waits while the bot's
   own `TradeRequest` is unanswered and under 15 s old (rule "prompt" answers a party invitation
   first); a request the bot cannot see refuses it as today.

A failed deposit (no keeper, no answer, fee short) keeps them carried, out of the NPC sale and the
store, for the next round `RoundGap` later; one that keeps failing still gets advertised rounds, so
the cap releases it after `MaxRounds` at most.

## Cut-off

The rule checks `called()` every decision: a bot called away (`follow`, assist, or the party leader
it follows, `followed`, `command.go:277-286`; `called`, `store.go:400-403`) ends the round at once,
carrying its valuables, depositing nothing. After its first ad line it counts as run to its end
(rounds step, asks drop, the cap runs); before it nothing changes.

Rule "warehouse" (`rules.go:500`) precedes "follow" (`rules.go:507`) and steps any running request
to its end (`warehouse.go:402-406`), so a `follow` during the deposit would still walk up to 6000 to
the keeper. So the cut-off ends the rule's running private request through `cancelPrivate(now)`
(V3a):

- before `RequestWarehouseDeposit` or `RequestWarehouseWithdraw` went out (`whTalk`, `whOpening`,
  `whListed`: walk, talk, `Chat` links, open link sent, list awaited or received): ended at once by
  `end()` (`warehouse.go:345-348`), no result, no `retryAt`, status unchanged; a sent open keeps its
  `openedAt`, its late list or 282 dropped (`warehouseEvent` drops events with no request,
  `warehouse.go:655-659`). A cancelled look stays not done; withdrawn items stay stored, deposited
  ones carried;
- once it went out (`whSent`): not cancelled, since the rows may have moved and only the errand's
  `InventoryUpdate` step applies them (`trackContents`, `warehouse.go:687-693`); the rule holds the
  turn without moving and steps it (`stepWarehouse`) to its answer or `warehouseReplyTimeout` (5 s,
  `warehouse.go:29`), then lets go;
- no walk back; a walk under way stops being stepped.

The look and the off-round errands cancel the same way; `cancelPrivate` touches only `Private`
requests.

## Off-round errands

Outside a round, after the session's look and under the [look's
conditions](ledger.md#every-session-looks-first), two errands, each one private request holding the
turn only while it and its walk back run (anchor: the position at its start; keeper as a round's);
the ad board never holds them. The bot sends no `TradeRequest` outside a round and `roundReady`'s
state checks refuse a trade prompt or window; a request it cannot see fails one as in step 5.

- **Release withdrawal** of stored released valuables ([release](release.md)), only while the budget
  is not spent (else they stay stored and marked); waiting for its open it is a [pending release
  withdrawal](round.md#the-store-waits-for-a-pending-round).
- **Budget-spent deposit** of every carried valuable, released or not; released ones keep the mark.

## Tests

All V7a.

- `TestValuablesFirstLineWait`: the board holding the first line past `FirstLineWait`: no Trade
  line, a deposit, rounds and asks unchanged.
- `TestValuablesHoldsTurn`: in a round a due gear, market, shop, supply or clan warehouse errand
  does not start and the bot stays; the deposit runs to its end without rule "warehouse"; a `follow`
  ends the round with no deposit (rounds and asks step only after the first line). A `follow` (in a
  variant, the followed leader moving off) while the deposit walks to a keeper 5348 away (Dion): the
  request ends that decision, no `_DepositP` or `RequestWarehouseDeposit`, no walk back, the next
  legs rule "follow"'s, carried, no `retryAt`. At `whListed`: no deposit request. At `whSent`: no
  legs until the `InventoryUpdate` (units counted stored) or the 5 s timeout, then it follows.
  During the withdrawal: ended likewise, stored. The cut-off's cap is V7b's.
- `TestValuablesRoundDeposit`: at `RoundWait` after the first line the bot walks to the keeper,
  follows its pages to the private link (on `Chat 1`, and on Wilford's first page), sends
  `npc_<obj>_DepositP`, takes the type 1 list, deposits with 30 a row counted; on the
  `InventoryUpdate` they count stored, the bot walks back within `anchorArrive`, then the store
  opens with the loot only at the anchor; within `AnswerAfter` a whisper naming the stored valuable
  gets `<name> not now, wts again soon`. An open timeout in the round: it ends on time, carried, no
  deposit or wait. With the budget spent the off-round errand deposits the carried ones with no Trade
  line.
- `TestValuablesGrace`: an open trade at `RoundWait` holds the deposit up to `Grace`; no
  `TradeRequest` within 15 s of the extended end. The bot's unanswered request holds the deposit
  until `TradeStart`, a refusal or 15 s, then `_DepositP` (no 142); a player's unanswered request
  likewise; a party invitation at the end is answered by rule "prompt" first; the store opening
  waits the same way. A 142 from an unseen request (a friend invitation): at a `Chat 1` keeper the
  deposit fails `WarehouseRefused` with no mark and the next round (or off-round deposit) uses that
  keeper; at Wilford's fixture the 142 to `_DepositP` ends it refused and the next open waits 60 s
  from that open. Either way they stay carried.
- `TestValuablesReadvertise`: a new valuable with two stored: `_WithdrawP` first, then one Trade
  line naming all three; the stored ones' asks `RoundStep` lower.
