package botwire

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The summon types the pet packets carry (model/actor/summon
// live_accessors.go SummonType): a servitor, or a pet.
const (
	SummonServitor int32 = 1
	SummonPet      int32 = 2
)

// petTemplateBase is what PetInfo adds to the summon's NPC template id.
const petTemplateBase = 1000000

// PetInfo opens or refreshes the window of the bot's own summon: everything
// the brain keeps of it. The server sends it to the owner alone; other
// players see the summon as an NPCInfo.
type PetInfo struct {
	SummonType int32
	ObjectID   int32
	// TemplateID is the summon's NPC template id (the wire's less 1000000).
	TemplateID      int32
	Pos             Point
	PAtkSpd         int32
	CollisionRadius float64
	InCombat, Dead  bool
	CurHP, MaxHP    int32
	CurMP, MaxMP    int32
	Level           int32
	PAtk, PDef      int32
	MAtk, MDef      int32
	AbnormalMask    int32
}

// PetStatusUpdate refreshes the bot's own summon after its status changed.
type PetStatusUpdate struct {
	SummonType   int32
	ObjectID     int32
	Pos          Point
	CurHP, MaxHP int32
	CurMP, MaxMP int32
	Level        int32
}

// PetDelete removes the bot's own summon. The server sends it when the
// summon leaves the world (unsummoned, expired, its corpse decayed), never
// when it dies.
type PetDelete struct{ SummonType, ObjectID int32 }

// SetSummonRemainTime is a servitor's lifetime bar: its whole lifetime and
// what remains of it.
type SetSummonRemainTime struct{ Total, Remaining time.Duration }

// PetStatusShow toggles the summon's action window; the brain keeps nothing
// of it.
type PetStatusShow struct{ SummonType int32 }

func (PetInfo) IsEvent()             {}
func (PetStatusUpdate) IsEvent()     {}
func (PetDelete) IsEvent()           {}
func (SetSummonRemainTime) IsEvent() {}
func (PetStatusShow) IsEvent()       {}

// decodePetInfo follows FramePetInfo (serverpackets/petinfo.go): type,
// object id, template id + 1000000, 0; x, y, z, heading, 0, M.Atk speed,
// P.Atk speed; four run and walk speed pairs; four float64 (move and attack
// speed multipliers, collision radius and height); weapon, armor, 0; five
// bytes (1, 1, in combat, alike dead, 2); name and title; then 134 fixed
// bytes: 1, PvP flag, karma, fed and max fed, HP, max HP, MP, max MP, SP,
// level; three int64 exp values; weight and its limit; P.Atk, P.Def, M.Atk,
// M.Def, accuracy, evasion, critical, move speed, P.Atk speed, M.Atk speed;
// the abnormal mask; uint16 mountable, uint8 move type, uint16 0, uint8
// team; soulshots and spiritshots per hit.
func decodePetInfo(r *wire.Reader) Event {
	p := PetInfo{SummonType: r.ReadInt32(), ObjectID: r.ReadInt32()}
	p.TemplateID = r.ReadInt32() - petTemplateBase
	r.Skip(4) // 0
	p.Pos = Point{r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
	r.Skip(4 + 4 + 4) // heading, 0, M.Atk speed
	p.PAtkSpd = r.ReadInt32()
	r.Skip(8 * 4) // run and walk speed pairs
	r.Skip(2 * 8) // move and attack speed multipliers
	p.CollisionRadius = r.ReadFloat64()
	r.Skip(8)     // collision height
	r.Skip(3 * 4) // weapon, armor, 0
	r.Skip(2)     // 1, 1
	p.InCombat = r.ReadUint8() != 0
	p.Dead = r.ReadUint8() != 0
	r.Skip(1)      // 2
	r.ReadString() // name
	r.ReadString() // title
	r.Skip(5 * 4)  // 1, PvP flag, karma, fed, max fed
	p.CurHP, p.MaxHP = r.ReadInt32(), r.ReadInt32()
	p.CurMP, p.MaxMP = r.ReadInt32(), r.ReadInt32()
	r.Skip(4) // SP
	p.Level = r.ReadInt32()
	r.Skip(3*8 + 2*4) // exp, exp of this and the next level, weight, its limit
	p.PAtk, p.PDef = r.ReadInt32(), r.ReadInt32()
	p.MAtk, p.MDef = r.ReadInt32(), r.ReadInt32()
	r.Skip(6 * 4) // accuracy, evasion, critical, move speed, P.Atk and M.Atk speed
	p.AbnormalMask = r.ReadInt32()
	r.Skip(2 + 1 + 2 + 1 + 2*4) // mountable, move type, 0, team, shots per hit
	return p
}

// decodePetStatusUpdate follows FramePetStatusUpdate: type, object id, x,
// y, z, title, fed and max fed, HP, max HP, MP, max MP, level, then three
// int64 exp values.
func decodePetStatusUpdate(r *wire.Reader) Event {
	p := PetStatusUpdate{SummonType: r.ReadInt32(), ObjectID: r.ReadInt32()}
	p.Pos = Point{r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
	r.ReadString() // title
	r.Skip(2 * 4)  // fed, max fed
	p.CurHP, p.MaxHP = r.ReadInt32(), r.ReadInt32()
	p.CurMP, p.MaxMP = r.ReadInt32(), r.ReadInt32()
	p.Level = r.ReadInt32()
	r.Skip(3 * 8) // exp, exp of this and the next level
	return p
}

// decodeSetSummonRemainTime follows FrameSetSummonRemainTime: the total and
// the remaining lifetime, in ms.
func decodeSetSummonRemainTime(r *wire.Reader) Event {
	total := r.ReadInt32()
	return SetSummonRemainTime{
		Total:     time.Duration(total) * time.Millisecond,
		Remaining: time.Duration(r.ReadInt32()) * time.Millisecond,
	}
}
