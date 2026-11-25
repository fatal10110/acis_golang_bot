package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestGearRole checks what the model values a bot's gear for (gearRole): a
// Support build that knows a heal, its heals a minute; a Support build without
// one, a tank; a Damage build, its first known damage rotation skill, else
// its melee hits.
func TestGearRole(t *testing.T) {
	heal := []KnownSkill{{ID: skillHeal, Level: 1}}
	tests := []struct {
		name   string
		class  int32
		role   Role
		skills []KnownSkill
		want   charmodel.Role
		tank   bool
	}{
		{
			"support with a heal", classHumanMystic, RoleSupport, heal,
			charmodel.Role{
				Kind: charmodel.Support, RestTime: charmodel.DefaultRestTime, SurvivalFloor: charmodel.DefaultSurvivalFloor,
				HealPower: 49, HealMP: 10, HealHitTime: 5 * time.Second, HealReuse: 10 * time.Second, HealMAtk: true,
			},
			false,
		},
		{
			"support without a heal is a tank", classHumanMystic, RoleSupport, nil,
			charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime},
			true,
		},
		{
			"damage with a magic rotation skill", classHumanMystic, RoleDamage,
			[]KnownSkill{{ID: skillWindStrike, Level: 1}},
			charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime, Attack: charmodel.Attack{Magic: true, Power: 12, HitTime: modelSkills[datapack.SkillRef{ID: skillWindStrike, Level: 1}].HitTime, Reuse: modelSkills[datapack.SkillRef{ID: skillWindStrike, Level: 1}].ReuseDelay, MPCost: 10}},
			false,
		},
		{
			"damage without a known rotation skill hits in melee", classHumanMystic, RoleDamage, heal,
			charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime},
			false,
		},
		{
			"fighter hits in melee", 0, RoleDamage, nil,
			charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newModelBrain(t, tt.class, 20, tt.role, tt.skills, nil)
			role, tank := b.gearRole()
			if role != tt.want || tank != tt.tank {
				t.Errorf("gearRole = %+v, tank %v; want %+v, tank %v", role, tank, tt.want, tt.tank)
			}
		})
	}
}

// TestGearWorth checks worth's branches: a tank's time to die; a winning
// fight's charmodel.Value; a losing damage role's time to kill over its
// time to die and a losing support's survival floor over it, negated, so a
// stronger profile still ranks higher.
func TestGearWorth(t *testing.T) {
	melee := charmodel.Role{Kind: charmodel.Damage, RestTime: charmodel.DefaultRestTime}
	support := charmodel.Role{Kind: charmodel.Support, RestTime: charmodel.DefaultRestTime, SurvivalFloor: charmodel.DefaultSurvivalFloor, HealPower: 49, HealMP: 10, HealHitTime: 5 * time.Second, HealReuse: 10 * time.Second, HealMAtk: true}
	weak := charmodel.Profile{MaxHP: 30, MaxMP: 40, PAtk: 5, PAtkSpd: 250, MAtkSpd: 200, PDef: 20, MDef: 20, Accuracy: 30, Evasion: 30, RunSpeed: 100}
	stronger := weak
	stronger.PAtk, stronger.PDef = 10, 40
	strong := charmodel.Profile{MaxHP: 5000, MaxMP: 2000, PAtk: 900, PAtkSpd: 400, MAtkSpd: 400, PDef: 900, MDef: 900, Accuracy: 90, Evasion: 90, RunSpeed: 120, Critical: 40}
	ttd := func(p charmodel.Profile) float64 { return charmodel.TimeToDie(p, modelTarget).Seconds() }
	tests := []struct {
		name   string
		v      gearModel
		p, q   charmodel.Profile // q ranks above p
		wantP  float64
		losing bool
	}{
		{"tank lasts", gearModel{role: melee, tank: true}, weak, stronger, ttd(weak), false},
		{"winning fight is its value", gearModel{role: melee}, weak, strong, -1, false},
		{
			"losing damage role",
			gearModel{role: melee},
			weak, stronger,
			-charmodel.TimeToKill(weak, melee.Attack, modelTarget).Seconds() / ttd(weak), true,
		},
		{"losing support", gearModel{role: support}, weak, stronger, -support.SurvivalFloor.Seconds() / ttd(weak), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.v.target = modelTarget
			p, q := tt.v.worth(tt.p), tt.v.worth(tt.q)
			if tt.wantP != -1 && p != tt.wantP {
				t.Errorf("worth(p) = %v, want %v", p, tt.wantP)
			}
			if tt.losing && charmodel.Value(tt.p, tt.v.role, modelTarget) > 0 {
				t.Fatalf("fixture p wins: Value %v", charmodel.Value(tt.p, tt.v.role, modelTarget))
			}
			if tt.losing && p >= 0 {
				t.Errorf("losing worth(p) = %v, want below 0", p)
			}
			if tt.wantP == -1 {
				if want := charmodel.Value(tt.q, tt.v.role, modelTarget); want <= 0 || q != want {
					t.Errorf("winning worth(q) = %v, want charmodel.Value %v > 0", q, want)
				}
			}
			if q <= p {
				t.Errorf("worth(q) = %v, want above worth(p) = %v", q, p)
			}
		})
	}
}

// TestIndexRefMobs checks the reference monster of each level: of its
// Monster templates with HP, the median by modeled max HP, ties to the
// lower template id; other types, HP 0 and level 0 are left out.
func TestIndexRefMobs(t *testing.T) {
	mob := func(id int32, typ string, level int, hp, pAtk float64) datapack.NPC {
		return datapack.NPC{ID: id, Type: typ, Level: level, Combat: datapack.Combat{HP: hp, PAtk: pAtk, AtkSpd: 250}}
	}
	npcs := map[int32]datapack.NPC{
		1: mob(1, "Monster", 10, 100, 1),
		2: mob(2, "Monster", 10, 300, 2),
		3: mob(3, "Monster", 10, 200, 3),
		4: mob(4, "Guard", 10, 250, 4),
		5: mob(5, "Monster", 10, 0, 5),
		6: mob(6, "Monster", 0, 250, 6),
		// Level 20: three of the same HP, the median by id is 12.
		13: mob(13, "Monster", 20, 400, 13),
		11: mob(11, "Monster", 20, 400, 11),
		12: mob(12, "Monster", 20, 400, 12),
	}
	got := indexRefMobs(npcs)
	want := map[int]charmodel.Profile{10: charmodel.Monster(npcs[3]), 20: charmodel.Monster(npcs[12])}
	if len(got) != len(want) {
		t.Fatalf("indexRefMobs levels = %d, want %d: %+v", len(got), len(want), got)
	}
	for lvl, p := range want {
		if got[lvl] != p {
			t.Errorf("level %d: %+v, want %+v", lvl, got[lvl], p)
		}
	}
}

// TestRefMob checks the reference monster a level takes: its own, else
// the nearest level below with one, else the nearest above; none from an
// empty index.
func TestRefMob(t *testing.T) {
	p := func(hp float64) charmodel.Profile { return charmodel.Profile{MaxHP: hp} }
	m := &TravelMap{refMobs: map[int]charmodel.Profile{10: p(10), 20: p(20), 30: p(30)}}
	tests := []struct {
		level int
		want  float64
	}{{20, 20}, {25, 20}, {29, 20}, {31, 30}, {5, 10}, {9, 10}, {10, 10}}
	for _, tt := range tests {
		if got, ok := m.refMob(tt.level); !ok || got.MaxHP != tt.want {
			t.Errorf("refMob(%d) = %v, %v; want the level %v monster", tt.level, got.MaxHP, ok, tt.want)
		}
	}
	if _, ok := (&TravelMap{}).refMob(20); ok {
		t.Error("refMob on an empty index ok")
	}
}
