package botclient

import (
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestGearRoleSkipsSustainSteps checks that a Damage build's model attack
// passes over a rotation step gated on the bot's own HP (#676): the
// sorcerer's Vampiric Touch under 60 % HP and the spellhowler's Vampiric
// Claw under 70 % are drains it casts when hurt, not what it kills with,
// so the model times Prominence and Hurricane. A build that knows only its
// sustain step is valued by its melee hits, as the bot melees until hurt.
func TestGearRoleSkipsSustainSteps(t *testing.T) {
	drain := func(power float64, reuse time.Duration) datapack.Skill {
		return datapack.Skill{Type: "DRAIN", OperateType: "ACTIVE", Power: power, HitTime: 4 * time.Second, ReuseDelay: reuse, MPConsume: 30}
	}
	nuke := datapack.Skill{Type: "MDAM", OperateType: "ACTIVE", Power: 52, HitTime: 4 * time.Second, ReuseDelay: 6 * time.Second, MPInitialConsume: 7, MPConsume: 28}
	skills := map[int32]datapack.Skill{
		skillVampiricTouch: drain(40, 12*time.Second),
		skillVampiricClaw:  drain(45, 10*time.Second),
		skillProminence:    nuke,
		skillHurricane:     nuke,
	}
	nukeAttack := charmodel.Attack{Magic: true, Power: 52, HitTime: 4 * time.Second, Reuse: 6 * time.Second, MPCost: 35}
	tests := []struct {
		name  string
		class int32
		known []int32
		want  charmodel.Attack
	}{
		{"SN: Prominence, not Vampiric Touch", classSorcerer, []int32{skillVampiricTouch, skillProminence}, nukeAttack},
		{"SH: Hurricane, not Vampiric Claw or Touch", classSpellhowler, []int32{skillVampiricClaw, skillVampiricTouch, skillHurricane}, nukeAttack},
		{"SN knowing only Vampiric Touch: melee", classSorcerer, []int32{skillVampiricTouch}, charmodel.Attack{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var known []KnownSkill
			for _, id := range tt.known {
				known = append(known, KnownSkill{ID: id, Level: 1})
			}
			b := newModelBrain(t, tt.class, 40, RoleDamage, known, nil)
			m := b.cfg.Travel
			m.skills = maps.Clone(m.skills)
			for id, sk := range skills {
				m.skills[datapack.SkillRef{ID: id, Level: 1}] = sk
			}
			role, tank := b.gearRole()
			if tank || role.Kind != charmodel.Damage || role.Attack != tt.want {
				t.Errorf("gearRole = %+v, tank %v; want the attack %+v", role, tank, tt.want)
			}
		})
	}
}
