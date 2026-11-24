package e2e

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Clan member scenario (#244): Recipe: Soulshot D (recipe 20, scroll 1804,
// Create Item 2) makes soulshotsMade Soulshots D of three Soul Ore and one
// Crystal D. The member orders one craft of them: its ShotStock.Max.
const (
	soulshotRecipe int32 = 20
	soulshotsMade  int32 = 156
	// memberLevel is the member's level, set by the GM each run: the D
	// grade's Expertise comes at 20, so the member wears its Saber, and
	// lowSpots lets it farm the newbie spots next to the village.
	memberLevel int32 = 20
	// memberShots back the Saber past memberShotsMin (shotTarget), so the
	// member wears it; they are never short of a trip (ReturnMinutes 0).
	memberShots, memberShotsMin int32 = 1000, 100
	// crafterCrystals is the Crystal D the crafter carries: one craft's,
	// with one to spare, so it publishes the D grade (ShotGrades).
	crafterCrystals int32 = 2
	// handOffLoad is the member's ReturnLoad and ShedMin, in percent of its
	// weight limit: its worn Saber is past the first, and the spare Saber
	// (1520) its visit hands off is past the second, so the GM's gift of a
	// spare Saber, kept and never worn (memberKeep), is what sends it home.
	handOffLoad = 1
	// Client packets of a warehouse visit (warehouse.go
	// encodeWarehouseRequest); the keeper's lists open by opBypass.
	opWHDeposit  byte = 0x31
	opWHWithdraw byte = 0x32
	// msgNoRightClanWH and msgClanWHLevel refuse a clan warehouse open
	// (network/clan_warehouse.go clanWarehouseBypass).
	msgNoRightClanWH int32 = 709
	msgClanWHLevel   int32 = 710
	// msgLeaderOnlyWithdraw refuses a member's withdrawal on a server
	// without MembersCanWithdrawFromClanWH (network/clan_warehouse.go
	// clanWithdrawGate): the scenario needs that setting.
	msgLeaderOnlyWithdraw int32 = 1039
	// withdrawAnswer is how long a withdrawal goes unanswered before the
	// scenario takes it for the server's silent refusal (no warehouse
	// privilege, network/warehouse.go clanWithdrawGate).
	withdrawAnswer = 20 * time.Second
	// spotArrival bounds the member's walk to its spot: from the village
	// at its entry, back from town after a visit.
	spotArrival = 10 * time.Minute
	// visitBound bounds a member's trip from its spot to its visit's
	// errands: the walk to town, as long as spotArrival's back, then
	// warehouse opens MinOpenGap (a minute) apart.
	visitBound = spotArrival + 5*time.Minute
	// deliveryBound bounds the crafter's delivery of a posted order: its
	// next open, the craft and the deposit, opens MinOpenGap apart.
	deliveryBound = 10 * time.Minute
	// tripGap is the brain's supplyTripGap: the least time between two
	// supply trips, which delays the member's second visit.
	tripGap = 10 * time.Minute
)

// TestLiveClanMember plays the clan member option against a clan crafter
// (#244, docs/crafting.md section 7): a scripted leader founds a level-1
// clan, gives the member rank the warehouse privilege (RequestPledgePower,
// which bots never send) and invites a Dwarven crafter with Create Item 2,
// the Soulshot D recipe and Crystal D, which accepts any invitation (D20),
// then a level 20 member farming with a Saber, D shots and three Soul Ore,
// which accepts its commander's. Given a spare Saber at its spot, the
// member's hand-off weight starts a supply trip: in town its first visit
// withdraws nothing, deposits the Saber and the ore and posts a Soulshot D
// order with its materials in. The crafter crafts it and deposits the shots,
// which delivers it. A second spare Saber sends the member home again past
// supplyTripGap: its visit withdraws the shots the board credits to it,
// which its clan warehouse list showed, and the board credits it nothing
// more. The member carries no potions and sells nothing, so the hand-off is
// its one trip cause. Every value is a persona setting (D6). It takes about
// half an hour: two trips supplyTripGap apart, opens MinOpenGap apart.
func TestLiveClanMember(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &newbieSpots)
	ctx := scenarioCtx(t, 50*time.Minute)
	travel, items := townData(t, tg)
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	spots := botclient.NewSpotRegistry(dp.Spots(), 1)
	centres := map[string]botclient.Point{}
	for _, s := range dp.Spots() {
		centres[s.Maker] = botclient.Point{X: s.Center.X, Y: s.Center.Y, Z: s.Z}
	}
	gm := enterGM(t, ctx, tg)
	board := botclient.NewCraftBoard()
	leaderAcc, _ := tg.Account("cm", 1)
	crafterAcc, _ := tg.Account("cm", 2)
	memberAcc, _ := tg.Account("cm", 3)

	crafted := with(items, soulOre, crystalD, soulshotD, soulshotScroll, adena)
	strip(t, ctx, tg, gm, crafted, crafterAcc, &botclient.DwarvenFighter, stripEnd{
		at: nearKeeper, level: clanCreateLevel, adena: whAdena,
		carry: map[int32]int32{soulshotScroll: 1, crystalD: crafterCrystals},
	})
	strip(t, ctx, tg, gm, with(with(items, consumables...), soulOre, crystalD, soulshotScroll), memberAcc, &botclient.HumanFighter, stripEnd{
		at: nearKeeper, level: memberLevel, adena: tripAdena,
		carry: map[int32]int32{scrollOfEscape: escapes, soulshotD: memberShots, soulOre: 3},
	}, saber, squiresShirt, squiresPants)

	leader := enter(t, ctx, tg, leaderAcc, character(t, leaderAcc, &botclient.HumanFighter), nil)
	leaderID := leader.Self.ObjectID
	tg.Own(leader.Self.Name, leaderID)
	gmTeleport(t, ctx, gm, nearKeeper)
	recallClient(t, ctx, gm, leader)
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(leaderID)); err != nil {
		t.Fatalf("GM sees %s recalled: %v", leader.Self.Name, err)
	}
	clan := foundClan(t, ctx, gm, leader)
	clanID := clan.id

	crafter := runWatched(t, ctx, tg, crafterAcc, botSetup{class: &botclient.DwarvenFighter, persona: clanCrafterPersona, brain: func(cfg *botclient.BrainConfig) {
		withTravel(travel)(cfg)
		cfg.CraftBoard = board
	}})
	gmOn(t, ctx, gm, crafter, fmt.Sprintf("skill set %d 2", createItem))
	waitFor(t, ctx, "the crafter with Create Item 2", func() bool {
		c := crafter.bot.Status().Crafter
		return c != nil && c.CreateItem >= 2
	})
	inviteToClan(t, ctx, leader, crafter, clanID)
	// From here on the clan is dissolved on the crafter, which outlives
	// the leader's session: its cleanup runs before the bots log out.
	t.Cleanup(func() { clan.dismissOn(t, gm, crafter.name, crafter.id) })
	untilServed(t, ctx, "the crafter on the board with the Soulshot D recipe and the D grade", crafter.watch, warehouseRefused(crafter.watch), func() string {
		for _, e := range board.Crafters(clanID) {
			if e.Bot == crafter.id && slices.Contains(e.Recipes, soulshotRecipe) && len(e.ShotGrades) > 0 {
				t.Logf("%s on the board: recipes %v, shot grades %v", crafter.name, e.Recipes, e.ShotGrades)
				return ""
			}
		}
		return fmt.Sprintf("board %+v, crafter %+v", board.Crafters(clanID), crafter.bot.Status().Crafter)
	})

	member := runWatched(t, ctx, tg, memberAcc, botSetup{class: &botclient.HumanFighter, persona: memberPersona, brain: func(cfg *botclient.BrainConfig) {
		cfg.Spots, cfg.Travel, cfg.CraftBoard = spots, travel, board
		a := botclient.NewAuthority(botclient.AuthorityConfig{Commanders: []int32{leaderID}, Clock: botclient.SystemClock{}})
		cfg.Authority = a
		cfg.Persona = a.Hooks(cfg.Persona)
	}})
	inviteToClan(t, ctx, leader, member, clanID)
	// The leader's part is done: idle for the rest of the scenario, its
	// session would only back up the server's sends to it.
	lctx, cancel := context.WithTimeout(ctx, time.Minute)
	if err := leader.Logout(lctx); err != nil {
		t.Logf("%s logout: %v", leader.Self.Name, err)
	}
	cancel()
	leader.Close()
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(member.digest())
			t.Log(member.visits())
			t.Log(crafter.digest())
		}
	})

	noteMember := noteChanges(t, member.name, func() string {
		st := member.bot.Status()
		return fmt.Sprintf("rule %q, spot %q, walk %q, %d D shots, %d Soul Ore, %d Sabers", st.Rule, st.Spot, st.Walk, member.carries(soulshotD), member.carries(soulOre), member.carries(saber))
	})
	noteCrafter := noteChanges(t, crafter.name, func() string {
		var orders []string
		for _, o := range board.Orders(clanID, time.Now()) {
			orders = append(orders, fmt.Sprintf("%d %s by %d", o.ID, o.State, o.Crafter))
		}
		return fmt.Sprintf("crafter %+v, rule %q; orders %v", crafter.bot.Status().Crafter, crafter.bot.Status().Rule, orders)
	})

	// The waits below outlast the server's idle timeout of a silent GM.
	awake := gmAwake(t, ctx, gm, nearKeeper)

	// First trip: the visit deposits the spare Saber and the ore and posts
	// the order; the board credits the member nothing yet.
	gave := spareSaber(t, ctx, gm, member, centres, func() { noteMember(); awake() })
	untilServedBy(t, ctx, "the member's first visit: a deposit and a Soulshot D order", time.Now().Add(visitBound), member.watch, memberRefused(member.watch), func() string {
		noteMember()
		awake()
		st := member.bot.Status()
		switch {
		case len(sentSince(member.watch.Sent(), opWHDeposit, gave)) == 0:
			return fmt.Sprintf("no deposit (rule %q, spot %q at %v)", st.Rule, st.Spot, st.Position)
		case memberOrder(board, clanID, member.name) == nil:
			return fmt.Sprintf("no order on the board: %+v", board.Orders(clanID, time.Now()))
		}
		return ""
	})
	order := memberOrder(board, clanID, member.name)
	t.Logf("%s posted order %+v", member.name, *order)
	if order.Product != soulshotD || order.Count != int64(soulshotsMade) {
		t.Errorf("order %+v, want %d of %d", *order, soulshotsMade, soulshotD)
	}

	var owned int64
	untilServedBy(t, ctx, "the crafter's delivery of the order", time.Now().Add(deliveryBound), crafter.watch, warehouseRefused(crafter.watch), func() string {
		noteCrafter()
		awake()
		if owned = board.Owned(clanID, member.name)[soulshotD]; owned > 0 {
			return ""
		}
		return fmt.Sprintf("nothing credited to %s; crafter %+v", member.name, crafter.bot.Status().Crafter)
	})
	delivered := time.Now()
	t.Logf("%s delivered %d Soulshots D to %s", crafter.name, owned, member.name)
	if w := sentOf(member.watch.Sent(), opWHWithdraw); len(w) > 0 {
		t.Errorf("%s withdrew at %v before anything was credited to it", member.name, w[0].At)
	}

	// Second trip: the visit withdraws the credited shots.
	gave = spareSaber(t, ctx, gm, member, centres, func() { noteMember(); awake() })
	var took time.Time
	untilServedBy(t, ctx, "the member's second visit withdrawing its shots", time.Now().Add(tripGap+visitBound), member.watch, memberRefused(member.watch), func() string {
		noteMember()
		noteCrafter()
		awake()
		st := member.bot.Status()
		w := sentSince(member.watch.Sent(), opWHWithdraw, gave)
		if len(w) == 0 {
			return fmt.Sprintf("no withdrawal (rule %q, spot %q at %v)", st.Rule, st.Spot, st.Position)
		}
		if took.IsZero() {
			if shotRise(member.watch.Seen(0), w[0].At) < owned {
				if time.Since(w[0].At) > withdrawAnswer {
					t.Fatalf("%s's withdrawal at %v unanswered: the member rank's warehouse privilege or MembersCanWithdrawFromClanWH missing?", member.name, w[0].At)
				}
				return "the withdrawal not in"
			}
			took = time.Now()
		}
		if n := board.Owned(clanID, member.name)[soulshotD]; n != 0 {
			return fmt.Sprintf("the board still credits %d", n)
		}
		return ""
	})
	listed := false
	for _, s := range member.watch.Seen(0) {
		if l, ok := s.Event.(botclient.WarehouseWithdrawList); ok && l.Type == clanWHType && s.At.After(delivered) {
			n := listedOf(l, soulshotD)
			t.Logf("%s's clan warehouse list: %d items, %d Soulshots D", member.name, len(l.Items), n)
			listed = listed || n >= owned
		}
	}
	if !listed {
		t.Errorf("no clan warehouse list %s read after the delivery showed %d Soulshots D", member.name, owned)
	}
	t.Logf("%s took its %d shots at %v; the board credits it %v", member.name, owned, took.Format(time.TimeOnly), board.Owned(clanID, member.name))
}

// runClan is a clan a scenario founded: its id and name, and whether the
// test's end dissolved it.
type runClan struct {
	id        int32
	name      string
	dismissed bool
}

// dismissOn has the GM dissolve c on its member name, objectID, once.
func (c *runClan) dismissOn(t *testing.T, gm *GMClient, name string, objectID int32) {
	t.Helper()
	if c.dismissed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := gm.Command(ctx, "teleportto "+name); err != nil {
		t.Errorf("dismiss clan %s: %v", c.name, err)
		return
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(objectID)); err != nil {
		t.Errorf("dismiss clan %s: GM next to %s: %v", c.name, name, err)
		return
	}
	if err := gm.OnTarget(ctx, objectID, "pledge dismiss"); err != nil {
		t.Errorf("dismiss clan %s: %v", c.name, err)
		return
	}
	c.dismissed = true
}

// foundClan has the GM level leader to the founding level and found a
// level-1 clan of the run's own with it, which the test's end dissolves on
// the leader unless it was dissolved on another member first, and has
// leader give the member rank the warehouse privilege. A clan an earlier
// run left it leading is dissolved first.
func foundClan(t *testing.T, ctx context.Context, gm *GMClient, leader *Client) *runClan {
	t.Helper()
	id := leader.Self.ObjectID
	on := func(cmd string) {
		t.Helper()
		if err := gm.OnTarget(ctx, id, cmd); err != nil {
			t.Fatal(err)
		}
	}
	ui := userInfo(t, leader)
	if ui.ClanID != 0 {
		t.Logf("%s leads a clan from an earlier run: dismissed", leader.Self.Name)
		on("pledge dismiss")
	}
	if ui.Level < clanCreateLevel {
		on("set level " + strconv.Itoa(clanCreateLevel))
		if _, err := leader.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
			u, ok := ev.(botclient.UserInfo)
			return ok && u.Level >= clanCreateLevel
		}); err != nil {
			t.Fatalf("%s at level %d: %v", leader.Self.Name, clanCreateLevel, err)
		}
	}
	clan := "cm" + strconv.FormatInt(time.Now().UnixNano()%1e12, 36)
	on("pledge create " + clan)
	ev, err := leader.Await(ctx, 30*time.Second, func(ev botclient.Event) bool {
		r, ok := ev.(botclient.PledgeShowMemberListAll)
		return ok && r.PledgeType == 0 && r.Name == clan
	})
	if err != nil {
		t.Fatalf("%s founds clan %s: %v", leader.Self.Name, clan, err)
	}
	rc := &runClan{id: ev.(botclient.PledgeShowMemberListAll).ClanID, name: clan}
	t.Cleanup(func() { rc.dismissOn(t, gm, leader.Self.Name, id) })
	on("pledge level 1")
	if _, err := gm.Await(ctx, EntryTimeout, clanLevelSet(clan, 1)); err != nil {
		t.Fatalf("clan %s at level 1: %v", clan, err)
	}
	// A set that takes effect refreshes every online member's clan window
	// (network/clan.go requestPledgePower, clan_view.go
	// broadcastClanStatus): PledgeShowMemberListDeleteAll, then the pledge
	// lists. One that does not is unanswered, and would show only as the
	// member's withdrawal left unanswered (network/warehouse.go
	// clanWithdrawGate), so it fails here.
	if err := leader.Send(ctx, encodeRequestPledgePower(memberRank, privWarehouseSearch)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []func(botclient.Event) bool{
		func(ev botclient.Event) bool { _, ok := ev.(botclient.PledgeShowMemberListDeleteAll); return ok },
		func(ev botclient.Event) bool {
			r, ok := ev.(botclient.PledgeShowMemberListAll)
			return ok && r.PledgeType == 0 && r.ClanID == rc.id
		},
	} {
		if _, err := leader.Await(ctx, 30*time.Second, want); err != nil {
			t.Fatalf("rank %d of clan %s given the warehouse privilege: %v", memberRank, clan, err)
		}
	}
	return rc
}

// inviteToClan has leader invite f, in its sight, into its clan clanID and
// waits until f's UserInfo shows the clan, inviting it once more after an
// unanswered invitation (partyAnswer): live, the leader's second invitation
// was once answered WaitingForAnotherReply (164) a moment after the first
// was accepted. It fails with what the leader was told and whether f was
// asked.
func inviteToClan(t *testing.T, ctx context.Context, leader *Client, f *farmer, clanID int32) {
	t.Helper()
	if got := f.bot.Status().ClanID; got != 0 && got != clanID {
		t.Fatalf("%s is in clan %d from an earlier run", f.name, got)
	}
	from := f.seen()
	var said []string
	for try := 1; ; try++ {
		if err := leader.Send(ctx, botclient.EncodeRequestJoinPledge(f.id)); err != nil {
			t.Fatal(err)
		}
		for end := time.Now().Add(partyAnswer); time.Now().Before(end) && f.bot.Status().ClanID != clanID; {
			if err := leader.Watch(ctx, 2*time.Second, func(ev botclient.Event) {
				if m, ok := ev.(botclient.SystemMessage); ok {
					said = append(said, fmt.Sprint(m.ID, m.Text))
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
		if f.bot.Status().ClanID == clanID {
			return
		}
		asked := slices.ContainsFunc(f.watch.Seen(from), func(s Seen) bool {
			_, ok := s.Event.(botclient.AskJoinPledge)
			return ok
		})
		if try == 2 {
			t.Fatalf("%s not in clan %d after %d invitations: asked %v, rule %q; %s told %v", f.name, clanID, try, asked, f.bot.Status().Rule, leader.Self.Name, said)
		}
		t.Logf("%s not in the clan after an invitation (asked %v, %s told %v): again", f.name, asked, leader.Self.Name, said)
	}
}

// gmAwake returns a func that teleports gm to at once gmKeepAlive passed
// since it last did: the server drops a connection it read nothing from for
// 15 minutes (session.go clientReadIdleTimeout), and the scenario's cleanup
// dissolves its clan through the GM.
func gmAwake(t *testing.T, ctx context.Context, gm *GMClient, at botclient.Point) func() {
	alive := time.Now()
	return func() {
		t.Helper()
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, at)
			alive = time.Now()
		}
	}
}

// noteChanges returns a func that logs line, with who, whenever it
// changed since the func last logged it.
func noteChanges(t *testing.T, who string, line func() string) func() {
	var last string
	return func() {
		if now := line(); now != last {
			last = now
			t.Logf("%s: %s", who, now)
		}
	}
}

// spareSaber waits up to spotArrival for the member at its spot, within its
// leash of the centre (centres, by spot name), calling note on every poll,
// has the GM give it a spare Saber there, and brings the GM back to the
// village; it returns when the GM gave it. The member need not be farming:
// on a spot other players farm bare it idles there (#360), and the gift's
// weight starts its trip all the same (rule "supply" runs ahead of "farm").
func spareSaber(t *testing.T, ctx context.Context, gm *GMClient, member *farmer, centres map[string]botclient.Point, note func()) time.Time {
	t.Helper()
	leash := int64(memberPersona().Spots.Leash)
	untilBy(t, ctx, member.name+" at its spot", time.Now().Add(spotArrival), func() string {
		note()
		st := member.bot.Status()
		if c, ok := centres[st.Spot]; ok && within(st.Position, c, leash) {
			return ""
		}
		return fmt.Sprintf("rule %q, spot %q, walk %q at %v", st.Rule, st.Spot, st.Walk, st.Position)
	})
	st := member.bot.Status()
	t.Logf("%s at %q (rule %q), wielding the Saber %v, %d D shots, %d Soul Ore", member.name, st.Spot, st.Rule, member.wields(saber), member.carries(soulshotD), member.carries(soulOre))
	gmOn(t, ctx, gm, member)
	gave := time.Now()
	gmGiveGear(t, ctx, gm, member, saber, 1)
	gmTeleport(t, ctx, gm, nearKeeper)
	return gave
}

// clanCrafterPersona is crafterPersona accepting any player's clan
// invitation while clanless (D20, CrafterPersona).
func clanCrafterPersona() botclient.Persona {
	p := crafterPersona()
	p.JoinClan = botclient.CrafterPersona().JoinClan
	return p
}

// memberPersona is the support preset's persona (DefaultPersona) with the
// clan member option, as a level 20 farmer of the newbie spots (lowSpots,
// errandsOff) whose one supply trip cause is the weight its visit hands
// off: its shots never run out within a trip (ReturnMinutes 0), it carries
// no potion, sells nothing, and its ReturnLoad and ShedMin are handOffLoad.
// Its ShotStock orders one craft of D shots.
func memberPersona() botclient.Persona {
	p := botclient.DefaultPersona()
	p.ClanMember = true
	lowSpots(&p)
	errandsOff(&p)
	p.Potions = nil
	p.Shop.SellAt = 0
	p.Shop.Soulshots = botclient.ShotStock{Min: memberShotsMin, Max: soulshotsMade}
	p.Supply.ReturnMinutes = 0
	p.Supply.ReturnLoad, p.Supply.ShedMin = handOffLoad, handOffLoad
	return p
}

// memberOrder is the open order member posted on clan's board, nil for none.
func memberOrder(board *botclient.CraftBoard, clan int32, member string) *botclient.CraftOrder {
	for _, o := range board.Orders(clan, time.Now()) {
		if o.By == botclient.OrderByMember && o.Member == member {
			return &o
		}
	}
	return nil
}

// warehouseRefused is what refuses a clan warehouse visit: its open by the
// keeper's bypass (network/clan_warehouse.go clanWarehouseBypass) and its
// deposit or withdrawal (network/warehouse.go). A silent refusal shows as
// no answer.
func warehouseRefused(w *Watch) RefusalCheck {
	return RefusalCheck{
		From: len(w.Sent()),
		Packet: func(s Sent) bool {
			return s.Op == opWHDeposit || s.Op == opWHWithdraw || s.Op == opBypass
		},
		Messages: []int32{msgNoRightClanWH, msgClanWHLevel, msgLeaderOnlyWithdraw, msgNotEnoughAdena, msgSlotsFull, msgWeightLimitExceeded, msgQuantityExceeded},
		// The leader-only message answers only the withdrawal
		// (clanWithdrawGate, from the withdraw handler): it blames the
		// 0x32, not the keeper's bypass the visit sent a moment before.
		MessageOps: map[int32][]byte{msgLeaderOnlyWithdraw: {opWHWithdraw}},
	}
}

// memberRefused is warehouseRefused for the member, which also reads a
// Scroll of Escape home (escapeRefused).
func memberRefused(w *Watch) RefusalCheck {
	c := warehouseRefused(w)
	escapes := objectsOf(w.Seen(0), scrollOfEscape)
	visit := c.Packet
	c.Packet = func(s Sent) bool { return visit(s) || s.Op == botclient.OpUseItem && escapes[s.Target] }
	c.Messages = append(c.Messages, msgS1CannotBeUsed)
	return c
}

// sentSince are the packets of opcode op in sent written after at.
func sentSince(sent []Sent, op byte, at time.Time) []Sent {
	var out []Sent
	for _, s := range sentOf(sent, op) {
		if s.At.After(at) {
			out = append(out, s)
		}
	}
	return out
}

// shotRise is the largest rise of the carried Soulshots D one inventory
// event of seen after at shows.
func shotRise(seen []Seen, at time.Time) int64 {
	var last, rise int64
	for _, s := range seen {
		n, _, ok := stock(s.Event, soulshotD)
		if !ok {
			continue
		}
		if s.At.After(at) {
			rise = max(rise, int64(n)-last)
		}
		last = int64(n)
	}
	return rise
}

// listedOf is how many of template tmpl list l shows.
func listedOf(l botclient.WarehouseWithdrawList, tmpl int32) int64 {
	var n int64
	for _, it := range l.Items {
		if it.TemplateID == tmpl {
			n += int64(it.Count)
		}
	}
	return n
}
