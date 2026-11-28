package charmodel

import "testing"

// TestClassRow checks the row and archetype of every class id 0-57 and
// 88-118 with a dagger and with a bow in the right hand
// (docs/pvp/rows-second.md "Tests"): each second profession with a row of
// its own to it, a third profession to its second's, a servitor line, the
// Overlord and the Warsmith to their first profession's, and the bow
// pairing TH-HK, PW-SR, AW-PR.
func TestClassRow(t *testing.T) {
	// rows are the row each class plays without a bow.
	rows := map[int32]string{
		0: "HF", 1: "WA", 2: "GL", 3: "WL", 4: "KN", 5: "PL", 6: "KN", 7: "RO", 8: "TH", 9: "TH",
		10: "HM", 11: "HW", 12: "SN", 13: "HW", 14: "HW", 15: "CL", 16: "BI", 17: "PP",
		18: "EF", 19: "EK", 20: "TK", 21: "SS", 22: "ES", 23: "PW", 24: "PW",
		25: "EM", 26: "EW", 27: "SG", 28: "EW", 29: "EO", 30: "EE",
		31: "DF", 32: "PK", 33: "SK", 34: "BD", 35: "AS", 36: "AW", 37: "AW",
		38: "DM", 39: "DW", 40: "SH", 41: "DW", 42: "SO", 43: "SE",
		44: "OF", 45: "OR", 46: "DS", 47: "MO", 48: "TY",
		49: "OM", 50: "OS", 51: "OS", 52: "WC",
		53: "WF", 54: "SC", 55: "BH", 56: "SC", 57: "SC",
		88: "GL", 89: "WL", 90: "PL", 91: "KN", 92: "TH", 93: "TH",
		94: "SN", 95: "HW", 96: "HW", 97: "BI", 98: "PP",
		99: "TK", 100: "SS", 101: "PW", 102: "PW", 103: "SG", 104: "EW", 105: "EE",
		106: "SK", 107: "BD", 108: "AW", 109: "AW", 110: "SH", 111: "DW", 112: "SE",
		113: "DS", 114: "TY", 115: "OS", 116: "WC", 117: "BH", 118: "SC",
	}
	// bowRows are the archer rows a bow makes of the dagger second rows.
	bowRows := map[string]string{"TH": "HK", "PW": "SR", "AW": "PR"}
	archetypes := map[string]Archetype{
		"HM": Caster, "EM": Caster, "DM": Caster, "HW": Caster, "EW": Caster, "DW": Caster,
		"SN": Caster, "SG": Caster, "SH": Caster,
		"CL": Healer, "EO": Healer, "SO": Healer, "BI": Healer, "PP": Healer, "EE": Healer, "SE": Healer,
		"OM": BattleCaster, "OS": BattleCaster, "WC": BattleCaster,
		"HK": Archer, "SR": Archer, "PR": Archer,
	}
	scouts := map[string]bool{"RO": true, "ES": true, "AS": true}
	for id, want := range rows {
		for _, bow := range []bool{false, true} {
			got, ok := ClassRow(id, bow)
			wantRow := Row{ID: want, Bow: bow && scouts[want]}
			if archer, ok := bowRows[want]; ok && bow {
				wantRow.ID = archer
			}
			if !ok || got != wantRow {
				t.Errorf("ClassRow(%d, bow %v) = %+v, %v, want %+v", id, bow, got, ok, wantRow)
			}
			wantArch, ok := archetypes[wantRow.ID]
			switch {
			case wantRow.Bow:
				wantArch = Archer
			case !ok:
				wantArch = Melee
			}
			if a := got.Archetype(); a != wantArch {
				t.Errorf("class %d bow %v archetype = %v, want %v", id, bow, a, wantArch)
			}
			if s := got.Shoots(); s != (wantArch == Archer) {
				t.Errorf("class %d bow %v shoots = %v, want %v", id, bow, s, wantArch == Archer)
			}
		}
	}
	for _, id := range []int32{-1, 58, 87, 119} {
		if r, ok := ClassRow(id, false); ok {
			t.Errorf("ClassRow(%d) = %+v, want none", id, r)
		}
	}
	for _, tc := range []struct {
		name string
		id   int32
		bow  bool
		want Row
	}{
		{"a third profession plays its second's: Duelist GL", 88, false, Row{ID: "GL"}},
		{"Treasure Hunter with a bow: HK", 8, true, Row{ID: "HK"}},
		{"Hawkeye with a bow: HK, no bow flag", 9, true, Row{ID: "HK"}},
		{"Hawkeye without a bow: TH", 9, false, Row{ID: "TH"}},
		{"Plains Walker with a bow: SR", 23, true, Row{ID: "SR"}},
		{"Silver Ranger without a bow: PW", 24, false, Row{ID: "PW"}},
		{"Abyss Walker with a bow: PR", 36, true, Row{ID: "PR"}},
		{"Phantom Ranger without a bow: AW", 37, false, Row{ID: "AW"}},
		{"Sagittarius with a bow: HK", 92, true, Row{ID: "HK"}},
		{"Rogue with a bow: RO's bow branch", 7, true, Row{ID: "RO", Bow: true}},
		{"Dark Avenger: KN", 6, false, Row{ID: "KN"}},
		{"Overlord: OS", 51, false, Row{ID: "OS"}},
		{"Artisan: SC", 56, false, Row{ID: "SC"}},
		{"Warsmith: SC", 57, false, Row{ID: "SC"}},
	} {
		if got, ok := ClassRow(tc.id, tc.bow); !ok || got != tc.want {
			t.Errorf("%s: ClassRow(%d, %v) = %+v, %v, want %+v", tc.name, tc.id, tc.bow, got, ok, tc.want)
		}
	}
	if r := (Row{ID: "HK"}); r.Archetype() != Archer || !r.Shoots() {
		t.Errorf("HK archetype %v shoots %v, want an archer", r.Archetype(), r.Shoots())
	}
}

// TestOwnRow checks the classes with a row of their own: the 24 second
// rows each to itself, whatever its weapon, and none for a class that
// plays an ancestor's row.
func TestOwnRow(t *testing.T) {
	own := map[int32]string{
		5: "PL", 20: "TK", 33: "SK", 21: "SS", 34: "BD", 2: "GL", 3: "WL", 46: "DS", 55: "BH",
		8: "TH", 23: "PW", 36: "AW", 9: "HK", 24: "SR", 37: "PR", 48: "TY",
		16: "BI", 17: "PP", 30: "EE", 43: "SE", 12: "SN", 27: "SG", 40: "SH", 52: "WC",
		0: "HF", 7: "RO", 54: "SC", 56: "SC",
	}
	for id, want := range own {
		if got, ok := OwnRow(id); !ok || got != (Row{ID: want}) {
			t.Errorf("OwnRow(%d) = %+v, %v, want %s", id, got, ok, want)
		}
	}
	for _, id := range []int32{6, 13, 14, 28, 41, 51, 57, 88, 92, 118, -1, 58} {
		if got, ok := OwnRow(id); ok {
			t.Errorf("OwnRow(%d) = %+v, want none", id, got)
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
