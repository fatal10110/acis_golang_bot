package botclient

import (
	"testing"
	"time"
)

// castOnSelf checks that a, decided at now, selects the bot, plays the
// server's MyTargetSelected for it (network/targeting.go selectLiveTarget)
// and returns the bot's next decision, 100 ms later.
func castOnSelf(t *testing.T, b *Brain, a Action, now time.Time) Action {
	t.Helper()
	if a.kind != actSelect || a.object != selfID {
		t.Fatalf("action %v object %#x skill %d, want the bot %#x selected first", a.kind, a.object, a.skill, selfID)
	}
	next := now.Add(100 * time.Millisecond)
	b.handleOne(t, MyTargetSelected{ObjectID: selfID}, next)
	_, a = b.decide(next)
	if a.kind == actCast && a.object != selfID {
		t.Fatalf("cast of %d on %#x, want it on the bot %#x", a.skill, a.object, selfID)
	}
	return a
}

// TestPvPSelfStepTarget checks a PvP row's self step (#560): Mental Shield
// (datapack target ONE, so the server casts it on the selected creature,
// handler/target/cast_rejection.go oneCastRejection) goes on the bot only
// once the bot selected itself, and the fight selects its opponent again
// after it; a SELF-target self step (Self Heal) is cast with the opponent
// still selected.
func TestPvPSelfStepTarget(t *testing.T) {
	t.Run("the CL shields itself against a Human Mystic", func(t *testing.T) {
		b := mysticRowBrain(t, classCleric, 30, lyrMystic, false)
		b.world.Target = pcID
		_, a := b.decide(rowNow)
		a = castOnSelf(t, b, a, rowNow)
		if a.kind != actCast || a.skill != skillMentalShield {
			t.Fatalf("action %v skill %d, want Mental Shield on the bot", a.kind, a.skill)
		}
		// The shield lands; the fight goes back to the opponent.
		landed := rowNow.Add(2 * time.Second)
		shield := MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skillMentalShield, Level: 1, HitTime: time.Second}
		b.handleOne(t, shield, rowNow.Add(200*time.Millisecond))
		b.handleOne(t, AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillMentalShield, Level: 1, Left: 1200 * time.Second}}}, landed)
		if _, a := b.decide(landed); a.kind != actSelect || a.object != pcID {
			t.Fatalf("action %v object %#x, want the opponent %#x selected again", a.kind, a.object, pcID)
		}
	})
	t.Run("a SELF-target step goes with the opponent selected", func(t *testing.T) {
		b := mysticRowBrain(t, classElvenWizard, 30, lyrWarrior, false)
		b.world.Target = pcID
		b.world.Self.HP = 300
		if _, a := b.decide(rowNow); a.kind != actCast || a.skill != skillSelfHeal || a.object != selfID {
			t.Fatalf("action %v object %#x skill %d, want Self Heal cast at once", a.kind, a.object, a.skill)
		}
	})
}
