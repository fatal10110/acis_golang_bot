package botclient

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// levelRow is what fakeLevels answers for one object id.
type levelRow struct {
	level   int32
	deleted bool
	err     error
}

// fakeLevels is a LevelLookup over fixed rows that counts its queries; an
// id with no row reads as level 0.
type fakeLevels struct {
	mu      sync.Mutex
	rows    map[int32]levelRow
	queries map[int32]int
}

func newFakeLevels(rows map[int32]levelRow) *fakeLevels {
	return &fakeLevels{rows: rows, queries: make(map[int32]int)}
}

func (f *fakeLevels) AccessLevel(_ context.Context, id int32) (int32, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries[id]++
	r := f.rows[id]
	return r.level, r.deleted, r.err
}

func (f *fakeLevels) count(id int32) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[id]
}

// answer runs the database check ask queued and returns its verdict.
func answer(t *testing.T, a *Authority, reply chan verdict) verdict {
	t.Helper()
	select {
	case r := <-a.queue:
		a.check(t.Context(), r)
	default:
		t.Fatal("no check queued")
	}
	select {
	case v := <-reply:
		return v
	default:
		t.Fatal("no verdict sent")
		return verdict{}
	}
}

func TestAuthorityGrants(t *testing.T) {
	tests := []struct {
		level, min int32
		want       bool
	}{
		{-1, 0, false},
		{-5, 0, false},
		{0, 0, false},
		{5, 0, false},
		{6, 0, false},
		{7, 0, true},
		{8, 0, true},
		// Undefined levels play as the user level.
		{9, 0, false},
		{100, 0, false},
		{-1, 5, false},
		{4, 5, false},
		{5, 5, true},
		{8, 5, true},
		{9, 5, false},
	}
	for _, tt := range tests {
		a := NewAuthority(AuthorityConfig{MinLevel: tt.min})
		if got := a.grants(tt.level); got != tt.want {
			t.Errorf("grants(%d) with min %d = %v, want %v", tt.level, tt.min, got, tt.want)
		}
	}
}

func TestAuthorityAskAtOnce(t *testing.T) {
	cmd := tell{sender: gmID, name: "GM", cmd: command{kind: cmdFollow}}
	tests := []struct {
		name   string
		cfg    AuthorityConfig
		wantOK bool
	}{
		{"no commanders, no database", AuthorityConfig{}, false},
		{"commander", AuthorityConfig{Commanders: []int32{gmID}}, true},
		{"not a commander", AuthorityConfig{Commanders: []int32{pcID}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAuthority(tt.cfg)
			v, done := a.ask(cmd, t0, make(chan verdict, 1))
			if !done || v.ok != tt.wantOK || v.tell != cmd {
				t.Fatalf("ask = %+v, %v; want ok %v at once", v, done, tt.wantOK)
			}
			if got := a.knows(gmID, t0); got != tt.wantOK {
				t.Fatalf("knows = %v, want %v", got, tt.wantOK)
			}
		})
	}
}

func TestAuthorityDatabase(t *testing.T) {
	const (
		admin   = int32(101)
		headGM  = int32(102)
		user    = int32(103)
		banned  = int32(104)
		leaving = int32(105)
		broken  = int32(106)
		unknown = int32(107)
	)
	rows := map[int32]levelRow{
		admin: {level: 7}, headGM: {level: 6}, user: {level: 0}, banned: {level: -1},
		leaving: {level: 8, deleted: true}, broken: {err: errors.New("db down")},
	}
	tests := []struct {
		id         int32
		wantOK     bool
		wantCached bool // refused from the negative cache afterwards
	}{
		{admin, true, false},
		{headGM, false, true},
		{user, false, true},
		{banned, false, true},
		{leaving, false, true},
		{broken, false, false},
		{unknown, false, true},
	}
	for _, tt := range tests {
		db := newFakeLevels(rows)
		a := NewAuthority(AuthorityConfig{Lookup: db, Clock: newFakeClock(t0)})
		reply := make(chan verdict, 1)
		tl := tell{sender: tt.id, name: "x", cmd: command{kind: cmdStay}}
		if _, done := a.ask(tl, t0, reply); done {
			t.Fatalf("id %d: answered at once, want a database check", tt.id)
		}
		if v := answer(t, a, reply); v.ok != tt.wantOK || v.tell != tl {
			t.Fatalf("id %d: verdict %+v, want ok %v", tt.id, v, tt.wantOK)
		}
		if got := a.knows(tt.id, t0); got != tt.wantOK {
			t.Errorf("id %d: knows = %v, want %v", tt.id, got, tt.wantOK)
		}
		_, done := a.ask(tl, at(time.Second), reply)
		if done != tt.wantCached {
			t.Errorf("id %d: second ask at once = %v, want %v", tt.id, done, tt.wantCached)
		}
	}
}

// TestAuthorityNegativeCache checks that a non-GM's commands cost one
// query a minute, however many it whispers.
func TestAuthorityNegativeCache(t *testing.T) {
	db := newFakeLevels(map[int32]levelRow{pcID: {level: 0}})
	clock := newFakeClock(t0)
	a := NewAuthority(AuthorityConfig{Lookup: db, Clock: clock})
	reply := make(chan verdict, 1)
	tl := tell{sender: pcID, name: "other", cmd: command{kind: cmdFollow}}
	if _, done := a.ask(tl, t0, reply); done {
		t.Fatal("first command answered at once")
	}
	answer(t, a, reply)
	for i := range 50 {
		now := at(time.Duration(i) * time.Second)
		if v, done := a.ask(tl, now, reply); !done || v.ok {
			t.Fatalf("command %d at %v: %+v, %v; want refused at once", i, now.Sub(t0), v, done)
		}
	}
	if got := db.count(pcID); got != 1 {
		t.Fatalf("%d queries, want 1", got)
	}
	if _, done := a.ask(tl, at(deniedFor), reply); done {
		t.Fatal("command after the negative cache expired answered at once")
	}
	clock.Advance(deniedFor)
	answer(t, a, reply)
	if got := db.count(pcID); got != 2 {
		t.Fatalf("%d queries, want 2", got)
	}
}

func TestAuthorityTokenBucket(t *testing.T) {
	db := newFakeLevels(map[int32]levelRow{gmID: {level: 8}})
	a := NewAuthority(AuthorityConfig{Lookup: db, Clock: newFakeClock(t0)})
	reply := make(chan verdict, authQueue)
	tl := tell{sender: gmID, name: "GM", cmd: command{kind: cmdFollow}}
	steps := []struct {
		at       time.Duration
		wantDone bool
	}{
		{0, false},
		{0, false},
		{0, false},
		{0, false},
		{0, false},
		{0, true},
		{authRefill / 2, true},
		{authRefill, false},
		{authRefill, true},
		{3 * authRefill, false},
		{3 * authRefill, false},
		{3 * authRefill, true},
	}
	for i, s := range steps {
		v, done := a.ask(tl, at(s.at), reply)
		if done != s.wantDone || (done && v.ok) {
			t.Fatalf("step %d at %v: %+v, %v; want at once %v", i, s.at, v, done, s.wantDone)
		}
	}
}

func TestAuthorityQueueFull(t *testing.T) {
	a := NewAuthority(AuthorityConfig{Lookup: newFakeLevels(nil), Clock: newFakeClock(t0)})
	reply := make(chan verdict, 1)
	for i := range authQueue {
		if _, done := a.ask(tell{sender: int32(1000 + i)}, t0, reply); done {
			t.Fatalf("check %d answered at once", i)
		}
	}
	if v, done := a.ask(tell{sender: 5000}, t0, reply); !done || v.ok {
		t.Fatalf("past the queue: %+v, %v; want refused at once", v, done)
	}
}

// TestAuthorityVerdictDropped checks that a check never blocks on a brain
// whose verdict queue is full.
func TestAuthorityVerdictDropped(t *testing.T) {
	a := NewAuthority(AuthorityConfig{Lookup: newFakeLevels(map[int32]levelRow{gmID: {level: 7}}), Clock: newFakeClock(t0)})
	reply := make(chan verdict, 1)
	reply <- verdict{}
	a.ask(tell{sender: gmID}, t0, reply)
	a.check(t.Context(), <-a.queue)
	if len(reply) != 1 {
		t.Fatalf("%d verdicts queued, want the 1 already there", len(reply))
	}
}

func TestAuthorityRun(t *testing.T) {
	db := newFakeLevels(map[int32]levelRow{gmID: {level: 7}})
	a := NewAuthority(AuthorityConfig{Lookup: db, Clock: newFakeClock(t0)})
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(stopped)
	}()
	reply := make(chan verdict, 1)
	tl := tell{sender: gmID, name: "GM", cmd: command{kind: cmdLeave}}
	if _, done := a.ask(tl, t0, reply); done {
		t.Fatal("answered at once")
	}
	if v := <-reply; !v.ok || v.tell != tl {
		t.Fatalf("verdict %+v, want ok", v)
	}
	cancel()
	<-stopped
	// Without a database Run returns at once.
	NewAuthority(AuthorityConfig{}).Run(t.Context())
}

func TestAuthoritySweep(t *testing.T) {
	a := NewAuthority(AuthorityConfig{Lookup: newFakeLevels(nil), Clock: newFakeClock(t0)})
	for i := range authSweepAbove + 1 {
		id := int32(1000 + i)
		a.denied[id] = t0
		a.buckets[id] = &tokenBucket{tokens: authBurst, filled: t0}
		a.known[id] = knownGM{at: t0}
	}
	a.mu.Lock()
	a.sweep(at(knownFor))
	a.mu.Unlock()
	if len(a.denied)+len(a.buckets)+len(a.known) != 0 {
		t.Fatalf("left %d denied, %d buckets, %d known", len(a.denied), len(a.buckets), len(a.known))
	}
}

func TestAuthorityHooks(t *testing.T) {
	const farGM = int32(0x30000099)
	tests := []struct {
		name      string
		cfg       AuthorityConfig
		whisper   bool // the commander whispered a command first
		join      func(*World, string, time.Time) bool
		requester string
		when      time.Duration
		wantJoin  bool
	}{
		{"commander in sight", AuthorityConfig{Commanders: []int32{pcID}}, false, nil, "other", 0, true},
		{"commander out of sight, never whispered", AuthorityConfig{Commanders: []int32{farGM}}, false, nil, "Far", 0, false},
		{"commander out of sight, whispered", AuthorityConfig{Commanders: []int32{farGM}}, true, nil, "Far", time.Hour, true},
		{"player, persona declines", AuthorityConfig{Commanders: []int32{farGM}}, false, nil, "other", 0, false},
		{"player, persona accepts", AuthorityConfig{Commanders: []int32{farGM}}, false, acceptParty, "other", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			a := NewAuthority(tt.cfg)
			if tt.whisper {
				a.ask(tell{sender: farGM, name: "Far"}, t0, nil)
			}
			p := a.Hooks(Persona{JoinParty: tt.join})
			if got := p.JoinParty(w, tt.requester, at(tt.when)); got != tt.wantJoin {
				t.Fatalf("JoinParty(%q) = %v, want %v", tt.requester, got, tt.wantJoin)
			}
		})
	}
}

// TestAuthorityHooksDatabase checks that the hooks know a GM a database
// check confirmed for knownFor, and never query themselves.
func TestAuthorityHooksDatabase(t *testing.T) {
	db := newFakeLevels(map[int32]levelRow{gmID: {level: 7}})
	clock := newFakeClock(t0)
	a := NewAuthority(AuthorityConfig{Lookup: db, Clock: clock})
	ownExempt := int32(42)
	p := a.Hooks(Persona{Trade: TradePolicy{Exempt: func(_ *World, id int32) bool { return id == ownExempt }}})
	w := newTestWorld(t)
	if p.JoinParty(w, "GM", t0) || p.Trade.Exempt(w, gmID) {
		t.Fatal("unchecked GM known")
	}
	reply := make(chan verdict, 1)
	a.ask(tell{sender: gmID, name: "GM"}, t0, reply)
	answer(t, a, reply)
	if !p.JoinParty(w, "GM", t0) || !p.Trade.Exempt(w, gmID) || !p.Trade.Exempt(w, ownExempt) {
		t.Fatal("checked GM or the persona's own exempt not known")
	}
	if p.Trade.Exempt(w, pcID) {
		t.Fatal("player exempt")
	}
	clock.Advance(knownFor)
	if p.JoinParty(w, "GM", at(knownFor)) || p.Trade.Exempt(w, gmID) {
		t.Fatal("GM still known after knownFor")
	}
	if got := db.count(gmID); got != 1 {
		t.Fatalf("%d queries, want only the command's 1", got)
	}
}
