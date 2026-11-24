package datapack

import "testing"

// TestBaseClassAndRace checks the line lookups of the profession table: a
// base class is its own base, a profession of any tier roots in its line's
// base class and race, and an id outside the table is not found.
func TestBaseClassAndRace(t *testing.T) {
	tests := []struct {
		id, base, race int32
		ok             bool
	}{
		{id: 0, base: 0, race: 0, ok: true},
		{id: 16, base: 10, race: 0, ok: true},  // Bishop: Human Mystic
		{id: 105, base: 25, race: 1, ok: true}, // Eva's Saint: Elven Mystic
		{id: 43, base: 38, race: 2, ok: true},  // Shillien Elder: Dark Mystic
		{id: 116, base: 49, race: 3, ok: true}, // Doom Cryer: Orc Mystic
		{id: 57, base: 53, race: 4, ok: true},  // Warsmith: Dwarven Fighter
		{id: 58, ok: false},                    // not a profession
		{id: -1, ok: false},
	}
	for _, tt := range tests {
		base, ok := BaseClass(tt.id)
		race, rok := ClassRace(tt.id)
		if ok != tt.ok || rok != tt.ok || ok && (base != tt.base || race != tt.race) {
			t.Errorf("BaseClass(%d) = %d %v, ClassRace = %d %v; want %d %d %v", tt.id, base, ok, race, rok, tt.base, tt.race, tt.ok)
		}
	}
}
