# The Olympiad

Ships in X16 (#604). Decision D42 (proposed, needs owner approval). Paths per
[README](README.md#paths). Today no bot is noble, so none can enter.

## Server facts

| Topic | Server behavior | Evidence |
| --- | --- | --- |
| Noblesse | Only nobles compete. On the owner's server the class master NPC 50000 (D15) grants noblesse for free, at any level, on its `become_noble` link, with the noble skills and a `UserInfo`. The quest path needs a subclass at 75. | `network/classmaster.go:34-40`, `:109-120` (`makeNoble`); `classmaster/command.go:57-58`; `aCis_datapack/data/html/mods/classmaster/50000.htm` |
| Window | Competition from `OlyStartTime` 18:00 for `OlyCPeriod` 6 h; registration refused outside it and in its last 10 minutes. No GM command opens it (`//endoly` ends the period, `//sethero` sets a hero). | `ops/docker/config/events.properties:4-23`; `olympiad/registration.go:18-50` (`RegisterClosing`); `network/admin_olympiad.go:3-15` |
| Registering | The Olympiad manager's bypass `OlympiadNoble <choice>`: 4 registers for non-classed games, 5 for classed ones, 1 unregisters, 10 trades points for Noblesse Gate Passes. Refused (one system message each) when not noble, on a subclass, with a cursed weapon, overweight, already registered, in a match, with no points. | `network/olympiad_manager.go:21-60` (`:42-47`), `:87-130`, `:148`; `olympiad/registration.go:18-50` |
| Draw | A classed game needs `OlyClassedParticipants` (5) registered of one class, a non-classed one `OlyNonClassedParticipants` (9) of any class: with few nobles nobody is drawn. | `olympiad/games.go:177`, `:205`; `events.properties:35-38` |
| Match | Told 1492 (stadium in S1 s), the competitor is force-teleported into a stadium; there it loses its effects, clan and hero skills, party, mount or pet, other players' cubics, forbidden items and automatic shots; it is buffed and healed, waits, then fights `OlyBattle` (6 min) without force; dead, offline, detached, observing, on a subclass, cursed, overweight or outside the stadium forfeits. The end sends `ExOlympiadMatchEnd` and points (1657, 1658), then the competitor back to where it was. Packets: `ExOlympiadUserInfo`, `ExOlympiadSpelledInfo`, `ExOlympiadMode`, `ExOlympiadMatchEnd` (ex 0x29-0x2c); messages 1492-1498, 1657, 1658, 1692, 1858. | `network/olympiad_match.go:300-330` (`Fitness`, `away`), `:357-380` (`Enter`), `:391-440` (`stripCompetitor`), `:455-483`, `:574-590` (`SendBack`); `network/serverpackets/olympiad_match.go:7-38`; `character_social.go:198-205` |
| Heroes | At the period's end one hero per third-class profession is elected among nobles with at least `OlyMinMatchesToBeClassed` (9) matches; an elected hero is inactive until it claims the status at a Monument of Heroes. | `hero/hero.go:1-8`, `:85-87`; `olympiad/olympiad.go:76-81`; `events.properties:32` |

## What the bot does

- **Noblesse.** A bot with `Olympiad` on takes noblesse on its next class master visit (one bypass
  in the visit D15 already makes, under its `bypassGap`). Its rows ignore the noble skills.
- **Registering.** Inside the window and at least 10 minutes before its end, in a town with an
  Olympiad manager on its way, a bot with `Olympiad` on, in no party (a match removes it from its
  party, which a bot party would lose), on its base class, not overweight and with points registers
  for classed games (choice 5, the smaller draw) once; it checks each refusal reason itself first,
  so a refusal (logged with its message id) stops it for the day.
- **While registered** it waits in that town (no farming: a death, a trip out of the town or a
  dungeon would forfeit the draw) for at most `OlympiadWait` (30 min), then unregisters (choice 1)
  and farms; at most `OlympiadTries` (2) a day. A forced teleport (1492, then the jump) is
  expected, not a nav desync.
- **In the match.** [MayFight](targets.md#mayfight) gains a rule 0, like the duel's rule 1: from
  the fight start to `ExOlympiadMatchEnd`, only the drawn opponent, any class, a bot included (a
  draw cannot be declined; D38 gains the stadium beside the arena). Before the fight it rebuffs
  and turns its automatic shots back on (the strip turned them off) inside the wait; it fights
  with its PvP order to the end, no break-off and no flee (leaving the stadium forfeits). 283 and
  284 inside a stadium during a match are the stadium's.
- **After.** Sent back, it resumes its persona.
- **Never:** spectates, trades points for passes, claims hero status at a Monument of Heroes.

## Risks

- **Players' games.** Every registered bot fills a draw a player could have had, and a noble bot
  makes draws happen that would not have (5 of a class); opt-in per bot keeps the number known.
- **Heroes.** A bot cannot be elected before its third profession (out of M10-M13's scope); a third
  profession bot would compete for a hero slot. Until the owner rules, `Olympiad` is refused for a
  third profession bot, and a bot never claims at a Monument.
- **Noblesse.** Taking it is a lasting character change (noble skills, the noble record), on the
  bots' own characters only.
- **Time.** A registered bot waits in town; `OlympiadWait` and `OlympiadTries` bound it.
- **Pacing.** One noblesse bypass per bot ever, at most `OlympiadTries` registrations and
  unregistrations a day, each under `bypassGap` (the manager is an NPC dialog like any).

## Mode

`Olympiad` (persona default, D6): off for every preset, opt-in per bot on the panel (X12's list).

## Tests

- `TestOlympiadFrames` (X16, codec): recorded `ExOlympiadMode`, `ExOlympiadUserInfo`,
  `ExOlympiadMatchEnd` decode, each consuming `len(payload)`.
- `TestOlympiadRegister` (X16): `Olympiad` on, not noble → `become_noble` on the class master
  visit; inside the window → choice 5 once; 9 minutes before the end, in a party, overweight or on
  a subclass → none; 30 minutes registered → choice 1, farming; a refusal → no retry that day.
- `TestOlympiadMatch` (X16): 1492, teleport, `ExOlympiadMode` → no target until the fight starts,
  then only the opponent (a bot too), no flee or break-off, shots back on; `ExOlympiadMatchEnd` →
  persona resumes; 283 in a stadium during a match → not a siege or arena.
- `TestLiveOlympiadRegister` (X16, live): a bot takes noblesse at the class master and registers;
  the manager confirms; it unregisters after `OlympiadWait` (shortened). It runs only inside the
  window (18:00-24:00 server time): [owner question 1](#owner-questions). A live match needs 5
  nobles of one class, over X13's 4 characters: [owner question 2](#owner-questions).

## Owner questions

1. No GM command opens the competition window. Run the live scenario only in the evening window,
   or give the test stack an `OlyStartTime` that keeps it open (a server setting)?
2. A live match needs 5 nobles of one class (or 9 for non-classed): allow a 5-character scenario,
   or cover the match with recorded frames only?
3. Noblesse for any `Olympiad` bot at the class master (proposed), or only bots the owner marks?
