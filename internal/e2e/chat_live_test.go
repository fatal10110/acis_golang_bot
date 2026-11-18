package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// TestLiveChatReplies whispers to a bot of the default chat policy (#36):
// the GM's shout and trade lines, which the bot hears, get no reply; the
// GM's first whisper gets exactly one, a whisper after the policy's least
// delay; the GM's second whisper, inside the policy's window per sender,
// gets none; nor does a whisper of a second client of the run, whose name
// carries the bot marker.
func TestLiveChatReplies(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	if botnames.DefaultMarker.Marks(gm.Self.Name) {
		t.Fatalf("the GM %s carries the bot marker: no reply to it is due", gm.Self.Name)
	}
	account, _ := tg.Account("ch", 1)
	f := runWatched(t, ctx, tg, account, botSetup{persona: partyPersona})
	policy := partyPersona().Chat
	// Shout and trade lines reach the players of a region: the bot stands
	// next to the GM.
	toTown(t, ctx, tg, gm, f)

	for _, line := range []struct {
		name    string
		channel int32
	}{{"shout", botclient.ChatShout}, {"trade", botclient.ChatTrade}} {
		from := f.seen()
		if err := gm.Send(ctx, botclient.EncodeSay2("e2e test line, please ignore", line.channel)); err != nil {
			t.Fatal(err)
		}
		if n := tells(t, ctx, gm.Client, f.name, silence); n != 0 {
			t.Errorf("%d replies to the GM's %s line, want none", n, line.name)
		}
		if !f.heard(from, gm.Self.Name, line.channel) {
			t.Errorf("%s never heard the GM's %s line", f.name, line.name)
		}
	}

	sent := time.Now()
	tell(t, ctx, gm.Client, f.name, "hello there")
	if _, err := gm.Await(ctx, policy.DelayMax+10*time.Second, tellFrom(f.name)); err != nil {
		t.Fatalf("%s replies to the GM's whisper: %v", f.name, err)
	}
	// The delay runs from the bot reading the whisper; the round trips
	// only add to it.
	if d := time.Since(sent); d < policy.DelayMin {
		t.Errorf("the reply came after %v, want %v at least", d, policy.DelayMin)
	}
	t.Logf("%s replied to the GM after %v", f.name, time.Since(sent).Round(time.Millisecond))
	if n := tells(t, ctx, gm.Client, f.name, silence); n != 0 {
		t.Errorf("%d more replies to the GM's whisper, want exactly one", n)
	}

	from := f.seen()
	tell(t, ctx, gm.Client, f.name, "you still there?")
	if n := tells(t, ctx, gm.Client, f.name, silence); n != 0 {
		t.Errorf("%d replies to the GM's second whisper within %v, want none", n, policy.PerSender)
	}
	if !f.heard(from, gm.Self.Name, chatTell) {
		t.Errorf("%s never heard the GM's second whisper", f.name)
	}

	otherAccount, _ := tg.Account("ch", 2)
	c := enter(t, ctx, tg, otherAccount, character(t, otherAccount, nil), nil)
	from = f.seen()
	tell(t, ctx, c, f.name, "hello there")
	if n := tells(t, ctx, c, f.name, silence); n != 0 {
		t.Errorf("%d replies to %s, a marked name, want none", n, c.Self.Name)
	}
	if !f.heard(from, c.Self.Name, chatTell) {
		t.Errorf("%s never heard %s's whisper", f.name, c.Self.Name)
	}
}
