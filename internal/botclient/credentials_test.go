package botclient

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCheckCredentials(t *testing.T) {
	priv, scrambled := newLoginKey(t)
	key := []byte("0123456789abcdef")
	const sessionID = 0x4242
	credentials := step{check: func(p []byte) error {
		var want [authBlockSize]byte
		copy(want[0x5e:], "bot7")
		copy(want[0x6c:], "pw")
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
		script   []step
		wantErr  *LoginError
		failText string
	}{
		{
			name: "licence shown",
			script: append(greeting[:3:3],
				sendStep(packet(0x03, 11, 22, 0, 0, 0x3ea, 0, 0, 0, make([]byte, 16))),
				recvStep(packet(0x05, 11, 22)),
				sendStep(serverList),
			),
		},
		{
			name:   "licence off",
			script: append(greeting[:3:3], sendStep(serverList)),
		},
		{
			name:    "wrong password",
			script:  append(greeting[:3:3], sendStep(packet(0x01, 2))),
			wantErr: &LoginError{Packet: "LoginFail", Reason: 2},
		},
		{
			name:     "hang up after credentials",
			script:   greeting[:3:3],
			failText: "read login reply",
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
			hangUp := tt.wantErr != nil || tt.failText != ""
			done := make(chan error, 1)
			go func() {
				if err := s.send(first); err != nil {
					done <- err
					return
				}
				s.decrypt, s.encrypt = decrypt, encrypt
				if err := s.play(tt.script); err != nil || hangUp {
					server.Close()
					done <- err
					return
				}
				// After the ServerList the client must close without
				// choosing a server: no RequestServerLogin, no play keys.
				if p, err := s.recv(); err == nil {
					done <- fmt.Errorf("client sent %x after the ServerList", p)
					return
				}
				done <- nil
			}()

			err := checkCredentials(t.Context(), client, "bot7", "pw", 5*time.Second)
			client.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			switch {
			case tt.wantErr != nil:
				var le *LoginError
				if !errors.As(err, &le) || *le != *tt.wantErr {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.failText != "":
				if err == nil || !strings.Contains(err.Error(), tt.failText) {
					t.Fatalf("err = %v, want %q", err, tt.failText)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}
