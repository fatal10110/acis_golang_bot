# Composition and level band

Who the [organizer](organizer.md) groups. Ships in P5.

| Slot | Who | Why |
| --- | --- | --- |
| Leader | A Damage-role bot (preset `farmer`), the highest-level Damage candidate of its group by [Grouping](#grouping); a Support member may stand above it | It farms alone today, picks spots and pulls; its kit is the damage. |
| Healer (required) | A Support bot with the [healer flag](#healer) | Keeps the leader up, so the party rests less and takes stronger packs. |
| Third (optional) | Any other Support bot (a second healer, an orc mystic with its chants, a Support fighter), else a second Damage bot with no healer to lead with | Buffs, a second target for packs, more damage. |
| Fourth (optional, off by default) | Any of the above | Size up to `MaxSize`. |

## Healer

From the S7 kit rows ([strategy](../strategy.md)): a human, elven or dark mystic from 7 (1011 Heal
@7), a Cleric, Elven Oracle or Shillien Oracle (heals @20), an Orc Shaman from 20 (1229 Chant of
Life, a party heal with a member under 50 %); not a level 1-6 mystic nor an orc mystic (1090 Life
Drain heals only itself). No tank flag: M10's margin (tank 1.5, normal 2.0, glass cannon 3.0) is per
row.

The heals are bought ([server](server.md#server-behavior), "Heals learned"), so the flag also needs
`Status.Heals`: true while the run's skill list (`World.Skills`, from `SkillList`,
`world.go:29-32`) holds its row's heal: 1011, 1015 or 1027 for a mystic, Cleric, Elven Oracle or
Shillien Oracle, 1229 for an Orc Shaman. `Status` has no skills today (`status.go:55-98`); the field
ships in P5, and a bot that trains later counts from the next pass (inside a bot party, through
[errands](members.md#errands-in-a-bot-party)).

## Size, band, location

- **Size:** 3 by default, at most `MaxSize` 4 (persona defaults, D6), easy on spot caps (a third of
  a maker's `maximumNpcs`, `spotregistry.go` `spotCap`) and visibility (plan, "Where the cost is").
- **Level band:** forms only with every bot member within `Band` (3) levels of every other, breaks
  once the spread exceeds `MaxSpread` (5): exp falls once the party level is more than 5 above the
  monster, and under today's `SpotPolicy` (`Below` 2, `Above` 3, `spots.go:110-117`, `fits` at
  `spots.go:120-125`) a spot fits `Lmin` and `Lmax` only while its top monster lies in `[Lmax-3,
  Lmin+2]`, empty once `Lmax - Lmin > 5`. The 2 levels between are hysteresis; the server's
  20-level cutoff never bites. Players are not counted.
- **Location:** members share the leader's town (`Status.Town`) within `MaxTravel` (20000, straight
  line), the farmer's own reach for a spot.

## Grouping

Per town, in two steps; a member is taken only when the group's bot members, it included, stay
within `Band` of one another (a leader at 20 with a healer at 17 takes a third from 17 to 20).

1. *Leaders and healers:* Damage candidates from the highest level down each take the nearest free
   healer; with none, no party, and the Damage candidate is *healerless*.
2. *Third members:* groups in the same order each take the nearest free Support candidate, else the
   nearest healerless Damage candidate. A Damage candidate with a healer always leads its own group.

At most `FormPerPass` (2) new parties a pass, the first groups in that order; a Damage candidate
whose group waits is neither healerless nor a third meanwhile. A fill takes one member a pass in
step 2's order.

## Tests (planner unit, table-driven)

- `TestPlanLeaderIsDamage`: two farmers and two supports of one town form one party, the higher
  farmer leading.
- `TestPlanNeedsHealer`: a farmer with an orc-mystic support forms nothing, nor with a level-6
  human mystic; a level-7 one with `Heals` forms a party, as does a level-20 Orc Shaman with
  `Heals`; either with `Heals` unset forms nothing, and a party whose only healer reads it unset
  starts `NoHealerSince`.
- `TestPlanBand`: a support 4 levels under the farmer is out, 3 under joins; farmer 20 and healer
  17: a support at 23 is not the third, one at 20 is.
- `TestPlanThirdMember`: farmer F1 20, healer H1 19, orc-mystic support S 18, farmer F2 19 nearer F1
  than S: F1 takes H1 and S; without S, F2 is F1's third; with a second healer H2 free, F2 leads
  with H2 and F1 gets no third; with two free supports F1 takes the nearer; with H2 free, no S and
  `FormPerPass` 1, F2's group waits and F2 is not F1's third.
- `TestPlanSameTown`: bots of two towns, or beyond `MaxTravel`, never share a party.
- `TestPlanFormPerPass`: six ready groups form two parties a pass.
- `TestPlanFillsSlot`: a short party gains one fitting free support per pass.
