package e2e

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The weight overload scenario's load (#488): Necklace of Anguish Chains
// (items/1800-1899.xml) are a stackable material weighing 60 a piece that
// a fighter neither wears nor keeps.
const (
	anguishChain       int32 = 1896
	anguishChainWeight int32 = 60
)

// Server answers that refuse a RequestDropItem (network/inventory.go
// dropLiveItem; serverpackets/systemmessage.go).
const (
	msgCannotDiscardThisItem   int32 = 98
	msgDiscardDistanceTooFar   int32 = 151
	msgNotAuthorizedToDoThat   int32 = 794
	msgCannotDiscardInShopMode int32 = 1065
)

// dropRefused is what refuses a RequestDropItem the bot sends from now on.
func dropRefused(w *Watch) RefusalCheck {
	return RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestDropItem },
		Messages: []int32{msgCannotDiscardThisItem, msgDiscardDistanceTooFar, msgNotAuthorizedToDoThat, msgCannotDiscardInShopMode},
	}
}

// TestLiveWeightOverload runs a level 1 fighter in town that a GM gives a
// stack of chains taking it past its weight limit (#488): the server puts
// it in the immobile band, and the bot drops the chains where it stands at
// once, ending under the half-speed band with no chain left.
func TestLiveWeightOverload(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("wo", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{level: 1}, squiresSword, squiresShirt, squiresPants)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		warriorGear(p)
		// Neither buys nor sells: the chains can only go by the drop.
		p.Gear.Buy, p.Shop = false, botclient.ShopPolicy{}
	})
	toTown(t, ctx, tg, gm, f)

	var cur, limit int32
	until(t, ctx, f.name+" shows its weight limit", func() string {
		if cur, limit = f.load(); limit <= 0 {
			return "no weight limit yet"
		}
		return ""
	})
	// 117 % of the limit, as hz1 carried.
	count := (int32(int64(limit)*117/100)-cur)/anguishChainWeight + 1
	from := f.seen()
	check := dropRefused(f.watch)
	gmGive(t, ctx, gm, f, anguishChain, count)

	untilServed(t, ctx, f.name+" drops the chains, under the half-speed band", f.watch, check, func() string {
		var have int32
		for _, s := range f.watch.Seen(0) {
			if n, _, ok := stock(s.Event, anguishChain); ok {
				have = n
			}
		}
		c, l := f.load()
		if have != 0 || l <= 0 || int64(c)*1000 >= int64(l)*666 {
			return fmt.Sprintf("%d chains, load %d/%d", have, c, l)
		}
		return ""
	})
	peak := int64(0)
	for _, s := range f.watch.Seen(from) {
		switch e := s.Event.(type) {
		case botclient.UserInfo:
			peak = max(peak, int64(e.Load)*100/int64(max(e.MaxLoad, 1)))
		case botclient.StatusUpdate:
			if e.ObjectID == f.id && e.Set&botclient.StatusLoad != 0 {
				peak = max(peak, int64(e.Load)*100/int64(limit))
			}
		}
	}
	if peak < 100 {
		t.Fatalf("%s: load peaked at %d %% of %d, want the grant to pass 100 %%", f.name, peak, limit)
	}
	if !slices.ContainsFunc(f.watch.Sent()[check.From:], func(s Sent) bool { return s.Op == botclient.OpRequestDropItem }) {
		t.Fatalf("%s: shed the chains without a RequestDropItem", f.name)
	}
	c, l := f.load()
	t.Logf("%s: peaked at %d %%, shed to %d/%d", f.name, peak, c, l)
}
