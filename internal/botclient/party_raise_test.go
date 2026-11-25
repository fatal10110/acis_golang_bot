package botclient

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// raiseParty links b under slot and puts it, out of combat, in leader's
// party with pcID and rowPC2 beside it; dead are the members that lie dead.
func raiseParty(b *Brain, slot Command, leader int32, dead ...int32) {
	linkBrain(b, standing{}).party = slot
	b.world.Self.InCombat = false
	b.world.Apply(CharInfo{ObjectID: pcID, Name: "other", Pos: Point{Y: 300}}, t0)
	b.world.Apply(CharInfo{ObjectID: rowPC2, Name: "third", Pos: Point{X: -300}}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: leader, Members: []PartyMember{
		{ObjectID: pcID, Name: "other", HP: 100, MaxHP: 100, ClassID: HumanFighter.ID},
		{ObjectID: rowPC2, Name: "third", HP: 100, MaxHP: 100, ClassID: classHumanMystic},
	}}, t0)
	for _, id := range dead {
		b.world.Apply(Die{ObjectID: id}, t0)
	}
}

// TestPartyRaise checks the raise rule (docs/parties/members.md, "Events"):
// a level-20 Cleric member out of combat, the leader dead in sight, selects
// it and casts Resurrection, the leader first; not in combat, not outside
// its bot party, not on reuse, not under SitMP, not a flagged member
// unless the PvP policy supports one; a cast in flight is waited
// out; a level-19 Elven Mystic never.
func TestPartyRaise(t *testing.T) {
	now := at(time.Second)
	join := Command{Kind: CommandJoin, Arg: "other"}
	for _, tt := range []struct {
		name   string
		class  int32
		level  int32
		skills []int32
		slot   *Command // nil: join
		leader int32
		dead   []int32
		setup  func(b *Brain)
		want   actionKind // actHold: no match
		object int32
		hold   bool // matched with nothing to send
	}{
		{name: "the leader dead: selects it", dead: []int32{pcID}, want: actSelect, object: pcID},
		{name: "the leader dead, selected: casts", dead: []int32{pcID}, setup: target(pcID), want: actCast, object: pcID},
		{name: "the leader first", dead: []int32{rowPC2, pcID}, setup: target(rowPC2), want: actSelect, object: pcID},
		{name: "another member", dead: []int32{rowPC2}, setup: target(rowPC2), want: actCast, object: rowPC2},
		{
			name: "a lead leader raises a member", slot: &Command{Kind: CommandLead}, leader: selfID,
			dead: []int32{rowPC2}, setup: target(rowPC2), want: actCast, object: rowPC2,
		},
		{name: "sitting: stands first", dead: []int32{pcID}, setup: func(b *Brain) { b.world.Self.Sitting = true }, want: actSitStand},
		{name: "none dead", setup: target(pcID)},
		{name: "in combat", dead: []int32{pcID}, setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "out of sight", dead: []int32{pcID}, setup: func(b *Brain) { delete(b.world.Objects, pcID) }},
		{name: "a flagged leader", dead: []int32{pcID}, setup: func(b *Brain) { b.world.Objects[pcID].PvPFlag = 1 }},
		{
			name: "a flagged leader, SupportFlagged: selects it", dead: []int32{pcID},
			setup: func(b *Brain) {
				b.world.Objects[pcID].PvPFlag = 1
				b.persona.PvP.SupportFlagged = true
			},
			want: actSelect, object: pcID,
		},
		{name: "a player's party", slot: &Command{}, dead: []int32{pcID}},
		{name: "another leader's party", slot: &Command{Kind: CommandJoin, Arg: "someone"}, dead: []int32{pcID}},
		{
			name: "on reuse", dead: []int32{pcID},
			setup: func(b *Brain) { b.world.skillReady[skillResurrection] = now.Add(time.Minute) },
		},
		{name: "MP under SitMP", dead: []int32{pcID}, setup: func(b *Brain) { b.world.Self.MP = 20 }},
		{name: "a cast in flight", dead: []int32{pcID}, setup: func(b *Brain) { b.castAt = now }, hold: true},
		{name: "without Resurrection", skills: []int32{}, dead: []int32{pcID}},
		{name: "a level-19 Elven Mystic", class: classElvenMystic, level: 19, skills: []int32{}, dead: []int32{pcID}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			class, level, skills, slot, leader := classCleric, int32(20), []int32{skillResurrection}, join, int32(pcID)
			if tt.class != 0 {
				class, level = tt.class, tt.level
			}
			if tt.skills != nil {
				skills = tt.skills
			}
			if tt.slot != nil {
				slot = *tt.slot
			}
			if tt.leader != 0 {
				leader = tt.leader
			}
			b := mysticBrain(t, class, level, RoleSupport, "", skills...)
			raiseParty(b, slot, leader, tt.dead...)
			if tt.setup != nil {
				tt.setup(b)
			}
			a, ok := b.ruleRaise(now)
			switch {
			case tt.want == actCast:
				if !ok {
					t.Fatal("ruleRaise: no raise")
				}
				wantCast(t, a, skillResurrection, tt.object)
			case tt.want != actHold:
				if !ok || a.kind != tt.want || a.object != tt.object {
					t.Fatalf("ruleRaise = kind %d object %#x, %v; want kind %d object %#x", a.kind, a.object, ok, tt.want, tt.object)
				}
			case ok != tt.hold || a.kind != 0:
				t.Fatalf("ruleRaise = kind %d, %v; want nothing sent, matched %v", a.kind, ok, tt.hold)
			}
		})
	}
}

// target selects id.
func target(id int32) func(b *Brain) {
	return func(b *Brain) { b.world.Target = id }
}

// TestPartyRaiseLearned checks the learn tables: the Cleric, Elven Oracle
// and Shillien Oracle rows learn Resurrection at 20, the base mystic rows
// never, and the raise rule runs right after the heal: a dead leader is
// selected ahead of every later rule.
func TestPartyRaiseLearned(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int32
		level int32
		want  bool
	}{
		{"CL", classCleric, 20, true},
		{"EO", classElvenOracle, 20, true},
		{"SO", classShillienOracle, 20, true},
		{"HM", classHumanMystic, 19, false},
		{"EM", classElvenMystic, 19, false},
		{"DM", classDarkMystic, 19, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := mysticBrain(t, tt.class, tt.level, RoleSupport, "")
			if got := slices.Contains(b.build().Kit.Skills, skillResurrection); got != tt.want {
				t.Errorf("learns Resurrection %v, want %v", got, tt.want)
			}
		})
	}
	b := mysticBrain(t, classCleric, 20, RoleSupport, "", skillResurrection)
	raiseParty(b, Command{Kind: CommandJoin, Arg: "other"}, pcID, pcID)
	if rule, a := b.decide(at(time.Second)); rule != "raise" || a.kind != actSelect || a.object != pcID {
		t.Errorf("decide = %q kind %d object %#x, want raise selecting the dead leader", rule, a.kind, a.object)
	}
	heal := slices.IndexFunc(playRules, func(r rule) bool { return r.name == "heal" })
	if heal < 0 || heal+1 >= len(playRules) || playRules[heal+1].name != "raise" {
		t.Error(`"raise" does not run right after "heal"`)
	}
}

// TestPartyRaiseShipped checks Resurrection against the shipped datapack
// when BOT_DATAPACK_XML names its data/xml directory, and skips otherwise:
// the Cleric, Elven Oracle and Shillien Oracle learn it first at 20, the
// Elven Mystic never (classes/elvenMystic.xml:266-267,362), and it targets
// a player's corpse.
func TestPartyRaiseShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	classes, err := datapack.LoadClasses(filepath.Join(dir, "classes"))
	if err != nil {
		t.Fatal(err)
	}
	skills, err := datapack.LoadSkills(filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		class int32
		want  int32 // 0: never
	}{
		{"CL", classCleric, 20},
		{"EO", classElvenOracle, 20},
		{"SO", classShillienOracle, 20},
		{"EM", classElvenMystic, 0},
	} {
		lvl, ok := firstMinLevel(classes[tt.class], skillResurrection)
		if !ok {
			lvl = 0
		}
		if lvl != tt.want {
			t.Errorf("%s learns Resurrection first at %d, want %d", tt.name, lvl, tt.want)
		}
	}
	if sk := skills[datapack.SkillRef{ID: skillResurrection, Level: 1}]; sk.Target != "CORPSE_PLAYER" {
		t.Errorf("Resurrection targets %q, want CORPSE_PLAYER", sk.Target)
	}
}
