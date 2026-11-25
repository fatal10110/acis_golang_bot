package botclient

import "cmp"

// The two server penalties that slow a character (#436, docs/items.md
// section 1):
//
//   - The weight penalty: a band of the carried weight over the weight
//     limit (model/actor/player/character_weight_penalty.go). Bands 2 and 3
//     (from 66.6 %) halve the move speed, band 4 (from 100 %) stops it, and
//     bands 1-3 halve the regeneration.
//   - The grade (expertise) penalty: every worn piece of a crystal grade
//     above the Expertise level (character_grade_penalty.go). Each armor
//     piece, a full armor twice, up to 4, slows the move and cast speed by
//     16 % and costs evasion; an over-grade weapon costs accuracy.
//
// The server reports both in EtcStatusUpdate, but it broadcasts another
// player's EtcStatusUpdate too, with nothing telling the two apart
// (World.applyDeathPenalty). So the bot reads them from self-only frames:
// the weight band from its load and weight limit (UserInfo, StatusUpdate),
// which the server sends with every band change (sendLiveWeightPenalty,
// network/live_player_events.go), and the grade penalty from skill 4267,
// which the server sets to level 1 while it holds and sends in the
// SkillList of every change (event.GradePenaltyChanged).

// gradePenaltySkill is the grade penalty's passive skill: level 1 while a
// worn piece is above the Expertise grade, absent otherwise
// (character_grade_penalty.go gradePenaltySkillID).
const gradePenaltySkill = 4267

// slowBand is the first weight band that halves the move speed.
const slowBand = 2

// weightBand is the server's weight band for load against the weight limit
// maxLoad (refreshWeightPenalty): 0 under 50 %, 1 under 66.6 %, 2 under
// 80 %, 3 under 100 %, 4 from there; 0 while the limit is unknown. The
// server subtracts a weight-reduction stat the bot does not know, so it
// is the band of the bot's whole load, never a lower one than the server's.
func weightBand(load, maxLoad int32) int32 {
	if maxLoad <= 0 {
		return 0
	}
	switch r := float64(load) / float64(maxLoad); {
	case r < .5:
		return 0
	case r < .666:
		return 1
	case r < .8:
		return 2
	case r < 1:
		return 3
	}
	return 4
}

// fitsUnderSlow reports whether a load stays under the half-speed band.
func fitsUnderSlow(load int64, maxLoad int32) bool {
	return maxLoad <= 0 || float64(load)/float64(maxLoad) < .666
}

// slowed reports whether the bot's load is in a band that halves its speed.
func (b *Brain) slowed() bool {
	s := &b.world.Self
	return weightBand(s.Load, s.MaxLoad) >= slowBand
}

// gradePenalty reports whether the server holds the grade penalty on the
// bot (skill 4267 in its SkillList).
func (w *World) gradePenalty() bool { return w.Skills[gradePenaltySkill].Level > 0 }

// overGrade is the worn piece ruleGear takes off for the grade penalty,
// the zero Item for none: while the server holds the penalty, the worn
// armor or jewelry of a grade above the Expertise level with the lowest
// object id, else such a weapon once a carried one of the bot's grade
// replaces it (an over-grade weapon only costs accuracy, so the bot never
// fights barehanded for it). A piece the server would not take off within
// gearTimeout is left alone.
func (b *Brain) overGrade(w *worn) Item {
	if !b.world.gradePenalty() {
		return Item{}
	}
	exp := b.world.Skills[expertiseSkill].Level
	var off Item
	for _, it := range b.world.Inventory.Items {
		g, ok := b.gearItem(it.TemplateID)
		if !it.Equipped || !ok || g.grade <= exp || b.gear.refused[it.TemplateID] {
			continue
		}
		if slotOf(g.part) == slotWeapon && !b.weaponInGrade(w) {
			continue
		}
		if off.ObjectID == 0 || it.ObjectID < off.ObjectID {
			off = it
		}
	}
	return off
}

// weaponInGrade reports whether the bot carries a weapon it would wear in
// an empty weapon slot (upgrade), which beatsPiece limits to its grade.
func (b *Brain) weaponInGrade(w *worn) bool {
	bare := *w
	bare[slotWeapon] = Item{}
	for _, it := range b.world.Inventory.Items {
		if g, ok := b.gearItem(it.TemplateID); ok && slotOf(g.part) == slotWeapon && b.upgrade(it, &bare) {
			return true
		}
	}
	return false
}

// gearFits reports whether buying rows on top of the load keeps it under
// the half-speed band; with the weight limit or the item weights unknown
// it does.
func (b *Brain) gearFits(rows []buyRow, add buyRow) bool {
	s := &b.world.Self
	t := b.cfg.Travel
	if s.MaxLoad <= 0 || t == nil || len(t.weights) == 0 {
		return true
	}
	weight := func(r buyRow) int64 { return int64(t.weights[r.item]) * int64(cmp.Or(r.count, 1)) }
	load := int64(s.Load) + weight(add)
	for _, r := range rows {
		load += weight(r)
	}
	return fitsUnderSlow(load, s.MaxLoad)
}
