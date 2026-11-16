package botclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
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

// MaxNameCandidates bounds the creations EnterGame tries on one connection:
// each name the server reports taken costs one RequestCharacterCreate.
const MaxNameCandidates = 8

// Character says which character EnterGame plays and how it is created when
// the account has none of that name.
type Character struct {
	// Names are the candidate names in preference order, 1 to
	// MaxNameCandidates of them. The first one an existing character bears
	// is selected; when none does, EnterGame creates one with the first name
	// the server accepts.
	Names      []string
	Class      StartingClass
	Appearance Appearance
}

// Validate reports a character the server would refuse to create.
func (c Character) Validate() error {
	if len(c.Names) == 0 || len(c.Names) > MaxNameCandidates {
		return fmt.Errorf("character: %d candidate names, want 1-%d", len(c.Names), MaxNameCandidates)
	}
	for _, n := range c.Names {
		if !botnames.Valid(n) {
			return fmt.Errorf("character name %q: want 1-16 letters or digits", n)
		}
	}
	if !isStartingClass(c.Class) {
		return fmt.Errorf("character class %d of race %d: not a starting class", c.Class.ID, c.Class.Race)
	}
	if err := c.Appearance.Validate(); err != nil {
		return fmt.Errorf("character: %w", err)
	}
	return nil
}

func isStartingClass(c StartingClass) bool {
	for _, s := range StartingClasses() {
		if s == c {
			return true
		}
	}
	return false
}

// CharCreateFail reasons (charcreatefail.go; character_flow.go maps the
// roster's outcomes onto them).
const (
	createFailNameTaken   int32 = 2 // another character, any account, has the name
	createFailInvalidName int32 = 4 // the spelling, or an NPC's name
)

// errNoCharacterSlot reports a character list without the character just
// created or selected.
var errNoCharacterSlot = errors.New("character not in CharSelectInfo")

// charSlot is what EnterGame needs of one CharSelectInfo slot.
type charSlot struct {
	Name string
	// DeleteTimer is -1 for a banned character, the seconds left for one
	// pending deletion, else 0.
	DeleteTimer int32
}

// paperdollSlots is the number of paperdoll entries CharSelectInfo writes
// per slot, object ids then template ids (paperdollWriteOrder).
const paperdollSlots = 17

// decodeCharSelectInfo reads every slot of CharSelectInfo in the server's
// field order (charselectinfo.go), keeping the name and delete timer, and
// fails unless it consumes the whole payload.
func decodeCharSelectInfo(raw []byte) ([]charSlot, error) {
	r := wire.NewPacketReader(raw)
	n := r.ReadInt32()
	if n < 0 {
		return nil, fmt.Errorf("decode CharSelectInfo: count %d", n)
	}
	var slots []charSlot
	for i := int32(0); i < n && r.Err() == nil; i++ {
		var s charSlot
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
		r.ReadInt32() // class id
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

// findSlot returns the slot index of the first of names a slot bears, and
// that name; -1 when none does. Names compare exactly, as the server stores
// them.
func findSlot(slots []charSlot, names []string) (int32, string) {
	for _, name := range names {
		for i, s := range slots {
			if s.Name == name {
				return int32(i), name
			}
		}
	}
	return -1, ""
}

// chooseSlot picks the slot of the first candidate an existing character
// bears, creating the character when none does. A banned character or one
// pending deletion is refused: the server aborts its selection silently.
func (g *GameConn) chooseSlot(ctx context.Context, slots []charSlot, ch Character) (int32, string, error) {
	slot, name := findSlot(slots, ch.Names)
	if slot < 0 {
		var err error
		if slots, name, err = g.createCharacter(ctx, ch); err != nil {
			return 0, "", err
		}
		if slot, _ = findSlot(slots, []string{name}); slot < 0 {
			return 0, "", fmt.Errorf("created %q: %w", name, errNoCharacterSlot)
		}
	}
	if t := slots[slot].DeleteTimer; t != 0 {
		return 0, "", fmt.Errorf("character %q is banned or pending deletion (delete timer %d)", name, t)
	}
	return slot, name, nil
}

// createCharacter tries ch's names in order until the server creates one,
// moving on only when it reports the name taken or refused (for a name of
// valid spelling, an NPC's): any other refusal is final. Requests go one at
// a time, each after the previous answer, at most MaxNameCandidates per
// connection; the server does not flood-gate creation. It returns the
// character list the server sends after CharCreateOk and the name created.
func (g *GameConn) createCharacter(ctx context.Context, ch Character) ([]charSlot, string, error) {
	var last int32
	for _, name := range ch.Names {
		if err := g.send(ctx, encodeRequestCharacterCreate(name, ch.Class, ch.Appearance)); err != nil {
			return nil, "", fmt.Errorf("send RequestCharacterCreate: %w", err)
		}
		reply, err := g.readUntil(ctx, opGameCharCreateOk, opGameCharCreateFail)
		if err != nil {
			return nil, "", fmt.Errorf("read CharCreateOk: %w", err)
		}
		if reply[0] == opGameCharCreateFail {
			last = wire.NewPacketReader(reply).ReadInt32()
			if last == createFailNameTaken || last == createFailInvalidName {
				continue
			}
			return nil, "", &GameStartError{Packet: "CharCreateFail", Reason: last}
		}
		reply, err = g.readUntil(ctx, opGameCharSelectInfo)
		if err != nil {
			return nil, "", fmt.Errorf("read CharSelectInfo after create: %w", err)
		}
		slots, err := decodeCharSelectInfo(reply)
		if err != nil {
			return nil, "", err
		}
		return slots, name, nil
	}
	return nil, "", fmt.Errorf("all %d candidate names refused: %w", len(ch.Names), &GameStartError{Packet: "CharCreateFail", Reason: last})
}
