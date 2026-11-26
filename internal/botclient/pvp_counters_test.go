package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botmetrics"
)

// metered gives b a metrics slot of a fresh registry and returns the
// registry.
func metered(b *Brain) *botmetrics.Registry {
	reg := botmetrics.NewRegistry()
	b.metrics = reg.NewBot()
	return reg
}

// sendOK sends a at now and fails unless it went out.
func sendOK(t *testing.T, b *Brain, a Action, now time.Time) {
	t.Helper()
	if sent, err := b.send(a, now); err != nil || !sent {
		t.Fatalf("send %v at %v: sent %v, err %v", a.kind, now, sent, err)
	}
}

// only is a label set's counts with want's labels as given and every other
// label 0.
func only(set map[string]uint64, want map[string]uint64) bool {
	for k, n := range set {
		if n != want[k] {
			return false
		}
	}
	for k := range want {
		if _, ok := set[k]; !ok {
			return false
		}
	}
	return true
}

// TestPvPCounters checks the four PvP counters (docs/pvp/modes.md "Panel
// and metrics (X12)"): a first attack on a duel, fight-back, arena, flagged
// and PK-mode opponent counts once under its kind, a second attack not; a
// death to each but the duel opponent, and to an attacker MayFight refused,
// counts under that kind or unfought; 1950 with its own name, the
// opponent's, 1952 count won, lost, tie; an interrupt then the opponent's
// MagicSkillCanceled counts cast, landed; an interrupt and the landing time
// passed cast, too_late.
func TestPvPCounters(t *testing.T) {
	// setups make pcID an opponent of one kind at t0 and report when the
	// bot may attack it.
	setups := []struct {
		kind  string
		arena bool
		setup func(t *testing.T, b *Brain) time.Time
	}{
		{"duel", false, func(t *testing.T, b *Brain) time.Time {
			duelUntil(t, b, DuelFighting, t0)
			return at(10 * time.Second)
		}},
		{"fight_back", false, func(t *testing.T, b *Brain) time.Time {
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
			b.handleOne(t, hitSelf(pcID, 10), t0)
			return at(time.Second)
		}},
		{"arena", true, func(t *testing.T, b *Brain) time.Time {
			b.handleOne(t, hitSelf(pcID, 10), t0)
			return at(time.Second)
		}},
		{"flagged", false, func(t *testing.T, b *Brain) time.Time {
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
			return at(time.Second)
		}},
		{"pk", false, func(t *testing.T, b *Brain) time.Time {
			b.persona.PvP.ForceAttack = true
			b.pkStart(pcID, t0)
			return at(time.Second)
		}},
	}

	t.Run("fights", func(t *testing.T) {
		for _, tc := range setups {
			t.Run(tc.kind, func(t *testing.T) {
				b := pvpBrain(t, tc.arena, nil)
				reg := metered(b)
				now := tc.setup(t, b)
				sendOK(t, b, attackAction(pcID, Point{}), now)
				sendOK(t, b, attackAction(pcID, Point{}), now.Add(time.Second))
				if got := reg.Collect(now).PvPFights; !only(got, map[string]uint64{tc.kind: 1}) {
					t.Fatalf("fights %v, want one %s", got, tc.kind)
				}
			})
		}
	})

	t.Run("deaths", func(t *testing.T) {
		cases := append(setups[:0:0], setups...)
		cases = append(cases, struct {
			kind  string
			arena bool
			setup func(t *testing.T, b *Brain) time.Time
		}{"unfought", false, func(t *testing.T, b *Brain) time.Time {
			b.handleOne(t, hitSelf(pcID, 10), t0)
			return at(time.Second)
		}})
		for _, tc := range cases {
			t.Run(tc.kind, func(t *testing.T) {
				b := pvpBrain(t, tc.arena, nil)
				b.spot.held = false // no spot registry to give it back to
				reg := metered(b)
				now := tc.setup(t, b)
				if tc.kind != "unfought" {
					sendOK(t, b, attackAction(pcID, Point{}), now)
				} else if b.mayFight(pcID, now) || b.pvp.opponents[pcID].Kind != FightUnfought {
					t.Fatal("a clean attacker is fought")
				}
				// The opponent fights back: rule 6 now calls a flagged
				// one or a PK victim (flagged by its hit) fight back.
				b.handleOne(t, hitSelf(pcID, 50), now.Add(time.Second))
				b.mayFight(pcID, now.Add(time.Second))
				b.handleOne(t, Die{ObjectID: selfID}, now.Add(2*time.Second))
				want := map[string]uint64{tc.kind: 1}
				if tc.kind == "duel" {
					want = nil // a duel defeat counts in bot_duels_total
				}
				if got := reg.Collect(now).PvPDeaths; !only(got, want) {
					t.Fatalf("deaths %v, want %v", got, want)
				}
			})
		}
	})

	t.Run("duels", func(t *testing.T) {
		won := SystemMessage{ID: msgS1WonTheDuel, Text: [2]string{"bot7"}}
		lost := SystemMessage{ID: msgS1WonTheDuel, Text: [2]string{foeName}}
		withdrew := SystemMessage{ID: msgS1WithdrewS2Won, Text: [2]string{foeName, "bot7"}}
		tie := SystemMessage{ID: msgDuelEndedInTie}
		for _, tc := range []struct {
			name string
			evs  []Event
			want string
		}{
			{"won", []Event{won}, "won"},
			{"lost", []Event{lost}, "lost"},
			{"withdrawal then won", []Event{withdrew, won}, "won"},
			{"tie", []Event{tie}, "tie"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				b := pvpBrain(t, false, nil)
				reg := metered(b)
				duelUntil(t, b, DuelFighting, t0)
				for _, ev := range tc.evs {
					b.handleOne(t, ev, at(30*time.Second))
				}
				b.handleOne(t, ExDuelEnd{}, at(30*time.Second))
				if got := reg.Collect(t0).Duels; !only(got, map[string]uint64{tc.want: 1}) {
					t.Fatalf("duels %v, want one %s", got, tc.want)
				}
			})
		}
	})

	t.Run("interrupts", func(t *testing.T) {
		stun := Step{Skill: lyrStun, Hold: true}
		heal := MagicSkillUse{CasterID: pcID, TargetID: pcID, SkillID: skillHeal, Level: 1, HitTime: 5 * time.Second}
		for _, tc := range []struct {
			name string
			then func(t *testing.T, b *Brain)
			want map[string]uint64
		}{
			{"canceled before the landing", func(t *testing.T, b *Brain) {
				b.handleOne(t, MagicSkillCanceled{CasterID: pcID}, at(4*time.Second))
			}, map[string]uint64{botmetrics.InterruptCast: 1, botmetrics.InterruptLanded: 1}},
			{"the landing time passed", func(t *testing.T, b *Brain) {
				if err := b.wake(at(7 * time.Second)); err != nil {
					t.Fatal(err)
				}
			}, map[string]uint64{botmetrics.InterruptCast: 1, botmetrics.InterruptTooLate: 1}},
			{"canceled after the landing", func(t *testing.T, b *Brain) {
				b.handleOne(t, MagicSkillCanceled{CasterID: pcID}, at(7*time.Second))
				b.handleOne(t, MagicSkillCanceled{CasterID: pcID}, at(8*time.Second))
			}, map[string]uint64{botmetrics.InterruptCast: 1, botmetrics.InterruptTooLate: 1}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				b := layerBrain(t, lyrCleric, stun)
				reg := metered(b)
				b.handleOne(t, heal, at(2*time.Second)) // lands at 7 s
				_, a := b.decide(at(2600 * time.Millisecond))
				if a.kind != actCast || a.skill != lyrStun || !a.interrupts {
					t.Fatalf("decision %+v, want the stun interrupt", a)
				}
				sendOK(t, b, a, at(2600*time.Millisecond))
				tc.then(t, b)
				if got := reg.Collect(t0).PvPInterrupts; !only(got, tc.want) {
					t.Fatalf("interrupts %v, want %v", got, tc.want)
				}
			})
		}
	})
}

// TestPvPStatus checks the panel's PvP view: the persona's modes, the kill
// counts of the latest UserInfo, the run's duel tally and the last result.
func TestPvPStatus(t *testing.T) {
	b := pvpBrain(t, false, func(p *Persona) { p.PvP.Duels, p.PvP.ForceAttack = true, true })
	b.cfg.link = &botLink{}
	b.handleOne(t, UserInfo{ObjectID: selfID, Name: "bot7", PKKills: 2, PvPKills: 5}, t0)
	duelUntil(t, b, DuelFighting, t0)
	b.handleOne(t, SystemMessage{ID: msgS1WonTheDuel, Text: [2]string{"bot7"}}, at(30*time.Second))
	b.handleOne(t, ExDuelEnd{}, at(30*time.Second))
	duelUntil(t, b, DuelFighting, at(time.Minute))
	b.handleOne(t, SystemMessage{ID: msgDuelEndedInTie}, at(90*time.Second))
	got := b.pvpStatus()
	want := PvPStatus{Modes: b.persona.PvP, PvPKills: 5, PKKills: 2, Duels: DuelTally{Won: 1, Tie: 1}, LastDuel: DuelTie}
	if *got != want {
		t.Fatalf("status %+v, want %+v", *got, want)
	}
	if !got.Modes.Duels || !got.Modes.ForceAttack {
		t.Errorf("modes %+v, want duels and PK on", got.Modes)
	}
}
