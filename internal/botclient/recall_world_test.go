package botclient

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"
)

// TestBrainLanding checks that after its Appearing the bot makes no
// decision until the server shows the landing, its UserInfo coming after
// the objects around the landing point, or until landingTimeout passes
// without it (#199).
func TestBrainLanding(t *testing.T) {
	land := Point{X: 1000, Y: 2000, Z: -300}
	shown := []Event{CharInfo{ObjectID: pcID, Name: "player"}, NPCInfo{ObjectID: npcID}}
	tests := []struct {
		name  string
		after []Event // read 50 ms after the Appearing
		// decide is when the held decision is made, after the Appearing.
		decide time.Duration
	}{
		{"landing shown", append(shown, UserInfo{ObjectID: selfID, Name: "Self"}), 50 * time.Millisecond},
		{"objects without the UserInfo", shown, landingTimeout},
		{"landing not shown", nil, landingTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for seed := range uint64(20) {
				var turns int
				b, out := newTestBrain(t, countGoal(&turns))
				b.rng = rand.New(rand.NewPCG(seed, seed))
				b.nextTick = at(3 * time.Second)
				if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID, Pos: land}}, at(100*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				appear := b.decisionAt()
				if err := b.wake(appear); err != nil {
					t.Fatal(err)
				}
				if got := opcodes(out); !bytes.Equal(got, []byte{opGameAppearing}) || turns != 0 {
					t.Fatalf("seed %d: at the reaction sent % x and %d decisions, want Appearing alone", seed, got, turns)
				}
				if tt.after != nil {
					if err := b.handle(tt.after, appear.Add(50*time.Millisecond)); err != nil {
						t.Fatal(err)
					}
				}
				want := appear.Add(tt.decide)
				if got := b.decisionAt(); !got.Equal(want) {
					t.Fatalf("seed %d: decision due %v after the Appearing, want %v", seed, got.Sub(appear), tt.decide)
				}
				if err := b.wake(want.Add(-time.Millisecond)); err != nil || turns != 0 {
					t.Fatalf("seed %d: %d decisions before %v (err %v), want none", seed, turns, tt.decide, err)
				}
				if err := b.wake(want); err != nil || turns != 1 {
					t.Fatalf("seed %d: %d decisions at %v (err %v), want 1", seed, turns, tt.decide, err)
				}
				if got := opcodes(out); !bytes.Equal(got, []byte{opGameAppearing}) {
					t.Fatalf("seed %d: sent % x, want one Appearing", seed, got)
				}
			}
		})
	}
}

// TestBrainLandingTeleportedAgain checks that a teleport the server held for
// the landing, made right after it, is answered by its own Appearing at the
// reaction to it, not after the landing timeout, and that the decision then
// waits for its landing.
func TestBrainLandingTeleportedAgain(t *testing.T) {
	var turns int
	b, out := newTestBrain(t, countGoal(&turns))
	b.nextTick = at(3 * time.Second)
	if err := b.handle([]Event{TeleportToLocation{ObjectID: selfID}}, at(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	first := b.decisionAt()
	if err := b.wake(first); err != nil {
		t.Fatal(err)
	}
	again := first.Add(50 * time.Millisecond)
	if err := b.handle([]Event{UserInfo{ObjectID: selfID}, TeleportToLocation{ObjectID: selfID}}, again); err != nil {
		t.Fatal(err)
	}
	react := b.decisionAt()
	if d := react.Sub(again); d < defaultReactMin || d > defaultReactMax {
		t.Fatalf("second Appearing due %v after the teleport, want [%v, %v]", d, defaultReactMin, defaultReactMax)
	}
	if err := b.wake(react); err != nil {
		t.Fatal(err)
	}
	if got := opcodes(out); !bytes.Equal(got, []byte{opGameAppearing, opGameAppearing}) || turns != 0 {
		t.Fatalf("sent % x and %d decisions, want two Appearing and no decision", got, turns)
	}
	landed := react.Add(50 * time.Millisecond)
	if err := b.handle([]Event{UserInfo{ObjectID: selfID}}, landed); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(landed); err != nil || turns != 1 {
		t.Fatalf("%d decisions once landed (err %v), want 1", turns, err)
	}
}

// TestBrainRecallClanInvite replays the live failure of #199: a GM, the
// bot's commander, recalls the bot and invites it into a clan before the
// bot's reaction to the teleport. The answer waits for the landing that
// shows the GM, and accepts; answered with the Appearing, it found nobody
// in sight and declined.
func TestBrainRecallClanInvite(t *testing.T) {
	p := NewAuthority(AuthorityConfig{Commanders: []int32{gmID}}).Hooks(DefaultPersona())
	b, out := newTestBrain(t, Persona{JoinClan: p.JoinClan})
	b.nextTick = at(3 * time.Second)
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM"}, t0)
	recalled := at(100 * time.Millisecond)
	if err := b.handle([]Event{
		TeleportToLocation{ObjectID: selfID},
		DeleteObject{ObjectID: gmID},
		SystemMessage{ID: msgInvitedToClan, Text: [2]string{"GM", "Crafters"}},
		AskJoinPledge{RequesterID: gmID, Clan: "Crafters"},
	}, recalled); err != nil {
		t.Fatal(err)
	}
	appear := b.decisionAt()
	if err := b.wake(appear); err != nil {
		t.Fatal(err)
	}
	landed := appear.Add(50 * time.Millisecond)
	if err := b.handle([]Event{CharInfo{ObjectID: gmID, Name: "GM"}, UserInfo{ObjectID: selfID, Name: "Self"}}, landed); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(b.decisionAt()); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{encodeAppearing(), encodeRequestAnswerJoinPledge(true)}
	if len(out.frames) != len(want) || !bytes.Equal(out.frames[0], want[0]) || !bytes.Equal(out.frames[1], want[1]) {
		t.Fatalf("sent % x, want Appearing then the acceptance % x", out.frames, want)
	}
}
