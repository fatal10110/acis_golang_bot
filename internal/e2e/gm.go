package e2e

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// maxGMFileSize bounds the GM file.
const maxGMFileSize = 4 << 10

// GM is the run's GM: an existing account whose character has the admin
// access level, read from the GM file. It formats as its account and
// character only.
type GM struct {
	Account   string
	password  string
	Character string
}

// Password is the GM account's password. Never log it.
func (g GM) Password() string { return g.password }

// Format prints the account and character, never the password.
func (g GM) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "%s/%s", g.Account, g.Character)
}

// LoadGM reads the GM file at path: a regular file of mode 0600 (or
// stricter) of three key=value lines, account, password and character.
// Blank lines and lines starting with # are skipped. No error carries the
// password.
func LoadGM(path string) (GM, error) {
	f, err := os.Open(path)
	if err != nil {
		return GM{}, fmt.Errorf("gm file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return GM{}, fmt.Errorf("gm file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return GM{}, fmt.Errorf("gm file %s: not a regular file", path)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return GM{}, fmt.Errorf("gm file %s: mode %#o, want 0600", path, perm)
	}
	var g GM
	sc := bufio.NewScanner(io.LimitReader(f, maxGMFileSize))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		var dst *string
		switch key {
		case "account":
			dst = &g.Account
		case "password":
			dst = &g.password
		case "character":
			dst = &g.Character
		}
		if !ok || dst == nil {
			return GM{}, fmt.Errorf("gm file %s: line %d: want account=, password= or character=", path, n)
		}
		*dst = value
	}
	if err := sc.Err(); err != nil {
		return GM{}, fmt.Errorf("gm file %s: %w", path, err)
	}
	if g.Account == "" || g.password == "" || g.Character == "" {
		return GM{}, fmt.Errorf("gm file %s: want account, password and character, all set", path)
	}
	return g, nil
}

// ErrCommandRefused reports a GM command the harness does not send: a form
// it does not know, or one that could touch something the run does not own.
var ErrCommandRefused = errors.New("GM command refused")

// Scope is what a run's GM commands may touch: the run's e2e characters,
// the monsters the run spawned, and the GM's current target, which the
// harness alone sets (GMClient.Select). It is not safe for concurrent use;
// its Target guards it.
type Scope struct {
	chars  map[string]int32 // lowercased name -> object id, 0 unknown
	npcs   map[int32]bool
	target int32 // 0: none
}

func newScope() *Scope {
	return &Scope{chars: make(map[string]int32), npcs: make(map[int32]bool)}
}

// addChar makes name one of the run's characters; a non-zero objectID
// replaces an unknown one.
func (s *Scope) addChar(name string, objectID int32) {
	key := strings.ToLower(name)
	if objectID != 0 || s.chars[key] == 0 {
		s.chars[key] = objectID
	}
}

func (s *Scope) isChar(name string) bool {
	_, ok := s.chars[strings.ToLower(name)]
	return ok
}

func (s *Scope) isCharID(objectID int32) bool {
	if objectID == 0 {
		return false
	}
	for _, id := range s.chars {
		if id == objectID {
			return true
		}
	}
	return false
}

// owns reports an object the GM may target.
func (s *Scope) owns(objectID int32) bool { return s.npcs[objectID] || s.isCharID(objectID) }

// check refuses cmd, a command line without its "//", unless it is one of
// the forms below and touches only what s owns. Every server-wide form
// (announce, shutdown, reload, kick all, recall party|clan, a radius) and
// every other command is refused.
//
//	teleport X Y Z       moves the GM itself
//	teleportto NAME      moves the GM to a run character
//	recall NAME          moves a run character to the GM
//	kick NAME            disconnects a run character
//	kill|res|heal [NAME] a run character, or with no name the GM's target
//	set level N          the GM's target, a run character
//	skill set ID LEVEL   the GM's target, a run character; also skill set all
//	spawn ID             a monster at the GM's target or the GM; it never respawns
//	delete               the GM's target, a monster the run spawned
func (s *Scope) check(cmd string) error {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return fmt.Errorf("%w: empty", ErrCommandRefused)
	}
	word, args := strings.ToLower(f[0]), f[1:]
	ok := false
	switch word {
	case "teleport":
		ok = len(args) == 3 && allInts(args)
	case "teleportto", "recall", "kick":
		ok = len(args) == 1 && s.isChar(args[0])
	case "kill", "res", "heal":
		ok = len(args) == 1 && s.isChar(args[0]) || len(args) == 0 && s.owns(s.target)
	case "set":
		ok = len(args) == 2 && args[0] == "level" && allInts(args[1:]) && s.isCharID(s.target)
	case "skill":
		ok = (len(args) == 3 && args[0] == "set" && allInts(args[1:]) ||
			len(args) == 2 && args[0] == "set" && args[1] == "all") && s.isCharID(s.target)
	case "spawn":
		ok = len(args) == 1 && allInts(args)
	case "delete":
		ok = len(args) == 0 && s.npcs[s.target]
	}
	if !ok {
		return fmt.Errorf("%w: %q", ErrCommandRefused, cmd)
	}
	return nil
}

func allInts(ss []string) bool {
	for _, s := range ss {
		if _, err := strconv.Atoi(s); err != nil {
			return false
		}
	}
	return true
}
