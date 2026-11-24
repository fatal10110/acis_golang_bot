package datapack

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// setElement is one <set name="..." val="..."/> element.
type setElement struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// attrReader reads typed values out of folded XML attributes. The first
// failure is kept in err and turns every later read into a no-op, so a
// builder reads all its fields and checks err once.
type attrReader struct {
	vals map[string]string
	err  error
}

// foldAttrs folds attrs into an attrReader keyed by local name, the last
// value winning.
func foldAttrs(attrs []xml.Attr) *attrReader {
	vals := make(map[string]string, len(attrs))
	for _, a := range attrs {
		vals[a.Name.Local] = a.Value
	}
	return &attrReader{vals: vals}
}

func (r *attrReader) fail(key string, err error) {
	if r.err == nil {
		r.err = fmt.Errorf("attribute %q: %w", key, err)
	}
}

// str returns the value at key, failing when it is absent.
func (r *attrReader) str(key string) string {
	if r.err != nil {
		return ""
	}
	v, ok := r.vals[key]
	if !ok {
		r.fail(key, errMissing)
	}
	return v
}

// strDefault returns the value at key, or def when it is absent.
func (r *attrReader) strDefault(key, def string) string {
	if v, ok := r.vals[key]; ok {
		return v
	}
	return def
}

// boolDefault returns the value at key as a bool, or def when it is
// absent. Like the server's loader (data/xml/attrvalues.go), any spelling
// but a case-insensitive "true" reads as false.
func (r *attrReader) boolDefault(key string, def bool) bool {
	raw, ok := r.vals[key]
	if !ok {
		return def
	}
	return strings.EqualFold(raw, "true")
}

// intDefault returns the base-10 value at key, or def when it is absent.
func (r *attrReader) intDefault(key string, def int) int {
	if r.err != nil {
		return def
	}
	raw, ok := r.vals[key]
	if !ok {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		r.fail(key, err)
		return def
	}
	return n
}

// int32Default returns the base-10 value at key, or def when it is
// absent; a value outside the int32 range fails the load.
func (r *attrReader) int32Default(key string, def int32) int32 {
	if r.err != nil {
		return def
	}
	raw, ok := r.vals[key]
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		r.fail(key, err)
		return def
	}
	return int32(n)
}

// int returns the required base-10 value at key.
func (r *attrReader) int(key string) int {
	if r.err != nil {
		return 0
	}
	if _, ok := r.vals[key]; !ok {
		r.fail(key, errMissing)
		return 0
	}
	return r.intDefault(key, 0)
}

// int32 returns the required base-10 value at key; a value outside the
// int32 range fails the load.
func (r *attrReader) int32(key string) int32 {
	if r.err != nil {
		return 0
	}
	raw, ok := r.vals[key]
	if !ok {
		r.fail(key, errMissing)
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		r.fail(key, err)
		return 0
	}
	return int32(n)
}

// literal returns the required integer literal at key (see decodeInt32).
func (r *attrReader) literal(key string) int32 {
	if r.err != nil {
		return 0
	}
	raw, ok := r.vals[key]
	if !ok {
		r.fail(key, errMissing)
		return 0
	}
	n, err := decodeInt32(raw)
	if err != nil {
		r.fail(key, err)
	}
	return n
}

// literalDefault returns the integer literal at key (see decodeInt32), or
// def when it is absent.
func (r *attrReader) literalDefault(key string, def int32) int32 {
	if _, ok := r.vals[key]; !ok || r.err != nil {
		return def
	}
	return r.literal(key)
}

// floatDefault returns the decimal value at key, or def when it is absent.
func (r *attrReader) floatDefault(key string, def float64) float64 {
	if r.err != nil {
		return def
	}
	raw, ok := r.vals[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		r.fail(key, err)
		return def
	}
	return f
}

// float returns the required decimal value at key.
func (r *attrReader) float(key string) float64 {
	if r.err != nil {
		return 0
	}
	if _, ok := r.vals[key]; !ok {
		r.fail(key, errMissing)
		return 0
	}
	return r.floatDefault(key, 0)
}

// floats returns the required value at key split on ";", each part a
// decimal, as the server's float64Array reads it (data/xml/attrvalues.go):
// an empty part fails.
func (r *attrReader) floats(key string) []float64 {
	raw := r.str(key)
	if r.err != nil {
		return nil
	}
	parts := strings.Split(raw, ";")
	out := make([]float64, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			r.fail(key, err)
			return nil
		}
		out[i] = f
	}
	return out
}

// ints returns the required value at key split on ";", each part a
// base-10 integer, as the server's intArray reads it (data/xml/files.go
// parseInts): an empty part fails.
func (r *attrReader) ints(key string) []int32 {
	raw := r.str(key)
	if r.err != nil {
		return nil
	}
	parts := strings.Split(raw, ";")
	out := make([]int32, len(parts))
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 32)
		if err != nil {
			r.fail(key, err)
			return nil
		}
		out[i] = int32(n)
	}
	return out
}

// duration returns the game duration at key (see parseGameDuration), or 0
// when it is absent or empty.
func (r *attrReader) duration(key string) time.Duration {
	raw := r.strDefault(key, "")
	if r.err != nil || raw == "" {
		return 0
	}
	d, err := parseGameDuration(raw)
	if err != nil {
		r.fail(key, err)
	}
	return d
}

var errMissing = errors.New("required")

// decodeInt32 parses an integer literal the way the server's data loader
// does (acis_golang internal/commons/intliteral.go DecodeInt32): an optional
// sign, then "0x"/"0X"/"#" hex, leading-zero octal, or decimal.
func decodeInt32(s string) (int32, error) {
	rest := s
	neg := false
	if rest != "" && (rest[0] == '-' || rest[0] == '+') {
		neg = rest[0] == '-'
		rest = rest[1:]
	}
	base := 10
	switch {
	case strings.HasPrefix(rest, "0x"), strings.HasPrefix(rest, "0X"):
		base, rest = 16, rest[2:]
	case strings.HasPrefix(rest, "#"):
		base, rest = 16, rest[1:]
	case len(rest) > 1 && rest[0] == '0':
		base, rest = 8, rest[1:]
	}
	// An explicit base keeps strconv from reading prefixes or "_" itself; a
	// second sign is rejected because strconv would accept it.
	if rest == "" || rest[0] == '-' || rest[0] == '+' {
		return 0, fmt.Errorf("%q: invalid integer literal", s)
	}
	n, err := strconv.ParseUint(rest, base, 32)
	if err != nil {
		return 0, fmt.Errorf("%q: invalid integer literal", s)
	}
	if neg {
		if n > 1<<31 {
			return 0, fmt.Errorf("%q: integer literal out of int32 range", s)
		}
		return int32(-int64(n)), nil
	}
	if n > 1<<31-1 {
		return 0, fmt.Errorf("%q: integer literal out of int32 range", s)
	}
	return int32(n), nil
}

// NoRespawn is the Spawn.Respawn value of respawn="no": the NPC never
// respawns once killed.
const NoRespawn = -time.Second

// parseGameDuration parses a data-file duration the way the server does
// (acis_golang internal/commons/gameduration.go ParseGameDuration): a decimal
// number followed by "hour", "min" or "sec", or "no" (any case) for
// NoRespawn. A string with none of those suffixes is 0, not an error.
func parseGameDuration(s string) (time.Duration, error) {
	if strings.EqualFold(s, "no") {
		return NoRespawn, nil
	}
	units := []struct {
		suffix string
		unit   time.Duration
	}{
		{"hour", time.Hour},
		{"min", time.Minute},
		{"sec", time.Second},
	}
	for _, u := range units {
		n, ok := strings.CutSuffix(s, u.suffix)
		if !ok {
			continue
		}
		v, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("game duration %q: %w", s, err)
		}
		return time.Duration(v) * u.unit, nil
	}
	return 0, nil
}
