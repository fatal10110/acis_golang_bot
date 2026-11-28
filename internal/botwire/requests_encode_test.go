package botwire

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// le lays out values as the client writes them: int32 and uint16 little
// endian, a byte as is, a string as NUL-terminated UTF-16LE.
func le(vals ...any) []byte {
	var b []byte
	for _, v := range vals {
		switch v := v.(type) {
		case int32:
			b = binary.LittleEndian.AppendUint32(b, uint32(v))
		case int:
			b = binary.LittleEndian.AppendUint32(b, uint32(v))
		case byte:
			b = append(b, v)
		case string:
			for _, u := range utf16.Encode([]rune(v)) {
				b = binary.LittleEndian.AppendUint16(b, u)
			}
			b = append(b, 0, 0)
		default:
			panic("le: unsupported value")
		}
	}
	return b
}

// TestBrainPacketEncoders pins every in-world client packet to the field

func TestEncodeAppearRestart(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"Appearing", EncodeAppearing(), []byte{0x30}},
		{"RequestRestartPoint village", EncodeRequestRestartPoint(RestartVillage), []byte{0x6d, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = % x, want % x", tt.name, tt.got, tt.want)
		}
	}
}

func TestEncodeSay2Tell(t *testing.T) {
	// "hi" in UTF-16LE with its terminator, the Tell type, then "GM".
	want := []byte{OpGameSay2, 'h', 0, 'i', 0, 0, 0, 2, 0, 0, 0, 'G', 0, 'M', 0, 0, 0}
	if got := EncodeSay2Tell("GM", "hi"); !bytes.Equal(got, want) {
		t.Fatalf("EncodeSay2Tell = % x, want % x", got, want)
	}
}

// TestBrainPacketEncoders pins every in-world client packet to the field
// order of the server's decoder (internal/gameserver/network/clientpackets).
func TestBrainPacketEncoders(t *testing.T) {
	to, from := Point{X: 100, Y: -200, Z: -3000}, Point{X: -1, Y: 2, Z: 3}
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			"MoveBackwardToLocation", EncodeMoveBackwardToLocation(to, from),
			le(byte(0x01), 100, -200, -3000, -1, 2, 3, 1),
		},
		{
			"Action", EncodeAction(0x10203040, from),
			le(byte(0x04), 0x10203040, -1, 2, 3, byte(0)),
		},
		{
			"RequestActionUse sit/stand", EncodeRequestActionUse(ActionSitStand),
			le(byte(0x45), 0, 0, byte(0)),
		},
		{
			"RequestActionUse walk/run", EncodeRequestActionUse(ActionWalkRun),
			le(byte(0x45), 1, 0, byte(0)),
		},
		{
			"RequestActionUse servitor attack", EncodeRequestActionUse(ActionServitorAttack),
			le(byte(0x45), 22, 0, byte(0)),
		},
		{
			"RequestActionUse servitor stop", EncodeRequestActionUse(ActionServitorStop),
			le(byte(0x45), 23, 0, byte(0)),
		},
		{
			"RequestSocialAction", EncodeRequestSocialAction(7),
			le(byte(0x1b), 7),
		},
		{
			"Say2 all", EncodeSay2("hi there", ChatAll),
			le(byte(0x38), "hi there", 0),
		},
		{
			"ValidatePosition", EncodeValidatePosition(to, 49152),
			le(byte(0x48), 100, -200, -3000, 49152, 0),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !bytes.Equal(tc.got, tc.want) {
				t.Fatalf("payload\n got % x\nwant % x", tc.got, tc.want)
			}
		})
	}
}

// TestCombatEncoders pins the combat client packets to the server decoders
// (clientpackets targetaction.go, magic_skill_use.go).
func TestCombatEncoders(t *testing.T) {
	from := Point{X: -1, Y: 2, Z: 3}
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"AttackRequest", EncodeAttackRequest(0x10203040, from), le(byte(0x0a), 0x10203040, -1, 2, 3, byte(0))},
		{"RequestMagicSkillUse", EncodeRequestMagicSkillUse(1177, false), le(byte(0x2f), 1177, 0, byte(0))},
		{"RequestMagicSkillUse forced", EncodeRequestMagicSkillUse(1177, true), le(byte(0x2f), 1177, 1, byte(0))},
	}
	for _, tc := range tests {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s\n got % x\nwant % x", tc.name, tc.got, tc.want)
		}
	}
}

// TestScriptEncoders pins the encoders the live e2e harness scripts its bots
// and its GM client with.
func TestScriptEncoders(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"SendBypassBuildCmd", EncodeSendBypassBuildCmd("kick e2e"), []byte{0x5b, 'k', 0, 'i', 0, 'c', 0, 'k', 0, ' ', 0, 'e', 0, '2', 0, 'e', 0, 0, 0}},
		{"RequestRestartPoint village", EncodeRequestRestartPoint(RestartVillage), []byte{0x6d, 0, 0, 0, 0}},
		{"Appearing", EncodeAppearing(), []byte{0x30}},
		{"Action", EncodeAction(0x01020304, Point{X: 1, Y: -1, Z: 2}), []byte{0x04, 4, 3, 2, 1, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0, 0}},
		{"MoveBackwardToLocation", EncodeMoveBackwardToLocation(Point{X: 1, Y: -1, Z: 2}, Point{X: 3, Y: 4, Z: 5}), []byte{0x01, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0}},
		{"RequestMagicSkillUse", EncodeRequestMagicSkillUse(28, false), []byte{0x2f, 28, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"RequestMagicSkillUse ctrl", EncodeRequestMagicSkillUse(0x0102, true), []byte{0x2f, 2, 1, 0, 0, 1, 0, 0, 0, 0}},
		{"RequestJoinParty", EncodeRequestJoinParty("ab", 1), []byte{0x29, 'a', 0, 'b', 0, 0, 0, 1, 0, 0, 0}},
		{"Say2Tell", EncodeSay2Tell("ab", "hi"), []byte{0x38, 'h', 0, 'i', 0, 0, 0, 2, 0, 0, 0, 'a', 0, 'b', 0, 0, 0}},
		// The server's own vector (clientpackets_item_trade_test.go).
		{"RequestDestroyItem", EncodeRequestDestroyItem(501, 2), []byte{0x59, 0xf5, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}},
		{"UseItem", EncodeUseItem(0x01020304), []byte{0x14, 4, 3, 2, 1, 0, 0, 0, 0}},
		{"RequestDropItem", EncodeRequestDropItem(0x01020304, 2, Point{X: 1, Y: -1, Z: 2}), []byte{0x12, 4, 3, 2, 1, 2, 0, 0, 0, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 2, 0, 0, 0}},
		{"Say2 shout", EncodeSay2("hi", ChatShout), []byte{0x38, 'h', 0, 'i', 0, 0, 0, 1, 0, 0, 0}},
		{"Say2 trade", EncodeSay2("hi", ChatTrade), []byte{0x38, 'h', 0, 'i', 0, 0, 0, 8, 0, 0, 0}},
		{"TradeRequest", EncodeTradeRequest(0x01020304), []byte{0x15, 4, 3, 2, 1}},
		{"AddTradeItem", EncodeAddTradeItem(7, 300), []byte{0x16, 0, 0, 0, 0, 7, 0, 0, 0, 0x2c, 1, 0, 0}},
		{"TradeDone confirm", EncodeTradeDone(true), []byte{0x17, 1, 0, 0, 0}},
		{"TradeDone cancel", EncodeTradeDone(false), []byte{0x17, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("% x, want % x", tt.got, tt.want)
			}
		})
	}
}

func TestSessionEncoders(t *testing.T) {
	tests := []struct {
		name      string
		got, want []byte
	}{
		{"EnterWorld", EncodeEnterWorld(), []byte{0x03}},
		{"Logout", EncodeLogout(), []byte{0x09}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, tt.want)
		}
	}
}
