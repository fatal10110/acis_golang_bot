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
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/geo/engine"
)

// The kill site of TestLiveNavPickUpObstacle, on Talking Island west of the
// human mage start and outside its peace zone: a wolf spawned at
// pickUpMob, which a bot coming from pickUpStand (120 away, an open line
// either way) fights from that side, stands against geodata that closes
// the straight walk from the bot's kill spot onto about half of the
// server's drop spots. A kill drops each item at a random offset of up to
// 70 on either axis from the corpse, clipped by ValidLocation from the
// corpse (server data/manager/killrewards.go KillReward.place), so no site
// closes every drop spot of a melee kill: a step blocked at once leaves the
// item on the corpse, which the bot reached straight. Measured with the
// bot's geodata over every scatter offset (pickUpScatterClosed): 63% closed
// from the kill spot 38 short of the wolf on its line to pickUpStand, and
// at least 49% from any kill spot 25 to 60 short of it with the wolf up to
// a cell off pickUpMob.
var (
	pickUpStand = botclient.Point{X: -87197, Y: 247126, Z: -2992}
	pickUpMob   = botclient.Point{X: -87152, Y: 247016, Z: -2936}
)

// dropScatter is the server's dropScatterOffset: a kill's item lands up to
// this far from the corpse on either axis (killrewards.go).
const dropScatter = 70

// Pick-up refusals (network/pickup.go failedPickupFrame and the trade
// lock), with msgWeightLimitExceeded and msgSlotsFull.
const (
	msgFailedToPickUpAdena int32 = 55
	msgFailedToPickUpS1    int32 = 56
	msgFailedToPickUpS2S1S int32 = 57
	msgCannotPickUpTrading int32 = 149
)

// pickUpScatterClosed is the share of the server's drop spots around a
// corpse at m, every offset of the scatter clipped by ValidLocation from m,
// that have no straight walk from a bot at from.
func pickUpScatterClosed(e *engine.Engine, m, from botclient.Point) float64 {
	n, closed := 0, 0
	for dx := -dropScatter; dx <= dropScatter; dx += 2 {
		for dy := -dropScatter; dy <= dropScatter; dy += 2 {
			s := e.ValidLocation(int(m.X), int(m.Y), int(m.Z), int(m.X)+dx, int(m.Y)+dy, int(m.Z))
			n++
			if !e.CanMove(int(from.X), int(from.Y), int(from.Z), s.X, s.Y, s.Z) {
				closed++
			}
		}
	}
	return float64(closed) / float64(n)
}

// pickUpKills bounds the wolves TestLiveNavPickUpObstacle kills for one
// drop the bot turns to from a spot it cannot pick it up straight from.
// Every non-herb drop of a kill is a trial (lootTurns), and the live server
// drops four to six items per wolf, adena always first (the CURRENCY
// category rolls before the DROP and herb ones, killrewards.go
// CalculateRewards). Measured over seven live runs: a hit in 7 of the 9
// kills the bot looted. At half that rate fifteen kills all miss about once
// in 1600 runs.
const pickUpKills = 15

// pickUpLootFor is how long TestLiveNavPickUpObstacle waits for the bot to
// turn to every drop of a kill before it spawns the next wolf.
const pickUpLootFor = 45 * time.Second

// pickUpStandReach is how far from pickUpStand the bot may stand when the
// next wolf is spawned: on the spot's floor, never down the ledge.
const pickUpStandReach = 60

// TestLiveNavPickUpObstacle has a farmer whose one spot is centred at
// pickUpStand kill wolves the GM spawns one at a time at pickUpMob until it
// turns to a drop (not a herb, which never reaches the inventory) where no
// straight walk reaches it from where the bot stood when it turned, from
// any point within half a cell of it. The bot loots one drop at a time
// (Brain.ruleLoot), so it turns to the first from its kill spot and to each
// next one from where the server walked it onto the one before; the
// scenario reads that spot from the bot's own packets (lootTurns), so every
// drop of a kill is a trial. The bot must not send the pick-up from there:
// the scenario asserts that it walked (ground moves before its Action on
// the item) to a point from which the straight walk onto the item's raw Z
// holds (the origin its pick-up Action carries), that the server handed it
// the item (GetItem) and that the item reached its inventory. A drop it can
// pick up straight is picked up straight, and the next wolf spawned once
// the kill is looted; runWatched holds its blocked stops to at most one and
// its desyncs to none. Like TestLiveFarmFight, all it carries but adena,
// potions and shots is junk dropped at any load, so loot kept over the runs
// never puts it over its weight limit.
func TestLiveNavPickUpObstacle(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	// Fifteen kills at about 40 s each and up to pickUpLootFor of loot.
	ctx := scenarioCtx(t, 30*time.Minute)
	e := tg.Nav(t).Engine
	a, m := ground(e, pickUpStand), ground(e, pickUpMob)
	canMove := func(p, q botclient.Point) bool {
		return e.CanMove(int(p.X), int(p.Y), int(p.Z), int(q.X), int(q.Y), int(q.Z))
	}
	if !canMove(a, m) || !canMove(m, a) {
		t.Fatalf("no straight walk between %v and %v: the bot would not fight from that side", a, m)
	}
	// The kill spot the measure assumes: 38 short of the wolf on its line
	// to the spot's centre.
	o := ground(e, botclient.Point{X: m.X + (a.X-m.X)*38/120, Y: m.Y + (a.Y-m.Y)*38/120, Z: m.Z})
	if c := pickUpScatterClosed(e, m, o); c < 0.5 {
		t.Fatalf("from %v only %.0f%% of the drop spots round %v are closed: the site no longer measures", o, 100*c, m)
	}
	spot := datapack.Spot{
		Maker: "e2e_pickup",
		Territory: &datapack.Territory{Name: "e2e_pickup", MinZ: a.Z - 100, MaxZ: a.Z + 100, Nodes: []datapack.Point{
			{X: a.X - 300, Y: a.Y - 300}, {X: a.X + 300, Y: a.Y - 300}, {X: a.X + 300, Y: a.Y + 300}, {X: a.X - 300, Y: a.Y + 300},
		}},
		Center:   datapack.Point{X: a.X, Y: a.Y},
		Z:        a.Z,
		MinLevel: 1, MaxLevel: 3, MaximumNPCs: 1,
	}
	spots := botclient.NewSpotRegistry([]datapack.Spot{spot}, 1)
	gm := enterGM(t, ctx, tg)
	var f *farmer
	t.Cleanup(func() { forgetKilled(gm, f) })
	account, _ := tg.Account("pu", 1)
	gmTeleport(t, ctx, gm, a)
	f = runFarmer(t, ctx, tg, account, spots, func(p *botclient.Persona) {
		p.Spots.Leash = 200
		p.Loot.Junk = junkBut(slices.Concat([]int32{adena, soulshotNG}, p.Potions)...)
		p.Loot.DropAbove = 1
		// The shots the GM gives every run pile up (4 each): the pinned
		// character carries over the default pickup cap of 65 %, and the
		// scenario checks the pickup, not the cap.
		p.Loot.StopAbove = 0
	})
	recallBot(t, ctx, tg, gm, f.name)
	botLanded(t, ctx, gm, f.bot, gm.Pos())
	gmOn(t, ctx, gm, f, "set level 5")
	// A character over its weight limit has speed 0, and the server
	// refuses its every move (movement.go moveLivePlayer).
	if u, ok := lastUserInfo(f.watch.Seen(0)); !ok || u.RunSpeed == 0 || u.MaxLoad == 0 || u.Load >= u.MaxLoad {
		t.Fatalf("%s cannot walk: run speed %d, load %d/%d", f.name, u.RunSpeed, u.Load, u.MaxLoad)
	}
	for kill := 1; ; kill++ {
		if kill > pickUpKills {
			t.Fatalf("%d wolves killed, no first drop on a spot closed from the kill spot", pickUpKills)
		}
		idle := func() {
			until(t, ctx, "the farmer idle at its spot", func() string {
				if s := f.bot.Status(); s.Rule != "goal" && s.Rule != "idle" {
					return fmt.Sprintf("rule %q at %v", s.Rule, s.Position)
				}
				return ""
			})
		}
		idle()
		// A pick-up the server walked down the ledge can leave the bot
		// where its own navigation finds no way out (#278): every next
		// wolf would be fought from elsewhere than the site measures, or
		// not at all. The GM stands at a and recalls it there.
		if at := f.bot.Status().Position; dist(at, a) > pickUpStandReach {
			t.Logf("%s at %v before wolf %d, off the spot's floor: recalled to %v", f.name, at, kill, a)
			recallBot(t, ctx, tg, gm, f.name)
			botLanded(t, ctx, gm, f.bot, gm.Pos())
			idle()
		}
		spawned := time.Now()
		gmTeleport(t, ctx, gm, m)
		mob, err := gm.Spawn(ctx, wolf)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
		// A spawned wolf wanders up to the server's MaxDriftRange (200)
		// from where it stands (hostile_wander.go), off the measured site
		// and down the ledge; paralysed it stays at m until it dies.
		if err := gm.OnTarget(ctx, mob, "para"); err != nil {
			t.Fatal(err)
		}
		gmTeleport(t, ctx, gm, a)
		kctx, kstop := context.WithTimeout(ctx, 3*time.Minute)
		until(t, kctx, "the wolf killed", func() string {
			if readFight(f, mob).spawnKilled {
				return ""
			}
			s := f.bot.Status()
			return fmt.Sprintf("no kill of wolf %d yet: rule %q walk %q at %v", kill, s.Rule, s.Walk, s.Position)
		})
		kstop()
		killed := time.Now()
		gm.Forget(mob)
		k, ok := killOf(f.watch.Seen(0), f.id, mob)
		if !ok {
			t.Fatalf("%s killed wolf %d without an Attack frame of its own on it", f.name, kill)
		}
		var (
			d      botclient.DropItem
			turn   lootTurn
			closed bool
			turns  []lootTurn
		)
		lctx, lstop := context.WithTimeout(ctx, 2*time.Minute)
		until(t, lctx, "a drop behind the obstacle, or the kill looted", func() string {
			turns = lootTurns(f.watch.Sent()[k.sentAt:])
			if d, turn, closed = closedTurn(turns, k.drops, func(from, q botclient.Point) bool { return closedFrom(e, from, q) }); closed {
				return ""
			}
			// A kill another player finished leaves its drops to that
			// player: the bot may loot none of them.
			s := f.bot.Status()
			if (s.Rule == "goal" || s.Rule == "idle") && (clickedAll(turns, k.drops) || time.Since(killed) > pickUpLootFor) {
				return ""
			}
			return fmt.Sprintf("wolf %d: %d turns for %d drops: rule %q walk %q at %v", kill, len(turns), len(k.drops), s.Rule, s.Walk, s.Position)
		})
		lstop()
		var seen strings.Builder
		for _, lt := range turns {
			fmt.Fprintf(&seen, " [item %d from %v, %d moves]", lt.target, lt.from, lt.moves)
		}
		t.Logf("wolf %d (%v after the spawn): killed from %v, drops %v, turns%s, closed %v", kill, time.Since(spawned).Round(time.Second), k.from, k.drops, seen.String(), closed)
		if !closed {
			continue
		}
		untilServed(t, ctx, "the drop behind the obstacle in the inventory", f.watch, RefusalCheck{
			From:     k.sentAt,
			Packet:   func(s Sent) bool { return s.Op == opAction && s.Target == d.Item.ObjectID },
			Messages: []int32{msgFailedToPickUpAdena, msgFailedToPickUpS1, msgFailedToPickUpS2S1S, msgCannotPickUpTrading, msgWeightLimitExceeded, msgSlotsFull},
		}, func() string {
			if _, ok := pickedUp(f.watch.Seen(0), f.id, d); ok {
				return ""
			}
			s := f.bot.Status()
			return fmt.Sprintf("item %d (template %d) at %v not picked up: rule %q walk %q at %v", d.Item.ObjectID, d.Item.TemplateID, d.Item.Pos, s.Rule, s.Walk, s.Position)
		})
		got, _ := pickedUp(f.watch.Seen(0), f.id, d)
		origin, _ := actionOrigin(turn.click)
		t.Logf("%s turned to item %d at %v from %v, %d ground moves, picked it up from %v at %v", f.name, d.Item.ObjectID, d.Item.Pos, turn.from, turn.moves, origin, got)
		if turn.moves == 0 {
			t.Errorf("%s sent the pick-up of item %d at %v from %v without a ground move round the obstacle", f.name, d.Item.ObjectID, d.Item.Pos, origin)
		}
		if !canMove(ground(e, origin), d.Item.Pos) {
			t.Errorf("%s sent the pick-up of item %d at %v from %v: no straight walk, no stand point", f.name, d.Item.ObjectID, d.Item.Pos, origin)
		}
		return
	}
}

// wolfKill is what a farmer's frames show of one kill: where it stood at
// its last swing on the monster before the Die, the monster's drops, and
// how many packets it had sent by the Die.
type wolfKill struct {
	from   botclient.Point
	drops  []botclient.DropItem
	sentAt int
}

// killOf reads seen for the kill of mob by self: the position of self's
// last Attack frame on mob before mob's Die (the server's own record of
// where it stood), and mob's drops, which the server sends before the Die.
func killOf(seen []Seen, self, mob int32) (wolfKill, bool) {
	var k wolfKill
	swung := false
	for _, s := range seen {
		switch e := s.Event.(type) {
		case botclient.Attack:
			if e.AttackerID == self && len(e.Hits) > 0 && e.Hits[0].TargetID == mob {
				k.from, swung = e.Pos, true
			}
		case botclient.DropItem:
			if e.DropperID == mob {
				k.drops = append(k.drops, e)
			}
		case botclient.Die:
			if e.ObjectID == mob {
				k.sentAt = s.SentBefore
				return k, swung
			}
		}
	}
	return wolfKill{}, false
}

// closedFrom reports that no straight walk reaches q from the ground at
// from, nor from any point half a cell off it on either axis.
func closedFrom(e *engine.Engine, from, q botclient.Point) bool {
	at := ground(e, from)
	for _, j := range [][2]int32{{0, 0}, {8, 0}, {-8, 0}, {0, 8}, {0, -8}} {
		p := ground(e, botclient.Point{X: at.X + j[0], Y: at.Y + j[1], Z: at.Z})
		if e.CanMove(int(p.X), int(p.Y), int(p.Z), int(q.X), int(q.Y), int(q.Z)) {
			return false
		}
	}
	return true
}

// lootTurn is the bot turning to one target after a kill: where it stood
// when it turned (the origin of its first ground move or Action), how many
// ground moves it sent before its first Action on the target, and that
// Action.
type lootTurn struct {
	target int32
	from   botclient.Point
	moves  int
	click  Sent
}

// lootTurns splits sent, the bot's packets after a kill, into its turns to
// one target each: ground moves, then Actions on the target. The bot loots
// one drop at a time and turns to the next only once the server walked it
// onto the last (Brain.ruleLoot), so each turn starts where the bot stood
// at the drop before. Moves followed by an Action on the target of the turn
// before are a retried click of that turn; moves with no Action after them
// yet are a turn under way and left out.
func lootTurns(sent []Sent) []lootTurn {
	var (
		turns []lootTurn
		moves []Sent
	)
	for _, s := range sent {
		switch s.Op {
		case opMoveBackwardToLocation:
			moves = append(moves, s)
		case opAction:
			if n := len(turns); n > 0 && turns[n-1].target == s.Target {
				moves = nil
				continue
			}
			lt := lootTurn{target: s.Target, moves: len(moves), click: s}
			lt.from, _ = actionOrigin(s)
			if len(moves) > 0 {
				if _, from, ok := groundClick(moves[0].Payload); ok {
					lt.from = from
				}
			}
			turns = append(turns, lt)
			moves = nil
		}
	}
	return turns
}

// closedTurn is the first of turns onto one of drops, herbs left out, whose
// item lies where closed reports no straight walk to from the spot the bot
// turned to it from.
func closedTurn(turns []lootTurn, drops []botclient.DropItem, closed func(from, q botclient.Point) bool) (botclient.DropItem, lootTurn, bool) {
	for _, lt := range turns {
		i := slices.IndexFunc(drops, func(d botclient.DropItem) bool { return d.Item.ObjectID == lt.target })
		if i >= 0 && !herb(drops[i].Item.TemplateID) && closed(lt.from, drops[i].Item.Pos) {
			return drops[i], lt, true
		}
	}
	return botclient.DropItem{}, lootTurn{}, false
}

// clickedAll reports that turns hold a turn onto every one of drops.
func clickedAll(turns []lootTurn, drops []botclient.DropItem) bool {
	for _, d := range drops {
		if !slices.ContainsFunc(turns, func(lt lootTurn) bool { return lt.target == d.Item.ObjectID }) {
			return false
		}
	}
	return true
}

// pickedUp reports when self got the ground item of d: its GetItem by self,
// then an InventoryUpdate adding or changing d's template.
func pickedUp(seen []Seen, self int32, d botclient.DropItem) (time.Time, bool) {
	got := false
	for _, s := range seen {
		switch e := s.Event.(type) {
		case botclient.GetItem:
			got = got || e.PickerID == self && e.ObjectID == d.Item.ObjectID
		case botclient.InventoryUpdate:
			if got && slices.ContainsFunc(e.Changes, func(c botclient.ItemChange) bool {
				return c.State != botclient.ItemRemoved && c.Item.TemplateID == d.Item.TemplateID
			}) {
				return s.At, true
			}
		}
	}
	return time.Time{}, false
}

// pickUpClick is the first Action on ground item id among sent, and the
// origin it carries (clientpackets Action: object id, origin, shift).
func pickUpClick(sent []Sent, id int32) (Sent, botclient.Point, bool) {
	for _, s := range sent {
		if s.Op != opAction || s.Target != id || len(s.Payload) < 17 {
			continue
		}
		o, _ := actionOrigin(s)
		return s, o, true
	}
	return Sent{}, botclient.Point{}, false
}

// actionOrigin is the origin an Action carries (clientpackets Action:
// object id, origin, shift).
func actionOrigin(s Sent) (botclient.Point, bool) {
	p := s.Payload
	if s.Op != opAction || len(p) < 17 {
		return botclient.Point{}, false
	}
	at := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
	return botclient.Point{X: at(5), Y: at(9), Z: at(13)}, true
}
