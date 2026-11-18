package e2e

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// raceClasses is the first starting class of each race.
func raceClasses() []botclient.StartingClass {
	var out []botclient.StartingClass
	for _, c := range botclient.StartingClasses() {
		if !slices.ContainsFunc(out, func(o botclient.StartingClass) bool { return o.Race == c.Race }) {
			out = append(out, c)
		}
	}
	return out
}

// enterOnce enters, checks the account holds one character, logs out and
// returns the character's name and class.
func enterOnce(t *testing.T, tg *Target, account string, ch botclient.Character) (string, int32) {
	t.Helper()
	ctx := scenarioCtx(t, 2*time.Minute)
	c, err := tg.EnterWhenFree(ctx, account, ch, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	w, err := botclient.NewWorld(c.Self, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.Self.Slots != 1 {
		t.Errorf("account %s holds %d characters, want 1", account, c.Self.Slots)
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatalf("logout %s: %v", account, err)
	}
	return c.Self.Name, w.Self.ClassID
}

// TestLiveCharacterClasses creates a character of each race with a marked
// name (adopted on later runs), then one whose first name is taken: the
// server answers CharCreateFail 2 and the next name is created (on the run
// that creates it). A relogin selects the first by name (#10).
func TestLiveCharacterClasses(t *testing.T) {
	t.Parallel()
	tg := Live(t)
	var first string
	for i, class := range raceClasses() {
		account, _ := tg.Account("c", i+1)
		ch := character(t, account, &class)
		name, classID := enterOnce(t, tg, account, ch)
		if !slices.Contains(ch.Names, name) || classID != class.ID {
			t.Fatalf("account %s entered as %q class %d, want one of %q class %d", account, name, classID, ch.Names, class.ID)
		}
		t.Logf("%s: %s, race %d class %d", account, name, class.Race, classID)
		if i == 0 {
			first = name
		}
	}

	account, _ := tg.Account("c", 6)
	ch := character(t, account, nil)
	ch.Names = append([]string{first}, ch.Names[:botclient.MaxNameCandidates-1]...)
	if name, _ := enterOnce(t, tg, account, ch); name != ch.Names[1] {
		t.Fatalf("account %s asking for the taken %q first entered as %q, want %q", account, first, name, ch.Names[1])
	}

	account, _ = tg.Account("c", 1)
	if name, _ := enterOnce(t, tg, account, botclient.Character{Name: first}); name != first {
		t.Fatalf("relogin by name entered as %q, want %q", name, first)
	}
}

// TestLivePinning runs the character choice rules against the server
// (#92): a known name selects only that character, a never-create bot
// adopts a character bearing one of its names or stops, and a bot that may
// create stops on an account holding other characters.
func TestLivePinning(t *testing.T) {
	t.Parallel()
	tg := Live(t)
	account, _ := tg.Account("c", 1)
	class := raceClasses()[0]
	name, _ := enterOnce(t, tg, account, character(t, account, &class))
	const absent = "NoSuchEtwoe"

	if got, _ := enterOnce(t, tg, account, botclient.Character{Names: []string{absent, name}, NeverCreate: true}); got != name {
		t.Fatalf("adoption entered as %q, want %q", got, name)
	}
	ctx := scenarioCtx(t, 3*time.Minute)
	other := character(t, account, &class)
	other.Names = []string{absent}
	tests := []struct {
		name string
		ch   botclient.Character
		want error
	}{
		{"known name missing", botclient.Character{Name: absent}, botclient.ErrCharacterMissing},
		{"never-create adopts none", botclient.Character{Names: []string{absent}, NeverCreate: true}, botclient.ErrNoCharacterToAdopt},
		{"account holds another", other, botclient.ErrAccountHasCharacters},
	}
	for _, tt := range tests {
		c, err := tg.EnterWhenFree(ctx, account, tt.ch, nil)
		if err == nil {
			c.Close()
		}
		if !errors.Is(err, tt.want) {
			t.Fatalf("%s: Enter = %v, want %v", tt.name, err, tt.want)
		}
	}
}
