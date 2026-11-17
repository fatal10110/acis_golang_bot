package botclient

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
	"github.com/fatal10110/acis_golang_bot/internal/testsupport/decodefuzz"
)

// named returns live slots (delete timer 0) bearing names, in order.
func named(names ...string) []charSlot {
	slots := make([]charSlot, len(names))
	for i, n := range names {
		slots[i] = charSlot{Name: n}
	}
	return slots
}

// charSelectInfoPacket is the server's CharSelectInfo (charselectinfo.go):
// the count, then every field of each slot in its order. Fields the client
// skips carry distinct non-zero values so a misaligned decoder shows.
func charSelectInfoPacket(slots ...charSlot) []byte {
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
	three := []charSlot{{Name: "AldenBot", ClassID: 18}, {Name: "GraeBot", DeleteTimer: 3600, ClassID: 1}, {Name: "x", DeleteTimer: -1}}
	full := charSelectInfoPacket(three...)
	tests := []struct {
		name    string
		raw     []byte
		want    []charSlot
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
			got, err := decodeCharSelectInfo(tt.raw)
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
		decodefuzz.Bounded(t, "decodeCharSelectInfo", raw, func() { _, _ = decodeCharSelectInfo(raw) })
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
		if got := encodeRequestCharacterCreate("AldenBot", tt.class, tt.look); !bytes.Equal(got, tt.want) {
			t.Errorf("%s = %x, want %x", tt.name, got, tt.want)
		}
	}
}

func TestCharacterValidate(t *testing.T) {
	ok := Character{Names: []string{"AldenBot"}, Class: HumanMystic}
	with := func(f func(*Character)) Character { c := ok; f(&c); return c }
	tests := []struct {
		name    string
		ch      Character
		wantErr string
	}{
		{"valid", ok, ""},
		{"female hair 6", with(func(c *Character) { c.Appearance = Appearance{Sex: SexFemale, HairStyle: 6, HairColor: 3, Face: 2} }), ""},
		{"max candidates", with(func(c *Character) { c.Names = slices.Repeat([]string{"A"}, MaxNameCandidates) }), ""},
		{"no names", with(func(c *Character) { c.Names = nil }), "candidate names"},
		{"too many names", with(func(c *Character) { c.Names = slices.Repeat([]string{"A"}, MaxNameCandidates+1) }), "candidate names"},
		{"name too long", with(func(c *Character) { c.Names = []string{strings.Repeat("a", 17)} }), "letters or digits"},
		{"name with space", with(func(c *Character) { c.Names = []string{"Al den"} }), "letters or digits"},
		{"dwarven mystic", with(func(c *Character) { c.Class = StartingClass{ID: 49, Race: RaceDwarf, Mystic: true} }), "not a starting class"},
		{"second profession", with(func(c *Character) { c.Class = StartingClass{ID: 1, Race: RaceHuman} }), "not a starting class"},
		{"sex etc", with(func(c *Character) { c.Appearance.Sex = 2 }), "sex 2"},
		{"male hair 5", with(func(c *Character) { c.Appearance.HairStyle = 5 }), "hair style 5"},
		{"hair color 4", with(func(c *Character) { c.Appearance.HairColor = 4 }), "hair color 4"},
		{"face 3", with(func(c *Character) { c.Appearance.Face = 3 }), "face 3"},
		{"negative face", with(func(c *Character) { c.Appearance.Face = -1 }), "face -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ch.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
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

// TestCharEnterGame drives EnterGame's choice of character: select by name,
// create with retries on a taken or refused name, and the refusals that end
// it.
func TestCharEnterGame(t *testing.T) {
	const objectID = 0x10000001
	authLogin := recvStep(packet(0x08, "bot7", 4, 3, 1, 2))
	list := func(slots ...charSlot) step { return sendStep(charSelectInfoPacket(slots...)) }
	create := func(name string) step {
		return recvStep(packet(0x0b, name, 2, 1, 38, 0, 0, 0, 0, 0, 0, 5, 1, 2))
	}
	fail := func(reason int) step { return sendStep(packet(0x1a, reason)) }
	created := sendStep(packet(0x19, 1))
	enter := func(slot int, name string) []step {
		return []step{
			recvStep(packet(0x0d, slot, uint16(0), 0, 0, 0)),
			sendStep(packet(0x15, name, objectID)),
			recvStep([]byte{0x03}),
			sendStep(packet(0x04, -71000, 258000, -3100, 49152, objectID, name)),
		}
	}
	ch := Character{
		Names:      []string{"DraelBot", "KiaBot", "ThorBot"},
		Class:      DarkMystic,
		Appearance: Appearance{Sex: SexFemale, HairStyle: 5, HairColor: 1, Face: 2},
	}
	other := charSlot{Name: "SomeoneElse"}
	tests := []struct {
		name     string
		script   []step
		wantName string
		wantErr  string
		wantFail int32 // CharCreateFail reason in the error, when set
	}{
		{"first candidate in slot 2", slices.Concat([]step{authLogin, list(other, charSlot{Name: "KiaBot"}, charSlot{Name: "DraelBot"})}, enter(2, "DraelBot")), "DraelBot", "", 0},
		{"later candidate in slot 1", slices.Concat([]step{authLogin, list(other, charSlot{Name: "ThorBot"})}, enter(1, "ThorBot")), "ThorBot", "", 0},
		{"case differs is another character", []step{authLogin, list(charSlot{Name: "draelbot"})}, "", "account has characters", 0},
		{"only other characters", []step{authLogin, list(other)}, "", "account has characters", 0},
		{"taken then refused then created", slices.Concat([]step{authLogin, list(), create("DraelBot"), fail(2), create("KiaBot"), fail(4), create("ThorBot"), created, list(charSlot{Name: "ThorBot"})}, enter(0, "ThorBot")), "ThorBot", "", 0},
		{"every name taken", []step{authLogin, list(), create("DraelBot"), fail(2), create("KiaBot"), fail(2), create("ThorBot"), fail(2)}, "", "all 3 candidate names refused", 2},
		{"too many characters", []step{authLogin, list(), create("DraelBot"), fail(1)}, "", "CharCreateFail", 1},
		{"creation failed", []step{authLogin, list(), create("DraelBot"), fail(0)}, "", "CharCreateFail", 0},
		{"created but not listed", []step{authLogin, list(), create("DraelBot"), created, list(other)}, "", "not in CharSelectInfo", 0},
		{"pending deletion", []step{authLogin, list(charSlot{Name: "DraelBot", DeleteTimer: 3600})}, "", "pending deletion", 0},
		{"banned", []step{authLogin, list(charSlot{Name: "KiaBot", DeleteTimer: -1})}, "", "banned", 0},
		{"server selected another", slices.Concat([]step{authLogin, list(charSlot{Name: "DraelBot"})}, enter(0, "SomeoneElse")[:2]), "", `server selected "SomeoneElse"`, 0},
		{"malformed list", []step{authLogin, sendStep(packet(0x13, 1, 7, byte(0)))}, "", "decode CharSelectInfo", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, done := dialFake(t, true, tt.script, tt.wantErr != "")
			self, err := EnterGame(t.Context(), g, "bot7", SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}, ch, nil)
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			if tt.wantErr == "" {
				if err != nil || self.Name != tt.wantName || self.ObjectID != objectID {
					t.Fatalf("EnterGame = %+v, %v; want %q", self, err, tt.wantName)
				}
				// Slots counts the list the character was chosen from: after
				// the creation when there was one.
				wantSlots := map[string]int{"first candidate in slot 2": 3, "later candidate in slot 1": 2, "taken then refused then created": 1}[tt.name]
				if self.Slots != wantSlots {
					t.Fatalf("Slots = %d, want %d", self.Slots, wantSlots)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			var gs *GameStartError
			if errors.As(err, &gs) != (tt.wantErr == "CharCreateFail" || tt.wantFail != 0) || (gs != nil && gs.Reason != tt.wantFail) {
				t.Fatalf("err = %v, want CharCreateFail reason %d", err, tt.wantFail)
			}
		})
	}
}

// TestCharEnterGameInvalid: an invalid Character fails before AuthLogin.
func TestCharEnterGameInvalid(t *testing.T) {
	g, done := dialFake(t, true, nil, false)
	_, err := EnterGame(t.Context(), g, "bot7", SessionKeys{}, Character{Names: []string{"bad name"}, Class: HumanFighter}, nil)
	g.Close()
	if serr := <-done; serr != nil {
		t.Fatalf("fake server: %v", serr)
	}
	if err == nil || !strings.Contains(err.Error(), "bad name") {
		t.Fatalf("err = %v, want the invalid name", err)
	}
}
