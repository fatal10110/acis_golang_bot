package botwire

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// trainerObj is a human mystic trainer (Zigaunt, NPC 30022) in sight.
const trainerObj = int32(0x20000010)

// trainerPage is the trainer's first page as the server sends it, with
// %objectId% filled in (data/html/trainer/30022.htm).
func trainerPage(id int32) string {
	return `<html><body>Trainer:<br><a action="bypass -h npc_` + itoa(id) + `_SkillList">Learn skills</a><br>` +
		`<a action="bypass -h npc_` + itoa(id) + `_Quest">Quest</a></body></html>`
}

func itoa(v int32) string { return strconv.Itoa(int(v)) }

// skillList is an AcquireSkillList frame in the server's layout: the
// level written twice, a zero after the cost.
func skillList(skills ...AcquirableSkill) []byte {
	fields := []any{acquireSkillUsual, int32(len(skills))}
	for _, s := range skills {
		fields = append(fields, s.ID, s.Level, s.Level, s.Cost, 0)
	}
	return packet(OpGameAcquireSkillList, fields...)
}

func TestDecodeTrainerEvents(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want Event
	}{
		// Object 7, page "ab" in UTF-16LE with its terminator, item 0.
		{"NpcHtmlMessage bytes", []byte{0x0f, 7, 0, 0, 0, 'a', 0, 'b', 0, 0, 0, 0, 0, 0, 0}, NpcHtmlMessage{ObjectID: 7, HTML: "ab"}},
		{"NpcHtmlMessage", packet(OpGameNpcHtmlMessage, trainerObj, trainerPage(trainerObj), 0), NpcHtmlMessage{ObjectID: trainerObj, HTML: trainerPage(trainerObj)}},
		// Type 0, one entry: Heal 1 (level twice), 160 SP, a zero.
		{
			"AcquireSkillList bytes",
			[]byte{0x8a, 0, 0, 0, 0, 1, 0, 0, 0, 0xf3, 3, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0xa0, 0, 0, 0, 0, 0, 0, 0},
			AcquireSkillList{Skills: []AcquirableSkill{{ID: 1011, Level: 1, Cost: 160}}},
		},
		{
			"AcquireSkillList", skillList(AcquirableSkill{1011, 2, 160}, AcquirableSkill{1068, 1, 470}),
			AcquireSkillList{Skills: []AcquirableSkill{{1011, 2, 160}, {1068, 1, 470}}},
		},
		{"AcquireSkillList empty", skillList(), AcquireSkillList{Skills: []AcquirableSkill{}}},
		{"AcquireSkillInfo", packet(OpGameAcquireSkillInfo, 1011, 1, 160, 0, 0), AcquireSkillInfo{ID: 1011, Level: 1, Cost: 160}},
		{
			"AcquireSkillInfo spellbook", packet(OpGameAcquireSkillInfo, 1011, 1, 160, 0, 1, 99, 1152, 1, 50),
			AcquireSkillInfo{ID: 1011, Level: 1, Cost: 160, Requirements: []SkillRequirement{{Type: 99, ItemID: 1152, Count: 1}}},
		},
		{"AcquireSkillDone", packet(OpGameAcquireSkillDone), AcquireSkillDone{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := DecodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("DecodeEvent = %v, %v, %v", got, ok, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DecodeEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeTrainerEventsRejects(t *testing.T) {
	list := skillList(AcquirableSkill{1011, 1, 160})
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{"NpcHtmlMessage without item id", packet(OpGameNpcHtmlMessage, 7, "page"), wire.ErrShortPacket},
		{"NpcHtmlMessage unterminated page", []byte{0x0f, 7, 0, 0, 0, 'a', 0}, wire.ErrShortPacket},
		{"NpcHtmlMessage trailing", packet(OpGameNpcHtmlMessage, 7, "page", 0, 0), ErrTrailingBytes},
		{"AcquireSkillList short entry", list[:len(list)-4], wire.ErrShortPacket},
		{"AcquireSkillList count past the frame", packet(OpGameAcquireSkillList, 0, 0x7fffffff), wire.ErrShortPacket},
		{"AcquireSkillList trailing", append(list, 0), ErrTrailingBytes},
		{"AcquireSkillInfo short requirement", packet(OpGameAcquireSkillInfo, 1011, 1, 160, 0, 1, 99, 1152, 1), wire.ErrShortPacket},
		{"AcquireSkillInfo trailing", packet(OpGameAcquireSkillInfo, 1011, 1, 160, 0, 0, 0), ErrTrailingBytes},
		{"AcquireSkillDone trailing", packet(OpGameAcquireSkillDone, 0), ErrTrailingBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := DecodeEvent(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("DecodeEvent error = %v, want %v", err, tt.want)
			}
		})
	}
}
