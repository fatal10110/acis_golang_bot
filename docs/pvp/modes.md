# Modes, the `pvp` preset, panel and metrics

Modes and preset ship in X4b, panel and metrics in X12. Decision D35.

## Modes

`PvPPolicy` (today `FightFlagged`, `ForceAttack`, `SupportFlagged`) gains these modes, persona
defaults (D6) shown and switched per bot on the panel. `ForceAttack` becomes the open-world PK mode
([rule 7](targets.md#mayfight), X11): from X4 it no longer lets any player pass `MayFight` ahead of
rules 1-8; from X11 it attacks under the limits of [PK mode](karma.md#pk-mode).

| Mode | What the bot does | `pvp` preset | Other presets |
| --- | --- | --- | --- |
| Fight back | Fights a flagged or karma player that hits it (today), with the PvP layer | on | on (today), with the PvP layer |
| Duels | Takes challenges it can take; challenges players ([finding](finding.md)) | on | off: declines at once (today a challenge expires unanswered after 15 s) |
| Flagged and karma | Attacks flagged and karma players in sight of its tier ([hunting](targets.md#hunting)) | on | off |
| Arenas | Visits an arena on a timer and fights there | on | off |
| Open-world PK | Force-attacks clean players under the D39 limits ([PK mode](karma.md#pk-mode)) | off (opt-in per bot) | off |
| War (`FightWar`, X14) | Fights and, with hunting on, attacks mutual clan-war enemies by force ([war](war.md), D40) | on | off |

`pvp` is M10's Damage role (build, branch weights, gear) farming like `farmer` between fights. Its
`SupportFlagged` is on (it keeps supporting a flagged party member, which flags it) except in its
own duel ([duel support](guards.md#duel-support)). `support`, `farmer`, `merchant`, `crafter`,
`town` keep fight back only and `SupportFlagged` off. PvP value and the duel, hunting and arena
modes stay with `pvp`.

## Who plays which row

Modes decide when; the PvP layer decides how, for every preset: [targets](targets.md),
[guards](guards.md), the [PvP order](layer.md), its [row](rows.md), [party PvP](party.md) in any
party. M10's Damage role never picks KN, CL, EO or SO (`docs/strategy.md` section 6, "Professions
by race and role": Support column only) and M11 bot parties take only `farmer` and `support`
([eligible bots](../parties/organizer.md#eligible-bots)). So `pvp` bots play HF, EF, DF, OF, WF,
HM, EM, DM, OM, WA, RO, EK, ES, PK, AS, OR, MO, SC, HW, EW, DW, OS; `support` bots play KN, CL, EO,
SO when they fight back, alone, in a player's party or in an M11 bot party. A bot moved to another
preset after its first profession keeps its row (a role change affects only the choices still
ahead, `docs/strategy.md:629`), so a `pvp` KN, CL, EO or SO can exist.

## Panel and metrics (X12)

The panel shows a `pvp` bot's mode switches (PK off), its PvP and PK kill counts (`UserInfo`; the
server's PvP count, which under the shipped `AwardPKKillPVPPoint = False` skips karma-player kills)
and its duel results. Counters:

- `bot_pvp_fights_total{kind}`: per opponent at the first attack, by its record's kind: `duel`,
  `fight_back` (X4), `arena` (X4b), `flagged` (X5), `pk` (X11), `war` (X14);
- `bot_pvp_deaths_total{kind}`: per death to a player or its summon, by the killer's kind
  (`fight_back`, `arena`, `flagged`, `pk`, `war`, or `unfought`, set by X4 on an attacker `MayFight`
  refused, such as a clean attacker fled with karma); a duel defeat leaves 1 HP and counts in
  `bot_duels_total`;
- `bot_duels_total{result}` from X4's duel result, read before `ExDuelEnd`: 1950 with the bot's
  name → won, the opponent's → lost (a 1955 withdrawal precedes the deciding 1950), 1952 → tie; a
  cancel and a timeout show the same tie (`network/duel.go:513-539`, `duelEndMessages`;
  `serverpackets/systemmessage_duel.go:22-25`);
- `bot_pvp_interrupts_total{result}` from X6's interrupts: `cast` per interrupt sent, then `landed`
  on the opponent's `MagicSkillCanceled` before its landing time, else `too late` at that time.

The switches are stored per bot in the registry: `POST /api/bots/{account}/pvp` with
`{"modes": {"duels": …, "hunt": …, "arenas": …, "pk": …, "war": …}}` replaces the persona's five defaults
(fight back and `SupportFlagged` stay the preset's; switches stored before X14 have no `war` and read
it off), `{"modes": null}` gives the defaults back, and
an online bot respawns with the change through the login gate. The bot's live view carries `pvp`
(the modes played, `pvp_kills`, `pk_kills`, the run's duels by result, `last_duel`). The metrics
snapshot names the counters without the `bot_` prefix, as its other fields: `pvp_fights_total`,
`pvp_deaths_total`, `duels_total` and `pvp_interrupts_total` (`too late` as `too_late`), every
label present. A death counts by the kind of the fight the bot started on its killer, so a hunted
flagged player or a PK victim that fights back stays `flagged` or `pk`; a killer whose record holds
no fight kind counts as `unfought`. An interrupt whose opponent leaves sight before the landing
time counts `cast` only.

## Tests

Modes are tested through the rules they switch.

- `TestPvPCounters` (X12): a first attack on a duel, fight-back, arena, flagged and PK-mode
  opponent → one count each under its kind, none for a second attack; a death to each but the duel
  opponent, and to an attacker `MayFight` refused → that kind, `unfought`; 1950 with its own name,
  the opponent's, 1952 → won, lost, tie; an interrupt then the opponent's `MagicSkillCanceled` →
  `cast`, `landed`; an interrupt and the landing time passed → `cast`, `too late`.
