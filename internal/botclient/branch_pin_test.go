package botclient

import (
	"slices"
	"testing"
)

// TestBranchPins checks the branches a base class may be pinned to are the
// first professions every role's table names for it (D22, D23: no servitor
// line, no Overlord), and that BranchBase finds each one's base class.
func TestBranchPins(t *testing.T) {
	tests := []struct {
		base int32
		want []int32
	}{
		{HumanFighter.ID, []int32{1, 4, 7}},
		{classHumanMystic, []int32{11, 15}},
		{ElvenFighter.ID, []int32{19, 22}},
		{classElvenMystic, []int32{26, 29}},
		{DarkFighter.ID, []int32{32, 35}},
		{classDarkMystic, []int32{39, 42}},
		{OrcFighter.ID, []int32{45, 47}},
		{classOrcMystic, []int32{50}},
		{DwarvenFighter.ID, []int32{54, 56}},
		{classKnight, nil},
	}
	for _, tt := range tests {
		got := BranchPins(tt.base)
		if !slices.Equal(got, tt.want) {
			t.Errorf("BranchPins(%d) = %v, want %v", tt.base, got, tt.want)
		}
		for _, pin := range got {
			if base, ok := BranchBase(pin); !ok || base != tt.base {
				t.Errorf("BranchBase(%d) = %d, %v, want %d", pin, base, ok, tt.base)
			}
		}
	}
	for _, c := range []int32{0, 2, 14, 51, 88, 500} {
		if base, ok := BranchBase(c); ok {
			t.Errorf("BranchBase(%d) = %d, want no base: not a pinnable branch", c, base)
		}
	}
}

// TestBrainStatusBuild checks the snapshot carries the build the brain
// plays: its role, the pinned branch while a base class, and its kit's
// skills; a persona with no role publishes none.
func TestBrainStatusBuild(t *testing.T) {
	p := DefaultPersona()
	p.Role = RoleDamage
	b, _ := newTestBrain(t, p)
	b.cfg.account, b.cfg.Branch, b.builtOK = "acct1", classRogue, false
	b.world.Apply(UserInfo{ObjectID: selfID, Name: "bot7", ClassID: HumanFighter.ID, Level: 20, RunSpeed: 100, WalkSpeed: 50}, t0)
	linkBrain(b, standing{})
	b.publishStatus(t0, true)
	got, want := b.Status().Build, b.build()
	if got == nil || got.Role != RoleDamage || got.Branch != classRogue || !slices.Equal(got.Skills, want.Kit.Skills) ||
		!slices.Equal(got.Rotation, stepSkills(want.Kit.Rotation)) || len(got.Skills) == 0 {
		t.Fatalf("published build %+v, want damage heading for Rogue with the kit %v", got, want.Kit.Skills)
	}

	p.Role = RoleNone
	n, _ := newTestBrain(t, p)
	linkBrain(n, standing{})
	n.publishStatus(t0, true)
	if got := n.Status().Build; got != nil {
		t.Fatalf("no-role brain published build %+v, want none", got)
	}
}
