package e2e

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// The support skills a human mystic learns by level 15 (skill ids of the
// datapack's skills XML), each seeded at level 1.
const (
	skillHeal       int32 = 1011
	skillBattleHeal int32 = 1015
	skillShield     int32 = 1040
	skillMight      int32 = 1068
)

// buffTime is the datapack's duration of Might and Shield at level 1.
const buffTime = 20 * time.Minute

// supportRebuff is the scenario's RebuffBefore: a minute into a buff the
// bot renews it, so the renewal shows in minutes, not in the twenty of the
// default (a minute before the end).
const supportRebuff = buffTime - time.Minute

// vukuOrc is a level 14 monster of the datapack that only fights back
// (data/xml/npcs/20000-20999.xml, id 20009): it hurts a level 15 fighter
// over a few blows.
const vukuOrc int32 = 20009

// bareSelection is how long the scenario watches the bot ignore a monster
// the leader only selected: past the 1.5 s an assist would take.
const bareSelection = 4 * time.Second

// casts records the casts and attacks a client sees, with when.
type casts struct {
	seen []castSeen
}

type castSeen struct {
	at  time.Time
	ev  botclient.Event
	who int32
}

func (c *casts) note(ev botclient.Event) {
	switch e := ev.(type) {
	case botclient.MagicSkillUse:
		c.seen = append(c.seen, castSeen{at: time.Now(), ev: e, who: e.CasterID})
	case botclient.Attack:
		c.seen = append(c.seen, castSeen{at: time.Now(), ev: e, who: e.AttackerID})
	case botclient.MoveToPawn:
		c.seen = append(c.seen, castSeen{at: time.Now(), ev: e, who: e.ObjectID})
	}
}

// first is the first noted event of caster that match accepts.
func (c *casts) first(caster int32, match func(botclient.Event) bool) (castSeen, bool) {
	for _, s := range c.seen {
		if s.who == caster && match(s.ev) {
			return s, true
		}
	}
	return castSeen{}, false
}

// all are the noted events of caster that match accepts.
func (c *casts) all(caster int32, match func(botclient.Event) bool) []castSeen {
	var out []castSeen
	for _, s := range c.seen {
		if s.who == caster && match(s.ev) {
			out = append(out, s)
		}
	}
	return out
}

// on matches an attack, cast or approach aimed at target.
func on(target int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		switch e := ev.(type) {
		case botclient.MagicSkillUse:
			return e.TargetID == target
		case botclient.Attack:
			return slices.ContainsFunc(e.Hits, func(h botclient.AttackHit) bool { return h.TargetID == target })
		case botclient.MoveToPawn:
			return e.TargetID == target
		}
		return false
	}
}

// hits matches a swing at target.
func hits(target int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		a, ok := ev.(botclient.Attack)
		return ok && on(target)(a)
	}
}

func skill(id, target int32) func(botclient.Event) bool {
	return func(ev botclient.Event) bool {
		m, ok := ev.(botclient.MagicSkillUse)
		return ok && m.SkillID == id && m.TargetID == target
	}
}

// TestLiveSupport has a level 15 human mystic bot, seeded by GM commands
// with Heal, Battle Heal, Might and Shield, support a player that leads its
// party: it buffs the leader and itself, leaves a monster the leader only
// selected be, assists the leader's attack on it 0.5 to 1.5 s after the
// first swing, heals the leader once the monster took it under 60 % HP, and
// renews its buffs once under RebuffBefore (#22).
func TestLiveSupport(t *testing.T) {
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 10*time.Minute)
	gm := enterGM(t, ctx, tg)
	acc, _ := tg.Account("su", 1)
	leader := enter(t, ctx, tg, acc, character(t, acc, &botclient.HumanFighter), nil)
	account, _ := tg.Account("su", 2)
	b := runBotWith(t, ctx, tg, account, botSetup{
		class: &botclient.HumanMystic,
		persona: func() botclient.Persona {
			p := partyPersona()
			p.RebuffBefore = supportRebuff
			return p
		},
	})
	name := botOnline(t, ctx, tg, b)
	recallClient(t, ctx, gm, leader)
	botID := recallBot(t, ctx, tg, gm, name)

	if err := gm.Select(ctx, botID); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"set level 15", "skill set 1011 1", "skill set 1015 1", "skill set 1068 1", "skill set 1040 1"} {
		if err := gm.Command(ctx, cmd); err != nil {
			t.Fatal(err)
		}
	}
	watchUntil(t, ctx, gm.Client, "the bot at level 15", func() bool { return b.Status().Level >= 15 }, nil)

	// The leader at level 15 too, so the orc it fights takes it under 60 %
	// HP over a few blows rather than killing it in two.
	self := leader.Self.ObjectID
	if err := gm.Select(ctx, self); err != nil {
		t.Fatal(err)
	}
	if err := gm.Command(ctx, "set level 15"); err != nil {
		t.Fatal(err)
	}
	var seen casts
	var hp, maxHP int32
	var hurtAt time.Time
	track := func(ev botclient.Event) {
		seen.note(ev)
		switch e := ev.(type) {
		case botclient.UserInfo:
			hp, maxHP = e.HP, e.MaxHP
		case botclient.StatusUpdate:
			if e.ObjectID != self {
				return
			}
			if e.Set&botclient.StatusHP != 0 {
				hp = e.HP
			}
			if e.Set&botclient.StatusMaxHP != 0 {
				maxHP = e.MaxHP
			}
		default:
			return
		}
		if hurtAt.IsZero() && maxHP > 0 && hp*100 < maxHP*60 {
			hurtAt = time.Now()
		}
	}
	ui := userInfo(t, leader)
	hp, maxHP = ui.HP, ui.MaxHP

	if r := invite(t, ctx, leader, name); r != 1 {
		t.Fatalf("%s answered %d, want 1", name, r)
	}
	buffs := []func(botclient.Event) bool{skill(skillMight, self), skill(skillShield, self), skill(skillMight, botID), skill(skillShield, botID)}
	watchUntil(t, ctx, leader, "Might and Shield on the leader and the bot", func() bool {
		for _, m := range buffs {
			if _, ok := seen.first(botID, m); !ok {
				return false
			}
		}
		return true
	}, track)
	might, _ := seen.first(botID, skill(skillMight, self))
	if maxHP < 200 {
		t.Fatalf("leader at %d/%d HP, want its level 15 HP", hp, maxHP)
	}

	// The leader only selects an orc: the bot leaves it be.
	mob, err := gm.Spawn(ctx, vukuOrc)
	if err != nil {
		t.Fatal(err)
	}
	if err := leader.Send(ctx, botclient.EncodeAction(mob, leader.Pos())); err != nil {
		t.Fatal(err)
	}
	quiet := time.Now().Add(bareSelection)
	watchUntil(t, ctx, leader, "the bare selection window", func() bool { return time.Now().After(quiet) }, track)
	if s, ok := seen.first(botID, on(mob)); ok {
		t.Fatalf("%s went for the orc on the leader's bare selection: %+v", name, s.ev)
	}

	// The leader attacks: the bot follows 0.5 to 1.5 s later.
	if err := leader.Send(ctx, botclient.EncodeAttackRequest(mob, leader.Pos())); err != nil {
		t.Fatal(err)
	}
	var attackAt time.Time
	assisted := func() time.Time { return firstAfter(seen.all(botID, on(mob)), attackAt) }
	watchUntil(t, ctx, leader, "the bot assisting", func() bool {
		if attackAt.IsZero() {
			if a, ok := seen.first(self, hits(mob)); ok {
				attackAt = a.at
			}
		}
		return !attackAt.IsZero() && !assisted().IsZero()
	}, track)
	if d := assisted().Sub(attackAt); d < 400*time.Millisecond || d > 5*time.Second {
		t.Fatalf("%s went for the orc %v after the leader's first swing, want 0.5 to 1.5 s and a reaction", name, d)
	} else {
		t.Logf("%s went for the orc %v after the leader's first swing", name, d)
	}

	// The orc hits back: once the leader is under 60 % HP the bot heals it.
	heal := func(ev botclient.Event) bool { return skill(skillHeal, self)(ev) || skill(skillBattleHeal, self)(ev) }
	watchUntil(t, ctx, leader, "the leader under 60 % HP and the bot healing it", func() bool {
		if hurtAt.IsZero() {
			return false
		}
		return !firstAfter(seen.all(botID, heal), hurtAt).IsZero()
	}, track)
	if d := firstAfter(seen.all(botID, heal), hurtAt).Sub(hurtAt); d > 15*time.Second {
		t.Fatalf("heal %v after the leader fell under 60 %%", d)
	} else {
		t.Logf("leader under 60 %% HP, healed %v later", d)
	}
	if err := gm.Delete(ctx, mob); err != nil {
		t.Fatal(err)
	}
	if err := gm.Command(ctx, "heal "+leader.Self.Name); err != nil {
		t.Fatal(err)
	}

	// Renewal: a second Might on the leader once a minute of it ran.
	watchUntil(t, ctx, leader, "Might renewed on the leader", func() bool {
		return len(seen.all(botID, skill(skillMight, self))) >= 2
	}, track)
	again := seen.all(botID, skill(skillMight, self))[1]
	if gap := again.at.Sub(might.at); gap < buffTime-supportRebuff-10*time.Second {
		t.Fatalf("Might renewed %v after the first, want about %v", gap, buffTime-supportRebuff)
	} else {
		t.Logf("Might renewed %v after the first", gap)
	}
}

// firstAfter is when the first of seen at or after t was seen; zero for
// none.
func firstAfter(seen []castSeen, t time.Time) time.Time {
	for _, s := range seen {
		if !s.at.Before(t) {
			return s.at
		}
	}
	return time.Time{}
}
