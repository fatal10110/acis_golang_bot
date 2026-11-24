package e2e

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveBranchHash takes two new human fighters to their first profession
// at level 20 with no branch pinned (D22, #328): each takes the branch the
// hash of its account name and role draws (botclient.BranchChoice), and the
// two accounts are picked so their draws differ, a Warrior and a Rogue, so
// the class each becomes is the hash's and not a default's (#333). A class
// change cannot be undone, so every run starts from new characters; the
// class master missing at its spawn (fatal10110/acis_golang#5419) fails the
// scenario.
func TestLiveBranchHash(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 25*time.Minute)
	accounts := hashedAccounts(t, tg, "bh", botclient.HumanFighter.ID)
	travel := travelMap(t, tg)
	gm := enterGM(t, ctx, tg)
	masterAt(t, ctx, gm)

	type pick struct {
		f      *farmer
		branch int32
	}
	var picks []pick
	for _, branch := range slices.Sorted(maps.Keys(accounts)) {
		account := accounts[branch]
		f := runWatched(t, ctx, tg, account, botSetup{
			class: &botclient.HumanFighter,
			persona: func() botclient.Persona {
				p := botclient.DefaultPersona()
				p.IdleChance = 0
				errandsOff(&p)
				// The class change on, its line the build's: no table of
				// the persona's (Builds nil) and no pin (BrainConfig.Branch).
				p.ClassChange = botclient.DefaultPersona().ClassChange
				return p
			},
			brain: withTravel(travel),
		})
		t.Cleanup(func() {
			if t.Failed() {
				t.Log(f.digest())
			}
		})
		t.Logf("%s's account hash draws branch %d", f.name, branch)
		picks = append(picks, pick{f: f, branch: branch})
	}
	for _, p := range picks {
		recallBot(t, ctx, tg, gm, p.f.name)
		levelTo(t, ctx, gm, p.f, 20)
	}
	for _, p := range picks {
		becomes(t, ctx, p.f, p.branch)
		if b := p.f.bot.Status().Build; b == nil || b.Branch != p.branch {
			t.Errorf("%s's build: %+v, want branch %d", p.f.name, b, p.branch)
		}
	}
}

// hashedAccounts are new accounts of group, one for each of two first
// professions the default persona's role draws for base class (the Damage
// table names more than one for the human fighter), by first profession.
// Their numbers start from the clock, as freshAccount's, so every run makes
// new characters.
func hashedAccounts(t *testing.T, tg *Target, group string, base int32) map[int32]string {
	t.Helper()
	race := botclient.RaceHuman
	role := botclient.DefaultPersona().Role
	pins := botclient.BranchPins(base)
	accounts := make(map[int32]string)
	start := int(time.Now().Unix() % 10_000_000)
	for n := start; n < start+1000 && len(accounts) < 2; n++ {
		account, _ := tg.Account(group, n)
		b := botclient.CompileBuild(race, base, 1, role, botclient.BranchChoice{Account: account}).Branch
		if !slices.Contains(pins, b) {
			t.Fatalf("account %s draws branch %d, not one of class %d's %v", account, b, base, pins)
		}
		if _, ok := accounts[b]; !ok {
			accounts[b] = account
		}
	}
	if len(accounts) < 2 {
		t.Fatalf("no two accounts of 1000 draw different branches of class %d: %v", base, accounts)
	}
	return accounts
}
