package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestBrainCastRefused: the server refuses a heal it cannot pay for with
// a system message alone (no MagicSkillUse, no ActionFailed). The refusal
// ends the wait at once, so the bot fights back instead of holding, and
// the heal is not asked again for castRefusedHold.
func TestBrainCastRefused(t *testing.T) {
	tests := []struct {
		name     string
		msg      int32
		wantRule string
	}{
		{"not enough MP", msgNotEnoughMP, "attacked"},
		{"cannot see target", msgCantSeeTarget, "attacked"},
		// Any other message leaves the cast pending: the heal waits on.
		{"unrelated message", 212, "heal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance = 0
			b, out := newTestBrain(t, p)
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillHeal, Level: 1}}}, t0)
			b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID, HP: 40, MaxHP: 100}}}, t0)
			b.world.Self.InCombat, b.world.Target = true, pcID
			step := func(d time.Duration, evs ...Event) {
				t.Helper()
				if err := b.handle(evs, at(d)); err != nil {
					t.Fatal(err)
				}
				b.reactAt = at(d)
				if err := b.wake(at(d)); err != nil {
					t.Fatal(err)
				}
			}
			hit := Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}
			step(time.Second, hit)
			if got := out.frames[len(out.frames)-1]; !bytes.Equal(got, botwire.EncodeRequestMagicSkillUse(skillHeal, false)) {
				t.Fatalf("packet % x, want Heal", got)
			}
			step(1200*time.Millisecond, hit, SystemMessage{ID: tc.msg})
			if b.lastRule != tc.wantRule {
				t.Fatalf("rule %q after the message, want %q", b.lastRule, tc.wantRule)
			}
			if tc.wantRule == "heal" {
				return
			}
			if b.world.SkillReady(skillHeal, at(1200*time.Millisecond+castRefusedHold-time.Millisecond)) {
				t.Error("the refused heal is ready again before castRefusedHold")
			}
		})
	}
}

// TestBrainAssistWake: the leader's hit wakes the bot for the assist
// without postponing a regular tick due sooner or the reaction to the bot
// being hit meanwhile; the assist's own wake-up survives both.
func TestBrainAssistWake(t *testing.T) {
	p := DefaultPersona()
	p.IdleChance = 0
	p.AssistMin, p.AssistMax = 3*time.Second, 3*time.Second
	b, _ := newTestBrain(t, p)
	b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID, HP: 100, MaxHP: 100}}}, t0)
	b.nextTick = at(1500 * time.Millisecond)

	b.handleOne(t, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: npcID, Damage: 30}}}, at(time.Second))
	if got := b.decisionAt(); got != at(1500*time.Millisecond) {
		t.Fatalf("decision at %v after the leader's hit, want the tick at %v", got.Sub(t0), 1500*time.Millisecond)
	}
	b.nextTick = at(10 * time.Second)
	if got, want := b.decisionAt(), at(4*time.Second); got != want {
		t.Fatalf("decision at %v, want the assist at %v", got.Sub(t0), want.Sub(t0))
	}

	hitAt := at(1010 * time.Millisecond)
	b.handleOne(t, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, hitAt)
	react := b.decisionAt()
	if d := react.Sub(hitAt); d < b.cfg.ReactMin || d > b.cfg.ReactMax {
		t.Fatalf("reacts %v after being hit, want %v-%v", d, b.cfg.ReactMin, b.cfg.ReactMax)
	}
	if err := b.wake(react); err != nil {
		t.Fatal(err)
	}
	if got, want := b.decisionAt(), at(4*time.Second); got.After(want) {
		t.Fatalf("next decision at %v, want the assist by %v", got.Sub(t0), want.Sub(t0))
	}
	if err := b.wake(at(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if b.assist.armed || !b.decisionAt().After(at(4*time.Second)) {
		t.Fatalf("assist wake-up not spent: decision at %v", b.decisionAt().Sub(t0))
	}
}
