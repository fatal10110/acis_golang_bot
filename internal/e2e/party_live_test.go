package e2e

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLivePartyGMInviteLeave has the GM invite a bot it commands: the bot
// accepts within the invitation window and the GM leads the party; the GM's
// whispered "leave" takes the bot out, which disperses the party of two
// (#17, #19).
func TestLivePartyGMInviteLeave(t *testing.T) {
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 5*time.Minute)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("pt", 1)
	b := runBotWith(t, ctx, tg, account, botSetup{persona: partyPersona, brain: commandedBy(gm)})
	name := botOnline(t, ctx, tg, b)

	if r := invite(t, ctx, gm.Client, name); r != 1 {
		t.Fatalf("%s answered the GM's invitation %d, want 1", name, r)
	}
	watchUntil(t, ctx, gm.Client, "the GM leading the bot's party", leaderIs(b, gm.Self.Name), nil)

	tell(t, ctx, gm.Client, name, "leave")
	if _, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.PartySmallWindowDeleteAll)
		return ok
	}); err != nil {
		t.Fatalf("party dispersed after leave: %v", err)
	}
	watchUntil(t, ctx, gm.Client, "the bot out of the party", leaderIs(b, ""), nil)
}

// TestLivePartySwarmOneJoins has a player, no GM, invite two bots of one
// swarm: the first joins, the second declines while the first holds the
// player as its leader; once the first left, the second joins (#72).
func TestLivePartySwarmOneJoins(t *testing.T) {
	tg := Live(t)
	ctx := scenarioCtx(t, 5*time.Minute)
	acc, _ := tg.Account("sw", 1)
	player := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)
	claims := botclient.NewPartyClaims()
	swarm := botSetup{persona: partyPersona, brain: claims.Apply}
	acc1, _ := tg.Account("sw", 2)
	acc2, _ := tg.Account("sw", 3)
	b1 := runBotWith(t, ctx, tg, acc1, swarm)
	b2 := runBotWith(t, ctx, tg, acc2, swarm)
	n1 := botOnline(t, ctx, tg, b1)
	n2 := botOnline(t, ctx, tg, b2)

	if r := invite(t, ctx, player, n1); r != 1 {
		t.Fatalf("%s answered %d, want 1", n1, r)
	}
	watchUntil(t, ctx, player, "the first bot in the player's party", leaderIs(b1, player.Self.Name), nil)
	if r := invite(t, ctx, player, n2); r != 0 {
		t.Fatalf("%s answered %d while %s holds the player, want 0", n2, r, n1)
	}
	if l := b2.Status().Leader; l != "" {
		t.Fatalf("%s joined %s's party", n2, l)
	}

	answer, err := b1.Command(botclient.Command{Kind: botclient.CommandLeave})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := player.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.PartySmallWindowDeleteAll)
		return ok
	}); err != nil {
		t.Fatalf("party dispersed after the first bot left: %v", err)
	}
	select {
	case err := <-answer:
		if err != nil {
			t.Fatalf("leave: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("leave: %v", ctx.Err())
	}
	watchUntil(t, ctx, player, "the claim given back", func() bool { return claims.Held() == 0 }, nil)
	if r := invite(t, ctx, player, n2); r != 1 {
		t.Fatalf("%s answered %d once %s left, want 1", n2, r, n1)
	}
	watchUntil(t, ctx, player, "the second bot in the player's party", leaderIs(b2, player.Self.Name), nil)
}
