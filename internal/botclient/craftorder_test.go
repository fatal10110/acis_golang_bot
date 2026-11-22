package botclient

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The craft order tests' second product: Mithril, a level 5 dwarven
// recipe the crafter's book does not hold.
const (
	mithril       int32 = 1876
	mithrilRecipe int32 = 9003
	leaderID            = int32(0x30000002)
	leaderName          = "Lead"
)

// newOrderBrain is newCrafterBrain whose clan leader is leaderName, obeying
// the commander gmID, named "GM", with the steel and mithril recipes named.
func newOrderBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newCrafterBrain(t, items...)
	b.world.Clan.Leader = leaderName
	b.cfg.Authority = NewAuthority(AuthorityConfig{Commanders: []int32{gmID}, Clock: b.clock})
	m := b.cfg.Travel
	m.recipes[mithrilRecipe] = datapack.Recipe{
		ID: mithrilRecipe, Level: 5, MPCost: 10, SuccessRate: 100, Dwarven: true,
		Materials: []datapack.Ingredient{{ItemID: ironOre, Count: 1}}, Product: datapack.Ingredient{ItemID: mithril, Count: 1},
	}
	m.craftItems = indexCraftItems(m.recipes)
	m.products, m.itemNames = indexProductNames(m.recipes, nil, map[int32]datapack.EtcItem{
		steel: {ID: steel, Name: "Steel"}, mithril: {ID: mithril, Name: "Mithril Ore"},
	})
	return b
}

// knownWarehouse sets the last clan warehouse list to ore Iron Ore.
func knownWarehouse(b *Brain, ore int32) {
	b.wh.status.Known = true
	b.wh.status.Contents = nil
	if ore > 0 {
		b.wh.status.Contents = []WarehouseItem{{ObjectID: 0x10000400, TemplateID: ironOre, Count: ore}}
	}
}

// say2Frames runs b's decisions for 10 s after now and returns the Say2
// frames it sent.
func say2Frames(t *testing.T, b *Brain, out *recordSender, now time.Time) [][]byte {
	t.Helper()
	for d := time.Duration(0); d <= 10*time.Second; d += 500 * time.Millisecond {
		if err := b.wake(now.Add(d)); err != nil {
			t.Fatal(err)
		}
	}
	var got [][]byte
	for _, f := range out.frames {
		if f[0] == opGameSay2 {
			got = append(got, f)
		}
	}
	return got
}

func TestParseCraft(t *testing.T) {
	tests := []struct {
		text  string
		item  string
		count int64
	}{
		{"craft steel", "steel", 1},
		{"  CRAFT   Mithril   Ore  12 ", "Mithril Ore", 12},
		{"craft steel 10000", "steel", 10000},
		{"craft steel 0", "steel", 0},
		{"craft steel 10001", "steel", 0},
		{"craft steel 99999999999999999999", "steel", 0},
		{"craft steel -3", "steel -3", 1},
		{"craft", "", 1},
		{"craft 5", "", 5},
	}
	for _, tt := range tests {
		c, ok := parseCommand(tt.text)
		if want := (command{kind: cmdCraft, item: tt.item, count: tt.count}); !ok || c != want {
			t.Errorf("parseCommand(%q) = %+v, %v, want %+v", tt.text, c, ok, want)
		}
	}
}

// TestBrainCraftOrders checks who may order a craft, on which channel the
// one answer goes, what it says and what goes on the board.
func TestBrainCraftOrders(t *testing.T) {
	type order struct {
		by    OrderSource
		name  string
		item  int32
		count int64
	}
	tell := func(text string) []byte { return encodeSay2Tell(leaderName, text) }
	clan := func(text string) []byte { return encodeSay2(text, chatClan) }
	lead := func(text string) CreatureSay { return whisper(leaderID, leaderName, text) }
	tests := []struct {
		name  string
		setup func(*Brain)
		ev    CreatureSay
		want  []byte
		order *order
	}{
		{
			name: "leader whisper", setup: func(b *Brain) { knownWarehouse(b, 10) }, ev: lead("craft steel 3"),
			want: tell("crafting 3 Steel"), order: &order{OrderByLeader, leaderName, steel, 3},
		},
		{
			name: "leader in clan chat", setup: func(b *Brain) { knownWarehouse(b, 10) },
			ev:   CreatureSay{ObjectID: leaderID, Type: chatClan, Name: leaderName, Text: "craft STEEL 3"},
			want: clan("crafting 3 Steel"), order: &order{OrderByLeader, leaderName, steel, 3},
		},
		{
			name: "GM whisper", setup: func(b *Brain) { knownWarehouse(b, 10) }, ev: whisper(gmID, "GM", "craft steel"),
			want: encodeSay2Tell("GM", "crafting 1 Steel"), order: &order{OrderByGM, "GM", steel, 1},
		},
		{
			name: "GM in clan chat", setup: func(b *Brain) { knownWarehouse(b, 10) },
			ev:   CreatureSay{ObjectID: gmID, Type: chatClan, Name: "GM", Text: "craft steel 2"},
			want: clan("crafting 2 Steel"), order: &order{OrderByGM, "GM", steel, 2},
		},
		{name: "another player's whisper", setup: func(b *Brain) { knownWarehouse(b, 10) }, ev: whisper(pcID, "Alden", "craft steel")},
		{
			name: "another member in clan chat", setup: func(b *Brain) { knownWarehouse(b, 10) },
			ev: CreatureSay{ObjectID: pcID, Type: chatClan, Name: "Alden", Text: "craft steel"},
		},
		{
			name: "the bot's own clan line", setup: func(b *Brain) { knownWarehouse(b, 10); b.world.Clan.Leader = "Me" },
			ev: CreatureSay{ObjectID: selfID, Type: chatClan, Name: "Me", Text: "craft steel"},
		},
		{
			name: "a whisper on the all channel", setup: func(b *Brain) { knownWarehouse(b, 10) },
			ev: CreatureSay{ObjectID: leaderID, Type: chatAll, Name: leaderName, Text: "craft steel"},
		},
		{
			name: "leader in a no clan bot", setup: func(b *Brain) { knownWarehouse(b, 10); b.world.Clan.ID = 0 },
			ev: lead("craft steel"),
		},
		{
			name: "not a crafter", setup: func(b *Brain) { knownWarehouse(b, 10); b.persona.Crafter = CrafterPolicy{} },
			ev: lead("craft steel"),
		},
		{name: "unknown item", ev: lead("craft gold bar 2"), want: tell("no recipe makes gold bar")},
		{name: "count out of bounds", ev: lead("craft steel 0"), want: tell("count 1 to 10000")},
		{name: "no item", ev: lead("craft 4"), want: tell("say: craft <item> [count]")},
		{name: "Create Item too low", ev: lead("craft mithril ore"), want: tell("my Create Item is too low for Mithril Ore (needs 5)")},
		{
			name: "recipe out of the book", setup: func(b *Brain) { b.world.Skills[skillCreateItem] = KnownSkill{ID: skillCreateItem, Level: 5} },
			ev: lead("craft mithril ore"), want: tell("no recipe for Mithril Ore in my book"),
		},
		{
			name: "another crafter's book",
			setup: func(b *Brain) {
				b.cfg.CraftBoard.Publish(b.world.Clan.ID, CrafterEntry{Bot: 0x10000009, Name: "Smith", Recipes: []int32{mithrilRecipe}, CreateItem: 5})
			},
			ev: lead("craft mithril ore 2"), want: tell("queued 2 Mithril Ore for Smith"), order: &order{OrderByLeader, leaderName, mithril, 2},
		},
		{name: "materials missing", setup: func(b *Brain) { knownWarehouse(b, 0) }, ev: lead("craft steel 3"), want: tell("materials missing for Steel")},
		{
			name: "part of the count", setup: func(b *Brain) { knownWarehouse(b, 4) }, ev: lead("craft steel 3"),
			want: tell("crafting 2 of 3 Steel, materials missing for the rest"), order: &order{OrderByLeader, leaderName, steel, 3},
		},
		{
			name: "warehouse not listed yet", ev: lead("craft steel 3"),
			want: tell("queued 3 Steel, warehouse not checked yet"), order: &order{OrderByLeader, leaderName, steel, 3},
		},
		{
			name: "carried ore before any list", setup: func(b *Brain) { b.world.Apply(ItemList{Items: []Item{carriedOre}}, t0) },
			ev: lead("craft steel 3"), want: tell("crafting 3 Steel"), order: &order{OrderByLeader, leaderName, steel, 3},
		},
		{
			name:  "leader named with the bot marker: queued, no answer",
			setup: func(b *Brain) { knownWarehouse(b, 10); b.world.Clan.Leader = "LeadBot" },
			ev:    whisper(leaderID, "LeadBot", "craft steel"), order: &order{OrderByLeader, "LeadBot", steel, 1},
		},
		{
			name: "chat Total reached: queued, no answer",
			setup: func(b *Brain) {
				knownWarehouse(b, 10)
				for range b.persona.Chat.Total {
					b.cfg.chat.count(t0, &b.persona.Chat)
				}
			},
			ev: lead("craft steel"), order: &order{OrderByLeader, leaderName, steel, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newOrderBrain(t)
			out := b.out.(*recordSender)
			if tt.setup != nil {
				tt.setup(b)
			}
			if err := b.handle([]Event{tt.ev}, t0); err != nil {
				t.Fatal(err)
			}
			var got []order
			for _, o := range b.cfg.CraftBoard.Orders(b.world.Clan.ID, t0) {
				if o.State != OrderMaterialsIn {
					t.Fatalf("order %+v not open for a claim", o)
				}
				got = append(got, order{o.By, o.Member, o.Product, o.Count})
			}
			var want []order
			if tt.order != nil {
				want = []order{*tt.order}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("board orders %+v, want %+v", got, want)
			}
			var wantFrames [][]byte
			if tt.want != nil {
				wantFrames = [][]byte{tt.want}
			}
			if frames := say2Frames(t, b, out, t0); !reflect.DeepEqual(frames, wantFrames) {
				t.Fatalf("Say2 frames %q, want %q", frames, wantFrames)
			}
		})
	}
}

// TestBrainCraftOrderLeaderOtherCommands: the clan leader orders crafts
// only; every other command stays GM-only, by whisper or in clan chat.
func TestBrainCraftOrderLeaderOtherCommands(t *testing.T) {
	for _, ev := range []CreatureSay{
		whisper(leaderID, leaderName, "follow"),
		whisper(leaderID, leaderName, "stay"),
		whisper(leaderID, leaderName, "logout"),
		{ObjectID: leaderID, Type: chatClan, Name: leaderName, Text: "follow"},
		{ObjectID: gmID, Type: chatClan, Name: "GM", Text: "stay"},
	} {
		b := newOrderBrain(t)
		if err := b.handle([]Event{ev}, t0); err != nil {
			t.Fatal(err)
		}
		if o := b.orders; o.follow != 0 || o.stay || b.logout.commanded || o.reply.kind != actHold {
			t.Fatalf("%q in channel %d obeyed: %+v", ev.Text, ev.Type, o)
		}
	}
}

// TestBrainCraftOrderClanLineAnsweredOnce: two crafters of a clan hear the
// leader's clan line; one answers it and one order goes on the board.
func TestBrainCraftOrderClanLineAnsweredOnce(t *testing.T) {
	b1, b2 := newOrderBrain(t), newOrderBrain(t)
	b2.cfg.CraftBoard = b1.cfg.CraftBoard
	line := CreatureSay{ObjectID: leaderID, Type: chatClan, Name: leaderName, Text: "craft steel"}
	var frames int
	for _, b := range []*Brain{b1, b2} {
		knownWarehouse(b, 10)
		if err := b.handle([]Event{line}, t0); err != nil {
			t.Fatal(err)
		}
		frames += len(say2Frames(t, b, b.out.(*recordSender), t0))
	}
	if n := len(b1.cfg.CraftBoard.Orders(b1.world.Clan.ID, t0)); frames != 1 || n != 1 {
		t.Fatalf("%d answers and %d orders, want 1 and 1", frames, n)
	}
	// The same line once craftHeardTTL passed is a new order.
	if err := b2.handle([]Event{line}, at(craftHeardTTL)); err != nil {
		t.Fatal(err)
	}
	if n := len(b1.cfg.CraftBoard.Orders(b1.world.Clan.ID, t0)); n != 2 {
		t.Fatalf("%d orders after craftHeardTTL, want 2", n)
	}
}

// deliverGMOrder puts a crafted order by by, named name, for 3 Steel on
// b's board and delivers it at now.
func deliverGMOrder(t *testing.T, b *Brain, by OrderSource, name string, now time.Time) {
	t.Helper()
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	id, ok := board.PostCommand(clan, by, name, steel, 3)
	if !ok || !board.Claim(clan, id, selfID, now) || !board.Crafted(clan, id, selfID, now) {
		t.Fatal("order not crafted")
	}
	b.craft.delivering = map[OrderID]int64{id: 3}
	b.deliverOrders(now)
}

// TestBrainCraftOrderAnnounce checks the one clan chat line of a delivered
// GM order: through the chat gate, none for a leader's order, a name with
// the bot marker or past the chat Total.
func TestBrainCraftOrderAnnounce(t *testing.T) {
	line := encodeSay2("crafted 3 Steel for GM, in the clan warehouse", chatClan)
	t.Run("two GM orders, chatGap apart", func(t *testing.T) {
		b := newOrderBrain(t)
		deliverGMOrder(t, b, OrderByGM, "GM", t0)
		deliverGMOrder(t, b, OrderByGM, "GM", t0)
		var sent []time.Time
		for d := time.Duration(0); d <= 2*chatGap; d += 100 * time.Millisecond {
			now := t0.Add(d)
			if rule, a := b.decide(now); rule == "announce" {
				if a.kind != actAnnounce || !bytes.Equal(a.payload, line) {
					t.Fatalf("announce %d %q, want %q", a.kind, a.payload, line)
				}
				if ok, err := b.send(a, now); err != nil || !ok {
					t.Fatalf("send = %v, %v", ok, err)
				}
				sent = append(sent, now)
			}
		}
		if len(sent) != 2 || sent[1].Sub(sent[0]) < chatGap {
			t.Fatalf("announced at %v, want twice chatGap apart", sent)
		}
	})
	for _, tt := range []struct {
		name  string
		by    OrderSource
		who   string
		setup func(*Brain)
	}{
		{name: "leader order", by: OrderByLeader, who: leaderName},
		{name: "GM with the bot marker", by: OrderByGM, who: "GMBot"},
		{name: "chat Total reached", by: OrderByGM, who: "GM", setup: func(b *Brain) {
			for range b.persona.Chat.Total {
				b.cfg.chat.count(t0, &b.persona.Chat)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newOrderBrain(t)
			if tt.setup != nil {
				tt.setup(b)
			}
			deliverGMOrder(t, b, tt.by, tt.who, t0)
			if len(b.announce) != 0 {
				t.Fatalf("announced %q", b.announce[0].payload)
			}
		})
	}
}

// TestBrainCraftOrderAnswerCountsInTotal: an order's answer counts in the
// ChatPolicy's Total, so the whisper replies after it stay capped.
func TestBrainCraftOrderAnswerCountsInTotal(t *testing.T) {
	b := newOrderBrain(t)
	knownWarehouse(b, 10)
	for i := range b.persona.Chat.Total {
		if b.cfg.chat.full(t0, &b.persona.Chat) {
			t.Fatalf("Total reached after %d answers", i)
		}
		if err := b.handle([]Event{whisper(leaderID, leaderName, "craft steel")}, t0); err != nil {
			t.Fatal(err)
		}
	}
	if !b.cfg.chat.full(t0, &b.persona.Chat) {
		t.Fatal("answers not counted in the Total")
	}
}

// TestBrainCraftOrdersPlanned: the claimed orders reach the planner as the
// members' first, then the leader's and GMs'; a leader or GM order a plan
// from a fresh list cannot start is cancelled, a member's never.
func TestBrainCraftOrdersPlanned(t *testing.T) {
	b := newOrderBrain(t)
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	gm, _ := board.PostCommand(clan, OrderByGM, "GM", steel, 1)
	member, _ := board.Post(clan, "Alden", steel, 2)
	board.MaterialsIn(clan, member)
	orders := b.claimCraftOrders(t0)
	if len(orders) != 2 || orders[0].ID != member || orders[1].ID != gm {
		t.Fatalf("claimed %+v, want the member's order first", orders)
	}
	b.craft.orders, b.craft.plan = orders, craftplan.Plan{}
	b.cancelUnplanned()
	left := board.Orders(clan, t0)
	if len(left) != 1 || left[0].ID != member {
		t.Fatalf("orders after the cancel %+v, want the member's only", left)
	}
}

func TestCraftBoardCommandOrders(t *testing.T) {
	board := NewCraftBoard()
	const clan, bot = int32(77), int32(5)
	if _, ok := board.PostCommand(clan, OrderByMember, "Alden", steel, 1); ok {
		t.Fatal("PostCommand took a member order")
	}
	if _, ok := board.PostCommand(0, OrderByGM, "GM", steel, 1); ok {
		t.Fatal("PostCommand took an order of no clan")
	}
	member, _ := board.Post(clan, "Alden", steel, 1)
	board.MaterialsIn(clan, member)
	var first OrderID
	for i := range craftCommandCap {
		id, ok := board.PostCommand(clan, OrderByLeader, leaderName, steel, 1)
		if !ok {
			t.Fatalf("order %d refused under the cap", i)
		}
		if i == 0 {
			first = id
		}
	}
	if _, ok := board.PostCommand(clan, OrderByGM, "GM", steel, 1); ok {
		t.Fatal("PostCommand past craftCommandCap")
	}
	switch {
	case board.Cancel(clan, first, bot):
		t.Fatal("Cancel of an unclaimed order")
	case !board.Claim(clan, first, bot, t0) || board.Cancel(clan, first, bot+1):
		t.Fatal("Cancel of another bot's claim")
	case !board.Cancel(clan, first, bot):
		t.Fatal("Cancel of the bot's claim refused")
	case !board.Claim(clan, member, bot, t0) || board.Cancel(clan, member, bot):
		t.Fatal("Cancel of a member order")
	}
	if _, ok := board.PostCommand(clan, OrderByGM, "GM", steel, 1); !ok {
		t.Fatal("PostCommand refused under the cap after a cancel")
	}
	if !board.Heard(clan, "Lead", "craft steel", t0) || board.Heard(clan, "Lead", "craft steel", at(craftHeardTTL-time.Millisecond)) {
		t.Fatal("Heard took a line twice within craftHeardTTL")
	}
	if !board.Heard(clan, "GM", "craft steel", t0) || !board.Heard(clan, "Lead", "craft steel", at(craftHeardTTL)) || board.Heard(0, "Lead", "x", t0) {
		t.Fatal("Heard keyed or expired wrong")
	}
}

func TestClipChat(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hi", "hi"},
		{strings.Repeat("a", 100), strings.Repeat("a", 100)},
		{strings.Repeat("a", 101), strings.Repeat("a", 100)},
		{strings.Repeat("a", 99) + "😀", strings.Repeat("a", 99)},
		{strings.Repeat("é", 120), strings.Repeat("é", 100)},
	}
	for _, tt := range tests {
		if got := clipChat(tt.in); got != tt.want {
			t.Errorf("clipChat(%d bytes) = %d bytes, want %d", len(tt.in), len(got), len(tt.want))
		}
	}
}

func TestIndexProductNames(t *testing.T) {
	recipes := map[int32]datapack.Recipe{
		1: {ID: 1, Product: datapack.Ingredient{ItemID: 10}},
		2: {ID: 2, Product: datapack.Ingredient{ItemID: 20}},
		3: {ID: 3, Product: datapack.Ingredient{ItemID: 30}},
		4: {ID: 4, Product: datapack.Ingredient{ItemID: 40}},
	}
	items := map[int32]datapack.Item{10: {ID: 10, Name: "Sword  of  Delusion"}, 40: {ID: 40, Name: "Steel"}}
	etc := map[int32]datapack.EtcItem{20: {ID: 20, Name: "Steel"}}
	ids, names := indexProductNames(recipes, items, etc)
	if want := map[string]int32{"sword of delusion": 10, "steel": 20}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids %v, want %v", ids, want)
	}
	if want := map[int32]string{10: "Sword  of  Delusion", 20: "Steel", 40: "Steel"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names %v, want %v", names, want)
	}
}
