package botclient

import (
	"cmp"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// earnMargin is the earning mode's survival bound (#527): the bot lasts
// against a spot's likely pack at least as long as it takes to kill one of
// its monsters, every monster counted (packFight with margin 1), where
// packFits asks its row's margin (1.5 to 3) of the aggressive ones only.
const earnMargin = 1.0

// indexKillValues is what a kill of each NPC template is worth in adena:
// its CURRENCY categories' adena at face value, and every item of its DROP
// categories at what a merchant pays for it, half its reference price
// (server inventory/sell.go), each by its category's chance times its own
// and its mean quantity, as supplyPerHour weighs them. SPOIL and HERB drops
// count nothing: a kill alone does not take them. An item a bot keeps is
// counted at its sell price all the same: what it saves the bot buying.
// Templates worth nothing are left out.
func indexKillValues(npcs map[int32]datapack.NPC, prices map[int32]int32) map[int32]float64 {
	out := make(map[int32]float64)
	for id, n := range npcs {
		v := 0.0
		for _, cat := range n.Drops {
			if cat.Kind != datapack.DropKindDrop && cat.Kind != datapack.DropKindCurrency {
				continue
			}
			for _, d := range cat.Drops {
				each := 1.0
				if d.ItemID != adenaID {
					each = float64(prices[d.ItemID] / 2)
				}
				v += cat.Chance * d.Chance / 10000 * float64(d.Min+d.Max) / 2 * each
			}
		}
		if v > 0 {
			out[id] = v
		}
	}
	return out
}

// dropShare is the share of its drops a monster of level mob gives a
// killer of level killer: a monster more than 5 levels under its killer
// cuts them by 18 % a level, to 10 % at least (server item/drop.go
// LevelPenaltyMultiplier).
func dropShare(killer, mob int) float64 {
	gap := killer - mob - 5
	if gap <= 0 {
		return 1
	}
	return max(0.1, 1-0.18*float64(gap))
}

// earnRate is the adena an hour the bot earns farming s by its earning
// mode's survival bound (earnMargin): the worth of a kill (killValues, cut
// by dropShare) averaged over s's monsters by their spawn totals, over its
// mean kill time (packFight over every monster, aggressive or not). Not ok
// when the bot cannot last a kill there, packFight cannot judge it (stats
// not in, an unpriced caster) or no travel map prices its drops. The spawn
// rate is left out: a bot under a death penalty kills slower than a spot
// respawns.
func (b *Brain) earnRate(s *datapack.Spot) (float64, bool) {
	m := b.cfg.Travel
	if m == nil || len(s.Monsters) == 0 {
		return 0, false
	}
	bd := b.build()
	all := *s
	all.Mobs = s.Monsters
	kill, die, ok := b.packFight(&all, &bd, earnMargin, &crew{})
	if !ok || kill <= 0 || die < earnMargin*kill {
		return 0, false
	}
	level := int(b.world.Self.Level)
	var n, v float64
	for _, mob := range s.Monsters {
		c := float64(mob.Count)
		n += c
		v += c * m.killValues[mob.NPCID] * dropShare(level, mob.Level)
	}
	if n == 0 {
		return 0, false
	}
	return v / n * 3600 / kill, true
}

// earnFits reports whether s may be an earning spot by its rate (claimEarn
// step 1): not above the bot's level window (SpotPolicy.Above) and no pack
// a death taught it to pass over.
func (b *Brain) earnFits(s *datapack.Spot, now time.Time) bool {
	self := &b.world.Self
	level := int(self.Level)
	return s.MaxLevel <= level+b.persona.Spots.Above && !b.spot.pack.covers(s, level, self.RunSpeed, now)
}

// earnOrder sorts cands (spots in reach, none avoided) for claimEarn: rich
// are those earnFits passes that earn anything (earnRate), the best paid
// first; low are all of cands, the lowest monsters first (MaxLevel), within
// MaxTravel before beyond it, then the nearest.
func (b *Brain) earnOrder(cands []spotCand, now time.Time) (rich, low []int) {
	r, p := b.cfg.Spots, &b.persona.Spots
	type paid struct {
		c    spotCand
		rate float64
	}
	var ranked []paid
	for _, c := range cands {
		sp := &r.spots[c.i]
		if !b.earnFits(sp, now) {
			continue
		}
		if rate, ok := b.earnRate(sp); ok && rate > 0 {
			ranked = append(ranked, paid{c, rate})
		}
	}
	slices.SortStableFunc(ranked, func(x, y paid) int { return cmp.Or(cmp.Compare(y.rate, x.rate), cmp.Compare(x.c.d, y.c.d)) })
	for _, e := range ranked {
		rich = append(rich, e.c.i)
	}
	beyond := func(c spotCand) int {
		if c.line > float64(p.MaxTravel) {
			return 1
		}
		return 0
	}
	sorted := slices.Clone(cands)
	slices.SortStableFunc(sorted, func(x, y spotCand) int {
		return cmp.Or(cmp.Compare(r.spots[x.i].MaxLevel, r.spots[y.i].MaxLevel), cmp.Compare(beyond(x), beyond(y)), cmp.Compare(x.d, y.d))
	})
	for _, c := range sorted {
		low = append(low, c.i)
	}
	return rich, low
}

// claimEarn is pickSpot's last fallback, the earning mode (#527): a bot
// with a death penalty, no spot of its level that fits, no trip and no
// fallback under its level window (claimLower) would stand forever, too
// weak for every spot and too poor for a Scroll: Recovery. It claims (1)
// the spot in reach that earns it the most adena an hour by a looser
// survival bound (earnOrder rich, earnRate), else (2) the one with the
// lowest monsters (earnOrder low), whatever its pack. Both honour the
// spots it avoids and claim under their caps (claimN, claimTiers), so
// penalized bots do not stack on a newbie spot past its cap; with every
// one full it claims none and looks again after spotRetry. A bot holding
// an earning spot keeps it while its penalty lasts; pickSpot looks every
// spotRetry for a spot that fits, which ends the mode, as does the penalty
// lifted: the spot it holds is then an ordinary one. What it earns buys
// Scrolls: Recovery first (penaltyTrip, recoveryWants). It reports whether
// the bot holds an earning spot.
func (b *Brain) claimEarn(here navPoint, now time.Time) bool {
	r, v := b.cfg.Spots, &b.spot
	if b.world.Self.DeathPenalty <= 0 {
		v.earning = false
		return false
	}
	if v.held && v.earning {
		return true
	}
	all := b.candsWhere(here, now, func(*datapack.Spot, time.Time) bool { return true })
	rich, low := b.earnOrder(all, now)
	i, ok := r.claimN(rich, 1)
	if !ok {
		i, ok = r.claimTiers(low, 1)
	}
	if !ok {
		return false
	}
	b.trip.onward = 0
	b.leaveSpot()
	v.idx, v.held, v.earning, v.at = i, true, true, candAt(all, i)
	return true
}
