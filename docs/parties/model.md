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

As shipped (#384, `party_danger.go`): a bot member is a healer when its class's Support build
heals with a skill its class tree grants by its level (the [healer](composition.md#healer) rows: a
human, elven or dark mystic from 7, a Cleric or Oracle, an Orc Shaman from 20), else a Damage
member through its Damage build's first damage step or its melee, whichever kills faster; a step
gated on its own HP (`SelfHPBelow`, a drain cast only when hurt) is passed over, as the solo fit and
the gear model do (`killAttack`, #712). A member's
profile wears nothing the model knows of (its gear is not in the window), so its damage and its
heal's √M.Atk are low bounds. Each healer counts the heal of its kit that restores most over the
leader's margin kills, at one cast a cast period, for as many casts as its window MP pays, shared
over the likely pack's fights as the leader's potions are. Present: alive, and in sight or last
reported (`PartyMemberPosition`) within `LeadPolicy.FarAway`; a member not reported yet waits for
its first report.

## Tests

- `TestPartyFitHealer` (unit): the same leader and pack fail alone and fit with a fixed healer
  profile; a healer with no MP left adds nothing.
