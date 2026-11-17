package botnames

import "testing"

// TestMarkersMarks: a set marks a name any of its markers marks, each where
// it places itself; an empty set marks nothing.
func TestMarkersMarks(t *testing.T) {
	set := Markers{DefaultMarker, {Text: "X1", Placement: Prefix}}
	tests := []struct {
		name string
		ms   Markers
		in   string
		want bool
	}{
		{"default suffix", set, "AldenBot", true},
		{"second prefix", set, "x1Alden", true},
		{"prefix marker at the end", set, "AldenX1", false},
		{"unmarked", set, "Alden", false},
		{"empty set", nil, "AldenBot", false},
	}
	for _, tt := range tests {
		if got := tt.ms.Marks(tt.in); got != tt.want {
			t.Errorf("%s: Marks(%q) = %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
}
