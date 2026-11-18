package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveAntiKS spawns an aggressive monster next to the GM, a few steps
// from a farmer at its spot: while the monster fights the GM the farmer
// leaves it alone past EngagedWindow, and once the GM is gone it takes the
// monster on (#28). A monster that goes for the farmer instead is fought,
// as it should be, and the GM tries again.
func TestLiveAntiKS(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	home := gm.Pos()
	account, _ := tg.Account("k", 1)
	f := runFarmer(t, ctx, tg, account, spots, nil)
	window := botclient.DefaultPersona().EngagedWindow
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })

	const attempts = 3
	for attempt := 1; ; attempt++ {
		gmTo(t, ctx, gm, f)
		p := gm.Pos()
		gmTeleport(t, ctx, gm, botclient.Point{X: p.X + 400, Y: p.Y, Z: p.Z})
		mob, err := gm.Spawn(ctx, gremlin)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
		ev, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
			a, ok := ev.(botclient.Attack)
			return ok && a.AttackerID == mob && len(a.Hits) > 0
		})
		if err != nil {
			t.Fatalf("the monster attacks: %v", err)
		}
		if prey := ev.(botclient.Attack).Hits[0].TargetID; prey != gm.Self.ObjectID {
			t.Logf("attempt %d: the monster went for %d, not the GM", attempt, prey)
			if attempt == attempts {
				t.Fatalf("the monster never went for the GM in %d attempts", attempts)
			}
			gmTeleport(t, ctx, gm, home)
			awaitDeath(t, ctx, gm, f, mob)
			continue
		}
		engaged := time.Now()
		hold := time.NewTimer(window + 5*time.Second)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-hold.C:
		}
		hit, click := mobContact(f, mob, engaged)
		if !click.IsZero() && (hit.IsZero() || click.Before(hit)) {
			t.Fatalf("the farmer clicked the monster %v after the GM engaged it, before it hit the farmer", click.Sub(engaged))
		}
		t.Logf("the farmer left the GM's monster alone for %v (first hit on the farmer: %v)", window+5*time.Second, !hit.IsZero())

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
