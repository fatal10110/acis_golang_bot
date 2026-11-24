# Whom the bot may fight

Ships in X4 (`MayFight`, summons; rule 3 in X4b), X5 (hunting, arena targets). Decisions D38, D39.
What keeps a fight from giving karma: [guards](guards.md).

## MayFight

For a player `o` (monsters and folk NPCs as today except in a duel, rule 1; summons by
[Summons and pets](#summons-and-pets)):

1. **Duel.** From the countdown to the end, only the duel opponent, and only while fighting
   ([wire](wire.md#world-model-x4)). In the countdown nobody: force-free duel hits need both
   Duelling (`duel/state.go:82-86`, `character_social.go:201`) and a hit on a duellist not yet
   Duelling interrupts the duel (`character_duel.go:114-124`), cancelled at the next 1 s check with
   nothing restored. An opponent's countdown hit is not answered; after the cancel (1952,
   `ExDuelEnd`) it is a clean player (no flag between duellists, `character_pvpflag.go:127-129`).
   While fighting, the opponent is fought even as a clan mate or party member (duel hits come before
   party and clan checks, `character_social.go:201`, `:230`; one-on-one duels check no party, clan
   or ally, `network/duel.go:31-62`, `:145-187`; only party duels do, `:102`). Every other player
   waits (hitting one flags the bot, cancelling the duel), and so does every NPC: none targeted, no
   area skill or pole swing with one in reach ([guards](guards.md#area-hits-and-bystanders)), since
   hitting one cancels the duel (`creature/duel.go:34-41`). A monster's hit cancels it too
   (`character_duel.go:121-124`); the bot holds that monster off until the cancel arrives (1952, the
   tie a cancel shows, then `ExDuelEnd`: `network/duel.go:538`, `duelEndMessages`;
   `serverpackets/systemmessage_duel.go:24`), then fights it and rests;
2. never a party member, in an arena too (`character_social.go:204-206`, `:233-235`);
3. in the same arena: anyone else, clan mates, allies and bots included (D38;
   `character_social.go:206`, `:235`);
4. never a clan mate (`CharInfo` clan id equal to its own non-zero one; clanless players are not
   clan mates, `IsInSameClan`, `character_social.go:120-122`, `charinfo.go:121`) or ally (equal
   non-zero ally id, its own from `UserInfo`);
5. never a bot (nickname marker) unless it hit the bot first (an `Attack` or hostile cast in the
   attackers table, D38);
6. `FightFlagged` and `o` flagged or with karma (today);
7. PK mode on and the D39 limits pass ([PK mode](karma.md#pk-mode)), a clean attacker of a karma
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
of sight or refused, and then only:

- outside a duel, never as a hunting or PK target, never a party member's (title equal to a member's
  name), clan mate's or ally's (the owner's ids, when in sight);
- while its own `Karma` is above 0 or `PvPFlag` is 1 (last `RelationChanged`); at 2 the
  [flag guard](guards.md#flag-guard) applies; at 0 and karma 0 never, whatever `Attackable` said;
- in an arena, only while its owner is seen inside by `ArenaEdge`, the arena-edge rule read on the
  owner (the server credits the summon's death by its owner's zone);
- with plain `Action` and casts only, never forced, so the server still refuses a summon of an owner
  the bot could not place.

A refused summon is held off: neither `attackers` nor `unfought` returns it
(`combat_rules.go:88-120`). The same holds for a summon a party leader attacks ([party](party.md)).

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
- e2e `TestLivePvPFightBack` (X13): a client force-attacks a `pvp` bot; it fights back; the client
  stops and unflags and the bot drops it (GM, client, bot).
