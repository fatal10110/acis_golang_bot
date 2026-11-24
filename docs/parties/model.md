# Party danger through the M10 model

Ships in P4. M10's S11 (#330) fits a spot for one bot when `TimeToDie` against its likely pack
stays above `TimeToKill` times the row's margin. A party fits by the same test with party terms:

- `TimeToKill`: one pack monster under the summed damage rate of the leader and every Damage
  member, each from its `charmodel` profile;
- `TimeToDie`: the pack's damage rate on the leader (the whole pack on whoever pulls) against its
  HP plus the healers' rate, each heal `power + √M.Atk` (`skill/formulas/heal.go`) over its hit time
  and reuse (S2's skill fields), for as long as the healer's MP pays for it; 1229 Chant of Life, a
  heal over time, counts by its ticks' total over its reuse
  ([server](server.md#server-behavior));
- the margin: the leader's row margin.

The leader computes it: its own profile is calibrated (M10 section 3); its bot members' (the rows
its `lead` list names) come uncalibrated from `PartyMember.ClassID` and `Level` through the class
templates (S2). A player adds neither damage nor heals. Only members present count: one away on a
trip or dead drops out until it is back. This changes only the model's terms, not the spot claim's
weight ([spots](spots.md#claim-weight)).

## Tests

- `TestPartyFitHealer` (unit): the same leader and pack fail alone and fit with a fixed healer
  profile; a healer with no MP left adds nothing.
