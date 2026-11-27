package botclient

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// masterObj is the class master (NPC 50000) once in sight.
const masterObj = int32(0x20000050)

// masterPage is the master's first page (data/html/mods/classmaster/
// 50000.htm) with obj for %objectId%.
func masterPage(obj int32) NpcHtmlMessage {
	html := `<html><body>Class Manager:<br>How may I assist you?<br><center>` +
		`<a action="bypass -h npc_%objectId%_1stClass">Complete the first class transfer</a><br>` +
		`<a action="bypass -h npc_%objectId%_2ndClass">Complete the second class transfer</a><br>` +
		`<a action="bypass -h npc_%objectId%_3rdClass">Complete the third class transfer</a><br>` +
		`<a action="bypass -h npc_%objectId%_become_noble">Become noble</a><br>` +
		`<a action="bypass -h npc_%objectId%_learn_skills">Learn all skills</a></center></body></html>`
	return NpcHtmlMessage{ObjectID: obj, HTML: strings.ReplaceAll(html, "%objectId%", strconv.Itoa(int(obj)))}
}

// menuPage is the master's menu (classmaster/dialog.go MenuPage, page -2)
// with one change_class link per class.
func menuPage(obj int32, classes ...int32) NpcHtmlMessage {
	var b strings.Builder
	b.WriteString(`<html><body>Class Manager:<br>`)
	for _, c := range classes {
		b.WriteString(`<a action="bypass -h npc_` + strconv.Itoa(int(obj)) + `_change_class ` + strconv.Itoa(int(c)) + `">Class</a><br>`)
	}
	b.WriteString(`</body></html>`)
	return NpcHtmlMessage{ObjectID: obj, HTML: b.String()}
}

func masterBypass(obj int32, cmd string) []byte {
	return botwire.EncodeRequestBypassToServer("npc_" + strconv.Itoa(int(obj)) + "_" + cmd)
}

// newClassBrain is a bot of class and level at the origin with the default
// persona, no idle drift, and a class master spawned at (5000, 0) on its
// travel map.
func newClassBrain(t *testing.T, class, level int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level = class, level
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{classMaster: {{X: 5000}}}, trainers: testTrainers}
	return walkOn(t, b)
}

// masterInSight puts the master in sight 200 units away, selected, as the
// live server shows it (liveMaster).
func masterInSight(b *Brain) {
	n := liveMaster
	n.ObjectID, n.Pos = masterObj, Point{X: 200}
	b.world.Apply(n, t0)
	b.world.Target = masterObj
}

// TestBrainClassChange takes a level 20 human mystic to the master, asks
// for the first transfer's menu and picks the build's Cleric; the new class
// in UserInfo ends the visit and makes a trainer visit due.
func TestBrainClassChange(t *testing.T) {
	b := newClassBrain(t, classHumanMystic, 20)
	b.train.doneLevel = 20
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: time.Second, setup: masterInSight, wantRule: "classchange", wantKind: actTalk},
		{name: "wait for the page", at: 2 * time.Second, wantRule: "classchange"},
		{name: "first transfer's menu", at: 3 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, "1stClass")},
		{name: "wait for the menu", at: 4 * time.Second, wantRule: "classchange"},
		{name: "the build's profession", at: 5 * time.Second, events: []Event{menuPage(masterObj, 11, classCleric)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, "change_class 15")},
		{name: "wait for the transfer", at: 6 * time.Second, wantRule: "classchange"},
		{
			name: "changed: the visit ends", at: 7 * time.Second,
			setup:  func(b *Brain) { b.world.Self.ClassID = classCleric },
			events: []Event{NpcHtmlMessage{ObjectID: masterObj, HTML: "<html><body>changed</body></html>"}},
		},
	})
	if b.class.v.active() || !b.class.retryAt.IsZero() {
		t.Fatalf("visit %+v, want it ended with no back-off", b.class)
	}
	if b.train.doneLevel != 0 {
		t.Fatalf("trainer doneLevel = %d, want 0: the new class learns at its trainer", b.train.doneLevel)
	}
}

// TestBrainClassChangeTrains checks the profession learns its skills at its
// trainer at the level the base class last trained at.
func TestBrainClassChangeTrains(t *testing.T) {
	b := newClassBrain(t, classHumanMystic, 20)
	b.world.Apply(NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: Point{X: 300}}, t0)
	b.train.doneLevel = 20
	if rule, a := b.decide(at(time.Second)); rule == "train" {
		t.Fatalf("decide = %q %d before the change, want no trainer visit", rule, a.kind)
	}
	b.class.v, b.class.from = npcVisit{npcID: classMaster, at: Point{X: 5000}}, classHumanMystic
	b.world.Self.ClassID = classCleric
	if rule, a := b.decide(at(2 * time.Second)); rule != "train" || a.kind != actSelect {
		t.Fatalf("decide = %q %d after the change, want the trainer selected", rule, a.kind)
	}
}

// TestClassChangeDue checks when a visit starts.
func TestClassChangeDue(t *testing.T) {
	tests := []struct {
		name         string
		class, level int32
		setup        func(b *Brain)
		wantKind     actionKind // actHold: no visit
	}{
		{name: "base class below 20", class: classHumanMystic, level: 19, setup: masterInSight},
		{name: "base class at 20", class: classHumanMystic, level: 20, setup: masterInSight, wantKind: actTalk},
		{name: "base class past 20", class: HumanFighter.ID, level: 27, setup: masterInSight, wantKind: actTalk},
		{name: "first profession below 40", class: classCleric, level: 39, setup: masterInSight},
		{name: "first profession at 40", class: classCleric, level: 40, setup: masterInSight, wantKind: actTalk},
		{name: "class without a build entry", class: classBishop, level: 76, setup: masterInSight},
		{name: "build entry removed", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			masterInSight(b)
			b.persona.ClassChange.Builds = map[int32]int32{classCleric: classBishop}
		}},
		{name: "backing off", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			masterInSight(b)
			b.class.retryAt = at(time.Hour)
		}},
		{name: "in combat", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			masterInSight(b)
			b.world.Self.InCombat = true
		}},
		{name: "a master in sight beyond reach", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			masterInSight(b)
			b.persona.ClassChange.Reach = 0
		}, wantKind: actTalk},
		{name: "spawn in reach", class: classHumanMystic, level: 20, wantKind: actMove},
		{name: "spawn beyond reach", class: classHumanMystic, level: 20, setup: func(b *Brain) { b.persona.ClassChange.Reach = 4999 }},
		{name: "no travel map", class: classHumanMystic, level: 20, setup: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "a party member walks to no spawn", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			b.world.Party.LeaderID = 99
		}},
		{name: "another NPC template", class: classHumanMystic, level: 20, setup: func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: masterObj, NPCID: 30009, Pos: Point{X: 200}}, t0)
			b.cfg.Travel = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newClassBrain(t, tt.class, tt.level)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(at(time.Second))
			if tt.wantKind == actHold && rule == "classchange" {
				t.Fatalf("decide = %q %d, want no visit", rule, a.kind)
			}
			if tt.wantKind != actHold && (rule != "classchange" || a.kind != tt.wantKind) {
				t.Fatalf("decide = %q %d, want %d", rule, a.kind, tt.wantKind)
			}
		})
	}
}

// TestClassChangeBacksOff checks a visit that changes nothing ends and
// leaves the masters alone for RetryAfter, the class kept.
func TestClassChangeBacksOff(t *testing.T) {
	talk := trainerStep{name: "talk", at: time.Second, setup: masterInSight, wantRule: "classchange", wantKind: actTalk}
	menu := trainerStep{name: "menu", at: 2 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass}
	pick := trainerStep{name: "pick", at: 3 * time.Second, events: []Event{menuPage(masterObj, 11, classCleric)}, wantRule: "classchange", wantKind: actBypass}
	tests := []struct {
		name  string
		steps []trainerStep
		end   time.Duration
	}{
		{
			name: "transfer refused in silence",
			steps: []trainerStep{
				talk, menu, pick,
				{name: "still waiting", at: 7 * time.Second, wantRule: "classchange"},
				{name: "unanswered", at: 8 * time.Second},
			},
			end: 8 * time.Second,
		},
		{
			name: "menu without the build's profession",
			steps: []trainerStep{
				talk, menu,
				{name: "no Cleric", at: 3 * time.Second, events: []Event{menuPage(masterObj, 11)}},
			},
			end: 3 * time.Second,
		},
		{
			name: "page with neither link",
			steps: []trainerStep{
				talk, menu,
				{name: "come back later", at: 3 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: masterObj, HTML: "<html><body>Come back at level 40.</body></html>"}}},
			},
			end: 3 * time.Second,
		},
		{
			name: "first page again after the menu link",
			steps: []trainerStep{
				talk, menu,
				{name: "same page", at: 3 * time.Second, events: []Event{masterPage(masterObj)}},
			},
			end: 3 * time.Second,
		},
		{
			name: "master not at its spawn",
			steps: []trainerStep{
				{name: "nobody there", at: time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) }},
			},
			end: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newClassBrain(t, classHumanMystic, 20)
			runTrainerSteps(t, b, tt.steps)
			if b.class.v.active() || b.class.retryAt != at(tt.end).Add(visitShunFor) {
				t.Fatalf("visit %+v, want it ended and backing off until %v", b.class, at(tt.end).Add(visitShunFor))
			}
			if b.world.Self.ClassID != classHumanMystic {
				t.Fatalf("class %d, want it kept", b.world.Self.ClassID)
			}
		})
	}
}

// TestTransferTier checks the level and menu of each class's next transfer
// (classmaster/dialog.go changeLevel).
func TestTransferTier(t *testing.T) {
	tests := []struct {
		class     int32
		wantLevel int32
		wantCmd   string
	}{
		{HumanFighter.ID, 20, firstClassCommand},
		{OrcMystic.ID, 20, firstClassCommand},
		{DwarvenFighter.ID, 20, firstClassCommand},
		{classCleric, 40, secondClassCommand},
		{classArtisan, 40, secondClassCommand},
		{54, 40, secondClassCommand},
		{classBishop, 76, thirdClassCommand},
		{classWarsmith, 76, thirdClassCommand},
	}
	for _, tt := range tests {
		if level, cmd := transferTier(tt.class); level != tt.wantLevel || cmd != tt.wantCmd {
			t.Errorf("transferTier(%d) = %d %q, want %d %q", tt.class, level, cmd, tt.wantLevel, tt.wantCmd)
		}
	}
}

// TestBuildTables checks every build step goes from a class to one of the
// next tier, every starting class has a first step, and every mystic class
// of the support build learns the persona's skills (its trainers come from
// the datapack: TestTrainersShipped).
func TestBuildTables(t *testing.T) {
	support := DefaultPersona()
	for _, c := range StartingClasses() {
		first := compileFor(support, c.ID, 20).Race.Professions[c.ID]
		if !slices.Contains(firstProfessions, first) {
			t.Errorf("starting class %d builds %d, want a first profession", c.ID, first)
		}
		second := compileFor(support, first, 40).Race.Professions[first]
		if second == 0 || slices.Contains(firstProfessions, second) {
			t.Errorf("first profession %d builds %d, want a second profession", first, second)
		}
		for _, cl := range []int32{c.ID, first, second} {
			if k := compileFor(support, cl, 40).Kit; c.Mystic && len(k.Skills) == 0 {
				t.Errorf("mystic class %d: learns nothing", cl)
			}
		}
	}
	crafter := CrafterPersona()
	if crafter.ClassChange.Builds[DwarvenFighter.ID] != classArtisan || crafter.ClassChange.Builds[classArtisan] != classWarsmith {
		t.Errorf("crafter builds %v, want 53 -> 56 -> 57 (D18)", crafter.ClassChange.Builds)
	}
	for _, cl := range []int32{DwarvenFighter.ID, classArtisan, classWarsmith} {
		if !slices.Contains(crafter.LearnSkills[cl], skillCreateItem) || len(crafter.Trainers[cl]) == 0 {
			t.Errorf("crafter class %d: learn %v, want Create Item at the dwarven trainers", cl, crafter.LearnSkills[cl])
		}
	}
	// Each crafter class walks only to trainers whose teachTo lists it
	// (data/xml/npcs): 30298 teaches 53-57, 30057 only 53-55, 31990 53, 56
	// and 118.
	for _, tc := range []struct {
		class   int32
		in, out []int32
	}{
		{DwarvenFighter.ID, []int32{30298, 30057, 31990}, nil},
		{classArtisan, []int32{30298, 31990}, []int32{30057, 30058, 30103, 30104}},
		{classWarsmith, []int32{30298}, []int32{30057, 31990}},
	} {
		for _, id := range tc.in {
			if !slices.Contains(crafter.Trainers[tc.class], id) {
				t.Errorf("crafter class %d trainers lack %d", tc.class, id)
			}
		}
		for _, id := range tc.out {
			if slices.Contains(crafter.Trainers[tc.class], id) {
				t.Errorf("crafter class %d trainers hold %d, which does not teach it", tc.class, id)
			}
		}
	}
	if got := compileFor(support, DwarvenFighter.ID, 20).Race.Professions[DwarvenFighter.ID]; got != 54 {
		t.Errorf("support dwarf builds %d, want 54: the crafter's table is its own", got)
	}
}
