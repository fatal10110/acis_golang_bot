package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The servitor orders' payloads (RequestActionUse 22 and 23).
var (
	servitorAttackFrame = botwire.EncodeRequestActionUse(botwire.ActionServitorAttack)
	servitorStopFrame   = botwire.EncodeRequestActionUse(botwire.ActionServitorStop)
)

// servitorBrain is kitBrain with the bot's servitor out, idle next to it.
func servitorBrain(t *testing.T, kit ClassKit) *Brain {
	t.Helper()
	b := kitBrain(t, kit, 100, skillWindStrike)
	b.world.Apply(testServitorInfo, t0)
	return b
}

// sendNow sends a, failing the test unless it went out.
func sendNow(t *testing.T, b *Brain, a Action, now time.Time) {
	t.Helper()
	if ok, err := b.send(a, now); err != nil || !ok {
		t.Fatalf("send %v: sent %v, %v", a.kind, ok, err)
	}
}

// TestBrainServitorOrderFlow plays a fight through the loop with the
// servitor out: a monster hits the bot, which selects it, orders the
// servitor onto it (22) and then casts its opener (docs/servitors.md
// section 2).
func TestBrainServitorOrderFlow(t *testing.T) {
	p := DefaultPersona()
	p.IdleChance, p.AttackSkills = 0, []int32{1177}
	b, out := newTestBrain(t, p)
	step := func(d time.Duration, evs ...Event) []byte {
		t.Helper()
		if err := b.handle(evs, at(d)); err != nil {
			t.Fatal(err)
		}
		b.reactAt = at(d)
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
		return out.frames[len(out.frames)-1]
	}
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: 1177, Level: 1}}}, t0)
	b.world.Apply(testServitorInfo, t0)

	if got := step(time.Second, AutoAttackStart{ObjectID: selfID}, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}); got[0] != opGameAction {
		t.Fatalf("first packet % x, want a select click", got)
	}
	if got := step(2*time.Second, MyTargetSelected{ObjectID: npcID}); !bytes.Equal(got, servitorAttackFrame) {
		t.Fatalf("second packet % x, want the servitor's attack order", got)
	}
	if got := step(3*time.Second, MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 36}); !bytes.Equal(got, botwire.EncodeRequestMagicSkillUse(1177, false)) {
		t.Fatalf("third packet % x, want the opener", got)
	}
	for _, f := range out.frames[2:] {
		if bytes.Equal(f, servitorAttackFrame) {
			t.Fatalf("the attack order went out again: % x", out.frames)
		}
	}
}

// TestServitorOrder checks when the fight step orders the servitor onto
// the bot's target: events and the clock in, the order out (or none).
func TestServitorOrder(t *testing.T) {
	strike := func(target int32) Attack {
		return Attack{AttackerID: ownServitor, Hits: []AttackHit{{TargetID: target}}}
	}
	tests := []struct {
		name string
		// setup runs at t0; ordered sends one 22 onto npcID at t0 first.
		setup   func(b *Brain)
		ordered bool
		later   []Event // applied at t0+1s
		at      time.Duration
		want    bool
	}{
		{name: "idle servitor, monster target", want: true},
		{name: "no servitor", setup: func(b *Brain) { b.world.Servitor = nil }},
		{name: "dead servitor", setup: func(b *Brain) { b.world.Apply(Die{ObjectID: ownServitor}, t0) }},
		{name: "servitor on it already", setup: func(b *Brain) { b.world.Apply(strike(npcID), t0) }},
		{name: "never onto a player", setup: func(b *Brain) { b.world.Target = pcID }},
		{
			name: "never onto a player's summon",
			setup: func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 12077, Attackable: true, Summon: true}, t0)
			},
		},
		{name: "no repeat inside ServitorOrderRetry", ordered: true, at: servitorOrderRetry - time.Millisecond},
		{name: "repeat after it with no answer", ordered: true, at: servitorOrderRetry, want: true},
		{
			name: "no repeat once the servitor walks to it", ordered: true, at: servitorOrderRetry,
			later: []Event{MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 36}},
		},
		{name: "no repeat once it swings at it", ordered: true, at: servitorOrderRetry, later: []Event{strike(npcID)}},
		{
			name: "repeat once it walks back to the bot", ordered: true, at: servitorOrderRetry, want: true,
			later: []Event{strike(npcID), MoveToPawn{ObjectID: ownServitor, TargetID: selfID}},
		},
		{
			name: "the refusal holds it", ordered: true, at: servitorOrderRetry,
			later: []Event{SystemMessage{ID: msgPetRefusingOrder}},
		},
		{
			name: "the hold ends", ordered: true, at: time.Second + servitorOrderRetry, want: true,
			later: []Event{SystemMessage{ID: msgPetRefusingOrder}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := servitorBrain(t, ClassKit{Rotation: []Step{{Skill: skillWindStrike}}})
			if tt.setup != nil {
				tt.setup(b)
			}
			if tt.ordered {
				a, ok := b.servitorOrder(npcID, t0)
				if !ok {
					t.Fatal("no first order")
				}
				sendNow(t, b, a, t0)
			}
			if err := b.handle(tt.later, at(time.Second)); err != nil {
				t.Fatal(err)
			}
			a := b.fight(b.world.Target, at(tt.at))
			if got := bytes.Equal(a.payload, servitorAttackFrame); got != tt.want {
				t.Fatalf("fight sends %v % x, want the attack order %v", a.kind, a.payload, tt.want)
			}
		})
	}
}

// TestServitorLead checks a kit with ServitorLead holds its first attack
// on a fresh target after the servitor's order, for Persona.ServitorLead
// or until the servitor's first swing at it, and a kit without it does not.
func TestServitorLead(t *testing.T) {
	tests := []struct {
		name  string
		lead  bool
		swing bool // the servitor swings at the target at t0+0.5s
		at    time.Duration
		cast  bool
	}{
		{name: "no lead: the opener right after the order", at: 500 * time.Millisecond, cast: true},
		{name: "the lead holds the opener", lead: true, at: time.Second},
		{name: "the lead ends", lead: true, at: 1500 * time.Millisecond, cast: true},
		{name: "the servitor's first swing ends it", lead: true, swing: true, at: time.Second, cast: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := servitorBrain(t, ClassKit{Rotation: []Step{{Skill: skillWindStrike}}, ServitorLead: tt.lead})
			a := b.fight(npcID, t0)
			if !bytes.Equal(a.payload, servitorAttackFrame) {
				t.Fatalf("first action %v % x, want the attack order", a.kind, a.payload)
			}
			sendNow(t, b, a, t0)
			if tt.swing {
				b.world.Apply(Attack{AttackerID: ownServitor, Hits: []AttackHit{{TargetID: npcID}}}, at(500*time.Millisecond))
			}
			a = b.fight(npcID, at(tt.at))
			if got := a.kind == actCast && a.skill == skillWindStrike; got != tt.cast {
				t.Fatalf("at %v: %v (skill %d), want the opener %v", tt.at, a.kind, a.skill, tt.cast)
			}
		})
	}
}

// TestServitorGuard checks the servitor rule's guard: with the servitor
// idle, a monster hitting the bot or the servitor is selected and the
// servitor ordered onto it; never a player, nor with the servitor busy,
// dead or absent.
func TestServitorGuard(t *testing.T) {
	hitServitor := func(attacker int32) Attack {
		return Attack{AttackerID: attacker, Hits: []AttackHit{{TargetID: ownServitor, Damage: 9}}}
	}
	hitBot := Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 9}}}
	tests := []struct {
		name   string
		events []Event
		want   bool // the guard selects npcID, then orders 22
	}{
		{name: "a monster hits the servitor", events: []Event{hitServitor(npcID)}, want: true},
		{name: "a monster hits the bot", events: []Event{hitBot}, want: true},
		{name: "nothing hits", events: nil},
		{name: "a player hits the servitor", events: []Event{hitServitor(pcID)}},
		{
			name:   "the servitor fights already",
			events: []Event{hitServitor(npcID), Attack{AttackerID: ownServitor, Hits: []AttackHit{{TargetID: npcID}}}},
		},
		{name: "dead servitor", events: []Event{hitServitor(npcID), Die{ObjectID: ownServitor}}},
		{name: "no servitor", events: []Event{hitServitor(npcID), PetDelete{SummonType: SummonServitor, ObjectID: ownServitor}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := servitorBrain(t, ClassKit{Rotation: []Step{{Skill: skillWindStrike}}})
			b.world.Target, b.world.Self.InCombat = 0, false
			for _, ev := range tt.events {
				b.world.Apply(ev, at(time.Second))
			}
			now := at(time.Second)
			a, ok := b.ruleServitor(now)
			if !tt.want {
				if ok || a.kind != actHold {
					t.Fatalf("guard %v (%v) % x, want none", ok, a.kind, a.payload)
				}
				return
			}
			if !ok || a.kind != actSelect || a.object != npcID {
				t.Fatalf("guard %v (%v on %d), want the monster selected", ok, a.kind, a.object)
			}
			sendNow(t, b, a, now)
			b.world.Apply(MyTargetSelected{ObjectID: npcID}, now)
			if a, ok = b.ruleServitor(now); !ok || !bytes.Equal(a.payload, servitorAttackFrame) {
				t.Fatalf("guard %v (%v) % x, want the attack order", ok, a.kind, a.payload)
			}
			sendNow(t, b, a, now)
			if a, ok = b.ruleServitor(at(2 * time.Second)); ok || a.kind != actHold {
				t.Fatalf("guard %v (%v) % x while the order awaits its answer, want none", ok, a.kind, a.payload)
			}
		})
	}
}

// TestServitorStop checks the stop order: once per give-up on a flee or a
// target the bot drops, held by a refusal, none without a servitor on a
// target.
func TestServitorStop(t *testing.T) {
	onMonster := Attack{AttackerID: ownServitor, Hits: []AttackHit{{TargetID: npcID}}}
	tests := []struct {
		name   string
		events []Event
		drop   bool // the bot blacklists npcID
		give   func(b *Brain, now time.Time) Action
		want   bool
	}{
		{name: "flee", events: []Event{onMonster}, give: fleeFromNPC, want: true},
		{name: "dropped target", events: []Event{onMonster}, drop: true, give: ruleServitorAction, want: true},
		{name: "kept target", events: []Event{onMonster}, give: ruleServitorAction},
		{name: "flee, servitor idle", give: fleeFromNPC},
		{name: "flee, dead servitor", events: []Event{onMonster, Die{ObjectID: ownServitor}}, give: fleeFromNPC},
		{name: "flee under a refusal's hold", events: []Event{onMonster, SystemMessage{ID: msgPetRefusingOrder}}, give: fleeFromNPC},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := servitorBrain(t, ClassKit{Rotation: []Step{{Skill: skillWindStrike}}})
			now := at(time.Second)
			if err := b.handle(tt.events, now); err != nil {
				t.Fatal(err)
			}
			if tt.drop {
				b.blacklist.set(npcID, now.Add(time.Minute))
			}
			a := tt.give(b, now)
			if got := bytes.Equal(a.payload, servitorStopFrame); got != tt.want {
				t.Fatalf("%v % x, want the stop order %v", a.kind, a.payload, tt.want)
			}
			if !tt.want {
				return
			}
			sendNow(t, b, a, now)
			if a := tt.give(b, at(5*time.Second)); bytes.Equal(a.payload, servitorStopFrame) {
				t.Fatal("a second stop order for the same give-up")
			}
		})
	}
}

// fleeFromNPC is the flee from npcID.
func fleeFromNPC(b *Brain, now time.Time) Action { return b.flee(npcID, now) }

// ruleServitorAction is ruleServitor's action.
func ruleServitorAction(b *Brain, now time.Time) Action {
	a, _ := b.ruleServitor(now)
	return a
}

// TestServitorRuleOrder checks the servitor rule runs after "heal" and
// before "raise" (docs/servitors.md section 2, "Rule order").
func TestServitorRuleOrder(t *testing.T) {
	for i, r := range playRules {
		if r.name != "servitor" {
			continue
		}
		if i == 0 || i+1 == len(playRules) || playRules[i-1].name != "heal" || playRules[i+1].name != "raise" {
			t.Fatalf("servitor rule at %d, want between heal and raise", i)
		}
		return
	}
	t.Fatal("no servitor rule")
}
