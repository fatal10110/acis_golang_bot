package botclient

import (
	"testing"
	"time"
)

// TestStoreLeavesOnOrder checks that a merchant in store mode told to
// follow or assist another player quits its store, counts what it sold and
// gives its registry claim back, holds until the server shows the store
// closed, and then hands over to the follow and assist rules.
func TestStoreLeavesOnOrder(t *testing.T) {
	quit := encodeRequestPrivateStoreQuitSell()
	opened := func(step storeStep) func(b *Brain) {
		return func(b *Brain) {
			id, _ := b.cfg.Stores.claim(Point{})
			b.store.claim, b.store.claimed, b.store.step = id, true, step
			b.store.adenaAtOpen = 10
			b.world.Self.StoreType = storeTypeSell
		}
	}
	closed := func(b *Brain) { b.world.Self.StoreType = 0 }
	tests := []struct {
		name  string
		setup func(b *Brain)
		// steps run from a minute in; the first is the order.
		steps []trainerStep
		// wantSold is the sales counted once the store is down.
		wantSold int64
	}{
		{
			name:  "GM follow order",
			setup: opened(storeOpen),
			steps: []trainerStep{
				{
					name: "order", at: time.Minute,
					setup:    func(b *Brain) { b.orders.follow = pcID },
					events:   []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: adenaStack(40)}}}},
					wantRule: "store", wantKind: actStoreQuit, wantPayload: quit,
				},
				{name: "store still shown", at: time.Minute + time.Second, wantRule: "store"},
				{name: "store closed: follow", at: time.Minute + 2*time.Second, setup: closed, wantRule: "follow"},
				{name: "stand up to follow", at: time.Minute + 3*time.Second, wantRule: "follow", wantKind: actSitStand},
			},
			wantSold: 30,
		},
		{
			name:  "GM assist order",
			setup: opened(storeOpen),
			steps: []trainerStep{
				{name: "order", at: time.Minute, setup: func(b *Brain) { b.orders.assist = pcID }, wantRule: "store", wantKind: actStoreQuit, wantPayload: quit},
				// Back to play, and no store opens while it assists.
				{name: "store closed", at: time.Minute + 2*time.Second, setup: closed},
			},
		},
		{
			name:  "party leader",
			setup: opened(storeOpen),
			steps: []trainerStep{
				{
					name: "joined", at: time.Minute,
					setup: func(b *Brain) {
						b.world.Party = Party{LeaderID: pcID, Members: map[int32]*PartyMate{pcID: {}}}
					},
					wantRule: "store", wantKind: actStoreQuit, wantPayload: quit,
				},
				{name: "store closed: follow", at: time.Minute + 2*time.Second, setup: closed, wantRule: "follow"},
			},
		},
		{
			name: "opening under way",
			setup: func(b *Brain) {
				opened(storeAskManage)(b)
				b.world.Self.StoreType = 0
				b.store.waitUntil = at(2 * time.Minute)
			},
			steps: []trainerStep{
				{name: "order", at: time.Minute, setup: func(b *Brain) { b.orders.follow = pcID }, wantRule: "store", wantKind: actStoreQuit, wantPayload: quit},
				{name: "follow", at: time.Minute + time.Second, wantRule: "follow"},
			},
		},
		{
			name:  "store the bot did not open",
			setup: func(b *Brain) { b.world.Self.StoreType = storeTypeSell },
			steps: []trainerStep{
				{name: "order", at: time.Minute, setup: func(b *Brain) { b.orders.follow = pcID }, wantRule: "store", wantKind: actStoreQuit, wantPayload: quit},
				// One quit per Retry while the server still shows it.
				{name: "store still shown", at: time.Minute + time.Second, wantRule: "store"},
				{name: "store closed: follow", at: time.Minute + 2*time.Second, setup: closed, wantRule: "follow"},
			},
		},
		{
			name:  "no order stays",
			setup: opened(storeOpen),
			steps: []trainerStep{
				{name: "own position", at: time.Minute, setup: func(b *Brain) { b.orders.follow = b.world.Self.ObjectID }, wantRule: "store"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStoreBrain(t, 1000, adenaStack(10), skinLoot)
			b.world.Self.Sitting = true
			tt.setup(b)
			runTrainerSteps(t, b, tt.steps)
			st := &b.store
			if tt.name == "no order stays" {
				if st.step != storeOpen || !st.claimed || b.cfg.Stores.Open() != 1 {
					t.Fatalf("store %+v, registry %d open, want it open and claimed", *st, b.cfg.Stores.Open())
				}
				return
			}
			if st.active() || st.claimed || b.cfg.Stores.Open() != 0 || st.sold != tt.wantSold || !st.nextAt.Equal(at(time.Minute+5*time.Minute)) {
				t.Errorf("store %+v, registry %d open, want it ended, released, %d sold and the next opening Retry away", *st, b.cfg.Stores.Open(), tt.wantSold)
			}
		})
	}
}
