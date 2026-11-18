package botclient

import (
	"testing"
	"time"
)

// TestBrainStoreGivenLoot opens a store that lists an item the server
// shows in the manage list before the InventoryUpdate that brings it (a GM
// give, live #120): its loot is priced when the list goes out.
func TestBrainStoreGivenLoot(t *testing.T) {
	b := newStoreBrain(t, 1000, adenaStack(1000), skinLoot)
	manage := PrivateStoreManageListSell{OwnerID: selfID, Adena: 1000, Candidates: []StoreItem{skinCand, swordCand}}
	runTrainerSteps(t, b, []trainerStep{
		{name: "open", at: time.Second, wantRule: "store", wantKind: actStoreManage, wantPayload: encodeRequestPrivateStoreManageSell()},
		{name: "title", at: 2 * time.Second, events: []Event{manage}, wantRule: "store", wantKind: actStoreTitle, wantPayload: encodeSetPrivateStoreMsgSell("Loot for sale")},
		{
			name: "list", at: 3 * time.Second, events: []Event{InventoryUpdate{Changes: []ItemChange{{ItemAdded, swordLoot}}}},
			wantRule: "store", wantKind: actStoreList,
			wantPayload: encodeSetPrivateStoreListSell(false, []storeRow{{102, 10, 7}, {107, 1, 700}}),
		},
	})
}
