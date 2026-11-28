package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/charmodel"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// kaiCat is Kai the Cat's template (npcs/14000-14999.xml 14207), the
// servitor Summon Kai the Cat (1276) level 1 names.
var kaiCat = datapack.NPC{ID: 14207, Name: "Kai the Cat", Type: "Servitor", Level: 42, Combat: datapack.Combat{
	HP: 1050.81652, PAtk: 198.34591, PDef: 164.88513, MAtk: 135.44454, MDef: 120.65578, AtkSpd: 253, Crit: 4,
	STR: 40, INT: 21, DEX: 30, WIT: 20, CON: 43, MEN: 25,
}}

var summonKai1 = datapack.SkillRef{ID: 1276, Level: 1}

const kaiObject = int32(0x10000777)

// withServitorTemplates gives b's travel map Kai's template and the
// summon skill level that names it.
func withServitorTemplates(b *Brain) {
	b.cfg.Travel.servitors = map[int32]datapack.NPC{kaiCat.ID: kaiCat, 14000: {ID: 14000, Type: "Monster"}}
	b.cfg.Travel.skills = map[datapack.SkillRef]datapack.Skill{summonKai1: {Type: "SUMMON", NPCID: kaiCat.ID}}
}

// kaiOut puts Kai out as its PetInfo reports it: live P.Atk and attack
// speed above the template's, as its passives and buffs raise them.
func kaiOut(b *Brain) *Servitor {
	b.world.Servitor = &Servitor{ObjectID: kaiObject, TemplateID: kaiCat.ID, Level: 42, HP: 1100, MaxHP: 1100, PAtk: 260, PDef: 210, PAtkSpd: 300}
	return b.world.Servitor
}

// summonerKit stands in a summoner row (#761) for b's build: glass cannon
// 3.0, normal 2.0 with its servitor out (docs/servitors.md section 5).
func summonerKit(b *Brain) {
	s := &b.world.Self
	race, _ := datapack.ClassRace(s.ClassID)
	b.built = CompileBuild(race, s.ClassID, s.Level, b.persona.Role, BranchChoice{})
	b.built.Kit.SpotMargin, b.built.Kit.ServitorSpotMargin = marginGlassCannon, marginNormal
	b.builtFor, b.builtOK = buildKey{s.ClassID, s.Level, b.persona.Role}, true
}

// TestServitorModel checks the servitor the model counts: the live one's
// template with its PetInfo numbers, else the template the summon the bot
// can cast names (also for a dead one); none without either, with a dead
// servitor and no summon, an unknown template or no travel map.
func TestServitorModel(t *testing.T) {
	live := charmodel.Servitor(kaiCat, charmodel.ServitorLive{MaxHP: 1100, PAtk: 260, PDef: 210, PAtkSpd: 300})
	tmpl := charmodel.Servitor(kaiCat, charmodel.ServitorLive{})
	tests := []struct {
		name string
		set  func(b *Brain)
		want charmodel.Profile
		ok   bool
	}{
		{"none", func(*Brain) {}, charmodel.Profile{}, false},
		{"out", func(b *Brain) { kaiOut(b) }, live, true},
		{"out and summonable: the live one", func(b *Brain) { kaiOut(b); b.summonReady = summonKai1 }, live, true},
		{"summonable", func(b *Brain) { b.summonReady = summonKai1 }, tmpl, true},
		{"dead", func(b *Brain) { kaiOut(b).Dead = true }, charmodel.Profile{}, false},
		{"dead, summonable: the template", func(b *Brain) { kaiOut(b).Dead = true; b.summonReady = summonKai1 }, tmpl, true},
		{"out with an unknown template", func(b *Brain) { kaiOut(b).TemplateID = 14999 }, charmodel.Profile{}, false},
		{"summonable, a skill naming no servitor", func(b *Brain) { b.summonReady = datapack.SkillRef{ID: 1177, Level: 1} }, charmodel.Profile{}, false},
		{"out, no travel map", func(b *Brain) { kaiOut(b); b.cfg.Travel = nil }, charmodel.Profile{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, squire14, 0)
			withServitorTemplates(b)
			tt.set(b)
			got, ok := b.servitorModel()
			if ok != tt.ok || got != tt.want {
				t.Fatalf("servitorModel = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestSpotMarginServitor checks the margin switch: a summoner row's normal
// 2.0 while its servitor is out or it can summon one (a dead one counts
// through summonReady), its glass cannon 3.0 with neither; a row without
// ServitorSpotMargin (the human fighter's normal 2.0) and
// SpotPolicy.Margin are unchanged by a servitor.
func TestSpotMarginServitor(t *testing.T) {
	tests := []struct {
		name string
		set  func(b *Brain)
		want float64
	}{
		{"fighter, no servitor", func(*Brain) {}, marginNormal},
		{"fighter, servitor out", func(b *Brain) { kaiOut(b) }, marginNormal},
		{"summoner, no servitor", summonerKit, marginGlassCannon},
		{"summoner, summonable", func(b *Brain) { summonerKit(b); b.summonReady = summonKai1 }, marginNormal},
		{"summoner, servitor out", func(b *Brain) { summonerKit(b); kaiOut(b) }, marginNormal},
		{"summoner, servitor dead", func(b *Brain) { summonerKit(b); kaiOut(b).Dead = true }, marginGlassCannon},
		{"summoner, servitor dead, summonable", func(b *Brain) { summonerKit(b); kaiOut(b).Dead = true; b.summonReady = summonKai1 }, marginNormal},
		{"summoner, SpotPolicy.Margin with a servitor out", func(b *Brain) { summonerKit(b); kaiOut(b); b.persona.Spots.Margin = 1.25 }, 1.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, squire14, 0)
			withServitorTemplates(b)
			tt.set(b)
			bd := b.build()
			if got := b.spotMargin(&bd); got != tt.want {
				t.Fatalf("spotMargin = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPackFitServitor checks the servitor in the spot fit (docs/servitors.md
// section 3): with Kai out, or summonable, the level 14 squire kills the
// werewolves together with it (1/(1/T_bot + 1/T_servitor)) and fits them,
// which it does not alone (TestPackFit); a dead servitor counts for
// nothing unless the bot can summon one again; and how long it lasts against the pack is the same either way,
// the conservative "Pack threat".
func TestPackFitServitor(t *testing.T) {
	alone := newSquireBrain(t, squire14, 0)
	bdAlone := alone.build()
	kill0, die0, ok := alone.packFight(&tiWolves, &bdAlone, bdAlone.Kit.SpotMargin, &crew{})
	if !ok || math.Abs(kill0-38.1) > 0.1 || math.Abs(die0-53.5) > 0.1 {
		t.Fatalf("alone packFight = %.2f, %.2f, %v; want TestPackFit's 38.1, 53.5", kill0, die0, ok)
	}
	tests := []struct {
		name   string
		set    func(b *Brain)
		helped bool
		fit    bool
	}{
		{"no servitor", func(*Brain) {}, false, false},
		{"Kai out", func(b *Brain) { kaiOut(b) }, true, true},
		{"Kai summonable", func(b *Brain) { b.summonReady = summonKai1 }, true, true},
		{"Kai dead", func(b *Brain) { kaiOut(b).Dead = true }, false, false},
		{"Kai dead, summonable", func(b *Brain) { kaiOut(b).Dead = true; b.summonReady = summonKai1 }, true, true},
		{"Kai out, dealing no damage", func(b *Brain) { kaiOut(b).PAtk = 0; b.cfg.Travel.servitors[kaiCat.ID] = noAttack(kaiCat) }, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSquireBrain(t, squire14, 0)
			withServitorTemplates(b)
			tt.set(b)
			bd := b.build()
			kill, die, ok := b.packFight(&tiWolves, &bd, bd.Kit.SpotMargin, &crew{})
			if !ok || die != die0 {
				t.Fatalf("packFight die = %.2f, %v; want %.2f, unchanged by a servitor", die, ok, die0)
			}
			if helped := kill < kill0-0.1; helped != tt.helped || (!tt.helped && math.Abs(kill-kill0) > 1e-9) {
				t.Errorf("packFight kill = %.2f, alone %.2f; want helped %v", kill, kill0, tt.helped)
			}
			if got := b.spotFits(&tiWolves, at(time.Second)); got != tt.fit {
				t.Errorf("spotFits = %v, want %v", got, tt.fit)
			}
		})
	}
}

// noAttack is n with no P.Atk: a servitor whose melee deals nothing.
func noAttack(n datapack.NPC) datapack.NPC {
	n.Combat.PAtk = 0
	return n
}

// TestPackFitPickServitor checks the spot a level 14 squire claims with 3
// potions, the werewolves the nearest and harder: alone the orcs, with Kai
// out the werewolves; as a summoner row (glass cannon 3.0 alone, normal 2.0
// with its servitor out) none alone and the werewolves with Kai out.
func TestPackFitPickServitor(t *testing.T) {
	place := func(s datapack.Spot, name string, x, y int32) datapack.Spot {
		s.Maker, s.Center, s.MaximumNPCs = name, datapack.Point{X: x, Y: y}, 11
		return s
	}
	wolves, orcs := place(tiWolves, "wolves", 1000, 0), place(tiOrcs01, "orcs", 0, 3000)
	tests := []struct {
		name     string
		summoner bool
		out      bool
		// dead kills the servitor out with a re-summon ready: the spot
		// stays (docs/servitors.md section 3).
		dead bool
		// want is the spot held, -1 for none.
		want int
	}{
		{"fighter alone: the orcs", false, false, false, 1},
		{"fighter with Kai: the werewolves", false, true, false, 0},
		{"summoner alone: none", true, false, false, -1},
		{"summoner with Kai: the werewolves", true, true, false, 0},
		{"summoner, Kai dead, summonable: the werewolves", true, true, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newSpotBrain(t, 14, wolves, orcs)
			b.persona.Potions = []int32{lesserHealingPotion}
			setSquire(b, squire14, 3)
			withServitorTemplates(b)
			if tt.summoner {
				summonerKit(b)
			}
			if tt.out {
				kaiOut(b).Dead = tt.dead
			}
			if tt.dead {
				b.summonReady = summonKai1
			}
			b.decide(at(time.Second))
			got := -1
			if b.spot.held {
				got = b.spot.idx
			}
			if got != tt.want {
				t.Fatalf("spot %d, want %d", got, tt.want)
			}
		})
	}
}

// TestServitorAttackersNotTheBots checks a monster hitting the servitor is
// no attacker of the bot's: Crowded and Outmatched, which count the bot's
// attackers, judge the bot's flight by the hits on the bot alone
// (docs/servitors.md section 3, "Pack threat").
func TestServitorAttackersNotTheBots(t *testing.T) {
	const wolf = int32(0x20000001)
	b := newSquireBrain(t, squire14, 0)
	withServitorTemplates(b)
	kaiOut(b)
	b.world.Apply(Attack{AttackerID: wolf, Pos: Point{X: 40}, Hits: []AttackHit{{TargetID: kaiObject, Damage: 50}}}, t0)
	if len(b.world.Attackers) != 0 {
		t.Fatalf("Attackers = %v after a hit on the servitor, want none", b.world.Attackers)
	}
	b.world.Apply(Attack{AttackerID: wolf, Pos: Point{X: 40}, Hits: []AttackHit{{TargetID: b.world.Self.ObjectID, Damage: 50}}}, t0)
	if !b.world.attacking(wolf, t0) {
		t.Fatal("a hit on the bot is no attacker")
	}
}
