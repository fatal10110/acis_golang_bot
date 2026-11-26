package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// lyrSlow is a slow (run speed × 0.5 for 60 s, no CharInfo bit) the
// cancel tests cast.
const lyrSlow = int32(9208)

// cancelBrain is kiteBrain (a caster on a melee opponent at Y 300) that
// also knows lyrSlow.
func cancelBrain(t *testing.T) *Brain {
	t.Helper()
	b := kiteBrain(t, false, Point{Y: 300}, nil, nil)
	b.cfg.Travel.skills[datapack.SkillRef{ID: lyrSlow, Level: 1}] = datapack.Skill{
		Type: "DEBUFF", Target: "ONE", Magic: true, Offensive: true, HitTime: time.Second,
		Effects: []datapack.Effect{{Name: "Debuff", Time: 60, Funcs: []datapack.StatFunc{{Op: "mul", Stat: "runSpd", Value: 0.5}}}},
	}
	b.world.Skills[lyrSlow] = KnownSkill{ID: lyrSlow, Level: 1}
	return b
}

// TestPvPCastCanceled checks that the bot's own cast the server cancels
// before its hit (MagicSkillCanceled to the caster) leaves no landing: no
// kite on a slow that never landed and the slow is due again; a cancel at
// or after the hit, or another caster's, keeps it; a cancelled recast puts
// back the earlier landing.
func TestPvPCastCanceled(t *testing.T) {
	slow := MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: lyrSlow, Level: 1, HitTime: time.Second}
	tests := []struct {
		name     string
		earlier  bool          // the slow was cast once before, at 9.5 s
		caster   int32         // the MagicSkillCanceled's caster; 0 for none
		cancelAt time.Duration // after the cast at 10 s
		wantKite bool
	}{
		{name: "cancelled before the hit", caster: selfID, cancelAt: 500 * time.Millisecond},
		{name: "no cancel", wantKite: true},
		{name: "cancel after the hit", caster: selfID, cancelAt: 1200 * time.Millisecond, wantKite: true},
		{name: "another caster's cancel", caster: pcID, cancelAt: 500 * time.Millisecond, wantKite: true},
		{name: "cancelled recast: the earlier slow holds", earlier: true, caster: selfID, cancelAt: 500 * time.Millisecond, wantKite: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := cancelBrain(t)
			if tc.earlier {
				b.handleOne(t, slow, at(9500*time.Millisecond))
			}
			b.handleOne(t, slow, at(10*time.Second))
			if tc.caster != 0 {
				b.handleOne(t, MagicSkillCanceled{CasterID: tc.caster}, at(10*time.Second+tc.cancelAt))
			}
			now := at(12 * time.Second)
			skill, kite := b.kiting(pcID, now)
			if kite != tc.wantKite || kite && skill != lyrSlow {
				t.Fatalf("kiting = %d, %v, want %v on the slow", skill, kite, tc.wantKite)
			}
			if due := b.controlDue(pcID, lyrSlow, charmodel.Fighter{}, charmodel.Fighter{}, false, now); due == tc.wantKite {
				t.Fatalf("slow due %v, want %v", due, !tc.wantKite)
			}
			if !tc.wantKite {
				if _, a := b.decide(now); a.kind == actMove {
					t.Fatalf("kite walk to %v on a slow that never landed", moveDest(t, a))
				}
			}
		})
	}
}

// TestPvPDuelFreshFight checks that a duel's start (its first
// ExDuelUpdateUserInfo) starts the layer's record of the opponent anew: two
// resists of a root in an earlier fight no longer bar it; a later window
// of the same duel keeps the record.
func TestPvPDuelFreshFight(t *testing.T) {
	b := cancelBrain(t)
	r := b.pvp.opponents[pcID]
	r.fight.resists = map[int32]int{grdRoot: 2}
	duelUntil(t, b, DuelFighting, at(10*time.Second))
	if n := b.pvp.opponents[pcID].fight.resists[grdRoot]; n != 0 {
		t.Fatalf("resists %d after the duel started, want 0", n)
	}
	b.pvp.opponents[pcID].fight.resists = map[int32]int{grdRoot: 1}
	b.handleOne(t, ExDuelUpdateUserInfo{Name: foeName, ObjectID: pcID, HP: 90, MaxHP: 100}, at(20*time.Second))
	if n := b.pvp.opponents[pcID].fight.resists[grdRoot]; n != 1 {
		t.Fatalf("resists %d after a later window, want 1 kept", n)
	}
}
