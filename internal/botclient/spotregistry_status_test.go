package botclient

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

func TestSpotRegistryStatuses(t *testing.T) {
	var none *SpotRegistry
	if got := none.Statuses(); got != nil {
		t.Fatalf("nil registry Statuses = %v, want nil", got)
	}
	a := testSpot("a", 0, 0, 1, 3, 6)
	a.AggroLevel = 2
	r := NewSpotRegistry([]datapack.Spot{a, testSpot("b", 0, 0, 10, 14, 3)}, 1)
	if _, ok := r.claim([]int{0}); !ok {
		t.Fatal("claim a refused")
	}
	want := []SpotStatus{
		{Name: "a", MinLevel: 1, MaxLevel: 3, AggroLevel: 2, Cap: 2, Held: 1},
		{Name: "b", MinLevel: 10, MaxLevel: 14, Cap: 1},
	}
	if got := r.Statuses(); !slices.Equal(got, want) {
		t.Fatalf("Statuses = %+v, want %+v", got, want)
	}
}
