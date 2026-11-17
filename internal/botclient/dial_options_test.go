package botclient

import (
	"errors"
	"net"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// fakeRawConn runs Control on a made-up descriptor, or fails it.
type fakeRawConn struct{ err error }

const fakeFD = 42

func (c fakeRawConn) Control(f func(fd uintptr)) error {
	if c.err != nil {
		return c.err
	}
	f(fakeFD)
	return nil
}
func (fakeRawConn) Read(func(fd uintptr) bool) error  { return errors.ErrUnsupported }
func (fakeRawConn) Write(func(fd uintptr) bool) error { return errors.ErrUnsupported }

func TestDialerSocketOptions(t *testing.T) {
	d := newDialer(3*time.Second, nil)
	if d.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", d.Timeout)
	}
	wantKA := net.KeepAliveConfig{Enable: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3}
	if d.KeepAliveConfig != wantKA {
		t.Errorf("KeepAliveConfig = %+v, want %+v", d.KeepAliveConfig, wantKA)
	}

	// TCP_USER_TIMEOUT (0x12 in linux/tcp.h) at the keepalive's give-up
	// time, in milliseconds; other systems set nothing beyond the keepalive.
	var want []sockopt
	if runtime.GOOS == "linux" {
		want = []sockopt{{level: syscall.IPPROTO_TCP, name: 0x12, value: 60000}}
	}
	var got []sockopt
	set := func(fd uintptr, level, name, value int) error {
		if fd != fakeFD {
			t.Errorf("fd = %d, want %d", fd, fakeFD)
		}
		got = append(got, sockopt{level, name, value})
		return nil
	}
	d = newDialer(time.Second, set)
	if d.Control == nil {
		if want != nil {
			t.Fatal("no Control, want the socket options set")
		}
		return
	}
	if err := d.Control("tcp4", "127.0.0.1:2106", fakeRawConn{}); err != nil {
		t.Fatalf("Control = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("socket options = %+v, want %+v", got, want)
	}
}

func TestControlSockoptsErrors(t *testing.T) {
	opts := []sockopt{{level: 6, name: 1, value: 2}, {level: 6, name: 3, value: 4}}
	errSet, errRaw := errors.New("setsockopt refused"), errors.New("raw conn closed")
	tests := []struct {
		name    string
		raw     fakeRawConn
		failAt  int // the call of set that fails, from 1; 0: none
		want    error
		wantSet int
	}{
		{name: "all set", wantSet: 2},
		{name: "first refused", failAt: 1, want: errSet, wantSet: 1},
		{name: "second refused", failAt: 2, want: errSet, wantSet: 2},
		{name: "raw conn fails", raw: fakeRawConn{err: errRaw}, want: errRaw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := 0
			set := func(uintptr, int, int, int) error {
				n++
				if n == tt.failAt {
					return errSet
				}
				return nil
			}
			err := controlSockopts(opts, set)("tcp", "x:1", tt.raw)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("Control = %v, want %v", err, tt.want)
			}
			if n != tt.wantSet {
				t.Errorf("set called %d times, want %d", n, tt.wantSet)
			}
		})
	}
}
