package wire

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"testing"
)

// cutReader hands out its stream in the chunks given, failing with a read
// deadline between chunks, the way a net.Conn read past its deadline fails
// while the rest of a frame is still in flight.
type cutReader struct {
	chunks [][]byte
	cut    bool // the next Read fails with the deadline
}

func (r *cutReader) Read(p []byte) (int, error) {
	if r.cut {
		r.cut = false
		return 0, os.ErrDeadlineExceeded
	}
	if len(r.chunks) == 0 {
		return 0, errors.New("stream drained")
	}
	n := copy(p, r.chunks[0])
	if r.chunks[0] = r.chunks[0][n:]; len(r.chunks[0]) == 0 {
		r.chunks = r.chunks[1:]
		r.cut = true
	}
	return n, nil
}

// TestFrameReaderResumesAfterDeadline cuts a two-frame stream at every
// offset: the read the deadline interrupts fails with it, and the next
// ReadFrame resumes the frame where it stopped instead of reading a header
// out of its middle (#311).
func TestFrameReaderResumesAfterDeadline(t *testing.T) {
	first := []byte{0x03, 0x10, 0x20, 0x30, 0x40, 0x50}
	second := []byte{0x04, 0xaa, 0xbb}
	var stream bytes.Buffer
	for _, p := range [][]byte{first, second} {
		if err := WriteFrame(&stream, p); err != nil {
			t.Fatal(err)
		}
	}
	all := stream.Bytes()
	for cut := 1; cut < len(all); cut++ {
		src := &cutReader{chunks: [][]byte{bytes.Clone(all[:cut]), bytes.Clone(all[cut:])}}
		frames := &FrameReader{r: bufio.NewReader(src)}
		var got [][]byte
		for len(got) < 2 {
			payload, err := frames.ReadFrame()
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			if err != nil {
				t.Fatalf("cut at %d: frame %d: %v", cut, len(got), err)
			}
			got = append(got, bytes.Clone(payload))
		}
		if !bytes.Equal(got[0], first) || !bytes.Equal(got[1], second) {
			t.Fatalf("cut at %d: frames %x, %x; want %x, %x", cut, got[0], got[1], first, second)
		}
	}
}

// TestFrameReaderReadsOnAfterShortHeader: a header shorter than itself is
// consumed with its error, and the frame after it reads whole.
func TestFrameReaderReadsOnAfterShortHeader(t *testing.T) {
	frames := NewFrameReader(bytes.NewReader([]byte{0x01, 0x00, 0x03, 0x00, 0x07}))
	if _, err := frames.ReadFrame(); !errors.Is(err, ErrFrameTooShort) {
		t.Fatalf("first ReadFrame() error = %v, want ErrFrameTooShort", err)
	}
	payload, err := frames.ReadFrame()
	if err != nil || !bytes.Equal(payload, []byte{0x07}) {
		t.Fatalf("second ReadFrame() = %x, %v; want 07", payload, err)
	}
}
