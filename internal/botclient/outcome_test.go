package botclient

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

const (
	stayMob1   = int32(0x7000001)
	stayMob2   = int32(0x7000002)
	stayAdena  = int32(0x5100001)
	stayPotion = int32(0x5100002)
)

// stayRun drives a brain through a scripted stay on its fake clock: at is
// the time since t0, the clock moves there first.
type stayRun struct {
	t *testing.T
	b *Brain
}

func (r stayRun) clockTo(d time.Duration) time.Time {
	c := r.b.clock.(*fakeClock)
	c.Advance(t0.Add(d).Sub(c.Now()))
	return c.Now()
}

func (r stayRun) at(d time.Duration, evs ...Event) {
	r.t.Helper()
	now := r.clockTo(d)
	if err := r.b.handle(evs, now); err != nil {
		r.t.Fatalf("at %s: handle: %v", d, err)
	}
}

// arrive runs rule "spot" at d with the bot at its spot's centre.
func (r stayRun) arrive(d time.Duration) {
	r.t.Helper()
	r.b.ruleSpot(r.clockTo(d))
	if !r.b.spot.arrived || !r.b.stay.running() {
		r.t.Fatalf("at %s: arrived %v, stay running %v; want both", d, r.b.spot.arrived, r.b.stay.running())
	}
}

func drainOutcomes(q *OutcomeQueue) []Outcome {
	var out []Outcome
	for {
		select {
		case o := <-q.Records():
			out = append(out, o)
		default:
			return out
		}
	}
}

// newStayBrain is newTripBrain recording its stays on a queue, not yet
// arrived at its spot "s" at the origin, carrying 1000 adena and 10 HP
// potions, with no spot search due for hours.
func newStayBrain(t *testing.T) (*Brain, *OutcomeQueue, stayRun) {
	t.Helper()
	b := newTripBrain(t,
		Item{ObjectID: stayAdena, TemplateID: adenaID, Count: 1000},
		Item{ObjectID: stayPotion, TemplateID: lesserHealingPotion, Count: 10, SubCategory: 5})
	q := NewOutcomeQueue()
	b.cfg.Outcomes = q
	b.spot.arrived = false
	b.spot.retryAt = at(10 * time.Hour)
	b.world.Self.Exp = 5000
	return b, q, stayRun{t, b}
}

// sameOutcome compares two records field by field through their JSON,
// floats to 1e-9.
func sameOutcome(t *testing.T, got, want Outcome) {
	t.Helper()
	asMap := func(o Outcome) map[string]any {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	g, w := asMap(got), asMap(want)
	for k, wv := range w {
		gv, ok := g[k]
		gf, gok := gv.(float64)
		wf, wok := wv.(float64)
		switch {
		case !ok:
			t.Errorf("%s missing, want %v", k, wv)
		case gok && wok:
			if math.Abs(gf-wf) > 1e-9 {
				t.Errorf("%s = %v, want %v", k, gv, wv)
			}
		case gv != wv:
			t.Errorf("%s = %v, want %v", k, gv, wv)
		}
	}
	for k, gv := range g {
		if _, ok := w[k]; !ok {
			t.Errorf("%s = %v, want absent", k, gv)
		}
	}
}

// TestOutcomeStay checks the record of a scripted stay: two kills, one
// shared with another attacker, HP drops (a heal not subtracted), a potion
// drunk in combat, experience and adena gained, then a teleport away (the
// time, fighting, experience and adena spent away not counted) and a
// return to the same spot that goes on in the same record, until the spot
// is given up.
func TestOutcomeStay(t *testing.T) {
	b, q, r := newStayBrain(t)
	self := b.world.Self.ObjectID
	r.arrive(0)
	r.at(0, AutoAttackStart{ObjectID: self}, NPCInfo{ObjectID: stayMob1, NPCID: 20001, Attackable: true, Pos: Point{X: 50}},
		NPCInfo{ObjectID: stayMob2, NPCID: 20001, Attackable: true, Pos: Point{X: -50}})
	r.at(10*time.Second, Attack{AttackerID: self, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}})
	r.at(15*time.Second, Attack{AttackerID: self, Hits: []AttackHit{{TargetID: stayMob1, Damage: 10}}})
	r.at(20*time.Second, StatusUpdate{ObjectID: self, Set: StatusHP, HP: 70})
	r.at(25*time.Second, InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: Item{ObjectID: stayPotion, TemplateID: lesserHealingPotion, Count: 9, SubCategory: 5}}}},
		StatusUpdate{ObjectID: self, Set: StatusHP, HP: 90})
	r.at(40*time.Second, Die{ObjectID: stayMob1})
	// A missed swing is no landed hit.
	r.at(45*time.Second, Attack{AttackerID: self, Hits: []AttackHit{{TargetID: stayMob2, Flags: HitMiss}}})
	r.at(50*time.Second, Attack{AttackerID: self, Hits: []AttackHit{{TargetID: stayMob2, Damage: 10}}})
	r.at(55*time.Second, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: stayMob2, Damage: 10}}})
	r.at(60*time.Second, StatusUpdate{ObjectID: self, Set: StatusHP, HP: 80})
	r.at(70*time.Second, Die{ObjectID: stayMob2})
	r.at(80*time.Second, AutoAttackStop{ObjectID: self})
	b.world.Self.Exp = 5600
	r.at(90*time.Second, InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: Item{ObjectID: stayAdena, TemplateID: adenaID, Count: 1150}}}})
	r.at(120*time.Second, TeleportToLocation{ObjectID: self, Pos: Point{X: 4000}, Fast: true})
	if b.spot.arrived || !b.stay.open || b.stay.running() {
		t.Fatalf("after the teleport: arrived %v, open %v, running %v; want false, true, false", b.spot.arrived, b.stay.open, b.stay.running())
	}
	// Away: none of this counts.
	b.world.Self.Exp = 6000
	r.at(200*time.Second, InventoryUpdate{Changes: []ItemChange{{State: ItemModified, Item: Item{ObjectID: stayAdena, TemplateID: adenaID, Count: 1050}}}})
	r.at(210*time.Second, AutoAttackStart{ObjectID: self}, StatusUpdate{ObjectID: self, Set: StatusHP, HP: 50})
	r.at(240*time.Second, AutoAttackStop{ObjectID: self})
	r.at(300*time.Second, TeleportToLocation{ObjectID: self, Pos: Point{}, Fast: true})
	r.arrive(300 * time.Second)
	if got := drainOutcomes(q); len(got) != 0 {
		t.Fatalf("records before the spot is given up: %+v", got)
	}
	b.world.Self.Exp = 6400
	r.at(330*time.Second, StatusUpdate{ObjectID: self, Set: StatusHP, HP: 40})
	r.clockTo(360 * time.Second)
	b.leaveSpot()

	got := drainOutcomes(q)
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1: %+v", len(got), got)
	}
	combat := 80.0 / 60
	sameOutcome(t, got[0], Outcome{
		Start: t0, Spot: "s", Level: 5, Role: b.persona.Role.String(), Margin: b.build().Kit.SpotMargin, Party: 1,
		Minutes: 3, Combat: combat, Exp: 1000, Adena: 150,
		HPLost: 50, HPPerCombatMinute: 50 / combat, Potions: 1,
		Kills: 2, Shared: 1, KillSeconds: 25, SoloKillSeconds: 30,
	})
	if b.stay.open {
		t.Error("stay still open after leaveSpot")
	}
}

// TestOutcomeStayKinds checks one record per stay of each kind: a death
// at the spot, a stay beside a full spot, a party leader's stay with its
// party's size, and a session that ends at the spot (the spot rule's stop).
func TestOutcomeStayKinds(t *testing.T) {
	tests := []struct {
		name  string
		setup func(b *Brain)
		end   func(r stayRun)
		want  func(o Outcome) string
	}{
		{
			"death",
			func(*Brain) {},
			func(r stayRun) { r.at(time.Minute, Die{ObjectID: r.b.world.Self.ObjectID}) },
			func(o Outcome) string {
				if o.Deaths != 1 || o.Minutes != 1 {
					return "want 1 death after 1 minute"
				}
				return ""
			},
		},
		{
			"beside a full spot",
			func(b *Brain) {
				b.cfg.Spots.releaseN(0, 1)
				b.spot.held = false
				if _, ok := b.cfg.Spots.farmBesideN([]int{0}, 1); !ok {
					panic("no beside")
				}
				b.spot.beside = true
			},
			func(r stayRun) { r.clockTo(time.Minute); r.b.leaveSpot() },
			func(o Outcome) string {
				if !o.Beside || o.Earning || o.Lower {
					return "want beside only"
				}
				return ""
			},
		},
		{
			"party leader",
			func(b *Brain) {
				b.world.Party = Party{LeaderID: b.world.Self.ObjectID, Members: map[int32]*PartyMate{0x10000201: {}, 0x10000202: {}}}
			},
			func(r stayRun) { r.clockTo(time.Minute); r.b.leaveSpot() },
			func(o Outcome) string {
				if o.Party != 3 {
					return "want party 3"
				}
				return ""
			},
		},
		{
			"session end",
			func(*Brain) {},
			func(r stayRun) {
				r.clockTo(2 * time.Minute)
				for _, pr := range playRules {
					if pr.name == "spot" {
						pr.stop(r.b)
					}
				}
			},
			func(o Outcome) string {
				if o.Minutes != 2 || o.Deaths != 0 || o.Party != 1 {
					return "want 2 minutes, no death, solo"
				}
				return ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, q, r := newStayBrain(t)
			tt.setup(b)
			r.arrive(0)
			tt.end(r)
			got := drainOutcomes(q)
			if len(got) != 1 {
				t.Fatalf("records = %d, want 1: %+v", len(got), got)
			}
			if msg := tt.want(got[0]); msg != "" {
				t.Errorf("%+v: %s", got[0], msg)
			}
		})
	}
}

// TestOutcomeModel checks the model's times on the record: packFight's at
// arrival for a row with a margin, absent with the margin off.
func TestOutcomeModel(t *testing.T) {
	for _, margin := range []float64{0, -1} {
		b := newSquireBrain(t, squire14, 13)
		b.persona.Spots.Margin = margin
		q := NewOutcomeQueue()
		b.cfg.Outcomes, b.cfg.Spots = q, NewSpotRegistry(nil, 1)
		b.cfg.Spots.spots = append(b.cfg.Spots.spots, tiWolves)
		b.spot.idx, b.spot.beside = 0, true
		b.stayArrive(t0)
		b.stayEnd(t0)
		got := drainOutcomes(q)
		if len(got) != 1 {
			t.Fatalf("margin %v: records = %d, want 1", margin, len(got))
		}
		o := got[0]
		if margin < 0 {
			if o.ModelKill != nil || o.ModelDie != nil || o.Margin != 0 {
				t.Errorf("margin off: model %v, %v, margin %v; want none", o.ModelKill, o.ModelDie, o.Margin)
			}
			continue
		}
		bd := b.build()
		kill, die, ok := b.packFight(&tiWolves, &bd, bd.Kit.SpotMargin, &crew{})
		if !ok || o.ModelKill == nil || o.ModelDie == nil || *o.ModelKill != kill || *o.ModelDie != die || o.Margin != bd.Kit.SpotMargin {
			t.Errorf("model %v, %v, margin %v; want %v, %v, %v", o.ModelKill, o.ModelDie, o.Margin, kill, die, bd.Kit.SpotMargin)
		}
	}
}

// TestOutcomeNoNames checks a record of a bot with an account, a character
// name and chat around it names none of them.
func TestOutcomeNoNames(t *testing.T) {
	b, q, r := newStayBrain(t)
	b.cfg.account = "acctsecret"
	r.arrive(0)
	r.at(time.Second, CreatureSay{ObjectID: pcID, Name: "other", Text: "whisper words"},
		CreatureSay{ObjectID: b.world.Self.ObjectID, Name: b.world.Self.Name, Text: "own words"})
	r.clockTo(time.Minute)
	b.leaveSpot()
	got := drainOutcomes(q)
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"acctsecret", b.world.Self.Name, "other", "whisper words", "own words"} {
		if strings.Contains(string(raw), s) {
			t.Errorf("record %s contains %q", raw, s)
		}
	}
}

// TestOutcomeOff checks a brain with no queue opens no stay.
func TestOutcomeOff(t *testing.T) {
	b, _, r := newStayBrain(t)
	b.cfg.Outcomes = nil
	b.ruleSpot(r.clockTo(0))
	if !b.spot.arrived || b.stay.open {
		t.Fatalf("arrived %v, stay open %v; want true, false", b.spot.arrived, b.stay.open)
	}
	b.leaveSpot()
}

// TestOutcomeQueueFull checks a full queue drops and counts a record and
// never blocks, its writer stopped.
func TestOutcomeQueueFull(t *testing.T) {
	q := NewOutcomeQueue()
	for i := range OutcomeQueueLen + 5 {
		q.Put(Outcome{Level: int32(i)})
	}
	if got := q.Dropped(); got != 5 {
		t.Errorf("dropped = %d, want 5", got)
	}
	got := drainOutcomes(q)
	if len(got) != OutcomeQueueLen || got[0].Level != 0 || got[len(got)-1].Level != OutcomeQueueLen-1 {
		t.Errorf("queued %d, want the first %d", len(got), OutcomeQueueLen)
	}
}
