package botclient

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// cleanFoe puts pcID, a clean player of class, at p, standing.
func cleanFoe(t *testing.T, b *Brain, class int32, p Point) {
	t.Helper()
	b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: class, Pos: p, RunSpeed: 124, CollisionRadius: 9}, t0)
	b.world.object(pcID).move = stopAt(p)
}

// pkVictimFight makes clean pcID the PK victim of b, which attacks it: in
// combat stance, pcID selected, nobody hitting the bot.
func pkVictimFight(t *testing.T, b *Brain, class int32, p Point) {
	t.Helper()
	b.persona.PvP.ForceAttack = true
	cleanFoe(t, b, class, p)
	clear(b.world.Attackers)
	b.world.Self.InCombat, b.world.Target = true, pcID
	b.pkStart(pcID, t0)
	b.reactAt = time.Time{}
}

// TestPvPPKCap checks PK mode (docs/pvp/karma.md "PK mode", MayFight rule
// 7): the victim pick on the hunting preconditions and the D39 limits, the
// forced attack's non-debuff damage on the WA and HM rows, the full order
// once the victim is flagged, and karma's work-off.
func TestPvPPKCap(t *testing.T) {
	around := box(-500, -500, 500, 500)
	pkBrain := func(t *testing.T) *Brain {
		b := huntBrain(t)
		b.persona.PvP.ForceAttack = true
		cleanFoe(t, b, 10, Point{Y: 300})
		return b
	}
	for _, tc := range []struct {
		name  string
		setup func(*Brain)
		pk    bool
	}{
		{name: "0 kills, no PK in the hour", pk: true},
		{name: "4 PK kills", setup: func(b *Brain) { b.world.Self.PKKills = pkCap }},
		{name: "3 PK kills", setup: func(b *Brain) { b.world.Self.PKKills = pkCap - 1 }, pk: true},
		{name: "last PK 30 minutes ago", setup: func(b *Brain) { b.karma.lastPK = at(time.Second).Add(-30 * time.Minute) }},
		{name: "last PK 2 hours ago", setup: func(b *Brain) { b.karma.lastPK = at(time.Second).Add(-2 * time.Hour) }, pk: true},
		{name: "HP under StandHP", setup: func(b *Brain) { b.world.Self.HP = 10 }},
		{name: "the bot in a town polygon", setup: func(b *Brain) { b.cfg.Travel.zones.Towns = []datapack.Zone{around} }},
		{name: "the victim in a town polygon", setup: func(b *Brain) {
			b.cfg.Travel.zones.Towns = []datapack.Zone{box(-100, 200, 100, 400)}
		}},
		{name: "the estimate failing", setup: func(b *Brain) {
			mystic, _ := charmodel.ClassRow(10, false)
			b.pvp.rows = charmodel.RowTable{mystic: {{Skill: datapack.Skill{Type: "MDAM", Magic: true, Power: 1e6, HitTime: time.Second}}}}
			b.pvp.rowsLevel = max(int(b.world.Self.Level), 1)
		}},
		{name: "PK mode off", setup: func(b *Brain) { b.persona.PvP.ForceAttack = false }},
		{name: "karma above 0", setup: func(b *Brain) { b.world.Self.Karma = 240 }},
		{name: "the victim killed the bot within 30 minutes", setup: func(b *Brain) {
			b.karma.avoid = map[string]time.Time{foeName: at(time.Minute)}
		}},
		{name: "a flagged victim is hunting's, not a PK", setup: func(b *Brain) {
			b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, t0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pkBrain(t)
			if tc.setup != nil {
				tc.setup(b)
			}
			now := at(time.Second)
			got := fightsWith(b, "hunt", pcID, now) && b.karma.victim == pcID
			if got != tc.pk {
				t.Fatalf("PK %v, want %v", got, tc.pk)
			}
			if got && !b.karma.lastPK.Equal(now) {
				t.Fatalf("PK clock at %v, want the pick at %v", b.karma.lastPK, now)
			}
		})
	}

	t.Run("the victim alive and in sight after the hour: a new PK, the clock restarted", func(t *testing.T) {
		b := pkBrain(t)
		b.pkStart(pcID, t0)
		later := t0.Add(pkEvery + time.Minute)
		if !fightsWith(b, "hunt", pcID, later) || b.karma.victim != pcID || !b.karma.lastPK.Equal(later) {
			t.Fatalf("victim %d PK clock %v, want %d restarted at %v", b.karma.victim, b.karma.lastPK, pcID, later)
		}
		// The victim gone, another clean player is no PK within the hour.
		b.handleOne(t, DeleteObject{ObjectID: pcID}, later)
		b.handleOne(t, CharInfo{ObjectID: pcID + 1, Name: "Bran", ClassID: 10, Pos: Point{X: 300}, RunSpeed: 124, CollisionRadius: 9}, later)
		b.world.object(pcID + 1).move = stopAt(Point{X: 300})
		b.world.Self.InCombat, b.world.Target = false, 0
		if fightsWith(b, "hunt", pcID+1, later.Add(30*time.Minute)) {
			t.Fatal("a second PK within the hour of the first")
		}
		if !fightsWith(b, "hunt", pcID+1, later.Add(pkEvery+time.Second)) {
			t.Fatal("no PK an hour after the last")
		}
	})
	t.Run("WA: AttackRequest and ctrl-held Power Smash, never Stun Attack", func(t *testing.T) {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		pkVictimFight(t, b, lyrWarrior, Point{X: 90})
		_, a := b.decide(at(time.Second))
		if a.kind != actCast || a.skill != skillPowerSmash || string(a.payload) != string(botwire.EncodeRequestMagicSkillUse(skillPowerSmash, true)) {
			t.Fatalf("decision %v skill %d, want the ctrl-held Power Smash", a.kind, a.skill)
		}
		b.world.holdSkill(skillPowerSmash, at(time.Minute))
		_, a = b.decide(at(time.Second))
		if a.kind != actAttack || a.object != pcID {
			t.Fatalf("decision %v skill %d, want the AttackRequest (Stun Attack is isDebuff)", a.kind, a.skill)
		}
	})
	t.Run("WA: the victim flagged, the full order", func(t *testing.T) {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		pkVictimFight(t, b, lyrWarrior, Point{X: 90})
		b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, t0)
		if got := castOf(t, b, at(time.Second)); got != skillStunAttack {
			t.Fatalf("cast %d, want Stun Attack", got)
		}
	})
	t.Run("HM: ctrl-held Wind Strike, never Ice Bolt or Curse: Poison", func(t *testing.T) {
		b := mysticRowBrain(t, classHumanMystic, 14, lyrWarrior, false)
		pkVictimFight(t, b, lyrWarrior, Point{Y: 300})
		for i := range 3 {
			_, a := b.decide(at(time.Second))
			switch {
			case a.kind == actCast && (a.skill == skillIceBolt || a.skill == skillCursePoison):
				t.Fatalf("cast %d on a clean victim", a.skill)
			case a.kind == actCast && a.skill == skillWindStrike:
				if string(a.payload) != string(botwire.EncodeRequestMagicSkillUse(skillWindStrike, true)) {
					t.Fatal("Wind Strike without ctrl")
				}
				return
			case a.kind == actCast:
				// A self step first: it lands, then the attack.
				b.world.holdSkill(a.skill, at(time.Minute))
			default:
				t.Fatalf("decision %d: %v, want Wind Strike", i, a.kind)
			}
		}
		t.Fatal("no Wind Strike")
	})
	t.Run("HM: the victim flagged, the full order", func(t *testing.T) {
		b := mysticRowBrain(t, classHumanMystic, 14, lyrWarrior, false)
		pkVictimFight(t, b, lyrWarrior, Point{Y: 300})
		b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 1}, t0)
		// The row's control and slow steps come back first (Curse:
		// Weakness, Ice Bolt), none of which goes on a clean player.
		if got := castOf(t, b, at(time.Second)); got == 0 || b.pkSkill(got) {
			t.Fatalf("cast %d, want a step of the full order the PK filter skips", got)
		}
	})
	t.Run("the flag guard is off for the victim", func(t *testing.T) {
		b := pvpRowBrain(t, 1, 36, "BIGBLUNT", lyrWarrior, skillPowerSmash, skillStunAttack)
		pkVictimFight(t, b, lyrWarrior, Point{X: 90})
		b.handleOne(t, RelationChanged{ObjectID: pcID, PvPFlag: 2}, t0)
		if o := b.world.object(pcID); b.blinking(pcID, o, at(time.Second)) {
			t.Fatal("the flag guard holds the PK victim")
		}
		b.karma.victim = 0
		if o := b.world.object(pcID); !b.blinking(pcID, o, at(time.Second)) {
			t.Fatal("the flag guard lets a blinking non-victim through")
		}
	})
	t.Run("karma above 0: a due supply trip waits, farming", func(t *testing.T) {
		for _, karma := range []int32{0, 240} {
			b := newTripBrain(t)
			b.supplyTrip.potions = true
			b.world.Self.Karma = karma
			b.handleOne(t, NPCInfo{ObjectID: mobObj, NPCID: mob, Attackable: true, Pos: Point{X: 100}}, t0)
			rule, a := b.decide(at(time.Minute))
			if karma == 0 {
				if rule != "supply" {
					t.Fatalf("karma 0: rule %q, want the supply trip", rule)
				}
				continue
			}
			if rule != "farm" || a.object != mobObj || !b.spot.held || b.supplyTrip.active() {
				t.Fatalf("karma %d: rule %q action %v on %d, spot held %v, trip %v: want farming at its spot",
					karma, rule, a.kind, a.object, b.spot.held, b.supplyTrip.active())
			}
		}
	})
	t.Run("karma above 0: the town rules held", func(t *testing.T) {
		for _, name := range []string{"supply", "travel", "shop", "warehouse", "train", "arena", "challenge"} {
			if !karmaRule(name) {
				t.Errorf("rule %q runs with karma", name)
			}
		}
		for _, name := range []string{"dead", "attacked", "recover", "rest", "spot", "farm"} {
			if karmaRule(name) {
				t.Errorf("rule %q held with karma", name)
			}
		}
		var held []string
		for _, r := range playRules {
			if r.karma {
				held = append(held, r.name)
			}
		}
		want := []string{
			"challenge", "valuables", "classchange", "train", "gear", "market", "shop", "dyes",
			"warehouse", "member", "recipe", "supply", "arena", "newbie", "signs", "travel",
		}
		if !slices.Equal(held, want) {
			t.Errorf("karma holds %q, want %q", held, want)
		}
		for _, r := range storeRules {
			if r.karma {
				t.Errorf("store rule %q held with karma", r.name)
			}
		}
	})
}
