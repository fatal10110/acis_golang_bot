package e2e

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The kit farms (#333, docs/strategy.md "Testing"): each runs three level
// kitLevel farmers of starting classes on one spot registry, the GM handing
// each its build's kit skills, until every one killed kitKills monsters it
// fought and cast its rotation kitCasts times over at least kitFarm. A
// rotation with weapon-gated steps (Step.Weapons) must have cast one of them,
// and no farmer may ever read SystemMessage 113, the server's refusal of a
// skill whose condition, the weapon among them, fails.
const (
	// kitLevel opens the fighters' attacks (from 5) and the mystics' second
	// spells (from 7).
	kitLevel = 7
	kitKills = 3
	kitCasts = 3
	kitFarm  = 3 * time.Minute
)

// farmerRole is the role the scenarios play: the farmer preset's, the
// default preset of cmd/bots (D21).
const farmerRole = botclient.RoleDamage

// farmerPersona is the farmer preset's persona (cmd/bots damagePersona)
// for a bot the GM levels to level in its newbie zone, with no loot, which
// carried run after run would weigh a character set back to level 1 down,
// and no errands. Nearly every monster of a newbie zone is aggressive
// (#133): held to AggroAbove past SpotPolicy.Newbie, a bot levelled there
// finds a fitting spot only when one is free, so the scenarios bind the
// aggressive monsters by Below up to level, as at the newbie levels.
func farmerPersona(level int32) func() botclient.Persona {
	return func() botclient.Persona {
		p := botclient.DefaultPersona()
		p.Role = farmerRole
		p.Loot.Pickup = false
		p.Spots.Newbie = int(level)
		errandsOff(&p)
		return p
	}
}

// TestLiveBuildFighters farms a human, an elven and a dark elven fighter
// with their kits: their attacks are gated by the weapon they wield (Power
// Strike with a sword or blunt, Mortal Blow with a dagger, Power Shot with
// a bow).
func TestLiveBuildFighters(t *testing.T) {
	t.Parallel()
	farmKits(t, "kf", nil, &botclient.HumanFighter, &botclient.ElvenFighter, &botclient.DarkFighter)
}

// TestLiveBuildMystics farms a human, an elven and a dark elven mystic with
// their kits: Wind Strike, Ice Bolt and their self heal.
func TestLiveBuildMystics(t *testing.T) {
	t.Parallel()
	farmKits(t, "km", mysticSpots, &botclient.HumanMystic, &botclient.ElvenMystic, &botclient.DarkMystic)
}

// mysticSpots are the spots by the human, elven and dark elven villages
// the level kitLevel mystics farmed live. The spot model would send them
// elsewhere (#446): oren23_2018_04m1 fits a dark mystic, but its territory
// is so wide that one held at its centre found no monster in its leash for
// 12 minutes.
var mysticSpots = []string{
	"gludio32_1725_04m1", "gludio32_1725_06m1",
	"oren02_2119_02m1", "oren02_2119_022sm1",
	"oren23_2018_16sm1",
}

// TestLiveBuildMixed farms the other races' starting classes with their
// kits: an orc fighter (Iron Punch with fists, Power Strike with a sword or
// blunt), an orc mystic (Chill Flame, Life Drain, Soul Cry) and a dwarven
// fighter, which melees: its kit's Spoil opens from 10
// (TestLiveSpoilSweep).
func TestLiveBuildMixed(t *testing.T) {
	t.Parallel()
	farmKits(t, "kx", nil, &botclient.OrcFighter, &botclient.OrcMystic, &botclient.DwarvenFighter)
}

// farmKits runs one kit farm of classes, the accounts of group, each a
// farmerPersona with the travel map and the spots, those named only when
// only is not nil; they go back to level 1 at the end (runFresh).
func farmKits(t *testing.T, group string, only []string, classes ...*botclient.StartingClass) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 25*time.Minute)
	spots, travel := spotRegistry(t, tg), travelMap(t, tg)
	if only != nil {
		spots = spotsNamed(t, tg, only)
	}
	gm := enterGM(t, ctx, tg)
	type kitFarmer struct {
		f    *farmer
		kit  kitSkills
		from int
	}
	farmers := make([]kitFarmer, len(classes))
	for i, class := range classes {
		account, _ := tg.Account(group, i+1)
		persona := farmerPersona(kitLevel)
		if class.Mystic {
			// The spot model prices a level 7 mystic in starter gear out
			// of every spot by its village once its stats are in (#446):
			// one that claimed a spot before keeps it, one that did not
			// idles in town. The kit run is what this scenario proves, so
			// its mystics farm mysticSpots with the model bound off.
			persona = func() botclient.Persona {
				p := farmerPersona(kitLevel)()
				p.Spots.Margin = -1
				return p
			}
		}
		setup := botSetup{
			class:   class,
			persona: persona,
			// The travel map as cmd/bots' farmers have it: the brain
			// tells the worn weapon's kind from its items (wornKind), and
			// a weapon-gated step never passes without it.
			brain: func(cfg *botclient.BrainConfig) { cfg.Spots, cfg.Travel = spots, travel },
		}
		f := runFresh(t, ctx, gm, kitLevel, func() *farmer { return runWatched(t, ctx, tg, account, setup) })
		logSpotOnFail(t, spots, f)
		kit := kitOf(t, class.ID, kitLevel, account)
		levelTo(t, ctx, gm, f, kitLevel)
		gmOn(t, ctx, gm, f, kit.grants()...)
		farmers[i] = kitFarmer{f: f, kit: kit, from: f.seen()}
	}
	// Out of the farmers' way, in a peace zone out of any monster's reach.
	gmTeleport(t, ctx, gm, townTI)
	start, alive := time.Now(), time.Now()
	for _, k := range farmers {
		untilBy(t, ctx, k.f.name+" farming with its kit", start.Add(15*time.Minute), func() string {
			if time.Since(alive) > gmKeepAlive {
				gmTeleport(t, ctx, gm, townTI)
				alive = time.Now()
			}
			r := kitRunOf(k.f.watch.Seen(k.from), k.f.id, k.kit)
			if bad := r.refusals(); bad != "" {
				t.Fatalf("%s: %s", k.f.name, bad)
			}
			return r.short(time.Since(start))
		})
	}
	for _, k := range farmers {
		r := kitRunOf(k.f.watch.Seen(k.from), k.f.id, k.kit)
		// The whole run, the level up and the grants included.
		if bad := kitRunOf(k.f.watch.Seen(0), k.f.id, k.kit).refusals(); bad != "" {
			t.Errorf("%s: %s", k.f.name, bad)
		}
		t.Logf("%s (class %d, level %d, worn %v) farmed %s: %d kills, casts %v, weapon-gated %v",
			k.f.name, k.f.bot.Status().ClassID, k.f.bot.Status().Level, slices.Sorted(maps.Keys(worn(inventory(k.f.watch.Seen(0))))),
			time.Since(start).Round(time.Second), r.kills, r.casts, r.gatedCasts())
	}
}

// kitSkills is what a kit farm reads of a build: every skill its kit learns
// or casts, the GM grants them all, and its rotation's skills, those of its
// weapon-gated steps apart.
type kitSkills struct {
	all, rotation, gated []int32
}

// kitOf is the kit of a bot of class at level in farmerRole,
// its line drawn for account as the bot draws it (CompileBuild).
func kitOf(t *testing.T, class, level int32, account string) kitSkills {
	t.Helper()
	race, ok := datapack.ClassRace(class)
	if !ok {
		t.Fatalf("class %d: no race", class)
	}
	return kitFrom(botclient.CompileBuild(race, class, level, farmerRole, botclient.BranchChoice{Account: account}).Kit)
}

// kitFrom is the kitSkills of k.
func kitFrom(k botclient.ClassKit) kitSkills {
	var s kitSkills
	add := func(ids *[]int32, id int32) {
		if id != 0 && !slices.Contains(*ids, id) {
			*ids = append(*ids, id)
		}
	}
	for _, id := range k.Skills {
		add(&s.all, id)
	}
	for _, st := range k.Rotation {
		add(&s.all, st.Skill)
		add(&s.rotation, st.Skill)
		if len(st.Weapons) > 0 {
			add(&s.gated, st.Skill)
		}
	}
	for _, b := range k.Buffs {
		add(&s.all, b.Skill)
	}
	for _, h := range k.Heals {
		add(&s.all, h.Skill)
	}
	for _, g := range k.Toggles {
		add(&s.all, g.Skill)
	}
	add(&s.all, k.Relax)
	add(&s.all, k.Sweep)
	return s
}

// grants are the GM commands that teach every skill of s at level 1.
func (s kitSkills) grants() []string {
	cmds := make([]string, len(s.all))
	for i, id := range s.all {
		cmds[i] = "skill set " + strconv.Itoa(int(id)) + " 1"
	}
	return cmds
}

// kitRun is what a farmer did with its kit: its casts by skill, the monsters
// it fought (swung at or cast at) that died, and the system messages of its
// refused skills.
type kitRun struct {
	kit     kitSkills
	casts   map[int32]int
	kills   int
	refused []int32
}

// refusedSkill are the system messages a kit farm never reads: 113, a
// skill's condition (its weapon among them) failing.
var refusedSkill = []int32{msgS1CannotBeUsed}

// kitRunOf is the kitRun of the bot self in seen.
func kitRunOf(seen []Seen, self int32, kit kitSkills) kitRun {
	r := kitRun{kit: kit, casts: make(map[int32]int)}
	fought, dead := make(map[int32]bool), make(map[int32]bool)
	for _, s := range seen {
		switch e := s.Event.(type) {
		case botclient.Attack:
			if e.AttackerID == self {
				for _, h := range e.Hits {
					fought[h.TargetID] = true
				}
			}
		case botclient.MagicSkillUse:
			if e.CasterID == self {
				r.casts[e.SkillID]++
				if e.TargetID != self && e.TargetID != 0 {
					fought[e.TargetID] = true
				}
			}
		case botclient.Die:
			if fought[e.ObjectID] && !dead[e.ObjectID] {
				dead[e.ObjectID] = true
				r.kills++
			}
		case botclient.SystemMessage:
			if slices.Contains(refusedSkill, e.ID) {
				r.refused = append(r.refused, e.ID)
			}
		}
	}
	return r
}

// rotationCasts is how many casts of r are of its kit's rotation.
func (r kitRun) rotationCasts() int {
	n := 0
	for _, id := range r.kit.rotation {
		n += r.casts[id]
	}
	return n
}

// gatedCasts are r's casts of its kit's weapon-gated steps, by skill.
func (r kitRun) gatedCasts() map[int32]int {
	g := make(map[int32]int)
	for _, id := range r.kit.gated {
		if r.casts[id] > 0 {
			g[id] = r.casts[id]
		}
	}
	return g
}

// refusals is "" when r read no refused skill, else what it read.
func (r kitRun) refusals() string {
	if len(r.refused) == 0 {
		return ""
	}
	return fmt.Sprintf("the server refused a skill: system messages %v (casts %v)", r.refused, r.casts)
}

// short is "" once r farmed its kit for long enough after farming for
// elapsed, else what it still lacks.
func (r kitRun) short(elapsed time.Duration) string {
	var miss []string
	if r.kills < kitKills {
		miss = append(miss, fmt.Sprintf("%d of %d kills", r.kills, kitKills))
	}
	if n := r.rotationCasts(); len(r.kit.rotation) > 0 && n < kitCasts {
		miss = append(miss, fmt.Sprintf("%d of %d casts of %v", n, kitCasts, r.kit.rotation))
	}
	if len(r.kit.gated) > 0 && len(r.gatedCasts()) == 0 {
		miss = append(miss, fmt.Sprintf("no weapon-gated cast of %v", r.kit.gated))
	}
	if elapsed < kitFarm {
		miss = append(miss, fmt.Sprintf("%s of %s farmed", elapsed.Round(time.Second), kitFarm))
	}
	return strings.Join(miss, ", ")
}

// spotsNamed is a registry of the datapack's spots named names, every one
// of which must exist.
func spotsNamed(t *testing.T, tg *Target, names []string) *botclient.SpotRegistry {
	t.Helper()
	dp, err := datapack.Load(tg.Config().Datapack)
	if err != nil {
		t.Fatal(err)
	}
	spots := slices.DeleteFunc(dp.Spots(), func(s datapack.Spot) bool { return !slices.Contains(names, s.Maker) })
	if len(spots) != len(names) {
		t.Fatalf("%d of the spots %v in the datapack", len(spots), names)
	}
	return botclient.NewSpotRegistry(spots, 1)
}

// logSpotOnFail logs, when the test fails, the spot f holds then and its
// registry status: levels, aggressive level, holders and cap. Registered
// after runFresh, it runs before the level goes back to 1.
func logSpotOnFail(t *testing.T, spots *botclient.SpotRegistry, f *farmer) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		st := f.bot.Status()
		for _, s := range spots.Statuses() {
			if s.Name == st.Spot {
				t.Logf("%s (level %d, rule %q) holds %+v", f.name, st.Level, st.Rule, s)
				return
			}
		}
		t.Logf("%s (level %d, rule %q) holds no spot of the registry (%q)", f.name, st.Level, st.Rule, st.Spot)
	})
}
