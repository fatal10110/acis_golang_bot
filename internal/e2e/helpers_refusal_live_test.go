package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// System messages the server refuses a buy or an item use with
// (acis_golang internal/gameserver/network/serverpackets/systemmessage.go).
const (
	msgS1CannotBeUsed      int32 = 113
	msgSlotsFull           int32 = 129
	msgNotEnoughAdena      int32 = 279
	msgWeightLimitExceeded int32 = 422
	msgNotAuthorized       int32 = 794
	msgQuantityExceeded    int32 = 1036
)

// merchantBuyRefused is what refuses a RequestBuyItem (network/merchant.go
// requestBuyItem). The ActionFailed it adds to an overflow follows one of
// these, and the bot's clicks on the merchant get ActionFailed too, so the
// step does not count it.
func merchantBuyRefused(w *Watch) RefusalCheck {
	return RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestBuyItem },
		Messages: []int32{msgQuantityExceeded, msgWeightLimitExceeded, msgSlotsFull, msgNotEnoughAdena},
	}
}

// storeBuyRefused is what refuses a RequestPrivateStoreBuy from store
// (network/privatestore.go buyFromStore, answerDeal); the server answers
// no refusal of it with ActionFailed.
func storeBuyRefused(w *Watch, store int32) RefusalCheck {
	return RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestPrivateStoreBuy && s.Target == store },
		Messages: []int32{msgNotAuthorized, msgNotEnoughAdena, msgWeightLimitExceeded, msgSlotsFull},
	}
}

// untilServed is until with a refusal check over w: it also fails the test
// at once when the server refuses the step's packet, with the refusal and
// the last packets and events around it, instead of waiting out ctx.
func untilServed(t *testing.T, ctx context.Context, what string, w *Watch, check RefusalCheck, report func() string) {
	t.Helper()
	wctx, cancel := context.WithCancel(ctx)
	refused := make(chan *Refusal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if r, err := w.AwaitRefusal(wctx, check); err == nil {
			refused <- r
		}
	}()
	defer func() {
		cancel()
		<-done
	}()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		missing := report()
		if missing == "" {
			return
		}
		select {
		case r := <-refused:
			t.Fatalf("%s: %v; missing: %s", what, r, missing)
		case <-ctx.Done():
			t.Fatalf("%s: %v; missing: %s", what, ctx.Err(), missing)
		case <-tick.C:
		}
	}
}

// escapeRefused is what refuses reading a Scroll of Escape, one of objects
// (network/inventory.go useItem and the skill's use conditions). A read
// the bot's death or an attack cuts short ends with ActionFailed, and the
// scenarios let the bot walk home after it, so the step does not count it.
func escapeRefused(w *Watch, objects map[int32]bool) RefusalCheck {
	return RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpUseItem && objects[s.Target] },
		Messages: []int32{msgS1CannotBeUsed},
	}
}
