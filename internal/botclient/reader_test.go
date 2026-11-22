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

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Payloads in the server's layouts (serverpackets/<file> named per decoder).
func moveToLocation(oid int32, dest, origin Point) []byte {
	return packet(opGameMoveToLocation, oid, dest.X, dest.Y, dest.Z, origin.X, origin.Y, origin.Z)
}

func stopMove(oid int32, at Point, heading int32) []byte {
	return packet(opGameStopMove, oid, at.X, at.Y, at.Z, heading)
}

func TestDecodeEvent(t *testing.T) {
	at := Point{X: 81000, Y: 148000, Z: -3400}
	to := Point{X: 81200, Y: 148100, Z: -3410}
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		{"MoveToLocation", moveToLocation(0x10000001, to, at), MoveToLocation{ObjectID: 0x10000001, Dest: to, Origin: at}},
		{"StopMove", stopMove(7, at, 49152), StopMove{ObjectID: 7, Pos: at, Heading: 49152}},
		{"ValidateLocation", packet(opGameValidateLocation, 7, at.X, at.Y, at.Z, 100), ValidateLocation{ObjectID: 7, Pos: at, Heading: 100}},
		{"TeleportToLocation", packet(opGameTeleportToLocation, 7, at.X, at.Y, at.Z, 0), TeleportToLocation{ObjectID: 7, Pos: at}},
		{"TeleportToLocation fast", packet(opGameTeleportToLocation, 7, at.X, at.Y, at.Z, 1), TeleportToLocation{ObjectID: 7, Pos: at, Fast: true}},
		{"DeleteObject", packet(opGameDeleteObject, 7, 1), DeleteObject{ObjectID: 7}},
		{"DeleteObject seated", packet(opGameDeleteObject, 7, 0), DeleteObject{ObjectID: 7, Seated: true}},
		{"ServerClose", packet(opGameServerClose), ServerClose{}},
		{"LeaveWorld", packet(opGameLeaveWorld), LeaveWorld{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("decodeEvent = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeEventRejects(t *testing.T) {
	move := moveToLocation(1, Point{}, Point{})
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"short", move[:len(move)-1], wire.ErrShortPacket},
		{"trailing", append(move, 0), errTrailingBytes},
		{"trailing on empty body", packet(opGameLeaveWorld, 0), errTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("decodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
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

func TestReadEventsMalformedFrameEnds(t *testing.T) {
	src := &scriptedSource{frames: [][]byte{packet(opGameStopMove, 5)}, end: io.EOF}
	err := readEvents(src, NewEvents(4), make(chan struct{}))
	if !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("readEvents = %v, want ErrShortPacket", err)
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

// recordedMustDecode pins the recorded opcodes whose decoder must run: a
// decoder dropped later, or a fixture deleted, fails the test instead of
// passing silently.
var recordedMustDecode = []byte{
	opGameMoveToLocation, opGameUserInfo, opGameStatusUpdate, opGameDeleteObject,
	opGameCharSelectInfo, opGameNPCInfo, opGameActionFailed, opGameChangeWaitType,
	opGameSystemMessage, opGameLeaveWorld, opGameSkillList, opGameSkillCoolTime, opGameItemList,
	opGameAskJoinPledge, opGameJoinPledge, opGamePledgeShowMemberListAll,
	opGamePledgeShowMemberListDeleteAll, opGameWarehouseDepositList, opGameWarehouseWithdrawList,
}

// TestRecordedFixtures replays the -record captures checked in under
// testdata/recorded: every payload with a decoder must decode with exactly
// len(payload) consumed and carry a plausible object id. Ids, coordinates
// and times vary between runs and are not pinned. A payload with no decoder
// yet is skipped, not passed.
func TestRecordedFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "recorded", "*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Skip("no recorded fixtures yet: they need a live server (#5)")
	}
	pinned := map[byte]bool{}
	for _, op := range recordedMustDecode {
		pinned[op] = true
	}
	decoded := map[byte]bool{}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) == 0 || filepath.Base(path) != fmt.Sprintf("%02x.bin", raw[0]) {
				t.Fatalf("fixture name does not match its opcode")
			}
			if raw[0] == opGameCharSelectInfo { // read in the handshake, not by the reader
				if _, err := decodeCharSelectInfo(raw); err != nil {
					t.Fatal(err)
				}
				decoded[raw[0]] = true
				return
			}
			ev, ok, err := decodeEvent(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				if pinned[raw[0]] {
					t.Fatalf("opcode %#02x is pinned but has no decoder", raw[0])
				}
				t.Skipf("no decoder for opcode %#02x yet", raw[0])
			}
			decoded[raw[0]] = true
			if oid := reflect.ValueOf(ev).FieldByName("ObjectID"); oid.IsValid() && oid.Int() == 0 {
				t.Errorf("%T has object id 0", ev)
			}
		})
	}
	for _, op := range recordedMustDecode {
		if !decoded[op] {
			t.Errorf("pinned opcode %#02x was not decoded from a fixture", op)
		}
	}
}
