package e2e

import (
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
// commander whispers "follow" and walks off: the bot stays with the GM.
// With the allowlist the player's whispers never reach a database lookup
// (Authority.ask refuses a non-commander at once) (#19).
func TestLiveWhisperCommands(t *testing.T) {
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 6*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("wh", 1)
	b := runBotWith(t, ctx, tg, account, botSetup{persona: partyPersona, brain: commandedBy(gm)})
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
	at := settled(t, ctx, gm.Client, b)
	if d := dist(at, l.stop); d > followReach {
		t.Fatalf("%s settled %.0f from the GM, want at most %d", name, d, followReach)
	}

	for range 3 {
		tell(t, ctx, other, name, "follow")
		if err := other.Watch(ctx, 2*time.Second, func(botclient.Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := moveToward(ctx, other, awayFrom(other.Pos(), 900, math.Pi)); err != nil {
		t.Fatal(err)
	}
	if err := gm.Watch(ctx, 15*time.Second, func(botclient.Event) {}); err != nil {
		t.Fatal(err)
	}
	at = settled(t, ctx, gm.Client, b)
	if d := dist(at, gm.Pos()); d > followReach {
		t.Fatalf("%s ended %.0f from the GM after a non-commander's follow, want at most %d (%.0f from %s)", name, d, followReach, dist(at, other.Pos()), other.Self.Name)
	}
}
