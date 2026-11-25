package botclient

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// originTile is the map tile of the world origin, where walkOn puts the
// bot (TestRestartRegion: tile 20;18).
var originTile = datapack.MapTile{X: 20, Y: 18}

// newAdBrain is newLedgerBrain on walkable ground at the origin, which its
// travel map puts in restart region region, claiming its ad lines on
// board, the ledger valuables named, carrying items.
func newAdBrain(t *testing.T, board *AdBoard, region int, items ...Item) *Brain {
	t.Helper()
	b := walkOn(t, newLedgerBrain(t, items...))
	m := b.cfg.Travel
	m.regions = map[datapack.MapTile]int{originTile: region}
	m.itemNames = map[int32]string{ledgerSword: "Sword of Watershadow", ledgerStaff: "Conjuror's Staff", ledgerRecipe: "Recipe: Halberd(60%)"}
	b.cfg.Ads = board
	return b
}

// openAdRound opens a round of b at now, anchored where the bot stands.
func openAdRound(b *Brain, now time.Time) {
	b.cfg.valuables.openRound(Point{}, now)
}

// adLine is the ad line naming the carried units of each key, in order.
func adLine(t *testing.T, b *Brain, keys ...valuableKey) string {
	t.Helper()
	b.syncValuables()
	labels := make([]string, len(keys))
	for i, k := range keys {
		e := entryOf(t, b, k.template, k.enchant)
		labels[i] = b.cfg.Travel.itemLabel(k, e.carried, e.ask)
	}
	return "WTS " + strings.Join(labels, ", ") + ", pm me"
}

// decideAd decides at now through rule "valuables" and sends what it
// decided: the Trade line's text, "" when the rule held the turn silent.
func decideAd(t *testing.T, name string, b *Brain, now time.Time) []byte {
	t.Helper()
	rule, a := b.decide(now)
	if rule != "valuables" {
		t.Fatalf("%s: decide = rule %q, want valuables", name, rule)
	}
	switch a.kind {
	case actHold:
		return nil
	case actSay:
		if sent, err := b.send(a, now); err != nil || !sent {
			t.Fatalf("%s: send = %v, %v; want the line out at its decision", name, sent, err)
		}
		return a.payload
	}
	t.Fatalf("%s: decide = action %d, want a Trade line or a hold", name, a.kind)
	return nil
}

var (
	swordKey  = valuableKey{template: ledgerSword}
	staffKey  = valuableKey{template: ledgerStaff}
	recipeKey = valuableKey{template: ledgerRecipe}
)

// TestAdText checks the ad line (docs/valuables/ads.md "Lines"): labels
// packed whole into 100 UTF-16 units, and a round's lines naming the
// carried entries by score, the rest of a full line on the next.
func TestAdText(t *testing.T) {
	label := func(n int) string { return strings.Repeat("x", n) }
	tests := []struct {
		name     string
		labels   []string
		want     string
		wantUsed int
	}{
		{
			name:   "several labels",
			labels: []string{"Sword of Watershadow 173k", "Tears of Eva 174k"},
			want:   "WTS Sword of Watershadow 173k, Tears of Eva 174k, pm me", wantUsed: 2,
		},
		{name: "one label", labels: []string{"2x +5 Conjuror's Staff (D) 1.2kk"}, want: "WTS 2x +5 Conjuror's Staff (D) 1.2kk, pm me", wantUsed: 1},
		{name: "exactly 100 units", labels: []string{label(40), label(47), "y"}, want: "WTS " + label(40) + ", " + label(47) + ", pm me", wantUsed: 2},
		{name: "one unit over", labels: []string{label(40), label(48)}, want: "WTS " + label(40) + ", pm me", wantUsed: 1},
		{name: "stops at the first that does not fit", labels: []string{label(40), label(60), "y"}, want: "WTS " + label(40) + ", pm me", wantUsed: 1},
		{name: "two-byte runes count one unit", labels: []string{strings.Repeat("é", 87)}, want: "WTS " + strings.Repeat("é", 87) + ", pm me", wantUsed: 1},
		{name: "surrogate pairs count two units", labels: []string{strings.Repeat("😀", 45)}, wantUsed: 0},
		{name: "first alone too long", labels: []string{label(90)}, wantUsed: 0},
		{name: "none", wantUsed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := adText(tt.labels)
			if got != tt.want || n != tt.wantUsed {
				t.Fatalf("adText = %q, %d; want %q, %d", got, n, tt.want, tt.wantUsed)
			}
			if chatUnits(got) > chatMaxText {
				t.Fatalf("line of %d units, over %d", chatUnits(got), chatMaxText)
			}
		})
	}

	t.Run("the rest on the next line by score", func(t *testing.T) {
		b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0,
			swordUnit(101, 0), Item{ObjectID: 102, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand}, lootStack(103, ledgerRecipe, 5))
		// Three labels do not fit in one line, two do: names of 30 units.
		m := b.cfg.Travel
		for id, name := range m.itemNames {
			m.itemNames[id] = name + strings.Repeat(".", 30-len(name))
		}
		every := b.persona.Valuables.AdEvery
		openAdRound(b, t0)
		// Score V: staff 644000, recipe 366000, sword 136000, all at R 2.
		steps := []struct {
			name string
			at   time.Duration
			want string
		}{
			{"first line: the two best", 0, adLine(t, b, staffKey, recipeKey)},
			{"second line: the rest", every, adLine(t, b, swordKey)},
			{"third line: from the top again", 2 * every, adLine(t, b, staffKey, recipeKey)},
		}
		for _, st := range steps {
			if got := decideAd(t, st.name, b, at(st.at)); !bytes.Equal(got, encodeSay2(st.want, chatTrade)) {
				t.Fatalf("%s: line % x, want Trade %q", st.name, got, st.want)
			}
		}
	})

	t.Run("an entry no longer carried is not named", func(t *testing.T) {
		b := newAdBrain(t, NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour), 0, swordUnit(101, 0), lootStack(103, ledgerRecipe, 5))
		openAdRound(b, t0)
		if got, want := decideAd(t, "both", b, t0), adLine(t, b, recipeKey, swordKey); !bytes.Equal(got, encodeSay2(want, chatTrade)) {
			t.Fatalf("line % x, want %q", got, want)
		}
		b.world.Apply(ItemList{Items: []Item{lootStack(103, ledgerRecipe, 5)}}, t0)
		now := at(b.persona.Valuables.AdEvery)
		if got, want := decideAd(t, "sword sold", b, now), adLine(t, b, recipeKey); !bytes.Equal(got, encodeSay2(want, chatTrade)) {
			t.Fatalf("line after the sword left % x, want %q", got, want)
		}
	})
}

// TestAdPacing checks the ad lines' limits (docs/valuables/ads.md "Ad
// board"): rounds opened by openRound, lines out of decide through rule
// "valuables".
func TestAdPacing(t *testing.T) {
	t.Run("one line per region gap between two brains", func(t *testing.T) {
		board := NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
		a, b, c := newAdBrain(t, board, 0, swordUnit(101, 0)), newAdBrain(t, board, 0, swordUnit(101, 0)), newAdBrain(t, board, 0, swordUnit(101, 0))
		other := newAdBrain(t, board, 1, swordUnit(101, 0))
		for _, x := range []*Brain{a, b, c, other} {
			openAdRound(x, t0)
		}
		line := encodeSay2(adLine(t, a, swordKey), chatTrade)
		steps := []struct {
			name string
			b    *Brain
			at   time.Duration
			says bool
		}{
			{"a claims", a, 0, true},
			{"b at the same instant", b, 0, false},
			{"another region at the same instant", other, 0, true},
			{"b within the gap", b, 59 * time.Second, false},
			{"b at the gap", b, 60 * time.Second, true},
			{"a within its AdEvery", a, 119 * time.Second, false},
			{"a at its AdEvery", a, 120 * time.Second, true},
			{"c within the gap of a's line", c, 179 * time.Second, false},
			{"c at the gap", c, 180 * time.Second, true},
		}
		for _, st := range steps {
			got := decideAd(t, st.name, st.b, at(st.at))
			if st.says && !bytes.Equal(got, line) || !st.says && got != nil {
				t.Fatalf("%s: line % x, want says %v (% x)", st.name, got, st.says, line)
			}
		}
	})

	t.Run("no claim while the frame bucket is empty", func(t *testing.T) {
		board := NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
		b := newAdBrain(t, board, 0, swordUnit(101, 0))
		openAdRound(b, t0)
		b.pace.tokens, b.pace.filled = 0, t0
		if got := decideAd(t, "empty bucket", b, t0); got != nil {
			t.Fatalf("line % x with an empty frame bucket", got)
		}
		if !board.Free(0, t0) {
			t.Fatal("board claimed with the line held")
		}
		if got := decideAd(t, "refilled", b, at(time.Second)); got == nil {
			t.Fatal("no line once the bucket refilled")
		}
	})

	t.Run("the chat gate", func(t *testing.T) {
		board := NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
		b := newAdBrain(t, board, 0, swordUnit(101, 0))
		openAdRound(b, t0)
		b.pace.sent(gateChat, at(-time.Second))
		if got := decideAd(t, "within chatGap", b, t0); got != nil {
			t.Fatalf("line % x within chatGap of a chat line", got)
		}
		if !board.Free(0, t0) {
			t.Fatal("board claimed with the line held")
		}
		if got := decideAd(t, "at chatGap", b, at(chatGap-time.Second)); got == nil {
			t.Fatal("no line at chatGap")
		}
	})

	t.Run("AdEvery and AdLines per round", func(t *testing.T) {
		board := NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
		b := newAdBrain(t, board, 0, swordUnit(101, 0))
		p := b.persona.Valuables
		openAdRound(b, t0)
		steps := []struct {
			name string
			at   time.Duration
			says bool
		}{
			{"first line", 0, true},
			{"within AdEvery", p.AdEvery - time.Second, false},
			{"second line", p.AdEvery, true},
			{"third line", 2 * p.AdEvery, true},
			{"past AdLines", 3 * p.AdEvery, false},
			{"still past AdLines", 10 * p.AdEvery, false},
		}
		for _, st := range steps {
			if got := decideAd(t, st.name, b, at(st.at)); (got != nil) != st.says {
				t.Fatalf("%s: line % x, want says %v", st.name, got, st.says)
			}
		}
		b.cfg.valuables.endRound(at(10 * p.AdEvery))
		next := at(10*p.AdEvery + time.Second)
		openAdRound(b, next)
		if got := decideAd(t, "next round", b, next); got == nil {
			t.Fatal("no line in the next round")
		}
	})

	t.Run("the hour cap", func(t *testing.T) {
		board := NewAdBoard(DefaultAdRegionGap, 2)
		brains := []*Brain{newAdBrain(t, board, 0, swordUnit(101, 0)), newAdBrain(t, board, 1, swordUnit(101, 0)), newAdBrain(t, board, 2, swordUnit(101, 0))}
		for _, b := range brains {
			openAdRound(b, t0)
		}
		steps := []struct {
			name string
			b    *Brain
			at   time.Duration
			says bool
		}{
			{"first of the hour", brains[0], 0, true},
			{"second of the hour", brains[1], time.Minute, true},
			{"over the cap", brains[2], 2 * time.Minute, false},
			{"over the cap, other region", brains[2], 59 * time.Minute, false},
			{"the first an hour old", brains[2], time.Hour, true},
		}
		for _, st := range steps {
			openAdRound(st.b, at(st.at))
			if got := decideAd(t, st.name, st.b, at(st.at)); (got != nil) != st.says {
				t.Fatalf("%s: line % x, want says %v", st.name, got, st.says)
			}
		}
	})

	t.Run("no ad", func(t *testing.T) {
		tests := []struct {
			name  string
			setup func(b *Brain)
		}{
			{"in no region", func(b *Brain) { b.cfg.Travel.regions = nil }},
			{"no ad board", func(b *Brain) { b.cfg.Ads = nil }},
			{"disabled policy", func(b *Brain) { b.persona.Valuables = ValuablePolicy{} }},
			{"nothing carried", func(b *Brain) { b.world.Apply(ItemList{}, t0) }},
			{"away from the anchor", func(b *Brain) { b.cfg.valuables.round.anchor = Point{X: 3000} }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				board := NewAdBoard(DefaultAdRegionGap, DefaultAdProcessPerHour)
				b := newAdBrain(t, board, 0, swordUnit(101, 0))
				openAdRound(b, t0)
				tt.setup(b)
				rule, a := b.decide(t0)
				if rule != "valuables" || a.kind == actSay {
					t.Fatalf("decide = rule %q action %d, want rule valuables and no line", rule, a.kind)
				}
				if !board.Free(0, t0) || !board.Free(1, t0) {
					t.Fatal("board claimed with no line")
				}
			})
		}
	})
}
