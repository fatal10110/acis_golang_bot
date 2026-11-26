package nav

import "testing"

// logLabelLayers logs how the labels of h's LRU results split between the
// ground layer and the layers above it, and how full the upper layers are:
// the measure for storing upper layers sparsely (#496).
func logLabelLayers(b *testing.B, h *Hierarchy) {
	var results, layered int
	var ground, upper, upperCells, upperFilled, sparse int64
	for _, el := range h.cache.items {
		r := el.Value.(*clusterResult)
		results++
		if len(r.labels16)+len(r.labels32) > 1 {
			layered++
		}
		for li, l := range r.labels16 {
			if l == nil {
				continue
			}
			if li == 0 {
				ground += int64(len(l)) * 2
				continue
			}
			upper += int64(len(l)) * 2
			upperCells += int64(len(l))
			for _, v := range l {
				if v != noEntry16 {
					upperFilled++
				}
			}
		}
		for _, l := range r.sparse16 {
			sparse += int64(len(l.cols)) * 4
		}
		for li, l := range r.labels32 {
			if l == nil {
				continue
			}
			if li == 0 {
				ground += int64(len(l)) * 4
				continue
			}
			upper += int64(len(l)) * 4
			upperCells += int64(len(l))
			for _, v := range l {
				if v != noEntry {
					upperFilled++
				}
			}
		}
	}
	b.Logf("labels: %d results, %d with upper layers; ground layers %.1f MB, dense upper layers %.1f MB (%.1f%% of their cells labelled), sparse upper layers %.1f MB",
		results, layered, float64(ground)/1e6, float64(upper)/1e6, 100*float64(upperFilled)/float64(max(upperCells, 1)), float64(sparse)/1e6)
}
