package botnames

import "testing"

// TestMarkerMarks checks which names carry the marker: at its placement
// only, in any case.
func TestMarkerMarks(t *testing.T) {
	suffix := Marker{Text: "Bot", Placement: Suffix}
	prefix := Marker{Text: "Bot", Placement: Prefix}
	tests := []struct {
		name   string
		marker Marker
		player string
		want   bool
	}{
		{"suffix", suffix, "AldenBot", true},
		{"suffix any case", suffix, "aldenbot", true},
		{"suffix at the front", suffix, "BotAlden", false},
		{"suffix inside", suffix, "AlBotden", false},
		{"marker alone", suffix, "Bot", true},
		{"shorter than the marker", suffix, "Bo", false},
		{"prefix", prefix, "BotAlden", true},
		{"prefix any case", prefix, "BOTalden", true},
		{"prefix at the end", prefix, "AldenBot", false},
		{"empty marker", Marker{}, "AldenBot", false},
		{"empty name", suffix, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.marker.Marks(tt.player); got != tt.want {
				t.Fatalf("%+v.Marks(%q) = %v, want %v", tt.marker, tt.player, got, tt.want)
			}
		})
	}
}
