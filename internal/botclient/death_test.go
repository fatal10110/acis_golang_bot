package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

func TestWorldResurrectOffer(t *testing.T) {
	offer := ConfirmDlg{MessageID: confirmResurrect, Name: "cleric"}
	tests := []struct {
		name   string
		events []Event
		want   ResurrectOffer
	}{
		{"offer to the dead bot", []Event{Die{ObjectID: selfID}, offer}, ResurrectOffer{Open: true, MessageID: confirmResurrect, Reviver: "cleric"}},
		{"restore offer", []Event{Die{ObjectID: selfID}, ConfirmDlg{MessageID: confirmRestore, RequesterID: 3}}, ResurrectOffer{Open: true, MessageID: confirmRestore, RequesterID: 3}},
		{"alive bot", []Event{offer}, ResurrectOffer{}},
		{"other question", []Event{Die{ObjectID: selfID}, ConfirmDlg{MessageID: 1842, Name: "GM"}}, ResurrectOffer{}},
		{"revive closes it", []Event{Die{ObjectID: selfID}, offer, Revive{ObjectID: selfID}}, ResurrectOffer{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, ev := range tt.events {
				w.Apply(ev, t0)
			}
			if w.Prompts.Resurrect != tt.want {
				t.Fatalf("Resurrect = %+v, want %+v", w.Prompts.Resurrect, tt.want)
			}
		})
	}
}

// deathStep decides at now and sends the decision, returning it.
func deathStep(t *testing.T, b *Brain, now time.Time) (string, Action) {
	t.Helper()
	name, a := b.decide(now)
	if a.kind != actHold {
		if _, err := b.send(a, now); err != nil {
			t.Fatal(err)
		}
	}
	return name, a
}

// deathApply hands events to the brain at now.
func deathApply(t *testing.T, b *Brain, now time.Time, events ...Event) {
	t.Helper()
	if err := b.handle(events, now); err != nil {
		t.Fatal(err)
	}
}

func TestRuleDeadRecover(t *testing.T) {
	village := Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}
	offer := ConfirmDlg{MessageID: confirmResurrect, Name: "cleric"}
	restart := botwire.EncodeRequestRestartPoint(restartVillage)
	tests := []struct {
		name    string
		recover func(*RecoverPolicy)
		party   bool
		events  []Event
		at      time.Duration // after the death at t0
		want    []byte        // nil holds
	}{
		{"offer accepted", nil, true, []Event{offer}, time.Second, botwire.EncodeDlgAnswer(confirmResurrect, true, 0)},
		{"offer declined", func(r *RecoverPolicy) { r.AcceptResurrection = false }, false, []Event{offer}, time.Second, botwire.EncodeDlgAnswer(confirmResurrect, false, 0)},
		{"party member waits", nil, true, nil, 2*time.Minute - time.Millisecond, nil},
		{"party member restarts after the wait", nil, true, nil, 2 * time.Minute, restart},
		{"alone restarts at once", nil, false, nil, time.Second, restart},
		{"no wait restarts at once", func(r *RecoverPolicy) { r.ResurrectWait = 0 }, true, nil, time.Second, restart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersona()
			if tt.recover != nil {
				tt.recover(&p.Recover)
			}
			b, _ := newTestBrain(t, p)
			if tt.party {
				deathApply(t, b, t0, PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}})
			}
			deathApply(t, b, t0, append([]Event{village}, tt.events...)...)
			name, a := b.decide(t0.Add(tt.at))
			if name != "dead" || !bytes.Equal(a.payload, tt.want) {
				t.Fatalf("decide = %q % x, want dead % x", name, a.payload, tt.want)
			}
		})
	}
}

// TestDeadBotReturnsToLeader kills a party member, which takes the offered
// resurrection or, offered none, restarts in town after the wait and then
// follows its leader back.
func TestDeadBotReturnsToLeader(t *testing.T) {
	p := DefaultPersona()
	b, out := newTestBrain(t, p)
	walkOn(t, b)
	b.persona.Follow = defaultFollowPolicy()
	leaderAt := Point{X: 6000, Y: 2000}
	deathApply(t, b, t0,
		PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}},
		PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: gmID, Pos: leaderAt}}},
		Die{ObjectID: selfID, Restart: RestartPoints{Village: true}},
		ConfirmDlg{MessageID: confirmResurrect, Name: "GM"},
	)
	if name, a := deathStep(t, b, at(time.Second)); name != "dead" || a.kind != actDlgAnswer {
		t.Fatalf("first decision %q kind %d, want the offer answered", name, a.kind)
	}
	// The server never revived: the answer holds a restart for restartRetry,
	// then the rest of the wait runs out.
	if name, a := deathStep(t, b, at(2*time.Minute-time.Second)); name != "dead" || a.kind != actHold {
		t.Fatalf("in the wait: %q kind %d, want dead holding", name, a.kind)
	}
	if name, a := deathStep(t, b, at(2*time.Minute)); name != "dead" || a.kind != actRestart {
		t.Fatalf("after the wait: %q kind %d, want a restart", name, a.kind)
	}
	town := Point{X: 1000, Y: 2000}
	deathApply(t, b, at(2*time.Minute+time.Second), Revive{ObjectID: selfID}, TeleportToLocation{ObjectID: selfID, Pos: town})
	if err := b.wake(at(2*time.Minute + 2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if f := out.frames[len(out.frames)-1]; f[0] != opGameAppearing {
		t.Fatalf("after the teleport sent % x, want Appearing", f)
	}
	var to Point
	for d := 3 * time.Second; d < 10*time.Second && to == (Point{}); d += 100 * time.Millisecond {
		if name, a := deathStep(t, b, at(2*time.Minute+d)); a.kind == actMove {
			if name != "follow" {
				t.Fatalf("moved by %q, want follow", name)
			}
			to = moveTarget(a.payload)
		}
	}
	if to == (Point{}) || dist2(to, leaderAt) >= dist2(town, leaderAt) {
		t.Fatalf("revived bot moved to %v, want toward the leader at %v", to, leaderAt)
	}
}

// newLeaderBrain is a started default-persona brain at the origin in gmID's
// party, the GM in sight 200 units off.
func newLeaderBrain(t *testing.T) *Brain {
	t.Helper()
	b, _ := newTestBrain(t, DefaultPersona())
	b.persona.IdleChance = 0
	deathApply(t, b, t0,
		PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}},
		CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 200}, RunSpeed: 100, WalkSpeed: 50, Running: true},
	)
	b.follow.anchored, b.follow.anchor = true, Point{X: 200}
	return b
}

func TestLeaderDeadSitByBody(t *testing.T) {
	b := newLeaderBrain(t)
	deathApply(t, b, at(time.Second), Die{ObjectID: gmID})
	if name, a := deathStep(t, b, at(2*time.Second)); name != "leader" || a.kind != actSitStand {
		t.Fatalf("leader dead: %q kind %d, want leader sitting down", name, a.kind)
	}
	deathApply(t, b, at(2500*time.Millisecond), ChangeWaitType{ObjectID: selfID, Type: WaitSitting})
	if name, a := deathStep(t, b, at(3*time.Second)); name != "leader" || a.kind != actHold {
		t.Fatalf("sitting by the body: %q kind %d, want leader holding", name, a.kind)
	}
	deathApply(t, b, at(4*time.Second), Revive{ObjectID: gmID})
	if name, a := deathStep(t, b, at(5*time.Second)); name != "leader" || a.kind != actSitStand {
		t.Fatalf("leader revived: %q kind %d, want leader standing up", name, a.kind)
	}
	deathApply(t, b, at(5500*time.Millisecond), ChangeWaitType{ObjectID: selfID, Type: WaitStanding})
	if name, _ := deathStep(t, b, at(6*time.Second)); name == "leader" {
		t.Fatal("standing bot still held by rule leader")
	}
}

func TestLeaderDeadFarAway(t *testing.T) {
	b := newLeaderBrain(t)
	deathApply(t, b, at(time.Second), CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 5000}, RunSpeed: 100}, Die{ObjectID: gmID})
	if name, a := deathStep(t, b, at(2*time.Second)); name != "follow" || a.kind == actSitStand {
		t.Fatalf("body far away: %q kind %d, want follow", name, a.kind)
	}
}

func TestLeaderLeftFallsBackToTown(t *testing.T) {
	b := newLeaderBrain(t)
	lost := at(time.Second)
	deathApply(t, b, lost, PartySmallWindowDeleteAll{}, DeleteObject{ObjectID: gmID})
	wait := b.persona.Recover.LeaderWait
	if name, a := deathStep(t, b, lost.Add(wait-time.Millisecond)); name != "leader" || a.kind != actHold {
		t.Fatalf("in the wait: %q kind %d, want leader holding", name, a.kind)
	}
	now := lost.Add(wait)
	name, a := deathStep(t, b, now)
	if name != "leader" || !bytes.Equal(a.payload, botwire.EncodeSay2Tell("GM", "where did you go?")) {
		t.Fatalf("after the wait: %q % x, want the whisper to GM", name, a.payload)
	}
	now = now.Add(chatGap)
	if name, a := deathStep(t, b, now); name != "leader" || !bytes.Equal(a.payload, botwire.EncodeRequestUserCommand(userCommandEscape)) {
		t.Fatalf("after the whisper: %q kind %d, want /unstuck", name, a.kind)
	}
	deathApply(t, b, now.Add(time.Second), MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: 2099, HitTime: 5 * time.Minute})
	if name, a := deathStep(t, b, now.Add(4*time.Minute)); name != "leader" || a.kind != actHold {
		t.Fatalf("recall casting: %q kind %d, want leader holding", name, a.kind)
	}
	// The cast ended with no recall: ask again.
	if name, a := deathStep(t, b, now.Add(5*time.Minute+time.Second)); name != "leader" || a.kind != actUserCommand {
		t.Fatalf("recall ended without a teleport: %q kind %d, want /unstuck again", name, a.kind)
	}
	home := now.Add(6 * time.Minute)
	deathApply(t, b, home, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 80000}})
	if !b.lead.home {
		t.Fatal("landing not noted")
	}
	if name, _ := deathStep(t, b, home.Add(time.Second)); name == "leader" {
		t.Fatal("bot in town with no leader still held by rule leader")
	}
	// A new invitation from the GM ends the loss.
	deathApply(t, b, home.Add(time.Minute), PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}})
	if !b.lead.lost.IsZero() || b.lead.id != gmID {
		t.Fatalf("watch after rejoining = %+v, want the GM followed again", b.lead)
	}
}

func TestLeaderTeleported(t *testing.T) {
	b := newLeaderBrain(t)
	lost := at(time.Second)
	deathApply(t, b, lost, TeleportToLocation{ObjectID: gmID, Pos: Point{X: 90000}}, DeleteObject{ObjectID: gmID})
	if b.lead.lost.IsZero() || !b.lead.teleported {
		t.Fatalf("watch = %+v, want the leader lost by teleport", b.lead)
	}
	if name, a := deathStep(t, b, lost.Add(time.Minute)); name != "leader" || a.kind != actHold {
		t.Fatalf("in the wait: %q kind %d, want leader holding", name, a.kind)
	}
	b.lead.told, b.lead.home = true, true
	if name, a := deathStep(t, b, lost.Add(10*time.Minute)); name != "leader" || a.kind != actHold {
		t.Fatalf("in town with the leader away: %q kind %d, want leader holding", name, a.kind)
	}
	deathApply(t, b, lost.Add(11*time.Minute), CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 300}})
	if name, _ := deathStep(t, b, lost.Add(11*time.Minute+time.Second)); name == "leader" {
		t.Fatal("leader back in sight, still held by rule leader")
	}
	if !b.lead.lost.IsZero() {
		t.Fatalf("watch = %+v, want the loss over", b.lead)
	}
}

func TestLeaderLossEdges(t *testing.T) {
	tests := []struct {
		name     string
		run      func(b *Brain)
		wantLost bool
	}{
		{"bot left on its own", func(b *Brain) {
			if _, err := b.send(leavePartyAction(), at(time.Second)); err != nil {
				t.Fatal(err)
			}
			deathApply(t, b, at(2*time.Second), PartySmallWindowDeleteAll{})
		}, false},
		{"bot expelled from a big party", func(b *Brain) {
			deathApply(t, b, at(time.Second), SystemMessage{ID: msgExpelledFromParty}, PartySmallWindowDeleteAll{})
		}, false},
		{"leader logged out of a big party, another took over", func(b *Brain) {
			deathApply(t, b, at(time.Second), PartySmallWindowDeleteAll{}, PartySmallWindowAll{LeaderID: mateID, Members: []PartyMember{mateRow}})
		}, false},
		{"bot took over the party", func(b *Brain) {
			deathApply(t, b, at(time.Second), PartySmallWindowDeleteAll{}, PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{mateRow}})
		}, true},
		{"leader short hop in sight", func(b *Brain) {
			deathApply(t, b, at(time.Second), TeleportToLocation{ObjectID: gmID, Pos: Point{X: 500}}, DeleteObject{ObjectID: gmID})
			deathApply(t, b, at(2*time.Second), CharInfo{ObjectID: gmID, Name: "GM", Pos: Point{X: 500}})
			b.watchLeader(at(3 * time.Second))
		}, false},
		{"fast teleport", func(b *Brain) {
			deathApply(t, b, at(time.Second), TeleportToLocation{ObjectID: gmID, Pos: Point{X: 500}, Fast: true})
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLeaderBrain(t)
			tt.run(b)
			if lost := !b.lead.lost.IsZero(); lost != tt.wantLost {
				t.Fatalf("lost = %v (%+v), want %v", lost, b.lead, tt.wantLost)
			}
		})
	}
}

func TestLeaderLossPolicyOff(t *testing.T) {
	b := newLeaderBrain(t)
	b.persona.Recover.LeaderWait = 0
	deathApply(t, b, at(time.Second), PartySmallWindowDeleteAll{})
	if name, _ := deathStep(t, b, at(time.Hour)); name == "leader" {
		t.Fatal("LeaderWait 0 still held by rule leader")
	}
}

// TestSupportLeavesFindersKeepers checks the support persona's default
// loot rule: under finders-keepers in another's party it never loots.
func TestSupportLeavesFindersKeepers(t *testing.T) {
	if !DefaultPersona().Loot.LeaveFindersKeepers {
		t.Fatal("default persona loots under finders-keepers")
	}
}
