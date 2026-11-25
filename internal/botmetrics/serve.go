package botmetrics

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"time"
)

// Endpoint timeouts: the endpoint serves a few kilobytes to a local reader.
const (
	readTimeout     = 5 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 30 * time.Second
	shutdownTimeout = 5 * time.Second
)

// VarsPath is where the endpoint serves its variables, as expvar does.
const VarsPath = "/debug/vars"

// Listen opens the metrics listener on addr, which must be a loopback IP
// literal and port ("127.0.0.1:6060", "[::1]:6060"). An empty host, an
// unspecified address, any other IP and any host name are refused, and so
// is a listener the system bound anywhere but loopback.
func Listen(addr string) (net.Listener, error) {
	if err := CheckAddr(addr); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("metrics listen: %w", err)
	}
	if a, ok := ln.Addr().(*net.TCPAddr); !ok || !a.IP.IsLoopback() {
		_ = ln.Close()
		return nil, fmt.Errorf("listener bound %v, not loopback", ln.Addr())
	}
	return ln, nil
}

// CheckAddr reports why Listen would refuse addr before binding: it must be
// a loopback IP literal and port. Flag parsing calls it to fail early.
func CheckAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q: %w", addr, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("address %q: host must be a loopback IP such as 127.0.0.1", addr)
	}
	if !ip.Unmap().IsLoopback() {
		return fmt.Errorf("address %q is not loopback: the endpoint binds 127.0.0.0/8 or ::1 only", addr)
	}
	return nil
}

// Handler serves the registry's latest collection as "bots" and the Go
// runtime's memory statistics as "memstats" at VarsPath, in expvar's
// format. It uses a private expvar.Map: the process-wide expvar variables
// (cmdline holds the flags, the account prefix among them) are not served.
func Handler(r *Registry) http.Handler {
	vars := new(expvar.Map)
	vars.Set("bots", expvar.Func(func() any { return r.Last() }))
	vars.Set("memstats", expvar.Func(func() any {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m
	}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+VarsPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, vars.String())
	})
	return mux
}

// Serve serves h on ln until ctx ends, then shuts the server down and
// returns nil once it stopped. It closes ln. A failure to serve is
// returned at once.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	return serve(ctx, ln, h, "metrics", writeTimeout)
}

// serve is Serve with the endpoint's name for its errors and its response
// write limit (zero is none).
func serve(ctx context.Context, ln net.Listener, h http.Handler, name string, write time.Duration) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      write,
		IdleTimeout:       idleTimeout,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return fmt.Errorf("%s serve: %w", name, err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	err := srv.Shutdown(sctx)
	if err != nil {
		// Connections still open at the deadline are cut.
		_ = srv.Close()
	}
	if serr := <-errc; !errors.Is(serr, http.ErrServerClosed) {
		err = errors.Join(err, serr)
	}
	if err != nil {
		return fmt.Errorf("%s shutdown: %w", name, err)
	}
	return nil
}
