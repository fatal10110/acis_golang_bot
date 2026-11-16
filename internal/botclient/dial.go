package botclient

import (
	"context"
	"fmt"
	"net"
	"time"
)

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

// dial opens a TCP connection to addr, giving up after timeout.
func dial(ctx context.Context, addr string, timeout time.Duration, opts []DialOption) (net.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	for _, o := range opts {
		if o != nil {
			o(d)
		}
	}
	return d.DialContext(ctx, "tcp", addr)
}
