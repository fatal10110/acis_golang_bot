package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Party-matching system messages (acis_golang internal/gameserver/network/
// serverpackets/partymatch.go).
const (
	msgPartyRoomCreated   = 1388
	msgCantEnterPartyRoom = 1413
	msgEnteredPartyRoom   = 1900
)

// roomJoinBound bounds the bot's room join: its first list comes within
// RoomPolicy.ListEvery of entering the world and each next one within one
// and a half of it, so two lists and a margin.
const roomJoinBound = 3 * time.Minute

// TestLivePartyRoomJoin has a player with an unmarked name open a
// party-matching room next to a partyless support bot: the bot lists the
// rooms near it, asks to enter the player's room, and accepts the player's
// party invitation through the claim table (#438). The GM only brings the
// two together: it carries the bot marker, so a bot enters no room it
// leads (#485).
func TestLivePartyRoomJoin(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 6*time.Minute)
	gm := enterGM(t, ctx, tg)
	gmToOpenGround(t, ctx, gm)
	w := NewWatch(botclient.SystemClock{})
	acc, _ := tg.Account("pr", 1)
	volunteer := func() botclient.Persona {
		p := partyPersona()
		p.Rooms.ListEvery = 30 * time.Second
		return p
	}
	claims := botclient.NewPartyClaims()
	b := runBotWith(t, ctx, tg, acc, botSetup{persona: volunteer, tap: w, brain: claims.Apply})
	name := botOnline(t, ctx, tg, b)
	leaderAcc, _ := tg.Account("pr", 2)
	leader := enter(t, ctx, tg, leaderAcc, unmarkedCharacter(t, leaderAcc, roomLeaderNames), nil)
	if botnames.DefaultMarker.Marks(leader.Self.Name) {
		t.Fatalf("the room leader %s carries the bot marker: no bot enters its room", leader.Self.Name)
	}
	recallClient(t, ctx, gm, leader)
	recallBot(t, ctx, tg, gm, name)

	// The leader joins the waiting list, as its window does, then opens a
	// room with places for levels 1-80.
	if err := leader.Send(ctx, botclient.EncodeRequestListPartyWaiting(botclient.RoomsAnywhere, botclient.RoomsAnyLevel)); err != nil {
		t.Fatal(err)
	}
	if _, err := leader.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.PartyMatchList)
		return ok
	}); err != nil {
		t.Fatalf("the leader's rooms list: %v", err)
	}
	if err := leader.Send(ctx, botwire.EncodeRequestManagePartyRoom(0, 4, 1, 80, 0, "e2e room")); err != nil {
		t.Fatal(err)
	}
	if _, err := leader.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == msgPartyRoomCreated
	}); err != nil {
		t.Fatalf("the leader's room opened: %v", err)
	}

	refused := RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestJoinPartyRoom },
		Messages: []int32{msgCantEnterPartyRoom},
	}
	from := w.Len()
	untilServedBy(t, ctx, name+" enters the leader's room", time.Now().Add(roomJoinBound), w, refused, func() string {
		for _, s := range w.Seen(from) {
			if _, ok := s.Event.(botclient.PartyMatchDetail); ok {
				return ""
			}
		}
		return "the room's terms (PartyMatchDetail)"
	})
	if _, err := leader.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == msgEnteredPartyRoom && m.Text[0] == name
	}); err != nil {
		t.Fatalf("the leader hears %s entered its room: %v", name, err)
	}

	if r := invite(t, ctx, leader, name); r != 1 {
		t.Fatalf("%s answered the room leader's invitation %d, want 1", name, r)
	}
	watchUntil(t, ctx, leader, "the room leader leading the bot's party", leaderIs(b, leader.Self.Name), nil)
}
