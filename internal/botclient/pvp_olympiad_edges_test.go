package botclient

import (
	"testing"
	"time"
)

// registeredSteps take olyBrain to the registration, registered at 10s,
// and back to the manager past its Wait, on noble_main from 31m01s on.
func registeredSteps() []trainerStep {
	return []trainerStep{
		{name: "registered: it waits", at: 10 * time.Second, events: []Event{SystemMessage{ID: msgRegisteredClassed}}, wantRule: "olympiad wait"},
		{name: "30 minutes: back to the manager", at: 31 * time.Minute, setup: managerInSight, wantRule: "olympiad entry", wantKind: actTalk},
	}
}

// TestOlympiadManagerEdges checks the manager visit's answers off the main
// path (docs/pvp/olympiad.md "Registering", "While registered"): a page
// without the link it wants and a refused unregistration.
func TestOlympiadManagerEdges(t *testing.T) {
	t.Run("a manager page without the registration link stops it for the day", func(t *testing.T) {
		b := olyBrain(t)
		runTrainerSteps(t, b, registerSteps(time.Second)[:2])
		b.handleOne(t, olyPage("Chat 1"), at(3*time.Second))
		if rule, _ := b.decide(at(4 * time.Second)); rule == "olympiad entry" || rule == "olympiad wait" {
			t.Fatalf("decide = %q on a page without the link, want the persona's own rules", rule)
		}
		if !b.oly.stopped || b.oly.v.active() || b.oly.tries != 0 {
			t.Fatalf("stopped %v visit %v tries %d, want stopped, the visit over and no try spent",
				b.oly.stopped, b.oly.v.active(), b.oly.tries)
		}
		managerInSight(b)
		if rule, _ := b.decide(at(2 * time.Hour)); rule == "olympiad entry" {
			t.Fatal("a manager visit later that day, want none")
		}
	})

	t.Run("registered, noble_main without the unregistration link: off the list, not stopped", func(t *testing.T) {
		b := olyBrain(t)
		runTrainerSteps(t, b, registerSteps(time.Second))
		runTrainerSteps(t, b, registeredSteps())
		b.handleOne(t, olyPage("Chat 10", "Chat 1"), at(31*time.Minute+time.Second))
		if rule, _ := b.decide(at(31*time.Minute + 2*time.Second)); rule == "olympiad entry" || rule == "olympiad wait" {
			t.Fatalf("decide = %q on a page without the link, want the persona's own rules", rule)
		}
		if !b.oly.registered.IsZero() || b.oly.stopped || b.oly.v.active() {
			t.Fatalf("registered %v stopped %v visit %v, want off the list, not stopped, the visit over",
				b.oly.registered, b.oly.stopped, b.oly.v.active())
		}
	})

	for _, tc := range []struct {
		name string
		id   int32
	}{
		{name: "1651 not running", id: msgOlympiadNotRunning},
		{name: "1674 noblesse only", id: msgOlympiadNoblesse},
	} {
		t.Run("unregistration refused by "+tc.name+": off the list, a Wait of farming", func(t *testing.T) {
			b := olyBrain(t)
			runTrainerSteps(t, b, registerSteps(time.Second))
			runTrainerSteps(t, b, registeredSteps())
			runTrainerSteps(t, b, []trainerStep{
				{name: "unregister", at: 31*time.Minute + time.Second, events: []Event{nobleMain}, wantRule: "olympiad entry", wantKind: actBypass, wantPayload: managerBypass(olympiadUnregister)},
			})
			answered := at(31*time.Minute + 2*time.Second)
			b.handleOne(t, SystemMessage{ID: tc.id}, answered)
			if !b.oly.registered.IsZero() || b.oly.stopped || b.oly.v.active() {
				t.Fatalf("registered %v stopped %v visit %v, want off the list, not stopped, the visit over",
					b.oly.registered, b.oly.stopped, b.oly.v.active())
			}
			if want := answered.Add(b.persona.Olympiad.Wait); !b.oly.retryAt.Equal(want) {
				t.Fatalf("retryAt %v, want %v (a Wait after the answer)", b.oly.retryAt, want)
			}
		})
	}
}
