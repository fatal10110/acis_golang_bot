package botwire

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pvpWireCases are the duel window frames and MagicSkillCanceled, each
// byte for byte as the server writes it (serverpackets/duel.go
// FrameExDuelAskStart, frameDuelFlag, FrameExDuelUpdateUserInfo;
// serverpackets/magic_skill.go FrameMagicSkillCanceled): the extended
// opcode 0xfe, the little-endian sub-opcode, then each field, a string as
// UTF-16LE closed by a zero character.
func pvpWireCases() []struct {
	name string
	raw  []byte
	want Event
} {
	return []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ExDuelAskStart", []byte{0xfe, 0x4b, 0x00, 'A', 0, 'b', 0, 0, 0, 0, 0, 0, 0}, ExDuelAskStart{Requester: "Ab"}},
		{"ExDuelAskStart party", []byte{0xfe, 0x4b, 0x00, 'A', 0, 0, 0, 1, 0, 0, 0}, ExDuelAskStart{Requester: "A", Party: true}},
		{"ExDuelReady", []byte{0xfe, 0x4c, 0x00, 0, 0, 0, 0}, ExDuelReady{}},
		{"ExDuelReady party", []byte{0xfe, 0x4c, 0x00, 1, 0, 0, 0}, ExDuelReady{Party: true}},
		{"ExDuelStart", []byte{0xfe, 0x4d, 0x00, 0, 0, 0, 0}, ExDuelStart{}},
		{"ExDuelStart party", []byte{0xfe, 0x4d, 0x00, 1, 0, 0, 0}, ExDuelStart{Party: true}},
		{"ExDuelEnd", []byte{0xfe, 0x4e, 0x00, 0, 0, 0, 0}, ExDuelEnd{}},
		{"ExDuelEnd party", []byte{0xfe, 0x4e, 0x00, 1, 0, 0, 0}, ExDuelEnd{Party: true}},
		{
			"ExDuelUpdateUserInfo",
			[]byte{
				0xfe, 0x4f, 0x00, 'M', 0, 0, 0,
				0x02, 0x00, 0x00, 0x10, // object id
				0x0a, 0, 0, 0, // class id
				0x14, 0, 0, 0, // level
				0x2c, 0x01, 0, 0, // HP 300
				0x9a, 0x01, 0, 0, // max HP 410
				0x78, 0, 0, 0, // MP 120
				0x96, 0, 0, 0, // max MP 150
				0x5a, 0, 0, 0, // CP 90
				0xc8, 0, 0, 0, // max CP 200
			},
			ExDuelUpdateUserInfo{Name: "M", ObjectID: 0x10000002, ClassID: 10, Level: 20, HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, CP: 90, MaxCP: 200},
		},
		{"MagicSkillCanceled", []byte{0x49, 0x02, 0x00, 0x00, 0x10}, MagicSkillCanceled{CasterID: 0x10000002}},
	}
}

func TestPvPWireDecode(t *testing.T) {
	for _, tt := range pvpWireCases() {
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

// Every frame one byte short fails, and one byte long fails with
// ErrTrailingBytes: each decoder consumes exactly len(payload).
func TestPvPWireDecodeRejects(t *testing.T) {
	for _, tt := range pvpWireCases() {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw[:len(tt.raw)-1]); err == nil {
				t.Fatal("short frame decoded without error")
			}
			long := append(append([]byte(nil), tt.raw...), 0)
			if _, _, err := DecodeEvent(long); !errors.Is(err, ErrTrailingBytes) {
				t.Fatalf("long frame error = %v, want %v", err, ErrTrailingBytes)
			}
		})
	}
}

// TestRecordedPvPFields: the recorded UserInfo (04.bin) and NPCInfo (16.bin)
// captures keep a plausible collision radius, and the bot's kill counts
// are not negative. A field read from the wrong offset lands far outside.
func TestRecordedPvPFields(t *testing.T) {
	decode := func(name string) Event {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("testdata", "recorded", name))
		if err != nil {
			t.Fatal(err)
		}
		ev, ok, err := DecodeEvent(raw)
		if err != nil || !ok {
			t.Fatalf("%s: ok = %v, err = %v", name, ok, err)
		}
		return ev
	}
	u, ok := decode("04.bin").(UserInfo)
	if !ok {
		t.Fatal("04.bin is not a UserInfo")
	}
	if u.CollisionRadius != 7.5 {
		t.Errorf("UserInfo CollisionRadius = %v, want 7.5", u.CollisionRadius)
	}
	if u.PKKills != 0 || u.PvPKills != 0 || u.AllyID != 0 {
		t.Errorf("UserInfo PKKills, PvPKills, AllyID = %d, %d, %d, want 0, 0, 0", u.PKKills, u.PvPKills, u.AllyID)
	}
	n, ok := decode("16.bin").(NPCInfo)
	if !ok {
		t.Fatal("16.bin is not an NPCInfo")
	}
	if n.CollisionRadius != 13 {
		t.Errorf("NPCInfo CollisionRadius = %v, want 13", n.CollisionRadius)
	}
}
