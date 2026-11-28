# Second-profession PvP rows (#421)

Status: design, for owner approval before implementation (#421). Docs only; no behavior changes
until the issues in [Delivery](#delivery) ship.

The 24 second professions of M10's "Second professions" (`docs/strategy.md`; epic #624, rows
shipped in #625-#635) get PvP rows of their own, in the [rows](rows.md) notation (`@N`, `[KIND]`,
`S<N`, `T≥N`, `≤N`, `A≥2`, `C≥N`, `vs M/A/C/H/B`, `Int`, `Hold`, `Break`, `Peel`,
[step fields](layer.md#step-fields)) with the second-profession notation (`docs/strategy.md`
"Second professions"): the row start is 40, so an untagged skill is known at 40; `@N` is learned
past 40; `(fixed)` is a step kept at the level the first profession reached by 39, behind its
successor. Land rates are [model](model.md#land-rates)'s golden table; skill facts are from
`skills/*.xml` and `classes/*.xml` (datapack paths as in the [README](README.md#paths)).

## Today

Each second-profession kit sets `PvP` to its first profession's list (`kits_tanks.go`,
`kits_songs.go`, `kits_heavy.go`, `kits_daggers.go`, `kits_archers.go`, `kits_tyrant.go`,
`kits_nukers.go`, `kits_healers.go`), and `pvpSteps` puts that list ahead of the M10 steps it does
not name. Three faults follow:

- **First-profession attacks outrank the second profession's.** HW's list names 1220 Blaze, 1274
  Energy Bolt and 1172 Aura Burn, so a Sorcerer casts them ahead of 1230 Prominence; the same for
  SG (EW's 1264 Solar Spark, 1175 Aqua Swirl), SH (DW's 1266 Shadow Spark, 1178 Twister), WC (OS's
  1107 Frost Flame and 1090 Life Drain ahead of 1245 Steal Essence) and TY (MO's 284 Hurricane
  Assault and 54 Force Blaster ahead of 280 Burning Fist and 281 Soul Breaker).
- **The second tree's control never plays as control.** 260 Hammer Crush (DS, BH, WC) and 281 Soul
  Breaker (TY) land a stun (`Stun`, `effectPower` 50, Stun Attack's land table) but run as plain
  damage, while the first row's (fixed) stun holds the `Hold`; 403 Shackle, 402 Arrest, 1394
  Trance and 1064 Silence are not cast at all (M10 left them out against monsters).
- **The row table has no second rows.** `rowClass` (`pvp_layer.go`) admits base classes and first
  professions only, and `charmodel.ClassRow` walks a second profession to its first, so the engage
  estimate and the PvP value see a Sorcerer as a Human Wizard with a Human Wizard's steps.

## Conventions

- A second row is a **whole** PvP list (`ClassKit.PvP`): it repeats every step of the first
  profession's list it keeps, as an M10 second-profession row does (`docs/strategy.md` "Second
  professions"). Its steps replace the M10 steps of every skill they name (`pvpSteps`; a side cast
  on another attacker stays).
- The list names control, `Hold`, `Int`, `Break`, `Peel`, sustain, PvP self steps and openers, and
  a damage skill only where its PvP play differs from M10 (a `Vs` gate, an `Int`). Every other
  attack plays its M10 step, in M10 order, after the list. The tables below write those as "then
  M10".
- A row "as XX" keeps XX's list unchanged: its kit already sets it, and the row id still maps to
  the second profession ([X20](#delivery)) so the row table reads its own M10 steps.
- A `Hold` step is also its row's interrupt (`Step.Hold`: kept ready against a caster or healer,
  cast only as an interrupt; at reuse against anyone else).
- The [area rule](guards.md#area-hits-and-bystanders) bars area and aura damage in the open world
  with a flagged bystander in reach; area steps below play under it unchanged.
- Break-off bounds follow the M10 spot margin of the second row (`docs/strategy.md` "Numbers and
  summary", column 10): tank 20, normal 30, glass cannon 40.

## Owner decisions to approve

- **O1, own-target holds against players.** M10 left out 402 Arrest, 403 Shackle, 1394 Trance and
  1064 Silence because they stop a kill against a monster. Against a player they are the second
  tree's control, under the layer's rules ([PvP order](layer.md#pvp-order): `MinLand`, the sleep
  rule, the break-cast gate). Proposed: admit them as below. Data (`skills/0400-0499.xml`,
  `1000-1099.xml`, `1300-1399.xml`):
  - 403 Shackle (PL @55): ROOT, power 80, hit 2000, reuse 8 s, root 10 s, cast range 600; land as
    Dryad Root (61-70 %).
  - 402 Arrest (TK, SK, SS, BD @55): ROOT plus TargetMe, power 70, hit 2000, reuse 8 s, root 8 s,
    cast range 150; land as Sleep (54-61 %).
  - 1394 Trance (BI, EE @56): SLEEP, power 80, hit 3000, reuse 5 s, sleep 15 s.
  - 1064 Silence (SH): MUTE, power 80, hit 4000, reuse 60 s, mute 30 s (a muted player casts no
    magic skill: server `model/actor/cast/player_actor.go` `MagicMuted`).
  - Kept left out: 404 Mass Shackling, 1072 Sleeping Cloud, 1069 Sleep ([left out](#left-out-and-why)).
- **O2, 320 Wrath.** CPDAMPERCENT, power 30 (30 % of the targets' CP), AREA radius 80, hit 1080,
  reuse 120 s, `weaponsAllowed` POLE (`skills/0300-0399.xml`). M10 left it out as "CP only: PvP,
  #421". Proposed: WL's opener from 66, under the area rule. DS and BH learn it but never wield a
  pole, so it stays out of their rows.
- **O3, rows kept as the first profession's.** TH and HK (as RO), PP (as CL), SE (as SO): their
  second tree adds attacks and heals the M10 row already plays, and no control.
- **O4, second rows in the row table.** The engage estimate and the PvP value mix read the 24 rows
  (X20), which changes the gear choices and hunting decisions of every bot past 40 (D24: X20 states
  it).

## Opener, control and interrupt

| Row | PvP opener | Control-first rotation | Interrupt |
| --- | --- | --- | --- |
| PL | vs C, A: 70 Drain Health (fixed) from 600. Else walks in. | 92 Shield Stun [SHIELD] Hold vs C, H, else at reuse; 403 Shackle @55 vs C, H, A (root: a kiting opponent stays for the walk in); then M10 (400 Tribunal @55, 3 Power Strike (fixed)). Peel: 28 Aggression. Break: 403 Shackle @55. | 92 Shield Stun Int. |
| TK | Walks in. | vs C, A: 102 Entangle; 402 Arrest @55 vs C, H, A; then M10 (400 Tribunal @55, 3 Power Strike (fixed)). Peel: 28 Aggression. Break: 402 Arrest @55, 102 Entangle. | Damage only. |
| SK | 70 Drain Health from 600 (M10). | vs C, H: 129 Poison; vs M, A: 115 Power Break; 105 Freezing Strike vs M; 402 Arrest @55 vs C, H, A; then M10 (289 Life Leech, 401 Judgment @55, 223 Sting, 3 Power Strike (fixed)). Peel: 28 Aggression. Break: 402 Arrest @55, 105 Freezing Strike. | Damage only. |
| SS | Walks in. | As TK without Tribunal: vs C, A: 102 Entangle; 402 Arrest @55 vs C, H, A; then M10 (3 Power Strike (fixed)). Peel: 28 Aggression. Break: 402 Arrest @55, 102 Entangle. | Damage only. |
| BD | 70 Drain Health from 600 (M10). | vs C, H: 129 Poison; vs M, A: 115 Power Break; 105 Freezing Strike vs M; 402 Arrest @55 vs C, H, A; then M10 (84 Poison Blade Dance and 408 Demonic Blade Dance [DUAL] @55 A≥2 under the area rule, 223 Sting). Peel: 28 Aggression. Break: 402 Arrest @55, 105 Freezing Strike. | Damage only. |
| GL | 6 Sonic Blaster @43 C≥2 from 600 (M10); 287 Lionheart vs C, H, B at engage. | No control: a DUAL takes neither 100 Stun Attack nor 260 Hammer Crush (BLUNT, BIGBLUNT); then M10 (the sonic slashes, 1 Triple Slash). | 1 Triple Slash [DUAL] Int (hit 1733) when the break chance passes `MinBreak`. |
| WL | Walks in; 287 Lionheart vs C, H, B at engage; 320 Wrath [POLE] @66 opens (O2, area rule). | 48 Thunder Storm [POLE] (aura stun, `effectPower` 50) Hold vs C, H, under the area rule; then M10 (36 Whirlwind, 245 Wild Sweep A≥2). | 48 Thunder Storm Int, where the area rule allows it. |
| DS | Walks in; vs C, H, B: 94 Rage and 287 Lionheart at engage (as OR). | 260 Hammer Crush [BLUNT, BIGBLUNT] Hold vs C, H; 100 Stun Attack (fixed) Hold vs C, H; then M10 (315 Crush of Doom @60, 190 Fatal Strike, 255 Power Smash (fixed)). | 260 Hammer Crush Int, else 100 Stun Attack Int. |
| BH | Walks in (no Spoil on players). | 260 Hammer Crush [BLUNT, BIGBLUNT] Hold vs C, H; 100 Stun Attack (fixed) Hold vs C, H; melee. | As DS. |
| TH | As RO (dagger). | As RO: no control; M10's blows. No 51 Lure on players (below). | Damage only. |
| PW | Walks in; vs C, A: 102 Entangle from 600. | vs C, A: 102 Entangle; then M10 (263 Deadly Blow, 321 Blinding Blow @66 as damage, 96 Bleed T≥50, 16 Mortal Blow (fixed)). No 51 Lure on players. Break: 102 Entangle, 230 Sprint. | Damage only. |
| AW | 70 Drain Health from 600 (M10). | vs C, H: 129 Poison; vs M: 115 Power Break; then M10 (263 Deadly Blow, 321 Blinding Blow @66 as damage, 96 Bleed T≥50, 223 Sting, 16 Mortal Blow (fixed)). Break: 105 Freezing Strike (slow, hit 3000). | Damage only. |
| HK | As RO (bow): 101 Stunning Shot at reuse vs M, A from 900, else 19 Double Shot (M10 order). | As RO: 101 Stunning Shot [BOW] Hold vs C, H; then M10 (19 Double Shot, 24 Burst Shot @46 A≥2, 56 Power Shot (fixed)). Break: 4 Dash. | 101 Stunning Shot Int, timed as RO's. |
| SR | vs M: 102 Entangle from 600 (kite); else as HK. | 102 Entangle vs M (kite); 101 Stunning Shot [BOW] Hold vs C, H; then M10 (19 Double Shot, 24 Burst Shot @46 A≥2, 56 Power Shot (fixed)). Break: 102 Entangle, 230 Sprint. | 101 Stunning Shot Int, timed as RO's. |
| PR | vs M: 105 Freezing Strike from 600 (kite); else as HK. | 105 Freezing Strike vs M (kite, as ES's bow branch after Entangle); vs C, H: 129 Poison; 101 Stunning Shot [BOW] Hold vs C, H; then M10 (314 Fatal Counter [BOW] @60 S<50, 19 Double Shot, 56 Power Shot (fixed)). Break: 105 Freezing Strike. | 101 Stunning Shot Int, timed as RO's. |
| TY | vs C, A: 54 Force Blaster [DUALFIST] C≥1 from 600. Else walks in. | vs C, A: 95 Cripple (slow); 281 Soul Breaker [DUALFIST] Hold vs C, H; 120 Stunning Fist (fixed) Hold vs C, H; then M10 (35 Force Storm and 17 Force Burst A≥2 under the area rule, 284 Hurricane Assault C≥2, 280 Burning Fist, 54 Force Blaster C≥1, 29 Iron Punch (fixed)). Break: 95 Cripple. | 281 Soul Breaker Int, else 120 Stunning Fist Int. |
| SN | vs M, A: 1184 Ice Bolt (fixed) from 600; vs C, H: 1230 Prominence from 900. | vs M: 1160 Slow, 1184 Ice Bolt (fixed) (kite); vs M, A: 1222 Curse Chaos; then M10 (1230 Prominence, 1083 Surrender To Fire once, 1296 Rain of Fire @58 A≥2, 1275 Aura Bolt ≤400, 1231 Aura Flare ≤150, 1220 Blaze (fixed)). Break: 1184 Ice Bolt unless still slowed, then flee. | 1275 Aura Bolt Int ≤400 (hit 2000), 1231 Aura Flare Int ≤150 (hit 1500), when the break chance passes `MinBreak`. |
| SG | vs M, A: 1206 Wind Shackle (fixed) from 600; else 1235 Hydro Blast from 900. | vs M: 1206 Wind Shackle (fixed), 1236 Frost Bolt (slow, hit 3100; kite), 1164 Curse: Weakness; then M10 (1235 Hydro Blast, 1237 Ice Dagger @44, 1265 Solar Flare, 1295 Aqua Splash @58 A≥2, 1275 Aura Bolt ≤400, 1231 Aura Flare ≤150, 1264 Solar Spark (fixed), 1175 Aqua Swirl (fixed)). Break: 1236 Frost Bolt unless still slowed, then flee. | As SN. |
| SH | vs M, A: 1206 Wind Shackle (fixed) from 600; vs C, H, B: 1064 Silence (O1) from 600; else 1239 Hurricane from 900. | vs C, H, B: 1064 Silence; vs C, H: 1168 Curse: Poison; vs M: 1206 Wind Shackle (fixed), 1160 Slow, 1184 Ice Bolt (fixed) (kite); then M10 (1239 Hurricane, 1148 Death Spike @44, 1267 Shadow Flare, 1176 Tempest @48 A≥2, 1266 Shadow Spark (fixed), 1178 Twister (fixed), 1172 Aura Burn (fixed) ≤150). Break: 1184 Ice Bolt unless still slowed, then flee. | 1172 Aura Burn (fixed) Int ≤150, as DW. |
| WC | 1245 Steal Essence from 900 (M10). | vs C, H: 1102 Aura Sink; 260 Hammer Crush [BLUNT, BIGBLUNT] ≤40 Hold vs C, H; 100 Stun Attack (fixed) ≤40 Hold vs C, H; then M10 (1245 Steal Essence, 1107 Frost Flame (fixed)). Break: 1208 Seal of Binding (fixed) on a melee opponent next to it, 1097 Dreaming Spirit only on an opponent carrying no DOT, 1092 Fear (as OS, under the break-cast gate). | 260 Hammer Crush Int ≤40, else 100 Stun Attack Int ≤40; 1097 Dreaming Spirit Int timed as OM's. |
| BI | vs M: 1201 Dryad Root from 600, then kite; else 1177 Wind Strike (fixed). | As CL: vs M: 1201 Dryad Root, 1184 Ice Bolt (kite); 1177 Wind Strike. Break: 1201 Dryad Root on a melee opponent, else 1394 Trance @56 (O1) only on an opponent carrying no DOT; then flee. | 1394 Trance @56 Int, timed by [interrupts](layer.md#interrupts) (a Heal, a Sleep, a long cast); none before 56. |
| PP | As CL. | As CL. | As CL (none). |
| EE | vs M: 1206 Wind Shackle, then 1201 Dryad Root (kite); else 1177 Wind Strike (fixed). | As EO; Break: 1201 Dryad Root on a melee opponent, else 1394 Trance @56 only on an opponent carrying no DOT; then flee. | As BI. |
| SE | As SO. | As SO. | As SO (none). |

## Matchup, CP and break-off

| Row | vs melee | vs caster or healer | Sustain and CP (besides CP potions) | Breaks off at |
| --- | --- | --- | --- | --- |
| PL | Stuns at reuse; Shackle to break off. | Shackles a kiting caster, holds Shield Stun for heals and long casts. | 262 Holy Blessing S<50 (hit 5000, long-cast rule); 45 Divine Heal (fixed) S<40 (long-cast rule); 70 Drain Health (fixed) S<60; 110 Ultimate Defense S<20 vs M; 112 Deflect Arrow vs A, `Sighted` (as KN). | tank 20 |
| TK | Trades blows behind a shield; Arrest or Entangle to break off. | Entangles and arrests a kiting caster, closes in. | 58 Elemental Heal S<50 (long-cast rule); 110 Ultimate Defense S<20 vs M; 112 Deflect Arrow vs A, as KN. No cubics (#636). | tank 20 |
| SK | Power Break, Freezing Strike, Sting. | Drains twice, poisons, arrests a kiter. | 289 Life Leech S<70; 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M; 112 Deflect Arrow vs A. No cubics (#636). | tank 20 |
| SS | As TK; songs up before the fight (M10). | As TK. | 58 Elemental Heal S<50 (long-cast rule); 110 Ultimate Defense S<20 vs M; 112 Deflect Arrow vs A. | normal 30 |
| BD | Power Break, Freezing Strike; blade dances only where the area rule allows. | Poisons, arrests a kiter. | 70 Drain Health S<60; 110 Ultimate Defense S<20 vs M; 112 Deflect Arrow vs A. | normal 30 |
| GL | Sonic slashes. | Lionheart, Sonic Blaster to close; Triple Slash breaks a magic cast. | 121 Battle Roar S<30. | normal 30 |
| WL | Pole sweeps under the area rule. | Lionheart; Wrath strips CP at 66; holds Thunder Storm for casts. | 121 Battle Roar S<30; 181 Revival @55 S<15. | normal 30 |
| DS | Hammer Crush at reuse, Crush of Doom. | Rage and Lionheart, holds Hammer Crush. | 121 Battle Roar S<30; 176 Frenzy S<30, else 139 Guts; 420 Zealot @58 S<30. | tank 20 |
| BH | Hammer Crush at reuse. | Holds Hammer Crush for casts. | none | tank 20 |
| TH | As RO (dagger). | As RO. | 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| PW | Blows; Entangle and Sprint to break off. | Entangles a kiting caster, closes in. | 58 Elemental Heal S<50 (long-cast rule); 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| AW | Power Break, then blows; Freezing Strike to break off. | Drains and poisons, hardest blows. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 vs M, A. | glass 40 |
| HK | Stuns at reuse, shoots. | Double Shot; holds Stunning Shot for casts. | 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| SR | Entangles, kites and shoots. | As HK. | 58 Elemental Heal S<50 (long-cast rule); 111 Ultimate Evasion S<30 vs M, A. | normal 30 |
| PR | Freezing Strike, kites and shoots; Fatal Counter when low. | Poisons; holds Stunning Shot. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 vs M, A. | glass 40 |
| TY | Soul Breaker at reuse, Burning Fist. | Cripples a kiter, holds Soul Breaker. | 420 Zealot @58 S<30. | normal 30 |
| SN | Slows, kites, Prominence. | Prominence race; Aura Bolt breaks long casts. | 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind. | glass 40 |
| SG | Shackles, slows with Frost Bolt, kites. | Hydro Blast race; Aura Bolt interrupts. | 1216 Self Heal S<40 (long-cast rule). | glass 40 |
| SH | Shackles, slows, kites. | Silences, poisons, Hurricane burst. | 1234 Vampiric Claw S<70; 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind. | glass 40 |
| WC | Stuns at reach; Seal of Binding to break off. | Aura Sink, holds Hammer Crush. | 1245 Steal Essence S<70; 1090 Life Drain (fixed) S<70; 1229 Chant of Life (long-cast rule); 1006 Chant of Fire vs C, B before the fight (as OS). | normal 30 |
| BI | Roots, kites. | Out-heals: 1218 Greater Battle Heal (hit 2000) S<35 first; Trance on a heal. | 1218 Greater Battle Heal S<35; 1258 Restore Life @44 S<20 (hit 8000, long-cast rule); 1401 Major Heal @56, else 1217 Greater Heal, else 1020 Vitalize @48 S<60 (long-cast rule); 1035 Mental Shield vs C, B. | glass 40 |
| PP | As CL. | As CL. | As CL. | glass 40 |
| EE | Shackles, roots, kites. | As BI, with 1015 Battle Heal (fixed) first. | 1015 Battle Heal (fixed) S<35; 1401 Major Heal @56, else 1217 Greater Heal @48, else 1020 Vitalize @48, else 1011 Heal S<60 (long-cast rule); 1035 Mental Shield vs C, B. | glass 40 |
| SE | As SO. | As SO. | As SO; 1217 Greater Heal @48 S<60 ahead of 1011 Heal (M10). | glass 40 |

Heals and drains above are M10 sustain under the [long-cast rule](layer.md#pvp-order); only the
PvP steps (`Vs`, `Sighted`, `Hold`, `Int`, `Break`, `Peel`) are new list entries.

## Left out, and why

- **404 Mass Shackling** (PL): an aura root of power 40 (half Dryad Root's base: about 31-35 %, at
  or under `MinLand`), reuse 40 s; it roots bystanders too.
- **1069 Sleep** (SN, SG, SH, BI, EE): hit 10000, too slow for an interrupt and barred from
  break-off ([break-cast gate](layer.md#cp-cures-pool-break-off)); **1072 Sleeping Cloud** (SN,
  SG): an area sleep of hit 15000.
- **1169 Curse Fear** (SN, SG, SH) and **1105 Madness** (WC): fear base 40 and confusion base 20,
  hit 4000; the rows' slows and roots already break off.
- **279 Lightning Strike** (SK): a Paralyze of `effectPower` 25 (half Stun Attack's 50, about
  16-26 %) makes it a control step for the layer (`isControl`), under `MinLand`, so the layer
  skips it against a player. Its 60 s reuse makes it a minor damage step; not listed.
- **321 Blinding Blow** (PW, AW): this server's Bluff only turns the target
  (`skill/effect/hooks_buff.go:44-50`, `bluffStart`), with no stun and no cast abort, so it is a
  damage blow at its M10 step. 30 Backstab after it stays out: the target turns back on its next
  action and the bot does not track headings.
- **Monster-only:** 2 Confusion (SK, BD, AW, PR; RandomizeHate), 12 Switch, 106 Veil, 1075 Peace,
  1273 Serenade of Eva, 18 Aura of Hate and 286 Provoke as aggro, and **51 Lure** (TH, PW): an
  AGGDAMAGE without TargetMe only notifies a monster (`handler/skill/disablers.go:162-168`,
  `disableAggDamage`), so against a player the M10 Lure step is skipped (X24). 450 Banish Seraph
  (SK) targets angels only (`target race_id 8`, `skills/0400-0499.xml`).
- **Owner drops (2026-11-26) stand:** 98 Sword Symphony, 452 Shock Stomp, 317 Focus Attack, 412
  Sand Bomb. 98 and 452 are auras the area rule would bar in the open world anyway.
- **Auras and areas past the rows:** 407 Psycho Symphony (SS; 5-minute reuse), 1417 Aura Flash,
  1171 Blazing Circle, 1174 Frost Wall, 1167 Poisonous Cloud, 1399 Mana Storm: area damage the
  area rule bars in the open world, the only place a break-off runs.
- **Debuffs and MP skills not proposed:** 116 Howl, 122 Hex (not control; the recast problem of
  M10), 1056 Cancellation (hit 6000, reuse 120 s), 1398 Mana Burn and 1396 Magical Backfire (BI,
  PP, EE, SE: a healer's casts go to heals), 1395 Erase (summons only;
  [summons](targets.md#summons-and-pets)).
- **DOTs:** 1233 Decay, 1183 Freezing Shackle, 1244 Freezing Flame stay out as in M10: the PvP DOT
  rule would stop the recast, but an HP DOT also bars the holding sleeps (WC's Dreaming Spirit,
  the healers' Trance) and adds nothing the nukes do not.
- **As M10:** 1159 Curse Death Link, 81 Punch of Doom, 222 Fist Fury, 313 Snipe; 314 Fatal Counter
  stays only in PR's M10 step.
- **400 Tribunal, 401 Judgment:** their TargetMe and debuff ride the M10 damage step; 28
  Aggression (hit 1000, range 400) stays the peel.
- **Cubics** (TK, SK): deferred (#636); no row depends on them.
- **320 Wrath on DS and BH:** POLE only.

## Delivery

Milestone M13. Each issue is buildable from its own text; X20 first, then the groups in any order.

| PR | Issue | Scope | Depends on |
| --- | --- | --- | --- |
| X20 | #718 | Row table foundation: the 24 row ids in `charmodel` (`classRows`, `rowArchetypes`, the bow pairing TH-HK, PW-SR, AW-PR), `RowClasses` for tier 2, `rowClass` admitting second professions; no list changes (O4). | #332 (shipped) |
| X21 | #719 | Tanks PL, TK, SK (O1: Shackle, Arrest). | X20 |
| X22 | #720 | Songs SS, BD (O1: Arrest). | X20 |
| X23 | #721 | Heavy damage GL, WL, DS, BH (O2: Wrath). | X20 |
| X24 | #722 | Daggers PW, AW; TH as RO; no Lure on players. | X20 |
| X25 | #723 | Archers SR, PR; HK as RO. | X20 |
| X26 | #724 | Tyrant TY. | X20 |
| X27 | #725 | Nukers SN, SG, SH, WC (O1: Silence). | X20 |
| X28 | #726 | Healers BI, EE (O1: Trance); PP as CL, SE as SO. | X20 |

Every group PR follows these conventions: a row is a whole list set as its kit's `PvP` (the first
profession's list var stays for the first profession); a new skill a list casts is in the kit's
`Skills`; each PR states the behavior it changes (D24); unit tests and recorded frames, no live
scenario (X13 #420 holds the live PvP scenarios).

## Tests

- `TestPvPRowsShipped` (X20 adds level 40 and 74 cases for the 24 rows with today's lists; each
  group updates its rows' cases and adds the level where a row's new step starts: 55, 56, 58, 66).
- `TestClassRow` (X20): each second profession to its own row, a third profession to its second's,
  the bow pairing, a servitor line (Dark Avenger 6) still to KN.
- One `TestPvPRow<ID>` per row with a list of its own (X21-X28): its opener against the named
  archetype, its `Hold` and `Int` step, its `Break` step, and the M10 attack it no longer outranks.
- `TestPvPControlLandRate` (X23): Hammer Crush against an Orc Fighter (33 %) skipped, against a
  Human Mystic (50 %) cast.
