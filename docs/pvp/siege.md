# Sieges

Ships in X15 (#603). Decision D41 (owner approved 2026-11-26). Paths per
[README](README.md#paths). Before X15 a running siege was only [forbidden duel
ground](finding.md#duel-ground); a bot farming or walking in a battlefield when a siege started
stayed there.

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
  in a siege polygon the bot leaves it by the nearest point outside, marks it blocked (no spot or
  duel inside, and its walks route around it) until the siege ends, and fights back on the way
  only an attacker that hits it (no karma is possible inside). Its PvE spot inside a castle
  battlefield waits.
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

## As shipped (X15)

- **Lists.** The bot asks its own side's list only (0xa2 for an attacker, 0xa3 for a defender),
  castles 1 to 9 in order, one per `bypassGap` (`actSiegeList` keeps the bypass gate; the server
  has no flood protector on them, `network/client_dispatch.go:150-151`), and stops at the first
  list holding its clan (a defender clan waiting for approval does not count): at most 9 frames
  per siege state change, inside the 18 above. No list holding its clan after all nine are
  answered or `siegeListWait` (5 s) after the last, and it takes no part (a clan hall's siege).
- **Sides.** A player's side is read from its `Attacker` bit against the bot's own state, and
  `Ally` is never fought: the server resends a player's relation when its own state changes, not
  the observer's (`network/relations.go:104-121`), so `Ally` and `Enemy` may still show the bot's
  state from before the siege.
- **Block.** A battlefield a 283 showed running is blocked until a 284 while the bot stands inside
  it and does not walk out of it (the zone turned off around it, `model/zone/siege.go` `replay`),
  the bot's own state cleared for its castle's, or `SiegeLength` (120 minutes,
  `ops/docker/config/siege.properties`) after the 283. Each battlefield is blocked on its own, with
  its own end: when two sieges run at once, a 283 in the second keeps the first blocked.
- **Walks.** Out by the point `siegeMargin` (300) past the nearest edge, the next edge's when the
  walker answers one unreachable; in by the point `siegeDepth` (400) inside the nearest edge, on
  foot only (the castle towns' gatekeepers refuse a trip there during the siege,
  `serverpackets` `SystemMessageCannotPortVillageInSiege`); no way in and it takes no part. A
  defender inside at the start stays while its lists are out.
- **Routes.** Every walk keeps `siegeClear` (100) from each blocked battlefield the bot does not
  fight in (the walker's avoid area, beside a karma bot's town margin), but one the walk starts or
  ends in: the walk out leaves straight, a walk to a point inside enters straight. A walk that
  starts or ends just outside one still keeps it out; only the ground outside it within
  `siegePocket` (200) of that end stays open, so the walk reaches the end without crossing (no 283,
  walk out, re-cross loop). 100 is past
  a geodata cell's diagonal, so a segment between two kept columns never clips the polygon, and
  short of `siegeMargin` less `siegeArrive`, so a bot that walked out stands clear of it. When no
  route keeps out (or the search runs out of budget), the walk crosses as before and the walk out
  above is the backstop. A route planned before a 283 is not dropped: a 283 only reaches a bot
  inside the battlefield, where the walk out replaces it.
- **Live test.** Not shipped: the e2e GM scope has no `//siege`, and a siege started on the shared
  live server banishes real players from the castle zone. Tracked in #735.

## Mode

`Siege` (persona default, D6): on for `pvp`, off for every other preset. The not-taking-part rule
holds for every bot.

## Tests

- `TestUserInfoSiegeRelation` (X15, codec): recorded `UserInfo` frames with 0x180 and 0x80 decode
  to attacker and defender; `RelationChanged` 0x200, 0x400, 0x800, 0x1000 to the record.
- `TestPvPSiegeZoneNotice` (X15): 283 inside an arena polygon → arena; inside a siege polygon →
  siege running there, blocked; in neither → ignored.
- `TestPvPSiegeRoute` (#734): a walk past a running battlefield keeps out of it, and out of each
  of two; one the bot stands in or walks to is crossed; its own castle's is crossed by a bot taking
  part; a karma margin and a battlefield are both kept out; no way around → the walk crosses; a
  lapsed block → straight.
- `TestPvPSiegeFields` (#734): 283s in two battlefields block both; a 284 or a lapse ends only its
  own.
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
