package botclient

import (
	"slices"
	"sync/atomic"
	"time"
)

// OutcomeQueueLen is how many spot stay records an OutcomeQueue holds for
// its writer (docs/structure.md, Design section 6): past it a record is
// dropped and counted, never waited for.
const OutcomeQueueLen = 1024

// stayHitsMax bounds the monsters a stay follows from the bot's first
// landed hit to their death: a monster past it is not followed.
const stayHitsMax = 256

// Outcome is one spot stay (OP0, D47): what a bot farming a spot gained and
// lost there, from its first arrival until it gave the spot up, with the
// model's estimate it chose the spot by. It names no account, character,
// password or chat text; the spot is its maker's name and centre, never the
// registry's index.
type Outcome struct {
	// Start is when the bot first arrived at the spot.
	Start time.Time `json:"start"`
	// Spot is the spot's maker name, X and Y its centre.
	Spot string `json:"spot"`
	X    int32  `json:"x"`
	Y    int32  `json:"y"`
	// Class, Level and Role are the bot's at arrival; Margin is its row's
	// spot margin at arrival (spotMargin), 0 for a row without one.
	Class  int32   `json:"class"`
	Level  int32   `json:"level"`
	Role   string  `json:"role"`
	Margin float64 `json:"margin,omitempty"`
	// Party is the party's size at arrival, the bot included: 1 solo.
	Party int `json:"party"`
	// Minutes are the minutes at the spot (only while arrived), Combat the
	// fighting minutes among them (useMeter.fought).
	Minutes float64 `json:"minutes"`
	Combat  float64 `json:"combat_minutes"`
	// Exp and Adena are what the bot gained at the spot, end less start of
	// each stretch there.
	Exp   int64 `json:"exp"`
	Adena int64 `json:"adena"`
	// Deaths are the bot's deaths in the stay.
	Deaths int `json:"deaths"`
	// HPLost is the sum of the drops in the bot's own HP at the spot, heals
	// not subtracted; HPPerCombatMinute that over Combat.
	HPLost            int64   `json:"hp_lost"`
	HPPerCombatMinute float64 `json:"hp_per_combat_minute"`
	// Potions are the HP potions (Persona.Potions) the bot drank in combat
	// at the spot.
	Potions int64 `json:"hp_potions"`
	// Kills are the monsters that died while the bot was engaged on them
	// (it landed a hit on them at the spot), Shared those another attacker
	// hit too; KillSeconds is the mean time from the bot's first landed hit
	// to the death over all of them, SoloKillSeconds over those not shared.
	Kills           int     `json:"kills"`
	Shared          int     `json:"kills_shared"`
	KillSeconds     float64 `json:"kill_seconds,omitempty"`
	SoloKillSeconds float64 `json:"solo_kill_seconds,omitempty"`
	// ModelKill and ModelDie are the model's seconds to kill one of the
	// spot's monsters and to die to its pack at arrival (packFight), nil
	// for a row with no margin or nothing to judge by.
	ModelKill *float64 `json:"model_kill_seconds,omitempty"`
	ModelDie  *float64 `json:"model_die_seconds,omitempty"`
	// Beside, Earning and Lower are the spot's flags at arrival: farmed
	// beside a full spot, an earning spot, under the bot's level window.
	Beside  bool `json:"beside"`
	Earning bool `json:"earning"`
	Lower   bool `json:"lower"`
}

// OutcomeQueue hands the spot stay records of every brain of a process to
// the one writer the command that opened the log runs. A brain never
// blocks on it: a full queue drops the record and counts the drop.
type OutcomeQueue struct {
	ch      chan Outcome
	dropped atomic.Int64
}

// NewOutcomeQueue returns an empty queue of OutcomeQueueLen records.
func NewOutcomeQueue() *OutcomeQueue {
	return &OutcomeQueue{ch: make(chan Outcome, OutcomeQueueLen)}
}

// Put queues o, or drops and counts it when the queue is full; it never
// blocks.
func (q *OutcomeQueue) Put(o Outcome) {
	select {
	case q.ch <- o:
	default:
		q.dropped.Add(1)
	}
}

// Records is what the writer reads the records from. It is never closed.
func (q *OutcomeQueue) Records() <-chan Outcome { return q.ch }

// Dropped is how many records a full queue dropped.
func (q *OutcomeQueue) Dropped() int64 { return q.dropped.Load() }

// spotStay is the spot stay the brain records (Outcome), owned by the brain
// goroutine. It opens when the bot first arrives at its spot (ruleSpot),
// runs while the spot's arrived is set, pauses on a teleport and resumes on
// the next arrival at the same spot, and ends when the bot gives the spot
// up (leaveSpot).
type spotStay struct {
	open bool
	idx  int
	rec  Outcome
	// since is when the running stretch began, zero while paused; fought,
	// exp and adena are the use meter's fighting minutes, the bot's
	// experience and its adena then, hp its HP at the last event.
	since  time.Time
	fought float64
	exp    int64
	adena  int64
	hp     int32
	// killSum and soloSum are the seconds from first hit to death of the
	// kills, all and not shared.
	killSum, soloSum float64
	// hits are the monsters the bot landed a hit on in the running
	// stretch, at most stayHitsMax.
	hits map[int32]stayHit
	// others are the monsters another attacker hit in the running stretch
	// before the bot landed a hit on them, at most stayHitsMax: a kill the
	// bot finishes on one of them is shared.
	others map[int32]struct{}
}

// stayHit is a monster the bot landed a hit on: when it first did and
// whether another attacker hit the monster too.
type stayHit struct {
	first  time.Time
	shared bool
}

func (st *spotStay) running() bool { return st.open && !st.since.IsZero() }

// stayArrive opens the stay at the spot the bot just arrived at, or resumes
// the one open there, at now. Nothing without an outcome queue.
func (b *Brain) stayArrive(now time.Time) {
	st, v := &b.stay, &b.spot
	if b.cfg.Outcomes == nil || b.cfg.Spots == nil {
		return
	}
	if st.open && st.idx != v.idx {
		b.stayEnd(now)
	}
	if !st.open {
		*st = spotStay{open: true, idx: v.idx, rec: b.stayRecord(now)}
	}
	if st.since.IsZero() {
		s := &b.world.Self
		st.since, st.fought, st.exp, st.adena, st.hp = now, b.meter.fought, s.Exp, int64(b.carried(adenaID)), s.HP
	}
}

// stayRecord is the record of a stay opening at now: the spot, the bot and
// the model's estimate at arrival.
func (b *Brain) stayRecord(now time.Time) Outcome {
	s, v := &b.world.Self, &b.spot
	sp := &b.cfg.Spots.spots[v.idx]
	bd := b.build()
	o := Outcome{
		Start: now.UTC(), Spot: sp.Maker, X: sp.Center.X, Y: sp.Center.Y,
		Class: s.ClassID, Level: s.Level, Role: b.persona.Role.String(), Margin: max(b.spotMargin(&bd), 0),
		Party:  1 + len(b.world.Party.Members),
		Beside: v.beside, Earning: v.earning, Lower: v.lower,
	}
	if !b.world.Party.In() {
		o.Party = 1
	}
	if margin := b.spotMargin(&bd); margin > 0 {
		c := b.partyCrew(now)
		if kill, die, ok := b.packFight(sp, &bd, margin, &c); ok {
			o.ModelKill, o.ModelDie = &kill, &die
		}
	}
	return o
}

// stayPause folds the running stretch into the record at now: the bot left
// the spot's centre by a teleport.
func (b *Brain) stayPause(now time.Time) {
	st := &b.stay
	if !st.running() {
		return
	}
	s, r := &b.world.Self, &st.rec
	r.Minutes += max(now.Sub(st.since).Minutes(), 0)
	r.Combat += b.meter.fought - st.fought
	r.Exp += s.Exp - st.exp
	r.Adena += int64(b.carried(adenaID)) - st.adena
	st.since, st.hits, st.others = time.Time{}, nil, nil
}

// stayEnd ends the stay at now and hands its record to the outcome queue:
// the bot gave its spot up.
func (b *Brain) stayEnd(now time.Time) {
	st := &b.stay
	if !st.open {
		return
	}
	b.stayPause(now)
	r := &st.rec
	if r.Combat > 0 {
		r.HPPerCombatMinute = float64(r.HPLost) / r.Combat
	}
	if r.Kills > 0 {
		r.KillSeconds = st.killSum / float64(r.Kills)
	}
	if solo := r.Kills - r.Shared; solo > 0 {
		r.SoloKillSeconds = st.soloSum / float64(solo)
	}
	b.cfg.Outcomes.Put(*r)
	*st = spotStay{}
}

// stayEvent feeds the open stay with ev at now, once the world applied it:
// the bot's deaths, and while the stay runs the drops in its HP and the
// monsters it hits and sees die.
func (b *Brain) stayEvent(ev Event, now time.Time) {
	st, self := &b.stay, b.world.Self.ObjectID
	if !st.open {
		return
	}
	if d, ok := ev.(Die); ok && d.ObjectID == self {
		st.rec.Deaths++
	}
	if !st.running() {
		return
	}
	if hp := b.world.Self.HP; hp < st.hp {
		st.rec.HPLost += int64(st.hp - hp)
	}
	st.hp = b.world.Self.HP
	switch e := ev.(type) {
	case Attack:
		for _, h := range e.Hits {
			if e.AttackerID == self {
				if !h.Missed() {
					b.stayHit(h.TargetID, now)
				}
			} else {
				b.otherHit(e.AttackerID, h.TargetID)
			}
		}
	case MagicSkillUse:
		if e.CasterID == self {
			b.stayHit(e.TargetID, now.Add(e.HitTime))
		} else {
			b.otherHit(e.CasterID, e.TargetID)
		}
	case Die:
		delete(st.others, e.ObjectID)
		h, ok := st.hits[e.ObjectID]
		if !ok {
			return
		}
		delete(st.hits, e.ObjectID)
		sec := max(now.Sub(h.first).Seconds(), 0)
		st.rec.Kills++
		st.killSum += sec
		if h.shared {
			st.rec.Shared++
		} else {
			st.soloSum += sec
		}
	case DeleteObject:
		delete(st.hits, e.ObjectID)
		delete(st.others, e.ObjectID)
	}
}

// stayHit follows monster id from the bot's first landed hit at at.
func (b *Brain) stayHit(id int32, at time.Time) {
	st := &b.stay
	if _, ok := st.hits[id]; ok {
		return
	}
	if o := b.world.Objects[id]; o == nil || o.Kind != KindNPC || o.Dead || len(st.hits) >= stayHitsMax {
		return
	}
	if st.hits == nil {
		st.hits = make(map[int32]stayHit)
	}
	_, shared := st.others[id]
	delete(st.others, id)
	st.hits[id] = stayHit{first: at, shared: shared}
}

// otherHit notes that attacker, not the bot, hit or cast on monster
// target: the bot's kill of it is shared, whether the bot hit it first or
// hits it later in the running stretch. A cast on itself and a monster's
// hit or cast (a monster healing or buffing another) share nothing; a
// summon's do.
func (b *Brain) otherHit(attacker, target int32) {
	if attacker == target {
		return
	}
	if o := b.world.Objects[attacker]; o != nil && o.Kind == KindNPC && !o.Summon {
		return
	}
	st := &b.stay
	if h, ok := st.hits[target]; ok {
		h.shared = true
		st.hits[target] = h
		return
	}
	if _, ok := st.others[target]; ok {
		return
	}
	if o := b.world.Objects[target]; o == nil || o.Kind != KindNPC || o.Dead || len(st.others) >= stayHitsMax {
		return
	}
	if st.others == nil {
		st.others = make(map[int32]struct{})
	}
	st.others[target] = struct{}{}
}

// stayDrank counts n of template id spent in combat (meterEvent) when it
// is an HP potion and the stay runs.
func (b *Brain) stayDrank(id, n int32) {
	if b.stay.running() && slices.Contains(b.persona.Potions, id) {
		b.stay.rec.Potions += int64(n)
	}
}
