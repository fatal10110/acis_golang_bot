package botmetrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPprofHandlerRoutes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"index", "GET", "/debug/pprof/", 200},
		{"heap", "GET", "/debug/pprof/heap?debug=1", 200},
		{"goroutine", "GET", "/debug/pprof/goroutine?debug=1", 200},
		{"symbol", "GET", "/debug/pprof/symbol", 200},
		{"cmdline hidden", "GET", "/debug/pprof/cmdline", 404},
		{"vars not served", "GET", "/debug/vars", 404},
		{"unknown profile", "GET", "/debug/pprof/nope", 404},
	}
	h := PprofHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
			}
		})
	}
}

// TestServePprof serves on a loopback listener and stops with the context.
func TestServePprof(t *testing.T) {
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- ServePprof(ctx, ln) }()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+PprofPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("ServePprof = %v, want nil", err)
	}
}
