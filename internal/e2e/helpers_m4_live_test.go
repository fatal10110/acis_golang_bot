package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Helpers of the M4 scenarios (#116, #111): town bots, merchants and
// stores, a second client that trades and buys, and chat.

// Templates and limits of the M4 scenarios (aCis_datapack/data/xml).
const (
	// lesserHealingPotion is Lesser Healing Potion, which every Talking
	// Island merchant sells (buyLists.xml).
	lesserHealingPotion int32 = 1060
	// steelPowder is Steel Powder, a stackable material of weight 2 and
	// reference price 80 that no NPC shop sells (buyLists.xml): loot a
	// merchant's store sells. A stem is no such loot: shops sell it.
	steelPowder int32 = 1664
	// storeBudget is the merchant preset's store budget per session
	// (cmd/bots -store-budget).
	storeBudget = 50000
	// storesPerTown and storeSpacing are cmd/bots' defaults
	// (-stores-per-town, -store-spacing).
	storesPerTown, storeSpacing = 5, 150
	// silence is how long a scenario listens for a reply that must not
	// come: past the chat policy's longest delay (12 s) and a margin.
	silence = 20 * time.Second
)

// travelMap is the datapack's travel map (BOT_E2E_DATAPACK): its
// merchants, guides, gatekeepers and the shops' prices.
func travelMap(t *testing.T, tg *Target) *botclient.TravelMap {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	return botclient.NewTravelMap(dp, botclient.DefaultSupplyConfig())
}

// withTravel gives the bot the travel map, as every cmd/bots preset does.
func withTravel(m *botclient.TravelMap) func(*botclient.BrainConfig) {
	return func(cfg *botclient.BrainConfig) { cfg.Travel = m }
}

// toTown brings the GM to Talking Island village and recalls f there.
func toTown(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, f *farmer) {
	t.Helper()
	gmTeleport(t, ctx, gm, townTI)
	recallBot(t, ctx, tg, gm, f.name)
}

// nearGrocer is a step in front of Katerina (npc 30004, -84204;240403;-3717
// heading 14000; spawnlist/17_25.xml), the Talking Island merchant who sells
// the potions the scenarios buy. Bots recalled to the village centre walk
// there on their own, but the routed walk stops short of her, at
// (-84529,241723), and a click's straight approach ends at the wall (#116).
var nearGrocer = botclient.Point{X: -84190, Y: 240460, Z: -3717}

// toGrocer brings the GM next to Katerina and recalls f there.
func toGrocer(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, f *farmer) {
	t.Helper()
	gmTeleport(t, ctx, gm, nearGrocer)
	recallBot(t, ctx, tg, gm, f.name)
}

// seen is the index of f's next event: what a scenario reads from.
func (f *farmer) seen() int { return len(f.watch.Seen(0)) }

// stockAt is the count of tmpl f's inventory showed in its events before
// index i.
func (f *farmer) stockAt(tmpl int32, i int) int32 {
	var n int32
	for _, s := range f.watch.Seen(0)[:i] {
		if c, _, ok := stock(s.Event, tmpl); ok {
			n = c
		}
	}
	return n
}

// counts is the run of counts of tmpl f's events showed from index from on,
// starting with the count before it.
func (f *farmer) counts(tmpl int32, from int) []int32 {
	seen := f.watch.Seen(0)
	run := []int32{f.stockAt(tmpl, from)}
	for _, s := range seen[from:] {
		if c, _, ok := stock(s.Event, tmpl); ok && c != run[len(run)-1] {
			run = append(run, c)
		}
	}
	return run
}

// rose and fell report whether a run of counts went up, or down, at some
// step.
func rose(run []int32) bool {
	for i := 1; i < len(run); i++ {
		if run[i] > run[i-1] {
			return true
		}
	}
	return false
}

func fell(run []int32) bool {
	for i := 1; i < len(run); i++ {
		if run[i] < run[i-1] {
			return true
		}
	}
	return false
}

// giveClient has the GM, next to c, give c count of template tmpl and
// waits until c's inventory shows it, returning c's count then.
func giveClient(t *testing.T, ctx context.Context, gm *GMClient, c *Client, tmpl, count int32) int32 {
	t.Helper()
	if err := gm.Give(ctx, c.Self.ObjectID, tmpl, count); err != nil {
		t.Fatal(err)
	}
	ev, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		n, _, ok := stock(ev, tmpl)
		return ok && n >= count
	})
	if err != nil {
		t.Fatalf("%s got %d of %d: %v", c.Self.Name, count, tmpl, err)
	}
	n, _, _ := stock(ev, tmpl)
	return n
}

// storeOf matches what shows the player id's sell store open to a player
// in sight: its CharInfo in a sell store mode, or the store's title.
func storeOf(id int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.CharInfo:
			return e.ObjectID == id && openStore(e.StoreType)
		case botclient.PrivateStoreMsgSell:
			return e.OwnerID == id
		}
		return false
	}
}

// storeClosedOf matches the CharInfo of the player id out of store mode.
func storeClosedOf(id int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		ci, ok := ev.(botclient.CharInfo)
		return ok && ci.ObjectID == id && ci.StoreType == 0
	}
}

// openStore reports a store mode of a private sell store (privatestore
// OperateSell 1, OperatePackageSell 8).
func openStore(mode uint8) bool { return mode == 1 || mode == 8 }

// storeSetup is how storeBot runs its merchant.
type storeSetup struct {
	// merchant runs the merchant preset (cmd/bots -persona merchant):
	// DefaultPersona with the Damage role (D21) farming the datapack's
	// spots; otherwise partyPersona,
	// a bot that moves only for what the scenario asserts.
	merchant bool
	// brain, when set, adjusts the brain config last (commandedBy).
	brain func(*botclient.BrainConfig)
}

// storeBot runs a merchant of account with a store budget, the travel map
// and a store registry of cmd/bots' defaults, has the GM give it steel
// powder (loot no NPC shop sells, so its store sells it), recalls it to
// Talking Island village and waits until the GM sees its store open there.
// The powder goes first: a merchant recalled with nothing to sell may walk off to farm,
// and a moving bot opens no store.
func storeBot(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, account string, s storeSetup) *farmer {
	t.Helper()
	travel := travelMap(t, tg)
	stores := botclient.NewStoreRegistry(travel.Towns(), storesPerTown, storeSpacing)
	var spots *botclient.SpotRegistry
	persona := partyPersona
	if s.merchant {
		spots, persona = spotRegistry(t, tg), botclient.DefaultPersona
	}
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := persona()
			if s.merchant {
				p.Role = botclient.RoleDamage
			}
			p.Store.Budget = storeBudget
			return p
		},
		brain: func(cfg *botclient.BrainConfig) {
			cfg.Travel, cfg.Stores, cfg.Spots = travel, stores, spots
			if s.brain != nil {
				s.brain(cfg)
			}
		},
	})
	gmOn(t, ctx, gm, f)
	gmGive(t, ctx, gm, f, steelPowder, 2)
	toTown(t, ctx, tg, gm, f)
	if _, err := gm.Await(ctx, 3*time.Minute, storeOf(f.id)); err != nil {
		for _, s := range f.watch.Seen(0) {
			if l, ok := s.Event.(botclient.PrivateStoreManageListSell); ok {
				t.Logf("%s's manage list: %+v", f.name, l)
			}
		}
		t.Fatalf("the GM sees %s's store open: %v", f.name, err)
	}
	t.Logf("%s opened its store at %v", f.name, f.bot.Status().Position)
	return f
}

// tellFrom matches a whisper the player named name sent.
func tellFrom(name string) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		cs, ok := ev.(botclient.CreatureSay)
		return ok && cs.Type == chatTell && strings.EqualFold(cs.Name, name)
	}
}

// chatTell is the whisper channel of Say2 and CreatureSay.
const chatTell int32 = 2

// heard reports whether f read a line of sender on channel since index
// from: the bot had the chance to answer it.
func (f *farmer) heard(from int, sender string, channel int32) bool {
	for _, s := range f.watch.Seen(from) {
		cs, ok := s.Event.(botclient.CreatureSay)
		if ok && cs.Type == channel && strings.EqualFold(cs.Name, sender) {
			return true
		}
	}
	return false
}

// tells counts the whispers from name c reads within d.
func tells(t *testing.T, ctx context.Context, c *Client, name string, d time.Duration) int {
	t.Helper()
	n := 0
	match := tellFrom(name)
	if err := c.Watch(ctx, d, func(ev botclient.Event) {
		if match(ev) {
			n++
		}
	}); err != nil {
		t.Fatalf("%s listens for %s: %v", c.Self.Name, name, err)
	}
	return n
}
