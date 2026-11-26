package e2e

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Shot materials scenario (#191): a Saber (123), a D weapon a recipe makes
// (so the crafter's cycle deposits it as crafting stock), breaks into 743
// Crystals D (1458) worth more than its reference price; Recipe: Soulshot D
// (recipes.xml recipe 20, scroll 1804, Create Item 2) makes 156 Soulshots D
// (1463) of three Soul Ore (1785) and one Crystal D.
const (
	saber          int32 = 123
	crystalD       int32 = 1458
	soulOre        int32 = 1785
	soulshotD      int32 = 1463
	soulshotScroll int32 = 1804
	crystallize    int32 = 248
	opSysMsg             = 0x64
	opCrystallize        = 0x72
	// msgItemCrystallized is ItemCrystallized (serverpackets/systemmessage.go).
	msgItemCrystallized int32 = 1258
)

// crystalTap hands the test the clan warehouse lists and the
// ItemCrystallized messages the bot reads, and counts the
// RequestCrystallizeItem frames it sends. It runs on the bot's reader and
// writer goroutines and keeps no payload.
type crystalTap struct {
	out  chan botclient.Event
	sent chan struct{}
}

func (c crystalTap) Record(payload []byte) {
	if len(payload) == 0 || payload[0] != opWHList && payload[0] != opSysMsg {
		return
	}
	ev, _, err := botclient.DecodeEvent(payload)
	if err != nil {
		return
	}
	if m, ok := ev.(botclient.SystemMessage); ok && m.ID != msgItemCrystallized {
		return
	}
	select {
	case c.out <- ev:
	default:
	}
}

func (c crystalTap) RecordSent(payload []byte) {
	if len(payload) > 0 && payload[0] == opCrystallize {
		select {
		case c.sent <- struct{}{}:
		default:
		}
	}
}

// TestLiveCrystallize plays the shot materials of the clan crafter (#191):
// a Dwarven Fighter given Crystallize 1 and Create Item 2 by the GM (the
// Artisan's levels; the class master is fatal10110/acis_golang#5419) leads
// a fresh level-1 clan, carrying a Saber, three Soul Ore and the Soulshot D
// recipe. Its first cycle, with nothing to craft, deposits them as crafting
// stock. The GM then posts an order for 156 Soulshots D, so the next cycle
// withdraws the Saber with the ore, crystallizes the Saber (one
// RequestCrystallizeItem, answered ItemCrystallized), crafts the soulshots
// from the ore and the crystals, and deposits them with the crystals left.
// The scenario asserts the clan lists before and after: the Saber gone, the
// soulshots and the crystals in, the ore spent. It takes about six minutes.
func TestLiveCrystallize(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 14*time.Minute)
	gm := enterGM(t, ctx, tg)
	tap := crystalTap{out: make(chan botclient.Event, 64), sent: make(chan struct{}, 8)}
	travel := travelMap(t, tg)
	board := botclient.NewCraftBoard()
	class := botclient.DwarvenFighter
	botAcc, _ := tg.Account("crz", 1)
	said := withClanSaid(tap)
	b := runBotWith(t, ctx, tg, botAcc, botSetup{tap: said, class: &class, persona: crafterPersona, brain: func(cfg *botclient.BrainConfig) {
		withTravel(travel)(cfg)
		cfg.CraftBoard = board
	}})
	name := botOnline(t, ctx, tg, b)
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", nearKeeper.X, nearKeeper.Y, nearKeeper.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM next to the keeper: %v", err)
	}
	botID := recallBot(t, ctx, tg, gm, name)
	leaveOldClan(t, ctx, gm, b, botID, name)
	if b.Status().Level < clanCreateLevel {
		onBot(t, ctx, gm, botID, "set level "+strconv.Itoa(clanCreateLevel))
		waitFor(t, ctx, "the bot at the founding level", func() bool { return b.Status().Level >= clanCreateLevel })
	}
	onBot(t, ctx, gm, botID,
		fmt.Sprintf("skill set %d 2", createItem), fmt.Sprintf("skill set %d 1", crystallize),
		fmt.Sprintf("give 57 %d", whAdena), fmt.Sprintf("give %d 1", soulshotScroll),
		fmt.Sprintf("give %d 1", saber), fmt.Sprintf("give %d 3", soulOre),
	)
	waitFor(t, ctx, "the crafter with Create Item 2", func() bool {
		c := b.Status().Crafter
		return c != nil && c.CreateItem >= 2
	})
	clan, clanID := foundBotClan(t, ctx, gm, b, botID, name, "cz", said.said)
	onBotJoined(t, ctx, gm, botID, name, "pledge level 1")
	if _, err := gm.Await(ctx, EntryTimeout, clanLevelSet(clan, 1)); err != nil {
		t.Fatalf("clan %s at level 1: %v", clan, err)
	}
	waitFor(t, ctx, "a crafter cycle with nothing to craft", func() bool {
		c := b.Status().Crafter
		return c != nil && c.Last.Outcome == botclient.CraftCycleNothing
	})
	order, ok := board.PostCommand(clanID, botclient.OrderByGM, "e2e GM", soulshotD, 156)
	if !ok {
		t.Fatal("GM order refused")
	}

	var before, after czStock
	crystallized, sent := false, 0
	// The step's bound: the crystallization and the deposit after it
	// (docs/e2e-live.md, "Step deadlines").
	overdue := time.After(stepBound)
	for done := false; !done; {
		select {
		case <-tap.sent:
			sent++
		case ev := <-tap.out:
			switch e := ev.(type) {
			case botclient.SystemMessage:
				t.Logf("crystallized: %v", e.Text)
				if before.saber < 1 {
					t.Fatalf("crystallized while the clan warehouse held %+v, want the Saber withdrawn from it", before)
				}
				crystallized = true
			case botclient.WarehouseWithdrawList:
				if e.Type != clanWHType {
					continue
				}
				n := czStockOf(e)
				t.Logf("clan warehouse list: %d items, %+v", len(e.Items), n)
				if !crystallized {
					before = n
				} else if n.shots > before.shots {
					after, done = n, true
				}
			}
		case <-overdue:
			t.Fatalf("waiting for the soulshots in the clan warehouse (crystallized %v): overdue after %v; crafter %+v", crystallized, stepBound, b.Status().Crafter)
		case <-ctx.Done():
			t.Fatalf("waiting for the soulshots in the clan warehouse (crystallized %v): %v; crafter %+v", crystallized, ctx.Err(), b.Status().Crafter)
		}
	}
	// A Saber an aborted earlier run left carried is deposited and
	// crystallized too: one request per Saber.
	if sent < 1 {
		t.Errorf("sent %d RequestCrystallizeItem, want one per Saber", sent)
	}
	if after.saber >= before.saber || after.crystals <= before.crystals || after.ore >= before.ore || after.shots < before.shots+156 {
		t.Errorf("clan warehouse held %+v before and %+v after, want the Saber crystallized, the ore spent, 156 soulshots and the crystals in", before, after)
	}
	for _, o := range board.Orders(clanID, time.Now()) {
		if o.ID == order {
			t.Errorf("order %+v still open, want it delivered with the deposit", o)
		}
	}
}

// czStock is what a clan warehouse list holds of the scenario's items.
type czStock struct{ saber, crystals, ore, shots int64 }

func czStockOf(l botclient.WarehouseWithdrawList) czStock {
	var n czStock
	for _, it := range l.Items {
		switch it.TemplateID {
		case saber:
			n.saber += int64(it.Count)
		case crystalD:
			n.crystals += int64(it.Count)
		case soulOre:
			n.ore += int64(it.Count)
		case soulshotD:
			n.shots += int64(it.Count)
		}
	}
	return n
}
