package botclient

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// shotItem is a player's shot template: its kind, crystal grade, and
// whether it is a beginner's shot, which no merchant buys back.
type shotItem struct {
	kind     datapack.EtcKind
	grade    int32
	beginner bool
}

// builtinShots are the soulshots, spiritshots and blessed spiritshots of
// every grade, the beginners' ones included (aCis_datapack/data/xml/items):
// the shot table of a bot without a datapack.
var builtinShots = map[int32]shotItem{
	5789: {datapack.EtcSoulshot, 0, true}, soulshotNoGrade: {datapack.EtcSoulshot, 0, false},
	1463: {datapack.EtcSoulshot, 1, false}, 1464: {datapack.EtcSoulshot, 2, false},
	1465: {datapack.EtcSoulshot, 3, false}, 1466: {datapack.EtcSoulshot, 4, false},
	1467: {datapack.EtcSoulshot, 5, false},
	5790: {datapack.EtcSpiritshot, 0, true}, spiritshotNoGrade: {datapack.EtcSpiritshot, 0, false},
	2510: {datapack.EtcSpiritshot, 1, false}, 2511: {datapack.EtcSpiritshot, 2, false},
	2512: {datapack.EtcSpiritshot, 3, false}, 2513: {datapack.EtcSpiritshot, 4, false},
	2514: {datapack.EtcSpiritshot, 5, false},
	3947: {datapack.EtcBlessedSpiritshot, 0, false}, 3948: {datapack.EtcBlessedSpiritshot, 1, false},
	3949: {datapack.EtcBlessedSpiritshot, 2, false}, 3950: {datapack.EtcBlessedSpiritshot, 3, false},
	3951: {datapack.EtcBlessedSpiritshot, 4, false}, 3952: {datapack.EtcBlessedSpiritshot, 5, false},
}

// weapon is what a weapon template charges: its crystal grade and how many
// soulshots and spiritshots an attack or cast spends, 0 for none. The
// server charges only a shot of the weapon's grade, and only into a weapon
// with room for its kind; blessed and plain spiritshots share the
// spiritshot room (character_attack_shots.go ChargeSoulshot,
// ChargeSpiritshot).
type weapon struct {
	grade                  int32
	soulshots, spiritshots int
}

// indexWeapons indexes every weapon template of items by what it charges.
func indexWeapons(items map[int32]datapack.Item) map[int32]weapon {
	out := make(map[int32]weapon)
	for id, it := range items {
		if it.Type == "Weapon" {
			out[id] = weapon{grade: int32(it.Grade), soulshots: it.Soulshots, spiritshots: it.Spiritshots}
		}
	}
	return out
}

// spawnedSold is the set of items the lists of the spawned NPCs of shops
// sell. The travel map indexes the lists of NPCs without a spawn too, and
// one of them, the Sales Cat's, is the only one selling D-grade shots.
func spawnedSold(shops map[int32][]shopList, spawns map[int32][]Point) map[int32]bool {
	out := make(map[int32]bool)
	for npc, lists := range shops {
		if len(spawns[npc]) == 0 {
			continue
		}
		for _, l := range lists {
			for _, p := range l.products {
				out[p.ItemID] = true
			}
		}
	}
	return out
}

// shotSource is where the bot gets more of a shot template than it
// carries: the merchants of the travel map, and next the clan crafter
// (M9).
type shotSource interface {
	sellsShot(id int32) bool
}

// sellsShot reports whether a spawned merchant sells shot template id.
func (m *TravelMap) sellsShot(id int32) bool { return m.spawnedSold[id] }

// shotSources are the bot's shot sources.
func (b *Brain) shotSources() []shotSource {
	if b.cfg.Travel == nil {
		return nil
	}
	return []shotSource{b.cfg.Travel}
}

// sourced reports whether a shot source has shot template id.
func (b *Brain) sourced(id int32) bool {
	return slices.ContainsFunc(b.shotSources(), func(s shotSource) bool { return s.sellsShot(id) })
}

// shotTable is the travel map's shot templates; builtinShots without one.
func (b *Brain) shotTable() map[int32]shotItem {
	if b.cfg.Travel != nil && len(b.cfg.Travel.shots) > 0 {
		return b.cfg.Travel.shots
	}
	return builtinShots
}

// weaponOf is what weapon template id charges. Without a weapon index (no
// datapack) it is taken as a no-grade weapon with room for both kinds:
// shots of a grade the bot cannot look up are never wanted.
func (b *Brain) weaponOf(id int32) weapon {
	if b.cfg.Travel == nil || len(b.cfg.Travel.weapons) == 0 {
		return weapon{soulshots: 1, spiritshots: 1}
	}
	return b.cfg.Travel.weapons[id]
}

// shotKindsOf are the shot kinds the bot's class shoots, the preferred
// first: blessed then plain spiritshots for a mystic, soulshots otherwise.
func (b *Brain) shotKindsOf() []datapack.EtcKind {
	if slices.Contains(mysticClasses, b.world.Self.ClassID) {
		return []datapack.EtcKind{datapack.EtcBlessedSpiritshot, datapack.EtcSpiritshot}
	}
	return []datapack.EtcKind{datapack.EtcSoulshot}
}

// shotsFor are the shot templates weapon w charges for the bot's class:
// of its kinds and w's grade, when w has room for them; the beginners'
// first, then by kind preference, then by id. None for a weapon without
// room.
func (b *Brain) shotsFor(w weapon) []int32 {
	kinds := b.shotKindsOf()
	room := w.soulshots
	if kinds[0] != datapack.EtcSoulshot {
		room = w.spiritshots
	}
	if room <= 0 {
		return nil
	}
	table := b.shotTable()
	var ids []int32
	for id, s := range table {
		if s.grade == w.grade && slices.Contains(kinds, s.kind) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(x, y int32) int {
		a, c := table[x], table[y]
		return cmp.Or(-cmpBool(a.beginner, c.beginner), cmp.Compare(slices.Index(kinds, a.kind), slices.Index(kinds, c.kind)), cmp.Compare(x, y))
	})
	return ids
}

// shotStock is the restock of the bot's class's shot kind
// (ShopPolicy.Soulshots or Spiritshots).
func (b *Brain) shotStock() ShotStock {
	if slices.Contains(mysticClasses, b.world.Self.ClassID) {
		return b.persona.Shop.Spiritshots
	}
	return b.persona.Shop.Soulshots
}

// shotTarget is how many shots of a weapon's the bot carries before they
// alone back it (shotsBacked): the restock minimum, until the trip target
// of the use meter (#170) replaces it.
func (b *Brain) shotTarget() int32 {
	return b.shotStock().Min
}

// carriedShots is how many shots weapon w charges the bot carries.
func (b *Brain) carriedShots(w weapon) int32 {
	var n int32
	for _, id := range b.shotsFor(w) {
		n += b.carried(id)
	}
	return n
}

// shotsBacked reports whether the bot can keep weapon w, template id,
// charged: a shot source has a shot w charges, or, when carried is set, the
// bot carries enough of them. Enough is shotTarget to switch to w, so a few
// looted shots do not back a weapon, and any once w is wielded, so the bot
// fires the stack it switched for to the last shot before it switches back.
func (b *Brain) shotsBacked(id int32, w weapon, carried bool) bool {
	ids := b.shotsFor(w)
	if slices.ContainsFunc(ids, b.sourced) {
		return true
	}
	have := b.carriedShots(w)
	if !carried || have == 0 {
		return false
	}
	return have >= b.shotTarget() || b.worn()[slotWeapon].TemplateID == id
}

// shotValues are the values beats compares for weapon template id against
// the wielded cur, gain and value, with the shot factor
// (GearPolicy.ShotFactor): the weapon's part of the one of the two the bot
// can keep charged (shotsBacked, carried shots counting when carried is
// set) is multiplied by it when the other is not. Between two it can keep
// charged, or two it cannot, the values stay as they are, so the factor
// never shifts a choice shots do not tell apart, such as a two-handed
// weapon against a one-handed one and its shield.
func (b *Brain) shotValues(id, cur int32, g, c gearItem, gain, value int64, carried bool) (int64, int64) {
	f := b.persona.Gear.ShotFactor
	if f <= 1 {
		return gain, value
	}
	backed := func(id int32) bool {
		w, ok := b.cfg.Travel.weapons[id]
		return ok && b.shotsBacked(id, w, carried)
	}
	extra := func(price int32) int64 { return int64(math.Round(float64(price)*f)) - int64(price) }
	switch gb, cb := backed(id), backed(cur); {
	case gb && !cb:
		gain += extra(g.price)
	case cb && !gb:
		value += extra(c.price)
	}
	return gain, value
}

// wornWeapon is the weapon the bot wields; ok false when it wields none.
func (b *Brain) wornWeapon() (weapon, bool) {
	it := b.worn()[slotWeapon]
	if it.ObjectID == 0 {
		return weapon{}, false
	}
	return b.weaponOf(it.TemplateID), true
}

// shotWeapons are the weapons whose shots the bot uses or will: the one it
// wields and the one the gear errand wears next (nextWeapon).
func (b *Brain) shotWeapons() []weapon {
	var out []weapon
	if w, ok := b.wornWeapon(); ok {
		out = append(out, w)
	}
	if it := b.nextWeapon(); it.ObjectID != 0 {
		out = append(out, b.weaponOf(it.TemplateID))
	}
	return out
}

// useShot is the shot template whose auto use the wielded weapon wants:
// the first carried of shotsFor's, so beginners' shots go first; 0 for
// none.
func (b *Brain) useShot() int32 {
	w, ok := b.wornWeapon()
	if !ok {
		return 0
	}
	for _, id := range b.shotsFor(w) {
		if b.carried(id) > 0 {
			return id
		}
	}
	return 0
}

// buyShot is the shot template the shop errand buys for weapon w: the
// first of shotsFor's a shot source has, beginners' shots aside, so a
// mystic buys blessed spiritshots where it can and plain ones otherwise.
// Without a travel map it is the plain one, as merchants in sight sell.
func (b *Brain) buyShot(w weapon) int32 {
	table := b.shotTable()
	var last int32
	for _, id := range b.shotsFor(w) {
		if table[id].beginner {
			continue
		}
		if b.sourced(id) {
			return id
		}
		last = id
	}
	if b.cfg.Travel == nil {
		return last
	}
	return 0
}

// shotWants are the shots the shop errand buys (ShopPolicy.Soulshots or
// Spiritshots): per weapon of shotWeapons, its buyShot up to the stock's
// Max once fewer than Min of the shots it charges are carried.
func (b *Brain) shotWants() []buyRow {
	st := b.shotStock()
	var rows []buyRow
	for _, w := range b.shotWeapons() {
		id := b.buyShot(w)
		if id == 0 || slices.ContainsFunc(rows, func(r buyRow) bool { return r.item == id }) {
			continue
		}
		if have := b.carriedShots(w); have < st.Min && st.Max > have {
			rows = append(rows, buyRow{item: id, count: st.Max - have})
		}
	}
	return rows
}

// usableShot reports whether shot template id is one the wielded weapon
// or the one worn next charges.
func (b *Brain) usableShot(id int32) bool {
	return slices.ContainsFunc(b.shotWeapons(), func(w weapon) bool { return slices.Contains(b.shotsFor(w), id) })
}

// keepsShot reports whether the bot keeps shot template id rather than
// sell it: a beginner's shot, which no merchant buys, one a weapon of
// shotWeapons charges, or one a weapon kept to fall back to does
// (fallsBack). Every other shot is junk.
func (b *Brain) keepsShot(id int32) bool {
	if b.shotTable()[id].beginner || b.usableShot(id) {
		return true
	}
	w := b.worn()
	for _, it := range b.world.Inventory.Items {
		if b.fallsBack(it, &w) && slices.Contains(b.shotsFor(b.weaponOf(it.TemplateID)), id) {
			return true
		}
	}
	return false
}

// carriedOnly reports whether the wielded weapon in w is charged only by
// the shots the bot carries: no shot source has its shots.
func (b *Brain) carriedOnly(w *worn) bool {
	cur := w[slotWeapon]
	if cur.ObjectID == 0 || b.cfg.Travel == nil {
		return false
	}
	wp, ok := b.cfg.Travel.weapons[cur.TemplateID]
	return ok && b.shotsBacked(cur.TemplateID, wp, true) && !b.shotsBacked(cur.TemplateID, wp, false)
}

// fallsBack reports whether carried item it is a weapon the gear errand
// would wear over w once the carried shots the wielded weapon relies on
// run out (carriedOnly): it is kept, not junk, so the bot switches back.
func (b *Brain) fallsBack(it Item, w *worn) bool {
	g, ok := b.gearItem(it.TemplateID)
	if !ok || slotOf(g.part) != slotWeapon || it.Equipped || it.Augmentation != 0 || !b.persona.Gear.Equip || !b.carriedOnly(w) {
		return false
	}
	return b.beatsBy(it.TemplateID, w, false)
}

// shotAsk is an auto-use request the bot sent for a shot template: when,
// and whether it asked the template on.
type shotAsk struct {
	at time.Time
	on bool
}

// ruleShots turns auto use on for the shot template the wielded weapon
// wants (useShot), and off for any other the server has on: the server
// keeps a mismatched template on and stays silent on its failed charges
// (network/item_shot_use.go replyShotRejection). A template the server
// disabled (its stock ran out) stays off until a new stack arrives, one
// the bot disabled is turned on again when wanted. One toggle per template
// goes out per change of what the bot wants, repeated only when the server
// left it unanswered for shotRetry (handleAutoSoulShot,
// network/inventory.go); the frame pacer bounds the rest.
func (b *Brain) ruleShots(now time.Time) (Action, bool) {
	if !b.persona.AutoShots {
		return Action{}, false
	}
	inv := &b.world.Inventory
	// Only a repeat of the last request for a template, the same way,
	// waits for shotRetry.
	ready := func(id int32, on bool) bool {
		a, asked := b.shotAsked[id]
		return !asked || a.on != on || now.Sub(a.at) >= shotRetry
	}
	use := b.useShot()
	if on, known := inv.AutoShots[use]; use != 0 && (!known || !on && b.shotOff[use]) && ready(use, true) {
		return autoShotAction(use, true), true
	}
	for _, id := range slices.Sorted(maps.Keys(inv.AutoShots)) {
		if inv.AutoShots[id] && id != use && ready(id, false) {
			return autoShotAction(id, false), true
		}
	}
	return Action{}, false
}
