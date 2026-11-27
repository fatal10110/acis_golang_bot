package botclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Opcodes and field orders in the expected payloads are the server's
// decoders (internal/loginserver/network/clientpackets) and encoders
// (internal/loginserver/network/serverpackets), written out literally.

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
		var want [botwire.AuthBlockSize]byte
		copy(want[0x5e:], "bot7")
		copy(want[0x6c:], "secret")
		if p[0] != 0x00 || len(p) < 1+botwire.AuthBlockSize || !bytes.Equal(rsaDecryptBlock(priv, p[1:1+botwire.AuthBlockSize]), want[:]) {
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
