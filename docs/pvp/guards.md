# No accidental karma

Ships in X4b ([duel support](#duel-support) in X4). Decision D39. A karma-free opponent whose flag
ended, or who left an arena, dies to the bot's direct hit as a PK (its hits never refresh that
flag); a player's DOT never kills.

**Damage steps**: direct damage (`IsDamage`: PDAM, MDAM, BLOW, DRAIN, CHARGEDAM, ...), any
`nextActionAttack` skill, swings. Pure DOTs and non-damage control (Poison 129, Curse: Poison 1168,
Dryad Root 1201, Wind Shackle 1206, Power Break 115) always go; Ice Bolt 1184 and Freezing Strike
105 are MDAM, so damage steps.

**Stop**: start no damage step on that opponent; Esc-cancel the bot's own damage step in flight
(`RequestTargetCancel`, `unselect` 0; honoured until 200 ms before the hit); send a disengage move
(today's `disengage`), dropping the auto-attack and the follow-up swing. A cast past the window
lands within about 200 ms plus latency, a swing under way within one attack interval.

Against a karma player any step goes (never a PK). PK mode lifts the guards for its one victim
([karma](karma.md#pk-mode)).

## Flag guard

Outside duels, arenas and PvP zones, against a flagged karma-free opponent:

- its flag blinks (state 2, 4-5 s before the end) → stop;
- **late Esc.** The window is timed from the bot's own `MagicSkillUse` arrival, so an Esc can land
  after the cast ended; the server then clears the target (`ActionFailed`, `TargetUnselected`, world
  `Target` 0: `network/targeting.go:898-915`; bot `world.go`, `case TargetUnselected`). Expected:
  before its next step on that opponent (a DOT, a root) the bot re-selects it with one `Action`
  (only a selection on a non-target, with a fresh `StatusUpdate`), never a second one (it would
  attack). The disengage move drops the intention the late Esc left;
- **follow-up swing.** A `nextActionAttack` cast that ends or is Esc-cancelled on a player the bot
  may attack without force (blinking included) makes the server swing at or walk to it (carriers:
  [server](server.md#server-behavior), Attack after a cast). None starts after the blink, Shield
  Stun and Bleed included; for one in flight the disengage move is the first send after it ends
  (`MagicSkillCanceled`, or its landing time);
- the flag back to state 1 → attacks resume; the flag gone → no attack.

## Arena edge

Arena hits raise no flag and an arena kill is free only with both in a PvP zone at the death
(`character_karma.go:29-34`, `character_pvpflag.go:131-133`), so an opponent who steps out (or the
bot stepping out) makes the kill a PK, with no blink, unless it still carries a flag from outside,
which the flag guard then covers. Against a karma-free arena opponent:

- a damage step starts only while the opponent's edge distance exceeds its run speed × (**free
  time** + `EdgeSlack` 0.5 s). Free time = time to landing (the predicted scaled hit time,
  [interrupts](layer.md#interrupts), or the attack interval for a swing; once its own
  `MagicSkillUse` arrives, that hit time) minus the time a root the bot landed still **holds** it,
  floored at 0. A root holds from a landing with no 139 and the 0x200 bit in the `CharInfo` mask,
  for its datapack duration (Dryad Root 1201, Seal of Binding 1208); a `CharInfo` clearing the bit
  ends it. Stun and sleep do not count (a hit ends a stun one time in ten and wakes a sleeper, and the arena's other players may hit it). The
  bot's own edge distance must exceed `ArenaEdge` (150);
- it stops, moving to a point at least `ArenaEdge` inside, when the opponent's edge distance falls
  under run speed × (attack interval + `EdgeSlack`), it is seen outside, a counted root ends with
  the step in flight and the distance is under run speed × (time to landing + `EdgeSlack`), or its
  own `MagicSkillUse` carries a longer hit time than predicted (no spiritshot) and the distance is
  under run speed × (time left + `EdgeSlack`); attacks resume once the opponent is back inside by
  the margin;
- its own moves (chase, kite, stand points) stay `ArenaEdge` inside; to leave (`ArenaAlone`,
  `ArenaStay`, `ArenaDeaths`) it stops, waits one attack interval after the move, then walks out;
- **cost.** Opponents run 121-144 and no point of Dion, Gludin or Giran is more than about 490
  from an edge, so a step starts only under about 2.9-3.5 s at the middle (490 / speed − 0.5).
  With the [long hit times](server.md#server-behavior) and a spiritshot, the Human and Elven
  Mystics start the 4000 ms nukes and drains near the middle, the Dark only at the middle against
  slower opponents, the Orc never; Ice Bolt starts; Solar and Shadow Spark only for the fastest
  casters at the very middle. Shot-less 4-5 s steps, Drain Health (fighter rows charge soulshots),
  Power Shot and Stunning Shot almost never start unless rooted first. So
  bow rows and the Orc Mystic fight square arenas with roots (CL, EO, SO, OS), DOTs, non-damage
  control, short steps (Energy Bolt 2 s, Aura Burn 1.5 s), plain shots and melee; the RO, ES and AS
  Stunning Shot `Hold` and `Int` steps wait with Power Shot. The Colosseum (up to about 1370 from
  an edge, about 9 s) lets every step start near its middle.

Left over: a Dash, Sprint or root cure taking the opponent out in a cast's last 200 ms. Under
[Ultimate Defense](layer.md#ultimate-defense) stop moves still go; only walks wait.

## Area hits and bystanders

An area skill or pole swing also hits everyone else in reach the bot may attack without force:
anyone flagged (blinking included) or with karma outside its party, clan and ally.

- **Reach**: the skill's radius around its anchor (target for AREA, bot for AURA), or for a pole
  the bot's attack range (spear 80) inside the 120° cone it faces (`attack/controller.go:367-375`);
  in reach when the 3D distance from the anchor is at most reach + both decoded collision radii
  (`world/visibility.go:837-862`, `rangeBody3D`), never a fixed figure: Wild Sweep (`skillRadius`
  150) reaches 160-172 from its target (168 between male humans), a spear 90-102 from the bot (98).
  With male humans a bystander 165 beyond the target is in Wild Sweep's reach (150 + 9 + 9,
  `castRange` 40), not the pole's.
- **Open world**: no area or aura damage skill (Wild Sweep 245, AREA, PDAM, `skills/0200-0299.xml`;
  any `IsDamage` skill with an AREA, AURA or cone target) and no pole swing while a flagged
  karma-free player other than the target, or a summon or pet showing a flag and no karma (its
  owner's, kept current by its `RelationChanged`; its kill after the flag ends gives karma,
  `character_karma.go:84-87`), is in reach: that bystander's flag may end before the hit and its
  death would be a PK (`character_karma.go:45-55`). Summons are not linked to owners, so the
  target's and a flagged party or clan mate's summon count too; a summoner next to its summon gets
  single-target steps.
- **Weapons**: off a pole, single-target steps (WA, OR: 255 Power Smash, 100 Stun Attack, M10
  gates SWORD, BLUNT, BIGBLUNT, BIGSWORD and BLUNT, BIGBLUNT, `docs/strategy.md:293`, `:299`; 255's
  `weaponsAllowed` in `skills/0200-0299.xml`; Warlords wield the pole, `:262`). **A pole has no
  single-target step:** a `nextActionAttack` cast ends in a pole swing (`attackAfterCast`), and 317
  Focus Attack (`PolearmTargetSingle`, `skills/0300-0399.xml:508`; `character_attack.go:620-627`)
  comes at 40 (`classes/humanFighter.xml:1289`). So with a bystander in the pole's reach the bot
  sends its disengage move (`combat_rules.go:324`) and **repositions** to a stand point in range
  with no bystander in the cone toward the target (the server turns to it before each swing,
  `attack/controller.go:334`), trying 30° steps around the target from the nearest, each held on
  the geodata and, in a duel, on [duel ground](finding.md#duel-ground); with none it **holds** (self
  buffs only). A swing under way lands within one attack interval, before a 4-5 s blink can end.
- **Arena**: the same for any karma-free player other than the target in reach and not at least
  `ArenaEdge` inside, flagged or not (Wild Sweep on a target anywhere reaches 160-172 past it; a
  flagged player outside is hit, flags the bot, `character_pvpflag.go:131-133`, and dies as a PK if
  its flag ended, `character_karma.go:29-34`; one near the edge may step out after the launch fixes
  the hit set, `network/magic_skill.go:287-302`, `launchCastTargets`), and for a summon whose owner
  (named by its title) is not seen inside by `ArenaEdge`. The target falls under
  [Arena edge](#arena-edge).
- **Duel**: from the countdown to the end, no offensive area or aura skill (Wild Sweep, Seal of
  Binding 1208, AURA) and no pole swing while any player but the opponent, any summon or pet, or any
  attackable NPC is in reach (radii included): a flag cancels the duel (`duel/duel.go:206-213`), so
  does a hit on an NPC or on a summon whose owner is not in it (`target_predicates.go:22`,
  `:33-37`, `attack/controller.go:370-381`; `creature/duel.go:43-50`). Every summon counts (duel
  team unread). A pole wielder repositions or holds; a swing under way still lands.

## Duel support

From the countdown to the end `SupportFlagged` counts as off through the new `MaySupport`
([wire](wire.md#changes-to-todays-code-x4)): supporting a flagged or karma player flags the bot
(`character_pvpflag.go:88-93`), which cancels the duel (`duel/duel.go:205-207`). So no beneficial
skill on such a player but itself, and no party skill (Chant of Life 1229, Chant of Fire 1006) while
such a member is in its radius (`character_pvpflag.go:65-95`). Nor on its duel opponent, a clean
party member included (one-on-one duels check no party, `network/duel.go:31-62`): `MaySupport`
refuses it, and a party skill waits while it is a member in the radius.

## Tests

- `TestPvPNoKarmaFlagEnding` (X4b): Wind Strike (hit 4000) at a flagged karma-free opponent whose
  `RelationChanged` turns to 2 → `RequestTargetCancel` (`unselect` 0), then no damage cast,
  `AttackRequest` or `Action`, a move; a DOT or root allowed; back to 1 → attacks; gone → none; the
  same blink on a karma opponent, in a duel or an arena → fight goes on; first seen in a `CharInfo`
  with flag 2 → no damage attack. On test-built `nextActionAttack` steps (PDAM, STUN, BLEED): none
  started after the blink; one in the window → cancel, then on `MagicSkillCanceled` a move as the
  next send; one past it → no cancel, a move first once its hit time passed; a pure DOT → cast, no
  move. Late Esc (`ActionFailed`, `TargetUnselected`) → the move, one selecting `Action` before the
  next DOT, never a second.
- `TestPvPArenaAttackable` (X4b): both in the Gludin polygon, stale `RelationChanged` → plain
  attack; the player walks out → disengage; opponent 100 from the edge, speed 120 → no Wind
  Strike, a DOT cast; a Human Mystic (cast 333) without spiritshot auto-use, opponent at the middle
  (about 490), speed 124 → no Wind Strike (124 × 4.5 = 558 > 490), a test-built 2000 ms damage step
  started; with blessed spiritshot auto-use (`ExAutoSoulShot`) and a stack → Wind Strike (124 × 3.3
  ≈ 409 < 490), at 380 → not; that cast's `MagicSkillUse` carrying 4000 ms, 0.1 s after the send,
  opponent at 450 → cancel at once (450 < 124 × 4.4 ≈ 546); shot-less, opponent under its Dryad
  Root (no 139, bit 0x200, 20 s left) → Wind Strike; a `CharInfo` clearing the bit with 2.5 s to
  landing, opponent (124) at 300 (< 372) → cancel; 0.4 s left (300 > 112) → none; the opponent
  nears the edge in the window → cancel, a move at least 150 inside; back by the margin → attacks;
  a karma opponent at the edge → fight goes on; a clan mate or ally in the arena → attack; a party
  member → never; an unflagged player healing or buffing the bot → not in the attackers table,
  casting Wind Strike → in it.
- `TestPvPAreaBystander` (X4b on a test-built area step and pole; the WA case in X7). Radii 9 (male
  Human Fighters) unless named. A WA with a pole vs a flagged karma-free opponent, a second flagged
  karma-free player 40 from the bot in the cone → no Wild Sweep, swing or `nextActionAttack` step
  (an ungated test PDAM carrying it not cast): a move, a walk to a stand point with the bystander
  out of the cone, then `Action`; bystanders on both sides → no swing or damage step, War Cry cast,
  until one leaves; a BIGBLUNT → 255 and 100; the bystander 95 from the bot in the cone → no swing;
  the bot 40 from its target, the bystander 165 beyond it (205 from the bot) → no Wild Sweep, swing
  allowed; at 175 → allowed; a male Orc Fighter (11) at 169 → no; a female Dwarven Fighter (5) at
  165 → allowed; the bystander gone or with karma → allowed; a summon (radius 9) flag 1, karma 0
  at 165 → no, then karma → allowed, flag 0 and karma 0 → allowed; in an arena, a summon whose
  owner is seen outside → no, owner inside by more than 150 → allowed; target 160 inside and a
  flagged karma-free player 165 from it just outside → no; that player unflagged 100 inside → no,
  200 inside → allowed, with karma outside → allowed; in a duel, any third player, summon or
  attackable NPC in reach → no Wild Sweep or swing, an NPC of radius 30 at 115 in the cone → the
  reposition move; in an arena → allowed.
- `TestPvPDuelNoFlaggedSupport` (X4): `SupportFlagged` on, in a duel, a flagged member or an
  unflagged karma member under the heal gate → no heal, after `ExDuelEnd` → heal; a PARTY buff with
  such a member in range → not cast. Out of a duel, `SupportFlagged` off: an unflagged karma member
  → no heal; a clean member → heal; both in an arena → heal. Duelling a clean party member → no heal
  or buff on it, no PARTY skill while it is in the radius; after `ExDuelEnd` → heal.
- e2e `TestLivePvPArena` (X13): two `pvp` bots of fighter rows and a client fight in the Gludin
  arena (base mystic long nukes are held there); a bot's death leaves its experience unchanged and
  it walks back (GM, client, two bots).
