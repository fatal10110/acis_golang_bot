package botwire

import (
	"bytes"
	"testing"
)

func TestTrainerEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"RequestBypassToServer", EncodeRequestBypassToServer("npc_7"), []byte{0x21, 'n', 0, 'p', 0, 'c', 0, '_', 0, '7', 0, 0, 0}},
		{
			"RequestAcquireSkillInfo", EncodeSkillRequest(OpGameRequestAcquireSkillInfo, 1011, 2, acquireSkillUsual),
			[]byte{0x6b, 0xf3, 3, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			"RequestAcquireSkill", EncodeSkillRequest(OpGameRequestAcquireSkill, 1068, 1, acquireSkillUsual),
			[]byte{0x6c, 0x2c, 4, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}
