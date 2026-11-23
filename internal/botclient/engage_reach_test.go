package botclient

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// TestFightReach checks the fight's ranges on the synthetic nav: a bow
// shoots from its 2D range and a skill is cast from its 3D cast range when
// the target is in sight, with no walk though the straight line is closed;
// else the bot walks to a stand point sized from that range.
func TestFightReach(t *testing.T) {
	n := testNav(t)
	// ledgeSelf is on the low ground 448 from ledgeMob horizontally, 490
	// in 3D: the climb closes the straight line, the sight is open.
	// ledgeFar is 608 from it horizontally, 640 in 3D.
	ledgeSelf, ledgeFar := scenePoint(geotest.LedgeLow, -10, 0), scenePoint(geotest.LedgeLow, -20, 0)
	ledgeMob := scenePoint(geotest.LedgeHigh, 3, 0)
	uSelf, uMob := scenePoint(geotest.UInside, 0, 0), scenePoint(geotest.UBehind, 0, 0)
	tests := []struct {
		name      string
		self, mob Point
		bow       bool
		cast      bool
		// castRange is the cast's range in the travel map; unknown leaves
		// the skill out of it.
		castRange int32
		unknown   bool
		wantKind  actionKind
		wantWalk  bool
		// wantReach bounds the stand point's 3D distance from the mob, and
		// wantLine asks the straight line to it open (a melee stand point).
		wantReach float64
		wantLine  bool
	}{
		{
			name: "bow within its range, in sight: shoot without walking", self: ledgeSelf, mob: ledgeMob, bow: true,
			wantKind: actAttack,
		},
		{
			name: "bow out of its range: walk to a stand point within it", self: ledgeFar, mob: ledgeMob, bow: true,
			wantKind: actMove, wantWalk: true, wantReach: bowRange - standMargin,
		},
		{
			name: "spell within its 3D range, in sight: cast without walking", self: ledgeSelf, mob: ledgeMob, cast: true, castRange: 600,
			wantKind: actCast,
		},
		{
			name: "spell within its range horizontally only: walk", self: ledgeFar, mob: ledgeMob, cast: true, castRange: 620,
			wantKind: actMove, wantWalk: true, wantReach: 620 - standMargin,
		},
		{
			name: "skill the travel map lacks: the default reach", self: uSelf, mob: uMob, cast: true, unknown: true,
			wantKind: actMove, wantWalk: true, wantReach: rangedReach - standMargin,
		},
		{
			name: "short-range skill behind the U's base: a nearer stand point", self: uSelf, mob: uMob, cast: true, castRange: 150,
			wantKind: actMove, wantWalk: true, wantReach: 150 - standMargin,
		},
		{
			name: "touch skill behind the U's base: a melee stand point", self: uSelf, mob: uMob, cast: true, castRange: 40,
			wantKind: actMove, wantWalk: true, wantReach: meleeStand, wantLine: true,
		},
		{
			name: "no-approach skill behind the U's base: cast", self: uSelf, mob: uMob, cast: true, castRange: -1,
			wantKind: actCast,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Persona{BlacklistFor: time.Minute}
			if tt.cast {
				p.AttackSkills = []int32{engageCast}
			}
			b, _, _ := newNavBrain(t, n, p, tt.self)
			travel := &TravelMap{arrows: map[int32]int32{shortBow: 17}, castRanges: map[datapack.SkillRef]int32{}}
			b.cfg.Travel = travel
			if tt.cast {
				b.world.Skills = map[int32]KnownSkill{engageCast: {ID: engageCast, Level: 1}}
				if !tt.unknown {
					travel.castRanges[datapack.SkillRef{ID: engageCast, Level: 1}] = tt.castRange
				}
			}
			if tt.bow {
				b.world.Inventory.Items = map[int32]Item{wornShortBow.ObjectID: wornShortBow}
			}
			b.world.Apply(NPCInfo{ObjectID: engageMob, NPCID: 20001, Attackable: true, Pos: tt.mob}, t0)
			b.world.Target = engageMob
			now := t0.Add(time.Second)
			a := b.fight(engageMob, now)
			if a.kind != tt.wantKind || a.walk != tt.wantWalk {
				t.Fatalf("fight = kind %v walk %v (%s), want kind %v walk %v", a.kind, a.walk, b.walk.reason, tt.wantKind, tt.wantWalk)
			}
			if now.Before(b.blacklist[engageMob]) {
				t.Errorf("mob blacklisted")
			}
			if !tt.wantWalk {
				return
			}
			stand := b.engage.stand
			if d := distance3(stand, tt.mob); d > tt.wantReach {
				t.Errorf("stand point %+v %.0f from the mob, want within %.0f", stand, d, tt.wantReach)
			}
			if tt.wantLine && !b.walk.lineOpen(stand, tt.mob) {
				t.Errorf("melee stand point %+v has no open line to the mob", stand)
			}
			if b.walk.dest != stand {
				t.Errorf("walk to %+v, want the stand point %+v", b.walk.dest, stand)
			}
		})
	}
}

// distance3 is the 3D distance from a to b.
func distance3(a, b Point) float64 {
	dz := float64(b.Z - a.Z)
	return math.Sqrt(float64(horizontal2(a, b)) + dz*dz)
}

// TestReach checks the range test against the server's (strictly inside,
// a bow's 2D, a cast's 3D) and the stand point search each range takes.
func TestReach(t *testing.T) {
	bow, cast := reach{rng: 500, flat: true}, reach{rng: 500}
	holds := []struct {
		name     string
		r        reach
		to       Point
		wantHold bool
	}{
		{"bow ignores the height", bow, Point{X: 499, Z: 300}, true},
		{"cast counts the height", cast, Point{X: 499, Z: 300}, false},
		{"cast inside in 3D", cast, Point{X: 300, Y: 300, Z: 200}, true},
		{"bow: the boundary is out", bow, Point{X: 500}, false},
		{"cast: the boundary is out", cast, Point{X: 300, Y: 400}, false},
	}
	for _, tt := range holds {
		if got := tt.r.holds(Point{}, tt.to); got != tt.wantHold {
			t.Errorf("%s: holds = %v, want %v", tt.name, got, tt.wantHold)
		}
	}
	stands := []struct {
		r        reach
		wantMode nav.StandMode
		wantRng  float64
	}{
		{reach{rng: 900}, nav.StandSight, 900 - standMargin},
		{bow, nav.StandSight, bowRange - standMargin},
		{reach{rng: 150}, nav.StandSight, 150 - standMargin},
		{reach{rng: meleeStand + standMargin}, nav.StandLine, meleeStand},
		{reach{rng: 40}, nav.StandLine, meleeStand},
		{reach{}, nav.StandLine, meleeStand},
	}
	for _, tt := range stands {
		if mode, rng := tt.r.stand(); mode != tt.wantMode || rng != tt.wantRng {
			t.Errorf("reach %+v: stand = %v %v, want %v %v", tt.r, mode, rng, tt.wantMode, tt.wantRng)
		}
	}
}

func TestIndexCastRanges(t *testing.T) {
	got := indexCastRanges(map[datapack.SkillRef]datapack.Skill{
		{ID: 1177, Level: 1}: {Name: "Wind Strike", CastRange: 600},
		{ID: 4110, Level: 2}: {Name: "Breath Attack", CastRange: 1000},
		{ID: 1, Level: 1}:    {Name: "aura", CastRange: -1},
	})
	want := map[datapack.SkillRef]int32{{ID: 1177, Level: 1}: 600, {ID: 4110, Level: 2}: 1000, {ID: 1, Level: 1}: -1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("indexCastRanges = %v, want %v", got, want)
	}
}
