package botclient

import (
	"time"
)

// ChatPolicy is how the bot answers whispers (rule "chat", docs/plan.md
// "Names and human feel"): only a whisper, never another channel, after a
// delay, at most once per sender per PerSender and at most Total times per
// Window for the bot. Every value is a persona default (D6); a reply also
// passes the chat gate (chatGap), the pace that bounds it since whispers
// have no server gate.
type ChatPolicy struct {
	// Replies are the lines a reply picks from at random; none never
	// replies. The server drops a whisper that starts with a walker
	// command ("SAY", "MSG", "EXIT" and others, upper case) and removes
	// literal `\n` pairs (handler/chat/chat.go Admit and Clean), so a line
	// has neither.
	Replies []string
	// DelayMin and DelayMax bound the uniform delay between the whisper
	// and the reply.
	DelayMin, DelayMax time.Duration
	// PerSender is the least time between two replies to one sender.
	PerSender time.Duration
	// Total is how many replies the bot sends within any Window, across
	// its sessions; 0 never replies. Window 0 counts them for the bot's
	// whole life.
	Total  int
	Window time.Duration
}

func defaultChatPolicy() ChatPolicy {
	return ChatPolicy{
		Replies:   []string{"hi", "hey", "busy now, sorry", "cant talk, farming", "hm?", "not now", "lol", "brb"},
		DelayMin:  3 * time.Second,
		DelayMax:  12 * time.Second,
		PerSender: 10 * time.Minute,
		Total:     5,
		Window:    time.Hour,
	}
}

// chatLog is when the bot replied to whispers, for the caps. It outlives a
// session: a Bot hands its log to the brain of each session it runs, one at
// a time, so its only owner is the current session's brain goroutine.
type chatLog struct {
	// to is when the bot last replied to each sender, by object id, for
	// PerSender; entries past PerSender are swept on each reply.
	to map[int32]time.Time
	// sent are the replies within Window, oldest first.
	sent []time.Time
	// joins are the whispers answering a join within joinReplyWindow,
	// oldest first, capped by LeadPolicy.JoinReplies apart from sent.
	joins []time.Time
}

// joinReplyWindow is the window of LeadPolicy.JoinReplies: an hour, as
// plan section 4's per-bot cap of whispers counts.
const joinReplyWindow = time.Hour

// joinAllows reports whether a join reply at now stays within limit, and
// counts it when it does.
func (l *chatLog) joinAllows(now time.Time, limit int) bool {
	i := 0
	for i < len(l.joins) && now.Sub(l.joins[i]) >= joinReplyWindow {
		i++
	}
	l.joins = l.joins[i:]
	if len(l.joins) >= limit {
		return false
	}
	l.joins = append(l.joins, now)
	return true
}

func newChatLog() *chatLog {
	return &chatLog{to: make(map[int32]time.Time)}
}

// allows reports whether a reply to sender at now stays within p's caps.
func (l *chatLog) allows(sender int32, now time.Time, p *ChatPolicy) bool {
	if l.full(now, p) {
		return false
	}
	last, ok := l.to[sender]
	return !ok || now.Sub(last) >= p.PerSender
}

// full reports whether p's Total is reached at now.
func (l *chatLog) full(now time.Time, p *ChatPolicy) bool {
	l.expire(now, p)
	return len(l.sent) >= p.Total
}

// record counts a reply to sender sent at now.
func (l *chatLog) record(sender int32, now time.Time, p *ChatPolicy) {
	for id, at := range l.to {
		if now.Sub(at) >= p.PerSender {
			delete(l.to, id)
		}
	}
	l.to[sender] = now
	l.count(now, p)
}

// count counts a line the bot said at now in p's Total: a reply, a craft
// order's answer or announcement.
func (l *chatLog) count(now time.Time, p *ChatPolicy) {
	l.expire(now, p)
	l.sent = append(l.sent, now)
}

// expire drops the replies older than Window.
func (l *chatLog) expire(now time.Time, p *ChatPolicy) {
	if p.Window <= 0 {
		return
	}
	i := 0
	for i < len(l.sent) && now.Sub(l.sent[i]) >= p.Window {
		i++
	}
	l.sent = l.sent[i:]
}

// chatReply is the whisper reply waiting for its time, owned by the brain
// goroutine. One waits at a time: a whisper heard meanwhile gets none.
type chatReply struct {
	// reply is the Say2 Tell (actChatReply), zero for none.
	reply Action
	// to is the sender's object id.
	to int32
	// alarm's at is when the reply is due; it brings a decision forward to
	// at (decisionAt) until one runs there; past it the regular tick
	// retries a reply the chat gate held.
	alarm
}

// chatReplyAction whispers text back to the player named target. It
// passes the chat gate like every whisper.
func chatReplyAction(target, text string) Action {
	return Action{kind: actChatReply, payload: encodeSay2Tell(target, text)}
}

// chatHeard schedules a reply at now to a whisper that is no command (heard
// filtered the channel, echoes and the bot's own lines): none to a sender
// whose name carries the bot marker, none while another waits, none past
// the caps.
func (b *Brain) chatHeard(e CreatureSay, now time.Time) {
	p := &b.persona.Chat
	if len(p.Replies) == 0 || p.Total <= 0 || e.Name == "" || b.chat.reply.kind != actHold ||
		b.cfg.markers.Marks(e.Name) || !b.cfg.chat.allows(e.ObjectID, now, p) {
		return
	}
	b.chat = chatReply{
		reply: chatReplyAction(e.Name, p.Replies[b.rng.IntN(len(p.Replies))]),
		to:    e.ObjectID,
		alarm: alarm{at: now.Add(b.chatDelay()), armed: true},
	}
}

// chatDelay is a uniform delay in ChatPolicy's DelayMin-DelayMax.
func (b *Brain) chatDelay() time.Duration {
	p := &b.persona.Chat
	return b.between(p.DelayMin, p.DelayMax)
}

// ruleChat sends the waiting whisper reply once due and the chat gate lets
// it go, and passes until then: the canned one or the valuables
// responder's (ruleWhisper), the earliest due first.
//
// Pacing: one Say2 Tell per chatGap through the frame bucket; the canned
// replies within ChatPolicy's caps (chatLog), the responder's within
// ValuablePolicy's Replies and RepliesPerPartner a round (whisperTalk).
// Whispers have no server gate.
func (b *Brain) ruleChat(now time.Time) (Action, bool) {
	b.whispers.out = whisperOut{}
	if !b.pace.allow(gateChat, now) {
		return Action{}, false
	}
	c := &b.chat
	var canned time.Time
	if c.reply.kind != actHold && !now.Before(c.at) {
		canned = c.at
	}
	if a, ok := b.ruleWhisper(now, canned); ok {
		return a, true
	}
	if canned.IsZero() {
		return Action{}, false
	}
	return c.reply, true
}

// chatSent counts the reply sent at now and frees its slot: the
// responder's in its round's caps, the canned one in chatLog.
func (b *Brain) chatSent(now time.Time) {
	if o := b.whispers.out; o.to != 0 {
		b.whisperSent(o)
		return
	}
	b.cfg.chat.record(b.chat.to, now, &b.persona.Chat)
	b.chat = chatReply{}
}
