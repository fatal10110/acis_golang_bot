package nav

import (
	"math/rand/v2"
	"testing"
)

// TestIndexTableMatchesMap runs getOrPut on random keys against a map, over
// resets and growth: every key answers the value it was first given, and a
// reset forgets them all.
func TestIndexTableMatchesMap(t *testing.T) {
	tests := []struct {
		name  string
		keys  int // per round
		space uint64
	}{
		{"few keys, many repeats", 50, 64},
		{"grows past the first table", 5000, 1 << 40},
		{"dense cells", 20000, 30000},
	}
	rng := rand.New(rand.NewPCG(1, 2))
	var tab indexTable // shared: each round starts from the last one's table
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for range 4 {
				tab.reset()
				ref := map[uint64]int32{}
				for i := range tc.keys {
					k := rng.Uint64N(tc.space)
					want, seen := ref[k]
					if !seen {
						want = int32(i)
						ref[k] = want
					}
					got, ok := tab.getOrPut(k, int32(i))
					if got != want || ok != seen {
						t.Fatalf("getOrPut(%d, %d) = %d, %v; want %d, %v", k, i, got, ok, want, seen)
					}
				}
				if tab.n != len(ref) {
					t.Fatalf("table holds %d keys, want %d", tab.n, len(ref))
				}
			}
		})
	}
}

// TestIndexTableGenerationWrap: a reset that wraps the generation clears the
// table, so no slot of an old generation passes as current.
func TestIndexTableGenerationWrap(t *testing.T) {
	var tab indexTable
	tab.reset()
	tab.getOrPut(7, 1)
	tab.gen = ^uint32(0) // the next reset wraps
	tab.slots[tab.home(7)].gen = 1
	tab.reset()
	if v, ok := tab.getOrPut(7, 2); ok || v != 2 {
		t.Fatalf("after the wrap getOrPut(7, 2) = %d, %v; want 2, false", v, ok)
	}
}

// TestCellKeyDistinct: cells differing in one coordinate, or by sign of the
// floor height, never share a key.
func TestCellKeyDistinct(t *testing.T) {
	cells := []Cell{
		{0, 0, 0},
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
		{0, 0, -1},
		{34815, 32767, 0},
		{34815, 32767, -32768},
		{34815, 32767, 32767},
		{32767, 34815, 0},
	}
	seen := map[uint64]Cell{}
	for _, c := range cells {
		if d, ok := seen[cellKey(c)]; ok {
			t.Fatalf("cells %v and %v share key %x", c, d, cellKey(c))
		}
		seen[cellKey(c)] = c
	}
}
