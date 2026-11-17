package gmdb

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenMalformedDSNHidesIt(t *testing.T) {
	const dsn = "acisbot:hunter2@tcp(127.0.0.1:13306" // no slash, no closing paren
	_, err := Open(dsn)
	if !errors.Is(err, errBadDSN) {
		t.Fatalf("Open = %v, want errBadDSN", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error %q carries the password", err)
	}
}

func TestOpenDialsNothing(t *testing.T) {
	l, err := Open("acisbot:hunter2@tcp(127.0.0.1:13306)/acis")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := l.db.Stats().MaxOpenConnections; got != maxConns {
		t.Errorf("MaxOpenConnections = %d, want %d", got, maxConns)
	}
	if got := l.db.Stats().OpenConnections; got != 0 {
		t.Errorf("OpenConnections = %d, want 0 before a query", got)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRedact(t *testing.T) {
	plain := errors.New("dial tcp 127.0.0.1:13306: connect: connection refused")
	tests := []struct {
		name    string
		err     error
		secrets []string
		want    string
		same    bool
	}{
		{"nil", nil, []string{"x"}, "", true},
		{"no secret", plain, []string{"hunter2", "user:hunter2@tcp(h)/db"}, plain.Error(), true},
		{"password", errors.New("bad password hunter2 here"), []string{"user:hunter2@tcp(h)/db", "hunter2"}, "bad password [redacted] here", false},
		{"dsn", errors.New("open user:hunter2@tcp(h)/db: refused"), []string{"user:hunter2@tcp(h)/db", "hunter2"}, "open [redacted]: refused", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redact(tt.err, tt.secrets)
			if tt.err == nil {
				if got != nil {
					t.Fatalf("redact(nil) = %v", got)
				}
				return
			}
			if got.Error() != tt.want {
				t.Errorf("redact = %q, want %q", got, tt.want)
			}
			if (got == tt.err) != tt.same {
				t.Errorf("redact returned the same error: %v, want %v", got == tt.err, tt.same)
			}
		})
	}
}
