package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestRebuffSelfTarget checks ruleRebuff on a buff of a caster-centred
// datapack target (#675): a level 49 sword singer in pcID's party, pcID
// selected, the members showing only the songs on, never selects a member
// nor casts its auras or Spirit Barrier (SELF) on one, whether the kit
// flags them Self or not; with them on it casts nothing, a missing one
// goes on the bot without selecting it, and the party songs still go out.
func TestRebuffSelfTarget(t *testing.T) {
	now := at(time.Second)
	songs := []int32{skillSongOfWarding, skillSongOfWind, skillSongOfHunter}
	selfBuffs := []int32{skillDefenseAura, skillAttackAura, skillSpiritBarrier}
	all := append(append([]int32{}, selfBuffs...), songs...)
	// unflagged is the sword singer's buffs with no Self flag: the datapack
	// target alone keeps them on the bot.
	unflagged := ClassKit{Buffs: []Buff{
		{Skill: skillDefenseAura},
		{Skill: skillAttackAura},
		{Skill: skillSpiritBarrier},
		{Skill: skillSongOfWarding, Party: true},
		{Skill: skillSongOfWind, Party: true},
		{Skill: skillSongOfHunter, Party: true},
	}}
	for _, tt := range []struct {
		name     string
		unflag   bool
		noData   bool
		on       []int32
		skill    int32
		selected bool
	}{
		{name: "all on", on: all},
		{name: "all on, unflagged", unflag: true, on: all},
		{name: "all on, no datapack entry", noData: true, on: all},
		{name: "songs missing", on: selfBuffs, skill: skillSongOfWarding},
		{name: "songs missing, unflagged", unflag: true, on: selfBuffs, skill: skillSongOfWarding},
		{name: "Defense Aura missing", on: all[1:], skill: skillDefenseAura},
		{name: "Defense Aura missing, unflagged", unflag: true, on: all[1:], skill: skillDefenseAura},
		{name: "Spirit Barrier missing", on: append([]int32{skillDefenseAura, skillAttackAura}, songs...), skill: skillSpiritBarrier},
		// With no datapack entry a Self buff still selects the bot first.
		{name: "Defense Aura missing, no datapack entry", noData: true, on: all[1:], selected: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, classSwordSinger, 49, "SWORD", all...)
			b.world.Self.InCombat = false
			if !tt.noData {
				b.cfg.Travel.skills = make(map[datapack.SkillRef]datapack.Skill)
				for _, id := range selfBuffs {
					b.cfg.Travel.skills[datapack.SkillRef{ID: id, Level: 1}] = datapack.Skill{Type: "BUFF", Target: "SELF"}
				}
				for _, id := range songs {
					b.cfg.Travel.skills[datapack.SkillRef{ID: id, Level: 1}] = datapack.Skill{Type: "BUFF", Target: "PARTY"}
				}
			}
			if tt.unflag {
				withKit(b, unflagged)
			}
			inParty(b, 100, 100)
			b.world.Target = pcID
			effectsOn(b, tt.on...)
			// The members show the songs on, never a SELF buff.
			var ef []Effect
			for _, id := range tt.on {
				if slices.Contains(songs, id) {
					ef = append(ef, Effect{SkillID: id, Level: 1, Left: rowEffectDuration})
				}
			}
			for _, id := range []int32{pcID, rowPC2} {
				b.world.Apply(PartySpelled{Kind: partySpelledPlayer, ObjectID: id, Effects: ef}, t0)
			}
			a, ok := b.ruleRebuff(now)
			switch {
			case tt.selected:
				if !ok || a.kind != actSelect || a.object != selfID {
					t.Fatalf("ruleRebuff = kind %d object %#x, want the bot selected", a.kind, a.object)
				}
			case tt.skill == 0:
				wantNoRebuff(t, b, now)
			default:
				wantCast(t, a, tt.skill, selfID)
			}
			// Sent and retried for a while, nothing goes to a member.
			for range 2 * len(all) {
				a, ok := b.ruleRebuff(now)
				if !ok {
					break
				}
				if a.object != selfID {
					t.Fatalf("ruleRebuff = kind %d skill %d on %#x, want nothing on a member", a.kind, a.skill, a.object)
				}
				if a.kind == actSelect {
					b.world.Target = selfID
					continue
				}
				b.buffSent(a, now)
			}
		})
	}
}
