package botclient

import "testing"

// TestDefaultPersonaOlympiad checks the owner's ruling on #729 (D42): the
// default persona lets a third-profession bot compete, and Olympiad itself
// stays off until a bot turns it on.
func TestDefaultPersonaOlympiad(t *testing.T) {
	p := DefaultPersona()
	if p.PvP.Olympiad {
		t.Error("DefaultPersona: PvP.Olympiad on, want off (opt-in per bot, D42)")
	}
	if !p.Olympiad.ThirdClass {
		t.Error("DefaultPersona: Olympiad.ThirdClass off, want on (#729)")
	}
}

// TestOlympiadOnThirdClass checks olympiadOn for a third-profession bot: with
// the default policy it plays once Olympiad is on, and never with it off.
func TestOlympiadOnThirdClass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		olympiad bool
		third    bool
		want     bool
	}{
		{name: "default policy, Olympiad on", olympiad: true, third: true, want: true},
		{name: "default policy, Olympiad off", olympiad: false, third: true, want: false},
		{name: "ThirdClass off, Olympiad on", olympiad: true, third: false, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newClassBrain(t, olyCardinal, 76)
			b.persona.PvP.Olympiad = tc.olympiad
			b.persona.Olympiad.ThirdClass = tc.third
			if got := b.olympiadOn(); got != tc.want {
				t.Errorf("olympiadOn() = %v, want %v", got, tc.want)
			}
		})
	}
}
