package e2e

import (
	"context"
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

// supportRebuff is the scenario's RebuffBefore: four minutes into a buff
// the bot renews it, so the renewal shows in minutes, not in the twenty of
// the default (a minute before the end), and falls due only after the
// assist and heal phases: a bot casting a renewal when the leader swings
// would assist late.
const supportRebuff = buffTime - 4*time.Minute

// vukuOrc is a level 14 monster of the datapack that only fights back
// (data/xml/npcs/20000-20999.xml, id 20009): it hurts a level 15 fighter
// over a few blows.
const vukuOrc int32 = 20009

// bareSelection is how long the scenario watches the bot ignore a monster
// the leader only selected: past the 1.5 s an assist would take.
const bareSelection = 4 * time.Second

// castQuiet is how long the bot has cast nothing before the leader swings,
// so no cast in flight delays the assist being timed.
const castQuiet = 3 * time.Second

// The assist bound: the persona's 0.5 to 1.5 s delay widened for the live
// server's latency on the swing, the bot's reaction and the observer.
const (
	assistMin = 400 * time.Millisecond
	assistMax = 5 * time.Second
)

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
	case botclient.Die:
		c.seen = append(c.seen, castSeen{at: time.Now(), ev: e, who: e.ObjectID})
	case botclient.Revive:
		c.seen = append(c.seen, castSeen{at: time.Now(), ev: e, who: e.ObjectID})
	}
}

// death matches a Die or a Revive.
func death(ev botclient.Event) bool {
	switch ev.(type) {
	case botclient.Die, botclient.Revive:
		return true
	}
	return false
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

// last is when caster's last noted event that match accepts was seen;
// zero for none.
func (c *casts) last(caster int32, match func(botclient.Event) bool) time.Time {
	var at time.Time
	for _, s := range c.seen {
		if s.who == caster && match(s.ev) {
			at = s.at
		}
	}
	return at
}

// between logs caster's noted events from from to to.
func (c *casts) between(t *testing.T, caster int32, from, to time.Time) {
	t.Helper()
	for _, s := range c.seen {
		if s.who == caster && !s.at.Before(from) && !s.at.After(to) {
			t.Logf("%v: %T %+v", s.at.Sub(from), s.ev, s.ev)
		}
	}
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

// anyCast matches any cast.
func anyCast(ev botclient.Event) bool {
	_, ok := ev.(botclient.MagicSkillUse)
	return ok
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
// selected be, assists the leader's attack on it 0.5 to 1.5 s (the
// persona's delay; assistMin to assistMax live) after the first swing, heals the leader once the monster took it under 60 % HP, and
// renews its buffs once under RebuffBefore (#22).
func TestLiveSupport(t *testing.T) {
	t.Parallel()
	tg := Live(t, NeedGM)
	ctx := scenarioCtx(t, 12*time.Minute)
	gm := enterGM(t, ctx, tg)
	// The leader, the bot and the orc meet at the GM: a pooled GM stands
	// wherever its last run left it, ground the mystic may not path across
	// to the orc (#367's live run: a ledge at the dwarven start).
	gmToOpenGround(t, ctx, gm)
	// The leader has an account of its own: e2esu1 is TestLiveSupplyTrip's
	// farmer (#252), which that run strips, levels to 6, loads and gets
	// killed. As this leader it swung every 3.8 s for 0 to 2 damage, so
	// the mystic's 2 or 3 drew the orc, which killed the bot and then the
	// leader before any heal (#282); and two runs at once would drop each
	// other's login.
	acc, _ := tg.Account("su", 3)
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

	// Effects outlast a logout: a bot or leader that kept Might and Shield
	// from an earlier run would renew them on that run's clock, minutes
	// into this one. Cancel them, then watch the bot cast both on itself,
	// from here on, through the GM (it sees the bot recalled to it).
	var seen casts
	start := time.Now()
	self := leader.Self.ObjectID
	// The leader's HP and when the orc first took it under 60 %, the orc
	// and its HP as the leader sees them: the failure digest logs them.
	var (
		hp, maxHP       int32
		hurtAt          time.Time
		mob             int32
		mobHP, mobMaxHP int32
	)
	defer func() {
		if !t.Failed() {
			return
		}
		now := time.Now()
		st := b.Status()
		t.Logf("bot: state %v level %d HP %d/%d MP %d/%d rule %q leader %q members %v, %.0f from the leader",
			st.State, st.Level, st.HP, st.MaxHP, st.MP, st.MaxMP, st.Rule, st.Leader, st.Members, dist(st.Position, leader.Pos()))
		seen.between(t, botID, start, now)
		t.Logf("leader: HP %d/%d, under 60 %% at %v", hp, maxHP, hurtAt.Sub(start))
		for _, s := range seen.all(self, death) {
			t.Logf("leader %v: %T", s.at.Sub(start), s.ev)
		}
		if mob != 0 {
			t.Logf("orc %d: HP %d/%d", mob, mobHP, mobMaxHP)
			seen.between(t, mob, start, now)
			for _, s := range seen.all(self, hits(mob)) {
				t.Logf("leader %v: %+v", s.at.Sub(start), s.ev)
			}
		}
	}()
	// A target command runs on the GM itself when the server cleared its
	// target, as it may a moment after the recall (#206): each one checks
	// that the bot is still the GM's target first.
	if err := gm.Select(ctx, botID); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"cancel " + name, "set level 15", "skill set 1011 1", "skill set 1015 1", "skill set 1068 1", "skill set 1040 1"} {
		if err := gm.OnTargetWatch(ctx, botID, cmd, seen.note); err != nil {
			t.Fatal(err)
		}
	}
	watchUntil(t, ctx, gm.Client, "the bot at level 15", func() bool { return b.Status().Level >= 15 }, seen.note)
	// Healed only once at level 15, so to a full bar of the level 15 MP:
	// with the MP of its old level the bot sits out its buffs resting.
	if err := gm.Command(ctx, "heal "+name); err != nil {
		t.Fatal(err)
	}
	watchUntil(t, ctx, gm.Client, "Might and Shield on the bot itself", func() bool {
		_, might := seen.first(botID, skill(skillMight, botID))
		_, shield := seen.first(botID, skill(skillShield, botID))
		return might && shield
	}, seen.note)
	st := b.Status()
	t.Logf("bot buffed itself: MP %d/%d rule %q", st.MP, st.MaxMP, st.Rule)

	// The leader at level 15 too, so the orc it fights takes it under 60 %
	// HP over a few blows rather than killing it in two.
	if err := gm.Select(ctx, self); err != nil {
		t.Fatal(err)
	}
	// Full HP too: a leader that entered dead restarts in the village
	// with part of it, and the heal phase starts from a full bar.
	for _, cmd := range []string{"cancel " + leader.Self.Name, "set level 15", "heal " + leader.Self.Name} {
		if err := gm.OnTargetWatch(ctx, self, cmd, seen.note); err != nil {
			t.Fatal(err)
		}
	}
	track := func(ev botclient.Event) {
		seen.note(ev)
		switch e := ev.(type) {
		case botclient.UserInfo:
			hp, maxHP = e.HP, e.MaxHP
		case botclient.StatusUpdate:
			if e.ObjectID == mob && mob != 0 {
				if e.Set&botclient.StatusHP != 0 {
					mobHP = e.HP
				}
				if e.Set&botclient.StatusMaxHP != 0 {
					mobMaxHP = e.MaxHP
				}
				return
			}
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
	spawned, err := gm.Spawn(ctx, vukuOrc)
	if err != nil {
		t.Fatal(err)
	}
	mob = spawned
	// Registered after the leader's logout, this runs first: a failure
	// before the Delete below leaves no orc hitting the leader as it logs
	// out.
	t.Cleanup(func() {
		if !slices.Contains(gm.Spawned(), mob) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := gm.Delete(ctx, mob); err != nil {
			t.Logf("cleanup: delete the orc %d: %v", mob, err)
		}
	})
	if err := leader.Send(ctx, botclient.EncodeAction(mob, leader.Pos())); err != nil {
		t.Fatal(err)
	}
	quiet := time.Now().Add(bareSelection)
	watchUntil(t, ctx, leader, "the bare selection window", func() bool { return time.Now().After(quiet) }, track)
	if s, ok := seen.first(botID, on(mob)); ok {
		t.Fatalf("%s went for the orc on the leader's bare selection: %+v", name, s.ev)
	}

	// The leader attacks once the bot has cast nothing for castQuiet: the
	// bot follows 0.5 to 1.5 s later.
	watchUntil(t, ctx, leader, "the bot done casting", func() bool {
		return time.Since(seen.last(botID, anyCast)) >= castQuiet
	}, track)
	if left := time.Until(might.at.Add(buffTime - supportRebuff)); left < time.Minute {
		t.Fatalf("the renewal falls due %v after the leader's swing: too close to time the assist", left)
	}
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
	if d := assisted().Sub(attackAt); d < assistMin || d > assistMax {
		seen.between(t, botID, attackAt.Add(-castQuiet), assisted())
		t.Fatalf("%s went for the orc %v after the leader's first swing, want %v to %v", name, d, assistMin, assistMax)
	} else {
		t.Logf("%s went for the orc %v after the leader's first swing", name, d)
	}

	// The orc hits back: once the leader is under 60 % HP the bot heals it.
	// A death on the way fails at once rather than at the deadline: the
	// bot's or the leader's leaves no heal to see, and the orc's before the
	// leader is hurt leaves nothing to hurt it.
	heal := func(ev botclient.Event) bool { return skill(skillHeal, self)(ev) || skill(skillBattleHeal, self)(ev) }
	died := func(ev botclient.Event) bool { _, ok := ev.(botclient.Die); return ok }
	watchUntil(t, ctx, leader, "the leader under 60 % HP and the bot healing it", func() bool {
		for _, who := range []int32{botID, self, mob} {
			if at := firstAfter(seen.all(who, died), attackAt); !at.IsZero() && (who != mob || hurtAt.IsZero()) {
				t.Fatalf("%d died %v after the leader's first swing, before the heal", who, at.Sub(attackAt))
			}
		}
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

	// Renewal: a second Might on the leader once four minutes of it ran.
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
