package botclient

import (
	"testing"
	"time"
)

func TestAlarm(t *testing.T) {
	due := at(2 * time.Second)
	tests := []struct {
		name   string
		alarm  alarm
		spends []time.Time
		armed  bool
	}{
		{name: "armed, not spent", alarm: alarm{at: due, armed: true}, armed: true},
		{name: "disarmed, not spent", alarm: alarm{at: due}},
		{name: "spent before at", alarm: alarm{at: due, armed: true}, spends: []time.Time{due.Add(-time.Millisecond)}, armed: true},
		{name: "spent at at", alarm: alarm{at: due, armed: true}, spends: []time.Time{due}},
		{name: "spent after at", alarm: alarm{at: due, armed: true}, spends: []time.Time{due.Add(time.Second)}},
		{name: "spent twice", alarm: alarm{at: due, armed: true}, spends: []time.Time{due, due.Add(time.Second)}},
		{name: "zero at spent", alarm: alarm{armed: true}, spends: []time.Time{t0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.alarm
			for _, now := range tc.spends {
				a.spend(now)
			}
			got, armed := a.due()
			if got != tc.alarm.at || armed != tc.armed {
				t.Fatalf("due() = %v, %v; want %v, %v", got.Sub(t0), armed, tc.alarm.at.Sub(t0), tc.armed)
			}
		})
	}
}
