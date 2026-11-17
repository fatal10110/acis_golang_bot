package botpanel

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRingKeepsTheLatestLinesPerBot(t *testing.T) {
	var stderr bytes.Buffer
	ring := NewRing(3)
	log := slog.New(ring.Handler(slog.NewTextHandler(&stderr, nil), nil))
	bot1 := log.With("account", "hz1")
	log.Info("panel starts")
	bot1.Info("bot starts")
	log.Info("bot ended", "account", "hz2")
	bot1.Warn("bot reconnects", "after", "eof")
	log.WithGroup("g").Info("grouped", "account", "hz3")

	if got := strings.Count(stderr.String(), "\n"); got != 5 {
		t.Fatalf("stderr got %d lines, want every 5:\n%s", got, stderr.String())
	}
	tests := []struct {
		name    string
		account string
		n       int
		want    []string
	}{
		{"bounded, oldest first", "", 0, []string{"bot ended", "bot reconnects", "grouped"}},
		{"last n", "", 1, []string{"grouped"}},
		{"one bot by WithAttrs", "hz1", 0, []string{"bot reconnects"}},
		{"one bot by the record", "hz2", 0, []string{"bot ended"}},
		{"an attribute in a group names no bot", "hz3", 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := ring.Lines(tt.account, tt.n)
			if len(lines) != len(tt.want) {
				t.Fatalf("lines %+v, want %q", lines, tt.want)
			}
			for i, l := range lines {
				if !strings.Contains(l.Text, tt.want[i]) || strings.HasSuffix(l.Text, "\n") {
					t.Fatalf("line %d %q, want %q", i, l.Text, tt.want[i])
				}
			}
		})
	}
	if l := ring.Lines("hz1", 0)[0]; l.Level != "WARN" || l.Account != "hz1" || !strings.Contains(l.Text, "account=hz1") || l.At.IsZero() {
		t.Fatalf("hz1's line %+v", l)
	}
}

func TestRingHonorsLevels(t *testing.T) {
	var stderr bytes.Buffer
	ring := NewRing(10)
	log := slog.New(ring.Handler(slog.NewTextHandler(&stderr, &slog.HandlerOptions{Level: slog.LevelWarn}), &slog.HandlerOptions{Level: slog.LevelDebug}))
	log.Debug("detail")
	log.Warn("problem")
	if strings.Contains(stderr.String(), "detail") || !strings.Contains(stderr.String(), "problem") {
		t.Fatalf("stderr %q", stderr.String())
	}
	if got := ring.Lines("", 0); len(got) != 2 {
		t.Fatalf("ring %+v, want both lines", got)
	}
}
