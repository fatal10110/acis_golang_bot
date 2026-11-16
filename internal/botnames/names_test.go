package botnames

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestNameMarkerValidate(t *testing.T) {
	tests := []struct {
		name    string
		marker  Marker
		wantErr bool
	}{
		{"default", DefaultMarker, false},
		{"one char prefix", Marker{Text: "B", Placement: Prefix}, false},
		{"digits", Marker{Text: "7x7", Placement: Suffix}, false},
		{"longest", Marker{Text: strings.Repeat("Z", MaxMarkerLen)}, false},
		{"empty", Marker{}, true},
		{"too long", Marker{Text: strings.Repeat("Z", MaxMarkerLen+1)}, true},
		{"space", Marker{Text: "B t"}, true},
		{"underscore", Marker{Text: "B_t"}, true},
		{"non ascii", Marker{Text: "Bé"}, true},
		{"bad placement", Marker{Text: "Bot", Placement: 2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.marker.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if _, nerr := New(tt.marker, rand.NewPCG(1, 2)); (nerr != nil) != tt.wantErr {
				t.Fatalf("New() = %v, wantErr %v", nerr, tt.wantErr)
			}
		})
	}
}

func TestNameNilSource(t *testing.T) {
	if _, err := New(DefaultMarker, nil); err == nil {
		t.Fatal("New(nil source) succeeded")
	}
}

// TestNameLimit draws many names per marker length and placement: each is
// marked, within MaxLen and accepted by the server's rule.
func TestNameLimit(t *testing.T) {
	tests := []struct {
		name   string
		marker Marker
	}{
		{"short suffix", Marker{Text: "B"}},
		{"default", DefaultMarker},
		{"short prefix", Marker{Text: "X1", Placement: Prefix}},
		{"longest suffix", Marker{Text: "Zz34567890"}},
		{"longest prefix", Marker{Text: "Zz34567890", Placement: Prefix}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := New(tt.marker, rand.NewPCG(7, 11))
			if err != nil {
				t.Fatal(err)
			}
			longest := 0
			for range 5000 {
				name := g.Name()
				longest = max(longest, len(name))
				if !Valid(name) || len(name) > MaxLen {
					t.Fatalf("Name() = %q: not a valid name of at most %d chars", name, MaxLen)
				}
				marked := strings.HasSuffix(name, tt.marker.Text)
				if tt.marker.Placement == Prefix {
					marked = strings.HasPrefix(name, tt.marker.Text)
				}
				base := strings.TrimSuffix(name, tt.marker.Text)
				if tt.marker.Placement == Prefix {
					base = strings.TrimPrefix(name, tt.marker.Text)
				}
				if !marked || len(base) < 2 || base[0] < 'A' || base[0] > 'Z' {
					t.Fatalf("Name() = %q: want a capitalized base of 2+ letters and marker %q (%v)", name, tt.marker.Text, tt.marker.Placement)
				}
			}
			if tt.marker.Text == "Zz34567890" && longest != MaxLen {
				t.Errorf("longest name = %d chars, want the %d-char limit reached", longest, MaxLen)
			}
		})
	}
}

func TestNameValid(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"A", true},
		{"Alden7Bot", true},
		{strings.Repeat("a", MaxLen), true},
		{strings.Repeat("a", MaxLen+1), false},
		{"", false},
		{"Al den", false},
		{"Al-den", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.name); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestNameDeterministic: the same seed yields the same names, a per-account
// seed differs across accounts, and Names never repeats a name.
func TestNameDeterministic(t *testing.T) {
	draw := func(src rand.Source) []string {
		g, err := New(DefaultMarker, src)
		if err != nil {
			t.Fatal(err)
		}
		return g.Names(8)
	}
	a, b := draw(AccountSource("bot0001")), draw(AccountSource("bot0001"))
	if !slices.Equal(a, b) {
		t.Fatalf("same account: %v then %v", a, b)
	}
	if c := draw(AccountSource("bot0002")); slices.Equal(a, c) {
		t.Fatalf("two accounts drew the same names %v", a)
	}
	seen := map[string]bool{}
	for _, n := range a {
		if seen[strings.ToLower(n)] {
			t.Fatalf("Names repeated %q in %v", n, a)
		}
		seen[strings.ToLower(n)] = true
	}
	if len(a) != 8 {
		t.Fatalf("Names(8) returned %d names", len(a))
	}
}
