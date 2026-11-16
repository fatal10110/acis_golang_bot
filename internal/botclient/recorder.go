package botclient

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Recorder is the -record tap: it writes the first decrypted game payload of
// each opcode, opcode byte included, to <dir>/<opcode as two hex
// digits>.bin, the recorded fixtures decoder tests replay. One Recorder
// serves every session of the process. Only server packets after the
// handshake pass through it, so no credential reaches a file.
type Recorder struct {
	dir string

	mu   sync.Mutex
	seen [256]bool // guarded by mu
	err  error     // guarded by mu; the first failed write
}

// NewRecorder creates dir (mode 0700) if needed and returns a Recorder
// writing into it.
func NewRecorder(dir string) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("record dir: %w", err)
	}
	return &Recorder{dir: dir}, nil
}

// Record writes payload (non-empty) when it is the first of its opcode;
// a nil Recorder records nothing. It returns before any other read of the
// frame buffer, and keeps no reference to payload. A failed write is kept
// for Err and does not stop the session.
func (r *Recorder) Record(payload []byte) {
	if r == nil {
		return
	}
	op := payload[0]
	r.mu.Lock()
	first := !r.seen[op]
	r.seen[op] = true
	r.mu.Unlock()
	if !first {
		return
	}
	path := filepath.Join(r.dir, fmt.Sprintf("%02x.bin", op))
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		r.mu.Lock()
		if r.err == nil {
			r.err = fmt.Errorf("record opcode %#02x: %w", op, err)
		}
		r.mu.Unlock()
	}
}

// Err returns the first write that failed, if any.
func (r *Recorder) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}
