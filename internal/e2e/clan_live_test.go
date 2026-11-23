package e2e

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// clanCreateLevel is the lowest level that founds a clan
// (clan/membership.go minCreateLevel).
const clanCreateLevel = 10

// TestLiveClanInvite has a clan leader the bot knows as a GM, its one
// commander, invite the bot: the bot accepts within the invitation window,
// its UserInfo carries the clan id and the clan's roster names the leader;
// the clan dissolved takes the bot out (#185). The pool's GM is below the
// founding level and the harness levels only run characters, so a run
// character leads the clan and the bot's -commanders allowlist makes it a
// known GM, the same Authority path a real GM takes. The bot records the
// first frame of each opcode it reads into the clan directory of
// BOT_E2E_RECORD when set, else a temporary one: the clan frames there are
// the recorded fixtures of the clan decoders.
func TestLiveClanInvite(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 5*time.Minute)
	gm := enterGM(t, ctx, tg)
	acc, _ := tg.Account("cl", 1)
	leader := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)
	botAcc, _ := tg.Account("cl", 2)
	rec, err := botclient.NewRecorder(filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "clan"))
	if err != nil {
		t.Fatal(err)
	}
	b := runBotWith(t, ctx, tg, botAcc, botSetup{persona: partyPersona, tap: rec, brain: func(cfg *botclient.BrainConfig) {
		a := botclient.NewAuthority(botclient.AuthorityConfig{Commanders: []int32{leader.Self.ObjectID}, Clock: botclient.SystemClock{}})
		cfg.Authority = a
		cfg.Persona = a.Hooks(cfg.Persona)
	}})
	name := botOnline(t, ctx, tg, b)
	// The GM teleports to the leader, which shows it the leader afresh even
	// when it stood in sight already, then recalls the bot to them both.
	leaderID := leader.Self.ObjectID
	tg.Own(leader.Self.Name, leaderID)
	if err := gm.Command(ctx, "teleportto "+leader.Self.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(leaderID)); err != nil {
		t.Fatalf("GM next to %s: %v", leader.Self.Name, err)
	}
	// The leader stays a stranger who never whispered: the bot accepts it
	// as the commander it sees once its landing shows it, even when the
	// invitation comes before that (#199).
	botID := recallBot(t, ctx, tg, gm, name)
	// Selected once: the server confirms no second click on the GM's
	// target, and nothing below clears it.
	if err := gm.Select(ctx, leaderID); err != nil {
		t.Fatal(err)
	}
	onLeader := gm.Command

	ui := userInfo(t, leader)
	if ui.ClanID != 0 {
		t.Logf("%s leads a clan from an earlier run: dismissed", leader.Self.Name)
		if err := onLeader(ctx, "pledge dismiss"); err != nil {
			t.Fatal(err)
		}
	}
	if ui.Level < clanCreateLevel {
		if err := onLeader(ctx, "set level "+strconv.Itoa(clanCreateLevel)); err != nil {
			t.Fatal(err)
		}
		if _, err := leader.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
			u, ok := ev.(botclient.UserInfo)
			return ok && u.Level >= clanCreateLevel
		}); err != nil {
			t.Fatalf("%s at level %d: %v", leader.Self.Name, clanCreateLevel, err)
		}
	}

	// A clan name of the run's own: letters and digits, at most 16.
	clan := "e2e" + strconv.FormatInt(time.Now().UnixNano()%1e12, 36)
	if err := onLeader(ctx, "pledge create "+clan); err != nil {
		t.Fatal(err)
	}
	ev, err := leader.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		r, ok := ev.(botclient.PledgeShowMemberListAll)
		return ok && r.PledgeType == 0 && r.Name == clan
	})
	if err != nil {
		t.Fatalf("%s founds clan %s: %v", leader.Self.Name, clan, err)
	}
	clanID := ev.(botclient.PledgeShowMemberListAll).ClanID
	dismissed := false
	t.Cleanup(func() {
		if dismissed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := onLeader(ctx, "pledge dismiss"); err != nil {
			t.Errorf("dismiss clan %s: %v", clan, err)
		}
	})

	if err := leader.Send(ctx, botclient.EncodeRequestJoinPledge(botID)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ctx, "the bot's UserInfo showing the clan id", func() bool { return b.Status().ClanID == clanID })
	waitFor(t, ctx, "the clan's roster naming the leader", func() bool { return b.Status().ClanLeader == leader.Self.Name })

	if err := onLeader(ctx, "pledge dismiss"); err != nil {
		t.Fatal(err)
	}
	dismissed = true
	waitFor(t, ctx, "the bot out of the dissolved clan", func() bool {
		st := b.Status()
		return st.ClanID == 0 && st.ClanLeader == ""
	})
	if err := rec.Err(); err != nil {
		t.Error(err)
	}
}
