package botmetrics

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestHistogramBuckets(t *testing.T) {
	tests := []struct {
		v      int64
		bucket int
	}{
		{-5, 0}, {0, 0}, {1, 1}, {2, 2}, {3, 2}, {4, 3}, {1023, 10}, {1024, 11}, {math.MaxInt64, 63},
	}
	for _, tt := range tests {
		var h Histogram
		h.Observe(tt.v)
		var s HistogramSnapshot
		s.add(&h)
		if s.Buckets[tt.bucket] != 1 || s.Count != 1 {
			t.Errorf("Observe(%d): buckets %v, want one value in bucket %d", tt.v, s.Buckets, tt.bucket)
		}
		if lo := max(tt.v, 0); uint64(lo) > bucketBound(tt.bucket) {
			t.Errorf("Observe(%d) in bucket %d bounded by %d", tt.v, tt.bucket, bucketBound(tt.bucket))
		}
	}
}

func TestHistogramSnapshotJSON(t *testing.T) {
	var a, b Histogram
	for _, v := range []int64{0, 1, 5, 6} {
		a.Observe(v)
	}
	b.Observe(100)
	var s HistogramSnapshot
	s.add(&a)
	s.add(&b)
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"count":5,"sum":112,"max":100,"buckets":[{"le":0,"n":1},{"le":1,"n":1},{"le":7,"n":2},{"le":127,"n":1}]}`
	if string(got) != want {
		t.Errorf("json = %s\nwant   %s", got, want)
	}
}

// TestMetricsNoAllocs checks that recording never allocates: a long run
// keeps the metrics' memory flat whatever it records.
func TestMetricsNoAllocs(t *testing.T) {
	bot := NewRegistry("idle").NewBot()
	var none *Bot
	tests := []struct {
		name string
		f    func()
	}{
		{"Histogram.Observe", func() { bot.depth.Observe(12345) }},
		{"Drained", func() { bot.Drained(3) }},
		{"Sent", bot.Sent},
		{"Paced", bot.Paced},
		{"Rule", func() { bot.Rule("idle") }},
		{"Rule other", func() { bot.Rule("unknown") }},
		{"WakeLag", func() { bot.WakeLag(3 * time.Millisecond) }},
		{"Reconnect", bot.Reconnect},
		{"Abort", bot.Abort},
		{"nil Bot", func() { none.Drained(1); none.Sent(); none.Rule("idle"); none.WakeLag(time.Second) }},
	}
	for _, tt := range tests {
		if n := testing.AllocsPerRun(1000, tt.f); n != 0 {
			t.Errorf("%s allocates %v times per call", tt.name, n)
		}
	}
}
