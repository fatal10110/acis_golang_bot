package botclient

import (
	"math"
	"testing"
	"time"
)

// TestFollowHoldsThroughLeaderLegInsideRepath replays the leader of the
// TestLiveWhisperCommands flake (#486): the server walked the GM's click in
// two legs, and the follower stood at its slot for over 3 s while the first
// leg kept the GM within Repath of the slot's anchor, then took its slot again
// once the second leg carried the GM past it. A follower standing still is
// no settled one while its leader still moves.
func TestFollowHoldsThroughLeaderLegInsideRepath(t *testing.T) {
	b := newFollowBrain(t)
	// A leader at 60 units a second walks its 240-unit leg in 4 s.
	b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM", Pos: followGM(0, 0), RunSpeed: 60, WalkSpeed: 30, Running: true}, t0)
	a, due := repath(t, b, at(time.Second))
	slot := checkSlot(t, b, a, followGM(0, 0), math.Pi)
	b.world.Apply(StopMove{ObjectID: selfID, Pos: slot}, due)

	b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(0, 0), Dest: followGM(240, 0)}, at(2*time.Second))
	for now := at(2500 * time.Millisecond); !now.After(at(6 * time.Second)); now = now.Add(500 * time.Millisecond) {
		b.follow.armed = false
		followHold(t, b, now)
		if !b.follow.at.IsZero() {
			lead := b.world.Objects[gmID].Position(now)
			t.Fatalf("re-path scheduled at %v with the leader at %v, %.0f from the anchor", now.Sub(t0), lead, math.Sqrt(float64(dist2(lead, followGM(0, 0)))))
		}
	}

	// The second leg: past Repath 60 units on, a second later.
	b.world.Apply(MoveToLocation{ObjectID: gmID, Origin: followGM(240, 0), Dest: followGM(400, 0)}, at(6*time.Second))
	a, due = repath(t, b, at(7*time.Second))
	checkSlot(t, b, a, b.world.Objects[gmID].Position(due), math.Pi)
}
