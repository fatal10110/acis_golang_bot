package botclient

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Class ids of the branch tables' lines the tests name (data/xml/classes).
const (
	classGladiator      int32 = 2
	classWarlord        int32 = 3
	classPaladin        int32 = 5
	classTempleKnight   int32 = 20
	classSwordSinger    int32 = 21
	classShillienKnight int32 = 33
	classBladedancer    int32 = 34
)

// accountFor is the first account name of the form "botN" whose draw for a
// base class in role is want.
func accountFor(t *testing.T, base int32, role Role, want line) string {
	t.Helper()
	for i := range 1000 {
		a := fmt.Sprintf("bot%d", i)
		if l, _ := chooseLine(base, role, BranchChoice{Account: a}); l.first == want.first && l.second == want.second {
			return a
		}
	}
	t.Fatalf("no account draws %d -> %d for class %d %v", want.first, want.second, base, role)
	return ""
}

// TestBranchTables checks every line of the tables goes from its base class
// to a direct child and on to the child's direct child, in one race, and
// that no line picks a servitor line, the Necromancer or the Overlord
// (D23).
func TestBranchTables(t *testing.T) {
	never := []int32{13, 14, 28, 41, 51} // Necromancer, Warlock, Elemental Summoner, Phantom Summoner, Overlord
	for role, tab := range branchTables {
		for base, ls := range tab {
			if p, ok := datapack.ParentClass(base); !ok || p >= 0 {
				t.Errorf("%v: class %d is not a base class", role, base)
			}
			for _, l := range ls {
				if !isChildOf(l.first, base) || !isChildOf(l.second, l.first) || l.weight == 0 {
					t.Errorf("%v class %d: line %+v is not base -> child -> child", role, base, l)
				}
				if slices.Contains(never, l.first) || slices.Contains(never, l.second) {
					t.Errorf("%v class %d: line %+v picks a class D23 leaves out", role, base, l)
				}
			}
		}
	}
	for _, c := range StartingClasses() {
		for _, role := range []Role{RoleSupport, RoleDamage} {
			if len(branchTables[role][c.ID]) == 0 {
				t.Errorf("%v: no line for starting class %d", role, c.ID)
			}
		}
	}
}

// TestBranchStable checks the same account and role always draw the same
// line, a base bot's draw and its draw at its first profession agree, and
// across accounts every line of the role's table is drawn.
func TestBranchStable(t *testing.T) {
	for _, role := range []Role{RoleSupport, RoleDamage} {
		for base, ls := range branchTables[role] {
			drawn := map[line]bool{}
			for i := range 400 {
				c := BranchChoice{Account: fmt.Sprintf("acct%d", i)}
				l, ok := chooseLine(base, role, c)
				if again, _ := chooseLine(base, role, c); !ok || again != l {
					t.Fatalf("%v class %d %q: draws %+v then %+v", role, base, c.Account, l, again)
				}
				if kept, _ := chooseLine(l.first, role, c); kept != l {
					t.Errorf("%v class %d %q: base draws %+v, its first profession %+v", role, base, c.Account, l, kept)
				}
				drawn[l] = true
			}
			for _, l := range ls {
				if !drawn[l] {
					t.Errorf("%v class %d: line %+v never drawn", role, base, l)
				}
			}
		}
	}
}

// TestBranchByRole checks the role tables (docs/strategy.md, Design section
// 6): support fighters take their race's tank line, support mystics the
// healer line, damage mystics the wizard line, and the compiled build
// carries the line as Branch and professions.
func TestBranchByRole(t *testing.T) {
	tests := []struct {
		base   int32
		role   Role
		firsts []int32
	}{
		{HumanFighter.ID, RoleSupport, []int32{classKnight}},
		{ElvenFighter.ID, RoleSupport, []int32{classElvenKnight}},
		{DarkFighter.ID, RoleSupport, []int32{classPalusKnight}},
		{OrcFighter.ID, RoleSupport, []int32{classOrcRaider}},
		{DwarvenFighter.ID, RoleSupport, []int32{classScavenger}},
		{classHumanMystic, RoleSupport, []int32{classCleric}},
		{classHumanMystic, RoleDamage, []int32{classHumanWizard}},
		{classElvenMystic, RoleDamage, []int32{26}},
		{classDarkMystic, RoleDamage, []int32{39}},
		{classOrcMystic, RoleDamage, []int32{classOrcShaman}},
		{HumanFighter.ID, RoleDamage, []int32{classWarrior, classRogue}},
		{DwarvenFighter.ID, RoleCrafter, []int32{classArtisan}},
	}
	for _, tt := range tests {
		for i := range 50 {
			c := BranchChoice{Account: fmt.Sprintf("acct%d", i)}
			race, _ := datapack.ClassRace(tt.base)
			bd := CompileBuild(race, tt.base, 10, tt.role, c)
			first := bd.Race.Professions[tt.base]
			if !slices.Contains(tt.firsts, first) || bd.Branch != first || !isChildOf(bd.Race.Professions[first], first) {
				t.Fatalf("class %d %v %q: branch %d professions %v, want a first of %v", tt.base, tt.role, c.Account, bd.Branch, bd.Race.Professions, tt.firsts)
			}
		}
	}
}

// TestBranchKeptLine checks a bot with a profession keeps its line under any
// role: its next profession is a child of its class, from its role's table
// when that names the class, else the other role's, drawn by the hash.
func TestBranchKeptLine(t *testing.T) {
	tests := []struct {
		name  string
		class int32
		role  Role
		want  []int32
	}{
		{"support Warrior", classWarrior, RoleSupport, []int32{classGladiator, classWarlord}},
		{"damage Warrior", classWarrior, RoleDamage, []int32{classGladiator, classWarlord}},
		{"support Elven Knight", classElvenKnight, RoleSupport, []int32{classTempleKnight}},
		{"damage Elven Knight", classElvenKnight, RoleDamage, []int32{classSwordSinger}},
		{"support Palus Knight", classPalusKnight, RoleSupport, []int32{classShillienKnight}},
		{"damage Palus Knight", classPalusKnight, RoleDamage, []int32{classBladedancer}},
		{"damage Knight", classKnight, RoleDamage, []int32{classPaladin}},
		{"support Rogue", classRogue, RoleSupport, []int32{classTreasureHunter, classHawkeye}},
		{"damage Cleric", classCleric, RoleDamage, []int32{classBishop, 17}},
		{"support Human Wizard", classHumanWizard, RoleSupport, []int32{12}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[int32]bool{}
			for i := range 200 {
				c := BranchChoice{Account: fmt.Sprintf("acct%d", i), Pin: classKnight}
				race, _ := datapack.ClassRace(tt.class)
				bd := CompileBuild(race, tt.class, 25, tt.role, c)
				next := bd.Race.Professions[tt.class]
				if !slices.Contains(tt.want, next) || bd.Branch != tt.class {
					t.Fatalf("%q: branch %d next %d, want branch %d and one of %v (the pin is a base class's)", c.Account, bd.Branch, next, tt.class, tt.want)
				}
				seen[next] = true
			}
			if len(seen) != len(tt.want) {
				t.Errorf("drew %v, want every one of %v", seen, tt.want)
			}
		})
	}
	// A second profession plays its first profession's line and takes no
	// profession next (the third is out of scope).
	if bd := CompileBuild(RaceHuman, classGladiator, 45, RoleSupport, BranchChoice{}); bd.Branch != classWarrior || len(bd.Race.Professions) != 0 {
		t.Errorf("Gladiator: branch %d professions %v, want Warrior and none", bd.Branch, bd.Race.Professions)
	}
}

// TestBranchPin checks a pinned branch the tables name for the bot's base
// class wins over the hash, in any role, and a pin they do not name is
// ignored.
func TestBranchPin(t *testing.T) {
	tests := []struct {
		name string
		base int32
		role Role
		pin  int32
		want int32
	}{
		{"support pins Rogue", HumanFighter.ID, RoleSupport, classRogue, classRogue},
		{"damage pins Knight", HumanFighter.ID, RoleDamage, classKnight, classKnight},
		{"support pins Elven Scout", ElvenFighter.ID, RoleSupport, 22, 22},
		{"support mystic pins Wizard", classHumanMystic, RoleSupport, classHumanWizard, classHumanWizard},
		{"a servitor line is ignored", classHumanMystic, RoleSupport, 14, classCleric},
		{"Dark Avenger is in no table", HumanFighter.ID, RoleSupport, 6, classKnight},
		{"another race's class is ignored", HumanFighter.ID, RoleSupport, classElvenKnight, classKnight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 50 {
				c := BranchChoice{Account: fmt.Sprintf("acct%d", i), Pin: tt.pin}
				race, _ := datapack.ClassRace(tt.base)
				bd := CompileBuild(race, tt.base, 5, tt.role, c)
				if bd.Branch != tt.want || bd.Race.Professions[tt.base] != tt.want || !isChildOf(bd.Race.Professions[tt.want], tt.want) {
					t.Fatalf("%q: branch %d professions %v, want %d", c.Account, bd.Branch, bd.Race.Professions, tt.want)
				}
			}
		})
	}
}

// TestBranchWeapon checks a base row's weapon follows its line from level 1
// (docs/strategy.md, column 1), and the first profession's too: the Rogue
// line's dagger or bow, the Monk line's fists, a shield otherwise; the
// Warrior's own line's two-hander leaves its base row's one-hander and
// shield as they are.
func TestBranchWeapon(t *testing.T) {
	tests := []struct {
		name    string
		base    int32
		role    Role
		line    line
		weapons []string
		shield  bool
	}{
		{"Treasure Hunter line", HumanFighter.ID, RoleDamage, line{first: classRogue, second: classTreasureHunter}, daggerBranch, false},
		{"Hawkeye line", HumanFighter.ID, RoleDamage, line{first: classRogue, second: classHawkeye}, bowBranch, false},
		{"Gladiator line", HumanFighter.ID, RoleDamage, line{first: classWarrior, second: classGladiator}, []string{"SWORD", "BLUNT"}, true},
		{"Warlord line", HumanFighter.ID, RoleDamage, line{first: classWarrior, second: classWarlord}, []string{"SWORD", "BLUNT"}, true},
		{"Knight line", HumanFighter.ID, RoleSupport, line{first: classKnight, second: classPaladin}, []string{"SWORD", "BLUNT"}, true},
		{"Silver Ranger line", ElvenFighter.ID, RoleDamage, line{first: 22, second: 24}, bowBranch, false},
		{"Abyss Walker line", DarkFighter.ID, RoleDamage, line{first: classAssassin, second: 36}, daggerBranch, false},
		{"Tyrant line", OrcFighter.ID, RoleDamage, line{first: classMonk, second: 48}, fistWeapons, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := BranchChoice{Account: accountFor(t, tt.base, tt.role, tt.line)}
			race, _ := datapack.ClassRace(tt.base)
			g := CompileBuild(race, tt.base, 1, tt.role, c).Kit.Gear
			if !slices.Equal(g.Weapons, tt.weapons) || g.Shield != tt.shield || g.Armor != "LIGHT" {
				t.Errorf("base row gear %+v, want %v shield %v in light armor", g, tt.weapons, tt.shield)
			}
			w := branchWeapons[tt.line.second]
			if lw := lineWeapons[tt.line.second]; lw != nil {
				w = lw
			}
			if w != nil {
				if g := CompileBuild(race, tt.line.first, 20, tt.role, c).Kit.Gear; !slices.Equal(g.Weapons, w) || g.Shield {
					t.Errorf("first profession gear %+v, want %v", g, w)
				}
			}
		})
	}
}

// TestBranchClassChange checks the brain's build follows the bot's line: a
// support human fighter heads for Knight at the master, and its class change
// at 20 switches the compiled build to the Knight's row, heading for
// Paladin; a pinned branch wins; a damage human mystic picks the Human
// Wizard on the master's menu.
func TestBranchClassChange(t *testing.T) {
	b, _ := newTestBrain(t, DefaultPersona())
	b.cfg.account, b.builtOK = "acct1", false
	b.world.Apply(UserInfo{ObjectID: selfID, Name: "bot7", ClassID: HumanFighter.ID, Level: 19, RunSpeed: 100, WalkSpeed: 50}, t0)
	base := b.build()
	if base.Branch != classKnight || base.Race.Professions[HumanFighter.ID] != classKnight || base.Race.Professions[classKnight] != classPaladin {
		t.Fatalf("level 19 build: branch %d professions %v, want the base row heading for Knight", base.Branch, base.Race.Professions)
	}
	b.world.Apply(UserInfo{ObjectID: selfID, Name: "bot7", ClassID: classKnight, Level: 20, RunSpeed: 100, WalkSpeed: 50}, t0)
	want := CompileBuild(RaceHuman, classKnight, 20, RoleSupport, BranchChoice{Account: "acct1"})
	bd := b.build()
	if bd.Branch != classKnight || bd.Race.Professions[classKnight] != classPaladin || !slices.Equal(bd.Kit.Skills, want.Kit.Skills) ||
		slices.Equal(bd.Kit.Skills, base.Kit.Skills) || !slices.Equal(stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation)) {
		t.Fatalf("Knight build: branch %d professions %v skills %v, want the Knight's row heading for Paladin", bd.Branch, bd.Race.Professions, bd.Kit.Skills)
	}

	pinned, _ := newTestBrain(t, DefaultPersona())
	pinned.cfg.account, pinned.cfg.Branch, pinned.builtOK = "acct1", classRogue, false
	pinned.world.Apply(UserInfo{ObjectID: selfID, Name: "bot7", ClassID: HumanFighter.ID, Level: 20, RunSpeed: 100, WalkSpeed: 50}, t0)
	if bd := pinned.build(); bd.Branch != classRogue || bd.Race.Professions[HumanFighter.ID] != classRogue {
		t.Fatalf("pinned build: branch %d professions %v, want Rogue", bd.Branch, bd.Race.Professions)
	}

	m := newClassBrain(t, classHumanMystic, 20)
	m.persona.Role, m.builtOK = RoleDamage, false
	m.train.doneLevel = 20
	runTrainerSteps(t, m, []trainerStep{
		{name: "talk", at: time.Second, setup: masterInSight, wantRule: "classchange", wantKind: actTalk},
		{name: "first transfer's menu", at: 3 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, "1stClass")},
		{name: "the line's profession", at: 5 * time.Second, events: []Event{menuPage(masterObj, classHumanWizard, classCleric)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, "change_class 11")},
	})
}
