package botwire

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRecordedUserInfoStats: the recorded UserInfo capture decodes to
// plausible base and combat stats. The values vary between characters and
// runs, so only their ranges are checked: a field read from the wrong offset
// lands far outside them.
func TestRecordedUserInfoStats(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "recorded", "04.bin"))
	if err != nil {
		t.Fatal(err)
	}
	ev, ok, err := DecodeEvent(raw)
	if err != nil || !ok {
		t.Fatalf("decode: ok = %v, err = %v", ok, err)
	}
	u, isUI := ev.(UserInfo)
	if !isUI {
		t.Fatalf("event = %T, want UserInfo", ev)
	}
	for name, v := range map[string]int32{"STR": u.STR, "DEX": u.DEX, "CON": u.CON, "INT": u.INT, "WIT": u.WIT, "MEN": u.MEN} {
		if v < 1 || v > 100 {
			t.Errorf("%s = %d, want 1..100", name, v)
		}
	}
	for name, v := range map[string]int32{
		"PAtk": u.PAtk, "PAtkSpd": u.PAtkSpd, "PDef": u.PDef, "Evasion": u.Evasion, "Accuracy": u.Accuracy,
		"Critical": u.Critical, "MAtk": u.MAtk, "CastSpd": u.CastSpd, "MDef": u.MDef,
	} {
		if v < 1 || v > 100000 {
			t.Errorf("%s = %d, want 1..100000", name, v)
		}
	}
}
