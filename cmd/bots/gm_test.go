package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestParseSwarmGMFlags(t *testing.T) {
	base := []string{"-secret-file", "s", "-prefix", "hz", "-bots", "3"}
	tests := []struct {
		name    string
		args    []string
		want    gmOptions
		wantErr string
	}{
		{"none", base, gmOptions{}, ""},
		{"commanders", append(base, "-commanders", "268435457, 268435458"), gmOptions{commanders: []int32{268435457, 268435458}}, ""},
		{"database", append(base, "-gm-dsn-file", "dsn", "-gm-min-level", "5"), gmOptions{dsnFile: "dsn", minLevel: 5}, ""},
		{"both", append(base, "-commanders", "7", "-gm-dsn-file", "dsn"), gmOptions{}, "choose one"},
		{"bad commander", append(base, "-commanders", "7,x"), gmOptions{}, "object id"},
		{"zero commander", append(base, "-commanders", "0"), gmOptions{}, "object id"},
		{"empty commanders", append(base, "-commanders", " , "), gmOptions{}, "no object id"},
		{"min level without database", append(base, "-gm-min-level", "7"), gmOptions{}, "needs -gm-dsn-file"},
		{"min level too high", append(base, "-gm-dsn-file", "dsn", "-gm-min-level", "9"), gmOptions{}, "want 1-8"},
		{"negative min level", append(base, "-gm-dsn-file", "dsn", "-gm-min-level", "-1"), gmOptions{}, "want 1-8"},
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
			if !slices.Equal(o.gm.commanders, tt.want.commanders) || o.gm.dsnFile != tt.want.dsnFile || o.gm.minLevel != tt.want.minLevel {
				t.Fatalf("gm options %+v, want %+v", o.gm, tt.want)
			}
		})
	}
}

const testDSN = "acisbot:hunter2@tcp(127.0.0.1:13306)/acis"

func writeDSN(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadDSNFile(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{"owner only", writeDSN(t, testDSN+"\n", 0o600), testDSN, ""},
		{"empty", writeDSN(t, " \n", 0o600), "", "empty"},
		{"too long", writeDSN(t, strings.Repeat("x", maxDSNFileSize+1), 0o600), "", "longer than"},
		{"missing", filepath.Join(t.TempDir(), "none"), "", "no such file"},
		{"directory", t.TempDir(), "", "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readDSNFile(tt.path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "hunter2") {
					t.Fatalf("error %q carries the password", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("readDSNFile = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestStartAuthority(t *testing.T) {
	var logs strings.Builder
	log := slog.New(slog.NewTextHandler(&logs, nil))
	clock := botclient.SystemClock{}
	tests := []struct {
		name     string
		o        gmOptions
		wantAuth bool
		wantErr  string
	}{
		{"none", gmOptions{}, false, ""},
		{"commanders", gmOptions{commanders: []int32{7}}, true, ""},
		{"database", gmOptions{dsnFile: writeDSN(t, testDSN, 0o600), minLevel: 7}, true, ""},
		{"malformed database", gmOptions{dsnFile: writeDSN(t, "acisbot:hunter2@tcp(127.0.0.1", 0o600)}, false, "malformed"},
		{"open file", gmOptions{dsnFile: writeDSN(t, testDSN, 0o644)}, false, "want 0600"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, stop, err := startAuthority(t.Context(), tt.o, clock, log)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || strings.Contains(err.Error(), "hunter2") {
					t.Fatalf("err = %v, want %q without the password", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (a != nil) != tt.wantAuth {
				t.Fatalf("authority %v, want one: %v", a, tt.wantAuth)
			}
			stop()
		})
	}
	if strings.Contains(logs.String(), "hunter2") || strings.Contains(logs.String(), "13306") {
		t.Fatalf("logs carry the data source name: %s", logs.String())
	}
}
