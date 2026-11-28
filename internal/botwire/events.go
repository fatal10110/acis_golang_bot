package botwire

// Event is one decoded server packet, handed from a session's reader to its
// brain through Events. Every event is a value type that shares no memory
// with the frame it was decoded from: the reader reuses its frame buffer, so
// no slice of it may cross goroutines. The brain type-switches on the
// concrete types below.
type Event interface{ IsEvent() }

// Point is a world coordinate.
type Point struct{ X, Y, Z int32 }

// MoveToLocation reports an object starting to move from Origin toward Dest.
// It is the one event Events coalesces: a newer one for the same object
// supersedes a queued one.
type MoveToLocation struct {
	ObjectID     int32
	Dest, Origin Point
}

// StopMove reports an object stopped at Pos.
type StopMove struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// ValidateLocation corrects an object's position to Pos.
type ValidateLocation struct {
	ObjectID int32
	Pos      Point
	Heading  int32
}

// TeleportToLocation reports an object placed at Pos without walking there.
// Fast is an in-place correction rather than a loading-screen teleport.
type TeleportToLocation struct {
	ObjectID int32
	Pos      Point
	Fast     bool
}

// DeleteObject reports an object leaving sight. Seated marks a sitting
// character the client stands up before removing it.
type DeleteObject struct {
	ObjectID int32
	Seated   bool
}

// LeaveWorld is the server's answer to an accepted Logout; it closes the
// connection right after.
type LeaveWorld struct{}

// ServerClose is the server's last packet before it drops the connection.
type ServerClose struct{}

// AbnormalStealth is the abnormal effect bit of an invisible game master
// in its own UserInfo (server skill.AbnormalStealth, userinfo.go).
const AbnormalStealth int32 = 0x100000

// UserInfo is the bot's own character in full. The server sends it on
// entering the world and again whenever the character changes.
type UserInfo struct {
	ObjectID int32
	Name     string
	Pos      Point
	Heading  int32
	// BaseClass is the class drawn on the model, ClassID the class played.
	Race, Sex, BaseClass, ClassID int32
	Level                         int32
	Exp                           int64
	SP                            int32
	HP, MaxHP                     int32
	MP, MaxMP                     int32
	CP, MaxCP                     int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad  int32
	PvPFlag, Karma int32
	// InvLimit is the inventory limit: how many items the character may
	// hold, worn ones included (80, 100 for a dwarf, plus bonuses).
	InvLimit int32
	// RunSpeed and WalkSpeed are effective speeds in world units per
	// second: the base speed the packet carries times its movement
	// multiplier, as the client computes them. SwimSpeed is the speed in
	// water, run or walk alike (userinfo.go writes the base swim speed
	// twice; the server's Character.MoveSpeed swims at SwimSpeed in water).
	RunSpeed, WalkSpeed, SwimSpeed int32
	Running                        bool
	// Noble is set for a noble (userinfo.go IsNoble): only nobles enter the
	// Olympiad.
	Noble bool
	// PAtkSpd is the attack speed a melee swing lasts by: about
	// 500000/PAtkSpd ms (the server's formulas.TimeBetweenAttacks).
	PAtkSpd int32
	// PAtk, PDef, Evasion and Accuracy are the character's final combat
	// stats, gear and buffs counted: what it weighs a fight with
	// (Brain.packFight).
	PAtk, PDef, Evasion, Accuracy int32
	// STR, DEX, CON, INT, WIT and MEN are the base stats; Critical, MAtk,
	// CastSpd (the magic attack speed) and MDef the rest of the combat
	// stats, gear and buffs counted. They are kept, not yet read by a rule
	// (docs/strategy.md, Design 1).
	STR, DEX, CON, INT, WIT, MEN  int32
	Critical, MAtk, CastSpd, MDef int32
	// CollisionHeight is the character's; the server adds it to the Z of
	// every ground click (network/movement.go moveLivePlayer).
	// CollisionRadius is its own reach margin in a distance check.
	CollisionRadius, CollisionHeight float64
	// StoreType is the private store mode (the server's operate type; 0
	// for none).
	StoreType uint8
	IsGM      bool
	// ClanID is the character's clan, AllyID its clan's alliance, 0 for
	// none.
	ClanID, AllyID int32
	// Relation is the character's own relation field: the clan leader bit
	// and the bits of its siege state (userinfo.go userInfoRelation),
	// read by SiegeSide.
	Relation int32
	// Abnormal is the abnormal effect mask drawn on the character; an
	// invisible game master sees AbnormalStealth in its own.
	Abnormal int32
	// PKKills and PvPKills count the players it killed without and with
	// a flag.
	PKKills, PvPKills int32
}

// StatusFields is a set of StatusUpdate fields.
type StatusFields uint16

// The StatusUpdate fields the model keeps; the packet's other attributes
// (attack and cast speed) are consumed and dropped.
const (
	StatusHP StatusFields = 1 << iota
	StatusMaxHP
	StatusMP
	StatusMaxMP
	StatusCP
	StatusMaxCP
	StatusSP
	StatusLoad
	StatusMaxLoad
	StatusKarma
)

// StatusUpdate reports new values for some of an object's attributes: Set
// names the fields the packet carried; the others are zero and unchanged.
type StatusUpdate struct {
	ObjectID  int32
	Set       StatusFields
	HP, MaxHP int32
	MP, MaxMP int32
	CP, MaxCP int32
	SP        int32
	// Load and MaxLoad are the carried weight and the weight limit.
	Load, MaxLoad int32
	Karma         int32
}

// CharInfo describes another player in sight.
type CharInfo struct {
	ObjectID int32
	Name     string
	Pos      Point
	Heading  int32
	ClassID  int32
	PvPFlag  int32
	Karma    int32
	// RunSpeed and WalkSpeed are effective speeds, as in UserInfo.
	RunSpeed, WalkSpeed int32
	Sitting, Running    bool
	InCombat, Dead      bool
	// StoreType is the private store mode, as in UserInfo.
	StoreType uint8
	// RHand is the template id of the weapon in its right hand (a two-handed
	// weapon too), LHand that of the shield, or of the arrows or lure paired
	// with a bow or rod; LHand is 0 under any other two-handed weapon. 0 for
	// none.
	RHand, LHand int32
	// CollisionRadius is its reach margin in a distance check.
	CollisionRadius float64
	// ClanID and AllyID are its clan and alliance, 0 for none.
	ClanID, AllyID int32
	// Abnormal is the abnormal effect mask drawn on it, as in UserInfo.
	Abnormal  int32
	CP, MaxCP int32
	// DuelTeam is its side in a duel (1 blue, 2 red), 0 outside one; a
	// spawn-protected player shows blue.
	DuelTeam uint8
}

// NPCInfo describes an NPC (a monster, a townsperson or a summon) in sight.
type NPCInfo struct {
	ObjectID int32
	// NPCID is the template id the wire shows: the template's idTemplate,
	// which is its own id for all but a few custom NPCs that borrow another
	// template's look (Template).
	NPCID      int32
	Attackable bool
	Pos        Point
	Heading    int32
	// RunSpeed and WalkSpeed are effective speeds, as in UserInfo.
	RunSpeed, WalkSpeed int32
	Running, InCombat   bool
	Dead, Summon        bool
	PvPFlag, Karma      int32
	// CollisionRadius is its reach margin in a distance check.
	CollisionRadius float64
	// Name and Title are the server-side name and title, "" unless the
	// template uses them (usingServerSideName and usingServerSideTitle,
	// npc/folk.go infoSnapshot): the client shows its own otherwise.
	Name, Title string
}

// npcLook is how the wire shows an NPC: its display template id and its
// server-side name.
type npcLook struct {
	npcID int32
	name  string
}

// ClassMaster is the class master's NPC template id (Hasha, the Class
// Manager, data/xml/npcs/50000-50999.xml; D15).
const ClassMaster int32 = 50000

// borrowedLooks are the custom NPCs whose wire template id is another
// NPC's, by the server-side name that tells them apart: the class master
// (50000, idTemplate 31228, data/xml/npcs/50000-50999.xml) shows as Roy the
// Cat (31228), who stands in most villages and has no server-side name.
var borrowedLooks = map[npcLook]int32{
	{npcID: 31228, name: "Hasha"}: ClassMaster,
}

// Template is the NPC's template id: NPCID, or for an NPC that borrows
// another template's look (borrowedLooks), its own.
func (n NPCInfo) Template() int32 {
	if id, ok := borrowedLooks[npcLook{npcID: n.NPCID, name: n.Name}]; ok {
		return id
	}
	return n.NPCID
}

// MoveToPawn reports an object starting to move from Origin toward
// TargetID until it is Distance away from it.
type MoveToPawn struct {
	ObjectID, TargetID int32
	Distance           int32
	Origin             Point
}

// RestartPoints are the places a dead character may choose to restart at.
type RestartPoints struct {
	Village, ClanHall, Castle, SiegeHQ bool
	// Fixed is a restart in place.
	Fixed bool
}

// Die reports an object's death. For the bot's own character, Restart lists
// the restart points it is offered.
type Die struct {
	ObjectID  int32
	Restart   RestartPoints
	Sweepable bool
}

// Revive reports an object brought back to life.
type Revive struct{ ObjectID int32 }

// MyTargetSelected confirms the bot's own target; Color is the level
// difference color the client draws.
type MyTargetSelected struct {
	ObjectID int32
	Color    uint16
}

// TargetUnselected reports that the object ObjectID, standing at Pos,
// cleared its target.
type TargetUnselected struct {
	ObjectID int32
	Pos      Point
}

// SystemMessage is a system message by id. Text holds its first two text
// parameters in order, "" past the ones it has, and Number its first
// number parameter, 0 without one; every other parameter is consumed and
// dropped.
type SystemMessage struct {
	ID     int32
	Text   [2]string
	Number int32
}

// ActionFailed tells the client its last action request was not carried
// out.
type ActionFailed struct{}

// CreatureSay is a chat line on channel Type, said by ObjectID (0 for none)
// under Name.
type CreatureSay struct {
	ObjectID, Type int32
	Name, Text     string
}

// WaitType is a ChangeWaitType stance.
type WaitType int32

// The stances ChangeWaitType reports (serverpackets/stance_social.go).
const (
	WaitSitting        WaitType = 0
	WaitStanding       WaitType = 1
	WaitFakeDeathStart WaitType = 2
	WaitFakeDeathStop  WaitType = 3
)

// ChangeWaitType reports an object at Pos changing stance.
type ChangeWaitType struct {
	ObjectID int32
	Type     WaitType
	Pos      Point
}

// AutoAttackStart reports an object entering its combat stance.
type AutoAttackStart struct{ ObjectID int32 }

// AutoAttackStop reports an object leaving its combat stance.
type AutoAttackStop struct{ ObjectID int32 }

func (MoveToLocation) IsEvent() {}

func (StopMove) IsEvent() {}

func (ValidateLocation) IsEvent() {}

func (TeleportToLocation) IsEvent() {}

func (DeleteObject) IsEvent() {}

func (LeaveWorld) IsEvent() {}

func (ServerClose) IsEvent() {}

func (UserInfo) IsEvent() {}

func (StatusUpdate) IsEvent() {}

func (CharInfo) IsEvent() {}

func (NPCInfo) IsEvent() {}

func (MoveToPawn) IsEvent() {}

func (Die) IsEvent() {}

func (Revive) IsEvent() {}

func (MyTargetSelected) IsEvent() {}

func (TargetUnselected) IsEvent() {}

func (SystemMessage) IsEvent() {}

func (ActionFailed) IsEvent() {}

func (CreatureSay) IsEvent() {}

func (ChangeWaitType) IsEvent() {}

func (AutoAttackStart) IsEvent() {}

func (AutoAttackStop) IsEvent() {}
