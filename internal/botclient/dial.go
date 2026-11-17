package botclient

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"
)

// A dead line (a PC asleep, a home IP change) ends a session within about a
// minute. Linux sends no keepalive probe while sent data is unacknowledged,
// and retransmits for about 15 minutes (tcp_retries2), so the dialer sets
// both: keepalive probes for an idle connection, and the user timeout
// (Linux TCP_USER_TIMEOUT, lineSockopts) for unacknowledged data. Either one
// ends the connection with ETIMEDOUT (deadLine), which is no abort.
const (
	keepAliveIdle     = 30 * time.Second
	keepAliveInterval = 10 * time.Second
	keepAliveCount    = 3
)

// sockopt is an integer socket option the dialer sets on every connection.
type sockopt struct{ level, name, value int }

// setsockoptFunc sets one integer socket option on fd: setsockopt, or a
// test's recorder.
type setsockoptFunc func(fd uintptr, level, name, value int) error

// DialOption adjusts how Login, CheckCredentials and DialGame open their
// connection. With no option the OS picks the source address.
type DialOption func(*net.Dialer)

// WithLocalAddr binds the connection's source to ip, with an OS-chosen port.
// A deployment gives the bots one dedicated source address (127.0.0.2) so the
// servers' accept-filter allowlist names only it; the address must be local to
// the host and of the same family as the destination.
func WithLocalAddr(ip net.IP) DialOption {
	return func(d *net.Dialer) { d.LocalAddr = &net.TCPAddr{IP: ip} }
}

// ParseLocalAddr parses a -local-addr flag value into a WithLocalAddr option.
// The empty string means no option.
func ParseLocalAddr(s string) (DialOption, error) {
	if s == "" {
		return nil, nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("local address %q: want an IP address", s)
	}
	return WithLocalAddr(ip), nil
}

// newDialer returns the dialer of every connection: timeout bounds the
// connect, and the keepalive and lineSockopts, set through set, end a
// connection on a dead line.
func newDialer(timeout time.Duration, set setsockoptFunc) *net.Dialer {
	d := &net.Dialer{
		Timeout: timeout,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     keepAliveIdle,
			Interval: keepAliveInterval,
			Count:    keepAliveCount,
		},
	}
	if len(lineSockopts) > 0 {
		d.Control = controlSockopts(lineSockopts, set)
	}
	return d
}

// controlSockopts returns a Dialer.Control that sets opts on the socket
// before it connects.
func controlSockopts(opts []sockopt, set setsockoptFunc) func(network, address string, c syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			for _, o := range opts {
				if err := set(fd, o.level, o.name, o.value); err != nil {
					serr = fmt.Errorf("set socket option %d/%#x to %d: %w", o.level, o.name, o.value, err)
					return
				}
			}
		})
		if err != nil {
			return err
		}
		return serr
	}
}

// dial opens a TCP connection to addr, giving up after timeout.
func dial(ctx context.Context, addr string, timeout time.Duration, opts []DialOption) (net.Conn, error) {
	d := newDialer(timeout, setsockopt)
	for _, o := range opts {
		if o != nil {
			o(d)
		}
	}
	return d.DialContext(ctx, "tcp", addr)
}
