package datapack

import (
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Recipe is one craft recipe (recipes.xml <recipe>): what a crafter with
// the recipe in its book spends to make Product.
type Recipe struct {
	ID int32
	// ScrollID is the template of the recipe item whose use writes the
	// recipe into the book (itemId).
	ScrollID int32
	// Level is the Create Item (Dwarven or Common Craft) level the recipe
	// needs.
	Level int32
	// MPCost is the MP a craft spends (mpConsume).
	MPCost int32
	// SuccessRate is the chance of a craft in percent.
	SuccessRate int32
	// Materials are consumed whole by every craft, failed ones included.
	Materials []Ingredient
	// Product is what a successful craft makes: the item and the yield.
	Product Ingredient
	// Dwarven is set for a dwarven recipe, clear for a common one.
	Dwarven bool
	Alias   string
}

// Ingredient is an item template and a count.
type Ingredient struct{ ItemID, Count int32 }

type recipesFile struct {
	Recipes []attrsElement `xml:"recipe"`
}

// LoadRecipes reads recipes.xml at path: every recipe by id. Every
// attribute is required, materials are "item-count;item-count" and the
// product "item-count", as on the server (data/xml/economy.go LoadRecipes,
// model/recipe ParseIngredients). A duplicate id, or a cycle through the
// recipes (a recipe needing, through the products of others, its own
// product), fails the load: the server takes both, a craft planner cannot.
func LoadRecipes(path string) (map[int32]Recipe, error) {
	var f recipesFile
	if err := decodeFile(path, &f); err != nil {
		return nil, err
	}
	out := make(map[int32]Recipe, len(f.Recipes))
	for _, e := range f.Recipes {
		r, err := buildRecipe(e.Attrs)
		if err != nil {
			return nil, fmt.Errorf("datapack: %s: %w", path, err)
		}
		if _, dup := out[r.ID]; dup {
			return nil, fmt.Errorf("datapack: %s: recipe %d: duplicate id", path, r.ID)
		}
		out[r.ID] = r
	}
	if err := recipeCycle(out); err != nil {
		return nil, fmt.Errorf("datapack: %s: %w", path, err)
	}
	return out, nil
}

// buildRecipe builds one <recipe> from its attributes.
func buildRecipe(attrs []xml.Attr) (Recipe, error) {
	a := foldAttrs(attrs)
	id := a.int32("id")
	if a.err != nil {
		return Recipe{}, fmt.Errorf("recipe: %w", a.err)
	}
	r := Recipe{
		ID:          id,
		ScrollID:    a.int32("itemId"),
		Level:       a.int32("level"),
		MPCost:      a.int32("mpConsume"),
		SuccessRate: a.int32("successRate"),
		Dwarven:     strings.EqualFold(a.str("isDwarven"), "true"),
		Alias:       a.str("alias"),
	}
	if raw := a.str("material"); a.err == nil {
		for part := range strings.SplitSeq(raw, ";") {
			in, err := parseIngredient(part)
			if err != nil {
				a.fail("material", err)
				break
			}
			r.Materials = append(r.Materials, in)
		}
	}
	if raw := a.str("product"); a.err == nil {
		in, err := parseIngredient(raw)
		if err != nil {
			a.fail("product", err)
		}
		r.Product = in
	}
	if a.err != nil {
		return Recipe{}, fmt.Errorf("recipe %d: %w", id, a.err)
	}
	return r, nil
}

// parseIngredient parses one "item-count" pair.
func parseIngredient(raw string) (Ingredient, error) {
	item, count, ok := strings.Cut(raw, "-")
	if !ok {
		return Ingredient{}, fmt.Errorf("%q: want item-count", raw)
	}
	id, err := strconv.ParseInt(item, 10, 32)
	if err != nil {
		return Ingredient{}, err
	}
	n, err := strconv.ParseInt(count, 10, 32)
	if err != nil {
		return Ingredient{}, err
	}
	return Ingredient{ItemID: int32(id), Count: int32(n)}, nil
}

// errRecipeCycle reports recipes that need, through the products of
// others, their own product.
var errRecipeCycle = errors.New("recipe cycle")

// recipeCycle reports the first cycle over recipes, naming the recipe ids
// on it in order. A recipe depends on every recipe whose product is one of
// its materials; a material no recipe makes (a recipe scroll, a crafting
// material bought or dropped) ends the path.
func recipeCycle(recipes map[int32]Recipe) error {
	makers := make(map[int32][]int32) // product item -> recipe ids
	ids := make([]int32, 0, len(recipes))
	for id, r := range recipes {
		makers[r.Product.ItemID] = append(makers[r.Product.ItemID], id)
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, m := range makers {
		slices.Sort(m)
	}
	const (
		onPath = 1 + iota
		done
	)
	state := make(map[int32]uint8, len(recipes))
	var path []int32
	var visit func(id int32) []int32
	visit = func(id int32) []int32 {
		switch state[id] {
		case onPath:
			return append(slices.Clone(path[slices.Index(path, id):]), id)
		case done:
			return nil
		}
		state[id] = onPath
		path = append(path, id)
		for _, m := range recipes[id].Materials {
			for _, dep := range makers[m.ItemID] {
				if c := visit(dep); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[id] = done
		return nil
	}
	for _, id := range ids {
		if c := visit(id); c != nil {
			names := make([]string, len(c))
			for i, r := range c {
				names[i] = strconv.Itoa(int(r))
			}
			return fmt.Errorf("%w: recipes %s", errRecipeCycle, strings.Join(names, " -> "))
		}
	}
	return nil
}
