package e2e

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Skills of the servitor upkeep scenario (aCis_datapack/data/xml):
// Servitor Heal (skills/1100-1199.xml 1127, target SUMMON, range 600) and
// Servitor Physical Shield (1140, a 1200 s buff).
const (
	skillServitorHeal int32 = 1127
	skillServitorPDef int32 = 1140
)

// TestLiveServitorUpkeep runs a brain-run summoner (#759): a Human Mystic
// set to level 40 by the GM, given Summon Kat the Cat level 5, Servitor
// Heal, Servitor Physical Shield and Crystal D, with them as its persona's
// summon, heal and servitor buff (the summoner rows are #761's). Its
// summon rule casts the summon (cast plus one upkeep step: 6 Crystal D),
// its rebuff rule puts the buff on the new servitor (MagicSkillLaunched on
// it). No GM command summons for a player (network/admin_summon.go), so the
// brain's own summon brings the servitor out. Not seen live yet (#768): the
// brain's attack order onto a monster it fights followed by the servitor's
// swing, and Servitor Heal on a hurt servitor; no GM command hurts a
// servitor, a monster that takes Kat the Cat's HP down kills the summoner
// first, and an owner that walks loses sight of its servitor
// (fatal10110/acis_golang#5589). botclient TestServitorHeal and the
// servitor order tests cover when they are sent.
func TestLiveServitorUpkeep(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 15*time.Minute)
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
			// Nothing walks it off the place the GM finds it at: the
			// server stops showing a servitor to an owner that walked
			// (fatal10110/acis_golang#5589).
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
		servitor int32
		buffed   bool
		from     int
	)
	// A failed run logs what the bot saw.
	t.Cleanup(func() {
		if t.Failed() {
			logServitorRun(t, f, servitor)
		}
	})
	// scan reads what the bot saw since the last scan.
	scan := func() {
		seen := f.watch.Seen(from)
		from += len(seen)
		for _, s := range seen {
			switch e := s.Event.(type) {
			case botclient.PetInfo:
				if e.SummonType == botclient.SummonServitor && servitor == 0 {
					servitor = e.ObjectID
					if e.TemplateID != summonKatNPC {
						t.Errorf("summoned npc %d, want Kat the Cat %d", e.TemplateID, summonKatNPC)
					}
				}
			case botclient.MagicSkillLaunched:
				if e.CasterID == f.id && e.SkillID == skillServitorPDef && servitor != 0 {
					buffed = buffed || slices.Contains(e.Targets, servitor)
				}
			case botclient.Die:
				if servitor != 0 && e.ObjectID == servitor {
					t.Fatalf("servitor %d died", servitor)
				}
			}
		}
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
}

// logServitorRun logs, in order, what the summoner f saw and sent about
// itself and its servitor: the servitor's HP, swings at or by either,
// deaths, casts, system messages and its casts and servitor orders; the
// servitor's HP and repeated swings are logged on change only.
func logServitorRun(t *testing.T, f *farmer, servitor int32) {
	ours := func(id int32) bool { return id != 0 && (id == f.id || id == servitor) }
	sent := f.watch.Sent()
	lines, lastHP, lastSwing, at := 0, int32(-1), "", 0
	logf := func(format string, args ...any) {
		if lines < 120 {
			t.Logf(format, args...)
		}
		lines++
	}
	for _, s := range f.watch.Seen(0) {
		for ; at < min(s.SentBefore, len(sent)); at++ {
			if p := sent[at]; p.Op == botwire.OpGameRequestActionUse || p.Op == botclient.OpRequestMagicSkillUse {
				logf("%s sent %#02x %d", p.At.Format("15:04:05.0"), p.Op, p.Target)
			}
		}
		ts := s.At.Format("15:04:05.0")
		switch e := s.Event.(type) {
		case botclient.PetInfo:
			logf("%s PetInfo %d npc %d HP %d/%d dead %v", ts, e.ObjectID, e.TemplateID, e.CurHP, e.MaxHP, e.Dead)
		case botclient.PetStatusUpdate:
			if e.ObjectID == servitor && e.CurHP != lastHP {
				lastHP = e.CurHP
				logf("%s servitor HP %d/%d", ts, e.CurHP, e.MaxHP)
			}
		case botclient.Attack:
			if len(e.Hits) > 0 && (ours(e.AttackerID) || ours(e.Hits[0].TargetID)) {
				if k := fmt.Sprintf("%d>%d", e.AttackerID, e.Hits[0].TargetID); k != lastSwing {
					lastSwing = k
					logf("%s swing %s", ts, k)
				}
			}
		case botclient.Die:
			if ours(e.ObjectID) {
				logf("%s Die %d", ts, e.ObjectID)
			}
		case botclient.MagicSkillUse:
			if ours(e.CasterID) {
				logf("%s cast %d by %d on %d", ts, e.SkillID, e.CasterID, e.TargetID)
			}
		case botclient.SystemMessage:
			logf("%s SystemMessage %d", ts, e.ID)
		case botclient.PetDelete:
			logf("%s PetDelete %d", ts, e.ObjectID)
		}
	}
	t.Logf("bot %d, servitor %d: %d lines", f.id, servitor, lines)
}
