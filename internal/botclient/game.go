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
}

// DialGame connects to a game server and completes the cleartext
// ProtocolVersion/VersionCheck exchange that arms the connection's cipher.
// Each later Send waits at most timeout; ctx ending fails the handshake at
// once.
func DialGame(ctx context.Context, addr string, revision int32, timeout time.Duration) (*GameConn, error) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
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
		return fmt.Errorf("send ProtocolVersion: %w", err)
	}
	raw, err := g.read(ctx, time.Now().Add(g.timeout))
	if err != nil {
		return fmt.Errorf("read VersionCheck: %w", err)
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
	return payload, nil
}

// Close closes the connection; a blocked Read returns.
func (g *GameConn) Close() error { return g.conn.Close() }

// Self is the bot's own character as the EnterWorld burst described it.
type Self struct {
	ObjectID int32
	X, Y, Z  int32
	Heading  int32
	// Burst holds copies of the EnterWorld frames that preceded UserInfo
	// (objects already in sight among them), in arrival order.
	Burst [][]byte
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
// selects the first character (creating charName, a human fighter, when the
// account has none) and enters the world. ctx ending fails the entry at
// once. It returns once the burst's UserInfo arrives, with the burst frames
// before it; the rest of the burst is left unread.
func EnterGame(ctx context.Context, g *GameConn, account string, keys SessionKeys, charName string) (Self, error) {
	stop := context.AfterFunc(ctx, func() { _ = g.conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	self, err := g.enterGame(ctx, account, keys, charName)
	if err != nil && ctx.Err() != nil {
		return Self{}, ctx.Err()
	}
	return self, err
}

func (g *GameConn) enterGame(ctx context.Context, account string, keys SessionKeys, charName string) (Self, error) {
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
	if wire.NewPacketReader(reply).ReadInt32() == 0 {
		if err := g.send(ctx, encodeRequestCharacterCreate(charName)); err != nil {
			return Self{}, fmt.Errorf("send RequestCharacterCreate: %w", err)
		}
		reply, err := g.readUntil(ctx, opGameCharCreateOk, opGameCharCreateFail)
		if err != nil {
			return Self{}, fmt.Errorf("read CharCreateOk: %w", err)
		}
		if reply[0] == opGameCharCreateFail {
			return Self{}, &GameStartError{Packet: "CharCreateFail", Reason: wire.NewPacketReader(reply).ReadInt32()}
		}
		if _, err := g.readUntil(ctx, opGameCharSelectInfo); err != nil {
			return Self{}, fmt.Errorf("read CharSelectInfo after create: %w", err)
		}
	}

	if err := g.send(ctx, encodeRequestGameStart(0)); err != nil {
		return Self{}, fmt.Errorf("send RequestGameStart: %w", err)
	}
	if _, err := g.readUntil(ctx, opGameCharSelected); err != nil {
		return Self{}, fmt.Errorf("read CharSelected: %w", err)
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
			self.Burst = burst
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
