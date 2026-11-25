package botclient

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// The words naming an item (docs/valuables/whispers.md "Naming an item"):
// a stop word never points at an entry, a kind word only when no
// distinctive word does.
var (
	whisperStopWords = map[string]bool{"of": true, "the": true, "and": true, "for": true, "with": true, "from": true}
	whisperKindWords = map[string]bool{"scroll": true, "enchant": true, "recipe": true, "grade": true, "weapon": true, "armor": true}
)

// whisperWordLetters is the fewest letters of a word that points at an
// entry.
const whisperWordLetters = 4

// walkerCommands are the prefixes the server drops a whisper starting with,
// case-sensitive (handler/chat/chat.go walkerCommands, isWalkerCommand).
var walkerCommands = [...]string{
	"USESKILL", "USEITEM", "BUYITEM", "SELLITEM", "SAVEITEM", "LOADITEM", "MSG", "DELAY", "LABEL",
	"JMP", "CALL", "RETURN", "MOVETO", "NPCSEL", "NPCDLG", "DLGSEL", "CHARSTATUS", "POSOUTRANGE",
	"POSINRANGE", "GOHOME", "SAY", "EXIT", "PAUSE", "STRINDLG", "STRNOTINDLG", "CHANGEWAITTYPE",
	"FORCEATTACK", "ISMEMBER", "REQUESTJOINPARTY", "REQUESTOUTPARTY", "QUITPARTY", "MEMBERSTATUS",
	"CHARBUFFS", "ITEMCOUNT", "FOLLOWTELEPORT",
}

// whisperDeal is a partner's haggle over one entry.
type whisperDeal struct {
	partner int32
	k       valuableKey
}

// whisperPrice is where a deal stands: the partner's current price of one
// unit, the counters it used, and whether the bot agreed to the current
// price ("ok <price>, trade me") since its last counter. A deal with
// neither a counter nor an agreement is an entry the partner named.
type whisperPrice struct {
	current  int64
	counters int
	agreed   bool
}

// whisperTalk is the responder's state of the latest round
// (docs/valuables/whispers.md "Pacing, queue and caps"): the replies sent
// in all and per partner, for the caps, and the deals. openRound resets
// it; endRound keeps it and sets ended, so the ended round's caps hold for
// AnswerAfter.
type whisperTalk struct {
	// ended is the latest round's end, zero while one is open and before
	// any.
	ended   time.Time
	replies int
	to      map[int32]int
	deals   map[whisperDeal]whisperPrice
}

// allows reports whether a reply to partner stays within p's caps.
func (t *whisperTalk) allows(partner int32, p ValuablePolicy) bool {
	return t.replies < p.Replies && t.to[partner] < p.RepliesPerPartner
}

// whisperWait is a whisper waiting for its reply, composed when it goes
// out.
type whisperWait struct {
	to   int32
	name string
	text string
	at   time.Time
	// wake brings a decision forward to at (decisionAt) until one runs
	// there, as chatReply's.
	wake bool
}

// whisperOut is the reply rule "chat" decided and its effect on the deal
// it answers, applied once it is sent (chatSent). to is 0 for none, deal's
// partner 0 for a reply that sets no price.
type whisperOut struct {
	to    int32
	deal  whisperDeal
	price int64
	// counter is set when the reply used a counter, agreed when it agreed
	// to price.
	counter, agreed bool
}

// whisperQueue is the responder's queue, owned by the brain goroutine:
// one reply per partner, any number of partners, ended with the session.
type whisperQueue struct {
	waits []whisperWait
	out   whisperOut
}

// put queues w, replacing the partner's waiting reply.
func (q *whisperQueue) put(w whisperWait) {
	for i := range q.waits {
		if q.waits[i].to == w.to {
			q.waits[i] = w
			return
		}
	}
	q.waits = append(q.waits, w)
}

// drop removes the partner's waiting reply.
func (q *whisperQueue) drop(partner int32) {
	q.waits = slices.DeleteFunc(q.waits, func(w whisperWait) bool { return w.to == partner })
}

// due is the index of the earliest reply due at now, -1 for none.
func (q *whisperQueue) due(now time.Time) int {
	i := -1
	for j, w := range q.waits {
		if !now.Before(w.at) && (i < 0 || w.at.Before(q.waits[i].at)) {
			i = j
		}
	}
	return i
}

// wakeAt is the earliest reply a decision is brought forward to.
func (q *whisperQueue) wakeAt() (time.Time, bool) {
	var t time.Time
	for _, w := range q.waits {
		if w.wake && (t.IsZero() || w.at.Before(t)) {
			t = w.at
		}
	}
	return t, !t.IsZero()
}

// spendWake clears the wake of every reply due at now.
func (q *whisperQueue) spendWake(now time.Time) {
	for i := range q.waits {
		if !now.Before(q.waits[i].at) {
			q.waits[i].wake = false
		}
	}
}

// answering reports whether now lies within AnswerAfter of the latest
// round's end (docs/valuables/whispers.md "After a round").
func (b *Brain) answering(now time.Time) bool {
	t := &b.cfg.valuables.talk
	return !t.ended.IsZero() && now.Sub(t.ended) < b.persona.Valuables.AnswerAfter
}

// whisperHeard takes a whisper that is no command for the valuables
// responder and reports whether it did: every one while a round is open,
// and within AnswerAfter of its end one naming a held entry or asking a
// price; the canned reply (chatHeard) gets the others. It queues the reply
// DelayMin-DelayMax later, none to a marked name or past the round's caps.
func (b *Brain) whisperHeard(e CreatureSay, now time.Time) bool {
	v := b.cfg.valuables
	if b.persona.Valuables == (ValuablePolicy{}) || b.cfg.Travel == nil || v == nil {
		return false
	}
	switch {
	case v.round.open:
	case b.answering(now):
		named, _ := b.cfg.Travel.whisperNames(e.Text, b.whisperKeys(true))
		if len(named) == 0 && !askingPrice(e.Text) {
			return false
		}
	default:
		return false
	}
	if e.Name == "" || b.cfg.markers.Marks(e.Name) || !v.talk.allows(e.ObjectID, b.persona.Valuables) {
		return true
	}
	b.whispers.put(whisperWait{to: e.ObjectID, name: e.Name, text: e.Text, at: now.Add(b.chatDelay()), wake: true})
	return true
}

// ruleWhisper decides the earliest due responder reply at now, composed
// now, and drops the ones with nothing to say: past the caps, past
// AnswerAfter, no entry to name. A canned reply due earlier goes first.
func (b *Brain) ruleWhisper(now time.Time, canned time.Time) (Action, bool) {
	for {
		i := b.whispers.due(now)
		if i < 0 || !canned.IsZero() && canned.Before(b.whispers.waits[i].at) {
			return Action{}, false
		}
		w := b.whispers.waits[i]
		text, out, ok := b.composeWhisper(w, now)
		if !ok {
			b.whispers.drop(w.to)
			continue
		}
		out.to = w.to
		b.whispers.out = out
		return chatReplyAction(w.name, text), true
	}
}

// whisperSent counts the reply o sent, frees its partner's slot and moves
// its deal.
func (b *Brain) whisperSent(o whisperOut) {
	t := &b.cfg.valuables.talk
	b.whispers.drop(o.to)
	b.whispers.out = whisperOut{}
	t.replies++
	if t.to == nil {
		t.to = make(map[int32]int)
	}
	t.to[o.to]++
	if o.deal.partner == 0 {
		return
	}
	if t.deals == nil {
		t.deals = make(map[whisperDeal]whisperPrice)
	}
	d := t.deals[o.deal]
	d.current = o.price
	if o.counter {
		d.counters++
	}
	switch {
	case o.agreed:
		d.agreed = true
	case o.counter:
		d.agreed = false
	}
	t.deals[o.deal] = d
}

// composeWhisper is the reply to w at now: in a round the haggle's, within
// AnswerAfter the after-round one; false when there is none to send.
func (b *Brain) composeWhisper(w whisperWait, now time.Time) (string, whisperOut, bool) {
	v := b.cfg.valuables
	if !v.talk.allows(w.to, b.persona.Valuables) {
		return "", whisperOut{}, false
	}
	var text string
	var out whisperOut
	switch {
	case v.round.open:
		text, out = b.roundReply(w)
	case b.answering(now):
		text = b.afterReply(w)
	}
	if text == "" {
		return "", whisperOut{}, false
	}
	return safeLead(text), out, true
}

// deal is where partner's haggle over k stands: the current price (the
// entry's ask until countered) and the counters used.
func (b *Brain) deal(partner int32, k valuableKey) whisperPrice {
	if d, ok := b.cfg.valuables.talk.deals[whisperDeal{partner, k}]; ok {
		return d
	}
	return whisperPrice{current: b.cfg.valuables.entries[k].ask}
}

// roundReply is the reply to w in a round (docs/valuables/whispers.md
// "Replies in a round") and its effect on the deal it answers.
func (b *Brain) roundReply(w whisperWait) (string, whisperOut) {
	keys := b.whisperKeys(false)
	if len(keys) == 0 {
		return "", whisperOut{}
	}
	m, p := b.cfg.Travel, b.persona.Valuables
	named, rest := m.whisperNames(w.text, keys)
	if len(named) > 1 {
		return b.listReply(w.to, named, false, ", which?"), whisperOut{}
	}
	if offer, ok := linePrice(rest); ok {
		k := keys[0]
		if len(named) == 1 {
			k = named[0]
		}
		d, floor := b.deal(w.to, k), b.valuableFloor(k)
		dl := whisperDeal{w.to, k}
		if 2*offer >= floor {
			switch {
			case offer >= d.current:
				return "ok " + priceText(d.current) + ", trade me", whisperOut{deal: dl, price: d.current, agreed: true}
			case d.counters < p.Counters:
				c := min(d.current, ceilDiv(max(floor, (offer+d.current)/2), 1000)*1000)
				return priceText(c) + " lowest", whisperOut{deal: dl, price: c, counter: true}
			case offer >= floor:
				return "ok " + priceText(offer) + ", trade me", whisperOut{deal: dl, price: offer, agreed: true}
			default:
				return "sorry, no", whisperOut{}
			}
		}
		if len(named) == 1 {
			return m.itemLabel(k, 1, d.current) + ", trade me", whisperOut{deal: dl, price: d.current}
		}
		return m.itemLabel(k, 1, d.current) + ", trade me", whisperOut{}
	}
	if len(named) == 1 {
		// The named entry's deal, for the trade's goods (valuableGoods).
		d := b.deal(w.to, named[0])
		return m.itemLabel(named[0], 1, d.current) + ", trade me", whisperOut{deal: whisperDeal{w.to, named[0]}, price: d.current}
	}
	return b.listReply(w.to, keys, false, ", trade me"), whisperOut{}
}

// afterReply is the reply to w within AnswerAfter of a round's end
// (docs/valuables/whispers.md "After a round"): no offer, no trade. A
// released entry's replies are V7c's.
func (b *Brain) afterReply(w whisperWait) string {
	keys := b.whisperKeys(true)
	if len(keys) == 0 {
		return ""
	}
	if named, _ := b.cfg.Travel.whisperNames(w.text, keys); len(named) == 1 {
		return b.cfg.Travel.itemName(named[0]) + " not now, wts again soon"
	}
	return b.listReply(w.to, keys, true, " again soon")
}

// listReply is "WTS " and the labels of keys that fit, whole and in
// order, then tail: carried units at the partner's current price in a
// round, held units at the ask after it. A first label too long alone is
// cut by clipChat.
func (b *Brain) listReply(partner int32, keys []valuableKey, after bool, tail string) string {
	labels := make([]string, len(keys))
	for i, k := range keys {
		e := b.cfg.valuables.entries[k]
		if after {
			labels[i] = b.cfg.Travel.itemLabel(k, e.carried+e.stored, e.ask)
		} else {
			labels[i] = b.cfg.Travel.itemLabel(k, e.carried, b.deal(partner, k).current)
		}
	}
	text, n := listText(adHead, tail, labels)
	if n == 0 {
		return clipChat(adHead + labels[0] + tail)
	}
	return text
}

// whisperKeys are the entries a whisper may name, by score highest first:
// those carried in a round, those carried or stored after it (held).
func (b *Brain) whisperKeys(held bool) []valuableKey {
	return b.valuablesByScore(func(e *valuableEntry) bool { return e.carried > 0 || held && e.stored > 0 })
}

// safeLead gives text a lowercase "wts " lead when it starts with a walker
// command, which the server drops, and cuts it to chatMaxText.
func safeLead(text string) string {
	for _, c := range walkerCommands {
		if strings.HasPrefix(text, c) {
			text = "wts " + text
			break
		}
	}
	return clipChat(text)
}

// askingPrice reports whether line asks a price.
func askingPrice(line string) bool {
	l := strings.ToLower(line)
	return strings.Contains(l, "?") || strings.Contains(l, "how much") || strings.Contains(l, "price")
}

// whisperNames are the entries among keys line names
// (docs/valuables/whispers.md "Naming an item"): one when it names one,
// several when it points at several, none when it names nothing; and the
// line, lower case, with every full name it contains taken out.
func (m *TravelMap) whisperNames(line string, keys []valuableKey) ([]valuableKey, string) {
	lower := strings.ToLower(line)
	names := make([]string, len(keys))
	best := 0
	var full []valuableKey
	for i, k := range keys {
		names[i] = strings.ToLower(m.itemNames[k.template])
		n := names[i]
		if n == "" || !strings.Contains(lower, n) {
			continue
		}
		switch {
		case len(n) > best:
			best, full = len(n), []valuableKey{k}
		case len(n) == best:
			full = append(full, k)
		}
	}
	rest := lower
	byLength := slices.Clone(names)
	slices.SortFunc(byLength, func(x, y string) int { return len(y) - len(x) })
	for _, n := range byLength {
		if n != "" {
			rest = strings.ReplaceAll(rest, n, " ")
		}
	}
	if len(full) > 0 {
		return byEnchant(full, lower), rest
	}
	words := strings.FieldsFunc(lower, notWordRune)
	for _, kind := range []bool{false, true} {
		var pointed []valuableKey
		for i, k := range keys {
			if pointsAt(words, names[i], kind) {
				pointed = append(pointed, k)
			}
		}
		if len(pointed) > 0 {
			return byEnchant(pointed, lower), rest
		}
	}
	return nil, rest
}

// pointsAt reports whether a word of the line points at the entry named
// name: a distinctive word, or a kind word when kind is set.
func pointsAt(words []string, name string, kind bool) bool {
	nameWords := strings.FieldsFunc(name, notWordRune)
	for _, w := range words {
		if whisperKindWords[w] != kind || whisperStopWords[w] || letters(w) < whisperWordLetters {
			continue
		}
		if slices.Contains(nameWords, w) {
			return true
		}
	}
	return false
}

// byEnchant narrows keys of one name to the enchant levels the line writes
// as "+N", when it writes one of them.
func byEnchant(keys []valuableKey, lower string) []valuableKey {
	if len(keys) < 2 {
		return keys
	}
	var levels []uint16
	for _, f := range strings.Fields(lower) {
		if n, err := strconv.ParseUint(strings.TrimPrefix(f, "+"), 10, 16); err == nil && strings.HasPrefix(f, "+") {
			levels = append(levels, uint16(n))
		}
	}
	out := slices.DeleteFunc(slices.Clone(keys), func(k valuableKey) bool { return !slices.Contains(levels, k.enchant) })
	if len(out) == 0 {
		return keys
	}
	return out
}

func notWordRune(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

func letters(w string) int {
	n := 0
	for _, r := range w {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// linePrice is the one price line writes (docs/valuables/whispers.md
// "Reading a price"); none when it writes none or two or more.
func linePrice(line string) (int64, bool) {
	var price int64
	n := 0
	for _, f := range strings.Fields(line) {
		if v, ok := parsePrice(f); ok {
			price = v
			n++
		}
	}
	return price, n == 1
}

// parsePrice reads one word as a price: digits with an optional "." or ","
// decimal part and "k", "kk" or "m", or bare digits of at least 1000.
// Trailing sentence signs are no part of the word.
func parsePrice(w string) (int64, bool) {
	w = strings.TrimRight(strings.ToLower(w), "?!.,;:")
	mult := int64(1)
	switch {
	case strings.HasSuffix(w, "kk"):
		mult, w = 1000000, w[:len(w)-2]
	case strings.HasSuffix(w, "k"):
		mult, w = 1000, w[:len(w)-1]
	case strings.HasSuffix(w, "m"):
		mult, w = 1000000, w[:len(w)-1]
	}
	whole, frac, decimal := strings.Cut(strings.ReplaceAll(w, ",", "."), ".")
	if !digits(whole, 10) || decimal && (mult == 1 || !digits(frac, 6)) {
		return 0, false
	}
	n, _ := strconv.ParseInt(whole, 10, 64)
	v := n * mult
	if decimal {
		f, _ := strconv.ParseInt(frac, 10, 64)
		scale := int64(1)
		for range frac {
			scale *= 10
		}
		v += f * mult / scale
	}
	if v <= 0 || mult == 1 && v < 1000 {
		return 0, false
	}
	return v, true
}

// digits reports whether s is 1 to most ASCII digits.
func digits(s string, most int) bool {
	if s == "" || len(s) > most {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
