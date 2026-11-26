package botclient

import (
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The support persona (docs/plan.md, personas): heal the party, keep its
// buffs up and assist the leader. Every cast goes through fight's cast
// path rules: one at a time (castPending, castTimeout), the skill's reuse
// after its MagicSkillUse, and the frame bucket.

// buffRetry is the least time between two casts of one buff on one
// target. A buff the server does not show on the target after a cast (a
// stronger one of its kind already there, a target it could not reach)
// is cast again only that late, so it never burns the bot's MP in a loop.
const buffRetry = time.Minute

// Effect is one effect icon: the skill that put it there, at Level, with
// Left to run, unless Permanent (a toggle, or an effect with no end).
type Effect struct {
	SkillID, Level int32
	Left           time.Duration
	Permanent      bool
}

// AbnormalStatusUpdate is the bot's own whole effect icon list, sent on
// every change to its effects (network/party_spelled.go updateEffectIcons).
type AbnormalStatusUpdate struct{ Effects []Effect }

// PartySpelled is the whole effect icon list of the party member ObjectID
// (Kind partySpelledPlayer) or of a member's summon, sent to every member,
// the owner included, on every change to its effects and to a member who
// joins (network/party.go IconRefresh). It is not resent as time passes.
type PartySpelled struct {
	Kind, ObjectID int32
	Effects        []Effect
}

// partySpelledPlayer is the PartySpelled kind of a player; 1 and 2 are a
// pet and a servitor (serverpackets/status_effects.go).
const partySpelledPlayer = 0

func (AbnormalStatusUpdate) event() {}
func (PartySpelled) event()         {}

// effectSize is the wire size of one effect entry: skill id, uint16
// level, seconds left.
const effectSize = 4 + 2 + 4

// decodeAbnormalStatusUpdate: a uint16 count, then per effect skill id,
// uint16 level and whole seconds left, -1 for a permanent one
// (serverpackets/status_effects.go FrameAbnormalStatusUpdate).
func decodeAbnormalStatusUpdate(r *wire.Reader) Event {
	n := int(r.ReadUint16())
	e := AbnormalStatusUpdate{Effects: make([]Effect, 0, capFor(r, n, effectSize))}
	for ; n > 0 && r.Err() == nil; n-- {
		e.Effects = append(e.Effects, readEffect(r))
	}
	return e
}

// decodePartySpelled: kind, object id, an int32 count, then per effect
// skill id, uint16 level and whole seconds left (FramePartySpelled). A
// permanent effect's -1 ms truncates to 0 there, so 0 reads permanent: an
// effect under a second from its end reads the same, and its end resends
// the list without it.
func decodePartySpelled(r *wire.Reader) Event {
	e := PartySpelled{Kind: r.ReadInt32(), ObjectID: r.ReadInt32()}
	n := int(r.ReadInt32())
	e.Effects = make([]Effect, 0, capFor(r, n, effectSize))
	for ; n > 0 && r.Err() == nil; n-- {
		e.Effects = append(e.Effects, readEffect(r))
	}
	return e
}

// readEffect reads one effect entry; seconds left under one are permanent.
func readEffect(r *wire.Reader) Effect {
	e := Effect{SkillID: r.ReadInt32(), Level: int32(r.ReadUint16())}
	if s := r.ReadInt32(); s > 0 {
		e.Left = time.Duration(s) * time.Second
	} else {
		e.Permanent = true
	}
	return e
}

// Buffs are the effects on a character by skill id, with when each ends;
// the zero time never ends.
type Buffs map[int32]time.Time

// buffsOf is the effect list effects received at now.
func buffsOf(effects []Effect, now time.Time) Buffs {
	b := make(Buffs, len(effects))
	for _, e := range effects {
		var end time.Time
		if !e.Permanent {
			end = now.Add(e.Left)
		}
		b[e.SkillID] = end
	}
	return b
}

// levelsOf is the skill level of each of effects, by skill id.
func levelsOf(effects []Effect) map[int32]int32 {
	l := make(map[int32]int32, len(effects))
	for _, e := range effects {
		l[e.SkillID] = e.Level
	}
	return l
}

// Due reports whether skill is missing from b or ends within before of
// now.
func (b Buffs) Due(skill int32, before time.Duration, now time.Time) bool {
	end, ok := b[skill]
	return !ok || !end.IsZero() && end.Sub(now) < before
}

// applyEffects replaces the bot's own effects or a party member's.
// PartySpelled about the bot itself is left to AbnormalStatusUpdate,
// which has its exact list; a summon's is not kept.
func (w *World) applyEffects(ev Event, now time.Time) {
	switch e := ev.(type) {
	case AbnormalStatusUpdate:
		w.Buffs, w.BuffLevels = buffsOf(e.Effects, now), levelsOf(e.Effects)
	case PartySpelled:
		if e.Kind != partySpelledPlayer {
			return
		}
		if m := w.Party.Members[e.ObjectID]; m != nil {
			m.Buffs, m.BuffLevels = buffsOf(e.Effects, now), levelsOf(e.Effects)
		}
	}
}

// Heal is a heal skill and the HP percent under which the bot casts it on
// a party member. With Members it is a party heal (Group Heal, Chant of
// Life: target PARTY), cast on the bot only while at least that many of the
// members the bot supports within partyBuffReach, itself included, are
// under the percent. Role puts it in that role's heals only, as Step.Role.
type Heal struct {
	Skill   int32
	HP      int
	Members int
	Role    Role
}

// Buff is a buff skill the bot keeps up. A Party buff is cast on the bot
// and lands on the members around it; any other is cast on each member.
// Weapons casts it only while the bot wields one of those datapack weapon
// kinds; Members casts a member buff only on a member of those classes;
// Self casts it on the bot only (a row's self-buff with a target ONE skill,
// Concentration).
// From is the level the buff starts at, as Step.From; Replaces is the
// buff it replaces from that level (a row's "replaced by"): CompileBuild
// leaves that one out. Role puts it in that role's buffs only, as
// Step.Role.
type Buff struct {
	Skill   int32
	Party   bool
	Weapons []string
	Members []int32
	From    int32
	// Replaces is a skill id; 0 for none.
	Replaces int32
	Self     bool
	Role     Role
}

// assistPlan is the creature the party leader last attacked: the bot
// attacks it from at, while the leader's last hit on it (seen) is within
// attackerWindow.
type assistPlan struct {
	target   int32
	at, seen time.Time
	// wake is set until a decision runs at or after at: the plan's own
	// wake-up, which pulls decisionAt earlier and never later.
	wake bool
}

// leaderHit records the attack or hostile cast on target at now of the one
// the bot assists (assisted: its party leader, or a commander), which the
// bot assists after the persona's assist delay. A target the bot may not
// fight (a party member healed, a player the PvP policy leaves alone) is no
// assist.
func (b *Brain) leaderHit(attacker, target int32, now time.Time) {
	lead := b.assisted()
	if b.persona.AssistMax <= 0 || lead == 0 || attacker != lead || !b.mayFight(target, now) {
		return
	}
	if b.assist.target == target {
		b.assist.seen = now
		return
	}
	lo := b.persona.AssistMin
	delay := lo + time.Duration(b.rng.Int64N(int64(max(b.persona.AssistMax-lo, 0))+1))
	b.assist = assistPlan{target: target, at: now.Add(delay), seen: now, wake: true}
}

// assistTarget is the creature to assist the leader against at now, or 0.
func (b *Brain) assistTarget(now time.Time) int32 {
	a := b.assist
	if a.target == 0 || now.Before(a.at) {
		return 0
	}
	if now.Sub(a.seen) > attackerWindow || b.assisted() == 0 || !b.mayFight(a.target, now) {
		b.assist = assistPlan{}
		return 0
	}
	return a.target
}

// ruleAssist is the rule 9 assist: out of combat, attack the creature the
// leader hit once the assist delay is over, after a cast in flight (a heal
// or a buff) ends. In combat rule 5 fights it.
func (b *Brain) ruleAssist(now time.Time) (Action, bool) {
	id := b.assistTarget(now)
	if id == 0 || b.duel(now).Phase != DuelIdle {
		// From a challenge until idle no monster fight starts: a hit on
		// an NPC cancels the duel (docs/pvp/finding.md "Clear of
		// monsters").
		return Action{}, false
	}
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now), true
	case s.Casting(now) || b.castPending(now):
		return Action{}, true
	}
	return b.fight(id, now), true
}

// patients are the party members the bot supports at now, in order: the
// leader, the bot, then the others by object id. A member out of sight,
// dead, or refused by the support gate (supportAllowed) is left out.
func (b *Brain) patients(now time.Time) []int32 {
	self := b.world.Self.ObjectID
	p := &b.world.Party
	ids := []int32{self}
	others := slices.Sorted(maps.Keys(p.Members))
	if p.In() && p.LeaderID != self && slices.Contains(others, p.LeaderID) {
		ids = []int32{p.LeaderID, self}
	}
	for _, id := range others {
		if id != p.LeaderID {
			ids = append(ids, id)
		}
	}
	return slices.DeleteFunc(ids, func(id int32) bool { return id != self && !b.maySupport(id, now) })
}

// maySupport reports whether the bot may heal or buff party member id at
// now: in sight, alive and allowed by the support gate (supportAllowed).
func (b *Brain) maySupport(id int32, now time.Time) bool {
	o := b.world.object(id)
	return o != nil && !o.Dead && b.supportAllowed(id, o, now)
}

// vitals are member id's HP and maximum: the bot's own, or the party
// window's coarse row.
func (b *Brain) vitals(id int32) (hp, maxHP int32) {
	if id == b.world.Self.ObjectID {
		return b.world.Self.HP, b.world.Self.MaxHP
	}
	if m := b.world.Party.Members[id]; m != nil {
		return m.HP, m.MaxHP
	}
	return 0, 0
}

// healSkill is the build's first heal ready at now whose HP percent
// member id is under, and whose member count is; ok is false for none.
func (b *Brain) healSkill(id int32, now time.Time) (h Heal, ok bool) {
	hp, maxHP := b.vitals(id)
	if hp <= 0 {
		return Heal{}, false
	}
	held := b.partySkillHeld(now)
	for _, h := range b.build().Kit.Heals {
		if h.Members > 0 && held {
			continue
		}
		if below(hp, maxHP, h.HP) && b.world.SkillReady(h.Skill, now) && (h.Members == 0 || b.patientsUnder(h.HP, now) >= h.Members) {
			return h, true
		}
	}
	return Heal{}, false
}

// patientsUnder is how many of the patients within partyBuffReach of the
// bot, the bot included, are alive with HP under pct: those a party heal
// reaches.
func (b *Brain) patientsUnder(pct int, now time.Time) int {
	s := &b.world.Self
	from := s.Position(now)
	n := 0
	for _, id := range b.patients(now) {
		if o := b.world.object(id); id != s.ObjectID && (o == nil || dist2(from, o.Position(now)) > partyBuffReach*partyBuffReach) {
			continue
		}
		if hp, maxHP := b.vitals(id); hp > 0 && below(hp, maxHP, pct) {
			n++
		}
	}
	return n
}

// heal heals member id at now when a heal is ready for its HP: a party
// heal is cast on the bot, any other on id. It reports false when none is.
func (b *Brain) heal(id int32, now time.Time) (Action, bool) {
	h, ok := b.healSkill(id, now)
	switch {
	case !ok:
		return Action{}, false
	case h.Members > 0:
		return b.castOn(b.world.Self.ObjectID, h.Skill, false, now), true
	}
	return b.castOn(id, h.Skill, true, now), true
}

// ruleHeal is the rule 3 heal of the party: the first patient under a
// ready heal's HP percent is healed, in or out of combat. With MP under
// SitMP a bot out of combat, or sitting, leaves it to rule 6: it likely
// could not pay for the heal, and standing for it would only toggle it up
// and down against the rest.
func (b *Brain) ruleHeal(now time.Time) (Action, bool) {
	s := &b.world.Self
	if bd := b.build(); len(bd.Kit.Heals) == 0 || (s.Sitting || !s.InCombat) && below(s.MP, s.MaxMP, bd.Thresholds.SitMP) {
		return Action{}, false
	}
	ids := b.patients(now)
	for _, id := range ids {
		if a, ok := b.heal(id, now); ok {
			return a, true
		}
	}
	// A heal in flight is waited out while anyone still needs one: the
	// heal's own skill is held until its MagicSkillUse, and nothing else
	// may retarget or move the bot meanwhile.
	if (s.Casting(now) || b.castPending(now)) && slices.ContainsFunc(ids, b.wounded) {
		return Action{}, true
	}
	return Action{}, false
}

// wounded reports whether member id is alive and under the HP percent of
// a heal the bot knows.
func (b *Brain) wounded(id int32) bool {
	hp, maxHP := b.vitals(id)
	return hp > 0 && slices.ContainsFunc(b.build().Kit.Heals, func(h Heal) bool {
		_, known := b.world.Skills[h.Skill]
		return known && below(hp, maxHP, h.HP)
	})
}

// partyBuffReach is how far from the bot a party member takes its Party
// buffs: the chants' skillRadius (data/xml/skills/1000-1099.xml, 1007 and
// 1009).
const partyBuffReach = 1000

// ruleRebuff is rule 7: out of combat, turn off the combat toggles left on,
// then cast the Idle steps, then put up a buff the stack rule lets go out
// on a patient (buffDue: missing, or its own effect with under
// RebuffBefore left), one cast at a time. A Party buff is cast on the bot
// while some patient within partyBuffReach, the bot included, needs it.
func (b *Brain) ruleRebuff(now time.Time) (Action, bool) {
	s := &b.world.Self
	kit := b.build().Kit
	if s.InCombat {
		return Action{}, false
	}
	if a, ok := b.toggleCast(kit.Toggles, false, now); ok {
		return a, true
	}
	if a, ok := b.idleStep(kit.Rotation, now); ok {
		return a, true
	}
	if len(kit.Buffs) == 0 {
		return Action{}, false
	}
	weapon := b.buffWeapon(kit.Buffs)
	from := s.Position(now)
	held := b.partySkillHeld(now)
	for _, id := range b.patients(now) {
		for _, bf := range kit.Buffs {
			on := id
			if bf.Party {
				if held {
					continue
				}
				on = s.ObjectID
				if o := b.world.object(id); id != s.ObjectID && (o == nil || dist2(from, o.Position(now)) > partyBuffReach*partyBuffReach) {
					continue
				}
			}
			if bf.Self && id != s.ObjectID || !bf.fits(weapon, b.classOf(id)) || !b.buffDue(id, bf.Skill, now) ||
				!b.world.SkillReady(bf.Skill, now) || now.Sub(b.buffedAt[buffKey{on, bf.Skill}]) < buffRetry {
				continue
			}
			return b.castOn(on, bf.Skill, !bf.Party, now), true
		}
	}
	return Action{}, false
}

// buffKey is a buff on one target.
type buffKey struct{ target, skill int32 }

// castOn casts skill on id at now: stand up, wait out a cast in flight,
// select id, then cast. A Party buff (selectFirst false) is cast on the
// bot whatever it has selected.
func (b *Brain) castOn(id, skill int32, selectFirst bool, now time.Time) Action {
	s := &b.world.Self
	switch {
	case s.Sitting:
		return b.toggle(now)
	case s.Casting(now) || b.castPending(now):
		return Action{}
	case selectFirst && b.world.Target != id:
		return selectAction(id, s.Position(now))
	}
	return Action{kind: actCast, payload: encodeRequestMagicSkillUse(skill, false), object: id, skill: skill}
}

// buffSent records a buff cast going out at now, for buffRetry.
func (b *Brain) buffSent(a Action, now time.Time) {
	if !slices.ContainsFunc(b.build().Kit.Buffs, func(bf Buff) bool { return bf.Skill == a.skill }) {
		return
	}
	maps.DeleteFunc(b.buffedAt, func(_ buffKey, at time.Time) bool { return now.Sub(at) >= buffRetry })
	b.buffedAt[buffKey{a.object, a.skill}] = now
}
