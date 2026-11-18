package e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestLiveTrainer runs a human mystic in the Talking Island village next to
// its trainers. Set to level 10 with SP, it talks to a trainer and opens
// its skill list, which offers no Heal level it lacks (on the run that
// creates the character, Heal 1-3, whose spellbook the GM gives when the
// trainer asks for it; the server takes the book with the learn). Set to
// level 15, it goes back and learns Heal's level 4, offered from level 14
// on (#21, #39). The GM sets the mystic back to level 1, which pulls Heal
// down to level 3, at the start and at the end.
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

	// Level 1 pulls Heal down to level 3 (healAt1); the visit the bot may
	// make at level 1 finds nothing to learn.
	gmOn(t, ctx, gm, f, "set level 1", "set sp 0")
	gmTeleport(t, ctx, gm, tiTrainers)
	recallBot(t, ctx, tg, gm, f.name)
	forgot, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	until(t, forgot, f.name+" keeps Heal 3 at most at level 1", func() string {
		if lv := skillLevel(f, skillHeal); lv > healAt1 {
			return fmt.Sprintf("SkillList shows Heal level %d", lv)
		}
		return ""
	})

	// Level 10: a visit, and Heal 1-3 learned when the bot lacks them.
	seen := f.watch.Len()
	gmOn(t, ctx, gm, f, fmt.Sprintf("set sp %d", trainerSP), "set level 10")
	vctx, vcancel := context.WithTimeout(ctx, 6*time.Minute)
	defer vcancel()
	asked := false
	until(t, vctx, f.name+"'s visit at level 10", func() string {
		spellbook(t, ctx, gm, f, &asked)
		listed := slices.ContainsFunc(f.watch.Seen(seen), func(s Seen) bool {
			_, ok := s.Event.(botclient.AcquireSkillList)
			return ok
		})
		switch rule := f.bot.Status().Rule; {
		case !listed:
			return fmt.Sprintf("no AcquireSkillList (rule %q)", rule)
		case rule == "train":
			return "the visit goes on"
		}
		return ""
	})
	t.Logf("%s visited at level 10: knows Heal level %d (book asked %v)", f.name, skillLevel(f, skillHeal), asked)
	if lv := skillLevel(f, skillHeal); lv != healAt1 {
		t.Fatalf("%s knows Heal level %d after the level 10 visit, want %d", f.name, lv, healAt1)
	}

	// Level 15: Heal 4 is offered now, and learned.
	from := len(f.watch.Sent())
	gmOn(t, ctx, gm, f, "set level 15", fmt.Sprintf("set sp %d", trainerSP))
	learn(t, ctx, gm, f, from, healAt15)
}

// spellbook gives f Heal's spellbook once a trainer's AcquireSkillInfo
// asked for it (Heal's first level) while f carries none, and records the
// ask in asked.
func spellbook(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, asked *bool) {
	t.Helper()
	if *asked || !asksBook(f) {
		return
	}
	*asked = true
	if carriedOf(inventory(f.watch.Seen(0)), spellbookHeal) == 0 {
		t.Logf("%s's trainer asks for Spellbook: Heal", f.name)
		gmOn(t, ctx, gm, f)
		gmGiveGear(t, ctx, gm, f, spellbookHeal, 1)
	}
}

// learn waits until f's SkillList shows Heal at level or higher, learned
// by a RequestAcquireSkill f sent from index from of its sent packets on.
func learn(t *testing.T, ctx context.Context, gm *GMClient, f *farmer, from int, level int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	fromSeen := f.watch.Len()
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(trainerTrail(f, fromSeen, from))
		}
	})
	until(t, ctx, fmt.Sprintf("%s learns Heal level %d", f.name, level), func() string {
		learned := false
		for _, p := range sentOf(f.watch.Sent()[from:], botclient.OpRequestAcquireSkill) {
			learned = learned || p.Target == skillHeal && len(p.Payload) >= 9 && int32(binary.LittleEndian.Uint32(p.Payload[5:9])) >= level
		}
		switch lv := skillLevel(f, skillHeal); {
		case !learned:
			return fmt.Sprintf("no RequestAcquireSkill for Heal level %d or higher (rule %q, knows level %d)", level, f.bot.Status().Rule, lv)
		case lv < level:
			return fmt.Sprintf("SkillList shows Heal level %d", lv)
		}
		return ""
	})
	t.Logf("%s learned Heal level %d (rule %q)", f.name, skillLevel(f, skillHeal), f.bot.Status().Rule)
}

// asksBook reports whether a trainer told f that learning Heal's first
// level takes its spellbook.
func asksBook(f *farmer) bool {
	for _, s := range f.watch.Seen(0) {
		if info, ok := s.Event.(botclient.AcquireSkillInfo); ok && info.ID == skillHeal && info.Level == 1 {
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
// next run finds Heal 4 to learn again.
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

// trainerTrail lists, for a failed visit, what f sent from index sentFrom
// on and the trainer's answers it read from index seenFrom on.
func trainerTrail(f *farmer, seenFrom, sentFrom int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s's trainer trail:", f.name)
	for _, p := range f.watch.Sent()[sentFrom:] {
		fmt.Fprintf(&b, "\n  %s sent %#02x %d", p.At.Format("15:04:05.000"), p.Op, p.Target)
	}
	for _, s := range f.watch.Seen(seenFrom) {
		switch e := s.Event.(type) {
		case botclient.NpcHtmlMessage:
			fmt.Fprintf(&b, "\n  %s read NpcHtmlMessage %d %.120q", s.At.Format("15:04:05.000"), e.ObjectID, e.HTML)
		case botclient.AcquireSkillList, botclient.AcquireSkillInfo, botclient.AcquireSkillDone, botclient.SystemMessage, botclient.ActionFailed:
			fmt.Fprintf(&b, "\n  %s read %T%+v", s.At.Format("15:04:05.000"), e, e)
		case botclient.UserInfo:
			fmt.Fprintf(&b, "\n  %s read UserInfo level %d SP %d", s.At.Format("15:04:05.000"), e.Level, e.SP)
		}
	}
	return b.String()
}
