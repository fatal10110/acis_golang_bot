package botclient

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The panel's inspect (#437): a one-off request in the Bot's mailbox that
// the session's brain answers with what its world model holds of the
// inventory, the worn gear, the penalties and the effects. The brain asks
// the server for the inventory once (RequestItemList) and answers once the
// ItemList that follows is applied, or with the model as it stands, marked
// stale, at the request's deadline: the server leaves the request
// unanswered while a shop or warehouse window is open
// (network/client_ingame.go handleRequestItemList, inventoryDisabled).
// Nothing of it is kept: the Inspection is built, handed over and dropped.

// ErrOffline answers an inspect while no session's brain runs: between
// sessions there is no world model to read.
var ErrOffline = errors.New("bot offline")

// Inspection is what a session's brain held of its character when it
// answered an inspect (Bot.Inspect). Items, Effects and the slices in it
// are the receiver's own.
type Inspection struct {
	// At is when it was taken, on the bot's clock.
	At time.Time
	// Stale is set when no ItemList came between the request and its
	// deadline: Items are the inventory as the model last held it.
	Stale bool
	// Items are the carried items, worn ones included, by object id.
	Items []Item
	// Effects are the bot's own effects (AbnormalStatusUpdate) by skill
	// id, Left counted from At.
	Effects []Effect
	ClassID int32
	// Load and MaxLoad are the carried weight and the weight limit, and
	// WeightPenalty the weight band (weightBand), as in Status.
	Load, MaxLoad, WeightPenalty int32
	// GradePenalty is set while the server holds the grade penalty
	// (skill 4267), and Expertise is the Expertise skill's level: the
	// highest crystal grade worn without it.
	GradePenalty bool
	Expertise    int32
	DeathPenalty int32
	// RunSpeed is the run speed the server gave last, its penalties
	// applied.
	RunSpeed int32
}

// Inspect hands an inspect to the running session's brain and returns the
// channel its one Inspection comes on. The brain answers by wait from now
// at the latest, stale when the server's ItemList did not come by then;
// the channel closes without a value when the bot stops first. It never
// waits for the bot: a full mailbox answers ErrBusy, a stopped bot
// ErrStopped, and a bot between sessions ErrOffline.
func (b *Bot) Inspect(wait time.Duration) (<-chan Inspection, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.stopped:
		return nil, ErrStopped
	case !b.inSession:
		return nil, ErrOffline
	}
	m := mail{inspect: make(chan Inspection, 1), deadline: b.cfg.Clock.Now().Add(wait)}
	select {
	case b.link.mail <- m:
		return m.inspect, nil
	default:
		return nil, ErrBusy
	}
}

// inspectState are the inspects the brain has not answered yet. They
// share one RequestItemList: asked is set once it went out, wakeAt is when
// the next try to send it is due, and deadline is the earliest of theirs.
type inspectState struct {
	replies          []chan<- Inspection
	deadline, wakeAt time.Time
	asked            bool
}

// due is when the loop must wake for the inspects: at wakeAt while the
// request still has to go out (the frame bucket may hold it), else at the
// deadline; ok is false while none waits.
func (s *inspectState) due() (t time.Time, ok bool) {
	if len(s.replies) == 0 {
		return time.Time{}, false
	}
	if !s.asked && s.wakeAt.Before(s.deadline) {
		return s.wakeAt, true
	}
	return s.deadline, true
}

// inspectAsked takes the inspect of m at now. One whose deadline passed
// (it waited in the mailbox between sessions) is answered at once and
// sends nothing; the others wait for the loop's next wake, which sends the
// one RequestItemList they share.
func (b *Brain) inspectAsked(m mail, now time.Time) {
	if !now.Before(m.deadline) {
		m.inspect <- b.inspection(now, true)
		return
	}
	s := &b.inspect
	if len(s.replies) == 0 || m.deadline.Before(s.deadline) {
		s.deadline = m.deadline
	}
	s.replies = append(s.replies, m.inspect)
	s.wakeAt = now
}

// inspectWake sends the inspects' RequestItemList, or answers them stale
// at their deadline. RequestItemList has no server gate (gateNone): the
// frame bucket bounds it, one per inspect at most, and the panel bounds the
// inspects (one per bot in flight, a cooldown, a global cap).
func (b *Brain) inspectWake(now time.Time) error {
	s := &b.inspect
	if len(s.replies) == 0 {
		return nil
	}
	if !now.Before(s.deadline) {
		b.answerInspect(now, true)
		return nil
	}
	if s.asked || now.Before(s.wakeAt) {
		return nil
	}
	sent, err := b.send(Action{kind: actItemList, payload: encodeRequestItemList()}, now)
	if err != nil {
		return err
	}
	if s.asked = sent; !sent {
		s.wakeAt = now.Add(time.Second / frameRate)
	}
	return nil
}

// answerInspect answers every inspect waiting with the model at now.
func (b *Brain) answerInspect(now time.Time, stale bool) {
	s := &b.inspect
	if len(s.replies) == 0 {
		return
	}
	in := b.inspection(now, stale)
	for i, r := range s.replies {
		if i > 0 {
			in.Items, in.Effects = slices.Clone(in.Items), slices.Clone(in.Effects)
		}
		r <- in
	}
	*s = inspectState{}
}

// inspection is the world model at now as an Inspection.
func (b *Brain) inspection(now time.Time, stale bool) Inspection {
	w := b.world
	s := &w.Self
	in := Inspection{
		At: now, Stale: stale,
		ClassID: s.ClassID, Load: s.Load, MaxLoad: s.MaxLoad, WeightPenalty: weightBand(s.Load, s.MaxLoad),
		GradePenalty: w.gradePenalty(), Expertise: w.Skills[expertiseSkill].Level,
		DeathPenalty: s.DeathPenalty, RunSpeed: s.RunSpeed,
	}
	in.Items = make([]Item, 0, len(w.Inventory.Items))
	for _, it := range w.Inventory.Items {
		in.Items = append(in.Items, it)
	}
	slices.SortFunc(in.Items, func(a, b Item) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
	in.Effects = make([]Effect, 0, len(w.Buffs))
	for id, end := range w.Buffs {
		e := Effect{SkillID: id, Level: w.BuffLevels[id], Permanent: end.IsZero()}
		if !e.Permanent {
			e.Left = max(end.Sub(now), 0)
		}
		in.Effects = append(in.Effects, e)
	}
	slices.SortFunc(in.Effects, func(a, b Effect) int { return cmp.Compare(a.SkillID, b.SkillID) })
	return in
}

// encodeRequestItemList asks the server for the whole inventory: it
// answers with an ItemList (network/client_ingame.go handleRequestItemList,
// no payload).
func encodeRequestItemList() []byte {
	return wire.NewPacketWriter(opGameRequestItemList).Bytes()
}

// bodySlotNames name the body part masks ItemList reports for a worn item
// (bodyParts).
var bodySlotNames = map[int32]string{
	partHead: "head", partRHand: "weapon", partLRHand: "two-handed weapon", partLHand: "shield",
	partChest: "chest", partFullArmor: "full armor", partLegs: "legs", partGloves: "gloves", partFeet: "feet",
	partNeck: "necklace", partLREar: "earring", partLRFinger: "ring",
}

// BodySlotName names the body part mask of a worn Item (Item.BodySlot).
func BodySlotName(mask int32) string {
	if n, ok := bodySlotNames[mask]; ok {
		return n
	}
	return fmt.Sprintf("slot %#x", mask)
}
