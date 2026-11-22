package e2e

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Recipe learning scenario: Recipe: Lesser Healing Potion, a common recipe
// of Create Item level 1 (recipes.xml recipe 686, scroll 6926), learned
// with Common Craft (skill 1322) level 1.
const (
	lhpRecipe   int32 = 686
	lhpScroll   int32 = 6926
	commonCraft int32 = 1322
)

// recipeScript hands the bot's recipe errand one scroll per ask, two in
// all, once the GM armed it and the bot has Common Craft and carries the
// scroll, and publishes every attempt's end. It runs on the brain
// goroutine but for armed and out.
type recipeScript struct {
	armed atomic.Bool
	left  int
	seen  time.Time
	out   chan botclient.LearnResult
}

func (s *recipeScript) learn(w *botclient.World, last botclient.LearnResult, _ time.Time) int32 {
	if last.Outcome != botclient.LearnNone && !last.At.Equal(s.seen) {
		s.seen = last.At
		select {
		case s.out <- last:
		default:
		}
	}
	if !s.armed.Load() || s.left == 0 || w.Skills[commonCraft].Level == 0 {
		return 0
	}
	if _, ok := w.Inventory.ByTemplate(lhpScroll); !ok {
		return 0
	}
	s.left--
	return lhpScroll
}

// TestLiveRecipeLearn learns a recipe by using its scroll: the bot opens
// both pages of its book (common first), uses the scroll and sees S1Added
// and the fresh common page holding the recipe; the second copy is refused as already
// registered, with no page. A character of an earlier run knows the recipe
// already: both copies are refused then.
func TestLiveRecipeLearn(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 4*time.Minute)
	gm := enterGM(t, ctx, tg)
	dir := filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "recipes")
	rec, err := botclient.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	script := &recipeScript{left: 2, out: make(chan botclient.LearnResult, 4)}
	botAcc, _ := tg.Account("rcp", 1)
	b := runBotWith(t, ctx, tg, botAcc, botSetup{tap: rec, persona: func() botclient.Persona {
		p := partyPersona()
		p.Recipes.Learn = script.learn
		return p
	}})
	name := botOnline(t, ctx, tg, b)
	botID := recallBot(t, ctx, tg, gm, name)
	if err := gm.Select(ctx, botID); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{fmt.Sprintf("skill set %d 1", commonCraft), fmt.Sprintf("give %d 2", lhpScroll)} {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	script.armed.Store(true)

	next := func(what string) botclient.LearnResult {
		t.Helper()
		select {
		case r := <-script.out:
			t.Logf("%s: %+v", what, r)
			return r
		case <-ctx.Done():
			t.Fatalf("waiting for the %s: %v", what, ctx.Err())
			return botclient.LearnResult{}
		}
	}
	refused := func(r botclient.LearnResult) bool {
		return r.Scroll == lhpScroll && r.Outcome == botclient.LearnRefused && r.Msg == 840 // RecipeAlreadyRegistered
	}
	first := next("first scroll")
	learned := first.Scroll == lhpScroll && first.Outcome == botclient.LearnDone && first.Added == lhpRecipe
	if !learned && !refused(first) {
		t.Fatalf("first scroll = %+v, want recipe %d added, or refused as known from an earlier run", first, lhpRecipe)
	}
	if !learned {
		t.Logf("%s knows recipe %d from an earlier run", name, lhpRecipe)
	}
	if second := next("second scroll"); !refused(second) {
		t.Fatalf("second scroll = %+v, want RecipeAlreadyRegistered", second)
	}
	// The first page recorded is the common page the errand opened first:
	// it holds the recipe when an earlier run learned it.
	raw, err := os.ReadFile(filepath.Join(dir, "d6.bin"))
	if err != nil {
		t.Fatal(err)
	}
	ev, _, err := botclient.DecodeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	page, ok := ev.(botclient.RecipeBookItemList)
	if !ok || page.Dwarven || page.MaxMP <= 0 || learned == slices.Contains(page.Recipes, lhpRecipe) {
		t.Fatalf("recipe book page = %+v, want the common page, holding recipe %d only if known before", ev, lhpRecipe)
	}
	if err := rec.Err(); err != nil {
		t.Error(err)
	}
}
