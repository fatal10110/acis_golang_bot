package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// outcomeRotateAt is the size past which the outcome log becomes
// <file>.1, replacing an older one, and a new file starts (OP0, D47).
const outcomeRotateAt = 64 << 20

// addOutcomeLogFlag adds -outcome-log, off by default.
func addOutcomeLogFlag(fs *flag.FlagSet, path *string) {
	fs.StringVar(path, "outcome-log", "", fmt.Sprintf("file that receives one JSON line per spot stay of a farming bot (spot, class, level, minutes, experience, adena, deaths, kills, the model's estimate; no names, no chat), rotated past %d MiB with one older file kept; default: record nothing", outcomeRotateAt>>20))
}

// outcomeFile is the outcome log: JSONL appended to path, rotated once a
// line would take it past limit. One writer goroutine owns it.
type outcomeFile struct {
	path  string
	limit int64
	f     *os.File
	size  int64
}

func openOutcomeFile(path string, limit int64) (*outcomeFile, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("outcome log: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("outcome log: %w", err)
	}
	return &outcomeFile{path: path, limit: limit, f: f, size: st.Size()}, nil
}

// write appends o as one line, rotating first when the line would take a
// non-empty file past the limit, so no line is split across files.
func (l *outcomeFile) write(o botclient.Outcome) error {
	line, err := json.Marshal(o)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if l.size > 0 && l.size+int64(len(line)) > l.limit {
		if err := l.rotate(); err != nil {
			return err
		}
	}
	n, err := l.f.Write(line)
	l.size += int64(n)
	return err
}

// rotate moves the file to <path>.1, replacing an older one, and starts a
// new one.
func (l *outcomeFile) rotate() error {
	if err := l.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	l.f, l.size = f, 0
	return nil
}

func (l *outcomeFile) close() error { return l.f.Close() }

// runOutcomeFile writes the records of q to l until stop closes, then the
// records still queued, and closes l. A failed write is logged once and
// its record dropped.
func runOutcomeFile(l *outcomeFile, q *botclient.OutcomeQueue, stop <-chan struct{}, log *slog.Logger) {
	failed := false
	write := func(o botclient.Outcome) {
		if err := l.write(o); err != nil && !failed {
			failed = true
			log.Error("outcome log write failed", "err", err)
		}
	}
	defer func() {
		if err := l.close(); err != nil {
			log.Error("outcome log close failed", "err", err)
		}
	}()
	for {
		select {
		case o := <-q.Records():
			write(o)
		case <-stop:
			for {
				select {
				case o := <-q.Records():
					write(o)
				default:
					return
				}
			}
		}
	}
}

// startOutcomeLog opens the outcome log at path and starts its one writer
// goroutine. The returned stop, called once every bot returned, ends the
// writer and joins it. An empty path opens nothing and starts nothing: the
// queue is nil and stop does nothing.
func startOutcomeLog(path string, log *slog.Logger) (q *botclient.OutcomeQueue, stop func(), err error) {
	if path == "" {
		return nil, func() {}, nil
	}
	l, err := openOutcomeFile(path, outcomeRotateAt)
	if err != nil {
		return nil, nil, err
	}
	q = botclient.NewOutcomeQueue()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { runOutcomeFile(l, q, done, log) })
	log.Info("outcome log opened", "file", path)
	return q, func() {
		close(done)
		wg.Wait()
		if n := q.Dropped(); n > 0 {
			log.Warn("outcome log dropped records", "dropped", n)
		}
	}, nil
}
