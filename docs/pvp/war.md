# Clan-war enemies as targets

Ships in X14 (#602). Decision D40 (owner approved 2026-11-26). Paths per
[README](README.md#paths). Today a clan-war enemy is a clean player to the bot
([MayFight](targets.md#mayfight) rule 8): it is never attacked unless flagged or with karma.

## Server facts

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Declaring | Only a clan leader (or a member with the privilege) sends `RequestStartPledgeWar` (0x4d); the target clan must be level 3 with `ClanMembersForWar` (15, all members, offline included) members, not allied, not dissolving, and not at war with it in the last `ClanWarPenaltyWhenEnded` (5) days. Stop (0x4f) is refused while any online member of the stopping clan is in combat (1677); surrender (0x51) ends it and costs the surrendering member a full death loss. | `network/clientpackets/pledge_subunit_war.go:7-16`; `clan/war.go:15`, `:136-170`, `:185`, `:213`, `:249`; `network/clan_war.go:18-60`, `:77-105`; `ops/docker/config/clans.properties:29`, `:32` |
| At war | Mutual only: both clans declared on each other (`AtWarWith`). A member of either clan's academy is never at war (`atWarOutsideAcademy`). | `model/actor/player/character_social.go:152-167` |
| On the wire | `RelationChanged` (0xce) carries the relation bits of a player as its observer sees them: `MutualWar` 0x8000 when both clans declared, `OneSidedWar` 0x10000 when the observer's clan declared; neither when either player is in an academy. Sent on sight and on a flag, karma or siege change. Declaring or ending a war sends only `PledgeShowInfoUpdate` and system messages 1561, 1562 (declared) and 1566, 1567 (ended) to the members, not a `RelationChanged` to players already in sight: the same as the reference (`ClanTable.storeClansWars`), so not a server gap. `CharInfo` has no war bit. | `network/serverpackets/relationchanged.go:6-18`; `network/relations.go:12-58`, `:98-121`; `network/clan_war.go:114-145`; `network/serverpackets/systemmessage_clan_subunit_war.go:9-15` |
| Attacking | Needs force at war too: an `AttackRequest` or a ctrl-held cast. With ctrl any offensive skill passes on a mutual-war enemy, debuffs included; one-sided war is a clean player (a debuff is refused even with ctrl). | `character_social.go:198-266` (`SocialWithoutForce`, `OffensiveCastAllowed`: `AtWarWith` → `ctrl`); `character_social_policy_test.go:174-177`, `:253` |
| Flag | A hit on a karma-free, unflagged war enemy flags the attacker like a hit on any clean player (no war exemption). | `character_pvpflag.go:89-102`, `:122-136` |
| Kill credit | A kill at mutual war outside every academy is a PvP kill: no karma, a PvP point. | `character_karma.go:24-71` (`pvpKill`: `atWarOutsideAcademy`); `character_kill_credit_test.go:13` |
| Death cost | A quarter of the normal loss when either clan declared war on the other (one-sided counts). | `character_clan_war_death.go:8-16`; `character_death_exp_karma.go:52`, `:94-100` |

## What the bot does

- **No war packets.** The bot never declares, answers, stops or surrenders a war (0x4d-0x52,
  `RequestPledgeWarList`): bots join a player's clan only as its clan member (D20) and are never
  its leader. X14 adds no sender; attacks and casts keep today's pacing.
- **War mark.** The opponent record gains the relation bits of its last `RelationChanged` (`RelationChanged.Relation`, decoded by
  `internal/botwire/combat.go` `decodeRelationChanged`). A player is a war
  enemy while its last bits hold `MutualWar`. The bit already excludes academies, so it matches
  the server's kill credit exactly.
- **Staleness.** A war declared while the enemy is in sight shows only on its next
  `RelationChanged` (a sighting, a flag, karma or siege change): the bot misses a target, never
  takes a wrong one. A war that ends while the mark is set is the danger (a kill would be a PK):
  on 1566 or 1567, on leaving or being dismissed from its clan, and on any change of its own clan
  id in `UserInfo`, every war mark is cleared until a new `RelationChanged` sets it again.
- **The enemy leaves its clan.** When the enemy leaves, is expelled or joins another clan, the
  server sends the players around it only its `CharInfo` with the new clan id: no
  `RelationChanged` (`broadcastLooks` sends no relation update) and no 1566 or 1567 to the bot.
  So the mark records the clan id the opponent record held when the mark was set (from its last
  `CharInfo`, `internal/botclient/pvp_world.go:351`), and a `CharInfo` with a different clan id
  (0 included) clears that player's mark until a new `RelationChanged` sets it again. Server:
  `network/clan.go:254-262` (`sendLeftClan` → `broadcastCharacterInfo`), expel path `:330-355`;
  `network/inventory.go:923-926`.
- **[MayFight](targets.md#mayfight).** A new rule 6b after rule 6: `FightWar` on and `o` marked
  at war → yes. Rules 1-5 come first: no clan mate or ally can be at war with the bot's clan (the
  server refuses war on an ally and on its own clan), and a war-enemy bot is still fought only
  after its hit (D38).
- **Hunting.** A war enemy is hunted on [hunting](targets.md#hunting)'s preconditions (tier,
  `StandHP` and `StandMP`, no peace zone, the engage estimate). The bot attacks with `AttackRequest`
  and its PvP order with every cast ctrl-held (the server needs force; debuffs pass with ctrl at
  mutual war), so the PK-mode skill filter does not apply.
- **Guards.** [The flag guard](guards.md) treats a marked enemy like a flagged one: a kill is a
  PvP kill. When the mark is cleared mid-fight and the target is neither flagged nor with karma,
  the guard's clean-player rule applies at once (Esc, no next swing or cast). Left over: a hit
  already in flight when the war ends (the server refuses a stop while the stopping clan fights,
  1677, so only a surrender or the other clan's stop can end it mid-fight).
- **Its own flag.** The first hit flags the bot 40 s: anyone may then hit it without karma, which
  it fights back under rule 6 as today.
- **Death.** A death to a war enemy costs a quarter of the loss; the killer is avoided for
  `AvoidAfterDeath` as in the [open world](karma.md#by-fight).

## Mode

`War` (persona default, D6): on for `pvp`, off for every other preset (they fight back as today:
a war enemy that hits them is flagged by its hit, rule 6). A switch on the panel (X12's list).

## Tests

- `TestRelationChangedBits` (X14, codec): a recorded `RelationChanged` with 0x8000 and one with
  0x10000 decode to the relation bits; the decoder consumes exactly `len(payload)`.
- `TestPvPWarTarget` (X14): `FightWar` on, a clean player marked 0x8000 → `MayFight` yes, hunted
  under the preconditions with `AttackRequest` and ctrl-held casts (a debuff included); 0x10000
  only → no; `FightWar` off → no; a marked bot that has not hit → no; after 1567 or 1566 → no until
  a new `RelationChanged` marks it; after the bot's clan id changes in `UserInfo` → no; a marked
  enemy whose next `CharInfo` shows clan 0 or another clan id → no until a new `RelationChanged`
  marks it; a `CharInfo` with the same clan id keeps the mark.
- `TestPvPWarGuard` (X14): mid-fight on a marked clean enemy, 1567 arrives → Esc, no further
  `Action`, `AttackRequest` or cast; mid-fight, the enemy's `CharInfo` arrives with clan 0 (it
  left or was expelled) → Esc, no further `Action`, `AttackRequest` or cast; the same enemy
  flagged → the fight goes on under the flag guard.
- `TestLivePvPClanWar` (X14, live, 4 characters: the bot, the scripted leaders of clans A and B,
  the GM): clans A (the bot's) and B at mutual war, B's leader out of town: the bot attacks it with
  force and kills it, the bot's `UserInfo` shows a PvP kill and no karma; B's leader restarts and
  comes back, A's leader stops the war (0x4f) while the bot approaches → the bot stops. Seed: scripted clan leaders
  (`internal/e2e/pledge.go`) declare with 0x4d and answer with 0x4e; the GM raises both clans to
  level 3 (`network/admin_pledge.go:110`). Not run for now: each clan needs 15 members (`ClanMembersForWar`), which no 4-character
  scenario has, so X14 ships on unit tests and recorded frames ([owner question 1](#owner-questions)).

## Owner questions

Answered by the owner on 2026-11-26, with the proposed answers: X14 ships on unit tests and recorded frames (no fixture
clans, no change to `ClanMembersForWar`); `War` is on by default for `pvp` bots.

1. The live war scenario needs two clans of 15 members. Options: fixture clans created once on
   the stack and reused by name (their extra members stay offline), or a test stack with a lower
   `ClanMembersForWar` (a server setting, not a bot special case). Until then X14 ships on unit
   tests and recorded frames.
2. `War` on by default for `pvp` (proposed) or opt-in per bot like PK mode?
