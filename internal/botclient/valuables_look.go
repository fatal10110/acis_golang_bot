package botclient

import "time"

// lookPhase is where the session's look at its private warehouse stands.
type lookPhase uint8

const (
	// lookIdle: no look runs; the next runs once lookReady holds, until
	// the private status is known.
	lookIdle lookPhase = iota
	// lookRunning: the look's private request runs (wh).
	lookRunning
	// lookBack: the request ended; the walk back to the anchor runs.
	lookBack
)

// valuablesLook is the session's look at its private warehouse
// (docs/valuables/ledger.md "Every session looks first"), owned by the
// brain goroutine. The look is done once the private status is known
// (wh.private.Known), which dies with the session; a cancelled or failed
// look leaves it unknown and runs again.
type valuablesLook struct {
	phase lookPhase
	// anchor is where the bot stood when the look started: the request's
	// Anchor and the end of the walk back.
	anchor Point
}

// wireValuables hands the bot's valuables value to this session's brain:
// its stored-units accessor reads this session's private status, and the
// warehouse errand reads and fills its noPrivate marks in place, so marks
// survive a reconnect (docs/valuables/warehouse.md "Keeper marks"). A
// round still open from an earlier session is marked stale for this
// session's first decision to end (endStaleRound).
func (b *Brain) wireValuables() {
	v := b.cfg.valuables
	v.stored = b.storedValuables
	if v.noPrivate == nil {
		v.noPrivate = make(map[int32]bool)
	}
	b.wh.noPrivate = v.noPrivate
	b.round.stale = v.round.open
}

// storedValuables are the units of every valuable the private warehouse
// holds, by entry key, from the private status's rows that pass the
// warehouse row test (valuableRow), and whether the status is known.
func (b *Brain) storedValuables() (map[valuableKey]int64, bool) {
	st := &b.wh.private
	if !st.Known {
		return nil, false
	}
	out := make(map[valuableKey]int64)
	for _, r := range st.Contents {
		if r.Count > 0 && b.valuableRow(r) {
			out[valuableKey{template: r.TemplateID, enchant: r.Enchant}] += int64(r.Count)
		}
	}
	return out, true
}

// lookReady reports whether the session's look may start at now
// (docs/valuables/round.md "Start", conditions 1, 2, 4 and 5): the state
// checks a store opening has (tradeReady), a registry town, an unmarked
// private keeper within RoundReach, no errand running, and an open due.
// It reads no region, ad board, RoundGap or budget.
func (b *Brain) lookReady(now time.Time) bool {
	if !b.tradeReady(now) || b.errandRunning() {
		return false
	}
	if !b.wh.openDue(now) || now.Before(b.wh.retryAt) {
		return false
	}
	at := b.world.Self.Position(now)
	if _, ok := b.cfg.Stores.town(at); !ok {
		return false
	}
	return b.privateKeeper(at)
}

// errandRunning reports whether an errand storeReady does not check runs:
// a warehouse request, a market visit, a gear UseItem, a crafter cycle, an
// enchant exchange, an unsettled clan visit, or a store opening or open
// (docs/valuables/round.md "Start", condition 4).
func (b *Brain) errandRunning() bool {
	return b.wh.running || b.market.active() || b.gear.obj != 0 || b.craft.phase != craftIdle ||
		b.ench.step != enchantIdle || b.member.asked || b.store.active() || b.world.Self.StoreType != 0
}

// privateKeeper reports whether a keeper spawn not marked noPrivate stands
// within RoundReach of at.
func (b *Brain) privateKeeper(at Point) bool {
	m := b.cfg.Travel
	reach := int64(b.persona.Valuables.RoundReach)
	for npcID := range m.keepers {
		if b.wh.noPrivate[npcID] {
			continue
		}
		for _, s := range m.spawns[npcID] {
			if dist2(at, s) <= reach*reach {
				return true
			}
		}
	}
	return false
}

// startLook starts the look's private request at the nearest unmarked
// keeper within RoundReach of anchor. It reports whether one runs; one
// that does not leaves nothing queued.
func (b *Brain) startLook(anchor Point, now time.Time) bool {
	b.wh.privateWant = WarehouseRequest{Look: true, Private: true, Anchor: anchor, Reach: b.persona.Valuables.RoundReach}
	if !b.startWarehouse(now) {
		b.wh.privateWant = WarehouseRequest{}
		return false
	}
	return true
}

// lookTurn is rule "valuables"'s turn for the session's look
// (docs/valuables/ledger.md "Every session looks first"). Before the
// private status is known and once lookReady holds, it starts the look and
// steps it (stepWarehouse), holding the turn. A request ended on a new
// noPrivate mark is started again at once from where the bot stands while
// an unmarked keeper remains. Then it walks back to within anchorArrive of
// where the look started and lets go. Called away (called), it ends the
// look as the round's cut-off does (docs/valuables/round-end.md "Cut-off"):
// cancelPrivate, no walk back; a request already sent is stepped to its
// answer.
//
// Pacing: one look a session, its open within the errand's open gap
// (openDue); legs and clicks as rule "warehouse"'s.
func (b *Brain) lookTurn(now time.Time) (Action, bool) {
	l, e := &b.look, &b.wh
	switch l.phase {
	case lookIdle:
		if b.persona.Valuables == (ValuablePolicy{}) || e.private.Known || !b.lookReady(now) {
			return Action{}, false
		}
		l.anchor = b.world.Self.Position(now)
		if !b.startLook(l.anchor, now) {
			return Action{}, false
		}
		l.phase = lookRunning
		fallthrough
	case lookRunning:
		if b.called() && (e.cancelPrivate() || !e.running) {
			l.phase = lookIdle
			return Action{}, false
		}
		if b.world.Self.InCombat {
			return Action{}, false
		}
		if e.running {
			marks := len(e.noPrivate)
			if a, ok := b.stepWarehouse(now); ok || e.running {
				return a, true
			}
			if b.called() {
				l.phase = lookIdle
				return Action{}, false
			}
			if len(e.noPrivate) > marks && b.privateKeeper(l.anchor) && b.startLook(l.anchor, now) {
				if a, ok := b.stepWarehouse(now); ok || e.running {
					return a, true
				}
			}
		}
		l.phase = lookBack
		fallthrough
	default:
		if b.called() {
			l.phase = lookIdle
			return Action{}, false
		}
		if a, st := b.goTo(l.anchor, anchorArrive, walkPlace, now); st == walkWalking {
			return a, true
		}
		l.phase = lookIdle
		return Action{}, false
	}
}
