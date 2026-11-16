// Package botnames generates bot character names: a pronounceable base
// built from syllables plus the configurable marker that identifies a bot in
// game (docs/plan.md, D7 and "Names and human feel"). Every name matches the
// server's rule ^[A-Za-z0-9]{1,16}$ (data/manager/roster.go).
package botnames

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"regexp"
	"strings"
)

// MaxLen is the server's longest character name.
const MaxLen = 16

// MaxMarkerLen is the longest marker: it leaves at least six characters,
// one syllable or more, for the base.
const MaxMarkerLen = MaxLen - 6

// validName is the server's character-name rule (data/manager/roster.go).
var validName = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

// validMarker is an alphanumeric marker of 1 to MaxMarkerLen characters.
var validMarker = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9]{1,%d}$`, MaxMarkerLen))

// Valid reports whether the server accepts name's spelling. The server also
// refuses a name equal to an NPC's or already taken; only it can tell.
func Valid(name string) bool { return validName.MatchString(name) }

// Placement is where the marker goes in a name.
type Placement uint8

const (
	// Suffix appends the marker to the base: "AldenBot".
	Suffix Placement = iota
	// Prefix puts the marker before the base: "BotAlden".
	Prefix
)

// Marker is the text that marks a name as a bot's (D7) and where it goes.
type Marker struct {
	Text      string
	Placement Placement
}

// DefaultMarker is the marker used when none is configured: a suffix
// players read at once.
var DefaultMarker = Marker{Text: "Bot", Placement: Suffix}

// Validate reports a marker that is empty, not alphanumeric, longer than
// MaxMarkerLen or placed neither before nor after the base.
func (m Marker) Validate() error {
	if !validMarker.MatchString(m.Text) {
		return fmt.Errorf("name marker %q: want 1-%d letters or digits", m.Text, MaxMarkerLen)
	}
	if m.Placement != Suffix && m.Placement != Prefix {
		return fmt.Errorf("name marker placement %d: want prefix or suffix", m.Placement)
	}
	return nil
}

// onsets and nuclei build syllables of two to four letters: a consonant
// onset, then a vowel nucleus.
var (
	onsets = [...]string{"b", "d", "f", "g", "h", "k", "l", "m", "n", "p", "r", "s", "t", "v", "z", "br", "dr", "gr", "kr", "th", "st"}
	nuclei = [...]string{"a", "e", "i", "o", "u", "ae", "ia", "or", "an", "el"}
)

// maxSyllables bounds a base: two or three syllables, as many as fit next to
// the marker (the first always does).
const maxSyllables = 3

// Generator draws names from a random source. It is not safe for concurrent
// use: one goroutine owns it.
type Generator struct {
	marker Marker
	src    rand.Source
}

// New returns a generator that marks every name with m and draws from src.
// A seeded source (see AccountSource) yields the same names in the same
// order on every run.
func New(m Marker, src rand.Source) (*Generator, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if src == nil {
		return nil, errors.New("botnames: nil random source")
	}
	return &Generator{marker: m, src: src}, nil
}

// AccountSource is a random source seeded from the account name: an
// account's generator yields the same candidates on every run, so a bot
// finds the character it created before without storing its name.
func AccountSource(account string) rand.Source {
	h := fnv.New64a()
	_, _ = h.Write([]byte(account))
	seed := h.Sum64()
	return rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)
}

// intN draws from [0, n) with the source's raw output, so a seeded sequence
// does not depend on math/rand's sampling algorithms.
func (g *Generator) intN(n int) int { return int(g.src.Uint64() % uint64(n)) }

// Name returns a fresh marked name of at most MaxLen characters.
func (g *Generator) Name() string {
	budget := MaxLen - len(g.marker.Text)
	want := 2 + g.intN(maxSyllables-1)
	var b strings.Builder
	for range want {
		syl := onsets[g.intN(len(onsets))] + nuclei[g.intN(len(nuclei))]
		if b.Len()+len(syl) > budget {
			break
		}
		b.WriteString(syl)
	}
	base := strings.ToUpper(b.String()[:1]) + b.String()[1:]
	if g.marker.Placement == Prefix {
		return g.marker.Text + base
	}
	return base + g.marker.Text
}

// Names returns up to n names, distinct ignoring case, in the order drawn.
// It gives up after drawing 64 names per name wanted, so it may return
// fewer.
func (g *Generator) Names(n int) []string {
	out := make([]string, 0, n)
	seen := make(map[string]bool, n)
	for draws := 0; len(out) < n && draws < 64*n; draws++ {
		name := g.Name()
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}
