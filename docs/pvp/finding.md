# How the bot finds fights

Ships in X5 (sources, holds, clearance) and X4b (duel ground). Decisions D36, D38. Values are
persona defaults.

## Sources

| Source | What the bot does | Bound |
| --- | --- | --- |
| A player attacks it | Fights back (today). | — |
| A duel challenge (`ExDuelAskStart`) | Accepts after the [reaction delay](wire.md#changes-to-todays-code-x4) when it can duel (out of combat, HP and MP at least half, no flag, karma or store, not a party duel, on [allowed ground](#where-a-duel-is-allowed), [clear of monsters](#clear-of-monsters)); else declines at once. Clan mates, allies and party members are taken like anyone ([rule 1](targets.md#mayfight)); a bot-marked challenger is declined at once (D38); party duels are declined. | one answer per challenge |
| A whispered `duel` | Challenges that player (`RequestDuelStart` with its name) when both can duel and it is within 2000. Extends D3 for this whisper, as D20 did (M11's proposed `join`, D28, would too). Dropped within `ChallengeEvery` of the bot's last `RequestDuelStart`, own or whispered. Dropped too when heard in combat stance or one starts before its reaction, or reached 30 s late behind another rule. Never held by `DeclineHold`; its decline (1931) or refusal (153, 1926, 2017-2028) starts `DeclineHold` for the bot's own challenges of that player only. | one per `ChallengeEvery` (3 min) per bot, shared; once per player per `ChallengeSameEvery` (30 min) |
| Its own challenge | Out of combat, clear of monsters, at its spot or on the road: challenges one non-bot player in sight within 2000, out of combat, unflagged, no karma or store, of its tier, both on allowed ground, when the [engage estimate](model.md#engage-estimate) passes against its row. A decline or refusal of any challenge to that player holds it. Holds are per bot. | one per `ChallengeEvery`, shared with whispers; once per player per `ChallengeSameEvery`; `DeclineHold` (2 h) |
| Flagged and karma players in sight | [Hunting](targets.md#hunting). | the fight rules |
| Arenas | Every `ArenaEvery` (90 min ± jitter) to the nearest arena reachable in its SCC or by gatekeeper (M7 trips); stays while another player is inside, attacking [arena targets](targets.md#arena-targets); leaves after `ArenaAlone` (5 min) alone or `ArenaStay` (20 min) by the [arena-edge](guards.md#arena-edge) leave. Its spot is released as for a supply trip (D16). | one trip per `ArenaEvery` |

## Where a duel is allowed

The server refuses a duellist in a peace zone, town, Derby track, PvP zone (arenas), running siege,
water or no-restart zone: 1940 to the one who cannot, 2017-2028 to a challenger whose target cannot
(`network/duel.go:37-38`, `:170-172`, `:199-232`, `duelProhibitedArea` `:226`;
`character_duel.go:67-69`; arenas set the PvP flag, `model/zone/kinds.go:17-21`). So neither the bot
nor its player may stand in a peace, town, Derby track, arena, water or no-restart polygon
(`zones/PeaceZone.xml`, `TownZone.xml`, `DerbyTrackZone.xml`, `ArenaZone.xml`, `WaterZone.xml`,
`NoRestartZone.xml`, X2). A siege zone blocks only during its siege (`model/zone/siege.go:45-57`),
which the bot sees only from inside (283 at its start or on entry, 284 on leaving,
`network/siege.go:220-229`); it then counts that polygon as blocked.

1940 covers every reason the bot's side cannot duel (`network/duel.go:37-40`, `:170-173`), passing
ones included (combat stance, HP or MP under half, `:202-205`), and always returns the state to
idle. Only an **unexplained** 1940, with the bot's own pre-checks passing (no combat stance by
`AutoAttackStart`/`AutoAttackStop`, `world.go:749-752`, `setCombat` `:988`; HP and MP at least half
by `StatusUpdate`; no flag, karma, store or known blocked polygon), also holds its answers
(declines) and own challenges until it moved 2000 away or `DuelBlockHold` (10 min) passed. So a
`pvp` bot in town or in an arena declines at once.

## Clear of monsters

Any hit between a duellist and an NPC cancels the duel ([duel end](server-duels.md)). So the bot accepts or
challenges only with no NPC in its attackers table and no aggressive monster in sight (`aggroRange`
above 0, `internal/datapack/npc.go` `AggroRange`) within its aggro range plus `DuelClear` (300).
From the challenge until idle it starts no monster fight (also refused as combat at the answer,
`network/duel.go:166-173`), and its duel chase and kite moves keep that clearance. A monster that
attacks anyway cancels the duel; the bot then fights it ([rule 1](targets.md#mayfight)).

## Duel ground

The server cancels a duel when a duellist stands in a peace, siege or PvP zone (arenas included)
or more than 2000 (3D) from its opponent ([duel end](server-duels.md);
`character_duel.go:66-69`, `InDuelBlockedZone`). From the
countdown to the end every move the bot sends (clearance, kite stand points, pole reposition,
disengage moves) is checked: destination and every point of its M7 path at least `DuelEdge` (100)
outside every peace, town, Derby track and arena polygon and every siege polygon it is not already
in (`zones/SiegeZone.xml`, 15 `NPoly`: 9 castles; clan halls `partisan_agit001`,
`devastated_castle`, `bandits_stronghold`, `rainbow_spring`, `beast_farm_guildhall`,
`fortress_of_dead`; a running siege is invisible from outside, `model/zone/siege.go:38-42`,
`setBattlefieldHolds`), and the destination within `DuelLeash` (1800) of the opponent's last
position.

- A failing stand point is skipped like a blocked one; with none the bot fights from where it
  stands (a pole wielder holds). A disengage move takes the first passing heading (`turnAround`,
  `internal/botclient/combat_rules.go:364`), else `orStay` (`:352`), which still ends the intention.
- **The server's follow.** After an `Action` or cast the server re-follows the moving opponent on
  its own path with no new packet (`model/actor/ai/player.go:358-394`, `MaybeStartOffensiveFollow`
  `:376`) until a move replaces the intention (`combat_rules.go:347-351`). So while it has an attack
  or cast intention on the opponent, on every decision and every own `MoveToLocation` or
  `MoveToPawn` (`world.go:682-694`), the bot checks the straight segment to the opponent and its
  current leg's destination. With the opponent out of range and either within `DuelEdge` of such a
  polygon, or itself already within `DuelEdge` of one, it sends the checked disengage move; then,
  and whenever the segment fails before an attack, no `Action` or out-of-range skill, only steps
  castable where it stands. Otherwise a chase only closes in, so an opponent beyond `DuelLeash` is
  chased at once.
- A duel run out this way times out, which restores everything (only a cancel does not). An
  opponent's own move into such ground cancels it.

## Tests

- `TestPvPDuelAccept` (X5): a challenge at full HP and MP out of combat → accepted after the delay;
  MP under half → declined at once; a `farmer` → declined; a party duel → declined; in a town, an
  arena, water or a no-restart zone → declined at once; an aggressive monster within its aggro range
  plus 300, or a monster in its attackers table → declined at once, no own challenge; after a 1940
  with the pre-checks passing → declined, no own challenge, until 2000 moved or 10 minutes; after a
  1940 in combat stance (`AutoAttackStart` without `AutoAttackStop`) → no hold, accepted once it
  ends; a whispered `duel` from a player in a town → no challenge; a clan mate's or party member's
  one-on-one → accepted; a bot-marked challenger → declined at once.
- `TestPvPDuelFight` (X4; the area, pole and duel ground cases in X4b): in the countdown (1945 at 3, 2
  and 1) no `Action`, `AttackRequest` or cast on anyone; the opponent's countdown `Attack` or Wind
  Strike → not answered, then 1952 and `ExDuelEnd` → the unflagged player not fought; `ExDuelStart`
  alone → no attack; the first `ExDuelUpdateUserInfo` plus the delay → plain `Action` and casts,
  never forced; its `MagicSkillUse` at the bot enters the attackers table; a clan mate or party
  member opponent → the same; 692 or `ExDuelEnd` stops it; no surrender; 1950 with its own name then
  `ExDuelEnd` → won, the opponent's → lost, 1955 then 1950 with its own → won, 1952 → tie; a refused
  attacker → kind `unfought`. A monster in sight never targeted; one that attacks fought only after
  1952 and `ExDuelEnd`; a test-built area step and a pole with a monster in reach → unused. Duel
  ground: repositioning a pole around a third player, nearest points inside or within 100 of a town
  polygon → skipped, the first point 100 outside; every point near a town, arena or foreign siege
  polygon or beyond 1800 from the opponent → no walk, the disengage move to the first passing
  heading or `orStay`, then the hold; the opponent 30 past a peace polygon's edge, the segment
  within 100 → only in-range steps; back 200 out → `Action`; already attacking a walking
  out-of-reach opponent, the segment coming within 100 of a town polygon → the disengage move at
  once, `Action` again once clear; its own `MoveToLocation` leg ending 60 from an arena polygon →
  the disengage move; its own position within 100 of a peace polygon while attacking → the disengage
  move along the first heading leading out.
- `TestPvPChallengeWhisper` (X5): a whispered `duel` → `RequestDuelStart` with its name; again
  within 30 minutes → nothing; a bot-marked name or a flagged target → nothing; another player's
  whisper within 3 minutes of the last challenge, own or whispered → nothing, after 3 → challenge; a
  player who declined (1931) the bot's challenge whispers 31 minutes later, inside `DeclineHold` →
  challenge; the whispered challenge refused with 2019 → no own challenge of that player for 2
  hours, its whisper 31 minutes later → challenge; a whisper heard in combat stance, or one a stance
  starts after, → no challenge after `AutoAttackStop`; one reached 31 s late → none.
- `TestPvPChallengeCooldown` (X5): one own challenge per 3 minutes; after 1931, 153 or 1926 that
  player waits 2 hours and the state is idle at once; on a test-built `charmodel.RowTable`, a
  candidate the estimate says it loses to → none, one it beats → `RequestDuelStart`; the losing
  candidate's whisper → `RequestDuelStart`; no own challenge off its spot, in a bot party or under a
  GM follow, assist or stay order.
- e2e `TestLivePvPDuel` (X13): a client challenges a `pvp` bot; it accepts and fights, the duel ends
  without a death and both get their vitals back (GM, client, bot).
