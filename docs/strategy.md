# Race and class strategy: each bot plays its race, class and role

Status: approved. The owner approved decisions D21-D24 on 2026-11-24 (see "Owner decisions").
Milestone M10 ("Race and class strategy"): epic #319, issues #320-#333 ("Delivery").

Nine rounds of independent review, every finding checked against the datapack, the server or the
bot code before it was fixed; the ninth found one wording contradiction (fixed) and two minor
points (fixed).

- **Round 1:** NPC stats are bases; migration of existing bots; Support role and default preset;
  toggles drain MP and cannot be cast seated; spoil message semantics; Overlord has no summons.
- **Round 2:** Hurricane Assault needs two charges; inherited skills' levels; Accuracy drains MP;
  imported bots' preset; thresholds against the CON rule.
- **Round 3:** gates the `Step` could not express; openers missing from rotations; sustain against
  `ruleCritical`'s crowded potion; branch and pin ordering between S5, S8 and S9.
- **Round 4:** the level and prefix notation as a test reads it; buff and heal gates; named rule
  changes; roles before S8; S6 split into S6 and S6b; the shield gate.
- **Round 5:** every rule-body change named; `town` keeps today's role; gated self buffs as steps.
- **Round 6:** stack partners (Frenzy and Guts, the Monk's totems); group heals first; toggles
  versus steps; heal tables for the base mystics.
- **Round 7:** the stack rule for party buffs (`stackOrder`); a `Melee` policy on every caster.
- **Round 8:** `RebuffBefore` renews only the skill's own effect; enemy sustain keeps the opener.
- **Round 9:** a weaker effect of another skill is upgraded; per-role melee for the orc mystic.

Server paths (`internal/gameserver/...`) refer to
[fatal10110/acis_golang](https://github.com/fatal10110/acis_golang). Datapack paths
(`classes/...`, `skills/...`, `armorSets.xml`) refer to `aCis_datapack/data/xml`. Bot paths name
this repository.

## Problem

A real player picks a race and a type for what they give: an orc fighter takes hits a dark elf
fighter cannot, a dark elven mystic nukes harder than any other caster, an elf outruns what it
cannot fight, a dwarf spoils and carries. Each class has its own skills, and the race decides
which armor and items pay off. The bots ignore almost all of it.

### What the bot does today

| Concern | Today | Where |
| --- | --- | --- |
| Skills learned | Only the four mystic base classes and the support build's healer professions have a learn table. Fighters learn nothing. | `trainer.go` `mysticSkills`, `mysticTrainers` |
| Skills cast | One shared list for every class: Wind Strike, Chill Flame, Life Drain, cast first ready, before melee, above `MPReserve` 50. No weapon check. | `support.go` `defaultAttackSkills`, `combat_rules.go` `fight` |
| Heals and buffs | One shared list (Heal, Battle Heal, Might, Shield, Soul Shield, Chant of Battle); a class casts the ones it knows. | `support.go` `defaultHeals`, `defaultBuffs` |
| Profession | One table: every mystic takes the healer line, every fighter one melee line. | `classchange.go` `supportBuilds` |
| Gear | One class table: weapon kinds, armor kind, shield, and stat weights over pAtk, mAtk, pDef, mDef, attack speed and shield defense. Every mystic, orc included, wears robes. Sets add a flat 15 % of their pieces' score. | `gear.go` `defaultGearClasses`, `StatWeights`; `gear_sets.go` `setBonus` |
| Dyes | One table per profession. | `dyes.go` `defaultDyeTable` |
| Thresholds | One set for every class: flee, rest, MP reserve, outmatched. | `rules.go` `DefaultPersona` |
| Spot | Fit by monster level bounds and pack size only; a death teaches the pack (#281). | `spots.go` `SpotPolicy.fits` |
| Own stats | `UserInfo` carries race, the six base stats and the eight combat stats; the decoder skips them. | `inworld.go` `decodeUserInfoEvent` |

So a fighter, five of the nine starting classes, only auto-attacks; every mystic plays a healer,
even alone; race shows only where it is free (run speed in `FleeOutrun`, the newbie zone).

## Goal

Every bot plays a **build**: what its race, its class and its persona's role make of it.

- It learns and casts its class's skills, gated by the weapon it wields, its HP and MP and its
  target's HP.
- It wears what pays off for its race and class: armor kinds its masteries allow, sets whose
  stat trades suit it, weapons its skills need.
- It takes a profession that fits its race and its role, varied across bots of one race.
- Its thresholds (flee, rest, MP reserve) and its spot choice follow how sturdy and how fast it is.
- One character model, fed by the bot's own reported stats, scores gear, sets, dyes and spots
  alike, so race counts through numbers rather than hand-written exceptions.

Out of scope: servitor professions (Warlock, Elemental Summoner, Phantom Summoner, and
Necromancer for its summons), the Overlord (no branch picks it: its seals and clan buffs serve a
group), and kiting. Second-profession kits are planned in "Second professions (levels 40+)" and ship
as S13-S17 (#587-#591); servitor lines are #592 (a later milestone).

## Race and class data

### Base stats and what they buy

The server turns each base stat into a multiplier (`skill/statbonus/statbonus.go`: a curve
`base^(stat - offset)`, rounded to hundredths): STR for P.Atk, CON for max HP and CP, HP and CP
regen and the weight limit, DEX for attack speed, critical rate, evasion and accuracy (through
`sqrt(DEX) * 6`) and run speed, INT for M.Atk (squared, `skill/funcs/attack.go` `MAtkMod`), WIT
for cast speed and magic critical rate, MEN for M.Def, max MP and MP regen. With the starting
stats of each class (`classes/*.xml`):

| Class | STR CON DEX INT WIT MEN | P.Atk | HP, weight | DEX | M.Atk | Cast | MP, M.Def | Run (base × DEX) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Human Fighter | 40 43 30 21 11 25 | 1.20 | 1.58 | 1.10 | | | 1.28 | 115 → 127 |
| Elven Fighter | 36 36 35 23 14 26 | **1.04** | 1.28 | **1.15** | | | 1.30 | 125 → **144** |
| Dark Fighter | 41 32 34 25 12 26 | **1.24** | **1.14** | 1.14 | | | 1.30 | 122 → 139 |
| Orc Fighter | 40 47 26 18 12 27 | 1.20 | **1.77** | 1.06 | | | 1.31 | 117 → 124 |
| Dwarven Fighter | 39 45 29 20 10 27 | 1.16 | 1.67 | 1.09 | | | 1.31 | 115 → 125 |
| Human Mystic | 22 27 21 41 20 39 | 0.63 | 0.98 | 1.01 | 1.46 | 1.00 | 1.48 | 120 → 121 |
| Elven Mystic | 21 25 24 37 23 40 | 0.61 | 0.93 | 1.04 | 1.25 | **1.16** | 1.49 | 122 → 127 |
| Dark Mystic | 23 24 23 44 19 37 | 0.66 | 0.90 | 1.03 | **1.64** | 0.95 | 1.45 | 122 → 126 |
| Orc Mystic | 27 31 24 31 15 42 | 0.76 | 1.10 | 1.04 | **0.98** | 0.78 | **1.52** | 121 → 126 |

The curves are exponential, so +1 of a stat is the same percentage for every race (about 3.6 %
P.Atk per STR). Race changes what an item or a dye is worth through:

- **flat bonuses on small bases:** a set's +294 or −270 max HP is a third of a low-CON caster's HP
  at 40 and a fifth of an orc fighter's;
- **concave terms:** accuracy and evasion grow with `sqrt(DEX)`, so a DEX point is worth more to
  an orc than to an elf;
- **role and kit:** a STR set is worth nothing to a nuker, an INT set nothing to a fighter;
- **masteries and penalties** (next section).

### Racial kit (levels 1-19, `classes/*.xml`, `skills/*.xml`)

| Class | Kit beyond the shared basics | Weapon gate (`weaponsAllowed`) |
| --- | --- | --- |
| Human Fighter | Power Strike (3), Mortal Blow (16), Power Shot (56), Relax (226) | 3: SWORD, BLUNT, BIGSWORD, BIGBLUNT; 16: DAGGER; 56: BOW |
| Elven Fighter | Power Strike, Mortal Blow, Power Shot; **Elemental Heal** (58, self heal, 15); Attack Aura (77), Defense Aura (91) | as above |
| Dark Fighter | Power Strike, Mortal Blow, Power Shot; **Drain Health** (70, range 600, 15); auras; Shadow Sense (294, +3 accuracy at night) | as above |
| Orc Fighter | Power Strike, **Iron Punch** (29, fists); Toughness (134), Iron Body (295) passives; Relax | 29: DUALFIST |
| Dwarven Fighter | **Spoil** (254) and **Sweeper** (42) from 10; Weight Limit (150, weight limit ×2 at 10) | none |
| Human Mystic | Wind Strike, Ice Bolt, Vampiric Touch, Curse: Poison, Curse: Weakness, Heal, Battle Heal, Self Heal, Might, Shield | none |
| Elven Mystic | Wind Strike, Ice Bolt, Wind Shackle, Curse: Weakness, Heal, Battle Heal, Group Heal, Self Heal, buffs | none |
| Dark Mystic | Wind Strike, Ice Bolt, Vampiric Touch, Curse: Poison, Wind Shackle, heals, buffs | none |
| Orc Mystic | Soul Cry (1001, toggle), Life Drain, Chill Flame, Venom, Fear, Dreaming Spirit, Soul Shield, Chant of Battle | none |

### Armor kinds by race

- **Human, elven and dark elven mystics wear robes.** Spellcraft (163) halves cast speed and
  Magician's Movement (118) cuts attack speed to 0.8 whenever the bot is not `using kind="MAGIC"`.
- **Orc mystics may wear armor at little cost.** Light Armor Mastery (252, from 7) multiplies
  cast speed by 1.9 and attack speed by 1.25 in light armor (net 0.95 and 1.0) and adds MP regen;
  Heavy Armor Mastery (253, from 14) gives 1.71 and 1.25 in heavy armor (net 0.855 and 1.0) and
  P.Def. A robe set can still cast faster (Devotion ×1.15), so the choice is the role's: casting
  in a robe, or standing in a fight in light or heavy armor (rows OM and OS below); Robe Mastery
  (251, from 7) adds P.Def in a robe too, so the robe is no weak choice. Today the orc mystic is
  held to robes by the mystic row of `defaultGearClasses`.
- **Fighters' Armor Mastery (142)** adds evasion only in light armor and only from its level 4
  (the first three levels add none), which the high-DEX races and the dagger and bow lines want.
- **Mystics' Weapon Mastery (249, 250)** cuts cast speed to 0.7 and accuracy by 9 with a bow or a
  pole.

### Armor sets trade base stats (`armorSets.xml`, set skills in `skills/*.xml`)

| Set | Effect | Fits |
| --- | --- | --- |
| Plated Leather | +4 STR, −1 CON | dark elven and human dagger users |
| Doom Heavy | +320 max HP, +3 CON, −3 STR | orc tank |
| Doom Light | +3 DEX, −1 STR, −2 CON, ×1.027 P.Atk | elven scout, archer |
| Blue Wolf Robe | +206 max MP, +3 WIT, −2 INT, −1 MEN | elven mystic (cast speed) |
| Doom Robe | +2 INT, +1 MEN, −3 WIT, +7 run | dark mystic (M.Atk) |
| Demon | +4 INT, −1 WIT, **−270 max HP** | a caster with HP to spare, not a low-CON one at 40 |
| Composite, Avadon Light | +5795 weight limit | dwarven looter, merchant |

The bot today values every set as 15 % of its pieces, so it cannot tell a Demon set from a Doom
Robe.

## Server behavior this rests on

Read in the Go server on 2026-11-23; every design choice below rests on these.

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Armor kind of a `using kind` condition | A chest piece counts its armor kind only while the legs are of the same kind, and the legs likewise; a full-body armor counts alone. Mixed or half-worn armor is of no kind, so a robe top over light legs is not `MAGIC` and triggers Spellcraft. | `skill/conditions/compile.go:61-62`, `conditions/skill.go:17-24`, `model/itemcontainer/inventory.go:751-755,790-802` |
| Wrong weapon for `weaponsAllowed` | `RequestMagicSkillUse` is answered SystemMessage 113 (S1 cannot be used) with no `ActionFailed`. The bot's `castRefused` already holds the skill 10 s on 113, so an ungated step would cost 10 s of that skill each try. | `model/actor/cast/conditions.go:112-118`, `network/magic_skill.go:380-395,867-868` |
| Blow land rate | `baseLandRate × 2 × DEX bonus × blow rate × position`, position 1.15 behind, 1.1 side, 1.0 front, capped at 800 per mille. Mortal Blow's base rate is 200. Position adds at most 15 %, so the bot does not walk behind a target. | `skill/formulas/damage.go:195-213`, `skills/0000-0099.xml:352` |
| Physical damage | Melee `pAtk × mods × 77 / pDef`, ×2 with a soulshot; skill `(pAtk × ss + power) × mods × 77 / pDef`, ss 2.04; critical ×2. | `skill/formulas/damage.go:71-79,148-153` |
| Magic damage | `91 × sqrt(mAtk × shot) / mDef × power`, shot 2 for a spiritshot and 4 for a blessed one; magic critical ×4. M.Atk counts by its square root. | `skill/formulas/damage.go:313-328` |
| Spoil | Success SystemMessage 612; 357 when the monster is already spoiled by anyone, the bot included, so a 357 on a monster the bot has no 612 for means another spoiler; 139 is the generic "S1 resisted your S2", sent for any resisted skill, so it is matched to the Spoil cast in flight. The spoiler's id marks the monster; its spoil drops roll at death. | `handler/skill/spoil.go:27-69`, `model/item/spoil.go:55`, `model/item/reward.go:33-48` |
| Sweep window | A corpse decays after its template `corpseTime` (default 7 s), doubled for a spoiled one; the spoil flag also lifts the corpse-too-old check. | `model/actor/npc/hostile_rewards.go:121-133`, `handler/target/target_predicates.go:63-78` |
| Sweeper | Only the spoiler, its party or command channel may sweep (else 683; not spoiled: 343). Items follow the party loot rule, else go to the sweeper: SystemMessage 53 or 54 and `InventoryUpdate`. | `handler/target/cast_rejection.go:156-181`, `handler/skill/spoil.go:87-117`, `network/inventory.go:889-892` |
| Toggles | `AbnormalStatusUpdate` (0x7f), sent to the owner only, lists active toggles after the other effects with duration −1. | `network/targeting.go:1225-1240`, `serverpackets/status_effects.go:25-81` |
| Relax | Cast standing (seated: SystemMessage 31); the effect sits the player, and each 2 s tick ends it once the player stands, dies or reaches full HP (SystemMessage 175). | `player_gate.go:109-127`, `skill/effect/hooks_status.go:197-222` |
| `UserInfo` fields | STR, DEX, CON, INT, WIT, MEN after the experience; then, after the paperdoll, P.Atk, P.Atk speed, P.Def, evasion, accuracy, critical, M.Atk, cast speed, P.Atk speed again, M.Def, then PvP flag and karma. | `serverpackets/userinfo.go:165-170,201-213` |
| `UserInfo` resends | Every stat function change (equip, buff, level) resends `UserInfo`. | `model/actor/player/character_effects.go` `statsModified`, `network/movement.go` `sendModifiedStats` |
| NPC combat stats | `<set>` names `hp`, `mp`, `pAtk`, `pDef`, `mAtk`, `mDef`, `crit`, `atkSpd` (default 300), `walkSpd`, `runSpd`; no cast speed. They are **bases**: the NPC runs the same stat functions as a player (P.Atk by STR × level factor, max HP by CON, and so on) on its own `str`, `con`, `dex`, `int`, `wit`, `men` (defaults 40, 21, 30, 20, 43, 20) and level. | `data/xml/npc.go:290-311`, `model/actor/npc/hostile_stats.go:209-240`, `npcs/20000-20999.xml:14-28` |
| Trainers | `<teachTo classes="0;1;2;…"/>` child of the trainer's `<npc>`; Auron (30010) teaches 0-9. | `data/xml/npc.go:40,247-250`, `npcs/30000-30999.xml:452` |
| Dyes | At most +5 per base stat from dyes, 3 slots (0/2/3 by class level). | `model/henna/list.go:7-11,57-65,109-113` |

Nothing the design needs is missing on the server, so this plan files no server issue.

## Behavior per class

This section is the spec that S5 (gear and dyes, columns 1, 2 and 11), S6 and S6b (fighter rows), S7
(mystic rows), S8 (the professions, the branch weapon and the class change between rows), S10
(spoil and sweep) and S11 (spot margins, column 10) implement, and that the brain tests check row by row (section
"Testing"). It covers the nine base classes (levels 1-19) and every first profession (20-39) of
the branch table in Design section 6, for the Support and Damage roles. The servitor lines are out
(D23): Human, Elven and Dark Wizard keep their nukes here and never cast their summon skills.
The second professions (40+) have their own section at the end.

Every skill below is in its class's skill tree (`classes/*.xml`, merged with the parents' as the
server does: a profession keeps the highest level of a parent's skill it learned), from the level
given; every weapon gate is the skill's `weaponsAllowed` (`skills/*.xml`); every profession is a
direct child of its class (`model/actor/player/class.go` `classParent`). A row's race reasons come
from the stat and kit tables above. `TestBehaviorRowsShipped` (S4) checks the tables against the
shipped datapack.

**Notation.** Skills are `id Name`. A weapon gate is in brackets, as the datapack's weapon kinds:
`[DAGGER]` (`SHIELD` is the left-hand shield, which the server's held-item check counts too).
Levels, as `TestBehaviorRowsShipped` checks them against the line's merged tree (row start: 1 for
a base class, 20 for a profession, 40 for a second profession):

- `@24`: the step starts at 24, which is max(row start, the skill's first merged `minLvl`).
- `(base)` or `(base tree)`: the profession knows the skill from its base class (first merged
  `minLvl` under 20), whatever level its own tree lists.
- No level tag: known at the row's start (first merged `minLvl` ≤ row start).
- `(level 9)`, `(levels 5-6)`: the skill level the row reaches by then, not a gate.

Prefixes, in any letter case: `Dagger:` or `Bow:` gives every step after it, up to the next
prefix, a `[DAGGER]` or `[BOW]` gate (a branch gate, which must fit the skill's own
`weaponsAllowed` when it has one); `Damage:` or `Support:`, and a step marked (Damage) or
(Support), put the step in that role's rotation only; no mark is every role. `Heals:` lists the row's `Heal`
table, which `ruleHeal` runs before any rotation, not steps; a `Heal` entry with a member count
is tried before the single heals with the same HP bound. "Melee ..." at the end of a rotation is
the kit's `Melee` policy, compiled per role: always (fighters, and the armored orc mystics: OM
Support, OS), only under `MPReserve` (other casters), or for a healer only alone and under
`MPReserve`.

Column 5: a sustain skill cast on the enemy (Drain Health, Vampiric Touch, Life Drain) is a step
ranked first among the attacks while its `S<` gate passes; it does not use up the opener. A
sustain skill cast on the bot (Self Heal, Elemental Heal, Divine Heal, Battle Roar, Ultimate
Defense, Ultimate Evasion, Body To Mind) is a self-target step. `ruleCritical` tries only the heals
and drains among them, as Design section 7 lists them. One cast on the party (Chant of Life) is a `Heal` entry
with a member count; "its heals on itself" is the row's `Heal` table.

Column 6: a toggle skill ("combat toggle") follows Design section 9. Any other entry without a
gate is an out-of-combat `Buff` (`ruleRebuff`); one gated "in combat" or by HP (`S<30`) is a self-target `Step` at the head of the rotation. The stack rule (Design
section 4, "Buffs and heals") keeps any buff from recasting at its reuse or replacing a stack
partner. Both skills of a stack stay in the kit, in the row's order (Frenzy before Guts), unless
the row says "replaced by" (the Monk's Bear Spirit Totem from 28): only then does
`CompileBuild` drop the earlier one. "Only to flee" is the kit's `Flee` skill. Several `open` steps: the first whose gates pass is the
opener, and column 3 restates them. A step with no `≤` gate is cast from where the bot stands,
and the server walks it into the skill's `castRange` (a 600 opener from a 750 stand point).
Other gates: `open`
the opener (first attack on a fresh target; a self-target, `Idle` or enemy sustain step before it does not use it up), `T≥50` target HP at least 50 %, `S<60` own HP under
60 %, `MP>30` own MP above 30 %, `≤400` cast only while the target is within 400 (`Step.Within`), so a short skill never pulls the bot into melee, `A≥2` at least two
attackers, `C≥2` at least two force charges, `C<max` fewer than the skill's own `maxCharges` at
the level the bot knows (Design section 4, `Step`). Steps are in priority order;
"melee" means auto-attack between steps. Branch weapons: a base class or a profession whose
branches wield different weapons lists them all; the weapon gates pick the steps for the weapon the
branch wears (Design section 6), so one kit serves every branch. Sets name the pieces the server
checks (`armorSets.xml`): chest and legs as a pair, or a full-body chest.

**Row ids.** Base classes: HF Human Fighter, EF Elven Fighter, DF Dark Fighter, OF Orc Fighter,
WF Dwarven Fighter, HM Human Mystic, EM Elven Mystic, DM Dark Mystic, OM Orc Mystic. First
professions: KN Knight, WA Warrior, RO Rogue, EK Elven Knight, ES Elven Scout, PK Palus Knight,
AS Assassin, OR Orc Raider, MO Monk, SC Scavenger, CL Cleric, HW Human Wizard, EO Elven Oracle,
EW Elven Wizard, SO Shillien Oracle, DW Dark Wizard, OS Orc Shaman.

### Gear and range (columns 1, 2, 9, 11)

Grades by level: no grade 1-19, D from 20 (Expertise), C from 40 (second professions, their section below).
Dyes need a profession (0 slots on a base class, 2 on a first profession; the +5 cap per stat
binds only at 3 slots). The direction raises the row's main stat and pays with the stat it needs
least, the race's highest of those where it has a choice (`hennas.xml` symbols 1-36; 37000 rows
lose 3, 87000 lose 2, 149000 lose 1).

| Row | Class (id), role → branch | 1. Weapon kind, and why | 2. Armor pair; preferred sets by grade | 9. Fight range | 11. Dyes |
| --- | --- | --- | --- | --- | --- |
| HF | Human Fighter (0) | SWORD or BLUNT with a shield (Warrior, Knight branches); DAGGER (Rogue → Treasure Hunter); BOW (Rogue → Hawkeye). Balanced STR 40 and CON 43 carry a one-hander; DEX 30 is middling, so daggers and bows are a branch choice, not a race strength. | Light pair; no grade: Wooden Set (chest 23, legs 2386, head 43: ×1.02 P.Def, +41 HP). | Melee; bow 500, Power Shot 700 | none (base class) |
| EF | Elven Fighter (18) | SWORD with a shield (Elven Knight); DAGGER or BOW (Elven Scout → Plains Walker, Silver Ranger). DEX 35, the fighters' highest (critical rate, evasion, attack speed), and run 144 suit dagger and bow; STR 36 is the fighters' lowest, so P.Atk gear matters more. | Light pair; Wooden Set. | Melee; bow 500, Power Shot 700 | none |
| DF | Dark Fighter (31) | SWORD with a shield (Palus Knight); DAGGER or BOW (Assassin → Abyss Walker, Phantom Ranger). STR 41, the highest P.Atk (1.24), makes every hit count; CON 32 is the fighters' lowest. | Light pair; Wooden Set. | Melee; Drain Health 600; bow 500, Power Shot 700 | none |
| OF | Orc Fighter (44) | BLUNT or SWORD with a shield (Orc Raider branch); DUALFIST (Monk branch, for Iron Punch). CON 47, the highest HP and weight (1.77), carries a shield fighter; DEX 26 is the lowest, so no dagger or bow. | Light pair; Wooden Set. | Melee | none |
| WF | Dwarven Fighter (53) | BLUNT with a shield (Scavenger's Stun Attack is BLUNT or BIGBLUNT). CON 45 and STR 39: a sturdy melee body with no attack skill before 20. | Light pair; Wooden Set. | Melee (Spoil 40, Sweeper 20) | none |
| HM | Human Mystic (10) | BLUNT/MAGIC or SWORD/MAGIC (one-handed, magical). INT 41 (M.Atk 1.46); Weapon Mastery 249 cuts cast speed with a bow or a pole. | Robe pair (Spellcraft halves cast speed out of it); Devotion Set (chest 1101, legs 1104, head 44: ×1.15 cast speed). | Cast 600 | none |
| EM | Elven Mystic (25) | BLUNT/MAGIC or SWORD/MAGIC. WIT 23, the fastest cast (1.16); INT 37. | Robe pair; Devotion Set. | Cast 600 | none |
| DM | Dark Mystic (38) | BLUNT/MAGIC or SWORD/MAGIC. INT 44, the highest M.Atk (1.64). | Robe pair; Devotion Set. | Cast 600 | none |
| OM | Orc Mystic (49) | BLUNT/MAGIC (a mace casts and, from 20, carries Stun Attack). MEN 42 (MP 1.52), CON 31 (the sturdiest mystic, HP 1.10), INT 31 (M.Atk 0.98). | Robe pair (Devotion Set) for the Damage role; light pair from 7 (Light Armor Mastery: cast ×0.95 net), or heavy from 14 when it scores higher, with the Wooden Set for the Support role, which stands near its party's fights. | Cast 600 | none |
| KN | Knight (4), Support → Paladin | SWORD or BLUNT with a SHIELD (Shield Stun's gate is the shield). Human CON 43 tanks. | Heavy pair; D: Brigandine Set (chest 352, legs 2378, head 2411, shield 2493: ×1.05 P.Def, +153 HP). | Melee; Aggression 400; Drain Health 600 | CON+1 DEX− (4, 16, 28): heavy armor gets no evasion from DEX |
| WA | Warrior (1), Damage → Gladiator 2, Warlord 1 | BIGBLUNT (Power Smash and Stun Attack both take it) for the Gladiator branch; POLE (Wild Sweep) for the Warlord branch. Human STR 40 and CON 43 carry a two-hander without a shield. | Heavy pair; D: Brigandine Set without its shield. | Melee | STR+1 CON− (1, 13, 25): human CON 43 has HP to spare |
| RO | Rogue (7), Damage → Treasure Hunter 1, Hawkeye 1 | DAGGER (Treasure Hunter branch); BOW (Hawkeye branch). | Light pair (Armor Mastery's evasion); D: Manticore Skin Set (chest 395, legs 417, feet 2424: +92 MP), else Reinforced Leather (394, 416, 2422: +80 MP). | Melee; bow 500, Power Shot 700, Stunning Shot 900 | DEX+1 CON− (6, 18, 30) and STR+1 CON− (1, 13, 25) |
| EK | Elven Knight (19), Support → Temple Knight; Damage → Sword Singer | SWORD with a shield (Power Strike, kept at level 9 from the base tree). | Heavy pair; D: Brigandine Set with its shield. | Melee; Aggression 400 | CON+1 DEX− (4, 16, 28): elven DEX 35 is the stat to spare |
| ES | Elven Scout (22), Damage → Plains Walker 1, Silver Ranger 1 | DAGGER (Plains Walker branch); BOW (Silver Ranger branch). DEX 35 and run 144. | Light pair; D: Manticore Skin, else Reinforced Leather. | Melee; bow 500, Power Shot 700, Stunning Shot 900 | STR+1 DEX− (2, 14, 26): low STR is its gap, high DEX loses least (`sqrt(DEX)` accuracy) |
| PK | Palus Knight (32), Support → Shillien Knight; Damage → Bladedancer | SWORD with a shield (Sting takes SWORD, DAGGER, DUAL, BIGSWORD; Power Strike SWORD; a sword serves both). | Heavy pair; D: Brigandine Set with its shield (+153 HP covers dark elven CON 32). | Melee; Drain Health 600; Aggression 400 | CON+1 DEX− (4, 16, 28) |
| AS | Assassin (35), Damage → Abyss Walker 2, Phantom Ranger 1 | DAGGER (Abyss Walker branch); BOW (Phantom Ranger branch). STR 41 makes the hardest blows. | Light pair; D: Manticore Skin, else Reinforced Leather. | Melee; Drain Health 600; bow 500, Power Shot 700, Stunning Shot 900 | STR+1 DEX− (2, 14, 26): CON 32 is too low to pay with |
| OR | Orc Raider (45), Support and Damage → Destroyer | BIGBLUNT (Power Smash and Stun Attack), two-handed. CON 47 lets it drop the shield. | Heavy pair; D: Brigandine Set without its shield. | Melee | STR+1 CON− (1, 13, 25): orc CON 47 |
| MO | Monk (47), Damage → Tyrant | DUALFIST (every Monk skill; Cripple also FIST). | Light pair; D: Manticore Skin, else Reinforced Leather. | Melee; Force Blaster 600 | STR+1 CON− (1, 13, 25) |
| SC | Scavenger (54), Support and Damage → Bounty Hunter | BLUNT with a shield (Stun Attack). | Heavy pair; D: Brigandine Set with its shield. | Melee (Spoil 40, Sweeper 20) | STR+1 CON− (1, 13, 25): dwarven CON 45 |
| CL | Cleric (15), Support → Bishop 2, Prophet 1 | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Mithril Robe Set (chest 437, legs 470, gloves 2450: +1 WIT −1 INT, +7 run): heal cast speed over M.Atk. | Cast 600 | WIT+1 MEN− (12, 24, 36): heal cast speed, as its Mithril Robe; the INT rows exclude healers, and human MEN 39 pays |
| HW | Human Wizard (11), Damage → Sorcerer | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Knowledge Set (chest 436, legs 469, gloves 2447: ×1.10 M.Atk, ×0.95 MP regen). | Cast 750 (Blaze) | INT+1 WIT− (8, 20, 32) and INT+1 MEN− (7, 19, 31): human WIT 20 and MEN 39 share the cost |
| EO | Elven Oracle (29), Support → Elven Elder | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Mithril Robe Set. | Cast 600 | WIT+1 MEN− (12, 24, 36): keeps the elven cast-speed lead, MEN 40 pays |
| EW | Elven Wizard (26), Damage → Spellsinger | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Knowledge Set. | Cast 750 (Aqua Swirl, Solar Spark) | INT+1 MEN− (7, 19, 31): WIT is its strength |
| SO | Shillien Oracle (42), Support → Shillien Elder | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Mithril Robe Set. | Cast 600 | MEN+1 WIT− (10, 22, 34) |
| DW | Dark Wizard (39), Damage → Spellhowler | BLUNT/MAGIC or SWORD/MAGIC. | Robe pair; D: Knowledge Set. | Cast 750 (Twister, Shadow Spark) | INT+1 WIT− (8, 20, 32) twice: all in on INT 44 |
| OS | Orc Shaman (50), Support and Damage → Warcryer | BLUNT/MAGIC (casts, and Stun Attack takes BLUNT). | Heavy pair (Heavy Armor Mastery: cast ×0.855 net, P.Def); D: Brigandine Set without its shield. The Knowledge robe is the fallback when no heavy pair is affordable. | Cast 750 (Frost Flame); melee with Stun Attack | MEN+1 WIT− (10, 22, 34): chants run on MP |

**Model-chosen sets (#369).** The model (Design section 3, Value) ranks a row's set by what its
set skill adds to the role's value; where the role's value cannot read the stat the set adds, the
row's set is model-chosen: it wins on its pieces' stats and the StatWeights tie-break, or loses to
a set worth more. These rows keep the model as it is:

- RO, ES, AS, MO, Damage: Manticore Skin and Reinforced Leather add max MP, which counts where the
  kill's MP binds the rest. Where HP binds it, or the fight does not pay off, MP rests nothing away
  and the sets are model-chosen.
- RO, ES, AS, MO, HW, EW, DW and OS, Support: the build knows no heal the model times (the
  shaman's Chant of Life heals over time, with no skill power), so it is a tank valued by how long
  it lasts; max MP and M.Atk buy no survival, and Manticore Skin, Reinforced Leather and
  Knowledge are model-chosen.
- HM, EM, DM (Devotion) and CL, EO, SO (Mithril Robe), Support: the sets gain where the support
  lasts the survival floor. Against its level's reference monster the naked support dies inside
  the floor, so its value is how long it lasts (`worth`), which reads no cast speed, max MP or
  M.Atk, and the sets are model-chosen there.

`TestRow<ID>SetValue` (`gear_value_rows_test.go`) holds each row of the shipped datapack to this.

### Combat (columns 3-7)

| Row | 3. Opener | 4. Rotation (priority order, gates) | 5. Sustain | 6. Self-buffs and toggles | 7. Rest |
| --- | --- | --- | --- | --- | --- |
| HF | Bow: 56 Power Shot @5 from 700, a pull. Else walks in. | 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | none (potions, rule 3) | none | HP; 226 Relax @5 |
| EF | Bow: 56 Power Shot @5 from 700, a pull. Else walks in. | 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | 58 Elemental Heal @15 S<50 | 91 Defense Aura @5, 77 Attack Aura @10, out of combat | HP; plain sit |
| DF | 70 Drain Health from 600 @15, a pull; before 15 as HF. | 70 Drain Health @15 open; 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | 70 Drain Health @15 S<60 | 91 Defense Aura @5, 77 Attack Aura @10 | HP; plain sit |
| OF | Walks in. | 29 Iron Punch [DUALFIST] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | none | none (Toughness 134, Iron Body 295 are passive) | HP; 226 Relax @5 |
| WF | 254 Spoil @10, open, at melee range, until 612 (spoiled by the bot); 357 (another's spoil) ends it with no sweep; a 139 to Spoil retries at reuse. | 254 Spoil @10 open; melee. After the kill: 42 Sweeper @10 on the spoiled corpse (section 8). | none | none | HP; plain sit |
| HM | 1177 Wind Strike from 600 before 7, then 1184 Ice Bolt: a pull. | 1184 Ice Bolt @7; 1177 Wind Strike @1; melee only under MPReserve. Both roles nuke; the Support role's heals and buffs go first (Heals: 1015 Battle Heal @14 under 35 %, 1027 Group Heal @14 with two members under 60 %, 1011 Heal @7 under 60 %), and its 50 % MPReserve stops the nukes sooner. | 1216 Self Heal @1 S<40; 1147 Vampiric Touch @14 S<60 | Support: 1068 Might @7, 1040 Shield @7 on itself and the party | MP; plain sit |
| EM | Damage: 1206 Wind Shackle @14 MP>50, from 600; otherwise the first ready nuke (1184 Ice Bolt from 7, else 1177 Wind Strike). | 1206 Wind Shackle @14 open MP>50 (Damage); 1184 Ice Bolt @7; 1177 Wind Strike @1; melee only under MPReserve. The Support role's heals (as HM) and buffs go first. | 1216 Self Heal @1 S<50 (no drain) | Support: 1068 Might @7, 1040 Shield @7 | MP; plain sit |
| DM | Damage: 1168 Curse: Poison @7 MP>60, from 600; otherwise the first ready nuke (1184 Ice Bolt from 7, else 1177 Wind Strike). | 1168 Curse: Poison @7 open MP>60 (Damage); 1184 Ice Bolt @7; 1177 Wind Strike @1; melee only under MPReserve. The Support role as HM. | 1147 Vampiric Touch @14 S<70; 1216 Self Heal @1 S<40 | Support: 1068 Might @7, 1040 Shield @7 | MP; plain sit |
| OM | 1100 Chill Flame @7 from 600, a pull; before 7 walks in. | 1100 Chill Flame @7 open; 1090 Life Drain @7; melee always (Support, in light or heavy armor), only under MPReserve (Damage, in a robe). | 1090 Life Drain @7 S<70 | 1001 Soul Cry @1 (combat toggle, on while MP>30); 1010 Soul Shield @7; 1007 Chant of Battle @14 (party) | MP; plain sit |
| KN | 28 Aggression @24 from 400, a pull; before 24 70 Drain Health from 600. | 28 Aggression @24 open; 70 Drain Health @20 open (from 24 only while Aggression is on reuse); 92 Shield Stun [SHIELD] @20; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 70 Drain Health @20 S<60; 45 Divine Heal @28 S<40; 110 Ultimate Defense @20 S<20 | 82 Majesty @20 | HP; 226 Relax (base tree) |
| WA | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20; 255 Power Smash [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @20; 245 Wild Sweep [POLE] @20 A≥2; melee. | 121 Battle Roar @28 S<30 | 78 War Cry @20 in combat (self step: 60 s, reuse 180 s, so recast when gone and ready); 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, on while MP>30) | HP; 226 Relax (base tree) |
| RO | Bow: 56 Power Shot from 700, a pull (101 Stunning Shot from 900 @36). Dagger: walks in. | Dagger: 96 Bleed [DAGGER] @24 T≥50; 16 Mortal Blow [DAGGER] @20. Bow: 101 Stunning Shot [BOW] @36; 56 Power Shot [BOW] @20. Melee between. | 111 Ultimate Evasion @28 S<30 | 99 Rapid Shot @32 (bow, in combat); 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, MP>30); 4 Dash @20 only to flee | HP; 226 Relax (base tree) |
| EK | 28 Aggression @24 from 400, a pull; before 24 walks in. | 28 Aggression @24 open; 102 Entangle @36 open; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 58 Elemental Heal @20 S<50; 110 Ultimate Defense @20 S<20 | 91 Defense Aura (base), 77 Attack Aura (base; level 2 from 28) | HP; plain sit |
| ES | From 36, 102 Entangle opens for both branches. Before: bow, 56 Power Shot from 700; dagger, walks in. | 102 Entangle @36 open; dagger: 96 Bleed [DAGGER] @24 T≥50, 16 Mortal Blow [DAGGER] @20; bow: 101 Stunning Shot [BOW] @36, 56 Power Shot [BOW] @20; melee. | 58 Elemental Heal @20 S<50; 111 Ultimate Evasion @28 S<30 | Auras 91, 77 (base); 99 Rapid Shot @32 (bow, in combat); 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, MP>30); 230 Sprint @32 only to flee | HP; plain sit |
| PK | 70 Drain Health @20 from 600, a pull; 28 Aggression @24 for the Support role. | 28 Aggression @24 open (Support); 70 Drain Health @20 open; 115 Power Break @32 open; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24; 105 Freezing Strike @36; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 70 Drain Health @20 S<60; 110 Ultimate Defense @20 S<20 | 91 Defense Aura (base), 77 Attack Aura (base; level 2 from 28) | HP; plain sit |
| AS | Dagger: 70 Drain Health @20 from 600, a pull. Bow: 56 Power Shot from 700 (101 Stunning Shot from 900 @36). | Dagger: 70 Drain Health @20 open; 96 Bleed [DAGGER] @24 T≥50; 16 Mortal Blow [DAGGER] @20; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24. Bow: 101 Stunning Shot [BOW] @36; 56 Power Shot [BOW] @20. Melee between. | 70 Drain Health @20 S<60; 111 Ultimate Evasion @28 S<30 | Auras 91, 77 (base); 99 Rapid Shot @32 (bow, in combat); 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, MP>30) | HP; plain sit |
| OR | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20; 255 Power Smash [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @20; melee. | 121 Battle Roar @28 S<30 | 94 Rage @24 in combat; 176 Frenzy @32 S<30, else 139 Guts @36 S<30 (one "pinch" stack: Guts only while Frenzy is on reuse and not on the bot); 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, MP>30) | HP; 226 Relax (base tree) |
| MO | 54 Force Blaster [DUALFIST] @24 C≥1 from 600, a pull; else walks in. | 50 Focused Force [DUALFIST] @24 C<max out of combat (1 charge at 24-31, 2 from 32); 54 Force Blaster [DUALFIST] @24 open C≥1; 95 Cripple [FIST, DUALFIST] @20 open; 284 Hurricane Assault [DUALFIST] @36 C≥2; 54 Force Blaster [DUALFIST] @24 C≥1; 120 Stunning Fist [DUALFIST] @20; 29 Iron Punch [DUALFIST] @20; melee. | none | 83 Wolf Spirit Totem [DUALFIST] @20, replaced by 76 Bear Spirit Totem [DUALFIST] from 28 (one "possession" stack: `CompileBuild` keeps only Bear) | HP; 226 Relax (base tree) |
| SC | 254 Spoil @20 (levels 2-4), open, at melee range, until spoiled. | 254 Spoil open; 100 Stun Attack [BLUNT, BIGBLUNT] @20; melee. After the kill: 42 Sweeper (section 8). | none | none | HP; plain sit |
| CL | 1177 Wind Strike (level 5) from 600, on the leader's target in a party. | Heals: 1015 Battle Heal @20 under 35 %, 1027 Group Heal @20 with two members under 60 %, 1011 Heal @20 under 60 %. Steps: 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. Raise: 1016 Resurrection @20 out of combat on a dead member of its bot party in sight, the leader first (`raise`, [parties](parties/members.md#events)). | Its heals on itself (rule 3 order: leader, self, others) | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1085 Acumen @20 (mystic members), 1078 Concentration @20 | MP; plain sit |
| HW | 1220 Blaze @20 from 750, a pull. | 1220 Blaze @20; 1184 Ice Bolt @20 (levels 5-6); 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150; melee only under MPReserve. | 1147 Vampiric Touch @20 S<60; 1157 Body To Mind @25 when MP under 30 % and S≥70 | 1078 Concentration @20 | MP; plain sit |
| EO | 1177 Wind Strike from 600, on the leader's target in a party. | Heals and raise as CL. Steps: 1201 Dryad Root @25 on a second attacker A≥2; 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1087 Agility @25, 1043 Holy Weapon @25 | MP; plain sit |
| EW | 1206 Wind Shackle MP>50 from 600, else the first ready nuke from 750 (1264 Solar Spark from 25, else 1175 Aqua Swirl). | 1206 Wind Shackle open MP>50; 1264 Solar Spark @25; 1175 Aqua Swirl @20; 1184 Ice Bolt @20; 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150; melee only under MPReserve. | 1216 Self Heal (base, level 1) S<40 (no drain) | 1078 Concentration @20 | MP; plain sit |
| SO | 1206 Wind Shackle from 600, once, on its target (the leader's in a party). | Heals and raise as CL. Steps: 1206 Wind Shackle open; 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1059 Greater Empower @25, 1077 Focus @25, 1268 Vampiric Rage @30 | MP; plain sit |
| DW | 1168 Curse: Poison MP>60 from 600, else the first ready nuke from 750 (1266 Shadow Spark from 25, else 1178 Twister). | 1168 Curse: Poison open MP>60; 1266 Shadow Spark @25; 1178 Twister @20; 1184 Ice Bolt @20; 1172 Aura Burn @20 ≤150; melee only under MPReserve. | 1147 Vampiric Touch @20 S<70; 1157 Body To Mind @25 when MP under 30 % and S≥70 | 1078 Concentration @20 | MP; plain sit |
| OS | 1107 Frost Flame @20 from 750, a pull. | 1107 Frost Flame @20 open; 1090 Life Drain @20 (levels 3-6); 100 Stun Attack [BLUNT, BIGBLUNT] @20 ≤40, else melee (always). | 1090 Life Drain S<70; 1229 Chant of Life @20 (party) with a member under 50 % | 1001 Soul Cry (base; combat toggle, on while MP>30); party chants 1007 Chant of Battle (base), 1009 Chant of Shielding @20, 1006 Chant of Fire @20, 1002 Flame Chant @30; 1010 Soul Shield (base) | MP; plain sit |

### Numbers and summary (columns 8, 10, 12)

Thresholds follow the HP multiplier (CON) and the effective run speed of the stat table: the
higher the CON, the later the bot drinks, flees from a crowd or sits; the faster it runs, the more
a flee works (`FleeOutrun` on for every row, so a slow bot fights on against a faster attacker
rather than run, #281). `MPReserve` gates attack steps only; sustain and heals run down to their
own cost. Where a row has both roles, values are Support / Damage. Spot margin: a spot fits while
`TimeToDie ≥ margin × TimeToKill` against one of its monsters at a time, the potions shared over
its likely pack (Design section 11); tank 1.5, normal 2.0, glass cannon 3.0.

| Row | 8. MPReserve | Sit HP / MP | Stand HP / MP | Critical | Crowded | Outmatched | 10. Spot | 12. Plays like |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| HF | 0 | 35 / 15 | 90 / 70 | 15 | 60 | 30 | normal 2.0 | A sturdy all-rounder that walks in, trades blows, Power Strikes on cooldown and rests with Relax. |
| EF | 20 | 45 / 20 | 90 / 80 | 25 | 80 | 50 | normal 2.0 | A light, fast skirmisher that heals itself and outruns what it cannot beat. |
| DF | 20 | 50 / 20 | 95 / 80 | 30 | 85 | 60 | glass cannon 3.0 | A glass cannon that pulls with Drain Health, kills fast, drains to stay up and leaves a crowd early. |
| OF | 0 | 30 / 15 | 85 / 70 | 15 | 50 | 25 | tank 1.5 | A tank that wades into packs and keeps swinging when others would run. |
| WF | 0 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | normal 2.0 | A spoiler that marks each target, beats it down, sweeps the corpse and carries everything home. |
| HM | 50 / 10 | 50 / 30 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A balanced caster: Ice Bolt and Wind Strike from range, Vampiric Touch to stay alive, sits on MP. |
| EM | 50 / 10 | 50 / 30 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | The quickest caster: slows first, out-casts the target, heals itself for want of a drain. |
| DM | 50 / 10 | 55 / 30 / 20 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | The hardest nuker and the most fragile: kills before it is reached and drains hard when it is not. |
| OM | 50 / 30 | 45 / 30 / 25 | 90 / 90 | 25 | 80 | 50 | normal 2.0 | A sturdy buffer: Soul Cry on, chants up, burns and drains, and can stand in a fight. |
| KN | 20 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | tank 1.5 | A shield tank that pulls with Aggression, stuns with its shield and heals itself through long fights. |
| WA | 0 | 35 / 15 | 90 / 70 | 15 | 60 | 30 | normal 2.0 | A two-handed bruiser: War Cry, then Stun Attack and Power Smash, Battle Roar when low. |
| RO | 10 | 40 / 20 | 90 / 80 | 20 | 70 | 40 | normal 2.0 | A human dagger or archer: blows up close or shots from afar, Dash away from trouble. |
| EK | 25 | 45 / 25 | 90 / 80 | 20 | 75 | 45 | normal 2.0 | A self-healing knight: steady sword and shield, Elemental Heal keeps it standing. |
| ES | 20 | 45 / 25 | 90 / 80 | 25 | 80 | 50 | normal 2.0 | The fastest skirmisher: shoots or stabs, heals itself, Sprints away from what it cannot kill. |
| PK | 25 | 50 / 25 | 95 / 80 | 25 | 80 | 50 | normal 2.0 | A dark knight that pulls and heals with Drain Health and hits hard with Sting behind a shield. |
| AS | 20 | 50 / 25 | 95 / 80 | 30 | 85 | 60 | glass cannon 3.0 | The hardest-hitting dagger: Bleed, blows and drains; drops a fight it cannot win fast. |
| OR | 0 | 30 / 15 | 85 / 70 | 10 | 50 | 25 | tank 1.5 | A berserker that swings a two-hander through packs, with Rage and Frenzy when the fight turns. |
| MO | 10 | 30 / 20 | 85 / 80 | 15 | 50 | 25 | normal 2.0 | A fist fighter that builds force, blasts from range, then lands a flurry of fist skills. |
| SC | 10 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | tank 1.5 | An armored spoiler: spoil, stun, kill, sweep, and haul twice the loot. |
| CL | 50 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A party healer: the leader first, Might, Shield and Wind Walk kept up, nukes only with MP to spare. |
| HW | 10 | 50 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A fire nuker: Blaze from the farthest range, Vampiric Touch and Body To Mind to keep going. |
| EO | 50 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A fast-casting healer: the elven cast speed turns into heals in time. |
| EW | 10 | 50 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A water caster that slows, then out-casts everything; with no drain it sits more. |
| SO | 50 | 55 / 30 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | A dark healer that heals, buffs and shackles what the party fights. |
| DW | 10 | 55 / 20 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | The highest-burst nuker: Shadow Spark and Twister kill before contact; drains when touched. |
| OS | 50 / 30 | 45 / 25 | 90 / 90 | 25 | 80 | 50 | normal 2.0 | A battle shaman in heavy armor: chants the party, burns from range, finishes with a mace. |

For HM, EM, DM and OM the Sit column is HP / MP (Support) / MP (Damage).

### What is purely racial

Classes with the same weapon play differently because of their race alone:

- **Outnumbered with a sword or blunt.** An orc (OF, OR: `OutmatchedHP` 25, `CrowdedHP` 50, HP
  ×1.77) fights on through a pack that has a dark elf (DF: 60 and 85, HP ×1.14) drinking early
  and leaving, and a human (HF, WA, KN: 30, 60, ×1.58) in between. With a sword, HF and OF cast
  the same step (Power Strike) and rest with Relax: only the thresholds and the spot margin
  differ; DF adds its Drain Health.
- **Self-healing.** Only elven fighters (EF, EK, ES) heal themselves from 15, with Elemental
  Heal (the human Knight gets Divine Heal only at 28): with the same dagger, an elven scout heals itself where a human rogue (RO) drinks a potion and a
  dark elven assassin (AS) drains with Drain Health.
- **Drain from range.** Only dark elven fighters (DF, PK, AS) and the human Knight have Drain
  Health: they open with a pull from 600 that also heals (the Knight until Aggression at 24, PK in
  the Damage role), where the other melee rows walk in.
- **Relax.** Only the human and orc fighter trees have it (HF, OF and their professions KN, WA, RO,
  OR, MO); elves, dark elves and dwarves rest with a plain sit.
- **Spoil, then sweep.** Only dwarves (WF, SC) open with Spoil and end a fight with Sweeper inside
  the 14 s window, and with Weight Limit carry about twice what another fighter does before a
  supply trip.
- **Mystic armor.** An orc mystic's Support role stands in light or heavy armor (OM from 7, OS),
  the Orc Shaman also with a mace and Stun Attack, where human, elven and dark elven mystics stay in robes at range, since out of a robe
  Spellcraft halves their cast speed.
- **Mystic openers.** At 14 the elven mystic opens with Wind Shackle, which the human mystic tree
  does not have; the human mystic, without a slow, opens with a nuke. The dark mystic's lowest HP
  widens its flee bounds (Critical 35, Outmatched 65) over the human and elven mystics' 30 and 60.
- **Fleeing.** With effective runs of 144 and 139, elven and dark elven fighters usually outrun
  their attackers and flee; an orc (124) or dwarf (125) seldom can, and `FleeOutrun` keeps them
  fighting.

### Second professions (levels 40+)

The 24 second professions of the branch table (Design section 6), in the same three tables and
notation as the rows above (#332). Until their rows ship (Delivery: S13-S17, #587-#591), a second profession
plays its first profession's row; then `professionKits` holds a row of its own, which CompileBuild
uses from the class change at 40. A second-profession row is a whole row: it repeats the first
profession's steps it keeps rather than inheriting them.

**Notation, second professions.** The row start is 40, so a skill with no level tag is known at 40
(first merged `minLvl` ≤ 40), whether it comes from the base, the first or the second tree, and
`@N` is a skill first learned past 40. `(fixed)` marks an attack step the row ranks behind its
successor because the second profession's own tree does not raise it: it stays at the level the
first profession reached by 39 (Mortal Blow behind Deadly Blow). The tag is required only on such
steps; other skills from earlier trees (226 Relax, 256 Accuracy, 28 Aggression, a heal or a toggle)
are untagged even though the own tree does not raise them either. `TestBehaviorRowsShipped` checks
the tag one way: a tagged skill is in the merged tree and absent from the profession's own tree; an
untagged skill is not checked for it. "replaced
by X from N" is the stack rule's "replaced by" (CompileBuild drops the earlier skill from N).
Row ids: PL Paladin, TK Temple Knight, SK Shillien Knight, SS Sword Singer, GL Gladiator, WL
Warlord, DS Destroyer, BD Bladedancer, BH Bounty Hunter, TH Treasure Hunter, HK Hawkeye, PW Plains
Walker, SR Silver Ranger, AW Abyss Walker, PR Phantom Ranger, TY Tyrant, BI Bishop, PP Prophet,
EE Elven Elder, SE Shillien Elder, SN Sorcerer, SG Spellsinger, SH Spellhowler, WC Warcryer.

**Grade C and dyes.** C-grade sets are worn from 40 (Expertise C); the pieces are the ones
`armorSets.xml` checks (a full-body chest has no legs). A second profession has 3 dye slots and
draws the second-profession symbols (`hennas.xml` 37+, whose class lists name only second and third
professions): the column lists, per direction, the +1 symbol of each price tier (87000 loses 3,
149000 loses 2, 206000 loses 1). Three +1 symbols stay under the +5 cap. Healers have no INT row
(the INT symbols 43-45 and 55-57 leave 16, 17, 30, 43, 52 out), as for the first professions.

#### Gear and range (columns 1, 2, 9, 11)

| Row | Class (id), role ← first profession | 1. Weapon kind, and why | 2. Armor pair; preferred C-grade sets | 9. Fight range | 11. Dyes |
| --- | --- | --- | --- | --- | --- |
| PL | Paladin (5), Support ← KN | SWORD or BLUNT with a SHIELD: Shield Stun keeps its shield gate, Tribunal takes SWORD or BLUNT. | Heavy; Full Plate Set (full-body chest 356, head 2414, shield 2497: +270 HP), else Composite (chest 60, head 517, shield 107: +5795 weight). | Melee; Aggression 400 | CON+1 DEX− (40, 52, 112) |
| TK | Temple Knight (20), Support ← EK | SWORD with a SHIELD (Tribunal, Power Strike). | Heavy; Full Plate Set with its shield. | Melee; Aggression 400 | CON+1 DEX− (40, 52, 112) |
| SK | Shillien Knight (33), Support ← PK | SWORD with a SHIELD: Judgment takes SWORD, Sting SWORD. | Heavy; Full Plate Set with its shield (+270 HP covers dark elven CON 32). | Melee; Drain Health and Life Leech 600; Aggression 400; Lightning Strike 400 | CON+1 DEX− (40, 52, 112) |
| SS | Sword Singer (21), Damage ← EK | SWORD with a shield: the songs take no weapon, and the tree has no dual mastery. | Heavy; Full Plate Set. | Melee; Aggression 400 | STR+1 DEX− (38, 50, 110): elven DEX 35 pays |
| GL | Gladiator (2), Damage ← WA | DUAL: Triple Slash, Double and Triple Sonic Slash take only DUAL; Dual Weapon Mastery 144. | Heavy; Full Plate Set without its shield. | Melee; Sonic Blaster 600; Sonic Storm 500 | STR+1 CON− (37, 49, 109): human CON 43 |
| WL | Warlord (3), Damage ← WA | POLE: Whirlwind, Thunder Storm, Shock Stomp, Wild Sweep. | Heavy; Full Plate Set without its shield. | Melee (pole) | STR+1 CON− (37, 49, 109) |
| DS | Destroyer (46), Support and Damage ← OR | BIGBLUNT or BIGSWORD: Crush of Doom takes both; Hammer Crush only the blunt, so BIGBLUNT first. | Heavy; Full Plate Set without its shield. | Melee | STR+1 CON− (37, 49, 109): orc CON 47 |
| BD | Bladedancer (34), Damage ← PK | DUAL: every dance and both blade dances take only DUAL; Sting takes DUAL. | Heavy; Full Plate Set without its shield. | Melee; Drain Health 600; Freezing Strike 600 | STR+1 DEX− (38, 50, 110): CON 32 is too low to pay with |
| BH | Bounty Hunter (55), Support and Damage ← SC | BLUNT with a shield (Hammer Crush, Stun Attack). | Heavy; Composite Set (+5795 weight: the spoil haul), else Full Plate. | Melee (Spoil 40, Sweeper 20) | STR+1 CON− (37, 49, 109): dwarven CON 45 |
| TH | Treasure Hunter (8), Damage ← RO | DAGGER (Deadly Blow, Critical Blow). | Light; Plated Leather Set (chest 398, legs 418, feet 2431: +4 STR −1 CON), else Mithril Light (397, 2387, feet 62: +4 evasion). | Melee; Lure 400 | DEX+1 CON− (42, 54, 114) and STR+1 CON− (37, 49, 109) |
| HK | Hawkeye (9), Damage ← RO | BOW (Double Shot, Burst Shot, Stunning Shot). | Light; Plated Leather Set. | Bow 500; Double Shot and Stunning Shot 900 | DEX+1 CON− (42, 54, 114) and STR+1 CON− (37, 49, 109) |
| PW | Plains Walker (23), Damage ← ES | DAGGER. | Light; Plated Leather, else Mithril Light. | Melee; Lure 400 | STR+1 DEX− (38, 50, 110) |
| SR | Silver Ranger (24), Damage ← ES | BOW. | Light; Plated Leather Set. | Bow 500; Double Shot and Stunning Shot 900 | STR+1 DEX− (38, 50, 110) |
| AW | Abyss Walker (36), Damage ← AS | DAGGER. | Light; Plated Leather, else Mithril Light. | Melee; Drain Health 600 | STR+1 DEX− (38, 50, 110) |
| PR | Phantom Ranger (37), Damage ← AS | BOW. | Light; Plated Leather Set. | Bow 500; Double Shot, Stunning Shot and Fatal Counter 900; Drain Health 600 | STR+1 DEX− (38, 50, 110) |
| TY | Tyrant (48), Damage ← MO | DUALFIST (every Tyrant skill). | Light; Plated Leather Set. | Melee; Force Blaster 600; Force Storm 500 | STR+1 CON− (37, 49, 109) |
| BI | Bishop (16), Support ← CL | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Divine Set (chest 442, legs 473, gloves 2463: +171 MP, +1 WIT −1 INT), else Karmian (439, 471, gloves 2454: ×1.15 cast). | Cast 600 | WIT+1 MEN− (48, 60, 120): heal cast speed |
| PP | Prophet (17), Support ← CL | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (buffs are cast speed), else Divine. | Cast 600; buffs 400 | WIT+1 MEN− (48, 60, 120) |
| EE | Elven Elder (30), Support ← EO | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Divine Set, else Karmian. | Cast 600 | WIT+1 MEN− (48, 60, 120) |
| SE | Shillien Elder (43), Support ← SO | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Divine Set, else Karmian. | Cast 600 | MEN+1 WIT− (46, 58, 118) |
| SN | Sorcerer (12), Damage ← HW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Demon Set (441, 472, gloves 2459: +4 INT −1 WIT, −270 HP) once its max HP covers the loss (over 810, three times the 270), else Karmian. | Cast 900 (Prominence) | INT+1 WIT− (44, 56, 116) and INT+1 MEN− (43, 55, 115) |
| SG | Spellsinger (27), Damage ← EW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set, else Demon as SN. | Cast 900 (Hydro Blast) | INT+1 MEN− (43, 55, 115): WIT is its strength |
| SH | Spellhowler (40), Damage ← DW | BLUNT/MAGIC or SWORD/MAGIC. | Robe; Karmian Set (Demon's −270 HP is a third of its HP at 40). | Cast 900 (Hurricane) | INT+1 WIT− (44, 56, 116) |
| WC | Warcryer (52), Support and Damage ← OS | BLUNT/MAGIC (casts; Hammer Crush and Stun Attack take BLUNT). | Heavy; Full Plate Set, else Composite; the Karmian robe is the fallback with no heavy pair affordable. | Cast 900 (Steal Essence); melee with Hammer Crush | MEN+1 WIT− (46, 58, 118): chants run on MP |

#### Combat (columns 3-7)

| Row | 3. Opener | 4. Rotation (priority order, gates) | 5. Sustain | 6. Self-buffs and toggles | 7. Rest |
| --- | --- | --- | --- | --- | --- |
| PL | 28 Aggression from 400, a pull. | 28 Aggression open; 18 Aura of Hate A≥2; 92 Shield Stun [SHIELD]; 400 Tribunal [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @55; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (fixed); melee. Heals: 262 Holy Blessing under 50 % (party, rule 3 order). | 262 Holy Blessing S<50 (on itself); 70 Drain Health (fixed) S<60; 45 Divine Heal (fixed) S<40; 110 Ultimate Defense S<20 | 82 Majesty, 72 Iron Will @43 (out of combat); 322 Shield Fortress @64 (combat toggle, MP>30) | HP; 226 Relax (base tree) |
| TK | 28 Aggression from 400, a pull. | 28 Aggression open; 102 Entangle open; 18 Aura of Hate A≥2; 400 Tribunal [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @55; 3 Power Strike (fixed); melee. | 58 Elemental Heal S<50; 110 Ultimate Defense S<20 | 91 Defense Aura, 77 Attack Aura (base); 123 Spirit Barrier; cubics 10 Storm Cubic and 67 Life Cubic @43 (kept up, S17); 288 Guard Stance @43 and 322 Shield Fortress @64 (combat toggles, MP>30) | HP; plain sit |
| SK | 28 Aggression from 400, a pull. | 28 Aggression open; 70 Drain Health open; 18 Aura of Hate A≥2; 289 Life Leech; 401 Judgment [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @55; 279 Lightning Strike @58; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD]; 105 Freezing Strike; 3 Power Strike (fixed); melee. | 289 Life Leech S<70; 70 Drain Health S<60; 110 Ultimate Defense S<20 | Auras 91, 77 (base); cubics 33 Phantom Cubic and 22 Vampiric Cubic @43 (kept up, S17); 288 Guard Stance @43 and 322 Shield Fortress @64 (combat toggles, MP>30) | HP; plain sit |
| SS | 28 Aggression from 400, a pull. | 28 Aggression open; 102 Entangle open; 98 Sword Symphony @55 A≥2; 3 Power Strike (fixed); melee. | 58 Elemental Heal S<50; 110 Ultimate Defense S<20 | Auras 91, 77 (base); 123 Spirit Barrier; party songs, out of combat: 267 Song of Warding, 268 Song of Wind @46, 269 Song of Hunter @49, 264 Song of Earth @55, 304 Song of Vitality @66 | HP; plain sit |
| GL | 6 Sonic Blaster @43 C≥2 from 600, a pull; before 43 walks in. | 8 Sonic Focus C<max out of combat; 6 Sonic Blaster @43 open C≥2; 261 Triple Sonic Slash [DUAL] @55 C≥4; 5 Double Sonic Slash [DUAL] @49 C≥3; 9 Sonic Buster @43 A≥2 C≥2; 7 Sonic Storm @49 A≥2 C≥2; 1 Triple Slash [DUAL]; 8 Sonic Focus C<max; melee. | 121 Battle Roar S<30 | 78 War Cry in combat; 297 Duelist Spirit [DUAL] @64 in combat; 256 Accuracy, 312 Vicious Stance and 424 War Frenzy @58 (combat toggles, MP>30) | HP; 226 Relax (base tree) |
| WL | Walks in. | 48 Thunder Storm [POLE]; 36 Whirlwind [POLE]; 452 Shock Stomp [POLE] @55 A≥2; 245 Wild Sweep [POLE] A≥2; melee. | 121 Battle Roar S<30; 181 Revival @55 S<15 | 78 War Cry, 317 Focus Attack [POLE] and 421 Fell Swoop [POLE] @58 in combat; 256 Accuracy, 312 Vicious Stance, 422 Polearm Accuracy [POLE] @58 and 424 War Frenzy @58 (combat toggles, MP>30) | HP; 226 Relax (base tree) |
| DS | Walks in. | 315 Crush of Doom [BIGBLUNT, BIGSWORD] @60; 260 Hammer Crush [BLUNT, BIGBLUNT]; 190 Fatal Strike [SWORD, BLUNT, BIGBLUNT, BIGSWORD]; 100 Stun Attack [BLUNT, BIGBLUNT] (fixed); 255 Power Smash [SWORD, BLUNT, BIGBLUNT, BIGSWORD] (fixed); melee. | 121 Battle Roar S<30 | 94 Rage in combat; 176 Frenzy S<30, else 139 Guts S<30 (the pinch stack, as OR); 420 Zealot @58 S<30 in combat; 256 Accuracy, 312 Vicious Stance and 424 War Frenzy @58 (combat toggles, MP>30) | HP; 226 Relax (base tree) |
| BD | 70 Drain Health from 600, a pull. | 70 Drain Health open; 115 Power Break open; 84 Poison Blade Dance [DUAL] @55 A≥2; 408 Demonic Blade Dance [DUAL] @55 A≥2; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD]; 105 Freezing Strike; melee. | 70 Drain Health S<60; 110 Ultimate Defense S<20 | Auras 91, 77 (base); party dances [DUAL], out of combat: 274 Dance of Fire, 272 Dance of Inspiration @46, 271 Dance of the Warrior @55, 275 Dance of Fury @58, 310 Dance of the Vampire @74 | HP; plain sit |
| BH | 254 Spoil, open, at melee range, until spoiled (as SC). | 254 Spoil open; 260 Hammer Crush [BLUNT, BIGBLUNT]; 100 Stun Attack [BLUNT, BIGBLUNT] (fixed); melee. After the kill: 42 Sweeper (section 8). | none | 424 War Frenzy @58 (combat toggle, MP>30) | HP; plain sit |
| TH | 51 Lure @52 from 400, a pull; before 52 walks in. | 51 Lure @52 open; 409 Critical Blow [DAGGER] @55; 263 Deadly Blow [DAGGER]; 96 Bleed [DAGGER] T≥50; 16 Mortal Blow [DAGGER] (fixed); 412 Sand Bomb @55 A≥2; melee. | 111 Ultimate Evasion S<30 | 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30); 4 Dash only to flee | HP; 226 Relax (base tree) |
| HK | 101 Stunning Shot [BOW] from 900, a pull. | 101 Stunning Shot [BOW] open; 19 Double Shot [BOW]; 24 Burst Shot [BOW] @46 A≥2; 101 Stunning Shot [BOW]; 56 Power Shot [BOW] (fixed); melee between. | 111 Ultimate Evasion S<30 | 131 Hawk Eye, 303 Soul of Sagittarius @46, 416 Blessing of Sagittarius @58 (out of combat); 99 Rapid Shot and 415 Spirit of Sagittarius @58 in combat; 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30); 4 Dash only to flee | HP; 226 Relax (base tree) |
| PW | 102 Entangle, else 51 Lure @52 from 400. | 102 Entangle open; 51 Lure @52 open; 321 Blinding Blow [DAGGER] @66; 263 Deadly Blow [DAGGER]; 96 Bleed [DAGGER] T≥50; 16 Mortal Blow [DAGGER] (fixed); 412 Sand Bomb @55 A≥2; melee. | 58 Elemental Heal S<50; 111 Ultimate Evasion S<30 | Auras 91, 77 (base); 123 Spirit Barrier; 410 Mortal Strike [DAGGER] @58 in combat; 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30); 230 Sprint only to flee | HP; plain sit, 296 Chameleon Rest @46 |
| SR | 102 Entangle, else 101 Stunning Shot [BOW] from 900. | 102 Entangle open; 101 Stunning Shot [BOW] open; 19 Double Shot [BOW]; 24 Burst Shot [BOW] @46 A≥2; 101 Stunning Shot [BOW]; 56 Power Shot [BOW] (fixed); melee between. | 58 Elemental Heal S<50; 111 Ultimate Evasion S<30 | Auras 91, 77 (base); 123 Spirit Barrier, 303 Soul of Sagittarius @46, 416 Blessing of Sagittarius @58 (out of combat); 99 Rapid Shot, 415 Spirit of Sagittarius @58 and 413 Rapid Fire [BOW] @60 in combat; 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30); 230 Sprint only to flee | HP; plain sit |
| AW | 70 Drain Health from 600, a pull. | 70 Drain Health open; 321 Blinding Blow [DAGGER] @66; 263 Deadly Blow [DAGGER]; 96 Bleed [DAGGER] T≥50; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD]; 16 Mortal Blow [DAGGER] (fixed); 412 Sand Bomb @55 A≥2; melee. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 | Auras 91, 77 (base); 410 Mortal Strike [DAGGER] @58 in combat; 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30) | HP; plain sit |
| PR | 101 Stunning Shot [BOW] from 900, a pull. | 101 Stunning Shot [BOW] open; 314 Fatal Counter [BOW] @60 S<50; 19 Double Shot [BOW]; 101 Stunning Shot [BOW]; 56 Power Shot [BOW] (fixed); melee between. | 70 Drain Health S<60; 111 Ultimate Evasion S<30 | Auras 91, 77 (base); 303 Soul of Sagittarius @46 (out of combat); 99 Rapid Shot, 415 Spirit of Sagittarius @58 and 414 Dead Eye [BOW] @60 in combat; 256 Accuracy and 312 Vicious Stance (combat toggles, MP>30) | HP; plain sit |
| TY | 54 Force Blaster [DUALFIST] C≥1 from 600, a pull. | 50 Focused Force [DUALFIST] C<max out of combat; 54 Force Blaster [DUALFIST] open C≥1; 95 Cripple [FIST, DUALFIST] open; 35 Force Storm [DUALFIST] @49 A≥2 C≥1; 17 Force Burst [DUALFIST] @43 A≥2 C≥1; 284 Hurricane Assault [DUALFIST] C≥2; 280 Burning Fist [DUALFIST]; 281 Soul Breaker [DUALFIST]; 54 Force Blaster [DUALFIST] C≥1; 120 Stunning Fist [DUALFIST] (fixed); 29 Iron Punch [DUALFIST] (fixed); 50 Focused Force [DUALFIST] C<max; melee. | none | 282 Puma Spirit Totem [DUALFIST] (in place of MO's 76 Bear Spirit Totem: attack speed and accuracy without Bear's ×0.7 run), replaced by 292 Bison Spirit Totem [DUALFIST] from 68 (one "possession" stack: `CompileBuild` keeps one); 420 Zealot @58 S<30 in combat; 424 War Frenzy @58 (combat toggle, MP>30) | HP; 226 Relax (base tree) |
| BI | 1177 Wind Strike (fixed) from 600, on the leader's target in a party. | Heals: 1258 Restore Life @44 under 20 %; 1218 Greater Battle Heal under 35 %; 1402 Major Group Heal @58 with two members under 60 %, else 1219 Greater Group Heal; 1401 Major Heal @56 under 60 %, else 1217 Greater Heal, else 1020 Vitalize @48. Steps: 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. Raise: 1016 Resurrection out of combat, as CL. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk, 1085 Acumen (mystic members), 1078 Concentration; 1307 Prayer @66 | MP; plain sit |
| PP | 1177 Wind Strike (fixed) from 600, on the leader's target in a party. | Heals and raise as CL (1015 Battle Heal, 1027 Group Heal, 1011 Heal, all fixed). Steps: 1201 Dryad Root on a second attacker A≥2; 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk, 1085 Acumen (mystic members), 1078 Concentration; 1243 Bless Shield, 1045 Blessed Body @44, 1036 Magic Barrier @44; fighter members: 1086 Haste @44, 1240 Guidance, 1242 Death Whisper, 1388 Greater Might @58; mystic members: 1048 Blessed Soul @44, 1389 Greater Shield @58 (Greater Might and Greater Shield share one stack) | MP; plain sit |
| EE | 1177 Wind Strike (fixed) from 600, on the leader's target in a party. | Heals and raise as CL, then from 48: 1401 Major Heal @56 under 60 %, else 1217 Greater Heal @48, else 1020 Vitalize @48 ahead of 1011 Heal. MP: 1013 Recharge on a member other than itself under 40 % MP (S16). Steps: 1201 Dryad Root A≥2; 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. | Its heals on itself | Party: as EO (1068 Might, 1040 Shield, 1204 Wind Walk, 1087 Agility, 1043 Holy Weapon); 1243 Bless Shield, 1304 Advanced Block @58; mystic members: 1397 Clarity @58, 1303 Wild Magic @62 | MP; plain sit |
| SE | 1206 Wind Shackle from 600, once, on its target (the leader's in a party). | Heals and raise as SO, then 1219 Greater Group Heal @48 with two members under 60 % ahead of 1027 Group Heal, 1217 Greater Heal @48 under 60 % ahead of 1011 Heal. MP: 1013 Recharge as EE. Steps: 1206 Wind Shackle open; 1177 Wind Strike above MPReserve; melee only alone and under MPReserve. | Its heals on itself | Party: as SO (1068 Might, 1040 Shield, 1204 Wind Walk, 1059 Greater Empower, 1077 Focus, 1268 Vampiric Rage); fighter members: 1240 Guidance, 1242 Death Whisper; mystic members: 1303 Wild Magic @62 | MP; plain sit |
| SN | 1230 Prominence from 900, a pull. | 1230 Prominence; 1083 Surrender To Fire T≥70 MP>50; 1296 Rain of Fire @58 A≥2; 1275 Aura Bolt ≤400; 1231 Aura Flare ≤150; 1220 Blaze (fixed); 1184 Ice Bolt (fixed); melee only under MPReserve. | 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind when MP under 30 % and S≥70 | 1078 Concentration; 1232 Blazing Skin (on itself) | MP; plain sit |
| SG | 1206 Wind Shackle (fixed) MP>50 from 600, else 1235 Hydro Blast from 900. | 1206 Wind Shackle open MP>50; 1235 Hydro Blast; 1237 Ice Dagger @44; 1265 Solar Flare; 1295 Aqua Splash @58 A≥2; 1275 Aura Bolt ≤400; 1231 Aura Flare ≤150; 1264 Solar Spark (fixed); 1175 Aqua Swirl (fixed); melee only under MPReserve. | 1216 Self Heal (base) S<40 (no drain) | 1078 Concentration; 1047 Mana Regeneration; 1238 Freezing Skin (on itself) | MP; plain sit |
| SH | 1168 Curse: Poison MP>60 from 600, else 1239 Hurricane from 900. | 1168 Curse: Poison open MP>60; 1239 Hurricane; 1148 Death Spike @44; 1267 Shadow Flare; 1176 Tempest @48 A≥2; 1266 Shadow Spark (fixed); 1178 Twister (fixed); 1172 Aura Burn (fixed) ≤150; melee only under MPReserve. | 1234 Vampiric Claw S<70; 1147 Vampiric Touch (fixed) S<60; 1157 Body To Mind when MP under 30 % and S≥70 | 1078 Concentration | MP; plain sit |
| WC | 1245 Steal Essence from 900, a pull. | 1245 Steal Essence open; 1107 Frost Flame (fixed); 260 Hammer Crush [BLUNT, BIGBLUNT] ≤40; 100 Stun Attack [BLUNT, BIGBLUNT] (fixed) ≤40, else melee (always). | 1245 Steal Essence S<70; 1090 Life Drain (fixed) S<70; 1229 Chant of Life (party) with a member under 50 % | 1001 Soul Cry (base; combat toggle, on while MP>30); party chants as OS (1007 Chant of Battle, 1009 Chant of Shielding, 1006 Chant of Fire, 1002 Flame Chant), 1310 Chant of Vampire @44, 1251 Chant of Fury @48, 1390 War Chant @58; 1010 Soul Shield (base) | MP; plain sit |

**Left out, and why** (every id is in its row's tree; the left-out skills are learned, never
cast):

- **Undead-only and corpse targets:** 49 Holy Strike, 97 Sanctuary, 405 Banish Undead, 107 Holy
  Aura, 1028 Might of Heaven, 1034 Repose, 1042 Hold Undead, 1049 Requiem, 1400 Turn Undead
  (target `UNDEAD` or `AURA_UNDEAD`, no gate reads a monster's race); 103 Corpse Plague, 1151
  Corpse Life Drain (corpse targets).
- **Aggro the bot does not want:** 286 Provoke (an `AGGDAMAGE` aura of radius 500 to 900 that
  pulls monsters not yet fighting; 18 Aura of Hate's radius is 200 and its A≥2 gate keeps it
  inside a fight already on); 12 Switch, 11 Trick, 106 Veil, 1273 Serenade of Eva, 1272 Word of Fear,
  1169 Curse Fear (drop or scatter the bot's own target).
- **Front-blocked or positional:** 30 Backstab lands 3 % from the front (server
  `formulas.BlowSucceeds`, `in.Backstab && in.Front`), and a solo bot faces what it fights; a
  party lane may add it with a "target attacks another" gate.
- **Holds, sleeps and roots of the bot's own target:** 402 Arrest, 403 Shackle, 404 Mass
  Shackling, 1072 Sleeping Cloud, 1394 Trance, 1064 Silence (they stop the kill they are cast
  in); 102 Entangle and 1201 Dryad Root stay as their first-profession rows use them.
- **Rare or self-limiting:** 60 Fake Death, 221 Silent Move, 411 Stealth (no flee rule reads
  them); 313 Snipe (roots the bot); 81 Punch of Doom (stuns the bot); 222 Fist Fury (an HP-draining
  toggle with no HP gate in Design section 9); 1430 Invocation (immobile); 320 Wrath (CP only:
  PvP, #421); 1159 Curse Death Link and 314 Fatal Counter outside the PR row (own-HP scaling); 417
  Pain of Sagittarius, 69 Sacrifice (cost HP); the seeds and 1288-1294 symphonies (one-hour reuse);
  1233 Decay, 1183 Freezing Shackle, 1244 Freezing Flame (DOTs the rotation would recast at reuse
  with no target-effect gate); 122 Hex and 116 Howl (debuffs with the same recast problem: Howl's
  P. Atk debuff lasts 15 s on an 8 s reuse).
- **Utility:** the Detect Weakness buffs (they pay against one monster race the bot does not
  pick), 419 Summon Treasure Key, the Quivers, 1050 Return, 1255 Party Recall, 1254 Mass
  Resurrection, 1311 Body Of Avatar, 1271 Benediction, 1418 Celestial Shield, 1395 Erase, 1396
  Magical Backfire, 1398 Mana Burn, 1399 Mana Storm, 1018 Purify, 453 Escape Shackle, 461 Break
  Duress, 1056 Cancellation, resist buffs (1182, 1189, 1259, 1033, 1392, 1393), 449 Summon
  Attractive Cubic, 278 Summon Viper Cubic (two cubics at a time; the row's two are kept).
- **Songs and dances past the row's five:** each costs 60 MP for 2 minutes; the rows keep the
  five that pay solo and in a bot party (P.Def, speed, crit, M.Def and HP regen for SS; crit
  damage, accuracy, P.Atk, attack speed and drain for BD).

#### Numbers and summary (columns 8, 10, 12)

| Row | 8. MPReserve | Sit HP / MP | Stand HP / MP | Critical | Crowded | Outmatched | 10. Spot | 12. Plays like |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| PL | 20 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | tank 1.5 | A holy shield tank: pulls with Aggression, holds a pack with Aura of Hate, heals itself and its party with Holy Blessing. |
| TK | 25 | 45 / 25 | 90 / 80 | 20 | 75 | 45 | tank 1.5 | An elven tank with cubics at its side: Guard Stance, Elemental Heal and a Life Cubic keep it standing. |
| SK | 25 | 50 / 25 | 95 / 80 | 25 | 80 | 50 | tank 1.5 | A dark tank that drains twice (Drain Health, Life Leech) and lets its cubics wear the target down. |
| SS | 30 | 45 / 25 | 90 / 80 | 20 | 75 | 45 | normal 2.0 | A singing swordsman: songs up before every pull, a sword and shield in the fight. |
| GL | 10 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | normal 2.0 | A dual-sword duelist: charges between fights, spends them on Sonic Slashes, Battle Roar when low. |
| WL | 10 | 35 / 20 | 90 / 80 | 15 | 50 | 25 | normal 2.0 | A polearm sweeper that hits a whole pack with Thunder Storm and Whirlwind. |
| DS | 0 | 30 / 15 | 85 / 70 | 10 | 50 | 25 | tank 1.5 | A berserker with a two-hander: Crush of Doom, Hammer Crush, and Zealot when the fight turns. |
| BD | 30 | 50 / 25 | 95 / 80 | 25 | 80 | 50 | normal 2.0 | A dark dancer: dances up, drains to pull, blade dances through a pack. |
| BH | 10 | 35 / 20 | 90 / 80 | 15 | 60 | 30 | tank 1.5 | A heavier spoiler: spoil, Hammer Crush, sweep, and a haul a Composite set carries. |
| TH | 10 | 40 / 20 | 90 / 80 | 20 | 70 | 40 | normal 2.0 | A human dagger: lures one monster out, Critical Blow and Deadly Blow it down, dashes from trouble. |
| HK | 10 | 40 / 20 | 90 / 80 | 20 | 70 | 40 | normal 2.0 | A human archer: stuns from 900, Double Shot, Burst Shot on a pack. |
| PW | 20 | 45 / 25 | 90 / 80 | 25 | 80 | 50 | normal 2.0 | The fastest dagger: entangles, blows, heals itself and rests unseen in Chameleon Rest. |
| SR | 20 | 45 / 25 | 90 / 80 | 25 | 80 | 50 | normal 2.0 | An elven archer: entangles, shoots, heals itself and sprints out of reach. |
| AW | 20 | 50 / 25 | 95 / 80 | 30 | 85 | 60 | glass cannon 3.0 | The hardest-hitting dagger: drains to pull, Deadly Blow and Bleed, leaves a fight it cannot win fast. |
| PR | 20 | 50 / 25 | 95 / 80 | 30 | 85 | 60 | glass cannon 3.0 | A dark archer: stuns from 900, drains when touched, Fatal Counter when low. |
| TY | 10 | 30 / 20 | 85 / 80 | 15 | 50 | 25 | normal 2.0 | A fist fighter with a totem: force charges, Force Storm on packs, Burning Fist and Soul Breaker. |
| BI | 50 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | The party healer: Greater and Major heals, the leader first, Restore Life when a member is about to fall. |
| PP | 50 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | The party buffer: Haste, Death Whisper and Guidance on the fighters, Greater Shield on the casters, heals between. |
| EE | 50 | 50 / 30 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A fast-casting healer that also refills the casters' MP with Recharge. |
| SE | 50 | 55 / 30 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | A dark healer: Empower and Focus on the casters, Recharge, Wind Shackle on the target. |
| SN | 10 | 50 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A fire nuker: Prominence from 900, Aura Flare when caught, Rain of Fire on a pack. |
| SG | 10 | 50 / 20 | 90 / 90 | 30 | 85 | 60 | glass cannon 3.0 | A water nuker: shackles, Hydro Blast and Ice Dagger, Mana Regeneration for its long sits. |
| SH | 10 | 55 / 20 | 95 / 90 | 35 | 90 | 65 | glass cannon 3.0 | The highest burst: Hurricane, Death Spike and Shadow Flare from 900; Vampiric Claw when touched. |
| WC | 50 / 30 | 45 / 25 | 90 / 90 | 25 | 80 | 50 | normal 2.0 | A battle shaman: War Chant and Chant of Fury on the party, drains with Steal Essence, finishes with a mace. |

Thresholds are the first profession's except where the kit changes the fight: the tanks (PL, TK,
SK, as KN) take the tank margin, the singers and dancers (SS, BD) keep 30 % MP for their songs and
dances (`MPReserve` gates attack steps only), the Gladiator and Warlord reserve 10 for their charge
and buff skills and sit and stand on MP as RO (20 / 80), and the Warlord's pack sweep fights on to
Crowded 50 and Outmatched 25, as the orc rows do. WC is Support / Damage as OS.

**Servitor lines (a later milestone, D23).** These second professions are never picked and get no
row here: Dark Avenger (6, ← KN: 283 Summon Dark Panther), Warlock (14, ← HW: 1276 Summon Kai the
Cat, 1331 Summon Feline Queen), Necromancer (13, ← HW: 1154, 1129, 1334 summon from a corpse),
Elemental Summoner (28, ← EW: 1277 Summon Merrow the Unicorn, 1332 Summon Unicorn Seraphim),
Phantom Summoner (41, ← DW: 1278 Summon Soulless, 1333 Summon Nightshade), with their servitor
buffs (1139-1141, 1299-1301). They need summon control (the servitor's own target, follow and
attack orders, its HP in the bot's rules), which the bot does not have: #592. The Overlord (51) is not
picked (a group class, D23), and the Warsmith (57, Summon Siege Golem 13 and the cannons) is the
crafter's line (M9, D18).

## Design

### 1. Own stats from `UserInfo` (`inworld.go`, `world.go`)

`decodeUserInfoEvent` keeps the six base stats and the eight combat stats it skips today, into
`Self`: `STR, DEX, CON, INT, WIT, MEN`, `PAtk, PDef, Evasion, Accuracy, Critical, MAtk, CastSpd,
MDef` (P.Atk speed is already kept). They are the truth for the bot's current state: hennas, set
skills, buffs and masteries are all in them. The model (section 3) uses them to calibrate and only
predicts the change a candidate would make. The server resends `UserInfo` on every change to
the character's stat functions, an equip and a buff included
(`model/actor/player/character_effects.go` `statsModified` emits `StatsModified` or, for run
speed, `RunSpeedChanged`; `network/movement.go` `sendModifiedStats` sends `UserInfo`), so the
kept stats stay current.

### 2. Datapack knowledge (`internal/datapack`)

New readers, each keeping exactly the fields it reads, with fixture tests like the existing ones:

- **Class templates** (`classes/*.xml`): per class id its race, parent, base stats, `runSpd`,
  `hpTable`/`mpTable`, and its skill tree (`<skill id lvl cost minLvl>`), merged with the
  parents' as the server does.
- **Skill combat fields** (`skills.go` extended): `skillType`, `operateType`, `target`,
  `weaponsAllowed`, `castRange`, the charge fields (`numCharges`, `maxCharges` per level, the
  `Charges` condition), toggle drains (`ManaDamOverTime`), each effect's `stackType` and `stackOrder` per level, `reuseDelay`, `hitTime`, `mpConsume` and `power` per level, and the armor-kind
  conditions of passive stat functions (`<using kind>` under `<not>` or not) with their stat,
  operation and value.
- **NPC combat stats** (`npc.go` extended): the template's base HP, P.Atk, P.Def, M.Atk, M.Def,
  attack speed, its six base stats (with the server's defaults when absent) and its `corpseTime`
  (the sweep window, section 8), for the time-to-kill and time-to-die estimates (section 3).
- **Trainers** (`npc.go`): the classes each trainer teaches, replacing the hand-written
  `mysticTrainers` and giving fighters their trainers.
- **Set skills** (`armorsets.go` extended): the stat functions of each set's skill, so a set is
  scored by what it grants.

### 3. The character model (`internal/charmodel`, new, pure)

A package with no brain or wire dependency. It owns copies of the server's formulas it needs: the
stat curves, the derived-stat functions of `skill/funcs`, the armor-kind conditions of the
masteries and penalties above, and simplified physical and magic damage (section "Server
behavior").

- **Input:** the bot's class template and level, its reported stats (section 1), its worn and
  candidate items, set skills, hennas; a monster template, whose final stats the model derives
  from its bases, base stats and level with the same stat functions (section "Server behavior",
  NPC combat stats). The monster side has no calibration, so the copied functions must match the
  server's on fixed templates (Testing).
- **Output:** a `Profile`: max HP and MP, P.Atk, M.Atk, attack and cast speed, critical rate,
  accuracy, evasion, P.Def, M.Def, run speed, weight limit; and against one monster template
  `TimeToKill` (melee, or the build's rotation) and `TimeToDie` (the monster's damage per second
  against the bot's HP and defense).
- **Calibration:** the model computes the bot's current profile from its template and gear; the
  ratio to the reported stats corrects every prediction, so buffs and anything the model does not
  know cancel out. Only the difference a candidate makes is used.
- **Value:** `Value(profile, role, target)` is the one number every errand compares: for damage
  roles the kills per minute a fight sustains before resting, for support roles the HP healed per
  minute: a full MP bar of its heal (the skill's power, plus √M.Atk for a `HEAL`, as the server's
  `formulas/heal.go` adds it unshot; a `HEAL_STATIC` adds nothing), cast at its cast speed, then the
  rest that wins the bar back, with a survival floor. Max MP counts for a damage role where the
  kill's MP binds the rest; a support's max MP, cast speed and M.Atk count while it lasts the
  survival floor (#369). A damage role fights with its first known rotation step that deals
  damage (not a heal or a bleed). A role's weights are persona defaults (D6).

### 4. The build (`internal/botclient`)

```go
// Build is what the bot plays: its race profile, its class kit and its role, compiled for its
// class and level.
type Build struct {
	Race       RaceProfile
	Kit        ClassKit
	Role       Role
	Branch     int32 // the first profession's class id the bot heads for; 0 until S8 chooses
	Thresholds Thresholds
}
```

- **RaceProfile** (one per race): the armor kinds its mystic or fighter line may wear (from the
  masteries the class tree grants by level), its racial skills, and its weighted profession
  branches per role (section 6).
- **ClassKit** (one per class id, falling back to the parent class): skills to learn in order, a
  rotation, self-buffs, party buffs and heals for support roles, toggles, the `Flee` skill (Dash, Sprint), the `Melee` policy, the fight range (melee, cast range, bow range), rest by
  HP or MP, and the dwarven spoil flag (section 8).
- **Rotation:** an ordered list of steps; the first whose gates pass is cast, else the kit's
  `Melee` policy:

  ```go
  type Step struct {
      Skill   int32
      // Weapons: the worn weapon's kind must be one, or SHIELD for a worn shield (the
      // server checks the held items); none for any.
      Weapons []string
      // The step applies only while the target's HP percent is at least MinTargetHP,
      // the bot's HP is under SelfHPBelow (sustain), and its MP is above MPAbove.
      MinTargetHP, SelfHPBelow, MPAbove int
      // Within casts the step only while the target is that close (a skill whose range is
      // shorter than the kit's fight range, such as Energy Bolt at 400 or Aura Burn at 150);
      // 0 for any distance. Attackers casts it only with at least that many attackers.
      Within    int32
      Attackers int
      // MPBelow casts it only with MP under that percent (Body To Mind); SelfHPAbove only
      // with HP above it.
      MPBelow, SelfHPAbove int
      // Target is whom the step is cast on: the current target (zero), the bot itself,
      // a second attacker (Dryad Root on the attacker the bot is not fighting) or the
      // party leader's target (a support's assist).
      Target StepTarget
      // Idle casts it only out of combat (Focused Force charging between fights).
      Idle bool
      // Charges casts it only with at least that many force charges (EtcStatusUpdate),
      // the skill's own Charges condition. ChargeToMax casts a charging skill only below
      // its maxCharges at the level the bot knows (the server refuses a full charge).
      Charges     int
      ChargeToMax bool
      // Opener casts it only as the first attack on a fresh target, once; a self-target,
      // Idle or enemy sustain step before it (Rapid Shot, War Cry, Focused Force, a Drain
      // Health under its HP gate) does not use the opener up.
      Opener bool
      Role   Role // zero for every role; else only in that role's rotation
  }
  ```

  A zero field is no gate (`SelfHPBelow` 0 casts at any HP, `Within` 0 at any distance).
  A target whose HP the bot has not seen yet (no `StatusUpdate` for it) counts as full. Weapons
  come from the worn weapon's datapack kind, known to the gear errand, so a step the server would
  refuse is never sent. "Behavior per class" fills these fields per row.
- **Buffs and heals** (columns 5 and 6) keep today's `Buff` and `Heal` entries and gain the gates
  the rows use: `Buff.Weapons` (the Monk's totems with fists), `Buff.Members` (a class
  filter: Acumen to mystic members); `Heal.Members` (cast only with that many party members under
  its HP bound: Group Heal at two under 60 %, Chant of Life at one under 50 %). **Stack rule**
  (S6, for every buff, self, party or member): a buff is cast on a target only while that target
  has no effect of the skill's `stackType` at an equal or higher `stackOrder`; the one exception
  is the same skill's own effect at the same order with less than `RebuffBefore` left, which it
  renews. An equal-order partner's effect (Guts over Frenzy) or a stronger one is never cast over;
  a weaker effect of another skill in the stack is upgraded, as the server replaces it (Chant of
  Shielding level 2, order 2, over Shield level 1, order 1). Sources:
  `AbnormalStatusUpdate` for the bot, `PartySpelled` (skill id, level, seconds left) for members,
  whose levels S6 keeps in `PartyMate.Buffs` (today only ids and end times); S2 reads the
  datapack fields. A party-target buff is cast while some member in range qualifies. So a
  chant never loops on a member holding a stronger effect (the Orc Shaman's Soul Shield level 2
  over Chant of Shielding level 1, both `pd_up`; Shield level 1 on a knight with Defense Aura
  level 2). In-combat self buffs (Rapid Shot, Rage, Frenzy, War Cry) are self-target rotation
  steps under the same rule ("Behavior per class", column 6).
- **Role:** `Support` plays for a party: mystics take the healer and buffer professions, fighters
  the tank line where their race has one (Knight, Elven Knight, Palus Knight); the orc and dwarven
  fighters, which have none, take Orc Raider and Scavenger. `Damage` farms alone and takes the
  damage professions. `Crafter` is the M9 dwarf. The preset sets the role (section 10).
- **Thresholds:** `MPReserve`, `SitHP`/`SitMP`, `StandHP`/`StandMP`, `CriticalHP`, `CrowdedHP`,
  `OutmatchedHP`, per row of "Behavior per class", as build defaults.
- **Compile:** `CompileBuild(race, class, level, role, data)` at entry and on every `UserInfo`
  whose class or level changed. It replaces `mysticSkills`, `mysticTrainers`, `supportBuilds`,
  `defaultGearClasses`, `defaultHeals`, `defaultBuffs`, `defaultAttackSkills`, `defaultDyeTable`
  and the persona thresholds. `Persona` keeps every role hook (goal, party, trade, store, chat,
  supply, crafter) and gains a `Role` and `Overrides`: a non-nil table (`LearnSkills`,
  `Trainers`, `ClassChange.Builds`, `Heals`, `Buffs`) or a non-zero threshold in it replaces the
  build's after `CompileBuild`. The crafter keeps its Create Item learning and Artisan line that
  way, and every threshold stays a persona setting (D6).
- **The rules stay,** with these named changes, all in S6 unless noted. `playRules` keeps its
  order and adds one rule, `sweep` after `outnumbered` (S10, section 8). Rule bodies that change:
  - `ruleCritical` tries the ready heal and drain steps before its potions (section 7);
  - `flee()`, which `ruleCritical` and `ruleOutnumbered` call, first casts the kit's `Flee` skill
    when ready;
  - `fight` runs the rotation (self-target steps first, under the stack rule, then enemy sustain steps under their HP
    gate, then attacks, then the kit's `Melee` policy), turns combat toggles on
    at engage and off when MP falls under the bound (section 9), and skips `Idle` steps;
  - `ruleRebuff`, out of combat, first turns off combat toggles left on, then casts `Idle` steps
    (Focused Force), then the out-of-combat buffs under the stack rule;
  - `ruleRest` turns combat toggles off and casts Relax instead of sitting (section 9).
  `fight`, `ruleHeal`, `ruleRebuff`, `ruleRest`, `ruleTrain`, `ruleClassChange`, `ruleGear` and
  `ruleDyes` read the build instead of the persona's tables.
- Tables stay Go tables in this repository, checked against the shipped datapack by tests
  (`TestBehaviorRowsShipped`, section "Testing"). A file format waits for a panel editor.

### 5. Gear, sets and dyes through the model

- **Gear score** becomes the model's value gain of wearing the piece, replacing `StatWeights`.
  Weapon kinds still come from the kit (a Mortal Blow kit wants a dagger), so a dagger user keeps
  a dagger even when a sword scores more in P.Atk alone. The shot factor and enchant gain (M8)
  stay as factors.
- **Armor kinds** come from the race profile: the orc mystic's Support role wears light armor from 7
  and heavy from 14 (rows OM and OS) when they score higher, its Damage role a robe; other mystics stay in robes, where the model's Spellcraft
  penalty already ranks every other kind far below.
- **Chest and legs as a pair.** The server counts an armor kind only for a matching chest and legs
  or a full-body piece (section "Server behavior"), so the model scores the chest and legs slots
  together: a robe top over light legs loses the mystic's `MAGIC` kind and halves its cast speed,
  and the gear errand never wears or buys such a mix.
- **Sets** are scored by their set skill through the model, replacing `GearPolicy.SetBonus`.
  Purchases and the grade-jump saving (M8 I6, I7) keep their flow and compare model values.
- **Dyes:** within each `hennas.xml` row's class list and the bot's budget, the bot draws the
  symbols with the highest value gain, replacing the per-profession table.
- **Weight** needs nothing new: the loot, supply and trade thresholds are already fractions of
  the server-reported `MaxLoad`, which counts CON and Weight Limit, so an orc or a dwarf already
  carries more before a supply trip.

### 6. Professions by race and role

`ClassChange.Builds` is computed from the race profile's branches for the bot's role:

| Base class | Support role | Damage role (weights) |
| --- | --- | --- |
| Human Fighter | Knight → Paladin | Warrior → Gladiator 2, Warlord 1; Rogue → Treasure Hunter 1, Hawkeye 1 |
| Elven Fighter | Elven Knight → Temple Knight | Elven Knight → Sword Singer 1; Elven Scout → Plains Walker 1, Silver Ranger 1 |
| Dark Fighter | Palus Knight → Shillien Knight | Palus Knight → Bladedancer 1; Assassin → Abyss Walker 2, Phantom Ranger 1 |
| Orc Fighter | Orc Raider → Destroyer | Orc Raider → Destroyer 2; Monk → Tyrant 1 |
| Dwarven Fighter | Scavenger → Bounty Hunter | Scavenger → Bounty Hunter (Artisan → Warsmith is the crafter role, M9) |
| Human Mystic | Cleric → Bishop 2, Prophet 1 | Human Wizard → Sorcerer |
| Elven Mystic | Elven Oracle → Elven Elder | Elven Wizard → Spellsinger |
| Dark Mystic | Shillien Oracle → Shillien Elder | Dark Wizard → Spellhowler |
| Orc Mystic | Orc Shaman → Warcryer | Orc Shaman → Warcryer (the Overlord serves a group, D23) |

The weights put the race's best fit first: dark elven STR to daggers and duals, elven DEX to
scouts and archers, orc CON to two-handed weapons. The branch is chosen as follows (D22):

- **A base-class bot** draws its branch from a hash of its account name and its role, so every
  session of the bot picks the same one, and its base row's weapon follows the branch from level 1.
- **A bot that already has a profession** keeps its line: its next profession is a child of its
  current class, from the role's table when the table names one there, else from the other role's,
  drawn by the same hash among the children. A class change is never undone: a bot that took
  today's `supportBuilds` line keeps its first profession, and at 40 its role's table picks the
  child (a support-preset Elven Knight goes Temple Knight, not Sword Singer; a support-preset
  Warrior, which the Support table does not name, goes Gladiator or Warlord by the hash).
- **A role change** (`SetPreset`) changes only the choices still ahead.
- **A pin**: the registry keeps an optional `Branch` (the first profession's class id) per bot,
  set from the panel (S9) and handed to the brain config, which S8 reads; a pinned branch wins over
  the hash while the bot is a base class.

Announced change (S8): a base-class fighter of the `support` preset that would take a melee line
today takes its race's tank line instead, and a support-preset Elven Knight or Palus Knight takes
Temple Knight or Shillien Knight at 40; a base-class mystic of a Damage-role preset (`merchant`,
which today takes the healer line, and `farmer`, S9) takes its race's damage line. A second profession at 40 plays its first profession's row until S13-S17
ship its own ("Second professions (levels 40+)").

### 7. Combat behavior per kit

- **Openers:** a pull, a debuff or a slow (Power Shot, Drain Health, Aggression, Wind Shackle,
  Curse: Poison, Entangle, Power Break, Cripple) only while the target is fresh (no attack step on it yet, as the `Opener` comment counts) and the
  bot's MP is above the step's bound.
- **Sustain:** the column-5 skills as steps gated by `SelfHPBelow`. `ruleCritical` tries the
  heals and drains among them: Self Heal, Elemental Heal, Divine Heal, Battle Roar (`HEAL_PERCENT`),
  Drain Health, Vampiric Touch and Life Drain. Body To Mind, Ultimate Defense and Ultimate Evasion
  stay in `fight`. `ruleCritical` runs before `fight` and drinks early when two or more
  attackers have the bot under `CrowdedHP` (`rules.go`), which is at or above every row's sustain
  bound; so `ruleCritical` tries the build's ready heal and drain steps whose gates pass before either
  of its potions (the crowded one and the critical one), and drinks only when none is ready.
- **Finishers and fillers:** Power Strike, Mortal Blow, Iron Punch, Wind Strike, Ice Bolt, Power
  Shot at their reuse, with the weapon gate.
- **Self-buffs:** Attack Aura and Defense Aura as self buffs out of combat (rule 7).
- **Range:** casters and archers fight from their cast or bow range (M7 stand points); melee kits
  walk in.
- **Mystic damage roles** rest on MP, keep a low `MPReserve` (10 %; 30 % for the orc mystic
  lines, whose Soul Cry and chants need MP in a fight), and melee only under `MPReserve`, except
  the armored orc mystics (OM Support, OS), which melee whenever no step is ready (the `Melee`
  policy is compiled per role); support roles keep 50 % for heals.

### 8. Dwarven spoil and sweep

A dwarf with the spoil flag casts Spoil as its opener on a fresh target. SystemMessage 612
(success) marks the monster spoiled by the bot, which then never casts Spoil on it again. A 357 (already
spoiled) on a monster without the bot's 612 means another player's spoil: the bot stops spoiling that monster and will not sweep it. 139 (resisted) counts only
while Spoil is the cast in flight (`castSkill`); then the bot tries again at Spoil's reuse while
the target lives. After a kill of a monster it spoiled it casts Sweeper on the corpse before
looting, from a new `sweep` rule right after `outnumbered`, so only the critical, heal and flee
rules go first: the corpse lasts twice its
template's `corpseTime` (14 s by default). The sweep is dropped on 343 or 683, on the corpse's
`DeleteObject`, or once the window is over. Swept materials are kept for the clan crafter (M9 keep
hook) or sold.

### 9. Toggles and Relax

The world model keeps the active toggles from `AbnormalStatusUpdate` (duration −1). A toggle is
turned on and off by casting it again, never while seated (the server refuses: SystemMessage 31),
and every one in the rows drains MP while on: Soul Cry 3 MP every 3 s at level 1, Accuracy 4.8 MP
every 5 s, Vicious Stance by level, Relax 2 MP every 2 s (`skills/1000-1099.xml` 1001,
`skills/0200-0299.xml` 226, 256, `skills/0300-0399.xml` 312). So:

- **Combat toggles** (Soul Cry, Accuracy, Vicious Stance) go on when the bot engages and its MP is above the
  row's bound, and off when MP falls under it, when the fight ends, and always before the bot sits.
- **Relax** replaces the plain sit for the rows that have it (human and orc fighter trees): out of
  combat, standing, below `SitHP` and with MP for it, the bot casts Relax, which sits it. It stands
  as today at `StandHP`, or the server ends Relax at full HP. A bot without the MP sits plainly.

Toggle tracking, the `ruleRest` change and Relax ship in S6; S7 adds Soul Cry.

### 10. Presets and roles

- A new `farmer` preset plays the Damage role and farms, and becomes the default `-persona` and the
  panel's first preset, so a bot made without a choice farms alone with a damage line (D21).
- `support` keeps the Support role for bots meant to join players' parties.
- `merchant` plays Damage.
- Imported bots (`botregistry.ImportPreset`, `support` before S9) get `farmer`, so an imported swarm
  farms alone as a fresh bot does. Bots already in the registry keep their stored preset; the
  panel's preset change moves them.
- `crafter` plays the Crafter role (M9).
- `town` keeps the Support role it plays today (`TownPersona` is `DefaultPersona` with idle
  settings): it farms nowhere, but learns, changes class, heals, buffs and fights back when
  attacked with that kit.
- The panel shows a bot's build (role, branch, kit) beside its class: `live.build` of
  `GET /api/bots` (role, branch and its name, the kit's skill ids), from the brain's published
  status. `POST /api/bots/{account}/branch` with `{"branch": <class id>}` pins a branch (the
  registry's `branch`), `0` or no branch clears it; `GET /api/classes` lists each base class's
  pinnable `branches`. A pin of another base class than the bot's is refused; an online bot whose
  pin changed logs in again with it.

### 11. Spot choice through the model

`SpotPolicy.fits` gains a model bound: a spot fits only while the model's `TimeToDie` stays above
`TimeToKill` times the row's margin (`ClassKit.SpotMargin`, column 10; `SpotPolicy.Margin`
replaces it), with the bot's potions counted. A tank kit takes spots a glass cannon passes over,
before any death there. This absorbs #294.

As shipped (#330), both times are averaged over the spot's monsters by their spawn totals, each
against one monster at a time: the likely pack (`spotPack`) enters only as the number of fights the
carried potions are shared over, since the pack is fought one monster after another. With every
monster of the pack hitting at once no margin of at least 1.5 would keep a level 6 squire at the
toads it farms live. The known limit is the simultaneous case: a bot that pulls more monsters than
it kills one at a time can still die at a spot the bound keeps (`TestLivePackSpot` died once at
the werewolves after pulling a third wolf); a death there still teaches the bot to pass over that
pack (`learnPack`).
With no stats yet, no monster with stats, or a cast on the target it has no travel map to price,
only the level bounds the spot.
The model prices risk, not reach: a spot whose monsters all fall to other players, or stay out of
reach, leaves the bot nothing to fight. The farmer takes farm targets anywhere in the spot's
territory, not only within the leash of its centre (#447), and moves on after
`SpotPolicy.Idle` (5 minutes) with none (#360).

#### Earning mode (#527)

A death penalty (skill 5076: at level 15 P./M.Atk and both speeds ×0.25, both defences ×0.4) can
leave a bot with no spot that passes the bound, of its level or under its window (`claimLower`),
and too poor for a Scroll: Recovery: it stood forever. When pickSpot finds nothing for a bot with
a death penalty that holds no spot, it claims an earning spot (`claimEarn`), under its cap, never
one it avoids:

1. By rate: among the spots in reach whose monsters stay within its level window
   (`SpotPolicy.Above`) and that no death taught it to pass over, the one it earns the most adena an
   hour at, by a survival bound of 1 instead of the row's margin with every monster counted, not
   only the aggressive ones. A kill is worth its CURRENCY drops at face value and its DROP items at
   half their reference price (what a merchant pays, server `inventory/sell.go`), by their chances
   and mean counts, cut by the server's level gap rule (`LevelPenaltyMultiplier`); the rate is that
   worth over the model's mean kill time. The spawn rate is left out.
2. Else the spot in reach with the lowest monsters, whatever its pack, the nearest among them.

It keeps that spot while its penalty lasts and looks every `spotRetry` for one that fits, which
ends the mode; once the penalty is gone the spot it holds is an ordinary one. What it earns goes to
Scrolls: Recovery first (#491: `recoveryWants`, the penalty's supply trip, rule `recover`). By the
model the level 5 orc mystic of #527 with penalty 15 kills nothing in time (`TestEarnShipped`), so
it takes step 2.

## Pacing

Nothing new is sent faster than today. Every cast still waits for its own `MagicSkillUse` or
`castTimeout`, then its skill's reuse; melee still clicks once per target; Spoil and Sweeper are
casts like any other. Trainer, class master and Symbol Maker visits keep `bypassGap` and their
reply timeouts. All of it goes through the per-connection frame bucket (`pacing.go`).

## Owner decisions

Approved by the owner on 2026-11-24, as proposed (also in `docs/plan.md`, "Owner decisions"):

| # | Question | Decision |
| --- | --- | --- |
| D21 | Which role does each preset play, and which is the default? | New `farmer`: Damage, and the new default for `-persona`, the panel and imported bots (`ImportPreset`); `support`: Support (healer and buffer mystics, tank-line fighters); `merchant`: Damage; `crafter`: Crafter; `town`: Support (as today; it farms nowhere). |
| D22 | How does a bot pick its profession branch? | Weighted by race fit (section 6), drawn from a hash of the account name and role while a base class; a bot with a profession keeps its line; the panel may pin a branch per bot (registry field). |
| D23 | Which professions are out of scope? | Servitor lines (Warlock, Elemental Summoner, Phantom Summoner) and Necromancer for its summons, a later milestone; the Overlord is not picked (a group class). |
| D24 | May the rewrite change today's behavior? | Yes, per PR: S4 lands behavior-neutral, every later PR states the behavior it changes. |

## Testing

- **Codec (unit):** `UserInfo` round trip and the recorded fixture keep the new fields and consume
  exactly `len(payload)`; the same for `EtcStatusUpdate` with its force charges (S6).
- **Datapack (unit):** fixture class, skill, NPC, trainer and armor set files; readers keep exactly
  their fields and fail on mangled values.
- **Model (unit, table-driven):** the stat curves against the server's values for the nine
  starting classes; Spellcraft and the orc masteries against fixed armor; a set's stat trade;
  calibration cancels an unknown buff; time to kill and to die on fixed templates.
- **Build (unit):** `TestBehaviorRowsShipped`: every row's skill is in its class's tree at the
  row's level, every bracket gate equals the skill's `weaponsAllowed` and every branch gate (`Dagger:`, `Bow:`) fits it, every profession is a direct
  child of its class, every set and dye row exists with the row's armor kind and class list; compile
  for each starting class and role; a class change recompiles; `Overrides` win over the build.
- **Brain, one test per row (unit, events and injected clock in, packets out),** named
  `TestRow<ID>`, each checking the row's key behavior:
  - HF: with a sword, Power Strike at its reuse and never Mortal Blow; with a bow, Power Shot from
    700 as the opener; below `SitHP` out of combat it casts Relax instead of sitting.
  - EF: below 50 % HP in combat, Elemental Heal (level 15+) goes out before rule 3's potion.
  - DF: Drain Health from 600 opens on a fresh target (level 15+); with Drain Health on reuse and
    no potion ready, two attackers make it flee at 60 % HP where HF fights on.
  - OF: with a DUALFIST weapon, Iron Punch (bare hands are FIST: never); with two attackers it keeps fighting down to 25 % HP.
  - WF: Spoil opens; after 612 and the kill, Sweeper goes out before any pick-up; after 357 it
    never sweeps.
  - HM: Damage role nukes Ice Bolt then Wind Strike down to 10 % MP; Support role stops at 50 %.
  - EM: in the Damage role, Wind Shackle opens above 50 % MP (level 14+); below 50 % HP it casts Self Heal.
  - DM: in the Damage role, Curse: Poison opens above 60 % MP; Vampiric Touch under 70 % HP.
  - OM: Soul Cry goes on at engage above 30 % MP and off before it sits (the light-armor pick of
    its Support role is S5's test below).
  - KN: Aggression opens from 400; Shield Stun only with a shield worn.
  - WA: with a two-handed blunt, Stun Attack then Power Smash; Wild Sweep only with a pole and two
    attackers; Accuracy and Vicious Stance off once MP falls under 30 % and before it sits.
  - RO: dagger branch casts Bleed above 50 % target HP, then Mortal Blow; bow branch never does.
  - EK: Elemental Heal under 50 % HP; Aggression opens.
  - ES: at 32-35 the bow branch opens with Power Shot from 700 and casts Rapid Shot in combat;
    from 36 Entangle opens for both branches.
  - PK: Sting with a sword; never with a blunt (gate).
  - AS: with a dagger, Drain Health pulls from 600, then Bleed and Mortal Blow.
  - OR: Frenzy under 30 % HP (level 32+); outnumbered it fights on down to 25 %.
  - MO: Focused Force charges out of combat only below its known level's maximum (1 charge at
    level 24-31); Force Blaster only with a charge, Hurricane Assault only with two.
  - SC: Spoil, Stun Attack with a blunt, then Sweeper.
  - CL, EO, SO: the leader under 35 % gets Battle Heal before anything else; Group Heal with two
    members under 60 %; SO shackles the assist target once.
  - HW, EW, DW: HW opens with Blaze from 750; EW and DW open with Wind Shackle or Curse: Poison
    from 600 above their MP bound, else their first ready 750 nuke (Solar Spark or Shadow Spark
    from 25, else Aqua Swirl or Twister);
    Energy Bolt (HW, EW) only within 400 and Aura Burn only within 150.
  - OS: Frost Flame opens from 750; Stun Attack only within 40 with a blunt; Chant of Life with a
    member under 50 %.
- **Brain (other):** a damage mystic and a support one of the same base class follow their own
  row parts (`MPReserve`, Sit MP, rotation, and the orc mystic's armor); an orc mystic picks
  light armor over a robe for the Support role and a human mystic never does; a robe top over light
  legs is never worn; a Demon set loses to Doom Robe for a low-HP dark mystic; a tank row fits the
  #294 spot that a glass-cannon row does not; a bot with a profession keeps its line under a new
  role; a pinned branch wins over the hash; the stack rule: at 36 an OR with Frenzy on never casts
  Guts, an OS holding Soul Shield level 2 casts no Chant of Shielding while no member qualifies,
  and casts it once a member in range has no `pd_up` effect, and a buff is renewed under `RebuffBefore` only over its own
  effect.
- **e2e (live harness), at most 4 bots a scenario:** three scenarios of three starting classes each
  farm for a set time with their kits; a level-20 class change takes the branch the account hash
  picks; a dwarf spoils and sweeps materials.

## Delivery (milestone M10, "Race and class strategy")

| PR | Scope | Depends on |
| --- | --- | --- |
| S1 (#320) | `UserInfo`: keep the base and combat stats in `Self` (section 1). | — |
| S2 (#321) | Datapack: class templates and skill trees, skill combat fields and armor-kind conditions, NPC base combat stats, base stats and `corpseTime`, trainers, set skills (section 2). | — |
| S3 (#322) | `internal/charmodel`: stat curves, masteries and penalties, sets, calibration, the NPC stat functions, time to kill and to die (section 3). | S1, S2 |
| S4 (#323) | The build: race profiles, class kits, roles, thresholds, `Overrides`, `CompileBuild`, the `Step` type with every gate, the `Buff` and `Heal` gates; the role from the preset as today's tables play it (`support` and `merchant` Support, `crafter` Crafter, `town` Support); swap every table for it, behavior-neutral; `TestBehaviorRowsShipped` over today's values, which S5-S11 extend to each row part they add (section 4, "Behavior per class" notation). | S2 |
| S5 (#324) | Gear, sets and dyes through the model; race armor kinds; chest and legs as a pair (section 5; columns 1, 2 and 11 of every row; a base row whose branches wield different weapons keeps today's weapon kinds until S8 sets `Build.Branch`). | S3, S4 |
| S6 (#325) | Fighter mechanics and base rows HF, EF, DF, OF, WF (columns 3-9 and 12; WF without Spoil and Sweeper, which S10 adds): the rotation runner with the `Step` gates, fighter learn tables and trainers, sustain in `ruleCritical`, the buff stack rule (self, party and member buffs), toggle tracking and the `ruleRest` change, Relax, the `Flee` skill, force charges decoded from `EtcStatusUpdate`, thresholds (sections 4, 7, 9). | S4 |
| S6b (#326) | Fighter first professions KN, WA, RO, EK, ES, PK, AS, OR, MO, SC (columns 3-9 and 12; SC without Spoil and Sweeper): their learn tables, rotations, sustain, buffs and toggles, Monk charges, thresholds. | S6 |
| S7 (#327) | Mystic rows HM, EM, DM, OM, CL, HW, EO, EW, SO, DW, OS (columns 3-9 and 12): learn tables, damage rotations and openers, heals and party buffs per row, sustain, per-role MP reserve and rest, Soul Cry (sections 4, 7, 9). | S4, S6 (rotation runner, toggle tracking) |
| S8 (#328) | Professions by race and role: `Build.Branch` and the branch choice (hash, kept line, and a pin read from the brain config), the branch's weapon kind for base rows (column 1), the class change at 20 and 40 moving a bot from its base row to its first-profession row, the announced change, `merchant` turning Damage (section 6). | S4, S6b, S7 |
| S9 (#331) | Presets and roles: the `farmer` preset as the default (`-persona`, the panel, `ImportPreset`), the registry `Branch` pin that feeds S8's brain config and its panel API, the panel's build view (section 10, D21, D22). | S8 |
| S10 (#329) | Dwarven spoil and sweep: rows WF and SC's Spoil opener, the `sweep` rule and Sweeper (section 8). | S2, S6b |
| S11 (#330) | Spot choice through the model, the spot margin of every row (column 10), absorbing #294 (section 11). | S3, S4 |
| S12 (#333) | e2e live scenarios for S5-S11 and S6b, at most 4 bots per scenario (the harness cap). | each |
| S13 (#587) | Second-profession gear and dyes (columns 1, 2, 9 and 11 of the 24 rows): C-grade sets, the 3-slot dye rows. | S5, S8 |
| S14 (#588) | Heavy fighter second professions PL, TK, SK, SS, GL, WL, DS, BD, BH (columns 3-8, 10 and 12): learn tables, rotations, songs and dances, the `(fixed)` tag. | S6b, S8 |
| S15 (#589) | Light fighter second professions TH, HK, PW, SR, AW, PR, TY (columns 3-8, 10 and 12): learn tables, rotations, Chameleon Rest from 46, the totem chain. | S6b, S8 |
| S16 (#590) | Mystic second professions BI, PP, EE, SE, SN, SG, SH, WC (columns 3-8, 10 and 12): learn tables, heals from a level, Recharge, member buffs. | S7, S8 |
| S17 (#591) | Cubics for TK and SK: `UserInfo` keeps the cubic ids, the cubic kept up as a buff. | S14 |
| Later (#592) | Servitor lines with summon control (D23). | — |

Most urgent: S1, S2 and S4 unblock everything; S6 and S6b give half the bots their skills.

## Risks

- **Size.** S4 and S5 rewrite the tables most brain tests use. S4 lands behavior-neutral with
  golden decision tests so the later PRs show their own changes. The fighter rows are split (S6:
  the shared mechanics and the five base rows; S6b: the ten professions); S7's eleven mystic rows
  share one rotation shape and the heal and buff tables, and may split the same way if review
  finds it too large.
- **Model error.** The model is approximate. It only compares candidates, and calibration against
  the reported stats cancels what it does not know on the bot's side; the monster side rests on
  copied NPC stat functions, tested on fixed templates. Thresholds are build defaults a persona
  may override.
- **Adena.** Orc mystic armor, sets and dyes cost more than robes and first sets; the M8 budget
  tiers still bound spending.
- **Variety versus fit.** Weighted branches leave some bots in weaker lines on purpose, as players
  do.

## Gaps

- Second-profession rows: S13-S17 (#587-#591); servitor professions: #592, a later milestone (D23).
- Kiting (step back from a melee monster) is out of scope.
