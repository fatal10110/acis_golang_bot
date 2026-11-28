package botwire

// Combat events and their decoders. Each decoder follows the server writer
// it names field by field (serverpackets under
// internal/gameserver/network), so DecodeEvent sees the whole frame
// consumed. A count read off the wire bounds a loop that ends at the first
// short read, and never sizes an allocation past what the frame can hold.

import (
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Attack hit flags (model/actor/attack/controller.go).
const (
	HitSoulshot uint8 = 0x10
	HitCritical uint8 = 0x20
	HitShield   uint8 = 0x40
	HitMiss     uint8 = 0x80
)

// AttackHit is one target of a swing.
type AttackHit struct {
	TargetID, Damage int32
	Flags            uint8
}

// Missed reports whether the target evaded the hit.
func (h AttackHit) Missed() bool { return h.Flags&HitMiss != 0 }

// Attack reports a physical attack: AttackerID, standing at Pos, swings at
// one target or, with a pole, at several.
type Attack struct {
	AttackerID int32
	Pos        Point
	Hits       []AttackHit
}

// MagicSkillUse reports a cast starting: CasterID casts SkillID at TargetID
// for HitTime, and the skill is reused Reuse after the cast starts.
type MagicSkillUse struct {
	CasterID, TargetID   int32
	SkillID, Level       int32
	HitTime, Reuse       time.Duration
	CasterPos, TargetPos Point
}

// MagicSkillLaunched reports a cast landing on Targets.
type MagicSkillLaunched struct {
	CasterID       int32
	SkillID, Level int32
	Targets        []int32
}

// KnownSkill is one skill of the character's skill window.
type KnownSkill struct {
	ID, Level         int32
	Passive, Disabled bool
}

// SkillList is the character's whole skill set, sent on world entry and on
// every change.
type SkillList struct{ Skills []KnownSkill }

// SkillTimer is one skill whose reuse is still running.
type SkillTimer struct {
	SkillID, Level   int32
	Reuse, Remaining time.Duration
}

// SkillCoolTime lists the character's running skill reuse timers.
type SkillCoolTime struct{ Timers []SkillTimer }

// RelationChanged reports another player's PvP flag or karma changing. It
// is the only packet that tells observers about it: the server resends
// UserInfo to the player alone (player/character_pvpflag.go).
type RelationChanged struct {
	ObjectID int32
	// Relation is the player's relation bits as the bot sees them
	// (serverpackets/relationchanged.go, network/relations.go relationTo):
	// RelationMutualWar while both clans declared war on each other, the
	// siege bits while the player takes part in a siege.
	Relation int32
	// AutoAttackable is set when the bot may attack the player without
	// force (Character.AttackableWithoutForceBy): flagged, with karma, in a
	// PvP zone or arena with the bot, or in its duel.
	AutoAttackable bool
	Karma          int32
	PvPFlag        uint8
}

// Relation bits of RelationChanged (serverpackets/relationchanged.go). The
// server sets RelationOneSidedWar with RelationMutualWar at mutual war
// (network/relations.go relationTo), so a mutual war reads 0x18000.
const (
	RelationMutualWar   int32 = 0x8000
	RelationOneSidedWar int32 = 0x10000
)

// The siege bits of RelationChanged (serverpackets/relationchanged.go,
// network/relations.go relationTo): a player with a siege state shows
// RelationInSiege, RelationAttacker on the attacking side, and
// RelationAlly to an observer of the same siege state or RelationEnemy to
// any other, a non-participant included (docs/pvp/siege.md). The server
// resends them only when the player's own state changes, not the
// observer's, so a side read from RelationAttacker stays right where
// RelationAlly and RelationEnemy may lag.
const (
	RelationInSiege  int32 = 0x200
	RelationAttacker int32 = 0x400
	RelationAlly     int32 = 0x800
	RelationEnemy    int32 = 0x1000
)

// MutualWar reports whether the player's clan and the bot's are at war
// with each other, neither of them in an academy: the war a kill is a PvP
// kill in (docs/pvp/war.md).
func (e RelationChanged) MutualWar() bool { return e.Relation&RelationMutualWar != 0 }

func (Attack) IsEvent() {}

func (MagicSkillUse) IsEvent() {}

func (MagicSkillLaunched) IsEvent() {}

func (SkillList) IsEvent() {}

func (SkillCoolTime) IsEvent() {}

func (RelationChanged) IsEvent() {}

// Wire sizes of the repeated entries, to bound allocations.
const (
	attackHitSize  = 4 + 4 + 1
	knownSkillSize = 4 + 4 + 4 + 1
	skillTimerSize = 4 * 4
)

// capFor is the capacity for n entries of size bytes each that r can still
// hold.
func capFor(r *wire.Reader, n, size int) int {
	return max(min(n, r.Remaining()/size), 0)
}

func readAttackHit(r *wire.Reader) AttackHit {
	return AttackHit{TargetID: r.ReadInt32(), Damage: r.ReadInt32(), Flags: r.ReadUint8()}
}

// decodeAttack: attacker id, the first hit (target, damage, flags), the
// attacker's position, a uint16 count of further hits, then each of them
// (serverpackets/attack.go).
func decodeAttack(r *wire.Reader) Event {
	a := Attack{AttackerID: r.ReadInt32()}
	first := readAttackHit(r)
	a.Pos = readPoint(r)
	n := int(r.ReadUint16())
	a.Hits = make([]AttackHit, 1, 1+capFor(r, n, attackHitSize))
	a.Hits[0] = first
	for ; n > 0 && r.Err() == nil; n-- {
		a.Hits = append(a.Hits, readAttackHit(r))
	}
	return a
}

// decodeMagicSkillUse: caster id, target id, skill id, level, hit time and
// reuse delay in ms, caster position, an int32 flag followed by a uint16
// when set, then the target position (serverpackets/magic_skill.go).
func decodeMagicSkillUse(r *wire.Reader) Event {
	m := MagicSkillUse{CasterID: r.ReadInt32(), TargetID: r.ReadInt32(), SkillID: r.ReadInt32(), Level: r.ReadInt32()}
	m.HitTime = time.Duration(r.ReadInt32()) * time.Millisecond
	m.Reuse = time.Duration(r.ReadInt32()) * time.Millisecond
	m.CasterPos = readPoint(r)
	if r.ReadInt32() != 0 {
		r.Skip(2)
	}
	m.TargetPos = readPoint(r)
	return m
}

// decodeMagicSkillLaunched: caster id, skill id, level, target count, then a
// target id each; no target is written as two zero int32s
// (serverpackets/magic_skill.go).
func decodeMagicSkillLaunched(r *wire.Reader) Event {
	m := MagicSkillLaunched{CasterID: r.ReadInt32(), SkillID: r.ReadInt32(), Level: r.ReadInt32()}
	n := int(r.ReadInt32())
	if n == 0 {
		r.Skip(4)
		return m
	}
	m.Targets = make([]int32, 0, capFor(r, n, 4))
	for ; n > 0 && r.Err() == nil; n-- {
		m.Targets = append(m.Targets, r.ReadInt32())
	}
	return m
}

// decodeSkillList: count, then per skill passive (int32), level, id and
// disabled (uint8) (serverpackets/skilllist.go).
func decodeSkillList(r *wire.Reader) Event {
	n := int(r.ReadInt32())
	l := SkillList{Skills: make([]KnownSkill, 0, capFor(r, n, knownSkillSize))}
	for ; n > 0 && r.Err() == nil; n-- {
		s := KnownSkill{Passive: r.ReadInt32() != 0, Level: r.ReadInt32(), ID: r.ReadInt32()}
		s.Disabled = r.ReadUint8() != 0
		l.Skills = append(l.Skills, s)
	}
	return l
}

// decodeSkillCoolTime: count, then per timer skill id, level, reuse and
// remaining time in seconds (serverpackets/skillcooltime.go).
func decodeSkillCoolTime(r *wire.Reader) Event {
	n := int(r.ReadInt32())
	c := SkillCoolTime{Timers: make([]SkillTimer, 0, capFor(r, n, skillTimerSize))}
	for ; n > 0 && r.Err() == nil; n-- {
		t := SkillTimer{SkillID: r.ReadInt32(), Level: r.ReadInt32()}
		t.Reuse = time.Duration(r.ReadInt32()) * time.Second
		t.Remaining = time.Duration(r.ReadInt32()) * time.Second
		c.Timers = append(c.Timers, t)
	}
	return c
}

// decodeRelationChanged: object id, relation bits, auto-attackable flag,
// karma, PvP flag (serverpackets/relationchanged.go).
func decodeRelationChanged(r *wire.Reader) Event {
	e := RelationChanged{ObjectID: r.ReadInt32(), Relation: r.ReadInt32()}
	e.AutoAttackable = r.ReadInt32() != 0
	e.Karma = r.ReadInt32()
	e.PvPFlag = uint8(r.ReadInt32())
	return e
}
