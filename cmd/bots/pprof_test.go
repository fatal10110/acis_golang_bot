package main

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

func TestParsePprofFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"off by default", nil, "", ""},
		{"loopback", []string{"-pprof", "127.0.0.1:6061"}, "127.0.0.1:6061", ""},
		{"loopback v6", []string{"-pprof", "[::1]:6061"}, "[::1]:6061", ""},
		{"unspecified", []string{"-pprof", "0.0.0.0:6061"}, "", "not loopback"},
		{"empty host", []string{"-pprof", ":6061"}, "", "-pprof"},
		{"host name", []string{"-pprof", "localhost:6061"}, "", "loopback IP"},
		{"no port", []string{"-pprof", "127.0.0.1"}, "", "-pprof"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(append([]string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}, tt.args...), io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || o.pprof != tt.want {
				t.Fatalf("pprof, err = %q, %v; want %q, nil", o.pprof, err, tt.want)
			}
		})
	}
}

// TestServePprof serves the profile index and stop joins the goroutine.
func TestServePprofLifecycle(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	if ln, err := listenPprof("", log); ln != nil || err != nil {
		t.Fatalf("empty addr = %v, %v; want none", ln, err)
	}
	ln, err := listenPprof("127.0.0.1:0", log)
	if err != nil {
		t.Fatal(err)
	}
	stop := servePprof(t.Context(), ln, log)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+botmetrics.PprofPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	stop()
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("endpoint still serves after stop")
	}
	if _, err := listenPprof("0.0.0.0:0", log); err == nil {
		t.Fatal("wildcard bound")
	}
}
