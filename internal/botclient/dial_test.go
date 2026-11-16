package botclient

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestParseLocalAddr(t *testing.T) {
	tests := []struct {
		in      string
		wantNil bool
		wantErr bool
	}{
		{in: "", wantNil: true},
		{in: "127.0.0.2"},
		{in: "::1"},
		{in: "localhost", wantErr: true},
		{in: "127.0.0.2:0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			opt, err := ParseLocalAddr(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if (opt == nil) != (tt.wantNil || tt.wantErr) {
				t.Fatalf("option nil = %v", opt == nil)
			}
		})
	}
}

// TestDialLocalAddr checks the server sees the bound source address (Linux:
// all of 127.0.0.0/8 is loopback).
func TestDialLocalAddr(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback: %v", err)
	}
	defer ln.Close()
	tests := []struct {
		name string
		opts []DialOption
		want string
	}{
		{"default", nil, "127.0.0.1"},
		{"bound", []DialOption{WithLocalAddr(net.ParseIP("127.0.0.2"))}, "127.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(chan string, 1)
			go func() {
				c, err := ln.Accept()
				if err != nil {
					got <- err.Error()
					return
				}
				defer c.Close()
				host, _, _ := net.SplitHostPort(c.RemoteAddr().String())
				got <- host
			}()
			c, err := dial(context.Background(), ln.Addr().String(), time.Second, tt.opts)
			if err != nil {
				t.Skipf("cannot bind %v here: %v", tt.want, err)
			}
			defer c.Close()
			if host := <-got; host != tt.want {
				t.Fatalf("server saw source %q, want %q", host, tt.want)
			}
		})
	}
}
