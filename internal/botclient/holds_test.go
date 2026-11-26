package botclient

import (
	"maps"
	"testing"
	"time"
)

func TestHoldsHeld(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name string
		at   time.Duration
		want bool
	}{
		{name: "before the time", at: time.Minute - time.Nanosecond, want: true},
		{name: "at the time", at: time.Minute, want: false},
		{name: "after the time", at: time.Minute + time.Second, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(holds[int32])
			h.set(7, t0.Add(time.Minute))
			if got := h.held(7, t0.Add(tt.at)); got != tt.want {
				t.Errorf("held = %v, want %v", got, tt.want)
			}
			if h.held(8, t0) {
				t.Error("a key never set is held")
			}
		})
	}
}

func TestHoldsSetExtend(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name  string
		have  time.Duration // 0: the key is not set
		apply func(h holds[string], k string, until time.Time)
		until time.Duration
		want  time.Duration
	}{
		{name: "set a new key", apply: holds[string].set, until: time.Minute, want: time.Minute},
		{name: "set shortens", have: time.Hour, apply: holds[string].set, until: time.Minute, want: time.Minute},
		{name: "extend a new key", apply: holds[string].extend, until: time.Minute, want: time.Minute},
		{name: "extend to a later time", have: time.Minute, apply: holds[string].extend, until: time.Hour, want: time.Hour},
		{name: "extend never shortens", have: time.Hour, apply: holds[string].extend, until: time.Minute, want: time.Hour},
		{name: "extend to the same time", have: time.Hour, apply: holds[string].extend, until: time.Hour, want: time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(holds[string])
			if tt.have > 0 {
				h.set("a", t0.Add(tt.have))
			}
			tt.apply(h, "a", t0.Add(tt.until))
			if got := h["a"]; !got.Equal(t0.Add(tt.want)) {
				t.Errorf("until = %v, want %v", got, t0.Add(tt.want))
			}
			if len(h) != 1 {
				t.Errorf("holds = %v, want only a", h)
			}
		})
	}
}

func TestHoldsPrune(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	h := holds[int]{1: t0.Add(-time.Second), 2: t0, 3: t0.Add(time.Nanosecond), 4: t0.Add(time.Hour)}
	h.prune(t0)
	if want := (holds[int]{3: t0.Add(time.Nanosecond), 4: t0.Add(time.Hour)}); !maps.Equal(h, want) {
		t.Errorf("pruned = %v, want %v", h, want)
	}
}

func TestHoldsNil(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var h holds[int32]
	if h.held(1, t0) {
		t.Error("nil holds holds a key")
	}
	h.prune(t0) // must not panic
	if h != nil {
		t.Errorf("prune made a nil holds %v", h)
	}
}
