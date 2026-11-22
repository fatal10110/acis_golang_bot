package botclient

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"time"
)

// classMaster is the class master's NPC template id (Hasha, the Class
// Manager, data/xml/npcs/50000-50999.xml; D15).
const classMaster int32 = 50000

// The class master's commands (classmaster/command.go): the menu of the
// first, second and third transfer, and the transfer to a class of the menu.
const (
	firstClassCommand  = "1stClass"
	secondClassCommand = "2ndClass"
	thirdClassCommand  = "3rdClass"
	changeClassCommand = "change_class"
)

// firstProfessions are the class ids of the first professions, the classes
// of the datapack's baseLvl 20 (data/xml/classes; model/actor/player/class.go
// classParent).
var firstProfessions = []int32{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}

// ClassChangePolicy is when and to what the bot changes class at a class
// master (rule 9 errand "classchange", D15). Every value is a configurable
// default (D6).
type ClassChangePolicy struct {
	// Builds is the persona's build table: the profession the bot takes
	// next, by the class it has. A base class maps to a first profession,
	// taken from level 20, a first profession to a second one, taken from
	// level 40, a second to a third, from 76. Class ids are per race, so the
	// table is per race too. A class with no entry is kept.
	Builds map[int32]int32
	// Masters are the NPC template ids of the class masters.
	Masters []int32
	// Reach is how far the bot walks to a master's spawn (from the travel
	// map, BrainConfig.Travel); a master in sight is visited from anywhere,
	// and 0 visits only one in sight.
	Reach int32
	// RetryAfter is how long the bot leaves the masters alone after a visit
	// that changed nothing: a master not at its spawn or out of reach, a
	// page without the build's profession, or a transfer unanswered within
	// trainerReplyTimeout.
	RetryAfter time.Duration
}

// supportBuilds is the default persona's build table, over each race's own
// tree (data/xml/classes, classParent): mystics take the healer and buffer
// line (Cleric and Bishop, Elven Oracle and Elven Elder, Shillien Oracle and
// Shillien Elder, Orc Shaman and Warcryer) whose heals and buffs the persona
// casts; fighters a melee line (Warrior and Gladiator, Elven Knight and
// Sword Singer, Palus Knight and Bladedancer, Orc Raider and Destroyer,
// Scavenger and Bounty Hunter).
var supportBuilds = map[int32]int32{
	0: 1, 1: 2, // Human Fighter: Warrior, Gladiator
	10: 15, 15: 16, // Human Mystic: Cleric, Bishop
	18: 19, 19: 21, // Elven Fighter: Elven Knight, Sword Singer
	25: 29, 29: 30, // Elven Mystic: Elven Oracle, Elven Elder
	31: 32, 32: 34, // Dark Fighter: Palus Knight, Bladedancer
	38: 42, 42: 43, // Dark Mystic: Shillien Oracle, Shillien Elder
	44: 45, 45: 46, // Orc Fighter: Orc Raider, Destroyer
	49: 50, 50: 52, // Orc Mystic: Orc Shaman, Warcryer
	53: 54, 54: 55, // Dwarven Fighter: Scavenger, Bounty Hunter
}

// Class ids of the dwarven crafter line (D18, docs/crafting.md).
const (
	classArtisan  int32 = 56
	classWarsmith int32 = 57
)

// defaultClassChangePolicy changes class at the class master with the
// support build table, walking to a master nearby; a visit that changed
// nothing waits visitShunFor before the next.
func defaultClassChangePolicy() ClassChangePolicy {
	return ClassChangePolicy{
		Builds:     maps.Clone(supportBuilds),
		Masters:    []int32{classMaster},
		Reach:      8000,
		RetryAfter: visitShunFor,
	}
}

// transferTier is the level from which class has its next transfer and the
// master's command for that transfer's menu (classmaster/dialog.go
// changeLevel): 20 and the first menu from a base class, 40 and the second
// from a first profession, 76 and the third from a second one.
func transferTier(class int32) (int32, string) {
	switch {
	case slices.ContainsFunc(StartingClasses(), func(c StartingClass) bool { return c.ID == class }):
		return 20, firstClassCommand
	case slices.Contains(firstProfessions, class):
		return 40, secondClassCommand
	}
	return 76, thirdClassCommand
}

// classVisit is the bot's errand at a class master (rule "classchange"). It
// is owned by the brain goroutine.
type classVisit struct {
	v npcVisit
	// from is the class the visit started at: a UserInfo with another
	// class ends it changed.
	from int32
	// menu is set once the visit sent the tier's menu link, asked once
	// set when a bypass went out and its answer is still awaited.
	menu, asked bool
	// retryAt is when the next visit may start after one that changed
	// nothing.
	retryAt time.Time
}

// backOff ends the visit, changing nothing, and leaves the masters alone
// for RetryAfter.
func (c *classVisit) backOff(now time.Time, d time.Duration) {
	c.v, c.menu, c.asked, c.retryAt = npcVisit{}, false, false, now.Add(d)
}

// sent records a packet of the visit that went out at now: a bypass to the
// master awaits its answer.
func (c *classVisit) sent(a Action, now time.Time) {
	if !c.v.active() {
		return
	}
	if a.kind == actBypass && a.object != 0 && a.object == c.v.obj {
		c.asked = true
	}
	c.v.sent(a, now)
}

// ruleClassChange is a rule 9 errand (D15): with the build's next
// profession due at the bot's level, the bot walks to a class master, talks
// to it, follows the page's link to the tier's menu and the menu's
// change_class link to the build's profession. The transfer's success is
// the new class in UserInfo; the server refuses a transfer in silence, so a
// bypass unanswered within trainerReplyTimeout, a page with neither link, or
// a master not found ends the visit for RetryAfter. Bypasses pass the bypass
// gate (gateBypass); legs and clicks the frame pacer, a visit spending at
// most visitClicks clicks. The page's learn_skills link is never used: the
// bot pays SP at its trainer (D6).
func (b *Brain) ruleClassChange(now time.Time) (Action, bool) {
	c, p := &b.class, &b.persona.ClassChange
	if c.v.active() && b.world.Self.ClassID != c.from {
		b.classChanged()
	}
	if b.world.Self.InCombat || (!c.v.active() && !b.startClassChange(now)) {
		return Action{}, false
	}
	v := &c.v
	if c.asked {
		if now.Before(v.waitUntil) {
			return Action{}, true
		}
		// Unanswered: the server refused the transfer in silence.
		c.backOff(now, p.RetryAfter)
		return Action{}, false
	}
	if v.opened {
		if cmd, ok := b.classLink(); ok {
			return bypassTo(v.obj, cmd), true
		}
		c.backOff(now, p.RetryAfter)
		return Action{}, false
	}
	a, ok := b.approach(v, now)
	if !ok {
		c.backOff(now, p.RetryAfter)
		return Action{}, false
	}
	return a, true
}

// classLink is the link of the master's last page the visit follows: the
// change_class link of the build's profession on the menu, else the tier's
// menu link once. A page with neither offers the bot nothing.
func (b *Brain) classLink() (string, bool) {
	c, s := &b.class, &b.world.Self
	prefix := "npc_" + strconv.Itoa(int(c.v.obj)) + "_"
	want := b.persona.ClassChange.Builds[s.ClassID]
	if cmd := prefix + changeClassCommand + " " + strconv.Itoa(int(want)); b.world.Dialog.Has(cmd) {
		return cmd, true
	}
	if _, tier := transferTier(s.ClassID); !c.menu && b.world.Dialog.Has(prefix+tier) {
		c.menu = true
		return prefix + tier, true
	}
	return "", false
}

// classChanged ends the visit of a bot whose class changed and makes a
// trainer visit due at once: the new class learns its own skills there,
// even at the level the last visit was at.
func (b *Brain) classChanged() {
	b.class.v, b.class.menu, b.class.asked = npcVisit{}, false, false
	b.train.doneLevel, b.train.needSP = 0, 0
}

// startClassChange starts a visit when the build's next profession is due
// at the bot's level: at the nearest master in sight, else at the nearest
// master spawn within Reach, reporting whether it did. A party member stays
// with its party and a bot told to follow stays with its commander: neither
// walks to a master out of sight.
func (b *Brain) startClassChange(now time.Time) bool {
	c, s, p := &b.class, &b.world.Self, &b.persona.ClassChange
	if p.Builds[s.ClassID] == 0 || now.Before(c.retryAt) {
		return false
	}
	if level, _ := transferTier(s.ClassID); s.Level < level {
		return false
	}
	from := s.Position(now)
	var best npcVisit
	bestDist := int64(math.MaxInt64)
	for _, npcID := range p.Masters {
		if id := b.nearestNPC(npcID, from, now); id != 0 {
			at := b.world.Objects[id].Position(now)
			if d := dist2(from, at); d < bestDist || best.obj == 0 {
				best, bestDist = npcVisit{npcID: npcID, at: at, obj: id}, d
			}
		}
	}
	if !best.active() && b.cfg.Travel != nil && !b.world.Party.In() && b.orders.follow == 0 {
		reach := int64(p.Reach) * int64(p.Reach)
		for _, npcID := range p.Masters {
			for _, at := range b.cfg.Travel.spawns[npcID] {
				if d := dist2(from, at); d <= reach && d < bestDist {
					best, bestDist = npcVisit{npcID: npcID, at: at}, d
				}
			}
		}
	}
	if !best.active() {
		return false
	}
	c.v, c.from, c.menu, c.asked = best, s.ClassID, false, false
	return true
}

// classPage moves the visit on with the master's page.
func (b *Brain) classPage(ev NpcHtmlMessage) {
	if b.class.v.active() && b.class.v.page(ev) {
		b.class.asked = false
	}
}
