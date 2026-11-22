package nav

import (
	"log/slog"
	"math"
	"slices"
	"strconv"
	"testing"
)

// TestDoorFootTagsTheNearestLayerOnly checks that a door blocks exactly the
// layer the server lifts on each footprint cell, the one nearest the
// door's floor, and no other height on that cell.
func TestDoorFootTagsTheNearestLayerOnly(t *testing.T) {
	e, doors := doorScene(t)
	static := e.Static()
	b := &builder{}
	if n, skipped := b.loadDoors(doors, static, slog.New(slog.DiscardHandler)); n != 1 || skipped != 0 {
		t.Fatalf("loaded %d doors, skipped %d; want 1, 0", n, skipped)
	}
	if len(b.doorCells) == 0 {
		t.Fatal("door has no footprint cells")
	}
	for k := range b.doorCells {
		z := static.HeightNearest(k[0], k[1], 0)
		tests := []struct {
			name string
			z    int16
			want []int32
		}{
			{"the lifted layer", z, []int32{doorID}},
			{"inside the door's height above it", z + 20, nil},
			{"within a cell's reach below it", z - 30, nil},
		}
		for _, tt := range tests {
			if got := b.doorsAt(nil, Cell{X: k[0], Y: k[1], Z: tt.z}); !slices.Equal(got, tt.want) {
				t.Errorf("cell %v, %s (z %d): doors %v, want %v", k, tt.name, tt.z, got, tt.want)
			}
		}
	}
}

// TestLoadDoorsSkipsIDOutsideInt32 checks that a door id the graph file
// cannot hold is skipped, not narrowed.
func TestLoadDoorsSkipsIDOutsideInt32(t *testing.T) {
	if strconv.IntSize == 32 {
		t.Skip("every int fits int32")
	}
	e, doors := doorScene(t)
	big := int64(math.MaxInt32) + 1
	doors[0].ID = int(big)
	b := &builder{}
	if n, skipped := b.loadDoors(doors, e.Static(), slog.New(slog.DiscardHandler)); n != 0 || skipped != 1 {
		t.Fatalf("loaded %d doors, skipped %d; want 0, 1", n, skipped)
	}
	if len(b.doorCells) != 0 {
		t.Fatalf("skipped door left %d footprint cells", len(b.doorCells))
	}
}

// TestAddEdgeMergesCrossings checks that crossings between the same two
// nodes merge into one edge whose closed-door fallback is the cheapest
// door-free crossing, whatever order they arrive in.
func TestAddEdgeMergesCrossings(t *testing.T) {
	door := []int32{doorID}
	tests := []struct {
		name string
		in   []globalEdge
		want globalEdge
	}{
		{
			"door crossing first, door-free crossing at the same cost",
			[]globalEdge{{to: 1, open: 10, free: noCost, doors: door}, {to: 1, open: 10, free: 10}},
			globalEdge{to: 1, open: 10, free: 10},
		},
		{
			"door-free crossing first",
			[]globalEdge{{to: 1, open: 10, free: 10}, {to: 1, open: 10, free: noCost, doors: door}},
			globalEdge{to: 1, open: 10, free: 10},
		},
		{
			"cheaper door crossing keeps the dearer door-free fallback",
			[]globalEdge{{to: 1, open: 14, free: 14}, {to: 1, open: 10, free: noCost, doors: door}},
			globalEdge{to: 1, open: 10, free: 14, doors: door},
		},
		{
			"door crossings only",
			[]globalEdge{{to: 1, open: 10, free: noCost, doors: door}, {to: 1, open: 12, free: noCost, doors: []int32{8}}},
			globalEdge{to: 1, open: 10, free: noCost, doors: door},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var edges []globalEdge
			for i, e := range tt.in {
				if isNew := addEdge(&edges, e); isNew != (i == 0) {
					t.Fatalf("crossing %d: new target %v", i, isNew)
				}
			}
			if len(edges) != 1 {
				t.Fatalf("%d edges, want 1", len(edges))
			}
			got := edges[0]
			if got.to != tt.want.to || got.open != tt.want.open || got.free != tt.want.free || !slices.Equal(got.doors, tt.want.doors) {
				t.Errorf("merged %+v, want %+v", got, tt.want)
			}
		})
	}
}
