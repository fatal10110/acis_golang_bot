package botregistry

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botparty"
)

// TestPartyPassSameOrders runs the Organizer's pass and the supervisor's
// pass (#656: one driver, planPass) over the same bots through the same
// steps: from the same candidates both hand every bot the same orders.
func TestPartyPassSameOrders(t *testing.T) {
	farmer, support := botparty.PresetFarmer, botparty.PresetSupport
	tests := []struct {
		name    string
		presets []string
		// steps edit the fakes before each pass; a nil step edits nothing.
		steps []func(f []*fakeBot)
		// handed is how many commands the passes hand in all.
		handed int
	}{
		{
			name:    "a party through its answers",
			presets: []string{farmer, support},
			steps: []func(f []*fakeBot){
				nil,
				nil,
				func(f []*fakeBot) { f[1].answer(nil, "join Bot1"); f[0].setBusy(true) },
				func(f []*fakeBot) { f[0].setBusy(false) },
				func(f []*fakeBot) { f[0].answer(botclient.ErrStopped, "") },
				func(f []*fakeBot) { f[0].answer(nil, "lead Bot2") },
			},
			handed: 4,
		},
		{
			name:    "at most FormPerPass parties a pass",
			presets: []string{farmer, farmer, farmer, support, support, support},
			steps:   []func(f []*fakeBot){nil, nil},
			handed:  3,
		},
		{
			name:    "an offline bot takes no part",
			presets: []string{farmer, support, support},
			steps: []func(f []*fakeBot){
				func(f []*fakeBot) { f[1].set(func(s *botclient.Status) { s.State = botclient.StateReconnecting }) },
			},
			handed: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newClock()
			o, org := organizerOf(t, clock, tt.presets...)
			var recs []Bot
			for i, p := range tt.presets {
				name := fmt.Sprintf("Bot%d", i+1)
				if p == farmer {
					recs = append(recs, farmerBot(name))
				} else {
					recs = append(recs, supportBot(name))
				}
			}
			h, _, fakes := partyRig(t, recs...)
			sup := make([]*fakeBot, len(org))
			for i := range org {
				st := org[i].Status()
				sup[i] = fakes[fmt.Sprintf("hz%d", i+1)]
				sup[i].set(func(s *botclient.Status) { *s = st })
			}
			for n, step := range tt.steps {
				if step != nil {
					step(org)
					step(sup)
				}
				clock.Advance(PartyTick)
				o.pass()
				h.partyPass(t)
				for i := range org {
					if got, want := sup[i].handed(), org[i].handed(); !slices.Equal(got, want) {
						t.Fatalf("pass %d, bot %d: the supervisor handed %q, the organizer %q", n+1, i+1, got, want)
					}
				}
			}
			total := 0
			for _, f := range org {
				total += len(f.handed())
			}
			if total != tt.handed {
				t.Errorf("handed %d commands in all, want %d", total, tt.handed)
			}
		})
	}
}
