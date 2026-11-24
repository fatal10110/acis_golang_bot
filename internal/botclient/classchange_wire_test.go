package botclient

import (
	"testing"
	"time"
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
		{"the class master", liveMaster, classMaster},
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

// TestClassChangeMasterFromWire checks the class change rule finds the
// master from live-shaped NPCInfo and never takes Roy the Cat for it: with
// only Roy in sight it walks to the master's spawn on the travel map.
func TestClassChangeMasterFromWire(t *testing.T) {
	const master, roy = int32(0x20000050), int32(0x20000051)
	place := func(n NPCInfo, obj, x int32) NPCInfo {
		n.ObjectID, n.Pos = obj, Point{X: x}
		return n
	}
	spawn := Point{X: 5000}
	tests := []struct {
		name    string
		inSight []NPCInfo
		want    npcVisit
	}{
		{"the master in sight", []NPCInfo{place(liveMaster, master, 400)}, npcVisit{npcID: classMaster, obj: master, at: Point{X: 400}}},
		{"Roy nearer than the master", []NPCInfo{place(liveRoy, roy, 200), place(liveMaster, master, 400)}, npcVisit{npcID: classMaster, obj: master, at: Point{X: 400}}},
		{"Roy only: the master's spawn", []NPCInfo{place(liveRoy, roy, 200)}, npcVisit{npcID: classMaster, at: spawn}},
		{"nothing in sight: the master's spawn", nil, npcVisit{npcID: classMaster, at: spawn}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newClassBrain(t, classHumanMystic, 20)
			for _, n := range tt.inSight {
				b.world.Apply(n, t0)
			}
			if !b.startClassChange(at(time.Second)) {
				t.Fatal("startClassChange = false, want a visit")
			}
			if got := b.class.v; got.npcID != tt.want.npcID || got.obj != tt.want.obj || got.at != tt.want.at {
				t.Fatalf("visit = %+v, want %+v", got, tt.want)
			}
		})
	}
}
