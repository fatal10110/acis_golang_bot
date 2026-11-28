package botclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
)

// TestServitorRecordedReplay replays a real summon recorded on the live
// server (e2e TestLiveServitorSummon, testdata/servitor_summon): a run
// character's first UserInfo, then every frame about its Kat the Cat
// (npc 14115) and the orc it was ordered onto, in arrival order, until it
// was unsummoned. Each frame decodes whole into the world built from the
// UserInfo; the servitor appears with the PetInfo, holds the orc as its
// target while it fights, drops it when the orc dies (and when it walks
// back to the owner, a walk this recording lacks: fatal10110/acis_golang#5589,
// TestWorldServitor covers it), and is gone after the PetDelete.
func TestServitorRecordedReplay(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "servitor_summon", "*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("%d recorded frames: the fixture is missing", len(paths))
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

	var (
		summon, mob             int32
		fought, mobDied, follow bool
	)
	for i, ev := range frames[1:] {
		w.Apply(ev, at(time.Duration(i+1)*100*time.Millisecond))
		s := w.Servitor
		switch e := ev.(type) {
		case PetInfo:
			if s == nil || s.ObjectID != e.ObjectID || s.TemplateID != 14115 || s.MaxHP <= 0 || s.PAtk <= 0 || s.Level <= 0 {
				t.Fatalf("frame %d PetInfo: servitor %+v", i+1, s)
			}
			summon = s.ObjectID
		case SetSummonRemainTime:
			if s == nil || s.Lifetime != 20*time.Minute || s.Remaining <= 0 || s.Remaining > s.Lifetime {
				t.Errorf("frame %d SetSummonRemainTime: servitor %+v", i+1, s)
			}
		case Attack:
			if e.AttackerID == summon && len(e.Hits) > 0 {
				mob, fought = e.Hits[0].TargetID, true
				if s.Target != mob {
					t.Errorf("frame %d: the servitor swung at %d, target %d", i+1, mob, s.Target)
				}
			}
		case Die:
			if mob != 0 && e.ObjectID == mob {
				mobDied = true
				if s.Target != 0 {
					t.Errorf("frame %d: the orc died, servitor target %d", i+1, s.Target)
				}
			}
		case MoveToPawn:
			if summon != 0 && e.ObjectID == summon && e.TargetID == ui.ObjectID {
				follow = true
				if s.Target != 0 {
					t.Errorf("frame %d: the servitor walks back to its owner, target %d", i+1, s.Target)
				}
			}
		case PetDelete:
			if s != nil {
				t.Errorf("frame %d PetDelete: servitor %+v kept", i+1, s)
			}
		}
	}
	if summon == 0 || !fought || !mobDied {
		t.Errorf("recording: summon %d, fought %v, orc died %v", summon, fought, mobDied)
	}
	if !follow {
		t.Log("no follow walk back to the owner in the recording")
	}
	if w.Servitor != nil {
		t.Errorf("after the recording: servitor %+v, want none (unsummoned)", w.Servitor)
	}
}
