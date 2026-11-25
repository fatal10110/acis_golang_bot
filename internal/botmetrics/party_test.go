package botmetrics

import (
	"maps"
	"testing"
	"time"
)

func TestMetricsParties(t *testing.T) {
	r := NewRegistry()
	if s := r.Collect(time.Time{}); s.PartiesFormed != 0 || len(s.PartiesBroken) != 0 || s.PartiesBroken == nil {
		t.Fatalf("fresh registry: formed %d broken %v", s.PartiesFormed, s.PartiesBroken)
	}
	r.PartyFormed()
	r.PartyFormed()
	r.PartyBroken("no healer")
	r.PartyBroken("no healer")
	r.PartyBroken("dissolved by the panel")
	s := r.Collect(time.Time{})
	want := map[string]uint64{"no healer": 2, "dissolved by the panel": 1}
	if s.PartiesFormed != 2 || !maps.Equal(s.PartiesBroken, want) {
		t.Errorf("formed %d broken %v, want 2 and %v", s.PartiesFormed, s.PartiesBroken, want)
	}
	r.PartyBroken("no healer")
	if s.PartiesBroken["no healer"] != 2 {
		t.Error("a snapshot shares the registry's map")
	}
}
