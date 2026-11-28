package botclient

import (
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// fitSustainSkills are the skills of the sustain-step tests, each at level 1:
// the drains the sorcerer, the spellhowler and the phantom ranger cast
// under their own HP gate (SelfHPBelow), and the attacks they kill with.
var fitSustainSkills = map[int32]datapack.Skill{
	skillVampiricTouch: {Type: "DRAIN", OperateType: "ACTIVE", Power: 40, HitTime: 4 * time.Second, ReuseDelay: 12 * time.Second, MPConsume: 30},
	skillVampiricClaw:  {Type: "DRAIN", OperateType: "ACTIVE", Power: 45, HitTime: 4 * time.Second, ReuseDelay: 10 * time.Second, MPConsume: 30},
	skillDrainHealth:   {Type: "DRAIN", OperateType: "ACTIVE", Power: 50, HitTime: 2 * time.Second, ReuseDelay: 13 * time.Second, MPConsume: 29},
	skillProminence:    {Type: "MDAM", OperateType: "ACTIVE", Power: 52, HitTime: 4 * time.Second, ReuseDelay: 6 * time.Second, MPInitialConsume: 7, MPConsume: 28},
	skillHurricane:     {Type: "MDAM", OperateType: "ACTIVE", Power: 52, HitTime: 4 * time.Second, ReuseDelay: 6 * time.Second, MPInitialConsume: 7, MPConsume: 28},
	skillStunningShot:  {Type: "PDAM", OperateType: "ACTIVE", Power: 1200, HitTime: 2 * time.Second, ReuseDelay: 13 * time.Second, MPInitialConsume: 9, MPConsume: 35},
}

// fitSustainNuke and fitSustainShot are Prominence's (or Hurricane's) and
// Stunning Shot's attacks in fitSustainSkills, each cast's MP its initial
// plus its consume.
var (
	fitSustainNuke = charmodel.Attack{Magic: true, Power: 52, HitTime: 4 * time.Second, Reuse: 6 * time.Second, MPCost: 35}
	fitSustainShot = charmodel.Attack{Power: 1200, HitTime: 2 * time.Second, Reuse: 13 * time.Second, MPCost: 44}
)

// fitSustainCases are the rows whose rotation opens with a sustain step and
// the attack each kills with, knowing known (#712): SN's Prominence past
// Vampiric Touch under 60 % HP, SH's Hurricane past Vampiric Claw and
// Touch, PR's Stunning Shot past Drain Health under 60 %; one that knows
// only its sustain step melees.
var fitSustainCases = []struct {
	name  string
	class int32
	known []int32
	want  charmodel.Attack
	skill bool
}{
	{"SN: Prominence, not Vampiric Touch", classSorcerer, []int32{skillVampiricTouch, skillProminence}, fitSustainNuke, true},
	{"SH: Hurricane, not Vampiric Claw or Touch", classSpellhowler, []int32{skillVampiricClaw, skillVampiricTouch, skillHurricane}, fitSustainNuke, true},
	{"PR: Stunning Shot, not Drain Health", classPhantomRanger, []int32{skillDrainHealth, skillStunningShot}, fitSustainShot, true},
	{"SN knowing only Vampiric Touch: melee", classSorcerer, []int32{skillVampiricTouch}, charmodel.Attack{}, false},
	{"PR knowing only Drain Health: melee", classPhantomRanger, []int32{skillDrainHealth}, charmodel.Attack{}, false},
}

// fitSustainBrain is a level 40 Damage bot of class knowing known, its travel
// map holding fitSustainSkills.
func fitSustainBrain(t *testing.T, class int32, known []int32) *Brain {
	t.Helper()
	var ks []KnownSkill
	for _, id := range known {
		ks = append(ks, KnownSkill{ID: id, Level: 1})
	}
	b := newModelBrain(t, class, 40, RoleDamage, ks, nil)
	m := b.cfg.Travel
	m.skills = maps.Clone(m.skills)
	for id, sk := range fitSustainSkills {
		m.skills[datapack.SkillRef{ID: id, Level: 1}] = sk
	}
	return b
}

// TestSpotAttackSkipsSustainSteps checks that the solo spot fit
// (spotAttack) times the attack the gear model does (gearRole, #676): a
// step gated on the bot's own HP is passed over, so the fit times
// Prominence, Hurricane and Stunning Shot, and a bot knowing only its
// sustain step is priced by its melee (#712).
func TestSpotAttackSkipsSustainSteps(t *testing.T) {
	for _, tt := range fitSustainCases {
		t.Run(tt.name, func(t *testing.T) {
			b := fitSustainBrain(t, tt.class, tt.known)
			bd := b.build()
			a, ok, priced := b.spotAttack(&bd)
			if a != tt.want || ok != tt.skill || !priced {
				t.Errorf("spotAttack = %+v, %v, %v; want %+v, %v, priced", a, ok, priced, tt.want, tt.skill)
			}
			if role, _ := b.gearRole(); role.Attack != a {
				t.Errorf("gearRole attack %+v, spotAttack %+v; want one attack", role.Attack, a)
			}
		})
	}
}

// TestSpotAttackSustainUnpriced checks spotAttack without a travel map (as
// TestLiveFarmMage's brain): a known attack step leaves the fit unpriced,
// to the level, but a sustain step alone does not, as the bot melees until
// hurt.
func TestSpotAttackSustainUnpriced(t *testing.T) {
	tests := []struct {
		name   string
		known  []int32
		priced bool
	}{
		{"Vampiric Touch and Prominence: unpriced", []int32{skillVampiricTouch, skillProminence}, false},
		{"only Vampiric Touch: melee, priced", []int32{skillVampiricTouch}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := fitSustainBrain(t, classSorcerer, tt.known)
			b.cfg.Travel = nil
			bd := b.build()
			if a, ok, priced := b.spotAttack(&bd); ok || priced != tt.priced {
				t.Errorf("spotAttack = %+v, %v, %v; want no attack, priced %v", a, ok, priced, tt.priced)
			}
		})
	}
}

// TestMemberModelSkipsSustainSteps checks that a leader's party fit models
// a level 40 Damage member by the attack the solo fit and the gear model
// time (#712): its class tree's grants by its level, past any sustain step.
func TestMemberModelSkipsSustainSteps(t *testing.T) {
	for _, tt := range fitSustainCases {
		t.Run(tt.name, func(t *testing.T) {
			b := fitSustainBrain(t, classHumanMystic, nil)
			tmpl := modelClass(tt.class, -1, 0, 40, 43, 30, 21, 11, 25, 4, 80, 500)
			for _, id := range tt.known {
				tmpl.Skills = append(tmpl.Skills, datapack.SkillGrant{ID: id, Level: 1, MinLevel: 40})
			}
			b.cfg.Travel.classes = map[int32]datapack.Class{tt.class: tmpl}
			mm, ok := b.memberModel(memberKey{class: tt.class, level: 40})
			if !ok || len(mm.heals) > 0 || mm.attack != tt.want || mm.skill != tt.skill {
				t.Errorf("memberModel = %+v, %v; want the attack %+v, skill %v", mm, ok, tt.want, tt.skill)
			}
		})
	}
}
