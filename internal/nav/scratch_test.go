package nav

import "testing"

// drainScratch empties the idle query scratch, so a test sees only what it
// puts back.
func drainScratch() {
	for {
		select {
		case <-scratchFree:
		default:
			return
		}
	}
}

// TestScratchFreeList: an idle query scratch survives until the next query
// takes it (a GC does not empty the free list), at most keptScratch are
// kept, and one holding more than maxKeptScratchBytes goes to the GC.
func TestScratchFreeList(t *testing.T) {
	tests := []struct {
		name string
		put  []*scratch
		want []bool // per put: kept for the next getScratch
	}{
		{"kept", []*scratch{{}}, []bool{true}},
		{"at most keptScratch", []*scratch{{}, {}, {}}, []bool{true, true, false}},
		{"too large", []*scratch{{nodes: make([]node, 0, maxKeptScratchBytes/20+1)}}, []bool{false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drainScratch()
			defer drainScratch()
			for _, s := range tc.put {
				putScratch(s)
			}
			kept := map[*scratch]bool{}
			for range len(tc.put) {
				kept[getScratch()] = true
			}
			for i, s := range tc.put {
				if kept[s] != tc.want[i] {
					t.Errorf("scratch %d kept = %v, want %v", i, kept[s], tc.want[i])
				}
			}
		})
	}
}

// TestHierarchyKeepsMemberScratch: a cold Locate puts its cluster
// computation scratch back on the hierarchy, and the next cold Locate takes
// that one instead of a new one.
func TestHierarchyKeepsMemberScratch(t *testing.T) {
	th := hierScenes32(t)
	e := testEngine(t)
	cells := clusterCells(e, 32, sceneWindows32())
	h := NewHierarchy(th.g, e, nil, DefaultMemberCacheBytes)
	if _, err := h.Locate(cells[0], nil); err != nil {
		t.Fatal(err)
	}
	if len(h.spare) != 1 {
		t.Fatalf("idle member scratch after a cold Locate = %d, want 1", len(h.spare))
	}
	s := <-h.spare
	h.spare <- s
	var other Cell
	for _, c := range cells {
		if c.X/32 != cells[0].X/32 || c.Y/32 != cells[0].Y/32 {
			other = c
			break
		}
	}
	if _, err := h.Locate(other, nil); err != nil {
		t.Fatal(err)
	}
	if got := <-h.spare; got != s {
		t.Fatal("the second cold Locate did not reuse the idle scratch")
	}
}
