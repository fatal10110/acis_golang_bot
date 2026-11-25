package botclient

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// pvpWireCases are the duel window frames and MagicSkillCanceled, each
// byte for byte as the server writes it (serverpackets/duel.go
// FrameExDuelAskStart, frameDuelFlag, FrameExDuelUpdateUserInfo;
// serverpackets/magic_skill.go FrameMagicSkillCanceled): the extended
// opcode 0xfe, the little-endian sub-opcode, then each field, a string as
// UTF-16LE closed by a zero character.
func pvpWireCases() []struct {
	name string
	raw  []byte
	want Event
} {
	return []struct {
		name string
		raw  []byte
		want Event
	}{
		{"ExDuelAskStart", []byte{0xfe, 0x4b, 0x00, 'A', 0, 'b', 0, 0, 0, 0, 0, 0, 0}, ExDuelAskStart{Requester: "Ab"}},
		{"ExDuelAskStart party", []byte{0xfe, 0x4b, 0x00, 'A', 0, 0, 0, 1, 0, 0, 0}, ExDuelAskStart{Requester: "A", Party: true}},
		{"ExDuelReady", []byte{0xfe, 0x4c, 0x00, 0, 0, 0, 0}, ExDuelReady{}},
		{"ExDuelReady party", []byte{0xfe, 0x4c, 0x00, 1, 0, 0, 0}, ExDuelReady{Party: true}},
		{"ExDuelStart", []byte{0xfe, 0x4d, 0x00, 0, 0, 0, 0}, ExDuelStart{}},
		{"ExDuelStart party", []byte{0xfe, 0x4d, 0x00, 1, 0, 0, 0}, ExDuelStart{Party: true}},
		{"ExDuelEnd", []byte{0xfe, 0x4e, 0x00, 0, 0, 0, 0}, ExDuelEnd{}},
		{"ExDuelEnd party", []byte{0xfe, 0x4e, 0x00, 1, 0, 0, 0}, ExDuelEnd{Party: true}},
		{
			"ExDuelUpdateUserInfo",
			[]byte{
				0xfe, 0x4f, 0x00, 'M', 0, 0, 0,
				0x02, 0x00, 0x00, 0x10, // object id
				0x0a, 0, 0, 0, // class id
				0x14, 0, 0, 0, // level
				0x2c, 0x01, 0, 0, // HP 300
				0x9a, 0x01, 0, 0, // max HP 410
				0x78, 0, 0, 0, // MP 120
				0x96, 0, 0, 0, // max MP 150
				0x5a, 0, 0, 0, // CP 90
				0xc8, 0, 0, 0, // max CP 200
			},
			ExDuelUpdateUserInfo{Name: "M", ObjectID: 0x10000002, ClassID: 10, Level: 20, HP: 300, MaxHP: 410, MP: 120, MaxMP: 150, CP: 90, MaxCP: 200},
		},
		{"MagicSkillCanceled", []byte{0x49, 0x02, 0x00, 0x00, 0x10}, MagicSkillCanceled{CasterID: 0x10000002}},
	}
}

func TestPvPWireDecode(t *testing.T) {
	for _, tt := range pvpWireCases() {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := decodeEvent(tt.raw)
			if err != nil || !ok {
				t.Fatalf("decodeEvent = %v, %v, %v", got, ok, err)
			}
			if got != tt.want {
				t.Fatalf("decodeEvent = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// Every frame one byte short fails, and one byte long fails with
// errTrailingBytes: each decoder consumes exactly len(payload).
func TestPvPWireDecodeRejects(t *testing.T) {
	for _, tt := range pvpWireCases() {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeEvent(tt.raw[:len(tt.raw)-1]); err == nil {
				t.Fatal("short frame decoded without error")
			}
			long := append(append([]byte(nil), tt.raw...), 0)
			if _, _, err := decodeEvent(long); !errors.Is(err, errTrailingBytes) {
				t.Fatalf("long frame error = %v, want %v", err, errTrailingBytes)
			}
		})
	}
}

// The requests encode the server's readImpl field order: the payloads the
// server's own decoder tests read (clientpackets/duel_test.go; RequestTargetCancel
// clientpackets/targetaction.go, one uint16).
func TestPvPWireEncode(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"RequestDuelStart party", encodeRequestDuelStart("Ab", true), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 1, 0, 0, 0}},
		{"RequestDuelStart", encodeRequestDuelStart("Ab", false), []byte{0xd0, 0x27, 0x00, 'A', 0, 'b', 0, 0, 0, 0, 0, 0, 0}},
		{"RequestDuelAnswerStart accepted", encodeRequestDuelAnswerStart(false, true), []byte{0xd0, 0x28, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}},
		{"RequestDuelAnswerStart party declined", encodeRequestDuelAnswerStart(true, false), []byte{0xd0, 0x28, 0x00, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestTargetCancel", encodeRequestTargetCancel(), []byte{0x37, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("encoded % x, want % x", tt.got, tt.want)
			}
		})
	}
}

// The SystemMessage ids match the server's constants
// (serverpackets/systemmessage.go, systemmessage_duel.go,
// olympiad_match.go).
func TestPvPSystemMessageIDs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		got, srv int32
	}{
		{"YouDidS1Dmg", msgYouDidS1Dmg, 35},
		{"S1GaveYouS2Dmg", msgS1GaveYouS2Dmg, 36},
		{"CantAtkPeacezone", msgCantAtkPeacezone, 84},
		{"TargetInPeacezone", msgTargetInPeacezone, 85},
		{"S1ResistedYourS2", msgResisted, 139},
		{"S1IsBusyTryLater", msgS1IsBusyTryLater, 153},
		{"EnteredCombatZone", msgEnteredCombatZone, 283},
		{"LeftCombatZone", msgLeftCombatZone, 284},
		{"OtherPartyIsFrozen", msgOtherPartyIsFrozen, 692},
		{"NoOpponentForDuel", msgNoOpponentForDuel, 1926},
		{"S1ChallengedToDuel", msgS1ChallengedToDuel, 1927},
		{"S1AcceptedYourDuel", msgS1AcceptedYourDuel, 1929},
		{"YouAcceptedS1Duel", msgYouAcceptedS1Duel, 1930},
		{"S1DeclinedYourDuel", msgS1DeclinedYourDuel, 1931},
		{"S1ChallengedYouToDuel", msgS1ChallengedYouToDuel, 1938},
		{"UnableToRequestDuel", msgUnableToRequestDuel, 1940},
		{"DuelBeginsInS1Seconds", msgDuelBeginsInS1Seconds, 1945},
		{"LetTheDuelBegin", msgLetTheDuelBegin, 1949},
		{"S1WonTheDuel", msgS1WonTheDuel, 1950},
		{"DuelEndedInTie", msgDuelEndedInTie, 1952},
		{"S1WithdrewS2Won", msgS1WithdrewS2Won, 1955},
		{"S1CannotDuelPrivateStore", msgS1CannotDuelPrivateStore, 2017},
		{"S1CannotDuelFishing", msgS1CannotDuelFishing, 2018},
		{"S1CannotDuelHPOrMPBelowHalf", msgS1CannotDuelHPOrMPBelowHalf, 2019},
		{"S1CannotDuelProhibitedArea", msgS1CannotDuelProhibitedArea, 2020},
		{"S1CannotDuelInBattle", msgS1CannotDuelInBattle, 2021},
		{"S1CannotDuelAlreadyDuelling", msgS1CannotDuelAlreadyDuelling, 2022},
		{"S1CannotDuelChaotic", msgS1CannotDuelChaotic, 2023},
		{"S1CannotDuelOlympiad", msgS1CannotDuelOlympiad, 2024},
		{"S1CannotDuelRiding", msgS1CannotDuelRiding, 2027},
		{"S1CannotReceiveDuelTooFar", msgS1CannotReceiveDuelTooFar, 2028},
	} {
		if tt.got != tt.srv {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.srv)
		}
	}
}

// TestRecordedPvPFields: the recorded UserInfo (04.bin) and NPCInfo (16.bin)
// captures keep a plausible collision radius, and the bot's kill counts
// are not negative. A field read from the wrong offset lands far outside.
func TestRecordedPvPFields(t *testing.T) {
	decode := func(name string) Event {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("testdata", "recorded", name))
		if err != nil {
			t.Fatal(err)
		}
		ev, ok, err := decodeEvent(raw)
		if err != nil || !ok {
			t.Fatalf("%s: ok = %v, err = %v", name, ok, err)
		}
		return ev
	}
	u, ok := decode("04.bin").(UserInfo)
	if !ok {
		t.Fatal("04.bin is not a UserInfo")
	}
	if u.CollisionRadius != 7.5 {
		t.Errorf("UserInfo CollisionRadius = %v, want 7.5", u.CollisionRadius)
	}
	if u.PKKills != 0 || u.PvPKills != 0 || u.AllyID != 0 {
		t.Errorf("UserInfo PKKills, PvPKills, AllyID = %d, %d, %d, want 0, 0, 0", u.PKKills, u.PvPKills, u.AllyID)
	}
	n, ok := decode("16.bin").(NPCInfo)
	if !ok {
		t.Fatal("16.bin is not an NPCInfo")
	}
	if n.CollisionRadius != 13 {
		t.Errorf("NPCInfo CollisionRadius = %v, want 13", n.CollisionRadius)
	}
}

// The bot's own MagicSkillCanceled ends its cast at once; another caster's
// leaves the bot's cast running.
func TestWorldMagicSkillCanceled(t *testing.T) {
	tests := []struct {
		name    string
		caster  int32
		casting bool
	}{
		{"own cast", selfID, false},
		{"another caster", pcID, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newTestWorld(t)
			w.Apply(MagicSkillUse{CasterID: selfID, TargetID: npcID, SkillID: 1177, HitTime: 2 * time.Second}, t0)
			if !w.Self.Casting(t0) {
				t.Fatal("not casting after its MagicSkillUse")
			}
			w.Apply(MagicSkillCanceled{CasterID: tt.caster}, at(time.Second))
			if got := w.Self.Casting(at(time.Second)); got != tt.casting {
				t.Fatalf("Casting = %v, want %v", got, tt.casting)
			}
		})
	}
}
