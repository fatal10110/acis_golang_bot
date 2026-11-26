package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The fighter rows' cases of the layer and guards tests (docs/pvp/layer.md
// "Tests", docs/pvp/guards.md "Tests"; X7): each plays a shipped row's
// real build on the rows' skill fixture (rowSkills).

// TestPvPInterruptHealKN checks the KN case of the heal interrupt
// (docs/pvp/layer.md "Interrupts"): a KN with a shield, of the damage and
// the support role fighting back, keeps Shield Stun against a Cleric,
// casts it before the Cleric's Heal (1011, hit 5000) lands, once, and at
// reuse against a Warrior.
func TestPvPInterruptHealKN(t *testing.T) {
	for _, role := range []Role{RoleDamage, RoleSupport} {
		knight := func(t *testing.T, opp int32) *Brain {
			b := pvpRowBrain(t, classKnight, 28, "SWORD", opp, skillShieldStun, skillPowerStrike)
			wearShield(b)
			b.persona.Role, b.builtOK = role, false
			foeOf(t, b, opp, Point{Y: 60}, 0)
			return b
		}
		t.Run(role.String()+": kept against a Cleric, then the interrupt", func(t *testing.T) {
			b := knight(t, lyrCleric)
			if got := castOf(t, b, at(time.Second)); got == skillShieldStun {
				t.Fatal("Shield Stun cast at reuse against a Cleric")
			}
			healing(t, b)
			if got := castOf(t, b, at(2600*time.Millisecond)); got != skillShieldStun {
				t.Fatalf("cast %d on the Heal, want Shield Stun", got)
			}
			if got := castOf(t, b, at(2700*time.Millisecond)); got == skillShieldStun {
				t.Fatal("a second interrupt on the same Heal")
			}
		})
		t.Run(role.String()+": at reuse against a Warrior", func(t *testing.T) {
			if got := castOf(t, knight(t, lyrWarrior), at(time.Second)); got != skillShieldStun {
				t.Fatalf("cast %d, want Shield Stun at reuse", got)
			}
		})
	}
}

// TestPvPLongCastRuleEF checks the EF case of the long-cast rule
// (docs/pvp/layer.md "PvP order"): under 50 % HP, no Elemental Heal (hit
// 5000) while a melee opponent stands in reach; stunned or out of reach,
// the heal.
func TestPvPLongCastRuleEF(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pos      Point
		abnormal int32
		want     bool
	}{
		{"a Warrior in reach", Point{Y: 100}, 0, false},
		{"the Warrior stunned", Point{Y: 100}, 0x40, true},
		{"the Warrior out of reach", Point{Y: 400}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, int32(ElvenFighter.ID), 20, "SWORD", lyrWarrior, skillElementalHeal, skillPowerStrike)
			b.world.Self.HP = 400
			foeOf(t, b, lyrWarrior, tc.pos, tc.abnormal)
			if got := castOf(t, b, at(time.Second)) == skillElementalHeal; got != tc.want {
				t.Errorf("Elemental Heal %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPvPControlLandRateStunAttack checks the Stun Attack case of the
// control steps' land rate (docs/pvp/layer.md "PvP order"): Stun Attack
// (base 50, resisted by CON) lands on an Orc Fighter (CON 47) at about
// 33 %, under MinLand: skipped; on a Human Mystic (CON 27) at about 50 %:
// cast.
func TestPvPControlLandRateStunAttack(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opp      int32
		min, max float64
		want     bool
	}{
		{"an Orc Fighter", rowOrcFighter, 30, MinLand, false},
		{"a Human Mystic", lyrMystic, 48, 52, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpRowBrain(t, 1, 20, "BLUNT", tc.opp, skillStunAttack)
			withKit(b, ClassKit{Rotation: []Step{{Skill: skillStunAttack, Weapons: stunWeapons}}})
			bot, opp, ok := b.pvpFighters(pcID)
			if !ok {
				t.Fatal("no fighters")
			}
			rate := charmodel.LandRate(rowSkills[datapack.SkillRef{ID: skillStunAttack, Level: 1}], bot, opp)
			if rate < tc.min || rate >= tc.max {
				t.Fatalf("Stun Attack lands at %.1f %%, want [%v, %v)", rate, tc.min, tc.max)
			}
			if got := castOf(t, b, at(time.Second)) == skillStunAttack; got != tc.want {
				t.Errorf("Stun Attack at %.1f %%: cast %v, want %v", rate, got, tc.want)
			}
		})
	}
}

// TestPvPBreakOffUltimateDefenseKN checks the KN case of Ultimate Defense
// around the break-off (docs/pvp/layer.md "Ultimate Defense"): a `support`
// KN, CP gone, under 20 % HP, losing to a Warrior, able to outrun it, flees
// and casts no Ultimate Defense; unable, or in a duel, it casts it. Under
// it: no flee, no chase; its karma-free opponent's flag blinking or the
// opponent nearing an arena edge with Drain Health in flight → the Esc,
// the move, no damage step; ArenaStay → the stop move and one interval,
// the walk out once 110 drops.
func TestPvPBreakOffUltimateDefenseKN(t *testing.T) {
	udOn := AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillUltimateDefense, Level: 1, Left: 30 * time.Second}}}
	// udUp puts Ultimate Defense on the bot at when, its 30-minute reuse
	// running.
	udUp := func(t *testing.T, b *Brain, when time.Time) {
		t.Helper()
		b.handleOne(t, udOn, when)
		b.world.holdSkill(skillUltimateDefense, when.Add(30*time.Minute))
	}
	knight := func(t *testing.T, skills ...int32) *Brain {
		b := pvpRowBrain(t, classKnight, 28, "SWORD", lyrWarrior, skills...)
		wearShield(b)
		b.persona.Role, b.builtOK = RoleSupport, false
		breakingOff(t, b)
		return b
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, b *Brain)
		kind  actionKind
		skill int32
	}{
		{name: "able to outrun: flees, no Ultimate Defense", kind: actMove},
		{name: "unable to outrun: Ultimate Defense", setup: func(_ *testing.T, b *Brain) { b.world.Self.RunSpeed = 100 }, kind: actCast, skill: skillUltimateDefense},
		{name: "in a duel: Ultimate Defense", setup: func(t *testing.T, b *Brain) { duelUntil(t, b, DuelFighting, t0) }, kind: actCast, skill: skillUltimateDefense},
		{name: "under it: no flee, no chase", setup: func(t *testing.T, b *Brain) { udUp(t, b, at(5*time.Second)) }, kind: actHold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := knight(t, skillUltimateDefense, skillShieldStun, skillPowerStrike)
			if tc.setup != nil {
				tc.setup(t, b)
			}
			_, a := b.decide(at(10 * time.Second))
			if a.kind != tc.kind || tc.kind == actCast && a.skill != tc.skill {
				t.Fatalf("action %v skill %d, want %v skill %d", a.kind, a.skill, tc.kind, tc.skill)
			}
		})
	}
	t.Run("under it: flee sends nothing", func(t *testing.T) {
		b := knight(t, skillUltimateDefense)
		udUp(t, b, at(5*time.Second))
		if a := b.flee(pcID, at(10*time.Second)); a.kind != actHold {
			t.Fatalf("flee %v under Ultimate Defense, want nothing", a.kind)
		}
	})
	t.Run("under it: a karma-free opponent's blink gets the Esc and the move, no damage step", func(t *testing.T) {
		b := knight(t, skillUltimateDefense, skillDrainHealth, skillShieldStun, skillPowerStrike)
		udUp(t, b, t0)
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: skillDrainHealth},
			{at: 100 * time.Millisecond, ev: selfCast(skillDrainHealth, 3*time.Second), noDecide: true},
			{at: time.Second, ev: flagTo(2), kind: actTargetCancel, rule: "guard"},
			{at: time.Second + 100*time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard"},
			{at: time.Second + 200*time.Millisecond, ev: ActionFailed{}, kind: actHold},
			{at: 3 * time.Second, kind: actHold},
		})
	})
	t.Run("under it: the opponent nearing the arena edge gets the Esc and the move", func(t *testing.T) {
		b := knight(t, skillUltimateDefense, skillDrainHealth)
		b.cfg.Travel.zones.Arenas = []datapack.Zone{box(-2000, -2000, 2000, 2000)}
		b.world.Self.move = stopAt(Point{X: 1840})
		b.handleOne(t, CharInfo{ObjectID: pcID, Name: foeName, ClassID: lyrWarrior, Pos: Point{X: 1300}, RunSpeed: 124, CollisionRadius: 9}, t0)
		udUp(t, b, t0)
		play(t, b, []beat{
			{at: 0, kind: actCast, skill: skillDrainHealth},
			{at: 100 * time.Millisecond, ev: selfCast(skillDrainHealth, 3*time.Second), noDecide: true},
			{at: time.Second, ev: StopMove{ObjectID: pcID, Pos: Point{X: 1300, Y: 1800}}, kind: actTargetCancel, rule: "guard"},
			{at: time.Second + 100*time.Millisecond, ev: MagicSkillCanceled{CasterID: selfID}, kind: actMove, rule: "guard"},
		})
	})
	t.Run("under it: ArenaStay sends the stop move, then walks out once it ends", func(t *testing.T) {
		b := huntBrain(t)
		s := &b.world.Self
		s.ClassID, s.Level, b.persona.Role, b.builtOK = classKnight, 28, RoleSupport, false
		b.handleOne(t, DeleteObject{ObjectID: pcID}, t0)
		arenaTripAt(b, box(-500, -500, 500, 500))
		b.cfg.Nav = testNav(t)
		b.walk = newWalker(b.cfg.Nav)
		b.world.SetGround(b.cfg.Nav.Engine)
		b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: "near", Pos: Point{Y: 100}, ClassID: lyrWarrior, RunSpeed: 124}, t0)
		place(t, b, pvpThird, Point{Y: 100})
		b.world.Target = pvpThird
		stay := at(arenaStay + time.Second)
		b.handleOne(t, udOn, stay)
		rule, a := b.decide(stay)
		if rule != "arena leave" || a.kind != actMove {
			t.Fatalf("rule %q action %v, want the stop move", rule, a.kind)
		}
		if _, err := b.send(a, stay); err != nil {
			t.Fatal(err)
		}
		interval := swingTime(b.world.Self.PAtkSpd)
		if rule, a := b.decide(stay.Add(interval + 100*time.Millisecond)); rule != "arena leave" || a.kind != actHold {
			t.Fatalf("rule %q action %v under Ultimate Defense after the interval, want the hold", rule, a.kind)
		}
		end := stay.Add(interval + 200*time.Millisecond)
		b.handleOne(t, AbnormalStatusUpdate{}, end)
		if rule, a := b.decide(end); rule != "arena leave" || a.kind != actMove || moveTarget(a.payload).X <= 0 {
			t.Fatalf("rule %q action %v once it ended, want the walk out", rule, a.kind)
		}
	})
}

// TestPvPAreaBystanderWA checks the WA case of the area rule
// (docs/pvp/guards.md "Area hits and bystanders"): with a pole against a
// flagged karma-free Warrior, a second flagged karma-free player 40 from
// the bot in the cone → no swing, Wild Sweep or nextActionAttack step: the
// move, a walk to a stand point with the bystander out of the cone, then
// the attack; a bystander next to the target → War Cry, then no swing or
// damage step until it leaves; with a two-handed blunt the same bystander
// holds nothing: Stun Attack, then Power Smash.
func TestPvPAreaBystanderWA(t *testing.T) {
	all := []int32{skillWildSweep, skillPowerSmash, skillStunAttack, skillWarCry}
	warlord := func(t *testing.T, weapon string, target Point) *Brain {
		b := pvpRowBrain(t, 1, 36, weapon, lyrWarrior, all...)
		foeOf(t, b, lyrWarrior, target, 0)
		b.reactAt = time.Time{}
		return b
	}
	t.Run("pole: the reposition", func(t *testing.T) {
		b := warlord(t, "POLE", Point{X: 90})
		buffed(skillWarCry)(t, b)
		player(t, b, pvpThird, "x", Point{X: 40, Y: 10}, 1, 0, 9)
		b.engaged, b.pvp.guard.intent = pcID, pcID
		now := at(time.Second)
		if !b.stepHeld(pcID, skillPowerSmash, now) {
			t.Error("a nextActionAttack step goes with a bystander in the pole's cone")
		}
		play(t, b, []beat{{at: time.Second, kind: actMove, rule: "guard"}})
		rule, a := b.decide(at(1200 * time.Millisecond))
		if rule != "attacked" {
			t.Fatalf("rule %q, want attacked", rule)
		}
		st := moveDest(t, a)
		if b.coneHit(pcID, st, Point{X: 90}, now) {
			t.Fatalf("stand point %v has the bystander in its cone", st)
		}
		b.world.Self.move = stopAt(st)
		if _, a := b.decide(at(2 * time.Second)); a.kind != actAttack {
			t.Fatalf("at the stand point: %v, want the attack", a.kind)
		}
	})
	t.Run("pole: a bystander next to the target, War Cry, then the hold", func(t *testing.T) {
		b := warlord(t, "POLE", Point{X: 60})
		player(t, b, pvpThird, "x", Point{X: 60, Y: 15}, 1, 0, 9)
		play(t, b, []beat{
			{at: time.Second, kind: actCast, skill: skillWarCry},
			{at: 1100 * time.Millisecond, ev: MagicSkillUse{CasterID: selfID, TargetID: selfID, SkillID: skillWarCry, Level: 1}, noDecide: true},
			{at: 1100 * time.Millisecond, ev: AbnormalStatusUpdate{Effects: []Effect{{SkillID: skillWarCry, Level: 1, Left: time.Hour}}}, noDecide: true},
			{at: 1200 * time.Millisecond, kind: actHold},
			{at: 2 * time.Second, ev: DeleteObject{ObjectID: pvpThird}, kind: actAttack},
		})
	})
	t.Run("two-handed blunt: Stun Attack, then Power Smash", func(t *testing.T) {
		b := warlord(t, "BIGBLUNT", Point{X: 90})
		buffed(skillWarCry)(t, b)
		player(t, b, pvpThird, "x", Point{X: 40, Y: 10}, 1, 0, 9)
		if got := castOf(t, b, at(time.Second)); got != skillStunAttack {
			t.Fatalf("cast %d, want Stun Attack", got)
		}
		b.world.holdSkill(skillStunAttack, at(time.Minute))
		if got := castOf(t, b, at(time.Second)); got != skillPowerSmash {
			t.Fatalf("cast %d, want Power Smash", got)
		}
	})
}
