package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// mysticStart is where the server makes a new human mystic
// (data/xml/classes/humanMystic.xml), about 6500 from the Talking Island
// village trainers.
var mysticStart = botclient.Point{X: -90875, Y: 248162, Z: -3570}

// opMoveBackward is MoveBackwardToLocation's opcode: a ground move.
const opMoveBackward = 0x01

// TestLiveMysticStart runs a low level human mystic from its start point, out
// of sight of any trainer: its trainer errand walks to the Talking Island
// village trainers, and within a few minutes it is over, learned or given
// up, and stays over, never a loop of legs and /unstuck where the bot
// stands (#134). From the start the server routes no leg out
// (acis_golang#5412), and an /unstuck lands the bot back beside it: the
// start is its own restart area.
func TestLiveMysticStart(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, _ := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("hm", 1)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanMystic, travel, func(p *botclient.Persona) {
		// No merchant or Newbie Guide visits: the trainer errand alone.
		p.Shop = botclient.ShopPolicy{}
		p.Newbie.MaxLevel = 0
	})
	t.Cleanup(func() { gmReset(t, gm, f) })

	// The bot plays from the moment it enters the world: one that logged
	// in by the trainers is over its errand before the recall (#241), and
	// a visit done at a level ends the errand until a higher level
	// (trainerVisit.finish). A level up after the recall starts a visit
	// from the start point whatever ran before it.
	level := levelOf(t, ctx, f)
	gmTeleport(t, ctx, gm, mysticStart)
	recallBot(t, ctx, tg, gm, f.name)
	sent, seen, start := len(f.watch.Sent()), f.watch.Len(), time.Now()
	gmOn(t, ctx, gm, f, "set sp 0", fmt.Sprintf("set level %d", level+1))

	moves := func() int {
		n := 0
		for _, p := range f.watch.Sent()[sent:] {
			if p.Op == opMoveBackward {
				n++
			}
		}
		return n
	}
	trained := false
	var logged time.Time
	wctx, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()
	until(t, wctx, f.name+" done with its trainer errand", func() string {
		s := f.bot.Status()
		echoes, _, casts := f.echoes(seen)
		state := fmt.Sprintf("rule %q at %v (%.0f from the trainers), %d moves, %d refused in place, %d /unstuck casts, walk %q",
			s.Rule, s.Position, dist(s.Position, tiTrainers), moves(), echoes, casts, s.Walk)
		if time.Since(logged) >= 10*time.Second {
			logged = time.Now()
			t.Logf("%s after %v: %s", f.name, time.Since(start).Round(time.Second), state)
		}
		switch {
		case s.Rule == "train":
			trained = true
			return state
		case !trained:
			return "no trainer errand yet: " + state
		}
		return ""
	})
	t.Logf("%s left its trainer errand after %v", f.name, time.Since(start).Round(time.Second))

	// Over for good: no walk to the next trainer's spawn.
	_, _, casts := f.echoes(seen)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for end := time.Now().Add(stayOver); time.Now().Before(end); {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-tick.C:
		}
		s := f.bot.Status()
		echoes, _, now := f.echoes(seen)
		if s.Rule == "train" || now > casts {
			t.Fatalf("%s back on its trainer errand: rule %q at %v, %d moves, %d refused in place, %d /unstuck casts",
				f.name, s.Rule, s.Position, moves(), echoes, now)
		}
	}
	t.Logf("%s off its trainer errand for %v: %d moves, %d /unstuck casts", f.name, stayOver, moves(), casts)
}

// stayOver is how long a trainer errand given up stays over in
// TestLiveMysticStart: longer than a walk to the next spawn takes to
// start.
const stayOver = 2 * time.Minute
