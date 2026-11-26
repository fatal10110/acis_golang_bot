package botregistry

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// fakeBot is the bot of a fake run: the test sets its snapshot, fills its
// mailbox and answers its commands.
type fakeBot struct {
	mu   sync.Mutex
	st   botclient.Status
	busy bool
	// cmds are every command handed, a busy one included; answers the
	// answer channels of those taken.
	cmds    []botclient.Command
	answers []chan error
}

// newFakeBot is an online bot of character name with no party slot.
func newFakeBot(name string) *fakeBot {
	return &fakeBot{st: botclient.Status{State: botclient.StateOnline, Character: name, Level: 20, HP: 100}}
}

func (f *fakeBot) Status() botclient.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}

func (f *fakeBot) Command(c botclient.Command) (<-chan error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, c)
	if f.busy {
		return nil, botclient.ErrBusy
	}
	ch := make(chan error, 1)
	f.answers = append(f.answers, ch)
	return ch, nil
}

func (f *fakeBot) Inspect(time.Duration) (<-chan botclient.Inspection, error) {
	return nil, botclient.ErrOffline
}

// set edits the snapshot.
func (f *fakeBot) set(fn func(*botclient.Status)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.st)
}

func (f *fakeBot) setBusy(busy bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.busy = busy
}

// handed are the commands handed so far, as the panel writes them.
func (f *fakeBot) handed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.cmds))
	for i, c := range f.cmds {
		out[i] = c.String()
	}
	return out
}

// answer answers the latest command taken with err and sets the slot the
// brain sets before it answers.
func (f *fakeBot) answer(err error, slot string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		f.st.Party, f.st.PartyLeft = slot, ""
	}
	f.answers[len(f.answers)-1] <- err
}

// handOver hands f to the supervisor as account's live run's bot, as
// Run.Live does, and returns once the supervisor holds it.
func (h *harness) handOver(t *testing.T, account string, f *fakeBot) {
	t.Helper()
	h.sup.mail.post(event{account: account, bot: f})
	for range 10000 {
		if s := h.list(t)[account]; s.Live != nil && s.Live.Character == f.Status().Character {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("supervisor never held the bot of %s", account)
}

// restored are the runs of hz1 and hz2 the supervisor restored, which start
// in any order.
func (h *harness) restored(t *testing.T) (*fakeRun, *fakeRun) {
	t.Helper()
	a, b := h.next(t), h.next(t)
	if a.Account != "hz1" {
		a, b = b, a
	}
	if a.Account != "hz1" || b.Account != "hz2" {
		t.Fatalf("restored %s and %s, want hz1 and hz2", a.Account, b.Account)
	}
	return a, b
}

// partyPass moves the clock on by PartyTick and returns once the supervisor
// ran the pass: it re-arms the party timer after the pass.
func (h *harness) partyPass(t *testing.T) {
	t.Helper()
	h.list(t) // Run set the timer before it served this request.
	pt := h.sup.partyTimer.(*fakeTimer)
	h.clock.Advance(PartyTick)
	for range 10000 {
		h.clock.mu.Lock()
		armed := pt.active
		h.clock.mu.Unlock()
		if armed {
			return
		}
		// Run serves this request before or after the pass, never
		// during it.
		h.list(t)
	}
	t.Fatal("the party pass never ran")
}

// party reads the organizer's state and account's Sent and new-run mark on
// the supervisor goroutine.
func (h *harness) party(t *testing.T, account string) (botparty.State, botparty.Sent, bool) {
	t.Helper()
	var st botparty.State
	var sent botparty.Sent
	var newRun bool
	if err := h.sup.do(t.Context(), func() {
		st = h.sup.party
		if r := h.sup.runs[account]; r != nil {
			sent, newRun = r.party.sent, r.party.newRun
		}
	}); err != nil {
		t.Fatal(err)
	}
	return st, sent, newRun
}

func withParty(c *Config) {
	p := botparty.DefaultPolicy()
	c.Party = &p
	c.CheckPreset = nil
}

func wantHanded(t *testing.T, step string, f *fakeBot, want ...string) {
	t.Helper()
	if got := f.handed(); !slices.Equal(got, want) {
		t.Fatalf("%s: handed %q, want %q", step, got, want)
	}
}

func TestSupervisorPartyTick(t *testing.T) {
	dir := t.TempDir()
	reg := openPanel(t, dir)
	// The test writes the pin's rows: farmer Lead leads support Mem.
	for _, b := range []Bot{
		{Name: "Lead", Class: ptr(0), Preset: botparty.PresetFarmer, Online: true, PartyGroup: "Pin", PartyLead: true},
		{Name: "Mem", Class: ptr(10), Preset: botparty.PresetSupport, Online: true, PartyGroup: "Pin"},
	} {
		if _, err := reg.NewBot(b); err != nil {
			t.Fatal(err)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(dir, FileName)); err != nil || !bytes.Contains(raw, []byte("party_group")) || !bytes.Contains(raw, []byte("party_lead")) {
		t.Fatalf("saved registry without the pin rows (%v):\n%s", err, raw)
	}
	clock := newClock()
	h := startSupervisor(t, reg, clock, 5, withParty)
	r1, r2 := h.restored(t)
	lead, mem := newFakeBot("Lead"), newFakeBot("Mem")
	h.handOver(t, "hz1", lead)
	h.handOver(t, "hz2", mem)
	if _, _, newRun := h.party(t, "hz2"); !newRun {
		t.Fatal("a bot handed over is not a new run")
	}

	clock.Advance(PartyTick - time.Second)
	h.list(t)
	wantHanded(t, "before the first tick", mem)

	// The pin forms: the member's join first, through Bot.Command.
	h.partyPass(t)
	wantHanded(t, "first pass", mem, "join Lead")
	wantHanded(t, "first pass", lead)
	st, sent, newRun := h.party(t, "hz2")
	if sent.Answer != botparty.AnswerPending || sent.Order.Slot() != "join Lead" || newRun {
		t.Fatalf("member after the first pass: sent %+v new run %v, want the join pending", sent, newRun)
	}
	if len(st.Parties) != 1 || !st.Parties[0].Pinned || st.NextID != 1 {
		t.Fatalf("state %+v, want the pin formed", st)
	}

	// Unanswered, the member gets nothing more and the leader no lead.
	h.partyPass(t)
	wantHanded(t, "join pending", mem, "join Lead")
	wantHanded(t, "join pending", lead)

	// The answer is polled before the pass reads the snapshot; the
	// leader's mailbox is full, so its lead waits for the next pass.
	mem.answer(nil, "join Lead")
	lead.setBusy(true)
	h.partyPass(t)
	if _, sent, _ := h.party(t, "hz2"); sent.Answer != botparty.AnswerOK {
		t.Fatalf("member's Sent %+v after its answer, want OK", sent)
	}
	wantHanded(t, "leader busy", lead, "lead Mem")
	if _, sent, _ := h.party(t, "hz1"); sent.Answer != botparty.AnswerNone {
		t.Fatalf("leader's Sent %+v after ErrBusy, want none", sent)
	}

	lead.setBusy(false)
	h.partyPass(t)
	wantHanded(t, "busy retried", lead, "lead Mem", "lead Mem")

	// An order answered with an error is planned again.
	lead.answer(botclient.ErrStopped, "")
	h.partyPass(t)
	wantHanded(t, "error answer", lead, "lead Mem", "lead Mem", "lead Mem")
	lead.answer(nil, "lead Mem")
	h.partyPass(t)
	wantHanded(t, "formed", lead, "lead Mem", "lead Mem", "lead Mem")
	wantHanded(t, "formed", mem, "join Lead")
	if st, _, _ := h.party(t, "hz1"); len(st.Parties) != 1 || st.Parties[0].ID != 0 || st.NextID != 1 {
		t.Fatalf("state %+v, want the one pin carried from pass to pass", st)
	}

	// A new run drops its Sent and gets its join again.
	if err := h.sup.Respawn(t.Context(), "hz2"); err != nil {
		t.Fatal(err)
	}
	<-r2.ctx.Done()
	r2.end <- nil
	r2 = h.next(t)
	mem = newFakeBot("Mem")
	h.handOver(t, "hz2", mem)
	if _, sent, newRun := h.party(t, "hz2"); sent.Answer != botparty.AnswerNone || sent.Order.Kind != 0 || !newRun {
		t.Fatalf("new run: sent %+v new run %v, want none and new", sent, newRun)
	}
	h.partyPass(t)
	wantHanded(t, "new run", mem, "join Lead")
	if _, sent, newRun := h.party(t, "hz2"); sent.Answer != botparty.AnswerPending || newRun {
		t.Fatalf("new run after the pass: sent %+v new run %v", sent, newRun)
	}
	h.stop(t, r1, r2)

	// A restart forgets the state; the pin's rows form it again.
	h = startSupervisor(t, reg, clock, 5, withParty)
	r1, r2 = h.restored(t)
	lead, mem = newFakeBot("Lead"), newFakeBot("Mem")
	h.handOver(t, "hz1", lead)
	h.handOver(t, "hz2", mem)
	h.partyPass(t)
	wantHanded(t, "after the restart", mem, "join Lead")
	mem.answer(nil, "join Lead")
	h.partyPass(t)
	wantHanded(t, "after the restart", lead, "lead Mem")
	lead.answer(nil, "lead Mem")
	h.partyPass(t)

	// The member leaves the pin: its rows are cleared, every bot's since
	// no member is left, and it does not form again.
	mem.set(func(s *botclient.Status) { s.Party, s.PartyLeft = "", botparty.LeftLeft })
	h.partyPass(t)
	wantHanded(t, "member left", lead, "lead Mem", "unparty")
	for _, a := range []string{"hz1", "hz2"} {
		if b := h.list(t)[a]; b.PartyGroup != "" || b.PartyLead {
			t.Fatalf("%s keeps its pin rows %q %v after the member left", a, b.PartyGroup, b.PartyLead)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("party_group")) || bytes.Contains(raw, []byte("party_lead")) {
		t.Fatalf("the saved registry keeps pin rows:\n%s", raw)
	}
	lead.answer(nil, "")
	for range 2 {
		h.partyPass(t)
	}
	wantHanded(t, "pin left", mem, "join Lead")
	wantHanded(t, "pin left", lead, "lead Mem", "unparty")
	if st, _, _ := h.party(t, "hz1"); len(st.Parties) != 0 {
		t.Fatalf("parties %+v after the pin dissolved, want none", st.Parties)
	}
	h.stop(t, r1, r2)
}

// TestSupervisorPartyOff checks that a supervisor with no organizer hands
// out no party order.
func TestSupervisorPartyOff(t *testing.T) {
	reg := openPanel(t, t.TempDir())
	for _, b := range []Bot{
		{Name: "Lead", Class: ptr(0), Preset: botparty.PresetFarmer, Online: true, PartyGroup: "Pin", PartyLead: true},
		{Name: "Mem", Class: ptr(10), Preset: botparty.PresetSupport, Online: true, PartyGroup: "Pin"},
	} {
		if _, err := reg.NewBot(b); err != nil {
			t.Fatal(err)
		}
	}
	clock := newClock()
	h := startSupervisor(t, reg, clock, 5, func(c *Config) { c.CheckPreset = nil })
	r1, r2 := h.restored(t)
	lead, mem := newFakeBot("Lead"), newFakeBot("Mem")
	h.handOver(t, "hz1", lead)
	h.handOver(t, "hz2", mem)
	clock.Advance(10 * PartyTick)
	h.list(t)
	wantHanded(t, "no organizer", lead)
	wantHanded(t, "no organizer", mem)
	if h.sup.partyTimer != nil {
		t.Fatal("a party timer without an organizer")
	}
	h.stop(t, r1, r2)
}

func TestSupervisorPartyPolicy(t *testing.T) {
	for _, size := range []int{1, botparty.MaxSize + 1} {
		p := botparty.DefaultPolicy()
		p.Size = size
		_, err := New(Config{Registry: &Registry{}, Run: func(ctx context.Context, r Run) error { return nil }, Gate: &botclient.LoginGate{}, Clock: newClock(), MaxOnline: 1, Party: &p})
		if err == nil {
			t.Errorf("party size %d accepted", size)
		}
	}
}
