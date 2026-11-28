package e2e

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// The corpse summon scenario's skill and items (aCis_datapack/data/xml):
// Summon Reanimated Man level 1 (skills/1100-1199.xml 1129: CORPSE_MOB,
// cast range 40, 1.5 s, 2 Crystal C a cast and 1 an upkeep step, npc
// 14001) and Crystal C.
const (
	skillSummonReanimatedMan int32 = 1129
	reanimatedManNPC         int32 = 14001
	crystalC                 int32 = 1459
	corpseSummonerLevel            = 44
	// Refusals of a corpse cast (serverpackets/systemmessage.go): the
	// corpse too old (1247), a second summon while one stands (580), no
	// corpse (109), a failed condition (113).
	msgCorpseTooOld    int32 = 1247
	msgSummonStands    int32 = 580
	msgCorpseInvalid   int32 = 109
	msgCorpseCannotUse int32 = 113
	// corpseKills bounds the wolves the GM kills next to the bot until it
	// casts the summon on one's corpse.
	corpseKills = 3
)

// TestLiveCorpseSummon runs a brain-run corpse summoner (#762): a Human
// Mystic set to level 44 by the GM, given Summon Reanimated Man and Crystal
// C, with it as its persona's corpse summon (the Necromancer's row draws
// only with ServitorLines). The GM spawns a wolf at the bot and kills it,
// so its corpse falls in melee range; the bot's corpse rule casts the
// summon on the fresh corpse, and the Reanimated Man comes out (PetInfo).
// A 1247 (corpse too old), 580, 109 or 113 answer to the cast fails the
// step at once. A wolf whose corpse the bot let pass is followed by
// another, up to corpseKills. Every frame about the bot's cast, the wolves
// and the servitor, after the character's first UserInfo, goes in order
// into the corpse directory of BOT_E2E_RECORD when set, else a temporary
// one, as NNN-OP.bin: those are botclient's testdata/corpse_summon
// fixtures.
func TestLiveCorpseSummon(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 10*time.Minute)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("cs", 1)
	tap := &corpseTap{}
	f := runWatched(t, ctx, tg, account, botSetup{
		class: &botclient.HumanMystic,
		tap:   tap,
		persona: func() botclient.Persona {
			p := botclient.DefaultPersona()
			p.IdleChance = 0
			p.CorpseSummons = []int32{skillSummonReanimatedMan}
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
		// The travel map as cmd/bots' bots have it: the corpse rule reads
		// the summon's cast range, crystals and the wolf's corpse time.
		brain: func(cfg *botclient.BrainConfig) { cfg.Travel = travel },
	})
	t.Cleanup(func() { backToLevel1(t, gm, f) })
	var wolves []int32
	var servitor int32
	defer func() {
		dir := filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "corpse")
		n, err := tap.write(dir, f.id, append([]int32{servitor}, wolves...))
		if err != nil {
			t.Error(err)
		}
		t.Logf("servitor %d, wolves %v: %d frames recorded in %s", servitor, wolves, n, dir)
	}()
	gmOn(t, ctx, gm, f,
		"set level "+fmt.Sprint(corpseSummonerLevel),
		fmt.Sprintf("skill set %d 1", skillSummonReanimatedMan),
		fmt.Sprintf("give %d 10", crystalC),
		"heal",
	)
	until(t, ctx, f.name+" knowing the summon and holding the crystals", func() string {
		var known bool
		var crystals int32
		for _, s := range f.watch.Seen(0) {
			switch e := s.Event.(type) {
			case botclient.SkillList:
				known = slices.ContainsFunc(e.Skills, func(k botclient.KnownSkill) bool { return k.ID == skillSummonReanimatedMan })
			default:
				if n, _, ok := stock(e, crystalC); ok {
					crystals = n
				}
			}
		}
		if !known || crystals < 3 {
			return fmt.Sprintf("summon known %v, %d Crystal C", known, crystals)
		}
		return ""
	})

	for i := range corpseKills {
		gmOn(t, ctx, gm, f)
		mob, err := gm.Spawn(ctx, wolf)
		if err != nil {
			t.Fatal(err)
		}
		wolves = append(wolves, mob)
		check := RefusalCheck{
			From: len(f.watch.Sent()),
			Packet: func(s Sent) bool {
				return s.Op == botclient.OpRequestMagicSkillUse && s.Target == skillSummonReanimatedMan
			},
			Messages: []int32{msgCorpseTooOld, msgSummonStands, msgCorpseInvalid, msgCorpseCannotUse},
		}
		if err := gm.Select(ctx, mob); err != nil {
			t.Fatal(err)
		}
		if err := gm.Command(ctx, "kill"); err != nil {
			t.Fatal(err)
		}
		dctx, cancel := context.WithTimeout(ctx, time.Minute)
		_, _, err = f.watch.Await(dctx, 0, func(ev botclient.Event) bool {
			d, ok := ev.(botclient.Die)
			return ok && d.ObjectID == mob
		})
		cancel()
		if err != nil {
			t.Fatalf("kill %d: %s sees wolf %d die: %v", i, f.name, mob, err)
		}
		gm.Forget(mob)
		cast := false
		untilServedBy(t, ctx, fmt.Sprintf("%s summoning from wolf %d's corpse", f.name, mob), time.Now().Add(30*time.Second), f.watch, check, func() string {
			for _, p := range f.watch.Sent()[check.From:] {
				cast = cast || check.Packet(p)
			}
			for _, s := range f.watch.Seen(0) {
				if e, ok := s.Event.(botclient.PetInfo); ok && e.SummonType == botclient.SummonServitor && servitor == 0 {
					servitor = e.ObjectID
					if e.TemplateID != reanimatedManNPC {
						t.Errorf("summoned npc %d, want the Reanimated Man %d", e.TemplateID, reanimatedManNPC)
					}
				}
			}
			switch {
			case servitor != 0:
				return ""
			case !cast && wolfGone(f, mob):
				// The corpse decayed with no cast at it: the next wolf.
				return ""
			case !cast:
				return "no summon cast yet"
			}
			return "summon cast, no servitor yet"
		})
		if servitor != 0 {
			t.Logf("%s summoned the Reanimated Man %d from wolf %d's corpse (kill %d)", f.name, servitor, mob, i+1)
			return
		}
		t.Logf("kill %d: %s let wolf %d's corpse pass", i, f.name, mob)
	}
	t.Fatalf("%s cast no corpse summon on %d wolves' corpses next to it", f.name, corpseKills)
}

// wolfGone reports whether f saw wolf mob's DeleteObject (its corpse
// decayed).
func wolfGone(f *farmer, mob int32) bool {
	for _, s := range f.watch.Seen(0) {
		if d, ok := s.Event.(botclient.DeleteObject); ok && d.ObjectID == mob {
			return true
		}
	}
	return false
}

// corpseTap keeps the frames the corpse replay reads: the character's
// UserInfo, the wolves' NPCInfo, Die and DeleteObject, the casts and the
// servitor's frames.
type corpseTap struct {
	mu     sync.Mutex
	frames [][]byte // guarded by mu
}

// corpseOps are the opcodes corpseTap keeps.
var corpseOps = map[byte]bool{
	botwire.OpGameUserInfo: true, botwire.OpGameNPCInfo: true, botwire.OpGameDie: true,
	botwire.OpGameDeleteObject: true, botwire.OpGameMagicSkillUse: true,
	botwire.OpGamePetStatusShow: true, botwire.OpGamePetInfo: true, botwire.OpGamePetStatusUpdate: true,
	botwire.OpGameSetSummonRemainTime: true,
}

func (c *corpseTap) Record(payload []byte) {
	if len(payload) == 0 || !corpseOps[payload[0]] {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, slices.Clone(payload))
}

func (*corpseTap) RecordSent([]byte) {}

// write writes the character's first UserInfo and then every frame about
// ids (the servitor and the wolves) and the character's casts into dir as
// NNN-OP.bin, and returns how many.
func (c *corpseTap) write(dir string, self int32, ids []int32) (int, error) {
	c.mu.Lock()
	frames := slices.Clone(c.frames)
	c.mu.Unlock()
	about := func(id int32) bool { return id != 0 && slices.Contains(ids, id) }
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	n := 0
	for _, raw := range frames {
		ev, ok, err := botwire.DecodeEvent(raw)
		if err != nil || !ok {
			return n, fmt.Errorf("frame %#02x: decoded %v: %w", raw[0], ok, err)
		}
		keep := false
		switch e := ev.(type) {
		case botclient.UserInfo:
			keep = n == 0 && e.ObjectID == self
		case botclient.NPCInfo:
			keep = n > 0 && about(e.ObjectID)
		case botclient.Die:
			keep = n > 0 && about(e.ObjectID)
		case botclient.DeleteObject:
			keep = n > 0 && about(e.ObjectID)
		case botclient.MagicSkillUse:
			keep = n > 0 && e.CasterID == self
		case botclient.PetStatusShow, botclient.SetSummonRemainTime:
			keep = n > 0
		case botclient.PetInfo:
			keep = n > 0 && about(e.ObjectID)
		case botclient.PetStatusUpdate:
			keep = n > 0 && about(e.ObjectID)
		}
		if !keep {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d-%02x.bin", n, raw[0])), raw, 0o600); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// teeTap hands every frame to w, then to also.
type teeTap struct {
	w    *Watch
	also botclient.FrameTap
}

func (t teeTap) Record(payload []byte) {
	t.w.Record(payload)
	t.also.Record(payload)
}

func (t teeTap) RecordSent(payload []byte) {
	t.w.RecordSent(payload)
	t.also.RecordSent(payload)
}
