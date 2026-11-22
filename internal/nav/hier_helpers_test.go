package nav

import (
	"context"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/door"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// regionCells is the side of one region in cells.
const regionCells = block.RegionBlocksX * block.CellsX

// regionOrigin returns the geodata cell of the test region's top-left cell.
func regionOrigin() (int, int) {
	return (geotest.RegionX - engine.TileXMin) * regionCells, (geotest.RegionY - engine.TileYMin) * regionCells
}

// geoCell returns the geodata cell of region-local cell (x, y) on floor z.
func geoCell(x, y int, z int16) Cell {
	ox, oy := regionOrigin()
	return Cell{X: ox + x, Y: oy + y, Z: z}
}

func pointCell(p geotest.Point) Cell { return geoCell(p.CellX, p.CellY, int16(p.Z)) }

// window returns the clusters of side size covering region-local cluster
// columns cx0..cx1 and rows cy0..cy1.
func window(size, cx0, cy0, cx1, cy1 int) []ClusterKey {
	ox, oy := regionOrigin()
	var keys []ClusterKey
	for y := cy0; y <= cy1; y++ {
		for x := cx0; x <= cx1; x++ {
			keys = append(keys, ClusterKey{int32(ox/size + x), int32(oy/size + y)})
		}
	}
	return keys
}

// testHier is a hierarchy built over some clusters of a synthetic region.
type testHier struct {
	h     *Hierarchy
	g     *Graph
	image []byte
	stats BuildStats
	cfg   BuildConfig
}

func buildHier(tb testing.TB, e *engine.Engine, size int, doors []*door.Template, keys []ClusterKey) *testHier {
	tb.Helper()
	cfg := BuildConfig{Engine: e, Doors: doors, Clusters: keys, ClusterCells: size, Workers: 2}
	image, stats, err := Build(context.Background(), cfg)
	if err != nil {
		tb.Fatal(err)
	}
	g, err := parseGraph(image)
	if err != nil {
		tb.Fatal(err)
	}
	return &testHier{h: NewHierarchy(g, e, cfg.Water, DefaultMemberCacheBytes), g: g, image: image, stats: stats, cfg: cfg}
}

// The scene windows at 32-cell clusters: the ledge, pit, bridge and pocket;
// the corridor and the U; the stair and the stacked layers.
func sceneWindows32() []ClusterKey {
	keys := window(32, 24, 8, 49, 9)
	keys = append(keys, window(32, 8, 8, 17, 9)...)
	return append(keys, window(32, 6, 31, 9, 31)...)
}

var (
	scenes32Once sync.Once
	scenes32     *testHier
	scenes256    *testHier
	scenes256Mu  sync.Mutex
)

// hierScenes32 is the hierarchy over sceneWindows32 of the shared engine.
func hierScenes32(tb testing.TB) *testHier {
	tb.Helper()
	e := testEngine(tb)
	scenes32Once.Do(func() { scenes32 = buildHier(tb, e, 32, nil, sceneWindows32()) })
	if scenes32 == nil {
		tb.Fatal("scene hierarchy failed to build")
	}
	return scenes32
}

// hierScenes256 is the hierarchy over the pit, bridge and pocket clusters at
// the default cluster side.
func hierScenes256(tb testing.TB) *testHier {
	tb.Helper()
	scenes256Mu.Lock()
	defer scenes256Mu.Unlock()
	if scenes256 == nil {
		scenes256 = buildHier(tb, testEngine(tb), DefaultClusterCells, nil, window(DefaultClusterCells, 4, 1, 6, 1))
	}
	return scenes256
}

// memberKey identifies the set of mutually reachable cells a Member is in.
type memberKey struct {
	in      bool
	scc     int32
	cluster int32
	entry   int32
}

func keyOf(m Member) memberKey {
	if m.Kind() == InSCC {
		return memberKey{in: true, scc: m.SCC()}
	}
	return memberKey{cluster: m.res.cluster, entry: m.entry}
}

// clusterCells returns every cell layer of the clusters, read through the
// static engine.
func clusterCells(e *engine.Engine, size int, keys []ClusterKey) []Cell {
	p := Planner{Engine: e.Static()}
	var g localGraph
	var cells []Cell
	for _, k := range keys {
		g.load(&p, int(k.X)*size, int(k.Y)*size, size)
		for i := range g.cells() {
			cells = append(cells, g.cell(int32(i)))
		}
	}
	return cells
}

// brute is the whole-window SCC computed by brute force over every cell of
// the built clusters.
type brute struct {
	cells   []Cell
	index   map[Cell]int32
	next    []int32 // 4 per cell
	label   []int32 // brute SCC per cell
	inSCC   []bool  // per brute SCC: holds an entrance cell
	reaches []bool  // per brute SCC: reaches an SCC holding an entrance
}

func bruteForce(e *engine.Engine, size int, keys []ClusterKey) *brute {
	p := Planner{Engine: e.Static()}
	built := map[ClusterKey]bool{}
	for _, k := range keys {
		built[k] = true
	}
	clusterOf := func(c Cell) ClusterKey { return ClusterKey{int32(c.X / size), int32(c.Y / size)} }
	b := &brute{cells: clusterCells(e, size, keys), index: map[Cell]int32{}}
	for i, c := range b.cells {
		b.index[c] = int32(i)
	}
	n := len(b.cells)
	b.next = make([]int32, 4*n)
	entrance := make([]bool, n)
	for i, c := range b.cells {
		for d, dir := range cardinals {
			b.next[4*i+d] = -1
			to, _, ok := p.cross(c, p.nswe(c), dir)
			if !ok {
				continue
			}
			j, ok := b.index[to]
			if !ok || !built[clusterOf(to)] {
				continue
			}
			b.next[4*i+d] = j
			if clusterOf(to) != clusterOf(c) {
				entrance[i], entrance[j] = true, true
			}
		}
	}
	var s sccScratch
	count := s.run(n, func(int32) int32 { return 4 }, func(v, k int32) int32 { return b.next[4*v+k] })
	b.label = s.label
	b.inSCC = make([]bool, count)
	byLabel := make([][]int32, count)
	for i := range n {
		byLabel[b.label[i]] = append(byLabel[b.label[i]], int32(i))
		if entrance[i] {
			b.inSCC[b.label[i]] = true
		}
	}
	b.reaches = make([]bool, count)
	for l := range count {
		r := b.inSCC[l]
		for _, v := range byLabel[l] {
			for k := range 4 {
				if w := b.next[4*int(v)+k]; w >= 0 && b.label[w] != int32(l) && b.reaches[b.label[w]] {
					r = true
				}
			}
		}
		b.reaches[l] = r
	}
	return b
}

// reach reports whether cell j is reachable from cell i in the window.
func (b *brute) reach(i, j int32) bool {
	seen := map[int32]bool{i: true}
	queue := []int32{i}
	for len(queue) > 0 {
		v := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if v == j {
			return true
		}
		for k := range 4 {
			if w := b.next[4*int(v)+k]; w >= 0 && !seen[w] {
				seen[w] = true
				queue = append(queue, w)
			}
		}
	}
	return false
}

// doorAt returns a door template whose footprint is region-local column x,
// rows y0..y1, as CorridorDoor shapes its own.
func doorAt(id, x, y0, y1 int) *door.Template {
	t := geotest.CorridorDoor()
	t.ID = id
	wx0, wx1 := geotest.CellWorldX(x)-8, geotest.CellWorldX(x)+7
	wy0, wy1 := geotest.CellWorldY(y0)-8, geotest.CellWorldY(y1)+7
	t.Position.X, t.Position.Y = (wx0+wx1)/2, (wy0+wy1)/2
	t.Coordinates[0].X, t.Coordinates[0].Y = wx0, wy0
	t.Coordinates[1].X, t.Coordinates[1].Y = wx1, wy0
	t.Coordinates[2].X, t.Coordinates[2].Y = wx1, wy1
	t.Coordinates[3].X, t.Coordinates[3].Y = wx0, wy1
	return t
}
