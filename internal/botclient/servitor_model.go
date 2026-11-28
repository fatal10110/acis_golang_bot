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

// servitorModel is the servitor the model counts as the bot's second
// attacker (docs/servitors.md section 3, "TimeToKill"): one the bot has out
// or can summon. While one is out and alive, its template's stats with the
// live P.Atk, P.Def, attack speed and max HP of its PetInfo; else, while the
// bot can summon one (summonReady), the template its summon skill level
// names (npcId). A dead or expired servitor counts through summonReady: the
// bot re-summons it out of combat after its PetDelete, a wait of seconds
// that does not matter for spot choice, so the spot it holds does not flip
// on each death (section 3: only two deaths within ServitorDeathWindow make
// a spot too hard). Not ok with neither, without a travel map or when the
// template is not a servitor's: the bot then counts itself alone, as
// before servitors.
func (b *Brain) servitorModel() (charmodel.Profile, bool) {
	m := b.cfg.Travel
	if m == nil {
		return charmodel.Profile{}, false
	}
	if s := b.world.Servitor; s != nil && !s.Dead {
		if n, ok := m.servitors[s.TemplateID]; ok {
			return charmodel.Servitor(n, charmodel.ServitorLive{
				MaxHP: float64(s.MaxHP), PAtk: float64(s.PAtk), PDef: float64(s.PDef), PAtkSpd: float64(s.PAtkSpd),
			}), true
		}
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
// (packFits): its ServitorSpotMargin while it has a servitor out or can
// summon one (servitorModel, the same condition as the kill credit), else
// its SpotMargin. Only the summoner rows set a ServitorSpotMargin
// (docs/servitors.md section 5), so every other row's margin is as before.
func (b *Brain) spotMargin(bd *Build) float64 {
	if m := bd.Kit.ServitorSpotMargin; m > 0 {
		if _, ok := b.servitorModel(); ok {
			return m
		}
	}
	return bd.Kit.SpotMargin
}
