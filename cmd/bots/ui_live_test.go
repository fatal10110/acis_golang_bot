package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/e2e"
)

// livePanel is a `bots ui` run in this process against the live target.
type livePanel struct {
	base string
	stop context.CancelFunc
	code chan int
}

func startLivePanel(t *testing.T, args []string, listen string) *livePanel {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	p := &livePanel{base: "http://" + listen, stop: stop, code: make(chan int, 1)}
	go func() { p.code <- runUI(ctx, append(args, "-listen", listen), os.Stderr) }()
	t.Cleanup(func() { p.close(t) })
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get(p.base + "/api/presets")
		if err == nil {
			resp.Body.Close()
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("panel never served: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// close stops the panel, which logs its bots out, and waits for it; later
// calls do nothing.
func (p *livePanel) close(t *testing.T) {
	if p.stop == nil {
		return
	}
	p.stop()
	p.stop = nil
	if code := <-p.code; code != 0 {
		t.Errorf("bots ui exited %d", code)
	}
}

func (p *livePanel) call(t *testing.T, method, path string, body, out any) {
	t.Helper()
	var in bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&in).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, p.base+path, &in)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e bytes.Buffer
		_, _ = e.ReadFrom(resp.Body)
		t.Fatalf("%s %s: %s %s", method, path, resp.Status, e.String())
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

type liveBotView struct {
	Account string `json:"account"`
	Name    string `json:"name"`
	Live    *struct {
		State     string `json:"state"`
		Character string `json:"character"`
	} `json:"live"`
}

// waitOnline waits until account's bot is online and returns its
// character's name.
func (p *livePanel) waitOnline(t *testing.T, ctx context.Context, account string) string {
	t.Helper()
	for {
		var v liveBotView
		p.call(t, http.MethodGet, "/api/bots/"+account, nil, &v)
		if v.Live != nil && v.Live.State == botclient.StateOnline.String() && v.Live.Character != "" {
			return v.Live.Character
		}
		select {
		case <-ctx.Done():
			t.Fatalf("bot %s never online (%+v)", account, v.Live)
		case <-time.After(time.Second):
		}
	}
}

func freeLoopback(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// TestLivePanelBots runs the M6 criteria against the live server (#94, #96,
// #89): from the panel API, two bots of different presets go online, one is
// despawned and spawned again as the same character; a second panel on the
// same data directory logs both back in as the same characters; and the
// first bot's account still lists exactly one character.
func TestLivePanelBots(t *testing.T) {
	tg := e2e.Live(t, e2e.NeedDatapack)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cfg := tg.Config()
	// The panel's bots count toward the run's cap.
	for range 2 {
		release, err := tg.Hold(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	args := []string{
		"-secret-file", cfg.SecretFile, "-data-dir", t.TempDir(), "-prefix", cfg.Prefix + "p",
		"-login", cfg.Login, "-server-id", fmt.Sprint(e2e.ServerID), "-max-online", "2", "-datapack", cfg.Datapack,
	}
	if cfg.Allowlisted {
		args = append(args, "-allowlisted")
	}
	listen := freeLoopback(t)

	p := startLivePanel(t, args, listen)
	var a, b liveBotView
	p.call(t, http.MethodPost, "/api/bots", map[string]string{"preset": "support"}, &a)
	p.call(t, http.MethodPost, "/api/bots", map[string]string{"preset": "town"}, &b)
	nameA, nameB := p.waitOnline(t, ctx, a.Account), p.waitOnline(t, ctx, b.Account)
	t.Logf("%s plays %s (support), %s plays %s (town)", a.Account, nameA, b.Account, nameB)

	p.call(t, http.MethodPost, "/api/bots/"+a.Account+"/despawn", nil, nil)
	for {
		var v liveBotView
		p.call(t, http.MethodGet, "/api/bots/"+a.Account, nil, &v)
		if v.Live == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("bot %s never despawned", a.Account)
		case <-time.After(time.Second):
		}
	}
	p.call(t, http.MethodPost, "/api/bots/"+a.Account+"/spawn", nil, nil)
	if got := p.waitOnline(t, ctx, a.Account); got != nameA {
		t.Fatalf("respawned %s as %q, want %q", a.Account, got, nameA)
	}
	p.close(t)

	p = startLivePanel(t, args, listen)
	if got := p.waitOnline(t, ctx, a.Account); got != nameA {
		t.Fatalf("after the restart %s plays %q, want %q", a.Account, got, nameA)
	}
	if got := p.waitOnline(t, ctx, b.Account); got != nameB {
		t.Fatalf("after the restart %s plays %q, want %q", b.Account, got, nameB)
	}
	p.close(t)

	// The panel's gate spaced its own dials; leave the accept filter's
	// burst window before the run's gate dials.
	time.Sleep(2 * time.Second)
	c, err := tg.EnterWhenFree(ctx, a.Account, botclient.Character{Name: nameA}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Self.Slots != 1 {
		t.Fatalf("account %s lists %d characters, want 1", a.Account, c.Self.Slots)
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("logout: %v", err)
	}
}
