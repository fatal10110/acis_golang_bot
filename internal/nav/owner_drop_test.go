package nav

import (
	"fmt"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo"
	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
)

// probeTown is one town point of the drop probe.
type probeTown struct {
	name string
	at   location.Location
}

// acceptedDrop is a drop past dropBound the place routes keep: those within
// 64 of at, of at most max (docs/navigation.md, "Drops").
type acceptedDrop struct {
	at  location.Location
	max int
	why string
}

// acceptedDrops are the drops the town probe accepts on the owner's data:
// none since the crossings are costed by their cell paths (#575).
var acceptedDrops []acceptedDrop

// TestOwnerTownDrops is the offline probe of #504 and #514: the place routes
// from every town point to every spot within 20000 of it, each with its
// largest single-cell descent. The town points are the home run's
// (TravelMap.Towns: the gatekeepers' fixed spawns, merged within 3000); the
// Orc village's, Tamil, is the point of #504. It runs on the owner's data only
// (ownerNav's variables, and BOT_NAV_DATAPACK for the towns and spots), and
// fails when a place route drops past dropBound anywhere but at an accepted
// drop (docs/navigation.md, "Drops").
func TestOwnerTownDrops(t *testing.T) {
	xmlDir := os.Getenv("BOT_NAV_DATAPACK")
	if xmlDir == "" {
		t.Skip("BOT_NAV_DATAPACK not set: the owner-geodata probe runs locally only")
	}
	e, g, w := ownerNav(t)
	dp, err := datapack.Load(xmlDir)
	if err != nil {
		t.Fatal(err)
	}
	towns := gatekeeperTowns(dp)
	h := NewHierarchy(g, e, w, 1<<30)
	p := &Planner{Engine: e, Lmax: testLmax, CollisionHeight: 23, Budget: 400000, Water: w}
	spots := dp.Spots()
	total, kept := 0, 0
	for _, town := range towns {
		routes, routed, over, accepted, worst := 0, 0, 0, 0, 0
		var worstAt Cell
		for _, s := range spots {
			to := location.Location{X: int(s.Center.X), Y: int(s.Center.Y), Z: int(s.Z)}
			if math.Hypot(float64(to.X-town.at.X), float64(to.Y-town.at.Y)) > 20000 {
				continue
			}
			routes++
			r, err := h.Route(p, town.at, to, nil)
			if err != nil {
				t.Logf("%s -> %s %v: %v", town.name, s.Maker, to, err)
				continue
			}
			routed++
			d, at := routeDescent(e, r)
			if d > worst {
				worst, worstAt = d, at
			}
			switch drops := routeDrops(e, r); {
			case len(drops) == 0:
			case !slices.ContainsFunc(drops, unaccepted):
				accepted++
			default:
				over++
				t.Logf("%s -> %s %v: DROP %v; route %v", town.name, s.Maker, to, drops, r)
			}
		}
		total += over
		kept += accepted
		t.Logf("town %s %v: %d routes, %d routed, %d past %d (%d accepted), worst descent %d at %v",
			town.name, town.at, routes, routed, over+accepted, dropBound, accepted, worst, worstAt.World())
	}
	for _, a := range acceptedDrops {
		t.Logf("accepted: drops up to %d within 64 of %v: %s", a.max, a.at, a.why)
	}
	t.Logf("%d towns, %d place routes with a descent past %d, %d of them accepted", len(towns), total+kept, dropBound, kept)
	if total > 0 {
		t.Errorf("%d place routes drop past %d", total, dropBound)
	}
}

// routeDrop is one step of a route's walk landing more than dropBound below
// the floor before it.
type routeDrop struct {
	descent int
	at      location.Location
}

// routeDrops are the drops of the walk along route, read as routeDescent
// reads it.
func routeDrops(e *engine.Engine, route []location.Location) []routeDrop {
	var drops []routeDrop
	var cells []lineCell
	for i := 1; i < len(route); i++ {
		cells = appendLineCells(cells[:0], route[i-1], route[i])
		z := route[i-1].Z
		for _, c := range cells[1:] {
			h, _, ok := e.NodeBelow(c.x, c.y, z+block.CellIgnoreHeight)
			if !ok {
				continue
			}
			if d := z - int(h); d > dropBound {
				drops = append(drops, routeDrop{d, Cell{X: c.x, Y: c.y, Z: h}.World()})
			}
			z = int(h)
		}
	}
	return drops
}

// unaccepted reports whether d is none of acceptedDrops.
func unaccepted(d routeDrop) bool {
	return !slices.ContainsFunc(acceptedDrops, func(a acceptedDrop) bool {
		return d.descent <= a.max && math.Hypot(float64(d.at.X-a.at.X), float64(d.at.Y-a.at.Y)) <= 64
	})
}

// gatekeeperTowns are the town points of the home run, the way
// botclient.TravelMap.Towns builds them (a nav test cannot import
// botclient): the single-position, non-event spawns of every gatekeeper with
// a standard adena destination, in NPC id order, each within 3000 of an
// earlier one merged into it. Each is named by its restart region and its
// gatekeeper.
func gatekeeperTowns(dp *datapack.Datapack) []probeTown {
	ids := make([]int32, 0, len(dp.Teleports))
	for id := range dp.Teleports {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var towns []probeTown
	for _, id := range ids {
		if dp.NPCs[id].Type != "Gatekeeper" || !slices.ContainsFunc(dp.Teleports[id], func(t datapack.Teleport) bool {
			return t.Kind == datapack.TeleportStandard && t.PriceID == 57
		}) {
			continue
		}
		for _, mk := range dp.Spawnlist.Makers {
			if mk.Event != "" {
				continue
			}
			for _, sp := range mk.Spawns {
				if sp.NPCID != id || len(sp.Positions) != 1 {
					continue
				}
				at := location.Location{X: int(sp.Positions[0].X), Y: int(sp.Positions[0].Y), Z: int(sp.Positions[0].Z)}
				if !slices.ContainsFunc(towns, func(t probeTown) bool {
					return math.Hypot(float64(at.X-t.at.X), float64(at.Y-t.at.Y)) <= 3000
				}) {
					towns = append(towns, probeTown{name: fmt.Sprintf("%s %s", restartRegionName(dp, at), dp.NPCs[id].Name), at: at})
				}
			}
		}
	}
	return towns
}

// restartRegionName is the name of the restart point whose map tiles hold
// at, "?" for none.
func restartRegionName(dp *datapack.Datapack, at location.Location) string {
	tile := datapack.MapTile{
		X: (at.X-geo.WorldXMin)/geo.TileSize + geo.TileXMin,
		Y: (at.Y-geo.WorldYMin)/geo.TileSize + geo.TileYMin,
	}
	for _, p := range dp.RestartPoints {
		if slices.Contains(p.Tiles, tile) {
			return p.Name
		}
	}
	return "?"
}
