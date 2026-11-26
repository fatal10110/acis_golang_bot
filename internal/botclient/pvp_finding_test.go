package botclient

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
	"github.com/fatal10110/acis_golang_bot/internal/nav"
)

// The fight finding's fixtures: a second player, a bot-marked name, and
// class templates with vitals the engage estimate can price.
const (
	findOther  = int32(0x20000005)
	otherName  = "other"
	markedName = "FoeBot"
)

var (
	findFighter = withVitals(humanFighterTmpl)
	findMystic  = withVitals(humanMysticTmpl)
)

// withVitals is c with HP and CP tables at every level up to 80.
func withVitals(c datapack.Class) datapack.Class {
	c.HPTable, c.MPTable, c.CPTable = make([]float64, 80), make([]float64, 80), make([]float64, 80)
	for i := range 80 {
		c.HPTable[i], c.MPTable[i], c.CPTable[i] = 100, 50, 50
	}
	return c
}

// findBrain is pvpBrain with the Duels mode on, edited by edit, the bot a
// Human Fighter at level 20 whose reported stats beat a naked template,
// and the travel map's class templates of the Human Fighter and Mystic.
func findBrain(t *testing.T, edit func(*Persona)) *Brain {
	t.Helper()
	b := pvpBrain(t, false, func(p *Persona) {
		p.PvP.Duels = true
		if edit != nil {
			edit(p)
		}
	})
	b.cfg.Travel.classes = map[int32]datapack.Class{0: findFighter, 10: findMystic}
	s := &b.world.Self
	s.ClassID, s.Level = 0, 20
	s.PAtk, s.PDef, s.MAtk, s.MDef, s.PAtkSpd, s.CastSpd, s.Accuracy, s.Evasion, s.Critical = 300, 300, 100, 300, 300, 333, 40, 40, 40
	s.MaxCP, s.CP = 100, 100
	b.spot.held = true // on the road to its spot: own challenges allowed
	return b
}

// ask is foeName's challenge at d, and the time its answer is due.
func ask(t *testing.T, b *Brain, d time.Duration) time.Time {
	t.Helper()
	b.handleOne(t, ExDuelAskStart{Requester: foeName}, at(d))
	return b.pvp.asked.due
}

// duelAnswer is the duel answer the bot decides at now: whether it answered,
// and yes.
func duelAnswer(b *Brain, now time.Time) (answered, yes bool) {
	rule, a := b.decide(now)
	if rule != "duel" || a.kind != actDuelAnswer {
		return false, false
	}
	return true, string(a.payload) == string(encodeRequestDuelAnswerStart(false, true))
}

// challenged reports the player the bot challenges at now, "" for none.
func challenged(t *testing.T, b *Brain, now time.Time) string {
	t.Helper()
	rule, a := b.decide(now)
	if a.kind != actDuelStart {
		return ""
	}
	if rule != "challenge" {
		t.Fatalf("challenge by rule %q", rule)
	}
	if _, err := b.send(a, now); err != nil {
		t.Fatal(err)
	}
	return b.pvp.duel.Name
}

// TestPvPDuelAccept checks the duel answer (docs/pvp/finding.md "Sources",
// "Where a duel is allowed", "Clear of monsters").
func TestPvPDuelAccept(t *testing.T) {
	around := box(-500, -500, 500, 500)
	for _, tc := range []struct {
		name  string
		edit  func(*Persona)
		setup func(*Brain)
		party bool
		yes   bool
	}{
		{name: "full HP and MP out of combat", yes: true},
		{name: "MP under half", setup: func(b *Brain) { b.world.Self.MP = 40 }},
		{name: "a farmer", edit: func(p *Persona) { p.PvP.Duels = false }},
		{name: "a party duel", party: true},
		{name: "in a town", setup: func(b *Brain) { b.cfg.Travel.zones.Towns = []datapack.Zone{around} }},
		{name: "in an arena", setup: func(b *Brain) { b.cfg.Travel.zones.Arenas = []datapack.Zone{around} }},
		{name: "in a no-restart zone", setup: func(b *Brain) { b.cfg.Travel.zones.NoRestart = []datapack.Zone{around} }},
		{name: "in water", setup: func(b *Brain) {
			b.cfg.Nav = &Nav{Water: nav.NewWater([]nav.WaterBox{{X1: -500, Y1: -500, X2: 500, Y2: 500, MinZ: -1000, MaxZ: 1000}})}
		}},
		{name: "an aggressive monster within its aggro range plus 300", setup: func(b *Brain) {
			b.cfg.Travel.aggro = map[int32]int{20001: 750}
		}},
		{name: "an aggressive monster beyond it", yes: true, setup: func(b *Brain) {
			b.cfg.Travel.aggro = map[int32]int{20001: 650}
		}},
		{name: "a monster attacking it", setup: func(b *Brain) { b.handleOne(t, hitSelf(npcID, 5), t0) }},
		{name: "a clan mate", yes: true, setup: func(b *Brain) {
			b.world.Clan.ID = 7
			b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, ClanID: 7}, t0)
		}},
		{name: "a party member", yes: true, setup: func(b *Brain) {
			b.world.Party.Members = map[int32]*PartyMate{pcID: {PartyMember: PartyMember{Name: foeName}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := findBrain(t, tc.edit)
			if tc.setup != nil {
				tc.setup(b)
			}
			b.handleOne(t, ExDuelAskStart{Requester: foeName, Party: tc.party}, at(time.Second))
			due := b.pvp.asked.due
			answered, _ := duelAnswer(b, at(time.Second))
			if tc.yes {
				if answered {
					t.Fatal("answered before the PvP reaction")
				}
				if answered, yes := duelAnswer(b, due); !answered || !yes {
					t.Fatalf("answered %v yes %v at the reaction, want the acceptance", answered, yes)
				}
				return
			}
			if !answered {
				t.Fatal("no answer at once, want the decline")
			}
			if tc.party || tc.edit != nil {
				return
			}
			if _, a := b.decide(at(time.Second)); a.kind == actDuelAnswer && string(a.payload) != string(encodeRequestDuelAnswerStart(false, false)) {
				t.Fatalf("payload % x, want the decline", a.payload)
			}
		})
	}
	t.Run("no own challenge where it declines", func(t *testing.T) {
		b := findBrain(t, nil)
		b.handleOne(t, hitSelf(npcID, 5), t0)
		b.world.Self.InCombat = false
		if got := challenged(t, b, at(time.Second)); got != "" {
			t.Fatalf("challenged %q with a monster attacking", got)
		}
	})
	t.Run("a bot-marked challenger", func(t *testing.T) {
		b := findBrain(t, nil)
		b.handleOne(t, ExDuelAskStart{Requester: markedName}, at(time.Second))
		if answered, yes := duelAnswer(b, at(time.Second)); !answered || yes {
			t.Fatalf("answered %v yes %v, want the decline at once", answered, yes)
		}
	})
	t.Run("an unexplained 1940 holds", func(t *testing.T) {
		b := findBrain(t, nil)
		b.handleOne(t, SystemMessage{ID: msgUnableToRequestDuel}, at(time.Second))
		due := ask(t, b, 2*time.Second)
		if answered, yes := duelAnswer(b, due); !answered || yes {
			t.Fatalf("answered %v yes %v after an unexplained 1940, want the decline", answered, yes)
		}
		if got := challenged(t, b, due.Add(time.Second)); got != "" {
			t.Fatalf("challenged %q under the 1940 hold", got)
		}
		due = ask(t, b, 10*time.Minute+2*time.Second)
		if answered, yes := duelAnswer(b, due); !answered || !yes {
			t.Fatalf("answered %v yes %v 10 minutes on, want the acceptance", answered, yes)
		}
	})
	t.Run("an unexplained 1940 ends 2000 away", func(t *testing.T) {
		b := findBrain(t, nil)
		b.handleOne(t, SystemMessage{ID: msgUnableToRequestDuel}, at(time.Second))
		place(t, b, selfID, Point{X: 2100})
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{X: 2100, Y: 300}}, at(time.Minute))
		due := ask(t, b, time.Minute)
		if answered, yes := duelAnswer(b, due); !answered || !yes {
			t.Fatalf("answered %v yes %v 2100 away, want the acceptance", answered, yes)
		}
	})
	t.Run("a 1940 in combat stance holds nothing", func(t *testing.T) {
		b := findBrain(t, nil)
		b.handleOne(t, AutoAttackStart{ObjectID: selfID}, at(time.Second))
		b.handleOne(t, SystemMessage{ID: msgUnableToRequestDuel}, at(time.Second))
		due := ask(t, b, 2*time.Second)
		if answered, yes := duelAnswer(b, due); !answered || yes {
			t.Fatalf("answered %v yes %v in combat stance, want the decline", answered, yes)
		}
		b.handleOne(t, AutoAttackStop{ObjectID: selfID}, at(3*time.Second))
		due = ask(t, b, 4*time.Second)
		if answered, yes := duelAnswer(b, due); !answered || !yes {
			t.Fatalf("answered %v yes %v once the stance ended, want the acceptance", answered, yes)
		}
	})
	t.Run("a whispered duel from a player in a town", func(t *testing.T) {
		b := findBrain(t, nil)
		b.cfg.Travel.zones.Towns = []datapack.Zone{box(-100, 200, 100, 400)}
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(time.Second))
		if got := challenged(t, b, at(2*time.Second)); got != "" {
			t.Fatalf("challenged %q in a town", got)
		}
	})
}

// TestPvPChallengeWhisper checks the whispered duel (docs/pvp/finding.md
// "Sources").
func TestPvPChallengeWhisper(t *testing.T) {
	// whisperAt is name's (id's) whispered duel at d, and the player the
	// bot challenges at its reaction.
	whisperAt := func(t *testing.T, b *Brain, id int32, name string, d time.Duration) string {
		t.Helper()
		b.handleOne(t, whisper(id, name, "duel"), at(d))
		return challenged(t, b, b.pvp.find.whisper.due)
	}
	solo := func(t *testing.T) *Brain {
		b := findBrain(t, nil)
		b.cfg.Travel.classes = nil // no estimate: no own challenge
		return b
	}
	t.Run("challenge once per 30 minutes", func(t *testing.T) {
		b := solo(t)
		if got := whisperAt(t, b, pcID, foeName, time.Second); got != foeName {
			t.Fatalf("challenged %q, want %q", got, foeName)
		}
		b.handleOne(t, SystemMessage{ID: msgS1DeclinedYourDuel, Text: [2]string{foeName}}, at(2*time.Second))
		if got := whisperAt(t, b, pcID, foeName, 10*time.Minute); got != "" {
			t.Fatalf("challenged %q again within 30 minutes", got)
		}
		if got := whisperAt(t, b, pcID, foeName, 31*time.Minute); got != foeName {
			t.Fatalf("challenged %q after a decline 31 minutes on, want %q (no DeclineHold)", got, foeName)
		}
	})
	t.Run("bot-marked name", func(t *testing.T) {
		b := solo(t)
		b.handleOne(t, CharInfo{ObjectID: findOther, Name: markedName, Pos: Point{X: 300}}, t0)
		if got := whisperAt(t, b, findOther, markedName, time.Second); got != "" {
			t.Fatalf("challenged %q", got)
		}
	})
	t.Run("flagged target", func(t *testing.T) {
		b := solo(t)
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, Pos: Point{Y: 300}, PvPFlag: 1}, t0)
		if got := whisperAt(t, b, pcID, foeName, time.Second); got != "" {
			t.Fatalf("challenged %q", got)
		}
	})
	t.Run("shared 3 minutes", func(t *testing.T) {
		b := solo(t)
		b.handleOne(t, CharInfo{ObjectID: findOther, Name: otherName, Pos: Point{X: 300}}, t0)
		if got := whisperAt(t, b, pcID, foeName, time.Second); got != foeName {
			t.Fatalf("challenged %q, want %q", got, foeName)
		}
		b.handleOne(t, SystemMessage{ID: msgS1DeclinedYourDuel, Text: [2]string{foeName}}, at(2*time.Second))
		if got := whisperAt(t, b, findOther, otherName, 2*time.Minute); got != "" {
			t.Fatalf("challenged %q within 3 minutes", got)
		}
		if got := whisperAt(t, b, findOther, otherName, 3*time.Minute+2*time.Second); got != otherName {
			t.Fatalf("challenged %q after 3 minutes, want %q", got, otherName)
		}
	})
	t.Run("refused with 2019", func(t *testing.T) {
		b := findBrain(t, nil)
		if got := whisperAt(t, b, pcID, foeName, time.Second); got != foeName {
			t.Fatalf("challenged %q, want %q", got, foeName)
		}
		b.handleOne(t, SystemMessage{ID: msgS1CannotDuelHPOrMPBelowHalf, Text: [2]string{foeName}}, at(2*time.Second))
		if d := b.duel(at(2 * time.Second)); d.Phase != DuelIdle {
			t.Fatalf("phase %v after the refusal, want idle", d.Phase)
		}
		for _, d := range []time.Duration{31 * time.Minute, 90 * time.Minute} {
			b.pvp.find.lastAsk = time.Time{}
			if got := challenged(t, b, at(d)); got != "" {
				t.Fatalf("own challenge of %q at %v, inside DeclineHold", got, d)
			}
		}
		if got := whisperAt(t, b, pcID, foeName, 91*time.Minute); got != foeName {
			t.Fatalf("challenged %q on its whisper, want %q", got, foeName)
		}
	})
}

// TestPvPChallengeCooldown checks the bot's own challenges
// (docs/pvp/finding.md "Sources"): one per 3 minutes, a decline or refusal
// holding that player 2 hours, and the engage estimate.
func TestPvPChallengeCooldown(t *testing.T) {
	t.Run("one per 3 minutes", func(t *testing.T) {
		b := findBrain(t, nil)
		if got := challenged(t, b, at(time.Second)); got != foeName {
			t.Fatalf("challenged %q, want %q", got, foeName)
		}
		b.handleOne(t, SystemMessage{ID: msgS1DeclinedYourDuel, Text: [2]string{foeName}}, at(2*time.Second))
		b.handleOne(t, CharInfo{ObjectID: findOther, Name: otherName, Pos: Point{X: 400}}, at(2*time.Second))
		if got := challenged(t, b, at(2*time.Minute)); got != "" {
			t.Fatalf("challenged %q within 3 minutes", got)
		}
		if got := challenged(t, b, at(3*time.Minute+time.Second)); got != otherName {
			t.Fatalf("challenged %q after 3 minutes, want %q", got, otherName)
		}
	})
	for _, id := range []int32{msgS1DeclinedYourDuel, msgS1IsBusyTryLater, msgNoOpponentForDuel} {
		t.Run("held after "+strconv.Itoa(int(id)), func(t *testing.T) {
			b := findBrain(t, nil)
			if got := challenged(t, b, at(time.Second)); got != foeName {
				t.Fatalf("challenged %q, want %q", got, foeName)
			}
			b.handleOne(t, SystemMessage{ID: id, Text: [2]string{foeName}}, at(2*time.Second))
			if d := b.duel(at(2 * time.Second)); d.Phase != DuelIdle {
				t.Fatalf("phase %v, want idle at once", d.Phase)
			}
			if got := challenged(t, b, at(119*time.Minute)); got != "" {
				t.Fatalf("challenged %q inside 2 hours", got)
			}
			if got := challenged(t, b, at(2*time.Hour+3*time.Second)); got != foeName {
				t.Fatalf("challenged %q after 2 hours, want %q", got, foeName)
			}
		})
	}
	t.Run("engage estimate", func(t *testing.T) {
		b := findBrain(t, nil)
		mystic, _ := charmodel.ClassRow(10, false)
		b.pvp.rows = charmodel.RowTable{mystic: {{Skill: datapack.Skill{Type: "MDAM", Magic: true, Power: 1e6, HitTime: time.Second}}}}
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: 10, Pos: Point{Y: 300}}, t0)
		if got := challenged(t, b, at(time.Second)); got != "" {
			t.Fatalf("challenged %q, a candidate it loses to", got)
		}
		b.handleOne(t, CharInfo{ObjectID: findOther, Name: otherName, Pos: Point{X: 400}}, t0)
		if got := challenged(t, b, at(2*time.Second)); got != otherName {
			t.Fatalf("challenged %q, want %q (it beats)", got, otherName)
		}
		b.handleOne(t, SystemMessage{ID: msgS1DeclinedYourDuel, Text: [2]string{otherName}}, at(3*time.Second))
		b.handleOne(t, whisper(pcID, foeName, "duel"), at(4*time.Minute))
		if got := challenged(t, b, b.pvp.find.whisper.due); got != foeName {
			t.Fatalf("challenged %q on the losing candidate's whisper, want %q", got, foeName)
		}
	})
}
