package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// recipeCraftStep is one step of a crafter that learns a recipe scroll
// while its craft row may want the book too (#652): events in, a decision
// through the rule table, the send, then the attempt's state.
type recipeCraftStep struct {
	name   string
	at     time.Duration
	setup  func(b *Brain)
	events []Event
	// wantRule and wantBook, when wantBook is set, are the rule that opens
	// the book and the exact open; a nil wantBook wants no book open.
	wantRule string
	wantBook []byte
	// wantLearn is the attempt after the send.
	wantLearn learnView
}

// learnView is the part of an attempt the steps check.
type learnView struct {
	scroll               int32
	opening, used, added bool
	waitUntil            time.Time
}

func viewLearn(e *recipeLearn) learnView {
	return learnView{scroll: e.scroll, opening: e.opening, used: e.used, added: e.added, waitUntil: e.waitUntil}
}

// newRecipeCrafterBrain is a crafter (newCrafterBrain) whose book is not
// known yet, carrying ssdScrollItem, that learns the scrolls script names.
func newRecipeCrafterBrain(t *testing.T, script *learnScript) *Brain {
	t.Helper()
	b := newCrafterBrain(t, ssdScrollItem)
	b.world.Recipes = RecipeBook{}
	b.persona.Recipes.Learn = script.learn
	return b
}

func runRecipeCraftSteps(t *testing.T, b *Brain, steps []recipeCraftStep) {
	t.Helper()
	for _, st := range steps {
		now := at(st.at)
		if st.setup != nil {
			st.setup(b)
		}
		if err := b.handle(st.events, now); err != nil {
			t.Fatalf("%s: handle: %v", st.name, err)
		}
		rule, a := b.decide(now)
		switch {
		case st.wantBook == nil && a.kind == actRecipeBook:
			t.Fatalf("%s: rule %q opens the book % x, want no open", st.name, rule, a.payload)
		case st.wantBook != nil && (rule != st.wantRule || a.kind != actRecipeBook || !bytes.Equal(a.payload, st.wantBook)):
			t.Fatalf("%s: decide = rule %q action %d % x, want rule %q open % x", st.name, rule, a.kind, a.payload, st.wantRule, st.wantBook)
		}
		if a.kind != actHold {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("%s: send = %v, %v", st.name, sent, err)
			}
		}
		if got := viewLearn(&b.learn); got != st.wantLearn {
			t.Fatalf("%s: attempt = %+v, want %+v (last %+v)", st.name, got, st.wantLearn, b.learn.last)
		}
	}
}

// TestBrainRecipeCraftRowDuringAttempt pins #652: while the recipe errand
// waits for its page, the craft row's check (due, in a clan, the book not
// known) opens no page of its own, so the attempt's open, its timeout and
// its end stay the errand's.
func TestBrainRecipeCraftRowDuringAttempt(t *testing.T) {
	script := &learnScript{scrolls: []int32{ssdScroll}}
	b := newRecipeCrafterBrain(t, script)
	opening := func(at time.Duration) learnView {
		return learnView{scroll: ssdScroll, opening: true, waitUntil: t0.Add(at + recipeReplyTimeout)}
	}
	runRecipeCraftSteps(t, b, []recipeCraftStep{
		{name: "errand opens the common page", wantRule: "recipe", wantBook: botwire.EncodeRequestRecipeBookOpen(false), wantLearn: opening(0)},
		{name: "craft check due: no open while the errand waits", at: time.Second, wantLearn: opening(0)},
		{
			name: "errand's page: errand opens the dwarven page", at: 2 * time.Second,
			events:   []Event{RecipeBookItemList{MaxMP: 100, Recipes: []int32{}}},
			wantRule: "recipe", wantBook: botwire.EncodeRequestRecipeBookOpen(true), wantLearn: opening(2 * time.Second),
		},
		{name: "craft check still due: no open", at: 3 * time.Second, wantLearn: opening(2 * time.Second)},
		{
			name: "dwarven page: errand uses the scroll", at: 4 * time.Second,
			events:    []Event{RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{}}},
			wantRule:  "recipe",
			wantLearn: learnView{scroll: ssdScroll, used: true, waitUntil: at(4*time.Second + recipeReplyTimeout)},
		},
	})
}

// TestBrainRecipeCraftRowKarmaMidAttempt: karma gained while the errand
// waits for its page skips the recipe row (a karma row), so nothing ends
// the attempt; the craft check holds only until the errand's open times
// out, then opens its page.
func TestBrainRecipeCraftRowKarmaMidAttempt(t *testing.T) {
	script := &learnScript{scrolls: []int32{ssdScroll}}
	b := newRecipeCrafterBrain(t, script)
	opening := learnView{scroll: ssdScroll, opening: true, waitUntil: t0.Add(recipeReplyTimeout)}
	runRecipeCraftSteps(t, b, []recipeCraftStep{
		{name: "errand opens the common page", wantRule: "recipe", wantBook: botwire.EncodeRequestRecipeBookOpen(false), wantLearn: opening},
		{
			name: "karma: craft check waits for the errand's open", at: time.Second,
			setup:     func(b *Brain) { b.world.Self.Karma = 100 },
			wantLearn: opening,
		},
		{name: "still within the errand's timeout", at: recipeReplyTimeout - time.Millisecond, wantLearn: opening},
		{
			name: "errand's open timed out: craft row opens its page", at: recipeReplyTimeout,
			wantRule: "craft", wantBook: botwire.EncodeRequestRecipeBookOpen(true), wantLearn: opening,
		},
	})
}

// TestBrainRecipeCraftRowOpenBeforeAttempt: a craft row open in flight
// when an attempt starts holds the errand's open until it is answered or
// timed out, so its answer, the page or the refusal while casting, does
// not end the attempt or stand for the errand's page.
func TestBrainRecipeCraftRowOpenBeforeAttempt(t *testing.T) {
	answers := []struct {
		name string
		ev   Event
	}{
		{"page", RecipeBookItemList{Dwarven: true, MaxMP: 100, Recipes: []int32{}}},
		{"refused while casting", SystemMessage{ID: msgNoRecipeBookWhileCasting}},
	}
	for _, ans := range answers {
		t.Run(ans.name, func(t *testing.T) {
			script := &learnScript{}
			b := newRecipeCrafterBrain(t, script)
			waiting := learnView{scroll: ssdScroll}
			runRecipeCraftSteps(t, b, []recipeCraftStep{
				{name: "craft row opens the dwarven page", wantRule: "craft", wantBook: botwire.EncodeRequestRecipeBookOpen(true)},
				{
					name: "attempt starts: the errand waits for the craft row's open", at: time.Second,
					setup:     func(*Brain) { script.scrolls = []int32{ssdScroll} },
					wantLearn: waiting,
				},
				{name: "craft row's answer ends nothing", at: 2 * time.Second, events: []Event{ans.ev}, wantLearn: waiting},
				{name: "still within the craft row's timeout", at: recipeReplyTimeout - time.Millisecond, wantLearn: waiting},
				{
					name: "errand opens the common page", at: recipeReplyTimeout,
					wantRule: "recipe", wantBook: botwire.EncodeRequestRecipeBookOpen(false),
					wantLearn: learnView{scroll: ssdScroll, opening: true, waitUntil: at(2 * recipeReplyTimeout)},
				},
			})
		})
	}
}

// TestRecipeLearnSentOwnOpen: the attempt's hook takes the errand's own
// book open only; another row's open changes nothing, and the scroll's use
// is told by its template.
func TestRecipeLearnSentOwnOpen(t *testing.T) {
	waiting := recipeLearn{scroll: ssdScroll, opening: true, waitUntil: t0.Add(recipeReplyTimeout)}
	now := t0.Add(time.Second)
	tests := []struct {
		name  string
		start recipeLearn
		a     Action
		want  learnView
	}{
		{"craft row's open while waiting", waiting, recipeBookAction(true), viewLearn(&waiting)},
		{"craft row's open between steps", recipeLearn{scroll: ssdScroll}, recipeBookAction(false), learnView{scroll: ssdScroll}},
		{
			"errand's own open",
			recipeLearn{scroll: ssdScroll},
			learnBookAction(false),
			learnView{scroll: ssdScroll, opening: true, waitUntil: now.Add(recipeReplyTimeout)},
		},
		{"errand's open with no attempt", recipeLearn{}, learnBookAction(true), learnView{}},
		{"another item's use", recipeLearn{scroll: ssdScroll}, Action{kind: actUseItem, item: ssdScroll + 1}, learnView{scroll: ssdScroll}},
		{
			"the scroll's use",
			recipeLearn{scroll: ssdScroll},
			useItemAction(ssdScrollItem),
			learnView{scroll: ssdScroll, used: true, waitUntil: now.Add(recipeReplyTimeout)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.start
			e.sent(tt.a, &RecipeBook{}, now)
			if got := viewLearn(&e); got != tt.want {
				t.Fatalf("attempt = %+v, want %+v", got, tt.want)
			}
		})
	}
}
