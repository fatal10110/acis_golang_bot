package botclient

import (
	"bytes"
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
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
		{"AuthGameGuard", encodeAuthGameGuard(0x5a5a5a5a), packet(0x07, 0x5a5a5a5a, 0, 0, 0, 0)},
		// sessionKey1, sessionKey2 (requestserverlist.go).
		{"RequestServerList", encodeRequestServerList(7, -9), packet(0x05, 7, -9)},
		// sessionKey1, sessionKey2, serverID (requestserverlogin.go).
		{"RequestServerLogin", encodeRequestServerLogin(7, -9, 3), packet(0x02, 7, -9, byte(3))},
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
			p := encodeRequestAuthLogin(&priv.PublicKey, tt.account, tt.password)
			if len(p) != 1+authBlockSize || p[0] != 0x00 {
				t.Fatalf("RequestAuthLogin = %d bytes, opcode %#x; want %d, 0x00", len(p), p[0], 1+authBlockSize)
			}
			var want [authBlockSize]byte
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
	got, err := decodeInit(encryptInit(t, initPayload(0x1234, scrambled, key), 0xdeadbeef))
	if err != nil {
		t.Fatal(err)
	}
	want := initPacket{sessionID: 0x1234, publicKey: &rsa.PublicKey{N: priv.N, E: priv.E}, blowfishKey: key}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Init = %+v, want %+v", got, want)
	}

	short := encryptInit(t, packet(0x00, 0x1234), 1)
	if _, err := decodeInit(short); err == nil {
		t.Error("short Init decoded without error")
	}
	if _, err := decodeInit(make([]byte, 7)); err == nil {
		t.Error("unaligned first packet decoded without error")
	}
}

func TestDecodeLoginReplies(t *testing.T) {
	// LoginOk carries more after its keys (loginok.go); PlayOk only them.
	loginOk := packet(0x03, 11, -22, 0, 0, 0x3ea, 0, 0, 0, make([]byte, 16))
	if k1, k2, err := decodeKeyPair(loginOk); err != nil || k1 != 11 || k2 != -22 {
		t.Errorf("LoginOk keys = %d, %d, %v", k1, k2, err)
	}
	if k1, k2, err := decodeKeyPair(packet(0x07, 33, -44)); err != nil || k1 != 33 || k2 != -44 {
		t.Errorf("PlayOk keys = %d, %d, %v", k1, k2, err)
	}
	if _, _, err := decodeKeyPair(packet(0x07, 33)); err == nil {
		t.Error("short PlayOk decoded without error")
	}

	// count, last server, then per server: id, ip, port, age limit, pvp,
	// current and max players, online, type bits, brackets (serverlist.go).
	list := packet(0x04, byte(2), byte(1),
		byte(1), []byte{127, 0, 0, 1}, 7777, byte(0), byte(0), uint16(0), uint16(300), byte(1), 0, byte(0),
		byte(2), []byte{10, 0, 0, 2}, 7778, byte(18), byte(1), uint16(5), uint16(100), byte(0), 0x06, byte(1))
	servers, err := decodeServerList(list)
	wantServers := []ServerEntry{{ID: 1, Addr: "127.0.0.1:7777", Online: true}, {ID: 2, Addr: "10.0.0.2:7778"}}
	if err != nil || !reflect.DeepEqual(servers, wantServers) {
		t.Errorf("ServerList = %+v, %v; want %+v", servers, err, wantServers)
	}
	if _, err := decodeServerList(list[:len(list)-1]); err == nil {
		t.Error("truncated ServerList decoded without error")
	}
}

func TestExpectLoginReply(t *testing.T) {
	tests := []struct {
		name   string
		reply  []byte
		opcode byte
		want   *LoginError // nil: no LoginError
		ok     bool
	}{
		{"expected", packet(0x07, 1, 2), opLoginPlayOk, nil, true},
		{"LoginFail", packet(0x01, 2), opLoginOk, &LoginError{Packet: "LoginFail", Reason: 2}, false},
		{"PlayFail", packet(0x06, byte(3)), opLoginPlayOk, &LoginError{Packet: "PlayFail", Reason: 3}, false},
		{"AccountKicked", packet(0x02, 1), opLoginOk, &LoginError{Packet: "AccountKicked", Reason: 1}, false},
		{"unexpected", packet(0x0b, 0), opLoginPlayOk, nil, false},
		{"empty", nil, opLoginPlayOk, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := expectLoginReply(tt.reply, tt.opcode, "want")
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok %v", err, tt.ok)
			}
			var le *LoginError
			if errors.As(err, &le) != (tt.want != nil) || (tt.want != nil && *le != *tt.want) {
				t.Fatalf("err = %#v, want %#v", err, tt.want)
			}
		})
	}
}

func TestLoginSession(t *testing.T) {
	priv, scrambled := newLoginKey(t)
	key := []byte("0123456789abcdef")
	const sessionID = 0x4242
	credentials := step{check: func(p []byte) error {
		var want [authBlockSize]byte
		copy(want[0x5e:], "bot7")
		copy(want[0x6c:], "secret")
		if p[0] != 0x00 || len(p) < 1+authBlockSize || !bytes.Equal(rsaDecryptBlock(priv, p[1:1+authBlockSize]), want[:]) {
			return fmt.Errorf("RequestAuthLogin %x does not carry the credentials", p)
		}
		return nil
	}}
	serverList := packet(0x04, byte(1), byte(1),
		byte(1), []byte{127, 0, 0, 1}, 7777, byte(0), byte(0), uint16(0), uint16(300), byte(1), 0, byte(0))
	greeting := []step{
		recvStep(packet(0x07, sessionID, 0, 0, 0, 0)),
		sendStep(packet(0x0b, sessionID, 0, 0, 0, 0)),
		credentials,
	}
	tests := []struct {
		name     string
		serverID byte
		script   []step
		want     LoginResult
		wantErr  error // matched with errors.As for a *LoginError
		failText string
	}{
		{
			name:     "licence shown",
			serverID: 1,
			script: append(greeting[:3:3],
				sendStep(packet(0x03, 11, 22, 0, 0, 0x3ea, 0, 0, 0, make([]byte, 16))),
				recvStep(packet(0x05, 11, 22)),
				sendStep(serverList),
				recvStep(packet(0x02, 11, 22, byte(1))),
				sendStep(packet(0x07, 33, 44)),
			),
			want: LoginResult{Keys: SessionKeys{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}, GameAddr: "127.0.0.1:7777"},
		},
		{
			name:     "licence off",
			serverID: 1,
			script: append(greeting[:3:3],
				sendStep(serverList),
				recvStep(packet(0x02, 0, 0, byte(1))),
				sendStep(packet(0x07, 33, 44)),
			),
			want: LoginResult{Keys: SessionKeys{PlayKey1: 33, PlayKey2: 44}, GameAddr: "127.0.0.1:7777"},
		},
		{
			name:     "wrong password",
			serverID: 1,
			script:   append(greeting[:3:3], sendStep(packet(0x01, 2))),
			wantErr:  &LoginError{Packet: "LoginFail", Reason: 2},
		},
		{
			name:     "server full",
			serverID: 1,
			script: append(greeting[:3:3],
				sendStep(serverList),
				recvStep(packet(0x02, 0, 0, byte(1))),
				sendStep(packet(0x06, byte(15))),
			),
			wantErr: &LoginError{Packet: "PlayFail", Reason: 15},
		},
		{
			name:     "server not listed",
			serverID: 9,
			script:   append(greeting[:3:3], sendStep(serverList)),
			failText: "server 9 is not in the ServerList",
		},
		{
			name:     "hang up after GameGuard",
			serverID: 1,
			script:   greeting[:1],
			failText: "read GGAuth",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			s := newFakeServer(server)
			s.padded = true
			decrypt, encrypt := loginCryptFuncs(t, key)
			first := encryptInit(t, initPayload(sessionID, scrambled, key), 0x01020304)
			done := make(chan error, 1)
			go func() {
				// Init goes out under the static key; the session crypt
				// takes over after it.
				if err := s.send(first); err != nil {
					done <- err
					return
				}
				s.decrypt, s.encrypt = decrypt, encrypt
				done <- s.run(tt.script, true)
			}()

			got, err := login(t.Context(), client, "bot7", "secret", tt.serverID, 5*time.Second)
			client.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			switch {
			case tt.wantErr != nil:
				var le *LoginError
				if !errors.As(err, &le) || *le != *tt.wantErr.(*LoginError) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.failText != "":
				if err == nil || !strings.Contains(err.Error(), tt.failText) {
					t.Fatalf("err = %v, want %q", err, tt.failText)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if got.Connected.IsZero() {
					t.Error("Connected not set")
				}
				got.Connected = time.Time{}
				if got != tt.want {
					t.Fatalf("result = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestLoginContextCanceled(t *testing.T) {
	_, scrambled := newLoginKey(t)
	key := []byte("0123456789abcdef")
	decrypt, encrypt := loginCryptFuncs(t, key)
	first := encryptInit(t, initPayload(0x4242, scrambled, key), 0x01020304)
	tests := []struct {
		name string
		// whileReading cancels once the server holds AuthGameGuard, so
		// the client is blocked reading GGAuth; else ctx ends before the
		// session starts and the server never sends Init.
		whileReading bool
	}{
		{"before Init", false},
		{"while reading GGAuth", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s := newFakeServer(server)
			s.padded = true
			done := make(chan error, 1)
			if tt.whileReading {
				go func() {
					if err := s.send(first); err != nil {
						done <- err
						return
					}
					s.decrypt, s.encrypt = decrypt, encrypt
					// The server answers nothing: only ctx ends the read.
					done <- s.run([]step{{check: func([]byte) error { cancel(); return nil }}}, false)
				}()
			} else {
				cancel()
				go func() { done <- s.run(nil, false) }()
			}

			_, err := login(ctx, client, "bot7", "secret", 1, time.Hour)
			client.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		})
	}
}
