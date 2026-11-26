package botclient

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The PvP world model (docs/pvp/wire.md "World model (X4)"): what the bot
// knows of the players it may fight, its duel and its arena. The brain
// goroutine owns all of it, as it owns the World.

// FightKind is the rule that let the bot fight a player
// (docs/pvp/targets.md "MayFight"), or FightUnfought for an attacker the
// rules refuse.
type FightKind uint8

// The fight kinds. fightNone is no fight; fightNPC a monster's or a
// summon's, which no opponent record holds.
const (
	fightNone FightKind = iota
	fightNPC
	FightDuel
	FightBack
	FightArena
	FightFlagged
	FightPK
	FightUnfought
)

// String is the kind's metric label (docs/pvp/modes.md).
func (k FightKind) String() string {
	switch k {
	case FightDuel:
		return "duel"
	case FightBack:
		return "fight_back"
	case FightArena:
		return "arena"
	case FightFlagged:
		return "flagged"
	case FightPK:
		return "pk"
	case FightUnfought:
		return "unfought"
	}
	return ""
}

// CastSeen is a player's last cast: its skill, its target and when it
// lands (the MagicSkillUse's arrival plus its hit time).
type CastSeen struct {
	Skill, Target int32
	Lands         time.Time
}

// Opponent is what the bot knows of one player in sight for PvP: its
// archetype and class tier (0 base class to 3 third profession), clan and
// alliance, the CharInfo abnormal mask, when its flag was last seen turning
// to 2 (zero while not blinking), its vitals as last reported (HP from a
// selection StatusUpdate or the duel window, CP from CharInfo or the duel
// window) less the damage the bot dealt it since (SystemMessage 35), when
// they were reported, its last cast, and the rule that let the bot fight it.
type Opponent struct {
	Archetype charmodel.Archetype
	// Row is its behavior row by class and weapon, the engage estimate's
	// reference opponent (Brain.engages).
	Row            charmodel.Row
	Tier           int
	ClanID, AllyID int32
	Abnormal       int32
	FlagBlinkAt    time.Time
	HP, MaxHP      int32
	CP, MaxCP      int32
	SeenAt         time.Time
	LastCast       CastSeen
	Kind           FightKind
	// RootUntil is when the root the bot landed on it stops holding it
	// (docs/pvp/guards.md "Arena edge"), zero for none.
	RootUntil time.Time
	// flag is the flag state last seen, what FlagBlinkAt and a change of
	// CharInfo are told from.
	flag uint8
}

// DuelPhase is where the bot's duel stands.
type DuelPhase uint8

// The duel phases: asked from a challenge until the countdown (1929 or
// 1930), started from ExDuelStart until the opponent's first
// ExDuelUpdateUserInfo, fighting from then, over from the result or a 692
// until ExDuelEnd.
const (
	DuelIdle DuelPhase = iota
	DuelAsked
	DuelCountdown
	DuelStarted
	DuelFighting
	DuelOver
)

// DuelResult is how the bot's last duel ended.
type DuelResult uint8

// The duel results: DuelNoResult until a 1950 or 1952 decides one.
const (
	DuelNoResult DuelResult = iota
	DuelWon
	DuelLost
	DuelTie
)

// Duel is the bot's one-on-one duel (docs/pvp/wire.md "World model (X4)").
type Duel struct {
	Phase DuelPhase
	// Name is the opponent's name (the challenger's, or the accepted
	// one's), Opponent its object id from its first ExDuelUpdateUserInfo.
	Name     string
	Opponent int32
	// Since is when the phase began; FightFrom is when the bot may attack:
	// the first ExDuelUpdateUserInfo plus the PvP reaction delay. The
	// server sends it from the opponent's own start step, and the bot's
	// own Duelling state comes in its own step, queued by the same loop
	// (network/duel.go startDuel, prepareToDuel).
	Since, FightFrom time.Time
	// Result is the last duel's, kept after it ends.
	Result DuelResult
	// Challenged marks a challenge the bot sent (asked phase): its decline
	// or refusal holds the bot's own challenges of that player
	// (Brain.unasked).
	Challenged bool
}

// Duel timing (docs/pvp/server-duels.md): a challenge stays open
// duelAskOpen; a duel counts down 5 s and lasts at most 2 minutes, so one
// still open past duelLimit from its countdown lost its ExDuelEnd; the end
// messages come with ExDuelEnd in one step (network/duel.go endDuel), so a
// duel over for duelOverLimit lost it too.
const (
	duelAskOpen   = 15 * time.Second
	duelLimit     = 5*time.Second + 2*time.Minute + 30*time.Second
	duelOverLimit = 5 * time.Second
)

// pvpWorld is the brain's PvP world model.
type pvpWorld struct {
	// opponents are the opponent records of the players in sight.
	opponents map[int32]*Opponent
	// owners are the owners' names of the summons and pets in sight (the
	// NPCInfo title, network/visibility.go summon NPCInfo).
	owners map[int32]string
	// radii are the decoded collision radii of the players, summons and
	// NPCs in sight, what the area rule reaches them by.
	radii map[int32]float64
	duel  Duel
	// asked is the duel challenge open for an answer, of either kind
	// (Brain.declineDuel).
	asked duelAsk
	// root, guard and stand are the guards' (pvp_guards.go, pvp_area.go).
	root  rootCast
	guard guardState
	stand poleStand
	// find and arena are the fight finding's (pvp_finding.go,
	// pvp_arena.go); rows is the engage estimate's row table, empty (plain
	// attacks) until X6 sets it.
	find  findState
	arena arenaTrip
	rows  charmodel.RowTable
}

// duelAsk is an open duel challenge: when it came, whether it is a party
// duel's, and when the PvP reaction to it is due (an acceptance waits for
// it, Brain.answerDuel).
type duelAsk struct {
	at, due time.Time
	party   bool
}

// pvpEvent keeps the PvP world model with ev, received at now after the
// World applied it, and reports when the PvP reaction to it is due: zero
// for an event the PvP reaction does not cover, the next frame for a guard
// stop (Brain.guardWake).
func (b *Brain) pvpEvent(ev Event, now time.Time) time.Time {
	at := b.pvpKeep(ev, now)
	if g := b.guardWake(ev, now); !g.IsZero() && (at.IsZero() || g.Before(at)) {
		at = g
	}
	return at
}

// pvpKeep keeps the PvP world model with ev, received at now, and reports
// when the PvP reaction to it is due.
func (b *Brain) pvpKeep(ev Event, now time.Time) time.Time {
	p := &b.pvp
	if p.opponents == nil {
		p.opponents, p.owners, p.radii = make(map[int32]*Opponent), make(map[int32]string), make(map[int32]float64)
	}
	self := b.world.Self.ObjectID
	switch e := ev.(type) {
	case CharInfo:
		p.radii[e.ObjectID] = e.CollisionRadius
		if b.charInfo(e, now) {
			return b.pvpReact(now)
		}
	case NPCInfo:
		p.radii[e.ObjectID] = e.CollisionRadius
		if e.Summon {
			p.owners[e.ObjectID] = e.Title
		} else {
			delete(p.owners, e.ObjectID)
		}
	case RelationChanged:
		if r := p.opponents[e.ObjectID]; r != nil {
			r.blink(uint8(e.PvPFlag), now)
		}
	case StatusUpdate:
		if r := p.opponents[e.ObjectID]; r != nil {
			if e.Set&StatusHP != 0 {
				r.HP, r.SeenAt = e.HP, now
			}
			if e.Set&StatusMaxHP != 0 {
				r.MaxHP = e.MaxHP
			}
		}
	case DeleteObject:
		delete(p.opponents, e.ObjectID)
		delete(p.owners, e.ObjectID)
		delete(p.radii, e.ObjectID)
	case TeleportToLocation:
		if e.ObjectID == self && !e.Fast {
			clear(p.opponents)
			clear(p.owners)
			clear(p.radii)
		}
	case MoveToLocation:
		if e.ObjectID != self && b.fights(e.ObjectID, now) {
			return b.pvpReact(now)
		}
	case MagicSkillUse:
		return b.castSeen(e, now)
	case MagicSkillCanceled:
		if r := p.opponents[e.CasterID]; r != nil {
			r.LastCast = CastSeen{}
		}
		if e.CasterID == self || b.fights(e.CasterID, now) {
			return b.pvpReact(now)
		}
	case SystemMessage:
		return b.duelMessage(e, now)
	case ExDuelAskStart:
		due := b.pvpReact(now)
		p.asked = duelAsk{at: now, due: due, party: e.Party}
		if d := b.duel(now); d.Phase == DuelIdle && !e.Party {
			*d = Duel{Phase: DuelAsked, Name: e.Requester, Since: now, Result: d.Result}
		}
		return due
	case ExDuelReady:
		if d := b.duel(now); d.Phase < DuelCountdown {
			*d = Duel{Phase: DuelCountdown, Name: d.Name, Since: now, Result: d.Result}
		}
		return b.pvpReact(now)
	case ExDuelStart:
		if d := b.duel(now); d.Phase == DuelCountdown {
			d.Phase = DuelStarted
		}
		return b.pvpReact(now)
	case ExDuelUpdateUserInfo:
		return b.duelWindow(e, now)
	case ExDuelEnd:
		p.duel = Duel{Result: p.duel.Result}
		return b.pvpReact(now)
	}
	return time.Time{}
}

// charInfo keeps the opponent record of e's player, and reports whether it
// is a player the bot fights whose abnormal mask or flag changed.
func (b *Brain) charInfo(e CharInfo, now time.Time) bool {
	if e.ObjectID == b.world.Self.ObjectID {
		return false
	}
	r := b.pvp.opponents[e.ObjectID]
	seen := r != nil
	if !seen {
		r = &Opponent{}
		b.pvp.opponents[e.ObjectID] = r
	}
	changed := seen && (r.Abnormal != e.Abnormal || r.flag != uint8(e.PvPFlag))
	_, bow := b.cfg.Travel.arrowsOf(e.RHand)
	if row, ok := charmodel.ClassRow(e.ClassID, bow); ok {
		r.Archetype, r.Row = row.Archetype(), row
	}
	r.Tier = classTier(e.ClassID)
	b.rootSeen(e.ObjectID, r, e.Abnormal, now)
	r.ClanID, r.AllyID, r.Abnormal = e.ClanID, e.AllyID, e.Abnormal
	r.CP, r.MaxCP, r.SeenAt = e.CP, e.MaxCP, now
	r.blink(uint8(e.PvPFlag), now)
	return changed && b.fights(e.ObjectID, now)
}

// blink keeps flag as the record's flag state: one turning to 2 (blinking,
// the flag about to end) sets FlagBlinkAt, any other state clears it.
func (r *Opponent) blink(flag uint8, now time.Time) {
	switch {
	case flag != 2:
		r.FlagBlinkAt = time.Time{}
	case r.flag != 2:
		r.FlagBlinkAt = now
	}
	r.flag = flag
}

// classTier is class id's profession tier: 0 for a base class, 1 to 3 for
// the first to third profession, the depth of its parent chain (the
// profession table, datapack.ClassParent). An id the table does not hold
// is 0.
func classTier(id int32) int {
	tier := 0
	for {
		parent, ok := datapack.ClassParent(id)
		if !ok || parent < 0 {
			return tier
		}
		tier++
		id = parent
	}
}

// castSeen records e as its caster's last cast, counts an offensive cast
// of a player or summon at the bot as a hit (the server lets it reach the
// bot only from one that may attack it, as every Attack), and reports when
// the PvP reaction to it is due: a cast by or at a player the bot fights,
// or by a player at a party member.
func (b *Brain) castSeen(e MagicSkillUse, now time.Time) time.Time {
	self := b.world.Self.ObjectID
	if e.CasterID == self {
		b.ownCast(e, now)
		return time.Time{}
	}
	if r := b.pvp.opponents[e.CasterID]; r != nil {
		r.LastCast = CastSeen{Skill: e.SkillID, Target: e.TargetID, Lands: now.Add(e.HitTime)}
	}
	o := b.world.object(e.CasterID)
	if o == nil {
		return time.Time{}
	}
	player := o.Kind == KindPlayer
	if e.TargetID == self && (player || o.Summon) && b.cfg.Travel.offensive(e.SkillID, e.Level) {
		b.world.hitBy(e.CasterID, 0, now)
	}
	_, member := b.world.Party.Members[e.TargetID]
	if b.fights(e.CasterID, now) || b.fights(e.TargetID, now) || player && member {
		return b.pvpReact(now)
	}
	return time.Time{}
}

// duelMessage moves the duel along a system message: the countdown from an
// acceptance, the result, the end of a challenge refused, and the damage
// the bot dealt its target (35).
func (b *Brain) duelMessage(e SystemMessage, now time.Time) time.Time {
	d := b.duel(now)
	switch e.ID {
	case msgYouDidS1Dmg:
		b.dealt(e.Number)
		return time.Time{}
	case msgResisted:
		b.pvp.root.resisted = true
		return time.Time{}
	case msgS1AcceptedYourDuel, msgYouAcceptedS1Duel:
		*d = Duel{Phase: DuelCountdown, Name: e.Text[0], Since: now}
	case msgS1WonTheDuel:
		d.Result = DuelLost
		if e.Text[0] == b.world.Self.Name {
			d.Result = DuelWon
		}
		d.over(now)
	case msgDuelEndedInTie:
		d.Result = DuelTie
		d.over(now)
	case msgS1WithdrewS2Won, msgOtherPartyIsFrozen:
		d.over(now)
	case msgEnteredCombatZone, msgLeftCombatZone:
		b.siegeSeen(e.ID == msgEnteredCombatZone, now)
		return time.Time{}
	case msgUnableToRequestDuel:
		b.unableSeen(now)
		return b.unasked(d, now, false)
	case msgS1DeclinedYourDuel, msgS1IsBusyTryLater, msgNoOpponentForDuel:
		return b.unasked(d, now, true)
	default:
		if e.ID >= msgS1CannotDuelPrivateStore && e.ID <= msgS1CannotReceiveDuelTooFar {
			return b.unasked(d, now, true)
		}
		return time.Time{}
	}
	return b.pvpReact(now)
}

// over ends the fighting of a duel that is on: nobody is fought until its
// ExDuelEnd.
func (d *Duel) over(now time.Time) {
	if d.Phase >= DuelCountdown {
		d.Phase, d.Since = DuelOver, now
	}
}

// unasked drops a challenge refused or declined at now, and reports when
// the PvP reaction to it is due: zero with no challenge open, as 153 also
// answers other requests. A decline or refusal (held) of the bot's own
// challenge holds its own challenges of that player declineHold.
func (b *Brain) unasked(d *Duel, now time.Time, held bool) time.Time {
	if d.Phase != DuelAsked {
		return time.Time{}
	}
	if held && d.Challenged {
		b.pvp.find.hold(d.Name, now)
	}
	*d = Duel{Result: d.Result}
	return b.pvpReact(now)
}

// arrowsOf is the arrow template bow weapon shoots; ok is false for a
// weapon that is no bow, or with no travel map.
func (m *TravelMap) arrowsOf(weapon int32) (arrow int32, ok bool) {
	if m == nil {
		return 0, false
	}
	arrow, ok = m.arrows[weapon]
	return arrow, ok
}

// offensive reports whether skill at level is offensive (the datapack's
// offensive, defaulted from its type and isDebuff); false for a skill the
// travel map does not hold, or with no travel map: a cast the bot cannot
// tell is no attack.
func (m *TravelMap) offensive(skill, level int32) bool {
	return m != nil && m.skills[datapack.SkillRef{ID: skill, Level: level}].Offensive
}

// duelWindow keeps the duel opponent's vitals from its ExDuelUpdateUserInfo;
// the first one of a started duel names the opponent and starts the fight
// at the PvP reaction delay.
func (b *Brain) duelWindow(e ExDuelUpdateUserInfo, now time.Time) time.Time {
	if r := b.pvp.opponents[e.ObjectID]; r != nil {
		r.HP, r.MaxHP, r.CP, r.MaxCP, r.SeenAt = e.HP, e.MaxHP, e.CP, e.MaxCP, now
	}
	d := b.duel(now)
	// The server also sends the window in the countdown, on every CP or HP
	// gauge change (network/duel.go sendDuelVitals): only a started duel
	// fights, and ExDuelStart always comes before its first window.
	if d.Phase != DuelStarted || e.ObjectID == b.world.Self.ObjectID {
		return time.Time{}
	}
	d.Phase, d.Name, d.Opponent = DuelFighting, e.Name, e.ObjectID
	d.FightFrom = b.pvpReact(now)
	return d.FightFrom
}

// dealt takes damage the bot dealt off its target's record: CP first, then
// HP, as the server takes a playable's damage (character_stats.go).
func (b *Brain) dealt(damage int32) {
	r := b.pvp.opponents[b.world.Target]
	if r == nil || damage <= 0 {
		return
	}
	cp := min(r.CP, damage)
	r.CP -= cp
	r.HP = max(r.HP-(damage-cp), 0)
}

// duel is the bot's duel at now, dropped once stale: a challenge unanswered
// for duelAskOpen, a duel past duelLimit or over for duelOverLimit.
func (b *Brain) duel(now time.Time) *Duel {
	d := &b.pvp.duel
	age := now.Sub(d.Since)
	if d.Phase == DuelAsked && age > duelAskOpen || d.Phase == DuelOver && age > duelOverLimit ||
		d.Phase >= DuelCountdown && age > duelLimit {
		*d = Duel{Result: d.Result}
	}
	return d
}

// inDuel reports whether the bot is in a duel at now, from the countdown to
// its end.
func (b *Brain) inDuel(now time.Time) bool { return b.duel(now).Phase >= DuelCountdown }

// duelFoe is the duel opponent the bot may attack at now, 0 outside the
// fight: only while fighting, from FightFrom, is the opponent attackable
// without force (duel/state.go SameActive).
func (b *Brain) duelFoe(now time.Time) int32 {
	if d := b.duel(now); d.Phase == DuelFighting && !now.Before(d.FightFrom) {
		return d.Opponent
	}
	return 0
}

// duelOpponent is the object id of the bot's duel opponent at now, by its
// duel window or else by its name; 0 outside a duel or out of sight.
func (b *Brain) duelOpponent(now time.Time) int32 {
	d := b.duel(now)
	if d.Phase < DuelCountdown {
		return 0
	}
	if d.Opponent != 0 {
		return d.Opponent
	}
	return b.playerNamed(d.Name)
}

// playerNamed is the object id of the player in sight called name, 0 for
// none.
func (b *Brain) playerNamed(name string) int32 {
	if name == "" {
		return 0
	}
	for id, o := range b.world.Objects {
		if o.Kind == KindPlayer && o.Name == name {
			return id
		}
	}
	return 0
}

// pvpReact is when a reaction to a PvP event at now is due: PvPReactMin to
// PvPReactMax later (D35).
func (b *Brain) pvpReact(now time.Time) time.Time {
	return now.Add(b.cfg.PvPReactMin + time.Duration(b.rng.Int64N(int64(b.cfg.PvPReactMax-b.cfg.PvPReactMin)+1)))
}

// fights reports whether id is a player the bot fights at now: its duel
// opponent, one hitting it, its target while it may fight it, or another
// player in its arena.
func (b *Brain) fights(id int32, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer {
		return false
	}
	return id == b.duelOpponent(now) || b.world.attacking(id, now) ||
		id == b.world.Target && b.mayFight(id, now) || b.sameArena(id, now)
}

// fightKind is the rule that lets the bot fight o (object id) at now,
// fightNone when none does (docs/pvp/targets.md "MayFight"). A player's
// record keeps the kind, or FightUnfought for an attacker refused.
func (b *Brain) fightKind(id int32, o *Object, now time.Time) FightKind {
	k := b.fightRule(id, o, now)
	if r := b.pvp.opponents[id]; r != nil && o.Kind == KindPlayer {
		switch {
		case k != fightNone:
			r.Kind = k
		case b.world.attacking(id, now):
			r.Kind = FightUnfought
		}
	}
	return k
}

// fightRule applies MayFight's rules to o at now. Rule 7 (PK mode) is
// X11's: until then ForceAttack lets nobody pass.
func (b *Brain) fightRule(id int32, o *Object, now time.Time) FightKind {
	// Rule 1: from the countdown to the end only the duel opponent, and
	// only while fighting, even a clan mate or a party member.
	if b.inDuel(now) {
		if o.Kind == KindPlayer && id == b.duelFoe(now) {
			return FightDuel
		}
		return fightNone
	}
	if o.Kind == KindNPC {
		if o.Summon {
			if b.maySummon(id, o, now) {
				return fightNPC
			}
			return fightNone
		}
		if o.Attackable {
			return fightNPC
		}
		return fightNone
	}
	// Rule 2: never a party member, in an arena too.
	if o.Kind != KindPlayer || b.partyMate(id) {
		return fightNone
	}
	// Rule 3: in the bot's arena anyone else, clan mates, allies and bots
	// included (D38; character_social.go SocialWithoutForce). A command
	// channel mate is not told apart: the bot does not track channels.
	if b.sameArena(id, now) {
		return FightArena
	}
	// Rule 4.
	if b.clanMate(id) {
		return fightNone
	}
	attacking := b.world.attacking(id, now)
	// Rule 5: a bot only once it hit the bot first (D38).
	if b.cfg.markers.Marks(o.Name) && !attacking {
		return fightNone
	}
	// Rule 6.
	if b.persona.PvP.FightFlagged && flagged(o) {
		if attacking {
			return FightBack
		}
		return FightFlagged
	}
	return fightNone
}

// friend reports whether player id is the bot's party member (rule 2), or
// its clan mate or ally (rule 4).
func (b *Brain) friend(id int32) bool { return b.partyMate(id) || b.clanMate(id) }

// partyMate reports whether player id is the bot's party member.
func (b *Brain) partyMate(id int32) bool {
	_, mate := b.world.Party.Members[id]
	return mate
}

// clanMate reports whether player id is the bot's clan mate or ally by its
// record: an equal non-zero clan or alliance id.
func (b *Brain) clanMate(id int32) bool {
	r := b.pvp.opponents[id]
	return r != nil && (r.ClanID != 0 && r.ClanID == b.world.Clan.ID || r.AllyID != 0 && r.AllyID == b.world.Self.AllyID)
}

// maySummon reports whether the bot may fight summon id itself at now
// (docs/pvp/targets.md "Summons and pets"): only with its owner out of sight
// or refused, never a party member's, clan mate's or ally's, and only while
// the summon's own flag or karma (its owner's, refreshed by RelationChanged)
// lets the bot attack it without force, whatever its NPCInfo's attackable
// flag said, or in its arena while its owner is seen inside it by
// arenaEdge (the server credits the summon's death by its owner's zone). A
// flag of 2 is the flag guard's.
func (b *Brain) maySummon(id int32, o *Object, now time.Time) bool {
	name := b.pvp.owners[id]
	for _, m := range b.world.Party.Members {
		if name != "" && m.Name == name {
			return false
		}
	}
	if owner := b.playerNamed(name); owner != 0 {
		if b.friend(owner) {
			return false
		}
		if oo := b.world.object(owner); !oo.Dead && b.fightRule(owner, oo, now) != fightNone {
			return false
		}
	}
	if own := b.arenaAt(b.world.Self.Position(now)); own.Arena != 0 {
		oo := b.world.object(b.playerNamed(name))
		if oo == nil {
			return false
		}
		at := b.arenaAt(oo.Position(now))
		return at.Arena == own.Arena && at.Edge > arenaEdge
	}
	return b.persona.PvP.FightFlagged && (o.Karma > 0 || o.PvPFlag != 0)
}

// summonOwner is the object id of summon id's owner when in sight and
// fightable at now: the one fought in the summon's stead.
func (b *Brain) summonOwner(id int32, now time.Time) int32 {
	o := b.world.object(id)
	if o == nil || !o.Summon {
		return 0
	}
	owner := b.playerNamed(b.pvp.owners[id])
	if owner == 0 || !b.mayFight(owner, now) {
		return 0
	}
	return owner
}

// supportAllowed reports whether the bot may heal or buff player o (object
// id) at now (docs/pvp/guards.md "Duel support"): in its duel never the
// opponent and never a flagged or karma player, SupportFlagged counting as
// off (a flag cancels the duel); in a PvP zone everyone, as nothing flags it
// there (character_pvpflag.go skillTargetFlagsPvP); else the policy's
// MaySupport.
func (b *Brain) supportAllowed(id int32, o *Object, now time.Time) bool {
	if b.inDuel(now) {
		return id != b.duelOpponent(now) && clean(o)
	}
	if b.inArena(now) {
		return true
	}
	return b.persona.PvP.MaySupport(o)
}

// clean reports whether o has neither a PvP flag nor karma.
func clean(o *Object) bool { return o.PvPFlag == 0 && o.Karma == 0 }

// partySkillHeld reports whether a party skill (a Party buff, a party heal)
// must wait at now: a party member it would land on, within partyBuffReach,
// is one the bot may not support (a flagged or karma one with the support
// gate off, its duel opponent).
func (b *Brain) partySkillHeld(now time.Time) bool {
	from := b.world.Self.Position(now)
	for id := range b.world.Party.Members {
		o := b.world.object(id)
		if o == nil || o.Dead || dist2(from, o.Position(now)) > partyBuffReach*partyBuffReach {
			continue
		}
		if !b.supportAllowed(id, o, now) {
			return true
		}
	}
	return false
}

// arenaPos is where a point stands in the arenas: the 1-based index of the
// arena polygon holding it (0 for none) and its distance to that arena's
// edge, what the guards' arena-edge rule reads (X4b).
type arenaPos struct {
	Arena int
	Edge  float64
}

// arenaAt is where p stands in the travel map's arenas.
func (b *Brain) arenaAt(p Point) arenaPos {
	if b.cfg.Travel == nil {
		return arenaPos{}
	}
	for i, z := range b.cfg.Travel.zones.Arenas {
		if z.Contains(p.X, p.Y, p.Z) {
			return arenaPos{Arena: i + 1, Edge: zoneEdge(z, p)}
		}
	}
	return arenaPos{}
}

// inArena reports whether the bot stands in an arena at now.
func (b *Brain) inArena(now time.Time) bool {
	return b.arenaAt(b.world.Self.Position(now)).Arena != 0
}

// sameArena reports whether player id stands in the arena the bot stands in
// at now: attackable without force there (character_social.go
// SocialWithoutForce).
func (b *Brain) sameArena(id int32, now time.Time) bool {
	o := b.world.object(id)
	if o == nil || o.Kind != KindPlayer {
		return false
	}
	a := b.arenaAt(b.world.Self.Position(now)).Arena
	return a != 0 && b.arenaAt(o.Position(now)).Arena == a
}

// zoneEdge is the distance from p, inside z, to z's edge in the plane.
func zoneEdge(z datapack.Zone, p Point) float64 {
	x, y := float64(p.X), float64(p.Y)
	if z.Cuboid {
		return min(x-float64(z.X[0]), float64(z.X[1])-x, y-float64(z.Y[0]), float64(z.Y[1])-y)
	}
	edge := math.Inf(1)
	for i := range z.X {
		j := (i + 1) % len(z.X)
		edge = min(edge, segmentDist(x, y, float64(z.X[i]), float64(z.Y[i]), float64(z.X[j]), float64(z.Y[j])))
	}
	return edge
}

// segmentDist is the distance from (x, y) to the segment (ax, ay)-(bx, by).
func segmentDist(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = max(0, min(1, ((x-ax)*dx+(y-ay)*dy)/l))
	}
	return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}

// ruleDuel is MayFight's rule 1 in the decision table: from the countdown
// to the end of its duel the bot attacks its opponent while fighting, with
// plain Action and casts (fight, forced), and else holds, every other rule
// below it waiting: a hit on any other creature, or a walk away, cancels
// the duel (docs/pvp/server-duels.md). The critical, heal and raise rules
// still go first.
func (b *Brain) ruleDuel(now time.Time) (Action, bool) {
	if a, ok := b.declineDuel(now); ok {
		return a, true
	}
	if a, ok := b.answerDuel(now); ok {
		return a, true
	}
	if !b.inDuel(now) {
		return Action{}, false
	}
	id := b.duelFoe(now)
	switch {
	case id == 0 || !b.mayFight(id, now):
		return Action{}, true
	case b.world.Self.Sitting:
		return b.toggle(now), true
	}
	return b.fight(id, now), true
}

// declineDuel declines the open duel challenge, of either kind, at once (at
// the PvP reaction to ExDuelAskStart) when the persona takes no duels
// (PvPPolicy.Duels, docs/pvp/modes.md): one RequestDuelAnswerStart per
// challenge, bounded by the per-connection frame limits
// (network/floodprotector.go), as the server takes the first answer. A
// challenge left open lapses after duelAskOpen.
func (b *Brain) declineDuel(now time.Time) (Action, bool) {
	a := b.pvp.asked
	if b.persona.PvP.Duels || a.at.IsZero() || now.Sub(a.at) > duelAskOpen {
		return Action{}, false
	}
	return Action{kind: actDuelAnswer, payload: encodeRequestDuelAnswerStart(a.party, false)}, true
}
