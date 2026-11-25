package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// Spoil and Sweeper (data/xml/skills; the dwarven fighter learns both at 10,
// data/xml/classes/dwarfFighter.xml) and the system messages about them
// (acis_golang serverpackets/systemmessage.go): 612 a spoil that took, 343
// and 683 Sweeper refused on a corpse the sweeper did not spoil or may not
// sweep, 53 and 54 the items a sweep earns (Character.AddEarnedItem).
const (
	skillSpoil         int32 = 254
	skillSweeper       int32 = 42
	spoilLevel         int32 = 10
	msgSpoilSuccess    int32 = 612
	msgSweepNotSpoiled int32 = 343
	msgSweepNotAllowed int32 = 683
	msgEarnedItems     int32 = 53
	msgEarnedItem      int32 = 54
)

// TestLiveSpoilSweep runs a level 10 dwarven fighter farmer with its kit
// (S10, #329): Spoil opens on a fresh monster, and once a monster it spoiled
// dies it casts Sweeper on the corpse, which earns it the corpse's spoil
// (#333). The GM grants both skills (the bot runs no trainer errand). Once
// the bot holds a spot the GM spawns a paralysed Werewolf Hunter at it, one
// at a time, until a spoil took and a sweep earned items, and the server
// never refused one of its skills (113) or a sweep (343, 683). It keeps no
// loot, so what it earned came from its sweeps, and goes back to level 1 at
// the end.
func TestLiveSpoilSweep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 35*time.Minute)
	spots, travel := spotRegistry(t, tg), travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("ss", 1)
	setup := botSetup{
		class: &botclient.DwarvenFighter,
		// The level 7 to 9 packs of the spots of its level are too much
		// for a dwarf of level 10 in starter gear (packFits): it falls back
		// to the highest spot under its level that fits (#446), where the
		// GM spawns the hunters.
		persona: farmerPersona(spoilLevel),
		// The travel map as cmd/bots' farmers have it: the corpse times
		// of its NPC templates bound the sweep window (TravelMap.corpseTime).
		brain: func(cfg *botclient.BrainConfig) { cfg.Spots, cfg.Travel = spots, travel },
	}
	f := runFresh(t, ctx, gm, int(spoilLevel), func() *farmer { return runWatched(t, ctx, tg, account, setup) })
	logSpotOnFail(t, spots, f)
	kit := kitOf(t, botclient.DwarvenFighter.ID, spoilLevel, account)
	if !slices.Contains(kit.rotation, skillSpoil) || !slices.Contains(kit.all, skillSweeper) {
		t.Fatalf("the level %d dwarven fighter's kit casts %v and learns %v: no Spoil or Sweeper", spoilLevel, kit.rotation, kit.all)
	}
	levelTo(t, ctx, gm, f, spoilLevel)
	gmOn(t, ctx, gm, f, kit.grants()...)
	from := f.seen()
	gmTeleport(t, ctx, gm, townTI)
	var hunters []hunterSpawn
	t.Cleanup(func() {
		if t.Failed() {
			seen := f.watch.Seen(from)
			for _, l := range hunterOutcomes(seen, f.id, hunters, time.Now()) {
				t.Log(l)
			}
			for _, l := range sweepTrace(seen, f.watch.Sent(), f.id) {
				t.Log(l)
			}
		}
	})
	start, alive := time.Now(), time.Now()
	var r spoilRun
	untilBy(t, ctx, f.name+" spoiling and sweeping", start.Add(spoilBound), func() string {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
		seen := f.watch.Seen(from)
		r = spoilRunOf(seen, f.id)
		if len(r.refused) > 0 {
			t.Fatalf("%s: the server refused %v (%s)", f.name, r.refused, r)
		}
		if r.short() == "" {
			return ""
		}
		if n := len(hunters); n > 0 && !hunterDone(seen, hunters[n-1].mob, hunters[n-1].at, time.Now()) {
			return fmt.Sprintf("hunter %d of %d: %s", n, spoilHunters, r)
		}
		s := f.bot.Status()
		if s.Spot == "" {
			return fmt.Sprintf("no spot held (rule %q at %v): %s", s.Rule, s.Position, r)
		}
		// The bot arrived and fights or roams within its spot's leash, where
		// a hunter spawned at it is a farm target (Brain.mayFarm): one spawned
		// on its walk to the spot is left behind (run 1909dce, hunter 1).
		if !slices.Contains(farmingRules, s.Rule) {
			return fmt.Sprintf("at spot %s, not farming it (rule %q at %v): %s", s.Spot, s.Rule, s.Position, r)
		}
		if len(hunters) == spoilHunters {
			t.Fatalf("%s: %d Werewolf Hunters spawned at it: %s", f.name, len(hunters), r)
		}
		hunters = append(hunters, hunterSpawn{mob: spawnHunter(t, ctx, gm, f), at: time.Now()})
		alive = time.Now()
		return fmt.Sprintf("hunter %d of %d spawned: %s", len(hunters), spoilHunters, r)
	})
	if all := spoilRunOf(f.watch.Seen(0), f.id); len(all.refused) > 0 {
		t.Errorf("%s: the server refused %v", f.name, all.refused)
	}
	t.Logf("%s spoiled and swept in %s, %d Werewolf Hunters spawned: %s (%v)", f.name, time.Since(start).Round(time.Second), len(hunters), r, r.earned)
	for _, l := range hunterOutcomes(f.watch.Seen(from), f.id, hunters, time.Now()) {
		t.Log(l)
	}
}

// hunterSpawn is the Werewolf Hunter mob the GM spawned at at.
type hunterSpawn struct {
	mob int32
	at  time.Time
}

// hunterOutcomes is a line for each hunter of what became of it at now in
// seen, as the bot self read it: how many Spoil casts the bot aimed at it and
// whether one took (612), whether it died, how long after its spawn, who
// struck it last (the attacker of the last Attack hitting it or the caster
// of the last skill aimed at it), whether its Die said sweepable, and
// whether the bot cast Sweeper at its corpse; or that it still lives, past
// hunterKill or not.
func hunterOutcomes(seen []Seen, self int32, hunters []hunterSpawn, now time.Time) []string {
	out := make([]string, 0, len(hunters))
	for i, h := range hunters {
		var (
			spoils, swept      int
			spoiled, sweepable bool
			striker, lastSpoil int32
			died               time.Time
		)
		for _, s := range seen {
			switch e := s.Event.(type) {
			case botclient.MagicSkillUse:
				if e.CasterID == self && e.SkillID == skillSpoil {
					lastSpoil = e.TargetID
				}
				if e.TargetID != h.mob {
					continue
				}
				switch {
				case e.CasterID == self && e.SkillID == skillSpoil:
					spoils++
				case e.CasterID == self && e.SkillID == skillSweeper:
					swept++
				}
				if died.IsZero() {
					striker = e.CasterID
				}
			case botclient.Attack:
				if died.IsZero() && slices.ContainsFunc(e.Hits, func(hit botclient.AttackHit) bool { return hit.TargetID == h.mob }) {
					striker = e.AttackerID
				}
			case botclient.SystemMessage:
				if e.ID == msgSpoilSuccess && lastSpoil == h.mob {
					spoiled = true
				}
			case botclient.Die:
				if e.ObjectID == h.mob && died.IsZero() {
					died, sweepable = s.At, e.Sweepable
				}
			}
		}
		line := fmt.Sprintf("hunter %d (%d): %d Spoil casts, spoiled %v; ", i+1, h.mob, spoils, spoiled)
		switch {
		case !died.IsZero():
			by := fmt.Sprint(striker)
			if striker == self {
				by = "the bot"
			}
			line += fmt.Sprintf("died %s after its spawn, last struck by %s, sweepable %v, %d Sweeper casts at it",
				died.Sub(h.at).Round(time.Second), by, sweepable, swept)
		case now.Sub(h.at) > hunterKill:
			line += fmt.Sprintf("alive past hunterKill (%s)", hunterKill)
		default:
			line += fmt.Sprintf("alive %s after its spawn", now.Sub(h.at).Round(time.Second))
		}
		out = append(out, line)
	}
	return out
}

// werewolfHunter is the Werewolf Hunter template (data/xml/npcs, 20343): a
// passive level 10 monster of 163 HP whose spoil rolls each of its items
// independently (model/item/drop.go rollOrdered), at least one of 40.4%,
// 68.7% and 1.3% about 82% of the time. At the spoiler's own level the
// server's deep blue rule leaves its chances whole (LevelPenaltyMultiplier),
// where a farming spot's monsters, more than 5 levels under the bot and with
// spoil chances of a few percent, rarely give anything to sweep: the spoiled
// monsters that died unsweepable in the runs so far were the spot's, and
// the one hunter the bot fought in run 1909dce died spoiled and sweepable
// (hunterOutcomes).
const werewolfHunter int32 = 20343

// farmingRules are the rules of a bot that arrived at its spot: it attacks
// a farm target (farm), or has none and roams or idles within the leash
// (goal, idle, or no rule at all). On its walk there the rule is spot.
var farmingRules = []string{"farm", "goal", "idle", ""}

// spoilHunters is how many hunters a run spawns at most, one at a time,
// each once the bot farms its spot. A hunter the bot fights most likely
// dies spoiled (a spoil resisted, 139, is cast again while it lives) and
// four in five of those leave items to sweep, but the bot, busy with its
// spot's own monsters, fought only one hunter in two within 3 minutes of
// its spawn (runs 1909dce and 04a273a: hunter 1 never fought, hunter 2
// spoiled and swept after 58s and 1m48s). At that rate nine all coming up
// empty is about one run in 500; the hunters left alive stay targets.
const spoilHunters = 9

// spoilBound bounds the spoil step: each hunter is hunterKill at most, then
// its sweep window.
const spoilBound = 20 * time.Minute

// hunterKill is how long a spawned hunter may live before the next one is
// spawned: the bot may be busy with its spot's monsters first.
const hunterKill = 2 * time.Minute

// hunterDone reports whether the hunter mob spawned at spawned is done with
// at now: dead and its sweep window over, or still alive past hunterKill.
func hunterDone(seen []Seen, mob int32, spawned, now time.Time) bool {
	for _, s := range seen {
		if d, ok := s.Event.(botclient.Die); ok && d.ObjectID == mob {
			return now.Sub(s.At) > sweepWindow
		}
	}
	return now.Sub(spawned) > hunterKill
}

// spawnHunter spawns a Werewolf Hunter at f and paralyses it so that it
// stays where f fights it and never hits back, and returns its object id.
// The GM leaves for a peace zone out of the way.
func spawnHunter(t *testing.T, ctx context.Context, gm *GMClient, f *farmer) int32 {
	t.Helper()
	gmOn(t, ctx, gm, f)
	mob, err := gm.Spawn(ctx, werewolfHunter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gmBackFor(t, gm, f, mob) })
	if err := gm.OnTarget(ctx, mob, "para"); err != nil {
		t.Fatal(err)
	}
	gmTeleport(t, ctx, gm, townTI)
	return mob
}

// spoilRun is what a spoiler did: its Spoil casts and those that took, the
// monsters it spoiled that died and those of them whose Die said sweepable,
// its Sweeper casts and those that earned items, the system messages of
// what it earned, and those of its refused skills and sweeps.
type spoilRun struct {
	spoils, spoiled, died, sweepable, sweeps, paid int
	earned                                         []botclient.SystemMessage
	refused                                        []int32
}

// refusedSpoil are the system messages a spoiler never reads.
var refusedSpoil = []int32{msgS1CannotBeUsed, msgSweepNotSpoiled, msgSweepNotAllowed}

// spoilRunOf is the spoilRun of the bot self in seen. A sweep earned items
// when an earned item message (53, 54) follows its Sweeper cast before the
// bot's next cast. A spoil took on each 612, which the server sends only to
// the caster, on the target of the bot's last Spoil cast.
func spoilRunOf(seen []Seen, self int32) spoilRun {
	var r spoilRun
	sweeping, paid := false, false
	var target int32
	spoiled := make(map[int32]bool)
	for _, s := range seen {
		switch e := s.Event.(type) {
		case botclient.MagicSkillUse:
			if e.CasterID != self {
				continue
			}
			sweeping, paid = e.SkillID == skillSweeper, false
			switch e.SkillID {
			case skillSpoil:
				r.spoils++
				target = e.TargetID
			case skillSweeper:
				r.sweeps++
			}
		case botclient.SystemMessage:
			switch {
			case e.ID == msgSpoilSuccess:
				r.spoiled++
				spoiled[target] = true
			case slices.Contains(refusedSpoil, e.ID):
				r.refused = append(r.refused, e.ID)
			case (e.ID == msgEarnedItems || e.ID == msgEarnedItem) && sweeping:
				r.earned = append(r.earned, e)
				if !paid {
					paid = true
					r.paid++
				}
			}
		case botclient.Die:
			if spoiled[e.ObjectID] {
				delete(spoiled, e.ObjectID)
				r.died++
				if e.Sweepable {
					r.sweepable++
				}
			}
		}
	}
	return r
}

// short is "" once a spoil of r took and a sweep earned items, else what
// is still missing.
func (r spoilRun) short() string {
	if r.spoiled > 0 && r.paid > 0 {
		return ""
	}
	return r.String()
}

// String is r in a line.
func (r spoilRun) String() string {
	return fmt.Sprintf("%d Spoil casts, %d took; %d spoiled monsters died, %d sweepable; %d Sweeper casts, %d earned items",
		r.spoils, r.spoiled, r.died, r.sweepable, r.sweeps, r.paid)
}

// sweepWindow is how long after a sweepable Die sweepTrace follows the bot:
// twice the default corpse time, the sweep window of a spoiled corpse.
const sweepWindow = 14 * time.Second

// sweepTrace is, for each monster the bot self spoiled (612 after its Spoil
// cast) that died sweepable, what the bot read and wrote in the sweepWindow
// after its Die: a failed scenario's account of a sweep that never went out.
func sweepTrace(seen []Seen, sent []Sent, self int32) []string {
	var out []string
	var target int32
	spoiled := make(map[int32]bool)
	for i, s := range seen {
		switch e := s.Event.(type) {
		case botclient.MagicSkillUse:
			if e.CasterID == self && e.SkillID == skillSpoil {
				target = e.TargetID
			}
		case botclient.SystemMessage:
			if e.ID == msgSpoilSuccess {
				spoiled[target] = true
			}
		case botclient.Die:
			if !spoiled[e.ObjectID] || !e.Sweepable {
				continue
			}
			end := s.At.Add(sweepWindow)
			out = append(out, fmt.Sprintf("sweepable corpse %d died at %s:", e.ObjectID, s.At.Format("15:04:05.000")))
			for _, n := range seen[i+1:] {
				if n.At.After(end) {
					break
				}
				out = append(out, fmt.Sprintf("  %s read %T %+v", n.At.Format("15:04:05.000"), n.Event, n.Event))
			}
			for _, p := range sent {
				if !p.At.Before(s.At) && !p.At.After(end) {
					out = append(out, fmt.Sprintf("  %s sent 0x%02x target %d", p.At.Format("15:04:05.000"), p.Op, p.Target))
				}
			}
		}
	}
	return out
}
