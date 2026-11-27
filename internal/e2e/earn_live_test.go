package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The earning scenario's character (#530): an orc mystic at level 5, as the
// home run's bot that stood idle under a death penalty (#527), with death
// penalty level 1. By the bot's model of its stats three spots around Orc
// Village fit it without the penalty and none with it, and four earn it
// adena (TestEarnShipped's model at level 5, penalty 1).
const (
	earnLevel   int32 = 5
	earnPenalty int32 = 1
	// earnKarma outlasts what a death at level 5 takes off (the level's
	// experience loss over 15, formulas.KarmaLoss): the death penalty
	// check reads the karma left, and with any left it always raises.
	earnKarma int32 = 100
	// lizardman is Lizardman, a level 19 monster that attacks nobody by
	// sight (no aggroRange) and fights back whoever hits it: it kills a
	// level 5 mystic in a few swings, and leaves it be once revived.
	lizardman int32 = 20152
	// earnDieTries bounds the attack clicks on the lizardman: a click the
	// server answers with ActionFailed (the monster not yet in sight) is
	// sent again after earnDieWait.
	earnDieTries = 6
	earnDieWait  = 20 * time.Second
)

// earnGround is where the home run's orc mystic stood idle (#527), north of
// Orc Village and out of its guards' reach: a guard attacks a character
// with karma.
var earnGround = botclient.Point{X: -42666, Y: -108339, Z: -1685}

// TestLiveEarn runs the earning mode (#527) against the server: an orc
// mystic the GM levelled to 5, under death penalty level 1 a monster gave
// it and with no adena, fits no farming spot; it claims an earning spot
// (Status.Earning), farms it and picks up adena a kill dropped. One pickup
// at its spot pays a Scroll: Recovery (#583: about 7,400 adena against a
// price of 2,200 to 3,300), so the bot then walks to Orc Village for one
// (penaltyTrip), buys it from the village trader, reads it, and its death
// penalty lifts to 0: its spot is an ordinary one again.
func TestLiveEarn(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	// The step bounds of the claim and the pickup fill most of 40 minutes;
	// the lift's walk to town, purchase and read add two more (#583).
	ctx := scenarioCtx(t, 50*time.Minute)
	spots := spotRegistry(t, tg)
	travel, _ := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("earn", 1)
	seedEarner(t, ctx, tg, gm, account)

	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.OrcMystic,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			p.Role = botclient.RoleDamage
			return p
		},
		// The travel map prices its monsters' drops (earnRate).
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots, cfg.Travel = spots, travel },
	})
	// Out of the farmer's way, in a peace zone out of any monster's reach.
	gmTeleport(t, ctx, gm, townTI)
	alive := time.Now()
	keepAlive := func() {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
	}

	untilBy(t, ctx, "the earning claim", time.Now().Add(stepBound), func() string {
		keepAlive()
		st := f.bot.Status()
		switch {
		case st.DeathPenalty < earnPenalty:
			return fmt.Sprintf("death penalty level %d, want %d at least", st.DeathPenalty, earnPenalty)
		case st.Spot == "":
			return "no spot held (rule " + st.Rule + ")"
		case !st.Earning:
			return st.Spot + " held as an ordinary spot, not an earning one"
		case st.Rule != "spot" && st.Rule != "farm":
			return "earning spot " + st.Spot + " held, rule " + st.Rule
		}
		return ""
	})
	st := f.bot.Status()
	t.Logf("%s (level %d, death penalty %d) claims earning spot %s, rule %s", f.name, st.Level, st.DeathPenalty, st.Spot, st.Rule)

	// The run's farming step bound, as TestLiveFarmFight's: a kill at
	// death penalty takes long, and not every kill drops adena.
	var picked earnPickup
	untilBy(t, ctx, "an adena pickup", time.Now().Add(25*time.Minute), func() string {
		keepAlive()
		picked = adenaPickup(f.watch.Seen(0), f.id)
		if picked.object == 0 {
			return fmt.Sprintf("%d adena drops in sight, none picked up by %s", picked.drops, f.name)
		}
		// The server sends GetItem before the picker's inventory update
		// (pickup.go takeGroundFromWorld), so a poll between the two sees
		// the pickup but no adena yet.
		if picked.adena <= 0 {
			return fmt.Sprintf("%s picked up adena %d, its inventory reports no adena yet", f.name, picked.object)
		}
		return ""
	})
	st = f.bot.Status()
	t.Logf("%s picked up adena %d, carries %d adena; spot %s earning %v, death penalty %d", f.name, picked.object, picked.adena, st.Spot, st.Earning, st.DeathPenalty)

	// The lift (#583): the supply trip's purchase (recoveryWants), refused
	// by no merchant, then the bot's own read (ruleRecover), refused by no
	// use condition. The walk to Orc Village's trader and its shop visit
	// take the step bound twice.
	scroll := botclient.RecoveryScroll(earnLevel)
	var lift earnLift
	report := func() string {
		st := f.bot.Status()
		return fmt.Sprintf("carries %d adena and %d Scrolls: Recovery %d (most %d), read %v, lifted %v; rule %s, spot %q earning %v, death penalty %d",
			lift.adena, lift.carried, scroll, lift.bought, lift.read, lift.lifted, st.Rule, st.Spot, st.Earning, st.DeathPenalty)
	}
	untilServedBy(t, ctx, "a Scroll: Recovery bought", time.Now().Add(2*stepBound), f.watch, merchantBuyRefused(f.watch), func() string {
		keepAlive()
		lift = penaltyLift(f.watch.Seen(0), scroll)
		if lift.bought == 0 {
			return report()
		}
		return ""
	})
	t.Logf("%s bought Scroll: Recovery %d (object %d), carries %d adena", f.name, scroll, lift.object, lift.adena)
	// The check's goroutine reads object; the polls below rewrite lift.
	object := lift.object
	read := RefusalCheck{
		From:     len(f.watch.Sent()),
		Packet:   func(s Sent) bool { return s.Op == botclient.OpUseItem && s.Target == object },
		Messages: []int32{msgS1CannotBeUsed},
	}
	untilServed(t, ctx, "the death penalty lifted", f.watch, read, func() string {
		keepAlive()
		lift = penaltyLift(f.watch.Seen(0), scroll)
		st := f.bot.Status()
		if !lift.read || !lift.lifted || st.DeathPenalty != 0 || st.Earning {
			return report()
		}
		return ""
	})
	st = f.bot.Status()
	t.Logf("%s read Scroll: Recovery %d: death penalty %d, spot %q earning %v, carries %d adena", f.name, scroll, st.DeathPenalty, st.Spot, st.Earning, lift.adena)
}

// earnLift is what penaltyLift read: the Scrolls: Recovery the bot carried
// last and the most it carried, the scroll's object id, whether its count
// fell (a read), whether the server lifted the bot's death penalty (system
// message msgDeathPenaltyLifted) and the adena it carried last.
type earnLift struct {
	carried int32
	bought  int32
	object  int32
	read    bool
	lifted  bool
	adena   int32
}

// penaltyLift reads seen, the bot's events, for its Scroll: Recovery
// scroll bought and read and its death penalty lifted.
func penaltyLift(seen []Seen, scroll int32) earnLift {
	var l earnLift
	for _, s := range seen {
		if m, ok := s.Event.(botclient.SystemMessage); ok {
			l.lifted = l.lifted || m.ID == msgDeathPenaltyLifted
			continue
		}
		if n, _, ok := stock(s.Event, adena); ok {
			l.adena = n
		}
		n, id, ok := stock(s.Event, scroll)
		if !ok {
			continue
		}
		if id != 0 {
			l.object = id
		}
		l.read = l.read || n < l.carried
		l.carried, l.bought = n, max(l.bought, n)
	}
	return l
}

// earnPickup is what adenaPickup read: the adena drops in sight, the
// first of them the picker picked up (0 for none) and the adena it carried
// last.
type earnPickup struct {
	drops  int
	object int32
	adena  int32
}

// adenaPickup reads seen for an adena ground item, dropped or in sight,
// that picker picked up (GetItem), and the adena its inventory reported
// last.
func adenaPickup(seen []Seen, picker int32) earnPickup {
	var p earnPickup
	ground := make(map[int32]bool)
	for _, s := range seen {
		var it botclient.GroundItem
		switch e := s.Event.(type) {
		case botclient.SpawnItem:
			it = e.Item
		case botclient.DropItem:
			it = e.Item
		case botclient.GetItem:
			if p.object == 0 && e.PickerID == picker && ground[e.ObjectID] {
				p.object = e.ObjectID
			}
			continue
		default:
			if n, _, ok := stock(s.Event, adena); ok {
				p.adena = n
			}
			continue
		}
		if it.TemplateID == adena && !ground[it.ObjectID] {
			ground[it.ObjectID] = true
			p.drops++
		}
	}
	return p
}

// seedEarner readies account's orc mystic for the earning scenario,
// whatever earlier runs left it: it enters as a scripted client, lifts a
// death penalty above earnPenalty (liftDeathPenalty), dies to a monster
// with karma until its penalty reaches earnPenalty (a monster kill with
// karma always raises it, character_deathpenalty.go; a GM's //kill never
// does: its killer is a player), is set to earnLevel, destroys its adena
// and any Scroll: Recovery of earnLevel, and logs out at earnGround with karma 0. The account is the scenario's
// own (group "earn"): no other scenario meets its penalty.
func seedEarner(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, account string) {
	t.Helper()
	c, err := tg.EnterWhenFree(ctx, account, character(t, account, &botclient.OrcMystic), nil)
	if err != nil {
		t.Fatal(err)
	}
	logoutAtEnd(t, c)
	tg.Own(c.Self.Name, c.Self.ObjectID)
	name, id := c.Self.Name, c.Self.ObjectID
	inv := make(map[int32]botclient.Item)
	list, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.ItemList)
		return ok
	})
	if err != nil {
		t.Fatalf("seed %s: its ItemList: %v", name, err)
	}
	applyInventory(inv, list)
	ui := userInfo(t, c)
	level := ui.Level
	if ui.HP == 0 {
		revive(t, ctx, c)
	}
	gmNext := func() {
		t.Helper()
		gmBeside(t, ctx, gm, c)
	}
	penalty, ok, err := entryDeathPenalty(c.Self.Burst)
	if err != nil || !ok {
		t.Fatalf("seed %s: no EtcStatusUpdate in its entry burst (%v): its death penalty is unknown", name, err)
	}
	t.Logf("%s entered at level %d, death penalty level %d, karma %d", name, level, penalty, ui.Karma)
	if penalty > earnPenalty {
		liftDeathPenalty(t, ctx, c, gm, inv, level, gmNext)
		penalty = 0
	}

	// Away from the village guards before it carries karma.
	gmTeleport(t, ctx, gm, earnGround)
	if err := gm.Command(ctx, "recall "+name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Await(ctx, EntryTimeout, teleportOf(id)); err != nil {
		t.Fatalf("seed %s: recalled to %v: %v", name, earnGround, err)
	}
	if ui.Karma != 0 {
		gmSetKarma(t, ctx, gm, c, 0)
	}
	for penalty < earnPenalty {
		gmSetKarma(t, ctx, gm, c, earnKarma)
		penalty = dieToMonster(t, ctx, gm, c, inv, &level)
		gmSetKarma(t, ctx, gm, c, 0)
		t.Logf("%s died to a lizardman with karma: death penalty level %d, level %d", name, penalty, level)
	}

	if level != earnLevel {
		if err := gm.OnTarget(ctx, id, fmt.Sprintf("set level %d", earnLevel)); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			u, ok := ev.(botclient.UserInfo)
			return ok && u.Level == earnLevel
		}); err != nil {
			t.Fatalf("seed %s: level %d: %v", name, earnLevel, err)
		}
	}
	// No adena: a bot with a Scroll: Recovery's price buys it (penaltyTrip)
	// rather than earning it, and earlier runs left it what it earned. No
	// Scroll: Recovery of its level either: a run that failed between the
	// purchase and the read, or a trip that bought more than one, left it
	// one, and the bot reads it on entry (ruleRecover), lifting the penalty
	// the earning claim needs.
	scroll := botclient.RecoveryScroll(earnLevel)
	for oid, it := range inv {
		if (it.TemplateID != adena && it.TemplateID != scroll) || it.Count <= 0 {
			continue
		}
		what := "adena"
		if it.TemplateID == scroll {
			what = fmt.Sprintf("Scrolls: Recovery %d", scroll)
		}
		if err := c.Send(ctx, botwire.EncodeRequestDestroyItem(oid, it.Count)); err != nil {
			t.Fatal(err)
		}
		refused := false
		if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			if m, ok := ev.(botclient.SystemMessage); ok && slices.Contains(destroyRefusals, m.ID) {
				refused = true
			}
			return refused || carriedOf(inv, it.TemplateID) == 0
		}); err != nil || refused {
			t.Fatalf("seed %s: destroy its %d %s: refused %v, %v", name, it.Count, what, refused, err)
		}
		t.Logf("%s destroyed the %d %s earlier runs left it", name, it.Count, what)
	}
	lctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := c.Logout(lctx); err != nil {
		t.Fatalf("seed %s: logout: %v", name, err)
	}
	c.Close()
}

// dieToMonster has c, a character with karma next to the GM, die to a
// lizardman the GM spawns at it: c hits it, it hits back. It waits for c's
// death and the death penalty level the server raises with it (system
// message msgDeathPenaltyLevel), then has the GM delete the lizardman and
// revive and heal c. It returns the new death penalty level, and keeps
// level, c's level, current (a death may take a level off).
func dieToMonster(t *testing.T, ctx context.Context, gm *GMClient, c *Client, inv map[int32]botclient.Item, level *int32) int32 {
	t.Helper()
	name, id := c.Self.Name, c.Self.ObjectID
	gmBeside(t, ctx, gm, c)
	mob, err := gm.Spawn(ctx, lizardman)
	if err != nil {
		t.Fatal(err)
	}
	dead, raised, seen := false, int32(0), false
	var last error
	for try := 0; !dead || raised == 0; try++ {
		if try == earnDieTries {
			t.Fatalf("seed %s: dead %v, death penalty raised to %d after %d attack clicks on lizardman %d: %v", name, dead, raised, earnDieTries, mob, last)
		}
		if seen && !dead {
			if err := c.Send(ctx, botwire.EncodeAttackRequest(mob, c.Pos())); err != nil {
				t.Fatal(err)
			}
		}
		_, last = c.Await(ctx, earnDieWait, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			switch e := ev.(type) {
			case botclient.NPCInfo:
				if e.ObjectID == mob && !seen {
					seen = true
					return true // click it now
				}
			case botclient.UserInfo:
				*level = e.Level
			case botclient.Die:
				dead = dead || e.ObjectID == id
			case botclient.SystemMessage:
				if e.ID == msgDeathPenaltyLevel {
					raised = e.Number
				}
			}
			return dead && raised > 0
		})
		if ctx.Err() != nil {
			t.Fatalf("seed %s: death to lizardman %d: %v", name, mob, ctx.Err())
		}
	}
	if err := gm.Delete(ctx, mob); err != nil {
		t.Fatal(err)
	}
	if err := gm.Command(ctx, "res "+name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		applyInventory(inv, ev)
		if u, ok := ev.(botclient.UserInfo); ok {
			*level = u.Level
		}
		r, ok := ev.(botclient.Revive)
		return ok && r.ObjectID == id
	}); err != nil {
		t.Fatalf("seed %s: revived by the GM: %v", name, err)
	}
	if err := gm.Command(ctx, "heal "+name); err != nil {
		t.Fatal(err)
	}
	return raised
}

// gmBeside brings the GM next to c and makes c its target.
func gmBeside(t *testing.T, ctx context.Context, gm *GMClient, c *Client) {
	t.Helper()
	if err := gm.Command(ctx, "teleportto "+c.Self.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(c.Self.ObjectID)); err != nil {
		t.Fatalf("GM next to %s: %v", c.Self.Name, err)
	}
	if err := gm.Select(ctx, c.Self.ObjectID); err != nil {
		t.Fatal(err)
	}
}

// gmSetKarma has the GM set c's karma to karma, and waits for the server's
// answer naming c (admin_set.go).
func gmSetKarma(t *testing.T, ctx context.Context, gm *GMClient, c *Client, karma int32) {
	t.Helper()
	if err := gm.OnTarget(ctx, c.Self.ObjectID, fmt.Sprintf("set karma %d", karma)); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("You successfully set %s's karma to %d.", c.Self.Name, karma)
	if _, err := gm.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.SystemMessage)
		return ok && m.ID == sysMsgS1 && strings.EqualFold(m.Text[0], want)
	}); err != nil {
		t.Fatalf("seed %s: karma %d: no answer %q: %v", c.Self.Name, karma, want, err)
	}
}
