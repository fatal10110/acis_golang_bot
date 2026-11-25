package botparty

import (
	"cmp"
	"slices"
	"time"
)

// group is a party being formed or filled this pass.
type group struct {
	// pt is the short party filled, nil for a new one.
	pt   *Party
	lead *Candidate
	// accounts are the bot members, the leader first, and levels those with a live run.
	accounts []string
	levels   []int32
	added    []*Candidate
	// take is how many members the group may still take this pass.
	take int
}

func (g *group) add(c *Candidate) {
	g.accounts = append(g.accounts, c.Account)
	g.levels = append(g.levels, c.Level)
	g.added = append(g.added, c)
	g.take--
}

// size is the party size grouping fills to.
func (pl *planner) size() int { return min(max(pl.p.Size, 2), MaxSize) }

// fits reports c a member of g: in its leader's town within MaxTravel, within Band of every bot
// member, banned with none.
func (pl *planner) fits(g *group, c *Candidate) bool {
	if c.Town == nil || g.lead.Town == nil || *c.Town != *g.lead.Town {
		return false
	}
	reach := int64(pl.p.MaxTravel) * int64(pl.p.MaxTravel)
	if dist2(c.Position, g.lead.Position) > reach {
		return false
	}
	for _, l := range g.levels {
		if d := l - c.Level; d > pl.p.Band || -d > pl.p.Band {
			return false
		}
	}
	for _, a := range g.accounts {
		if _, ok := pl.st.PairBans[pairKey(a, c.Account)]; ok {
			return false
		}
	}
	return true
}

// nearest is the candidate of pool nearest g's leader that is unused, wanted and fits, nil for
// none.
func (pl *planner) nearest(g *group, pool []*Candidate, used map[string]bool, want func(*Candidate) bool) *Candidate {
	var best *Candidate
	var bestD int64
	for _, c := range pool {
		if used[c.Account] || !want(c) || !pl.fits(g, c) {
			continue
		}
		d := dist2(c.Position, g.lead.Position)
		if best == nil || d < bestD || d == bestD && c.Account < best.Account {
			best, bestD = c, d
		}
	}
	return best
}

// group forms new parties and fills short ones (composition.md, "Grouping") from the eligible
// bots with a known name in no party, holding no slot, with no order pending and none refused
// (never resent), not cooling down. Damage candidates from the highest level down each take the
// nearest free healer, else are healerless; the first FormPerPass groups and the short auto
// parties then, in the same order, take the nearest free Support candidate, else the nearest
// healerless Damage candidate. A waiting group's members are neither; a short party takes one
// member a pass.
func (pl *planner) group() {
	live := pl.live()
	var free []*Candidate
	for i := range pl.cands {
		c := &pl.cands[i]
		if c.Online && c.Character != "" && !offOnPurpose(c) && c.Town != nil && c.Group == "" && c.Leader == "" &&
			c.Slot == "" && !pending(c) && sent(c).Answer != AnswerBadCommand && !live[c.Account] &&
			!pl.cooling(c.Account) {
			free = append(free, c)
		}
	}
	slices.SortStableFunc(free, func(a, b *Candidate) int {
		return cmp.Or(cmp.Compare(b.Level, a.Level), cmp.Compare(a.Account, b.Account))
	})
	size := pl.size()
	used := make(map[string]bool)
	var groups []*group
	var healerless []*Candidate
	for _, d := range free {
		if d.Preset != PresetFarmer {
			continue
		}
		g := &group{lead: d, accounts: []string{d.Account}, levels: []int32{d.Level}, take: size - 1}
		h := pl.nearest(g, free, used, (*Candidate).healer)
		if h == nil {
			healerless = append(healerless, d)
			continue
		}
		used[d.Account], used[h.Account] = true, true
		g.add(h)
		groups = append(groups, g)
	}
	fillers := groups[:min(len(groups), max(pl.p.FormPerPass, 0))]
	for i := range pl.st.Parties {
		pt := &pl.st.Parties[i]
		if pt.State == Broken || pt.Pinned || pl.changed[pt.ID] || 1+len(pt.Members) >= size {
			continue
		}
		lead := pl.online(pt.Leader.Account)
		if lead == nil || lead.Town == nil {
			continue
		}
		g := &group{pt: pt, lead: lead, accounts: pt.accounts(), take: 1}
		for _, a := range g.accounts {
			if c := pl.by[a]; c != nil {
				g.levels = append(g.levels, c.Level)
			}
		}
		fillers = append(fillers, g)
	}
	slices.SortStableFunc(fillers, func(a, b *group) int {
		return cmp.Or(cmp.Compare(b.lead.Level, a.lead.Level), cmp.Compare(a.lead.Account, b.lead.Account))
	})
	support := func(c *Candidate) bool { return c.Preset == PresetSupport }
	anyone := func(*Candidate) bool { return true }
	for progress := true; progress; {
		progress = false
		for _, g := range fillers {
			if g.take <= 0 || len(g.accounts) >= size {
				continue
			}
			c := pl.nearest(g, free, used, support)
			if c == nil {
				c = pl.nearest(g, healerless, used, anyone)
			}
			if c == nil {
				continue
			}
			used[c.Account] = true
			g.add(c)
			progress = true
		}
	}
	// Fills first: a new party's append moves the parties the fills point into.
	for _, g := range fillers {
		if g.pt != nil {
			g.pt.Members = append(g.pt.Members, members(g.added)...)
		}
	}
	for _, g := range fillers {
		if g.pt == nil {
			pl.st.Parties = append(pl.st.Parties, Party{
				ID:          pl.st.NextID,
				Leader:      Member{Account: g.lead.Account, Character: g.lead.Character},
				Members:     members(g.added),
				FormedAt:    pl.now,
				GoneSince:   map[string]time.Time{},
				BehindSince: map[string]time.Time{},
				Ordered:     map[string]bool{},
			})
			pl.st.NextID++
		}
	}
}

// members are cs as party members.
func members(cs []*Candidate) []Member {
	out := make([]Member, 0, len(cs))
	for _, c := range cs {
		out = append(out, Member{Account: c.Account, Character: c.Character})
	}
	return out
}

// pins adds the candidates of a pin's group to its live party and forms the pins whose leader
// has a live run and a member: whatever band and composition say.
func (pl *planner) pins() {
	live := pl.live()
	// A bot that left its pin this run waits for the supervisor to clear its rows.
	pinnable := func(c *Candidate) bool {
		return c.Group != "" && c.Character != "" && !live[c.Account] && c.Mode != ModeOff && partyPreset(c.Preset) &&
			(c.Slot != "" || c.PartyLeft != LeftLeft)
	}
	byGroup := make(map[string]int)
	for i := range pl.st.Parties {
		if pt := &pl.st.Parties[i]; pt.Pinned && pt.State != Broken {
			byGroup[pt.Group] = i
		}
	}
	for i := range pl.cands {
		c := &pl.cands[i]
		if j, ok := byGroup[c.Group]; ok && pinnable(c) {
			pl.st.Parties[j].Members = append(pl.st.Parties[j].Members, Member{Account: c.Account, Character: c.Character})
			live[c.Account] = true
		}
	}
	for i := range pl.cands {
		l := &pl.cands[i]
		if _, ok := byGroup[l.Group]; ok || !l.Lead || !pinnable(l) {
			continue
		}
		pt := Party{
			ID:          pl.st.NextID,
			Leader:      Member{Account: l.Account, Character: l.Character},
			FormedAt:    pl.now,
			Pinned:      true,
			Group:       l.Group,
			GoneSince:   map[string]time.Time{},
			BehindSince: map[string]time.Time{},
			Ordered:     map[string]bool{},
		}
		for j := range pl.cands {
			if c := &pl.cands[j]; c != l && c.Group == l.Group && pinnable(c) {
				pt.Members = append(pt.Members, Member{Account: c.Account, Character: c.Character})
			}
		}
		if len(pt.Members) == 0 {
			continue
		}
		pl.st.NextID++
		pl.st.Parties = append(pl.st.Parties, pt)
		byGroup[pt.Group] = len(pl.st.Parties) - 1
		for _, a := range pt.accounts() {
			live[a] = true
		}
	}
}
