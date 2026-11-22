package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// Helpers of the M3 scenarios (#115): farmers, brain-driven bots watched
// through their frame tap (Watch), and the GM seeding them.

// Templates the scenarios seed or cast (aCis_datapack/data/xml).
const (
	// healingPotion is Healing Potion: skill 2032, reuse 10 s.
	healingPotion int32 = 1061
	// soulshotNG is Soulshot: No Grade; the human fighter's starting
	// Squire's Sword (2369, equipped) takes one per hit.
	soulshotNG int32 = 1835
	// wolf is Wolf, a level 4 monster (68 HP) of the Warrior AI: it attacks
	// no one on sight (script/ai/warrior/base.go), only whoever hits it.
	wolf int32 = 20120
	// orcGrunt is Orc Grunt, a level 7 monster of the same AI with 108 HP
	// and 53 P.Def: a level 5 fighter's shot swing or Power Strike seldom
	// kills it at once, so a kill takes swings of the server's auto-attack
	// (#20).
	orcGrunt int32 = 20131
	// packSize is how many orc grunts the GM spawns at once at a farmer:
	// each one the farmer hits fights back, and a pack keeps it fighting
	// long enough to take hits.
	packSize = 3
	// adena is the currency, never junk.
	adena int32 = 57
	// stem is a droppable material of weight 2: the farmer's junk.
	stem int32 = 1864
	// powerStrike is skill 3 level 1: hit time 1080 ms, reuse 13 s, 10 MP,
	// a sword or blunt weapon. A new human fighter learns it at level 5:
	// the GM sets it.
	powerStrike int32 = 3
)

// spotRegistry is the datapack's farming spots (BOT_E2E_DATAPACK), shared
// by the scenario's farmers, at the server's default spawn multiplier.
func spotRegistry(t *testing.T, tg *Target) *botclient.SpotRegistry {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	return botclient.NewSpotRegistry(dp.Spots(), 1)
}

// farmer is a whole brain-driven Bot farming the spots of a registry, and
// the window on what it reads and writes.
type farmer struct {
	bot   *botclient.Bot
	watch *Watch
	id    int32
	name  string
}

// runFarmer runs a human fighter of account with the persona tune adjusts
// (DefaultPersona when nil) on spots, through runWatched. It returns once
// the bot is in the world, its character one of the run's.
func runFarmer(t *testing.T, ctx context.Context, tg *Target, account string, spots *botclient.SpotRegistry, tune func(*botclient.Persona)) *farmer {
	t.Helper()
	return runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			if tune != nil {
				tune(&p)
			}
			return p
		},
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots = spots },
	})
}

// runWatched runs a whole Bot of account as setup says, through runBotWith
// with a Watch as its frame tap (setup.tap is replaced). It returns once the
// bot is in the world, its character one of the run's; a failed test logs
// the bot's digest. Its walker must never desync from the server on its own
// moves (docs/navigation.md, Testing): nav_desync_total stays 0, door
// replans counted apart, nav_blocked_total at most 1 per 100 segments, and
// every blocked stop is one the bot's step chain (WalkSteps) predicts or one
// after a step longer than Emax.
func runWatched(t *testing.T, ctx context.Context, tg *Target, account string, setup botSetup) *farmer {
	t.Helper()
	w := NewWatch(botclient.SystemClock{})
	setup.tap = w
	n := tg.Nav(t)
	var self int32
	var speed float64
	if setup.metrics == nil {
		reg := botmetrics.NewRegistry()
		setup.metrics = reg.NewBot()
		t.Cleanup(func() {
			s := reg.Collect(time.Now())
			t.Logf("bot %s nav: %d segments, blocked %d, desync %d, door replans %d", account, s.NavSegments, s.NavBlocked, s.NavDesync, s.NavDoorReplan)
			for _, bad := range navVerdict(s) {
				t.Errorf("bot %s: %s", account, bad)
			}
		})
	}
	t.Cleanup(func() {
		if self == 0 {
			return
		}
		plan := nav.Planner{Engine: n.Engine, Water: n.Water}
		explained, bad := stepVerdict(blockedStops(w.Seen(0), w.Sent(), self, speed), func(b blockedStop) bool {
			return stepPredicted(n.Engine, b, speed)
		}, func(b blockedStop) bool {
			return plan.CorridorOpen(toLocation(b.from), toLocation(b.to))
		})
		for _, e := range explained {
			t.Logf("bot %s step chain: %s", account, e)
		}
		for _, e := range bad {
			t.Errorf("bot %s step chain: %s", account, e)
		}
	})
	b := runBotWith(t, ctx, tg, account, setup)
	_, seen, err := w.Await(ctx, 0, func(ev botclient.Event) bool {
		_, ok := ev.(botclient.UserInfo)
		return ok
	})
	if err != nil {
		t.Fatalf("bot %s in the world: %v", account, err)
	}
	ui := seen.Event.(botclient.UserInfo)
	tg.Own(ui.Name, ui.ObjectID)
	// A character runs unless it toggled walking (a follower matching a
	// walking leader); the run speed is the strict reading of a stop.
	self, speed = ui.ObjectID, float64(max(ui.RunSpeed, 1))
	f := &farmer{bot: b, watch: w, id: ui.ObjectID, name: ui.Name}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.digest())
		}
	})
	return f
}

// forgetKilled drops from the run's monsters every one f saw die, so that
// a monster cleanup fails to delete is one still alive. The scenarios
// register it before runFarmer: it runs once the farmer stopped, so no
// kill comes after it.
func forgetKilled(gm *GMClient, f *farmer) {
	if f == nil {
		return
	}
	for _, s := range f.watch.Seen(0) {
		if d, ok := s.Event.(botclient.Die); ok && gm.owned(d.ObjectID) {
			gm.Forget(d.ObjectID)
		}
	}
}

// digest sums up what the farmer did, for a failed scenario: its status,
// the packets it sent by opcode and the events it read by type.
func (f *farmer) digest() string {
	s := f.bot.Status()
	sent, read := map[string]int{}, map[string]int{}
	for _, p := range f.watch.Sent() {
		sent[fmt.Sprintf("%#02x", p.Op)]++
	}
	deaths, items, load := 0, 0, ""
	for _, e := range f.watch.Seen(0) {
		read[fmt.Sprintf("%T", e.Event)]++
		switch ev := e.Event.(type) {
		case botclient.Die:
			if ev.ObjectID == f.id {
				deaths++
			}
		case botclient.UserInfo:
			load = fmt.Sprintf("%d/%d", ev.Load, ev.MaxLoad)
		case botclient.ItemList:
			items = len(ev.Items)
		}
	}
	return fmt.Sprintf("%s: state %v, rule %q, spot %q, level %d, HP %d/%d, at %v, died %d times, load %s, %d items at login; sent %v; read %v; last moves %s",
		f.name, s.State, s.Rule, s.Spot, s.Level, s.HP, s.MaxHP, s.Position, deaths, load, items, sent, read, f.trail(trailLen))
}

// trailLen is how many lines of a failed farmer's trail its digest shows.
const trailLen = 40

// trail is the last n of f's clicks and of the moves the server reported
// for it, oldest first, timed from the first: where an approach the server
// walked got to.
func (f *farmer) trail(n int) string {
	type line struct {
		at   time.Time
		text string
	}
	var lines []line
	for _, p := range f.watch.Sent() {
		switch p.Op {
		case botclient.OpAction:
			lines = append(lines, line{p.At, fmt.Sprintf("click %d", p.Target)})
		case botclient.OpMoveBackwardToLocation:
			if len(p.Payload) >= 9 {
				x, y := int32(binary.LittleEndian.Uint32(p.Payload[1:5])), int32(binary.LittleEndian.Uint32(p.Payload[5:9]))
				lines = append(lines, line{p.At, fmt.Sprintf("click to (%d,%d)", x, y)})
			}
		}
	}
	for _, e := range f.watch.Seen(0) {
		var text string
		switch ev := e.Event.(type) {
		case botclient.MoveToPawn:
			if ev.ObjectID == f.id {
				text = fmt.Sprintf("to pawn %d from %v", ev.TargetID, ev.Origin)
			}
		case botclient.MoveToLocation:
			if ev.ObjectID == f.id {
				text = fmt.Sprintf("move %v to %v", ev.Origin, ev.Dest)
			}
		case botclient.StopMove:
			if ev.ObjectID == f.id {
				text = fmt.Sprintf("stop %v", ev.Pos)
			}
		case botclient.ValidateLocation:
			if ev.ObjectID == f.id {
				text = fmt.Sprintf("at %v", ev.Pos)
			}
		case botclient.TeleportToLocation:
			if ev.ObjectID == f.id {
				text = fmt.Sprintf("teleport %v", ev.Pos)
			}
		case botclient.PartyMemberPosition:
			text = fmt.Sprintf("party positions %v", ev.Members)
		case botclient.ActionFailed:
			text = "action failed"
		case botclient.NpcHtmlMessage:
			text = fmt.Sprintf("page of %d", ev.ObjectID)
		}
		if text != "" {
			lines = append(lines, line{e.At, text})
		}
	}
	slices.SortStableFunc(lines, func(a, b line) int { return a.at.Compare(b.at) })
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var out []string
	for _, l := range lines {
		out = append(out, fmt.Sprintf("%s %s", l.at.Format("15:04:05.000"), l.text))
	}
	return strings.Join(out, "; ")
}

// junkBut is every item template but keep: a farmer whose Loot.Junk it is
// drops all it carries unequipped but keep.
func junkBut(keep ...int32) []int32 {
	// maxTemplate is past the datapack's highest item template id.
	const maxTemplate = 10000
	var junk []int32
	for id := int32(1); id < maxTemplate; id++ {
		if !slices.Contains(keep, id) {
			junk = append(junk, id)
		}
	}
	return junk
}

// gmGive gives f, the GM's target, count of template tmpl and waits until
// f's inventory shows them. A full inventory refuses a give: it asks again
// while f sheds its junk, for at most three minutes.
func gmGive(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, tmpl, count int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		seen := f.watch.Seen(0)
		var had int32
		for _, s := range seen {
			if n, _, ok := stock(s.Event, tmpl); ok {
				had = n
			}
		}
		if err := gm.Give(ctx, f.id, tmpl, count); err != nil {
			t.Fatal(err)
		}
		// Any rise counts, from what the stock was just before: a bot
		// selling or spending tmpl meanwhile lowers it first.
		wait, stop := context.WithTimeout(ctx, 15*time.Second)
		_, _, err := f.watch.Await(wait, len(seen), func(ev botclient.Event) bool {
			n, _, ok := stock(ev, tmpl)
			if !ok {
				return false
			}
			rose := n > had
			had = n
			return rose
		})
		stop()
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("%s never got %d of %d: %v", f.name, count, tmpl, ctx.Err())
		}
		t.Logf("%s did not get %d of %d (a full inventory?): again", f.name, count, tmpl)
	}
}

// gmTo teleports the GM next to f and waits until it sees f there.
func gmTo(t *testing.T, ctx context.Context, gm *GMClient, f *farmer) {
	t.Helper()
	if err := gm.Command(ctx, "teleportto "+f.name); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Fatalf("GM next to %s: %v", f.name, err)
	}
}

// gmTeleport teleports the GM to p and waits until it stands there.
func gmTeleport(t *testing.T, ctx context.Context, gm *GMClient, p botclient.Point) {
	t.Helper()
	if err := gm.Command(ctx, fmt.Sprintf("teleport %d %d %d", p.X, p.Y, p.Z)); err != nil {
		t.Fatal(err)
	}
	if _, err := gm.Await(ctx, EntryTimeout, teleportOf(gm.Self.ObjectID)); err != nil {
		t.Fatalf("GM teleport to %v: %v", p, err)
	}
}

// gmOn brings the GM next to f, selects it and runs cmds with f as its
// target: each one reselects f when the server cleared the GM's target
// (#206).
func gmOn(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, cmds ...string) {
	t.Helper()
	gmTo(t, ctx, gm, f)
	if err := gm.Select(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if err := gm.OnTarget(ctx, f.id, cmd); err != nil {
			t.Fatal(err)
		}
	}
}

// stock is the count of template tmpl an event reports, ok false when it
// reports none: an ItemList lists the whole inventory (0 when the template
// is missing), an InventoryUpdate only what changed.
func stock(ev botclient.Event, tmpl int32) (count int32, objectID int32, ok bool) {
	switch e := ev.(type) {
	case botclient.ItemList:
		for _, it := range e.Items {
			if it.TemplateID == tmpl {
				return it.Count, it.ObjectID, true
			}
		}
		return 0, 0, true
	case botclient.InventoryUpdate:
		for _, c := range e.Changes {
			if c.Item.TemplateID != tmpl {
				continue
			}
			if c.State == botclient.ItemRemoved {
				return 0, c.Item.ObjectID, true
			}
			return c.Item.Count, c.Item.ObjectID, true
		}
	}
	return 0, 0, false
}

// stockSpent reports whether, in seen, the count of tmpl fell below a
// count it had reached, and the template's object id.
func stockSpent(seen []Seen, tmpl int32) (spent bool, objectID int32) {
	var high int32
	for _, s := range seen {
		n, id, ok := stock(s.Event, tmpl)
		if !ok {
			continue
		}
		if id != 0 {
			objectID = id
		}
		if n < high {
			spent = true
		}
		high = max(high, n)
	}
	return spent, objectID
}

// maxPerSecond is the most packets sent within any one second.
func maxPerSecond(sent []Sent) int {
	most, from := 0, 0
	for i, s := range sent {
		for s.At.Sub(sent[from].At) >= time.Second {
			from++
		}
		most = max(most, i-from+1)
	}
	return most
}

// frameCap is the pacer's bound on frames in any second
// (botclient.Pacer: burst 10 plus rate 5), far under the server's 160.
const frameCap = 15

// until checks report every two seconds until it returns "" and fails the
// test with its last answer, what is still missing, once ctx ends.
func until(t *testing.T, ctx context.Context, what string, report func() string) {
	t.Helper()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		missing := report()
		if missing == "" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %v; missing: %s", what, ctx.Err(), missing)
		case <-tick.C:
		}
	}
}
