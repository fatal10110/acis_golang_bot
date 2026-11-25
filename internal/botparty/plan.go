package botparty

import (
	"slices"
	"strings"
	"time"
)

// Why a party broke or a member was dropped (Party.Why; the supervisor logs it).
const (
	WhyBadCommand    = "bad command"
	WhyLeaderCleared = "leader cleared its slot"
	WhyLeaderOff     = "leader ineligible"
	WhyLeaderGone    = "leader gone"
	WhyLeaderLeft    = "leader left the pin"
	WhyNoMember      = "no bot member"
	WhyNoHealer      = "no healer"
	WhyNoSpot        = "no spot"
	WhySpread        = "level spread"
	WhyDissolved     = "dissolved by the panel"
	WhyAutoOff       = "auto parties off"
	WhyTaken         = "a bot taken by the panel"
)

// Plan returns the organizer's next state and the orders that move the parties toward the groups
// policy wants, given its current state and every candidate bot, at now. It never mutates s.
//
// A pass forgets the parties the last one broke, tends every live party (timers, drops, breaks),
// fills short parties and forms new ones (composition.md, "Grouping"), orders every bot whose
// slot differs from its record (join first, lead once each join was answered nil), and sends
// unparty to every bot holding a slot no live record gives it.
func Plan(s State, cands []Candidate, p Policy, now time.Time) (State, []Order) {
	pl := &planner{
		p:       p,
		now:     now,
		st:      s.clone(),
		cands:   cands,
		by:      make(map[string]*Candidate, len(cands)),
		emitted: make(map[string]bool),
	}
	for i := range cands {
		pl.by[cands[i].Account] = &cands[i]
	}
	pl.expire()
	pl.pins()
	for i := range pl.st.Parties {
		pl.tend(&pl.st.Parties[i])
	}
	pl.group()
	for i := range pl.st.Parties {
		if pl.st.Parties[i].State != Broken {
			pl.order(&pl.st.Parties[i])
		}
	}
	pl.sweep()
	return pl.st, pl.orders
}

// planner is one pass of Plan.
type planner struct {
	p     Policy
	now   time.Time
	st    State
	cands []Candidate
	by    map[string]*Candidate
	// changed are the parties whose members changed this pass: they are not filled.
	changed map[int]bool
	orders  []Order
	emitted map[string]bool
}

// expire forgets the parties the last pass broke, and cool-downs and bans that ran out.
func (pl *planner) expire() {
	pl.st.Unpin = nil
	pl.st.Parties = slices.DeleteFunc(pl.st.Parties, func(pt Party) bool { return pt.State == Broken })
	for a, t := range pl.st.Freed {
		if pl.now.Sub(t) >= pl.p.Cooldown {
			delete(pl.st.Freed, a)
		}
	}
	for k, until := range pl.st.PairBans {
		if !pl.now.Before(until) {
			delete(pl.st.PairBans, k)
		}
	}
	pl.changed = make(map[int]bool)
}

// sent is the run's Sent; a new run has none.
func sent(c *Candidate) Sent {
	if c.NewRun {
		return Sent{}
	}
	return c.Sent
}

// pending reports an order not answered yet: the slot then says nothing and the bot gets no
// further order.
func pending(c *Candidate) bool { return sent(c).Answer == AnswerPending }

// refused reports o as the order Command.valid refused last: never resent.
func refused(c *Candidate, o Order) bool {
	s := sent(c)
	return s.Answer == AnswerBadCommand && s.Order.Kind == o.Kind && slices.Equal(s.Order.Names, o.Names)
}

// delivered reports a run that holds pt's slot by an answered order of pt's: from then on an
// empty slot means the bot cleared it itself.
func delivered(c *Candidate, pt *Party) bool {
	s := sent(c)
	if !pt.Ordered[c.Account] || s.Answer != AnswerOK {
		return false
	}
	if c.Account == pt.Leader.Account {
		return s.Order.Kind == OrderLead
	}
	return s.Order.Kind == OrderJoin && len(s.Order.Names) > 0 && s.Order.Names[0] == pt.Leader.Character
}

// cleared reports a run that cleared pt's slot itself.
func cleared(c *Candidate, pt *Party) bool {
	return !pending(c) && c.Slot == "" && delivered(c, pt)
}

// partyPreset reports a preset that takes part in bot parties.
func partyPreset(preset string) bool { return preset == PresetFarmer || preset == PresetSupport }

// offOnPurpose reports a bot made ineligible on purpose: mode off, a preset out of farmer and
// support, a standing or whispered order.
func offOnPurpose(c *Candidate) bool {
	return c.Mode == ModeOff || !partyPreset(c.Preset) || c.Order != "" || c.Whispered
}

// leftPin reports a pinned bot that leaves its pin: its delivered slot reads empty with PartyLeft
// left, or the panel set it off or out of farmer and support.
func leftPin(c *Candidate, pt *Party) bool {
	return c.Mode == ModeOff || !partyPreset(c.Preset) || cleared(c, pt) && c.PartyLeft == LeftLeft
}

// online is the candidate of account online in the world, nil for none.
func (pl *planner) online(account string) *Candidate {
	if c := pl.by[account]; c != nil && c.Online {
		return c
	}
	return nil
}

// live are the accounts of the parties not broken.
func (pl *planner) live() map[string]bool {
	out := make(map[string]bool)
	for i := range pl.st.Parties {
		if pl.st.Parties[i].State != Broken {
			for _, a := range pl.st.Parties[i].accounts() {
				out[a] = true
			}
		}
	}
	return out
}

// cooling reports a bot freed less than Cooldown ago.
func (pl *planner) cooling(account string) bool {
	t, ok := pl.st.Freed[account]
	return ok && pl.now.Sub(t) < pl.p.Cooldown
}

// since reports a timer set at least d ago.
func (pl *planner) since(t time.Time, d time.Duration) bool {
	return !t.IsZero() && pl.now.Sub(t) >= d
}

// setTimer sets *t to now the first pass cond holds and clears it while it fails.
func (pl *planner) setTimer(t *time.Time, cond bool) {
	switch {
	case !cond:
		*t = time.Time{}
	case t.IsZero():
		*t = pl.now
	}
}

// setMapTimer is setTimer for one account's timer of m.
func (pl *planner) setMapTimer(m map[string]time.Time, account string, cond bool) {
	t := m[account]
	pl.setTimer(&t, cond)
	if t.IsZero() {
		delete(m, account)
		return
	}
	m[account] = t
}

// tend runs the timers, drops and breaks of a live party.
func (pl *planner) tend(pt *Party) {
	if pt.GoneSince == nil {
		pt.GoneSince = make(map[string]time.Time)
	}
	if pt.BehindSince == nil {
		pt.BehindSince = make(map[string]time.Time)
	}
	if pt.Ordered == nil {
		pt.Ordered = make(map[string]bool)
	}
	leader := pl.by[pt.Leader.Account]
	lOnline := pl.online(pt.Leader.Account)
	if why := pl.badCommand(pt); why != "" {
		pl.breakParty(pt, why)
		return
	}
	switch {
	case pt.Dissolve != "":
		pl.breakParty(pt, pt.Dissolve)
		return
	case pl.st.AutoOff && !pt.Pinned:
		pl.breakParty(pt, WhyAutoOff)
		return
	}
	rng := int64(pl.p.Range) * int64(pl.p.Range)
	kept := pt.Members[:0]
	for _, m := range pt.Members {
		c := pl.by[m.Account]
		mOnline := c != nil && c.Online
		pl.setMapTimer(pt.GoneSince, m.Account, !mOnline)
		// An offline snapshot keeps a stale position: only both online count.
		pl.setMapTimer(pt.BehindSince, m.Account,
			mOnline && lOnline != nil && dist2(c.Position, lOnline.Position) > rng)
		if pl.dropped(pt, m, c) {
			delete(pt.GoneSince, m.Account)
			delete(pt.BehindSince, m.Account)
			delete(pt.Ordered, m.Account)
			pl.st.Freed[m.Account] = pl.now
			pl.changed[pt.ID] = true
			continue
		}
		kept = append(kept, m)
	}
	pt.Members = kept

	names := pt.memberNames()
	memberOnline, healer := false, false
	for _, m := range pt.Members {
		if c := pl.online(m.Account); c != nil {
			memberOnline = true
			healer = healer || c.HP > 0 && c.healer()
		}
	}
	leads := lOnline != nil && lOnline.Leader == lOnline.Character &&
		slices.ContainsFunc(lOnline.Members, func(n string) bool { return slices.Contains(names, n) })
	led := leader != nil && delivered(leader, pt)
	pl.setTimer(&pt.LeaderGoneSince, lOnline == nil || led && memberOnline && !leads)
	pl.setTimer(&pt.NoSpotSince, lOnline != nil && lOnline.Spot == "")
	pl.setTimer(&pt.NoHealerSince, !healer)
	if leads && pt.State == Forming {
		pt.State, pt.Since = Active, pl.now
	}

	if pt.Pinned {
		switch {
		case leader != nil && leftPin(leader, pt):
			pl.breakParty(pt, WhyLeaderLeft)
		case len(pt.Members) == 0:
			pl.breakParty(pt, WhyNoMember)
		}
		return
	}
	switch {
	case leader != nil && cleared(leader, pt):
		pl.breakParty(pt, WhyLeaderCleared)
	case leader != nil && (offOnPurpose(leader) || leader.Preset != PresetFarmer):
		pl.breakParty(pt, WhyLeaderOff)
	case pl.since(pt.LeaderGoneSince, pl.p.LeaderGone):
		pl.breakParty(pt, WhyLeaderGone)
	case len(pt.Members) == 0:
		pl.breakParty(pt, WhyNoMember)
	case pl.since(pt.NoHealerSince, pl.p.NoHealerFor):
		pl.breakParty(pt, WhyNoHealer)
	case pl.since(pt.NoSpotSince, pl.p.NoSpotFor):
		for _, m := range pt.Members {
			pl.st.PairBans[pairKey(pt.Leader.Account, m.Account)] = pl.now.Add(pl.p.RePairAfter)
		}
		pl.breakParty(pt, WhyNoSpot)
	case pl.spread(pt) > pl.p.MaxSpread:
		pl.breakParty(pt, WhySpread)
	}
}

// badCommand is WhyBadCommand when Command.valid refused an order of pt's.
func (pl *planner) badCommand(pt *Party) string {
	for _, a := range pt.accounts() {
		if c := pl.by[a]; c != nil && pt.Ordered[a] && sent(c).Answer == AnswerBadCommand {
			return WhyBadCommand
		}
	}
	return ""
}

// dropped reports a member pt drops this pass. An auto party drops a member that cleared its
// slot, was gone past LeaderGone, fell behind past BehindFor (the pair banned) or was made
// ineligible on purpose; a pin only one that left it.
func (pl *planner) dropped(pt *Party, m Member, c *Candidate) bool {
	if pt.Pinned {
		if c != nil && leftPin(c, pt) {
			pl.st.Unpin = append(pl.st.Unpin, m.Account)
			return true
		}
		return false
	}
	switch {
	case c != nil && (cleared(c, pt) || offOnPurpose(c)):
		return true
	case pl.since(pt.GoneSince[m.Account], pl.p.LeaderGone):
		return true
	case pl.since(pt.BehindSince[m.Account], pl.p.BehindFor):
		pl.st.PairBans[pairKey(pt.Leader.Account, m.Account)] = pl.now.Add(pl.p.RePairAfter)
		return true
	}
	return false
}

// spread is the level spread of the party's bot members with a live run.
func (pl *planner) spread(pt *Party) int32 {
	var lo, hi int32
	first := true
	for _, a := range pt.accounts() {
		c := pl.by[a]
		if c == nil {
			continue
		}
		if first {
			lo, hi, first = c.Level, c.Level, false
		}
		lo, hi = min(lo, c.Level), max(hi, c.Level)
	}
	return hi - lo
}

// breakParty breaks pt: every member is freed, a pin's rows are cleared, and the sweep sends
// unparty, the leader first. A leader the panel took into another party is left to that one.
func (pl *planner) breakParty(pt *Party, why string) {
	pt.State, pt.Why, pt.Since = Broken, why, pl.now
	for _, a := range pt.accounts() {
		if pl.inOther(pt, a) {
			continue
		}
		pl.st.Freed[a] = pl.now
		if pt.Pinned {
			pl.st.Unpin = append(pl.st.Unpin, a)
		}
	}
}

// inOther reports account a in a live party other than pt.
func (pl *planner) inOther(pt *Party, a string) bool {
	for i := range pl.st.Parties {
		if o := &pl.st.Parties[i]; o.ID != pt.ID && o.State != Broken && slices.Contains(o.accounts(), a) {
			return true
		}
	}
	return false
}

// memberNames are the character names of pt's members but the leader.
func (pt *Party) memberNames() []string {
	out := make([]string, 0, len(pt.Members))
	for _, m := range pt.Members {
		out = append(out, m.Character)
	}
	return out
}

// joinOrder is the join slot of member m of pt: the leader, then the other members.
func (pt *Party) joinOrder(m Member) Order {
	names := []string{pt.Leader.Character}
	for _, o := range pt.Members {
		if o.Account != m.Account {
			names = append(names, o.Character)
		}
	}
	return Order{Account: m.Account, Kind: OrderJoin, Names: names}
}

// order orders every bot of a live party whose slot differs from its record. Members get join
// first; the leader gets lead once each member with a live run holds its join, or at once when
// the lead only drops names (organizer.md, "Formation order"). While a pin's leader is offline
// its members get no join.
func (pl *planner) order(pt *Party) {
	lOnline := pl.online(pt.Leader.Account) != nil
	ready := true
	for _, m := range pt.Members {
		c := pl.by[m.Account]
		if c == nil {
			continue
		}
		if pl.awaiting(pt, c) {
			ready = false
			continue
		}
		o := pt.joinOrder(m)
		if pending(c) || c.Slot != o.Slot() {
			ready = false
		}
		if pending(c) || pt.Pinned && (!lOnline || c.Whispered) {
			continue
		}
		if c.Slot != o.Slot() {
			pl.emit(pt, c, o)
		}
	}
	leader := pl.by[pt.Leader.Account]
	if leader == nil || pl.awaiting(pt, leader) || pending(leader) || pt.Pinned && leader.Whispered {
		return
	}
	o := Order{Account: leader.Account, Kind: OrderLead, Names: pt.memberNames()}
	if leader.Slot == o.Slot() {
		return
	}
	if ready || subset(o.Names, slotNames(leader.Slot, OrderLead)) {
		pl.emit(pt, leader, o)
	}
}

// awaiting reports a bot of pt.Await that is not ready for pt's slot yet, and hands it unparty
// while it holds a slot with no order pending. It leaves Await once online with no slot, no
// order pending and no party.
func (pl *planner) awaiting(pt *Party, c *Candidate) bool {
	if !pt.Await[c.Account] {
		return false
	}
	switch {
	case pending(c):
	case c.Slot != "":
		pl.emit(nil, c, Order{Account: c.Account, Kind: OrderUnparty})
	case c.Online && c.Leader == "":
		delete(pt.Await, c.Account)
		return false
	}
	return true
}

// slotNames are the names of a slot of kind k, nil for another slot.
func slotNames(slot string, k OrderKind) []string {
	rest, ok := strings.CutPrefix(slot, k.String()+" ")
	if !ok {
		return nil
	}
	return strings.Split(rest, " ")
}

// subset reports every name of a in b.
func subset(a, b []string) bool {
	for _, n := range a {
		if !slices.Contains(b, n) {
			return false
		}
	}
	return true
}

// emit hands o to c, unless o is what Command.valid refused last; pt, when set, records it.
func (pl *planner) emit(pt *Party, c *Candidate, o Order) {
	if refused(c, o) || pl.emitted[c.Account] {
		return
	}
	if pt != nil {
		pt.Ordered[c.Account] = true
	}
	pl.emitted[c.Account] = true
	pl.orders = append(pl.orders, o)
}

// sweep sends unparty to every bot holding a slot that no live record gives it and no order
// pending: the broken parties' first, each leader first, then the rest in candidate order.
func (pl *planner) sweep() {
	live := pl.live()
	unparty := func(a string) {
		c := pl.by[a]
		if c == nil || live[a] || c.Slot == "" || pending(c) {
			return
		}
		pl.emit(nil, c, Order{Account: a, Kind: OrderUnparty})
	}
	for i := range pl.st.Parties {
		if pl.st.Parties[i].State == Broken {
			for _, a := range pl.st.Parties[i].accounts() {
				unparty(a)
			}
		}
	}
	for i := range pl.cands {
		unparty(pl.cands[i].Account)
	}
}
