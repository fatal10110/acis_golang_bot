package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

func TestParseSwarmFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		check   func(swarmOptions) bool
		wantErr string
	}{
		{"defaults", base, func(o swarmOptions) bool {
			return o.loginAddr == "127.0.0.1:2106" && o.gameAddr == "" && o.serverID == 1 && o.timeout == 10*time.Second &&
				o.marker == botnames.DefaultMarker && o.record == "" && !o.accounts.Persistent && o.accounts.Count == 3 && !o.allowlisted
		}, ""},
		{
			"all set", append(base, "-persistent", "-login", "127.0.0.1:1", "-game", "127.0.0.1:7777", "-server-id", "2",
				"-local-addr", "127.0.0.2", "-record", "fx", "-name-marker", "Npc", "-name-marker-at", "prefix", "-timeout", "3s"),
			func(o swarmOptions) bool {
				return o.accounts.Persistent && o.loginAddr == "127.0.0.1:1" && o.gameAddr == "127.0.0.1:7777" && o.serverID == 2 &&
					o.localAddr == "127.0.0.2" && o.record == "fx" && o.timeout == 3*time.Second &&
					o.marker == botnames.Marker{Text: "Npc", Placement: botnames.Prefix}
			}, "",
		},
		{"allowlisted", append(base, "-local-addr", "127.0.0.2", "-allowlisted"), func(o swarmOptions) bool {
			return o.allowlisted && o.localAddr == "127.0.0.2"
		}, ""},
		{"allowlisted without a local address", append(base, "-allowlisted"), nil, "-allowlisted needs -local-addr"},
		{"bad placement", append(base, "-name-marker-at", "middle"), nil, "-name-marker-at"},
		{"bad marker", append(base, "-name-marker", "B-t"), nil, "-name-marker"},
		{"server id 0", append(base, "-server-id", "0"), nil, "-server-id"},
		{"server id 256", append(base, "-server-id", "256"), nil, "-server-id"},
		{"bad local address", append(base, "-local-addr", "localhost"), nil, "-local-addr"},
		{"no timeout", append(base, "-timeout", "0s"), nil, "-timeout"},
		{"stray argument", append(base, "extra"), nil, "unexpected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := parseSwarmFlags(tt.args, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tt.check(o) {
				t.Fatalf("options %+v", o)
			}
		})
	}
}

func TestRunSwarm(t *testing.T) {
	dir := t.TempDir()
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "bots.secret")
	if err := os.WriteFile(good, []byte(hex.EncodeToString(secret)), 0o600); err != nil {
		t.Fatal(err)
	}
	weak := filepath.Join(dir, "weak.secret")
	if err := os.WriteFile(weak, []byte("changeme"), 0o600); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		args []string
		code int
		want string
	}{
		{"persistent, default secret", t.Context(), []string{"-persistent", "-secret-file", weak, "-prefix", "bot", "-bots", "2"}, 1, "persistent mode"},
		{"no secret file", t.Context(), []string{"-prefix", "bot", "-bots", "2"}, 1, "no secret file"},
		{"unwritable record directory", t.Context(), []string{"-secret-file", good, "-prefix", "bot", "-bots", "2", "-record", filepath.Join(good, "sub")}, 1, "bots swarm:"},
		{"bad flag", t.Context(), []string{"-interval", "1s"}, 2, "-interval"},
		{"help", t.Context(), []string{"-h"}, 0, "-name-marker"},
		// A signal before any turn of the gate: every bot returns at once.
		{"signal before the first login", canceled, []string{"-persistent", "-secret-file", good, "-prefix", "bot", "-bots", "3", "-record", filepath.Join(dir, "fx")}, 0, "swarm stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			// The login address is unroutable: nothing may reach a dial.
			args := append([]string{"-login", "192.0.2.1:1", "-timeout", "1ms"}, tt.args...)
			if code := run(tt.ctx, append([]string{"swarm"}, args...), &stdout, &stderr); code != tt.code {
				t.Fatalf("exit %d, want %d; stderr %q", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr %q, want %q", stderr.String(), tt.want)
			}
			if strings.Contains(stderr.String()+stdout.String(), "changeme") || strings.Contains(stderr.String(), hex.EncodeToString(secret)) {
				t.Fatal("output quotes the secret")
			}
			if strings.Contains(stderr.String(), "bot stopped") {
				t.Fatalf("stderr %q: no bot may log in", stderr.String())
			}
		})
	}
}

func TestSwarmExit(t *testing.T) {
	halted := botclient.ErrLoginsHalted
	tests := []struct {
		name            string
		ctxErr, breaker error
		want            int
	}{
		{"signal", context.Canceled, nil, 0},
		{"every bot stopped by itself", nil, nil, exitStopped},
		{"breaker", nil, halted, exitStopped},
		{"breaker, then a signal", context.Canceled, halted, exitStopped},
	}
	for _, tt := range tests {
		if got := swarmExit(tt.ctxErr, tt.breaker); got != tt.want {
			t.Errorf("%s: exit %d, want %d", tt.name, got, tt.want)
		}
	}
}
