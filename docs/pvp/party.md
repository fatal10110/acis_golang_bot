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
- Party duels: see [below](#party-duels).

## Party duels

Ships with #424 (M11). Only a party's leader challenges a party duel and only the challenged
party's leader sees `ExDuelAskStart` with the party flag; on the acceptance the server re-checks
every member of both sides (`duelRefusal`) and refuses the whole duel when one cannot
(`network/duel.go:95-124`, `:143-197`).

- **The leader answers.** A bot leading a party with `Duels` on accepts a party duel after the
  [reaction delay](wire.md#changes-to-todays-code-x4) when it may duel itself
  ([sources](finding.md#sources)), the challenger is no bot (D38) and every member is in sight and
  passes the server's checks by what it sees: alive, out of combat stance, no flag, karma or store,
  on [allowed ground](finding.md#where-a-duel-is-allowed), HP and MP at least half by the party
  window. A member out of sight may stand in a town or fight, so the leader declines rather than
  have the server refuse it. A bot that does not lead never accepts one (it never gets one); a
  `farmer` leader (`Duels` off) declines at once.
- **The leader challenges** a party duel on a whispered `duel` under the same checks and bounds as
  a one-on-one whisper ([sources](finding.md#sources)); its refusals (1936 declined, 1937 not in a
  party, 1942 the other party cannot) hold that player as a decline does. It makes no party duel
  challenge of its own: it roams in no bot party. A member that does not lead challenges a whisper
  one-on-one, as before, and so does the leader a party mate's (the server refuses a party duel
  within one party with 1940, `network/duel.go:100-104`).
- **Members** never see the challenge: 1944 (transported to the duel site, sent to both sides as
  the duel begins) starts their countdown, which holds them as a one-on-one countdown does. Both
  parties are teleported to the duel arena at the 33rd second of the 35 s countdown and back where
  they stood at the end (`network/duel.go:348-361`); the bot takes both as any teleport and walks
  back to its spot afterwards. A party duel ends 2 minutes after it begins, its 35 s countdown
  included (`duel/duel.go:82-85`, `duel/state.go:58-64`).
- **The fight.** Every `ExDuelUpdateUserInfo` names a foe (one per opposing member). Each member,
  leader or not, fights the foe it fights while that one stays live, else the one its leader hit
  last (focus), else the nearest; never a bot-marked foe (D38). A duel never kills: a foe left at
  1 HP is defeated and frozen (a 692 answers a click on it, `network/duel.go:626-660`), so the bot
  moves to the next; the latest window decides, as its HP is truncated, and a 692 marks the foe
  only when the bot's last click was on it; defeated itself (1 HP) it holds, and it never flees in a duel (a run would
  stray from the other side and cancel it, `duel/duel.go:203-212`). Every foe is outside its
  support ([duel support](guards.md#duel-support)) and none is an area bystander; its own members
  are supported as before, but for one defeated at 1 HP by the party window, frozen as a foe is. The result is the side's: 1951 names the winning side's leader.
  [Party PvP](#party-pvp) waits while the duel is on.

## Tests

- `TestPvPTargetHealer` (X9): a player's party (a player leader from the party packets: the GM
  commander, who may lead several support bots) of a `pvp` damage bot, a `support` KN and a
  `support` CL; two flagged enemies hitting them, one casting 1015 Battle Heal on the other → the
  `pvp` bot switches to the healer; the KN casts Aggression on the enemy hitting the CL or leader; a
  test sleep step skips the healer being hit and sleeps a second enemy no member hits; the CL does
  not heal the flagged damage bot with `SupportFlagged` off, does with it on, and buffs Mental
  Shield against a caster. A `support` CL alone with a player leader → the same.
- `TestPvPTargetHealerFarmer` (#423): the same cases in an M11 bot party led by a `farmer` (fight
  back only, `SupportFlagged` off) with a `support` KN and a `support` CL. The leading farmer assists
  nobody, so its enemy set is the party's hitters and their healers (no leader's target; a member's
  own hit adds nobody); a hit on the CL wakes it onto the hitter, and a Battle Heal on that enemy
  switches it to the healer. The KN casts Aggression on the enemy hitting the CL or the farmer
  leading. The CL (`support`, `SupportFlagged` off by default) leaves the flagged farmer unhealed and
  unshielded and stands behind it; with `SupportFlagged` on it heals the farmer and buffs it Mental
  Shield against a caster.
- `TestPvPPartyDuelAnswer` (#424): the bot leading the farmer's bot party with `Duels` on accepts a
  party duel at the reaction when every member is in sight and ready; declines at once with
  `Duels` off, a member out of sight, under half HP or MP, in combat stance, flagged or in a town,
  when it does not lead, under half MP itself, or a bot-marked challenger. 1933 and 1944 start the
  countdown that holds it, and the other side's leader is never supported.
- `TestPvPPartyDuelChallenge` (#424): the leader challenges a whispered duel as a party duel, and
  1934 starts its countdown; 1936, 1937 and 1942 drop it and hold that player; no challenge with a
  member out of sight; a member challenges one-on-one.
- `TestPvPPartyDuelFight` (#424): a member's 1944 starts the countdown, the arena teleport and the
  countdown windows hold it, and the duel outlives the one-on-one limit; it fights the nearest foe
  from the reaction, any live foe is a duel fight, a window at 1 HP or a 692 drops a foe; the
  leader's target goes first; a bot-marked foe is never fought; defeated at 1 HP in combat stance
  it holds and does not flee; its own CL is supported and a foe never; 1951 naming its leader is
  won, the other side's lost, and `ExDuelEnd` ends it.
- e2e `TestLivePvPHealer` (X13; needs M11, a bot leading the two bots within the 4-character cap):
  in the open world an M11 bot party of a `farmer` and a `support` Cleric (`SupportFlagged` on) is
  attacked by the GM and a client heals the GM (flagging the client); the farmer fights back and
  targets the client, the Cleric heals the flagged farmer (GM, client, two bots).
