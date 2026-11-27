package botmetrics

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// record counts n of everything a Bot records and sets its held gauge to
// 10n units.
func record(b *Bot, n int) {
	for range n {
		b.Drained(3)
		b.Sent()
		b.Paced()
		b.Rule("idle")
		b.Rule("store")
		b.WakeLag(2 * time.Millisecond)
		b.Reconnect()
		b.Abort()
		b.Panic()
		b.NavSegment()
		b.NavWalkIdle(5 * time.Millisecond)
		b.NavWalkEfficiency(800)
		b.NavPlan(PlanOK, 40*time.Microsecond)
		b.NavPlan(PlanDoor, time.Millisecond)
		b.NavBlocked()
		b.NavDesync()
		b.NavDoorReplan()
		b.ValuablesAd()
		b.ValuablesReply()
		b.ValuablesSold(ViaTrade)
		b.ValuablesSold(ViaStore)
		b.ValuablesDeposited()
		b.ValuablesReleased(2)
		b.DecodeErrors(0x4a, 0, 1)
		b.DecodeErrors(extendedOpcode, 0x12, 1)
		b.LoginDecodeError(0x03)
		b.PvPFight("duel")
		b.PvPDeath("pk")
		b.Duel("won")
		b.PvPInterrupt(InterruptCast)
	}
	b.ValuablesHeld(int64(10 * n))
}

// unrecorded are the Snapshot fields no Bot records.
var unrecorded = map[string]bool{
	"At": true, "BreakerOpen": true, "AcceptClosed": true, "NavLocates": true,
	"NavLRUBytes": true, "NavPlaces": true, "PartiesFormed": true, "PartiesBroken": true,
}

// TestMetricsRecordCoversSnapshot keeps record complete: a counter Collect
// sums that record leaves at zero would go untested by the removal tests.
func TestMetricsRecordCoversSnapshot(t *testing.T) {
	r := NewRegistry("idle")
	record(r.NewBot(), 1)
	v := reflect.ValueOf(*r.Collect(t0))
	for i := range v.NumField() {
		name, f := v.Type().Field(i).Name, v.Field(i)
		if unrecorded[name] {
			continue
		}
		var zero bool
		switch f.Kind() {
		case reflect.Map:
			zero = true
			for _, k := range f.MapKeys() {
				zero = zero && f.MapIndex(k).Uint() == 0
			}
		case reflect.Struct:
			zero = f.Interface().(HistogramSnapshot).Count == 0
		default:
			zero = f.IsZero()
		}
		if zero {
			t.Errorf("record leaves Snapshot.%s at zero", name)
		}
	}
}

func TestMetricsRemove(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the registry and the bot to remove.
		setup func() (*Registry, *Bot)
		// gone is the live slots and held units the removal takes away.
		gone, held int
	}{
		{"only bot", func() (*Registry, *Bot) {
			r := NewRegistry("idle")
			b := r.NewBot()
			record(b, 2)
			return r, b
		}, 1, 20},
		{"one of three", func() (*Registry, *Bot) {
			r := NewRegistry("idle")
			a, b, c := r.NewBot(), r.NewBot(), r.NewBot()
			record(a, 1)
			record(b, 3)
			record(c, 5)
			return r, b
		}, 1, 30},
		{"removed before", func() (*Registry, *Bot) {
			r := NewRegistry("idle")
			a, b := r.NewBot(), r.NewBot()
			record(a, 1)
			record(b, 4)
			r.Remove(b)
			return r, b
		}, 0, 0},
		{"nil", func() (*Registry, *Bot) {
			r := NewRegistry("idle")
			record(r.NewBot(), 2)
			return r, nil
		}, 0, 0},
		{"of another registry", func() (*Registry, *Bot) {
			r := NewRegistry("idle")
			record(r.NewBot(), 2)
			b := NewRegistry("idle").NewBot()
			record(b, 1)
			return r, b
		}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, b := tt.setup()
			before := *r.Collect(t0)
			r.Remove(b)
			after := *r.Collect(t0)
			if after.Bots != before.Bots-tt.gone || after.ValuablesHeld != before.ValuablesHeld-int64(tt.held) {
				t.Errorf("after the removal %d bots holding %d, want %d holding %d",
					after.Bots, after.ValuablesHeld, before.Bots-tt.gone, before.ValuablesHeld-int64(tt.held))
			}
			after.Bots, after.ValuablesHeld = before.Bots, before.ValuablesHeld
			if !reflect.DeepEqual(after, before) {
				t.Errorf("a sum changed at the removal:\nbefore %+v\nafter  %+v", before, after)
			}
		})
	}
}

func TestMetricsRemoveKeepsOthersLive(t *testing.T) {
	r := NewRegistry("idle")
	a, b := r.NewBot(), r.NewBot()
	record(a, 2)
	record(b, 3)
	r.Remove(a)
	before := r.Collect(t0)
	b.Sent()
	b.Rule("idle")
	b.PvPInterrupt(InterruptLanded)
	b.DecodeErrors(0x4a, 0, 2)
	a.Sent() // after its removal: lost, as Remove documents
	s := r.Collect(t0)
	if s.Sent != before.Sent+1 || s.Rules["idle"] != before.Rules["idle"]+1 ||
		s.PvPInterrupts[InterruptLanded] != 1 || s.DecodeErrors["0x4a"] != before.DecodeErrors["0x4a"]+2 {
		t.Errorf("the bot left is not live: sent %d->%d, idle %d->%d, landed %d, 0x4a %d->%d",
			before.Sent, s.Sent, before.Rules["idle"], s.Rules["idle"], s.PvPInterrupts[InterruptLanded],
			before.DecodeErrors["0x4a"], s.DecodeErrors["0x4a"])
	}
}

func TestMetricsRemoveEvery(t *testing.T) {
	const n = 50
	r := NewRegistry("idle")
	bots := make([]*Bot, n)
	for i := range bots {
		bots[i] = r.NewBot()
		record(bots[i], 1)
	}
	before := r.Collect(t0)
	for _, b := range bots {
		r.Remove(b)
	}
	live := r.NewBot()
	s := r.Collect(t0)
	r.mu.Lock()
	held := len(r.bots)
	r.mu.Unlock()
	if held != 1 || s.Bots != 1 || s.Sent != before.Sent || s.Events != before.Events || s.ValuablesHeld != 0 {
		t.Errorf("after %d removals and a new bot: %d held, %d bots, sent %d (want %d), events %d (want %d), held units %d",
			n, held, s.Bots, s.Sent, before.Sent, s.Events, before.Events, s.ValuablesHeld)
	}
	live.Sent()
	if got := r.Collect(t0).Sent; got != before.Sent+1 {
		t.Errorf("sent after the new bot's send = %d, want %d", got, before.Sent+1)
	}
}

// TestMetricsRemoveConcurrentCollect collects while bots are removed: PR CI
// runs it under -race, and no collection may see a sum fall.
func TestMetricsRemoveConcurrentCollect(t *testing.T) {
	const n = 40
	r := NewRegistry("idle")
	bots := make([]*Bot, n)
	for i := range bots {
		bots[i] = r.NewBot()
		record(bots[i], 1)
	}
	want := r.Collect(t0).Sent
	var wg sync.WaitGroup
	wg.Go(func() {
		for _, b := range bots {
			r.Remove(b)
		}
	})
	for range 2 {
		wg.Go(func() {
			for range n {
				if s := r.Collect(t0); s.Sent != want {
					t.Errorf("a collection during the removals summed %d sent, want %d", s.Sent, want)
					return
				}
			}
		})
	}
	wg.Wait()
	if s := r.Collect(t0); s.Bots != 0 || s.Sent != want {
		t.Errorf("after the removals %d bots and %d sent, want 0 and %d", s.Bots, s.Sent, want)
	}
}
