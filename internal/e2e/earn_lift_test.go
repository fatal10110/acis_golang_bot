package e2e

import (
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPenaltyLift checks what TestLiveEarn's lift steps read from the bot's
// events (#583): the Scroll: Recovery bought (the most it carried) and its
// object, a read (its count fell), the lift message and the adena carried
// last.
func TestPenaltyLift(t *testing.T) {
	const scroll, object, adenaObject = 8594, 7001, 7000
	list := botclient.ItemList{Items: []botclient.Item{{ObjectID: adenaObject, TemplateID: adena, Count: 7400}}}
	buy := botclient.InventoryUpdate{Changes: []botclient.ItemChange{
		{State: botclient.ItemModified, Item: botclient.Item{ObjectID: adenaObject, TemplateID: adena, Count: 5100}},
		{State: botclient.ItemAdded, Item: botclient.Item{ObjectID: object, TemplateID: scroll, Count: 1}},
	}}
	use := botclient.InventoryUpdate{Changes: []botclient.ItemChange{
		{State: botclient.ItemRemoved, Item: botclient.Item{ObjectID: object, TemplateID: scroll, Count: 1}},
	}}
	lifted := botclient.SystemMessage{ID: msgDeathPenaltyLifted}
	other := botclient.SystemMessage{ID: msgDeathPenaltyLevel, Number: 1}
	tests := []struct {
		name   string
		events []botclient.Event
		want   earnLift
	}{
		{"nothing seen", nil, earnLift{}},
		{"entry list, no scroll", []botclient.Event{list}, earnLift{adena: 7400}},
		{"bought", []botclient.Event{list, buy}, earnLift{carried: 1, bought: 1, object: object, adena: 5100}},
		{"bought and read", []botclient.Event{list, buy, use}, earnLift{bought: 1, object: object, read: true, adena: 5100}},
		{"bought, read, lifted", []botclient.Event{list, buy, use, lifted}, earnLift{bought: 1, object: object, read: true, lifted: true, adena: 5100}},
		{"another penalty message lifts nothing", []botclient.Event{list, buy, other}, earnLift{carried: 1, bought: 1, object: object, adena: 5100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make([]Seen, len(tt.events))
			for i, ev := range tt.events {
				seen[i] = Seen{Event: ev}
			}
			if got := penaltyLift(seen, scroll); got != tt.want {
				t.Errorf("penaltyLift = %+v, want %+v", got, tt.want)
			}
		})
	}
}
