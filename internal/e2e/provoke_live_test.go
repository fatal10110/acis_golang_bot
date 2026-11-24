package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// gmProvoke has the GM hit mob and step aside, and returns the monster's
// first landed attack. A monster of the datapack's Warrior AI aggroes
// nobody by sight without an aggro param (script/ai/warrior/base.go
// seeCreature), but fights back whoever hits it (OnAttacked). The GM stops
// at its second swing, its first hit resolved, or once the monster hits
// back: the teleport aside idles it (instant_teleport.go goIdle) and the
// monster follows the GM it hates. An attack the GM does not start within
// provokeRetry is asked again; a GM that never swings fails with what it
// read (provokeLog). A monster that dies before it fights back returns a
// zero Attack: a bot assisting the GM may have killed it, so the caller
// asserts on the bot's own attacks (TestLiveArrows: the archer's bow kills
// a wolf in a few shots).
func gmProvoke(t *testing.T, ctx context.Context, gm *GMClient, mob int32) (_ botclient.Attack, tries int) {
	t.Helper()
	if err := gm.Select(ctx, mob); err != nil {
		t.Fatal(err)
	}
	pl := provokeLog{gm: gm.Self.ObjectID, mob: mob, hp: userInfo(t, gm.Client).HP}
	attack := func() {
		pl.tries++
		pl.last = time.Now()
		if err := gm.Send(ctx, botclient.EncodeAttackRequest(mob, gm.Pos())); err != nil {
			t.Fatal(err)
		}
	}
	attack()
	hitsBack := func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.Attack:
			return e.AttackerID == mob && len(e.Hits) > 0
		case botclient.Die:
			return e.ObjectID == mob
		}
		return false
	}
	swings := 0
	ev, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		pl.note(ev)
		if a, ok := ev.(botclient.Attack); ok && a.AttackerID == gm.Self.ObjectID &&
			len(a.Hits) > 0 && a.Hits[0].TargetID == mob {
			swings++
			return swings == 2
		}
		if swings == 0 && time.Since(pl.last) >= provokeRetry {
			attack()
		}
		return hitsBack(ev)
	})
	if err != nil {
		t.Fatalf("the GM hits the monster: %v; %s", err, pl)
	}
	if _, ok := ev.(botclient.Die); ok {
		// Dead before it fought back: a bot assisting the GM may have
		// killed it, which the caller checks (its own attacks).
		return botclient.Attack{}, pl.tries
	}
	p := gm.Pos()
	gmTeleport(t, ctx, gm, botclient.Point{X: p.X + 150, Y: p.Y, Z: p.Z})
	if a, ok := ev.(botclient.Attack); ok && a.AttackerID == mob {
		return a, pl.tries
	}
	// The step aside is blind: a monster that finds no path to the GM's
	// landing, or gives up on it, swings at no one, and the read times out
	// (#138). What the GM read then says which.
	if ev, err = gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		pl.note(ev)
		return hitsBack(ev)
	}); err != nil {
		t.Fatalf("the monster fights back: %v; %s", err, pl)
	}
	if _, ok := ev.(botclient.Die); ok {
		return botclient.Attack{}, pl.tries
	}
	return ev.(botclient.Attack), pl.tries
}

// provokeRetry is how long the GM waits for its first swing at the monster
// before it asks for the attack again: one AttackRequest per try, a few in
// all, far under the frame pacing.
const provokeRetry = 8 * time.Second

// provokeLog is what the GM read while it tried to hit the monster: why
// an attack never started.
type provokeLog struct {
	gm, mob     int32
	tries       int
	last        time.Time
	hp          int32
	gmDied      bool
	failed      int
	msgs        []int32
	mobInfo     int
	mobGone     bool
	mobAttacked int
	// mobChased and mobWalked count the monster's MoveToPawn (after
	// someone) and MoveToLocation (to a point: home, or a path leg).
	mobChased, mobWalked int
}

func (pl *provokeLog) note(ev botclient.Event) {
	switch e := ev.(type) {
	case botclient.ActionFailed:
		pl.failed++
	case botclient.SystemMessage:
		pl.msgs = append(pl.msgs, e.ID)
	case botclient.UserInfo:
		if e.ObjectID == pl.gm {
			pl.hp = e.HP
		}
	case botclient.Die:
		pl.gmDied = pl.gmDied || e.ObjectID == pl.gm
	case botclient.NPCInfo:
		if e.ObjectID == pl.mob {
			pl.mobInfo++
		}
	case botclient.DeleteObject:
		pl.mobGone = pl.mobGone || e.ObjectID == pl.mob
	case botclient.Attack:
		if e.AttackerID == pl.mob {
			pl.mobAttacked++
		}
	case botclient.MoveToPawn:
		if e.ObjectID == pl.mob {
			pl.mobChased++
		}
	case botclient.MoveToLocation:
		if e.ObjectID == pl.mob {
			pl.mobWalked++
		}
	}
}

func (pl provokeLog) String() string {
	return fmt.Sprintf("%d attack requests, GM HP %d (died %v), %d ActionFailed, system messages %v; monster: %d NPCInfo after the spawn, deleted %v, %d attacks, %d chases, %d walks",
		pl.tries, pl.hp, pl.gmDied, pl.failed, pl.msgs, pl.mobInfo, pl.mobGone, pl.mobAttacked, pl.mobChased, pl.mobWalked)
}

// tiSpots is a place among the Talking Island newbie spots, in walking
// reach of them.
var tiSpots = botclient.Point{X: -75296, Y: 254141, Z: -3328}

// gmVisibleAt makes the GM visible, teleporting it to p: //hide toggles
// invisibility and answers with the GM's UserInfo, the last one read before
// the teleport lands.
func gmVisibleAt(t *testing.T, ctx context.Context, gm *GMClient, p botclient.Point) {
	t.Helper()
	for range 2 {
		if err := gm.Command(ctx, "hide"); err != nil {
			t.Fatal(err)
		}
		if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", p.X, p.Y, p.Z)); err != nil {
			t.Fatal(err)
		}
		var last botclient.UserInfo
		if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
			if u, ok := ev.(botclient.UserInfo); ok && u.ObjectID == gm.Self.ObjectID {
				last = u
			}
			tp, ok := ev.(botclient.TeleportToLocation)
			return ok && tp.ObjectID == gm.Self.ObjectID
		}); err != nil {
			t.Fatalf("GM hide and teleport: %v", err)
		}
		if last.ObjectID == 0 {
			t.Fatal("no UserInfo answered //hide")
		}
		if last.Abnormal&botclient.AbnormalStealth == 0 {
			return
		}
	}
	t.Fatal("the GM is still invisible after two //hide")
}

// awaitDeath waits up to deathBound until f sees mob die, a monster the
// run spawned, and drops it from the run's monsters.
func awaitDeath(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, mob int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, deathBound)
	defer cancel()
	if _, _, err := f.watch.Await(ctx, 0, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && d.ObjectID == mob
	}); err != nil {
		t.Fatalf("the monster dies: %v", err)
	}
	gm.Forget(mob)
}

// deathBound is how long awaitDeath waits: one wolf takes a farmer a few
// swings.
const deathBound = 3 * time.Minute
