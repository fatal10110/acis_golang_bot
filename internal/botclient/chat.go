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
}

func newChatLog() *chatLog {
	return &chatLog{to: make(map[int32]time.Time)}
}

// allows reports whether a reply to sender at now stays within p's caps.
func (l *chatLog) allows(sender int32, now time.Time, p *ChatPolicy) bool {
	l.expire(now, p)
	if len(l.sent) >= p.Total {
		return false
	}
	last, ok := l.to[sender]
	return !ok || now.Sub(last) >= p.PerSender
}

// record counts a reply to sender sent at now.
func (l *chatLog) record(sender int32, now time.Time, p *ChatPolicy) {
	l.expire(now, p)
	for id, at := range l.to {
		if now.Sub(at) >= p.PerSender {
			delete(l.to, id)
		}
	}
	l.to[sender] = now
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
	// to is the sender's object id, at when the reply is due.
	to int32
	at time.Time
	// wake brings a decision forward to at (decisionAt) until one runs
	// there; past it the regular tick retries a reply the chat gate held.
	wake bool
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
	delay := p.DelayMin + time.Duration(b.rng.Int64N(int64(max(p.DelayMax-p.DelayMin, 0))+1))
	b.chat = chatReply{
		reply: chatReplyAction(e.Name, p.Replies[b.rng.IntN(len(p.Replies))]),
		to:    e.ObjectID,
		at:    now.Add(delay),
		wake:  true,
	}
}

// ruleChat sends the waiting whisper reply once due and the chat gate lets
// it go, and passes until then.
func (b *Brain) ruleChat(now time.Time) (Action, bool) {
	c := &b.chat
	if c.reply.kind == actHold || now.Before(c.at) || !b.pace.allow(gateChat, now) {
		return Action{}, false
	}
	return c.reply, true
}

// chatSent counts the reply sent at now and frees the slot.
func (b *Brain) chatSent(now time.Time) {
	b.cfg.chat.record(b.chat.to, now, &b.persona.Chat)
	b.chat = chatReply{}
}
