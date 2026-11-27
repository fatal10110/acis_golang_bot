package botwire

// The duel window packets (serverpackets/duel.go). Party is set for a party
// duel.

// ExDuelAskStart asks the bot whether it takes Requester's challenge.
type ExDuelAskStart struct {
	Requester string
	Party     bool
}

// ExDuelReady opens the duel window; the server sends it right before
// ExDuelStart, once the countdown is over (network/duel.go startDuel).
type ExDuelReady struct{ Party bool }

// ExDuelStart starts the duel.
type ExDuelStart struct{ Party bool }

// ExDuelEnd ends the duel.
type ExDuelEnd struct{ Party bool }

// ExDuelUpdateUserInfo refreshes one opponent in the duel window: its
// gauges as the server narrows them to whole numbers.
type ExDuelUpdateUserInfo struct {
	Name           string
	ObjectID       int32
	ClassID, Level int32
	HP, MaxHP      int32
	MP, MaxMP      int32
	CP, MaxCP      int32
}

// MagicSkillCanceled reports that CasterID's cast in progress stopped
// before it hit (network/magic_skill.go broadcastCastAborted), an Esc
// cancel inside the interrupt window among others (network/targeting.go).
type MagicSkillCanceled struct{ CasterID int32 }

func (ExDuelAskStart) IsEvent() {}

func (ExDuelReady) IsEvent() {}

func (ExDuelStart) IsEvent() {}

func (ExDuelEnd) IsEvent() {}

func (ExDuelUpdateUserInfo) IsEvent() {}

func (MagicSkillCanceled) IsEvent() {}
