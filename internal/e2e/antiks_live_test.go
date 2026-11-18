package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveAntiKS has a farmer find a monster fighting the GM
// at its spot: it leaves the monster alone past EngagedWindow, and once the
// GM is gone it takes the monster on (#28). The farmer is told to stay
// while the monster appears, so it cannot be the first to attack it, and
// sent back to its spot once the monster fights the GM, which hit it first
// (gmProvoke). A monster that goes
// for the farmer instead is fought, and the GM tries again.
func TestLiveAntiKS(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	home := townTI // a peace zone far from the spots: the monster loses the GM there
	account, _ := tg.Account("k", 1)
	f := runFarmer(t, ctx, tg, account, spots, nil)
	window := botclient.DefaultPersona().EngagedWindow
	levelOf(t, ctx, f) // in the world
	if at := f.bot.Status().Position; within(at, townTI, 3000) {
		// A farmer walks no way out of the village: the GM brings it to
		// the newbie spots.
		t.Logf("%s is in town at %v: the GM brings it out", f.name, at)
		gmTeleport(t, ctx, gm, tiSpots)
		if err := gm.Command(ctx, "recall "+f.name); err != nil {
			t.Fatal(err)
		}
		if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
			t.Fatalf("%s recalled: %v", f.name, err)
		}
	}
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })
	spot := f.bot.Status().Spot

	// A GM logs in invisible (GMStartupInvisible), and no monster attacks
	// an invisible player.
	gmVisibleAt(t, ctx, gm, home)

	const attempts = 3
	for attempt := 1; ; attempt++ {
		order(t, ctx, f, botclient.Command{Kind: botclient.CommandStay})
		gmTo(t, ctx, gm, f)
		p := gm.Pos()
		gmTeleport(t, ctx, gm, botclient.Point{X: p.X + 400, Y: p.Y, Z: p.Z})
		mob, err := gm.Spawn(ctx, wolf)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
		attack := gmProvoke(t, ctx, gm, mob)
		order(t, ctx, f, botclient.Command{Kind: botclient.CommandGo, Arg: spot})
		if prey := attack.Hits[0].TargetID; prey != gm.Self.ObjectID {
			t.Logf("attempt %d: the monster went for %d, not the GM", attempt, prey)
			if attempt == attempts {
				t.Fatalf("the monster never went for the GM in %d attempts", attempts)
			}
			gmTeleport(t, ctx, gm, home)
			awaitDeath(t, ctx, gm, f, mob)
			continue
		}
		engaged := time.Now()
		hold := time.NewTimer(2 * window)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-hold.C:
		}
		hit, click := mobContact(f, mob, engaged)
		if !click.IsZero() && (hit.IsZero() || click.Before(hit)) {
			t.Fatalf("the farmer clicked the monster %v after the GM engaged it, before it hit the farmer", click.Sub(engaged))
		}
		t.Logf("the farmer left the GM's monster alone for %v (first hit on the farmer: %v)", 2*window, !hit.IsZero())

		gmTeleport(t, ctx, gm, home)
		until(t, scenarioCtx(t, 3*time.Minute), "the farmer takes the monster on", func() string {
			if _, click := mobContact(f, mob, engaged); click.IsZero() {
				return "an Action or AttackRequest on the monster"
			}
			return ""
		})
		awaitDeath(t, ctx, gm, f, mob)
		return
	}
}

// gmProvoke has the GM hit mob and step aside, and returns the monster's
// first landed attack. A monster of the datapack's Warrior AI aggroes
// nobody by sight without an aggro param (script/ai/warrior/base.go
// seeCreature), but fights back whoever hits it (OnAttacked). The GM stops
// at its second swing, its first hit resolved, or once the monster hits
// back: the teleport aside idles it (instant_teleport.go goIdle) and the
// monster follows the GM it hates. An attack the GM does not start within
// provokeRetry is asked again; a GM that never swings fails with what it
// read (provokeLog).
func gmProvoke(t *testing.T, ctx context.Context, gm *GMClient, mob int32) botclient.Attack {
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
		t.Fatal("the GM killed the monster before it fought back")
	}
	p := gm.Pos()
	gmTeleport(t, ctx, gm, botclient.Point{X: p.X + 150, Y: p.Y, Z: p.Z})
	if a, ok := ev.(botclient.Attack); ok && a.AttackerID == mob {
		return a
	}
	if ev, err = gm.Await(ctx, 30*time.Second, hitsBack); err != nil {
		t.Fatalf("the monster fights back: %v", err)
	}
	if _, ok := ev.(botclient.Die); ok {
		t.Fatal("the GM killed the monster before it fought back")
	}
	return ev.(botclient.Attack)
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
	}
}

func (pl provokeLog) String() string {
	return fmt.Sprintf("%d attack requests, GM HP %d (died %v), %d ActionFailed, system messages %v; monster: %d NPCInfo after the spawn, deleted %v, %d attacks",
		pl.tries, pl.hp, pl.gmDied, pl.failed, pl.msgs, pl.mobInfo, pl.mobGone, pl.mobAttacked)
}

// tiSpots is where a farmer k1 farmed, among the Talking Island newbie
// spots, in walking reach of them.
var tiSpots = botclient.Point{X: -75296, Y: 254141, Z: -3328}

// order hands c to f's bot and waits for its answer.
func order(t *testing.T, ctx context.Context, f *farmer, c botclient.Command) {
	t.Helper()
	answer, err := f.bot.Command(c)
	if err == nil {
		select {
		case err = <-answer:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	if err != nil {
		t.Fatalf("%s told %q: %v", f.name, c, err)
	}
}

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

// mobContact is when, since from, mob first hit f and f first clicked mob
// (Action or AttackRequest); zero for none.
func mobContact(f *farmer, mob int32, from time.Time) (hit, click time.Time) {
	for _, s := range f.watch.Seen(0) {
		if a, ok := s.Event.(botclient.Attack); ok && hit.IsZero() && !s.At.Before(from) &&
			a.AttackerID == mob && len(a.Hits) > 0 && a.Hits[0].TargetID == f.id {
			hit = s.At
		}
	}
	for _, p := range f.watch.Sent() {
		if (p.Op == botclient.OpAction || p.Op == botclient.OpAttackRequest) && p.Target == mob && !p.At.Before(from) {
			return hit, p.At
		}
	}
	return hit, time.Time{}
}

// awaitDeath waits until f sees mob die, a monster the run spawned, and
// drops it from the run's monsters.
func awaitDeath(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, mob int32) {
	t.Helper()
	if _, _, err := f.watch.Await(ctx, 0, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && d.ObjectID == mob
	}); err != nil {
		t.Fatalf("the monster dies: %v", err)
	}
	gm.Forget(mob)
}
