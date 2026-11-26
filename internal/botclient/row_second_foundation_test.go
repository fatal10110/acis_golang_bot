package botclient

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// lastClassID is past the last class id of the profession table (118, the
// third professions' last).
const lastClassID = 200

// neverRows are the second professions that never get a row of their own
// (epic #624): the servitor lines Dark Avenger, Necromancer, Warlock,
// Elemental Summoner and Phantom Summoner (#592), the Overlord (D23) and the
// Warsmith (the M9 crafter line).
var neverRows = []int32{6, 13, 14, 28, 41, 51, 57}

// TestSecondProfessionFallback checks every second profession without a row
// of its own (no professionKits entry) plays its first profession's row: at
// level 45, in both roles, the same rotation and thresholds. The classes of
// neverRows are always among them.
func TestSecondProfessionFallback(t *testing.T) {
	var fallback []int32
	for id := range int32(lastClassID) {
		if _, kit := professionKits[id]; kit {
			continue
		}
		if start, _, ok := rowSpan(id); ok && start == 40 {
			fallback = append(fallback, id)
		}
	}
	for _, id := range neverRows {
		if !slices.Contains(fallback, id) {
			t.Errorf("class %d has a row of its own or is no second profession, want it playing its first profession's", id)
		}
	}
	for _, class := range fallback {
		first, _ := datapack.ParentClass(class)
		race, _ := datapack.ClassRace(class)
		for _, role := range []Role{RoleDamage, RoleSupport} {
			t.Run(fmt.Sprintf("class %d %v", class, role), func(t *testing.T) {
				bd, want := CompileBuild(race, class, 45, role, BranchChoice{}), CompileBuild(race, first, 45, role, BranchChoice{})
				if got, wantRot := stepSkills(bd.Kit.Rotation), stepSkills(want.Kit.Rotation); !slices.Equal(got, wantRot) {
					t.Errorf("rotation %v, want class %d's %v", got, first, wantRot)
				}
				if bd.Thresholds != want.Thresholds {
					t.Errorf("thresholds %+v, want class %d's %+v", bd.Thresholds, first, want.Thresholds)
				}
			})
		}
	}
}

// TestRowSpan checks the row levels of a base class, a first, a second and
// a third profession, and of an id the profession table does not hold.
func TestRowSpan(t *testing.T) {
	for _, tt := range []struct {
		class      int32
		start, end int32
		ok         bool
	}{
		{HumanFighter.ID, 1, 19, true},
		{classWarrior, 20, 39, true},
		{classGladiator, 40, 75, true},
		{88, 0, 0, false},
		{lastClassID, 0, 0, false},
	} {
		if start, end, ok := rowSpan(tt.class); start != tt.start || end != tt.end || ok != tt.ok {
			t.Errorf("class %d: rowSpan %d %d %v, want %d %d %v", tt.class, start, end, ok, tt.start, tt.end, tt.ok)
		}
	}
}

// TestCritSustainSecond checks ruleCritical casts each second profession's
// heal or drain of critSustain (owner decision 5 on #624) under its
// SelfHPBelow gate before a potion, on the target or the bot as the step
// says, and drinks with it cooling; a step of a skill outside critSustain
// leaves it to the potion.
func TestCritSustainSecond(t *testing.T) {
	now := at(10 * time.Second)
	for _, tt := range []struct {
		name     string
		skill    int32
		target   StepTarget
		object   int32
		sustains bool
	}{
		{"Life Leech", skillLifeLeech, TargetEnemy, npcID, true},
		{"Steal Essence", skillStealEssence, TargetEnemy, npcID, true},
		{"Vampiric Claw", skillVampiricClaw, TargetEnemy, npcID, true},
		{"Revival", skillRevival, TargetSelf, selfID, true},
		{"not a sustain skill", skillPowerStrike, TargetEnemy, npcID, false},
	} {
		for _, cooling := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s cooling %v", tt.name, cooling), func(t *testing.T) {
				b := kitBrain(t, ClassKit{Rotation: []Step{{Skill: tt.skill, Target: tt.target, SelfHPBelow: 30}}}, 100, tt.skill)
				b.world.Apply(ItemList{Items: []Item{potionsOf(20)}}, t0)
				b.world.Self.HP = 15
				b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
				if cooling {
					b.world.holdSkill(tt.skill, now.Add(time.Minute))
				}
				rule, a := b.decide(now)
				if rule != "critical" {
					t.Fatalf("decide = %q kind %d skill %d, want critical", rule, a.kind, a.skill)
				}
				if tt.sustains && !cooling {
					wantCast(t, a, tt.skill, tt.object)
				} else if a.kind != actUseItem {
					t.Fatalf("decide = kind %d skill %d, want a potion", a.kind, a.skill)
				}
			})
		}
	}
}
