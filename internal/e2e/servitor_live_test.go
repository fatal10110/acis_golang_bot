package e2e

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// Summon Kat the Cat (datapack skills/1100-1199.xml): level 5 calls npc
// 14115 for 5 Crystal: Grade D (1458) and 70 MP, with a 15 s cast and a
// 20 s reuse (levels 1 to 4 reuse in 6 h, too long for a run character
// that summons again on the next run).
const (
	skillSummonKat     int32 = 1111
	summonKatLevel           = 5
	summonKatNPC       int32 = 14115
	summonerLevel            = 40
	actionServitorAtk  int32 = 22 // network/summon.go: servitor attack
	actionUnsummonServ int32 = 52 // network/summon.go: servitor unsummon
)

// summonCast bounds the cast (15 s) and the PetInfo after it.
const summonCast = 40 * time.Second

// TestLiveServitorSummon records a real summon for the servitor world
// model's replay (#757): a run character given Summon Kat the Cat and
// crystals by the GM summons, orders the servitor onto a spawned orc
// (RequestActionUse 22), lets it kill the orc and come back, then
// unsummons it (RequestActionUse 52). Every frame about the servitor or
// the orc, after the character's first UserInfo, goes in order into the
// servitor directory of BOT_E2E_RECORD when set, else a temporary one, as
// NNN-OP.bin: those are botclient's testdata/servitor_summon fixtures.
// The orders are the scenario's own, sent as a player's client sends them,
// paced like the brain's; the bot sends none of them (#758 adds them).
func TestLiveServitorSummon(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 6*time.Minute)
	gm := enterGM(t, ctx, tg)
	acc, _ := tg.Account("sv", 1)
	tap := &summonTap{}
	c := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanMystic), tap)
	self := c.Self.ObjectID
	recallClient(t, ctx, gm, c)
	onBot(t, ctx, gm, self,
		"set level "+strconv.Itoa(summonerLevel),
		fmt.Sprintf("skill set %d %d", skillSummonKat, summonKatLevel),
		fmt.Sprintf("give %d 20", crystalD),
		"heal",
	)

	var (
		summon, mob int32
		ordered     bool // the attack order went out
		mobDied     bool
		followed    bool
		deleted     bool
	)
	track := func(ev botclient.Event) {
		switch e := ev.(type) {
		case botclient.PetInfo:
			if e.SummonType == botclient.SummonServitor && summon == 0 {
				summon = e.ObjectID
				if e.TemplateID != summonKatNPC {
					t.Errorf("summoned npc %d, want Kat the Cat %d", e.TemplateID, summonKatNPC)
				}
			}
		case botclient.Die:
			mobDied = mobDied || (mob != 0 && e.ObjectID == mob)
		case botclient.MoveToPawn:
			followed = followed || (ordered && summon != 0 && e.ObjectID == summon && e.TargetID == self)
		case botclient.PetDelete:
			deleted = deleted || (summon != 0 && e.ObjectID == summon)
		}
	}
	// What was recorded is written however the scenario ends: a failed run
	// leaves its frames to look at.
	defer func() {
		dir := filepath.Join(cmp.Or(os.Getenv("BOT_E2E_RECORD"), t.TempDir()), "servitor")
		n, err := tap.write(dir, self, summon, mob)
		if err != nil {
			t.Error(err)
		}
		t.Logf("servitor %d, orc %d: %d frames recorded in %s", summon, mob, n, dir)
	}()
	if err := c.Send(ctx, botwire.EncodeRequestMagicSkillUse(skillSummonKat, false)); err != nil {
		t.Fatal(err)
	}
	watchUntilBy(t, ctx, c, "the servitor summoned", time.Now().Add(summonCast),
		func() bool { return summon != 0 }, track)

	spawned, err := gm.Spawn(ctx, vukuOrc)
	if err != nil {
		t.Fatal(err)
	}
	mob = spawned
	t.Cleanup(func() {
		if mobDied || !slices.Contains(gm.Spawned(), mob) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := gm.Delete(ctx, mob); err != nil {
			t.Logf("cleanup: delete the orc %d: %v", mob, err)
		}
	})
	if err := c.Send(ctx, botwire.EncodeAction(mob, c.Pos())); err != nil {
		t.Fatal(err)
	}
	watchUntil(t, ctx, c, "the orc selected", func() bool { return c.Target() == mob }, track)
	if err := c.Send(ctx, botwire.EncodeRequestActionUse(actionServitorAtk)); err != nil {
		t.Fatal(err)
	}
	ordered = true
	watchUntilBy(t, ctx, c, "the servitor killing the orc", time.Now().Add(2*time.Minute), func() bool { return mobDied }, track)
	// The server refuses an unsummon while the servitor is in combat
	// (summon/command.go resolveUnsummonServitor): its stance outlasts the
	// fight by about 15 s. The order goes out again every unsummonEvery,
	// one frame each, until the PetDelete. The wait also takes the
	// servitor's follow walk back to its owner (ai/summon.go
	// FinishedAttack), which a servitor standing next to it may skip.
	next := time.Now()
	watchUntilBy(t, ctx, c, "the servitor unsummoned", time.Now().Add(unsummonBound), func() bool {
		if !deleted && time.Now().After(next) {
			if err := c.Send(ctx, botwire.EncodeRequestActionUse(actionUnsummonServ)); err != nil {
				t.Fatal(err)
			}
			next = time.Now().Add(unsummonEvery)
		}
		return deleted
	}, track)
	t.Logf("follow walk back to the owner seen: %v", followed)
}

// unsummonEvery spaces the unsummon orders; unsummonBound bounds them
// (the combat stance and a margin).
const (
	unsummonEvery = 5 * time.Second
	unsummonBound = 90 * time.Second
)

// summonTap keeps, in arrival order, every server frame the servitor's
// world model reads (botclient World.applyServitor and its hooks), for
// write to sort out once the ids are known. The connection's reader feeds
// it; write runs after the scenario's last read.
type summonTap struct {
	mu     sync.Mutex
	frames [][]byte // guarded by mu
}

// summonOps are the opcodes the servitor's world model reads, and the
// character's UserInfo, which the replay builds its world from.
var summonOps = map[byte]bool{
	botwire.OpGameUserInfo: true, botwire.OpGameAttack: true, botwire.OpGameDie: true,
	botwire.OpGameRevive: true, botwire.OpGameMoveToPawn: true, botwire.OpGameDeleteObject: true,
	botwire.OpGameAutoAttackStart: true, botwire.OpGameAutoAttackStop: true,
	botwire.OpGamePetStatusShow: true, botwire.OpGamePetInfo: true, botwire.OpGamePetStatusUpdate: true,
	botwire.OpGamePetDelete: true, botwire.OpGameSetSummonRemainTime: true,
}

func (s *summonTap) Record(payload []byte) {
	if len(payload) == 0 || !summonOps[payload[0]] {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames = append(s.frames, slices.Clone(payload))
}

func (*summonTap) RecordSent([]byte) {}

// write writes the character's first UserInfo and then every frame about
// the servitor or the orc into dir as NNN-OP.bin, and returns how many.
func (s *summonTap) write(dir string, self, summon, mob int32) (int, error) {
	s.mu.Lock()
	frames := slices.Clone(s.frames)
	s.mu.Unlock()
	about := func(ids ...int32) bool {
		return slices.ContainsFunc(ids, func(id int32) bool { return id != 0 && (id == summon || id == mob) })
	}
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
		case botclient.PetStatusShow, botclient.SetSummonRemainTime:
			keep = n > 0
		case botclient.PetInfo:
			keep = n > 0 && about(e.ObjectID)
		case botclient.PetStatusUpdate:
			keep = n > 0 && about(e.ObjectID)
		case botclient.PetDelete:
			keep = n > 0 && about(e.ObjectID)
		case botclient.Attack:
			ids := []int32{e.AttackerID}
			for _, h := range e.Hits {
				ids = append(ids, h.TargetID)
			}
			keep = n > 0 && about(ids...)
		case botclient.MoveToPawn:
			keep = n > 0 && about(e.ObjectID, e.TargetID)
		case botclient.Die:
			keep = n > 0 && about(e.ObjectID)
		case botclient.Revive:
			keep = n > 0 && about(e.ObjectID)
		case botclient.DeleteObject:
			keep = n > 0 && about(e.ObjectID)
		case botclient.AutoAttackStart:
			keep = n > 0 && about(e.ObjectID)
		case botclient.AutoAttackStop:
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
