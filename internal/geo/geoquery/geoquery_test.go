package geoquery_test

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/geo/block"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geoquery"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
	"github.com/fatal10110/acis_golang_bot/internal/geo/location"
	"github.com/fatal10110/acis_golang_bot/internal/geo/reader"
)

func scenes(tb testing.TB) *engine.Engine {
	tb.Helper()
	path, err := geotest.Scenes().WriteL2OFF(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	decoded, _, err := reader.ReadL2OFF(path)
	if err != nil {
		tb.Fatal(err)
	}
	var stored bytes.Buffer
	if _, err := decoded.WriteTo(&stored); err != nil {
		tb.Fatal(err)
	}
	region, err := block.ParseRegion(stored.Bytes())
	if err != nil {
		tb.Fatal(err)
	}
	e := engine.New()
	if err := e.SetRegion(geotest.RegionX, geotest.RegionY, region); err != nil {
		tb.Fatal(err)
	}
	return e
}

func at(p geotest.Point, dx, dy, dz int) location.Location {
	x, y, z := p.World()
	return location.Location{X: x + dx, Y: y + dy, Z: z + dz}
}

// TestQueryID pins the ids to geoprobe's: probe.ParseQuery reads them back.
func TestQueryID(t *testing.T) {
	tests := []struct {
		q    geoquery.Query
		want string
	}{
		{geoquery.Query{Category: geoquery.Height, From: location.Location{X: -1, Y: 2, Z: -3}}, "height:-1,2,-3"},
		{geoquery.Query{Category: geoquery.CanMove, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: 4, Y: 5, Z: 6}}, "canmove:1,2,3->4,5,6"},
		{geoquery.Query{Category: geoquery.LineOfSight, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: 4, Y: 5, Z: 6}}, "los:1,2,3->4,5,6"},
		{geoquery.Query{Category: geoquery.ValidLocation, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: 4, Y: 5, Z: 6}}, "validlocation:1,2,3->4,5,6"},
	}
	for _, tt := range tests {
		if got := tt.q.ID(); got != tt.want {
			t.Errorf("ID() = %q, want %q", got, tt.want)
		}
	}
}

// TestEvaluate pins the answers and field names on the fixture scenes,
// among them the multilayer rule a click's head-level Z goes through: the
// nearest layer wins, and of two equally near layers the lower one.
func TestEvaluate(t *testing.T) {
	e := scenes(t)
	deck := geotest.BridgeWestDeck
	east := 3 * 16
	tests := []struct {
		name string
		q    geoquery.Query
		want map[string]string
	}{
		{"deck at deck level", geoquery.Query{Category: geoquery.Height, From: at(deck, 0, 0, 0)}, map[string]string{"height": "160"}},
		{"deck at head level", geoquery.Query{Category: geoquery.Height, From: at(deck, 0, 0, 24)}, map[string]string{"height": "160"}},
		{"ground under the deck at head level", geoquery.Query{Category: geoquery.Height, From: at(geotest.BridgeWestGround, 0, 0, 24)}, map[string]string{"height": "0"}},
		{"midpoint tie takes the lower layer", geoquery.Query{Category: geoquery.Height, From: at(geotest.BridgeWestGround, 0, 0, 80)}, map[string]string{"height": "0"}},
		{"one above the midpoint takes the upper layer", geoquery.Query{Category: geoquery.Height, From: at(geotest.BridgeWestGround, 0, 0, 81)}, map[string]string{"height": "160"}},
		{"drop off the ledge", geoquery.Query{Category: geoquery.CanMove, From: at(geotest.LedgeHigh, 0, 0, 0), To: at(geotest.LedgeLow, 0, 0, 0)}, map[string]string{"result": "true"}},
		{"climb the ledge", geoquery.Query{Category: geoquery.CanMove, From: at(geotest.LedgeLow, 0, 0, 0), To: at(geotest.LedgeHigh, 0, 0, 0)}, map[string]string{"result": "false"}},
		{"through the U wall", geoquery.Query{Category: geoquery.CanMove, From: at(geotest.UInside, 0, 0, 0), To: at(geotest.UBehind, 0, 0, 0)}, map[string]string{"result": "false"}},
		{"los on open ground", geoquery.Query{Category: geoquery.LineOfSight, From: at(geotest.AboveCorridor, 0, 0, 0), To: at(geotest.AboveCorridor, east, 0, 0)}, map[string]string{"result": "true"}},
		{
			"valid location on open ground",
			geoquery.Query{Category: geoquery.ValidLocation, From: at(geotest.CorridorWest, 0, 0, 0), To: at(geotest.CorridorWest, east, 0, 0)},
			map[string]string{"x": strconv.Itoa(at(geotest.CorridorWest, east, 0, 0).X), "y": strconv.Itoa(at(geotest.CorridorWest, 0, 0, 0).Y), "z": "0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := geoquery.Evaluate(e, tt.q)
			if r.ID != tt.q.ID() {
				t.Errorf("ID = %q, want %q", r.ID, tt.q.ID())
			}
			if !maps.Equal(r.Fields, tt.want) {
				t.Errorf("Fields = %v, want %v", r.Fields, tt.want)
			}
		})
	}
}

func TestWriteDump(t *testing.T) {
	var out bytes.Buffer
	records := []geoquery.Record{
		{ID: "los:1,1,1->2,2,2", Fields: map[string]string{"result": "true"}},
		{ID: "height:1,1,1", Fields: map[string]string{"height": "-5"}},
		{ID: "validlocation:1,1,1->2,2,2", Fields: map[string]string{"z": "3", "x": "1", "y": "2"}},
	}
	if err := geoquery.WriteDump(&out, records); err != nil {
		t.Fatal(err)
	}
	want := "height:1,1,1\theight=-5\nlos:1,1,1->2,2,2\tresult=true\nvalidlocation:1,1,1->2,2,2\tx=1\ty=2\tz=3\n"
	if out.String() != want {
		t.Errorf("dump = %q, want %q", out.String(), want)
	}
	for _, bad := range []geoquery.Record{
		{ID: "height:1,1,1\t", Fields: nil},
		{ID: "height:1,1,1", Fields: map[string]string{"a=b": "1"}},
		{ID: "height:1,1,1", Fields: map[string]string{"height": "1\n"}},
	} {
		if err := geoquery.WriteDump(&bytes.Buffer{}, []geoquery.Record{bad}); err == nil {
			t.Errorf("WriteDump(%q) = nil error, want one", bad)
		}
	}
}

// TestCurate checks the curated set on the fixture scenes: every kind is
// found, ids are unique and sorted, the same seed gives the same set, and
// the multilayer sites ask the bridge's heights around its midpoint.
func TestCurate(t *testing.T) {
	e := scenes(t)
	regions := []geodata.Region{{X: geotest.RegionX, Y: geotest.RegionY}}
	set := geoquery.Curate(e, regions, 7, 6)
	for _, k := range geoquery.Kinds {
		if set.Sites[k] != 6 {
			t.Errorf("Sites[%s] = %d, want 6", k, set.Sites[k])
		}
	}
	ids := make([]string, len(set.Queries))
	cats := make(map[geoquery.Category]int)
	for i, q := range set.Queries {
		ids[i] = q.ID()
		cats[q.Category]++
		rx, ry := engine.RegionOf(q.From.X, q.From.Y)
		if rx != geotest.RegionX || ry != geotest.RegionY {
			t.Errorf("%s: from outside the listed region", ids[i])
		}
	}
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Error("query ids are not sorted and unique")
	}
	for _, c := range []geoquery.Category{geoquery.Height, geoquery.CanMove, geoquery.LineOfSight, geoquery.ValidLocation} {
		if cats[c] == 0 {
			t.Errorf("no %s query", c)
		}
	}
	again := geoquery.Curate(e, regions, 7, 6)
	if !slices.Equal(set.Queries, again.Queries) {
		t.Error("the same seed gave a different set")
	}
	if other := geoquery.Curate(e, regions, 8, 6); slices.Equal(set.Queries, other.Queries) {
		t.Error("another seed gave the same set")
	}
	var mid bool
	for _, id := range ids {
		if strings.HasPrefix(id, "height:") && strings.HasSuffix(id, ",80") {
			mid = true
		}
	}
	if !mid {
		t.Error("no multilayer height at the bridge's midpoint Z 80")
	}
	if empty := geoquery.Curate(e, nil, 7, 6); len(empty.Queries) != 0 {
		t.Errorf("no regions gave %d queries", len(empty.Queries))
	}
}
