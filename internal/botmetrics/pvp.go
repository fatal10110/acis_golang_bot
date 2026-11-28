package botmetrics

import (
	"slices"
	"sync/atomic"
)

// The PvP counters (docs/pvp/modes.md "Panel and metrics (X12)"), each
// keyed by a label of a fixed set; Snapshot holds every label of a set.
var (
	// pvpFightKinds label Snapshot.PvPFights: the opponent record's kind at
	// the bot's first attack on it.
	pvpFightKinds = [...]string{"duel", "fight_back", "arena", "flagged", "pk", "war", "olympiad"}
	// pvpDeathKinds label Snapshot.PvPDeaths: the killer's kind.
	pvpDeathKinds = [...]string{"fight_back", "arena", "flagged", "pk", "war", "olympiad", "unfought"}
	// duelResults label Snapshot.Duels.
	duelResults = [...]string{"won", "lost", "tie"}
	// interruptResults label Snapshot.PvPInterrupts.
	interruptResults = [...]string{InterruptCast, InterruptLanded, InterruptTooLate}
)

// The result labels of Snapshot.PvPInterrupts: an interrupt sent, then the
// opponent's cast canceled before its landing time, or the landing time
// passed first.
const (
	InterruptCast    = "cast"
	InterruptLanded  = "landed"
	InterruptTooLate = "too_late"
)

// pvpCounters are one bot's PvP counters, indexed like their label sets.
type pvpCounters struct {
	fights     [len(pvpFightKinds)]atomic.Uint64
	deaths     [len(pvpDeathKinds)]atomic.Uint64
	duels      [len(duelResults)]atomic.Uint64
	interrupts [len(interruptResults)]atomic.Uint64
}

// pvpSums are the PvP counters summed over the bots.
type pvpSums struct {
	fights     [len(pvpFightKinds)]uint64
	deaths     [len(pvpDeathKinds)]uint64
	duels      [len(duelResults)]uint64
	interrupts [len(interruptResults)]uint64
}

func (s *pvpSums) add(c *pvpCounters) {
	for i := range c.fights {
		s.fights[i] += c.fights[i].Load()
	}
	for i := range c.deaths {
		s.deaths[i] += c.deaths[i].Load()
	}
	for i := range c.duels {
		s.duels[i] += c.duels[i].Load()
	}
	for i := range c.interrupts {
		s.interrupts[i] += c.interrupts[i].Load()
	}
}

// fold adds src's counters to c.
func (c *pvpCounters) fold(src *pvpCounters) {
	for i := range src.fights {
		c.fights[i].Add(src.fights[i].Load())
	}
	for i := range src.deaths {
		c.deaths[i].Add(src.deaths[i].Load())
	}
	for i := range src.duels {
		c.duels[i].Add(src.duels[i].Load())
	}
	for i := range src.interrupts {
		c.interrupts[i].Add(src.interrupts[i].Load())
	}
}

// fill stores the sums in snap, every label present.
func (s *pvpSums) fill(snap *Snapshot) {
	snap.PvPFights = labeled(pvpFightKinds[:], s.fights[:])
	snap.PvPDeaths = labeled(pvpDeathKinds[:], s.deaths[:])
	snap.Duels = labeled(duelResults[:], s.duels[:])
	snap.PvPInterrupts = labeled(interruptResults[:], s.interrupts[:])
}

// labeled maps each of names to its count in n.
func labeled(names []string, n []uint64) map[string]uint64 {
	m := make(map[string]uint64, len(names))
	for i, name := range names {
		m[name] = n[i]
	}
	return m
}

// count adds one to the counter of label in set; a label of another set
// counts nothing.
func count(set []string, n []atomic.Uint64, label string) {
	if i := slices.Index(set, label); i >= 0 {
		n[i].Add(1)
	}
}

// PvPFight records the bot's first attack on an opponent of kind (duel,
// fight_back, arena, flagged, pk); another kind counts nothing.
func (b *Bot) PvPFight(kind string) {
	if b != nil {
		count(pvpFightKinds[:], b.pvp.fights[:], kind)
	}
}

// PvPDeath records the bot's death to a player or its summon by the
// killer's kind (fight_back, arena, flagged, pk, unfought); another kind
// counts nothing.
func (b *Bot) PvPDeath(kind string) {
	if b != nil {
		count(pvpDeathKinds[:], b.pvp.deaths[:], kind)
	}
}

// Duel records a duel's result (won, lost, tie); another result counts
// nothing.
func (b *Bot) Duel(result string) {
	if b != nil {
		count(duelResults[:], b.pvp.duels[:], result)
	}
}

// PvPInterrupt records an interrupt's step (InterruptCast, InterruptLanded,
// InterruptTooLate); another result counts nothing.
func (b *Bot) PvPInterrupt(result string) {
	if b != nil {
		count(interruptResults[:], b.pvp.interrupts[:], result)
	}
}
