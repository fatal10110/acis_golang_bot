package botmetrics

import (
	"testing"
	"time"
)

// TestCollectNavCache checks that Collect reads the nav cache's counts set
// by SetNavCache, and that every locate key is present without them.
func TestCollectNavCache(t *testing.T) {
	keys := []string{LocatePinned, LocateCached, LocateComputed, LocatePlace}
	tests := []struct {
		name  string
		cache *NavCache
		want  map[string]uint64
	}{
		{name: "none", want: map[string]uint64{}},
		{
			name:  "counts",
			cache: &NavCache{Pinned: 1, Cached: 2, Computed: 3, Places: 4, LRUBytes: 5, PlaceCells: 6},
			want:  map[string]uint64{LocatePinned: 1, LocateCached: 2, LocateComputed: 3, LocatePlace: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			if got := r.Last().NavLocates; len(got) != len(keys) {
				t.Fatalf("empty snapshot locates %v, want every key", got)
			}
			if tt.cache != nil {
				r.SetNavCache(func() NavCache { return *tt.cache })
			}
			s := r.Collect(time.Unix(1, 0))
			for _, k := range keys {
				if v, ok := s.NavLocates[k]; !ok || v != tt.want[k] {
					t.Errorf("%s = %d (present %v), want %d", k, v, ok, tt.want[k])
				}
			}
			if tt.cache != nil && (s.NavLRUBytes != tt.cache.LRUBytes || s.NavPlaces != tt.cache.PlaceCells) {
				t.Errorf("LRU bytes %d places %d, want %d and %d", s.NavLRUBytes, s.NavPlaces, tt.cache.LRUBytes, tt.cache.PlaceCells)
			}
		})
	}
}
