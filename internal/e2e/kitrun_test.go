package e2e

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

func TestKitFrom(t *testing.T) {
	k := botclient.ClassKit{
		Skills: []int32{3, 16, 226},
		Rotation: []botclient.Step{
			{Skill: 58},
			{Skill: 16, Weapons: []string{"DAGGER"}},
			{Skill: 3, Weapons: []string{"SWORD"}},
		},
		Buffs:   []botclient.Buff{{Skill: 91}},
		Toggles: []botclient.Toggle{{Skill: 1001}},
		Relax:   226,
		Sweep:   42,
	}
	got := kitFrom(k)
	want := kitSkills{all: []int32{3, 16, 226, 58, 91, 1001, 42}, rotation: []int32{58, 16, 3}, gated: []int32{16, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kitFrom = %+v, want %+v", got, want)
	}
	if g := got.grants(); g[0] != "skill set 3 1" || len(g) != len(want.all) {
		t.Errorf("grants = %v", g)
	}
}

func TestKitRunOf(t *testing.T) {
	const self, other, mob1, mob2 = int32(1), int32(2), int32(10), int32(11)
	kit := kitSkills{rotation: []int32{3, 16}, gated: []int32{3, 16}}
	cast := func(caster, skill, target int32) Seen {
		return Seen{Event: botclient.MagicSkillUse{CasterID: caster, SkillID: skill, TargetID: target}}
	}
	swing := func(attacker, target int32) Seen {
		return Seen{Event: botclient.Attack{AttackerID: attacker, Hits: []botclient.AttackHit{{TargetID: target}}}}
	}
	die := func(id int32) Seen { return Seen{Event: botclient.Die{ObjectID: id}} }
	msg := func(id int32) Seen { return Seen{Event: botclient.SystemMessage{ID: id}} }
	tests := []struct {
		name    string
		seen    []Seen
		casts   map[int32]int
		kills   int
		refused []int32
	}{
		{name: "none", casts: map[int32]int{}},
		{
			name:  "a swing and a cast, each kill once",
			seen:  []Seen{swing(self, mob1), cast(self, 3, mob2), die(mob1), die(mob2), die(mob2)},
			casts: map[int32]int{3: 1}, kills: 2,
		},
		{
			name:  "another's fights and a self cast",
			seen:  []Seen{swing(other, mob1), cast(other, 3, mob2), cast(self, 58, self), die(mob1), die(mob2), die(self)},
			casts: map[int32]int{58: 1},
		},
		{
			name:  "a refused skill",
			seen:  []Seen{msg(612), msg(msgS1CannotBeUsed)},
			casts: map[int32]int{}, refused: []int32{msgS1CannotBeUsed},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := kitRunOf(tt.seen, self, kit)
			if !reflect.DeepEqual(r.casts, tt.casts) || r.kills != tt.kills || !slices.Equal(r.refused, tt.refused) {
				t.Errorf("kitRunOf = casts %v kills %d refused %v, want %v %d %v", r.casts, r.kills, r.refused, tt.casts, tt.kills, tt.refused)
			}
			if (r.refusals() == "") != (len(tt.refused) == 0) {
				t.Errorf("refusals = %q", r.refusals())
			}
		})
	}
}

func TestKitRunShort(t *testing.T) {
	gated := kitSkills{rotation: []int32{3, 56}, gated: []int32{3}}
	tests := []struct {
		name    string
		r       kitRun
		elapsed time.Duration
		want    []string // what the answer names; none for ""
	}{
		{name: "done", r: kitRun{kit: gated, kills: kitKills, casts: map[int32]int{3: kitCasts}}, elapsed: kitFarm},
		{name: "too soon", r: kitRun{kit: gated, kills: kitKills, casts: map[int32]int{3: kitCasts}}, elapsed: time.Minute, want: []string{"farmed"}},
		{name: "few kills", r: kitRun{kit: gated, casts: map[int32]int{3: kitCasts}}, elapsed: kitFarm, want: []string{"kills"}},
		{name: "ungated casts only", r: kitRun{kit: gated, kills: kitKills, casts: map[int32]int{56: kitCasts}}, elapsed: kitFarm, want: []string{"weapon-gated"}},
		{name: "no rotation melees", r: kitRun{kills: kitKills, casts: map[int32]int{}}, elapsed: kitFarm},
		{name: "few casts", r: kitRun{kit: gated, kills: kitKills, casts: map[int32]int{3: 1}}, elapsed: kitFarm, want: []string{"casts"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.short(tt.elapsed)
			if (got == "") != (len(tt.want) == 0) {
				t.Fatalf("short = %q, want naming %v", got, tt.want)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("short = %q, want it to name %q", got, w)
				}
			}
		})
	}
}

func TestSpoilRunOf(t *testing.T) {
	const self, other, mob = int32(1), int32(2), int32(10)
	cast := func(caster, skill int32) Seen {
		return Seen{Event: botclient.MagicSkillUse{CasterID: caster, SkillID: skill, TargetID: mob}}
	}
	msg := func(id int32) Seen { return Seen{Event: botclient.SystemMessage{ID: id}} }
	die := func(id int32, sweepable bool) Seen {
		return Seen{Event: botclient.Die{ObjectID: id, Sweepable: sweepable}}
	}
	tests := []struct {
		name                                           string
		seen                                           []Seen
		spoils, spoiled, died, sweepable, sweeps, paid int
		refused                                        []int32
		done                                           bool
	}{
		{name: "none"},
		{
			name:   "a spoil, a kill and a sweep that pays twice",
			seen:   []Seen{cast(self, skillSpoil), msg(msgSpoilSuccess), die(mob, true), die(mob, true), cast(self, skillSweeper), msg(msgEarnedItems), msg(msgEarnedItem)},
			spoils: 1, spoiled: 1, died: 1, sweepable: 1, sweeps: 1, paid: 1, done: true,
		},
		{
			name:   "earned items after another cast are no sweep's",
			seen:   []Seen{cast(self, skillSpoil), msg(msgSpoilSuccess), die(other, true), die(mob, false), cast(self, skillSweeper), cast(self, 3), msg(msgEarnedItem)},
			spoils: 1, spoiled: 1, died: 1, sweeps: 1,
		},
		{
			name:   "another's casts do not end a sweep",
			seen:   []Seen{cast(self, skillSweeper), cast(other, skillSpoil), msg(msgEarnedItem)},
			sweeps: 1, paid: 1,
		},
		{
			name:    "refused sweeps and skills",
			seen:    []Seen{msg(msgSweepNotSpoiled), msg(msgSweepNotAllowed), msg(msgS1CannotBeUsed)},
			refused: []int32{msgSweepNotSpoiled, msgSweepNotAllowed, msgS1CannotBeUsed},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := spoilRunOf(tt.seen, self)
			got := [6]int{r.spoils, r.spoiled, r.died, r.sweepable, r.sweeps, r.paid}
			if want := [6]int{tt.spoils, tt.spoiled, tt.died, tt.sweepable, tt.sweeps, tt.paid}; got != want || !slices.Equal(r.refused, tt.refused) {
				t.Errorf("spoilRunOf = %v refused %v, want %v refused %v", got, r.refused, want, tt.refused)
			}
			if (r.short() == "") != tt.done {
				t.Errorf("short = %q, want done %v", r.short(), tt.done)
			}
		})
	}
}

func TestSweepTrace(t *testing.T) {
	const self, mob = int32(1), int32(10)
	at := time.Date(2026, 11, 25, 1, 0, 0, 0, time.UTC)
	seen := []Seen{
		{At: at, Event: botclient.MagicSkillUse{CasterID: self, SkillID: skillSpoil, TargetID: mob}},
		{At: at, Event: botclient.SystemMessage{ID: msgSpoilSuccess}},
		{At: at.Add(time.Second), Event: botclient.Die{ObjectID: 11, Sweepable: true}},
		{At: at.Add(2 * time.Second), Event: botclient.Die{ObjectID: mob, Sweepable: true}},
		{At: at.Add(3 * time.Second), Event: botclient.MyTargetSelected{ObjectID: mob}},
		{At: at.Add(20 * time.Second), Event: botclient.DeleteObject{ObjectID: mob}},
	}
	sent := []Sent{{At: at, Op: 0x2f, Target: skillSpoil}, {At: at.Add(4 * time.Second), Op: 0x04, Target: mob}}
	got := sweepTrace(seen, sent, self)
	if len(got) != 3 || !strings.Contains(got[0], "corpse 10") || !strings.Contains(got[1], "MyTargetSelected") || !strings.Contains(got[2], "sent 0x04") {
		t.Errorf("sweepTrace = %q", got)
	}
}
