package botwire

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"math/big"
	"reflect"
	"testing"

	commoncrypt "github.com/fatal10110/acis_golang_bot/internal/commons/crypt"
)

// Opcodes and field orders in the expected payloads are the server's
// decoders (internal/loginserver/network/clientpackets) and encoders
// (internal/loginserver/network/serverpackets), written out literally.

func TestLoginEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		// sessionID, then four GameGuard words (authgameguard.go).
		{"AuthGameGuard", EncodeAuthGameGuard(0x5a5a5a5a), packet(0x07, 0x5a5a5a5a, 0, 0, 0, 0)},
		// sessionKey1, sessionKey2 (requestserverlist.go).
		{"RequestServerList", EncodeRequestServerList(7, -9), packet(0x05, 7, -9)},
		// sessionKey1, sessionKey2, serverID (requestserverlogin.go).
		{"RequestServerLogin", EncodeRequestServerLogin(7, -9, 3), packet(0x02, 7, -9, byte(3))},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, tt.want)
		}
	}
}

func TestEncodeRequestAuthLogin(t *testing.T) {
	priv, _ := newLoginKey(t)
	tests := []struct {
		name, account, password string
		wantName, wantPass      string
	}{
		{"full fields", "bot12345678901", "pass456789012345", "bot12345678901", "pass456789012345"},
		{"short", "bot7", "pw", "bot7", "pw"},
		{"truncated", "bot123456789012345", "pass4567890123456789", "bot12345678901", "pass456789012345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := EncodeRequestAuthLogin(&priv.PublicKey, tt.account, tt.password)
			if len(p) != 1+AuthBlockSize || p[0] != 0x00 {
				t.Fatalf("RequestAuthLogin = %d bytes, opcode %#x; want %d, 0x00", len(p), p[0], 1+AuthBlockSize)
			}
			var want [AuthBlockSize]byte
			copy(want[0x5e:], tt.wantName)
			copy(want[0x6c:], tt.wantPass)
			if block := rsaDecryptBlock(priv, p[1:]); !bytes.Equal(block, want[:]) {
				t.Fatalf("credential block = %x, want %x", block, want)
			}
		})
	}
}

func TestDecodeInit(t *testing.T) {
	priv, scrambled := newLoginKey(t)
	key := []byte("fedcba9876543210")
	got, err := DecodeInit(encryptInit(t, initPayload(0x1234, scrambled, key), 0xdeadbeef))
	if err != nil {
		t.Fatal(err)
	}
	want := Init{SessionID: 0x1234, PublicKey: &rsa.PublicKey{N: priv.N, E: priv.E}, BlowfishKey: key}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Init = %+v, want %+v", got, want)
	}

	short := encryptInit(t, packet(0x00, 0x1234), 1)
	if _, err := DecodeInit(short); err == nil {
		t.Error("short Init decoded without error")
	}
	if _, err := DecodeInit(make([]byte, 7)); err == nil {
		t.Error("unaligned first packet decoded without error")
	}
}

func TestDecodeLoginReplies(t *testing.T) {
	// LoginOk carries more after its keys (loginok.go); PlayOk only them.
	loginOk := packet(0x03, 11, -22, 0, 0, 0x3ea, 0, 0, 0, make([]byte, 16))
	if k1, k2, err := DecodeKeyPair(loginOk); err != nil || k1 != 11 || k2 != -22 {
		t.Errorf("LoginOk keys = %d, %d, %v", k1, k2, err)
	}
	if k1, k2, err := DecodeKeyPair(packet(0x07, 33, -44)); err != nil || k1 != 33 || k2 != -44 {
		t.Errorf("PlayOk keys = %d, %d, %v", k1, k2, err)
	}
	if _, _, err := DecodeKeyPair(packet(0x07, 33)); err == nil {
		t.Error("short PlayOk decoded without error")
	}

	// count, last server, then per server: id, ip, port, age limit, pvp,
	// current and max players, online, type bits, brackets (serverlist.go).
	list := packet(0x04, byte(2), byte(1),
		byte(1), []byte{127, 0, 0, 1}, 7777, byte(0), byte(0), uint16(0), uint16(300), byte(1), 0, byte(0),
		byte(2), []byte{10, 0, 0, 2}, 7778, byte(18), byte(1), uint16(5), uint16(100), byte(0), 0x06, byte(1))
	servers, err := DecodeServerList(list)
	wantServers := []ServerEntry{{ID: 1, Addr: "127.0.0.1:7777", Online: true}, {ID: 2, Addr: "10.0.0.2:7778"}}
	if err != nil || !reflect.DeepEqual(servers, wantServers) {
		t.Errorf("ServerList = %+v, %v; want %+v", servers, err, wantServers)
	}
	if _, err := DecodeServerList(list[:len(list)-1]); err == nil {
		t.Error("truncated ServerList decoded without error")
	}
}

// testStaticBootstrapKey is the login server's fixed first-packet Blowfish
// key (internal/loginserver/crypt/logincrypt.go).
var testStaticBootstrapKey = []byte{
	0x6b, 0x60, 0xcb, 0x5b, 0x82, 0xce, 0x90, 0xb1,
	0xcc, 0x2b, 0x6c, 0x55, 0x6c, 0x6c, 0x6c, 0x6c,
}

// newLoginKey generates the login server's 1024-bit RSA key and the
// scrambled modulus Init carries.
func newLoginKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return priv, scrambleModulus(t, priv.N)
}

// scrambleModulus obfuscates a 1024-bit modulus as the login server does
// (internal/commons/crypt/rsakey.go in the server).
func scrambleModulus(t *testing.T, modulus *big.Int) []byte {
	t.Helper()
	s := modulus.Bytes()
	if len(s) != 128 {
		t.Fatalf("modulus is %d bytes, want 128", len(s))
	}
	for i := range 4 {
		s[i], s[0x4d+i] = s[0x4d+i], s[i]
	}
	for i := range 0x40 {
		s[i] ^= s[0x40+i]
	}
	for i := range 4 {
		s[0x0d+i] ^= s[0x34+i]
	}
	for i := range 0x40 {
		s[0x40+i] ^= s[i]
	}
	return s
}

// rsaDecryptBlock reverses the unpadded RSA of a credential block into its
// full 128 bytes, leading zeros kept.
func rsaDecryptBlock(priv *rsa.PrivateKey, ciphertext []byte) []byte {
	m := new(big.Int).Exp(new(big.Int).SetBytes(ciphertext), priv.D, priv.N)
	return m.FillBytes(make([]byte, 128))
}

// initPayload is the server's Init: session id, protocol revision, the
// scrambled modulus, the GameGuard placeholder, the dynamic Blowfish key and
// a trailing zero.
func initPayload(sessionID int32, scrambled, blowfishKey []byte) []byte {
	return packet(0x00, sessionID, int32(0xc621), scrambled, make([]byte, 16), blowfishKey, byte(0))
}

// encryptInit encrypts Init as the server's first packet: padded, rolling-XOR
// pass with key, then the static bootstrap key.
func encryptInit(t *testing.T, payload []byte, key uint32) []byte {
	t.Helper()
	static, err := commoncrypt.NewBlowfishCipher(testStaticBootstrapKey)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, commoncrypt.PaddedSize(len(payload)+4+4))
	copy(buf, payload)
	stop := len(buf) - 8
	ecx := key
	pos := 4
	for ; pos < stop; pos += 4 {
		edx := binary.LittleEndian.Uint32(buf[pos:])
		ecx += edx
		binary.LittleEndian.PutUint32(buf[pos:], edx^ecx)
	}
	binary.LittleEndian.PutUint32(buf[pos:], ecx)
	commoncrypt.EncryptBlocks(static, buf)
	return buf
}
