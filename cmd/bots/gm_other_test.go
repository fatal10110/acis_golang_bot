//go:build !windows

package main

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/ownerfile"
)

// TestReadDSNFileSharedMode: a DSN file with a group or other mode bit is
// refused (internal/ownerfile tests the Windows DACL check).
func TestReadDSNFileSharedMode(t *testing.T) {
	_, err := readDSNFile(writeDSN(t, testDSN, 0o640))
	if !errors.Is(err, ownerfile.ErrShared) || !strings.Contains(err.Error(), "want 0600") {
		t.Fatalf("err = %v, want ErrShared naming 0600", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error %q carries the password", err)
	}
}

// TestStartAuthorityRefusesASharedDSNFile: a group-readable DSN file stops
// the GM authority before any connection.
func TestStartAuthorityRefusesASharedDSNFile(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	o := gmOptions{dsnFile: writeDSN(t, testDSN, 0o644)}
	_, _, err := startAuthority(t.Context(), o, botclient.SystemClock{}, log)
	if err == nil || !strings.Contains(err.Error(), "want 0600") || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err = %v, want \"want 0600\" without the password", err)
	}
}
