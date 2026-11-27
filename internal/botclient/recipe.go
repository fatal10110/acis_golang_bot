package botclient

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Recipe learning limits.
const (
	// recipeReplyTimeout is how long a recipe book open or a recipe scroll
	// use waits for its answer. The server answers a scroll whose recipe it
	// does not load, and a UseItem it refuses before the recipe handler, in
	// silence (craft/craft.go Register; network/inventory.go).
	recipeReplyTimeout = 5 * time.Second
	// learnRetry is the least time from an attempt that did not add a
	// recipe to the next one, so a refused or unanswered scroll is not used
	// again at the tick rate.
	learnRetry = 10 * time.Second
)

// System messages of crafting and recipe learning
// (serverpackets/systemmessage.go; network/recipe.go sendCraftNotices).
// NotEnoughMP is msgNotEnoughMP.
const (
	msgEarnedS2S1S                  = 53
	msgEarnedItemS1                 = 54
	msgS2S1Disappeared              = 301
	msgS1Disappeared                = 302
	msgCreateLvlTooLowToRegister    = 404
	msgItemMixingFailed             = 719
	msgRecipeAlreadyRegistered      = 840
	msgS1Added                      = 851
	msgCantAlterRecipeBook          = 853
	msgMissingS2S1ToCreate          = 854
	msgUpToS1RecipesCanRegister     = 894
	msgCantRegisterNoAbilityToCraft = 1061
	msgNoRecipeBookWhileCasting     = 1124
	msgCantOperateStoreDuringCombat = 1135
)

// learnRefusals are the system messages that refuse a recipe scroll, in
// the server's order (craft/craft.go Register): the recipe is in the book,
// no craft ability for its page, a manufacture store is open, the Create
// Item level is below the recipe's, the page is full. No book page follows
// any of them.
var learnRefusals = []int32{
	msgRecipeAlreadyRegistered, msgCantRegisterNoAbilityToCraft, msgCantAlterRecipeBook,
	msgCreateLvlTooLowToRegister, msgUpToS1RecipesCanRegister,
}

// Pages of the recipe book on the wire: RequestRecipeBookOpen,
// RecipeBookItemList and RecipeItemMakeInfo name the dwarven page 0 and the
// common page 1 (clientpackets/recipe.go, serverpackets/recipe.go).
const (
	recipePageDwarven int32 = 0
	recipePageCommon  int32 = 1
)

// RecipeBookItemList is one page of the bot's recipe book: dwarven or
// common, the bot's max MP, and the recipe ids in book order.
type RecipeBookItemList struct {
	Dwarven bool
	MaxMP   int32
	Recipes []int32
}

// Outcomes of a craft a RecipeItemMakeInfo reports
// (serverpackets/recipe.go RecipeMakeInfo*).
const (
	RecipeMadeNone    int32 = -1
	RecipeMadeFailed  int32 = 0
	RecipeMadeSuccess int32 = 1
)

// RecipeItemMakeInfo is the craft window of RecipeID after a craft: its
// page, the bot's MP and max MP, and Status, the craft's outcome
// (RecipeMade*). A failed craft is told apart by the system message ahead
// of it (craftFailure).
type RecipeItemMakeInfo struct {
	RecipeID  int32
	Dwarven   bool
	MP, MaxMP int32
	Status    int32
}

func (RecipeBookItemList) event() {}
func (RecipeItemMakeInfo) event() {}

// decodeRecipeBookItemList: page, max MP, count, then per recipe its id
// and its 1-based position (serverpackets/recipe.go
// FrameRecipeBookItemList). The position is the list order and is dropped.
// The loop ends at the first short read.
func decodeRecipeBookItemList(r *wire.Reader) Event {
	e := RecipeBookItemList{Dwarven: r.ReadInt32() == recipePageDwarven, MaxMP: r.ReadInt32()}
	n := int(r.ReadInt32())
	e.Recipes = make([]int32, 0, capFor(r, n, 8))
	for ; n > 0 && r.Err() == nil; n-- {
		id := r.ReadInt32()
		r.ReadInt32()
		e.Recipes = append(e.Recipes, id)
	}
	return e
}

// decodeRecipeItemMakeInfo: recipe id, page, MP, max MP, status
// (serverpackets/recipe.go FrameRecipeItemMakeInfo).
func decodeRecipeItemMakeInfo(r *wire.Reader) Event {
	return RecipeItemMakeInfo{
		RecipeID: r.ReadInt32(), Dwarven: r.ReadInt32() == recipePageDwarven,
		MP: r.ReadInt32(), MaxMP: r.ReadInt32(), Status: r.ReadInt32(),
	}
}

// encodeRequestRecipeBookOpen asks for the dwarven or the common page of
// the recipe book (clientpackets/recipe.go: any page but 0 is common).
func encodeRequestRecipeBookOpen(dwarven bool) []byte {
	w := wire.NewPacketWriter(opGameRequestRecipeBookOpen)
	page := recipePageCommon
	if dwarven {
		page = recipePageDwarven
	}
	w.WriteInt32(page)
	return w.Bytes()
}

// encodeRequestRecipeItemMakeSelf crafts recipeID for the bot itself.
func encodeRequestRecipeItemMakeSelf(recipeID int32) []byte {
	w := wire.NewPacketWriter(opGameRequestRecipeItemMakeSelf)
	w.WriteInt32(recipeID)
	return w.Bytes()
}

// recipeBookAction opens the dwarven or the common page of the book.
func recipeBookAction(dwarven bool) Action {
	return Action{kind: actRecipeBook, payload: encodeRequestRecipeBookOpen(dwarven)}
}

// learnBookAction is the recipe errand's own open of a page (ruleRecipe),
// the one its attempt waits on (recipeLearn.sent).
func learnBookAction(dwarven bool) Action {
	a := recipeBookAction(dwarven)
	a.learn = true
	return a
}

// craftAction crafts recipeID for the bot. It passes the craft gate
// (gateCraft, craftGap).
func craftAction(recipeID int32) Action {
	return Action{kind: actCraft, payload: encodeRequestRecipeItemMakeSelf(recipeID), item: recipeID}
}

// CraftFailure is why a craft failed: the system message the server sends
// ahead of RecipeItemMakeInfo status 0, or alone.
type CraftFailure uint8

// Craft failures (craft/craft.go MakeSelf; network/recipe.go
// sendCraftNotices).
const (
	// CraftFailNone: the message tells no craft failure.
	CraftFailNone CraftFailure = iota
	// CraftMixingFailed: the roll failed; the materials and MP are spent.
	CraftMixingFailed
	// CraftMissingMaterial: a material is short (one message per short
	// material); nothing is spent.
	CraftMissingMaterial
	// CraftNotEnoughMP: MP is below the recipe's cost; nothing is spent.
	CraftNotEnoughMP
	// CraftInCombat: the bot is in combat or a duel. No RecipeItemMakeInfo
	// follows this one.
	CraftInCombat
)

// craftFailure maps a system message to the craft failure it tells.
func craftFailure(msg int32) CraftFailure {
	switch msg {
	case msgItemMixingFailed:
		return CraftMixingFailed
	case msgMissingS2S1ToCreate:
		return CraftMissingMaterial
	case msgNotEnoughMP:
		return CraftNotEnoughMP
	case msgCantOperateStoreDuringCombat:
		return CraftInCombat
	}
	return CraftFailNone
}

// RecipeBook is the bot's recipe book as the last RecipeBookItemList of
// each page showed it. The server sends a page only when asked
// (RequestRecipeBookOpen) and after a recipe is learned, so a page is
// unknown until then.
type RecipeBook struct {
	Dwarven, Common           []int32
	DwarvenKnown, CommonKnown bool
	// MaxMP is the max MP the last page showed.
	MaxMP int32
}

// Has reports whether either page holds recipeID.
func (b *RecipeBook) Has(recipeID int32) bool {
	return slices.Contains(b.Dwarven, recipeID) || slices.Contains(b.Common, recipeID)
}

// apply takes page e.
func (b *RecipeBook) apply(e RecipeBookItemList) {
	b.MaxMP = e.MaxMP
	if e.Dwarven {
		b.Dwarven, b.DwarvenKnown = slices.Clone(e.Recipes), true
	} else {
		b.Common, b.CommonKnown = slices.Clone(e.Recipes), true
	}
}

// LearnOutcome is how a recipe learning attempt ended.
type LearnOutcome uint8

// Outcomes of a recipe learning attempt.
const (
	// LearnNone: none ended yet.
	LearnNone LearnOutcome = iota
	// LearnDone: the server used the scroll (S1Added) and sent the page.
	// LearnResult.Added is the new recipe, 0 when the page did not change
	// (a subclass: the scroll is spent and the book stays the base
	// class's).
	LearnDone
	// LearnRefused: the server refused the scroll; LearnResult.Msg is the
	// system message (learnRefusals), or a book open refused while casting.
	LearnRefused
	// LearnTimeout: the server answered nothing (a silent refusal).
	LearnTimeout
	// LearnNoScroll: the bot does not carry the scroll.
	LearnNoScroll
)

// LearnResult is the end of the last recipe learning attempt: the scroll
// template it used, its outcome, the refusal's system message, the recipe
// it added, and when.
type LearnResult struct {
	Scroll  int32
	Outcome LearnOutcome
	Msg     int32
	Added   int32
	At      time.Time
}

// RecipePolicy is recipe learning (rule 9 errand "recipe"). Learn is asked
// when no attempt runs, the bot is out of combat and learnRetry passed
// since an attempt that added nothing; it names a recipe scroll template
// the bot carries, 0 for none, and nil never learns. The server is the
// authority on the craft ability, the Create Item level and the book's
// room: a refusal ends the attempt.
type RecipePolicy struct {
	Learn func(w *World, last LearnResult, now time.Time) int32
}

// recipeLearn is the recipe learning errand (rule 9 errand "recipe"). It
// is owned by the brain goroutine. An attempt opens each page of the book
// the bot does not know yet, then uses the scroll once.
type recipeLearn struct {
	// scroll is the template the attempt under way uses, 0 for none.
	scroll int32
	// opening is set from a book open's send to its page; used from the
	// scroll's use to the attempt's end; added once S1Added came.
	opening, used, added bool
	// waitUntil is when the open or the use under way times out.
	waitUntil time.Time
	// before is the book when the scroll went out.
	before RecipeBook
	last   LearnResult
	// retryAt is when an attempt may start after one that added nothing.
	retryAt time.Time
}

// end ends the attempt under way with outcome o.
func (e *recipeLearn) end(o LearnOutcome, msg, added int32, now time.Time) {
	e.last = LearnResult{Scroll: e.scroll, Outcome: o, Msg: msg, Added: added, At: now}
	if o != LearnDone {
		e.retryAt = now.Add(learnRetry)
	}
	*e = recipeLearn{last: e.last, retryAt: e.retryAt}
}

// sent records a packet of the attempt that went out at now; book is the
// recipe book then. A book open the errand did not send (the craft row's
// openBook) is not the attempt's (#652).
func (e *recipeLearn) sent(a Action, book *RecipeBook, now time.Time) {
	if e.scroll == 0 {
		return
	}
	switch {
	case a.kind == actRecipeBook && a.learn:
		e.opening, e.waitUntil = true, now.Add(recipeReplyTimeout)
	case a.kind == actUseItem && a.item == e.scroll && !e.used:
		e.used, e.waitUntil = true, now.Add(recipeReplyTimeout)
		e.before = RecipeBook{Dwarven: slices.Clone(book.Dwarven), Common: slices.Clone(book.Common)}
	}
}

// ruleRecipe is rule 9 errand "recipe": it learns the recipe scrolls
// RecipePolicy.Learn names. One frame goes out per answer: a book open waits
// for its page and the scroll's use for its refusal or page, each at most
// recipeReplyTimeout, and an attempt that adds nothing holds the next for
// learnRetry. RequestRecipeBookOpen and UseItem have no server reuse gate
// (network/client_dispatch.go): the frame pacer bounds them on top.
//
// The server answers every open with the same page or refusal, so an
// answer tells not whose open it was: the errand opens no page while the
// craft row's open waits for its answer (crafterCycle.bookPending), and
// the craft row opens none during an attempt (ruleCraft).
func (b *Brain) ruleRecipe(now time.Time) (Action, bool) {
	e := &b.learn
	if e.scroll == 0 && !b.startLearn(now) {
		return Action{}, false
	}
	if e.opening || e.used {
		if now.Before(e.waitUntil) {
			return Action{}, false
		}
		e.end(LearnTimeout, 0, 0, now)
		return Action{}, false
	}
	// The scroll's answer is told by its page changing: know both pages
	// first.
	book := &b.world.Recipes
	if !(book.CommonKnown && book.DwarvenKnown) && b.craft.bookPending(now) {
		return Action{}, false
	}
	if !book.CommonKnown {
		return learnBookAction(false), true
	}
	if !book.DwarvenKnown {
		return learnBookAction(true), true
	}
	it, ok := b.world.Inventory.ByTemplate(e.scroll)
	if !ok {
		e.end(LearnNoScroll, 0, 0, now)
		return Action{}, false
	}
	return useItemAction(it), true
}

// startLearn starts the attempt the crafter's plan (craftLearn) or Learn
// asks for, if one is due.
func (b *Brain) startLearn(now time.Time) bool {
	e, p := &b.learn, &b.persona.Recipes
	if b.world.Self.InCombat || now.Before(e.retryAt) {
		return false
	}
	scroll := b.craftLearn()
	if scroll == 0 && p.Learn != nil {
		scroll = p.Learn(b.world, e.last, now)
	}
	if scroll == 0 {
		return false
	}
	e.scroll = scroll
	if _, ok := b.world.Inventory.ByTemplate(scroll); !ok {
		e.end(LearnNoScroll, 0, 0, now)
		return false
	}
	return true
}

// recipeEvent moves the attempt on with the server's answer ev. The world
// took ev already.
func (b *Brain) recipeEvent(ev Event, now time.Time) {
	e := &b.learn
	switch m := ev.(type) {
	case SystemMessage:
		switch {
		case e.used && m.ID == msgS1Added:
			e.added = true
		case e.used && slices.Contains(learnRefusals, m.ID):
			e.end(LearnRefused, m.ID, 0, now)
		case e.opening && m.ID == msgNoRecipeBookWhileCasting:
			e.end(LearnRefused, m.ID, 0, now)
		}
	case RecipeBookItemList:
		switch {
		case e.used && e.added:
			var added int32
			for _, id := range m.Recipes {
				if !e.before.Has(id) {
					added = id
					break
				}
			}
			e.end(LearnDone, 0, added, now)
		case e.opening:
			e.opening, e.waitUntil = false, time.Time{}
		}
	}
}
