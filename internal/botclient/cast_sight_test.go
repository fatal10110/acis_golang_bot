package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/geo/geotest"
)

// sightBrain is a level-level class Support bot knowing skills, out of
// combat at self on the synthetic nav (or with no geodata), in pcID's bot
// party with pcID at at: dead for a raise, at 30 % HP else.
func sightBrain(t *testing.T, geodata bool, class, level int32, self, at Point, dead bool, skills ...int32) *Brain {
	t.Helper()
	p := DefaultPersona()
	p.IdleChance, p.Role = 0, RoleSupport
	var n *Nav
	if geodata {
		n = testNav(t)
	}
	b, _, _ := newNavBrain(t, n, p, self)
	s := &b.world.Self
	s.ClassID, s.Level, s.RunSpeed, s.InCombat = class, level, 200, false
	b.builtOK = false
	b.cfg.Travel = &TravelMap{}
	known := make([]KnownSkill, len(skills))
	for i, id := range skills {
		known[i] = KnownSkill{ID: id, Level: 1}
	}
	b.world.Apply(SkillList{Skills: known}, t0)
	castRange(b, skillResurrection, 400)
	linkBrain(b, standing{}).party = Command{Kind: CommandJoin, Arg: "other"}
	b.world.Apply(CharInfo{ObjectID: pcID, Name: "other", Pos: at}, t0)
	b.world.Apply(PartySmallWindowAll{LeaderID: pcID, Members: []PartyMember{
		{ObjectID: pcID, Name: "other", HP: 30, MaxHP: 100, ClassID: HumanFighter.ID},
	}}, t0)
	if dead {
		b.world.Apply(Die{ObjectID: pcID}, t0)
	}
	b.world.Target = pcID
	return b
}

// TestCastSight checks the cast after a 181 refusal (can't see target,
// #545): a Cleric's raise of its dead leader and a mystic's heal of its
// wounded leader, refused for sight, walk before they are cast again: to a
// stand point the bot's geodata sees the leader from within the cast's
// range, or, where it sees the leader from where it stands though the
// server did not, to a melee stand point with the straight line open. With
// no geodata, a leader that moved, or the server taking a cast at the leader
// meanwhile, the cast goes out again from where the bot stands once its
// castRefusedHold ends, as before.
func TestCastSight(t *testing.T) {
	// inU is inside the U's west mouth, 208 from behindBase past the U's
	// base: in cast range, out of sight. open and openFar stand on open
	// ground 240 apart, in sight of each other.
	inU, behindBase := scenePoint(geotest.UInside, 5, 0), scenePoint(geotest.UBehind, -12, 0)
	open, openFar := scenePoint(geotest.UBehind, 0, 0), scenePoint(geotest.UBehind, 15, 0)
	raise := func(b *Brain, now time.Time) (Action, bool) { return b.ruleRaise(now) }
	heal := func(b *Brain, now time.Time) (Action, bool) { return b.ruleHeal(now) }
	for _, tt := range []struct {
		name     string
		cast     func(b *Brain, now time.Time) (Action, bool)
		raise    bool
		geodata  bool
		self, at Point
		// between runs between the refusal and the next cast.
		between func(b *Brain, now time.Time)
		// wantWalk: the next cast walks first; wantReach bounds the stand
		// point's 3D distance from the leader, wantLine asks the straight
		// line to it open (a melee stand point), else the sight.
		wantWalk  bool
		wantReach float64
		wantLine  bool
	}{
		{
			name: "raise behind a wall: walks into sight", cast: raise, raise: true, geodata: true, self: inU, at: behindBase,
			wantWalk: true, wantReach: 400 - standMargin,
		},
		{
			name: "heal behind a wall: walks into sight", cast: heal, geodata: true, self: inU, at: behindBase,
			wantWalk: true, wantReach: rangedReach - standMargin,
		},
		{
			name: "raise in the bot's sight: approaches", cast: raise, raise: true, geodata: true, self: open, at: openFar,
			wantWalk: true, wantReach: meleeStand, wantLine: true,
		},
		{
			name: "heal in the bot's sight: approaches", cast: heal, geodata: true, self: open, at: openFar,
			wantWalk: true, wantReach: meleeStand, wantLine: true,
		},
		{name: "raise without geodata: casts again", cast: raise, raise: true, self: inU, at: behindBase},
		{name: "heal without geodata: casts again", cast: heal, self: inU, at: behindBase},
		{
			name: "the patient moved: casts again", cast: heal, geodata: true, self: inU, at: behindBase,
			between: func(b *Brain, now time.Time) {
				b.world.Apply(StopMove{ObjectID: pcID, Pos: scenePoint(geotest.UInside, 0, 3)}, now)
			},
		},
		{
			name: "a cast at the leader taken meanwhile: casts again", cast: raise, raise: true, geodata: true, self: inU, at: behindBase,
			between: func(b *Brain, now time.Time) {
				if err := b.handle([]Event{MagicSkillUse{CasterID: selfID, TargetID: pcID, SkillID: skillResurrection, Level: 1}}, now); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			class, level, skills := classCleric, int32(20), []int32{skillResurrection}
			if !tt.raise {
				class, level, skills = classHumanMystic, 14, []int32{skillHeal, skillBattleHeal}
			}
			b := sightBrain(t, tt.geodata, class, level, tt.self, tt.at, tt.raise, skills...)
			now := t0.Add(time.Second)
			a, ok := tt.cast(b, now)
			if !ok || a.kind != actCast || a.object != pcID {
				t.Fatalf("first cast = kind %d object %#x, %v; want a cast at the leader", a.kind, a.object, ok)
			}
			if sent, err := b.send(a, now); !sent || err != nil {
				t.Fatalf("send = %v, %v", sent, err)
			}
			if err := b.handle([]Event{SystemMessage{ID: msgCantSeeTarget}}, now); err != nil {
				t.Fatal(err)
			}
			if tt.between != nil {
				tt.between(b, now)
			}
			// The refused skill is held for castRefusedHold either way.
			if a, ok := tt.cast(b, now.Add(time.Second)); ok && a.kind == actCast && a.skill == b.castSkill {
				t.Fatalf("refused skill %d cast again within castRefusedHold", a.skill)
			}
			now = now.Add(castRefusedHold + time.Second)
			a, ok = tt.cast(b, now)
			if !tt.wantWalk {
				if !ok || a.kind != actCast || a.object != pcID {
					t.Fatalf("next cast = kind %d object %#x walk %v, %v; want a cast at the leader", a.kind, a.object, a.walk, ok)
				}
				return
			}
			if !ok || a.kind != actMove || !a.walk {
				t.Fatalf("next cast = kind %d object %#x walk %v (%s), %v; want a walk", a.kind, a.object, a.walk, b.walk.reason, ok)
			}
			stand := b.unseen.stand
			if d := distance3(stand, tt.at); d > tt.wantReach {
				t.Errorf("stand point %+v %.0f from the leader, want within %.0f", stand, d, tt.wantReach)
			}
			if tt.wantLine && !b.walk.lineOpen(stand, tt.at) {
				t.Errorf("melee stand point %+v has no open line to the leader", stand)
			}
			if !tt.wantLine && !b.walk.sightOpen(stand, tt.at, defaultTargetHeight) {
				t.Errorf("stand point %+v out of sight of the leader", stand)
			}
			if b.walk.dest != stand {
				t.Errorf("walk to %+v, want the stand point %+v", b.walk.dest, stand)
			}
			// Stopped at the stand point, the bot casts.
			now = now.Add(10 * time.Second)
			if err := b.handle([]Event{StopMove{ObjectID: selfID, Pos: stand}}, now); err != nil {
				t.Fatal(err)
			}
			if a, ok = tt.cast(b, now); !ok || a.kind != actCast || a.object != pcID {
				t.Fatalf("cast at the stand point = kind %d object %#x walk %v (%s), %v; want a cast at the leader", a.kind, a.object, a.walk, b.walk.reason, ok)
			}
		})
	}
}
