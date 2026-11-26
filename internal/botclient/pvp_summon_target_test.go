package botclient

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// skillHealTrick is a pet's heal on its owner (Heal Trick,
// skills/4700-4799.xml 4717: HEAL, target OWNER_PET).
const skillHealTrick = int32(4717)

// TestPvPSummonFirst checks when the bot fights a summon before its owner in
// sight (docs/pvp/targets.md "Summons and pets"): only when killing it first
// pays off (its damage above switchFactor times its owner's while both hit,
// or a heal on its owner within enemyHealerFor), and only under the summon
// rules a summon with its owner out of sight passes.
func TestPvPSummonFirst(t *testing.T) {
	now := at(time.Second)
	const owner = "caller"
	// pair puts the flagged owner (pvpThird) at ownerAt and
	// its flagged summon 400 to the left, the summon hitting the bot for
	// summonHit and the owner for ownerHit (none at 0).
	pair := func(t *testing.T, b *Brain, ownerAt Point, summonHit, ownerHit int32) {
		b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: owner, PvPFlag: 1, Pos: ownerAt, RunSpeed: 100}, t0)
		b.handleOne(t, NPCInfo{ObjectID: pvpSummon, NPCID: 12077, Summon: true, Attackable: true, PvPFlag: 1, Title: owner, Pos: Point{X: -400}}, t0)
		b.handleOne(t, hitSelf(pvpSummon, summonHit), t0)
		if ownerHit > 0 {
			b.handleOne(t, hitSelf(pvpThird, ownerHit), t0)
		}
		// hitSelf's Attack places its attacker at the origin: the owner
		// is seen back where it stands.
		b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: owner, PvPFlag: 1, Pos: ownerAt, RunSpeed: 100}, t0)
		b.world.Self.InCombat = true
	}
	// healed has the summon cast Heal Trick on its owner at when.
	healed := func(t *testing.T, b *Brain, when time.Time) {
		b.handleOne(t, MagicSkillUse{CasterID: pvpSummon, TargetID: pvpThird, SkillID: skillHealTrick, Level: 1, HitTime: 4 * time.Second}, when)
	}
	tests := []struct {
		name  string
		arena bool
		setup func(t *testing.T, b *Brain)
		// wantRule is the decision's rule; wantKind and wantObject its
		// action, a zero wantObject checking none; notObject an object the
		// action must not be on.
		wantRule   string
		wantKind   actionKind
		wantObject int32
		notObject  int32
		check      func(t *testing.T, b *Brain)
	}{
		{
			name:     "an owner standing back is fought, not its summon",
			setup:    func(t *testing.T, b *Brain) { pair(t, b, Point{Y: -300}, 100, 0) },
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name:     "a summon dealing over twice its owner's damage is selected first",
			setup:    func(t *testing.T, b *Brain) { pair(t, b, Point{Y: -300}, 100, 20) },
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
			check: func(t *testing.T, b *Brain) {
				if hits := b.attackers(now); len(hits) != 1 || hits[0] != pvpSummon || b.damageOf(pvpSummon, now) != 120 {
					t.Errorf("attackers = %#x, summon damage %d, want the summon alone standing in for its owner, 120", hits, b.damageOf(pvpSummon, now))
				}
			},
		},
		{
			name:     "a summon dealing under twice its owner's damage leaves the owner first",
			setup:    func(t *testing.T, b *Brain) { pair(t, b, Point{Y: -300}, 30, 20) },
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name: "a selected summon still out-hitting its owner is attacked with a plain Action",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 30, 20)
				b.world.Target = pvpSummon
			},
			wantRule: "attacked", wantKind: actAttack, wantObject: pvpSummon,
			check: func(t *testing.T, b *Brain) {
				if b.forced(pvpSummon, now) {
					t.Error("an attack on the summon would be forced")
				}
			},
		},
		{
			name: "a selected summon the owner out-hits is left for the owner",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 15, 20)
				b.world.Target = pvpSummon
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name: "a pet that healed its owner is selected first",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 5, 40)
				healed(t, b, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name: "a pet healing the bot's target takes the target's place",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 5, 40)
				b.world.Target = pvpThird
				healed(t, b, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name: "a heal older than enemyHealerFor no longer counts",
			setup: func(t *testing.T, b *Brain) {
				healed(t, b, now.Add(-enemyHealerFor-time.Second))
				pair(t, b, Point{Y: -300}, 5, 40)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name: "a pet's heal on another player does not count",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 5, 40)
				b.handleOne(t, MagicSkillUse{CasterID: pvpSummon, TargetID: pcID, SkillID: skillHealTrick, Level: 1}, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name: "a clean summon is never fought first, whatever it dealt",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 100, 20)
				healed(t, b, t0)
				b.handleOne(t, RelationChanged{ObjectID: pvpSummon}, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpThird,
		},
		{
			name: "a summon with karma paying off is fought first",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 100, 20)
				b.handleOne(t, RelationChanged{ObjectID: pvpSummon, Karma: 100}, t0)
			},
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name: "a blinking summon fought first gets no damage step",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 100, 20)
				b.handleOne(t, RelationChanged{ObjectID: pvpSummon, PvPFlag: 2}, t0)
				b.world.Target = pvpSummon
			},
			wantRule: "attacked", notObject: pvpSummon,
		},
		{
			name: "a clan mate's summon is never fought",
			setup: func(t *testing.T, b *Brain) {
				b.world.Clan.ID = 7
				pair(t, b, Point{Y: -300}, 100, 20)
				b.handleOne(t, CharInfo{ObjectID: pvpThird, Name: owner, PvPFlag: 1, ClanID: 7, Pos: Point{Y: -300}}, t0)
			},
			notObject: pvpSummon,
		},
		{
			name: "a party member's summon is never fought",
			setup: func(t *testing.T, b *Brain) {
				b.world.Apply(PartySmallWindowAll{LeaderID: selfID, Members: []PartyMember{{ObjectID: pvpThird, Name: owner, HP: 100, MaxHP: 100}}}, t0)
				pair(t, b, Point{Y: -300}, 100, 0)
				healed(t, b, t0)
			},
			notObject: pvpSummon,
		},
		{
			name: "a summon paying off in the bot's duel is not fought",
			setup: func(t *testing.T, b *Brain) {
				duelUntil(t, b, DuelFighting, t0.Add(-10*time.Second))
				pair(t, b, Point{Y: -300}, 100, 20)
			},
			wantRule: "duel", wantKind: actSelect, wantObject: pcID,
		},
		{
			name:     "in an arena, with its owner inside by more than arenaEdge",
			arena:    true,
			setup:    func(t *testing.T, b *Brain) { pair(t, b, Point{Y: -300}, 100, 20) },
			wantRule: "attacked", wantKind: actSelect, wantObject: pvpSummon,
		},
		{
			name:     "in an arena, with its owner 100 from the edge",
			arena:    true,
			setup:    func(t *testing.T, b *Brain) { pair(t, b, Point{X: 1900}, 100, 20) },
			wantRule: "attacked", notObject: pvpSummon,
		},
		{
			name: "a healing pet is never a hunting target",
			setup: func(t *testing.T, b *Brain) {
				pair(t, b, Point{Y: -300}, 0, 0)
				delete(b.world.Attackers, pvpSummon)
				healed(t, b, t0)
				b.world.Self.InCombat = false
			},
			notObject: pvpSummon,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := pvpBrain(t, tc.arena, nil)
			b.cfg.Travel.skills[datapack.SkillRef{ID: skillHealTrick, Level: 1}] = datapack.Skill{Type: "HEAL"}
			tc.setup(t, b)
			rule, a := b.decide(now)
			if tc.wantRule != "" && rule != tc.wantRule || a.kind != tc.wantKind && tc.wantObject != 0 || tc.wantObject != 0 && a.object != tc.wantObject {
				t.Fatalf("decide = rule %q action %d on %#x, want rule %q action %d on %#x", rule, a.kind, a.object, tc.wantRule, tc.wantKind, tc.wantObject)
			}
			if tc.notObject != 0 && a.object == tc.notObject {
				t.Fatalf("decide = rule %q action %d on %#x, want none on it", rule, a.kind, a.object)
			}
			if tc.check != nil {
				tc.check(t, b)
			}
		})
	}
}
