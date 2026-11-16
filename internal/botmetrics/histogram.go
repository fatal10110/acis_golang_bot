package botmetrics

import (
	"encoding/json"
	"math/bits"
	"sync/atomic"
)

// histBuckets is one bucket per bit length of a uint64, plus bucket 0.
const histBuckets = 65

// Histogram counts values in power-of-two buckets: bucket 0 holds values of
// zero or less, bucket i holds [2^(i-1), 2^i). It is a fixed array, so it
// never grows with the run, and Observe never allocates. Observe is safe
// for one writer at a time against concurrent snapshots.
type Histogram struct {
	counts [histBuckets]atomic.Uint64
	sum    atomic.Uint64
	max    atomic.Int64
}

// Observe counts v.
func (h *Histogram) Observe(v int64) {
	v = max(v, 0)
	h.counts[bits.Len64(uint64(v))].Add(1)
	h.sum.Add(uint64(v))
	for {
		m := h.max.Load()
		if v <= m || h.max.CompareAndSwap(m, v) {
			return
		}
	}
}

// HistogramSnapshot is the sum of histograms at one collection.
type HistogramSnapshot struct {
	Count   uint64
	Sum     uint64
	Max     int64
	Buckets [histBuckets]uint64
}

// add folds h into s.
func (s *HistogramSnapshot) add(h *Histogram) {
	for i := range h.counts {
		n := h.counts[i].Load()
		s.Buckets[i] += n
		s.Count += n
	}
	s.Sum += h.sum.Load()
	s.Max = max(s.Max, h.max.Load())
}

// bucketBound is the largest value bucket i holds.
func bucketBound(i int) uint64 {
	if i == 0 {
		return 0
	}
	return 1<<uint(i) - 1
}

// MarshalJSON writes the totals and the non-empty buckets, each with the
// largest value it holds ("le").
func (s HistogramSnapshot) MarshalJSON() ([]byte, error) {
	type bucket struct {
		LE uint64 `json:"le"`
		N  uint64 `json:"n"`
	}
	out := struct {
		Count   uint64   `json:"count"`
		Sum     uint64   `json:"sum"`
		Max     int64    `json:"max"`
		Buckets []bucket `json:"buckets"`
	}{Count: s.Count, Sum: s.Sum, Max: s.Max, Buckets: []bucket{}}
	for i, n := range s.Buckets {
		if n > 0 {
			out.Buckets = append(out.Buckets, bucket{LE: bucketBound(i), N: n})
		}
	}
	return json.Marshal(out)
}
