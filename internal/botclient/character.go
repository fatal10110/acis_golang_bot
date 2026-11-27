package botclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang_bot/internal/botwire"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// MaxNameCandidates bounds the creations EnterGame tries on one connection:
// each name the server reports taken costs one RequestCharacterCreate.
const MaxNameCandidates = 8

// Character says which character EnterGame plays and, for a bot that may
// create one, how (docs/plan.md section 8, "Character pinning"):
//   - with a known Name, the character bearing it, never creating one: none
//     is ErrCharacterMissing;
//   - else the first of Names an existing character bears (adoption);
//   - else, for a bot that may create, a new character with the first of
//     Names the server accepts, on an account holding no character: one
//     holding only other characters is ErrAccountHasCharacters;
//   - else, for a NeverCreate bot, ErrNoCharacterToAdopt.
type Character struct {
	// Name is the known name; empty for a bot whose character is not known
	// yet. With it, Names, NeverCreate, Class and Appearance are unused.
	Name string
	// Names are the candidate names in preference order, 1 to
	// MaxNameCandidates of them: the names derived for the bot's account
	// (NewCharacter).
	Names []string
	// NeverCreate is set for a bot imported from an earlier swarm: it adopts
	// a character bearing one of Names or stops, and carries no starting
	// class.
	NeverCreate bool
	// Class and Appearance are what a bot that may create creates.
	Class      StartingClass
	Appearance Appearance
}

// NewCharacter returns the character of a new bot of account: its names
// derive from the account and m (botnames.AccountSource), so every session
// finds the character the first one created; class and appearance are drawn
// from rng.
func NewCharacter(m botnames.Marker, account string, rng *rand.Rand) (Character, error) {
	gen, err := botnames.New(m, botnames.AccountSource(account))
	if err != nil {
		return Character{}, err
	}
	classes := StartingClasses()
	return Character{
		Names:      gen.Names(MaxNameCandidates),
		Class:      classes[rng.IntN(len(classes))],
		Appearance: RandomAppearance(rng),
	}, nil
}

// Validate reports a character EnterGame could not select, or the server
// would refuse to create. Class and appearance are checked only for a bot
// that may create.
func (c Character) Validate() error {
	if c.Name != "" {
		if !botnames.Valid(c.Name) {
			return fmt.Errorf("character name %q: want 1-16 letters or digits", c.Name)
		}
		return nil
	}
	if len(c.Names) == 0 || len(c.Names) > MaxNameCandidates {
		return fmt.Errorf("character: %d candidate names, want 1-%d", len(c.Names), MaxNameCandidates)
	}
	for _, n := range c.Names {
		if !botnames.Valid(n) {
			return fmt.Errorf("character name %q: want 1-16 letters or digits", n)
		}
	}
	if c.NeverCreate {
		return nil
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

// The stops of a character choice: the bot ends, and its owner decides what
// follows (docs/plan.md section 8).
var (
	// ErrCharacterMissing stops a bot whose known character is not on its
	// account: deleted or renamed on the server.
	ErrCharacterMissing = errors.New("character missing")
	// ErrAccountHasCharacters stops a new bot whose account holds only
	// characters it has no name of.
	ErrAccountHasCharacters = errors.New("account has characters")
	// ErrNoCharacterToAdopt stops an imported bot whose account holds no
	// character bearing one of its names.
	ErrNoCharacterToAdopt = errors.New("no character to adopt")
)

// errNoCharacterSlot reports a character list without the character just
// created or selected.
var errNoCharacterSlot = errors.New("character not in CharSelectInfo")

// errCharacterUnplayable reports a character the bot will not select: one
// banned or pending deletion.
var errCharacterUnplayable = errors.New("character banned or pending deletion")

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

// chooseSlot picks the slot of ch's character by the rules of Character,
// creating it when they allow, and returns the slot index and the character
// list it indexes (the one after the creation, if any). A
// banned character is refused because the server aborts its selection
// silently; one pending deletion is refused by bot policy (the server would
// still select it).
func (g *GameConn) chooseSlot(ctx context.Context, slots []charSlot, ch Character) (int32, []charSlot, error) {
	var slot int32
	var name string
	switch {
	case ch.Name != "":
		if slot, name = findSlot(slots, []string{ch.Name}); slot < 0 {
			return 0, nil, fmt.Errorf("%q: %w", ch.Name, ErrCharacterMissing)
		}
	default:
		if slot, name = findSlot(slots, ch.Names); slot >= 0 {
			break
		}
		switch {
		case ch.NeverCreate:
			return 0, nil, fmt.Errorf("%d characters, none of %d candidate names: %w", len(slots), len(ch.Names), ErrNoCharacterToAdopt)
		case len(slots) > 0:
			return 0, nil, fmt.Errorf("%d characters, none of %d candidate names: %w", len(slots), len(ch.Names), ErrAccountHasCharacters)
		}
		var err error
		if slots, name, err = g.createCharacter(ctx, ch); err != nil {
			return 0, nil, err
		}
		if slot, _ = findSlot(slots, []string{name}); slot < 0 {
			return 0, nil, fmt.Errorf("created %q: %w", name, errNoCharacterSlot)
		}
	}
	if t := slots[slot].DeleteTimer; t != 0 {
		return 0, nil, fmt.Errorf("character %q (delete timer %d): %w", name, t, errCharacterUnplayable)
	}
	return slot, slots, nil
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
		if err := g.send(ctx, botwire.EncodeRequestCharacterCreate(name, ch.Class, ch.Appearance)); err != nil {
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
		slots, err := botwire.DecodeCharSelectInfo(reply)
		if err != nil {
			return nil, "", gameDecodeError("CharSelectInfo", reply, err)
		}
		return slots, name, nil
	}
	return nil, "", fmt.Errorf("all %d candidate names refused: %w", len(ch.Names), &GameStartError{Packet: "CharCreateFail", Reason: last})
}
