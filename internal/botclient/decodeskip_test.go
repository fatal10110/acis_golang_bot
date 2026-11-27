package botclient

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// seed is a field value the skip tests look for in log lines: no payload
// byte may reach one.
const seed = 0x5eed5eed

// TestReadEventsSkipsMalformedFrame: a frame that fails to decode is queued
// as decodeSkipped, its opcode and length, and the reader reads on; a
// ServerClose or LeaveWorld that fails is still queued, as its opcode alone.
func TestReadEventsSkipsMalformedFrame(t *testing.T) {
	at := Point{X: 1, Y: 2, Z: 3}
	tests := []struct {
		name    string
		frame   []byte
		want    decodeSkipped
		wantErr error
		then    Event // queued after the skip; nil: the next frame's event
	}{
		{"short frame", packet(opGameStopMove, seed), decodeSkipped{key: opcodeKey{op: opGameStopMove}, length: 5}, wire.ErrShortPacket, nil},
		{"trailing bytes", append(stopMove(5, at, 9), 0), decodeSkipped{key: opcodeKey{op: opGameStopMove}, length: 22}, botwire.ErrTrailingBytes, nil},
		{"extended", packet(opGameExtended, opGameExAutoSoulShot, seed), decodeSkipped{key: opcodeKey{op: opGameExtended, sub: opGameExAutoSoulShot}, length: 7}, wire.ErrShortPacket, nil},
		{"ServerClose", append(packet(opGameServerClose), 0), decodeSkipped{key: opcodeKey{op: opGameServerClose}, length: 2}, botwire.ErrTrailingBytes, ServerClose{}},
		{"LeaveWorld", append(packet(opGameLeaveWorld), 0), decodeSkipped{key: opcodeKey{op: opGameLeaveWorld}, length: 2}, botwire.ErrTrailingBytes, LeaveWorld{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &scriptedSource{frames: [][]byte{tt.frame, stopMove(5, at, 9)}, end: io.EOF}
			q := NewEvents(8)
			if err := readEvents(src, q, make(chan struct{})); !errors.Is(err, io.EOF) {
				t.Fatalf("readEvents = %v, want io.EOF", err)
			}
			want := []Event{tt.want}
			if tt.then != nil {
				want = append(want, tt.then)
			}
			want = append(want, StopMove{ObjectID: 5, Pos: at, Heading: 9})
			got := q.Drain(nil)
			if len(got) != len(want) {
				t.Fatalf("events = %#v, want %#v", got, want)
			}
			skip, ok := got[0].(decodeSkipped)
			if !ok || skip.key != tt.want.key || skip.length != tt.want.length || !errors.Is(skip.err, tt.wantErr) {
				t.Fatalf("first event = %#v, want %#v with %v", got[0], tt.want, tt.wantErr)
			}
			for i := 1; i < len(want); i++ {
				if got[i] != want[i] {
					t.Fatalf("event %d = %#v, want %#v", i, got[i], want[i])
				}
			}
		})
	}
}

// checkSkipLog fails unless log holds want lines of skipped frames, each
// free of the payload seed.
func checkSkipLog(t *testing.T, log string, want int) {
	t.Helper()
	if n := strings.Count(log, "undecodable packet skipped"); n != want {
		t.Errorf("%d skip lines, want %d:\n%s", n, want, log)
	}
	for _, payload := range []string{"5eed", "1592614637", string([]byte{0xed, 0x5e})} {
		if strings.Contains(log, payload) {
			t.Errorf("a log line holds the payload (%q):\n%s", payload, log)
		}
	}
}

// TestBrainSkipsUndecodable: the brain logs the first skipped frame of an
// opcode once, with its opcode and length, counts every one by opcode and
// applies the next event; a ServerClose or LeaveWorld that failed to decode
// still ends the session as a kick or a logout.
func TestBrainSkipsUndecodable(t *testing.T) {
	at := Point{X: 100, Y: 200, Z: -30}
	tests := []struct {
		name    string
		end     []byte // the frame after the skips and the StopMove; nil: none
		wantErr error
		counts  map[string]uint64
		lines   int
	}{
		{"plays on", nil, nil, map[string]uint64{"0x47": 2}, 1},
		{"ServerClose kicks", append(packet(opGameServerClose), 0), errServerClose, map[string]uint64{"0x47": 2, "0x26": 1}, 2},
		{"LeaveWorld logs out", append(packet(opGameLeaveWorld), 0), errLeftWorld, map[string]uint64{"0x47": 2, "0x7e": 1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBrain(t, Persona{})
			var buf bytes.Buffer
			b.log = slog.New(slog.NewTextHandler(&buf, nil))
			reg := botmetrics.NewRegistry()
			b.metrics = reg.NewBot()
			frames := [][]byte{packet(opGameStopMove, seed), packet(opGameStopMove, seed, seed), stopMove(selfID, at, 9)}
			if tt.end != nil {
				frames = append(frames, tt.end)
			}
			q := NewEvents(8)
			if err := readEvents(&scriptedSource{frames: frames, end: io.EOF}, q, make(chan struct{})); !errors.Is(err, io.EOF) {
				t.Fatalf("readEvents = %v", err)
			}
			err := b.handle(q.Drain(nil), t0)
			if tt.wantErr == nil && err != nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("handle = %v, want %v", err, tt.wantErr)
			}
			if got := b.world.Self.Position(t0); got != at {
				t.Errorf("self at %v after the skips, want the StopMove applied at %v", got, at)
			}
			if s := reg.Collect(t0); !maps.Equal(s.DecodeErrors, tt.counts) || s.Aborts != 0 {
				t.Errorf("decode errors %v, aborts %d; want %v, 0", s.DecodeErrors, s.Aborts, tt.counts)
			}
			log := buf.String()
			checkSkipLog(t, log, tt.lines)
			if !strings.Contains(log, "opcode=0x47 length=5 count=1 in=world") {
				t.Errorf("no skip line of the first StopMove:\n%s", log)
			}
		})
	}
}

// TestBrainBurstSkips: NewWorld skips a malformed burst frame, and NewBrain
// logs each skipped opcode once and counts every frame.
func TestBrainBurstSkips(t *testing.T) {
	self := Self{ObjectID: selfID, Name: "bot7"}
	for range 300 {
		self.Burst = append(self.Burst, packet(opGameStopMove, seed))
	}
	self.Burst = append(self.Burst, packet(opGameDie, seed), packet(opGameExtended, opGameExAutoSoulShot, seed))
	w, skips := NewWorld(self, t0)
	var buf bytes.Buffer
	reg := botmetrics.NewRegistry()
	NewBrain(w, NewEvents(4), &recordSender{}, newFakeClock(t0), BrainConfig{
		Metrics: reg.NewBot(), log: slog.New(slog.NewTextHandler(&buf, nil)), burstSkips: skips,
	})
	want := map[string]uint64{"0x47": 300, "0x06": 1, "0xfe:0x0012": 1}
	if s := reg.Collect(t0); !maps.Equal(s.DecodeErrors, want) {
		t.Errorf("decode errors %v, want %v", s.DecodeErrors, want)
	}
	log := buf.String()
	checkSkipLog(t, log, 3)
	if !strings.Contains(log, "opcode=0x47 length=5 count=300 in=burst") {
		t.Errorf("no skip line of the burst's StopMoves:\n%s", log)
	}
}

// newDecodeBot returns a bot of account over n sharing gate, timed by
// clock, logging to log, with a metrics Bot of reg.
func newDecodeBot(t *testing.T, account string, persistent bool, n *fakeNet, gate *LoginGate, clock *fakeClock, reg *botmetrics.Registry, log *bytes.Buffer) *Bot {
	t.Helper()
	n.t = t
	cfg := SessionConfig{Persistent: persistent, Clock: clock, Gate: gate}
	rng := rand.New(rand.NewPCG(1, 2))
	ch, err := NewCharacter(botnames.DefaultMarker, account, rng)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBot(cfg, account, "not-a-secret", BotConfig{Character: ch, Brain: BrainConfig{Metrics: reg.NewBot()}}, rng)
	if err != nil {
		t.Fatal(err)
	}
	b.net = n
	b.log = slog.New(slog.NewTextHandler(log, nil))
	return b
}

// TestSessionDecodeSkips: 300 malformed burst frames do not block play, and
// skipped world frames neither end the session nor count as an abort; each
// opcode is logged once in the session.
func TestSessionDecodeSkips(t *testing.T) {
	clock := newFakeClock(t0)
	gate := NewLoginGate(clock, rand.New(rand.NewPCG(3, 4)))
	reg := botmetrics.NewRegistry()
	bad := packet(opGameStopMove, seed)
	n := &fakeNet{games: []gameScript{{events: []Event{
		decodeSkipped{key: opcodeKey{op: opGameStopMove}, length: len(bad), err: wire.ErrShortPacket},
		decodeSkipped{key: opcodeKey{op: opGameStopMove}, length: len(bad), err: wire.ErrShortPacket},
		ServerClose{},
	}}}}
	for range 300 {
		n.burst = append(n.burst, bad)
	}
	var buf bytes.Buffer
	b := newDecodeBot(t, "bot1", false, n, gate, clock, reg, &buf)
	err := runBot(t.Context(), b, clock)
	if !errors.Is(err, errServerClose) {
		t.Fatalf("Run = %v, want the session ended by its ServerClose", err)
	}
	if s := reg.Collect(clock.Now()); b.aborts != 0 || s.Aborts != 0 || s.DecodeErrors["0x47"] != 302 {
		t.Errorf("aborts %d (metrics %d), decode errors %v; want 0, 0, 302 of 0x47", b.aborts, s.Aborts, s.DecodeErrors)
	}
	checkSkipLog(t, buf.String(), 1)
	checkJoined(t, n)
}

// TestSessionDecodeErrorBeforeWorld: a packet before the world that fails to
// decode (the login server's Init, VersionCheck, the burst's UserInfo) ends
// the attempt with errDecode before the world; the bot retries, logs and
// counts it, never as an abort, and the endings of two bots in a row step
// the fleet backoff.
func TestSessionDecodeErrorBeforeWorld(t *testing.T) {
	clock := newFakeClock(t0)
	// Unspaced, so a turn's second dial needs no clock move.
	gate := NewAllowlistedLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	reg := botmetrics.NewRegistry()
	_, shortErr := botwire.DecodeUserInfo(packet(opGameUserInfo, 1))
	tests := []struct {
		account string
		n       *fakeNet
		packet  string
		steps   int
	}{
		{"bot1", &fakeNet{enters: []error{gameDecodeError("UserInfo", packet(opGameUserInfo, 1), shortErr)}}, "UserInfo", 0},
		{"bot2", &fakeNet{enters: []error{gameDecodeError("UserInfo", packet(opGameUserInfo, 1), shortErr)}}, "UserInfo", 1},
		{"bot3", &fakeNet{logins: []error{loginDecodeError("Init", opLoginInit, 7, errors.New("short"))}}, "Init", 2},
		{"bot1", &fakeNet{dials: []error{gameDecodeError("VersionCheck", packet(0x26), errors.New("not VersionCheck"))}}, "VersionCheck", 3},
	}
	for i, tt := range tests {
		var buf bytes.Buffer
		b := newDecodeBot(t, tt.account, true, tt.n, gate, clock, reg, &buf)
		entered, played, err := b.attempt(t.Context())
		if entered || !errors.Is(err, errDecode) {
			t.Fatalf("attempt %d = entered %v, %v; want errDecode before the world", i, entered, err)
		}
		delay, stop := b.after(entered, played, err)
		if stop != nil || delay <= 0 || b.aborts != 0 {
			t.Fatalf("after %d = %s, %v, aborts %d; want a retry, no abort", i, delay, stop, b.aborts)
		}
		if got := gate.refused.n; got != tt.steps {
			t.Fatalf("attempt %d (%s): %d backoff steps, want %d", i, tt.account, got, tt.steps)
		}
		if log := buf.String(); !strings.Contains(log, "undecodable packet: attempt ends") || !strings.Contains(log, "packet="+tt.packet) {
			t.Errorf("attempt %d: no decode error line of %s:\n%s", i, tt.packet, log)
		}
		clock.Advance(max(gate.next.Sub(clock.Now()), 0))
	}
	want := map[string]uint64{"0x04": 2, "login:0x00": 1, "0x26": 1}
	if s := reg.Collect(clock.Now()); !maps.Equal(s.DecodeErrors, want) || s.Aborts != 0 {
		t.Errorf("decode errors %v, aborts %d; want %v, 0", s.DecodeErrors, s.Aborts, want)
	}
}

// TestEnterGameDecodeErrors: a CharSelectInfo, UserInfo head or whole burst
// UserInfo that fails to decode ends EnterGame with errDecode.
func TestEnterGameDecodeErrors(t *testing.T) {
	const objectID = 0x10000001
	authLogin := recvStep(packet(0x08, "bot7", 4, 3, 1, 2))
	gameStart := []step{
		authLogin,
		sendStep(charSelectInfoPacket(named("bot7")...)),
		recvStep(packet(0x0d, 0, uint16(0), 0, 0, 0)),
		sendStep(packet(0x15, "bot7", objectID)),
		recvStep([]byte{0x03}),
		sendStep(packet(0x16, 0x10000002, 20101)),
	}
	tests := []struct {
		name   string
		script []step
		want   error
	}{
		{"CharSelectInfo", []step{authLogin, sendStep(packet(opGameCharSelectInfo, -1))}, nil},
		{"UserInfo head", append(gameStart[:6:6], sendStep(packet(opGameUserInfo, 1, 2, 3, 4))), wire.ErrShortPacket},
		{"burst UserInfo", append(gameStart[:6:6], sendStep(userInfoHead(1, 2, 3, 4, objectID))), wire.ErrShortPacket},
		{"burst UserInfo trailing", append(gameStart[:6:6], sendStep(append(userInfo(1, 2, 3, 4, objectID), 0))), botwire.ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, done := dialFake(t, true, tt.script, true)
			_, err := EnterGame(t.Context(), g, "bot7", SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}, testCharacter, nil)
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			if !errors.Is(err, errDecode) || tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want errDecode (%v)", err, tt.want)
			}
		})
	}
}

// TestHandshakeLoginDecodeErrors: a VersionCheck or a login Init that fails
// to decode is errDecode.
func TestHandshakeLoginDecodeErrors(t *testing.T) {
	t.Run("VersionCheck", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		s := newFakeServer(server)
		done := make(chan error, 1)
		go func() { done <- s.run([]step{recvStep(packet(0x00, 746)), sendStep(packet(0x00, seed))}, true) }()
		if _, err := newGameConn(t.Context(), client, 746, 5*time.Second); !errors.Is(err, errDecode) {
			t.Fatalf("err = %v, want errDecode", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("fake server: %v", err)
		}
	})
	t.Run("Init", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		s := newFakeServer(server)
		s.padded = true
		done := make(chan error, 1)
		go func() { done <- s.send(encryptInit(t, packet(0x00, 0x1234), 1)) }()
		_, err := login(t.Context(), client, "bot7", "secret", 1, time.Hour)
		client.Close()
		if serr := <-done; serr != nil {
			t.Fatalf("fake server: %v", serr)
		}
		if !errors.Is(err, errDecode) {
			t.Fatalf("err = %v, want errDecode", err)
		}
	})
}
