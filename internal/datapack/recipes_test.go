package datapack

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRecipesSample(t *testing.T) {
	got, err := LoadRecipes(filepath.Join("testdata", "xml", "recipes.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32]Recipe{
		1: {
			ID: 1, ScrollID: 1666, Level: 1, MPCost: 30, SuccessRate: 100,
			Materials: []Ingredient{{1864, 4}, {1869, 2}}, Product: Ingredient{17, 500}, Dwarven: true, Alias: "mk_wooden_arrow",
		},
		20: {
			ID: 20, ScrollID: 1804, Level: 2, MPCost: 40, SuccessRate: 100,
			Materials: []Ingredient{{1785, 3}, {1458, 1}}, Product: Ingredient{1463, 156}, Dwarven: true, Alias: "mk_soulshot_d",
		},
		300: {
			ID: 300, ScrollID: 2141, Level: 1, MPCost: 30, SuccessRate: 100,
			Materials: []Ingredient{{1869, 5}, {1870, 5}}, Product: Ingredient{1880, 1}, Alias: "mk_steel",
		},
		// 301 needs 300's product and 300's scroll: a chain, no cycle. The
		// flag reads true in any case, as on the server.
		301: {
			ID: 301, ScrollID: 2142, Level: 2, MPCost: 40, SuccessRate: 70,
			Materials: []Ingredient{{1880, 2}, {2141, 1}}, Product: Ingredient{1888, 1}, Dwarven: true, Alias: "mk_steel_mold",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadRecipes = %+v, want %+v", got, want)
	}
	dp, err := Load(filepath.Join("testdata", "xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dp.Recipes, want) {
		t.Fatalf("Load Recipes = %+v, want %+v", dp.Recipes, want)
	}
}

// recipeXML is one <recipe> of id making product from materials.
func recipeXML(id, materials, product string) string {
	return `<recipe alias="a" id="` + id + `" material="` + materials + `" product="` + product +
		`" itemId="9` + id + `" level="1" mpConsume="1" successRate="100" isDwarven="true"/>`
}

func TestLoadRecipesErrors(t *testing.T) {
	const all = `id="1" material="2-1" product="3-1" itemId="4" level="1" mpConsume="1" successRate="100" isDwarven="true" alias="a"`
	drop := func(attr string) string {
		i := strings.Index(all, attr+`="`)
		j := strings.Index(all[i+len(attr)+2:], `"`)
		return "<list><recipe " + all[:i] + all[i+len(attr)+2+j+1:] + "/></list>"
	}
	tests := []struct {
		name, body, wantErr string
	}{
		{"malformed", "<list><recipe", "decode"},
		{"no id", drop("id"), `"id"`},
		{"no material", drop("material"), `"material"`},
		{"no product", drop("product"), `"product"`},
		{"no itemId", drop("itemId"), `"itemId"`},
		{"no level", drop("level"), `"level"`},
		{"no mpConsume", drop("mpConsume"), `"mpConsume"`},
		{"no successRate", drop("successRate"), `"successRate"`},
		{"no isDwarven", drop("isDwarven"), `"isDwarven"`},
		{"no alias", drop("alias"), `"alias"`},
		{"material without count", "<list>" + recipeXML("1", "2", "3-1") + "</list>", `"material"`},
		{"bad material count", "<list>" + recipeXML("1", "2-1;5-x", "3-1") + "</list>", `"material"`},
		{"empty material", "<list>" + recipeXML("1", "", "3-1") + "</list>", `"material"`},
		{"bad product", "<list>" + recipeXML("1", "2-1", "3") + "</list>", `"product"`},
		{"product past int32", "<list>" + recipeXML("1", "2-1", "3-2147483648") + "</list>", `"product"`},
		{"duplicate id", "<list>" + recipeXML("1", "2-1", "3-1") + recipeXML("1", "2-1", "5-1") + "</list>", "duplicate"},
		{"self cycle", "<list>" + recipeXML("7", "2-1;3-1", "3-1") + "</list>", "recipes 7 -> 7"},
		{
			"cycle through others",
			"<list>" + recipeXML("1", "2-1", "3-1") + recipeXML("2", "3-1", "4-1") + recipeXML("3", "4-1", "2-1") + "</list>",
			"recipes 1 -> 3 -> 2 -> 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recipes.xml")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadRecipes(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadRecipes error = %v, want it to contain %s", err, tt.wantErr)
			}
		})
	}
	if _, err := LoadRecipes(filepath.Join(t.TempDir(), "recipes.xml")); err == nil {
		t.Fatal("LoadRecipes of a missing file: no error")
	}
}

func TestRecipeCycle(t *testing.T) {
	r := func(id, product int32, materials ...int32) Recipe {
		in := make([]Ingredient, len(materials))
		for i, m := range materials {
			in[i] = Ingredient{m, 1}
		}
		return Recipe{ID: id, Product: Ingredient{product, 1}, Materials: in}
	}
	tests := []struct {
		name    string
		recipes []Recipe
		want    string
	}{
		{"none", nil, ""},
		{"chain", []Recipe{r(1, 10, 11), r(2, 11, 12), r(3, 12, 99)}, ""},
		// Two variants make item 10; one of them needs 10 back.
		{"variant loop", []Recipe{r(1, 10, 11), r(2, 10, 12), r(3, 12, 10)}, "recipes 2 -> 3 -> 2"},
		// A diamond shares a dependency: visited twice, no cycle.
		{"diamond", []Recipe{r(1, 10, 11, 12), r(2, 11, 13), r(3, 12, 13), r(4, 13, 99)}, ""},
		{"long", []Recipe{r(1, 10, 11), r(2, 11, 12), r(3, 12, 13), r(4, 13, 10)}, "recipes 1 -> 2 -> 3 -> 4 -> 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := make(map[int32]Recipe, len(tt.recipes))
			for _, x := range tt.recipes {
				m[x.ID] = x
			}
			err := recipeCycle(m)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("recipeCycle = %v, want none", err)
				}
				return
			}
			if !errors.Is(err, errRecipeCycle) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("recipeCycle = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestLoadShippedRecipes checks the reader on the shipped recipes.xml when
// BOT_DATAPACK_XML names a real data/xml directory (the server checkout's
// aCis_datapack/data/xml), and skips otherwise: the datapack is not part of
// this repository. Counts that drift fail the test, so a datapack change
// shows here before a craft planner meets it.
func TestLoadShippedRecipes(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	recipes, err := LoadRecipes(filepath.Join(dir, "recipes.xml"))
	if err != nil {
		t.Fatal(err)
	}
	yields, ownScroll := 0, 0
	makers := map[int32]int{}
	for _, r := range recipes {
		if r.Product.Count > 1 {
			yields++
		}
		makers[r.Product.ItemID]++
		for _, m := range r.Materials {
			if m.ItemID == r.ScrollID {
				ownScroll++
				break
			}
		}
	}
	variants := 0
	for _, n := range makers {
		if n > 1 {
			variants++
		}
	}
	got := [4]int{len(recipes), yields, variants, ownScroll}
	if want := [4]int{871, 42, 178, 448}; got != want {
		t.Fatalf("recipes, yields above one, products with variants, own scroll as material = %v, want %v", got, want)
	}
}
