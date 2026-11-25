package botmetrics

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
)

// PprofPath is the profile index; the profiles are served below it.
const PprofPath = "/debug/pprof/"

// PprofHandler serves the Go profiles under PprofPath on its own mux:
// the index, every named profile (heap, goroutine, block, mutex, ...),
// profile (CPU), trace and symbol. It does not touch http.DefaultServeMux,
// and cmdline is not served: it holds the process flags, the account
// prefix among them.
func PprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PprofPath, pprof.Index) // Index serves the named profiles below it.
	mux.HandleFunc("GET "+PprofPath+"profile", pprof.Profile)
	mux.HandleFunc("GET "+PprofPath+"symbol", pprof.Symbol)
	mux.HandleFunc("POST "+PprofPath+"symbol", pprof.Symbol)
	mux.HandleFunc("GET "+PprofPath+"trace", pprof.Trace)
	return mux
}

// ServePprof serves PprofHandler on ln until ctx ends, like Serve. A CPU
// profile or trace runs for the seconds the caller asks, so the response
// has no write limit; the read limits and the shutdown deadline hold.
func ServePprof(ctx context.Context, ln net.Listener) error {
	return serve(ctx, ln, PprofHandler(), "pprof", 0)
}
