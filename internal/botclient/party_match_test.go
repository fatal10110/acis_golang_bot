package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// matchRoom is a room a player named Lea leads, free and open to levels
// 1-20.
var matchRoom = PartyRoom{ID: 5, Title: "exp", Location: 1, MinLevel: 1, MaxLevel: 20, Members: 1, MaxMembers: 4, Leader: "Lea"}

const leaID = int32(0x30000010)

// Every frame the room rule sends, as the tests name it.
var (
	roomListFrame     = botwire.EncodeRequestListPartyWaiting(1, roomsNearMe, roomsLevelsMine)
	roomExitFrame     = botwire.EncodeRequestExitPartyMatchingWaitingRoom()
	roomWithdrawFrame = botwire.EncodeRequestWithdrawPartyRoom(matchRoom.ID)
	roomJoinFrame     = botwire.EncodeRequestJoinPartyRoom(matchRoom.ID, roomsNearMe, roomsLevelsMine)
)

// roomPolicy is the room policy of the tests: the floors.
var roomPolicy = RoomPolicy{Join: true, ListEvery: roomListMin, Wait: time.Minute, Retry: roomRetryMin}

// roomBrain is a level-10 support bot of id self volunteering under
// roomPolicy, accepting every invitation through claims.
func roomBrain(t *testing.T, self int32, claims *PartyClaims) (*Brain, *recordSender) {
	t.Helper()
	b, out := newTestBrain(t, Persona{})
	b.world.Self.ObjectID, b.world.Self.Level = self, 10
	cfg := BrainConfig{Persona: Persona{Role: RoleSupport, JoinParty: acceptParty, Rooms: roomPolicy}}
	if claims != nil {
		claims.Apply(&cfg)
	}
	b.persona.Role, b.persona.JoinParty, b.persona.Rooms = RoleSupport, cfg.Persona.JoinParty, roomPolicy
	b.cfg.PartyClaims = cfg.PartyClaims
	return b, out
}

// roomStep runs the room rule at now and sends what it decides.
func roomStep(t *testing.T, b *Brain, now time.Time) {
	t.Helper()
	if a, ok := b.ruleRooms(now); ok {
		if sent, err := b.send(a, now); err != nil || !sent {
			t.Fatalf("send %v: %v, %v", a.kind, sent, err)
		}
	}
}

// roomList runs the room rule until its first list is due (within
// ListEvery of t0), hands it rooms one second later and runs it then; it
// returns that time.
func roomList(t *testing.T, b *Brain, rooms ...PartyRoom) time.Time {
	t.Helper()
	roomStep(t, b, t0)
	listed := at(roomPolicy.ListEvery)
	roomStep(t, b, listed)
	answered := listed.Add(time.Second)
	handleAt(t, b, answered, PartyMatchList{Rooms: rooms})
	roomStep(t, b, answered)
	return answered
}

// wantFrames fails unless out sent exactly want.
func wantFrames(t *testing.T, out *recordSender, want ...[]byte) {
	t.Helper()
	if len(out.frames) != len(want) {
		t.Fatalf("sent % x, want % x", out.frames, want)
	}
	for i := range want {
		if !bytes.Equal(out.frames[i], want[i]) {
			t.Fatalf("frame %d % x, want % x", i, out.frames[i], want[i])
		}
	}
}

// TestRoomVolunteer checks the room rule's choice after one list: an
// eligible room gets exactly one join; a bot that may not volunteer lists
// nothing; a room it may not enter leaves the waiting list instead.
func TestRoomVolunteer(t *testing.T) {
	full, high := matchRoom, matchRoom
	full.Members = full.MaxMembers
	high.MinLevel = 11
	botLed := matchRoom
	botLed.Leader = "PiaBot"
	tests := []struct {
		name  string
		setup func(b *Brain, c *PartyClaims)
		rooms []PartyRoom
		want  [][]byte
	}{
		{"eligible room", nil, []PartyRoom{matchRoom}, [][]byte{roomListFrame, roomJoinFrame}},
		{"no room", nil, nil, [][]byte{roomListFrame, roomExitFrame}},
		{"in a party", func(b *Brain, _ *PartyClaims) {
			handleAt(t, b, t0, PartySmallWindowAll{LeaderID: leaID, Members: []PartyMember{{ObjectID: leaID, Name: "Lea"}}})
		}, []PartyRoom{matchRoom}, nil},
		{"party slot", func(b *Brain, _ *PartyClaims) {
			linkBrain(b, standing{})
			b.setPartySlot(joinSlot)
		}, []PartyRoom{matchRoom}, nil},
		{"in combat", func(b *Brain, _ *PartyClaims) { b.world.Self.InCombat = true }, []PartyRoom{matchRoom}, nil},
		{"on a supply trip", func(b *Brain, _ *PartyClaims) { b.supplyTrip.since = t0 }, []PartyRoom{matchRoom}, nil},
		{"damage role", func(b *Brain, _ *PartyClaims) { b.persona.Role = RoleDamage }, []PartyRoom{matchRoom}, nil},
		{"policy off", func(b *Brain, _ *PartyClaims) { b.persona.Rooms.Join = false }, []PartyRoom{matchRoom}, nil},
		{"no claim table", func(b *Brain, _ *PartyClaims) { b.cfg.PartyClaims = nil }, []PartyRoom{matchRoom}, nil},
		{"level out of range", nil, []PartyRoom{high}, [][]byte{roomListFrame, roomExitFrame}},
		{"room full", nil, []PartyRoom{full}, [][]byte{roomListFrame, roomExitFrame}},
		{"bot leads it", nil, []PartyRoom{botLed}, [][]byte{roomListFrame, roomExitFrame}},
		{"claimed by another bot", func(_ *Brain, c *PartyClaims) { c.claim("Lea", otherBotID, t0) }, []PartyRoom{matchRoom}, [][]byte{roomListFrame, roomExitFrame}},
		{"leader in retry", func(b *Brain, _ *PartyClaims) {
			b.rooms.retry = map[string]time.Time{"Lea": at(time.Hour)}
		}, []PartyRoom{matchRoom}, [][]byte{roomListFrame, roomExitFrame}},
		{"second room eligible", nil, []PartyRoom{full, matchRoom}, [][]byte{roomListFrame, roomJoinFrame}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := NewPartyClaims()
			b, out := roomBrain(t, selfID, claims)
			if tt.setup != nil {
				tt.setup(b, claims)
			}
			roomList(t, b, tt.rooms...)
			wantFrames(t, out, tt.want...)
		})
	}
}

// TestRoomJoinAccepted checks the room joined then the leader's invitation:
// the claiming bot accepts it, outlives the 15 s claim lapse while it waits,
// and sends nothing more once in the party.
func TestRoomJoinAccepted(t *testing.T) {
	claims := NewPartyClaims()
	b, out := roomBrain(t, selfID, claims)
	asked := roomList(t, b, matchRoom)
	handleAt(t, b, asked.Add(time.Second), PartyMatchDetail{RoomID: matchRoom.ID, MaxMembers: 4, MinLevel: 1, MaxLevel: 20})
	b.tidyPartyClaim(asked.Add(30 * time.Second))
	if leader, _, ok := claims.held(selfID); !ok || leader != "Lea" {
		t.Fatalf("claim %q %v while waiting, want Lea", leader, ok)
	}
	inv := asked.Add(40 * time.Second)
	handleAt(t, b, inv, AskJoinParty{Requester: "Lea", Loot: 1})
	a, ok := b.rulePrompt(inv)
	if !ok || !bytes.Equal(a.payload, acceptFrame) {
		t.Fatalf("prompt answer % x %v, want accept", a.payload, ok)
	}
	if _, err := b.send(a, inv); err != nil {
		t.Fatal(err)
	}
	handleAt(t, b, inv.Add(time.Second), PartySmallWindowAll{LeaderID: leaID, Members: []PartyMember{{ObjectID: leaID, Name: "Lea"}}})
	for _, d := range []time.Duration{2 * time.Second, 2 * time.Minute, time.Hour} {
		roomStep(t, b, inv.Add(d))
	}
	wantFrames(t, out, roomListFrame, roomJoinFrame, acceptFrame)
	if leader, _, ok := claims.held(selfID); !ok || leader != "Lea" {
		t.Fatalf("claim %q %v in the party, want Lea", leader, ok)
	}
}

// TestRoomJoinRefused checks a 1413: the bot leaves the waiting list, gives
// the leader back, and asks that leader's room again only after Retry.
func TestRoomJoinRefused(t *testing.T) {
	claims := NewPartyClaims()
	b, out := roomBrain(t, selfID, claims)
	asked := roomList(t, b, matchRoom)
	handleAt(t, b, asked.Add(time.Second), SystemMessage{ID: msgCantEnterPartyRoom})
	roomStep(t, b, asked.Add(time.Second))
	wantFrames(t, out, roomListFrame, roomJoinFrame, roomExitFrame)
	if claims.Held() != 0 {
		t.Fatalf("%d claims after the refusal, want 0", claims.Held())
	}
	// The next lists within Retry exit; the one after asks again.
	var listed int
	for d := 2 * time.Second; d <= roomPolicy.Retry+2*roomPolicy.ListEvery; d += time.Second {
		now := asked.Add(d)
		n := len(out.frames)
		roomStep(t, b, now)
		if len(out.frames) > n && bytes.Equal(out.frames[n], roomListFrame) {
			listed++
			handleAt(t, b, now, PartyMatchList{Rooms: []PartyRoom{matchRoom}})
		}
		if joins := framesOf(out, opGameRequestJoinPartyRoom); len(joins) > 1 {
			if now.Before(asked.Add(roomPolicy.Retry)) {
				t.Fatalf("second join %v after the refusal, want none within %v", d, roomPolicy.Retry)
			}
			break
		}
	}
	if joins := framesOf(out, opGameRequestJoinPartyRoom); len(joins) != 2 || listed < 2 {
		t.Fatalf("%d joins after %d lists, want the second once Retry ended", len(joins), listed)
	}
}

// TestRoomNoInvitation checks a room the leader never invites from: after
// Wait the bot leaves the room, gives the leader back and lists again only
// after ListEvery.
func TestRoomNoInvitation(t *testing.T) {
	claims := NewPartyClaims()
	b, out := roomBrain(t, selfID, claims)
	asked := roomList(t, b, matchRoom)
	handleAt(t, b, asked.Add(time.Second), PartyMatchDetail{RoomID: matchRoom.ID})
	roomStep(t, b, asked.Add(roomPolicy.Wait-time.Second))
	roomStep(t, b, asked.Add(roomPolicy.Wait))
	for d := time.Second; d < roomPolicy.ListEvery; d += time.Second {
		roomStep(t, b, asked.Add(roomPolicy.Wait+d))
	}
	wantFrames(t, out, roomListFrame, roomJoinFrame, roomWithdrawFrame)
	if claims.Held() != 0 {
		t.Fatalf("%d claims after the wait, want 0", claims.Held())
	}
	if !b.rooms.retry["Lea"].After(asked.Add(roomPolicy.Wait)) {
		t.Fatal("Lea not in retry after the wait")
	}
}

// TestRoomUnanswered checks a list the server never answers, and a room
// the server closes on the bot: the bot leaves the waiting list.
func TestRoomUnanswered(t *testing.T) {
	b, out := roomBrain(t, selfID, NewPartyClaims())
	roomStep(t, b, t0)
	listed := at(roomPolicy.ListEvery)
	roomStep(t, b, listed)
	roomStep(t, b, listed.Add(roomListAnswer-time.Millisecond))
	roomStep(t, b, listed.Add(roomListAnswer))
	wantFrames(t, out, roomListFrame, roomExitFrame)

	handleAt(t, b, listed.Add(10*time.Second), ExClosePartyRoom{})
	roomStep(t, b, listed.Add(10*time.Second))
	wantFrames(t, out, roomListFrame, roomExitFrame, roomExitFrame)
}

// TestRoomListPacing checks the cadence: one bot lists at most once per
// ListEvery, and two bots of one process sharing a due time list
// roomListGap apart.
func TestRoomListPacing(t *testing.T) {
	claims := NewPartyClaims()
	b1, out1 := roomBrain(t, selfID, claims)
	b2, out2 := roomBrain(t, otherBotID, claims)
	b1.rooms.next, b2.rooms.next = t0, t0
	roomStep(t, b1, t0)
	roomStep(t, b2, t0)
	if len(out1.frames)+len(out2.frames) != 1 {
		t.Fatalf("%d lists at once, want 1", len(out1.frames)+len(out2.frames))
	}
	roomStep(t, b2, t0.Add(roomListGap))
	wantFrames(t, out2, roomListFrame)
	var lists int
	for d := time.Second; d < roomPolicy.ListEvery; d += time.Second {
		roomStep(t, b1, t0.Add(d))
	}
	for _, f := range out1.frames {
		if bytes.Equal(f, roomListFrame) {
			lists++
		}
	}
	if lists != 1 {
		t.Fatalf("bot listed %d times within ListEvery, want 1", lists)
	}
	if got := (RoomPolicy{ListEvery: time.Second, Retry: time.Second}).withDefaults(); got.ListEvery != roomListMin || got.Retry != roomRetryMin {
		t.Fatalf("withDefaults = %+v, want the floors", got)
	}
}

// TestRoomOtherInvitation checks a bot that waits in Lea's room and takes
// another player's invitation: it leaves Lea's room once, as the server
// keeps it there for the whole party otherwise.
func TestRoomOtherInvitation(t *testing.T) {
	const monID = int32(0x30000020)
	claims := NewPartyClaims()
	b, out := roomBrain(t, selfID, claims)
	asked := roomList(t, b, matchRoom)
	handleAt(t, b, asked.Add(time.Second), PartyMatchDetail{RoomID: matchRoom.ID, MaxMembers: 4, MinLevel: 1, MaxLevel: 20})
	inv := asked.Add(10 * time.Second)
	handleAt(t, b, inv, AskJoinParty{Requester: "Mon", Loot: 1})
	a, ok := b.rulePrompt(inv)
	if !ok || !bytes.Equal(a.payload, acceptFrame) {
		t.Fatalf("prompt answer % x %v, want accept", a.payload, ok)
	}
	if _, err := b.send(a, inv); err != nil {
		t.Fatal(err)
	}
	handleAt(t, b, inv.Add(time.Second), PartySmallWindowAll{LeaderID: monID, Members: []PartyMember{{ObjectID: monID, Name: "Mon"}}})
	for _, d := range []time.Duration{2 * time.Second, 3 * time.Second, 2 * time.Minute, time.Hour} {
		roomStep(t, b, inv.Add(d))
	}
	wantFrames(t, out, roomListFrame, roomJoinFrame, acceptFrame, roomWithdrawFrame)
	if leader, _, ok := claims.held(selfID); !ok || leader != "Mon" {
		t.Fatalf("claim %q %v in Mon's party, want Mon", leader, ok)
	}
}
