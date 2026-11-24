package e2e

import (
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
// (#333). The GM grants both skills (the bot runs no trainer errand). It
// farms until a spoil took and a sweep earned items, and the server never
// refused one of its skills (113) or a sweep (343, 683). It keeps no loot,
// so what it earned came from its sweeps, and goes back to level 1 at the
// end.
func TestLiveSpoilSweep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 30*time.Minute)
	spots, travel := spotRegistry(t, tg), travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("ss", 1)
	setup := botSetup{
		class: &botclient.DwarvenFighter,
		// The spots of monsters up to 5 levels under it: a monster more
		// than 5 levels under the killer cuts its spoil chances (the
		// server's deep blue drop rules, model/item/drop.go), and a level
		// 2 monster's few-percent spoil items then hardly ever roll (none
		// in 11 spoils live); the level 7 to 9 packs of the spots of its
		// level are too much for a dwarf of level 10 in starter gear
		// (packFits), which leaves it no spot by its village.
		persona: func() botclient.Persona {
			p := farmerPersona(spoilLevel)()
			p.Spots.Above = 5
			return p
		},
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
	start, alive := time.Now(), time.Now()
	var r spoilRun
	untilBy(t, ctx, f.name+" spoiling and sweeping", start.Add(22*time.Minute), func() string {
		if time.Since(alive) > gmKeepAlive {
			gmTeleport(t, ctx, gm, townTI)
			alive = time.Now()
		}
		r = spoilRunOf(f.watch.Seen(from), f.id)
		if len(r.refused) > 0 {
			t.Fatalf("%s: the server refused %v (%s)", f.name, r.refused, r)
		}
		return r.short()
	})
	if all := spoilRunOf(f.watch.Seen(0), f.id); len(all.refused) > 0 {
		t.Errorf("%s: the server refused %v", f.name, all.refused)
	}
	t.Logf("%s farmed %s: %s (%v)", f.name, time.Since(start).Round(time.Second), r, r.earned)
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
