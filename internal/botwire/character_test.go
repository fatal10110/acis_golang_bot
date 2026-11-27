package botwire

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/testsupport/decodefuzz"
)

// named returns live slots (delete timer 0) bearing names, in order.
func named(names ...string) []CharSlot {
	slots := make([]CharSlot, len(names))
	for i, n := range names {
		slots[i] = CharSlot{Name: n}
	}
	return slots
}

// charSelectInfoPacket is the server's CharSelectInfo (charselectinfo.go):
// the count, then every field of each slot in its order. Fields the client
// skips carry distinct non-zero values so a misaligned decoder shows.
func charSelectInfoPacket(slots ...CharSlot) []byte {
	w := wire.NewPacketWriter(0x13)
	w.WriteInt32(int32(len(slots)))
	for i, s := range slots {
		v := int32(100 * (i + 1))
		next := func() int32 { v++; return v }
		w.WriteString(s.Name)
		w.WriteInt32(0x00030b7a) // char id
		w.WriteString("bot7")    // login name
		for range 10 {           // session id .. z
			w.WriteInt32(next())
		}
		w.WriteFloat64(97.5) // current hp
		w.WriteFloat64(41.5) // current mp
		w.WriteInt32(next()) // sp
		w.WriteInt64(1 << 40)
		for range 4 + 7 + 2*paperdollSlots + 3 {
			w.WriteInt32(next())
		}
		w.WriteFloat64(120) // max hp
		w.WriteFloat64(60)  // max mp
		w.WriteInt32(s.DeleteTimer)
		w.WriteInt32(s.ClassID)
		w.WriteInt32(int32(i % 2))
		w.WriteUint8(3)      // weapon enchant
		w.WriteInt32(next()) // augmentation
	}
	return w.Bytes()
}

func TestCharSelectInfoDecode(t *testing.T) {
	three := []CharSlot{{Name: "AldenBot", ClassID: 18}, {Name: "GraeBot", DeleteTimer: 3600, ClassID: 1}, {Name: "x", DeleteTimer: -1}}
	full := charSelectInfoPacket(three...)
	tests := []struct {
		name    string
		raw     []byte
		want    []CharSlot
		wantErr string
	}{
		{"empty account", charSelectInfoPacket(), nil, ""},
		{"one slot", charSelectInfoPacket(named("bot7")...), named("bot7"), ""},
		{"three slots", full, three, ""},
		{"truncated", full[:len(full)-1], nil, "decode CharSelectInfo"},
		{"trailing byte", append(append([]byte(nil), full...), 0), nil, "trailing"},
		{"count beyond slots", append([]byte{0x13, 4, 0, 0, 0}, full[5:]...), nil, "decode CharSelectInfo"},
		{"negative count", packet(0x13, -1), nil, "count -1"},
		{"opcode only", []byte{0x13}, nil, "decode CharSelectInfo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeCharSelectInfo(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("slots = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// FuzzCharSelectInfo feeds the decoder arbitrary payloads: it must return
// slots or an error, never panic, and allocate in proportion to the input.
func FuzzCharSelectInfo(f *testing.F) {
	f.Add(charSelectInfoPacket())
	f.Add(charSelectInfoPacket(named("AldenBot", "Gr")...))
	f.Add([]byte{0x13, 0xff, 0xff, 0xff, 0x7f})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) == 0 {
			return
		}
		decodefuzz.Bounded(t, "DecodeCharSelectInfo", raw, func() { _, _ = DecodeCharSelectInfo(raw) })
	})
}

func TestCharCreateEncoder(t *testing.T) {
	tests := []struct {
		name  string
		class StartingClass
		look  Appearance
		want  []byte
	}{
		// name, race, sex, class, six stats, hair style, hair color, face
		// (requestcharactercreate.go).
		{"human fighter", HumanFighter, Appearance{HairStyle: 1}, packet(0x0b, "AldenBot", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0)},
		{"elven mystic female", ElvenMystic, Appearance{Sex: SexFemale, HairStyle: 6, HairColor: 3, Face: 2}, packet(0x0b, "AldenBot", 1, 1, 25, 0, 0, 0, 0, 0, 0, 6, 3, 2)},
		{"orc mystic", OrcMystic, Appearance{HairStyle: 4, HairColor: 2, Face: 1}, packet(0x0b, "AldenBot", 3, 0, 49, 0, 0, 0, 0, 0, 0, 4, 2, 1)},
		{"dwarven fighter", DwarvenFighter, Appearance{Sex: SexFemale}, packet(0x0b, "AldenBot", 4, 1, 53, 0, 0, 0, 0, 0, 0, 0, 0, 0)},
	}
	for _, tt := range tests {
		if got := EncodeRequestCharacterCreate("AldenBot", tt.class, tt.look); !bytes.Equal(got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, got, tt.want)
		}
	}
}

// TestCharRandomAppearance: every draw is one the server accepts, and both
// sexes and the female-only hair styles come up.
func TestCharRandomAppearance(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	var female, longHair bool
	for range 2000 {
		a := RandomAppearance(r)
		if err := a.Validate(); err != nil {
			t.Fatalf("RandomAppearance() = %+v: %v", a, err)
		}
		female = female || a.Sex == SexFemale
		longHair = longHair || a.HairStyle > 4
	}
	if !female || !longHair {
		t.Fatalf("2000 draws: female %v, hair style above 4 %v", female, longHair)
	}
}

// TestRecordedCharSelectInfo replays the -record capture of CharSelectInfo
// checked in under testdata/recorded: the handshake reads it, not the
// reader, so TestRecordedFixtures leaves it to this test. It must decode
// with exactly len(payload) consumed.
func TestRecordedCharSelectInfo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "recorded", fmt.Sprintf("%02x.bin", OpGameCharSelectInfo)))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != OpGameCharSelectInfo {
		t.Fatalf("fixture name does not match its opcode")
	}
	if _, err := DecodeCharSelectInfo(raw); err != nil {
		t.Fatal(err)
	}
}
