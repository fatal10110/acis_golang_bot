# Party PvP

Ships in X9. Runs for every preset in any party, whoever leads it: a `support` bot in a player's
party (today's main use, `docs/plan.md` "support"), a `pvp` bot in a player's party, and with M11
`farmer` and `support` bots in a bot party ([who plays which row](modes.md#who-plays-which-row)).
Fighting the enemy set counts as fighting back (no hunting mode needed).

- **Enemies**: players hitting a member (`Attack`, or a hostile `MagicSkillUse` with that member as
  target, [wire](wire.md#changes-to-todays-code-x4)), a player healing one of them once `MayFight`
  allows it (that heal flags it outside a PvP zone), and the leader's target (today's assist). A
  summon hitting a member, or the leader's target, adds its owner when in sight; the summon itself
  only under [summons and pets](targets.md#summons-and-pets).
- **The healer first.** An enemy healing another (HEAL, HOT, HEAL_PERCENT, BALANCE_LIFE) is the
  enemy healer for 60 s; a healer-class enemy ([archetypes](model.md#archetypes)) is the prior.
  Damage rows target it within reach and spend `Hold` and `Int` steps on its heals (a stun holds,
  ended one non-DOT hit in ten, `character_stats.go:876`). A sleep row never sleeps the healer being
  hit; it sleeps another enemy no member hits, carrying no DOT ([sleep](layer.md#pvp-order)); a root
  row roots a fleeing healer. Otherwise assist the leader; a member under 30 % HP pulls them onto
  its attacker.
- **Peel.** A row with a `Peel` step (KN, EK, PK) casts 28 Aggression (`TargetMe`) on the enemy hitting its healer
  or leader.
- **The bot's healer** heals as today (leader, self, others) under its own `SupportFlagged`: on for
  a healing `pvp` bot (base mystic 1011 Heal, OS Chant of Life; off in its own duel, never its duel
  opponent, [duel support](guards.md#duel-support)); off for a `support` healer, which in the open
  world stops healing a member flagged by fighting back unless the owner switches it on (D6, D35).
  In an arena or against a karma attacker nobody gets flagged. Under the same setting it buffs 1035
  Mental Shield on members against casters and battle casters (a tank buffs its own 112 Deflect
  Arrow against archers) and stands behind the leader.
- Party duels are declined until M11 gives a bot party a leader that can answer one.

## Tests

- `TestPvPTargetHealer` (X9): a player's party (a player leader from the party packets: the GM
  commander, who may lead several support bots) of a `pvp` damage bot, a `support` KN and a
  `support` CL; two flagged enemies hitting them, one casting 1015 Battle Heal on the other → the
  `pvp` bot switches to the healer; the KN casts Aggression on the enemy hitting the CL or leader; a
  test sleep step skips the healer being hit and sleeps a second enemy no member hits; the CL does
  not heal the flagged damage bot with `SupportFlagged` off, does with it on, and buffs Mental
  Shield against a caster. A `support` CL alone with a player leader → the same. The `farmer` in a
  bot-led party cases land with M11 (X13).
- e2e `TestLivePvPHealer` (X13; needs M11, a bot leading the two bots within the 4-character cap):
  in the open world an M11 bot party of a `farmer` and a `support` Cleric (`SupportFlagged` on) is
  attacked by the GM and a client heals the GM (flagging the client); the farmer fights back and
  targets the client, the Cleric heals the flagged farmer (GM, client, two bots).
