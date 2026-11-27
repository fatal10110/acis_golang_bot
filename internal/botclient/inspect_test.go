package botclient

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// inspectMail is an inspect with deadline at.
func inspectMail(at time.Time) mail {
	return mail{inspect: make(chan Inspection, 1), deadline: at}
}

// answered is the Inspection waiting on m's reply, false for none.
func answered(m mail) (Inspection, bool) {
	select {
	case in, ok := <-m.inspect:
		return in, ok
	default:
		return Inspection{}, false
	}
}

// TestBrainInspect checks the brain's answer to an inspect: one
// RequestItemList, then the model once the ItemList answering it is applied,
// or the model as it stands, stale, at the deadline.
func TestBrainInspect(t *testing.T) {
	sword := Item{ObjectID: 30, TemplateID: 2, Count: 1, Equipped: true, BodySlot: partRHand, Enchant: 3}
	potion := Item{ObjectID: 20, TemplateID: 1060, Count: 14}
	listed := Item{ObjectID: 10, TemplateID: 57, Count: 900}
	deadline := t0.Add(3 * time.Second)
	tests := []struct {
		name string
		// run drives the brain with the inspect mails ms; it returns when
		// the answer is due.
		run       func(t *testing.T, b *Brain, ms []mail)
		mails     int
		at        []time.Time
		wantSent  int
		wantStale bool
		wantItems []Item
	}{
		{
			name:  "the ItemList answers it",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, ms []mail) {
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				if at, ok := b.inspect.due(); !ok || !at.Equal(deadline) {
					t.Fatalf("due %v %v, want the deadline once asked", at, ok)
				}
				if _, ok := answered(ms[0]); ok {
					t.Fatal("answered before the ItemList")
				}
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, t0.Add(80*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
			},
			wantSent: 1, wantItems: []Item{listed},
		},
		{
			name:  "no ItemList: stale at the deadline",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, ms []mail) {
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				if err := b.wake(deadline.Add(-time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				if _, ok := answered(ms[0]); ok {
					t.Fatal("answered before the deadline")
				}
				if got := b.due(); got.After(deadline) {
					t.Fatalf("due %v, past the deadline %v", got, deadline)
				}
				if err := b.wake(deadline); err != nil {
					t.Fatal(err)
				}
			},
			wantSent: 1, wantStale: true, wantItems: []Item{potion, sword},
		},
		{
			name:  "a deadline past in the mailbox: stale at once, nothing sent",
			mails: 1, at: []time.Time{t0},
			run:       func(*testing.T, *Brain, []mail) {},
			wantStale: true, wantItems: []Item{potion, sword},
		},
		{
			name:  "an ItemList before the ask answers with none sent",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, t0); err != nil {
					t.Fatal(err)
				}
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
			},
			wantItems: []Item{listed},
		},
		{
			name:  "the frame bucket holds it: it goes out on the next wake",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.pace.tokens = 0
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				retry := t0.Add(time.Second / frameRate)
				if got := b.due(); !got.Equal(retry) {
					t.Fatalf("due %v, want the retry %v", got, retry)
				}
				if err := b.wake(retry); err != nil {
					t.Fatal(err)
				}
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, retry); err != nil {
					t.Fatal(err)
				}
			},
			wantSent: 1, wantItems: []Item{listed},
		},
		{
			name:  "two inspects share one RequestItemList",
			mails: 2, at: []time.Time{deadline, deadline.Add(time.Second)},
			run: func(t *testing.T, b *Brain, _ []mail) {
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, t0); err != nil {
					t.Fatal(err)
				}
			},
			wantSent: 1, wantItems: []Item{listed},
		},
		{
			name:  "Appearing still due holds it: stale at the deadline, nothing sent",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.appearing = true
				heldUntilDeadline(t, b, deadline)
			},
			wantStale: true, wantItems: []Item{potion, sword},
		},
		{
			name:  "held until Appearing is out, then it goes out",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.appearing = true
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				b.appearing = false
				retry := t0.Add(time.Second / frameRate)
				if err := b.wake(retry); err != nil {
					t.Fatal(err)
				}
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, retry); err != nil {
					t.Fatal(err)
				}
			},
			wantSent: 1, wantItems: []Item{listed},
		},
		{
			name:  "a Logout awaiting its answer holds it",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.logout.requested, b.logout.pending, b.logout.deadline = true, true, deadline.Add(time.Minute)
				heldUntilDeadline(t, b, deadline)
			},
			wantStale: true, wantItems: []Item{potion, sword},
		},
		{
			name:  "a market visit holds it",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.market.v.obj = 0x10000001
				heldUntilDeadline(t, b, deadline)
			},
			wantStale: true, wantItems: []Item{potion, sword},
		},
		{
			name:  "a shop purchase holds it and its ItemList answers it",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				b.shop.v.npcID, b.shop.step = 30001, shopBuying
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				if err := b.handle([]Event{ItemList{Items: []Item{listed}}}, t0.Add(80*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				if b.shop.step != shopIdle {
					t.Errorf("shop step %d, want the purchase answered", b.shop.step)
				}
			},
			wantItems: []Item{listed},
		},
		{
			name:  "the session ends first: stale",
			mails: 1, at: []time.Time{deadline},
			run: func(t *testing.T, b *Brain, _ []mail) {
				if err := b.wake(t0); err != nil {
					t.Fatal(err)
				}
				b.answerInspect(t0.Add(time.Second), true)
			},
			wantSent: 1, wantStale: true, wantItems: []Item{potion, sword},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, out := newTestBrain(t, Persona{})
			b.cfg.Tick = time.Hour
			b.nextTick = t0.Add(time.Hour)
			w := b.world
			w.Apply(ItemList{Items: []Item{sword, potion}}, t0)
			w.Apply(AbnormalStatusUpdate{Effects: []Effect{
				{SkillID: 1204, Level: 2, Left: 20 * time.Minute},
				{SkillID: 1068, Level: 1, Permanent: true},
			}}, t0)
			w.Apply(SkillList{Skills: []KnownSkill{
				{ID: expertiseSkill, Level: 1, Passive: true},
				{ID: gradePenaltySkill, Level: 1, Passive: true},
			}}, t0)
			s := &w.Self
			s.ClassID, s.Load, s.MaxLoad, s.DeathPenalty, s.RunSpeed = 1, 7000, 10000, 2, 63
			out.frames = nil
			ms := make([]mail, tc.mails)
			for i := range ms {
				ms[i] = inspectMail(tc.at[i])
				b.command(ms[i], t0)
			}
			tc.run(t, b, ms)
			sent := 0
			for _, f := range out.frames {
				if f[0] == opGameRequestItemList {
					sent++
				}
			}
			if sent != tc.wantSent {
				t.Errorf("sent %d RequestItemList, want %d", sent, tc.wantSent)
			}
			if _, ok := b.inspect.due(); ok {
				t.Error("an inspect still waits")
			}
			for i, m := range ms {
				in, ok := answered(m)
				if !ok {
					t.Fatalf("inspect %d not answered", i)
				}
				if in.Stale != tc.wantStale || !reflect.DeepEqual(in.Items, tc.wantItems) {
					t.Errorf("inspect %d: stale %v, items %+v; want %v, %+v", i, in.Stale, in.Items, tc.wantStale, tc.wantItems)
				}
				left := 20*time.Minute - in.At.Sub(t0)
				wantFx := []Effect{{SkillID: 1068, Level: 1, Permanent: true}, {SkillID: 1204, Level: 2, Left: left}}
				if !reflect.DeepEqual(in.Effects, wantFx) {
					t.Errorf("inspect %d: effects %+v, want %+v", i, in.Effects, wantFx)
				}
				want := Inspection{ClassID: 1, Load: 7000, MaxLoad: 10000, WeightPenalty: 2, GradePenalty: true, Expertise: 1, DeathPenalty: 2, RunSpeed: 63}
				got := in
				got.At, got.Stale, got.Items, got.Effects = time.Time{}, false, nil, nil
				if !reflect.DeepEqual(got, want) {
					t.Errorf("inspect %d: %+v, want %+v", i, got, want)
				}
			}
		})
	}
}

// heldUntilDeadline wakes b while its inspect is held: the request waits,
// retried at the frame rate, and the deadline answers it.
func heldUntilDeadline(t *testing.T, b *Brain, deadline time.Time) {
	t.Helper()
	if err := b.wake(t0); err != nil {
		t.Fatal(err)
	}
	retry := t0.Add(time.Second / frameRate)
	if at, ok := b.inspect.due(); !ok || !at.Equal(retry) {
		t.Fatalf("due %v %v, want the retry %v", at, ok, retry)
	}
	if err := b.wake(retry); err != nil {
		t.Fatal(err)
	}
	if err := b.wake(deadline); err != nil {
		t.Fatal(err)
	}
}

// TestBotInspect checks what Bot.Inspect answers at once, and that a bot
// stopping closes the reply of an inspect left in its mailbox.
func TestBotInspect(t *testing.T) {
	b, _, _ := startBot(t, true, &fakeNet{})
	if _, err := b.Inspect(time.Second); !errors.Is(err, ErrOffline) {
		t.Fatalf("inspect between sessions = %v, want ErrOffline", err)
	}
	b.setInSession(true)
	first, err := b.Inspect(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for range mailboxSize - 1 {
		if _, err := b.Inspect(time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Inspect(time.Second); !errors.Is(err, ErrBusy) {
		t.Fatalf("inspect past the mailbox = %v, want ErrBusy", err)
	}
	b.stop()
	if in, ok := <-first; ok {
		t.Fatalf("stopped bot answered %+v, want the reply closed", in)
	}
	if _, err := b.Inspect(time.Second); !errors.Is(err, ErrStopped) {
		t.Fatalf("inspect after the stop = %v, want ErrStopped", err)
	}
}

// TestRequestItemListGate checks that RequestItemList passes no gate of its
// own: the frame bucket bounds it.
func TestRequestItemListGate(t *testing.T) {
	if g := (Action{kind: actItemList}).gate(); g != gateNone {
		t.Fatalf("RequestItemList gate %v, want none: the frame bucket bounds it", g)
	}
}
