package botclient

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

const (
	gmID   = int32(0x30000001)
	mateID = int32(0x30000002)
)

var (
	gmRow   = PartyMember{ObjectID: gmID, Name: "GM", CP: 100, MaxCP: 200, HP: 300, MaxHP: 400, MP: 50, MaxMP: 60, Level: 40, ClassID: 88}
	mateRow = PartyMember{ObjectID: mateID, Name: "mate", CP: 1, MaxCP: 2, HP: 3, MaxHP: 4, MP: 5, MaxMP: 6, Level: 7, ClassID: 18}
)

func TestPartyEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestJoinParty", encodeRequestJoinParty("GM", 1), []byte{0x29, 'G', 0, 'M', 0, 0, 0, 1, 0, 0, 0}},
		{"RequestAnswerJoinParty accept", encodeRequestAnswerJoinParty(true), []byte{0x2a, 1, 0, 0, 0}},
		{"RequestAnswerJoinParty decline", encodeRequestAnswerJoinParty(false), []byte{0x2a, 0, 0, 0, 0}},
		{"RequestWithdrawParty", encodeRequestWithdrawParty(), []byte{0x2b}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// TestWorldParty applies a party's life, joining a GM's party through to
// leaving it, and checks the model after each step.
func TestWorldParty(t *testing.T) {
	gm := func(pos Point, posAt time.Time) *PartyMate {
		return &PartyMate{PartyMember: gmRow, Pos: pos, PosAt: posAt}
	}
	hurt := gmRow
	hurt.HP = 120
	steps := []struct {
		name       string
		ev         Event
		at         time.Time
		wantInvite PartyInvite
		wantParty  Party
	}{
		{
			"invited",
			AskJoinParty{Requester: "GM", Loot: 1},
			at(time.Second),
			PartyInvite{Open: true, Requester: "GM", Loot: 1, At: at(time.Second)},
			Party{},
		},
		{
			"joined",
			PartySmallWindowAll{LeaderID: gmID, Loot: 1, Members: []PartyMember{gmRow}},
			at(2 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{gmID: gm(Point{}, time.Time{})}},
		},
		{
			"positions, own and an unknown id skipped",
			PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: selfID, Pos: Point{X: 1, Y: 1, Z: 1}}, {ObjectID: gmID, Pos: Point{X: 10, Y: 20, Z: 30}}, {ObjectID: mateID, Pos: Point{X: 9, Y: 9, Z: 9}}}},
			at(8 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{gmID: gm(Point{X: 10, Y: 20, Z: 30}, at(8*time.Second))}},
		},
		{
			"member added",
			PartySmallWindowAdd{LeaderID: gmID, Loot: 1, Member: mateRow},
			at(9 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: gm(Point{X: 10, Y: 20, Z: 30}, at(8*time.Second)), mateID: {PartyMember: mateRow},
			}},
		},
		{
			"leader hurt keeps its position",
			PartySmallWindowUpdate{Member: hurt},
			at(10 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{X: 10, Y: 20, Z: 30}, PosAt: at(8 * time.Second)}, mateID: {PartyMember: mateRow},
			}},
		},
		{
			"update of a stranger ignored",
			PartySmallWindowUpdate{Member: PartyMember{ObjectID: pcID}},
			at(10 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{X: 10, Y: 20, Z: 30}, PosAt: at(8 * time.Second)}, mateID: {PartyMember: mateRow},
			}},
		},
		{
			"member left",
			PartySmallWindowDelete{ObjectID: mateID, Name: "mate"},
			at(11 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{X: 10, Y: 20, Z: 30}, PosAt: at(8 * time.Second)},
			}},
		},
		{"left the party", PartySmallWindowDeleteAll{}, at(12 * time.Second), PartyInvite{}, Party{}},
		{
			"bot invites and leads",
			PartySmallWindowAdd{LeaderID: selfID, Member: gmRow},
			at(13 * time.Second),
			PartyInvite{},
			Party{LeaderID: selfID, Members: map[int32]*PartyMate{gmID: {PartyMember: gmRow}}},
		},
	}
	w := newTestWorld(t)
	if w.Party.In() {
		t.Fatal("a new world is in a party")
	}
	for _, s := range steps {
		w.Apply(s.ev, s.at)
		if w.Prompts.Party != s.wantInvite {
			t.Fatalf("%s: invite %+v, want %+v", s.name, w.Prompts.Party, s.wantInvite)
		}
		if !reflect.DeepEqual(w.Party, s.wantParty) {
			t.Fatalf("%s: party %+v, want %+v", s.name, w.Party, s.wantParty)
		}
		if w.Party.In() != (s.wantParty.LeaderID != 0) {
			t.Fatalf("%s: In() = %v", s.name, w.Party.In())
		}
	}
}

// TestBrainPartyInvite checks that an invitation is answered once, at the
// reaction to it, per the persona, and not after the server dropped it.
func TestBrainPartyInvite(t *testing.T) {
	onlyGM := func(_ *World, requester string, _ time.Time) bool { return requester == "GM" }
	accept := encodeRequestAnswerJoinParty(true)
	decline := encodeRequestAnswerJoinParty(false)
	tests := []struct {
		name      string
		persona   Persona
		requester string
		wakeAt    time.Duration // after the reaction; 0 wakes at it
		want      [][]byte
	}{
		{"default persona accepts", Persona{JoinParty: DefaultPersona().JoinParty}, "anyone", 0, [][]byte{accept}},
		{"GM accepted", Persona{JoinParty: onlyGM}, "GM", 0, [][]byte{accept}},
		{"player declined", Persona{JoinParty: onlyGM}, "player", 0, [][]byte{decline}},
		{"no policy declines", Persona{}, "GM", 0, [][]byte{decline}},
		{"expired before the decision", Persona{JoinParty: onlyGM}, "GM", partyInviteTimeout, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, out := newTestBrain(t, tt.persona)
			ev := at(100 * time.Millisecond)
			if err := b.handle([]Event{AskJoinParty{Requester: tt.requester, Loot: 1}}, ev); err != nil {
				t.Fatal(err)
			}
			react := b.decisionAt()
			if d := react.Sub(ev); d < defaultReactMin || d > defaultReactMax {
				t.Fatalf("answer due %v after the invitation, want [%v, %v]", d, defaultReactMin, defaultReactMax)
			}
			first := react
			if tt.wakeAt > 0 {
				first = ev.Add(tt.wakeAt)
			}
			for _, w := range []time.Time{first, first.Add(2 * time.Second), first.Add(4 * time.Second)} {
				if err := b.wake(w); err != nil {
					t.Fatal(err)
				}
			}
			var got [][]byte
			for _, f := range out.frames {
				if f[0] == opGameRequestAnswerJoinParty {
					got = append(got, f)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("answers % x, want % x", got, tt.want)
			}
			if b.world.Prompts.Party.Pending(first.Add(4 * time.Second)) {
				t.Fatal("invitation still pending after the answer")
			}
		})
	}
}

// TestBrainLeaveParty checks that a persona's leave decision goes out as
// RequestWithdrawParty.
func TestBrainLeaveParty(t *testing.T) {
	b, out := newTestBrain(t, Persona{Goal: func(*World, time.Time) (Action, bool) { return leavePartyAction(), true }})
	b.world.Apply(PartySmallWindowAll{LeaderID: gmID, Members: []PartyMember{gmRow}}, t0)
	for _, w := range []time.Time{at(time.Second), at(3 * time.Second), at(5 * time.Second)} {
		if err := b.wake(w); err != nil {
			t.Fatal(err)
		}
	}
	if len(out.frames) == 0 || !bytes.Equal(out.frames[0], []byte{opGameRequestWithdrawParty}) {
		t.Fatalf("sent % x, want RequestWithdrawParty first", out.frames)
	}
}
