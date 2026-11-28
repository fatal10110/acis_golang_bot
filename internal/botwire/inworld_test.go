package botwire

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// userInfoPacket writes UserInfo for u; run and walk are the base speeds
// and mult the movement multiplier (userinfo.go, writeUserInfo).
func userInfoPacket(u UserInfo, run, walk int32, mult float64, cubics int) []byte {
	w := wire.NewPacketWriter(OpGameUserInfo)
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
	for _, v := range []int32{u.STR, u.DEX, u.CON, u.INT, u.WIT, u.MEN} {
		w.WriteInt32(v)
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
	// The attack speed is written twice (userinfo.go).
	for _, v := range []int32{
		u.PAtk, u.PAtkSpd, u.PDef, u.Evasion, u.Accuracy, u.Critical, u.MAtk, u.CastSpd, u.PAtkSpd, u.MDef,
	} {
		w.WriteInt32(v)
	}
	w.WriteInt32(u.PvPFlag)
	w.WriteInt32(u.Karma)
	for _, v := range []int32{run, walk, 50, 50, 0, 0, 0, 0} {
		w.WriteInt32(v)
	}
	w.WriteFloat64(mult)
	w.WriteFloat64(1.1)
	w.WriteFloat64(u.CollisionRadius)
	w.WriteFloat64(23.5)
	for _, v := range []int32{1, 2, 3, wire.BoolInt32(u.IsGM)} {
		w.WriteInt32(v)
	}
	w.WriteString("a title")
	w.WriteInt32(u.ClanID)
	for _, v := range []int32{601, u.AllyID, 603, 604} { // crest, ally, ally crest, relation
		w.WriteInt32(v)
	}
	w.WriteUint8(2) // mount type
	w.WriteUint8(u.StoreType)
	w.WriteUint8(1)
	w.WriteInt32(u.PKKills)
	w.WriteInt32(u.PvPKills)
	writeCubics(w, cubics)
	w.WriteUint8(1)
	w.WriteInt32(u.Abnormal)
	w.WriteUint8(0)
	w.WriteInt32(0x7f)
	w.WriteUint16(3)
	w.WriteUint16(4)
	w.WriteInt32(1012526)
	w.WriteUint16(uint16(u.InvLimit))
	w.WriteInt32(u.ClassID)
	w.WriteInt32(0)
	w.WriteInt32(u.MaxCP)
	w.WriteInt32(u.CP)
	w.WriteUint8(16)
	w.WriteUint8(1)
	w.WriteInt32(900)
	w.WriteUint8(wire.BoolByte(u.Noble))
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
	w := wire.NewPacketWriter(OpGameCharInfo)
	w.WriteInt32(c.Pos.X)
	w.WriteInt32(c.Pos.Y)
	w.WriteInt32(c.Pos.Z)
	w.WriteInt32(55) // boat
	w.WriteInt32(c.ObjectID)
	w.WriteString(c.Name)
	// Race, sex, base class, then the paperdoll template ids in
	// charInfoPaperdollOrder: the hands are slots 2 and 3, slot 9 the right
	// hand again.
	for _, v := range []int32{2000, 2001, 2002, 2003, 2004, c.RHand, c.LHand, 2007, 2008, 2009, 2010, 2011, c.RHand, 2013, 2014} {
		w.WriteInt32(v)
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
	w.WriteFloat64(c.CollisionRadius)
	w.WriteFloat64(23.5)
	for _, v := range []int32{1, 2, 3} {
		w.WriteInt32(v)
	}
	w.WriteString("a title")
	for _, v := range []int32{c.ClanID, 601, c.AllyID, 603} { // clan, crest, ally, ally crest
		w.WriteInt32(v)
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
	w.WriteInt32(c.Abnormal)
	w.WriteUint8(3)
	w.WriteUint16(4)
	w.WriteInt32(c.ClassID)
	w.WriteInt32(c.MaxCP)
	w.WriteInt32(c.CP)
	w.WriteUint8(16)
	w.WriteUint8(c.DuelTeam)
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
	w := wire.NewPacketWriter(OpGameNPCInfo)
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
	w.WriteFloat64(n.CollisionRadius)
	w.WriteFloat64(23.5)
	for _, v := range []int32{2369, 0, 626} { // right hand, chest, left hand
		w.WriteInt32(v)
	}
	w.WriteUint8(1)
	w.WriteUint8(wire.BoolByte(n.Running))
	w.WriteUint8(wire.BoolByte(n.InCombat))
	w.WriteUint8(wire.BoolByte(n.Dead))
	w.WriteUint8(2) // summon animation
	w.WriteString(n.Name)
	w.WriteString(n.Title)
	w.WriteInt32(wire.BoolInt32(n.Summon))
	w.WriteInt32(n.PvPFlag)
	w.WriteInt32(n.Karma)
	for i := range int32(5) {
		w.WriteInt32(600 + i)
	}
	w.WriteUint8(1)
	w.WriteUint8(2)
	w.WriteFloat64(n.CollisionRadius)
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
		HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, CP: 90, MaxCP: 200, Load: 1500, MaxLoad: 80000, InvLimit: 100,
		PvPFlag: 1, Karma: 7, RunSpeed: 156, WalkSpeed: 104, SwimSpeed: 65, Running: true, Noble: true, PAtk: 300, PAtkSpd: 301, PDef: 302, Evasion: 303, Accuracy: 304, CollisionHeight: 23.5, StoreType: 3, IsGM: true,
		Abnormal: AbnormalStealth | 0x2000, ClanID: 0x0a000001, AllyID: 0x0a000002,
		CollisionRadius: 9, PKKills: 11, PvPKills: 12,
		STR: 40, DEX: 41, CON: 42, INT: 43, WIT: 44, MEN: 45, Critical: 305, MAtk: 306, CastSpd: 307, MDef: 309,
	}
	testCharInfo = CharInfo{
		ObjectID: 0x10000002, Name: "other", Pos: Point{X: 1, Y: 2, Z: 3}, Heading: 1000, ClassID: 10,
		PvPFlag: 1, Karma: 50, RunSpeed: 156, WalkSpeed: 104, Sitting: true, InCombat: true, StoreType: 1,
		RHand: 2369, LHand: 626, CollisionRadius: 8.5, ClanID: 0x0a000003, AllyID: 0x0a000004,
		Abnormal: 0x2000, CP: 1100, MaxCP: 1200, DuelTeam: 2,
	}
	testNPCInfo = NPCInfo{
		ObjectID: 0x10000003, NPCID: 20001, Attackable: true, Pos: Point{X: 4, Y: 5, Z: 6}, Heading: 2000,
		RunSpeed: 195, WalkSpeed: 60, Running: true, Dead: true, Summon: true, PvPFlag: 2, Karma: 3,
		Name: "Gremlin", Title: "a title", CollisionRadius: 10.5,
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
		{"NPCInfo no server-side name", npcInfoPacket(NPCInfo{ObjectID: 7, NPCID: 31228}, 0, 0, 1), NPCInfo{ObjectID: 7, NPCID: 31228}},
		{"NPCInfo class master", npcInfoPacket(liveMaster, 0, 0, 1), liveMaster},
		{
			"StatusUpdate", packet(OpGameStatusUpdate, 7, 12, 9, 300, 10, 410, 11, 120, 12, 150, 13, 5000, 14, 1500, 15, 80000, 18, 333, 24, 444, 27, 7, 33, 90, 34, 200),
			StatusUpdate{ObjectID: 7, Set: StatusHP | StatusMaxHP | StatusMP | StatusMaxMP | StatusSP | StatusLoad | StatusMaxLoad | StatusKarma | StatusCP | StatusMaxCP, HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, SP: 5000, Load: 1500, MaxLoad: 80000, Karma: 7, CP: 90, MaxCP: 200},
		},
		{"StatusUpdate unknown type only", packet(OpGameStatusUpdate, 7, 1, 18, 333), StatusUpdate{ObjectID: 7}},
		{"StatusUpdate empty", packet(OpGameStatusUpdate, 7, 0), StatusUpdate{ObjectID: 7}},
		{"MoveToPawn", packet(OpGameMoveToPawn, 7, 8, 40, at.X, at.Y, at.Z), MoveToPawn{ObjectID: 7, TargetID: 8, Distance: 40, Origin: at}},
		{"Die", packet(OpGameDie, 7, 1, 0, 1, 0, 1, 1), Die{ObjectID: 7, Restart: RestartPoints{Village: true, Castle: true, Fixed: true}, Sweepable: true}},
		{"Die village only", packet(OpGameDie, 7, 1, 0, 0, 0, 0, 0), Die{ObjectID: 7, Restart: RestartPoints{Village: true}}},
		{"Revive", packet(OpGameRevive, 7), Revive{ObjectID: 7}},
		{"MyTargetSelected", packet(OpGameMyTargetSelected, 7, uint16(0xfffb)), MyTargetSelected{ObjectID: 7, Color: 0xfffb}},
		{"TargetUnselected", packet(OpGameTargetUnselected, 7, at.X, at.Y, at.Z), TargetUnselected{ObjectID: 7, Pos: at}},
		{"SystemMessage no params", packet(OpGameSystemMessage, 212, 0), SystemMessage{ID: 212}},
		{
			"SystemMessage every param type",
			packet(OpGameSystemMessage, 1147, 6, 0, "text", 1, 5, 3, 57, 4, 1216, 2, 6, 100, 7, at.X, at.Y, at.Z),
			SystemMessage{ID: 1147, Text: [2]string{"text"}, Number: 5},
		},
		{"SystemMessage other type is one int32", packet(OpGameSystemMessage, 9, 2, 2, 20001, 5, 3), SystemMessage{ID: 9}},
		{"ActionFailed", packet(OpGameActionFailed), ActionFailed{}},
		{"CreatureSay", packet(OpGameCreatureSay, 7, 1, "bot7", "hello"), CreatureSay{ObjectID: 7, Type: 1, Name: "bot7", Text: "hello"}},
		{"ChangeWaitType", packet(OpGameChangeWaitType, 7, 0, at.X, at.Y, at.Z), ChangeWaitType{ObjectID: 7, Type: WaitSitting, Pos: at}},
		{"AutoAttackStart", packet(OpGameAutoAttackStart, 7), AutoAttackStart{ObjectID: 7}},
		{"AutoAttackStop", packet(OpGameAutoAttackStop, 7), AutoAttackStop{ObjectID: 7}},
	}
}

func TestDecodeInWorldEvent(t *testing.T) {
	for _, tt := range inworldCases() {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("DecodeEvent = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// Every in-world frame one byte short fails, and one byte long fails with
// ErrTrailingBytes: each decoder consumes exactly its layout.
func TestDecodeInWorldEventRejects(t *testing.T) {
	for _, tt := range inworldCases() {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.raw) > 1 {
				if _, _, err := DecodeEvent(tt.raw[:len(tt.raw)-1]); err == nil {
					t.Fatal("short frame decoded without error")
				}
			}
			long := append(append([]byte(nil), tt.raw...), 0)
			if _, _, err := DecodeEvent(long); !errors.Is(err, ErrTrailingBytes) {
				t.Fatalf("long frame error = %v, want %v", err, ErrTrailingBytes)
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
		{"StatusUpdate", packet(OpGameStatusUpdate, 7, 0x7fffffff, 9, 1)},
		{"SystemMessage", packet(OpGameSystemMessage, 7, 0x7fffffff, 1, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, wire.ErrShortPacket) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, wire.ErrShortPacket)
			}
		})
	}
}
