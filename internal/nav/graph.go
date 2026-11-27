package nav

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/fatal10110/acis_golang_bot/internal/mmap"
)

// The nav cache file (`bots navbuild`), all integers little-endian:
//
//	header    magic "ACISNAV\0", version u32, CRC-32C u32 of every byte after
//	          the CRC field, then the key (geodata cache CRC u32, cluster
//	          side u32, doors.xml hash 8 bytes), the section counts
//	          (clusters, nodes, edges, door refs, SCCs, condensation edges,
//	          u32 each), the water hash (8 bytes), the landmark count
//	          u32 and the covered cost u32 (BuildConfig.CoveredCost, the
//	          extra of a step out of covered ground), navHeaderSize in all
//	clusters  x i32, y i32, first node u32; sorted by (y, x)
//	nodes     representative cell x i32, y i32, z i16, border side u8, pad
//	          u8, run first and last position u16 each, cluster u32, SCC
//	          u32, first edge u32
//	edges     target node u32, cost with every door open u32, cost crossing
//	          no door u32 (noCost when none), first door ref u32, door count
//	          u32; by target per node. An edge between two SCCs (one way:
//	          a crossing, or a pair joined inside a cluster) costs its
//	          straight line; a pair joined inside a cluster has no doors,
//	          a crossing keeps its door set
//	doorRefs  door id i32: the doors an edge's all-open path crosses
//	sccEdges  first condensation edge u32 per SCC, then the edge count
//	condEdges target SCC u32
//	landmarks node u32 per landmark, then per node its distance from each
//	          landmark u32, then to each u32, over the all-open costs
//	          (noDist when there is no way); at most navLandmarks
//	          (docs/navigation.md, "Landmarks")
//
// The graph is read in place from the read-only mapping; nothing is decoded
// onto the heap. A file built for another geodata cache, doors.xml, cluster
// side or water is stale and refused, as is a corrupt one.
const (
	navMagic       = "ACISNAV\x00"
	navVersion     = 7
	navHeaderSize  = 72
	clusterRecSize = 12
	nodeRecSize    = 28
	edgeRecSize    = 20
	// maxCoveredCost bounds BuildConfig.CoveredCost: ten cells per step
	// out of covered ground, past which a route inside floods the cell
	// search's budget long before it ends (docs/navigation.md, "Covered
	// ground").
	maxCoveredCost = 100
	// noCost marks an edge with no door-free way.
	noCost = math.MaxUint32
	// MaxNavCacheBytes bounds the nav cache file a process maps: about four
	// times the 113 MB N3 measured for the owner's 139 regions
	// (docs/navigation.md, "Measured by N3"). A larger file is refused.
	MaxNavCacheBytes = 512 << 20
)

var navCRC = crc32.MakeTable(crc32.Castagnoli)

// Errors opening a nav cache. Both stop walking personas at start; the fix
// is `bots navbuild`, never a silent rebuild (docs/navigation.md, "Build").
var (
	ErrNavCacheMissing = errors.New("nav: nav cache missing; run `bots navbuild`")
	ErrNavCacheStale   = errors.New("nav: nav cache is stale or corrupt; run `bots navbuild`")
)

// NavKey is what a nav cache is built for: the geodata cache's CRC, a hash
// of doors.xml, the build parameters and the water zones' Hash.
type NavKey struct {
	GeoCRC       uint32
	DoorsHash    [8]byte
	ClusterCells int
	WaterHash    [8]byte
}

// DoorsHash returns the doors.xml half of the nav cache key: the first 8
// bytes of the file's SHA-256.
func DoorsHash(doorsXML []byte) [8]byte {
	sum := sha256.Sum256(doorsXML)
	return [8]byte(sum[:8])
}

// Graph is the abstract graph of the hierarchy: clusters, entrance nodes,
// directed edges with their two costs and door sets, each node's SCC and the
// condensation. It reads a nav cache image in place and is immutable, so any
// number of goroutines may query it.
type Graph struct {
	b       []byte
	mapping []byte // the mapped file, nil for an in-memory image
	key     NavKey
	counts  graphCounts
	// covered is the extra cost of a step out of covered ground the graph
	// was built with (BuildConfig.CoveredCost).
	covered int32
	// section offsets
	clusters, nodes, edges, doorRefs, sccEdges, condEdges int
	landmarkIDs, landmarkDist                             int
}

type graphCounts struct {
	clusters, nodes, edges, doorRefs, sccs, condEdges, landmarks int
}

// parseGraph checks a nav cache image and returns the graph reading it.
func parseGraph(b []byte) (*Graph, error) {
	if len(b) < navHeaderSize || string(b[:8]) != navMagic {
		return nil, errors.New("not a nav cache")
	}
	if v := binary.LittleEndian.Uint32(b[8:]); v != navVersion {
		return nil, fmt.Errorf("format version %d, want %d", v, navVersion)
	}
	if crc32.Checksum(b[16:], navCRC) != binary.LittleEndian.Uint32(b[12:]) {
		return nil, errors.New("CRC mismatch")
	}
	g := &Graph{b: b}
	g.key.GeoCRC = binary.LittleEndian.Uint32(b[16:])
	g.key.ClusterCells = int(binary.LittleEndian.Uint32(b[20:]))
	copy(g.key.DoorsHash[:], b[24:32])
	c := &g.counts
	for i, p := range []*int{&c.clusters, &c.nodes, &c.edges, &c.doorRefs, &c.sccs, &c.condEdges} {
		*p = int(binary.LittleEndian.Uint32(b[32+4*i:]))
	}
	copy(g.key.WaterHash[:], b[56:64])
	c.landmarks = int(binary.LittleEndian.Uint32(b[64:]))
	if c.landmarks > navLandmarks {
		return nil, fmt.Errorf("%d landmarks, at most %d", c.landmarks, navLandmarks)
	}
	covered := binary.LittleEndian.Uint32(b[68:])
	if covered > maxCoveredCost {
		return nil, fmt.Errorf("covered cost %d, at most %d", covered, maxCoveredCost)
	}
	g.covered = int32(covered)
	if off := g.layout(); off != len(b) {
		return nil, fmt.Errorf("size %d, sections need %d", len(b), off)
	}
	if g.key.ClusterCells <= 0 {
		return nil, errors.New("bad cluster side")
	}
	return g, nil
}

// layout sets the section offsets from the counts and returns where the
// image ends.
func (g *Graph) layout() int {
	c := &g.counts
	off := navHeaderSize
	for _, s := range []struct {
		at   *int
		size int
	}{
		{&g.clusters, c.clusters * clusterRecSize},
		{&g.nodes, c.nodes * nodeRecSize},
		{&g.edges, c.edges * edgeRecSize},
		{&g.doorRefs, c.doorRefs * 4},
		{&g.sccEdges, (c.sccs + 1) * 4},
		{&g.condEdges, c.condEdges * 4},
		{&g.landmarkIDs, c.landmarks * 4},
		{&g.landmarkDist, c.nodes * c.landmarks * 8},
	} {
		*s.at = off
		off += s.size
	}
	return off
}

// OpenNavCache maps the nav cache at path read-only and checks it against
// key. A missing file is ErrNavCacheMissing; a stale or corrupt one is
// ErrNavCacheStale. The graph reads the mapping until Close.
func OpenNavCache(path string, key NavKey) (*Graph, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNavCacheMissing, path)
	}
	if err != nil {
		return nil, fmt.Errorf("nav: open nav cache: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("nav: open nav cache: %w", err)
	}
	if info.Size() < navHeaderSize || info.Size() > MaxNavCacheBytes {
		return nil, fmt.Errorf("%w: %s has size %d", ErrNavCacheStale, path, info.Size())
	}
	b, err := mmap.Map(f, int(info.Size()))
	if err != nil {
		return nil, fmt.Errorf("nav: map nav cache %s: %w", path, err)
	}
	g, err := parseGraph(b)
	if err == nil && g.key != key {
		err = fmt.Errorf("built for geodata cache %08x, doors %x, cluster side %d, water %x; want %08x, %x, %d, %x",
			g.key.GeoCRC, g.key.DoorsHash, g.key.ClusterCells, g.key.WaterHash, key.GeoCRC, key.DoorsHash, key.ClusterCells, key.WaterHash)
	}
	if err != nil {
		mmap.Unmap(b)
		return nil, fmt.Errorf("%w: %s: %v", ErrNavCacheStale, path, err)
	}
	mmap.AdviseRandom(b)
	g.mapping = b
	return g, nil
}

// Close unmaps the cache. Nothing may query the graph, or a Hierarchy over
// it, afterwards; a process keeps its nav cache mapped for its whole life.
func (g *Graph) Close() {
	if g.mapping != nil {
		mmap.Unmap(g.mapping)
		g.mapping, g.b = nil, nil
	}
}

// WriteNavCache writes a nav cache image to path atomically: a temporary
// file in the same directory, synced, then renamed over path.
func WriteNavCache(path string, image []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("nav: create nav cache: %w", err)
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err := f.Write(image); err != nil {
		return fmt.Errorf("nav: write nav cache: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("nav: sync nav cache: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("nav: close nav cache: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("nav: install nav cache: %w", err)
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// Key returns what the graph was built for.
func (g *Graph) Key() NavKey { return g.key }

// Nodes returns the entrance node count.
func (g *Graph) Nodes() int { return g.counts.nodes }

// Edges returns the directed abstract edge count.
func (g *Graph) Edges() int { return g.counts.edges }

// SCCs returns the abstract SCC count.
func (g *Graph) SCCs() int { return g.counts.sccs }

// CoveredCost returns the extra cost of a step out of covered ground the
// graph was built with, in tenths of a cell (BuildConfig.CoveredCost).
func (g *Graph) CoveredCost() int { return int(g.covered) }

// Clusters returns the cluster count.
func (g *Graph) Clusters() int { return g.counts.clusters }

func (g *Graph) u32(off int) uint32 { return binary.LittleEndian.Uint32(g.b[off:]) }

// cluster returns the index of the cluster holding cell (x, y), or -1 when
// the graph has no such cluster.
func (g *Graph) cluster(x, y int) int {
	size := g.key.ClusterCells
	k := ClusterKey{int32(floorDiv(x, size)), int32(floorDiv(y, size))}
	n := g.counts.clusters
	i := sort.Search(n, func(i int) bool {
		c := g.clusterKey(i)
		return c.Y > k.Y || c.Y == k.Y && c.X >= k.X
	})
	if i < n && g.clusterKey(i) == k {
		return i
	}
	return -1
}

func (g *Graph) clusterKey(i int) ClusterKey {
	off := g.clusters + i*clusterRecSize
	return ClusterKey{int32(g.u32(off)), int32(g.u32(off + 4))}
}

// clusterOrigin returns the top-left cell of cluster i.
func (g *Graph) clusterOrigin(i int) (int, int) {
	k := g.clusterKey(i)
	return int(k.X) * g.key.ClusterCells, int(k.Y) * g.key.ClusterCells
}

// clusterNodes returns the node range [first, end) of cluster i.
func (g *Graph) clusterNodes(i int) (int, int) {
	first := int(g.u32(g.clusters + i*clusterRecSize + 8))
	end := g.counts.nodes
	if i+1 < g.counts.clusters {
		end = int(g.u32(g.clusters + (i+1)*clusterRecSize + 8))
	}
	return first, end
}

// nodeRec is one node record's fields.
type nodeRec struct {
	rep        Cell
	side       int
	pos0, pos1 int
	cluster    int
	scc        int32
}

func (g *Graph) node(n int) nodeRec {
	off := g.nodes + n*nodeRecSize
	b := g.b[off:]
	return nodeRec{
		rep:     Cell{X: int(int32(binary.LittleEndian.Uint32(b))), Y: int(int32(binary.LittleEndian.Uint32(b[4:]))), Z: int16(binary.LittleEndian.Uint16(b[8:]))},
		side:    int(b[10]),
		pos0:    int(binary.LittleEndian.Uint16(b[12:])),
		pos1:    int(binary.LittleEndian.Uint16(b[14:])),
		cluster: int(binary.LittleEndian.Uint32(b[16:])),
		scc:     int32(binary.LittleEndian.Uint32(b[20:])),
	}
}

func (g *Graph) nodeRep(n int) Cell {
	b := g.b[g.nodes+n*nodeRecSize:]
	return Cell{X: int(int32(binary.LittleEndian.Uint32(b))), Y: int(int32(binary.LittleEndian.Uint32(b[4:]))), Z: int16(binary.LittleEndian.Uint16(b[8:]))}
}

func (g *Graph) nodeCluster(n int) int32 { return int32(g.u32(g.nodes + n*nodeRecSize + 16)) }

func (g *Graph) nodeSCC(n int) int32 { return int32(g.u32(g.nodes + n*nodeRecSize + 20)) }

// nodeEdges returns the edge range [first, end) out of node n.
func (g *Graph) nodeEdges(n int) (int, int) {
	first := int(g.u32(g.nodes + n*nodeRecSize + 24))
	end := g.counts.edges
	if n+1 < g.counts.nodes {
		end = int(g.u32(g.nodes + (n+1)*nodeRecSize + 24))
	}
	return first, end
}

// edgeRec is one edge record's fields.
type edgeRec struct {
	to                int
	open, free        uint32
	firstDoor, nDoors int
}

func (g *Graph) edge(e int) edgeRec {
	off := g.edges + e*edgeRecSize
	return edgeRec{
		to:        int(g.u32(off)),
		open:      g.u32(off + 4),
		free:      g.u32(off + 8),
		firstDoor: int(g.u32(off + 12)),
		nDoors:    int(g.u32(off + 16)),
	}
}

// door returns door ref i.
func (g *Graph) door(i int) int { return int(int32(g.u32(g.doorRefs + 4*i))) }

// edgeCost returns e's cost under the door state closed (nil: every door
// open): the all-open cost unless the path it stands for crosses a closed
// door, then the door-free cost (docs/navigation.md, "Doors in the
// hierarchy"). It reports false when the edge has no way at all.
func (g *Graph) edgeCost(e edgeRec, closed func(door int) bool) (uint32, bool) {
	if closed != nil {
		for i := range e.nDoors {
			if closed(g.door(e.firstDoor + i)) {
				return e.free, e.free != noCost
			}
		}
	}
	return e.open, true
}

// condEdges returns the condensation edge range [first, end) out of SCC s.
func (g *Graph) condRange(s int32) (int, int) {
	off := g.sccEdges + 4*int(s)
	return int(g.u32(off)), int(g.u32(off + 4))
}

func (g *Graph) condTarget(i int) int32 { return int32(g.u32(g.condEdges + 4*i)) }

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
