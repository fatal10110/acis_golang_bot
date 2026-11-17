package botclient

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botnames"
)

// pinned is one call of an entry's chosen callback.
type pinned struct {
	name    string
	classID int32
}

// TestPinningEnterGame drives the character rules of Character through
// EnterGame: a known name is selected or stops the bot, a new bot adopts
// its derived name before it creates and creates only on an empty account,
// an imported bot adopts or stops, and chosen reports the name and the
// slot's class id before RequestGameStart. A RequestCharacterCreate the
// script does not expect fails the fake server.
func TestPinningEnterGame(t *testing.T) {
	const objectID = 0x10000001
	authLogin := recvStep(packet(0x08, "bot7", 4, 3, 1, 2))
	list := func(slots ...charSlot) step { return sendStep(charSelectInfoPacket(slots...)) }
	created := sendStep(packet(0x19, 1))
	enter := func(slot int, name string) []step {
		return []step{
			recvStep(packet(0x0d, slot, uint16(0), 0, 0, 0)),
			sendStep(packet(0x15, name, objectID)),
			recvStep([]byte{0x03}),
			sendStep(packet(0x04, -71000, 258000, -3100, 49152, objectID, name)),
		}
	}
	// noClass is no starting class: a known-name or imported bot carries
	// none and is not refused for it.
	noClass := StartingClass{ID: 1, Race: RaceHuman}
	newBot := Character{
		Names:      []string{"DraelBot", "KiaBot"},
		Class:      OrcMystic,
		Appearance: Appearance{HairStyle: 4, HairColor: 2, Face: 1},
	}
	// The orc mystic of newBot: race 3, male, class 49, six zero stats,
	// hair style, hair color, face (requestcharactercreate.go).
	create := recvStep(packet(0x0b, "DraelBot", 3, 0, 49, 0, 0, 0, 0, 0, 0, 4, 2, 1))
	known := Character{Name: "KiaBot", Class: noClass, Appearance: Appearance{Sex: 2}}
	imported := Character{Names: []string{"DraelBot", "KiaBot"}, NeverCreate: true, Class: noClass}
	other := charSlot{Name: "SomeoneElse", ClassID: 10}
	tests := []struct {
		name     string
		ch       Character
		script   []step
		wantName string
		wantPin  []pinned
		wantErr  error
	}{
		{"known name selected", known, slices.Concat([]step{authLogin, list(other, charSlot{Name: "KiaBot", ClassID: 25})}, enter(1, "KiaBot")), "KiaBot", []pinned{{"KiaBot", 25}}, nil},
		{"known name missing", known, []step{authLogin, list(other, charSlot{Name: "DraelBot"})}, "", nil, ErrCharacterMissing},
		{"known name missing on an empty account", known, []step{authLogin, list()}, "", nil, ErrCharacterMissing},
		{"new bot adopts a later name", newBot, slices.Concat([]step{authLogin, list(other, charSlot{Name: "KiaBot", ClassID: 39})}, enter(1, "KiaBot")), "KiaBot", []pinned{{"KiaBot", 39}}, nil},
		{"new bot creates on an empty account", newBot, slices.Concat([]step{authLogin, list(), create, created, list(charSlot{Name: "DraelBot", ClassID: 49})}, enter(0, "DraelBot")), "DraelBot", []pinned{{"DraelBot", 49}}, nil},
		{"new bot beside other characters", newBot, []step{authLogin, list(other)}, "", nil, ErrAccountHasCharacters},
		{"imported bot adopts", imported, slices.Concat([]step{authLogin, list(other, charSlot{Name: "DraelBot", ClassID: 44})}, enter(1, "DraelBot")), "DraelBot", []pinned{{"DraelBot", 44}}, nil},
		{"imported bot beside other characters", imported, []step{authLogin, list(other)}, "", nil, ErrNoCharacterToAdopt},
		{"imported bot on an empty account", imported, []step{authLogin, list()}, "", nil, ErrNoCharacterToAdopt},
		// The connection drops between CharCreateOk and UserInfo: the name
		// was reported before RequestGameStart.
		{"drop after creation", newBot, []step{authLogin, list(), create, created, list(charSlot{Name: "DraelBot", ClassID: 49})}, "", []pinned{{"DraelBot", 49}}, io.ErrClosedPipe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, done := dialFake(t, true, tt.script, tt.wantErr != nil)
			var got []pinned
			chosen := func(name string, classID int32) { got = append(got, pinned{name, classID}) }
			self, err := EnterGame(t.Context(), g, "bot7", SessionKeys{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}, tt.ch, chosen)
			g.Close()
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}
			switch {
			case tt.wantErr == nil:
				if err != nil || self.Name != tt.wantName || self.ObjectID != objectID {
					t.Fatalf("EnterGame = %+v, %v; want %q", self, err, tt.wantName)
				}
			case tt.wantErr == io.ErrClosedPipe:
				if err == nil {
					t.Fatal("EnterGame = nil error, want the dropped connection")
				}
			case !errors.Is(err, tt.wantErr):
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.wantPin) {
				t.Fatalf("chosen calls %v, want %v", got, tt.wantPin)
			}
		})
	}
}

// TestPinningValidate: the class and appearance checks apply only to a
// bot that may create; a known name must be a valid one.
func TestPinningValidate(t *testing.T) {
	bad := StartingClass{ID: 1, Race: RaceHuman}
	tests := []struct {
		name    string
		ch      Character
		wantErr string
	}{
		{"known name, no starting class", Character{Name: "KiaBot", Class: bad, Appearance: Appearance{Sex: 2}}, ""},
		{"known name invalid", Character{Name: "Kia Bot"}, "letters or digits"},
		{"never create, no starting class", Character{Names: []string{"KiaBot"}, NeverCreate: true, Class: bad, Appearance: Appearance{Face: 3}}, ""},
		{"never create needs names", Character{NeverCreate: true}, "candidate names"},
		{"may create, no starting class", Character{Names: []string{"KiaBot"}, Class: bad}, "not a starting class"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ch.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			cfg := SessionConfig{Clock: newFakeClock(t0)}
			_, err = NewBot(cfg, "bot7", "not-a-secret", BotConfig{Character: tt.ch}, rand.New(rand.NewPCG(1, 2)))
			if (err == nil) != (tt.wantErr == "") {
				t.Fatalf("NewBot err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestPinningNewCharacter: a new bot's names derive from its account and
// marker, the same on every call, and its drawn class and look are valid.
func TestPinningNewCharacter(t *testing.T) {
	m := botnames.Marker{Text: "X1", Placement: botnames.Prefix}
	a, err := NewCharacter(m, "hz7", rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewCharacter(m, "hz7", rand.New(rand.NewPCG(9, 9)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); err != nil || !slices.Equal(a.Names, b.Names) || len(a.Names) != MaxNameCandidates {
		t.Fatalf("names %v and %v (%v), want the same %d", a.Names, b.Names, err, MaxNameCandidates)
	}
	for _, n := range a.Names {
		if !m.Marks(n) {
			t.Fatalf("name %q not marked by %v", n, m)
		}
	}
	if _, err := NewCharacter(botnames.Marker{}, "hz7", rand.New(rand.NewPCG(1, 2))); err == nil {
		t.Fatal("NewCharacter with no marker = nil error")
	}
}

// TestPinningBot: the name a session chose is pinned before it enters, so
// a drop before the world and every reconnect log in by it; it is reported
// once with the slot's class id; a pinned character gone from the account
// stops the bot instead of creating one.
func TestPinningBot(t *testing.T) {
	drop := errors.New("read CharSelected: EOF")
	missing := fmt.Errorf("%q: %w", "DraelBot", ErrCharacterMissing)
	tests := []struct {
		name string
		// enter scripts the entries: the error of each, after chosen when
		// pin is set.
		enter    []error
		pin      []bool
		games    []gameScript
		cancelAt int
		want     error
		// wantChars is the character name each entry asked for.
		wantChars []string
	}{
		{"drop before the world, then a retry", []error{drop, nil}, []bool{true, true}, nil, 1, nil, []string{"", "DraelBot"}},
		{"reconnect after a session", []error{nil, nil}, []bool{true, true}, []gameScript{abortEnd}, 2, nil, []string{"", "DraelBot"}},
		{"deleted before the reconnect", []error{nil, missing}, []bool{true, false}, []gameScript{abortEnd}, 0, ErrCharacterMissing, []string{"", "DraelBot"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked []string
			n := &fakeNet{games: tt.games, cancelAt: tt.cancelAt}
			n.onEnter = func(i int, ch Character, chosen func(string, int32)) error {
				asked = append(asked, ch.Name)
				if tt.pin[i] {
					chosen("DraelBot", 49)
				}
				return tt.enter[i]
			}
			b, clock, ctx := startBot(t, true, n)
			var learned []pinned
			b.learned = func(name string, classID int32) { learned = append(learned, pinned{name, classID}) }
			err := runBot(ctx, b, clock)
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Run = %v, want %v", err, tt.want)
			}
			if !slices.Equal(asked, tt.wantChars) {
				t.Errorf("entries asked for %q, want %q", asked, tt.wantChars)
			}
			if want := []pinned{{"DraelBot", 49}}; !slices.Equal(learned, want) {
				t.Errorf("learned %v, want %v", learned, want)
			}
			checkJoined(t, n)
		})
	}
}

// TestPinningStops: each character choice that cannot succeed stops the
// bot at once, with one login.
func TestPinningStops(t *testing.T) {
	for _, want := range []error{ErrCharacterMissing, ErrAccountHasCharacters, ErrNoCharacterToAdopt} {
		t.Run(want.Error(), func(t *testing.T) {
			n := &fakeNet{enters: []error{fmt.Errorf("enter game: %w", want)}}
			b, clock, ctx := startBot(t, true, n)
			if err := runBot(ctx, b, clock); !errors.Is(err, want) {
				t.Fatalf("Run = %v, want %v", err, want)
			}
			if n.nLogin != 1 {
				t.Errorf("logins = %d, want 1", n.nLogin)
			}
		})
	}
}
