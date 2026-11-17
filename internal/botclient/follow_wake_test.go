package botclient

import (
	"testing"
	"time"
)

// followNotice takes the bot to its slot, moves the leader 300 units on and
// lets a regular tick at notice notice it, with a fixed re-path delay; it
// returns when the re-path is due.
func followNotice(t *testing.T, b *Brain, notice time.Time, delay time.Duration) time.Time {
	t.Helper()
	_, due := repath(t, b, at(time.Second))
	b.world.Apply(StopMove{ObjectID: selfID, Pos: b.follow.dest}, due)
	b.world.Apply(StopMove{ObjectID: gmID, Pos: Point{X: 2300}}, notice.Add(-time.Second))
	b.persona.Follow.DelayMin, b.persona.Follow.DelayMax = delay, delay
	b.reactAt, b.nextTick = time.Time{}, notice
	if err := b.wake(notice); err != nil {
		t.Fatal(err)
	}
	if b.lastRule != "follow" || b.follow.dueAt != notice.Add(delay) || !b.follow.wake {
		t.Fatalf("after the leader moved: rule %q, re-path at %v (wake %v), want follow scheduling it at %v",
			b.lastRule, b.follow.dueAt.Sub(t0), b.follow.wake, notice.Add(delay).Sub(t0))
	}
	return b.follow.dueAt
}

// TestBrainFollowWake: the leader moving wakes the bot for the re-path
// without postponing a regular tick due sooner or the reaction to the bot
// being hit meanwhile; the re-path's own wake-up survives both and is spent
// once it runs.
func TestBrainFollowWake(t *testing.T) {
	b := newFollowBrain(t)
	notice := at(5 * time.Second)
	due := followNotice(t, b, notice, 3*time.Second)

	b.nextTick = notice.Add(time.Second)
	if got := b.decisionAt(); got != b.nextTick {
		t.Fatalf("decision at %v after the leader moved, want the tick at %v", got.Sub(t0), b.nextTick.Sub(t0))
	}
	b.nextTick = notice.Add(time.Minute)
	if got := b.decisionAt(); got != due {
		t.Fatalf("decision at %v, want the re-path at %v", got.Sub(t0), due.Sub(t0))
	}

	hitAt := notice.Add(10 * time.Millisecond)
	b.handleOne(t, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, hitAt)
	react := b.decisionAt()
	if d := react.Sub(hitAt); d < b.cfg.ReactMin || d > b.cfg.ReactMax {
		t.Fatalf("reacts %v after being hit, want %v-%v", d, b.cfg.ReactMin, b.cfg.ReactMax)
	}
	if err := b.wake(react); err != nil {
		t.Fatal(err)
	}
	if got := b.decisionAt(); got.After(due) {
		t.Fatalf("next decision at %v, want the re-path by %v", got.Sub(t0), due.Sub(t0))
	}

	sent := b.pace.lastSent
	b.nextTick = due.Add(time.Minute)
	if err := b.wake(due); err != nil {
		t.Fatal(err)
	}
	if b.lastRule != "follow" || b.pace.lastSent != due {
		t.Fatalf("at the re-path: rule %q, last sent %v (was %v), want follow moving at %v",
			b.lastRule, b.pace.lastSent.Sub(t0), sent.Sub(t0), due.Sub(t0))
	}
	if b.follow.wake || !b.decisionAt().After(due) {
		t.Fatalf("re-path wake-up not spent: decision at %v", b.decisionAt().Sub(t0))
	}
}

// TestBrainFollowWakeAppearing: a re-path due while the pacer holds back
// the Appearing a teleport asks for spends its wake-up all the same, so the
// next decision is the Appearing's retry rather than the past re-path time.
func TestBrainFollowWakeAppearing(t *testing.T) {
	b := newFollowBrain(t)
	due := followNotice(t, b, at(5*time.Second), time.Second)
	b.appearing = true
	b.pace.refill(due)
	b.pace.tokens = 0
	if err := b.wake(due); err != nil {
		t.Fatal(err)
	}
	if !b.appearing {
		t.Fatal("Appearing went out with no pacer token")
	}
	if got := b.decisionAt(); b.follow.wake || !got.After(due) {
		t.Fatalf("re-path wake-up not spent: wake %v, decision at %v, want after %v", b.follow.wake, got.Sub(t0), due.Sub(t0))
	}
}
