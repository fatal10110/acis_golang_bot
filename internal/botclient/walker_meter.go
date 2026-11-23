package botclient

import (
	"math"
	"time"
)

// walkMeter measures the walk in progress for the walking-efficiency
// metrics (docs/walking.md, "Measurement (#304)"). It is part of the walker,
// owned by the brain goroutine, and changes nothing the walker sends.
type walkMeter struct {
	// arrivedAt is the modeled arrival (SelfState.arrival) of the walk's last
	// segment that arrived, until the next segment goes out; zero for none.
	arrivedAt time.Time
	// start is when the walk's first segment went out; length the summed
	// horizontal length of its segments, segs how many went out.
	start  time.Time
	length float64
	segs   int
}

// meterSegment records the walker's segment going out at now: the idle
// since the previous segment of the walk arrived, and the walk's length so
// far. The first segment of a walk, and one after a segment that did not
// arrive (blocked, desync, watchdog), has no idle.
func (b *Brain) meterSegment(now time.Time) {
	m, seg := &b.walk.meter, &b.walk.next
	if !m.arrivedAt.IsZero() {
		b.metrics.NavWalkIdle(now.Sub(m.arrivedAt))
		m.arrivedAt = time.Time{}
	}
	if m.segs == 0 {
		m.start = now
	}
	m.segs++
	m.length += math.Sqrt(float64(horizontal2(seg.from, seg.to)))
}

// meterArrived ends the measured walk at now, the decision that saw the bot
// arrive: its efficiency is the route length over the elapsed time since
// its first segment times the run speed. A walk without a segment of its own
// is not measured.
func (b *Brain) meterArrived(now time.Time) {
	m := &b.walk.meter
	defer func() { *m = walkMeter{} }()
	run := b.world.Self.RunSpeed
	elapsed := now.Sub(m.start)
	if m.segs == 0 || run <= 0 || elapsed <= 0 {
		return
	}
	permille := int64(math.Round(1000 * m.length / (elapsed.Seconds() * float64(run))))
	b.metrics.NavWalkEfficiency(permille)
	b.log.Debug("walk arrived", "segments", m.segs, "length", int(math.Round(m.length)),
		"elapsed", elapsed.Round(time.Millisecond), "run_speed", run, "efficiency_permille", permille)
}
