# Whom the bot may fight

Ships in X4 (`MayFight`, summons; rule 3 in X4b, rule 7 in X11), X5 (hunting, arena targets). Decisions D38, D39.
What keeps a fight from giving karma: [guards](guards.md).

## MayFight

For a player `o` (monsters and folk NPCs as today except in a duel, rule 1; summons by
[Summons and pets](#summons-and-pets)):

1. **Duel.** From the countdown to the end, only the duel opponent, and only while fighting
   ([wire](wire.md#world-model-x4)); in a party duel any live foe of the other side
   ([party duels](party.md#party-duels)). In the countdown nobody; an opponent's countdown hit cancels
   the duel and is not answered, and after the cancel (1952, `ExDuelEnd`) it is a clean player.
   While fighting, the opponent is fought even as a clan mate or party member (duel hits come before
   party and clan checks, `character_social.go:201`, `:230`; one-on-one duels check no party, clan
   or ally, `network/duel.go:31-62`, `:145-187`; only party duels do, `:102`). Every other player
   and every NPC waits: none targeted, no area skill or pole swing with one in reach
   ([guards](guards.md#area-hits-and-bystanders)). A monster that hits the bot is held off until
   the cancel arrives (1952, then `ExDuelEnd`), then fought, then the bot rests. What cancels a
   duel: [duel end, duel opponent attackable](server-duels.md);
2. never a party member, in an arena too (`character_social.go:204-206`, `:233-235`);
3. in the same arena: anyone else, clan mates, allies and bots included (D38;
   `character_social.go:206`, `:235`), but a member of its command channel (`:205`), which the bot
   does not track ([gap](README.md#risks-and-gaps));
4. never a clan mate (`CharInfo` clan id equal to its own non-zero one; clanless players are not
   clan mates, `IsInSameClan`, `character_social.go:120-122`, `charinfo.go:121`) or ally (equal
   non-zero ally id, its own from `UserInfo`);
5. never a bot (nickname marker) unless it hit the bot first (an `Attack` or hostile cast in the
   attackers table, D38);
6. `FightFlagged` and `o` flagged or with karma (today);
7. PK mode (today's `ForceAttack`) on and the D39 limits pass ([PK mode](karma.md#pk-mode)), a clean attacker of a karma
   bot included ([with karma](karma.md#with-karma));
8. else no.

## Hunting

Attacking a player that has not touched the bot also needs, for rule 6: `o`'s class tier equal to
the bot's (`CharInfo` has no level); the bot's HP and MP above `StandHP` and `StandMP`; neither in a
peace zone, town or the Derby track (X2; the server refuses with 84, 85,
`model/zone/kinds.go:394-406`); the [engage estimate](model.md#engage-estimate) passing. A PK-mode
victim needs the same four plus the D39 limits. Fighting back needs none.

## Arena targets

Whom it attacks unprompted in an arena (X5): no engage estimate, the other hunting preconditions
apply. With nobody in its attackers table, and its HP and MP above `StandHP` and `StandMP` (else it
rests at least `ArenaEdge` inside), it picks the nearest player rule 3 allows, seen inside the same
polygon by at least `ArenaEdge` ([arena edge](guards.md#arena-edge)), of its class tier. It keeps
the target until it dies, leaves or falls out of the tier; an attacker is fought back first
(`ruleAttacked`). A player who just walked in is a target like any once inside by the margin.

## Summons and pets

The bot fights the owner: a summon in the attackers table adds its owner (the player in sight whose
`CharInfo` name is the summon's title), fought by rules 1-8 first (the summon's hit usually flags
its owner, `character_pvpflag.go:52-62`, `:122-136`). The summon itself only when its owner is out
of sight or refused, or before its owner when killing it first [pays off](#summon-first) (#428), and
then only:

- outside a duel, never as a hunting or PK target, never a party member's (title equal to a member's
  name), clan mate's or ally's (the owner's ids, when in sight);
- while its own `Karma` is above 0 or `PvPFlag` is 1 (last `RelationChanged`); at 2 the
  [flag guard](guards.md#flag-guard) applies; at 0 and karma 0 never, whatever `Attackable` said;
- in an arena, only while its owner is seen inside by `ArenaEdge`, the arena-edge rule read on the
  owner (the server credits the summon's death by its owner's zone);
- with plain `Action` and casts only, never forced, so the server still refuses a summon of an owner
  the bot could not place.

A refused summon is held off: `attackers` never returns it, nor does `unfought` while the bot has
no karma (`combat_rules.go:88-120`; with karma it flees it, [with karma](karma.md#with-karma)).
The same holds for a summon a party leader attacks ([party](party.md)).

### Summon first

Killing a summon before its owner in sight and fightable pays off (`summonPays`, `pvp_summon.go`)
when:

- it healed its owner within `enemyHealerFor` (60 s): a `MagicSkillUse` of the summon on its owner
  whose skill is a heal (`HEAL`, `HOT`, `HEAL_PERCENT`, `BALANCE_LIFE`), a pet's Heal Trick (4717,
  target `OWNER_PET`) for one; a pet's heals undo the bot's damage on the owner;
- or both hit the bot (within `attackerWindow`) and the summon's streak dealt more than
  `switchFactor` (2) times its owner's, more than its owner's alone once the summon is the bot's
  target, so a close race does not flip the target: a summoner's damage is its servitor's.

An owner standing back, not hitting the bot, is still fought first. Such a summon stands in for its
owner (`standIn`): `attackers` lists it in the owner's place with the pair's damage, and a bot whose
target is the owner turns to it (`ruleAttacked`). It passes the summon rules above on its own, so
the kill never costs karma: the server credits a summon's death by its owner's state
(`AwardSummonKillKarma`, `character_karma.go:83-86`), and a summon's flag and karma mirror its
owner's, so flag 1 or karma means a karma-free kill, and a summon at flag 0 and karma 0 is never
fought first, whatever it dealt or healed (a PK-mode victim's summon included). It is never a
hunting or PK target, never fought in a duel, and a party fight keeps its own target rule: the
party's hits by a summon still count for its owner ([party](party.md)). Behavior change (D24):
before #428 the owner in sight and fightable was always fought first.

## Tests

- `TestPvPHuntFlagged` (X5): a flagged player of its tier → attack; flagged and clanless with the
  bot clanless (clan ids 0) → attack; a flagged party member, clan mate, ally, other-tier or clean
  player → none; in a peace zone, a town or the Derby track → none; a flagged third player in its
  own duel → none.
- `TestPvPBotsOnlyInArena` (X5, D38): a flagged bot-marked player outside an arena → not hunted;
  fought once it hits, by `Attack` or (a second case) only a Wind Strike `MagicSkillUse`; a Heal at
  the bot → not fought; inside an arena → fought.
- `TestPvPArenaTarget` (X5): at the Gludin arena's middle, no attacker, HP and MP above the stand
  levels: players of its tier at 100 and 250 (both 240+ from the edge) → `Action` on the one at 100;
  a level-1 base class at 50 (the bot a first profession) → not picked; only a tier player 120 from
  the edge → no pick; a party member → not picked; MP under `StandMP` → rests; while it attacks its
  pick the player at 250 hits it → it fights that one back; a tier player just walked in, 300
  inside, idle, the only candidate → picked. `ArenaStay` reached with its damage cast in flight →
  cancel, a move inside, one attack interval, then the walk out, never the walk first.
- `TestPvPSummon` (X4; the flag-2 and arena cases in X4b): a summon (`Summon`, `Attackable`, flag 1,
  karma 0, owner out of sight) hits the bot → fought with plain `Action` and casts only; its flag
  turns to 2 → no damage or `nextActionAttack` step, its cast in flight cancelled, a move; flag 0
  and karma 0 with `Attackable` still true → held (today's `unfought`); karma above 0 → fought; the
  flagged owner in sight → the owner fought, not the summon; a party member's flagged summon →
  never; any summon hitting it in its duel → not fought; in an arena, the owner inside by more than
  150 → fought, outside → not; a summon's Wind Strike at the bot → in the attackers table, a Heal →
  not.
- `TestPvPSummonFirst` (#428): a flagged owner in sight and its flagged summon hitting the bot: the
  owner standing back → the owner; summon 100, owner 20 → the summon selected, alone in
  `attackers` with 120; 30 against 20 → the owner, but the summon kept with a plain `Action`, not
  forced, once it is the target; 15 against 20 with the summon the target → the owner; a Heal
  Trick on the owner → the summon, also when the owner was the target; a heal older than
  `enemyHealerFor` or on another player → the owner; flag 0 and karma 0 → the owner; karma → the
  summon; flag 2 with the summon the target → no attack on it; a clan mate's or party member's
  summon → never; in its duel → the duel; in an arena with the owner inside by 1700 → the summon,
  100 from the edge → not; out of combat a healing pet → not hunted.
- e2e `TestLivePvPFightBack` (X13): a client force-attacks a `pvp` bot; it fights back; the client
  stops and unflags and the bot drops it (GM, client, bot).
