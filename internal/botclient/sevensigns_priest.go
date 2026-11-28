package botclient

import (
	"bytes"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The priest stop (X19, docs/pvp/cp-potions.md "X19, the priest trip",
// D43): in a seal validation Dusk won, a Seven Signs member short of its CP
// stock turns its seal stones into Ancient Adena at a Dusk Priestess and
// buys CP potions with it from her multisell 500.

// itemAncientAdena is Ancient Adena, what the stone exchange pays and
// multisell 500 takes.
const itemAncientAdena int32 = 5575

// Links of the validation greeting (dusk_priest_2a, 2c) and of the stone
// exchange page (signs_16_dusk) the stop follows.
const (
	priestShop     = "multisell 500"   // "Purchase consumable items."
	priestExchange = "SevenSigns 16"   // "Exchange seal stones."
	priestAllStone = "SevenSigns 17 4" // "Exchange all seal stones."
)

const (
	// priestListID is multisell 500, "Purchase consumable items"
	// (multisell/500.xml; its npcs hold every Dusk Priestess).
	priestListID int32 = 500
	// priestChooses caps the MultiSellChoose of a trip: the Greater CP row,
	// then the plain one.
	priestChooses = 2
	// multiSellMaxAmount is the most units one MultiSellChoose may ask for:
	// the server forgets the list in silence past it (multisell.go
	// maxAmount).
	multiSellMaxAmount = 9999
)

// priestRows are the CP rows of multisell 500 the stop trades, in order:
// the Greater CP Potion first, then the CP Potion for what is left, each
// with the CP Potions one unit is worth (cpHeld).
var priestRows = []struct{ item, units int32 }{
	{itemGreaterCPPotion, cpGreaterUnits},
	{itemCPPotion, 1},
}

// priestStep is where the priest stop is. The zero priestStep is no stop.
type priestStep uint8

const (
	priestNone priestStep = iota
	// priestExchanging: SevenSigns 16 went out, the exchange page awaited.
	priestExchanging
	// priestExchanged: SevenSigns 17 4 went out, its answer awaited, then
	// the priest is talked to again.
	priestExchanged
	// priestGreet: the talk after the exchange went out, the greeting
	// awaited.
	priestGreet
	// priestList: multisell 500 went out; the list awaited, then traded.
	priestList
	// priestChose: a MultiSellChoose went out, its InventoryUpdate awaited.
	priestChose
)

// sealStones is how many seal stones of any color the bot carries.
func (b *Brain) sealStones() int32 {
	return b.carried(itemBlueSealStone) + b.carried(itemGreenSealStone) + b.carried(itemRedSealStone)
}

// priestDue reports whether a priest stop is due at now: a member in a
// validation Dusk won (read lately), short of its CP stock (cpShort),
// holding seal stones or Ancient Adena, its last stop over signsFresh ago.
func (b *Brain) priestDue(now time.Time) bool {
	s := &b.signs
	return b.signsMode() && s.member && s.period == signsValidation && s.fresh(now) && !now.Before(s.stopUntil) &&
		b.cpShort() > 0 && (b.sealStones() > 0 || b.carried(itemAncientAdena) > 0)
}

// priestNext is the stop's next link on the priest's open page, "" when
// the stop ends or none is due. Only a page linking both multisell 500 and
// the stone exchange (2a, 2c) starts it or goes on to the list: the bot
// reads links, not text, and any other page ends the stop. It changes no
// state: the stop moves on once the link goes out (priestSent), so a link
// the pacer holds is offered again.
func (b *Brain) priestNext(now time.Time) string {
	s := &b.signs
	has := func(cmd string) bool { return b.world.Dialog.Has(npcBypass(s.v.obj, cmd)) }
	greeting := has(priestShop) && has(priestExchange)
	switch s.step {
	case priestExchanging:
		if has(priestAllStone) {
			return priestAllStone
		}
		return ""
	case priestNone:
		if !greeting || !b.priestDue(now) {
			return ""
		}
		if b.sealStones() > 0 {
			return priestExchange
		}
	case priestGreet:
		// The Ancient Adena the exchange paid is counted when the list
		// is traded, past the bypass gate: the InventoryUpdate that
		// reports it comes within the server's InventoryUpdateTick.
		if !greeting {
			return ""
		}
	default:
		return ""
	}
	return priestShop
}

// priestSent moves the stop on with a packet of the visit that went out at
// now, and only then, so a link, talk or choice the pacer holds is offered
// again: a link to where it goes, the first one starting the stop (one a
// signsFresh, however it ends); the talk after the stone exchange to the
// greeting; a MultiSellChoose past its row, awaiting its InventoryUpdate
// for trainerReplyTimeout.
func (s *signsErrand) priestSent(a Action, now time.Time) {
	link := func(cmd string) bool {
		return a.kind == actBypass && bytes.Equal(a.payload, botwire.EncodeRequestBypassToServer(npcBypass(s.v.obj, cmd)))
	}
	start := func(step priestStep) {
		if s.step == priestNone {
			s.stopUntil = now.Add(signsFresh)
		}
		s.step = step
	}
	switch {
	case a.kind == actMultiSellChoose:
		s.step, s.row, s.chooses, s.v.waitUntil = priestChose, s.row+1, s.chooses+1, now.Add(trainerReplyTimeout)
	case a.kind == actTalk:
		if s.step == priestExchanged {
			s.step = priestGreet
		}
	case link(priestAllStone):
		s.step = priestExchanged
	case link(priestExchange):
		start(priestExchanging)
	case link(priestShop):
		start(priestList)
	}
}

// priestTalk talks to the priest again once the stone exchange answered:
// that page links nothing the stop goes on with, and the server accepts
// multisell 500 only as a link of the last page it sent (bypass.go). The
// talk also keeps the priestess the server's last selected NPC, whose
// list a MultiSellChoose must match.
func (b *Brain) priestTalk(now time.Time) (Action, bool) {
	s := &b.signs
	o := b.world.object(s.v.obj)
	if o == nil || o.Dead || b.world.Target != s.v.obj {
		s.end(now, signsFresh)
		return Action{}, false
	}
	b.walk.talking(o.Position(now), now)
	return talkAction(s.v.obj, b.world.Self.Position(now)), true
}

// priestTrade trades the shown multisell 500: one MultiSellChoose per
// answer (the InventoryUpdate of the one before), the Greater CP row then
// the plain one, at most priestChooses a trip, each passing the multisell
// gate (gateMultiSell, multiSellGap above the server's MultisellTime). No
// InventoryUpdate within trainerReplyTimeout is a silent drop or a
// refusal (weight, slots, items): the stop ends, as it does once no row
// has an amount left.
func (b *Brain) priestTrade(now time.Time) (Action, bool) {
	s := &b.signs
	if s.step == priestChose {
		if now.Before(s.v.waitUntil) {
			return Action{}, true
		}
		s.end(now, signsFresh)
		return Action{}, false
	}
	for i := s.row; s.chooses < priestChooses && i < len(priestRows); i++ {
		r := priestRows[i]
		if id, n := b.priestAmount(r.item, r.units); n > 0 {
			// The row moves on once the choice goes out (priestSent).
			s.row = i
			return Action{kind: actMultiSellChoose, payload: botwire.EncodeMultiSellChoose(priestListID, id, n), object: s.v.obj}, true
		}
	}
	s.end(now, signsFresh)
	return Action{}, false
}

// priestAmount is the entry id of the shown list's row trading item, and
// how many units of it the stop buys: what the CP stock lacks (cpLack,
// itself bounded by the weight under Supply.BuyLoad) in whole units worth
// units CP Potions each, bounded by every ingredient the bot holds (the
// Ancient Adena, and the adena, the castle tax included, past the reserve:
// the server shows them per unit, multisell.Entry.prepare), by the weight
// left under the weight limit, and by the server's caps (multiSellMaxAmount,
// one unit of a non-stackable row). 0 when the list has no such row.
func (b *Brain) priestAmount(item, units int32) (int32, int32) {
	var e *MultiSellEntry
	for i := range b.signs.list {
		if r := &b.signs.list[i]; len(r.Products) == 1 && r.Products[0].ItemID == item && r.Products[0].Count > 0 {
			e = r
			break
		}
	}
	if e == nil {
		return 0, 0
	}
	per := int64(units) * int64(e.Products[0].Count)
	n := int64(b.cpLack()) / per
	for _, in := range e.Ingredients {
		if in.Count <= 0 {
			continue
		}
		held := int64(b.carried(in.ItemID))
		if in.ItemID == adenaID {
			held -= b.floor(tierSupplies)
		}
		n = min(n, max(held, 0)/int64(in.Count))
	}
	if self := &b.world.Self; self.MaxLoad > 0 && b.cfg.Travel != nil {
		if wt := int64(b.cfg.Travel.weights[item]) * int64(e.Products[0].Count); wt > 0 {
			n = min(n, max(int64(self.MaxLoad)-int64(self.Load), 0)/wt)
		}
	}
	if !e.Stackable {
		n = min(n, 1)
	}
	return e.ID, int32(min(n, multiSellMaxAmount))
}

// signsList takes a page of multisell 500 the stop asked for.
func (b *Brain) signsList(ev MultiSellList) {
	s := &b.signs
	if !s.v.active() || s.step != priestList || s.listed || ev.ListID != priestListID {
		return
	}
	s.list = append(s.list, ev.Entries...)
	s.listed = ev.Finished
}

// signsInventory moves the stop on with an InventoryUpdate reporting a CP
// potion: the answer to its MultiSellChoose.
func (b *Brain) signsInventory(ev InventoryUpdate) {
	s := &b.signs
	if s.step != priestChose {
		return
	}
	for _, c := range ev.Changes {
		if id := c.Item.TemplateID; id == itemCPPotion || id == itemGreaterCPPotion {
			s.step, s.v.waitUntil = priestList, time.Time{}
			return
		}
	}
}
