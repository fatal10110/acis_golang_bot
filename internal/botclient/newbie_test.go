package botclient

import (
	"testing"
	"time"
)

// guideObj is the Newbie Guide of Talking Island (NPC 30598) once in sight.
const guideObj = int32(0x20000030)

// guidePage is the guide's first page (data/html/script/feature/
// NewbieHelper/30598.htm, reduced to two of its links).
const guidePage = `<html><body>Newbie Guide:<br>` +
	`<a action="bypass -h Quest NewbieHelper NewbieToken">Teleport with a token</a><br>` +
	`<a action="bypass -h Quest NewbieHelper SupportMagic">Receive help</a></body></html>`

// newNewbieBrain is a bot of level at the origin with the default
// persona, no idle drift, and the guide 30598 spawned at (5000, 0) on its
// travel map.
func newNewbieBrain(t *testing.T, level int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	b, _ := newTestBrain(t, p)
	b.world.Self.Level = level
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{30598: {{X: 5000}}}}
	return b
}

// guideInSight puts the guide in sight 200 units away, selected.
func guideInSight(b *Brain) {
	b.world.Apply(NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: Point{X: 200}}, t0)
	b.world.Target = guideObj
}

var supportMagic = encodeRequestBypassToServer(supportMagicBypass)

// TestBrainNewbieBuffs walks a level 8 bot to the guide's spawn, talks to
// the guide and asks for the buffs once per level of the window.
func TestBrainNewbieBuffs(t *testing.T) {
	b := newNewbieBrain(t, 8)
	runTrainerSteps(t, b, []trainerStep{
		{name: "leg to the guide's spawn", at: time.Second, wantRule: "newbie", wantKind: actMove, wantPayload: encodeMoveBackwardToLocation(Point{X: 3000}, Point{})},
		{name: "walking", at: 2 * time.Second, setup: func(b *Brain) {
			b.world.Self.move = motion{from: Point{}, to: Point{X: 3000}, start: at(time.Second), speed: 100}
		}, wantRule: "newbie"},
		{
			name: "guide in sight far off: a leg to it", at: 40 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 3000}) },
			events:   []Event{NPCInfo{ObjectID: guideObj, NPCID: 30598, Pos: Point{X: 5000}}},
			wantRule: "newbie", wantKind: actMove, wantPayload: encodeMoveBackwardToLocation(Point{X: 5000}, Point{X: 3000}),
		},
		{
			name: "close: select", at: 41 * time.Second,
			setup:    func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) },
			wantRule: "newbie", wantKind: actSelect,
		},
		{name: "talk", at: 42 * time.Second, events: []Event{MyTargetSelected{ObjectID: guideObj}}, wantRule: "newbie", wantKind: actTalk},
		{name: "a page of another NPC changes nothing", at: 43 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: npcID, HTML: guidePage}}, wantRule: "newbie"},
		{name: "buff link", at: 44 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: guideObj, HTML: guidePage}}, wantRule: "newbie", wantKind: actBypass, wantPayload: supportMagic},
		{name: "not due again at the same level", at: 45 * time.Second},
		{name: "due after a level-up", at: 50 * time.Second, setup: func(b *Brain) { b.world.Self.Level = 9 }, wantRule: "newbie", wantKind: actTalk},
	})
	if b.newbie.doneLevel != 8 {
		t.Fatalf("doneLevel = %d, want 8", b.newbie.doneLevel)
	}
}

// TestNewbieDue checks when a visit starts.
func TestNewbieDue(t *testing.T) {
	tests := []struct {
		name     string
		level    int32
		setup    func(b *Brain)
		wantKind actionKind // actHold: no visit
	}{
		{name: "below the window", level: 7, setup: guideInSight},
		{name: "first level of the window", level: 8, setup: guideInSight, wantKind: actTalk},
		{name: "last level of the window", level: 24, setup: guideInSight, wantKind: actTalk},
		{name: "above the window", level: 25, setup: guideInSight},
		{name: "asked at this level", level: 10, setup: func(b *Brain) { guideInSight(b); b.newbie.doneLevel = 10 }},
		{name: "persona never asks", level: 10, setup: func(b *Brain) { guideInSight(b); b.persona.Newbie.MaxLevel = 0 }},
		{name: "guide shunned", level: 10, setup: func(b *Brain) {
			guideInSight(b)
			b.newbie.shunned = map[int32]time.Time{30598: at(time.Hour)}
			b.cfg.Travel = nil
		}},
		{name: "a guide in sight beyond reach", level: 10, setup: func(b *Brain) { guideInSight(b); b.persona.Newbie.Reach = 0 }, wantKind: actTalk},
		{name: "spawn in reach", level: 10, wantKind: actMove},
		{name: "spawn beyond reach", level: 10, setup: func(b *Brain) { b.persona.Newbie.Reach = 4999 }},
		{name: "no travel map", level: 10, setup: func(b *Brain) { b.cfg.Travel = nil }},
		{name: "in combat", level: 10, setup: func(b *Brain) { guideInSight(b); b.world.Self.InCombat = true }},
		{name: "another NPC template", level: 10, setup: func(b *Brain) {
			b.world.Apply(NPCInfo{ObjectID: guideObj, NPCID: 30009, Pos: Point{X: 200}}, t0)
			b.cfg.Travel = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newNewbieBrain(t, tt.level)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(at(time.Second))
			if a.kind != tt.wantKind || (tt.wantKind != actHold && rule != "newbie") {
				t.Fatalf("decide = %q %d, want %d", rule, a.kind, tt.wantKind)
			}
		})
	}
}

// TestNewbieGivesUp checks a guide that buffs nothing is shunned.
func TestNewbieGivesUp(t *testing.T) {
	tests := []struct {
		name  string
		steps []trainerStep
	}{
		{
			name: "page without the buff link",
			steps: []trainerStep{
				{name: "talk", at: time.Second, setup: guideInSight, wantRule: "newbie", wantKind: actTalk},
				{name: "no link", at: 2 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: guideObj, HTML: gkPage(guideObj)}}},
			},
		},
		{
			name: "guide not at its spawn",
			steps: []trainerStep{
				{name: "nobody there", at: time.Second, setup: func(b *Brain) { b.world.Self.move = stopAt(Point{X: 4800}) }},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newNewbieBrain(t, 10)
			runTrainerSteps(t, b, tt.steps)
			if b.newbie.v.active() || b.newbie.shunned[30598].IsZero() || b.newbie.doneLevel != 0 {
				t.Fatalf("visit %+v, want it ended, the guide shunned and nothing asked", b.newbie)
			}
		})
	}
}
