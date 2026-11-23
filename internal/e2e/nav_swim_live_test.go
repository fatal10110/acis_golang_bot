package e2e

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// In Schuttgart's region 23_11 the sea (WaterZone.xml 23_11_water1, maxZ
// -3780) covers the low ground between swimShore, a STANDARD teleport
// destination (teleports.xml), and swimFar, about 2000 west of it on the
// way to the farming spot schttgart08_2311_05m1. The hierarchy keeps out
// of water and finds no dry way within its budget, so the bot reaches
// swimFar only on the swimming cell planner's route: it wades in, swims
// about 600 and climbs out (docs/navigation.md, "Swimming"; #223, #161's
// main loss).
var (
	swimShore = botclient.Point{X: 124054, Y: -200170, Z: -3704}
	swimFar   = botclient.Point{X: 122072, Y: -199944, Z: -3640}
)

// TestLiveNavSwim has a bot follow the GM from swimShore across the water
// to swimFar, out of sight. runWatched asserts nav_desync_total 0 and the
// blocked stops the step chain predicts.
func TestLiveNavSwim(t *testing.T) {
	t.Parallel()
	followAcross(t, "wm", swimShore, swimFar)
}

// TestLiveNavSwimBack is TestLiveNavSwim the other way, from swimFar back
// across the water to swimShore.
func TestLiveNavSwimBack(t *testing.T) {
	t.Parallel()
	followAcross(t, "wr", swimFar, swimShore)
}
