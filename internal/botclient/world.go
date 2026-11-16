package botclient

import (
	"fmt"
	"math"
	"time"
)

// snapDrift is how far, in world units, a position the server reports for
// a moving object may stray from the dead-reckoned one before the model
// snaps to it. Closer reports keep the estimate.
const snapDrift = 200

// World is one bot's model of the game world, built from the events its
// session's reader decodes. It is owned by the bot's brain goroutine: no
// other goroutine reads or writes it, so it has no lock. Positions are dead
// reckoned from the last movement report, so readers pass the current time.
type World struct {
	Self SelfState
	// Objects are the objects in sight by object id, the bot's own
	// character excluded.
	Objects map[int32]*Object
	// Target is the object id the server confirmed as the bot's target, 0
	// for none.
	Target  int32
	Prompts Prompts
}

// SelfState is the bot's own character.
type SelfState struct {
	ObjectID int32
	Name     string
	// BaseClass is the class drawn on the model, ClassID the class played.
	BaseClass, ClassID int32
	Level              int32
	Exp                int64
	SP                 int32
	HP, MaxHP          int32
	MP, MaxMP          int32
	CP, MaxCP          int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad int32
	PvPFlag       uint8
	Karma         int32
	// StoreType is the private store mode, 0 for none.
	StoreType                        uint8
	Sitting, InCombat, Dead, Running bool
	RunSpeed, WalkSpeed, Heading     int32
	move                             motion
}

// Position is where the character is at now, dead reckoned along its
// current move.
func (s *SelfState) Position(now time.Time) Point { return s.move.at(now) }

// Moving reports whether the character is still on its way at now.
func (s *SelfState) Moving(now time.Time) bool { return s.move.moving(now) }

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
	Attackable, Summon  bool
	Sitting, Running    bool
	InCombat, Dead      bool
	move                motion
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
}

// RestartPrompt is the restart-point choice offered to a dead character.
type RestartPrompt struct {
	Open   bool
	Points RestartPoints
}

// motion is a straight move from from to to at speed world units per
// second, begun at start. A stopped object has from == to.
type motion struct {
	from, to Point
	start    time.Time
	speed    int32
}

// stopAt is a motion standing still at p.
func stopAt(p Point) motion { return motion{from: p, to: p} }

// at is the dead-reckoned position at now: along the line from from to to
// in the horizontal plane, z following in proportion, never past to.
func (m motion) at(now time.Time) Point {
	if m.from == m.to || m.speed <= 0 {
		return m.to
	}
	elapsed := now.Sub(m.start).Seconds()
	if elapsed <= 0 {
		return m.from
	}
	dx, dy := float64(m.to.X-m.from.X), float64(m.to.Y-m.from.Y)
	dist := math.Hypot(dx, dy)
	travel := float64(m.speed) * elapsed
	if dist == 0 || travel >= dist {
		return m.to
	}
	f := travel / dist
	return Point{
		X: m.from.X + int32(math.Round(dx*f)),
		Y: m.from.Y + int32(math.Round(dy*f)),
		Z: m.from.Z + int32(math.Round(float64(m.to.Z-m.from.Z)*f)),
	}
}

// moving reports whether the motion has not reached its end at now.
func (m motion) moving(now time.Time) bool {
	return m.at(now) != m.to
}

// moveTo is a motion from the position at now toward dest at speed.
func (m motion) moveTo(now time.Time, origin, dest Point, speed int32) motion {
	return motion{from: m.sample(now, origin), to: dest, start: now, speed: speed}
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
	return motion{from: from, to: m.to, start: now, speed: m.speed}
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
		Self:    SelfState{ObjectID: self.ObjectID, Name: self.Name, Heading: self.Heading, move: stopAt(pos)},
		Objects: make(map[int32]*Object),
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

// Apply updates the model with one event received at now. Events the
// model does not track (chat, system messages, ActionFailed, lifecycle) are
// left to the brain and change nothing here.
func (w *World) Apply(ev Event, now time.Time) {
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
		if m := w.motionOf(e.ObjectID); m != nil {
			*m = m.moveTo(now, e.Origin, e.Dest, w.speedOf(e.ObjectID))
		}
	case MoveToPawn:
		if m := w.motionOf(e.ObjectID); m != nil {
			from := m.sample(now, e.Origin)
			dest := from
			if t := w.motionOf(e.TargetID); t != nil {
				dest = short(from, t.at(now), e.Distance)
			}
			*m = motion{from: from, to: dest, start: now, speed: w.speedOf(e.ObjectID)}
		}
	case StopMove:
		w.stop(e.ObjectID, e.Pos, e.Heading)
	case ValidateLocation:
		if m := w.motionOf(e.ObjectID); m != nil {
			*m = m.resample(now, e.Pos)
			w.setHeading(e.ObjectID, e.Heading)
		}
	case TeleportToLocation:
		w.applyTeleport(e)
	case DeleteObject:
		delete(w.Objects, e.ObjectID)
		if w.Target == e.ObjectID {
			w.Target = 0
		}
	case Die:
		w.applyDie(e, now)
	case Revive:
		if e.ObjectID == w.Self.ObjectID {
			w.Self.Dead = false
			w.Prompts.Restart = RestartPrompt{}
		} else if o := w.object(e.ObjectID); o != nil {
			o.Dead = false
		}
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
	}
}

func (w *World) applyUserInfo(e UserInfo, now time.Time) {
	s := &w.Self
	s.ObjectID, s.Name = e.ObjectID, e.Name
	s.BaseClass, s.ClassID, s.Level, s.Exp, s.SP = e.BaseClass, e.ClassID, e.Level, e.Exp, e.SP
	s.HP, s.MaxHP, s.MP, s.MaxMP, s.CP, s.MaxCP = e.HP, e.MaxHP, e.MP, e.MaxMP, e.CP, e.MaxCP
	s.Load, s.MaxLoad = e.Load, e.MaxLoad
	s.PvPFlag, s.Karma, s.StoreType = uint8(e.PvPFlag), e.Karma, e.StoreType
	s.RunSpeed, s.WalkSpeed, s.Running = e.RunSpeed, e.WalkSpeed, e.Running
	s.Heading = e.Heading
	s.move = s.move.resample(now, e.Pos)
	if s.move.speed != 0 {
		s.move.speed = pace(s.Running, s.RunSpeed, s.WalkSpeed)
	}
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
		PvPFlag: uint8(e.PvPFlag), StoreType: e.StoreType,
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
		Kind: KindNPC, NPCID: e.NPCID, Karma: e.Karma,
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

// stop ends id's move at the position the server reports.
func (w *World) stop(id int32, pos Point, heading int32) {
	if m := w.motionOf(id); m != nil {
		*m = stopAt(pos)
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
		clear(w.Objects)
		w.Target = 0
	}
}

func (w *World) applyDie(e Die, now time.Time) {
	if e.ObjectID == w.Self.ObjectID {
		w.Self.Dead = true
		w.Self.move = stopAt(w.Self.move.at(now))
		w.Prompts.Restart = RestartPrompt{Open: true, Points: e.Restart}
		return
	}
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
