package botpanel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botaccounts"
	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/botregistry"
)

// secretText is the test's secret file content: no response, page or log
// line may carry it, nor a password derived from it.
const secretText = "panel-test-secret-Xq7Lw9Zr2Vb5Nt8K"

// run is one run the supervisor started: its bot is handed over, and the
// test ends it.
type run struct {
	botregistry.Run
	bot *botclient.Bot
	ctx context.Context
	end chan error
}

// rig is a panel over a running supervisor whose runs are fake: each builds
// its bot as `bots ui` does, hands it over, and plays nothing.
type rig struct {
	t      *testing.T
	srv    *httptest.Server
	panel  *Panel
	reg    *botregistry.Registry
	sup    *botregistry.Supervisor
	runs   chan *run
	secret botaccounts.Secret
	ring   *Ring
	gate   *botclient.LoginGate
	stderr *lockedBuffer
	cancel context.CancelFunc
	done   chan error
	// bodies are every response body read, for the secret check.
	mu     sync.Mutex
	bodies []string
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func newRig(t *testing.T, cfg Config) *rig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(secretText+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := botaccounts.LoadSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := botregistry.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	if err := reg.SetPanel("hz", botnames.DefaultMarker); err != nil {
		t.Fatal(err)
	}
	g := &rig{t: t, reg: reg, runs: make(chan *run), secret: secret, ring: NewRing(1000), stderr: &lockedBuffer{}, done: make(chan error, 1)}
	log := slog.New(g.ring.Handler(slog.NewTextHandler(g.stderr, nil), nil))
	clock := botclient.SystemClock{}
	gate := botclient.NewLoginGate(clock, rand.New(rand.NewPCG(1, 2)))
	g.gate = gate
	metrics := botmetrics.NewRegistry()
	presets := []string{"support", "town"}
	sup, err := botregistry.New(botregistry.Config{
		Registry:  reg,
		Run:       g.run(log),
		Gate:      gate,
		Clock:     clock,
		MaxOnline: 3,
		Metrics:   metrics,
		CheckPreset: func(p string) error {
			if !slices.Contains(presets, p) {
				return fmt.Errorf("preset %q needs -datapack", p)
			}
			return nil
		},
		Rand: rand.New(rand.NewPCG(3, 4)),
		Log:  log,
	})
	if err != nil {
		t.Fatal(err)
	}
	g.sup = sup
	ctx, cancel := context.WithCancel(t.Context())
	g.cancel = cancel
	go func() { g.done <- sup.Run(ctx) }()
	cfg.Supervisor, cfg.Gate, cfg.Metrics, cfg.Logs, cfg.MaxOnline, cfg.Log = sup, gate, metrics, g.ring, 3, log
	if cfg.Presets == nil {
		cfg.Presets = []Preset{{Name: "support", Help: "farms"}, {Name: "town", Help: "idles"}}
	}
	g.panel, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	g.srv = httptest.NewServer(g.panel)
	return g
}

// run is the supervisor's RunFunc: it builds the bot as `bots ui` does (the
// password derived from the secret), hands it over and waits for its end.
// Once canceled, it runs the bot on its done context, which stops it and
// refuses the commands left in its mailbox.
func (g *rig) run(log *slog.Logger) botregistry.RunFunc {
	return func(ctx context.Context, r botregistry.Run) error {
		cfg := botclient.SessionConfig{Clock: botclient.SystemClock{}, Gate: g.gate, Log: log, Persistent: true}
		b, err := botclient.NewBot(cfg, r.Account, g.secret.Password(r.Account), botclient.BotConfig{Character: r.Character, Preset: r.Preset, Learned: r.Learned}, rand.New(rand.NewPCG(5, 6)))
		if err != nil {
			return err
		}
		r.Live(b)
		fr := &run{Run: r, bot: b, ctx: ctx, end: make(chan error)}
		g.runs <- fr
		err = <-fr.end
		if ctx.Err() != nil {
			_ = b.Run(ctx)
		}
		return err
	}
}

func (g *rig) next() *run {
	g.t.Helper()
	select {
	case r := <-g.runs:
		return r
	case err := <-g.done:
		g.t.Fatalf("supervisor returned %v while a run was awaited", err)
	}
	return nil
}

// endOnCancel ends r once the supervisor canceled it.
func endOnCancel(r *run) {
	<-r.ctx.Done()
	r.end <- nil
}

// stop stops the panel's supervisor, ending each live run.
func (g *rig) stop(live ...*run) {
	g.t.Helper()
	g.panel.Stop()
	g.srv.Close()
	g.cancel()
	for _, r := range live {
		endOnCancel(r)
	}
	if err := <-g.done; err != nil {
		g.t.Fatalf("supervisor Run = %v", err)
	}
}

// do sends a request and returns its status and body.
func (g *rig) do(method, path string, body any) (int, string) {
	g.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			g.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(g.t.Context(), method, g.srv.URL+path, rd)
	if err != nil {
		g.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := g.srv.Client().Do(req)
	if err != nil {
		g.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		g.t.Fatal(err)
	}
	g.mu.Lock()
	g.bodies = append(g.bodies, string(b))
	g.mu.Unlock()
	return res.StatusCode, string(b)
}

// want sends a request and fails unless it answers code.
func (g *rig) want(code int, method, path string, body any) string {
	g.t.Helper()
	got, b := g.do(method, path, body)
	if got != code {
		g.t.Fatalf("%s %s = %d %s, want %d", method, path, got, b, code)
	}
	return b
}

func (g *rig) bots() map[string]botView {
	g.t.Helper()
	var l []botView
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/bots", nil)), &l); err != nil {
		g.t.Fatal(err)
	}
	m := make(map[string]botView, len(l))
	for _, v := range l {
		m[v.Account] = v
	}
	return m
}

// settle waits until account's run left state from: its report was handled.
func (g *rig) settle(account string, from botregistry.RunState) botView {
	g.t.Helper()
	for range 10000 {
		if v := g.bots()[account]; v.Run != from {
			return v
		}
	}
	g.t.Fatalf("bot %s stays %s", account, from)
	return botView{}
}

// noSecret fails when a response body, a ring line or stderr carries the
// secret or a password of the registry's accounts.
func (g *rig) noSecret() {
	g.t.Helper()
	needles := []string{secretText}
	for account := range g.bots() {
		needles = append(needles, g.secret.Password(account))
	}
	var hay []string
	g.mu.Lock()
	hay = append(hay, g.bodies...)
	g.mu.Unlock()
	for _, l := range g.ring.Lines("", 0) {
		hay = append(hay, l.Text)
	}
	hay = append(hay, g.stderr.String())
	page := g.want(http.StatusOK, "GET", "/", nil) + g.want(http.StatusOK, "GET", "/app.js", nil)
	hay = append(hay, page)
	for _, h := range hay {
		for _, n := range needles {
			if strings.Contains(h, n) {
				g.t.Fatalf("a secret reached the panel: %q", h)
			}
		}
	}
	if len(g.ring.Lines("", 0)) == 0 {
		g.t.Fatal("no log line to check")
	}
}

func TestPanelCreateSpawnDespawnPreset(t *testing.T) {
	g := newRig(t, Config{})

	g.want(http.StatusBadRequest, "POST", "/api/bots", map[string]any{"preset": "merchant"})
	g.want(http.StatusBadRequest, "POST", "/api/bots", map[string]any{"preset": "support", "class": 1})
	g.want(http.StatusBadRequest, "POST", "/api/bots", map[string]any{"preset": "support", "secret": "x"})
	var b1, b2 botregistry.Bot
	if err := json.Unmarshal([]byte(g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})), &b1); err != nil {
		t.Fatal(err)
	}
	r1 := g.next()
	if err := json.Unmarshal([]byte(g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "town", "class": 18})), &b2); err != nil {
		t.Fatal(err)
	}
	r2 := g.next()
	if b1.Account != "hz1" || b2.Account != "hz2" || r1.Preset != "support" || r2.Preset != "town" || *b2.Class != 18 {
		t.Fatalf("created %+v (run %s) and %+v (run %s)", b1, r1.Preset, b2, r2.Preset)
	}
	r1.Learned("AldenBot", *b1.Class)
	bots := g.bots()
	if v := bots["hz1"]; v.Run != botregistry.RunActive || v.Live == nil || v.Live.State != "new" || v.Live.Preset != "support" || v.Name != "AldenBot" {
		t.Fatalf("hz1 %+v live %+v", v, v.Live)
	}
	var one botView
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/bots/hz2", nil)), &one); err != nil || one.Preset != "town" {
		t.Fatalf("GET hz2 = %+v, %v", one, err)
	}
	g.want(http.StatusNotFound, "GET", "/api/bots/hz9", nil)

	// Despawn hz1 and spawn it again: the same character.
	g.want(http.StatusOK, "POST", "/api/bots/hz1/despawn", nil)
	endOnCancel(r1)
	if v := g.settle("hz1", botregistry.RunStopping); v.Online || v.Live != nil || v.StopReason != "despawned" {
		t.Fatalf("despawned hz1 %+v", v)
	}
	g.want(http.StatusOK, "POST", "/api/bots/hz1/spawn", nil)
	r1 = g.next()
	if r1.Account != "hz1" || r1.Character.Name != "AldenBot" {
		t.Fatalf("respawned run %+v, want hz1 playing AldenBot", r1.Run)
	}
	g.want(http.StatusConflict, "POST", "/api/bots/hz1/spawn", nil)

	// A persona change logs hz2 out and back in with the new preset.
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz2/preset", map[string]any{"preset": "merchant"})
	g.want(http.StatusOK, "POST", "/api/bots/hz2/preset", map[string]any{"preset": "support"})
	endOnCancel(r2)
	r2 = g.next()
	if r2.Account != "hz2" || r2.Preset != "support" {
		t.Fatalf("run after the persona change %+v", r2.Run)
	}

	// Delete wants a despawned bot.
	g.want(http.StatusConflict, "DELETE", "/api/bots/hz2", nil)
	g.want(http.StatusOK, "POST", "/api/bots/hz2/despawn", nil)
	endOnCancel(r2)
	g.settle("hz2", botregistry.RunStopping)
	g.want(http.StatusOK, "DELETE", "/api/bots/hz2", nil)
	if _, ok := g.bots()["hz2"]; ok {
		t.Fatal("deleted hz2 still listed")
	}
	g.want(http.StatusNotFound, "DELETE", "/api/bots/hz2", nil)

	// Import and the dashboard.
	g.want(http.StatusBadRequest, "POST", "/api/import", map[string]any{"prefix": "hy", "count": 0})
	g.want(http.StatusBadRequest, "POST", "/api/import", map[string]any{"prefix": "hy", "count": 2, "marker_at": "middle"})
	if b := g.want(http.StatusOK, "POST", "/api/import", map[string]any{"prefix": "hy", "count": 2}); !strings.Contains(b, `"added":2`) {
		t.Fatalf("import answered %s", b)
	}
	if v := g.bots()["hy2"]; !v.NeverCreate || v.Online || v.Preset != botregistry.ImportPreset {
		t.Fatalf("imported hy2 %+v", v)
	}
	var d dashboardView
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/dashboard", nil)), &d); err != nil {
		t.Fatal(err)
	}
	if d.Halted || d.Bots != 3 || d.Running != 1 || d.MaxOnline != 3 || d.Metrics == nil {
		t.Fatalf("dashboard %+v", d)
	}
	g.want(http.StatusOK, "POST", "/api/resume-logins", nil)
	var p struct {
		Presets []Preset `json:"presets"`
		Classes []int32  `json:"classes"`
	}
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/presets", nil)), &p); err != nil || len(p.Presets) != 2 || len(p.Classes) != 9 {
		t.Fatalf("presets %+v, %v", p, err)
	}

	// The bots' log lines are kept per bot.
	for _, l := range g.ring.Lines("hz1", 0) {
		if l.Account != "hz1" {
			t.Fatalf("line of %q in hz1's log", l.Account)
		}
	}
	if len(g.ring.Lines("hz1", 0)) == 0 {
		t.Fatal("no log line of hz1")
	}
	var lines []Line
	if err := json.Unmarshal([]byte(g.want(http.StatusOK, "GET", "/api/logs?account=hz2&n=2", nil)), &lines); err != nil || len(lines) != 2 || lines[1].Account != "hz2" {
		t.Fatalf("hz2's last two log lines %+v, %v", lines, err)
	}
	g.want(http.StatusBadRequest, "GET", "/api/logs?n=x", nil)
	g.noSecret()
	g.stop(r1)
}

func TestPanelCommands(t *testing.T) {
	g := newRig(t, Config{CommandWait: time.Nanosecond})
	g.want(http.StatusNotFound, "POST", "/api/bots/hz1/command", map[string]any{"command": "stay"})
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r := g.next()

	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/command", map[string]any{"command": "dance"})
	g.want(http.StatusBadRequest, "POST", "/api/bots/hz1/command", map[string]any{"command": "go"})
	// No session runs: the commands wait in the mailbox until it is full.
	var queued int
	for {
		code, body := g.do("POST", "/api/bots/hz1/command", map[string]any{"command": "go Wolves"})
		if code == http.StatusConflict {
			if !strings.Contains(body, botclient.ErrBusy.Error()) {
				t.Fatalf("full mailbox answered %s", body)
			}
			break
		}
		if code != http.StatusAccepted || !strings.Contains(body, "queued") {
			t.Fatalf("command answered %d %s, want queued", code, body)
		}
		if queued++; queued > 100 {
			t.Fatal("the mailbox never fills")
		}
	}
	// The bot stops: the commands left in its mailbox are refused.
	g.want(http.StatusOK, "POST", "/api/bots/hz1/despawn", nil)
	endOnCancel(r)
	g.settle("hz1", botregistry.RunStopping)
	if _, err := g.sup.Command(t.Context(), "hz1", botclient.Command{Kind: botclient.CommandStay}); err == nil {
		t.Fatal("command to a despawned bot taken")
	}
	g.want(http.StatusConflict, "POST", "/api/bots/hz1/command", map[string]any{"command": "stay"})
	if got := r.bot.Status().State; got != botclient.StateStopped {
		t.Fatalf("despawned bot %s, want stopped", got)
	}
	g.noSecret()
	g.stop()
}

func TestPanelCommandAnswer(t *testing.T) {
	g := newRig(t, Config{CommandWait: time.Hour})
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r := g.next()
	type answer struct {
		code int
		body string
	}
	got := make(chan answer, 1)
	go func() {
		code, body := g.do("POST", "/api/bots/hz1/command", map[string]any{"command": "follow Alden"})
		got <- answer{code, body}
	}()
	// The panel logs the command once the bot's mailbox took it.
	for !slices.ContainsFunc(g.ring.Lines("hz1", 0), func(l Line) bool { return strings.Contains(l.Text, "follow Alden") }) {
		runtime.Gosched()
	}
	// The bot stops before a session carried it out: its answer is
	// ErrStopped, which the request returns as the command's answer.
	g.want(http.StatusOK, "POST", "/api/bots/hz1/despawn", nil)
	endOnCancel(r)
	a := <-got
	if a.code != http.StatusUnprocessableEntity || !strings.Contains(a.body, botclient.ErrStopped.Error()) {
		t.Fatalf("command answered %d %s, want the bot's refusal", a.code, a.body)
	}
	g.noSecret()
	g.stop()
}

func TestPanelEventsEndWithThePanel(t *testing.T) {
	g := newRig(t, Config{Every: time.Hour})
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() { served <- g.panel.Serve(ctx, ln) }()
	if _, err := g.sup.Create(t.Context(), "support", nil); err != nil {
		t.Fatal(err)
	}
	r := g.next()

	req, err := http.NewRequestWithContext(t.Context(), "GET", "http://"+ln.Addr().String()+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(nil, 1<<20)
	var ev, data string
	for sc.Scan() && sc.Text() != "" {
		k, v, _ := strings.Cut(sc.Text(), ": ")
		switch k {
		case "event":
			ev = v
		case "data":
			data = v
		}
	}
	var u update
	if err := json.Unmarshal([]byte(data), &u); err != nil || ev != "update" {
		t.Fatalf("event %q data %q: %v", ev, data, err)
	}
	if len(u.Bots) != 1 || u.Bots[0].Account != "hz1" || u.Bots[0].Live == nil || u.Dashboard.Running != 1 {
		t.Fatalf("update %+v", u)
	}
	// Stopping the panel ends the stream, though its next update is an
	// hour away, and Serve returns once its handler did.
	cancel()
	for sc.Scan() {
		t.Fatalf("stream went on after the panel stopped: %q", sc.Text())
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
	g.stop(r)
}

func TestPanelRefusesOtherSites(t *testing.T) {
	g := newRig(t, Config{})
	tests := []struct {
		name   string
		method string
		header map[string]string
		host   string
		want   int
	}{
		{"same origin page", "GET", nil, "", http.StatusOK},
		{"localhost by name", "GET", nil, "localhost:1", http.StatusOK},
		{"rebound host name", "GET", nil, "evil.example:80", http.StatusMisdirectedRequest},
		{"cross-site write", "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, "", http.StatusForbidden},
		{"cross-origin write", "POST", map[string]string{"Origin": "https://evil.example"}, "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/bots"
			req, err := http.NewRequestWithContext(t.Context(), tt.method, g.srv.URL+path, strings.NewReader(`{"preset":"support"}`))
			if err != nil {
				t.Fatal(err)
			}
			if tt.host != "" {
				req.Host = tt.host
			}
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			res, err := g.srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != tt.want {
				t.Fatalf("status %d, want %d", res.StatusCode, tt.want)
			}
		})
	}
	if l := g.bots(); len(l) != 0 {
		t.Fatalf("a refused request created %+v", l)
	}
	g.stop()
}

func TestCheckAddr(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:8090", true},
		{"[::1]:8090", true},
		{"127.0.0.2:0", true},
		{":8090", false},
		{"0.0.0.0:8090", false},
		{"[::]:8090", false},
		{"localhost:8090", false},
		{"192.168.1.5:8090", false},
		{"127.0.0.1", false},
	}
	for _, tt := range tests {
		if err := CheckAddr(tt.addr); (err == nil) != tt.ok {
			t.Errorf("CheckAddr(%q) = %v, want ok %v", tt.addr, err, tt.ok)
		}
	}
	if ln, err := Listen("0.0.0.0:0"); err == nil {
		_ = ln.Close()
		t.Fatal("Listen bound every interface")
	}
}
