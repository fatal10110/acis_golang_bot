# Sieges

Ships in X15 (#603). Decision D41 (proposed, needs owner approval). Paths per
[README](README.md#paths). Today a running siege is only [forbidden duel
ground](finding.md#duel-ground); a bot farming or walking in a battlefield when a siege starts stays
there.

## Server facts

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Registering | A clan member with `PrivCastleSiege` sends `RequestJoinSiege` (0xa4) for a castle as attacker or defender; refused (`NotAuthorizedToDoThat`) without it, and for a dissolving clan, a clan under `SiegeClanMinLevel` (4), an allied castle, a full defender side. The castle lord confirms defenders (0xa5). Lists: 0xa2, 0xa3 (answers 0xca, 0xcb); `SiegeInfo` 0xc9. | `network/clientpackets/siege.go:7-10`; `network/serverpackets/siege.go:7-9`; `network/siege.go:49-83`, `:85`, `:154`; `siege/registration.go:13-48`, `:126`; `ops/docker/config/siege.properties:9`, `:23` |
| Siege state | At the siege start every online member of a registered clan gets its side (attacker, defender; the owner's clan defends), cleared at the end; set again at login while the siege runs. `UserInfo`'s relation field shows it to the player itself (attacker 0x180, defender 0x80); `RelationChanged` shows it to others: `InSiege` 0x200, `Attacker` 0x400, and `Ally` 0x800 to an observer of the same state or `Enemy` 0x1000 to any other, a non-participant included. | `siege/lifecycle.go:425-436`; `network/siege.go:370-380`; `network/clan.go:54-59`; `network/serverpackets/userinfo.go:95-104`; `network/relations.go:12-36` |
| Battlefield | While the siege runs, its zone (`SiegeZone.xml`, 9 castles and 6 clan halls) is a PvP zone for everyone inside, participant or not: anyone attacks anyone without force (same-side participants need force like clan mates), no flag, no karma, no kill credit. Entering or the start sends 283, leaving 284; leaving while it runs flags the player. Invisible from outside. | `model/zone/siege.go:30-90` (`setBattlefieldHolds`: `FlagPvP`); `network/siege.go:215-246`; `character_social.go:198-213` (`inSameActiveSiegeSide`); `character_karma.go:29-34` |
| Banishment | At the start, and when the castle changes hands, every player not of the owner's clan is teleported out of the battlefield to its town restart point; thrown out at the start it gets no 283, 284 or flag. | `siege/lifecycle.go:194`, `:315`, `:395`; `model/zone/siege.go:129-139` (`BanishForeigners`); `network/siege.go:205-215`, `:248-270` |
| Death cost | A quarter of the normal loss in a siege zone (no arena exemption); only a Charm of Courage spares it. | `character_death_exp_karma.go:94-104`; [karma](server-karma.md) |
| Guards and towers | Siege guards attack the attackers; towers and flags are NPCs of the siege. | `network/siege_guards.go`; `siege/towers.go`; `network/siege_flag.go` |
| Clan halls | Six halls have sieges of their own (attackers marked by `Besieging`). | `network/hall_siege.go`; `network/clan.go:57-59` |

## What the bot does

- **It never registers.** A bot sends no 0xa4 or 0xa5: it is a member of a player's clan (D20),
  never its leader, and holds no siege privilege unless a player grants it; registering stays the
  clan leader's call. It takes part only when its clan is registered and the siege runs.
- **Telling 283 and 284 apart.** By position (X2's zone reader): inside an arena polygon they are
  the arena's; inside a siege polygon they mean that siege runs (from 283 to 284 or the siege's
  end); inside an Olympiad stadium during a match they are the stadium's ([Olympiad](olympiad.md));
  elsewhere they are logged and ignored.
- **Not taking part** (no siege state in `UserInfo`, or `Siege` off, or a clan-hall siege): on 283
  in a siege polygon the bot leaves it by the nearest point outside, marks it blocked (no spot,
  route or duel inside) until the siege ends, and fights back on the way only an attacker that
  hits it (no karma is possible inside). Its PvE spot inside a castle battlefield waits.
- **Taking part** (`UserInfo` attacker or defender, `Siege` on): the castle is found from the
  attacker and defender lists, 0xa2 and 0xa3 for each of the 9 castles, one request per
  `bypassGap`, once per siege state change (at most 18 frames per siege). It walks to the
  battlefield after the start (the start banishes everyone but the owner's clan) and fights there
  like in an [arena](targets.md#arena-targets): the nearest player showing `InSiege` and `Enemy` of
  any tier, never one showing `Ally`, never a non-participant unless it hits first, never a bot
  unless it hits first (D38 unchanged). It attacks no door, tower, flag or NPC, engraves nothing,
  and fights a siege guard only when it hits.
- **Deaths.** Village restart (no siege HQ or castle restart), walk back; after `SiegeDeaths` (5)
  it leaves for its spot as from an arena. Each death costs a quarter of the loss: the bot holds
  no Charm of Courage (buying one is out of scope).
- **Leaving.** At the siege's end (the state cleared in `UserInfo`) it returns to its spot. Leaving
  a running battlefield flags it: it fights back as today.
- **Guards.** The [flag guard](guards.md) is off inside a running battlefield (no karma there);
  the [arena edge](guards.md#arena-edge) applies to its polygon: an enemy who stepped out of it is
  no longer hit (a kill outside is a PK unless the enemy is flagged, which leaving does).

## Mode

`Siege` (persona default, D6): on for `pvp`, off for every other preset. The not-taking-part rule
holds for every bot.

## Tests

- `TestUserInfoSiegeRelation` (X15, codec): recorded `UserInfo` frames with 0x180 and 0x80 decode
  to attacker and defender; `RelationChanged` 0x200, 0x400, 0x800, 0x1000 to the record.
- `TestPvPSiegeZoneNotice` (X15): 283 inside an arena polygon → arena; inside a siege polygon →
  siege running there, blocked; in neither → ignored.
- `TestPvPSiegeLeave` (X15): no siege state, 283 inside a castle polygon → walks out by the nearest
  exit, no target picked; an attacker hits it → fought back.
- `TestPvPSiegeFight` (X15): attacker state, `Siege` on → 0xa2 and 0xa3 paced by `bypassGap`, walk
  in after the start; targets an `InSiege`+`Enemy` player, never an `Ally`, a non-participant or a
  bot that has not hit; an enemy outside the polygon → not hit; the fifth death → its spot.
- `TestLivePvPSiege` (X15, live, at most 4 characters): a GM registers clan A (the bot's, level 4
  by the GM) as attacker of a castle and a scripted clan B as defender (`//siege attack|defend`,
  `network/admin_siege.go:11-64`), starts it (`//siege start`); the bot walks in, 283, attacks the
  scripted defender, never a second, clanless bot standing inside; that bot, inside at the start,
  is banished or walks out on 283; `//siege end` → the state cleared, the first bot leaves. The
  lists answer without a dialog (`network/siege.go:24-31`).
