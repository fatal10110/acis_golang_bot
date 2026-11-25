package charmodel

import "testing"

// TestClassRow checks the row and archetype of every class id 0-57 and
// 88-118 with a dagger and with a bow in the right hand.
func TestClassRow(t *testing.T) {
	rows := map[int32]string{
		0: "HF", 1: "WA", 2: "WA", 3: "WA", 4: "KN", 5: "KN", 6: "KN", 7: "RO", 8: "RO", 9: "RO",
		10: "HM", 11: "HW", 12: "HW", 13: "HW", 14: "HW", 15: "CL", 16: "CL", 17: "CL",
		18: "EF", 19: "EK", 20: "EK", 21: "EK", 22: "ES", 23: "ES", 24: "ES",
		25: "EM", 26: "EW", 27: "EW", 28: "EW", 29: "EO", 30: "EO",
		31: "DF", 32: "PK", 33: "PK", 34: "PK", 35: "AS", 36: "AS", 37: "AS",
		38: "DM", 39: "DW", 40: "DW", 41: "DW", 42: "SO", 43: "SO",
		44: "OF", 45: "OR", 46: "OR", 47: "MO", 48: "MO",
		49: "OM", 50: "OS", 51: "OS", 52: "OS",
		53: "WF", 54: "SC", 55: "SC", 56: "SC", 57: "SC",
		88: "WA", 89: "WA", 90: "KN", 91: "KN", 92: "RO", 93: "RO",
		94: "HW", 95: "HW", 96: "HW", 97: "CL", 98: "CL",
		99: "EK", 100: "EK", 101: "ES", 102: "ES", 103: "EW", 104: "EW", 105: "EO",
		106: "PK", 107: "PK", 108: "AS", 109: "AS", 110: "DW", 111: "DW", 112: "SO",
		113: "OR", 114: "MO", 115: "OS", 116: "OS", 117: "SC", 118: "SC",
	}
	archetypes := map[string]Archetype{
		"HM": Caster, "EM": Caster, "DM": Caster, "HW": Caster, "EW": Caster, "DW": Caster,
		"CL": Healer, "EO": Healer, "SO": Healer, "OM": BattleCaster, "OS": BattleCaster,
	}
	scouts := map[string]bool{"RO": true, "ES": true, "AS": true}
	for id, want := range rows {
		for _, bow := range []bool{false, true} {
			got, ok := ClassRow(id, bow)
			wantRow := Row{ID: want, Bow: bow && scouts[want]}
			if !ok || got != wantRow {
				t.Errorf("ClassRow(%d, bow %v) = %+v, %v, want %+v", id, bow, got, ok, wantRow)
			}
			wantArch, caster := archetypes[want]
			switch {
			case wantRow.Bow:
				wantArch = Archer
			case !caster:
				wantArch = Melee
			}
			if a := got.Archetype(); a != wantArch {
				t.Errorf("class %d bow %v archetype = %v, want %v", id, bow, a, wantArch)
			}
		}
	}
	for _, id := range []int32{-1, 58, 87, 119} {
		if r, ok := ClassRow(id, false); ok {
			t.Errorf("ClassRow(%d) = %+v, want none", id, r)
		}
	}
}

// TestArchetypeLetters checks the Vs gate letters.
func TestArchetypeLetters(t *testing.T) {
	for a, want := range map[Archetype]string{Melee: "M", Archer: "A", Caster: "C", Healer: "H", BattleCaster: "B"} {
		if got := a.String(); got != want {
			t.Errorf("%d = %q, want %q", a, got, want)
		}
	}
}
