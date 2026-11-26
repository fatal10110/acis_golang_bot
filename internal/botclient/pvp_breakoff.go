package botclient

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// CP potions, cures, the break-off and Ultimate Defense
// (docs/pvp/layer.md "CP, cures, pool, break off", "Ultimate Defense";
// X6b). Against a player the PvP order starts here: a cure for the bot's
// own poison or bleed, a CP potion outside a duel, then, in the open world,
// the break-off (its ready Break step under the break-cast gate, then the
// flee). The only new sender is UseItem, bounded by the items' own 500 ms
// reuse (pvpItemReuse), one cure per effect seen, and the frame bucket.

// Sustain and break-off figures (docs/pvp/layer.md).
const (
	// CPPotion is the CP percent under which the bot drinks a CP potion
	// in a PvP fight other than a duel.
	CPPotion = 50
	// pvpItemReuse is the reuse of the CP potions and the cure items
	// (items/5500-5599.xml 5591, 5592 and items/1800-1899.xml 1831-1834,
	// reuse_delay 500): one use of each per pvpItemReuse at most.
	pvpItemReuse = 500 * time.Millisecond
	// breakHit is the scaled hit time under which a Break step always
	// goes; breakOdds the cast-break chance, in percent, under which a
	// longer one still does (the break-cast gate).
	breakHit  = 4 * time.Second
	breakOdds = 50
	// plainCure and greaterCure are the highest abnormal levels the plain
	// and greater cures remove (skills 2042-2045, negateLvl 3 and 7).
	plainCure   = 3
	greaterCure = 7
	// cureSame is how far apart two ends of one effect's skill may lie in
	// the bot's effect list and still be the effect a cure already went
	// for: AbnormalStatusUpdate carries whole seconds left.
	cureSame = 2 * time.Second
)

// The CP potions and cure items (docs/pvp/server-items.md).
const (
	itemCPPotion          int32 = 5591
	itemGreaterCPPotion   int32 = 5592
	itemAntidote          int32 = 1831
	itemGreaterAntidote   int32 = 1832
	itemBandage           int32 = 1833
	itemEmergencyDressing int32 = 1834
)

// cpPotions are the CP potions, greater first.
var cpPotions = []int32{itemGreaterCPPotion, itemCPPotion}

// cure is a cure item and the highest abnormal level it removes.
type cure struct {
	item  int32
	level int
}

// cures are the cure items of each DOT kind (the negateStats they remove),
// plain first.
var cures = map[string][]cure{
	"POISON": {{itemAntidote, plainCure}, {itemGreaterAntidote, greaterCure}},
	"BLEED":  {{itemBandage, plainCure}, {itemEmergencyDressing, greaterCure}},
}

// pvpItem is the item the bot uses fighting player id at now: a cure for its
// own poison or bleed (duels too), else a CP potion. ok is false for none.
func (b *Brain) pvpItem(id int32, now time.Time) (Action, bool) {
	if !b.fights(id, now) {
		return Action{}, false
	}
	if a, ok := b.cureDue(now); ok {
		return a, true
	}
	return b.cpPotion(now)
}

// cpPotion is a CP potion, greater first, with the bot's CP under CPPotion
// outside a duel (an uncancelled duel end restores CP), once pvpItemReuse
// has run out since the last one.
func (b *Brain) cpPotion(now time.Time) (Action, bool) {
	s := &b.world.Self
	if b.inDuel(now) || !below(s.CP, s.MaxCP, CPPotion) {
		return Action{}, false
	}
	for _, id := range cpPotions {
		if at, used := b.usedAt[id]; used && now.Sub(at) < pvpItemReuse {
			return Action{}, false
		}
	}
	for _, id := range cpPotions {
		if it, ok := b.world.Inventory.ByTemplate(id); ok && it.Count > 0 {
			return useItemAction(it), true
		}
	}
	return Action{}, false
}

// cureDue is the cure of the first of the bot's own effects (by skill id)
// one is due for (cureFor) and none went for yet, its item out of its
// reuse.
func (b *Brain) cureDue(now time.Time) (Action, bool) {
	maps.DeleteFunc(b.pvp.cured, func(skill int32, end time.Time) bool {
		cur, ok := b.world.Buffs[skill]
		return !ok || absDur(cur.Sub(end)) > cureSame
	})
	for _, skill := range slices.Sorted(maps.Keys(b.world.Buffs)) {
		if _, done := b.pvp.cured[skill]; done {
			continue
		}
		it, ok := b.cureFor(skill, now)
		if !ok {
			continue
		}
		if at, used := b.usedAt[it.TemplateID]; used && now.Sub(at) < pvpItemReuse {
			continue
		}
		return useItemAction(it), true
	}
	return Action{}, false
}

// cureFor is the cure item carried for the bot's own effect of skill at now:
// a poison or bleed (dotKind) with half its ticks or more left, of an
// abnormal level a carried cure removes (cureLevel), the plain cure first.
// ok is false for none.
func (b *Brain) cureFor(skill int32, now time.Time) (Item, bool) {
	end := b.world.Buffs[skill]
	s, ok := b.skillAt(skill, b.world.BuffLevels[skill])
	if !ok || end.IsZero() || !now.Before(end) {
		return Item{}, false
	}
	kind := dotKind(s)
	level, ok := cureLevel(s)
	if kind == "" || !ok {
		return Item{}, false
	}
	if lasts, _, _, _ := spanOf(s); 2*end.Sub(now) < lasts {
		return Item{}, false
	}
	for _, c := range cures[kind] {
		if level > c.level {
			continue
		}
		if it, ok := b.world.Inventory.ByTemplate(c.item); ok && it.Count > 0 {
			return it, true
		}
	}
	return Item{}, false
}

// cureSent records a UseItem of item going out at now: every effect it is
// the cure of counts as cured, one cure per effect seen.
func (b *Brain) cureSent(item int32, now time.Time) {
	for skill, end := range b.world.Buffs {
		if it, ok := b.cureFor(skill, now); ok && it.TemplateID == item {
			if b.pvp.cured == nil {
				b.pvp.cured = make(map[int32]time.Time)
			}
			b.pvp.cured[skill] = end
		}
	}
}

// dotKind is the negate kind of s, POISON or BLEED, or "" for neither:
// what a negate matches (skill/effect/hooks_buff.go negateTypeMatches), its
// skill type or its landing type. A DOT matched by neither, like the NPC
// Bleed 4615 (MDAM, stack type bleeding), no cure removes.
func dotKind(s datapack.Skill) string {
	lt := landingType(s)
	for _, k := range []string{"POISON", "BLEED"} {
		if strings.EqualFold(s.Type, k) || strings.EqualFold(lt, k) {
			return k
		}
	}
	return ""
}

// landingType is the server's resolved effect type of s (model/skill/
// landing.go LandingEffectType): its first effect's effect type, else its
// own, else STUN for PDAM and PARALYZE for MDAM, else its skill type.
func landingType(s datapack.Skill) string {
	for _, e := range s.Effects {
		if e.EffectType != "" {
			return e.EffectType
		}
	}
	if s.EffectType != "" {
		return s.EffectType
	}
	switch strings.ToUpper(strings.TrimSpace(s.Type)) {
	case "PDAM":
		return "STUN"
	case "MDAM":
		return "PARALYZE"
	}
	return s.Type
}

// cureLevel is the abnormal level a cure must reach to remove s
// (hooks_buff.go negateLevelAllows): the lower of its set levels,
// effectAbnormalLvl counting only with a landing type (the server's
// candidate carries LandingEffectType, skill/effect/core.go). ok is false
// with neither set: no level-limited cure removes it.
func cureLevel(s datapack.Skill) (level int, ok bool) {
	if landingType(s) != "" && s.EffectAbnormalLevel >= 0 {
		level, ok = s.EffectAbnormalLevel, true
	}
	if s.AbnormalLevel >= 0 && (!ok || s.AbnormalLevel < level) {
		level, ok = s.AbnormalLevel, true
	}
	return level, ok
}

// absDur is d's magnitude.
func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// pvpBreakHP is the HP percent under which the bot breaks off
// (docs/pvp/rows.md "Breaks off at"): tank 20, normal 30, glass cannon 40,
// by the class's spot margin (ClassKit.SpotMargin), which names the same
// three.
func pvpBreakHP(margin float64) int {
	switch {
	case margin >= marginGlassCannon:
		return 40
	case margin >= marginNormal:
		return 30
	}
	return 20
}

// breaksOff reports whether the bot breaks off its fight with player id at
// now (docs/pvp/layer.md "Break off"): in the open world only, its CP gone,
// its HP under pvpBreakHP, no heal or potion ready (sustainReady), its pool
// fraction under id's (losing), and able to get away (outruns); else it
// fights on, Ultimate Defense included.
func (b *Brain) breaksOff(id int32, rot []Step, st stepState, now time.Time) bool {
	s := &b.world.Self
	if b.inDuel(now) || b.inArena(now) || s.CP > 0 ||
		!below(s.HP, s.MaxHP, pvpBreakHP(b.build().Kit.SpotMargin)) {
		return false
	}
	return b.losing(id) && !b.sustainReady(rot, st, now) && b.outruns(id, now)
}

// breakAway is the break-off from player id at now: the first Break step
// of rot that is due (breakDue) and goes, else M10's flee away from id.
func (b *Brain) breakAway(id int32, rot []Step, st stepState, now time.Time) Action {
	st.breaking = true
	bot, opp, fighters := b.pvpFighters(id)
	for _, step := range rot {
		if !step.Break || step.Target == TargetEnemy && !b.breakDue(id, step.Skill, bot, opp, fighters, now) {
			continue
		}
		if a, ok := b.tryStep(id, step, st, false, now); ok {
			return a
		}
	}
	return b.flee(id, now)
}

// breakDue reports whether the bot's Break step of skill goes on player id
// at now: none while its last landing still holds id, a slow none while
// any slow of the bot still holds id (docs/pvp/rows.md, "Ice Bolt unless
// the opponent is still slowed"), a control step only when due
// (controlDue: a sleep under the sleep rule, at MinLand or more), and only
// under the break-cast gate (breakCastOK).
func (b *Brain) breakDue(id, skill int32, bot, opp charmodel.Fighter, fighters bool, now time.Time) bool {
	s, ok := b.skillData(skill)
	if !ok {
		return false
	}
	if r := b.pvp.opponents[id]; r != nil {
		if l, ok := r.fight.landed[skill]; ok && l.holds(r.Abnormal, now) {
			return false
		}
		if _, bits, _, kites := spanOf(s); kites && bits&rootAbnormal == 0 {
			for _, l := range r.fight.landed {
				if l.kites && l.bits&rootAbnormal == 0 && l.holds(r.Abnormal, now) {
					return false
				}
			}
		}
	}
	if isControl(s) && !b.controlDue(id, skill, bot, opp, fighters, now) {
		return false
	}
	return b.breakCastOK(id, s, skill, bot, opp, fighters, now)
}

// breakCastOK is the break-cast gate on the bot's Break step of skill (its
// datapack entry s) at player id at now (docs/pvp/layer.md): it goes when
// no hit of id can break it (not magic: damage breaks only a magic cast,
// docs/pvp/server-casts.md "Cast break"), its scaled hit time is under
// breakHit, id is stunned, asleep or rooted (its CharInfo mask), id is
// melee and cannot reach the bot before it lands (the gap past a pole's
// reach, the longest melee one, over id's run speed), or the chance id's
// hits break it inside its interrupt window is under breakOdds. Else the
// bot flees at once.
func (b *Brain) breakCastOK(id int32, s datapack.Skill, skill int32, bot, opp charmodel.Fighter, fighters bool, now time.Time) bool {
	hit := b.hitTime(skill)
	if !s.Magic || hit < breakHit {
		return true
	}
	r, o, self := b.pvp.opponents[id], b.world.object(id), &b.world.Self
	if r == nil || o == nil {
		return false
	}
	if r.Abnormal&disabledMask != 0 {
		return true
	}
	if r.Archetype == charmodel.Melee && o.RunSpeed > 0 {
		gap := dist3(self.Position(now), o.Position(now)) - poleRange - self.CollisionRadius - b.radiusOf(id)
		if time.Duration(gap/float64(o.RunSpeed)*float64(time.Second)) > hit {
			return true
		}
	}
	return fighters && charmodel.CastBreakOdds(opp, bot, hit-escLead) < breakOdds
}

// losing reports whether the bot's pool fraction (HP and CP over their
// maxima) is under player id's by its pool estimate (Brain.pool). An
// opponent of no known HP is not: blind HP errs toward fighting on.
func (b *Brain) losing(id int32) bool {
	pool, ok := b.pool(id)
	s, r := &b.world.Self, b.pvp.opponents[id]
	if !ok || s.MaxHP <= 0 {
		return false
	}
	return float64(s.HP+s.CP)*float64(r.MaxHP+r.MaxCP) < pool*float64(s.MaxHP+s.MaxCP)
}

// sustainReady reports whether a heal or potion of the bot is ready at now:
// a heal of the kit's Heals (healSkill), an HP potion out of its reuse
// (drinkPotion), or a heal or drain step of rot whose gates pass in st and
// whose skill is ready, as rule 3's sustain picks them.
func (b *Brain) sustainReady(rot []Step, st stepState, now time.Time) bool {
	if _, ok := b.healSkill(b.world.Self.ObjectID, now); ok {
		return true
	}
	if _, ok := b.drinkPotion(now); ok {
		return true
	}
	return slices.ContainsFunc(rot, func(step Step) bool {
		return step.SelfHPBelow > 0 && slices.Contains(critSustain, step.Skill) && step.passes(st) &&
			b.world.SkillReady(step.Skill, now) && !b.longCastHeld(step.Skill, now)
	})
}

// outruns reports whether the bot gets away from player id at now: not
// locked in place (immobile), and, with FleeOutrun, faster than id
// (CharInfo against UserInfo run speeds) and every other attacker
// (mayFlee).
func (b *Brain) outruns(id int32, now time.Time) bool {
	if b.immobile(now) || !b.mayFlee(now) {
		return false
	}
	if !b.persona.FleeOutrun {
		return true
	}
	o := b.world.object(id)
	return o == nil || o.RunSpeed < b.world.Self.RunSpeed
}

// pvpFoe reports whether the bot's target at now is a player it fights:
// rule 3 leaves the flee from it to the PvP layer's break-off.
func (b *Brain) pvpFoe(now time.Time) bool {
	t := b.world.Target
	return t != 0 && b.fights(t, now)
}

// immobile reports whether Ultimate Defense (110, an ImobileBuff) locks
// the bot in place at now (docs/pvp/layer.md "Ultimate Defense"): its own
// effect list shows it still running. While it is on no walk starts: the
// server idles a player that cannot move on any move, dropping every
// intention (network/movement.go startLiveMove, live_player.go tryToIdle).
func (b *Brain) immobile(now time.Time) bool {
	end, ok := b.world.Buffs[skillUltimateDefense]
	return ok && (end.IsZero() || now.Before(end))
}
