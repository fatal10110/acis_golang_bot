package botclient

import (
	"testing"
	"time"
)

// TestPvPDuelAcceptKeeps checks that the bot's acceptance keeps the
// challenge with its challenger's name until ExDuelReady, so no own
// challenge goes out in between and the countdown names the opponent, while
// a decline drops it (docs/pvp/finding.md "Sources").
func TestPvPDuelAcceptKeeps(t *testing.T) {
	t.Run("acceptance", func(t *testing.T) {
		b := findBrain(t, nil)
		due := ask(t, b, time.Second)
		rule, a := b.decide(due)
		if rule != "duel" || a.kind != actDuelAnswer || !a.yes {
			t.Fatalf("decide = %q kind %v yes %v, want the acceptance", rule, a.kind, a.yes)
		}
		if _, err := b.send(a, due); err != nil {
			t.Fatal(err)
		}
		if d := b.duel(due); d.Phase != DuelAsked || d.Name != foeName {
			t.Fatalf("after the acceptance phase %v name %q, want asked by %q", d.Phase, d.Name, foeName)
		}
		b.pvp.find.lastAsk = time.Time{}
		if got := challenged(t, b, due.Add(time.Second)); got != "" {
			t.Fatalf("challenged %q between the acceptance and ExDuelReady", got)
		}
		b.handleOne(t, ExDuelReady{}, due.Add(2*time.Second))
		if d := b.duel(due.Add(2 * time.Second)); d.Phase != DuelCountdown || d.Name != foeName {
			t.Fatalf("after ExDuelReady phase %v name %q, want the countdown with %q", d.Phase, d.Name, foeName)
		}
	})
	t.Run("decline", func(t *testing.T) {
		b := findBrain(t, nil)
		b.world.Self.MP = 40
		ask(t, b, time.Second)
		rule, a := b.decide(at(time.Second))
		if rule != "duel" || a.kind != actDuelAnswer || a.yes {
			t.Fatalf("decide = %q kind %v yes %v, want the decline", rule, a.kind, a.yes)
		}
		if _, err := b.send(a, at(time.Second)); err != nil {
			t.Fatal(err)
		}
		if d := b.duel(at(time.Second)); d.Phase != DuelIdle {
			t.Fatalf("after the decline phase %v, want idle", d.Phase)
		}
	})
}

// TestPvPChallengeWhisperDropped checks that a whispered duel the challenge
// rule cannot take at its reaction is dropped, not taken later
// (docs/pvp/finding.md "Sources").
func TestPvPChallengeWhisperDropped(t *testing.T) {
	solo := func(t *testing.T) *Brain {
		b := findBrain(t, nil)
		b.cfg.Travel.classes = nil // no estimate: no own challenge
		return b
	}
	for _, tc := range []struct {
		name         string
		stanceBefore bool
	}{
		{name: "heard in combat stance", stanceBefore: true},
		{name: "a stance before its reaction"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := solo(t)
			if tc.stanceBefore {
				b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(time.Second))
			}
			b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
			if !tc.stanceBefore {
				b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(time.Second))
			}
			due := at(2 * time.Second)
			if got := challenged(t, b, due); got != "" {
				t.Fatalf("challenged %q in combat stance", got)
			}
			b.handleOne(t, AutoAttackStop{ObjectID: selfID}, due.Add(time.Second))
			if got := challenged(t, b, due.Add(2*time.Second)); got != "" {
				t.Fatalf("challenged %q after AutoAttackStop", got)
			}
		})
	}
	for _, tc := range []struct {
		late time.Duration
		want string
	}{
		{late: 10 * time.Second, want: foeName},
		{late: whisperLapse + time.Second},
	} {
		t.Run("reached "+tc.late.String()+" late", func(t *testing.T) {
			b := solo(t)
			b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
			if got := challenged(t, b, b.pvp.find.whisper.due.Add(tc.late)); got != tc.want {
				t.Fatalf("challenged %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPvPOwnChallengeRoams checks that the bot challenges on its own only at
// its spot or on the road to it, on its own (docs/pvp/finding.md
// "Sources"), while a whispered duel is taken anyway.
func TestPvPOwnChallengeRoams(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Brain)
		want  string
	}{
		{name: "on the road to its spot", want: foeName},
		{name: "off its spot", setup: func(b *Brain) { b.spot.held = false }},
		{name: "a GM stay", setup: func(b *Brain) { b.orders.stay = true }},
		{name: "a GM follow", setup: func(b *Brain) { b.orders.follow = gmID }},
		{name: "a GM assist", setup: func(b *Brain) { b.orders.assist = gmID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := findBrain(t, nil)
			if tc.setup != nil {
				tc.setup(b)
			}
			if got := challenged(t, b, at(time.Second)); got != tc.want {
				t.Fatalf("challenged %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("a whisper off its spot", func(t *testing.T) {
		b := findBrain(t, nil)
		b.spot.held = false
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
		if got := challenged(t, b, b.pvp.find.whisper.due); got != foeName {
			t.Fatalf("challenged %q, want %q", got, foeName)
		}
	})
}

// TestPvPAssistHoldsInDuel checks that the assist starts no monster fight
// from a challenge until the duel is idle (docs/pvp/finding.md "Clear of
// monsters").
func TestPvPAssistHoldsInDuel(t *testing.T) {
	p := Persona{AssistMin: 100 * time.Millisecond, AssistMax: 100 * time.Millisecond}
	b, _ := newCommandBrain(t, p)
	b.obey(verdict{tell: tell{sender: gmID, name: "GM", cmd: command{kind: cmdAssist}}, ok: true}, at(time.Second))
	hit := Attack{AttackerID: gmID, Hits: []AttackHit{{TargetID: npcID}}}
	if err := b.handle([]Event{hit}, at(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	b.pvp.duel = Duel{Phase: DuelAsked, Name: foeName, Since: at(2 * time.Second)}
	if name, _ := b.decide(at(3 * time.Second)); name == "assist" {
		t.Fatal("assist on a monster with a duel challenge open")
	}
	b.pvp.duel = Duel{}
	if name, a := b.decide(at(3 * time.Second)); name != "assist" || a.object != npcID {
		t.Fatalf("decide = %q on %d with the duel idle, want assist on the npc", name, a.object)
	}
}
