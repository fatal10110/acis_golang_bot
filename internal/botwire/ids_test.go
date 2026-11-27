package botwire

// Game data the codec tests use as field values, copied from the brain's
// constants in botclient (the file of each is named): the wire carries them
// as plain numbers.
const (
	adenaID             int32 = 57   // travel.go: the adena item template
	lesserHealingPotion int32 = 1060 // shop.go
	spiritshotNoGrade   int32 = 2509 // shop.go
	skillShield         int32 = 1040 // trainer.go
	skillMight          int32 = 1068 // trainer.go

	msgInvitedToClan          = 67   // clan.go: S1HasInvitedYouToJoinTheClanS2
	msgDeathPenaltyLevelAdded = 1916 // deathpenalty.go

	chatClan          int32  = 4 // craftorder.go: the Say2 channel of clan chat
	acquireSkillUsual int32  = 0 // trainer.go: the class trainer's skill list
	clanWarehouseType uint16 = 2 // warehouse.go: a clan warehouse list
)
