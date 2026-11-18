package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Helpers of the M3 scenarios (#115): farmers, brain-driven bots watched
// through their frame tap (Watch), and the GM seeding them.

// Templates the scenarios seed or cast (aCis_datapack/data/xml).
const (
	// healingPotion is Healing Potion: skill 2032, reuse 10 s.
	healingPotion int32 = 1061
	// soulshotNG is Soulshot: No Grade; the human fighter's starting
	// Squire's Sword (2369, equipped) takes one per hit.
	soulshotNG int32 = 1835
	// stem is a droppable material of weight 2: the farmer's junk.
	stem int32 = 1864
	// powerStrike is skill 3 level 1: hit time 1080 ms, reuse 13 s, 10 MP,
	// a sword or blunt weapon. A new human fighter learns it at level 5:
	// the GM sets it.
	powerStrike int32 = 3
)

// spotRegistry is the datapack's farming spots (BOT_E2E_DATAPACK), shared
// by the scenario's farmers, at the server's default spawn multiplier.
func spotRegistry(t *testing.T, tg *Target) *botclient.SpotRegistry {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	return botclient.NewSpotRegistry(dp.Spots(), 1)
}

// farmer is a whole brain-driven Bot farming the spots of a registry, and
// the window on what it reads and writes.
type farmer struct {
	bot   *botclient.Bot
	watch *Watch
	id    int32
	name  string
}

// runFarmer runs a human fighter of account with the persona tune adjusts
// (DefaultPersona when nil) on spots, on the run's gate and a place of its
// own, until the test ends, which logs it out and joins it. It returns once
// the bot is in the world, its character one of the run's.
func runFarmer(t *testing.T, ctx context.Context, tg *Target, account string, spots *botclient.SpotRegistry, tune func(*botclient.Persona)) *farmer {
	t.Helper()
	release, err := tg.Hold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWatch(botclient.SystemClock{})
	cfg := tg.SessionConfig(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cfg.Recorder = w
	p := botclient.DefaultPersona()
	if tune != nil {
		tune(&p)
	}
	brain := botclient.BrainConfig{Persona: p, Spots: spots}
	ch := character(t, account, &botclient.HumanFighter)
	b, err := botclient.NewBot(cfg, account, tg.Password(account), botclient.BotConfig{Character: ch, Brain: brain}, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	if err != nil {
		release()
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Logf("farmer %s stopped: %v", account, err)
		}
		release()
	})
	_, seen, err := w.Await(ctx, 0, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.UserInfo)
		return ok
	})
	if err != nil {
		t.Fatalf("farmer %s in the world: %v", account, err)
	}
	ui := seen.Event.(botclient.UserInfo)
	tg.Own(ui.Name, ui.ObjectID)
	return &farmer{bot: b, watch: w, id: ui.ObjectID, name: ui.Name}
}

// gmTo teleports the GM next to f and waits until it sees f there.
func gmTo(t *testing.T, ctx context.Context, gm *GMClient, f *farmer) {
	t.Helper()
	if err := gm.Command(ctx, "teleportto "+f.name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Fatalf("GM next to %s: %v", f.name, err)
	}
}

// gmTeleport teleports the GM to p and waits until it stands there.
func gmTeleport(t *testing.T, ctx context.Context, gm *GMClient, p botclient.Point) {
	t.Helper()
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", p.X, p.Y, p.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM teleport to %v: %v", p, err)
	}
}

// gmOn brings the GM next to f, selects it and runs cmds on it.
func gmOn(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, cmds ...string) {
	t.Helper()
	gmTo(t, ctx, gm, f)
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
}

// stock is the count of template tmpl an event reports, ok false when it
// reports none: an ItemList lists the whole inventory (0 when the template
// is missing), an InventoryUpdate only what changed.
func stock(ev botclient.Event, tmpl int32) (count int32, objectID int32, ok bool) {
	switch e := ev.(type) {
	case botclient.ItemList:
		for _, it := range e.Items {
			if it.TemplateID == tmpl {
				return it.Count, it.ObjectID, true
			}
		}
		return 0, 0, true
	case botclient.InventoryUpdate:
		for _, c := range e.Changes {
			if c.Item.TemplateID != tmpl {
				continue
			}
			if c.State == botclient.ItemRemoved {
				return 0, c.Item.ObjectID, true
			}
			return c.Item.Count, c.Item.ObjectID, true
		}
	}
	return 0, 0, false
}

// stockSpent reports whether, in seen, the count of tmpl fell below a
// count it had reached, and the template's object id.
func stockSpent(seen []Seen, tmpl int32) (spent bool, objectID int32) {
	var high int32
	for _, s := range seen {
		n, id, ok := stock(s.Event, tmpl)
		if !ok {
			continue
		}
		if id != 0 {
			objectID = id
		}
		if n < high {
			spent = true
		}
		high = max(high, n)
	}
	return spent, objectID
}

// maxPerSecond is the most packets sent within any one second.
func maxPerSecond(sent []Sent) int {
	most, from := 0, 0
	for i, s := range sent {
		for s.At.Sub(sent[from].At) >= time.Second {
			from++
		}
		most = max(most, i-from+1)
	}
	return most
}

// frameCap is the pacer's bound on frames in any second
// (botclient.Pacer: burst 10 plus rate 5), far under the server's 160.
const frameCap = 15

// until checks report every two seconds until it returns "" and fails the
// test with its last answer, what is still missing, once ctx ends.
func until(t *testing.T, ctx context.Context, what string, report func() string) {
	t.Helper()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		missing := report()
		if missing == "" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %v; missing: %s", what, ctx.Err(), missing)
		case <-tick.C:
		}
	}
}
