# The PvP layer over the M10 rotation

Ships in X6 (the hit-time predictor in [Interrupts](#interrupts) in X4b, for the arena edge; CP,
cures, break-off and Ultimate Defense in X6b).
Against a player the bot runs its M10 row (same `Step` list, gates, `Melee` policy) with its
[PvP row](rows.md) on top. Land rates, archetypes and the engage estimate: [model](model.md).

## Step fields

New `Step` fields: `Vs []Archetype` (cast only against those archetypes; empty for any); `PvP` (only
against a player: Lionheart, Mental Shield, Aggression as a peel); `Interrupt` (only on an
interrupt-worthy cast landing later than this step would); `Hold` (kept ready, not cast at reuse,
while the opponent is a caster or healer; against melee and archers cast at reuse); `Break` (only
while breaking off); `Peel` (only on an enemy attacking the bot's healer or leader,
[party](party.md)).

## PvP order

Each group under the M10 gates:

1. sustain under its HP gate, with the **long-cast rule**: a heal or HOT of template hit time 4000
   ms or more on itself or its party (Self Heal, Elemental Heal, Divine Heal, Heal, 1229 Chant of
   Life: HOT, PARTY, `hitTime` 7000, `skills/1200-1299.xml`, about 9 s at an Orc Shaman's cast
   ×0.78, 6.3 s with a spiritshot) only while no melee opponent is within its attack range plus
   150, or the opponent is stunned, asleep or rooted (`CharInfo` mask);
2. [interrupts](#interrupts);
3. control steps on a fresh opponent and again when the effect wore off (mask cleared or datapack
   duration passed), at a land rate of at least `MinLand` (35 %); no sleep or root on an opponent
   seen to resist that skill twice this fight (139);
4. the opener, then damage steps, finishers first once the pool estimate is under their expected
   damage;
5. the `Melee` policy.

**Sleep and damage.** Every hit and DOT tick ends sleep (`character_stats.go:858-869`,
`applyNonConsumptionDamageEffects`: `StopByType(effect.TypeSleep)`); root and fear stay. A holding
sleep (1097 Dreaming Spirit, 1069 Sleep, as `Break` or party control) goes only on an opponent with
no HP DOT (mask bits bleeding 0x1, poison 0x2, flame 0x4000 clear: Bleed 96, Sting 223, Poison 129,
Curse: Poison 1168, Venom 1095 set `bleeding` or `poison`, Chill Flame 1100 and Frost Flame 1107
`flame`, `model/skill/abnormal.go:32-33`, `:46`; and no DOT of its own landed within that DOT's
ticks × period: Venom 3 s × 10, Chill and Frost Flame 15 ticks, `skills/1000-1099.xml` 1095,
`skills/1100-1199.xml` 1100, 1107), and that nobody will hit while it lasts (a `Break` sleep is
followed by the flee; in a party a target no member hits). Else the row's root or fear `Break`
(1092 Fear: 10 ticks of 2 s, `skills/1000-1099.xml:1672`), or none. An `Int` sleep is exempt.

**Kite.** A caster (mystic rows, bow branches) whose root or slow landed on a melee opponent (Dryad
Root, Seal of Binding, Ice Bolt, Slow; Entangle on the ES bow branch) walks to a stand point at its
cast range (M7 search, checked segments; in a duel passing [duel ground](finding.md#duel-ground))
before its next cast and keeps the distance while the effect lasts. Melee slows (Freezing Strike
on PK, Cripple on MO, Entangle on EK) do not kite. No kiting of monsters.

## Interrupts

An opponent's `MagicSkillUse` is **interrupt-worthy** when its type is HEAL, HOT, HEAL_PERCENT,
RESURRECT (aimed at anyone), or SLEEP, ROOT, FEAR, MUTE, or MDAM with template hit time 3000 or
more (aimed at the bot or the bot's party); it lands at arrival + `HitTime`. The bot answers
with its `Interrupt` step when that step's scaled hit time plus the
[reaction delay](wire.md#changes-to-todays-code-x4) and the walk to range end before the landing (a stun or sleep), or at least 200 ms before it on an `isMagic` cast (a
damage step: damage breaks only a magic cast inside its interrupt window). Its own time as the
server computes it ([cast time](server-casts.md)): template time × 333 / cast speed (magic) or
attack speed (`UserInfo`), truncated; × 0.70 for an `isMagic` step while a spiritshot is expected
charged (auto-use reported on, `Inventory.AutoShots` from `ExAutoSoulShot`, `items.go:178-181`,
`:224-227`, and a stack carried); floored at 500 ms for a template time of 500 or more. A short
prediction costs one late interrupt; the arena-edge rule re-checks against its own `MagicSkillUse`.

- A landed stun (Shield Stun, Stun Attack, Stunning Fist, Stunning Shot) or sleep aborts the cast.
- A damage step rolls `15 + sqrt(13 × damage) − MEN offset` on the expected damage, used only at
  `MinBreak` (30 %) or more: about 280-345 damage against a mystic's −45 to −52 (MEN 37: 277, 39:
  305, 42: 345), 140-165 against a fighter's −28 to −31 (`skill/formulas/cast.go:13-14`). A level
  7-39 Ice Bolt (power 8-16, `skills/1100-1199.xml` 1184) is far below, so the base and healer
  mystic rows have no interrupt. No damage and no stun (Wind Shackle, Curse: Weakness): never.

## CP, cures, pool, break off

- **CP.** CP potion (5592, then 5591) under `CPPotion` (50 %) CP in a PvP fight other than a duel,
  under the 500 ms reuse; HP potions, heals and drains follow M10's `ruleCritical`. CP potions are
  kept (M8 keep hook: never sold, deposited or used on monsters) and not drunk in a duel (none are
  bought, and an uncancelled duel end restores CP, [duel end](server-duels.md)).
- **Cures.** In a PvP fight, duels too (cures are cheap and a DOT decides a 2-minute duel): its own `AbnormalStatusUpdate` shows
  a poison or bleed (stack type `poison`, `bleeding`, or effect type POISON, BLEED) with half its
  ticks left → the cure. Its level is the lower set level (`hooks_buff.go:221-233`,
  `negateLevelAllows`): `effectAbnormalLvl` (only with an `effectType`; Sting 223 sets only this) or
  `abnormalLvl` (Poison, Curse: Poison, Venom, Bleed); neither set → no cure. Up to 3: Antidote 1831
  or Bandage 1833; 4-7: Greater Antidote 1832 or Emergency Dressing 1834 (plain first when both
  work); 8: none. One `UseItem` per effect seen, 500 ms reuse. None against monsters.
- **Pool estimate.** The opponent record's vitals: HP and max HP from the selection `StatusUpdate`
  or the latest `ExDuelUpdateUserInfo` (resent only when a gauge leaves its segment), CP and max CP
  from the latest `CharInfo` or duel window; each 35 since that read comes off CP, then HP. It drives finishers and the break-off, never a re-selection.
- **Break off** (open world only): CP gone, HP under the row's `PvPBreakHP` (tank 20, normal 30,
  glass cannon 40 %), no heal or potion ready, its pool fraction under the opponent's → its ready
  `Break` step (root, stun, fear, slow, Dash, a sleep under the rule above), then M10's flee
  (`FleeOutrun` against the opponent's `CharInfo` run speed). Unable to outrun: fights on.
- **Break-cast gate.** A `Break` step only when its scaled hit time is under 4 s, the opponent is
  stunned, asleep or rooted, a melee opponent cannot reach it first ((distance − attack range) /
  run speed > hit time), or the chance its hits break the cast (the cast-break chance with the bot's
  MEN over the expected hits) is under 50 %; else flee at once. No row breaks off with Sleep 1069
  (10 s).

### Ultimate Defense

110 (KN, EK, PK, `S<20 vs M`) locks the caster 30 s, and its gate falls where the break-off starts:
in the open world it is cast only when the bot will not break off (conditions not all met, or it
cannot outrun), in a duel or arena at its gate. While on (`AbnormalStatusUpdate`, 110) no walk
starts (flee, kite, chase, stand point, pole reposition, walk out of an arena); the
[guards'](guards.md) disengage move still goes after the Esc: a move from a player that cannot move
(`MovementDisabled`, counting `ImobileBuff`) idles it, dropping every intention, with `ActionFailed`
(`network/movement.go:87-91`, `startLiveMove`; `network/live_player.go:992-1001`, `tryToIdle`,
`:1024-1038`, `goIdle`; `model/actor/player/character_attack.go:306-313`;
`skill/effect/core.go:245`). A swing under way still lands. An arena leave walks out once it ends.

## Tests

On test-built `Step` lists; a case naming a shipped row runs in that row's PR (X7, X8); supply-list
and junk-rule cases in X10.

- `TestPvPInterruptHeal` (X6; KN in X7, a `support` bot fighting back): a KN with a shield against
  a Cleric casting 1011 Heal (hit 5000) on itself → Shield Stun before it lands; not at reuse
  against the Cleric, at reuse against a Warrior.
- `TestPvPInterruptTooLate` (X6): a cast landing before its own hit time plus delay → none; a damage
  interrupt under `MinBreak`, landing in the cast's last 200 ms, or on a non-magic cast → none; a
  test-built magic sleep (hit 4000) at cast speed 260 against a cast landing 4.5 s after its
  `MagicSkillUse`: no auto-use → none (5.1 s); auto-use and a stack → the interrupt (3.6 s plus at
  most 0.6 s); auto-use, no stack → none.
- `TestPvPLongCastRule` (X6; EF in X7): EF under 50 % HP, a melee opponent in reach → no Elemental
  Heal; the opponent's mask shows a stun → Elemental Heal.
- `TestPvPControlLandRate` (X6; Stun Attack in X7, Dryad Root in X8): Stun Attack vs an Orc Fighter
  (33 %) under `MinLand` 35 % → skipped; vs a Human Mystic (50 %) → cast; two 139s on CL's Dryad
  Root → no third.
- `TestPvPKite` (X6; CL in X8, a `support` bot fighting back): CL's Dryad Root lands on a melee
  opponent → a walk to a stand point at 600, then the next cast; in a duel with points at 600 in or
  within 100 of a town polygon → skipped, the first passing one; none passes → no walk, the next
  cast in place; a kiting caster whose duel opponent is 1900 away → `Action` (the chase), no kite.
- `TestPvPCPPotion` (X6b; the junk set in X10): CP under 50 % in a PvP fight with 5592 → `UseItem`;
  PvE → none; a duel → none, but a Poison 129 in it → the cure; CP potions not junk.
- `TestPvPCure` (X6b; the supply list in X10): Poison 129 at skill level 1 (`abnormalLvl` 3), most
  ticks left → 1831, at skill level 3 (6) → 1832; Bleed `abnormalLvl` 5 → 1834, 8 → none; Sting 223
  at skill level 1 (`effectAbnormalLvl` 3) → 1833, 20 (level 5) → 1834, 45 (level 8) → none; a test
  poison with neither level → none; vs a monster → none; the `pvp` supply list holds 1831 and 1833,
  from level 20 also 1832 and 1834.
- `TestPvPBreakOff` (X6b; OM in X8, Ultimate Defense in X7): open world, CP gone, HP under the bound,
  losing, faster → the `Break` step, then flee; duel or arena → fights on; an OM whose melee
  opponent would break Dreaming Spirit more often than not → no `Break`, flees at once; a test sleep
  `Break` vs a poison mask bit, or 5 s after its own DOT landed with ticks left → the fear (OM:
  Venom on the opponent → 1092 Fear; below 14 → flees at once); no DOT → the sleep; a KN
  (`support`), CP gone, HP under 20 %, losing, able to outrun → flees, no Ultimate Defense; unable,
  or in a duel → Ultimate Defense; under it → no flee, kite or stand-point walk, but its karma-free
  opponent's flag blinking with a damage cast in flight → `RequestTargetCancel`, the move (its
  `ActionFailed` changes nothing), no damage step; in an arena the opponent nearing the edge → the
  same; `ArenaStay` → the stop move and one interval, the walk out after `AbnormalStatusUpdate`
  drops 110.
