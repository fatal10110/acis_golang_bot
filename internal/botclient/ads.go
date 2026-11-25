package botclient

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// The ad board's limits at the design's defaults (docs/valuables/preset.md
// "Process settings"): one line a region each DefaultAdRegionGap across the
// process, and DefaultAdProcessPerHour lines an hour. They are operational
// limits, not persona defaults (D6).
const (
	DefaultAdRegionGap      = 60 * time.Second
	DefaultAdProcessPerHour = 30
)

// adKeep is how long the board keeps a claim it counts toward the hour cap:
// twice the hour, so a brain whose clock lags another's never misses a claim
// the other pruned.
const adKeep = 2 * time.Hour

// AdBoard is the Trade ad lines of a process (docs/valuables/ads.md "Ad
// board"): a line goes out only when no bot of the process said one in its
// restart region within regionGap and the process said fewer than perHour
// in the last hour. Every bot of the process shares one. The limits are
// read-only after NewAdBoard; mu owns the claims.
type AdBoard struct {
	regionGap time.Duration
	perHour   int

	mu sync.Mutex
	// last is the last line claimed in each region.
	last map[int]time.Time
	// lines are the claims of the last adKeep, in claim order.
	lines []time.Time
}

// NewAdBoard returns a board of one line a region each regionGap and
// perHour lines an hour.
func NewAdBoard(regionGap time.Duration, perHour int) *AdBoard {
	return &AdBoard{regionGap: regionGap, perHour: perHour, last: make(map[int]time.Time)}
}

// Free reports whether a line in region may go out at now. It is only a
// hint: another bot may claim the line first; Claim decides. A nil board is
// never free.
func (a *AdBoard) Free(region int, now time.Time) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.freeLocked(region, now)
}

// Claim records a line in region at now and reports it may go out; none
// when a limit holds it. The line counts from its claim, sent or not. A nil
// board claims nothing.
func (a *AdBoard) Claim(region int, now time.Time) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.freeLocked(region, now) {
		return false
	}
	a.last[region] = now
	a.lines = append(a.lines, now)
	return true
}

// freeLocked reports whether a line in region may go out at now, pruning
// the claims past adKeep. A claim at or after now counts: clocks of two
// brains may differ. a.mu must be held.
func (a *AdBoard) freeLocked(region int, now time.Time) bool {
	if last, ok := a.last[region]; ok && now.Sub(last) < a.regionGap {
		return false
	}
	a.lines = slices.DeleteFunc(a.lines, func(t time.Time) bool { return now.Sub(t) >= adKeep })
	n := 0
	for _, t := range a.lines {
		if now.Sub(t) < time.Hour {
			n++
		}
	}
	return n < a.perHour
}

// adHead and adTail wrap an ad's labels (docs/valuables/ads.md "Lines").
const (
	adHead = "WTS "
	adTail = ", pm me"
)

// adText is the ad line naming the first of labels that fit in chatMaxText
// UTF-16 units, in order, and how many it names: the rest go to the next
// line (docs/valuables/ads.md "Lines"). It names none, "", when the first
// alone does not fit.
func adText(labels []string) (string, int) {
	return listText(adHead, adTail, labels)
}

// listText is head, the first of labels that fit in chatMaxText UTF-16
// units with tail, in order and comma-separated, then tail, and how many it
// names: "" and 0 when the first alone does not fit.
func listText(head, tail string, labels []string) (string, int) {
	n, units := 0, chatUnits(head)+chatUnits(tail)
	for i, l := range labels {
		u := chatUnits(l)
		if i > 0 {
			u += chatUnits(", ")
		}
		if units+u > chatMaxText {
			break
		}
		units += u
		n++
	}
	if n == 0 {
		return "", 0
	}
	return head + strings.Join(labels[:n], ", ") + tail, n
}

// chatUnits is the length of text in UTF-16 units, as the server counts it.
func chatUnits(text string) int {
	n := 0
	for _, r := range text {
		n += max(utf16.RuneLen(r), 1)
	}
	return n
}

// adEntries are the entries not released with carried units an ad names,
// by score V (then template and enchant), highest first.
func (b *Brain) adEntries() []valuableKey {
	return b.valuablesByScore(func(e *valuableEntry) bool { return e.carried > 0 && !e.released })
}

// valuablesByScore are the entries keep keeps, by score V (then template
// and enchant), highest first.
func (b *Brain) valuablesByScore(keep func(*valuableEntry) bool) []valuableKey {
	m, p := b.cfg.Travel, b.persona.Valuables
	type scored struct {
		k valuableKey
		v float64
	}
	var es []scored
	for k, e := range b.cfg.valuables.entries {
		if keep(e) {
			es = append(es, scored{k, m.valueScore(p, k.template, int(k.enchant)).v})
		}
	}
	slices.SortFunc(es, func(x, y scored) int {
		return cmp.Or(cmp.Compare(y.v, x.v), cmp.Compare(x.k.template, y.k.template), cmp.Compare(x.k.enchant, y.k.enchant))
	})
	keys := make([]valuableKey, len(es))
	for i, e := range es {
		keys[i] = e.k
	}
	return keys
}

// adLabels are the labels of the carried entries an ad may name, by score,
// and their keys: none in named, none too long for a line alone.
func (b *Brain) adLabels(named map[valuableKey]bool) ([]valuableKey, []string) {
	var keys []valuableKey
	var labels []string
	for _, k := range b.adEntries() {
		e := b.cfg.valuables.entries[k]
		l := b.cfg.Travel.itemLabel(k, e.carried, e.ask)
		if _, n := adText([]string{l}); n == 0 || named[k] {
			continue
		}
		keys, labels = append(keys, k), append(labels, l)
	}
	return keys, labels
}

// adTurn is the ad step of an open valuables round at its anchor
// (docs/valuables/ads.md): one Trade line naming the round's next carried
// entries by score, from where the last line left off and from the top once
// every entry was named; a label too long for a line alone is never named.
// It says one when the round said fewer than AdLines, none within AdEvery,
// the bot stands in a restart region, the chat gate lets a line out at now
// (frame bucket included, so send at now never holds it), and the ad board
// grants the claim; otherwise it says nothing this decision and queues
// nothing.
//
// Pacing: Say2 Trade, one per AdEvery and AdLines a round per bot (RoundGap
// between rounds), each through chatGap and the frame bucket; across the
// process one per DefaultAdRegionGap a region and DefaultAdProcessPerHour an
// hour (AdBoard). The server's TradeChatTime gate is 0 by default
// (network/chat.go chatTrade); the bot never relies on it.
func (b *Brain) adTurn(now time.Time) (Action, bool) {
	r, p := &b.cfg.valuables.round, b.persona.Valuables
	m := b.cfg.Travel
	if m == nil || b.cfg.Ads == nil || r.ads >= p.AdLines || !r.adAt.IsZero() && now.Sub(r.adAt) < p.AdEvery {
		return Action{}, false
	}
	pos := b.world.Self.Position(now)
	region := m.restartRegion(pos.X, pos.Y)
	if region < 0 {
		return Action{}, false
	}
	keys, labels := b.adLabels(r.named)
	if len(labels) == 0 {
		keys, labels = b.adLabels(nil)
		r.named = nil
	}
	text, n := adText(labels)
	if n == 0 || !b.pace.allow(gateChat, now) || !b.cfg.Ads.Claim(region, now) {
		return Action{}, false
	}
	if r.named == nil {
		r.named = make(map[valuableKey]bool)
	}
	for _, k := range keys[:n] {
		r.named[k] = true
	}
	if r.ads == 0 {
		r.first = now
	}
	r.ads++
	r.adAt = now
	b.metrics.ValuablesAd()
	return Action{kind: actSay, payload: encodeSay2(text, chatTrade)}, true
}
