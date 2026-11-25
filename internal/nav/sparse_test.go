package nav

import (
	"math"
	"testing"
)

// TestSparseLabelsMatchDense computes every cluster of the scenes with the
// upper label layers sparse and all dense, and checks that both answer the
// same entry for every column of every layer (#496).
func TestSparseLabelsMatchDense(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *testHier
	}{
		{"32-cell clusters", hierScenes32(t)},
		{"256-cell clusters", hierScenes256(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, sparseLayers := tc.th.h, 0
			for ci := range h.graph.Clusters() {
				sparse, err := h.computeFill(ci, sparseFill)
				if err != nil {
					t.Fatal(err)
				}
				dense, err := h.computeFill(ci, math.MaxInt32)
				if err != nil {
					t.Fatal(err)
				}
				for _, l := range sparse.sparse16 {
					if len(l.cols) > 0 {
						sparseLayers++
					}
				}
				if sparse.bytes > dense.bytes {
					t.Errorf("cluster %d: sparse result %d bytes over the dense %d", ci, sparse.bytes, dense.bytes)
				}
				for layer := range max(len(dense.labels16), len(dense.labels32), 1) {
					for col := range dense.size * dense.size {
						lx, ly := col%dense.size, col/dense.size
						if got, want := sparse.entryOf(layer, lx, ly), dense.entryOf(layer, lx, ly); got != want {
							t.Fatalf("cluster %d layer %d column %d: entry %d, dense %d", ci, layer, col, got, want)
						}
					}
				}
			}
			if sparseLayers == 0 {
				t.Error("no cluster stored a sparse layer: the scenes do not cover it")
			}
		})
	}
}

// TestSparseLayerEntryOf checks the sparse layer's lookup after sorting
// columns appended out of order.
func TestSparseLayerEntryOf(t *testing.T) {
	l := sparseLayer{cols: []uint16{9, 2, 65535, 4}, entries: []uint16{90, 20, 7, 40}}
	l.sort()
	for _, tc := range []struct {
		col  int
		want uint32
	}{{2, 20}, {4, 40}, {9, 90}, {65535, 7}, {0, noEntry}, {3, noEntry}, {10, noEntry}} {
		if got := l.entryOf(tc.col); got != tc.want {
			t.Errorf("column %d: %d, want %d", tc.col, got, tc.want)
		}
	}
}
