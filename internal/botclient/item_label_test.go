package botclient

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestItemLabel checks how an ad names a valuable (docs/valuables/ads.md
// "Lines"): every priced template's name in itemNames, a recipe scroll's
// among them; "2x" for several units, "+5" enchanted, "(D)" for graded
// gear; the price in k and kk rounded up to the unit shown, never below the
// ask.
func TestItemLabel(t *testing.T) {
	const (
		halberd   int32 = 5458 // Recipe: Halberd(60%), an etc item the reader does not keep
		staff     int32 = 184  // Conjuror's Staff, D grade
		sword     int32 = 121  // Sword of Watershadow, no grade
		unnamed   int32 = 7777
		product   int32 = 7778 // an unpriced recipe product
		productOf int32 = 9500
	)
	m := scoreMap(t, func(dp *datapack.Datapack) {
		dp.Items = map[int32]datapack.Item{
			staff: {ID: staff, Type: "Weapon", Name: "Conjuror's Staff", Grade: 1, CrystalCount: 1000, BodyPart: "lrhand"},
			sword: {ID: sword, Type: "Weapon", Name: "Sword of Watershadow", BodyPart: "rhand"},
		}
		dp.EtcItems = map[int32]datapack.EtcItem{product: {ID: product, Name: "Steel"}}
		dp.Recipes = map[int32]datapack.Recipe{productOf: {ID: productOf, Product: datapack.Ingredient{ItemID: product, Count: 1}}}
		dp.Prices[halberd], dp.Prices[staff], dp.Prices[sword], dp.Prices[unnamed] = 366000, 644000, 136000, 50000
		dp.Names = map[int32]string{halberd: "Recipe: Halberd(60%)", staff: "Conjuror's Staff", sword: "Sword of Watershadow", scoreLoot: "Loot"}
	})
	for id, name := range map[int32]string{halberd: "Recipe: Halberd(60%)", staff: "Conjuror's Staff", sword: "Sword of Watershadow", scoreLoot: "Loot", product: "Steel"} {
		if got := m.itemNames[id]; got != name {
			t.Errorf("itemNames[%d] = %q, want %q", id, got, name)
		}
	}
	tests := []struct {
		name  string
		key   valuableKey
		units int64
		price int64
		want  string
	}{
		{"recipe", valuableKey{template: halberd}, 1, 366000, "Recipe: Halberd(60%) 366k"},
		{"no grade", valuableKey{template: sword}, 1, 173000, "Sword of Watershadow 173k"},
		{"two units, price each", valuableKey{template: sword}, 2, 173000, "2x Sword of Watershadow 173k"},
		{"enchanted, graded", valuableKey{template: staff, enchant: 5}, 1, 894000, "+5 Conjuror's Staff (D) 894k"},
		{"graded", valuableKey{template: staff}, 1, 966000, "Conjuror's Staff (D) 966k"},
		{"k rounded up", valuableKey{template: sword}, 1, 172001, "Sword of Watershadow 173k"},
		{"kk to a tenth", valuableKey{template: staff}, 1, 1200000, "Conjuror's Staff (D) 1.2kk"},
		{"kk rounded up", valuableKey{template: staff}, 1, 1200001, "Conjuror's Staff (D) 1.3kk"},
		{"whole kk", valuableKey{template: staff}, 1, 1988000, "Conjuror's Staff (D) 2kk"},
		{"k rounding into kk", valuableKey{template: sword}, 1, 999001, "Sword of Watershadow 1kk"},
		{"no name", valuableKey{template: unnamed}, 1, 50000, "item 7777 50k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.itemLabel(tt.key, tt.units, tt.price); got != tt.want {
				t.Errorf("itemLabel = %q, want %q", got, tt.want)
			}
		})
	}
	for _, price := range []int64{1, 999, 1000, 1001, 10000, 99999, 172946, 999999, 1000000, 1000001, 1049999, 1988000, 25000001} {
		if shown := parsePriceText(t, priceText(price)); shown < price || shown-price >= priceUnit(price) {
			t.Errorf("priceText(%d) = %q (%d): want at least the ask, under one shown unit above", price, priceText(price), shown)
		}
	}
}

// parsePriceText reads a priceText back as players would.
func parsePriceText(t *testing.T, s string) int64 {
	t.Helper()
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "kk"):
		s, mult = strings.TrimSuffix(s, "kk"), 1e6
	case strings.HasSuffix(s, "k"):
		s, mult = strings.TrimSuffix(s, "k"), 1e3
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("price %q: %v", s, err)
	}
	return int64(f*mult + 0.5)
}

// priceUnit is the unit priceText shows price in.
func priceUnit(price int64) int64 {
	switch {
	case price < 1000:
		return 1
	case price <= 999000:
		return 1000
	default:
		return 100000
	}
}
