package botpanel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// Body part masks of ItemList (botclient's bodyParts).
const (
	maskRHand     = 0x0080
	maskChest     = 0x0400
	maskFullArmor = 0x8000
	maskHead      = 0x0040
	maskLegs      = 0x0800
)

// testCatalog is a catalog of the items and skills the inspect tests use.
func testCatalog() *Catalog {
	return NewCatalog(&datapack.Datapack{
		Items: map[int32]datapack.Item{
			2:   {ID: 2, Name: "Long Sword", Type: "Weapon", BodyPart: "rhand", Grade: 0, Weight: 1560},
			70:  {ID: 70, Name: "Claymore", Type: "Weapon", BodyPart: "lrhand", Grade: 2, Weight: 1600},
			356: {ID: 356, Name: "Full Plate Armor", Type: "Armor", BodyPart: "fullarmor", Grade: 2, Weight: 10000},
			500: {ID: 500, Name: "Great Helmet", Type: "Armor", BodyPart: "head", Grade: 2, Weight: 640},
			17:  {ID: 17, Name: "Wooden Arrow", Type: "EtcItem", Grade: 0, Weight: 6},
		},
		EtcItems: map[int32]datapack.EtcItem{
			1060: {ID: 1060, Name: "Lesser Healing Potion", Weight: 5},
		},
		Skills: map[datapack.SkillRef]datapack.Skill{
			{ID: 1204, Level: 2}: {Name: "Wind Walk"},
			{ID: 1204, Level: 1}: {Name: "Wind Walk"},
			{ID: 1068, Level: 1}: {Name: "Might"},
		},
		Classes: map[int32]datapack.Class{1: {RunSpeed: 115}},
	})
}

// TestInspectView checks the JSON of an inspect answer, with the datapack's
// names and without.
func TestInspectView(t *testing.T) {
	at := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	in := botclient.Inspection{
		At: at, ClassID: 1, Load: 13000, MaxLoad: 20000, WeightPenalty: 1, GradePenalty: true, Expertise: 1,
		DeathPenalty: 2, RunSpeed: 97,
		Items: []botclient.Item{
			{ObjectID: 1, TemplateID: 57, Count: 900},
			{ObjectID: 2, TemplateID: 1060, Count: 14},
			{ObjectID: 3, TemplateID: 70, Count: 1, Equipped: true, BodySlot: maskRHand, Enchant: 3},
			{ObjectID: 4, TemplateID: 356, Count: 1, Equipped: true, BodySlot: maskFullArmor},
			{ObjectID: 5, TemplateID: 500, Count: 1, Equipped: true, BodySlot: maskHead},
		},
		Effects: []botclient.Effect{
			{SkillID: 1068, Level: 1, Permanent: true},
			{SkillID: 1204, Level: 2, Left: 90*time.Second + 400*time.Millisecond},
		},
	}
	tests := []struct {
		name    string
		cat     *Catalog
		state   string
		want    string
		wantErr bool
	}{
		{
			name: "named by the catalog", cat: testCatalog(), state: inspectFresh,
			want: `{"state":"fresh","at":"2026-11-25T10:00:00Z",
				"inventory":{"items":[
					{"id":57,"name":"item #57","count":900},
					{"id":1060,"name":"Lesser Healing Potion","count":14,"weight":70}],
					"load":13000,"max_load":20000,"percent":65},
				"equipment":[
					{"slot":"weapon","id":70,"name":"Claymore","enchant":3,"grade":"C"},
					{"slot":"full armor","id":356,"name":"Full Plate Armor","grade":"C"},
					{"slot":"head","id":500,"name":"Great Helmet","grade":"C"}],
				"penalties":{"weight_level":1,"grade":true,"grade_weapon":true,"grade_armor":3,"expertise":1,
					"death":2,"run_speed":97,"base_run_speed":115},
				"effects":[
					{"skill_id":1068,"name":"Might","level":1,"left_s":0,"no_end":true},
					{"skill_id":1204,"name":"Wind Walk","level":2,"left_s":90}]}`,
		},
		{
			name: "no catalog: ids", state: inspectStale,
			want: `{"state":"stale","at":"2026-11-25T10:00:00Z",
				"inventory":{"items":[
					{"id":57,"name":"item #57","count":900},
					{"id":1060,"name":"item #1060","count":14}],
					"load":13000,"max_load":20000,"percent":65},
				"equipment":[
					{"slot":"weapon","id":70,"name":"item #70","enchant":3,"grade":""},
					{"slot":"full armor","id":356,"name":"item #356","grade":""},
					{"slot":"head","id":500,"name":"item #500","grade":""}],
				"penalties":{"weight_level":1,"grade":true,"grade_weapon":false,"grade_armor":0,"expertise":1,
					"death":2,"run_speed":97},
				"effects":[
					{"skill_id":1068,"name":"skill #1068","level":1,"left_s":0,"no_end":true},
					{"skill_id":1204,"name":"skill #1204","level":2,"left_s":90}]}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(inspectViewOf(in, tc.state, tc.cat))
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("view\n%s\nwant\n%s", raw, tc.want)
			}
		})
	}
}

// TestInspectGradeArmorCap checks the server's cap of 4 on the over-grade
// armor count, and that an arrow and an in-grade piece count nothing.
func TestInspectGradeArmorCap(t *testing.T) {
	in := botclient.Inspection{Expertise: 0, Items: []botclient.Item{
		{ObjectID: 1, TemplateID: 356, Equipped: true, BodySlot: maskFullArmor},
		{ObjectID: 2, TemplateID: 500, Equipped: true, BodySlot: maskHead},
		{ObjectID: 3, TemplateID: 500, Equipped: true, BodySlot: maskLegs},
		{ObjectID: 4, TemplateID: 2, Equipped: true, BodySlot: maskRHand},
		{ObjectID: 5, TemplateID: 17, Equipped: true, BodySlot: 0x0100},
	}}
	pen := inspectViewOf(in, inspectFresh, testCatalog()).Penalties
	if pen.GradeArmor != 4 || pen.GradeWeapon {
		t.Fatalf("armor %d, weapon %v; want 4, false", pen.GradeArmor, pen.GradeWeapon)
	}
}

// TestInspectGate checks the panel's bounds: one inspect in flight per bot,
// the per-bot cooldown and the global cap.
func TestInspectGate(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	g := newInspectGate()
	g.now = func() time.Time { return now }
	if err := g.enter("a"); err != nil {
		t.Fatal(err)
	}
	if err := g.enter("a"); !errors.Is(err, errInspectLimit) {
		t.Fatalf("second inspect in flight = %v, want refused", err)
	}
	g.leave("a")
	now = now.Add(inspectCooldown - time.Millisecond)
	if err := g.enter("a"); !errors.Is(err, errInspectLimit) {
		t.Fatalf("inspect in the cooldown = %v, want refused", err)
	}
	now = now.Add(time.Millisecond)
	for _, a := range []string{"a", "b", "c", "d"} {
		if err := g.enter(a); err != nil {
			t.Fatalf("inspect of %s = %v", a, err)
		}
	}
	if err := g.enter("e"); !errors.Is(err, errInspectLimit) {
		t.Fatalf("inspect past the cap = %v, want refused", err)
	}
	g.leave("b")
	now = now.Add(inspectCooldown)
	if err := g.enter("e"); err != nil {
		t.Fatalf("inspect once one ended = %v", err)
	}
	if got := statusOf(g.enter("e")); got != http.StatusTooManyRequests {
		t.Fatalf("refusal status %d, want 429", got)
	}
}

// TestAwaitInspection checks the states of the bot's answer.
func TestAwaitInspection(t *testing.T) {
	p := &Panel{cfg: Config{InspectWait: time.Hour}, quit: make(chan struct{})}
	reply := func(in *botclient.Inspection) <-chan botclient.Inspection {
		c := make(chan botclient.Inspection, 1)
		if in == nil {
			close(c)
		} else {
			c <- *in
		}
		return c
	}
	ctx := t.Context()
	if _, s := p.awaitInspection(ctx, reply(&botclient.Inspection{})); s != inspectFresh {
		t.Errorf("answer: %q, want fresh", s)
	}
	if _, s := p.awaitInspection(ctx, reply(&botclient.Inspection{Stale: true})); s != inspectStale {
		t.Errorf("stale answer: %q, want stale", s)
	}
	if _, s := p.awaitInspection(ctx, reply(nil)); s != inspectOffline {
		t.Errorf("stopped bot: %q, want offline", s)
	}
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if _, s := p.awaitInspection(gone, make(chan botclient.Inspection)); s != "" {
		t.Errorf("request gone: %q, want none", s)
	}
	close(p.quit)
	if _, s := p.awaitInspection(ctx, make(chan botclient.Inspection)); s != inspectUnresponsive {
		t.Errorf("no answer: %q, want unresponsive", s)
	}
}

// TestPanelInspect checks the endpoint over a bot with no session: the
// state offline alone, and the bounds.
func TestPanelInspect(t *testing.T) {
	g := newRig(t, Config{})
	g.want(http.StatusNotFound, "GET", "/api/bots/hz1/inspect", nil)
	g.panel.inspects.now = func() time.Time { return time.Now().Add(time.Hour) }
	g.want(http.StatusCreated, "POST", "/api/bots", map[string]any{"preset": "support"})
	r := g.next()
	code, body := g.do("GET", "/api/bots/hz1/inspect", nil)
	if code != http.StatusOK || body != "{\"state\":\"offline\"}\n" {
		t.Fatalf("inspect between sessions answered %d %s, want offline", code, body)
	}
	g.want(http.StatusTooManyRequests, "GET", "/api/bots/hz1/inspect", nil)
	g.want(http.StatusOK, "POST", "/api/bots/hz1/despawn", nil)
	endOnCancel(r)
	g.panel.inspects.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	code, body = g.do("GET", "/api/bots/hz1/inspect", nil)
	if code != http.StatusOK || body != "{\"state\":\"offline\"}\n" {
		t.Fatalf("inspect of a despawned bot answered %d %s, want offline", code, body)
	}
	g.noSecret()
	g.stop()
}
