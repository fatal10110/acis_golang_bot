package botclient

// The server halves the tests play against, copied from the acis_golang
// server as test oracles: login key generation and modulus scrambling, the
// first-packet encryption of Init, and a scripted fake server on a net.Pipe.
// The bot never runs any of them, so they stay out of the shipped package.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"testing"

	commoncrypt "github.com/fatal10110/acis_golang_bot/internal/commons/crypt"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang_bot/internal/gameserver/network/cipher"
	logincrypt "github.com/fatal10110/acis_golang_bot/internal/loginserver/crypt"
)

// testStaticBootstrapKey is the login server's fixed first-packet Blowfish
// key (internal/loginserver/crypt/logincrypt.go).
var testStaticBootstrapKey = []byte{
	0x6b, 0x60, 0xcb, 0x5b, 0x82, 0xce, 0x90, 0xb1,
	0xcc, 0x2b, 0x6c, 0x55, 0x6c, 0x6c, 0x6c, 0x6c,
}

// packet builds a payload: the opcode, then each field in order (int and
// int32 as int32, byte, uint16, string as UTF-16 with its terminator, []byte
// as is).
func packet(opcode byte, fields ...any) []byte {
	w := wire.NewPacketWriter(opcode)
	for _, f := range fields {
		switch v := f.(type) {
		case int:
			w.WriteInt32(int32(v))
		case int32:
			w.WriteInt32(v)
		case byte:
			w.WriteUint8(v)
		case uint16:
			w.WriteUint16(v)
		case string:
			w.WriteString(v)
		case []byte:
			w.WriteBytes(v)
		default:
			panic(fmt.Sprintf("packet: unsupported field %T", f))
		}
	}
	return w.Bytes()
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

// step is one move of a scripted fake server: send writes a payload; else it
// reads one client packet, which must equal want (when set) and pass check
// (when set).
type step struct {
	send  []byte
	want  []byte
	check func([]byte) error
}

func sendStep(p []byte) step { return step{send: p} }

// recvStep expects exactly want from the client.
func recvStep(want []byte) step { return step{want: want} }

// fakeServer is the server end of a net.Pipe. decrypt and encrypt are its
// session crypt once armed (nil: cleartext). padded marks login packets,
// whose decrypted payloads keep their block padding and checksum.
type fakeServer struct {
	conn    net.Conn
	frames  *wire.FrameReader
	decrypt func([]byte) ([]byte, error)
	encrypt func([]byte) []byte
	padded  bool
}

func newFakeServer(conn net.Conn) *fakeServer {
	return &fakeServer{conn: conn, frames: wire.NewFrameReader(conn)}
}

func (s *fakeServer) send(p []byte) error {
	if s.encrypt != nil {
		p = s.encrypt(append([]byte(nil), p...))
	}
	return wire.WriteFrame(s.conn, p)
}

func (s *fakeServer) recv() ([]byte, error) {
	p, err := s.frames.ReadFrame()
	if err != nil || s.decrypt == nil {
		return p, err
	}
	return s.decrypt(p)
}

// play plays steps. A padded login packet must carry only zero padding
// between want and its 4-byte checksum.
func (s *fakeServer) play(steps []step) error {
	for i, st := range steps {
		if st.send != nil {
			if err := s.send(st.send); err != nil {
				return fmt.Errorf("step %d: send %#x: %w", i, st.send[0], err)
			}
			continue
		}
		got, err := s.recv()
		if err != nil {
			return fmt.Errorf("step %d: recv: %w", i, err)
		}
		if st.want != nil {
			if s.padded && len(got) >= len(st.want)+4 && allZero(got[len(st.want):len(got)-4]) {
				got = got[:len(st.want)]
			}
			if !bytes.Equal(got, st.want) {
				return fmt.Errorf("step %d: client sent %x, want %x", i, got, st.want)
			}
		}
		if st.check != nil {
			if err := st.check(got); err != nil {
				return fmt.Errorf("step %d: %w", i, err)
			}
		}
	}
	return nil
}

// run plays steps, then closes the connection when hangUp is set, else
// drains it until the client closes. A failed step closes the connection so
// the client never blocks on it.
func (s *fakeServer) run(steps []step, hangUp bool) error {
	if err := s.play(steps); err != nil {
		s.conn.Close()
		return err
	}
	if hangUp {
		return s.conn.Close()
	}
	for {
		if _, err := s.frames.ReadFrame(); err != nil {
			return nil
		}
	}
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// loginCryptFuncs returns the server's post-Init login crypt for key: the
// same Blowfish and checksum as the client side, in both directions.
// Decrypted payloads keep their padding and checksum.
func loginCryptFuncs(t *testing.T, key []byte) (func([]byte) ([]byte, error), func([]byte) []byte) {
	t.Helper()
	c, err := logincrypt.NewClientLoginCrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	decrypt := func(p []byte) ([]byte, error) { return p, c.Decrypt(p) }
	return decrypt, c.Encrypt
}

// gameCipherFuncs returns the server's game cipher for key (16 bytes),
// armed.
func gameCipherFuncs(t *testing.T, key []byte) (func([]byte) ([]byte, error), func([]byte) []byte) {
	t.Helper()
	c, err := gamecipher.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	c.Encrypt(nil)
	decrypt := func(p []byte) ([]byte, error) { c.Decrypt(p); return p, nil }
	encrypt := func(p []byte) []byte { c.Encrypt(p); return p }
	return decrypt, encrypt
}
