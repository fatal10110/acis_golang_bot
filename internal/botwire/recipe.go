package botwire

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Pages of the recipe book on the wire: RequestRecipeBookOpen,
// RecipeBookItemList and RecipeItemMakeInfo name the dwarven page 0 and the
// common page 1 (clientpackets/recipe.go, serverpackets/recipe.go).
const (
	RecipePageDwarven int32 = 0
	RecipePageCommon  int32 = 1
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

func (RecipeBookItemList) IsEvent() {}

func (RecipeItemMakeInfo) IsEvent() {}

// decodeRecipeBookItemList: page, max MP, count, then per recipe its id
// and its 1-based position (serverpackets/recipe.go
// FrameRecipeBookItemList). The position is the list order and is dropped.
// The loop ends at the first short read.
func decodeRecipeBookItemList(r *wire.Reader) Event {
	e := RecipeBookItemList{Dwarven: r.ReadInt32() == RecipePageDwarven, MaxMP: r.ReadInt32()}
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
		RecipeID: r.ReadInt32(), Dwarven: r.ReadInt32() == RecipePageDwarven,
		MP: r.ReadInt32(), MaxMP: r.ReadInt32(), Status: r.ReadInt32(),
	}
}
