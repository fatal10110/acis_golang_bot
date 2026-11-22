package botclient

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

// TestCrafterPersonaClanInvite: the crafter preset accepts a clan
// invitation while clanless, of any player or, when its inviters name
// some, only of one so named; in a clan it declines (D20).
func TestCrafterPersonaClanInvite(t *testing.T) {
	tests := []struct {
		name     string
		inviters []string
		inClan   bool
		want     bool
	}{
		{"clanless accepts any player", nil, false, true},
		{"listed inviter accepted", []string{"Alden", "Player"}, false, true},
		{"unlisted inviter declined", []string{"Alden"}, false, false},
		{"in a clan declines", nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAuthority(AuthorityConfig{Commanders: []int32{gmID}}).Hooks(CrafterPersona(tt.inviters...))
			p.IdleChance = 0
			b, _ := newTestBrain(t, p)
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "player"}, t0)
			if tt.inClan {
				b.world.Apply(JoinPledge{ClanID: otherClan}, t0)
			}
			evs := []Event{SystemMessage{ID: msgInvitedToClan, Text: [2]string{"player", "Crafters"}}, AskJoinPledge{RequesterID: pcID, Clan: "Crafters"}}
			if err := b.handle(evs, at(100*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			_, a := b.decide(at(time.Second))
			if a.kind != actAnswerClan || !bytes.Equal(a.payload, encodeRequestAnswerJoinPledge(tt.want)) {
				t.Fatalf("answer %d % x, want accept=%v", a.kind, a.payload, tt.want)
			}
		})
	}
}

// TestCrafterPersonaCreateItem: the crafter learns Create Item as a
// Dwarven Fighter at the dwarven trainers; the default persona does not.
func TestCrafterPersonaCreateItem(t *testing.T) {
	p := CrafterPersona()
	if got := p.LearnSkills[DwarvenFighter.ID]; !slices.Equal(got, []int32{skillCreateItem}) {
		t.Fatalf("learn skills of class 53 = %v, want Create Item", got)
	}
	for _, id := range []int32{30298, 31267} { // a blacksmith, a warehouse keeper
		if !slices.Contains(p.Trainers[DwarvenFighter.ID], id) {
			t.Errorf("trainer %d missing for class 53", id)
		}
	}
	if d := DefaultPersona(); len(d.LearnSkills[DwarvenFighter.ID]) > 0 || d.Crafter.CheckEvery > 0 {
		t.Fatal("the default persona learns Create Item or crafts")
	}
	if p.Crafter.CheckEvery < warehouseOpenGap || p.Crafter.FailStreak == 0 || p.Crafter.TimeoutStreak == 0 {
		t.Fatalf("crafter policy %+v: want checks at least MinOpenGap apart and both breakers", p.Crafter)
	}
	// A level 5 Dwarven Fighter visits the blacksmith in sight.
	p.IdleChance, p.AutoShots = 0, false
	b, _ := newTestBrain(t, p)
	b.world.Self.ClassID, b.world.Self.Level, b.world.Self.SP = DwarvenFighter.ID, 5, 1000
	b.world.Apply(NPCInfo{ObjectID: trainerObj, NPCID: 30298, Pos: Point{X: 300}}, t0)
	if rule, a := b.decide(at(time.Second)); rule != "train" || a.kind != actSelect {
		t.Fatalf("decide = %q %d, want to select the dwarven trainer", rule, a.kind)
	}
}
