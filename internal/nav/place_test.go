package nav

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// TestLocatePlaceKeepsStaticAnswers locates each place twice through a
// hierarchy whose LRU keeps nothing: a place in an abstract SCC and a cell
// off the navigation are answered from the place table the second time,
// with no cluster computed, while ground in no abstract SCC is computed
// again (#496).
func TestLocatePlaceKeepsStaticAnswers(t *testing.T) {
	th := hierScenes256(t)
	for _, tc := range []struct {
		name     string
		cell     Cell
		kind     Kind
		noNav    bool
		computes uint64 // by the second LocatePlace
	}{
		{name: "ground by the pit", cell: pointCell(geotest.PitOutside), kind: InSCC},
		{name: "ground under the bridge", cell: pointCell(geotest.BridgeWestGround), kind: InSCC},
		{name: "bridge deck", cell: pointCell(geotest.BridgeWestDeck), kind: OneWayOut, computes: 1},
		{name: "pit floor", cell: pointCell(geotest.PitInside), kind: Isolated, computes: 1},
		{name: "off the navigation", cell: Cell{X: -1, Y: -1}, noNav: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHierarchy(th.g, testEngine(t), nil, 0)
			want, err := th.h.Locate(tc.cell, nil)
			if tc.noNav != errors.Is(err, ErrNoNav) {
				t.Fatalf("Locate: %v", err)
			}
			for pass := range 2 {
				before := h.Stats()
				m, err := h.LocatePlace(tc.cell)
				if tc.noNav {
					if !errors.Is(err, ErrNoNav) {
						t.Fatalf("pass %d: err %v, want ErrNoNav", pass, err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("pass %d: %v", pass, err)
				}
				if m.Kind() != tc.kind || m.SCC() != want.SCC() || m.Cell() != tc.cell {
					t.Fatalf("pass %d: %v SCC %d at %v, want %v SCC %d", pass, m.Kind(), m.SCC(), m.Cell(), tc.kind, want.SCC())
				}
				if !th.h.SameSCC(m, want) && tc.kind == InSCC {
					t.Errorf("pass %d: the place is not SameSCC with its Locate", pass)
				}
				if pass == 1 {
					if got := h.Stats().Computed - before.Computed; got != tc.computes {
						t.Errorf("second LocatePlace computed %d clusters, want %d", got, tc.computes)
					}
				}
			}
			kept := 0
			if tc.noNav || tc.kind == InSCC {
				kept = 1
			}
			if s := h.Stats(); s.PlaceCells != kept || s.Places != uint64(kept) {
				t.Errorf("place table holds %d cells, answered %d, want %d", s.PlaceCells, s.Places, kept)
			}
		})
	}
}

// TestLocatePlaceNeverPins checks that a place, which holds no labels, never
// answers a Locate as a pin, and that it ranks as its Locate does.
func TestLocatePlaceNeverPins(t *testing.T) {
	th := hierScenes256(t)
	h := NewHierarchy(th.g, testEngine(t), nil, 0)
	a, b := pointCell(geotest.PitOutside), pointCell(geotest.PocketOutside)
	pa, err := h.LocatePlace(a)
	if err != nil {
		t.Fatal(err)
	}
	if pa.res.labels16 != nil || pa.res.labels32 != nil {
		t.Fatal("a place holds its cluster's labels")
	}
	before := h.Stats()
	m, err := h.Locate(a, &Pins{Current: pa, Destination: pa})
	if err != nil {
		t.Fatal(err)
	}
	if s := h.Stats(); s.Pinned != before.Pinned || s.Computed != before.Computed+1 || m.res == pa.res {
		t.Errorf("Locate pinned to a place: %+v after %+v", s, before)
	}
	mb, err := th.h.Locate(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	ma, _ := th.h.Locate(a, nil)
	want, werr := th.h.Distance(ma, mb)
	got, gerr := h.Distance(pa, mb)
	if got != want || !errors.Is(gerr, werr) {
		t.Errorf("Distance from the place %d (%v), from its Locate %d (%v)", got, gerr, want, werr)
	}
	if h.Reachable(pa, mb) != th.h.Reachable(ma, mb) {
		t.Error("Reachable from the place differs from its Locate's")
	}
}

// TestHierarchyStatsCountLocates checks how Stats counts Locates: computed,
// then from the LRU, then from a pin.
func TestHierarchyStatsCountLocates(t *testing.T) {
	th := hierScenes256(t)
	h := NewHierarchy(th.g, testEngine(t), nil, DefaultMemberCacheBytes)
	c := pointCell(geotest.PitInside)
	m, err := h.Locate(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Locate(c, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Locate(c, &Pins{Current: m}); err != nil {
		t.Fatal(err)
	}
	s := h.Stats()
	if s.Computed != 1 || s.Cached != 1 || s.Pinned != 1 || s.LRUResults != 1 || s.LRUBytes != m.res.bytes {
		t.Errorf("stats %+v, want one of each and the result's %d bytes", s, m.res.bytes)
	}
}
