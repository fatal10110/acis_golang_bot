package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// windStrike is skill 1177 level 1, a mystic's first attack spell: cast
// range 600, hit time 4 s before cast speed, so the server stops the
// caster before it casts.
const windStrike int32 = 1177

// TestLiveFarmMage runs a dark elf mystic farmer that casts Wind Strike at
// its targets, in its own newbie zone, until several of its casts followed
// a move of its own (a cast approach). The server stops a caster before it
// casts and MagicSkillUse carries its position then; the bot stands there,
// so its walker's segments after a cast go out from the server's position:
// runWatched fails the run on any walker desync and names each desync's
// cause (#270, as #246 for Attack). The farmer goes back to level 1 at the
// end.
func TestLiveFarmMage(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 30*time.Minute) // run 1 passed after 16m45s: few casts on a level 1 mystic's MP
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	spots := botclient.NewSpotRegistry(dp.Spots(), 1)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("mg", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.DarkMystic,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.AttackSkills = []int32{windStrike}
			p.MPReserve = 0
			// No loot: carried run after run, it would weigh the
			// character down.
			p.Loot.Pickup = false
			return p
		},
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots = spots },
	})
	t.Cleanup(func() { backToLevel1(t, gm, f) })
	if lvl := levelOf(t, ctx, f); lvl > 1 {
		t.Logf("%s is level %d from an earlier run: back to 1", f.name, lvl)
		gmOn(t, ctx, gm, f, "set level 1")
	}
	gmOn(t, ctx, gm, f, "skill set 1177 1")
	// Out of the farmer's way, in a peace zone out of any monster's reach:
	// a monster that keeps attacking the GM is engaged by another player,
	// which the farmer leaves alone.
	gmTeleport(t, ctx, gm, townTI)
	start, alive := time.Now(), time.Now()

	const wantApproached = 3
	var c castCount
	waitFor(t, ctx, f.name+" casting Wind Strike after its own approaches", func() bool {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
		c = countCasts(f.watch.Seen(0), f.id, windStrike)
		return c.approached >= wantApproached
	})
	t.Logf("%s cast Wind Strike %d times in %s: %d after a move of its own, %d after a server approach (MoveToPawn)",
		f.name, c.casts, time.Since(start).Round(time.Second), c.approached, c.pawn)
}

// castCount counts a caster's casts of one skill: all of them, those after
// a move of the caster's own since its last cast (a walk or a server
// approach), and those after a server approach (MoveToPawn).
type castCount struct {
	casts, approached, pawn int
}

// countCasts counts the casts of skill by caster in seen.
func countCasts(seen []Seen, caster, skill int32) castCount {
	var c castCount
	moved, pawn := false, false
	for _, s := range seen {
		switch e := s.Event.(type) {
		case botclient.MoveToLocation:
			moved = moved || e.ObjectID == caster
		case botclient.MoveToPawn:
			if e.ObjectID == caster {
				moved, pawn = true, true
			}
		case botclient.MagicSkillUse:
			if e.CasterID != caster || e.SkillID != skill {
				continue
			}
			c.casts++
			if moved {
				c.approached++
			}
			if pawn {
				c.pawn++
			}
			moved, pawn = false, false
		}
	}
	return c
}
