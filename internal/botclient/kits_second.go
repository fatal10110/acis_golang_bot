package botclient

// Class ids of the second professions with a row of their own in the plan
// (docs/strategy.md "Second professions (levels 40+)"; data/xml/classes),
// which the row issues of epic #624 share. Bishop (16), Elven Elder (30),
// Shillien Elder (43) and Warcryer (52) are in trainer.go, Prophet (17) in
// status.go.
const (
	classGladiator      int32 = 2
	classWarlord        int32 = 3
	classPaladin        int32 = 5
	classTreasureHunter int32 = 8
	classHawkeye        int32 = 9
	classSorcerer       int32 = 12
	classTempleKnight   int32 = 20
	classSwordSinger    int32 = 21
	classPlainsWalker   int32 = 23
	classSilverRanger   int32 = 24
	classSpellsinger    int32 = 27
	classShillienKnight int32 = 33
	classBladedancer    int32 = 34
	classAbyssWalker    int32 = 36
	classPhantomRanger  int32 = 37
	classSpellhowler    int32 = 40
	classDestroyer      int32 = 46
	classTyrant         int32 = 48
	classBountyHunter   int32 = 55
)

// The second professions' heals and drains ruleCritical tries before its
// potions (critSustain; owner decision 5 on #624): Life Leech (SK), Steal
// Essence (WC), Vampiric Claw (SH) and Revival (WL).
const (
	skillRevival      int32 = 181
	skillLifeLeech    int32 = 289
	skillVampiricClaw int32 = 1234
	skillStealEssence int32 = 1245
)
