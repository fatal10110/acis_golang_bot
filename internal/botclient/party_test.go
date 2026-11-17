package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

const (
	gmID   = int32(0x30000001)
	mateID = int32(0x30000002)
)

var (
	gmRow   = PartyMember{ObjectID: gmID, Name: "GM", CP: 100, MaxCP: 200, HP: 300, MaxHP: 400, MP: 50, MaxMP: 60, Level: 40, ClassID: 88}
	mateRow = PartyMember{ObjectID: mateID, Name: "mate", CP: 1, MaxCP: 2, HP: 3, MaxHP: 4, MP: 5, MaxMP: 6, Level: 7, ClassID: 18}
)

// vitals are the fields of a party window row, in the server's order
// (writePartyMemberVitals).
func vitals(m PartyMember) []any {
	return []any{m.ObjectID, m.Name, m.CP, m.MaxCP, m.HP, m.MaxHP, m.MP, m.MaxMP, m.Level, m.ClassID}
}

func partyWindowAll(leader, loot int32, races []int32, members ...PartyMember) []byte {
	fields := []any{leader, loot, int32(len(members))}
	for i, m := range members {
		fields = append(append(fields, vitals(m)...), 0, races[i])
	}
	return packet(opGamePartySmallWindowAll, fields...)
}

func TestDecodePartyEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// "GM" in UTF-16LE with its terminator, then loot rule 1.
		{"AskJoinParty bytes", []byte{0x39, 'G', 0, 'M', 0, 0, 0, 1, 0, 0, 0}, AskJoinParty{Requester: "GM", Loot: 1}},
		{"AskJoinParty", packet(opGameAskJoinParty, "leader", 2), AskJoinParty{Requester: "leader", Loot: 2}},
		{"JoinParty", packet(opGameJoinParty, 1), JoinParty{Response: 1}},
		{
			"PartySmallWindowAll", partyWindowAll(gmID, 1, []int32{0, 3}, gmRow, mateRow),
			PartySmallWindowAll{LeaderID: gmID, Loot: 1, Members: []PartyMember{gmRow, mateRow}},
		},
		{"PartySmallWindowAll alone", partyWindowAll(selfID, 0, nil), PartySmallWindowAll{LeaderID: selfID}},
		{
			"PartySmallWindowAdd", packet(opGamePartySmallWindowAdd, append(append([]any{gmID, 2}, vitals(mateRow)...), 0, 0)...),
			PartySmallWindowAdd{LeaderID: gmID, Loot: 2, Member: mateRow},
		},
		{"PartySmallWindowUpdate", packet(opGamePartySmallWindowUpdate, vitals(gmRow)...), PartySmallWindowUpdate{Member: gmRow}},
		{"PartySmallWindowDelete", packet(opGamePartySmallWindowDelete, mateID, "mate"), PartySmallWindowDelete{ObjectID: mateID, Name: "mate"}},
		{"PartySmallWindowDeleteAll", packet(opGamePartySmallWindowDeleteAll), PartySmallWindowDeleteAll{}},
		{
			"PartyMemberPosition", packet(opGamePartyMemberPosition, 2, selfID, 1, 2, 3, gmID, -4, 5, -6),
			PartyMemberPosition{Members: []PartyMemberAt{{ObjectID: selfID, Pos: Point{1, 2, 3}}, {ObjectID: gmID, Pos: Point{-4, 5, -6}}}},
		},
		{"PartyMemberPosition empty", packet(opGamePartyMemberPosition, 0), PartyMemberPosition{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodePartyEventsRejects(t *testing.T) {
	all := partyWindowAll(gmID, 1, []int32{0}, gmRow)
	add := packet(opGamePartySmallWindowAdd, append(append([]any{gmID, 2}, vitals(mateRow)...), 0, 0)...)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"AskJoinParty without loot", packet(opGameAskJoinParty, "GM"), wire.ErrShortPacket},
		{"WindowAll short of its count", packet(opGamePartySmallWindowAll, append([]any{gmID, 1, 2}, vitals(gmRow)...)...), wire.ErrShortPacket},
		{"WindowAll count past the frame", packet(opGamePartySmallWindowAll, gmID, 1, 0x7fffffff), wire.ErrShortPacket},
		{"WindowAll trailing", append(all, 0), errTrailingBytes},
		{"WindowAdd without its zeros", add[:len(add)-8], wire.ErrShortPacket},
		{"DeleteAll trailing", packet(opGamePartySmallWindowDeleteAll, 0), errTrailingBytes},
		{"MemberPosition short row", packet(opGamePartyMemberPosition, 1, gmID, 1, 2), wire.ErrShortPacket},
		{"MemberPosition trailing", packet(opGamePartyMemberPosition, 0, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

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
			PartyMemberPosition{Members: []PartyMemberAt{{selfID, Point{1, 1, 1}}, {gmID, Point{10, 20, 30}}, {mateID, Point{9, 9, 9}}}},
			at(8 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{gmID: gm(Point{10, 20, 30}, at(8*time.Second))}},
		},
		{
			"member added",
			PartySmallWindowAdd{LeaderID: gmID, Loot: 1, Member: mateRow},
			at(9 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: gm(Point{10, 20, 30}, at(8*time.Second)), mateID: {PartyMember: mateRow},
			}},
		},
		{
			"leader hurt keeps its position",
			PartySmallWindowUpdate{Member: hurt},
			at(10 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{10, 20, 30}, PosAt: at(8 * time.Second)}, mateID: {PartyMember: mateRow},
			}},
		},
		{
			"update of a stranger ignored",
			PartySmallWindowUpdate{Member: PartyMember{ObjectID: pcID}},
			at(10 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{10, 20, 30}, PosAt: at(8 * time.Second)}, mateID: {PartyMember: mateRow},
			}},
		},
		{
			"member left",
			PartySmallWindowDelete{ObjectID: mateID, Name: "mate"},
			at(11 * time.Second),
			PartyInvite{},
			Party{LeaderID: gmID, Loot: 1, Members: map[int32]*PartyMate{
				gmID: {PartyMember: hurt, Pos: Point{10, 20, 30}, PosAt: at(8 * time.Second)},
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
