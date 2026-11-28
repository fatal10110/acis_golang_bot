package botwire

// The Olympiad match packets a competitor gets (serverpackets/olympiad_match.go).

// The modes ExOlympiadMode switches the client to (serverpackets
// OlympiadMode*): none, a competitor's side, or a spectator's.
const (
	OlympiadModeNone     uint8 = 0
	OlympiadModeSide1    uint8 = 1
	OlympiadModeSide2    uint8 = 2
	OlympiadModeObserver uint8 = 3
)

// ExOlympiadMode switches the bot's client into an Olympiad mode: its side
// as it enters a stadium (network/olympiad_match.go Enter), none as the
// match releases it (Release).
type ExOlympiadMode struct{ Mode uint8 }

// ExOlympiadMatchEnd closes the match window; the server sends it right
// after 284 as the competitor leaves the stadium (stadiumCombatNotice).
type ExOlympiadMatchEnd struct{}

// ExOlympiadUserInfo shows one competitor's gauges in the match window, at
// the fight's start and on every change: Side is its side (1 or 2), the
// gauges narrowed to whole numbers.
type ExOlympiadUserInfo struct {
	Side      uint8
	ObjectID  int32
	Name      string
	ClassID   int32
	HP, MaxHP int32
	CP, MaxCP int32
}

func (ExOlympiadMode) IsEvent() {}

func (ExOlympiadMatchEnd) IsEvent() {}

func (ExOlympiadUserInfo) IsEvent() {}
