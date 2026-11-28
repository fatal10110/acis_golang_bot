package charmodel

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// ServitorLive are the numbers PetInfo reports of a servitor that is out
// (docs/servitors.md section 1): its stats after its passives and buffs. A
// zero field is one not reported yet.
type ServitorLive struct {
	MaxHP, PAtk, PDef, PAtkSpd float64
}

// Servitor is a servitor's profile (docs/servitors.md section 3): its NPC
// template's (npcs/14000-14999.xml) final stats, which the server derives
// through the same finalize step as a monster's (summon/formula.go
// defaultBuiltin: max HP by CON, P.Atk by STR and level, P.Def by level,
// attack speed and critical rate by DEX), with each number live reports
// in place of the template's.
func Servitor(template datapack.NPC, live ServitorLive) Profile {
	p := Monster(template)
	for _, f := range []struct {
		dst *float64
		v   float64
	}{{&p.MaxHP, live.MaxHP}, {&p.PAtk, live.PAtk}, {&p.PDef, live.PDef}, {&p.PAtkSpd, live.PAtkSpd}} {
		if f.v > 0 {
			*f.dst = f.v
		}
	}
	return p
}

// TimeWithServitor is how long a bot, killing target alone in t, and its
// servitor s with its melee take to kill it together: 1/(1/t + 1/T_s),
// T_s the servitor's TimeToKill (TimeTogether). A servitor that deals no
// damage leaves t; a bot that never kills it (t never) leaves the
// servitor's own time.
func TimeWithServitor(t time.Duration, s, target Profile) time.Duration {
	return TimeTogether(t, TimeToKill(s, Attack{}, target))
}
