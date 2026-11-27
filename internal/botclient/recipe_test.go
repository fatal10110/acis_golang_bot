package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Recipe: Soulshot (D-grade) (recipes.xml recipe 20, scroll 1804).
const (
	ssdRecipe int32 = 20
	ssdScroll int32 = 1804
)

var ssdScrollItem = Item{ObjectID: 300, TemplateID: ssdScroll, Count: 1, Category: 5, SubCategory: 5}

// learnScript names scrolls one per ask and records what each ask saw.
type learnScript struct {
	scrolls []int32
	seen    []LearnResult
}

func (s *learnScript) learn(_ *World, last LearnResult, _ time.Time) int32 {
	s.seen = append(s.seen, last)
	if len(s.scrolls) == 0 {
		return 0
	}
	id := s.scrolls[0]
	s.scrolls = s.scrolls[1:]
	return id
}

// newRecipeBrain is a fighter carrying items that learns the scrolls
// script names.
func newRecipeBrain(t *testing.T, script *learnScript, items ...Item) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.AutoShots = 0, false
	p.Recipes.Learn = script.learn
	b, _ := newTestBrain(t, p)
	b.world.Apply(ItemList{Items: items}, t0)
	return b
}

// openBookSteps opens the empty common page, then the dwarven page,
// holding recipe 1, from start.
func openBookSteps(start time.Duration) []trainerStep {
	return []trainerStep{
		{name: "open the common page", at: start, wantRule: "recipe", wantKind: actRecipeBook, wantPayload: botwire.EncodeRequestRecipeBookOpen(false)},
		{name: "wait for the page", at: start + time.Second},
		{
			name: "open the dwarven page", at: start + 2*time.Second,
			events:   []Event{RecipeBookItemList{MaxMP: 100, Recipes: []int32{}}},
			wantRule: "recipe", wantKind: actRecipeBook, wantPayload: botwire.EncodeRequestRecipeBookOpen(true),
		},
		{
			name: "use the scroll", at: start + 3*time.Second,
			events:   []Event{RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{1}}},
			wantRule: "recipe", wantKind: actUseItem, wantPayload: botwire.EncodeUseItem(ssdScrollItem.ObjectID),
		},
	}
}

func TestBrainLearnRecipe(t *testing.T) {
	script := &learnScript{scrolls: []int32{ssdScroll}}
	b := newRecipeBrain(t, script, ssdScrollItem)
	steps := append(openBookSteps(0),
		trainerStep{name: "wait for the answer", at: 4 * time.Second},
		trainerStep{
			name: "learned", at: 5 * time.Second,
			events: []Event{
				SystemMessage{ID: msgS1Added},
				InventoryUpdate{Changes: []ItemChange{{State: ItemRemoved, Item: ssdScrollItem}}},
				RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{1, ssdRecipe}},
			},
		},
	)
	runTrainerSteps(t, b, steps)
	want := LearnResult{Scroll: ssdScroll, Outcome: LearnDone, Added: ssdRecipe, At: at(5 * time.Second)}
	if b.learn.last != want {
		t.Fatalf("last = %+v, want %+v", b.learn.last, want)
	}
	if !b.world.Recipes.Has(ssdRecipe) || !b.world.Recipes.Has(1) || b.world.Recipes.MaxMP != 100 {
		t.Fatalf("book = %+v, want recipes 1 and %d", b.world.Recipes, ssdRecipe)
	}
	// A learned recipe asks for the next scroll at once.
	if got := script.seen[len(script.seen)-1]; got != want {
		t.Fatalf("Learn saw %+v, want %+v", got, want)
	}
}

// TestBrainLearnRecipeSubclass: the server spends the scroll and says it
// was added, but a subclass's book stays the base class's.
func TestBrainLearnRecipeSubclass(t *testing.T) {
	script := &learnScript{scrolls: []int32{ssdScroll}}
	b := newRecipeBrain(t, script, ssdScrollItem)
	steps := append(openBookSteps(0), trainerStep{
		name: "added, page unchanged", at: 4 * time.Second,
		events: []Event{SystemMessage{ID: msgS1Added}, RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{1}}},
	})
	runTrainerSteps(t, b, steps)
	if got := b.learn.last; got.Outcome != LearnDone || got.Added != 0 {
		t.Fatalf("last = %+v, want done with nothing added", got)
	}
}

// TestBrainLearnRecipeRefused ends the attempt at each refusal, with no
// page after it, and waits learnRetry before the next.
func TestBrainLearnRecipeRefused(t *testing.T) {
	for _, msg := range learnRefusals {
		t.Run(itoa(msg), func(t *testing.T) {
			script := &learnScript{scrolls: []int32{ssdScroll, ssdScroll}}
			b := newRecipeBrain(t, script, ssdScrollItem)
			steps := append(openBookSteps(0),
				trainerStep{name: "refused", at: 4 * time.Second, events: []Event{SystemMessage{ID: msg}}},
				trainerStep{name: "no retry before learnRetry", at: 4*time.Second + learnRetry - time.Millisecond},
				trainerStep{
					name: "retry due", at: 4*time.Second + learnRetry,
					wantRule: "recipe", wantKind: actUseItem, wantPayload: botwire.EncodeUseItem(ssdScrollItem.ObjectID),
				},
			)
			runTrainerSteps(t, b, steps)
			want := LearnResult{Scroll: ssdScroll, Outcome: LearnRefused, Msg: msg, At: at(4 * time.Second)}
			if got := script.seen[len(script.seen)-1]; got != want {
				t.Fatalf("Learn saw %+v, want %+v", got, want)
			}
		})
	}
}

func TestBrainLearnRecipeTimeout(t *testing.T) {
	script := &learnScript{scrolls: []int32{ssdScroll}}
	b := newRecipeBrain(t, script, ssdScrollItem)
	steps := append(openBookSteps(0),
		// A message of no attempt, and a page with no S1Added, end nothing.
		trainerStep{
			name: "still waiting", at: 3*time.Second + recipeReplyTimeout - time.Millisecond,
			events: []Event{SystemMessage{ID: msgItemMixingFailed}, RecipeBookItemList{Dwarven: true, Recipes: []int32{1}}},
		},
		trainerStep{name: "unanswered", at: 3*time.Second + recipeReplyTimeout},
	)
	runTrainerSteps(t, b, steps)
	want := LearnResult{Scroll: ssdScroll, Outcome: LearnTimeout, At: at(3*time.Second + recipeReplyTimeout)}
	if b.learn.last != want || !b.learn.retryAt.Equal(want.At.Add(learnRetry)) {
		t.Fatalf("last = %+v retry %v, want %+v", b.learn.last, b.learn.retryAt, want)
	}
}

// TestBrainLearnRecipeBookOpen ends the attempt when a page never comes or
// the server refuses the open while the bot casts.
func TestBrainLearnRecipeBookOpen(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		at     time.Duration
		want   LearnResult
	}{
		{"unanswered", nil, recipeReplyTimeout, LearnResult{Scroll: ssdScroll, Outcome: LearnTimeout, At: at(recipeReplyTimeout)}},
		{
			"casting",
			[]Event{SystemMessage{ID: msgNoRecipeBookWhileCasting}},
			time.Second,
			LearnResult{Scroll: ssdScroll, Outcome: LearnRefused, Msg: msgNoRecipeBookWhileCasting, At: at(time.Second)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := &learnScript{scrolls: []int32{ssdScroll}}
			b := newRecipeBrain(t, script, ssdScrollItem)
			runTrainerSteps(t, b, []trainerStep{
				{name: "open", at: 0, wantRule: "recipe", wantKind: actRecipeBook},
				{name: "still waiting", at: tt.at - time.Millisecond},
				{name: "ended", at: tt.at, events: tt.events},
			})
			if b.learn.last != tt.want || b.world.Recipes.CommonKnown {
				t.Fatalf("last = %+v, want %+v", b.learn.last, tt.want)
			}
		})
	}
}

// TestBrainLearnRecipeNotDue sends nothing with no scroll carried, in
// combat, or with no Learn hook.
func TestBrainLearnRecipeNotDue(t *testing.T) {
	t.Run("no scroll", func(t *testing.T) {
		script := &learnScript{scrolls: []int32{ssdScroll}}
		b := newRecipeBrain(t, script)
		runTrainerSteps(t, b, []trainerStep{{name: "nothing", at: 0}})
		if got := b.learn.last; got.Outcome != LearnNoScroll || got.Scroll != ssdScroll {
			t.Fatalf("last = %+v, want no scroll", got)
		}
	})
	t.Run("combat", func(t *testing.T) {
		script := &learnScript{scrolls: []int32{ssdScroll}}
		b := newRecipeBrain(t, script, ssdScrollItem)
		b.world.Self.InCombat = true
		if rule, a := b.decide(t0); rule == "recipe" || a.kind != actHold || len(script.seen) != 0 {
			t.Fatalf("decide = %q %d, asked %d; want no attempt in combat", rule, a.kind, len(script.seen))
		}
	})
	t.Run("no hook", func(t *testing.T) {
		b := newRecipeBrain(t, &learnScript{}, ssdScrollItem)
		b.persona.Recipes.Learn = nil
		if rule, _ := b.decide(t0); rule == "recipe" {
			t.Fatal("learned with no Learn hook")
		}
	})
}

func TestRecipeBookApply(t *testing.T) {
	w := newTestWorld(t)
	w.Apply(RecipeBookItemList{Dwarven: true, MaxMP: 80, Recipes: []int32{1, 20}}, t0)
	w.Apply(RecipeBookItemList{MaxMP: 90, Recipes: []int32{300}}, t0)
	got := w.Recipes
	if !slices.Equal(got.Dwarven, []int32{1, 20}) || !slices.Equal(got.Common, []int32{300}) ||
		!got.DwarvenKnown || !got.CommonKnown || got.MaxMP != 90 {
		t.Fatalf("book = %+v", got)
	}
	if !got.Has(300) || !got.Has(20) || got.Has(2) {
		t.Fatalf("Has wrong on %+v", got)
	}
	w.Apply(RecipeBookItemList{Dwarven: true, MaxMP: 90, Recipes: []int32{}}, t0)
	if w.Recipes.Has(1) || !w.Recipes.Has(300) {
		t.Fatalf("a fresh dwarven page left %+v", w.Recipes)
	}
}
