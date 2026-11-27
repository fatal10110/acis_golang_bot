package botwire

import (
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// StartingClass is one of the nine root professions a character may be
// created with (classes/*.xml, baseLvl 1; data/manager/roster.go refuses any
// other), with the race it belongs to.
type StartingClass struct {
	ID     int32
	Race   int32
	Mystic bool
}

// The races, as RequestCharacterCreate and CharSelectInfo number them.
const (
	RaceHuman int32 = iota
	RaceElf
	RaceDarkElf
	RaceOrc
	RaceDwarf
)

// The starting classes (class ids from the datapack's classes/*.xml).
var (
	HumanFighter   = StartingClass{ID: 0, Race: RaceHuman}
	HumanMystic    = StartingClass{ID: 10, Race: RaceHuman, Mystic: true}
	ElvenFighter   = StartingClass{ID: 18, Race: RaceElf}
	ElvenMystic    = StartingClass{ID: 25, Race: RaceElf, Mystic: true}
	DarkFighter    = StartingClass{ID: 31, Race: RaceDarkElf}
	DarkMystic     = StartingClass{ID: 38, Race: RaceDarkElf, Mystic: true}
	OrcFighter     = StartingClass{ID: 44, Race: RaceOrc}
	OrcMystic      = StartingClass{ID: 49, Race: RaceOrc, Mystic: true}
	DwarvenFighter = StartingClass{ID: 53, Race: RaceDwarf}
)

// StartingClasses returns the nine starting classes, fighters and mystics of
// each race in race order; dwarves have no mystic.
func StartingClasses() []StartingClass {
	return []StartingClass{
		HumanFighter, HumanMystic, ElvenFighter, ElvenMystic, DarkFighter, DarkMystic,
		OrcFighter, OrcMystic, DwarvenFighter,
	}
}

// The sexes RequestCharacterCreate accepts for a player.
const (
	SexMale   int32 = 0
	SexFemale int32 = 1
)

// Appearance is a new character's sex, hair and face. The server refuses a
// face above 2, a hair color above 3 and a hair style above 4 for a male or
// 6 for a female (data/manager/roster.go).
type Appearance struct {
	Sex       int32
	HairStyle int32
	HairColor int32
	Face      int32
}

// maxHairStyle is the highest hair style index sex may choose.
func maxHairStyle(sex int32) int32 {
	if sex == SexMale {
		return 4
	}
	return 6
}

// Validate reports an appearance the server would refuse.
func (a Appearance) Validate() error {
	switch {
	case a.Sex != SexMale && a.Sex != SexFemale:
		return fmt.Errorf("sex %d: want male (0) or female (1)", a.Sex)
	case a.HairStyle < 0 || a.HairStyle > maxHairStyle(a.Sex):
		return fmt.Errorf("hair style %d: want 0-%d", a.HairStyle, maxHairStyle(a.Sex))
	case a.HairColor < 0 || a.HairColor > 3:
		return fmt.Errorf("hair color %d: want 0-3", a.HairColor)
	case a.Face < 0 || a.Face > 2:
		return fmt.Errorf("face %d: want 0-2", a.Face)
	}
	return nil
}

// RandomAppearance draws a sex, hair and face the server accepts.
func RandomAppearance(r *rand.Rand) Appearance {
	a := Appearance{Sex: r.Int32N(2)}
	a.HairStyle = r.Int32N(maxHairStyle(a.Sex) + 1)
	a.HairColor = r.Int32N(4)
	a.Face = r.Int32N(3)
	return a
}

// CharSlot is what EnterGame needs of one CharSelectInfo slot.
type CharSlot struct {
	Name string
	// DeleteTimer is -1 for a banned character, the seconds left for one
	// pending deletion, else 0.
	DeleteTimer int32
	// ClassID is the character's current class.
	ClassID int32
}

// paperdollSlots is the number of paperdoll entries CharSelectInfo writes
// per slot, object ids then template ids (paperdollWriteOrder).
const paperdollSlots = 17

// DecodeCharSelectInfo reads every slot of CharSelectInfo in the server's
// field order (charselectinfo.go), keeping the name, delete timer and class, and
// fails unless it consumes the whole payload.
func DecodeCharSelectInfo(raw []byte) ([]CharSlot, error) {
	r := wire.NewPacketReader(raw)
	n := r.ReadInt32()
	if n < 0 {
		return nil, fmt.Errorf("decode CharSelectInfo: count %d", n)
	}
	var slots []CharSlot
	for i := int32(0); i < n && r.Err() == nil; i++ {
		var s CharSlot
		s.Name = r.ReadString()
		r.ReadInt32()  // char id
		r.ReadString() // login name
		// session id, clan id, builder level, sex, race, base class, a one,
		// x, y, z
		for range 10 {
			r.ReadInt32()
		}
		r.ReadFloat64() // current hp
		r.ReadFloat64() // current mp
		r.ReadInt32()   // sp
		r.ReadInt64()   // exp
		// level, karma, pk, pvp, seven zeros, paperdoll object then template
		// ids, hair style, hair color, face
		for range 4 + 7 + 2*paperdollSlots + 3 {
			r.ReadInt32()
		}
		r.ReadFloat64() // max hp
		r.ReadFloat64() // max mp
		s.DeleteTimer = r.ReadInt32()
		s.ClassID = r.ReadInt32()
		r.ReadInt32() // active
		r.ReadUint8() // weapon enchant
		r.ReadInt32() // augmentation
		slots = append(slots, s)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("decode CharSelectInfo: %w", err)
	}
	if r.Remaining() != 0 {
		return nil, fmt.Errorf("decode CharSelectInfo: %d trailing bytes", r.Remaining())
	}
	return slots, nil
}
