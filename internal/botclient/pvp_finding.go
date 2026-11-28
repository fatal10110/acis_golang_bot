package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Finding fights (docs/pvp/finding.md, X5): the persona defaults that bound
// the bot's duels. Its RequestDuelStart has no server gate
// (network/duel.go requestDuelStart), so challengeEvery bounds it, shared by
// its own and its whispered challenges, and challengeSameEvery bounds the
// challenges of one player; a decline or refusal of its challenge holds its
// own challenges of that player declineHold. With Challenges set, both
// per-player holds also hold every other bot's own challenges (#425). An
// unexplained 1940 holds its answers and own challenges duelBlockHold or
// until it moved duelBlockMove away. duelClear is the margin past a monster's aggro range a duel keeps
// (any hit between a duellist and an NPC cancels it), and duelRange the
// server's challenge range (duel.Range, 3D). A whispered duel the challenge
// rule reaches whisperLapse past its reaction (an earlier rule held) is
// dropped.
const (
	challengeEvery     = 3 * time.Minute
	challengeSameEvery = 30 * time.Minute
	declineHold        = 2 * time.Hour
	duelClear          = 300
	duelBlockHold      = 10 * time.Minute
	duelBlockMove      = 2000
	duelRange          = 2000
	whisperLapse       = 30 * time.Second
)

// findState is the memory of the bot's duel finding: when its last
// RequestDuelStart went out (own or whispered), when it challenged each
// player last, the players its own challenges hold off and until when, the
// unexplained 1940 hold (when and where it came) and the whispered duel
// waiting for its reaction.
type findState struct {
	lastAsk  time.Time
	asked    map[string]time.Time
	held     holds[string]
	blockAt  time.Time
	blockPos Point
	whisper  duelWhisper
}

// duelWhisper is a player's whispered duel: its name and when the PvP
// reaction to it is due.
type duelWhisper struct {
	name string
	due  time.Time
}

// recent reports whether the bot challenged name within challengeSameEvery
// of now.
func (f *findState) recent(name string, now time.Time) bool {
	at, ok := f.asked[name]
	return ok && now.Sub(at) < challengeSameEvery
}

// hold holds the bot's own challenges of name declineHold from now.
func (f *findState) hold(name string, now time.Time) {
	f.held.set(name, now.Add(declineHold))
}

// heardDuel takes a player's whispered duel at now (docs/pvp/finding.md
// "Sources"): the challenge rule answers it at the PvP reaction. No
// authority is asked: anyone may whisper it, as a join (D20). One heard in
// combat stance is dropped, as is one waiting when a stance starts.
func (b *Brain) heardDuel(t tell, now time.Time) {
	if b.world.Self.InCombat {
		return
	}
	b.pvp.find.whisper = duelWhisper{name: t.name, due: b.pvpReact(now)}
}

// halfOf reports whether v is at least half of most, as the server's duel
// check reads HP and MP (network/duel.go duelRefusal); a maximum not
// reported yet passes.
func halfOf(v, most int32) bool { return most <= 0 || 2*int64(v) >= int64(most) }

// duelReady reports whether the bot's own state lets it duel by its
// pre-checks: out of combat stance (AutoAttackStart and AutoAttackStop),
// alive, HP and MP at least half, no flag, karma or store
// (network/duel.go duelRefusal).
func (b *Brain) duelReady() bool {
	s := &b.world.Self
	return !s.InCombat && !s.Dead && halfOf(s.HP, s.MaxHP) && halfOf(s.MP, s.MaxMP) &&
		s.PvPFlag == 0 && s.Karma == 0 && s.StoreType == 0
}

// duelGround reports whether a duellist may stand at p
// (docs/pvp/finding.md "Where a duel is allowed"): in no peace, town, Derby
// track, arena, no-restart or water zone, nor in the siege polygon the bot
// saw a running siege in (283, siegeState.field).
func (b *Brain) duelGround(p Point) bool {
	if n := b.cfg.Nav; n != nil && n.Water != nil && n.Water.Contains(int(p.X), int(p.Y), int(p.Z)) {
		return false
	}
	m := b.cfg.Travel
	if m == nil {
		return true
	}
	zs := &m.zones
	for _, set := range [...][]datapack.Zone{zs.Peace, zs.Towns, zs.DerbyTracks, zs.Arenas, zs.NoRestart} {
		if zoneHolds(set, p) {
			return false
		}
	}
	i := b.pvp.siege.field
	return i <= 0 || i > len(zs.Sieges) || !zs.Sieges[i-1].Contains(p.X, p.Y, p.Z)
}

// zoneHolds reports whether a zone of set holds p.
func zoneHolds(set []datapack.Zone, p Point) bool {
	for _, z := range set {
		if z.Contains(p.X, p.Y, p.Z) {
			return true
		}
	}
	return false
}

// clearOfMonsters reports whether the bot may start a duel at now
// (docs/pvp/finding.md "Clear of monsters"): no NPC attacking it and no
// aggressive monster in sight within its aggro range plus duelClear.
func (b *Brain) clearOfMonsters(now time.Time) bool {
	for id := range b.world.Attackers {
		if o := b.world.object(id); o != nil && o.Kind == KindNPC && b.world.attacking(id, now) {
			return false
		}
	}
	from := b.world.Self.Position(now)
	for _, d := range b.aggroDiscs(now) {
		if dist2(from, d.at) <= d.r*d.r {
			return false
		}
	}
	return true
}

// duelBlocked reports whether an unexplained 1940 still holds the bot's
// answers and own challenges at now: less than duelBlockHold ago and less
// than duelBlockMove from where it came.
func (b *Brain) duelBlocked(now time.Time) bool {
	f := &b.pvp.find
	if f.blockAt.IsZero() {
		return false
	}
	if now.Sub(f.blockAt) >= duelBlockHold || dist2(b.world.Self.Position(now), f.blockPos) >= duelBlockMove*duelBlockMove {
		f.blockAt = time.Time{}
		return false
	}
	return true
}

// unableSeen takes a 1940 (the bot's side cannot duel) at now: one its own
// pre-checks cannot explain (duelReady, duelGround) starts the duel block;
// one they explain (a combat stance, HP or MP under half) holds nothing.
func (b *Brain) unableSeen(now time.Time) {
	p := b.world.Self.Position(now)
	if b.duelReady() && b.duelGround(p) {
		b.pvp.find.blockAt, b.pvp.find.blockPos = now, p
	}
}

// mayDuel reports whether the bot may answer yes to a duel, or challenge,
// at now: ready by its pre-checks, on allowed ground, clear of monsters and
// not held by an unexplained 1940.
func (b *Brain) mayDuel(now time.Time) bool {
	return b.duelReady() && b.duelGround(b.world.Self.Position(now)) && b.clearOfMonsters(now) && !b.duelBlocked(now)
}

// foeMayDuel reports whether player id may duel the bot at now by what the
// bot sees of it: in sight within duelRange, alive, out of combat stance,
// with no flag, karma or store, on allowed ground.
func (b *Brain) foeMayDuel(id int32, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer || o.Dead || o.InCombat || !clean(o) || o.StoreType != 0 {
		return false
	}
	p := o.Position(now)
	return b.duelGround(p) && dist3(b.world.Self.Position(now), p) <= duelRange
}

// answerDuel answers the open challenge when the persona takes duels
// (docs/pvp/finding.md "Sources"): yes at the PvP reaction when the bot may
// duel, the challenger is no bot (D38) and the bot has no challenge of its
// own out; else no at once. A party duel's yes also needs the bot leading a
// party that may duel (partyMayDuel, docs/pvp/party.md "Party duels"). One
// RequestDuelAnswerStart per challenge (the server takes the first),
// bounded by the frame limits.
func (b *Brain) answerDuel(now time.Time) (Action, bool) {
	a := b.pvp.asked
	if !b.persona.PvP.Duels || a.at.IsZero() || now.Sub(a.at) > duelAskOpen {
		return Action{}, false
	}
	d := b.duel(now)
	yes := d.Phase == DuelAsked && d.Party == a.party && !d.Challenged && !b.cfg.markers.Marks(d.Name) && b.mayDuel(now) &&
		(!a.party || b.partyMayDuel(now))
	if yes && now.Before(a.due) {
		return Action{}, false
	}
	return Action{kind: actDuelAnswer, payload: botwire.EncodeRequestDuelAnswerStart(a.party, yes), yes: yes}, true
}

// ruleChallenge is the bot's RequestDuelStart (docs/pvp/finding.md
// "Sources"), for a persona that takes duels, out of combat: first a
// whispered duel at its PvP reaction, then its own challenge of the nearest
// non-bot player of its tier that may duel and that the engage estimate
// passes, not held by a decline, only at its spot or on the road to it
// (roams). Both wait challengeEvery from the last RequestDuelStart and
// challengeSameEvery per player, an idle duel and the bot free to duel
// (mayDuel); a whisper that fails them is dropped, as is one past due by
// whisperLapse (an earlier rule held) or heard in combat stance. Its sender bound is
// challengeEvery (the server has no gate on the packet). The bot leading
// a party challenges a whisper to a party duel, once its party may duel
// (partyMayDuel, docs/pvp/party.md "Party duels").
func (b *Brain) ruleChallenge(now time.Time) (Action, bool) {
	f := &b.pvp.find
	if !b.persona.PvP.Duels {
		return Action{}, false
	}
	w := f.whisper
	if w.name != "" && !now.Before(w.due) {
		f.whisper = duelWhisper{}
	}
	if b.world.Self.InCombat {
		return Action{}, false
	}
	free := b.duel(now).Phase == DuelIdle && (f.lastAsk.IsZero() || now.Sub(f.lastAsk) >= challengeEvery) && !b.pvp.arena.active()
	if w.name != "" && !now.Before(w.due) && now.Sub(w.due) <= whisperLapse {
		if free && !b.cfg.markers.Marks(w.name) && !f.recent(w.name, now) && !b.avoided(w.name, now) && b.mayDuel(now) {
			// A party mate gets a one-on-one duel: the server refuses a
			// party duel within one party (network/duel.go partyDuelSides).
			if id := b.playerNamed(w.name); id != 0 && b.foeMayDuel(id, now) {
				if party := b.leading() && !b.partyMate(id); !party || b.partyMayDuel(now) {
					return b.challenge(id, party), true
				}
			}
		}
	}
	if !free || !b.roams() || !b.mayDuel(now) {
		return Action{}, false
	}
	if id := b.challengeFoe(now); id != 0 {
		return b.challenge(id, false), true
	}
	return Action{}, false
}

// roams reports whether the bot is on its own at its spot or on the road to
// it, where it may challenge on its own: farming a spot, on no errand, in
// no bot party and under no GM follow, assist or stay order.
func (b *Brain) roams() bool {
	o := &b.orders
	return b.spot.farming() && !b.onErrand() && !b.inBotParty() && !o.stay && o.follow == 0 && o.assist == 0
}

// challengeFoe is the player the bot challenges on its own at now, 0 for
// none: the nearest one in sight of its class tier, no bot, not held by a
// decline nor challenged within challengeSameEvery, that may duel and that
// the engage estimate passes. The process-wide holds (Challenges) are
// claimed last, so only the player chosen is recorded there.
func (b *Brain) challengeFoe(now time.Time) int32 {
	f, s := &b.pvp.find, &b.world.Self
	tier := classTier(s.ClassID)
	return b.nearestPlayer(now, func(id int32, o *Object) bool {
		return !b.cfg.markers.Marks(o.Name) && !f.recent(o.Name, now) && !f.held.held(o.Name, now) &&
			!b.avoided(o.Name, now) && classTier(o.ClassID) == tier && b.foeMayDuel(id, now) && b.engages(id) &&
			b.cfg.Challenges.claim(o.Name, s.ObjectID, now)
	})
}

// nearestPlayer is the player in sight nearest the bot at now that ok
// passes, 0 for none; ok runs nearest first, so a costly check runs only
// until one passes.
func (b *Brain) nearestPlayer(now time.Time, ok func(id int32, o *Object) bool) int32 {
	from := b.world.Self.Position(now)
	type cand struct {
		id int32
		d  int64
	}
	var cs []cand
	for id, o := range b.world.Objects {
		if o.Kind == KindPlayer && !o.Dead && id != b.world.Self.ObjectID {
			cs = append(cs, cand{id, dist2(from, o.Position(now))})
		}
	}
	slices.SortFunc(cs, func(x, y cand) int { return cmp.Or(cmp.Compare(x.d, y.d), cmp.Compare(x.id, y.id)) })
	for _, c := range cs {
		if ok(c.id, b.world.Objects[c.id]) {
			return c.id
		}
	}
	return 0
}

// challenge is the RequestDuelStart of player id, a one-on-one duel or a
// party duel (yes marks it).
func (b *Brain) challenge(id int32, party bool) Action {
	return Action{kind: actDuelStart, payload: botwire.EncodeRequestDuelStart(b.world.object(id).Name, party), object: id, yes: party}
}

// challengeSent keeps the bot's challenge a sent at now: the shared and
// per-player challenge clocks, the process-wide one (Challenges), and its
// duel asked with the player named, a party duel's when a.yes.
func (b *Brain) challengeSent(a Action, now time.Time) {
	o := b.world.object(a.object)
	if o == nil {
		return
	}
	f := &b.pvp.find
	if f.asked == nil {
		f.asked = make(map[string]time.Time)
	}
	f.lastAsk, f.asked[o.Name] = now, now
	b.cfg.Challenges.sent(o.Name, b.world.Self.ObjectID, now)
	b.pvp.duel = Duel{Phase: DuelAsked, Name: o.Name, Since: now, Challenged: true, Result: b.pvp.duel.Result, Party: a.yes}
}

// aboveStand reports whether the bot's HP and MP are at least its stand
// levels (Thresholds.StandHP, StandMP): what hunting and arena targets
// need.
func (b *Brain) aboveStand() bool {
	s, p := &b.world.Self, b.build().Thresholds
	return !below(s.HP, s.MaxHP, p.StandHP) && !below(s.MP, s.MaxMP, p.StandMP)
}

// noHunt reports whether p is ground the server refuses a hunting attack
// on (84, 85, model/zone/kinds.go): a peace zone, a town or the Derby
// track.
func (b *Brain) noHunt(p Point) bool {
	m := b.cfg.Travel
	if m == nil {
		return false
	}
	return zoneHolds(m.zones.Peace, p) || zoneHolds(m.zones.Towns, p) || zoneHolds(m.zones.DerbyTracks, p)
}

// ruleHunt is hunting (docs/pvp/targets.md "Hunting", PvPPolicy.Hunt): out
// of combat with no attacker, no duel, no karma and its HP and MP at its
// stand levels, off peace, town and Derby ground, the bot attacks the
// nearest flagged or karma player of its tier rule 6 lets it fight, or
// mutual-war enemy rule 6b does (docs/pvp/war.md "Hunting"), off
// that ground too, not avoided after a death (avoidAfterDeath), that the
// engage estimate passes; else, in PK mode, its PK victim (pkHunt). The
// fight is the fight rules' (fight): the attack's own frame pacing; PK
// mode's limits bound the PKs.
func (b *Brain) ruleHunt(now time.Time) (Action, bool) {
	p, s := b.persona.PvP, &b.world.Self
	hunt := p.Hunt && (p.FightFlagged || p.FightWar)
	if !(hunt || p.ForceAttack) || s.InCombat || s.Dead || s.Karma > 0 || b.duel(now).Phase != DuelIdle ||
		len(b.attackers(now)) > 0 || !b.aboveStand() || b.noHunt(s.Position(now)) {
		return Action{}, false
	}
	tier := classTier(s.ClassID)
	var id int32
	if hunt {
		id = b.nearestPlayer(now, func(id int32, o *Object) bool {
			k := b.fightRule(id, o, now)
			return b.mayFight(id, now) && (k == FightFlagged || k == FightWar) && classTier(o.ClassID) == tier &&
				!b.noHunt(o.Position(now)) && !b.avoided(o.Name, now) && b.engages(id)
		})
	}
	if id == 0 {
		// A victim past pkEvery, still in sight, is a new PK: its clock
		// restarts.
		if id = b.pkHunt(now); id != 0 && !b.pkVictim(id, now) {
			b.pkStart(id, now)
		}
	}
	if id == 0 {
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// engages is the engage estimate against player id (docs/pvp/model.md
// "Engage estimate"): the bot against the reference opponent of id's row
// (pvpFighters), with the bot row's spot margin (marginNormal without one),
// over the PvP row table (Brain.pvpRows). False without the bot's stats,
// the opponent's class template or a travel map.
func (b *Brain) engages(id int32) bool {
	bot, opp, ok := b.pvpFighters(id)
	if !ok {
		return false
	}
	margin := b.build().Kit.SpotMargin
	if margin <= 0 {
		margin = marginNormal
	}
	return charmodel.Engage(bot, opp, b.pvpRows(), margin)
}

// pvpFighters are the bot, by its reported stats, and the reference
// opponent of player id's row: its class template at the bot's level in
// the grade-typical gear of that level, a bow where the row shoots
// (Brain.pvpRef, docs/pvp/model.md).
// ok is false without the bot's stats, the opponent's class template or a
// travel map.
func (b *Brain) pvpFighters(id int32) (bot, opp charmodel.Fighter, ok bool) {
	m, o, s := b.cfg.Travel, b.world.object(id), &b.world.Self
	if m == nil || o == nil {
		return bot, opp, false
	}
	self := b.reported()
	if self.MaxHP <= 0 || self.PAtkSpd <= 0 {
		return bot, opp, false
	}
	level := max(int(s.Level), 1)
	var row charmodel.Row
	if r := b.pvp.opponents[id]; r != nil {
		row = r.Row
	}
	if opp, ok = b.pvpRef(m, o.ClassID, row.Bow, level); !ok {
		return bot, opp, false
	}
	opp.Row = row
	bot = charmodel.Fighter{Profile: self, MaxCP: float64(s.MaxCP), Level: level, CON: int(s.CON), MEN: int(s.MEN)}
	bot.Row, _ = charmodel.ClassRow(s.ClassID, b.bowEquipped())
	return bot, opp, true
}
