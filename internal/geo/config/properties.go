// Package config parses Java .properties files, such as the copy of the
// server's geoengine.properties the geodata cache is built and checked from.
package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Properties holds one parsed .properties file.
type Properties struct {
	values map[string]string
}

// Parse reads .properties content: key=value lines (also accepting ':' or
// whitespace as the separator), '#'/'!' comment lines, backslash line
// continuations, and '\t'/'\n'/'\r'/'\f'/'\uXXXX' escapes in keys and
// values. Each input byte is one ISO-8859-1 character, so a non-ASCII value
// is reachable only through a \uXXXX escape; an escaped UTF-16 surrogate
// pair reads as its one character.
func Parse(r io.Reader) (*Properties, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	p := &Properties{values: make(map[string]string)}
	for _, line := range logicalLines(decodeLatin1(data)) {
		key, value, err := splitProperty(line.text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line.number, err)
		}
		p.values[key] = value
	}
	return p, nil
}

// ParseString reads .properties content (see Parse) from a string holding
// the file's bytes.
func ParseString(s string) (*Properties, error) {
	return Parse(strings.NewReader(s))
}

// LoadFile reads one .properties file.
func LoadFile(name string) (*Properties, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", name, err)
	}
	defer f.Close()

	p, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", name, err)
	}
	return p, nil
}

// Len returns the number of loaded properties.
func (p *Properties) Len() int {
	return len(p.values)
}

// Keys returns loaded property keys in sorted order.
func (p *Properties) Keys() []string {
	keys := make([]string, 0, len(p.values))
	for key := range p.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Lookup returns the raw property value.
func (p *Properties) Lookup(key string) (string, bool) {
	value, ok := p.values[key]
	return value, ok
}

func warnMissing(key string, def any) {
	slog.Warn("config property missing; using default value", "key", key, "default", def)
}

// String returns a string property or def when key is missing.
func (p *Properties) String(key, def string) string {
	if value, ok := p.Lookup(key); ok {
		return value
	}
	warnMissing(key, def)
	return def
}

// Bool returns a bool property or def when key is missing.
func (p *Properties) Bool(key string, def bool) bool {
	if value, ok := p.Lookup(key); ok {
		return strings.EqualFold(value, "true")
	}
	warnMissing(key, def)
	return def
}

// parseInt32 parses a base-10 int that must fit in 32 bits, as every int
// config value does.
func parseInt32(s string) (int, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return int(n), err
}

// Int returns an int property or def when key is missing. The value must
// fit in 32 bits.
func (p *Properties) Int(key string, def int) (int, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := parseInt32(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s as int: %w", key, err)
		}
		return n, nil
	}
	warnMissing(key, def)
	return def, nil
}

// decodeLatin1 reads every byte of data as the ISO-8859-1 character of the
// same value.
func decodeLatin1(data []byte) string {
	var out strings.Builder
	out.Grow(len(data))
	for _, b := range data {
		out.WriteRune(rune(b))
	}
	return out.String()
}

// logicalLine is one key/value entry with its continuations joined and
// leading whitespace removed; number is the 1-based line it starts on.
type logicalLine struct {
	number int
	text   string
}

// logicalLines joins backslash continuations and drops blank and comment
// lines. A '#' or '!' starts a comment only where it is the first
// character of an entry, so a comment never continues onto the next line
// and a continued line starting with one is ordinary text.
func logicalLines(data string) []logicalLine {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")

	var lines []logicalLine
	var current strings.Builder
	start := 0
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimLeftFunc(line, isSpaceRune)
		if current.Len() == 0 {
			start = i + 1
			if line != "" && (line[0] == '#' || line[0] == '!') {
				continue
			}
		}
		if hasContinuation(line) {
			current.WriteString(line[:len(line)-1])
			continue
		}
		current.WriteString(line)
		if current.Len() > 0 {
			lines = append(lines, logicalLine{number: start, text: current.String()})
		}
		current.Reset()
	}
	if current.Len() > 0 {
		lines = append(lines, logicalLine{number: start, text: current.String()})
	}
	return lines
}

func hasContinuation(line string) bool {
	slashes := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 1
}

func splitProperty(line string) (string, string, error) {
	sep := -1
	spaceSep := false
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '=' || c == ':' {
			sep = i
			break
		}
		if isSpaceByte(c) {
			sep = i
			spaceSep = true
			break
		}
	}

	keyText := line
	valueText := ""
	if sep >= 0 {
		keyText = line[:sep]
		i := sep
		if spaceSep {
			for i < len(line) && isSpaceByte(line[i]) {
				i++
			}
			if i < len(line) && (line[i] == '=' || line[i] == ':') {
				i++
			}
		} else {
			i++
		}
		for i < len(line) && isSpaceByte(line[i]) {
			i++
		}
		valueText = line[i:]
	}

	key, err := unescape(keyText)
	if err != nil {
		return "", "", fmt.Errorf("key: %w", err)
	}
	value, err := unescape(valueText)
	if err != nil {
		return "", "", fmt.Errorf("value: %w", err)
	}
	return key, value, nil
}

func unescape(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			out.WriteByte('\\')
			break
		}
		switch s[i] {
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(s) {
				return "", fmt.Errorf("short unicode escape")
			}
			r, ok := hexUnit(s[i+1 : i+5])
			if !ok {
				return "", fmt.Errorf("bad unicode escape %q", s[i+1:i+5])
			}
			i += 4
			// A high surrogate escape followed by a low one is a single
			// character; an unpaired surrogate has no UTF-8 form and
			// reads as U+FFFD.
			if utf16.IsSurrogate(r) && i+6 < len(s) && s[i+1] == '\\' && s[i+2] == 'u' {
				if low, ok := hexUnit(s[i+3 : i+7]); ok {
					if pair := utf16.DecodeRune(r, low); pair != utf8.RuneError {
						r = pair
						i += 6
					}
				}
			}
			out.WriteRune(r)
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String(), nil
}

// hexUnit reads exactly four hex digits, with no sign or prefix, as one
// UTF-16 code unit.
func hexUnit(s string) (rune, bool) {
	var r rune
	for i := range len(s) {
		c := s[i]
		switch {
		case '0' <= c && c <= '9':
			r = r<<4 | rune(c-'0')
		case 'a' <= c && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case 'A' <= c && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\f'
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\f'
}
