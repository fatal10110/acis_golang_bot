# Death and karma

Ships in X11. Decision D39. Outside PK mode the bot gets karma only through timing it cannot see:
[guards](guards.md) keeps it off everything else.

## By fight

| Fight | Death costs (server) | What the bot does |
| --- | --- | --- |
| Duel | Nothing: no death; vitals back at the end | Fights to the end, never surrenders; after `ExDuelEnd` rests and rebuffs as after any fight. |
| Arena | No experience, no drop | Accepts a resurrection, else restarts in the village. After each of its first four deaths in the last hour it walks back; the fifth (`ArenaDeaths`, 5) sends it to its spot. |
| Open world, flagged or karma opponent | The normal loss (a quarter at clan war) | [Breaks off](layer.md#cp-cures-pool-break-off). A player who killed it is not hunted or challenged for `AvoidAfterDeath` (30 min); fighting back stays. M8 lifts the death penalty as today. |
| Open-world PK (opt-in) | The killer gains karma; with karma a death costs `RateKarmaExpLost` × the loss, and from `MinimumPKRequiredToDrop` PK kills items may drop | [PK mode](#pk-mode), then [with karma](#with-karma). |

## PK mode

Only a clean player of its tier, picked on the [hunting](targets.md#hunting) preconditions, at
most one per `PKEvery` (1 h), never with PK kills (`UserInfo`) at or above `PKCap` (4, one under
the drop limit, so its items never drop). Until the victim is flagged or has karma the server
refuses `isDebuff` skills even with ctrl, so the bot uses only `AttackRequest` and ctrl-held
direct-damage skills that are not `isDebuff` (Power Strike, Mortal Blow, Power Shot, Drain Health,
Wind Strike, Blaze, Energy Bolt, ...), skipping the row's control, `Hold` and `Int` stuns, slows
and DOT openers (Shield Stun, Stun Attack, Stunning Shot, Stunning Fist, Ice Bolt, Freezing Strike,
Sting, Bleed, Poison, Curse: Poison, Chill Flame, Frost Flame, Dryad Root, Sleep, Wind Shackle,
Entangle, ...). Once the victim hits back and is flagged, the full PvP order runs. The guards are
off for this one victim.

## With karma

After a PK, or a timing accident. While karma (`UserInfo`) is above 0:

- **Clean attackers** stay clean, so [rule 6](targets.md#mayfight) never allows them and killing one
  is another PK. The bot fights back only with PK mode on and the limits passing then (`PKEvery`
  since its last PK, under `PKCap`); the kill is that PK, non-debuff damage only. Else it flees via
  `ruleAttacked`'s flee branch (`rules.go:744-746`, `unfought`, then `flee`) with its `Flee` skill
  (Dash, Sprint) and no `Break` step (`isDebuff` is refused on a clean player). Today `unfought`
  returns only a blacklisted attacker (`combat_rules.go:105-120`); X11 also returns, while karma is
  above 0, a live attacker `MayFight` refuses (hits and hostile casts alike, so nukes count). Unable
  to outrun, it takes the death (`RateKarmaExpLost` × the loss, karma lowered, no drop under
  `PKCap`) over a PK past the limits, then avoids that attacker (`AvoidAfterDeath`).
- **Flagged and karma attackers** are fought under rule 6 and the flag guard: no PK whatever the
  killer's karma (`character_karma.go:44-56`).
- **Restart** at a chaotic point outside town (resurrections accepted as today): no town trip, M8
  death penalty or supply trip until karma is 0. It walks to its spot (M7) on a **karma route**: on
  foot in its SCC, no gatekeeper (gatekeepers and shops refuse it, [server](server.md) Karma and NPC
  services), every segment at least `KarmaClear` (600: the guards' 450 see range plus a margin)
  from every town polygon (X2) and guard spawn (spawn-list NPCs of template type `Guard`,
  `internal/datapack/npc.go:13-15`). With no such route it farms the nearest spot of its band that
  has one until karma is 0. Its flee skips headings entering that margin like blocked ones.
- **Work-off**: it farms monsters at its spot and starts no hunt, challenge or arena trip (a
  karma player cannot duel, and a karma bot in an arena is everyone's target); at karma
  0 its modes and town trips resume.

## Tests

- `TestPvPArenaDeath` (X11): arena deaths one to four within an hour → restart, walk back; the
  fifth → its spot; a death after the first aged past the hour → walk back.
- `TestPvPAvoidAfterDeath` (X11): killed by X in the open world → X not hunted or challenged for 30
  minutes, still fought back.
- `TestPvPPKCap` (X11; WA, HM rows): 4 PK kills → no forced attack; 0 against an unflagged clean
  victim: a WA → `AttackRequest` and ctrl-held 255 Power Smash, never 100 Stun Attack; an HM →
  ctrl-held Wind Strike, never Ice Bolt or Curse: Poison; the victim flagged → the full order; karma
  above 0 → no town trip, farming. With 0 kills and no PK in the hour, no attack with HP under
  `StandHP`, either in a town polygon, or the estimate failing (test `RowTable`); each removed → the
  attack.
- `TestPvPKarma` (X11): karma 240, PK mode off, hit by a clean player → flee (no `Action`,
  `AttackRequest` or cast), no `Break`, Dash if ready; a clean mystic's Wind Strike only → flee;
  karma 0 and a clean player's ctrl-held Wind Strike → in the table, held, not fled, fought once a
  `RelationChanged` shows its flag; unable to outrun → keeps fleeing; PK mode on, 1 PK kill, last PK
  2 hours ago → fights back with `AttackRequest` and ctrl-held non-debuff damage, the PK clock
  restarts; last PK 30 minutes ago or 4 kills → flee; a flagged attacker → fought under the guard; a
  karma attacker → fought. Dead with karma → village restart, then a position outside every town:
  no town trip, a walk to its spot, farming; on a test map with a shorter gatekeeper hop and a
  straight way through a town → walking only, no `Action` or bypass on the gatekeeper, every
  segment 600 from the town polygon and guard spawns; no such route → the nearest band spot with
  one, its own again after karma 0; a flee heading into a town margin → the first heading keeping
  out; no hunt, challenge (declined at once) or arena trip until karma 0, then all resume.
