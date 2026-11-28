package botwire

import (
	"encoding/binary"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// petSnap is the server's PetInfoSnapshot (serverpackets/petinfo.go), the
// input of the golden PetInfo and PetStatusUpdate builders below.
type petSnap struct {
	summonType, objectID, templateID int32
	x, y, z, heading                 int32
	mAtkSpd, pAtkSpd                 int32
	runSpd, walkSpd                  int32
	moveMult, atkSpdMult             float64
	collisionRadius, collisionHeight float64
	inCombat, alikeDead              bool
	name, title                      string
	pvpFlag, karma, curFed, maxFed   int32
	curHP, maxHP, curMP, maxMP       int32
	sp, level                        int32
	exp, expThis, expNext            int64
	weight, weightLimit              int32
	pAtk, pDef, mAtk, mDef           int32
	accuracy, evasion, critical      int32
	moveSpeed                        int32
	abnormal                         int32
	mountable                        bool
	moveType, team                   int32
	soulShots, spiritShots           int32
	weapon, armor                    int32
}

// testServitor is a level 40 Kat the Cat with distinct values in every
// field, so a decoder reading one field off by a slot fails.
var testServitor = petSnap{
	summonType: SummonServitor, objectID: 0x10000123, templateID: 14111,
	x: 82000, y: 148000, z: -3400, heading: 1234,
	mAtkSpd: 333, pAtkSpd: 278, runSpd: 155, walkSpd: 60,
	moveMult: 1.1, atkSpdMult: 1.05, collisionRadius: 11.5, collisionHeight: 21.5,
	inCombat: true, name: "Kat the Cat", title: "Master",
	pvpFlag: 1, karma: 7, curFed: 0, maxFed: 0,
	curHP: 1200, maxHP: 1500, curMP: 300, maxMP: 450, sp: 9, level: 40,
	exp: 1 << 33, expThis: 1 << 32, expNext: 1 << 34, weight: 13, weightLimit: 14,
	pAtk: 401, pDef: 402, mAtk: 403, mDef: 404,
	accuracy: 405, evasion: 406, critical: 407, moveSpeed: 408,
	abnormal: 0x2010, mountable: true, moveType: 1, team: 2,
	soulShots: 3, spiritShots: 4,
}

// petInfoPacket writes s in FramePetInfo's field order.
func petInfoPacket(s petSnap) []byte {
	w := wire.NewPacketWriter(OpGamePetInfo)
	for _, v := range []int32{s.summonType, s.objectID, s.templateID + 1000000, 0, s.x, s.y, s.z, s.heading, 0, s.mAtkSpd, s.pAtkSpd} {
		w.WriteInt32(v)
	}
	for range 4 {
		w.WriteInt32(s.runSpd)
		w.WriteInt32(s.walkSpd)
	}
	for _, v := range []float64{s.moveMult, s.atkSpdMult, s.collisionRadius, s.collisionHeight} {
		w.WriteFloat64(v)
	}
	for _, v := range []int32{s.weapon, s.armor, 0} {
		w.WriteInt32(v)
	}
	for _, v := range []bool{true, true, s.inCombat, s.alikeDead} {
		w.WriteUint8(b2u(v))
	}
	w.WriteUint8(2)
	w.WriteString(s.name)
	w.WriteString(s.title)
	for _, v := range []int32{1, s.pvpFlag, s.karma, s.curFed, s.maxFed, s.curHP, s.maxHP, s.curMP, s.maxMP, s.sp, s.level} {
		w.WriteInt32(v)
	}
	for _, v := range []int64{s.exp, s.expThis, s.expNext} {
		w.WriteInt64(v)
	}
	for _, v := range []int32{s.weight, s.weightLimit, s.pAtk, s.pDef, s.mAtk, s.mDef, s.accuracy, s.evasion, s.critical, s.moveSpeed, s.pAtkSpd, s.mAtkSpd, s.abnormal} {
		w.WriteInt32(v)
	}
	w.WriteUint16(uint16(b2u(s.mountable)))
	w.WriteUint8(uint8(s.moveType))
	w.WriteUint16(0)
	w.WriteUint8(uint8(s.team))
	w.WriteInt32(s.soulShots)
	w.WriteInt32(s.spiritShots)
	return w.Bytes()
}

// petStatusUpdatePacket writes s in FramePetStatusUpdate's field order.
func petStatusUpdatePacket(s petSnap) []byte {
	w := wire.NewPacketWriter(OpGamePetStatusUpdate)
	for _, v := range []int32{s.summonType, s.objectID, s.x, s.y, s.z} {
		w.WriteInt32(v)
	}
	w.WriteString(s.title)
	for _, v := range []int32{s.curFed, s.maxFed, s.curHP, s.maxHP, s.curMP, s.maxMP, s.level} {
		w.WriteInt32(v)
	}
	for _, v := range []int64{s.exp, s.expThis, s.expNext} {
		w.WriteInt64(v)
	}
	return w.Bytes()
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// TestPetInfoLayout pins the golden builder to the writer's offsets
// (docs/servitors.md section 1): 125 fixed bytes after the opcode, the two
// strings, then 134 fixed bytes whose ten stats start at +76 and whose
// abnormal mask is at +116.
func TestPetInfoLayout(t *testing.T) {
	s := testServitor
	raw := petInfoPacket(s)
	strings := 2*(len(s.name)+1) + 2*(len(s.title)+1)
	tail := 1 + 125 + strings
	if got, want := len(raw), tail+134; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	at := func(off int) int32 { return int32(binary.LittleEndian.Uint32(raw[tail+off:])) }
	stats := []int32{s.pAtk, s.pDef, s.mAtk, s.mDef, s.accuracy, s.evasion, s.critical, s.moveSpeed, s.pAtkSpd, s.mAtkSpd}
	for i, want := range stats {
		if got := at(76 + 4*i); got != want {
			t.Errorf("stat %d at +%d = %d, want %d", i, 76+4*i, got, want)
		}
	}
	if got := at(116); got != s.abnormal {
		t.Errorf("abnormal mask at +116 = %#x, want %#x", got, s.abnormal)
	}
	if got := at(20); got != s.curHP {
		t.Errorf("HP at +20 = %d, want %d", got, s.curHP)
	}
	if got := raw[1+122]; got != 1 {
		t.Errorf("in combat byte = %d, want 1", got)
	}
}

// TestPetDecoders decodes the five pet packets in the server writers'
// field order (serverpackets/petinfo.go, serverpackets/pet.go); each
// decoder consumes exactly len(payload), and a frame one byte short or one
// byte long is an error.
func TestPetDecoders(t *testing.T) {
	dead := testServitor
	dead.inCombat, dead.alikeDead, dead.name, dead.title = false, true, "", ""
	pet := testServitor
	pet.summonType = SummonPet
	cases := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"PetInfo", petInfoPacket(testServitor), PetInfo{
			SummonType: SummonServitor, ObjectID: 0x10000123, TemplateID: 14111,
			Pos: Point{82000, 148000, -3400}, PAtkSpd: 278, CollisionRadius: 11.5, InCombat: true,
			CurHP: 1200, MaxHP: 1500, CurMP: 300, MaxMP: 450, Level: 40,
			PAtk: 401, PDef: 402, MAtk: 403, MDef: 404, AbnormalMask: 0x2010,
		}},
		{"PetInfo of a dead servitor, no names", petInfoPacket(dead), PetInfo{
			SummonType: SummonServitor, ObjectID: 0x10000123, TemplateID: 14111,
			Pos: Point{82000, 148000, -3400}, PAtkSpd: 278, CollisionRadius: 11.5, Dead: true,
			CurHP: 1200, MaxHP: 1500, CurMP: 300, MaxMP: 450, Level: 40,
			PAtk: 401, PDef: 402, MAtk: 403, MDef: 404, AbnormalMask: 0x2010,
		}},
		{"PetInfo of a pet", petInfoPacket(pet), PetInfo{
			SummonType: SummonPet, ObjectID: 0x10000123, TemplateID: 14111,
			Pos: Point{82000, 148000, -3400}, PAtkSpd: 278, CollisionRadius: 11.5, InCombat: true,
			CurHP: 1200, MaxHP: 1500, CurMP: 300, MaxMP: 450, Level: 40,
			PAtk: 401, PDef: 402, MAtk: 403, MDef: 404, AbnormalMask: 0x2010,
		}},
		{"PetStatusUpdate", petStatusUpdatePacket(testServitor), PetStatusUpdate{
			SummonType: SummonServitor, ObjectID: 0x10000123, Pos: Point{82000, 148000, -3400},
			CurHP: 1200, MaxHP: 1500, CurMP: 300, MaxMP: 450, Level: 40,
		}},
		{"PetDelete", []byte{
			0xb6,
			0x01, 0x00, 0x00, 0x00, // servitor
			0x23, 0x01, 0x00, 0x10, // object id 0x10000123
		}, PetDelete{SummonType: SummonServitor, ObjectID: 0x10000123}},
		{"SetSummonRemainTime", []byte{
			0xd1,
			0xc0, 0x27, 0x09, 0x00, // total 600000 ms
			0x50, 0x10, 0x09, 0x00, // remaining 594000 ms
		}, SetSummonRemainTime{Total: 10 * time.Minute, Remaining: 594 * time.Second}},
		{"PetStatusShow", []byte{
			0xb0,
			0x01, 0x00, 0x00, 0x00, // servitor
		}, PetStatusShow{SummonType: SummonServitor}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok, err := DecodeEvent(tc.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v", ok, err)
			}
			if !reflect.DeepEqual(ev, tc.want) {
				t.Errorf("got %+v\nwant %+v", ev, tc.want)
			}
			if _, _, err := DecodeEvent(tc.raw[:len(tc.raw)-1]); err == nil {
				t.Error("a frame one byte short decoded")
			}
			if _, _, err := DecodeEvent(append(tc.raw[:len(tc.raw):len(tc.raw)], 0)); err == nil {
				t.Error("a frame one byte long decoded")
			}
		})
	}
}
