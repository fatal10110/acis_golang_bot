package botclient

import "time"

// Party duels (docs/pvp/party.md "Party duels", #424): the bot leading a
// party answers a party duel's challenge and turns a whispered duel into
// one; every member, leader or not, fights the other side under the duel
// rules. Only a party's leader may challenge one, and only the challenged
// party's leader sees the challenge (network/duel.go partyDuelSides,
// requestDuelStart); the server re-checks every member of both sides on
// the acceptance and refuses the whole duel when one cannot duel
// (duelRefusal). Everything here is the brain goroutine's, as the PvP world
// model is; the answer and the challenge are the X5 senders' (one answer
// per challenge, challengeEvery), so this adds no sender.

// partyMayDuel reports whether the bot may answer yes to a party duel, or
// challenge one, at now: it leads its party, and every member is in sight
// and passes the server's duel checks by what the bot sees of it (alive,
// out of combat stance, no flag, karma or store, on allowed ground; HP and
// MP at least half by the party window). A member out of sight may stand
// in a town or fight: the bot answers no rather than have the server
// refuse it.
func (b *Brain) partyMayDuel(now time.Time) bool {
	if !b.leading() {
		return false
	}
	for id, m := range b.world.Party.Members {
		o := b.world.object(id)
		if o == nil || o.Kind != KindPlayer || o.Dead || o.InCombat || !clean(o) || o.StoreType != 0 ||
			!b.duelGround(o.Position(now)) || !halfOf(m.HP, m.MaxHP) || !halfOf(m.MP, m.MaxMP) {
			return false
		}
	}
	return true
}

// partyMateNamed reports whether name is one of the bot's party members.
func (b *Brain) partyMateNamed(name string) bool {
	for _, m := range b.world.Party.Members {
		if m.Name == name {
			return true
		}
	}
	return false
}

// partyDuelLive reports whether id is a foe of the bot's party duel it may
// attack at now: fighting, from FightFrom, the bot not defeated, the foe
// named by a duel window, not defeated and no bot (D38).
func (b *Brain) partyDuelLive(id int32, now time.Time) bool {
	d := b.duel(now)
	if !d.Party || d.Phase != DuelFighting || now.Before(d.FightFrom) || d.Down {
		return false
	}
	down, foe := d.Foes[id]
	if !foe || down {
		return false
	}
	o := b.world.object(id)
	return o != nil && o.Kind == KindPlayer && !o.Dead && !b.cfg.markers.Marks(o.Name)
}

// partyDuelFoe is the foe the bot fights in party duel d at now, 0 for
// none: the one it fights while that one stays live, else the target its
// leader hit within attackerWindow (focus), else the nearest live foe; a new foe starts a fresh
// layer record of it. The bot defeated at 1 HP fights no more.
func (b *Brain) partyDuelFoe(d *Duel, now time.Time) int32 {
	s := &b.world.Self
	if s.MaxHP > 0 && s.HP <= 1 {
		d.Down = true
	}
	if b.partyDuelLive(d.Opponent, now) {
		return d.Opponent
	}
	id := int32(0)
	if a := b.assist; b.assisted() != 0 && now.Sub(a.seen) <= attackerWindow && b.partyDuelLive(a.target, now) {
		id = a.target
	} else {
		id = b.nearestPlayer(now, func(id int32, _ *Object) bool { return b.partyDuelLive(id, now) })
	}
	d.Opponent = id
	if r := b.pvp.opponents[id]; r != nil {
		r.fight = layerFight{}
	}
	return id
}
