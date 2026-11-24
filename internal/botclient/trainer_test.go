package botclient

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// trainerObj is a human mystic trainer (Zigaunt, NPC 30022) in sight.
const trainerObj = int32(0x20000010)

// trainerPage is the trainer's first page as the server sends it, with
// %objectId% filled in (data/html/trainer/30022.htm).
func trainerPage(id int32) string {
	return `<html><body>Trainer:<br><a action="bypass -h npc_` + itoa(id) + `_SkillList">Learn skills</a><br>` +
		`<a action="bypass -h npc_` + itoa(id) + `_Quest">Quest</a></body></html>`
}

func itoa(v int32) string { return strconv.Itoa(int(v)) }

// skillList is an AcquireSkillList frame in the server's layout: the
// level written twice, a zero after the cost.
func skillList(skills ...AcquirableSkill) []byte {
	fields := []any{acquireSkillUsual, int32(len(skills))}
	for _, s := range skills {
		fields = append(fields, s.ID, s.Level, s.Level, s.Cost, 0)
	}
	return packet(opGameAcquireSkillList, fields...)
}

func TestDecodeTrainerEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Object 7, page "ab" in UTF-16LE with its terminator, item 0.
		{"NpcHtmlMessage bytes", []byte{0x0f, 7, 0, 0, 0, 'a', 0, 'b', 0, 0, 0, 0, 0, 0, 0}, NpcHtmlMessage{ObjectID: 7, HTML: "ab"}},
		{"NpcHtmlMessage", packet(opGameNpcHtmlMessage, trainerObj, trainerPage(trainerObj), 0), NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}},
		// Type 0, one entry: Heal 1 (level twice), 160 SP, a zero.
		{
			"AcquireSkillList bytes",
			[]byte{0x8a, 0, 0, 0, 0, 1, 0, 0, 0, 0xf3, 3, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0xa0, 0, 0, 0, 0, 0, 0, 0},
			AcquireSkillList{Skills: []AcquirableSkill{{ID: 1011, Level: 1, Cost: 160}}},
		},
		{
			"AcquireSkillList", skillList(AcquirableSkill{1011, 2, 160}, AcquirableSkill{1068, 1, 470}),
			AcquireSkillList{Skills: []AcquirableSkill{{1011, 2, 160}, {1068, 1, 470}}},
		},
		{"AcquireSkillList empty", skillList(), AcquireSkillList{Skills: []AcquirableSkill{}}},
		{"AcquireSkillInfo", packet(opGameAcquireSkillInfo, 1011, 1, 160, 0, 0), AcquireSkillInfo{ID: 1011, Level: 1, Cost: 160}},
		{
			"AcquireSkillInfo spellbook", packet(opGameAcquireSkillInfo, 1011, 1, 160, 0, 1, 99, 1152, 1, 50),
			AcquireSkillInfo{ID: 1011, Level: 1, Cost: 160, Requirements: []SkillRequirement{{Type: 99, ItemID: 1152, Count: 1}}},
		},
		{"AcquireSkillDone", packet(opGameAcquireSkillDone), AcquireSkillDone{}},
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

func TestDecodeTrainerEventsRejects(t *testing.T) {
	list := skillList(AcquirableSkill{1011, 1, 160})
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"NpcHtmlMessage without item id", packet(opGameNpcHtmlMessage, 7, "page"), wire.ErrShortPacket},
		{"NpcHtmlMessage unterminated page", []byte{0x0f, 7, 0, 0, 0, 'a', 0}, wire.ErrShortPacket},
		{"NpcHtmlMessage trailing", packet(opGameNpcHtmlMessage, 7, "page", 0, 0), errTrailingBytes},
		{"AcquireSkillList short entry", list[:len(list)-4], wire.ErrShortPacket},
		{"AcquireSkillList count past the frame", packet(opGameAcquireSkillList, 0, 0x7fffffff), wire.ErrShortPacket},
		{"AcquireSkillList trailing", append(list, 0), errTrailingBytes},
		{"AcquireSkillInfo short requirement", packet(opGameAcquireSkillInfo, 1011, 1, 160, 0, 1, 99, 1152, 1), wire.ErrShortPacket},
		{"AcquireSkillInfo trailing", packet(opGameAcquireSkillInfo, 1011, 1, 160, 0, 0, 0), errTrailingBytes},
		{"AcquireSkillDone trailing", packet(opGameAcquireSkillDone, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTrainerEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestBypassToServer", encodeRequestBypassToServer("npc_7"), []byte{0x21, 'n', 0, 'p', 0, 'c', 0, '_', 0, '7', 0, 0, 0}},
		{
			"RequestAcquireSkillInfo", encodeSkillRequest(opGameRequestAcquireSkillInfo, 1011, 2, acquireSkillUsual),
			[]byte{0x6b, 0xf3, 3, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			"RequestAcquireSkill", encodeSkillRequest(opGameRequestAcquireSkill, 1068, 1, acquireSkillUsual),
			[]byte{0x6c, 0x2c, 4, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
	if got := skillListBypass(268435473); got != "npc_268435473_SkillList" {
		t.Errorf("skillListBypass = %q", got)
	}
}

// TestDialogLinks checks the links read off a page against the server's
// whitelist rules (bypass.go bypassWhitelist.record).
func TestDialogLinks(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{"trainer page", trainerPage(5), []string{"npc_5_SkillList", "npc_5_Quest"}},
		{"without -h", `<a action="bypass npc_5_Chat 1">x</a>`, []string{"npc_5_Chat 1"}},
		{"trimmed", `<a action="bypass -h  npc_5_Chat 1 ">x</a>`, []string{"npc_5_Chat 1"}},
		{"edit box link left out", `<a action="bypass -h npc_5_Link $value">x</a><a action="bypass -h npc_5_Chat 0">y</a>`, []string{"npc_5_Chat 0"}},
		{"no links", `<html><body>I cannot teach you.</body></html>`, nil},
		{"unterminated", `<a action="bypass -h npc_5_SkillList`, nil},
		{"empty", `<a action="bypass ">x</a>`, nil},
	}
	for _, tt := range tests {
		if got := dialogLinks(tt.html); !slices.Equal(got, tt.want) {
			t.Errorf("%s: dialogLinks = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestPacerBypassGate checks that bypasses are spaced by bypassGap, well
// past the server's 100 ms ServerBypassTime, and that only the bypass
// action passes the bypass gate.
func TestPacerBypassGate(t *testing.T) {
	const serverBypassTime = 100 * time.Millisecond
	if bypassGap < serverBypassTime {
		t.Fatalf("bypassGap = %v, under the server's %v", bypassGap, serverBypassTime)
	}
	p := newPacer(t0)
	for _, try := range []struct {
		at   time.Duration
		want bool
	}{{0, true}, {bypassGap - time.Millisecond, false}, {bypassGap, true}} {
		now := at(try.at)
		if got := p.allow(gateBypass, now); got != try.want {
			t.Fatalf("allow(bypass) at %v = %v, want %v", try.at, got, try.want)
		}
		if try.want {
			p.sent(gateBypass, now)
		}
	}
	for _, a := range []Action{bypassAction("x"), talkAction(1, Point{}), {kind: actSkillInfo}, {kind: actLearnSkill}} {
		want := gateNone
		if a.kind == actBypass {
			want = gateBypass
		}
		if g := a.gate(); g != want {
			t.Errorf("kind %d gate = %d, want %d", a.kind, g, want)
		}
	}
}

// newTrainerBrain is a level 15 human mystic with sp SP, trainer
// trainerObj in sight 300 units away.
func newTrainerBrain(t *testing.T, sp int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance = 0
	p.Buffs = []Buff{} // a skill learned would be cast at once (rule 7)
	b, _ := newTestBrain(t, p)
	s := &b.world.Self
	s.ClassID, s.Level, s.SP = classHumanMystic, 15, sp
	b.world.Apply(NPCInfo{ObjectID: trainerObj, NPCID: 30022, Pos: Point{X: 300}}, t0)
	b.cfg.Travel = &TravelMap{trainers: testTrainers}
	return b
}

// testTrainers are the trainers of the tests' mystics (indexTrainers).
var testTrainers = map[int32][]int32{classHumanMystic: {30022, 30030}, classCleric: {30022, 30030}}

// trainerStep is one step of a visit: events in, then a decision.
type trainerStep struct {
	name     string
	at       time.Duration
	setup    func(b *Brain)
	events   []Event
	wantRule string
	wantKind actionKind
	// wantPayload, when set, is the exact packet.
	wantPayload []byte
	// wantWithin, when set, is how far (horizontally) from wantTo the
	// destination of the walk the move belongs to may lie: a walk to a
	// stand point near an NPC.
	wantTo     Point
	wantWithin int32
}

// runTrainerSteps feeds each step's events, decides and sends the action.
func runTrainerSteps(t *testing.T, b *Brain, steps []trainerStep) {
	t.Helper()
	for _, st := range steps {
		now := at(st.at)
		if st.setup != nil {
			st.setup(b)
		}
		if err := b.handle(st.events, now); err != nil {
			t.Fatalf("%s: handle: %v", st.name, err)
		}
		rule, a := b.decide(now)
		if rule != st.wantRule || a.kind != st.wantKind {
			t.Fatalf("%s: decide = rule %q action %d, want rule %q action %d", st.name, rule, a.kind, st.wantRule, st.wantKind)
		}
		if st.wantPayload != nil && !bytes.Equal(a.payload, st.wantPayload) {
			t.Fatalf("%s: payload % x, want % x", st.name, a.payload, st.wantPayload)
		}
		if st.wantWithin > 0 {
			if to := b.walk.dest; !a.walk || horizontal2(to, st.wantTo) > int64(st.wantWithin)*int64(st.wantWithin) {
				t.Fatalf("%s: walk %v to %+v, want a walk within %d of %+v", st.name, a.walk, to, st.wantWithin, st.wantTo)
			}
		}
		if a.kind != actHold {
			if sent, err := b.send(a, now); err != nil || !sent {
				t.Fatalf("%s: send = %v, %v", st.name, sent, err)
			}
		}
	}
}

func infoReq(id, level int32) []byte {
	return encodeSkillRequest(opGameRequestAcquireSkillInfo, id, level, acquireSkillUsual)
}

func learnReq(id, level int32) []byte {
	return encodeSkillRequest(opGameRequestAcquireSkill, id, level, acquireSkillUsual)
}

// TestBrainTrainerVisit walks a level 15 human mystic through a visit:
// select, talk, open the skill list, learn Heal one level at a time while
// the SP lasts, then come back once the SP reaches the next level's cost
// and again after a level-up.
func TestBrainTrainerVisit(t *testing.T) {
	b := newTrainerBrain(t, 200)
	page := NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}
	heal1, heal2, might := AcquirableSkill{skillHeal, 1, 160}, AcquirableSkill{skillHeal, 2, 160}, AcquirableSkill{skillMight, 1, 470}
	anti := AcquirableSkill{146, 1, 240} // offered, not wanted
	runTrainerSteps(t, b, []trainerStep{
		{name: "select", wantRule: "train", wantKind: actSelect, wantPayload: encodeAction(trainerObj, Point{})},
		{name: "selection not confirmed yet clicks again", at: time.Second, wantRule: "train", wantKind: actSelect},
		{name: "talk", at: 2 * time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk, wantPayload: encodeAction(trainerObj, Point{})},
		{name: "wait for the page", at: 10 * time.Second, wantRule: "train"},
		{name: "a page of another NPC changes nothing", at: 11 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: npcID, HTML: trainerPage(npcID)}}, wantRule: "train"},
		{name: "skill list link", at: 12 * time.Second, events: []Event{page}, wantRule: "train", wantKind: actBypass, wantPayload: encodeRequestBypassToServer(skillListBypass(trainerObj))},
		{name: "wait for the list", at: 13 * time.Second, wantRule: "train"},
		{name: "ask the cost of the first wanted skill", at: 14 * time.Second, events: []Event{AcquireSkillList{Skills: []AcquirableSkill{anti, might, heal1}}}, wantRule: "train", wantKind: actSkillInfo, wantPayload: infoReq(skillHeal, 1)},
		{name: "wait for the cost", at: 15 * time.Second, wantRule: "train"},
		{name: "an answer for another skill changes nothing", at: 15 * time.Second, events: []Event{AcquireSkillInfo{ID: skillMight, Level: 1, Cost: 470}}, wantRule: "train"},
		{name: "learn", at: 16 * time.Second, events: []Event{AcquireSkillInfo{ID: skillHeal, Level: 1, Cost: 160}}, wantRule: "train", wantKind: actLearnSkill, wantPayload: learnReq(skillHeal, 1)},
		{name: "wait for the learn", at: 17 * time.Second, wantRule: "train"},
		{
			name: "the next level costs more than the SP left", at: 18 * time.Second,
			setup:  func(b *Brain) { b.world.Self.SP = 40 },
			events: []Event{SkillList{Skills: []KnownSkill{{ID: skillHeal, Level: 1}}}, AcquireSkillList{Skills: []AcquirableSkill{anti, might, heal2}}},
		},
		{name: "not due below the cheapest cost", at: 30 * time.Second, setup: func(b *Brain) { b.world.Self.SP = 159 }},
		{name: "due again at the SP of the cheapest cost", at: 31 * time.Second, setup: func(b *Brain) { b.world.Self.SP = 160 }, wantRule: "train", wantKind: actTalk},
		{name: "page", at: 32 * time.Second, events: []Event{page}, wantRule: "train", wantKind: actBypass},
		{name: "learn window closes", at: 33 * time.Second, events: []Event{AcquireSkillDone{}}},
		{name: "not due at the same level", at: 40 * time.Second},
		{name: "due after a level-up", at: 50 * time.Second, setup: func(b *Brain) { b.world.Self.Level = 16 }, wantRule: "train", wantKind: actTalk},
	})
	if b.train.npc != trainerObj || b.train.doneLevel != 15 {
		t.Errorf("visit = %+v, want one running after level 15's", b.train)
	}
}

// TestBrainTrainerSkips checks the skills a visit leaves out: a cost the
// server corrects past the SP and a learn the server refuses.
func TestBrainTrainerSkips(t *testing.T) {
	b := newTrainerBrain(t, 500)
	b.world.Target = trainerObj
	heal, might, shield := AcquirableSkill{skillHeal, 1, 160}, AcquirableSkill{skillMight, 1, 470}, AcquirableSkill{skillShield, 1, 470}
	runTrainerSteps(t, b, []trainerStep{
		{name: "talk", wantRule: "train", wantKind: actTalk},
		{name: "page", at: time.Second, events: []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}}, wantRule: "train", wantKind: actBypass},
		{name: "Heal first", at: 2 * time.Second, events: []Event{AcquireSkillList{Skills: []AcquirableSkill{shield, might, heal}}}, wantRule: "train", wantKind: actSkillInfo, wantPayload: infoReq(skillHeal, 1)},
		{name: "corrected cost past the SP skips Heal", at: 3 * time.Second, events: []Event{AcquireSkillInfo{ID: skillHeal, Level: 1, Cost: 600}}, wantRule: "train", wantKind: actSkillInfo, wantPayload: infoReq(skillMight, 1)},
		{name: "Might", at: 4 * time.Second, events: []Event{AcquireSkillInfo{ID: skillMight, Level: 1, Cost: 470}}, wantRule: "train", wantKind: actLearnSkill, wantPayload: learnReq(skillMight, 1)},
		{
			name: "a refused learn skips Might", at: 5 * time.Second,
			events:   []Event{AcquireSkillList{Skills: []AcquirableSkill{shield, might, heal}}},
			wantRule: "train", wantKind: actSkillInfo, wantPayload: infoReq(skillShield, 1),
		},
		{name: "Shield", at: 6 * time.Second, events: []Event{AcquireSkillInfo{ID: skillShield, Level: 1, Cost: 470}}, wantRule: "train", wantKind: actLearnSkill},
		{
			name: "learned; nothing else affordable ends the visit", at: 7 * time.Second,
			setup:  func(b *Brain) { b.world.Self.SP = 30 },
			events: []Event{SkillList{Skills: []KnownSkill{{ID: skillShield, Level: 1}}}, AcquireSkillList{Skills: []AcquirableSkill{might, heal}}},
		},
	})
	if b.train.npc != 0 || b.train.doneLevel != 15 || b.train.needSP != 600 {
		t.Errorf("visit = %+v, want ended at level 15, due again at 600 SP", b.train)
	}
}

// TestBrainTrainerRetries checks unanswered requests and trainers that
// teach the bot nothing.
func TestBrainTrainerRetries(t *testing.T) {
	t.Run("unanswered bypass talks again", func(t *testing.T) {
		b := newTrainerBrain(t, 500)
		b.world.Target = trainerObj
		runTrainerSteps(t, b, []trainerStep{
			{name: "talk", wantRule: "train", wantKind: actTalk},
			{name: "page", at: time.Second, events: []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}}, wantRule: "train", wantKind: actBypass},
			{name: "still waiting", at: time.Second + trainerReplyTimeout - time.Millisecond, wantRule: "train"},
			{name: "talk again", at: time.Second + trainerReplyTimeout, wantRule: "train", wantKind: actTalk},
		})
	})
	t.Run("a page replaced before the link talks again", func(t *testing.T) {
		b := newTrainerBrain(t, 500)
		b.world.Target = trainerObj
		runTrainerSteps(t, b, []trainerStep{
			{name: "talk", wantRule: "train", wantKind: actTalk},
			{
				name: "another page replaces the trainer's", at: time.Second,
				events:   []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}, NpcHtmlMessage{ObjectID: npcID, HTML: trainerPage(npcID)}},
				wantRule: "train", wantKind: actTalk,
			},
		})
	})
	t.Run("no-skills page shuns the trainer", func(t *testing.T) {
		b := newTrainerBrain(t, 500)
		b.world.Target = trainerObj
		runTrainerSteps(t, b, []trainerStep{
			{name: "talk", wantRule: "train", wantKind: actTalk},
			{name: "no link", at: time.Second, events: []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: "<html><body>I cannot teach you.</body></html>"}}},
			{name: "still shunned", at: time.Second + trainerShunFor - time.Millisecond},
			{name: "visited again later", at: time.Second + trainerShunFor, wantRule: "train", wantKind: actTalk},
		})
	})
	t.Run("a trainer that never answers is shunned", func(t *testing.T) {
		b := newTrainerBrain(t, 500)
		b.world.Target = trainerObj
		var steps []trainerStep
		for i := range trainerClicks {
			steps = append(steps, trainerStep{name: "talk", at: time.Duration(i) * trainerTalkTimeout, wantRule: "train", wantKind: actTalk})
		}
		steps = append(steps, trainerStep{name: "give up", at: trainerClicks * trainerTalkTimeout})
		runTrainerSteps(t, b, steps)
		if !b.train.shunned[trainerObj].Equal(at(trainerClicks*trainerTalkTimeout + trainerShunFor)) {
			t.Errorf("shunned = %v", b.train.shunned)
		}
	})
	t.Run("trainer out of sight ends the visit", func(t *testing.T) {
		b := newTrainerBrain(t, 500)
		runTrainerSteps(t, b, []trainerStep{
			{name: "select", wantRule: "train", wantKind: actSelect},
			{name: "gone", at: time.Second, events: []Event{DeleteObject{ObjectID: trainerObj}}},
		})
		if b.train.npc != 0 || b.train.doneLevel != 0 {
			t.Errorf("visit = %+v, want none and still due", b.train)
		}
	})
}

// TestRuleTrainStarts checks when a visit starts.
func TestRuleTrainStarts(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(b *Brain)
		wantRule string
		wantKind actionKind
	}{
		{"trainer of the class in sight", nil, "train", actSelect},
		{"a fighter wants no skill", func(b *Brain) { b.world.Self.ClassID = 0 }, "", actHold},
		{"no trainer of the class in sight", func(b *Brain) { b.world.Self.ClassID = classDarkMystic }, "", actHold},
		{"persona with no skills for the class", func(b *Brain) { b.persona.LearnSkills = map[int32][]int32{} }, "", actHold},
		{"dead trainer", func(b *Brain) { b.world.Objects[trainerObj].Dead = true }, "", actHold},
		{"sitting stands up first", func(b *Brain) { b.world.Self.Sitting, b.world.Target = true, trainerObj }, "train", actSitStand},
		{"combat goes first", func(b *Brain) { b.world.Self.InCombat = true }, "attacked", actHold},
		{"resting goes first", func(b *Brain) { b.world.Self.HP = 10 }, "rest", actSitStand},
		{"done at this level", func(b *Brain) { b.train.doneLevel = 15 }, "", actHold},
		{"done at this level, SP short of the cheapest skill", func(b *Brain) { b.train.doneLevel, b.train.needSP = 15, 501 }, "", actHold},
		{"done at a lower level", func(b *Brain) { b.train.doneLevel = 14 }, "train", actSelect},
		{
			"nearest trainer first",
			func(b *Brain) {
				b.world.Apply(NPCInfo{ObjectID: trainerObj + 1, NPCID: 30030, Pos: Point{X: 100}}, t0)
				b.world.Target = trainerObj + 1
			},
			"train", actTalk,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTrainerBrain(t, 500)
			if tt.setup != nil {
				tt.setup(b)
			}
			rule, a := b.decide(at(time.Second))
			if rule != tt.wantRule || a.kind != tt.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tt.wantRule, tt.wantKind)
			}
		})
	}
}

// TestDefaultPersonaSkills checks the support skills of the default
// persona's builds against the classes the plan names: healers learn Heal,
// Battle Heal, Might and Shield, then Wind Strike; orc mystics their buffs,
// then Life Drain and Chill Flame, and no heal. The persona overrides none
// of them; the trainers come from the datapack (TestTrainersShipped).
func TestDefaultPersonaSkills(t *testing.T) {
	p := DefaultPersona()
	if p.LearnSkills != nil || p.Trainers != nil {
		t.Fatalf("DefaultPersona overrides the build: learn %v trainers %v", p.LearnSkills, p.Trainers)
	}
	kit := func(c int32) ClassKit { return compileFor(p, c, 1).Kit }
	for _, c := range []int32{classHumanMystic, classElvenMystic, classDarkMystic} {
		if got := kit(c).Skills; !slices.Equal(got, []int32{skillHeal, skillBattleHeal, skillMight, skillShield, skillWindStrike}) {
			t.Errorf("class %d skills = %v", c, got)
		}
	}
	if got := kit(classOrcMystic).Skills; !slices.Equal(got, []int32{skillSoulShield, skillChantBattle, skillLifeDrain, skillChillFlame}) {
		t.Errorf("orc mystic skills = %v, want buffs then attack spells", got)
	}
}
