package botclient

import (
	"testing"
	"time"
)

// TestPvPDuelDecline checks the Duels mode (docs/pvp/modes.md): without it
// the bot declines a challenge, of either kind, at the PvP reaction; with
// it, it leaves the challenge to the duel answer (X5).
func TestPvPDuelDecline(t *testing.T) {
	for _, tc := range []struct {
		name         string
		duels, party bool
	}{
		{"one-on-one declined", false, false},
		{"party duel declined", false, true},
		{"taken by a pvp persona", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, false, func(p *Persona) { p.PvP.Duels = tc.duels })
			b.nextTick = t0.Add(time.Hour)
			b.reactAt = time.Time{}
			b.handleOne(t, ExDuelAskStart{Requester: foeName, Party: tc.party}, at(time.Second))
			due := b.reactAt
			if due.Before(at(time.Second+b.cfg.PvPReactMin)) || due.After(at(time.Second+b.cfg.PvPReactMax)) {
				t.Fatalf("woke at %v, want the PvP reaction", due.Sub(t0))
			}
			rule, a := b.decide(due)
			if tc.duels {
				if a.kind == actDuelAnswer {
					t.Fatal("a pvp persona declined the challenge")
				}
				return
			}
			if rule != "duel" || a.kind != actDuelAnswer || string(a.payload) != string(encodeRequestDuelAnswerStart(tc.party, false)) {
				t.Fatalf("rule %q kind %v payload % x, want the decline", rule, a.kind, a.payload)
			}
			if _, err := b.send(a, due); err != nil {
				t.Fatal(err)
			}
			if _, a := b.decide(due.Add(time.Second)); a.kind == actDuelAnswer {
				t.Fatal("answered the challenge twice")
			}
			if d := b.duel(due); d.Phase != DuelIdle {
				t.Errorf("phase %v after the decline, want idle", d.Phase)
			}
		})
	}
}

// TestPvPSummonGuard checks the X4b cases of the summon rules
// (docs/pvp/targets.md "Summons and pets"): a summon whose flag turns to 2
// gets no damage step, its cast in flight is cancelled and a move goes;
// in an arena a summon's owner inside by more than 150 is fought in its
// stead, and a summon whose owner is outside is not fought.
func TestPvPSummonGuard(t *testing.T) {
	t.Run("blink", func(t *testing.T) {
		b := guardBrain(t, false, 0, 0, Step{Skill: skillWindStrike}, Step{Skill: grdPoison})
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		summonAt(t, b, "nobody", Point{Y: 200}, 1, 0)
		b.handleOne(t, hitSelf(pvpSummon, 5), t0)
		b.world.Target = pvpSummon
		toTwo := RelationChanged{ObjectID: pvpSummon, PvPFlag: 2}
		cast := MagicSkillUse{CasterID: selfID, TargetID: pvpSummon, SkillID: skillWindStrike, Level: 1, HitTime: 4 * time.Second}
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: skillWindStrike},
			{at: 100 * time.Millisecond, ev: cast, noDecide: true},
			{at: time.Second, ev: toTwo, kind: actTargetCancel, rule: "guard"},
			{at: 1100 * time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard"},
			{at: 1300 * time.Millisecond, kind: actCast, skill: grdPoison},
		})
	})
	for _, tc := range []struct {
		name   string
		owner  Point
		fought bool
	}{
		{"arena owner inside", Point{X: 1000}, true},
		{"arena owner outside", Point{X: 2500}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := guardBrain(t, true, 0, 0)
			b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
			player(t, b, pvpThird, "owner", tc.owner, 0, 0, 9)
			summonAt(t, b, "owner", Point{Y: 200}, 0, 0)
			b.handleOne(t, hitSelf(pvpSummon, 5), t0)
			got := b.attackers(at(time.Second))
			if fought := len(got) == 1 && got[0] == pvpThird; fought != tc.fought || !tc.fought && len(got) != 0 {
				t.Fatalf("attackers %v, fought %v", got, tc.fought)
			}
		})
	}
	t.Run("arena owner inside and dead: the summon itself", func(t *testing.T) {
		b := guardBrain(t, true, 0, 0)
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		player(t, b, pvpThird, "owner", Point{X: 1000}, 0, 0, 9)
		b.world.object(pvpThird).Dead = true
		summonAt(t, b, "owner", Point{Y: 200}, 0, 0)
		if !b.maySummon(pvpSummon, b.world.object(pvpSummon), at(time.Second)) {
			t.Fatal("a summon whose owner lies dead inside the arena is not fought")
		}
	})
}
