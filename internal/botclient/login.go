package botclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	logincrypt "github.com/fatal10110/acis_golang_bot/internal/loginserver/crypt"
)

// LoginResult is what a completed login-server session yields.
type LoginResult struct {
	Keys SessionKeys
	// GameAddr is the address the chosen server advertised.
	GameAddr string
	// Connected is when the TCP connection and Init completed.
	Connected time.Time
}

// ErrClosedAtAccept reports a connection the server closed before its first
// frame: a login or game dial that connected, then ended before the server's
// first byte. A server closes a connection at accept for the accept filter's ban
// (server internal/commons/netutil/acceptfilter.go, filteredListener.Accept),
// the login server's ban list (credential and admin bans, internal/
// loginserver/clientlink.go, before Init) and the game listener's ban list
// (internal/commons/netutil/banlist.go, RefuseBanned); a server that fails
// between accept and its first write looks the same. The login gate goes
// quiet for banQuiet on it.
var ErrClosedAtAccept = errors.New("closed at accept")

// closedOnRead marks err, from a connection's first read, as
// ErrClosedAtAccept when the server closed it before any byte: EOF (no
// byte of a frame header came) or a reset. A timeout or a partial frame is
// not.
func closedOnRead(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		return fmt.Errorf("%w: %w", ErrClosedAtAccept, err)
	}
	return err
}

// closedOnWrite marks err, from a connection's first write, as
// ErrClosedAtAccept when the server closed it before reading: a broken pipe
// or a reset. A timeout is not.
func closedOnWrite(err error) error {
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return fmt.Errorf("%w: %w", ErrClosedAtAccept, err)
	}
	return err
}

// LoginError reports a login-server rejection: the reply (LoginFail,
// PlayFail or AccountKicked) and its reason code.
type LoginError struct {
	Packet string
	Reason int32
}

func (e *LoginError) Error() string {
	return fmt.Sprintf("login server replied %s (reason %d)", e.Packet, e.Reason)
}

// loginConn is the client side of one login-server connection.
type loginConn struct {
	conn    net.Conn
	frames  *wire.FrameReader
	crypt   *logincrypt.LoginCrypt
	timeout time.Duration
}

// send and read set the connection's deadline, then check ctx: the
// context.AfterFunc that expires the deadline once ctx ends either ran before
// (ctx.Err is set) or runs after and is not overwritten.
func (c *loginConn) send(ctx context.Context, payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

func (c *loginConn) read(ctx context.Context) ([]byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := c.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if err := c.crypt.Decrypt(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// Login runs one login-server session for account: Init, GameGuard,
// credentials, the server list and server selection. With no ServerList
// entry for serverID it fails. Each read and write waits at most timeout;
// ctx ending fails the session at once. The connection is closed on return.
func Login(ctx context.Context, addr, account, password string, serverID byte, timeout time.Duration, opts ...DialOption) (LoginResult, error) {
	conn, err := dial(ctx, addr, timeout, opts)
	if err != nil {
		return LoginResult{}, fmt.Errorf("dial login server: %w", err)
	}
	defer conn.Close()
	return login(ctx, conn, account, password, serverID, timeout)
}

// login runs the login session on an established connection. Once ctx has
// ended, any failure is reported as ctx.Err().
func login(ctx context.Context, conn net.Conn, account, password string, serverID byte, timeout time.Duration) (LoginResult, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	res, err := loginSession(ctx, conn, account, password, serverID, timeout)
	if err != nil && ctx.Err() != nil {
		return LoginResult{}, ctx.Err()
	}
	return res, err
}

// CheckCredentials runs a login session for account up to the ServerList and
// closes it without choosing a server: with AutoCreateAccounts on, the first
// one creates the account with password, and every later one verifies it. A
// refused credential is a *LoginError; the login server counts it toward its
// per-IP ban. With no play keys issued, the server drops the session at once
// (loginserver/clientlink.go releaseSession), so a game login right after
// does not meet AccountInUse. Each read and write waits at most timeout.
func CheckCredentials(ctx context.Context, addr, account, password string, timeout time.Duration, opts ...DialOption) error {
	conn, err := dial(ctx, addr, timeout, opts)
	if err != nil {
		return fmt.Errorf("dial login server: %w", err)
	}
	defer conn.Close()
	return checkCredentials(ctx, conn, account, password, timeout)
}

// checkCredentials runs CheckCredentials on an established connection. Once
// ctx has ended, any failure is reported as ctx.Err().
func checkCredentials(ctx context.Context, conn net.Conn, account, password string, timeout time.Duration) error {
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	_, _, _, err := authenticate(ctx, conn, account, password, timeout)
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// authenticate runs a login session up to the ServerList: Init, GameGuard,
// the credentials and, when the server shows the licence, the list request.
// It returns the connection's crypt state for the server selection that
// follows.
func authenticate(ctx context.Context, conn net.Conn, account, password string, timeout time.Duration) (*loginConn, LoginResult, []ServerEntry, error) {
	var res LoginResult
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, res, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, res, nil, err
	}
	frames := wire.NewFrameReader(conn)
	first, err := frames.ReadFrame()
	if err != nil {
		return nil, res, nil, fmt.Errorf("read Init: %w", closedOnRead(err))
	}
	init, err := botwire.DecodeInit(first)
	if err != nil {
		return nil, res, nil, loginDecodeError("Init", opLoginInit, len(first), err)
	}
	cr, err := logincrypt.NewClientLoginCrypt(init.BlowfishKey)
	if err != nil {
		return nil, res, nil, err
	}
	res.Connected = time.Now()
	c := &loginConn{conn: conn, frames: frames, crypt: cr, timeout: timeout}

	if err := c.send(ctx, botwire.EncodeAuthGameGuard(init.SessionID)); err != nil {
		return nil, res, nil, fmt.Errorf("send AuthGameGuard: %w", err)
	}
	reply, err := c.read(ctx)
	if err != nil {
		return nil, res, nil, fmt.Errorf("read GGAuth: %w", err)
	}
	if err := expectLoginReply(reply, opLoginGGAuth, "GGAuth"); err != nil {
		return nil, res, nil, err
	}

	if err := c.send(ctx, botwire.EncodeRequestAuthLogin(init.PublicKey, account, password)); err != nil {
		return nil, res, nil, fmt.Errorf("send RequestAuthLogin: %w", err)
	}
	if reply, err = c.read(ctx); err != nil {
		return nil, res, nil, fmt.Errorf("read login reply: %w", err)
	}
	// With ShowLicence off the server answers the credentials with the
	// ServerList itself and does not check the session keys afterwards.
	if len(reply) > 0 && reply[0] == opLoginOk {
		if res.Keys.LoginKey1, res.Keys.LoginKey2, err = botwire.DecodeKeyPair(reply); err != nil {
			return nil, res, nil, loginDecodeError("LoginOk", opLoginOk, len(reply), err)
		}
		if err := c.send(ctx, botwire.EncodeRequestServerList(res.Keys.LoginKey1, res.Keys.LoginKey2)); err != nil {
			return nil, res, nil, fmt.Errorf("send RequestServerList: %w", err)
		}
		if reply, err = c.read(ctx); err != nil {
			return nil, res, nil, fmt.Errorf("read ServerList: %w", err)
		}
	}
	if err := expectLoginReply(reply, opLoginServerList, "ServerList"); err != nil {
		return nil, res, nil, err
	}
	servers, err := botwire.DecodeServerList(reply)
	if err != nil {
		return nil, res, nil, loginDecodeError("ServerList", opLoginServerList, len(reply), err)
	}
	return c, res, servers, nil
}

func loginSession(ctx context.Context, conn net.Conn, account, password string, serverID byte, timeout time.Duration) (LoginResult, error) {
	c, res, servers, err := authenticate(ctx, conn, account, password, timeout)
	if err != nil {
		return res, err
	}
	for _, s := range servers {
		if s.ID == serverID {
			res.GameAddr = s.Addr
		}
	}
	if res.GameAddr == "" {
		return res, fmt.Errorf("server %d is not in the ServerList", serverID)
	}

	if err := c.send(ctx, botwire.EncodeRequestServerLogin(res.Keys.LoginKey1, res.Keys.LoginKey2, serverID)); err != nil {
		return res, fmt.Errorf("send RequestServerLogin: %w", err)
	}
	reply, err := c.read(ctx)
	if err != nil {
		return res, fmt.Errorf("read PlayOk: %w", err)
	}
	if err := expectLoginReply(reply, opLoginPlayOk, "PlayOk"); err != nil {
		return res, err
	}
	if res.Keys.PlayKey1, res.Keys.PlayKey2, err = botwire.DecodeKeyPair(reply); err != nil {
		return res, loginDecodeError("PlayOk", opLoginPlayOk, len(reply), err)
	}
	return res, nil
}

// expectLoginReply maps a LoginFail, PlayFail, AccountKicked or unexpected
// reply to an error.
func expectLoginReply(reply []byte, opcode byte, name string) error {
	if len(reply) == 0 {
		return errors.New("empty login reply")
	}
	r := wire.NewPacketReader(reply)
	switch reply[0] {
	case opcode:
		return nil
	case opLoginFail:
		return &LoginError{Packet: "LoginFail", Reason: r.ReadInt32()}
	case opLoginPlayFail:
		return &LoginError{Packet: "PlayFail", Reason: int32(r.ReadUint8())}
	case opLoginAccountKicked:
		return &LoginError{Packet: "AccountKicked", Reason: r.ReadInt32()}
	}
	return fmt.Errorf("login reply opcode %#x, want %s (%#x)", reply[0], name, opcode)
}
