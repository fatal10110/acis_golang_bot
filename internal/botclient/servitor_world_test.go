package botclient

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// ownServitor is the bot's servitor in the tests below.
const ownServitor = int32(0x10000123)

// testServitorInfo is a summoned Kat the Cat (npc 14111) at level 40.
var testServitorInfo = PetInfo{
	SummonType: SummonServitor, ObjectID: ownServitor, TemplateID: 14111,
	Pos: Point{X: 50, Y: 50}, PAtkSpd: 278, CollisionRadius: 11.5,
	CurHP: 1200, MaxHP: 1500, CurMP: 300, MaxMP: 450, Level: 40,
	PAtk: 401, PDef: 402, MAtk: 403, MDef: 404, AbnormalMask: 0x2010,
}

// testServitorState is testServitorInfo as the world keeps it.
var testServitorState = Servitor{
	ObjectID: ownServitor, TemplateID: 14111, Level: 40, Pos: Point{X: 50, Y: 50},
	HP: 1200, MaxHP: 1500, MP: 300, MaxMP: 450,
	PAtk: 401, PDef: 402, MAtk: 403, MDef: 404,
	PAtkSpd: 278, CollisionRadius: 11.5, AbnormalMask: 0x2010,
}

// TestWorldServitor applies the servitor's own packets and the frames it
// shows up in (Attack, MoveToPawn, Die) by object id (docs/servitors.md
// section 1): events in, the world's Servitor out.
func TestWorldServitor(t *testing.T) {
	with := func(f func(s *Servitor)) *Servitor {
		s := testServitorState
		f(&s)
		return &s
	}
	pet := testServitorInfo
	pet.SummonType = SummonPet
	other := testServitorInfo
	other.ObjectID = ownServitor + 1
	tests := []struct {
		name   string
		events []Event
		want   *Servitor
	}{
		{"none at first", nil, nil},
		{"PetInfo creates it", []Event{testServitorInfo}, with(func(*Servitor) {})},
		{"a pet's PetInfo is ignored", []Event{pet}, nil},
		{"PetStatusUpdate refreshes place, HP, MP and level", []Event{
			testServitorInfo,
			PetStatusUpdate{SummonType: SummonServitor, ObjectID: ownServitor, Pos: Point{X: 70}, CurHP: 900, MaxHP: 1600, CurMP: 250, MaxMP: 460, Level: 41},
		}, with(func(s *Servitor) {
			s.Pos, s.HP, s.MaxHP, s.MP, s.MaxMP, s.Level = Point{X: 70}, 900, 1600, 250, 460, 41
		})},
		{"another summon's PetStatusUpdate is ignored", []Event{
			testServitorInfo,
			PetStatusUpdate{SummonType: SummonServitor, ObjectID: ownServitor + 1, CurHP: 1},
			PetStatusUpdate{SummonType: SummonPet, ObjectID: ownServitor, CurHP: 1},
		}, with(func(*Servitor) {})},
		{"SetSummonRemainTime sets both lifetimes", []Event{
			testServitorInfo,
			SetSummonRemainTime{Total: 10 * time.Minute, Remaining: 9 * time.Minute},
		}, with(func(s *Servitor) { s.Lifetime, s.Remaining = 10*time.Minute, 9*time.Minute })},
		{"SetSummonRemainTime with no servitor is dropped", []Event{
			SetSummonRemainTime{Total: 10 * time.Minute, Remaining: 9 * time.Minute},
		}, nil},
		{"a second PetInfo refreshes and keeps target and lifetime", []Event{
			testServitorInfo,
			SetSummonRemainTime{Total: 10 * time.Minute, Remaining: 9 * time.Minute},
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			func() PetInfo { p := testServitorInfo; p.CurHP, p.InCombat = 1000, true; return p }(),
		}, with(func(s *Servitor) {
			s.HP, s.InCombat, s.Target = 1000, true, npcID
			s.Lifetime, s.Remaining = 10*time.Minute, 9*time.Minute
		})},
		{"another servitor's PetInfo replaces it", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			other,
		}, with(func(s *Servitor) { s.ObjectID = ownServitor + 1 })},
		{"its Attack sets its target and place", []Event{
			testServitorInfo,
			Attack{AttackerID: ownServitor, Pos: Point{X: 960}, Hits: []AttackHit{{TargetID: npcID, Damage: 80}}},
		}, with(func(s *Servitor) { s.Target, s.Pos = npcID, Point{X: 960} })},
		{"its MoveToPawn sets its target", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
		}, with(func(s *Servitor) { s.Target = npcID })},
		{"its follow walk to the bot clears the target", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			MoveToPawn{ObjectID: ownServitor, TargetID: selfID, Distance: 36},
		}, with(func(*Servitor) {})},
		{"others' Attack and MoveToPawn are ignored", []Event{
			testServitorInfo,
			Attack{AttackerID: pcID, Hits: []AttackHit{{TargetID: npcID}}},
			Attack{AttackerID: npcID, Hits: []AttackHit{{TargetID: ownServitor}}},
			MoveToPawn{ObjectID: selfID, TargetID: npcID, Distance: 40},
		}, with(func(*Servitor) {})},
		{"its target's Die clears the target", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			Die{ObjectID: npcID},
		}, with(func(*Servitor) {})},
		{"another's Die keeps the target", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			Die{ObjectID: pcID},
		}, with(func(s *Servitor) { s.Target = npcID })},
		{"its target leaving sight clears the target", []Event{
			testServitorInfo,
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			DeleteObject{ObjectID: npcID},
		}, with(func(*Servitor) {})},
		{"its own DeleteObject keeps it: only PetDelete removes it", []Event{
			testServitorInfo,
			DeleteObject{ObjectID: ownServitor},
		}, with(func(*Servitor) {})},
		{"its Die marks it dead and keeps it", []Event{
			testServitorInfo,
			AutoAttackStart{ObjectID: ownServitor},
			MoveToPawn{ObjectID: ownServitor, TargetID: npcID, Distance: 40},
			Die{ObjectID: ownServitor},
		}, with(func(s *Servitor) { s.Dead = true })},
		{"its combat stance", []Event{
			testServitorInfo,
			AutoAttackStart{ObjectID: ownServitor},
		}, with(func(s *Servitor) { s.InCombat = true })},
		{"its combat stance ends", []Event{
			testServitorInfo,
			AutoAttackStart{ObjectID: ownServitor},
			AutoAttackStop{ObjectID: ownServitor},
		}, with(func(*Servitor) {})},
		{"PetDelete removes it", []Event{
			testServitorInfo,
			Die{ObjectID: ownServitor},
			PetDelete{SummonType: SummonServitor, ObjectID: ownServitor},
		}, nil},
		{"another summon's PetDelete keeps it", []Event{
			testServitorInfo,
			PetDelete{SummonType: SummonServitor, ObjectID: ownServitor + 1},
			PetDelete{SummonType: SummonPet, ObjectID: ownServitor},
		}, with(func(*Servitor) {})},
		{"PetStatusShow changes nothing", []Event{
			testServitorInfo,
			PetStatusShow{SummonType: SummonServitor},
		}, with(func(*Servitor) {})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWorld(t)
			for i, ev := range tc.events {
				w.Apply(ev, at(time.Duration(i)*time.Second))
			}
			if !reflect.DeepEqual(w.Servitor, tc.want) {
				t.Errorf("Servitor = %+v\nwant %+v", w.Servitor, tc.want)
			}
			if _, ok := w.Objects[ownServitor]; ok {
				t.Error("the servitor is among the objects")
			}
		})
	}
}

// summonFrame writes one server frame of op with fields in order: int32,
// int64, float64, uint8, uint16 and string values.
func summonFrame(op byte, fields ...any) []byte {
	w := wire.NewPacketWriter(op)
	for _, f := range fields {
		switch v := f.(type) {
		case int32:
			w.WriteInt32(v)
		case int64:
			w.WriteInt64(v)
		case float64:
			w.WriteFloat64(v)
		case uint8:
			w.WriteUint8(v)
		case uint16:
			w.WriteUint16(v)
		case string:
			w.WriteString(v)
		default:
			panic(fmt.Sprintf("summonFrame: %T", f))
		}
	}
	return w.Bytes()
}

// servitorPetInfo writes a PetInfo frame in FramePetInfo's field order
// (serverpackets/petinfo.go) for a servitor with hp, in combat or dead.
func servitorPetInfo(summonType int32, hp int32, inCombat, dead uint8) []byte {
	return summonFrame(botwire.OpGamePetInfo,
		summonType, ownServitor, int32(14111+1000000), int32(0),
		int32(50), int32(50), int32(0), int32(0), int32(0), int32(333), int32(278),
		int32(155), int32(60), int32(155), int32(60), int32(155), int32(60), int32(155), int32(60),
		1.1, 1.05, 11.5, 21.5,
		int32(0), int32(0), int32(0),
		uint8(1), uint8(1), inCombat, dead, uint8(2),
		"Kat the Cat", "bot7",
		int32(1), int32(0), int32(0), int32(0), int32(0), hp, int32(1500), int32(300), int32(450), int32(0), int32(40),
		int64(0), int64(0), int64(0),
		int32(0), int32(0),
		int32(401), int32(402), int32(403), int32(404), int32(70), int32(30), int32(40), int32(155), int32(278), int32(333),
		int32(0x2010),
		uint16(0), uint8(0), uint16(0), uint8(0), int32(1), int32(1),
	)
}

// TestServitorSummonReplay replays the frames a summon sends its owner, in
// the server's order, through the decoder into the world. No live capture
// of a summon exists (it needs a summoner character), so the frames are
// built from the server writers (serverpackets/petinfo.go, pet.go, attack.go,
// movetopawn.go, die.go): PetInfo on discovery (visibility.go), the
// lifetime bar (summon_events.go LifetimeShown), its walk to and swing at a
// monster, PetStatusUpdate after each status change (summon_events.go), the
// monster's death, its own death, and PetDelete once the corpse goes
// (summon_events.go: never on death).
func TestServitorSummonReplay(t *testing.T) {
	statusUpdate := func(hp int32) []byte {
		return summonFrame(botwire.OpGamePetStatusUpdate,
			SummonServitor, ownServitor, int32(60), int32(50), int32(0), "bot7",
			int32(0), int32(0), hp, int32(1500), int32(300), int32(450), int32(40),
			int64(0), int64(0), int64(0))
	}
	steps := []struct {
		raw   []byte
		check func(t *testing.T, s *Servitor)
	}{
		{servitorPetInfo(SummonServitor, 1500, 0, 0), func(t *testing.T, s *Servitor) {
			if s == nil || s.ObjectID != ownServitor || s.TemplateID != 14111 || s.HP != 1500 || s.PAtk != 401 || s.MDef != 404 || s.AbnormalMask != 0x2010 {
				t.Fatalf("after PetInfo: %+v", s)
			}
		}},
		{summonFrame(botwire.OpGamePetStatusShow, SummonServitor), nil},
		{summonFrame(botwire.OpGameSetSummonRemainTime, int32(1_200_000), int32(1_196_591)), func(t *testing.T, s *Servitor) {
			if s.Lifetime != 20*time.Minute || s.Remaining != 1_196_591*time.Millisecond {
				t.Errorf("lifetime = %v of %v", s.Remaining, s.Lifetime)
			}
		}},
		// MoveToPawn (movetopawn.go): mover, target, distance, origin x y z.
		{summonFrame(botwire.OpGameMoveToPawn, ownServitor, npcID, int32(36), int32(50), int32(50), int32(0)), func(t *testing.T, s *Servitor) {
			if s.Target != npcID {
				t.Errorf("target after its walk = %#x", s.Target)
			}
		}},
		// Attack (attack.go): attacker, target, damage, flags, the
		// attacker's x y z, no more hits.
		{summonFrame(botwire.OpGameAttack, ownServitor, npcID, int32(120), uint8(0), int32(960), int32(0), int32(0), uint16(0)), func(t *testing.T, s *Servitor) {
			if s.Target != npcID || s.Pos != (Point{X: 960}) {
				t.Errorf("after its swing: target %#x at %v", s.Target, s.Pos)
			}
		}},
		{statusUpdate(1400), func(t *testing.T, s *Servitor) {
			if s.HP != 1400 || s.Pos != (Point{X: 60, Y: 50}) {
				t.Errorf("after PetStatusUpdate: HP %d at %v", s.HP, s.Pos)
			}
		}},
		// Die (die.go): object id, to village, clan hall, castle, siege HQ,
		// sweepable, fixed.
		{summonFrame(botwire.OpGameDie, npcID, int32(0), int32(0), int32(0), int32(0), int32(0), int32(0)), func(t *testing.T, s *Servitor) {
			if s.Target != 0 || s.Dead {
				t.Errorf("after its target died: target %#x, dead %v", s.Target, s.Dead)
			}
		}},
		{statusUpdate(0), nil},
		{summonFrame(botwire.OpGameDie, ownServitor, int32(0), int32(0), int32(0), int32(0), int32(0), int32(0)), func(t *testing.T, s *Servitor) {
			if s == nil || !s.Dead {
				t.Fatalf("after its Die: %+v", s)
			}
		}},
		{summonFrame(botwire.OpGamePetDelete, SummonServitor, ownServitor), func(t *testing.T, s *Servitor) {
			if s != nil {
				t.Errorf("after PetDelete: %+v", s)
			}
		}},
	}
	w := newTestWorld(t)
	for i, st := range steps {
		ev, ok, err := botwire.DecodeEvent(st.raw)
		if err != nil || !ok {
			t.Fatalf("step %d (%#02x): DecodeEvent = %v, %v", i, st.raw[0], ok, err)
		}
		w.Apply(ev, at(time.Duration(i)*time.Second))
		if st.check != nil {
			st.check(t, w.Servitor)
		}
	}
}

// TestPetEventLogsOnce has the brain log a pet's PetInfo once a session
// and keep no Servitor for it.
func TestPetEventLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	b, _ := newTestBrain(t, Persona{})
	b.log = slog.New(slog.NewTextHandler(&buf, nil))
	for i := range 3 {
		ev, ok, err := botwire.DecodeEvent(servitorPetInfo(SummonPet, 1500, 0, 0))
		if err != nil || !ok {
			t.Fatalf("DecodeEvent = %v, %v", ok, err)
		}
		b.handleOne(t, ev, at(time.Duration(i)*time.Second))
	}
	if n := strings.Count(buf.String(), "pet ignored"); n != 1 {
		t.Errorf("logged %d times, want 1:\n%s", n, buf.String())
	}
	if b.world.Servitor != nil {
		t.Errorf("Servitor = %+v, want none", b.world.Servitor)
	}
}
