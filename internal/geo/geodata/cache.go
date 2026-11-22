package geodata

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/mmap"
)

// A geodata cache file holds the stored form of every listed region:
//
//	header  magic [8], version u32, geodata type u32, region count u32,
//	        CRC-32C u32 of the region data followed by the region table
//	table   per region: x u32, y u32, source size u64, source mtime i64
//	        (Unix ns), stored offset u64, stored length u64
//	data    each region's stored form, in table order
//
// All integers are little-endian. The table pins the exact region list and
// each source file's size and modification time, so a cache built from any
// other list or file contents is stale and is refused, never used.
const (
	cacheMagic      = "ACISGEO\x00"
	cacheVersion    = 2
	cacheHeaderSize = 24
	cacheEntrySize  = 40
)

var cacheCRC = crc32.MakeTable(crc32.Castagnoli)

// sourceFile is one listed region file as the cache records it.
type sourceFile struct {
	region  Region
	size    int64
	modTime int64
}

// Cache is an opened geodata cache: the engine serving the listed regions
// and the read-only mapping their views read in place.
type Cache struct {
	Engine  *engine.Engine
	mapping []byte
	crc     uint32
}

// CRC returns the cache's CRC-32C, the geodata half of the nav cache's key:
// a nav cache built over any other geodata cache is stale.
func (c *Cache) CRC() uint32 { return c.crc }

// Open maps the geodata cache file at cachePath, built by `bots geopack` (or
// the server's geopack) from the listed region files in dir, and returns
// the engine serving those regions as views over the mapped bytes. Nothing
// is decoded onto the heap: a missing listed region file, or a missing,
// stale or corrupt cache, is an error, and the fix is to run `bots
// geopack`. The region files are only stat'ed, since the cache is keyed on
// each one's size and modification time. The engine's static regions are
// written here, before it is returned, and only read afterwards.
func Open(dir string, geoType GeoType, regions []Region, cachePath string, options ...engine.Options) (*Cache, error) {
	if !geoType.Valid() {
		return nil, fmt.Errorf("geodata: unknown geodata type %q", geoType)
	}
	sources, err := statSources(dir, geoType, regions)
	if err != nil {
		return nil, fmt.Errorf("geodata: listed region file: %w", err)
	}
	stored, mapping, err := openCache(cachePath, geoType, sources)
	if err != nil {
		return nil, fmt.Errorf("%w; run `bots geopack`", err)
	}
	e := engine.New(options...)
	for i, r := range regions {
		if err := e.SetRegion(r.X, r.Y, stored[i]); err != nil {
			mmap.Unmap(mapping)
			return nil, err
		}
	}
	return &Cache{Engine: e, mapping: mapping, crc: binary.LittleEndian.Uint32(mapping[20:])}, nil
}

// Close unmaps the cache. The process keeps its cache mapped for its whole
// life; Close is for tests and tools, and the engine must not be queried
// after it, since its regions point into the mapping.
func (c *Cache) Close() {
	if c.mapping != nil {
		mmap.Unmap(c.mapping)
		c.mapping = nil
	}
}

// WriteCache decodes the listed region files in dir into a new geodata
// cache file at cachePath, replacing any previous one atomically, and
// returns the stored regions' summed statistics.
func WriteCache(cachePath, dir string, geoType GeoType, regions []Region, log *slog.Logger) (block.RegionStats, error) {
	if !geoType.Valid() {
		return block.RegionStats{}, fmt.Errorf("geodata: unknown geodata type %q", geoType)
	}
	sources, err := statSources(dir, geoType, regions)
	if err != nil {
		return block.RegionStats{}, err
	}
	return writeCache(cachePath, dir, geoType, sources, log)
}

func statSources(dir string, geoType GeoType, regions []Region) ([]sourceFile, error) {
	sources := make([]sourceFile, len(regions))
	for i, r := range regions {
		info, err := os.Stat(RegionPath(dir, geoType, r.X, r.Y))
		if err != nil {
			return nil, err
		}
		sources[i] = sourceFile{region: r, size: info.Size(), modTime: info.ModTime().UnixNano()}
	}
	return sources, nil
}

func geoTypeCode(geoType GeoType) uint32 {
	if geoType == L2J {
		return 2
	}
	return 1
}

func writeCache(cachePath, dir string, geoType GeoType, sources []sourceFile, log *slog.Logger) (stats block.RegionStats, err error) {
	f, err := os.CreateTemp(filepath.Dir(cachePath), filepath.Base(cachePath)+".tmp*")
	if err != nil {
		return stats, fmt.Errorf("geodata: create geodata cache: %w", err)
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()

	head := make([]byte, cacheHeaderSize+len(sources)*cacheEntrySize)
	if _, err := f.Write(head); err != nil {
		return stats, fmt.Errorf("geodata: write geodata cache: %w", err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	crc := uint32(0)
	off := uint64(len(head))
	for i, src := range sources {
		path := RegionPath(dir, geoType, src.region.X, src.region.Y)
		region, trailing, err := readRegion(path, geoType)
		if err != nil {
			return stats, fmt.Errorf("geodata: load region %s: %w", path, err)
		}
		if trailing > 0 {
			log.Warn("geodata region has trailing bytes", "region_x", src.region.X, "region_y", src.region.Y, "trailing_bytes", trailing)
		}
		n, err := region.WriteTo(crcWriter{w: w, crc: &crc})
		if err != nil {
			return stats, fmt.Errorf("geodata: write geodata cache: %w", err)
		}
		e := head[cacheHeaderSize+i*cacheEntrySize:]
		binary.LittleEndian.PutUint32(e, uint32(src.region.X))
		binary.LittleEndian.PutUint32(e[4:], uint32(src.region.Y))
		binary.LittleEndian.PutUint64(e[8:], uint64(src.size))
		binary.LittleEndian.PutUint64(e[16:], uint64(src.modTime))
		binary.LittleEndian.PutUint64(e[24:], off)
		binary.LittleEndian.PutUint64(e[32:], uint64(n))
		off += uint64(n)
		stats.Add(region.Stats())
	}
	if err := w.Flush(); err != nil {
		return stats, fmt.Errorf("geodata: write geodata cache: %w", err)
	}

	copy(head, cacheMagic)
	binary.LittleEndian.PutUint32(head[8:], cacheVersion)
	binary.LittleEndian.PutUint32(head[12:], geoTypeCode(geoType))
	binary.LittleEndian.PutUint32(head[16:], uint32(len(sources)))
	binary.LittleEndian.PutUint32(head[20:], crc32.Update(crc, cacheCRC, head[cacheHeaderSize:]))
	if _, err := f.WriteAt(head, 0); err != nil {
		return stats, fmt.Errorf("geodata: write geodata cache: %w", err)
	}
	if err := f.Sync(); err != nil {
		return stats, fmt.Errorf("geodata: sync geodata cache: %w", err)
	}
	if err := f.Close(); err != nil {
		return stats, fmt.Errorf("geodata: close geodata cache: %w", err)
	}
	if err := os.Rename(f.Name(), cachePath); err != nil {
		return stats, fmt.Errorf("geodata: install geodata cache: %w", err)
	}
	syncDir(filepath.Dir(cachePath))
	return stats, nil
}

// syncDir makes a rename into dir durable; a platform that cannot sync a
// directory still has the renamed file.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

type crcWriter struct {
	w   *bufio.Writer
	crc *uint32
}

func (c crcWriter) Write(p []byte) (int, error) {
	*c.crc = crc32.Update(*c.crc, cacheCRC, p)
	return c.w.Write(p)
}

// openCache maps the cache file and returns its regions in sources order
// with the mapping they view, or an error when the file is missing, built
// for other sources, or corrupt.
func openCache(cachePath string, geoType GeoType, sources []sourceFile) ([]*block.Region, []byte, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, nil, fmt.Errorf("geodata: geodata cache: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("geodata: geodata cache: %w", err)
	}
	tableEnd := cacheHeaderSize + len(sources)*cacheEntrySize
	if info.Size() < int64(tableEnd) {
		return nil, nil, fmt.Errorf("geodata: geodata cache %s is truncated", cachePath)
	}

	b, err := mmap.Map(f, int(info.Size()))
	if err != nil {
		return nil, nil, fmt.Errorf("geodata: map geodata cache %s: %w", cachePath, err)
	}
	regions, err := parseCache(b, geoType, sources)
	if err != nil {
		mmap.Unmap(b)
		return nil, nil, fmt.Errorf("geodata: geodata cache %s: %w", cachePath, err)
	}
	mmap.AdviseRandom(b)
	return regions, b, nil
}

func parseCache(b []byte, geoType GeoType, sources []sourceFile) ([]*block.Region, error) {
	if string(b[:8]) != cacheMagic {
		return nil, errors.New("not a geodata cache")
	}
	if v := binary.LittleEndian.Uint32(b[8:]); v != cacheVersion {
		return nil, fmt.Errorf("format version %d, want %d", v, cacheVersion)
	}
	if binary.LittleEndian.Uint32(b[12:]) != geoTypeCode(geoType) || binary.LittleEndian.Uint32(b[16:]) != uint32(len(sources)) {
		return nil, errors.New("built for another geodata type or region list")
	}
	tableEnd := cacheHeaderSize + len(sources)*cacheEntrySize
	type span struct{ off, n uint64 }
	spans := make([]span, len(sources))
	for i, src := range sources {
		e := b[cacheHeaderSize+i*cacheEntrySize:]
		if binary.LittleEndian.Uint32(e) != uint32(src.region.X) || binary.LittleEndian.Uint32(e[4:]) != uint32(src.region.Y) {
			return nil, errors.New("built for another region list")
		}
		if binary.LittleEndian.Uint64(e[8:]) != uint64(src.size) || binary.LittleEndian.Uint64(e[16:]) != uint64(src.modTime) {
			return nil, fmt.Errorf("region %d_%d source file changed", src.region.X, src.region.Y)
		}
		spans[i] = span{off: binary.LittleEndian.Uint64(e[24:]), n: binary.LittleEndian.Uint64(e[32:])}
		if spans[i].off < uint64(tableEnd) || spans[i].off > uint64(len(b)) || spans[i].n > uint64(len(b))-spans[i].off {
			return nil, fmt.Errorf("region %d_%d lies outside the file", src.region.X, src.region.Y)
		}
	}
	crc := crc32.Update(crc32.Checksum(b[tableEnd:], cacheCRC), cacheCRC, b[cacheHeaderSize:tableEnd])
	if crc != binary.LittleEndian.Uint32(b[20:]) {
		return nil, errors.New("checksum mismatch")
	}

	regions := make([]*block.Region, len(sources))
	for i, s := range spans {
		r, err := block.ParseRegion(b[s.off : s.off+s.n : s.off+s.n])
		if err != nil {
			return nil, fmt.Errorf("region %d_%d: %w", sources[i].region.X, sources[i].region.Y, err)
		}
		regions[i] = r
	}
	return regions, nil
}
