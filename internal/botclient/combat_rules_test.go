package botclient

import (
	"bytes"
	"math"
	"testing"
	"time"
)

// TestCombatRules checks rules 4 and 5 with the PvP defaults, attack
// skills, the blacklist and stuck detection: world state in, the decided
// rule and packet out.
func TestCombatRules(t *testing.T) {
	now := at(time.Second)
	const pc2 = int32(0x20000003)
	hitBy := func(b *Brain, id, dmg int32) {
		pos := b.world.Objects[id].Position(t0)
		b.world.Apply(Attack{AttackerID: id, Pos: pos, Hits: []AttackHit{{TargetID: selfID, Damage: dmg}}}, t0)
	}
	flag := func(b *Brain, id int32) { b.world.Apply(RelationChanged{ObjectID: id, PvPFlag: 1}, t0) }
	fighting := func(b *Brain) {
		b.world.Self.InCombat = true
		b.world.Apply(CharInfo{ObjectID: pc2, Name: "third", Pos: Point{X: -1000}}, t0)
	}
	tests := []struct {
		name     string
		persona  func(*Persona)
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
		// wantOp and wantObject check the packet's opcode and the object it
		// acts on when set.
		wantOp     byte
		wantObject int32
		check      func(t *testing.T, b *Brain, a Action)
	}{
		{
			name:     "hit with nothing selected selects the attacker",
			setup:    func(b *Brain) { fighting(b); hitBy(b, npcID, 10) },
			wantRule: "attacked", wantKind: actSelect, wantOp: opGameAction, wantObject: npcID,
		},
		{
			name:     "a selected attacker is attacked once",
			setup:    func(b *Brain) { fighting(b); hitBy(b, npcID, 10); b.world.Target = npcID },
			wantRule: "attacked", wantKind: actAttack, wantOp: opGameAction, wantObject: npcID,
		},
		{
			name: "attackers go before a fresh target",
			setup: func(b *Brain) {
				fighting(b)
				b.world.Apply(NPCInfo{ObjectID: 0x20000009, Attackable: true}, t0)
				b.world.Target = 0x20000009
				hitBy(b, npcID, 10)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: npcID,
		},
		{
			name:     "a flagged player is fought back without force",
			setup:    func(b *Brain) { fighting(b); flag(b, pcID); hitBy(b, pcID, 10); b.world.Target = pcID },
			wantRule: "attacked", wantKind: actAttack, wantOp: opGameAction, wantObject: pcID,
		},
		{
			name: "a player with karma is fought back",
			setup: func(b *Brain) {
				fighting(b)
				b.world.Apply(RelationChanged{ObjectID: pcID, Karma: 100}, t0)
				hitBy(b, pcID, 10)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pcID,
		},
		{
			name:     "an unflagged player is never force-attacked",
			setup:    func(b *Brain) { fighting(b); hitBy(b, pcID, 10); b.world.Target = pcID },
			wantRule: "attacked",
		},
		{
			name:     "the force-attack policy force-attacks an unflagged player",
			persona:  func(p *Persona) { p.PvP.ForceAttack = true },
			setup:    func(b *Brain) { fighting(b); hitBy(b, pcID, 10); b.world.Target = pcID },
			wantRule: "attacked", wantKind: actAttack, wantOp: opGameAttackRequest, wantObject: pcID,
		},
		{
			name:     "the policy may leave flagged players alone",
			persona:  func(p *Persona) { p.PvP.FightFlagged = false },
			setup:    func(b *Brain) { fighting(b); flag(b, pcID); hitBy(b, pcID, 10); b.world.Target = pcID },
			wantRule: "attacked",
		},
		{
			name: "an attacker who unflags is dropped",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, pcID, 10)
				b.world.Target, b.engaged, b.progressAt = pcID, pcID, now
				b.world.Apply(RelationChanged{ObjectID: pcID}, t0)
			},
			wantRule: "attacked", wantKind: actMove,
		},
		{
			name: "an attacker who unflagged is dropped before another is selected",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, pcID, 10)
				hitBy(b, npcID, 5)
				b.world.Target, b.engaged, b.progressAt = pcID, pcID, now
				b.world.Apply(RelationChanged{ObjectID: pcID}, t0)
			},
			wantRule: "attacked", wantKind: actMove,
		},
		{
			name:     "two attackers: the hardest hitter is selected",
			setup:    func(b *Brain) { fighting(b); flag(b, pcID); hitBy(b, npcID, 10); hitBy(b, pcID, 30) },
			wantRule: "outnumbered", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "two attackers: a target in a close race is kept",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, npcID, 10)
				hitBy(b, pcID, 20)
				b.world.Target = npcID
			},
			wantRule: "outnumbered", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "two attackers: a much harder hitter takes over",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, npcID, 10)
				hitBy(b, pcID, 21)
				b.world.Target = npcID
			},
			wantRule: "outnumbered", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "two attackers and outmatched: flee from the hardest hitter",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, npcID, 10)
				hitBy(b, pcID, 30)
				b.world.Self.HP = 40
			},
			wantRule: "outnumbered", wantKind: actMove,
			check: func(t *testing.T, _ *Brain, a Action) {
				// The player stands 1000 north of the bot.
				if to := moveTarget(a.payload); to.X != groundShift.X || to.Y != groundShift.Y-600 {
					t.Errorf("flee to %+v, want 600 south of %+v", to, groundShift)
				}
			},
		},
		{
			name:     "an unflagged player does not count toward two attackers",
			setup:    func(b *Brain) { fighting(b); hitBy(b, npcID, 10); hitBy(b, pcID, 30); b.world.Self.HP = 40 },
			wantRule: "attacked", wantKind: actSelect, wantObject: npcID,
		},
		{
			name:    "a ready attack skill goes before melee",
			persona: func(p *Persona) { p.AttackSkills = []int32{1177} },
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
				b.world.Apply(SkillList{Skills: []KnownSkill{{ID: 1177, Level: 3}}}, t0)
			},
			wantRule: "attacked", wantKind: actCast, wantOp: opGameRequestMagicSkillUse, wantObject: npcID,
			check: func(t *testing.T, _ *Brain, a Action) {
				if a.skill != 1177 || !bytes.Equal(a.payload, encodeRequestMagicSkillUse(1177, false)) {
					t.Errorf("cast % x, want skill 1177 unforced", a.payload)
				}
			},
		},
		{
			name:    "a skill the bot does not know falls back to melee",
			persona: func(p *Persona) { p.AttackSkills = []int32{1177} },
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: npcID,
		},
		{
			name:    "a skill in reuse falls back to melee",
			persona: func(p *Persona) { p.AttackSkills = []int32{1177} },
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
				b.world.Apply(SkillList{Skills: []KnownSkill{{ID: 1177}}}, t0)
				b.world.Apply(SkillCoolTime{Timers: []SkillTimer{{SkillID: 1177, Remaining: 5 * time.Second}}}, t0)
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "nothing goes out while a cast waits for its answer",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.castAt = npcID, t0
			},
			wantRule: "attacked",
		},
		{
			name: "a cast unanswered past the safety timeout lets the bot act",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.castAt = npcID, now.Add(-castTimeout)
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "nothing goes out while the bot casts",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
				b.world.Apply(MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: 2 * time.Second}, t0)
			},
			wantRule: "attacked",
		},
		{
			name: "a target out of attempts is blacklisted",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
				b.tries[npcID] = 4
			},
			wantRule: "attacked",
			check: func(t *testing.T, b *Brain, _ Action) {
				if !b.blacklist[npcID].Equal(now.Add(3*time.Minute)) || b.mayFight(npcID, now) {
					t.Errorf("blacklist = %v, want npc until +3m", b.blacklist)
				}
				if !b.mayFight(npcID, now.Add(3*time.Minute)) {
					t.Error("blacklist does not expire")
				}
			},
		},
		{
			name: "a blacklisted attacker is passed over for another",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, npcID, 50)
				hitBy(b, pcID, 10)
				b.world.Target = npcID
				b.blacklist[npcID] = now.Add(time.Minute)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "hit only by a blacklisted attacker: flee from it",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target = npcID
				b.blacklist[npcID] = now.Add(time.Minute)
			},
			wantRule: "attacked", wantKind: actMove,
			check: func(t *testing.T, b *Brain, a Action) {
				npc, self := b.world.Objects[npcID].Position(now), b.world.Self.Position(now)
				if to := moveTarget(a.payload); (to.X-self.X)*(npc.X-self.X)+(to.Y-self.Y)*(npc.Y-self.Y) >= 0 {
					t.Errorf("flee to %+v, want away from the NPC at %+v", to, npc)
				}
			},
		},
		{
			name: "the engaged target dies: no step, the next attacker is selected",
			setup: func(b *Brain) {
				fighting(b)
				flag(b, pcID)
				hitBy(b, npcID, 10)
				hitBy(b, pcID, 5)
				b.world.Target, b.engaged, b.progressAt = npcID, npcID, now
				if err := b.handle([]Event{Die{ObjectID: npcID}}, t0); err != nil {
					t.Fatal(err)
				}
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "the engaged target dies alone: the bot holds",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.engaged, b.progressAt = npcID, npcID, now
				if err := b.handle([]Event{Die{ObjectID: npcID}}, t0); err != nil {
					t.Fatal(err)
				}
			},
			wantRule: "attacked", wantKind: actHold,
		},
		{
			name:     "blacklist 0 never gives up",
			persona:  func(p *Persona) { p.BlacklistAfter = 0 },
			setup:    func(b *Brain) { fighting(b); hitBy(b, npcID, 10); b.world.Target = npcID; b.tries[npcID] = 100 },
			wantRule: "attacked", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "engaged and still past 2.5 s side steps",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.engaged, b.progressAt = npcID, npcID, now.Add(-stuckAfter)
			},
			wantRule: "attacked", wantKind: actMove,
			check: func(t *testing.T, b *Brain, a Action) {
				// The NPC stands on the bot's X axis: the step goes along Y.
				self := b.world.Self.Position(now)
				if to := moveTarget(a.payload); math.Abs(float64(to.X-self.X)) > 1 || math.Abs(float64(to.Y-self.Y)) != sideStep {
					t.Errorf("side step to %+v, want X 0 Y ±%d", to, sideStep)
				}
			},
		},
		{
			name: "engaged and swinging within 2.5 s holds",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.engaged, b.progressAt = npcID, npcID, now.Add(-stuckAfter+time.Millisecond)
			},
			wantRule: "attacked",
		},
		{
			name: "engaged and on the way holds and counts it as progress",
			setup: func(b *Brain) {
				fighting(b)
				hitBy(b, npcID, 10)
				b.world.Target, b.engaged = npcID, npcID
				b.world.Apply(MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 40}, t0)
			},
			wantRule: "attacked",
			check: func(t *testing.T, b *Brain, _ Action) {
				if !b.progressAt.Equal(now) {
					t.Errorf("progressAt = %v, want now", b.progressAt)
				}
			},
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
			tc.setup(b)
			// A flee walks: it needs ground under it.
			if tc.wantKind == actMove {
				onGround(t, b)
			}
			rule, a := b.decide(now)
			if rule != tc.wantRule || a.kind != tc.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tc.wantRule, tc.wantKind)
			}
			if tc.wantOp != 0 && a.payload[0] != tc.wantOp {
				t.Errorf("opcode %#02x, want %#02x", a.payload[0], tc.wantOp)
			}
			if tc.wantObject != 0 && a.object != tc.wantObject {
				t.Errorf("object %#x, want %#x", a.object, tc.wantObject)
			}
			if tc.check != nil {
				tc.check(t, b, a)
			}
		})
	}
}

// TestPvPPolicySupport checks the heal and buff default: never a flagged
// player unless the policy allows it.
func TestPvPPolicySupport(t *testing.T) {
	flagged, clean := &Object{Kind: KindPlayer, PvPFlag: 1}, &Object{Kind: KindPlayer, Karma: 10}
	if def := DefaultPersona().PvP; def.MaySupport(flagged) || !def.MaySupport(clean) {
		t.Error("default supports a flagged player or refuses an unflagged one")
	}
	if !(PvPPolicy{SupportFlagged: true}).MaySupport(flagged) {
		t.Error("SupportFlagged refuses a flagged player")
	}
}

// TestBrainCombatFlow plays a fight through the loop with a fake clock:
// a monster hits the bot, which selects it, attacks once, casts its skill
// and waits out the skill's hit time plus reuse before casting again.
func TestBrainCombatFlow(t *testing.T) {
	p := DefaultPersona()
	p.IdleChance, p.AttackSkills = 0, []int32{1177}
	b, out := newTestBrain(t, p)
	step := func(d time.Duration, evs ...Event) {
		t.Helper()
		if err := b.handle(evs, at(d)); err != nil {
			t.Fatal(err)
		}
		b.reactAt = at(d) // decide now: the reaction delay is covered by TestBrain*
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
	}
	last := func() []byte { return out.frames[len(out.frames)-1] }
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: 1177, Level: 1}}}, t0)

	step(time.Second, AutoAttackStart{ObjectID: selfID}, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}})
	if got := last(); got[0] != opGameAction {
		t.Fatalf("first packet % x, want a select click", got)
	}
	step(2*time.Second, MyTargetSelected{ObjectID: npcID})
	if got := last(); !bytes.Equal(got, encodeRequestMagicSkillUse(1177, false)) {
		t.Fatalf("second packet % x, want the cast", got)
	}
	sent := len(out.frames)
	// The server defers the cast behind a swing and answers ActionFailed:
	// the bot waits for the MagicSkillUse instead of clicking again.
	step(2500*time.Millisecond, ActionFailed{})
	if len(out.frames) != sent {
		t.Fatalf("sent % x while the cast was pending", last())
	}
	step(3*time.Second, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: time.Second, Reuse: 4 * time.Second})
	if len(out.frames) != sent {
		t.Fatalf("sent % x while casting", last())
	}
	step(4*time.Second, Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}})
	if got := last(); got[0] != opGameAction || b.engaged != npcID {
		t.Fatalf("after the cast % x engaged %#x, want a melee click", got, b.engaged)
	}
	step(7999*time.Millisecond, Attack{AttackerID: selfID, Hits: []AttackHit{{TargetID: npcID, Damage: 9}}})
	if len(out.frames) != sent+1 {
		t.Fatalf("sent % x before the skill's reuse ended", last())
	}
	step(8 * time.Second)
	if got := last(); !bytes.Equal(got, encodeRequestMagicSkillUse(1177, false)) {
		t.Fatalf("packet % x at reuse end, want the cast", got)
	}
	if b.tries[npcID] != 1 {
		t.Errorf("tries = %d, want 1 after a landed swing and one cast", b.tries[npcID])
	}
}
