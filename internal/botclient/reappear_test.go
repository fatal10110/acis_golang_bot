package botclient

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"
)

// opcodes lists the opcode of every frame out.
func opcodes(out *recordSender) []byte {
	ops := make([]byte, 0, len(out.frames))
	for _, f := range out.frames {
		ops = append(ops, f[0])
	}
	return ops
}

// TestBrainAppearing checks that the bot's own loading-screen teleport is
// answered by one Appearing at the reaction to it, and nothing else is.
func TestBrainAppearing(t *testing.T) {
	land := Point{X: 1000, Y: 2000, Z: -300}
	tests := []struct {
		name   string
		events []Event
		want   []byte
	}{
		{"own teleport", []Event{TeleportToLocation{ObjectID: selfID, Pos: land}}, []byte{opGameAppearing}},
		{"two teleports before the reaction", []Event{
			TeleportToLocation{ObjectID: selfID, Pos: land},
			TeleportToLocation{ObjectID: selfID, Pos: land},
		}, []byte{opGameAppearing}},
		{"own fast teleport", []Event{TeleportToLocation{ObjectID: selfID, Pos: land, Fast: true}}, []byte{}},
		{"another object's teleport", []Event{TeleportToLocation{ObjectID: pcID, Pos: land}}, []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for seed := range uint64(50) {
				b, out := newTestBrain(t, Persona{})
				b.rng = rand.New(rand.NewPCG(seed, seed))
				b.nextTick = at(3 * time.Second)
				ev := at(100 * time.Millisecond)
				if err := b.handle(tt.events, ev); err != nil {
					t.Fatal(err)
				}
				react := b.decisionAt()
				if d := react.Sub(ev); len(tt.want) > 0 && (d < defaultReactMin || d > defaultReactMax) {
					t.Fatalf("seed %d: Appearing due %v after the teleport, want [%v, %v]", seed, d, defaultReactMin, defaultReactMax)
				}
				if err := b.wake(react.Add(-time.Millisecond)); err != nil || len(out.frames) != 0 {
					t.Fatalf("seed %d: sent % x before the reaction (err %v)", seed, opcodes(out), err)
				}
				for _, w := range []time.Time{react, react.Add(2 * time.Second), react.Add(4 * time.Second)} {
					if err := b.wake(w); err != nil {
						t.Fatal(err)
					}
				}
				if got := opcodes(out); !bytes.Equal(got, tt.want) {
					t.Fatalf("seed %d: sent % x, want % x", seed, got, tt.want)
				}
				if tt.name == "own teleport" && b.anchor != land {
					t.Fatalf("seed %d: anchor %v, want the landing %v", seed, b.anchor, land)
				}
			}
		})
	}
}

// TestBrainAppearingPaced checks that an Appearing the pacer refuses goes
// out once the frame bucket refills, and that the decision waits for it
// and for the landing it asks.
func TestBrainAppearingPaced(t *testing.T) {
	var turns int
	b, out := newTestBrain(t, countGoal(&turns))
	b.nextTick = at(time.Hour)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID}}, at(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	react := b.decisionAt()
	b.pace.tokens, b.pace.filled = 0, react
	if err := b.wake(react); err != nil {
		t.Fatal(err)
	}
	if len(out.frames) != 0 || turns != 0 {
		t.Fatalf("refused Appearing: sent % x, %d decisions; want none", opcodes(out), turns)
	}
	retry := b.due()
	if retry.Sub(react) != time.Second/frameRate {
		t.Fatalf("retry due %v after the refusal, want %v", retry.Sub(react), time.Second/frameRate)
	}
	if err := b.wake(retry); err != nil {
		t.Fatal(err)
	}
	if got := opcodes(out); !bytes.Equal(got, []byte{opGameAppearing}) || turns != 0 {
		t.Fatalf("after the refill: sent % x and %d decisions, want Appearing and no decision before the landing", got, turns)
	}
	landed := retry.Add(50 * time.Millisecond)
	if err := b.handle([]Event{UserInfo{ObjectID: selfID}}, landed); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(landed); err != nil {
		t.Fatal(err)
	}
	if turns != 1 {
		t.Fatalf("landed: %d decisions, want 1", turns)
	}
}

// TestBrainRestart checks the default death policy: a killed bot asks to
// restart in town after the reaction delay, asks again only every
// restartRetry while the server leaves it dead, and once revived and
// teleported to town sends Appearing and no more restarts.
func TestBrainRestart(t *testing.T) {
	restart := []byte{opGameRequestRestartPoint, 0, 0, 0, 0}
	for seed := range uint64(50) {
		p := DefaultPersona()
		p.IdleChance = 0
		b, out := newTestBrain(t, p)
		b.rng = rand.New(rand.NewPCG(seed, seed))
		b.nextTick = at(time.Hour)
		died := at(100 * time.Millisecond)
		if err := b.handle([]Event{Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}}, died); err != nil {
			t.Fatal(err)
		}
		react := b.decisionAt()
		if d := react.Sub(died); d < defaultReactMin || d > defaultReactMax {
			t.Fatalf("seed %d: restart due %v after death, want [%v, %v]", seed, d, defaultReactMin, defaultReactMax)
		}
		if err := b.wake(react.Add(-time.Millisecond)); err != nil || len(out.frames) != 0 {
			t.Fatalf("seed %d: sent % x before the reaction (err %v)", seed, opcodes(out), err)
		}
		// Wake on every tick until the restart goes out again.
		for now := react; len(out.frames) < 2; now = b.decisionAt() {
			if now.After(react.Add(restartRetry + defaultTick + defaultTickJitter)) {
				t.Fatalf("seed %d: no second restart by %v after the first", seed, now.Sub(react))
			}
			if err := b.wake(now); err != nil {
				t.Fatal(err)
			}
			if len(out.frames) == 2 && now.Before(react.Add(restartRetry)) {
				t.Fatalf("seed %d: restart repeated %v after the first, want at least %v", seed, now.Sub(react), restartRetry)
			}
		}
		for i, f := range out.frames {
			if !bytes.Equal(f, restart) {
				t.Fatalf("seed %d: frame %d = % x, want % x", seed, i, f, restart)
			}
		}
		out.frames = nil
		revived := b.decisionAt().Add(-time.Millisecond)
		evs := []Event{Revive{ObjectID: selfID}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 83400, Y: 147943, Z: -3404}}}
		if err := b.handle(evs, revived); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			if err := b.wake(b.decisionAt()); err != nil {
				t.Fatal(err)
			}
		}
		if got := opcodes(out); !bytes.Equal(got, []byte{opGameAppearing}) {
			t.Fatalf("seed %d: after the revive sent % x, want only Appearing", seed, got)
		}
		if !b.restartedAt.IsZero() {
			t.Fatalf("seed %d: restart still pending after the revive", seed)
		}
	}
}

func TestDeathPolicy(t *testing.T) {
	tests := []struct {
		name   string
		points RestartPoints
		want   actionKind
	}{
		{"village offered", RestartPoints{Village: true, Fixed: true}, actRestart},
		{"no village", RestartPoints{ClanHall: true}, actHold},
	}
	for _, tt := range tests {
		b, _ := newTestBrain(t, DefaultPersona())
		b.world.Apply(Die{ObjectID: selfID, Restart: tt.points}, t0)
		name, a := b.decide(at(time.Second))
		if name != "dead" || a.kind != tt.want {
			t.Errorf("%s: rule %q kind %d, want dead kind %d", tt.name, name, a.kind, tt.want)
		}
	}
}
