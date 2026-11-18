package botclient

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

// tapLog is a FrameTap keeping copies of what it saw.
type tapLog struct {
	mu        sync.Mutex
	read, out [][]byte
}

func (l *tapLog) Record(p []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.read = append(l.read, bytes.Clone(p))
}

func (l *tapLog) RecordSent(p []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = append(l.out, bytes.Clone(p))
}

// TestGameConnTapsBothWays: the tap sees each frame written in clear, as the
// brain encoded it, and each frame read decrypted, on an encrypted
// connection.
func TestGameConnTapsBothWays(t *testing.T) {
	attack := encodeAttackRequest(7, Point{X: 1, Y: 2, Z: 3})
	move := moveToLocation(1, Point{X: 1}, Point{})
	g, served := dialFake(t, true, []step{recvStep(attack), sendStep(move)}, true)
	tap := &tapLog{}
	g.SetRecorder(tap)
	if err := g.Send(attack); err != nil {
		t.Fatal(err)
	}
	if err := g.ReadEvents(NewEvents(8), make(chan struct{})); err == nil {
		t.Fatal("ReadEvents = nil after the hang-up, want the read error")
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	tap.mu.Lock()
	defer tap.mu.Unlock()
	if len(tap.out) != 1 || !bytes.Equal(tap.out[0], attack) {
		t.Errorf("sent frames = %x, want [%x]", tap.out, attack)
	}
	if len(tap.read) != 1 || !bytes.Equal(tap.read[0], move) {
		t.Errorf("read frames = %x, want [%x]", tap.read, move)
	}
}

// TestRecorderIgnoresSent: the -record fixtures are server packets only.
func TestRecorderIgnoresSent(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, served := dialFake(t, false, []step{recvStep(encodeAppearing())}, true)
	g.SetRecorder(rec)
	if err := g.Send(encodeAppearing()); err != nil {
		t.Fatal(err)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if err := rec.Err(); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("recorded %v (%v), want nothing for a sent frame", entries, err)
	}
	var none *Recorder
	g.SetRecorder(none) // a nil Recorder in the interface is off too
	none.RecordSent(encodeAppearing())
}
