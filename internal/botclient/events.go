package botclient

import (
	"slices"
	"sync"
)

// Event is one decoded server packet, handed from a session's reader to its
// brain through Events. Every event is a value type that shares no memory
// with the frame it was decoded from: the reader reuses its frame buffer, so
// no slice of it may cross goroutines. The brain type-switches on the
// concrete types below.
type Event interface{ event() }

// Point is a world coordinate.
type Point struct{ X, Y, Z int32 }

// MoveToLocation reports an object starting to move from Origin toward Dest.
// It is the one event Events coalesces: a newer one for the same object
// supersedes a queued one.
type MoveToLocation struct {
	ObjectID     int32
	Dest, Origin Point
}

// StopMove reports an object stopped at Pos.
type StopMove struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// ValidateLocation corrects an object's position to Pos.
type ValidateLocation struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// TeleportToLocation reports an object placed at Pos without walking there.
// Fast is an in-place correction rather than a loading-screen teleport.
type TeleportToLocation struct {
	ObjectID int32
	Pos      Point
	Fast     bool
}

// DeleteObject reports an object leaving sight. Seated marks a sitting
// character the client stands up before removing it.
type DeleteObject struct {
	ObjectID int32
	Seated   bool
}

// LeaveWorld is the server's answer to an accepted Logout; it closes the
// connection right after.
type LeaveWorld struct{}

// ServerClose is the server's last packet before it drops the connection.
type ServerClose struct{}

// UserInfo is the bot's own character in full. The server sends it on
// entering the world and again whenever the character changes.
type UserInfo struct {
	ObjectID int32
	Name     string
	Pos      Point
	Heading  int32
	// BaseClass is the class drawn on the model, ClassID the class played.
	Race, Sex, BaseClass, ClassID int32
	Level                         int32
	Exp                           int64
	SP                            int32
	HP, MaxHP                     int32
	MP, MaxMP                     int32
	CP, MaxCP                     int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad  int32
	PvPFlag, Karma int32
	// RunSpeed and WalkSpeed are effective speeds in world units per
	// second: the base speed the packet carries times its movement
	// multiplier, as the client computes them.
	RunSpeed, WalkSpeed int32
	Running             bool
	// StoreType is the private store mode (the server's operate type; 0
	// for none).
	StoreType uint8
	IsGM      bool
	// ClanID is the character's clan, 0 for none.
	ClanID int32
	// Abnormal is the abnormal effect mask drawn on the character; an
	// invisible game master sees AbnormalStealth in its own.
	Abnormal int32
}

// AbnormalStealth is the abnormal effect bit of an invisible game master
// in its own UserInfo (server skill.AbnormalStealth, userinfo.go).
const AbnormalStealth int32 = 0x100000

// StatusFields is a set of StatusUpdate fields.
type StatusFields uint16

// The StatusUpdate fields the model keeps; the packet's other attributes
// (attack and cast speed) are consumed and dropped.
const (
	StatusHP StatusFields = 1 << iota
	StatusMaxHP
	StatusMP
	StatusMaxMP
	StatusCP
	StatusMaxCP
	StatusSP
	StatusLoad
	StatusMaxLoad
	StatusKarma
)

// StatusUpdate reports new values for some of an object's attributes: Set
// names the fields the packet carried; the others are zero and unchanged.
type StatusUpdate struct {
	ObjectID  int32
	Set       StatusFields
	HP, MaxHP int32
	MP, MaxMP int32
	CP, MaxCP int32
	SP        int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad int32
	Karma         int32
}

// CharInfo describes another player in sight.
type CharInfo struct {
	ObjectID int32
	Name     string
	Pos      Point
	Heading  int32
	ClassID  int32
	PvPFlag  int32
	Karma    int32
	// RunSpeed and WalkSpeed are effective speeds, as in UserInfo.
	RunSpeed, WalkSpeed int32
	Sitting, Running    bool
	InCombat, Dead      bool
	// StoreType is the private store mode, as in UserInfo.
	StoreType uint8
}

// NPCInfo describes an NPC (a monster, a townsperson or a summon) in sight.
type NPCInfo struct {
	ObjectID int32
	// NPCID is the NPC template id.
	NPCID      int32
	Attackable bool
	Pos        Point
	Heading    int32
	// RunSpeed and WalkSpeed are effective speeds, as in UserInfo.
	RunSpeed, WalkSpeed int32
	Running, InCombat   bool
	Dead, Summon        bool
	PvPFlag, Karma      int32
}

// MoveToPawn reports an object starting to move from Origin toward
// TargetID until it is Distance away from it.
type MoveToPawn struct {
	ObjectID, TargetID int32
	Distance           int32
	Origin             Point
}

// RestartPoints are the places a dead character may choose to restart at.
type RestartPoints struct {
	Village, ClanHall, Castle, SiegeHQ bool
	// Fixed is a restart in place.
	Fixed bool
}

// Die reports an object's death. For the bot's own character, Restart lists
// the restart points it is offered.
type Die struct {
	ObjectID  int32
	Restart   RestartPoints
	Sweepable bool
}

// Revive reports an object brought back to life.
type Revive struct{ ObjectID int32 }

// MyTargetSelected confirms the bot's own target; Color is the level
// difference color the client draws.
type MyTargetSelected struct {
	ObjectID int32
	Color    uint16
}

// TargetUnselected reports that the object ObjectID, standing at Pos,
// cleared its target.
type TargetUnselected struct {
	ObjectID int32
	Pos      Point
}

// SystemMessage is a system message by id. Text holds its first two text
// parameters in order, "" past the ones it has; every other parameter is
// consumed and dropped.
type SystemMessage struct {
	ID   int32
	Text [2]string
}

// ActionFailed tells the client its last action request was not carried
// out.
type ActionFailed struct{}

// CreatureSay is a chat line on channel Type, said by ObjectID (0 for none)
// under Name.
type CreatureSay struct {
	ObjectID, Type int32
	Name, Text     string
}

// WaitType is a ChangeWaitType stance.
type WaitType int32

// The stances ChangeWaitType reports (serverpackets/stance_social.go).
const (
	WaitSitting        WaitType = 0
	WaitStanding       WaitType = 1
	WaitFakeDeathStart WaitType = 2
	WaitFakeDeathStop  WaitType = 3
)

// ChangeWaitType reports an object at Pos changing stance.
type ChangeWaitType struct {
	ObjectID int32
	Type     WaitType
	Pos      Point
}

// AutoAttackStart reports an object entering its combat stance.
type AutoAttackStart struct{ ObjectID int32 }

// AutoAttackStop reports an object leaving its combat stance.
type AutoAttackStop struct{ ObjectID int32 }

func (MoveToLocation) event()     {}
func (StopMove) event()           {}
func (ValidateLocation) event()   {}
func (TeleportToLocation) event() {}
func (DeleteObject) event()       {}
func (LeaveWorld) event()         {}
func (ServerClose) event()        {}
func (UserInfo) event()           {}
func (StatusUpdate) event()       {}
func (CharInfo) event()           {}
func (NPCInfo) event()            {}
func (MoveToPawn) event()         {}
func (Die) event()                {}
func (Revive) event()             {}
func (MyTargetSelected) event()   {}
func (TargetUnselected) event()   {}
func (SystemMessage) event()      {}
func (ActionFailed) event()       {}
func (CreatureSay) event()        {}
func (ChangeWaitType) event()     {}
func (AutoAttackStart) event()    {}
func (AutoAttackStop) event()     {}

// Events is the bounded queue from one session's reader to its brain, one
// per session attempt.
//
// The contract: the reader (GameConn.ReadEvents) is the only producer and
// the brain the only consumer. The brain selects on Ready next to its timer
// and the session's done channel, then takes everything queued with Drain.
// When limit events are queued the reader waits for a Drain or for done,
// whichever comes first, so closing done never deadlocks a reader on a full
// queue. While the brain is behind, a MoveToLocation replaces the queued
// MoveToLocation of the same object and moves to the back of the queue: an
// older move is superseded by a newer one, and the newer one stays behind
// every other event already queued for that object (StopMove, a teleport,
// DeleteObject), so coalescing never reorders one object's events.
type Events struct {
	mu    sync.Mutex
	queue []Event // guarded by mu; at most one MoveToLocation per object
	limit int
	ready chan struct{} // capacity 1: signaled when an event is queued
	space chan struct{} // capacity 1: signaled when Drain empties the queue
}

// NewEvents returns an empty queue that holds up to limit events (at least
// one).
func NewEvents(limit int) *Events {
	limit = max(limit, 1)
	return &Events{
		queue: make([]Event, 0, limit),
		limit: limit,
		ready: make(chan struct{}, 1),
		space: make(chan struct{}, 1),
	}
}

// Ready yields once events are queued. A wakeup may find the queue already
// drained; Drain then returns dst unchanged.
func (q *Events) Ready() <-chan struct{} { return q.ready }

// Drain appends every queued event to dst in arrival order, empties the
// queue and returns dst.
func (q *Events) Drain(dst []Event) []Event {
	q.mu.Lock()
	dst = append(dst, q.queue...)
	clear(q.queue)
	q.queue = q.queue[:0]
	q.mu.Unlock()
	signal(q.space)
	return dst
}

// push queues ev, coalescing a MoveToLocation, and waits for room while the
// queue is full. It reports false when done closed first.
func (q *Events) push(ev Event, done <-chan struct{}) bool {
	for {
		q.mu.Lock()
		ok := q.put(ev)
		q.mu.Unlock()
		if ok {
			signal(q.ready)
			return true
		}
		select {
		case <-q.space:
		case <-done:
			return false
		}
	}
}

// put queues ev unless the queue is full. A MoveToLocation first drops the
// queued move of its object, so it always fits.
func (q *Events) put(ev Event) bool {
	if m, ok := ev.(MoveToLocation); ok {
		if i := slices.IndexFunc(q.queue, func(e Event) bool {
			prev, ok := e.(MoveToLocation)
			return ok && prev.ObjectID == m.ObjectID
		}); i >= 0 {
			q.queue = slices.Delete(q.queue, i, i+1)
		}
	}
	if len(q.queue) >= q.limit {
		return false
	}
	q.queue = append(q.queue, ev)
	return true
}

// signal wakes c's receiver without blocking; a pending wakeup suffices.
func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}
