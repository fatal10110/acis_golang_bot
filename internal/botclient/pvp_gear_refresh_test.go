package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// TestPvPGearRefresh checks when the gear model's PvP side is made again
// (gearPvP): an own event after a player seen again unchanged keeps it, a
// new player, one playing another class, or one aged out of the mix table
// makes it again with the new rows.
func TestPvPGearRefresh(t *testing.T) {
	wind := modelSkills[datapack.SkillRef{ID: skillWindStrike, Level: 1}]
	wind.Magic = true
	b := newModelBrain(t, 0, 20, RoleDamage, nil, nil)
	b.persona.PvP.GearWeight = 0.5
	b.pvp.rows, b.pvp.rowsLevel = charmodel.RowTable{{ID: "HM"}: {{Skill: wind}}}, 20
	foe := func(id, class int32) CharInfo {
		return CharInfo{ObjectID: id, Name: "Foe", ClassID: class, Pos: Point{Y: 900}}
	}
	steps := []struct {
		name   string
		ev     Event
		after  time.Duration
		remade bool
		rows   int // players in the PvP side's rows
	}{
		{name: "a mystic seen", ev: foe(0x1000, classHumanMystic), remade: true, rows: 1},
		{name: "own HP only", ev: StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: 7}, after: time.Minute, rows: 1},
		{name: "the mystic seen again", ev: foe(0x1000, classHumanMystic), after: 2 * time.Minute, rows: 1},
		{name: "the mystic now a fighter", ev: foe(0x1000, 0), after: 3 * time.Minute, remade: true, rows: 1},
		{name: "a second fighter", ev: foe(0x1001, 0), after: 4 * time.Minute, remade: true, rows: 2},
		{name: "both aged out: the tier's rows", ev: DeleteObject{ObjectID: 0x7fff}, after: 65 * time.Minute, remade: true, rows: len(charmodel.RowClasses(0))},
	}
	var last *gearPvP
	for i, s := range steps {
		now := t0.Add(s.after)
		b.handleOne(t, s.ev, now)
		// An own event bumps the world's generation, so the valuer checks
		// its PvP side again.
		b.handleOne(t, StatusUpdate{ObjectID: selfID, Set: StatusHP, HP: int32(10 + i)}, now)
		v := b.valuer()
		if v == nil || v.pvp == nil {
			t.Fatalf("%s: no PvP side", s.name)
		}
		if remade := v.pvp != last; remade != s.remade {
			t.Errorf("%s: remade %v, want %v", s.name, remade, s.remade)
		}
		n := 0
		for _, c := range v.pvp.rows {
			n += c
		}
		if n != s.rows {
			t.Errorf("%s: rows %v, want %d players", s.name, v.pvp.rows, s.rows)
		}
		last = v.pvp
	}
}
