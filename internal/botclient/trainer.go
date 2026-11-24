package botclient

import (
	"math"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Trainer visit timing.
const (
	// trainerTalkTimeout is how long a click on the selected trainer waits
	// for its dialog: out of reach, the server walks the bot over first
	// (MoveToPawn), then opens the page.
	trainerTalkTimeout = 20 * time.Second
	// trainerReplyTimeout is how long a bypass or a skill request waits for
	// its answer. The server answers nothing to one it refuses (out of
	// reach, not the next level of the skill), so the visit talks again.
	trainerReplyTimeout = 5 * time.Second
	// trainerClicks is how many clicks (selections and talks) a visit
	// spends before it leaves the trainer alone for trainerShunFor.
	trainerClicks  = 6
	trainerShunFor = 10 * time.Minute
)

// acquireSkillUsual is the list type of the class trainer's skills
// (skill_acquisition.go acquireSkillTypeUsual).
const acquireSkillUsual int32 = 0

// skillListCommand is the trainer page's link to its skill list
// (model/actor/npc/folk_bypass.go BypassSkillList).
const skillListCommand = "SkillList"

// Class ids of the four mystic base classes (data/xml/classes).
const (
	classHumanMystic int32 = 10
	classElvenMystic int32 = 25
	classDarkMystic  int32 = 38
	classOrcMystic   int32 = 49
)

// Skill ids of the support persona's skills (data/xml/classes).
const (
	skillChantBattle int32 = 1007
	skillSoulShield  int32 = 1010
	skillHeal        int32 = 1011
	skillBattleHeal  int32 = 1015
	skillShield      int32 = 1040
	skillMight       int32 = 1068
	// The mystics' attack spells: Wind Strike for the human, elven and
	// dark elven lines, Life Drain and Chill Flame for the orc line.
	skillLifeDrain  int32 = 1090
	skillChillFlame int32 = 1100
	skillWindStrike int32 = 1177
)

// Class ids of the healer and buffer professions of the support build
// (supportBuilds; data/xml/classes).
const (
	classCleric         int32 = 15
	classBishop         int32 = 16
	classElvenOracle    int32 = 29
	classElvenElder     int32 = 30
	classShillienOracle int32 = 42
	classShillienElder  int32 = 43
	classOrcShaman      int32 = 50
	classWarcryer       int32 = 52
)

// mysticSkills are the support persona's skills by mystic base class and by
// profession of the support build (docs/plan.md, support persona): human,
// elven and dark elven mystics heal and buff, orc mystics buff. A
// profession's trainer offers the next level of every skill of its tree
// merged with its parents' (Cleric: Heal, Battle Heal, Might and Shield up
// to 35; Shillien and Elven Elder: Might and Shield at 40-44; Orc Shaman:
// Soul Shield and Chant of Battle at 25-35, Warcryer at 44). Their attack
// spells come after them, so the SP goes to the support skills first: Wind
// Strike (level 1 granted at creation, 2-5 at 7-14), Life Drain and Chill
// Flame. Ids and levels are the datapack's class skill trees
// (data/xml/classes).
var mysticSkills = map[int32][]int32{
	classHumanMystic:    {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classElvenMystic:    {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classDarkMystic:     {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classOrcMystic:      {skillSoulShield, skillChantBattle, skillLifeDrain, skillChillFlame},
	classCleric:         {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classBishop:         {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classElvenOracle:    {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classElvenElder:     {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classShillienOracle: {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classShillienElder:  {skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike},
	classOrcShaman:      {skillSoulShield, skillChantBattle, skillLifeDrain, skillChillFlame},
	classWarcryer:       {skillSoulShield, skillChantBattle, skillLifeDrain, skillChillFlame},
}

// mysticTrainers are the NPC template ids of the trainers that teach each
// mystic base class and support build profession: the datapack NPCs whose
// teachTo lists it (data/xml/npcs). A trainer for another class opens its
// no-skills page. Grand Magister Celes (32098) teaches human mystics, Clerics
// and Bishops too but is left out: its first page
// (data/html/villagemaster/32098.htm) has no skill list link, so a visit
// would only end in a shun.
var mysticTrainers = map[int32][]int32{
	classHumanMystic: {
		30022, 30030, 30032, 30033, 30034, 30035, 30036, 30067, 30069, 30110, 30111, 30114,
		30116, 30117, 30118, 30171, 30188, 30190, 30344, 30695, 30696, 30715, 30855, 30858,
		30859, 30906, 30907, 31280, 31281, 31282, 31329, 31332, 31969,
	},
	classElvenMystic: {
		30068, 30112, 30113, 30119, 30157, 30158, 30189, 30293, 30345, 30375, 30376, 30461,
		30473, 30680, 30697, 30698, 30717, 30718, 30856, 30860, 30861, 30908, 30909, 31280,
		31281, 31283, 31330, 31333, 31970, 31971, 31972,
	},
	classDarkMystic: darkMysticTrainers,
	classOrcMystic:  orcMysticTrainers,
	classCleric:     clericTrainers,
	classBishop:     clericTrainers,
	classElvenOracle: {
		30068, 30119, 30158, 30293, 30375, 30473, 30680, 30698, 30860, 30861, 30908, 31280,
		31281, 31283, 31330, 31333, 31970, 31971, 31972,
	},
	classElvenElder: {
		30068, 30119, 30158, 30293, 30375, 30473, 30680, 30698, 30860, 30861, 30908, 31280,
		31281, 31283, 31330, 31333, 31970, 31972,
	},
	classShillienOracle: darkMysticTrainers,
	classShillienElder:  darkMysticTrainers,
	classOrcShaman:      orcMysticTrainers,
	classWarcryer:       orcMysticTrainers,
}

// The trainers that teach a whole line alike (teachTo, data/xml/npcs): the
// dark elven mystic line, the orc mystic line, and Clerics and Bishops.
var (
	darkMysticTrainers = []int32{
		30144, 30145, 30194, 30330, 30377, 30464, 30476, 30701, 30720, 30721, 30833, 30864,
		30912, 31287, 31335, 31581, 31976,
	}
	orcMysticTrainers = []int32{
		30502, 30507, 30510, 30515, 30571, 30572, 30682, 30706, 30867, 30915, 31290, 31337,
		31979,
	}
	clericTrainers = []int32{
		30022, 30030, 30032, 30036, 30067, 30116, 30117, 30118, 30188, 30835, 30858, 30859,
		30906, 31280, 31281, 31282, 31329, 31332, 31969,
	}
)

// AcquirableSkill is one skill level a trainer offers: the next level of a
// skill the bot may learn at its level, and its SP cost.
type AcquirableSkill struct{ ID, Level, Cost int32 }

// AcquireSkillList is a trainer's list of skills the bot may learn now, of
// list type Type (0 the class trainer's).
type AcquireSkillList struct {
	Type   int32
	Skills []AcquirableSkill
}

// SkillRequirement is an item learning a skill level takes: Type 99 marks
// a spellbook.
type SkillRequirement struct{ Type, ItemID, Count int32 }

// AcquireSkillInfo answers RequestAcquireSkillInfo: the SP cost of one
// offered skill level, corrected for the bot, and the items it takes.
type AcquireSkillInfo struct {
	ID, Level, Cost, Type int32
	Requirements          []SkillRequirement
}

// spellbookRequirement is the requirement type of a spellbook
// (serverpackets/acquireskill.go).
const spellbookRequirement int32 = 99

// AcquireSkillDone closes the learn window: the trainer has nothing left to
// teach the bot at its level.
type AcquireSkillDone struct{}

func (AcquireSkillList) event() {}
func (AcquireSkillInfo) event() {}
func (AcquireSkillDone) event() {}

// Wire sizes of a list entry (five int32s) and a requirement (four).
const (
	acquirableSkillSize  = 5 * 4
	skillRequirementSize = 4 * 4
)

// decodeAcquireSkillList: list type, count, then per skill id, level
// written twice, SP cost and a zero (serverpackets/acquireskill.go).
func decodeAcquireSkillList(r *wire.Reader) Event {
	l := AcquireSkillList{Type: r.ReadInt32()}
	n := int(r.ReadInt32())
	l.Skills = make([]AcquirableSkill, 0, capFor(r, n, acquirableSkillSize))
	for ; n > 0 && r.Err() == nil; n-- {
		s := AcquirableSkill{ID: r.ReadInt32(), Level: r.ReadInt32()}
		r.Skip(4) // the level again
		s.Cost = r.ReadInt32()
		r.Skip(4)
		l.Skills = append(l.Skills, s)
	}
	return l
}

// decodeAcquireSkillInfo: skill id, level, SP cost, list type, count, then
// per requirement type, item id, count and a trailing value
// (serverpackets/acquireskill.go).
func decodeAcquireSkillInfo(r *wire.Reader) Event {
	i := AcquireSkillInfo{ID: r.ReadInt32(), Level: r.ReadInt32(), Cost: r.ReadInt32(), Type: r.ReadInt32()}
	n := int(r.ReadInt32())
	if n == 0 {
		return i
	}
	i.Requirements = make([]SkillRequirement, 0, capFor(r, n, skillRequirementSize))
	for ; n > 0 && r.Err() == nil; n-- {
		i.Requirements = append(i.Requirements, SkillRequirement{Type: r.ReadInt32(), ItemID: r.ReadInt32(), Count: r.ReadInt32()})
		r.Skip(4)
	}
	return i
}

// encodeSkillRequest is RequestAcquireSkillInfo or RequestAcquireSkill
// (opcode op): skill id, level, list type (clientpackets/acquire_skill.go).
func encodeSkillRequest(op byte, id, level, typ int32) []byte {
	w := wire.NewPacketWriter(op)
	w.WriteInt32(id)
	w.WriteInt32(level)
	w.WriteInt32(typ)
	return w.Bytes()
}

// talkAction clicks the selected NPC id from origin, which talks to it.
func talkAction(id int32, from Point) Action {
	return Action{kind: actTalk, payload: encodeAction(id, from), object: id}
}

// trainStep is where a trainer visit stands.
type trainStep uint8

const (
	// trainTalk selects the trainer and talks to it until its page opens.
	trainTalk trainStep = iota
	// trainOpen sends the page's skill list link.
	trainOpen
	// trainPick asks the cost of the next wanted skill the list offers.
	trainPick
	// trainLearn learns the skill whose cost came back affordable.
	trainLearn
)

// trainerVisit is the bot's errand at its class trainer (rule 9). It is
// owned by the brain goroutine. A visit is due once per level and when the
// SP reaches the cheapest wanted skill the last visit could not afford.
type trainerVisit struct {
	// npc is the trainer's object id while a visit runs, 0 otherwise.
	npc  int32
	step trainStep
	// waitUntil, when set, is when the answer to the last packet of the
	// visit is given up on.
	waitUntil time.Time
	clicks    int
	// offer is the trainer's last list; pending is the skill asked about or
	// learned and not yet answered.
	offer   []AcquirableSkill
	pending AcquirableSkill
	// skipped are the skills of this visit that failed to learn or cost
	// more than the bot has; short is the cheapest of those it could not
	// afford.
	skipped map[int32]bool
	short   int32
	// again makes the visit end due again: a spellbook it may have skipped
	// a skill for arrived while it ran (booksArrived).
	again bool
	// doneLevel is the bot's level at the end of its last visit, 0 before
	// the first; needSP, when set, makes a visit due again at that SP.
	doneLevel, needSP int32
	// shunned are the trainers that taught the bot nothing, by object id,
	// until a time.
	shunned map[int32]time.Time
	// walk is the walk to a trainer's spawn while none is in sight (zero
	// for none); walkShunned are the trainer templates whose spawn the bot
	// reached without finding one or that no route reaches, until a time.
	walk        npcVisit
	walkShunned map[int32]time.Time
	// near holds the stand point of the trainer in sight the bot walks to
	// before it clicks (closeIn): its object and stand point only.
	near npcVisit
}

// reset ends the visit in progress.
func (t *trainerVisit) reset() {
	t.npc, t.step, t.waitUntil, t.clicks = 0, trainTalk, time.Time{}, 0
	t.offer, t.pending, t.skipped, t.short, t.again = nil, AcquirableSkill{}, nil, 0, false
	t.walk, t.near = npcVisit{}, npcVisit{}
}

// finish ends a visit that got the trainer's list as far as it could go at
// level.
func (t *trainerVisit) finish(level int32) {
	t.doneLevel, t.needSP = level, t.short
	if t.again {
		t.doneLevel, t.needSP = 0, 0
	}
	t.reset()
}

// shun ends the visit and leaves its trainer alone until now plus
// trainerShunFor.
func (t *trainerVisit) shun(now time.Time) {
	if t.shunned == nil {
		t.shunned = make(map[int32]time.Time)
	}
	t.shunned[t.npc] = now.Add(trainerShunFor)
	t.reset()
}

// skip leaves skill s out of the rest of the visit; one the bot could not
// afford at cost makes the next visit due at that SP.
func (t *trainerVisit) skip(s AcquirableSkill, cost, sp int32) {
	if t.skipped == nil {
		t.skipped = make(map[int32]bool)
	}
	t.skipped[s.ID] = true
	if cost > sp && (t.short == 0 || cost < t.short) {
		t.short = cost
	}
}

// sent records a packet of the visit that went out at now.
func (t *trainerVisit) sent(a Action, now time.Time) {
	if t.npc == 0 {
		return
	}
	switch a.kind {
	case actSelect:
		if a.object == t.npc {
			t.clicks++
		}
	case actTalk:
		t.clicks++
		t.waitUntil = now.Add(trainerTalkTimeout)
	case actBypass, actSkillInfo, actLearnSkill:
		t.waitUntil = now.Add(trainerReplyTimeout)
	}
}

// skillListBypass is the bypass of trainer id's skill list link
// (bypass.go bypassNpc: npc_<object id>_<command>).
func skillListBypass(id int32) string {
	return "npc_" + strconv.Itoa(int(id)) + "_" + skillListCommand
}

// ruleTrain is a rule 9 errand: with a visit due, the bot walks to the
// nearest spawn of a trainer of its class until one is in sight, talks to
// the trainer, opens its skill list and learns the wanted skills it can
// afford, one request at a time, each after the server's answer to the
// last. Legs pass the frame pacer like the spot rule's.
func (b *Brain) ruleTrain(now time.Time) (Action, bool) {
	t := &b.train
	if t.npc == 0 {
		if !b.startVisit(now) {
			return Action{}, false
		}
		if t.npc == 0 {
			return b.trainerLeg(now)
		}
	}
	o := b.world.object(t.npc)
	if o == nil || o.Dead {
		// Out of sight: the visit starts over in this decision, by the walk
		// to a spawn when no other trainer is in sight, so the errand runs
		// on without a gap. The trainer is gone or dead, so startVisit
		// cannot pick it again.
		t.reset()
		return b.ruleTrain(now)
	}
	if !t.waitUntil.IsZero() {
		if now.Before(t.waitUntil) {
			return Action{}, true
		}
		// Unanswered: the server refused in silence; talk again.
		t.waitUntil, t.step, t.pending = time.Time{}, trainTalk, AcquirableSkill{}
	}
	s := &b.world.Self
	switch t.step {
	case trainOpen:
		if b.world.Dialog.Has(skillListBypass(t.npc)) {
			return bypassAction(skillListBypass(t.npc)), true
		}
		// Another page replaced the trainer's: the server would refuse
		// the link, so talk again.
		t.step = trainTalk
	case trainPick:
		sk, ok := b.nextSkill()
		if !ok {
			t.finish(s.Level)
			return Action{}, false
		}
		t.pending = sk
		return Action{kind: actSkillInfo, payload: encodeSkillRequest(opGameRequestAcquireSkillInfo, sk.ID, sk.Level, acquireSkillUsual)}, true
	case trainLearn:
		sk := t.pending
		return Action{kind: actLearnSkill, payload: encodeSkillRequest(opGameRequestAcquireSkill, sk.ID, sk.Level, acquireSkillUsual)}, true
	}
	// The click walks the bot straight at the trainer, never routed: walk
	// to a stand point with an open line first, as every NPC visit does.
	if t.near.obj != t.npc {
		t.near = npcVisit{obj: t.npc}
	}
	if a, ok := b.closeIn(&t.near, visitArrive, now); ok {
		return a, true
	}
	if t.clicks >= trainerClicks {
		t.shun(now)
		return Action{}, false
	}
	if s.Sitting {
		return b.toggle(now), true
	}
	if b.world.Target != t.npc {
		return selectAction(t.npc, s.Position(now)), true
	}
	b.walk.talking(o.Position(now), now)
	return talkAction(t.npc, s.Position(now)), true
}

// startVisit starts a visit when one is due: at the nearest trainer of the
// bot's class in sight, else by a walk to the nearest spawn of one within
// Persona.TrainerReach (from the travel map), reporting whether a visit or
// walk runs.
func (b *Brain) startVisit(now time.Time) bool {
	t, s := &b.train, &b.world.Self
	if len(b.build().Kit.Skills) == 0 ||
		(t.doneLevel != 0 && s.Level <= t.doneLevel && (t.needSP == 0 || s.SP < t.needSP)) {
		t.walk = npcVisit{}
		return false
	}
	from := s.Position(now)
	var best int32
	var bestDist int64
	for _, npcID := range b.build().Kit.Trainers {
		for id, o := range b.world.Objects {
			if o.Kind != KindNPC || o.NPCID != npcID || o.Dead || now.Before(t.shunned[id]) {
				continue
			}
			if d := dist2(from, o.Position(now)); best == 0 || d < bestDist {
				best, bestDist = id, d
			}
		}
	}
	if best != 0 {
		t.reset()
		t.npc = best
		return true
	}
	return b.startTrainerWalk(from, now)
}

// startTrainerWalk keeps or starts the walk to the nearest spawn of a
// trainer of the bot's class within Persona.TrainerReach, reporting
// whether one runs. A walk the bot left the reach of (a teleport) is
// dropped. A party member stays with its party and a bot told to follow
// stays with its commander: neither walks to a trainer out of sight. After
// a walk that got nowhere, none starts again before the next level.
func (b *Brain) startTrainerWalk(from Point, now time.Time) bool {
	t, m := &b.train, b.cfg.Travel
	reach := int64(b.persona.TrainerReach) * int64(b.persona.TrainerReach)
	if m == nil || reach == 0 || b.world.Party.In() || b.orders.follow != 0 {
		t.walk = npcVisit{}
		return false
	}
	if t.walk.active() && dist2(from, t.walk.at) <= reach {
		return true
	}
	var best npcVisit
	bestDist := int64(math.MaxInt64)
	for _, npcID := range b.build().Kit.Trainers {
		if now.Before(t.walkShunned[npcID]) {
			continue
		}
		for _, at := range m.spawns[npcID] {
			if d := dist2(from, at); d <= reach && d < bestDist {
				best, bestDist = npcVisit{npcID: npcID, at: at}, d
			}
		}
	}
	t.walk = best
	return best.active()
}

// trainerLeg is the next leg of the walk to a trainer's spawn. Arrived
// with no trainer in sight, the bot leaves that trainer template alone for
// visitShunFor, and so with no route there that holds.
func (b *Brain) trainerLeg(now time.Time) (Action, bool) {
	t := &b.train
	if a, ok := b.walkTo(&t.walk, now); ok {
		return a, true
	}
	if t.walkShunned == nil {
		t.walkShunned = make(map[int32]time.Time)
	}
	t.walkShunned[t.walk.npcID] = now.Add(visitShunFor)
	t.walk = npcVisit{}
	return Action{}, false
}

// dist2 is the squared horizontal distance between a and b.
func dist2(a, b Point) int64 {
	dx, dy := int64(a.X-b.X), int64(a.Y-b.Y)
	return dx*dx + dy*dy
}

// nextSkill is the first wanted skill, in the persona's order, the list
// offers and the bot has the SP for; skills it cannot afford are skipped.
func (b *Brain) nextSkill() (AcquirableSkill, bool) {
	t, sp := &b.train, b.world.Self.SP
	for _, id := range b.build().Kit.Skills {
		for _, s := range t.offer {
			if s.ID != id || t.skipped[id] {
				continue
			}
			if s.Cost > sp {
				t.skip(s, s.Cost, sp)
				continue
			}
			return s, true
		}
	}
	return AcquirableSkill{}, false
}

// trainEvent moves the visit on with the server's answer ev.
func (b *Brain) trainEvent(ev Event, now time.Time) {
	t := &b.train
	if t.npc == 0 {
		return
	}
	switch e := ev.(type) {
	case NpcHtmlMessage:
		if e.ObjectID != t.npc {
			return
		}
		if !b.world.Dialog.Has(skillListBypass(t.npc)) {
			// A page with no skill list link: the trainer does not teach
			// the bot's class (its no-skills page).
			t.shun(now)
			return
		}
		t.step, t.waitUntil = trainOpen, time.Time{}
	case AcquireSkillList:
		if e.Type != acquireSkillUsual {
			return
		}
		if p := t.pending; p.ID != 0 && b.world.Skills[p.ID].Level < p.Level {
			// The learn failed (SP short, a missing spellbook).
			t.skip(p, 0, 0)
		}
		t.offer, t.pending = e.Skills, AcquirableSkill{}
		t.step, t.waitUntil = trainPick, time.Time{}
	case AcquireSkillInfo:
		p := t.pending
		if t.step != trainPick || e.ID != p.ID || e.Level != p.Level {
			return
		}
		t.waitUntil = time.Time{}
		if sp := b.world.Self.SP; e.Cost > sp {
			t.skip(p, e.Cost, sp)
			t.pending = AcquirableSkill{}
			return
		}
		if r, ok := b.missingItem(e.Requirements); ok {
			// The server refuses the learn without the item
			// (ItemMissingToLearnSkill); a spellbook goes on the shop
			// list, and its purchase makes the visit due again.
			t.skip(p, 0, 0)
			t.pending = AcquirableSkill{}
			if r.Type == spellbookRequirement {
				b.shop.wantBook(r.ItemID)
			}
			return
		}
		t.step = trainLearn
	case AcquireSkillDone:
		t.finish(b.world.Self.Level)
	}
}
