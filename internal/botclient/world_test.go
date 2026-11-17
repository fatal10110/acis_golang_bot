package botclient

import (
	"strings"
	"testing"
	"time"
	"unsafe"
)

var t0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

const (
	selfID = int32(0x10000001)
	npcID  = int32(0x20000001)
	pcID   = int32(0x20000002)
)

func at(d time.Duration) time.Time { return t0.Add(d) }

// newTestWorld is a world whose self (selfID, running at 100 units per
// second) stands at the origin, with an NPC and a player in sight.
func newTestWorld(t *testing.T) *World {
	t.Helper()
	w, err := NewWorld(Self{ObjectID: selfID, Name: "bot7"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	w.Apply(UserInfo{ObjectID: selfID, Name: "bot7", RunSpeed: 100, WalkSpeed: 50, Running: true, HP: 10, MaxHP: 20}, t0)
	w.Apply(NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 1000}, RunSpeed: 200, WalkSpeed: 50, Running: true}, t0)
	w.Apply(CharInfo{ObjectID: pcID, Name: "other", Pos: Point{Y: 1000}, RunSpeed: 100, Running: true}, t0)
	return w
}

func TestWorldDeadReckoning(t *testing.T) {
	type step struct {
		at time.Duration
		ev Event
	}
	tests := []struct {
		name  string
		steps []step
		check time.Duration
		want  Point
		moves bool
	}{
		{"start of move", []step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}}, 0, Point{}, true},
		{"along the move", []step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000, Z: 100}}}}, 2 * time.Second, Point{X: 200, Z: 20}, true},
		{"never past the destination", []step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}}, 20 * time.Second, Point{X: 1000}, false},
		{
			"report within drift keeps the estimate",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {2 * time.Second, ValidateLocation{ObjectID: selfID, Pos: Point{X: 350}}}},
			3 * time.Second,
			Point{X: 300},
			true,
		},
		{
			"report past drift snaps",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {2 * time.Second, ValidateLocation{ObjectID: selfID, Pos: Point{X: 500}}}},
			3 * time.Second,
			Point{X: 600},
			true,
		},
		{
			"new move from a close origin continues the estimate",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {2 * time.Second, MoveToLocation{ObjectID: selfID, Origin: Point{X: 150}, Dest: Point{X: 200, Y: 1000}}}},
			3 * time.Second,
			Point{X: 200, Y: 100},
			true,
		},
		{
			"new move from a far origin snaps",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {2 * time.Second, MoveToLocation{ObjectID: selfID, Origin: Point{Y: 500}, Dest: Point{Y: 1000}}}},
			3 * time.Second,
			Point{Y: 600},
			true,
		},
		{
			"StopMove stops at the reported position",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {2 * time.Second, StopMove{ObjectID: selfID, Pos: Point{X: 190}, Heading: 5}}},
			5 * time.Second,
			Point{X: 190},
			false,
		},
		{
			"walking speed after UserInfo",
			[]step{{0, UserInfo{ObjectID: selfID, RunSpeed: 100, WalkSpeed: 50}}, {0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}},
			2 * time.Second,
			Point{X: 100},
			true,
		},
		{
			"MoveToPawn stops short of the target",
			[]step{{0, MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 100}}},
			20 * time.Second,
			Point{X: 900},
			false,
		},
		{
			"MoveToPawn to an unknown target stays",
			[]step{{0, MoveToPawn{ObjectID: selfID, TargetID: 42, Distance: 100, Origin: Point{X: 10}}}},
			time.Second,
			Point{},
			false,
		},
		{
			"teleport places at once",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {time.Second, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5000, Y: 5000}}}},
			3 * time.Second,
			Point{X: 5000, Y: 5000},
			false,
		},
		{
			"sitting stops at the reported position",
			[]step{{0, MoveToLocation{ObjectID: selfID, Dest: Point{X: 1000}}}, {time.Second, ChangeWaitType{ObjectID: selfID, Type: WaitSitting, Pos: Point{X: 120}}}},
			3 * time.Second,
			Point{X: 120},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, s := range tt.steps {
				w.Apply(s.ev, at(s.at))
			}
			now := at(tt.check)
			if got := w.Self.Position(now); got != tt.want {
				t.Fatalf("Position = %+v, want %+v", got, tt.want)
			}
			if got := w.Self.Moving(now); got != tt.moves {
				t.Fatalf("Moving = %v, want %v", got, tt.moves)
			}
		})
	}
}

func TestWorldObjectMoves(t *testing.T) {
	w := newTestWorld(t)
	w.Apply(MoveToLocation{ObjectID: npcID, Origin: Point{X: 1000}, Dest: Point{X: 2000}}, t0)
	if got := w.Objects[npcID].Position(at(time.Second)); got != (Point{X: 1200}) {
		t.Fatalf("NPC at %+v, want X 1200", got)
	}
	// A player's MoveToPawn onto the moving NPC aims at where the NPC is.
	w.Apply(MoveToPawn{ObjectID: pcID, TargetID: npcID, Distance: 0, Origin: Point{Y: 1000}}, at(time.Second))
	if got, want := w.Objects[pcID].move.to, (Point{X: 1200}); got != want {
		t.Fatalf("player heads to %+v, want %+v", got, want)
	}
	// An object not in sight is ignored.
	w.Apply(MoveToLocation{ObjectID: 42, Dest: Point{X: 1}}, t0)
	if _, ok := w.Objects[42]; ok {
		t.Fatal("move created an object")
	}
}

func TestWorldState(t *testing.T) {
	tests := []struct {
		name  string
		evs   []Event
		check func(t *testing.T, w *World)
	}{
		{
			"StatusUpdate sets only the fields it carries",
			[]Event{StatusUpdate{ObjectID: selfID, Set: StatusHP | StatusMaxCP | StatusKarma | StatusLoad, HP: 15, MaxCP: 30, Karma: 4, Load: 9}},
			func(t *testing.T, w *World) {
				s := w.Self
				if s.HP != 15 || s.MaxHP != 20 || s.MaxCP != 30 || s.Karma != 4 || s.Load != 9 {
					t.Fatalf("self = %+v", s)
				}
			},
		},
		{
			"StatusUpdate of an NPC",
			[]Event{StatusUpdate{ObjectID: npcID, Set: StatusHP | StatusMaxHP, HP: 40, MaxHP: 80}, NPCInfo{ObjectID: npcID, NPCID: 20001, Pos: Point{X: 1000}, InCombat: true}},
			func(t *testing.T, w *World) {
				o := w.Objects[npcID]
				if o.HP != 40 || o.MaxHP != 80 || !o.InCombat || o.Kind != KindNPC {
					t.Fatalf("npc = %+v", o)
				}
			},
		},
		{
			"sit and stand",
			[]Event{ChangeWaitType{ObjectID: selfID, Type: WaitSitting}, ChangeWaitType{ObjectID: pcID, Type: WaitSitting, Pos: Point{Y: 1000}}, ChangeWaitType{ObjectID: selfID, Type: WaitStanding}},
			func(t *testing.T, w *World) {
				if w.Self.Sitting || !w.Objects[pcID].Sitting {
					t.Fatalf("self sitting %v, player sitting %v", w.Self.Sitting, w.Objects[pcID].Sitting)
				}
			},
		},
		{
			"fake death shows another dead",
			[]Event{ChangeWaitType{ObjectID: pcID, Type: WaitFakeDeathStart}, ChangeWaitType{ObjectID: selfID, Type: WaitFakeDeathStart}},
			func(t *testing.T, w *World) {
				if !w.Objects[pcID].Dead || w.Self.Dead {
					t.Fatalf("player dead %v, self dead %v", w.Objects[pcID].Dead, w.Self.Dead)
				}
			},
		},
		{
			"own death opens the restart prompt",
			[]Event{Die{ObjectID: selfID, Restart: RestartPoints{Village: true, Fixed: true}}},
			func(t *testing.T, w *World) {
				want := RestartPrompt{Open: true, Points: RestartPoints{Village: true, Fixed: true}, At: t0}
				if !w.Self.Dead || w.Prompts.Restart != want {
					t.Fatalf("dead %v, prompt %+v", w.Self.Dead, w.Prompts.Restart)
				}
			},
		},
		{
			"revive closes it",
			[]Event{Die{ObjectID: selfID, Restart: RestartPoints{Village: true}}, Revive{ObjectID: selfID}},
			func(t *testing.T, w *World) {
				if w.Self.Dead || w.Prompts.Restart != (RestartPrompt{}) {
					t.Fatalf("dead %v, prompt %+v", w.Self.Dead, w.Prompts.Restart)
				}
			},
		},
		{
			"others die and revive",
			[]Event{Die{ObjectID: npcID}, Die{ObjectID: pcID}, Revive{ObjectID: pcID}},
			func(t *testing.T, w *World) {
				if !w.Objects[npcID].Dead || w.Objects[pcID].Dead || w.Self.Dead || w.Prompts.Restart.Open {
					t.Fatalf("npc dead %v, player dead %v", w.Objects[npcID].Dead, w.Objects[pcID].Dead)
				}
			},
		},
		{
			"target set",
			[]Event{MyTargetSelected{ObjectID: npcID}},
			func(t *testing.T, w *World) {
				if w.Target != npcID {
					t.Fatalf("target %d", w.Target)
				}
			},
		},
		{
			"another's unselect keeps the target",
			[]Event{MyTargetSelected{ObjectID: npcID}, TargetUnselected{ObjectID: pcID}},
			func(t *testing.T, w *World) {
				if w.Target != npcID {
					t.Fatalf("target %d", w.Target)
				}
			},
		},
		{
			"own unselect clears it",
			[]Event{MyTargetSelected{ObjectID: npcID}, TargetUnselected{ObjectID: selfID}},
			func(t *testing.T, w *World) {
				if w.Target != 0 {
					t.Fatalf("target %d", w.Target)
				}
			},
		},
		{
			"DeleteObject removes the object and the target on it",
			[]Event{MyTargetSelected{ObjectID: npcID}, DeleteObject{ObjectID: npcID}},
			func(t *testing.T, w *World) {
				if _, ok := w.Objects[npcID]; ok || w.Target != 0 || len(w.Objects) != 1 {
					t.Fatalf("objects %v, target %d", w.Objects, w.Target)
				}
			},
		},
		{
			"own teleport forgets objects and the target",
			[]Event{MyTargetSelected{ObjectID: npcID}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5}}},
			func(t *testing.T, w *World) {
				if len(w.Objects) != 0 || w.Target != 0 {
					t.Fatalf("objects %v, target %d", w.Objects, w.Target)
				}
			},
		},
		{
			"own fast teleport keeps them",
			[]Event{MyTargetSelected{ObjectID: npcID}, TeleportToLocation{ObjectID: selfID, Pos: Point{X: 5}, Fast: true}},
			func(t *testing.T, w *World) {
				if len(w.Objects) != 2 || w.Target != npcID {
					t.Fatalf("objects %v, target %d", w.Objects, w.Target)
				}
			},
		},
		{
			"combat stance",
			[]Event{AutoAttackStart{ObjectID: selfID}, AutoAttackStart{ObjectID: npcID}, AutoAttackStop{ObjectID: npcID}},
			func(t *testing.T, w *World) {
				if !w.Self.InCombat || w.Objects[npcID].InCombat {
					t.Fatalf("self %v, npc %v", w.Self.InCombat, w.Objects[npcID].InCombat)
				}
			},
		},
		{
			"CharInfo replaces a player's record",
			[]Event{CharInfo{ObjectID: pcID, Name: "other", PvPFlag: 1, Karma: 30, StoreType: 3, ClassID: 9, Pos: Point{Y: 1000}}},
			func(t *testing.T, w *World) {
				o := w.Objects[pcID]
				if o.Kind != KindPlayer || o.Name != "other" || o.PvPFlag != 1 || o.Karma != 30 || o.StoreType != 3 || o.ClassID != 9 {
					t.Fatalf("player = %+v", o)
				}
			},
		},
		{
			"UserInfo fills self",
			[]Event{UserInfo{ObjectID: selfID, Name: "bot7", Level: 40, ClassID: 26, PvPFlag: 1, Karma: 2, StoreType: 1, Load: 5, MaxLoad: 6, CP: 7, MaxCP: 8}},
			func(t *testing.T, w *World) {
				s := w.Self
				if s.Level != 40 || s.ClassID != 26 || s.PvPFlag != 1 || s.Karma != 2 || s.StoreType != 1 || s.Load != 5 || s.MaxLoad != 6 || s.CP != 7 || s.MaxCP != 8 {
					t.Fatalf("self = %+v", s)
				}
			},
		},
		{
			"events about self never become objects",
			[]Event{CharInfo{ObjectID: selfID}, NPCInfo{ObjectID: selfID}},
			func(t *testing.T, w *World) {
				if _, ok := w.Objects[selfID]; ok {
					t.Fatal("self recorded as an object")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			for _, ev := range tt.evs {
				w.Apply(ev, t0)
			}
			tt.check(t, w)
		})
	}
}

// NewWorld replays the EnterWorld burst and then UserInfo through
// decodeEvent; frames without a decoder are skipped.
func TestNewWorldReplaysBurst(t *testing.T) {
	npc := NPCInfo{ObjectID: npcID, NPCID: 20001, Attackable: true, Pos: Point{X: 1000}, RunSpeed: 195, WalkSpeed: 60}
	pc := CharInfo{ObjectID: pcID, Name: "other", Pos: Point{Y: 1000}, RunSpeed: 156, WalkSpeed: 104, Running: true}
	ui := testUserInfo
	ui.ObjectID = selfID
	self := Self{
		Name: "bot7", ObjectID: selfID, X: ui.Pos.X, Y: ui.Pos.Y, Z: ui.Pos.Z,
		Burst: [][]byte{
			npcInfoPacket(npc, 130, 40, 1.5),
			{opGameCharSelectInfo, 1},
			charInfoPacket(pc, 120, 80, 1.3, 0),
			packet(opGameChangeWaitType, selfID, 0, ui.Pos.X, ui.Pos.Y, ui.Pos.Z),
		},
		UserInfo: userInfoPacket(ui, 120, 80, 1.3, 0),
	}
	w, err := NewWorld(self, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Objects) != 2 || w.Objects[npcID].NPCID != 20001 || w.Objects[pcID].Name != "other" {
		t.Fatalf("objects = %v", w.Objects)
	}
	s := w.Self
	if s.ObjectID != selfID || s.HP != ui.HP || s.MaxCP != ui.MaxCP || s.Level != ui.Level || !s.Sitting || s.Position(t0) != ui.Pos {
		t.Fatalf("self = %+v", s)
	}

	self.Burst = append(self.Burst, packet(opGameDie, selfID))
	if _, err := NewWorld(self, t0); err == nil || !strings.Contains(err.Error(), "burst") {
		t.Fatalf("malformed burst frame: err = %v", err)
	}
}

// Object records stay compact: thousands of objects in sight per bot must
// not add up to megabytes.
func TestObjectRecordCompact(t *testing.T) {
	if size := unsafe.Sizeof(Object{}); size > 128 {
		t.Fatalf("Object is %d bytes, want at most 128", size)
	}
}
