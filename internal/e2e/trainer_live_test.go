package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveTrainer runs a human mystic in the Talking Island village next to
// its trainers. Set to level 15 with SP, it talks to a trainer, opens its
// skill list and learns Heal; when the trainer asks for Heal's spellbook
// the GM gives it, the bot asks again and the server takes the book (#21).
// Set to level 20 with SP again, it goes back and learns Heal's level 7,
// offered from level 20 on (#39). The GM sets the mystic back to level 1,
// which takes Heal away, at the start and at the end.
func TestLiveTrainer(t *testing.T) {
	tg := Live(t, NeedGM, NeedDatapack)
	ctx := scenarioCtx(t, 20*time.Minute)
	travel, _ := townData(t, tg)
	gm := enterGM(t, ctx, tg)
	account, _ := tg.Account("t", 1)
	f := runTownBot(t, ctx, tg, account, &botclient.HumanMystic, travel, func(p *botclient.Persona) {
		// No merchant visits: the GM supplies the spellbook.
		p.Shop = botclient.ShopPolicy{}
	})
	t.Cleanup(func() { gmReset(t, gm, f) })

	// Level 1 forgets Heal (the server drops the skills a level no longer
	// allows, skill/leveling.go correctInvalidSkills); the visit the bot
	// may make at level 1 finds nothing to learn.
	gmOn(t, ctx, gm, f, "set level 1", "set sp 0")
	gmRecall(t, ctx, gm, f, tiTrainers)
	forgot, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	until(t, forgot, f.name+" forgets Heal at level 1", func() string {
		if lv := skillLevel(f, skillHeal); lv != 0 {
			return fmt.Sprintf("SkillList shows Heal level %d", lv)
		}
		return ""
	})
	from := len(f.watch.Sent())
	gmOn(t, ctx, gm, f, fmt.Sprintf("set sp %d", trainerSP), "set level 15")
	learn(t, ctx, gm, f, from, 1)

	from = len(f.watch.Sent())
	gmOn(t, ctx, gm, f, "set level 20", fmt.Sprintf("set sp %d", trainerSP))
	learn(t, ctx, gm, f, from, healAt20)
}

// learn waits until f's SkillList shows Heal at level or higher, learned
// by a RequestAcquireSkill f sent from index from of its sent packets on.
// The GM gives the spellbook once a trainer's AcquireSkillInfo asks for it,
// and the server then takes it.
func learn(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, from int, level int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	booked := false
	until(t, ctx, fmt.Sprintf("%s learns Heal level %d", f.name, level), func() string {
		if !booked && asksBook(f) {
			t.Logf("%s's trainer asks for Spellbook: Heal", f.name)
			gmOn(t, ctx, gm, f)
			gmGive(t, ctx, gm, f, spellbookHeal, 1)
			booked = true
		}
		asked := false
		for _, p := range sentOf(f.watch.Sent()[from:], botclient.OpRequestAcquireSkill) {
			asked = asked || p.Target == skillHeal && len(p.Payload) >= 9 && int32(binary.LittleEndian.Uint32(p.Payload[5:9])) >= level
		}
		switch lv := skillLevel(f, skillHeal); {
		case !asked:
			return fmt.Sprintf("no RequestAcquireSkill for Heal level %d or higher (rule %q, knows level %d)", level, f.bot.Status().Rule, lv)
		case lv < level:
			return fmt.Sprintf("SkillList shows Heal level %d", lv)
		}
		return ""
	})
	if booked {
		if n := carriedOf(inventory(f.watch.Seen(0)), spellbookHeal); n != 0 {
			t.Errorf("%s still carries %d Spellbook: Heal after learning", f.name, n)
		}
	}
	t.Logf("%s learned Heal level %d (rule %q)", f.name, skillLevel(f, skillHeal), f.bot.Status().Rule)
}

// asksBook reports whether a trainer told f that learning Heal takes its
// spellbook.
func asksBook(f *farmer) bool {
	for _, s := range f.watch.Seen(0) {
		if info, ok := s.Event.(botclient.AcquireSkillInfo); ok && info.ID == skillHeal {
			for _, r := range info.Requirements {
				if r.ItemID == spellbookHeal {
					return true
				}
			}
		}
	}
	return false
}

// skillLevel is the level of skill id f's last SkillList shows, 0 for none.
func skillLevel(f *farmer, id int32) int32 {
	var level int32
	for _, s := range f.watch.Seen(0) {
		if l, ok := s.Event.(botclient.SkillList); ok {
			level = 0
			for _, k := range l.Skills {
				if k.ID == id {
					level = k.Level
				}
			}
		}
	}
	return level
}

// gmReset sets f back to level 1 with no SP once the scenario ends, so the
// next run finds Heal to learn again.
func gmReset(t *testing.T, gm *GMClient, f *farmer) {
	if gm.Closed() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := gm.Command(ctx, "teleportto "+f.name); err != nil {
		t.Logf("cleanup: GM to %s: %v", f.name, err)
		return
	}
	if _, err := gm.Await(ctx, EntryTimeout, charInfoOf(f.id)); err != nil {
		t.Logf("cleanup: GM to %s: %v", f.name, err)
		return
	}
	if err := gm.Select(ctx, f.id); err != nil {
		t.Logf("cleanup: select %s: %v", f.name, err)
		return
	}
	for _, cmd := range []string{"set level 1", "set sp 0"} {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Logf("cleanup: %s on %s: %v", cmd, f.name, err)
		}
	}
}
