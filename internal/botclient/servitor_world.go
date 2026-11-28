package botclient

import "time"

// Servitor is the bot's own servitor as its owner-only packets tell it
// (PetInfo, PetStatusUpdate, SetSummonRemainTime, PetDelete; docs/servitors.md
// section 1). The server shows it to its owner with these alone, never as an
// NPCInfo, so it is not among World.Objects. The brain goroutine owns it with
// the rest of the World.
type Servitor struct {
	ObjectID int32
	// TemplateID is its NPC template id.
	TemplateID int32
	Level      int32
	// Pos is where the last PetInfo, PetStatusUpdate or swing put it.
	Pos                    Point
	HP, MaxHP, MP, MaxMP   int32
	PAtk, PDef, MAtk, MDef int32
	PAtkSpd                int32
	CollisionRadius        float64
	AbnormalMask           int32
	InCombat, Dead         bool
	// Remaining and Lifetime are the last lifetime bar's remaining and
	// whole lifetime, zero before the first.
	Remaining, Lifetime time.Duration
	// Target is the object it last swung at or walked to (Attack,
	// MoveToPawn), 0 for none; no pet packet carries it.
	Target int32
}

// applyServitor applies one of the servitor's own packets. A pet's (summon
// type 2) are ignored: the bot keeps servitors only (Brain.petEvent logs
// them). PetDelete is the removal; a dead servitor stays until its corpse
// goes.
func (w *World) applyServitor(ev Event) {
	switch e := ev.(type) {
	case PetInfo:
		if e.SummonType != SummonServitor {
			return
		}
		s := w.Servitor
		if s == nil || s.ObjectID != e.ObjectID {
			s = &Servitor{ObjectID: e.ObjectID}
			w.Servitor = s
		}
		s.TemplateID, s.Level, s.Pos = e.TemplateID, e.Level, e.Pos
		s.HP, s.MaxHP, s.MP, s.MaxMP = e.CurHP, e.MaxHP, e.CurMP, e.MaxMP
		s.PAtk, s.PDef, s.MAtk, s.MDef = e.PAtk, e.PDef, e.MAtk, e.MDef
		s.PAtkSpd, s.CollisionRadius, s.AbnormalMask = e.PAtkSpd, e.CollisionRadius, e.AbnormalMask
		s.InCombat, s.Dead = e.InCombat, e.Dead
	case PetStatusUpdate:
		if s := w.servitor(e.SummonType, e.ObjectID); s != nil {
			s.Pos, s.Level = e.Pos, e.Level
			s.HP, s.MaxHP, s.MP, s.MaxMP = e.CurHP, e.MaxHP, e.CurMP, e.MaxMP
		}
	case PetDelete:
		if w.servitor(e.SummonType, e.ObjectID) != nil {
			w.Servitor = nil
		}
	case SetSummonRemainTime:
		// Only a servitor has a lifetime (summon_events.go LifetimeShown).
		if s := w.Servitor; s != nil {
			s.Remaining, s.Lifetime = e.Remaining, e.Total
		}
	}
}

// servitor returns the bot's servitor when it is the summon of the given
// type and object id, else nil.
func (w *World) servitor(summonType, id int32) *Servitor {
	if s := w.Servitor; s != nil && summonType == SummonServitor && s.ObjectID == id {
		return s
	}
	return nil
}

// servitorAttack takes the servitor's target from its own swing, and its
// place: the server stops an attacker before it swings and Pos is where it
// stands (applyAttack).
func (w *World) servitorAttack(e Attack) {
	s := w.Servitor
	if s == nil || e.AttackerID != s.ObjectID || len(e.Hits) == 0 {
		return
	}
	s.Target, s.Pos = e.Hits[0].TargetID, e.Pos
}

// servitorTargets takes the servitor's target from its walk to an object.
// Its walk to the bot is the follow (move/controller.go FollowTarget, sent
// as MoveToPawn after the summon and after every fight): no target.
func (w *World) servitorTargets(mover, target int32) {
	if s := w.Servitor; s != nil && s.ObjectID == mover {
		s.Target = target
		if target == w.Self.ObjectID {
			s.Target = 0
		}
	}
}

// servitorCombat follows the servitor's combat stance.
func (w *World) servitorCombat(id int32, on bool) {
	if s := w.Servitor; s != nil && s.ObjectID == id {
		s.InCombat = on
	}
}

// servitorDie marks the servitor dead, or drops its target when the target
// dies. The server sends no PetDelete on death, only when the corpse goes.
func (w *World) servitorDie(id int32) {
	s := w.Servitor
	switch {
	case s == nil:
	case s.ObjectID == id:
		s.Dead, s.Target, s.InCombat = true, 0, false
	case s.Target == id:
		s.Target = 0
	}
}

// servitorTargetGone drops the servitor's target when it leaves sight.
func (w *World) servitorTargetGone(id int32) {
	if s := w.Servitor; s != nil && s.Target == id {
		s.Target = 0
	}
}

// petEvent logs, once a session, a pet's window: the bot keeps servitors
// only, so a pet's packets change nothing (World.applyServitor).
func (b *Brain) petEvent(ev Event) {
	if p, ok := ev.(PetInfo); ok && p.SummonType != SummonServitor && !b.petLogged {
		b.petLogged = true
		b.log.Info("pet ignored: the bot keeps servitors only", "summonType", p.SummonType, "npc", p.TemplateID)
	}
}
