package botclient

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang_bot/internal/craftplan"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// chatClan is the Say2 channel of clan chat (server handler/chat/chat.go
// Clan): the server sends the line to every online member of the speaker's
// clan as CreatureSay of that type, the speaker included, and has no flood
// gate on it (network/chat.go chatClan).
const chatClan int32 = 4

// chatTrade is the Say2 channel of trade chat (server handler/chat/chat.go
// Trade): the server sends the line to every player in the speaker's restart
// region (network/chat.go chatTrade, broadcastToRegion), behind the
// TradeChatTime flood gate (0 by default), dropping a gated line silently.
const chatTrade int32 = 8

// chatMaxText is the longest line the server takes, in UTF-16 units: it
// drops a longer one whole (handler/chat/chat.go MaxTextLength, Admit).
const chatMaxText = 100

// craftMaxCount bounds the count of a craft order.
const craftMaxCount = 10000

// parseCraft reads the words after "craft": the item name, then an optional
// count, 1 when none; a count out of 1..craftMaxCount is 0.
func parseCraft(args []string) command {
	c := command{kind: cmdCraft, count: 1}
	if n := len(args); n > 0 && strings.Trim(args[n-1], "0123456789") == "" {
		c.count = 0
		if v, err := strconv.ParseInt(args[n-1], 10, 64); err == nil && v >= 1 && v <= craftMaxCount {
			c.count = v
		}
		args = args[:n-1]
	}
	c.item = strings.Join(args, " ")
	return c
}

// heardClan takes a clan chat line at now: only a craft order, only by a
// crafter, and only by the first crafter of the clan to hear it (Heard), so
// a line answers once.
func (b *Brain) heardClan(e CreatureSay, now time.Time) {
	cmd, ok := parseCommand(e.Text)
	if !ok || cmd.kind != cmdCraft || !b.crafting() || !b.cfg.CraftBoard.Heard(b.world.Clan.ID, e.Name, e.Text, now) {
		return
	}
	b.heardCraft(tell{sender: e.ObjectID, name: e.Name, cmd: cmd, clan: true}, now)
}

// heardCraft routes a craft order at now: none unless the bot crafts in a
// clan; the clan leader's is taken at once (D20), anyone else's goes to the
// authority, which grants a GM's (D3). The leader reaches no other command.
func (b *Brain) heardCraft(t tell, now time.Time) {
	if !b.crafting() || b.world.Clan.ID == 0 {
		return
	}
	if l := b.world.Clan.Leader; l != "" && t.name == l {
		b.craftOrder(t, OrderByLeader, now)
		return
	}
	b.askAuthority(t, now)
}

// craftOrder places the craft order t of the clan leader or a GM (by) on
// the board and answers it once, at now.
func (b *Brain) craftOrder(t tell, by OrderSource, now time.Time) {
	if !b.crafting() || b.world.Clan.ID == 0 {
		return
	}
	b.answerOrder(t, b.placeOrder(t.cmd, by, t.name, now), now)
	b.wakeBy(now.Add(b.reactDelay()))
}

// placeOrder posts the craft order c by name (by) for the clan when a
// crafter of the clan can make it, and returns the answer: the plan, or why
// not (an unknown item, the recipe out of the book, Create Item too low,
// materials missing, too many orders open). The bot judges the materials
// from what it knows: the last warehouse list and what it carries; it opens
// no warehouse to answer.
func (b *Brain) placeOrder(c command, by OrderSource, name string, now time.Time) string {
	m := b.cfg.Travel
	switch {
	case c.item == "":
		return "say: craft <item> [count]"
	case c.count <= 0:
		return fmt.Sprintf("count 1 to %d", craftMaxCount)
	}
	product, ok := m.products[strings.ToLower(c.item)]
	if !ok {
		return "no recipe makes " + c.item
	}
	label := m.itemNames[product]
	crafter, ok := b.orderCrafter(product)
	if !ok {
		return b.cannotMake(product, label)
	}
	made, judged := c.count, false
	if crafter == "" {
		made, judged = b.orderCovers(product, c.count, now)
		if judged && made <= 0 {
			return "materials missing for " + label
		}
	}
	if _, ok := b.cfg.CraftBoard.PostCommand(b.world.Clan.ID, by, name, product, c.count); !ok {
		return "too many orders open"
	}
	switch {
	case crafter != "":
		return fmt.Sprintf("queued %d %s for %s", c.count, label, crafter)
	case made >= c.count:
		return fmt.Sprintf("crafting %d %s", c.count, label)
	case !judged:
		return fmt.Sprintf("queued %d %s, warehouse not checked yet", c.count, label)
	}
	return fmt.Sprintf("crafting %d of %d %s, materials missing for the rest", made, c.count, label)
}

// orderCrafter is who of the clan can make product: "" for the bot, else
// the name of another crafter whose published book holds a recipe for it;
// false for none.
func (b *Brain) orderCrafter(product int32) (string, bool) {
	if b.canMake(product) {
		return "", true
	}
	for _, e := range b.cfg.CraftBoard.Crafters(b.world.Clan.ID) {
		if e.Bot == b.world.Self.ObjectID {
			continue
		}
		for _, id := range e.Recipes {
			if r, ok := b.cfg.Travel.recipes[id]; ok && r.Product.ItemID == product {
				return e.Name, true
			}
		}
	}
	return "", false
}

// cannotMake is why the bot cannot make product, named label, with no
// recipe for it in its book: its craft level is under the lowest recipe's,
// or the recipe is not in the book.
func (b *Brain) cannotMake(product int32, label string) string {
	var low *datapack.Recipe
	for _, r := range b.cfg.Travel.recipes {
		if r.Product.ItemID == product && (low == nil || r.Level < low.Level || r.Level == low.Level && r.ID < low.ID) {
			low = &r
		}
	}
	if low != nil {
		skill, what := skillCreateItem, "Create Item"
		if !low.Dwarven {
			skill, what = skillCommonCraft, "Common Craft"
		}
		if b.world.Skills[skill].Level < low.Level {
			return fmt.Sprintf("my %s is too low for %s (needs %d)", what, label, low.Level)
		}
	}
	return "no recipe for " + label + " in my book"
}

// orderCovers is how many of count units of product a plan of the order
// alone makes from the bot's stock and, once it knows one, the last
// warehouse list; judged reports the list was known.
func (b *Brain) orderCovers(product int32, count int64, now time.Time) (made int64, judged bool) {
	known := b.wh.status.Known
	in := b.craftInput(nil, known, now)
	in.Orders = []craftplan.Order{{Product: product, Count: count}}
	cfg := b.persona.Crafter.Plan
	cfg.MaxChains = 0
	for _, ch := range craftplan.Build(in, cfg).Chains {
		if ch.Order == 0 {
			made += ch.Count
		}
	}
	return min(made, count), known
}

// answerOrder queues text as the one answer to the craft order t, on the
// channel it came by: a whisper back, or a clan chat line. It joins the
// queue of craft order lines (rule "announce"), so answers to orders inside
// one chat gap go out in turn and never replace a command's reply. None to
// a name with the bot marker, none with craftCommandCap lines queued, none
// past the ChatPolicy's Total; the answer counts in it. It is no reply to a
// whisper, so PerSender does not hold it back.
func (b *Brain) answerOrder(t tell, text string, now time.Time) {
	p := &b.persona.Chat
	if text == "" || b.cfg.markers.Marks(t.name) || len(b.announce) >= craftCommandCap || b.cfg.chat.full(now, p) {
		return
	}
	b.cfg.chat.count(now, p)
	text = clipChat(text)
	payload := encodeSay2(text, chatClan)
	if !t.clan {
		payload = encodeSay2Tell(t.name, text)
	}
	b.announce = append(b.announce, Action{kind: actAnswer, payload: payload})
}

// announceOrder queues the one clan chat line that says GM order o was
// delivered at now: none for a name with the bot marker, none past the
// ChatPolicy's Total, in which it counts.
func (b *Brain) announceOrder(o CraftOrder, now time.Time) {
	p := &b.persona.Chat
	if o.By != OrderByGM || o.Count <= 0 || b.cfg.markers.Marks(o.Member) ||
		len(b.announce) >= craftCommandCap || b.cfg.chat.full(now, p) {
		return
	}
	b.cfg.chat.count(now, p)
	label := b.cfg.Travel.itemNames[o.Product]
	if label == "" {
		label = fmt.Sprintf("item %d", o.Product)
	}
	text := clipChat(fmt.Sprintf("crafted %d %s for %s, in the clan warehouse", o.Count, label, o.Member))
	b.announce = append(b.announce, Action{kind: actAnnounce, payload: encodeSay2(text, chatClan)})
}

// ruleAnnounce says the first queued craft order line (an order's answer,
// or a finished GM order's clan chat line) once the chat gate lets it go
// (clan chat and whispers have no server flood gate), and passes until
// then.
func (b *Brain) ruleAnnounce(now time.Time) (Action, bool) {
	if len(b.announce) == 0 || !b.pace.allow(gateChat, now) {
		return Action{}, false
	}
	return b.announce[0], true
}

// clipChat cuts text to chatMaxText UTF-16 units, on a rune.
func clipChat(text string) string {
	n := 0
	for i, r := range text {
		if n += max(utf16.RuneLen(r), 1); n > chatMaxText {
			return text[:i]
		}
	}
	return text
}

// cancelUnplanned drops the leader and GM orders the cycle claimed that its
// plan, from a fresh warehouse list and not cut short, cannot start: the
// pool holds none of their materials, and an open order would bring a check
// back each time its claim lapses.
func (b *Brain) cancelUnplanned() {
	c := &b.craft
	for i, o := range c.orders {
		if o.By == OrderByMember || slices.ContainsFunc(c.plan.Chains, func(ch craftplan.Chain) bool { return ch.Order == i }) {
			continue
		}
		b.cfg.CraftBoard.Cancel(b.world.Clan.ID, o.ID, b.world.Self.ObjectID)
	}
}

// indexProductNames maps the lower-case name of every recipe product with a
// datapack template (an item or an etc item), its spaces single, to its
// template, the lowest id when two share a name, and each such product to
// its name.
func indexProductNames(recipes map[int32]datapack.Recipe, items map[int32]datapack.Item, etc map[int32]datapack.EtcItem) (map[string]int32, map[int32]string) {
	ids, names := make(map[string]int32), make(map[int32]string)
	for _, r := range recipes {
		id := r.Product.ItemID
		name := items[id].Name
		if name == "" {
			name = etc[id].Name
		}
		if name == "" {
			continue
		}
		names[id] = name
		if k := strings.Join(strings.Fields(strings.ToLower(name)), " "); ids[k] == 0 || id < ids[k] {
			ids[k] = id
		}
	}
	return ids, names
}
