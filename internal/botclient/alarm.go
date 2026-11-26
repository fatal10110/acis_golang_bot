package botclient

import "time"

// alarm is a wake-up armed for at: it pulls decisionAt earlier, never
// later, until a decision runs at or after at and spends it. Its owner
// keeps reading at for its own logic; armed only says whether the wake-up
// is still pending.
type alarm struct {
	at    time.Time
	armed bool
}

// due is the time the alarm wakes the brain at and whether it still does.
func (a *alarm) due() (time.Time, bool) {
	return a.at, a.armed
}

// spend disarms the alarm once at is not after now; at stays as it is.
func (a *alarm) spend(now time.Time) {
	if !now.Before(a.at) {
		a.armed = false
	}
}
