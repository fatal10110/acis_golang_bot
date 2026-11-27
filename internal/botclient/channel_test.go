package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Channel fixtures: another party of the bot's channel, led by mateLeader,
// with pcID in it, and a third party led by thirdLeader.
const (
	mateLeader  = int32(0x20000101)
	thirdLeader = int32(0x20000201)
	bystander   = int32(0x20000301)
)

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
	// A leader change resets the party windows but keeps the channel
	// (server party/party.go changeLeader): no ExOpenMPCC follows.
	b.world.Apply(PartySmallWindowDeleteAll{}, at(time.Minute))
	b.world.Apply(PartySmallWindowAll{LeaderID: 900, Members: []PartyMember{{ObjectID: 900, Name: "Mine", HP: 100, MaxHP: 100}}}, at(time.Minute))
	if !c.Open || !c.Mate(pcID) {
		t.Error("a leader change ended the channel")
	}
	b.world.Apply(ExCloseMPCC{}, at(time.Minute))
	if c.Open || c.Mate(pcID) {
		t.Error("after ExCloseMPCC the bot is still in a channel")
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
	overview := botwire.EncodeRequestUserCommand(userCommandChannelList)
	members := botwire.EncodeRequestExMPCCShowPartyMembersInfo
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
