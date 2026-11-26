package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestStatusEarning checks Status.Earning (#530): set while the bot holds
// an earning spot (claimEarn), clear while it holds an ordinary one or
// none, and clear again once the penalty lifted turns its earning spot
// into an ordinary one.
func TestStatusEarning(t *testing.T) {
	toads := []datapack.Spot{earnSpot("toads", 2000)}
	tests := []struct {
		name     string
		self     squire
		dp       int32
		lift     bool
		wantSpot string
		want     bool
	}{
		{name: "penalized, nothing fits: earning", self: penal6, dp: 15, wantSpot: "toads", want: true},
		{name: "no penalty, the spot fits: ordinary", self: squire6, wantSpot: "toads"},
		{name: "penalized, nothing fits, lifted: ordinary", self: penal6, dp: 15, lift: true, wantSpot: "toads"},
		{name: "no penalty, nothing fits: none", self: penal6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEarnBrain(t, tt.self, tt.dp, map[int32]float64{toadNPC: 20}, toads...)
			linkBrain(b, standing{})
			now := at(time.Second)
			b.decide(now)
			if tt.lift {
				b.world.Apply(SystemMessage{ID: msgDeathPenaltyLifted}, now)
				now = now.Add(spotRetry)
				b.decide(now)
			}
			b.publishStatus(now, true)
			if st := b.Status(); st.Spot != tt.wantSpot || st.Earning != tt.want {
				t.Fatalf("Status Spot %q Earning %v, want %q %v", st.Spot, st.Earning, tt.wantSpot, tt.want)
			}
		})
	}
}
