package botclient

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The siege tests' ground, on the open navigation east and south of the
// origin (walkOn): castle 5's battlefield from x 1000 to 5000 across y 1000
// to 3000, a clan hall's battlefield far east, and an arena far west. The
// bot's clan is siegeClan.
const (
	siegeCastle = int32(5)
	siegeClan   = int32(7)
)

var (
	siegeEnemy = RelationChanged{ObjectID: pcID, Relation: botwire.RelationInSiege | botwire.RelationEnemy}
	siegeAlly  = RelationChanged{ObjectID: pcID, Relation: botwire.RelationInSiege | botwire.RelationAlly}
)

// siegeBrain is huntBrain with Siege mode on as set, in clan siegeClan,
// standing at self, over the siege tests' ground, walking on the open
// navigation.
func siegeBrain(t *testing.T, siege bool, self Point) *Brain {
	t.Helper()
	b := walkOn(t, huntBrain(t))
	b.persona.PvP.Siege = siege
	b.world.Clan.ID = siegeClan
	castle := box(1000, 1000, 5000, 3000)
	castle.ResidenceCastle = int(siegeCastle)
	hall := box(20000, 1000, 24000, 3000)
	hall.ResidenceClanHall = 34
	z := &b.cfg.Travel.zones
	z.Sieges = []datapack.Zone{hall, castle}
	z.Arenas = []datapack.Zone{box(-24000, 1000, -20000, 3000)}
	b.world.Self.move = stopAt(self)
	b.spot.held = false
	b.reactAt = time.Time{}
	return b
}

// siegeSide gives the bot the siege state of relation through its UserInfo
// relation field (the World's fields stay the fixture's).
func siegeSide(b *Brain, relation int32, now time.Time) {
	b.siegeEvent(UserInfo{ObjectID: selfID, Relation: relation}, now)
}

// siegeFoe stands pcID, named name, at p with relation.
func siegeFoeAt(t *testing.T, b *Brain, name string, p Point, rel RelationChanged) {
	t.Helper()
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: name, Pos: p, RunSpeed: 124, Running: true, CollisionRadius: 9}, t0)
	place(t, b, pcID, p)
	b.handleOne(t, rel, t0)
	clear(b.world.Attackers)
}

// TestPvPSiegeZoneNotice checks how 283 and 284 are told apart
// (docs/pvp/siege.md "Telling 283 and 284 apart"): inside an arena polygon
// they are the arena's, inside a siege polygon 283 blocks it until 284
// there or siegeLength, elsewhere they are ignored.
func TestPvPSiegeZoneNotice(t *testing.T) {
	entered, left := SystemMessage{ID: msgEnteredCombatZone}, SystemMessage{ID: msgLeftCombatZone}
	inside := Point{X: 3000, Y: 2000}
	t.Run("283 inside an arena polygon is the arena's", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: -22000, Y: 2000})
		b.handleOne(t, entered, t0)
		if len(b.pvp.siege.fields) != 0 {
			t.Fatalf("fields %v, want none", b.pvp.siege.fields)
		}
	})
	t.Run("283 where an arena and a battlefield overlap is the arena's", func(t *testing.T) {
		b := siegeBrain(t, false, inside)
		b.cfg.Travel.zones.Arenas = append(b.cfg.Travel.zones.Arenas, box(2000, 1500, 4000, 2500))
		b.handleOne(t, entered, t0)
		if len(b.pvp.siege.fields) != 0 {
			t.Fatalf("fields %v, want none", b.pvp.siege.fields)
		}
	})
	t.Run("283 inside a siege polygon: running there, blocked", func(t *testing.T) {
		b := siegeBrain(t, false, inside)
		b.handleOne(t, entered, t0)
		if until, ok := b.pvp.siege.fields[2]; len(b.pvp.siege.fields) != 1 || !ok || !until.Equal(t0.Add(siegeLength)) {
			t.Fatalf("fields %v, want 2 until %v", b.pvp.siege.fields, t0.Add(siegeLength))
		}
		if !b.siegeBlocks(Point{X: 1500, Y: 2000}, at(time.Second)) || b.duelGround(Point{X: 1500, Y: 2000}) {
			t.Fatal("ground inside not blocked")
		}
		if b.siegeBlocks(Point{X: 500, Y: 2000}, at(time.Second)) || !b.duelGround(Point{X: 500, Y: 2000}) {
			t.Fatal("ground outside blocked")
		}
		if b.siegeRunning(2, t0.Add(siegeLength)) {
			t.Fatal("the block outlasts siegeLength")
		}
	})
	t.Run("283 in neither: ignored", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 7000, Y: 2000})
		b.handleOne(t, entered, t0)
		if len(b.pvp.siege.fields) != 0 {
			t.Fatalf("fields %v, want none", b.pvp.siege.fields)
		}
	})
	t.Run("284 inside: the siege ended", func(t *testing.T) {
		b := siegeBrain(t, true, inside)
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		b.pvp.siege.castle = siegeCastle
		b.handleOne(t, entered, t0)
		b.handleOne(t, left, at(time.Second))
		if len(b.pvp.siege.fields) != 0 {
			t.Fatalf("fields %v after 284 inside, want none", b.pvp.siege.fields)
		}
	})
	t.Run("284 at the edge walking out: still running", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 1050, Y: 2000})
		b.handleOne(t, entered, t0)
		if rule, _ := b.decide(at(time.Second)); rule != "siege leave" {
			t.Fatalf("rule %q, want siege leave", rule)
		}
		b.handleOne(t, left, at(2*time.Second))
		if _, ok := b.pvp.siege.fields[2]; !ok {
			t.Fatalf("fields %v after 284 on the way out, want 2", b.pvp.siege.fields)
		}
	})
}

// TestPvPSiegeLeave checks a bot not taking part (docs/pvp/siege.md "Not
// taking part"): 283 inside a castle polygon walks it out by the nearest
// exit, no target picked, an attacker fought back; a spot inside waits.
func TestPvPSiegeLeave(t *testing.T) {
	entered := SystemMessage{ID: msgEnteredCombatZone}
	for _, tc := range []struct {
		name  string
		setup func(*Brain)
	}{
		{name: "no siege state"},
		{name: "Siege mode off, an attacker", setup: func(b *Brain) {
			b.persona.PvP.Siege = false
			siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
			b.pvp.siege.castle = siegeCastle
		}},
		{name: "another castle's attacker", setup: func(b *Brain) {
			siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
			b.pvp.siege.castle = 3
		}},
		{name: "an attacker past its deaths", setup: func(b *Brain) {
			siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
			b.pvp.siege.castle = siegeCastle
			for range siegeDeaths {
				b.diedPvP(t0)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := siegeBrain(t, true, Point{X: 4500, Y: 2000})
			if tc.setup != nil {
				tc.setup(b)
			}
			siegeFoeAt(t, b, foeName, Point{X: 4400, Y: 2300}, siegeEnemy)
			b.handleOne(t, entered, t0)
			rule, a := b.decide(at(time.Second))
			if rule != "siege leave" || a.kind != actMove {
				t.Fatalf("rule %q action %v, want siege leave's move", rule, a.kind)
			}
			if got := moveTarget(a.payload); got.X != 5000+siegeMargin || got.Y != 2000 {
				t.Fatalf("walks to %v, want the east exit (%d, 2000)", got, 5000+siegeMargin)
			}
			if b.mayFight(pcID, at(time.Second)) {
				t.Fatal("MayFight a participant that has not hit")
			}
			b.handleOne(t, hitSelf(pcID, 5), at(time.Second))
			place(t, b, pcID, Point{X: 4400, Y: 2300})
			b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(time.Second))
			if !b.mayFight(pcID, at(2*time.Second)) {
				t.Fatal("no MayFight on the attacker inside")
			}
			if rule, _ := b.decide(at(2 * time.Second)); rule != "attacked" {
				t.Fatalf("rule %q, want attacked", rule)
			}
		})
	}

	t.Run("the exits and entries: one per edge, the nearest first", func(t *testing.T) {
		b := siegeBrain(t, false, Point{})
		z, p := b.fieldZone(2), Point{X: 4500, Y: 2000}
		want := []Point{{X: 5300, Y: 2000}, {X: 4500, Y: 700}, {X: 4500, Y: 3300}, {X: 700, Y: 2000}}
		if got := siegeExits(z, p); !slices.Equal(got, want) {
			t.Fatalf("exits %v, want %v", got, want)
		}
		want = []Point{{X: 4600, Y: 2000}, {X: 4500, Y: 1400}, {X: 4500, Y: 2600}, {X: 1400, Y: 2000}}
		if got := siegeEntries(z, p); !slices.Equal(got, want) {
			t.Fatalf("entries %v, want %v", got, want)
		}
		ring := datapack.Zone{X: []int32{1000, 5000, 5000, 1000}, Y: []int32{1000, 1000, 3000, 3000}, MinZ: -1000, MaxZ: 1000}
		if got := siegeExits(ring, p); len(got) != 4 || got[0] != (Point{X: 5300, Y: 2000}) {
			t.Fatalf("polygon exits %v, want the east one first", got)
		}
	})

	t.Run("no way out latches: the later rules run until a teleport", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 4500, Y: 2000})
		b.walk.nav = nil // every walk ends unreachable
		b.handleOne(t, entered, t0)
		var leaves []int
		for i := 1; i <= 12; i++ {
			if rule, _ := b.decide(at(time.Duration(i) * time.Second)); rule == "siege leave" {
				leaves = append(leaves, i)
			}
		}
		if want := []int{1, 2, 3, 4}; !slices.Equal(leaves, want) {
			t.Fatalf("siege leave at decides %v, want %v (one per exit, then latched)", leaves, want)
		}
		if b.pvp.siege.stuck != 2 {
			t.Fatalf("stuck %d, want field 2", b.pvp.siege.stuck)
		}
		b.siegeTeleported()
		if rule, _ := b.decide(at(13 * time.Second)); rule != "siege leave" {
			t.Fatalf("after a teleport rule %q, want siege leave again", rule)
		}
	})

	t.Run("outside, a spot inside waits; another outside is open", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 3000, Y: 2000})
		b.handleOne(t, entered, t0)
		in := datapack.Spot{Center: datapack.Point{X: 3000, Y: 2000}}
		out := datapack.Spot{Center: datapack.Point{X: 500, Y: 2000}}
		if b.spotOpen(&in, at(time.Second)) || !b.spotOpen(&out, at(time.Second)) {
			t.Fatal("spot inside open or spot outside closed")
		}
		if !b.spotOpen(&in, t0.Add(siegeLength)) {
			t.Fatal("spot inside still closed after siegeLength")
		}
	})

	t.Run("a held spot inside is given back, another claimed", func(t *testing.T) {
		b := newSpotBrain(t, 3, testSpot("in", 400, 0, 1, 3, 9), testSpot("out", 0, 2000, 1, 3, 9))
		b.cfg.Travel = pvpTravel(false)
		b.cfg.Travel.zones.Sieges = []datapack.Zone{box(200, -300, 600, 300)}
		if rule, a := b.decide(at(time.Second)); rule != "spot" || a.kind != actMove || b.spot.idx != 0 {
			t.Fatalf("first decide = %q %d spot %d, want the walk to spot 0", rule, a.kind, b.spot.idx)
		}
		b.pvp.siege.fields = map[int]time.Time{1: t0.Add(siegeLength)} // a 283 there
		rule, a := b.decide(at(2 * time.Second))
		if rule != "spot" || a.kind != actMove || b.spot.idx != 1 || b.cfg.Spots.Held(0) != 0 {
			t.Fatalf("decide = %q %d spot %d, spot 0 held %d, want spot 0 given back and the walk to spot 1", rule, a.kind, b.spot.idx, b.cfg.Spots.Held(0))
		}
	})

	t.Run("a defender inside at the start stays", func(t *testing.T) {
		b := siegeBrain(t, true, Point{X: 3000, Y: 2000})
		b.handleOne(t, entered, t0)
		siegeSide(b, botwire.UserRelationSiegeDefender, t0)
		if rule, a := b.decide(at(time.Second)); rule != "siege" || a.kind != actSiegeList {
			t.Fatalf("rule %q action %v, want the defender list asked, no leave", rule, a.kind)
		}
		b.handleOne(t, SiegeDefenderList{CastleID: siegeCastle, Clans: []botwire.SiegeClan{{ClanID: siegeClan, Side: botwire.SiegeDefenderOwner}}}, at(time.Second))
		if rule, a := b.decide(at(2 * time.Second)); rule != "siege" || a.kind != actHold {
			t.Fatalf("rule %q action %v, want the siege's hold inside", rule, a.kind)
		}
	})

	t.Run("the flag guard is off inside", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 3000, Y: 2000})
		b.handleOne(t, entered, t0)
		player(t, b, pcID, foeName, Point{X: 3100, Y: 2000}, 2, 0, 9)
		if b.blinking(pcID, b.world.object(pcID), at(time.Second)) {
			t.Fatal("flag guard holds on a blinking player inside the battlefield")
		}
		if b.forced(pcID, at(time.Second)) {
			t.Fatal("force needed inside the battlefield")
		}
	})

	t.Run("no notice: the bot walks nowhere", func(t *testing.T) {
		b := siegeBrain(t, false, Point{X: 3000, Y: 2000})
		if rule, _ := b.decide(at(time.Second)); rule == "siege leave" {
			t.Fatal("left a battlefield with no 283")
		}
	})
}

// listSends runs the bot's decisions every 100 ms from from for d,
// sending the siege list requests through the pacer, and returns them with
// their times; answer, if set, answers each as the server would.
func listSends(t *testing.T, b *Brain, from time.Time, d time.Duration, answer func(castle int32, defender bool) Event) (frames [][]byte, times []time.Time) {
	t.Helper()
	for now := from; now.Before(from.Add(d)); now = now.Add(100 * time.Millisecond) {
		rule, a := b.decide(now)
		if rule != "siege" || a.kind != actSiegeList {
			continue
		}
		sent, err := b.send(a, now)
		if err != nil {
			t.Fatal(err)
		}
		if !sent {
			continue
		}
		frames, times = append(frames, a.payload), append(times, now)
		if answer != nil {
			castle := int32(a.payload[1])
			b.handleOne(t, answer(castle, a.payload[0] == botwire.OpGameRequestSiegeDefenderList), now)
		}
	}
	return frames, times
}

// TestPvPSiegeFight checks a bot taking part (docs/pvp/siege.md "Taking
// part"): its side's list asked per castle paced by bypassGap, the walk in
// after the start, its targets, the fifth death and the siege's end.
func TestPvPSiegeFight(t *testing.T) {
	entered := SystemMessage{ID: msgEnteredCombatZone}
	t.Run("the lists: its side's, paced, until its clan shows", func(t *testing.T) {
		b := siegeBrain(t, true, Point{})
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		frames, times := listSends(t, b, t0, 10*time.Second, func(castle int32, _ bool) Event {
			l := SiegeAttackerList{CastleID: castle, Clans: []botwire.SiegeClan{{ClanID: 99}}}
			if castle == siegeCastle {
				l.Clans = append(l.Clans, botwire.SiegeClan{ClanID: siegeClan})
			}
			return l
		})
		if len(frames) != int(siegeCastle) {
			t.Fatalf("%d list requests, want %d (castles 1 to 5)", len(frames), siegeCastle)
		}
		for i, f := range frames {
			if want := botwire.EncodeRequestSiegeAttackerList(int32(i + 1)); !bytes.Equal(f, want) {
				t.Fatalf("request %d = % x, want % x", i, f, want)
			}
			if i > 0 && times[i].Sub(times[i-1]) < bypassGap {
				t.Fatalf("requests %d and %d %v apart, under bypassGap", i-1, i, times[i].Sub(times[i-1]))
			}
		}
		if b.pvp.siege.castle != siegeCastle {
			t.Fatalf("castle %d, want %d", b.pvp.siege.castle, siegeCastle)
		}
	})
	t.Run("a defender asks the defender lists, a waiting clan is not its own", func(t *testing.T) {
		b := siegeBrain(t, true, Point{})
		siegeSide(b, botwire.UserRelationSiegeDefender, t0)
		frames, _ := listSends(t, b, t0, 10*time.Second, func(castle int32, defender bool) Event {
			if !defender {
				t.Fatal("a defender asked an attacker list")
			}
			side := botwire.SiegeDefenderPending
			if castle == 8 {
				side = botwire.SiegeDefenderApproved
			}
			return SiegeDefenderList{CastleID: castle, Clans: []botwire.SiegeClan{{ClanID: siegeClan, Side: side}}}
		})
		if len(frames) != 8 || b.pvp.siege.castle != 8 {
			t.Fatalf("%d requests, castle %d, want 8 and castle 8", len(frames), b.pvp.siege.castle)
		}
	})
	t.Run("no list holds its clan: nine requests, then no part", func(t *testing.T) {
		b := siegeBrain(t, true, Point{X: 3000, Y: 2000})
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		frames, _ := listSends(t, b, t0, 20*time.Second, func(castle int32, _ bool) Event {
			return SiegeAttackerList{CastleID: castle}
		})
		if len(frames) != siegeCastles || !b.pvp.siege.quit {
			t.Fatalf("%d requests, quit %v, want %d and quit", len(frames), b.pvp.siege.quit, siegeCastles)
		}
		b.handleOne(t, entered, at(21*time.Second))
		if rule, _ := b.decide(at(22 * time.Second)); rule != "siege leave" {
			t.Fatalf("rule %q, want siege leave (a clan hall's siege is left)", rule)
		}
	})
	t.Run("unanswered lists: no part after siegeListWait", func(t *testing.T) {
		b := siegeBrain(t, true, Point{})
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		frames, times := listSends(t, b, t0, (siegeCastles-1)*bypassGap+time.Millisecond, nil)
		last := times[len(times)-1]
		if len(frames) != siegeCastles {
			t.Fatalf("%d requests, want %d", len(frames), siegeCastles)
		}
		b.decide(last.Add(siegeListWait - time.Millisecond))
		if b.pvp.siege.quit {
			t.Fatal("quit before siegeListWait")
		}
		b.decide(last.Add(siegeListWait))
		if !b.pvp.siege.quit {
			t.Fatal("still waiting after siegeListWait")
		}
	})
	t.Run("Siege mode off: no list", func(t *testing.T) {
		b := siegeBrain(t, false, Point{})
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		if frames, _ := listSends(t, b, t0, 10*time.Second, nil); len(frames) != 0 {
			t.Fatalf("%d list requests with Siege off", len(frames))
		}
	})

	// taking is a bot of castle 5's attacking clan, its castle found.
	taking := func(t *testing.T, self Point) *Brain {
		b := siegeBrain(t, true, self)
		siegeSide(b, botwire.UserRelationSiegeAttacker, t0)
		b.handleOne(t, SiegeAttackerList{CastleID: siegeCastle, Clans: []botwire.SiegeClan{{ClanID: siegeClan}}}, t0)
		return b
	}
	t.Run("walks in after the start", func(t *testing.T) {
		b := taking(t, Point{X: 500, Y: 2000})
		rule, a := b.decide(at(time.Second))
		if rule != "siege" || a.kind != actMove {
			t.Fatalf("rule %q action %v, want the siege's walk in", rule, a.kind)
		}
		if got := moveTarget(a.payload); got.X != 1000+siegeDepth || got.Y != 2000 {
			t.Fatalf("walks to %v, want (%d, 2000)", got, 1000+siegeDepth)
		}
		b.world.Self.move = stopAt(Point{X: 1000 + siegeDepth, Y: 2000})
		b.handleOne(t, entered, at(2*time.Second))
		if rule, a := b.decide(at(3 * time.Second)); rule != "siege" || a.kind != actHold {
			t.Fatalf("rule %q action %v inside, want the siege's hold", rule, a.kind)
		}
	})

	inside := Point{X: 3000, Y: 2000}
	for _, tc := range []struct {
		name  string
		foe   string
		at    Point
		evs   []Event
		fight bool
	}{
		{name: "a defender showing Enemy", evs: []Event{siegeEnemy}, fight: true},
		{name: "a defender clan leader", evs: []Event{RelationChanged{ObjectID: pcID, Relation: siegeEnemy.Relation | 0x80}}, fight: true},
		{name: "an ally", evs: []Event{siegeAlly}},
		{name: "an attacker showing a stale Enemy", evs: []Event{RelationChanged{ObjectID: pcID, Relation: botwire.RelationInSiege | botwire.RelationAttacker | botwire.RelationEnemy}}},
		{name: "a non-participant", evs: []Event{RelationChanged{ObjectID: pcID}}},
		{name: "a non-participant that hits", evs: []Event{RelationChanged{ObjectID: pcID}, hitSelf(pcID, 5)}, fight: true},
		{name: "a bot defender that has not hit", foe: markedName, evs: []Event{siegeEnemy}},
		{name: "a bot defender that hits", foe: markedName, evs: []Event{siegeEnemy, hitSelf(pcID, 5)}, fight: true},
		{name: "a defender outside the polygon", at: Point{X: 900, Y: 2000}, evs: []Event{siegeEnemy}},
		{name: "a defender within arenaEdge of the edge", at: Point{X: 1100, Y: 2000}, evs: []Event{siegeEnemy}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := taking(t, inside)
			b.handleOne(t, entered, t0)
			name, p := tc.foe, tc.at
			if name == "" {
				name = foeName
			}
			if p == (Point{}) {
				p = Point{X: 3000, Y: 2300}
			}
			siegeFoeAt(t, b, name, p, RelationChanged{ObjectID: pcID})
			for _, ev := range tc.evs {
				b.handleOne(t, ev, t0)
			}
			place(t, b, pcID, p) // an Attack moves its attacker to its own position
			if got := b.mayFight(pcID, at(time.Second)); got != tc.fight {
				t.Fatalf("MayFight %v, want %v", got, tc.fight)
			}
		})
	}

	t.Run("the nearest enemy picked, a siege fight", func(t *testing.T) {
		b := taking(t, inside)
		b.handleOne(t, entered, t0)
		siegeFoeAt(t, b, foeName, Point{X: 3000, Y: 2300}, siegeEnemy)
		rule, a := b.decide(at(time.Second))
		if rule != "siege fight" || a.object != pcID || a.kind != actSelect {
			t.Fatalf("rule %q action %v on %d, want the siege fight's selection of %d", rule, a.kind, a.object, pcID)
		}
		if k := b.pvp.opponents[pcID].Kind; k != FightSiege || k.String() != "siege" {
			t.Fatalf("fight kind %v, want siege", k)
		}
		if b.forced(pcID, at(time.Second)) {
			t.Fatal("force needed on an enemy inside the battlefield")
		}
	})

	t.Run("the fifth death: its spot", func(t *testing.T) {
		b := taking(t, inside)
		for i := range siegeDeaths {
			if b.pvp.siege.quit {
				t.Fatalf("quit after %d deaths", i)
			}
			b.diedPvP(t0)
		}
		if !b.pvp.siege.quit {
			t.Fatal("still taking part after the fifth death")
		}
	})

	t.Run("the siege's end: the state cleared, the battlefield open", func(t *testing.T) {
		b := taking(t, inside)
		b.handleOne(t, entered, t0)
		siegeSide(b, 0, at(time.Second))
		if len(b.pvp.siege.fields) != 0 || b.pvp.siege.castle != 0 {
			t.Fatalf("fields %v castle %d after the end, want none", b.pvp.siege.fields, b.pvp.siege.castle)
		}
		if rule, _ := b.decide(at(2 * time.Second)); rule == "siege" || rule == "siege leave" || rule == "siege fight" {
			t.Fatalf("rule %q after the end", rule)
		}
	})
}
