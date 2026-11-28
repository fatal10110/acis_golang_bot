package charmodel

import (
	"testing"
	"time"
)

// TestCastSpeedShortensReuse checks that the time between two casts scales
// with the speed that times them past the reuse too, as the server scales
// a skill's reuse by 333 over its cast or attack speed unless the skill has
// a static reuse (cast/controller.go buildPlan; no nuke of the shipped
// datapack has one): a reuse-bound nuke still casts more often with more
// cast speed, so Karmian's x1.15 counts for a sorcerer's Prominence (#676).
func TestCastSpeedShortensReuse(t *testing.T) {
	prominence := Attack{Magic: true, Power: 52, HitTime: 4 * time.Second, Reuse: 6 * time.Second}
	shot := prominence
	shot.Shot = BlessedSpiritshot
	bolt := Attack{Magic: true, Power: 10, HitTime: 4 * time.Second, Reuse: 2 * time.Second}
	strike := Attack{Power: 20, HitTime: 1080 * time.Millisecond, Reuse: 9 * time.Second}
	tests := []struct {
		name string
		p    Profile
		a    Attack
		want time.Duration
	}{
		{"reuse-bound at 333", Profile{MAtkSpd: 333}, prominence, 6000 * time.Millisecond},
		{"reuse-bound at 383 (Karmian)", Profile{MAtkSpd: 383}, prominence, 5216 * time.Millisecond},
		{"reuse-bound with a spiritshot at 383", Profile{MAtkSpd: 383}, shot, 5216 * time.Millisecond},
		{"cast-bound at 383", Profile{MAtkSpd: 383}, bolt, 3477 * time.Millisecond},
		{"physical, by attack speed", Profile{PAtkSpd: 666, MAtkSpd: 333}, strike, 4500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := castPeriod(tt.p, tt.a); got != tt.want {
				t.Errorf("castPeriod = %v, want %v", got, tt.want)
			}
		})
	}
}
