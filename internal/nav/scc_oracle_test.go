package nav

import (
	"math/rand/v2"
	"testing"
)

// TestSCCScratchMatchesMutualReach checks the Tarjan the brute-force
// oracle shares with the build against plain BFS: two vertices share a
// label exactly when each reaches the other, and every edge runs from a
// label to itself or a lower one (completion order).
func TestSCCScratchMatchesMutualReach(t *testing.T) {
	rng := rand.New(rand.NewPCG(155, 1))
	for round := range 200 {
		n := 1 + rng.IntN(40)
		adj := make([][]int32, n)
		edges := rng.IntN(3 * n)
		for range edges {
			u, v := rng.IntN(n), rng.IntN(n)
			adj[u] = append(adj[u], int32(v))
		}
		var s sccScratch
		count := s.run(n,
			func(v int32) int32 { return int32(len(adj[v])) },
			func(v, k int32) int32 { return adj[v][k] })
		reach := make([][]bool, n)
		for u := range n {
			reach[u] = bfsReach(adj, u)
		}
		for u := range n {
			if l := s.label[u]; l < 0 || int(l) >= int(count) {
				t.Fatalf("round %d: vertex %d label %d outside 0..%d", round, u, l, count)
			}
			for _, v := range adj[u] {
				if s.label[v] > s.label[u] {
					t.Fatalf("round %d: edge %d->%d climbs label %d->%d", round, u, v, s.label[u], s.label[v])
				}
			}
			for v := range n {
				same := s.label[u] == s.label[v]
				if mutual := reach[u][v] && reach[v][u]; same != mutual {
					t.Fatalf("round %d: %d,%d same label %v, mutually reachable %v", round, u, v, same, mutual)
				}
			}
		}
	}
}

func bfsReach(adj [][]int32, from int) []bool {
	seen := make([]bool, len(adj))
	seen[from] = true
	queue := []int{from}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if !seen[v] {
				seen[v] = true
				queue = append(queue, int(v))
			}
		}
	}
	return seen
}
