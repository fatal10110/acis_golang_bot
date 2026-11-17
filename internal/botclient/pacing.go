package botclient

import "time"

// Bot-side send limits. Every frame the brain sends spends a token of one
// frame bucket, and some kinds also wait out a reuse gate. Each limit sits
// well inside the server rule it guards (server paths under
// internal/gameserver/network). They are operational limits, not persona
// defaults (D6), so they are constants.
const (
	// frameRate and frameBurst bound every frame. A full bucket allows at
	// most frameBurst+frameRate (15) frames in any second against the
	// server's 160 (clientstats.go maxPacketsPerSecond), and
	// frameBurst+5*frameRate (35) in any 5 s against its 400 (80/s averaged
	// over floodMeasureInterval). Two floods within a minute disconnect.
	frameRate  = 5
	frameBurst = 10
	// emoteGap spaces RequestSocialAction. The server silently refuses an
	// emote within SocialTime (2 s by default) of the previous one
	// (client_ingame.go handleRequestSocialAction).
	emoteGap = 3 * time.Second
	// chatGap spaces Say2 on the all channel. Its server gate,
	// GlobalChatTime, defaults to 0 (chat.go chatAll); a human pace keeps
	// bots from flooding the chat of the players around them.
	chatGap = 3 * time.Second
	// bypassGap spaces RequestBypassToServer. The server silently drops a
	// bypass within ServerBypassTime (100 ms by default) of the previous one
	// (client_ingame.go handleRequestBypassToServer); a dialog link is
	// clicked at a human pace.
	bypassGap = 500 * time.Millisecond
	// dropGap spaces RequestDropItem. The server silently ignores a drop
	// within DropItemTime (1 s by default) of the previous one
	// (config_gates.go handleRequestDropItem).
	dropGap = 2 * time.Second
	// keepaliveAfter is how long the brain may stay silent before it sends
	// ValidatePosition. The server closes an authenticated connection that
	// sends nothing for 15 minutes (session.go clientReadIdleTimeout).
	keepaliveAfter = 5 * time.Minute
)

// gate is the reuse gate a frame passes on top of the frame bucket.
type gate uint8

const (
	gateNone gate = iota
	gateEmote
	gateChat
	gateBypass
	gateDrop
	gateCount
)

// gateGaps is the least time between two frames of one gate.
var gateGaps = [gateCount]time.Duration{gateEmote: emoteGap, gateChat: chatGap, gateBypass: bypassGap, gateDrop: dropGap}

// pacer applies the send limits. It is owned by the brain goroutine, the
// connection's only writer while the brain runs.
type pacer struct {
	tokens float64
	// filled is when tokens was last refilled.
	filled time.Time
	// gateSent is the last send of each gate, zero before the first.
	gateSent [gateCount]time.Time
	// lastSent is the last send of any frame; the brain started at it.
	lastSent time.Time
}

// newPacer returns a pacer with a full bucket for a brain starting at now.
func newPacer(now time.Time) pacer {
	return pacer{tokens: frameBurst, filled: now, lastSent: now}
}

func (p *pacer) refill(now time.Time) {
	if now.After(p.filled) {
		p.tokens = min(frameBurst, p.tokens+now.Sub(p.filled).Seconds()*frameRate)
		p.filled = now
	}
}

// allow reports whether a frame passing g may go out at now.
func (p *pacer) allow(g gate, now time.Time) bool {
	p.refill(now)
	if p.tokens < 1 {
		return false
	}
	last := p.gateSent[g]
	return gateGaps[g] == 0 || last.IsZero() || now.Sub(last) >= gateGaps[g]
}

// sent records a frame passing g that went out at now.
func (p *pacer) sent(g gate, now time.Time) {
	p.refill(now)
	p.tokens--
	p.gateSent[g] = now
	p.lastSent = now
}
