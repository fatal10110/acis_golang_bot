package botclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// selfStats is the base and combat stat block Self keeps from UserInfo.
type selfStats struct {
	STR, DEX, CON, INT, WIT, MEN                                          int32
	PAtk, PAtkSpd, PDef, Evasion, Accuracy, Critical, MAtk, CastSpd, MDef int32
}

func statsOf(s SelfState) selfStats {
	return selfStats{
		s.STR, s.DEX, s.CON, s.INT, s.WIT, s.MEN,
		s.PAtk, s.PAtkSpd, s.PDef, s.Evasion, s.Accuracy, s.Critical, s.MAtk, s.CastSpd, s.MDef,
	}
}

func userInfoWithStats(st selfStats) UserInfo {
	return UserInfo{
		ObjectID: selfID, Name: "bot7", RunSpeed: 100, WalkSpeed: 50, Running: true, HP: 10, MaxHP: 20,
		STR: st.STR, DEX: st.DEX, CON: st.CON, INT: st.INT, WIT: st.WIT, MEN: st.MEN,
		PAtk: st.PAtk, PAtkSpd: st.PAtkSpd, PDef: st.PDef, Evasion: st.Evasion, Accuracy: st.Accuracy,
		Critical: st.Critical, MAtk: st.MAtk, CastSpd: st.CastSpd, MDef: st.MDef,
	}
}

// TestWorldUserInfoStatsReplaced: the server resends UserInfo on every stat
// change, so each one replaces the stats Self keeps, a stat dropping to zero
// included.
func TestWorldUserInfoStatsReplaced(t *testing.T) {
	first := selfStats{40, 30, 43, 21, 11, 25, 120, 330, 80, 33, 34, 44, 60, 213, 70}
	cases := []struct {
		name string
		next selfStats
	}{
		{"every stat changes", selfStats{41, 31, 44, 22, 12, 26, 150, 400, 95, 35, 36, 48, 75, 250, 90}},
		{"a buff ends", selfStats{40, 30, 43, 21, 11, 25, 120, 330, 80, 33, 34, 0, 60, 213, 70}},
		{"no change", first},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newTestWorld(t)
			w.Apply(userInfoWithStats(first), t0)
			if got := statsOf(w.Self); got != first {
				t.Fatalf("after the first UserInfo: stats = %+v, want %+v", got, first)
			}
			w.Apply(userInfoWithStats(c.next), at(time.Second))
			if got := statsOf(w.Self); got != c.next {
				t.Fatalf("after the second UserInfo: stats = %+v, want %+v", got, c.next)
			}
		})
	}
}

// TestRecordedUserInfoStats: the recorded UserInfo capture decodes to
// plausible base and combat stats. The values vary between characters and
// runs, so only their ranges are checked: a field read from the wrong offset
// lands far outside them.
func TestRecordedUserInfoStats(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "recorded", "04.bin"))
	if err != nil {
		t.Fatal(err)
	}
	ev, ok, err := decodeEvent(raw)
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
