package e2e

import (
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The Olympiad scenario's places and classes.
var (
	// gludinManager is the Grand Olympiad Manager's spawn in Gludin
	// (spawnlist/17_22.xml), the nearest to Talking Island's class master.
	gludinManager = botclient.Point{X: -84816, Y: 151056, Z: -3127}
)

const (
	// classCardinal is a third profession of the human mystic line: the
	// manager registers only nobles of a third profession
	// (folk_olympiad.go olympiadNobleCommand).
	classCardinal = int32(97)
	// The answers to the bot's registration and unregistration
	// (network/olympiad_manager.go).
	msgOlyRegistered   = 1503
	msgOlyUnregistered = 1505
	// olympiadScenario is the time the scenario needs inside the window,
	// before registration closes 10 minutes ahead of its end.
	olympiadScenario = 20 * time.Minute
)

// olympiadWindowOpen reports whether the competition window takes a
// registration for the whole scenario at now: from OlyStartTime 18:00 for
// OlyCPeriod 6 h in the server's local time (UTC on the owner's stack,
// ops/docker compose TZ), closing 10 minutes before its end
// (olympiad/registration.go RegisterClosing). No GM command opens it
// (network/admin_olympiad.go), and the owner keeps the server's window
// (D42), so the scenario runs only inside it.
func olympiadWindowOpen(now time.Time) (bool, string) {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from, to := day.Add(18*time.Hour), day.Add(24*time.Hour-10*time.Minute-olympiadScenario)
	if now.Before(from) || !now.Before(to) {
		return false, fmt.Sprintf("the Olympiad registers from 18:00 to 23:50 server time (UTC) and the scenario needs %v of it; it is %s UTC (no GM command opens the window, D42)",
			olympiadScenario, now.Format("15:04"))
	}
	return true, ""
}

// TestLiveOlympiadRegister checks the Olympiad's registration
// (docs/pvp/olympiad.md, X16, D42): a bot of a third profession with
// Olympiad on and the default OlympiadPolicy (ThirdClass on: the owner's
// ruling on #729) takes noblesse at the class master with
// one become_noble, walks to the Grand Olympiad Manager, registers for
// classed games once (the manager answers 1503) and unregisters after its
// shortened OlympiadWait (1505). It runs only inside the competition
// window; outside it, it skips with the reason.
func TestLiveOlympiadRegister(t *testing.T) {
	if ok, why := olympiadWindowOpen(time.Now()); !ok {
		t.Skip(why)
	}
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 30*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	masterAt(t, ctx, gm)

	account := freshAccount(tg, "ol")
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanMystic,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			errandsOff(&p)
			// The visit for noblesse alone: no profession is due.
			p.ClassChange = botclient.DefaultPersona().ClassChange
			p.ClassChange.Builds = map[int32]int32{}
			p.PvP.Olympiad = true
			p.Olympiad.Wait = time.Minute
			return p
		},
		brain: withTravel(travel),
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.digest())
			t.Log(f.visits())
		}
	})
	recallBot(t, ctx, tg, gm, f.name)
	levelTo(t, ctx, gm, f, 76)
	gmOn(t, ctx, gm, f, "set class "+strconv.Itoa(int(classCardinal)))
	becomes(t, ctx, f, classCardinal)

	until(t, ctx, f.name+" takes noblesse", func() string {
		if !slices.ContainsFunc(f.watch.Seen(0), func(s Seen) bool {
			u, ok := s.Event.(botclient.UserInfo)
			return ok && u.Noble
		}) {
			return "not noble (rule " + f.bot.Status().Rule + ")"
		}
		return ""
	})

	gmTeleport(t, ctx, gm, gludinManager)
	recallBot(t, ctx, tg, gm, f.name)
	from := f.seen()
	for _, msg := range []int32{msgOlyRegistered, msgOlyUnregistered} {
		until(t, ctx, fmt.Sprintf("%s is answered %d", f.name, msg), func() string {
			if !slices.ContainsFunc(f.watch.Seen(from), func(s Seen) bool {
				m, ok := s.Event.(botclient.SystemMessage)
				return ok && m.ID == msg
			}) {
				return "no answer yet (rule " + f.bot.Status().Rule + ")"
			}
			return ""
		})
	}
}
