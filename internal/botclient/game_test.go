package botclient

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

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
		{"ProtocolVersion", encodeProtocolVersion(746), packet(0x00, 746)},
		// name, playKey2, playKey1, loginKey1, loginKey2 (authlogin.go).
		{"AuthLogin", encodeAuthLogin("bot7", keys), packet(0x08, "bot7", 4, 3, 1, 2)},
		// name, race, sex, class, six stats, hair style, hair color, face
		// (requestcharactercreate.go).
		{"RequestCharacterCreate", encodeRequestCharacterCreate("bot7", HumanFighter, Appearance{HairStyle: 1}), packet(0x0b, "bot7", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0)},
		// slot, uint16, three int32 (requestgamestart.go).
		{"RequestGameStart", encodeRequestGameStart(2), packet(0x0d, 2, uint16(0), 0, 0, 0)},
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
			key, enabled, err := decodeVersionCheck(tt.raw)
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

// userInfoHead is the leading part of UserInfo alone: x, y, z, heading,
// object id, then the name.
func userInfoHead(x, y, z, heading, objectID int32) []byte {
	return packet(0x04, x, y, z, heading, objectID, "bot7")
}

func TestDecodeUserInfo(t *testing.T) {
	got, err := decodeUserInfo(userInfo(-71000, 258000, -3100, 49152, 0x10000001))
	want := Self{ObjectID: 0x10000001, X: -71000, Y: 258000, Z: -3100, Heading: 49152}
	if err != nil || got.ObjectID != want.ObjectID || got.X != want.X || got.Y != want.Y || got.Z != want.Z || got.Heading != want.Heading {
		t.Fatalf("UserInfo = %+v, %v; want %+v", got, err, want)
	}
	if _, err := decodeUserInfo(packet(0x04, 1, 2, 3, 4)); err == nil {
		t.Fatal("short UserInfo decoded without error")
	}
}

var testDynamicKey = []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}

// dialFake runs a GameConn's handshake against a fake game server, which
// then plays script and hangs up or drains (see fakeServer.run). With crypt
// the server enables the cipher. The returned channel yields the server's
// result once the client end is closed.
func dialFake(t *testing.T, crypt bool, script []step, hangUp bool) (*GameConn, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close() })
	s := newFakeServer(server)
	decrypt, encrypt := gameCipherFuncs(t, append(append([]byte(nil), testDynamicKey...), gamecipher.StaticKey[:]...))
	done := make(chan error, 1)
	go func() {
		err := s.play([]step{recvStep(packet(0x00, 746)), sendStep(versionCheck(testDynamicKey, crypt))})
		if err != nil {
			server.Close()
			done <- err
			return
		}
		if crypt {
			s.decrypt, s.encrypt = decrypt, encrypt
		}
		done <- s.run(script, hangUp)
	}()
	g, err := newGameConn(t.Context(), client, 746, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if (g.cipher != nil) != crypt {
		t.Fatalf("cipher armed = %v, want %v", g.cipher != nil, crypt)
	}
	return g, done
}

func TestNewGameConnRejectsNonVersionCheck(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	s := newFakeServer(server)
	done := make(chan error, 1)
	go func() { done <- s.run([]step{recvStep(packet(0x00, 746)), sendStep(packet(0x26))}, true) }()
	if _, err := newGameConn(t.Context(), client, 746, 5*time.Second); err == nil || !strings.Contains(err.Error(), "not VersionCheck") {
		t.Fatalf("err = %v, want a VersionCheck error", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("fake server: %v", err)
	}
}

// testCharacter is a human fighter named bot7 with hair style 1.
var testCharacter = Character{Names: []string{"bot7"}, Class: HumanFighter, Appearance: Appearance{HairStyle: 1}}

func TestEnterGame(t *testing.T) {
	const objectID = 0x10000001
	authLogin := recvStep(packet(0x08, "bot7", 4, 3, 1, 2))
	charSelectInfo := func(names ...string) step { return sendStep(charSelectInfoPacket(named(names...)...)) }
	gameStart := []step{
		recvStep(packet(0x0d, 0, uint16(0), 0, 0, 0)),
		sendStep(packet(0x2f, 1)), // unrelated frames before CharSelected are skipped
		sendStep(packet(0x15, "bot7", objectID)),
		recvStep([]byte{0x03}),
	}
	npcInfo := packet(0x16, 0x10000002, 20101)
	enter := append(gameStart[:4:4], sendStep(npcInfo), sendStep(userInfo(-71000, 258000, -3100, 49152, objectID)))
	wantSelf := Self{ObjectID: objectID, X: -71000, Y: 258000, Z: -3100, Heading: 49152, Burst: [][]byte{npcInfo}, Slots: 1}
	cat := func(parts ...[]step) []step {
		var out []step
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	tests := []struct {
		name    string
		crypt   bool
		script  []step
		want    Self
		wantErr error // a *GameStartError compared by value, else errors.Is
	}{
		{"existing character", true, cat([]step{authLogin, charSelectInfo("bot7")}, enter), wantSelf, nil},
		{"crypt off", false, cat([]step{authLogin, charSelectInfo("bot7")}, enter), wantSelf, nil},
		{
			"new character", true,
			cat([]step{
				authLogin, charSelectInfo(),
				recvStep(packet(0x0b, "bot7", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0)),
				sendStep(packet(0x19, 1)),
				charSelectInfo("bot7"),
			}, enter),
			wantSelf, nil,
		},
		{"auth rejected", true, []step{authLogin, sendStep(packet(0x14, 4))}, Self{}, &GameStartError{Packet: "AuthLoginFail", Reason: 4}},
		{
			"name taken", true,
			[]step{authLogin, charSelectInfo(), recvStep(packet(0x0b, "bot7", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0)), sendStep(packet(0x1a, 2))},
			Self{},
			&GameStartError{Packet: "CharCreateFail", Reason: 2},
		},
		{"ServerClose in burst", true, cat([]step{authLogin, charSelectInfo("bot7")}, gameStart, []step{sendStep(packet(0x26))}), Self{}, errServerClose},
		{"ServerClose before select", true, []step{authLogin, sendStep(packet(0x26))}, Self{}, errServerClose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, done := dialFake(t, tt.crypt, tt.script, tt.wantErr != nil)
			self, err := EnterGame(t.Context(), g, "bot7", SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}, testCharacter, nil)
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			var want *GameStartError
			switch {
			case errors.As(tt.wantErr, &want):
				var got *GameStartError
				if !errors.As(err, &got) || *got != *want {
					t.Fatalf("err = %v, want %v", err, want)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			}
			if self.ObjectID != tt.want.ObjectID || self.X != tt.want.X || self.Y != tt.want.Y || self.Z != tt.want.Z ||
				self.Heading != tt.want.Heading || len(self.Burst) != len(tt.want.Burst) {
				t.Fatalf("Self = %+v, want %+v", self, tt.want)
			}
			for i := range self.Burst {
				if !bytes.Equal(self.Burst[i], tt.want.Burst[i]) {
					t.Fatalf("Burst[%d] = %x, want %x", i, self.Burst[i], tt.want.Burst[i])
				}
			}
		})
	}
}

func TestLogout(t *testing.T) {
	logout := recvStep([]byte{0x09})
	leaveWorld := sendStep(packet(0x7e))
	actionFailed := sendStep(packet(0x25))
	tests := []struct {
		name     string
		script   []step
		hangUp   bool
		grace    time.Duration
		wantErr  error
		failText string
	}{
		{"left", []step{logout, sendStep(packet(0x04, 1, 2, 3, 4, 5, "bot7")), leaveWorld}, true, time.Minute, nil, ""},
		// An ActionFailed already in flight is followed by LeaveWorld.
		{"ActionFailed in flight", []step{logout, actionFailed, leaveWorld}, true, time.Minute, nil, ""},
		// A refusal is ActionFailed and nothing more; a zero grace expires
		// at once.
		{"refused", []step{logout, actionFailed}, false, 0, ErrLogoutRefused, ""},
		{"closed without LeaveWorld", []step{logout}, true, time.Minute, nil, "read LeaveWorld"},
		{"ServerClose", []step{logout, sendStep(packet(0x26))}, true, time.Minute, errServerClose, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, done := dialFake(t, true, tt.script, tt.hangUp)
			g.logoutGrace = tt.grace
			err := g.Logout(t.Context())
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			switch {
			case tt.failText != "":
				if err == nil || !strings.Contains(err.Error(), tt.failText) {
					t.Fatalf("err = %v, want %q", err, tt.failText)
				}
			case !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil):
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGameContextCanceled(t *testing.T) {
	enterGame := func(ctx context.Context, g *GameConn) error {
		_, err := EnterGame(ctx, g, "bot7", SessionKeys{}, testCharacter, nil)
		return err
	}
	logout := func(ctx context.Context, g *GameConn) error { return g.Logout(ctx) }
	tests := []struct {
		name string
		run  func(ctx context.Context, g *GameConn) error
		// whileReading cancels once the server holds the request, so the
		// client is blocked reading the reply; else ctx ends before the
		// request is sent.
		whileReading bool
	}{
		{"EnterGame before send", enterGame, false},
		{"Logout before send", logout, false},
		{"EnterGame while reading CharSelectInfo", enterGame, true},
		{"Logout while reading LeaveWorld", logout, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// The server answers nothing: only ctx ends the call.
			var script []step
			if tt.whileReading {
				script = []step{{check: func([]byte) error { cancel(); return nil }}}
			} else {
				cancel()
			}
			g, done := dialFake(t, true, script, false)
			err := tt.run(ctx, g)
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		})
	}
}
