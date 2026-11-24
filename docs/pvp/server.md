# Server facts, the bot today, pacing

Read in the Go server and the datapack on 2026-11-24. Path conventions: [README](README.md#paths).
Nothing the design needs is missing on the server and nothing differs from the reference, so this
plan files no server issue. Two facts shape the design and are not gaps: a bot sees another
player's HP once per selection, and no `RelationChanged` follows an arena entry or a duel start
(the reference server does the same).

## Server behavior

By topic: [duels](server-duels.md) (challenge, answer, run, end, flood gates); [flags, arenas and
karma](server-karma.md) (relations, forcing, flags, kill credit, death, karma); [casts and
combat](server-casts.md) (area reach, run speed, hit and cast times, Esc, land rates, cast break);
[PvP items](server-items.md) (CP potions, cures, jewels, weapons, sets, self buffs).

## The bot today

| Concern | Today | Where |
| --- | --- | --- |
| Whom it fights | Attackable NPCs, flagged or karma players (`FightFlagged`); never a clean player (`ForceAttack` off); never supports a flagged player (`SupportFlagged` off), but the gate reads only the flag (`SupportFlagged \|\| o.PvPFlag == 0`), so an unflagged karma player passes although healing one flags the caster. A persona default (D6). | `combat_rules.go` `PvPPolicy`, `MayFight`, `MaySupport` (`:71-73`); `rules.go` `DefaultPersona` (`PvP: PvPPolicy{FightFlagged: true}`) |
| Fighting back | Every `Attack` hit puts its attacker in the attackers table unchecked (`applyAttack`, `:74-78`); a `MagicSkillUse` at the bot only from an `Attackable` monster or summon or a flagged or karma player (`hostileCaster`, `:107-120`), beneficial casts included. It fights the hardest hitter `MayFight` allows and drops a target that unflags. | `combat_world.go` `applyAttack`, `applyMagicSkillUse`, `hostileCaster`, `flagged`; `rules.go` `ruleAttacked` |
| Player relation | `RelationChanged` decoded: auto-attackable, karma, PvP flag. | `combat.go` `decodeRelationChanged`; `world.go` (`case RelationChanged`) |
| Forced attack | `AttackRequest` and ctrl-held casts on a clean player, only when `ForceAttack` lets `MayFight` pass. | `brainpackets.go` `encodeAttackRequest`, `encodeRequestMagicSkillUse`; `combat_rules.go` `fight`, `forced` |
| Another player | `CharInfo`: name, class id, PvP flag, karma, speeds, sitting, combat, dead, store. Skipped: paperdoll, clan, ally, abnormal mask, CP, duel team. HP only from a `StatusUpdate`. | `inworld.go` `decodeCharInfo` (`r.Skip(5 * 4) // clan, crest, ally, ally crest, relation`); `world.go` `applyStatus` |
| Its own CP, clan, effects | CP decoded, unused; clan id kept; ally id, PK and PvP counts skipped; own effects from `AbnormalStatusUpdate`. | `world.go` (`Self.CP`, `applyClanID`), `inworld.go`; `support_test.go` |
| Duels | No duel packet decoded or sent. | `opcodes.go` |
| Combat | One M10 rotation for monsters and players: no control-first order, interrupt or class read. | `docs/strategy.md` "Behavior per class" (S6, S6b, S7) |
| Potions | HP potions in `ruleCritical`; no CP potion. | `items.go` `healingPotions`, `drinkPotion` |
| Death | Accept a resurrection, else restart in the village; M8 death penalty scrolls; same for every death. | `death.go` `RecoverPolicy`; `rules.go:422` `restartToVillage` (wired at `:226`), `ruleDead` |
| Whispers | GM commands (D3), the crafter's (D20); no `duel`; bot-marked names never answered. | `chat.go` `ruleChat`; `command.go` |

## Pacing

Every sender goes through the per-connection frame bucket (`pacing.go` `frameRate` 5 a second,
`frameBurst` 10: at most 15 frames in any second and 35 in any 5 s, against the server's 160 a
second and 400 in 5 s). PvP sends share those 5 a second with everything else; the brain sends at
most one per decision, in the [PvP order](layer.md#pvp-order) with the Esc cancel first.

| New sender | What bounds it |
| --- | --- |
| `RequestDuelAnswerStart` | [Sources](finding.md#sources), Bound; inside the 15 s window, never repeated. |
| `RequestDuelStart` | [Sources](finding.md#sources), Bound; the server keeps one open request per target. No server gate. |
| Hunting attacks, control, interrupts | Casts as today: one in flight, the next after its `MagicSkillUse` or `castTimeout`, then reuse; at most one interrupt per opponent cast. Melee clicks once per target. |
| Kite moves, arena trips | Walker segments as today (one checked segment from standstill); gatekeeper bypasses under `bypassGap` and reply timeouts. |
| CP potions, cures | `UseItem` at most once per 500 ms item reuse and the potion reuse; a cure once per poison or bleed effect seen. |
| Whispered `duel` | Answered by a challenge, not chat; the chat policy's per-sender limit applies to anything said. |
| Esc cancel (`RequestTargetCancel`) | At most one per own damage or `nextActionAttack` cast in flight, only at a [guard](guards.md) stop: the opponent's flag (for a summon it fights, the summon's own) turning to state 2 (at most once a second); a karma-free arena opponent nearing the edge or seen outside, a counted root ending, or its own `MagicSkillUse` carrying a longer hit time than predicted; or before leaving an arena. |
| Re-select after a late Esc | One `Action` (a selection) per `TargetUnselected` following its own Esc, before its next step on that opponent. |
| Disengage moves | One per blink or arena-edge stop, after the Esc, or for a `nextActionAttack` cast in flight after it ends (a new arena-edge stop only after the opponent was back inside); one per duel-ground crossing (the next only after a new `Action` or cast, sent once the check passes). |
| Pole reposition | One disengage move, then one walker segment; a new stand point only after reaching the last or a bystander moved, at most once per attack interval. A hold sends nothing. |
| Target refresh | None: the bot never re-selects to read HP. |
| Fleeing with karma | Today's `flee` and the `Flee` skill under its reuse; a fight back as a PK only under `PKEvery` (1 h) and `PKCap`; the karma route walk sends no gatekeeper bypass. |
