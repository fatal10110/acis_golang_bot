package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Channel fixtures: another party of the bot's channel, led by mateLeader,
// with pcID in it, and a third party led by thirdLeader.
const (
	mateLeader  = int32(0x20000101)
	thirdLeader = int32(0x20000201)
	bystander   = int32(0x20000301)
)

// TestDecodeChannelEvents decodes the command channel frames as the server
// lays them out (serverpackets/party.go FrameExOpenMPCC, FrameExCloseMPCC,
// FrameExMPCCPartyInfoUpdate, FrameExMPCCShowPartyMemberInfo;
// usercommand.go FrameExMultiPartyCommandChannelInfo).
func TestDecodeChannelEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ExOpenMPCC", []byte{0xfe, 0x25, 0x00}, ExOpenMPCC{}},
		{"ExCloseMPCC", []byte{0xfe, 0x26, 0x00}, ExCloseMPCC{}},
		// Leader "L", id 0x01020304, 3 members, added.
		{"ExMPCCPartyInfoUpdate added", []byte{
			0xfe, 0x5a, 0x00, 'L', 0, 0, 0, 0x04, 0x03, 0x02, 0x01, 3, 0, 0, 0, 1, 0, 0, 0,
		}, ExMPCCPartyInfoUpdate{LeaderName: "L", LeaderID: 0x01020304, Count: 3, Added: true}},
		{
			"ExMPCCPartyInfoUpdate removed", packet(opGameExtended, opGameExMPCCPartyInfoUpdate, "Lead", mateLeader, 2, 0),
			ExMPCCPartyInfoUpdate{LeaderName: "Lead", LeaderID: mateLeader, Count: 2},
		},
		// One member "A", id 7, class 10.
		{"ExMPCCShowPartyMemberInfo", []byte{
			0xfe, 0x4a, 0x00, 1, 0, 0, 0, 'A', 0, 0, 0, 7, 0, 0, 0, 10, 0, 0, 0,
		}, ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{Name: "A", ObjectID: 7, ClassID: 10}}}},
		{
			"ExMPCCShowPartyMemberInfo two", packet(opGameExtended, opGameExMPCCShowPartyMemberInfo, 2, "Lead", mateLeader, 1, foeName, pcID, 0),
			ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{Name: "Lead", ObjectID: mateLeader, ClassID: 1}, {Name: foeName, ObjectID: pcID}}},
		},
		{"ExMPCCShowPartyMemberInfo empty", packet(opGameExtended, opGameExMPCCShowPartyMemberInfo, 0), ExMPCCShowPartyMemberInfo{}},
		// Leader "C", loot 0, 5 members over 1 party: "C", id 9, 5.
		{"ExMultiPartyCommandChannelInfo", []byte{
			0xfe, 0x30, 0x00, 'C', 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 'C', 0, 0, 0, 9, 0, 0, 0, 5, 0, 0, 0,
		}, ExMultiPartyCommandChannelInfo{LeaderName: "C", Count: 5, Parties: []ChannelPartyRow{{LeaderName: "C", LeaderID: 9, Count: 5}}}},
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

func TestDecodeChannelEventsRejects(t *testing.T) {
	list := packet(opGameExtended, opGameExMPCCShowPartyMemberInfo, 1, "A", 7, 10)
	info := packet(opGameExtended, opGameExMultiPartyCommandChannelInfo, "C", 0, 5, 1, "C", 9, 5)
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"open trailing", []byte{0xfe, 0x25, 0x00, 0}, errTrailingBytes},
		{"close trailing", []byte{0xfe, 0x26, 0x00, 0}, errTrailingBytes},
		{"update short", packet(opGameExtended, opGameExMPCCPartyInfoUpdate, "L", 1, 2), wire.ErrShortPacket},
		{"update trailing", packet(opGameExtended, opGameExMPCCPartyInfoUpdate, "L", 1, 2, 1, 0), errTrailingBytes},
		{"members short row", list[:len(list)-4], wire.ErrShortPacket},
		{"members count past the frame", packet(opGameExtended, opGameExMPCCShowPartyMemberInfo, 0x7fffffff), wire.ErrShortPacket},
		{"members trailing", append(list, 0), errTrailingBytes},
		{"info short row", info[:len(info)-4], wire.ErrShortPacket},
		{"info trailing", append(info, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestEncodeChannelAsks checks the asks against the server's decoders
// (clientpackets/party.go DecodeRequestExMPCCShowPartyMembersInfo: 0xd0,
// sub-opcode 0x0026, leader id; RequestUserCommand: 0xaa, command id).
func TestEncodeChannelAsks(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"members", encodeRequestExMPCCShowPartyMembersInfo(0x01020304), []byte{0xd0, 0x26, 0x00, 0x04, 0x03, 0x02, 0x01}},
		{"overview", encodeRequestUserCommand(userCommandChannelList), []byte{0xaa, 97, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

// channelBrain is a pvpBrain in a party with selfID leading, whose party is
// in a channel with mateLeader's (pcID in it) and thirdLeader's, the
// overview and mateLeader's list answered at t0.
func channelBrain(t *testing.T, arena bool) *Brain {
	t.Helper()
	b := pvpBrain(t, arena, nil)
	b.handleOne(t, CharInfo{ObjectID: bystander, Name: "Other", Pos: Point{X: 100, Y: 300}}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: 900, Name: "Mine", HP: 100, MaxHP: 100}}}, t0)
	b.world.Apply(ExOpenMPCC{}, t0)
	b.world.Apply(ExMultiPartyCommandChannelInfo{LeaderName: "Self", Count: 5, Parties: []ChannelPartyRow{
		{LeaderName: "Self", LeaderID: selfID, Count: 2}, {LeaderName: "Lead", LeaderID: mateLeader, Count: 2}, {LeaderName: "Third", LeaderID: thirdLeader, Count: 1},
	}}, t0)
	b.world.Channel.asked = mateLeader
	b.world.Apply(ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{Name: "Lead", ObjectID: mateLeader}, {Name: foeName, ObjectID: pcID}}}, t0)
	return b
}

// TestPvPArenaChannelMate checks MayFight's rule 2 for a command channel
// mate (docs/pvp/targets.md, #429): in the bot's arena it is neither fought
// nor picked as an arena target, while a player out of the channel is.
func TestPvPArenaChannelMate(t *testing.T) {
	now := at(time.Second)
	arena := func(b *Brain) int { return b.arenaAt(Point{Y: 300}).Arena }

	b := pvpBrain(t, true, nil)
	if k := b.fightKind(pcID, b.world.object(pcID), now); k != FightArena {
		t.Fatalf("before the channel: kind %v, want arena", k)
	}

	b = channelBrain(t, true)
	if arena(b) == 0 {
		t.Fatal("the arena fixture has no arena at the players")
	}
	for _, id := range []int32{pcID, mateLeader, thirdLeader} {
		if !b.channelMate(id) {
			t.Errorf("player %#x is no channel mate", id)
		}
	}
	if k := b.fightKind(pcID, b.world.object(pcID), now); k == FightArena {
		t.Error("a channel mate in the arena: kind arena")
	}
	if b.mayFight(pcID, now) || b.arenaTarget(pcID, arena(b), now) {
		t.Error("a channel mate in the arena may be fought")
	}
	if !b.friend(pcID) {
		t.Error("a channel mate is no friend")
	}
	if k := b.fightKind(bystander, b.world.object(bystander), now); k != FightArena {
		t.Errorf("a player out of the channel: kind %v, want arena", k)
	}
	if !b.arenaTarget(bystander, arena(b), now) {
		t.Error("a player out of the channel is no arena target")
	}

	b.world.Apply(ExCloseMPCC{}, now)
	if b.channelMate(pcID) || b.fightKind(pcID, b.world.object(pcID), now) != FightArena {
		t.Error("after ExCloseMPCC the former channel mate is still spared")
	}
}

// TestChannelModel checks the channel model's updates: a party added and
// removed, a recount making a list due, and leaving the party ending the
// channel.
func TestChannelModel(t *testing.T) {
	b := channelBrain(t, false)
	c := &b.world.Channel
	b.world.Apply(ExMPCCPartyInfoUpdate{LeaderName: "New", LeaderID: 600, Count: 3, Added: true}, t0)
	if !c.Mate(600) || c.Parties[600].Members != nil {
		t.Fatalf("added party: %+v", c.Parties[600])
	}
	b.world.Apply(ExMPCCPartyInfoUpdate{LeaderName: "Third", LeaderID: thirdLeader, Count: 1}, t0)
	if c.Mate(thirdLeader) {
		t.Error("a removed party's leader is still a mate")
	}
	b.world.Apply(ExMultiPartyCommandChannelInfo{Parties: []ChannelPartyRow{
		{LeaderID: selfID, Count: 2}, {LeaderID: mateLeader, Count: 3},
	}}, at(time.Minute))
	if c.Mate(600) {
		t.Error("a party the overview no longer lists is still kept")
	}
	if p := c.Parties[mateLeader]; !p.Members[pcID] || !p.ListedAt.IsZero() {
		t.Errorf("recounted party: %+v, want its members kept and its list due", p)
	}
	b.world.Apply(PartySmallWindowDeleteAll{}, at(time.Minute))
	if c.Open || c.Mate(pcID) {
		t.Error("out of the party the bot is still in a channel")
	}
	b.world.Apply(ExMPCCPartyInfoUpdate{LeaderID: 600, Count: 1, Added: true}, at(time.Minute))
	if c.Mate(600) {
		t.Error("an update out of a channel is kept")
	}
}

// TestRuleChannelAsks checks the channel asks and their pacing: the
// overview on opening, retried after channelRetry, then each other party's
// members, one per channelAskGap, and in an arena all again after
// channelRefresh.
func TestRuleChannelAsks(t *testing.T) {
	overview := encodeRequestUserCommand(userCommandChannelList)
	members := encodeRequestExMPCCShowPartyMembersInfo
	type step struct {
		at    time.Duration
		ev    Event
		want  []byte // nil: no ask
		asked int32  // the leader the list that arrives answers
	}
	tests := []struct {
		name  string
		arena bool
		steps []step
	}{
		{"out of an arena", false, []step{
			{at: 0},
			{at: time.Second, ev: ExOpenMPCC{}, want: overview},
			{at: 1500 * time.Millisecond},
			{at: 10 * time.Second},
			{at: time.Second + channelRetry, want: overview},
			{at: 20 * time.Second, ev: ExMultiPartyCommandChannelInfo{Parties: []ChannelPartyRow{
				{LeaderID: selfID, Count: 2}, {LeaderID: thirdLeader, Count: 1}, {LeaderID: mateLeader, Count: 2},
			}}, want: members(mateLeader), asked: mateLeader},
			{at: 20*time.Second + 500*time.Millisecond},
			{at: 21 * time.Second, want: members(thirdLeader), asked: thirdLeader},
			{at: 22 * time.Second, ev: ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{ObjectID: thirdLeader}}}},
			{at: 23 * time.Second, ev: ExMPCCPartyInfoUpdate{LeaderID: 600, Count: 1, Added: true}, want: members(600), asked: 600},
			// mateLeader never answered: asked again after channelRetry.
			{at: 20*time.Second + channelRetry, want: members(mateLeader), asked: mateLeader},
			{at: 36 * time.Second, ev: ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{ObjectID: mateLeader}, {ObjectID: pcID}}}},
			// 600's list never came either: asked again, then answered.
			{at: 23*time.Second + channelRetry, want: members(600), asked: 600},
			{at: 39 * time.Second, ev: ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{ObjectID: 600}}}},
			// Out of an arena a list is not refreshed.
			{at: 5 * time.Minute},
			{at: 6 * time.Minute, ev: ExCloseMPCC{}},
		}},
		{"in an arena", true, []step{
			{at: 0, ev: ExOpenMPCC{}, want: overview},
			{
				at: time.Second, ev: ExMultiPartyCommandChannelInfo{Parties: []ChannelPartyRow{{LeaderID: selfID, Count: 2}, {LeaderID: mateLeader, Count: 2}}},
				want: members(mateLeader), asked: mateLeader,
			},
			{at: 2 * time.Second, ev: ExMPCCShowPartyMemberInfo{Members: []ChannelMember{{ObjectID: mateLeader}, {ObjectID: pcID}}}},
			{at: time.Second + channelRefresh - time.Millisecond},
			{at: time.Second + channelRefresh, want: overview},
			{at: time.Second + channelRefresh + 100*time.Millisecond, ev: ExMultiPartyCommandChannelInfo{Parties: []ChannelPartyRow{
				{LeaderID: selfID, Count: 2}, {LeaderID: mateLeader, Count: 2},
			}}},
			{at: 2*time.Second + channelRefresh, want: members(mateLeader), asked: mateLeader},
			{at: 3*time.Second + channelRefresh},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := pvpBrain(t, tt.arena, nil)
			b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: 900, HP: 100, MaxHP: 100}}}, t0)
			for _, s := range tt.steps {
				now := at(s.at)
				if s.ev != nil {
					b.world.Apply(s.ev, now)
				}
				a, ok := b.ruleChannel(now)
				switch {
				case s.want == nil && ok:
					t.Fatalf("%v: ask % x, want none", s.at, a.payload)
				case s.want != nil && (!ok || a.kind != actChannelAsk || !bytes.Equal(a.payload, s.want)):
					t.Fatalf("%v: ask %v % x, want % x", s.at, ok, a.payload, s.want)
				case s.asked != 0 && b.world.Channel.asked != s.asked:
					t.Fatalf("%v: asked %#x, want %#x", s.at, b.world.Channel.asked, s.asked)
				}
			}
			if c := b.world.Channel; !tt.arena && (c.Open || c.Mate(pcID)) {
				t.Errorf("after ExCloseMPCC: %+v", c)
			}
		})
	}
}
