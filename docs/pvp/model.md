# The PvP model: land rates, matchups, PvP value, gear

Ships in X3 (`charmodel`: land rates, archetypes, row map, `RowTable`, `PvPValue`, engage
estimate) and X10 (mix table, gear scores, consumables). Decision D37.

## Land rates

`charmodel.LandRate(skill, caster, target)` copies the server formula ([debuff land rate](server-casts.md)). The opponent's CON and MEN come from its class template (dyes, at most ±5, ignored), its
M.Def and level from its tier at the bot's band (cancelling in `matk` to about 1 at equal level);
the bot's own vulnerabilities from its gear and buffs (jewel, set and buff stat functions, X2). A
jewel or set multiplies the matching rate by `1 − v/100` (Earring of Antharas: stun × 0.4).

Golden table at equal level (the skill's `magicLvl`, or the caster's level, equals the target's, so
Δ = `lvlDepend`: ×1.01 for Shield Stun, Sleep, Dryad Root, `lvlDepend` 2; ×1.005 for Stun Attack,
`lvlDepend` 1; `formula.go` `SkillLevelModifier` `:599-613`; `skills/0000-0099.xml` 92,
`0100-0199.xml` 100, `1000-1099.xml` 1069, `1200-1299.xml` 1201), `matk` 1, no dyes or resist gear;
bases Shield Stun `power` 80, Stun Attack `effectPower` 50, Sleep and Dryad Root `power` 70, 80.
Cell = `floor(base × stat × level term)` (`LandRate` returns the float; the roll lands on
`ceil(rate)` of 100), for example Human Fighter Shield Stun 80 × 0.743 × 1.01 = 60.04 → 60:

| Opponent base | CON, MEN | Shield Stun (80, ×1.01) | Stun Attack (50, ×1.005) | Sleep (70, ×1.01) | Dryad Root (80, ×1.01) | Cast-break offset |
| --- | --- | --- | --- | --- | --- | --- |
| Human Fighter | 43, 25 | 60 % | 37 % | 61 % | 70 % | −28 |
| Elven Fighter | 36, 26 | 70 % | 43 % | 60 % | 69 % | −30 |
| Dark Fighter | 32, 26 | 75 % | 46 % | 60 % | 69 % | −30 |
| Orc Fighter | 47, 27 | 54 % | 33 % | 60 % | 69 % | −31 |
| Dwarven Fighter | 45, 27 | 57 % | 35 % | 60 % | 69 % | −31 |
| Human Mystic | 27, 39 | 81 % | 50 % | 55 % | 63 % | −48 |
| Elven Mystic | 25, 40 | 83 % | 52 % | 55 % | 62 % | −49 |
| Dark Mystic | 24, 37 | 84 % | 52 % | 56 % | 64 % | −45 |
| Orc Mystic | 31, 42 | 76 % | 47 % | 54 % | 61 % | −52 |

Stuns suit fighters against casters and dark elves (a third less on orcs and dwarves); sleep and
root land about evenly; mystic MEN makes casts hard to break.

## Archetypes

From the class id through its first-profession ancestor (`classParent`, copied with M10's class
templates); the `CharInfo` right-hand weapon splits the scouts:

| Archetype | Classes |
| --- | --- |
| Melee | the fighter base classes; Warrior, Knight, Elven Knight, Palus Knight, Orc Raider, Monk, Scavenger, Artisan and their lines; Rogue, Elven Scout and Assassin lines with a dagger or a sword |
| Archer | Rogue, Elven Scout and Assassin lines with a bow |
| Caster | Human, Elven and Dark Mystic; Human, Elven and Dark Wizard and their lines (summoners included: the bot fights the owner) |
| Healer | Cleric, Elven Oracle, Shillien Oracle and their lines |
| Battle caster | Orc Mystic, Orc Shaman and their lines |

The Artisan line (56, 57, 118; left to the crafter role, `docs/strategy.md:612`) uses SC's row
wherever a row is needed (share, reference opponent, interrupt-worthy steps): both dwarven BLUNT
melee with 100 Stun Attack from 20 (`classes/dwarfFighter.xml:94`).

## Row table

`charmodel` is pure (`docs/strategy.md` section 3) while `Step` and the rows live in
`internal/botclient` (its section 4, `type Step`), so rows come as data. X3 defines `Archetype`
(`Step.Vs`, from X6, is a list of it), `RowTable` (per row, `RefStep{Skill, Level, Vs}` for its
damage, DOT and control steps at the bot's level; skill data from X2) and the class-to-row map
(second professions as their first, scouts by weapon branch, Artisan as SC). `PvPValue(profile, mix,
rows)` and the engage estimate take a `RowTable`; a missing row plays only plain attacks with
grade-typical gear. The brain's `pvpRowTable` (X6; each shipped `Step` list filtered to damage, DOT
and control, `Vs` copied) is filled by X7 and X8; X5 passes an empty table until X6 swaps in
`pvpRowTable`; X10 wires it into the gear scores.

The **mix** (X10, its only reader; owned by the brain goroutine): per non-bot player seen in a
`CharInfo`, object id, class id, last seen; older than an hour drops out; each class maps to its
row. Nobody seen: equal shares over the rows of the bot's tier.

## Engage estimate

For hunting, own challenges and PK victims only (not offered or whispered duels, fighting back,
arenas): the model's time to kill the reference opponent of the opponent's row (class template, the
bot's level, grade-typical gear, the row's `RowTable` steps) against its time to die, with the bot
row's M10 spot margin (tank 1.5, normal 2.0, glass cannon 3.0).

## PvP value

The expected share of won fights over the mix (row `r`, share `s_r`). A **reference opponent**
plays its row's damage, DOT and control steps under their `Vs` gates against the bot's archetype.
Every poison step is gated `vs C, H` (`vs H` on HM; [rows](rows.md)), so melee, archer and
battle-caster bots are never poisoned; bleeds (Bleed 96, Sting 223) are ungated.

- `TTD_r` = (bot max CP + max HP) / the opponent's damage per second: physical vs P.Def and evasion,
  magic vs M.Def, poison and bleed ticks by land rate vs the bot's CON, `poisonVuln`, `bleedVuln`.
- `TTK_r` = (its max CP + max HP) / (the bot's damage per second with `pvpPhysDmg`,
  `pvpPhysSkillsDmg`, `pvpMagicalDmg`, × `u_r`), `u_r` = 1 − the time share `r`'s control holds the
  bot (land rates vs its CON and `stunVuln`, MEN and sleep and root vulns). An effect with its own
  `effectPower` lands only when the skill's own roll landed first, so its land rate is the product of
  the two rolls. Any hit wakes a sleeping player and the holder keeps attacking, so a Sleep holds
  only until the holder's next hit (its attack interval), not for its full time.
- The opponent's damage per second in `TTD_r` is cut the same way by the time share the bot's own
  row's control holds it, so an equal opponent, control rows included, is an even fight.
- `win_r = TTD_r / (TTD_r + TTK_r)` (0.5 even); `PvPValue = Σ s_r × win_r`.

**Blend.** M10 compares gear by the candidate's difference to `Value` (kills per minute for Damage,
`docs/strategy.md` section 3 "Value", section 5 "Gear score"). For `pvp`:
`Value(now) × ((1 − w) × ΔValue / Value(now) + w × ΔPvPValue / PvPValue(now))`, `w` 0.5, `Δ` =
candidate − current loadout; in M10's unit, so every M10 comparison (gear, sets, dyes, jewels,
purchases, grade-jump saving) takes it; `w` 0 is exactly `ΔValue` (other presets). One loadout, no
set swap.

## Gear and consumables (X10)

- At 1-39 PvP value means HP, CP, P.Def, M.Def and cures; jewels to C carry only M.Def, so a
  mystic-heavy mix buys jewels sooner. Mithril Heavy (D, sold like Brigandine) is a purchase candidate,
  not expected to beat Brigandine for a heavy row: Brigandine's pieces have 208 P.Def (41 + 103 + 64) to Mithril's 193 (37 + 95 + 61), its
  set P.Def ×1.05 and 153 HP, its shield 20 HP, against Mithril's `poisonVuln` 20 and shield 126 HP
  (`armorSets.xml:5-6`; `items/0000-0099.xml` 47, 58, 59; `items/2400-2499.xml` 2411,
  `items/0300-0399.xml` 352, `items/2300-2399.xml` 2378; `skills/3500-3599.xml:22-30`, `:59-66`,
  `:492-507`). `poisonVuln` only cuts the gated poison steps ([PvP value](#pvp-value)), so it counts
  only for a caster or healer in those pieces.
- **PvP damage weapons** (143 A- and S-grade; A 99, S 44; for example 4809 Blood
  Tornado - Haste, `item_skill` 3068-2), the B-and-up [PvP sets](server-items.md) and the boss jewels
  score through the same model, worn when owned and within grade (M8), never bought (boss jewels
  list at 616 000 to 4 940 000 adena); all past the M10 range (C grade is 40).
- **Consumables.** The `pvp` M8 supply list (`docs/items.md` "The default list") adds `CureStock`
  (5) Antidotes 1831 and Bandages 1833, from level 20 as many 1832 and 1834 where its trip's
  merchant sells them (23 buy lists each, 30 for the plain); 86-270 adena, bought on the usual
  trips, kept by the junk rule, used per [cures](layer.md#cp-cures-pool-break-off). CP potions:
  kept, PvP only, none bought. HP and M8 buff potions as today.

## Tests

- **Model (X3, test-built `RowTable`s):** `floor(LandRate)` equals the table at the skill's
  `magicLvl` (Human Fighter Sleep 70 × 0.8686 × 1.01 = 61.41 → 61; Human Mystic Shield Stun 80 ×
  1.0100 × 1.01 = 81.61 → 81; Orc Fighter Stun Attack 50 × 0.6696 × 1.005 = 33.65 → 33); `LandRate`
  equals the server formula within 1e-9 (a vuln jewel, a blessed spiritshot, a level gap); the land
  base of a PDAM, an MDAM, an effect-power and a level `effectPower` skill; the cast-break chance;
  the archetype and row of class ids 0-57 and 88-118 per right-hand weapon (56, 57, 118 → SC); a
  row with only Poison 129 `vs C, H` lowers a CL's `PvPValue`, leaves a WA's (within 1e-9), ungated
  lowers both; a missing row plays plain attacks; `win_r` 0.5 against an equal; `PvPValue` the
  share-weighted sum; blend `w` 0 equals `ΔValue` exactly, `w` 0.5 scores +10 % `Value` as +10 %
  `PvPValue`; a 1.05 `pvpPhysDmg` weapon above the same without; Earring of Antharas raises `u_r`
  against a stun row.
- **Shipped rows (X10, after X7, X8):** `pvpRowTable` holds every row's damage, DOT and control
  steps with `Vs`; Mithril Heavy with set skill 3502 on a CL beats the same pieces without it
  against a poison-row mix (HM, DM, SO, DW, PK, AS, OM), on a WA equal (within 1e-9), on either
  equal against HF, WA, KN, HW, CL (3502 adds only `poisonVuln`, `skills/3500-3599.xml:22-30`). Mix:
  seen 59 minutes ago counts, 61 not, a bot-marked player never; nobody seen → equal over the tier.
