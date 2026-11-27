package botwire

import (
	"testing"
)

// The class master and Roy the Cat as the live server shows them in NPCInfo
// (#343): the master (50000) borrows Roy's look, idTemplate 31228, and is
// told apart by its server-side name; Roy has none.
var (
	liveMaster = NPCInfo{ObjectID: 268470358, NPCID: 31228, Pos: Point{X: -83326, Y: 242864, Z: -3720}, Name: "Hasha", Title: "Class Manager"}
	liveRoy    = NPCInfo{ObjectID: 268470359, NPCID: 31228, Pos: Point{X: -84516, Y: 242971, Z: -3720}}
)

func TestNPCInfoTemplate(t *testing.T) {
	tests := []struct {
		name string
		npc  NPCInfo
		want int32
	}{
		{"the class master", liveMaster, ClassMaster},
		{"Roy the Cat", liveRoy, 31228},
		{"another server-side name on Roy's look", NPCInfo{NPCID: 31228, Name: "Roy"}, 31228},
		{"the master's name on another look", NPCInfo{NPCID: 30022, Name: "Hasha"}, 30022},
		{"a monster", NPCInfo{NPCID: 20001}, 20001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.npc.Template(); got != tt.want {
				t.Fatalf("Template() = %d, want %d", got, tt.want)
			}
		})
	}
}
