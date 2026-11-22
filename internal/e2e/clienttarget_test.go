package e2e

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestClientTarget checks that a Client keeps its target from the events
// it reads: the server's selection, and only its own deselection clears it.
func TestClientTarget(t *testing.T) {
	const self, bot, other int32 = 1, 2, 3
	tests := []struct {
		name   string
		events []botclient.Event
		want   int32
	}{
		{"none", nil, 0},
		{"selected", []botclient.Event{botclient.MyTargetSelected{ObjectID: bot}}, bot},
		{"selected anew", []botclient.Event{botclient.MyTargetSelected{ObjectID: bot}, botclient.MyTargetSelected{ObjectID: other}}, other},
		{"its own deselection", []botclient.Event{botclient.MyTargetSelected{ObjectID: bot}, botclient.TargetUnselected{ObjectID: self}}, 0},
		{"another's deselection", []botclient.Event{botclient.MyTargetSelected{ObjectID: bot}, botclient.TargetUnselected{ObjectID: other}}, bot},
		{"the target's own deselection", []botclient.Event{botclient.MyTargetSelected{ObjectID: bot}, botclient.TargetUnselected{ObjectID: bot}}, bot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Self: botclient.Self{ObjectID: self}}
			for _, ev := range tt.events {
				if err := c.follow(t.Context(), ev); err != nil {
					t.Fatal(err)
				}
			}
			if got := c.Target(); got != tt.want {
				t.Errorf("Target() = %d, want %d", got, tt.want)
			}
		})
	}
}
