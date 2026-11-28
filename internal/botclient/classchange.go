package botclient

import (
	"bytes"
	"slices"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

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
	// table is per race too. A class with no entry is kept. nil plays the
	// build's professions (Persona, overrides).
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

// Class ids of the dwarven crafter line (D18, docs/crafting.md).
const (
	classArtisan  int32 = 56
	classWarsmith int32 = 57
)

// defaultClassChangePolicy changes class at the class master to the
// build's professions (chooseLine), walking to a master nearby; a visit that changed
// nothing waits visitShunFor before the next.
func defaultClassChangePolicy() ClassChangePolicy {
	return ClassChangePolicy{
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
	// nobleAsked is set once become_noble went out (one per bot: the
	// server shows it noble from then on), nobling while its page is
	// awaited.
	nobleAsked, nobling bool
	// retryAt is when the next visit may start after one that changed
	// nothing.
	retryAt time.Time
}

// backOff ends the visit, changing nothing, and leaves the masters alone
// for RetryAfter.
func (c *classVisit) backOff(now time.Time, d time.Duration) {
	c.v, c.menu, c.asked, c.nobling, c.retryAt = npcVisit{}, false, false, false, now.Add(d)
}

// sent records a packet of the visit that went out at now: a bypass to the
// master awaits its answer.
func (c *classVisit) sent(a Action, now time.Time) {
	if !c.v.active() {
		return
	}
	if a.kind == actBypass && a.object != 0 && a.object == c.v.obj {
		c.asked = true
		if bytes.Equal(a.payload, botwire.EncodeRequestBypassToServer(npcBypass(c.v.obj, becomeNobleCommand))) {
			c.nobleAsked, c.nobling = true, true
		}
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
// become_noble link of an Olympiad bot not noble yet (nobleDue,
// docs/pvp/olympiad.md "Noblesse"), else the change_class link of the
// build's profession on the menu, else the tier's menu link once. A page
// with none offers the bot nothing.
func (b *Brain) classLink() (string, bool) {
	c, s := &b.class, &b.world.Self
	if cmd := npcBypass(c.v.obj, becomeNobleCommand); b.nobleDue() && b.world.Dialog.Has(cmd) {
		return cmd, true
	}
	want := b.build().Race.Professions[s.ClassID]
	if cmd := npcBypass(c.v.obj, changeClassCommand+" "+strconv.Itoa(int(want))); b.world.Dialog.Has(cmd) {
		return cmd, true
	}
	if _, tier := transferTier(s.ClassID); !c.menu {
		if cmd := npcBypass(c.v.obj, tier); b.world.Dialog.Has(cmd) {
			c.menu = true
			return cmd, true
		}
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
// at the bot's level, or an Olympiad bot's noblesse (nobleDue), at a master
// nearby (pickVisit), reporting whether it did.
func (b *Brain) startClassChange(now time.Time) bool {
	c, s, p := &b.class, &b.world.Self, &b.persona.ClassChange
	if now.Before(c.retryAt) || !b.professionDue() && !b.nobleDue() {
		return false
	}
	best, ok := b.pickVisit(p.Masters, p.Reach, now)
	if !ok {
		return false
	}
	c.v, c.from, c.menu, c.asked = best, s.ClassID, false, false
	return true
}

// professionDue reports whether the build's next profession is due at the
// bot's level.
func (b *Brain) professionDue() bool {
	s := &b.world.Self
	if b.build().Race.Professions[s.ClassID] == 0 {
		return false
	}
	level, _ := transferTier(s.ClassID)
	return s.Level >= level
}

// classPage moves the visit on with the master's page. The page answering
// become_noble has no link (classmaster/dialog.go NoblePage, 50000-6.htm):
// a visit with a profession due talks to the master again for its main
// page, else it is over.
func (b *Brain) classPage(ev NpcHtmlMessage) {
	c := &b.class
	if !c.v.active() || !c.v.page(ev) {
		return
	}
	c.asked = false
	if c.nobling {
		c.nobling = false
		if b.professionDue() {
			c.v.opened = false
		} else {
			c.v, c.menu = npcVisit{}, false
		}
	}
}
