package botclient

import (
	"testing"
	"time"
)

// TestRestUnderMPReserve checks rule "rest" for a caster: out of combat
// under MPReserve it sits, since fight would cast nothing there, and it
// stands only once MP is back above both StandMP and MPReserve. A bot that
// knows none of its AttackSkills keeps SitMP: no rule fires.
func TestRestUnderMPReserve(t *testing.T) {
	now := at(time.Second)
	knows := func(ids ...int32) func(*Brain) {
		return func(b *Brain) {
			var skills []KnownSkill
			for _, id := range ids {
				skills = append(skills, KnownSkill{ID: id, Level: 1})
			}
			b.world.Apply(SkillList{Skills: skills}, t0)
		}
	}
	tests := []struct {
		name     string
		persona  func(*Persona)
		skills   []int32
		mp       int32
		sitting  bool
		resting  bool
		wantRule string
		wantKind actionKind
	}{
		{name: "mystic at 40% MP sits", skills: []int32{skillWindStrike}, mp: 40, wantRule: "rest", wantKind: actSitStand},
		{name: "orc mystic at 40% MP sits", skills: []int32{skillChillFlame}, mp: 40, wantRule: "rest", wantKind: actSitStand},
		{name: "fighter at 40% MP does not sit", skills: []int32{3}, mp: 40, wantRule: ""},
		{name: "mystic at the reserve does not sit", skills: []int32{skillWindStrike}, mp: 50, wantRule: ""},
		{name: "mystic without a reserve keeps SitMP", persona: func(p *Persona) { p.MPReserve = 0 }, skills: []int32{skillWindStrike}, mp: 40, wantRule: ""},
		{
			name: "resting mystic stays seated above the reserve until StandMP", skills: []int32{skillWindStrike},
			mp: 60, sitting: true, resting: true, wantRule: "rest",
		},
		{
			name: "reserve above StandMP holds the stand", persona: func(p *Persona) { p.MPReserve, p.StandMP = 80, 70 },
			skills: []int32{skillWindStrike}, mp: 75, sitting: true, resting: true, wantRule: "rest",
		},
		{
			name: "recovered mystic stands", skills: []int32{skillWindStrike},
			mp: 95, sitting: true, resting: true, wantRule: "rest", wantKind: actSitStand,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			if tc.persona != nil {
				tc.persona(&p)
			}
			b, _ := newTestBrain(t, p)
			knows(tc.skills...)(b)
			b.world.Self.MP, b.world.Self.Sitting, b.resting = tc.mp, tc.sitting, tc.resting
			rule, a := b.decide(now)
			if rule != tc.wantRule || a.kind != tc.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tc.wantRule, tc.wantKind)
			}
		})
	}
}
