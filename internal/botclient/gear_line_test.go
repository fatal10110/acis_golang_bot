package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestBrainGearWarriorLine has a warrior of each line, wearing a sword and
// shield and carrying a pole and a two-handed blunt, wear its line's weapon
// from its compiled build (docs/strategy.md, row WA column 1): the pole on
// the Warlord line, the two-handed blunt on the Gladiator line.
func TestBrainGearWarriorLine(t *testing.T) {
	const spear, staff = int32(220), int32(221)
	tests := []struct {
		name   string
		second int32
		wear   int32
	}{
		{"Warlord line wears the pole", classWarlord, spear},
		{"Gladiator line wears the two-handed blunt", classGladiator, staff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSlotsBrain(t, classWarrior,
				wornAt(201, shortSword, partRHand), wornAt(204, buckler, partLHand),
				carried(spear, shortSpear, partLRHand), carried(staff, conjurorsStaff, partLRHand))
			b.persona.Gear.Classes, b.persona.Role = nil, RoleDamage
			b.cfg.account = accountFor(t, HumanFighter.ID, RoleDamage, line{first: classWarrior, second: tt.second})
			b.world.Self.Level, b.builtOK = 24, false
			runTrainerSteps(t, b, []trainerStep{
				{name: "wear the line's weapon", at: time.Second, wantRule: "gear", wantKind: actUseItem, wantPayload: botwire.EncodeUseItem(tt.wear)},
			})
		})
	}
}
