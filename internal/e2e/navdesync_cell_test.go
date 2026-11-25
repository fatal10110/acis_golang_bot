package e2e

import (
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/geo/geodata"
)

// TestDesyncTalkStandCell replays, on the real geodata, the frames before
// the desync of #434: a squire on Talking Island clicks the newbie guide
// from out of reach (MoveToPawn, the approach), the server stops it in reach
// and faces the guide from where it holds it (MoveToPawn with the
// interaction distance, targeting.go thinkInteract), and the bot then walks
// toward the newbie spots. The bot must stand where the server holds it, so
// its check of that segment reads the line the server's tier-1 CanMove reads
// (move/creature.go resolvePathLocked, the head-level target of
// network/movement.go): closed here, which the server routed. The bot's own
// estimate of the approach's end, a cell off, opened it: the desync. It
// needs the geodata of a live run (BOT_E2E_GEODATA, BOT_E2E_GEOENGINE,
// BOT_E2E_GEODATA_CACHE) and skips without it.
func TestDesyncTalkStandCell(t *testing.T) {
	files := map[string]string{}
	for _, env := range []string{EnvGeodata, EnvGeoengine, EnvGeodataCache} {
		p := os.Getenv(env)
		if _, err := os.Stat(p); p == "" || err != nil {
			t.Skipf("%s unset or missing: the real geodata is a live run's", env)
		}
		files[env] = p
	}
	settings, err := geodata.LoadSettings(files[EnvGeoengine])
	if err != nil {
		t.Fatal(err)
	}
	c, err := geodata.Open(files[EnvGeodata], settings.GeoType, settings.Regions, files[EnvGeodataCache], settings.Options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)

	const self, guide, collision = 268494896, 268437312, 23
	var (
		stand  = botclient.Point{X: -84008, Y: 243016, Z: -3728} // the end of the walk to the guide
		guided = botclient.Point{X: -84081, Y: 243227, Z: -3723} // the newbie guide (spawnlist 17_25.xml, npc 30598)
		server = botclient.Point{X: -84052, Y: 243144, Z: -3728} // where the server stopped the squire in reach
		spot   = botclient.Point{X: -86024, Y: 244424, Z: -3728} // the next click: toward the newbie spots
	)
	t0 := time.Date(2026, 11, 25, 11, 14, 35, 555_000_000, time.UTC)
	w, err := botclient.NewWorld(botclient.Self{ObjectID: self, Name: "FisuBot", X: stand.X, Y: stand.Y, Z: stand.Z}, t0)
	if err != nil {
		t.Fatal(err)
	}
	w.SetGround(c.Engine)
	w.Apply(botclient.UserInfo{ObjectID: self, Name: "FisuBot", RunSpeed: 126, WalkSpeed: 88, Running: true, HP: 520, MaxHP: 520, CollisionHeight: collision}, t0)
	w.Apply(botclient.NPCInfo{ObjectID: guide, NPCID: 30598, Pos: guided}, t0)
	w.Apply(botclient.MoveToPawn{ObjectID: self, TargetID: guide, Distance: 100, Origin: stand}, t0)
	faced := t0.Add(967 * time.Millisecond)
	w.Apply(botclient.MoveToPawn{ObjectID: self, TargetID: guide, Distance: 150, Origin: server}, faced)

	now := t0.Add(3194 * time.Millisecond) // the click toward the spot
	from := w.Self.Position(now)
	if from != server {
		t.Fatalf("the bot stands at %v, want the server's %v", from, server)
	}
	head := spot
	head.Z += collision
	if c.Engine.CanMove(int(from.X), int(from.Y), int(from.Z), int(head.X), int(head.Y), int(head.Z)) {
		t.Errorf("CanMove(%v, %v) open, want closed as the server's tier-1 check (it routed toward {-84808 243624 -3728})", from, head)
	}
}
