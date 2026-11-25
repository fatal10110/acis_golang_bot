package botclient

import (
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestSoloMPCost checks that the solo spot fit (spotAttack) and the gear
// model (gearRole) charge a cast its mpInitialConsume plus its mpConsume,
// as the server's CanCast does (cast/controller.go) and the party fit
// already did (#499): 1011 Heal level 4's 4 + 13 is 17 MP a cast, not 13.
// A heal that costs only its initial MP is still a heal, not a tank; one
// that costs nothing is no heal.
func TestSoloMPCost(t *testing.T) {
	tests := []struct {
		name             string
		role             Role
		skill            int32
		initial, consume int
		wantMP           float64
		wantTank         bool
	}{
		{"damage skill, initial and consume", RoleDamage, skillWindStrike, 4, 13, 17, false},
		{"damage skill, consume only", RoleDamage, skillWindStrike, 0, 13, 13, false},
		{"damage skill, initial only", RoleDamage, skillWindStrike, 6, 0, 6, false},
		{"heal, initial and consume", RoleSupport, skillHeal, 4, 13, 17, false},
		{"heal, initial only", RoleSupport, skillHeal, 5, 0, 5, false},
		{"heal costing no MP is no heal", RoleSupport, skillHeal, 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, classHumanMystic, 20, tt.role, []KnownSkill{{ID: tt.skill, Level: 1}}, nil)
			m := b.cfg.Travel
			m.skills = maps.Clone(m.skills)
			ref := datapack.SkillRef{ID: tt.skill, Level: 1}
			sk := m.skills[ref]
			sk.MPInitialConsume, sk.MPConsume = tt.initial, tt.consume
			m.skills[ref] = sk

			role, tank := b.gearRole()
			got := role.Attack.MPCost
			if tt.role == RoleSupport {
				got = role.HealMP
			}
			if got != tt.wantMP || tank != tt.wantTank {
				t.Errorf("gearRole MP = %v, tank %v; want %v, tank %v", got, tank, tt.wantMP, tt.wantTank)
			}
			if tt.role != RoleDamage {
				return
			}
			bd := b.build()
			a, ok, priced := b.spotAttack(&bd)
			if !ok || !priced || a.MPCost != tt.wantMP {
				t.Errorf("spotAttack = %+v, %v, %v; want MPCost %v", a, ok, priced, tt.wantMP)
			}
			if a.HitTime != 4*time.Second {
				t.Errorf("spotAttack HitTime = %v, want Wind Strike's 4s", a.HitTime)
			}
		})
	}
}
