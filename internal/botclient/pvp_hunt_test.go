package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// huntBrain is findBrain hunting, with the Warrior's (1) and Knight's (4)
// templates too.
func huntBrain(t *testing.T) *Brain {
	t.Helper()
	b := findBrain(t, func(p *Persona) { p.PvP.Hunt, p.PvP.Arenas = true, true })
	b.cfg.Travel.classes[1] = withVitals(datapack.Class{ID: 1, Parent: 0, CON: 43, MEN: 25, PAtk: 4, PDef: 72, MAtk: 2, MDef: 47})
	b.cfg.Travel.classes[4] = withVitals(datapack.Class{ID: 4, Parent: 0, CON: 43, MEN: 25, PAtk: 4, PDef: 72, MAtk: 2, MDef: 47})
	return b
}

// fightsWith reports whether the bot's decision at now by rule acts on id.
func fightsWith(b *Brain, rule string, id int32, now time.Time) bool {
	got, a := b.decide(now)
	return got == rule && a.object == id && (a.kind == actSelect || a.kind == actAttack || a.kind == actCast)
}

// TestPvPHuntFlagged checks hunting (docs/pvp/targets.md "Hunting").
func TestPvPHuntFlagged(t *testing.T) {
	around := box(-500, -500, 500, 500)
	for _, tc := range []struct {
		name  string
		foe   CharInfo
		setup func(*Brain)
		hunt  bool
	}{
		{name: "flagged of its tier", foe: CharInfo{PvPFlag: 1}, hunt: true},
		{name: "karma of its tier", foe: CharInfo{Karma: 100}, hunt: true},
		{name: "flagged and clanless, the bot clanless", foe: CharInfo{PvPFlag: 1, ClanID: 0}, hunt: true},
		{name: "flagged party member", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) {
			b.world.Party.Members = map[int32]*PartyMate{pcID: {PartyMember: PartyMember{Name: foeName}}}
		}},
		{name: "flagged clan mate", foe: CharInfo{PvPFlag: 1, ClanID: 7}, setup: func(b *Brain) { b.world.Clan.ID = 7 }},
		{name: "flagged ally", foe: CharInfo{PvPFlag: 1, ClanID: 8, AllyID: 3}, setup: func(b *Brain) { b.world.Clan.ID, b.world.Self.AllyID = 7, 3 }},
		{name: "flagged of another tier", foe: CharInfo{PvPFlag: 1, ClassID: 1}},
		{name: "clean", foe: CharInfo{}},
		{name: "the bot in a peace zone", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) { b.cfg.Travel.zones.Peace = []datapack.Zone{around} }},
		{name: "the bot in a town", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) { b.cfg.Travel.zones.Towns = []datapack.Zone{around} }},
		{name: "the bot on the Derby track", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) { b.cfg.Travel.zones.DerbyTracks = []datapack.Zone{around} }},
		{name: "the player in a town", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) {
			b.cfg.Travel.zones.Towns = []datapack.Zone{box(-100, 200, 100, 400)}
		}},
		{name: "MP under its stand level", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) { b.world.Self.MP = 50 }},
		{name: "hunting off", foe: CharInfo{PvPFlag: 1}, setup: func(b *Brain) { b.persona.PvP.Hunt = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := huntBrain(t)
			if tc.setup != nil {
				tc.setup(b)
			}
			foe := tc.foe
			foe.ObjectID, foe.Name, foe.Pos = pcID, foeName, Point{Y: 300}
			b.handleOne(t, foe, t0)
			if got := fightsWith(b, "hunt", pcID, at(time.Second)); got != tc.hunt {
				t.Fatalf("hunted %v, want %v", got, tc.hunt)
			}
		})
	}
	t.Run("a flagged third player in its own duel", func(t *testing.T) {
		b := huntBrain(t)
		b.handleOne(t, CharInfo{ObjectID: findOther, Name: otherName, Pos: Point{X: 300}, PvPFlag: 1}, t0)
		duelUntil(t, b, DuelFighting, at(time.Second))
		_, a := b.decide(at(7 * time.Second))
		if a.object == findOther {
			t.Fatalf("acted on the flagged third player (%v) in its duel", a.kind)
		}
	})
}

// TestPvPBotsOnlyInArena checks D38: a bot is fought outside an arena only
// once it hit the bot, and inside one like anyone.
func TestPvPBotsOnlyInArena(t *testing.T) {
	marked := func(t *testing.T) *Brain {
		b := huntBrain(t)
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		player(t, b, findOther, markedName, Point{X: 300}, 1, 0, 9)
		return b
	}
	t.Run("not hunted", func(t *testing.T) {
		b := marked(t)
		if _, a := b.decide(at(time.Second)); a.object == findOther {
			t.Fatalf("acted on a flagged bot (%v)", a.kind)
		}
	})
	for _, tc := range []struct {
		name   string
		ev     Event
		fought bool
	}{
		{"its Attack", hitSelf(findOther, 5), true},
		{"its Wind Strike", castAt(findOther, selfID, skillWindStrike), true},
		{"its Heal", castAt(findOther, selfID, skillHeal), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := marked(t)
			b.handleOne(t, tc.ev, at(time.Second))
			b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(time.Second))
			if got := fightsWith(b, "attacked", findOther, at(2*time.Second)); got != tc.fought {
				t.Fatalf("fought %v, want %v", got, tc.fought)
			}
		})
	}
	t.Run("inside an arena", func(t *testing.T) {
		b := marked(t)
		arenaTripAt(b, box(-1000, -1000, 1000, 1000))
		if !fightsWith(b, "arena fight", findOther, at(time.Second)) {
			t.Fatal("a bot inside the arena is not fought")
		}
	})
}

// arenaTripAt puts the bot, a Human Fighter, inside arena z on an arena
// trip that set out from far outside it.
func arenaTripAt(b *Brain, z datapack.Zone) {
	b.cfg.Travel.zones.Arenas = []datapack.Zone{z}
	b.pvp.arena = arenaTrip{due: t0.Add(time.Hour), since: t0, arena: 1, to: arenaMiddle(z, 0), from: Point{X: 5000}, inside: t0}
}

// TestPvPArenaTarget checks arena targets (docs/pvp/targets.md "Arena
// targets") in an arena 500 to each side of the bot, a Warrior (first
// profession), and the arena-edge leave (docs/pvp/guards.md "Arena edge").
func TestPvPArenaTarget(t *testing.T) {
	const near, far, base = pvpThird, findOther, int32(0x20000006)
	arena := func(t *testing.T) *Brain {
		b := huntBrain(t)
		b.world.Self.ClassID = 1
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		arenaTripAt(b, box(-500, -500, 500, 500))
		return b
	}
	tier := func(t *testing.T, b *Brain, id int32, name string, p Point, class int32) {
		b.handleOne(t, CharInfo{ObjectID: id, Name: name, Pos: p, ClassID: class, RunSpeed: 124}, t0)
		place(t, b, id, p)
	}
	two := func(t *testing.T, b *Brain) {
		tier(t, b, near, "near", Point{Y: 100}, 1)
		tier(t, b, far, "far", Point{Y: 250}, 4)
	}
	t.Run("the nearest of its tier", func(t *testing.T) {
		b := arena(t)
		two(t, b)
		tier(t, b, base, "base", Point{X: 50}, 0)
		if !fightsWith(b, "arena fight", near, at(time.Second)) {
			t.Fatal("the tier player at 100 is not picked")
		}
	})
	t.Run("only a tier player 120 from the edge", func(t *testing.T) {
		b := arena(t)
		tier(t, b, near, "near", Point{X: 380}, 1)
		if rule, a := b.decide(at(time.Second)); a.object == near || rule == "arena fight" {
			t.Fatalf("rule %q acted on the player by the edge", rule)
		}
	})
	t.Run("a party member", func(t *testing.T) {
		b := arena(t)
		two(t, b)
		b.world.Party.Members = map[int32]*PartyMate{near: {PartyMember: PartyMember{Name: "near"}}}
		if !fightsWith(b, "arena fight", far, at(time.Second)) {
			t.Fatal("the party member is picked over the player at 250")
		}
	})
	t.Run("MP under its stand level", func(t *testing.T) {
		b := arena(t)
		two(t, b)
		b.world.Self.MP = 50
		if _, a := b.decide(at(time.Second)); a.kind != actSitStand {
			t.Fatalf("action %v, want it to sit", a.kind)
		}
	})
	t.Run("an attacker fought back first", func(t *testing.T) {
		b := arena(t)
		two(t, b)
		if !fightsWith(b, "arena fight", near, at(time.Second)) {
			t.Fatal("the tier player at 100 is not picked")
		}
		b.world.Target = near
		b.handleOne(t, hitSelf(far, 5), at(2*time.Second))
		b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(2*time.Second))
		if !fightsWith(b, "attacked", far, at(3*time.Second)) {
			t.Fatal("the player at 250 hitting it is not fought back")
		}
	})
	t.Run("a player just walked in", func(t *testing.T) {
		b := arena(t)
		tier(t, b, near, "near", Point{X: 200}, 1)
		if !fightsWith(b, "arena fight", near, at(time.Second)) {
			t.Fatal("the player 300 inside is not picked")
		}
	})
	t.Run("ArenaStay with its cast in flight", func(t *testing.T) {
		b := arena(t)
		b.cfg.Nav = testNav(t)
		b.walk = newWalker(b.cfg.Nav)
		b.world.SetGround(b.cfg.Nav.Engine)
		two(t, b)
		b.world.Target = near
		stay := at(arenaStay + time.Second)
		b.handleOne(t, AutoAttackStart{ObjectID: selfID}, stay)
		b.handleOne(t, MagicSkillUse{CasterID: selfID, TargetID: near, SkillID: skillWindStrike, Level: 1, HitTime: 4 * time.Second}, stay)
		rule, a := b.decide(stay)
		if rule != "arena leave" || a.kind != actTargetCancel {
			t.Fatalf("rule %q action %v, want the Esc", rule, a.kind)
		}
		if _, err := b.send(a, stay); err != nil {
			t.Fatal(err)
		}
		b.handleOne(t, MagicSkillCanceled{CasterID: selfID}, stay.Add(100*time.Millisecond))
		rule, a = b.decide(stay.Add(100 * time.Millisecond))
		if rule != "arena leave" || a.kind != actMove || b.arenaAt(moveTarget(a.payload)).Edge < arenaEdge {
			t.Fatalf("rule %q action %v, want the stop move inside", rule, a.kind)
		}
		if _, err := b.send(a, stay.Add(100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		interval := swingTime(b.world.Self.PAtkSpd)
		if rule, a := b.decide(stay.Add(100*time.Millisecond + interval/2)); rule != "arena leave" || a.kind != actHold {
			t.Fatalf("rule %q action %v inside the attack interval, want the hold", rule, a.kind)
		}
		rule, a = b.decide(stay.Add(200*time.Millisecond + interval))
		if rule != "arena leave" || a.kind != actMove || moveTarget(a.payload).X <= 0 {
			t.Fatalf("rule %q action %v after the interval, want the walk out", rule, a.kind)
		}
	})
}

// TestPvPArenaTrip checks the arena trip (docs/pvp/finding.md "Sources"):
// due ArenaEvery after the first decision, it releases the spot and walks
// to the nearest arena; inside, it stays while another player is there and
// leaves after ArenaAlone alone.
func TestPvPArenaTrip(t *testing.T) {
	b := newTripBrain(t)
	b.persona.PvP.Arenas = true
	b.cfg.Nav = testNav(t)
	b.walk = newWalker(b.cfg.Nav)
	b.world.SetGround(b.cfg.Nav.Engine)
	b.cfg.Travel.zones.Arenas = []datapack.Zone{box(1000, -500, 2000, 500), box(50000, -500, 51000, 500)}
	if rule, _ := b.decide(at(time.Second)); rule == "arena" {
		t.Fatal("an arena trip at the first decision")
	}
	due := b.pvp.arena.due
	if d := due.Sub(at(time.Second)); d < arenaEvery-arenaJitter || d > arenaEvery+arenaJitter {
		t.Fatalf("next trip in %v, want %v give or take %v", d, arenaEvery, arenaJitter)
	}
	if rule, _ := b.decide(due.Add(-time.Second)); rule == "arena" {
		t.Fatal("an arena trip before it is due")
	}
	rule, a := b.decide(due)
	if rule != "arena" || a.kind != actMove {
		t.Fatalf("rule %q action %v when due, want the walk", rule, a.kind)
	}
	if b.spot.held || b.pvp.arena.arena != 1 {
		t.Fatalf("spot held %v, arena %d: want the spot released and the nearest arena", b.spot.held, b.pvp.arena.arena)
	}
	in := due.Add(time.Minute)
	place(t, b, selfID, Point{X: 1500})
	b.handleOne(t, CharInfo{ObjectID: findOther, Name: otherName, Pos: Point{X: 1600}, ClassID: 4}, in)
	place(t, b, findOther, Point{X: 1600})
	if rule, a := b.decide(in); rule != "arena" || a.kind != actHold {
		t.Fatalf("rule %q action %v inside with company, want the stay", rule, a.kind)
	}
	b.handleOne(t, DeleteObject{ObjectID: findOther}, in.Add(time.Minute))
	b.decide(in.Add(time.Minute))
	if rule, a := b.decide(in.Add(time.Minute + arenaAlone - time.Second)); rule != "arena" || a.kind != actHold {
		t.Fatalf("rule %q action %v alone under ArenaAlone, want the stay", rule, a.kind)
	}
	if rule, a := b.decide(in.Add(time.Minute + arenaAlone)); rule != "arena leave" || a.kind != actMove {
		t.Fatalf("rule %q action %v alone ArenaAlone, want the stop move", rule, a.kind)
	}
}
