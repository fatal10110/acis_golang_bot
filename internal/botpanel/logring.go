package botpanel

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// accountKey is the attribute that names a bot's account in a log line:
// every log line of a bot (botclient.NewBot) and of the supervisor about
// one carries it.
const accountKey = "account"

// Line is one log line of the ring.
type Line struct {
	At    time.Time `json:"at"`
	Level string    `json:"level"`
	// Account is the line's account attribute, "" for a line about no bot.
	Account string `json:"account,omitempty"`
	// Text is the line as the text handler writes it, without its newline.
	Text string `json:"text"`
}

// Ring keeps the latest log lines for the panel, at most its size: a newer
// line drops the oldest. It is safe from any goroutine.
type Ring struct {
	mu    sync.Mutex
	lines []Line
	// next is where the next line goes once lines is full.
	next int
	size int
	// buf is where the ring's text handlers write a line, under mu.
	buf bytes.Buffer
}

// NewRing returns a ring of the latest size lines.
func NewRing(size int) *Ring {
	return &Ring{lines: make([]Line, 0, size), size: max(size, 1)}
}

// Lines returns the latest n lines, oldest first, of account's bot, or of
// every bot and none for an empty account; n <= 0 is every line kept.
func (r *Ring) Lines(account string, n int) []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Line, 0, len(r.lines))
	for i := range r.lines {
		l := r.lines[(r.next+i)%len(r.lines)]
		if account == "" || l.Account == account {
			out = append(out, l)
		}
	}
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func (r *Ring) push(l Line) {
	if len(r.lines) < r.size {
		r.lines = append(r.lines, l)
		return
	}
	r.lines[r.next] = l
	r.next = (r.next + 1) % r.size
}

// Handler returns a slog.Handler that passes every record to next and keeps
// it in the ring, formatted as slog's text handler does with opts.
func (r *Ring) Handler(next slog.Handler, opts *slog.HandlerOptions) slog.Handler {
	return &ringHandler{ring: r, next: next, text: slog.NewTextHandler(&r.buf, opts)}
}

// ringHandler is a Ring's slog.Handler: text writes into the ring's buffer,
// and account is the account attribute its WithAttrs added outside any
// group.
type ringHandler struct {
	ring    *Ring
	next    slog.Handler
	text    slog.Handler
	account string
	grouped bool
}

func (h *ringHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l) || h.text.Enabled(ctx, l)
}

func (h *ringHandler) Handle(ctx context.Context, rec slog.Record) error {
	var err error
	if h.next.Enabled(ctx, rec.Level) {
		err = h.next.Handle(ctx, rec)
	}
	if !h.text.Enabled(ctx, rec.Level) {
		return err
	}
	account := h.account
	if !h.grouped {
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == accountKey {
				account = a.Value.String()
				return false
			}
			return true
		})
	}
	r := h.ring
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Reset()
	if terr := h.text.Handle(ctx, rec); terr != nil {
		return err
	}
	r.push(Line{At: rec.Time, Level: rec.Level.String(), Account: account, Text: strings.TrimSuffix(r.buf.String(), "\n")})
	return err
}

func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.next, c.text = h.next.WithAttrs(attrs), h.text.WithAttrs(attrs)
	if !h.grouped {
		for _, a := range attrs {
			if a.Key == accountKey {
				c.account = a.Value.String()
			}
		}
	}
	return &c
}

func (h *ringHandler) WithGroup(name string) slog.Handler {
	c := *h
	c.next, c.text = h.next.WithGroup(name), h.text.WithGroup(name)
	c.grouped = c.grouped || name != ""
	return &c
}
