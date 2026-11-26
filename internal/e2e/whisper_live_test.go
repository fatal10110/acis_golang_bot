package e2e

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveWhisperCommands commands a bot by whisper. The GM, its one
// commander (the -commanders allowlist: the run has no database), whispers
// "invite me": the bot whispers it back and joins the GM's invitation, the
// GM leading; the GM's "follow" has it follow. A player who is no
// commander whispers "follow" and walks off: the bot keeps its slot by the
// GM.
// With the allowlist the player's whispers never reach a database lookup
// (Authority.ask refuses a non-commander at once) (#19).
func TestLiveWhisperCommands(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 6*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("wh", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: partyPersona, brain: commandedBy(gm)})
	b := f.bot
	name := botOnline(t, ctx, tg, b)
	acc, _ := tg.Account("wh", 2)
	other := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)
	gmToOpenGround(t, ctx, gm)
	recallBot(t, ctx, tg, gm, name)
	recallClient(t, ctx, gm, other)

	tell(t, ctx, gm.Client, name, "invite me")
	if _, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		s, ok := ev.(botclient.CreatureSay)
		return ok && s.Type == 2 && strings.EqualFold(s.Name, name) && s.Text == "invite me"
	}); err != nil {
		t.Fatalf("%s whispers \"invite me\" back: %v", name, err)
	}
	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)

	tell(t, ctx, gm.Client, name, "follow")
	ui := userInfo(t, gm.Client)
	speed := ui.RunSpeed
	if !ui.Running {
		speed = ui.WalkSpeed
	}
	l := lead(t, ctx, gm.Client, b, awayFrom(gm.Pos(), 600, 0), speed)
	at := settledNear(t, ctx, gm.Client, f, l.stop, name+" by the GM")
	t.Logf("%s settled at %v, %.0f from the GM's stop %v", name, at, dist(at, l.stop), l.stop)

	// The GM stands still from here: a bot that ignores the player keeps
	// its slot, one that obeyed would trail the player far off.
	for range 3 {
		tell(t, ctx, other, name, "follow")
		if err := other.Watch(ctx, 2*time.Second, func(botclient.Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	walkOff(t, ctx, other, gm.Pos(), offReach)
	if err := gm.Watch(ctx, 15*time.Second, func(botclient.Event) {}); err != nil {
		t.Fatal(err)
	}
	end := settledNear(t, ctx, gm.Client, f, gm.Pos(), name+" after a non-commander's follow")
	if d := dist(end, at); d > followRepath {
		t.Fatalf("%s moved %.0f from its slot by the GM after a non-commander's follow, want at most %d (%.0f from %s)", name, d, followRepath, dist(end, other.Pos()), other.Self.Name)
	}
	if d := dist(end, other.Pos()); d <= followReach {
		t.Fatalf("%s ended %.0f from %s, who is no commander, want over %d", name, d, other.Self.Name, followReach)
	}
}

// offReach is how far the non-commander walks from the GM: a bot that
// obeyed it would settle within followReach of it, over followReach plus
// Repath from any slot by the GM.
const offReach = 2*followReach + followRepath

// walkOff walks c until it stands at least d from from, in up to four
// moves (geodata may cut each short), each awaited to its arrival.
func walkOff(t *testing.T, ctx context.Context, c *Client, from botclient.Point, d float64) {
	t.Helper()
	ui := userInfo(t, c)
	speed := ui.RunSpeed
	if !ui.Running {
		speed = ui.WalkSpeed
	}
	for range 4 {
		left := d - dist(c.Pos(), from)
		if left <= 0 {
			return
		}
		a := math.Atan2(float64(c.Pos().Y-from.Y), float64(c.Pos().X-from.X))
		if c.Pos() == from {
			a = math.Pi
		}
		m, err := moveToward(ctx, c, awayFrom(c.Pos(), left+50, a))
		if err != nil {
			t.Fatal(err)
		}
		travel := time.Duration(dist(m.Origin, m.Dest) / float64(speed) * float64(time.Second))
		if err := c.Watch(ctx, travel+time.Second, func(botclient.Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	if got := dist(c.Pos(), from); got < d {
		t.Fatalf("%s walked %.0f off, geodata cut it short of %.0f", c.Self.Name, got, d)
	}
}
