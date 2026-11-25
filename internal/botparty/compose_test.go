package botparty

import (
	"reflect"
	"testing"
	"time"
)

// formed plans one pass from an empty state and returns the live parties.
func formed(cs []Candidate, p Policy) (map[string][]string, []Order) {
	st, os := Plan(State{}, cs, p, t0)
	return parties(st), os
}

func TestPlanLeaderIsDamage(t *testing.T) {
	cs := []Candidate{farmer("Fa", 18, 50), healer("Heal", 19, 100), support("Sup", 19, 200), farmer("Fb", 20, 0)}
	got, os := formed(cs, DefaultPolicy())
	want := map[string][]string{"Fb": {"Heal", "Sup"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parties %v, want %v", got, want)
	}
	if got, want := strs(os), []string{"heal:join Fb Sup", "sup:join Fb Heal"}; !reflect.DeepEqual(got, want) {
		t.Errorf("orders %q, want %q", got, want)
	}
}

func TestPlanNeedsHealer(t *testing.T) {
	shaman := func(level int32, heals bool) Candidate {
		c := cand("Sham", PresetSupport, classOrcShaman, level, 100)
		c.Heals = heals
		return c
	}
	mystic := func(level int32, heals bool) Candidate {
		c := healer("Myst", level, 100)
		c.Heals = heals
		return c
	}
	tests := []struct {
		name string
		sup  Candidate
		fLvl int32
		want bool
	}{
		{"orc mystic", support("Orc", 10, 100), 10, false},
		{"mystic 6", mystic(6, true), 6, false},
		{"mystic 7", mystic(7, true), 7, true},
		{"mystic 7 no heals", mystic(7, false), 7, false},
		{"orc shaman 20", shaman(20, true), 20, true},
		{"orc shaman 20 no heals", shaman(20, false), 20, false},
		{"cleric 20", cand("Cler", PresetSupport, classCleric, 20, 100), 20, true},
		{"elven oracle 20", cand("Orac", PresetSupport, classElvenOracle, 20, 100), 20, true},
		{"shillien oracle 20", cand("Shil", PresetSupport, classShillienOracle, 20, 100), 20, true},
		{"healer set farmer", cand("Myst", PresetFarmer, classHumanMystic, 10, 100), 10, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := formed([]Candidate{farmer("Farm", tc.fLvl, 0), tc.sup}, DefaultPolicy())
			if (len(got) == 1) != tc.want {
				t.Errorf("parties %v, want a party %v", got, tc.want)
			}
		})
	}

	t.Run("healer heals unset", func(t *testing.T) {
		st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100))
		find(cs, "heal").Heals = false
		next, _ := Plan(st, cs, DefaultPolicy(), t0)
		if got := partyByID(next, 1).NoHealerSince; !got.Equal(t0) {
			t.Errorf("NoHealerSince %v, want %v", got, t0)
		}
	})
}

func TestPlanBand(t *testing.T) {
	tests := []struct {
		name string
		cs   []Candidate
		want map[string][]string
	}{
		{"healer 4 under", []Candidate{farmer("Farm", 20, 0), healer("Heal", 16, 100)}, map[string][]string{}},
		{
			"healer 3 under",
			[]Candidate{farmer("Farm", 20, 0), healer("Heal", 17, 100)},
			map[string][]string{"Farm": {"Heal"}},
		},
		{
			"third 23 out",
			[]Candidate{farmer("Farm", 20, 0), healer("Heal", 17, 100), support("Sup", 23, 200)},
			map[string][]string{"Farm": {"Heal"}},
		},
		{
			"third 20 in",
			[]Candidate{farmer("Farm", 20, 0), healer("Heal", 17, 100), support("Sup", 20, 200)},
			map[string][]string{"Farm": {"Heal", "Sup"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := formed(tc.cs, DefaultPolicy()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parties %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanThirdMember(t *testing.T) {
	f1 := farmer("Fone", 20, 0)
	h1 := healer("Hone", 19, 100)
	s := support("Sup", 18, 500)
	f2 := farmer("Ftwo", 19, 200)
	h2 := healer("Htwo", 19, 300)
	s2 := support("Stwo", 18, 400)
	one := DefaultPolicy()
	one.FormPerPass = 1
	tests := []struct {
		name string
		cs   []Candidate
		p    Policy
		want map[string][]string
	}{
		{
			"support before a nearer farmer",
			[]Candidate{f1, h1, s, f2},
			DefaultPolicy(),
			map[string][]string{"Fone": {"Hone", "Sup"}},
		},
		{
			"healerless farmer third",
			[]Candidate{f1, h1, f2},
			DefaultPolicy(),
			map[string][]string{"Fone": {"Hone", "Ftwo"}},
		},
		{
			"second healer leads",
			[]Candidate{f1, h1, f2, h2},
			DefaultPolicy(),
			map[string][]string{"Fone": {"Hone"}, "Ftwo": {"Htwo"}},
		},
		{
			"nearer support",
			[]Candidate{f1, h1, s, s2},
			DefaultPolicy(),
			map[string][]string{"Fone": {"Hone", "Stwo"}},
		},
		{
			"waiting group no third",
			[]Candidate{f1, h1, f2, h2},
			one,
			map[string][]string{"Fone": {"Hone"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := formed(tc.cs, tc.p); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parties %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanSameTown(t *testing.T) {
	other := healer("Heal", 20, 100)
	b := townB
	other.Town = &b
	tests := []struct {
		name string
		h    Candidate
		want int
	}{
		{"other town", other, 0},
		{"beyond MaxTravel", healer("Heal", 20, 20001), 0},
		{"at MaxTravel", healer("Heal", 20, 20000), 1},
		{"no town", func() Candidate { c := healer("Heal", 20, 100); c.Town = nil; return c }(), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := formed([]Candidate{farmer("Farm", 20, 0), tc.h}, DefaultPolicy()); len(got) != tc.want {
				t.Errorf("parties %v, want %d", got, tc.want)
			}
		})
	}
}

func TestPlanFormPerPass(t *testing.T) {
	var cs []Candidate
	for i := range int32(6) {
		cs = append(cs, farmer("F"+string(rune('a'+i)), 20-i/2, i*3000), healer("H"+string(rune('a'+i)), 20-i/2, i*3000+100))
	}
	st := State{}
	for pass, want := range []int{2, 4, 6, 6} {
		var os []Order
		st, os = Plan(st, cs, DefaultPolicy(), t0.Add(time.Duration(pass)*30*time.Second))
		if got := len(parties(st)); got != want {
			t.Fatalf("pass %d: %d parties %v, want %d", pass, got, parties(st), want)
		}
		answer(cs, os, AnswerOK)
	}
	if got := parties(st); len(got["Fa"]) != 1 || got["Fa"][0] != "Ha" {
		t.Errorf("the highest farmer formed %v, want with Ha", got)
	}
}

func TestPlanFillsSlot(t *testing.T) {
	four := DefaultPolicy()
	four.Size = 4
	tests := []struct {
		name  string
		p     Policy
		steps []step
	}{
		{"one per pass", four, []step{
			{
				want: []string{"heal:join Farm Sone", "sone:join Farm Heal"}, deliver: true,
				parties: map[string][]string{"Farm": {"Heal", "Sone"}},
			},
			{
				at: 30 * time.Second, want: []string{"heal:join Farm Sone Stwo", "sone:join Farm Heal Stwo", "stwo:join Farm Heal Sone"},
				parties: map[string][]string{"Farm": {"Heal", "Sone", "Stwo"}}, deliver: true,
			},
			{at: time.Minute, want: []string{"farm:lead Heal Sone Stwo"}},
		}},
		{"full at size", DefaultPolicy(), []step{
			{
				want: []string{"heal:join Farm Sone", "sone:join Farm Heal"}, deliver: true,
				parties: map[string][]string{"Farm": {"Heal", "Sone"}},
			},
			{
				at: 30 * time.Second, want: []string{"farm:lead Heal Sone"},
				parties: map[string][]string{"Farm": {"Heal", "Sone"}},
			},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, cs := established(farmer("Farm", 20, 0), healer("Heal", 20, 100))
			cs = append(cs, support("Sone", 20, 200), support("Stwo", 20, 300))
			run(t, st, cs, tc.p, tc.steps)
		})
	}
}
