package botmetrics

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestListenLoopback(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:0", true},
		{"127.0.0.2:0", true},
		{"[::1]:0", true},
		{"0.0.0.0:0", false},
		{":8080", false},
		{":0", false},
		{"[::]:0", false},
		{"192.0.2.1:80", false},
		{"10.0.0.1:80", false},
		{"[::ffff:10.0.0.1]:80", false},
		{"localhost:0", false},
		{"example.com:80", false},
		{"127.0.0.1", false},
	}
	for _, tt := range tests {
		ln, err := Listen(tt.addr)
		if tt.ok {
			if err != nil {
				if strings.Contains(tt.addr, "::1") && strings.Contains(err.Error(), "listen") {
					t.Logf("Listen(%q): %v (no IPv6 loopback here)", tt.addr, err)
					continue
				}
				t.Errorf("Listen(%q) = %v, want a listener", tt.addr, err)
				continue
			}
			_ = ln.Close()
			continue
		}
		if err == nil {
			_ = ln.Close()
			t.Errorf("Listen(%q) succeeded, want it refused", tt.addr)
		}
	}
}

func TestMetricsEndpoint(t *testing.T) {
	r := NewRegistry("idle")
	b := r.NewBot()
	b.Sent()
	b.Rule("idle")
	r.Collect(t0)

	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Serve(ctx, ln, Handler(r)) }()

	resp, err := http.Get("http://" + ln.Addr().String() + VarsPath)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var vars map[string]json.RawMessage
	err = json.NewDecoder(resp.Body).Decode(&vars)
	_ = resp.Body.Close()
	cancel()
	if serr := <-errc; serr != nil {
		t.Errorf("Serve = %v, want nil after cancel", serr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["cmdline"]; ok {
		t.Error("endpoint serves cmdline")
	}
	var bots Snapshot
	if err := json.Unmarshal(vars["bots"], &bots); err != nil {
		t.Fatal(err)
	}
	if bots.Bots != 1 || bots.Sent != 1 || bots.Rules["idle"] != 1 {
		t.Errorf("bots = %+v", bots)
	}
	var mem struct{ HeapAlloc, Sys uint64 }
	if err := json.Unmarshal(vars["memstats"], &mem); err != nil || mem.Sys == 0 {
		t.Errorf("memstats = %s (%v)", vars["memstats"], err)
	}
}

// TestServeListenerFailure: a listener that cannot accept makes Serve return
// the serve error at once, without waiting for its context to end.
func TestServeListenerFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	err = Serve(context.Background(), ln, http.NotFoundHandler())
	if err == nil {
		t.Fatal("Serve on a closed listener returned nil")
	}
	if !strings.HasPrefix(err.Error(), "metrics serve: ") || !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Serve error = %v, want \"metrics serve: \" wrapping net.ErrClosed", err)
	}
}
