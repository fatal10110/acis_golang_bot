package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The crafter tests' recipe: two Iron Ore make one Steel
// (a test recipe over the datapack's item ids), 40 MP a craft.
const (
	ironOre     int32 = 1869
	steel       int32 = 1880
	steelRecipe int32 = 9001
	steelScroll int32 = 9002
	steelMP     int32 = 40
)

var (
	whStoredOre = WarehouseItem{ObjectID: 0x10000400, TemplateID: ironOre, Count: 10}
	carriedOre  = Item{ObjectID: 200, TemplateID: ironOre, Count: 10}
)

// newCrafterBrain is a crafter in clan 77 with Create Item 1 and the steel
// recipe in its book, carrying items, the keeper in sight 300 away.
func newCrafterBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	p := CrafterPersona()
	p.IdleChance, p.AutoShots, p.Crafter.CheckJitter = 0, false, 0
	b, _ := newTestBrain(t, p)
	b.world.Clan.ID = 77
	recipes := map[int32]datapack.Recipe{steelRecipe: {
		ID: steelRecipe, ScrollID: steelScroll, Level: 1, MPCost: steelMP, SuccessRate: 100,
		Materials: []datapack.Ingredient{{ItemID: ironOre, Count: 2}}, Product: datapack.Ingredient{ItemID: steel, Count: 1}, Dwarven: true,
	}}
	b.cfg.Travel = &TravelMap{
		spawns: map[int32][]Point{keeperNPC: {{X: 300}}}, keepers: map[int32]bool{keeperNPC: true},
		recipes: recipes, craftItems: indexCraftItems(recipes), prices: map[int32]int32{ironOre: 10, steel: 100},
	}
	b.cfg.CraftBoard = NewCraftBoard()
	b.world.Recipes = RecipeBook{Dwarven: []int32{steelRecipe}, DwarvenKnown: true, CommonKnown: true, MaxMP: 100}
	b.world.Skills[skillCreateItem] = KnownSkill{ID: skillCreateItem, Level: 1}
	b.world.Apply(ItemList{Items: items}, t0)
	b.world.Apply(NPCInfo{ObjectID: keeperObj, NPCID: keeperNPC, Pos: Point{X: 300}}, t0)
	return b
}

func craftReq() []byte { return encodeRequestRecipeItemMakeSelf(steelRecipe) }

func madeInfo(status int32) RecipeItemMakeInfo {
	return RecipeItemMakeInfo{RecipeID: steelRecipe, Dwarven: true, MP: 70, MaxMP: 100, Status: status}
}

// TestBrainCrafterCycle runs a full cycle: the check's look, the plan, the
// withdrawal under the board's lease MinOpenGap later, five crafts at least
// craftGap apart, the products' deposit MinOpenGap after the withdrawal,
// and the next check CheckEvery after.
func TestBrainCrafterCycle(t *testing.T) {
	b := newCrafterBrain(t, adenaStack(1000))
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	steps := []trainerStep{{name: "check due: look", wantRule: "craft"}}
	steps = append(steps, openSteps(time.Second, false)...)
	steps = append(steps,
		trainerStep{
			name: "listed: plan, withdraw after MinOpenGap", at: 6 * time.Second,
			events: []Event{WarehouseWithdrawList{Type: clanWarehouseType, Items: []WarehouseItem{whStoredOre}}}, wantRule: "craft",
		},
		trainerStep{name: "no open before MinOpenGap", at: 4*time.Second + warehouseOpenGap - time.Millisecond, wantRule: "craft"},
		trainerStep{name: "reopen under the lease", at: 4*time.Second + warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
		trainerStep{
			name: "withdraw link", at: 65 * time.Second, events: []Event{keeperPage(keeperObj, "WithdrawC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(clanWarehouseCommand(keeperObj, false)),
		},
		trainerStep{
			name: "withdraw the plan's ore", at: 66 * time.Second,
			setup: func(b *Brain) {
				if !board.HoldsLease(clan, selfID, at(66*time.Second)) {
					t.Fatal("withdrawing without the board's lease")
				}
			},
			events:   []Event{WarehouseWithdrawList{Type: clanWarehouseType, Items: []WarehouseItem{whStoredOre}}},
			wantRule: "warehouse", wantKind: actWarehouseWithdraw, wantPayload: withdrawReq(warehouseRow{whStoredOre.ObjectID, ironOre, 10}),
		},
		trainerStep{
			name: "withdrawn: release the lease, craft", at: 67 * time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{ItemAdded, carriedOre}}}},
			wantRule: "craft", wantKind: actCraft, wantPayload: craftReq(),
		},
	)
	runTrainerSteps(t, b, steps)
	if board.HoldsLease(clan, selfID, at(67*time.Second)) {
		t.Fatal("lease held after the withdrawal")
	}
	// Five crafts, each answered at once: never two within craftGap.
	sent := at(67 * time.Second)
	for i := 1; i <= 5; i++ {
		evs := []Event{madeInfo(RecipeMadeSuccess)}
		if i == 5 {
			evs = append(evs, InventoryUpdate{Changes: []ItemChange{{ItemRemoved, carriedOre}, {ItemAdded, Item{ObjectID: 300, TemplateID: steel, Count: 5}}}})
		}
		answered := sent.Add(100 * time.Millisecond)
		if err := b.handle(evs, answered); err != nil {
			t.Fatal(err)
		}
		if rule, a := b.decide(sent.Add(craftGap - time.Millisecond)); rule != "craft" || a.kind != actHold {
			t.Fatalf("craft %d: decide inside craftGap = %q %d, want a hold", i, rule, a.kind)
		}
		next := sent.Add(craftGap + craftJitter)
		rule, a := b.decide(next)
		if i == 5 {
			if rule != "craft" || a.kind != actHold || b.craft.phase != craftDeposit {
				t.Fatalf("after the last craft: %q %d phase %d, want the deposit", rule, a.kind, b.craft.phase)
			}
			break
		}
		if rule != "craft" || a.kind != actCraft {
			t.Fatalf("craft %d: decide = %q %d, want the next craft", i+1, rule, a.kind)
		}
		if ok, err := b.send(a, next); err != nil || !ok {
			t.Fatalf("craft %d: send = %v, %v", i+1, ok, err)
		}
		sent = next
	}
	if got, want := b.craft.req.Deposit, []WarehouseRow{{TemplateID: steel, Count: 5}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deposit = %+v, want %+v", got, want)
	}
	runTrainerSteps(t, b, []trainerStep{
		{name: "deposit after MinOpenGap", at: 65*time.Second + warehouseOpenGap, wantRule: "warehouse", wantKind: actTalk},
		{
			name: "deposit link", at: 126 * time.Second, events: []Event{keeperPage(keeperObj, "DepositC")},
			wantRule: "warehouse", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(clanWarehouseCommand(keeperObj, true)),
		},
		{
			name: "deposit the steel", at: 127 * time.Second,
			events:   []Event{WarehouseDepositList{Type: clanWarehouseType, Adena: 1000, Items: []WarehouseItem{{ObjectID: 300, TemplateID: steel, Count: 5}}}},
			wantRule: "warehouse", wantKind: actWarehouseDeposit, wantPayload: depositReq(warehouseRow{300, steel, 5}),
		},
		{
			name: "deposited: the cycle ends", at: 128 * time.Second,
			events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemRemoved, Item{ObjectID: 300, TemplateID: steel, Count: 5}}}}}, wantRule: "craft",
		},
		{name: "idle until CheckEvery", at: 128*time.Second + 5*time.Minute - time.Millisecond},
		{name: "next check", at: 128*time.Second + 5*time.Minute, wantRule: "craft"},
	})
	if es := board.Crafters(clan); len(es) != 1 || es[0].Bot != selfID || es[0].CreateItem != 1 || !reflect.DeepEqual(es[0].Recipes, []int32{steelRecipe}) {
		t.Fatalf("board crafters = %+v, want this crafter's entry", es)
	}
}

// makingBrain is a crafter carrying ten ore, crafting three steel.
func makingBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newCrafterBrain(t, append([]Item{adenaStack(1000), carriedOre}, items...)...)
	b.craft.plan = craftplan.Plan{Chains: []craftplan.Chain{{Product: steel, Count: 3, Order: -1, Steps: []craftplan.Step{{Recipe: steelRecipe, Times: 3}}}}}
	b.startMake(t0)
	return b
}

// sendCraft decides and sends the next craft at now.
func sendCraft(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	rule, a := b.decide(now)
	if rule != "craft" || a.kind != actCraft {
		t.Fatalf("decide at %v = %q %d, want a craft", now.Sub(t0), rule, a.kind)
	}
	if ok, err := b.send(a, now); err != nil || !ok {
		t.Fatalf("send = %v, %v", ok, err)
	}
}

// TestBrainCrafterOutcomes takes each answer to a craft: a success and a
// failed roll count a craft done; a missing material re-plans, short MP
// rests and the combat refusal (no 0xd7) waits, each counting toward the
// breaker; another recipe's window answers nothing.
func TestBrainCrafterOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		events      []Event
		wantPending bool
		wantDone    int64
		wantFails   int
		wantReplan  bool
		// wantRule is the decision after craftGap.
		wantRule string
	}{
		{"success", []Event{SystemMessage{ID: 53}, madeInfo(RecipeMadeSuccess)}, false, 1, 0, false, "craft"},
		{"failed roll", []Event{SystemMessage{ID: msgItemMixingFailed}, madeInfo(RecipeMadeFailed)}, false, 1, 0, false, "craft"},
		{"missing material", []Event{SystemMessage{ID: msgMissingS2S1ToCreate}, madeInfo(RecipeMadeFailed)}, false, 0, 1, true, "craft"},
		{"not enough MP rests", []Event{SystemMessage{ID: msgNotEnoughMP}, madeInfo(RecipeMadeFailed)}, false, 0, 1, false, "rest"},
		{"combat refusal", []Event{SystemMessage{ID: msgCantOperateStoreDuringCombat}}, false, 0, 1, false, "craft"},
		{"status 0 with no message", []Event{madeInfo(RecipeMadeFailed)}, false, 0, 1, false, "craft"},
		{"another recipe's window", []Event{RecipeItemMakeInfo{RecipeID: 1, Status: RecipeMadeSuccess}}, true, 0, 0, false, "craft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := makingBrain(t)
			// MP enough for a craft, under StandMP: a rest asked sits.
			b.world.Self.MP = 50
			sendCraft(t, b, at(time.Second))
			if err := b.handle(tt.events, at(time.Second+100*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			c := &b.craft
			if c.pending != tt.wantPending || c.done != tt.wantDone || c.fails != tt.wantFails || c.replan != tt.wantReplan {
				t.Fatalf("pending %v done %d fails %d replan %v, want %v %d %d %v", c.pending, c.done, c.fails, c.replan,
					tt.wantPending, tt.wantDone, tt.wantFails, tt.wantReplan)
			}
			if rule, _ := b.decide(at(2 * time.Second)); rule != tt.wantRule {
				t.Fatalf("next decision rule %q, want %q", rule, tt.wantRule)
			}
		})
	}
}

// TestBrainCrafterBreaker stops the cycle after FailStreak results that are
// neither a success nor a failed roll, or TimeoutStreak unanswered crafts,
// and holds the next check for BreakerHold.
func TestBrainCrafterBreaker(t *testing.T) {
	tests := []struct {
		name   string
		answer []Event
		gap    time.Duration
		times  int
	}{
		{"fail streak", []Event{madeInfo(RecipeMadeFailed)}, time.Second, 5},
		{"timeout streak", nil, craftReplyTimeout + time.Second, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := makingBrain(t)
			now := at(time.Second)
			for i := range tt.times {
				if b.craft.phase != craftMake {
					t.Fatalf("craft %d: the cycle stopped early", i+1)
				}
				sendCraft(t, b, now)
				if err := b.handle(tt.answer, now.Add(100*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				now = now.Add(tt.gap)
			}
			if tt.answer == nil {
				// The last timeout is seen at the next decision.
				b.decide(now)
			}
			// The ore is what the next plan keeps: nothing to deposit.
			if c := &b.craft; c.phase != craftIdle || c.nextCheck.Sub(now) < b.persona.Crafter.BreakerHold-tt.gap {
				t.Fatalf("phase %d next check in %v, want idle for BreakerHold", c.phase, c.nextCheck.Sub(now))
			}
		})
	}
}

// TestBrainCrafterRefusals: no craft in combat; MP short of the recipe's
// cost rests first.
func TestBrainCrafterRefusals(t *testing.T) {
	b := makingBrain(t)
	b.world.Self.InCombat = true
	if rule, _ := b.decide(at(time.Second)); rule == "craft" {
		t.Fatal("crafted in combat")
	}
	b.world.Self.InCombat = false
	b.world.Self.MP = steelMP - 1
	if rule, a := b.decide(at(2 * time.Second)); rule != "craft" || a.kind != actHold || !b.resting {
		t.Fatalf("short MP: %q %d resting %v, want a hold into a rest", rule, a.kind, b.resting)
	}
	if rule, a := b.decide(at(3 * time.Second)); rule != "rest" || a.kind != actSitStand {
		t.Fatalf("short MP: %q %d, want to sit", rule, a.kind)
	}
}

// TestBrainCrafterWeightGuard deposits at 50 % weight what the crafts left
// to go do not use, then crafts on.
func TestBrainCrafterWeightGuard(t *testing.T) {
	b := makingBrain(t, Item{ObjectID: 300, TemplateID: steel, Count: 2})
	sendCraft(t, b, at(time.Second))
	b.world.Self.Load, b.world.Self.MaxLoad = 500, 1000
	if err := b.handle([]Event{madeInfo(RecipeMadeSuccess)}, at(time.Second+100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	b.decide(at(2 * time.Second))
	c := &b.craft
	// Two crafts to go use four of the ten ore.
	want := []WarehouseRow{{TemplateID: ironOre, Count: 6}, {TemplateID: steel, Count: 2}}
	if c.phase != craftDeposit || !c.resume || !reflect.DeepEqual(c.req.Deposit, want) {
		t.Fatalf("phase %d resume %v deposit %+v, want the guard's %+v", c.phase, c.resume, c.req.Deposit, want)
	}
	c.asked, c.askedAt = true, at(3*time.Second)
	b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: at(4 * time.Second)}
	if rule, _ := b.decide(at(5 * time.Second)); rule != "craft" || c.phase != craftMake {
		t.Fatalf("after the guard's deposit: %q phase %d, want to craft on", rule, c.phase)
	}
}

// TestBrainCrafterIdleFarming: an empty warehouse and nothing carried to
// craft end the cycle with nothing to deposit; the bot farms its spot and
// checks again ReturnEvery later.
func TestBrainCrafterIdleFarming(t *testing.T) {
	b := newCrafterBrain(t, adenaStack(1000))
	b.world.Self.Level = 10
	b.cfg.Spots = NewSpotRegistry([]datapack.Spot{testSpot("Wolves", 5000, 0, 8, 10, 3)}, 1)
	steps := []trainerStep{{name: "check due: look", wantRule: "craft"}}
	steps = append(steps, openSteps(time.Second, false)...)
	steps = append(steps,
		trainerStep{name: "empty: nothing to craft", at: 6 * time.Second, events: []Event{SystemMessage{ID: msgNoItemDepositedInW}}, wantRule: "craft"},
		trainerStep{name: "farm meanwhile", at: 7 * time.Second, wantRule: "spot", wantKind: actMove},
	)
	runTrainerSteps(t, b, steps)
	if got, want := b.craft.nextCheck, at(6*time.Second+b.persona.Crafter.ReturnEvery); !got.Equal(want) {
		t.Fatalf("next check %v, want ReturnEvery later %v", got.Sub(t0), want.Sub(t0))
	}
	if rule, _ := b.decide(b.craft.nextCheck); rule != "craft" {
		t.Fatalf("at ReturnEvery: rule %q, want the check", rule)
	}
}

// TestBrainCrafterOrder: a board order with its materials in brings the
// check early; the crafter claims it, crafts it from what it carries, and
// delivers it with the deposit.
func TestBrainCrafterOrder(t *testing.T) {
	b := newCrafterBrain(t, adenaStack(1000), Item{ObjectID: 200, TemplateID: ironOre, Count: 2})
	board, clan := b.cfg.CraftBoard, b.world.Clan.ID
	b.craft.nextCheck = at(time.Hour)
	if rule, _ := b.decide(at(time.Second)); rule == "craft" {
		t.Fatal("checked before CheckEvery with no order")
	}
	id, _ := board.Post(clan, "Member", steel, 1)
	board.MaterialsIn(clan, id)
	if rule, _ := b.decide(at(2 * time.Second)); rule != "craft" || b.craft.phase != craftLook {
		t.Fatalf("order in: rule %q phase %d, want the check", rule, b.craft.phase)
	}
	b.craftPlan(at(3*time.Second), false)
	if os := board.Orders(clan, at(3*time.Second)); len(os) != 1 || os[0].State != OrderClaimed || os[0].Crafter != selfID {
		t.Fatalf("orders = %+v, want claimed by the crafter", os)
	}
	sendCraft(t, b, at(4*time.Second))
	evs := []Event{madeInfo(RecipeMadeSuccess), InventoryUpdate{Changes: []ItemChange{
		{ItemRemoved, Item{ObjectID: 200, TemplateID: ironOre}}, {ItemAdded, Item{ObjectID: 300, TemplateID: steel, Count: 1}},
	}}}
	if err := b.handle(evs, at(4*time.Second+100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if os := board.Orders(clan, at(5*time.Second)); os[0].State != OrderCrafted {
		t.Fatalf("order %+v, want crafted", os[0])
	}
	b.decide(at(5 * time.Second))
	if c := &b.craft; c.phase != craftDeposit || c.delivering[id] != 1 {
		t.Fatalf("phase %d delivering %v, want the deposit carrying the order", c.phase, c.delivering)
	}
	b.craft.asked, b.craft.askedAt = true, at(6*time.Second)
	b.wh.status.Deposit = WarehouseResult{Outcome: WarehouseDone, At: at(7 * time.Second)}
	b.decide(at(8 * time.Second))
	if os := board.Orders(clan, at(8*time.Second)); len(os) != 0 || board.Owned(clan, "Member")[steel] != 1 {
		t.Fatalf("orders %+v owned %v, want the order delivered", os, board.Owned(clan, "Member"))
	}
}
