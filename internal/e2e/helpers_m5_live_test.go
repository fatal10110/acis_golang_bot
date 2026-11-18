package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Helpers of the M5 scenarios (#117): bots running their town errands
// (trainer, shop, gear) in the Talking Island village, the inventory their
// frames show, and the reset that makes a rerun start from the same gear.

// Templates the scenarios seed, buy or assert (aCis_datapack/data/xml).
const (
	// A human mystic learns Heal (skillHeal) levels 1-3 at level 7, 4-6 at
	// 14 and 7-9 at 20 (classes/humanMystic.xml).
	healAt20 int32 = 7
	// spellbookHeal is Spellbook: Heal, which learning Heal's first level
	// takes when the server needs spellbooks (spellbooks.xml).
	spellbookHeal int32 = 1152
	// The human fighter's starting set: a one-handed sword, a light shirt
	// and light pants, all of no grade.
	squiresSword int32 = 2369
	squiresShirt int32 = 1146
	squiresPants int32 = 1147
	// The pieces of #79: a shield, a necklace and two kinds of ring sold
	// in the village (buylists 11 and 13), and a two-handed sword of no
	// grade.
	smallShield     int32 = 19
	necklaceOfMagic int32 = 118
	magicRing       int32 = 116
	ringOfKnowledge int32 = 875
	redSunsetSword  int32 = 981
	// msgNotEnoughArrows is SystemMessage 112, the server's answer to a bow
	// attack without arrows (network/attack_refusal.go).
	msgNotEnoughArrows int32 = 112
	// trainerSP is the SP the GM sets for a trainer visit: more than
	// every level of the support persona's skills up to level 20 costs.
	trainerSP = 20000
)

// Points of the Talking Island village (spawnlist/17_25.xml): next to the
// human mystic trainers (30032, 30033, 30035, 30036), between the weapon
// and armor merchants (Lector 30001, Jackson 30002), and between those and
// the jewelry and grocery merchants (Silvia 30003, Katerina 30004).
var (
	tiTrainers  = botclient.Point{X: -84850, Y: 244950, Z: -3726}
	tiArmory    = botclient.Point{X: -86400, Y: 243000, Z: -3717}
	tiMarketWay = botclient.Point{X: -85100, Y: 241900, Z: -3717}
)

// townData is the datapack's travel map (trainers, merchants, their spawns
// and wares) and its weapon, armor, jewelry and arrow templates.
func townData(t *testing.T, tg *Target) (*botclient.TravelMap, map[int32]datapack.Item) {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	return botclient.NewTravelMap(dp), dp.Items
}

// runTownBot runs a whole Bot of account, a character of class, through
// runWatched, with the travel map its trainer, shop and gear errands need
// and no farming spots: it runs its errands where it stands. Its persona is
// DefaultPersona without idle drift, adjusted by tune when set.
func runTownBot(t *testing.T, ctx context.Context, tg *Target, account string, class *botclient.StartingClass, travel *botclient.TravelMap, tune func(*botclient.Persona)) *farmer {
	t.Helper()
	return runWatched(t, ctx, tg, account, botSetup{
		class: class,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			if tune != nil {
				tune(&p)
			}
			return p
		},
		brain: func(cfg *botclient.BrainConfig) { cfg.Travel = travel },
	})
}

// gmRecall brings f to p, the GM going there first, and waits until the GM
// sees f arrive.
func gmRecall(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, p botclient.Point) {
	t.Helper()
	gmTeleport(t, ctx, gm, p)
	if err := gm.Command(ctx, "recall "+f.name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Fatalf("%s recalled to %v: %v", f.name, p, err)
	}
}

// gmAdena gives f, the GM's target, n stacks of maxGive adena, one give at
// a time.
func gmAdena(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, n int) {
	t.Helper()
	for range n {
		gmGive(t, ctx, gm, f, adena, maxGive)
	}
}

// gmGiveGear gives f, the GM's target, n items of template tmpl, one
// object each, and waits until f carries n more of them.
func gmGiveGear(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, tmpl int32, n int32) {
	t.Helper()
	seen := f.watch.Seen(0)
	inv := inventory(seen)
	had := carriedOf(inv, tmpl)
	if err := gm.Command(ctx, fmt.Sprintf("give %d %d", tmpl, n)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// The match runs under the watch's lock: it folds each event itself.
	if _, _, err := f.watch.Await(ctx, len(seen), func(ev botclient.Event) bool {
		applyInventory(inv, ev)
		return carriedOf(inv, tmpl) >= had+n
	}); err != nil {
		t.Fatalf("%s never got %d of %d: %v", f.name, n, tmpl, err)
	}
}

// inventory folds the ItemList and InventoryUpdate events of seen into
// the inventory they leave, by object id.
func inventory(seen []Seen) map[int32]botclient.Item {
	inv := make(map[int32]botclient.Item)
	for _, s := range seen {
		applyInventory(inv, s.Event)
	}
	return inv
}

// applyInventory applies ev to inv when it is an ItemList or an
// InventoryUpdate.
func applyInventory(inv map[int32]botclient.Item, ev botclient.Event) {
	switch e := ev.(type) {
	case botclient.ItemList:
		clear(inv)
		for _, it := range e.Items {
			inv[it.ObjectID] = it
		}
	case botclient.InventoryUpdate:
		for _, c := range e.Changes {
			if c.State == botclient.ItemRemoved {
				delete(inv, c.Item.ObjectID)
			} else {
				inv[c.Item.ObjectID] = c.Item
			}
		}
	}
}

// worn is how many items of each template inv has on the paperdoll.
func worn(inv map[int32]botclient.Item) map[int32]int {
	w := make(map[int32]int)
	for _, it := range inv {
		if it.Equipped {
			w[it.TemplateID]++
		}
	}
	return w
}

// carriedOf is how many of template tmpl inv holds, every stack counted.
func carriedOf(inv map[int32]botclient.Item, tmpl int32) int32 {
	var n int32
	for _, it := range inv {
		if it.TemplateID == tmpl {
			n += it.Count
		}
	}
	return n
}

// wornNames lists inv's worn templates by name, for a failure message.
func wornNames(inv map[int32]botclient.Item, items map[int32]datapack.Item) string {
	var names []string
	for id, n := range worn(inv) {
		name := items[id].Name
		if name == "" {
			name = fmt.Sprint(id)
		}
		names = append(names, fmt.Sprintf("%s x%d", name, n))
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// sentOf is the packets of opcode op in sent.
func sentOf(sent []Sent, op byte) []Sent {
	return slices.DeleteFunc(slices.Clone(sent), func(s Sent) bool { return s.Op != op })
}

// soldObjects is the item object ids a RequestSellItem names: list id, row
// count, then per row object id, item id and count
// (clientpackets/shop_trade.go).
func soldObjects(p Sent) []int32 {
	b := p.Payload
	if len(b) < 9 {
		return nil
	}
	n := int(int32(binary.LittleEndian.Uint32(b[5:9])))
	var ids []int32
	for i := range n {
		off := 9 + 12*i
		if off+4 > len(b) {
			break
		}
		ids = append(ids, int32(binary.LittleEndian.Uint32(b[off:off+4])))
	}
	return ids
}

// stripDropGap spaces a strip's drops as the bot's pacer does (dropGap):
// the server silently ignores a drop within DropItemTime (1 s by default)
// of the previous one (config_gates.go handleRequestDropItem).
const stripDropGap = 2 * time.Second

// strip readies account's character of class for a gear scenario, whatever
// earlier runs left it wearing or carrying: it enters as a scripted client,
// drops every weapon, armor and jewelry piece and every arrow it carries,
// worn or not, but one item of each template of keep, has the GM give it
// those of keep it lacks, wears them, and logs out. The drops land where it
// stands, as the farm scenario's junk does; adena and the rest stay.
func strip(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, items map[int32]datapack.Item, account string, class *botclient.StartingClass, keep ...int32) {
	t.Helper()
	c, err := tg.EnterWhenFree(ctx, account, character(t, account, class), nil)
	if err != nil {
		t.Fatal(err)
	}
	logoutAtEnd(t, c)
	tg.Own(c.Self.Name, c.Self.ObjectID)
	inv := make(map[int32]botclient.Item)
	await := func(what string, done func() bool) {
		t.Helper()
		if done() {
			return
		}
		if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			return done()
		}); err != nil {
			t.Fatalf("strip %s: %s: %v", c.Self.Name, what, err)
		}
	}
	list, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.ItemList)
		return ok
	})
	if err != nil {
		t.Fatalf("strip %s: its ItemList: %v", c.Self.Name, err)
	}
	applyInventory(inv, list)
	if userInfo(t, c).HP == 0 {
		revive(t, ctx, c)
	}

	kept := make(map[int32]int32) // template -> object
	var drops []int32
	for _, id := range slices.Sorted(maps.Keys(inv)) {
		it := inv[id]
		if _, gear := items[it.TemplateID]; !gear {
			continue
		}
		if slices.Contains(keep, it.TemplateID) && kept[it.TemplateID] == 0 {
			kept[it.TemplateID] = id
			continue
		}
		drops = append(drops, id)
	}
	// One at a time: the server refuses some (a starting item that cannot
	// be discarded) with a SystemMessage only, and a refused one is worn
	// no more.
	dropped, refused := 0, []string{}
	var last time.Time
	for _, id := range drops {
		it := inv[id]
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Until(last.Add(stripDropGap))):
		}
		last = time.Now()
		if err := c.Send(ctx, botclient.EncodeRequestDropItem(id, it.Count, c.Pos())); err != nil {
			t.Fatal(err)
		}
		var read []string
		ev, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			read = append(read, fmt.Sprintf("%T%+v", ev, ev))
			applyInventory(inv, ev)
			_, carried := inv[id]
			switch ev.(type) {
			case botclient.SystemMessage, botclient.ActionFailed:
				return true
			}
			return !carried
		})
		if err != nil {
			t.Fatalf("strip %s: drop %s %+v: %v; read %.2000s", c.Self.Name, items[it.TemplateID].Name, it, err, strings.Join(read, " | "))
		}
		switch ev.(type) {
		case botclient.SystemMessage, botclient.ActionFailed:
			refused = append(refused, fmt.Sprintf("%s %+v (%T%+v)", items[it.TemplateID].Name, it, ev, ev))
			if inv[id].Equipped {
				if err := c.Send(ctx, botclient.EncodeUseItem(id)); err != nil {
					t.Fatal(err)
				}
				await(items[it.TemplateID].Name+" taken off", func() bool { return !inv[id].Equipped })
			}
			continue
		}
		dropped++
	}

	for _, tmpl := range keep {
		if kept[tmpl] != 0 {
			continue
		}
		if err := gm.Command(ctx, "teleportto "+c.Self.Name); err != nil {
			t.Fatal(err)
		}
		if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(c.Self.ObjectID)); err != nil {
			t.Fatalf("GM next to %s: %v", c.Self.Name, err)
		}
		if err := gm.Select(ctx, c.Self.ObjectID); err != nil {
			t.Fatal(err)
		}
		if err := gm.Command(ctx, fmt.Sprintf("give %d", tmpl)); err != nil {
			t.Fatal(err)
		}
		await(fmt.Sprintf("the GM's %d", tmpl), func() bool {
			for id, it := range inv {
				if it.TemplateID == tmpl {
					kept[tmpl] = id
					return true
				}
			}
			return false
		})
	}
	for _, tmpl := range keep {
		id := kept[tmpl]
		if inv[id].Equipped {
			continue
		}
		if err := c.Send(ctx, botclient.EncodeUseItem(id)); err != nil {
			t.Fatal(err)
		}
		await(fmt.Sprintf("%d worn", tmpl), func() bool { return inv[id].Equipped })
	}
	t.Logf("%s stripped: dropped %d pieces, kept %v, wears %s", c.Self.Name, dropped, refused, wornNames(inv, items))
	lctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := c.Logout(lctx); err != nil {
		t.Fatalf("strip %s: logout: %v", c.Self.Name, err)
	}
	c.Close()
}
