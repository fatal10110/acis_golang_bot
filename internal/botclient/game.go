package botclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang_bot/internal/gameserver/network/cipher"
)

// logoutRefusalGrace is how long Logout waits for LeaveWorld after an
// ActionFailed: one already in flight before the Logout is followed by
// LeaveWorld and the close, a refusal by nothing.
const logoutRefusalGrace = time.Second

// GameConn is the client side of one game-server connection. One goroutine
// may Send while another reads: the cipher keeps separate inbound and
// outbound keys, and both are armed before DialGame returns. Concurrent
// Sends must be serialized by the caller. Read, EnterGame and Logout all read
// the connection and share its frame buffer and inbound cipher state: at most
// one of them may run at a time, so a reader goroutine is stopped and joined
// before EnterGame or Logout, or the call is routed through it.
type GameConn struct {
	conn    net.Conn
	frames  *wire.FrameReader
	cipher  *gamecipher.Cipher // nil when the server left crypt off
	timeout time.Duration
	// logoutGrace is logoutRefusalGrace; tests shorten it.
	logoutGrace time.Duration
	// rec is the frame tap; set before the first read, then only read.
	rec FrameTap
}

// FrameTap sees the decrypted game frames of a connection: Record each
// frame read, on the connection's reader, and RecordSent each frame
// written, before its encryption, on its writer; the two may run at once.
// Neither may keep payload past the call. *Recorder (-record) is one; the
// live e2e harness watches its brain-driven bots through another.
type FrameTap interface {
	Record(payload []byte)
	RecordSent(payload []byte)
}

// SetRecorder makes every later read of the connection (EnterGame, Logout,
// Read and ReadEvents) and every later write pass its decrypted frame to
// rec; nil turns the tap off. Call it while no read or write runs,
// typically right after DialGame, so the handshake's VersionCheck is never
// recorded.
func (g *GameConn) SetRecorder(rec FrameTap) { g.rec = rec }

// DialGame connects to a game server and completes the cleartext
// ProtocolVersion/VersionCheck exchange that arms the connection's cipher.
// Each later Send waits at most timeout; ctx ending fails the handshake at
// once.
func DialGame(ctx context.Context, addr string, revision int32, timeout time.Duration, opts ...DialOption) (*GameConn, error) {
	conn, err := dial(ctx, addr, timeout, opts)
	if err != nil {
		return nil, fmt.Errorf("dial game server: %w", err)
	}
	g, err := newGameConn(ctx, conn, revision, timeout)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return g, nil
}

// newGameConn runs the handshake on an established connection.
func newGameConn(ctx context.Context, conn net.Conn, revision int32, timeout time.Duration) (*GameConn, error) {
	g := &GameConn{conn: conn, frames: wire.NewFrameReader(conn), timeout: timeout, logoutGrace: logoutRefusalGrace}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	if err := g.handshake(ctx, revision); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return g, nil
}

func (g *GameConn) handshake(ctx context.Context, revision int32) error {
	if err := g.send(ctx, encodeProtocolVersion(revision)); err != nil {
		return fmt.Errorf("send ProtocolVersion: %w", closedOnWrite(err))
	}
	raw, err := g.read(ctx, time.Now().Add(g.timeout))
	if err != nil {
		return fmt.Errorf("read VersionCheck: %w", closedOnRead(err))
	}
	key, enabled, err := decodeVersionCheck(raw)
	if err != nil {
		return err
	}
	if enabled {
		c, err := gamecipher.NewCipher(key)
		if err != nil {
			return err
		}
		c.Encrypt(nil) // arms both directions: VersionCheck itself was cleartext
		g.cipher = c
	}
	return nil
}

// Send encrypts a copy of payload and writes it as one frame.
func (g *GameConn) Send(payload []byte) error { return g.send(context.Background(), payload) }

// Read waits until deadline (zero: forever) for one frame and returns it
// decrypted.
func (g *GameConn) Read(deadline time.Time) ([]byte, error) {
	return g.read(context.Background(), deadline)
}

// send and read set the connection's deadline, then check ctx: the
// context.AfterFunc that expires the deadline once ctx ends either ran before
// (ctx.Err is set) or runs after and is not overwritten.
func (g *GameConn) send(ctx context.Context, payload []byte) error {
	if err := g.conn.SetWriteDeadline(time.Now().Add(g.timeout)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.rec != nil {
		g.rec.RecordSent(payload)
	}
	buf := append([]byte(nil), payload...)
	if g.cipher != nil {
		g.cipher.Encrypt(buf)
	}
	return wire.WriteFrame(g.conn, buf)
}

func (g *GameConn) read(ctx context.Context, deadline time.Time) ([]byte, error) {
	if err := g.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := g.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if g.cipher != nil {
		g.cipher.Decrypt(payload)
	}
	if len(payload) == 0 {
		return nil, errors.New("empty game frame")
	}
	if g.rec != nil {
		g.rec.Record(payload)
	}
	return payload, nil
}

// Close closes the connection; a blocked Read returns.
func (g *GameConn) Close() error { return g.conn.Close() }

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

// GameStartError reports a game-server rejection before the world was
// entered: AuthLoginFail or CharCreateFail and its reason code.
type GameStartError struct {
	Packet string
	Reason int32
}

func (e *GameStartError) Error() string {
	return fmt.Sprintf("game server replied %s (reason %d)", e.Packet, e.Reason)
}

// errServerClose reports the server's ServerClose, its last packet before it
// drops the connection.
var errServerClose = errors.New("server sent ServerClose")

// ErrLogoutRefused reports a Logout the server refused (in combat, in a zone
// that forbids it): it answered ActionFailed and kept the connection open.
var ErrLogoutRefused = errors.New("server refused Logout")

// EnterGame authenticates account with the keys its login session issued,
// selects ch's character (creating it when the rules of Character allow)
// and enters the world. chosen, when set, is called on the caller's
// goroutine with the character's name and class id once its slot is chosen
// or it is created, before RequestGameStart: a later failure of the entry
// does not lose it. An invalid ch fails before anything is sent. ctx ending
// fails the entry at once. It returns once the burst's UserInfo arrives,
// with the burst frames before it; the rest of the burst is left unread.
func EnterGame(ctx context.Context, g *GameConn, account string, keys SessionKeys, ch Character, chosen func(name string, classID int32)) (Self, error) {
	if err := ch.Validate(); err != nil {
		return Self{}, err
	}
	stop := context.AfterFunc(ctx, func() { _ = g.conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	self, err := g.enterGame(ctx, account, keys, ch, chosen)
	if err != nil && ctx.Err() != nil {
		return Self{}, ctx.Err()
	}
	return self, err
}

func (g *GameConn) enterGame(ctx context.Context, account string, keys SessionKeys, ch Character, chosen func(string, int32)) (Self, error) {
	if err := g.send(ctx, encodeAuthLogin(account, keys)); err != nil {
		return Self{}, fmt.Errorf("send AuthLogin: %w", err)
	}
	reply, err := g.readUntil(ctx, opGameCharSelectInfo, opGameAuthLoginFail)
	if err != nil {
		return Self{}, fmt.Errorf("read CharSelectInfo: %w", err)
	}
	if reply[0] == opGameAuthLoginFail {
		return Self{}, &GameStartError{Packet: "AuthLoginFail", Reason: wire.NewPacketReader(reply).ReadInt32()}
	}
	slots, err := decodeCharSelectInfo(reply)
	if err != nil {
		return Self{}, err
	}
	slot, slots, err := g.chooseSlot(ctx, slots, ch)
	if err != nil {
		return Self{}, err
	}
	s := slots[slot]
	name := s.Name
	if chosen != nil {
		chosen(name, s.ClassID)
	}

	if err := g.send(ctx, encodeRequestGameStart(slot)); err != nil {
		return Self{}, fmt.Errorf("send RequestGameStart: %w", err)
	}
	selected, err := g.readUntil(ctx, opGameCharSelected)
	if err != nil {
		return Self{}, fmt.Errorf("read CharSelected: %w", err)
	}
	if got := wire.NewPacketReader(selected).ReadString(); got != name {
		return Self{}, fmt.Errorf("selected slot %d as %q, server selected %q", slot, name, got)
	}
	if err := g.send(ctx, encodeEnterWorld()); err != nil {
		return Self{}, fmt.Errorf("send EnterWorld: %w", err)
	}
	var burst [][]byte
	deadline := time.Now().Add(g.timeout)
	for {
		raw, err := g.read(ctx, deadline)
		if err != nil {
			return Self{}, fmt.Errorf("read UserInfo: %w", err)
		}
		switch raw[0] {
		case opGameUserInfo:
			self, err := decodeUserInfo(raw)
			if err != nil {
				return Self{}, err
			}
			self.Name = name
			self.Slots = len(slots)
			self.Burst = burst
			self.UserInfo = append([]byte(nil), raw...)
			return self, nil
		case opGameServerClose:
			return Self{}, errServerClose
		}
		burst = append(burst, append([]byte(nil), raw...))
	}
}

// Logout sends Logout and reads until the server's LeaveWorld, the last
// packet it sends before it closes the connection; frames before it are
// skipped. An ActionFailed followed by no LeaveWorld within
// logoutRefusalGrace is ErrLogoutRefused: the character stays in the world.
// The connection closing without LeaveWorld, or no answer within the
// timeout, is an error too. ctx ending fails the logout at once. The caller
// closes the connection in every case. Logout reads the connection itself:
// no other goroutine may be in Read while it runs (see GameConn).
func (g *GameConn) Logout(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() { _ = g.conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	if err := g.send(ctx, encodeLogout()); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("send Logout: %w", err)
	}
	deadline := time.Now().Add(g.timeout)
	refused := false
	for {
		raw, err := g.read(ctx, deadline)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil && refused && errors.Is(err, os.ErrDeadlineExceeded):
			return ErrLogoutRefused
		case err != nil:
			return fmt.Errorf("read LeaveWorld: %w", err)
		}
		switch raw[0] {
		case opGameLeaveWorld:
			return nil
		case opGameServerClose:
			return errServerClose
		case opGameActionFailed:
			if !refused {
				refused = true
				if grace := time.Now().Add(g.logoutGrace); grace.Before(deadline) {
					deadline = grace
				}
			}
		}
	}
}

// readUntil skips frames until one with any of opcodes arrives, all within
// one timeout budget.
func (g *GameConn) readUntil(ctx context.Context, opcodes ...byte) ([]byte, error) {
	deadline := time.Now().Add(g.timeout)
	for {
		reply, err := g.read(ctx, deadline)
		if err != nil {
			return nil, err
		}
		for _, op := range opcodes {
			if reply[0] == op {
				return reply, nil
			}
		}
		if reply[0] == opGameServerClose {
			return nil, errServerClose
		}
	}
}
