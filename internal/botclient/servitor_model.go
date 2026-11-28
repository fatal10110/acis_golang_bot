package botclient

import (
	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// indexServitors are the NPC templates of type Servitor
// (npcs/14000-14999.xml): what a summon skill's npcId and a PetInfo's
// template id name.
func indexServitors(npcs map[int32]datapack.NPC) map[int32]datapack.NPC {
	out := make(map[int32]datapack.NPC)
	for id, n := range npcs {
		if n.Type == "Servitor" {
			out[id] = n
		}
	}
	return out
}

// servitorOut reports whether the bot's servitor is out and alive: a
// dead one fights no more, and none can be summoned before its PetDelete
// (docs/servitors.md section 3).
func (b *Brain) servitorOut() bool {
	s := b.world.Servitor
	return s != nil && !s.Dead
}

// servitorModel is the servitor the model counts as the bot's second
// attacker (docs/servitors.md section 3, "TimeToKill"): while one is out,
// its template's stats with the live P.Atk, P.Def, attack speed and max HP
// of its PetInfo; else, while the bot can summon one (summonReady), the
// template its summon skill level names (npcId). Not ok with neither, with
// a dead servitor, without a travel map or when the template is not a
// servitor's: the bot then counts itself alone, as before servitors.
func (b *Brain) servitorModel() (charmodel.Profile, bool) {
	m := b.cfg.Travel
	if m == nil {
		return charmodel.Profile{}, false
	}
	if s := b.world.Servitor; s != nil {
		n, ok := m.servitors[s.TemplateID]
		if s.Dead || !ok {
			return charmodel.Profile{}, false
		}
		return charmodel.Servitor(n, charmodel.ServitorLive{
			MaxHP: float64(s.MaxHP), PAtk: float64(s.PAtk), PDef: float64(s.PDef), PAtkSpd: float64(s.PAtkSpd),
		}), true
	}
	if b.summonReady == (datapack.SkillRef{}) {
		return charmodel.Profile{}, false
	}
	n, ok := m.servitors[m.skills[b.summonReady].NPCID]
	if !ok {
		return charmodel.Profile{}, false
	}
	return charmodel.Servitor(n, charmodel.ServitorLive{}), true
}

// spotMargin is the spot margin the bot's row fits spots by now
// (packFits): its ServitorSpotMargin while its servitor is out, else its
// SpotMargin. Only the summoner rows set a ServitorSpotMargin
// (docs/servitors.md section 5), so every other row's margin is as before.
func (b *Brain) spotMargin(bd *Build) float64 {
	if m := bd.Kit.ServitorSpotMargin; m > 0 && b.servitorOut() {
		return m
	}
	return bd.Kit.SpotMargin
}
