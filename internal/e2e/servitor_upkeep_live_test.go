package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Skills and monster of the servitor upkeep scenario
// (aCis_datapack/data/xml): Servitor Heal (skills/1100-1199.xml 1127,
// target SUMMON, range 600), Servitor Physical Shield (1140, a 1200 s
// buff) and Crokian Warrior (npcs/20000-20999.xml 20791), an aggressive
// level 38 melee monster of 886 HP and 183 P.Atk: Kat the Cat (level 42,
// 1050 HP) kills it in a fight that takes it about half its HP.
const (
	skillServitorHeal int32 = 1127
	skillServitorPDef int32 = 1140
	crokianWarrior    int32 = 20791
	// upkeepFights bounds the warriors the scenario spawns before the
	// servitor is under half its HP.
	upkeepFights = 4
	// fightBoundUpkeep bounds one warrior's fight and the heal after it
	// (the combat stance outlasts the fight by about 15 s).
	fightBoundUpkeep = 3 * time.Minute
)

// TestLiveServitorUpkeep runs a brain-run summoner (#759): a Human Mystic
// set to level 40 by the GM, given Summon Kat the Cat level 5, Servitor
// Heal, Servitor Physical Shield and Crystal D, with them as its persona's
// summon, heal and servitor buff (the summoner rows are #761's). Its
// summon rule casts the summon (cast plus one upkeep step: 6 Crystal D),
// its rebuff rule puts the buff on the new servitor, and a Crokian Warrior
// spawned next to it starts a fight: the bot orders the servitor onto the
// warrior (RequestActionUse 22, #758) and the servitor's swing at it
// follows; once the servitor is under half its HP the bot casts Servitor
// Heal on it (in combat under a quarter, else after the fight). No GM
// command summons for a player (network/admin_summon.go), so the brain's
// own summon brings the servitor out.
func TestLiveServitorUpkeep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("sk", 1)
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanMystic,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			p.Summons = []int32{skillSummonKat}
			p.ServitorHeal = skillServitorHeal
			p.ServitorBuffs = []int32{skillServitorPDef}
			// Nothing walks it off the place the GM finds it at.
			p.Loot.Pickup = false
			p.Gear.Buy = false
			p.Shop.Reach = 0
			p.TrainerReach = 0
			p.ClassChange = botclient.ClassChangePolicy{}
			p.Newbie = botclient.NewbiePolicy{}
			p.Supply = botclient.SupplyPolicy{}
			p.Travel = botclient.TravelPolicy{}
			return p
		},
		// The travel map as cmd/bots' bots have it: the summon gate reads
		// the summon's crystals and upkeep from its skills.
		brain: func(cfg *botclient.BrainConfig) { cfg.Travel = travel },
	})
	t.Cleanup(func() { backToLevel1(t, gm, f) })
	gmOn(t, ctx, gm, f,
		"set level "+fmt.Sprint(summonerLevel),
		fmt.Sprintf("skill set %d %d", skillSummonKat, summonKatLevel),
		fmt.Sprintf("skill set %d 1", skillServitorHeal),
		fmt.Sprintf("skill set %d 1", skillServitorPDef),
		fmt.Sprintf("give %d 20", crystalD),
		"heal",
	)
	// Out of the summoner's way, in a peace zone out of any monster's
	// reach, between the spawns.
	gmTeleport(t, ctx, gm, townTI)

	var (
		servitor, mob    int32
		hp, maxHP        int32
		buffed, hurtSeen bool
		ordered, struck  bool
		hurtAt           time.Time
		from             int
	)
	// scan reads what the bot saw since the last scan.
	scan := func() (died, mobDied, healed bool) {
		seen := f.watch.Seen(from)
		from += len(seen)
		for _, s := range seen {
			switch e := s.Event.(type) {
			case botclient.PetInfo:
				if e.SummonType != botclient.SummonServitor {
					continue
				}
				if servitor == 0 {
					servitor = e.ObjectID
					if e.TemplateID != summonKatNPC {
						t.Errorf("summoned npc %d, want Kat the Cat %d", e.TemplateID, summonKatNPC)
					}
				}
				if e.ObjectID == servitor {
					hp, maxHP = e.CurHP, e.MaxHP
				}
			case botclient.PetStatusUpdate:
				if e.ObjectID == servitor {
					hp, maxHP = e.CurHP, e.MaxHP
				}
			case botclient.MagicSkillLaunched:
				if e.CasterID == f.id && e.SkillID == skillServitorPDef && servitor != 0 {
					for _, id := range e.Targets {
						buffed = buffed || id == servitor
					}
				}
			case botclient.Attack:
				// The servitor's swing at the warrior after the bot's
				// attack order (22) went out.
				if servitor != 0 && mob != 0 && e.AttackerID == servitor && len(e.Hits) > 0 && e.Hits[0].TargetID == mob {
					struck = struck || orderedBy(f.watch.Sent(), s.SentBefore)
				}
			case botclient.MagicSkillUse:
				healed = healed || e.CasterID == f.id && e.SkillID == skillServitorHeal && hurtSeen
			case botclient.Die:
				died = died || servitor != 0 && e.ObjectID == servitor
				mobDied = mobDied || mob != 0 && e.ObjectID == mob
			}
			if !hurtSeen && servitor != 0 && maxHP > 0 && hp*2 < maxHP {
				hurtSeen, hurtAt = true, s.At
			}
		}
		return died, mobDied, healed
	}

	waitForBy(t, ctx, f.name+" summoning Kat the Cat", time.Now().Add(2*time.Minute), func() bool {
		scan()
		return servitor != 0
	})
	summons := 0
	for _, p := range f.watch.Sent() {
		if p.Op == botclient.OpRequestMagicSkillUse && p.Target == skillSummonKat {
			summons++
		}
	}
	t.Logf("%s summoned servitor %d after %d summon casts", f.name, servitor, summons)
	waitForBy(t, ctx, f.name+" buffing its servitor with Servitor Physical Shield", time.Now().Add(time.Minute), func() bool {
		scan()
		return buffed
	})

	for n := 1; ; n++ {
		if n > upkeepFights {
			t.Fatalf("servitor at %d/%d HP after %d warriors: never under half", hp, maxHP, upkeepFights)
		}
		gmTo(t, ctx, gm, f)
		spawned, err := gm.Spawn(ctx, crokianWarrior)
		if err != nil {
			t.Fatal(err)
		}
		mob = spawned
		gmTeleport(t, ctx, gm, townTI)
		healed, over := false, false
		waitForBy(t, ctx, fmt.Sprintf("warrior %d fought and the servitor healed", n), time.Now().Add(fightBoundUpkeep), func() bool {
			died, mobDied, h := scan()
			if died {
				t.Fatalf("servitor %d died before its heal", servitor)
			}
			healed = h
			over = mobDied
			return healed || mobDied && !hurtSeen
		})
		gm.Forget(mob)
		if healed {
			t.Logf("Servitor Heal %v after the servitor fell under half its HP, warrior %d (died %v)", time.Since(hurtAt).Round(time.Second), n, over)
			break
		}
	}
	ordered = orderedBy(f.watch.Sent(), len(f.watch.Sent()))
	if !ordered {
		t.Error("the bot never ordered its servitor to attack (RequestActionUse 22)")
	}
	if !struck {
		t.Error("the servitor never swung at a warrior the bot fought")
	}
}

// orderedBy reports whether a servitor attack order (RequestActionUse 22)
// is among the first n packets of sent.
func orderedBy(sent []Sent, n int) bool {
	for _, p := range sent[:min(n, len(sent))] {
		if p.Op == botwire.OpGameRequestActionUse && p.Target == botwire.ActionServitorAttack {
			return true
		}
	}
	return false
}
