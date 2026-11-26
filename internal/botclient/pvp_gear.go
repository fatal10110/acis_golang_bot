package botclient

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
)

const (
	// cureGreaterLevel is the character level from which the supply list
	// carries the greater cures too (docs/pvp/model.md "Gear and
	// consumables").
	cureGreaterLevel = 20
	// mixAge is how long a player seen stays in the mix table
	// (docs/pvp/model.md "Row table").
	mixAge = time.Hour
)

// supplyCures are the cures the supply list carries (PvPPolicy.CureStock):
// none at 0; else Antidotes and Bandages, and from cureGreaterLevel the
// Greater Antidotes and Emergency Dressings a merchant the shop errand can
// visit sells (soldNear).
func (b *Brain) supplyCures() []int32 {
	if b.persona.PvP.CureStock <= 0 {
		return nil
	}
	out := []int32{itemAntidote, itemBandage}
	if b.world.Self.Level >= cureGreaterLevel && b.cfg.Travel != nil {
		for _, id := range []int32{itemGreaterAntidote, itemEmergencyDressing} {
			if b.soldNear(id) {
				out = append(out, id)
			}
		}
	}
	return out
}

// keepsPvPItem reports whether the bot keeps template id for its PvP fights
// (D37): every cure while PvPPolicy.CureStock is above 0, the CP potions
// with PvPPolicy.KeepCPPotions.
func (b *Brain) keepsPvPItem(id int32) bool {
	p := &b.persona.PvP
	switch id {
	case itemAntidote, itemGreaterAntidote, itemBandage, itemEmergencyDressing:
		return p.CureStock > 0
	case itemCPPotion, itemGreaterCPPotion:
		return p.KeepCPPotions
	}
	return false
}

// pvpMix is the mix table (docs/pvp/model.md "Row table"), what PvP value
// weighs its rows by: per player seen in a CharInfo, its class, row, name
// and when it was last seen, for mixAge. The brain goroutine owns it; it is
// kept only while PvPPolicy.GearWeight is above 0.
type pvpMix struct {
	seen map[int32]mixSeen
	// now is the time of the last event the brain handled, what the table
	// ages by.
	now time.Time
	// version changes whenever the mix rows may have: a player new to the
	// table or playing another class or row or under another name, one
	// aged out. The gear model's PvP side is made again only when it moved.
	version uint64
}

// mixSeen is one player of the mix table.
type mixSeen struct {
	class int32
	row   charmodel.Row
	name  string
	at    time.Time
}

// mixRow is a class and the row it plays: one share of the mix, its
// reference opponent the class template.
type mixRow struct {
	class int32
	row   charmodel.Row
}

// see records player e, playing row, as seen at now; a player new to the
// table first drops the ones seen longer than mixAge ago.
func (x *pvpMix) see(e CharInfo, row charmodel.Row, now time.Time) {
	old, ok := x.seen[e.ObjectID]
	if !ok {
		x.prune(now)
		if x.seen == nil {
			x.seen = make(map[int32]mixSeen)
		}
	}
	if !ok || old.class != e.ClassID || old.row != row || old.name != e.Name {
		x.version++
	}
	x.seen[e.ObjectID] = mixSeen{class: e.ClassID, row: row, name: e.Name, at: now}
}

// prune drops the players seen longer than mixAge before now.
func (x *pvpMix) prune(now time.Time) {
	n := len(x.seen)
	maps.DeleteFunc(x.seen, func(_ int32, s mixSeen) bool { return now.Sub(s.at) > mixAge })
	if len(x.seen) != n {
		x.version++
	}
}

// mixRows are the rows of the mix and how many players play each: the
// players of the mix table seen within mixAge of the last event, a
// bot-marked one never (D38); with none, one of each row of the bot's
// profession tier (charmodel.RowClasses).
func (b *Brain) mixRows() map[mixRow]int {
	x := &b.pvp.mix
	x.prune(x.now)
	out := make(map[mixRow]int)
	for _, s := range x.seen {
		if !b.cfg.markers.Marks(s.name) {
			out[mixRow{s.class, s.row}]++
		}
	}
	if len(out) == 0 {
		for _, id := range charmodel.RowClasses(classTier(b.world.Self.ClassID)) {
			if row, ok := charmodel.ClassRow(id, false); ok {
				out[mixRow{id, row}] = 1
			}
		}
	}
	return out
}

// gearPvP is the PvP side of the gear model's comparisons
// (docs/pvp/model.md "Blend"): the persona's weight, the mix rows it was
// made for and their shares with their reference opponents (the class
// template at the bot's level), the PvP row table, and the PvP value of
// what the bot wears now (norm) and of each arrangement asked for. version
// and tier are the mix table's version and the bot's profession tier it
// was last checked at.
type gearPvP struct {
	weight  float64
	version uint64
	tier    int
	rows    map[mixRow]int
	mix     []charmodel.Share
	table   charmodel.RowTable
	norm    float64
	values  map[gearValueKey]float64
}

// gearPvP keeps v's PvP side: none while PvPPolicy.GearWeight is 0; made
// again when the mix rows change (mixRows). The rows are read again only
// when the mix table's version or the bot's profession tier (the rows with
// nobody seen) moved since the last check, so an own event that bumps the
// world's generation costs no more than with no PvP side (#494). A player
// seen in a CharInfo reaches the PvP side at the next event that does.
func (b *Brain) gearPvP(m *TravelMap, v *gearModel) {
	weight := b.persona.PvP.GearWeight
	if weight <= 0 {
		v.pvp = nil
		return
	}
	x := &b.pvp.mix
	x.prune(x.now)
	tier := classTier(b.world.Self.ClassID)
	if p := v.pvp; p != nil && p.weight == weight && p.version == x.version && p.tier == tier {
		return
	}
	rows := b.mixRows()
	if p := v.pvp; p != nil && p.weight == weight && maps.Equal(p.rows, rows) {
		p.version, p.tier = x.version, tier
		return
	}
	p := &gearPvP{weight: weight, version: x.version, tier: tier, rows: rows, mix: mixShares(m, rows, v.build.Level), table: b.pvpRows(), values: make(map[gearValueKey]float64)}
	v.pvp = p
	now := b.rawWorn()
	p.norm = v.pvpValue(m, &now, true, v.build.Dyes)
}

// mixShares are the shares of the mix rows, each of the players playing
// it, with its reference opponent: its class template at character level
// (docs/pvp/model.md "Engage estimate"), no gear. A class the travel map
// lacks is left out.
func mixShares(m *TravelMap, rows map[mixRow]int, level int) []charmodel.Share {
	known := slices.SortedFunc(maps.Keys(rows), func(x, y mixRow) int {
		return cmp.Or(cmp.Compare(x.class, y.class), cmp.Compare(x.row.ID, y.row.ID), boolCmp(x.row.Bow, y.row.Bow))
	})
	known = slices.DeleteFunc(known, func(r mixRow) bool { _, ok := m.classes[r.class]; return !ok })
	total := 0
	for _, r := range known {
		total += rows[r]
	}
	var out []charmodel.Share
	for _, r := range known {
		opp := charmodel.ModelFighter(charmodel.Build{Class: m.classes[r.class], Level: level, Mage: charmodel.MageClass(m.classes, r.class)})
		opp.Row = r.row
		out = append(out, charmodel.Share{Opponent: opp, Share: float64(rows[r]) / float64(total)})
	}
	return out
}

// boolCmp orders false before true.
func boolCmp(x, y bool) int {
	switch {
	case x == y:
		return 0
	case x:
		return 1
	}
	return -1
}

// pvpValue is PvP value (charmodel.PvPValue) of arrangement w with dyes
// over v's mix, cached like value.
func (v *gearModel) pvpValue(m *TravelMap, w *worn, sets bool, dyes charmodel.Attrs) float64 {
	p := v.pvp
	k := gearValueKey{worn: keyOf(w), sets: sets, dyes: dyes}
	if x, ok := p.values[k]; ok {
		return x
	}
	x := charmodel.PvPValue(v.fighter(m, w, sets, dyes), p.mix, p.table)
	p.values[k] = x
	return x
}

// fighter is the bot as a PvP fighter wearing arrangement w with dyes: its
// calibrated profile, CP, CON, MEN and the vulnerabilities and PvP bonuses
// of what it wears (a complete set's skills counted when sets is set), and
// its class's row by the weapon (a bow makes a scout an archer).
func (v *gearModel) fighter(m *TravelMap, w *worn, sets bool, dyes charmodel.Attrs) charmodel.Fighter {
	f := charmodel.ModelFighter(v.buildOf(m, w, sets, dyes))
	f.Profile = v.calib.Apply(f.Profile)
	_, bow := m.arrowsOf(w[slotWeapon].TemplateID)
	f.Row, _ = charmodel.ClassRow(v.in.key.class, bow)
	return f
}

// delta is what arrangement w2 with dyes d2 adds over w1 with d1, in
// value's unit: the value difference, blended with the PvP value
// difference by PvPPolicy.GearWeight while it is above 0
// (charmodel.Blend, the value of what the bot wears now the norm).
func (v *gearModel) delta(m *TravelMap, w2 *worn, sets2 bool, d2 charmodel.Attrs, w1 *worn, sets1 bool, d1 charmodel.Attrs) float64 {
	dv := v.value(m, w2, sets2, d2) - v.value(m, w1, sets1, d1)
	p := v.pvp
	if p == nil {
		return dv
	}
	dp := v.pvpValue(m, w2, sets2, d2) - v.pvpValue(m, w1, sets1, d1)
	return charmodel.Blend(v.norm, dv, p.norm, dp, p.weight)
}
