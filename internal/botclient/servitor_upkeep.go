package botclient

import (
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The servitor upkeep (docs/servitors.md sections 3 and 4, #759): the
// summon, Servitor Heal, the servitor buffs, the lifetime's upkeep
// checkpoints and the spot a servitor dies at too often.
const (
	// expPenaltyMax is the XP penalty from which the bot never summons:
	// 1154 Summon Corrupted Man and the first-profession 1225, 1227 and
	// 1228 cost 0.9 of every kill's experience while out (section 3,
	// "Experience").
	expPenaltyMax = 0.5
	// summonRetry spaces the summon casts: one per summonRetry, whatever
	// came of the last (a refusal, SystemMessage 113, 580 or "cannot summon
	// in combat", starts no reuse, cast/controller.go CanCast; damage
	// breaks the 15 s cast). The summon's own reuse is 20 s.
	summonRetry = 30 * time.Second
	// upkeepHorizon is how close, in lifetime, the next upkeep checkpoint
	// is due: within one fight, at the lifetime a second of combat costs
	// (summonTimeLostActive 1000 ms). The bot starts no pull while a due
	// checkpoint is one it cannot pay (upkeepBlocks).
	upkeepHorizon = 30 * time.Second
	// servitorBuffTime is a servitor buff's time when its datapack entry
	// names none: 1140, 1139, 1141, 1144, 1145 and 1146 all last 1200 s
	// (skills/1100-1199.xml).
	servitorBuffTime = 1200 * time.Second
)

// servitorUpkeep is the record of the summons, the servitor buffs and the
// servitor deaths. The brain goroutine owns it.
type servitorUpkeep struct {
	// summonAt is when the last summon cast went out (summonRetry).
	summonAt time.Time
	// buffSent and buffed are when a servitor buff went out (buffRetry)
	// and when its cast started (its MagicSkillUse; the buff's time runs
	// from there), keyed by the servitor's object id and the skill: a new
	// servitor has none.
	buffSent, buffed map[buffKey]time.Time
	// casting is the servitor buff whose cast is in flight until castEnd
	// (its MagicSkillUse plus its hit time): the bot's own
	// MagicSkillCanceled before then drops its buffed record, so it is due
	// again after buffRetry.
	casting buffKey
	castEnd time.Time
	// died is the servitor whose death counted, deaths the spots and times
	// of the deaths within ServitorDeathWindow.
	died   int32
	deaths []servitorDeath
}

// servitorDeath is a servitor's death at a farming spot (its index in the
// spot registry).
type servitorDeath struct {
	spot int
	at   time.Time
}

// summonGate is the summon skill level the bot can cast now
// (Brain.summonReady): the first of the kit's Summons the bot knows, of a
// SUMMON skill of an XP penalty under expPenaltyMax, whose crystals it
// carries for the cast plus one upkeep step (summonAffords). A line lists
// its first-profession Crystal D summon after its own, so a bot holding
// only Crystal D falls back on it (owner answer 1, D51). The zero value for
// none, without a travel map, or for a kit without Summons (every row
// before #761: the rules that read it do nothing, D24).
func (b *Brain) summonGate() datapack.SkillRef {
	return b.summonFrom(b.build().Kit.Summons)
}

// upkeepCost is what one upkeep checkpoint of a servitor of summon skill
// level sk costs; the zero value for a summon that pays none: no item or
// no checkpoint (summon/lifetime.go InitialNextConsumeTime), or a count of
// 0 (live_lifecycle.go TickServitor pays only above 0).
func upkeepCost(sk datapack.Skill) reagent {
	if sk.ItemConsumeIDOT <= 0 || sk.ItemConsumeCountOT <= 0 || sk.ItemConsumeSteps <= 0 {
		return reagent{}
	}
	return reagent{item: sk.ItemConsumeIDOT, count: sk.ItemConsumeCountOT}
}

// summonAffords reports whether the bot carries the crystals of a cast of
// summon skill level sk plus one upkeep step (docs/servitors.md section 4,
// the D49 reagent gate): the cast's alone would summon a servitor the
// first checkpoint sends away.
func (b *Brain) summonAffords(sk datapack.Skill) bool {
	need := make(map[int32]int32, 2)
	if sk.ItemConsumeID > 0 && sk.ItemConsumeCount > 0 {
		need[sk.ItemConsumeID] += sk.ItemConsumeCount
	}
	if u := upkeepCost(sk); u.count > 0 {
		need[u.item] += u.count
	}
	for item, n := range need {
		if b.carried(item) < n {
			return false
		}
	}
	return true
}

// summonLife is what one full life of the kit's summon skill id costs at
// the level the bot knows it: the cast plus steps+1 checkpoints, the last
// at remaining 0 (upkeepNext). The restock (reagentNeeds) counts it as one
// cast (docs/servitors.md section 4).
func (b *Brain) summonLife(id int32) []reagent {
	k, ok := b.world.Skills[id]
	if !ok || b.cfg.Travel == nil {
		return nil
	}
	sk := b.cfg.Travel.skills[datapack.SkillRef{ID: id, Level: k.Level}]
	var out []reagent
	if sk.ItemConsumeID > 0 && sk.ItemConsumeCount > 0 {
		out = append(out, reagent{item: sk.ItemConsumeID, count: sk.ItemConsumeCount})
	}
	if u := upkeepCost(sk); u.count > 0 {
		out = append(out, reagent{item: u.item, count: u.count * (sk.ItemConsumeSteps + 1)})
	}
	return out
}

// upkeepNext is the remaining lifetime, in ms, at which the servitor next
// pays its upkeep, from its remaining and total lifetime and its summon's
// steps: the checkpoints are total - k*(total/(steps+1)) for k from 1 to
// steps+1, the last at 0 when steps+1 divides total, and one is paid on
// the tick that takes the remaining from above it to at or below it
// (summon/lifetime.go Tick), so at remaining exactly on one it is paid
// already. Not ok past the last, or for a summon without checkpoints.
func upkeepNext(remaining, total int64, steps int32) (int64, bool) {
	if steps <= 0 || total <= 0 || remaining < 0 {
		return 0, false
	}
	step := total / int64(steps+1)
	if step <= 0 {
		return 0, false
	}
	k := max((total-remaining)/step+1, 1)
	if k > int64(steps)+1 {
		return 0, false
	}
	return total - k*step, true
}

// servitorSummon is the summon skill level the bot's servitor s came from:
// the kit's Summons at the levels up to the one the bot knows whose npcId
// is s's template, ok false for none (a summon the kit does not list). Its
// corpse summons (CorpseKit.Summons) count too: a corpse servitor pays its
// upkeep as any.
func (b *Brain) servitorSummon(s *Servitor) (datapack.Skill, bool) {
	m := b.cfg.Travel
	if m == nil {
		return datapack.Skill{}, false
	}
	kit := b.build().Kit
	for _, id := range slices.Concat(kit.Summons, kit.Corpse.Summons) {
		k, ok := b.world.Skills[id]
		if !ok {
			continue
		}
		for l := min(k.Level, enchantLevels); l >= 1; l-- {
			if sk, ok := m.skills[datapack.SkillRef{ID: id, Level: l}]; ok && sk.NPCID == s.TemplateID {
				return sk, true
			}
		}
	}
	return datapack.Skill{}, false
}

// upkeepBlocks reports whether the bot starts no new pull at now: its
// live servitor's next upkeep checkpoint is due within upkeepHorizon of
// its last lifetime bar (SetSummonRemainTime) and the bot cannot pay it,
// so the servitor would leave mid-fight (docs/servitors.md section 3,
// "Lifetime and upkeep"). The bot lets it run out between fights and never
// unsummons it. False before the first lifetime bar and for a servitor that
// pays no upkeep.
func (b *Brain) upkeepBlocks() bool {
	s := b.liveServitor()
	if s == nil || s.Lifetime <= 0 {
		return false
	}
	sk, ok := b.servitorSummon(s)
	if !ok {
		return false
	}
	cost := upkeepCost(sk)
	if cost.count == 0 || b.carried(cost.item) >= cost.count {
		return false
	}
	next, ok := upkeepNext(s.Remaining.Milliseconds(), s.Lifetime.Milliseconds(), sk.ItemConsumeSteps)
	return ok && s.Remaining.Milliseconds()-next <= upkeepHorizon.Milliseconds()
}

// freshPull reports whether id is a target nobody on the bot's side has
// engaged at now: it attacks neither the bot nor its servitor, no attack of
// the bot's went out on it, and the servitor fights it not. The upkeep gate
// (upkeepBlocks) keeps the bot off such a target only, never off a fight
// it is in.
func (b *Brain) freshPull(id int32, now time.Time) bool {
	if id == b.engaged || id == b.opened || slices.Contains(b.attackers(now), id) {
		return false
	}
	s := b.liveServitor()
	return s == nil || (id != s.Target && id != s.Struck &&
		(id != s.AttackedBy || now.Sub(s.AttackedAt) > attackerWindow))
}

// ruleSummon summons the servitor out of combat (docs/servitors.md section
// 2, "Rule order"): the summon the gate opens (summonReady), once no
// servitor stands (a dead one stays until its PetDelete, and the server
// refuses a second with SystemMessage 580), with nothing attacking the bot,
// the MP of the cast and the skill ready, cast on itself (target SELF).
// Its bound: one cast per summonRetry, on top of the skill's reuse.
func (b *Brain) ruleSummon(now time.Time) (Action, bool) {
	ref, s := b.summonReady, &b.world.Self
	if ref == (datapack.SkillRef{}) || b.world.Servitor != nil || s.InCombat || len(b.attackers(now)) > 0 ||
		now.Sub(b.upkeep.summonAt) < summonRetry || !b.castReady(ref.ID, now) {
		return Action{}, false
	}
	if sk := b.cfg.Travel.skills[ref]; int(s.MP) < sk.MPInitialConsume+sk.MPConsume {
		return Action{}, false
	}
	return b.castOn(s.ObjectID, ref.ID, false, now), true
}

// servitorCast reports whether the bot may cast skill on its servitor s at
// now: it knows the skill, the skill is ready with its reagent, the bot has
// the MP of it and s stands within its cast range. The SUMMON target takes
// the caster's own servitor whatever is selected (handler/target
// target_handlers_summon.go), so the cast needs no selection.
func (b *Brain) servitorCast(s *Servitor, skill int32, now time.Time) bool {
	if _, known := b.world.Skills[skill]; !known || !b.castReady(skill, now) {
		return false
	}
	sk, ok := b.skillData(skill)
	if !ok {
		return false
	}
	self := &b.world.Self
	reach := int64(sk.CastRange)
	return int(self.MP) >= sk.MPInitialConsume+sk.MPConsume && dist2(self.Position(now), s.Pos) <= reach*reach
}

// servitorHeal is the Servitor Heal (ClassKit.ServitorHeal) of the servitor
// rule: on the live servitor under Persona.ServitorHealHP, in range; while
// the bot is in combat only under half of it, so between the two it keeps
// to its own attack steps and heals once the fight is over (docs/servitors.md
// section 3). The rule runs after the critical and party heal rules, so it
// never outranks them.
func (b *Brain) servitorHeal(s *Servitor, now time.Time) (Action, bool) {
	h, pct := b.build().Kit.ServitorHeal, b.persona.ServitorHealHP
	if h == 0 || !below(s.HP, s.MaxHP, pct) || b.world.Self.InCombat && !below(s.HP, s.MaxHP, pct/2) ||
		!b.servitorCast(s, h, now) {
		return Action{}, false
	}
	return b.castOn(s.ObjectID, h, false, now), true
}

// servitorBuff is the next servitor buff (ClassKit.ServitorBuffs) the
// rebuff rule puts on the live servitor: one it has not had from this bot,
// or whose time from its cast's start runs out within RebuffBefore. A
// servitor's effect list reaches its owner only in refreshes this bot does
// not read, so the buffs are timed by cast, per servitor (docs/servitors.md
// section 3). Its bound: one cast per buff and servitor per buffRetry, as
// the kit buffs.
func (b *Brain) servitorBuff(now time.Time) (Action, bool) {
	s := b.liveServitor()
	if s == nil {
		return Action{}, false
	}
	u := &b.upkeep
	for _, id := range b.build().Kit.ServitorBuffs {
		k := buffKey{s.ObjectID, id}
		if at, ok := u.buffed[k]; ok && now.Before(at.Add(b.servitorBuffTime(id)-b.persona.RebuffBefore)) ||
			now.Sub(u.buffSent[k]) < buffRetry || !b.servitorCast(s, id, now) {
			continue
		}
		return b.castOn(s.ObjectID, id, false, now), true
	}
	return Action{}, false
}

// servitorBuffTime is how long buff skill lasts: its longest effect's time,
// servitorBuffTime without a datapack entry.
func (b *Brain) servitorBuffTime(skill int32) time.Duration {
	sk, ok := b.skillData(skill)
	if !ok {
		return servitorBuffTime
	}
	d := time.Duration(0)
	for _, e := range sk.Effects {
		d = max(d, time.Duration(e.Time)*time.Second)
	}
	if d <= time.Second {
		return servitorBuffTime
	}
	return d
}

// upkeepSent records a summon or servitor buff cast a going out at now.
func (b *Brain) upkeepSent(a Action, now time.Time) {
	kit := b.build().Kit
	u := &b.upkeep
	switch {
	case slices.Contains(kit.Summons, a.skill):
		u.summonAt = now
	case slices.Contains(kit.ServitorBuffs, a.skill):
		if s := b.world.Servitor; s != nil {
			u.buffSent = keepServitor(u.buffSent, s.ObjectID)
			u.buffSent[buffKey{s.ObjectID, a.skill}] = now
		}
	}
}

// servitorBuffStarted records the start of the bot's cast of a servitor
// buff on its servitor (MagicSkillUse): the buff's time runs from there.
// Any other cast of the bot's ends the one in flight.
func (b *Brain) servitorBuffStarted(e MagicSkillUse, now time.Time) {
	if e.CasterID != b.world.Self.ObjectID {
		return
	}
	u := &b.upkeep
	u.casting, u.castEnd = buffKey{}, time.Time{}
	s := b.world.Servitor
	if s == nil || !slices.Contains(b.build().Kit.ServitorBuffs, e.SkillID) {
		return
	}
	k := buffKey{s.ObjectID, e.SkillID}
	u.buffed = keepServitor(u.buffed, s.ObjectID)
	u.buffed[k] = now
	u.casting, u.castEnd = k, now.Add(e.HitTime)
}

// servitorBuffCanceled drops the record of the servitor buff in flight
// when the bot's own cast stops at now before it hits (MagicSkillCanceled):
// the buff never landed.
func (b *Brain) servitorBuffCanceled(e MagicSkillCanceled, now time.Time) {
	u := &b.upkeep
	if e.CasterID != b.world.Self.ObjectID || u.casting == (buffKey{}) {
		return
	}
	if !now.After(u.castEnd) {
		delete(u.buffed, u.casting)
	}
	u.casting, u.castEnd = buffKey{}, time.Time{}
}

// keepServitor drops from m the records of servitors other than id,
// making it when nil.
func keepServitor(m map[buffKey]time.Time, id int32) map[buffKey]time.Time {
	if m == nil {
		return make(map[buffKey]time.Time)
	}
	maps.DeleteFunc(m, func(k buffKey, _ time.Time) bool { return k.target != id })
	return m
}

// servitorDied counts the death of the bot's servitor id at now
// (docs/servitors.md section 3, "Death and re-summon"): the second within
// Persona.ServitorDeathWindow at the spot the bot farms marks that spot too
// hard, as the bot's own death does (diedAtSpot). One count per servitor.
func (b *Brain) servitorDied(id int32, now time.Time) {
	s, u := b.world.Servitor, &b.upkeep
	if s == nil || s.ObjectID != id || u.died == id {
		return
	}
	u.died = id
	w := b.persona.ServitorDeathWindow
	if w <= 0 || b.cfg.Spots == nil || !b.spot.farming() {
		return
	}
	idx := b.spot.idx
	u.deaths = slices.DeleteFunc(u.deaths, func(d servitorDeath) bool { return now.Sub(d.at) > w || d.spot != idx })
	u.deaths = append(u.deaths, servitorDeath{spot: idx, at: now})
	if len(u.deaths) < 2 || b.persona.Spots.DeathAvoid <= 0 {
		return
	}
	u.deaths = nil
	b.log.Info("servitor died twice at the spot: too hard for it", "spot", idx)
	b.diedAtSpot(now)
}
