// Package botpanel is the web control panel of `bots ui` (docs/plan.md
// section 8): a JSON API over the supervisor, server-sent events for live
// updates, the log ring, and a plain HTML and JavaScript page embedded in the
// binary. It serves on loopback only. No password, secret or DSN reaches it:
// it sees the registry, the bots' snapshots, the gate, the metrics and the
// log lines, none of which carries one.
package botpanel

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// Server timeouts. No write timeout: an event stream lasts as long as its
// page; each stream ends with the panel instead.
const (
	readTimeout     = 5 * time.Second
	idleTimeout     = 30 * time.Second
	shutdownTimeout = 5 * time.Second
)

// Defaults of Config.
const (
	// DefaultEvery is how often an event stream sends an update.
	DefaultEvery = 2 * time.Second
	// DefaultCommandWait is how long a command request waits for the bot's
	// answer before it answers "queued".
	DefaultCommandWait = 5 * time.Second
)

//go:embed web
var webFS embed.FS

// Preset is a persona preset the panel offers.
type Preset struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

// Config is what a Panel serves.
type Config struct {
	// Supervisor runs every request on the bots.
	Supervisor *botregistry.Supervisor
	// Gate is the login gate whose breaker the dashboard shows.
	Gate *botclient.LoginGate
	// Metrics is the metrics registry the dashboard shows; its owner
	// collects it.
	Metrics *botmetrics.Registry
	// MaxOnline is the supervisor's online cap, for the dashboard.
	MaxOnline int
	// Presets are the presets offered: the catalog's, those that need the
	// datapack only with it.
	Presets []Preset
	// Logs is the ring the log pages read; nil serves none.
	Logs *Ring
	// Every is the update period of an event stream; zero is DefaultEvery.
	Every time.Duration
	// CommandWait bounds the wait for a command's answer; zero is
	// DefaultCommandWait.
	CommandWait time.Duration
	// Log receives the commands the panel hands the bots; nil discards
	// them.
	Log *slog.Logger
}

// Panel serves the control panel. Its handlers only send the supervisor
// requests and read snapshots.
type Panel struct {
	cfg     Config
	handler http.Handler
	// quit closes when the panel stops: event streams and command waits end.
	quit     chan struct{}
	quitOnce sync.Once
}

// New returns the panel of cfg.
func New(cfg Config) (*Panel, error) {
	if cfg.Supervisor == nil || cfg.Gate == nil || cfg.Metrics == nil {
		return nil, errors.New("panel: supervisor, gate and metrics are required")
	}
	if cfg.Every <= 0 {
		cfg.Every = DefaultEvery
	}
	if cfg.CommandWait <= 0 {
		cfg.CommandWait = DefaultCommandWait
	}
	if cfg.Logs == nil {
		cfg.Logs = NewRing(1)
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	p := &Panel{cfg: cfg, quit: make(chan struct{})}
	web, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(web))
	p.routes(mux)
	// A page of another site the owner's browser shows must not drive the
	// panel: cross-origin writes are refused, and so is a Host that is not
	// loopback (DNS rebinding).
	p.handler = loopbackHost(http.NewCrossOriginProtection().Handler(mux))
	return p, nil
}

// ServeHTTP serves the panel's page and API.
func (p *Panel) ServeHTTP(w http.ResponseWriter, r *http.Request) { p.handler.ServeHTTP(w, r) }

// Stop ends every event stream and command wait; Serve calls it.
func (p *Panel) Stop() { p.quitOnce.Do(func() { close(p.quit) }) }

// Serve serves the panel on ln until ctx ends, then ends its event streams,
// shuts the server down and returns once every handler returned. It closes
// ln. A failure to serve is returned at once.
func (p *Panel) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           p,
		ReadHeaderTimeout: readTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	var err error
	select {
	case serr := <-errc:
		p.Stop()
		return fmt.Errorf("panel serve: %w", serr)
	case <-ctx.Done():
	}
	p.Stop()
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err = srv.Shutdown(sctx); err != nil {
		// Connections still open at the deadline are cut.
		_ = srv.Close()
	}
	if serr := <-errc; !errors.Is(serr, http.ErrServerClosed) {
		err = errors.Join(err, serr)
	}
	return err
}

// Listen opens the panel's listener on addr, which must be a loopback IP
// literal and port ("127.0.0.1:8080", "[::1]:8080"), as the metrics
// endpoint's (botmetrics.Listen).
func Listen(addr string) (net.Listener, error) {
	if err := CheckAddr(addr); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("panel listen: %w", err)
	}
	if a, ok := ln.Addr().(*net.TCPAddr); !ok || !a.IP.IsLoopback() {
		_ = ln.Close()
		return nil, fmt.Errorf("panel listener bound %v, not loopback", ln.Addr())
	}
	return ln, nil
}

// CheckAddr reports why Listen would refuse addr before binding.
func CheckAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("panel address %q: %w", addr, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Unmap().IsLoopback() {
		return fmt.Errorf("panel address %q: want a loopback IP and port such as 127.0.0.1:8080", addr)
	}
	return nil
}

// loopbackHost refuses a request whose Host is not localhost or a loopback
// IP.
func loopbackHost(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		ip, err := netip.ParseAddr(host)
		if host != "localhost" && (err != nil || !ip.Unmap().IsLoopback()) {
			http.Error(w, "the panel answers on localhost only", http.StatusMisdirectedRequest)
			return
		}
		h.ServeHTTP(w, r)
	})
}
