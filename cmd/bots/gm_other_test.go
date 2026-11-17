//go:build !windows

package main

import (
	"errors"
	"strings"
	"testing"

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
