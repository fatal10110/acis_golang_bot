package botclient

import (
	"fmt"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang_bot/internal/gameserver/network/cipher"
)

// versionCheckSize is VersionCheck's payload length, opcode included.
const versionCheckSize = 18

// decodeVersionCheck returns the full cipher key (the dynamic half the
// packet carries plus the static half) and whether the server enabled crypt.
func decodeVersionCheck(raw []byte) ([]byte, bool, error) {
	if len(raw) != versionCheckSize || raw[0] != opGameVersionCheck {
		return nil, false, fmt.Errorf("first game packet is not VersionCheck (%d bytes, opcode %#x)", len(raw), raw[0])
	}
	key := make([]byte, gamecipher.KeySize)
	copy(key[:8], raw[2:10])
	copy(key[8:], gamecipher.StaticKey[:])
	return key, wire.NewReader(raw[10:14]).ReadInt32() != 0, nil
}

// decodeUserInfo reads the leading position and object id of UserInfo.
func decodeUserInfo(raw []byte) (Self, error) {
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

func encodeProtocolVersion(revision int32) []byte {
	w := wire.NewPacketWriter(opGameProtocolVersion)
	w.WriteInt32(revision)
	return w.Bytes()
}

// encodeAuthLogin writes the keys in the protocol's fixed order: play key 2
// before play key 1, then the login keys.
func encodeAuthLogin(account string, k SessionKeys) []byte {
	w := wire.NewPacketWriter(opGameAuthLogin)
	w.WriteString(account)
	w.WriteInt32(k.PlayKey2)
	w.WriteInt32(k.PlayKey1)
	w.WriteInt32(k.LoginKey1)
	w.WriteInt32(k.LoginKey2)
	return w.Bytes()
}

// encodeRequestCharacterCreate asks for a character named name of class c
// with appearance a; the six stat fields are ignored by the server.
func encodeRequestCharacterCreate(name string, c StartingClass, a Appearance) []byte {
	w := wire.NewPacketWriter(opGameRequestCharacterCreate)
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

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(opGameRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	for range 3 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}
