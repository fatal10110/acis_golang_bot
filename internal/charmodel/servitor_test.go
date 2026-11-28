package charmodel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// kai is Kai the Cat's template (npcs/14000-14999.xml 14207), the servitor
// of Summon Kai the Cat level 1.
var kai = datapack.NPC{ID: 14207, Type: "Servitor", Level: 42, Combat: datapack.Combat{
	HP: 1050.81652, PAtk: 198.34591, PDef: 164.88513, MAtk: 135.44454, MDef: 120.65578, AtkSpd: 253, Crit: 4,
	STR: 40, INT: 21, DEX: 30, WIT: 20, CON: 43, MEN: 25,
}}

// TestServitorProfile checks a servitor's profile: its template's final
// stats as a monster's (Monster), with each live number PetInfo reported in
// place of the template's and the rest the template's.
func TestServitorProfile(t *testing.T) {
	tmpl := Monster(kai)
	live := tmpl
	live.MaxHP, live.PAtk, live.PDef, live.PAtkSpd = 1200, 400, 300, 380
	partial := tmpl
	partial.PAtk = 400
	tests := []struct {
		name string
		live ServitorLive
		want Profile
	}{
		{"template only", ServitorLive{}, tmpl},
		{"live numbers", ServitorLive{MaxHP: 1200, PAtk: 400, PDef: 300, PAtkSpd: 380}, live},
		{"P.Atk reported alone", ServitorLive{PAtk: 400}, partial},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Servitor(kai, tt.live); got != tt.want {
				t.Fatalf("Servitor = %+v, want %+v", got, tt.want)
			}
		})
	}
	if tmpl.PAtk <= 0 || tmpl.PAtkSpd <= 0 || tmpl.Accuracy <= 0 {
		t.Fatalf("Kai's template profile %+v has no attack", tmpl)
	}
}

// TestTimeWithServitor checks the combined kill time of a bot and its
// servitor against fixed profiles: 1/(1/T_bot + 1/T_servitor), the
// servitor alone when the bot never kills, the bot alone with no servitor
// damage.
func TestTimeWithServitor(t *testing.T) {
	mob := Profile{MaxHP: 2000, PDef: 150, Evasion: 40}
	bot := Profile{MaxHP: 1500, PAtk: 300, PAtkSpd: 330, Accuracy: 80}
	cat := Servitor(kai, ServitorLive{})
	idle := cat
	idle.PAtk = 0
	tb := TimeToKill(bot, Attack{}, mob)
	ts := TimeToKill(cat, Attack{}, mob)
	if tb <= 0 || tb == never || ts <= 0 || ts == never {
		t.Fatalf("fixture times: bot %v, servitor %v", tb, ts)
	}
	both := time.Duration(float64(tb) * float64(ts) / float64(tb+ts))
	tests := []struct {
		name string
		t    time.Duration
		s    Profile
		want time.Duration
	}{
		{"bot alone", tb, idle, tb},
		{"servitor alone", never, cat, ts},
		{"both", tb, cat, both},
		{"neither", never, idle, never},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TimeWithServitor(tt.t, tt.s, mob); (got - tt.want).Abs() > time.Microsecond {
				t.Fatalf("TimeWithServitor = %v, want %v", got, tt.want)
			}
		})
	}
	if got := TimeWithServitor(tb, cat, mob); got >= min(tb, ts) {
		t.Errorf("together %v is not faster than either alone (bot %v, servitor %v)", got, tb, ts)
	}
}
