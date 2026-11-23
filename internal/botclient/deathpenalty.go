package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// EtcStatusUpdate is a character's miscellaneous status: its death
// penalty level (0-15), among flags the bot does not use. It carries no
// object id: the server sends the bot's own at entry and at every change,
// and broadcasts another player's to everyone that sees it when its Charm
// of Courage starts or ends (event.EtcStatusBroadcast,
// network/live_player_events.go). World.applyDeathPenalty sorts them out.
type EtcStatusUpdate struct {
	DeathPenalty int32
}

func (EtcStatusUpdate) event() {}

// decodeEtcStatusUpdate: seven int32s, charges, weight penalty, blocked,
// danger area, grade penalty, charm of courage and death penalty level
// (serverpackets/etcstatusupdate.go).
func decodeEtcStatusUpdate(r *wire.Reader) Event {
	r.Skip(6 * 4)
	return EtcStatusUpdate{DeathPenalty: r.ReadInt32()}
}

// System messages of the bot's own death penalty level, sent to it alone
// with every change (applyLiveDeathPenalty, network/live_player_events.go).
const (
	msgDeathPenaltyLevelAdded = 1916 // the new level is its number
	msgDeathPenaltyLifted     = 1917
)

// applyDeathPenalty keeps the bot's death penalty level. Only the
// session's first EtcStatusUpdate is surely the bot's own: the server
// sends it at EnterWorld, ahead of the spawn that lets another player's
// broadcast reach the bot (network/character_flow.go). A later one may be
// another player's, so every later level comes from the self-only system
// messages the server sends with each change: 1916 with the new level,
// 1917 when it is lifted.
func (w *World) applyDeathPenalty(ev Event) {
	s := &w.Self
	switch e := ev.(type) {
	case EtcStatusUpdate:
		if !s.deathPenaltyKnown {
			s.DeathPenalty, s.deathPenaltyKnown = e.DeathPenalty, true
		}
	case SystemMessage:
		switch e.ID {
		case msgDeathPenaltyLevelAdded:
			s.DeathPenalty = e.Number
		case msgDeathPenaltyLifted:
			s.DeathPenalty = 0
		}
	}
}

// recoveryScrolls are the Scroll: Recovery templates by character level
// band, each lifting one death penalty level (item skill 2286, effect
// Recovery; data/xml/items/8500-8599.xml). The skill's condition takes a
// character level of at least the band's floor and under the next one
// (data/xml/skills/2200-2299.xml #minLvl, #maxLvl); out of its band the
// server refuses the scroll (msg 113).
var recoveryScrolls = [...]struct{ minLevel, item int32 }{
	{76, 8599}, {61, 8598}, {52, 8597}, {40, 8596}, {20, 8595}, {0, 8594},
}

// recoveryReuse is how long the bot reads no Scroll: Recovery after one:
// the scroll's reuse_delay is 10 s, whether or not the last one lifted a
// level, and the server drops a use still cooling in silence
// (live.ItemDisabled, network/inventory.go).
const recoveryReuse = 11 * time.Second

// recoveryScroll is the Scroll: Recovery template a character of level
// reads.
func recoveryScroll(level int32) int32 {
	for _, s := range recoveryScrolls {
		if level >= s.minLevel {
			return s.item
		}
	}
	return recoveryScrolls[len(recoveryScrolls)-1].item
}

// liftsPenalty reports whether the bot lifts its death penalty with
// Scrolls: Recovery (RecoverPolicy.LiftDeathPenalty) and has one to lift.
func (b *Brain) liftsPenalty() bool {
	return b.persona.Recover.LiftDeathPenalty && b.world.Self.DeathPenalty > 0
}

// ruleRecover reads a Scroll: Recovery of the bot's level band while it
// has a death penalty level, carries one, and stands out of combat with no
// attacker and no cast of its own running. One UseItem goes out per
// recoveryReuse, the scroll's own reuse; the frame pacer bounds the rest.
func (b *Brain) ruleRecover(now time.Time) (Action, bool) {
	s := &b.world.Self
	if !b.liftsPenalty() || s.Dead || s.InCombat || s.Sitting || s.Casting(now) || len(b.attackers(now)) > 0 {
		return Action{}, false
	}
	id := recoveryScroll(s.Level)
	if at, used := b.usedAt[id]; used && now.Sub(at) < recoveryReuse {
		return Action{}, false
	}
	it, ok := b.world.Inventory.ByTemplate(id)
	if !ok || it.Count <= 0 {
		return Action{}, false
	}
	return useItemAction(it), true
}

// recoveryWants is the supplies tier's Scroll: Recovery purchase: one of
// the bot's level band per death penalty level, less those it carries,
// when a merchant sells it.
func (b *Brain) recoveryWants() []buyRow {
	if !b.liftsPenalty() || b.cfg.Travel == nil {
		return nil
	}
	id := recoveryScroll(b.world.Self.Level)
	if _, sold := b.cfg.Travel.cheapest[id]; !sold {
		return nil
	}
	if n := b.world.Self.DeathPenalty - b.carried(id); n > 0 {
		return []buyRow{{item: id, count: n}}
	}
	return nil
}

// keepsRecovery reports whether template id is the Scroll: Recovery of the
// bot's level band while it lifts its death penalty: the keep set holds it.
// A scroll of another band is of no use to the bot and is sold.
func (b *Brain) keepsRecovery(id int32) bool {
	return b.persona.Recover.LiftDeathPenalty && id == recoveryScroll(b.world.Self.Level)
}
