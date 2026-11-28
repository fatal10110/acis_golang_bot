package botclient

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// encodeCast is the cast request of skill (no ctrl).
func encodeCast(skill int32) []byte { return botwire.EncodeRequestMagicSkillUse(skill, false) }

// Skills of the servitor upkeep tests (aCis_datapack/data/xml/skills).
const (
	skillSummonKai     int32 = 1276
	skillSummonKat     int32 = 1111
	skillSummonMew     int32 = 1225
	skillServitorHeal  int32 = 1127
	skillServitorPDef  int32 = 1140
	skillServitorHaste int32 = 1141
	kaiServitor              = int32(0x10000888)
)

// upkeepSkills are the upkeep tests' skill levels, as the datapack writes
// them: Kai the Cat level 1 (1 Crystal C a cast and an upkeep step, 14
// steps), a Kat the Cat level of 5 Crystal D a cast and 1 a step, Mew the
// Cat (0.9 XP penalty), Servitor Heal (range 600) and two servitor buffs
// (range 400, 1200 s).
var upkeepSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillSummonKai, Level: 1}: {
		Type: "SUMMON", Target: "SELF", NPCID: kaiCat.ID, MPInitialConsume: 14, MPConsume: 56, ExpPenalty: 0.1,
		ItemConsumeID: crystalC, ItemConsumeCount: 1, ItemConsumeIDOT: crystalC, ItemConsumeCountOT: 1, ItemConsumeSteps: 14,
		SummonLifetime: 1200 * time.Second,
	},
	{ID: skillSummonKat, Level: 1}: {
		Type: "SUMMON", Target: "SELF", NPCID: 14115, MPInitialConsume: 14, MPConsume: 56, ExpPenalty: 0.3,
		ItemConsumeID: crystalD, ItemConsumeCount: 5, ItemConsumeIDOT: crystalD, ItemConsumeCountOT: 1, ItemConsumeSteps: 14,
		SummonLifetime: 1200 * time.Second,
	},
	{ID: skillSummonMew, Level: 1}: {
		Type: "SUMMON", Target: "SELF", NPCID: 14301, MPConsume: 30, ExpPenalty: 0.9,
		ItemConsumeID: crystalD, ItemConsumeCount: 1, SummonLifetime: 1200 * time.Second,
	},
	{ID: skillServitorHeal, Level: 1}:  {Type: "HEAL", Target: "SUMMON", CastRange: 600, MPConsume: 10},
	{ID: skillServitorPDef, Level: 1}:  {Type: "BUFF", Target: "SUMMON", CastRange: 400, MPConsume: 10, Effects: []datapack.Effect{{Name: "Buff", Time: 1200}}},
	{ID: skillServitorHaste, Level: 1}: {Type: "BUFF", Target: "SUMMON", CastRange: 400, MPConsume: 10, Effects: []datapack.Effect{{Name: "Buff", Time: 1200}}},
}

// summonerTestKit is a summoner row as #761 will ship it: Kai the Cat,
// then the Crystal D fallback, then Mew (which the gate never casts), its
// heal and two servitor buffs.
var summonerTestKit = ClassKit{
	Summons:       []int32{skillSummonKai, skillSummonKat, skillSummonMew},
	ServitorHeal:  skillServitorHeal,
	ServitorBuffs: []int32{skillServitorPDef, skillServitorHaste},
}

// upkeepBrain is kitBrain with kit, out of combat at full MP, knowing
// skills at level 1 of upkeepSkills, carrying items.
func upkeepBrain(t *testing.T, kit ClassKit, skills []int32, items ...Item) *Brain {
	t.Helper()
	b := kitBrain(t, kit, 100, skills...)
	b.world.Self.InCombat = false
	b.world.Target = 0
	b.cfg.Travel = &TravelMap{
		skills: upkeepSkills, reagents: indexReagents(upkeepSkills),
		servitors: map[int32]datapack.NPC{kaiCat.ID: kaiCat},
	}
	carrying(b, items...)
	b.summonReady = b.summonGate()
	return b
}

// kaiInfo is Kai the Cat (Summon Kai the Cat level 1) out next to the bot
// at hp of its 1000 HP.
func kaiInfo(hp int32) PetInfo {
	return PetInfo{
		SummonType: SummonServitor, ObjectID: kaiServitor, TemplateID: kaiCat.ID, Level: 42,
		Pos: Point{X: 50, Y: 50}, CurHP: hp, MaxHP: 1000, CurMP: 300, MaxMP: 300,
	}
}

// allSummons are the skills of summonerTestKit.
var allSummons = []int32{skillSummonKai, skillSummonKat, skillSummonMew, skillServitorHeal, skillServitorPDef, skillServitorHaste}

// serverTick is the server's lifetime tick (summon/lifetime.go Tick),
// copied as the reference the prediction is checked against: remaining
// loses cost, expired below 0, and an upkeep is due when the tick takes
// remaining from above next to at or below it, next then moving one step
// down.
func serverTick(remaining, next, total, steps, cost int64) (int64, int64, bool, bool) {
	old := remaining
	remaining -= cost
	if remaining < 0 {
		return remaining, next, true, false
	}
	if remaining <= next && old > next {
		return remaining, next - total/(steps+1), false, true
	}
	return remaining, next, false, false
}

// TestUpkeepNext checks the predicted next checkpoint against fixed
// vectors and against the server's tick over full lives: steps+1 payments,
// the last at remaining 0 (docs/servitors.md section 3).
func TestUpkeepNext(t *testing.T) {
	for _, tt := range []struct {
		remaining, total int64
		steps            int32
		want             int64
		ok               bool
	}{
		{1200000, 1200000, 14, 1120000, true}, // InitialNextConsumeTime
		{1120001, 1200000, 14, 1120000, true},
		{1120000, 1200000, 14, 1040000, true}, // paid on the tick that reached it
		{80000, 1200000, 14, 0, true},         // the last step
		{1, 1200000, 14, 0, true},
		{0, 1200000, 14, 0, false}, // past the last payment
		{10000, 10000, 1, 5000, true},
		{5000, 10000, 1, 0, true},
		{1200000, 1200000, 0, 0, false}, // no steps (Kat the Cat levels 1 and 2)
		{1200000, 1200000, 6, 1028572, true},
		{171432, 1200000, 6, 4, true}, // 7 does not divide the total: the last at 4
	} {
		got, ok := upkeepNext(tt.remaining, tt.total, tt.steps)
		if got != tt.want || ok != tt.ok {
			t.Errorf("upkeepNext(%d, %d, %d) = %d, %v; want %d, %v", tt.remaining, tt.total, tt.steps, got, ok, tt.want, tt.ok)
		}
	}
	for _, tt := range []struct {
		name                string
		total, steps, cost  int64
		payments, lastAtRem int64
	}{
		{"Kai at combat cost", 1200000, 14, 1000, 15, 0},
		{"Kai idle", 1200000, 14, 500, 15, 0},
		{"server test vector", 10000, 1, 2000, 2, 0},
		{"Dark Panther", 1200000, 4, 1000, 5, 0},
		{"seven checkpoints", 1200000, 6, 1000, 7, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			remaining, next := tt.total, tt.total-tt.total/(tt.steps+1) // InitialNextConsumeTime
			payments, last := int64(0), int64(-1)
			for {
				if want, ok := upkeepNext(remaining, tt.total, int32(tt.steps)); ok && want != next {
					t.Fatalf("at %d: upkeepNext %d, the server's next %d", remaining, want, next)
				}
				var expired, due bool
				remaining, next, expired, due = serverTick(remaining, next, tt.total, tt.steps, tt.cost)
				if expired {
					break
				}
				if due {
					payments++
					last = remaining
				}
			}
			if payments != tt.payments || last > tt.lastAtRem {
				t.Errorf("%d payments, the last at %d; want %d, the last at %d", payments, last, tt.payments, tt.lastAtRem)
			}
		})
	}
}

// TestSummonGate checks the summon the gate opens: the kit's first summon
// the bot knows whose crystals for the cast plus one upkeep step it
// carries, the Crystal D fallback with Crystal D only, never one of an XP
// penalty of 0.5 or more, none for a kit without Summons.
func TestSummonGate(t *testing.T) {
	kai, kat := datapack.SkillRef{ID: skillSummonKai, Level: 1}, datapack.SkillRef{ID: skillSummonKat, Level: 1}
	for _, tt := range []struct {
		name   string
		kit    ClassKit
		skills []int32
		items  []Item
		want   datapack.SkillRef
	}{
		{"cast plus one step", summonerTestKit, allSummons, []Item{reagentStack(crystalC, 2)}, kai},
		{"the cast only", summonerTestKit, allSummons, []Item{reagentStack(crystalC, 1)}, datapack.SkillRef{}},
		{"Crystal D fallback", summonerTestKit, allSummons, []Item{reagentStack(crystalD, 6)}, kat},
		{"own summon first", summonerTestKit, allSummons, []Item{reagentStack(crystalC, 2), reagentStack(crystalD, 6)}, kai},
		{"too few Crystal D, never Mew", summonerTestKit, allSummons, []Item{reagentStack(crystalD, 5)}, datapack.SkillRef{}},
		{"unknown summon", summonerTestKit, []int32{skillSummonKat}, []Item{reagentStack(crystalC, 9)}, datapack.SkillRef{}},
		{"no Summons in the kit", ClassKit{}, allSummons, []Item{reagentStack(crystalC, 9), reagentStack(crystalD, 9)}, datapack.SkillRef{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := upkeepBrain(t, tt.kit, tt.skills, tt.items...)
			if got := b.summonGate(); got != tt.want {
				t.Errorf("summonGate = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRuleSummon checks when the summon rule casts the summon: no
// servitor out (a dead one stays until its PetDelete), out of combat and
// unattacked, with the MP of the cast, one cast per summonRetry.
func TestRuleSummon(t *testing.T) {
	kaiCast := encodeCast(skillSummonKai)
	for _, tt := range []struct {
		name  string
		setup func(b *Brain)
		at    time.Duration
		want  bool
	}{
		{name: "nothing out", want: true},
		{name: "servitor out", setup: func(b *Brain) { b.world.Apply(kaiInfo(1000), t0) }},
		{name: "dead servitor before its PetDelete", setup: func(b *Brain) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Apply(Die{ObjectID: kaiServitor}, t0)
		}},
		{name: "after its PetDelete", want: true, setup: func(b *Brain) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Apply(Die{ObjectID: kaiServitor}, t0)
			b.world.Apply(PetDelete{SummonType: SummonServitor, ObjectID: kaiServitor}, t0)
		}},
		{name: "in combat", setup: func(b *Brain) { b.world.Self.InCombat = true }},
		{name: "attacked", setup: func(b *Brain) {
			b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, t0)
		}},
		{name: "short of MP", setup: func(b *Brain) { b.world.Self.MP = 60 }},
		{name: "no crystals", setup: func(b *Brain) {
			carrying(b)
			b.summonReady = b.summonGate()
		}},
		{name: "within summonRetry", setup: func(b *Brain) { b.upkeep.summonAt = t0 }, at: summonRetry - time.Second},
		{name: "after summonRetry", setup: func(b *Brain) { b.upkeep.summonAt = t0 }, at: summonRetry, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := upkeepBrain(t, summonerTestKit, allSummons, reagentStack(crystalC, 2))
			if tt.setup != nil {
				tt.setup(b)
			}
			a, ok := b.ruleSummon(at(tt.at))
			if got := ok && bytes.Equal(a.payload, kaiCast); got != tt.want {
				t.Fatalf("ruleSummon = %v (% x), want the summon cast %v", ok, a.payload, tt.want)
			}
		})
	}
}

// TestSummonFlow plays a servitor's death through the loop: the bot
// summons, holds while the cast is out, sends no second summon while the
// dead servitor's corpse stands, and summons again after its PetDelete.
func TestSummonFlow(t *testing.T) {
	b := upkeepBrain(t, summonerTestKit, allSummons, reagentStack(crystalC, 4))
	if rule, a := b.decide(at(time.Second)); rule != "summon" || !bytes.Equal(a.payload, encodeCast(skillSummonKai)) {
		t.Fatalf("decide = %q % x, want the summon", rule, a.payload)
	}
	sendNow(t, b, Action{kind: actCast, payload: encodeCast(skillSummonKai), object: selfID, skill: skillSummonKai}, at(time.Second))
	b.world.Apply(kaiInfo(1000), at(17*time.Second))
	b.world.Apply(Die{ObjectID: kaiServitor}, at(60*time.Second))
	if rule, _ := b.decide(at(61 * time.Second)); rule == "summon" {
		t.Fatal("summoned over the dead servitor's corpse")
	}
	b.world.Apply(PetDelete{SummonType: SummonServitor, ObjectID: kaiServitor}, at(67*time.Second))
	if rule, a := b.decide(at(68 * time.Second)); rule != "summon" || !bytes.Equal(a.payload, encodeCast(skillSummonKai)) {
		t.Fatalf("decide after PetDelete = %q % x, want the summon", rule, a.payload)
	}
}

// TestServitorHeal checks Servitor Heal: under ServitorHealHP out of
// combat, in combat only under half of it, in its range, known.
func TestServitorHeal(t *testing.T) {
	heal := encodeCast(skillServitorHeal)
	for _, tt := range []struct {
		name   string
		hp     int32
		combat bool
		setup  func(b *Brain)
		want   bool
	}{
		{name: "under half, out of combat", hp: 400, want: true},
		{name: "above half", hp: 600},
		{name: "under half in combat", hp: 400, combat: true},
		{name: "under a quarter in combat", hp: 200, combat: true, want: true},
		{name: "out of range", hp: 200, setup: func(b *Brain) { b.world.Servitor.Pos = Point{X: 700} }},
		{name: "heal unknown", hp: 200, setup: func(b *Brain) { delete(b.world.Skills, skillServitorHeal) }},
		{name: "no heal in the kit", hp: 200, setup: func(b *Brain) { b.built.Kit.ServitorHeal = 0 }},
		{name: "dead servitor", hp: 200, setup: func(b *Brain) { b.world.Apply(Die{ObjectID: kaiServitor}, t0) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := upkeepBrain(t, summonerTestKit, allSummons)
			b.world.Apply(kaiInfo(tt.hp), t0)
			b.world.Self.InCombat = tt.combat
			if tt.setup != nil {
				tt.setup(b)
			}
			a, ok := b.ruleServitor(at(time.Second))
			if got := ok && bytes.Equal(a.payload, heal); got != tt.want {
				t.Fatalf("ruleServitor = %v (% x), want Servitor Heal %v", ok, a.payload, tt.want)
			}
		})
	}
}

// TestServitorBuffs checks the servitor buffs: each goes on a new
// servitor, once per buffRetry while its cast never started, again once its
// 1200 s from its cast's start run out within RebuffBefore, and afresh on
// the next servitor.
func TestServitorBuffs(t *testing.T) {
	pdef, haste := encodeCast(skillServitorPDef), encodeCast(skillServitorHaste)
	b := upkeepBrain(t, summonerTestKit, allSummons)
	b.world.Apply(kaiInfo(1000), t0)
	rebuff := func(d time.Duration) []byte {
		t.Helper()
		a, ok := b.ruleRebuff(at(d))
		if !ok {
			return nil
		}
		return a.payload
	}
	cast := func(d time.Duration, skill int32, started bool) {
		t.Helper()
		sendNow(t, b, Action{kind: actCast, payload: encodeCast(skill), object: kaiServitor, skill: skill}, at(d))
		b.castAt = time.Time{}
		b.world.holdSkill(skill, time.Time{})
		if started {
			if err := b.handle([]Event{MagicSkillUse{CasterID: selfID, TargetID: kaiServitor, SkillID: skill, Level: 1, HitTime: 4 * time.Second}}, at(d)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := rebuff(time.Second); !bytes.Equal(got, pdef) {
		t.Fatalf("first buff % x, want Servitor Physical Shield", got)
	}
	cast(time.Second, skillServitorPDef, true)
	if got := rebuff(10 * time.Second); !bytes.Equal(got, haste) {
		t.Fatalf("second buff % x, want Servitor Haste", got)
	}
	cast(10*time.Second, skillServitorHaste, false) // refused: its cast never started
	if got := rebuff(20 * time.Second); got != nil {
		t.Fatalf("within buffRetry: % x, want none", got)
	}
	if got := rebuff(10*time.Second + buffRetry); !bytes.Equal(got, haste) {
		t.Fatalf("after buffRetry % x, want Servitor Haste again", got)
	}
	cast(10*time.Second+buffRetry, skillServitorHaste, true)
	expiry := time.Second + servitorBuffTime - b.persona.RebuffBefore
	if got := rebuff(expiry - time.Second); got != nil {
		t.Fatalf("before expiry: % x, want none", got)
	}
	if got := rebuff(expiry); !bytes.Equal(got, pdef) {
		t.Fatalf("at expiry % x, want Servitor Physical Shield again", got)
	}
	// A new servitor has none of the buffs.
	b.world.Apply(PetDelete{SummonType: SummonServitor, ObjectID: kaiServitor}, at(time.Minute))
	next := kaiInfo(1000)
	next.ObjectID = kaiServitor + 1
	b.world.Apply(next, at(2*time.Minute))
	if got := rebuff(2*time.Minute + time.Second); !bytes.Equal(got, pdef) {
		t.Fatalf("new servitor % x, want Servitor Physical Shield", got)
	}
}

// TestUpkeepBlocks checks the pull gate: no new pull while the servitor's
// next checkpoint is due within upkeepHorizon and the bot cannot pay it.
func TestUpkeepBlocks(t *testing.T) {
	const total = 1200 * time.Second
	for _, tt := range []struct {
		name      string
		remaining time.Duration
		crystals  int32
		lifetime  bool
		want      bool
	}{
		{name: "due, unpaid", remaining: 1120*time.Second + upkeepHorizon, lifetime: true, want: true},
		{name: "due, paid", remaining: 1120*time.Second + upkeepHorizon, crystals: 1, lifetime: true},
		{name: "not due", remaining: 1120*time.Second + upkeepHorizon + time.Second, lifetime: true},
		{name: "last step due", remaining: 10 * time.Second, lifetime: true, want: true},
		{name: "checkpoint just paid", remaining: 1120 * time.Second, lifetime: true},
		{name: "before the first lifetime bar"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var items []Item
			if tt.crystals > 0 {
				items = append(items, reagentStack(crystalC, tt.crystals))
			}
			b := upkeepBrain(t, summonerTestKit, allSummons, items...)
			b.world.Apply(kaiInfo(1000), t0)
			if tt.lifetime {
				b.world.Apply(SetSummonRemainTime{Total: total, Remaining: tt.remaining}, t0)
			}
			if got := b.upkeepBlocks(); got != tt.want {
				t.Fatalf("upkeepBlocks = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFarmUpkeepGate checks the farm rule starts no fight on a fresh
// monster while the upkeep blocks, does with the upkeep carried, and still
// fights an attacker.
func TestFarmUpkeepGate(t *testing.T) {
	for _, tt := range []struct {
		name           string
		attacked, paid bool
		want           bool
	}{
		{name: "fresh monster"},
		{name: "fresh monster, upkeep carried", paid: true, want: true},
		{name: "attacker", attacked: true, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 0, 0, 1, 3, 3))
			withKit(b, summonerTestKit)
			b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillSummonKai, Level: 1}}}, t0)
			b.cfg.Travel = &TravelMap{skills: upkeepSkills}
			if tt.paid {
				carrying(b, reagentStack(crystalC, 1))
			}
			b.decide(at(time.Second))
			if !b.spot.farming() {
				t.Fatalf("spot %+v: want one held", b.spot)
			}
			b.spot.arrived = true
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Apply(SetSummonRemainTime{Total: 1200 * time.Second, Remaining: 1121 * time.Second}, t0)
			b.world.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 30}}, t0)
			if tt.attacked {
				b.world.Apply(Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: selfID, Damage: 5}}}, at(2*time.Second))
			}
			b.world.Self.InCombat = false
			_, ok := b.ruleFarm(at(2 * time.Second))
			if ok != tt.want {
				t.Fatalf("ruleFarm = %v, want %v", ok, tt.want)
			}
		})
	}
}

// TestServitorDeathWindow checks two servitor deaths within
// ServitorDeathWindow at the farmed spot mark it too hard, as the bot's
// own death does; deaths farther apart do not, nor one servitor's Die
// twice.
func TestServitorDeathWindow(t *testing.T) {
	for _, tt := range []struct {
		name   string
		second time.Duration
		same   bool
		want   bool
	}{
		{name: "two within the window", second: 9 * time.Minute, want: true},
		{name: "two apart", second: 11 * time.Minute},
		{name: "one servitor's Die twice", second: time.Minute, same: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 1, testSpot("a", 2000, 0, 1, 3, 3))
			b.world.Objects = map[int32]*Object{}
			b.decide(at(time.Second))
			if !b.spot.farming() {
				t.Fatalf("spot %+v: want one held", b.spot)
			}
			die := func(id int32, d time.Duration) {
				t.Helper()
				info := kaiInfo(1000)
				info.ObjectID = id
				if b.world.Servitor == nil || b.world.Servitor.ObjectID != id {
					b.world.Apply(info, at(d))
				}
				if err := b.handle([]Event{Die{ObjectID: id}}, at(d)); err != nil {
					t.Fatal(err)
				}
			}
			die(kaiServitor, 2*time.Second)
			if !b.spot.farming() {
				t.Fatal("one servitor death gave the spot up")
			}
			second := kaiServitor + 1
			if tt.same {
				second = kaiServitor
			}
			die(second, 2*time.Second+tt.second)
			if got := !b.spot.farming() && at(2*time.Second+tt.second).Before(b.spot.avoid[0]); got != tt.want {
				t.Fatalf("spot %+v avoid %v: marked too hard %v, want %v", b.spot, b.spot.avoid, got, tt.want)
			}
		})
	}
}

// TestReagentNeedsSummonLife checks a summon counts one full life, the
// cast and its steps+1 checkpoints, as one cast of the restock, which buys
// no crystal: no spawned merchant sells them.
func TestReagentNeedsSummonLife(t *testing.T) {
	b := upkeepBrain(t, summonerTestKit, []int32{skillSummonKai})
	b.cfg.Travel = reagentTravel()
	b.cfg.Travel.skills = upkeepSkills
	b.persona.Shop.ReagentCastsMin, b.persona.Shop.ReagentCastsMax = 2, 3
	got := b.reagentNeeds()
	// Kai level 1: a 1 C cast and 15 steps of 1 C.
	want := []reagentNeed{{item: crystalC, min: 2 * 16, max: 3 * 16}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("reagentNeeds = %+v, want %+v", got, want)
	}
	if !b.usesReagent(crystalC) {
		t.Error("Crystal C is not kept from the junk sale")
	}
	if rows := b.reagentWants(); len(rows) != 0 {
		t.Errorf("reagentWants = %+v, want no crystal bought", rows)
	}
}

// TestNoSummonRowsUnchanged checks a row without Summons (every row
// before #761, D24) neither summons nor heals or buffs a servitor, nor
// gates its pulls, crystals and skills carried.
func TestNoSummonRowsUnchanged(t *testing.T) {
	b := upkeepBrain(t, ClassKit{}, allSummons, reagentStack(crystalC, 9), reagentStack(crystalD, 9))
	if b.summonReady != (datapack.SkillRef{}) {
		t.Fatalf("summonReady = %v, want none", b.summonReady)
	}
	if a, ok := b.ruleSummon(at(time.Second)); ok {
		t.Fatalf("ruleSummon = % x, want none", a.payload)
	}
	b.world.Apply(kaiInfo(200), t0)
	b.world.Apply(SetSummonRemainTime{Total: 1200 * time.Second, Remaining: time.Second}, t0)
	if a, ok := b.servitorHeal(b.world.Servitor, at(time.Second)); ok {
		t.Fatalf("servitorHeal = % x, want none", a.payload)
	}
	if a, ok := b.servitorBuff(at(time.Second)); ok {
		t.Fatalf("servitorBuff = % x, want none", a.payload)
	}
	if b.upkeepBlocks() {
		t.Fatal("upkeepBlocks without a kit summon")
	}
}

// TestPersonaServitorOverride checks the persona's Summons, ServitorHeal
// and ServitorBuffs replace the build's when set, and leave it alone
// unset.
func TestPersonaServitorOverride(t *testing.T) {
	bd := Build{Kit: summonerTestKit}
	var p Persona
	if got := p.override(bd, classHumanMystic).Kit; got.ServitorHeal != skillServitorHeal || len(got.Summons) != 3 || len(got.ServitorBuffs) != 2 {
		t.Fatalf("unset override changed the kit: %+v", got)
	}
	p.Summons, p.ServitorHeal, p.ServitorBuffs = []int32{skillSummonKat}, 1, []int32{skillServitorHaste}
	got := p.override(bd, classHumanMystic).Kit
	if got.ServitorHeal != 1 || len(got.Summons) != 1 || got.Summons[0] != skillSummonKat || len(got.ServitorBuffs) != 1 || got.ServitorBuffs[0] != skillServitorHaste {
		t.Fatalf("override = %+v, want the persona's", got)
	}
}
