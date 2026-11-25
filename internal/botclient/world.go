package botclient

import (
	"fmt"
	"math"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// snapDrift is how far, in world units, a position the server reports for
// a moving object may stray from the dead-reckoned one before the model
// snaps to it. Closer reports keep the estimate; the bot's own
// ValidateLocation is a correction and always adopted (SelfState.correct).
const snapDrift = 200

// World is one bot's model of the game world, built from the events its
// session's reader decodes. It is owned by the bot's brain goroutine: no
// other goroutine reads or writes it, so it has no lock. Positions are dead
// reckoned from the last movement report, so readers pass the current time.
type World struct {
	Self SelfState
	// gen counts the events applied but the moves and the status and looks
	// of other objects, which change nothing of the bot's own: a cache of
	// values derived from the bot's character that checked its inputs at
	// one count needs no check again until the next (Brain.valuer).
	gen uint64
	// Objects are the objects in sight by object id, the bot's own
	// character excluded.
	Objects map[int32]*Object
	// Target is the object id the server confirmed as the bot's target, 0
	// for none.
	Target  int32
	Prompts Prompts
	// Skills are the character's skills by id (SkillList); skillReady is
	// when each skill's reuse ends, kept apart because SkillCoolTime and
	// the bot's own casts set it whether or not the list came first.
	Skills     map[int32]KnownSkill
	skillReady map[int32]time.Time
	// Attackers are the creatures that hit the bot recently, by object id.
	Attackers map[int32]*Attacker
	Party     Party
	// Clan is the bot's clan; clanInviter the inviter the last clan
	// invitation's system message named (applyClan).
	Clan        Clan
	clanInviter clanInviter
	// Dialog is the last dialog page the server sent.
	Dialog Dialog
	// Inventory is what the bot carries.
	Inventory Inventory
	// Trade is the bot's open trade window.
	Trade Trade
	// Buffs are the bot's own effects (AbnormalStatusUpdate), and
	// BuffLevels the skill level of each, by skill id.
	Buffs      Buffs
	BuffLevels map[int32]int32
	// Ground are the items on the ground in sight by object id.
	Ground map[int32]GroundItem
	// Recipes is the bot's recipe book.
	Recipes RecipeBook
	// Symbols are the bot's dye symbols (HennaInfo).
	Symbols Symbols
}

// Symbols are the bot's dye symbols as the last HennaInfo told them: the
// slots its class has and the symbols worn, in slot order. Known is set
// once a HennaInfo came (at login, a class change, a draw or a removal).
type Symbols struct {
	Known    bool
	MaxSlots int32
	Worn     []WornSymbol
}

// SelfState is the bot's own character.
type SelfState struct {
	ObjectID int32
	Name     string
	// BaseClass is the class drawn on the model, ClassID the class played;
	// Race is the character's (RaceHuman to RaceDwarf).
	BaseClass, ClassID int32
	Race               int32
	Level              int32
	Exp                int64
	SP                 int32
	HP, MaxHP          int32
	MP, MaxMP          int32
	CP, MaxCP          int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad int32
	// InvLimit is the inventory limit (UserInfo.InvLimit), 0 while
	// unknown: the server refuses a pickup needing a slot past it
	// (Brain.slotsFit).
	InvLimit int32
	// DeathPenalty is the death penalty level (0-15); deathPenaltyKnown
	// is set once the session's first EtcStatusUpdate gave it
	// (World.applyDeathPenalty).
	DeathPenalty      int32
	deathPenaltyKnown bool
	// Charges are the force charges of the latest EtcStatusUpdate: the
	// server sends the bot's own on every change (event.ChargesChanged,
	// network/live_player_events.go), and another player's only when its
	// Charm of Courage starts or ends, which the bot's next change of its
	// own corrects, or the refusal of a cast it gated on the wrong count
	// (Brain.chargesRefused).
	Charges int32
	PvPFlag uint8
	Karma   int32
	// AllyID is its clan's alliance (UserInfo), 0 for none.
	AllyID int32
	// StoreType is the private store mode, 0 for none.
	StoreType                        uint8
	Sitting, InCombat, Dead, Running bool
	RunSpeed, WalkSpeed, Heading     int32
	// SwimSpeed is UserInfo's: the bot's speed in water.
	SwimSpeed int32
	move      motion
	// answerLag is the walker's answer latency over its recent segments
	// (walker.answered); see lag.
	answerLag time.Duration
	// water is the water zones the bot's own moves swim in; nil is none.
	water *nav.Water
	// castEnd is when the bot's own cast in progress hits, swingEnd when
	// its own melee swing in progress ends.
	castEnd, swingEnd time.Time
	// PAtkSpd, CollisionHeight, the combat stats and the base stats are
	// UserInfo's; every UserInfo replaces them.
	PAtkSpd                       int32
	CollisionHeight               float64
	PAtk, PDef, Evasion, Accuracy int32
	Critical, MAtk, CastSpd, MDef int32
	STR, DEX, CON, INT, WIT, MEN  int32
	// ground, when set, is the geodata the bot's own walk is simulated on
	// (World.SetGround).
	ground groundHeight
}

// Position is where the character is at now, dead reckoned along its
// current move. With geodata (World.SetGround) the height on the way is the
// floor under the dead-reckoned point, as the server's step chain lands each
// step on the layer nearest the walker, instead of a straight line between
// the move's two heights; the end of a move keeps the server's height.
func (s *SelfState) Position(now time.Time) Point {
	p := s.move.at(now)
	if s.ground != nil && p != s.move.to && !s.move.swimming(p) {
		p.Z = int32(s.ground.Height(int(p.X), int(p.Y), int(p.Z)))
	}
	return p
}

// groundHeight is the geodata height lookup the bot's walk is simulated on;
// *engine.Engine is the real one.
type groundHeight interface {
	Height(worldX, worldY, worldZ int) int16
}

// SetGround simulates the bot's own walk on g (see SelfState.Position).
func (w *World) SetGround(g groundHeight) { w.Self.ground = g }

// SetWater simulates the bot's own moves as swims where they pass water
// (see motion).
func (w *World) SetWater(water *nav.Water) { w.Self.water = water }

// swimmer is the bot's own swim: its water and swim speed, the zero
// swimLine when it has no water or no swim speed.
func (s *SelfState) swimmer() swimLine {
	if s.water == nil || s.SwimSpeed <= 0 {
		return swimLine{}
	}
	return swimLine{water: s.water, speed: s.SwimSpeed}
}

// lag is A, the walker's answer latency floored at positionTick: the bot's
// own move received within positionTick plus A of its modeled end counts as
// a fresh start (docs/walking.md, "One arrival model").
func (s *SelfState) lag() time.Duration { return max(s.answerLag, positionTick) }

// arrival is the modeled end of the bot's own move (docs/walking.md, "One
// arrival model"): when it stopped for a stop, zero before its first move.
func (s *SelfState) arrival() time.Time { return s.move.end() }

// moved is the bot's own motion once the server's answer moving it from
// origin toward dest is received at now, timed like the server's arrival
// timer. An answer whose origin is its destination stands the bot there at
// now: the server's blocked correction, or a click on the cell it stands in.
// A move received while the last one is more than positionTick plus A from
// its modeled end retargets it and keeps its anchor, the server's start-speed
// update count running (creature.go moveToLocationLocked); any other starts
// afresh, which errs late. The distance timed is the larger of origin to
// dest and the sampled position to dest.
func (s *SelfState) moved(now time.Time, origin, dest Point) motion {
	old := s.move
	if origin.X == dest.X && origin.Y == dest.Y {
		m := stopAt(dest)
		m.start = now
		return m
	}
	anchor := now
	if old.moving(now) && now.Before(old.end().Add(-(positionTick + s.lag()))) {
		anchor = old.anchor()
	}
	m := old
	m.self = &selfMove{swim: s.swimmer()}
	m = m.withLine(old.sample(now, origin), dest, now)
	m.speed = pace(s.Running, s.RunSpeed, s.WalkSpeed)
	return m.timed(anchor, s.WalkSpeed, scale(origin, m.from, dest), time.Time{})
}

// resample applies the server's position report p at now to the bot's own
// move: a move carries on toward its destination from the sampled position
// at the speeds the bot has now, re-timed from now for the distance left with
// the updates done since its anchor (creature.go retimeLocked), and ending no
// earlier than before unless its speed changed; a move over stands at the
// sampled position. A move the bot's speed fell to 0 in stalls at p: the
// server's immobile weight band (refreshWeightPenalty) holds the character
// where it reports it, sends no StopMove and keeps the leg in flight with
// no arrival until the speed returns, then walks on to the old destination
// (creature.go SetSpeed, retimeLocked; #488).
func (s *SelfState) resample(now time.Time, p Point) {
	s.continueFrom(now, s.move.sample(now, p), p)
}

// correct applies the server's ValidateLocation p at now to the bot's own
// move: resample continuing from p itself. The server sends it only when the
// bot's reported position strays more than the move speed from its own and
// never adopts the report (network/movement.go validateLivePlayerPosition),
// so p is where the server holds the character, however close to the
// estimate (#464).
func (s *SelfState) correct(now time.Time, p Point) {
	s.continueFrom(now, p, p)
}

// continueFrom is resample with from, the position to continue from, chosen
// by the caller for the server's report p.
func (s *SelfState) continueFrom(now time.Time, from, p Point) {
	old := s.move
	if !old.moving(now) {
		m := stopAt(from)
		m.start = old.end()
		s.move = m
		return
	}
	speed := pace(s.Running, s.RunSpeed, s.WalkSpeed)
	if speed <= 0 {
		from = p
	}
	m := old.withLine(from, old.to, now)
	m.speed = speed
	var keep time.Time
	if m.speed == old.speed {
		keep = old.end()
	}
	s.move = m.timed(old.anchor(), s.WalkSpeed, scale(p, from, old.to), keep)
}

// scale is how much longer the server's line from origin to dest is than the
// bot's from from: at least 1, so the larger distance is timed.
func scale(origin, from, dest Point) float64 {
	d := math.Sqrt(float64(horizontal2(from, dest)))
	if d == 0 {
		return 1
	}
	return max(math.Sqrt(float64(horizontal2(origin, dest)))/d, 1)
}

// Swinging reports whether the bot's own melee swing is in progress at now:
// the server defers a move asked during it (network/movement.go
// tryLiveMoveTo) and answers ActionFailed.
func (s *SelfState) Swinging(now time.Time) bool { return now.Before(s.swingEnd) }

// Moving reports whether the character is still on its way at now, a
// move stalled at speed 0 included (Stalled).
func (s *SelfState) Moving(now time.Time) bool { return s.move.moving(now) }

// Stalled reports whether the character's move is held where it stands at
// speed 0, as the server's immobile weight band holds it (resample).
func (s *SelfState) Stalled() bool { return s.move.stalled() }

// ObjectKind tells NPCs from players.
type ObjectKind uint8

// The kinds of objects in sight.
const (
	KindNPC ObjectKind = iota + 1
	KindPlayer
)

// Object is one object in sight. Records stay compact: a name only for
// players, narrow fields where the wire values are small.
type Object struct {
	Kind ObjectKind
	// NPCID is the NPC template id (NPCs only).
	NPCID int32
	// Name and ClassID describe players only.
	Name    string
	ClassID int32
	Karma   int32
	// HP and MaxHP are 0 until a StatusUpdate reports them.
	HP, MaxHP           int32
	RunSpeed, WalkSpeed int32
	Heading             int32
	PvPFlag             uint8
	StoreType           uint8
	// Attackable is, for an NPC, NPCInfo's attackable flag; for a player,
	// the auto-attackable flag of its last RelationChanged (whether the bot
	// may attack it without force), kept across CharInfo updates.
	Attackable, Summon bool
	Sitting, Running   bool
	InCombat, Dead     bool
	move               motion
}

// Position is where the object is at now, dead reckoned along its current
// move.
func (o *Object) Position(now time.Time) Point { return o.move.at(now) }

// Moving reports whether the object is still on its way at now.
func (o *Object) Moving(now time.Time) bool { return o.move.moving(now) }

// Prompts are the server's open questions to the bot.
type Prompts struct {
	// Restart is open while the bot's character is dead.
	Restart RestartPrompt
	// Party is open from a party invitation until the bot answers it or
	// joins a party.
	Party PartyInvite
	// Clan is open from a clan invitation until the bot answers it or
	// joins a clan.
	Clan ClanInvite
	// Trade is open from a trade request until the bot answers it or a
	// trade window opens.
	Trade TradeInvite
	// Resurrect is open from a resurrection offered to the dead bot until
	// it answers the offer or revives.
	Resurrect ResurrectOffer
}

// RestartPrompt is the restart-point choice offered to a dead character
// since At.
type RestartPrompt struct {
	Open   bool
	Points RestartPoints
	At     time.Time
}

// motion is a straight move from from to to at speed world units per
// second, begun at start. A stopped object has from == to. A motion with a
// swim swims the part of its line in water: there the server moves a player
// at its swim speed and measures the distance in 3D (acis_golang
// Character.MoveSpeed, model/actor/move creature.go span).
type motion struct {
	from, to Point
	start    time.Time
	speed    int32
	// self is the bot's own move's swim and arrival model, nil for every
	// other object: a pointer keeps Object compact
	// (TestObjectRecordCompact). A selfMove is never changed once a motion
	// holds it, so copies of the motion may share it.
	self *selfMove
}

// walkStartUpdates is how many position updates of a player's move from
// standstill advance at its start speed (acis_golang model/actor/move
// creature.go walkStartUpdates).
const walkStartUpdates = 5

// selfMove is the arrival model of the bot's own move, timed like the
// server's arrival timer (creature.go travelTicksLocked, arrivalDelayLocked,
// retimeLocked; docs/walking.md, "One arrival model"), and its swim, the zero
// swimLine for none. anchor is the receipt
// of the answer that started the move from standstill, from which the
// server's position updates count; walk is the start speed the first
// walkStartUpdates updates advance at on land, below the move's speed or
// none (the server's start speed is the walk speed on land, even running,
// and the full speed in water: character_attack.go moveStartSpeed); end is
// the modeled arrival.
type selfMove struct {
	swim   swimLine
	anchor time.Time
	walk   int32
	end    time.Time
}

// timed is m, the bot's own move re-timed at its start: anchored at anchor
// with start speed walk, its line's length scaled by k timed from the
// position updates done since the anchor, and ending no earlier than keep.
// A move at zero speed ends at its start.
func (m motion) timed(anchor time.Time, walk int32, k float64, keep time.Time) motion {
	if walk >= m.speed {
		walk = 0
	}
	sm := &selfMove{anchor: anchor, walk: walk, end: m.start}
	if m.self != nil {
		sm.swim = m.self.swim
	}
	m.self = sm
	if m.speed > 0 {
		u := max(int(m.start.Sub(anchor)/positionTick), 0)
		sm.end = m.start.Add(time.Duration(m.arrivalTicks(u, k)) * positionTick)
	}
	if sm.end.Before(keep) {
		sm.end = keep
	}
	return m
}

// arrivalTicks is how many position updates the server takes to cover the
// motion's line, its length scaled by k, from u updates done: on land the
// start-speed updates left first, each stretch at its speed, rounded up to
// whole updates (creature.go travelTicksLocked), plus one update per change
// between land and water along the line, where the server re-times the rest
// (creature.go update, retimeLocked). At least one: a same-cell move ends on
// the next update.
func (m motion) arrivalTicks(u int, k float64) int {
	dx, dy, dz := float64(m.to.X-m.from.X), float64(m.to.Y-m.from.Y), float64(m.to.Z-m.from.Z)
	flat := math.Hypot(dx, dy) * k
	step := float64(m.speed) / 10
	type stretch struct {
		dist, step float64
		land       bool
	}
	line := []stretch{{flat, step, true}}
	turns := 0
	if m.wet() {
		s := m.swim()
		space := math.Sqrt(dx*dx+dy*dy+dz*dz) * k
		line = []stretch{
			{flat * s.wet0, step, true},
			{space * (s.wet1 - s.wet0), float64(s.speed) / 10, false},
			{flat * (1 - s.wet1), step, true},
		}
		if s.wet0 > 0 {
			turns++
		}
		if s.wet1 < 1 {
			turns++
		}
	}
	var walk float64
	if m.self != nil {
		walk = float64(m.self.walk) / 10
	}
	t := float64(u)
	for _, st := range line {
		d := st.dist
		if d <= 0 {
			continue
		}
		if st.land && walk > 0 && t < walkStartUpdates {
			left := walkStartUpdates - t
			if d <= left*walk {
				t += d / walk
				continue
			}
			t += left
			d -= left * walk
		}
		t += d / st.step
	}
	return max(int(math.Ceil(t-float64(u)-1e-9)), 1) + turns
}

// end is when the motion arrives: the bot's own move's modeled arrival,
// else its start plus its duration.
func (m motion) end() time.Time {
	if m.self != nil {
		return m.self.end
	}
	return m.start.Add(m.duration())
}

// anchor is where the server's position updates of the motion count from:
// the bot's own move's anchor, else its start.
func (m motion) anchor() time.Time {
	if m.self != nil {
		return m.self.anchor
	}
	return m.start
}

// swimLine is what a motion swims by: the water zones and the swim speed,
// and the part of the line in water, wet0 to wet1 as fractions of it.
type swimLine struct {
	water      *nav.Water
	speed      int32
	wet0, wet1 float64
}

// withLine is m moving from from to to, begun at start, at m's speeds.
func (m motion) withLine(from, to Point, start time.Time) motion {
	m.from, m.to, m.start = from, to, start
	if m.self != nil {
		sm := *m.self
		sm.swim.wet0, sm.swim.wet1 = 0, 0
		if s := &sm.swim; s.water != nil && s.speed > 0 && from != to {
			if t0, t1, ok := s.water.Span(toLoc(from), toLoc(to)); ok {
				s.wet0, s.wet1 = t0, t1
			}
		}
		m.self = &sm
	}
	return m
}

// swim is what the motion swims by, nil for none.
func (m motion) swim() *swimLine {
	if m.self == nil || m.self.swim.water == nil {
		return nil
	}
	return &m.self.swim
}

// wet reports whether a part of the motion's line is in water.
func (m motion) wet() bool { s := m.swim(); return s != nil && s.wet1 > s.wet0 }

// swimming reports whether the point p of the motion lies in water.
func (m motion) swimming(p Point) bool {
	return m.wet() && m.swim().water.Contains(int(p.X), int(p.Y), int(p.Z))
}

// pieces are the motion's stretches in order: each as the fraction of the
// line it covers and the seconds it takes, walked, swum, walked.
func (m motion) pieces() [3][2]float64 {
	dx, dy, dz := float64(m.to.X-m.from.X), float64(m.to.Y-m.from.Y), float64(m.to.Z-m.from.Z)
	flat := math.Hypot(dx, dy)
	speed := float64(max(m.speed, 1))
	if !m.wet() {
		return [3][2]float64{{1, flat / speed}}
	}
	space := math.Sqrt(dx*dx + dy*dy + dz*dz)
	s := m.swim()
	return [3][2]float64{
		{s.wet0, flat * s.wet0 / speed},
		{s.wet1 - s.wet0, space * (s.wet1 - s.wet0) / float64(s.speed)},
		{1 - s.wet1, flat * (1 - s.wet1) / speed},
	}
}

// progress is the fraction of the line covered elapsed seconds in.
func (m motion) progress(elapsed float64) float64 {
	f := 0.0
	for _, pc := range m.pieces() {
		if pc[0] <= 0 {
			continue
		}
		if elapsed < pc[1] {
			return f + pc[0]*elapsed/pc[1]
		}
		elapsed -= pc[1]
		f += pc[0]
	}
	return 1
}

// duration is how long the whole motion takes.
func (m motion) duration() time.Duration {
	if m.from == m.to {
		return 0
	}
	total := 0.0
	for _, pc := range m.pieces() {
		total += pc[1]
	}
	return time.Duration(total * float64(time.Second))
}

// stalled reports whether the motion is the bot's own move held at its
// from at speed 0, on its way to to with no arrival until the speed
// returns (creature.go retimeLocked).
func (m motion) stalled() bool { return m.self != nil && m.speed <= 0 && m.from != m.to }

// stopAt is a motion standing still at p.
func stopAt(p Point) motion { return motion{from: p, to: p} }

// at is the dead-reckoned position at now: along the line from from to to
// in the horizontal plane, z following in proportion, never past to; the
// part in water at the swim speed, measured in 3D.
func (m motion) at(now time.Time) Point {
	if m.stalled() {
		return m.from
	}
	if m.from == m.to || m.speed <= 0 {
		return m.to
	}
	elapsed := now.Sub(m.start).Seconds()
	if elapsed <= 0 {
		return m.from
	}
	dx, dy := float64(m.to.X-m.from.X), float64(m.to.Y-m.from.Y)
	if dx == 0 && dy == 0 && !m.wet() {
		return m.to
	}
	f := m.progress(elapsed)
	if f >= 1 {
		return m.to
	}
	return Point{
		X: m.from.X + int32(math.Round(dx*f)),
		Y: m.from.Y + int32(math.Round(dy*f)),
		Z: m.from.Z + int32(math.Round(float64(m.to.Z-m.from.Z)*f)),
	}
}

// moving reports whether the motion has not reached its end at now: for
// the bot's own move, its modeled arrival, and never while it is stalled.
func (m motion) moving(now time.Time) bool {
	if m.stalled() {
		return true
	}
	if m.self != nil {
		return now.Before(m.self.end)
	}
	return m.at(now) != m.to
}

// moveTo is a motion from the position at now toward dest at speed.
func (m motion) moveTo(now time.Time, origin, dest Point, speed int32) motion {
	n := m.withLine(m.sample(now, origin), dest, now)
	n.speed = speed
	return n
}

// sample is the position to continue from when the server reports p at
// now: p when it strays more than snapDrift from the estimate, else the
// estimate.
func (m motion) sample(now time.Time, p Point) Point {
	est := m.at(now)
	dx, dy, dz := int64(p.X-est.X), int64(p.Y-est.Y), int64(p.Z-est.Z)
	if dx*dx+dy*dy+dz*dz > snapDrift*snapDrift {
		return p
	}
	return est
}

// resample applies a position report p at now to a motion: a move carries
// on toward its destination from the sampled position, a still object
// stands at it.
func (m motion) resample(now time.Time, p Point) motion {
	from := m.sample(now, p)
	if !m.moving(now) {
		return stopAt(from)
	}
	return m.withLine(from, m.to, now)
}

// short is the point dist short of to on the line from from, or from when
// it is already that close.
func short(from, to Point, dist int32) Point {
	dx, dy := float64(to.X-from.X), float64(to.Y-from.Y)
	d := math.Hypot(dx, dy)
	if d <= float64(dist) || d == 0 {
		return from
	}
	f := (d - float64(max(dist, 0))) / d
	return Point{
		X: from.X + int32(math.Round(dx*f)),
		Y: from.Y + int32(math.Round(dy*f)),
		Z: from.Z + int32(math.Round(float64(to.Z-from.Z)*f)),
	}
}

// NewWorld returns the model of a character that just entered the world:
// self as EnterGame returned it, then every frame of its EnterWorld burst
// and its UserInfo applied in arrival order at now. A malformed frame is an
// error, as it is for the reader.
func NewWorld(self Self, now time.Time) (*World, error) {
	pos := Point{X: self.X, Y: self.Y, Z: self.Z}
	w := &World{
		Self:       SelfState{ObjectID: self.ObjectID, Name: self.Name, Heading: self.Heading, move: stopAt(pos)},
		Objects:    make(map[int32]*Object),
		Skills:     make(map[int32]KnownSkill),
		skillReady: make(map[int32]time.Time),
		Attackers:  make(map[int32]*Attacker),
		Ground:     make(map[int32]GroundItem),
	}
	for _, raw := range append(self.Burst[:len(self.Burst):len(self.Burst)], self.UserInfo) {
		if len(raw) == 0 {
			continue
		}
		ev, ok, err := decodeEvent(raw)
		if err != nil {
			return nil, fmt.Errorf("replay EnterWorld burst: %w", err)
		}
		if ok {
			w.Apply(ev, now)
		}
	}
	return w, nil
}

// object is the record of id, nil for the bot's own character or an
// object not in sight.
func (w *World) object(id int32) *Object {
	if id == w.Self.ObjectID {
		return nil
	}
	return w.Objects[id]
}

// motionOf is the motion of id, nil when id is neither the bot's own
// character nor in sight.
func (w *World) motionOf(id int32) *motion {
	if id == w.Self.ObjectID {
		return &w.Self.move
	}
	if o := w.Objects[id]; o != nil {
		return &o.move
	}
	return nil
}

// speedOf is the speed id moves at.
func (w *World) speedOf(id int32) int32 {
	if id == w.Self.ObjectID {
		return pace(w.Self.Running, w.Self.RunSpeed, w.Self.WalkSpeed)
	}
	if o := w.Objects[id]; o != nil {
		return pace(o.Running, o.RunSpeed, o.WalkSpeed)
	}
	return 0
}

func pace(running bool, run, walk int32) int32 {
	if running {
		return run
	}
	return walk
}

// count advances gen for ev unless it is a move, or another object's
// status or looks.
func (w *World) count(ev Event) {
	switch e := ev.(type) {
	case MoveToLocation, MoveToPawn, CharInfo, NPCInfo:
		return
	case StatusUpdate:
		if e.ObjectID != w.Self.ObjectID {
			return
		}
	}
	w.gen++
}

// Apply updates the model with one event received at now. Events the
// model does not track (chat, system messages, ActionFailed, lifecycle) are
// left to the brain and change nothing here.
func (w *World) Apply(ev Event, now time.Time) {
	w.count(ev)
	switch e := ev.(type) {
	case UserInfo:
		w.applyUserInfo(e, now)
	case StatusUpdate:
		w.applyStatus(e)
	case CharInfo:
		w.applyCharInfo(e, now)
	case NPCInfo:
		w.applyNPCInfo(e, now)
	case MoveToLocation:
		if e.ObjectID == w.Self.ObjectID {
			w.Self.move = w.Self.moved(now, e.Origin, e.Dest)
		} else if m := w.motionOf(e.ObjectID); m != nil {
			*m = m.moveTo(now, e.Origin, e.Dest, w.speedOf(e.ObjectID))
		}
	case MoveToPawn:
		if m := w.motionOf(e.ObjectID); m != nil {
			from := m.sample(now, e.Origin)
			dest := from
			t := w.motionOf(e.TargetID)
			if t != nil {
				dest = short(from, t.at(now), e.Distance)
			}
			switch {
			case e.ObjectID == w.Self.ObjectID && t != nil && e.Distance > 0 && short(e.Origin, t.at(now), e.Distance) == e.Origin:
				// In reach already, the server faces the target from where it
				// holds the bot (targeting.go thinkInteract, the player's
				// current location): the bot stands there, not on its own
				// estimate of a pawn approach's end, so its next segment
				// starts where the server's walk does (#434).
				w.Self.move = w.Self.moved(now, e.Origin, e.Origin)
			case e.ObjectID == w.Self.ObjectID:
				// The bot's own approach keeps its swim and arrival model.
				// Stopping short of a creature, the server's timer runs one
				// update past the one that ends the walk (creature.go
				// arrivalDelayLocked, within).
				mv := w.Self.moved(now, e.Origin, dest)
				if t != nil && e.Distance > 0 && mv.moving(now) {
					mv.self.end = mv.self.end.Add(positionTick)
				}
				w.Self.move = mv
			default:
				*m = motion{from: from, to: dest, start: now, speed: w.speedOf(e.ObjectID)}
			}
		}
	case StopMove:
		w.stop(e.ObjectID, e.Pos, e.Heading, now)
	case ValidateLocation:
		if e.ObjectID == w.Self.ObjectID {
			w.Self.correct(now, e.Pos)
			w.Self.Heading = e.Heading
		} else if m := w.motionOf(e.ObjectID); m != nil {
			*m = m.resample(now, e.Pos)
			w.setHeading(e.ObjectID, e.Heading)
		}
	case TeleportToLocation:
		w.applyTeleport(e)
	case DeleteObject:
		delete(w.Objects, e.ObjectID)
		delete(w.Attackers, e.ObjectID)
		delete(w.Ground, e.ObjectID)
		if w.Target == e.ObjectID {
			w.Target = 0
		}
	case Die:
		w.applyDie(e, now)
	case Revive:
		if e.ObjectID == w.Self.ObjectID {
			w.Self.Dead = false
			w.Prompts.Restart = RestartPrompt{}
			w.Prompts.Resurrect = ResurrectOffer{}
		} else if o := w.object(e.ObjectID); o != nil {
			o.Dead = false
		}
	case EtcStatusUpdate:
		w.Self.Charges = e.Charges
		w.applyDeathPenalty(e)
	case MyTargetSelected:
		w.Target = e.ObjectID
	case TargetUnselected:
		if e.ObjectID == w.Self.ObjectID {
			w.Target = 0
		}
	case ChangeWaitType:
		w.applyWaitType(e, now)
	case AutoAttackStart:
		w.setCombat(e.ObjectID, true)
	case AutoAttackStop:
		w.setCombat(e.ObjectID, false)
	case Attack:
		w.applyAttack(e, now)
	case MagicSkillUse:
		w.applyMagicSkillUse(e, now)
	case MagicSkillCanceled:
		// The bot's own cast stopped before it hit (X1); another caster's
		// last cast is the PvP opponent record's (X4).
		if e.CasterID == w.Self.ObjectID {
			w.Self.castEnd = time.Time{}
		}
	case SkillList:
		w.applySkillList(e)
	case SkillCoolTime:
		for _, t := range e.Timers {
			// The server truncates the remaining time to whole seconds
			// (character_flow.go skillCoolTimePacket): round up.
			w.skillReady[t.SkillID] = now.Add(t.Remaining + time.Second)
		}
	case RelationChanged:
		if o := w.object(e.ObjectID); o != nil {
			o.Karma, o.PvPFlag = e.Karma, e.PvPFlag
			if o.Kind == KindPlayer {
				o.Attackable = e.AutoAttackable
			}
		}
	case NpcHtmlMessage:
		w.Dialog = Dialog{ObjectID: e.ObjectID, Links: dialogLinks(e.HTML)}
	case ConfirmDlg:
		w.applyConfirmDlg(e)
	case AskJoinParty, PartySmallWindowAll, PartySmallWindowAdd, PartySmallWindowUpdate,
		PartySmallWindowDelete, PartySmallWindowDeleteAll, PartyMemberPosition:
		w.applyParty(ev, now)
	case SystemMessage:
		w.applyDeathPenalty(e)
		w.applyClan(ev, now)
	case AskJoinPledge, JoinPledge, PledgeShowMemberListAll, PledgeShowMemberListDeleteAll:
		w.applyClan(ev, now)
	case ItemList, InventoryUpdate, ExAutoSoulShot:
		w.applyItems(ev)
	case SendTradeRequest, TradeStart, TradeOwnAdd, TradeOtherAdd, TradePressOwnOk, TradePressOtherOk, SendTradeDone:
		w.applyTrade(ev, now)
	case AbnormalStatusUpdate, PartySpelled:
		w.applyEffects(ev, now)
	case SpawnItem, DropItem, GetItem:
		w.applyGround(ev)
	case RecipeBookItemList:
		w.Recipes.apply(e)
	case HennaInfo:
		w.Symbols = Symbols{Known: true, MaxSlots: e.MaxSlots, Worn: e.Worn}
	}
}

func (w *World) applyUserInfo(e UserInfo, now time.Time) {
	s := &w.Self
	s.ObjectID, s.Name = e.ObjectID, e.Name
	s.BaseClass, s.ClassID, s.Level, s.Exp, s.SP = e.BaseClass, e.ClassID, e.Level, e.Exp, e.SP
	s.Race = e.Race
	s.HP, s.MaxHP, s.MP, s.MaxMP, s.CP, s.MaxCP = e.HP, e.MaxHP, e.MP, e.MaxMP, e.CP, e.MaxCP
	s.Load, s.MaxLoad, s.InvLimit = e.Load, e.MaxLoad, e.InvLimit
	s.AllyID = e.AllyID
	s.PvPFlag, s.Karma, s.StoreType = uint8(e.PvPFlag), e.Karma, e.StoreType
	s.RunSpeed, s.WalkSpeed, s.Running = e.RunSpeed, e.WalkSpeed, e.Running
	s.SwimSpeed = e.SwimSpeed
	s.PAtkSpd, s.CollisionHeight = e.PAtkSpd, e.CollisionHeight
	s.PAtk, s.PDef, s.Evasion, s.Accuracy = e.PAtk, e.PDef, e.Evasion, e.Accuracy
	s.Critical, s.MAtk, s.CastSpd, s.MDef = e.Critical, e.MAtk, e.CastSpd, e.MDef
	s.STR, s.DEX, s.CON, s.INT, s.WIT, s.MEN = e.STR, e.DEX, e.CON, e.INT, e.WIT, e.MEN
	s.Heading = e.Heading
	w.applyClanID(e.ClanID)
	s.resample(now, e.Pos)
	delete(w.Objects, e.ObjectID)
}

func (w *World) applyStatus(e StatusUpdate) {
	if e.ObjectID == w.Self.ObjectID {
		s := &w.Self
		set := func(f StatusFields, dst *int32, v int32) {
			if e.Set&f != 0 {
				*dst = v
			}
		}
		set(StatusHP, &s.HP, e.HP)
		set(StatusMaxHP, &s.MaxHP, e.MaxHP)
		set(StatusMP, &s.MP, e.MP)
		set(StatusMaxMP, &s.MaxMP, e.MaxMP)
		set(StatusCP, &s.CP, e.CP)
		set(StatusMaxCP, &s.MaxCP, e.MaxCP)
		set(StatusSP, &s.SP, e.SP)
		set(StatusLoad, &s.Load, e.Load)
		set(StatusMaxLoad, &s.MaxLoad, e.MaxLoad)
		set(StatusKarma, &s.Karma, e.Karma)
		return
	}
	o := w.object(e.ObjectID)
	if o == nil {
		return
	}
	if e.Set&StatusHP != 0 {
		o.HP = e.HP
	}
	if e.Set&StatusMaxHP != 0 {
		o.MaxHP = e.MaxHP
	}
	if e.Set&StatusKarma != 0 {
		o.Karma = e.Karma
	}
}

// upsert returns the record of id, a new one standing at pos when id was
// not in sight, else the existing one with pos applied as a position
// report. It returns nil for the bot's own character.
func (w *World) upsert(id int32, pos Point, now time.Time) *Object {
	if id == w.Self.ObjectID {
		return nil
	}
	o := w.Objects[id]
	if o == nil {
		o = &Object{move: stopAt(pos)}
		w.Objects[id] = o
		return o
	}
	o.move = o.move.resample(now, pos)
	return o
}

func (w *World) applyCharInfo(e CharInfo, now time.Time) {
	o := w.upsert(e.ObjectID, e.Pos, now)
	if o == nil {
		return
	}
	*o = Object{
		Kind: KindPlayer, Name: e.Name, ClassID: e.ClassID, Karma: e.Karma,
		HP: o.HP, MaxHP: o.MaxHP,
		RunSpeed: e.RunSpeed, WalkSpeed: e.WalkSpeed, Heading: e.Heading,
		PvPFlag: uint8(e.PvPFlag), StoreType: e.StoreType, Attackable: o.Attackable,
		Sitting: e.Sitting, Running: e.Running, InCombat: e.InCombat, Dead: e.Dead,
		move: o.move,
	}
	if o.move.speed != 0 {
		o.move.speed = pace(o.Running, o.RunSpeed, o.WalkSpeed)
	}
}

func (w *World) applyNPCInfo(e NPCInfo, now time.Time) {
	o := w.upsert(e.ObjectID, e.Pos, now)
	if o == nil {
		return
	}
	*o = Object{
		Kind: KindNPC, NPCID: e.Template(), Karma: e.Karma,
		HP: o.HP, MaxHP: o.MaxHP,
		RunSpeed: e.RunSpeed, WalkSpeed: e.WalkSpeed, Heading: e.Heading,
		PvPFlag:    uint8(e.PvPFlag),
		Attackable: e.Attackable, Summon: e.Summon,
		Running: e.Running, InCombat: e.InCombat, Dead: e.Dead,
		move: o.move,
	}
	if o.move.speed != 0 {
		o.move.speed = pace(o.Running, o.RunSpeed, o.WalkSpeed)
	}
}

// stop ends id's move at the position the server reports at now.
func (w *World) stop(id int32, pos Point, heading int32, now time.Time) {
	if m := w.motionOf(id); m != nil {
		*m = stopAt(pos)
		m.start = now
		w.setHeading(id, heading)
	}
}

func (w *World) setHeading(id, heading int32) {
	if id == w.Self.ObjectID {
		w.Self.Heading = heading
	} else if o := w.Objects[id]; o != nil {
		o.Heading = heading
	}
}

// applyTeleport places the object at its new position. The bot's own
// loading-screen teleport also forgets every object in sight and the
// target, as the client does: the server shows the objects around the
// landing point again once the client reports Appearing.
func (w *World) applyTeleport(e TeleportToLocation) {
	m := w.motionOf(e.ObjectID)
	if m == nil {
		return
	}
	*m = stopAt(e.Pos)
	if e.ObjectID == w.Self.ObjectID && !e.Fast {
		// The landing closes the bot's store and says nothing
		// (network/privatestore.go closeStoreOnTeleport).
		w.Self.StoreType = 0
		clear(w.Objects)
		clear(w.Attackers)
		clear(w.Ground)
		w.Target = 0
	}
}

func (w *World) applyDie(e Die, now time.Time) {
	if e.ObjectID == w.Self.ObjectID {
		w.Self.Dead = true
		w.Self.move = stopAt(w.Self.move.at(now))
		w.Self.castEnd = time.Time{}
		clear(w.Attackers)
		w.Prompts.Restart = RestartPrompt{Open: true, Points: e.Restart, At: now}
		return
	}
	delete(w.Attackers, e.ObjectID)
	if o := w.object(e.ObjectID); o != nil {
		o.Dead = true
		o.move = stopAt(o.move.at(now))
	}
}

// applyWaitType records a sit or stand, which also stops the object at
// the reported position. A fake death shows the object dead to others; the
// bot's own fake death leaves its model alive.
func (w *World) applyWaitType(e ChangeWaitType, now time.Time) {
	self := e.ObjectID == w.Self.ObjectID
	o := w.object(e.ObjectID)
	if !self && o == nil {
		return
	}
	switch e.Type {
	case WaitSitting, WaitStanding:
		sitting := e.Type == WaitSitting
		if self {
			w.Self.Sitting = sitting
			w.Self.move = stopAt(e.Pos)
		} else {
			o.Sitting = sitting
			o.move = stopAt(e.Pos)
		}
	case WaitFakeDeathStart, WaitFakeDeathStop:
		if !self {
			o.Dead = e.Type == WaitFakeDeathStart
			o.move = stopAt(o.move.at(now))
		}
	}
}

func (w *World) setCombat(id int32, on bool) {
	if id == w.Self.ObjectID {
		w.Self.InCombat = on
	} else if o := w.Objects[id]; o != nil {
		o.InCombat = on
	}
}
