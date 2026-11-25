package botclient

import (
	"maps"
	"slices"
	"time"
)

// ruleRaise is the raise of a bot party (docs/parties/members.md,
// "Events"): a bot that knows Resurrection (1016: CORPSE_PLAYER, castRange
// 400, hitTime 6 s, reuse 120 s; data/xml/skills/1000-1099.xml), out of
// combat in its own bot party, selects a dead member in sight, the leader
// first, and casts it. The server sends the dead player its offer
// (ConfirmDlg), which a dead bot accepts (ruleDead). With MP under SitMP it
// leaves the corpse to rule 6's rest, as ruleHeal does. A cast in flight is
// waited out while a member lies dead, so nothing moves the bot out of the
// cast. It is paced as every cast: one at a time, until its MagicSkillUse or
// castTimeout, then the skill's reuse. Outside a bot party it never matches.
func (b *Brain) ruleRaise(now time.Time) (Action, bool) {
	s := &b.world.Self
	if _, known := b.world.Skills[skillResurrection]; !known || s.InCombat || !b.inBotParty() {
		return Action{}, false
	}
	dead := b.corpses()
	switch {
	case len(dead) == 0:
		return Action{}, false
	case s.Casting(now) || b.castPending(now):
		return Action{}, true
	case !b.world.SkillReady(skillResurrection, now) || below(s.MP, s.MaxMP, b.build().Thresholds.SitMP):
		return Action{}, false
	}
	return b.castOn(dead[0], skillResurrection, true, now), true
}

// corpses are the dead players of the bot's party in sight that it may
// raise, in order: the leader, then the others by object id. The bot itself
// and a member the PvP policy does not let it support are left out.
func (b *Brain) corpses() []int32 {
	p := &b.world.Party
	self := b.world.Self.ObjectID
	ids := slices.Sorted(maps.Keys(p.Members))
	if i := slices.Index(ids, p.LeaderID); i > 0 {
		ids = slices.Insert(slices.Delete(ids, i, i+1), 0, p.LeaderID)
	}
	return slices.DeleteFunc(ids, func(id int32) bool {
		o := b.world.object(id)
		return id == self || o == nil || o.Kind != KindPlayer || !o.Dead || !b.persona.PvP.MaySupport(o)
	})
}
