package e2e

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// leatherBoots are D-grade boots (items/0000-0099.xml): over the grade of
// a character without Expertise.
const leatherBoots int32 = 40

// gradePenaltySkill is the grade penalty's skill, level 1 while it holds
// (character_grade_penalty.go).
const gradePenaltySkill int32 = 4267

// TestLiveGradePenalty runs a level 1 fighter that logs in wearing D-grade
// boots over its starting set (#436): the server holds the grade penalty
// on it (skill 4267 in its SkillList, the boots slowing it), the bot takes
// the boots off, the server lifts the penalty, and its snapshot shows the
// penalty gone and its load.
func TestLiveGradePenalty(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM, NeedDatapack)
	alone(t, &townNPCs)
	ctx := scenarioCtx(t, 15*time.Minute)
	travel, items := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("gp", 1)
	strip(t, ctx, tg, gm, items, account, &botclient.HumanFighter, stripEnd{level: 1}, squiresSword, squiresShirt, squiresPants, leatherBoots)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanFighter, travel, func(p *botclient.Persona) {
		fighterGear(p)
		// Wearing only: what it carries, never what it could buy or what
		// other players leave on the town's ground (their gear would
		// weigh it into a weight band).
		p.Gear.Buy, p.Shop, p.Loot.Ground = false, botclient.ShopPolicy{}, botclient.GroundNone
	})
	toTown(t, ctx, tg, gm, f)

	wears(t, ctx, f, items, "its starting set, the D-grade boots off", func(w map[int32]int) bool {
		return w[leatherBoots] == 0 && w[squiresSword] == 1 && w[squiresShirt] == 1 && w[squiresPants] == 1
	})
	held, lifted := false, false
	for _, s := range f.watch.Seen(0) {
		l, ok := s.Event.(botclient.SkillList)
		if !ok {
			continue
		}
		pen := slices.ContainsFunc(l.Skills, func(k botclient.KnownSkill) bool { return k.ID == gradePenaltySkill && k.Level > 0 })
		held = held || pen
		lifted = held && !pen
	}
	if !held || !lifted {
		t.Fatalf("%s: grade penalty held %v, then lifted %v, want both (SkillList skill %d)", f.name, held, lifted, gradePenaltySkill)
	}
	until(t, ctx, f.name+" publishes no grade penalty and its load", func() string {
		if st := f.bot.Status(); st.GradePenalty || st.MaxLoad <= 0 || st.WeightPenalty != 0 {
			return fmt.Sprintf("grade penalty %v, load %d/%d (band %d)", st.GradePenalty, st.Load, st.MaxLoad, st.WeightPenalty)
		}
		return ""
	})
}
