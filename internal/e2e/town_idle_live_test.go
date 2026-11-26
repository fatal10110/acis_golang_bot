package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// townIdleMoves is how many moves of the bot the server reports once it is
// in town before the scenario judges where its drift stays: enough drifts
// that one heading back to the login point would show.
const townIdleMoves = 6

// TestLiveTownIdle logs a town bot (TownPersona and the travel map with no
// spot registry, so it never rotates to farming, #618) in out in the field,
// on recoverGround about 3600 from Talking Island village's gatekeeper, the
// travel map's town point there (#567). Its idle drift walks it to the
// village, and every move the server reports for it afterwards ends near
// that town point: it drifts around the town, never back to where it
// logged in. No adena, no restock, no shots and an hour's idle spell: no
// errand or hop walks it anywhere.
func TestLiveTownIdle(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	persona := botclient.TownPersona()
	// Persona defaults (D6), with the errands that walk a bot off: no
	// purchase, no hop within the run.
	persona.Shop.Restock = nil
	persona.Shop.Soulshots, persona.Shop.Spiritshots = botclient.ShotStock{}, botclient.ShotStock{}
	persona.Town.HopMin, persona.Town.HopMax = time.Hour, time.Hour
	radius := float64(persona.IdleRadius)

	var town botclient.Point
	for i, p := range travel.Towns() {
		if i == 0 || dist(recoverGround, p) < dist(recoverGround, town) {
			town = p
		}
	}
	if d := dist(town, townTI); d > 1000 {
		t.Fatalf("the town nearest %v is %v, %.0f from Talking Island village", recoverGround, town, d)
	}
	if d := dist(recoverGround, town); d < 4*radius {
		t.Fatalf("login point %v only %.0f from the town point %v", recoverGround, d, town)
	}

	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("ti", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{at: recoverGround}, squiresSword, squiresShirt, squiresPants)
	f := runWatched(t, ctx, tg, account, botSetup{
		class:   &botclient.HumanFighter,
		persona: func() botclient.Persona { return persona },
		brain:   withTravel(travel),
	})
	t.Logf("%s logs in at %v, %.0f from the town point %v", f.name, recoverGround, dist(recoverGround, town), town)

	var arrived int
	until(t, ctx, "the walk to town", func() string {
		moves := f.movesSince(0)
		if n := len(moves); n > 0 && dist(moves[n-1], town) <= 2*radius {
			arrived = f.seen()
			return ""
		}
		return "a move ending near the town point"
	})
	t.Logf("%s in town after %d moves", f.name, len(f.movesSince(0)))
	until(t, ctx, "the drift in town", func() string {
		if len(f.movesSince(arrived)) < townIdleMoves {
			return "more drift moves"
		}
		return ""
	})
	for i, p := range f.movesSince(arrived) {
		if d := dist(p, town); d > 2*radius {
			t.Errorf("drift move %d in town ends at %v, %.0f from the town point %v", i, p, d, town)
		}
	}
}

// movesSince are the ends of f's own moves the server reported since index
// from: a move's destination, a stop's or a correction's position.
func (f *farmer) movesSince(from int) []botclient.Point {
	var at []botclient.Point
	for _, s := range f.watch.Seen(from) {
		switch e := s.Event.(type) {
		case botclient.MoveToLocation:
			if e.ObjectID == f.id && e.Origin != e.Dest {
				at = append(at, e.Dest)
			}
		case botclient.StopMove:
			if e.ObjectID == f.id {
				at = append(at, e.Pos)
			}
		}
	}
	return at
}
