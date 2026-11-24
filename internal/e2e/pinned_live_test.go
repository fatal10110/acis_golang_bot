package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Pinned scenario characters keep what every earlier run left them: the
// helpers here bound it, so each run starts from the same character.

// System messages about the character's own death penalty and its Scroll:
// Recovery (serverpackets/systemmessage.go): a level added, with the new
// level as its number, and sent too when a scroll lowers it to a level
// above 0; and the penalty lifted. A scroll whose item skill condition
// fails, such as one out of the character's level band, answers
// msgS1CannotBeUsed.
const (
	msgDeathPenaltyLevel  = 1916
	msgDeathPenaltyLifted = 1917
)

// entryDeathPenalty is the death penalty level of the EtcStatusUpdate in
// burst, c.Self.Burst: the server sends the character's own ahead of its
// UserInfo at EnterWorld (network/character_flow.go), so the burst holds
// it, and nothing another player broadcasts can reach the character
// before its spawn. ok is false when the burst holds none.
func entryDeathPenalty(burst [][]byte) (level int32, ok bool, err error) {
	for _, raw := range burst {
		ev, decoded, err := botclient.DecodeEvent(raw)
		if err != nil {
			return 0, false, err
		}
		if e, is := ev.(botclient.EtcStatusUpdate); decoded && is {
			return e.DeathPenalty, true, nil
		}
	}
	return 0, false, nil
}

// liftDeathPenalty lifts c's death penalty to level 0 with Scrolls:
// Recovery of level's band, the character's level, which the GM tops up
// to one per penalty level (after gmNext): a character that died in an
// earlier run would else start the scenario with its stats lowered
// (#341). It logs the level the character entered at, and fails naming
// the level when the server refuses a scroll or never answers one. c is
// alive, out of combat and not casting: strip revived it.
func liftDeathPenalty(t *testing.T, ctx context.Context, c *Client, gm *GMClient, inv map[int32]botclient.Item, level int32, gmNext func()) {
	t.Helper()
	penalty, ok, err := entryDeathPenalty(c.Self.Burst)
	if err != nil || !ok {
		t.Fatalf("strip %s: no EtcStatusUpdate in its entry burst (%v): its death penalty is unknown", c.Self.Name, err)
	}
	t.Logf("%s entered at death penalty level %d", c.Self.Name, penalty)
	if penalty == 0 {
		return
	}
	scroll := botclient.RecoveryScroll(level)
	if have := carriedOf(inv, scroll); have < penalty {
		gmNext()
		if err := gm.Give(ctx, c.Self.ObjectID, scroll, penalty-have); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			return carriedOf(inv, scroll) >= penalty
		}); err != nil {
			t.Fatalf("strip %s: death penalty level %d: the GM's %d Scrolls: Recovery %d: %v", c.Self.Name, penalty, penalty-have, scroll, err)
		}
	}
	from := penalty
	for penalty > 0 {
		id := int32(0)
		for oid, it := range inv {
			if it.TemplateID == scroll && it.Count > 0 {
				id = oid
				break
			}
		}
		if id == 0 {
			t.Fatalf("strip %s: death penalty level %d: out of Scrolls: Recovery %d", c.Self.Name, penalty, scroll)
		}
		read := time.Now()
		if err := c.Send(ctx, botclient.EncodeUseItem(id)); err != nil {
			t.Fatal(err)
		}
		was, refused := penalty, false
		if _, err := c.Await(ctx, time.Minute, func(ev botclient.Event) bool {
			applyInventory(inv, ev)
			m, ok := ev.(botclient.SystemMessage)
			switch {
			case !ok:
				return false
			case m.ID == msgDeathPenaltyLevel:
				penalty = m.Number
			case m.ID == msgDeathPenaltyLifted:
				penalty = 0
			case m.ID == msgS1CannotBeUsed:
				refused = true
			default:
				return false
			}
			return true
		}); err != nil {
			t.Fatalf("strip %s: death penalty level %d: Scroll: Recovery %d read, no answer: %v", c.Self.Name, was, scroll, err)
		}
		if refused {
			t.Fatalf("strip %s: death penalty level %d: the server refused Scroll: Recovery %d at character level %d", c.Self.Name, was, scroll, level)
		}
		if penalty >= was {
			t.Fatalf("strip %s: death penalty level %d after a Scroll: Recovery read at level %d", c.Self.Name, penalty, was)
		}
		if penalty == 0 {
			break
		}
		// The server drops a read still cooling in silence (network/
		// inventory.go, ItemDisabled), so strip waits out the scroll's reuse,
		// as the bot does; one read per reuse is all it sends meanwhile.
		if err := c.Watch(ctx, time.Until(read.Add(botclient.RecoveryReuse)), func(ev botclient.Event) { applyInventory(inv, ev) }); err != nil {
			t.Fatalf("strip %s: death penalty level %d: the scroll's reuse: %v", c.Self.Name, penalty, err)
		}
	}
	t.Logf("%s: death penalty level %d lifted to 0", c.Self.Name, from)
}

// gmTopUp has the GM give f, its target, what it lacks of want of template
// tmpl, and nothing when it carries as many: a pinned character keeps
// what earlier runs gave it, and a give every run would pile it up past
// its weight limit (#339).
func gmTopUp(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, tmpl, want int32) {
	t.Helper()
	have := f.carries(tmpl)
	if have < want {
		gmGive(t, ctx, gm, f, tmpl, want-have)
	}
	t.Logf("%s carried %d of template %d, topped up to %d", f.name, have, tmpl, max(have, want))
}
