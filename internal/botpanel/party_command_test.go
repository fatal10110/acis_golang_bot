package botpanel

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// TestPartyCommandRawRoute checks that the raw command route refuses the
// party commands: they reach a bot only through Bot.Command.
func TestPartyCommandRawRoute(t *testing.T) {
	for _, s := range []string{"lead A B", "join L A B", "join L", "unparty"} {
		if c, err := parseCommand(s); !errors.Is(err, botclient.ErrBadCommand) {
			t.Errorf("parseCommand(%q) = %q, %v; want ErrBadCommand", s, c, err)
		}
	}
	if c, err := parseCommand("logout"); err != nil || c.Kind != botclient.CommandLogout {
		t.Errorf("parseCommand(logout) = %q, %v", c, err)
	}
}
