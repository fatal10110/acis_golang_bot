package e2e

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// sysMsgFrame encodes the SystemMessage id the server sends (FrameSystemMessage),
// with text as its one text parameter when set.
func sysMsgFrame(id int32, text string) []byte {
	b := []byte{opSysMsg}
	b = binary.LittleEndian.AppendUint32(b, uint32(id))
	if text == "" {
		return binary.LittleEndian.AppendUint32(b, 0)
	}
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = binary.LittleEndian.AppendUint32(b, 0) // a text parameter
	for _, u := range utf16.Encode([]rune(text)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return binary.LittleEndian.AppendUint16(b, 0)
}

// TestClanSaidTap checks that the bot's tap hands the test every outcome of
// //pledge create the founder is told, and nothing else (#524).
func TestClanSaidTap(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		cause string // "" for a frame the tap keeps from the test
	}{
		{"created", sysMsgFrame(msgClanCreated, ""), "ClanCreated"},
		{"already in a clan or no id", sysMsgFrame(msgFailedToCreateClan, ""), "FailedToCreateClan"},
		{"below the level", sysMsgFrame(msgNotMeetCriteriaToCreateClan, ""), "NotMeetCriteriaToCreateClan"},
		{"must wait", sysMsgFrame(msgMustWaitBeforeCreatingClan, ""), "MustWaitBeforeCreatingClan"},
		{"invalid name", sysMsgFrame(msgClanNameInvalid, ""), "ClanNameInvalid"},
		{"name length", sysMsgFrame(msgClanNameLengthIncorrect, ""), "ClanNameLengthIncorrect"},
		{"name taken", sysMsgFrame(msgS1AlreadyExists, "cr1"), `S1AlreadyExists ("cr1")`},
		{"another message", sysMsgFrame(msgTargetMustBeInClan, ""), ""},
		{"another packet", []byte{opWHList, 0}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tap := withClanSaid(nil)
			tap.Record(tt.frame)
			select {
			case m := <-tap.said:
				cause, _ := foundingCause(m)
				if tt.cause == "" || !strings.HasPrefix(cause, tt.cause) {
					t.Errorf("tap handed %+v (%q), want cause %q", m, cause, tt.cause)
				}
			default:
				if tt.cause != "" {
					t.Errorf("tap handed nothing, want cause %q", tt.cause)
				}
			}
		})
	}
}

// TestClanSaidTapPassesFrames checks that the wrapped tap still sees every
// frame both ways.
func TestClanSaidTapPassesFrames(t *testing.T) {
	inner := &countTap{}
	tap := withClanSaid(inner)
	tap.Record(sysMsgFrame(msgClanCreated, ""))
	tap.Record([]byte{opWHList})
	tap.RecordSent([]byte{0x74})
	if inner.read != 2 || inner.sent != 1 {
		t.Errorf("inner tap saw %d read and %d sent, want 2 and 1", inner.read, inner.sent)
	}
	if len(tap.said) != 1 {
		t.Errorf("tap handed %d outcomes, want 1", len(tap.said))
	}
}

type countTap struct{ read, sent int }

func (c *countTap) Record([]byte)     { c.read++ }
func (c *countTap) RecordSent([]byte) { c.sent++ }

var _ botclient.FrameTap = clanSaidTap{}
