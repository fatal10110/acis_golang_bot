package botclient

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// The builders below write each packet field by field in the server's
// layout (serverpackets/<file> per builder). Fields the decoders skip carry
// distinct nonzero filler, so a decoder that skips the wrong width reads
// filler into a kept field or ends off the frame's end.

// userInfoPacket writes UserInfo for u; run and walk are the base speeds
// and mult the movement multiplier (userinfo.go, writeUserInfo).
func userInfoPacket(u UserInfo, run, walk int32, mult float64, cubics int) []byte {
	w := wire.NewPacketWriter(opGameUserInfo)
	w.WriteInt32(u.Pos.X)
	w.WriteInt32(u.Pos.Y)
	w.WriteInt32(u.Pos.Z)
	w.WriteInt32(u.Heading)
	w.WriteInt32(u.ObjectID)
	w.WriteString(u.Name)
	for _, v := range []int32{u.Race, u.Sex, u.BaseClass, u.Level} {
		w.WriteInt32(v)
	}
	w.WriteInt64(u.Exp)
	for i := range int32(6) { // STR..MEN
		w.WriteInt32(40 + i)
	}
	for _, v := range []int32{u.MaxHP, u.HP, u.MaxMP, u.MP, u.SP, u.Load, u.MaxLoad, 40} {
		w.WriteInt32(v)
	}
	for i := range int32(2 * 17) { // paperdoll object ids, template ids
		w.WriteInt32(5000 + i)
	}
	fillUint16(w, 14)
	w.WriteInt32(7777)
	fillUint16(w, 12)
	w.WriteInt32(8888)
	fillUint16(w, 4)
	for i := range int32(10) { // pAtk..mDef
		w.WriteInt32(300 + i)
	}
	w.WriteInt32(u.PvPFlag)
	w.WriteInt32(u.Karma)
	for _, v := range []int32{run, walk, 50, 50, 0, 0, 0, 0} {
		w.WriteInt32(v)
	}
	w.WriteFloat64(mult)
	w.WriteFloat64(1.1)
	w.WriteFloat64(9)
	w.WriteFloat64(23.5)
	for _, v := range []int32{1, 2, 3, wire.BoolInt32(u.IsGM)} {
		w.WriteInt32(v)
	}
	w.WriteString("a title")
	for i := range int32(5) { // clan, crest, ally, ally crest, relation
		w.WriteInt32(600 + i)
	}
	w.WriteUint8(2) // mount type
	w.WriteUint8(u.StoreType)
	w.WriteUint8(1)
	w.WriteInt32(11) // PK kills
	w.WriteInt32(12) // PvP kills
	writeCubics(w, cubics)
	w.WriteUint8(1)
	w.WriteInt32(0x2000)
	w.WriteUint8(0)
	w.WriteInt32(0x7f)
	w.WriteUint16(3)
	w.WriteUint16(4)
	w.WriteInt32(1012526)
	w.WriteUint16(80)
	w.WriteInt32(u.ClassID)
	w.WriteInt32(0)
	w.WriteInt32(u.MaxCP)
	w.WriteInt32(u.CP)
	w.WriteUint8(16)
	w.WriteUint8(1)
	w.WriteInt32(900)
	w.WriteUint8(1)
	w.WriteUint8(1)
	w.WriteUint8(1)
	for _, v := range []int32{100, 200, 300, 0xffffff} { // bait x, y, z, name color
		w.WriteInt32(v)
	}
	w.WriteUint8(wire.BoolByte(u.Running))
	for i := range int32(4) { // pledge class, pledge type, title color, cursed stage
		w.WriteInt32(700 + i)
	}
	return w.Bytes()
}

// charInfoPacket writes CharInfo for c (charinfo.go, writeCharInfo).
func charInfoPacket(c CharInfo, run, walk int32, mult float64, cubics int) []byte {
	w := wire.NewPacketWriter(opGameCharInfo)
	w.WriteInt32(c.Pos.X)
	w.WriteInt32(c.Pos.Y)
	w.WriteInt32(c.Pos.Z)
	w.WriteInt32(55) // boat
	w.WriteInt32(c.ObjectID)
	w.WriteString(c.Name)
	for i := range int32(3 + 12) { // race, sex, base class, paperdoll template ids
		w.WriteInt32(2000 + i)
	}
	fillUint16(w, 4)
	w.WriteInt32(7777)
	fillUint16(w, 12)
	w.WriteInt32(8888)
	fillUint16(w, 4)
	w.WriteInt32(c.PvPFlag)
	w.WriteInt32(c.Karma)
	for _, v := range []int32{333, 444, c.PvPFlag, c.Karma, run, walk, 50, 50, run, walk, 0, 0} {
		w.WriteInt32(v)
	}
	w.WriteFloat64(mult)
	w.WriteFloat64(1.1)
	w.WriteFloat64(9)
	w.WriteFloat64(23.5)
	for _, v := range []int32{1, 2, 3} {
		w.WriteInt32(v)
	}
	w.WriteString("a title")
	for i := range int32(4) {
		w.WriteInt32(600 + i)
	}
	w.WriteInt32(0) // relation
	w.WriteUint8(wire.BoolByte(!c.Sitting))
	w.WriteUint8(wire.BoolByte(c.Running))
	w.WriteUint8(wire.BoolByte(c.InCombat))
	w.WriteUint8(wire.BoolByte(c.Dead))
	w.WriteUint8(1) // hidden
	w.WriteUint8(2) // mount type
	w.WriteUint8(c.StoreType)
	writeCubics(w, cubics)
	w.WriteUint8(1)
	w.WriteInt32(0x2000)
	w.WriteUint8(3)
	w.WriteUint16(4)
	w.WriteInt32(c.ClassID)
	w.WriteInt32(1200)
	w.WriteInt32(1100)
	w.WriteUint8(16)
	w.WriteUint8(1)
	w.WriteInt32(900)
	w.WriteUint8(1)
	w.WriteUint8(1)
	w.WriteUint8(1)
	for _, v := range []int32{100, 200, 300, 0xffffff, c.Heading} {
		w.WriteInt32(v)
	}
	for i := range int32(4) {
		w.WriteInt32(700 + i)
	}
	return w.Bytes()
}

// npcInfoPacket writes NPCInfo for n (npcinfo.go, FrameNPCInfo).
func npcInfoPacket(n NPCInfo, run, walk int32, mult float64) []byte {
	w := wire.NewPacketWriter(opGameNPCInfo)
	w.WriteInt32(n.ObjectID)
	w.WriteInt32(n.NPCID + npcTemplateOffset)
	w.WriteInt32(wire.BoolInt32(n.Attackable))
	for _, v := range []int32{n.Pos.X, n.Pos.Y, n.Pos.Z, n.Heading, 0, 333, 444} {
		w.WriteInt32(v)
	}
	for range 4 {
		w.WriteInt32(run)
		w.WriteInt32(walk)
	}
	w.WriteFloat64(mult)
	w.WriteFloat64(1.1)
	w.WriteFloat64(9)
	w.WriteFloat64(23.5)
	for _, v := range []int32{2369, 0, 626} { // right hand, chest, left hand
		w.WriteInt32(v)
	}
	w.WriteUint8(1)
	w.WriteUint8(wire.BoolByte(n.Running))
	w.WriteUint8(wire.BoolByte(n.InCombat))
	w.WriteUint8(wire.BoolByte(n.Dead))
	w.WriteUint8(2) // summon animation
	w.WriteString("Gremlin")
	w.WriteString("a title")
	w.WriteInt32(wire.BoolInt32(n.Summon))
	w.WriteInt32(n.PvPFlag)
	w.WriteInt32(n.Karma)
	for i := range int32(5) {
		w.WriteInt32(600 + i)
	}
	w.WriteUint8(1)
	w.WriteUint8(2)
	w.WriteFloat64(9)
	w.WriteFloat64(23.5)
	w.WriteInt32(3)
	w.WriteInt32(1)
	return w.Bytes()
}

func fillUint16(w *wire.Writer, n int) {
	for i := range n {
		w.WriteUint16(uint16(0x100 + i))
	}
}

func writeCubics(w *wire.Writer, n int) {
	w.WriteUint16(uint16(n))
	for i := range n {
		w.WriteUint16(uint16(1 + i))
	}
}

var (
	testUserInfo = UserInfo{
		ObjectID: 0x10000001, Name: "bot7", Pos: Point{X: -71000, Y: 258000, Z: -3100}, Heading: 49152,
		Race: 1, Sex: 1, BaseClass: 25, ClassID: 26, Level: 20, Exp: 1 << 33, SP: 5000,
		HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, CP: 90, MaxCP: 200, Load: 1500, MaxLoad: 80000,
		PvPFlag: 1, Karma: 7, RunSpeed: 156, WalkSpeed: 104, Running: true, StoreType: 3, IsGM: true,
	}
	testCharInfo = CharInfo{
		ObjectID: 0x10000002, Name: "other", Pos: Point{X: 1, Y: 2, Z: 3}, Heading: 1000, ClassID: 10,
		PvPFlag: 1, Karma: 50, RunSpeed: 156, WalkSpeed: 104, Sitting: true, InCombat: true, StoreType: 1,
	}
	testNPCInfo = NPCInfo{
		ObjectID: 0x10000003, NPCID: 20001, Attackable: true, Pos: Point{X: 4, Y: 5, Z: 6}, Heading: 2000,
		RunSpeed: 195, WalkSpeed: 60, Running: true, Dead: true, Summon: true, PvPFlag: 2, Karma: 3,
	}
)

// inworldCases are well-formed frames of every in-world decoder.
func inworldCases() []struct {
	name string
	raw  []byte
	want Event
} {
	at := Point{X: 81000, Y: 148000, Z: -3400}
	return []struct {
		name string
		raw  []byte
		want Event
	}{
		{"UserInfo", userInfoPacket(testUserInfo, 120, 80, 1.3, 0), testUserInfo},
		{"UserInfo cubics", userInfoPacket(testUserInfo, 120, 80, 1.3, 3), testUserInfo},
		{"CharInfo", charInfoPacket(testCharInfo, 120, 80, 1.3, 2), testCharInfo},
		{"CharInfo standing", charInfoPacket(CharInfo{ObjectID: 9, Running: true, Dead: true}, 0, 0, 1, 0), CharInfo{ObjectID: 9, Running: true, Dead: true}},
		{"NPCInfo", npcInfoPacket(testNPCInfo, 130, 40, 1.5), testNPCInfo},
		{
			"StatusUpdate", packet(opGameStatusUpdate, 7, 12, 9, 300, 10, 410, 11, 120, 12, 150, 13, 5000, 14, 1500, 15, 80000, 18, 333, 24, 444, 27, 7, 33, 90, 34, 200),
			StatusUpdate{ObjectID: 7, Set: StatusHP | StatusMaxHP | StatusMP | StatusMaxMP | StatusSP | StatusLoad | StatusMaxLoad | StatusKarma | StatusCP | StatusMaxCP, HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, SP: 5000, Load: 1500, MaxLoad: 80000, Karma: 7, CP: 90, MaxCP: 200},
		},
		{"StatusUpdate unknown type only", packet(opGameStatusUpdate, 7, 1, 18, 333), StatusUpdate{ObjectID: 7}},
		{"StatusUpdate empty", packet(opGameStatusUpdate, 7, 0), StatusUpdate{ObjectID: 7}},
		{"MoveToPawn", packet(opGameMoveToPawn, 7, 8, 40, at.X, at.Y, at.Z), MoveToPawn{ObjectID: 7, TargetID: 8, Distance: 40, Origin: at}},
		{"Die", packet(opGameDie, 7, 1, 0, 1, 0, 1, 1), Die{ObjectID: 7, Restart: RestartPoints{Village: true, Castle: true, Fixed: true}, Sweepable: true}},
		{"Die village only", packet(opGameDie, 7, 1, 0, 0, 0, 0, 0), Die{ObjectID: 7, Restart: RestartPoints{Village: true}}},
		{"Revive", packet(opGameRevive, 7), Revive{ObjectID: 7}},
		{"MyTargetSelected", packet(opGameMyTargetSelected, 7, uint16(0xfffb)), MyTargetSelected{ObjectID: 7, Color: 0xfffb}},
		{"TargetUnselected", packet(opGameTargetUnselected, 7, at.X, at.Y, at.Z), TargetUnselected{ObjectID: 7, Pos: at}},
		{"SystemMessage no params", packet(opGameSystemMessage, 212, 0), SystemMessage{ID: 212}},
		{
			"SystemMessage every param type",
			packet(opGameSystemMessage, 1147, 6, 0, "text", 1, 5, 3, 57, 4, 1216, 2, 6, 100, 7, at.X, at.Y, at.Z),
			SystemMessage{ID: 1147},
		},
		{"SystemMessage other type is one int32", packet(opGameSystemMessage, 9, 2, 2, 20001, 5, 3), SystemMessage{ID: 9}},
		{"ActionFailed", packet(opGameActionFailed), ActionFailed{}},
		{"CreatureSay", packet(opGameCreatureSay, 7, 1, "bot7", "hello"), CreatureSay{ObjectID: 7, Type: 1, Name: "bot7", Text: "hello"}},
		{"ChangeWaitType", packet(opGameChangeWaitType, 7, 0, at.X, at.Y, at.Z), ChangeWaitType{ObjectID: 7, Type: WaitSitting, Pos: at}},
		{"AutoAttackStart", packet(opGameAutoAttackStart, 7), AutoAttackStart{ObjectID: 7}},
		{"AutoAttackStop", packet(opGameAutoAttackStop, 7), AutoAttackStop{ObjectID: 7}},
	}
}

func TestDecodeInWorldEvent(t *testing.T) {
	for _, tt := range inworldCases() {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("decodeEvent = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// Every in-world frame one byte short fails, and one byte long fails with
// errTrailingBytes: each decoder consumes exactly its layout.
func TestDecodeInWorldEventRejects(t *testing.T) {
	for _, tt := range inworldCases() {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.raw) > 1 {
				if _, _, err := decodeEvent(tt.raw[:len(tt.raw)-1]); err == nil {
					t.Fatal("short frame decoded without error")
				}
			}
			long := append(append([]byte(nil), tt.raw...), 0)
			if _, _, err := decodeEvent(long); !errors.Is(err, errTrailingBytes) {
				t.Fatalf("long frame error = %v, want %v", err, errTrailingBytes)
			}
		})
	}
}

// A count off the wire larger than the frame holds fails short instead of
// looping or allocating per count.
func TestDecodeInWorldEventCounts(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"StatusUpdate", packet(opGameStatusUpdate, 7, 0x7fffffff, 9, 1)},
		{"SystemMessage", packet(opGameSystemMessage, 7, 0x7fffffff, 1, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, wire.ErrShortPacket) {
				t.Fatalf("decodeEvent error = %v, want %v", err, wire.ErrShortPacket)
			}
		})
	}
}
