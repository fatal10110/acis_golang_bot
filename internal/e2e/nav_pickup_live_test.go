package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
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
// drop the bot cannot pick up straight: a wolf drops adena 70% of the time
// at drop rate 1 (npcs 20120, its CURRENCY category), about half of it on
// a closed spot, and the CURRENCY category rolls before the DROP and herb
// ones (killrewards.go CalculateRewards), so adena heads the drops: fifteen
// kills all miss about once in 600 runs.
const pickUpKills = 15

// TestLiveNavPickUpObstacle has a farmer whose one spot is centred at
// pickUpStand kill wolves the GM spawns one at a time at pickUpMob until
// one's first drop is an item (not a herb, which never reaches the
// inventory) where no straight walk reaches it from where the bot stood at
// the kill, from any point within half a cell of it. Only the first drop
// counts: the bot loots in drop order (Brain.lootEvent), so it judges a
// later drop from where it picked up the one before. The bot must not send
// the pick-up from there: the scenario asserts that it walked (ground moves
// after the kill) to a point from which the straight walk onto the item's
// raw Z holds (the origin its pick-up Action carries), that the server
// handed it the item (GetItem) and that the item reached its inventory. A
// drop it can pick up straight is picked up straight, and the next wolf
// spawned; runWatched holds its blocked stops to at most one and its
// desyncs to none. Like TestLiveFarmFight, all it carries but adena,
// potions and shots is junk dropped at any load, so loot kept over the runs
// never puts it over its weight limit.
func TestLiveNavPickUpObstacle(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
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
		until(t, ctx, "the farmer idle at its spot", func() string {
			if s := f.bot.Status(); s.Rule != "goal" && s.Rule != "idle" {
				return fmt.Sprintf("rule %q at %v", s.Rule, s.Position)
			}
			return ""
		})
		spawned := time.Now()
		gmTeleport(t, ctx, gm, m)
		mob, err := gm.Spawn(ctx, wolf)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
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
		gm.Forget(mob)
		k, ok := killOf(f.watch.Seen(0), f.id, mob)
		if !ok {
			t.Fatalf("%s killed wolf %d without an Attack frame of its own on it", f.name, kill)
		}
		at := ground(e, k.from)
		d, closed := closedHead(k.drops, func(q botclient.Point) bool {
			for _, j := range [][2]int32{{0, 0}, {8, 0}, {-8, 0}, {0, 8}, {0, -8}} {
				if canMove(ground(e, botclient.Point{X: at.X + j[0], Y: at.Y + j[1], Z: at.Z}), q) {
					return true
				}
			}
			return false
		})
		t.Logf("wolf %d (%v after the spawn): killed from %v, drops %v, closed %v", kill, time.Since(spawned).Round(time.Second), at, k.drops, closed)
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
		click, origin, ok := pickUpClick(f.watch.Sent()[k.sentAt:], d.Item.ObjectID)
		if !ok {
			t.Fatalf("%s got item %d without a pick-up Action on it", f.name, d.Item.ObjectID)
		}
		segs := 0
		for _, s := range f.watch.Sent()[k.sentAt:] {
			if s.Op == opMoveBackwardToLocation && s.At.Before(click.At) {
				segs++
			}
		}
		t.Logf("%s killed from %v, %d ground moves, picked item %d at %v up from %v at %v", f.name, at, segs, d.Item.ObjectID, d.Item.Pos, origin, got)
		if segs == 0 {
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

// closedHead is the first of drops when it is no herb and lies on a spot
// open reports no straight walk to. Only the first: the bot loots its
// drops in arrival order, so it heads for a later one from where it picked
// up the one before, not from the kill spot open measures from.
func closedHead(drops []botclient.DropItem, open func(botclient.Point) bool) (botclient.DropItem, bool) {
	if len(drops) == 0 {
		return botclient.DropItem{}, false
	}
	if d := drops[0]; !herb(d.Item.TemplateID) && !open(d.Item.Pos) {
		return d, true
	}
	return botclient.DropItem{}, false
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
		p := s.Payload
		at := func(off int) int32 { return int32(binary.LittleEndian.Uint32(p[off:])) }
		return s, botclient.Point{X: at(5), Y: at(9), Z: at(13)}, true
	}
	return Sent{}, botclient.Point{}, false
}
