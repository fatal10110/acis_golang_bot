package botclient

import (
	"encoding/binary"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// recordSender records every payload sent.
type recordSender struct{ frames [][]byte }

func (r *recordSender) Send(p []byte) error {
	r.frames = append(r.frames, slices.Clone(p))
	return nil
}

// newTestBrain is a started brain at t0 over newTestWorld with full HP and
// MP, playing p with no idle drift unless p asks for it.
func newTestBrain(t *testing.T, p Persona) (*Brain, *recordSender) {
	t.Helper()
	w := newTestWorld(t)
	w.Self.HP, w.Self.MaxHP, w.Self.MP, w.Self.MaxMP = 100, 100, 100, 100
	out := &recordSender{}
	b := NewBrain(w, NewEvents(64), out, newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(1, 2))})
	b.start(t0)
	return b, out
}

// moveTarget is the destination of a MoveBackwardToLocation payload.
func moveTarget(p []byte) Point {
	i := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
	return Point{X: i(1), Y: i(5), Z: i(9)}
}

func TestRuleTable(t *testing.T) {
	now := at(time.Second)
	hook := func(a Action) func(*World, time.Time) Action { return func(*World, time.Time) Action { return a } }
	tests := []struct {
		name     string
		persona  func(*Persona)
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
		// anyAction accepts any action but the zero one.
		anyAction bool
		check     func(t *testing.T, b *Brain, a Action)
	}{
		{name: "nothing to do", wantRule: ""},
		{
			name:     "dead waits for the death policy",
			setup:    func(b *Brain) { b.world.Self.Dead, b.world.Self.InCombat, b.world.Self.HP = true, true, 5 },
			wantRule: "dead",
		},
		{
			name:     "dead runs the death hook",
			persona:  func(p *Persona) { p.Death = hook(emoteAction(2)) },
			setup:    func(b *Brain) { b.world.Self.Dead = true },
			wantRule: "dead", wantKind: actEmote,
		},
		{
			name:     "open prompt waits for an answer",
			setup:    func(b *Brain) { b.world.Prompts.Restart.Open = true; b.world.Self.InCombat = true },
			wantRule: "prompt",
		},
		{
			name:     "open prompt runs the answer hook",
			persona:  func(p *Persona) { p.Answer = hook(sayAction("ok")) },
			setup:    func(b *Brain) { b.world.Prompts.Restart.Open = true },
			wantRule: "prompt", wantKind: actSay,
		},
		{
			name:     "critical HP in combat flees away from the target",
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Self.HP, b.world.Target = true, 10, npcID },
			wantRule: "critical", wantKind: actMove,
			check: func(t *testing.T, _ *Brain, a Action) {
				// The NPC stands 1000 east of the bot.
				if to := moveTarget(a.payload); to.X != groundShift.X-600 || to.Y != groundShift.Y || !a.walk {
					t.Errorf("flee to %+v (walker %v), want a walker segment 600 west of %+v", to, a.walk, groundShift)
				}
			},
		},
		{
			name:     "critical HP stands up before fleeing",
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Self.HP, b.world.Self.Sitting = true, 10, true },
			wantRule: "critical", wantKind: actSitStand,
		},
		{
			name: "critical HP keeps fleeing while on the way",
			setup: func(b *Brain) {
				b.world.Self.InCombat, b.world.Self.HP = true, 10
				b.world.Apply(MoveToLocation{ObjectID: selfID, Dest: Point{X: -5000}}, t0)
			},
			wantRule: "critical",
		},
		{
			name:     "critical threshold 0 never flees",
			persona:  func(p *Persona) { p.CriticalHP = 0 },
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Self.HP = true, 1 },
			wantRule: "attacked",
		},
		{
			name:     "attacked attacks the selected monster",
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Target = true, npcID },
			wantRule: "attacked", wantKind: actAttack,
			check: func(t *testing.T, _ *Brain, a Action) {
				if a.object != npcID {
					t.Errorf("attack on %#x, want %#x", a.object, npcID)
				}
			},
		},
		{
			name: "attacked clicks a monster once",
			setup: func(b *Brain) {
				b.world.Self.InCombat, b.world.Target, b.engaged, b.progressAt = true, npcID, npcID, now
			},
			wantRule: "attacked",
		},
		{
			name:     "attacked leaves a player target alone",
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Target = true, pcID },
			wantRule: "attacked",
		},
		{
			name: "attacked leaves a dead monster",
			setup: func(b *Brain) {
				b.world.Self.InCombat, b.world.Target = true, npcID
				b.world.Objects[npcID].Dead = true
			},
			wantRule: "attacked",
		},
		{
			name:     "attacked never stays seated",
			setup:    func(b *Brain) { b.world.Self.InCombat, b.world.Self.Sitting = true, true },
			wantRule: "attacked", wantKind: actSitStand,
		},
		{
			name:     "low HP out of combat sits",
			setup:    func(b *Brain) { b.world.Self.HP = 30 },
			wantRule: "rest", wantKind: actSitStand,
			check: func(t *testing.T, b *Brain, _ Action) {
				if !b.resting {
					t.Error("not resting after sitting down")
				}
			},
		},
		{
			name:     "low MP out of combat sits",
			setup:    func(b *Brain) { b.world.Self.MP = 20 },
			wantRule: "rest", wantKind: actSitStand,
		},
		{
			name:     "a toggle waits for the server's answer",
			setup:    func(b *Brain) { b.world.Self.HP, b.toggledAt = 30, now.Add(-time.Second) },
			wantRule: "rest",
		},
		{
			name:     "resting stays seated until recovered",
			setup:    func(b *Brain) { b.world.Self.HP, b.world.Self.Sitting, b.resting = 80, true, true },
			wantRule: "rest",
		},
		{
			name:     "recovered stands up",
			setup:    func(b *Brain) { b.world.Self.HP, b.world.Self.Sitting, b.resting = 95, true, true },
			wantRule: "rest", wantKind: actSitStand,
		},
		{
			name:     "recovered and standing stops resting",
			setup:    func(b *Brain) { b.world.Self.HP, b.resting = 95, true },
			wantRule: "",
			check: func(t *testing.T, b *Brain, _ Action) {
				if b.resting {
					t.Error("still resting")
				}
			},
		},
		{
			name: "goal hook runs out of combat",
			persona: func(p *Persona) {
				p.Goal = func(*World, time.Time) (Action, bool) { return moveAction(Point{X: 5}, Point{}), true }
			},
			wantRule: "goal", wantKind: actMove,
		},
		{
			name: "idle goal leaves the turn to idle drift",
			persona: func(p *Persona) {
				p.IdleChance = 1
				p.Goal = func(*World, time.Time) (Action, bool) { return Action{}, false }
			},
			wantRule: "idle", anyAction: true,
		},
		{
			name:     "idle drift stands up a bot sitting idle",
			persona:  func(p *Persona) { p.IdleChance = 1 },
			setup:    func(b *Brain) { b.world.Self.Sitting = true },
			wantRule: "idle", wantKind: actSitStand,
		},
		{
			name:     "store mode holds in combat",
			setup:    func(b *Brain) { b.world.Self.StoreType, b.world.Self.InCombat, b.world.Self.HP = 1, true, 10 },
			wantRule: "store",
		},
		{
			name:     "store opening holds instead of idle drift",
			persona:  func(p *Persona) { p.IdleChance = 1 },
			setup:    func(b *Brain) { b.store.step, b.store.waitUntil = storeAskManage, now },
			wantRule: "store",
		},
		{
			name:     "store mode still follows the death policy",
			persona:  func(p *Persona) { p.Death = hook(emoteAction(3)) },
			setup:    func(b *Brain) { b.world.Self.Dead, b.world.Self.StoreType = true, storeTypeSell },
			wantRule: "dead", wantKind: actEmote,
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
			if tc.setup != nil {
				tc.setup(b)
			}
			// A flee walks: it needs ground under it.
			if tc.wantKind == actMove {
				onGround(t, b)
			}
			rule, a := b.decide(now)
			if tc.anyAction && a.kind != actHold {
				tc.wantKind = a.kind
			}
			if rule != tc.wantRule || a.kind != tc.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tc.wantRule, tc.wantKind)
			}
			if tc.check != nil {
				tc.check(t, b, a)
			}
		})
	}
}

// TestRuleIdleDriftStaysNear checks idle drift over many decisions: moves
// stay within IdleRadius of the arrival point, emotes come from the
// persona's list.
func TestRuleIdleDriftStaysNear(t *testing.T) {
	p := DefaultPersona()
	p.IdleChance = 1
	b, _ := newTestBrain(t, p)
	kinds := map[actionKind]int{}
	for i := range 2000 {
		b.world.Self.Sitting = false
		b.toggledAt = time.Time{}
		rule, a := b.decide(at(time.Duration(i) * time.Second))
		if rule != "idle" {
			t.Fatalf("decision %d: rule %q, want idle", i, rule)
		}
		kinds[a.kind]++
		switch a.kind {
		case actMove:
			to := moveTarget(a.payload)
			if d2 := int64(to.X)*int64(to.X) + int64(to.Y)*int64(to.Y); d2 > int64(p.IdleRadius+1)*int64(p.IdleRadius+1) {
				t.Fatalf("idle move to %+v, farther than %d from the arrival point", to, p.IdleRadius)
			}
		case actEmote:
			id := int32(binary.LittleEndian.Uint32(a.payload[1:]))
			if !slices.Contains(p.Emotes, id) {
				t.Fatalf("emote %d not in the persona's list", id)
			}
		case actSitStand:
		default:
			t.Fatalf("idle action kind %d", a.kind)
		}
	}
	for _, k := range []actionKind{actMove, actEmote, actSitStand} {
		if kinds[k] == 0 {
			t.Errorf("idle drift never chose action kind %d", k)
		}
	}
}
