package botclient

import (
	"math/rand/v2"
	"testing"
	"time"
)

const (
	claimLeaderID = int32(0x20000101)
	claimOtherID  = int32(0x20000102)
	claimGMID     = int32(0x20000103)
	claimBot0     = int32(0x10000101)
	claimBot1     = int32(0x10000102)
)

// newClaimBrain is a started brain of bot self that accepts every
// invitation, sharing claims, with Leader, Other and the commander Gm in
// sight.
func newClaimBrain(t *testing.T, self int32, claims *PartyClaims) (*Brain, *recordSender) {
	t.Helper()
	w, err := NewWorld(Self{ObjectID: self, Name: "bot"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	w.Apply(UserInfo{ObjectID: self, Name: "bot", RunSpeed: 100, WalkSpeed: 50, Running: true, HP: 100, MaxHP: 100, MP: 100, MaxMP: 100}, t0)
	for id, name := range map[int32]string{claimLeaderID: "Leader", claimOtherID: "Other", claimGMID: "Gm"} {
		w.Apply(CharInfo{ObjectID: id, Name: name, Pos: Point{Y: 1000}, RunSpeed: 100, Running: true}, t0)
	}
	cfg := BrainConfig{Persona: Persona{JoinParty: acceptParty}, Rand: rand.New(rand.NewPCG(1, 2))}
	claims.Apply(&cfg)
	cfg.Persona = NewAuthority(AuthorityConfig{Commanders: []int32{claimGMID}}).Hooks(cfg.Persona)
	out := &recordSender{}
	b := NewBrain(w, NewEvents(64), out, newFakeClock(t0), cfg)
	b.start(t0)
	return b, out
}

// partyWindow is the party window of a party led by leader, with Leader and
// Other as the bot's mates.
func partyWindow(leader int32) PartySmallWindowAll {
	return PartySmallWindowAll{LeaderID: leader, Loot: 1, Members: []PartyMember{
		{ObjectID: claimLeaderID, Name: "Leader"}, {ObjectID: claimOtherID, Name: "Other"},
	}}
}

// TestPartyClaims checks that at most one bot of a process joins each
// non-GM leader's party, that a GM is never limited, and that the claim is
// given back when the party ends, the invitation lapses, the leader changes
// or the bot leads.
func TestPartyClaims(t *testing.T) {
	type step struct {
		bot int // 0 or 1
		at  time.Duration
		// ev is handled at at; an AskJoinParty is then answered at its
		// reaction, and want is the answer. A nil ev wakes the bot at at.
		ev   Event
		want bool
	}
	invite := func(name string) Event { return AskJoinParty{Requester: name, Loot: 1} }
	tests := []struct {
		name  string
		steps []step
		held  int // leaders held at the end
	}{
		{"second bot declines, accepted after the first leaves", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 1, at: 3 * time.Second, ev: invite("Leader"), want: false},
			{bot: 0, at: 4 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 1, at: 6 * time.Second, ev: invite("Leader"), want: false},
			{bot: 0, at: 8 * time.Second, ev: PartySmallWindowDeleteAll{}},
			{bot: 1, at: 9 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
		{"other leaders are not limited", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 1, at: 3 * time.Second, ev: invite("Other"), want: true},
		}, 2},
		{"a GM is never limited", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Gm"), want: true},
			{bot: 1, at: 3 * time.Second, ev: invite("Gm"), want: true},
		}, 0},
		{"out of sight requester is keyed by name", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Far"), want: true},
			{bot: 1, at: 3 * time.Second, ev: invite("Far"), want: false},
		}, 1},
		{"own leader inviting again is accepted", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 3 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
		{"invitation never answered by a window lapses", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 10 * time.Second},
			{bot: 1, at: 12 * time.Second, ev: invite("Leader"), want: false},
			{bot: 0, at: 18 * time.Second},
			{bot: 1, at: 19 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
		{"joined party keeps the claim past the timeout", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 4 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 0, at: 30 * time.Second},
			{bot: 1, at: 32 * time.Second, ev: invite("Leader"), want: false},
		}, 1},
		{"leader change moves the claim", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 4 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 0, at: 6 * time.Second, ev: partyWindow(claimOtherID)},
			{bot: 1, at: 8 * time.Second, ev: invite("Other"), want: false},
			{bot: 1, at: 10 * time.Second, ev: invite("Leader"), want: true},
		}, 2},
		{"leader change to a held leader gives the claim back", []step{
			{bot: 1, at: 1 * time.Second, ev: invite("Other"), want: true},
			{bot: 0, at: 3 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 5 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 0, at: 7 * time.Second, ev: partyWindow(claimOtherID)},
			{bot: 1, at: 9 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
		{"bot leading gives the claim back", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Leader"), want: true},
			{bot: 0, at: 4 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 0, at: 6 * time.Second, ev: partyWindow(claimBot0)},
			{bot: 1, at: 8 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
		{"GM party takes no claim on a leader change", []step{
			{bot: 0, at: 1 * time.Second, ev: invite("Gm"), want: true},
			{bot: 0, at: 4 * time.Second, ev: partyWindow(claimGMID)},
			{bot: 0, at: 6 * time.Second, ev: partyWindow(claimLeaderID)},
			{bot: 1, at: 8 * time.Second, ev: invite("Leader"), want: true},
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := NewPartyClaims()
			b0, out0 := newClaimBrain(t, claimBot0, claims)
			b1, out1 := newClaimBrain(t, claimBot1, claims)
			bots := []struct {
				b   *Brain
				out *recordSender
			}{{b0, out0}, {b1, out1}}
			for i, s := range tt.steps {
				bot := bots[s.bot]
				now := at(s.at)
				if s.ev == nil {
					if err := bot.b.wake(now); err != nil {
						t.Fatal(err)
					}
					continue
				}
				if err := bot.b.handle([]Event{s.ev}, now); err != nil {
					t.Fatal(err)
				}
				if _, ok := s.ev.(AskJoinParty); !ok {
					continue
				}
				sent := len(bot.out.frames)
				react := bot.b.decisionAt()
				if react.Before(now) {
					react = now
				}
				if err := bot.b.wake(react); err != nil {
					t.Fatal(err)
				}
				var answers [][]byte
				for _, f := range bot.out.frames[sent:] {
					if f[0] == opGameRequestAnswerJoinParty {
						answers = append(answers, f)
					}
				}
				want := encodeRequestAnswerJoinParty(s.want)
				if len(answers) != 1 || string(answers[0]) != string(want) {
					t.Fatalf("step %d: bot %d answered % x, want % x", i, s.bot, answers, want)
				}
			}
			if got := claims.Held(); got != tt.held {
				t.Fatalf("%d leaders held, want %d", got, tt.held)
			}
		})
	}
}

// TestPartyClaimRunReleases checks that the brain stopping gives its claim
// back.
func TestPartyClaimRunReleases(t *testing.T) {
	claims := NewPartyClaims()
	b, _ := newClaimBrain(t, claimBot0, claims)
	if !claims.claim("Leader", claimBot0, t0) {
		t.Fatal("claim refused on an empty table")
	}
	done := make(chan struct{})
	close(done)
	if err := b.Run(done, nil); err != nil {
		t.Fatal(err)
	}
	if got := claims.Held(); got != 0 {
		t.Fatalf("%d leaders held after Run, want 0", got)
	}
}

// TestPartyClaimsApplyNilJoin checks that a persona declining every
// invitation still declines, and the brain still releases.
func TestPartyClaimsApplyNilJoin(t *testing.T) {
	claims := NewPartyClaims()
	var cfg BrainConfig
	claims.Apply(&cfg)
	if cfg.Persona.JoinParty != nil || cfg.PartyClaims != claims {
		t.Fatalf("JoinParty set %v, PartyClaims %p, want nil and %p", cfg.Persona.JoinParty != nil, cfg.PartyClaims, claims)
	}
}
