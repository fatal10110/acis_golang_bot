package botclient

import "testing"

// TestStatusHeals checks Status.Heals for every healer row, with and
// without its heal in the skill list, and for rows that never heal a party
// (docs/parties/composition.md, "Healer").
func TestStatusHeals(t *testing.T) {
	tests := []struct {
		name   string
		class  int32
		skills []int32
		want   bool
	}{
		{"human mystic with Heal", classHumanMystic, []int32{skillHeal}, true},
		{"human mystic before Heal", classHumanMystic, []int32{skillWindStrike}, false},
		{"elven mystic with Heal", classElvenMystic, []int32{skillHeal}, true},
		{"elven mystic before Heal", classElvenMystic, nil, false},
		{"dark mystic with Heal", classDarkMystic, []int32{skillHeal}, true},
		{"dark mystic before Heal", classDarkMystic, []int32{skillWindStrike}, false},
		{"Cleric with Battle Heal", classCleric, []int32{skillBattleHeal}, true},
		{"Cleric with Heal only", classCleric, []int32{skillHeal}, true},
		{"Cleric without a heal", classCleric, []int32{skillWindStrike}, false},
		{"Elven Oracle with Group Heal", classElvenOracle, []int32{skillGroupHeal}, true},
		{"Elven Oracle without a heal", classElvenOracle, nil, false},
		{"Shillien Oracle with Group Heal", classShillienOracle, []int32{skillGroupHeal}, true},
		{"Shillien Oracle without a heal", classShillienOracle, []int32{skillWindStrike}, false},
		{"Bishop keeps its heals", classBishop, []int32{skillBattleHeal}, true},
		{"Prophet keeps its heals", classProphet, []int32{skillHeal}, true},
		{"Orc Shaman with Chant of Life", classOrcShaman, []int32{skillChantLife}, true},
		{"Orc Shaman before Chant of Life", classOrcShaman, []int32{skillHeal}, false},
		{"Warcryer keeps Chant of Life", classWarcryer, []int32{skillChantLife}, true},
		{"orc mystic: Life Drain heals only itself", classOrcMystic, []int32{skillLifeDrain, skillHeal}, false},
		{"a fighter row never heals", 0, []int32{skillHeal, skillChantLife}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, DefaultPersona())
			linkBrain(b, standing{})
			b.world.Self.ClassID = tt.class
			b.world.Skills = make(map[int32]KnownSkill)
			for _, id := range tt.skills {
				b.world.Skills[id] = KnownSkill{ID: id, Level: 1}
			}
			b.publishStatus(t0, true)
			if got := b.Status().Heals; got != tt.want {
				t.Fatalf("Heals = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStatusHealsKeptOffline checks that a snapshot between sessions keeps
// the last session's Heals, and the next session's skill list sets it anew.
func TestStatusHealsKeptOffline(t *testing.T) {
	b, _ := newTestBrain(t, DefaultPersona())
	bot := &Bot{cfg: SessionConfig{Clock: newFakeClock(t0)}}
	bot.link.mail = make(chan mail, mailboxSize)
	bot.link.board.store(Status{})
	b.cfg.link, b.mail = &bot.link, bot.link.mail
	b.world.Self.ClassID = classCleric
	b.world.Skills = map[int32]KnownSkill{skillBattleHeal: {ID: skillBattleHeal, Level: 1}}
	b.publishStatus(t0, true)
	bot.publish(StateReconnecting, nil)
	if st := bot.Status(); !st.Heals || st.State != StateReconnecting {
		t.Fatalf("offline snapshot %+v, want the last session's Heals kept", st)
	}
	b.world.Skills = map[int32]KnownSkill{}
	b.publishStatus(at(1), true)
	if b.Status().Heals {
		t.Fatal("Heals kept after the skill list lost the heal")
	}
}
