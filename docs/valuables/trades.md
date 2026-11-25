# Trades and invites (D32)

## Trades

The trade rule (`trade.go`) learns valuables:

- `acceptTrade` counts a valuable as goods during a round; `Every` does not hold back a partner with
  an agreed price;
- `valuableGoods` offers every valuable the partner agreed a price for, in one window (#403), else
  the one it named, else the top-scored one, never a released one, in score order and within the
  [budget's](release.md#valuables-budget-d34) offer rule: each while the budget has some left, its
  price then taken off what is left, so a budget left that covers only the first offers it alone.
  `tradeStep` sends an `AddTradeItem` a turn, each once the server's `TradeOwnAdd` took the one
  before; a partner confirm before the last add locks the offers (the server drops later adds), so
  the bot cancels. Non-valuable goods (`tradeGoods`) stay one item per trade;
- `tradePrice` of a valuable is the agreed price, else the partner's current price, else the ask;
  `tradePaid` confirms only once the partner's adena covers the sum over the offered rows, so the
  window is a haggle too. As today (`tradeStep`) a partner who stays short gets the cancel at
  `Timeout`, one who confirms short gets it at once (the confirm locks the offers below the price).
  The settled trade counts the sum in the budget and one `valuables_sold_total{via="trade"}` per
  entry sold.

## Asking for a trade

A partner who whispered this round, stands in sight (the bot's known list, which the server requires)
within `InviteReach` (1200) and within `RoundReach` less 100 of the round's anchor (not the town
point, a gatekeeper spawn, `storeregistry.go:114-131`): the bot walks to within 100 of it (inside the
150 confirm range), sends `TradeRequest` once, and after the trade, a refusal or expiry
[walks back](round.md#back-to-the-anchor) before its next line or invite. A partner outside the area
gets only the whisper (`trade me`); its own request is still accepted.

- It waits for `TradeStart` up to 15 s (the expiry); on any refusal (119, 153, 144, 340 or 1065, 940
  or 619, 794, 1987; [server](server.md#server), "Trade request") or silence it asks that partner no
  more this round.
- At most one request open; none while the store is open or opening, during the round's private
  warehouse request, or within 15 s (`RequestTimeout`) of the round's end as it stands (`RoundWait`
  after the first line, or the extended `Grace` end; [round-end](round-end.md#steps), steps 5, 6).

## Tests

- `TestValuablesTradeInvite` (V6; a round opened by `openRound` at a given anchor, legs and request
  out of `decide`): a whisperer in sight within `InviteReach` gets legs then one `TradeRequest`; after
  119, 340, 1065 or 940 none more that round; none to one out of sight, or within `InviteReach` but
  farther than `RoundReach` less 100 from the anchor; none while one is open; none within 15 s of
  `RoundWait` after the first line; after the trade, a refusal or 15 s of silence, legs back within
  `anchorArrive` come before the next Trade line or `TradeRequest`.
- `TestValuablesTradeConfirm` (V6; a round by `openRound`, the partner having named a valuable or
  agreed a price; and with no round, where it is not offered): `TradeStart` brings `AddTradeItem` of
  it; `TradeOtherAdd` adena under the agreed price no `TradeDone` and the cancel at `Timeout`; a
  confirm while short the cancel at once; enough adena within range `TradeDone(1)`; success counts in
  the valuables budget, not the store's; a valuable priced above the budget left is offered while
  sales are under `Budget`, none once they reach it. After the success and the `InventoryUpdate`
  taking the sword away, its entry is gone and the next ad line (V4's step) names only those still
  carried. The cap is V7c's ([`TestValuablesRelease`](release.md#tests)).
- `TestValuablesTradeMulti` (#403; a partner who agreed prices for the sword and the staff): an
  `AddTradeItem` each, the second only after the first's `TradeOwnAdd`; adena short of the sum holds
  and gets the cancel at `Timeout`, a confirm while short or before the second add the cancel at
  once; the sum paid confirms, counts the sum in the budget and both entries leave the ledger; a
  budget left equal to the first's price offers the first alone, its price due.
