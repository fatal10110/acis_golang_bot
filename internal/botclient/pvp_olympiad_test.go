package botclient

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

const (
	// managerObj is the Grand Olympiad Manager (NPC 31688) once in sight.
	managerObj = int32(0x20000060)
	// olyCardinal is a third profession (Cardinal, of Bishop, of Cleric, of
	// Mystic): the only kind the manager registers.
	olyCardinal = int32(97)
	// olyShot is a soulshot template the stadium's strip turns off.
	olyShot = int32(1463)
)

// olyPage is a manager page (data/html/olympiad) with one link per command.
func olyPage(cmds ...string) NpcHtmlMessage {
	var b strings.Builder
	b.WriteString(`<html><body>Grand Olympiad Manager:<br>`)
	for _, c := range cmds {
		b.WriteString(`<a action="bypass -h npc_` + strconv.Itoa(int(managerObj)) + `_` + c + `">link</a><br>`)
	}
	b.WriteString(`</body></html>`)
	return NpcHtmlMessage{ObjectID: managerObj, HTML: b.String()}
}

// The manager's pages on the way to the registration and back.
var (
	nobleMain = olyPage("Chat 10", olympiadUnregister, "Chat 1")
	noble10   = olyPage("Chat 11", "Chat 12", "OlympiadNoble 2", "OlympiadNoble 3", "Chat 0")
	noble12   = olyPage(olympiadRegister)
)

// olyOpen sets p's window to open an hour before t0 for six hours.
func olyOpen(p *OlympiadPolicy) {
	p.Zone, p.Start, p.Period = time.UTC, 11*time.Hour, 6*time.Hour
}

// olyBrain is a noble Cardinal with Olympiad on, OlympiadPolicy.ThirdClass
// on and the window open, a manager spawned at (5000, 0) on its travel map.
func olyBrain(t *testing.T) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.PvP.Olympiad, p.Olympiad.ThirdClass = true, true
	olyOpen(&p.Olympiad)
	b, _ := newTestBrain(t, p)
	s := &b.world.Self
	s.ClassID, s.BaseClass, s.Level, s.Noble = olyCardinal, olyCardinal, 78, true
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{olympiadManager: {{X: 5000}}}, trainers: testTrainers}
	return walkOn(t, b)
}

// managerInSight puts the manager in sight 200 units away, selected.
func managerInSight(b *Brain) {
	n := liveMaster
	n.ObjectID, n.NPCID, n.Pos = managerObj, olympiadManager, Point{X: 200}
	b.world.Apply(n, t0)
	b.world.Target = managerObj
}

func managerBypass(cmd string) []byte { return masterBypass(managerObj, cmd) }

// registerSteps take olyBrain through the manager's pages to the
// registration, from d on, one second apart.
func registerSteps(d time.Duration) []trainerStep {
	s := func(i int) time.Duration { return d + time.Duration(i)*time.Second }
	return []trainerStep{
		{name: "talk", at: s(0), setup: managerInSight, wantRule: "olympiad entry", wantKind: actTalk},
		{name: "wait for the page", at: s(1), wantRule: "olympiad entry"},
		{name: "participate", at: s(2), events: []Event{nobleMain}, wantRule: "olympiad entry", wantKind: actBypass, wantPayload: managerBypass("Chat 10")},
		{name: "wait for noble_10", at: s(3), wantRule: "olympiad entry"},
		{name: "class-based games", at: s(4), events: []Event{noble10}, wantRule: "olympiad entry", wantKind: actBypass, wantPayload: managerBypass("Chat 12")},
		{name: "wait for noble_12", at: s(5), wantRule: "olympiad entry"},
		{name: "register", at: s(6), events: []Event{noble12}, wantRule: "olympiad entry", wantKind: actBypass, wantPayload: managerBypass(olympiadRegister)},
		{name: "wait for the answer", at: s(7), wantRule: "olympiad entry"},
	}
}

// TestOlympiadRegister checks the noblesse and the registration
// (docs/pvp/olympiad.md "Noblesse", "Registering", "While registered").
func TestOlympiadRegister(t *testing.T) {
	t.Run("not noble: become_noble on the class master visit, then the transfer", func(t *testing.T) {
		b := newClassBrain(t, classHumanMystic, 20)
		b.persona.PvP.Olympiad = true
		b.train.doneLevel = 20
		runTrainerSteps(t, b, []trainerStep{
			{name: "talk", at: time.Second, setup: masterInSight, wantRule: "classchange", wantKind: actTalk},
			{name: "noblesse first", at: 2 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, becomeNobleCommand)},
			{name: "wait for the noble page", at: 3 * time.Second, wantRule: "classchange"},
			{
				name: "the noble page has no link: talk again", at: 4 * time.Second,
				events:   []Event{UserInfo{ObjectID: selfID, Name: "bot", ClassID: classHumanMystic, BaseClass: classHumanMystic, Level: 20, Noble: true}, NpcHtmlMessage{ObjectID: masterObj, HTML: "<html><body>Congratulations on becoming a Noble !</body></html>"}},
				wantRule: "classchange", wantKind: actTalk,
			},
			{name: "the transfer's menu", at: 5 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, "1stClass")},
		})
		if !b.world.Self.Noble || !b.class.nobleAsked {
			t.Fatalf("noble %v asked %v, want both", b.world.Self.Noble, b.class.nobleAsked)
		}
	})

	t.Run("not noble, no profession due: a visit for noblesse alone", func(t *testing.T) {
		b := newClassBrain(t, classCleric, 30)
		b.persona.PvP.Olympiad = true
		runTrainerSteps(t, b, []trainerStep{
			{name: "talk", at: time.Second, setup: masterInSight, wantRule: "classchange", wantKind: actTalk},
			{name: "noblesse", at: 2 * time.Second, events: []Event{masterPage(masterObj)}, wantRule: "classchange", wantKind: actBypass, wantPayload: masterBypass(masterObj, becomeNobleCommand)},
		})
		b.handleOne(t, NpcHtmlMessage{ObjectID: masterObj, HTML: "<html><body>Congratulations</body></html>"}, at(3*time.Second))
		if b.class.v.active() {
			t.Fatal("visit still active after the noble page, want it over")
		}
		// Noble or not, become_noble never goes out twice.
		if rule, _ := b.decide(at(4 * time.Second)); rule == "classchange" {
			t.Fatal("a second visit, want one become_noble per bot")
		}
	})

	for _, tc := range []struct {
		name  string
		class int32
		setup func(*Brain)
	}{
		{name: "Olympiad off", class: classCleric, setup: func(b *Brain) { b.persona.PvP.Olympiad = false }},
		{name: "already noble", class: classCleric, setup: func(b *Brain) { b.world.Self.Noble = true }},
		{name: "a third profession, ThirdClass off", class: olyCardinal, setup: func(b *Brain) { b.persona.Olympiad.ThirdClass = false }},
	} {
		t.Run("no noblesse: "+tc.name, func(t *testing.T) {
			b := newClassBrain(t, tc.class, 30)
			b.persona.PvP.Olympiad = true
			if tc.setup != nil {
				tc.setup(b)
			}
			masterInSight(b)
			if rule, _ := b.decide(at(time.Second)); rule == "classchange" {
				t.Fatal("a class master visit, want none")
			}
		})
	}

	t.Run("registers once, waits 30 minutes in town, unregisters and farms", func(t *testing.T) {
		b := olyBrain(t)
		runTrainerSteps(t, b, registerSteps(time.Second))
		runTrainerSteps(t, b, []trainerStep{
			{name: "registered: it waits", at: 10 * time.Second, events: []Event{SystemMessage{ID: msgRegisteredClassed}}, wantRule: "olympiad wait"},
			{name: "still waiting", at: 29 * time.Minute, wantRule: "olympiad wait"},
			{name: "30 minutes: back to the manager", at: 31 * time.Minute, setup: managerInSight, wantRule: "olympiad entry", wantKind: actTalk},
			{name: "unregister", at: 31*time.Minute + time.Second, events: []Event{nobleMain}, wantRule: "olympiad entry", wantKind: actBypass, wantPayload: managerBypass(olympiadUnregister)},
		})
		b.handleOne(t, SystemMessage{ID: msgUnregistered}, at(31*time.Minute+2*time.Second))
		if rule, _ := b.decide(at(31*time.Minute + 3*time.Second)); rule == "olympiad wait" || rule == "olympiad entry" {
			t.Fatalf("decide = %q after unregistering, want the persona's farming", rule)
		}
		if b.oly.tries != 1 || !b.oly.registered.IsZero() {
			t.Fatalf("tries %d registered %v, want 1 and none", b.oly.tries, b.oly.registered)
		}
		// The second try waits a Wait of farming.
		if b.mayRegister(at(40 * time.Minute)) {
			t.Fatal("registers again 9 minutes after unregistering, want a Wait of farming first")
		}
		if !b.mayRegister(at(62 * time.Minute)) {
			t.Fatal("no second try a Wait later, want one")
		}
	})

	for _, tc := range []struct {
		name  string
		setup func(*Brain)
	}{
		{name: "9 minutes before the window's end", setup: func(b *Brain) { b.persona.Olympiad.Start = 6*time.Hour + 9*time.Minute }},
		{name: "before the window", setup: func(b *Brain) { b.persona.Olympiad.Start = 12*time.Hour + time.Minute }},
		{name: "in a party", setup: func(b *Brain) { b.world.Party.LeaderID = pcID }},
		{name: "overweight", setup: func(b *Brain) { b.world.Self.Load, b.world.Self.MaxLoad = 85000, 100000 }},
		{name: "inventory 80% full", setup: func(b *Brain) {
			b.world.Self.InvLimit = 5
			b.world.Inventory.Items = make(map[int32]Item)
			for i := range int32(4) {
				b.world.Inventory.Items[100+i] = Item{ObjectID: 100 + i, TemplateID: 57}
			}
		}},
		{name: "on a subclass", setup: func(b *Brain) { b.world.Self.BaseClass = classHumanMystic }},
		{name: "a second profession (the server registers third ones only)", setup: func(b *Brain) {
			b.world.Self.ClassID, b.world.Self.BaseClass = classBishop, classBishop
		}},
		{name: "a third profession with ThirdClass off", setup: func(b *Brain) { b.persona.Olympiad.ThirdClass = false }},
		{name: "not noble", setup: func(b *Brain) { b.world.Self.Noble = false }},
		{name: "Olympiad off", setup: func(b *Brain) { b.persona.PvP.Olympiad = false }},
		{name: "both tries spent today", setup: func(b *Brain) { b.oly.day, b.oly.tries = "2026-11-16", 2 }},
	} {
		t.Run("no registration: "+tc.name, func(t *testing.T) {
			b := olyBrain(t)
			managerInSight(b)
			tc.setup(b)
			if rule, _ := b.decide(at(time.Second)); rule == "olympiad entry" {
				t.Fatal("a manager visit, want none")
			}
		})
	}

	for _, tc := range []struct {
		name string
		ev   Event
	}{
		{name: "1651 not running", ev: SystemMessage{ID: msgOlympiadNotRunning}},
		{name: "1803 closing", ev: SystemMessage{ID: msgRequestCannotBeMade}},
		{name: "1691 overweight", ev: SystemMessage{ID: msgInventoryTooFull}},
		{name: "the no-points page", ev: olyPage("Chat 10")},
	} {
		t.Run("refused by "+tc.name+": no retry that day", func(t *testing.T) {
			b := olyBrain(t)
			runTrainerSteps(t, b, registerSteps(time.Second))
			b.handleOne(t, tc.ev, at(9*time.Second))
			if !b.oly.stopped || b.oly.v.active() {
				t.Fatalf("stopped %v visit %v, want stopped with the visit over", b.oly.stopped, b.oly.v.active())
			}
			managerInSight(b)
			if rule, _ := b.decide(at(2 * time.Hour)); rule == "olympiad entry" || rule == "olympiad wait" {
				t.Fatalf("decide = %q later that day, want no retry", rule)
			}
			// The next day it may register again.
			b.persona.Olympiad.Start = 0
			b.persona.Olympiad.Period = 24 * time.Hour
			if rule, a := b.decide(at(13 * time.Hour)); rule != "olympiad entry" || a.kind != actTalk {
				t.Fatalf("decide = %q %d the next day, want a manager visit", rule, a.kind)
			}
		})
	}

	t.Run("an unanswered registration stops it for the day", func(t *testing.T) {
		b := olyBrain(t)
		runTrainerSteps(t, b, registerSteps(time.Second))
		b.decide(at(time.Minute))
		if !b.oly.stopped || b.oly.tries != 1 {
			t.Fatalf("stopped %v tries %d, want stopped after one try", b.oly.stopped, b.oly.tries)
		}
	})
}

// TestOlympiadWindow checks the window registration opens in
// (registration.go RegisterClosing): 18:00 to 23:50 server time.
func TestOlympiadWindow(t *testing.T) {
	p := defaultOlympiadPolicy()
	day := time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at   time.Duration
		open bool
	}{
		{at: 17*time.Hour + 59*time.Minute},
		{at: 18 * time.Hour, open: true},
		{at: 23*time.Hour + 49*time.Minute, open: true},
		{at: 23*time.Hour + 50*time.Minute},
		{at: 24 * time.Hour},
		{at: 12 * time.Hour},
	} {
		if got := p.open(day.Add(tc.at)); got != tc.open {
			t.Errorf("open at %v = %v, want %v", tc.at, got, tc.open)
		}
	}
}

// olyFoe is the drawn opponent: a bot (markedName) that never hit the bot.
func olyFoe(p Point) CharInfo {
	return CharInfo{ObjectID: pcID, Name: markedName, ClassID: 92, Pos: p, RunSpeed: 124, Running: true, CollisionRadius: 9}
}

// olyStadium plays the call and the stadium's entry: 1492, the forced
// teleport, ExOlympiadMode, the opponent in sight and the strip's shots
// off.
func olyStadium(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	for _, ev := range []Event{
		SystemMessage{ID: msgStadiumInS1Seconds, Number: 30},
		TeleportToLocation{ObjectID: selfID, Pos: Point{X: 400}},
		ExOlympiadMode{Mode: botwire.OlympiadModeSide1},
		SystemMessage{ID: msgEnteredCombatZone},
		olyFoe(Point{X: 400, Y: 300}),
		ExAutoSoulShot{TemplateID: olyShot},
	} {
		b.handleOne(t, ev, now)
	}
}

// olyStart plays the fight's start: both competitors' match windows, then
// 1496.
func olyStart(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	for _, ev := range []Event{
		ExOlympiadUserInfo{Side: 1, ObjectID: selfID, Name: "bot", ClassID: olyCardinal},
		ExOlympiadUserInfo{Side: 2, ObjectID: pcID, Name: markedName, ClassID: 92},
		SystemMessage{ID: msgGameStarted},
	} {
		b.handleOne(t, ev, now)
	}
}

// TestOlympiadMatch checks the match (docs/pvp/olympiad.md "In the match",
// MayFight rule 0).
func TestOlympiadMatch(t *testing.T) {
	t.Run("the match from the call to its end", func(t *testing.T) {
		b := huntBrain(t)
		b.persona.PvP.Olympiad, b.persona.Olympiad.ThirdClass = true, true
		b.pvp.siege.fields = map[int]time.Time{3: t0.Add(siegeLength)}
		olyStadium(t, b, t0)
		if !b.inOlympiad(at(time.Second)) {
			t.Fatal("not in a match after the stadium's entry")
		}
		if _, ok := b.pvp.siege.fields[3]; !ok || len(b.pvp.siege.fields) != 1 {
			t.Fatalf("sieges %v after 283 in the stadium, want 3 kept: the stadium's, not a siege's", b.pvp.siege.fields)
		}
		if b.mayFight(pcID, at(time.Second)) {
			t.Fatal("MayFight the opponent before the fight, want no target")
		}
		if !b.shotOff[olyShot] {
			t.Fatal("the strip's shot not marked to turn on again")
		}
		if rule, a := b.decide(at(time.Second)); rule != "olympiad wait" && rule != "shots" && rule != "rebuff" && rule != "rest" || a.kind == actMove {
			t.Fatalf("decide = %q %d in the wait, want the wait held (rest, buffs and shots allowed)", rule, a.kind)
		}
		olyStart(t, b, at(time.Minute))
		now := at(time.Minute + time.Second)
		if !b.mayFight(pcID, now) {
			t.Fatal("MayFight the drawn opponent (a bot that never hit) false, want true")
		}
		if k := b.pvp.opponents[pcID].Kind; k != FightOlympiad || k.String() != "olympiad" {
			t.Fatalf("fight kind %v, want olympiad", k)
		}
		if b.forced(pcID, now) {
			t.Fatal("forced attack on the opponent, want plain Action")
		}
		b.handleOne(t, CharInfo{ObjectID: pcID + 1, Name: "flagged", Pos: Point{X: 500}, PvPFlag: 1, RunSpeed: 124}, now)
		b.handleOne(t, hitSelf(pcID+1, 5), now)
		if b.mayFight(pcID+1, now) {
			t.Fatal("MayFight a flagged attacker in the match, want only the opponent")
		}
		if !fightsWith(b, "olympiad", pcID, now) {
			t.Fatal("not fighting the opponent by rule olympiad")
		}
		b.world.Self.HP, b.world.Self.CP = 5, 0
		if rule, a := b.decide(at(time.Minute + 2*time.Second)); a.kind == actMove || rule != "olympiad" && rule != "critical" {
			t.Fatalf("decide = %q %d at 5 HP, want the fight on, no flee", rule, a.kind)
		}
		if b.breaksOff(pcID, nil, stepState{}, now) {
			t.Fatal("breaks off in the match, want none")
		}
		for _, ev := range []Event{SystemMessage{ID: msgS1WonTheGame, Text: [2]string{"bot"}}, SystemMessage{ID: msgS1GainedS2Points, Text: [2]string{"bot"}, Number: 6}, ExOlympiadMode{}} {
			b.handleOne(t, ev, at(5*time.Minute))
		}
		if b.mayFight(pcID, at(5*time.Minute)) {
			t.Fatal("MayFight the opponent after the result, want none")
		}
		b.handleOne(t, TeleportToLocation{ObjectID: selfID}, at(6*time.Minute))
		b.handleOne(t, SystemMessage{ID: msgLeftCombatZone}, at(6*time.Minute))
		b.handleOne(t, ExOlympiadMatchEnd{}, at(6*time.Minute))
		if b.inOlympiad(at(6 * time.Minute)) {
			t.Fatal("still in a match after ExOlympiadMatchEnd")
		}
		if rule, _ := b.decide(at(6*time.Minute + time.Second)); rule == "olympiad wait" || rule == "olympiad" {
			t.Fatalf("decide = %q after the match, want the persona", rule)
		}
	})

	for _, tc := range []struct {
		name  string
		evs   []Event
		after time.Duration
		in    bool
	}{
		{name: "1492 alone holds the bot", evs: []Event{SystemMessage{ID: msgStadiumInS1Seconds, Number: 30}}, after: 20 * time.Second, in: true},
		{name: "a call with no stadium lapses", evs: []Event{SystemMessage{ID: msgStadiumInS1Seconds, Number: 30}}, after: 3 * time.Minute},
		{name: "the opponent unfit before the teleport cancels", evs: []Event{SystemMessage{ID: msgStadiumInS1Seconds, Number: 30}, SystemMessage{ID: msgOpponentUnfit}}, after: time.Second},
		{name: "a lost ExOlympiadMatchEnd lapses", evs: []Event{SystemMessage{ID: msgStadiumInS1Seconds}, ExOlympiadMode{Mode: botwire.OlympiadModeSide2}}, after: 16 * time.Minute},
		{name: "a spectator's mode is no match", evs: []Event{ExOlympiadMode{Mode: botwire.OlympiadModeObserver}}, after: time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := huntBrain(t)
			b.persona.PvP.Olympiad = true
			for _, ev := range tc.evs {
				b.handleOne(t, ev, t0)
			}
			if got := b.inOlympiad(at(tc.after)); got != tc.in {
				t.Fatalf("in a match %v, want %v", got, tc.in)
			}
		})
	}

	t.Run("a draw ends the registration", func(t *testing.T) {
		b := olyBrain(t)
		b.oly.registered = t0
		b.handleOne(t, SystemMessage{ID: msgStadiumInS1Seconds, Number: 30}, at(time.Second))
		if !b.oly.registered.IsZero() {
			t.Fatal("still registered after the call, want the waiting list left")
		}
	})

	t.Run("283 outside a match is a siege's", func(t *testing.T) {
		b := huntBrain(t)
		b.cfg.Travel.zones.Sieges = []datapack.Zone{box(-500, -500, 500, 500)}
		b.handleOne(t, SystemMessage{ID: msgEnteredCombatZone}, t0)
		if _, ok := b.pvp.siege.fields[1]; !ok {
			t.Fatalf("sieges %v, want combatZoneSeen to run", b.pvp.siege.fields)
		}
	})
}
