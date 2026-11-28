package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Monsters of the corpse tests besides npcID.
const (
	corpseMob2 = int32(0x20000071)
	corpseMob3 = int32(0x20000072)
)

// corpseSkills are the corpse tests' skill levels, as the datapack writes
// their cast range, radius and target (skills/1100-1199.xml,
// 1300-1399.xml, 0000-0099.xml): the corpse summons (range 40, 1 Crystal
// C a cast and an upkeep step), Corpse Life Drain and Life Scavenge (range
// 400), Corpse Burst (range 400, radius 200); MP costs kept small.
var corpseSkills = map[datapack.SkillRef]datapack.Skill{
	{ID: skillSummonCursedMan, Level: 1}: {
		Type: "SUMMON", Target: "CORPSE_MOB", NPCID: 14074, CastRange: 40, MPInitialConsume: 5, MPConsume: 20,
		ItemConsumeID: crystalC, ItemConsumeCount: 1, ItemConsumeIDOT: crystalC, ItemConsumeCountOT: 1, ItemConsumeSteps: 14,
	},
	{ID: skillSummonReanimatedMan, Level: 1}: {
		Type: "SUMMON", Target: "CORPSE_MOB", NPCID: 14001, CastRange: 40, MPInitialConsume: 5, MPConsume: 20,
		ItemConsumeID: crystalC, ItemConsumeCount: 1, ItemConsumeIDOT: crystalC, ItemConsumeCountOT: 1, ItemConsumeSteps: 14,
	},
	{ID: skillCorpseLifeDrain, Level: 1}: {Type: "DRAIN", Target: "CORPSE_MOB", CastRange: 400, MPConsume: 10},
	{ID: skillCorpseBurst, Level: 1}:     {Type: "MDAM", Target: "AREA_CORPSE_MOB", CastRange: 400, Radius: 200, MPConsume: 10},
	{ID: skillLifeScavenge, Level: 1}:    {Type: "DRAIN", Target: "CORPSE_MOB", CastRange: 400, MPConsume: 10},
}

// corpseMap is a travel map of corpseSkills, with npcID's template 20001
// lasting corpseTime (0 for the 7 s default).
func corpseMap(corpseTime time.Duration) *TravelMap {
	m := &TravelMap{skills: corpseSkills, reagents: indexReagents(corpseSkills), castRanges: make(map[datapack.SkillRef]int32)}
	for ref, sk := range corpseSkills {
		m.castRanges[ref] = int32(sk.CastRange)
	}
	if corpseTime > 0 {
		m.corpseTimes = map[int32]time.Duration{20001: corpseTime}
	}
	return m
}

// corpseBrain is a Necromancer of level 58 running at 200 that knows
// skills at level 1, carrying 10 Crystal C, with npcID 30 away and the
// default corpse time.
func corpseBrain(t *testing.T, skills ...int32) *Brain {
	t.Helper()
	b := rowBrain(t, classNecromancer, 58, "", skills...)
	b.cfg.Travel = corpseMap(0)
	carrying(b, reagentStack(crystalC, 10))
	return b
}

// allCorpse are the Necromancer's corpse skills.
var allCorpse = []int32{skillSummonCursedMan, skillSummonReanimatedMan, skillCorpseLifeDrain, skillCorpseBurst}

// killMob has id die at p at now.
func killMob(t *testing.T, b *Brain, id int32, p Point, now time.Time) {
	t.Helper()
	b.world.Apply(NPCInfo{ObjectID: id, NPCID: 20001, Attackable: true, Pos: p}, now)
	mustHandle(t, b, now, Die{ObjectID: id})
}

// corpseCastOf is the corpse rule's cast at now, 0 and 0 for none.
func corpseCastOf(b *Brain, now time.Time) (int32, int32) {
	b.pruneCorpses(now)
	return b.corpseCast(now)
}

// TestCorpseWindow checks the corpse window per template corpse time: a
// corpse at the bot's feet takes the summon until half its corpse time
// less corpseMargin and corpseFrameDelay, then it is out of reach, and gone
// once half its corpse time has passed.
func TestCorpseWindow(t *testing.T) {
	for _, tt := range []struct {
		name string
		ct   time.Duration
		half time.Duration
	}{
		{"default 7 s", 0, 3500 * time.Millisecond},
		{"template 20 s", 20 * time.Second, 10 * time.Second},
		{"template 4 s", 4 * time.Second, 2 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			b.cfg.Travel = corpseMap(tt.ct)
			dead := at(time.Second)
			killMob(t, b, npcID, Point{X: 30}, dead)
			last := tt.half - corpseMargin - corpseFrameDelay
			if s, id := corpseCastOf(b, dead.Add(last-time.Millisecond)); s != skillSummonCursedMan || id != npcID {
				t.Fatalf("just inside the window: cast %d on %#x, want the summon on the corpse", s, id)
			}
			if s, id := corpseCastOf(b, dead.Add(last)); id != 0 {
				t.Fatalf("at the margin: cast %d on %#x, want none", s, id)
			}
			b.pruneCorpses(dead.Add(tt.half))
			if len(b.mobCorpses) != 0 {
				t.Fatalf("corpses %v after half the corpse time, want none", b.mobCorpses)
			}
		})
	}
}

// TestCorpseReach checks the reach arithmetic: distance − cast range −
// both collision radii against the run speed times what is left of the
// window, at the death and later; the drain's 400 range reaches farther
// than the summon's 40.
func TestCorpseReach(t *testing.T) {
	// The window less the margins at the death: 3.5 − 0.5 − 0.25 = 2.75 s,
	// 550 units at 200 a second.
	for _, tt := range []struct {
		name   string
		x      int32
		radius float64
		after  time.Duration
		skill  int32
		want   bool
	}{
		{"summon at 590", 590, 0, 0, skillSummonCursedMan, true},
		{"summon at 591", 591, 0, 0, skillSummonCursedMan, false},
		{"collision radii count", 609, 10, 0, skillSummonCursedMan, true},
		{"collision radii count, one past", 610, 10, 0, skillSummonCursedMan, false},
		{"a second later: 350 units", 390, 0, time.Second, skillSummonCursedMan, true},
		{"a second later, one past", 391, 0, time.Second, skillSummonCursedMan, false},
		{"drain at 950", 950, 0, 0, skillCorpseLifeDrain, true},
		{"drain at 951", 951, 0, 0, skillCorpseLifeDrain, false},
		{"inside cast range past the window's margins", 20, 0, 2750 * time.Millisecond, skillCorpseLifeDrain, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			dead := at(time.Second)
			killMob(t, b, npcID, Point{X: tt.x}, dead)
			if tt.radius > 0 {
				b.world.Self.CollisionRadius, b.pvp.radii[npcID] = 9, tt.radius
			}
			c, ok := b.mobCorpses[npcID]
			if !ok {
				t.Fatal("corpse not tracked")
			}
			if got := b.corpseReach(npcID, c, tt.skill, dead.Add(tt.after)); got != tt.want {
				t.Errorf("corpseReach = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCorpsePriority checks summon before drain before burst: the summon
// while no servitor stands and the crystals are carried, Summon Cursed Man
// before Summon Reanimated Man; the drain under the 70 % sustain bound;
// Corpse Burst with two attackers around the corpse and no other monster.
func TestCorpsePriority(t *testing.T) {
	burstScene := func(b *Brain) {
		hitByTwo(b)
		b.world.Self.HP = 90
	}
	for _, tt := range []struct {
		name   string
		skills []int32
		setup  func(b *Brain, now time.Time)
		want   int32
	}{
		{name: "summon: cursed man first", skills: allCorpse, want: skillSummonCursedMan},
		{name: "summon: reanimated man before 56", skills: []int32{skillSummonReanimatedMan, skillCorpseLifeDrain}, want: skillSummonReanimatedMan},
		{name: "summon while hurt", skills: allCorpse, setup: func(b *Brain, _ time.Time) { b.world.Self.HP = 30 }, want: skillSummonCursedMan},
		{name: "servitor out: drain under 70 %", skills: allCorpse, setup: func(b *Brain, _ time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Self.HP = 65
		}, want: skillCorpseLifeDrain},
		{name: "dead servitor before its PetDelete: no summon", skills: allCorpse, setup: func(b *Brain, now time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Servitor.Dead = true
			b.world.Self.HP = 65
		}, want: skillCorpseLifeDrain},
		{name: "no crystals: drain", skills: allCorpse, setup: func(b *Brain, _ time.Time) {
			carrying(b)
			b.world.Self.HP = 65
		}, want: skillCorpseLifeDrain},
		{name: "summons cool: drain", skills: allCorpse, setup: func(b *Brain, now time.Time) {
			b.world.holdSkill(skillSummonCursedMan, now.Add(time.Minute))
			b.world.holdSkill(skillSummonReanimatedMan, now.Add(time.Minute))
			b.world.Self.HP = 65
		}, want: skillCorpseLifeDrain},
		{name: "summon MP short: drain", skills: allCorpse, setup: func(b *Brain, _ time.Time) {
			b.world.Self.MP = 20
			b.world.Self.HP = 65
		}, want: skillCorpseLifeDrain},
		{name: "at 70 %: no drain", skills: allCorpse, setup: func(b *Brain, _ time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Self.HP = 70
		}},
		{name: "two attackers: burst", skills: allCorpse, setup: func(b *Brain, _ time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			burstScene(b)
		}, want: skillCorpseBurst},
		{name: "one attacker: no burst", skills: allCorpse, setup: func(b *Brain, now time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			b.world.Self.HP = 90
			b.world.Apply(Attack{AttackerID: npcID, Pos: Point{X: 30}, Hits: []AttackHit{{TargetID: selfID, Damage: 10}}}, t0)
		}},
		{name: "another monster in the burst: none", skills: allCorpse, setup: func(b *Brain, now time.Time) {
			b.world.Apply(kaiInfo(1000), t0)
			burstScene(b)
			b.world.Apply(NPCInfo{ObjectID: corpseMob3, NPCID: 20001, Attackable: true, Pos: Point{X: 100}}, t0)
		}},
		{name: "no corpse skills known: none", skills: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, tt.skills...)
			now := at(time.Second)
			// The corpse lies 60 off the bot, between npcID (30) and
			// rowNPC2 (-30) of hitByTwo.
			killMob(t, b, corpseMob2, Point{Y: 60}, now)
			if tt.setup != nil {
				tt.setup(b, now)
			}
			s, id := corpseCastOf(b, now)
			if s != tt.want || (tt.want != 0) != (id == corpseMob2) {
				t.Errorf("cast %d on %#x, want %d on the corpse", s, id, tt.want)
			}
		})
	}
}

// TestCorpseNearest checks the rule takes the nearest corpse in reach.
func TestCorpseNearest(t *testing.T) {
	b := corpseBrain(t, allCorpse...)
	now := at(time.Second)
	killMob(t, b, corpseMob2, Point{X: 200}, now)
	killMob(t, b, corpseMob3, Point{X: -100}, now)
	if s, id := corpseCastOf(b, now); s != skillSummonCursedMan || id != corpseMob3 {
		t.Fatalf("cast %d on %#x, want the summon on the nearer corpse %#x", s, id, corpseMob3)
	}
}

// TestCorpseSpoiledExcluded checks the rule never takes a corpse the bot
// spoiled (swept or not: the sweep comes first) or anyone spoiled
// (Die.Sweepable: a party member's sweep comes first), and takes one whose
// Spoil the bot saw resisted.
func TestCorpseSpoiledExcluded(t *testing.T) {
	for _, tt := range []struct {
		name      string
		mark      spoilState
		sweepable bool
		want      bool
	}{
		{name: "spoiled by the bot", mark: spoilMine, sweepable: true},
		{name: "spoiled by the bot, nothing to sweep", mark: spoilMine},
		{name: "spoiled by another", mark: spoilOther},
		{name: "spoiled by a party member", sweepable: true},
		{name: "spoil resisted", mark: spoilResisted, want: true},
		{name: "not spoiled", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			if tt.mark != 0 {
				b.markSpoil(npcID, tt.mark)
			}
			now := at(time.Second)
			mustHandle(t, b, now, Die{ObjectID: npcID, Sweepable: tt.sweepable})
			if _, got := b.mobCorpses[npcID]; got != tt.want {
				t.Fatalf("corpse tracked %v, want %v", got, tt.want)
			}
			if _, id := corpseCastOf(b, now); (id == npcID) != tt.want {
				t.Fatalf("cast on %#x, want a cast on the corpse %v", id, tt.want)
			}
		})
	}
}

// TestCorpseRefusedDrop checks the server's refusal of a corpse cast in
// flight: 1247 (too old) and 109 (no corpse) drop the corpse, and 1247
// ends the cast in flight so the next corpse is tried at once; the same
// message with no corpse cast in flight drops nothing.
func TestCorpseRefusedDrop(t *testing.T) {
	for _, tt := range []struct {
		name    string
		msg     int32
		pending bool
		drop    bool
	}{
		{"too old in flight", msgCorpseTooOld, true, true},
		{"no corpse in flight", msgInvalidTarget, true, true},
		{"too old, nothing in flight", msgCorpseTooOld, false, false},
		{"another refusal", msgNotEnoughMP, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			now := at(time.Second)
			killMob(t, b, npcID, Point{X: 30}, now)
			killMob(t, b, corpseMob2, Point{X: -40}, now)
			b.world.Target = npcID
			a, ok := b.ruleCorpse(now)
			if !ok {
				t.Fatal("corpse rule yielded")
			}
			wantCast(t, a, skillSummonCursedMan, npcID)
			wantFrame(t, a, skillSummonCursedMan)
			mustSend(t, b, a, now)
			refused := now.Add(100 * time.Millisecond)
			if !tt.pending {
				refused = now.Add(castTimeout + time.Millisecond)
			}
			mustHandle(t, b, refused, SystemMessage{ID: tt.msg})
			if _, kept := b.mobCorpses[npcID]; kept == tt.drop {
				t.Fatalf("corpse kept %v, want dropped %v", kept, tt.drop)
			}
			if tt.msg != msgCorpseTooOld || !tt.pending {
				return
			}
			if b.castPending(refused) {
				t.Fatal("cast still in flight after 1247")
			}
			a, _ = b.ruleCorpse(refused)
			if a.kind != actSelect || a.object != corpseMob2 {
				t.Fatalf("after 1247: kind %d object %#x, want the next corpse selected", a.kind, a.object)
			}
		})
	}
}

// TestCorpseDropped checks a corpse is forgotten on its DeleteObject and on
// the bot's own corpse cast at it (the skill consumes it), and that
// another's cast at it keeps it.
func TestCorpseDropped(t *testing.T) {
	for _, tt := range []struct {
		name string
		ev   Event
		drop bool
	}{
		{"delete object", DeleteObject{ObjectID: npcID}, true},
		{"own corpse cast", MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skillCorpseLifeDrain, HitTime: 1500 * time.Millisecond}, true},
		{"another's cast", MagicSkillUse{CasterID: corpseMob3, TargetID: npcID, SkillID: skillCorpseLifeDrain}, false},
		{"own other cast", MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: skillBlaze}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			now := at(time.Second)
			killMob(t, b, npcID, Point{X: 30}, now)
			mustHandle(t, b, now.Add(100*time.Millisecond), tt.ev)
			if _, kept := b.mobCorpses[npcID]; kept == tt.drop {
				t.Fatalf("corpse kept %v, want dropped %v", kept, tt.drop)
			}
		})
	}
}

// TestCorpseRuleSteps checks what the rule does before it casts: it stands
// a seated bot up, holds the turn while a cast is in flight, and selects
// the corpse when it is not the target.
func TestCorpseRuleSteps(t *testing.T) {
	for _, tt := range []struct {
		name     string
		setup    func(b *Brain, now time.Time)
		wantKind actionKind
		object   int32
	}{
		{name: "ready: casts", setup: func(*Brain, time.Time) {}, wantKind: actCast, object: npcID},
		{name: "seated: stands up", setup: func(b *Brain, _ time.Time) { b.world.Self.Sitting = true }, wantKind: actSitStand},
		{name: "cast in flight: holds", setup: func(b *Brain, now time.Time) { b.castAt = now }},
		{name: "corpse not the target: selects it", setup: func(b *Brain, _ time.Time) { b.world.Target = 0 }, wantKind: actSelect, object: npcID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := corpseBrain(t, allCorpse...)
			now := at(time.Second)
			killMob(t, b, npcID, Point{X: 30}, now)
			b.world.Target = npcID
			tt.setup(b, now)
			a, ok := b.ruleCorpse(now)
			if !ok || a.kind != tt.wantKind || a.object != tt.object {
				t.Errorf("ruleCorpse = kind %d object %#x ok %v, want kind %d object %#x", a.kind, a.object, ok, tt.wantKind, tt.object)
			}
		})
	}
}

// TestCorpseRows checks only the corpse rows track corpses: the Dark
// Avenger drains one under its 60 % sustain bound with Life Scavenge, and a
// row without corpse skills (the Warlock) tracks none, so the rule never
// takes its turn (D24).
func TestCorpseRows(t *testing.T) {
	t.Run("dark avenger", func(t *testing.T) {
		b := rowBrain(t, classDarkAvenger, 58, "", skillLifeScavenge)
		b.cfg.Travel = corpseMap(0)
		now := at(time.Second)
		killMob(t, b, corpseMob2, Point{X: 100}, now)
		b.world.Self.HP = 60
		if s, _ := corpseCastOf(b, now); s != 0 {
			t.Fatalf("at 60 %%: cast %d, want none", s)
		}
		b.world.Self.HP = 59
		if s, id := corpseCastOf(b, now); s != skillLifeScavenge || id != corpseMob2 {
			t.Fatalf("under 60 %%: cast %d on %#x, want Life Scavenge on the corpse", s, id)
		}
	})
	t.Run("warlock", func(t *testing.T) {
		b := rowBrain(t, classWarlock, 58, "", skillSummonFelineQueen)
		b.cfg.Travel = corpseMap(0)
		killMob(t, b, npcID, Point{X: 30}, at(time.Second))
		if len(b.mobCorpses) != 0 {
			t.Fatalf("corpses %v, want none", b.mobCorpses)
		}
		if _, ok := b.ruleCorpse(at(time.Second)); ok {
			t.Fatal("corpse rule took the turn")
		}
	})
}

// TestCorpseRuleOrder checks the corpse rule comes right after sweep.
func TestCorpseRuleOrder(t *testing.T) {
	for i, r := range playRules {
		if r.name == "sweep" {
			if next := playRules[i+1].name; next != "corpse" {
				t.Fatalf("rule after sweep = %q, want corpse", next)
			}
			return
		}
	}
	t.Fatal("no sweep rule")
}
