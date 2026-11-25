# Wire, datapack and world model

Ships in X1 (wire), X2 (datapack), X4 (world model, changes to today's code).

## Wire (X1)

Each decoder and encoder has a recorded or server-built fixture and an exact `len(payload)` test.

- `ExDuelAskStart`, `ExDuelReady`, `ExDuelStart`, `ExDuelEnd`, `ExDuelUpdateUserInfo` (0xfe subs
  0x004b-0x004f); `RequestDuelStart`, `RequestDuelAnswerStart` (0xd0 subs 0x0027, 0x0028). No
  `RequestDuelSurrender` (D39).
- `RequestTargetCancel` (0x37, `unselect` 0): the Esc cast cancel ([guards](guards.md)).
- `MagicSkillCanceled` (0x49: caster object id), broadcast to the caster and watchers when a cast
  stops, Esc included (`serverpackets/magic_skill.go:15-16`, `:78-82`, `FrameMagicSkillCanceled`;
  `network/magic_skill.go:765-779`, `broadcastCastAborted`; `network/targeting.go:898-906`); absent
  from `opcodes.go` today. As a typed event it ends the bot's own cast (`castEnd` cleared) or clears
  another caster's `LastCast`; the guards read it to tell a cancel from a late Esc.
- `CharInfo` keeps hand template ids, collision radius, clan and ally ids, abnormal mask, max CP
  and CP, duel team; `NPCInfo` its collision radius and title (a summon's owner); `UserInfo` the
  bot's collision radius, ally id, PK and PvP kills. Dropped today: `UserInfo` at `inworld.go:56`,
  `:62`, `:66`; `CharInfo` at `:127`, `:137`, `:140`, `:148`, `:150-151`; `NPCInfo` at `:171`,
  `:183`, its title read at `:178` but not kept by `applyNPCInfo` (`world.go:888-900`).
- SystemMessages: 35, 36 (damage dealt, taken), 139 (resisted), 283, 284 (combat zone in, out:
  an arena or a running siege), 84, 85 (peace refusals), 692 (frozen duel opponent), 153 (busy), the duel set 1926-2028.

## Datapack (X2)

- A zone reader for arena, peace, town, Derby track, siege (`SiegeZone.xml`: 15 `NPoly`, 9 castle
  and 6 clan hall battlefields) and no-restart zones: `NPoly` (all but one) and `Cuboid` (Rune
  Castle Town, `zones/TownZone.xml:445`, corners and Z band normalized as `NewCuboid` does); any
  other shape fails the load. Water stays on `LoadWaterZones` (`internal/datapack/zones.go:34-60`,
  every shipped water zone a `Cuboid`).
- Skill fields: `isDebuff`, `isMagic`, `lvlDepend`, `magicLvl`, effect powers and types,
  `abnormalLvl`, `effectAbnormalLvl` (both default -1), stack types, `nextActionAttack`, `offensive`
  (defaulted from skill type and `isDebuff`, `model/skill/definition.go:443-446`), target types,
  `skillRadius`; cure negate fields; vulnerability stat functions of jewels, sets and active `Buff`
  effects (Lionheart 287, Mental Shield 1035); weapon `pvp*Dmg`; `cpTable`.

## World model (X4)

The **opponent record**, per player in sight, owned by the brain goroutine: `Archetype`
([model](model.md#archetypes)); `Tier` (0 base class to 3 third profession); `ClanID`, `AllyID`;
`Abnormal` (the `CharInfo` mask); `FlagBlinkAt` (when its flag was last seen turn to 2, by
`RelationChanged` or a `CharInfo` with flag 2, first sight included; zero while steady or gone);
`HP`, `MaxHP` (selection `StatusUpdate` or duel window), `CP`, `MaxCP` (`CharInfo` or duel window),
minus damage dealt since, and `SeenAt` (the [pool estimate](layer.md#cp-cures-pool-break-off));
`LastCast` (skill, target, landing time = arrival + hit time); `Kind` (`duel`, `fight_back`,
`arena`, `flagged`, `pk`, set by the rule that allowed the fight, or `unfought` for an attacker
`MayFight` refuses; [metrics](modes.md#panel-and-metrics-x12)).

- **Duel state**: `idle → asked` (`ExDuelAskStart`, or its own challenge) `→ countdown` (1930 or
  1929) `→ fighting` (`ExDuelStart`, the opponent's first `ExDuelUpdateUserInfo`, carrying its
  object id, then the reaction delay) `→ idle` (`ExDuelEnd`, 1950-1956, 1931, a refusal 153, 1926,
  1940, 2017-2028, or 15 s unanswered). A result (1950-1956) or a 692 holds the duel `over`, nobody
  fought, until its `ExDuelEnd`, which the server sends in the same step (`network/duel.go:433-445`);
  a challenge refusal ends only an open challenge (153 also answers other requests). The code is
  `internal/botclient/pvp_world.go`. The PvP [reaction](#changes-to-todays-code-x4) covers the
  gap to the bot's own start step, queued by the same loop (`network/duel.go:364-395`, `:396-399`; `duel/state.go:82-86`).
  Only while fighting is the opponent attackable with a plain `Action`.
- **Arena state**: the bot's position in an arena polygon (X2; 283 and 284 come from sieges too).
  Another player in the same arena is attackable without force by [rule 3](targets.md#mayfight). The server reads positions
  live at every attack and at death, so the bot keeps its own and each arena opponent's edge
  distance (from `CharInfo` and moves).

## Changes to today's code (X4)

- **Forced attacks.** `forced()` (`combat_rules.go:251-254`; ctrl casts `:214`, `AttackRequest`
  `:228-230`) becomes "not attackable without force": not the duel opponent while fighting, not in
  the same arena, not flagged or with karma; those get plain `Action` and casts.
- **Hostile casts.** `hostileCaster` (`combat_world.go:115-120`; today only from flagged or karma
  players, and any `MagicSkillUse` at the bot, beneficial ones included, `:107-113`) becomes: an
  `offensive` skill whose `MagicSkillUse` names the bot, from any player, no test of the caster or
  `MayFight`. The server lets it reach the bot only from a player who may attack it
  ([attack without force, forcing](server-karma.md)), so it is an attack like every `Attack` hit (`applyAttack`,
  `combat_world.go:74-77`). `MayFight` then decides: a karma bot flees a clean mystic's nukes
  ([karma](karma.md#with-karma)), a flagged bot's nukes are [rule 5](targets.md#mayfight)'s first
  hit, a countdown cast goes unanswered. Heals and buffs are no attack; summons' casts count the
  same way.
- **Blink on arrival.** A `CharInfo` with flag state 2 (`serverpackets/charinfo.go:81`, `:85`;
  decoded at `inworld.go:131`) sets `FlagBlinkAt` too.
- **Support gate.** `MaySupport` (`combat_rules.go:71-73`) becomes
  `SupportFlagged || (o.PvPFlag == 0 && o.Karma == 0)` (supporting a karma player flags the caster
  too, `character_pvpflag.go:88-93`, `skillTargetFlagsPvP`); in the bot's own duel `SupportFlagged`
  counts as off and the duel opponent is never supported ([duel support](guards.md#duel-support));
  in a PvP zone nothing flags the bot, so it passes everyone.
- **Summons and pets.** A summon (`KindNPC`, `NPCInfo` with `Summon`, `world.go:888-900`) is taken
  by `o.Attackable` (`combat_rules.go:60-68`, `combat_world.go:115-120`), a snapshot from its
  `NPCInfo` that `RelationChanged` never refreshes (`world.go:766-771`, `if o.Kind == KindPlayer`):
  after its owner's flag ends it still reads attackable, and its death then gives karma
  ([area skills](server-casts.md)). `MayFight` applies
  [Summons and pets](targets.md#summons-and-pets) on its own `PvPFlag` and `Karma` instead. Farming
  already skips summons (`farm.go:79`, `monster`).

- **Reaction.** `salient` (`brain.go:595-625`) also takes an `ExDuel*` packet, `MagicSkillCanceled`,
  a `MagicSkillUse` by or at a player it fights or by a player at a party member, and a fought player's
  `MoveToLocation` or `CharInfo` changing its mask or flag. Those arm the reaction at 200-600 ms
  (`PvPReact`; others keep 300-1200 ms, `:17-21`), moving an armed one earlier, never later (today
  the first wins, `:587-588`). A [guard](guards.md) stop (Esc, disengage move) takes no delay: its
  event wakes the brain at the next frame (`time.Second / frameRate`), which `EdgeSlack` covers.

## Tests

- **Codec (X1):** the five duel server packets decode from server-layout frames and consume
  exactly `len(payload)`, as does `MagicSkillCanceled` from a `FrameMagicSkillCanceled` frame; the
  two requests encode the server's field order; `CharInfo`, `NPCInfo`, `UserInfo` keep their new
  fields (radii included) over the recorded fixtures, consuming exactly `len(payload)`.
- **Reaction (X4):** an opponent's Heal on its ally, the first `ExDuelUpdateUserInfo`, an arena
  opponent's move → a decision 200-600 ms later, a pending 300-1200 ms one moved earlier; a stranger's
  move → none before the tick.
- **Datapack (X2):** the zone reader on fixtures for each zone kind, a position in and out: an
  `NPoly` with `minZ`/`maxZ`; a reversed-corner `Cuboid` (as Rune, `zones/TownZone.xml:445-449`: x
  47150 then 32531, y -44815 then -52045) contains a point between; any other shape fails. Shipped
  `TownZone.xml`: 18 towns with Rune; shipped `SiegeZone.xml`: 15 (9 castles, 6 clan halls); water
  on `LoadWaterZones` and its tests. The skill fields, Sting 223 read as `effectAbnormalLvl` 3 at
  skill level 1 and 8 at 45, `abnormalLvl` -1; `offensive` defaulted; target types and
  `skillRadius`; `negateStats`, `negateLvl`; the jewel, set and Lionheart vulnerabilities; `pvp*Dmg`; `cpTable`.
