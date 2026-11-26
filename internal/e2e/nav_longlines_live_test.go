package e2e

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// longLine is one desync of the home run of 2026-11-25 (#502): the server
// held the bot at origin when it clicked to, and answered toward dest
// although the bot's geodata opens the line from origin to to.
type longLine struct {
	name             string
	origin, to, dest botclient.Point
}

var longLines = []longLine{
	{"hz275", botclient.Point{X: 140901, Y: -201564, Z: -3584}, botclient.Point{X: 141272, Y: -204024, Z: -3528}, botclient.Point{X: 140969, Y: -201536, Z: -3576}},
	{"hz318", botclient.Point{X: -30600, Y: -112008, Z: -1472}, botclient.Point{X: -29656, Y: -111976, Z: -1352}, botclient.Point{X: -28680, Y: -111928, Z: -1296}},
	{"hz387", botclient.Point{X: -30312, Y: -107512, Z: -1448}, botclient.Point{X: -28520, Y: -106328, Z: -1792}, botclient.Point{X: -30216, Y: -107416, Z: -1320}},
	{"hz27", botclient.Point{X: 119960, Y: -187064, Z: -3296}, botclient.Point{X: 119480, Y: -184616, Z: -3240}, botclient.Point{X: 120104, Y: -187464, Z: -3289}},
	{"hz411", botclient.Point{X: -30216, Y: -107416, Z: -1344}, botclient.Point{X: -28472, Y: -106280, Z: -1848}, botclient.Point{X: -28664, Y: -105464, Z: -1372}},
	{"hz545", botclient.Point{X: -45384, Y: -110136, Z: -72}, botclient.Point{X: -43688, Y: -108328, Z: -1616}, botclient.Point{X: -43560, Y: -109560, Z: -1686}},
	{"hz893", botclient.Point{X: -32168, Y: -112056, Z: -2048}, botclient.Point{X: -29672, Y: -111976, Z: -1352}, botclient.Point{X: -30600, Y: -112008, Z: -1453}},
	{"hz668", botclient.Point{X: 45908, Y: 44204, Z: -3704}, botclient.Point{X: 46192, Y: 44134, Z: -3704}, botclient.Point{X: 45920, Y: 44166, Z: -3677}},
	{"hz336", botclient.Point{X: -16584, Y: -112696, Z: -2704}, botclient.Point{X: -18344, Y: -114456, Z: -2736}, botclient.Point{X: -19080, Y: -112696, Z: -2552}},
	{"hz20", botclient.Point{X: 45512, Y: 46552, Z: -2992}, botclient.Point{X: 45416, Y: 47528, Z: -2992}, botclient.Point{X: 45192, Y: 47304, Z: -2970}},
	{"hz897", botclient.Point{X: -45267, Y: -112390, Z: -240}, botclient.Point{X: -45112, Y: -112968, Z: -240}, botclient.Point{X: -45264, Y: -112512, Z: -217}},
}

// TestLiveLongLines replays each long-line desync of #502 on the live
// server: the GM stands where the server held the bot and clicks the bot's
// target, as the walker clicked it (MoveBackwardToLocation, floor Z). From
// a standstill the server answers every one with a direct walk to the
// click, which is what the bot's geodata says of the line (CanMove from the
// server's origin to the click at head height): the line check of the
// walker and the server's agree, and the home run's answers were legs of a
// route the server was already walking (docs/navigation.md, "Server answers
// to its own moves"). Each click goes twice, so a GM fence or a door that
// comes and goes shows as a split verdict.
func TestLiveLongLines(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	e := tg.Nav(t).Engine
	gm := enterGM(t, ctx, tg)
	ch := int32(userInfo(t, gm.Client).CollisionHeight)
	for _, c := range longLines {
		for try := range 2 {
			gmTeleport(t, ctx, gm, c.origin)
			m, err := gm.Move(ctx, c.to)
			if errors.Is(err, ErrMoveRefused) {
				t.Errorf("%s try %d: the click from %v to %v was refused", c.name, try, c.origin, c.to)
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			open := e.CanMove(int(m.Origin.X), int(m.Origin.Y), int(m.Origin.Z), int(c.to.X), int(c.to.Y), int(c.to.Z+ch))
			direct := near2(m.Dest, c.to) <= clickSlop*clickSlop
			t.Logf("%s try %d: origin %v click %v: server dest %v (home run %v), bot CanMove %v", c.name, try, m.Origin, c.to, m.Dest, c.dest, open)
			if direct != open {
				t.Errorf("%s try %d: the server walks from %v toward %v for the click %v (direct %v), the bot's CanMove is %v", c.name, try, m.Origin, m.Dest, c.to, direct, open)
			}
		}
	}
}
