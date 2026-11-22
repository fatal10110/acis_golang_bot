package e2e

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Clan warehouse scenario: keeper Walderal (30005) of Talking Island
// village stands at -81512 243424 -3720 (spawnlist/17_25.xml); the GM
// lands 100 short of it and recalls the bot there.
var nearKeeper = botclient.Point{X: -81412, Y: 243424, Z: -3720}

const (
	// whSkin is Animal Skin, a tradable stackable material, and whSkins
	// how many the bot deposits and takes back.
	whSkin  int32 = 1867
	whSkins int32 = 10
	// whAdena, in adena (template 57), pays the deposit fee (30 a row)
	// with room to spare.
	whAdena = 1000
)

// whReport is what the bot's warehouse errand saw at the end of each part:
// its status, and how many skins the bot carried then and before the
// deposit (a character of an earlier run may carry some).
type whReport struct {
	st              botclient.WarehouseStatus
	carried, before int32
}

// carriedOfWorld is how many of template tmpl w's bot carries.
func carriedOfWorld(w *botclient.World, tmpl int32) int32 {
	var n int32
	for _, it := range w.Inventory.Items {
		if it.TemplateID == tmpl {
			n += it.Count
		}
	}
	return n
}

// whScript is the bot's Persona.Warehouse.Want for the scenario. Until the
// test arms it, it only publishes how many skins the bot carries; armed, it
// deposits the skins, then takes them all back, and reports the end of each
// part on out. want runs on the bot's brain goroutine, which owns phase and
// before.
type whScript struct {
	// carried is the skins the bot carried at the latest ask, -1 before
	// the first; armed is set by the test once the clan may use its
	// warehouse and the bot carries the skins.
	carried atomic.Int32
	armed   atomic.Bool
	out     chan whReport
	phase   int
	before  int32
}

func newWHScript() *whScript {
	s := &whScript{out: make(chan whReport, 2)}
	s.carried.Store(-1)
	return s
}

func (s *whScript) report(w *botclient.World, st botclient.WarehouseStatus) {
	select {
	case s.out <- whReport{st: st, carried: carriedOfWorld(w, whSkin), before: s.before}:
	default:
	}
}

func (s *whScript) want(w *botclient.World, st botclient.WarehouseStatus, _ time.Time) botclient.WarehouseRequest {
	s.carried.Store(carriedOfWorld(w, whSkin))
	if !s.armed.Load() {
		return botclient.WarehouseRequest{}
	}
	switch s.phase {
	case 0:
		s.before, s.phase = carriedOfWorld(w, whSkin), 1
		return botclient.WarehouseRequest{Deposit: []botclient.WarehouseRow{{TemplateID: whSkin, Count: whSkins}}}
	case 1:
		if st.Deposit.Outcome != botclient.WarehouseNone {
			s.report(w, st)
			s.phase = 2
			return botclient.WarehouseRequest{Withdraw: []botclient.WarehouseRow{{TemplateID: whSkin}}}
		}
	case 2:
		if st.Withdraw.Outcome != botclient.WarehouseNone {
			s.report(w, st)
			s.phase = 3
		}
	}
	return botclient.WarehouseRequest{}
}

// clanLevelSet matches the GM's word that clan is at level: the server
// sends it once the clan has the level (network/admin_pledge.go).
func clanLevelSet(clan string, level int) func(botclient.Event) bool {
	want := fmt.Sprintf("You have set clan %s to level %d", clan, level)
	return func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.Text[0] == want
	}
}

// TestLiveClanWarehouse has the bot found a level-1 clan, deposit skins at
// a warehouse keeper, and take them back an open later: the deposit shows
// in the WarehouseWithdrawList the second open reads, and the withdrawal
// returns the skins (#186). The bot leads its clan, which may withdraw
// whatever MembersCanWithdrawFromClanWH says; a member with privilege 8 is
// the same request past the server's gate. Opens are MinOpenGap apart, so
// the scenario takes about three minutes. The bot records the first frame of
// each opcode it reads into the warehouse directory of BOT_E2E_RECORD when
// set, else a temporary one: the 0x41 and 0x42 frames there are the
// recorded fixtures of the warehouse list decoders.
func TestLiveClanWarehouse(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 7*time.Minute)
	gm := enterGM(t, ctx, tg)
	dir := filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "warehouse")
	rec, err := botclient.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	script := newWHScript()
	travel := travelMap(t, tg)
	botAcc, _ := tg.Account("wh", 1)
	b := runBotWith(t, ctx, tg, botAcc, botSetup{tap: rec, brain: withTravel(travel), persona: func() botclient.Persona {
		p := partyPersona()
		p.Warehouse.Want = script.want
		return p
	}})
	name := botOnline(t, ctx, tg, b)
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
	clan := "wh" + strconv.FormatInt(time.Now().UnixNano()%1e12, 36)
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
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		// The bot is still the GM's target: the server confirms no
		// second click on it.
		if err := gm.Command(ctx, "pledge dismiss"); err != nil {
			t.Errorf("dismiss clan %s: %v", clan, err)
		}
	})
	// The script asks once the bot is in a clan; a character of an
	// earlier run may carry skins already.
	waitFor(t, ctx, "the warehouse script asked", func() bool { return script.carried.Load() >= 0 })
	left := script.carried.Load()
	for _, cmd := range []string{fmt.Sprintf("give 57 %d", whAdena), fmt.Sprintf("give %d %d", whSkin, whSkins)} {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, ctx, "the skins in the bot's bag", func() bool { return script.carried.Load() >= left+whSkins })
	// The clan warehouse needs clan level 1 (clan/warehouse.go).
	if err := gm.Command(ctx, "pledge level 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, clanLevelSet(clan, 1)); err != nil {
		t.Fatalf("clan %s at level 1: %v", clan, err)
	}
	script.armed.Store(true)

	next := func(what string) whReport {
		t.Helper()
		select {
		case r := <-script.out:
			return r
		case <-ctx.Done():
			t.Fatalf("waiting for the %s: %v", what, ctx.Err())
			return whReport{}
		}
	}
	dep := next("deposit")
	if dep.st.Deposit.Outcome != botclient.WarehouseDone || dep.carried != dep.before-whSkins {
		t.Fatalf("deposit = %+v carrying %d skins of %d, want done and %d fewer", dep.st.Deposit, dep.carried, dep.before, whSkins)
	}
	wd := next("withdrawal")
	if wd.st.Withdraw.Outcome != botclient.WarehouseDone || wd.carried != wd.before {
		t.Fatalf("withdrawal = %+v carrying %d skins, want done and the %d back", wd.st.Withdraw, wd.carried, wd.before)
	}
	// The second open's list held the deposit.
	raw, err := os.ReadFile(filepath.Join(dir, "42.bin"))
	if err != nil {
		t.Fatal(err)
	}
	ev, _, err := botclient.DecodeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	list, ok := ev.(botclient.WarehouseWithdrawList)
	if !ok || list.Type != 2 || len(list.Items) != 1 || list.Items[0].TemplateID != whSkin || list.Items[0].Count != whSkins {
		t.Fatalf("withdrawal list = %+v, want the clan's holding the %d skins", ev, whSkins)
	}
	if err := rec.Err(); err != nil {
		t.Error(err)
	}
}
