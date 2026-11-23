# Race and class strategy: each bot plays its race, class and role

Status: draft for owner review. Proposed decisions D21-D24 are open (see "Owner decisions").
Milestone M10 ("Race and class strategy"), epic and issues to be filed from the "Delivery"
table.

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
group), kiting, and second-profession kits (the backlog issue in "Delivery").

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

This section is the spec that S5 (gear and dyes, columns 1, 2 and 11), S6 (fighter rows), S7
(mystic rows), S8 (the professions, the branch weapon and the class change between rows), S10
(spoil and sweep) and S11 (spot margins, column 10) implement, and that the brain tests check row by row (section
"Testing"). It covers the nine base classes (levels 1-19) and every first profession (20-39) of
the branch table in Design section 6, for the Support and Damage roles. The servitor lines are out
(D23): Human, Elven and Dark Wizard keep their nukes here and never cast their summon skills.
Second professions are the backlog sub-table at the end.

Every skill below is in its class's skill tree (`classes/*.xml`, merged with the parents' as the
server does: a profession keeps the highest level of a parent's skill it learned), from the level
given; every weapon gate is the skill's `weaponsAllowed` (`skills/*.xml`); every profession is a
direct child of its class (`model/actor/player/class.go` `classParent`). A row's race reasons come
from the stat and kit tables above. `TestBehaviorRowsShipped` (S4) checks the tables against the
shipped datapack.

**Notation.** Skills are `id Name`. A weapon gate is in brackets, as the datapack's weapon kinds:
`[DAGGER]`. Gates: `@24` the level the step starts: the skill's first `minLvl` in the line's merged tree, and in a
profession row never below 20 (the row starts at 20, so a skill the base class taught earlier is
`@20`); `(base)` the same, for a skill the profession's own tree lists only later than 20 (it
knows it from its base class; its own tree only raises the level). `TestBehaviorRowsShipped`
checks `@N` = max(row start, first merged `minLvl`) and `(base)` = first merged `minLvl` under 20.
Several `open` steps: the first whose gates pass is the opener, and column 3 restates them. A step
marked (Support) or (Damage) is in that role's rotation only. A step whose skill's `castRange` is
shorter than the row's fight range gets that range as its `Within` gate from the datapack, written
or not (Ice Bolt and Wind Shackle at 600 for a 750 caster). Other gates: `open`
the opener (first action on a fresh target), `T≥50` target HP at least 50 %, `S<60` own HP under
60 %, `MP>30` own MP above 30 %, `≤400` target within 400 (`Step.Within`), `A≥2` at least two
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

Grades by level: no grade 1-19, D from 20 (Expertise), C from 40 (second professions, backlog).
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

### Combat (columns 3-7)

| Row | 3. Opener | 4. Rotation (priority order, gates) | 5. Sustain | 6. Self-buffs and toggles | 7. Rest |
| --- | --- | --- | --- | --- | --- |
| HF | Bow: 56 Power Shot from 700, a pull. Else walks in. | 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | none (potions, rule 3) | none | HP; 226 Relax @5 |
| EF | Bow: 56 Power Shot from 700, a pull. Else walks in. | 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | 58 Elemental Heal @15 S<50 | 91 Defense Aura @5, 77 Attack Aura @10, out of combat | HP; plain sit |
| DF | 70 Drain Health from 600 @15, a pull; before 15 as HF. | 70 Drain Health @15 open; 56 Power Shot [BOW] @5 MP>20; 16 Mortal Blow [DAGGER] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | 70 Drain Health @15 S<60 | 91 Defense Aura @5, 77 Attack Aura @10 | HP; plain sit |
| OF | Walks in. | 29 Iron Punch [DUALFIST] @5; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] @5; melee. | none | none (Toughness 134, Iron Body 295 are passive) | HP; 226 Relax @5 |
| WF | 254 Spoil @10, open, at melee range, until 612 (spoiled by the bot); 357 (another's spoil) ends it with no sweep; a 139 to Spoil retries at reuse. | 254 Spoil @10 open; melee. After the kill: 42 Sweeper @10 on the spoiled corpse (section 8). | none | none | HP; plain sit |
| HM | 1177 Wind Strike from 600, a pull. | Damage role: 1184 Ice Bolt @7; 1177 Wind Strike @1; melee only under MPReserve. Support role: heals and buffs first (Heals, Buffs), then the same nukes above MPReserve. | 1216 Self Heal @1 S<40; 1147 Vampiric Touch @14 S<60 | Support: 1068 Might @7, 1040 Shield @7 on itself and the party | MP; plain sit |
| EM | 1206 Wind Shackle @14 MP>50, from 600; before 14 1177 Wind Strike. | Damage: 1206 Wind Shackle @14 open MP>50; 1184 Ice Bolt @7; 1177 Wind Strike @1. Support: heals (1015 Battle Heal @14, 1011 Heal @7, 1027 Group Heal @14) and buffs, then nukes. | 1216 Self Heal @1 S<50 (no drain) | Support: Might, Shield | MP; plain sit |
| DM | 1168 Curse: Poison @7 MP>60, from 600; before 7 Wind Strike. | Damage: 1168 Curse: Poison @7 open MP>60; 1184 Ice Bolt @7; 1177 Wind Strike @1. Support: as HM. | 1147 Vampiric Touch @14 S<70; 1216 Self Heal @1 S<40 | Support: Might, Shield | MP; plain sit |
| OM | 1100 Chill Flame @7 from 600, a pull; before 7 walks in. | 1100 Chill Flame @7 open; 1090 Life Drain @7; melee. | 1090 Life Drain @7 S<70 | 1001 Soul Cry @1 (combat toggle, on while MP>30); 1010 Soul Shield @7; 1007 Chant of Battle @14 (party) | MP; plain sit |
| KN | 28 Aggression @24 from 400, a pull; before 24 70 Drain Health from 600. | 28 Aggression @24 open; 70 Drain Health @20 open (before Aggression's level); 92 Shield Stun [SHIELD] @20; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 70 Drain Health @20 S<60; 45 Divine Heal @28 S<40; 110 Ultimate Defense @20 S<20 | 82 Majesty @20 | HP; 226 Relax (base tree) |
| WA | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20; 255 Power Smash [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @20; 245 Wild Sweep [POLE] @20 A≥2; melee. | 121 Battle Roar @28 S<30 | 78 War Cry @20 at a fight's start; 256 Accuracy @24 and 312 Vicious Stance @20 (combat toggles, on while MP>30) | HP; 226 Relax (base tree) |
| RO | Bow: 56 Power Shot from 700, a pull (101 Stunning Shot from 900 @36). Dagger: walks in. | Dagger: 96 Bleed [DAGGER] @24 T≥50; 16 Mortal Blow [DAGGER] @20. Bow: 101 Stunning Shot [BOW] @36; 56 Power Shot [BOW] @20. Melee between. | 111 Ultimate Evasion @28 S<30 | 99 Rapid Shot @32 (bow, in combat); 256 Accuracy @24 (in combat, MP>30); 312 Vicious Stance @20 (in combat, MP>30); 4 Dash @20 only to flee | HP; 226 Relax (base tree) |
| EK | 28 Aggression @24 from 400, a pull; before 24 walks in. | 28 Aggression @24 open; 102 Entangle @36 open; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 58 Elemental Heal @20 S<50; 110 Ultimate Defense @20 S<20 | 91 Defense Aura (base), 77 Attack Aura (base; level 2 @28) | HP; plain sit |
| ES | Bow: 56 Power Shot from 700 (101 Stunning Shot from 900 @36). Dagger: walks in. | 102 Entangle @36 open; dagger: 96 Bleed [DAGGER] @24 T≥50, 16 Mortal Blow [DAGGER] @20; bow: 101 Stunning Shot [BOW] @36, 56 Power Shot [BOW] @20; melee. | 58 Elemental Heal @20 S<50; 111 Ultimate Evasion @28 S<30 | Auras 91, 77 (base); 99 Rapid Shot @32 (bow); 256 Accuracy @24 (in combat, MP>30); 312 Vicious Stance @20 (in combat, MP>30); 230 Sprint @32 only to flee | HP; plain sit |
| PK | 70 Drain Health @20 from 600, a pull; 28 Aggression @24 for the Support role. | 28 Aggression @24 open (Support); 70 Drain Health @20 open; 115 Power Break @32 open; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24; 105 Freezing Strike @36; 3 Power Strike [SWORD, BLUNT, BIGSWORD, BIGBLUNT] (level 9); melee. | 70 Drain Health @20 S<60; 110 Ultimate Defense @20 S<20 | 91 Defense Aura (base), 77 Attack Aura (base; level 2 @28) | HP; plain sit |
| AS | Dagger: 70 Drain Health @20 from 600, a pull. Bow: 56 Power Shot from 700 (101 Stunning Shot from 900 @36). | Dagger: 70 Drain Health @20 open; 96 Bleed [DAGGER] @24 T≥50; 16 Mortal Blow [DAGGER] @20; 223 Sting [SWORD, DAGGER, DUAL, BIGSWORD] @24. Bow: 101 Stunning Shot [BOW] @36; 56 Power Shot [BOW] @20. Melee between. | 70 Drain Health @20 S<60; 111 Ultimate Evasion @28 S<30 | Auras 91, 77 (base); 99 Rapid Shot @32 (bow); 256 Accuracy @24 (in combat, MP>30); 312 Vicious Stance @20 (in combat, MP>30) | HP; plain sit |
| OR | Walks in. | 100 Stun Attack [BLUNT, BIGBLUNT] @20; 255 Power Smash [SWORD, BLUNT, BIGBLUNT, BIGSWORD] @20; melee. | 121 Battle Roar @28 S<30 | 94 Rage @24 in combat; 176 Frenzy @32 S<30; 139 Guts @36 S<30; 256 Accuracy @24 (in combat, MP>30); 312 Vicious Stance @20 (in combat, MP>30) | HP; 226 Relax (base tree) |
| MO | 54 Force Blaster [DUALFIST] @24 C≥1 from 600, a pull; else walks in. | 50 Focused Force [DUALFIST] @24 C<max out of combat (1 charge at 24-31, 2 from 32); 95 Cripple [FIST, DUALFIST] @20 open; 284 Hurricane Assault [DUALFIST] @36 C≥2; 54 Force Blaster [DUALFIST] @24 C≥1; 120 Stunning Fist [DUALFIST] @20; 29 Iron Punch [DUALFIST] @20; melee. | none | 83 Wolf Spirit Totem [DUALFIST] @20, then 76 Bear Spirit Totem [DUALFIST] @28 | HP; 226 Relax (base tree) |
| SC | 254 Spoil @20 (levels 2-4), open, at melee range, until spoiled. | 254 Spoil open; 100 Stun Attack [BLUNT, BIGBLUNT] @20; melee. After the kill: 42 Sweeper (section 8). | none | none | HP; plain sit |
| CL | Support: none, it assists. Alone: 1177 Wind Strike (level 5) from 600. | Heals: 1015 Battle Heal @20 under 35 %, 1011 Heal @20 under 60 %, 1027 Group Heal @20 with two members under 60 %. Then 1177 Wind Strike above MPReserve; melee only alone and out of MP. | Its heals on itself (rule 3 order: leader, self, others) | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1085 Acumen @20 (mystic members), 1078 Concentration @20 | MP; plain sit |
| HW | 1220 Blaze @20 from 750, a pull. | 1220 Blaze @20; 1184 Ice Bolt @20 (levels 5-6); 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150; melee only out of MP. | 1147 Vampiric Touch @20 S<60; 1157 Body To Mind @25 when MP under 30 % and S≥70 | 1078 Concentration @20 | MP; plain sit |
| EO | Support: none, it assists. Alone: 1177 Wind Strike from 600. | Heals as CL. Then 1201 Dryad Root @25 on a second attacker A≥2; 1177 Wind Strike above MPReserve. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1087 Agility @25, 1043 Holy Weapon @25 | MP; plain sit |
| EW | 1206 Wind Shackle @20 MP>50 from 600, else 1175 Aqua Swirl from 750. | 1206 Wind Shackle open MP>50; 1264 Solar Spark @25; 1175 Aqua Swirl @20; 1184 Ice Bolt @20; 1274 Energy Bolt @20 ≤400; 1172 Aura Burn @20 ≤150. | 1216 Self Heal (base, level 1) S<40 (no drain) | 1078 Concentration @20 | MP; plain sit |
| SO | Support: none, it assists; 1206 Wind Shackle @20 on the leader's target once. Alone: Wind Strike. | Heals as CL; 1206 Wind Shackle on the assist target open; 1177 Wind Strike above MPReserve. | Its heals on itself | Party: 1068 Might, 1040 Shield (base), 1204 Wind Walk @20, 1059 Greater Empower @25, 1077 Focus @25, 1268 Vampiric Rage @30 | MP; plain sit |
| DW | 1168 Curse: Poison @20 MP>60 from 600, else 1178 Twister from 750. | 1168 Curse: Poison open MP>60; 1266 Shadow Spark @25; 1178 Twister @20; 1184 Ice Bolt @20; 1172 Aura Burn @20 ≤150. | 1147 Vampiric Touch @20 S<70; 1157 Body To Mind @25 when MP under 30 % and S≥70 | 1078 Concentration @20 | MP; plain sit |
| OS | 1107 Frost Flame @20 from 750, a pull. | 1107 Frost Flame @20 open; 1090 Life Drain @20 (levels 3-6); 100 Stun Attack [BLUNT, BIGBLUNT] @20 ≤40, else melee. | 1090 Life Drain S<70; 1229 Chant of Life @20 (party) with a member under 50 % | 1001 Soul Cry (base; combat toggle, on while MP>30); party chants 1007 Chant of Battle (base), 1009 Chant of Shielding @20, 1006 Chant of Fire @20, 1002 Flame Chant @30; 1010 Soul Shield (base) | MP; plain sit |

### Numbers and summary (columns 8, 10, 12)

Thresholds follow the HP multiplier (CON) and the effective run speed of the stat table: the
higher the CON, the later the bot drinks, flees from a crowd or sits; the faster it runs, the more
a flee works (`FleeOutrun` on for every row, so a slow bot fights on against a faster attacker
rather than run, #281). `MPReserve` gates attack steps only; sustain and heals run down to their
own cost. Where a row has both roles, values are Support / Damage. Spot margin: a spot fits while
`TimeToDie ≥ margin × TimeToKill` against its likely pack (Design section 11); tank 1.5, normal
2.0, glass cannon 3.0.

| Row | MPReserve | Sit HP / MP | Stand HP / MP | Critical | Crowded | Outmatched | 10. Spot | 12. Plays like |
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

### Second professions (backlog, levels 40+)

A known gap: these classes play their first profession's row (kit, thresholds, dyes direction)
until the backlog issue adds their own kit. Weapons and armor below are what their gear and the
C-grade sets would be; their skills are not specified here.

| Class (id) | Parent | Weapon | Armor pair; C-grade sets |
| --- | --- | --- | --- |
| Paladin (5) | KN | SWORD or BLUNT with a shield | Heavy; Full Plate (+270 HP), Composite (weight) |
| Gladiator (2) | WA | DUAL | Heavy; Full Plate |
| Warlord (3) | WA | POLE | Heavy; Full Plate |
| Treasure Hunter (8) | RO | DAGGER | Light; Plated Leather (+4 STR −1 CON), Mithril Light (+4 evasion) |
| Hawkeye (9) | RO | BOW | Light; Plated Leather |
| Temple Knight (20) | EK | SWORD with a shield | Heavy; Full Plate |
| Sword Singer (21) | EK | SWORD with a shield | Heavy; Full Plate |
| Plains Walker (23) | ES | DAGGER | Light; Plated Leather, Mithril Light |
| Silver Ranger (24) | ES | BOW | Light; Plated Leather |
| Shillien Knight (33) | PK | SWORD with a shield | Heavy; Full Plate |
| Bladedancer (34) | PK | DUAL | Heavy; Full Plate |
| Abyss Walker (36) | AS | DAGGER | Light; Plated Leather, Mithril Light |
| Phantom Ranger (37) | AS | BOW | Light; Plated Leather |
| Destroyer (46) | OR | BIGSWORD or BIGBLUNT | Heavy; Full Plate |
| Tyrant (48) | MO | DUALFIST | Light; Plated Leather |
| Bounty Hunter (55) | SC | BLUNT with a shield | Heavy; Composite (+5795 weight), Full Plate |
| Bishop (16) | CL | BLUNT/MAGIC or SWORD/MAGIC | Robe; Divine (+171 MP, +1 WIT −1 INT), Karmian (×1.15 cast) |
| Prophet (17) | CL | BLUNT/MAGIC or SWORD/MAGIC | Robe; Karmian, Divine |
| Sorcerer (12) | HW | BLUNT/MAGIC or SWORD/MAGIC | Robe; Demon (+4 INT, −270 HP) once its HP covers the loss, else Karmian |
| Elven Elder (30) | EO | BLUNT/MAGIC or SWORD/MAGIC | Robe; Divine, Karmian |
| Spellsinger (27) | EW | BLUNT/MAGIC or SWORD/MAGIC | Robe; Karmian, Demon |
| Shillien Elder (43) | SO | BLUNT/MAGIC or SWORD/MAGIC | Robe; Divine, Karmian |
| Spellhowler (40) | DW | BLUNT/MAGIC or SWORD/MAGIC | Robe; Karmian (the −270 HP of Demon is a third of its HP at 40) |
| Warcryer (52) | OS | BLUNT/MAGIC | Heavy; Full Plate, Composite |

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
  `weaponsAllowed`, `reuseDelay`, `hitTime`, `mpConsume` and `power` per level, and the armor-kind
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
  MP with a survival floor. A role's weights are persona defaults (D6).

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
  rotation, self-buffs, party buffs and heals for support roles, toggles, the fight range (melee,
  cast range, bow range), rest by HP or MP, and the dwarven spoil flag (section 8).
- **Rotation:** an ordered list of steps; the first whose gates pass is cast, else melee:

  ```go
  type Step struct {
  	Skill   int32
  	Weapons []string // the worn weapon's kind must be one; none for any
  	// The step applies only while the target's HP percent is at least MinTargetHP
  	// (a finisher sets MaxTargetHP), the bot's HP is under SelfHPBelow (sustain),
  	// and its MP is above MPAbove.
  	MinTargetHP, MaxTargetHP, SelfHPBelow, MPAbove int
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
  	Opener bool // only as the first action on a fresh target (once per target; War Cry, a self step, at a fight's start)
  	Role   Role // zero for every role; else only in that role's rotation
  }
  ```

  A zero field is no gate (`SelfHPBelow` 0 casts at any HP, `MaxTargetHP` 0 at any target HP).
  `Within` defaults to the skill's `castRange` when that is shorter than the kit's fight range.
  A target whose HP the bot has not seen yet (no `StatusUpdate` for it) counts as full. Weapons
  come from the worn weapon's datapack kind, known to the gear errand, so a step the server would
  refuse is never sent. "Behavior per class" fills these fields per row.
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
- **The rules stay.** `playRules` keeps its order; `fight`, `ruleHeal`, `ruleRebuff`, `ruleRest`,
  `ruleTrain`, `ruleClassChange`, `ruleGear` and `ruleDyes` read the build instead of the
  persona's tables.
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
which today takes the healer line; `farmer` once S9 adds it) takes its race's damage line. The second-profession kits at 40 take the first profession's row
until the backlog issue adds theirs.

### 7. Combat behavior per kit

- **Openers:** a pull, a debuff or a slow (Power Shot, Drain Health, Aggression, Wind Shackle,
  Curse: Poison, Entangle, Power Break, Cripple) only while the target is fresh and the bot's MP is above the step's bound.
- **Sustain:** Self Heal, Elemental Heal, Drain Health, Vampiric Touch and Life Drain as steps
  gated by `SelfHPBelow`. `ruleCritical` runs before `fight` and drinks early when two or more
  attackers have the bot under `CrowdedHP` (`rules.go`), which is above every row's sustain
  bound; so `ruleCritical` tries the build's ready sustain steps whose gates pass before either
  of its potions (the crowded one and the critical one), and drinks only when none is ready. This
  is the one change to a rule's body; the rule order stays.
- **Finishers and fillers:** Power Strike, Mortal Blow, Iron Punch, Wind Strike, Ice Bolt, Power
  Shot at their reuse, with the weapon gate.
- **Self-buffs:** Attack Aura and Defense Aura as self buffs out of combat (rule 7).
- **Range:** casters and archers fight from their cast or bow range (M7 stand points); melee kits
  walk in.
- **Mystic damage roles** rest on MP, keep a low `MPReserve` (10 %; 30 % for the orc mystic
  lines, whose Soul Cry and chants need MP in a fight), and melee only when out of MP; support
  roles keep 50 % for heals.

### 8. Dwarven spoil and sweep

A dwarf with the spoil flag casts Spoil as its opener on a fresh target. SystemMessage 612
(success) marks the monster spoiled by the bot, which then never casts Spoil on it again. A 357 (already
spoiled) on a monster without the bot's 612 means another player's spoil: the bot stops spoiling that monster and will not sweep it. 139 (resisted) counts only
while Spoil is the cast in flight (`castSkill`); then the bot tries again at Spoil's reuse while
the target lives. After a kill of a monster it spoiled it casts Sweeper on the corpse before
looting, ahead of every other rule but the flee and heal rules: the corpse lasts twice its
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

Toggle tracking ships with the first toggles (S6); S7 adds Soul Cry.

### 10. Presets and roles

- A new `farmer` preset plays the Damage role and farms, and becomes the default `-persona` and the
  panel's first preset, so a bot made without a choice farms alone with a damage line (D21).
- `support` keeps the Support role for bots meant to join players' parties.
- `merchant` plays Damage.
- Imported bots (`botregistry.ImportPreset`, today `support`) get `farmer`, so an imported swarm
  farms alone as a fresh bot does. Bots already in the registry keep their stored preset; the
  panel's preset change moves them.
- `crafter` plays the Crafter role (M9).
- `town` plays no fight role: it never fights, so it compiles no rotation.
- The panel shows a bot's build (role, branch, kit) beside its class.

### 11. Spot choice through the model

`SpotPolicy.fits` gains a model bound: a spot fits only while the model's `TimeToDie` against its
likely pack (`spotPack`) stays above `TimeToKill` times a persona margin, with the bot's potions
counted. A tank kit takes spots a glass cannon passes over, before any death there. This absorbs
#294.

## Pacing

Nothing new is sent faster than today. Every cast still waits for its own `MagicSkillUse` or
`castTimeout`, then its skill's reuse; melee still clicks once per target; Spoil and Sweeper are
casts like any other. Trainer, class master and Symbol Maker visits keep `bypassGap` and their
reply timeouts. All of it goes through the per-connection frame bucket (`pacing.go`).

## Owner decisions

Proposed, awaiting the owner:

| # | Question | Proposal |
| --- | --- | --- |
| D21 | Which role does each preset play, and which is the default? | New `farmer`: Damage, and the new default for `-persona`, the panel and imported bots (`ImportPreset`); `support`: Support (healer and buffer mystics, tank-line fighters); `merchant`: Damage; `crafter`: Crafter; `town`: none (no fights). |
| D22 | How does a bot pick its profession branch? | Weighted by race fit (section 6), drawn from a hash of the account name and role while a base class; a bot with a profession keeps its line; the panel may pin a branch per bot (registry field). |
| D23 | Which professions are out of scope? | Servitor lines (Warlock, Elemental Summoner, Phantom Summoner) and Necromancer for its summons, a later milestone; the Overlord is not picked (a group class). |
| D24 | May the rewrite change today's behavior? | Yes, per PR: S4 lands behavior-neutral, every later PR states the behavior it changes. |

## Testing

- **Codec (unit):** `UserInfo` round trip and the recorded fixture keep the new fields and consume
  exactly `len(payload)`.
- **Datapack (unit):** fixture class, skill, NPC, trainer and armor set files; readers keep exactly
  their fields and fail on mangled values.
- **Model (unit, table-driven):** the stat curves against the server's values for the nine
  starting classes; Spellcraft and the orc masteries against fixed armor; a set's stat trade;
  calibration cancels an unknown buff; time to kill and to die on fixed templates.
- **Build (unit):** `TestBehaviorRowsShipped`: every row's skill is in its class's tree at the
  row's level, every weapon gate equals the skill's `weaponsAllowed`, every profession is a direct
  child of its class, every set and dye row exists with the row's armor kind and class list; compile
  for each starting class and role; a class change recompiles; `Overrides` win over the build.
- **Brain, one test per row (unit, events and injected clock in, packets out),** named
  `TestRow<ID>`, each checking the row's key behavior:
  - HF: with a sword, Power Strike at its reuse and never Mortal Blow; with a bow, Power Shot from
    700 as the opener; below `SitHP` out of combat it casts Relax instead of sitting.
  - EF: below 50 % HP in combat, Elemental Heal (level 15+) goes out before rule 3's potion.
  - DF: Drain Health from 600 opens on a fresh target (level 15+); with two attackers it flees at
    60 % HP where HF fights on.
  - OF: with a DUALFIST weapon, Iron Punch (bare hands are FIST: never); with two attackers it keeps fighting down to 25 % HP.
  - WF: Spoil opens; after 612 and the kill, Sweeper goes out before any pick-up; after 357 it
    never sweeps.
  - HM: Damage role nukes Ice Bolt then Wind Strike down to 10 % MP; Support role stops at 50 %.
  - EM: Wind Shackle opens above 50 % MP (level 14+); below 50 % HP it casts Self Heal.
  - DM: Curse: Poison opens above 60 % MP; Vampiric Touch under 70 % HP.
  - OM: Soul Cry goes on at engage above 30 % MP and off before it sits (the light-armor pick of
    its Support role is S5's test below).
  - KN: Aggression opens from 400; Shield Stun only with a shield worn.
  - WA: with a two-handed blunt, Stun Attack then Power Smash; Wild Sweep only with a pole and two
    attackers; Accuracy and Vicious Stance off once MP falls under 30 % and before it sits.
  - RO: dagger branch casts Bleed above 50 % target HP, then Mortal Blow; bow branch never does.
  - EK: Elemental Heal under 50 % HP; Aggression opens.
  - ES: bow branch opens with Power Shot from 700 and casts Rapid Shot in combat (level 32+).
  - PK: Sting with a sword; never with a blunt (gate).
  - AS: with a dagger, Drain Health pulls from 600, then Bleed and Mortal Blow.
  - OR: Frenzy under 30 % HP (level 32+); outnumbered it fights on down to 25 %.
  - MO: Focused Force charges out of combat only below its known level's maximum (1 charge at
    level 24-31); Force Blaster only with a charge, Hurricane Assault only with two.
  - SC: Spoil, Stun Attack with a blunt, then Sweeper.
  - CL, EO, SO: the leader under 35 % gets Battle Heal before anything else; Group Heal with two
    members under 60 %; SO shackles the assist target once.
  - HW, EW, DW: HW opens with Blaze from 750; EW and DW open with Wind Shackle or Curse: Poison
    from 600 above their MP bound, else Aqua Swirl or Twister from 750; Ice Bolt only within 600;
    Energy Bolt (HW, EW) only within 400 and Aura Burn only within 150.
  - OS: Frost Flame opens from 750; Stun Attack only within 40 with a blunt; Chant of Life with a
    member under 50 %.
- **Brain (other):** a damage mystic and a support one of the same base class follow their own
  row parts (`MPReserve`, Sit MP, rotation, and the orc mystic's armor); an orc mystic picks
  light armor over a robe for the Support role and a human mystic never does; a robe top over light
  legs is never worn; a Demon set loses to Doom Robe for a low-HP dark mystic; a tank row fits the
  #294 spot that a glass-cannon row does not; a bot with a profession keeps its line under a new
  role; a pinned branch wins over the hash.
- **e2e (live harness), at most 4 bots a scenario:** three scenarios of three starting classes each
  farm for a set time with their kits; a level-20 class change takes the branch the account hash
  picks; a dwarf spoils and sweeps materials.

## Delivery (milestone M10, "Race and class strategy")

| PR | Scope | Depends on |
| --- | --- | --- |
| S1 | `UserInfo`: keep the base and combat stats in `Self` (section 1). | — |
| S2 | Datapack: class templates and skill trees, skill combat fields and armor-kind conditions, NPC base combat stats, base stats and `corpseTime`, trainers, set skills (section 2). | — |
| S3 | `internal/charmodel`: stat curves, masteries and penalties, sets, calibration, the NPC stat functions, time to kill and to die (section 3). | S1, S2 |
| S4 | The build: race profiles, class kits, roles, thresholds, `Overrides`, `CompileBuild`, the `Step` type with every gate; swap every table for it, behavior-neutral; `TestBehaviorRowsShipped` over today's values, which S5-S11 extend to each row part they add (section 4, "Behavior per class" notation). | S2 |
| S5 | Gear, sets and dyes through the model; race armor kinds; chest and legs as a pair (section 5; columns 1, 2 and 11 of every row; a base row whose branches wield different weapons keeps today's weapon kinds until S8 sets `Build.Branch`). | S3, S4 |
| S6 | Fighter rows HF, EF, DF, OF, WF, KN, WA, RO, EK, ES, PK, AS, OR, MO, SC (columns 3-9 and 12; WF and SC without their Spoil and Sweeper steps, which S10 adds): learn tables, weapon-gated rotations with the `Step` gates, sustain, self-buffs, toggle tracking, Relax, force charges from `EtcStatusUpdate` (Monk), thresholds (sections 4, 7, 9). | S4 |
| S7 | Mystic rows HM, EM, DM, OM, CL, HW, EO, EW, SO, DW, OS (columns 3-9 and 12): learn tables, damage rotations and openers, heals and party buffs per row, sustain, per-role MP reserve and rest, Soul Cry (sections 4, 7, 9). | S4, S6 (toggle tracking) |
| S8 | Professions by race and role: the roles of today's presets (`support` Support, `merchant` Damage, `crafter` Crafter), `Build.Branch` and the branch choice (hash, kept line, and a pin read from the brain config), the branch's weapon kind for base rows (column 1), the class change at 20 and 40 moving a bot from its base row to its first-profession row, the announced change (section 6). | S4, S6, S7 |
| S9 | Presets and roles: the `farmer` preset as the default (`-persona`, the panel, `ImportPreset`), the registry `Branch` pin that feeds S8's brain config and its panel API, the panel's build view (section 10, D21, D22). | S8 |
| S10 | Dwarven spoil and sweep: rows WF and SC's Spoil opener and Sweeper (section 8). | S2, S6 |
| S11 | Spot choice through the model, the spot margin of every row (column 10), absorbing #294 (section 11). | S3, S4 |
| S12 | e2e live scenarios for S5-S11, at most 4 bots per scenario (the harness cap). | each |
| — | Backlog: second-profession kits (40+, the backlog sub-table) and servitor lines. | S8 |

Most urgent: S1, S2 and S4 unblock everything; S6 gives half the bots their skills.

## Risks

- **Size.** S4 and S5 rewrite the tables most brain tests use. S4 lands behavior-neutral with
  golden decision tests so the later PRs show their own changes.
- **Model error.** The model is approximate. It only compares candidates, and calibration against
  the reported stats cancels what it does not know on the bot's side; the monster side rests on
  copied NPC stat functions, tested on fixed templates. Thresholds are build defaults a persona
  may override.
- **Adena.** Orc mystic armor, sets and dyes cost more than robes and first sets; the M8 budget
  tiers still bound spending.
- **Variety versus fit.** Weighted branches leave some bots in weaker lines on purpose, as players
  do.

## Gaps

- Servitor professions and second-profession kits at 40+: the backlog issue.
- Kiting (step back from a melee monster) is out of scope.
