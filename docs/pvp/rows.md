# Behavior per class (PvP)

X7 ships the fighter rows, X8 the mystic rows: M10's nine base classes and seventeen first
professions with its row ids and notation (`docs/strategy.md` "Behavior per class": `@N` level,
`[KIND]` weapon, `S<N` own HP, `T≥N` target HP, `≤N` range, `A≥2` attackers, `C≥N` charges), plus
`vs M`, `vs A`, `vs C`, `vs H`, `vs B` and `Int`, `Hold`, `Break`, `Peel`
([step fields](layer.md#step-fields)). Skill ids are in the row's class tree at that level
(`classes/*.xml`), land bases from `skills/*.xml`; a row lists only what differs from M10.
[Who plays which row](modes.md#who-plays-which-row); second professions use their first
profession's row until #332.

## Opener, control and interrupt

| Row | PvP opener | Control-first rotation | Interrupt |
| --- | --- | --- | --- |
| HF | Bow: 56 Power Shot @5 from 700. Else walks in. | No control skill: 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5, 16 Mortal Blow [DAGGER] @5, melee. | Damage only (Power Strike), under `MinBreak`. |
| EF | As HF. | As HF. | As HF. |
| DF | 70 Drain Health @15 from 600 (vs any). | 70 Drain Health; then as HF. | As HF. |
| OF | Walks in. | 29 Iron Punch [DUALFIST] @5, 3 Power Strike, melee. | Iron Punch, under `MinBreak`. |
| WF | Walks in (no Spoil on players). | Melee. | none |
| HM | vs M, A: 1184 Ice Bolt @7 (slow) from 600; vs C, H, B: 1177 Wind Strike. | vs M: 1164 Curse: Weakness @14 (P.Atk down, base 80); 1184 Ice Bolt @7 (kite); vs H: 1168 Curse: Poison @7; 1177 Wind Strike. | None: Ice Bolt (power 8-16) never reaches `MinBreak` against a mystic's MEN ([interrupts](layer.md#interrupts)). |
| EM | vs M, A: 1206 Wind Shackle @14 (attack speed down, base 80); else 1184 Ice Bolt. | vs M: 1206 Wind Shackle, 1164 Curse: Weakness @14, 1184 Ice Bolt (kite); 1177 Wind Strike. | As HM. |
| DM | vs M, A: 1206 Wind Shackle @14; vs C, H: 1168 Curse: Poison @7. | vs M: 1206 Wind Shackle, 1184 Ice Bolt (kite); vs C, H: 1168 Curse: Poison; 1177 Wind Strike. | As HM. |
| OM | 1100 Chill Flame @7 from 600. | vs C, H: 1095 Venom @7; 1100 Chill Flame; 1090 Life Drain @7. Break: 1097 Dreaming Spirit @7 (sleep, base 80) only on an opponent carrying no DOT ([sleep and damage](layer.md#pvp-order)), else 1092 Fear @14 (base 40); with its own Venom or Chill Flame on the opponent (usual) that means Fear; below 14 no `Break` step: it flees at once. | 1097 Dreaming Spirit Int, timed by [interrupts](layer.md#interrupts): a Sleep 1069, a Cleric's 1011 Heal without a spiritshot, not with one; without its own spiritshot no heal. |
| KN | vs C, A: 70 Drain Health @20 from 600. Else walks in. | 92 Shield Stun [SHIELD] @20 (base 80) Hold vs C, H, else at reuse; 3 Power Strike; melee. Peel: 28 Aggression @24 (TargetMe). | 92 Shield Stun Int. |
| WA | Walks in; 78 War Cry @20 at engage. | vs C, H, B: 287 Lionheart @36 at engage; 100 Stun Attack [BLUNT, BIGBLUNT] @20 (base 50) Hold vs C, H; 255 Power Smash @20; 245 Wild Sweep [POLE] A≥2 ([area rule](guards.md#area-hits-and-bystanders)). | 100 Stun Attack Int. |
| RO | Dagger: walks in, 4 Dash @20 from beyond 400 vs C, A. Bow: 56 Power Shot from 700; 101 Stunning Shot @36 from 900. | Dagger: 96 Bleed [DAGGER] @24 T≥50, 16 Mortal Blow. Bow: 101 Stunning Shot [BOW] @36 (stun and target drop, base 50) Hold vs C, H; 56 Power Shot. Break: 4 Dash. | Bow: 101 Stunning Shot Int, timed by [interrupts](layer.md#interrupts): a Heal, a Sleep, a slow caster's 4-5 s nukes, never a Battle Heal (2000 ms). Dagger: damage only. |
| EK | Walks in. | vs C, A: 102 Entangle @36 (slow, base 80); 3 Power Strike; melee. Peel: 28 Aggression @24. Break: 102 Entangle. | Damage only. |
| ES | Bow: 56 Power Shot from 700; vs M: 102 Entangle @36 first (kite). Dagger: walks in. | 102 Entangle vs M (bow) or vs C, A (dagger); dagger: 96 Bleed, 16 Mortal Blow; bow: 101 Stunning Shot @36 Hold vs C, H, 56 Power Shot. Break: 102 Entangle, 230 Sprint @32. | Bow: 101 Stunning Shot Int, timed as RO's. |
| PK | 70 Drain Health @20 from 600. | vs C, H: 129 Poison @20 (base 70, CON); vs M, A: 115 Power Break @32 (P.Atk down, base 80); 105 Freezing Strike @36 (slow) vs M; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24; 3 Power Strike. Peel: 28 Aggression @24. | Damage only. |
| AS | Dagger: 70 Drain Health @20 from 600. Bow: as RO. | Dagger: vs C, H: 129 Poison @20; vs M: 115 Power Break @32; 96 Bleed; 16 Mortal Blow; 223 Sting. Bow: as RO. | Bow: as RO. Dagger: damage only. |
| OR | Walks in. | vs C, H, B: 94 Rage @24 and 287 Lionheart @36 at engage (Rage costs P.Def, so never vs M, A); 100 Stun Attack Hold vs C, H; 255 Power Smash. | 100 Stun Attack Int. |
| MO | 54 Force Blaster [DUALFIST] @24 C≥1 from 600 vs C, A. Else walks in. | vs C, A: 95 Cripple [FIST, DUALFIST] @20 (slow, base 80); 120 Stunning Fist [DUALFIST] @20 (base 50) Hold vs C, H; 284 Hurricane Assault @36 C≥2; 54 Force Blaster C≥1; 29 Iron Punch. | 120 Stunning Fist Int. |
| SC | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20 Hold vs C, H; melee (no Spoil on players). | 100 Stun Attack Int. |
| CL | vs M: 1201 Dryad Root @25 (base 80) from 600, then kite; else 1177 Wind Strike. | vs M: 1201 Dryad Root, 1184 Ice Bolt (kite); vs C: 1177 Wind Strike. Break: 1201 Dryad Root (hit 2500, a 30 s root), then flee. | None: no stun or sleep; Ice Bolt never reaches `MinBreak`. |
| HW | vs M, A: 1184 Ice Bolt (slow); vs C, H: 1220 Blaze @20 from 750. | vs M: 1184 Ice Bolt, 1160 Slow @35 (kite); vs M, A: 1222 Curse Chaos @35 (accuracy down); 1220 Blaze; 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | 1274 Energy Bolt Int (hit 2000), 1172 Aura Burn Int ≤150 (hit 1500), when the break chance passes `MinBreak`. |
| EO | vs M: 1206 Wind Shackle (base), then 1201 Dryad Root @25 (kite); else 1177 Wind Strike. | As CL, with 1206 Wind Shackle vs M first. | As CL. |
| EW | vs M, A: 1206 Wind Shackle; else 1264 Solar Spark @25 from 750, before 25 1175 Aqua Swirl. | vs M: 1206 Wind Shackle, 1184 Ice Bolt (kite); 1264 Solar Spark; 1175 Aqua Swirl; 1274 Energy Bolt ≤400; 1172 Aura Burn ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | As HW. |
| SO | vs M: 1206 Wind Shackle, then 1201 Dryad Root @25; vs C, H: 1168 Curse: Poison. | As CL, with 1206 Wind Shackle vs M and 1168 Curse: Poison vs C, H. | As CL. |
| DW | vs M, A: 1206 Wind Shackle; vs C, H: 1168 Curse: Poison; else 1266 Shadow Spark @25 from 750. | vs M: 1206 Wind Shackle, 1160 Slow @35, 1184 Ice Bolt (kite); 1266 Shadow Spark; 1178 Twister; 1172 Aura Burn ≤150. Break: 1184 Ice Bolt (slow, hit 3100) unless the opponent is still slowed, then flee. | 1172 Aura Burn Int ≤150, when the break chance passes `MinBreak`. |
| OS | 1107 Frost Flame @20 from 750. | vs C, H: 1102 Aura Sink @25 (MP drain over time); 1107 Frost Flame; 1090 Life Drain; 100 Stun Attack [BLUNT, BIGBLUNT] ≤40 Hold vs C, H. Break: 1208 Seal of Binding @25 (root around itself) on a melee opponent next to it, 1097 Dreaming Spirit only on an opponent carrying no DOT (its own Frost Flame included), 1092 Fear (all hit 4000, so under the [break-cast gate](layer.md#cp-cures-pool-break-off): an orc mystic's MEN keeps the break chance low). | 100 Stun Attack Int ≤40; 1097 Dreaming Spirit Int timed as OM's. |

## Matchup, CP and break-off

| Row | vs melee | vs caster or healer | Sustain and CP (besides CP potions) | Breaks off at |
| --- | --- | --- | --- | --- |
| HF | Trades blows. | Closes in fast; stays in melee. | HP potions (rule 3). | normal 30 |
| EF | Trades blows. | Closes in (run 144). | 58 Elemental Heal @15 S<50, long-cast rule. | normal 30 |
| DF | Drains first. | Drain Health from 600 while it closes. | 70 Drain Health S<60. | glass 40 |
| OF | Outlasts (HP ×1.77). | Closes in; at about 124 it outruns only a Human Mystic line (about 121), so it usually fights on (`FleeOutrun` decides). | none | tank 20 |
| WF | Trades blows. | Closes in. | none | normal 30 |
| HM | Slows and weakens, kites, nukes. | Out-casts; poisons a healer. | 1147 Vampiric Touch @14 S<60; 1216 Self Heal S<40, long-cast rule. | glass 40 |
| EM | Shackles and weakens, kites. | Out-casts (cast ×1.16). | 1216 Self Heal S<50, long-cast rule. | glass 40 |
| DM | Shackles, kites. | Poisons, then nukes hardest. | 1147 Vampiric Touch @14 S<70. | glass 40 |
| OM | Burns and drains; fears to break off (sleeps only an opponent free of DOTs). | Venom and burns; sleeps a long cast. | 1090 Life Drain S<70. | normal 30 |
| KN | Stuns at reuse; heavy armor. | Holds Shield Stun for heals and long casts. | 45 Divine Heal @28 S<40 (long-cast rule); 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M, only where it will not break off ([Ultimate Defense](layer.md#ultimate-defense)). vs A: 112 Deflect Arrow @24 buffed out of combat once an archer is in sight. | tank 20 |
| WA | Stuns, smashes. | Lionheart, then holds Stun Attack for casts. | 121 Battle Roar @28 S<30. | normal 30 |
| RO | Bleeds and blows; Ultimate Evasion. | Dashes in; bow: Stunning Shot on long casts. | 111 Ultimate Evasion @28 S<30 vs M, A (no help against magic). | normal 30 |
| EK | Trades blows behind a shield. | Entangle a kiting caster, closes in. | 58 Elemental Heal @20 S<50 (long-cast rule); 110 Ultimate Defense S<20 vs M, as KN; 112 Deflect Arrow vs A. | normal 30 |
| ES | Bow: Entangle, kite and shoot. Dagger: blows. | Closes in (dagger) or shoots (bow). | 58 Elemental Heal S<50 (long-cast rule); 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| PK | Power Break, Freezing Strike, Sting. | Drains and poisons, closes in. | 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M, as KN; 112 Deflect Arrow vs A. | normal 30 |
| AS | Power Break, then blows. | Drains and poisons, hardest blows. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 vs M, A. | glass 40 |
| OR | Smashes; no Rage. | Rage and Lionheart, holds Stun Attack. | 176 Frenzy @32 S<30, else 139 Guts @36; 121 Battle Roar S<30. | tank 20 |
| MO | Stunning Fist at reuse, Hurricane Assault. | Cripples a kiter, holds Stunning Fist. | none | normal 30 |
| SC | Stuns, trades blows. | Holds Stun Attack for casts. | none | tank 20 |
| CL | Roots, kites, nukes. | Out-heals: 1015 Battle Heal @20 (hit 2000) S<35 first. | 1015 Battle Heal S<35; 1011 Heal S<60 (long-cast rule); 1035 Mental Shield @25 on itself vs C, B. | glass 40 |
| HW | Slows, kites, burns. | Blaze race; breaks long casts with Energy Bolt. | 1147 Vampiric Touch S<60; 1157 Body To Mind @25. | glass 40 |
| EO | Shackles, roots, kites. | As CL. | As CL. | glass 40 |
| EW | Shackles, kites. | Solar Spark race; Energy Bolt interrupts. | 1216 Self Heal S<40 (long-cast rule). | glass 40 |
| SO | Shackles, roots, kites. | Poisons, out-heals. | As CL; 1147 Vampiric Touch S<60. | glass 40 |
| DW | Shackles, slows, kites. | Poisons, Shadow Spark burst. | 1147 Vampiric Touch S<70; 1157 Body To Mind. | glass 40 |
| OS | Stuns at reach; Seal of Binding to break off. | Aura Sink drains a caster's MP; holds Stun Attack. | 1090 Life Drain S<70; 1229 Chant of Life under the long-cast rule; 1006 Chant of Fire @20 (M.Def) vs C, B before the fight. | normal 30 |

## Tests

- `TestPvPRowsShipped` (X6 with no row, extended by X7 and X8): the tables against the shipped
  datapack, as `TestBehaviorRowsShipped` does.
- One `TestPvPRow<ID>` (X7, X8) per row: its opener against the named archetype, its `Hold` and
  `Int` step, its `Break` step.
- Row cases of [layer](layer.md#tests) and [guards](guards.md#tests) tests run in X7 and X8;
  `TestPvPPKCap`'s WA and HM cases in X11.
