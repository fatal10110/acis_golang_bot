package botclient

import (
	"cmp"
	"fmt"
	"slices"
	"sync/atomic"
	"time"
)

// statusEvery is the least time between two snapshots a session's brain
// publishes.
const statusEvery = time.Second

// BotState is where a Bot is in its loop of sessions.
type BotState uint8

// The states of a Bot.
const (
	// StateNew is a bot whose Run has not started.
	StateNew BotState = iota
	// StateWaiting waits for its turn of the login gate.
	StateWaiting
	// StateEntering logs in and enters the world.
	StateEntering
	// StateOnline plays a session in the world.
	StateOnline
	// StateReconnecting waits before its next attempt; Status.Err is why
	// the last one ended.
	StateReconnecting
	// StateStopped is a bot whose Run returned; Status.Err is why.
	StateStopped
)

var stateNames = [...]string{
	StateNew:          "new",
	StateWaiting:      "waiting for the gate",
	StateEntering:     "entering",
	StateOnline:       "online",
	StateReconnecting: "reconnecting",
	StateStopped:      "stopped",
}

func (s BotState) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("BotState(%d)", uint8(s))
}

// Status is a snapshot of a Bot (Bot.Status). It is immutable once
// published: Members, Crafter and Build are shared by every copy and must not be
// changed.
// Between sessions it keeps what the last session saw of the character.
type Status struct {
	// At is when it was taken, on the bot's clock.
	At    time.Time
	State BotState
	// Character is the character's name, "" until known.
	Character      string
	ClassID, Level int32
	HP, MaxHP      int32
	MP, MaxMP      int32
	CP, MaxCP      int32
	Position       Point
	// RunSpeed is the run speed the server gave last, its penalties
	// applied (UserInfo), and Running whether the bot runs rather than
	// walks.
	RunSpeed int32
	Running  bool
	// Load and MaxLoad are the carried weight and the weight limit, 0
	// until known; Weight is Load in percent of MaxLoad, and WeightPenalty
	// the server's weight band for it (weightBand, 0-4: 2 and 3 halve the
	// move speed, 4 stops the bot).
	Load, MaxLoad int32
	Weight        int32
	WeightPenalty int32
	// Stash is how the last half-speed stash ended (docs/items.md section
	// 7, #449): the deposit, in the bot's own warehouse, of the kept items
	// it does not use while its load is in the half-speed band.
	// WarehouseUnreachable reports no warehouse keeper (none on the travel
	// map, or none reached), WarehouseShort the fee unpaid, WarehouseFull
	// no slot; zero before any.
	Stash WarehouseResult
	// GradePenalty is set while the server holds the grade penalty on the
	// bot: a worn piece above its Expertise grade slows it (skill 4267).
	// DeathPenalty is its death penalty level (0-15).
	GradePenalty bool
	DeathPenalty int32
	// Town is the town of the travel map nearest to Position, nil without
	// a travel map (BrainConfig.Travel).
	Town *Point
	// Preset is the persona preset the bot plays (BotConfig.Preset).
	Preset string
	// Rule is the rule of the latest decision while online.
	Rule string
	// Spot is the npcmaker name of the farming spot the bot holds in its
	// spot registry (BrainConfig.Spots), "" while it holds none.
	// Earning is set while that spot is an earning spot (the earning mode,
	// #527): a bot under a death penalty that no spot fits farms it for
	// the adena a Scroll: Recovery costs.
	Spot    string
	Earning bool
	// Walk is "unreachable: <reason>" while the bot's latest walk ended
	// with no route that holds (blocked, desync, door, watchdog, no route,
	// isolated, no nav), "" otherwise (docs/navigation.md, "Walker").
	Walk string
	// Leader is the party leader's name, the bot's own when it leads, and
	// Members the other members' names in order; empty outside a party.
	Leader  string
	Members []string
	// ClanID is the bot's clan, 0 for none, and ClanLeader its leader's
	// name, "" until the clan's roster arrived.
	ClanID     int32
	ClanLeader string
	// Crafter is the crafter's cycle while online, nil for a bot that
	// plays none (BrainConfig.CraftBoard, Persona.Crafter) and offline.
	Crafter *CrafterStatus
	// Build is the build the bot plays (BuildStatus), nil for a persona
	// with no role and before the first session's entry.
	Build *BuildStatus
	// PvP is the bot's PvP view (PvPStatus) while online; between sessions
	// it keeps the last session's.
	PvP *PvPStatus
	// Order is the standing order of the panel, the zero Command for none.
	Order Command
	// Party is the party slot, "lead A B" or "join L A B", "" for none
	// (docs/parties/orders.md).
	Party string
	// Whispered is true while a GM's whispered follow, assist, stay or go
	// holds the session: a lead or a join then answers ErrWhispered.
	Whispered bool
	// PartyLeft is why the brain last cleared its own party slot this run,
	// "left" or "timeout"; "" while a slot is set or after an unparty.
	PartyLeft string
	// Heals is true while the skill list holds the heal of the bot's row
	// (docs/parties/composition.md, "Healer"): 1011, 1015 or 1027 for a
	// mystic, Cleric, Elven Oracle or Shillien Oracle line, 1229 for an
	// Orc Shaman line. Between sessions it keeps the last session's.
	Heals bool
	// Reconnects counts the bot's attempts after its first.
	Reconnects int
	// Err is why the latest attempt ended, or why the bot stopped; nil
	// while none ended.
	Err error
}

// BuildStatus is the build a bot plays (Brain.build, docs/strategy.md
// section 10): its role, the first profession its line heads for or plays
// (Build.Branch, 0 for none) and its kit's skill ids in their kit order.
type BuildStatus struct {
	Role   Role
	Branch int32
	// Skills are the skills the kit learns at a trainer; Rotation, Buffs,
	// Heals and Toggles the skills it casts.
	Skills, Rotation, Buffs, Heals, Toggles []int32
}

// buildStatus is bd's BuildStatus; its slices are its own, since a
// published Status is shared.
func buildStatus(bd Build) *BuildStatus {
	k := &bd.Kit
	v := &BuildStatus{Role: bd.Role, Branch: bd.Branch, Skills: slices.Clone(k.Skills)}
	for _, s := range k.Rotation {
		v.Rotation = append(v.Rotation, s.Skill)
	}
	for _, s := range k.Buffs {
		v.Buffs = append(v.Buffs, s.Skill)
	}
	for _, s := range k.Heals {
		v.Heals = append(v.Heals, s.Skill)
	}
	for _, s := range k.Toggles {
		v.Toggles = append(v.Toggles, s.Skill)
	}
	return v
}

// statusBoard is a Bot's published snapshot. Run's goroutine is its one
// writer; any goroutine reads it.
type statusBoard struct {
	cur atomic.Pointer[Status]
	// at is the brain's last publication, owned by the writer.
	at time.Time
}

func (s *statusBoard) load() Status    { return *s.cur.Load() }
func (s *statusBoard) store(st Status) { s.cur.Store(&st) }

// Status is the bot's latest snapshot. It is safe from any goroutine.
func (b *Bot) Status() Status { return b.link.board.load() }

// publish publishes the bot's offline state, keeping what the last session
// saw; err, when set, becomes the snapshot's error. It runs on Run's
// goroutine.
func (b *Bot) publish(state BotState, err error) {
	st := b.link.board.load()
	st.At, st.State, st.Rule, st.Spot, st.Walk, st.Crafter = b.cfg.Clock.Now(), state, "", "", "", nil
	st.Character, st.Order, st.Reconnects = cmp.Or(b.char.Name, st.Character), b.link.order.cmd, b.reconnects
	st.Party, st.Whispered, st.PartyLeft = b.link.party.String(), false, b.link.partyLeft
	if err != nil {
		st.Err = err
	}
	b.link.board.store(st)
}

// publishStatus publishes the bot's snapshot online at now: at once when
// force, else at most once every statusEvery.
func (b *Brain) publishStatus(now time.Time, force bool) {
	l := b.cfg.link
	if l == nil || !force && now.Sub(l.board.at) < statusEvery {
		return
	}
	l.board.at = now
	st, w := l.board.load(), b.world
	s := &w.Self
	st.At, st.State, st.Rule, st.Order = now, StateOnline, b.lastRule, l.order.cmd
	st.Party, st.Whispered, st.PartyLeft = l.party.String(), b.orders.whispered, l.partyLeft
	st.Spot, st.Earning, st.Walk = "", false, b.walk.failed()
	if b.spot.farming() {
		st.Spot, st.Earning = b.cfg.Spots.spots[b.spot.idx].Maker, b.spot.earning
	}
	st.Character, st.ClassID, st.Level = s.Name, s.ClassID, s.Level
	st.HP, st.MaxHP, st.MP, st.MaxMP, st.CP, st.MaxCP = s.HP, s.MaxHP, s.MP, s.MaxMP, s.CP, s.MaxCP
	st.Position = s.Position(now)
	st.RunSpeed, st.Running, st.Load, st.MaxLoad, st.Weight = s.RunSpeed, s.Running, s.Load, s.MaxLoad, 0
	if s.MaxLoad > 0 {
		st.Weight = int32(int64(s.Load) * 100 / int64(s.MaxLoad))
	}
	st.WeightPenalty, st.GradePenalty, st.DeathPenalty = weightBand(s.Load, s.MaxLoad), w.gradePenalty(), s.DeathPenalty
	st.Stash = b.wh.stashed
	st.Stash.Rows = slices.Clone(st.Stash.Rows)
	st.Town = b.nearestTown(st.Position)
	st.ClanID, st.ClanLeader = w.Clan.ID, w.Clan.Leader
	st.Heals = rowHeals(s.ClassID, w.Skills)
	st.Crafter = b.crafterStatus(now)
	st.Build = nil
	if b.persona.Role != RoleNone {
		st.Build = buildStatus(b.build())
	}
	st.PvP = b.pvpStatus()
	st.Leader, st.Members = "", nil
	if p := &w.Party; p.In() {
		if p.LeaderID == s.ObjectID {
			st.Leader = s.Name
		} else if m := p.Members[p.LeaderID]; m != nil {
			st.Leader = m.Name
		}
		for _, m := range p.Members {
			st.Members = append(st.Members, m.Name)
		}
		slices.Sort(st.Members)
	}
	l.board.store(st)
}

// classProphet is the Cleric's buffer profession (data/xml/classes): its
// row keeps the heals the Cleric learned.
const classProphet int32 = 17

// rowHeals reports a skill list holding the heal of classID's row: 1011
// Heal, 1015 Battle Heal or 1027 Group Heal for a human, elven or dark
// mystic, a Cleric, Elven Oracle or Shillien Oracle and the professions
// after them, 1229 Chant of Life for an Orc Shaman or Warcryer. No other
// row heals a party: an orc mystic's 1090 Life Drain heals only itself.
func rowHeals(classID int32, skills map[int32]KnownSkill) bool {
	switch classID {
	case classHumanMystic, classElvenMystic, classDarkMystic,
		classCleric, classBishop, classProphet, classElvenOracle, classElvenElder,
		classShillienOracle, classShillienElder:
		return hasSkill(skills, skillHeal) || hasSkill(skills, skillBattleHeal) || hasSkill(skills, skillGroupHeal)
	case classOrcShaman, classWarcryer:
		return hasSkill(skills, skillChantLife)
	}
	return false
}

// hasSkill reports id in skills.
func hasSkill(skills map[int32]KnownSkill, id int32) bool {
	_, ok := skills[id]
	return ok
}

// nearestTown is the town of the travel map nearest to at, nil without a
// travel map or towns.
func (b *Brain) nearestTown(at Point) *Point {
	if b.cfg.Travel == nil {
		return nil
	}
	if b.towns == nil {
		b.towns = b.cfg.Travel.Towns()
	}
	var best *Point
	for i := range b.towns {
		if best == nil || dist2D(at, b.towns[i]) < dist2D(at, *best) {
			best = &b.towns[i]
		}
	}
	if best == nil {
		return nil
	}
	t := *best
	return &t
}
