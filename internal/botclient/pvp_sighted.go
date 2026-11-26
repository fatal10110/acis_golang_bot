package botclient

import (
	"maps"
	"slices"
	"time"
)

// sightedReach is how near a player of a Sighted step's Vs comes before the
// bot puts the step's buff up out of combat: twice a bow's reach (bowRange),
// so an archer closing in is still out of its own reach when the cast (112
// Deflect Arrow: hitTime 1500, skills/0100-0199.xml) lands, and one passing
// farther off in the bot's known list does not trigger it.
const sightedReach = 2 * bowRange

// sightedBuff is the out-of-combat cast of the first Sighted self step of
// steps (the knights' 112 Deflect Arrow vs A, #557) on the bot at now: due
// while a player of one of the step's archetypes the bot may fight
// (sightedFoe) is within sightedReach and the step's gates pass against it,
// under the stack rule (buffDue), with the skill ready and no cast of it
// within buffRetry (buffSent). One cast per buff: the effect lasts 20
// minutes and the stack rule holds the next until it runs low.
func (b *Brain) sightedBuff(steps []Step, now time.Time) (Action, bool) {
	self := b.world.Self.ObjectID
	for _, step := range steps {
		if !step.Sighted || step.Target != TargetSelf || len(step.Vs) == 0 || !b.world.SkillReady(step.Skill, now) ||
			!b.buffDue(self, step.Skill, now) || now.Sub(b.buffedAt[buffKey{self, step.Skill}]) < buffRetry {
			continue
		}
		for _, id := range slices.Sorted(maps.Keys(b.pvp.opponents)) {
			r := b.pvp.opponents[id]
			if !slices.Contains(step.Vs, r.Archetype) || !b.inReach(id, sightedReach, now) || !b.sightedFoe(id, now) ||
				!step.passes(b.stepState(id, steps, now)) {
				continue
			}
			sk, _ := b.skillData(step.Skill)
			return b.castOn(self, step.Skill, sk.Target != "SELF", now), true
		}
	}
	return Action{}, false
}

// sightedFoe reports whether the bot may fight player id at now by
// MayFight's rules (mayFight), without recording the rule on its opponent
// record: seeing it is not fighting it.
func (b *Brain) sightedFoe(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && o.Kind == KindPlayer && !o.Dead && b.fightRule(id, o, now) != fightNone && !now.Before(b.blacklist[id])
}
