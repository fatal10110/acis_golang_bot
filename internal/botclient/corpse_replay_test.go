package botclient

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestCorpseRecordedReplay replays a real corpse summon recorded on the
// live server (e2e TestLiveCorpseSummon, testdata/corpse_summon): a run
// character's first UserInfo, then every frame about the wolf the GM
// spawned at it and killed, the character's cast and its servitor, in
// arrival order. Fed to a brain whose persona casts Summon Reanimated Man
// on corpses, the wolf's Die opens its corpse; the corpse rule selects it
// and sends the cast request of 1129; the bot's MagicSkillUse at it drops
// the corpse (the skill consumes it), the PetInfo brings the Reanimated Man
// (npc 14001) out, so the rule casts no second summon, and the corpse's
// DeleteObject follows.
func TestCorpseRecordedReplay(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "corpse_summon", "*.bin"))
	if err != nil || len(paths) < 2 {
		t.Fatalf("recorded frames: %d, %v", len(paths), err)
	}
	frames := make([]Event, len(paths))
	for i, path := range paths { // Glob sorts: NNN-OP.bin in arrival order
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ev, ok, err := botwire.DecodeEvent(raw)
		if err != nil || !ok {
			t.Fatalf("%s: DecodeEvent = %v, %v", filepath.Base(path), ok, err)
		}
		frames[i] = ev
	}
	ui, ok := frames[0].(UserInfo)
	if !ok {
		t.Fatalf("first frame is %T, want the character's UserInfo", frames[0])
	}
	w, _ := NewWorld(Self{ObjectID: ui.ObjectID, Name: ui.Name}, t0)
	w.Apply(ui, t0)
	p := DefaultPersona()
	p.IdleChance = 0
	p.CorpseSummons = []int32{skillSummonReanimatedMan}
	b := NewBrain(w, NewEvents(64), &recordSender{}, newFakeClock(t0), BrainConfig{Persona: p, Rand: rand.New(rand.NewPCG(1, 2))})
	b.start(t0)
	b.cfg.Travel = corpseMap(0)
	b.world.Apply(SkillList{Skills: []KnownSkill{{ID: skillSummonReanimatedMan, Level: 1}}}, t0)
	carrying(b, reagentStack(crystalC, 10))

	var wolf int32
	now := t0
	i := 1
	for ; i < len(frames); i++ {
		now = at(time.Duration(i) * 100 * time.Millisecond)
		mustHandle(t, b, now, frames[i])
		if d, ok := frames[i].(Die); ok {
			wolf = d.ObjectID
			break
		}
	}
	if wolf == 0 {
		t.Fatal("no Die in the recording")
	}
	if _, ok := b.mobCorpses[wolf]; !ok {
		t.Fatalf("wolf %d's corpse not tracked after its Die", wolf)
	}
	name, a := b.decide(now)
	if name != "corpse" || a.kind != actSelect || a.object != wolf {
		t.Fatalf("after the Die: rule %q kind %d object %d, want the corpse rule selecting wolf %d", name, a.kind, a.object, wolf)
	}
	mustSend(t, b, a, now)
	b.world.Target = wolf
	name, a = b.decide(now)
	if name != "corpse" {
		t.Fatalf("rule %q, want corpse", name)
	}
	wantCast(t, a, skillSummonReanimatedMan, wolf)
	wantFrame(t, a, skillSummonReanimatedMan)
	mustSend(t, b, a, now)

	var cast, summoned, deleted bool
	for i++; i < len(frames); i++ {
		now = now.Add(100 * time.Millisecond)
		mustHandle(t, b, now, frames[i])
		switch e := frames[i].(type) {
		case MagicSkillUse:
			if e.CasterID != ui.ObjectID || e.SkillID != skillSummonReanimatedMan || e.TargetID != wolf {
				t.Fatalf("frame %d: MagicSkillUse %+v, want the bot's 1129 at wolf %d", i, e, wolf)
			}
			cast = true
			if _, ok := b.mobCorpses[wolf]; ok {
				t.Fatalf("frame %d: the corpse is still tracked after the bot's cast at it", i)
			}
		case PetInfo:
			if s := b.world.Servitor; s == nil || s.TemplateID != 14001 {
				t.Fatalf("frame %d: servitor %+v, want the Reanimated Man (14001)", i, s)
			}
			summoned = true
		case DeleteObject:
			deleted = deleted || e.ObjectID == wolf
		}
	}
	if !cast || !summoned || !deleted {
		t.Fatalf("cast %v, summoned %v, corpse deleted %v: want all", cast, summoned, deleted)
	}
	if s, id := corpseCastOf(b, now); s != 0 {
		t.Fatalf("with the servitor out: corpse cast %d on %d, want none", s, id)
	}
}
