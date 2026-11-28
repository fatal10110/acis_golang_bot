package botclient

import (
	"hash/fnv"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// line is one way through a base class's tree: its first profession, the
// second profession that follows it, and the weight of the pair in its
// role's draw (docs/strategy.md, Design section 6).
type line struct {
	first, second int32
	weight        uint64
}

// branchTables are the professions by role and base class (docs/strategy.md,
// Design section 6, D22): Support takes the healer and buffer lines for the
// mystics and the race's tank line for the fighters (Orc Raider and
// Scavenger for the orc and the dwarf, which have none); Damage farms alone,
// weighted by race fit: dark elven STR to daggers, elven DEX to scouts and
// archers, orc CON to two-handed weapons. The crafter role is the dwarven
// Artisan line (D18). These are the tables with the persona switch
// ServitorLines off, its default (D51): the servitor lines (Dark Avenger,
// Warlock, Elemental Summoner, Phantom Summoner) and the Necromancer are
// picked only with it on (servitorBranches), and the Overlord never (D23).
// Every first profession is a direct child of its base class and every
// second one of its first (TestBranchTables).
var branchTables = map[Role]map[int32][]line{
	RoleSupport: {
		0:  {{4, 5, 1}},                // Knight → Paladin
		18: {{19, 20, 1}},              // Elven Knight → Temple Knight
		31: {{32, 33, 1}},              // Palus Knight → Shillien Knight
		44: {{45, 46, 1}},              // Orc Raider → Destroyer
		53: {{54, 55, 1}},              // Scavenger → Bounty Hunter
		10: {{15, 16, 2}, {15, 17, 1}}, // Cleric → Bishop, Prophet
		25: {{29, 30, 1}},              // Elven Oracle → Elven Elder
		38: {{42, 43, 1}},              // Shillien Oracle → Shillien Elder
		49: {{50, 52, 1}},              // Orc Shaman → Warcryer
	},
	RoleDamage: {
		0:  {{1, 2, 2}, {1, 3, 1}, {7, 8, 1}, {7, 9, 1}}, // Warrior → Gladiator, Warlord; Rogue → Treasure Hunter, Hawkeye
		18: {{19, 21, 1}, {22, 23, 1}, {22, 24, 1}},      // Elven Knight → Sword Singer; Elven Scout → Plains Walker, Silver Ranger
		31: {{32, 34, 1}, {35, 36, 2}, {35, 37, 1}},      // Palus Knight → Bladedancer; Assassin → Abyss Walker, Phantom Ranger
		44: {{45, 46, 2}, {47, 48, 1}},                   // Orc Raider → Destroyer; Monk → Tyrant
		53: {{54, 55, 1}},                                // Scavenger → Bounty Hunter
		10: {{11, 12, 1}},                                // Human Wizard → Sorcerer
		25: {{26, 27, 1}},                                // Elven Wizard → Spellsinger
		38: {{39, 40, 1}},                                // Dark Wizard → Spellhowler
		49: {{50, 52, 1}},                                // Orc Shaman → Warcryer
	},
	RoleCrafter: {
		53: {{classArtisan, classWarsmith, 1}},
	},
}

// servitorBranches are the lines that replace branchTables' for their role
// and base class while the persona switch ServitorLines is on
// (docs/servitors.md section 5 "Branch tables", D51 answers 2, 4 and 6):
// the Dark Avenger a Support line next to the Paladin, the Necromancer and
// the summoners Damage lines next to their first profession's nuker. The
// first professions are the tables' own, so the branch pins
// (BranchPins) do not change with the switch.
var servitorBranches = map[Role]map[int32][]line{
	RoleSupport: {
		0: {{4, classPaladin, 2}, {4, classDarkAvenger, 1}}, // Knight → Paladin, Dark Avenger
	},
	RoleDamage: {
		10: {{11, classSorcerer, 2}, {11, classNecromancer, 1}, {11, classWarlock, 1}}, // Human Wizard → Sorcerer, Necromancer, Warlock
		25: {{26, classSpellsinger, 2}, {26, classElementalSummoner, 1}},               // Elven Wizard → Spellsinger, Elemental Summoner
		38: {{39, classSpellhowler, 2}, {39, classPhantomSummoner, 1}},                 // Dark Wizard → Spellhowler, Phantom Summoner
	},
}

// servitorTables are branchTables with servitorBranches' lines in place of
// theirs: the tables of a bot whose persona turns ServitorLines on.
var servitorTables = func() map[Role]map[int32][]line {
	tabs := make(map[Role]map[int32][]line, len(branchTables))
	for r, tab := range branchTables {
		tabs[r] = make(map[int32][]line, len(tab))
		for base, ls := range tab {
			tabs[r][base] = ls
		}
		for base, ls := range servitorBranches[r] {
			tabs[r][base] = ls
		}
	}
	return tabs
}()

// tableOrder is the order the kept-line rule reads the other roles' tables
// in when the bot's own names nothing for its class.
var tableOrder = []Role{RoleSupport, RoleDamage, RoleCrafter}

// BranchChoice is what the branch draw reads besides the class and role: the
// account name whose hash, with the role, draws the branch, so every session
// of the bot picks the same one, and the pinned branch (a first profession's
// class id, 0 for none) that wins over the hash while the bot is a base
// class (D22). ServitorLines draws from the tables with the servitor lines
// (servitorTables), the persona switch's (Persona.ServitorLines, D51);
// false draws from branchTables, as before the switch.
type BranchChoice struct {
	Account       string
	Pin           int32
	ServitorLines bool
}

// tables are the branch tables the choice draws from.
func (c BranchChoice) tables() map[Role]map[int32][]line {
	if c.ServitorLines {
		return servitorTables
	}
	return branchTables
}

// hash is the draw's 64 bits for role: FNV-1a of the account name and the
// role. The low half draws the first profession, the high half the second.
func (c BranchChoice) hash(role Role) uint64 {
	h := fnv.New64a()
	h.Write([]byte(c.Account + "/" + role.String())) //nolint:errcheck // hash.Hash never fails
	return h.Sum64()
}

// branchLines are the lines of role's table of tabs for base, the Damage
// table's when role's names none (a role that farms alone).
func branchLines(tabs map[Role]map[int32][]line, role Role, base int32) []line {
	if ls := tabs[role][base]; len(ls) > 0 {
		return ls
	}
	return tabs[RoleDamage][base]
}

// keptLines are the lines of tabs that go on from first: role's table's
// when it names first, else the first other table's in tableOrder that
// does.
func keptLines(tabs map[Role]map[int32][]line, role Role, first int32) []line {
	of := func(r Role) []line {
		var ls []line
		for _, tab := range tabs[r] {
			for _, l := range tab {
				if l.first == first {
					ls = append(ls, l)
				}
			}
		}
		return ls
	}
	if ls := of(role); len(ls) > 0 {
		return ls
	}
	for _, r := range tableOrder {
		if ls := of(r); r != role && len(ls) > 0 {
			return ls
		}
	}
	return nil
}

// drawLine is the line of ls a draw of r picks, each line's chance its
// weight's share; ok is false for no lines.
func drawLine(ls []line, r uint64) (line, bool) {
	var total uint64
	for _, l := range ls {
		total += l.weight
	}
	if total == 0 {
		return line{}, false
	}
	r %= total
	for _, l := range ls {
		if r < l.weight {
			return l, true
		}
		r -= l.weight
	}
	return line{}, false
}

// chooseLine is the line of a bot of class in role (docs/strategy.md, Design
// section 6, D22): a base class draws its first profession from its role's
// table, each first weighted by the sum of its lines, unless c pins one the
// tables name for it; a profession keeps its line. Either way the second
// profession is drawn among the first's lines (keptLines) by the hash's
// high half, so a base bot's draw and its later one at the first profession
// agree while the role does not change. A second profession's line is its
// own; ok is false for a class no table goes on from.
func chooseLine(class int32, role Role, c BranchChoice) (line, bool) {
	h, tabs := c.hash(role), c.tables()
	parent, ok := datapack.ParentClass(class)
	switch {
	case !ok:
		return line{}, false
	case parent < 0:
		first := c.Pin
		if first == 0 || !slices.ContainsFunc(keptLines(tabs, role, first), func(l line) bool { return isChildOf(l.first, class) }) {
			var firsts []line
			for _, l := range branchLines(tabs, role, class) {
				if i := slices.IndexFunc(firsts, func(f line) bool { return f.first == l.first }); i >= 0 {
					firsts[i].weight += l.weight
				} else {
					firsts = append(firsts, line{first: l.first, weight: l.weight})
				}
			}
			f, ok := drawLine(firsts, uint64(uint32(h)))
			if !ok {
				return line{}, false
			}
			first = f.first
		}
		return drawLine(keptLines(tabs, role, first), h>>32)
	}
	if grand, _ := datapack.ParentClass(parent); grand < 0 {
		return drawLine(keptLines(tabs, role, class), h>>32)
	}
	return line{first: parent, second: class}, true
}

// BranchPins are the first professions of base class the branch tables
// name, every role's, ascending: the branches a bot of base may be pinned to
// (BrainConfig.Branch, D22); none for a class that is not a base class.
func BranchPins(base int32) []int32 {
	var pins []int32
	for _, r := range tableOrder {
		for _, l := range branchTables[r][base] {
			if !slices.Contains(pins, l.first) {
				pins = append(pins, l.first)
			}
		}
	}
	slices.Sort(pins)
	return pins
}

// BranchBase is the base class pin is a branch of (BranchPins); ok is false
// for a class the tables name as no base class's first profession.
func BranchBase(pin int32) (base int32, ok bool) {
	parent, ok := datapack.ParentClass(pin)
	if !ok || parent < 0 || !slices.Contains(BranchPins(parent), pin) {
		return 0, false
	}
	return parent, true
}

// isChildOf reports whether class upgrades from parent.
func isChildOf(class, parent int32) bool {
	p, ok := datapack.ParentClass(class)
	return ok && p == parent
}

// branchWeapons are the weapon kinds of the lines whose base class and first
// profession wield another weapon than the gear table's for them, by second
// profession (docs/strategy.md, column 1 of rows HF EF DF OF RO ES AS): the
// dagger and bow branches of the human, elven and dark elven fighters, and
// the orc fighter's Monk branch. Their base row wears no shield.
var branchWeapons = map[int32][]string{
	8: daggerBranch, 23: daggerBranch, 36: daggerBranch,
	9: bowBranch, 24: bowBranch, 37: bowBranch,
	48: fistWeapons,
}

// lineWeapons are the weapon kinds of the first professions whose row names
// one per line, by second profession (docs/strategy.md, column 1 of row
// WA): the Warrior's two-handed blunt on the Gladiator line (Stun Attack and
// Power Smash take it) and its polearm on the Warlord line (Wild Sweep).
// Their base row keeps its own (row HF: a sword or a blunt with a shield).
var lineWeapons = map[int32][]string{
	2: {"BIGBLUNT"},
	3: {"POLE"},
}
