package botclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

const (
	clanID    = int32(0x0a000001)
	otherClan = int32(0x0a000002)
)

func TestWorldClan(t *testing.T) {
	invited := SystemMessage{ID: msgInvitedToClan, Text: [2]string{"Alden", "Crafters"}}
	steps := []struct {
		name       string
		ev         Event
		wantInvite ClanInvite
		wantClan   Clan
	}{
		{"other system message", SystemMessage{ID: 9, Text: [2]string{"x", "Crafters"}}, ClanInvite{}, Clan{}},
		{"invitation's system message", invited, ClanInvite{}, Clan{}},
		{
			"requester named by the system message",
			AskJoinPledge{RequesterID: 0x20000099, Clan: "Crafters"},
			ClanInvite{Open: true, RequesterID: 0x20000099, Requester: "Alden", Clan: "Crafters", At: at(2)},
			Clan{},
		},
		{
			"the system message serves one invitation",
			AskJoinPledge{RequesterID: 0x20000099, Clan: "Crafters"},
			ClanInvite{Open: true, RequesterID: 0x20000099, Clan: "Crafters", At: at(3)},
			Clan{},
		},
		{"system message naming another clan", invited, ClanInvite{Open: true, RequesterID: 0x20000099, Clan: "Crafters", At: at(3)}, Clan{}},
		{
			"requester unknown when the clans differ",
			AskJoinPledge{RequesterID: 0x20000099, Clan: "Smiths"},
			ClanInvite{Open: true, RequesterID: 0x20000099, Clan: "Smiths", At: at(5)},
			Clan{},
		},
		{
			"requester named by the object in sight",
			AskJoinPledge{RequesterID: pcID, Clan: "Smiths"},
			ClanInvite{Open: true, RequesterID: pcID, Requester: "player", Clan: "Smiths", At: at(6)},
			Clan{},
		},
		{"joined", JoinPledge{ClanID: clanID}, ClanInvite{}, Clan{ID: clanID}},
		{"sub-unit roster keeps the main", PledgeShowMemberListAll{ClanID: clanID, PledgeType: 100, Name: "Guard", Leader: "Captain"}, ClanInvite{}, Clan{ID: clanID}},
		{"main roster", PledgeShowMemberListAll{ClanID: clanID, Name: "Crafters", Leader: "Alden"}, ClanInvite{}, Clan{ID: clanID, Name: "Crafters", Leader: "Alden"}},
		{"UserInfo of the clan keeps it", UserInfo{ObjectID: selfID, Name: "Self", ClanID: clanID}, ClanInvite{}, Clan{ID: clanID, Name: "Crafters", Leader: "Alden"}},
		{"JoinPledge of the clan keeps it", JoinPledge{ClanID: clanID}, ClanInvite{}, Clan{ID: clanID, Name: "Crafters", Leader: "Alden"}},
		{"UserInfo clanless clears", UserInfo{ObjectID: selfID, Name: "Self"}, ClanInvite{}, Clan{}},
		{"UserInfo of another clan", UserInfo{ObjectID: selfID, Name: "Self", ClanID: otherClan}, ClanInvite{}, Clan{ID: otherClan}},
		{"roster re-read at entry", PledgeShowMemberListAll{ClanID: otherClan, Name: "Smiths", Leader: "Boss"}, ClanInvite{}, Clan{ID: otherClan, Name: "Smiths", Leader: "Boss"}},
		{"left", PledgeShowMemberListDeleteAll{}, ClanInvite{}, Clan{}},
	}
	w := newTestWorld(t)
	w.Apply(CharInfo{ObjectID: pcID, Name: "player"}, t0)
	for i, s := range steps {
		w.Apply(s.ev, at(time.Duration(i)))
		if w.Prompts.Clan != s.wantInvite {
			t.Fatalf("%s: invite %+v, want %+v", s.name, w.Prompts.Clan, s.wantInvite)
		}
		if w.Clan != s.wantClan {
			t.Fatalf("%s: clan %+v, want %+v", s.name, w.Clan, s.wantClan)
		}
	}
}

func TestBrainClanInvite(t *testing.T) {
	accept := botwire.EncodeRequestAnswerJoinPledge(true)
	decline := botwire.EncodeRequestAnswerJoinPledge(false)
	auth := func(p Persona) Persona { return NewAuthority(AuthorityConfig{Commanders: []int32{gmID}}).Hooks(p) }
	tests := []struct {
		name      string
		persona   Persona
		inClan    bool
		requester int32
		named     string // the system message's inviter, "" for none
		wakeAt    time.Duration
		want      [][]byte
	}{
		{"GM accepted by the default persona", auth(DefaultPersona()), false, gmID, "GM", 0, [][]byte{accept}},
		{"GM in sight, no system message", auth(DefaultPersona()), false, gmID, "", 0, [][]byte{accept}},
		{"player declined by the default persona", auth(DefaultPersona()), false, pcID, "player", 0, [][]byte{decline}},
		{"player accepted by a clanless crafter", auth(Persona{JoinClan: AcceptClanFrom()}), false, pcID, "player", 0, [][]byte{accept}},
		{"player out of sight accepted by a clanless crafter", Persona{JoinClan: AcceptClanFrom()}, false, 0x20000099, "Alden", 0, [][]byte{accept}},
		{"player declined by a crafter in a clan", Persona{JoinClan: AcceptClanFrom()}, true, pcID, "player", 0, [][]byte{decline}},
		{"listed inviter accepted", Persona{JoinClan: AcceptClanFrom("Alden", "Player")}, false, pcID, "player", 0, [][]byte{accept}},
		{"unlisted inviter declined", Persona{JoinClan: AcceptClanFrom("Alden")}, false, pcID, "player", 0, [][]byte{decline}},
		{"unknown inviter declined", Persona{JoinClan: AcceptClanFrom()}, false, 0x20000099, "", 0, [][]byte{decline}},
		{"no policy declines", Persona{}, false, gmID, "GM", 0, [][]byte{decline}},
		{"expired before the decision", auth(DefaultPersona()), false, gmID, "GM", clanInviteTimeout, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.persona
			b, out := newTestBrain(t, Persona{JoinClan: p.JoinClan})
			b.world.Apply(CharInfo{ObjectID: gmID, Name: "GM"}, t0)
			b.world.Apply(CharInfo{ObjectID: pcID, Name: "player"}, t0)
			if tt.inClan {
				b.world.Apply(JoinPledge{ClanID: otherClan}, t0)
			}
			ev := at(100 * time.Millisecond)
			var evs []Event
			if tt.named != "" {
				evs = append(evs, SystemMessage{ID: msgInvitedToClan, Text: [2]string{tt.named, "Crafters"}})
			}
			if err := b.handle(append(evs, AskJoinPledge{RequesterID: tt.requester, Clan: "Crafters"}), ev); err != nil {
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
				if f[0] == opGameRequestAnswerJoinPledge {
					got = append(got, f)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("answers % x, want % x", got, tt.want)
			}
			if b.world.Prompts.Clan.Pending(first.Add(4 * time.Second)) {
				t.Fatal("invitation still pending after the answer")
			}
		})
	}
}

// TestBrainClanChatUnanswered: a clan chat line is decoded with its
// channel and gets no reply, whoever says it.
func TestBrainClanChatUnanswered(t *testing.T) {
	b, out := newTestBrain(t, chatPersona())
	if err := b.handle([]Event{CreatureSay{ObjectID: pcID, Type: chatClan, Name: "Alden", Text: "hi"}}, t0); err != nil {
		t.Fatal(err)
	}
	if b.chat.reply.kind != actHold {
		t.Fatalf("reply %v scheduled to clan chat", b.chat.reply.kind)
	}
	for _, w := range []time.Time{at(time.Second), at(30 * time.Second)} {
		if err := b.wake(w); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range out.frames {
		if f[0] == opGameSay2 {
			t.Fatalf("sent Say2 % x after clan chat", f)
		}
	}
}
