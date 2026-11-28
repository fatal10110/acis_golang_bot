package botclient

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// priestObj is a Dusk Priestess (NPC 31085) once in sight.
const priestObj = int32(0x20000040)

// signsBot is the test bot's character, the one Seven Signs member.
const signsBot = "Alden"

// The Dusk Priestess's pages as the server sends them (data/html/
// seven_signs, %objectId% filled in, reduced to the text that tells them
// apart and every link), and the status page signspriest.sealStatus
// builds: the recorded frames the sign-up is tested on, as no live
// scenario can reach a period on demand (X18, owner approval on D43).
func priestLink(cmd, text string) string {
	return `<a action="bypass -h npc_` + itoa(priestObj) + `_` + cmd + `">` + text + `</a><br>`
}

var (
	// duskPriest1 is dusk_priest_1.htm, the competition's greeting.
	duskPriest1 = `<html><body>Dusk Priestess:<br>Until the day of revolution! The competition period has begun.<br>` +
		priestLink("SevenSignsDesc 20", "What is the Seven Signs?") + priestLink("SevenSignsDesc 13", "Record") +
		priestLink("SevenSignsDesc 18", "How do I participate?") + priestLink(signsJoin, "I want to participate in the Seven Signs.") +
		priestLink(signsHunting, "I want to teleport to the nearest Necropolis or Catacomb.") + priestLink("SevenSignsDesc 4", "Seal stones") +
		priestLink("SevenSigns 5 1", "I want to contribute seal stones.") + `</body></html>`
	// signs3Dusk is signs_3_dusk.htm.
	signs3Dusk = `<html><body>Dusk Priestess:<br>Will you join the Revolutionaries of Dusk?<br>` +
		priestLink(signsAsk, "Participate.") + priestLink(signsBack, "Go back.") + `</body></html>`
	// signs33Dusk is signs_33_dusk.htm.
	signs33Dusk = `<html><body>Dusk Priestess:<br>[ Choose a seal ]<br>` +
		priestLink("SevenSigns 19 1 1", "Seal of Avarice") + priestLink("SevenSigns 19 1 2", "Seal of Gnosis") +
		priestLink("SevenSigns 19 1 3", "Seal of Strife") + priestLink(signsStatus, "Seal status") +
		priestLink(signsStatus, "Seal status") + `</body></html>`
	// signsStatusPage is sealStatus's page, Dusk holding the Seal of
	// Avarice and the Seal of Strife.
	signsStatusPage = `<html><body>Dusk Priestess:<br><font color="LEVEL">[ Status of the Seals ]</font><br>` +
		`[Seal of Avarice: Revolutionaries of Dusk]<br>[Seal of Gnosis: Lords of Dawn]<br>[Seal of Strife: Revolutionaries of Dusk]<br>` +
		priestLink(signsBack, "Go back.") + `</body></html>`
	// signsNone is sealStatus's page with no seal owned.
	signsNone = `<html><body>Dusk Priestess:<br>[Seal of Avarice: Nothingness]<br>[Seal of Gnosis: Nothingness]<br>` +
		`[Seal of Strife: Nothingness]<br>` + priestLink(signsBack, "Go back.") + `</body></html>`
	// signs19AvariceDusk and signs19GnosisDusk are signs_19_Avarice_dusk.htm
	// and signs_19_Gnosis_dusk.htm.
	signs19AvariceDusk = `<html><body>Dusk Priestess:<br>[ Seal of Avarice ]<br>` +
		priestLink("SevenSigns 4 1 1", "Choose this seal.") + priestLink(signsJoin, "Go back.") + `</body></html>`
	signs19GnosisDusk = `<html><body>Dusk Priestess:<br>[ Seal of Gnosis ]<br>` +
		priestLink("SevenSigns 4 1 2", "Choose this seal.") + priestLink(signsJoin, "Go back.") + `</body></html>`
	// signs4Dusk is signs_4_dusk.htm, the accepted sign-up.
	signs4Dusk = `<html><body>Dusk Priestess:<br>Your application has been accepted. Congratulations! You're now a Revolutionary of Dusk.</body></html>`
	// signs33DuskMember, signs33DuskNo and signs33DuskFirstclass are
	// participate's other answers.
	signs33DuskMember     = `<html><body>Dusk Priestess:<br>[ Participate in the Seven Signs ]<br>You're already a member of Revolutionaries of Dusk!<br>` + priestLink(signsBack, "Go back.") + `</body></html>`
	signs33DuskNo         = `<html><body>Dusk Priestess:<br>[ Participation Denied. ]<br>You must not belong to a clan or alliance that controls a castle to join the Revolutionaries of Dusk!<br>` + priestLink(signsBack, "Go back.") + `</body></html>`
	signs33DuskFirstclass = `<html><body>Dusk Priestess:<br>[ Participation Denied ]<br>Come back after completing your first class transfer.<br>` + priestLink(signsBack, "Go back.") + `</body></html>`
	// duskPriest2a and duskPriest2c are the validation greetings of a
	// member of Dusk, the winner: owning the Seal of Gnosis, and not.
	duskPriest2a = `<html><body>Dusk Priestess:<br>The Revolutionaries of Dusk have vanquished the arrogant Lords of Dawn!<br>[ The seal validation period is underway. ]<br>` +
		priestLink("SevenSignsDesc 1", "What is the Seven Signs?") + priestLink(signsHunting, "Teleport to nearby hunting grounds.") +
		priestLink("multisell 500", "Purchase consumable items.") + priestLink("SevenSigns 16", "Exchange seal stones.") + `</body></html>`
	duskPriest2c = `<html><body>Dusk Priestess:<br>The Revolutionaries of Dusk have vanquished the arrogant Lords of Dawn!<br>[ The seal validation period is underway. ]<br>` +
		priestLink("SevenSignsDesc 20", "What is the Seven Signs?") + priestLink("multisell 500", "Purchase consumable items.") +
		priestLink("SevenSigns 16", "Exchange seal stones.") + `</body></html>`
	// duskPriest6 is dusk_priest_6.htm, recruitment.
	duskPriest6 = `<html><body>Dusk Priestess:<br>The recruitment period is underway.<br>` + priestLink("SevenSignsDesc 13", "Record") + `</body></html>`
)

// newSignsBrain is a level 40 Warrior (a first profession) named
// signsBot at the origin, the one Seven Signs member, with the
// priestess 31085 spawned at (3000, 0).
func newSignsBrain(t *testing.T) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.Newbie.MaxLevel = 0
	p.SevenSigns = SevenSignsPolicy{Members: []string{signsBot}, MaxMembers: 1}
	b, _ := newTestBrain(t, p)
	b.world.Self.Name, b.world.Self.Level, b.world.Self.ClassID = signsBot, 40, 1
	b.cfg.Travel = &TravelMap{spawns: map[int32][]Point{31085: {{X: 3000}}}}
	return walkOn(t, b)
}

// priestInSight puts the priestess in sight 200 units away, selected.
func priestInSight(b *Brain) {
	b.world.Apply(NPCInfo{ObjectID: priestObj, NPCID: 31085, Pos: Point{X: 200}}, t0)
	b.world.Target = priestObj
}

func priestBypass(cmd string) []byte {
	return botwire.EncodeRequestBypassToServer(npcBypass(priestObj, cmd))
}

// TestBrainSevenSignsSignUp walks the sign-up of a member: the status page
// first, then the seal Dusk holds (Avarice, not the preferred Gnosis and
// never Strife), and the accepted page makes it a member in the
// competition. No other link goes out: never the contribution
// (SevenSigns 5, 6) nor the Dawn fee (34).
func TestBrainSevenSignsSignUp(t *testing.T) {
	b := newSignsBrain(t)
	priestInSight(b)
	page := func(html string) []Event { return []Event{NpcHtmlMessage{ObjectID: priestObj, HTML: html}} }
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", at: time.Second, wantRule: "signs", wantKind: actTalk},
		{name: "wait for the page", at: 2 * time.Second, wantRule: "signs"},
		{name: "participate", at: 3 * time.Second, events: page(duskPriest1), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsJoin)},
		{name: "wait for the answer", at: 3500 * time.Millisecond, wantRule: "signs"},
		{name: "ask", at: 4 * time.Second, events: page(signs3Dusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsAsk)},
		{name: "status page first", at: 5 * time.Second, events: page(signs33Dusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsStatus)},
		{name: "back from the status", at: 6 * time.Second, events: page(signsStatusPage), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsBack)},
		{name: "participate again", at: 7 * time.Second, events: page(duskPriest1), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsJoin)},
		{name: "ask again", at: 8 * time.Second, events: page(signs3Dusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass(signsAsk)},
		{name: "the seal Dusk holds", at: 9 * time.Second, events: page(signs33Dusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass("SevenSigns 19 1 1")},
		{name: "sign up", at: 10 * time.Second, events: page(signs19AvariceDusk), wantRule: "signs", wantKind: actBypass, wantPayload: priestBypass("SevenSigns 4 1 1")},
		{name: "accepted: a member, nothing more", at: 11 * time.Second, events: page(signs4Dusk)},
		{name: "not due again soon", at: time.Hour},
	})
	s := &b.signs
	if !s.member || s.period != signsCompetition || s.v.active() {
		t.Fatalf("after the sign-up: member %v period %d visit %+v, want a member in the competition, the visit over", s.member, s.period, s.v)
	}
	if !b.signsEnters(SealGnosis, at(2*time.Hour)) || !b.signsFarms("ssq_event", at(2*time.Hour)) {
		t.Fatal("a member in the competition does not enter the dungeons or farm their spots")
	}
}

// TestSevenSignsVote checks the seal the sign-up votes from the status
// page: the preferred seal when Dusk holds it, else the other seal Dusk
// holds, never Strife, else the preferred one.
func TestSevenSignsVote(t *testing.T) {
	tests := []struct {
		name   string
		status string // "" reads no status page
		pref   Seal
		want   Seal
	}{
		{name: "no status page: the preferred seal", pref: SealGnosis, want: SealGnosis},
		{name: "unset preference: Gnosis", want: SealGnosis},
		{name: "Dusk holds Avarice and Strife: Avarice", status: signsStatusPage, pref: SealGnosis, want: SealAvarice},
		{name: "Dusk holds the preferred Avarice", status: signsStatusPage, pref: SealAvarice, want: SealAvarice},
		{name: "Dusk holds none: the preferred seal", status: signsNone, pref: SealAvarice, want: SealAvarice},
		{
			name: "Dusk holds Strife only: never Strife", pref: SealGnosis, want: SealGnosis,
			status: `[Seal of Avarice: Lords of Dawn]<br>[Seal of Gnosis: Nothingness]<br>[Seal of Strife: Revolutionaries of Dusk]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s signsErrand
			if tt.status != "" {
				if classifySignsPage(tt.status) != signsSealStatus {
					t.Fatalf("status page not told apart: %q", tt.status)
				}
				s.held = duskSeals(tt.status)
			}
			if got := s.vote(SevenSignsPolicy{Seal: tt.pref}.seal()); got != tt.want {
				t.Fatalf("vote = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSevenSignsPages checks what each page of the priest tells the errand
// and whether the visit sends a link on it.
func TestSevenSignsPages(t *testing.T) {
	tests := []struct {
		name       string
		member     bool
		setup      func(s *signsErrand)
		html       string
		wantCmd    string
		wantRest   time.Duration
		wantMember bool
		wantPeriod signsPeriod
		wantGnosis bool
	}{
		{name: "competition, not a member: participate", html: duskPriest1, wantCmd: signsJoin, wantPeriod: signsCompetition},
		{name: "competition, a member: done", member: true, setup: competition, html: duskPriest1, wantRest: signsFresh, wantMember: true, wantPeriod: signsCompetition},
		{
			name: "a member of the last validation: participate again", member: true, html: duskPriest1, wantCmd: signsJoin, wantPeriod: signsCompetition,
			setup: func(s *signsErrand) { s.period, s.readAt = signsValidation, at(-time.Hour) },
		},
		{
			name: "a membership read a cycle ago: participate again", member: true, html: duskPriest1, wantCmd: signsJoin, wantPeriod: signsCompetition,
			setup: func(s *signsErrand) { competition(s); s.readAt = at(-signsCycle) },
		},
		{name: "a member never told a period: participate", member: true, html: duskPriest1, wantCmd: signsJoin, wantPeriod: signsCompetition},
		{name: "recruitment: the membership of a cycle past", member: true, setup: competition, html: duskPriest6, wantRest: signsFresh, wantPeriod: signsOther},
		{name: "already a member", html: signs33DuskMember, wantRest: signsFresh, wantMember: true, wantPeriod: signsCompetition},
		{name: "accepted", html: signs4Dusk, wantRest: signsFresh, wantMember: true, wantPeriod: signsCompetition},
		{name: "castle clan: the cycle over", html: signs33DuskNo, wantRest: signsCycle},
		{name: "no first profession: the cycle over", html: signs33DuskFirstclass, wantRest: signsCycle},
		{name: "validation, Dusk owns Gnosis", html: duskPriest2a, wantRest: signsFresh, wantMember: true, wantPeriod: signsValidation, wantGnosis: true},
		{name: "validation, Dusk without Gnosis", html: duskPriest2c, wantRest: signsFresh, wantMember: true, wantPeriod: signsValidation},
		{name: "recruitment: nothing to do", html: duskPriest6, wantRest: signsFresh, wantPeriod: signsOther},
		{name: "the Gnosis page: sign up", html: signs19GnosisDusk, wantCmd: "SevenSigns 4 1 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSignsBrain(t)
			priestInSight(b)
			s := &b.signs
			if tt.setup != nil {
				tt.setup(s)
			}
			s.v, s.member = npcVisit{npcID: 31085, at: Point{X: 200}, obj: priestObj}, tt.member
			b.handleOne(t, NpcHtmlMessage{ObjectID: priestObj, HTML: tt.html}, t0)
			cmd, rest := b.signsNext(t0)
			if cmd != tt.wantCmd || (cmd == "" && rest != tt.wantRest) {
				t.Fatalf("next = %q rest %v, want %q rest %v", cmd, rest, tt.wantCmd, tt.wantRest)
			}
			if s.member != tt.wantMember || s.period != tt.wantPeriod || s.gnosis != tt.wantGnosis {
				t.Fatalf("member %v period %d gnosis %v, want %v %d %v", s.member, s.period, s.gnosis, tt.wantMember, tt.wantPeriod, tt.wantGnosis)
			}
			for _, never := range []string{"SevenSigns 34", "SevenSigns 5", "SevenSigns 6", "SevenSigns 19 1 3", "SevenSigns 4 1 3"} {
				if strings.HasPrefix(cmd, never) {
					t.Fatalf("next = %q, a link the member never sends", cmd)
				}
			}
		})
	}
}

// TestSevenSignsDue checks when a visit to a priestess starts.
func TestSevenSignsDue(t *testing.T) {
	tests := []struct {
		name  string
		setup func(b *Brain)
		want  bool
	}{
		{name: "a listed member with a first profession", setup: priestInSight, want: true},
		{name: "the priestess's spawn in reach", want: true},
		{name: "the spawn out of reach", setup: func(b *Brain) { b.cfg.Travel.spawns[31085] = []Point{{X: signsReach + 1000}} }},
		{name: "not listed", setup: func(b *Brain) { priestInSight(b); b.persona.SevenSigns.Members = []string{"Grae"} }},
		{name: "name case ignored", setup: func(b *Brain) { priestInSight(b); b.persona.SevenSigns.Members = []string{"alden"} }, want: true},
		{name: "no member list (every preset)", setup: func(b *Brain) { priestInSight(b); b.persona.SevenSigns = SevenSignsPolicy{} }},
		{name: "no first profession", setup: func(b *Brain) { priestInSight(b); b.world.Self.ClassID = 0 }},
		{name: "resting", setup: func(b *Brain) { priestInSight(b); b.signs.restUntil = at(time.Hour) }},
		{name: "the priestess shunned", setup: func(b *Brain) { priestInSight(b); b.signs.shunned.set(31085, at(time.Hour)) }},
		{name: "in a player's party: no walk to the spawn", setup: func(b *Brain) { b.world.Party.LeaderID = 999 }},
		{name: "told to follow: no walk to the spawn", setup: func(b *Brain) { b.orders.follow = 999 }},
		{name: "in a player's party: the priestess in sight", setup: func(b *Brain) { priestInSight(b); b.world.Party.LeaderID = 999 }, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSignsBrain(t)
			if tt.setup != nil {
				tt.setup(b)
			}
			if got := b.startSigns(at(time.Second)); got != tt.want {
				t.Fatalf("startSigns = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSevenSignsDungeons checks which dungeons and dungeon spots open to
// the bot by its membership, the period and the seal's owner (server
// sevensigns.DungeonEntry), and that none opens to a non-member.
func TestSevenSignsDungeons(t *testing.T) {
	now := at(time.Hour)
	tests := []struct {
		name                        string
		setup                       func(s *signsErrand)
		listed                      bool
		avarice, gnosis             bool
		event, seal1Dusk, seal2Dusk bool
	}{
		{name: "a member in the competition", listed: true, setup: competition, avarice: true, gnosis: true, event: true},
		{name: "a non-member", setup: competition},
		{name: "listed, never signed up", listed: true, setup: func(s *signsErrand) { competition(s); s.member = false }},
		{name: "what the priest told is stale", listed: true, setup: func(s *signsErrand) { competition(s); s.readAt = at(-signsFresh) }},
		{name: "the catacombs refused", listed: true, setup: func(s *signsErrand) { competition(s); s.refused.set(SealGnosis, at(2*time.Hour)) }, avarice: true, event: true},
		{
			name: "validation won by Dusk owning Gnosis", listed: true, avarice: true, gnosis: true, seal1Dusk: true, seal2Dusk: true,
			setup: func(s *signsErrand) { s.member, s.period, s.readAt, s.gnosis = true, signsValidation, t0, true },
		},
		{
			name: "validation won by Dusk without Gnosis", listed: true, avarice: true, seal1Dusk: true,
			setup: func(s *signsErrand) { s.member, s.period, s.readAt = true, signsValidation, t0 },
		},
		{name: "another period", listed: true, setup: func(s *signsErrand) { s.member, s.period, s.readAt = true, signsOther, t0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSignsBrain(t)
			if !tt.listed {
				b.persona.SevenSigns.Members = nil
			}
			tt.setup(&b.signs)
			if got := b.signsEnters(SealAvarice, now); got != tt.avarice {
				t.Errorf("enters a necropolis = %v, want %v", got, tt.avarice)
			}
			if got := b.signsEnters(SealGnosis, now); got != tt.gnosis {
				t.Errorf("enters a catacomb = %v, want %v", got, tt.gnosis)
			}
			for ev, want := range map[string]bool{"ssq_event": tt.event, "ssq_seal1_twilight": tt.seal1Dusk, "ssq_seal2_twilight": tt.seal2Dusk, "ssq_seal2_dawn": false, "ssq_seal1_none": false} {
				sp := testSpot("dungeon", 0, 0, 38, 42, 9)
				sp.Event = ev
				if got := b.spotOpen(&sp, now); got != want {
					t.Errorf("farms an %s spot = %v, want %v", ev, got, want)
				}
			}
			if sp := testSpot("open", 0, 0, 38, 42, 9); !b.spotOpen(&sp, now) {
				t.Error("a spot of no event closed")
			}
		})
	}
}

// competition sets s to a member in the competition, read at t0.
func competition(s *signsErrand) {
	s.member, s.period, s.readAt = true, signsCompetition, t0
}

// TestSevenSignsKeepsStones: a member keeps its seal stones (never sold,
// deposited, offered or contributed); a bot of no member mode keeps none.
func TestSevenSignsKeepsStones(t *testing.T) {
	for _, member := range []bool{true, false} {
		b := newSignsBrain(t)
		if !member {
			b.persona.SevenSigns.Members = nil
		}
		for _, id := range []int32{itemBlueSealStone, itemGreenSealStone, itemRedSealStone} {
			if got := b.keepsPvPItem(id) && b.uses(id) && b.keepsItem(Item{TemplateID: id, Count: 5}); got != member {
				t.Errorf("member %v: keeps stone %d = %v, want %v", member, id, got, member)
			}
		}
	}
}

// TestSevenSignsCheck checks the config validation of the member list
// (static cap, D43) and the seal.
func TestSevenSignsCheck(t *testing.T) {
	tests := []struct {
		name    string
		p       SevenSignsPolicy
		wantErr bool
	}{
		{name: "none", p: SevenSignsPolicy{MaxMembers: DefaultSignsMembers}},
		{name: "one at the default cap", p: SevenSignsPolicy{Members: []string{"a"}, MaxMembers: DefaultSignsMembers}},
		{name: "two over the default cap", p: SevenSignsPolicy{Members: []string{"a", "b"}, MaxMembers: DefaultSignsMembers}, wantErr: true},
		{name: "two at a cap of two", p: SevenSignsPolicy{Members: []string{"a", "b"}, MaxMembers: 2}},
		{name: "negative cap", p: SevenSignsPolicy{MaxMembers: -1}, wantErr: true},
		{name: "Avarice", p: SevenSignsPolicy{MaxMembers: 1, Seal: SealAvarice}},
		{name: "Strife", p: SevenSignsPolicy{MaxMembers: 1, Seal: SealStrife}, wantErr: true},
		{name: "unknown seal", p: SevenSignsPolicy{MaxMembers: 1, Seal: 7}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.p.Check(); (err != nil) != tt.wantErr {
				t.Fatalf("Check = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// TestSevenSignsUnansweredLink: a link the priestess leaves unanswered
// ends the visit and shuns her; the bypass cap ends a visit going round.
func TestSevenSignsUnansweredLink(t *testing.T) {
	b := newSignsBrain(t)
	priestInSight(b)
	s := &b.signs
	s.v = npcVisit{npcID: 31085, at: Point{X: 200}, obj: priestObj}
	b.handleOne(t, NpcHtmlMessage{ObjectID: priestObj, HTML: duskPriest1}, t0)
	if rule, a := b.decide(at(time.Second)); rule != "signs" || a.kind != actBypass {
		t.Fatalf("decide = %q %d, want the signs bypass", rule, a.kind)
	} else if _, err := b.send(a, at(time.Second)); err != nil {
		t.Fatal(err)
	}
	if rule, _ := b.decide(at(time.Minute)); rule == "signs" || s.v.active() || !s.shunned.held(31085, at(time.Minute)) {
		t.Fatalf("after no answer: rule %q visit %+v, want the visit over and the priestess shunned", rule, s.v)
	}

	b = newSignsBrain(t)
	priestInSight(b)
	s = &b.signs
	s.v, s.bypasses = npcVisit{npcID: 31085, at: Point{X: 200}, obj: priestObj}, signsBypasses
	b.handleOne(t, NpcHtmlMessage{ObjectID: priestObj, HTML: duskPriest1}, t0)
	if _, ok := b.ruleSigns(at(time.Second)); ok || s.v.active() || !s.shunned.held(31085, at(time.Minute)) {
		t.Fatalf("over the cap: visit %+v, want it over and the priestess shunned", s.v)
	}
	if slices.Contains(duskPriestesses, 31078) {
		t.Fatal("a Priest of Dawn among the Dusk Priestesses")
	}
}
