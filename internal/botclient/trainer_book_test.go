package botclient

import (
	"testing"
	"time"
)

// TestTrainerBookArrivesMidVisit checks a spellbook that arrives while the
// visit that skipped its skill goes on (a gift, a trade) makes the trainer
// due again once that visit ends, as one bought after it does.
func TestTrainerBookArrivesMidVisit(t *testing.T) {
	b := newTrainerBrain(t, 200)
	heal1, might1 := AcquirableSkill{ID: skillHeal, Level: 1, Cost: 160}, AcquirableSkill{ID: skillMight, Level: 1, Cost: 30}
	needsBook := AcquireSkillInfo{ID: skillHeal, Level: 1, Cost: 160, Requirements: []SkillRequirement{{Type: spellbookRequirement, ItemID: healBook, Count: 1}}}
	book := Item{ObjectID: 300, TemplateID: healBook, Count: 1}
	runTrainerSteps(t, b, []trainerStep{
		{name: "select", wantRule: "train", wantKind: actSelect},
		{name: "talk", at: time.Second, events: []Event{MyTargetSelected{ObjectID: trainerObj}}, wantRule: "train", wantKind: actTalk},
		{name: "skill list", at: 2 * time.Second, events: []Event{NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}}, wantRule: "train", wantKind: actBypass},
		{name: "heal's cost", at: 3 * time.Second, events: []Event{AcquireSkillList{Skills: []AcquirableSkill{heal1, might1}}}, wantRule: "train", wantKind: actSkillInfo},
		{name: "book missing: might's cost", at: 4 * time.Second, events: []Event{needsBook}, wantRule: "train", wantKind: actSkillInfo},
		{
			name: "the book arrives, might learned", at: 5 * time.Second,
			events:   []Event{InventoryUpdate{Changes: []ItemChange{{State: ItemAdded, Item: book}}}, AcquireSkillInfo{ID: skillMight, Level: 1, Cost: 30}},
			wantRule: "train", wantKind: actLearnSkill,
		},
	})
	// The list after the learn offers Heal only, skipped in this visit:
	// the visit ends.
	if err := b.handle([]Event{SkillList{Skills: []KnownSkill{{ID: skillMight, Level: 1}}}, AcquireSkillList{Skills: []AcquirableSkill{heal1}}}, at(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	b.decide(at(6 * time.Second))
	if rule, a := b.decide(at(7 * time.Second)); rule != "train" || a.kind != actTalk {
		t.Fatalf("decide after the visit = rule %q action %d, want the trainer talked to again", rule, a.kind)
	}
}
