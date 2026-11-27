package botclient

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Payloads in the server's layouts (serverpackets/<file> named per decoder).
func moveToLocation(oid int32, dest, origin Point) []byte {
	return packet(opGameMoveToLocation, oid, dest.X, dest.Y, dest.Z, origin.X, origin.Y, origin.Z)
}

func stopMove(oid int32, at Point, heading int32) []byte {
	return packet(opGameStopMove, oid, at.X, at.Y, at.Z, heading)
}

// scriptedSource replays frames through one reused buffer, the way
// wire.FrameReader does, poisoning it before every read; then returns end.
type scriptedSource struct {
	frames [][]byte
	end    error
	buf    []byte
	reads  int
	// onRead, when set, runs at the start of each Read with its 1-based
	// index.
	onRead func(n int)
}

func (s *scriptedSource) Read(time.Time) ([]byte, error) {
	s.reads++
	if s.onRead != nil {
		s.onRead(s.reads)
	}
	for i := range s.buf {
		s.buf[i] = 0xff
	}
	if len(s.frames) == 0 {
		return nil, s.end
	}
	f := s.frames[0]
	s.frames = s.frames[1:]
	if cap(s.buf) < len(f) {
		s.buf = make([]byte, len(f), 64*1024)
	}
	s.buf = s.buf[:len(f)]
	copy(s.buf, f)
	return s.buf, nil
}

func TestReadEventsFiltersAndQueues(t *testing.T) {
	at := Point{X: 1, Y: 2, Z: 3}
	src := &scriptedSource{
		frames: [][]byte{
			{opGameCharSelectInfo}, // no decoder: dropped unparsed, though far too short
			moveToLocation(5, at, at),
			{0xfe, 0x01}, // unknown opcode
			stopMove(5, at, 9),
			packet(opGameServerClose),
		},
		end: io.EOF,
	}
	q := NewEvents(16)
	err := readEvents(src, q, make(chan struct{}))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("readEvents = %v, want io.EOF", err)
	}
	want := []Event{MoveToLocation{ObjectID: 5, Dest: at, Origin: at}, StopMove{ObjectID: 5, Pos: at, Heading: 9}, ServerClose{}}
	if got := q.Drain(nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

// TestReadEventsTeardownFullQueue: a reader blocked on a full queue nobody
// drains returns once done closes.
func TestReadEventsTeardownFullQueue(t *testing.T) {
	const limit = 2
	done := make(chan struct{})
	src := &scriptedSource{end: io.EOF}
	for oid := range int32(limit + 2) {
		src.frames = append(src.frames, moveToLocation(oid+1, Point{}, Point{}))
	}
	// done closes while the third frame is read, so its push finds the
	// queue full and waits on done.
	src.onRead = func(n int) {
		if n == limit+1 {
			close(done)
		}
		if n > limit+1 {
			t.Error("reader read past the full queue")
		}
	}
	q := NewEvents(limit)
	result := make(chan error, 1)
	go func() { result <- readEvents(src, q, done) }()
	if err := <-result; err != nil {
		t.Fatalf("readEvents = %v, want nil after done", err)
	}
	if got := q.Drain(nil); len(got) != limit {
		t.Fatalf("queued %d events, want %d", len(got), limit)
	}
}

// TestGameConnReadEvents runs the reader on a real connection, whose frame
// buffer is reused on every read, while this goroutine drains as the brain
// would; -race flags any event that shares memory with the buffer.
func TestGameConnReadEvents(t *testing.T) {
	var script []step
	var want []Event
	for oid := range int32(50) {
		at := Point{X: oid, Y: -oid, Z: 7}
		script = append(script,
			sendStep(packet(opGameCharSelectInfo, oid, "burst noise")), // no decoder: dropped
			sendStep(moveToLocation(oid+1, at, Point{})),
			sendStep(packet(opGameTeleportToLocation, oid+1, at.X, at.Y, at.Z, 1)),
		)
		want = append(want, MoveToLocation{ObjectID: oid + 1, Dest: at}, TeleportToLocation{ObjectID: oid + 1, Pos: at, Fast: true})
	}
	script = append(script, sendStep(packet(opGameServerClose)))
	want = append(want, ServerClose{})
	g, served := dialFake(t, true, script, true)

	q := NewEvents(4)
	result := make(chan error, 1)
	go func() { result <- g.ReadEvents(q, make(chan struct{})) }()
	var got []Event
	var err error
	for finished := false; !finished; {
		select {
		case <-q.Ready():
			got = q.Drain(got)
		case err = <-result:
			finished = true
		}
	}
	got = q.Drain(got)
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("ReadEvents = nil after the hang-up, want the read error")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events differ:\n got %v\nwant %v", got, want)
	}
}

// TestReadEventsReadErrorAfterDone: the normal teardown. The session
// closes done, then the connection, so the blocked Read fails; that error
// is the teardown, not a failure.
func TestReadEventsReadErrorAfterDone(t *testing.T) {
	done := make(chan struct{})
	src := &scriptedSource{frames: [][]byte{stopMove(5, Point{}, 0)}, end: net.ErrClosed}
	src.onRead = func(n int) {
		if n == 2 {
			close(done)
		}
	}
	q := NewEvents(4)
	if err := readEvents(src, q, done); err != nil {
		t.Fatalf("readEvents = %v, want nil after done", err)
	}
	if got := q.Drain(nil); len(got) != 1 {
		t.Fatalf("queued %d events, want 1", len(got))
	}
}

// TestGameConnReadEventsTeardown: a reader blocked in Read on a quiet
// connection returns nil once the session closes done and then the
// connection.
func TestGameConnReadEventsTeardown(t *testing.T) {
	g, served := dialFake(t, true, nil, false)
	done := make(chan struct{})
	result := make(chan error, 1)
	go func() { result <- g.ReadEvents(NewEvents(4), done) }()
	close(done)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("ReadEvents = %v, want nil after done", err)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

// TestGameConnRecords: the tap sits in GameConn's read, so it records the
// first frame of each opcode whether the reader decodes it or not.
func TestGameConnRecords(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rec")
	rec, err := NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := moveToLocation(1, Point{X: 1}, Point{})
	unknown := []byte{0xfe, 0x01, 0x02}
	script := []step{sendStep(first), sendStep(unknown), sendStep(moveToLocation(2, Point{X: 2}, Point{})), sendStep([]byte{0xfe, 0x09})}
	g, served := dialFake(t, true, script, true)
	g.SetRecorder(rec)
	if err := g.ReadEvents(NewEvents(8), make(chan struct{})); err == nil {
		t.Fatal("ReadEvents = nil after the hang-up, want the read error")
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if err := rec.Err(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{"01.bin": first, "fe.bin": unknown} {
		path := filepath.Join(dir, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %x, want the first payload %x", name, got, want)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s mode = %v, want owner-only", name, perm)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("recorded %d files, want one per opcode (2)", len(entries))
	}
}

func TestRecorderKeepsFirstWriteError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rec")
	rec, err := NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	rec.Record([]byte{opGameStopMove})
	rec.Record([]byte{opGameMoveToLocation})
	if err := rec.Err(); err == nil || !strings.Contains(err.Error(), "0x47") {
		t.Fatalf("Err = %v, want the first failed write (opcode 0x47)", err)
	}
	var none *Recorder
	none.Record([]byte{opGameStopMove}) // a nil Recorder is off
}

// TestRecordedCharSelectInfo replays the -record capture of CharSelectInfo
// checked in under testdata/recorded: the handshake reads it, not the
// reader, so botwire's TestRecordedFixtures leaves it here. It must decode
// with exactly len(payload) consumed.
func TestRecordedCharSelectInfo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "recorded", fmt.Sprintf("%02x.bin", opGameCharSelectInfo)))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != opGameCharSelectInfo {
		t.Fatalf("fixture name does not match its opcode")
	}
	if _, err := decodeCharSelectInfo(raw); err != nil {
		t.Fatal(err)
	}
}
