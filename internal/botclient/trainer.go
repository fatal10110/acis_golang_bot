package botclient

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
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
// (branchTables; data/xml/classes).
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

// noSkillPage are trainers whose teachTo lists classes but whose first page
// has no skill list link, so a visit would only end in a shun: Grand
// Magister Celes (32098, data/html/villagemaster/32098.htm).
var noSkillPage = []int32{32098}

// indexTrainers maps each class id to the NPC templates whose teachTo lists
// it (data/xml/npcs), ascending, without noSkillPage: the trainers a bot of
// that class learns its skills at (Brain.build). A trainer for another
// class opens its no-skills page.
func indexTrainers(npcs map[int32]datapack.NPC) map[int32][]int32 {
	trainers := make(map[int32][]int32)
	for id, n := range npcs {
		if slices.Contains(noSkillPage, id) {
			continue
		}
		for _, c := range n.TeachTo {
			trainers[c] = append(trainers[c], id)
		}
	}
	for _, ids := range trainers {
		slices.Sort(ids)
	}
	return trainers
}

// spellbookRequirement is the requirement type of a spellbook
// (serverpackets/acquireskill.go).
const spellbookRequirement int32 = 99

// talkAction clicks the selected NPC id from origin, which talks to it.
func talkAction(id int32, from Point) Action {
	return Action{kind: actTalk, payload: botwire.EncodeAction(id, from), object: id}
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
	npc int32
	// v is the way to the trainer's page (approach): the stand point, the
	// clicks and their wait, at trainer npc.
	v    npcVisit
	step trainStep
	// waitUntil, when set, is when the answer to the last packet of the
	// visit after the page (the skill list link, a skill request) is given
	// up on.
	waitUntil time.Time
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
	shunned holds[int32]
	// walk is the walk to a trainer's spawn while none is in sight (zero
	// for none); walkShunned are the trainer templates whose spawn the bot
	// reached without finding one or that no route reaches, until a time.
	walk        npcVisit
	walkShunned holds[int32]
}

// reset ends the visit in progress.
func (t *trainerVisit) reset() {
	t.npc, t.step, t.waitUntil = 0, trainTalk, time.Time{}
	t.offer, t.pending, t.skipped, t.short, t.again = nil, AcquirableSkill{}, nil, 0, false
	t.v, t.walk = npcVisit{}, npcVisit{}
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

// shun ends the visit and leaves its trainer alone, by object id, until now
// plus visitShunFor: a second trainer of the class in sight is still picked.
func (t *trainerVisit) shun(now time.Time) {
	t.shunned.set(t.npc, now.Add(visitShunFor))
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

// sent records a packet of the visit that went out at now: a click or a
// bypass acting on the trainer, or a skill request (which only the trainer
// visit sends). Another row's click or bypass on another NPC counts for
// nothing here.
func (t *trainerVisit) sent(a Action, now time.Time) {
	if t.npc == 0 {
		return
	}
	switch a.kind {
	case actSelect, actTalk:
		t.v.sent(a, now)
	case actBypass:
		if a.object == t.npc {
			t.waitUntil = now.Add(trainerReplyTimeout)
		}
	case actSkillInfo, actLearnSkill:
		t.waitUntil = now.Add(trainerReplyTimeout)
	}
}

// skillListBypass is the bypass of trainer id's skill list link
// (bypass.go bypassNpc: npc_<object id>_<command>).
func skillListBypass(id int32) string {
	return npcBypass(id, skillListCommand)
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
		// cannot pick it again. This runs before approach, which would look
		// again for the same template instead.
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
			return bypassTo(t.npc, skillListBypass(t.npc)), true
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
		return Action{kind: actSkillInfo, payload: botwire.EncodeSkillRequest(opGameRequestAcquireSkillInfo, sk.ID, sk.Level, acquireSkillUsual)}, true
	case trainLearn:
		sk := t.pending
		return Action{kind: actLearnSkill, payload: botwire.EncodeSkillRequest(opGameRequestAcquireSkill, sk.ID, sk.Level, acquireSkillUsual)}, true
	}
	// The way to the page, as every NPC visit's: a stand point with an open
	// line first, then the clicks, bounded by visitClicks.
	a, ok := b.approach(&t.v, now)
	if !ok {
		t.shun(now)
		return Action{}, false
	}
	return a, true
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
	var best npcVisit
	var bestDist int64
	for _, npcID := range b.build().Kit.Trainers {
		for id, o := range b.world.Objects {
			if o.Kind != KindNPC || o.NPCID != npcID || o.Dead || t.shunned.held(id, now) {
				continue
			}
			at := o.Position(now)
			if d := dist2(from, at); best.obj == 0 || d < bestDist {
				best, bestDist = npcVisit{npcID: npcID, at: at, obj: id}, d
			}
		}
	}
	if best.obj != 0 {
		t.reset()
		t.npc, t.v = best.obj, best
		return true
	}
	return b.startTrainerWalk(from, now)
}

// startTrainerWalk keeps or starts the walk to the nearest spawn of a
// trainer of the bot's class within Persona.TrainerReach, reporting
// whether one runs. A walk the bot left the reach of (a teleport) is
// dropped. A member of a player's party (or of no bot party of its own)
// stays with its party and a bot told to follow stays with its commander:
// neither walks to a trainer out of sight. A bot in its own bot party
// (inBotParty) walks within the reach and back, its party following or
// gathering (docs/parties/members.md, "Errands in a bot party"). After a
// walk that got nowhere, none starts again before the next level.
func (b *Brain) startTrainerWalk(from Point, now time.Time) bool {
	t, m := &b.train, b.cfg.Travel
	reach := int64(b.persona.TrainerReach) * int64(b.persona.TrainerReach)
	if m == nil || reach == 0 || b.world.Party.In() && !b.inBotParty() || b.orders.follow != 0 {
		t.walk = npcVisit{}
		return false
	}
	if t.walk.active() && dist2(from, t.walk.at) <= reach {
		return true
	}
	var best npcVisit
	bestDist := int64(math.MaxInt64)
	for _, npcID := range b.build().Kit.Trainers {
		if t.walkShunned.held(npcID, now) {
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
	t.walkShunned.set(t.walk.npcID, now.Add(visitShunFor))
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
		if !t.v.page(e) {
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
