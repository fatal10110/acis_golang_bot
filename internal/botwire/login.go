package botwire

import (
	"crypto/rsa"
	"fmt"
	"net"
	"strconv"

	commoncrypt "github.com/fatal10110/acis_golang_bot/internal/commons/crypt"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	logincrypt "github.com/fatal10110/acis_golang_bot/internal/loginserver/crypt"
)

// The login server's packets (internal/loginserver/network): what the login
// client reads and sends from Init to PlayOk.

// Credential block layout of RequestAuthLogin: the name and password sit at
// fixed offsets of one AuthBlockSize-byte RSA block.
const (
	AuthBlockSize   = 128
	authNameOffset  = 0x5e
	authNameMax     = 14
	authPassOffset  = 0x6c
	authPassMax     = 16
	initPayloadSize = 1 + 4 + 4 + 128 + 16 + 16 + 1
)

// SessionKeys are the login and play key pairs the login server hands out;
// the game server checks them against the login server on AuthLogin.
type SessionKeys struct {
	LoginKey1, LoginKey2 int32
	PlayKey1, PlayKey2   int32
}

// ServerEntry is one row of the login server's ServerList.
type ServerEntry struct {
	ID     byte
	Addr   string
	Online bool
}

// Init is the login server's first packet: the session id, the RSA key the
// credentials are encrypted with and the Blowfish key of the rest of the
// session.
type Init struct {
	SessionID   int32
	PublicKey   *rsa.PublicKey
	BlowfishKey []byte
}

// DecodeInit decrypts the first login packet in place and parses it.
func DecodeInit(frame []byte) (Init, error) {
	var p Init
	if err := logincrypt.DecryptFirst(frame); err != nil {
		return p, err
	}
	if len(frame) < initPayloadSize || frame[0] != OpLoginInit {
		return p, fmt.Errorf("first login packet is not Init (%d bytes, opcode %#x)", len(frame), frame[0])
	}
	r := wire.NewPacketReader(frame)
	p.SessionID = r.ReadInt32()
	_ = r.ReadInt32() // protocol revision
	pub, err := commoncrypt.UnscrambleModulus(r.ReadBytes(128))
	if err != nil {
		return p, err
	}
	p.PublicKey = pub
	_ = r.ReadBytes(16) // GameGuard placeholder
	p.BlowfishKey = append([]byte(nil), r.ReadBytes(16)...)
	return p, r.Err()
}

// DecodeKeyPair reads the two leading session keys of LoginOk or PlayOk.
func DecodeKeyPair(reply []byte) (key1, key2 int32, err error) {
	r := wire.NewPacketReader(reply)
	key1, key2 = r.ReadInt32(), r.ReadInt32()
	return key1, key2, r.Err()
}

// DecodeServerList reads every row of ServerList (serverlist.go).
func DecodeServerList(reply []byte) ([]ServerEntry, error) {
	r := wire.NewPacketReader(reply)
	n := int(r.ReadUint8())
	_ = r.ReadUint8() // last server
	servers := make([]ServerEntry, 0, n)
	for range n {
		var s ServerEntry
		s.ID = r.ReadUint8()
		ip := net.IP(r.ReadBytes(4))
		port := r.ReadInt32()
		s.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
		_ = r.ReadUint8()  // age limit
		_ = r.ReadUint8()  // PvP
		_ = r.ReadUint16() // current players
		_ = r.ReadUint16() // max players
		s.Online = r.ReadUint8() != 0
		_ = r.ReadInt32() // server type bits
		_ = r.ReadUint8() // brackets
		if r.Err() != nil {
			break
		}
		servers = append(servers, s)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("decode ServerList: %w", err)
	}
	return servers, nil
}

// EncodeAuthGameGuard writes the session id, then four zero GameGuard words
// (authgameguard.go).
func EncodeAuthGameGuard(sessionID int32) []byte {
	w := wire.NewPacketWriter(OpLoginAuthGameGuard)
	w.WriteInt32(sessionID)
	for range 4 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}

// EncodeRequestAuthLogin RSA-encrypts (no padding) the credential block
// with the session's public key. Longer names and passwords are truncated
// to their fields.
func EncodeRequestAuthLogin(pub *rsa.PublicKey, account, password string) []byte {
	var block [AuthBlockSize]byte
	copy(block[authNameOffset:authNameOffset+authNameMax], account)
	copy(block[authPassOffset:authPassOffset+authPassMax], password)
	w := wire.NewPacketWriter(OpLoginRequestAuthLogin)
	w.WriteBytes(commoncrypt.EncryptDynamicKey(pub, block[:]))
	return w.Bytes()
}

// EncodeRequestServerList writes the login key pair (requestserverlist.go).
func EncodeRequestServerList(key1, key2 int32) []byte {
	w := wire.NewPacketWriter(OpLoginRequestServerList)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	return w.Bytes()
}

// EncodeRequestServerLogin writes the login key pair and the chosen server
// (requestserverlogin.go).
func EncodeRequestServerLogin(key1, key2 int32, serverID byte) []byte {
	w := wire.NewPacketWriter(OpLoginRequestServerLogin)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	w.WriteUint8(serverID)
	return w.Bytes()
}
