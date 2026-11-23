package nav

import "math/bits"

// indexTable maps uint64 keys to the int32 node indexes of one search: open
// addressing with linear probing over a power-of-two table. reset clears it
// in O(1) by bumping the generation, so a search pays for the nodes it
// visits, not for the largest search the scratch ran before. It replaces a
// Go map, whose hashing of a Cell key was a third of a route's CPU (#209).
type indexTable struct {
	slots []indexSlot
	gen   uint32
	shift uint8 // 64 - log2(len(slots))
	n     int
}

// indexSlot is one table slot: it holds a key of this generation when gen
// matches the table's.
type indexSlot struct {
	key uint64
	val int32
	gen uint32
}

// bytes is the memory the table holds.
func (t *indexTable) bytes() int { return capBytes(t.slots) }

// reset empties the table.
func (t *indexTable) reset() {
	t.n = 0
	t.gen++
	if t.gen == 0 { // wrapped: an old slot could pass as current
		clear(t.slots)
		t.gen = 1
	}
}

// getOrPut returns key's value and true, or stores val under key and returns
// val and false.
func (t *indexTable) getOrPut(key uint64, val int32) (int32, bool) {
	if 2*(t.n+1) > len(t.slots) {
		t.grow()
	}
	mask := len(t.slots) - 1
	for i := t.home(key); ; i = (i + 1) & mask {
		s := &t.slots[i]
		if s.gen != t.gen {
			*s = indexSlot{key: key, val: val, gen: t.gen}
			t.n++
			return val, false
		}
		if s.key == key {
			return s.val, true
		}
	}
}

// get returns key's value and whether the table holds key.
func (t *indexTable) get(key uint64) (int32, bool) {
	if len(t.slots) == 0 {
		return 0, false
	}
	mask := len(t.slots) - 1
	for i := t.home(key); ; i = (i + 1) & mask {
		s := &t.slots[i]
		if s.gen != t.gen {
			return 0, false
		}
		if s.key == key {
			return s.val, true
		}
	}
}

// home is key's first slot: Fibonacci hashing, the top bits of the product.
func (t *indexTable) home(key uint64) int {
	return int((key * 0x9E3779B97F4A7C15) >> t.shift)
}

// grow doubles the table and re-inserts the current generation's keys.
func (t *indexTable) grow() {
	old := t.slots
	size := max(2*len(old), 1<<10)
	t.slots = make([]indexSlot, size)
	t.shift = uint8(64 - bits.Len(uint(size-1)))
	gen := t.gen
	t.gen = 1
	mask := size - 1
	for _, s := range old {
		if s.gen != gen {
			continue
		}
		i := t.home(s.key)
		for t.slots[i].gen == t.gen {
			i = (i + 1) & mask
		}
		t.slots[i] = indexSlot{key: s.key, val: s.val, gen: t.gen}
	}
}

// cellKey packs a cell into an indexTable key. Geodata coordinates fit in 24
// bits (the world is 17 × 2048 cells wide), the floor height in 16.
func cellKey(c Cell) uint64 {
	return uint64(uint32(c.X)&0xFFFFFF)<<40 | uint64(uint32(c.Y)&0xFFFFFF)<<16 | uint64(uint16(c.Z))
}
