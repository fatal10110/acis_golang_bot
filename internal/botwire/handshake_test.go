package botwire

import (
	"bytes"
	"testing"

	gamecipher "github.com/fatal10110/acis_golang_bot/internal/gameserver/network/cipher"
)

// Opcodes and field orders in the expected payloads are the server's
// decoders (internal/gameserver/network/clientpackets) and encoders
// (internal/gameserver/network/serverpackets), written out literally.

func TestGameEncoders(t *testing.T) {
	keys := SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		// revision (protocolversion.go).
		{"ProtocolVersion", EncodeProtocolVersion(746), packet(0x00, 746)},
		// name, playKey2, playKey1, loginKey1, loginKey2 (authlogin.go).
		{"AuthLogin", EncodeAuthLogin("bot7", keys), packet(0x08, "bot7", 4, 3, 1, 2)},
		// name, race, sex, class, six stats, hair style, hair color, face
		// (requestcharactercreate.go).
		{"RequestCharacterCreate", EncodeRequestCharacterCreate("bot7", HumanFighter, Appearance{HairStyle: 1}), packet(0x0b, "bot7", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0)},
		// slot, uint16, three int32 (requestgamestart.go).
		{"RequestGameStart", EncodeRequestGameStart(2), packet(0x0d, 2, uint16(0), 0, 0, 0)},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, tt.want)
		}
	}
}

// versionCheck is the server's VersionCheck: a one, the dynamic key half,
// the crypt flag, then a fixed int32 one (versioncheck.go).
func versionCheck(dynamic []byte, enabled bool) []byte {
	flag := 0
	if enabled {
		flag = 1
	}
	return packet(0x00, byte(1), dynamic, flag, 1)
}

func TestDecodeVersionCheck(t *testing.T) {
	dynamic := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	wantKey := append(append([]byte(nil), dynamic...), gamecipher.StaticKey[:]...)
	tests := []struct {
		name        string
		raw         []byte
		wantEnabled bool
		wantErr     bool
	}{
		{"enabled", versionCheck(dynamic, true), true, false},
		{"disabled", versionCheck(dynamic, false), false, false},
		{"short", versionCheck(dynamic, true)[:versionCheckSize-1], false, true},
		{"long", append(versionCheck(dynamic, true), 0), false, true},
		{"wrong opcode", append([]byte{0x2e}, versionCheck(dynamic, true)[1:]...), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, enabled, err := DecodeVersionCheck(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("decoded without error")
				}
				return
			}
			if err != nil || enabled != tt.wantEnabled || !bytes.Equal(key, wantKey) {
				t.Fatalf("= %x, %v, %v; want %x, %v", key, enabled, err, wantKey, tt.wantEnabled)
			}
		})
	}
}

// userInfo is a whole UserInfo of bot7 at x, y, z, heading, with object id
// objectID (userinfo.go): EnterGame decodes it whole.
func userInfo(x, y, z, heading, objectID int32) []byte {
	u := testUserInfo
	u.Pos, u.Heading, u.ObjectID = Point{X: x, Y: y, Z: z}, heading, objectID
	return userInfoPacket(u, 120, 80, 1.3, 0)
}

func TestDecodeUserInfo(t *testing.T) {
	got, err := DecodeUserInfo(userInfo(-71000, 258000, -3100, 49152, 0x10000001))
	want := Self{ObjectID: 0x10000001, X: -71000, Y: 258000, Z: -3100, Heading: 49152}
	if err != nil || got.ObjectID != want.ObjectID || got.X != want.X || got.Y != want.Y || got.Z != want.Z || got.Heading != want.Heading {
		t.Fatalf("UserInfo = %+v, %v; want %+v", got, err, want)
	}
	if _, err := DecodeUserInfo(packet(0x04, 1, 2, 3, 4)); err == nil {
		t.Fatal("short UserInfo decoded without error")
	}
}
