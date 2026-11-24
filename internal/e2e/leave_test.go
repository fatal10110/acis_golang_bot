package e2e

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// fakeLeaver answers each logout with the next error of logouts and records
// every step leave takes.
type fakeLeaver struct {
	logouts []error
	waitErr error
	steps   []string
}

func (f *fakeLeaver) logout(context.Context) error {
	f.steps = append(f.steps, "logout")
	err := f.logouts[0]
	f.logouts = f.logouts[1:]
	return err
}

func (f *fakeLeaver) drop() { f.steps = append(f.steps, "drop") }

func (f *fakeLeaver) wait(_ context.Context, d time.Duration) error {
	f.steps = append(f.steps, "wait "+d.String())
	return f.waitErr
}

func TestLeave(t *testing.T) {
	lost := errors.New("write: broken pipe")
	refused := botclient.ErrLogoutRefused
	linger := "wait " + leaveLinger.String()
	tests := []struct {
		name    string
		logouts []error
		waitErr error
		steps   []string
		want    []error
	}{
		{"clean logout", []error{nil}, nil, []string{"logout"}, nil},
		{
			"refused in attack stance, then clean",
			[]error{refused, nil},
			nil,
			[]string{"logout", linger, "logout"},
			nil,
		},
		{
			"connection lost: the character lingers",
			[]error{lost},
			nil,
			[]string{"logout", "drop", linger},
			[]error{lost},
		},
		{
			"refused twice: dropped, then the linger",
			[]error{refused, refused},
			nil,
			[]string{"logout", linger, "logout", "drop", linger},
			[]error{refused},
		},
		{
			"refused, then lost",
			[]error{refused, lost},
			nil,
			[]string{"logout", linger, "logout", "drop", linger},
			[]error{lost},
		},
		{
			"refused and the cleanup ends while waiting",
			[]error{refused},
			context.DeadlineExceeded,
			[]string{"logout", linger, "drop"},
			[]error{refused, context.DeadlineExceeded},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLeaver{logouts: tt.logouts, waitErr: tt.waitErr}
			err := leave(t.Context(), f)
			if !slices.Equal(f.steps, tt.steps) {
				t.Errorf("steps = %q, want %q", f.steps, tt.steps)
			}
			if (err == nil) != (len(tt.want) == 0) {
				t.Fatalf("leave = %v, want %v", err, tt.want)
			}
			for _, w := range tt.want {
				if !errors.Is(err, w) {
					t.Errorf("leave = %v, want it to wrap %v", err, w)
				}
			}
			if len(f.logouts) != 0 {
				t.Errorf("%d logouts not sent", len(f.logouts))
			}
		})
	}
}
