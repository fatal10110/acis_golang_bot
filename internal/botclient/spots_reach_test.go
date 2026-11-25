package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// arriveAtSpot has b, a newSpotBrain at the origin, walk to its spot 0 at
// spot and arrive there at at(2s), returned.
func arriveAtSpot(t *testing.T, b *Brain, spot Point) time.Time {
	t.Helper()
	if rule, a := b.decide(at(time.Second)); rule != "spot" || a.kind != actMove {
		t.Fatalf("first decide = %q %d, want a move to the spot", rule, a.kind)
	}
	b.world.Self.move = stopAt(spot)
	arrive := at(2 * time.Second)
	b.decide(arrive)
	if !b.spot.arrived || !b.spot.held || b.spot.idx != 0 {
		t.Fatalf("spot %d held %v arrived %v, want spot 0 held and arrived", b.spot.idx, b.spot.held, b.spot.arrived)
	}
	return arrive
}

// TestSpotIdleMovesOn checks a farmer that finds nothing to fight at its
// spot for Persona.Spots.Idle moves on and avoids the spot for AvoidFor
// (#360): there, other players killed every monster first, and the bot
// idled for 24 minutes. Decisions come every second from the arrival but
// in a pause, where a rule ahead held the turn; a pause longer than idleGap
// starts the idle stretch over. A farm target before the timeout keeps the
// spot, and one in reach only briefly starts the stretch over from when it
// went.
func TestSpotIdleMovesOn(t *testing.T) {
	const mob = int32(0x20000031)
	tests := []struct {
		name string
		idle time.Duration
		// pauseFrom and pauseTo, after the arrival, bound a stretch with no
		// decision.
		pauseFrom, pauseTo time.Duration
		// mobAt, when not 0, is when a monster comes within the leash;
		// mobGone, when not 0, is when it leaves sight.
		mobAt, mobGone time.Duration
		// wantMoved is when, after the arrival, the bot moves on; 0 for
		// never within 30 minutes.
		wantMoved time.Duration
	}{
		{name: "nothing to fight: moves on", idle: 5 * time.Minute, wantMoved: 5 * time.Minute},
		{name: "a monster before the timeout keeps the spot", idle: 5 * time.Minute, mobAt: 4 * time.Minute},
		{
			name: "a monster in reach briefly starts it over", idle: 5 * time.Minute,
			mobAt: 4*time.Minute + 50*time.Second, mobGone: 5 * time.Minute, wantMoved: 10 * time.Minute,
		},
		{
			name: "a short pause goes on with the stretch", idle: 5 * time.Minute,
			pauseFrom: 2 * time.Minute, pauseTo: 2*time.Minute + idleGap, wantMoved: 5 * time.Minute,
		},
		{
			name: "a long pause starts it over", idle: 5 * time.Minute,
			pauseFrom: 3 * time.Minute, pauseTo: 5 * time.Minute, wantMoved: 10 * time.Minute,
		},
		{name: "Idle 0 never moves on", idle: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 3, testSpot("near", 400, 0, 1, 3, 9), testSpot("next", 0, 2000, 1, 3, 9))
			b.persona.Spots.Idle = tt.idle
			b.world.Objects = map[int32]*Object{}
			arrive := arriveAtSpot(t, b, Point{X: 400})
			var moved time.Duration
			for d := time.Second; d <= 30*time.Minute && moved == 0; d += time.Second {
				if d > tt.pauseFrom && d < tt.pauseTo {
					continue
				}
				now := arrive.Add(d)
				if tt.mobAt != 0 && d == tt.mobAt {
					b.world.Apply(NPCInfo{ObjectID: mob, NPCID: 20003, Attackable: true, Pos: Point{X: 600}}, now)
				}
				if tt.mobGone != 0 && d == tt.mobGone {
					b.world.Apply(DeleteObject{ObjectID: mob}, now)
				}
				inReach := tt.mobAt != 0 && d >= tt.mobAt && (tt.mobGone == 0 || d < tt.mobGone)
				rule, a := b.decide(now)
				if inReach && (rule != "farm" || a.object != mob) {
					t.Fatalf("at %v: rule %q object %d, want farming the monster %d", d, rule, a.object, mob)
				}
				if b.spot.held {
					continue
				}
				moved = d
				if rule != "farm" {
					t.Fatalf("moved on under rule %q, want farm", rule)
				}
				if got, want := b.spot.avoid[0], now.Add(b.persona.Spots.AvoidFor); !got.Equal(want) {
					t.Fatalf("spot avoided until %v, want %v", got, want)
				}
				if b.cfg.Spots.Held(0) != 0 {
					t.Fatalf("left spot still held %d", b.cfg.Spots.Held(0))
				}
				rule, a = b.decide(now.Add(time.Second))
				if rule != "spot" || a.kind != actMove || b.spot.idx != 1 {
					t.Fatalf("after moving on = %q %d spot %d, want a move to spot 1", rule, a.kind, b.spot.idx)
				}
			}
			if moved != tt.wantMoved {
				t.Fatalf("moved on %v after the arrival, want %v (0: never)", moved, tt.wantMoved)
			}
		})
	}
}

// wideSpot is a spot centred at (4000, 4000) whose territory, a 6000 square
// between heights -500 and 500, reaches far past the leash, as
// oren23_2018_04 does (#447); with territory false it has none.
func wideSpot(territory bool) datapack.Spot {
	sp := testSpot("wide", 4000, 4000, 1, 3, 9)
	if territory {
		sp.Territory = &datapack.Territory{Name: "wide", MinZ: -500, MaxZ: 500, Nodes: []datapack.Point{
			{X: 1000, Y: 1000}, {X: 7000, Y: 1000}, {X: 7000, Y: 7000}, {X: 1000, Y: 7000},
		}}
	}
	return sp
}

// TestSpotTerritoryReach checks a farmer at a spot whose territory is far
// wider than the leash takes the monsters anywhere in it as farm targets
// (#447): held at the centre of oren23_2018_04, the bot saw its monsters
// fought beyond the leash and made no kill in 12 minutes. A monster outside
// the territory and the leash, or above its height band, is not one; a spot
// with no territory keeps the leash alone.
func TestSpotTerritoryReach(t *testing.T) {
	const mob = int32(0x20000031)
	tests := []struct {
		name      string
		territory bool
		mobAt     Point
		want      bool
	}{
		{name: "inside the territory past the leash", territory: true, mobAt: Point{X: 6500, Y: 4000}, want: true},
		{name: "within the leash", territory: true, mobAt: Point{X: 5000, Y: 4000}, want: true},
		{name: "outside the territory past the leash", territory: true, mobAt: Point{X: 7500, Y: 4000}},
		{name: "inside it above its height band", territory: true, mobAt: Point{X: 6500, Y: 4000, Z: 1000}},
		{name: "no territory: within the leash", mobAt: Point{X: 5000, Y: 4000}, want: true},
		{name: "no territory: past the leash", mobAt: Point{X: 6500, Y: 4000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 3, wideSpot(tt.territory))
			b.world.Objects = map[int32]*Object{}
			now := arriveAtSpot(t, b, Point{X: 4000, Y: 4000}).Add(time.Second)
			b.world.Apply(NPCInfo{ObjectID: mob, NPCID: 20003, Attackable: true, Pos: tt.mobAt}, now)
			if got := b.farmTarget(now) == mob; got != tt.want {
				t.Fatalf("farm target %d, want the monster %v", b.farmTarget(now), tt.want)
			}
			rule, a := b.decide(now)
			if got := rule == "farm" && a.object == mob; got != tt.want {
				t.Fatalf("decide = %q %d object %d, want farming the monster %v", rule, a.kind, a.object, tt.want)
			}
		})
	}
}

// TestSpotTerritoryWalkBack checks a fight that ends past the leash but
// inside the spot's territory leaves the bot there (#447), while one that
// ends outside both walks it back to the spot.
func TestSpotTerritoryWalkBack(t *testing.T) {
	tests := []struct {
		name     string
		standAt  Point
		wantBack bool
	}{
		{name: "inside the territory", standAt: Point{X: 6500, Y: 4000}},
		{name: "outside it", standAt: Point{X: 7500, Y: 4000}, wantBack: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 3, wideSpot(true))
			b.world.Objects = map[int32]*Object{}
			now := arriveAtSpot(t, b, Point{X: 4000, Y: 4000}).Add(time.Second)
			b.world.Self.move = stopAt(tt.standAt)
			rule, a := b.decide(now)
			if back := rule == "spot" && a.kind == actMove; back != tt.wantBack {
				t.Fatalf("decide = %q %d, want walking back %v", rule, a.kind, tt.wantBack)
			}
		})
	}
}
