package botwire

import (
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang_bot/internal/gameserver/network/cipher"
)

// The game server's handshake packets, from ProtocolVersion to the UserInfo
// of the EnterWorld burst (internal/gameserver/network).

// versionCheckSize is VersionCheck's payload length, opcode included.
const versionCheckSize = 18

// Self is the bot's own character as the EnterWorld burst described it.
type Self struct {
	// Name is the character's, the one selected or created.
	Name string
	// Slots is how many characters the account held once this one was
	// chosen: CharSelectInfo's count, after the creation if there was one.
	Slots    int
	ObjectID int32
	X, Y, Z  int32
	Heading  int32
	// Burst holds copies of the EnterWorld frames that preceded UserInfo
	// (objects already in sight among them), in arrival order.
	Burst [][]byte
	// UserInfo is a copy of the burst's UserInfo frame. NewWorld replays
	// it after Burst.
	UserInfo []byte
}

// DecodeVersionCheck returns the full cipher key (the dynamic half the
// packet carries plus the static half) and whether the server enabled crypt.
func DecodeVersionCheck(raw []byte) ([]byte, bool, error) {
	if len(raw) != versionCheckSize || raw[0] != OpGameVersionCheck {
		return nil, false, fmt.Errorf("first game packet is not VersionCheck (%d bytes, opcode %#x)", len(raw), raw[0])
	}
	key := make([]byte, gamecipher.KeySize)
	copy(key[:8], raw[2:10])
	copy(key[8:], gamecipher.StaticKey[:])
	return key, wire.NewReader(raw[10:14]).ReadInt32() != 0, nil
}

// DecodeUserInfo reads the leading position and object id of UserInfo.
func DecodeUserInfo(raw []byte) (Self, error) {
	r := wire.NewPacketReader(raw)
	var s Self
	s.X, s.Y, s.Z = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	s.Heading = r.ReadInt32()
	s.ObjectID = r.ReadInt32()
	if err := r.Err(); err != nil {
		return Self{}, fmt.Errorf("decode UserInfo: %w", err)
	}
	return s, nil
}

// EncodeProtocolVersion writes the client's protocol revision
// (protocolversion.go).
func EncodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(OpGameProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

// EncodeAuthLogin writes the keys in the protocol's fixed order: play key 2
// before play key 1, then the login keys.
func EncodeAuthLogin(account string, k SessionKeys) []byte {
	w := wire.NewPacketWriter(OpGameAuthLogin)
	w.WriteString(account)
	w.WriteInt32(k.PlayKey2)
	w.WriteInt32(k.PlayKey1)
	w.WriteInt32(k.LoginKey1)
	w.WriteInt32(k.LoginKey2)
	return w.Bytes()
}

// EncodeRequestCharacterCreate asks for a character named name of class c
// with appearance a; the six stat fields are ignored by the server.
func EncodeRequestCharacterCreate(name string, c StartingClass, a Appearance) []byte {
	w := wire.NewPacketWriter(OpGameRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(c.Race)
	w.WriteInt32(a.Sex)
	w.WriteInt32(c.ID)
	for range 6 {
		w.WriteInt32(0)
	}
	w.WriteInt32(a.HairStyle)
	w.WriteInt32(a.HairColor)
	w.WriteInt32(a.Face)
	return w.Bytes()
}

// EncodeRequestGameStart selects the character in slot; the trailing fields
// are unused by the server (requestgamestart.go).
func EncodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(OpGameRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	for range 3 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}
