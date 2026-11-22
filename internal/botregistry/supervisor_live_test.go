package botregistry

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// liveBot builds the bot of r as a RunFunc does, without running it, and
// hands it over.
func liveBot(t *testing.T, h *harness, r *fakeRun) *botclient.Bot {
	t.Helper()
	b, err := botclient.NewBot(botclient.SessionConfig{Clock: h.clock, Gate: h.gate}, r.Account, "", botclient.BotConfig{Character: r.Character, Preset: r.Preset}, rand.New(rand.NewPCG(5, 6)))
	if err != nil {
		t.Fatal(err)
	}
	r.Live(b)
	return b
}

func TestSupervisorCommandsTheLiveBot(t *testing.T) {
	h := startSupervisor(t, openPanel(t, t.TempDir()), newClock(), 5)
	stay := botclient.Command{Kind: botclient.CommandStay}

	if _, err := h.sup.Command(t.Context(), "hz9", stay); !errors.Is(err, ErrNoBot) {
		t.Fatalf("command to an unknown bot = %v, want ErrNoBot", err)
	}
	b, r := h.create(t, "support", nil)
	if _, err := h.sup.Command(t.Context(), b.Account, stay); !errors.Is(err, ErrNoRun) {
		t.Fatalf("command before the run handed its bot over = %v, want ErrNoRun", err)
	}
	if s := h.list(t)[b.Account]; s.Live != nil {
		t.Fatalf("snapshot %+v before the run handed its bot over", s.Live)
	}
	liveBot(t, h, r)
	if s := h.list(t)[b.Account]; s.Live == nil || s.Live.State != botclient.StateNew || s.Live.Preset != "support" {
		t.Fatalf("live snapshot %+v, want a new support bot", s.Live)
	}
	if _, err := h.sup.Command(t.Context(), b.Account, botclient.Command{Kind: botclient.CommandGo}); !errors.Is(err, botclient.ErrBadCommand) {
		t.Fatalf("go without a spot = %v, want ErrBadCommand", err)
	}
	var answers []<-chan error
	for {
		a, err := h.sup.Command(t.Context(), b.Account, stay)
		if errors.Is(err, botclient.ErrBusy) {
			break
		}
		if err != nil {
			t.Fatalf("command %d = %v", len(answers)+1, err)
		}
		if len(answers) > 100 {
			t.Fatal("the mailbox never fills")
		}
		answers = append(answers, a)
	}
	if len(answers) == 0 {
		t.Fatal("first command answered busy")
	}

	if err := h.sup.Despawn(t.Context(), b.Account); err != nil {
		t.Fatal(err)
	}
	if _, err := h.sup.Command(t.Context(), b.Account, stay); !errors.Is(err, ErrNoRun) {
		t.Fatalf("command to a bot logging out = %v, want ErrNoRun", err)
	}
	<-r.ctx.Done()
	r.end <- nil
	s := h.settle(t, b.Account, RunStopping)
	if s.Live != nil {
		t.Fatalf("snapshot %+v kept after the run ended", s.Live)
	}
	if _, err := h.sup.Command(t.Context(), b.Account, stay); !errors.Is(err, ErrNoRun) {
		t.Fatalf("command to a despawned bot = %v, want ErrNoRun", err)
	}
	h.stop(t)
}

func TestKeepProgress(t *testing.T) {
	old := &Position{X: 1, Y: 2, Z: 3}
	tests := []struct {
		name      string
		st        botclient.Status
		wantLevel int32
		wantClass int32
		wantPos   *Position
	}{
		{"never entered keeps the stored ones", botclient.Status{State: botclient.StateStopped, ClassID: 15}, 7, 10, old},
		{"entered copies level, class and position", botclient.Status{Level: 12, ClassID: 10, Position: botclient.Point{X: -5, Y: 6, Z: -7}}, 12, 10, &Position{X: -5, Y: 6, Z: -7}},
		{"a class changed at the master is kept", botclient.Status{Level: 20, ClassID: 15, Position: botclient.Point{X: 1}}, 20, 15, &Position{X: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := int32(10)
			b := Bot{Level: 7, Class: &class, Position: old}
			keepProgress(&b, tt.st)
			if b.Level != tt.wantLevel || *b.Class != tt.wantClass || *b.Position != *tt.wantPos {
				t.Fatalf("level %d class %d position %+v, want %d %d %+v", b.Level, *b.Class, *b.Position, tt.wantLevel, tt.wantClass, *tt.wantPos)
			}
		})
	}
}
