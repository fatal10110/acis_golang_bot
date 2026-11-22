package nav

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// BenchmarkCorridorOpen measures the cheap test on a 1000-unit open segment.
func BenchmarkCorridorOpen(b *testing.B) {
	p := testPlanner(b)
	a, c := loc(Open), at(Open, 900, 430, 0)
	b.ReportAllocs()
	for b.Loop() {
		if !p.CorridorOpen(a, c) {
			b.Fatal("corridor closed")
		}
	}
}

// BenchmarkExactStretch measures the exact check up the stair: the cost N3
// sets its route thresholds from (docs/navigation.md, "Exact check").
func BenchmarkExactStretch(b *testing.B) {
	p := testPlanner(b)
	a, c := loc(StairLow), loc(StairDeck)
	b.ReportAllocs()
	for b.Loop() {
		if !p.StepChainOK(a, c) {
			b.Fatal("stair rejected")
		}
	}
}

// BenchmarkExactBridge measures the exact check along 8 cells of the bridge.
func BenchmarkExactBridge(b *testing.B) {
	p := testPlanner(b)
	a, c := loc(geotest.BridgeWestDeck), at(geotest.BridgeWestDeck, 8*16, 0, 0)
	b.ReportAllocs()
	for b.Loop() {
		if !p.StepChainOK(a, c) {
			b.Fatal("bridge rejected")
		}
	}
}

// BenchmarkRoute measures a route out of the U and around it.
func BenchmarkRoute(b *testing.B) {
	p := testPlanner(b)
	from, to := loc(geotest.UInside), loc(geotest.UBehind)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := p.Route(from, to); err != nil {
			b.Fatal(err)
		}
	}
}
