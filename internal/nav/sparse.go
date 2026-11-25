package nav

import (
	"cmp"
	"slices"
)

// sparseFill sets the share of its columns, one in sparseFill, from which an
// upper label layer is stored dense: a sparse cell takes 4 bytes, a dense
// column 2. Upper layers measured 4.4% full on the owner's geodata (#496).
const sparseFill = 2

// labelLayers16 sets r's 16-bit labels of g's n cells over layers layers of
// cols columns, entry giving each cell's: the ground layer and every upper
// layer with cells in at least one column of share dense, the others sparse.
// Columns past 16 bits (a cluster side over 256) are always dense.
func (r *clusterResult) labelLayers16(g *localGraph, n, layers, cols, share int, entry func(i int) uint16) {
	count := make([]int, layers)
	for i := range n {
		count[g.layer(int32(i))]++
	}
	r.labels16 = make([][]uint16, layers)
	for li, k := range count {
		switch {
		case k == 0:
		case li == 0 || k*share >= cols || cols > 1<<16:
			r.labels16[li] = slices.Repeat([]uint16{noEntry16}, cols)
			r.bytes += int64(cols) * 2
		default:
			if r.sparse16 == nil {
				r.sparse16 = make([]sparseLayer, layers)
			}
			r.sparse16[li] = sparseLayer{cols: make([]uint16, 0, k), entries: make([]uint16, 0, k)}
			r.bytes += int64(k) * 4
		}
	}
	for i := range n {
		li := g.layer(int32(i))
		if l := r.labels16[li]; l != nil {
			l[g.col[i]] = entry(i)
			continue
		}
		sl := &r.sparse16[li]
		sl.cols = append(sl.cols, uint16(g.col[i]))
		sl.entries = append(sl.entries, entry(i))
	}
	for li := range r.sparse16 {
		r.sparse16[li].sort()
	}
}

// sparseLayer is an upper label layer few columns have: their columns in
// increasing order and the entry of each.
type sparseLayer struct {
	cols    []uint16
	entries []uint16
}

func (l *sparseLayer) entryOf(col int) uint32 {
	if k, ok := slices.BinarySearch(l.cols, uint16(col)); ok {
		return uint32(l.entries[k])
	}
	return noEntry
}

// sort orders l by column, keeping each column's entry.
func (l *sparseLayer) sort() {
	if slices.IsSorted(l.cols) {
		return
	}
	idx := make([]int, len(l.cols))
	for k := range idx {
		idx[k] = k
	}
	slices.SortFunc(idx, func(a, b int) int { return cmp.Compare(l.cols[a], l.cols[b]) })
	cols, entries := make([]uint16, len(idx)), make([]uint16, len(idx))
	for k, j := range idx {
		cols[k], entries[k] = l.cols[j], l.entries[j]
	}
	l.cols, l.entries = cols, entries
}
