package botclient

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// The PvP counters and the panel's PvP view (docs/pvp/modes.md "Panel and
// metrics (X12)"). The brain goroutine keeps them; they change nothing the
// bot sends.

// String is the result's metric label: won, lost, tie, "" for none.
func (r DuelResult) String() string {
	switch r {
	case DuelWon:
		return "won"
	case DuelLost:
		return "lost"
	case DuelTie:
		return "tie"
	}
	return ""
}

// DuelTally counts the duels a bot decided this run, by result.
type DuelTally struct {
	Won, Lost, Tie int
}

// PvPStatus is the panel's PvP view of a bot: the modes its persona plays
// (PvPPolicy, the panel's switches applied), its PvP and PK kill counts as
// the server last sent them (UserInfo; the server's PvP count skips karma
// players under the shipped AwardPKKillPVPPoint = False), its duels this
// run and its last duel's result.
type PvPStatus struct {
	Modes             PvPPolicy
	PvPKills, PKKills int32
	Duels             DuelTally
	LastDuel          DuelResult
}

// pvpStatus is the bot's PvPStatus.
func (b *Brain) pvpStatus() *PvPStatus {
	s := &b.world.Self
	v := &PvPStatus{Modes: b.persona.PvP, PvPKills: s.PvPKills, PKKills: s.PKKills, LastDuel: b.pvp.duel.Result}
	if l := b.cfg.link; l != nil {
		v.Duels = l.duels
	}
	return v
}

// fightSent counts the bot's first attack on a player, a under way at now,
// by its record's kind (bot_pvp_fights_total): an attack, or a damage cast
// (guardSent's castDamage, which ran first). A record counts once per
// fight: a new duel with the same player counts again (Brain.duelWindow
// clears the record's fight).
func (b *Brain) fightSent(a Action, now time.Time) {
	if a.kind != actAttack && (a.kind != actCast || !b.pvp.guard.castDamage) {
		return
	}
	r := b.pvp.opponents[a.object]
	o := b.world.object(a.object)
	if r == nil || r.fight.kind != fightNone || o == nil || o.Kind != KindPlayer {
		return
	}
	k := b.fightKind(a.object, o, now)
	if k < FightDuel || k == FightUnfought {
		return
	}
	r.fight.kind = k
	b.metrics.PvPFight(k.String())
}

// deathCounted counts the bot's death to the player called killer, or to
// its summon, by the killer's kind (bot_pvp_deaths_total): the kind of the
// fight the bot started on it, else its record's kind. A hunted flagged
// player or a PK victim fighting back stays flagged or pk, though rule 6
// now calls it fight back. A killer out of sight, one whose record holds no
// kind, or one whose kind is a past duel's counts as unfought: the bot did
// not fight it under any rule.
func (b *Brain) deathCounted(killer string) {
	if killer == "" {
		return
	}
	k := FightUnfought
	if r := b.pvp.opponents[b.playerNamed(killer)]; r != nil {
		switch {
		case r.fight.kind > FightDuel:
			k = r.fight.kind
		case r.Kind > FightDuel:
			k = r.Kind
		}
	}
	b.metrics.PvPDeath(k.String())
}

// duelDecided counts the result of the bot's duel (bot_duels_total) and
// keeps it in the run's tally.
func (b *Brain) duelDecided(r DuelResult) {
	b.metrics.Duel(r.String())
	l := b.cfg.link
	if l == nil {
		return
	}
	switch r {
	case DuelWon:
		l.duels.Won++
	case DuelLost:
		l.duels.Lost++
	case DuelTie:
		l.duels.Tie++
	}
}

// interruptSent counts an interrupt cast sent on opponent id
// (bot_pvp_interrupts_total cast) and waits for its outcome until the
// landing time of the cast it answers.
func (b *Brain) interruptSent(id int32) {
	b.metrics.PvPInterrupt(botmetrics.InterruptCast)
	if r := b.pvp.opponents[id]; r != nil {
		r.fight.interrupting = r.LastCast.Lands
	}
}

// interruptEnded counts the outcome of r's interrupt waiting at now, on
// r's MagicSkillCanceled: landed before the landing time, too late after.
func (b *Brain) interruptEnded(r *Opponent, now time.Time) {
	if r.fight.interrupting.IsZero() {
		return
	}
	res := botmetrics.InterruptLanded
	if !now.Before(r.fight.interrupting) {
		res = botmetrics.InterruptTooLate
	}
	r.fight.interrupting = time.Time{}
	b.metrics.PvPInterrupt(res)
}

// interruptsDue counts as too late every interrupt whose landing time
// passed by now with no cancel. An opponent gone from sight before it
// counts neither outcome.
func (b *Brain) interruptsDue(now time.Time) {
	for _, r := range b.pvp.opponents {
		if t := r.fight.interrupting; !t.IsZero() && !now.Before(t) {
			r.fight.interrupting = time.Time{}
			b.metrics.PvPInterrupt(botmetrics.InterruptTooLate)
		}
	}
}
