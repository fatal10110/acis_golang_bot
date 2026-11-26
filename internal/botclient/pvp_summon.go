package botclient

import "time"

// standIn is whom attacker id is fought as at now (docs/pvp/targets.md
// "Summons and pets"): an owner's summon worth killing first stands in for
// its owner (firstSummon), a summon's owner in sight that the bot may fight
// stands in for the summon otherwise, and anyone else is itself.
func (b *Brain) standIn(id int32, now time.Time) int32 {
	if s := b.firstSummon(id, now); s != 0 {
		return s
	}
	if owner := b.summonOwner(id, now); owner != 0 && !b.mayFight(id, now) {
		return owner
	}
	return id
}

// firstSummon is the summon of player owner (object id) the bot fights
// before it at now, 0 for none: the owner is in sight and fightable, and
// the summon pays off (summonPays) and passes MayFight's summon rules on its
// own (maySummon: the same flag, karma, arena and friend guards as a summon
// whose owner is out of sight).
func (b *Brain) firstSummon(owner int32, now time.Time) int32 {
	o := b.world.object(owner)
	if o == nil || o.Kind != KindPlayer || o.Name == "" {
		return 0
	}
	for id, name := range b.pvp.owners {
		if name == o.Name && b.summonOwner(id, now) == owner && b.mayFight(id, now) {
			return id
		}
	}
	return 0
}

// summonPays reports whether killing summon id before its owner (object id)
// pays off at now (docs/pvp/targets.md "Summons and pets"): it healed its
// owner within enemyHealerFor (a pet's heals undo the bot's damage on the
// owner), or both are hitting the bot and the summon's streak dealt more
// than switchFactor times the owner's (a summoner's damage is its
// servitor's), more than the owner's alone once the summon is the bot's
// target, so a close race does not flip the target.
func (b *Brain) summonPays(id, owner int32, now time.Time) bool {
	if at, ok := b.pvp.petHeals[id]; ok && now.Sub(at) <= enemyHealerFor {
		return true
	}
	if !b.world.attacking(id, now) || !b.world.attacking(owner, now) {
		return false
	}
	s, o := b.world.Attackers[id].Damage, b.world.Attackers[owner].Damage
	if b.world.Target == id {
		return s > o
	}
	return s > switchFactor*o
}

// petHealSeen marks summon e.CasterID as healing its owner at now when e is
// a heal (enemyHeals) the summon casts on its owner in sight: a pet's Heal
// Trick, for one (target OWNER_PET).
func (b *Brain) petHealSeen(e MagicSkillUse, now time.Time) {
	owner := b.playerNamed(b.pvp.owners[e.CasterID])
	if owner == 0 || e.TargetID != owner {
		return
	}
	if s, ok := b.skillAt(e.SkillID, e.Level); !ok || !enemyHeals[s.Type] {
		return
	}
	if b.pvp.petHeals == nil {
		b.pvp.petHeals = make(map[int32]time.Time)
	}
	b.pvp.petHeals[e.CasterID] = now
}
