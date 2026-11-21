package geodata

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// writeMixedL2OFF writes a full-size L2OFF region whose blocks cycle through
// flat, complex and multilayer layouts, block 0 a flat floor at height.
func writeMixedL2OFF(t *testing.T, dir string, tileX, tileY int, height int16) {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(tileX), uint64(tileY)))
	buf := make([]byte, 18, 18+block.RegionBlockCount*64)
	u16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }
	for i := range block.RegionBlockCount {
		switch i % 3 {
		case 0:
			u16(0x0000)
			u16(uint16(height))
			u16(0)
		case 1:
			u16(0x0040)
			for range block.CellCount {
				u16(uint16(rng.Uint32()))
			}
		default:
			u16(0x0001)
			for range block.CellCount {
				n := 1 + rng.IntN(3)
				u16(uint16(n))
				for range n {
					u16(uint16(rng.Uint32()))
				}
			}
		}
	}
	if err := os.WriteFile(RegionPath(dir, L2OFF, tileX, tileY), buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func storedBytes(t *testing.T, r *block.Region) []byte {
	t.Helper()
	var b bytes.Buffer
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// scenesL2OFF is the fixture scenes region file, mostly flat and so cheap to
// pack.
var scenesL2OFF = sync.OnceValue(func() []byte { return geotest.Scenes().L2OFF() })

// packFixture writes n regions into a temp dir, packs them, and returns the
// dir, the region list and the cache path. Mixed regions cycle every block
// through flat, complex and multilayer; the others are the fixture scenes.
func packFixture(t *testing.T, n int, mixed bool) (string, []Region, string) {
	t.Helper()
	dir := t.TempDir()
	var regions []Region
	for i := range n {
		r := Region{X: 16 + i, Y: 10}
		if mixed {
			writeMixedL2OFF(t, dir, r.X, r.Y, int16(100*i))
		} else if err := os.WriteFile(RegionPath(dir, L2OFF, r.X, r.Y), scenesL2OFF(), 0o600); err != nil {
			t.Fatal(err)
		}
		regions = append(regions, r)
	}
	cachePath := filepath.Join(dir, "geodata.cache")
	if _, err := WriteCache(cachePath, dir, L2OFF, regions, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("WriteCache(): %v", err)
	}
	return dir, regions, cachePath
}

func mustOpen(t *testing.T, dir string, regions []Region, cachePath string) *Cache {
	t.Helper()
	c, err := Open(dir, L2OFF, regions, cachePath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestOpenServesTheRegionFilesFromTheMapping pins that the mapped cache
// answers exactly as the region files decoded onto the heap, and that every
// region's stored form is a view into the mapping, never a copy.
func TestOpenServesTheRegionFilesFromTheMapping(t *testing.T) {
	dir, regions, cachePath := packFixture(t, 2, true)
	sources, err := statSources(dir, L2OFF, regions)
	if err != nil {
		t.Fatal(err)
	}
	stored, mapping, err := openCache(cachePath, L2OFF, sources)
	if err != nil {
		t.Fatalf("openCache(): %v", err)
	}
	t.Cleanup(func() { unmapFile(mapping) })

	lo := uintptr(unsafe.Pointer(unsafe.SliceData(mapping)))
	hi := lo + uintptr(len(mapping))
	for i, r := range regions {
		heap, trailing, err := readRegion(RegionPath(dir, L2OFF, r.X, r.Y), L2OFF)
		if err != nil || trailing != 0 {
			t.Fatalf("readRegion(%v) = trailing %d, err %v", r, trailing, err)
		}
		view := stored[i].Stored()
		if p := uintptr(unsafe.Pointer(unsafe.SliceData(view))); p < lo || p+uintptr(len(view)) > hi {
			t.Errorf("region %v stored form at %#x is outside the mapping [%#x, %#x)", r, p, lo, hi)
		}
		if !bytes.Equal(view, storedBytes(t, heap)) {
			t.Errorf("region %v: mapped stored form differs from the decoded region file", r)
		}
	}
	c := mustOpen(t, dir, regions, cachePath)
	x, y := engine.WorldX((regions[1].X-engine.TileXMin)*2048+4), engine.WorldY((regions[1].Y-engine.TileYMin)*2048+4)
	if got := c.Engine.Height(x, y, 0); got != 100 {
		t.Errorf("Height on region %v block 0 = %d, want its flat floor 100", regions[1], got)
	}
}

// TestOpenRefusesWithoutAUsableCache pins D13: no cache, a stale one, or a
// corrupt one is an error telling to run bots geopack, never a decode onto
// the heap.
func TestOpenRefusesWithoutAUsableCache(t *testing.T) {
	tests := []struct {
		name  string
		spoil func(t *testing.T, dir string, regions []Region, cachePath string) ([]Region, string)
		want  string
	}{
		{"missing cache", func(t *testing.T, dir string, regions []Region, _ string) ([]Region, string) {
			return regions, filepath.Join(dir, "absent.cache")
		}, "run `bots geopack`"},
		{"source mtime changed", func(t *testing.T, dir string, regions []Region, cachePath string) ([]Region, string) {
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(RegionPath(dir, L2OFF, regions[0].X, regions[0].Y), later, later); err != nil {
				t.Fatal(err)
			}
			return regions, cachePath
		}, "source file changed"},
		{"other region list", func(t *testing.T, _ string, regions []Region, cachePath string) ([]Region, string) {
			return regions[:1], cachePath
		}, "region list"},
		{"corrupt byte", func(t *testing.T, _ string, regions []Region, cachePath string) ([]Region, string) {
			b, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			b[len(b)-1] ^= 0xff
			if err := os.WriteFile(cachePath, b, 0o600); err != nil {
				t.Fatal(err)
			}
			return regions, cachePath
		}, "checksum mismatch"},
		{"truncated", func(t *testing.T, _ string, regions []Region, cachePath string) ([]Region, string) {
			if err := os.Truncate(cachePath, 30); err != nil {
				t.Fatal(err)
			}
			return regions, cachePath
		}, "truncated"},
		{"missing region file", func(t *testing.T, dir string, regions []Region, cachePath string) ([]Region, string) {
			if err := os.Remove(RegionPath(dir, L2OFF, regions[1].X, regions[1].Y)); err != nil {
				t.Fatal(err)
			}
			return regions, cachePath
		}, "listed region file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, regions, cachePath := packFixture(t, 2, false)
			regions, cachePath = tt.spoil(t, dir, regions, cachePath)
			c, err := Open(dir, L2OFF, regions, cachePath)
			if err == nil {
				c.Close()
				t.Fatal("Open() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open() error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestOpenGrowsTheHeapByLittle pins that opening a cache keeps the cell data
// off the Go heap: the heap grows by far less than the file's size.
func TestOpenGrowsTheHeapByLittle(t *testing.T) {
	dir, regions, cachePath := packFixture(t, 3, true)
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	const bound = 256 << 10
	if info.Size() < 4*bound {
		t.Fatalf("fixture cache is %d bytes, too small to tell a copy from a view", info.Size())
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	c := mustOpen(t, dir, regions, cachePath)
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(c)

	if grown := int64(after.HeapAlloc) - int64(before.HeapAlloc); grown > bound {
		t.Errorf("Open() grew HeapAlloc by %d bytes for a %d-byte cache, want at most %d", grown, info.Size(), bound)
	}
}

// TestWriteCacheReplacesAtomically pins that rebuilding writes a new file
// and renames it over the old one, so a mapping of the old cache keeps
// reading the old bytes. Windows refuses to replace a mapped file, so there
// the rebuild fails and the old cache stays in place and usable.
func TestWriteCacheReplacesAtomically(t *testing.T) {
	dir, regions, cachePath := packFixture(t, 1, false)
	old := mustOpen(t, dir, regions, cachePath)
	want := append([]byte(nil), old.mapping...)

	_, err := WriteCache(cachePath, dir, L2OFF, regions, slog.New(slog.DiscardHandler))
	if runtime.GOOS == "windows" {
		if err == nil {
			t.Fatal("WriteCache() over a mapped cache on Windows succeeded, want the rename refused")
		}
	} else if err != nil {
		t.Fatalf("WriteCache(): %v", err)
	}
	if !bytes.Equal(old.mapping, want) {
		t.Error("the old mapping changed under a rebuild")
	}
	mustOpen(t, dir, regions, cachePath)
	if matches, _ := filepath.Glob(cachePath + ".tmp*"); len(matches) != 0 {
		t.Errorf("rebuild left temporary files %v", matches)
	}
}

// TestWriteCacheWarnsOnTrailingBytes pins that a region file with bytes
// after its last block still packs, with a warning naming the region.
func TestWriteCacheWarnsOnTrailingBytes(t *testing.T) {
	dir := t.TempDir()
	writeMixedL2OFF(t, dir, 16, 10, 0)
	f, err := os.OpenFile(RegionPath(dir, L2OFF, 16, 10), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	if _, err := WriteCache(filepath.Join(dir, "geodata.cache"), dir, L2OFF, []Region{{X: 16, Y: 10}}, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatalf("WriteCache(): %v", err)
	}
	if !strings.Contains(logs.String(), "trailing_bytes=3") {
		t.Errorf("logs = %q, want a trailing_bytes=3 warning", logs.String())
	}
}
