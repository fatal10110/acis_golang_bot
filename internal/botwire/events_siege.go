package botwire

// The siege packets the bot reads (serverpackets/siege.go, userinfo.go;
// docs/pvp/siege.md).

// The bits of UserInfo's relation field (userinfo.go userInfoRelation): the
// clan leader's, then a siege attacker's and a siege defender's. The
// attacker's bits hold the defender's.
const (
	UserRelationLeader        int32 = 0x40
	UserRelationSiegeAttacker int32 = 0x180
	UserRelationSiegeDefender int32 = 0x80
)

// SiegeSide is a character's siege state: none, or the side it fights on.
type SiegeSide uint8

const (
	SiegeNone SiegeSide = iota
	SiegeAttacker
	SiegeDefender
)

// SiegeSide is the bot's own siege state from its relation field: the
// server sets it on every online member of a registered clan as the siege
// starts and clears it at the end (siege/lifecycle.go siegeStatesLocked),
// and only UserInfo shows it to the player itself.
func (u UserInfo) SiegeSide() SiegeSide {
	switch u.Relation & UserRelationSiegeAttacker {
	case UserRelationSiegeAttacker:
		return SiegeAttacker
	case UserRelationSiegeDefender:
		return SiegeDefender
	}
	return SiegeNone
}

// SiegeClan is one clan row of a siege list. Side is the defender list's
// side code (SiegeDefenderOwner, ...), 0 in the attacker list.
type SiegeClan struct {
	ClanID    int32
	Name      string
	Leader    string
	CrestID   int32
	Side      int32
	AllyID    int32
	AllyName  string
	AllyCrest int32
}

// The defender list's side codes (serverpackets/siege.go).
const (
	SiegeDefenderOwner    int32 = 1
	SiegeDefenderPending  int32 = 2
	SiegeDefenderApproved int32 = 3
)

// SiegeAttackerList answers RequestSiegeAttackerList: the attacking clans
// of castle (or siegable clan hall) CastleID.
type SiegeAttackerList struct {
	CastleID int32
	Clans    []SiegeClan
}

// SiegeDefenderList answers RequestSiegeDefenderList: the defending clans
// of castle CastleID, the owner and the approved defenders first, then the
// clans waiting for approval.
type SiegeDefenderList struct {
	CastleID int32
	Clans    []SiegeClan
}

func (SiegeAttackerList) IsEvent() {}

func (SiegeDefenderList) IsEvent() {}
