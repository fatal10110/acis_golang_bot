package e2e

import (
	"slices"
	"testing"
)

// TestGMClientForget: a forgotten monster is no longer the run's to select
// or delete, and the GM's target falls back to none when it was the one.
func TestGMClientForget(t *testing.T) {
	tg := &Target{scope: newScope()}
	tg.scope.npcs[5], tg.scope.npcs[6] = true, true
	tg.scope.target = 5
	gm := &GMClient{t: tg}
	gm.Forget(5)
	if got := gm.Spawned(); !slices.Equal(got, []int32{6}) {
		t.Errorf("Spawned = %v, want [6]", got)
	}
	if tg.scope.owns(5) || tg.scope.target != 0 {
		t.Errorf("monster 5 still owned (%v) or targeted (%d)", tg.scope.owns(5), tg.scope.target)
	}
	gm.Forget(6)
	if got := gm.Spawned(); len(got) != 0 {
		t.Errorf("Spawned = %v, want none", got)
	}
}
