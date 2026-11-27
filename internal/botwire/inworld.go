package botwire

// Decoders of the in-world packets the world model reads. Each follows the
// server writer it names field by field; fields the model does not keep are
// skipped by width, so DecodeEvent still sees the whole frame consumed.

import (
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// npcTemplateOffset is what NPCInfo adds to an NPC's template id.
const npcTemplateOffset = 1000000

// effectiveSpeed is a base speed times the movement multiplier, the speed
// the client moves the object at.
func effectiveSpeed(base int32, mult float64) int32 {
	return int32(float64(base) * mult)
}

// skipCubics consumes a cubic list: a uint16 count, then a uint16 id each.
func skipCubics(r *wire.Reader) {
	r.Skip(int(r.ReadUint16()) * 2)
}

// decodeUserInfoEvent decodes the full UserInfo (serverpackets/userinfo.go,
// writeUserInfo).
func decodeUserInfoEvent(r *wire.Reader) Event {
	var u UserInfo
	u.Pos = readPoint(r)
	u.Heading = r.ReadInt32()
	u.ObjectID = r.ReadInt32()
	u.Name = r.ReadString()
	u.Race, u.Sex, u.BaseClass = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	u.Level = r.ReadInt32()
	u.Exp = r.ReadInt64()
	u.STR, u.DEX, u.CON = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	u.INT, u.WIT, u.MEN = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	u.MaxHP, u.HP = r.ReadInt32(), r.ReadInt32()
	u.MaxMP, u.MP = r.ReadInt32(), r.ReadInt32()
	u.SP = r.ReadInt32()
	u.Load, u.MaxLoad = r.ReadInt32(), r.ReadInt32()
	r.Skip(4)          // bonus inventory slots
	r.Skip(2 * 17 * 4) // paperdoll object ids, then template ids
	// Augmentation block: 14 uint16, right hand, 12 uint16, left hand,
	// 4 uint16.
	r.Skip(14*2 + 4 + 12*2 + 4 + 4*2)
	u.PAtk, u.PAtkSpd = r.ReadInt32(), r.ReadInt32()
	u.PDef, u.Evasion, u.Accuracy = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	u.Critical, u.MAtk, u.CastSpd = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	r.Skip(4) // attack speed again
	u.MDef = r.ReadInt32()
	u.PvPFlag, u.Karma = r.ReadInt32(), r.ReadInt32()
	run, walk, swim := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	r.Skip(5 * 4) // swim walk, 0, 0, fly run, fly walk
	mult := r.ReadFloat64()
	u.RunSpeed, u.WalkSpeed = effectiveSpeed(run, mult), effectiveSpeed(walk, mult)
	u.SwimSpeed = effectiveSpeed(swim, mult)
	r.Skip(8) // attack speed multiplier
	u.CollisionRadius, u.CollisionHeight = r.ReadFloat64(), r.ReadFloat64()
	r.Skip(3 * 4) // hair style, hair color, face
	u.IsGM = r.ReadInt32() != 0
	r.ReadString() // title
	u.ClanID = r.ReadInt32()
	r.Skip(4) // crest
	u.AllyID = r.ReadInt32()
	r.Skip(2 * 4) // ally crest, relation
	r.Skip(1)     // mount type
	u.StoreType = r.ReadUint8()
	r.Skip(1) // can crystallize
	u.PKKills, u.PvPKills = r.ReadInt32(), r.ReadInt32()
	skipCubics(r)
	r.Skip(1) // party room
	u.Abnormal = r.ReadInt32()
	r.Skip(1 + 4)   // 0, clan privileges
	r.Skip(2*2 + 4) // recommendations left and had, mount NPC id
	u.InvLimit = int32(r.ReadUint16())
	u.ClassID = r.ReadInt32()
	r.Skip(4) // 0
	u.MaxCP, u.CP = r.ReadInt32(), r.ReadInt32()
	r.Skip(1 + 1 + 4)   // enchant effect, team, large clan crest
	r.Skip(3 + 3*4 + 4) // noble, hero, fishing, fishing bait x, y, z, name color
	u.Running = r.ReadUint8() != 0
	r.Skip(4 * 4) // pledge class, pledge type, title color, cursed weapon stage
	return u
}

// decodeStatusUpdate: object id, attribute count, then a type and a value
// each (serverpackets/targeting.go). The count comes off the wire, so the
// loop ends at the first short read; attributes the model does not keep are
// consumed and dropped.
func decodeStatusUpdate(r *wire.Reader) Event {
	s := StatusUpdate{ObjectID: r.ReadInt32()}
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		typ, v := r.ReadInt32(), r.ReadInt32()
		var f StatusFields
		switch typ {
		case 9:
			f, s.HP = StatusHP, v
		case 10:
			f, s.MaxHP = StatusMaxHP, v
		case 11:
			f, s.MP = StatusMP, v
		case 12:
			f, s.MaxMP = StatusMaxMP, v
		case 13:
			f, s.SP = StatusSP, v
		case 14:
			f, s.Load = StatusLoad, v
		case 15:
			f, s.MaxLoad = StatusMaxLoad, v
		case 27:
			f, s.Karma = StatusKarma, v
		case 33:
			f, s.CP = StatusCP, v
		case 34:
			f, s.MaxCP = StatusMaxCP, v
		}
		s.Set |= f
	}
	return s
}

// decodeCharInfo decodes CharInfo (serverpackets/charinfo.go,
// writeCharInfo).
func decodeCharInfo(r *wire.Reader) Event {
	var c CharInfo
	c.Pos = readPoint(r)
	r.Skip(4) // boat object id
	c.ObjectID = r.ReadInt32()
	c.Name = r.ReadString()
	r.Skip(3 * 4) // race, sex, base class
	// Paperdoll template ids in charInfoPaperdollOrder: the hair
	// accessory, the head, then the right hand and the left hand.
	r.Skip(2 * 4)
	c.RHand, c.LHand = r.ReadInt32(), r.ReadInt32()
	r.Skip(8 * 4)
	// Augmentation block: 4 uint16, right hand, 12 uint16, left hand,
	// 4 uint16.
	r.Skip(4*2 + 4 + 12*2 + 4 + 4*2)
	c.PvPFlag, c.Karma = r.ReadInt32(), r.ReadInt32()
	r.Skip(4 * 4) // cast speed, attack speed, PvP flag, karma
	run, walk := r.ReadInt32(), r.ReadInt32()
	r.Skip(6 * 4) // swim run, swim walk, run, walk, fly run, fly walk
	mult := r.ReadFloat64()
	c.RunSpeed, c.WalkSpeed = effectiveSpeed(run, mult), effectiveSpeed(walk, mult)
	r.Skip(8) // attack speed multiplier
	c.CollisionRadius = r.ReadFloat64()
	r.Skip(8)      // collision height
	r.Skip(3 * 4)  // hair style, hair color, face
	r.ReadString() // title
	c.ClanID = r.ReadInt32()
	r.Skip(4) // crest
	c.AllyID = r.ReadInt32()
	r.Skip(2 * 4) // ally crest, relation
	c.Sitting = r.ReadUint8() == 0
	c.Running = r.ReadUint8() != 0
	c.InCombat = r.ReadUint8() != 0
	c.Dead = r.ReadUint8() != 0
	r.Skip(2) // hidden, mount type
	c.StoreType = r.ReadUint8()
	skipCubics(r)
	r.Skip(1) // party room
	c.Abnormal = r.ReadInt32()
	r.Skip(1 + 2) // recommendations left and had
	c.ClassID = r.ReadInt32()
	c.MaxCP, c.CP = r.ReadInt32(), r.ReadInt32()
	r.Skip(1) // enchant effect
	c.DuelTeam = r.ReadUint8()
	r.Skip(4)           // large clan crest
	r.Skip(3 + 3*4 + 4) // noble, hero, fishing, fishing bait x, y, z, name color
	c.Heading = r.ReadInt32()
	r.Skip(4 * 4) // pledge class, pledge type, title color, cursed weapon stage
	return c
}

// decodeNPCInfo decodes NPCInfo (serverpackets/npcinfo.go, FrameNPCInfo).
func decodeNPCInfo(r *wire.Reader) Event {
	var n NPCInfo
	n.ObjectID = r.ReadInt32()
	n.NPCID = r.ReadInt32() - npcTemplateOffset
	n.Attackable = r.ReadInt32() != 0
	n.Pos = readPoint(r)
	n.Heading = r.ReadInt32()
	r.Skip(3 * 4) // 0, cast speed, attack speed
	run, walk := r.ReadInt32(), r.ReadInt32()
	r.Skip(6 * 4) // swim, fly and fly-mounted run and walk speeds
	mult := r.ReadFloat64()
	n.RunSpeed, n.WalkSpeed = effectiveSpeed(run, mult), effectiveSpeed(walk, mult)
	r.Skip(8) // attack speed multiplier
	n.CollisionRadius = r.ReadFloat64()
	r.Skip(8)     // collision height
	r.Skip(3 * 4) // right hand, chest, left hand
	r.Skip(1)     // name shown
	n.Running = r.ReadUint8() != 0
	n.InCombat = r.ReadUint8() != 0
	n.Dead = r.ReadUint8() != 0
	r.Skip(1) // summon animation
	n.Name, n.Title = r.ReadString(), r.ReadString()
	n.Summon = r.ReadInt32() != 0
	n.PvPFlag, n.Karma = r.ReadInt32(), r.ReadInt32()
	r.Skip(5 * 4) // abnormal effect, clan, clan crest, ally, ally crest
	r.Skip(2)     // move type, team
	r.Skip(2 * 8) // collision radius and height again
	r.Skip(2 * 4) // enchant effect, flying
	return n
}

// decodeMoveToPawn: object id, target id, distance, origin
// (serverpackets/movetopawn.go).
func decodeMoveToPawn(r *wire.Reader) Event {
	return MoveToPawn{ObjectID: r.ReadInt32(), TargetID: r.ReadInt32(), Distance: r.ReadInt32(), Origin: readPoint(r)}
}

// decodeDie: object id, then the village, clan hall, castle and siege HQ
// restart flags, sweepable, fixed restart, each an int32
// (serverpackets/die.go).
func decodeDie(r *wire.Reader) Event {
	d := Die{ObjectID: r.ReadInt32()}
	d.Restart.Village = r.ReadInt32() != 0
	d.Restart.ClanHall = r.ReadInt32() != 0
	d.Restart.Castle = r.ReadInt32() != 0
	d.Restart.SiegeHQ = r.ReadInt32() != 0
	d.Sweepable = r.ReadInt32() != 0
	d.Restart.Fixed = r.ReadInt32() != 0
	return d
}

// decodeMyTargetSelected: object id, uint16 color
// (serverpackets/targeting.go).
func decodeMyTargetSelected(r *wire.Reader) Event {
	return MyTargetSelected{ObjectID: r.ReadInt32(), Color: r.ReadUint16()}
}

// decodeTargetUnselected: object id, position (serverpackets/targeting.go).
func decodeTargetUnselected(r *wire.Reader) Event {
	return TargetUnselected{ObjectID: r.ReadInt32(), Pos: readPoint(r)}
}

// SystemMessage parameter types (serverpackets/systemmessage.go).
const (
	sysParamText      = 0
	sysParamNumber    = 1
	sysParamSkillName = 4
	sysParamZoneName  = 7
)

// decodeSystemMessage: message id, parameter count, then per parameter its
// type and value (serverpackets/systemmessage.go): text is a string, a
// skill name its id and level, a zone name a point, every other type one
// int32. The first two texts and the first number are kept. The count
// comes off the wire, so the loop ends at the first short read.
func decodeSystemMessage(r *wire.Reader) Event {
	m := SystemMessage{ID: r.ReadInt32()}
	texts, numbered := 0, false
	for n := r.ReadInt32(); n > 0 && r.Err() == nil; n-- {
		switch r.ReadInt32() {
		case sysParamText:
			if s := r.ReadString(); texts < len(m.Text) {
				m.Text[texts] = s
				texts++
			}
		case sysParamNumber:
			if v := r.ReadInt32(); !numbered {
				m.Number, numbered = v, true
			}
		case sysParamSkillName:
			r.Skip(2 * 4)
		case sysParamZoneName:
			r.Skip(3 * 4)
		default:
			r.Skip(4)
		}
	}
	return m
}

// decodeCreatureSay: object id, chat type, name, text
// (serverpackets/creaturesay.go).
func decodeCreatureSay(r *wire.Reader) Event {
	return CreatureSay{ObjectID: r.ReadInt32(), Type: r.ReadInt32(), Name: r.ReadString(), Text: r.ReadString()}
}

// decodeChangeWaitType: object id, stance, position
// (serverpackets/stance_social.go).
func decodeChangeWaitType(r *wire.Reader) Event {
	return ChangeWaitType{ObjectID: r.ReadInt32(), Type: WaitType(r.ReadInt32()), Pos: readPoint(r)}
}
