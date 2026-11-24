package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Dwarven spoil and sweep (docs/strategy.md, Design section 8; rows WF and
// SC): Spoil opens on a fresh target, the server's verdict marks the
// monster, and Sweeper goes out on the corpse of a monster the bot spoiled
// before any pick-up, inside its window.

// skillOther is a cast that is not Spoil: its 139 is no Spoil verdict.
const skillOther = int32(1)

// spoilRows are the rows that spoil: the class, its level and weapon, the
// skills it knows, and what fight casts once the target is spoiled (0 for a
// melee click).
var spoilRows = []struct {
	name         string
	class, level int32
	weapon       string
	skills       []int32
	then         int32
}{
	{"WF", DwarvenFighter.ID, 10, "BLUNT", []int32{skillSpoil, skillSweeper}, 0},
	{"SC", classScavenger, 20, "BLUNT", []int32{skillSpoil, skillStunAttack, skillSweeper}, skillStunAttack},
}

// mustSend sends a at now through the brain's own send path.
func mustSend(t *testing.T, b *Brain, a Action, now time.Time) {
	t.Helper()
	if ok, err := b.send(a, now); !ok || err != nil {
		t.Fatalf("send kind %d skill %d = %v, %v; want sent", a.kind, a.skill, ok, err)
	}
}

// mustHandle hands the brain events at now.
func mustHandle(t *testing.T, b *Brain, now time.Time, evs ...Event) {
	t.Helper()
	if err := b.handle(evs, now); err != nil {
		t.Fatalf("handle %T at %v: %v", evs[0], now.Sub(t0), err)
	}
}

// Times of a spoil: the Spoil cast, its MagicSkillUse, the verdict, and the
// first decision with Spoil and the bot ready again.
var (
	spoilCastAt    = at(time.Second)
	spoilUseAt     = spoilCastAt.Add(100 * time.Millisecond)
	spoilVerdictAt = spoilCastAt.Add(2 * time.Second)
	spoilReadyAt   = spoilCastAt.Add(12 * time.Second)
)

// castSpoil has b open on npcID with Spoil, the server start the cast, then
// hands b the verdict events.
func castSpoil(t *testing.T, b *Brain, verdict ...Event) {
	t.Helper()
	a := b.fight(npcID, spoilCastAt)
	wantCast(t, a, skillSpoil, npcID)
	mustSend(t, b, a, spoilCastAt)
	mustHandle(t, b, spoilUseAt, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skillSpoil, Level: 1, HitTime: 1800 * time.Millisecond, Reuse: 10 * time.Second})
	if len(verdict) > 0 {
		mustHandle(t, b, spoilVerdictAt, verdict...)
	}
}

// TestSpoilOpensThenSweeps checks each spoiling row: Spoil opens on a fresh
// target; after the 612 the bot never casts it on that target again, even
// fresh, and fights on with its next step; after the kill Sweeper goes out
// on the corpse ahead of the loot rule's pick-up of its drop; once the
// sweep's MagicSkillUse is in, the corpse leaves the sweep memory.
func TestSpoilOpensThenSweeps(t *testing.T) {
	for _, row := range spoilRows {
		t.Run(row.name, func(t *testing.T) {
			b := rowBrain(t, row.class, row.level, row.weapon, row.skills...)
			castSpoil(t, b, SystemMessage{ID: msgSpoilSuccess})
			b.opened = 0
			fightCast(t, b, spoilReadyAt, row.then, npcID)

			dead := spoilReadyAt.Add(time.Second)
			mustHandle(t, b, dead, Die{ObjectID: npcID, Sweepable: true},
				DropItem{DropperID: npcID, Item: GroundItem{ObjectID: 7, TemplateID: 57, Pos: Point{X: 20}, Count: 1}})
			rule, a := b.decide(dead)
			if rule != "sweep" {
				t.Fatalf("decide after the kill = %q, want sweep", rule)
			}
			wantCast(t, a, skillSweeper, npcID)
			mustSend(t, b, a, dead)
			// Waiting for the sweep's MagicSkillUse, nothing is picked up.
			if rule, a := b.decide(dead.Add(100 * time.Millisecond)); rule != "sweep" || a.payload != nil {
				t.Errorf("decide with the sweep in flight = %q kind %d, want sweep holding", rule, a.kind)
			}
			swept := dead.Add(200 * time.Millisecond)
			mustHandle(t, b, swept, MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skillSweeper, Level: 1, HitTime: 500 * time.Millisecond})
			if rule, _ := b.decide(swept.Add(time.Second)); rule == "sweep" || len(b.spoils) != 0 {
				t.Errorf("decide after the sweep = %q with spoils %v, want no sweep", rule, b.spoils)
			}
		})
	}
}

// TestSpoilVerdicts checks what each server answer to the Spoil cast does:
// whether fight casts Spoil again at its reuse on the target it already
// opened, and whether its corpse is swept.
func TestSpoilVerdicts(t *testing.T) {
	for _, tt := range []struct {
		name string
		// other sends another cast before the verdict.
		other   bool
		verdict []Event
		respoil bool
		sweep   bool
	}{
		{name: "612 spoiled by the bot", verdict: []Event{SystemMessage{ID: msgSpoilSuccess}}, sweep: true},
		{name: "357 alone: another's spoil", verdict: []Event{SystemMessage{ID: msgAlreadySpoiled}}},
		{name: "612 then 357 stays the bot's", verdict: []Event{SystemMessage{ID: msgSpoilSuccess}, SystemMessage{ID: msgAlreadySpoiled}}, sweep: true},
		{name: "357 then 612 is another's", verdict: []Event{SystemMessage{ID: msgAlreadySpoiled}, SystemMessage{ID: msgSpoilSuccess}}},
		{name: "139 to Spoil retries", verdict: []Event{SystemMessage{ID: msgResisted, Text: [2]string{"Gremlin"}}}, respoil: true},
		{name: "139 to another cast", other: true, verdict: []Event{SystemMessage{ID: msgResisted, Text: [2]string{"Gremlin"}}}},
		{name: "no verdict"},
		{name: "another message", verdict: []Event{SystemMessage{ID: msgCantSeeTarget}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, DwarvenFighter.ID, 10, "BLUNT", skillSpoil, skillSweeper)
			castSpoil(t, b)
			if tt.other {
				mustSend(t, b, Action{kind: actCast, payload: encodeRequestMagicSkillUse(skillOther, false), object: selfID, skill: skillOther}, spoilVerdictAt)
			}
			for _, ev := range tt.verdict {
				mustHandle(t, b, spoilVerdictAt, ev)
			}
			want := int32(0)
			if tt.respoil {
				want = skillSpoil
			}
			fightCast(t, b, spoilReadyAt, want, npcID)

			dead := spoilReadyAt.Add(time.Second)
			mustHandle(t, b, dead, Die{ObjectID: npcID, Sweepable: true})
			if rule, _ := b.decide(dead); (rule == "sweep") != tt.sweep {
				t.Errorf("decide after the kill = %q, want a sweep %v", rule, tt.sweep)
			}
			if !tt.sweep && len(b.spoils) != 0 {
				t.Errorf("spoils after the kill %v, want none", b.spoils)
			}
		})
	}
}

// TestSweepWindow checks the sweep window: twice the corpse's template
// corpseTime from its death (the datapack default 7 s without one), on the
// injected clock; a corpse with no spoil items to sweep (Die.Sweepable
// false) is not swept.
func TestSweepWindow(t *testing.T) {
	for _, tt := range []struct {
		name      string
		corpse    map[int32]time.Duration
		sweepable bool
		window    time.Duration
	}{
		{name: "default corpse time", sweepable: true, window: 14 * time.Second},
		{name: "template corpse time", corpse: map[int32]time.Duration{20001: 10 * time.Second}, sweepable: true, window: 20 * time.Second},
		{name: "not sweepable", sweepable: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, after := range []time.Duration{tt.window - time.Millisecond, tt.window} {
				b := rowBrain(t, DwarvenFighter.ID, 10, "BLUNT", skillSpoil, skillSweeper)
				b.cfg.Travel.corpseTimes = tt.corpse
				castSpoil(t, b, SystemMessage{ID: msgSpoilSuccess})
				dead := spoilReadyAt
				mustHandle(t, b, dead, Die{ObjectID: npcID, Sweepable: tt.sweepable})
				open := tt.sweepable && after < tt.window
				if rule, a := b.decide(dead.Add(after)); (rule == "sweep" && a.kind == actCast) != open {
					t.Errorf("decide %v after the kill = %q kind %d, want a sweep %v", after, rule, a.kind, open)
				}
				if !open && len(b.spoils) != 0 {
					t.Errorf("spoils %v after the window, want none", b.spoils)
				}
			}
		})
	}
}

// TestSweepDropped checks a sweep is dropped on the server's refusal of the
// Sweeper cast (343 not spoiled, 683 not the spoiler's), which also ends the
// wait for its MagicSkillUse, and on the corpse's DeleteObject; a refusal of
// another kind keeps it.
func TestSweepDropped(t *testing.T) {
	for _, tt := range []struct {
		name string
		ev   Event
		drop bool
	}{
		{"343", SystemMessage{ID: msgSweepNotSpoiled}, true},
		{"683", SystemMessage{ID: msgSweepNotAllowed}, true},
		{"DeleteObject", DeleteObject{ObjectID: npcID}, true},
		{"another refusal", SystemMessage{ID: msgNotEnoughMP}, false},
		{"another's DeleteObject", DeleteObject{ObjectID: rowNPC2}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := rowBrain(t, DwarvenFighter.ID, 10, "BLUNT", skillSpoil, skillSweeper)
			castSpoil(t, b, SystemMessage{ID: msgSpoilSuccess})
			dead := spoilReadyAt
			mustHandle(t, b, dead, Die{ObjectID: npcID, Sweepable: true})
			_, a := b.decide(dead)
			wantCast(t, a, skillSweeper, npcID)
			mustSend(t, b, a, dead)
			mustHandle(t, b, dead.Add(50*time.Millisecond), tt.ev)
			_, kept := b.spoils[npcID]
			if kept == tt.drop {
				t.Errorf("sweep kept %v, want dropped %v", kept, tt.drop)
			}
			if _, msg := tt.ev.(SystemMessage); msg && tt.drop && b.castPending(dead.Add(100*time.Millisecond)) {
				t.Error("the refused or gone sweep still waits for its MagicSkillUse")
			}
		})
	}
}

// TestSweepRuleOrder checks the sweep rule comes right after outnumbered.
func TestSweepRuleOrder(t *testing.T) {
	for i, r := range playRules {
		if r.name == "outnumbered" {
			if next := playRules[i+1].name; next != "sweep" {
				t.Fatalf("rule after outnumbered = %q, want sweep", next)
			}
			return
		}
	}
	t.Fatal("no outnumbered rule")
}

// TestIndexCorpseTimes checks only the templates whose corpse time is not
// the default are indexed, and the lookup falls back to the default.
func TestIndexCorpseTimes(t *testing.T) {
	m := &TravelMap{corpseTimes: indexCorpseTimes(map[int32]datapack.NPC{
		1: {CorpseTime: defaultCorpseTime},
		2: {CorpseTime: 30 * time.Second},
	})}
	for _, tt := range []struct {
		npc  int32
		want time.Duration
	}{{1, defaultCorpseTime}, {2, 30 * time.Second}, {3, defaultCorpseTime}} {
		if got := m.corpseTime(tt.npc); got != tt.want {
			t.Errorf("corpseTime(%d) = %v, want %v", tt.npc, got, tt.want)
		}
	}
	if len(m.corpseTimes) != 1 {
		t.Errorf("indexed %v, want template 2 only", m.corpseTimes)
	}
	if got := (*TravelMap)(nil).corpseTime(1); got != defaultCorpseTime {
		t.Errorf("nil map corpseTime = %v, want %v", got, defaultCorpseTime)
	}
}
