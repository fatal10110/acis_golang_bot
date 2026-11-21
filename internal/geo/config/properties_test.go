package config

import (
	"strings"
	"testing"
)

// TestParse pins the java.util.Properties grammar geoengine.properties is
// read with: the three separators, both comment marks, line continuations,
// escapes, and one ISO-8859-1 character per byte.
func TestParse(t *testing.T) {
	p, err := ParseString("a=1\nb : 2\nc 3\n# d=4\n! e=5\n  f = one \\\n    two\ng=\\u0041\\tB\nh=\xe9\n16_10\n")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"a": "1", "b": "2", "c": "3", "f": "one two", "g": "A\tB", "h": "é", "16_10": ""} {
		if got, ok := p.Lookup(key); !ok || got != want {
			t.Errorf("Lookup(%q) = %q, %v, want %q", key, got, ok, want)
		}
	}
	for _, key := range []string{"d", "e", "# d"} {
		if _, ok := p.Lookup(key); ok {
			t.Errorf("Lookup(%q) found a commented-out key", key)
		}
	}
	if got := strings.Join(p.Keys(), ","); got != "16_10,a,b,c,f,g,h" {
		t.Errorf("Keys() = %s", got)
	}
}

func TestTypedGetters(t *testing.T) {
	p, err := ParseString("n=42\nbig=2147483648\nword=x\nyes=TRUE\nno=1\n")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		key     string
		def     int
		want    int
		wantErr bool
	}{
		{"n", 7, 42, false},
		{"absent", 7, 7, false},
		{"big", 0, 0, true},
		{"word", 0, 0, true},
	}
	for _, tt := range tests {
		got, err := p.Int(tt.key, tt.def)
		if (err != nil) != tt.wantErr || (!tt.wantErr && got != tt.want) {
			t.Errorf("Int(%q) = %d, %v; want %d, error %v", tt.key, got, err, tt.want, tt.wantErr)
		}
	}
	if !p.Bool("yes", false) || p.Bool("no", true) || !p.Bool("absent", true) {
		t.Error("Bool() does not read a case-insensitive true, anything else as false, absent as the default")
	}
	if got := p.String("absent", "dflt"); got != "dflt" {
		t.Errorf("String(absent) = %q, want the default", got)
	}
}
