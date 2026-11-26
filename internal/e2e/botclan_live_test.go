package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The scenarios whose bot leads a clan of the run's own (the crafter, the
// crystallizer, the clan warehouse) found it through GM commands on the bot.
// Every one runs with the bot as the GM's target (OnTarget): the server runs
// a target command on the GM itself when its target is none, as it is a
// moment after a recall (#206), and a //pledge create on the GM founds
// nothing for the bot (#524).

// System messages the server sends about //pledge create and dismiss
// (serverpackets/systemmessage_clan.go, systemmessage_alliance.go).
const (
	msgS1AlreadyExists             int32 = 5
	msgClanCreated                 int32 = 189
	msgFailedToCreateClan          int32 = 190
	msgNotMeetCriteriaToCreateClan int32 = 229
	msgMustWaitBeforeCreatingClan  int32 = 230
	msgTargetMustBeInClan          int32 = 234
	msgClanNameInvalid             int32 = 261
	msgClanNameLengthIncorrect     int32 = 262
)

// clanDisbanded is the GM's word that //pledge dismiss dissolved its
// target's clan (network/admin_pledge.go).
const clanDisbanded = "The clan is now disbanded."

// foundingCause names the outcome of //pledge create that m tells the
// founder (network/clan_villagemaster.go createClan), and reports whether m
// is one. The GM is told "There was a problem while creating the clan." for
// every refusal, so only the founder's message names the cause.
func foundingCause(m botclient.SystemMessage) (string, bool) {
	switch m.ID {
	case msgClanCreated:
		return "ClanCreated", true
	case msgFailedToCreateClan:
		return "FailedToCreateClan (already in a clan, or the server failed to allocate a clan id)", true
	case msgNotMeetCriteriaToCreateClan:
		return "NotMeetCriteriaToCreateClan (below the founding level)", true
	case msgMustWaitBeforeCreatingClan:
		return "MustWaitBeforeCreatingClan", true
	case msgClanNameInvalid:
		return "ClanNameInvalid", true
	case msgClanNameLengthIncorrect:
		return "ClanNameLengthIncorrect", true
	case msgS1AlreadyExists:
		return fmt.Sprintf("S1AlreadyExists (%q)", m.Text[0]), true
	}
	return "", false
}

// clanSaidTap is a bot's tap that also hands the test the founding outcomes
// the bot reads (foundingCause), dropping one when said is full. It runs on
// the bot's reader goroutine and keeps no payload.
type clanSaidTap struct {
	botclient.FrameTap
	said chan botclient.SystemMessage
}

// withClanSaid wraps tap, nil for none, in a clanSaidTap.
func withClanSaid(tap botclient.FrameTap) clanSaidTap {
	return clanSaidTap{FrameTap: tap, said: make(chan botclient.SystemMessage, 8)}
}

func (c clanSaidTap) Record(payload []byte) {
	if c.FrameTap != nil {
		c.FrameTap.Record(payload)
	}
	if len(payload) == 0 || payload[0] != opSysMsg {
		return
	}
	ev, _, err := botclient.DecodeEvent(payload)
	if err != nil {
		return
	}
	m, ok := ev.(botclient.SystemMessage)
	if !ok {
		return
	}
	if _, ok := foundingCause(m); !ok {
		return
	}
	select {
	case c.said <- m:
	default:
	}
}

func (c clanSaidTap) RecordSent(payload []byte) {
	if c.FrameTap != nil {
		c.FrameTap.RecordSent(payload)
	}
}

// onBot runs each admin command cmds with botID, the run's bot, as the GM's
// target.
func onBot(t *testing.T, ctx context.Context, gm *GMClient, botID int32, cmds ...string) {
	t.Helper()
	for _, cmd := range cmds {
		if err := gm.OnTarget(ctx, botID, cmd); err != nil {
			t.Fatal(err)
		}
	}
}

// dismissAnswer matches the server's answer to //pledge dismiss: the clan
// disbanded, or its target in none.
func dismissAnswer(ev botclient.Event) bool {
	m, ok := ev.(botclient.SystemMessage)
	return ok && (m.ID == msgTargetMustBeInClan || m.Text[0] == clanDisbanded)
}

// leaveOldClan dissolves the clan an earlier run left the bot b (botID,
// named name) leading. It asks the server, not the bot's status: the GM
// dismisses whatever clan the bot is in, and the server answers that it
// disbanded one or that the bot is in none.
func leaveOldClan(t *testing.T, ctx context.Context, gm *GMClient, b *botclient.Bot, botID int32, name string) {
	t.Helper()
	onBot(t, ctx, gm, botID, "pledge dismiss")
	ev, err := gm.Await(ctx, EntryTimeout, dismissAnswer)
	if err != nil {
		t.Fatalf("dismiss %s's clan of an earlier run: %v", name, err)
	}
	if ev.(botclient.SystemMessage).ID == msgTargetMustBeInClan {
		return
	}
	t.Logf("%s led a clan from an earlier run: dismissed", name)
	waitFor(t, ctx, "the bot out of its old clan", func() bool { return b.Status().ClanID == 0 })
}

// foundBotClan has the GM found a clan of the run's own, named prefix and a
// base-36 stamp, led by the bot b (botID, named name, at the founding level
// and in no clan), and dissolve it when the test ends. said is the bot's
// clanSaidTap channel: a refused founding fails the test with what the
// server told the bot.
func foundBotClan(t *testing.T, ctx context.Context, gm *GMClient, b *botclient.Bot, botID int32, name, prefix string, said <-chan botclient.SystemMessage) (string, int32) {
	t.Helper()
	for drained := false; !drained; {
		select {
		case <-said:
		default:
			drained = true
		}
	}
	clan := prefix + strconv.FormatInt(time.Now().UnixNano()%1e12, 36)
	onBot(t, ctx, gm, botID, "pledge create "+clan)
	ev, err := gm.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && (strings.HasPrefix(m.Text[0], "Clan "+clan+" ") || strings.Contains(m.Text[0], "creating the clan"))
	})
	if err != nil {
		t.Fatalf("pledge create %s: %v; %s was told %s", clan, err, name, founderTold(said))
	}
	if m := ev.(botclient.SystemMessage); !strings.HasPrefix(m.Text[0], "Clan "+clan+" have been created") {
		t.Fatalf("pledge create %s: the GM was told %q, %s %s", clan, m.Text[0], name, founderTold(said))
	}
	waitFor(t, ctx, "the bot leading its clan", func() bool { return b.Status().ClanID != 0 })
	t.Cleanup(func() { dismissBotClan(t, gm, botID, name, clan) })
	return clan, b.Status().ClanID
}

// toldWait is how long founderTold waits for the founder's message, which
// the server sends it before it answers the GM.
const toldWait = 5 * time.Second

// founderTold names the founding outcome the bot read: none within toldWait
// means the command acted on another than the bot.
func founderTold(said <-chan botclient.SystemMessage) string {
	select {
	case m := <-said:
		cause, _ := foundingCause(m)
		return "was told " + cause
	case <-time.After(toldWait):
		return fmt.Sprintf("was told nothing within %v (the command did not act on it)", toldWait)
	}
}

// dismissBotClan dissolves clan, the run's clan the bot (botID, named name)
// leads, at the test's end: the GM joins the bot, which may have walked out
// of its sight, and dismisses its clan, reporting a clan the server did not
// disband.
func dismissBotClan(t *testing.T, gm *GMClient, botID int32, name, clan string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := gm.Command(ctx, "teleportto "+name); err != nil {
		t.Errorf("dismiss clan %s: %v", clan, err)
		return
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(botID)); err != nil {
		t.Errorf("dismiss clan %s: GM next to %s: %v", clan, name, err)
		return
	}
	if err := gm.OnTarget(ctx, botID, "pledge dismiss"); err != nil {
		t.Errorf("dismiss clan %s: %v", clan, err)
		return
	}
	ev, err := gm.Await(ctx, EntryTimeout, dismissAnswer)
	if err != nil {
		t.Errorf("dismiss clan %s: %v", clan, err)
		return
	}
	if m := ev.(botclient.SystemMessage); m.Text[0] != clanDisbanded {
		t.Errorf("dismiss clan %s: the server answered message %d, want %q", clan, m.ID, clanDisbanded)
	}
}
