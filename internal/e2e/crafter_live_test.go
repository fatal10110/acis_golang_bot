package e2e

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Crafter scenario: Recipe: Steel (recipes.xml recipe 30, scroll 2137), a
// dwarven recipe of Create Item level 1, makes one Steel (1880) of five
// Varnish (1865) and five Iron Ore (1869) for 10 MP, always. Steel is worth
// its materials at reference prices, so the planner crafts it for an order
// only: the scenario posts a GM order for one.
const (
	steelRecipe  int32  = 30
	steelScroll  int32  = 2137
	steelItem    int32  = 1880
	varnish      int32  = 1865
	ironOre      int32  = 1869
	steelEach    int32  = 5
	createItem   int32  = 172
	clanWHType   uint16 = 2 // WarehouseWithdrawList.Type of a clan warehouse
	opWHList            = 0x42
	opRecipeMade        = 0xd7
)

// crafterTap records the bot's frames (a Recorder) and hands the test, in
// the order the bot read them, the warehouse lists and craft results. It
// runs on the bot's reader goroutine and keeps no payload.
type crafterTap struct {
	*botclient.Recorder
	out chan botclient.Event
}

func (c crafterTap) Record(payload []byte) {
	c.Recorder.Record(payload)
	if len(payload) == 0 || payload[0] != opWHList && payload[0] != opRecipeMade {
		return
	}
	ev, _, err := botclient.DecodeEvent(payload)
	if err != nil {
		return
	}
	select {
	case c.out <- ev:
	default:
	}
}

// crafterPersona is the crafter's cycle on partyPersona, its checks a
// minute apart and without jitter so the scenario stays short; opens stay
// MinOpenGap apart. The GM sets Create Item and founds the clan, so it
// needs no trainer and no clan invitation.
func crafterPersona() botclient.Persona {
	p := partyPersona()
	p.Crafter = botclient.CrafterPersona().Crafter
	p.Crafter.CheckEvery, p.Crafter.CheckJitter, p.Crafter.ReturnEvery = time.Minute, 0, time.Minute
	return p
}

// TestLiveCrafter plays the clan crafter's cycle (#190, #194): a Dwarven
// Fighter with Create Item 1 leads a fresh level-1 clan, carrying the steel
// recipe's scroll and materials. With no order on the board its first cycle
// that ends with nothing to craft (learning the recipe on the account's
// first run) deposits the materials, which no plan needs. Only then the GM
// posts an order for one Steel, so the next cycle withdraws the materials
// from the clan warehouse, crafts the steel and deposits it, which delivers
// the order, on a rerun of the account as on its first run. The scenario
// asserts the clan list before the craft held the materials and the one
// after holds the steel and fewer materials. The leader withdraws whatever
// MembersCanWithdrawFromClanWH says. Opens are MinOpenGap apart, so it
// takes about six minutes. The bot records the first frame of each opcode
// it reads into the crafter directory of BOT_E2E_RECORD when set, else a
// temporary one: its d7.bin is the recorded RecipeItemMakeInfo fixture.
func TestLiveCrafter(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 14*time.Minute)
	gm := enterGM(t, ctx, tg)
	dir := filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "crafter")
	rec, err := botclient.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	tap := crafterTap{Recorder: rec, out: make(chan botclient.Event, 64)}
	travel := travelMap(t, tg)
	board := botclient.NewCraftBoard()
	class := botclient.DwarvenFighter
	botAcc, _ := tg.Account("crf", 1)
	b := runBotWith(t, ctx, tg, botAcc, botSetup{tap: tap, class: &class, persona: crafterPersona, brain: func(cfg *botclient.BrainConfig) {
		withTravel(travel)(cfg)
		cfg.CraftBoard = board
	}})
	name := botOnline(t, ctx, tg, b)
	if id := b.Status().ClassID; id != class.ID {
		t.Fatalf("%s is class %d, want the Dwarven Fighter (%d)", name, id, class.ID)
	}
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", nearKeeper.X, nearKeeper.Y, nearKeeper.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM next to the keeper: %v", err)
	}
	botID := recallBot(t, ctx, tg, gm, name)
	if err := gm.Select(ctx, botID); err != nil {
		t.Fatal(err)
	}
	if b.Status().ClanID != 0 {
		t.Logf("%s leads a clan from an earlier run: dismissed", name)
		if err := gm.Command(ctx, "pledge dismiss"); err != nil {
			t.Fatal(err)
		}
		waitFor(t, ctx, "the bot out of its old clan", func() bool { return b.Status().ClanID == 0 })
	}
	if b.Status().Level < clanCreateLevel {
		if err := gm.Command(ctx, "set level "+strconv.Itoa(clanCreateLevel)); err != nil {
			t.Fatal(err)
		}
		waitFor(t, ctx, "the bot at the founding level", func() bool { return b.Status().Level >= clanCreateLevel })
	}
	// The crafter's cycle waits for a clan: everything it needs is in
	// before the clan is founded.
	cmds := []string{
		fmt.Sprintf("skill set %d 1", createItem), fmt.Sprintf("give 57 %d", whAdena), fmt.Sprintf("give %d 1", steelScroll),
		fmt.Sprintf("give %d %d", varnish, steelEach), fmt.Sprintf("give %d %d", ironOre, steelEach),
	}
	for _, cmd := range cmds {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, ctx, "the crafter with Create Item 1", func() bool {
		c := b.Status().Crafter
		return c != nil && c.CreateItem >= 1
	})
	clan := "cr" + strconv.FormatInt(time.Now().UnixNano()%1e12, 36)
	if err := gm.Command(ctx, "pledge create "+clan); err != nil {
		t.Fatal(err)
	}
	said, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && (strings.HasPrefix(m.Text[0], "Clan "+clan+" ") || strings.Contains(m.Text[0], "creating the clan"))
	})
	if err != nil {
		t.Fatalf("pledge create %s: %v", clan, err)
	}
	if m := said.(botclient.SystemMessage); !strings.HasPrefix(m.Text[0], "Clan "+clan+" have been created") {
		t.Fatalf("pledge create %s: %q", clan, m.Text[0])
	}
	waitFor(t, ctx, "the bot leading its clan", func() bool { return b.Status().ClanID != 0 })
	clanID := b.Status().ClanID
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := gm.Command(ctx, "pledge dismiss"); err != nil {
			t.Errorf("dismiss clan %s: %v", clan, err)
		}
	})
	// The clan warehouse needs clan level 1 (clan/warehouse.go); a look
	// before it fails and the next check comes CheckEvery later.
	if err := gm.Command(ctx, "pledge level 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, clanLevelSet(clan, 1)); err != nil {
		t.Fatalf("clan %s at level 1: %v", clan, err)
	}
	// The first cycle with nothing to craft deposits the materials: the
	// order comes after it, so the craft needs them withdrawn. One that
	// aborted (a look before the clan level) leaves them carried.
	waitFor(t, ctx, "a crafter cycle with nothing to craft", func() bool {
		c := b.Status().Crafter
		return c != nil && c.Last.Outcome == botclient.CraftCycleNothing
	})
	order, ok := board.PostCommand(clanID, botclient.OrderByGM, "e2e GM", steelItem, 1)
	if !ok {
		t.Fatal("GM order refused")
	}

	// What the clan's list held before the craft: the materials the first
	// cycle deposited, and steel a character of an earlier run carried.
	var before, after whStock
	made := false
	for done := false; !done; {
		select {
		case ev := <-tap.out:
			switch e := ev.(type) {
			case botclient.RecipeItemMakeInfo:
				t.Logf("craft result %+v", e)
				if e.RecipeID == steelRecipe && e.Status == botclient.RecipeMadeSuccess && !made {
					made = true
					if before.varnish < int64(steelEach) || before.ore < int64(steelEach) {
						t.Fatalf("steel crafted while the clan warehouse held %+v, want the materials withdrawn from it", before)
					}
				}
			case botclient.WarehouseWithdrawList:
				if e.Type != clanWHType {
					continue
				}
				n := stockOf(e)
				t.Logf("clan warehouse list: %d items, %+v", len(e.Items), n)
				if !made {
					before = n
				} else if n.steel > before.steel {
					after, done = n, true
				}
			}
		case <-ctx.Done():
			t.Fatalf("waiting for the crafted steel in the clan warehouse (crafted %v): %v; crafter %+v", made, ctx.Err(), b.Status().Crafter)
		}
	}
	if after.varnish >= before.varnish || after.ore >= before.ore {
		t.Errorf("clan warehouse held %+v before the craft and %+v after, want the materials withdrawn", before, after)
	}
	for _, o := range board.Orders(clanID, time.Now()) {
		if o.ID == order {
			t.Errorf("order %+v still open, want it delivered with the deposit", o)
		}
	}
	if c := b.Status().Crafter; c == nil || c.Recipes < 1 || !c.BookKnown {
		t.Errorf("crafter status %+v, want the book read and the steel recipe in it", c)
	}
	if err := rec.Err(); err != nil {
		t.Error(err)
	}
}

// whStock is what a clan warehouse list holds of the scenario's items.
type whStock struct{ steel, varnish, ore int64 }

func stockOf(l botclient.WarehouseWithdrawList) whStock {
	var n whStock
	for _, it := range l.Items {
		switch it.TemplateID {
		case steelItem:
			n.steel += int64(it.Count)
		case varnish:
			n.varnish += int64(it.Count)
		case ironOre:
			n.ore += int64(it.Count)
		}
	}
	return n
}
