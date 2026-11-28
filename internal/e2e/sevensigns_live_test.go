package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

const (
	// duskPriestess is the Dusk Priestess of Gludin (npcs/31000-31999.xml,
	// type SignsPriest); cataGate the Dungeon Gatekeeper enter_catacomb1.
	duskPriestess int32 = 31085
	cataGate      int32 = 31114
	// gateRefusalWait is how long a refused gatekeeper link is watched for
	// a teleport that must not come.
	gateRefusalWait = 5 * time.Second
)

// TestLiveSevenSignsJoin is Seven Signs member mode against the server
// (X18, D43): the one member named in Persona.SevenSigns signs up with
// Dusk at a Dusk Priestess, voting the seal Dusk holds, and is let into a
// catacomb by its Dungeon Gatekeeper, which turns a non-member away. Only
// the competition allows a sign-up, and no GM command changes the period:
// in any other period the scenario skips with what the priestess's
// greeting told (never faked; the unit tests of sevensigns_test.go play
// every page from recorded frames instead).
func TestLiveSevenSignsJoin(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 12*time.Minute)
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	travel := botclient.NewTravelMap(dp, botclient.DefaultSupplyConfig())
	priestAt, ok := fixedSpawn(dp, duskPriestess)
	if !ok {
		t.Fatalf("no fixed spawn of the Dusk Priestess %d", duskPriestess)
	}
	gateAt, ok := fixedSpawn(dp, cataGate)
	if !ok || len(dp.InstantTeleports[cataGate]) == 0 {
		t.Fatalf("no fixed spawn or instant teleport of the Dungeon Gatekeeper %d", cataGate)
	}
	gm := enterGM(t, ctx, tg)
	otherAcc, _ := tg.Account("ssj", 2)

	// The period, from the priestess's greeting to a non-member.
	gmTeleport(t, ctx, gm, priestAt)
	other := enter(t, ctx, tg, otherAcc, character(t, otherAcc, &botclient.HumanFighter), nil)
	recallClient(t, ctx, gm, other)
	greeting := npcPage(t, ctx, other, duskPriestess, "")
	if !strings.Contains(greeting.page, "_SevenSigns 3 1\"") {
		t.Skipf("Seven Signs period is not the competition (the Dusk Priestess greets a non-member with no sign-up link: %.160q); join needs Competition", greeting.page)
	}

	// The member: a new human fighter (a class change and a membership
	// cannot be undone) levelled to 20 by the class master, where it
	// becomes a Warrior, a first profession, then brought to the
	// priestess.
	alone(t, &townNPCs)
	masterAt(t, ctx, gm)
	memberAcc := freshAccount(tg, "ssj")
	// The bot's character takes one of its account's candidate names
	// (botclient.NewCharacter): the list names one character all the same.
	names := character(t, memberAcc, &botclient.HumanFighter).Names
	member := runWatched(t, ctx, tg, memberAcc, botSetup{
		class: &botclient.HumanFighter,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			errandsOff(&p)
			p.ClassChange = botclient.DefaultPersona().ClassChange
			p.ClassChange.Builds = map[int32]int32{botclient.HumanFighter.ID: classWarrior}
			p.SevenSigns = botclient.SevenSignsPolicy{Members: names, MaxMembers: len(names)}
			return p
		},
		brain: withTravel(travel),
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(member.visits())
		}
	})
	name := member.name
	recallBot(t, ctx, tg, gm, member.name)
	levelTo(t, ctx, gm, member, 20)
	becomes(t, ctx, member, classWarrior)
	gmTeleport(t, ctx, gm, priestAt)
	recallBot(t, ctx, tg, gm, member.name)
	waitFor(t, ctx, name+" signs up with Dusk", func() bool {
		for _, e := range member.watch.Seen(0) {
			if p, ok := e.Event.(botclient.NpcHtmlMessage); ok &&
				(strings.Contains(p.HTML, "application has been accepted") || strings.Contains(p.HTML, "already a member")) {
				return true
			}
		}
		return false
	})
	for _, s := range sentOf(member.watch.Sent(), opBypass) {
		for _, never := range []string{"SevenSigns 34", "SevenSigns 6", "SevenSigns 5 ", "SevenSigns 19 1 3", "SevenSigns 4 1 3"} {
			if strings.Contains(bypassText(s.Payload), never) {
				t.Errorf("%s sent %q, a link a member never sends", name, never)
			}
		}
	}
	member.stop()

	// The catacomb's gatekeeper lets the member in and turns the other
	// away.
	gmTeleport(t, ctx, gm, gateAt)
	in := enter(t, ctx, tg, memberAcc, character(t, memberAcc, &botclient.HumanFighter), nil)
	recallClient(t, ctx, gm, in)
	recallClient(t, ctx, gm, other)
	dest := dp.InstantTeleports[cataGate][0]
	if !gateTeleports(t, ctx, in, "cata") {
		t.Fatalf("the member %s was not let into the catacomb (destination %d %d %d)", in.Self.Name, dest.X, dest.Y, dest.Z)
	}
	if gateTeleports(t, ctx, other, "cata") {
		t.Fatalf("the non-member %s was let into the catacomb", other.Self.Name)
	}
}

// fixedSpawn is the first fixed spawn point of NPC template id in dp's
// spawnlist, outside every event.
func fixedSpawn(dp *datapack.Datapack, id int32) (botclient.Point, bool) {
	for _, mk := range dp.Spawnlist.Makers {
		if mk.Event != "" {
			continue
		}
		for _, sp := range mk.Spawns {
			if sp.NPCID == id && len(sp.Positions) == 1 {
				p := sp.Positions[0]
				return botclient.Point{X: p.X, Y: p.Y, Z: p.Z}, true
			}
		}
	}
	return botclient.Point{}, false
}

// shownPage is a page an NPC showed a scripted client: the NPC's object id
// and the page.
type shownPage struct {
	obj  int32
	page string
}

// npcPage has c click the NPC of template id in sight twice (select, then
// talk), or send link cmd of its open page when cmd is set, and returns the
// page it answers with.
func npcPage(t *testing.T, ctx context.Context, c *Client, id int32, cmd string) shownPage {
	t.Helper()
	obj := int32(0)
	if _, err := c.Await(ctx, EntryTimeout, func(ev botclient.Event) bool {
		if n, ok := ev.(botclient.NPCInfo); ok && n.NPCID == id {
			obj = n.ObjectID
		}
		return obj != 0
	}); err != nil {
		t.Fatalf("%s: NPC %d not in sight: %v", c.Self.Name, id, err)
	}
	sends := [][]byte{botwire.EncodeAction(obj, c.Pos()), botwire.EncodeAction(obj, c.Pos())}
	if cmd != "" {
		sends = [][]byte{botwire.EncodeRequestBypassToServer("npc_" + fmt.Sprint(obj) + "_" + cmd)}
	}
	for _, p := range sends {
		if err := c.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	ev, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
		p, ok := ev.(botclient.NpcHtmlMessage)
		return ok && p.ObjectID == obj
	})
	if err != nil {
		t.Fatalf("%s: no page of NPC %d: %v", c.Self.Name, id, err)
	}
	return shownPage{obj: obj, page: ev.(botclient.NpcHtmlMessage).HTML}
}

// gateTeleports has c talk to the catacomb's gatekeeper and send its link
// of command prefix cmd, and reports whether the server teleported c
// (a TeleportToLocation of c's own) within gateRefusalWait.
func gateTeleports(t *testing.T, ctx context.Context, c *Client, cmd string) bool {
	t.Helper()
	first := npcPage(t, ctx, c, cataGate, "")
	link := ""
	for _, l := range strings.Split(first.page, "bypass -h ")[1:] {
		if l, _, _ = strings.Cut(l, "\""); strings.HasPrefix(l, fmt.Sprintf("npc_%d_%s", first.obj, cmd)) {
			link = l
			break
		}
	}
	if link == "" {
		t.Fatalf("%s: the gatekeeper's page has no %s link: %.200q", c.Self.Name, cmd, first.page)
	}
	if err := c.Send(ctx, botwire.EncodeRequestBypassToServer(link)); err != nil {
		t.Fatal(err)
	}
	_, err := c.Await(ctx, gateRefusalWait, func(ev botclient.Event) bool {
		tp, ok := ev.(botclient.TeleportToLocation)
		return ok && tp.ObjectID == c.Self.ObjectID
	})
	return err == nil
}
