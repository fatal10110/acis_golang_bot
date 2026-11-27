package botwire

import "github.com/fatal10110/acis_golang_bot/internal/commons/wire"

// AcquirableSkill is one skill level a trainer offers: the next level of a
// skill the bot may learn at its level, and its SP cost.
type AcquirableSkill struct{ ID, Level, Cost int32 }

// AcquireSkillList is a trainer's list of skills the bot may learn now, of
// list type Type (0 the class trainer's).
type AcquireSkillList struct {
	Type   int32
	Skills []AcquirableSkill
}

// SkillRequirement is an item learning a skill level takes: Type 99 marks
// a spellbook.
type SkillRequirement struct{ Type, ItemID, Count int32 }

// AcquireSkillInfo answers RequestAcquireSkillInfo: the SP cost of one
// offered skill level, corrected for the bot, and the items it takes.
type AcquireSkillInfo struct {
	ID, Level, Cost, Type int32
	Requirements          []SkillRequirement
}

// AcquireSkillDone closes the learn window: the trainer has nothing left to
// teach the bot at its level.
type AcquireSkillDone struct{}

func (AcquireSkillList) IsEvent() {}

func (AcquireSkillInfo) IsEvent() {}

func (AcquireSkillDone) IsEvent() {}

// Wire sizes of a list entry (five int32s) and a requirement (four).
const (
	acquirableSkillSize  = 5 * 4
	skillRequirementSize = 4 * 4
)

// decodeAcquireSkillList: list type, count, then per skill id, level
// written twice, SP cost and a zero (serverpackets/acquireskill.go).
func decodeAcquireSkillList(r *wire.Reader) Event {
	l := AcquireSkillList{Type: r.ReadInt32()}
	n := int(r.ReadInt32())
	l.Skills = make([]AcquirableSkill, 0, capFor(r, n, acquirableSkillSize))
	for ; n > 0 && r.Err() == nil; n-- {
		s := AcquirableSkill{ID: r.ReadInt32(), Level: r.ReadInt32()}
		r.Skip(4) // the level again
		s.Cost = r.ReadInt32()
		r.Skip(4)
		l.Skills = append(l.Skills, s)
	}
	return l
}

// decodeAcquireSkillInfo: skill id, level, SP cost, list type, count, then
// per requirement type, item id, count and a trailing value
// (serverpackets/acquireskill.go).
func decodeAcquireSkillInfo(r *wire.Reader) Event {
	i := AcquireSkillInfo{ID: r.ReadInt32(), Level: r.ReadInt32(), Cost: r.ReadInt32(), Type: r.ReadInt32()}
	n := int(r.ReadInt32())
	if n == 0 {
		return i
	}
	i.Requirements = make([]SkillRequirement, 0, capFor(r, n, skillRequirementSize))
	for ; n > 0 && r.Err() == nil; n-- {
		i.Requirements = append(i.Requirements, SkillRequirement{Type: r.ReadInt32(), ItemID: r.ReadInt32(), Count: r.ReadInt32()})
		r.Skip(4)
	}
	return i
}

// EncodeSkillRequest is RequestAcquireSkillInfo or RequestAcquireSkill
// (opcode op): skill id, level, list type (clientpackets/acquire_skill.go).
func EncodeSkillRequest(op byte, id, level, typ int32) []byte {
	w := wire.NewPacketWriter(op)
	w.WriteInt32(id)
	w.WriteInt32(level)
	w.WriteInt32(typ)
	return w.Bytes()
}
