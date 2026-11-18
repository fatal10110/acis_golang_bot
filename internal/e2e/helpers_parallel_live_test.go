package e2e

import (
	"sync"
	"testing"
)

// Scenario groups whose scenarios share ground another scenario of the
// group would disturb, so they never run at the same time; the scenarios
// of different groups, and those of none, run in parallel (t.Parallel, at
// most MaxScenarios at once).
var (
	// newbieSpots: farmers on the newbie spots, and a GM at tiSpots whose
	// sight a bot's assist depends on. Each scenario's spot registry knows
	// only its own farmers, so another's farmer could take its monster.
	newbieSpots sync.Mutex
	// townNPCs: bots sent to the village's grocer, stores, trainers and
	// merchants, who would crowd each other there.
	townNPCs sync.Mutex
)

// alone waits while another scenario of group runs, then keeps the group
// until the test ends. Call it after Live: a scenario holds its group only
// while it holds its place among the run's scenarios, which keeps the
// group from waiting on a scenario that waits on the group.
func alone(t *testing.T, group *sync.Mutex) {
	t.Helper()
	group.Lock()
	t.Cleanup(group.Unlock)
}
