# Advertising on the Trade channel (D30)

## Lines

- **Channel:** Trade (Say2 channel 8), never Shout: players read Trade for sales, Shout reads as noise
  (its gate is shared with the general channel, which no bot line uses, so no reason either way). No
  channel a bot can use reaches the whole server, despite the owner's "global message": Trade and
  Shout reach only the speaker's restart region (`network/chat.go:108-122`), Hero Voice drops a
  non-hero's line (`network/chat.go:123-128`); an ad in Giran is not heard in Aden. D30.
- **Wording:** `WTS <name> <price>[, <name> <price>]..., pm me`. The datapack reader keeps every name
  (`datapack/items.go:238` weapons and armor, `:354` etc items), but the travel map's `itemNames`
  holds only recipe products (`travel.go:88-91,138`, `indexProductNames`, `craftorder.go:260-277`,
  read by the craft order's label, `craftorder.go:212`); V2b extends it to every scored template.
  **Labels:** `2x <name> <price>` for several units (price each), `+5 <name>` enchanted,
  `<name> (C)` graded; prices as players write them, `173k`, `1.2kk`, rounded up to the shown unit,
  never below the ask. At most 100 UTF-16 units a line; labels that do not fit go to the round's next
  line, in score order. Example: `WTS Sword of Watershadow 173k, Tears of Eva 174k, pm me`.
- **Entries:** the entries with carried units not released at the time of the line, so an item sold
  earlier is not named again.

## Region

The bot computes its restart region from its position with the server's map-tile formula (copied,
`model/restart/restart.go` `mapRegion`, with the bot's `geo` bounds) over the restart points' `map`
lists (V1). In no region (-1) it does not advertise: the line would reach every player outside one.

## Ad board

`AdBoard`, one per process like `StoreRegistry` (read-only regions, a mutex owns the counters): a line
goes out only when no bot of the process said one in that region within `RegionGap` (60 s) and the
process said fewer than `ProcessPerHour` (30) in the last hour.

- `Free(region, now)`, asked by `roundReady` and the pending round, is only a hint: brains decide on
  their own goroutines. The ad step claims at the decision: `Claim(region, now)` checks both limits
  and records the line in one step under the mutex, as `StoreRegistry.claim`
  (`storeregistry.go:14-25,90-101`; `store.go:345-350`); a failed claim says nothing that decision.
- It claims only when the chat gate lets a line out now (`b.pace.allow(gateChat, now)`, as rule
  "announce", `craftorder.go:225`); a line the frame bucket holds (`send`, `brain.go:774-779`) stays
  the round's pending line and goes out later with no second claim. A line counts at its claim, even
  if never sent (session ended): the board may over-count, never under-count. No line queues past the
  round; `FirstLineWait` bounds the first.
- **Per bot:** `AdEvery`, `AdLines` and `RoundGap` give at most 6 lines an hour, each through the chat
  gate (`chatGap`, 3 s).

## Tests

- `TestRestartRegion` (V2a): the copied formula against the server's at fixed points (Talking Island
  town, a point in no region).
- `TestItemLabel` (V2b): names (every scored template in `itemNames`, not only recipe products),
  enchant, grade, two units as `2x <name>`, `k` and `kk` rounded up to the shown unit, never below the
  ask.
- `TestAdText` (V4): a line of several labels; the 100-unit clip, the rest on the next line by score.
- `TestAdPacing` (V4; rounds by `openRound`, lines out of `decide` through rule "valuables"): two
  brains on one `AdBoard` in one region say one line per 60 s between them, also deciding at the same
  instant (one claims, the other says nothing); a claimed line held by the frame bucket goes out later
  with no second claim; `AdEvery` and `AdLines` per round; the hour cap; the chat gate.
