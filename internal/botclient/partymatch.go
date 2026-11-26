package botclient

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Party matching (Alt+L) as a partyless support bot uses it: it lists the
// rooms players opened near it, asks to enter one, and waits there for the
// leader's party invitation (docs/parties/rooms.md). Server paths are under
// internal/gameserver: network/partymatch.go, partymatch/partymatch.go.

// The window filters a list request and a room join name (partymatch
// NearMe, LevelsMine): the rooms whose leader stands in the asker's world
// region and whose level range holds the asker's level.
const (
	roomsNearMe     int32 = -2
	roomsLevelsMine int32 = 0
)

// The party-matching system messages the bot reads
// (serverpackets/partymatch.go).
const (
	// msgCantEnterPartyRoom refuses a room join: no such room, a full one,
	// or a level out of its range.
	msgCantEnterPartyRoom = 1413
)

// Room pacing. They are operational limits, not persona defaults (D6): no
// RoomPolicy lists or asks faster.
const (
	// roomListMin is the shortest RoomPolicy.ListEvery. The server limits
	// RequestListPartyWaiting by the frame flood gate alone
	// (network/client_dispatch.go registers it whenLive), so each bot's
	// cadence and roomListGap are the only bounds on the listing load.
	roomListMin = 30 * time.Second
	// roomListGap spaces the RequestListPartyWaiting of every bot of a
	// process (PartyClaims.listTurn): a swarm lists at most two rooms
	// windows a second, however many bots it runs.
	roomListGap = 500 * time.Millisecond
	// roomListAnswer is how long the bot waits for the PartyMatchList its
	// request asks before it leaves the waiting list without one.
	roomListAnswer = 5 * time.Second
	// roomRetryMin is the shortest RoomPolicy.Retry: one room join per bot
	// per leader within it.
	roomRetryMin = 2 * time.Minute
)

// RoomPolicy is how a partyless support bot volunteers for the
// party-matching rooms players open (rule "rooms", docs/parties/rooms.md).
// Persona defaults (D6); roomListMin and roomRetryMin floor the timings.
type RoomPolicy struct {
	// Join enables the rule. Only a RoleSupport persona with a JoinParty
	// hook in a process with a claim table volunteers.
	Join bool
	// ListEvery is the time between two room lists of the bot, plus a
	// random part of up to half of it; the first list comes a random part
	// of it after the bot may first volunteer.
	ListEvery time.Duration
	// Wait is how long the bot waits for the leader's party invitation
	// after it asked to enter the room; then it leaves the room.
	Wait time.Duration
	// Retry is how long a leader whose room the bot asked to enter, or
	// which refused it, waits before the bot asks its room again.
	Retry time.Duration
}

// defaultRoomPolicy is DefaultPersona's.
func defaultRoomPolicy() RoomPolicy {
	return RoomPolicy{Join: true, ListEvery: 2 * time.Minute, Wait: 2 * time.Minute, Retry: 15 * time.Minute}
}

// withDefaults is p with each timing not above zero the default's, and
// none below its floor.
func (p RoomPolicy) withDefaults() RoomPolicy {
	d := defaultRoomPolicy()
	return RoomPolicy{
		Join:      p.Join,
		ListEvery: max(positiveOr(p.ListEvery, d.ListEvery), roomListMin),
		Wait:      positiveOr(p.Wait, d.Wait),
		Retry:     max(positiveOr(p.Retry, d.Retry), roomRetryMin),
	}
}

// PartyRoom is one row of the rooms list.
type PartyRoom struct {
	ID         int32
	Title      string
	Location   int32
	MinLevel   int32
	MaxLevel   int32
	Members    int32
	MaxMembers int32
	Leader     string
}

// PartyMatchList answers RequestListPartyWaiting: the rooms the asker may
// browse, none full (partymatch Rooms).
type PartyMatchList struct{ Rooms []PartyRoom }

// PartyMatchDetail shows the terms of the room the receiver is in: sent
// when it enters one, and when the room's terms change.
type PartyMatchDetail struct {
	RoomID, MaxMembers, MinLevel, MaxLevel, Loot, Location int32
	Title                                                  string
}

// ExClosePartyRoom closes the receiver's room window: it left the room,
// was ousted, or the room was disbanded.
type ExClosePartyRoom struct{}

func (PartyMatchList) event()   {}
func (PartyMatchDetail) event() {}
func (ExClosePartyRoom) event() {}

// decodePartyMatchList: a flag (1 when rows follow), the row count, then
// per row id, title, location, min level, max level, members, max members
// and leader name (FramePartyMatchList). The count comes off the wire, so
// the loop ends at the first short read.
func decodePartyMatchList(r *wire.Reader) Event {
	var e PartyMatchList
	r.Skip(4)
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		e.Rooms = append(e.Rooms, PartyRoom{
			ID: r.ReadInt32(), Title: r.ReadString(), Location: r.ReadInt32(),
			MinLevel: r.ReadInt32(), MaxLevel: r.ReadInt32(),
			Members: r.ReadInt32(), MaxMembers: r.ReadInt32(), Leader: r.ReadString(),
		})
	}
	return e
}

// decodePartyMatchDetail: room id, max members, min level, max level,
// loot, location, title (FramePartyMatchDetail).
func decodePartyMatchDetail(r *wire.Reader) Event {
	return PartyMatchDetail{
		RoomID: r.ReadInt32(), MaxMembers: r.ReadInt32(), MinLevel: r.ReadInt32(),
		MaxLevel: r.ReadInt32(), Loot: r.ReadInt32(), Location: r.ReadInt32(), Title: r.ReadString(),
	}
}

// encodeRequestListPartyWaiting opens the rooms window: the server puts the
// asker on the waiting list and answers PartyMatchList with the rooms at
// location whose level range levelMode filters. auto is unused by the
// server.
func encodeRequestListPartyWaiting(auto, location, levelMode int32) []byte {
	w := wire.NewPacketWriter(opGameRequestListPartyWaiting)
	w.WriteInt32(auto)
	w.WriteInt32(location)
	w.WriteInt32(levelMode)
	return w.Bytes()
}

// encodeRequestManagePartyRoom opens a room (roomID 0) or revises the room
// roomID the sender leads.
func encodeRequestManagePartyRoom(roomID, maxMembers, minLevel, maxLevel, loot int32, title string) []byte {
	w := wire.NewPacketWriter(opGameRequestManagePartyRoom)
	w.WriteInt32(roomID)
	w.WriteInt32(maxMembers)
	w.WriteInt32(minLevel)
	w.WriteInt32(maxLevel)
	w.WriteInt32(loot)
	w.WriteString(title)
	return w.Bytes()
}

// encodeRequestJoinPartyRoom enters the room roomID (0: the first room
// location and levelMode list). A refusal is system message 1413; a sender
// off the waiting list enters nothing and hears nothing.
func encodeRequestJoinPartyRoom(roomID, location, levelMode int32) []byte {
	w := wire.NewPacketWriter(opGameRequestJoinPartyRoom)
	w.WriteInt32(roomID)
	w.WriteInt32(location)
	w.WriteInt32(levelMode)
	return w.Bytes()
}

// encodeRequestWithdrawPartyRoom leaves the room roomID: the room id, then
// an int the server reads and drops (clientpackets decodePartyRoomRef). A
// member of the leader's party stays.
func encodeRequestWithdrawPartyRoom(roomID int32) []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestWithdrawPartyRoom)
	w.WriteInt32(roomID)
	w.WriteInt32(0)
	return w.Bytes()
}

// encodeRequestExitPartyMatchingWaitingRoom takes the sender off the
// waiting list; it has no body and no answer.
func encodeRequestExitPartyMatchingWaitingRoom() []byte {
	w := wire.NewPacketWriter(opGameClientExtended)
	w.WriteUint16(opGameRequestExitPartyMatchingWaitingRoom)
	return w.Bytes()
}

// roomVolunteer is the bot's part in the party-matching rooms (rule
// "rooms"). It is owned by the brain goroutine.
type roomVolunteer struct {
	// next is when the next list is due; zero until the bot first may
	// volunteer.
	next time.Time
	// listedAt is when the list request in flight went out, zero for none;
	// list is its answer, got set once it came.
	listedAt time.Time
	list     []PartyRoom
	got      bool
	// waiting is set while the server holds the bot on its waiting list:
	// from a list request until the bot leaves it or enters a room.
	waiting bool
	// pick is the room the join the rule returned asks for; asked is the
	// room the bot asked to enter, and when, until the party invitation
	// came or its wait ran out.
	pick  PartyRoom
	asked roomAsk
	// inRoom is the room the bot entered (PartyMatchDetail), 0 for none;
	// roomLeader is the leader of the room the bot asked to enter there,
	// empty for a room it did not ask.
	inRoom     int32
	roomLeader string
	// retry holds each leader the bot asked, until it may ask again.
	retry holds[string]
}

// roomAsk is a room join the bot sent.
type roomAsk struct {
	room   int32
	leader string
	at     time.Time
}

// roomListAction asks for the rooms near the bot that its level fits.
func roomListAction() Action {
	return Action{kind: actRoomList, payload: encodeRequestListPartyWaiting(1, roomsNearMe, roomsLevelsMine)}
}

// ruleRooms runs a partyless support bot's room volunteering
// (docs/parties/rooms.md): every RoomPolicy.ListEvery it lists the rooms
// near it, asks to enter the first one a player leads that has a place,
// fits its level, no other bot of the process claimed, and whose leader it
// did not ask within RoomPolicy.Retry, claiming the leader; then it waits
// RoomPolicy.Wait for the leader's party invitation, which rule "prompt"
// answers through the persona's JoinParty. It leaves the waiting list
// after every list that ends in no join, and a room whose invitation never
// came. The steps that leave run whatever the bot does; a list goes out
// only while the bot may volunteer.
//
// Pacing: one RequestListPartyWaiting per ListEvery (at least roomListMin)
// per bot and per roomListGap per process (PartyClaims.listTurn); one
// RequestJoinPartyRoom per list, and per leader per Retry (at least
// roomRetryMin); one exit per list or refusal and one withdrawal per room.
// All under the frame bucket; the server gates none of them beyond the
// frame flood limits (network/client_dispatch.go).
func (b *Brain) ruleRooms(now time.Time) (Action, bool) {
	v, pol := &b.rooms, b.persona.Rooms.withDefaults()
	inParty := b.world.Party.In()
	if v.asked.leader != "" && (inParty || now.Sub(v.asked.at) >= pol.Wait) {
		if !inParty {
			b.roomGiveUp(now, pol)
		}
		v.asked = roomAsk{}
	}
	if v.asked.leader != "" {
		return Action{}, false
	}
	// The bot leaves its room once it is in no party, or in one its room's
	// leader does not lead (it took another invitation): the server takes
	// it out only when it leaves the party (partymatch LeaveWithParty).
	if v.inRoom != 0 && (!inParty || v.roomLeader != "" && b.ledByOther(v.roomLeader)) {
		return Action{kind: actRoomWithdraw, payload: encodeRequestWithdrawPartyRoom(v.inRoom)}, true
	}
	if !v.listedAt.IsZero() && (v.got || now.Sub(v.listedAt) >= roomListAnswer) {
		if !b.mayVolunteer(now) {
			v.pick = PartyRoom{}
		} else if r, ok := b.pickRoom(now); ok {
			v.pick = r
			return Action{kind: actRoomJoin, payload: encodeRequestJoinPartyRoom(r.ID, roomsNearMe, roomsLevelsMine)}, true
		}
		v.pick, v.listedAt, v.list, v.got = PartyRoom{}, time.Time{}, nil, false
	}
	if !v.listedAt.IsZero() {
		return Action{}, false
	}
	if v.waiting {
		return Action{kind: actRoomExit, payload: encodeRequestExitPartyMatchingWaitingRoom()}, true
	}
	if !b.mayVolunteer(now) {
		return Action{}, false
	}
	if v.next.IsZero() {
		v.next = now.Add(time.Duration(b.rng.Int64N(int64(pol.ListEvery) + 1)))
	}
	if now.Before(v.next) || !b.cfg.PartyClaims.listTurn(now) {
		return Action{}, false
	}
	return roomListAction(), true
}

// ledByOther reports whether the bot's party has a leader of a known name
// other than leader.
func (b *Brain) ledByOther(leader string) bool {
	p, self := &b.world.Party, &b.world.Self
	name := self.Name
	if p.LeaderID != self.ObjectID {
		m := p.Members[p.LeaderID]
		if m == nil || m.Name == "" {
			return false
		}
		name = m.Name
	}
	return !strings.EqualFold(name, leader)
}

// mayVolunteer reports whether the bot may list rooms and ask to enter one
// at now: its persona volunteers (RoomPolicy.Join, RoleSupport, a JoinParty
// hook) in a process with a claim table; it is alive, out of combat, in no
// party, holds no party slot or claim, has no invitation open, and runs no
// errand or trip.
func (b *Brain) mayVolunteer(now time.Time) bool {
	c, p, w := b.cfg.PartyClaims, &b.persona, b.world
	if c == nil || !p.Rooms.Join || p.Role != RoleSupport || p.JoinParty == nil {
		return false
	}
	if w.Self.Dead || w.Self.InCombat || w.Party.In() || b.slotted() || w.Prompts.Party.Pending(now) {
		return false
	}
	// A claim of the bot's own room join is no other party's.
	if leader, _, ok := c.held(w.Self.ObjectID); ok && leader != b.rooms.pick.Leader && leader != b.rooms.asked.leader {
		return false
	}
	return !b.onErrand()
}

// onErrand reports whether an errand or a trip runs: one errandRunning
// covers, a trainer, shop, class, dye or Newbie Guide visit, a gatekeeper
// or supply trip, or a valuables round.
func (b *Brain) onErrand() bool {
	return b.errandRunning() || b.train.npc != 0 || b.shop.v.active() || b.class.v.active() ||
		b.dyes.v.active() || b.newbie.v.active() || b.trip.v.active() || b.supplyTrip.active() || b.pvp.arena.active() ||
		b.round.phase != roundIdle
}

// pickRoom returns the first room of the list in flight the bot may ask to
// enter at now, claiming its leader for the bot: a player leads it (no
// name with the bot marker, so no bot party), it has a place, the bot's
// level fits its range, its leader waits for no retry and no other bot of
// the process holds the leader.
func (b *Brain) pickRoom(now time.Time) (PartyRoom, bool) {
	v, self, c := &b.rooms, &b.world.Self, b.cfg.PartyClaims
	if c == nil {
		return PartyRoom{}, false
	}
	for _, r := range v.list {
		if r.Leader == "" || strings.EqualFold(r.Leader, self.Name) || b.cfg.markers.Marks(r.Leader) ||
			r.Members >= r.MaxMembers || self.Level < r.MinLevel || self.Level > r.MaxLevel ||
			v.retry.held(r.Leader, now) {
			continue
		}
		if c.claim(r.Leader, self.ObjectID, now) {
			return r, true
		}
	}
	return PartyRoom{}, false
}

// roomGiveUp ends the room join in flight with no party from it: its
// leader waits pol.Retry, the bot gives its claim on the leader back, and
// its next list waits a whole ListEvery from now.
func (b *Brain) roomGiveUp(now time.Time, pol RoomPolicy) {
	v := &b.rooms
	v.next = b.nextRoomList(now, pol)
	v.retry.prune(now)
	v.retry.set(v.asked.leader, now.Add(pol.Retry))
	if c := b.cfg.PartyClaims; c != nil {
		if leader, _, ok := c.held(b.world.Self.ObjectID); ok && leader == v.asked.leader {
			c.release(b.world.Self.ObjectID)
		}
	}
}

// nextRoomList is when the list after one at now is due: ListEvery and a
// random part of up to half of it later.
func (b *Brain) nextRoomList(now time.Time, pol RoomPolicy) time.Time {
	return now.Add(pol.ListEvery + time.Duration(b.rng.Int64N(int64(pol.ListEvery/2)+1)))
}

// roomEvent takes the answers to the bot's room requests: the list it
// asked, the room it entered or left, a refused join.
func (b *Brain) roomEvent(ev Event, now time.Time) {
	v := &b.rooms
	switch e := ev.(type) {
	case PartyMatchList:
		if !v.listedAt.IsZero() && !v.got {
			v.list, v.got = e.Rooms, true
		}
	case PartyMatchDetail:
		// Entering a room takes the bot off the waiting list
		// (partymatch join). A detail of the room the bot is in revises its
		// terms and keeps its leader.
		switch {
		case v.asked.leader != "" && v.asked.room == e.RoomID:
			v.roomLeader = v.asked.leader
		case e.RoomID != v.inRoom:
			v.roomLeader = ""
		}
		v.inRoom, v.waiting = e.RoomID, false
	case ExClosePartyRoom:
		// Ousted, or the room disbanded: the server may have put the bot
		// back on the waiting list, which an exit leaves at no cost.
		v.inRoom, v.roomLeader, v.waiting = 0, "", true
	case SystemMessage:
		if e.ID == msgCantEnterPartyRoom && v.asked.leader != "" {
			// A refused join leaves the bot on the waiting list.
			b.roomGiveUp(now, b.persona.Rooms.withDefaults())
			v.asked = roomAsk{}
		}
	}
}

// roomSent records a room request the brain sent at now.
func (b *Brain) roomSent(a Action, now time.Time) {
	v, pol := &b.rooms, b.persona.Rooms.withDefaults()
	switch a.kind {
	case actRoomList:
		v.listedAt, v.list, v.got, v.waiting = now, nil, false, true
		v.next = b.nextRoomList(now, pol)
	case actRoomJoin:
		v.asked = roomAsk{room: v.pick.ID, leader: v.pick.Leader, at: now}
		v.pick, v.listedAt, v.list, v.got = PartyRoom{}, time.Time{}, nil, false
	case actRoomExit:
		v.waiting = false
	case actRoomWithdraw:
		v.inRoom, v.roomLeader = 0, ""
	}
}
