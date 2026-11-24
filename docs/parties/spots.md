# Spots for a party

Ships in P2. Code as after #142 (#353): `claimSpot` (`spots.go:495-541`) tries `claim` within
`MaxTravel`, then beyond (`:509-512`); with neither it sets `retryAt` (`:514`), plans a trip
(`:515-517`), gives its spot back for an active trip (`:518-523`), keeps a held spot it is not
outlevelled for (`:524-526`, `if b.trip.v.active() || (v.held && !b.outleveled())`), tries
`claimOver` when `Overflow` is on (`:527-529`), and a bot holding no claim calls `farmBeside`
(`:530-534`). The registry counts `beside` apart from `held`, toward neither cap nor limit
(`spotregistry.go:23-27,152-167`); `Beside` and `Overflow` are on by default (`spots.go:113`).
`ruleSpot` re-picks when not held, outlevelled, over pack risk or over cap (`spots.go:254`),
`spotRetry` (1 minute, `spots.go:197`) after a fruitless search.

## Claim weight

The leader claims room for the whole party: `claimN(order, n)` takes `n` of a cap (empty spots
first, as `claim`, `spotregistry.go:103-119`); `claimOverN` passes the cap only where held plus `n`
stays within the limit (`claimOver`, `spotregistry.go:126-144`); `farmBesideN(order, n)` picks as
`farmBeside` (fewest beside per monster) and adds `n`. `n` is the leader's party window size,
players included, changed only by a window add or delete, never a death, raise or trip. Members
never claim and never farm beside.

The weight is `n` only while the bot leads its bot party (`lead` slot set, window naming it
leader), else 1: a leader re-weighs to 1 at once, keeping its spot, when its party ends
(`PartySmallWindowDeleteAll` zeroes `World.Party`, `party.go:259-260`: a member of two logged out,
its withdrawal, a panel or GM order, a break) or its slot clears; else the spot stays full to
others (`overCap`, `spotregistry.go:179-183`), its own crowd check subtracting `n` (`crowded`, `spots.go:416-418`), even after
a `go` to it (`command.go:235-236`). It re-weighs on every window event and slot change.

A claimless leader with no room for `n` below any limit farms beside with its whole weight, so
others do not count its members as strangers; it re-picks every decision past `retryAt`
(`!v.held`), and `farmBeside` keeps its spot while a candidate (`spots.go:550-552`). The beside spot
is the leader's spot: members follow, the gather gate applies, `Spot` names it (`status.go:139-141`,
`farming()`, `spots.go:229`).

## Weighted release

Today `release(i)` (`spotregistry.go:193-199`) and `leaveBeside(i)` (`spotregistry.go:169-176`)
take 1, called once by `leaveSpot` (`spots.go:601-610`). The brain records `spot.weight` (1 solo,
`n` for a leader); `releaseN(i, n)` and `leaveBesideN(i, n)` take `n` (never below 0), and
`leaveSpot` gives back `spot.weight` from the count the bot sits in. Callers in `spots.go`: the trip
give-back (`:522`), the old spot on a new claim (`:539`), `farmBeside` moving (`:553`), `moveOn`
(`:584`, from an unreachable walk `:274`, a crowded spot `:283`, a death with `DeathAvoid` `:593`)
and `diedAtSpot` (`:596`); outside: `supplytrip.go:121`, `command.go:241`, `brain.go:359`. P1c adds
the release on [joining](members.md#joining), P2 the [gated give-back](#no-gatekeeper-trip).

A new claim sets `spot.weight`: `n` for `claimN`, `claimOverN`, `farmBesideN`; 1 for the `go`
claim (`command.go:238-242`), whose order clears the slot first. A size change (formation, end of
leading) is one `reweigh(i, beside, old, new)` under the registry's lock; down, or beside, needs no
check. A held re-weigh up may pass the cap, and `ruleSpot` re-picks (`overCap`): room for `n`
(within `MaxTravel` first, `spots.go:497-512`) moves the party; with none the leader keeps its spot,
re-picking every `spotRetry`, never travelling. `claimOverN` serves a claimless leader and an
`outleveled` or `misfit` holder (`spots.go:524-529`), and `farmBesideN` only a claimless one
`claimOverN` found no room for.

## Crowd check

`crowded` (`spots.go:402-418`) skips party members and subtracts `Held + Beside - 1` (`:418`); it
subtracts `Held + Beside - spot.weight` instead (a solo check is today's), so a party of 3 does not
need `Crowd + 2` strangers. A solo bot where a party farms beside counts its members as players and
subtracts the party's whole beside weight.

## Fit by level

The leader tells bot members from players by its `lead` list; spot fit and the [model](model.md)
use only bot members. A spot fits when `Below` and `AggroAbove` hold against the lowest bot
member's level and `Above` and `Outlevel` against the highest (`PartyMember.Level`). When no spot of
the map fits that band, the leader fits by its own level. The test is map-wide, by level only
(`SpotPolicy.fits`, `spots.go:120-125`), not reach, avoidance or pack risk as `spotCands`
(`spots.go:466-491`): band-fitting spots only beyond reach leave the candidates empty even if one
in reach fits the leader's level, so the party idles until `NoSpotFor`. No pin flag is needed: a
pin past `MaxSpread`, or an auto party up to one `PartyTick` after a level-up past it, sees no fit.

## Misfit

`outleveled` checks only the bot's own level against `MaxLevel + Outlevel` (`spots.go:423-426`), so
a leader's formation spot (fit alone: `Below` 2, `Above` 3, `AggroAbove` 0, `spots.go:110-125`)
could keep members 3 lower at a top monster up to `Lmin + 5`, aggressive up to `Lmin + 3`, against
the band's `Lmin + 2` and `Lmin`; level-ups and member changes too. P2 adds `misfit`: leading bot
members, the held spot fails the band fit while some spot of the map fits it. It counts as
`outleveled`: `ruleSpot` re-picks (`!v.held || b.outleveled() || b.misfit() || ...`) and
`claimSpot` keeps only while `!b.outleveled() && !b.misfit()`, so the leader claims a band-fitting
spot, past its cap if needed (`claimOverN`), keeps the misfit spot only while every band-fitting
candidate is too near its limit for `n` (as a solo bot keeps an outlevelled spot with no trip,
`spots.go:513-535`; re-picking every `spotRetry`), and with none in reach
gives it back (below). Like `outleveled` (`spots.go:425`) it reads only a held claim; a beside spot
leaving the fit is no candidate and `farmBeside` leaves it (`spots.go:550-553`).

## No gatekeeper trip

While it leads members the leader takes no gatekeeper trip (`planTrip`, `travel.go:416-431`):
members follow by walking only (`follow.go`, the walker within the SCC). Its supply trip (D16) runs
([members](members.md#events)). Denied the trip, it goes on down `claimSpot`.

An `outleveled` or `misfit` holder with no candidate in reach would keep its spot for good (the
keep is skipped, `spots.go:524`, `claimOver` gets an empty order, `:527-529`, a holder skips
`farmBeside`, `:530-532`, `spotCands` never lists the held spot). So P2 adds the **gated trip's
give-back**: where a solo bot calls `planTrip` (`:515-517`, `here != nil`), such a leader with
`len(cands) == 0` gives the spot back (`leaveSpot`, the whole weight), farms beside nothing, and
`NoSpotSince` starts; the spot fails the fit, so it is not reclaimed. It so idles only when no spot
in reach fits its band (or own level), or with `Beside` off when every fitting one is at its limit,
until `NoSpotFor` ([organizer](organizer.md#breaks-and-drops)) frees it to plan its trip alone.

## Gather gate

At its spot the leader picks no new target while a living bot member stands farther than
`GatherRange` (1200, inside the 1500 party range) from it, for at most `GatherWait` (60 s); a
member beyond `FarAway` (10000) does not hold it.

## Tests (brain unit, P2)

- `TestPartyClaimN`: a party of 3 claims 3, skips room for 2; a member leaving: held -1 in one step;
  a member's death, raise or trip: unchanged; a player joining at a cap-3 spot: 4, then a move to
  room for 4 in reach, or with none kept over cap, no trip, re-pick 1 minute later; the leader's
  trip, death and stop restore held; all fitting spots at cap: a claimless leader of 3 takes one
  whose held plus 3 stays within its limit, not one with room for 2; all too near: beside +3 (held
  unchanged, `Spot` set), a member leaving: beside 2 in one step, room for 2 freeing: claim 2 and
  beside -2, stop: beside restored; a solo `go` after a leader's slot cleared takes and returns 1;
  a leader of 2 whose member logs out: held 2 to 1 in one step, spot kept, 2 again on re-invite; a
  leader of 3 withdrawing on `unparty`: weight 1 (held -2 in one step), a whispered `go` to it keeps
  held 1 and returns 1; a leader of 3 beside whose party disperses: beside -2; a slot clearing while
  leading: weight 1 at once.
- `TestPartyCrowdOwnWeight`: a leader of 3 alone (`Held` 3) is crowded by 4 strangers (`Crowd` 4);
  with another bot's claim (`Held` 4) by 5; a leader of 3 beside (`Beside` 3) a spot one bot holds
  by 5; a solo bot (`Held` 1) where a party of 3 farms beside is not crowded by the party's 3
  players and 3 others, and is by them and 4 others; a solo check is today's.
- `TestPartySpotFitByLevels`: bot members at 10 and 15 fit only top-monster-12 spots; at 10 and 16
  none; a player member at 30 changes nothing.
- `TestPartyHeldSpotMisfit`: leader 15 on a top-17 solo spot, member 12 (band `[12, 14]`): a free
  top-13 spot in reach (no aggressive above 12) is claimed, also at cap with room for 2 within its
  limit; with no spot of the map fitting the band it keeps its own; with band-fitting spots only
  beyond reach and an own-level one in reach it claims neither, gives its spot back (held -2),
  `Spot` empty. Leader 15 on a top-15 spot (no aggressive above 12), member 12, the top-13 spot too
  near its limit for 2: it keeps the misfit spot, re-picking every `spotRetry`; the member reaching
  13 (band `[12, 15]`) clears `misfit`, and it stays when the top-13 spot frees. A member 12 joining
  14 and 15 at a top-16 spot re-picks the next decision; a beside spot leaving the fit is left at
  the next re-pick; a solo held spot is kept as today.
- `TestPartyNoGatekeeperTrip`: a leader with members plans no trip, alone it does; leader of 3 at 21
  on a top-15 spot (past `Outlevel` 5), nothing in reach fitting band or level: no trip, spot given
  back at the next re-pick (held -3), beside nothing, `Spot` empty, not reclaimed; one candidate in
  reach too near its limit for 3: spot kept, re-pick every `spotRetry`; a solo bot as today (trip
  and give-back, or no trip and keep).
- `TestPartyGatherGate`: a member at 2000 holds the leader's next target until within 1200 or 60 s;
  one at 12000 does not.
