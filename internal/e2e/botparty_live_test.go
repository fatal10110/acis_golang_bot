package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The bot-party scenarios (docs/parties/e2e.md, #392): bots handed their
// party orders as the organizer hands them, playing against the real
// server. The harness runs no supervisor: a scenario forms each party in
// the organizer's formation order (bpForm).

const (
	// bpCleric is the Cleric, the human mystic's Support profession at 20.
	bpCleric int32 = 15
	// bpResurrection is Resurrection (1016): the Cleric learns it at 20.
	bpResurrection int32 = 1016
	// bpPartyRange is the server's party range: a kill pays exp to the
	// members within it (docs/parties/members.md, "A member falls behind").
	bpPartyRange = 1500
	// bpFarmBound bounds a party's wait for exp: the leader picks a spot,
	// gathers its members and kills.
	bpFarmBound = 10 * time.Minute
)

// bpRun runs a whole watched Bot of account (runWatched) of class, playing
// persona, its brain config adjusted by brain when set, and logs its
// digest when the test fails.
func bpRun(t *testing.T, ctx context.Context, tg *Target, account string, class *botclient.StartingClass, persona func() botclient.Persona, brain func(*botclient.BrainConfig)) *farmer {
	t.Helper()
	f := runWatched(t, ctx, tg, account, botSetup{class: class, persona: persona, brain: brain})
	t.Cleanup(func() {
		if t.Failed() {
			s := f.bot.Status()
			t.Logf("%s: state %v class %d level %d HP %d/%d MP %d/%d rule %q spot %q walk %q party %q leader %q members %v left %q at %v",
				f.name, s.State, s.ClassID, s.Level, s.HP, s.MaxHP, s.MP, s.MaxMP, s.Rule, s.Spot, s.Walk, s.Party, s.Leader, s.Members, s.PartyLeft, s.Position)
			t.Log(f.digest())
			t.Log(f.visits())
		}
	})
	return f
}

// bpLeaderPersona is partyPersona playing Damage, the leader's role in a
// bot party (D25); the brain's default persona plays Support.
func bpLeaderPersona() botclient.Persona {
	p := partyPersona()
	p.Role = botclient.RoleDamage
	return p
}

// bpOrder hands b the party command kind with the names as its Arg and
// waits for its answer, which must be nil.
func bpOrder(t *testing.T, ctx context.Context, b *botclient.Bot, kind botclient.CommandKind, names ...string) {
	t.Helper()
	c := botclient.Command{Kind: kind, Arg: strings.Join(names, " ")}
	answer, err := b.Command(c)
	if err == nil {
		// A bot in session answers within a brain tick.
		wait, cancel := context.WithTimeout(ctx, EntryTimeout)
		select {
		case err = <-answer:
		case <-wait.Done():
			err = fmt.Errorf("no answer: %w", wait.Err())
		}
		cancel()
	}
	if err != nil {
		t.Fatalf("%s told %q: %v", b.Status().Character, c, err)
	}
}

// bpForm forms leader's bot party of members in the organizer's formation
// order (docs/parties/organizer.md, "Formation order"): each member's
// `join <leader> <others>` answered nil first, so its slot holds before the
// invitation arrives, then the leader's `lead <members>`. It returns once
// every member is in the leader's party.
func bpForm(t *testing.T, ctx context.Context, leader *farmer, members ...*farmer) {
	t.Helper()
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.name
	}
	for i, m := range members {
		join := append([]string{leader.name}, slices.Delete(slices.Clone(names), i, i+1)...)
		bpOrder(t, ctx, m.bot, botclient.CommandJoin, join...)
	}
	bpOrder(t, ctx, leader.bot, botclient.CommandLead, names...)
	for _, m := range members {
		waitForBy(t, ctx, fmt.Sprintf("%s in %s's party", m.name, leader.name), time.Now().Add(2*time.Minute), leaderIs(m.bot, leader.name))
	}
	t.Logf("%s leads %v", leader.name, names)
}

// bpExp is the exp of f's character in the last UserInfo f read (the
// server sends one on every exp change), -1 before any.
func bpExp(f *farmer) int64 {
	seen := f.watch.Seen(0)
	for i := len(seen) - 1; i >= 0; i-- {
		if u, ok := seen[i].Event.(botclient.UserInfo); ok {
			return u.Exp
		}
	}
	return -1
}

// bpGains waits until every one of fs has more exp than it had at from
// (bpExpAll), each still in leader's party.
func bpGains(t *testing.T, ctx context.Context, what string, leader *farmer, from map[*farmer]int64, fs ...*farmer) {
	t.Helper()
	untilBy(t, ctx, what, time.Now().Add(bpFarmBound), func() string {
		var short []string
		for _, f := range fs {
			if f != leader && !leaderIs(f.bot, leader.name)() {
				short = append(short, fmt.Sprintf("%s out of the party (leader %q)", f.name, f.bot.Status().Leader))
			} else if e := bpExp(f); e <= from[f] {
				short = append(short, fmt.Sprintf("%s exp %d, %.0f from the leader", f.name, e, dist(f.bot.Status().Position, leader.bot.Status().Position)))
			}
		}
		return strings.Join(short, "; ")
	})
}

// bpExpAll is the exp of each of fs now (bpExp).
func bpExpAll(fs ...*farmer) map[*farmer]int64 {
	m := make(map[*farmer]int64, len(fs))
	for _, f := range fs {
		m[f] = bpExp(f)
	}
	return m
}

// bpTrio runs a farming leader (a human fighter on the datapack's spots)
// and two human mystic Supports of accounts group 1-3, all of Talking
// Island, recalls them to the GM at recoverGround (open ground about 4000
// out of the village, where the dead restart) and forms the party.
func bpTrio(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, group string, leaderPersona func() botclient.Persona) (leader, m1, m2 *farmer) {
	t.Helper()
	spots := spotRegistry(t, tg)
	acc := func(n int) string { a, _ := tg.Account(group, n); return a }
	leader = bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, leaderPersona, func(cfg *botclient.BrainConfig) { cfg.Spots = spots })
	m1 = bpRun(t, ctx, tg, acc(2), &botclient.HumanMystic, partyPersona, nil)
	m2 = bpRun(t, ctx, tg, acc(3), &botclient.HumanMystic, partyPersona, nil)
	bpGather(t, ctx, tg, gm, leader, m1, m2)
	bpForm(t, ctx, leader, m1, m2)
	return leader, m1, m2
}

// bpGather teleports the GM to recoverGround and recalls every one of fs
// to it, waiting until each stands there.
func bpGather(t *testing.T, ctx context.Context, tg *Target, gm *GMClient, fs ...*farmer) {
	t.Helper()
	gmTeleport(t, ctx, gm, recoverGround)
	for _, f := range fs {
		recallBot(t, ctx, tg, gm, f.name)
	}
	for _, f := range fs {
		waitForBy(t, ctx, f.name+" at recoverGround", time.Now().Add(time.Minute), func() bool {
			return dist(f.bot.Status().Position, recoverGround) <= 300
		})
	}
}

// bpDied waits until f reads its own Die at index from or later.
func bpDied(t *testing.T, ctx context.Context, f *farmer, from int) {
	t.Helper()
	wait, cancel := context.WithTimeout(ctx, EntryTimeout)
	defer cancel()
	if _, _, err := f.watch.Await(wait, from, func(ev botclient.Event) bool {
		d, ok := ev.(botclient.Die)
		return ok && d.ObjectID == f.id
	}); err != nil {
		t.Fatalf("%s killed: %v", f.name, err)
	}
}

// bpRevived waits up to bound until f reads its own Revive at index from
// or later and returns the index.
func bpRevived(t *testing.T, ctx context.Context, f *farmer, from int, bound time.Duration) int {
	t.Helper()
	wait, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	i, _, err := f.watch.Await(wait, from, func(ev botclient.Event) bool {
		r, ok := ev.(botclient.Revive)
		return ok && r.ObjectID == f.id
	})
	if err != nil {
		t.Fatalf("%s revived: %v", f.name, err)
	}
	return i
}

// TestLiveBotPartyForms has a farmer and two Supports of Talking Island
// form a bot party: the members follow the leader to its spot, and its
// kills pay every member, within party range, exp. Whether a member hit
// the monster is not asserted: the server pays exp in range, attacker or
// not.
func TestLiveBotPartyForms(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
	gm := enterGM(t, ctx, tg)
	leader, m1, m2 := bpTrio(t, ctx, tg, gm, "bpf", bpLeaderPersona)

	from := bpExpAll(leader, m1, m2)
	bpGains(t, ctx, "the leader's kills paying every member exp", leader, from, leader, m1, m2)
	at := leader.bot.Status().Position
	for _, m := range []*farmer{m1, m2} {
		d := dist(m.bot.Status().Position, at)
		t.Logf("%s gained %d exp, %.0f from the leader", m.name, bpExp(m)-from[m], d)
		if d > 2*bpPartyRange {
			t.Errorf("%s stands %.0f from the leader, want it following", m.name, d)
		}
	}
}

// TestLiveBotPartyLeaderDeath has the GM kill the leader of a farming bot
// party: unraised (its members are level 1), the leader restarts in the
// village, and the same party farms together again, each member gaining
// exp in the leader's party.
func TestLiveBotPartyLeaderDeath(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 25*time.Minute)
	gm := enterGM(t, ctx, tg)
	leader, m1, m2 := bpTrio(t, ctx, tg, gm, "bpd", func() botclient.Persona {
		p := bpLeaderPersona()
		// No member can raise it: the restart need not wait the
		// party's two minutes (a persona default, D6).
		p.Recover.ResurrectWait = 10 * time.Second
		// The GM's kill, not the spot's monsters: a death holding a spot
		// would pass over it and every spot of a like pack for 30
		// minutes (SpotPolicy.DeathAvoid, PackShare), which left the
		// restarted leader of a live run no spot of its party's band.
		p.Spots.DeathAvoid = 0
		return p
	})
	bpGains(t, ctx, "the party farming", leader, bpExpAll(leader), leader)

	from := leader.seen()
	if err := gm.Command(ctx, "kill "+leader.name); err != nil {
		t.Fatal(err)
	}
	bpDied(t, ctx, leader, from)
	bpRevived(t, ctx, leader, from, 2*time.Minute)
	t.Logf("%s restarted at %v", leader.name, leader.bot.Status().Position)

	bpGains(t, ctx, "the same party farming again", leader, bpExpAll(leader, m1, m2), leader, m1, m2)
}

// TestLiveBotPartyRaise has a human mystic Support of a bot party,
// levelled to 20 by the GM, walk to Talking Island's class master in its
// party (P2b lifted the errand gate) and become a Cleric (D15). The GM's
// forms set level, SP and skills, not the class (gm.go), and //set level
// gives no SP for Resurrection's 3300, so the GM gives it 1016. The GM
// kills the leader, and the Cleric raises it before the leader's restart.
// A class change cannot be undone, so the Cleric is a new character each
// run.
func TestLiveBotPartyRaise(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 25*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)

	acc := func(n int) string { a, _ := tg.Account("bpr", n); return a }
	leader := bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, func() botclient.Persona {
		p := bpLeaderPersona()
		// The raise lands well inside it; a restart would end the
		// scenario failed, not race the Cleric.
		p.Recover.ResurrectWait = 5 * time.Minute
		return p
	}, nil)
	mate := bpRun(t, ctx, tg, acc(2), &botclient.HumanMystic, partyPersona, nil)
	cleric := bpRun(t, ctx, tg, freshAccount(tg, "bprc"), &botclient.HumanMystic, func() botclient.Persona {
		p := partyPersona()
		errandsOff(&p)
		p.ClassChange = botclient.DefaultPersona().ClassChange
		// The flow under test, not the branch choice (#328).
		p.ClassChange.Builds = map[int32]int32{botclient.HumanMystic.ID: bpCleric}
		return p
	}, withTravel(travel))

	// The party forms about 4300 from the master, out of the Cleric's
	// sight: its walk there takes the travel map, which the old party
	// gate refused. The GM goes there in one teleport: a GM whose restart
	// left it by the master read the master before a second teleport's
	// TeleportToLocation, and recalled the party to the master (the
	// first live run), where the dead leader stood out of the Cleric's
	// sight (181).
	bpGather(t, ctx, tg, gm, leader, mate, cleric)
	bpForm(t, ctx, leader, mate, cleric)

	levelTo(t, ctx, gm, cleric, 20)
	becomes(t, ctx, cleric, bpCleric)
	if !leaderIs(cleric.bot, leader.name)() {
		t.Fatalf("%s left %s's party for its class change (leader %q)", cleric.name, leader.name, cleric.bot.Status().Leader)
	}
	from := cleric.seen()
	gmOn(t, ctx, gm, cleric, "skill set 1016 1", "heal "+cleric.name)
	wait, cancel := context.WithTimeout(ctx, EntryTimeout)
	if _, _, err := cleric.watch.Await(wait, from, func(ev botclient.Event) bool {
		l, ok := ev.(botclient.SkillList)
		return ok && slices.ContainsFunc(l.Skills, func(s botclient.KnownSkill) bool { return s.ID == bpResurrection })
	}); err != nil {
		t.Fatalf("%s given Resurrection: %v", cleric.name, err)
	}
	cancel()
	waitFor(t, ctx, fmt.Sprintf("%s back by %s", cleric.name, leader.name), func() bool {
		return leaderIs(cleric.bot, leader.name)() && dist(cleric.bot.Status().Position, leader.bot.Status().Position) <= 400
	})

	from = leader.seen()
	if err := gm.Command(ctx, "kill "+leader.name); err != nil {
		t.Fatal(err)
	}
	bpDied(t, ctx, leader, from)
	revived := bpRevived(t, ctx, leader, from, 4*time.Minute)
	raised := false
	for _, s := range leader.watch.Seen(from)[:revived-from] {
		if m, ok := s.Event.(botclient.MagicSkillUse); ok && m.CasterID == cleric.id && m.TargetID == leader.id && m.SkillID == bpResurrection {
			raised = true
		}
	}
	if !raised {
		t.Fatalf("%s revived, but %s never cast Resurrection on it", leader.name, cleric.name)
	}
	t.Logf("%s raised %s", cleric.name, leader.name)
}

// TestLiveBotPartyPlayers has a plain player, its name carrying the bot
// marker as every harness player's does, whisper `join` to a bot party's
// leader: it is invited all the same (no party rule reads the marker). The
// GM's whispered `invite me` takes the member out of the bot party, and
// it whispers "invite me" back (D3).
func TestLiveBotPartyPlayers(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	acc := func(n int) string { a, _ := tg.Account("bpp", n); return a }
	player := enter(t, ctx, tg, acc(3), character(t, acc(3), &botclient.HumanFighter), nil)
	leader := bpRun(t, ctx, tg, acc(1), &botclient.HumanFighter, bpLeaderPersona, nil)
	member := bpRun(t, ctx, tg, acc(2), &botclient.HumanMystic, partyPersona, commandedBy(gm))
	gmToOpenGround(t, ctx, gm)
	recallClient(t, ctx, gm, player)
	for _, f := range []*farmer{leader, member} {
		recallBot(t, ctx, tg, gm, f.name)
	}
	bpForm(t, ctx, leader, member)

	tell(t, ctx, player, leader.name, "join")
	if _, err := player.Await(ctx, partyAnswer, func(ev botclient.Event) bool {
		a, ok := ev.(botclient.AskJoinParty)
		return ok && strings.EqualFold(a.Requester, leader.name)
	}); err != nil {
		t.Fatalf("%s invited %s on its join: %v", leader.name, player.Self.Name, err)
	}

	tell(t, ctx, gm.Client, member.name, "invite me")
	if _, err := gm.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		s, ok := ev.(botclient.CreatureSay)
		return ok && strings.EqualFold(s.Name, member.name) && s.Text == "invite me"
	}); err != nil {
		t.Fatalf("%s whispered the GM back: %v", member.name, err)
	}
	watchUntil(t, ctx, gm.Client, member.name+" out of the bot party", func() bool {
		s := member.bot.Status()
		return s.Leader == "" && s.Party == ""
	}, nil)
}
