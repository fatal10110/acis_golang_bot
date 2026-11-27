package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Helpers of the scenarios that run whole bots next to scripted clients:
// party, follow, whisper, support and recover.

// botSetup is how runBotWith runs a bot.
type botSetup struct {
	persistent bool
	metrics    *botmetrics.Bot
	// class is the class of the bot's character when it creates one; nil
	// draws one.
	class *botclient.StartingClass
	// persona is the bot's persona; nil plays DefaultPersona.
	persona func() botclient.Persona
	// brain, when set, adjusts the brain config last: authority, claims.
	brain func(*botclient.BrainConfig)
	// tap, when set, sees every frame the bot reads and writes (a Watch).
	tap botclient.FrameTap
}

// runBotWith runs a whole Bot of account as setup says, on the run's gate
// and a place of its own, until the test ends, which logs it out and joins
// it.
func runBotWith(t *testing.T, ctx context.Context, tg *Target, account string, setup botSetup) *botclient.Bot {
	t.Helper()
	b, _ := startBot(t, ctx, tg, account, setup)
	return b
}

// startBot runs a whole Bot as runBotWith does and returns with it the stop
// that logs it out, joins it and gives its place back: a scenario may call
// it before the test ends, which calls it in any case.
func startBot(t *testing.T, ctx context.Context, tg *Target, account string, setup botSetup) (*botclient.Bot, func()) {
	t.Helper()
	walkOn := tg.Nav(t) // before Hold: a skip there would keep the slot
	release, err := tg.Hold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := tg.SessionConfig(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cfg.Persistent = setup.persistent
	cfg.Recorder = setup.tap
	persona := botclient.DefaultPersona
	if setup.persona != nil {
		persona = setup.persona
	}
	brain := botclient.BrainConfig{Persona: persona(), Metrics: setup.metrics, Nav: walkOn}
	if setup.brain != nil {
		setup.brain(&brain)
	}
	b, err := botclient.NewBot(cfg, account, tg.Password(account), botclient.BotConfig{Character: character(t, account, setup.class), Brain: brain}, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	if err != nil {
		release()
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(runCtx) }()
	var once sync.Once
	stopBot := func() {
		once.Do(func() {
			stop()
			if err := <-done; err != nil {
				t.Logf("bot %s stopped: %v", account, err)
			}
			release()
		})
	}
	t.Cleanup(stopBot)
	return b, stopBot
}

// partyPersona is DefaultPersona without the errands that outrank following
// the party leader (trainer, shop, gear, newbie buffs) and without idle
// drift: a scenario's bot then moves only for what the scenario asserts.
// Every one of them is a persona default (D6), not a safety limit.
func partyPersona() botclient.Persona {
	p := botclient.DefaultPersona()
	p.LearnSkills, p.Trainers = map[int32][]int32{}, map[int32][]int32{}
	p.Shop, p.Gear, p.Newbie = botclient.ShopPolicy{}, botclient.GearPolicy{Classes: []botclient.GearClass{}}, botclient.NewbiePolicy{}
	p.IdleChance = 0
	return p
}

// commandedBy makes the GM the bot's one commander (the -commanders
// allowlist: the run has no database), whose party invitations it accepts.
func commandedBy(gm *GMClient) func(*botclient.BrainConfig) {
	return func(cfg *botclient.BrainConfig) {
		a := botclient.NewAuthority(botclient.AuthorityConfig{Commanders: []int32{gm.Self.ObjectID}, Clock: botclient.SystemClock{}})
		cfg.Authority = a
		cfg.Persona = a.Hooks(cfg.Persona)
	}
}

// botOnline waits for b to be online and returns its character's name, one
// of the run's from then on.
func botOnline(t *testing.T, ctx context.Context, tg *Target, b *botclient.Bot) string {
	t.Helper()
	waitFor(t, ctx, "the bot online", online(b))
	name := b.Status().Character
	tg.Own(name, 0)
	return name
}

// recallBot recalls the run character name to the GM and returns its
// object id, which GM commands may then target.
func recallBot(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, name string) int32 {
	t.Helper()
	if err := gm.Command(ctx, "recall "+name); err != nil {
		t.Fatal(err)
	}
	ev, err := gm.Await(ctx, EntryTimeout, charInfoNamed(name))
	if err != nil {
		t.Fatalf("GM sees %s recalled: %v", name, err)
	}
	id := ev.(botclient.CharInfo).ObjectID
	tg.Own(name, id)
	return id
}

// openGround is the human newbie start of Talking Island: open, level
// ground. The GM stands wherever the last run left it, and some of those
// spots refuse every move (ActionFailed in all directions), so the
// scenarios that walk the GM start it here.
var openGround = botclient.Point{X: -71338, Y: 258271, Z: -3104}

// gmToOpenGround teleports the GM to openGround and waits for it to land.
func gmToOpenGround(t *testing.T, ctx context.Context, gm *GMClient) {
	t.Helper()
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", openGround.X, openGround.Y, openGround.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM teleported to open ground: %v", err)
	}
}

// recallClient recalls c's character to the GM and waits for it to land.
func recallClient(t *testing.T, ctx context.Context, gm *GMClient, c *Client) {
	t.Helper()
	if err := gm.Command(ctx, "recall "+c.Self.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Await(ctx, EntryTimeout, teleportOf(c.Self.ObjectID)); err != nil {
		t.Fatalf("%s recalled: %v", c.Self.Name, err)
	}
}

func charInfoNamed(name string) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		ci, ok := ev.(botclient.CharInfo)
		return ok && strings.EqualFold(ci.Name, name)
	}
}

// partyAnswer is how long an inviter waits for the answer: the server's
// window (15 s) and a margin.
const partyAnswer = 20 * time.Second

// invite invites the player named name into c's party and returns the
// answer, 1 accepted and 0 declined.
func invite(t *testing.T, ctx context.Context, c *Client, name string) int32 {
	t.Helper()
	if err := c.Send(ctx, botwire.EncodeRequestJoinParty(name, 0)); err != nil {
		t.Fatal(err)
	}
	ev, err := c.Await(ctx, partyAnswer, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.JoinParty)
		return ok
	})
	if err != nil {
		t.Fatalf("%s's answer to %s's invitation: %v", name, c.Self.Name, err)
	}
	return ev.(botclient.JoinParty).Response
}

// tell whispers text to the player named name.
func tell(t *testing.T, ctx context.Context, c *Client, name, text string) {
	t.Helper()
	if err := c.Send(ctx, botwire.EncodeSay2Tell(name, text)); err != nil {
		t.Fatal(err)
	}
}

// leaderIs reports b in a party led by leader ("" for in none).
func leaderIs(b *botclient.Bot, leader string) func() bool {
	return func() bool { return strings.EqualFold(b.Status().Leader, leader) }
}

// watchUntil is watchUntilBy with the default step bound.
func watchUntil(t *testing.T, ctx context.Context, c *Client, what string, cond func() bool, fn func(botclient.Event)) {
	t.Helper()
	watchUntilBy(t, ctx, c, what, time.Now().Add(stepBound), cond, fn)
}

// watchUntilBy reads c, handing fn (when set) every event, until cond
// holds, or fails the test, with the last events c read, once the step is
// overdue at due or ctx ends. It is waitForBy for a scenario whose client
// must keep reading meanwhile.
func watchUntilBy(t *testing.T, ctx context.Context, c *Client, what string, due time.Time, cond func() bool, fn func(botclient.Event)) {
	t.Helper()
	tail := eventTail{clock: botclient.SystemClock{}}
	runStep(t, ctx, what, due, func(ctx context.Context) error {
		return c.Watch(ctx, 500*time.Millisecond, func(ev botclient.Event) {
			tail.note(ev)
			if fn != nil {
				fn(ev)
			}
		})
	}, pending(cond), tail.String)
}

// settleFor is how long a following bot's position stays put before it
// counts as settled.
const settleFor = 3 * time.Second

// dist is the ground distance between a and b.
func dist(a, b botclient.Point) float64 {
	return math.Hypot(float64(a.X)-float64(b.X), float64(a.Y)-float64(b.Y))
}

// userInfo decodes c's entry UserInfo: its speeds and stance at entry.
func userInfo(t *testing.T, c *Client) botclient.UserInfo {
	t.Helper()
	ev, ok, err := botclient.DecodeEvent(c.Self.UserInfo)
	ui, isUI := ev.(botclient.UserInfo)
	if err != nil || !ok || !isUI {
		t.Fatalf("decode %s's UserInfo: %v", c.Self.Name, err)
	}
	return ui
}

func castBy(caster, skill, target int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MagicSkillUse)
		return ok && m.CasterID == caster && m.SkillID == skill && (target == 0 || m.TargetID == target)
	}
}
