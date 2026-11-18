package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveAntiKS has a farmer find an aggressive monster fighting the GM
// at its spot: it leaves the monster alone past EngagedWindow, and once the
// GM is gone it takes the monster on (#28). The monster appears while the
// farmer is away, recalled to town, so it cannot be the first to attack
// it. A monster that goes for someone else instead is fought, and the GM
// tries again.
func TestLiveAntiKS(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	spots := spotRegistry(t, tg)
	gm := enterGM(t, ctx, tg)
	home := townTI // a peace zone far from the spots: the monster loses the GM there
	account, _ := tg.Account("k", 1)
	f := runFarmer(t, ctx, tg, account, spots, nil)
	window := botclient.DefaultPersona().EngagedWindow
	waitFor(t, ctx, "the farmer at its spot", func() bool { return f.bot.Status().Rule == "farm" })

	// A GM logs in invisible (GMStartupInvisible), and no monster attacks
	// an invisible player.
	gmVisibleAt(t, ctx, gm, home)

	const attempts = 3
	for attempt := 1; ; attempt++ {
		spot := f.bot.Status().Position
		gmTeleport(t, ctx, gm, home)
		if err := gm.Command(ctx, "recall "+f.name); err != nil {
			t.Fatal(err)
		}
		if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
			t.Fatalf("%s recalled to town: %v", f.name, err)
		}
		gmTeleport(t, ctx, gm, spot)
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
		leash := int64(botclient.DefaultPersona().Spots.Leash)
		until(t, scenarioCtx(t, 5*time.Minute), "the farmer back at its spot", func() string {
			if at := f.bot.Status().Position; !within(at, spot, leash) {
				return fmt.Sprintf("%s at %v, within %d of %v", f.name, at, leash, spot)
			}
			return ""
		})
		back := time.Now()
		t.Logf("%s back at its spot %v after the monster engaged the GM", f.name, back.Sub(engaged))
		hold := time.NewTimer(2 * window)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-hold.C:
		}
		hit, click := mobContact(f, mob, engaged)
		if !click.IsZero() && (hit.IsZero() || click.Before(hit)) {
			t.Fatalf("the farmer clicked the monster %v after it came back, before the monster hit it", click.Sub(back))
		}
		t.Logf("the farmer left the GM's monster alone for %v at its spot (first hit on the farmer: %v)", 2*window, !hit.IsZero())

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
