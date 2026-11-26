package botclient

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// The whisper fixture's valuables beside the ledger's, each with a supply
// of one an hour (R 2, a first ask of 125 % of the reference price):
// Tears of Eva at 100000 and two scrolls at 40000.
const (
	whisperTears   int32 = 9101
	whisperScrollA int32 = 9102
	whisperScrollB int32 = 9103
	// whisperLong is the first of seven long-named recipes.
	whisperLong int32 = 9200
)

// newWhisperBrain is newLedgerBrain with the whisper fixture's names and
// prices, its ledger synced over items.
func newWhisperBrain(t *testing.T, items ...Item) *Brain {
	t.Helper()
	b := newLedgerBrain(t, items...)
	// The tests' rounds say no ad line: long enough for every exchange.
	b.persona.Valuables.FirstLineWait = time.Hour
	m := b.cfg.Travel
	m.itemNames = map[int32]string{
		ledgerSword: "Sword of Watershadow", ledgerStaff: "Conjuror's Staff", ledgerRecipe: "Recipe: Halberd(60%)",
		whisperTears: "Tears of Eva", whisperScrollA: "Scroll: Enchant Armor (Grade C)", whisperScrollB: "Blessed Scroll of Escape",
	}
	for _, id := range []int32{whisperTears, whisperScrollA, whisperScrollB} {
		m.prices[id] = 40000
		m.supply[id] = itemSupply{perHour: 1}
	}
	m.prices[whisperTears] = 100000
	for i := range int32(7) {
		id := whisperLong + i
		m.itemNames[id] = fmt.Sprintf("Recipe: Sealed Dragon Blade (%d0%%)", i+1)
		m.prices[id] = 100000 + 10000*i
		m.supply[id] = itemSupply{perHour: 1}
	}
	b.syncValuables(t0)
	return b
}

func tearsUnit(obj int32) Item { return lootStack(obj, whisperTears, 1) }

// sentReply is a whisper reply rule "chat" sent: to whom, what, when.
type sentReply struct {
	to, text string
	at       time.Time
}

// tellText reads the text and target of a Say2 Tell payload.
func tellText(t *testing.T, payload []byte) (string, string) {
	t.Helper()
	read := func(p []byte) (string, []byte) {
		var u []uint16
		for len(p) >= 2 {
			c := binary.LittleEndian.Uint16(p)
			p = p[2:]
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		return string(utf16.Decode(u)), p
	}
	text, p := read(payload[1:])
	if len(p) < 4 || int32(binary.LittleEndian.Uint32(p)) != chatTell {
		t.Fatalf("payload % x is no Say2 Tell", payload)
	}
	to, _ := read(p[4:])
	return text, to
}

// drainChat decides every 100 ms in (from, until], sending what rule "chat"
// decides, and returns the replies sent.
func drainChat(t *testing.T, b *Brain, from, until time.Time) []sentReply {
	t.Helper()
	var out []sentReply
	for now := from.Add(100 * time.Millisecond); !now.After(until); now = now.Add(100 * time.Millisecond) {
		rule, a := b.decide(now)
		if rule != "chat" {
			continue
		}
		if sent, err := b.send(a, now); err != nil || !sent {
			t.Fatalf("send at %v = %v, %v; want the reply out", now.Sub(t0), sent, err)
		}
		text, to := tellText(t, a.payload)
		out = append(out, sentReply{to: to, text: text, at: now})
	}
	return out
}

// whisperIn hears text whispered by name (object id from) at now and
// returns the replies sent in the 15 s after it, checking a queued reply
// is due 3-12 s after the whisper and no decision sends it earlier.
func whisperIn(t *testing.T, b *Brain, from int32, name, text string, now time.Time) []sentReply {
	t.Helper()
	if err := b.handle([]Event{whisper(from, name, text)}, now); err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(b.whispers.waits, func(w whisperWait) bool { return w.to == from }); i >= 0 {
		w := b.whispers.waits[i]
		if d := w.at.Sub(now); d < 3*time.Second || d > 12*time.Second {
			t.Fatalf("%q: reply due %v after it, want 3-12 s", text, d)
		}
		if b.decisionAt().After(w.at) {
			t.Fatalf("%q: decision at %v, after the reply's %v", text, b.decisionAt().Sub(t0), w.at.Sub(t0))
		}
		if early := drainChat(t, b, now, w.at.Add(-time.Millisecond)); len(early) > 0 {
			t.Fatalf("%q: replies %+v before the delay", text, early)
		}
		return drainChat(t, b, w.at.Add(-100*time.Millisecond), now.Add(15*time.Second))
	}
	return drainChat(t, b, now, now.Add(15*time.Second))
}

// replyTo is whisperIn's one reply to name, "" for none, failing on more.
func replyTo(t *testing.T, b *Brain, from int32, name, text string, now time.Time) string {
	t.Helper()
	got := whisperIn(t, b, from, name, text, now)
	switch {
	case len(got) == 0:
		return ""
	case len(got) > 1 || got[0].to != name:
		t.Fatalf("%q: replies %+v, want one to %s", text, got, name)
	}
	if u := chatUnits(got[0].text); u > chatMaxText {
		t.Fatalf("%q: reply of %d units, over %d", got[0].text, u, chatMaxText)
	}
	return got[0].text
}

// whisperLine is a whisper and the reply it gets.
type whisperLine struct{ say, want string }

// talk whispers each line from name 20 s apart from start and checks each
// reply.
func talk(t *testing.T, b *Brain, from int32, name string, start time.Time, lines ...whisperLine) {
	t.Helper()
	for i, l := range lines {
		if got := replyTo(t, b, from, name, l.say, start.Add(time.Duration(i)*20*time.Second)); got != l.want {
			t.Fatalf("%s %q: reply %q, want %q", name, l.say, got, l.want)
		}
	}
}

// TestValuablesWhisperHaggle checks the valuables whisper responder
// (docs/valuables/whispers.md, D32): the haggle in a round, naming an item,
// reading a price, the length of a reply and the replies after a round.
func TestValuablesWhisperHaggle(t *testing.T) {
	// The sword asks 170000 (125 % of 136000) with a floor of 136000.
	t.Run("Haggle", func(t *testing.T) {
		t.Run("question, two counters, agreed at the floor", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			talk(t, b, pcID, "Alden", t0,
				whisperLine{"how much for sword of watershadow?", "Sword of Watershadow 170k, trade me"},
				whisperLine{"140k", "155k lowest"},
				whisperLine{"145k", "150k lowest"},
				whisperLine{"136k", "ok 136k, trade me"},
			)
			if d := b.deal(pcID, swordKey); d.current != 136000 || d.counters != 2 {
				t.Fatalf("deal %+v, want agreed at 136000 after 2 counters", d)
			}
		})
		t.Run("below the floor after the counters is refused", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			talk(t, b, pcID, "Alden", t0,
				whisperLine{"100k", "136k lowest"},
				whisperLine{"100k", "136k lowest"},
				whisperLine{"100k", "sorry, no"},
			)
		})
		t.Run("at or above the current price is agreed at it", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			talk(t, b, pcID, "Alden", t0, whisperLine{"200k", "ok 170k, trade me"})
		})
		t.Run("under half the floor is a question", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			talk(t, b, pcID, "Alden", t0, whisperLine{"60k", "Sword of Watershadow 170k, trade me"})
			if d := b.deal(pcID, swordKey); d.counters != 0 {
				t.Fatalf("deal %+v, want no counter used", d)
			}
		})
		t.Run("+5 floor holds five scrolls", func(t *testing.T) {
			// ask 1095250 (131.25 % of 644000 plus 250000), floor 894000.
			b := newWhisperBrain(t, Item{ObjectID: 101, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand, Enchant: 5})
			openAdRound(b, t0)
			talk(t, b, pcID, "Alden", t0,
				whisperLine{"conjuror's staff?", "+5 Conjuror's Staff 1.1kk, trade me"},
				whisperLine{"644k", "894k lowest"},
				whisperLine{"644k", "894k lowest"},
				whisperLine{"644k", "sorry, no"},
			)
		})
		t.Run("caps, markers and the queue", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			if got := whisperIn(t, b, pcID+100, "AldenBot", "sword?", t0); len(got) > 0 || len(b.whispers.waits) > 0 {
				t.Fatalf("marked name: replies %+v, queue %+v; want none", got, b.whispers.waits)
			}
			now := at(time.Minute)
			talk(t, b, pcID, "Alden", now,
				whisperLine{"sword?", "Sword of Watershadow 170k, trade me"},
				whisperLine{"sword?", "Sword of Watershadow 170k, trade me"},
				whisperLine{"sword?", "Sword of Watershadow 170k, trade me"},
				whisperLine{"sword?", "Sword of Watershadow 170k, trade me"},
				whisperLine{"sword?", ""},
			)
			now = at(10 * time.Minute)
			for i := range int32(16) {
				talk(t, b, pcID+1+i, fmt.Sprintf("P%d", i), now.Add(time.Duration(i)*20*time.Second), whisperLine{"sword?", "Sword of Watershadow 170k, trade me"})
			}
			if b.cfg.valuables.talk.replies != 20 {
				t.Fatalf("replies %d, want 20", b.cfg.valuables.talk.replies)
			}
			talk(t, b, pcID+50, "Late", at(20*time.Minute), whisperLine{"sword?", ""})
			if len(b.cfg.chat.sent) != 0 || len(b.cfg.chat.to) != 0 || b.chat.reply.kind != actHold {
				t.Fatalf("chat log %+v, canned %+v; want the responder outside both", b.cfg.chat, b.chat)
			}
		})
		t.Run("two partners 1 s apart, a newer whisper replaces", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			for _, e := range []struct {
				from       int32
				name, text string
				at         time.Duration
			}{
				{pcID, "Alden", "hi", 0},
				{pcID + 1, "Brin", "sword?", time.Second},
				{pcID, "Alden", "150k", 2 * time.Second},
			} {
				if err := b.handle([]Event{whisper(e.from, e.name, e.text)}, at(e.at)); err != nil {
					t.Fatal(err)
				}
			}
			if len(b.whispers.waits) != 2 {
				t.Fatalf("queue %+v, want one reply per partner", b.whispers.waits)
			}
			got := drainChat(t, b, at(2*time.Second), at(time.Minute))
			want := map[string]string{"Alden": "160k lowest", "Brin": "Sword of Watershadow 170k, trade me"}
			if len(got) != 2 || want[got[0].to] != got[0].text || want[got[1].to] != got[1].text || got[1].at.Sub(got[0].at) < chatGap {
				t.Fatalf("replies %+v, want %v a chat gap apart", got, want)
			}
		})
	})

	t.Run("Names", func(t *testing.T) {
		// The sword scores above the tears (ask 125000, floor 100000); the
		// scrolls ask 50000.
		swordTears := []Item{swordUnit(101, 0), tearsUnit(102)}
		both := "WTS Sword of Watershadow 170k, Tears of Eva 125k, trade me"
		tests := []struct {
			name  string
			items []Item
			say   string
			want  string
		}{
			{"of course names nothing", swordTears, "of course", both},
			{"out of adena names nothing", swordTears, "out of adena", both},
			{"an offer naming nothing goes to the top-scored", swordTears, "of course 150k", "160k lowest"},
			{"a distinctive word names", swordTears, "tears?", "Tears of Eva 125k, trade me"},
			{"an offer on the named", swordTears, "watershadow 150k", "160k lowest"},
			{"the full name names", swordTears, "sword of watershadow", "Sword of Watershadow 170k, trade me"},
			{
				"a kind word at two scrolls",
				[]Item{lootStack(102, whisperScrollA, 1), lootStack(103, whisperScrollB, 1)},
				"scroll 50k",
				"WTS Scroll: Enchant Armor (Grade C) 50k, Blessed Scroll of Escape 50k, which?",
			},
			{"a kind word at one scroll", []Item{swordUnit(101, 0), lootStack(102, whisperScrollA, 1)}, "scroll 50k", "ok 50k, trade me"},
			{"a distinctive word wins over a kind word", []Item{swordUnit(101, 0), lootStack(102, whisperScrollA, 1)}, "watershadow scroll", "Sword of Watershadow 170k, trade me"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := newWhisperBrain(t, tt.items...)
				openAdRound(b, t0)
				talk(t, b, pcID, "Alden", t0, whisperLine{tt.say, tt.want})
			})
		}
	})

	t.Run("Prices", func(t *testing.T) {
		tests := []struct {
			name  string
			items []Item
			say   string
			want  string
		}{
			{"a +5 question gets the ask", []Item{{ObjectID: 101, TemplateID: ledgerStaff, Count: 1, BodySlot: partRHand, Enchant: 5}}, "how much for +5 conjuror's staff?", "+5 Conjuror's Staff 1.1kk, trade me"},
			{"a price is per unit", []Item{lootStack(102, whisperTears, 2)}, "2x tears 110k", "118k lowest"},
			{"a price at the ask is per unit", []Item{lootStack(102, whisperTears, 2)}, "2x tears 150k", "ok 125k, trade me"},
			{"a name's own number is no price", []Item{lootStack(103, ledgerRecipe, 1)}, "recipe: halberd(60%) 300k", "379k lowest"},
			{"under 1000 is no price", []Item{tearsUnit(102)}, "tears 500", "Tears of Eva 125k, trade me"},
			{"two prices are none", []Item{tearsUnit(102)}, "tears 150k 140k", "Tears of Eva 125k, trade me"},
			{"a spaced number is none", []Item{tearsUnit(102)}, "tears 150 000", "Tears of Eva 125k, trade me"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := newWhisperBrain(t, tt.items...)
				openAdRound(b, t0)
				talk(t, b, pcID, "Alden", t0, whisperLine{tt.say, tt.want})
				for k, d := range b.cfg.valuables.talk.deals {
					if d.counters > 0 && !strings.HasSuffix(tt.want, "lowest") {
						t.Fatalf("deal %v %+v used a counter", k, d)
					}
				}
			})
		}

		words := []struct {
			word string
			want int64
		}{
			{"150000", 150000},
			{"150k", 150000},
			{"150K", 150000},
			{"1.2kk", 1200000},
			{"1.2m", 1200000},
			{"1,5k", 1500},
			{"2kk", 2000000},
			{"150k?", 150000},
			{"1000", 1000},
			{"150", 0},
			{"999", 0},
			{"+5", 0},
			{"+150k", 0},
			{"2x", 0},
			{"x2", 0},
			{"150kx", 0},
			{"halberd(60%)", 0},
			{"(150k)", 0},
			{"items/5400-5499.xml:476", 0},
			{"0k", 0},
			{"1,000", 0},
			{"1.5", 0},
			{"k", 0},
			{"1.2.3k", 0},
		}
		for _, w := range words {
			got, ok := parsePrice(w.word)
			if got != w.want || ok != (w.want > 0) {
				t.Errorf("parsePrice(%q) = %d, %v; want %d", w.word, got, ok, w.want)
			}
		}
	})

	t.Run("Length", func(t *testing.T) {
		var items []Item
		for i := range int32(7) {
			items = append(items, lootStack(110+i, whisperLong+i, 1))
		}
		b := newWhisperBrain(t, items...)
		var labels []string
		for _, k := range b.whisperKeys(false) {
			e := b.cfg.valuables.entries[k]
			labels = append(labels, b.cfg.Travel.itemLabel(k, e.carried, e.ask))
		}
		if len(labels) != 7 {
			t.Fatalf("labels %q, want seven", labels)
		}
		check := func(name, got, tail string, replies int) {
			t.Helper()
			if chatUnits(got) > chatMaxText || !strings.HasPrefix(got, "WTS ") || !strings.HasSuffix(got, tail) {
				t.Fatalf("%s: reply %q (%d units), want WTS ...%s within %d", name, got, chatUnits(got), tail, chatMaxText)
			}
			named := strings.Split(strings.TrimSuffix(strings.TrimPrefix(got, "WTS "), tail), ", ")
			if len(named) == 0 || len(named) == len(labels) || !slices.Equal(named, labels[:len(named)]) {
				t.Fatalf("%s: names %q, want whole top-scored labels of %q", name, named, labels)
			}
			if n := b.cfg.valuables.talk.replies; n != replies {
				t.Fatalf("%s: replies counted %d, want %d", name, n, replies)
			}
		}
		openAdRound(b, t0)
		check("pointing at several", replyTo(t, b, pcID, "Alden", "dragon blade?", t0), ", which?", 1)
		check("naming none", replyTo(t, b, pcID+1, "Brin", "hello", at(20*time.Second)), ", trade me", 2)
		b.cfg.valuables.endRound(at(time.Minute))
		check("after the round", replyTo(t, b, pcID+2, "Cael", "price?", at(2*time.Minute)), " again soon", 3)
	})

	t.Run("After the round", func(t *testing.T) {
		const notNow = "Sword of Watershadow not now, wts again soon"
		ended := at(time.Minute)
		newEnded := func(t *testing.T) *Brain {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			b.cfg.valuables.endRound(ended)
			return b
		}
		t.Run("naming a held entry", func(t *testing.T) {
			b := newEnded(t)
			talk(t, b, pcID, "Alden", ended.Add(time.Minute), whisperLine{"sword of watershadow 150k", notNow})
			if b.cfg.valuables.talk.replies != 1 || len(b.cfg.valuables.talk.deals) != 0 {
				t.Fatalf("talk %+v, want one reply in the round's caps, no offer", b.cfg.valuables.talk)
			}
		})
		t.Run("a price question naming nothing", func(t *testing.T) {
			b := newEnded(t)
			talk(t, b, pcID, "Alden", ended.Add(time.Minute), whisperLine{"how much?", "WTS Sword of Watershadow 170k again soon"})
		})
		t.Run("a reply pending at the end", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			if err := b.handle([]Event{whisper(pcID, "Alden", "sword 150k")}, t0); err != nil {
				t.Fatal(err)
			}
			b.cfg.valuables.endRound(at(time.Second))
			got := drainChat(t, b, at(time.Second), at(15*time.Second))
			if len(got) != 1 || got[0].text != notNow {
				t.Fatalf("replies %+v, want %q", got, notNow)
			}
		})
		t.Run("the ended round's caps", func(t *testing.T) {
			b := newWhisperBrain(t, swordUnit(101, 0))
			openAdRound(b, t0)
			for i := range 4 {
				replyTo(t, b, pcID, "Alden", "sword?", at(time.Duration(i)*20*time.Second))
			}
			b.cfg.valuables.endRound(at(2 * time.Minute))
			if got := whisperIn(t, b, pcID, "Alden", "sword?", at(3*time.Minute)); len(got) > 0 || b.chat.reply.kind != actHold {
				t.Fatalf("replies %+v, canned %+v; want silence", got, b.chat)
			}
		})
		t.Run("a pending reply past AnswerAfter is dropped", func(t *testing.T) {
			b := newEnded(t)
			heard := ended.Add(b.persona.Valuables.AnswerAfter - 2*time.Second)
			if err := b.handle([]Event{whisper(pcID, "Alden", "sword?")}, heard); err != nil {
				t.Fatal(err)
			}
			if got := drainChat(t, b, heard, heard.Add(15*time.Second)); len(got) > 0 || len(b.whispers.waits) > 0 {
				t.Fatalf("replies %+v, queue %+v; want it dropped", got, b.whispers.waits)
			}
		})
		canned := []struct {
			name string
			b    func(t *testing.T) *Brain
			at   time.Time
			say  string
		}{
			{"an unrelated whisper", newEnded, ended.Add(time.Minute), "hi there"},
			{"past AnswerAfter", newEnded, ended.Add(30 * time.Minute), "sword?"},
			{"before any round", func(t *testing.T) *Brain { return newWhisperBrain(t, swordUnit(101, 0)) }, ended, "sword?"},
		}
		for _, tt := range canned {
			t.Run(tt.name+" gets the canned reply", func(t *testing.T) {
				b := tt.b(t)
				if err := b.handle([]Event{whisper(pcID, "Alden", tt.say)}, tt.at); err != nil {
					t.Fatal(err)
				}
				if b.chat.reply.kind != actChatReply || len(b.whispers.waits) > 0 {
					t.Fatalf("canned %+v, queue %+v; want the canned reply", b.chat, b.whispers.waits)
				}
			})
		}
	})
}

// TestWhisperSafeLead checks a reply never starts with a walker command
// the server drops, and stays within chatMaxText.
func TestWhisperSafeLead(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"Sword of Watershadow 170k, trade me", "Sword of Watershadow 170k, trade me"},
		{"SAY Hammer 10k, trade me", "wts SAY Hammer 10k, trade me"},
		{"Say Hammer 10k, trade me", "Say Hammer 10k, trade me"},
		{"MSG" + strings.Repeat("x", 120), "wts MSG" + strings.Repeat("x", 93)},
	} {
		if got := safeLead(tt.in); got != tt.want {
			t.Errorf("safeLead(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
