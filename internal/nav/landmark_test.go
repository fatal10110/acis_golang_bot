package nav

import (
	"encoding/binary"
	"hash/crc32"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// TestLandmarkDistancesAreShortest checks the nav cache's landmark section
// against the shortest-path conditions over the all-open edge costs: each
// landmark is 0 from itself, no edge shortens a distance, and every other
// reached node has an edge it is reached over exactly; a node with no way
// has no reached neighbour on its side. The landmarks are distinct nodes of
// the largest SCC.
func TestLandmarkDistancesAreShortest(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   func(testing.TB) *testHier
	}{
		{"scenes32", hierScenes32},
		{"scenes256", hierScenes256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := tc.th(t)
			g := th.g
			k := g.counts.landmarks
			if want := min(navLandmarks, th.stats.LargestSCC); k != want || th.stats.Landmarks != want {
				t.Fatalf("%d landmarks (stats %d), want %d", k, th.stats.Landmarks, want)
			}
			sizes := map[int32]int{}
			for v := range g.Nodes() {
				sizes[g.nodeSCC(v)]++
			}
			var ids []int
			for i := range k {
				l := int(g.u32(g.landmarkIDs + 4*i))
				if slices.Contains(ids, l) || sizes[g.nodeSCC(l)] != th.stats.LargestSCC {
					t.Fatalf("landmark %d: node %d repeated or outside the largest SCC", i, l)
				}
				ids = append(ids, l)
			}
			from := func(i, v int) uint32 { return g.u32(g.landmarkDist + 4*(v*2*k+i)) }
			to := func(i, v int) uint32 { return g.u32(g.landmarkDist + 4*(v*2*k+k+i)) }
			for i, l := range ids {
				// d(L, v) forward, d(v, L) over the reversed edges.
				for dir, d := range []func(v int) uint32{
					func(v int) uint32 { return from(i, v) },
					func(v int) uint32 { return to(i, v) },
				} {
					if d(l) != 0 {
						t.Fatalf("landmark %d dir %d: %d from itself", i, dir, d(l))
					}
					tight := make([]bool, g.Nodes())
					tight[l] = true
					for u := range g.Nodes() {
						lo, hi := g.nodeEdges(u)
						for e := lo; e < hi; e++ {
							r := g.edge(e)
							a, b := u, r.to // a before b on the way from L
							if dir == 1 {
								a, b = b, a
							}
							if d(a) == noDist {
								continue
							}
							if d(b) == noDist || uint64(d(b)) > uint64(d(a))+uint64(r.open) {
								t.Fatalf("landmark %d dir %d: edge %d->%d (cost %d) shortens %d to %d+%d", i, dir, u, r.to, r.open, d(b), d(a), r.open)
							}
							if uint64(d(b)) == uint64(d(a))+uint64(r.open) {
								tight[b] = true
							}
						}
					}
					for v, ok := range tight {
						if !ok && d(v) != noDist {
							t.Fatalf("landmark %d dir %d: node %d at %d has no edge reaching it exactly", i, dir, v, d(v))
						}
					}
				}
			}
		})
	}
}

// TestLandmarksKeepDistances compares the abstract distance with the
// landmark bound to the octile-only search's on random pairs of the scene
// windows: the bound only prunes, so every answer is the same.
func TestLandmarksKeepDistances(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   func(testing.TB) *testHier
		keys []ClusterKey
		size int
	}{
		{"scenes32", hierScenes32, sceneWindows32(), 32},
		{"scenes256", hierScenes256, window(DefaultClusterCells, 4, 1, 6, 1), DefaultClusterCells},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := tc.th(t)
			if th.g.counts.landmarks == 0 {
				t.Fatal("no landmarks")
			}
			plain := *th.g
			plain.counts.landmarks = 0
			octile := NewHierarchy(&plain, th.cfg.Engine, th.cfg.Water, DefaultMemberCacheBytes)
			cells := clusterCells(th.cfg.Engine, tc.size, tc.keys)
			rng := rand.New(rand.NewPCG(315, 1))
			compared, crossed := 0, 0
			for range 3000 {
				a, b := cells[rng.IntN(len(cells))], cells[rng.IntN(len(cells))]
				ma, errA := th.h.Locate(a, nil)
				mb, errB := th.h.Locate(b, nil)
				if errA != nil || errB != nil || !th.h.SameSCC(ma, mb) {
					continue
				}
				got, gerr := th.h.Distance(ma, mb)
				want, werr := octile.Distance(ma, mb)
				if got != want || (gerr == nil) != (werr == nil) {
					t.Fatalf("%v -> %v: distance %d (%v) with landmarks, %d (%v) without", a, b, got, gerr, want, werr)
				}
				compared++
				if ma.res.cluster != mb.res.cluster {
					crossed++
				}
			}
			if crossed < 100 {
				t.Fatalf("compared %d pairs, %d across clusters: too few", compared, crossed)
			}
		})
	}
}

// TestNavCacheRefusesTooManyLandmarks refuses an image whose header names
// more landmarks than a build writes.
func TestNavCacheRefusesTooManyLandmarks(t *testing.T) {
	img := slices.Clone(hierScenes32(t).image)
	binary.LittleEndian.PutUint32(img[64:], navLandmarks+1)
	binary.LittleEndian.PutUint32(img[12:], crc32.Checksum(img[16:], navCRC))
	if _, err := parseGraph(img); err == nil || !strings.Contains(err.Error(), "landmarks") {
		t.Fatalf("parseGraph: %v, want a landmark count error", err)
	}
}
