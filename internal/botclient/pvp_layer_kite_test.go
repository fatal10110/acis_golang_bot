package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// kiteBrain is layerBrain for a Human Mystic (a caster row) casting Dryad
// Root (cast range 600) and Wind Strike on a Human Fighter (melee) at to,
// in a duel from t0 when duel is set, with the town and arena zones.
func kiteBrain(t *testing.T, duel bool, to Point, towns, arenas []datapack.Zone) *Brain {
	t.Helper()
	b := layerBrain(t, 0, Step{Skill: grdRoot}, Step{Skill: skillWindStrike})
	b.world.Self.ClassID = lyrMystic
	withKit(b, b.built.Kit)
	m := b.cfg.Travel
	if m.castRanges == nil {
		m.castRanges = make(map[datapack.SkillRef]int32)
	}
	m.castRanges[datapack.SkillRef{ID: grdRoot, Level: 1}] = 600
	m.zones.Towns, m.zones.Arenas = towns, arenas
	if duel {
		duelUntil(t, b, DuelFighting, t0)
	}
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 0, Pos: to, RunSpeed: 124, Running: true, PvPFlag: 1, CollisionRadius: 9}, at(9*time.Second))
	return b
}

// rootLands plays the bot's root on pcID at to from 10 s: its cast, its
// MagicSkillUse (hit 1 s) and the CharInfo showing the root.
func rootLands(t *testing.T, b *Brain, to Point) {
	t.Helper()
	play(t, b, []beat{
		{at: 10 * time.Second, kind: actCast, skill: grdRoot},
		{at: 10*time.Second + 100*time.Millisecond, ev: selfCast(grdRoot, time.Second), noDecide: true},
		{at: 11*time.Second + 200*time.Millisecond, ev: CharInfo{ObjectID: pcID, Name: foeName, Pos: to, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: rootAbnormal, CollisionRadius: 9}, noDecide: true},
	})
}

// kiteBox is a cuboid zone of x and y bounds.
func kiteBox(x0, x1, y0, y1 int32) datapack.Zone {
	return datapack.Zone{Cuboid: true, X: []int32{x0, x1}, Y: []int32{y0, y1}, MinZ: -1000, MaxZ: 1000}
}

// TestPvPKite checks the kite (docs/pvp/layer.md "Kite"): a caster whose
// root landed on a melee opponent walks to a stand point at its cast range
// (less walkSlop), straight away first, then casts; in a duel the points
// on or within duelEdge of a town are skipped, the first passing one taken;
// in an arena those within arenaEdge of its edge; none passing, it casts
// in place; a duel opponent far out of range is no
// kite.
func TestPvPKite(t *testing.T) {
	foe := Point{Y: 300}
	tests := []struct {
		name          string
		duel          bool
		towns, arenas []datapack.Zone
		want          Point // the kite's stand point; zero for none
	}{
		{name: "straight away", want: Point{Y: 300 - 584}},
		{name: "duel: the first point clear of a town", duel: true, towns: []datapack.Zone{kiteBox(-300, 300, -1000, -250)}, want: Point{X: 413, Y: -113}},
		{name: "arena: the first point arenaEdge inside", arenas: []datapack.Zone{kiteBox(-2000, 2000, -200, 2000)}, want: Point{X: 540, Y: 77}},
		{name: "duel: no point passes", duel: true, towns: []datapack.Zone{
			kiteBox(-5000, 5000, 750, 5000), kiteBox(-5000, 5000, -5000, -150), kiteBox(450, 5000, -5000, 5000), kiteBox(-5000, -450, -5000, 5000),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := kiteBrain(t, tc.duel, foe, tc.towns, tc.arenas)
			rootLands(t, b, foe)
			now := at(11*time.Second + 500*time.Millisecond)
			_, a := b.decide(now)
			if tc.want == (Point{}) {
				if a.kind != actCast || a.skill != skillWindStrike {
					t.Fatalf("kind %v skill %d, want Wind Strike in place", a.kind, a.skill)
				}
				return
			}
			if got := moveDest(t, a); got != tc.want {
				t.Fatalf("kite to %v, want %v", got, tc.want)
			}
			if _, err := b.send(a, now); err != nil {
				t.Fatal(err)
			}
			b.world.Self.move = stopAt(tc.want)
			if got := castOf(t, b, at(13*time.Second)); got != skillWindStrike {
				t.Fatalf("cast %d at the stand point, want Wind Strike %d", got, skillWindStrike)
			}
		})
	}
	t.Run("duel opponent 1900 away", func(t *testing.T) {
		far := Point{Y: 1900}
		b := kiteBrain(t, true, far, nil, nil)
		b.layerCast(selfCast(grdRoot, time.Second), at(10*time.Second))
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: far, RunSpeed: 124, Running: true, PvPFlag: 1, Abnormal: rootAbnormal, CollisionRadius: 9}, at(11*time.Second+200*time.Millisecond))
		if _, ok := b.kiting(pcID, at(12*time.Second)); !ok {
			t.Fatal("the root does not count for the kite")
		}
		_, a := b.decide(at(12 * time.Second))
		if a.kind == actMove {
			t.Fatalf("kite move to %v, want the chase", moveDest(t, a))
		}
		if a.kind != actCast || a.skill != skillWindStrike {
			t.Fatalf("kind %v skill %d, want Wind Strike, whose cast the server walks to", a.kind, a.skill)
		}
	})
	t.Run("a fighter does not kite", func(t *testing.T) {
		b := kiteBrain(t, false, foe, nil, nil)
		b.world.Self.ClassID = 0
		withKit(b, b.built.Kit)
		rootLands(t, b, foe)
		if _, a := b.decide(at(11*time.Second + 500*time.Millisecond)); a.kind == actMove {
			t.Fatalf("a melee row kites to %v", moveDest(t, a))
		}
	})
}
