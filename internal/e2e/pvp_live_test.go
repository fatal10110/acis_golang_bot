package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The PvP scenarios (docs/pvp/README.md, X13, #420): bots playing the pvp
// persona against a scenario's own client on the real server, at most four
// characters each, the GM included. The duel, fight-back and healer
// scenarios meet at recoverGround, open ground in no peace, town or water
// zone, where duels and attacks are allowed; the arena scenario fights in
// the Gludin arena.

const (
	// pvpHeal and pvpBattleHeal are Heal (1011) and Battle Heal (1015).
	pvpHeal       int32 = 1011
	pvpBattleHeal int32 = 1015
	// pvpArenaLevel is the arena bots' and client's level: below 10 Lucky
	// spares a death its experience anywhere (character_death_exp_karma.go),
	// from 10 only the arena does.
	pvpArenaLevel = 10
	// pvpHealerLevel is the healer scenario's client's level: a level 1
	// mystic falls to the farmer's blows before the Cleric's heal.
	pvpHealerLevel = 10
	// pvpFlagBound bounds the wait for a PvP flag to run out: 40 s after
	// the last hit or heal (PvPVsNormalTime), its blink and the reaction.
	pvpFlagBound = 90 * time.Second
	// pvpDropGrace is how long after its target's flag ends a bot's swing
	// in flight or its reaction may still land; pvpDropWatch how long after
	// it the bot must leave the player be.
	pvpDropGrace = 5 * time.Second
	pvpDropWatch = 20 * time.Second
	// pvpDuelBound bounds a duel: at most 2 minutes (duel/state.go), its
	// countdown and end.
	pvpDuelBound = 3 * time.Minute
)

var (
	// gludinArenaMiddle is the middle of the Gludin arena (ArenaZone.xml
	// gludin_pvp); its Z is the geodata floor's (arenaFloor).
	gludinArenaMiddle = botclient.Point{X: -87920, Y: 142220, Z: -3600}
	// gludinTown is a Gludin village restart point (restartPointAreas.xml
	// gludin_town), in its peace zone: the GM waits there, out of the
	// arena bots' reach.
	gludinTown = botclient.Point{X: -83520, Y: 150560, Z: -3000}
)

// inGludinArena reports whether p lies inside the Gludin arena's polygon
// (ArenaZone.xml gludin_pvp, an almost square NPoly), by its inner box.
func inGludinArena(p botclient.Point) bool {
	return p.X > -88408 && p.X < -87429 && p.Y > 141733 && p.Y < 142708 && p.Z >= -3752 && p.Z <= -3352
}

// duelRefused reports whether system message id refuses a duel challenge
// to its challenger (network/duel.go, docs/pvp/server-duels.md): 1926 no
// such player, 1931 declined, 1940 the challenger cannot duel, 2017-2028
// the target cannot or is too far.
func duelRefused(id int32) bool {
	return id == 1926 || id == 1931 || id == 1940 || id >= 2017 && id <= 2028
}

// pvpPersona is the pvp preset's persona as a scenario runs it: a Damage
// fighter (bpLeaderPersona) whose PvP modes all play, as the preset sets
// them (botclient's pvpPolicy, the registry's "pvp" preset).
func pvpPersona() botclient.Persona {
	p := bpLeaderPersona()
	p.PvP = botclient.PvPPolicy{FightFlagged: true, Duels: true, Hunt: true, Arenas: true, SupportFlagged: true}
	return p
}

// hitOn matches an Attack of attacker with a hit on target.
func hitOn(attacker, target int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		a, ok := ev.(botclient.Attack)
		return ok && a.AttackerID == attacker && slices.ContainsFunc(a.Hits, func(h botclient.AttackHit) bool { return h.TargetID == target })
	}
}

// seenSince reports whether f read an event match accepts at index from or
// later.
func seenSince(f *farmer, from int, match func(botclient.Event) bool) bool {
	return slices.ContainsFunc(f.watch.Seen(from), func(s Seen) bool { return match(s.Event) })
}

// pvpMeet brings the GM to recoverGround, recalls the bots fs and the
// client c to it and has the GM heal them all: a duel wants HP and MP at
// least half, and a heal starts every scenario from full bars.
func pvpMeet(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, c *Client, fs ...*farmer) {
	t.Helper()
	bpGather(t, ctx, tg, gm, fs...)
	recallClient(t, ctx, gm, c)
	names := []string{c.Self.Name}
	for _, f := range fs {
		names = append(names, f.name)
	}
	for _, n := range names {
		if err := gm.Command(ctx, "heal "+n); err != nil {
			t.Fatal(err)
		}
	}
}

// pvpSelf follows a client's own HP and PvP flag from what it reads: its
// UserInfo carries the flag (character_pvpflag.go), StatusUpdate the HP.
type pvpSelf struct {
	id           int32
	hp, maxHP    int32
	flagged      bool
	flaggedAt    time.Time
	unflaggedAt  time.Time
	lowest       int32
	lowestFollow bool
}

func (s *pvpSelf) note(ev botclient.Event) {
	switch e := ev.(type) {
	case botclient.UserInfo:
		if e.ObjectID != s.id {
			return
		}
		s.hp, s.maxHP = e.HP, e.MaxHP
		switch {
		case e.PvPFlag != 0 && !s.flagged:
			s.flagged, s.flaggedAt, s.unflaggedAt = true, time.Now(), time.Time{}
		case e.PvPFlag == 0 && s.flagged:
			s.flagged, s.unflaggedAt = false, time.Now()
		}
	case botclient.StatusUpdate:
		if e.ObjectID != s.id {
			return
		}
		if e.Set&botclient.StatusHP != 0 {
			s.hp = e.HP
		}
		if e.Set&botclient.StatusMaxHP != 0 {
			s.maxHP = e.MaxHP
		}
	default:
		return
	}
	if s.lowestFollow && s.hp < s.lowest {
		s.lowest = s.hp
	}
}

// keepAlive has the GM heal c whenever s shows it under half HP, at most
// every 3 s: the bot fighting it back must not kill it before its flag
// runs out. The heal sets the vitals alone (admin_manage.go healCreature).
func keepAlive(t *testing.T, ctx context.Context, gm *GMClient, c *Client, s *pvpSelf) func() {
	var healed time.Time
	return func() {
		if s.maxHP > 0 && 2*s.hp < s.maxHP && time.Since(healed) >= 3*time.Second {
			healed = time.Now()
			if err := gm.Command(ctx, "heal "+c.Self.Name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestLivePvPDuel has a client challenge a pvp bot to a duel
// (docs/pvp/finding.md, X5): the bot accepts, and both fight, the client
// swinging back once the duel starts; the duel ends with no death (a
// killing hit leaves 1 HP), and the server gives both the vitals they
// started it with.
func TestLivePvPDuel(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 12*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	acc := func(n int) string { a, _ := tg.Account("pvd", n); return a }
	c := enter(t, ctx, tg, acc(2), unmarkedCharacter(t, acc(2), pvpNames), nil)
	bot := bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, pvpPersona, withTravel(travel))
	pvpMeet(t, ctx, tg, gm, c, bot)
	waitForBy(t, ctx, bot.name+" healed", time.Now().Add(time.Minute), func() bool {
		s := bot.bot.Status()
		return s.MaxHP > 0 && s.HP == s.MaxHP && s.MP == s.MaxMP
	})

	self := &pvpSelf{id: c.Self.ObjectID}
	botLowest := int32(-1)
	died := map[int32]bool{}
	var ended bool
	var result []string
	track := func(ev botclient.Event) {
		self.note(ev)
		switch e := ev.(type) {
		case botclient.ExDuelUpdateUserInfo:
			if e.ObjectID == bot.id {
				if botLowest < 0 || e.HP < botLowest {
					botLowest = e.HP
				}
			}
		case botclient.Die:
			died[e.ObjectID] = true
		case botclient.ExDuelEnd:
			ended = true
		case botclient.SystemMessage:
			if e.ID == 1950 || e.ID == 1952 || e.ID == 1955 {
				result = append(result, fmt.Sprintf("%d %q", e.ID, e.Text[0]))
			}
		}
	}

	botStart := bot.bot.Status()
	from := bot.seen()
	if err := c.Send(ctx, botclient.EncodeRequestDuelStart(bot.name, false)); err != nil {
		t.Fatal(err)
	}
	ev, err := c.Await(ctx, 45*time.Second, func(ev botclient.Event) bool {
		track(ev)
		switch e := ev.(type) {
		case botclient.ExDuelStart:
			return true
		case botclient.SystemMessage:
			return duelRefused(e.ID)
		}
		return false
	})
	if err != nil {
		t.Fatalf("the duel of %s and %s started: %v", c.Self.Name, bot.name, err)
	}
	if m, ok := ev.(botclient.SystemMessage); ok {
		t.Fatalf("the server refused %s's challenge of %s: system message %d %q", c.Self.Name, bot.name, m.ID, m.Text)
	}
	if self.maxHP == 0 {
		t.Fatalf("%s read none of its own HP before the duel", c.Self.Name)
	}
	start := self.hp
	self.lowest, self.lowestFollow = start, true
	t.Logf("%s accepted the duel; %s HP %d/%d, %s HP %d/%d", bot.name, c.Self.Name, start, self.maxHP, bot.name, botStart.HP, botStart.MaxHP)

	// Duellists attack each other without force once both are Duelling;
	// a ctrl-click is as good and raises no flag in the same duel.
	for _, p := range [][]byte{botclient.EncodeAction(bot.id, c.Pos()), botclient.EncodeAttackRequest(bot.id, c.Pos())} {
		if err := c.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	watchUntilBy(t, ctx, c, bot.name+" fighting "+c.Self.Name+" in the duel", time.Now().Add(time.Minute), func() bool {
		return seenSince(bot, from, hitOn(bot.id, c.Self.ObjectID))
	}, track)
	watchUntilBy(t, ctx, c, "the duel's end", time.Now().Add(pvpDuelBound), func() bool { return ended }, track)
	t.Logf("the duel ended: %v; %s lowest HP %d, %s lowest HP %d", result, c.Self.Name, self.lowest, bot.name, botLowest)
	if died[c.Self.ObjectID] || died[bot.id] || seenSince(bot, from, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && (d.ObjectID == bot.id || d.ObjectID == c.Self.ObjectID)
	}) {
		t.Fatalf("a duellist died: %s %v, %s %v", c.Self.Name, died[c.Self.ObjectID], bot.name, died[bot.id])
	}
	if self.lowest >= start && (botLowest < 0 || botLowest >= botStart.HP) {
		t.Fatalf("neither duellist lost HP (%s lowest %d of %d, %s lowest %d of %d): nothing to restore", c.Self.Name, self.lowest, start, bot.name, botLowest, botStart.HP)
	}
	// The end restores the CP, HP and MP saved at the start
	// (network/duel.go restoreDuelCondition), each player told by a
	// StatusUpdate.
	watchUntilBy(t, ctx, c, c.Self.Name+"'s and "+bot.name+"'s HP back", time.Now().Add(30*time.Second), func() bool {
		return self.hp >= start && bot.bot.Status().HP >= botStart.HP
	}, track)
}

// TestLivePvPFightBack has a client force-attack a pvp bot
// (docs/pvp/targets.md, X4): the bot fights back; the client stops
// swinging, its flag runs out 40 s after its last hit, and the bot drops
// it. The GM heals the client meanwhile, so the bot's blows do not kill
// it first.
func TestLivePvPFightBack(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 12*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	acc := func(n int) string { a, _ := tg.Account("pvf", n); return a }
	c := enter(t, ctx, tg, acc(2), unmarkedCharacter(t, acc(2), pvpNames), nil)
	bot := bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, pvpPersona, withTravel(travel))
	pvpMeet(t, ctx, tg, gm, c, bot)

	self := &pvpSelf{id: c.Self.ObjectID}
	keep := keepAlive(t, ctx, gm, c, self)
	var hit time.Time
	var botHits []time.Time
	track := func(ev botclient.Event) {
		self.note(ev)
		switch {
		case hitOn(c.Self.ObjectID, bot.id)(ev):
			hit = time.Now()
		case hitOn(bot.id, c.Self.ObjectID)(ev):
			botHits = append(botHits, time.Now())
		}
	}

	from := bot.seen()
	for _, p := range [][]byte{botclient.EncodeAction(bot.id, c.Pos()), botclient.EncodeAttackRequest(bot.id, c.Pos())} {
		if err := c.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	watchUntilBy(t, ctx, c, c.Self.Name+"'s first hit on "+bot.name, time.Now().Add(30*time.Second), func() bool { return !hit.IsZero() }, track)
	// A move ends the client's attack: it swings no more.
	stand := c.Pos()
	if _, err := c.Move(ctx, botclient.Point{X: stand.X + 150, Y: stand.Y, Z: stand.Z}); err != nil {
		t.Fatalf("%s stops attacking: %v", c.Self.Name, err)
	}
	watchUntilBy(t, ctx, c, bot.name+" fighting "+c.Self.Name+" back", time.Now().Add(30*time.Second), func() bool {
		keep()
		return seenSince(bot, from, hitOn(bot.id, c.Self.ObjectID))
	}, track)
	watchUntilBy(t, ctx, c, c.Self.Name+"'s flag running out", time.Now().Add(pvpFlagBound), func() bool {
		keep()
		return !self.unflaggedAt.IsZero()
	}, track)
	if self.flaggedAt.IsZero() {
		t.Fatalf("%s's hit never flagged it", c.Self.Name)
	}
	t.Logf("%s flagged %v, unflagged %v after its last hit", c.Self.Name, self.flaggedAt.Sub(hit).Round(time.Second), self.unflaggedAt.Sub(hit).Round(time.Second))

	quiet := self.unflaggedAt.Add(pvpDropGrace)
	done := quiet.Add(pvpDropWatch)
	watchUntilBy(t, ctx, c, bot.name+" leaving "+c.Self.Name+" be", done.Add(30*time.Second), func() bool {
		keep()
		return time.Now().After(done)
	}, track)
	for _, at := range botHits {
		if at.After(quiet) {
			t.Errorf("%s hit %s %v after its flag ran out", bot.name, c.Self.Name, at.Sub(self.unflaggedAt).Round(100*time.Millisecond))
		}
	}
	for _, s := range bot.watch.Sent() {
		if (s.Op == botclient.OpAction || s.Op == botclient.OpAttackRequest) && s.Target == c.Self.ObjectID && s.At.After(quiet) {
			t.Errorf("%s acted on %s %v after its flag ran out (opcode %#x)", bot.name, c.Self.Name, s.At.Sub(self.unflaggedAt).Round(100*time.Millisecond), s.Op)
		}
	}
	if self.flagged {
		t.Errorf("%s flagged again: it attacked anew", c.Self.Name)
	}
}

// arenaFloor is p on the geodata floor near its Z.
func arenaFloor(t *testing.T, tg *Target, p botclient.Point) botclient.Point {
	t.Helper()
	p.Z = int32(tg.Nav(t).Engine.Height(int(p.X), int(p.Y), int(p.Z)))
	return p
}

// arenaSpot is a spot at the middle of the Gludin arena, of level
// pvpArenaLevel and no monster: a bot farming it sets out on its arena trip
// from there (arenaReady wants a spot reached), already inside.
func arenaSpot(mid botclient.Point) *botclient.SpotRegistry {
	spot := datapack.Spot{
		Maker: "e2e_gludin_arena",
		Territory: &datapack.Territory{Name: "e2e_gludin_arena", MinZ: mid.Z - 100, MaxZ: mid.Z + 100, Nodes: []datapack.Point{
			{X: mid.X - 300, Y: mid.Y - 300}, {X: mid.X + 300, Y: mid.Y - 300}, {X: mid.X + 300, Y: mid.Y + 300}, {X: mid.X - 300, Y: mid.Y + 300},
		}},
		Center:   datapack.Point{X: mid.X, Y: mid.Y},
		Z:        mid.Z,
		MinLevel: pvpArenaLevel, MaxLevel: pvpArenaLevel, MaximumNPCs: 1,
	}
	return botclient.NewSpotRegistry([]datapack.Spot{spot}, 1)
}

// TestLivePvPArena has two pvp bots of a fighter row and a client fight in
// the Gludin arena (docs/pvp/guards.md, X4b, X5, X11). The arena trip is
// due at once (BrainConfig.ArenaFirst) from a spot in the arena's middle.
// Once both bots fought, one dies, the GM killing it unless a fight did:
// an arena death to a player costs no experience
// (character_death_exp_karma.go deathLossExemption), and the bot restarts
// in Gludin village and walks back into the arena (arenaDied).
func TestLivePvPArena(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 25*time.Minute)
	travel := travelMap(t, tg)
	mid := arenaFloor(t, tg, gludinArenaMiddle)
	if !inGludinArena(mid) {
		t.Fatalf("the arena's middle %v lies outside it", mid)
	}
	town := arenaFloor(t, tg, gludinTown)
	gm := enterGM(t, ctx, tg)
	acc := func(n int) string { a, _ := tg.Account("pva", n); return a }
	persona := func() botclient.Persona {
		p := pvpPersona()
		// The arena's other players stand at the spot: never too crowded.
		p.Spots.Crowd = 0
		// No raise comes: the restart need not wait for one.
		p.Recover.ResurrectWait = 5 * time.Second
		return p
	}
	run := func(n int) *farmer {
		spots := arenaSpot(mid)
		return bpRun(t, ctx, tg, acc(n), &botclient.HumanFighter, persona, func(cfg *botclient.BrainConfig) {
			cfg.Travel, cfg.Spots, cfg.ArenaFirst = travel, spots, time.Second
		})
	}
	a, b := run(1), run(2)
	c := enter(t, ctx, tg, acc(3), character(t, acc(3), &botclient.HumanFighter), nil)
	levelTo(t, ctx, gm, a, pvpArenaLevel)
	levelTo(t, ctx, gm, b, pvpArenaLevel)

	gmTeleport(t, ctx, gm, mid)
	for _, f := range []*farmer{a, b} {
		recallBot(t, ctx, tg, gm, f.name)
	}
	recallClient(t, ctx, gm, c)
	if err := gm.Select(ctx, c.Self.ObjectID); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{fmt.Sprintf("set level %d", pvpArenaLevel), "heal " + c.Self.Name, "heal " + a.name, "heal " + b.name} {
		if err := gm.OnTarget(ctx, c.Self.ObjectID, cmd); err != nil {
			t.Fatal(err)
		}
	}
	// Out of the arena, where the bots would take the GM for a target.
	gmTeleport(t, ctx, gm, town)

	trip := map[*farmer]bool{}
	onTrip := func() bool {
		for _, f := range []*farmer{a, b} {
			if r := f.bot.Status().Rule; r == "arena" || r == "arena fight" {
				trip[f] = true
			}
		}
		return trip[a] && trip[b]
	}
	// The spot retry (a minute) after an unreachable pick from the
	// village the bot entered in, then the walk to the spot's centre.
	watchUntilBy(t, ctx, c, "both bots on their arena trip", time.Now().Add(4*time.Minute), onTrip, nil)
	exp := bpExpAll(a, b)
	if exp[a] < 0 || exp[b] < 0 {
		t.Fatalf("no UserInfo with the bots' exp: %s %d, %s %d", a.name, exp[a], b.name, exp[b])
	}
	from := map[*farmer]int{a: a.seen(), b: b.seen()}
	t.Logf("%s exp %d and %s exp %d on their arena trip", a.name, exp[a], b.name, exp[b])

	for _, p := range [][]byte{botclient.EncodeAction(b.id, c.Pos()), botclient.EncodeAttackRequest(b.id, c.Pos())} {
		if err := c.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	ids := []int32{a.id, b.id, c.Self.ObjectID}
	fought := func(f *farmer) bool {
		return seenSince(f, from[f], func(ev botclient.Event) bool {
			at, ok := ev.(botclient.Attack)
			return ok && at.AttackerID == f.id && slices.ContainsFunc(at.Hits, func(h botclient.AttackHit) bool {
				return h.TargetID != f.id && slices.Contains(ids, h.TargetID)
			})
		})
	}
	watchUntilBy(t, ctx, c, "both bots fighting in the arena", time.Now().Add(3*time.Minute), func() bool {
		return fought(a) && fought(b)
	}, nil)

	diedAt := func(f *farmer) int {
		for i, s := range f.watch.Seen(from[f]) {
			if d, ok := s.Event.(botclient.Die); ok && d.ObjectID == f.id {
				return from[f] + i
			}
		}
		return -1
	}
	victim := a
	died := diedAt(a)
	if died < 0 {
		if died = diedAt(b); died >= 0 {
			victim = b
		}
	}
	if died < 0 {
		t.Logf("no fight killed a bot yet: the GM kills %s", a.name)
		killFrom := a.seen()
		if err := gm.Command(ctx, "kill "+a.name); err != nil {
			t.Fatal(err)
		}
		bpDied(t, ctx, a, killFrom)
		died = diedAt(a)
	}
	revived := bpRevived(t, ctx, victim, died, 2*time.Minute)
	t.Logf("%s died in the arena and restarted", victim.name)
	for _, s := range victim.watch.Seen(from[victim]) {
		if u, ok := s.Event.(botclient.UserInfo); ok && u.ObjectID == victim.id && u.Exp < exp[victim] {
			t.Fatalf("%s's arena death took experience: %d, had %d", victim.name, u.Exp, exp[victim])
		}
	}
	waitForBy(t, ctx, victim.name+" restarted out of the arena", time.Now().Add(time.Minute), func() bool {
		return !inGludinArena(victim.bot.Status().Position)
	})
	landed := victim.bot.Status().Position
	t.Logf("%s restarted at %v, %.0f from the arena's middle", victim.name, landed, dist(landed, mid))
	watchUntilBy(t, ctx, c, victim.name+" back in the arena", time.Now().Add(6*time.Minute), func() bool {
		return inGludinArena(victim.bot.Status().Position)
	}, nil)
	moves := 0
	for _, s := range sentOf(victim.watch.Sent(), botclient.OpMoveBackwardToLocation) {
		if s.At.After(victim.watch.Seen(revived)[0].At) {
			moves++
		}
	}
	if moves == 0 {
		t.Fatalf("%s came back into the arena with no move of its own", victim.name)
	}
	if got := bpExp(victim); got != exp[victim] {
		t.Errorf("%s exp %d back in the arena, had %d", victim.name, got, exp[victim])
	}
	t.Logf("%s walked back into the arena (%d moves)", victim.name, moves)
}

// TestLivePvPHealer has an M11 bot party of a farmer (the leader, Damage)
// and a support Cleric (SupportFlagged on) attacked by the GM in the open
// world, and a client heal the GM, which flags the client
// (docs/pvp/party.md, X9): the farmer turns on the enemy healer, the
// client, which flags the farmer, and the Cleric heals the flagged farmer.
// The Cleric is a new human mystic each run, levelled to 20 by the GM, that
// becomes a Cleric at Talking Island's class master (TestLiveBotPartyRaise);
// the GM gives it its heals, as //set level gives no SP to learn them.
func TestLivePvPHealer(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 25*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	acc := func(n int) string { a, _ := tg.Account("pvh", n); return a }
	leader := bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, bpLeaderPersona, nil)
	cleric := bpRun(t, ctx, tg, freshAccount(tg, "pvhc"), &botclient.HumanMystic, func() botclient.Persona {
		p := partyPersona()
		errandsOff(&p)
		p.ClassChange = botclient.DefaultPersona().ClassChange
		p.ClassChange.Builds = map[int32]int32{botclient.HumanMystic.ID: bpCleric}
		p.PvP.SupportFlagged = true
		return p
	}, withTravel(travel))
	ch := unmarkedCharacter(t, acc(2), pvpNames)
	ch.Class = botclient.HumanMystic
	c := enter(t, ctx, tg, acc(2), ch, nil)

	// The class change where the new character entered, by the master.
	levelTo(t, ctx, gm, cleric, 20)
	becomes(t, ctx, cleric, bpCleric)
	gmOn(t, ctx, gm, cleric, fmt.Sprintf("skill set %d 1", pvpHeal), fmt.Sprintf("skill set %d 1", pvpBattleHeal), "heal "+cleric.name)
	waitFor(t, ctx, cleric.name+" knowing its heals", func() bool { return cleric.bot.Status().Heals })

	pvpMeet(t, ctx, tg, gm, c, leader, cleric)
	if err := gm.Select(ctx, c.Self.ObjectID); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{fmt.Sprintf("set level %d", pvpHealerLevel), fmt.Sprintf("skill set %d 1", pvpHeal), "heal " + c.Self.Name} {
		if err := gm.OnTarget(ctx, c.Self.ObjectID, cmd); err != nil {
			t.Fatal(err)
		}
	}
	bpForm(t, ctx, leader, cleric)
	waitFor(t, ctx, cleric.name+" by "+leader.name, func() bool {
		return dist(cleric.bot.Status().Position, leader.bot.Status().Position) <= 400
	})

	// The GM's force attack flags it (PvPVsNormalTime). The GM reads on
	// through every later wait (pump): its fight sends it frames.
	gmSelf := &pvpSelf{id: gm.Self.ObjectID}
	t.Cleanup(func() { pvpUnflagGM(t, gm, gmSelf, leader, cleric) })
	pump := func() {
		if err := gm.Watch(ctx, 200*time.Millisecond, gmSelf.note); err != nil {
			t.Fatalf("the GM reading: %v", err)
		}
	}
	from := leader.seen()
	for _, p := range [][]byte{botclient.EncodeAction(leader.id, gm.Pos()), botclient.EncodeAttackRequest(leader.id, gm.Pos())} {
		if err := gm.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		gmSelf.note(ev)
		return hitOn(gm.Self.ObjectID, leader.id)(ev)
	}); err != nil {
		t.Fatalf("the GM hits %s: %v", leader.name, err)
	}
	gmFlagged := func(ev botclient.Event) bool {
		r, ok := ev.(botclient.RelationChanged)
		return ok && r.ObjectID == gm.Self.ObjectID && r.PvPFlag != 0
	}
	waitForBy(t, ctx, leader.name+" seeing the GM flagged", time.Now().Add(30*time.Second), func() bool {
		pump()
		return seenSince(leader, from, gmFlagged)
	})

	// The client's Heal on the flagged GM flags the client
	// (character_pvpflag.go skillTargetFlagsPvP).
	healFrom := leader.seen()
	for _, p := range [][]byte{botclient.EncodeAction(gm.Self.ObjectID, c.Pos()), botclient.EncodeRequestMagicSkillUse(pvpHeal, false)} {
		if err := c.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MagicSkillUse)
		return ok && m.CasterID == c.Self.ObjectID && m.TargetID == gm.Self.ObjectID && m.SkillID == pvpHeal
	}); err != nil {
		t.Fatalf("%s heals the GM: %v", c.Self.Name, err)
	}

	targets := func() bool {
		pump()
		return slices.ContainsFunc(leader.watch.Sent(), func(s Sent) bool {
			return (s.Op == botclient.OpAction || s.Op == botclient.OpAttackRequest) && s.Target == c.Self.ObjectID
		})
	}
	watchUntilBy(t, ctx, c, leader.name+" turning on "+c.Self.Name+", the enemy healer", time.Now().Add(time.Minute), targets, nil)
	t.Logf("%s turned on %s", leader.name, c.Self.Name)

	// The Cleric's heal on the leader while the leader's own UserInfo shows
	// it flagged: its hit on the flagged client flagged it.
	heals := func(skill int32) bool { return skill == pvpHeal || skill == pvpBattleHeal }
	healedFlagged := func() string {
		flagged := false
		for _, s := range leader.watch.Seen(healFrom) {
			switch e := s.Event.(type) {
			case botclient.UserInfo:
				if e.ObjectID == leader.id {
					flagged = e.PvPFlag != 0
				}
			case botclient.MagicSkillUse:
				if flagged && e.CasterID == cleric.id && e.TargetID == leader.id && heals(e.SkillID) {
					return ""
				}
			}
		}
		st := leader.bot.Status()
		return fmt.Sprintf("%s HP %d/%d, flagged %v", leader.name, st.HP, st.MaxHP, flagged)
	}
	untilBy(t, ctx, cleric.name+" healing the flagged "+leader.name, time.Now().Add(3*time.Minute), func() string {
		if err := c.Watch(ctx, 300*time.Millisecond, func(botclient.Event) {}); err != nil {
			return err.Error()
		}
		pump()
		return healedFlagged()
	})
	if seenSince(leader, from, func(ev botclient.Event) bool { d, ok := ev.(botclient.Die); return ok && d.ObjectID == leader.id }) {
		t.Fatalf("%s died in the fight", leader.name)
	}
	t.Logf("%s healed the flagged %s", cleric.name, leader.name)
}

// pvpUnflagGM logs the bots fs out, stops the GM's attack and waits, at
// most pvpFlagBound, until the GM's flag runs out: a pooled GM must not
// walk flagged into the next scenario, whose bots fight flagged players,
// and a bot it attacked would fight it back meanwhile.
func pvpUnflagGM(t *testing.T, gm *GMClient, s *pvpSelf, fs ...*farmer) {
	for _, f := range fs {
		f.stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), pvpFlagBound+30*time.Second)
	defer cancel()
	p := gm.Pos()
	if _, err := gm.Move(ctx, botclient.Point{X: p.X + 100, Y: p.Y, Z: p.Z}); err != nil {
		t.Logf("the GM stops attacking: %v", err)
	}
	if _, err := gm.Await(ctx, pvpFlagBound, func(ev botclient.Event) bool {
		s.note(ev)
		return !s.unflaggedAt.IsZero()
	}); err != nil {
		t.Logf("the GM's flag running out (flagged %v): %v", s.flagged, err)
	}
}
