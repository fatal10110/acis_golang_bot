package e2e

import (
	"context"
	"testing"
)

// runFresh runs a farmer through run, a runWatched of a character the
// scenario wants at most at level top, and has the GM set it back to level
// 1 when the test ends, for the next run. A character an earlier run left
// above top (its cleanup failed, or the farmer gained a level fighting on
// between the cleanup and its logout) the GM sets back to 1 at once; but its
// brain claimed a spot for the old level as it entered, and a lower level
// never outlevels a spot, so it would keep that one (#268). The bot logs
// out instead, which gives the spot up, and run enters the character
// again: its new brain claims a spot for level 1.
func runFresh(t *testing.T, ctx context.Context, gm *GMClient, top int, run func() *farmer) *farmer {
	t.Helper()
	f := run()
	if lvl := levelOf(t, ctx, f); lvl > top {
		t.Logf("%s is level %d from an earlier run: back to 1, then in again", f.name, lvl)
		levelTo(t, ctx, gm, f, 1)
		f.stop()
		f = run()
		if lvl := levelOf(t, ctx, f); lvl != 1 {
			t.Fatalf("%s is level %d back in the world, want 1", f.name, lvl)
		}
	}
	// After run's own cleanups, so it runs first: the bot is still in the
	// world.
	t.Cleanup(func() { backToLevel1(t, gm, f) })
	return f
}
