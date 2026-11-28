package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// servitorOrderRetry spaces the servitor orders (docs/servitors.md section
// 2, "Pacing"). Pet orders have no server gate (only the connection's 160
// frames a second, network/clientstats.go), so the frame bucket
// (frameRate, frameBurst) bounds them, and on top of it: an attack order
// (22) onto one target goes out again only when no Attack or MoveToPawn of
// the servitor toward it arrived within servitorOrderRetry, a stop order
// (23) goes out once per give-up, and a refusal (1864) holds every order
// for servitorOrderRetry.
const servitorOrderRetry = 2 * time.Second

// msgPetRefusingOrder is the server's refusal of an order to a servitor out
// of control (summon/command.go OutcomeRefusedOutOfControl,
// serverpackets/systemmessage.go SystemMessagePetRefusingOrder).
const msgPetRefusingOrder int32 = 1864

// servitorOrders is the record of the orders the bot gave its servitor.
// The brain goroutine owns it.
type servitorOrders struct {
	// servitor is the servitor the last order went to; target the target
	// of its last attack order since the last stop, sent at at.
	servitor, target int32
	at               time.Time
	// stopped is the target the last stop order gave up, 0 for none since
	// the last attack order: one stop per give-up.
	stopped int32
	// held holds every order until then after a refusal.
	held time.Time
}

// liveServitor is the bot's servitor while it is out and alive, else nil:
// no order goes to a dead or absent servitor, so a bot without one sends
// none (D24).
func (b *Brain) liveServitor() *Servitor {
	if s := b.world.Servitor; s != nil && !s.Dead {
		return s
	}
	return nil
}

// servitorOrder is the attack order (RequestActionUse 22) that sends the
// servitor onto id, the bot's target, at now: the server gives the
// servitor its owner's target (summon/command.go resolveAttack), so the
// caller selects id first. None (ok false) without a live servitor, when
// the servitor's target is id already, when id is no monster (never a
// player or a player's summon, owner question 5), while a refusal holds
// the orders, or within servitorOrderRetry of a 22 onto id the servitor
// did not answer. Its bound: the frame bucket and servitorOrderRetry.
func (b *Brain) servitorOrder(id int32, now time.Time) (Action, bool) {
	s, o := b.liveServitor(), &b.servitorOrders
	switch {
	case s == nil, s.Target == id, b.world.Target != id, !b.world.monster(id), now.Before(o.held):
		return Action{}, false
	case o.servitor == s.ObjectID && o.target == id && now.Sub(o.at) < servitorOrderRetry:
		return Action{}, false
	}
	return Action{kind: actServitorAttack, payload: botwire.EncodeRequestActionUse(actionServitorAttack), object: id}, true
}

// servitorStop is the stop order (RequestActionUse 23) that recalls the
// servitor from target at now, a target the bot gives up, 0 for whatever
// it holds: the servitor goes idle and back to following its owner
// (summon/live_accessors.go, ai/summon.go). None without a live servitor
// holding a target, while a refusal holds the orders, or once one went out
// for this give-up. Its bound: the frame bucket and one per give-up.
func (b *Brain) servitorStop(target int32, now time.Time) (Action, bool) {
	s, o := b.liveServitor(), &b.servitorOrders
	switch {
	case s == nil, s.Target == 0, target != 0 && s.Target != target, now.Before(o.held):
		return Action{}, false
	case o.servitor == s.ObjectID && o.stopped == s.Target:
		return Action{}, false
	}
	return Action{kind: actServitorStop, payload: botwire.EncodeRequestActionUse(actionServitorStop), object: s.Target}, true
}

// servitorSent records an order a sent at now.
func (b *Brain) servitorSent(a Action, now time.Time) {
	o := &b.servitorOrders
	if s := b.world.Servitor; s != nil {
		o.servitor = s.ObjectID
	}
	switch a.kind {
	case actServitorAttack:
		o.target, o.at, o.stopped = a.object, now, 0
	case actServitorStop:
		o.target, o.stopped = 0, a.object
	}
}

// servitorRefused holds the orders for servitorOrderRetry after the
// server refused one with SystemMessage id at now; a stop it refused may
// go out again after the hold.
func (b *Brain) servitorRefused(id int32, now time.Time) {
	if id != msgPetRefusingOrder || b.world.Servitor == nil {
		return
	}
	o := &b.servitorOrders
	o.held, o.stopped = now.Add(servitorOrderRetry), 0
}

// servitorLeading reports whether the first attack on id waits for the
// servitor at now (ClassKit.ServitorLead): Persona.ServitorLead after its
// order onto id, cut short by its first swing at id, so the servitor holds
// the monster's hate. No order onto id (a held, refused or needless one)
// no wait.
func (b *Brain) servitorLeading(id int32, now time.Time) bool {
	s, o := b.liveServitor(), &b.servitorOrders
	return s != nil && o.servitor == s.ObjectID && o.target == id && s.Struck != id &&
		now.Sub(o.at) < b.persona.ServitorLead
}

// ruleServitor gives the servitor the orders no fight step gives
// (docs/servitors.md section 2, "Rule order"): it recalls the servitor
// (23) from a target the bot may no longer fight (another player's claim,
// a blacklist, a creature out of sight), and guards the bot: with the
// servitor idle and a monster hitting the bot or the servitor, the bot
// selects that monster and orders the servitor onto it (22), since the
// server never makes a servitor defend (summon/attacked.go). It waits
// while an attack order awaits its answer or a refusal holds the orders,
// and does nothing without a live servitor.
func (b *Brain) ruleServitor(now time.Time) (Action, bool) {
	s, o := b.liveServitor(), &b.servitorOrders
	if s == nil {
		return Action{}, false
	}
	if s.Target != 0 {
		if b.mayFight(s.Target, now) {
			return Action{}, false
		}
		return b.servitorStop(s.Target, now)
	}
	if now.Before(o.held) || o.servitor == s.ObjectID && o.target != 0 && now.Sub(o.at) < servitorOrderRetry {
		return Action{}, false
	}
	id := b.servitorGuardTarget(s, now)
	if id == 0 {
		return Action{}, false
	}
	if b.world.Target != id {
		self := &b.world.Self
		if self.Casting(now) || b.castPending(now) {
			// A new selection waits for the cast in flight.
			return Action{}, false
		}
		return selectAction(id, self.Position(now)), true
	}
	return b.servitorOrder(id, now)
}

// servitorGuardTarget is the monster the guard sends the idle servitor s onto at
// now: the bot's target when it hits the bot or the servitor, else the
// bot's hardest hitter, else the servitor's last attacker, 0 for none. A
// target the bot fights already is its fight step's to order (Brain.fight).
func (b *Brain) servitorGuardTarget(s *Servitor, now time.Time) int32 {
	hitsServitor := func(id int32) bool {
		return id != 0 && id == s.AttackedBy && now.Sub(s.AttackedAt) <= attackerWindow
	}
	if t := b.world.Target; b.world.monster(t) && b.mayFight(t, now) {
		if b.engaged == t || b.opened == t {
			return 0
		}
		if b.world.attacking(t, now) || hitsServitor(t) {
			return t
		}
	}
	for _, id := range b.attackers(now) {
		if b.world.monster(id) {
			return id
		}
	}
	if id := s.AttackedBy; hitsServitor(id) && b.world.monster(id) && b.mayFight(id, now) {
		return id
	}
	return 0
}
