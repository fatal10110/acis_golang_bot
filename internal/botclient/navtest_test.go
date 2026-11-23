package botclient

import (
	"bytes"
	"context"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// The navigation the walker tests walk on: the geotest fixture scenes
// (synthetic regions, never real geodata), with a nav hierarchy built over
// the clusters holding the corridor, the U, the ledge, the pit, the bridge
// and the pocket at the default cluster side, where each scene lies inside
// one cluster, as the nav package's own tests build it, and over the open
// ground along the X axis east of the world origin (the fixture region's
// north-west corner), where the consumer tests stand.
var (
	navOnce   sync.Once
	navShared *Nav
	navErr    error
)

// testNav is the shared synthetic navigation, built once per test binary.
func testNav(tb testing.TB) *Nav {
	tb.Helper()
	navOnce.Do(func() {
		// Row 0 holds the open flat ground along the X axis from the world
		// origin, where the consumer tests put their NPCs (walkOn); row 1 the
		// scenes.
		navShared, navErr = buildTestNav(geotest.Scenes(), append(navWindow(0, 0, 1, 0), navWindow(1, 1, 6, 1)...))
	})
	if navErr != nil {
		tb.Fatal(navErr)
	}
	return navShared
}

// buildTestNav is the navigation over terrain t, its hierarchy built over
// the clusters keys.
func buildTestNav(t *geotest.Terrain, keys []nav.ClusterKey) (*Nav, error) {
	return buildTestNavWater(t, keys, nil)
}

// buildTestNavWater is buildTestNav with the water zones water.
func buildTestNavWater(t *geotest.Terrain, keys []nav.ClusterKey, water *nav.Water) (*Nav, error) {
	dir, err := os.MkdirTemp("", "botnav")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path, err := t.WriteL2OFF(dir)
	if err != nil {
		return nil, err
	}
	decoded, _, err := reader.ReadL2OFF(path)
	if err != nil {
		return nil, err
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		return nil, err
	}
	region, err := block.ParseRegion(stored.Bytes())
	if err != nil {
		return nil, err
	}
	e := engine.New()
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		return nil, err
	}
	key := nav.NavKey{ClusterCells: navTestCluster, WaterHash: water.Hash()}
	image, _, err := nav.Build(context.Background(), nav.BuildConfig{Engine: e, Water: water, Clusters: keys, ClusterCells: navTestCluster, Workers: 2, Key: key})
	if err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, "nav.bin")
	if err := nav.WriteNavCache(cache, image); err != nil {
		return nil, err
	}
	// The graph maps the file; it stays mapped after the directory goes.
	g, err := nav.OpenNavCache(cache, key)
	if err != nil {
		return nil, err
	}
	return &Nav{Engine: e, Hier: nav.NewHierarchy(g, e, water, nav.DefaultMemberCacheBytes), Water: water}, nil
}

const navTestCluster = nav.DefaultClusterCells

// navWindow lists the clusters of region-local cluster columns cx0..cx1 and
// rows cy0..cy1 of the fixture region.
func navWindow(cx0, cy0, cx1, cy1 int) []nav.ClusterKey {
	cells := block.RegionBlocksX * block.CellsX
	ox, oy := (geotest.RegionX-engine.TileXMin)*cells, (geotest.RegionY-engine.TileYMin)*cells
	var keys []nav.ClusterKey
	for y := cy0; y <= cy1; y++ {
		for x := cx0; x <= cx1; x++ {
			keys = append(keys, nav.ClusterKey{X: int32(ox/navTestCluster + x), Y: int32(oy/navTestCluster + y)})
		}
	}
	return keys
}

// scenePoint is fixture spot p as a world point, moved by dx, dy cells.
func scenePoint(p geotest.Point, dx, dy int) Point {
	x, y, z := p.World()
	return Point{X: int32(x + dx*16), Y: int32(y + dy*16), Z: int32(z)}
}

// newNavBrain is a started brain at t0 walking on n, standing at pos, its
// counters in a registry of its own.
func newNavBrain(t *testing.T, n *Nav, p Persona, pos Point) (*Brain, *recordSender, *botmetrics.Registry) {
	t.Helper()
	w := newTestWorld(t)
	w.Self.HP, w.Self.MaxHP, w.Self.MP, w.Self.MaxMP = 100, 100, 100, 100
	w.Self.move = stopAt(pos)
	reg := botmetrics.NewRegistry()
	out := &recordSender{}
	b := NewBrain(w, NewEvents(64), out, newFakeClock(t0), BrainConfig{
		Persona: p, Rand: rand.New(rand.NewPCG(1, 2)), Nav: n, Metrics: reg.NewBot(),
	})
	b.start(t0)
	return b, out, reg
}

// segAnswer is the server's MoveToLocation for the bot's segment from origin to
// dest: the click's height plus the collision height, as moveLivePlayer
// answers an open straight click.
func segAnswer(origin, dest Point) MoveToLocation {
	dest.Z += defaultCollisionHeight
	return MoveToLocation{ObjectID: selfID, Origin: origin, Dest: dest}
}

// runFor is the time a segment from a to b takes at the test speed, plus
// one position tick.
func runFor(a, b Point) time.Duration {
	return time.Duration(float64(time.Second)*math.Sqrt(float64(horizontal2(a, b)))/100) + positionTick
}

// walkOn has brain b walk on the shared synthetic navigation, as NewBrain
// sets it up from BrainConfig.Nav: the consumer tests' points along the X
// axis east of the origin lie on its open flat ground at height 0.
func walkOn(t *testing.T, b *Brain) *Brain {
	t.Helper()
	n := testNav(t)
	b.walk = newWalker(n)
	b.world.SetGround(n.Engine)
	return b
}

// TestWalkOnOrigin pins the ground walkOn puts the consumer tests on: the
// origin and the points east of it are walkable at height 0 in one SCC.
func TestWalkOnOrigin(t *testing.T) {
	n := testNav(t)
	w := newWalker(n)
	var first nav.Member
	for i, p := range []Point{{}, {X: 300}, {X: 3000}, {X: 5000}, {X: 8000}} {
		c, ok := w.plan.Locate(toLoc(p))
		if !ok || c.Z != 0 {
			t.Fatalf("%+v: cell %+v %v, want walkable at 0", p, c, ok)
		}
		m, err := n.Hier.Locate(c, nil)
		if err != nil || m.Kind() != nav.InSCC {
			t.Fatalf("%+v: member %v %v, want in an SCC", p, m, err)
		}
		if i == 0 {
			first = m
		} else if !n.Hier.SameSCC(first, m) {
			t.Fatalf("%+v not in the origin's SCC", p)
		}
	}
}
