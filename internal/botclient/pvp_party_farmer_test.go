package botclient

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

// Bot-led party PvP fixtures (docs/pvp/party.md, docs/parties/): an M11 bot
// party led by a farmer (Damage role, fight back only, SupportFlagged off)
// with a support KN and a support CL. Seen by a support member the farmer is
// ptyDmg; seen by the farmer itself it is the bot (selfID) and leads.

// farmerPersona is the farmer preset's persona as the persona edit: the
// default persona in the Damage role (cmd/bots damagePersona), PvP policy
// untouched (fight back only, SupportFlagged off).
func farmerPersona(p *Persona) { p.Role = RoleDamage }

// farmerParty is the farmer's party seen by self, the farmer at farmerHP of
// 100 and the support members at full HP; the farmer is ptyDmg unless self.
func farmerParty(self, farmerHP int32) []PartyMember {
	var out []PartyMember
	for _, r := range []PartyMember{ptyRow(ptyDmg, "farmer", 10, farmerHP), ptyRow(ptyKN, "kn", 4, 100), ptyRow(ptyCL, "cl", 15, 100)} {
		// The farmer's own window lists the others only: the bot is the farmer.
		if r.ObjectID == self || self == selfID && r.ObjectID == ptyDmg {
			continue
		}
		out = append(out, r)
	}
	return out
}

// farmerBrain is partyBrain in the farmer's bot-led party instead of the
// GM's: the party window names leader (selfID when the bot is the farmer,
// ptyDmg when a support member) as the party's leader.
func farmerBrain(t *testing.T, class int32, edit func(*Persona), kit ClassKit, leader int32, rows []PartyMember, flagged ...int32) *Brain {
	t.Helper()
	b := partyBrain(t, class, edit, kit, rows, flagged...)
	b.handleOne(t, PartySmallWindowAll{LeaderID: leader, Members: rows}, t0)
	b.nextTick = t0.Add(time.Hour)
	b.reactAt = time.Time{}
	return b
}

// TestPvPTargetHealerFarmer checks party PvP (docs/pvp/party.md, X9) in an
// M11 bot party led by a farmer with a support KN and a support CL, hit by
// two flagged enemies (#423): the farmer's enemy set and healer first, the
// KN's peel for the CL and the farmer, and the CL's SupportFlagged.
func TestPvPTargetHealerFarmer(t *testing.T) {
	dmgKit := ClassKit{Rotation: []Step{{Skill: lyrNuke}}}
	t.Run("the leading farmer's enemy set is the party's hitters and healers", func(t *testing.T) {
		b := farmerBrain(t, 10, farmerPersona, dmgKit, selfID, farmerParty(selfID, 100))
		if !b.leading() || b.assisted() != 0 {
			t.Fatalf("leading %v assisted %#x, want the farmer leading and assisting nobody", b.leading(), b.assisted())
		}
		// A member's own hit is no leader's target: the farmer leads.
		b.handleOne(t, ptyHit(ptyKN, ptyFoe2, Point{X: 100}), at(500*time.Millisecond))
		b.handleOne(t, ptyHit(pcID, ptyCL, ptyFoe1Pos), at(time.Second))
		if b.reactAt.IsZero() {
			t.Error("a hit on the support CL did not wake the farmer")
		}
		if got := b.partyEnemies(at(1500 * time.Millisecond)); !slices.Equal(got, []int32{pcID}) {
			t.Fatalf("enemies %#x, want only the CL's hitter %#x", got, pcID)
		}
		wantDecision(t, b, at(1500*time.Millisecond), "party fight", actSelect, pcID, 0)
		b.handleOne(t, ptyHeal(ptyFoe2, pcID, ptyFoe2Pos), at(2*time.Second))
		want := []int32{pcID, ptyFoe2}
		slices.Sort(want)
		if got := b.partyEnemies(at(2500 * time.Millisecond)); !slices.Equal(got, want) {
			t.Fatalf("enemies %#x, want the hitter and its healer %#x", got, want)
		}
	})
	t.Run("the leading farmer switches to the enemy healer", func(t *testing.T) {
		b := farmerBrain(t, 10, farmerPersona, dmgKit, selfID, farmerParty(selfID, 100))
		b.handleOne(t, ptyHit(ptyFoe2, ptyCL, ptyFoe2Pos), at(time.Second))
		b.handleOne(t, ptyHit(pcID, ptyKN, ptyFoe1Pos), at(2*time.Second))
		wantDecision(t, b, at(2500*time.Millisecond), "party fight", actSelect, pcID, 0)
		b.handleOne(t, ptyHeal(ptyFoe2, pcID, ptyFoe2Pos), at(3*time.Second))
		if r := b.pvp.opponents[ptyFoe2]; r == nil || !r.HealedAt.Equal(at(3*time.Second)) {
			t.Fatal("Battle Heal on an enemy did not mark the enemy healer")
		}
		wantDecision(t, b, at(3500*time.Millisecond), "party fight", actSelect, ptyFoe2, 0)
		b.world.Target = ptyFoe2
		wantDecision(t, b, at(3600*time.Millisecond), "party fight", actCast, ptyFoe2, lyrNuke)
	})
	knKit := ClassKit{PvP: []Step{{Skill: skillAggression, Peel: true}}}
	for _, tc := range []struct {
		name   string
		victim int32
	}{
		{"the support KN peels the enemy hitting the CL", ptyCL},
		{"the support KN peels the enemy hitting the farmer leading", ptyDmg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := farmerBrain(t, 4, nil, knKit, ptyDmg, farmerParty(ptyKN, 100))
			if b.assisted() != ptyDmg {
				t.Fatalf("assisted %#x, want the farmer %#x", b.assisted(), ptyDmg)
			}
			b.handleOne(t, ptyHit(pcID, tc.victim, ptyFoe1Pos), at(time.Second))
			wantDecision(t, b, at(1500*time.Millisecond), "party fight", actSelect, pcID, 0)
			b.world.Target = pcID
			if got := castOf(t, b, at(1600*time.Millisecond)); got != skillAggression {
				t.Errorf("cast %d, want Aggression %d", got, skillAggression)
			}
		})
	}
	clKit := ClassKit{
		Heals: []Heal{{Skill: skillHeal, HP: 60}},
		PvP:   []Step{{Skill: skillMentalShield, Target: TargetSelf, Vs: mysticVsCB}},
	}
	shielded := []Effect{{SkillID: skillMentalShield, Level: 1, Left: time.Hour}}
	// Behind the farmer (ptyDmg at 50,-50) by behindDist, away from the
	// enemy hitting it (ptyFoe2 at 300,300).
	behindFarmer := Point{X: -66, Y: -213}
	for _, tc := range []struct {
		name    string
		support bool
		shield  bool
		rule    string
		kind    actionKind
		object  int32
	}{
		{"the support CL leaves the flagged farmer unhealed with SupportFlagged off", false, false, "party buff", actSelect, selfID},
		{"the support CL heals the flagged farmer with SupportFlagged on", true, false, "heal", actSelect, ptyDmg},
		{"Mental Shield skips the flagged farmer with SupportFlagged off", false, true, "party fight", actMove, 0},
		{"Mental Shield on the flagged farmer with SupportFlagged on", true, true, "party buff", actSelect, ptyDmg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var edit func(*Persona)
			if tc.support {
				edit = func(p *Persona) { p.PvP.SupportFlagged = true }
			}
			hp := int32(20)
			if tc.shield {
				hp = 100
			}
			rows := farmerParty(ptyCL, hp)
			b := farmerBrain(t, 15, edit, clKit, ptyDmg, rows, ptyDmg)
			if b.persona.PvP.SupportFlagged != tc.support {
				t.Fatalf("SupportFlagged %v, want %v", b.persona.PvP.SupportFlagged, tc.support)
			}
			if tc.shield {
				b.handleOne(t, AbnormalStatusUpdate{Effects: shielded}, t0)
				b.handleOne(t, PartySpelled{ObjectID: ptyKN, Effects: shielded}, t0)
			}
			b.handleOne(t, ptyHit(ptyFoe2, ptyDmg, ptyFoe2Pos), at(time.Second))
			got, a := b.decide(at(1500 * time.Millisecond))
			if got != tc.rule || a.kind != tc.kind || a.object != tc.object {
				t.Fatalf("decision rule %q kind %v object %#x, want rule %q kind %v object %#x", got, a.kind, a.object, tc.rule, tc.kind, tc.object)
			}
			switch tc.rule {
			case "party buff":
				b.world.Target = tc.object
				wantDecision(t, b, at(1600*time.Millisecond), tc.rule, actCast, tc.object, skillMentalShield)
			case "party fight":
				if want := moveAction(behindFarmer, Point{}); !bytes.Equal(a.payload, want.payload) {
					t.Errorf("move %x, want to %+v behind the farmer", a.payload, behindFarmer)
				}
			}
		})
	}
}
