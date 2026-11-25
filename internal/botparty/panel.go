package botparty

import (
	"slices"
	"time"
)

// Form returns s with a party the panel formed by hand at now: leader leads members, a pin of
// group when pin is set, else an auto party that band, composition and every break govern
// (docs/parties/panel.md). Each named bot leaves any other party as a drop without cool-down: a
// party it led or whose last bot member it was breaks on the next pass, and a pin's bot that left
// it is listed in Unpin. The new party orders each bot only once it holds no slot, has no order
// pending and is online in no party (Party.Await). The caller checked the bots; Form never
// mutates s and returns the new party's id.
func Form(s State, leader Member, members []Member, pin bool, group string, now time.Time) (State, int) {
	st := s.clone()
	st.Unpin = append([]string(nil), s.Unpin...)
	taken := make([]string, 0, 1+len(members))
	taken = append(taken, leader.Account)
	for _, m := range members {
		taken = append(taken, m.Account)
	}
	for i := range st.Parties {
		pt := &st.Parties[i]
		if pt.State == Broken {
			continue
		}
		if slices.Contains(taken, pt.Leader.Account) {
			pt.Dissolve = WhyTaken
		}
		kept := pt.Members[:0]
		for _, m := range pt.Members {
			if !slices.Contains(taken, m.Account) {
				kept = append(kept, m)
				continue
			}
			delete(pt.GoneSince, m.Account)
			delete(pt.BehindSince, m.Account)
			delete(pt.Ordered, m.Account)
			delete(pt.Await, m.Account)
			if pt.Pinned {
				st.Unpin = append(st.Unpin, m.Account)
			}
		}
		pt.Members = kept
		if len(pt.Members) == 0 {
			pt.Dissolve = WhyTaken
		}
		if pt.Pinned && slices.Contains(taken, pt.Leader.Account) {
			st.Unpin = append(st.Unpin, pt.Leader.Account)
		}
	}
	for _, a := range taken {
		delete(st.Freed, a)
	}
	await := make(map[string]bool, len(taken))
	for _, a := range taken {
		await[a] = true
	}
	pt := Party{
		ID:          st.NextID,
		Leader:      leader,
		Members:     append([]Member(nil), members...),
		FormedAt:    now,
		Since:       now,
		Pinned:      pin,
		GoneSince:   map[string]time.Time{},
		BehindSince: map[string]time.Time{},
		Ordered:     map[string]bool{},
		Await:       await,
	}
	if pin {
		pt.Group = group
	}
	st.NextID++
	st.Parties = append(st.Parties, pt)
	return st, pt.ID
}

// Dissolve returns s with the live party id marked for the next pass to break, as the panel's
// dissolve: an unparty to each member, the leader first, and a pin's rows cleared. It reports
// false for no live party of that id. It never mutates s.
func Dissolve(s State, id int) (State, bool) {
	i := slices.IndexFunc(s.Parties, func(pt Party) bool { return pt.ID == id && pt.State != Broken })
	if i < 0 {
		return s, false
	}
	st := s.clone()
	st.Unpin = append([]string(nil), s.Unpin...)
	st.Parties[i].Dissolve = WhyDissolved
	return st, true
}
