package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
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

// TestLivePartyRoomJoin has a GM, a player to the bot (no commander), open a
// party-matching room next to a partyless support bot: the bot lists the
// rooms near it, asks to enter the GM's room, and accepts the GM's party
// invitation through the claim table (#438).
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
	recallBot(t, ctx, tg, gm, name)

	// The GM joins the waiting list, as its window does, then opens a room
	// with places for levels 1-80.
	if err := gm.Send(ctx, botclient.EncodeRequestListPartyWaiting(botclient.RoomsAnywhere, botclient.RoomsAnyLevel)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.PartyMatchList)
		return ok
	}); err != nil {
		t.Fatalf("GM's rooms list: %v", err)
	}
	if err := gm.Send(ctx, botclient.EncodeRequestManagePartyRoom(0, 4, 1, 80, 0, "e2e room")); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == msgPartyRoomCreated
	}); err != nil {
		t.Fatalf("GM's room opened: %v", err)
	}

	refused := RefusalCheck{
		From:     len(w.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpRequestJoinPartyRoom },
		Messages: []int32{msgCantEnterPartyRoom},
	}
	from := w.Len()
	untilServedBy(t, ctx, name+" enters the GM's room", time.Now().Add(roomJoinBound), w, refused, func() string {
		for _, s := range w.Seen(from) {
			if _, ok := s.Event.(botclient.PartyMatchDetail); ok {
				return ""
			}
		}
		return "the room's terms (PartyMatchDetail)"
	})
	if _, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == msgEnteredPartyRoom && m.Text[0] == name
	}); err != nil {
		t.Fatalf("GM hears %s entered its room: %v", name, err)
	}

	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered the room leader's invitation %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)
}
