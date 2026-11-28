package e2e

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// pipeConn is a scripted game connection. Read takes the frames the test's
// server hands over one at a time, unbuffered as net.Pipe is: a server send
// completes only once the client read it. Sent frames are kept in order.
type pipeConn struct {
	frames chan []byte
	closed chan struct{}
	once   sync.Once

	mu   sync.Mutex
	sent [][]byte // guarded by mu
}

func newPipeConn() *pipeConn {
	return &pipeConn{frames: make(chan []byte), closed: make(chan struct{})}
}

func (p *pipeConn) Read(time.Time) ([]byte, error) {
	select {
	case f, ok := <-p.frames:
		if !ok {
			return nil, io.EOF
		}
		return f, nil
	case <-p.closed:
		return nil, net.ErrClosed
	}
}

func (p *pipeConn) Send(payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, bytes.Clone(payload))
	return nil
}

func (p *pipeConn) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

// serve hands frames to the client, then closes the stream when end is set,
// and reports on the returned channel once the client read them all.
func (p *pipeConn) serve(end bool, frames ...[]byte) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, f := range frames {
			p.frames <- f
		}
		if end {
			close(p.frames)
		}
	}()
	return done
}

// fireClock's timers of at most fires fire at once; longer ones never do.
type fireClock struct{ fires time.Duration }

func (fireClock) Now() time.Time { return time.Unix(1000, 0) }

func (c fireClock) NewTimer(d time.Duration) botclient.Timer {
	if d > c.fires {
		return idleTimer{}
	}
	ch := make(chan time.Time, 1)
	ch <- time.Unix(1000, 0)
	return firedTimer(ch)
}

const drainSelf int32 = 7

func frameDeleteObject(id int32) []byte {
	return binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32([]byte{botwire.OpGameDeleteObject}, uint32(id)), 0)
}

func frameMyTarget(id int32) []byte {
	return binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint32([]byte{botwire.OpGameMyTargetSelected}, uint32(id)), 0)
}

// undecoded is a frame whose opcode has no decoder.
func undecoded(t *testing.T) []byte {
	t.Helper()
	for op := range 256 {
		if _, ok, err := botclient.DecodeEvent([]byte{byte(op)}); !ok && err == nil {
			return []byte{byte(op)}
		}
	}
	t.Fatal("every opcode has a decoder")
	return nil
}

func drainClient(clock botclient.Clock) (*Client, *pipeConn, *int) {
	p := newPipeConn()
	released := new(int)
	c := newClient("acct", botclient.Self{ObjectID: drainSelf}, p, clock, func() { *released++ })
	return c, p, released
}

func anyEvent(botclient.Event) bool { return true }

// TestDrainReadsWhileIdle: the server writes far past the inbox while the
// scenario is not in Await; the client reads every frame, keeps the early
// answer to its own request and the newest broadcasts, and counts the drops.
func TestDrainReadsWhileIdle(t *testing.T) {
	c, p, _ := drainClient(fireClock{})
	defer c.Close()
	n := 3 * inboxLimit
	frames := [][]byte{frameMyTarget(42), undecoded(t)}
	for id := 1; id <= n; id++ {
		frames = append(frames, frameDeleteObject(int32(id)))
	}
	// The client reading the trailing frame, which it skips, means it
	// queued every earlier one.
	frames = append(frames, undecoded(t))
	<-p.serve(false, frames...) // completes only if the client keeps reading

	ev, err := c.Await(t.Context(), time.Minute, anyEvent)
	if err != nil || ev != (botclient.MyTargetSelected{ObjectID: 42}) {
		t.Fatalf("first event = %v, %v; want MyTargetSelected 42", ev, err)
	}
	if c.Target() != 42 {
		t.Errorf("Target() = %d, want 42", c.Target())
	}
	ev, err = c.Await(t.Context(), time.Minute, anyEvent)
	if want := (botclient.DeleteObject{ObjectID: int32(n - inboxLimit + 2), Seated: true}); err != nil || ev != want {
		t.Fatalf("next event = %v, %v; want %v", ev, err, want)
	}
	if got, want := c.Dropped(), uint64(n-inboxLimit+1); got != want {
		t.Errorf("Dropped() = %d, want %d", got, want)
	}
}

func TestDrainEndsAwait(t *testing.T) {
	tests := []struct {
		name   string
		frames [][]byte
		end    bool // the server ends the stream, else the client closes
		want   []any
	}{
		{"stream ends", [][]byte{{botwire.OpGameActionFailed}}, true, []any{botclient.ActionFailed{}, io.EOF, io.EOF}},
		{"decode error in order", [][]byte{{botwire.OpGameDeleteObject, 1}, {botwire.OpGameActionFailed}}, true, []any{"decode", botclient.ActionFailed{}, io.EOF}},
		{"closed", nil, false, []any{net.ErrClosed, net.ErrClosed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, p, released := drainClient(fireClock{})
			<-p.serve(tt.end, tt.frames...)
			if !tt.end {
				c.Close()
			}
			for i, want := range tt.want {
				ev, err := c.Await(t.Context(), time.Minute, anyEvent)
				switch w := want.(type) {
				case error:
					if !errors.Is(err, w) {
						t.Errorf("Await %d = %v, %v; want %v", i, ev, err, w)
					}
				case string:
					if err == nil || errors.Is(err, io.EOF) {
						t.Errorf("Await %d = %v, %v; want a decode error", i, ev, err)
					}
				default:
					if err != nil || ev != w {
						t.Errorf("Await %d = %v, %v; want %v", i, ev, err, w)
					}
				}
			}
			c.Close()
			c.Close()
			if *released != 1 {
				t.Errorf("released %d times, want 1", *released)
			}
		})
	}
}

// TestDrainCloseUnblocksAwait: Close joins the reader and fails an Await
// waiting on an idle connection.
func TestDrainCloseUnblocksAwait(t *testing.T) {
	c, _, _ := drainClient(fireClock{})
	errc := make(chan error, 1)
	waiting := make(chan struct{})
	go func() {
		close(waiting)
		_, err := c.Await(t.Context(), time.Hour, anyEvent)
		errc <- err
	}()
	<-waiting
	c.Close()
	<-c.drained
	if err := <-errc; !errors.Is(err, net.ErrClosed) {
		t.Errorf("Await = %v, want net.ErrClosed", err)
	}
}

func TestDrainAwaitTimeout(t *testing.T) {
	c, _, _ := drainClient(fireClock{fires: time.Hour})
	defer c.Close()
	if _, err := c.Await(t.Context(), time.Minute, anyEvent); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("Await = %v, want os.ErrDeadlineExceeded", err)
	}
}

func TestInboxDrop(t *testing.T) {
	del := func(id int32) item { return item{ev: botclient.DeleteObject{ObjectID: id}} }
	own := item{ev: botclient.TeleportToLocation{ObjectID: drainSelf}}
	other := item{ev: botclient.TeleportToLocation{ObjectID: 9}}
	bad := item{err: errors.New("bad frame")}
	tests := []struct {
		name string
		push []item
		want []item
	}{
		{"under the limit", []item{del(1), del(2), del(3)}, []item{del(1), del(2), del(3)}},
		{"oldest broadcast", []item{del(1), del(2), del(3), del(4)}, []item{del(2), del(3), del(4)}},
		{"past retained ones", []item{own, bad, del(1), del(2)}, []item{own, bad, del(2)}},
		{"another's teleport", []item{own, other, del(1), del(2)}, []item{own, del(1), del(2)}},
		{"all retained", []item{own, bad, {ev: botclient.ActionFailed{}}, {ev: botclient.LeaveWorld{}}}, []item{bad, {ev: botclient.ActionFailed{}}, {ev: botclient.LeaveWorld{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newInbox(3, drainSelf)
			for _, it := range tt.push {
				b.push(it)
			}
			var got []item
			for {
				it, ok := b.pop()
				if !ok {
					break
				}
				got = append(got, it)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("kept %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i].ev != tt.want[i].ev || got[i].err != tt.want[i].err {
					t.Errorf("kept %v, want %v", got, tt.want)
					break
				}
			}
			if want := uint64(len(tt.push) - len(tt.want)); b.drops() != want {
				t.Errorf("drops() = %d, want %d", b.drops(), want)
			}
		})
	}
}

// TestClientLogout: logout reads the server's answer from the drained
// events, as botclient.GameConn's Logout reads it from the connection.
func TestClientLogout(t *testing.T) {
	tests := []struct {
		name    string
		fires   time.Duration
		frames  [][]byte
		end     bool
		want    error
		staysUp bool
	}{
		{"accepted", 0, [][]byte{frameDeleteObject(1), {botwire.OpGameLeaveWorld}}, false, nil, false},
		{"accepted after an earlier refusal", 0, [][]byte{{botwire.OpGameActionFailed}, {botwire.OpGameLeaveWorld}}, false, nil, false},
		{"refused", logoutRefusalGrace, [][]byte{{botwire.OpGameActionFailed}}, false, botclient.ErrLogoutRefused, true},
		{"server close", 0, [][]byte{{botwire.OpGameServerClose}}, false, errServerClose, false},
		{"stream ends", 0, nil, true, io.EOF, false},
		{"no answer", EntryTimeout, nil, false, os.ErrDeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, p, released := drainClient(fireClock{fires: tt.fires})
			defer c.Close()
			served := p.serve(tt.end, tt.frames...)
			err := c.Logout(t.Context())
			<-served
			if tt.want == nil && err != nil || !errors.Is(err, tt.want) {
				t.Errorf("Logout = %v, want %v", err, tt.want)
			}
			if c.Closed() == tt.staysUp || (*released == 1) == tt.staysUp {
				t.Errorf("closed %v, released %d; want closed %v", c.Closed(), *released, !tt.staysUp)
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if len(p.sent) != 1 || !bytes.Equal(p.sent[0], botwire.EncodeLogout()) {
				t.Errorf("sent %x, want one Logout", p.sent)
			}
		})
	}
}
