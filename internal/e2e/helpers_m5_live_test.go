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
	// 14 and 7-9 at 20 (classes/humanMystic.xml). A level change pulls a
	// skill down to the highest level learnable up to 9 levels above the
	// new level (skill/leveling.go correctInvalidSkills): at level 1 the
	// mystic keeps Heal 3 at most, which level 10 adds nothing to, and
	// level 15 offers 4. Higher levels belong to the Cleric (class 15).
	healAt1  int32 = 3
	healAt15 int32 = 4
	// spellbookHeal is Spellbook: Heal, which learning Heal's first level
	// takes when the server needs spellbooks (spellbooks.xml).
	spellbookHeal int32 = 1152
	// The human fighter's starting set: a one-handed sword, a light shirt
	// and light pants, all of no grade.
	squiresSword int32 = 2369
	squiresShirt int32 = 1146
	squiresPants int32 = 1147
	// shortSword, shirt and pants are no-grade pieces of the starting
	// set's slots a merchant buys back: the starting set is not sellable.
	shortSword int32 = 1
	shirt      int32 = 21
	pants      int32 = 28
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
	// every level of the support persona's skills up to level 15 costs.
	trainerSP = 20000
)

// Points of the Talking Island village (spawnlist/17_25.xml): next to the
// human mystic trainers (30032, 30033, 30035, 30036), between the weapon
// and armor merchants (Lector 30001, Jackson 30002), next to the weapon
// merchant (nearer him than the armor one), and between the
// jewelry and grocery merchants (Silvia 30003, Katerina 30004).
var (
	tiTrainers = botclient.Point{X: -84850, Y: 244950, Z: -3726}
	tiArmory   = botclient.Point{X: -86400, Y: 243000, Z: -3717}
	tiWeapons  = botclient.Point{X: -86385, Y: 243217, Z: -3717}
	tiGrocery  = botclient.Point{X: -84000, Y: 240600, Z: -3717}
)

// townData is the datapack's travel map (trainers, merchants, their spawns
// and wares) and its weapon, armor, jewelry and arrow templates. travelMap
// (helpers_m4) is the same map alone.
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
	f := runWatched(t, ctx, tg, account, botSetup{
		class: class,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			if tune != nil {
				tune(&p)
			}
			return p
		},
		brain: withTravel(travel),
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.visits())
		}
	})
	return f
}

// opBypass is RequestBypassToServer's opcode: an NPC page's link.
const opBypass = 0x21

// visits is f's NPC dealings in order, for a failed scenario: the clicks
// and links it sent (an NPC as object/template), the pages, buy lists and
// SystemMessages it read.
func (f *farmer) visits() string {
	npcs := make(map[int32]int32)
	type line struct {
		at   time.Time
		text string
	}
	var lines []line
	for _, e := range f.watch.Seen(0) {
		switch ev := e.Event.(type) {
		case botclient.NPCInfo:
			npcs[ev.ObjectID] = ev.NPCID
		case botclient.NpcHtmlMessage:
			html := ev.HTML
			if len(html) > 160 {
				html = html[:160]
			}
			lines = append(lines, line{e.At, fmt.Sprintf("page of %d/%d: %q", ev.ObjectID, npcs[ev.ObjectID], html)})
		case botclient.BuyList:
			lines = append(lines, line{e.At, fmt.Sprintf("buy list %d (%d items, %d adena)", ev.ListID, len(ev.Items), ev.Money)})
		case botclient.SystemMessage:
			lines = append(lines, line{e.At, fmt.Sprintf("system message %d", ev.ID)})
		}
	}
	for _, p := range f.watch.Sent() {
		switch p.Op {
		case botclient.OpAction:
			lines = append(lines, line{p.At, fmt.Sprintf("click %d/%d", p.Target, npcs[p.Target])})
		case opBypass:
			var b strings.Builder
			for i := 1; i+1 < len(p.Payload); i += 2 {
				if r := binary.LittleEndian.Uint16(p.Payload[i:]); r != 0 {
					b.WriteRune(rune(r))
				}
			}
			lines = append(lines, line{p.At, fmt.Sprintf("link %q", b.String())})
		case botclient.OpRequestBuyItem, botclient.OpRequestSellItem:
			lines = append(lines, line{p.At, fmt.Sprintf("op %#02x list %d", p.Op, p.Target)})
		}
	}
	slices.SortStableFunc(lines, func(a, b line) int { return a.at.Compare(b.at) })
	var b strings.Builder
	fmt.Fprintf(&b, "%s's visits:", f.name)
	for _, l := range lines {
		fmt.Fprintf(&b, "\n  %s %s", l.at.Format("15:04:05.000"), l.text)
	}
	return b.String()
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

// stripEnd is where strip leaves a character: recalled to at (none for
// the zero Point) with adena at least (topped up by the GM), so that its
// bot logs in next to the merchants with the adena to shop at once: one
// logging in with nothing to buy at a merchant in reach holds its shop
// errand off for five minutes (shopRetry).
type stripEnd struct {
	at    botclient.Point
	adena int32
}

// destroyRefusals are the SystemMessages refusing a RequestDestroyItem
// (inventory.go destroyLiveItem): cannot discard, number incorrect, a hero
// weapon, a summoned pet's collar, a trade or store open.
var destroyRefusals = []int32{98, 163, 1845, 557, 1065}

// strip readies account's character of class for a gear scenario, whatever
// earlier runs left it wearing or carrying: it enters as a scripted client,
// destroys every weapon, armor and jewelry piece and every arrow it
// carries, worn or not, but one item of each template of keep, has the GM
// give it those of keep it lacks, wears them, and logs out where end says.
// Adena and the rest stay. (A drop would leave litter in the village, and
// the server refuses to drop the starting weapons.)
func strip(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, items map[int32]datapack.Item, account string, class *botclient.StartingClass, end stripEnd, keep ...int32) {
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
	var gone []int32
	for _, id := range slices.Sorted(maps.Keys(inv)) {
		it := inv[id]
		if _, gear := items[it.TemplateID]; !gear {
			continue
		}
		if slices.Contains(keep, it.TemplateID) && kept[it.TemplateID] == 0 {
			kept[it.TemplateID] = id
			continue
		}
		gone = append(gone, id)
	}
	// One at a time: the server refuses some with a SystemMessage only,
	// and a refused one is worn no more.
	destroyed, refused := 0, []string{}
	for _, id := range gone {
		it := inv[id]
		if err := c.Send(ctx, botclient.EncodeRequestDestroyItem(id, it.Count)); err != nil {
			t.Fatal(err)
		}
		// A destroy also tells the item disarmed (417) or gone (302)
		// ahead of the InventoryUpdate: only a refusal ends the wait early.
		// The server never answers a destroy with ActionFailed; one read
		// here answers something earlier (seen live after EnterWorld).
		ev, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			if m, ok := ev.(botclient.SystemMessage); ok && slices.Contains(destroyRefusals, m.ID) {
				return true
			}
			_, carried := inv[id]
			return !carried
		})
		if err != nil {
			t.Fatalf("strip %s: destroy %s %+v: %v", c.Self.Name, items[it.TemplateID].Name, it, err)
		}
		if _, carried := inv[id]; !carried {
			destroyed++
			continue
		}
		refused = append(refused, fmt.Sprintf("%s (%T%+v)", items[it.TemplateID].Name, ev, ev))
		if inv[id].Equipped {
			if err := c.Send(ctx, botclient.EncodeUseItem(id)); err != nil {
				t.Fatal(err)
			}
			await(items[it.TemplateID].Name+" taken off", func() bool { return !inv[id].Equipped })
		}
	}

	gmNext := func() {
		t.Helper()
		if err := gm.Command(ctx, "teleportto "+c.Self.Name); err != nil {
			t.Fatal(err)
		}
		if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(c.Self.ObjectID)); err != nil {
			t.Fatalf("GM next to %s: %v", c.Self.Name, err)
		}
		if err := gm.Select(ctx, c.Self.ObjectID); err != nil {
			t.Fatal(err)
		}
	}
	for _, tmpl := range keep {
		if kept[tmpl] != 0 {
			continue
		}
		gmNext()
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
	if have := carriedOf(inv, adena); have < end.adena {
		gmNext()
		for have < end.adena {
			n := min(end.adena-have, maxGive)
			if err := gm.Command(ctx, fmt.Sprintf("give %d %d", adena, n)); err != nil {
				t.Fatal(err)
			}
			want := have + n
			await(fmt.Sprintf("%d adena", want), func() bool { return carriedOf(inv, adena) >= want })
			have = carriedOf(inv, adena)
		}
	}
	if end.at != (botclient.Point{}) {
		gmTeleport(t, ctx, gm, end.at)
		if err := gm.Command(ctx, "recall "+c.Self.Name); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Await(ctx, EntryTimeout, teleportOf(c.Self.ObjectID)); err != nil {
			t.Fatalf("strip %s: recalled to %v: %v", c.Self.Name, end.at, err)
		}
	}
	t.Logf("%s stripped: destroyed %d pieces, kept %v, wears %s, carries %d adena", c.Self.Name, destroyed, refused, wornNames(inv, items), carriedOf(inv, adena))
	lctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := c.Logout(lctx); err != nil {
		t.Fatalf("strip %s: logout: %v", c.Self.Name, err)
	}
	c.Close()
}
