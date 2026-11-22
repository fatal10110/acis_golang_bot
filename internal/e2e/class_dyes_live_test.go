package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Adena the GM gives for the dyes: a Warrior's two symbols' dyes (10 each
// at Colleen's 6120) and fees (37000 each), and a Gladiator's three fees
// (87000 each) and the removal of the two first ones (a fifth of their
// fee), each with the gear floor (GearPolicy.KeepAdena 10000) and the
// trip's supplies the shop errand buys first.
const (
	firstDyesAdena  = 300000
	secondDyesAdena = 320000
)

// TestLiveClassChangeDyes takes a new human fighter through its two
// professions and their dye symbols (#178, #179, #180). Levelled to 20
// next to Talking Island's class master, it becomes a Warrior. In Gludin,
// between the dye seller and the Symbol Maker, with adena, it buys ten of
// each of its two symbols' dyes, keeping the fees, and draws both: each
// draw a SymbolAdded, its HennaInfo showing both symbols active. Levelled
// to 40 at the master, it becomes a Gladiator; back in Gludin, given its
// three symbols' dyes and fees, it removes the two first-profession
// symbols the new class leaves inactive and draws its three. A class
// change cannot be undone, so every run starts from a new character;
// without the class master at its spawn (fatal10110/acis_golang#5419) the
// scenario is skipped.
func TestLiveClassChangeDyes(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 30*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	masterAt(t, ctx, gm)

	account := freshAccount(tg, "cd")
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			errandsOff(&p)
			p.ClassChange = botclient.DefaultPersona().ClassChange
			p.Dyes = botclient.DefaultPersona().Dyes
			return p
		},
		brain: withTravel(travel),
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.digest())
			t.Log(f.visits())
		}
	})
	recallBot(t, ctx, tg, gm, f.name)
	levelTo(t, ctx, gm, f, 20)
	becomes(t, ctx, f, classWarrior)

	gmTeleport(t, ctx, gm, gludinSymbols)
	recallBot(t, ctx, tg, gm, f.name)
	from := f.seen()
	gmOn(t, ctx, gm, f)
	gmGiveAdena(t, ctx, gm, f, firstDyesAdena)
	draws(t, ctx, f, from, []int32{symbolStr1, symbolDex1}, 0)
	for _, dye := range []int32{dyeStr1, dyeDex1} {
		if run := f.counts(dye, from); !rose(run) || run[len(run)-1] != 0 {
			t.Errorf("%s's dyes %d: %v, want bought and spent", f.name, dye, run)
		}
		if soldTemplate(f.watch.Seen(0), f.watch.Sent(), dye) {
			t.Errorf("%s sold its dyes %d before the draw", f.name, dye)
		}
	}

	levelTo(t, ctx, gm, f, 40)
	masterAt(t, ctx, gm)
	recallBot(t, ctx, tg, gm, f.name)
	becomes(t, ctx, f, classGladiator)

	gmTeleport(t, ctx, gm, gludinSymbols)
	recallBot(t, ctx, tg, gm, f.name)
	from = f.seen()
	gmOn(t, ctx, gm, f)
	gmGive(t, ctx, gm, f, dyeStr3, 2*dyesPerDraw)
	gmGive(t, ctx, gm, f, dyeDex3, dyesPerDraw)
	gmGiveAdena(t, ctx, gm, f, secondDyesAdena)
	draws(t, ctx, f, from, []int32{symbolStr3, symbolDex3, symbolStr3}, 2)
	// A removed symbol hands back dyesReturned of its dye.
	for _, dye := range []int32{dyeStr1, dyeDex1} {
		if run := f.counts(dye, from); slices.Max(run)-run[0] != dyesReturned {
			t.Errorf("%s's dyes %d after the removals: %v, want %d handed back", f.name, dye, run, dyesReturned)
		}
	}
}

// masterAt teleports the GM to the class master's Talking Island spawn and
// skips the test unless the master shows there.
func masterAt(t *testing.T, ctx context.Context, gm *GMClient) {
	t.Helper()
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", tiMaster.X, tiMaster.Y, tiMaster.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		n, ok := ev.(botclient.NPCInfo)
		return ok && n.NPCID == classMaster
	}); err != nil {
		t.Skipf("the class master (%d) is not at %v on this server, fatal10110/acis_golang#5419: %v", classMaster, tiMaster, err)
	}
}

// becomes waits until f's class is class.
func becomes(t *testing.T, ctx context.Context, f *farmer, class int32) {
	t.Helper()
	until(t, ctx, fmt.Sprintf("%s becomes class %d", f.name, class), func() string {
		if s := f.bot.Status(); s.ClassID != class {
			return fmt.Sprintf("class %d (rule %q)", s.ClassID, s.Rule)
		}
		return ""
	})
	t.Logf("%s is class %d at level %d", f.name, class, f.bot.Status().Level)
}

// draws waits until, since index from, f removed removals symbols and drew
// symbols in order at a Symbol Maker: a draw window, removed ones each a
// RequestHennaUnequip and a SymbolDeleted, drawn ones each a
// RequestHennaEquip and a SymbolAdded, and its last HennaInfo shows
// symbols worn, every one active.
func draws(t *testing.T, ctx context.Context, f *farmer, from int, symbols []int32, removals int) {
	t.Helper()
	until(t, ctx, fmt.Sprintf("%s draws %v", f.name, symbols), func() string {
		var window bool
		var added, deleted int
		var info *botclient.HennaInfo
		for _, s := range f.watch.Seen(from) {
			switch e := s.Event.(type) {
			case botclient.HennaEquipList:
				window = true
			case botclient.HennaInfo:
				info = &e
			case botclient.SystemMessage:
				switch e.ID {
				case msgSymbolAdded:
					added++
				case msgSymbolDeleted:
					deleted++
				}
			}
		}
		var wornIDs []int32
		active := true
		if info != nil {
			for _, w := range info.Worn {
				wornIDs = append(wornIDs, w.SymbolID)
				active = active && w.Active != 0
			}
		}
		switch {
		case !window:
			return fmt.Sprintf("no draw window (rule %q)", f.bot.Status().Rule)
		case deleted < removals:
			return fmt.Sprintf("%d of %d removed", deleted, removals)
		case added < len(symbols):
			return fmt.Sprintf("%d of %d drawn, wears %v", added, len(symbols), wornIDs)
		case !slices.Equal(wornIDs, symbols) || !active:
			return fmt.Sprintf("wears %v (all active %v)", wornIDs, active)
		}
		return ""
	})
	sent := f.watch.Sent()
	t.Logf("%s drew %v: %d RequestHennaEquip, %d RequestHennaUnequip", f.name, symbols,
		len(sentOf(sent, botclient.OpRequestHennaEquip)), len(sentOf(sent, botclient.OpRequestHennaUnequip)))
}
