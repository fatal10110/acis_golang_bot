package botclient

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

// TestDecodeEffects pins the effect icon decoders to the server writers
// (serverpackets/status_effects.go): each decodes its whole frame, and a
// frame one byte short or long is refused.
func TestDecodeEffects(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{
			"AbnormalStatusUpdate timed and toggle",
			packet(opGameAbnormalStatusUpdate, uint16(2), int32(skillMight), uint16(2), int32(1150), int32(312), uint16(1), int32(-1)),
			AbnormalStatusUpdate{Effects: []Effect{
				{SkillID: skillMight, Level: 2, Left: 1150 * time.Second},
				{SkillID: 312, Level: 1, Permanent: true},
			}},
		},
		{
			"AbnormalStatusUpdate none",
			packet(opGameAbnormalStatusUpdate, uint16(0)),
			AbnormalStatusUpdate{Effects: []Effect{}},
		},
		{
			"PartySpelled player",
			packet(opGamePartySpelled, 0, pcID, 2, int32(skillShield), uint16(3), int32(45), int32(312), uint16(1), int32(0)),
			PartySpelled{ObjectID: pcID, Effects: []Effect{
				{SkillID: skillShield, Level: 3, Left: 45 * time.Second},
				{SkillID: 312, Level: 1, Permanent: true},
			}},
		},
		{
			"PartySpelled servitor none",
			packet(opGamePartySpelled, 2, npcID, 0),
			PartySpelled{Kind: 2, ObjectID: npcID, Effects: []Effect{}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok, err := decodeEvent(tc.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(ev, tc.want) {
				t.Fatalf("decoded\n got %+v\nwant %+v", ev, tc.want)
			}
			if _, _, err := decodeEvent(tc.raw[:len(tc.raw)-1]); err == nil {
				t.Error("a frame one byte short decoded")
			}
			if _, _, err := decodeEvent(append(bytes.Clone(tc.raw), 0)); err == nil {
				t.Error("a frame one byte long decoded")
			}
		})
	}
	for _, raw := range [][]byte{
		packet(opGameAbnormalStatusUpdate, uint16(0xffff), 1),
		packet(opGamePartySpelled, 0, pcID, 0x7fffffff, 1),
	} {
		if _, _, err := decodeEvent(raw); err == nil {
			t.Errorf("% x with a huge count decoded", raw[:2])
		}
	}
}

// TestWorldEffects checks the buff model: the bot's own list from
// AbnormalStatusUpdate, a member's from PartySpelled, each replaced whole
// and kept across a leader change's new window.
func TestWorldEffects(t *testing.T) {
	const pc2 = int32(0x20000003)
	w := newTestWorld(t)
	w.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID}, {ObjectID: pc2}}}, t0)
	might := []Effect{{SkillID: skillMight, Left: 90 * time.Second}, {SkillID: 312, Permanent: true}}
	steps := []Event{
		AbnormalStatusUpdate{Effects: might},
		PartySpelled{ObjectID: pcID, Effects: might},
		PartySpelled{ObjectID: pc2, Effects: might},
		PartySpelled{ObjectID: pc2, Effects: nil},
		PartySpelled{Kind: 1, ObjectID: pcID, Effects: nil}, // its pet
		PartySpelled{ObjectID: selfID, Effects: nil},        // the bot's own: AbnormalStatusUpdate has it
		PartySpelled{ObjectID: 0x20000009, Effects: might},  // no member
		PartySmallWindowAll{LeaderID: pc2, Members: []PartyMember{{ObjectID: pcID}, {ObjectID: pc2}}},
	}
	for _, ev := range steps {
		w.Apply(ev, at(time.Second))
	}
	want := Buffs{skillMight: at(91 * time.Second), 312: {}}
	if !reflect.DeepEqual(w.Buffs, want) {
		t.Errorf("own buffs %v, want %v", w.Buffs, want)
	}
	if got := w.Party.Members[pcID].Buffs; !reflect.DeepEqual(got, want) {
		t.Errorf("leader buffs %v, want %v", got, want)
	}
	if got := w.Party.Members[pc2].Buffs; got == nil || len(got) != 0 {
		t.Errorf("member buffs %v, want an empty list", got)
	}

	due := []struct {
		name  string
		skill int32
		now   time.Duration
		want  bool
	}{
		{"missing", skillShield, time.Second, true},
		{"90 s left", skillMight, time.Second, false},
		{"61 s left", skillMight, 30 * time.Second, false},
		{"59 s left", skillMight, 32 * time.Second, true},
		{"permanent", 312, time.Hour, false},
	}
	for _, tc := range due {
		if got := w.Buffs.Due(tc.skill, time.Minute, at(tc.now)); got != tc.want {
			t.Errorf("%s: Due = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !Buffs(nil).Due(skillMight, time.Minute, t0) {
		t.Error("a member with no list is not due")
	}
}

// TestSupportRules checks heals (rule 3), rebuffs (rule 7) and the assist
// (rule 9) of the default persona with an injected clock: the leader
// first, then the bot, then the others; only skills the bot knows.
func TestSupportRules(t *testing.T) {
	now := at(time.Second)
	const pc2 = int32(0x20000003)
	know := func(b *Brain, ids ...int32) {
		var s []KnownSkill
		for _, id := range ids {
			s = append(s, KnownSkill{ID: id, Level: 1})
		}
		b.world.Apply(SkillList{Skills: s}, t0)
	}
	// party puts the bot in pcID's party with pc2, at the HP given out of
	// 100, every buff they may get lasting an hour.
	party := func(b *Brain, leaderHP, otherHP int32) {
		b.world.Apply(CharInfo{ObjectID: pc2, Name: "third", Pos: Point{X: -300}}, t0)
		b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{
			{ObjectID: pcID, HP: leaderHP, MaxHP: 100}, {ObjectID: pc2, HP: otherHP, MaxHP: 100},
		}}, t0)
		hour := []Effect{{SkillID: skillMight, Left: time.Hour}, {SkillID: skillShield, Left: time.Hour}, {SkillID: skillChantBattle, Left: time.Hour}, {SkillID: skillSoulShield, Left: time.Hour}}
		b.world.Apply(AbnormalStatusUpdate{Effects: hour}, t0)
		b.world.Apply(PartySpelled{ObjectID: pcID, Effects: hour}, t0)
		b.world.Apply(PartySpelled{ObjectID: pc2, Effects: hour}, t0)
	}
	healer := func(b *Brain, leaderHP, otherHP int32) {
		know(b, skillHeal, skillBattleHeal, skillMight, skillShield)
		party(b, leaderHP, otherHP)
	}
	leaderHits := func(b *Brain, d time.Duration) {
		b.handleOne(t, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: npcID, Damage: 30}}}, at(d))
	}
	tests := []struct {
		name       string
		setup      func(b *Brain)
		wantRule   string
		wantKind   actionKind
		wantObject int32
		wantSkill  int32
	}{
		{
			name:     "leader under 60 % is selected",
			setup:    func(b *Brain) { healer(b, 59, 100) },
			wantRule: "heal", wantKind: actSelect, wantObject: pcID,
		},
		{
			name:     "selected leader under 60 % gets Heal",
			setup:    func(b *Brain) { healer(b, 59, 100); b.world.Target = pcID },
			wantRule: "heal", wantKind: actCast, wantObject: pcID, wantSkill: skillHeal,
		},
		{
			name:     "selected leader under 35 % gets Battle Heal",
			setup:    func(b *Brain) { healer(b, 34, 100); b.world.Target = pcID },
			wantRule: "heal", wantKind: actCast, wantObject: pcID, wantSkill: skillBattleHeal,
		},
		{
			name: "Battle Heal on reuse falls back to Heal",
			setup: func(b *Brain) {
				healer(b, 34, 100)
				b.world.Target = pcID
				b.world.holdSkill(skillBattleHeal, at(time.Minute))
			},
			wantRule: "heal", wantKind: actCast, wantObject: pcID, wantSkill: skillHeal,
		},
		{
			name:     "leader at 60 % is not healed",
			setup:    func(b *Brain) { healer(b, 60, 100) },
			wantRule: "",
		},
		{
			name:     "leader before self",
			setup:    func(b *Brain) { healer(b, 50, 100); b.world.Self.HP = 10 },
			wantRule: "heal", wantKind: actSelect, wantObject: pcID,
		},
		{
			name:     "self before others",
			setup:    func(b *Brain) { healer(b, 100, 10); b.world.Self.HP = 50 },
			wantRule: "heal", wantKind: actSelect, wantObject: selfID,
		},
		{
			name:     "another member last",
			setup:    func(b *Brain) { healer(b, 100, 50); b.world.Target = pc2 },
			wantRule: "heal", wantKind: actCast, wantObject: pc2, wantSkill: skillHeal,
		},
		{
			name: "a flagged member is not healed",
			setup: func(b *Brain) {
				healer(b, 100, 50)
				b.world.Apply(RelationChanged{ObjectID: pc2, PvPFlag: 1}, t0)
			},
			wantRule: "",
		},
		{
			name: "a dead leader is not healed",
			setup: func(b *Brain) {
				healer(b, 0, 100)
			},
			wantRule: "",
		},
		{
			name:     "a fighter knows no heal",
			setup:    func(b *Brain) { party(b, 10, 10) },
			wantRule: "",
		},
		{
			name: "a cast in flight is waited out",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Target = pcID
				b.castAt = t0.Add(time.Second / 2)
			},
			wantRule: "heal", wantKind: actHold,
		},
		{
			name: "a sitting bot stands up to heal",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Self.Sitting = true
			},
			wantRule: "heal", wantKind: actSitStand,
		},
		{
			name: "a bot resting its MP stays seated",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Self.Sitting, b.world.Self.MP = true, 20
				b.resting = true
			},
			wantRule: "rest",
		},
		{
			name: "a standing bot short of MP rests instead of healing",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Self.MP = 20
			},
			wantRule: "rest", wantKind: actSitStand,
		},
		{
			name: "in combat a bot short of MP still heals",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Self.InCombat, b.world.Self.MP = true, 20
			},
			wantRule: "heal", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "heals before fighting back",
			setup: func(b *Brain) {
				healer(b, 50, 100)
				b.world.Self.InCombat = true
				b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, t0)
			},
			wantRule: "heal", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "critical heals itself before fleeing",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Self.InCombat, b.world.Self.HP, b.world.Target = true, 10, selfID
			},
			wantRule: "critical", wantKind: actCast, wantObject: selfID, wantSkill: skillBattleHeal,
		},
		{
			name: "critical with no heal flees",
			setup: func(b *Brain) {
				party(b, 100, 100)
				b.world.Self.InCombat, b.world.Self.HP = true, 10
			},
			wantRule: "critical", wantKind: actMove,
		},
		{
			name: "own buff missing is renewed on the bot",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Target = selfID
				b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillShield, Left: time.Hour}}}, t0)
			},
			wantRule: "rebuff", wantKind: actCast, wantObject: selfID, wantSkill: skillMight,
		},
		{
			name: "leader's buff under 60 s is renewed first",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Apply(PartySpelled{ObjectID: pcID, Effects: []Effect{{SkillID: skillMight, Left: time.Hour}, {SkillID: skillShield, Left: 30 * time.Second}}}, t0)
				b.world.Apply(AbnormalStatusUpdate{}, t0)
			},
			wantRule: "rebuff", wantKind: actSelect, wantObject: pcID,
		},
		{
			name: "a member with no list gets every buff",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Party.Members[pc2].Buffs = nil
				b.world.Target = pc2
			},
			wantRule: "rebuff", wantKind: actCast, wantObject: pc2, wantSkill: skillMight,
		},
		{
			name: "buff cast lately on a member is not cast again",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Party.Members[pc2].Buffs = Buffs{skillShield: {}}
				b.buffedAt[buffKey{pc2, skillMight}] = now.Add(-buffRetry + time.Second)
			},
			wantRule: "",
		},
		{
			name: "no rebuff in combat",
			setup: func(b *Brain) {
				healer(b, 100, 100)
				b.world.Apply(AbnormalStatusUpdate{}, t0)
				b.world.Self.InCombat = true
			},
			wantRule: "attacked", wantKind: actHold,
		},
		{
			name: "an orc mystic casts its party chant on itself",
			setup: func(b *Brain) {
				know(b, skillSoulShield, skillChantBattle)
				party(b, 100, 100)
				b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillSoulShield, Left: time.Hour}}}, t0)
				b.world.Target = npcID
			},
			wantRule: "rebuff", wantKind: actCast, wantObject: selfID, wantSkill: skillChantBattle,
		},
		{
			name: "the leader's hit is assisted after the delay",
			setup: func(b *Brain) {
				party(b, 100, 100)
				leaderHits(b, 0)
			},
			wantRule: "assist", wantKind: actSelect, wantObject: npcID,
		},
		{
			name: "no assist before the delay",
			setup: func(b *Brain) {
				party(b, 100, 100)
				leaderHits(b, time.Second-assistMinTest+time.Millisecond)
			},
			wantRule: "",
		},
		{
			name: "no assist on the leader's bare selection or buff",
			setup: func(b *Brain) {
				party(b, 100, 100)
				b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: pc2, SkillID: skillMight}, t0)
			},
			wantRule: "",
		},
		{
			name: "no assist of another member",
			setup: func(b *Brain) {
				party(b, 100, 100)
				b.handleOne(t, Attack{AttackerID: pc2, Hits: []AttackHit{{TargetID: npcID}}}, t0)
			},
			wantRule: "",
		},
		{
			name: "the leader's hostile cast is assisted",
			setup: func(b *Brain) {
				party(b, 100, 100)
				b.handleOne(t, MagicSkillUse{CasterID: pcID, TargetID: npcID, SkillID: 1177}, t0)
				b.world.Target = npcID
			},
			wantRule: "assist", wantKind: actAttack, wantObject: npcID,
		},
		{
			name: "an assist the leader stopped hitting expires",
			setup: func(b *Brain) {
				party(b, 100, 100)
				leaderHits(b, -attackerWindow)
			},
			wantRule: "",
		},
		{
			name: "in combat with no one to fight it assists",
			setup: func(b *Brain) {
				party(b, 100, 100)
				b.world.Self.InCombat = true
				leaderHits(b, 0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: npcID,
		},
		{
			name: "under the MP reserve it hits instead of casting",
			setup: func(b *Brain) {
				know(b, 1177)
				party(b, 100, 100)
				b.world.Self.InCombat, b.world.Self.MP = true, 49
				b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, t0)
				b.world.Target = npcID
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: npcID,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPersona()
			p.IdleChance, p.AttackSkills = 0, []int32{1177}
			p.AssistMin, p.AssistMax = assistMinTest, assistMinTest
			p.Follow = FollowPolicy{} // following the leader is #18's, not checked here
			b, _ := newTestBrain(t, p)
			tc.setup(b)
			rule, a := b.decide(now)
			if rule != tc.wantRule || a.kind != tc.wantKind {
				t.Fatalf("decide = rule %q action %d, want rule %q action %d", rule, a.kind, tc.wantRule, tc.wantKind)
			}
			if a.object != tc.wantObject || a.skill != tc.wantSkill {
				t.Errorf("action on %#x skill %d, want %#x skill %d", a.object, a.skill, tc.wantObject, tc.wantSkill)
			}
			if a.kind == actCast && !bytes.Equal(a.payload, encodeRequestMagicSkillUse(tc.wantSkill, false)) {
				t.Errorf("cast payload % x", a.payload)
			}
		})
	}
}

// assistMinTest is the assist delay TestSupportRules plays with.
const assistMinTest = 700 * time.Millisecond

// handleOne feeds ev to the brain at now.
func (b *Brain) handleOne(t *testing.T, ev Event, now time.Time) {
	t.Helper()
	if err := b.handle([]Event{ev}, now); err != nil {
		t.Fatal(err)
	}
}

// TestBrainSupportFlow plays a heal and an assist through the loop with a
// fake clock: the leader's hit wakes the bot after the assist delay, never
// sooner; a heal goes out once, then the bot waits for its MagicSkillUse
// and records the buff cast for buffRetry.
func TestBrainSupportFlow(t *testing.T) {
	p := DefaultPersona()
	p.IdleChance = 0
	b, out := newTestBrain(t, p)
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillHeal, Level: 1}, {ID: skillMight, Level: 1}}}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{{ObjectID: pcID, HP: 100, MaxHP: 100}}}, t0)
	b.world.Apply(AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillMight, Left: time.Hour}}}, t0)
	b.world.Apply(PartySpelled{ObjectID: pcID, Effects: []Effect{{SkillID: skillMight, Left: time.Hour}}}, t0)

	b.handleOne(t, Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: npcID, Damage: 30}}}, at(time.Second))
	b.nextTick = at(time.Minute)
	if d := b.decisionAt().Sub(at(time.Second)); d < p.AssistMin || d > p.AssistMax {
		t.Fatalf("assist wakes %v after the hit, want %v-%v", d, p.AssistMin, p.AssistMax)
	}
	if b.assist.at != b.decisionAt() {
		t.Fatalf("wake %v, assist due %v", b.decisionAt(), b.assist.at)
	}
	if err := b.wake(b.decisionAt()); err != nil {
		t.Fatal(err)
	}
	if n := len(out.frames); n != 1 || out.frames[0][0] != opGameAction {
		t.Fatalf("frames %d, want one select click on the monster", n)
	}

	// The leader drops under 60 %: select it, then Heal it once.
	step := func(d time.Duration, evs ...Event) {
		t.Helper()
		if err := b.handle(evs, at(d)); err != nil {
			t.Fatal(err)
		}
		b.reactAt = at(d)
		if err := b.wake(at(d)); err != nil {
			t.Fatal(err)
		}
	}
	step(3*time.Second, PartySmallWindowUpdate{Member: PartyMember{ObjectID: pcID, HP: 40, MaxHP: 100}})
	if got := out.frames[len(out.frames)-1]; got[0] != opGameAction {
		t.Fatalf("packet % x, want the leader selected", got)
	}
	step(4*time.Second, MyTargetSelected{ObjectID: pcID})
	if got := out.frames[len(out.frames)-1]; !bytes.Equal(got, encodeRequestMagicSkillUse(skillHeal, false)) {
		t.Fatalf("packet % x, want Heal", got)
	}
	sent := len(out.frames)
	step(5 * time.Second)
	if len(out.frames) != sent {
		t.Fatalf("sent % x while the heal was pending", out.frames[len(out.frames)-1])
	}
	step(6*time.Second, MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: skillHeal, HitTime: time.Second, Reuse: 10 * time.Second})
	if len(out.frames) != sent {
		t.Fatalf("sent % x while casting", out.frames[len(out.frames)-1])
	}

	// The leader's Might runs low: after the heal, the bot renews it.
	step(8*time.Second, PartySmallWindowUpdate{Member: PartyMember{ObjectID: pcID, HP: 90, MaxHP: 100}},
		PartySpelled{ObjectID: pcID, Effects: []Effect{{SkillID: skillMight, Left: 50 * time.Second}}})
	if got := out.frames[len(out.frames)-1]; !bytes.Equal(got, encodeRequestMagicSkillUse(skillMight, false)) {
		t.Fatalf("packet % x, want Might", got)
	}
	if got := b.buffedAt[buffKey{pcID, skillMight}]; !got.Equal(at(8 * time.Second)) {
		t.Errorf("buffedAt = %v, want the cast time", got)
	}
}
