package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

func TestParseSwarmMetricsFlag(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"off by default", nil, "", ""},
		{"loopback", []string{"-metrics", "127.0.0.1:6060"}, "127.0.0.1:6060", ""},
		{"loopback v6", []string{"-metrics", "[::1]:6060"}, "[::1]:6060", ""},
		{"unspecified", []string{"-metrics", "0.0.0.0:6060"}, "", "not loopback"},
		{"empty host", []string{"-metrics", ":6060"}, "", "-metrics"},
		{"host name", []string{"-metrics", "localhost:6060"}, "", "loopback IP"},
		{"no port", []string{"-metrics", "127.0.0.1"}, "", "-metrics"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(append(base, tt.args...), io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || o.metrics != tt.want {
				t.Fatalf("metrics, err = %q, %v; want %q, nil", o.metrics, err, tt.want)
			}
		})
	}
}

func TestRunSwarmMetrics(t *testing.T) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "bots.secret")
	if err := os.WriteFile(file, []byte(hex.EncodeToString(secret)), 0o600); err != nil {
		t.Fatal(err)
	}
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	geo := testGeoArgs(t)
	tests := []struct {
		name    string
		metrics string
		code    int
		want    string
	}{
		{"not loopback", "0.0.0.0:6060", 2, "not loopback"},
		{"port taken", taken.Addr().String(), 1, "metrics: listen"},
		// A signal before any turn of the gate: the endpoint starts, every
		// bot returns at once, and the endpoint stops.
		{"serves until the bots return", "127.0.0.1:0", 0, "metrics serve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			// The login address is unroutable: nothing may reach a dial.
			args := append([]string{"swarm", "-login", "192.0.2.1:1", "-timeout", "1ms", "-secret-file", file, "-prefix", "bot", "-bots", "2", "-metrics", tt.metrics}, geo...)
			if code := run(canceled, args, &stdout, &stderr); code != tt.code {
				t.Fatalf("exit %d, want %d; stderr %q", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

// TestServeMetrics serves a registry, collects it on two ticks and reads
// the endpoint; stop then joins both goroutines and closes the listener.
func TestServeMetrics(t *testing.T) {
	ln, err := botmetrics.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reg := botmetrics.NewRegistry("rule")
	reg.NewBot().Reconnect()
	reg.NewBot().Abort()
	reg.SetBreaker(true)
	tick := make(chan time.Time)
	stop := serveMetrics(t.Context(), ln, reg, tick, slog.New(slog.DiscardHandler))
	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	// Unbuffered: the second send waits until the first collection is done.
	tick <- at
	tick <- at

	url := "http://" + ln.Addr().String() + botmetrics.VarsPath
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var vars struct {
		Bots     *botmetrics.Snapshot `json:"bots"`
		Memstats json.RawMessage      `json:"memstats"`
	}
	err = json.NewDecoder(resp.Body).Decode(&vars)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if b := vars.Bots; b == nil || b.Bots != 2 || b.Reconnects != 1 || b.Aborts != 1 || !b.BreakerOpen || !b.At.Equal(at) {
		t.Errorf("bots = %+v, want 2 bots, 1 reconnect, 1 abort, breaker open at %v", b, at)
	}
	if len(vars.Memstats) == 0 {
		t.Error("no memstats")
	}

	stop()
	select {
	case tick <- at:
		t.Error("collector still runs after stop")
	default:
	}
	if c, err := net.Dial("tcp", ln.Addr().String()); err == nil {
		c.Close()
		t.Error("endpoint still accepts after stop")
	}
}
