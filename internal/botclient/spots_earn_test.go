package botclient

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// toadNPC is the giant toads' template in the earning tests.
const toadNPC int32 = 20121

// penal6 is squire6 with a death penalty's HP to spare: it lasts 24.6 s
// against a level 5 toad it kills in 16 s, so the toads fail its margin of 2
// (packFits) and pass the earning mode's margin of 1 (earnMargin).
var penal6 = squire{level: 6, pAtk: 6, pDef: 79, evasion: 38, accuracy: 38, atkSpd: 416, maxHP: 150}

// earnSpot is a level 5 toad spot of the earning tests at x on the split
// navigation, its five toads its monsters.
func earnSpot(name string, x int32) datapack.Spot {
	s := tiToads27
	s.Maker, s.Center, s.MaximumNPCs = name, datapack.Point{X: x}, 6
	s.Mobs = []datapack.Mob{{NPCID: toadNPC, Level: 5, Count: 5, Combat: giantToad}}
	s.Monsters = s.Mobs
	return s
}

// wolfSpot is a spot of five werewolf chieftains of level at x: no level 6
// bot lasts there.
func wolfSpot(name string, x int32, level int) datapack.Spot {
	s := tiWolves
	s.Maker, s.Center, s.MaximumNPCs = name, datapack.Point{X: x}, 6
	s.MinLevel, s.MaxLevel, s.AggroLevel = level, level, level
	s.Mobs = []datapack.Mob{{NPCID: toadNPC + 1, Level: level, Count: 5, Combat: werewolfChieftain}}
	s.Monsters = s.Mobs
	return s
}

// newEarnBrain is a level 6 brain at the origin playing self with death
// penalty level dp and 1,000 adena, picking from spots, a toad's kill
// worth worth[0] adena at the first spot's template and worth[1] at the
// second's.
func newEarnBrain(t *testing.T, self squire, dp int32, worth map[int32]float64, spots ...datapack.Spot) *Brain {
	t.Helper()
	b := newSpotBrain(t, self.level, spots...)
	b.persona.Potions = []int32{lesserHealingPotion}
	setSquire(b, self, 0)
	b.cfg.Travel.killValues = worth
	b.world.Apply(EtcStatusUpdate{DeathPenalty: dp}, t0)
	b.world.Apply(ItemList{Items: []Item{adenaStack(1000)}}, t0)
	return b
}

// TestEarnClaim checks the earning mode's claim (#527): a level 6 bot with
// death penalty 15 and 1,000 adena, no spot of its level fitting its stats
// (packFits) and none under its level window, claims the spot in reach
// earning it the most adena an hour by the looser survival bound (step 1),
// else the one with the lowest monsters whatever its pack (step 2); never a
// spot it avoids nor one full to its cap. A bot with no penalty picks as
// before: none.
func TestEarnClaim(t *testing.T) {
	rich := map[int32]float64{toadNPC: 20}
	tests := []struct {
		name    string
		dp      int32
		self    squire
		worth   map[int32]float64
		spots   []datapack.Spot
		edit    func(b *Brain)
		want    int // -1: none held
		earning bool
	}{
		{name: "step 1: the toads earn", dp: 15, self: penal6, worth: rich, spots: []datapack.Spot{earnSpot("toads", 2000)}, want: 0, earning: true},
		{
			name: "step 1: the better paid of two", dp: 15, self: penal6,
			worth: map[int32]float64{toadNPC: 5, toadNPC + 2: 20},
			spots: []datapack.Spot{earnSpot("near", 1000), func() datapack.Spot {
				s := earnSpot("far", 3000)
				s.Monsters = []datapack.Mob{{NPCID: toadNPC + 2, Level: 5, Count: 5, Combat: giantToad}}
				return s
			}()},
			want: 1, earning: true,
		},
		{
			name: "step 2: none earns, the lowest monsters", dp: 15, self: penal6,
			spots: []datapack.Spot{wolfSpot("wolves 7", 1000, 7), wolfSpot("wolves 4", 3000, 4), wolfSpot("wolves 12", 2000, 12)},
			want:  1, earning: true,
		},
		{
			name: "step 2: the werewolves kill it, the lowest", dp: 15, self: penal6, worth: map[int32]float64{toadNPC + 1: 1000},
			spots: []datapack.Spot{wolfSpot("wolves 7", 1000, 7), wolfSpot("wolves 4", 3000, 4)},
			want:  1, earning: true,
		},
		{
			name: "an avoided spot is not claimed", dp: 15, self: penal6, worth: rich,
			spots: []datapack.Spot{earnSpot("toads", 2000), wolfSpot("wolves 4", 3000, 4)},
			edit:  func(b *Brain) { b.spot.avoid = map[int]time.Time{0: at(time.Hour)} },
			want:  1, earning: true,
		},
		{
			name: "full earning spots: none", dp: 15, self: penal6, worth: rich,
			spots: []datapack.Spot{earnSpot("toads", 2000)},
			edit: func(b *Brain) {
				for {
					if _, ok := b.cfg.Spots.claimN([]int{0}, 1); !ok {
						break
					}
				}
			},
			want: -1,
		},
		{name: "no penalty: as before, none", dp: 0, self: penal6, worth: rich, spots: []datapack.Spot{earnSpot("toads", 2000)}, want: -1},
		{name: "a spot fits: as before", dp: 15, self: squire6, worth: rich, spots: []datapack.Spot{earnSpot("toads", 2000)}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newEarnBrain(t, tt.self, tt.dp, tt.worth, tt.spots...)
			if tt.edit != nil {
				tt.edit(b)
			}
			b.decide(at(time.Second))
			v := &b.spot
			if tt.want < 0 {
				if v.farming() {
					t.Fatalf("spot %d held %v beside %v, want none", v.idx, v.held, v.beside)
				}
				return
			}
			if !v.held || v.idx != tt.want || v.earning != tt.earning {
				t.Fatalf("spot %d held %v earning %v, want %d earning %v", v.idx, v.held, v.earning, tt.want, tt.earning)
			}
		})
	}
}

// TestEarnFarms checks that a bot at its earning spot fights its monsters
// (rule "farm").
func TestEarnFarms(t *testing.T) {
	b := newEarnBrain(t, penal6, 15, map[int32]float64{toadNPC: 20}, earnSpot("toads", 2000))
	b.decide(at(time.Second))
	if !b.spot.held || !b.spot.earning {
		t.Fatalf("spot held %v earning %v, want an earning spot", b.spot.held, b.spot.earning)
	}
	b.spot.arrived = true
	b.world.Self.move = stopAt(b.spot.at)
	b.world.Apply(NPCInfo{ObjectID: mob, NPCID: toadNPC, Attackable: true, Pos: Point{X: b.spot.at.X + 300, Y: b.spot.at.Y, Z: b.spot.at.Z}}, at(time.Second))
	a, ok := b.ruleFarm(at(2 * time.Second))
	if !ok || a.object != mob {
		t.Fatalf("ruleFarm = %v (action %d on %#x), want the toad", ok, a.kind, a.object)
	}
}

// TestEarnEnds checks the earning mode's end: a bot farming an earning spot
// keeps it while its penalty lasts and nothing fits, claims the spot that
// fits once a Scroll: Recovery lifts enough of its penalty, and holds its
// spot as an ordinary one once the penalty is gone. Its supply trip for
// the scroll (penaltyTrip) holds at an earning spot as at any other.
func TestEarnEnds(t *testing.T) {
	spots := []datapack.Spot{wolfSpot("wolves 4", 1000, 4), earnSpot("toads", 3000)}
	b := newEarnBrain(t, penal6, 15, nil, spots...)
	b.decide(at(time.Second))
	if !b.spot.held || b.spot.idx != 0 || !b.spot.earning {
		t.Fatalf("spot %d held %v earning %v, want the werewolves earning", b.spot.idx, b.spot.held, b.spot.earning)
	}
	b.decide(at(time.Second + spotRetry))
	if !b.spot.held || b.spot.idx != 0 || !b.spot.earning {
		t.Fatalf("after spotRetry: spot %d held %v earning %v, want the werewolves kept", b.spot.idx, b.spot.held, b.spot.earning)
	}
	// A scroll read: penalty 14, and stats that fit the toads.
	b.world.Apply(SystemMessage{ID: msgDeathPenaltyLevelAdded, Number: 14}, at(time.Minute))
	setSquire(b, squire6, 0)
	b.decide(at(time.Second + 2*spotRetry))
	if !b.spot.held || b.spot.idx != 1 || b.spot.earning {
		t.Fatalf("after a read: spot %d held %v earning %v, want the toads, no earning", b.spot.idx, b.spot.held, b.spot.earning)
	}

	b = newEarnBrain(t, penal6, 15, map[int32]float64{toadNPC: 20}, earnSpot("toads", 3000))
	b.decide(at(time.Second))
	b.world.Apply(SystemMessage{ID: msgDeathPenaltyLifted}, at(time.Minute))
	b.decide(at(time.Second + spotRetry))
	if !b.spot.held || b.spot.idx != 0 || b.spot.earning {
		t.Fatalf("penalty lifted: spot %d held %v earning %v, want the toads kept, no earning", b.spot.idx, b.spot.held, b.spot.earning)
	}

	b = newPenaltyTripBrain(t, 15, recoveryPrice)
	b.spot.earning = true
	if got := b.supplyCause(); got != causePenalty {
		t.Fatalf("supplyCause at an earning spot = %d, want causePenalty", got)
	}
}

// TestIndexKillValues checks a kill's worth: adena at face value, a DROP
// item at half its reference price, each by its chances and mean count;
// SPOIL and HERB count nothing.
func TestIndexKillValues(t *testing.T) {
	npcs := map[int32]datapack.NPC{
		1: {Drops: []datapack.DropCategory{
			{Kind: datapack.DropKindCurrency, Chance: 70, Drops: []datapack.Drop{{ItemID: adenaID, Min: 10, Max: 20, Chance: 100}}},
			{Kind: datapack.DropKindDrop, Chance: 50, Drops: []datapack.Drop{{ItemID: 1864, Min: 1, Max: 3, Chance: 40}}},
			{Kind: datapack.DropKindSpoil, Chance: 100, Drops: []datapack.Drop{{ItemID: 1864, Min: 5, Max: 5, Chance: 100}}},
			{Kind: datapack.DropKindHerb, Chance: 100, Drops: []datapack.Drop{{ItemID: 8600, Min: 1, Max: 1, Chance: 100}}},
		}},
		2: {Drops: []datapack.DropCategory{{Kind: datapack.DropKindSpoil, Chance: 100, Drops: []datapack.Drop{{ItemID: 1864, Min: 1, Max: 1, Chance: 100}}}}},
	}
	got := indexKillValues(npcs, map[int32]int32{1864: 41, 8600: 100})
	// 0.7 × 15 adena + 0.5 × 0.4 × 2 × 20 (41 / 2).
	if want := map[int32]float64{1: 0.7*15 + 0.5*0.4*2*20}; len(got) != 1 || math.Abs(got[1]-want[1]) > 1e-9 {
		t.Fatalf("indexKillValues = %v, want %v", got, want)
	}
}

// TestDropShare checks the server's level gap cut of a kill's drops.
func TestDropShare(t *testing.T) {
	tests := []struct {
		killer, mob int
		want        float64
	}{{6, 5, 1}, {10, 5, 1}, {11, 5, 0.82}, {15, 5, 0.1}, {40, 5, 0.1}}
	for _, tt := range tests {
		if got := dropShare(tt.killer, tt.mob); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("dropShare(%d, %d) = %v, want %v", tt.killer, tt.mob, got, tt.want)
		}
	}
}

// hz30 is the bot of the home run of 2026-11-25 (#527): a level 5 Orc
// Mystic (class 49) with death penalty 15 and about 1,000 adena, wearing
// templates hz30Worn, at hz30At.
var (
	hz30Worn = []int32{425, 461, 2368, 118, 112, 112, 116, 116}
	hz30At   = Point{X: -42666, Y: -108339, Z: -1685}
)

// TestEarnShipped checks hz30 against the real datapack: no spot in reach
// fits it by its model with its penalty (the cause of #527), and the
// earning mode orders every spot in reach for it by level (step 2), the
// lowest first. By the model it kills nothing in time with its penalty
// (a melee of P.Atk 1: about 700 s a level 2 monster, which kills it in
// 9 s), so no spot earns it anything by rate (step 1); it logs those that
// do.
// The model's stats are the class's at level 5 in its gear with the Death
// Penalty's level 15 functions (skill 5076); the walk is the straight
// line, as no navigation is loaded.
func TestEarnShipped(t *testing.T) {
	dir := os.Getenv("BOT_DATAPACK_XML")
	if dir == "" {
		t.Skip("BOT_DATAPACK_XML not set")
	}
	dp, err := datapack.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := newSpotBrain(t, 5, dp.Spots()...)
	b.cfg.Travel = NewTravelMap(dp, DefaultSupplyConfig())
	b.persona.Role = RoleDamage
	s := &b.world.Self
	s.ClassID = 49
	var worn []datapack.Item
	for _, id := range hz30Worn {
		if it, ok := dp.Items[id]; ok {
			worn = append(worn, it)
		}
	}
	penalty := dp.Skills[datapack.SkillRef{ID: 5076, Level: 15}].Funcs
	if len(penalty) == 0 {
		t.Fatal("no Death Penalty level 15 functions")
	}
	p := charmodel.Model(charmodel.Build{Class: dp.Classes[49], Level: 5, Mage: true, Worn: worn, Funcs: penalty})
	s.MaxHP, s.HP, s.MaxMP, s.MP = int32(p.MaxHP), int32(p.MaxHP), int32(p.MaxMP), int32(p.MaxMP)
	s.PAtk, s.MAtk, s.PAtkSpd, s.CastSpd = int32(p.PAtk), int32(p.MAtk), int32(p.PAtkSpd), int32(p.MAtkSpd)
	s.PDef, s.MDef, s.Accuracy, s.Evasion, s.Critical = int32(p.PDef), int32(p.MDef), int32(p.Accuracy), int32(p.Evasion), int32(p.Critical)
	s.RunSpeed = int32(p.RunSpeed)
	b.world.Apply(EtcStatusUpdate{DeathPenalty: 15}, t0)
	b.world.Apply(ItemList{Items: []Item{adenaStack(1000)}}, t0)
	b.world.Skills = map[int32]KnownSkill{}
	for _, st := range b.build().Kit.Rotation {
		b.world.Skills[st.Skill] = KnownSkill{ID: st.Skill, Level: 1}
	}
	now := at(time.Second)
	wander := float64(b.persona.Spots.Wander)
	var cands []spotCand
	fits := 0
	for i := range b.cfg.Spots.spots {
		sp := &b.cfg.Spots.spots[i]
		line := math.Sqrt(float64(horizontal2(hz30At, Point{X: sp.Center.X, Y: sp.Center.Y})))
		if line > wander {
			continue
		}
		cands = append(cands, spotCand{i: i, d: line, line: line})
		if b.spotFits(sp, now) || b.lowerFits(sp, now) {
			fits++
			t.Logf("fits: %s (levels %d-%d)", sp.Maker, sp.MinLevel, sp.MaxLevel)
		}
	}
	t.Logf("stats: HP %d P.Atk %d M.Atk %d P.Def %d M.Def %d cast %d; %d spots in reach", s.MaxHP, s.PAtk, s.MAtk, s.PDef, s.MDef, s.CastSpd, len(cands))
	rich, low := b.earnOrder(cands, now)
	for k, i := range rich[:min(5, len(rich))] {
		sp := &b.cfg.Spots.spots[i]
		rate, _ := b.earnRate(sp)
		t.Logf("by rate %d: %s (levels %d-%d) %.0f adena an hour", k, sp.Maker, sp.MinLevel, sp.MaxLevel, rate)
	}
	if fits > 0 {
		t.Fatalf("%d spots fit hz30 by its model: #527 has no cause", fits)
	}
	if len(low) != len(cands) {
		t.Fatalf("%d spots by level, want all %d in reach", len(low), len(cands))
	}
	first := &b.cfg.Spots.spots[low[0]]
	for _, c := range cands {
		if sp := &b.cfg.Spots.spots[c.i]; sp.MaxLevel < first.MaxLevel {
			t.Fatalf("by level: %s (up to %d) first, %s (up to %d) lower", first.Maker, first.MaxLevel, sp.Maker, sp.MaxLevel)
		}
	}
	t.Logf("%d spots by rate; by level: %s (levels %d-%d) first", len(rich), first.Maker, first.MinLevel, first.MaxLevel)
}
