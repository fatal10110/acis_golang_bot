package botclient

import (
	"fmt"
	"testing"
	"time"
)

// stuckWalk is a walk of errand rule toward an NPC spawn from the origin
// whose legs stall where the bot stands, the first leg at start: three
// legs, the sidesteps, the /unstuck at start+10s, refused, the walk given up
// escapeRetry later (stuckWalkEnd).
func stuckWalk(rule string, start time.Duration) []trainerStep {
	echo := []Event{MoveToLocation{ObjectID: selfID}}
	var steps []trainerStep
	for i := range 3 + len(sidestepTurns) {
		st := trainerStep{name: fmt.Sprintf("leg %d", i+1), at: start + time.Duration(i)*time.Second, wantRule: rule, wantKind: actMove}
		if i > 0 {
			st.events = echo
		}
		steps = append(steps, st)
	}
	escapeAt := start + time.Duration(3+len(sidestepTurns))*time.Second
	return append(steps,
		trainerStep{
			name: "/unstuck", at: escapeAt, events: echo, wantRule: rule, wantKind: actUserCommand,
			wantPayload: encodeRequestUserCommand(userCommandEscape),
		},
		trainerStep{name: "no cast yet", at: escapeAt + time.Second, wantRule: rule},
		trainerStep{name: "refused: given up", at: escapeAt + escapeRetry},
	)
}

// stuckWalkEnd is when the walk of stuckWalk(_, start) is given up.
func stuckWalkEnd(start time.Duration) time.Duration {
	return start + time.Duration(3+len(sidestepTurns))*time.Second + escapeRetry
}

// TestErrandWalkStuck has the Newbie Guide, shop and gatekeeper errands
// walk toward an NPC spawn the bot's legs get nowhere near, its sidesteps
// and /unstuck spent (#135): the errand starts no other walk to an NPC out
// of sight, the Newbie Guide's before the next level and the others' for
// Persona.WalkStuckFor, past the template shun, while an NPC already in
// sight is still visited: the back-off shuns no NPC.
func TestErrandWalkStuck(t *testing.T) {
	stuckFor := DefaultPersona().WalkStuckFor
	quiet := func(at time.Duration) trainerStep {
		return trainerStep{name: fmt.Sprintf("no walk at %v", at), at: at}
	}
	newbie := func(t *testing.T) *Brain { return newNewbieBrain(t, 10) }
	shop := func(t *testing.T) *Brain { return newShopBrain(t, adenaStack(5000), potionsOf(0), spiritsOf(100)) }
	travel := func(t *testing.T) *Brain { return newTravelBrain(t, 5000) }
	planTrip := trainerStep{name: "no spot in reach plans a trip", at: time.Second}
	merchantInSight := func(b *Brain) {
		b.world.Apply(NPCInfo{ObjectID: merchantObj, NPCID: 30004, Pos: Point{X: 200}}, t0)
		b.world.Target = merchantObj
	}
	gkInSight := func(b *Brain) {
		b.world.Self.move = stopAt(Point{X: 1900})
		b.world.Apply(NPCInfo{ObjectID: gkObj, NPCID: 30006, Pos: Point{X: 2000}}, t0)
		b.world.Target = gkObj
		b.spot.retryAt = time.Time{}
	}
	tests := []struct {
		name  string
		brain func(t *testing.T) *Brain
		// rule is the errand's rule; steps go before its walk.
		rule  string
		steps []trainerStep
		// start is when the walk's first leg goes out.
		start time.Duration
		// backedOff reports whether the errand backs its walks off after
		// the walk given up at end.
		backedOff func(b *Brain, end time.Time) bool
		// then are the steps after the walk was given up at end.
		then func(end time.Duration) []trainerStep
	}{
		{
			name: "newbie: until the next level", brain: newbie, rule: "newbie", start: time.Second,
			backedOff: func(b *Brain, _ time.Time) bool {
				n := &b.newbie
				return n.stuckLevel == 10 && n.shunned[30598].IsZero() && n.doneLevel == 0
			},
			then: func(end time.Duration) []trainerStep {
				return []trainerStep{
					quiet(end + time.Second),
					quiet(end + visitShunFor + time.Minute),
					quiet(end + 10*time.Hour),
					{
						name: "next level: walks again", at: end + 10*time.Hour + time.Second,
						setup:    func(b *Brain) { b.world.Self.Level = 11 },
						wantRule: "newbie", wantKind: actMove,
					},
				}
			},
		},
		{
			name: "newbie: a guide in sight is visited", brain: newbie, rule: "newbie", start: time.Second,
			then: func(end time.Duration) []trainerStep {
				return []trainerStep{
					quiet(end + time.Second),
					{name: "guide in sight: talk", at: end + 2*time.Second, setup: guideInSight, wantRule: "newbie", wantKind: actTalk},
				}
			},
		},
		{
			name: "shop: for WalkStuckFor", brain: shop, rule: "shop", start: time.Second,
			backedOff: func(b *Brain, end time.Time) bool {
				sh := &b.shop
				return sh.stuckUntil.Equal(end.Add(stuckFor)) && sh.nextAt.Equal(end.Add(shopRetry)) && sh.shunned[30004].IsZero()
			},
			then: func(end time.Duration) []trainerStep {
				return []trainerStep{
					quiet(end + shopRetry),
					quiet(end + visitShunFor + time.Minute),
					quiet(end + stuckFor - time.Minute),
					// The last check put the next visit off by shopRetry.
					{name: "backed off no more: walks again", at: end + stuckFor - time.Minute + shopRetry, wantRule: "shop", wantKind: actMove},
				}
			},
		},
		{
			name: "shop: a merchant in sight is visited", brain: shop, rule: "shop", start: time.Second,
			then: func(end time.Duration) []trainerStep {
				return []trainerStep{
					{name: "merchant in sight: talk", at: end + shopRetry, setup: merchantInSight, wantRule: "shop", wantKind: actTalk},
				}
			},
		},
		{
			name: "gatekeeper: for WalkStuckFor", brain: travel, rule: "travel", steps: []trainerStep{planTrip}, start: 2 * time.Second,
			backedOff: func(b *Brain, end time.Time) bool {
				return b.trip.stuckUntil.Equal(end.Add(stuckFor)) && b.trip.shunned[0].IsZero()
			},
			then: func(end time.Duration) []trainerStep {
				replan := func(b *Brain) { b.spot.retryAt = time.Time{} }
				return []trainerStep{
					{name: "no trip planned", at: end + visitShunFor + time.Minute, setup: replan},
					quiet(end + visitShunFor + time.Minute + time.Second),
					{name: "nor later", at: end + stuckFor - time.Minute, setup: replan},
					quiet(end + stuckFor - time.Minute + time.Second),
					{name: "backed off no more: plans", at: end + stuckFor, setup: replan},
					{name: "walks again", at: end + stuckFor + time.Second, wantRule: "travel", wantKind: actMove},
				}
			},
		},
		{
			name: "gatekeeper: one in sight is visited", brain: travel, rule: "travel", steps: []trainerStep{planTrip}, start: 2 * time.Second,
			then: func(end time.Duration) []trainerStep {
				return []trainerStep{
					{name: "gatekeeper in sight: plans", at: end + visitShunFor + time.Minute, setup: gkInSight},
					{name: "talk", at: end + visitShunFor + time.Minute + time.Second, wantRule: "travel", wantKind: actTalk},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.brain(t)
			runTrainerSteps(t, b, append(tt.steps, stuckWalk(tt.rule, tt.start)...))
			end := stuckWalkEnd(tt.start)
			if b.newbie.v.active() || b.shop.v.active() || b.trip.v.active() {
				t.Fatalf("visits newbie %+v shop %+v trip %+v, want all over", b.newbie.v, b.shop.v, b.trip.v)
			}
			if tt.backedOff != nil && !tt.backedOff(b, at(end)) {
				t.Fatalf("newbie %+v shop %+v trip %+v, want the errand backed off", b.newbie, b.shop, b.trip)
			}
			runTrainerSteps(t, b, tt.then(end))
		})
	}
}
